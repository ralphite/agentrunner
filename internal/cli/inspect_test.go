package cli

import (
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
)

func mkEnv(t *testing.T, typ string, payload any) event.Envelope {
	t.Helper()
	env, err := event.New(typ, payload)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

// The inspect report groups activities by turn, attaches each call's verdict
// and deciding gate, and totals token/cache usage from the fold.
func TestBuildInspectReport(t *testing.T) {
	events := []event.Envelope{
		mkEnv(t, event.TypeRunStarted, &event.RunStarted{
			SpecName: "demo", Model: "gemini-x", SubStateVersions: state.SubStateVersions()}),
		mkEnv(t, event.TypeInputReceived, &event.InputReceived{Text: "go", Source: "cli"}),
		mkEnv(t, event.TypeTurnStarted, &event.TurnStarted{Turn: 1}),
		// LLM call resolved allow, with usage.
		mkEnv(t, event.TypeEffectResolved, &event.EffectResolved{
			EffectID: "eff-llm-t1", Verdict: event.VerdictAllow,
			GateResults: []event.GateResult{{Gate: "budget", Decision: event.VerdictAllow}}}),
		mkEnv(t, event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: "llm-t1", Kind: event.KindLLM, Name: "complete", Attempt: 1}),
		mkEnv(t, event.TypeActivityCompleted, &event.ActivityCompleted{
			ActivityID: "llm-t1", Usage: &provider.Usage{InputTokens: 500, OutputTokens: 40, CacheReadTokens: 100}}),
		// A denied tool call.
		mkEnv(t, event.TypeEffectResolved, &event.EffectResolved{
			EffectID: "eff-tool-c1", CallID: "c1", Verdict: event.VerdictDeny,
			GateResults: []event.GateResult{{Gate: "permission", Decision: event.VerdictDeny, Reason: "escapes workspace"}}}),
		mkEnv(t, event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: "tool-c1", Kind: event.KindTool, Name: "read_file", CallID: "c1", Attempt: 1}),
		mkEnv(t, event.TypeActivityCompleted, &event.ActivityCompleted{
			ActivityID: "tool-c1", IsError: true}),
		mkEnv(t, event.TypeRunEnded, &event.RunEnded{Reason: "completed", Turns: 1}),
	}
	s, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	r := buildInspectReport(events, s)

	if r.Spec != "demo" || r.Model != "gemini-x" || r.Status != state.StatusEnded {
		t.Fatalf("meta = %+v", r)
	}
	if len(r.Entries) != 2 {
		t.Fatalf("entries = %d, want 2: %+v", len(r.Entries), r.Entries)
	}
	llm := r.Entries[0]
	if llm.Kind != "llm" || llm.Verdict != event.VerdictAllow || llm.InputTokens != 500 || llm.CacheRead != 100 {
		t.Errorf("llm entry = %+v", llm)
	}
	tool := r.Entries[1]
	if tool.Kind != "tool" || tool.Name != "read_file" || tool.CallID != "c1" ||
		tool.Verdict != event.VerdictDeny || !strings.Contains(tool.Gate, "permission") {
		t.Errorf("tool entry = %+v", tool)
	}
	// Usage totals + billed = input+output-cache_read.
	if r.Usage.InputTokens != 500 || r.Usage.CacheRead != 100 || r.Usage.Billed != 440 {
		t.Errorf("usage = %+v", r.Usage)
	}
}

// The human-readable render includes the timeline and usage line.
func TestRenderInspect(t *testing.T) {
	r := inspectReport{
		Spec: "demo", Model: "m", Mode: "default", Status: "ended", Reason: "completed", Turns: 1,
		Entries: []entryReport{
			{Turn: 1, Kind: "llm", Name: "complete", Verdict: "allow", InputTokens: 10, OutputTokens: 5},
			{Turn: 1, Kind: "tool", Name: "bash", CallID: "c1", Verdict: "allow", Gate: "permission"},
		},
		Usage: usageReport{InputTokens: 10, OutputTokens: 5, Billed: 15},
	}
	var sb strings.Builder
	renderInspect(&sb, r)
	out := sb.String()
	for _, want := range []string{"TIMELINE", "turn 1", "complete", "bash", "billed 15", "completed"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q:\n%s", want, out)
		}
	}
}
