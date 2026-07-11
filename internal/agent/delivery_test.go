package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
)

// INC-43: a send to a RUNNING session carries a per-message delivery mode.
// "steer" lands the message at the loop's next safe boundary WITHIN the current
// turn (the model sees it this turn); "queue" (default) holds it for the idle,
// so it enters the NEXT turn. These twins pin the timing by event sequence,
// mirroring TestReceiptsModeControlsSettlementTiming.
//
// Mid-turn injection is made deterministic with a file-gated foreground bash
// tool: the loop blocks inside the tool until the test drops a release file, so
// the injected input is guaranteed to be on the channel BEFORE the post-tool
// safe boundary runs.

// gatedTurn is a two-generation-step turn: step 1 runs a foreground bash tool
// that blocks on a release file; step 2 (after the tool result) generates the
// turn's final message. A third step is served only if a queued input starts a
// second turn.
func gatedTurn(gate string, finalMsg, nextTurnMsg string) scripted.Fixture {
	return scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "gate1", Name: "bash",
				Args: map[string]any{"command": "while [ ! -f " + gate + " ]; do sleep 0.01; done; echo gate-open"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: finalMsg}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: nextTurnMsg}, {Finish: "end_turn"}}},
	}}
}

// seqOf returns the seq of the first event of type typ whose payload contains
// substr (0 if none).
func seqOf(t *testing.T, dir, typ, substr string) int64 {
	t.Helper()
	evs, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Type == typ && (substr == "" || strings.Contains(string(e.Payload), substr)) {
			return e.Seq
		}
	}
	return 0
}

func TestSteerDeliversMidTurn(t *testing.T) {
	root := t.TempDir()
	gate := filepath.Join(root, "release")
	l := testLoop(t, gatedTurn(gate, "turn over", "second turn"), root)
	inputs := make(chan protocol.UserInput, 2)
	l.UserInputs = inputs
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Wait until the loop is blocked inside the gated tool, then inject a
		// steer message and release the tool. The steer must be consumed at the
		// post-tool safe boundary — mid-turn, before the turn's final generation.
		waitForEvent(t, l.Store, event.TypeActivityStarted, 1)
		inputs <- protocol.UserInput{Text: "STEER_ME", CommandID: "c-steer", DeliverySeq: 1,
			Delivery: protocol.DeliverySteer}
		if err := os.WriteFile(gate, []byte("go"), 0o644); err != nil {
			t.Error(err)
		}
		waitAnswers(t, l.Store.Dir(), 1) // turn ends → idle
		close(inputs)
	}()
	if _, err := l.Run(context.Background(), "do multi-step work"); err != nil {
		t.Fatal(err)
	}
	<-done

	steerSeq := seqOf(t, l.Store.Dir(), event.TypeInputReceived, "STEER_ME")
	finalSeq := seqOf(t, l.Store.Dir(), event.TypeAssistantMessage, "turn over")
	if steerSeq == 0 || finalSeq == 0 {
		t.Fatalf("missing anchors: steer=%d final=%d", steerSeq, finalSeq)
	}
	if steerSeq > finalSeq {
		t.Fatalf("steer landed at seq %d, AFTER the turn's final generation %d — it missed the in-turn boundary",
			steerSeq, finalSeq)
	}
	// The steered message must actually be in the model's context this turn:
	// no SECOND turn was started for it (that is the queue behavior).
	if s := seqOf(t, l.Store.Dir(), event.TypeAssistantMessage, "second turn"); s != 0 {
		t.Fatalf("a second turn ran at seq %d — steer should fold into the current turn, not start a new one", s)
	}
}

func TestQueueDefersToTurnEnd(t *testing.T) {
	for _, mode := range []string{protocol.DeliveryQueue, "" /* default */} {
		t.Run("mode="+mode, func(t *testing.T) {
			root := t.TempDir()
			gate := filepath.Join(root, "release")
			l := testLoop(t, gatedTurn(gate, "turn over", "queued reply"), root)
			inputs := make(chan protocol.UserInput, 2)
			l.UserInputs = inputs
			done := make(chan struct{})
			go func() {
				defer close(done)
				waitForEvent(t, l.Store, event.TypeActivityStarted, 1)
				inputs <- protocol.UserInput{Text: "QUEUE_ME", CommandID: "c-queue", DeliverySeq: 1,
					Delivery: mode}
				if err := os.WriteFile(gate, []byte("go"), 0o644); err != nil {
					t.Error(err)
				}
				waitAnswers(t, l.Store.Dir(), 2) // wait for the SECOND turn's answer
				close(inputs)
			}()
			if _, err := l.Run(context.Background(), "do multi-step work"); err != nil {
				t.Fatal(err)
			}
			<-done

			queueSeq := seqOf(t, l.Store.Dir(), event.TypeInputReceived, "QUEUE_ME")
			finalSeq := seqOf(t, l.Store.Dir(), event.TypeAssistantMessage, "turn over")
			if queueSeq == 0 || finalSeq == 0 {
				t.Fatalf("missing anchors: queue=%d final=%d", queueSeq, finalSeq)
			}
			if queueSeq < finalSeq {
				t.Fatalf("queue landed at seq %d, BEFORE the current turn's final generation %d — it steered instead of queuing",
					queueSeq, finalSeq)
			}
			// The queued message must get its OWN turn afterwards.
			if s := seqOf(t, l.Store.Dir(), event.TypeAssistantMessage, "queued reply"); s == 0 {
				t.Fatal("queued message never got its own turn")
			}
		})
	}
}

// TestSteerFlushesQueuedBacklog pins the seq-monotonicity rule: a queue message
// (lower seq) sent just before a steer message (higher seq) must NOT be dropped.
// The steer flushes the whole pending backlog in seq order, so both land this
// turn and ConsumedInputSeq (a high-water mark) never skips the lower seq.
func TestSteerFlushesQueuedBacklog(t *testing.T) {
	root := t.TempDir()
	gate := filepath.Join(root, "release")
	l := testLoop(t, gatedTurn(gate, "turn over", "unused"), root)
	inputs := make(chan protocol.UserInput, 3)
	l.UserInputs = inputs
	done := make(chan struct{})
	go func() {
		defer close(done)
		waitForEvent(t, l.Store, event.TypeActivityStarted, 1)
		// Queue arrives first (seq 1), steer second (seq 2). The steer must pull
		// the earlier queue forward, in order.
		inputs <- protocol.UserInput{Text: "EARLIER_QUEUE", CommandID: "c-q", DeliverySeq: 1,
			Delivery: protocol.DeliveryQueue}
		inputs <- protocol.UserInput{Text: "LATER_STEER", CommandID: "c-s", DeliverySeq: 2,
			Delivery: protocol.DeliverySteer}
		if err := os.WriteFile(gate, []byte("go"), 0o644); err != nil {
			t.Error(err)
		}
		waitAnswers(t, l.Store.Dir(), 1)
		close(inputs)
	}()
	if _, err := l.Run(context.Background(), "do multi-step work"); err != nil {
		t.Fatal(err)
	}
	<-done

	qSeq := seqOf(t, l.Store.Dir(), event.TypeInputReceived, "EARLIER_QUEUE")
	sSeq := seqOf(t, l.Store.Dir(), event.TypeInputReceived, "LATER_STEER")
	finalSeq := seqOf(t, l.Store.Dir(), event.TypeAssistantMessage, "turn over")
	if qSeq == 0 {
		t.Fatal("EARLIER_QUEUE was dropped — the steer flush skipped the lower seq (high-water hazard)")
	}
	if sSeq == 0 || finalSeq == 0 {
		t.Fatalf("missing anchors: steer=%d final=%d", sSeq, finalSeq)
	}
	if qSeq >= sSeq {
		t.Fatalf("backlog out of order: queue seq %d not before steer seq %d", qSeq, sSeq)
	}
	if sSeq > finalSeq {
		t.Fatalf("steer landed after the final generation (%d > %d): not mid-turn", sSeq, finalSeq)
	}
}
