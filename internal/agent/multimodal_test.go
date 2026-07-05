package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/protocol"

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

// v2 M4 (C9 twin): an image attached to a conversational input flows end to
// end — CAS blob before the event, ref-only journal, inflated bytes on the
// provider request — and the model's answer lands normally.
func TestConversationalImageInputEndToEnd(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "两个 worker 一个报错"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "看到截图了:CI 编译失败"}, {Finish: "end_turn"}}},
	}}
	cap := &capturingProvider{inner: scripted.New(fix)}
	inputs := make(chan protocol.UserInput, 1)
	l := testLoop(t, fix, t.TempDir())
	l.Provider = cap
	l.Conversational = true
	l.UserInputs = inputs
	png := []byte("\x89PNG e2e bytes")
	go func() {
		waitAnswers(t, l.Store.Dir(), 1)
		inputs <- protocol.UserInput{Text: "这是 CI 报错截图,帮我看看",
			Images: []protocol.ImageAttachment{{MediaType: "image/png", Data: png}}}
		waitAnswers(t, l.Store.Dir(), 2)
		close(inputs)
	}()
	res, err := l.Run(context.Background(), "先随便聊聊")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" || res.Turns != 2 {
		t.Fatalf("res = %+v", res)
	}

	// Journal: the input event carries the ref, and NO raw/base64 bytes.
	evs, _ := store.ReadEvents(l.Store.Dir())
	var ref string
	for _, e := range evs {
		if e.Type != event.TypeInputReceived {
			continue
		}
		dec, _ := event.DecodePayload(e)
		in := dec.(*event.InputReceived)
		if len(in.Images) > 0 {
			ref = in.Images[0].Ref
			if in.Images[0].MediaType != "image/png" {
				t.Errorf("media type = %q", in.Images[0].MediaType)
			}
		}
	}
	if ref == "" {
		t.Fatal("no journaled image ref")
	}
	b64 := base64.StdEncoding.EncodeToString(png)
	for _, e := range evs {
		if bytes.Contains(e.Payload, png) || bytes.Contains(e.Payload, []byte(b64)) {
			t.Error("journal contains image bytes; must be ref-only")
		}
	}
	// CAS: the ref resolves to the exact bytes (blob-before-event held).
	got, err := l.Artifacts.Get(ref)
	if err != nil || !bytes.Equal(got, png) {
		t.Fatalf("CAS blob = %v, %v", got, err)
	}
	// Wire: the second request's user message carried the INFLATED part.
	last := cap.requests[len(cap.requests)-1]
	var sawInflated bool
	for _, m := range last.Messages {
		for _, p := range m.Parts {
			if p.Kind == provider.PartImage && bytes.Equal(p.Data, png) && p.Ref == ref {
				sawInflated = true
			}
		}
	}
	if !sawInflated {
		t.Error("provider request lacks the inflated image part")
	}
}
