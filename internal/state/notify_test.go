package state

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
)

// foldBackgroundOutcome runs one background task from spawn to the given
// terminal event and reports whether its outcome landed in the conversation
// (the INC-39 notify gate) and whether the handle was removed.
func foldBackgroundOutcome(t *testing.T, notify string, terminal string, isError bool) (gotMessage, handleGone bool) {
	t.Helper()
	args := `{"command":"true","background":true`
	if notify != "" {
		args += `,"notify":` + string(mustJSON(t, notify))
	}
	args += `}`
	s := New()
	var err error
	s, err = Apply(s, env(t, event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: "tool-call_1_0", Kind: event.KindTool, Name: "bash",
		Args: json.RawMessage(args), CallID: "call_1_0", Attempt: 1, Background: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Handles["call_1_0"]; !ok {
		t.Fatal("background spawn must register a handle")
	}
	var terminalPayload any
	switch terminal {
	case event.TypeActivityCompleted:
		terminalPayload = &event.ActivityCompleted{ActivityID: "tool-call_1_0",
			Result: json.RawMessage(`{"stdout":"tail","exit_code":1}`), IsError: isError}
	case event.TypeActivityFailed:
		terminalPayload = &event.ActivityFailed{ActivityID: "tool-call_1_0", Attempt: 1, Final: true,
			Error: event.ErrorInfo{Class: "runtime", Message: "boom"}}
	case event.TypeActivityCancelled:
		terminalPayload = &event.ActivityCancelled{ActivityID: "tool-call_1_0", PartialOutput: "partial"}
	}
	s, err = Apply(s, env(t, terminal, terminalPayload))
	if err != nil {
		t.Fatal(err)
	}
	_, still := s.Handles["call_1_0"]
	for _, m := range s.Conversation.Messages {
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "[background work call_1_0") {
				gotMessage = true
			}
		}
	}
	return gotMessage, !still
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBackgroundNotifyGate(t *testing.T) {
	cases := []struct {
		name     string
		notify   string
		terminal string
		isError  bool
		wantMsg  bool
	}{
		{"default always ok", "", event.TypeActivityCompleted, false, true},
		{"always error", "always", event.TypeActivityCompleted, true, true},
		{"none ok", "none", event.TypeActivityCompleted, false, false},
		{"none error", "none", event.TypeActivityCompleted, true, false},
		{"none failed", "none", event.TypeActivityFailed, false, false},
		{"on_fail ok suppressed", "on_fail", event.TypeActivityCompleted, false, false},
		{"on_fail error flows", "on_fail", event.TypeActivityCompleted, true, true},
		{"on_fail failed flows", "on_fail", event.TypeActivityFailed, false, true},
		{"unknown value falls back to always", "quiet", event.TypeActivityCompleted, false, true},
		// A kill is an explicit act — its partial output always renders,
		// the gate does not apply (INC-39 裁决).
		{"cancelled ignores none", "none", event.TypeActivityCancelled, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotMsg, handleGone := foldBackgroundOutcome(t, c.notify, c.terminal, c.isError)
			if gotMsg != c.wantMsg {
				t.Errorf("message flowed = %v, want %v", gotMsg, c.wantMsg)
			}
			if !handleGone {
				t.Error("terminal event must remove the handle regardless of notify")
			}
		})
	}
}

func TestBackgroundCompletedWithErrorRendersFailed(t *testing.T) {
	s := New()
	var err error
	if s, err = Apply(s, env(t, event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: "tool-call_1_0", Kind: event.KindTool, Name: "bash",
		Args:   json.RawMessage(`{"command":"false","background":true}`),
		CallID: "call_1_0", Attempt: 1, Background: true,
	})); err != nil {
		t.Fatal(err)
	}
	if s, err = Apply(s, env(t, event.TypeActivityCompleted, &event.ActivityCompleted{
		ActivityID: "tool-call_1_0", Result: json.RawMessage(`{"exit_code":1}`), IsError: true,
	})); err != nil {
		t.Fatal(err)
	}
	for _, m := range s.Conversation.Messages {
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "[background work call_1_0 failed]") {
				return
			}
		}
	}
	t.Fatalf("background error completion did not render failed: %+v", s.Conversation.Messages)
}
