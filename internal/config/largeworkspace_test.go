package config

import (
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/wsprobe"
)

// scaleSettings writes a settings.yaml into a fresh temp dir, reusing the
// package's existing helper.
func scaleSettings(t *testing.T, body string) string {
	t.Helper()
	return writeSettings(t, t.TempDir(), body)
}

// Absent config must land on the measured default, not on "off".
func TestLargeWorkspaceDefaults(t *testing.T) {
	m := Merge(Settings{}, Settings{}, nil, false)
	if m.LargeWorkspaceThreshold != wsprobe.DefaultThreshold {
		t.Errorf("threshold = %d, want %d", m.LargeWorkspaceThreshold, wsprobe.DefaultThreshold)
	}
	if m.LargeWorkspaceMode != wsprobe.ModeAuto {
		t.Errorf("mode = %q, want %q", m.LargeWorkspaceMode, wsprobe.ModeAuto)
	}
}

// The scale gate is a performance knob with no security surface, so unlike
// permissions it is NOT subject to the untrusted-project tightening ladder:
// a monorepo's scale is a property of the repo, and the repo knows it. This
// asserts the carve-out holds even when the workspace is untrusted.
func TestLargeWorkspaceProjectWinsEvenUntrusted(t *testing.T) {
	n := 1234
	user := Settings{LargeWorkspace: LargeWorkspaceSpec{Threshold: intp(9999), Mode: wsprobe.ModeAuto}}
	project := Settings{LargeWorkspace: LargeWorkspaceSpec{Threshold: &n, Mode: wsprobe.ModeNever}}

	for _, trusted := range []bool{true, false} {
		m := Merge(user, project, nil, trusted)
		if m.LargeWorkspaceThreshold != n {
			t.Errorf("trusted=%v: threshold = %d, want project's %d", trusted, m.LargeWorkspaceThreshold, n)
		}
		if m.LargeWorkspaceMode != wsprobe.ModeNever {
			t.Errorf("trusted=%v: mode = %q, want project's never", trusted, m.LargeWorkspaceMode)
		}
	}
}

// User settings still apply when the project says nothing.
func TestLargeWorkspaceUserOnly(t *testing.T) {
	m := Merge(Settings{LargeWorkspace: LargeWorkspaceSpec{Threshold: intp(77)}}, Settings{}, nil, true)
	if m.LargeWorkspaceThreshold != 77 {
		t.Errorf("threshold = %d, want 77", m.LargeWorkspaceThreshold)
	}
	if m.LargeWorkspaceMode != wsprobe.ModeAuto {
		t.Errorf("mode = %q, want auto when unset", m.LargeWorkspaceMode)
	}
}

// An explicit `threshold: 0` means "gate off" and must survive merging — the
// whole reason Threshold is a pointer. If 0 were indistinguishable from unset,
// turning the gate off would silently do nothing.
func TestLargeWorkspaceExplicitZeroSurvives(t *testing.T) {
	m := Merge(Settings{LargeWorkspace: LargeWorkspaceSpec{Threshold: intp(0)}}, Settings{}, nil, true)
	if m.LargeWorkspaceThreshold != 0 {
		t.Errorf("threshold = %d, want the explicit 0", m.LargeWorkspaceThreshold)
	}
}

// A typo'd mode must fail LOUDLY. A file that reads "never degrade" but
// behaves as auto is exactly the silent mismatch this gate must not add.
func TestLargeWorkspaceRejectsBadMode(t *testing.T) {
	_, err := LoadFile(scaleSettings(t, "large_workspace:\n  mode: nevr\n"))
	if err == nil {
		t.Fatal("expected an error for a misspelled mode")
	}
	if !strings.Contains(err.Error(), "auto|never|always") {
		t.Errorf("error must name the valid modes, got: %v", err)
	}
}

func TestLargeWorkspaceRejectsNegativeThreshold(t *testing.T) {
	_, err := LoadFile(scaleSettings(t, "large_workspace:\n  threshold: -1\n"))
	if err == nil {
		t.Fatal("expected an error for a negative threshold")
	}
}

func TestLargeWorkspaceLoadsValid(t *testing.T) {
	s, err := LoadFile(scaleSettings(t, "large_workspace:\n  threshold: 12345\n  mode: always\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.LargeWorkspace.Threshold == nil || *s.LargeWorkspace.Threshold != 12345 {
		t.Errorf("threshold not parsed: %#v", s.LargeWorkspace.Threshold)
	}
	if s.LargeWorkspace.Mode != wsprobe.ModeAlways {
		t.Errorf("mode = %q, want always", s.LargeWorkspace.Mode)
	}
}

// Unlike default_model, large_workspace IS allowed in project settings —
// that is the point of the carve-out.
func TestLargeWorkspaceAllowedInProjectFile(t *testing.T) {
	if _, err := LoadProjectFile(scaleSettings(t, "large_workspace:\n  threshold: 10\n")); err != nil {
		t.Errorf("project settings must accept large_workspace: %v", err)
	}
}

func intp(n int) *int { return &n }
