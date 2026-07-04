package agent

import (
	"context"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
)

// countKind counts emitted protocol events of one kind (reuses the shared
// captureSink from stream_test.go).
func countKind(sink *captureSink, k protocol.Kind) int {
	n := 0
	for _, s := range sink.kinds() {
		if s == string(k) {
			n++
		}
	}
	return n
}

func countEvents(t *testing.T, dir, typ string) int {
	t.Helper()
	events, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range events {
		if e.Type == typ {
			n++
		}
	}
	return n
}

// S4.6: a malformed_tool_call finish records the fact and retries the SAME
// turn; a subsequent clean finish completes the run normally.
func TestMalformedToolCallRetriesThenSucceeds(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "calling..."}, {Finish: "malformed_tool_call"}}},
		{Respond: []scripted.Event{{Text: "recovered"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	res, err := l.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v, want completed after retry", res)
	}
	if n := countEvents(t, l.Store.Dir(), event.TypeMalformedToolCall); n != 1 {
		t.Errorf("malformed_tool_call events = %d, want 1", n)
	}
}

// S4.6: consecutive malformed finishes past the retry bound end the run with
// a user-visible error, not an infinite loop.
func TestMalformedToolCallExhaustionErrors(t *testing.T) {
	malformed := scripted.Step{Respond: []scripted.Event{{Text: "bad"}, {Finish: "malformed_tool_call"}}}
	fix := scripted.Fixture{Steps: []scripted.Step{malformed, malformed, malformed}}
	l := testLoop(t, fix, t.TempDir())
	sink := &captureSink{}
	l.Out = sink

	res, err := l.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "malformed_tool_call" {
		t.Fatalf("res = %+v, want reason malformed_tool_call", res)
	}
	// maxMalformedRetries = 2 → attempts 1,2,3 all malformed, then escalate.
	if n := countEvents(t, l.Store.Dir(), event.TypeMalformedToolCall); n != 3 {
		t.Errorf("malformed_tool_call events = %d, want 3", n)
	}
	if countKind(sink, protocol.KindError) != 1 {
		t.Errorf("expected exactly one user-visible error event")
	}
	// The run ended: a run_ended fact terminates the log.
	events, _ := store.ReadEvents(l.Store.Dir())
	if last := events[len(events)-1]; last.Type != event.TypeRunEnded {
		t.Errorf("last event = %s, want run_ended", last.Type)
	}
}

// S4.6: a blocked/safety finish surfaces a user-visible error and ends the
// run (reason "blocked"), preserving any assistant text.
func TestBlockedFinishEndsRun(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "I cannot help with that"}, {Finish: "blocked"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	sink := &captureSink{}
	l.Out = sink

	res, err := l.Run(context.Background(), "do something disallowed")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "blocked" {
		t.Fatalf("res = %+v, want blocked", res)
	}
	if countKind(sink, protocol.KindError) != 1 {
		t.Errorf("expected a user-visible error event")
	}
	// The assistant text was preserved as a durable message.
	if countKind(sink, protocol.KindMessage) != 1 {
		t.Errorf("assistant text should still be surfaced before the error")
	}
}

// S4.6: an empty candidate (no text, no tool calls) ends the run cleanly
// instead of spinning.
func TestEmptyCandidateEndsCleanly(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	res, err := l.Run(context.Background(), "say nothing")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.Turns != 1 {
		t.Fatalf("res = %+v, want a clean single-turn completion", res)
	}
}
