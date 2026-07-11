package daemon

import (
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/store"
)

// TestReplayProjectsModeChanged pins the attach projection for INC-42: a
// journaled user mode switch must reach a (re)attaching client as a
// mode_changed protocol event carrying the new mode, so the UI's permission
// pill reflects the live mode without private state.
func TestReplayProjectsModeChanged(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	s, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	appendEv := func(typ string, payload any) {
		t.Helper()
		env, err := event.New(typ, payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Append(env); err != nil {
			t.Fatal(err)
		}
	}
	appendEv(event.TypeSessionStarted, &event.SessionStarted{})
	appendEv(event.TypeModeChanged, &event.ModeChanged{To: "acceptEdits", Cause: "user"})
	_ = s.Close()

	sink := &captureSink{}
	if err := ReplayJournal(dir, sink); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range sink.events {
		if e.Kind == protocol.KindModeChanged {
			found = true
			if e.Mode != "acceptEdits" {
				t.Errorf("Mode = %q, want acceptEdits", e.Mode)
			}
		}
	}
	if !found {
		t.Fatal("no mode_changed event projected")
	}
}
