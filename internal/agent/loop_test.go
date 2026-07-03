package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

func testLoop(t *testing.T, fix scripted.Fixture, root string) *Loop {
	t.Helper()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	j, err := store.OpenJournal(filepath.Join(t.TempDir(), "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return &Loop{
		Spec: &AgentSpec{
			Name:         "test",
			Model:        ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
			SystemPrompt: "be helpful",
			Tools:        []string{"read_file", "edit_file", "bash"},
			MaxTurns:     10,
		},
		Provider: scripted.New(fix),
		Exec:     &tool.Executor{WS: ws},
		Journal:  j,
	}
}

func TestLoopMultiTurnEditsFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "greet.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Turn 1: model reads, edits. Turn 2: model confirms and stops.
	fix := scripted.Fixture{Steps: []scripted.Step{
		{
			Expect: scripted.Expect{ToolsInclude: []string{"read_file"}, LastMessageContains: "make it loud"},
			Respond: []scripted.Event{
				{Text: "reading then editing"},
				{ToolCall: &scripted.ToolCallEvent{Name: "read_file", Args: map[string]any{"path": "greet.txt"}}},
				{ToolCall: &scripted.ToolCallEvent{Name: "edit_file", Args: map[string]any{
					"path": "greet.txt", "old": "hello world", "new": "HELLO WORLD"}}},
				{Usage: &scripted.UsageEvent{InputTokens: 10, OutputTokens: 5}},
				{Finish: "tool_use"},
			},
		},
		{
			Expect:  scripted.Expect{LastMessageContains: "edited greet.txt"},
			Respond: []scripted.Event{{Text: "done"}, {Usage: &scripted.UsageEvent{InputTokens: 8, OutputTokens: 2}}, {Finish: "end_turn"}},
		},
	}}

	l := testLoop(t, fix, root)
	res, err := l.Run(context.Background(), "make it loud")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.Turns != 2 {
		t.Errorf("result = %+v", res)
	}
	if res.Usage.InputTokens != 18 || res.Usage.OutputTokens != 7 {
		t.Errorf("usage = %+v", res.Usage)
	}

	got, _ := os.ReadFile(filepath.Join(root, "greet.txt"))
	if string(got) != "HELLO WORLD" {
		t.Errorf("file = %q, want HELLO WORLD", got)
	}
}

func TestLoopTextOnlyStops(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "no tools needed"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	res, err := l.Run(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.Turns != 1 {
		t.Errorf("result = %+v", res)
	}
}

func TestLoopToolErrorContinues(t *testing.T) {
	// The model calls read_file on a missing path; the error result feeds
	// back and the model recovers on the next turn.
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "read_file", Args: map[string]any{"path": "nope.txt"}}},
			{Finish: "tool_use"},
		}},
		{
			Expect:  scripted.Expect{LastMessageContains: "error"},
			Respond: []scripted.Event{{Text: "ah, missing"}, {Finish: "end_turn"}},
		},
	}}
	l := testLoop(t, fix, t.TempDir())
	res, err := l.Run(context.Background(), "read nope")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.Turns != 2 {
		t.Errorf("result = %+v", res)
	}
}

func TestLoopMaxTurns(t *testing.T) {
	// Model always calls a tool → never stops on its own.
	steps := make([]scripted.Step, 5)
	for i := range steps {
		steps[i] = scripted.Step{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "bash", Args: map[string]any{"command": "true"}}},
			{Finish: "tool_use"},
		}}
	}
	l := testLoop(t, scripted.Fixture{Steps: steps}, t.TempDir())
	l.Spec.MaxTurns = 3
	res, err := l.Run(context.Background(), "loop forever")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "max_turns" || res.Turns != 3 {
		t.Errorf("result = %+v", res)
	}
}
