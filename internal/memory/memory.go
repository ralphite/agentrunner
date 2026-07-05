// Package memory merges CLAUDE.md memory files hierarchically (S5.2): from
// the outermost ancestor (the git repo root, or the workspace itself when no
// repo encloses it) DOWN to the workspace root. Outer files render first and
// nearer files later, so the nearest instructions take precedence for the
// model. The merged text is frozen into SessionStarted at session start — memory
// edits mid-run never rewrite the prompt prefix.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// File is one discovered memory file.
type File struct {
	Dir     string // directory the CLAUDE.md was found in (absolute)
	Content string
}

// Collect walks from root upward to the enclosing git root (or stops at the
// filesystem root) and returns every CLAUDE.md found, ordered outermost
// first. Unreadable files are skipped.
func Collect(root string) ([]File, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}
	// Gather candidate dirs innermost→outermost, stopping at the git root
	// (inclusive) when one exists.
	var dirs []string
	dir := abs
	for {
		dirs = append(dirs, dir)
		if isGitRoot(dir) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// No enclosing repo: only the workspace root itself counts —
			// walking to / would hoover up unrelated ancestors.
			dirs = dirs[:1]
			break
		}
		dir = parent
	}
	// Render outermost first.
	var out []File
	for i := len(dirs) - 1; i >= 0; i-- {
		raw, err := os.ReadFile(filepath.Join(dirs[i], "CLAUDE.md"))
		if err != nil {
			continue
		}
		out = append(out, File{Dir: dirs[i], Content: string(raw)})
	}
	return out, nil
}

func isGitRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// Render merges collected files into one byte-stable memory block, each
// section labeled with its source path relative to the workspace root (or
// absolute when outside it).
func Render(files []File, root string) string {
	if len(files) == 0 {
		return ""
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	var b strings.Builder
	b.WriteString("<memory>\n")
	for i, f := range files {
		label := f.Dir
		if rel, err := filepath.Rel(abs, f.Dir); err == nil && !strings.HasPrefix(rel, "..") {
			label = rel
		}
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "# CLAUDE.md (%s)\n%s", label, strings.TrimRight(f.Content, "\n"))
		b.WriteString("\n")
	}
	b.WriteString("</memory>")
	return b.String()
}
