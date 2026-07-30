package wsprobe

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/index"
)

// tree writes n files named f0..f(n-1) under dir.
func tree(t *testing.T, dir string, n int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		p := filepath.Join(dir, "f"+itoa(i)+".go")
		if err := os.WriteFile(p, []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// The whole point of the probe is that it does NOT pay for the whole tree:
// the count saturates one past the threshold no matter how much is there.
func TestProbeSaturatesAtThreshold(t *testing.T) {
	root := t.TempDir()
	tree(t, root, 50)

	if got := Probe(root, 10); got != 11 {
		t.Errorf("Probe(threshold=10) = %d, want 11 (saturated)", got)
	}
	if got := Probe(root, 100); got != 50 {
		t.Errorf("Probe(threshold=100) = %d, want the true 50", got)
	}
}

// The probe must measure the SAME set the IndexStore would resident-ize.
// A repo whose bulk is node_modules must not trip a gate over files the
// index would never read — otherwise the gate fires on cost that isn't there.
func TestProbeSkipsWhatTheIndexSkips(t *testing.T) {
	root := t.TempDir()
	tree(t, root, 3)
	for _, skipped := range []string{"node_modules", ".git", "dist", "vendor"} {
		if !index.SkipDir(skipped) {
			t.Fatalf("precondition: index.SkipDir(%q) must be true", skipped)
		}
		tree(t, filepath.Join(root, skipped, "deep"), 100)
	}
	if got := Probe(root, 1000); got != 3 {
		t.Errorf("Probe counted %d, want 3 — vendored/derived trees must be pruned", got)
	}
}

func TestProbeCountsOrdinaryProjectDotDirs(t *testing.T) {
	root := t.TempDir()
	tree(t, filepath.Join(root, ".github"), 2)
	tree(t, filepath.Join(root, ".claude"), 3)
	if got := Probe(root, 100); got != 5 {
		t.Fatalf("Probe counted %d, want 5 project-dotdir files", got)
	}
}

// never/always and threshold=0 must short-circuit WITHOUT walking, so turning
// the gate off is genuinely free. An unreadable root proves no walk happened:
// a real walk would still return 0 here, so instead assert on Probed.
func TestResolveShortCircuits(t *testing.T) {
	root := t.TempDir()
	tree(t, root, 5)

	for _, tc := range []struct {
		name      string
		threshold int
		mode      string
		wantLarge bool
	}{
		{"never", 1, ModeNever, false},
		{"always", 1_000_000, ModeAlways, true},
		{"threshold zero", 0, ModeAuto, false},
	} {
		v := Resolve(root, tc.threshold, tc.mode)
		if v.Large != tc.wantLarge {
			t.Errorf("%s: Large = %v, want %v", tc.name, v.Large, tc.wantLarge)
		}
		if v.Probed {
			t.Errorf("%s: must not walk the tree", tc.name)
		}
		if v.Reason == "" {
			t.Errorf("%s: verdict must carry an operator-facing reason", tc.name)
		}
	}
}

func TestResolveAuto(t *testing.T) {
	root := t.TempDir()
	tree(t, root, 20)

	big := Resolve(root, 5, ModeAuto)
	if !big.Large || !big.Probed {
		t.Errorf("20 files over threshold 5: got Large=%v Probed=%v", big.Large, big.Probed)
	}
	small := Resolve(root, 500, ModeAuto)
	if small.Large {
		t.Errorf("20 files under threshold 500 must not be large")
	}
	if small.Files != 20 {
		t.Errorf("Files = %d, want 20", small.Files)
	}
	// An unrecognized mode is treated as auto rather than silently disabling
	// the gate — config validation is what rejects typos, and if one ever got
	// through, failing toward "measure it" beats failing toward "spend it".
	if v := Resolve(root, 5, "nonsense"); !v.Large {
		t.Errorf("unknown mode must behave as auto")
	}
}

// Directories themselves are not files, and neither are symlinks: the index
// reads regular files only, so the probe must count only those.
func TestProbeCountsRegularFilesOnly(t *testing.T) {
	root := t.TempDir()
	tree(t, root, 2)
	if err := os.MkdirAll(filepath.Join(root, "sub", "deeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "f0.go"), filepath.Join(root, "link.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got := Probe(root, 100); got != 2 {
		t.Errorf("Probe = %d, want 2 (dirs and symlinks are not counted)", got)
	}
}
