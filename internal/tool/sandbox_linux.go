//go:build linux

package tool

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// The probe error is the operator's only clue in a fresh environment (a CI
// runner, a new machine), so it must carry the fix, not just the failure
// (INC-75). Two failure shapes, two distinct remedies.
const (
	bwrapMissingHint = "fix: install the distro bubblewrap package (e.g. `sudo apt-get install -y bubblewrap`); `ar doctor` verifies"
	bwrapProbeHint   = "fix: a userns/permission denial here usually means the kernel restricts unprivileged user namespaces (Ubuntu 23.10+ AppArmor) — install the distro bubblewrap package (its AppArmor profile covers /usr/bin/bwrap) or `sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0`; `ar doctor` verifies"
)

func platformSandboxProbe(networkNone bool) (string, error) {
	bin, err := exec.LookPath("bwrap")
	if err != nil {
		return "bwrap", fmt.Errorf("bubblewrap unavailable: %w — %s", err, bwrapMissingHint)
	}
	args := []string{"--ro-bind", "/", "/", "--proc", "/proc", "--dev", "/dev", "--unshare-pid"}
	if networkNone {
		args = append(args, "--unshare-net")
	}
	args = append(args, "/bin/true")
	if out, err := exec.Command(bin, args...).CombinedOutput(); err != nil {
		return "bwrap", fmt.Errorf("bubblewrap probe: %v: %s — %s", err, bytes.TrimSpace(out), bwrapProbeHint)
	}
	return "bwrap", nil
}

func platformSandboxCommand(root, command string, writable []string, denied []sandboxDeny, networkNone bool) (*exec.Cmd, error) {
	bin, err := exec.LookPath("bwrap")
	if err != nil {
		return nil, fmt.Errorf("bubblewrap unavailable: %w", err)
	}
	args := []string{"--die-with-parent", "--new-session", "--tmpfs", "/", "--proc", "/proc", "--dev", "/dev", "--unshare-pid", "--unshare-ipc", "--unshare-uts"}
	if networkNone {
		args = append(args, "--unshare-net")
	}
	for _, path := range []string{"/usr", "/bin", "/sbin", "/lib", "/lib64", "/opt", "/nix/store"} {
		if _, err := os.Stat(path); err == nil {
			args = append(args, "--ro-bind", path, path)
		}
	}
	for _, path := range []string{"/etc/ld.so.cache", "/etc/nsswitch.conf", "/etc/passwd", "/etc/group", "/etc/hosts", "/etc/resolv.conf", "/etc/ssl", "/etc/ca-certificates"} {
		if _, err := os.Stat(path); err == nil {
			args = append(args, "--ro-bind", path, path)
		}
	}
	parents := map[string]bool{}
	for _, path := range writable {
		for cur := filepath.Dir(path); cur != "/" && cur != "."; cur = filepath.Dir(cur) {
			parents[cur] = true
		}
	}
	ordered := make([]string, 0, len(parents))
	for path := range parents {
		ordered = append(ordered, path)
	}
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) < len(ordered[j]) })
	for _, path := range ordered {
		args = append(args, "--dir", path)
	}
	for _, path := range writable {
		args = append(args, "--bind", path, path)
	}
	// Hide credential-shaped paths even though their workspace parent is
	// writable. /dev/null covers files; an empty tmpfs covers directories.
	for _, d := range denied {
		if d.Subpath {
			args = append(args, "--tmpfs", d.Path)
		} else {
			args = append(args, "--ro-bind", "/dev/null", d.Path)
		}
	}
	args = append(args, "--chdir", root, "bash", "-c", command)
	return exec.Command(bin, args...), nil
}
