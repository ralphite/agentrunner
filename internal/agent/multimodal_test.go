package agent

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// v2 M4.1: an input with attached images folds as ref-only image parts —
// the journal and every serialized view stay byte-free (blob-before-event:
// bytes live in the CAS, the fold carries the ref).
func TestFoldImageInputRefOnly(t *testing.T) {
	s := state.New()
	s = mustApply(t, s, event.TypeInputReceived, &event.InputReceived{
		Text: "看看这个报错截图", Source: "user",
		Images: []event.ImageInput{{Ref: "sha256-abc", MediaType: "image/png"}},
	})
	msgs := s.Conversation.Messages
	if len(msgs) != 1 || len(msgs[0].Parts) != 2 {
		t.Fatalf("messages = %+v, want 1 message with text+image parts", msgs)
	}
	img := msgs[0].Parts[1]
	if img.Kind != provider.PartImage || img.Ref != "sha256-abc" || img.MediaType != "image/png" {
		t.Fatalf("image part = %+v", img)
	}
	if len(img.Data) != 0 {
		t.Fatal("fold carries blob bytes; must be ref-only")
	}
	// A serialized fold (snapshot path) must not leak bytes either: Data is
	// json:"-", so no "data" field appears even after inflation elsewhere.
	blob, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, []byte(`"data"`)) {
		t.Error("serialized fold contains a data field")
	}
}

// v2 M4.1: assembly-time inflation loads bytes from the CAS into a COPY of
// the affected message parts; the fold's own parts stay byte-free.
func TestInflateBlobsCopiesNotMutates(t *testing.T) {
	l := testLoop(t, scripted.Fixture{}, t.TempDir())
	arts, err := store.OpenArtifactStore(filepath.Join(l.Store.Dir(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG fake bytes")
	ref, err := arts.Put(png)
	if err != nil {
		t.Fatal(err)
	}
	l.Artifacts = arts

	s := state.New()
	s = mustApply(t, s, event.TypeInputReceived, &event.InputReceived{
		Text: "这是截图", Source: "user",
		Images: []event.ImageInput{{Ref: ref, MediaType: "image/png"}},
	})
	req := Assemble(s, l.Spec, nil, 1)
	if err := l.inflateBlobs(req.Messages); err != nil {
		t.Fatal(err)
	}
	var inflated *provider.Part
	for i := range req.Messages[0].Parts {
		if req.Messages[0].Parts[i].Kind == provider.PartImage {
			inflated = &req.Messages[0].Parts[i]
		}
	}
	if inflated == nil || !bytes.Equal(inflated.Data, png) {
		t.Fatalf("image part not inflated: %+v", inflated)
	}
	// Copy-on-write: the fold's message parts still have no bytes.
	for _, p := range s.Conversation.Messages[0].Parts {
		if len(p.Data) != 0 {
			t.Error("inflation mutated the fold's parts")
		}
	}
	// A dangling ref is a journal-integrity error, not a soft skip.
	s2 := state.New()
	s2 = mustApply(t, s2, event.TypeInputReceived, &event.InputReceived{
		Text: "bad", Source: "user",
		Images: []event.ImageInput{{Ref: "sha256-missing", MediaType: "image/png"}},
	})
	req2 := Assemble(s2, l.Spec, nil, 1)
	if err := l.inflateBlobs(req2.Messages); err == nil {
		t.Error("missing blob inflated without error")
	}
}
