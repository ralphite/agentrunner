package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/config"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/workspace"
)

func newRememberWS(t *testing.T) *workspace.Workspace {
	t.Helper()
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func bashEffectFor(command string) pipeline.Effect {
	args, _ := json.Marshal(map[string]string{"command": command})
	return pipeline.Effect{ID: "eff-x", Kind: "tool_call", ToolName: "bash",
		Class: "execute", Args: args, CallID: "call_x"}
}

// rememberRule derives an EXACT allow rule from an approved effect.
func TestRememberRuleFromEffect(t *testing.T) {
	cases := []struct {
		name string
		req  event.ApprovalRequested
		want pipeline.PermissionRule
		ok   bool
	}{
		{"bash exact command",
			event.ApprovalRequested{ToolName: "bash", Args: json.RawMessage(`{"command":"npm test"}`)},
			pipeline.PermissionRule{Tool: "bash", Command: "npm test", Action: "allow"}, true},
		{"edit exact path",
			event.ApprovalRequested{ToolName: "edit_file", Args: json.RawMessage(`{"path":"src/a.go","old":"x","new":"y"}`)},
			pipeline.PermissionRule{Tool: "edit_file", Path: "src/a.go", Action: "allow"}, true},
		{"write exact path",
			event.ApprovalRequested{ToolName: "write_file", Args: json.RawMessage(`{"path":"out.txt"}`)},
			pipeline.PermissionRule{Tool: "write_file", Path: "out.txt", Action: "allow"}, true},
		{"bash without command not remembered",
			event.ApprovalRequested{ToolName: "bash", Args: json.RawMessage(`{}`)},
			pipeline.PermissionRule{}, false},
		{"unknown tool not remembered",
			event.ApprovalRequested{ToolName: "web_fetch", Args: json.RawMessage(`{"url":"http://x"}`)},
			pipeline.PermissionRule{}, false},
		{"no args not remembered",
			event.ApprovalRequested{ToolName: "bash"},
			pipeline.PermissionRule{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := rememberRule(tc.req)
			if ok != tc.ok || got != tc.want {
				t.Errorf("rememberRule = (%+v, %v), want (%+v, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

// config.AppendRule writes the rule, is idempotent, and preserves siblings.
func TestAppendRuleIdempotentAndPreserving(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")
	// Seed an existing hook so we can prove it survives.
	if err := os.WriteFile(path, []byte("hooks:\n  pre_tool: [\"echo hi\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rule := pipeline.PermissionRule{Tool: "bash", Command: "npm test", Action: "allow"}

	added, err := config.AppendRule(path, rule)
	if err != nil || !added {
		t.Fatalf("first append = (%v, %v), want (true, nil)", added, err)
	}
	// Idempotent second write.
	added, err = config.AppendRule(path, rule)
	if err != nil || added {
		t.Fatalf("second append = (%v, %v), want (false, nil) — idempotent", added, err)
	}
	// Reload and verify: the rule is there, and the pre-existing hook survived.
	s, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range s.Permissions {
		if r == rule {
			found = true
		}
	}
	if !found {
		t.Errorf("appended rule missing: %+v", s.Permissions)
	}
	if len(s.Hooks.PreTool) != 1 || s.Hooks.PreTool[0] != "echo hi" {
		t.Errorf("pre-existing hook clobbered: %+v", s.Hooks)
	}
	// And the rule count did not grow past one (no duplicate).
	n := 0
	for _, r := range s.Permissions {
		if r == rule {
			n++
		}
	}
	if n != 1 {
		t.Errorf("rule written %d times, want 1", n)
	}
}

// End-to-end of the "next session" contract: a remembered rule, once in the
// user config, makes the same command allow (no ask) when a fresh pipeline is
// built from that config. This is the whole point of INC-17.
func TestRememberedRuleAllowsNextSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")
	rule := pipeline.PermissionRule{Tool: "bash", Command: "npm test", Action: "allow"}
	if _, err := config.AppendRule(path, rule); err != nil {
		t.Fatal(err)
	}
	// Rebuild the merged config as a fresh session would, then adjudicate.
	user, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	merged := config.Merge(user, config.Settings{}, nil, false)
	ws := newRememberWS(t)
	g := &pipeline.PermissionGate{Rules: merged.Permissions, WS: ws}

	// The remembered exact command is now allowed without asking.
	if d := g.Check(context.Background(), bashEffectFor(`npm test`)); d.Action != event.VerdictAllow {
		t.Fatalf("remembered command = %+v, want allow (next session no longer asks)", d)
	}
	// A DIFFERENT command still falls to the default (exact match, not a glob).
	if d := g.Check(context.Background(), bashEffectFor(`npm run build`)); d.Action == event.VerdictAllow {
		t.Errorf("unrelated command wrongly allowed = %+v (exact match must not widen)", d)
	}
}
