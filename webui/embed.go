package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

// distFS holds the Vite-built React frontend. `frontend/dist` must contain at
// least an index.html at compile time (a committed placeholder suffices until
// the real build runs — see frontend/README).
//
//go:embed all:frontend/dist
var distFS embed.FS

const (
	// Vite stamps /assets/* filenames with a content hash, so those bytes are
	// immutable for the lifetime of their URL. Everything else (index.html,
	// favicon.svg) keeps a stable URL across releases and MUST be revalidated,
	// or a browser that cached the old shell never sees a new deploy again.
	cacheImmutable  = "public, max-age=31536000, immutable"
	cacheRevalidate = "no-cache"

	// Below ~1KB gzip's own framing eats the win, and the round-trip is one
	// packet either way.
	minGzipSize = 1024
)

var gzipExts = map[string]bool{
	".js": true, ".css": true, ".html": true, ".svg": true, ".json": true,
}

// asset is one embedded file with everything the response needs precomputed.
// The bytes are fixed at compile time, so compressing and hashing per request
// would burn CPU for an answer that can never change.
type asset struct {
	raw          []byte
	gz           []byte // nil when the file is not worth compressing
	etag         string // quoted, e.g. `"a1b2…"`
	gzETag       string // distinct from etag: a cache keyed on one must never serve the other
	contentType  string
	cacheControl string
}

// assets is built once per process: staticHandler() may be called by every test
// that constructs routes(), and gzipping the ~900KB bundle at BestCompression
// is not something to repeat.
var assets = sync.OnceValue(loadAssets)

// staticHandler serves the embedded SPA: real asset paths resolve to files,
// everything else falls back to index.html (client-side routing / hash nav).
// Assets are served pre-gzipped, cache-tagged and revalidatable (ETag/304).
func staticHandler() http.Handler {
	files := assets()
	index, ok := files["index.html"]
	if !ok {
		panic("arwebui: frontend/dist/index.html missing from the embedded FS")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := files[strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")]
		if !ok {
			// Unknown path: hand back index.html so the SPA can route it.
			a = index
		}
		serveAsset(w, r, a)
	})
}

func serveAsset(w http.ResponseWriter, r *http.Request, a *asset) {
	h := w.Header()
	h.Set("Content-Type", a.contentType)
	h.Set("Cache-Control", a.cacheControl)
	h.Set("Vary", "Accept-Encoding")

	body, etag := a.raw, a.etag
	if a.gz != nil && acceptsGzip(r.Header.Get("Accept-Encoding")) {
		body, etag = a.gz, a.gzETag
		h.Set("Content-Encoding", "gzip")
	}
	h.Set("ETag", etag)
	if etagMatch(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// acceptsGzip honours an explicit `gzip;q=0` rejection; any other appearance of
// the token is a yes.
func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(token), "gzip") {
			continue
		}
		if q, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			if v, err := strconv.ParseFloat(q, 64); err == nil && v == 0 {
				return false
			}
		}
		return true
	}
	return false
}

// etagMatch implements If-None-Match against the one ETag we would have sent
// (weak-compare per RFC 9110 §13.1.2: the W/ prefix does not defeat a match).
func etagMatch(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
}

func loadAssets() map[string]*asset {
	sub, err := fs.Sub(distFS, "frontend/dist")
	if err != nil {
		panic(err)
	}
	files := map[string]*asset{}
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		files[p] = newAsset(p, raw)
		return nil
	})
	if err != nil {
		panic(err)
	}
	return files
}

func newAsset(p string, raw []byte) *asset {
	ext := path.Ext(p)
	sum := sha256.Sum256(raw)
	tag := hex.EncodeToString(sum[:])[:16]
	a := &asset{
		raw:          raw,
		etag:         `"` + tag + `"`,
		gzETag:       `"` + tag + `-gzip"`,
		contentType:  contentType(ext, raw),
		cacheControl: cacheRevalidate,
	}
	if strings.HasPrefix(p, "assets/") {
		a.cacheControl = cacheImmutable
	}
	if gzipExts[ext] && len(raw) > minGzipSize {
		if gz := gzipBytes(raw); len(gz) < len(raw) {
			a.gz = gz
		}
	}
	return a
}

// contentType keeps the charset that http.FileServer's sniffing used to supply:
// mime.TypeByExtension merges the OS mime table on some hosts and can come back
// charset-less for .js/.json, which makes browsers guess the encoding.
func contentType(ext string, raw []byte) string {
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		return http.DetectContentType(raw)
	}
	textual := strings.HasPrefix(ct, "text/") ||
		strings.HasPrefix(ct, "application/javascript") ||
		strings.HasPrefix(ct, "application/json")
	if textual && !strings.Contains(ct, "charset") {
		ct += "; charset=utf-8"
	}
	return ct
}

func gzipBytes(raw []byte) []byte {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		panic(err) // BestCompression is a valid level; anything else is a bug
	}
	if _, err := zw.Write(raw); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
