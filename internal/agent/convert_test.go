package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
)

var convertEpoch = time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)

// convertHarness runs one convertible activity to its window: the run
// blocks until release (a long command), the fake clock fires the window,
// and the handoff's cancel/outcome are captured for the test to drive.
type convertHarness struct {
	m       *memAppend
	fake    *clock.Fake
	release chan struct{}
	runCtx  chan context.Context
	cancel  context.CancelCauseFunc
	outcome <-chan ConvertOutcome
	handoff int
	doErr   error
}

func runConvert(t *testing.T, batchCtx context.Context, advance bool) *convertHarness {
	t.Helper()
	h := &convertHarness{
		m: &memAppend{}, fake: clock.NewFake(convertEpoch),
		release: make(chan struct{}), runCtx: make(chan context.Context, 1),
	}
	x := testExecutor(h.m)
	x.Clock = h.fake

	done := make(chan error, 1)
	go func() {
		done <- x.Do(batchCtx, Activity{
			ID: "tool-call_1_0", Kind: event.KindTool, Name: "bash",
			CallID: "call_1_0", Timeout: 5 * time.Second,
			Convert: &ConvertSpec{
				Base:   context.Background(),
				Notice: "moved to background",
				HandOff: func(cancel context.CancelCauseFunc, outcome <-chan ConvertOutcome) {
					h.handoff++
					h.cancel, h.outcome = cancel, outcome
				},
			},
			Run: func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
				h.runCtx <- ctx
				select {
				case <-ctx.Done():
					return json.RawMessage(`{"canceled":true}`), nil, true, nil
				case <-h.release:
					return json.RawMessage(`{"ok":true}`), nil, false, nil
				}
			},
		})
	}()
	if advance {
		for h.fake.Waiters() == 0 {
			runtime.Gosched()
		}
		h.fake.Advance(5 * time.Second)
	}
	h.doErr = <-done
	return h
}

// The window firing converts the attempt instead of killing it: Do returns
// ErrBackgrounded with no terminal journaled, the run keeps going, and the
// eventual outcome flows through the handed-off channel. The fold pairs the
// call with a handle placeholder and the completion settles as a message.
func TestTimeoutConvertsToBackground(t *testing.T) {
	h := runConvert(t, context.Background(), true)
	if !errors.Is(h.doErr, ErrBackgrounded) {
		t.Fatalf("Do = %v, want ErrBackgrounded", h.doErr)
	}
	want := []string{"activity_started", "timer_set", "timer_fired", "activity_backgrounded"}
	if got := h.m.types(); !equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if h.handoff != 1 {
		t.Fatalf("handoff called %d times, want 1", h.handoff)
	}

	// The run survived the conversion: its ctx is live until release.
	rc := <-h.runCtx
	if rc.Err() != nil {
		t.Fatal("converted run's ctx canceled — conversion must not kill")
	}
	close(h.release)
	out := <-h.outcome
	if out.Canceled || out.IsError || string(out.Result) != `{"ok":true}` {
		t.Fatalf("outcome = %+v", out)
	}

	// Fold: the placeholder pairs the call and the work is a live handle.
	s := state.New()
	var err error
	for _, e := range h.m.events {
		if s, err = state.Apply(s, e); err != nil {
			t.Fatal(err)
		}
	}
	started, ok := s.Handles["call_1_0"]
	if !ok || !started.Background {
		t.Fatalf("handle missing or not background: %+v", s.Handles)
	}
	tr, ok := s.Conversation.ToolResults["call_1_0"]
	if !ok || !strings.Contains(string(tr.Result), `"status":"running"`) ||
		!strings.Contains(string(tr.Result), "moved to background") {
		t.Fatalf("placeholder = %+v", tr)
	}

	// Settle (what settleBackground journals): handle leaves, the outcome
	// arrives as a user-role message.
	env, err := event.New(event.TypeActivityCompleted, &event.ActivityCompleted{
		ActivityID: "tool-call_1_0", Result: json.RawMessage(`{"ok":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if s, err = state.Apply(s, env); err != nil {
		t.Fatal(err)
	}
	if _, still := s.Handles["call_1_0"]; still {
		t.Fatal("handle not removed at settle")
	}
	last := s.Conversation.Messages[len(s.Conversation.Messages)-1]
	if last.Role != provider.RoleUser ||
		!strings.Contains(last.Parts[0].Text, "[background work call_1_0 completed]") {
		t.Fatalf("outcome message = %+v", last)
	}
	// The placeholder result never changes (Gemini 1:1 pairing).
	if tr2 := s.Conversation.ToolResults["call_1_0"]; string(tr2.Result) != string(tr.Result) {
		t.Fatal("placeholder mutated at settle")
	}
}

// The handed-off cancel is the kill path: it cancels the run and the
// outcome reports Canceled.
func TestConvertedWorkKillDeliversCanceledOutcome(t *testing.T) {
	h := runConvert(t, context.Background(), true)
	if !errors.Is(h.doErr, ErrBackgrounded) {
		t.Fatalf("Do = %v, want ErrBackgrounded", h.doErr)
	}
	rc := <-h.runCtx
	h.cancel(&errs.KilledError{Source: "user"})
	<-rc.Done()
	out := <-h.outcome
	if !out.Canceled {
		t.Fatalf("outcome = %+v, want Canceled", out)
	}
}

// A batch cancellation (steering interrupt) BEFORE the window fires keeps
// the foreground kill semantics: the run dies with the batch, the terminal
// is ActivityCancelled, nothing converts.
func TestConvertBatchCancelBeforeWindowKills(t *testing.T) {
	batch, cancel := context.WithCancel(context.Background())
	cancel() // canceled before the attempt starts: the watcher trips immediately
	h := runConvert(t, batch, false)
	if h.doErr == nil || errs.ClassOf(h.doErr) != errs.Canceled {
		t.Fatalf("Do = %v, want canceled class", h.doErr)
	}
	if h.handoff != 0 {
		t.Fatal("handoff must not run for a batch-canceled foreground attempt")
	}
	types := h.m.types()
	for _, typ := range types {
		if typ == "activity_backgrounded" {
			t.Fatalf("converted despite batch cancel: %v", types)
		}
	}
	if types[len(types)-1] != "activity_cancelled" {
		t.Fatalf("terminal = %v, want activity_cancelled", types)
	}
}

// A journal that cannot record the conversion falls back to the kill: the
// work must not outlive the record of why it is still running.
func TestConvertStoreFailureFallsBackToKill(t *testing.T) {
	m := &memAppend{}
	fake := clock.NewFake(convertEpoch)
	x := testExecutor(m)
	x.Clock = fake
	inner := x.Append
	x.Append = func(typ string, payload any) (event.Envelope, error) {
		if typ == event.TypeActivityBackgrounded {
			return event.Envelope{}, fmt.Errorf("disk full")
		}
		return inner(typ, payload)
	}

	handoffs := 0
	killed := false
	done := make(chan error, 1)
	go func() {
		done <- x.Do(context.Background(), Activity{
			ID: "tool-call_1_0", Kind: event.KindTool, Name: "bash",
			CallID: "call_1_0", Timeout: 5 * time.Second,
			Convert: &ConvertSpec{Base: context.Background(),
				HandOff: func(context.CancelCauseFunc, <-chan ConvertOutcome) { handoffs++ }},
			Run: func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
				<-ctx.Done()
				killed = errors.Is(context.Cause(ctx), errs.ErrActivityTimeout)
				return json.RawMessage(`{"timed_out":true}`), nil, true, nil
			},
		})
	}()
	for fake.Waiters() == 0 {
		runtime.Gosched()
	}
	fake.Advance(5 * time.Second)
	if err := <-done; err == nil || errors.Is(err, ErrBackgrounded) {
		t.Fatalf("Do = %v, want the store failure", err)
	}
	if handoffs != 0 || !killed {
		t.Fatalf("handoffs=%d killed=%v, want 0/true", handoffs, killed)
	}
}

// The fold ignores a conversion with no matching in-flight foreground tool
// activity (crash artifacts, double conversion): the placeholder must never
// pair twice.
func TestFoldBackgroundedGuards(t *testing.T) {
	s := state.New()
	apply := func(typ string, payload any) {
		t.Helper()
		env, err := event.New(typ, payload)
		if err != nil {
			t.Fatal(err)
		}
		if s, err = state.Apply(s, env); err != nil {
			t.Fatal(err)
		}
	}
	// No in-flight activity: no-op.
	apply(event.TypeActivityBackgrounded, &event.ActivityBackgrounded{ActivityID: "tool-x"})
	if len(s.Handles) != 0 || len(s.Conversation.ToolResults) != 0 {
		t.Fatalf("orphan conversion mutated state: %+v", s.Handles)
	}
	// Explicit background launch then a conversion for it: already
	// background, second pairing suppressed.
	apply(event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: "tool-c1", Kind: event.KindTool, Name: "bash",
		CallID: "c1", Attempt: 1, Background: true, Notice: "first",
	})
	before := s.Conversation.ToolResults["c1"]
	apply(event.TypeActivityBackgrounded, &event.ActivityBackgrounded{
		ActivityID: "tool-c1", Notice: "second"})
	if got := s.Conversation.ToolResults["c1"]; string(got.Result) != string(before.Result) {
		t.Fatalf("double pairing mutated the placeholder: %s", got.Result)
	}
}

// toolWindow's decision table: who converts, with which window.
func TestToolWindowResolution(t *testing.T) {
	s := state.New()
	s.Session.CommandTools = []event.CommandToolDef{
		{Name: "lint", TimeoutS: 600},
		{Name: "fmt"},
	}
	call := func(name, args string) provider.ToolCall {
		return provider.ToolCall{Name: name, CallID: "c", Args: json.RawMessage(args)}
	}

	off := &Loop{} // zero value: conversion disabled, legacy kill semantics
	if d, conv := off.toolWindow(s, call("bash", `{"command":"x"}`)); conv || d != executeToolTimeout {
		t.Fatalf("disabled bash = (%v,%v), want (120s,false)", d, conv)
	}

	l := &Loop{ForegroundWindow: 10 * time.Second}
	cases := []struct {
		name, args string
		want       time.Duration
		convert    bool
	}{
		{"bash", `{"command":"x"}`, 10 * time.Second, true},
		{"bash", `{"command":"x","timeout_s":120}`, 120 * time.Second, true},
		{"bash", `{"command":"x","timeout_s":0.2}`, time.Second, true}, // clamp floor
		{"bash", `{"command":"x","timeout_s":7200}`, time.Hour, true},  // clamp ceiling
		{"lint", `{}`, 600 * time.Second, true},                        // manifest window
		{"fmt", `{}`, 10 * time.Second, true},                          // manifest absent → default
		{"glob", `{}`, 0, false},                                       // read class: no timeout
		{"web_fetch", `{}`, executeToolTimeout, false},                 // execute but not convertible
	}
	for _, tc := range cases {
		d, conv := l.toolWindow(s, call(tc.name, tc.args))
		if d != tc.want || conv != tc.convert {
			t.Errorf("toolWindow(%s %s) = (%v,%v), want (%v,%v)",
				tc.name, tc.args, d, conv, tc.want, tc.convert)
		}
	}
}
