// Package runtime wires the harness together; this file owns filesystem
// locations and naming (S1.7a — the single place these decisions live).
package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// DataDir is the harness state root: $XDG_DATA_HOME/agentrunner, falling
// back to ~/.local/share/agentrunner (same rule on macOS by decision —
// not ~/Library).
func DataDir() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "agentrunner"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("data dir: %w", err)
	}
	return filepath.Join(home, ".local", "share", "agentrunner"), nil
}

// SessionDir returns (and creates, 0700) the directory for one session.
func SessionDir(sessionID string) (string, error) {
	if !ValidSessionID(sessionID) {
		return "", fmt.Errorf("session dir: invalid session id %q", sessionID)
	}
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(data, "sessions", sessionID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("session dir: %w", err)
	}
	return dir, nil
}

// ValidSessionID accepts the basename alphabet used by top-level session ids,
// child tree addresses, and user-entered unique prefixes. In particular it
// excludes path syntax: session lookup must never become arbitrary filesystem
// lookup under a user-controlled ../ or absolute path.
func ValidSessionID(id string) bool {
	if id == "" || len(id) > 512 {
		return false
	}
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

// UserConfigPath locates user-level settings.
func UserConfigPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "agentrunner", "settings.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config path: %w", err)
	}
	return filepath.Join(home, ".config", "agentrunner", "settings.yaml"), nil
}

// ProjectConfigPath locates project-level settings under a workspace root.
func ProjectConfigPath(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ".agentrunner", "settings.yaml")
}

// UserToolsDir locates the user-level command-tool manifest directory
// (INC-55): a sibling of the user settings file. These are the user's own
// machine — always loaded, like user-level hooks (决策 #19).
func UserToolsDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "agentrunner", "tools"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("tools dir: %w", err)
	}
	return filepath.Join(home, ".config", "agentrunner", "tools"), nil
}

// ProjectToolsDir locates the project-level command-tool manifest directory
// (INC-55): repo content under the Claude Code convention dir, a sibling of
// .claude/skills and .claude/commands. Loaded ONLY when the workspace is
// trusted (决策 #19) — a cloned repo must not hand over arbitrary execution.
func ProjectToolsDir(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ".claude", "tools")
}

// NewSessionID builds the sortable session id:
// YYYYMMDD-HHMMSS-<slug>-<16hex> (slug = first 30 bytes of the prompt,
// lowercased; the random suffix prevents same-second collisions from
// interleaving two runs into one journal).
func NewSessionID(now time.Time, prompt string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// A predictable fallback would recreate the exact collision this suffix
		// exists to prevent. Match event.NewCommandID: entropy failure is fatal.
		panic(fmt.Sprintf("session id: crypto/rand unavailable: %v", err))
	}
	return now.UTC().Format("20060102-150405") + "-" + slugify(prompt, 30) + "-" + hex.EncodeToString(b[:])
}

func slugify(s string, maxLen int) string {
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	var sb strings.Builder
	lastDash := true // suppress leading dash
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) && r < 128 || unicode.IsDigit(r) {
			sb.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			sb.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.TrimSuffix(sb.String(), "-")
	if slug == "" {
		return "session"
	}
	return slug
}
