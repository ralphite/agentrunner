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

// INC-105 multi-root: the boundary is the union of primary and extras.
func TestMultiRootResolveSpansEveryRoot(t *testing.T) {
	primary := t.TempDir()
	extra := t.TempDir()
	outside := t.TempDir()
	ws, err := NewMultiRoot(primary, []string{extra})
	if err != nil {
		t.Fatal(err)
	}

	// Relative paths stay anchored on the PRIMARY.
	got, err := ws.Resolve("sub/file.txt")
	if err != nil {
		t.Fatalf("relative resolve: %v", err)
	}
	if want := filepath.Join(ws.Root(), "sub/file.txt"); got != want {
		t.Fatalf("relative base must be the primary: %s != %s", got, want)
	}

	// Absolute paths inside the extra root are inside the boundary.
	if _, err := ws.Resolve(filepath.Join(extra, "notes.md")); err != nil {
		t.Fatalf("extra-root path must resolve: %v", err)
	}
	// Outside every root stays refused.
	if _, err := ws.Resolve(filepath.Join(outside, "x")); err == nil {
		t.Fatalf("path outside every root must be refused")
	}
	// A prefix-sibling of an extra root (extra + suffix) is NOT inside it.
	sibling := extra + "-sibling"
	if err := os.Mkdir(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Resolve(filepath.Join(sibling, "x")); err == nil {
		t.Fatalf("prefix sibling of an extra root must be refused")
	}
}

func TestMultiRootDedupesAndOrdersRoots(t *testing.T) {
	primary := t.TempDir()
	extra := t.TempDir()
	ws, err := NewMultiRoot(primary, []string{extra, primary, extra})
	if err != nil {
		t.Fatal(err)
	}
	roots := ws.Roots()
	if len(roots) != 2 || roots[0] != ws.Root() || roots[1] == roots[0] {
		t.Fatalf("roots must be [primary, extra] deduped: %v", roots)
	}
	// Single-root workspaces answer Roots() = [Root()] so range sites need no
	// special case.
	single, err := New(primary)
	if err != nil {
		t.Fatal(err)
	}
	if got := single.Roots(); len(got) != 1 || got[0] != single.Root() {
		t.Fatalf("single-root Roots() = %v", got)
	}
}

func TestMultiRootRejectsMissingExtra(t *testing.T) {
	primary := t.TempDir()
	if _, err := NewMultiRoot(primary, []string{filepath.Join(primary, "nope")}); err == nil {
		t.Fatalf("a missing extra root must fail loudly, not narrow the boundary silently")
	}
}
