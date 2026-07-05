package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
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
		mkEnv(t, event.TypeSessionStarted, &event.SessionStarted{
			SpecName: "demo", Model: "gemini-x", SubStateVersions: state.SubStateVersions()}),
		mkEnv(t, event.TypeInputReceived, &event.InputReceived{Text: "go", Source: "cli"}),
		mkEnv(t, event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}),
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
		mkEnv(t, event.TypeRunEnded, &event.RunEnded{Reason: "completed", GenSteps: 1}),
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

// S5.9: the tree report recurses into child journals under sub/, and the
// artifacts section lists published versions.
func TestBuildInspectTree(t *testing.T) {
	dir := t.TempDir()
	write := func(sub string, evs []event.Envelope) {
		t.Helper()
		d := dir
		if sub != "" {
			d = filepath.Join(dir, "sub", sub)
		}
		es, err := store.OpenEventStore(d)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = es.Close() }()
		for _, e := range evs {
			if _, err := es.Append(e); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Parent journal: a spawn (SubagentCompleted names the child session)
	// and an artifact.
	write("", []event.Envelope{
		mkEnv(t, event.TypeSessionStarted, &event.SessionStarted{SpecName: "lead",
			SubStateVersions: state.SubStateVersions()}),
		mkEnv(t, event.TypeArtifactPublished, &event.ArtifactPublished{
			Stream: "report", Version: 1, Ref: "sha256-abc", Source: "tool"}),
		mkEnv(t, event.TypeSubagentCompleted, &event.SubagentCompleted{
			CallID: "s1", Agent: "researcher", ChildSession: "lead-sub-s1-a1",
			Reason: "completed", GenSteps: 2}),
		mkEnv(t, event.TypeRunEnded, &event.RunEnded{Reason: "completed", GenSteps: 3}),
	})
	// Child journal under sub/s1-a1.
	write("s1-a1", []event.Envelope{
		mkEnv(t, event.TypeSessionStarted, &event.SessionStarted{SpecName: "researcher",
			SubStateVersions: state.SubStateVersions()}),
		mkEnv(t, event.TypeRunEnded, &event.RunEnded{Reason: "completed", GenSteps: 2}),
	})

	report, err := buildInspectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Spec != "lead" || len(report.Children) != 1 {
		t.Fatalf("report = spec %q, children %d", report.Spec, len(report.Children))
	}
	child := report.Children[0]
	if child.Agent != "researcher" || child.Report.Spec != "researcher" ||
		child.Report.Status != state.StatusEnded {
		t.Errorf("child = %+v", child)
	}
	if len(report.Artifacts) != 1 || report.Artifacts[0].Stream != "report" {
		t.Errorf("artifacts = %+v", report.Artifacts)
	}

	// The render shows the nested tree and the artifact line.
	var sb strings.Builder
	renderInspect(&sb, report)
	out := sb.String()
	for _, want := range []string{"CHILD   s1 → researcher", "researcher", "report@v1", "sha256-abc"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q:\n%s", want, out)
		}
	}
}

// The human-readable render includes the timeline and usage line.
func TestRenderInspect(t *testing.T) {
	r := inspectReport{
		Spec: "demo", Model: "m", Mode: "default", Status: "ended", Reason: "completed", GenSteps: 1,
		Entries: []entryReport{
			{GenStep: 1, Kind: "llm", Name: "complete", Verdict: "allow", InputTokens: 10, OutputTokens: 5},
			{GenStep: 1, Kind: "tool", Name: "bash", CallID: "c1", Verdict: "allow", Gate: "permission"},
		},
		Usage: usageReport{InputTokens: 10, OutputTokens: 5, Billed: 15},
	}
	var sb strings.Builder
	renderInspect(&sb, r)
	out := sb.String()
	for _, want := range []string{"TIMELINE", "gen-step 1", "complete", "bash", "billed 15", "completed"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q:\n%s", want, out)
		}
	}
}
