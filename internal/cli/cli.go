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
	case "run":
		return runCmd(args[1:], false, version, stdout, stderr)
	case "drive":
		return driveCmd(args[1:], version, stdout, stderr)
	case "record-fixture":
		return runCmd(args[1:], true, version, stdout, stderr)
	case "accept":
		return acceptCmd(args[1:], stdout, stderr)
	case "events":
		return eventsCmd(args[1:], stdout, stderr)
	case "inspect":
		return inspectCmd(args[1:], stdout, stderr)
	case "resume":
		return resumeCmd(args[1:], version, stdout, stderr)
	case "daemon":
		return daemonCmd(args[1:], version, stdout, stderr)
	case "attach":
		return attachCmd(args[1:], stdout, stderr)
	case "submit":
		return submitCmd(args[1:], stdout, stderr)
	case "new":
		return newCmd(args[1:], stdout, stderr)
	case "send":
		return sendCmd(args[1:], stdout, stderr)
	case "close":
		return closeCmd(args[1:], stdout, stderr)
	case "interrupt":
		return interruptCmd(args[1:], stdout, stderr)
	case "approve":
		return approveCmd(args[1:], stdout, stderr)
	case "fork":
		return forkCmd(args[1:], stdout, stderr)
	case "barrier":
		return barrierCmd(args[1:], stdout, stderr)
	case "sessions":
		return sessionsCmd(args[1:], stdout, stderr)
	case "trust":
		return trustCmd(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "agentrunner: unknown command %q\n%s", args[0], usage())
		return ExitUsage
	}
}

func usage() string {
	return "usage: agentrunner <run|drive|daemon|new|send|close|interrupt|submit|attach|approve|resume|fork|barrier|sessions|events|inspect|trust|record-fixture|accept|--version> [flags] [<spec.yaml> \"task\"]\n"
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
