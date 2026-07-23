package cli

import (
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

func TestPendingQueueExcludesCurrentTurnSteer(t *testing.T) {
	dir := t.TempDir()
	events, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	started, err := event.New(event.TypeSessionStarted, &event.SessionStarted{
		SubStateVersions: state.SubStateVersions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Append(started); err != nil {
		t.Fatal(err)
	}
	if err := events.Close(); err != nil {
		t.Fatal(err)
	}

	for _, input := range []protocol.UserInput{
		{Text: "legacy next turn"},
		{Text: "explicit next turn", Delivery: protocol.DeliveryQueue},
		{Text: "current turn", Delivery: protocol.DeliverySteer},
	} {
		if _, err := store.AppendCommand(dir, protocol.SessionCommand{
			CommandRef: protocol.CommandRef{CommandID: "cmd-" + input.Text},
			Kind:       protocol.CommandInput,
			Input:      &input,
		}); err != nil {
			t.Fatal(err)
		}
	}

	pending, _, err := pendingQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 || pending[0].Text != "legacy next turn" ||
		pending[1].Text != "explicit next turn" {
		t.Fatalf("pending = %+v, want only next-turn queue inputs", pending)
	}
}
