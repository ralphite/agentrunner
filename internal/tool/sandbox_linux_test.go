//go:build linux

package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// INC-75: the probe error is an operator's only clue in a fresh
// environment — both failure shapes must carry their fix.

func TestLinuxSandboxHintMissingBwrap(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no bwrap anywhere on this PATH
	_, err := platformSandboxProbe(false)
	if err == nil {
		t.Fatal("probe succeeded with an empty PATH, want bubblewrap-unavailable error")
	}
	for _, want := range []string{"bubblewrap unavailable", "sudo apt-get install -y bubblewrap", "ar doctor"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

func TestLinuxSandboxHintProbeFailure(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "bwrap")
	script := "#!/bin/sh\necho 'bwrap: setting up uid map: Permission denied' >&2\nexit 1\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	_, err := platformSandboxProbe(false)
	if err == nil {
		t.Fatal("probe succeeded with a failing bwrap stub, want probe error")
	}
	for _, want := range []string{"bubblewrap probe", "Permission denied", "apparmor_restrict_unprivileged_userns", "ar doctor"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

// A working bwrap (stubbed) keeps the probe green — the hints must not leak
// into the success path.
func TestLinuxSandboxProbeSuccess(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "bwrap")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	backend, err := platformSandboxProbe(true)
	if err != nil {
		t.Fatalf("probe = %v, want success with stub bwrap", err)
	}
	if backend != "bwrap" {
		t.Fatalf("backend = %q, want bwrap", backend)
	}
}
