package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ralphite/agentrunner/internal/workspace"
)

// INC-43: write/edit results carry line-delta accounting.
func TestLineDeltaAccounting(t *testing.T) {
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &Executor{WS: ws}
	delta := func(res Result) (int, int) {
		t.Helper()
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Payload)
		}
		var d struct {
			Added   int `json:"lines_added"`
			Removed int `json:"lines_removed"`
		}
		if err := json.Unmarshal(res.Payload, &d); err != nil {
			t.Fatal(err)
		}
		return d.Added, d.Removed
	}

	// New file: all lines added (trailing newline does not inflate).
	a, r := delta(e.Execute(context.Background(), "write_file",
		json.RawMessage(`{"path":"n.txt","content":"一\n二\n三\n"}`)))
	if a != 3 || r != 0 {
		t.Fatalf("new file: want 3/0, got %d/%d", a, r)
	}
	// Overwrite counts as rewrite: old lines out, new lines in.
	a, r = delta(e.Execute(context.Background(), "write_file",
		json.RawMessage(`{"path":"n.txt","content":"only one line"}`)))
	if a != 1 || r != 3 {
		t.Fatalf("overwrite: want 1/3, got %d/%d", a, r)
	}
	// Edit replacement counts the edited span's lines.
	a, r = delta(e.Execute(context.Background(), "edit_file",
		json.RawMessage(`{"path":"n.txt","old":"only one line","new":"first\nsecond"}`)))
	if a != 2 || r != 1 {
		t.Fatalf("edit: want 2/1, got %d/%d", a, r)
	}
	// Create-via-edit (empty old).
	a, r = delta(e.Execute(context.Background(), "edit_file",
		json.RawMessage(`{"path":"m.txt","old":"","new":"x\ny"}`)))
	if a != 2 || r != 0 {
		t.Fatalf("create-via-edit: want 2/0, got %d/%d", a, r)
	}
}

func TestCountLines(t *testing.T) {
	for s, want := range map[string]int{"": 0, "a": 1, "a\n": 1, "a\nb": 2, "a\nb\n": 2, "\n": 1} {
		if got := countLines(s); got != want {
			t.Errorf("countLines(%q) = %d, want %d", s, got, want)
		}
	}
}
