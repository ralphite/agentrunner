package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

func journalTypes(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var types []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("bad journal line: %s", sc.Text())
		}
		types = append(types, rec.Type)
	}
	return types
}

// A provider error mid-run must surface wrapped with the turn number AND
// leave a terminal run_end{error} record (a failed journal must be
// distinguishable from a truncated one).
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
	journalPath := filepath.Join(t.TempDir(), "journal.jsonl")
	journal, err := store.OpenJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = journal.Close() }()

	loop := &Loop{
		Spec: &AgentSpec{Name: "t", Model: ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 10},
			SystemPrompt: "s", Tools: []string{"bash"}, MaxTurns: 5},
		Provider: scripted.New(fix),
		Exec:     &tool.Executor{WS: ws},
		Journal:  journal,
	}

	_, err = loop.Run(context.Background(), "loop until exhausted")
	if err == nil || !strings.Contains(err.Error(), "turn 2") {
		t.Fatalf("err = %v, want wrapped turn 2", err)
	}

	types := journalTypes(t, journalPath)
	if len(types) == 0 || types[len(types)-1] != "run_end" {
		t.Fatalf("journal types = %v, want terminal run_end", types)
	}
}

// A journal write failure aborts the run with an error (best-effort terminal
// record may also fail — the error must still propagate).
func TestLoopJournalWriteFailureAborts(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenJournal(filepath.Join(t.TempDir(), "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	_ = journal.Close() // every subsequent write fails

	loop := &Loop{
		Spec: &AgentSpec{Name: "t", Model: ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 10},
			SystemPrompt: "s", MaxTurns: 3},
		Provider: scripted.New(fix),
		Exec:     &tool.Executor{WS: ws},
		Journal:  journal,
	}
	if _, err := loop.Run(context.Background(), "hi"); err == nil {
		t.Fatal("expected error from failing journal writes")
	}
}
