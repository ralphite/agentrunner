package fork

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
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

	appendT(event.TypeSessionStarted, &event.SessionStarted{SpecName: "fixer",
		WorkspaceRoot: "/w", Spec: []byte(`{"name":"fixer"}`)}, "")
	appendT(event.TypeInputReceived, &event.InputReceived{Text: "fix it", Source: "cli"}, "")
	appendT(event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}, "evt-2")
	appendT(event.TypeCheckpointBarrier, &event.CheckpointBarrier{
		BarrierID: "bar-t1", GenStep: 1, Vector: map[string]int64{".": 3, "sub/s1-a1": 1}, SnapshotRef: "ref-aaa"}, "evt-3")
	appendT(event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 2}, "evt-4")
	appendT(event.TypeCheckpointBarrier, &event.CheckpointBarrier{
		BarrierID: "bar-t2", GenStep: 2, Vector: map[string]int64{".": 5, "sub/s1-a1": 1}, SnapshotRef: "ref-bbb"}, "evt-5")
	appendT(event.TypeAssistantMessage, &event.AssistantMessage{GenStep: 2,
		Message: provider.Message{Role: provider.RoleAssistant,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "done"}}}}, "evt-6")

	childDir := filepath.Join(dir, "sub/s1-a1")
	child, err := store.OpenEventStore(childDir)
	if err != nil {
		t.Fatal(err)
	}
	childGenesis, _ := event.New(event.TypeSessionStarted, &event.SessionStarted{SpecName: "child"})
	if _, err := child.Append(childGenesis); err != nil {
		t.Fatal(err)
	}
	_ = child.Close()
	for _, f := range []string{"artifacts/blobs/sha256-xx"} {
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
	// session_closed events are outside the cut.
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
		t.Errorf("generation_started causation = %q, want evt-3", got[3].CausationID)
	}

	fold2, err := state.Fold(got)
	if err != nil {
		t.Fatal(err)
	}
	if fold2.Session.ForkedFrom == nil || fold2.Session.ForkedFrom.ParentSession != parentSession ||
		fold2.Session.ForkedFrom.BarrierID != "bar-t1" {
		t.Errorf("fold provenance = %+v", fold2.Session.ForkedFrom)
	}
	if fold2.Session.Status != state.StatusRunning || fold2.Session.GenStep != 1 {
		t.Errorf("fork state = %s turn %d, want running turn 1", fold2.Session.Status, fold2.Session.GenStep)
	}
	if len(fold2.Barriers) != 1 || fold2.Barriers[0].Seq != 5 {
		t.Errorf("fork barriers = %+v, want bar-t1 at shifted seq 5", fold2.Barriers)
	}

	// Child journals travel as the exact vector prefix; immutable artifacts
	// travel verbatim.
	childEvents, err := store.ReadEvents(filepath.Join(newDir, "sub/s1-a1"))
	if err != nil || len(childEvents) != 1 || childEvents[0].Type != event.TypeSessionStarted {
		t.Fatalf("child cut = %+v err=%v", childEvents, err)
	}
	artifact := "artifacts/blobs/sha256-xx"
	raw, err := os.ReadFile(filepath.Join(newDir, artifact))
	if err != nil || string(raw) != "payload:"+artifact {
		t.Errorf("aux file %s: %q err=%v", artifact, raw, err)
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

func TestCutInheritsCurrentParentTitle(t *testing.T) {
	parentDir, fold := seedParent(t)
	newDir := filepath.Join(t.TempDir(), "forked")
	_, err := Cut(Options{
		ParentDir: parentDir, ParentSession: parentSession,
		NewDir: newDir, NewSession: "20260704-110000-fork-title",
		Barrier: fold.Barriers[0], WorkspaceRoot: "/w-fork",
		Now:   time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		Title: "Initialize Taskledger CLI Skeleton",
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(newDir)
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if last.Type != event.TypeSessionTitled {
		t.Fatalf("last event = %s, want session_titled", last.Type)
	}
	folded, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if folded.Session.RawTitle != "Initialize Taskledger CLI Skeleton" ||
		folded.Session.TitleSource != event.TitleSourceFork {
		t.Fatalf("title/source = %q/%q", folded.Session.RawTitle, folded.Session.TitleSource)
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

func TestCutRejectsChildJournalShorterThanBarrierVector(t *testing.T) {
	parentDir, fold := seedParent(t)
	barrier := fold.Barriers[0]
	barrier.Vector["sub/s1-a1"] = 2
	_, err := Cut(Options{ParentDir: parentDir, ParentSession: parentSession,
		NewDir: filepath.Join(t.TempDir(), "x"), NewSession: "s", Barrier: barrier,
		Now: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)})
	if err == nil || !strings.Contains(err.Error(), "shorter than barrier vector") {
		t.Fatalf("short child cut err=%v", err)
	}
}

// S7 出口 review P0: a fork of a fork carries exactly ONE genesis — its
// own, naming the IMMEDIATE parent — with session_started right behind it.
func TestCutOfForkKeepsSingleGenesis(t *testing.T) {
	parentDir, fold := seedParent(t)
	fork1Dir := filepath.Join(t.TempDir(), "fork1")
	const fork1 = "20260704-110000-fork-bbbb"
	if _, err := Cut(Options{ParentDir: parentDir, ParentSession: parentSession,
		NewDir: fork1Dir, NewSession: fork1, Barrier: fold.Barriers[1], // bar-t2 @6
		WorkspaceRoot: "/w-fork1", Now: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	f1Events, err := store.ReadEvents(fork1Dir)
	if err != nil {
		t.Fatal(err)
	}
	f1Fold, err := state.Fold(f1Events)
	if err != nil {
		t.Fatal(err)
	}

	fork2Dir := filepath.Join(t.TempDir(), "fork2")
	const fork2 = "20260704-120000-fork-cccc"
	if _, err := Cut(Options{ParentDir: fork1Dir, ParentSession: fork1,
		NewDir: fork2Dir, NewSession: fork2, Barrier: f1Fold.Barriers[0], // bar-t1 in fork1
		WorkspaceRoot: "/w-fork2", Now: time.Date(2026, 7, 4, 13, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	got, err := store.ReadEvents(fork2Dir)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Type != event.TypeForkedFrom || got[1].Type != event.TypeSessionStarted {
		t.Fatalf("head = [%s %s], want [forked_from session_started]", got[0].Type, got[1].Type)
	}
	geneses := 0
	for _, e := range got {
		if e.Type == event.TypeForkedFrom {
			geneses++
		}
	}
	if geneses != 1 {
		t.Fatalf("geneses = %d, want exactly one", geneses)
	}
	for i, e := range got {
		if e.Seq != int64(i+1) {
			t.Fatalf("seq gap at %d: %d", i, e.Seq)
		}
	}
	fold2, err := state.Fold(got)
	if err != nil {
		t.Fatal(err)
	}
	if fold2.Session.ForkedFrom == nil || fold2.Session.ForkedFrom.ParentSession != fork1 {
		t.Errorf("provenance = %+v, want immediate parent %s", fold2.Session.ForkedFrom, fork1)
	}
}

// S7 出口 review P1: the barrier's cancel_at_fork disposition is APPLIED —
// the fork's journal renders the in-flight background work cancelled, so the fold has
// no in-doubt activity and the model sees the outcome.
func TestCutAppliesCancelAtFork(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "parent")
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()
	appendT := func(typ string, payload any) {
		t.Helper()
		env, err := event.New(typ, payload)
		if err != nil {
			t.Fatal(err)
		}
		env.CorrelationID = parentSession
		if _, err := es.Append(env); err != nil {
			t.Fatal(err)
		}
	}
	appendT(event.TypeSessionStarted, &event.SessionStarted{SpecName: "fixer", WorkspaceRoot: "/w"})
	appendT(event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1})
	appendT(event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: "tool-bg1", Kind: event.KindTool, Name: "bash",
		CallID: "bg1", Background: true, Attempt: 1})
	appendT(event.TypeCheckpointBarrier, &event.CheckpointBarrier{
		BarrierID: "bar-t2", GenStep: 2, Vector: map[string]int64{".": 3},
		SnapshotRef: "ref-live",
		Handles:     []event.BarrierHandle{{Handle: "bg1"}}})

	events, _ := store.ReadEvents(dir)
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	newDir := filepath.Join(t.TempDir(), "forked")
	if _, err := Cut(Options{ParentDir: dir, ParentSession: parentSession,
		NewDir: newDir, NewSession: "fork-sess", Barrier: fold.Barriers[0],
		WorkspaceRoot: "/w-fork", Now: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	got, err := store.ReadEvents(newDir)
	if err != nil {
		t.Fatal(err)
	}
	last := got[len(got)-1]
	if last.Type != event.TypeActivityCancelled {
		t.Fatalf("tail = %s, want activity_cancelled (disposition applied)", last.Type)
	}
	fold2, err := state.Fold(got)
	if err != nil {
		t.Fatal(err)
	}
	if len(fold2.Handles) != 0 || len(fold2.Activities) != 0 {
		t.Errorf("fork fold still has handles=%d activities=%d — resume would refuse as in-doubt",
			len(fold2.Handles), len(fold2.Activities))
	}
	var sawOutcome bool
	for _, m := range fold2.Conversation.Messages {
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "bg1") && strings.Contains(p.Text, "canceled") {
				sawOutcome = true
			}
		}
	}
	if !sawOutcome {
		t.Error("cancellation outcome not visible to the model")
	}
}
