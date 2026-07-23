package snapshot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestShadowRepoDiffAgainstSnapshot(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "modified.txt", "before\n")
	write(t, ws, "deleted.txt", "delete me\n")
	write(t, ws, "old-name.txt", "rename me\n")
	s := newStore(t, ws)
	ctx := context.Background()
	ref, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	headBefore, err := s.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	indexBefore, err := os.ReadFile(filepath.Join(s.gitDir, "index"))
	if err != nil {
		t.Fatal(err)
	}

	write(t, ws, "modified.txt", "after\n")
	if err := os.Remove(filepath.Join(ws, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(ws, "old-name.txt"), filepath.Join(ws, "new-name.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, ws, "created.txt", "new\n")

	got, err := s.Diff(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"modified.txt", "deleted.txt", "created.txt", "old-name.txt", "new-name.txt", "-before", "+after"} {
		if !strings.Contains(got.Diff, want) {
			t.Errorf("diff missing %q:\n%s", want, got.Diff)
		}
	}
	for _, want := range []string{"modified.txt", "deleted.txt", "created.txt"} {
		if !strings.Contains(got.Numstat, want) {
			t.Errorf("numstat missing %q:\n%s", want, got.Numstat)
		}
	}
	headAfter, _ := s.git(ctx, "rev-parse", "HEAD")
	indexAfter, _ := os.ReadFile(filepath.Join(s.gitDir, "index"))
	if headAfter != headBefore || !bytes.Equal(indexAfter, indexBefore) {
		t.Fatal("read-only diff mutated shadow HEAD or index")
	}
}

func TestShadowRepoDiffQuietsNewGeneratedLargeAndBinaryFiles(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "seed.txt", "baseline\n")
	s := newStore(t, ws)
	ctx := context.Background()
	ref, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}

	write(t, ws, "small.txt", "review me\n")
	write(t, ws, "large.txt", strings.Repeat("x", 256*1024+1))
	if err := os.WriteFile(filepath.Join(ws, "binary.bin"), []byte{'a', 0, 'b'}, 0o644); err != nil {
		t.Fatal(err)
	}
	write(t, ws, "node_modules/pkg/index.js", "generated\n")

	got, err := s.Diff(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Diff, "small.txt") {
		t.Fatalf("small text addition must stay inline:\n%s", got.Diff)
	}
	for _, omitted := range []string{"large.txt", "binary.bin", "node_modules"} {
		if strings.Contains(got.Diff, omitted) || strings.Contains(got.Numstat, omitted) {
			t.Errorf("%s leaked into inline diff/numstat:\ndiff=%s\nnumstat=%s", omitted, got.Diff, got.Numstat)
		}
	}
	if strings.Join(got.Untracked, ",") != "binary.bin,large.txt" {
		t.Fatalf("name-only additions = %v, want binary.bin,large.txt", got.Untracked)
	}
	if got.UntrackedReasons["binary.bin"] != "binary" || got.UntrackedReasons["large.txt"] != "large" {
		t.Fatalf("name-only reasons = %v, want binary/large", got.UntrackedReasons)
	}
	if got.HiddenUntracked != 1 {
		t.Fatalf("hidden generated additions = %d, want 1", got.HiddenUntracked)
	}
}

// A CJK (non-ASCII) filename must appear literally in the diff header, not
// octal-escaped (`"a/\345\233\276.md"`). The Last-turn review card renders this
// diff text verbatim, so an escaped path shows the user garbage. QA-0719 t11
// caught this on a real turn (图表说明.md) after the working-tree path was
// already fixed; the shadow backend needed the same core.quotePath=false pin.
func TestShadowRepoDiffKeepsUnicodePathsUnescaped(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "seed.txt", "seed\n")
	s := newStore(t, ws)
	ctx := context.Background()
	ref, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const cjkName = "图表说明.md"
	write(t, ws, cjkName, "这是柱状图的说明文档\n")
	got, err := s.Diff(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Diff, cjkName) {
		t.Errorf("diff missing literal CJK filename %q:\n%s", cjkName, got.Diff)
	}
	if strings.Contains(got.Diff, `\345`) || strings.Contains(got.Numstat, `\345`) {
		t.Errorf("diff/numstat octal-escaped the CJK path (want quotePath=false):\ndiff=%s\nnumstat=%s", got.Diff, got.Numstat)
	}
}

func TestShadowRepoDiffRejectsInvalidRef(t *testing.T) {
	s := newStore(t, t.TempDir())
	for _, ref := range []string{"", "HEAD", "-p", strings.Repeat("a", 39), strings.Repeat("A", 40), strings.Repeat("a", 65)} {
		if _, err := s.Diff(context.Background(), ref); err == nil || !strings.Contains(err.Error(), "invalid snapshot ref") {
			t.Errorf("Diff(%q) err = %v, want invalid ref", ref, err)
		}
	}
}

func TestShadowRepoDiffKeepsCredentialsExcluded(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "app.go", "before\n")
	s := newStore(t, ws)
	ref, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	write(t, ws, "app.go", "after\n")
	write(t, ws, ".env", "SECRET=must-not-leak\n")
	got, err := s.Diff(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Diff, "app.go") || strings.Contains(got.Diff, ".env") || strings.Contains(got.Diff, "must-not-leak") {
		t.Fatalf("credential exclusion failed:\n%s", got.Diff)
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
	if _, err := s.Diff(context.Background(), strings.Repeat("a", 40)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("diff err = %v", err)
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

// PushRefs moves a snapshot's object closure into ANOTHER shadow repo
// (S7.3 fork): the destination can materialize the ref without ever
// touching the source again.
func TestPushRefsTransfersSnapshots(t *testing.T) {
	wsA, wsB := t.TempDir(), t.TempDir()
	write(t, wsA, "main.go", "v1")
	src := newStore(t, wsA)
	dst := newStore(t, wsB)
	ctx := context.Background()

	ref, err := src.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := src.PushRefs(ctx, dst.GitDir(), []string{ref}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out")
	if err := dst.Materialize(ctx, ref, out); err != nil {
		t.Fatalf("materialize from destination store: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "main.go"))
	if err != nil || string(raw) != "v1" {
		t.Errorf("main.go = %q err=%v", raw, err)
	}
	// Re-pushing the same ref is a no-op, not an error.
	if err := src.PushRefs(ctx, dst.GitDir(), []string{ref, ""}); err != nil {
		t.Fatalf("idempotent push: %v", err)
	}
}

// S7 出口 review: widened credential excludes — common credential stores
// beyond the original list, and .aws/credentials at ANY depth.
func TestHardExcludesWidened(t *testing.T) {
	ws := t.TempDir()
	for _, f := range []string{".git-credentials", ".netrc", ".npmrc", ".pypirc",
		"credentials.json", ".envrc", "sub/.aws/credentials"} {
		write(t, ws, f, "supersecretvalue")
	}
	write(t, ws, "ok.txt", "fine")
	s := newStore(t, ws)
	ctx := context.Background()
	ref, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out")
	if err := s.Materialize(ctx, ref, out); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{".git-credentials", ".netrc", ".npmrc", ".pypirc",
		"credentials.json", ".envrc", "sub/.aws/credentials"} {
		if _, err := os.Stat(filepath.Join(out, f)); !os.IsNotExist(err) {
			t.Errorf("%s leaked into the snapshot", f)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "ok.txt")); err != nil {
		t.Errorf("ok.txt missing: %v", err)
	}
}

// Materialize failure (bad ref) must leave the target ABSENT — partial
// trees would be mistaken for complete worktrees on resume.
func TestMaterializeFailureLeavesNoTarget(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "a.txt", "x")
	s := newStore(t, ws)
	out := filepath.Join(t.TempDir(), "out")
	if err := s.Materialize(context.Background(), "deadbeef", out); err == nil {
		t.Fatal("bad ref must fail")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("failed materialize left target behind")
	}
}

func TestShadowRepoSerializesConcurrentInitAndSnapshots(t *testing.T) {
	ws := t.TempDir()
	for i := 0; i < 100; i++ {
		write(t, ws, filepath.Join("src", fmt.Sprintf("f-%03d.txt", i)), strings.Repeat("x", 1024))
	}
	gitDir := filepath.Join(t.TempDir(), "shared-shadow.git")
	const workers = 12
	stores := make([]*ShadowRepo, workers)
	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			stores[i], errs[i] = NewShadowRepo(gitDir, ws)
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
	}

	refs := make([]string, workers)
	start = make(chan struct{})
	wg = sync.WaitGroup{}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			refs[i], errs[i] = stores[i].Snapshot(context.Background())
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("snapshot %d: %v", i, err)
		}
		if refs[i] != refs[0] {
			t.Fatalf("snapshot %d ref=%s, want deduplicated %s", i, refs[i], refs[0])
		}
	}
}
