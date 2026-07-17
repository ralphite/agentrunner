//go:build linux

package tool

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParseProcStat(t *testing.T) {
	cases := []struct {
		stat       string
		ppid, pgid int
		ok         bool
	}{
		{"42 (sleep) S 1 42 42 0 -1", 1, 42, true},
		{"42 (weird name) with) parens) R 7 9 9 0", 7, 9, true},
		{"no closing paren", 0, 0, false},
		{"42 (x) S", 0, 0, false},
	}
	for _, c := range cases {
		ppid, pgid, ok := parseProcStat(c.stat)
		if ppid != c.ppid || pgid != c.pgid || ok != c.ok {
			t.Errorf("parseProcStat(%q) = (%d,%d,%v), want (%d,%d,%v)",
				c.stat, ppid, pgid, ok, c.ppid, c.pgid, c.ok)
		}
	}
}

// TestSweepOrphanSessionProcessesKillsStrayGroup covers G22c end to end: a
// marker-tagged process whose spawner died is found by the scan and its whole
// group is killed by the sweep. Environments where orphans reparent to a
// subreaper instead of pid 1 skip (the sweep deliberately under-collects
// there — see orphan_linux.go).
func TestSweepOrphanSessionProcessesKillsStrayGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	// The sh wrapper gets its own process group, spawns a long sleep into it,
	// writes the sleep's pid, and exits — orphaning the group.
	cmd := exec.Command("sh", "-c", fmt.Sprintf("sleep 300 & echo $! > %q", pidFile))
	cmd.Env = append(os.Environ(), SessionEnvVar+"=qa-orphan-sweep")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	pgid := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	sleepPid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("bad pid file %q: %v", raw, err)
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", sleepPid))
	if err != nil {
		t.Fatalf("orphan sleep vanished: %v", err)
	}
	ppid, gotPgid, ok := parseProcStat(string(stat))
	if !ok || gotPgid != pgid {
		t.Fatalf("stat parse = (%d,%d,%v), want pgid %d", ppid, gotPgid, ok, pgid)
	}
	if ppid != 1 {
		t.Skipf("orphan reparented to subreaper %d, not init; sweep out of scope here", ppid)
	}

	found := false
	for _, g := range listOrphanSessionGroups() {
		if g == pgid {
			found = true
		}
	}
	if !found {
		t.Fatalf("scan missed orphan group %d", pgid)
	}

	killed := SweepOrphanSessionProcesses()
	hit := false
	for _, g := range killed {
		if g == pgid {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("sweep did not kill group %d (killed %v)", pgid, killed)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-pgid, syscall.Signal(0)); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("process group %d still alive after sweep", pgid)
}
