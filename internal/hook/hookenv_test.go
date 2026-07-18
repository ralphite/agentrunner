package hook

import (
	"context"
	"strings"
	"testing"
)

// Hooks lose credential vars by default, keep sealed passthrough ones, and a
// failing hook's note names what was withheld (audit-0718 P0-2/P0-3).
func TestHookEnvPassthroughAndExplicitWithholding(t *testing.T) {
	t.Setenv("HOOKTEST_API_KEY", "hook-hidden-value-1")
	t.Setenv("HOOKTEST_TOKEN", "hook-passed-value-1")

	r := &Runner{Dir: t.TempDir()}
	r.SealEnvPassthrough([]string{"HOOKTEST_TOKEN"})

	env, withheld := r.scrubbedEnv()
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "hook-hidden-value-1") {
		t.Fatal("withheld credential leaked into hook env")
	}
	if !strings.Contains(joined, "HOOKTEST_TOKEN=hook-passed-value-1") {
		t.Fatal("passthrough var missing from hook env")
	}
	if got := strings.Join(withheld, ","); !strings.Contains(got, "HOOKTEST_API_KEY") ||
		strings.Contains(got, "HOOKTEST_TOKEN") {
		t.Fatalf("withheld = %v", withheld)
	}

	// A failing hook's note carries the withheld names — not silent.
	r2 := &Runner{Dir: t.TempDir(), PostTool: []string{"exit 3"}}
	notes := r2.RunPost(context.Background(), PostInput{ToolName: "bash"})
	if len(notes) != 1 || !strings.Contains(notes[0], "credential env vars withheld") ||
		!strings.Contains(notes[0], "HOOKTEST_API_KEY") {
		t.Fatalf("failing hook note not explicit about withholding: %v", notes)
	}

	// First seal wins: a child re-seal cannot widen.
	r.SealEnvPassthrough([]string{"HOOKTEST_API_KEY"})
	if _, w := r.scrubbedEnv(); !strings.Contains(strings.Join(w, ","), "HOOKTEST_API_KEY") {
		t.Fatal("child seal widened the hook env face")
	}
}
