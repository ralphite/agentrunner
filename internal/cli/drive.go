package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/driver"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// driveOptions carries everything driveAgent needs; factored for testability.
type driveOptions struct {
	specPath  string
	workspace string
	version   string
	factory   providerFactory
	stdout    io.Writer
	stderr    io.Writer
	sink      protocol.Sink
}

// driveCmd parses `drive` args and runs an IterationDriver to its terminal
// state (S6). The task lives in the driver spec, not on the command line.
func driveCmd(args []string, version string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("drive", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceDir := fs.String("workspace", ".", "workspace root (default: current directory)")
	jsonOut := fs.Bool("json", false, "emit the child runs' output event stream as JSON lines")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintf(stderr, "usage: agentrunner drive [flags] <driver.yaml>\n")
		return ExitUsage
	}
	var sink protocol.Sink
	if *jsonOut {
		sink = protocol.NewJSONSink(stdout)
	} else {
		sink = newTextRenderer(stdout)
	}
	return driveAgent(driveOptions{
		specPath:  rest[0],
		workspace: *workspaceDir,
		version:   version,
		factory:   defaultProviderFactory,
		stdout:    stdout,
		stderr:    stderr,
		sink:      sink,
	})
}

func driveAgent(opts driveOptions) int {
	ctx, _, stop := signalContext()
	defer stop()
	loadDotEnv(".env")

	spec, err := driver.LoadSpec(opts.specPath)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitUsage
	}
	ws, err := workspace.New(opts.workspace)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitUsage
	}
	prov, err := opts.factory(ctx, spec.Agent.Model.Provider)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		if errors.Is(err, errUnknownProvider) {
			return ExitUsage
		}
		return ExitRun
	}

	driverID := runtime.NewSessionID(time.Now(), spec.Name)
	sessionDir, err := runtime.SessionDir(driverID)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitRun
	}
	dStore, err := store.OpenEventStore(sessionDir)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitRun
	}
	defer func() { _ = dStore.Close() }()
	artifacts, err := store.OpenArtifactStore(filepath.Join(sessionDir, "artifacts"))
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitRun
	}
	fmt.Fprintf(opts.stderr, "driver %s\n", driverID)

	exec := &tool.Executor{WS: ws, Session: driverID}
	approvals := approvalResolver(opts.stderr)
	d := &driver.Driver{
		Spec:      spec,
		Store:     dStore,
		Clock:     clock.Real{},
		DriverID:  driverID,
		Exec:      exec,
		Judge:     prov,
		Approvals: approvals,
		Artifacts: artifacts,
		// Each iteration's child mirrors a plain `run`: same pipeline
		// construction, same approval seam; the min-aggregated allowance
		// clamps the frozen spec AND its budget gate.
		NewChild: func(cs *store.EventStore, session string, iter, budgetTokens int) *agent.Loop {
			frozen := *spec.Agent
			if budgetTokens > 0 {
				frozen.Budget.MaxTotalTokens = budgetTokens
			}
			pipe, hooks, perr := buildPipeline(ws, frozen.Permissions, frozen.Mode,
				frozen.Budget.MaxTotalTokens, opts.stderr)
			if perr != nil {
				// Surfaced by the child run's immediate failure; the driver's
				// on_child_failure policy decides what happens next.
				fmt.Fprintln(opts.stderr, perr)
			}
			fmt.Fprintf(opts.stderr, "iteration %d (%s)\n", iter, session)
			return &agent.Loop{
				Spec:      &frozen,
				Provider:  prov,
				Exec:      &tool.Executor{WS: ws, Session: session},
				Store:     cs,
				Clock:     clock.Real{},
				Out:       opts.sink,
				SessionID: session,
				Version:   opts.version,
				Pipeline:  pipe,
				Mode:      frozen.Mode,
				Hooks:     hooks,
				Approvals: approvals,
				SubSpecs:  siblingSpecResolver(opts.specPath),
			}
		},
	}

	res, runErr := d.Run(ctx)
	if runErr != nil {
		fmt.Fprintf(opts.stderr, "drive failed: %v\n", runErr)
		return ExitRun
	}
	fmt.Fprintf(opts.stderr, "driver %s: %d iterations (best %d)\n",
		res.Reason, res.Iterations, res.BestIter)
	if !driveSucceeded(spec, res.Reason) {
		return ExitRun
	}
	return ExitOK
}

// driveSucceeded maps a terminal reason to the exit code contract: goal mode
// succeeds only when satisfied; a bounded loop-mode series also ends
// normally at max_iterations.
func driveSucceeded(spec *driver.DriverSpec, reason string) bool {
	if reason == "satisfied" {
		return true
	}
	loopMode := spec.Schedule != "" && spec.Schedule != driver.ScheduleImmediate
	return loopMode && reason == "max_iterations"
}
