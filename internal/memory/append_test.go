package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Append creates CLAUDE.md when absent, appends under a stable section, keeps
// prior content, and is idempotent for a repeated note (INC-14 replay safety).
func TestMemoryAppendCreatesAndPreserves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")

	// Create.
	if err := Append(dir, "use pnpm"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "## Remembered") || !strings.Contains(got, "- use pnpm\n") {
		t.Fatalf("create: %q", got)
	}

	// Append a second note, keeping the first.
	if err := Append(dir, "run tests with -race"); err != nil {
		t.Fatal(err)
	}
	got = readFile(t, path)
	if !strings.Contains(got, "- use pnpm\n") || !strings.Contains(got, "- run tests with -race\n") {
		t.Fatalf("second append lost content: %q", got)
	}
	if strings.Count(got, "## Remembered") != 1 {
		t.Errorf("section opened more than once: %q", got)
	}

	// Idempotent: repeating an existing note must not double-write.
	before := readFile(t, path)
	if err := Append(dir, "use pnpm"); err != nil {
		t.Fatal(err)
	}
	if after := readFile(t, path); after != before {
		t.Errorf("duplicate note double-written:\nbefore=%q\nafter=%q", before, after)
	}
}

// Append never clobbers hand-written project memory.
func TestMemoryAppendPreservesHandwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("# Project\n\nStack: Go 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Append(dir, "deploy via make ship"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "Stack: Go 1.23") {
		t.Errorf("clobbered existing content: %q", got)
	}
	if !strings.Contains(got, "- deploy via make ship\n") {
		t.Errorf("note not appended: %q", got)
	}
}

func TestMemoryAppendRejectsEmpty(t *testing.T) {
	if err := Append(t.TempDir(), "   "); err == nil {
		t.Error("empty note should error")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
