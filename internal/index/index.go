// Package index is the IndexStore (S7 模块 4): the FOURTH state class —
// a derived index rebuildable from the workspace at any time. Deleting it
// loses re-index time and nothing else, so it lives OUTSIDE the run's
// sub-state version set (same doctrine as the driver/notifier streams),
// is never journaled, never snapshotted, and never travels with a fork.
//
// The v0 backend is lexical (BM25 over line chunks with identifier-aware
// tokenization): deterministic, offline, credential-free. The Indexer is
// the resident-actor seam — refresh and query serialize through it — and
// an embedding backend can replace the scoring without touching callers.
package index

import (
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
)

const (
	chunkLines   = 30
	maxFileBytes = 512 * 1024
	sniffBytes   = 8 * 1024
	// DefaultK and MaxK bound how many hits a query returns.
	DefaultK = 8
	MaxK     = 20
)

// skipDirs are derived/vendored trees that would drown the signal.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".venv": true,
	"venv": true, "dist": true, "build": true, "target": true,
	"__pycache__": true, ".ssh": true, ".aws": true,
}

// skipFiles are credential-shaped paths (kept in LOCKSTEP with the
// snapshot hard excludes): their content must never surface in search
// snippets, which land verbatim in the journal.
var skipFileNames = map[string]bool{
	".env": true, ".envrc": true, ".git-credentials": true, ".netrc": true,
	".npmrc": true, ".pypirc": true, "credentials.json": true,
}

func skipFile(name string) bool {
	if skipFileNames[name] || strings.HasPrefix(name, ".env.") {
		return true
	}
	for _, pat := range []string{"*.pem", "*.key", "id_rsa*", "id_ed25519*"} {
		if ok, _ := filepath.Match(pat, name); ok {
			return true
		}
	}
	return false
}

// SkipFile reports whether a file name is credential-shaped and must never
// have its content surface in a journaled tool result. Exported so the grep
// tool stays in GENUINE lockstep with semantic_search (both land verbatim
// content in the journal) rather than copy-pasting the exclusion set.
func SkipFile(name string) bool { return skipFile(name) }

// SkipDir reports whether a directory should be excluded from a
// content-surfacing walk: derived/vendored trees and dotdirs (which harbor
// credential stores like .ssh/.aws). Shared with the grep/glob tools.
func SkipDir(name string) bool { return skipDirs[name] || strings.HasPrefix(name, ".") }

// Hit is one search result. Line is the 1-based first line of the chunk.
type Hit struct {
	Path    string  `json:"path"`
	Line    int     `json:"line"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

type chunk struct {
	path  string
	line  int // 1-based start
	text  string
	tf    map[string]int
	terms int
}

type fileEntry struct {
	size   int64
	mtime  int64
	chunks []chunk
}

// Indexer is the resident indexer actor for one workspace: an incremental,
// in-memory index refreshed against file fingerprints on every query.
type Indexer struct {
	root  string
	mu    sync.Mutex
	files map[string]*fileEntry
}

// New builds an (empty) indexer over a workspace root; the first query
// pays the initial walk.
func New(root string) *Indexer {
	return &Indexer{root: root, files: map[string]*fileEntry{}}
}

// Search refreshes the index incrementally and returns the top-k chunks
// for the query, best first. k is clamped to [1, MaxK]; 0 means DefaultK.
func (ix *Indexer) Search(query string, k int) ([]Hit, int, error) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if err := ix.refresh(); err != nil {
		return nil, 0, err
	}
	if k <= 0 {
		k = DefaultK
	}
	if k > MaxK {
		k = MaxK
	}
	qTerms := tokenize(query)
	if len(qTerms) == 0 {
		return nil, len(ix.files), nil
	}

	// Corpus stats for the query terms only.
	var n, totalTerms int
	df := map[string]int{}
	for _, fe := range ix.files {
		for _, c := range fe.chunks {
			n++
			totalTerms += c.terms
			for _, t := range qTerms {
				if c.tf[t] > 0 {
					df[t]++
				}
			}
		}
	}
	if n == 0 {
		return nil, len(ix.files), nil
	}
	avgLen := float64(totalTerms) / float64(n)

	const k1, b = 1.2, 0.75
	var hits []Hit
	for _, fe := range ix.files {
		for _, c := range fe.chunks {
			var score float64
			for _, t := range qTerms {
				tf := float64(c.tf[t])
				if tf == 0 {
					continue
				}
				idf := math.Log(1 + (float64(n)-float64(df[t])+0.5)/(float64(df[t])+0.5))
				score += idf * tf * (k1 + 1) / (tf + k1*(1-b+b*float64(c.terms)/avgLen))
			}
			if score > 0 {
				hits = append(hits, Hit{Path: c.path, Line: c.line,
					Score: score, Snippet: snippet(c.text, qTerms)})
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].Path != hits[j].Path {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Line < hits[j].Line
	})
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits, len(ix.files), nil
}

// refresh walks the workspace and re-chunks files whose fingerprint moved;
// deleted files drop out. Symlinks are never followed.
func (ix *Indexer) refresh() error {
	seen := map[string]bool{}
	err := filepath.WalkDir(ix.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: index what we can
		}
		name := d.Name()
		if d.IsDir() {
			if path != ix.root && SkipDir(name) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || SkipFile(name) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxFileBytes {
			return nil
		}
		rel, err := filepath.Rel(ix.root, path)
		if err != nil {
			return nil
		}
		seen[rel] = true
		if fe, ok := ix.files[rel]; ok && fe.size == info.Size() && fe.mtime == info.ModTime().UnixNano() {
			return nil // unchanged
		}
		raw, err := os.ReadFile(path)
		if err != nil || looksBinary(raw) {
			delete(ix.files, rel)
			return nil
		}
		ix.files[rel] = &fileEntry{
			size: info.Size(), mtime: info.ModTime().UnixNano(),
			chunks: chunkFile(rel, string(raw)),
		}
		return nil
	})
	for rel := range ix.files {
		if !seen[rel] {
			delete(ix.files, rel)
		}
	}
	return err
}

func chunkFile(rel, content string) []chunk {
	lines := strings.Split(content, "\n")
	var chunks []chunk
	for start := 0; start < len(lines); start += chunkLines {
		end := min(start+chunkLines, len(lines))
		text := strings.Join(lines[start:end], "\n")
		terms := tokenize(text)
		if len(terms) == 0 {
			continue
		}
		tf := make(map[string]int, len(terms))
		for _, t := range terms {
			tf[t]++
		}
		chunks = append(chunks, chunk{path: rel, line: start + 1,
			text: text, tf: tf, terms: len(terms)})
	}
	return chunks
}

func looksBinary(raw []byte) bool {
	n := min(len(raw), sniffBytes)
	for _, b := range raw[:n] {
		if b == 0 {
			return true
		}
	}
	return false
}

// tokenize lowercases and splits on non-alphanumerics AND camelCase
// humps, so "userName" and "user_name" both index as {user, name}.
func tokenize(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 1 { // single letters are noise
			out = append(out, strings.ToLower(string(cur)))
		}
		cur = cur[:0]
	}
	prevLower := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r):
			if unicode.IsUpper(r) && prevLower {
				flush()
			}
			cur = append(cur, r)
			prevLower = unicode.IsLower(r)
		case unicode.IsDigit(r):
			cur = append(cur, r)
			prevLower = false
		default:
			flush()
			prevLower = false
		}
	}
	flush()
	return out
}

// snippet returns up to ~240 chars starting at the first line containing
// any query term, so the model sees the matching region, not the chunk head.
func snippet(text string, qTerms []string) string {
	lines := strings.Split(text, "\n")
	start := 0
	for i, line := range lines {
		if lineMatches(line, qTerms) {
			start = i
			break
		}
	}
	s := strings.Join(lines[start:], "\n")
	if len(s) > 240 {
		s = s[:240] + "…"
	}
	return s
}

func lineMatches(line string, qTerms []string) bool {
	for _, t := range tokenize(line) {
		for _, q := range qTerms {
			if t == q {
				return true
			}
		}
	}
	return false
}
