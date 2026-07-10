package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// realPNG carries the true PNG signature so the executor sniffs image/png.
var realPNG = append([]byte("\x89PNG\r\n\x1a\n"), []byte("tool-side-media")...)

// mediaResultPart maps ONLY read_file envelopes; everything else is inert.
func TestMediaResultPartGating(t *testing.T) {
	env := []byte(`{"kind":"image","media_type":"image/png","ref":"blob-1"}`)
	if p, ok := mediaResultPart("read_file", env); !ok || p.Kind != provider.PartImage || p.Ref != "blob-1" {
		t.Fatalf("read_file envelope not mapped: %v %v", p, ok)
	}
	if _, ok := mediaResultPart("some_mcp_tool", env); ok {
		t.Error("non-read_file tool grew a media part")
	}
	if _, ok := mediaResultPart("read_file", []byte(`{"content":"plain"}`)); ok {
		t.Error("plain text result grew a media part")
	}
	if _, ok := mediaResultPart("read_file", []byte(`{"kind":"image","media_type":"image/png"}`)); ok {
		t.Error("ref-less envelope grew a media part")
	}
	pdf := []byte(`{"kind":"file","media_type":"application/pdf","ref":"blob-2"}`)
	if p, ok := mediaResultPart("read_file", pdf); !ok || p.Kind != provider.PartFile {
		t.Errorf("pdf envelope = %v %v", p, ok)
	}
}

// TestReadFileImageEndToEnd (INC-33, #32 tool side): the model READS an image
// with read_file; the journaled tool result carries only the CAS ref, and the
// next provider request carries the inflated image part on the tool-result
// message — the model actually sees the pixels.
func TestReadFileImageEndToEnd(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "shot.png"), realPNG, 0o644); err != nil {
		t.Fatal(err)
	}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "read_file", Args: map[string]any{"path": "shot.png"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "I can see the screenshot"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, root)
	cap := &capturingProvider{inner: scripted.New(fix)}
	l.Provider = cap

	if _, err := l.Run(context.Background(), "look at shot.png and describe it"); err != nil {
		t.Fatal(err)
	}

	// Journal: the tool result is a ref envelope; NO raw or base64 bytes.
	evs, _ := store.ReadEvents(l.Store.Dir())
	b64 := base64.StdEncoding.EncodeToString(realPNG)
	for _, e := range evs {
		if bytes.Contains(e.Payload, []byte("tool-side-media")) || bytes.Contains(e.Payload, []byte(b64)) {
			t.Fatal("journal contains media bytes; must be ref-only")
		}
	}
	st, err := state.Fold(evs)
	if err != nil {
		t.Fatal(err)
	}
	var ref string
	for _, tr := range st.Conversation.ToolResults {
		var env struct {
			Kind string `json:"kind"`
			Ref  string `json:"ref"`
		}
		if json.Unmarshal(tr.Result, &env) == nil && env.Kind == "image" {
			ref = env.Ref
		}
	}
	if ref == "" {
		t.Fatal("no image envelope in the folded tool results")
	}
	// CAS: the ref resolves to the exact bytes (blob-before-event held).
	got, err := l.Artifacts.Get(ref)
	if err != nil || !bytes.Equal(got, realPNG) {
		t.Fatalf("CAS blob = %d bytes, %v", len(got), err)
	}
	// Wire: the SECOND request's tool message carries tool_result first and
	// the inflated image part after it.
	requests := cap.Requests()
	last := requests[len(requests)-1]
	var sawResult, sawImage bool
	for _, m := range last.Messages {
		imageAfterResult := false
		for _, p := range m.Parts {
			if p.Kind == provider.PartToolResult && p.ToolName == "read_file" {
				sawResult = true
				imageAfterResult = true
			}
			if p.Kind == provider.PartImage {
				if !imageAfterResult {
					t.Error("image part precedes the tool_result part (Anthropic block order)")
				}
				if !bytes.Equal(p.Data, realPNG) || p.Ref != ref {
					t.Errorf("image part not inflated correctly: ref=%q data=%d bytes", p.Ref, len(p.Data))
				}
				sawImage = true
			}
		}
	}
	if !sawResult || !sawImage {
		t.Fatalf("second request lacks tool_result+image (result=%v image=%v)", sawResult, sawImage)
	}
}
