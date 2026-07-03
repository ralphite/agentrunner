package state

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
)

func env(t *testing.T, typ string, payload any) event.Envelope {
	t.Helper()
	e, err := event.New(typ, payload)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// A representative full run: start → input → turn → LLM activity →
// assistant message with a tool call → tool activity → result → end.
func runEvents(t *testing.T) []event.Envelope {
	t.Helper()
	usage := &provider.Usage{InputTokens: 10, OutputTokens: 5}
	asst := provider.Message{Role: provider.RoleAssistant, Parts: []provider.Part{
		{Kind: provider.PartToolCall, CallID: "call_1_0", ToolName: "read_file",
			Args: json.RawMessage(`{"path":"a.go"}`)},
	}}
	events := []event.Envelope{
		env(t, event.TypeRunStarted, &event.RunStarted{SpecName: "hello", Model: "m",
			Task: "fix", Version: "dev", SubStateVersions: SubStateVersions()}),
		env(t, event.TypeInputReceived, &event.InputReceived{Text: "fix", Source: "cli"}),
		env(t, event.TypeTurnStarted, &event.TurnStarted{Turn: 1}),
		env(t, event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: "llm-t1", Kind: event.KindLLM, Name: "complete", Attempt: 1}),
		env(t, event.TypeActivityCompleted, &event.ActivityCompleted{
			ActivityID: "llm-t1", Usage: usage}),
		env(t, event.TypeAssistantMessage, &event.AssistantMessage{Turn: 1, Message: asst}),
		env(t, event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: "tool-call_1_0", Kind: event.KindTool, Name: "read_file",
			CallID: "call_1_0", Attempt: 1}),
		env(t, event.TypeActivityCompleted, &event.ActivityCompleted{
			ActivityID: "tool-call_1_0", Result: json.RawMessage(`{"content":"pkg"}`)}),
		env(t, event.TypeRunEnded, &event.RunEnded{Reason: "completed", Turns: 1,
			Usage: *usage}),
	}
	for i := range events {
		events[i].Seq = int64(i + 1)
		events[i].ID = event.EventID(int64(i + 1))
	}
	return events
}

func TestFoldFullRun(t *testing.T) {
	s, err := Fold(runEvents(t))
	if err != nil {
		t.Fatal(err)
	}
	if s.Run.Status != StatusEnded || s.Run.Reason != "completed" || s.Run.Turn != 1 {
		t.Errorf("run = %+v", s.Run)
	}
	if s.Run.Usage.InputTokens != 10 || s.Run.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v", s.Run.Usage)
	}
	if len(s.Conversation.Messages) != 2 {
		t.Fatalf("messages = %d, want user + assistant", len(s.Conversation.Messages))
	}
	tr, ok := s.Conversation.ToolResults["call_1_0"]
	if !ok || tr.IsError || string(tr.Result) != `{"content":"pkg"}` {
		t.Errorf("tool result = %+v (ok=%v)", tr, ok)
	}
	if len(s.Activities) != 0 {
		t.Errorf("in-flight not drained: %+v", s.Activities)
	}
}

// fold(all) == Apply-tail-onto-fold(prefix) — the snapshot-resume
// equivalence property, checked at every split point.
func TestFoldSnapshotEquivalence(t *testing.T) {
	events := runEvents(t)
	want, err := Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	for k := 0; k <= len(events); k++ {
		got, err := Fold(events[:k])
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range events[k:] {
			if got, err = Apply(got, e); err != nil {
				t.Fatal(err)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("split at %d diverges:\n got %+v\nwant %+v", k, got, want)
		}
	}
}

// Apply must not mutate its input state.
func TestApplyIsPure(t *testing.T) {
	events := runEvents(t)
	s, err := Fold(events[:6]) // mid-run, non-empty containers
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events[6:] {
		if _, err := Apply(s, e); err != nil {
			t.Fatal(err)
		}
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("input state mutated:\nbefore %s\nafter  %s", before, after)
	}
}

func TestInFlightIsInDoubtSignal(t *testing.T) {
	events := runEvents(t)[:7] // ends right after tool ActivityStarted
	s, err := Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	started, ok := s.Activities["tool-call_1_0"]
	if !ok || started.CallID != "call_1_0" {
		t.Fatalf("in-flight = %+v", s.Activities)
	}
}

func TestFailedAndCancelledDrainInFlight(t *testing.T) {
	terminals := []event.Envelope{}
	for i, terminal := range []any{
		&event.ActivityFailed{ActivityID: "a", Attempt: 1,
			Error: event.ErrorInfo{Class: "timeout", Retryable: true}},
		&event.ActivityCancelled{ActivityID: "a"},
	} {
		typ := event.TypeActivityFailed
		if i == 1 {
			typ = event.TypeActivityCancelled
		}
		terminals = append(terminals, env(t, typ, terminal))
	}
	for _, terminal := range terminals {
		s := New()
		var err error
		s, err = Apply(s, env(t, event.TypeActivityStarted,
			&event.ActivityStarted{ActivityID: "a", Kind: event.KindTool, Attempt: 1}))
		if err != nil {
			t.Fatal(err)
		}
		if s, err = Apply(s, terminal); err != nil {
			t.Fatal(err)
		}
		if len(s.Activities) != 0 {
			t.Errorf("%s did not drain in-flight: %+v", terminal.Type, s.Activities)
		}
	}
}

func TestWaitingTransitions(t *testing.T) {
	s := New()
	var err error
	entered := env(t, event.TypeWaitingEntered,
		&event.WaitingEntered{Kind: event.WaitApproval, Detail: json.RawMessage(`{"call_id":"c"}`)})
	entered.Seq = 42
	if s, err = Apply(s, entered); err != nil {
		t.Fatal(err)
	}
	if s.Waiting == nil || s.Waiting.Kind != event.WaitApproval || s.Waiting.Since != 42 {
		t.Fatalf("waiting = %+v", s.Waiting)
	}
	if s.Run.Status != StatusWaiting {
		t.Errorf("status = %q", s.Run.Status)
	}
	if s, err = Apply(s, env(t, event.TypeWaitingResolved,
		&event.WaitingResolved{Kind: event.WaitApproval, Resolution: "approved"})); err != nil {
		t.Fatal(err)
	}
	if s.Waiting != nil || s.Run.Status != StatusRunning {
		t.Fatalf("after resolve: waiting=%+v status=%q", s.Waiting, s.Run.Status)
	}
}

func TestTimersPendingSet(t *testing.T) {
	s := New()
	var err error
	if s, err = Apply(s, env(t, event.TypeTimerSet,
		&event.TimerSet{TimerID: "tm-1", Purpose: "activity_timeout"})); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Timers["tm-1"]; !ok {
		t.Fatalf("timers = %+v", s.Timers)
	}
	if s, err = Apply(s, env(t, event.TypeTimerFired, &event.TimerFired{TimerID: "tm-1"})); err != nil {
		t.Fatal(err)
	}
	if len(s.Timers) != 0 {
		t.Fatalf("timers after fire = %+v", s.Timers)
	}
}

// Every type in event.Registry must have a fold case: feed each sample-free
// zero payload through Apply and require no UnhandledEventError.
func TestApplyCoversRegistry(t *testing.T) {
	for typ, mk := range event.Registry {
		s := New()
		if _, err := Apply(s, env(t, typ, mk())); err != nil {
			var unhandled *UnhandledEventError
			if errors.As(err, &unhandled) {
				t.Errorf("registered type %q has no fold case", typ)
			} else {
				t.Errorf("apply(%q) = %v", typ, err)
			}
		}
	}
}

func TestUnknownEventTypeIsError(t *testing.T) {
	_, err := Apply(New(), event.Envelope{Type: "from_the_future", Payload: json.RawMessage(`{}`)})
	if err == nil {
		t.Fatal("unknown type must be a fold error")
	}
}

// A cancelled tool call must resolve its call_id: decide() re-running a
// provably half-executed command after a post-cancel crash is the bug.
func TestCancelledToolCallResolvesResult(t *testing.T) {
	s := New()
	var err error
	if s, err = Apply(s, env(t, event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: "tool-call_1_0", Kind: event.KindTool, Name: "bash",
		CallID: "call_1_0", Attempt: 1})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeActivityCancelled, &event.ActivityCancelled{
		ActivityID: "tool-call_1_0", PartialOutput: "partial stdout"})); err != nil {
		t.Fatal(err)
	}
	tr, ok := s.Conversation.ToolResults["call_1_0"]
	if !ok || !tr.IsError {
		t.Fatalf("cancelled call not resolved: %+v (ok=%v)", tr, ok)
	}
	for _, want := range []string{"[interrupted by user]", "partial stdout"} {
		if !strings.Contains(string(tr.Result), want) {
			t.Errorf("result %s missing %q", tr.Result, want)
		}
	}
	if len(s.Activities) != 0 {
		t.Errorf("in-flight not drained: %+v", s.Activities)
	}
}
