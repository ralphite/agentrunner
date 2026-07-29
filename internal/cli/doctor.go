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
// sandbox probes the containment gate uses (决策 #34), for both network modes,
// so a missing backend surfaces with the fix in hand instead of as a
// mid-session denial. Since 决策 #34 修订 the boundary is OPT-IN, so what the
// report answers is "can this machine honor a containment request", not "can
// bash run at all" — the terminal-parity default needs no backend.
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
		fmt.Fprintf(stdout, "bash and command tools still run here: the default is terminal parity —\nyour full environment and real HOME, no backend required.\nOnly a spec asking for sandbox.filesystem=workspace or sandbox.network=none\nis fail-closed until the fix above is applied; then re-run `ar doctor`.\n")
		return ExitRun
	}
	fmt.Fprintf(stdout, "bash and command tools can run OS-contained here when a spec asks for it;\nwithout that opt-in they run with terminal parity.\n")
	return ExitOK
}
