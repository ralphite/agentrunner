package daemon

import (
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/store"
)

// journalFor writes the given (type,payload) events to a fresh store and
// returns its dir, so a replay projection can be asserted end-to-end.
func journalFor(t *testing.T, events ...struct {
	typ string
	p   any
}) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "sess")
	s, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		env, err := event.New(e.typ, e.p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Append(env); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.Close()
	return dir
}

// TestReplayProjectsUserInput pins that attach replay renders the user's half
// of the conversation, not only the assistant's answers (QA Wave1 cli-life-02).
func TestReplayProjectsUserInput(t *testing.T) {
	dir := journalFor(t,
		struct {
			typ string
			p   any
		}{event.TypeSessionStarted, &event.SessionStarted{}},
		struct {
			typ string
			p   any
		}{event.TypeInputReceived, &event.InputReceived{Text: "hello agent", Source: "cli"}},
	)
	sink := &captureSink{}
	if err := ReplayJournal(dir, sink); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range sink.events {
		if e.Kind == protocol.KindUserInput && e.Text == "hello agent" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no user_input event projected; got %+v", sink.events)
	}
}

// TestReplayProjectsNonToolFailure pins that a final non-tool (LLM) failure
// surfaces as an error event instead of replaying as silence (QA Wave1
// carol-04, Wave2 carol-07/grace-03).
func TestReplayProjectsNonToolFailure(t *testing.T) {
	dir := journalFor(t,
		struct {
			typ string
			p   any
		}{event.TypeSessionStarted, &event.SessionStarted{}},
		struct {
			typ string
			p   any
		}{event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}},
		struct {
			typ string
			p   any
		}{event.TypeActivityFailed, &event.ActivityFailed{
			ActivityID: "llm-t1",
			Error:      event.ErrorInfo{Class: "provider_server", Message: "500 boom"},
			Final:      true,
		}},
	)
	sink := &captureSink{}
	if err := ReplayJournal(dir, sink); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range sink.events {
		if e.Kind == protocol.KindError && e.Text != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("non-tool failure did not project an error event; got %+v", sink.events)
	}
}

// TestReplayProjectsBudgetDenial pins that a budget-truncated turn surfaces its
// reason rather than a blank gen-step (QA Wave2 heidi-05/carol-06).
func TestReplayProjectsBudgetDenial(t *testing.T) {
	dir := journalFor(t,
		struct {
			typ string
			p   any
		}{event.TypeSessionStarted, &event.SessionStarted{}},
		struct {
			typ string
			p   any
		}{event.TypeLimitExceeded, &event.LimitExceeded{Kind: "tokens", Limit: 120, Used: 28}},
	)
	sink := &captureSink{}
	if err := ReplayJournal(dir, sink); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range sink.events {
		if e.Kind == protocol.KindError && e.Text != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("budget denial did not project a reason; got %+v", sink.events)
	}
}
