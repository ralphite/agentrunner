package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/config"
	"github.com/ralphite/agentrunner/internal/workspace"
	"github.com/ralphite/agentrunner/internal/wsprobe"
)

// A large workspace declines to snapshot. Returning nil — rather than skipping
// barriers downstream — is what keeps DESIGN 决策 7's bold clause ("无 snapshot
// 则不落 barrier") true for free: the loop already treats a nil store as "no
// barrier", so the harness never advertises a rewind it cannot honor.
func TestSnapshotStoreNilForLargeWorkspace(t *testing.T) {
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ws.SetScale(999_999, true)

	var stderr bytes.Buffer
	if st := snapshotStoreFor(ws, &stderr); st != nil {
		t.Errorf("large workspace must get no snapshot store, got %T", st)
	}
}

// The same workspace at normal scale still gets a real store — the gate is the
// only thing that changed.
func TestSnapshotStoreOpensWhenNotLarge(t *testing.T) {
	if _, err := os.Stat("/usr/bin/git"); err != nil {
		if _, err := os.Stat("/opt/homebrew/bin/git"); err != nil {
			t.Skip("git not available")
		}
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if st := snapshotStoreFor(ws, &stderr); st == nil {
		t.Errorf("normal workspace must get a store; stderr: %s", stderr.String())
	}
}

// Degradation is announced. A run that quietly stopped snapshotting would look
// like a harness that lost rewind for no reason, so the note must name both
// what degraded and how to override it.
func TestStampWorkspaceScaleAnnouncesDegradation(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filepath.Join(root, "f"+string(rune('a'+i))+".go"), []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	stampWorkspaceScale(ws, config.Merged{
		LargeWorkspaceThreshold: 2, LargeWorkspaceMode: wsprobe.ModeAuto,
	}, &stderr)

	if !ws.IsLarge() {
		t.Fatal("5 files over threshold 2 must be large")
	}
	msg := stderr.String()
	for _, want := range []string{"large workspace", "grep", "snapshots off", "large_workspace.mode"} {
		if !strings.Contains(msg, want) {
			t.Errorf("note must mention %q; got %q", want, msg)
		}
	}
}

// Below the threshold nothing degrades and nothing is printed — the gate must
// be invisible in the ordinary case.
func TestStampWorkspaceScaleQuietWhenSmall(t *testing.T) {
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	stampWorkspaceScale(ws, config.Merged{
		LargeWorkspaceThreshold: wsprobe.DefaultThreshold, LargeWorkspaceMode: wsprobe.ModeAuto,
	}, &stderr)

	if ws.IsLarge() {
		t.Error("an empty workspace must not be large")
	}
	if stderr.Len() != 0 {
		t.Errorf("no note expected for a normal workspace, got %q", stderr.String())
	}
}

// mode=always is the escape hatch for reproducing large-repo behavior on a
// small tree, and must not need a big tree to take effect.
func TestStampWorkspaceScaleModeAlways(t *testing.T) {
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	stampWorkspaceScale(ws, config.Merged{
		LargeWorkspaceThreshold: wsprobe.DefaultThreshold, LargeWorkspaceMode: wsprobe.ModeAlways,
	}, &stderr)

	if !ws.IsLarge() {
		t.Error("mode=always must degrade regardless of size")
	}
}
