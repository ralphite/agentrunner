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
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// modeControlLoop is rememberLoop's sibling wired with a real PermissionGate,
// so a mode switch is observable in how the gate treats a subsequent edit
// (the mode flows in via the effect — no gate reconfiguration on switch).
func modeControlLoop(t *testing.T, fix scripted.Fixture, mode string) (string, *store.EventStore, chan protocol.UserInput, chan protocol.Control, chan error) {
	t.Helper()
	wsDir := t.TempDir()
	ws, err := workspace.New(wsDir)
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
			Name:               "modectl",
			Model:              ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			Tools:              []string{"read_file", "edit_file", "bash"},
			MaxGenerationSteps: 8,
		},
		Provider:   scripted.New(fix),
		Exec:       &tool.Executor{WS: ws},
		Store:      es,
		Clock:      clock.NewFake(time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)),
		SessionID:  "modectl-sess",
		UserInputs: inbox,
		Controls:   controls,
		Mode:       mode,
		Pipeline:   &pipeline.Pipeline{Gates: []pipeline.Gate{&pipeline.PermissionGate{}}},
	}
	done := make(chan error, 1)
	go func() { _, e := l.Run(context.Background(), "start"); done <- e }()
	return wsDir, es, inbox, controls, done
}

// modeChanges folds the journal's ModeChanged events into (payload list).
func modeChanges(t *testing.T, dir string) []*event.ModeChanged {
	t.Helper()
	evs, _ := store.ReadEvents(dir)
	var out []*event.ModeChanged
	for _, e := range evs {
		if e.Type != event.TypeModeChanged {
			continue
		}
		dec, err := event.DecodePayload(e)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, dec.(*event.ModeChanged))
	}
	return out
}

func foldMode(t *testing.T, dir string) string {
	t.Helper()
	evs, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	final, err := state.Fold(evs)
	if err != nil {
		t.Fatal(err)
	}
	return final.CurrentMode()
}

// A user mode control switches default→acceptEdits at the idle boundary: the
// journal carries ModeChanged{user}, and the NEXT edit executes without any
// approval ask (the gate reads the live fold mode off the effect).
func TestModeControlSwitchesToAcceptEdits(t *testing.T) {
	t.Setenv("AGENTRUNNER_APPROVE", "never") // a stray ask would deny, not hang
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "ready"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "edit_file", Args: map[string]any{
				"path": "a.txt", "old": "hello", "new": "world"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "edited"}, {Finish: "end_turn"}}},
	}}
	wsDir, es, inbox, controls, done := modeControlLoop(t, fix, "")
	if err := os.WriteFile(filepath.Join(wsDir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	waitForEvent(t, es, event.TypeAssistantMessage, 1) // turn 1 done → idle
	controls <- protocol.Control{Kind: protocol.ControlMode, Directive: pipeline.ModeAcceptEdits}
	waitForEvent(t, es, event.TypeModeChanged, 1)
	inbox <- protocol.UserInput{Text: "now edit a.txt"}
	waitForEvent(t, es, event.TypeAssistantMessage, 2)

	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	mcs := modeChanges(t, es.Dir())
	if len(mcs) != 1 || mcs[0].To != pipeline.ModeAcceptEdits || mcs[0].Cause != "user" {
		t.Fatalf("mode changes = %+v, want one user→acceptEdits", mcs)
	}
	if n := countEvents(t, es.Dir(), event.TypeApprovalRequested); n != 0 {
		t.Errorf("acceptEdits edit asked for approval %d times, want 0", n)
	}
	if got, _ := os.ReadFile(filepath.Join(wsDir, "a.txt")); string(got) != "world" {
		t.Errorf("edit did not land: a.txt = %q", got)
	}
	if m := foldMode(t, es.Dir()); m != pipeline.ModeAcceptEdits {
		t.Errorf("folded mode = %q, want acceptEdits", m)
	}
}

// Switching back acceptEdits→default restores the ask: the same edit that
// would auto-run now raises an approval (denied by the test env, so the loop
// finishes without hanging).
func TestModeControlSwitchBack(t *testing.T) {
	t.Setenv("AGENTRUNNER_APPROVE", "never")
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "ready"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "edit_file", Args: map[string]any{
				"path": "a.txt", "old": "hello", "new": "world"}}},
			{Finish: "tool_use"},
		}},
		{
			Expect:  scripted.Expect{LastMessageContains: "denied"},
			Respond: []scripted.Event{{Text: "blocked"}, {Finish: "end_turn"}},
		},
	}}
	wsDir, es, inbox, controls, done := modeControlLoop(t, fix, pipeline.ModeAcceptEdits)
	if err := os.WriteFile(filepath.Join(wsDir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- protocol.Control{Kind: protocol.ControlMode, Directive: pipeline.ModeDefault}
	waitForEvent(t, es, event.TypeModeChanged, 2) // startup(acceptEdits) + user(default)
	inbox <- protocol.UserInput{Text: "now edit a.txt"}
	waitForEvent(t, es, event.TypeAssistantMessage, 2)

	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	mcs := modeChanges(t, es.Dir())
	last := mcs[len(mcs)-1]
	if last.To != pipeline.ModeDefault || last.Cause != "user" {
		t.Fatalf("last mode change = %+v, want user→default", last)
	}
	if n := countEvents(t, es.Dir(), event.TypeApprovalRequested); n != 1 {
		t.Errorf("default edit approvals = %d, want 1", n)
	}
	if got, _ := os.ReadFile(filepath.Join(wsDir, "a.txt")); string(got) != "hello" {
		t.Errorf("denied edit must not land: a.txt = %q", got)
	}
	if m := foldMode(t, es.Dir()); m != pipeline.ModeDefault {
		t.Errorf("folded mode = %q, want default", m)
	}
}

// Invalid targets never transition and answer with an explicit rejected
// receipt: bypass (start-only), unknown names, and any switch out of plan
// (approval-gated via exit_plan_mode, not user command).
func TestModeControlRejectsInvalid(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "ready"}, {Finish: "end_turn"}}},
	}}
	_, es, inbox, controls, done := modeControlLoop(t, fix, "")

	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- protocol.Control{CommandRef: protocol.CommandRef{CommandID: "cmd-bypass", CommandSeq: 1},
		Kind: protocol.ControlMode, Directive: pipeline.ModeBypass}
	waitForEvent(t, es, event.TypeCommandHandled, 1)
	controls <- protocol.Control{CommandRef: protocol.CommandRef{CommandID: "cmd-nonsense", CommandSeq: 2},
		Kind: protocol.ControlMode, Directive: "yolo"}
	waitForEvent(t, es, event.TypeCommandHandled, 2)

	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if n := len(modeChanges(t, es.Dir())); n != 0 {
		t.Fatalf("invalid controls produced %d ModeChanged events, want 0", n)
	}
	if m := foldMode(t, es.Dir()); m != pipeline.ModeDefault {
		t.Errorf("folded mode = %q, want default", m)
	}
	evs, _ := store.ReadEvents(es.Dir())
	var results []string
	for _, e := range evs {
		if e.Type != event.TypeCommandHandled {
			continue
		}
		dec, err := event.DecodePayload(e)
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, dec.(*event.CommandHandled).Result)
	}
	if len(results) != 2 || !strings.Contains(results[0], "rejected") ||
		!strings.Contains(results[0], "not a valid runtime transition") ||
		!strings.Contains(results[1], "rejected") || !strings.Contains(results[1], "unknown mode") {
		t.Fatalf("receipts = %q, want two explicit rejections", results)
	}

	// Plan mode: user command may not leave plan — that is exit_plan_mode's
	// approval flow.
	planFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "planning"}, {Finish: "end_turn"}}},
	}}
	_, es2, inbox2, controls2, done2 := modeControlLoop(t, planFix, pipeline.ModePlan)
	waitForEvent(t, es2, event.TypeAssistantMessage, 1)
	controls2 <- protocol.Control{CommandRef: protocol.CommandRef{CommandID: "cmd-plan", CommandSeq: 1},
		Kind: protocol.ControlMode, Directive: pipeline.ModeDefault}
	waitForEvent(t, es2, event.TypeCommandHandled, 1)
	close(inbox2)
	if err := <-done2; err != nil {
		t.Fatal(err)
	}
	if m := foldMode(t, es2.Dir()); m != pipeline.ModePlan {
		t.Errorf("plan session folded mode = %q, want plan", m)
	}
	evs2, _ := store.ReadEvents(es2.Dir())
	for _, e := range evs2 {
		if e.Type != event.TypeCommandHandled {
			continue
		}
		dec, _ := event.DecodePayload(e)
		if r := dec.(*event.CommandHandled).Result; !strings.Contains(r, "exit_plan_mode") {
			t.Errorf("plan rejection receipt = %q, want exit_plan_mode pointer", r)
		}
	}
}

// Re-delivering the same mode command is effect-level idempotent: the second
// application is a same-mode no-op with its own receipt, never a second
// ModeChanged (durable-command replay safety).
func TestModeControlIdempotentReplay(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "ready"}, {Finish: "end_turn"}}},
	}}
	_, es, inbox, controls, done := modeControlLoop(t, fix, "")

	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	ctl := protocol.Control{CommandRef: protocol.CommandRef{CommandID: "cmd-mode-1", CommandSeq: 1},
		Kind: protocol.ControlMode, Directive: pipeline.ModeAcceptEdits}
	controls <- ctl
	waitForEvent(t, es, event.TypeModeChanged, 1)
	controls <- ctl // replay of the identical durable command
	waitForEvent(t, es, event.TypeCommandHandled, 1)

	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if n := len(modeChanges(t, es.Dir())); n != 1 {
		t.Fatalf("ModeChanged count = %d, want exactly 1 across a replay", n)
	}
	if m := foldMode(t, es.Dir()); m != pipeline.ModeAcceptEdits {
		t.Errorf("folded mode = %q, want acceptEdits", m)
	}
}
