//go:build live

package anthropic

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
)

// loadDotEnv populates missing env vars from the repo-root .env (local
// convenience per PLAN §0; never overrides existing values).
func loadDotEnv(t *testing.T) {
	f, err := os.Open("../../../.env")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && os.Getenv(k) == "" {
			t.Setenv(k, v)
		}
	}
}

func TestLiveSmoke(t *testing.T) {
	loadDotEnv(t)
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set; live smoke deferred to stage-exit checkpoint")
	}

	ctx := context.Background()
	p, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}

	turn, err := provider.CollectTurn(p.Complete(ctx, provider.CompleteRequest{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 200,
		System:    "You are terse.",
		Messages: []provider.Message{{
			Role:  provider.RoleUser,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "Say the single word: pong"}},
		}},
		Turn: 1,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.Message.Parts) == 0 || !strings.Contains(strings.ToLower(turn.Message.Parts[0].Text), "pong") {
		t.Errorf("unexpected reply: %+v", turn.Message)
	}
	if turn.Usage.OutputTokens == 0 {
		t.Errorf("usage not populated: %+v", turn.Usage)
	}
}

// TestLiveToolAndThinkingRoundTrip exercises a two-turn tool use under
// extended thinking — the path where the thinking signature must round-trip.
func TestLiveToolAndThinkingRoundTrip(t *testing.T) {
	loadDotEnv(t)
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	ctx := context.Background()
	p, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}

	tools := []provider.ToolDef{{
		Name:        "get_weather",
		Description: "Get the weather for a city",
		InputSchema: []byte(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
	}}
	first, err := provider.CollectTurn(p.Complete(ctx, provider.CompleteRequest{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 2048,
		Thinking:  provider.ThinkingConfig{Enabled: true, BudgetTokens: 1024},
		Tools:     tools,
		Messages: []provider.Message{{Role: provider.RoleUser,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "What's the weather in Paris? Use the tool."}}}},
		Turn: 1,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(first.ToolCalls) == 0 {
		t.Skipf("model did not call the tool: %+v", first.Message)
	}

	// Feed the tool result back — the assistant message (carrying the thinking
	// block in Extras) must replay cleanly.
	call := first.ToolCalls[0]
	second, err := provider.CollectTurn(p.Complete(ctx, provider.CompleteRequest{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 2048,
		Thinking:  provider.ThinkingConfig{Enabled: true, BudgetTokens: 1024},
		Tools:     tools,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Parts: []provider.Part{{Kind: provider.PartText, Text: "What's the weather in Paris? Use the tool."}}},
			first.Message,
			{Role: provider.RoleTool, Parts: []provider.Part{{Kind: provider.PartToolResult,
				CallID: call.CallID, Result: []byte(`{"temp_c":18,"sky":"clear"}`)}}},
		},
		Turn: 2,
	}))
	if err != nil {
		t.Fatalf("thinking round-trip failed: %v", err)
	}
	if txt := strings.ToLower(providerText(second.Message)); !strings.Contains(txt, "18") && !strings.Contains(txt, "paris") {
		t.Errorf("unexpected follow-up: %+v", second.Message)
	}
}

func providerText(m provider.Message) string {
	var b strings.Builder
	for _, p := range m.Parts {
		if p.Kind == provider.PartText {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}
