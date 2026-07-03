package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newWS(t *testing.T) (*Workspace, string) {
	t.Helper()
	root := t.TempDir()
	ws, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	return ws, ws.Root()
}

func TestResolveInside(t *testing.T) {
	ws, root := newWS(t)
	got, err := ws.Resolve("src/main.go") // does not exist yet — legal for writes
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "src", "main.go"); got != want {
		t.Errorf("resolve = %q, want %q", got, want)
	}
}

func TestResolveAbsoluteInside(t *testing.T) {
	ws, root := newWS(t)
	got, err := ws.Resolve(filepath.Join(root, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "a.txt"); got != want {
		t.Errorf("resolve = %q, want %q", got, want)
	}
}

func TestDotDotEscapeRejected(t *testing.T) {
	ws, _ := newWS(t)
	_, err := ws.Resolve("src/../../etc/passwd")
	if err == nil || !strings.Contains(err.Error(), "path escapes workspace") {
		t.Fatalf("err = %v, want escape rejection", err)
	}
}

func TestAbsoluteOutsideRejected(t *testing.T) {
	ws, _ := newWS(t)
	_, err := ws.Resolve("/etc/passwd")
	if err == nil || !strings.Contains(err.Error(), "path escapes workspace") {
		t.Fatalf("err = %v, want escape rejection", err)
	}
}

func TestSymlinkEscapeRejected(t *testing.T) {
	ws, root := newWS(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "sneaky")); err != nil {
		t.Fatal(err)
	}

	// Existing target behind the symlink.
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Resolve("sneaky/secret.txt"); err == nil || !strings.Contains(err.Error(), "path escapes workspace") {
		t.Fatalf("existing target: err = %v, want escape rejection", err)
	}

	// New (not yet existing) file behind the symlinked dir must also reject.
	if _, err := ws.Resolve("sneaky/new-file.txt"); err == nil || !strings.Contains(err.Error(), "path escapes workspace") {
		t.Fatalf("new target: err = %v, want escape rejection", err)
	}
}

func TestRootItselfResolves(t *testing.T) {
	ws, root := newWS(t)
	got, err := ws.Resolve(".")
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("resolve(.) = %q, want %q", got, root)
	}
}

// A workspace root that is itself a symlink must resolve consistently:
// Root() and Resolve() both live in the fully-resolved space.
func TestRootSymlinkResolves(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	ws, err := New(link)
	if err != nil {
		t.Fatal(err)
	}
	resolvedReal, _ := filepath.EvalSymlinks(real)
	if ws.Root() != resolvedReal {
		t.Errorf("Root() = %q, want %q", ws.Root(), resolvedReal)
	}
	got, err := ws.Resolve("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(resolvedReal, "a.txt") {
		t.Errorf("Resolve = %q", got)
	}
}

// /x/ws-evil must not pass a boundary check for root /x/ws (the classic
// HasPrefix-without-separator bug).
func TestSiblingPrefixRejected(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "ws")
	evil := filepath.Join(parent, "ws-evil")
	for _, d := range []string{root, evil} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ws, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Resolve(filepath.Join(evil, "f.txt")); err == nil {
		t.Fatal("sibling-prefix path must be rejected")
	}
}
