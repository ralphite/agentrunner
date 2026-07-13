package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// controlLoop builds a live conversational Loop over a scripted fixture with
// inbox + controls channels wired, and starts it. Returns the store, the two
// channels, and a done channel carrying Run's error.
func controlLoop(t *testing.T, fix scripted.Fixture, maxSteps int) (*store.EventStore, chan protocol.UserInput, chan protocol.Control, chan error) {
	t.Helper()
	ws, err := workspace.New(t.TempDir())
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
			Name:               "ctl",
			Model:              ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			Tools:              []string{"bash"},
			MaxGenerationSteps: maxSteps,
		},
		Provider:   scripted.New(fix),
		Exec:       &tool.Executor{WS: ws},
		Store:      es,
		Clock:      clock.NewFake(time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)),
		SessionID:  "ctl-sess",
		UserInputs: inbox,
		Controls:   controls,
	}
	done := make(chan error, 1)
	go func() { _, e := l.Run(context.Background(), "first question"); done <- e }()
	return es, inbox, controls, done
}

func waitForEvent(t *testing.T, es *store.EventStore, typ string, min int) {
	t.Helper()
	// These are live loop tests, not fake-clock unit tests: under a full
	// package run the verifier and journal goroutines can legitimately be
	// descheduled for several seconds. Keep the bound finite, but do not turn
	// host contention into a false product failure.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		evs, _ := store.ReadEvents(es.Dir())
		n := 0
		for _, e := range evs {
			if e.Type == typ {
				n++
			}
		}
		if n >= min {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d %s events", min, typ)
}

// A manual compact at idle runs the summarizer and journals ContextCompacted;
// the session stays parked (no turn is started by a compact).
func TestManualCompactControl(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answer one"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "SUMMARY of the conversation"}, {Finish: "end_turn"}}}, // summarizer
	}}
	es, inbox, controls, done := controlLoop(t, fix, 5)

	waitForEvent(t, es, event.TypeAssistantMessage, 1) // turn 1 done, now idle
	controls <- protocol.Control{Kind: protocol.ControlCompact, Directive: "keep the decisions"}
	waitForEvent(t, es, event.TypeContextCompacted, 1)

	close(inbox) // graceful close
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// Exactly one compaction, carrying the summarizer's text; no turn 2 ran.
	if n := countEvents(t, es.Dir(), event.TypeContextCompacted); n != 1 {
		t.Fatalf("ContextCompacted count = %d, want 1", n)
	}
	evs, _ := store.ReadEvents(es.Dir())
	var summary string
	var cleared bool
	gens := 0
	for _, e := range evs {
		switch e.Type {
		case event.TypeContextCompacted:
			dec, _ := event.DecodePayload(e)
			cc := dec.(*event.ContextCompacted)
			summary, cleared = cc.Summary, cc.Cleared
		case event.TypeGenerationStarted:
			gens++
		}
	}
	if summary == "" || cleared {
		t.Fatalf("manual compact should carry a summary and not be cleared: summary=%q cleared=%v", summary, cleared)
	}
	if gens != 1 {
		t.Fatalf("generation_started = %d, want 1 (compact must not start a turn)", gens)
	}
}

// An empty summarizer reply must NOT journal a compaction — that would drop
// the whole context prefix. The guard keeps the context and skips (regression
// for the idle-compact-on-Gemini empty-reply defect, caught in real-API QA).
func TestManualCompactEmptySummarySkipped(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answer one"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "   \n  "}, {Finish: "end_turn"}}}, // whitespace-only summary
	}}
	es, inbox, controls, done := controlLoop(t, fix, 5)

	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- protocol.Control{Kind: protocol.ControlCompact}
	// Let the compact attempt run and be skipped.
	time.Sleep(200 * time.Millisecond)

	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, es.Dir(), event.TypeContextCompacted); n != 0 {
		t.Fatalf("empty summary should NOT journal a compaction, got %d", n)
	}
}

// A /clear drops the context with an EMPTY summary and NO summarizer call; a
// second clear with nothing new is a no-op (the guard).
func TestManualClearControl(t *testing.T) {
	// Only one provider step: turn 1. Clear never calls the provider — a
	// second step would mean an unexpected LLM call.
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answer one"}, {Finish: "end_turn"}}},
	}}
	es, inbox, controls, done := controlLoop(t, fix, 5)

	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	// Two clears queued together: the second is a no-op (nothing new).
	controls <- protocol.Control{CommandRef: protocol.CommandRef{CommandID: "cmd-clear-1", CommandSeq: 1}, Kind: protocol.ControlClear}
	controls <- protocol.Control{CommandRef: protocol.CommandRef{CommandID: "cmd-clear-2", CommandSeq: 2}, Kind: protocol.ControlClear}
	waitForEvent(t, es, event.TypeContextCompacted, 1)
	// Give the loop a moment in case a spurious second event were coming.
	time.Sleep(100 * time.Millisecond)

	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if n := countEvents(t, es.Dir(), event.TypeContextCompacted); n != 1 {
		t.Fatalf("clear events = %d, want exactly 1 (second clear is a no-op)", n)
	}
	evs, _ := store.ReadEvents(es.Dir())
	var sawFirst, sawNoop bool
	for _, e := range evs {
		if e.Type == event.TypeContextCompacted {
			sawFirst = e.CommandID == "cmd-clear-1"
			dec, _ := event.DecodePayload(e)
			cc := dec.(*event.ContextCompacted)
			if !cc.Cleared || cc.Summary != "" {
				t.Fatalf("clear event should be Cleared with empty summary: %+v", cc)
			}
		} else if e.Type == event.TypeCommandHandled && e.CommandID == "cmd-clear-2" {
			sawNoop = true
		}
	}
	if !sawFirst || !sawNoop {
		t.Fatalf("durable control receipts: first=%v noop=%v", sawFirst, sawNoop)
	}
}

func TestDurableCloseControlCarriesReceipt(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answer"}, {Finish: "end_turn"}}},
	}}
	es, _, controls, done := controlLoop(t, fix, 5)
	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- protocol.Control{
		CommandRef: protocol.CommandRef{CommandID: "cmd-close", CommandSeq: 9},
		Kind:       protocol.ControlClose,
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(es.Dir())
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range events {
		if env.Type == event.TypeSessionClosed {
			if env.CommandID != "cmd-close" {
				t.Fatalf("close command receipt = %q", env.CommandID)
			}
			return
		}
	}
	t.Fatal("session_closed not journaled")
}

// A manual compact over a conversation that carries an image must inflate
// the blob for the summarizer call exactly like a turn's own call — it used
// to send a bare CAS ref, the provider refused the whole request, and the
// compact silently failed (QA Round1 F-A03).
func TestManualCompactInflatesImageParts(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hello"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "看到图了"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "SUMMARY covering the screenshot"}, {Finish: "end_turn"}}}, // summarizer
	}}
	cap := &capturingProvider{inner: scripted.New(fix)}
	inputs := make(chan protocol.UserInput, 1)
	controls := make(chan protocol.Control, 1)
	l := testLoop(t, fix, t.TempDir())
	l.Provider = cap
	l.UserInputs = inputs
	l.Controls = controls
	png := []byte("\x89PNG compact bytes")
	go func() {
		waitAnswers(t, l.Store.Dir(), 1)
		inputs <- protocol.UserInput{Text: "这是截图",
			Images: []protocol.ImageAttachment{{MediaType: "image/png", Data: png}}}
		waitAnswers(t, l.Store.Dir(), 2)
		controls <- protocol.Control{Kind: protocol.ControlCompact}
		waitForEvent(t, l.Store, event.TypeContextCompacted, 1)
		close(inputs)
	}()
	if _, err := l.Run(context.Background(), "先聊聊"); err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, l.Store.Dir(), event.TypeContextCompacted); n != 1 {
		t.Fatalf("ContextCompacted count = %d, want 1 (the compact must succeed)", n)
	}
	// The summarizer request (the last one) carried the INFLATED image part.
	requests := cap.Requests()
	last := requests[len(requests)-1]
	var sawInflated bool
	for _, m := range last.Messages {
		for _, p := range m.Parts {
			if p.Kind == provider.PartImage && len(p.Data) > 0 {
				sawInflated = true
			}
		}
	}
	if !sawInflated {
		t.Fatal("summarizer request lacks the inflated image part (bare ref would be refused by the provider)")
	}
}
