package driver_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/driver"
	"github.com/ralphite/agentrunner/internal/event"
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
		Name:         "worker",
		Model:        agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt: "make progress",
		Tools:        []string{"bash"},
		MaxTurns:     5,
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
		SystemPrompt: "work", Tools: []string{"bash"}, MaxTurns: 5,
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

// Loop mode with an interval parks on the clock between iterations: the driver
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
		SystemPrompt: "tick", MaxTurns: 5,
	}
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	// A text-only child arms no activity-timeout timer, so the fake clock's
	// only waiter is the driver's interval park — waitParked stays unambiguous.
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
		waitParked(t, clk)
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

// waitParked spins until the driver goroutine is blocked on the fake clock.
func waitParked(t *testing.T, clk *clock.Fake) {
	t.Helper()
	for i := 0; i < 5000; i++ {
		if clk.Waiters() > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("driver never parked on the interval")
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
