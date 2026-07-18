package driver_test

import (
	"context"
	"path/filepath"
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
