package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
)

func instrumentQuiescent(t *testing.T, order *[]string, failAt string) {
	t.Helper()
	saved := make([]quiescentHook, len(quiescentSequence))
	copy(saved, quiescentSequence)
	t.Cleanup(func() { copy(quiescentSequence, saved) })
	for i := range quiescentSequence {
		name := quiescentSequence[i].name
		quiescentSequence[i].run = func(context.Context, *Loop, *driveState, AppendFunc, *string) error {
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

// The quiescent-actions sequence is fixed: auto_publish → barrier →
// goal_verify — and it journals NO terminal fact when no goal is attached
// (决策 #31: quiescence is a shape).
func TestQuiescentSequenceOrder(t *testing.T) {
	var order []string
	instrumentQuiescent(t, &order, "")
	m := &memAppend{}
	ds := &driveState{s: state.New()}

	reason := "completed"
	if err := (&Loop{Spec: &AgentSpec{}}).quiescentActions(context.Background(), ds, foldingAppend(m, ds), &reason); err != nil {
		t.Fatal(err)
	}
	want := []string{"auto_publish", "barrier", "goal_verify"}
	if !equal(order, want) {
		t.Fatalf("hook order = %v, want %v", order, want)
	}
	if len(m.events) != 0 {
		t.Fatalf("quiescent actions journaled a terminal fact: %v", m.types())
	}
}

// A slot error surfaces to the caller and stops the sequence.
func TestQuiescentHookErrorStopsSequence(t *testing.T) {
	var order []string
	instrumentQuiescent(t, &order, "auto_publish")
	m := &memAppend{}
	ds := &driveState{s: state.New()}

	reason := "completed"
	err := (&Loop{Spec: &AgentSpec{}}).quiescentActions(context.Background(), ds, foldingAppend(m, ds), &reason)
	if err == nil || !equal(order, []string{"auto_publish"}) {
		t.Fatalf("err = %v, order = %v", err, order)
	}
	if len(m.events) != 0 {
		t.Fatalf("event written despite hook failure: %v", m.types())
	}
}

// closeSession journals exactly the close MARK, with the user origin.
func TestCloseSessionJournalsMark(t *testing.T) {
	m := &memAppend{}
	ds := &driveState{s: state.New()}

	res, err := (&Loop{Spec: &AgentSpec{}}).closeSession(context.Background(), ds, foldingAppend(m, ds), 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" {
		t.Errorf("res = %+v", res)
	}
	if types := m.types(); !equal(types, []string{"session_closed"}) {
		t.Fatalf("events = %v", types)
	}
	if ds.s.Session.Closed == nil || ds.s.Session.Closed.Reason != "closed" || ds.s.Session.Closed.Source != "user" {
		t.Errorf("mark = %+v", ds.s.Session.Closed)
	}
}
