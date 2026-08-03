package kill

import (
	"context"
	"errors"
	"testing"

	"github.com/ralphite/agentrunner/internal/errs"
)

func cause(t *testing.T, ctx context.Context) error {
	t.Helper()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	default:
		return nil
	}
}

// A kill cuts exactly its target and carries the killed cause; siblings
// keep running. That isolation is the whole reason the registry exists —
// the batch context alone can only cancel everything at once.
func TestKillCutsOnlyItsTarget(t *testing.T) {
	r := NewRegistry()
	a, cancelA := context.WithCancelCause(context.Background())
	b, cancelB := context.WithCancelCause(context.Background())
	defer cancelA(nil)
	defer cancelB(nil)
	r.Register(Target{ID: "call_a", Kind: "tool", Name: "bash", Session: "s"}, cancelA)
	r.Register(Target{ID: "call_b", Kind: "tool", Name: "read_file", Session: "s"}, cancelB)

	got, ok := r.Kill("call_a")
	if !ok || got.Name != "bash" {
		t.Fatalf("kill call_a = %+v, %v; want the bash entry", got, ok)
	}
	if err := cause(t, a); !errors.Is(err, errs.ErrKilled) {
		t.Fatalf("target cause = %v; want ErrKilled", err)
	}
	if err := cause(t, b); err != nil {
		t.Fatalf("sibling was cancelled too: %v", err)
	}
}

// An id nobody is running is an ordinary miss, not an error: work finishes
// between the render and the click all the time.
func TestKillUnknownIsAMiss(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Kill("gone"); ok {
		t.Fatal("kill of an unknown id reported a hit")
	}
	if _, ok := (*Registry)(nil).Kill("gone"); ok {
		t.Fatal("nil registry reported a hit")
	}
}

// A foreground call that converts to background keeps its id but changes
// which cancel reaches it. The superseded unregister must NOT unpublish the
// live entry — otherwise the handoff silently makes the work unkillable,
// which is precisely the window a user is most likely to reach for.
func TestReRegisterSurvivesStaleUnregister(t *testing.T) {
	r := NewRegistry()
	fg, cancelFG := context.WithCancelCause(context.Background())
	bg, cancelBG := context.WithCancelCause(context.Background())
	defer cancelFG(nil)
	defer cancelBG(nil)

	unregisterFG := r.Register(Target{ID: "call_1", Kind: "tool", Name: "bash", Session: "s"}, cancelFG)
	r.Register(Target{ID: "call_1", Kind: "tool", Name: "bash", Session: "s"}, cancelBG)
	unregisterFG() // the foreground goroutine's defer, running after the handoff

	if _, ok := r.Kill("call_1"); !ok {
		t.Fatal("handed-off work became unkillable")
	}
	if err := cause(t, bg); !errors.Is(err, errs.ErrKilled) {
		t.Fatalf("handed-off cause = %v; want ErrKilled", err)
	}
	if err := cause(t, fg); err != nil {
		t.Fatalf("superseded cancel fired: %v", err)
	}
}

// Killing a subagent must also cut the tool call that agent is parked in —
// otherwise "stop this agent" leaves its command running.
func TestKillSessionCutsEverythingThatMemberRuns(t *testing.T) {
	r := NewRegistry()
	childTool, cancelChildTool := context.WithCancelCause(context.Background())
	_, cancelChildAgent := context.WithCancelCause(context.Background())
	parent, cancelParent := context.WithCancelCause(context.Background())
	defer cancelChildTool(nil)
	defer cancelChildAgent(nil)
	defer cancelParent(nil)
	r.Register(Target{ID: "call_c", Kind: "tool", Name: "bash", Session: "child"}, cancelChildTool)
	r.Register(Target{ID: "call_s", Kind: "agent", Name: "worker", Session: "child"}, cancelChildAgent)
	r.Register(Target{ID: "call_p", Kind: "tool", Name: "grep", Session: "root"}, cancelParent)

	hits := r.KillSession("child")
	if len(hits) != 2 || hits[0].ID != "call_c" || hits[1].ID != "call_s" {
		t.Fatalf("KillSession(child) = %+v; want both child entries, id-ordered", hits)
	}
	if err := cause(t, childTool); !errors.Is(err, errs.ErrKilled) {
		t.Fatalf("child tool cause = %v; want ErrKilled", err)
	}
	if err := cause(t, parent); err != nil {
		t.Fatalf("another member's work was cut: %v", err)
	}
}

// List answers "what can I stop right now" from live memory, and an entry
// disappears the moment its owner unregisters.
func TestListReflectsLiveWork(t *testing.T) {
	r := NewRegistry()
	_, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	unregister := r.Register(Target{ID: "call_1", Kind: "tool", Name: "bash", Session: "s"}, cancel)
	if got := r.List(); len(got) != 1 || got[0].Name != "bash" {
		t.Fatalf("List = %+v; want the one live call", got)
	}
	unregister()
	if got := r.List(); len(got) != 0 {
		t.Fatalf("List after unregister = %+v; want empty", got)
	}
}
