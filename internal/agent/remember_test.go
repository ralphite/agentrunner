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
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// rememberLoop is controlLoop's sibling that also hands back the workspace
// root, so a test can assert the CLAUDE.md that `remember` writes.
func rememberLoop(t *testing.T, fix scripted.Fixture) (string, *store.EventStore, chan protocol.UserInput, chan protocol.Control, chan error) {
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
			Name:               "rem",
			Model:              ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			Tools:              []string{"bash"},
			MaxGenerationSteps: 5,
		},
		Provider:   scripted.New(fix),
		Exec:       &tool.Executor{WS: ws},
		Store:      es,
		Clock:      clock.NewFake(time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)),
		SessionID:  "rem-sess",
		UserInputs: inbox,
		Controls:   controls,
	}
	done := make(chan error, 1)
	go func() { _, e := l.Run(context.Background(), "first question"); done <- e }()
	return wsDir, es, inbox, controls, done
}

// programInputWith finds a program-source InputReceived whose text carries the
// note, and returns how many such notes were journaled.
func rememberedInputs(t *testing.T, dir, note string) int {
	t.Helper()
	evs, _ := store.ReadEvents(dir)
	n := 0
	for _, e := range evs {
		if e.Type != event.TypeInputReceived {
			continue
		}
		dec, err := event.DecodePayload(e)
		if err != nil {
			t.Fatal(err)
		}
		in := dec.(*event.InputReceived)
		if in.Source == "program" && strings.Contains(in.Text, note) {
			n++
		}
	}
	return n
}

// A remember control writes the note to the workspace CLAUDE.md AND surfaces it
// as a program-source input so the running conversation honors it — this
// continues the thread with a confirmation turn (same shape as goal
// reinjection: a program input is user-role, so decide() runs one turn).
func TestRememberControlWritesFileAndMessage(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answer one"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "好的，之后一律用 pnpm"}, {Finish: "end_turn"}}}, // confirmation turn
	}}
	wsDir, es, inbox, controls, done := rememberLoop(t, fix)

	waitForEvent(t, es, event.TypeAssistantMessage, 1) // turn 1 done → idle
	controls <- protocol.Control{Kind: protocol.ControlRemember, Directive: "always use pnpm"}
	waitForEvent(t, es, event.TypeAssistantMessage, 2) // remember's confirmation turn ran

	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// File: note under the Remembered section, workspace-root CLAUDE.md.
	got, err := os.ReadFile(filepath.Join(wsDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md not written: %v", err)
	}
	if !strings.Contains(string(got), "- always use pnpm\n") {
		t.Errorf("note missing from CLAUDE.md: %q", got)
	}
	// Journal: exactly one program-source input carrying the note.
	if n := rememberedInputs(t, es.Dir(), "always use pnpm"); n != 1 {
		t.Errorf("program-source remember inputs = %d, want 1", n)
	}
}

// Repeating the same note is idempotent at the file level (replay safety): the
// CLAUDE.md carries it once even though two remember controls ran.
func TestRememberControlIsIdempotent(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answer one"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "记住了"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "已经记着"}, {Finish: "end_turn"}}},
	}}
	wsDir, es, inbox, controls, done := rememberLoop(t, fix)

	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- protocol.Control{Kind: protocol.ControlRemember, Directive: "use pnpm"}
	waitForEvent(t, es, event.TypeAssistantMessage, 2)
	controls <- protocol.Control{Kind: protocol.ControlRemember, Directive: "use pnpm"}
	waitForEvent(t, es, event.TypeAssistantMessage, 3)

	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(wsDir, "CLAUDE.md"))
	if c := strings.Count(string(got), "- use pnpm\n"); c != 1 {
		t.Errorf("note written %d times, want 1 (idempotent file write): %q", c, got)
	}
}
