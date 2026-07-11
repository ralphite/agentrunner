package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// controlLoopJudge is controlLoop plus an injected llm_judge provider
// (INC-48). judgeFix scripts the judge's verdict turns.
func controlLoopJudge(t *testing.T, fix, judgeFix scripted.Fixture, maxSteps int) (*store.EventStore, chan protocol.UserInput, chan protocol.Control, chan error) {
	t.Helper()
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })
	inbox := make(chan protocol.UserInput, 4)
	controls := make(chan protocol.Control, 4)
	l := &Loop{
		Spec: &AgentSpec{
			Name:               "ctl",
			Model:              ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			Tools:              []string{"bash"},
			MaxGenerationSteps: maxSteps,
		},
		Provider:   scripted.New(fix),
		Judge:      scripted.New(judgeFix),
		Exec:       &tool.Executor{WS: ws},
		Store:      es,
		Clock:      clock.NewFake(time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)),
		SessionID:  "ctl-sess",
		UserInputs: inbox,
		Controls:   controls,
	}
	done := make(chan error, 1)
	go func() { _, e := l.Run(context.Background(), "first question"); done <- e }()
	return es, inbox, controls, done
}

func attachLLMGoal(controls chan protocol.Control) {
	controls <- protocol.Control{Kind: protocol.ControlGoalAttach, Goal: &protocol.GoalControl{
		GoalID: "goal", Goal: "refactor all handlers, behavior unchanged",
		Verifiers: []event.GoalVerifier{{Kind: "llm_judge", Rubric: "The handlers are refactored and behavior is preserved."}},
		Budget:    &event.GoalBudget{MaxChecks: 5},
	}}
}

func judgeActivityCount(t *testing.T, dir string) int {
	t.Helper()
	evs, _ := store.ReadEvents(dir)
	n := 0
	for _, e := range evs {
		if e.Type != event.TypeActivityStarted {
			continue
		}
		dec, err := event.DecodePayload(e)
		if err != nil {
			t.Fatal(err)
		}
		if dec.(*event.ActivityStarted).Name == "verifier:llm_judge" {
			n++
		}
	}
	return n
}

// TestGoalLLMJudgePass: claim → judge passes → GoalAchieved{satisfied}, and
// the judge WAS invoked exactly once.
func TestGoalLLMJudgePass(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "starting"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "working"}, {Finish: "end_turn"}}}, // no claim → miss (no judge call)
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "goal_complete",
				Args: map[string]any{"summary": "all handlers refactored, tests green"}}},
			{Finish: "tool_use"}}},
		{Respond: []scripted.Event{{Text: "declared"}, {Finish: "end_turn"}}},
	}}
	judge := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: `{"pass": true, "reason": "handlers refactored, behavior preserved"}`}, {Finish: "end_turn"}}},
	}}
	es, inbox, controls, done := controlLoopJudge(t, fix, judge, 10)
	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	attachLLMGoal(controls)
	waitForEvent(t, es, event.TypeGoalAchieved, 1)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := judgeActivityCount(t, es.Dir()); n != 1 {
		t.Fatalf("judge activity count = %d, want 1 (claim-gated: only the claimed boundary calls the judge)", n)
	}
	checks := decodeGoalCheckpoints(t, es.Dir())
	// [miss (no claim, no judge), pass (claim + judge)].
	if len(checks) != 2 || checks[0].Pass || !checks[1].Pass {
		t.Fatalf("checkpoints = %+v, want [miss pass]", checks)
	}
	if !strings.Contains(checks[1].Detail, "judge") {
		t.Fatalf("pass detail = %q, want judge attribution", checks[1].Detail)
	}
	if ach := decodeGoalAchieved(t, es.Dir()); ach.Reason != "satisfied" {
		t.Fatalf("GoalAchieved.Reason = %q, want satisfied", ach.Reason)
	}
}

// TestGoalLLMJudgeRejectThenPass: judge rejects the first claim (continuation
// re-injects), then passes the second.
func TestGoalLLMJudgeRejectThenPass(t *testing.T) {
	// Each claim needs its own call ID (as a real provider would issue):
	// a reused ID hits the tool activity's idempotency window and the second
	// goal_complete never re-runs.
	claim := func(id string) scripted.Event {
		return scripted.Event{ToolCall: &scripted.ToolCallEvent{CallID: id, Name: "goal_complete",
			Args: map[string]any{"summary": "done"}}}
	}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "starting"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{claim("c1"), {Finish: "tool_use"}}}, // claim 1 → judge rejects
		{Respond: []scripted.Event{{Text: "reacting to rejection"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{claim("c2"), {Finish: "tool_use"}}}, // claim 2 → judge passes
		{Respond: []scripted.Event{{Text: "done for real"}, {Finish: "end_turn"}}},
	}}
	judge := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: `{"pass": false, "reason": "two handlers still on the old style"}`}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: `{"pass": true, "reason": "now complete"}`}, {Finish: "end_turn"}}},
	}}
	es, inbox, controls, done := controlLoopJudge(t, fix, judge, 12)
	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	attachLLMGoal(controls)
	waitForEvent(t, es, event.TypeGoalAchieved, 1)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := judgeActivityCount(t, es.Dir()); n != 2 {
		t.Fatalf("judge activity count = %d, want 2 (one per claim)", n)
	}
	checks := decodeGoalCheckpoints(t, es.Dir())
	if len(checks) != 2 || checks[0].Pass || !checks[1].Pass {
		t.Fatalf("checkpoints = %+v, want [reject pass]", checks)
	}
	if !strings.Contains(checks[0].Detail, "rejected") {
		t.Fatalf("first check detail = %q, want rejected", checks[0].Detail)
	}
}

// TestGoalLLMJudgeCrashReuse: a crash after the judge's ActivityCompleted but
// before the GoalCheckpoint must reuse the journaled verdict on resume, never
// re-judge. Judge is nil here — a live call would fail closed, so reaching
// GoalAchieved{satisfied} proves the verdict was replayed from the journal
// (and parsed as judge JSON, not as a command exit code — INC-48 MINOR-2).
func TestGoalLLMJudgeCrashReuse(t *testing.T) {
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "s"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })
	verdict := json.RawMessage(`{"pass": true, "reason": "verified before the crash"}`)
	appendSynthetic(t, es, []struct {
		typ     string
		payload any
	}{
		{event.TypeActivityStarted, &event.ActivityStarted{ActivityID: "goal-judge-goal-g2",
			Kind: event.KindLLM, Name: "verifier:llm_judge", Idempotent: true, Attempt: 1}},
		{event.TypeActivityCompleted, &event.ActivityCompleted{ActivityID: "goal-judge-goal-g2",
			Result: verdict}},
	})
	ds := &driveState{s: state.State{Goal: &state.Goal{GoalID: "goal", Goal: "x",
		Verifiers: []event.GoalVerifier{{Kind: "llm_judge", Rubric: "done?"}},
		Budget:    event.GoalBudget{MaxChecks: 5}, Claimed: true, ClaimSummary: "did it"}}}
	ds.s.Session.GenStep = 2
	ds.s.Conversation.Messages = []provider.Message{
		{Role: provider.RoleUser, Parts: []provider.Part{{Kind: provider.PartText, Text: "go"}}},
		{Role: provider.RoleAssistant, Parts: []provider.Part{{Kind: provider.PartText, Text: "done"}}},
	}
	l := &Loop{Store: es, SessionID: "g"} // Judge nil: live judging is impossible
	if err := l.goalResumeCheck(context.Background(), ds, l.appender(ds)); err != nil {
		t.Fatal(err)
	}
	if ach := decodeGoalAchieved(t, es.Dir()); ach.Reason != "satisfied" {
		t.Fatalf("GoalAchieved.Reason = %q, want satisfied (replayed verdict)", ach.Reason)
	}
	if n := judgeActivityCount(t, es.Dir()); n != 1 {
		t.Fatalf("judge activity count = %d, want 1 (the pre-crash one; resume must not re-judge)", n)
	}
	checks := decodeGoalCheckpoints(t, es.Dir())
	if len(checks) != 1 || !checks[0].Pass || !strings.Contains(checks[0].Detail, "verified before the crash") {
		t.Fatalf("checkpoints = %+v, want one pass carrying the replayed reason", checks)
	}
}

// TestGoalLLMJudgeClaimGatedNoCall: a model that never claims must NOT invoke
// the judge (zero LLM cost) and is truncated by max_checks, not judged.
func TestGoalLLMJudgeClaimGatedNoCall(t *testing.T) {
	steps := []scripted.Step{{Respond: []scripted.Event{{Text: "starting"}, {Finish: "end_turn"}}}}
	for i := 0; i < 6; i++ {
		steps = append(steps, scripted.Step{Respond: []scripted.Event{{Text: "still working, no claim"}, {Finish: "end_turn"}}})
	}
	// A judge fixture that would PASS if ever called — it must not be.
	judge := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: `{"pass": true, "reason": "should never run"}`}, {Finish: "end_turn"}}},
	}}
	es, inbox, controls, done := controlLoopJudge(t, scripted.Fixture{Steps: steps}, judge, 20)
	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- protocol.Control{Kind: protocol.ControlGoalAttach, Goal: &protocol.GoalControl{
		GoalID: "goal", Goal: "impossible without a claim",
		Verifiers: []event.GoalVerifier{{Kind: "llm_judge", Rubric: "done?"}},
		Budget:    &event.GoalBudget{MaxChecks: 3},
	}}
	waitForEvent(t, es, event.TypeGoalAchieved, 1)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := judgeActivityCount(t, es.Dir()); n != 0 {
		t.Fatalf("judge activity count = %d, want 0 (claim-gated: no claim, no judge call)", n)
	}
	if ach := decodeGoalAchieved(t, es.Dir()); ach.Reason != "budget" {
		t.Fatalf("GoalAchieved.Reason = %q, want budget (max_checks truncation)", ach.Reason)
	}
}
