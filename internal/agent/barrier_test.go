package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/snapshot"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// withShadowRepo attaches a real shadow snapshot store to the loop; skips
// the test when git is unavailable (the CheckpointBarrier path is exactly
// the feature under test — a None store would journal nothing).
func withShadowRepo(t *testing.T, l *Loop, root string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo, err := snapshot.NewShadowRepo(filepath.Join(t.TempDir(), "shadow.git"), root)
	if err != nil {
		t.Fatal(err)
	}
	l.Snapshots = repo
}

func decodedBarriers(t *testing.T, dir string) []*event.CheckpointBarrier {
	t.Helper()
	events, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []*event.CheckpointBarrier
	for _, e := range events {
		if e.Type != event.TypeCheckpointBarrier {
			continue
		}
		dec, err := event.DecodePayload(e)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, dec.(*event.CheckpointBarrier))
	}
	return out
}

// S7.2: every turn boundary and the run end cut a barrier — snapshot ref,
// self seq in the vector — and the fold's barriers sub-state records them.
func TestBarrierPerTurnAndTerminal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "greet.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "edit_file", Args: map[string]any{
				"path": "greet.txt", "old": "hello", "new": "HELLO"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, root)
	withShadowRepo(t, l, root)

	res, err := l.Run(context.Background(), "make it loud")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.GenSteps != 2 {
		t.Fatalf("res = %+v", res)
	}

	barriers := decodedBarriers(t, l.Store.Dir())
	if len(barriers) != 3 {
		t.Fatalf("barriers = %d, want 3 (t1, t2, final)", len(barriers))
	}
	wantIDs := []string{"bar-t1", "bar-t2", "bar-final"}
	for i, b := range barriers {
		if b.BarrierID != wantIDs[i] {
			t.Errorf("barrier[%d].id = %q, want %q", i, b.BarrierID, wantIDs[i])
		}
		if b.SnapshotRef == "" {
			t.Errorf("barrier %s has no snapshot ref", b.BarrierID)
		}
		if b.Vector["."] <= 0 {
			t.Errorf("barrier %s vector[.] = %d, want > 0", b.BarrierID, b.Vector["."])
		}
	}
	// The workspace changed between t1 and t2 (edit_file), so the refs differ;
	// nothing changed between t2 and the end, so dedup reuses the ref.
	if barriers[0].SnapshotRef == barriers[1].SnapshotRef {
		t.Error("t1 and t2 refs identical despite a workspace edit")
	}
	if barriers[1].SnapshotRef != barriers[2].SnapshotRef {
		t.Error("t2 and final refs differ despite an unchanged workspace")
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if len(fold.Barriers) != 3 {
		t.Fatalf("fold.Barriers = %d, want 3", len(fold.Barriers))
	}
	for i, b := range fold.Barriers {
		if b.Seq <= 0 {
			t.Errorf("fold barrier %s seq = %d", b.BarrierID, b.Seq)
		}
		if b.BarrierID != wantIDs[i] || b.SnapshotRef != barriers[i].SnapshotRef {
			t.Errorf("fold barrier[%d] = %+v, want journal's %+v", i, b, barriers[i])
		}
	}
}

// Feature gate: without a snapshot store there are NO barrier events — every
// pre-S7 run shape is untouched.
func TestNoSnapshotStoreNoBarriers(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	if _, err := l.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if got := decodedBarriers(t, l.Store.Dir()); len(got) != 0 {
		t.Fatalf("barriers = %d, want none without a store", len(got))
	}
}

// A barrier cut while background work is in flight records its handle with
// its fork-time disposition; the quiescent barrier comes after the work
// settled (决策 #31: nothing in flight at quiescence), so it records none.
func TestBarrierRecordsLiveWork(t *testing.T) {
	root := t.TempDir()
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "bg1", Name: "bash",
				Args: map[string]any{"command": "while [ ! -f .release-bg1 ]; do sleep 0.01; done; echo eventually", "background": true}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "not waiting yet"}, {Finish: "end_turn"}}},
		{
			Expect:  scripted.Expect{LastMessageContains: "eventually"},
			Respond: []scripted.Event{{Text: "saw it, done"}, {Finish: "end_turn"}},
		},
	}}
	l := testLoop(t, fix, root)
	withShadowRepo(t, l, root)
	released := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			events, _ := store.ReadEvents(l.Store.Dir())
			turnTwoFinished := false
			for _, env := range events {
				if env.Type == event.TypeAssistantMessage {
					decoded, _ := event.DecodePayload(env)
					turnTwoFinished = decoded.(*event.AssistantMessage).GenStep >= 2
				}
			}
			if turnTwoFinished {
				released <- os.WriteFile(filepath.Join(root, ".release-bg1"), []byte("go"), 0o600)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		released <- context.DeadlineExceeded
	}()

	if _, err := l.Run(context.Background(), "fire and follow"); err != nil {
		t.Fatal(err)
	}
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	barriers := decodedBarriers(t, l.Store.Dir())
	if len(barriers) != 4 {
		var ids []string
		for _, b := range barriers {
			ids = append(ids, b.BarrierID)
		}
		t.Fatalf("barriers = %v, want [bar-t1 bar-t2 bar-t3 bar-final]", ids)
	}
	t2 := barriers[1]
	if len(t2.Handles) != 1 || t2.Handles[0].Handle != "bg1" || t2.Handles[0].Policy != "cancel_at_fork" {
		t.Errorf("bar-t2 handles = %+v, want [{bg1 cancel_at_fork}]", t2.Handles)
	}
	final := barriers[len(barriers)-1]
	if len(final.Handles) != 0 {
		t.Errorf("quiescent barrier handles = %+v, want none", final.Handles)
	}
}

// The barrier vector covers child streams: after a spawn completes, later
// barriers pin the child journal's final seq under its sub/ path.
func TestBarrierVectorIncludesChildStreams(t *testing.T) {
	root := t.TempDir()
	parentFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "prompt": "PIN-THE-VECTOR job"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	childFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "REPORT: ok"}, {Finish: "end_turn"}}},
	}}
	l, _ := routedSpawnLoop(t, parentFix, root,
		scripted.RoutePair{Key: "PIN-THE-VECTOR", Fixture: childFix})
	withShadowRepo(t, l, root)

	if _, err := l.Run(context.Background(), "delegate"); err != nil {
		t.Fatal(err)
	}
	barriers := decodedBarriers(t, l.Store.Dir())
	if len(barriers) < 3 {
		t.Fatalf("barriers = %d, want >= 3", len(barriers))
	}
	// bar-t1 predates the spawn: self only. When exactly the receipt lands
	// is settle timing; the QUIESCENT barrier must pin the child stream.
	if _, ok := barriers[0].Vector["sub/s1-a1"]; ok {
		t.Error("bar-t1 already has the child stream")
	}
	final := barriers[len(barriers)-1]
	if final.Vector["sub/s1-a1"] <= 0 {
		t.Errorf("quiescent barrier vector = %+v, want sub/s1-a1 pinned", final.Vector)
	}
}
