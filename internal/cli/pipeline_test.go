package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// buildPipelineFromLayers rebuilds one chained gate per journaled layer: the
// intersection semantics (every layer must allow) survive a standalone
// resume — a deny in EITHER layer still denies, and a flat merge would not
// preserve that under first-match.
func TestBuildPipelineFromLayers(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	layers := [][]pipeline.PermissionRule{
		{{Tool: "edit_file", Action: "deny"}, {Action: "allow"}}, // parent
		{{Tool: "bash", Action: "deny"}, {Action: "allow"}},      // child
	}
	var errOut bytes.Buffer
	pipe, _, err := buildPipelineFromLayers(ws, layers, "", 0, &errOut)
	if err != nil {
		t.Fatal(err)
	}

	verdict := func(toolName, class string) string {
		t.Helper()
		args, _ := json.Marshal(map[string]string{})
		out, err := pipe.Evaluate(context.Background(), pipeline.Effect{
			ID: "eff-x", Kind: "tool_call", ToolName: toolName, Class: class,
			CallID: "x", Args: args,
		})
		if err != nil {
			t.Fatal(err)
		}
		return out.Verdict
	}

	// The parent layer's deny binds even though the child layer allows it.
	if got := verdict("edit_file", "edit"); got != event.VerdictDeny {
		t.Errorf("edit_file = %s, want deny (parent layer)", got)
	}
	// The child layer's deny binds even though the parent layer allows it —
	// a flat first-match merge would have returned the parent's allow.
	if got := verdict("bash", "execute"); got != event.VerdictDeny {
		t.Errorf("bash = %s, want deny (child layer)", got)
	}
	// Both layers allow reads.
	if got := verdict("read_file", "read"); got != event.VerdictAllow {
		t.Errorf("read_file = %s, want allow", got)
	}
}
