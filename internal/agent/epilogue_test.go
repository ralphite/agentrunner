package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
)

func instrumentEpilogue(t *testing.T, order *[]string, failAt string) {
	t.Helper()
	saved := make([]epilogueHook, len(epilogueSequence))
	copy(saved, epilogueSequence)
	t.Cleanup(func() { copy(epilogueSequence, saved) })
	for i := range epilogueSequence {
		name := epilogueSequence[i].name
		epilogueSequence[i].run = func(context.Context, *Loop, *driveState, AppendFunc, *string) error {
			*order = append(*order, name)
			if name == failAt {
				return errors.New(name + " exploded")
			}
			return nil
		}
	}
}

func foldingAppend(m *memAppend, ds *driveState) AppendFunc {
	return func(typ string, payload any) (event.Envelope, error) {
		env, err := m.append(typ, payload)
		if err != nil {
			return env, err
		}
		ds.s, err = state.Apply(ds.s, env)
		return env, err
	}
}

// The sequence is fixed: quiesce → auto_publish → barrier → task_completed.
func TestEpilogueSequenceOrder(t *testing.T) {
	var order []string
	instrumentEpilogue(t, &order, "")
	m := &memAppend{}
	ds := &driveState{s: state.New()}

	res, err := (&Loop{Spec: &AgentSpec{}}).runEpilogue(context.Background(), ds, foldingAppend(m, ds), "completed", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.GenSteps != 3 {
		t.Errorf("res = %+v", res)
	}
	want := []string{"quiesce", "auto_publish", "barrier"}
	if !equal(order, want) {
		t.Fatalf("hook order = %v, want %v", order, want)
	}
	if types := m.types(); !equal(types, []string{"task_completed"}) {
		t.Fatalf("events = %v", types)
	}
	if ds.s.Session.Status != state.StatusCompleted {
		t.Errorf("fold status = %q", ds.s.Session.Status)
	}
}

// A hook error aborts a normal ending before the terminal event…
func TestEpilogueHookErrorAbortsNormalEnding(t *testing.T) {
	var order []string
	instrumentEpilogue(t, &order, "auto_publish")
	m := &memAppend{}
	ds := &driveState{s: state.New()}

	_, err := (&Loop{Spec: &AgentSpec{}}).runEpilogue(context.Background(), ds, foldingAppend(m, ds), "completed", 1, false)
	if err == nil || !equal(order, []string{"quiesce", "auto_publish"}) {
		t.Fatalf("err = %v, order = %v", err, order)
	}
	if len(m.events) != 0 {
		t.Fatalf("terminal event written despite hook failure: %v", m.types())
	}
}

// …but a best-effort (abort-path) ending presses on and still journals.
func TestEpilogueBestEffortPressesOn(t *testing.T) {
	var order []string
	instrumentEpilogue(t, &order, "quiesce")
	m := &memAppend{}
	ds := &driveState{s: state.New()}

	res, err := (&Loop{Spec: &AgentSpec{}}).runEpilogue(context.Background(), ds, foldingAppend(m, ds), "error", 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "error" {
		t.Errorf("res = %+v", res)
	}
	if !equal(order, []string{"quiesce", "auto_publish", "barrier"}) {
		t.Fatalf("order = %v", order)
	}
	if types := m.types(); !equal(types, []string{"task_completed"}) {
		t.Fatalf("events = %v", types)
	}
}
