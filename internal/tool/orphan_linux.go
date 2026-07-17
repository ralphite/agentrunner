//go:build linux

package tool

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// listOrphanSessionGroups scans procfs for processes tagged with
// SessionEnvVar whose parent is init — the runtime that spawned them is gone.
// Unreadable entries (permissions, exit races) are skipped: only positive
// evidence acts. Known limit: under a PID-namespace subreaper orphans
// reparent to the subreaper, not pid 1, and stay out of reach — the sweep
// under-collects there, it never over-collects.
func listOrphanSessionGroups() []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	seen := map[int]bool{}
	var groups []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 1 {
			continue
		}
		environ, err := os.ReadFile(filepath.Join("/proc", e.Name(), "environ"))
		if err != nil || !hasSessionMarker(environ) {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		ppid, pgid, ok := parseProcStat(string(stat))
		if !ok || ppid != 1 || pgid <= 1 {
			continue
		}
		if !seen[pgid] {
			seen[pgid] = true
			groups = append(groups, pgid)
		}
	}
	return groups
}

func hasSessionMarker(environ []byte) bool {
	for _, kv := range bytes.Split(environ, []byte{0}) {
		if bytes.HasPrefix(kv, []byte(SessionEnvVar+"=")) {
			return true
		}
	}
	return false
}

// parseProcStat pulls ppid and pgrp (fields 4 and 5) out of /proc/<pid>/stat.
// The comm field may itself contain spaces and parens, so fields are counted
// from after its closing paren.
func parseProcStat(stat string) (ppid, pgid int, ok bool) {
	i := strings.LastIndex(stat, ")")
	if i < 0 {
		return 0, 0, false
	}
	fields := strings.Fields(stat[i+1:])
	if len(fields) < 3 {
		return 0, 0, false
	}
	ppid, err1 := strconv.Atoi(fields[1])
	pgid, err2 := strconv.Atoi(fields[2])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return ppid, pgid, true
}
