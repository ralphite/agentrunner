package index

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, root, name, content string) {
	t.Helper()
	full := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSearchFindsRelevantChunk(t *testing.T) {
	root := t.TempDir()
	write(t, root, "auth/login.go", "package auth\n\nfunc ValidatePassword(hash, plain string) bool {\n\treturn subtle(hash, plain)\n}\n")
	write(t, root, "readme.md", "This project ships a tiny web server.\n")
	ix := New(root)

	hits, files, err := ix.Search("password validation", 0)
	if err != nil {
		t.Fatal(err)
	}
	if files != 2 {
		t.Errorf("indexed_files = %d, want 2", files)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %+v, want exactly the auth chunk", hits)
	}
	h := hits[0]
	if h.Path != "auth/login.go" || h.Line != 1 || h.Score <= 0 {
		t.Errorf("hit = %+v", h)
	}
	if want := "ValidatePassword"; !contains(h.Snippet, want) {
		t.Errorf("snippet %q missing %s", h.Snippet, want)
	}
}

// Identifier-aware tokenization: camelCase and snake_case both match a
// natural-language query.
func TestTokenizeIdentifiers(t *testing.T) {
	got := tokenize("userName user_name HTTPServer2 x")
	want := []string{"user", "name", "user", "name", "httpserver2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokenize = %v, want %v", got, want)
	}
}

func TestSearchRanksDenserFileHigher(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "retry retry retry backoff retry\n")
	write(t, root, "b.txt", "one retry mention here\nplus other words entirely\n")
	ix := New(root)
	hits, _, err := ix.Search("retry", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Path != "a.txt" {
		t.Errorf("order = %+v, want a.txt first", hits)
	}
}

// The fourth state class is DERIVED: edits and deletions are visible on
// the next query, no invalidation protocol needed.
func TestIncrementalRefresh(t *testing.T) {
	root := t.TempDir()
	write(t, root, "note.txt", "the walrus lives here\n")
	ix := New(root)
	if hits, _, _ := ix.Search("walrus", 0); len(hits) != 1 {
		t.Fatalf("initial hits = %+v", hits)
	}

	// Rewrite with different content (bump mtime to defeat the fingerprint
	// granularity on fast filesystems).
	write(t, root, "note.txt", "the octopus lives here\n")
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(root, "note.txt"), future, future); err != nil {
		t.Fatal(err)
	}
	if hits, _, _ := ix.Search("walrus", 0); len(hits) != 0 {
		t.Errorf("stale hits survived rewrite: %+v", hits)
	}
	if hits, _, _ := ix.Search("octopus", 0); len(hits) != 1 {
		t.Errorf("new content not indexed")
	}

	if err := os.Remove(filepath.Join(root, "note.txt")); err != nil {
		t.Fatal(err)
	}
	if hits, files, _ := ix.Search("octopus", 0); len(hits) != 0 || files != 0 {
		t.Errorf("deleted file still indexed: hits=%+v files=%d", hits, files)
	}
}

// Credential-shaped paths and derived trees never enter the index — their
// content must not surface in journaled snippets.
func TestExcludesCredentialsAndVendoredTrees(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".env", "GEMINI_API_KEY=supersecretvalue\n")
	write(t, root, "server.pem", "supersecretvalue certificate\n")
	write(t, root, "node_modules/pkg/index.js", "supersecretvalue in vendor\n")
	write(t, root, ".git/config", "supersecretvalue in git\n")
	write(t, root, "ok.txt", "no secrets here\n")
	ix := New(root)
	hits, files, err := ix.Search("supersecretvalue", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("excluded content surfaced: %+v", hits)
	}
	if files != 1 {
		t.Errorf("indexed_files = %d, want just ok.txt", files)
	}
}

func TestBinaryAndOversizeSkipped(t *testing.T) {
	root := t.TempDir()
	write(t, root, "bin.dat", "prefix\x00binary")
	big := make([]byte, maxFileBytes+1)
	for i := range big {
		big[i] = 'a'
	}
	if err := os.WriteFile(filepath.Join(root, "big.txt"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	ix := New(root)
	if _, files, _ := ix.Search("prefix", 0); files != 0 {
		t.Errorf("indexed_files = %d, want 0", files)
	}
}

func TestEmptyQueryAndNoMatches(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "content\n")
	ix := New(root)
	if hits, _, err := ix.Search("???", 0); err != nil || len(hits) != 0 {
		t.Errorf("punctuation-only query: hits=%v err=%v", hits, err)
	}
	if hits, _, err := ix.Search("nomatchword", 0); err != nil || len(hits) != 0 {
		t.Errorf("no-match query: hits=%v err=%v", hits, err)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

// S7 出口 review: index skip list stays in lockstep with the snapshot
// hard excludes.
func TestExcludesWidenedLockstep(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{".git-credentials", ".netrc", ".npmrc", ".pypirc",
		"credentials.json", ".envrc"} {
		write(t, root, f, "supersecretvalue token\n")
	}
	write(t, root, "ok.txt", "clean\n")
	ix := New(root)
	hits, files, err := ix.Search("supersecretvalue", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 || files != 1 {
		t.Errorf("hits=%v files=%d — credential store content indexed", hits, files)
	}
}
