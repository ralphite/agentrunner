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

// Workspace is a rooted directory with realpath boundary checks.
type Workspace struct {
	root string // absolute, symlink-resolved
}

// New builds a Workspace rooted at dir.
func New(dir string) (*Workspace, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("workspace root %s: %w", dir, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("workspace root %s: %w", dir, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("workspace root %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root %s: not a directory", dir)
	}
	return &Workspace{root: resolved}, nil
}

// Root returns the resolved workspace root.
func (w *Workspace) Root() string {
	return w.root
}

// Resolve maps a tool-supplied path (relative to root, or absolute) to a
// real absolute path, rejecting anything that escapes the workspace after
// symlink and ".." resolution — including paths that do not exist yet
// (their deepest existing ancestor is resolved instead, so a new file
// behind an out-of-tree symlinked directory is still rejected).
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

	if resolved != w.root && !strings.HasPrefix(resolved, w.root+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %s -> %s", requested, resolved)
	}
	return resolved, nil
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
