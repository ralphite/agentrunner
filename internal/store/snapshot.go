package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	if err := os.WriteFile(tmp, full, 0o600); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	crash.Point(crash.PointAfterSnapshotWrite)
	return nil
}

// LatestSnapshot loads the highest-seq snapshot, reporting ok=false when
// none exists.
func LatestSnapshot(sessionDir string) (Snapshot, bool, error) {
	entries, err := os.ReadDir(filepath.Join(sessionDir, snapshotsDir))
	if os.IsNotExist(err) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("snapshot: %w", err)
	}
	best := int64(-1)
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue // .tmp leftovers etc.
		}
		seq, err := strconv.ParseInt(name, 10, 64)
		if err != nil {
			continue
		}
		if seq > best {
			best = seq
		}
	}
	if best < 0 {
		return Snapshot{}, false, nil
	}
	raw, err := os.ReadFile(filepath.Join(sessionDir, snapshotsDir, fmt.Sprintf("%d.json", best)))
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("snapshot: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return Snapshot{}, false, fmt.Errorf("snapshot %d: %w", best, err)
	}
	return snap, true, nil
}
