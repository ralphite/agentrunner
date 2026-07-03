// Package cli implements the agentrunner command-line interface.
package cli

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
)

// Exit codes per PLAN.md S1 执行包.
const (
	ExitOK    = 0 // run completed
	ExitRun   = 1 // run failed (provider/tool fatal)
	ExitUsage = 2 // usage or spec error
)

// Run executes the CLI and returns the process exit code.
func Run(args []string, version string, stdout, stderr io.Writer) int {
	setupLogging(stderr)

	if len(args) == 0 {
		fmt.Fprint(stderr, usage())
		return ExitUsage
	}

	switch args[0] {
	case "--version", "version":
		fmt.Fprintf(stdout, "agentrunner %s (%s)\n", version, runtime.Version())
		return ExitOK
	default:
		fmt.Fprintf(stderr, "agentrunner: unknown command %q\n%s", args[0], usage())
		return ExitUsage
	}
}

func usage() string {
	return "usage: agentrunner --version\n"
}

// setupLogging configures the process-wide slog default. Logs go to stderr
// (never stdout, which belongs to run output); AGENTRUNNER_DEBUG=1 raises
// the level to Debug.
func setupLogging(stderr io.Writer) {
	level := slog.LevelInfo
	if os.Getenv("AGENTRUNNER_DEBUG") == "1" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level})))
}
