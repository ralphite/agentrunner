package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// hashedAssetPath returns the URL of the LARGEST Vite-built /assets/*.js from
// the embedded dist, so the tests never hard-code a content hash. Largest, not
// first: code-splitting (mermaid lazy chunk, INC-51 余项) added glue chunks
// under the 1KB gzip floor, and map order once handed those to the gzip
// negotiation test — the main bundle is always big enough to have a gz form.
func hashedAssetPath(t *testing.T) string {
	t.Helper()
	best, size := "", -1
	for p, a := range assets() {
		if strings.HasPrefix(p, "assets/") && strings.HasSuffix(p, ".js") && len(a.raw) > size {
			best, size = "/"+p, len(a.raw)
		}
	}
	if best == "" {
		t.Skip("embedded dist has no built assets/*.js (placeholder dist?)")
	}
	return best
}

func get(t *testing.T, url string, header map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("GET", url, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	staticHandler().ServeHTTP(rec, req)
	return rec.Result()
}

func TestStaticGzipNegotiation(t *testing.T) {
	p := hashedAssetPath(t)

	res := get(t, p, map[string]string{"Accept-Encoding": "gzip, deflate, br"})
	if res.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", res.Header.Get("Content-Encoding"))
	}
	if res.Header.Get("Vary") != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding", res.Header.Get("Vary"))
	}
	gzBody, _ := io.ReadAll(res.Body)
	if n, _ := strconv.Atoi(res.Header.Get("Content-Length")); n != len(gzBody) {
		t.Errorf("Content-Length = %d, body = %d bytes", n, len(gzBody))
	}
	zr, err := gzip.NewReader(strings.NewReader(string(gzBody)))
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	inflated, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("inflate: %v", err)
	}

	// No Accept-Encoding: identity bytes, no Content-Encoding, honest length.
	plain := get(t, p, nil)
	if ce := plain.Header.Get("Content-Encoding"); ce != "" {
		t.Fatalf("Content-Encoding = %q without Accept-Encoding, want none", ce)
	}
	rawBody, _ := io.ReadAll(plain.Body)
	if n, _ := strconv.Atoi(plain.Header.Get("Content-Length")); n != len(rawBody) {
		t.Errorf("Content-Length = %d, body = %d bytes", n, len(rawBody))
	}
	if string(inflated) != string(rawBody) {
		t.Error("inflated gzip body != identity body")
	}
	if len(gzBody) >= len(rawBody) {
		t.Errorf("gzip body (%d) not smaller than raw (%d)", len(gzBody), len(rawBody))
	}

	// An explicit rejection is honoured.
	if res := get(t, p, map[string]string{"Accept-Encoding": "gzip;q=0"}); res.Header.Get("Content-Encoding") != "" {
		t.Error("gzip;q=0 still got a gzip body")
	}
}

func TestStaticCacheControl(t *testing.T) {
	if cc := get(t, hashedAssetPath(t), nil).Header.Get("Cache-Control"); cc != cacheImmutable {
		t.Errorf("hashed asset Cache-Control = %q, want %q", cc, cacheImmutable)
	}
	// index.html has a stable URL: it must be revalidated or the deploy never
	// reaches an already-warm browser.
	for _, p := range []string{"/", "/index.html", "/favicon.svg", "/foo/bar"} {
		if cc := get(t, p, nil).Header.Get("Cache-Control"); cc != cacheRevalidate {
			t.Errorf("%s Cache-Control = %q, want %q", p, cc, cacheRevalidate)
		}
	}
}

func TestStaticETagRevalidation(t *testing.T) {
	p := hashedAssetPath(t)

	raw := get(t, p, nil)
	rawETag := raw.Header.Get("ETag")
	if !strings.HasPrefix(rawETag, `"`) || !strings.HasSuffix(rawETag, `"`) {
		t.Fatalf("ETag = %q, want a quoted tag", rawETag)
	}
	gz := get(t, p, map[string]string{"Accept-Encoding": "gzip"})
	gzETag := gz.Header.Get("ETag")
	if gzETag == rawETag {
		t.Fatalf("gzip and identity share ETag %q — an intermediary cache would cross the wires", gzETag)
	}

	// Matching tag → 304 with an empty body, per variant.
	for _, tc := range []struct {
		name   string
		header map[string]string
	}{
		{"identity", map[string]string{"If-None-Match": rawETag}},
		{"gzip", map[string]string{"If-None-Match": gzETag, "Accept-Encoding": "gzip"}},
		{"weak", map[string]string{"If-None-Match": "W/" + rawETag}},
		{"list", map[string]string{"If-None-Match": `"stale", ` + rawETag}},
	} {
		res := get(t, p, tc.header)
		if res.StatusCode != http.StatusNotModified {
			t.Errorf("%s: status = %d, want 304", tc.name, res.StatusCode)
		}
		if body, _ := io.ReadAll(res.Body); len(body) != 0 {
			t.Errorf("%s: 304 carried %d body bytes", tc.name, len(body))
		}
	}

	// A stale tag, or the other variant's tag, must re-send the file.
	for _, tc := range []struct {
		name   string
		header map[string]string
	}{
		{"stale", map[string]string{"If-None-Match": `"0000000000000000"`}},
		{"cross-variant", map[string]string{"If-None-Match": gzETag}}, // no Accept-Encoding
	} {
		if res := get(t, p, tc.header); res.StatusCode != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", tc.name, res.StatusCode)
		}
	}
}

func TestStaticSPAFallbackAndContentType(t *testing.T) {
	index, _ := io.ReadAll(get(t, "/index.html", nil).Body)
	if len(index) == 0 {
		t.Fatal("index.html is empty")
	}
	for _, p := range []string{"/", "/foo/bar", "/sessions/20260711-1", "/../etc/passwd"} {
		res := get(t, p, nil)
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (SPA fallback)", p, res.StatusCode)
		}
		body, _ := io.ReadAll(res.Body)
		if string(body) != string(index) {
			t.Errorf("%s: did not fall back to index.html", p)
		}
		if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s: Content-Type = %q, want text/html", p, ct)
		}
	}

	for _, tc := range []struct{ path, wantPrefix string }{
		{hashedAssetPath(t), "text/javascript"},
		{"/index.html", "text/html"},
		{"/favicon.svg", "image/svg+xml"},
	} {
		ct := get(t, tc.path, nil).Header.Get("Content-Type")
		if !strings.HasPrefix(ct, tc.wantPrefix) {
			t.Errorf("%s: Content-Type = %q, want prefix %q", tc.path, ct, tc.wantPrefix)
		}
		if strings.HasPrefix(ct, "text/") && !strings.Contains(ct, "charset=utf-8") {
			t.Errorf("%s: Content-Type = %q, want charset=utf-8", tc.path, ct)
		}
	}
}

func TestStaticCSSIsCompressed(t *testing.T) {
	var css string
	for p := range assets() {
		if strings.HasSuffix(p, ".css") {
			css = "/" + p
		}
	}
	if css == "" {
		t.Skip("no css in the embedded dist")
	}
	res := get(t, css, map[string]string{"Accept-Encoding": "gzip"})
	if res.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("%s: Content-Encoding = %q, want gzip", css, res.Header.Get("Content-Encoding"))
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("%s: Content-Type = %q, want text/css", css, ct)
	}
}
