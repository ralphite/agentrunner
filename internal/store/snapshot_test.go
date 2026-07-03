package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	versions := map[string]int{"run": 1}
	if err := WriteSnapshot(dir, 7, versions, map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	snap, ok, err := LatestSnapshot(dir)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if snap.UptoSeq != 7 || snap.SubStateVersions["run"] != 1 {
		t.Fatalf("snap = %+v", snap)
	}
	fi, err := os.Stat(filepath.Join(dir, "snapshots", "7.json"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %v err = %v", fi.Mode(), err)
	}
}

// A corrupt newest snapshot degrades to the next-older readable one —
// snapshots are an optimization and must never block resume.
func TestLatestSnapshotSkipsCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSnapshot(dir, 3, map[string]int{"run": 1}, "old"); err != nil {
		t.Fatal(err)
	}
	// Newest is torn (power loss shape: exists but unparseable).
	if err := os.WriteFile(filepath.Join(dir, "snapshots", "9.json"), []byte(`{"upto_`), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, ok, err := LatestSnapshot(dir)
	if err != nil || !ok || snap.UptoSeq != 3 {
		t.Fatalf("snap=%+v ok=%v err=%v", snap, ok, err)
	}

	// All corrupt → ok=false, no error.
	dir2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir2, "snapshots"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "snapshots", "5.json"), []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, ok, err = LatestSnapshot(dir2)
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want graceful none", ok, err)
	}
}

func TestLatestSnapshotNone(t *testing.T) {
	_, ok, err := LatestSnapshot(t.TempDir())
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
