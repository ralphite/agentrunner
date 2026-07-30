// Package wsprobe answers one question cheaply: is this workspace too large
// for the harness's whole-tree subsystems (IndexStore, shadow snapshot, the
// sandbox credential scan)?
//
// The question is circular by nature — you cannot walk the tree to decide
// whether walking the tree is affordable — so the answer comes from a BOUNDED
// probe: count regular files and bail the instant the threshold is passed.
// Cost is therefore capped at threshold-many lstat calls (~190ms at the 50k
// default), paid once per run, no matter how large the tree actually is. Same
// shape as the webui's @-mention picker, which has been bounded this way from
// the start.
//
// The count deliberately reuses index.SkipDir so the probe measures the SAME
// set the IndexStore would resident-ize: a repo whose bulk is node_modules
// must not trip a gate over files the index would never have read. For the
// snapshot gate the probe is an acknowledged HEURISTIC — real staging cost is
// decided by the workspace's own .gitignore, which no bounded probe can know.
package wsprobe

import (
	"io/fs"
	"path/filepath"

	"github.com/ralphite/agentrunner/internal/index"
)

// DefaultThreshold is the file count above which whole-tree subsystems
// degrade. Chosen from measurement, not taste: the BM25 IndexStore holds a
// steady 9.8x the indexed source bytes resident (1.6 GB at 100k files, 4.9 GB
// at 300k), so 50k lands near 800 MB — the most this harness should hold by
// default without being asked. See docs/LOG.md for the full numbers.
const DefaultThreshold = 50000

// Modes for Verdict resolution. Anything else is treated as ModeAuto.
const (
	ModeAuto   = "auto"   // probe and decide
	ModeNever  = "never"  // never degrade, whatever the size
	ModeAlways = "always" // always degrade (reproducing large-repo behavior on a small tree)
)

// Verdict is the resolved scale policy for one workspace.
type Verdict struct {
	// Files is how many regular files the probe counted. It saturates at
	// Threshold+1 — it is a gate input, never a true total, and must not be
	// reported as one.
	Files int
	// Large is the verdict the gates read.
	Large bool
	// Threshold that produced the verdict (0 when the gate is off).
	Threshold int
	// Reason is a short operator-facing explanation.
	Reason string
	// Probed records whether the tree was actually walked, so callers can
	// tell a real measurement from a forced or disabled verdict.
	Probed bool
}

// Resolve decides the scale policy. mode ModeNever/ModeAlways and a
// non-positive threshold all short-circuit WITHOUT walking the tree, so
// turning the gate off costs nothing at all.
func Resolve(root string, threshold int, mode string) Verdict {
	switch mode {
	case ModeNever:
		return Verdict{Threshold: threshold, Reason: "large_workspace.mode=never"}
	case ModeAlways:
		return Verdict{Large: true, Threshold: threshold, Reason: "large_workspace.mode=always"}
	}
	if threshold <= 0 {
		return Verdict{Reason: "large_workspace.threshold=0 (gate off)"}
	}
	n := Probe(root, threshold)
	v := Verdict{Files: n, Large: n > threshold, Threshold: threshold, Probed: true}
	if v.Large {
		v.Reason = "more than " + itoa(threshold) + " indexable files"
	} else {
		v.Reason = itoa(n) + " indexable files"
	}
	return v
}

// Probe counts regular files under root, stopping as soon as the count
// exceeds threshold. The return therefore saturates at threshold+1. Skipped
// directories match index.SkipDir (vendored/derived trees and every dotdir),
// and symlinks are never followed — WalkDir does not descend them.
func Probe(root string, threshold int) int {
	n := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: count what we can, same as the index
		}
		if d.IsDir() {
			if path != root && index.SkipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		n++
		if n > threshold {
			return fs.SkipAll
		}
		return nil
	})
	return n
}

// itoa avoids pulling strconv in for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
