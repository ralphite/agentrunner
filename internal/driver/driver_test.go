package driver_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/driver"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// workFixture is one iteration's happy path: append a line to progress.txt
// and report a fixed 150 billed tokens.
func workFixture() scripted.Fixture {
	return scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "bash",
				Args: map[string]any{"command": "echo tick >> progress.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{
			{Text: "appended a line"},
			{Usage: &scripted.UsageEvent{InputTokens: 100, OutputTokens: 50}}, // billed 150
			{Finish: "end_turn"},
		}},
	}}
}

// harness wires a driver whose child runs workFixture each iteration over a
// shared workspace. The command verifier is supplied by the caller.
func harness(t *testing.T, spec *driver.DriverSpec) (*driver.Driver, *store.EventStore) {
	return harnessFix(t, spec, workFixture())
}

// harnessFix is harness with a caller-chosen child fixture — an empty fixture
// makes every child run fail (the provider exhausts on the first turn), which
// drives the on_child_failure paths.
func harnessFix(t *testing.T, spec *driver.DriverSpec, fix scripted.Fixture) (*driver.Driver, *store.EventStore) {
	t.Helper()
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	exec := &tool.Executor{WS: ws}
	dStore, err := store.OpenEventStore(filepath.Join(root, "driver"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dStore.Close() })

	childSpec := &agent.AgentSpec{
		Name:               "worker",
		Model:              agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt:       "make progress",
		Tools:              []string{"bash"},
		MaxGenerationSteps: 5,
	}
	spec.Agent = childSpec
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))

	d := &driver.Driver{
		Spec:     spec,
		Store:    dStore,
		Clock:    clk,
		DriverID: "drv-1",
		Exec:     exec,
		// The scripted child bills a fixed 150/iteration and does not enforce
		// its own cap (agent budget tests cover that) — this isolates the
		// DRIVER's reserve/settle/stop accounting.
		NewChild: func(cs *store.EventStore, session string, iter, budgetTokens int) *agent.Loop {
			return &agent.Loop{
				Spec:      childSpec,
				Provider:  scripted.New(fix),
				Exec:      exec,
				Store:     cs,
				Clock:     clk,
				SessionID: session,
			}
		},
	}
	return d, dStore
}

// Goal mode: a metric-free command verifier that passes once the workspace
// has accumulated enough lines. The child adds one line per iteration, so the
// goal is satisfied on exactly the third iteration.
func TestDriverGoalSatisfied(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "add a line", MaxIterations: 5,
		Verifiers: []driver.VerifierSpec{{
			Kind: driver.VerifierCommand, Command: "test $(wc -l < progress.txt) -ge 3",
		}},
	})

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.Iterations != 3 {
		t.Fatalf("res = %+v, want satisfied at iteration 3", res)
	}

	events, err := store.ReadEvents(dStore.Dir())
	if err != nil {
		t.Fatal(err)
	}
	st, err := driver.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != driver.StatusEnded || st.Reason != "satisfied" {
		t.Fatalf("fold status=%s reason=%s", st.Status, st.Reason)
	}
	if len(st.Iterations) != 3 {
		t.Fatalf("iterations = %d, want 3", len(st.Iterations))
	}
	if !st.Iterations[2].Completed || !st.Iterations[2].Verdict.Pass {
		t.Errorf("iteration 3 = %+v, want completed+pass", st.Iterations[2])
	}
	if st.Iterations[0].Verdict.Pass || st.Iterations[1].Verdict.Pass {
		t.Errorf("iterations 1-2 should have failed the verifier")
	}
	// The terminal DriverCompleted is the last fact.
	last := events[len(events)-1]
	if last.Type != event.TypeDriverCompleted {
		t.Fatalf("last event = %s, want driver_completed", last.Type)
	}
}

// Goal mode: a verifier that never passes ends the run at the iteration cap.
func TestDriverGoalMaxIterations(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "add a line", MaxIterations: 2,
		Verifiers: []driver.VerifierSpec{{
			Kind: driver.VerifierCommand, Command: "test -f never-created",
		}},
	})

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "max_iterations" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want max_iterations at 2", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if len(st.Iterations) != 2 {
		t.Fatalf("iterations = %d, want 2", len(st.Iterations))
	}
	// Each iteration DID run a child (fresh journal under sub/iter-N).
	for i := 1; i <= 2; i++ {
		sub := filepath.Join(dStore.Dir(), "sub", "iter-"+itoa(i))
		cev, err := store.ReadEvents(sub)
		if err != nil || len(cev) == 0 {
			t.Fatalf("child iter-%d journal missing", i)
		}
	}
}

// Goal mode with a scored metric verifier: the score is the current line
// count, threshold 3. Score climbs 1→2→3 and passes on the third.
func TestDriverMetricVerifier(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "add a line", MaxIterations: 5,
		Verifiers: []driver.VerifierSpec{{
			Kind:        driver.VerifierCommand,
			Command:     "wc -l < progress.txt",
			MetricRegex: `(\d+)`,
			Threshold:   3,
		}},
	})

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.Iterations != 3 {
		t.Fatalf("res = %+v, want satisfied at 3", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if got := st.Iterations[2].Verdict.Score; got != 3 {
		t.Errorf("iteration 3 score = %v, want 3", got)
	}
	if st.BestIter != 3 {
		t.Errorf("best iter = %d, want 3 (highest score)", st.BestIter)
	}
}

// Goal mode with patience: a metric verifier whose threshold is unreachable
// (the score plateaus once it stops climbing). Here the child appends one
// line per iteration so the score keeps rising — to force a plateau we cap
// the metric with a threshold above any reachable value AND a patience that
// trips once no NEW best appears. We assert the stalled path by making the
// verifier score constant (a fixed metric) so no iteration improves.
func TestDriverGoalStalled(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "add a line", MaxIterations: 10, Patience: 2,
		Verifiers: []driver.VerifierSpec{{
			Kind:        driver.VerifierCommand,
			Command:     "echo 1", // constant score 1, threshold 5 → never passes, never improves
			MetricRegex: `(\d+)`,
			Threshold:   5,
		}},
	})

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "stalled" {
		t.Fatalf("res = %+v, want stalled", res)
	}
	// Best is iteration 1 (first to reach the constant score); patience 2 means
	// two more non-improving iterations end it — iteration 3.
	if res.Iterations != 3 {
		t.Fatalf("stalled at iteration %d, want 3", res.Iterations)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if st.BestIter != 1 {
		t.Errorf("best iter = %d, want 1", st.BestIter)
	}
}

// Goal mode with a tree budget: each child bills 150; a 250-token budget
// admits two iterations, then the reserve refuses the third (spent 300 ≥ 250)
// and the run ends as budget.
func TestDriverBudgetStop(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "add a line", MaxIterations: 10,
		Budget: driver.BudgetSpec{MaxTotalTokens: 250},
		Verifiers: []driver.VerifierSpec{{
			Kind: driver.VerifierCommand, Command: "test -f never-created",
		}},
	})

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "budget" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want budget at iteration 2", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if st.SpentTokens != 300 {
		t.Errorf("spent = %d, want 300 (2×150)", st.SpentTokens)
	}
	last := events[len(events)-1]
	if last.Type != event.TypeDriverCompleted {
		t.Fatalf("last = %s, want driver_completed", last.Type)
	}
}

// on_child_failure default (stop): a failing child ends the driver as
// child_failed at the first iteration.
func TestDriverChildFailStop(t *testing.T) {
	d, dStore := harnessFix(t, &driver.DriverSpec{
		Name: "goal", Task: "work", MaxIterations: 5,
		Verifiers: []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "true"}},
	}, scripted.Fixture{}) // empty fixture → child errors on its first turn

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "child_failed" || res.Iterations != 1 {
		t.Fatalf("res = %+v, want child_failed at 1", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if len(st.Iterations) != 1 || st.Iterations[0].ChildReason != "error" {
		t.Fatalf("iteration 0 = %+v, want one error iteration", st.Iterations)
	}
}

// on_child_failure surface: a failing child is a spent iteration but the
// driver keeps going, exhausting max_iterations across failures.
func TestDriverChildFailSurface(t *testing.T) {
	d, dStore := harnessFix(t, &driver.DriverSpec{
		Name: "goal", Task: "work", MaxIterations: 3,
		OnChildFailure: driver.FailurePolicy{Mode: driver.OnFailSurface},
		Verifiers:      []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "true"}},
	}, scripted.Fixture{})

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "max_iterations" || res.Iterations != 3 {
		t.Fatalf("res = %+v, want max_iterations at 3 (all surfaced)", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if len(st.Iterations) != 3 {
		t.Fatalf("iterations = %d, want 3", len(st.Iterations))
	}
	for i, it := range st.Iterations {
		if it.ChildReason != "error" {
			t.Errorf("iteration %d reason = %q, want error", i+1, it.ChildReason)
		}
	}
}

// on_child_failure retry: the first two attempts fail, the third succeeds —
// the iteration recovers and the goal is satisfied, with all three attempt
// journals on disk.
func TestDriverChildFailRetryRecovers(t *testing.T) {
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	exec := &tool.Executor{WS: ws}
	dStore, err := store.OpenEventStore(filepath.Join(root, "driver"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dStore.Close() })

	childSpec := &agent.AgentSpec{
		Name: "worker", Model: agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt: "work", Tools: []string{"bash"}, MaxGenerationSteps: 5,
	}
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	calls := 0
	d := &driver.Driver{
		Spec: &driver.DriverSpec{
			Name: "goal", Task: "work", MaxIterations: 5, Agent: childSpec,
			OnChildFailure: driver.FailurePolicy{Mode: driver.OnFailRetry, Max: 2},
			Verifiers:      []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "test -f progress.txt"}},
		},
		Store: dStore, Clock: clk, DriverID: "drv-1", Exec: exec,
		NewChild: func(cs *store.EventStore, session string, iter, budget int) *agent.Loop {
			calls++
			fix := scripted.Fixture{} // attempts 1-2 fail
			if calls >= 3 {
				fix = workFixture() // attempt 3 writes progress.txt and succeeds
			}
			return &agent.Loop{Spec: childSpec, Provider: scripted.New(fix),
				Exec: exec, Store: cs, Clock: clk, SessionID: session}
		},
	}

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.Iterations != 1 {
		t.Fatalf("res = %+v, want satisfied at 1 (retry recovered)", res)
	}
	if calls != 3 {
		t.Errorf("child built %d times, want 3 (2 failures + 1 success)", calls)
	}
	// All three attempt journals exist on disk.
	for _, sub := range []string{"iter-1", "iter-1-a2", "iter-1-a3"} {
		if _, err := store.ReadEvents(filepath.Join(dStore.Dir(), "sub", sub)); err != nil {
			t.Errorf("attempt journal %s missing: %v", sub, err)
		}
	}
}

// llm_judge: the judge scores below threshold on iteration 1 and above on
// iteration 2, so the goal is satisfied on the second iteration.
func TestDriverLLMJudge(t *testing.T) {
	judge := scripted.New(scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: `{"score":0.5,"pass":false,"reason":"needs more"}`}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: `here: {"score":0.9,"pass":true,"reason":"good now"}`}, {Finish: "end_turn"}}},
	}})
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "work", MaxIterations: 5,
		Verifiers: []driver.VerifierSpec{{
			Kind: driver.VerifierLLMJudge, Rubric: "Is the work complete?", Threshold: 0.8,
		}},
	})
	d.Judge = judge

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want satisfied at 2", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if st.Iterations[0].Verdict.Pass || !st.Iterations[1].Verdict.Pass {
		t.Errorf("verdicts = [%+v, %+v], want fail then pass", st.Iterations[0].Verdict, st.Iterations[1].Verdict)
	}
	if st.Iterations[1].Verdict.Score != 0.9 {
		t.Errorf("iteration 2 score = %v, want 0.9 (judge parsed from wrapped prose)", st.Iterations[1].Verdict.Score)
	}
	if st.BestIter != 2 {
		t.Errorf("best iter = %d, want 2", st.BestIter)
	}
}

// stubResolver answers the human verifier's ask path without a tty or env.
type stubResolver struct{ approve bool }

func (s stubResolver) Resolve(context.Context, agent.ApprovalRequest) (agent.ApprovalDecision, error) {
	return agent.ApprovalDecision{Approve: s.approve, Reason: "stub", Source: "test"}, nil
}

// human verifier: an approving human satisfies the goal on the first iteration.
func TestDriverHumanVerifier(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "work", MaxIterations: 3,
		Verifiers: []driver.VerifierSpec{{Kind: driver.VerifierHuman, Rubric: "Does this meet the bar?"}},
	})
	d.Approvals = stubResolver{approve: true}

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.Iterations != 1 {
		t.Fatalf("res = %+v, want satisfied at 1", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if !st.Iterations[0].Verdict.Pass || st.Iterations[0].Verdict.Verifier != driver.VerifierHuman {
		t.Errorf("iteration 1 verdict = %+v, want human pass", st.Iterations[0].Verdict)
	}
}

// A denying human never satisfies the goal — the run exhausts max_iterations.
func TestDriverHumanVerifierDeny(t *testing.T) {
	d, _ := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "work", MaxIterations: 2,
		Verifiers: []driver.VerifierSpec{{Kind: driver.VerifierHuman, Rubric: "ok?"}},
	})
	d.Approvals = stubResolver{approve: false}

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "max_iterations" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want max_iterations at 2", res)
	}
}

// With an ArtifactStore, each completed iteration publishes its full carry to
// the CAS and IterationCompleted keeps the ref (resolving to the report) plus
// a short excerpt.
func TestDriverCarryToArtifactStore(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "work", MaxIterations: 5,
		Verifiers: []driver.VerifierSpec{{
			Kind: driver.VerifierCommand, Command: "test $(wc -l < progress.txt) -ge 2",
		}},
	})
	cas, err := store.OpenArtifactStore(filepath.Join(dStore.Dir(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	d.Artifacts = cas

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" {
		t.Fatalf("res = %+v, want satisfied", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	last := st.Iterations[len(st.Iterations)-1]
	if last.CarryRef == "" {
		t.Fatal("last iteration has no carry ref")
	}
	blob, err := cas.Get(last.CarryRef)
	if err != nil {
		t.Fatalf("carry ref does not resolve: %v", err)
	}
	if !strings.Contains(string(blob), "appended a line") {
		t.Errorf("carry blob = %q, want the child report", blob)
	}
}

// journal appends a raw driver-stream fact — used to synthesize the partial
// journals a crash would leave, without actually crashing a run.
func journal(t *testing.T, es *store.EventStore, typ string, p any) {
	t.Helper()
	env, err := event.New(typ, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := es.Append(env); err != nil {
		t.Fatal(err)
	}
}

// Resuming an already-ended driver returns its recorded result and appends
// nothing.
func TestDriverResumeEnded(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "work", MaxIterations: 5,
		Verifiers: []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "test $(wc -l < progress.txt) -ge 1"}},
	})
	res1, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := store.ReadEvents(dStore.Dir())

	res2, err := d.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res2.Reason != res1.Reason || res2.Iterations != res1.Iterations {
		t.Fatalf("resume = %+v, want the recorded %+v", res2, res1)
	}
	after, _ := store.ReadEvents(dStore.Dir())
	if len(after) != len(before) {
		t.Errorf("resume of an ended driver appended %d events", len(after)-len(before))
	}
}

// A crash between the satisfying IterationCompleted and DriverCompleted:
// resume re-derives satisfied from the fold without launching a new iteration.
func TestDriverResumeReDerivesSatisfied(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "work", MaxIterations: 5,
		Verifiers: []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "test $(wc -l < progress.txt) -ge 1"}},
	})
	journal(t, dStore, event.TypeIterationScheduled, &event.IterationScheduled{DriverID: "drv-1", Iter: 1, Schedule: "immediate"})
	journal(t, dStore, event.TypeIterationLaunched, &event.IterationLaunched{DriverID: "drv-1", Iter: 1, ChildSession: "drv-1-iter-1"})
	journal(t, dStore, event.TypeIterationCompleted, &event.IterationCompleted{
		DriverID: "drv-1", Iter: 1, ChildSession: "drv-1-iter-1", ChildReason: "completed",
		Verdict: event.IterationVerdict{Pass: true, Score: 1},
	})

	res, err := d.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.Iterations != 1 {
		t.Fatalf("res = %+v, want satisfied at 1 (re-derived, no new iteration)", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	if events[len(events)-1].Type != event.TypeDriverCompleted {
		t.Fatalf("last = %s, want driver_completed", events[len(events)-1].Type)
	}
	// No iteration 2 was ever launched.
	for _, e := range events {
		if e.Type == event.TypeIterationLaunched && strings.Contains(string(e.Payload), `"iter":2`) {
			t.Fatal("resume launched a redundant iteration 2")
		}
	}
}

// A crash after a FAILING iteration completed: resume continues from the next
// iteration and reaches the goal.
func TestDriverResumeContinues(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "work", MaxIterations: 5,
		Verifiers: []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "test $(wc -l < progress.txt) -ge 1"}},
	})
	journal(t, dStore, event.TypeIterationScheduled, &event.IterationScheduled{DriverID: "drv-1", Iter: 1, Schedule: "immediate"})
	journal(t, dStore, event.TypeIterationLaunched, &event.IterationLaunched{DriverID: "drv-1", Iter: 1, ChildSession: "drv-1-iter-1"})
	journal(t, dStore, event.TypeIterationCompleted, &event.IterationCompleted{
		DriverID: "drv-1", Iter: 1, ChildSession: "drv-1-iter-1", ChildReason: "completed",
		Verdict: event.IterationVerdict{Pass: false, Score: 0},
	})

	res, err := d.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Iteration 2 runs a real child (writes the first line) and satisfies.
	if res.Reason != "satisfied" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want satisfied at 2 (resumed past the failed iteration)", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if len(st.Iterations) != 2 || !st.Iterations[1].Verdict.Pass {
		t.Fatalf("iterations = %+v, want 2 with the second passing", st.Iterations)
	}
}

// Loop mode with no interval runs iterations back to back and, with no
// verifier, ends only at max_iterations — it never "satisfies".
func TestDriverLoopBackToBack(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "loop", Schedule: driver.ScheduleInterval, Task: "tick", MaxIterations: 3,
	})
	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "max_iterations" || res.Iterations != 3 {
		t.Fatalf("res = %+v, want max_iterations at 3", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if len(st.Iterations) != 3 {
		t.Fatalf("iterations = %d, want 3", len(st.Iterations))
	}
	for i, it := range st.Iterations {
		if it.Verdict.Pass {
			t.Errorf("iteration %d marked pass with no verifier", i+1)
		}
	}
}

// Loop mode with an interval goes idle on the clock between iterations: the driver
// runs iteration 1 immediately, then each later iteration waits for its tick.
func TestDriverLoopIntervalCadence(t *testing.T) {
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	exec := &tool.Executor{WS: ws}
	dStore, err := store.OpenEventStore(filepath.Join(root, "driver"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dStore.Close() })

	childSpec := &agent.AgentSpec{
		Name: "worker", Model: agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt: "tick", MaxGenerationSteps: 5,
	}
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	// A text-only child arms no activity-timeout timer, so the fake clock's
	// only waiter is the driver's interval idle — waitIdle stays unambiguous.
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "tick"}, {Finish: "end_turn"}}},
	}}
	d := &driver.Driver{
		Spec: &driver.DriverSpec{
			Name: "loop", Schedule: driver.ScheduleInterval, Interval: "1m",
			Task: "tick", MaxIterations: 3, Agent: childSpec,
		},
		Store: dStore, Clock: clk, DriverID: "drv-1", Exec: exec,
		NewChild: func(cs *store.EventStore, session string, iter, budget int) *agent.Loop {
			return &agent.Loop{Spec: childSpec, Provider: scripted.New(fix),
				Exec: exec, Store: cs, Clock: clk, SessionID: session}
		},
	}

	resCh := make(chan driver.Result, 1)
	go func() {
		res, rerr := d.Run(context.Background())
		if rerr != nil {
			t.Errorf("driver run: %v", rerr)
		}
		resCh <- res
	}()

	// Iteration 1 fires immediately; iterations 2 and 3 each wait for a tick.
	for i := 0; i < 2; i++ {
		waitIdle(t, clk)
		clk.Advance(time.Minute)
	}
	select {
	case res := <-resCh:
		if res.Reason != "max_iterations" || res.Iterations != 3 {
			t.Fatalf("res = %+v, want max_iterations at 3", res)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("driver did not finish after advancing the clock")
	}
}

// cronHarness wires a loop-mode cron driver with a text-only child (no bash
// timeout timer polluting Waiters). advanceOnCall1 simulates iteration 1
// running long: the factory advances the fake clock past later ticks.
func cronHarness(t *testing.T, spec *driver.DriverSpec, clk *clock.Fake, advanceOnCall1 time.Duration) (*driver.Driver, *store.EventStore) {
	t.Helper()
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	dStore, err := store.OpenEventStore(filepath.Join(root, "driver"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dStore.Close() })
	childSpec := &agent.AgentSpec{
		Name: "worker", Model: agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt: "tick", MaxGenerationSteps: 5,
	}
	spec.Agent = childSpec
	calls := 0
	d := &driver.Driver{
		Spec: spec, Store: dStore, Clock: clk, DriverID: "drv-1",
		Exec: &tool.Executor{WS: ws},
		NewChild: func(cs *store.EventStore, session string, iter, budget int) *agent.Loop {
			calls++
			if calls == 1 && advanceOnCall1 > 0 {
				clk.Advance(advanceOnCall1)
			}
			fix := scripted.Fixture{Steps: []scripted.Step{
				{Respond: []scripted.Event{{Text: "tick"}, {Finish: "end_turn"}}},
			}}
			return &agent.Loop{Spec: childSpec, Provider: scripted.New(fix),
				Exec: &tool.Executor{WS: ws}, Store: cs, Clock: clk, SessionID: session}
		},
	}
	return d, dStore
}

// Cron cadence with overlap=skip (default): iteration 1 waits for its tick
// and then runs 2h long, so the 02:00 and 03:00 ticks pass — each becomes a
// journaled IterationSkipped, and the next real run waits for 04:00.
func TestDriverCronOverlapSkip(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 30, 0, 0, time.UTC))
	d, dStore := cronHarness(t, &driver.DriverSpec{
		Name: "nightly", Schedule: driver.ScheduleCron, Cron: "0 * * * *",
		Task: "tick", MaxIterations: 4,
	}, clk, 2*time.Hour)

	resCh := make(chan driver.Result, 1)
	go func() {
		res, rerr := d.Run(context.Background())
		if rerr != nil {
			t.Errorf("driver run: %v", rerr)
		}
		resCh <- res
	}()

	// Idle 1: waiting for the 01:00 tick. Iteration 1 then runs "2h long".
	waitIdle(t, clk)
	clk.Advance(30 * time.Minute)
	// Ticks 02:00 and 03:00 were missed (skipped as iterations 2 and 3);
	// idle 2 waits for 04:00.
	waitIdle(t, clk)
	clk.Advance(time.Hour)

	res := <-resCh
	if res.Reason != "max_iterations" || res.Iterations != 4 {
		t.Fatalf("res = %+v, want max_iterations at 4", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if len(st.Iterations) != 4 {
		t.Fatalf("iterations = %d, want 4", len(st.Iterations))
	}
	wantSkipped := []bool{false, true, true, false}
	for i, it := range st.Iterations {
		if it.Skipped != wantSkipped[i] {
			t.Errorf("iteration %d skipped = %v, want %v", i+1, it.Skipped, wantSkipped[i])
		}
		if it.Completed != !wantSkipped[i] {
			t.Errorf("iteration %d completed = %v, want %v", i+1, it.Completed, !wantSkipped[i])
		}
	}
}

// Cron cadence with overlap=coalesce: the missed 02:00 and 03:00 ticks fold
// into ONE immediate iteration 2 — no skip events, no extra idle.
func TestDriverCronOverlapCoalesce(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 30, 0, 0, time.UTC))
	d, dStore := cronHarness(t, &driver.DriverSpec{
		Name: "nightly", Schedule: driver.ScheduleCron, Cron: "0 * * * *",
		Task: "tick", MaxIterations: 2, Overlap: driver.OverlapCoalesce,
	}, clk, 2*time.Hour)

	resCh := make(chan driver.Result, 1)
	go func() {
		res, rerr := d.Run(context.Background())
		if rerr != nil {
			t.Errorf("driver run: %v", rerr)
		}
		resCh <- res
	}()

	// One idle only (the 01:00 tick); iteration 2 coalesces and runs at once.
	waitIdle(t, clk)
	clk.Advance(30 * time.Minute)

	select {
	case res := <-resCh:
		if res.Reason != "max_iterations" || res.Iterations != 2 {
			t.Fatalf("res = %+v, want max_iterations at 2", res)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("coalesce should not idle again — the missed tick runs immediately")
	}
	events, _ := store.ReadEvents(dStore.Dir())
	for _, e := range events {
		if e.Type == event.TypeIterationSkipped {
			t.Error("coalesce must not journal IterationSkipped")
		}
	}
	st, _ := driver.Fold(events)
	if len(st.Iterations) != 2 || !st.Iterations[0].Completed || !st.Iterations[1].Completed {
		t.Fatalf("iterations = %+v, want 2 completed", st.Iterations)
	}
}

// selfPacedHarness wires a self_paced driver whose per-iteration fixture is
// chosen by call number (1-based). Fixtures should be text/data-tools only so
// the fake clock's Waiters reflects the pace idle alone.
func selfPacedHarness(t *testing.T, spec *driver.DriverSpec, clk *clock.Fake,
	fixFor func(call int) scripted.Fixture) (*driver.Driver, *store.EventStore) {
	t.Helper()
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	dStore, err := store.OpenEventStore(filepath.Join(root, "driver"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dStore.Close() })
	childSpec := &agent.AgentSpec{
		Name: "worker", Model: agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt: "pace yourself", MaxGenerationSteps: 5,
	}
	spec.Agent = childSpec
	exec := &tool.Executor{WS: ws}
	calls := 0
	d := &driver.Driver{
		Spec: spec, Store: dStore, Clock: clk, DriverID: "drv-1", Exec: exec,
		NewChild: func(cs *store.EventStore, session string, iter, budget int) *agent.Loop {
			calls++
			return &agent.Loop{Spec: childSpec, Provider: scripted.New(fixFor(calls)),
				Exec: exec, Store: cs, Clock: clk, SessionID: session}
		},
	}
	return d, dStore
}

func paceFixture(after string) scripted.Fixture {
	return scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "p1", Name: "schedule_next",
				Args: map[string]any{"after": after}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "scheduled the next pass"}, {Finish: "end_turn"}}},
	}}
}

func finishFixture() scripted.Fixture {
	return scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "f1", Name: "finish_series",
				Args: map[string]any{"reason": "all caught up"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done with the series"}, {Finish: "end_turn"}}},
	}}
}

// self_paced: iteration 1 declares schedule_next{1m} (the driver goes idle on
// it); iteration 2 claims finish_series and the human gate approves —
// the series ends satisfied.
func TestDriverSelfPaced(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	d, dStore := selfPacedHarness(t, &driver.DriverSpec{
		Name: "series", Schedule: driver.ScheduleSelfPaced, Task: "keep up", MaxIterations: 5,
	}, clk, func(call int) scripted.Fixture {
		if call == 1 {
			return paceFixture("1m")
		}
		return finishFixture()
	})
	d.Approvals = stubResolver{approve: true}

	resCh := make(chan driver.Result, 1)
	go func() {
		res, rerr := d.Run(context.Background())
		if rerr != nil {
			t.Errorf("driver run: %v", rerr)
		}
		resCh <- res
	}()

	waitIdle(t, clk) // idle on the declared 1m pace
	clk.Advance(time.Minute)

	res := <-resCh
	if res.Reason != "satisfied" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want satisfied at 2 (approved finish_series)", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if len(st.Iterations) != 2 || !st.Iterations[0].Completed || !st.Iterations[1].Completed {
		t.Fatalf("iterations = %+v", st.Iterations)
	}
}

// self_paced default on_no_intent (finish): a child that neither schedules
// nor claims completion ends the series after its iteration.
func TestDriverSelfPacedNoIntentFinish(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	d, _ := selfPacedHarness(t, &driver.DriverSpec{
		Name: "series", Schedule: driver.ScheduleSelfPaced, Task: "one shot?", MaxIterations: 5,
	}, clk, func(int) scripted.Fixture {
		return scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "nothing more to plan"}, {Finish: "end_turn"}}},
		}}
	})

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.Iterations != 1 {
		t.Fatalf("res = %+v, want satisfied at 1 (no intent → finish)", res)
	}
}

// self_paced clamp: a 10h request under pace_max=1h goes idle exactly 1h.
func TestDriverSelfPacedClamp(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	d, _ := selfPacedHarness(t, &driver.DriverSpec{
		Name: "series", Schedule: driver.ScheduleSelfPaced, Task: "pace", MaxIterations: 5,
		PaceMax: "1h",
	}, clk, func(call int) scripted.Fixture {
		if call == 1 {
			return paceFixture("10h")
		}
		return scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "no more"}, {Finish: "end_turn"}}},
		}}
	})

	resCh := make(chan driver.Result, 1)
	go func() {
		res, rerr := d.Run(context.Background())
		if rerr != nil {
			t.Errorf("driver run: %v", rerr)
		}
		resCh <- res
	}()

	waitIdle(t, clk)
	clk.Advance(time.Hour) // 1h suffices only because the 10h ask was clamped

	select {
	case res := <-resCh:
		if res.Reason != "satisfied" || res.Iterations != 2 {
			t.Fatalf("res = %+v, want satisfied at 2", res)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("driver still idle after 1h — the pace was not clamped to pace_max")
	}
}

// self_paced denied finish: the human gate rejects the claim, the series
// continues (floor pace 0 → immediately) and ends on the next iteration's
// no-intent finish.
func TestDriverSelfPacedFinishDenied(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	d, dStore := selfPacedHarness(t, &driver.DriverSpec{
		Name: "series", Schedule: driver.ScheduleSelfPaced, Task: "try to quit", MaxIterations: 5,
	}, clk, func(call int) scripted.Fixture {
		if call == 1 {
			return finishFixture()
		}
		return scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "ok, wrapping up quietly"}, {Finish: "end_turn"}}},
		}}
	})
	d.Approvals = stubResolver{approve: false}

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want the denied finish to force iteration 2", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if len(st.Iterations) != 2 {
		t.Fatalf("iterations = %d, want 2 (finish denied → continued)", len(st.Iterations))
	}
}

// Series memory: iteration 1 writes the memory doc; iteration 2's task must
// carry it (the child's scripted Expect asserts the injected block — a
// mismatch fails the child and the driver would end child_failed).
func TestDriverSeriesMemoryInjection(t *testing.T) {
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	exec := &tool.Executor{WS: ws}
	dStore, err := store.OpenEventStore(filepath.Join(root, "driver"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dStore.Close() })
	childSpec := &agent.AgentSpec{
		Name: "worker", Model: agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt: "keep a series log", Tools: []string{"bash"}, MaxGenerationSteps: 5,
	}
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	calls := 0
	d := &driver.Driver{
		Spec: &driver.DriverSpec{
			Name: "series", Schedule: driver.ScheduleInterval, Task: "do the rounds",
			MaxIterations: 2, Agent: childSpec, SeriesMemory: "SERIES.md",
		},
		Store: dStore, Clock: clk, DriverID: "drv-1", Exec: exec,
		NewChild: func(cs *store.EventStore, session string, iter, budget int) *agent.Loop {
			calls++
			var fix scripted.Fixture
			if calls == 1 {
				fix = scripted.Fixture{Steps: []scripted.Step{
					{Respond: []scripted.Event{
						{ToolCall: &scripted.ToolCallEvent{CallID: "w1", Name: "bash",
							Args: map[string]any{"command": "echo remember-the-milk > SERIES.md"}}},
						{Finish: "tool_use"},
					}},
					{Respond: []scripted.Event{{Text: "noted"}, {Finish: "end_turn"}}},
				}}
			} else {
				fix = scripted.Fixture{Steps: []scripted.Step{
					{
						// The injected series memory rides the task — the run's
						// first (and here last) user message.
						Expect:  scripted.Expect{LastMessageContains: "remember-the-milk"},
						Respond: []scripted.Event{{Text: "picked up where I left off"}, {Finish: "end_turn"}},
					},
				}}
			}
			return &agent.Loop{Spec: childSpec, Provider: scripted.New(fix),
				Exec: exec, Store: cs, Clock: clk, SessionID: session}
		},
	}

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "max_iterations" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want both iterations completed (Expect matched)", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if !st.Iterations[1].Completed || st.Iterations[1].ChildReason != "completed" {
		t.Fatalf("iteration 2 = %+v — the memory block did not reach the child", st.Iterations[1])
	}
}

// waitIdle spins until the driver goroutine is blocked on the fake clock.
func waitIdle(t *testing.T, clk *clock.Fake) {
	t.Helper()
	for i := 0; i < 5000; i++ {
		if clk.Waiters() > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("driver never idle on the interval")
}

// S6 review P0: a loop-mode series whose last iteration PASSED its quality
// verifier must survive a restart — resume must NOT re-derive "satisfied"
// (that is goal-mode semantics only).
func TestDriverLoopResumeContinues(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "rounds", Schedule: driver.ScheduleInterval, Task: "tick", MaxIterations: 3,
		Verifiers: []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "true"}},
	})
	journal(t, dStore, event.TypeIterationScheduled, &event.IterationScheduled{DriverID: "drv-1", Iter: 1})
	journal(t, dStore, event.TypeIterationLaunched, &event.IterationLaunched{DriverID: "drv-1", Iter: 1, ChildSession: "drv-1-iter-1"})
	journal(t, dStore, event.TypeIterationCompleted, &event.IterationCompleted{
		DriverID: "drv-1", Iter: 1, ChildSession: "drv-1-iter-1", ChildReason: "completed",
		Verdict: event.IterationVerdict{Pass: true, Score: 1},
	})

	res, err := d.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "max_iterations" || res.Iterations != 3 {
		t.Fatalf("res = %+v, want the series to CONTINUE to max_iterations, not end satisfied", res)
	}
}

// S6 review: a skipped iteration is a consumed slot — resume must not
// re-run it (the fold would end self-contradictory: Skipped AND Completed).
func TestDriverResumeSkipsSkippedIterations(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "rounds", Schedule: driver.ScheduleInterval, Task: "tick", MaxIterations: 3,
	})
	journal(t, dStore, event.TypeIterationScheduled, &event.IterationScheduled{DriverID: "drv-1", Iter: 1})
	journal(t, dStore, event.TypeIterationLaunched, &event.IterationLaunched{DriverID: "drv-1", Iter: 1, ChildSession: "drv-1-iter-1"})
	journal(t, dStore, event.TypeIterationCompleted, &event.IterationCompleted{
		DriverID: "drv-1", Iter: 1, ChildSession: "drv-1-iter-1", ChildReason: "completed"})
	journal(t, dStore, event.TypeIterationSkipped, &event.IterationSkipped{DriverID: "drv-1", Iter: 2, Reason: "overlap"})

	res, err := d.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "max_iterations" {
		t.Fatalf("res = %+v", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if st.Iterations[1].Completed || !st.Iterations[1].Skipped {
		t.Fatalf("iteration 2 = %+v — a skipped slot was re-run on resume", st.Iterations[1])
	}
	if !st.Iterations[2].Completed {
		t.Fatalf("iteration 3 = %+v, want completed", st.Iterations[2])
	}
}

// S6 review: the pace a self_paced child declared must survive a driver
// restart — resume re-derives it from the child journal and goes idle on it
// instead of firing immediately.
func TestDriverSelfPacedResumeRespectsPace(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	d, dStore := selfPacedHarness(t, &driver.DriverSpec{
		Name: "series", Schedule: driver.ScheduleSelfPaced, Task: "keep up", MaxIterations: 5,
	}, clk, func(int) scripted.Fixture {
		return scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "no more"}, {Finish: "end_turn"}}},
		}}
	})

	// Synthesize the pre-crash state: iteration 1 completed, and its child
	// journal carries a successful schedule_next{1h} declaration.
	journal(t, dStore, event.TypeIterationScheduled, &event.IterationScheduled{DriverID: "drv-1", Iter: 1})
	journal(t, dStore, event.TypeIterationLaunched, &event.IterationLaunched{DriverID: "drv-1", Iter: 1, ChildSession: "drv-1-iter-1"})
	journal(t, dStore, event.TypeIterationCompleted, &event.IterationCompleted{
		DriverID: "drv-1", Iter: 1, ChildSession: "drv-1-iter-1", ChildReason: "completed"})
	childDir := filepath.Join(dStore.Dir(), "sub", "iter-1")
	ces, err := store.OpenEventStore(childDir)
	if err != nil {
		t.Fatal(err)
	}
	cj := func(typ string, p any) {
		t.Helper()
		env, err := event.New(typ, p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ces.Append(env); err != nil {
			t.Fatal(err)
		}
	}
	cj(event.TypeSessionStarted, &event.SessionStarted{SpecName: "worker", Model: "x", Task: "keep up"})
	cj(event.TypeAssistantMessage, &event.AssistantMessage{GenStep: 1, Message: provider.Message{
		Role: provider.RoleAssistant,
		Parts: []provider.Part{{Kind: provider.PartToolCall, CallID: "p1",
			ToolName: "schedule_next", Args: json.RawMessage(`{"after":"1h"}`)}},
	}})
	cj(event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: "tool-p1", Kind: event.KindTool, Name: "schedule_next", CallID: "p1", Attempt: 1})
	cj(event.TypeActivityCompleted, &event.ActivityCompleted{
		ActivityID: "tool-p1", Result: json.RawMessage(`{"output":"ok"}`)})
	cj(event.TypeRunEnded, &event.RunEnded{Reason: "completed", GenSteps: 1})
	_ = ces.Close()

	resCh := make(chan driver.Result, 1)
	go func() {
		res, rerr := d.Resume(context.Background())
		if rerr != nil {
			t.Errorf("resume: %v", rerr)
		}
		resCh <- res
	}()

	// The resumed driver must PARK on the declared 1h pace, not fire now.
	waitIdle(t, clk)
	select {
	case res := <-resCh:
		t.Fatalf("resume fired iteration 2 without honoring the pace: %+v", res)
	default:
	}
	clk.Advance(time.Hour)
	res := <-resCh
	// Iteration 2's child declares nothing → on_no_intent finish → satisfied.
	if res.Reason != "satisfied" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want satisfied at 2 after honoring the pace", res)
	}
}

// S6 review: retried attempts burn real tokens — the iteration's journaled
// usage must sum EVERY attempt, not just the last.
func TestDriverRetrySpendSettlesAllAttempts(t *testing.T) {
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	exec := &tool.Executor{WS: ws}
	dStore, err := store.OpenEventStore(filepath.Join(root, "driver"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dStore.Close() })
	childSpec := &agent.AgentSpec{
		Name: "worker", Model: agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt: "work", MaxGenerationSteps: 5,
	}
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	calls := 0
	d := &driver.Driver{
		Spec: &driver.DriverSpec{
			Name: "goal", Task: "work", MaxIterations: 2, Agent: childSpec,
			OnChildFailure: driver.FailurePolicy{Mode: driver.OnFailRetry, Max: 1},
			Verifiers:      []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "true"}},
		},
		Store: dStore, Clock: clk, DriverID: "drv-1", Exec: exec,
		NewChild: func(cs *store.EventStore, session string, iter, budget int) *agent.Loop {
			calls++
			var fix scripted.Fixture
			if calls == 1 {
				// Attempt 1: bill 100, then die (fixture exhausted mid-run —
				// the next request errors after this turn's usage settled).
				fix = scripted.Fixture{Steps: []scripted.Step{
					{Respond: []scripted.Event{
						{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "bash",
							Args: map[string]any{"command": "true"}}},
						{Usage: &scripted.UsageEvent{InputTokens: 60, OutputTokens: 40}},
						{Finish: "tool_use"},
					}},
				}}
			} else {
				// Attempt 2: bill 150 and complete.
				fix = scripted.Fixture{Steps: []scripted.Step{
					{Respond: []scripted.Event{
						{Text: "done"},
						{Usage: &scripted.UsageEvent{InputTokens: 100, OutputTokens: 50}},
						{Finish: "end_turn"},
					}},
				}}
			}
			childSpec2 := *childSpec
			childSpec2.Tools = []string{"bash"}
			return &agent.Loop{Spec: &childSpec2, Provider: scripted.New(fix),
				Exec: exec, Store: cs, Clock: clk, SessionID: session}
		},
	}
	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" {
		t.Fatalf("res = %+v", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	// 100 (failed attempt) + 150 (successful attempt) = 250 billed.
	if st.SpentTokens != 250 {
		t.Fatalf("spent = %d, want 250 (both attempts settled)", st.SpentTokens)
	}
}

// S7 还债①: verifiers are adjudicated effects with a journaled trace. An
// explicit user deny rule blocks the command verifier (fail closed, reason
// visible); the journal carries EffectRequested/Resolved and the activity
// bracket for every verifier execution.
func TestVerifierPipelineDenyBinds(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "add a line", MaxIterations: 2,
		Verifiers: []driver.VerifierSpec{{
			Kind: driver.VerifierCommand, Command: "test -f progress.txt",
		}},
	})
	d.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		&pipeline.PermissionGate{Rules: []pipeline.PermissionRule{
			{Command: "test *", Action: "deny"},
			{Action: "allow"},
		}},
	}}

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The verifier can never pass (denied), so the goal exhausts iterations.
	if res.Reason != "max_iterations" {
		t.Fatalf("res = %+v, want max_iterations (denied verifier must not pass)", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if !strings.Contains(st.Iterations[0].Verdict.Detail, "denied") {
		t.Fatalf("verdict detail = %q, want the denial reason", st.Iterations[0].Verdict.Detail)
	}
	var sawDenyResolved bool
	for _, e := range events {
		if e.Type == event.TypeEffectResolved && strings.Contains(string(e.Payload), `"deny"`) {
			sawDenyResolved = true
		}
	}
	if !sawDenyResolved {
		t.Fatal("the denial was not journaled as an EffectResolved")
	}
}

// S7 还债①: an allowed verifier leaves the full trace — effect resolution
// plus the ActivityStarted/Completed bracket with the verdict as result.
func TestVerifierActivityTrace(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "add a line", MaxIterations: 3,
		Verifiers: []driver.VerifierSpec{{
			Kind: driver.VerifierCommand, Command: "test $(wc -l < progress.txt) -ge 2",
		}},
	})
	// nil Pipeline = allow-all; the trace must exist regardless.
	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" {
		t.Fatalf("res = %+v", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	var requested, resolvedAllow, started, completed int
	for _, e := range events {
		switch e.Type {
		case event.TypeEffectRequested:
			requested++
		case event.TypeEffectResolved:
			if strings.Contains(string(e.Payload), `"allow"`) {
				resolvedAllow++
			}
		case event.TypeActivityStarted:
			if strings.Contains(string(e.Payload), "verifier:command") {
				started++
			}
		case event.TypeActivityCompleted:
			if strings.Contains(string(e.Payload), "verify-i") {
				completed++
			}
		}
	}
	// Two iterations verified (satisfied on the second).
	if requested != 2 || resolvedAllow != 2 || started != 2 || completed != 2 {
		t.Fatalf("trace = requested %d, allow %d, started %d, completed %d — want 2 each",
			requested, resolvedAllow, started, completed)
	}
}

// S7 还债①: ask tightens to deny — a config-declared verifier has nobody
// behind it to answer.
func TestVerifierAskTightensToDeny(t *testing.T) {
	d, _ := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "add a line", MaxIterations: 1,
		Verifiers: []driver.VerifierSpec{{
			Kind: driver.VerifierCommand, Command: "true",
		}},
	})
	d.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		&pipeline.PermissionGate{Rules: []pipeline.PermissionRule{
			{Command: "*", Action: "ask"},
		}},
	}}
	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "max_iterations" {
		t.Fatalf("res = %+v, want the ask-tightened denial to block satisfaction", res)
	}
}

// S7 还债: the stream header guards resume — a fold-version mismatch is
// refused loudly, never silently migrated; headerless S6 streams (the other
// resume tests) keep resuming as version 1.
func TestDriverResumeRefusesVersionMismatch(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "t", MaxIterations: 2,
		Verifiers: []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "true"}},
	})
	journal(t, dStore, event.TypeDriverStarted, &event.DriverStarted{
		DriverID: "drv-1", SpecName: "goal", FoldVersion: 99,
	})
	if _, err := d.Resume(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "fold version 99") {
		t.Fatalf("err = %v, want a version-mismatch refusal", err)
	}
}

// A fresh run's stream now opens with the header carrying spec provenance.
func TestDriverStreamHeader(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "add a line", MaxIterations: 5,
		Verifiers: []driver.VerifierSpec{{
			Kind: driver.VerifierCommand, Command: "test -f progress.txt",
		}},
	})
	if _, err := d.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	if events[0].Type != event.TypeDriverStarted {
		t.Fatalf("first event = %s, want driver_started", events[0].Type)
	}
	decoded, err := event.DecodePayload(events[0])
	if err != nil {
		t.Fatal(err)
	}
	h := decoded.(*event.DriverStarted)
	if h.FoldVersion != driver.FoldVersion || h.SpecName != "goal" || len(h.Spec) == 0 {
		t.Fatalf("header = %+v", h)
	}
}

// Every event in event.DriverStream must fold into driver state — the mirror
// of the run fold's TestApplyCoversRegistry, so no driver-stream type is left
// unhandled anywhere.
func TestFoldCoversDriverStream(t *testing.T) {
	for typ := range event.DriverStream {
		e, err := event.New(typ, event.Registry[typ]())
		if err != nil {
			t.Fatalf("new %s: %v", typ, err)
		}
		if _, err := driver.Fold([]event.Envelope{e}); err != nil {
			t.Errorf("driver fold rejects %q: %v", typ, err)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
