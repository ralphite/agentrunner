// Package runtime wires the harness together; this file owns filesystem
// locations and naming (S1.7a — the single place these decisions live).
package runtime

import (
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

// NewSessionID builds the sortable session id: YYYYMMDD-HHMMSS-<slug>
// (slug = first 30 chars of the task, lowercased, non-alphanumerics → "-").
func NewSessionID(now time.Time, task string) string {
	return now.UTC().Format("20060102-150405") + "-" + slugify(task, 30)
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
		return "task"
	}
	return slug
}
