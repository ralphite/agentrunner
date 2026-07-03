package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

var timerEpoch = time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)

// A tool-style activity that outlives its timeout: the timer fires
// (TimerFired journaled), the run ctx is canceled with the timeout cause,
// and the model-visible IsError result completes the activity normally.
func TestActivityTimeoutFiresDurableTimer(t *testing.T) {
	m := &memAppend{}
	fake := clock.NewFake(timerEpoch)
	x := testExecutor(m)
	x.Clock = fake

	done := make(chan error, 1)
	go func() {
		done <- x.Do(context.Background(), Activity{
			ID: "tool-call_1_0", Kind: event.KindTool, Name: "bash",
			CallID: "call_1_0", Timeout: 5 * time.Second,
			Run: func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
				<-ctx.Done() // simulates bash blocking until the group is killed
				if errors.Is(context.Cause(ctx), errs.ErrActivityTimeout) {
					return json.RawMessage(`{"timed_out":true}`), nil, true, nil
				}
				return json.RawMessage(`{"canceled":true}`), nil, true, nil
			},
		})
	}()
	for fake.Waiters() == 0 {
		runtime.Gosched()
	}
	fake.Advance(5 * time.Second)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	want := []string{"activity_started", "timer_set", "timer_fired", "activity_completed"}
	if got := m.types(); !equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	var completed event.ActivityCompleted
	_ = json.Unmarshal(m.events[3].Payload, &completed)
	if !completed.IsError || string(completed.Result) != `{"timed_out":true}` {
		t.Errorf("completed = %+v", completed)
	}
}

// An LLM-style activity whose cancellation surfaces as an error gets the
// timeout class (retryable), not canceled.
func TestActivityTimeoutReclassifiesError(t *testing.T) {
	m := &memAppend{}
	fake := clock.NewFake(timerEpoch)
	x := testExecutor(m)
	x.Clock = fake
	x.MaxAttempts = 1

	done := make(chan error, 1)
	go func() {
		done <- x.Do(context.Background(), Activity{
			ID: "llm-t1", Kind: event.KindLLM, Name: "complete", Timeout: 30 * time.Second,
			Run: func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
				<-ctx.Done()
				return nil, nil, false, ctx.Err() // provider surfaces context.Canceled
			},
		})
	}()
	for fake.Waiters() == 0 {
		runtime.Gosched()
	}
	fake.Advance(30 * time.Second)
	err := <-done
	if err == nil || errs.ClassOf(err) != errs.Timeout {
		t.Fatalf("err = %v (class %s), want timeout class", err, errs.ClassOf(err))
	}
	var failed event.ActivityFailed
	_ = json.Unmarshal(m.events[3].Payload, &failed)
	if failed.Error.Class != string(errs.Timeout) || !failed.Error.Retryable {
		t.Errorf("failed = %+v", failed)
	}
}

// Fast completion cancels the pending timer — the fold must not carry a
// stale timer forward.
func TestActivityTimerCancelledOnCompletion(t *testing.T) {
	m := &memAppend{}
	x := testExecutor(m)
	if err := x.Do(context.Background(), Activity{
		ID: "tool-call_1_0", Kind: event.KindTool, Name: "bash",
		CallID: "call_1_0", Timeout: time.Minute,
		Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			return json.RawMessage(`{}`), nil, false, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"activity_started", "timer_set", "timer_cancelled", "activity_completed"}
	if got := m.types(); !equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	// Fold the events: pending timer set must be empty.
	s := state.New()
	for _, e := range m.events {
		var err error
		if s, err = state.Apply(s, e); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.Timers) != 0 {
		t.Errorf("stale timers in fold: %+v", s.Timers)
	}
}

// Crash between TimerSet and TimerFired: the reopened log still knows the
// timer, and the resume sweep fires expired ones immediately.
func TestTimerSurvivesCrashAndFiresOnResume(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	env, err := event.New(event.TypeTimerSet, &event.TimerSet{
		TimerID: "tm-x", FireAt: timerEpoch.Add(time.Hour), Purpose: "activity_timeout:tool-x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := es.Append(env); err != nil {
		t.Fatal(err)
	}
	_ = es.Close() // crash

	// Resume: reopen, fold, sweep.
	es2, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es2.Close() }()
	events, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Timers["tm-x"]; !ok {
		t.Fatal("pending timer lost across crash")
	}

	appendE := func(typ string, payload any) (event.Envelope, error) {
		env, err := event.New(typ, payload)
		if err != nil {
			return env, err
		}
		return es2.Append(env)
	}

	// Before due: stays pending.
	fake := clock.NewFake(timerEpoch.Add(30 * time.Minute))
	future, err := FirePendingTimers(s, fake, appendE)
	if err != nil {
		t.Fatal(err)
	}
	if len(future) != 1 {
		t.Fatalf("future = %+v, want the pending timer", future)
	}

	// Past due: fires now.
	fake.Advance(31 * time.Minute)
	future, err = FirePendingTimers(s, fake, appendE)
	if err != nil {
		t.Fatal(err)
	}
	if len(future) != 0 {
		t.Fatalf("future = %+v, want none", future)
	}
	all, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if last := all[len(all)-1]; last.Type != event.TypeTimerFired {
		t.Fatalf("last event = %s, want timer_fired", last.Type)
	}
}
