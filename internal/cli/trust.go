package cli

import (
	"fmt"
	"io"

	"github.com/ralphite/agentrunner/internal/config"
	"github.com/ralphite/agentrunner/internal/runtime"
)

// trustCmd implements `agentrunner trust <dir>`: register a workspace so
// its project-level hooks may run and its permission rules may grant.
func trustCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner trust <dir>")
		return ExitUsage
	}
	dataDir, err := runtime.DataDir()
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	if err := config.Trust(dataDir, args[0]); err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	fmt.Fprintf(stdout, "trusted %s\n", args[0])
	return ExitOK
}
