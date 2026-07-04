package snapshot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newStore(t *testing.T, ws string) *ShadowRepo {
	t.Helper()
	s, err := NewShadowRepo(filepath.Join(t.TempDir(), "shadow.git"), ws)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Snapshot → mutate → snapshot: two refs, each materializing its OWN state
// into a fresh directory; an unchanged tree dedups to the same ref.
func TestSnapshotAndMaterialize(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "main.go", "v1")
	write(t, ws, "docs/notes.md", "hello")
	s := newStore(t, ws)
	ctx := context.Background()

	ref1, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Unchanged tree → the SAME ref (dedup).
	refAgain, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if refAgain != ref1 {
		t.Fatalf("unchanged tree minted a new ref: %s vs %s", refAgain, ref1)
	}

	write(t, ws, "main.go", "v2")
	write(t, ws, "new.txt", "added")
	ref2, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ref2 == ref1 {
		t.Fatal("changed tree reused the old ref")
	}

	out1 := filepath.Join(t.TempDir(), "m1")
	if err := s.Materialize(ctx, ref1, out1); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(out1, "main.go")); string(got) != "v1" {
		t.Fatalf("ref1 main.go = %q, want v1", got)
	}
	if _, err := os.Stat(filepath.Join(out1, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("ref1 must not contain the later file")
	}
	if got, _ := os.ReadFile(filepath.Join(out1, "docs/notes.md")); string(got) != "hello" {
		t.Fatalf("nested file = %q", got)
	}

	out2 := filepath.Join(t.TempDir(), "m2")
	if err := s.Materialize(ctx, ref2, out2); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(out2, "main.go")); string(got) != "v2" {
		t.Fatalf("ref2 main.go = %q, want v2", got)
	}
	if got, _ := os.ReadFile(filepath.Join(out2, "new.txt")); string(got) != "added" {
		t.Fatalf("ref2 new.txt = %q", got)
	}

	// Refs stay pinned: ref1 still materializes after ref2 exists (a
	// rewind must not strand newer snapshots either — both live).
	out3 := filepath.Join(t.TempDir(), "m3")
	if err := s.Materialize(ctx, ref1, out3); err != nil {
		t.Fatalf("older ref unpinned: %v", err)
	}
}

// Credentials never enter snapshots: the hard-exclude table keeps .env and
// key material out, so a rewind cannot resurrect them.
func TestSnapshotHardExcludes(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "app.go", "code")
	write(t, ws, ".env", "GEMINI_API_KEY=secret")
	write(t, ws, "deploy.pem", "PRIVATE KEY")
	write(t, ws, ".ssh/id_rsa", "PRIVATE")
	s := newStore(t, ws)
	ctx := context.Background()

	ref, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "m")
	if err := s.Materialize(ctx, ref, out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "app.go")); err != nil {
		t.Fatal("code file missing from snapshot")
	}
	for _, banned := range []string{".env", "deploy.pem", ".ssh/id_rsa"} {
		if _, err := os.Stat(filepath.Join(out, banned)); !os.IsNotExist(err) {
			t.Errorf("%s leaked into the snapshot", banned)
		}
	}
}

// The user's .git is invisible in both directions: never snapshotted, never
// touched by shadow operations.
func TestSnapshotUserRepoInvisible(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "code.go", "x")
	write(t, ws, ".git/config", "[core]\n")
	write(t, ws, ".git/HEAD", "ref: refs/heads/main\n")
	s := newStore(t, ws)
	ctx := context.Background()

	before, _ := os.ReadFile(filepath.Join(ws, ".git/config"))
	ref, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(ws, ".git/config"))
	if string(before) != string(after) {
		t.Fatal("shadow snapshot mutated the user's .git")
	}
	out := filepath.Join(t.TempDir(), "m")
	if err := s.Materialize(ctx, ref, out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, ".git")); !os.IsNotExist(err) {
		t.Fatal("the user's .git leaked into the snapshot")
	}
}

// Materialize refuses a non-empty target — forks never share directories.
func TestMaterializeRefusesNonEmpty(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "a.txt", "a")
	s := newStore(t, ws)
	ref, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	write(t, target, "occupied.txt", "here first")
	if err := s.Materialize(context.Background(), ref, target); err == nil {
		t.Fatal("non-empty target must be refused")
	}
}

// backend=none: unavailable, loudly and gracefully.
func TestNoneBackend(t *testing.T) {
	var s Store = None{}
	if _, err := s.Snapshot(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if err := s.Materialize(context.Background(), "x", t.TempDir()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

// Open degrades to None when git is missing from PATH.
func TestOpenDegradesWithoutGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	s, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Snapshot(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want graceful ErrUnavailable, got %v", err)
	}
}

// Latency observation (记档 basis, not an assertion): first vs incremental
// snapshot on a modest tree.
func TestSnapshotLatencyObservation(t *testing.T) {
	ws := t.TempDir()
	for i := 0; i < 200; i++ {
		write(t, ws, filepath.Join("pkg", string(rune('a'+i%26)), itoa(i)+".txt"), "content")
	}
	s := newStore(t, ws)
	ctx := context.Background()
	t0 := time.Now()
	if _, err := s.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}
	first := time.Since(t0)
	write(t, ws, "pkg/a/changed.txt", "delta")
	t1 := time.Now()
	if _, err := s.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}
	incr := time.Since(t1)
	t.Logf("snapshot latency: first=%v incremental=%v (200 files)", first, incr)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
