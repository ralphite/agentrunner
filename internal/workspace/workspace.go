// Package workspace enforces the filesystem boundary all file-class tools
// must pass through (STAGES 钩子 1). Nothing in the harness touches the
// filesystem on the agent's behalf without resolving through here.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Workspace is the session's filesystem boundary: one PRIMARY root plus zero
// or more extra roots (INC-105 multi-root projects). The primary keeps every
// single-root semantic — cwd, relative-path base, the git/snapshot anchor —
// while extras add same-level read-write reach: a project the user declared
// as "these folders" is one boundary, not one folder and N exceptions.
type Workspace struct {
	root   string   // primary — absolute, symlink-resolved
	extras []string // additional roots, same resolution; never contains root
	// large is the resolved large-workspace verdict (internal/wsprobe),
	// stamped ONCE by the run assembly seam and read by the whole-tree gates
	// (IndexStore, shadow snapshot, sandbox credential scan). The zero value
	// is "not large", so a Workspace built by any path that never stamps it
	// keeps the un-gated behavior — the gate fails OPEN, toward today's
	// semantics, never toward silent degradation.
	large      bool
	largeFiles int
}

// SetScale stamps the large-workspace verdict. Called once per run from the
// config-assembly seam; files is the probe's saturating count, for messages
// only. Deliberately not part of New: New is O(1) by contract (three syscalls)
// and the probe is not free.
func (w *Workspace) SetScale(files int, large bool) {
	w.largeFiles, w.large = files, large
}

// IsLarge reports whether whole-tree subsystems must degrade for this
// workspace. False unless SetScale said otherwise.
func (w *Workspace) IsLarge() bool { return w.large }

// ScaleFiles is the probe's count behind IsLarge. It SATURATES at the
// threshold — never present it as the workspace's true file count.
func (w *Workspace) ScaleFiles() int { return w.largeFiles }

// New builds a Workspace rooted at dir.
func New(dir string) (*Workspace, error) {
	resolved, err := resolveRoot(dir)
	if err != nil {
		return nil, err
	}
	return &Workspace{root: resolved}, nil
}

// NewMultiRoot builds a Workspace whose boundary is the union of primary and
// extras (INC-105). Extras resolve exactly like the primary; duplicates of
// the primary or of each other collapse. A missing extra is an error — the
// caller declared it part of the boundary, and silently narrowing a boundary
// is worse than failing loudly.
func NewMultiRoot(primary string, extras []string) (*Workspace, error) {
	w, err := New(primary)
	if err != nil {
		return nil, err
	}
	for _, extra := range extras {
		resolved, err := resolveRoot(extra)
		if err != nil {
			return nil, err
		}
		if resolved == w.root || contains(w.extras, resolved) {
			continue
		}
		w.extras = append(w.extras, resolved)
	}
	return w, nil
}

func resolveRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("workspace root %s: %w", dir, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("workspace root %s: %w", dir, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("workspace root %s: %w", dir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace root %s: not a directory", dir)
	}
	return resolved, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Root returns the resolved PRIMARY root: the cwd, the relative-path base,
// and the anchor for every per-repo face (git, snapshots, memory, probes).
func (w *Workspace) Root() string {
	return w.root
}

// Roots returns every root, primary first. Single-root workspaces return
// exactly [Root()], so range-over-Roots call sites degrade to today's
// behavior with no special case.
func (w *Workspace) Roots() []string {
	return append([]string{w.root}, w.extras...)
}

// ExtraRoots returns the non-primary roots (empty for single-root sessions).
func (w *Workspace) ExtraRoots() []string {
	return append([]string(nil), w.extras...)
}

// Resolve maps a tool-supplied path (relative to the primary root, or
// absolute) to a real absolute path, rejecting anything that escapes the
// workspace after symlink and ".." resolution — including paths that do not
// exist yet (their deepest existing ancestor is resolved instead, so a new
// file behind an out-of-tree symlinked directory is still rejected). With
// extra roots (INC-105) "the workspace" is their union: a path inside ANY
// root is inside the boundary.
func (w *Workspace) Resolve(requested string) (string, error) {
	path := requested
	if !filepath.IsAbs(path) {
		path = filepath.Join(w.root, path)
	}
	path = filepath.Clean(path)

	resolved, err := resolveWithMissingTail(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", requested, err)
	}

	if underRoot(resolved, w.root) {
		return resolved, nil
	}
	for _, extra := range w.extras {
		if underRoot(resolved, extra) {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("path escapes workspace: %s -> %s", requested, resolved)
}

func underRoot(resolved, root string) bool {
	return resolved == root || strings.HasPrefix(resolved, root+string(filepath.Separator))
}

// ResolveOutside resolves a path the same way Resolve does — symlinks on the
// deepest existing ancestor — but without the workspace bound. It exists so
// that an out-of-workspace path the user has APPROVED (see Executor.GrantPath)
// is spelled identically when granted and when later looked up; comparing raw
// strings would let `/tmp/x` and `/private/tmp/x` disagree on macOS.
func ResolveOutside(path string) (string, error) {
	return resolveWithMissingTail(path)
}

// resolveWithMissingTail resolves symlinks for the deepest existing ancestor
// of path and re-appends the non-existing remainder.
func resolveWithMissingTail(path string) (string, error) {
	var tail []string
	current := path
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing ancestor for %s", path)
		}
		tail = append(tail, filepath.Base(current))
		current = parent
	}
}
