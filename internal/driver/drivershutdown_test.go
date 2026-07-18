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
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// loopDriverForCancel wires a 1-minute interval series whose child is a
// text-only turn, mirroring TestDriverLoopIntervalCadence, and returns the
// driver plus its store and fake clock.
func loopDriverForCancel(t *testing.T, schedule string) (*driver.Driver, *store.EventStore, *clock.Fake) {
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
		Name: "worker", Model: agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt: "tick", MaxGenerationSteps: 5,
	}
	clk := clock.NewFake(time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC))
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "tick"}, {Finish: "end_turn"}}},
	}}
	spec := &driver.DriverSpec{
		Name: "loop", Schedule: schedule, Interval: "1m",
		Prompt: "tick", MaxIterations: 5, Agent: childSpec,
	}
	if schedule == driver.ScheduleImmediate {
		// Goal mode refuses to start without a verifier.
		spec.Verifiers = []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "true"}}
	}
	d := &driver.Driver{
		Spec: spec,
		Store: dStore, Clock: clk, DriverID: "drv-shutdown", Exec: exec,
		NewChild: func(cs *store.EventStore, session string, iter, budget int) *agent.Loop {
			return &agent.Loop{Spec: childSpec, Provider: scripted.New(fix),
				Exec: exec, Store: cs, Clock: clk, SessionID: session}
		},
	}
	return d, dStore, clk
}

func lastEventType(t *testing.T, dStore *store.EventStore) string {
	t.Helper()
	events, err := store.ReadEvents(dStore.Dir())
	if err != nil || len(events) == 0 {
		t.Fatalf("read events: %v (%d)", err, len(events))
	}
	return events[len(events)-1].Type
}

// INC-72 (G22b): a graceful host shutdown ends a loop-mode series WITHOUT a
// terminal — the journal keeps the crash shape, so the boot sweep's
// scanDriveSessions still sees a running series and re-hosts it.
func TestDriverShutdownCutLeavesNoTerminal(t *testing.T) {
	d, dStore, clk := loopDriverForCancel(t, driver.ScheduleInterval)
	ctx, cancel := context.WithCancelCause(context.Background())
	resCh := make(chan driver.Result, 1)
	go func() {
		res, rerr := d.Run(ctx)
		if rerr != nil {
			t.Errorf("driver run: %v", rerr)
		}
		resCh <- res
	}()
	waitIdle(t, clk) // iteration 1 done, series waiting for the next tick
	cancel(errs.ErrHostShutdown)
	res := <-resCh
	if res.Reason != "shutdown" {
		t.Fatalf("res = %+v, want reason shutdown", res)
	}
	events, err := store.ReadEvents(dStore.Dir())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Type == event.TypeDriverCompleted {
			t.Fatalf("shutdown wrote a terminal DriverCompleted: %s", e.Payload)
		}
	}
	st, err := driver.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status == driver.StatusEnded {
		t.Fatalf("fold status = %s — the boot sweep would skip this series", st.Status)
	}
}

// A user stop is an explicit end: the stopped terminal still lands and the
// series is over (决策 #30 — DriverCompleted is the drive's end mark).
func TestDriverUserStopStillWritesTerminal(t *testing.T) {
	d, dStore, clk := loopDriverForCancel(t, driver.ScheduleInterval)
	ctx, cancel := context.WithCancelCause(context.Background())
	resCh := make(chan driver.Result, 1)
	go func() {
		res, _ := d.Run(ctx)
		resCh <- res
	}()
	waitIdle(t, clk)
	cancel(errs.ErrSessionStopped)
	if res := <-resCh; res.Reason != "stopped" {
		t.Fatalf("res = %+v, want reason stopped", res)
	}
	if typ := lastEventType(t, dStore); typ != event.TypeDriverCompleted {
		t.Fatalf("last event = %s, want driver_completed", typ)
	}
}

// A bounded (goal/immediate) series keeps its stopped terminal even on host
// shutdown: nothing re-hosts bounded runs, and a terminal-less journal would
// strand them forever (INC-72 记档). MaxIterations 1 makes iteration 1 the
// whole series: the pre-cancelled ctx is seen at the loop top.
func TestDriverGoalShutdownStillWritesTerminal(t *testing.T) {
	d, dStore, _ := loopDriverForCancel(t, driver.ScheduleImmediate)
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errs.ErrHostShutdown) // cancelled before the first iteration
	res, err := d.Run(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Reason != "stopped" {
		t.Fatalf("res = %+v, want reason stopped", res)
	}
	if typ := lastEventType(t, dStore); typ != event.TypeDriverCompleted {
		t.Fatalf("last event = %s, want driver_completed", typ)
	}
}
