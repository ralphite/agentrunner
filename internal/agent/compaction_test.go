package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/state/statetest"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// S4.5: after a ContextCompacted, the assembled view is the summary plus
// everything AFTER the boundary — the pre-boundary messages are gone from
// the model's view, though the log still holds them.
func TestCompactionFoldView(t *testing.T) {
	s := state.New()
	s = mustApply(t, s, event.TypeRunStarted, &event.RunStarted{SubStateVersions: state.SubStateVersions()})
	s = mustApply(t, s, event.TypeInputReceived, &event.InputReceived{Text: "old question", Source: "cli"})
	s = mustApply(t, s, event.TypeAssistantMessage, &event.AssistantMessage{Turn: 1,
		Message: provider.Message{Role: provider.RoleAssistant,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "old answer"}}}})
	// Boundary lands here: 2 messages so far.
	s = mustApply(t, s, event.TypeContextCompacted, &event.ContextCompacted{
		UptoTurn: 1, Summary: "we discussed the old thing"})
	s = mustApply(t, s, event.TypeInputReceived, &event.InputReceived{Text: "new question", Source: "cli"})

	if s.Compaction.Boundary != 2 {
		t.Fatalf("boundary = %d, want 2", s.Compaction.Boundary)
	}
	msgs := assembleMessages(s)
	// Expect: [summary(user), new question(user)].
	if len(msgs) != 2 {
		t.Fatalf("assembled %d messages, want 2: %+v", len(msgs), msgs)
	}
	if !strings.Contains(msgs[0].Parts[0].Text, "we discussed the old thing") {
		t.Errorf("first message is not the summary: %q", msgs[0].Parts[0].Text)
	}
	joined := msgs[0].Parts[0].Text + msgs[1].Parts[0].Text
	if strings.Contains(joined, "old answer") || strings.Contains(joined, "old question") {
		t.Errorf("pre-boundary content leaked into view: %+v", msgs)
	}
	if msgs[1].Parts[0].Text != "new question" {
		t.Errorf("post-boundary message wrong: %q", msgs[1].Parts[0].Text)
	}
}

// S4.5: fold equivalence must hold ACROSS the compaction boundary — folding
// the whole log equals folding a snapshot taken mid-log (at or after the
// ContextCompacted) plus the remaining tail. The compacted view is a pure
// function of the events, so where you cut makes no difference.
func TestCompactionFoldEquivalence(t *testing.T) {
	events := []struct {
		typ     string
		payload any
	}{
		{event.TypeRunStarted, &event.RunStarted{SubStateVersions: state.SubStateVersions()}},
		{event.TypeInputReceived, &event.InputReceived{Text: "q1", Source: "cli"}},
		{event.TypeTurnStarted, &event.TurnStarted{Turn: 1}},
		{event.TypeAssistantMessage, &event.AssistantMessage{Turn: 1,
			Message: provider.Message{Role: provider.RoleAssistant,
				Parts: []provider.Part{{Kind: provider.PartText, Text: "a1"}}}}},
		{event.TypeContextCompacted, &event.ContextCompacted{UptoTurn: 1, Summary: "sum1"}},
		{event.TypeTurnStarted, &event.TurnStarted{Turn: 2}},
		{event.TypeInputReceived, &event.InputReceived{Text: "q2", Source: "cli"}},
		{event.TypeAssistantMessage, &event.AssistantMessage{Turn: 2,
			Message: provider.Message{Role: provider.RoleAssistant,
				Parts: []provider.Part{{Kind: provider.PartText, Text: "a2"}}}}},
	}

	// Full fold.
	full := state.New()
	for _, e := range events {
		full = mustApply(t, full, e.typ, e.payload)
	}

	// Cut at every seq, including right after the ContextCompacted, and fold
	// prefix→snapshot→tail. Each must equal the full fold.
	for cut := 1; cut <= len(events); cut++ {
		mid := state.New()
		for _, e := range events[:cut] {
			mid = mustApply(t, mid, e.typ, e.payload)
		}
		// mid stands in for a snapshot; continue with the tail.
		resumed := mid
		for _, e := range events[cut:] {
			resumed = mustApply(t, resumed, e.typ, e.payload)
		}
		statetest.AssertFoldEqual(t, resumed, full)
	}
}

// S4.5 end-to-end: with a low CompactAtTokens threshold and a bulky first
// turn, the loop compacts at the turn boundary — a ContextCompacted is
// journaled and turn 2's assembled request carries the summary, not the
// bulky original text.
func TestCompactionTriggeredInLoop(t *testing.T) {
	big := strings.Repeat("verbose reasoning that inflates the context. ", 200) // ~9KB
	// Turn 1 emits a bulky answer AND a tool call, so the loop advances to a
	// turn-2 boundary (the compaction point) instead of ending. Step 2 is the
	// summarizer call; step 3 is turn 2's LLM call, which sees the summary.
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: big},
			{ToolCall: &scripted.ToolCallEvent{Name: "bash", Args: map[string]any{"command": "echo hi"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "concise summary of it all"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}

	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })

	cap := &capturingProvider{inner: scripted.New(fix)}
	l := &Loop{
		Spec: &AgentSpec{
			Name:     "compact",
			Model:    ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100, CompactAtTokens: 500},
			Tools:    []string{"bash"},
			MaxTurns: 3,
		},
		Provider:  cap,
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)),
		SessionID: "compact-sess",
	}
	if _, err := l.Run(context.Background(), "please elaborate at length"); err != nil {
		t.Fatal(err)
	}

	events, err := store.ReadEvents(es.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var compacted *event.ContextCompacted
	for _, e := range events {
		if e.Type == event.TypeContextCompacted {
			dec, derr := event.DecodePayload(e)
			if derr != nil {
				t.Fatal(derr)
			}
			compacted = dec.(*event.ContextCompacted)
		}
	}
	if compacted == nil {
		t.Fatal("expected a ContextCompacted event to be journaled")
	}
	if !strings.Contains(compacted.Summary, "concise summary") {
		t.Errorf("summary = %q", compacted.Summary)
	}

	// Three provider calls: turn-1 LLM, the summarizer, turn-2 LLM. The
	// turn-2 request must carry the summary and NOT the bulky turn-1 text.
	if len(cap.requests) != 3 {
		t.Fatalf("provider calls = %d, want 3 (turn1, compact, turn2)", len(cap.requests))
	}
	turn2 := cap.requests[2]
	var seenSummary, seenBig bool
	for _, m := range turn2.Messages {
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "concise summary") {
				seenSummary = true
			}
			if strings.Contains(p.Text, "verbose reasoning that inflates") {
				seenBig = true
			}
		}
	}
	if !seenSummary {
		t.Errorf("turn-2 request missing the summary: %+v", turn2.Messages)
	}
	if seenBig {
		t.Errorf("turn-2 request still carries the pre-compaction bulk")
	}
}
