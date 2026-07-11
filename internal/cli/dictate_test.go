package cli

import (
	"bytes"
	"context"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
)

// fakeHelperProvider records every request it serves and replays a canned
// reply — enough to drive the tool-less one-shot calls dictate/optimize make.
type fakeHelperProvider struct {
	reply    string
	err      error
	requests []provider.CompleteRequest
}

func (f *fakeHelperProvider) Capabilities() provider.Capabilities { return provider.Capabilities{} }

func (f *fakeHelperProvider) Complete(_ context.Context, req provider.CompleteRequest) iter.Seq2[provider.StreamEvent, error] {
	f.requests = append(f.requests, req)
	return func(yield func(provider.StreamEvent, error) bool) {
		if f.err != nil {
			yield(provider.StreamEvent{}, f.err)
			return
		}
		if !yield(provider.StreamEvent{Kind: provider.EventTextDelta, TextDelta: f.reply}, nil) {
			return
		}
		yield(provider.StreamEvent{Kind: provider.EventFinish, Finish: provider.FinishEndTurn}, nil)
	}
}

func fakeFactory(p provider.Provider) providerFactory {
	return func(context.Context, string) (provider.Provider, error) { return p, nil }
}

// TestDictateEncodesAudioPartAndContext is the round-trip arrow: a recording on
// disk becomes a PartAudio with its bytes + MIME intact, the caller's context
// rides the system prompt for proper-noun disambiguation, and only the
// transcript reaches stdout.
func TestDictateEncodesAudioPartAndContext(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "note.wav")
	bytesIn := []byte("RIFF\x00\x00\x00\x00WAVEfmt ")
	if err := os.WriteFile(audio, bytesIn, 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeHelperProvider{reply: "  deploy the kubelet on cluster-A  \n"}
	var out, errb bytes.Buffer
	code := runDictate(dictateOptions{
		audioPath: audio,
		model:     "test-model",
		context:   "Kubernetes, kubelet, cluster-A",
		factory:   fakeFactory(fake),
		stdout:    &out,
		stderr:    &errb,
	})
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errb.String())
	}
	// stdout is the transcript alone, trimmed — the webui captures it verbatim.
	if got := out.String(); got != "deploy the kubelet on cluster-A\n" {
		t.Fatalf("stdout = %q, want the trimmed transcript", got)
	}
	if len(fake.requests) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(fake.requests))
	}
	req := fake.requests[0]
	if req.Model != "test-model" {
		t.Errorf("model = %q, want test-model", req.Model)
	}
	// The audio part carries the file bytes + inferred MIME, unmutated.
	var audioPart *provider.Part
	for i := range req.Messages[0].Parts {
		if req.Messages[0].Parts[i].Kind == provider.PartAudio {
			audioPart = &req.Messages[0].Parts[i]
		}
	}
	if audioPart == nil {
		t.Fatalf("no audio part in request: %+v", req.Messages[0].Parts)
	}
	if audioPart.MediaType != "audio/wav" {
		t.Errorf("audio MIME = %q, want audio/wav", audioPart.MediaType)
	}
	if !bytes.Equal(audioPart.Data, bytesIn) {
		t.Errorf("audio bytes not carried verbatim")
	}
	// The context rides the system prompt so proper nouns get spelled right.
	if !strings.Contains(req.System, "cluster-A") {
		t.Errorf("context hint missing from system prompt: %q", req.System)
	}
	// A tool-less turn — audio is a helper input, never a tool call.
	if len(req.Tools) != 0 {
		t.Errorf("dictate issued tools: %+v", req.Tools)
	}
}

// TestDictateRejectsOversizeAudio: the size cap is enforced BEFORE any provider
// call, so an abusive upload never reaches (or bills) the model.
func TestDictateRejectsOversizeAudio(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "big.wav")
	if err := os.WriteFile(audio, make([]byte, 128), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeHelperProvider{reply: "should not be called"}
	var out, errb bytes.Buffer
	code := runDictate(dictateOptions{
		audioPath: audio,
		maxBytes:  64, // smaller than the file
		factory:   fakeFactory(fake),
		stdout:    &out,
		stderr:    &errb,
	})
	if code != ExitUsage {
		t.Fatalf("exit = %d, want ExitUsage for oversize audio", code)
	}
	if len(fake.requests) != 0 {
		t.Fatalf("provider was called %d time(s) despite oversize reject", len(fake.requests))
	}
	if !strings.Contains(errb.String(), "limit") {
		t.Errorf("stderr = %q, want it to name the limit", errb.String())
	}
}

// TestDictateUnknownMIMENeedsFlag: an extension we can't map is a clean usage
// error asking for --mime, not a raw provider failure.
func TestDictateUnknownMIMENeedsFlag(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "clip.xyz")
	if err := os.WriteFile(audio, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeHelperProvider{reply: "x"}
	var out, errb bytes.Buffer
	code := runDictate(dictateOptions{audioPath: audio, factory: fakeFactory(fake), stdout: &out, stderr: &errb})
	if code != ExitUsage {
		t.Fatalf("exit = %d, want ExitUsage", code)
	}
	if !strings.Contains(errb.String(), "--mime") {
		t.Errorf("stderr = %q, want a --mime hint", errb.String())
	}
	// An explicit --mime overrides the missing inference and reaches the model.
	out.Reset()
	errb.Reset()
	code = runDictate(dictateOptions{audioPath: audio, mime: "audio/ogg", factory: fakeFactory(fake), stdout: &out, stderr: &errb})
	if code != ExitOK {
		t.Fatalf("exit = %d with explicit --mime, stderr = %q", code, errb.String())
	}
	if fake.requests[0].Messages[0].Parts[1].MediaType != "audio/ogg" {
		t.Errorf("explicit --mime not honored: %+v", fake.requests[0].Messages[0].Parts[1])
	}
}

func TestDictateMissingAndEmptyFile(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runDictate(dictateOptions{audioPath: "/no/such/file.wav", factory: fakeFactory(&fakeHelperProvider{}), stdout: &out, stderr: &errb}); code != ExitUsage {
		t.Errorf("missing file exit = %d, want ExitUsage", code)
	}
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.wav")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	errb.Reset()
	if code := runDictate(dictateOptions{audioPath: empty, factory: fakeFactory(&fakeHelperProvider{}), stdout: &out, stderr: &errb}); code != ExitUsage {
		t.Errorf("empty file exit = %d, want ExitUsage", code)
	}
}

func TestDictateCmdUsage(t *testing.T) {
	var out, errb bytes.Buffer
	if code := dictateCmd(nil, &out, &errb); code != ExitUsage {
		t.Errorf("no-arg dictate exit = %d, want ExitUsage", code)
	}
	if !strings.Contains(errb.String(), "usage:") {
		t.Errorf("stderr = %q, want a usage line", errb.String())
	}
}

func TestAudioMIMEInference(t *testing.T) {
	cases := map[string]string{
		"a.wav": "audio/wav", "a.mp3": "audio/mp3", "a.ogg": "audio/ogg",
		"a.opus": "audio/ogg", "a.flac": "audio/flac", "a.m4a": "audio/mp4",
		"a.webm": "audio/webm", "a.aac": "audio/aac", "a.aiff": "audio/aiff",
		"a.WAV": "audio/wav", "a.txt": "", "noext": "",
	}
	for name, want := range cases {
		if got := audioMIME(name); got != want {
			t.Errorf("audioMIME(%q) = %q, want %q", name, got, want)
		}
	}
}
