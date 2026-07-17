package tool

import (
	"strconv"
	"strings"
)

// Orphaned session processes (G22c, audit-0717 B3): every process a session
// spawns carries SessionEnvVar (2.12). When the hosting runtime dies hard
// (kill -9), in-flight bash process groups survive it, reparented to init.
// The daemon boot sweep kills exactly those: the marker plus init-parentage
// is read from the LIVE process at sweep time — current-truth evidence, so
// PID reuse cannot misfire the way a journaled pid could.

// SweepOrphanSessionProcesses kills every process group that carries the
// session marker and lost its runtime, and returns the pgids it signalled.
// Platforms without a scan backend return nothing — the sweep acts only on
// positive evidence, never by guessing.
func SweepOrphanSessionProcesses() (killed []int) {
	groups := listOrphanSessionGroups()
	for _, pgid := range groups {
		killGroup(pgid, bashKillGrace)
	}
	return groups
}

type psProc struct{ pid, ppid, pgid int }

// parsePSTable parses `ps -axo pid=,ppid=,pgid=` output and keeps the rows
// whose ppid is 1 (reparented to init). Pure and platform-neutral so the
// parser has test coverage everywhere, not only where ps runs.
func parsePSTable(out string) []psProc {
	var procs []psProc
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		pgid, err3 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		if ppid == 1 && pid > 1 && pgid > 1 {
			procs = append(procs, psProc{pid: pid, ppid: ppid, pgid: pgid})
		}
	}
	return procs
}
