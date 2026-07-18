package cli

import (
	"fmt"
	"io"

	"github.com/ralphite/agentrunner/internal/tool"
)

// doctorProbe is swappable in tests: the real probes depend on the host
// having (or lacking) bubblewrap / sandbox-exec, which the test environment
// can't promise either way.
var doctorProbe = tool.DoctorSandbox

// doctorCmd is the environment preflight (INC-75): it runs the same OS
// sandbox probes the containment gate uses (决策 #34, fail-closed), for both
// network modes, so a missing backend surfaces before the first bash call —
// with the fix in hand — instead of as a mid-session denial.
func doctorCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprint(stderr, commandHelp("doctor"))
		return ExitUsage
	}
	backend, openErr, restrictedErr := doctorProbe()
	fmt.Fprintf(stdout, "OS sandbox backend: %s\n", backend)
	ok := true
	for _, probe := range []struct {
		mode string
		err  error
	}{{"network=all", openErr}, {"network=none", restrictedErr}} {
		if probe.err != nil {
			ok = false
			fmt.Fprintf(stdout, "  %-13s FAIL — %v\n", probe.mode+":", probe.err)
		} else {
			fmt.Fprintf(stdout, "  %-13s OK\n", probe.mode+":")
		}
	}
	if !ok {
		fmt.Fprintf(stdout, "bash and command tools refuse to run without the OS sandbox (fail-closed).\nApply the fix above, then re-run `ar doctor`.\n")
		return ExitRun
	}
	fmt.Fprintf(stdout, "bash and command tools will run OS-contained in this environment.\n")
	return ExitOK
}
