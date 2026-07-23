package messagefork

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

func appendEvent(t *testing.T, es *store.EventStore, typ string, payload any) event.Envelope {
	t.Helper()
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

func TestContinueFromUserMessageCutsBeforeInputAndIsIdempotent(t *testing.T) {
	data := t.TempDir()
	sessions := filepath.Join(data, "sessions")
	parentID := "parent-session"
	parentDir := filepath.Join(sessions, parentID)
	workspace := filepath.Join(data, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	shadow, err := openShadow(data, workspace)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := shadow.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(parentDir)
	if err != nil {
		t.Fatal(err)
	}
	appendEvent(t, es, event.TypeSessionStarted, &event.SessionStarted{
		SpecName: "chat", Model: "scripted", WorkspaceRoot: workspace,
		Spec: []byte(`{"name":"chat"}`), SubStateVersions: state.SubStateVersions(),
	})
	appendEvent(t, es, event.TypeCheckpointBarrier, &event.CheckpointBarrier{
		BarrierID: "bar-user", SnapshotRef: ref, Vector: map[string]int64{".": 1},
		MessageAnchor: &event.MessageAnchor{Side: SideBeforeUser, ItemID: "item-u", TurnID: "turn-u"},
	})
	artifacts, err := store.OpenArtifactStore(filepath.Join(parentDir, "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	blobRef, err := artifacts.Put([]byte("attachment"))
	if err != nil {
		t.Fatal(err)
	}
	appendEvent(t, es, event.TypeInputReceived, &event.InputReceived{
		Text: "draft text", Source: "cli", ItemID: "item-u", TurnID: "turn-u",
		Content: []provider.Part{{Kind: provider.PartText, Text: "draft text"},
			{Kind: provider.PartFile, Ref: blobRef, MediaType: "text/plain", Name: "note.txt"}},
		Files: []event.AttachmentRef{{Ref: blobRef, MediaType: "text/plain", Name: "note.txt"}},
	})
	appendEvent(t, es, event.TypeSessionTitled, &event.SessionTitled{
		Title: "Short parent title", Source: event.TitleSourceAuto,
	})
	if err := es.Close(); err != nil {
		t.Fatal(err)
	}

	req := Request{ParentSession: parentID, ItemID: "item-u", RequestID: "request_12345678"}
	first, err := continueLocked(context.Background(), data, parentDir, req)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || first.Draft == nil || first.Draft.Text != "draft text" {
		t.Fatalf("result = %+v", first)
	}
	childDir := filepath.Join(sessions, first.SessionID)
	childEvents, err := store.ReadEvents(childDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range childEvents {
		if env.Type == event.TypeInputReceived {
			t.Fatal("target input leaked into before-user cut")
		}
	}
	folded, err := state.Fold(childEvents)
	if err != nil {
		t.Fatal(err)
	}
	if folded.ForkPark == nil || folded.Session.ForkedFrom == nil || folded.Session.ForkedFrom.Draft == nil {
		t.Fatalf("child is not durably parked with draft: %+v", folded.Session.ForkedFrom)
	}
	if folded.Session.RawTitle != "Short parent title" || folded.Session.TitleSource != event.TitleSourceFork {
		t.Fatalf("child title/source = %q/%q", folded.Session.RawTitle, folded.Session.TitleSource)
	}
	if folded.Session.ForkedFrom.Draft.Content[1].Data != nil {
		t.Fatal("draft contains raw attachment bytes")
	}
	if _, err := os.Stat(filepath.Join(childDir, "artifacts", "blobs", blobRef)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(firstWorkspace(t, childEvents), "note.txt")); err != nil || string(got) != "before" {
		t.Fatalf("workspace note=%q err=%v", got, err)
	}
	second, err := continueLocked(context.Background(), data, parentDir, req)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created || second.SessionID != first.SessionID {
		t.Fatalf("retry = %+v, first=%+v", second, first)
	}
	_, err = continueLocked(context.Background(), data, parentDir, Request{
		ParentSession: parentID, ItemID: "other", RequestID: req.RequestID,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting request err = %v", err)
	}
}

func firstWorkspace(t *testing.T, events []event.Envelope) string {
	t.Helper()
	for _, env := range events {
		if env.Type != event.TypeForkedFrom {
			continue
		}
		decoded, err := event.DecodePayload(env)
		if err != nil {
			t.Fatal(err)
		}
		return decoded.(*event.ForkedFrom).WorkspaceRoot
	}
	t.Fatal("no fork genesis")
	return ""
}

func TestResolveRejectsNonFinalAndDuplicateAnchors(t *testing.T) {
	lines := []event.Envelope{}
	makeEnv := func(seq int64, typ string, payload any) event.Envelope {
		env, err := event.New(typ, payload)
		if err != nil {
			t.Fatal(err)
		}
		env.Seq, env.ID = seq, event.EventID(seq)
		return env
	}
	lines = append(lines,
		makeEnv(1, event.TypeSessionStarted, &event.SessionStarted{SpecName: "x"}),
		makeEnv(2, event.TypeAssistantMessage, &event.AssistantMessage{GenStep: 1, ItemID: "a", Message: provider.Message{
			Role: provider.RoleAssistant, Parts: []provider.Part{{Kind: provider.PartText, Text: "not final"}},
		}}),
	)
	if _, err := resolve(lines, "a"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-final err=%v", err)
	}
}

func TestAuthorizeDraftPartsPreservesOrderAndAuthoritativeMetadata(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	childID := "20260722-120000-continue-test-abcd1234"
	dir := filepath.Join(xdg, "agentrunner", "sessions", childID)
	artifacts, err := store.OpenArtifactStore(filepath.Join(dir, "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	fileRef, _ := artifacts.Put([]byte("file"))
	imageRef, _ := artifacts.Put([]byte("image"))
	draft := &event.ForkDraft{DraftID: "draft-item-u", Text: "caption", Content: []provider.Part{
		{Kind: provider.PartText, Text: "caption"},
		{Kind: provider.PartFile, Ref: fileRef, MediaType: "text/plain", Name: "authoritative.txt", PartID: "p-file"},
		{Kind: provider.PartImage, Ref: imageRef, MediaType: "image/png", Name: "authoritative.png", PartID: "p-image"},
	}}
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	appendEvent(t, es, event.TypeForkedFrom, &event.ForkedFrom{ParentSession: "parent", SourceItemID: "item-u", Draft: draft})
	appendEvent(t, es, event.TypeSessionStarted, &event.SessionStarted{SpecName: "x"})
	appendEvent(t, es, event.TypeForkAwaitingInput, &event.ForkAwaitingInput{DraftID: draft.DraftID})
	_ = es.Close()

	parts, source, err := AuthorizeDraftParts(childID, draft.DraftID, "caption", []DraftPartRequest{
		{Kind: provider.PartFile, Ref: fileRef, Ordinal: 0}, {Kind: provider.PartImage, Ref: imageRef, Ordinal: 1},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if source != "item-u" || len(parts) != 3 || parts[0].Kind != provider.PartText ||
		parts[1].Name != "authoritative.txt" || parts[1].PartID != "p-file" ||
		parts[2].Name != "authoritative.png" || parts[2].PartID != "p-image" {
		t.Fatalf("authorized exact content = %+v source=%q", parts, source)
	}
	if _, _, err := AuthorizeDraftParts(childID, draft.DraftID, "caption", []DraftPartRequest{
		{Kind: provider.PartImage, Ref: fileRef, Ordinal: 0}, {Kind: provider.PartImage, Ref: imageRef, Ordinal: 1},
	}, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("kind swap err=%v, want invalid", err)
	}
	duplicateDraft := &event.ForkDraft{DraftID: draft.DraftID, Text: draft.Text, Content: []provider.Part{
		{Kind: provider.PartText, Text: "caption"},
		{Kind: provider.PartFile, Ref: fileRef, MediaType: "text/plain", Name: "first.txt", PartID: "first"},
		{Kind: provider.PartFile, Ref: fileRef, MediaType: "text/plain", Name: "second.txt", PartID: "second"},
	}}
	// Replace only the in-memory genesis payload for a focused identity test.
	events, _ := store.ReadEvents(dir)
	genesis, _ := event.New(event.TypeForkedFrom, &event.ForkedFrom{ParentSession: "parent", SourceItemID: "item-u", Draft: duplicateDraft})
	genesis.Seq, genesis.ID = 1, event.EventID(1)
	events[0] = genesis
	raw, _ := json.Marshal(events[0])
	journal, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = journal.Write(append(raw, '\n'))
	for _, e := range events[1:] {
		line, _ := json.Marshal(e)
		_, _ = journal.Write(append(line, '\n'))
	}
	_ = journal.Close()
	kept, _, err := AuthorizeDraftParts(childID, draft.DraftID, "edited", []DraftPartRequest{
		{Kind: provider.PartFile, Ref: fileRef, Ordinal: 1},
	}, false)
	if err != nil || len(kept) != 1 || kept[0].Name != "second.txt" || kept[0].PartID != "second" {
		t.Fatalf("duplicate ref ordinal selection = %+v err=%v", kept, err)
	}
}
