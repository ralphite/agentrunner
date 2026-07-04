package fork

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

const parentSession = "20260704-100000-fix-aaaa"

// seedParent journals a two-barrier run shape plus sub/ and artifacts/
// side stores, and returns the parent dir and its fold.
func seedParent(t *testing.T) (string, state.State) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "parent")
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()

	appendT := func(typ string, payload any, causation string) event.Envelope {
		t.Helper()
		env, err := event.New(typ, payload)
		if err != nil {
			t.Fatal(err)
		}
		env.CorrelationID = parentSession
		env.CausationID = causation
		appended, err := es.Append(env)
		if err != nil {
			t.Fatal(err)
		}
		return appended
	}

	appendT(event.TypeRunStarted, &event.RunStarted{SpecName: "fixer",
		WorkspaceRoot: "/w", Spec: []byte(`{"name":"fixer"}`)}, "")
	appendT(event.TypeInputReceived, &event.InputReceived{Text: "fix it", Source: "cli"}, "")
	appendT(event.TypeTurnStarted, &event.TurnStarted{Turn: 1}, "evt-2")
	appendT(event.TypeCheckpointBarrier, &event.CheckpointBarrier{
		BarrierID: "bar-t1", Turn: 1, Vector: map[string]int64{".": 3}, SnapshotRef: "ref-aaa"}, "evt-3")
	appendT(event.TypeTurnStarted, &event.TurnStarted{Turn: 2}, "evt-4")
	appendT(event.TypeCheckpointBarrier, &event.CheckpointBarrier{
		BarrierID: "bar-t2", Turn: 2, Vector: map[string]int64{".": 5}, SnapshotRef: "ref-bbb"}, "evt-5")
	appendT(event.TypeRunEnded, &event.RunEnded{Reason: "completed", Turns: 2}, "evt-6")

	for _, f := range []string{"sub/s1-a1/events.jsonl", "artifacts/blobs/sha256-xx"} {
		path := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("payload:"+f), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	events, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	return dir, fold
}

func TestCutCopiesBarrierSlice(t *testing.T) {
	parentDir, fold := seedParent(t)
	newDir := filepath.Join(t.TempDir(), "forked")
	const newSession = "20260704-110000-fork-bbbb"

	refs, err := Cut(Options{
		ParentDir: parentDir, ParentSession: parentSession,
		NewDir: newDir, NewSession: newSession,
		Barrier:       fold.Barriers[0], // bar-t1, seq 4
		WorkspaceRoot: "/w-fork",
		Now:           time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != "ref-aaa" {
		t.Errorf("refs = %v, want [ref-aaa] (only the cut's barriers)", refs)
	}

	got, err := store.ReadEvents(newDir)
	if err != nil {
		t.Fatal(err)
	}
	// Genesis + the 4 events up to and including bar-t1; bar-t2 and
	// run_ended are outside the cut.
	if len(got) != 5 {
		t.Fatalf("events = %d, want 5", len(got))
	}
	if got[0].Type != event.TypeForkedFrom {
		t.Fatalf("genesis = %s", got[0].Type)
	}
	genesis, err := event.DecodePayload(got[0])
	if err != nil {
		t.Fatal(err)
	}
	ff := genesis.(*event.ForkedFrom)
	if ff.ParentSession != parentSession || ff.BarrierID != "bar-t1" ||
		ff.SnapshotRef != "ref-aaa" || ff.WorkspaceRoot != "/w-fork" {
		t.Errorf("forked_from = %+v", ff)
	}
	parentEvents, _ := store.ReadEvents(parentDir)
	for i, e := range got {
		if e.Seq != int64(i+1) || e.ID != event.EventID(e.Seq) {
			t.Errorf("event %d: seq=%d id=%s, want gapless remap", i, e.Seq, e.ID)
		}
		if e.CorrelationID != newSession {
			t.Errorf("event %d correlation = %q, want fork session", i, e.CorrelationID)
		}
		if i > 0 && !bytes.Equal(e.Payload, parentEvents[i-1].Payload) {
			t.Errorf("event %d payload diverged from parent provenance", i)
		}
	}
	// Causation ids that referenced events moved with the shift.
	if got[3].CausationID != "evt-3" { // was evt-2 in the parent
		t.Errorf("turn_started causation = %q, want evt-3", got[3].CausationID)
	}

	fold2, err := state.Fold(got)
	if err != nil {
		t.Fatal(err)
	}
	if fold2.Run.ForkedFrom == nil || fold2.Run.ForkedFrom.ParentSession != parentSession ||
		fold2.Run.ForkedFrom.BarrierID != "bar-t1" {
		t.Errorf("fold provenance = %+v", fold2.Run.ForkedFrom)
	}
	if fold2.Run.Status != state.StatusRunning || fold2.Run.Turn != 1 {
		t.Errorf("fork state = %s turn %d, want running turn 1", fold2.Run.Status, fold2.Run.Turn)
	}
	if len(fold2.Barriers) != 1 || fold2.Barriers[0].Seq != 5 {
		t.Errorf("fork barriers = %+v, want bar-t1 at shifted seq 5", fold2.Barriers)
	}

	// Side stores traveled verbatim.
	for _, f := range []string{"sub/s1-a1/events.jsonl", "artifacts/blobs/sha256-xx"} {
		raw, err := os.ReadFile(filepath.Join(newDir, f))
		if err != nil || string(raw) != "payload:"+f {
			t.Errorf("aux file %s: %q err=%v", f, raw, err)
		}
	}
	// Fold snapshots must NOT travel (they cache parent seqs).
	if _, ok, _ := store.LatestSnapshot(newDir); ok {
		t.Error("fold snapshot leaked into the fork")
	}

	// A second cut into the same dir refuses: a journal is never overwritten.
	if _, err := Cut(Options{ParentDir: parentDir, ParentSession: parentSession,
		NewDir: newDir, NewSession: newSession, Barrier: fold.Barriers[0],
		Now: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)}); err == nil {
		t.Error("overwriting an existing fork journal must fail")
	}
}

func TestCutUnknownBarrier(t *testing.T) {
	parentDir, fold := seedParent(t)
	bogus := fold.Barriers[0]
	bogus.Seq = 99
	if _, err := Cut(Options{ParentDir: parentDir, ParentSession: parentSession,
		NewDir: filepath.Join(t.TempDir(), "x"), NewSession: "s", Barrier: bogus,
		Now: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)}); err == nil {
		t.Fatal("a barrier absent from the journal must be refused")
	}
}
