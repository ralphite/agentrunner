package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

func eventTypes(t *testing.T, dir string) []string {
	t.Helper()
	events, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, e := range events {
		types = append(types, e.Type)
	}
	return types
}

// A provider error mid-run must surface wrapped with the turn number AND
// leave a terminal run_ended{error} event (a failed log must be
// distinguishable from a truncated one). The failure itself must be
// journaled as activity_failed before the terminal event.
func TestLoopProviderErrorWritesTerminalRecord(t *testing.T) {
	// One scripted step; the second Complete call exhausts the fixture.
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "bash", Args: map[string]any{"command": "true"}}},
			{Finish: "tool_use"},
		}},
	}}

	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sessDir := filepath.Join(t.TempDir(), "sess")
	es, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()

	loop := &Loop{
		Spec: &AgentSpec{Name: "t", Model: ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 10},
			SystemPrompt: "s", Tools: []string{"bash"}, MaxTurns: 5},
		Provider:  scripted.New(fix),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)),
		SessionID: "sess-err",
	}

	_, err = loop.Run(context.Background(), "loop until exhausted")
	if err == nil || !strings.Contains(err.Error(), "turn 2") {
		t.Fatalf("err = %v, want wrapped turn 2", err)
	}

	types := eventTypes(t, sessDir)
	if len(types) == 0 || types[len(types)-1] != event.TypeRunEnded {
		t.Fatalf("event types = %v, want terminal run_ended", types)
	}
	var sawFailed bool
	for _, typ := range types {
		if typ == event.TypeActivityFailed {
			sawFailed = true
		}
	}
	if !sawFailed {
		t.Errorf("event types = %v, want activity_failed for the exhausted provider call", types)
	}
}

// An event append failure aborts the run with an error (the best-effort
// terminal event may also fail — the error must still propagate).
func TestLoopEventAppendFailureAborts(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	_ = es.Close() // every subsequent append fails

	loop := &Loop{
		Spec: &AgentSpec{Name: "t", Model: ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 10},
			SystemPrompt: "s", MaxTurns: 3},
		Provider:  scripted.New(fix),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)),
		SessionID: "sess-closed",
	}
	if _, err := loop.Run(context.Background(), "hi"); err == nil {
		t.Fatal("expected error from failing event appends")
	}
}
