//go:build live

package gemini

import (
	"context"
	"os"
	"testing"

	"google.golang.org/genai"

	"github.com/ralphite/agentrunner/internal/provider"
)

// A reasoning-inducing prompt: gemini-flash-latest spends many thought tokens
// on it, so with a small MaxOutputTokens cap an UNCLAMPED thinking budget
// starves the answer to an empty message — the 会话死亡 defect. The fix caps
// the thinking budget so the answer always keeps room.
const starvationPrompt = "Think carefully step by step, then answer. " +
	"A snail climbs a 12-meter well. Each day it climbs 3 meters and each " +
	"night it slides back 2 meters. On which day does it reach the top? " +
	"Explain your full reasoning, then give the final numeric answer."

// rawResult characterizes what one raw generate produced.
type rawResult struct {
	answerToks  int
	thoughtToks int
	hasAnswer   bool // any non-thought text or tool call reached the caller
	finish      genai.FinishReason
}

// rawGenerate calls the API directly with a hand-built config so a test can
// probe model behavior the provider's toConfig would otherwise normalize away.
func rawGenerate(t *testing.T, client *genai.Client, model string, maxTokens int32, tc *genai.ThinkingConfig, prompt string) rawResult {
	t.Helper()
	cfg := &genai.GenerateContentConfig{MaxOutputTokens: maxTokens, ThinkingConfig: tc}
	contents := []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: prompt}}}}
	var res rawResult
	for resp, err := range client.Models.GenerateContentStream(context.Background(), model, contents, cfg) {
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		if len(resp.Candidates) > 0 {
			c := resp.Candidates[0]
			if c.FinishReason != "" {
				res.finish = c.FinishReason
			}
			if c.Content != nil {
				for _, p := range c.Content.Parts {
					switch {
					case p.Thought:
						// thought summary — not an answer part
					case p.Text != "" || p.FunctionCall != nil:
						res.hasAnswer = true
					}
				}
			}
		}
		if resp.UsageMetadata != nil {
			res.answerToks = int(resp.UsageMetadata.CandidatesTokenCount)
			res.thoughtToks = int(resp.UsageMetadata.ThoughtsTokenCount)
		}
	}
	return res
}

func budgetPtr(v int32) *int32 { return &v }

// rawGenerateTool is rawGenerate with a single function tool offered, forcing a
// tool-call turn. A tool call is ATOMIC — it cannot be partially emitted — so
// when thinking crowds it out of the cap the completion arrives with NO parts:
// the literal empty message that wedges a session.
func rawGenerateTool(t *testing.T, client *genai.Client, model string, maxTokens int32, tc *genai.ThinkingConfig, prompt string) rawResult {
	t.Helper()
	tool := &genai.Tool{FunctionDeclarations: []*genai.FunctionDeclaration{{
		Name:        "list_dir",
		Description: "List the files in the current directory.",
		ParametersJsonSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"path": map[string]any{"type": "string"}},
		},
	}}}
	cfg := &genai.GenerateContentConfig{MaxOutputTokens: maxTokens, ThinkingConfig: tc, Tools: []*genai.Tool{tool}}
	contents := []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: prompt}}}}
	var res rawResult
	for resp, err := range client.Models.GenerateContentStream(context.Background(), model, contents, cfg) {
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		if len(resp.Candidates) > 0 {
			c := resp.Candidates[0]
			if c.FinishReason != "" {
				res.finish = c.FinishReason
			}
			if c.Content != nil {
				for _, p := range c.Content.Parts {
					switch {
					case p.Thought:
					case p.Text != "" || p.FunctionCall != nil:
						res.hasAnswer = true
					}
				}
			}
		}
		if resp.UsageMetadata != nil {
			res.answerToks = int(resp.UsageMetadata.CandidatesTokenCount)
			res.thoughtToks = int(resp.UsageMetadata.ThoughtsTokenCount)
		}
	}
	return res
}

// TestLiveThinkingStarvation is the real-API regression guard for the empty-
// message defect: with a small cap and a reasoning-heavy prompt, an unclamped
// thinking budget starves the answer, while the clamped budget the provider now
// sends always leaves room for a real answer.
func TestLiveThinkingStarvation(t *testing.T) {
	loadDotEnv(t)
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set; live thinking test deferred to checkpoint")
	}
	ctx := context.Background()
	p, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const model = "gemini-flash-latest"
	// A deliberately small cap: gemini-flash-latest spends several hundred
	// thought tokens on the prompt, so an uncapped budget overruns this before
	// any answer — the empty-message defect. resolveThinkingBudget then disables
	// thinking (ceiling < 0) so the whole cap goes to the answer.
	const cap = 256

	// (1) BEFORE — the pre-fix Enabled path honored budget_tokens verbatim with
	// no clamp against the cap. A budget larger than MaxOutputTokens lets the
	// model spend the WHOLE cap on thoughts and never reach the answer — the
	// empty-message defect, reproduced deterministically.
	overCap := int32(cap * 4)
	before := rawGenerate(t, p.client, model, cap,
		&genai.ThinkingConfig{IncludeThoughts: true, ThinkingBudget: &overCap}, starvationPrompt)
	t.Logf("BEFORE budget>cap (%d>%d): hasAnswer=%v answerToks=%d thoughtToks=%d finish=%s",
		overCap, cap, before.hasAnswer, before.answerToks, before.thoughtToks, before.finish)
	// Thinking consuming the bulk of the cap is the defect: a tiny answer
	// remainder cannot hold an atomic tool call, so a tool-call turn arrives
	// with no parts — the empty message. Documented (model behavior varies).
	if before.finish == genai.FinishReasonMaxTokens && before.thoughtToks > before.answerToks {
		t.Logf("REPRODUCED: thinking (%d tok) crowded the answer (%d tok) out of the %d cap — a tool call would not fit",
			before.thoughtToks, before.answerToks, cap)
	}

	// (2) The old "budget 0 ⇒ thinking off" probe was RETIRED on 2026-07-21:
	// gemini-flash-latest now REJECTS thinkingBudget:0 with INVALID_ARGUMENT (the
	// "latest" alias moved to a think-by-default model). The provider no longer
	// sends 0 anywhere — an unrequested-thinking spec gets a positive clamped
	// budget instead (toConfig / TestToConfigDisablesDefaultThinking). A raw
	// budget:0 call here would now 400, so the probe is gone.

	// (3) AFTER — the request the FIXED provider builds for an enabled thinking
	// spec with the same small cap AND an over-large requested budget. toConfig
	// runs resolveThinkingBudget, which clamps the budget to reserve answer
	// room. Assert a real answer always comes back.
	req := provider.CompleteRequest{
		Model:     model,
		MaxTokens: cap,
		Messages: []provider.Message{{
			Role:  provider.RoleUser,
			Parts: []provider.Part{{Kind: provider.PartText, Text: starvationPrompt}},
		}},
		Thinking: provider.ThinkingConfig{Enabled: true, BudgetTokens: cap * 4},
		GenStep:  1,
	}
	turn, err := provider.CollectTurn(p.Complete(ctx, req))
	if err != nil {
		t.Fatalf("fixed enabled-thinking turn errored: %v", err)
	}
	var gotText, gotTool bool
	for _, part := range turn.Message.Parts {
		switch part.Kind {
		case provider.PartText:
			if part.Text != "" {
				gotText = true
			}
		case provider.PartToolCall:
			gotTool = true
		}
	}
	t.Logf("AFTER clamped: parts=%d gotText=%v gotTool=%v finish=%s in=%d out=%d",
		len(turn.Message.Parts), gotText, gotTool, turn.Finish,
		turn.Usage.InputTokens, turn.Usage.OutputTokens)
	if !gotText && !gotTool {
		t.Fatalf("FIX FAILED: clamped enabled-thinking still produced an empty message (finish=%s)", turn.Finish)
	}

	// (4) TOOL-CALL turn under the same tight cap. A tool call is ATOMIC — it
	// cannot be partially emitted — so if thoughts crowd it out the completion
	// arrives with NO parts (the empty message). In practice the model often
	// adapts, shortening its thoughts to leave room for the call; the fix does
	// not RELY on that adaptivity — it structurally reserves answer room so the
	// call space never depends on the model's mood. hasAnswer=false here would
	// be the literal empty message; either way the fix removes the vector.
	toolPrompt := "First reason briefly about what to do, then call the list_dir tool for the current directory."
	beforeTool := rawGenerateTool(t, p.client, model, cap,
		&genai.ThinkingConfig{IncludeThoughts: true, ThinkingBudget: &overCap}, toolPrompt)
	t.Logf("BEFORE tool-call unclamped: hasAnswer=%v thoughtToks=%d finish=%s (hasAnswer=false ⇒ literal empty message)",
		beforeTool.hasAnswer, beforeTool.thoughtToks, beforeTool.finish)

	// The fix (thinking disabled at this cap) leaves the whole cap for the call.
	afterTool := rawGenerateTool(t, p.client, model, cap,
		&genai.ThinkingConfig{ThinkingBudget: budgetPtr(0)}, toolPrompt)
	t.Logf("AFTER tool-call (thinking off): hasAnswer=%v thoughtToks=%d finish=%s",
		afterTool.hasAnswer, afterTool.thoughtToks, afterTool.finish)
	if !afterTool.hasAnswer {
		t.Fatalf("FIX FAILED: tool-call turn empty even with thinking disabled (finish=%s)", afterTool.finish)
	}
}
