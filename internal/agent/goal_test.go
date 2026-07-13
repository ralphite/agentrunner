package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
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
	var verifierActivities int
	for _, env := range readEvents(t, es.Dir()) {
		if env.Type == event.TypeActivityStarted {
			p, _ := event.DecodePayload(env)
			if p.(*event.ActivityStarted).Name == "verifier:command" {
				verifierActivities++
			}
		}
	}
	if verifierActivities != 2 {
		t.Fatalf("verifier activity traces = %d, want 2", verifierActivities)
	}
}

func TestInSessionGoalVerifierPipelineDenyBinds(t *testing.T) {
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()
	ds := &driveState{s: state.State{Goal: &state.Goal{
		GoalID: "g-deny", Goal: "must not run",
		Verifiers: []event.GoalVerifier{{Kind: "command", Command: "touch escaped"}},
		Budget:    event.GoalBudget{MaxChecks: 1},
	}}}
	ds.s.Session.GenStep = 1
	l := &Loop{
		Spec: &AgentSpec{}, Exec: &tool.Executor{WS: ws}, Store: es,
		Clock:     clock.NewFake(time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)),
		SessionID: "goal-deny",
		Pipeline: &pipeline.Pipeline{Gates: []pipeline.Gate{
			&pipeline.PermissionGate{Rules: []pipeline.PermissionRule{
				{Tool: "bash", Action: "deny"}, {Action: "allow"},
			}, WS: ws},
		}},
	}
	reason := "completed"
	if err := goalCheckpoint(context.Background(), l, ds, l.appender(ds), &reason); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ws.Root(), "escaped")); !os.IsNotExist(err) {
		t.Fatalf("denied verifier executed: %v", err)
	}
	var requested, denied, started bool
	for _, env := range readEvents(t, es.Dir()) {
		switch env.Type {
		case event.TypeEffectRequested:
			requested = true
		case event.TypeEffectResolved:
			p, _ := event.DecodePayload(env)
			denied = p.(*event.EffectResolved).Verdict == event.VerdictDeny
		case event.TypeActivityStarted:
			p, _ := event.DecodePayload(env)
			started = started || p.(*event.ActivityStarted).Name == "verifier:command"
		}
	}
	if !requested || !denied || started {
		t.Fatalf("trace requested=%v denied=%v verifier_started=%v", requested, denied, started)
	}
}

func TestInSessionGoalVerifierCompletedResultIsNotRerun(t *testing.T) {
	l := testLoop(t, scripted.Fixture{}, t.TempDir())
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		&pipeline.PermissionGate{Rules: []pipeline.PermissionRule{{Action: "allow"}}, WS: l.Exec.WS},
	}}
	g := &state.Goal{GoalID: "g", Verifiers: []event.GoalVerifier{{Kind: "command", Command: "true"}}}
	ds := &driveState{s: state.State{Goal: g}}
	ds.s.Session.GenStep = 3
	appendE := l.appender(ds)
	for i := 0; i < 2; i++ {
		pass, detail, err := l.goalVerify(context.Background(), ds, appendE, g)
		if err != nil || !pass {
			t.Fatalf("verify %d = pass %v detail %q err %v", i+1, pass, detail, err)
		}
	}
	var effects, activities int
	for _, env := range readEvents(t, l.Store.Dir()) {
		switch env.Type {
		case event.TypeEffectRequested:
			effects++
		case event.TypeActivityStarted:
			p, _ := event.DecodePayload(env)
			if p.(*event.ActivityStarted).Name == "verifier:command" {
				activities++
			}
		}
	}
	if effects != 1 || activities != 1 {
		t.Fatalf("replayed completed verifier: effects=%d activities=%d", effects, activities)
	}
}

func readEvents(t *testing.T, dir string) []event.Envelope {
	t.Helper()
	events, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	return events
}

// TestInSessionGoalBudgetTruncation: a verifier that never passes stops at the
// budget (max_checks) with GoalExhausted{budget}; the unmet goal remains
// available for update and does NOT re-inject forever.
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
	waitForEvent(t, es, event.TypeGoalExhausted, 1)
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
	if exhausted := decodeGoalExhausted(t, es.Dir()); exhausted.Reason != "budget" {
		t.Fatalf("GoalExhausted.Reason = %q, want budget", exhausted.Reason)
	}
}

func TestGoalUpdateRecoversExhaustedGoal(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "starting"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "not done yet"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "recover", Name: "bash", Args: map[string]any{"command": "touch recovered.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done after budget update"}, {Finish: "end_turn"}}},
	}}
	es, inbox, controls, done := controlLoop(t, fix, 10)
	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- protocol.Control{Kind: protocol.ControlGoalAttach, Goal: &protocol.GoalControl{
		GoalID: "goal", Goal: "create recovered.txt",
		Verifiers: []event.GoalVerifier{{Kind: "command", Command: "test -f recovered.txt"}},
		Budget:    &event.GoalBudget{MaxChecks: 1},
	}}
	waitForEvent(t, es, event.TypeGoalExhausted, 1)
	controls <- protocol.Control{Kind: protocol.ControlGoalUpdate, Goal: &protocol.GoalControl{
		Budget: &event.GoalBudget{MaxChecks: 3},
	}}
	waitForEvent(t, es, event.TypeGoalAchieved, 1)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(es.Dir())
	if err != nil {
		t.Fatal(err)
	}
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if fold.Goal != nil {
		t.Fatalf("goal still attached after recovery: %+v", fold.Goal)
	}
	if fold.Session.GoalOutcome != "satisfied" {
		t.Fatalf("recovered goal outcome = %q, want satisfied", fold.Session.GoalOutcome)
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

	t.Run("budget-spent re-emits exhausted{budget}", func(t *testing.T) {
		l, ds, es := newLoop(t, &state.Goal{GoalID: "goal", Goal: "x",
			Budget: event.GoalBudget{MaxChecks: 2}, Checks: 2, CheckpointedGenStep: 3})
		if err := l.goalRecover(ds, l.appender(ds)); err != nil {
			t.Fatal(err)
		}
		if exhausted := decodeGoalExhausted(t, es.Dir()); exhausted.Reason != "budget" {
			t.Fatalf("recovered exhausted reason = %q, want budget", exhausted.Reason)
		}
		if ds.s.Goal == nil || !ds.s.Goal.Exhausted {
			t.Fatal("budget exhaustion did not retain the goal")
		}
	})
}

// TestInSessionGoalSelfCertify is the INC-10 core proof: a goal WITHOUT a
// command verifier is legal and completable — the boundary before any claim is
// a miss whose feedback teaches the goal_complete path; the model's claim then
// passes the next boundary as model-certified (GoalAchieved{satisfied}), all
// on ONE continuing session.
func TestInSessionGoalSelfCertify(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		// turn 1 — the opening "first question".
		{Respond: []scripted.Event{{Text: "starting"}, {Finish: "end_turn"}}},
		// turn 2 — runs on the attached goal's statement; no claim yet → MISS.
		{Respond: []scripted.Event{{Text: "working on it"}, {Finish: "end_turn"}}},
		// turn 3 — runs on the continuation feedback; declares completion.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "goal_complete",
				Args: map[string]any{"summary": "wrote the summary file and re-read it"}}},
			{Finish: "tool_use"},
		}},
		// turn 3 cont — after the claim's tool result; boundary adjudicates.
		{Respond: []scripted.Event{{Text: "declared done"}, {Finish: "end_turn"}}},
	}}
	es, inbox, controls, done := controlLoop(t, fix, 10)
	waitForEvent(t, es, event.TypeAssistantMessage, 1)

	controls <- protocol.Control{Kind: protocol.ControlGoalAttach, Goal: &protocol.GoalControl{
		GoalID: "goal", Goal: "summarize the repo into notes.md",
		Budget: &event.GoalBudget{MaxChecks: 5}, // NO verifiers: self-certified
	}}

	waitForEvent(t, es, event.TypeGoalAchieved, 1)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	checks := decodeGoalCheckpoints(t, es.Dir())
	if len(checks) != 2 {
		t.Fatalf("checkpoints = %d, want 2: %+v", len(checks), checks)
	}
	if checks[0].Pass || !checks[1].Pass {
		t.Fatalf("checkpoint verdicts = [%v %v], want [miss pass]", checks[0].Pass, checks[1].Pass)
	}
	// The miss feedback must teach the completion path (the continuation).
	if !strings.Contains(checks[0].Feedback, "goal_complete") {
		t.Fatalf("miss feedback does not mention goal_complete:\n%s", checks[0].Feedback)
	}
	if !strings.Contains(checks[1].Detail, "model-certified") {
		t.Fatalf("pass detail = %q, want model-certified", checks[1].Detail)
	}
	if n := countEvents(t, es.Dir(), event.TypeGoalCompletionClaimed); n != 1 {
		t.Fatalf("GoalCompletionClaimed = %d, want 1", n)
	}
	if ach := decodeGoalAchieved(t, es.Dir()); ach.Reason != "satisfied" {
		t.Fatalf("GoalAchieved.Reason = %q, want satisfied", ach.Reason)
	}
	// Context continuity: one session, no fresh child run.
	if n := countEvents(t, es.Dir(), event.TypeSessionStarted); n != 1 {
		t.Fatalf("SessionStarted = %d, want 1", n)
	}
}

// TestInSessionGoalClaimDoesNotOverrideVerifier: with a command verifier the
// verifier stays the SOLE judge — a goal_complete claim on a missing artifact
// is rejected (annotated in the miss detail), and the budget truncation still
// lands (决策 #31).
func TestInSessionGoalClaimDoesNotOverrideVerifier(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "turn 1"}, {Finish: "end_turn"}}},
		// turn 2 — claims completion though the verifier's file was never made.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "goal_complete",
				Args: map[string]any{"summary": "done (it is not)"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "claimed"}, {Finish: "end_turn"}}},
		// turn 3 — runs on the rejection feedback; still never makes the file.
		{Respond: []scripted.Event{{Text: "still not making it"}, {Finish: "end_turn"}}},
	}}
	es, inbox, controls, done := controlLoop(t, fix, 10)
	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- protocol.Control{Kind: protocol.ControlGoalAttach, Goal: &protocol.GoalControl{
		GoalID: "goal", Goal: "make never.txt",
		Verifiers: []event.GoalVerifier{{Kind: "command", Command: "test -f never.txt"}},
		Budget:    &event.GoalBudget{MaxChecks: 2},
	}}
	waitForEvent(t, es, event.TypeGoalExhausted, 1)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	checks := decodeGoalCheckpoints(t, es.Dir())
	if len(checks) != 2 {
		t.Fatalf("checkpoints = %d, want 2", len(checks))
	}
	for _, c := range checks {
		if c.Pass {
			t.Fatal("a checkpoint passed; the verifier never passes and the claim must not override it")
		}
	}
	if !strings.Contains(checks[0].Detail, "completion claim rejected") {
		t.Fatalf("first miss detail = %q, want the claim-rejected annotation", checks[0].Detail)
	}
	if exhausted := decodeGoalExhausted(t, es.Dir()); exhausted.Reason != "budget" {
		t.Fatalf("GoalExhausted.Reason = %q, want budget", exhausted.Reason)
	}
}

// TestInSessionGoalResumeContinues (INC-10 review P1): resuming a paused goal
// on an idle session must RE-ARM the boundary discipline — the resume injects
// a program input so the loop runs a turn and the next boundary adjudicates
// (here: the verifier passes after the resumed turn creates the file).
func TestInSessionGoalResumeContinues(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "turn 1"}, {Finish: "end_turn"}}},
		// turn 2 — runs on the goal statement while the goal is already paused
		// (attach+pause drain at the same safe point); boundary skips.
		{Respond: []scripted.Event{{Text: "noted, goal is paused"}, {Finish: "end_turn"}}},
		// turn 3 — runs on the resume re-injection; does the work.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "b1", Name: "bash", Args: map[string]any{"command": "touch resumed.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done after resume"}, {Finish: "end_turn"}}},
	}}
	es, inbox, controls, done := controlLoop(t, fix, 10)
	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- protocol.Control{Kind: protocol.ControlGoalAttach, Goal: &protocol.GoalControl{
		GoalID: "goal", Goal: "create resumed.txt",
		Verifiers: []event.GoalVerifier{{Kind: "command", Command: "test -f resumed.txt"}},
		Budget:    &event.GoalBudget{MaxChecks: 5},
	}}
	controls <- protocol.Control{Kind: protocol.ControlGoalPause}
	waitForEvent(t, es, event.TypeGoalPaused, 1)
	controls <- protocol.Control{Kind: protocol.ControlGoalResume}
	// The whole point: after resume the session must continue on its own and
	// reach an adjudicated boundary — without any user send.
	waitForEvent(t, es, event.TypeGoalAchieved, 1)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if ach := decodeGoalAchieved(t, es.Dir()); ach.Reason != "satisfied" {
		t.Fatalf("GoalAchieved.Reason = %q, want satisfied", ach.Reason)
	}
}

// TestInSessionGoalNoVerifierBudget: a self-certified goal whose model never
// claims completion still terminates — every boundary is a counted miss and
// the check budget ends it with a visible truncation (决策 #31 preserved).
func TestInSessionGoalNoVerifierBudget(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "turn 1"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "working 1"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "working 2"}, {Finish: "end_turn"}}},
	}}
	es, inbox, controls, done := controlLoop(t, fix, 10)
	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- protocol.Control{Kind: protocol.ControlGoalAttach, Goal: &protocol.GoalControl{
		GoalID: "goal", Goal: "a goal never claimed done",
		Budget: &event.GoalBudget{MaxChecks: 2},
	}}
	waitForEvent(t, es, event.TypeGoalExhausted, 1)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, es.Dir(), event.TypeGoalCompletionClaimed); n != 0 {
		t.Fatalf("GoalCompletionClaimed = %d, want 0", n)
	}
	if exhausted := decodeGoalExhausted(t, es.Dir()); exhausted.Reason != "budget" {
		t.Fatalf("GoalExhausted.Reason = %q, want budget", exhausted.Reason)
	}
}

// TestGoalResumeCheck proves the OTHER crash-window repair (INC-10 review
// P1): a crash after a graceful turn end but BEFORE its goal checkpoint
// resumes into an already-quiescent shape where the goal_verify cell never
// runs — goalResumeCheck at the drive-loop safe point must adjudicate that
// boundary (here: a recorded claim on a self-certified goal → achieved).
func TestGoalResumeCheck(t *testing.T) {
	newDS := func(t *testing.T, g *state.Goal) (*Loop, *driveState, *store.EventStore) {
		es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "s"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = es.Close() })
		ds := &driveState{s: state.State{Goal: g}}
		ds.s.Session.GenStep = 2
		// The quiescent "completed" shape: a final assistant message with no
		// tool calls and nothing after it.
		ds.s.Conversation.Messages = []provider.Message{
			{Role: provider.RoleUser, Parts: []provider.Part{{Kind: provider.PartText, Text: "go"}}},
			{Role: provider.RoleAssistant, Parts: []provider.Part{{Kind: provider.PartText, Text: "done"}}},
		}
		return &Loop{Store: es, SessionID: "g"}, ds, es
	}

	t.Run("pending claim adjudicated on resume", func(t *testing.T) {
		l, ds, es := newDS(t, &state.Goal{GoalID: "goal", Goal: "x",
			Budget: event.GoalBudget{MaxChecks: 5}, Claimed: true, ClaimSummary: "did it"})
		if err := l.goalResumeCheck(context.Background(), ds, l.appender(ds)); err != nil {
			t.Fatal(err)
		}
		if ach := decodeGoalAchieved(t, es.Dir()); ach.Reason != "satisfied" {
			t.Fatalf("GoalAchieved.Reason = %q, want satisfied", ach.Reason)
		}
		if ds.s.Goal != nil {
			t.Fatal("goal not detached after the recovered adjudication")
		}
		// Idempotent: the goal is gone, a second pass is a no-op.
		if err := l.goalResumeCheck(context.Background(), ds, l.appender(ds)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("queued input defers to the live boundary", func(t *testing.T) {
		l, ds, es := newDS(t, &state.Goal{GoalID: "goal", Goal: "x",
			Budget: event.GoalBudget{MaxChecks: 5}, Claimed: true, ClaimSummary: "did it"})
		ds.s.Conversation.Messages = append(ds.s.Conversation.Messages,
			provider.Message{Role: provider.RoleUser, Parts: []provider.Part{{Kind: provider.PartText, Text: "more"}}})
		if err := l.goalResumeCheck(context.Background(), ds, l.appender(ds)); err != nil {
			t.Fatal(err)
		}
		if n := countEvents(t, es.Dir(), event.TypeGoalCheckpoint); n != 0 {
			t.Fatalf("checkpointed despite a queued input: %d", n)
		}
	})

	t.Run("already-checkpointed gen step is a no-op", func(t *testing.T) {
		l, ds, es := newDS(t, &state.Goal{GoalID: "goal", Goal: "x",
			Budget: event.GoalBudget{MaxChecks: 5}, Claimed: true, CheckpointedGenStep: 2})
		if err := l.goalResumeCheck(context.Background(), ds, l.appender(ds)); err != nil {
			t.Fatal(err)
		}
		if n := countEvents(t, es.Dir(), event.TypeGoalCheckpoint); n != 0 {
			t.Fatalf("re-checkpointed an adjudicated gen step: %d", n)
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

func decodeGoalExhausted(t *testing.T, dir string) *event.GoalExhausted {
	t.Helper()
	evs, _ := store.ReadEvents(dir)
	for _, e := range evs {
		if e.Type == event.TypeGoalExhausted {
			p, _ := event.DecodePayload(e)
			return p.(*event.GoalExhausted)
		}
	}
	t.Fatal("no GoalExhausted event")
	return nil
}
