package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// pngBytes is a real 2×2 PNG — DetectContentType sniffs the magic bytes, so the
// fixture must be an actual image, not a label.
func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// seedBlob plants a blob under the fake data dir exactly where the daemon's
// ArtifactStore puts it: <data>/sessions/<sid>/artifacts/blobs/<ref>.
func seedBlob(t *testing.T, sid, ref string, content []byte) {
	t.Helper()
	dir := filepath.Join(os.Getenv("XDG_DATA_HOME"), "agentrunner", "sessions", sid, "artifacts", "blobs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ref), content, 0o644); err != nil {
		t.Fatal(err)
	}
}

const testRef = "sha256-eaf9cd193327401850ad147ff99582b9d2a2851701b10f3eb81bc35ea45544b6"

// imageReq builds the request the {sid}/image/{ref} route would dispatch. The
// handler is exercised directly (not through routes(), which needs the built
// SPA bundle embedded).
func imageReq(sid, ref string) *http.Request {
	r := httptest.NewRequest("GET", "/api/sessions/x/image/y", nil)
	r.SetPathValue("sid", sid)
	r.SetPathValue("ref", ref)
	return r
}

// TestSessionImageServesDurableBlob pins RT-6: the image a user attached is
// served from the session's durable artifact store, so a thumbnail survives a
// reload (the uploads dir behind /api/uploads is per-browser-session).
func TestSessionImageServesDurableBlob(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	want := pngBytes(t)
	seedBlob(t, "20260710-task-abcd", testRef, want)

	s := &server{}
	rec := httptest.NewRecorder()
	s.handleSessionImage(rec, imageReq("20260710-task-abcd", testRef))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type = %q, want image/png", ct)
	}
	if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatalf("body is not the blob bytes (%d vs %d)", rec.Body.Len(), len(want))
	}
	if cc := rec.Header().Get("Cache-Control"); cc == "" {
		t.Fatal("content-addressed blob should be cacheable")
	}
}

// TestSessionImageRejectsBadRefs pins the path jail: only a well-formed CAS ref
// resolves, so no traversal (and no absolute path) can be smuggled through the
// {ref} segment, and no session id can climb out of the sessions dir.
func TestSessionImageRejectsBadRefs(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	seedBlob(t, "sess", testRef, pngBytes(t))
	secret := filepath.Join(os.Getenv("XDG_DATA_HOME"), "agentrunner", "trusted.yaml")
	if err := os.WriteFile(secret, []byte("top: secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &server{}
	for _, c := range []struct{ sid, ref string }{
		{"sess", "../../../trusted.yaml"},
		{"sess", "sha256-.."},
		{"sess", "sha256-" + testRef}, // not 64 hex chars
		{"sess", "SHA256-EAF9CD19"},   // uppercase is not what the store mints
		{"sess", "sha256-zz"},
		{"..", testRef},
		{"../..", testRef},
		{"sess/../..", testRef},
	} {
		rec := httptest.NewRecorder()
		s.handleSessionImage(rec, imageReq(c.sid, c.ref))
		if rec.Code == http.StatusOK {
			t.Fatalf("sid=%q ref=%q: served 200; a malformed ref/id must never resolve", c.sid, c.ref)
		}
	}
}

// TestSessionImageMissingBlobIs404 pins the fallback contract: a ref whose blob
// is gone answers 404 (the view then falls back to a text stub) — never a 200
// with an empty body, which would paint a broken thumbnail.
func TestSessionImageMissingBlobIs404(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	s := &server{}
	rec := httptest.NewRecorder()
	s.handleSessionImage(rec, imageReq("sess", testRef))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestSessionImageRefusesNonImage pins the XSS guard: the same blob store holds
// model-authored artifacts, and serving one as text/html from this origin would
// make a run's output executable in the user's browser.
func TestSessionImageRefusesNonImage(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	seedBlob(t, "sess", testRef, []byte("<html><script>alert(1)</script></html>"))
	s := &server{}
	rec := httptest.NewRecorder()
	s.handleSessionImage(rec, imageReq("sess", testRef))
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}
}
