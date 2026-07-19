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
		env(t, event.TypeSessionStarted, &event.SessionStarted{SpecName: "hello", Model: "m",
			Prompt: "fix", Version: "dev", SubStateVersions: SubStateVersions()}),
		env(t, event.TypeInputReceived, &event.InputReceived{Text: "fix", Source: "cli"}),
		env(t, event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}),
		env(t, event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: "llm-t1", Kind: event.KindLLM, Name: "complete", Attempt: 1}),
		env(t, event.TypeActivityCompleted, &event.ActivityCompleted{
			ActivityID: "llm-t1", Usage: usage}),
		env(t, event.TypeAssistantMessage, &event.AssistantMessage{GenStep: 1, Message: asst}),
		env(t, event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: "tool-call_1_0", Kind: event.KindTool, Name: "read_file",
			CallID: "call_1_0", Attempt: 1}),
		env(t, event.TypeActivityCompleted, &event.ActivityCompleted{
			ActivityID: "tool-call_1_0", Result: json.RawMessage(`{"content":"pkg"}`)}),
		env(t, event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 2}),
		env(t, event.TypeAssistantMessage, &event.AssistantMessage{GenStep: 2,
			Message: provider.Message{Role: provider.RoleAssistant,
				Parts: []provider.Part{{Kind: provider.PartText, Text: "done"}}}}),
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
	if quiescent, reason := Quiescence(s); !quiescent || reason != "completed" || s.Session.GenStep != 2 {
		t.Errorf("run = %+v (quiescent=%v reason=%q)", s.Session, quiescent, reason)
	}
	if s.Session.Usage.InputTokens != 10 || s.Session.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v", s.Session.Usage)
	}
	if len(s.Conversation.Messages) != 3 {
		t.Fatalf("messages = %d, want user + 2 assistants", len(s.Conversation.Messages))
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

// TestGoalClaimFold covers the goal_complete claim lifecycle (INC-10): a
// claim folds into the goal, a GoalUpdated voids it, a GoalCheckpoint
// consumes it — and every goal fold is copy-on-write (Apply purity).
func TestGoalClaimFold(t *testing.T) {
	s := State{}
	var err error
	if s, err = Apply(s, env(t, event.TypeGoalAttached, &event.GoalAttached{
		GoalID: "goal", Goal: "x", Budget: event.GoalBudget{MaxChecks: 5}, Source: "user"})); err != nil {
		t.Fatal(err)
	}
	prev := s // shares the *Goal pointer with s until a goal fold copies
	if s, err = Apply(s, env(t, event.TypeGoalCompletionClaimed, &event.GoalCompletionClaimed{
		GoalID: "goal", Summary: "done", Source: "model"})); err != nil {
		t.Fatal(err)
	}
	if !s.Goal.Claimed || s.Goal.ClaimSummary != "done" {
		t.Fatalf("claim did not fold: %+v", s.Goal)
	}
	if prev.Goal.Claimed {
		t.Fatal("Apply mutated the input state's goal (copy-on-write violated)")
	}
	// A mismatched goal id is a harmless orphan (no-op).
	if s2, _ := Apply(s, env(t, event.TypeGoalCompletionClaimed, &event.GoalCompletionClaimed{
		GoalID: "other", Summary: "nope", Source: "model"})); s2.Goal.ClaimSummary != "done" {
		t.Fatal("orphan claim touched the goal")
	}
	// An update voids the pending claim (the objective changed).
	if s, err = Apply(s, env(t, event.TypeGoalUpdated, &event.GoalUpdated{
		GoalID: "goal", Goal: "y", Source: "user"})); err != nil {
		t.Fatal(err)
	}
	if s.Goal.Claimed || s.Goal.ClaimSummary != "" {
		t.Fatalf("update did not void the claim: %+v", s.Goal)
	}
	// A checkpoint consumes a (re-)claim.
	if s, err = Apply(s, env(t, event.TypeGoalCompletionClaimed, &event.GoalCompletionClaimed{
		GoalID: "goal", Summary: "again", Source: "model"})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeGoalCheckpoint, &event.GoalCheckpoint{
		GoalID: "goal", GenStep: 1, Check: 1, Pass: true, Detail: "model-certified: again"})); err != nil {
		t.Fatal(err)
	}
	if s.Goal.Claimed || s.Goal.ClaimSummary != "" {
		t.Fatalf("checkpoint did not consume the claim: %+v", s.Goal)
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
		&event.ActivityFailed{ActivityID: "a", Attempt: 3, Final: true,
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
	if s.Session.Status != StatusWaiting {
		t.Errorf("status = %q", s.Session.Status)
	}
	if s, err = Apply(s, env(t, event.TypeWaitingResolved,
		&event.WaitingResolved{Kind: event.WaitApproval, Resolution: "approved"})); err != nil {
		t.Fatal(err)
	}
	if s.Waiting != nil || s.Session.Status != StatusRunning {
		t.Fatalf("after resolve: waiting=%+v status=%q", s.Waiting, s.Session.Status)
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
		if event.DriverStream[typ] || event.NotifierStream[typ] {
			continue // driver/notifier stream events never enter the run fold
		}
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

// INC-52 (HANDA-PARITY #14): the SessionTitled fold projection and its
// source-precedence invariant. auto sets RawTitle only over an empty or a
// prior auto title; manual/fork always win and auto never overrides them.
func TestSessionTitledFoldProjection(t *testing.T) {
	titled := func(title, source string) event.Envelope {
		return env(t, event.TypeSessionTitled, &event.SessionTitled{Title: title, Source: source})
	}
	cases := []struct {
		name       string
		events     []event.Envelope
		wantTitle  string
		wantSource string
	}{
		{
			name:       "auto sets over empty",
			events:     []event.Envelope{titled("Fix the auth boundary", event.TitleSourceAuto)},
			wantTitle:  "Fix the auth boundary",
			wantSource: event.TitleSourceAuto,
		},
		{
			name: "auto never overrides manual",
			events: []event.Envelope{
				titled("My renamed prompt", event.TitleSourceManual),
				titled("An auto guess", event.TitleSourceAuto),
			},
			wantTitle:  "My renamed prompt",
			wantSource: event.TitleSourceManual,
		},
		{
			name: "manual replaces a prior auto",
			events: []event.Envelope{
				titled("An auto guess", event.TitleSourceAuto),
				titled("My renamed prompt", event.TitleSourceManual),
			},
			wantTitle:  "My renamed prompt",
			wantSource: event.TitleSourceManual,
		},
		{
			name: "auto replaces a prior auto (re-title)",
			events: []event.Envelope{
				titled("First guess", event.TitleSourceAuto),
				titled("Better guess", event.TitleSourceAuto),
			},
			wantTitle:  "Better guess",
			wantSource: event.TitleSourceAuto,
		},
		{
			name: "auto never overrides fork",
			events: []event.Envelope{
				titled("Forked title", event.TitleSourceFork),
				titled("An auto guess", event.TitleSourceAuto),
			},
			wantTitle:  "Forked title",
			wantSource: event.TitleSourceFork,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			var err error
			for _, e := range tc.events {
				if s, err = Apply(s, e); err != nil {
					t.Fatal(err)
				}
			}
			if s.Session.RawTitle != tc.wantTitle || s.Session.TitleSource != tc.wantSource {
				t.Fatalf("RawTitle/TitleSource = %q/%q, want %q/%q",
					s.Session.RawTitle, tc.wantSource, tc.wantTitle, tc.wantSource)
			}
		})
	}
}

// A legacy journal predating INC-52 carries no SessionTitled: the projection
// stays empty and the surfaces fall back to the opening prompt's first line.
func TestSessionTitledAbsentFoldsEmpty(t *testing.T) {
	s := New()
	var err error
	if s, err = Apply(s, env(t, event.TypeSessionStarted, &event.SessionStarted{
		SpecName: "hello", Model: "m", Prompt: "make it loud\nand also quiet",
		Version: "dev", SubStateVersions: SubStateVersions()})); err != nil {
		t.Fatal(err)
	}
	if s.Session.RawTitle != "" || s.Session.TitleSource != "" {
		t.Fatalf("legacy journal RawTitle/TitleSource = %q/%q, want empty",
			s.Session.RawTitle, s.Session.TitleSource)
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

// 3.9 + S3 回访项: a NON-final failure keeps the activity in flight (the
// backoff window is in-doubt territory for non-idempotent activities); a
// FINAL tool failure renders as the call's model-visible result.
func TestActivityFailedFinality(t *testing.T) {
	s := New()
	var err error
	if s, err = Apply(s, env(t, event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: "tool-call_1_0", Kind: event.KindTool, Name: "bash",
		CallID: "call_1_0", Attempt: 1})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeActivityFailed, &event.ActivityFailed{
		ActivityID: "tool-call_1_0", Attempt: 1, Final: false,
		Error: event.ErrorInfo{Class: "timeout", Retryable: true}})); err != nil {
		t.Fatal(err)
	}
	if _, inFlight := s.Activities["tool-call_1_0"]; !inFlight {
		t.Fatal("non-final failure must keep the activity in flight")
	}
	if s, err = Apply(s, env(t, event.TypeActivityFailed, &event.ActivityFailed{
		ActivityID: "tool-call_1_0", Attempt: 3, Final: true,
		Error: event.ErrorInfo{Class: "timeout", Message: "killed after 120s"}})); err != nil {
		t.Fatal(err)
	}
	if len(s.Activities) != 0 {
		t.Fatal("final failure must drain in-flight")
	}
	tr, ok := s.Conversation.ToolResults["call_1_0"]
	if !ok || !tr.IsError {
		t.Fatalf("final tool failure not rendered: %+v (ok=%v)", tr, ok)
	}
	for _, want := range []string{"timed out", "killed after 120s", `"class":"timeout"`} {
		if !strings.Contains(string(tr.Result), want) {
			t.Errorf("rendered result %s missing %q", tr.Result, want)
		}
	}
}

func TestFinalLLMFailureMarksSessionFailed(t *testing.T) {
	s := New()
	var err error
	if s, err = Apply(s, env(t, event.TypeSessionStarted, &event.SessionStarted{})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeInputReceived, &event.InputReceived{Text: "go", Source: "cli"})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: "llm-t1", Kind: event.KindLLM, Name: "complete", Attempt: 1,
	})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeActivityFailed, &event.ActivityFailed{
		ActivityID: "llm-t1", Attempt: 1, Final: true,
		Error: event.ErrorInfo{Class: "provider_invalid", Message: "bad model", Retryable: false},
	})); err != nil {
		t.Fatal(err)
	}
	if s.Session.Status != StatusFailed || s.Session.Failure == nil {
		t.Fatalf("session failure = status %q mark %+v", s.Session.Status, s.Session.Failure)
	}
	if q, reason := Quiescence(s); !q || reason != "failed:provider_invalid" {
		t.Fatalf("quiescence = %v %q, want failed:provider_invalid", q, reason)
	}
	if s, err = Apply(s, env(t, event.TypeInputReceived, &event.InputReceived{Text: "again", Source: "cli"})); err != nil {
		t.Fatal(err)
	}
	if q, _ := Quiescence(s); q {
		t.Fatal("new input after failure must make the session runnable")
	}
	if s, err = Apply(s, env(t, event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 2})); err != nil {
		t.Fatal(err)
	}
	if s.Session.Status != StatusRunning || s.Session.Failure != nil {
		t.Fatalf("generation did not clear failure: status %q mark %+v", s.Session.Status, s.Session.Failure)
	}
}

// TestMaintenanceAfterCloseKeepsMark pins the INC-82 verb model: a
// WaitingEntered AFTER a close mark (compact/clear on a closed session —
// maintenance, no new generation) does NOT clear the mark. The session stays
// closed and Quiescence honestly says so. Only real input starting a turn
// (GenerationStarted — the send path, 决策 #30) reopens and clears the mark.
func TestMaintenanceAfterCloseKeepsMark(t *testing.T) {
	s := New()
	var err error
	seq := []struct {
		typ string
		p   any
	}{
		{event.TypeSessionStarted, &event.SessionStarted{}},
		{event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}},
		{event.TypeAssistantMessage, &event.AssistantMessage{GenStep: 1, Message: provider.Message{
			Role: provider.RoleAssistant, Parts: []provider.Part{{Kind: provider.PartText, Text: "hi"}}}}},
		{event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitInput}},
		{event.TypeWaitingResolved, &event.WaitingResolved{Kind: event.WaitInput}},
		{event.TypeSessionClosed, &event.SessionClosed{Reason: "closed", Source: "user"}},
		// Maintenance on a closed session: compact runs, then re-parks.
		{event.TypeContextCompacted, &event.ContextCompacted{Summary: "s", UptoGenStep: 1}},
		{event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitInput}},
	}
	for _, e := range seq {
		if s, err = Apply(s, env(t, e.typ, e.p)); err != nil {
			t.Fatal(err)
		}
	}
	if s.Session.Closed == nil || s.Session.Closed.Reason != "closed" {
		t.Fatalf("maintenance must keep the close mark, got %+v", s.Session.Closed)
	}
	if q, reason := Quiescence(s); !q || reason != "closed" {
		t.Fatalf("quiescence = %v %q, want closed", q, reason)
	}
	// send reopens: real input starts a turn and GenerationStarted clears it.
	if s, err = Apply(s, env(t, event.TypeInputReceived, &event.InputReceived{Text: "hi again", Source: "cli"})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 2})); err != nil {
		t.Fatal(err)
	}
	if s.Session.Closed != nil {
		t.Fatalf("send reopen must clear the close mark, got %+v", s.Session.Closed)
	}
}

func TestSessionClosedOverridesWaitingAndInFlightState(t *testing.T) {
	s := New()
	var err error
	if s, err = Apply(s, env(t, event.TypeSessionStarted, &event.SessionStarted{})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: "tool-call_1_0", Kind: event.KindTool, Name: "bash",
		CallID: "call_1_0", Attempt: 1,
	})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeWaitingEntered, &event.WaitingEntered{
		Kind: event.WaitApproval, Detail: json.RawMessage(`{"approval_id":"apr-eff-tool-call_1_0","effect_id":"eff-tool-call_1_0"}`),
	})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeSessionClosed, &event.SessionClosed{Reason: "killed", Source: "user"})); err != nil {
		t.Fatal(err)
	}
	if s.Waiting != nil || len(s.Activities) != 0 || len(s.Effects.Allowed) != 0 {
		t.Fatalf("closed session retained live state: waiting=%+v activities=%+v effects=%+v", s.Waiting, s.Activities, s.Effects)
	}
	if q, reason := Quiescence(s); !q || reason != "canceled" {
		t.Fatalf("quiescence = %v %q, want canceled", q, reason)
	}
}

func TestGoalSatisfiedClearsGenerationStepTruncation(t *testing.T) {
	s := New()
	var err error
	if s, err = Apply(s, env(t, event.TypeSessionStarted, &event.SessionStarted{})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeGoalAttached, &event.GoalAttached{
		GoalID: "goal", Goal: "done", Budget: event.GoalBudget{MaxChecks: 3},
	})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeLimitExceeded, &event.LimitExceeded{
		Kind: "generation_steps", Limit: 1, Used: 1,
	})); err != nil {
		t.Fatal(err)
	}
	if q, reason := Quiescence(s); !q || reason != "max_generation_steps" {
		t.Fatalf("precondition quiescence = %v %q", q, reason)
	}
	if s, err = Apply(s, env(t, event.TypeGoalAchieved, &event.GoalAchieved{
		GoalID: "goal", Reason: "satisfied", Checks: 1,
	})); err != nil {
		t.Fatal(err)
	}
	if s.Session.TruncatedAtGenStep != 0 || s.Session.TruncatedKind != "" {
		t.Fatalf("truncation not cleared: %+v", s.Session)
	}
	if q, reason := Quiescence(s); !q || reason != "goal_satisfied" {
		t.Fatalf("satisfied goal quiescence = %v %q, want goal_satisfied", q, reason)
	}
}

func TestGoalExhaustionRetainsGoalAndUpdateRearmsIt(t *testing.T) {
	s := New()
	var err error
	s.Session.GenStep = 2
	if s, err = Apply(s, env(t, event.TypeGoalAttached, &event.GoalAttached{
		GoalID: "goal", Goal: "ship it", Budget: event.GoalBudget{MaxChecks: 2},
	})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeGoalCheckpoint, &event.GoalCheckpoint{
		GoalID: "goal", GenStep: 2, Check: 2,
	})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeGoalExhausted, &event.GoalExhausted{
		GoalID: "goal", Reason: "budget", Checks: 2,
	})); err != nil {
		t.Fatal(err)
	}
	if s.Goal == nil || !s.Goal.Exhausted {
		t.Fatalf("exhausted goal was not retained: %+v", s.Goal)
	}
	if q, reason := Quiescence(s); !q || reason != "goal_budget_exhausted" {
		t.Fatalf("exhausted quiescence = %v %q", q, reason)
	}
	budget := event.GoalBudget{MaxChecks: 4}
	if s, err = Apply(s, env(t, event.TypeGoalUpdated, &event.GoalUpdated{
		GoalID: "goal", Budget: &budget,
	})); err != nil {
		t.Fatal(err)
	}
	if s.Goal.Exhausted || s.Goal.CheckpointedGenStep != 0 || s.Session.GoalOutcome != "" {
		t.Fatalf("goal update did not re-arm: goal=%+v session=%+v", s.Goal, s.Session)
	}
}
