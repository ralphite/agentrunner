package pipeline

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
)

// A command tool (INC-55) adjudicates on its FIXED manifest command carried in
// eff.Command, not the model's args (which are stdin data). This proves the
// gate treats it exactly like a bash command line: default ask, tool/command
// allow rules match, a deny on the fixed command holds, and a compound command
// is split per segment.
func TestCommandToolEffectAdjudication(t *testing.T) {
	ws := newPermWS(t)

	// The effect the loop builds for a command tool: execute class, model args
	// as data, the fixed command supplied out-of-band.
	ctEff := func(name, command, modelArgs string) Effect {
		return Effect{
			ID: "eff-ct", Kind: "tool_call", ToolName: name, Class: "execute",
			Args: json.RawMessage(modelArgs), CallID: "c_1_0", Command: command,
		}
	}

	t.Run("no rule falls to execute default (ask)", func(t *testing.T) {
		g := &PermissionGate{WS: ws} // default mode
		d := g.Check(context.Background(), ctEff("deploy", "./deploy.sh prod", `{"target":"prod"}`))
		if d.Action != event.VerdictAsk {
			t.Fatalf("decision = %+v, want ask", d)
		}
	})

	t.Run("tool-name allow rule", func(t *testing.T) {
		g := &PermissionGate{WS: ws, Rules: []PermissionRule{{Tool: "deploy", Action: "allow"}}}
		d := g.Check(context.Background(), ctEff("deploy", "./deploy.sh prod", `{"target":"prod"}`))
		if d.Action != event.VerdictAllow {
			t.Fatalf("decision = %+v, want allow", d)
		}
	})

	t.Run("command glob allow matches the fixed command", func(t *testing.T) {
		g := &PermissionGate{WS: ws, Rules: []PermissionRule{{Command: "./deploy.sh *", Action: "allow"}}}
		d := g.Check(context.Background(), ctEff("deploy", "./deploy.sh prod", `{"target":"prod"}`))
		if d.Action != event.VerdictAllow {
			t.Fatalf("decision = %+v, want allow", d)
		}
	})

	t.Run("deny on the fixed command holds regardless of model args", func(t *testing.T) {
		g := &PermissionGate{WS: ws, Rules: []PermissionRule{{Command: "rm *", Action: "deny"}}}
		// The model's args say nothing dangerous; the manifest command does.
		d := g.Check(context.Background(), ctEff("cleanup", "rm -rf /tmp/build", `{}`))
		if d.Action != event.VerdictDeny {
			t.Fatalf("decision = %+v, want deny", d)
		}
	})

	t.Run("compound fixed command is adjudicated per segment", func(t *testing.T) {
		// git segment allowed, rm segment unmatched → strictest (ask) wins.
		g := &PermissionGate{WS: ws, Rules: []PermissionRule{{Command: "git *", Action: "allow"}}}
		d := g.Check(context.Background(), ctEff("gitclean", "git status && rm -rf x", `{}`))
		if d.Action != event.VerdictAsk {
			t.Fatalf("decision = %+v, want ask (rm segment holds it back)", d)
		}
	})

	t.Run("plan mode denies execute-class command tools at the floor", func(t *testing.T) {
		g := &PermissionGate{WS: ws, Mode: ModePlan}
		d := g.Check(context.Background(), ctEff("deploy", "./deploy.sh", `{}`))
		if d.Action != event.VerdictDeny {
			t.Fatalf("decision = %+v, want deny in plan mode", d)
		}
	})
}

// Regression: a bash effect (no eff.Command) still adjudicates on its
// {"command":...} args exactly as before the command-tool seam was added.
func TestBashStillUsesArgsCommand(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Rules: []PermissionRule{{Tool: "bash", Command: "rm *", Action: "deny"}}}
	d := g.Check(context.Background(), toolEffect("bash", "execute", `{"command":"rm -rf build"}`))
	if d.Action != event.VerdictDeny {
		t.Fatalf("decision = %+v, want deny", d)
	}
}
