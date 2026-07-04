package anthropic

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/provider"
)

func TestCapabilities(t *testing.T) {
	c := (&Provider{}).Capabilities()
	if !c.Thinking || !c.PromptCaching || !c.ParallelTools {
		t.Fatalf("anthropic capabilities = %+v, want all true", c)
	}
}

func TestMapFinish(t *testing.T) {
	cases := map[sdk.StopReason]provider.FinishReason{
		sdk.StopReasonEndTurn:      provider.FinishEndTurn,
		sdk.StopReasonToolUse:      provider.FinishToolUse,
		sdk.StopReasonMaxTokens:    provider.FinishMaxTokens,
		sdk.StopReasonRefusal:      provider.FinishBlocked,
		sdk.StopReasonStopSequence: provider.FinishEndTurn,
		sdk.StopReasonPauseTurn:    provider.FinishOther,
		"something_new":            provider.FinishOther,
	}
	for in, want := range cases {
		if got := mapFinish(in); got != want {
			t.Errorf("mapFinish(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClassifyStatusMapping(t *testing.T) {
	cases := map[int]errs.Class{
		429: errs.ProviderRateLimit,
		503: errs.ProviderServer,
		401: errs.ProviderAuth,
		400: errs.ProviderInvalid,
	}
	for code, want := range cases {
		// Only the class is asserted; *sdk.Error.Error() nil-derefs on a bare
		// struct, and errs.ClassOf never calls it.
		if got := errs.ClassOf(classify(&sdk.Error{StatusCode: code})); got != want {
			t.Errorf("classify(status %d) class = %q, want %q", code, got, want)
		}
	}
}

// toParams: thinking config is sent when requested (floored), and the system
// block carries a cache_control breakpoint.
func TestToParamsThinkingAndCache(t *testing.T) {
	req := provider.CompleteRequest{
		Model: "claude-x", MaxTokens: 4096, System: "be precise",
		Thinking: provider.ThinkingConfig{Enabled: true, BudgetTokens: 100},
		Messages: []provider.Message{{Role: provider.RoleUser,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "hi"}}}},
	}
	params, err := toParams(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)
	if !strings.Contains(js, `"cache_control"`) {
		t.Errorf("system block missing cache_control: %s", js)
	}
	if !strings.Contains(js, `"thinking"`) {
		t.Errorf("thinking config missing: %s", js)
	}
	// Budget floored to the Anthropic minimum.
	if !strings.Contains(js, `"budget_tokens":1024`) {
		t.Errorf("budget not floored to 1024: %s", js)
	}
}

// toParams with no thinking must not emit a thinking config.
func TestToParamsNoThinking(t *testing.T) {
	req := provider.CompleteRequest{Model: "claude-x", MaxTokens: 100,
		Messages: []provider.Message{{Role: provider.RoleUser,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "hi"}}}}}
	params, _ := toParams(req)
	raw, _ := json.Marshal(params)
	if strings.Contains(string(raw), `"thinking"`) {
		t.Errorf("unexpected thinking config: %s", raw)
	}
}

func TestToToolsSchema(t *testing.T) {
	req := provider.CompleteRequest{
		Model: "claude-x", MaxTokens: 100,
		Tools: []provider.ToolDef{{
			Name: "read_file", Description: "read a file",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		}},
		Messages: []provider.Message{{Role: provider.RoleUser,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "go"}}}},
	}
	params, err := toParams(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(params)
	js := string(raw)
	for _, want := range []string{`"read_file"`, `"read a file"`, `"path"`, `"required":["path"]`} {
		if !strings.Contains(js, want) {
			t.Errorf("tool schema missing %s: %s", want, js)
		}
	}
}

// The thinking signature round-trips: an assistant tool_call carrying the
// captured thinking block replays a thinking block (with signature) BEFORE
// the tool_use block.
func TestThinkingRoundTrip(t *testing.T) {
	thinking, _ := json.Marshal(map[string]string{"thinking": "let me reason", "signature": "sig-xyz=="})
	req := provider.CompleteRequest{
		Model: "claude-x", MaxTokens: 100,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Parts: []provider.Part{{Kind: provider.PartText, Text: "go"}}},
			{Role: provider.RoleAssistant, Parts: []provider.Part{
				{Kind: provider.PartToolCall, CallID: "toolu_1", ToolName: "bash",
					Args:   json.RawMessage(`{"command":"ls"}`),
					Extras: map[string]json.RawMessage{extrasThinkingKey: thinking}},
			}},
			{Role: provider.RoleTool, Parts: []provider.Part{
				{Kind: provider.PartToolResult, CallID: "toolu_1", Result: json.RawMessage(`{"stdout":"a"}`)}}},
		},
	}
	params, err := toParams(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(params)
	js := string(raw)
	if !strings.Contains(js, `"sig-xyz=="`) {
		t.Errorf("thinking signature not replayed: %s", js)
	}
	// Thinking must precede the tool_use in the assistant message.
	if ti, ui := strings.Index(js, `"thinking"`), strings.Index(js, `"tool_use"`); ti < 0 || ui < 0 || ti > ui {
		t.Errorf("thinking block must precede tool_use: thinking@%d tool_use@%d", ti, ui)
	}
	// The tool result appears in a user-role message.
	if !strings.Contains(js, `"tool_result"`) {
		t.Errorf("tool result missing: %s", js)
	}
}

// emitAccumulated maps a completed message's tool_use (preceded by a thinking
// block) into a normalized ToolCall carrying the thinking signature in Extras.
func TestEmitAccumulatedToolCallCarriesThinking(t *testing.T) {
	acc := sdk.Message{
		Content: []sdk.ContentBlockUnion{
			{Type: "thinking", Thinking: "reasoning", Signature: "abc=="},
			{Type: "tool_use", ID: "toolu_9", Name: "bash", Input: json.RawMessage(`{"command":"ls"}`)},
		},
		Usage:      sdk.Usage{InputTokens: 12, OutputTokens: 5, CacheReadInputTokens: 4, CacheCreationInputTokens: 2},
		StopReason: sdk.StopReasonToolUse,
	}
	var got []provider.StreamEvent
	emitAccumulated(acc, func(e provider.StreamEvent, err error) bool {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, e)
		return true
	})

	var toolCall *provider.ToolCall
	var usage *provider.Usage
	for _, e := range got {
		switch e.Kind {
		case provider.EventToolCall:
			toolCall = e.ToolCall
		case provider.EventUsage:
			usage = e.Usage
		}
	}
	if toolCall == nil {
		t.Fatal("no tool call emitted")
	}
	if toolCall.CallID != "toolu_9" || toolCall.Name != "bash" {
		t.Errorf("tool call = %+v", toolCall)
	}
	extra, ok := toolCall.Extras[extrasThinkingKey]
	if !ok || !strings.Contains(string(extra), "abc==") {
		t.Errorf("thinking signature not attached: %v", toolCall.Extras)
	}
	// InputTokens is normalized to the TOTAL input including the cached
	// prefix: 12 (uncached) + 4 (cache read) + 2 (cache creation) = 18.
	if usage == nil || usage.InputTokens != 18 || usage.CacheReadTokens != 4 || usage.CacheWriteTokens != 2 {
		t.Errorf("usage = %+v, want input 18 / cache_read 4 / cache_write 2", usage)
	}
	// Billed = input + output − cache_read = 18 + 5 − 4 = 19 (cache read
	// discounted, cache creation charged).
	if got := usage.Billed(); got != 19 {
		t.Errorf("billed = %d, want 19", got)
	}
}

func TestToMessagesRejectsBadPart(t *testing.T) {
	_, err := toMessages([]provider.Message{{Role: provider.RoleUser,
		Parts: []provider.Part{{Kind: provider.PartToolResult}}}})
	if err == nil {
		t.Fatal("expected error for tool_result in a user message")
	}
}

// Complete surfaces request-conversion errors through the stream.
func TestCompleteYieldsConversionError(t *testing.T) {
	p := &Provider{}
	req := provider.CompleteRequest{Messages: []provider.Message{{Role: "bogus",
		Parts: []provider.Part{{Kind: provider.PartText, Text: "x"}}}}}
	var gotErr error
	for _, err := range p.Complete(context.Background(), req) {
		if err != nil {
			gotErr = err
			break
		}
	}
	if gotErr == nil {
		t.Fatal("expected a conversion error before any network call")
	}
}
