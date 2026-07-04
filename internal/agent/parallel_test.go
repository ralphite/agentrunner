package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// S4.3: the allow-verdict tool calls of ONE assistant turn execute
// concurrently. Three ~300ms sleeps finish in ~one sleep run concurrently,
// ~three run serially — the wall-clock is the proof.
func TestParallelToolCalls(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: "three at once"},
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "bash",
				Args: map[string]any{"command": "sleep 0.3; echo one"}}},
			{ToolCall: &scripted.ToolCallEvent{CallID: "c2", Name: "bash",
				Args: map[string]any{"command": "sleep 0.3; echo two"}}},
			{ToolCall: &scripted.ToolCallEvent{CallID: "c3", Name: "bash",
				Args: map[string]any{"command": "sleep 0.3; echo three"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "all done"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())

	start := time.Now()
	res, err := l.Run(context.Background(), "run three")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.Turns != 2 {
		t.Fatalf("res = %+v", res)
	}
	// Serial would be ~900ms; concurrent ~300ms. Generous ceiling for CI.
	if elapsed > 700*time.Millisecond {
		t.Errorf("elapsed %v — tool calls ran serially, not concurrently", elapsed)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"c1", "c2", "c3"} {
		tr, ok := fold.Conversation.ToolResults[id]
		if !ok {
			t.Errorf("no tool result for %s", id)
			continue
		}
		if tr.IsError {
			t.Errorf("%s errored: %s", id, tr.Result)
		}
	}
}

// S4.3: terminal events land in ARRIVAL order (whoever finishes first), but
// results are keyed by call_id in the fold — so the fast call journals its
// completion first even though it was issued last, and assembly still reads
// them back in the assistant message's call order.
func TestParallelToolArrivalOrder(t *testing.T) {
	// Issue order c1, c2, c3; finish order c2 (0.1s), c3 (0.3s), c1 (0.5s).
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "bash",
				Args: map[string]any{"command": "sleep 0.5; echo slow"}}},
			{ToolCall: &scripted.ToolCallEvent{CallID: "c2", Name: "bash",
				Args: map[string]any{"command": "sleep 0.1; echo fast"}}},
			{ToolCall: &scripted.ToolCallEvent{CallID: "c3", Name: "bash",
				Args: map[string]any{"command": "sleep 0.3; echo mid"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "ok"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	if _, err := l.Run(context.Background(), "stagger"); err != nil {
		t.Fatal(err)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	// Journal order of tool completions must follow FINISH time, not issue
	// order — the concurrent activities race to the single serialized append.
	var completedOrder []string
	for _, e := range events {
		if e.Type != event.TypeActivityCompleted {
			continue
		}
		dec, err := event.DecodePayload(e)
		if err != nil {
			t.Fatal(err)
		}
		id := dec.(*event.ActivityCompleted).ActivityID
		if strings.HasPrefix(id, "tool-") {
			completedOrder = append(completedOrder, strings.TrimPrefix(id, "tool-"))
		}
	}
	if want := []string{"c2", "c3", "c1"}; !equalStrings(completedOrder, want) {
		t.Errorf("completion (arrival) order = %v, want %v", completedOrder, want)
	}

	// Assembly reads results back in the assistant message's call order,
	// regardless of arrival order (fold ToolResults is a map keyed by call_id).
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	req := Assemble(fold, l.Spec, nil, 2)
	var toolResultOrder []string
	for _, m := range req.Messages {
		for _, p := range m.Parts {
			if p.Kind == provider.PartToolResult {
				toolResultOrder = append(toolResultOrder, p.CallID)
			}
		}
	}
	if want := []string{"c1", "c2", "c3"}; !equalStrings(toolResultOrder, want) {
		t.Errorf("assembled tool-result order = %v, want issue order %v", toolResultOrder, want)
	}
}

// S4.3 / 3.7d under REAL parallelism: adjudication is serialized (asks and
// reservations happen one at a time before any execution), so reserve-then-
// settle cannot double-commit. A turn issues three execute calls (2000 each)
// against a budget that affords two — the third is denied, and the run never
// overspends.
func TestParallelToolBudgetNoOverspend(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "b1", Name: "bash",
				Args: map[string]any{"command": "echo one"}}},
			{ToolCall: &scripted.ToolCallEvent{CallID: "b2", Name: "bash",
				Args: map[string]any{"command": "echo two"}}},
			{ToolCall: &scripted.ToolCallEvent{CallID: "b3", Name: "bash",
				Args: map[string]any{"command": "echo three"}}},
			{Usage: &scripted.UsageEvent{InputTokens: 60, OutputTokens: 40}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.Model.MaxTokens = 100
	l.Spec.Budget.MaxTotalTokens = 5000
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		&pipeline.BudgetGate{MaxTotalTokens: 5000},
	}}

	if _, err := l.Run(context.Background(), "spend it"); err != nil {
		t.Fatal(err)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	// LLM settled 100; b1 (2000) + b2 (2000) fit under 5000; b3 would reach
	// 6100 and is denied. Exactly two executes were allowed.
	deniedCount := 0
	for _, id := range []string{"b1", "b2", "b3"} {
		tr, ok := fold.Conversation.ToolResults[id]
		if !ok {
			t.Errorf("no result for %s", id)
			continue
		}
		if strings.Contains(string(tr.Result), "denied") {
			deniedCount++
		}
	}
	if deniedCount != 1 {
		t.Errorf("denied %d of 3 execute calls, want exactly 1 (2000+2000 fit, third overflows 5000)", deniedCount)
	}

	// Peak reservation never exceeded the budget: replay the fold and assert
	// settled+reserved stayed within 5000 at every step.
	s := state.New()
	for _, e := range events {
		s, err = state.Apply(s, e)
		if err != nil {
			t.Fatal(err)
		}
		if peak := s.Run.Usage.Billed() + s.Budget.ReservedTotal(); peak > 5000 {
			t.Fatalf("overspent: settled+reserved = %d > 5000 after %s", peak, e.Type)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
