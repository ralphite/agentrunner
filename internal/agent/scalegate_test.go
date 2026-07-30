package agent

import (
	"testing"

	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

func loopAtScale(t *testing.T, large bool, tools ...string) *Loop {
	t.Helper()
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ws.SetScale(0, large)
	return &Loop{
		Spec: &AgentSpec{Tools: tools},
		Exec: &tool.Executor{WS: ws},
	}
}

func TestGatedToolsWithholdsIndexedSearchWhenLarge(t *testing.T) {
	l := loopAtScale(t, true, "read_file", "keyword_search", "grep", "glob")
	got := l.gatedTools()

	for _, banned := range []string{"keyword_search", "semantic_search"} {
		for _, n := range got {
			if n == banned {
				t.Errorf("%s must be withheld in a large workspace; got %v", banned, got)
			}
		}
	}
	// The bounded alternatives must SURVIVE — withholding search entirely
	// would cripple the agent rather than bound it.
	for _, keep := range []string{"read_file", "grep", "glob"} {
		if !contains(got, keep) {
			t.Errorf("%s must remain available; got %v", keep, got)
		}
	}
}

// The spec itself is never rewritten: a small workspace (or mode=never) gets
// the full face back with no migration.
func TestGatedToolsUntouchedWhenSmall(t *testing.T) {
	l := loopAtScale(t, false, "read_file", "keyword_search", "grep")
	got := l.gatedTools()
	if len(got) != 3 || !contains(got, "keyword_search") {
		t.Errorf("small workspace must keep the full tool face; got %v", got)
	}
	if len(l.Spec.Tools) != 3 || !contains(l.Spec.Tools, "keyword_search") {
		t.Errorf("the spec must not be mutated; got %v", l.Spec.Tools)
	}
}

// The gate fails OPEN: no Executor or no Workspace means un-degraded behavior,
// never silent degradation.
func TestLargeWorkspaceFailsOpen(t *testing.T) {
	if (&Loop{Spec: &AgentSpec{}}).largeWorkspace() {
		t.Error("a Loop with no Executor must not report a large workspace")
	}
	if (&Loop{Spec: &AgentSpec{}, Exec: &tool.Executor{}}).largeWorkspace() {
		t.Error("a Loop with no Workspace must not report a large workspace")
	}
	// A Workspace nobody stamped is likewise not large.
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ws.IsLarge() {
		t.Error("an unstamped Workspace must default to not-large")
	}
}

// scaleWithheldTools is what keeps the dispatch allowlist a function of FOLDED
// facts rather than live environment: the withheld names stay allowed, so a
// session recorded in a small tree and resumed in a grown one is not refused a
// call its original happily served.
func TestScaleWithheldToolsIsTheComplement(t *testing.T) {
	large := loopAtScale(t, true, "read_file", "keyword_search", "grep")
	withheld := large.scaleWithheldTools()
	if len(withheld) != 1 || withheld[0] != "keyword_search" {
		t.Errorf("withheld = %v, want exactly [keyword_search]", withheld)
	}
	// gated + withheld must reconstruct the spec's list exactly — no name may
	// be lost or duplicated by the split.
	if len(large.gatedTools())+len(withheld) != len(large.Spec.Tools) {
		t.Errorf("split lost or duplicated names: %v + %v vs %v",
			large.gatedTools(), withheld, large.Spec.Tools)
	}

	small := loopAtScale(t, false, "read_file", "keyword_search")
	if got := small.scaleWithheldTools(); got != nil {
		t.Errorf("nothing is withheld in a small workspace; got %v", got)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
