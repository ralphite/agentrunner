package snapshot

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// diffViaReadTree reproduces the PRE-G61 seed (`read-tree <ref>` into a fresh
// private index) so the fast seed can be checked against it for equivalence.
// Kept deliberately close to the shape Diff used to have.
func (s *ShadowRepo) diffViaReadTree(ctx context.Context, ref string) (DiffResult, error) {
	f, err := os.CreateTemp(s.gitDir, "legacy-index-*")
	if err != nil {
		return DiffResult{}, err
	}
	index := f.Name()
	if err := f.Close(); err != nil {
		return DiffResult{}, err
	}
	if err := os.Remove(index); err != nil {
		return DiffResult{}, err
	}
	defer func() {
		_ = os.Remove(index)
		_ = os.Remove(index + ".lock")
	}()
	env := []string{"GIT_INDEX_FILE=" + index}
	if _, err := s.gitWithEnv(ctx, env, "read-tree", ref); err != nil {
		return DiffResult{}, err
	}
	if _, err := s.gitWithEnv(ctx, env, "add", "-A", "."); err != nil {
		return DiffResult{}, err
	}
	untracked, reasons, hidden, err := s.quietNewReviewFiles(ctx, env, ref)
	if err != nil {
		return DiffResult{}, err
	}
	diff, err := s.gitWithEnv(ctx, env, "diff", "--cached", "--no-ext-diff", "--no-color", "--find-renames", ref, "--")
	if err != nil {
		return DiffResult{}, err
	}
	numstat, err := s.gitWithEnv(ctx, env, "diff", "--cached", "--numstat", ref, "--")
	if err != nil {
		return DiffResult{}, err
	}
	return DiffResult{Diff: diff, Numstat: numstat, Untracked: untracked,
		UntrackedReasons: reasons, HiddenUntracked: hidden}, nil
}

// G61: seeding the review index from the repo's persistent index (which carries
// a stat cache) instead of `read-tree <ref>` must produce the IDENTICAL result.
// This is the whole safety argument for the 34x speedup — the seed's content is
// irrelevant because `add -A` ends up representing the working tree either way.
//
// Every mutation shape is exercised in one tree, because the risky ones are
// precisely those the old seed got "for free" from ref: deletions and renames.
func TestReviewIndexSeedEquivalence(t *testing.T) {
	ctx := context.Background()
	ws := t.TempDir()
	write(t, ws, "keep.go", "package keep\n")
	write(t, ws, "edit.go", "package edit\nv1\n")
	write(t, ws, "delete.go", "package gone\n")
	write(t, ws, "rename_from.go", strings.Repeat("package r\n// stable body\n", 20))
	write(t, ws, "sub/nested.go", "package nested\nv1\n")

	s := newStore(t, ws)
	ref, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Mutate every way that matters.
	write(t, ws, "edit.go", "package edit\nv2 changed\n")
	write(t, ws, "sub/nested.go", "package nested\nv2 changed\n")
	write(t, ws, "added.go", "package added\n")
	write(t, ws, "sub/deeper/also_added.go", "package also\n")
	if err := os.Remove(filepath.Join(ws, "delete.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(ws, "rename_from.go"), filepath.Join(ws, "rename_to.go")); err != nil {
		t.Fatal(err)
	}

	fast, err := s.Diff(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := s.diffViaReadTree(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}

	if fast.Diff != legacy.Diff {
		t.Errorf("diff differs between seeds\n--- fast ---\n%s\n--- legacy ---\n%s", fast.Diff, legacy.Diff)
	}
	if fast.Numstat != legacy.Numstat {
		t.Errorf("numstat differs\n fast: %q\n legacy: %q", fast.Numstat, legacy.Numstat)
	}
	if fast.HiddenUntracked != legacy.HiddenUntracked {
		t.Errorf("hidden_untracked: fast %d, legacy %d", fast.HiddenUntracked, legacy.HiddenUntracked)
	}
	if strings.Join(fast.Untracked, "\x00") != strings.Join(legacy.Untracked, "\x00") {
		t.Errorf("untracked: fast %v, legacy %v", fast.Untracked, legacy.Untracked)
	}

	// And the diff must actually be substantive — an equivalence test that
	// compared two empty strings would prove nothing.
	for _, want := range []string{"edit.go", "delete.go", "added.go", "nested.go"} {
		if !strings.Contains(fast.Diff, want) {
			t.Errorf("diff should mention %s:\n%s", want, fast.Diff)
		}
	}
}

// A deletion is the case the old seed got for free from `read-tree <ref>`, so
// it gets its own direct assertion rather than only the equivalence check.
func TestReviewIndexSeedCatchesDeletion(t *testing.T) {
	ctx := context.Background()
	ws := t.TempDir()
	write(t, ws, "a.go", "package a\n")
	write(t, ws, "doomed.go", "package doomed\nbody\n")

	s := newStore(t, ws)
	ref, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(ws, "doomed.go")); err != nil {
		t.Fatal(err)
	}

	got, err := s.Diff(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Diff, "doomed.go") {
		t.Errorf("deletion missing from diff:\n%s", got.Diff)
	}
	if !strings.Contains(got.Numstat, "doomed.go") {
		t.Errorf("deletion missing from numstat: %q", got.Numstat)
	}
}

// Before any snapshot has been taken there is no persistent index to copy, so
// the seed must fall back to read-tree rather than erroring. Diffing against an
// empty-tree ref is the honest shape of "nothing durable yet".
func TestReviewIndexSeedFallsBackWithoutPersistentIndex(t *testing.T) {
	ctx := context.Background()
	ws := t.TempDir()
	write(t, ws, "only.go", "package only\n")
	s := newStore(t, ws)

	// The empty tree object exists in every git repo; use it as a ref that is
	// valid while no index file has been written yet.
	empty, err := s.git(ctx, "hash-object", "-t", "tree", "--stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.gitDir, "index")); err == nil {
		t.Skip("repo already has an index; fallback path not reachable here")
	}

	commit, err := s.git(ctx, "commit-tree", empty, "-m", "empty")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Diff(ctx, commit)
	if err != nil {
		t.Fatalf("fallback seed must not error: %v", err)
	}
	// only.go is new relative to the empty tree, so it shows up as an addition.
	if !strings.Contains(got.Diff, "only.go") && len(got.Untracked) == 0 {
		t.Errorf("expected only.go as an addition; diff=%q untracked=%v", got.Diff, got.Untracked)
	}
}

// Correctness alone cannot protect this optimization: seeding from read-tree,
// from the persistent index, or from nothing at all ALL produce the right diff,
// because `add -A` is what makes the index represent the working tree. Only the
// persistent-index copy carries a stat cache, so only it is fast — and a revert
// to read-tree would leave every other test in this file green while silently
// restoring the 34x cost (28.1s vs 847ms on a 300k-file tree).
//
// So assert the mechanism structurally: the seeded index must be a byte-for-byte
// copy of the repo's persistent index. A read-tree-built index cannot match it —
// that is exactly what "carries no stat cache" means on disk.
func TestReviewIndexSeedCopiesPersistentIndexVerbatim(t *testing.T) {
	ctx := context.Background()
	ws := t.TempDir()
	write(t, ws, "a.go", "package a\n")
	write(t, ws, "sub/b.go", "package b\n")

	s := newStore(t, ws)
	ref, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	persistent, err := os.ReadFile(filepath.Join(s.gitDir, "index"))
	if err != nil {
		t.Fatalf("snapshot must leave a persistent index: %v", err)
	}

	seeded := filepath.Join(t.TempDir(), "review-index")
	if err := s.seedReviewIndex(ctx, seeded, ref, []string{"GIT_INDEX_FILE=" + seeded}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(seeded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, persistent) {
		t.Errorf("seeded index is not a verbatim copy of the persistent index "+
			"(%d vs %d bytes) — the stat cache is what makes Diff fast; "+
			"did the seed revert to read-tree?", len(got), len(persistent))
	}

	// Sanity: a read-tree seed really does differ, so the assertion above is
	// discriminating rather than trivially true.
	legacy := filepath.Join(t.TempDir(), "legacy-index")
	if _, err := s.gitWithEnv(ctx, []string{"GIT_INDEX_FILE=" + legacy}, "read-tree", ref); err != nil {
		t.Fatal(err)
	}
	legacyBytes, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(legacyBytes, persistent) {
		t.Skip("this git builds an identical index from read-tree; the byte check cannot discriminate here")
	}
}

// The copy helper refuses to clobber an existing file: the private index path is
// created fresh per Diff, and silently overwriting one would be a real bug.
func TestCopyIndexFileRefusesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyIndexFile(src, dst); err == nil {
		t.Error("copying onto an existing file must fail")
	}
	if b, _ := os.ReadFile(dst); string(b) != "existing" {
		t.Errorf("target was modified: %q", b)
	}
	if err := copyIndexFile(filepath.Join(dir, "missing"), filepath.Join(dir, "out")); err == nil {
		t.Error("a missing source must fail so the caller falls back")
	}
}
