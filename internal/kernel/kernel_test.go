package kernel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
)

func cmd(t *testing.T, id, target string) event.Envelope {
	t.Helper()
	env, err := event.New(event.TypeInputReceived, &event.InputReceived{Text: "x", Source: "test"})
	if err != nil {
		t.Fatal(err)
	}
	env.ID = id
	env.Target = target
	env.CorrelationID = "sess-1"
	return env
}

// A duplicate command (same envelope id) is processed exactly once.
// Mailboxes are FIFO, so once the marker command is recorded, both copies
// of the duplicate are already past the dedup gate.
func TestCommandDedup(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	got := make(chan string, 8)
	bus.Register("worker", func(_ context.Context, env event.Envelope) ([]event.Envelope, error) {
		got <- env.ID
		return nil, nil
	})

	dup := cmd(t, "cmd-aaaaaaaa", "worker")
	if err := bus.Send(dup); err != nil {
		t.Fatal(err)
	}
	if err := bus.Send(dup); err != nil {
		t.Fatal(err)
	}
	if err := bus.Send(cmd(t, "cmd-marker00", "worker")); err != nil {
		t.Fatal(err)
	}

	if id := <-got; id != "cmd-aaaaaaaa" {
		t.Fatalf("first processed = %q", id)
	}
	if id := <-got; id != "cmd-marker00" {
		t.Fatalf("second processed = %q, want marker (duplicate must be skipped)", id)
	}
}

// Children returned by a handler carry causation (parent id) and inherited
// correlation, across a two-actor hop.
func TestCausationCorrelationChain(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	seen := make(chan event.Envelope, 1)
	bus.Register("b", func(_ context.Context, env event.Envelope) ([]event.Envelope, error) {
		seen <- env
		return nil, nil
	})
	bus.Register("a", func(_ context.Context, _ event.Envelope) ([]event.Envelope, error) {
		child, err := event.New(event.TypeTurnStarted, &event.TurnStarted{Turn: 1})
		if err != nil {
			return nil, err
		}
		child.Target = "b"
		return []event.Envelope{child}, nil
	})

	if err := bus.Send(cmd(t, "cmd-root0000", "a")); err != nil {
		t.Fatal(err)
	}
	child := <-seen
	if child.CausationID != "cmd-root0000" {
		t.Errorf("causation = %q, want parent id", child.CausationID)
	}
	if child.CorrelationID != "sess-1" {
		t.Errorf("correlation = %q, want inherited sess-1", child.CorrelationID)
	}
}

// Publish fans out by type to subscribers; children with empty Target are
// published.
func TestPublishFanOut(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	got1 := make(chan string, 1)
	got2 := make(chan string, 1)
	bus.Register("sub1", func(_ context.Context, env event.Envelope) ([]event.Envelope, error) {
		got1 <- env.Type
		return nil, nil
	})
	bus.Register("sub2", func(_ context.Context, env event.Envelope) ([]event.Envelope, error) {
		got2 <- env.Type
		return nil, nil
	})
	bus.Subscribe("sub1", event.TypeTurnStarted)
	bus.Subscribe("sub2", event.TypeTurnStarted)

	env, err := event.New(event.TypeTurnStarted, &event.TurnStarted{Turn: 2})
	if err != nil {
		t.Fatal(err)
	}
	bus.Publish(env)
	if typ := <-got1; typ != event.TypeTurnStarted {
		t.Errorf("sub1 got %q", typ)
	}
	if typ := <-got2; typ != event.TypeTurnStarted {
		t.Errorf("sub2 got %q", typ)
	}
}

// A handler error crashes the actor: ActorCrashed is published as a child
// of the failing envelope, the actor is dead, and there is no restart.
func TestActorCrashNoRestart(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	crashes := make(chan event.Envelope, 1)
	bus.Register("watcher", func(_ context.Context, env event.Envelope) ([]event.Envelope, error) {
		crashes <- env
		return nil, nil
	})
	bus.Subscribe("watcher", event.TypeActorCrashed)
	bus.Register("fragile", func(_ context.Context, _ event.Envelope) ([]event.Envelope, error) {
		return nil, errors.New("boom")
	})

	if err := bus.Send(cmd(t, "cmd-kill0000", "fragile")); err != nil {
		t.Fatal(err)
	}
	crashed := <-crashes
	p, err := event.DecodePayload(crashed)
	if err != nil {
		t.Fatal(err)
	}
	info := p.(*event.ActorCrashed)
	if info.Actor != "fragile" || !strings.Contains(info.Error, "boom") {
		t.Errorf("crash info = %+v", info)
	}
	if crashed.CausationID != "cmd-kill0000" {
		t.Errorf("crash causation = %q, want failing envelope id", crashed.CausationID)
	}

	err = bus.Send(cmd(t, "cmd-next0000", "fragile"))
	if err == nil || !strings.Contains(err.Error(), "crashed") {
		t.Fatalf("send to dead actor: err = %v, want crashed error", err)
	}
}

// A panicking handler is a crash, not a process abort.
func TestActorPanicIsCrash(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	crashes := make(chan event.Envelope, 1)
	bus.Register("watcher", func(_ context.Context, env event.Envelope) ([]event.Envelope, error) {
		crashes <- env
		return nil, nil
	})
	bus.Subscribe("watcher", event.TypeActorCrashed)
	bus.Register("panicky", func(_ context.Context, _ event.Envelope) ([]event.Envelope, error) {
		panic("unhinged")
	})

	if err := bus.Send(cmd(t, "cmd-panic000", "panicky")); err != nil {
		t.Fatal(err)
	}
	crashed := <-crashes
	p, _ := event.DecodePayload(crashed)
	if info := p.(*event.ActorCrashed); !strings.Contains(info.Error, "unhinged") {
		t.Errorf("crash info = %+v", info)
	}
}

func TestSendErrors(t *testing.T) {
	bus := NewBus()
	if err := bus.Send(cmd(t, "cmd-x0000000", "nobody")); err == nil {
		t.Error("unknown target must error")
	}
	bus.Close()
	if err := bus.Send(cmd(t, "cmd-y0000000", "nobody")); err == nil {
		t.Error("send after close must error")
	}
}
