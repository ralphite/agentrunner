package agent

import (
	"context"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

type captureSink struct {
	mu     sync.Mutex
	events []protocol.Event
}

func (c *captureSink) Emit(e protocol.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *captureSink) kinds() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, e := range c.events {
		out = append(out, string(e.Kind))
	}
	return out
}

// S4.1: assistant text streams as deltas (ephemeral) and the assembled
// message follows; tool call/result surface too.
func TestStreamingDeltasAndProtocol(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: "let me "}, {Text: "read that"},
			{ToolCall: &scripted.ToolCallEvent{Name: "read_file", Args: map[string]any{"path": "greet.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "greet.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := testLoop(t, fix, root)
	sink := &captureSink{}
	l.Out = sink

	if _, err := l.Run(context.Background(), "read the greeting"); err != nil {
		t.Fatal(err)
	}

	kinds := strings.Join(sink.kinds(), ",")
	for _, want := range []string{"session_start", "generation_start", "text_delta", "tool_call", "tool_result", "message", "run_end"} {
		if !strings.Contains(kinds, want) {
			t.Errorf("protocol stream missing %q: %s", want, kinds)
		}
	}

	// GenStep 1 streamed two deltas ("let me ", "read that") before its message.
	var t1deltas []string
	for _, e := range sink.events {
		if e.Kind == protocol.KindTextDelta && e.N == 1 {
			t1deltas = append(t1deltas, e.Text)
		}
	}
	if len(t1deltas) != 2 || t1deltas[0] != "let me " || t1deltas[1] != "read that" {
		t.Fatalf("turn-1 deltas = %v", t1deltas)
	}
}

// retryStreamProvider streams a delta then errs on attempt 1 (retryable),
// and succeeds on attempt 2 — exercising the GenerationDiscarded seam.
type retryStreamProvider struct{ attempt int }

func (p *retryStreamProvider) Capabilities() provider.Capabilities { return provider.Capabilities{} }
func (p *retryStreamProvider) Complete(_ context.Context, _ provider.CompleteRequest) iter.Seq2[provider.StreamEvent, error] {
	return func(yield func(provider.StreamEvent, error) bool) {
		p.attempt++
		if p.attempt == 1 {
			yield(provider.StreamEvent{Kind: provider.EventTextDelta, TextDelta: "partial..."}, nil)
			yield(provider.StreamEvent{}, errs.New(errs.ProviderServer, "503"))
			return
		}
		yield(provider.StreamEvent{Kind: provider.EventTextDelta, TextDelta: "final answer"}, nil)
		yield(provider.StreamEvent{Kind: provider.EventFinish, Finish: provider.FinishEndTurn}, nil)
	}
}

// alwaysErrProvider fails every attempt with a non-retryable class, so the
// turn fails immediately (no backoff) — modeling a provider error that
// outlasts the retries.
type alwaysErrProvider struct{}

func (p *alwaysErrProvider) Capabilities() provider.Capabilities { return provider.Capabilities{} }
func (p *alwaysErrProvider) Complete(_ context.Context, _ provider.CompleteRequest) iter.Seq2[provider.StreamEvent, error] {
	return func(yield func(provider.StreamEvent, error) bool) {
		yield(provider.StreamEvent{}, errs.New(errs.ProviderInvalid, "bad model"))
	}
}

// TestFailedTurnEmitsLiveError pins that a permanent turn failure reaches LIVE
// watchers as an error event, not just the journal: the stream used to go
// silent after generation_start and a reconnecting UI never learned the turn
// failed (QA Wave6, SSE-silent-on-failed-turn).
func TestFailedTurnEmitsLiveError(t *testing.T) {
	root := t.TempDir()
	l := testLoop(t, scripted.Fixture{}, root)
	l.Provider = &alwaysErrProvider{}
	sink := &captureSink{}
	l.Out = sink

	if _, err := l.Run(context.Background(), "do it"); err == nil {
		t.Fatal("expected a run error on a permanently failing turn")
	}
	var sawError bool
	for _, e := range sink.events {
		if e.Kind == protocol.KindError && strings.Contains(e.Text, "bad model") {
			sawError = true
		}
	}
	if !sawError {
		t.Fatalf("no live error event for the failed turn; kinds = %s", strings.Join(sink.kinds(), ","))
	}
}

// S4.1: an LLM retry after deltas already streamed emits a GenerationDiscarded
// event (durable) and a discard protocol event (surface reopen signal).
func TestGenerationDiscardedOnPartialStreamRetry(t *testing.T) {
	root := t.TempDir()
	l := testLoop(t, scripted.Fixture{}, root)
	l.Provider = &retryStreamProvider{}
	l.Clock = clock.Real{} // real backoff (one ~1s wait) — FakeClock would block
	sink := &captureSink{}
	l.Out = sink

	res, err := l.Run(context.Background(), "answer me")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}

	// The discarded partial and the final both streamed; a discard sits between.
	kinds := strings.Join(sink.kinds(), ",")
	if !strings.Contains(kinds, "text_delta,discard,text_delta") {
		t.Fatalf("expected partial→discard→final, got %s", kinds)
	}

	// GenerationDiscarded is durable in the log.
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var sawDiscard bool
	for _, e := range events {
		if e.Type == event.TypeGenerationDiscarded {
			sawDiscard = true
		}
	}
	if !sawDiscard {
		t.Fatal("generation_discarded not journaled")
	}
	// The final assembled message is the clean one (no "partial...").
	final, _ := state.Fold(events)
	last := final.Conversation.Messages[len(final.Conversation.Messages)-1]
	if last.Parts[0].Text != "final answer" {
		t.Fatalf("final message = %q", last.Parts[0].Text)
	}
}

// TestGenerationStartCarriesInputSeqs pins the INC-73 anchor: the live
// KindGenerationStart of a turn names the input DeliverySeqs it consumed, and a
// tool-loop continuation step carries none (its owner carries over). This is
// what lets a concurrent follower render only its own turn.
func TestGenerationStartCarriesInputSeqs(t *testing.T) {
	// Turn 1 answers the opening prompt (seq 0) with a read_file tool loop (so
	// it spans two gen-steps in ONE turn); turn 2 answers a second input seq 7.
	if err := os.WriteFile(filepath.Join(t.TempDir(), "unused"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "read_file", Args: map[string]any{"path": "f.txt"}}},
			{Finish: "tool_use"}}},
		{Respond: []scripted.Event{{Text: "reply one"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "reply two"}, {Finish: "end_turn"}}},
	}}
	inputs := make(chan protocol.UserInput)
	l := testLoop(t, fix, root)
	l.UserInputs = inputs
	sink := &captureSink{}
	l.Out = sink
	go func() {
		waitAnswers(t, l.Store.Dir(), 2) // turn 1 (2 gen-steps) done
		inputs <- protocol.UserInput{Text: "second", DeliverySeq: 7}
		waitAnswers(t, l.Store.Dir(), 3)
		close(inputs)
	}()
	if _, err := l.Run(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}

	var starts [][]int64
	for _, e := range sink.events {
		if e.Kind == protocol.KindGenerationStart {
			starts = append(starts, e.InputSeqs)
		}
	}
	// Three gen-steps: turn1 step1 (opening prompt seq 0 → none), turn1 step2
	// (tool-loop continuation → none), turn2 (seq 7).
	if len(starts) != 3 {
		t.Fatalf("gen-starts = %d, want 3: %v", len(starts), starts)
	}
	if len(starts[0]) != 0 || len(starts[1]) != 0 {
		t.Fatalf("turn-1 steps should carry no input seqs: %v", starts[:2])
	}
	if len(starts[2]) != 1 || starts[2][0] != 7 {
		t.Fatalf("turn-2 gen-start InputSeqs = %v, want [7]", starts[2])
	}
}
