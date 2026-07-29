package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCmd(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "commands")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExpandArguments(t *testing.T) {
	root := t.TempDir()
	writeCmd(t, root, "review", "Review the diff focusing on: $ARGUMENTS. Be concise.")
	got, ok := Expand(root, "/review concurrency and error handling")
	if !ok || got != "Review the diff focusing on: concurrency and error handling. Be concise." {
		t.Fatalf("expand = %q ok=%v", got, ok)
	}
}

func TestExpandAppendsWhenNoPlaceholder(t *testing.T) {
	root := t.TempDir()
	writeCmd(t, root, "deploy-check", "Run the deploy checklist.")
	got, ok := Expand(root, "/deploy-check staging")
	if !ok || got != "Run the deploy checklist.\n\nstaging" {
		t.Fatalf("expand = %q ok=%v", got, ok)
	}
	// No args → just the body.
	got, ok = Expand(root, "/deploy-check")
	if !ok || got != "Run the deploy checklist." {
		t.Fatalf("no-arg expand = %q ok=%v", got, ok)
	}
}

func TestExpandUnknownAndNonSlashPassThrough(t *testing.T) {
	root := t.TempDir()
	writeCmd(t, root, "known", "body")
	for _, in := range []string{"/unknown do a thing", "just a normal message", "email me at a/b", "/"} {
		got, ok := Expand(root, in)
		if ok || got != in {
			t.Fatalf("input %q should pass through, got %q ok=%v", in, got, ok)
		}
	}
}

func TestExpandRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	// A name with a path separator or dots must not resolve a file.
	for _, in := range []string{"/../secret", "/a/b", "/foo.bar"} {
		if got, ok := Expand(root, in); ok {
			t.Fatalf("traversal-ish %q expanded to %q — must be rejected", in, got)
		}
	}
}

func TestExpandStripsFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeCmd(t, root, "fm", "---\ndescription: a test command\n---\nThe actual prompt body.")
	got, ok := Expand(root, "/fm")
	if !ok || got != "The actual prompt body." {
		t.Fatalf("frontmatter not stripped: %q ok=%v", got, ok)
	}
}

func TestExpandLeadingWhitespace(t *testing.T) {
	root := t.TempDir()
	writeCmd(t, root, "go", "GO")
	got, ok := Expand(root, "   /go")
	if !ok || got != "GO" {
		t.Fatalf("leading-ws slash not handled: %q ok=%v", got, ok)
	}
}

func TestExpandEmptyRoot(t *testing.T) {
	if got, ok := Expand("", "/anything"); ok || got != "/anything" {
		t.Fatalf("empty root should pass through: %q ok=%v", got, ok)
	}
}

// /name with no command file falls back to the skill of that name; a command
// file shadows a same-named skill; unknown names pass through untouched.
func TestExpandSkillFallback(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, ".claude", "skills", "release-ritual")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: release-ritual\ndescription: d\n---\nCut the release.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := Expand(root, "/release-ritual for v2")
	if !ok || got != "Cut the release.\n\nfor v2" {
		t.Fatalf("workspace skill fallback = %q, %v", got, ok)
	}

	// Shipped layer: create-agent is embedded in the binary.
	got, ok = Expand(root, "/create-agent a haiku bot")
	if !ok || !strings.Contains(got, "save_agent") || !strings.Contains(got, "a haiku bot") {
		t.Fatalf("shipped skill fallback = %.60q…, %v", got, ok)
	}

	// A command file shadows the skill.
	cmdDir := filepath.Join(root, ".claude", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "release-ritual.md"),
		[]byte("Command wins: $ARGUMENTS"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok = Expand(root, "/release-ritual for v2")
	if !ok || got != "Command wins: for v2" {
		t.Fatalf("command should shadow skill = %q, %v", got, ok)
	}

	// Unknown name: untouched pass-through.
	if got, ok := Expand(root, "/no-such-thing hi"); ok || got != "/no-such-thing hi" {
		t.Fatalf("unknown name must pass through = %q, %v", got, ok)
	}
}

func TestDiscoverCommands(t *testing.T) {
	root := t.TempDir()
	if got := Discover(root); got != nil {
		t.Fatalf("no dir should be nil, got %v", got)
	}
	dir := filepath.Join(root, ".claude", "commands")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ship.md"),
		[]byte("---\ndescription: Ship it\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Discover(root)
	if len(got) != 1 || got[0].Name != "ship" || got[0].Description != "Ship it" {
		t.Fatalf("Discover = %+v", got)
	}
}
