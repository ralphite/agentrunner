//go:build darwin

package tool

import (
	"os/exec"
	"strconv"
	"strings"
)

// listOrphanSessionGroups uses ps (no procfs on macOS): one pass for
// pid/ppid/pgid, then a per-candidate -E pass for the environment — ps only
// exposes same-user environments, which is exactly a session sweep's scope.
// Any exec or parse failure yields a no-op sweep: only positive evidence
// acts. The -E output folds env into the command string, so a command line
// merely MENTIONING the marker of an init-parented process is a theoretical
// false positive; combined with orphanhood the risk is accepted (记档).
func listOrphanSessionGroups() []int {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,pgid=").Output()
	if err != nil {
		return nil
	}
	seen := map[int]bool{}
	var groups []int
	for _, c := range parsePSTable(string(out)) {
		env, err := exec.Command("ps", "-p", strconv.Itoa(c.pid), "-Eww", "-o", "command=").Output()
		if err != nil || !strings.Contains(string(env), SessionEnvVar+"=") {
			continue
		}
		if !seen[c.pgid] {
			seen[c.pgid] = true
			groups = append(groups, c.pgid)
		}
	}
	return groups
}
