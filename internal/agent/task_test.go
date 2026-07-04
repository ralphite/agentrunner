package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// S6.1 e2e: a background bash pairs with a handle immediately (turn 1
// continues without blocking), its result arrives as a user-role message,
// and the model sees it on a later turn.
func TestBackgroundTaskHandleAndOutcome(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		// Turn 1: launch a background task, then keep working (text) — proves
		// the handle paired without blocking on the sleep.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "bg1", Name: "bash",
				Args: map[string]any{"command": "sleep 0.2; echo done-work", "background": true}}},
			{Finish: "tool_use"},
		}},
		// Turn 2: model acknowledges the handle (the task is still running or
		// just finished) and yields with text; the loop then parks on the
		// task (WAITING_TASKS) if it hasn't settled.
		{
			Expect:  scripted.Expect{LastMessageContains: "task_id"},
			Respond: []scripted.Event{{Text: "started it, waiting"}, {Finish: "end_turn"}},
		},
		// Turn 3: the task outcome has arrived as a user message; wrap up.
		{
			Expect:  scripted.Expect{LastMessageContains: "done-work"},
			Respond: []scripted.Event{{Text: "task finished, all done"}, {Finish: "end_turn"}},
		},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.OnRunEnd = "await" // stay alive for the task so its outcome feeds back

	res, err := l.Run(context.Background(), "run a background job")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	// The Started fact is Background; the Completed settles it.
	var sawBgStart, sawComplete bool
	for _, e := range events {
		if e.Type == event.TypeActivityStarted && strings.Contains(string(e.Payload), `"background":true`) {
			sawBgStart = true
		}
		if e.Type == event.TypeActivityCompleted && strings.Contains(string(e.Payload), "tool-bg1") {
			sawComplete = true
		}
	}
	if !sawBgStart || !sawComplete {
		t.Fatalf("bg start=%v complete=%v", sawBgStart, sawComplete)
	}

	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	// The task drained out of the tasks sub-state at the end.
	if len(fold.Run.Env) == 0 && len(fold.Tasks) != 0 {
		t.Errorf("tasks not drained: %+v", fold.Tasks)
	}
	if len(fold.Tasks) != 0 {
		t.Errorf("tasks sub-state not empty at end: %+v", fold.Tasks)
	}
	// The handle paired the call, and the outcome is a user message.
	tr := fold.Conversation.ToolResults["bg1"]
	if !strings.Contains(string(tr.Result), "running") {
		t.Errorf("handle = %s, want running status", tr.Result)
	}
	var sawOutcome bool
	for _, m := range fold.Conversation.Messages {
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "background task bg1 completed") &&
				strings.Contains(p.Text, "done-work") {
				sawOutcome = true
			}
		}
	}
	if !sawOutcome {
		t.Error("task outcome did not arrive as a user-role message")
	}
}

// S6.1: on_run_end default (cancel) — a run ending with a task still
// running cancels it in the epilogue quiesce slot; the log ends clean.
func TestBackgroundTaskCancelledAtRunEnd(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "bg1", Name: "bash",
				Args: map[string]any{"command": "sleep 30; echo never", "background": true}}},
			{Finish: "tool_use"},
		}},
		// The model ends the run immediately — the task is still sleeping.
		{Respond: []scripted.Event{{Text: "not waiting for it"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.OnRunEnd = "" // default = cancel

	res, err := l.Run(context.Background(), "fire and forget")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	// The task was cancelled (quiesce slot) and settled BEFORE run_ended.
	var cancelSeq, endSeq int64
	for _, e := range events {
		if e.Type == event.TypeActivityCancelled && strings.Contains(string(e.Payload), "tool-bg1") {
			cancelSeq = e.Seq
		}
		if e.Type == event.TypeRunEnded {
			endSeq = e.Seq
		}
	}
	if cancelSeq == 0 || endSeq == 0 || cancelSeq > endSeq {
		t.Fatalf("task must be cancelled (seq %d) before run_ended (seq %d)", cancelSeq, endSeq)
	}
	fold, _ := state.Fold(events)
	if len(fold.Tasks) != 0 {
		t.Errorf("tasks not quiesced at end: %+v", fold.Tasks)
	}
}

// S7 还债: the epilogue's await quiesce is BOUNDED by a durable timer — a
// task that outlives await_timeout is cancelled when the timer fires, and
// the whole story (timer_set → timer_fired → activity_cancelled → run_ended)
// is journaled in order.
func TestAwaitQuiesceTimerBound(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "bg1", Name: "bash",
				Args: map[string]any{"command": "sleep 30; echo never", "background": true}}},
			{Finish: "tool_use"},
		}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.OnRunEnd = "await"
	l.Spec.AwaitTimeout = "1m"
	l.Spec.MaxTurns = 1 // forced ending → epilogue quiesce owns the await

	clk := l.Clock.(*clock.Fake)
	resCh := make(chan RunResult, 1)
	go func() {
		res, err := l.Run(context.Background(), "fire and outlive")
		if err != nil {
			t.Errorf("run: %v", err)
		}
		resCh <- res
	}()

	// The quiesce parks on the await timer; advancing past it fires the
	// bound and cancels the straggler.
	deadline := time.Now().Add(5 * time.Second)
	for clk.Waiters() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if clk.Waiters() == 0 {
		t.Fatal("quiesce never armed the await timer")
	}
	clk.Advance(time.Minute)

	select {
	case res := <-resCh:
		if res.Reason != "max_turns" {
			t.Fatalf("res = %+v", res)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not end after the await timer fired")
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	var setSeq, firedSeq, cancelledSeq, endSeq int64
	for _, e := range events {
		switch e.Type {
		case event.TypeTimerSet:
			if strings.Contains(string(e.Payload), "await_quiesce") {
				setSeq = e.Seq
			}
		case event.TypeTimerFired:
			if strings.Contains(string(e.Payload), "tm-await-quiesce") {
				firedSeq = e.Seq
			}
		case event.TypeActivityCancelled:
			if strings.Contains(string(e.Payload), "tool-bg1") {
				cancelledSeq = e.Seq
			}
		case event.TypeRunEnded:
			endSeq = e.Seq
		}
	}
	if setSeq == 0 || firedSeq <= setSeq || cancelledSeq <= firedSeq || endSeq <= cancelledSeq {
		t.Fatalf("order = set %d, fired %d, cancelled %d, end %d — want strictly increasing",
			setSeq, firedSeq, cancelledSeq, endSeq)
	}
}

// S7 还债: a task that finishes INSIDE the await bound cancels the timer —
// the log closes clean, no pending timer survives the run.
func TestAwaitQuiesceTimerCancelledOnFinish(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "bg1", Name: "bash",
				Args: map[string]any{"command": "sleep 0.2; echo done-fast", "background": true}}},
			{Finish: "tool_use"},
		}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.OnRunEnd = "await"
	l.Spec.MaxTurns = 1

	if _, err := l.Run(context.Background(), "quick task"); err != nil {
		t.Fatal(err)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	var sawCancelledTimer bool
	for _, e := range events {
		if e.Type == event.TypeTimerCancelled && strings.Contains(string(e.Payload), "tm-await-quiesce") {
			sawCancelledTimer = true
		}
	}
	if !sawCancelledTimer {
		t.Fatal("the await timer must be cancelled when tasks finish in time")
	}
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if len(fold.Timers) != 0 {
		t.Fatalf("pending timers at end: %+v", fold.Timers)
	}
}

// S6.1: task_kill cancels a running task; the cancellation lands as a
// message and the tasks set empties.
func TestTaskKill(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "bg1", Name: "bash",
				Args: map[string]any{"command": "sleep 30", "background": true}}},
			{Finish: "tool_use"},
		}},
		// Turn 2: kill it.
		{
			Expect: scripted.Expect{LastMessageContains: "task_id"},
			Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "k1", Name: "task_kill",
					Args: map[string]any{"task_id": "bg1"}}},
				{Finish: "tool_use"},
			},
		},
		// Turn 3: the cancellation arrived as a message; done.
		{
			Expect:  scripted.Expect{LastMessageContains: "canceled"},
			Respond: []scripted.Event{{Text: "killed it"}, {Finish: "end_turn"}},
		},
	}}
	l := testLoop(t, fix, t.TempDir())

	res, err := l.Run(context.Background(), "start then kill")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if len(fold.Tasks) != 0 {
		t.Errorf("task not removed after kill: %+v", fold.Tasks)
	}
	// The kill tool paired normally; the cancellation is a user message.
	kr := fold.Conversation.ToolResults["k1"]
	if !strings.Contains(string(kr.Result), "cancelling") {
		t.Errorf("task_kill result = %s", kr.Result)
	}
}

// S6.1: on_run_end=await lets a still-running task FINISH before the run
// ends — its real output settles AND feeds back to the model as a user-role
// input that earns one more turn (S6 review: the outcome must actually
// reach the model, and the park is an explicit journaled waiting state).
func TestBackgroundTaskAwaitAtRunEnd(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "bg1", Name: "bash",
				Args: map[string]any{"command": "sleep 0.2; echo awaited-output", "background": true}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "ending now, but await the task"}, {Finish: "end_turn"}}},
		// The awaited outcome arrived as a user message → one more turn.
		{
			Expect:  scripted.Expect{LastMessageContains: "awaited-output"},
			Respond: []scripted.Event{{Text: "saw the result, done"}, {Finish: "end_turn"}},
		},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.OnRunEnd = "await"

	res, err := l.Run(context.Background(), "await on end")
	if err != nil {
		t.Fatal(err)
	}
	if res.Turns != 3 {
		t.Fatalf("turns = %d, want 3 (the outcome earned a turn)", res.Turns)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	var sawCompleteWithOutput, sawWaitEntered, sawWaitResolved bool
	for _, e := range events {
		if e.Type == event.TypeActivityCompleted && strings.Contains(string(e.Payload), "awaited-output") {
			sawCompleteWithOutput = true
		}
		if e.Type == event.TypeWaitingEntered && strings.Contains(string(e.Payload), `"tasks"`) {
			sawWaitEntered = true
		}
		if e.Type == event.TypeWaitingResolved && strings.Contains(string(e.Payload), "tasks_done") {
			sawWaitResolved = true
		}
	}
	if !sawCompleteWithOutput {
		t.Error("await must let the task finish with real output, not cancel it")
	}
	if !sawWaitEntered || !sawWaitResolved {
		t.Errorf("WAITING_TASKS park must be journaled: entered=%v resolved=%v", sawWaitEntered, sawWaitResolved)
	}
}
