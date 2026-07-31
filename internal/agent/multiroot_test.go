package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// INC-105 multi-root: a session whose Workspace spans several roots reads and
// edits files in an EXTRA root with no approval, journals the full boundary
// into its genesis, and discloses every root in the frozen env block.
func TestMultiRootSessionReachesExtraRootAndJournalsBoundary(t *testing.T) {
	primary := t.TempDir()
	extra := t.TempDir()
	if err := os.WriteFile(filepath.Join(extra, "notes.md"), []byte("multi-root hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: "reading the extra root"},
			{ToolCall: &scripted.ToolCallEvent{Name: "read_file", Args: map[string]any{
				"path": filepath.Join(extra, "notes.md")}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, primary)
	ws, err := workspace.NewMultiRoot(primary, []string{extra})
	if err != nil {
		t.Fatal(err)
	}
	l.Exec.WS = ws

	if _, err := l.Run(context.Background(), "read the notes"); err != nil {
		t.Fatalf("run: %v", err)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var started *event.SessionStarted
	sawReadResult := false
	for _, e := range events {
		switch e.Type {
		case event.TypeSessionStarted:
			decoded, derr := event.DecodePayload(e)
			if derr != nil {
				t.Fatal(derr)
			}
			started = decoded.(*event.SessionStarted)
		case event.TypeActivityCompleted:
			if strings.Contains(string(e.Payload), "multi-root hello") {
				sawReadResult = true
			}
		}
	}
	if started == nil {
		t.Fatal("no SessionStarted")
	}
	// Genesis carries the full boundary, primary first — resume rebuilds the
	// same reach from this field alone.
	if len(started.WorkspaceRoots) != 2 || started.WorkspaceRoots[0] != started.WorkspaceRoot {
		t.Fatalf("WorkspaceRoots = %v (WorkspaceRoot %s)", started.WorkspaceRoots, started.WorkspaceRoot)
	}
	// The frozen env block discloses every root so the model knows its reach.
	if !strings.Contains(started.Env, "workspace roots (read-write") ||
		!strings.Contains(started.Env, started.WorkspaceRoots[1]) {
		t.Fatalf("env block must disclose extra roots: %q", started.Env)
	}
	// And the read actually succeeded — no approval, no escape error.
	if !sawReadResult {
		t.Fatalf("read_file in the extra root did not return its content")
	}
}

// A single-root genesis stays byte-shaped exactly as before INC-105: no
// workspace_roots key, no env drift — the frozen prefix of every existing
// journal must not shift.
func TestSingleRootGenesisUnchangedByMultiRoot(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	if _, err := l.Run(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Type != event.TypeSessionStarted {
			continue
		}
		if strings.Contains(string(e.Payload), "workspace_roots") {
			t.Fatalf("single-root genesis must not carry workspace_roots: %s", e.Payload)
		}
		if strings.Contains(string(e.Payload), "workspace roots (read-write") {
			t.Fatalf("single-root env must not disclose a roots line")
		}
		return
	}
	t.Fatal("no SessionStarted")
}
