package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/provider"
)

func TestToContentsRolesAndSignature(t *testing.T) {
	sig, _ := json.Marshal([]byte{0xde, 0xad})
	msgs := []provider.Message{
		{Role: provider.RoleUser, Parts: []provider.Part{{Kind: provider.PartText, Text: "fix it"}}},
		{Role: provider.RoleAssistant, Parts: []provider.Part{
			{Kind: provider.PartText, Text: "reading"},
			{Kind: provider.PartToolCall, CallID: "call_1_0", ToolName: "read_file",
				Args:   json.RawMessage(`{"path":"a.go"}`),
				Extras: map[string]json.RawMessage{extrasSignatureKey: sig}},
		}},
		{Role: provider.RoleTool, Parts: []provider.Part{
			{Kind: provider.PartToolResult, CallID: "call_1_0", ToolName: "read_file",
				Result: json.RawMessage(`{"content":"package a"}`)},
		}},
	}

	contents, err := toContents(msgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 3 {
		t.Fatalf("contents = %d, want 3", len(contents))
	}
	if contents[0].Role != genai.RoleUser || contents[1].Role != genai.RoleModel || contents[2].Role != genai.RoleUser {
		t.Errorf("roles = %s/%s/%s", contents[0].Role, contents[1].Role, contents[2].Role)
	}

	fc := contents[1].Parts[1]
	if fc.FunctionCall == nil || fc.FunctionCall.Name != "read_file" {
		t.Fatalf("function call part = %+v", fc)
	}
	if string(fc.ThoughtSignature) != "\xde\xad" {
		t.Errorf("thought signature not restored: %v", fc.ThoughtSignature)
	}

	fr := contents[2].Parts[0]
	if fr.FunctionResponse == nil || fr.FunctionResponse.Response["content"] != "package a" {
		t.Errorf("function response = %+v", fr)
	}
}

func TestToResponseMapConventions(t *testing.T) {
	cases := []struct {
		name string
		part provider.Part
		key  string
	}{
		{"object passthrough", provider.Part{Result: json.RawMessage(`{"a":1}`)}, "a"},
		{"scalar wraps as output", provider.Part{Result: json.RawMessage(`"hi"`)}, "output"},
		{"error wraps as error", provider.Part{Result: json.RawMessage(`"boom"`), IsError: true}, "error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := toResponseMap(tc.part)
			if _, ok := m[tc.key]; !ok {
				t.Errorf("response map = %v, want key %q", m, tc.key)
			}
		})
	}
}

func TestToConfigTools(t *testing.T) {
	req := provider.CompleteRequest{
		System:    "be brief",
		MaxTokens: 100,
		Tools: []provider.ToolDef{{
			Name:        "read_file",
			Description: "read a file",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		}},
	}
	config, err := toConfig(req)
	if err != nil {
		t.Fatal(err)
	}
	if config.SystemInstruction == nil || config.SystemInstruction.Parts[0].Text != "be brief" {
		t.Errorf("system instruction = %+v", config.SystemInstruction)
	}
	if config.MaxOutputTokens != 100 {
		t.Errorf("max tokens = %d", config.MaxOutputTokens)
	}
	decls := config.Tools[0].FunctionDeclarations
	if len(decls) != 1 || decls[0].Name != "read_file" || decls[0].ParametersJsonSchema == nil {
		t.Errorf("declarations = %+v", decls)
	}
}

// Flash thinks by default and thought tokens eat MaxOutputTokens; with no
// thinking requested we must turn it OFF (budget 0) so the whole cap goes to
// the answer — the root-cause fix for the empty-message session-death bug.
func TestToConfigDisablesDefaultThinking(t *testing.T) {
	flash, err := toConfig(provider.CompleteRequest{Model: "gemini-flash-latest", MaxTokens: 2048})
	if err != nil {
		t.Fatal(err)
	}
	if flash.ThinkingConfig == nil || flash.ThinkingConfig.ThinkingBudget == nil || *flash.ThinkingConfig.ThinkingBudget != 0 {
		t.Errorf("flash without thinking must force budget 0, got %+v", flash.ThinkingConfig)
	}
	// Pro cannot fully disable thinking (min budget 128) — leave its config alone.
	pro, err := toConfig(provider.CompleteRequest{Model: "gemini-2.5-pro", MaxTokens: 2048})
	if err != nil {
		t.Fatal(err)
	}
	if pro.ThinkingConfig != nil {
		t.Errorf("pro must not get a forced budget, got %+v", pro.ThinkingConfig)
	}
	// Requested thinking is honored with thought summaries and the given budget.
	on, err := toConfig(provider.CompleteRequest{Model: "gemini-flash-latest", MaxTokens: 2048,
		Thinking: provider.ThinkingConfig{Enabled: true, BudgetTokens: 500}})
	if err != nil {
		t.Fatal(err)
	}
	if on.ThinkingConfig == nil || !on.ThinkingConfig.IncludeThoughts ||
		on.ThinkingConfig.ThinkingBudget == nil || *on.ThinkingConfig.ThinkingBudget != 500 {
		t.Errorf("requested thinking must be honored, got %+v", on.ThinkingConfig)
	}
}

// resolveThinkingBudget must always cap thinking so the answer keeps room
// within maxTokens — the core of the empty-message fix.
func TestResolveThinkingBudget(t *testing.T) {
	cases := []struct {
		name       string
		maxTokens  int
		requested  int
		wantBudget int32
		wantOK     bool
	}{
		// The user's failing shape: enabled with no explicit budget must NOT be
		// unbounded — it takes the default cap, and the answer keeps ≥1/4.
		{"enabled no budget, default cap", 8192, 0, 6144, true},
		{"enabled no budget, big cap clamps to default", 40000, 0, 8192, true},
		// Explicit budget honored when it fits under the reserve ceiling.
		{"explicit fits", 10240, 6144, 6144, true},
		{"small explicit honored", 2048, 500, 500, true},
		// Over-large budget clamped so a quarter of the cap survives.
		{"budget starving answer clamped", 8192, 8000, 6144, true},
		{"budget above gemini max clamped", 60000, 100000, 24576, true},
		// Cap too small to afford any thinking → disable (ok=false).
		{"tiny cap disables thinking", 1024, 4096, 0, false},
		{"zero cap disables thinking", 0, 4096, 0, false},
		// Negative requested behaves like unspecified (never unbounded).
		{"negative request treated as default", 8192, -5, 6144, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := resolveThinkingBudget(c.maxTokens, c.requested)
			if ok != c.wantOK || got != c.wantBudget {
				t.Errorf("resolveThinkingBudget(%d,%d) = (%d,%v), want (%d,%v)",
					c.maxTokens, c.requested, got, ok, c.wantBudget, c.wantOK)
			}
			// Invariant: an allowed budget always leaves answer room.
			if ok && int(got) >= c.maxTokens {
				t.Errorf("budget %d does not reserve answer room within maxTokens %d", got, c.maxTokens)
			}
		})
	}
}

// A too-small cap with thinking enabled must disable thinking on flash (budget
// 0 → full cap to the answer) rather than send an unbounded/answer-starving
// request.
func TestToConfigEnabledTinyCapDisables(t *testing.T) {
	cfg, err := toConfig(provider.CompleteRequest{Model: "gemini-flash-latest", MaxTokens: 1024,
		Thinking: provider.ThinkingConfig{Enabled: true, BudgetTokens: 4096}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ThinkingConfig == nil || cfg.ThinkingConfig.ThinkingBudget == nil || *cfg.ThinkingConfig.ThinkingBudget != 0 {
		t.Errorf("tiny cap must disable thinking (budget 0), got %+v", cfg.ThinkingConfig)
	}
}

// Enabled with no explicit budget must send a positive, clamped budget — never
// leave ThinkingBudget nil (Gemini's unbounded dynamic default that starves the
// answer, the empty-message defect).
func TestToConfigEnabledNoBudgetIsBounded(t *testing.T) {
	cfg, err := toConfig(provider.CompleteRequest{Model: "gemini-flash-latest", MaxTokens: 8192,
		Thinking: provider.ThinkingConfig{Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ThinkingConfig == nil || cfg.ThinkingConfig.ThinkingBudget == nil {
		t.Fatalf("enabled thinking must carry an explicit budget, got %+v", cfg.ThinkingConfig)
	}
	if b := *cfg.ThinkingConfig.ThinkingBudget; b <= 0 || int(b) >= 8192 {
		t.Errorf("budget %d must be positive and reserve answer room within 8192", b)
	}
}

func TestStreamStateMapping(t *testing.T) {
	st := newStreamState(2)
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			FinishReason: genai.FinishReasonStop,
			Content: &genai.Content{Parts: []*genai.Part{
				{Text: "on it"},
				{FunctionCall: &genai.FunctionCall{Name: "bash", Args: map[string]any{"command": "ls"}}},
				{FunctionCall: &genai.FunctionCall{Name: "read_file", Args: map[string]any{"path": "x"}}},
			}},
		}},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount: 5, CandidatesTokenCount: 7, CachedContentTokenCount: 3,
		},
	}

	events := st.mapResponse(resp)
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4 (text + 2 calls + usage)", len(events))
	}
	if events[1].ToolCall.CallID != "call_2_0" || events[2].ToolCall.CallID != "call_2_1" {
		t.Errorf("call ids = %q, %q", events[1].ToolCall.CallID, events[2].ToolCall.CallID)
	}
	if events[3].Usage.CacheReadTokens != 3 {
		t.Errorf("usage = %+v", events[3].Usage)
	}
	if got := st.finish(); got != provider.FinishToolUse {
		t.Errorf("finish = %q, want tool_use", got)
	}
}

func TestFinishMapping(t *testing.T) {
	cases := []struct {
		reason  genai.FinishReason
		sawCall bool
		want    provider.FinishReason
	}{
		{genai.FinishReasonStop, false, provider.FinishEndTurn},
		{genai.FinishReasonStop, true, provider.FinishToolUse},
		{genai.FinishReasonMaxTokens, false, provider.FinishMaxTokens},
		{genai.FinishReasonSafety, false, provider.FinishOther},
	}
	for _, tc := range cases {
		st := &streamState{sawToolCall: tc.sawCall, finishReason: tc.reason}
		if got := st.finish(); got != tc.want {
			t.Errorf("finish(%s, saw=%v) = %q, want %q", tc.reason, tc.sawCall, got, tc.want)
		}
	}
}

func TestToContentErrors(t *testing.T) {
	cases := []struct {
		name string
		msg  provider.Message
	}{
		{"no parts", provider.Message{Role: provider.RoleUser}},
		{"unknown role", provider.Message{Role: "narrator",
			Parts: []provider.Part{{Kind: provider.PartText, Text: "x"}}}},
		{"unknown part kind", provider.Message{Role: provider.RoleUser,
			Parts: []provider.Part{{Kind: "hologram"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := toContent(tc.msg); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

// Conversion failures must surface through the Complete iterator (the
// client is never touched when conversion fails, so a nil client is safe).
func TestCompleteYieldsConversionError(t *testing.T) {
	p := &Provider{client: nil}
	_, err := provider.CollectTurn(p.Complete(context.Background(), provider.CompleteRequest{
		Model:    "m",
		Messages: []provider.Message{{Role: "narrator"}},
	}))
	if err == nil {
		t.Fatal("expected conversion error from stream")
	}
}

// Empty tool results pin the {"output": nil} convention.
func TestToResponseMapEmptyResult(t *testing.T) {
	m := toResponseMap(provider.Part{})
	if _, ok := m["output"]; !ok {
		t.Errorf("empty result map = %v, want output key", m)
	}
}

// 2.8: SDK errors map onto the taxonomy — retry (2.10) and rendering (3.9)
// consume only the class.
func TestClassifyTable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want errs.Class
	}{
		{"429", genai.APIError{Code: 429}, errs.ProviderRateLimit},
		{"503", genai.APIError{Code: 503}, errs.ProviderServer},
		{"401", genai.APIError{Code: 401}, errs.ProviderAuth},
		{"400", genai.APIError{Code: 400}, errs.ProviderInvalid},
		{"404 retired model", genai.APIError{Code: 404}, errs.ProviderInvalid},
		{"wrapped api error", fmt.Errorf("stream: %w", genai.APIError{Code: 429}), errs.ProviderRateLimit},
		{"context canceled", context.Canceled, errs.Canceled},
		{"deadline", context.DeadlineExceeded, errs.Timeout},
		{"transport", errors.New("connection reset"), errs.ProviderServer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := errs.ClassOf(classify(tc.err)); got != tc.want {
				t.Errorf("classify → %s, want %s", got, tc.want)
			}
		})
	}
}

// A 404 (retired or misspelled model id) keeps its class but gains an
// actionable hint pointing at a current model alias (黑盒 R2 minor).
func TestClassify404Hint(t *testing.T) {
	err := classify(genai.APIError{Code: 404, Message: "models/gemini-2.5-flash is not found"})
	if got := errs.ClassOf(err); got != errs.ProviderInvalid {
		t.Fatalf("class = %s, want provider_invalid", got)
	}
	if !strings.Contains(err.Error(), "gemini-flash-latest") {
		t.Errorf("404 error lacks the current-model hint: %v", err)
	}
}

// v2 M4.2: an inflated image part maps to inline_data; a ref-only part
// (inflate skipped) is an error, never a silent empty blob.
func TestToPartImage(t *testing.T) {
	part, err := toPart(provider.Part{Kind: provider.PartImage,
		MediaType: "image/png", Ref: "sha256-x", Data: []byte{1, 2, 3}})
	if err != nil {
		t.Fatal(err)
	}
	if part.InlineData == nil || part.InlineData.MIMEType != "image/png" ||
		len(part.InlineData.Data) != 3 {
		t.Fatalf("inline_data = %+v", part.InlineData)
	}
	if _, err := toPart(provider.Part{Kind: provider.PartImage,
		MediaType: "image/png", Ref: "sha256-x"}); err == nil {
		t.Error("ref-only (uninflated) image part mapped without error")
	}
}

// INC-9: an inflated PDF file part rides inline_data with its own MIME —
// Gemini accepts application/pdf inline, so no special-casing beyond the
// existing generic media path.
func TestToPartFilePDF(t *testing.T) {
	part, err := toPart(provider.Part{Kind: provider.PartFile,
		MediaType: "application/pdf", Ref: "sha256-x", Data: []byte("%PDF-1.7")})
	if err != nil {
		t.Fatal(err)
	}
	if part.InlineData == nil || part.InlineData.MIMEType != "application/pdf" {
		t.Fatalf("inline_data = %+v, want application/pdf", part.InlineData)
	}
}

// INC-56 (ar dictate): an audio part rides the same inline_data path as any
// binary media — its bytes and MIME reach Gemini untouched. The dictate helper
// sets Data directly (no CAS inflate), so a data-bearing audio part encodes,
// while a byte-less one is a hard error like every other media kind.
func TestToPartAudio(t *testing.T) {
	part, err := toPart(provider.Part{Kind: provider.PartAudio,
		MediaType: "audio/wav", Data: []byte("RIFF....WAVE")})
	if err != nil {
		t.Fatal(err)
	}
	if part.InlineData == nil || part.InlineData.MIMEType != "audio/wav" ||
		string(part.InlineData.Data) != "RIFF....WAVE" {
		t.Fatalf("inline_data = %+v, want audio/wav bytes verbatim", part.InlineData)
	}
	// A byte-less audio part (nothing to transcribe) must error, not send an
	// empty blob the API would 400 on.
	if _, err := toPart(provider.Part{Kind: provider.PartAudio, MediaType: "audio/wav"}); err == nil {
		t.Error("byte-less audio part mapped without error")
	}
}
