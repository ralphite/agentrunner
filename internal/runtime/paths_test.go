package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDataDirXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-test")
	dir, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != "/tmp/xdg-test/agentrunner" {
		t.Errorf("dir = %q", dir)
	}
}

func TestDataDirFallback(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	dir, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(dir, filepath.Join(".local", "share", "agentrunner")) {
		t.Errorf("dir = %q", dir)
	}
}

func TestSessionDirCreated0700(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir, err := SessionDir("20260703-120000-test")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("mode = %o, want 0700", info.Mode().Perm())
	}
}

func TestNewSessionID(t *testing.T) {
	at := time.Date(2026, 7, 3, 12, 30, 45, 0, time.UTC)
	cases := []struct{ task, wantPrefix string }{
		{"Fix the failing test!", "20260703-123045-fix-the-failing-test-"},
		{"修复 bug in parser", "20260703-123045-bug-in-parser-"},
		{"???", "20260703-123045-task-"},
		{strings.Repeat("x", 100), "20260703-123045-" + strings.Repeat("x", 30) + "-"},
	}
	for _, tc := range cases {
		got := NewSessionID(at, tc.task)
		if !strings.HasPrefix(got, tc.wantPrefix) || len(got) != len(tc.wantPrefix)+4 {
			t.Errorf("NewSessionID(%q) = %q, want prefix %q + 4 hex", tc.task, got, tc.wantPrefix)
		}
	}
	first := NewSessionID(at, "same")
	second := NewSessionID(at, "same")
	if first == second {
		t.Error("same-second ids should differ")
	}
}

func TestProjectConfigPath(t *testing.T) {
	if got := ProjectConfigPath("/w"); got != "/w/.agentrunner/settings.yaml" {
		t.Errorf("got %q", got)
	}
}
