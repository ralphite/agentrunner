package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/driver"
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
		// A denied tool call: the assistant issues it, adjudication denies it,
		// and it NEVER becomes an activity (deny precedes execution). Its name
		// comes from the assistant message; the timeline must still surface it.
		mkEnv(t, event.TypeAssistantMessage, &event.AssistantMessage{
			GenStep: 1, Message: provider.Message{Role: provider.RoleAssistant, Parts: []provider.Part{
				{Kind: provider.PartToolCall, CallID: "c1", ToolName: "read_file"}}}}),
		mkEnv(t, event.TypeEffectResolved, &event.EffectResolved{
			EffectID: "eff-tool-c1", CallID: "c1", Verdict: event.VerdictDeny,
			GateResults: []event.GateResult{{Gate: "permission", Decision: event.VerdictDeny, Reason: "escapes workspace"}}}),
		mkEnv(t, event.TypeSessionClosed, &event.SessionClosed{Reason: "closed", Source: "user", GenSteps: 1}),
	}
	s, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	r := buildInspectReport(events, s)

	if r.Spec != "demo" || r.Model != "gemini-x" || r.Status != "marked" {
		t.Fatalf("meta = %+v", r)
	}
	if r.Turns != 1 || r.Items != 2 {
		t.Fatalf("turn/item counts = %d/%d", r.Turns, r.Items)
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

func TestInspectReportSurfacesFailedAndStopped(t *testing.T) {
	failEvents := []event.Envelope{
		mkEnv(t, event.TypeSessionStarted, &event.SessionStarted{SpecName: "demo", SubStateVersions: state.SubStateVersions()}),
		mkEnv(t, event.TypeInputReceived, &event.InputReceived{Text: "go", Source: "cli"}),
		mkEnv(t, event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}),
		mkEnv(t, event.TypeActivityStarted, &event.ActivityStarted{ActivityID: "llm-t1", Kind: event.KindLLM, Name: "complete", Attempt: 1}),
		mkEnv(t, event.TypeActivityFailed, &event.ActivityFailed{ActivityID: "llm-t1", Attempt: 1, Final: true,
			Error: event.ErrorInfo{Class: "provider_invalid", Message: "bad model"}}),
	}
	s, err := state.Fold(failEvents)
	if err != nil {
		t.Fatal(err)
	}
	r := buildInspectReport(failEvents, s)
	if r.Status != "failed" || !strings.Contains(r.Reason, "provider_invalid") {
		t.Fatalf("failed report = %+v", r)
	}

	stopEvents := []event.Envelope{
		mkEnv(t, event.TypeSessionStarted, &event.SessionStarted{SpecName: "demo", SubStateVersions: state.SubStateVersions()}),
		mkEnv(t, event.TypeSessionClosed, &event.SessionClosed{Reason: "stopped", Source: "user"}),
	}
	s, err = state.Fold(stopEvents)
	if err != nil {
		t.Fatal(err)
	}
	r = buildInspectReport(stopEvents, s)
	if r.Status != "stopped" {
		t.Fatalf("stopped report = %+v", r)
	}
}

// TestInspectIdleConversationMatchesSessions pins that an idle post-turn
// conversational session (quiescent "completed" but parked on a durable input
// wait) reports "waiting:input" from inspect — the same label `ar sessions`
// and the web list already use. inspect used to render "quiescent (completed)"
// while the list said "waiting:input", so the two primary observability
// surfaces disagreed on the identical state (QA Wave6 mia-04).
func TestInspectIdleConversationMatchesSessions(t *testing.T) {
	events := []event.Envelope{
		mkEnv(t, event.TypeSessionStarted, &event.SessionStarted{SpecName: "demo", SubStateVersions: state.SubStateVersions()}),
		mkEnv(t, event.TypeInputReceived, &event.InputReceived{Text: "hi", Source: "cli"}),
		mkEnv(t, event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}),
		mkEnv(t, event.TypeAssistantMessage, &event.AssistantMessage{
			GenStep: 1, Message: provider.Message{Role: provider.RoleAssistant, Parts: []provider.Part{
				{Kind: provider.PartText, Text: "hello"}}}}),
		mkEnv(t, event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitInput}),
	}
	s, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	// Precondition: the fold IS quiescent "completed" — the divergence is purely
	// in how each surface labels that same shape.
	if q, reason := state.Quiescence(s); !q || reason != "completed" {
		t.Fatalf("precondition: quiescent=%v reason=%q, want true/completed", q, reason)
	}
	r := buildInspectReport(events, s)
	if r.Status != "waiting:input" {
		t.Fatalf("idle conversation status = %q (reason %q), want waiting:input to match ar sessions", r.Status, r.Reason)
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
		mkEnv(t, event.TypeSessionClosed, &event.SessionClosed{Reason: "closed", Source: "user", GenSteps: 3}),
	})
	// Child journal under sub/s1-a1.
	write("s1-a1", []event.Envelope{
		mkEnv(t, event.TypeSessionStarted, &event.SessionStarted{SpecName: "researcher",
			SubStateVersions: state.SubStateVersions()}),
		mkEnv(t, event.TypeSessionClosed, &event.SessionClosed{Reason: "closed", Source: "user", GenSteps: 2}),
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
		child.Report.Status != "marked" {
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

// TestInspectChildrenDedupedAcrossRevive nails G26: a revived child journals
// one SubagentCompleted per settlement; inspect must show that child ONCE,
// with the latest settlement's status (same contract as webui).
func TestInspectChildrenDedupedAcrossRevive(t *testing.T) {
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
	write("", []event.Envelope{
		mkEnv(t, event.TypeSessionStarted, &event.SessionStarted{SpecName: "lead",
			SubStateVersions: state.SubStateVersions()}),
		mkEnv(t, event.TypeSubagentCompleted, &event.SubagentCompleted{
			CallID: "s1", Agent: "worker", ChildSession: "lead-sub-s1-a1",
			Reason: "killed", GenSteps: 1}),
		mkEnv(t, event.TypeSubagentCompleted, &event.SubagentCompleted{
			CallID: "s1", Agent: "worker", ChildSession: "lead-sub-s1-a1",
			Reason: "completed", GenSteps: 3}),
	})
	write("s1-a1", []event.Envelope{
		mkEnv(t, event.TypeSessionStarted, &event.SessionStarted{SpecName: "worker",
			SubStateVersions: state.SubStateVersions()}),
	})

	report, err := buildInspectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Children) != 1 {
		t.Fatalf("children = %d, want 1 after dedupe: %+v", len(report.Children), report.Children)
	}
	if got := report.Children[0].Reason; got != "completed" {
		t.Errorf("reason = %q, want the LATEST settlement %q", got, "completed")
	}
}

func TestBuildInspectTreeUsesDriverFold(t *testing.T) {
	dir := t.TempDir()
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		typ string
		v   any
	}{
		{event.TypeDriverStarted, &event.DriverStarted{DriverID: "drv", SpecName: "nightly", FoldVersion: 1}},
		{event.TypeIterationScheduled, &event.IterationScheduled{DriverID: "drv", Iter: 1}},
		{event.TypeIterationLaunched, &event.IterationLaunched{DriverID: "drv", Iter: 1, ChildSession: "drv-i1"}},
		{event.TypeIterationCompleted, &event.IterationCompleted{DriverID: "drv", Iter: 1, ChildSession: "drv-i1", ChildReason: "completed", Verdict: event.IterationVerdict{Pass: true, Score: 1}, Usage: provider.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 20, CacheWriteTokens: 5}}},
		{event.TypeDriverCompleted, &event.DriverCompleted{DriverID: "drv", Reason: "satisfied", Iterations: 1, BestIter: 1}},
	} {
		env := mkEnv(t, item.typ, item.v)
		if _, err := es.Append(env); err != nil {
			t.Fatal(err)
		}
	}
	if err := es.Close(); err != nil {
		t.Fatal(err)
	}
	report, err := buildInspectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Kind != "driver" || report.Spec != "nightly" || report.Status != "ended" ||
		report.Reason != "satisfied" || report.GenSteps != 1 || len(report.Entries) != 1 {
		t.Fatalf("driver report = %+v", report)
	}
	if report.Usage.InputTokens != 100 || report.Usage.OutputTokens != 50 ||
		report.Usage.CacheRead != 20 || report.Usage.CacheWrite != 5 || report.Usage.Billed != 130 {
		t.Fatalf("driver usage = %+v", report.Usage)
	}
}

func TestDriverUsageReportIncludesSettledRetryBeforeIterationCompletion(t *testing.T) {
	s := driver.State{Iterations: []driver.Iteration{{Attempts: []driver.Attempt{
		{Completed: true, Usage: provider.Usage{InputTokens: 60, OutputTokens: 40}},
		{Started: true},
	}}}}
	got := driverUsageReport(s)
	if got.InputTokens != 60 || got.OutputTokens != 40 || got.Billed != 100 {
		t.Fatalf("driver usage = %+v", got)
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

func TestSettleChildReportRemovesRunningProjection(t *testing.T) {
	report := inspectReport{
		Status: "running",
		Progress: []event.ProgressItem{
			{ID: "a", Status: "done"},
			{ID: "b", Status: "running"},
			{ID: "c", Status: "pending"},
		},
	}
	settleChildReport(&report, "canceled")
	if report.Status != "canceled" || report.Reason != "canceled" {
		t.Fatalf("terminal status = %q/%q", report.Status, report.Reason)
	}
	if report.Progress[1].Status != "failed" || report.Progress[2].Status != "failed" {
		t.Fatalf("terminal progress = %+v", report.Progress)
	}
}
