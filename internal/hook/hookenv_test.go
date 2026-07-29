package hook

import (
	"context"
	"os"
	"testing"
)

// Hooks are the operator's own commands, so they inherit the operator's
// environment WHOLE — credentials included (决策 #34 修订). The old
// audit-0718 P0-2 scrub made every auth-using hook (deploy, notify, gh api)
// fail for a reason the user could not see from their own shell; withholding
// a variable the user put in their own environment is not a boundary, it is
// a bug. Journaled surfaces still value-redact — that was always the
// separate question.
func TestHookInheritsCredentialEnv(t *testing.T) {
	t.Setenv("HOOKTEST_API_KEY", "hook-visible-value-1")
	t.Setenv("HOOKTEST_TOKEN", "hook-visible-value-2")

	r := &Runner{Dir: t.TempDir(), PostTool: []string{`printf '%s/%s' "$HOOKTEST_API_KEY" "$HOOKTEST_TOKEN"`}}
	notes := r.RunPost(context.Background(), PostInput{ToolName: "bash"})
	if len(notes) != 1 || notes[0] != "hook-visible-value-1/hook-visible-value-2" {
		t.Fatalf("hook did not inherit credential env: %v", notes)
	}
}

// HOME is the operator's own, never an isolated temp: a hook calling gh/git
// must find the same config the user's terminal finds.
func TestHookKeepsRealHome(t *testing.T) {
	want := os.Getenv("HOME")
	if want == "" {
		t.Skip("no HOME in this environment")
	}
	r := &Runner{Dir: t.TempDir(), PostTool: []string{`printf '%s' "$HOME"`}}
	notes := r.RunPost(context.Background(), PostInput{ToolName: "bash"})
	if len(notes) != 1 || notes[0] != want {
		t.Fatalf("hook HOME = %v, want the operator's %q", notes, want)
	}
}
