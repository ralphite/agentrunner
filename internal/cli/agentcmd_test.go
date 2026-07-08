package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

const firstAgentYAML = `name: first-hat
model: { provider: scripted, id: x }
system_prompt: you wear the first hat
`

const secondAgentYAML = `name: second-hat
model: { provider: scripted, id: x }
system_prompt: you wear the second hat
`

const firstHatFixtureYAML = `steps:
  - respond:
      - text: "first hat speaking"
      - finish: end_turn
`

const secondHatFixtureYAML = `steps:
  - respond:
      - text: "second hat speaking"
      - finish: end_turn
`

// 决策 #32: a session is not bound to an agent. `agentrunner agent`
// journals SpecChanged with NO confirmation; the next resume runs the new
// spec — new identity in the fold, new prefix generation, new permission
// layers.
func TestAgentSwitchTakesEffectOnResume(t *testing.T) {
	base := t.TempDir()
	xdg := filepath.Join(base, "xdg")
	ws := filepath.Join(base, "ws")
	if err := os.Mkdir(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", xdg)
	write := func(name, content string) string {
		t.Helper()
		p := filepath.Join(base, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	spec1 := write("first.yaml", firstAgentYAML)
	spec2 := write("second.yaml", secondAgentYAML)
	fix1 := write("fix1.yaml", firstHatFixtureYAML)
	fix2 := write("fix2.yaml", secondHatFixtureYAML)

	// Open under the first agent and quiesce.
	t.Setenv("AGENTRUNNER_SCRIPTED_FIXTURE", fix1)
	var out, errOut bytes.Buffer
	if code := Run([]string{"run", "--workspace", ws, spec1, "hello"}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("run exit = %d\n%s", code, errOut.String())
	}
	sessions, err := os.ReadDir(filepath.Join(xdg, "agentrunner", "sessions"))
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %v (%v)", sessions, err)
	}
	sid := sessions[0].Name()

	// Switch agents — no confirmation, just the command.
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"agent", sid[:8], spec2}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("agent exit = %d\n%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "agent switched to second-hat") {
		t.Fatalf("agent output = %q", out.String())
	}
	dir := filepath.Join(xdg, "agentrunner", "sessions", sid)
	events, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	var changed *event.SpecChanged
	for _, e := range events {
		if e.Type == event.TypeSpecChanged {
			dec, _ := event.DecodePayload(e)
			changed = dec.(*event.SpecChanged)
		}
	}
	if changed == nil || changed.SpecName != "second-hat" || changed.Source != "user" {
		t.Fatalf("spec_changed = %+v", changed)
	}
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if fold.Session.SpecName != "second-hat" {
		t.Fatalf("fold spec = %q, want second-hat", fold.Session.SpecName)
	}

	// A queued input + resume runs the NEW agent (prefix regenerated, the
	// second fixture answers).
	if _, err := store.AppendInbox(dir, protocol.UserInput{Text: "who are you now?"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTRUNNER_SCRIPTED_FIXTURE", fix2)
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"resume", sid[:8]}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("resume exit = %d\n%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "second hat speaking") {
		t.Fatalf("resume output = %q, want the second agent's reply", out.String())
	}
	final, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	ffold, err := state.Fold(final)
	if err != nil {
		t.Fatal(err)
	}
	if q, reason := state.Quiescence(ffold); !q || reason != "completed" {
		t.Fatalf("post-switch shape = %v %q", q, reason)
	}
}
