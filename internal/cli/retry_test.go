package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

func retryEnv(t *testing.T, cmdID, typ string, payload any) event.Envelope {
	t.Helper()
	e, err := event.New(typ, payload)
	if err != nil {
		t.Fatal(err)
	}
	e.CommandID = cmdID
	return e
}

func TestPlanRetryTargetsLastUserInput(t *testing.T) {
	evs := []event.Envelope{
		retryEnv(t, "cmd-1", event.TypeInputReceived, &event.InputReceived{Text: "first", Source: "cli"}),
		retryEnv(t, "cmd-2", event.TypeInputReceived, &event.InputReceived{Text: "fix the test", Source: "user"}),
		// Program/agent inputs after the user's message must not be targets.
		retryEnv(t, "cmd-3", event.TypeInputReceived, &event.InputReceived{Text: "[goal check]", Source: "program"}),
		retryEnv(t, "cmd-4", event.TypeInputReceived, &event.InputReceived{Text: "[from worker]", Source: "agent"}),
	}
	target, id, err := planRetry(evs, state.New(), false)
	if err != nil {
		t.Fatal(err)
	}
	if target.Text != "fix the test" || id != "retry:cmd-2" {
		t.Fatalf("want last USER input with derived id, got %q id=%q", target.Text, id)
	}
}

func TestPlanRetryGuardsAndLegacy(t *testing.T) {
	user := retryEnv(t, "", event.TypeInputReceived, &event.InputReceived{Text: "hi", Source: ""})
	user.Seq = 7

	// Waiting session: retry would masquerade as the wait's answer.
	s := state.New()
	s.Waiting = &state.Waiting{Kind: "input"}
	if _, _, err := planRetry([]event.Envelope{user}, s, false); err == nil || !strings.Contains(err.Error(), "waiting") {
		t.Fatalf("want waiting guard, got %v", err)
	}
	// Mid-turn with a live writer: double-run risk.
	if _, _, err := planRetry([]event.Envelope{user}, state.New(), true); err == nil || !strings.Contains(err.Error(), "mid-turn") {
		t.Fatalf("want mid-turn guard, got %v", err)
	}
	// No user input at all.
	if _, _, err := planRetry(nil, state.New(), false); err == nil || !strings.Contains(err.Error(), "no user input") {
		t.Fatalf("want no-input error, got %v", err)
	}
	// Legacy journal without command ids falls back to the seq.
	_, id, err := planRetry([]event.Envelope{user}, state.New(), false)
	if err != nil || id != "retry:seq7" {
		t.Fatalf("legacy id: got %q err=%v", id, err)
	}
}

func TestRetryAttachmentsRoundTripCAS(t *testing.T) {
	dir := t.TempDir()
	as, err := store.OpenArtifactStore(filepath.Join(dir, "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	ref, err := as.Put([]byte("png-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	in := &event.InputReceived{Text: "with image",
		Images: []event.AttachmentRef{{Ref: ref, MediaType: "image/png"}}}
	images, files, err := attachmentsFromCAS(dir, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 || len(images) != 1 || string(images[0].Data) != "png-bytes" ||
		images[0].MediaType != "image/png" {
		t.Fatalf("round trip mismatch: %+v", images)
	}
}
