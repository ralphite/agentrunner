package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeAR drops an executable stub `ar` that appends its argv (one line per
// arg) to argsFile and prints a canned reply — enough to prove the handler
// forwards the right command without a real provider call.
func writeFakeAR(t *testing.T, argsFile, reply string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "ar")
	body := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> \"" + argsFile + "\"; done\nprintf '%s\\n' '" + reply + "'\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func postJSON(t *testing.T, h http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

// TestHandleDictateRejectsNonUploadPath is the security guard: the audio path
// becomes a positional arg to `ar dictate`, so a path outside the uploads dir
// (or a "../" escape) must be refused BEFORE any ar spawn — the webui can't be
// steered into transcribing an arbitrary file.
func TestHandleDictateRejectsNonUploadPath(t *testing.T) {
	rt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rt, "uploads"), 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(rt, "secret.wav")
	if err := os.WriteFile(secret, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &server{runtimeDir: rt, arPath: "/nonexistent-ar-should-never-run"}

	for _, p := range []string{
		secret, // sibling of uploads, not inside
		filepath.Join(rt, "uploads", "..", "secret.wav"), // traversal escape
		"",            // empty
		"/etc/passwd", // absolute elsewhere
	} {
		rec := postJSON(t, s.handleDictate, "/api/dictate", `{"path":`+jsonStr(p)+`}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("path %q: code = %d, want 400", p, rec.Code)
		}
	}
}

func TestHandleDictateForwardsToAR(t *testing.T) {
	rt := t.TempDir()
	uploads := filepath.Join(rt, "uploads")
	if err := os.MkdirAll(uploads, 0o755); err != nil {
		t.Fatal(err)
	}
	audio := filepath.Join(uploads, "note.wav")
	if err := os.WriteFile(audio, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	argsFile := filepath.Join(rt, "args.txt")
	s := &server{runtimeDir: rt, arPath: writeFakeAR(t, argsFile, "deploy the kubelet")}

	rec := postJSON(t, s.handleDictate, "/api/dictate", `{"path":`+jsonStr(audio)+`,"context":"Kubernetes"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "deploy the kubelet") {
		t.Errorf("body = %s, want the transcript", rec.Body.String())
	}
	got, _ := os.ReadFile(argsFile)
	args := string(got)
	for _, want := range []string{"dictate", "--context", "Kubernetes", audio} {
		if !strings.Contains(args, want) {
			t.Errorf("forwarded args %q missing %q", args, want)
		}
	}
}

func TestHandleOptimizeForwardsAndGuardsDraft(t *testing.T) {
	rt := t.TempDir()
	argsFile := filepath.Join(rt, "args.txt")
	s := &server{runtimeDir: rt, arPath: writeFakeAR(t, argsFile, "Fix the failing login refresh.")}

	// Empty draft is rejected before any ar spawn.
	if rec := postJSON(t, s.handleOptimize, "/api/optimize", `{"draft":"   "}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty draft code = %d, want 400", rec.Code)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Fatalf("ar was spawned for an empty draft")
	}

	rec := postJSON(t, s.handleOptimize, "/api/optimize", `{"draft":"fix it","context":"internal/auth"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Fix the failing login refresh.") {
		t.Errorf("body = %s, want the rewrite", rec.Body.String())
	}
	got, _ := os.ReadFile(argsFile)
	args := string(got)
	// The draft rides after a "--" so a "-"-leading draft is never a flag.
	for _, want := range []string{"optimize", "--context", "internal/auth", "--", "fix it"} {
		if !strings.Contains(args, want) {
			t.Errorf("forwarded args %q missing %q", args, want)
		}
	}
}

func TestHandleCompactForwardsDirective(t *testing.T) {
	rt := t.TempDir()
	argsFile := filepath.Join(rt, "args.txt")
	s := &server{runtimeDir: rt, arPath: writeFakeAR(t, argsFile, "compact requested")}

	req := httptest.NewRequest("POST", "/api/sessions/sess-1/compact", strings.NewReader(`{"directive":" preserve API decisions "}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("sid", "sess-1")
	rec := httptest.NewRecorder()
	s.handleCompact(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	got, _ := os.ReadFile(argsFile)
	if string(got) != "compact\nsess-1\npreserve API decisions\n" {
		t.Fatalf("args = %q", got)
	}
}

func TestHandleCompactOmitsEmptyDirective(t *testing.T) {
	rt := t.TempDir()
	argsFile := filepath.Join(rt, "args.txt")
	s := &server{runtimeDir: rt, arPath: writeFakeAR(t, argsFile, "compact requested")}

	req := httptest.NewRequest("POST", "/api/sessions/sess-1/compact", strings.NewReader(`{"directive":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("sid", "sess-1")
	rec := httptest.NewRecorder()
	s.handleCompact(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	got, _ := os.ReadFile(argsFile)
	if string(got) != "compact\nsess-1\n" {
		t.Fatalf("args = %q", got)
	}
}

func TestUnderDir(t *testing.T) {
	base := "/data/uploads"
	cases := map[string]bool{
		"/data/uploads/a.wav":      true,
		"/data/uploads":            true, // the dir itself (caller then Stat-rejects it)
		"/data/uploads/../a.wav":   false,
		"/data/secret.wav":         false,
		"/etc/passwd":              false,
		"/data/uploads/sub/b.webm": true,
	}
	for p, want := range cases {
		if got := underDir(base, filepath.Clean(p)); got != want {
			t.Errorf("underDir(%q, %q) = %v, want %v", base, p, got, want)
		}
	}
}

// jsonStr quotes a string as a JSON literal for inline request bodies.
func jsonStr(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}
