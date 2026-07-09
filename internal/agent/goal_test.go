package agent

import (
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// TestInSessionGoalContinuity is the INC-D1 core proof: an in-session goal
// continues the SAME thread across a verifier miss. Verifier `test -f done.txt`
// misses (file absent) → the feedback is re-injected as a program input → the
// agent creates the file → verifier passes → GoalAchieved. Context continuity
// is proven by a single SessionStarted (no fresh child run) across the miss.
func TestInSessionGoalContinuity(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		// turn 1 — the opening "first question".
		{Respond: []scripted.Event{{Text: "starting"}, {Finish: "end_turn"}}},
		// turn 2 — runs on the attached goal's statement; file not yet made → MISS.
		{Respond: []scripted.Event{{Text: "I have not created the file yet"}, {Finish: "end_turn"}}},
		// turn 3 — runs on the re-injected miss feedback (proves continuation);
		// creates the file.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "b1", Name: "bash", Args: map[string]any{"command": "touch done.txt"}}},
			{Finish: "tool_use"},
		}},
		// turn 3 cont — after the tool result; verifier now PASSES.
		{Respond: []scripted.Event{{Text: "created done.txt"}, {Finish: "end_turn"}}},
	}}
	es, inbox, controls, done := controlLoop(t, fix, 10)

	waitForEvent(t, es, event.TypeAssistantMessage, 1) // turn 1 quiesced (no goal yet)

	controls <- protocol.Control{Kind: protocol.ControlGoalAttach, Goal: &protocol.GoalControl{
		GoalID: "goal", Goal: "create done.txt in the workspace",
		Verifiers: []event.GoalVerifier{{Kind: "command", Command: "test -f done.txt"}},
		Budget:    &event.GoalBudget{MaxChecks: 5},
	}}

	waitForEvent(t, es, event.TypeGoalAchieved, 1)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// Two checkpoints — the first missed, the second passed.
	checks := decodeGoalCheckpoints(t, es.Dir())
	if len(checks) != 2 {
		t.Fatalf("checkpoints = %d, want 2: %+v", len(checks), checks)
	}
	if checks[0].Pass || !checks[1].Pass {
		t.Fatalf("checkpoint verdicts = [%v %v], want [miss pass]", checks[0].Pass, checks[1].Pass)
	}
	if n := countEvents(t, es.Dir(), event.TypeGoalAttached); n != 1 {
		t.Fatalf("GoalAttached = %d, want 1", n)
	}
	// The whole point: exactly ONE session — the context continued across the
	// miss, it was NOT a fresh child run.
	if n := countEvents(t, es.Dir(), event.TypeSessionStarted); n != 1 {
		t.Fatalf("SessionStarted = %d, want 1 (context must continue, not restart)", n)
	}
	if ach := decodeGoalAchieved(t, es.Dir()); ach.Reason != "satisfied" {
		t.Fatalf("GoalAchieved.Reason = %q, want satisfied", ach.Reason)
	}
}

// TestInSessionGoalBudgetTruncation: a verifier that never passes stops at the
// budget (max_checks) with GoalAchieved{budget} — a visible truncation — and
// does NOT re-inject forever.
func TestInSessionGoalBudgetTruncation(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "turn 1"}, {Finish: "end_turn"}}},
		// Every subsequent turn just says it's trying; the file is never made,
		// so the verifier misses every check up to the budget.
		{Respond: []scripted.Event{{Text: "still trying 1"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "still trying 2"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "still trying 3"}, {Finish: "end_turn"}}},
	}}
	es, inbox, controls, done := controlLoop(t, fix, 10)
	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- protocol.Control{Kind: protocol.ControlGoalAttach, Goal: &protocol.GoalControl{
		GoalID: "goal", Goal: "make a file that never gets made",
		Verifiers: []event.GoalVerifier{{Kind: "command", Command: "test -f never.txt"}},
		Budget:    &event.GoalBudget{MaxChecks: 2},
	}}
	waitForEvent(t, es, event.TypeGoalAchieved, 1)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	checks := decodeGoalCheckpoints(t, es.Dir())
	if len(checks) != 2 {
		t.Fatalf("checkpoints = %d, want 2 (budget)", len(checks))
	}
	for _, c := range checks {
		if c.Pass {
			t.Fatal("a checkpoint passed; the verifier should never pass")
		}
	}
	if ach := decodeGoalAchieved(t, es.Dir()); ach.Reason != "budget" {
		t.Fatalf("GoalAchieved.Reason = %q, want budget", ach.Reason)
	}
}

// TestInSessionGoalPauseCancel: a paused goal does not verify; a cancel detaches
// it (no achievement).
func TestInSessionGoalPauseCancel(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "turn 1"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "on the goal"}, {Finish: "end_turn"}}},
	}}
	es, inbox, controls, done := controlLoop(t, fix, 10)
	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- protocol.Control{Kind: protocol.ControlGoalAttach, Goal: &protocol.GoalControl{
		GoalID: "goal", Goal: "a goal we'll pause",
		Verifiers: []event.GoalVerifier{{Kind: "command", Command: "test -f never.txt"}},
		Budget:    &event.GoalBudget{MaxChecks: 5},
	}}
	waitForEvent(t, es, event.TypeGoalAttached, 1)
	// Pause before it can accumulate checks, then cancel.
	controls <- protocol.Control{Kind: protocol.ControlGoalPause}
	waitForEvent(t, es, event.TypeGoalPaused, 1)
	controls <- protocol.Control{Kind: protocol.ControlGoalCancel}
	waitForEvent(t, es, event.TypeGoalCancelled, 1)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, es.Dir(), event.TypeGoalAchieved); n != 0 {
		t.Fatalf("GoalAchieved = %d, want 0 (cancelled, not achieved)", n)
	}
}

// TestGoalRecover proves the crash-recovery repair (review Bug 1): a crash that
// leaves a checkpoint at the current gen step with its follow-up event missing
// is repaired by goalRecover at the drive-loop safe point — since the
// goal_verify quiescent cell is skipped on resume (the shape is already
// quiescent). It must re-emit a dropped achievement receipt AND re-inject a
// dropped miss feedback, without doing so twice.
func TestGoalRecover(t *testing.T) {
	newLoop := func(t *testing.T, g *state.Goal) (*Loop, *driveState, *store.EventStore) {
		es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "s"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = es.Close() })
		ds := &driveState{s: state.State{Goal: g}}
		ds.s.Session.GenStep = 3
		return &Loop{Store: es, SessionID: "g"}, ds, es
	}

	t.Run("miss re-injects feedback", func(t *testing.T) {
		l, ds, es := newLoop(t, &state.Goal{GoalID: "goal", Goal: "x",
			Budget: event.GoalBudget{MaxChecks: 5}, Checks: 1, CheckpointedGenStep: 3, LastFeedback: "retry the fix"})
		if err := l.goalRecover(ds, l.appender(ds)); err != nil {
			t.Fatal(err)
		}
		if n := countEvents(t, es.Dir(), event.TypeInputReceived); n != 1 {
			t.Fatalf("re-injected inputs = %d, want 1", n)
		}
		if !hasInputAfterLastAssistant(ds.s) {
			t.Fatal("recovered feedback did not fold into the conversation")
		}
		// Idempotent: a second recovery (feedback now present) does nothing.
		if err := l.goalRecover(ds, l.appender(ds)); err != nil {
			t.Fatal(err)
		}
		if n := countEvents(t, es.Dir(), event.TypeInputReceived); n != 1 {
			t.Fatalf("re-injected inputs after 2nd recover = %d, want 1 (no double-inject)", n)
		}
	})

	t.Run("pass re-emits achieved", func(t *testing.T) {
		l, ds, es := newLoop(t, &state.Goal{GoalID: "goal", Goal: "x",
			Budget: event.GoalBudget{MaxChecks: 5}, Checks: 2, CheckpointedGenStep: 3, LastPass: true})
		if err := l.goalRecover(ds, l.appender(ds)); err != nil {
			t.Fatal(err)
		}
		if ach := decodeGoalAchieved(t, es.Dir()); ach.Reason != "satisfied" {
			t.Fatalf("recovered achieved reason = %q, want satisfied", ach.Reason)
		}
		if ds.s.Goal != nil {
			t.Fatal("goal not detached after recovered achievement")
		}
	})

	t.Run("budget-spent re-emits achieved{budget}", func(t *testing.T) {
		l, ds, es := newLoop(t, &state.Goal{GoalID: "goal", Goal: "x",
			Budget: event.GoalBudget{MaxChecks: 2}, Checks: 2, CheckpointedGenStep: 3})
		if err := l.goalRecover(ds, l.appender(ds)); err != nil {
			t.Fatal(err)
		}
		if ach := decodeGoalAchieved(t, es.Dir()); ach.Reason != "budget" {
			t.Fatalf("recovered achieved reason = %q, want budget", ach.Reason)
		}
	})
}

func decodeGoalCheckpoints(t *testing.T, dir string) []*event.GoalCheckpoint {
	t.Helper()
	evs, _ := store.ReadEvents(dir)
	var out []*event.GoalCheckpoint
	for _, e := range evs {
		if e.Type == event.TypeGoalCheckpoint {
			p, _ := event.DecodePayload(e)
			out = append(out, p.(*event.GoalCheckpoint))
		}
	}
	return out
}

func decodeGoalAchieved(t *testing.T, dir string) *event.GoalAchieved {
	t.Helper()
	evs, _ := store.ReadEvents(dir)
	for _, e := range evs {
		if e.Type == event.TypeGoalAchieved {
			p, _ := event.DecodePayload(e)
			return p.(*event.GoalAchieved)
		}
	}
	t.Fatal("no GoalAchieved event")
	return nil
}
