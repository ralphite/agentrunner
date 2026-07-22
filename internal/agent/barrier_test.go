package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
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

func regularBarriers(all []*event.CheckpointBarrier) []*event.CheckpointBarrier {
	var out []*event.CheckpointBarrier
	for _, b := range all {
		if b.MessageAnchor == nil {
			out = append(out, b)
		}
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

	allBarriers := decodedBarriers(t, l.Store.Dir())
	barriers := regularBarriers(allBarriers)
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
	var messageAnchors int
	for _, b := range allBarriers {
		if b.MessageAnchor != nil && b.MessageAnchor.Side == "after_assistant" {
			messageAnchors++
		}
	}
	if messageAnchors != 1 {
		t.Errorf("after-assistant anchors = %d, want 1", messageAnchors)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if len(fold.Barriers) != 4 {
		t.Fatalf("fold.Barriers = %d, want 4 (3 regular + message anchor)", len(fold.Barriers))
	}
	var regularFold []state.Barrier
	for _, b := range fold.Barriers {
		if b.MessageAnchor == nil {
			regularFold = append(regularFold, b)
		}
	}
	for i, b := range regularFold {
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
	barriers := regularBarriers(decodedBarriers(t, l.Store.Dir()))
	if len(barriers) != 4 {
		var ids []string
		for _, b := range barriers {
			ids = append(ids, b.BarrierID)
		}
		t.Fatalf("barriers = %v, want [bar-t1 bar-t2 bar-t3 bar-final]", ids)
	}
	t2 := barriers[1]
	if len(t2.Handles) != 1 || t2.Handles[0].Handle != "bg1" {
		t.Errorf("bar-t2 handles = %+v, want [{bg1}] (fork cancels it — PLAN 5.9)", t2.Handles)
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

// INC-91: a durable human opening is cut before its InputReceived, and an
// eligible final answer is cut immediately after its AssistantMessage.
func TestMessageAnchorsBracketHumanAndFinalAssistant(t *testing.T) {
	root := t.TempDir()
	l := testLoop(t, scripted.Fixture{Steps: []scripted.Step{{Respond: []scripted.Event{
		{Text: "answer"}, {Finish: "end_turn"},
	}}}}, root)
	l.DurableOpening = true
	withShadowRepo(t, l, root)
	if _, err := l.Run(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var inputSeq, assistantSeq int64
	anchors := map[string]*event.CheckpointBarrier{}
	for _, env := range events {
		decoded, err := event.DecodePayload(env)
		if err != nil {
			t.Fatal(err)
		}
		switch p := decoded.(type) {
		case *event.InputReceived:
			inputSeq = env.Seq
			if p.ItemID == "" {
				t.Error("durable opening has no item id")
			}
		case *event.AssistantMessage:
			assistantSeq = env.Seq
			if p.ContinuationCheckpoint == nil {
				t.Error("eligible final assistant has no repair receipt")
			}
		case *event.CheckpointBarrier:
			if p.MessageAnchor != nil {
				anchors[p.MessageAnchor.Side] = p
			}
		}
	}
	before, after := anchors["before_user"], anchors["after_assistant"]
	if before == nil || after == nil {
		t.Fatalf("message anchors = %+v", anchors)
	}
	if before.Vector["."] >= inputSeq {
		t.Errorf("before-user vector = %d, input seq = %d", before.Vector["."], inputSeq)
	}
	if after.Vector["."] != assistantSeq {
		t.Errorf("after-assistant vector = %d, assistant seq = %d", after.Vector["."], assistantSeq)
	}
}

func TestInputBarrierSnapshotFailureDoesNotBlockInput(t *testing.T) {
	l := testLoop(t, scripted.Fixture{Steps: []scripted.Step{{Respond: []scripted.Event{
		{Text: "answer"}, {Finish: "end_turn"},
	}}}}, t.TempDir())
	l.DurableOpening = true
	l.Snapshots = snapshot.None{}
	if _, err := l.Run(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var inputs, messageAnchors int
	for _, env := range events {
		if env.Type == event.TypeInputReceived {
			inputs++
		}
		if env.Type == event.TypeCheckpointBarrier {
			decoded, _ := event.DecodePayload(env)
			if decoded.(*event.CheckpointBarrier).MessageAnchor != nil {
				messageAnchors++
			}
		}
	}
	if inputs != 1 || messageAnchors != 0 {
		t.Fatalf("inputs=%d messageAnchors=%d, want 1/0", inputs, messageAnchors)
	}
}

func TestResumeRestoresDurableOpeningCommandFromGenesis(t *testing.T) {
	root := t.TempDir()
	l := testLoop(t, scripted.Fixture{Steps: []scripted.Step{{Respond: []scripted.Event{
		{Text: "recovered"}, {Finish: "end_turn"},
	}}}}, root)
	if err := l.ensureArtifacts(); err != nil {
		t.Fatal(err)
	}
	ref, err := l.Artifacts.Put([]byte("attachment"))
	if err != nil {
		t.Fatal(err)
	}
	materializeRef, err := l.Artifacts.Put([]byte("bootstrap file"))
	if err != nil {
		t.Fatal(err)
	}
	withShadowRepo(t, l, root)
	env, err := event.New(event.TypeSessionStarted, &event.SessionStarted{
		SpecName: "test", Model: "scripted", Prompt: "opening",
		SubStateVersions: state.SubStateVersions(), OpeningCommandID: "cmd-opening-recovery",
		Inputs:        []event.ArtifactInput{{Ref: materializeRef, Path: "bootstrap.txt"}},
		OpeningItemID: "item-cmd-opening-recovery", OpeningContent: []provider.Part{
			{Kind: provider.PartText, Text: "opening"},
			{Kind: provider.PartFile, Ref: ref, MediaType: "text/plain", Name: "note.txt", PartID: "part-file"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Store.Append(env); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	commands, err := store.ReadCommands(l.Store.Dir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].CommandID != "cmd-opening-recovery" {
		t.Fatalf("restored commands = %+v", commands)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var got *event.InputReceived
	var materializedSeq, barrierSeq, inputSeq int64
	for _, e := range events {
		if e.Type == event.TypeActivityCompleted {
			decoded, _ := event.DecodePayload(e)
			if decoded.(*event.ActivityCompleted).ActivityID == "materialize" {
				materializedSeq = e.Seq
			}
		}
		if e.Type == event.TypeCheckpointBarrier {
			decoded, _ := event.DecodePayload(e)
			if anchor := decoded.(*event.CheckpointBarrier).MessageAnchor; anchor != nil && anchor.Side == "before_user" {
				barrierSeq = e.Seq
			}
		}
		if e.Type == event.TypeInputReceived {
			decoded, _ := event.DecodePayload(e)
			got = decoded.(*event.InputReceived)
			inputSeq = e.Seq
		}
	}
	if got == nil || got.ItemID != "item-cmd-opening-recovery" || len(got.Content) != 2 ||
		got.Content[1].Name != "note.txt" || got.Content[1].PartID != "part-file" {
		t.Fatalf("recovered opening = %+v", got)
	}
	if materializedSeq == 0 || barrierSeq == 0 || inputSeq == 0 ||
		materializedSeq >= barrierSeq || barrierSeq >= inputSeq {
		t.Fatalf("bootstrap ordering materialized=%d barrier=%d input=%d", materializedSeq, barrierSeq, inputSeq)
	}
}

func TestForkedBeforeOpeningDoesNotRebuildParentOpeningCommand(t *testing.T) {
	l := testLoop(t, scripted.Fixture{}, t.TempDir())
	appendRaw := func(typ string, payload any) {
		env, _ := event.New(typ, payload)
		if _, err := l.Store.Append(env); err != nil {
			t.Fatal(err)
		}
	}
	appendRaw(event.TypeForkedFrom, &event.ForkedFrom{ParentSession: "parent", SourceItemID: "item-opening"})
	appendRaw(event.TypeSessionStarted, &event.SessionStarted{SpecName: "test", Prompt: "parent opening",
		SubStateVersions: state.SubStateVersions(), OpeningCommandID: "cmd-parent-opening",
		OpeningItemID: "item-opening", OpeningContent: []provider.Part{{Kind: provider.PartText, Text: "parent opening"}}})
	appendRaw(event.TypeForkAwaitingInput, &event.ForkAwaitingInput{RequestID: "request_12345678", DraftID: "draft-item-opening"})
	res, err := l.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "waiting_input" {
		t.Fatalf("result = %+v", res)
	}
	commands, err := store.ReadCommands(l.Store.Dir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 0 {
		t.Fatalf("fork rebuilt parent opening commands: %+v", commands)
	}
}

func TestFinalAssistantAnchorCrashRepairIsFirstAppend(t *testing.T) {
	dir := t.TempDir()
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	appendRaw := func(typ string, payload any) event.Envelope {
		env, err := event.New(typ, payload)
		if err != nil {
			t.Fatal(err)
		}
		env, err = es.Append(env)
		if err != nil {
			t.Fatal(err)
		}
		return env
	}
	appendRaw(event.TypeSessionStarted, &event.SessionStarted{SpecName: "x", Model: "m"})
	assistant := appendRaw(event.TypeAssistantMessage, &event.AssistantMessage{
		GenStep: 1, TurnID: "turn-1", ItemID: "item-a", Message: provider.Message{
			Role: provider.RoleAssistant, Parts: []provider.Part{{Kind: provider.PartText, Text: "done"}},
		}, ContinuationCheckpoint: &event.ContinuationCheckpoint{
			SnapshotRef: "ref-1", Vector: map[string]int64{"sub/c": 9},
		},
	})
	initial, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	folded, err := state.Fold(initial)
	if err != nil {
		t.Fatal(err)
	}
	l := &Loop{Store: es, SessionID: "s"}
	ds := &driveState{s: folded, lastID: assistant.ID}
	if err := l.repairAssistantMessageBarrier(ds, l.appender(ds), initial); err != nil {
		t.Fatal(err)
	}
	all, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	last := all[len(all)-1]
	if last.Type != event.TypeCheckpointBarrier || last.Seq != assistant.Seq+1 {
		t.Fatalf("repair = seq %d %s, want immediate barrier", last.Seq, last.Type)
	}
	decoded, _ := event.DecodePayload(last)
	b := decoded.(*event.CheckpointBarrier)
	if b.Vector["."] != assistant.Seq || b.Vector["sub/c"] != 9 || b.MessageAnchor.ItemID != "item-a" {
		t.Fatalf("repair barrier = %+v", b)
	}
	_ = es.Close()
}

func TestContinuationEligibilityRejectsMaxTokensAndToolCalls(t *testing.T) {
	visible := provider.Message{Role: provider.RoleAssistant,
		Parts: []provider.Part{{Kind: provider.PartText, Text: "visible"}}}
	if !eligibleFinalAssistant(provider.GenStep{Message: visible, Finish: provider.FinishEndTurn}) ||
		!eligibleFinalAssistant(provider.GenStep{Message: visible, Finish: provider.FinishBlocked}) {
		t.Fatal("normal and blocked visible finals must be eligible")
	}
	if eligibleFinalAssistant(provider.GenStep{Message: visible, Finish: provider.FinishMaxTokens}) {
		t.Fatal("truncated max-token output must not be a continuation target")
	}
	withTool := provider.Message{Role: provider.RoleAssistant, Parts: []provider.Part{
		{Kind: provider.PartText, Text: "working"},
		{Kind: provider.PartToolCall, CallID: "c1", ToolName: "read_file"},
	}}
	if eligibleFinalAssistant(provider.GenStep{Message: withTool, Finish: provider.FinishEndTurn}) {
		t.Fatal("assistant output with a pending tool call must not be eligible")
	}
}
