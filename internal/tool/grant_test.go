package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutsidePathOpenByDefaultAndSandboxRequiresGrant(t *testing.T) {
	e, _ := newExec(t)
	outside := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(outside, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := `{"path":` + quote(outside) + `}`

	if out, isErr := run(t, e, "read_file", args); isErr {
		t.Fatalf("terminal-parity outside read failed: %v", out)
	}

	e.ContainFilesystem()
	if out, isErr := run(t, e, "read_file", args); !isErr {
		t.Fatalf("opt-in filesystem sandbox must require a grant, got %v", out)
	}
	e.GrantPath(outside)
	out, isErr := run(t, e, "read_file", args)
	if isErr {
		t.Fatalf("granted outside read failed: %v", out)
	}
	if s, _ := out["content"].(string); !strings.Contains(s, "hello") {
		t.Errorf("content = %v, want the file's text", out["content"])
	}
}

func TestGrantCoversWriteAndEdit(t *testing.T) {
	e, _ := newExec(t)
	e.ContainFilesystem()
	outside := filepath.Join(t.TempDir(), "cfg.txt")
	e.GrantPath(outside)

	if out, isErr := run(t, e, "write_file", `{"path":`+quote(outside)+`,"content":"one\n"}`); isErr {
		t.Fatalf("granted write failed: %v", out)
	}
	if out, isErr := run(t, e, "edit_file", `{"path":`+quote(outside)+`,"old":"one","new":"two"}`); isErr {
		t.Fatalf("granted edit failed: %v", out)
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "two" {
		t.Errorf("file = %q, want the edited text", got)
	}
}

// A grant is for the path that was approved — not its neighbours, and not its
// parent directory.
func TestGrantDoesNotWidenToSiblings(t *testing.T) {
	e, _ := newExec(t)
	e.ContainFilesystem()
	dir := t.TempDir()
	granted := filepath.Join(dir, "yes.txt")
	sibling := filepath.Join(dir, "no.txt")
	for _, p := range []string{granted, sibling} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e.GrantPath(granted)

	if out, isErr := run(t, e, "read_file", `{"path":`+quote(granted)+`}`); isErr {
		t.Fatalf("granted read failed: %v", out)
	}
	if out, isErr := run(t, e, "read_file", `{"path":`+quote(sibling)+`}`); !isErr {
		t.Fatalf("sibling of a granted path must stay refused, got %v", out)
	}
}

// Grants and lookups must agree on spelling: the same file reached by a
// relative traversal is the same grant.
func TestGrantMatchesAcrossSpellings(t *testing.T) {
	e, root := newExec(t)
	e.ContainFilesystem()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "shared.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.GrantPath(outside)

	rel, err := filepath.Rel(root, outside)
	if err != nil {
		t.Fatal(err)
	}
	if out, isErr := run(t, e, "read_file", `{"path":`+quote(rel)+`}`); isErr {
		t.Fatalf("granted path via relative traversal failed: %v", out)
	}
}

// The sandbox must hear about a grant too, or an approved edit_file would work
// while the very next bash line on the same file failed at the OS boundary.
func TestGrantedPathsReachTheSandbox(t *testing.T) {
	e, _ := newExec(t)
	outside := filepath.Join(t.TempDir(), "tool.sh")
	e.GrantPath(outside)
	paths := e.GrantedPaths()
	if len(paths) != 1 || !strings.HasSuffix(paths[0], "tool.sh") {
		t.Fatalf("GrantedPaths = %v, want the one granted file", paths)
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
