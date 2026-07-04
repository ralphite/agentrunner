package pipeline

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/workspace"
)

func toolEffect(name, class string, args string) Effect {
	return Effect{ID: "eff-x", Kind: "tool_call", ToolName: name, Class: class,
		Args: json.RawMessage(args), CallID: "call_1_0"}
}

func newPermWS(t *testing.T) *workspace.Workspace {
	t.Helper()
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestPermissionRulesTable(t *testing.T) {
	ws := newPermWS(t)
	rules := []PermissionRule{
		{Tool: "bash", Command: "go test *", Action: "allow"},
		{Tool: "bash", Command: "rm *", Action: "deny"},
		{Tool: "edit_file", Path: "src/**", Action: "allow"},
		{Tool: "edit_file", Path: "*.md", Action: "allow"},
		{Tool: "read_file", Path: "secrets/*", Action: "deny"},
		{Action: "ask"}, // catch-all
	}
	g := &PermissionGate{Rules: rules, WS: ws}

	cases := []struct {
		name string
		eff  Effect
		want string
	}{
		{"command glob allow", toolEffect("bash", "execute", `{"command":"go test ./..."}`), event.VerdictAllow},
		{"command glob deny", toolEffect("bash", "execute", `{"command":"rm -rf build"}`), event.VerdictDeny},
		{"command falls to catch-all", toolEffect("bash", "execute", `{"command":"make lint"}`), event.VerdictAsk},
		{"doublestar path allow", toolEffect("edit_file", "edit", `{"path":"src/a/b/c.go","old":"x","new":"y"}`), event.VerdictAllow},
		{"singlestar no slash", toolEffect("edit_file", "edit", `{"path":"README.md"}`), event.VerdictAllow},
		{"singlestar rejects slash", toolEffect("edit_file", "edit", `{"path":"docs/x.md"}`), event.VerdictAsk},
		{"path deny", toolEffect("read_file", "read", `{"path":"secrets/key.pem"}`), event.VerdictDeny},
		{"llm effects pass through", Effect{ID: "eff-llm-t1", Kind: "llm_call"}, event.VerdictAllow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := g.Check(context.Background(), tc.eff); got.Action != tc.want {
				t.Errorf("decision = %+v, want %s", got, tc.want)
			}
		})
	}
}

// First match wins: an early ask shadows a later allow.
func TestPermissionFirstMatchWins(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Rules: []PermissionRule{
		{Tool: "bash", Command: "go *", Action: "ask"},
		{Tool: "bash", Command: "go test *", Action: "allow"},
	}}
	d := g.Check(context.Background(), toolEffect("bash", "execute", `{"command":"go test ./..."}`))
	if d.Action != event.VerdictAsk {
		t.Fatalf("decision = %+v, want first rule's ask", d)
	}
}

// The PLAN-mandated case: a traversal path is denied before any rule or
// mode default can apply — even in bypass mode.
func TestPermissionEscapeDeniedEvenInBypass(t *testing.T) {
	for _, mode := range []string{ModeDefault, ModeBypass, ModePlan, ModeAcceptEdits} {
		g := &PermissionGate{WS: newPermWS(t), Mode: mode, Rules: []PermissionRule{{Action: "allow"}}}
		d := g.Check(context.Background(), toolEffect("read_file", "read", `{"path":"src/../../etc/passwd"}`))
		if d.Action != event.VerdictDeny || !strings.Contains(d.Reason, "escapes workspace") {
			t.Errorf("mode %s: decision = %+v, want escape denial", mode, d)
		}
	}
}

// The no-rule-matched mode default table, every cell.
func TestPermissionModeDefaults(t *testing.T) {
	cases := []struct {
		mode, class string
		want        string
	}{
		{ModeDefault, "read", event.VerdictAllow},
		{ModeDefault, "edit", event.VerdictAsk},
		{ModeDefault, "execute", event.VerdictAsk},
		{ModeDefault, "wait", event.VerdictAllow},
		{ModePlan, "read", event.VerdictAllow},
		{ModePlan, "edit", event.VerdictDeny},
		{ModePlan, "execute", event.VerdictDeny},
		{ModePlan, "wait", event.VerdictAllow},
		{ModeAcceptEdits, "read", event.VerdictAllow},
		{ModeAcceptEdits, "edit", event.VerdictAllow},
		{ModeAcceptEdits, "execute", event.VerdictAsk},
		{ModeBypass, "read", event.VerdictAllow},
		{ModeBypass, "edit", event.VerdictAllow},
		{ModeBypass, "execute", event.VerdictAllow},
	}
	ws := newPermWS(t)
	for _, tc := range cases {
		t.Run(tc.mode+"/"+tc.class, func(t *testing.T) {
			g := &PermissionGate{Mode: tc.mode, WS: ws}
			d := g.Check(context.Background(), toolEffect("any_tool", tc.class, `{}`))
			if d.Action != tc.want {
				t.Errorf("decision = %+v, want %s", d, tc.want)
			}
		})
	}
}

// A rule with both path and command requires both to match.
func TestPermissionRuleConjunction(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Rules: []PermissionRule{
		{Tool: "bash", Command: "cat *", Path: "logs/**", Action: "allow"},
	}}
	// bash has no path arg → path clause cannot match → falls to default ask.
	d := g.Check(context.Background(), toolEffect("bash", "execute", `{"command":"cat logs/x"}`))
	if d.Action != event.VerdictAsk {
		t.Fatalf("decision = %+v", d)
	}
}

// Empty tool matches any tool.
func TestPermissionEmptyToolMatchesAny(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Rules: []PermissionRule{{Action: "deny"}}}
	d := g.Check(context.Background(), toolEffect("read_file", "read", `{"path":"a.txt"}`))
	if d.Action != event.VerdictDeny {
		t.Fatalf("decision = %+v", d)
	}
}

// Security review: a command deny rule must not be evadable by putting the
// dangerous part on a second line (regex `.` must cross newlines).
func TestCommandDenyResistsNewlineEvasion(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Rules: []PermissionRule{
		{Tool: "bash", Command: "*rm -rf*", Action: "deny"},
	}}
	d := g.Check(context.Background(), toolEffect("bash", "execute",
		`{"command":"git status\nrm -rf /"}`))
	if d.Action != event.VerdictDeny {
		t.Fatalf("newline-hidden command evaded deny: %+v", d)
	}
}

// Security review: plan mode's edit/execute prohibition cannot be overridden
// by an allow rule (the hard floor precedes the rule list).
func TestPlanModeDenyUnbypassableByRule(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Mode: ModePlan, Rules: []PermissionRule{
		{Tool: "edit_file", Action: "allow"},
		{Tool: "bash", Action: "allow"},
	}}
	for _, tc := range []struct{ tool, class string }{
		{"edit_file", "edit"}, {"bash", "execute"},
	} {
		d := g.Check(context.Background(), toolEffect(tc.tool, tc.class, `{"path":"a.txt"}`))
		if d.Action != event.VerdictDeny {
			t.Errorf("plan mode %s allowed by rule: %+v", tc.tool, d)
		}
	}
	// exit_plan_mode is exempt (the sanctioned way out).
	if d := g.Check(context.Background(), toolEffect("exit_plan_mode", "wait", `{}`)); d.Action != event.VerdictAsk {
		t.Errorf("exit_plan_mode in plan mode = %+v, want ask", d)
	}
}

// FloorGate short-circuits hard denials before any later (side-effecting) gate.
func TestFloorGatePrecedesHooks(t *testing.T) {
	ws := newPermWS(t)
	floor := &FloorGate{WS: ws, Mode: ModePlan}
	d := floor.Check(context.Background(), toolEffect("edit_file", "edit", `{"path":"a.txt"}`))
	if d.Action != event.VerdictDeny {
		t.Fatalf("floor must deny plan-mode edit: %+v", d)
	}
	// Escape denied even in bypass, via the floor.
	floorBypass := &FloorGate{WS: ws, Mode: ModeBypass}
	esc := floorBypass.Check(context.Background(), toolEffect("read_file", "read", `{"path":"../../etc/passwd"}`))
	if esc.Action != event.VerdictDeny {
		t.Fatalf("floor must deny escape even in bypass: %+v", esc)
	}
}

// Unknown/empty tool class fails closed (ask), not open (allow).
func TestUnknownClassFailsClosed(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t)}
	for _, mode := range []string{ModeDefault, ModePlan, ModeAcceptEdits} {
		g.Mode = mode
		if d := g.Check(context.Background(), toolEffect("mystery_tool", "", `{}`)); d.Action == event.VerdictAllow {
			t.Errorf("mode %s: unknown class allowed: %+v", mode, d)
		}
	}
}
