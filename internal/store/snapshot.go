package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ralphite/agentrunner/internal/crash"
)

// Snapshot is a turn-boundary serialization of the fold state. It is an
// optimization, never a source of truth: resume must produce the same
// state from snapshot+tail as from a full fold (pinned by test).
type Snapshot struct {
	UptoSeq          int64           `json:"upto_seq"`
	SubStateVersions map[string]int  `json:"sub_state_versions"`
	State            json.RawMessage `json:"state"`
}

const snapshotsDir = "snapshots"

// WriteSnapshot atomically writes snapshots/<upto_seq>.json (0600).
func WriteSnapshot(sessionDir string, uptoSeq int64, versions map[string]int, st any) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	full, err := json.Marshal(Snapshot{UptoSeq: uptoSeq, SubStateVersions: versions, State: raw})
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	dir := filepath.Join(sessionDir, snapshotsDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	final := filepath.Join(dir, fmt.Sprintf("%d.json", uptoSeq))
	tmp := final + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	if _, err := f.Write(full); err != nil {
		_ = f.Close()
		return fmt.Errorf("snapshot: %w", err)
	}
	// fsync before rename: without it a power loss can make the rename
	// durable while the data is not (zero-length file after rename).
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("snapshot: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	crash.Point(crash.PointAfterSnapshotWrite)
	return nil
}

// LatestSnapshot loads the newest READABLE snapshot: corrupt or torn ones
// are skipped in favor of older siblings (snapshots are an optimization —
// the full fold is always available as the last resort). ok=false when
// none is usable.
func LatestSnapshot(sessionDir string) (Snapshot, bool, error) {
	entries, err := os.ReadDir(filepath.Join(sessionDir, snapshotsDir))
	if os.IsNotExist(err) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("snapshot: %w", err)
	}
	var seqs []int64
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue // .tmp leftovers etc.
		}
		seq, err := strconv.ParseInt(name, 10, 64)
		if err != nil {
			continue
		}
		seqs = append(seqs, seq)
	}
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] > seqs[j] })
	for _, seq := range seqs {
		raw, err := os.ReadFile(filepath.Join(sessionDir, snapshotsDir, fmt.Sprintf("%d.json", seq)))
		if err != nil {
			continue
		}
		var snap Snapshot
		if err := json.Unmarshal(raw, &snap); err != nil || snap.UptoSeq != seq {
			continue
		}
		return snap, true, nil
	}
	return Snapshot{}, false, nil
}
