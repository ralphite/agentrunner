package driver_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/driver"
	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// INC-77.2 goal form: the merged stream is a SESSION journal — head
// SessionStarted, iterations as spawn facts + SeriesIteration, terminal
// SeriesEnded — and the session fold carries the whole series position.
func TestSeriesGoalSatisfied(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Prompt: "add a line", MaxIterations: 5,
		Verifiers: []driver.VerifierSpec{{
			Kind: driver.VerifierCommand, Command: "test $(wc -l < progress.txt) -ge 3",
		}},
	})

	res, err := d.RunSeries(context.Background())
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
	if events[0].Type != event.TypeSessionStarted {
		t.Fatalf("head = %s, want session_started (the merged stream IS a session journal)", events[0].Type)
	}
	if last := events[len(events)-1]; last.Type != event.TypeSeriesEnded {
		t.Fatalf("last event = %s, want series_ended", last.Type)
	}
	spawns, subagents, legacy := 0, 0, 0
	for _, e := range events {
		switch e.Type {
		case event.TypeSpawnRequested:
			spawns++
		case event.TypeSubagentCompleted:
			subagents++
		case event.TypeDriverStarted, event.TypeIterationScheduled, event.TypeIterationCompleted,
			event.TypeIterationSkipped, event.TypeDriverCompleted:
			legacy++
		}
	}
	if spawns != 3 || subagents != 3 {
		t.Fatalf("spawn facts = %d/%d, want 3/3 (iterations ride the spawn vocabulary)", spawns, subagents)
	}
	if legacy != 0 {
		t.Fatalf("legacy driver events in the merged stream: %d, want 0", legacy)
	}
	ss, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	sr := ss.Series
	if sr == nil || !sr.Ended || sr.EndReason != "satisfied" || len(sr.Iterations) != 3 {
		t.Fatalf("series fold = %+v, want ended satisfied with 3 iterations", sr)
	}
	if !sr.Iterations[2].Verdict.Pass || sr.Iterations[0].Verdict.Pass {
		t.Fatalf("verdicts wrong: %+v", sr.Iterations)
	}
	if sr.SpentTokens == 0 {
		t.Fatal("series spend did not settle (settle-at-completion is pure fold)")
	}
	if len(ss.Session.ChildSessions) != 3 {
		t.Fatalf("child sessions = %v, want 3 recorded", ss.Session.ChildSessions)
	}

	// A resume of an ended series returns the recorded result — no re-run.
	res2, err := d.ResumeSeries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res2.Reason != "satisfied" || res2.Iterations != 3 {
		t.Fatalf("resume of ended series = %+v", res2)
	}
}

// seriesLoopHarness wires an interval-mode merged-stream series whose first
// child run advances the fake clock past later ticks (a long iteration).
func seriesLoopHarness(t *testing.T, overlap string, max int) (*driver.Driver, *store.EventStore, *clock.Fake) {
	t.Helper()
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	dStore, err := store.OpenEventStore(filepath.Join(root, "series"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dStore.Close() })
	clk := clock.NewFake(time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC))
	childSpec := &agent.AgentSpec{
		Name: "worker", Model: agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt: "tick", MaxGenerationSteps: 5,
	}
	calls := 0
	d := &driver.Driver{
		Spec: &driver.DriverSpec{
			Name: "loop", Schedule: driver.ScheduleInterval, Interval: "1m", Overlap: overlap,
			Prompt: "tick", MaxIterations: max, Agent: childSpec,
		},
		Store: dStore, Clock: clk, DriverID: "ser-1", Exec: &tool.Executor{WS: ws},
		NewChild: func(cs *store.EventStore, session string, iter, budget int) *agent.Loop {
			calls++
			if calls == 1 {
				clk.Advance(150 * time.Second)
			}
			fix := scripted.Fixture{Steps: []scripted.Step{
				{Respond: []scripted.Event{{Text: "tick"}, {Finish: "end_turn"}}},
			}}
			return &agent.Loop{Spec: childSpec, Provider: scripted.New(fix),
				Exec: &tool.Executor{WS: ws}, Store: cs, Clock: clk, SessionID: session}
		},
	}
	return d, dStore, clk
}

// INC-77.2 loop form: interval cadence with overlap=skip journals skipped
// slots as SeriesIteration{skipped} facts, and each real wait is bracketed
// by a durable series_tick TimerSet/TimerFired pair (the daemon's wake
// hint).
func TestSeriesIntervalOverlapSkip(t *testing.T) {
	d, dStore, clk := seriesLoopHarness(t, driver.OverlapSkip, 4)
	resCh := make(chan driver.Result, 1)
	go func() {
		res, err := d.RunSeries(context.Background())
		if err != nil {
			t.Errorf("run: %v", err)
		}
		resCh <- res
	}()
	waitIdle(t, clk)
	clk.Advance(30 * time.Second)
	res := <-resCh
	if res.Reason != "max_iterations" {
		t.Fatalf("result = %+v", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	ss, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	sr := ss.Series
	wantSkipped := []bool{false, true, true, false}
	if len(sr.Iterations) != len(wantSkipped) {
		t.Fatalf("iterations = %d, want %d: %+v", len(sr.Iterations), len(wantSkipped), sr.Iterations)
	}
	for i, want := range wantSkipped {
		if sr.Iterations[i].Skipped != want {
			t.Errorf("iteration %d skipped=%v, want %v", i+1, sr.Iterations[i].Skipped, want)
		}
	}
	timerSets, timerFires := 0, 0
	for _, e := range events {
		switch e.Type {
		case event.TypeTimerSet:
			timerSets++
		case event.TypeTimerFired:
			timerFires++
		}
	}
	if timerSets == 0 || timerSets != timerFires {
		t.Fatalf("tick timers set=%d fired=%d, want equal and >0 (durable wake hints)", timerSets, timerFires)
	}
	if len(ss.Timers) != 0 {
		t.Fatalf("pending timers after series end = %v, want none", ss.Timers)
	}
}

// INC-83 (PLAN 6.1): a USER cancel of a running series lands the DOMAIN
// terminal SeriesEnded{cancelled} — the loop/round itself is over; the
// session carries no lifecycle mark (no SessionClosed) and the sweep never
// revives an ended series.
func TestSeriesUserCancelWritesCancelledTerminal(t *testing.T) {
	d, dStore, clk := seriesLoopHarness(t, driver.OverlapCoalesce, 3)
	ctx, cancel := context.WithCancelCause(context.Background())
	resCh := make(chan driver.Result, 1)
	go func() {
		res, _ := d.RunSeries(ctx)
		resCh <- res
	}()
	waitIdle(t, clk)
	cancel(errs.ErrSessionStopped)
	res := <-resCh
	if res.Reason != "cancelled" {
		t.Fatalf("cancel result = %+v, want reason cancelled", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	ss, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if ss.Series == nil || !ss.Series.Ended || ss.Series.EndReason != "cancelled" {
		t.Fatalf("series fold = %+v, want Ended cancelled", ss.Series)
	}
	if ss.Session.Closed != nil {
		t.Fatalf("user cancel left a session lifecycle mark: %+v — the terminal belongs to the SERIES", ss.Session.Closed)
	}
}

// INC-77.2 INC-72 semantics carried over: a graceful host shutdown ends a
// cadenced series WITHOUT a terminal, and ResumeSeries continues it — the
// pending tick timer is cancelled (a wake hint, not a fact source) and the
// cadence re-anchors from the fold's LastTick.
func TestSeriesShutdownLeavesNoTerminalAndResumes(t *testing.T) {
	d, dStore, clk := seriesLoopHarness(t, driver.OverlapCoalesce, 3)
	ctx, cancel := context.WithCancelCause(context.Background())
	resCh := make(chan driver.Result, 1)
	go func() {
		res, _ := d.RunSeries(ctx)
		resCh <- res
	}()
	// Iteration 1 runs immediately (its child advances the clock 150s);
	// iteration 2 coalesces the passed ticks and runs immediately too;
	// iteration 3 then waits for the next slot — shut the host down
	// during that wait.
	waitIdle(t, clk)
	cancel(errs.ErrHostShutdown)
	res := <-resCh
	if res.Reason != "shutdown" {
		t.Fatalf("shutdown result = %+v", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	for _, e := range events {
		if e.Type == event.TypeSeriesEnded {
			t.Fatal("graceful shutdown wrote a series_ended terminal — INC-72 red line")
		}
	}

	// Restart: the missed slot backfills per the overlap policy and the
	// series runs to its cap with exactly one terminal.
	clk.Advance(5 * time.Minute)
	res2, err := d.ResumeSeries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res2.Reason != "max_iterations" || res2.Iterations != 3 {
		t.Fatalf("resume result = %+v, want max_iterations at 3", res2)
	}
	events, _ = store.ReadEvents(dStore.Dir())
	ss, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if ss.Series == nil || !ss.Series.Ended {
		t.Fatalf("series not ended after resume: %+v", ss.Series)
	}
	if len(ss.Timers) != 0 {
		t.Fatalf("stale timers after resume = %v, want none (hints cancelled)", ss.Timers)
	}
	terminals := 0
	for _, e := range events {
		if e.Type == event.TypeSeriesEnded {
			terminals++
		}
	}
	if terminals != 1 {
		t.Fatalf("series_ended count = %d, want exactly 1", terminals)
	}
}

// INC-80.2b step 1: on_child_failure=retry in the merged stream — each
// attempt is its own spawn fact pair with its own child journal
// `sub/iter-N-aM`, spend SUMS every attempt, and a recovered retry
// satisfies the goal on iteration 1.
func TestSeriesChildFailRetryRecovers(t *testing.T) {
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
			Name: "goal", Prompt: "work", MaxIterations: 5, Agent: childSpec,
			OnChildFailure: driver.FailurePolicy{Mode: driver.OnFailRetry, Max: 2},
			Verifiers:      []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "test -f progress.txt"}},
		},
		Store: dStore, Clock: clk, DriverID: "drv-1", Exec: exec,
		NewChild: func(cs *store.EventStore, session string, iter, budget int) *agent.Loop {
			calls++
			fix := scripted.Fixture{} // attempts 1-2 fail (provider exhausts)
			if calls >= 3 {
				fix = workFixture()
			}
			return &agent.Loop{Spec: childSpec, Provider: scripted.New(fix),
				Exec: exec, Store: cs, Clock: clk, SessionID: session}
		},
	}

	res, err := d.RunSeries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.Iterations != 1 {
		t.Fatalf("res = %+v, want satisfied at 1 (retry recovered)", res)
	}
	if calls != 3 {
		t.Errorf("child built %d times, want 3 (2 failures + 1 success)", calls)
	}
	// All three attempt journals exist, one per attempt suffix.
	for _, sub := range []string{"iter-1-a1", "iter-1-a2", "iter-1-a3"} {
		if _, err := store.ReadEvents(filepath.Join(dStore.Dir(), "sub", sub)); err != nil {
			t.Errorf("attempt journal %s missing: %v", sub, err)
		}
	}
	events, err := store.ReadEvents(dStore.Dir())
	if err != nil {
		t.Fatal(err)
	}
	spawns, completions := 0, 0
	var failedAttempts int
	for _, e := range events {
		switch e.Type {
		case event.TypeSpawnRequested:
			spawns++
		case event.TypeSubagentCompleted:
			completions++
			if dec, derr := event.DecodePayload(e); derr == nil {
				if dec.(*event.SubagentCompleted).Reason == "error" {
					failedAttempts++
				}
			}
		}
	}
	if spawns != 3 || completions != 3 || failedAttempts != 2 {
		t.Fatalf("spawn facts = %d/%d (failed %d), want 3/3 with 2 failed attempts", spawns, completions, failedAttempts)
	}
	ss, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if ss.Series == nil || !ss.Series.Ended || ss.Series.EndReason != "satisfied" {
		t.Fatalf("series fold = %+v", ss.Series)
	}
	// Spend doctrine: the iteration's usage SUMS all attempts. The failed
	// fixtures here exhaust before billing (0 tokens each), so the sum is
	// exactly the successful attempt's 150 — the point is that nothing
	// billed goes missing across attempts.
	if got := ss.Series.Iterations[0].Usage.Billed(); got != 150 {
		t.Fatalf("iteration usage = %d, want 150 (all attempts' spend summed)", got)
	}
}

// on_child_failure=retry that never recovers exhausts its attempts and ends
// the series child_failed — with every attempt journaled.
func TestSeriesChildFailRetryExhausts(t *testing.T) {
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
	d := &driver.Driver{
		Spec: &driver.DriverSpec{
			Name: "goal", Prompt: "work", MaxIterations: 3, Agent: childSpec,
			OnChildFailure: driver.FailurePolicy{Mode: driver.OnFailRetry, Max: 1},
			Verifiers:      []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "test -f progress.txt"}},
		},
		Store: dStore, Clock: clk, DriverID: "drv-1", Exec: exec,
		NewChild: func(cs *store.EventStore, session string, iter, budget int) *agent.Loop {
			return &agent.Loop{Spec: childSpec, Provider: scripted.New(scripted.Fixture{}),
				Exec: exec, Store: cs, Clock: clk, SessionID: session}
		},
	}
	res, err := d.RunSeries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "child_failed" {
		t.Fatalf("res = %+v, want child_failed after retry exhaustion", res)
	}
	for _, sub := range []string{"iter-1-a1", "iter-1-a2"} {
		if _, err := store.ReadEvents(filepath.Join(dStore.Dir(), "sub", sub)); err != nil {
			t.Errorf("attempt journal %s missing: %v", sub, err)
		}
	}
}

// INC-80.2b②: self_paced in the merged stream — iteration 1 declares
// schedule_next{1m} (the runner idles on it), iteration 2's approved
// finish_series ends the series satisfied; the journal is a session stream.
func TestSeriesSelfPaced(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	d, dStore := selfPacedHarness(t, &driver.DriverSpec{
		Name: "series", Schedule: driver.ScheduleSelfPaced, Prompt: "keep up", MaxIterations: 5,
	}, clk, func(call int) scripted.Fixture {
		if call == 1 {
			return paceFixture("1m")
		}
		return finishFixture()
	})
	d.Approvals = stubResolver{approve: true}

	resCh := make(chan driver.Result, 1)
	go func() {
		res, rerr := d.RunSeries(context.Background())
		if rerr != nil {
			t.Errorf("series run: %v", rerr)
		}
		resCh <- res
	}()

	waitIdle(t, clk) // idle on the declared 1m pace
	clk.Advance(time.Minute)

	res := <-resCh
	if res.Reason != "satisfied" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want satisfied at 2 (approved finish_series)", res)
	}
	events, err := store.ReadEvents(dStore.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Type != event.TypeSessionStarted {
		t.Fatalf("head = %s, want session_started", events[0].Type)
	}
	ss, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if ss.Series == nil || !ss.Series.Ended || ss.Series.EndReason != "satisfied" ||
		len(ss.Series.Iterations) != 2 {
		t.Fatalf("series fold = %+v", ss.Series)
	}
}

// self_paced no-intent default (finish): a child with no declaration ends
// the merged-stream series satisfied after its iteration.
func TestSeriesSelfPacedNoIntentFinish(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	d, _ := selfPacedHarness(t, &driver.DriverSpec{
		Name: "series", Schedule: driver.ScheduleSelfPaced, Prompt: "one shot", MaxIterations: 5,
	}, clk, func(int) scripted.Fixture {
		return scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "did the pass"}, {Finish: "end_turn"}}},
		}}
	})
	res, err := d.RunSeries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.Iterations != 1 {
		t.Fatalf("res = %+v, want satisfied at 1 (on_no_intent=finish)", res)
	}
}

// self_paced denied finish: the human gate rejects the claim — the series
// continues at floor pace instead of ending.
func TestSeriesSelfPacedFinishDenied(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	d, _ := selfPacedHarness(t, &driver.DriverSpec{
		Name: "series", Schedule: driver.ScheduleSelfPaced, Prompt: "keep going",
		MaxIterations: 2, PaceMin: "30s",
	}, clk, func(call int) scripted.Fixture {
		return finishFixture()
	})
	d.Approvals = stubResolver{approve: false}

	resCh := make(chan driver.Result, 1)
	go func() {
		res, rerr := d.RunSeries(context.Background())
		if rerr != nil {
			t.Errorf("series run: %v", rerr)
		}
		resCh <- res
	}()
	waitIdle(t, clk) // denied finish → floor pace idle
	clk.Advance(30 * time.Second)
	res := <-resCh
	// Both iterations claimed finish and were denied; the bounded series
	// ends at max_iterations, never satisfied.
	if res.Reason != "max_iterations" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want max_iterations at 2 (denied finish keeps going)", res)
	}
}

// INC-80.2b③: best-of-N in the merged stream — two attempts from one base
// snapshot (pinned on SeriesStarted), each judged in its own worktree; the
// selection rides SeriesEnded.BestIter and the fold applies it; the main
// workspace is never touched.
func TestSeriesParallelBestOfN(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "bash",
				Args: map[string]any{"command": "echo wrong > answer.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "bash",
				Args: map[string]any{"command": "echo right > answer.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	d, dStore, root := parallelHarness(t, &driver.DriverSpec{
		Name: "pick", Schedule: driver.ScheduleParallel, N: 2,
		Prompt: "write the right answer",
		Verifiers: []driver.VerifierSpec{{Kind: driver.VerifierCommand,
			Command: "test -f base.txt && grep -qx right answer.txt"}},
	}, fix)

	res, err := d.RunSeries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.BestIter != 2 || res.Iterations != 2 {
		t.Fatalf("res = %+v, want satisfied best=2 of 2", res)
	}
	// Isolation: per-attempt answers, base materialized, main untouched.
	for n, want := range map[int]string{1: "wrong", 2: "right"} {
		wt := filepath.Join(dStore.Dir(), "wt", "att-"+string(rune('0'+n)))
		if got, rerr := os.ReadFile(filepath.Join(wt, "answer.txt")); rerr != nil || strings.TrimSpace(string(got)) != want {
			t.Errorf("attempt %d answer = %q err=%v, want %s", n, got, rerr, want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "answer.txt")); !os.IsNotExist(err) {
		t.Error("main workspace was touched by an attempt")
	}
	events, err := store.ReadEvents(dStore.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Type != event.TypeSessionStarted {
		t.Fatalf("head = %s, want session_started", events[0].Type)
	}
	ss, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	sr := ss.Series
	if sr == nil || sr.Kind != "best_of_n" || sr.BaseRef == "" {
		t.Fatalf("series fold = %+v, want best_of_n with a pinned base ref", sr)
	}
	// The fold applies the terminal's selection: attempt 2 passed, and the
	// authority is SeriesEnded.BestIter (not the max-score default).
	if !sr.Ended || sr.EndReason != "satisfied" || sr.BestIter != 2 {
		t.Fatalf("series terminal = ended=%v reason=%q best=%d, want satisfied best=2",
			sr.Ended, sr.EndReason, sr.BestIter)
	}
}

// No attempt passes: the merged-stream round ends stalled; every attempt
// journaled, the best score's attempt is the recorded BestIter.
func TestSeriesParallelAllFailStalls(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "pass 1"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "pass 2"}, {Finish: "end_turn"}}},
	}}
	d, _, _ := parallelHarness(t, &driver.DriverSpec{
		Name: "never", Schedule: driver.ScheduleParallel, N: 2,
		Prompt: "try", Verifiers: []driver.VerifierSpec{{
			Kind: driver.VerifierCommand, Command: "false"}},
	}, fix)
	res, err := d.RunSeries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "stalled" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want stalled after 2 attempts", res)
	}
}

// A worktree lost across a restart fails its attempt instead of judging a
// rolled-back tree (S7 出口 review P1, merged-stream mirror).
func TestSeriesParallelWorktreeLostFailsAttempt(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		// Only attempt 2's script exists — attempt 1 must NOT reach the
		// provider (it is failed on the worktree-lost guard).
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "bash",
				Args: map[string]any{"command": "echo right > answer.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	d, dStore, _ := parallelHarness(t, &driver.DriverSpec{
		Name: "pick", Schedule: driver.ScheduleParallel, N: 2,
		Prompt: "write the right answer",
		Verifiers: []driver.VerifierSpec{{Kind: driver.VerifierCommand,
			Command: "grep -qx right answer.txt"}},
	}, fix)

	// Crash aftermath: the series opened with a pinned base and attempt 1
	// left a child journal, but its worktree is gone.
	ref, err := d.Snapshots.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seed := func(typ string, payload any) {
		t.Helper()
		env, eerr := event.New(typ, payload)
		if eerr != nil {
			t.Fatal(eerr)
		}
		if _, aerr := dStore.Append(env); aerr != nil {
			t.Fatal(aerr)
		}
	}
	seed(event.TypeSessionStarted, &event.SessionStarted{SpecName: "pick",
		SubStateVersions: state.SubStateVersions()})
	seed(event.TypeSeriesStarted, &event.SeriesStarted{SeriesID: "drv-1",
		Kind: "best_of_n", MaxIterations: 2, Source: "user", BaseRef: ref})
	childStore, err := store.OpenEventStore(filepath.Join(dStore.Dir(), "sub", "att-1-a1"))
	if err != nil {
		t.Fatal(err)
	}
	env, _ := event.New(event.TypeSessionStarted, &event.SessionStarted{SpecName: "worker"})
	if _, err := childStore.Append(env); err != nil {
		t.Fatal(err)
	}
	_ = childStore.Close()

	res, err := d.ResumeSeries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.BestIter != 2 {
		t.Fatalf("res = %+v, want attempt 2 to win after attempt 1 fails on the guard", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	ss, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	var lostDetail bool
	for _, it := range ss.Series.Iterations {
		if it.N == 1 && strings.Contains(it.Verdict.Detail, "worktree lost") {
			lostDetail = true
		}
	}
	if !lostDetail {
		t.Fatalf("attempt 1 not failed as worktree-lost: %+v", ss.Series.Iterations)
	}
}

// INC-80 安全 review P1: the merged-stream appender blanket-redacts EVERY
// journaled payload — a credential value pasted into the prompt must never
// reach the series journal (SessionStarted.Prompt and SpawnRequested.Prompt
// were the exposed carriers).
func TestSeriesJournalRedactsCredentialInPrompt(t *testing.T) {
	t.Setenv("LEAKY_API_KEY", "supersecretvalue123")
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Prompt: "use key supersecretvalue123 to fetch", MaxIterations: 1,
		Verifiers: []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "true"}},
	})
	if _, err := d.RunSeries(context.Background()); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(dStore.Dir())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if strings.Contains(string(e.Payload), "supersecretvalue123") {
			t.Fatalf("credential value journaled in %s: %s", e.Type, e.Payload)
		}
	}
}

// INC-80 review P1-1: a hard crash mid-iteration (child spawned, no
// SeriesIteration yet) must RESUME that iteration — its cadence slot was
// consumed at spawn — never write it off as an overlap-skip. The orphaned
// child's completed work and spend settle into the series.
func TestSeriesIntervalCrashMidIterationResumesInFlightChild(t *testing.T) {
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	dStore, err := store.OpenEventStore(filepath.Join(root, "series"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dStore.Close() })
	clk := clock.NewFake(time.Date(2026, 7, 19, 0, 10, 0, 0, time.UTC))
	childSpec := &agent.AgentSpec{
		Name: "worker", Model: agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt: "tick", MaxGenerationSteps: 5,
	}
	// Pre-crash journal: series opened (interval 1m, overlap=skip default),
	// iteration 1 spawned; the crash hit before its SeriesIteration.
	seed := func(typ string, payload any) {
		t.Helper()
		env, eerr := event.New(typ, payload)
		if eerr != nil {
			t.Fatal(eerr)
		}
		if _, aerr := dStore.Append(env); aerr != nil {
			t.Fatal(aerr)
		}
	}
	seed(event.TypeSessionStarted, &event.SessionStarted{SpecName: "loop",
		SubStateVersions: state.SubStateVersions()})
	seed(event.TypeSeriesStarted, &event.SeriesStarted{SeriesID: "ser-1",
		Kind: driver.ScheduleInterval, MaxIterations: 2, Source: "user"})
	seed(event.TypeSpawnRequested, &event.SpawnRequested{CallID: "iter-1",
		Agent: "worker", ChildSession: "ser-1-sub-iter-1-a1", Depth: 1})
	// The orphaned child: run it to quiescence exactly as the pre-crash
	// runner would have, so the journal is a real settled child.
	{
		cr, err := agent.OpenChildRun(filepath.Join(dStore.Dir(), "sub", "iter-1-a1"))
		if err != nil {
			t.Fatal(err)
		}
		fix := scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "crashed-iteration work"},
				{Usage: &scripted.UsageEvent{InputTokens: 100, OutputTokens: 50}},
				{Finish: "end_turn"}}},
		}}
		child := &agent.Loop{Spec: childSpec, Provider: scripted.New(fix),
			Exec: &tool.Executor{WS: ws}, Store: cr.Store(), Clock: clk,
			SessionID: "ser-1-sub-iter-1-a1"}
		if _, _, err := cr.Run(context.Background(), child, "tick"); err != nil {
			t.Fatal(err)
		}
		cr.Close()
	}

	calls := 0
	d := &driver.Driver{
		Spec: &driver.DriverSpec{Name: "loop", Schedule: driver.ScheduleInterval,
			Interval: "1m", Prompt: "tick", MaxIterations: 2, Agent: childSpec},
		Store: dStore, Clock: clk, DriverID: "ser-1", Exec: &tool.Executor{WS: ws},
		NewChild: func(cs *store.EventStore, session string, iter, budget int) *agent.Loop {
			calls++
			fix := scripted.Fixture{Steps: []scripted.Step{
				{Respond: []scripted.Event{{Text: "iteration 2"}, {Finish: "end_turn"}}},
			}}
			return &agent.Loop{Spec: childSpec, Provider: scripted.New(fix),
				Exec: &tool.Executor{WS: ws}, Store: cs, Clock: clk, SessionID: session}
		},
	}
	resCh := make(chan driver.Result, 1)
	go func() {
		res, rerr := d.ResumeSeries(context.Background())
		if rerr != nil {
			t.Errorf("resume: %v", rerr)
		}
		resCh <- res
	}()
	waitIdle(t, clk) // iteration 2 waits on the next interval slot
	clk.Advance(time.Minute)
	res := <-resCh
	if res.Reason != "max_iterations" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want the series to finish both iterations", res)
	}
	if calls != 1 {
		t.Fatalf("fresh children built = %d, want 1 (iteration 1 settles from its journal)", calls)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	ss, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	it1 := ss.Series.Iterations[0]
	if it1.N != 1 || it1.Skipped || it1.Reason != "completed" {
		t.Fatalf("iteration 1 = %+v, want the crashed iteration RESUMED (not skipped)", it1)
	}
	if it1.Usage.Billed() == 0 {
		t.Fatal("iteration 1 spend vanished — the orphaned child's usage must settle")
	}
}
