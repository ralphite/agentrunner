package snapshot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The harness-level floor DESIGN asks for: a workspace with NO .gitignore at all
// must still keep machine-regenerable trees out of snapshots. Before G62 only the
// credential half of the exclude table was implemented, so such a workspace got
// its whole tree hashed at every barrier.
func TestSnapshotExcludesDerivedTreesWithoutGitignore(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "main.go", "package main\n")
	write(t, ws, "node_modules/pkg/index.js", "generated\n")
	write(t, ws, "__pycache__/mod.cpython-311.pyc", "bytecode\n")
	write(t, ws, "sub/dist/bundle.js", "bundled\n")
	write(t, ws, ".venv/lib/site-packages/x/__init__.py", "env\n")
	write(t, ws, "pkg.egg-info/PKG-INFO", "meta\n")
	write(t, ws, "coverage/lcov.info", "cov\n")

	s := newStore(t, ws)
	ctx := context.Background()
	ref, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.git(ctx, "ls-tree", "-r", "--name-only", ref)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "main.go") {
		t.Fatalf("real source must be snapshotted:\n%s", out)
	}
	for _, derived := range []string{
		"node_modules", "__pycache__", "dist", ".venv", "site-packages",
		"egg-info", "coverage",
	} {
		if strings.Contains(out, derived) {
			t.Errorf("%s must be excluded from the snapshot:\n%s", derived, out)
		}
	}
}

func TestSnapshotIncludesCredentialShapedProjectFiles(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "main.go", "package main\n")
	write(t, ws, ".env", "SECRET=shh\n")
	write(t, ws, "deploy.pem", "key\n")

	s := newStore(t, ws)
	ctx := context.Background()
	ref, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.git(ctx, "ls-tree", "-r", "--name-only", ref)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{".env", "deploy.pem"} {
		if !strings.Contains(out, secret) {
			t.Errorf("%s missing from capability-first snapshot:\n%s", secret, out)
		}
	}
}

// .gitignore semantics govern only UNTRACKED files, so a shadow repo created
// before these excludes existed would keep staging the node_modules it had
// already picked up — the new floor would silently never apply there. This is
// the convergence step, and it is the difference between "new repos are fixed"
// and "the fix actually reaches users".
func TestPruneDerivedConvergesAPreExistingIndex(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "main.go", "package main\n")
	write(t, ws, "node_modules/pkg/index.js", "generated\n")
	gitDir := filepath.Join(t.TempDir(), "shadow.git")
	ctx := context.Background()

	// Simulate the pre-G62 world: a repo without derived-tree excludes.
	s, err := NewShadowRepo(gitDir, ws)
	if err != nil {
		t.Fatal(err)
	}
	legacy := "# legacy\n"
	if err := os.WriteFile(filepath.Join(gitDir, "info", "exclude"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.git(ctx, "add", "-A", "."); err != nil {
		t.Fatal(err)
	}
	tracked, err := s.git(ctx, "ls-files")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tracked, "node_modules") {
		t.Fatalf("precondition: legacy excludes should have tracked node_modules, got:\n%s", tracked)
	}

	// Reopening the repo is what a new session does; it must converge.
	if _, err := NewShadowRepo(gitDir, ws); err != nil {
		t.Fatal(err)
	}
	tracked, err = s.git(ctx, "ls-files")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(tracked, "node_modules") {
		t.Errorf("reopen must prune already-tracked derived paths, still tracked:\n%s", tracked)
	}
	if !strings.Contains(tracked, "main.go") {
		t.Errorf("pruning must not touch real source:\n%s", tracked)
	}

	// And a snapshot taken afterwards is clean.
	ref, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.git(ctx, "ls-tree", "-r", "--name-only", ref)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "node_modules") {
		t.Errorf("post-prune snapshot still contains node_modules:\n%s", out)
	}
}

// Pruning must be driven by OUR list, not by `git ls-files -i
// --exclude-standard`: the latter would also untrack whatever the USER's
// .gitignore happens to list, which is not this function's call to make.
func TestPruneDerivedLeavesUserIgnoredTrackedFilesAlone(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "main.go", "package main\n")
	write(t, ws, "generated.txt", "committed on purpose\n")
	write(t, ws, ".gitignore", "generated.txt\n")

	gitDir := filepath.Join(t.TempDir(), "shadow.git")
	ctx := context.Background()
	s, err := NewShadowRepo(gitDir, ws)
	if err != nil {
		t.Fatal(err)
	}
	// Force-add the user-ignored file, as a pre-existing index might have.
	if _, err := s.git(ctx, "add", "-f", "--", "generated.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewShadowRepo(gitDir, ws); err != nil {
		t.Fatal(err)
	}
	tracked, err := s.git(ctx, "ls-files")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tracked, "generated.txt") {
		t.Errorf("a user-ignored but tracked file is not ours to untrack:\n%s", tracked)
	}
}

// The file written to info/exclude and the Go predicate must agree, or the
// review projection and the snapshot would disagree about "derived".
func TestDerivedExcludePatternsMatchPredicate(t *testing.T) {
	for _, pattern := range derivedExcludePatterns() {
		name := strings.TrimSuffix(strings.TrimPrefix(pattern, "*"), "/")
		probe := "a/" + name + "/b.txt"
		if strings.HasPrefix(pattern, "*") {
			probe = "a/pkg" + name + "/b.txt"
		}
		if !derivedPath(probe) {
			t.Errorf("pattern %q is in info/exclude but derivedPath(%q) is false", pattern, probe)
		}
	}
	// The review filter must be a strict SUPERSET of the snapshot excludes.
	for name := range derivedDirs {
		if !reviewHiddenUntrackedPath("a/" + name + "/b.txt") {
			t.Errorf("%s is snapshot-excluded but not review-hidden", name)
		}
	}
	// vendor is the documented asymmetry: hidden from cards, kept in snapshots.
	if !reviewHiddenUntrackedPath("vendor/x/y.go") {
		t.Error("vendor must be hidden from review cards")
	}
	if derivedPath("vendor/x/y.go") {
		t.Error("vendor must NOT be excluded from snapshots — projects commit it as source")
	}
}
