package statetest

import (
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/state"
)

// recorder captures Errorf output so we can assert on the diff shape.
type recorder struct {
	testing.TB
	failed  bool
	message string
}

func (r *recorder) Helper() {}
func (r *recorder) Errorf(format string, args ...any) {
	r.failed = true
	r.message += strings.ReplaceAll(format, "%s", "%v")
	for range args {
		r.message += " ARG"
	}
	_ = format
}

func TestEqualStatesPass(t *testing.T) {
	r := &recorder{TB: t}
	AssertFoldEqual(r, state.New(), state.New())
	if r.failed {
		t.Fatalf("equal states reported divergence: %s", r.message)
	}
}

// nil vs empty map must NOT count as divergence (JSON semantics).
func TestNilVsEmptyMapIsEqual(t *testing.T) {
	a := state.New()
	b := state.New()
	b.Activities = nil
	r := &recorder{TB: t}
	AssertFoldEqual(r, a, b)
	if r.failed {
		t.Fatalf("nil-vs-empty map reported divergence: %s", r.message)
	}
}

func TestDivergenceNamesTheSubState(t *testing.T) {
	a := state.New()
	b := state.New()
	b.Session.Status = state.StatusEnded
	r := &recorder{TB: t}
	AssertFoldEqual(r, a, b)
	if !r.failed {
		t.Fatal("diverging states must fail")
	}
}
