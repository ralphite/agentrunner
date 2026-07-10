package pipeline

import (
	"context"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
)

func editEff(path string) Effect {
	return toolEffect("edit_file", "edit", `{"path":"`+path+`","old":"x","new":"y"}`)
}

// unit: isProtectedWritePath
func TestIsProtectedWritePath(t *testing.T) {
	protected := []string{
		".git/config", ".git/hooks/pre-commit", "sub/.git/config",
		".claude/settings.yaml", ".claude/agents/x.md",
		".bashrc", "home/.zshrc", ".npmrc", ".mcp.json", ".claude.json",
		".gitconfig", ".pre-commit-config.yaml",
		".config/git/config", ".vscode/settings.json", ".github/workflows/ci.yml",
		"gradle-wrapper.properties",
	}
	for _, p := range protected {
		if !isProtectedWritePath(p) {
			t.Errorf("isProtectedWritePath(%q) = false, want true", p)
		}
	}
	notProtected := []string{
		"src/main.go", "README.md", "docs/x.md", "gitconfig", // no leading dot
		".claude/worktrees/w1/f.go",  // carve-out
		".claude/worktrees/w/.git/x", // under worktree carve-out (the .claude match yields carve-out first)
		"",
	}
	for _, p := range notProtected {
		if isProtectedWritePath(p) {
			t.Errorf("isProtectedWritePath(%q) = true, want false", p)
		}
	}
}

// acceptEdits auto-allows normal edits but must ask for protected writes.
func TestAcceptEditsProtectedRequiresApproval(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Mode: ModeAcceptEdits}
	// Normal file: acceptEdits allows.
	if d := g.Check(context.Background(), editEff("src/a.go")); d.Action != event.VerdictAllow {
		t.Fatalf("acceptEdits normal edit = %+v, want allow", d)
	}
	// Protected files: acceptEdits must ask.
	for _, p := range []string{".git/config", ".claude/settings.yaml", ".bashrc", ".mcp.json"} {
		if d := g.Check(context.Background(), editEff(p)); d.Action != event.VerdictAsk {
			t.Errorf("acceptEdits edit %s = %+v, want ask (protected)", p, d)
		}
	}
}

// bypass ignores protected paths (it explicitly forgoes protection).
func TestBypassIgnoresProtected(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Mode: ModeBypass}
	if d := g.Check(context.Background(), editEff(".git/config")); d.Action != event.VerdictAllow {
		t.Fatalf("bypass edit .git/config = %+v, want allow", d)
	}
}

// An explicit allow rule outranks the protected-path tightening (rules run
// before the mode default): a user who explicitly allows .git/** means it.
func TestExplicitAllowOverridesProtected(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Mode: ModeAcceptEdits, Rules: []PermissionRule{
		{Tool: "edit_file", Path: ".git/**", Action: "allow"},
	}}
	if d := g.Check(context.Background(), editEff(".git/config")); d.Action != event.VerdictAllow {
		t.Fatalf("explicit allow .git/** = %+v, want allow (rule beats protected)", d)
	}
}

// The .claude/worktrees carve-out: acceptEdits still auto-allows there.
func TestProtectedWorktreeCarveout(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Mode: ModeAcceptEdits}
	if d := g.Check(context.Background(), editEff(".claude/worktrees/w1/main.go")); d.Action != event.VerdictAllow {
		t.Fatalf("acceptEdits edit under worktrees = %+v, want allow (carve-out)", d)
	}
}

// default mode already asks for edits — protected does not change that (and
// must not turn an ask into an allow).
func TestDefaultModeProtectedStillAsks(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Mode: ModeDefault}
	if d := g.Check(context.Background(), editEff(".git/config")); d.Action != event.VerdictAsk {
		t.Fatalf("default edit .git/config = %+v, want ask", d)
	}
	if d := g.Check(context.Background(), editEff("src/a.go")); d.Action != event.VerdictAsk {
		t.Fatalf("default edit normal = %+v, want ask (unchanged)", d)
	}
}

// An explicit DENY still denies a protected write (deny is strongest).
func TestExplicitDenyBeatsProtected(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Mode: ModeAcceptEdits, Rules: []PermissionRule{
		{Tool: "edit_file", Path: ".git/**", Action: "deny"},
	}}
	if d := g.Check(context.Background(), editEff(".git/config")); d.Action != event.VerdictDeny {
		t.Fatalf("explicit deny .git/** = %+v, want deny", d)
	}
}
