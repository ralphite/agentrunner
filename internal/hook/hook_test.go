package hook

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
)

func TestPreHookProtocol(t *testing.T) {
	dir := t.TempDir()
	r := &Runner{Dir: dir}
	in := PreInput{ToolName: "bash", Class: "execute",
		Args: json.RawMessage(`{"command":"rm -rf /"}`), CallID: "call_1_0"}

	t.Run("exit 0 observes", func(t *testing.T) {
		r.PreTool = []string{"cat > observed.json"}
		res := r.RunPre(context.Background(), in)
		if res.Blocked || len(res.Notes) != 0 {
			t.Fatalf("res = %+v", res)
		}
		raw, err := os.ReadFile(filepath.Join(dir, "observed.json"))
		if err != nil || !strings.Contains(string(raw), `"tool_name":"bash"`) {
			t.Fatalf("hook stdin not delivered: %s (%v)", raw, err)
		}
	})

	t.Run("exit 2 blocks with stderr reason", func(t *testing.T) {
		r.PreTool = []string{`echo "destructive commands are forbidden" >&2; exit 2`}
		res := r.RunPre(context.Background(), in)
		if !res.Blocked || res.Reason != "destructive commands are forbidden" {
			t.Fatalf("res = %+v", res)
		}
	})

	t.Run("other exits observe with warning", func(t *testing.T) {
		r.PreTool = []string{"exit 3"}
		res := r.RunPre(context.Background(), in)
		if res.Blocked || len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "exit 3") {
			t.Fatalf("res = %+v", res)
		}
	})

	t.Run("first block wins", func(t *testing.T) {
		marker := filepath.Join(dir, "second-ran")
		r.PreTool = []string{"exit 2", "touch " + marker}
		res := r.RunPre(context.Background(), in)
		if !res.Blocked {
			t.Fatalf("res = %+v", res)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatal("hook after a block must not run")
		}
	})

	t.Run("timeout is a warning, not a veto", func(t *testing.T) {
		r.PreTool = []string{"sleep 5"}
		r.Timeout = 100 * time.Millisecond
		res := r.RunPre(context.Background(), in)
		if res.Blocked || len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "timed out") {
			t.Fatalf("res = %+v", res)
		}
		r.Timeout = 0
	})
}

func TestPostHookNotes(t *testing.T) {
	r := &Runner{Dir: t.TempDir(), PostTool: []string{
		`echo "lint clean"`,
		`exit 1`,
	}}
	notes := r.RunPost(context.Background(), PostInput{
		ToolName: "edit_file", CallID: "call_1_0",
		Result: json.RawMessage(`{"output":"edited"}`),
	})
	if len(notes) != 2 || notes[0] != "lint clean" || !strings.Contains(notes[1], "exit 1") {
		t.Fatalf("notes = %v", notes)
	}
}

func TestGateAdaptsPreHooks(t *testing.T) {
	g := &Gate{Runner: &Runner{Dir: t.TempDir(),
		PreTool: []string{`echo "no edits on fridays" >&2; exit 2`}}}
	if !g.SideEffecting() {
		t.Fatal("hook gate with pre hooks must declare side effects")
	}
	d := g.Check(context.Background(), pipeline.Effect{
		Kind: "tool_call", ToolName: "edit_file", Class: "edit"})
	if d.Action != event.VerdictDeny || d.Reason != "no edits on fridays" {
		t.Fatalf("decision = %+v", d)
	}

	// LLM effects and empty hook chains pass through.
	if d := g.Check(context.Background(), pipeline.Effect{Kind: "llm_call"}); d.Action != event.VerdictAllow {
		t.Fatalf("llm effect = %+v", d)
	}
	empty := &Gate{Runner: &Runner{}}
	if empty.SideEffecting() {
		t.Fatal("no pre hooks = no side effects")
	}
}
