package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/driver"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// driveOptions carries everything driveAgent needs; factored for testability.
type driveOptions struct {
	specPath  string
	spec      *driver.DriverSpec
	workspace string
	version   string
	factory   providerFactory
	stdout    io.Writer
	stderr    io.Writer
	sink      protocol.Sink
	// series opts into the merged-stream session form (INC-80.2a): the
	// series journals as a SESSION (SessionStarted+SeriesStarted head), not
	// a DriverStarted stream. Opt-in until the webui cadence projection
	// reads both forms (PROCESS 回归红线: behavior changes land opt-in).
	series bool
}

// driveCmd parses `drive` args and runs an IterationDriver to its terminal
// state (S6). The prompt lives in the driver spec, not on the command line.
func driveCmd(args []string, version string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("drive", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceDir := fs.String("workspace", ".", "workspace root (default: current directory)")
	jsonOut := fs.Bool("json", false, "emit the child runs' output event stream as JSON lines")
	retry := fs.String("retry", "", "start a new driver series from a prior driver session")
	series := fs.Bool("series", false, "journal as a session-form series (merged stream; goal/interval/cron only)")
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	rest := fs.Args()
	if (*retry == "" && len(rest) != 1) || (*retry != "" && len(rest) != 0) {
		fmt.Fprintf(stderr, "usage: agentrunner drive [--retry <driver-session>] [flags] [driver.yaml]\n")
		return ExitUsage
	}
	var sink protocol.Sink
	if *jsonOut {
		sink = protocol.NewJSONSink(stdout)
	} else {
		sink = newTextRenderer(stdout)
	}
	opts := driveOptions{
		workspace: *workspaceDir,
		version:   version,
		factory:   defaultProviderFactory,
		stdout:    stdout,
		stderr:    stderr,
		sink:      sink,
		series:    *series,
	}
	if *retry == "" {
		opts.specPath = rest[0]
	} else {
		dir, err := resolveSessionDir(*retry)
		if err != nil {
			fmt.Fprintf(stderr, "agentrunner: %v\n", err)
			return ExitUsage
		}
		// Either journal form seeds a retry (INC-80.2c): a merged-stream
		// series carries the spec on its SessionStarted head, a legacy
		// stream on DriverStarted.
		if spec, wsRoot, ok := readSeriesSpec(dir); ok {
			if spec.Agent == nil {
				fmt.Fprintln(stderr, "agentrunner: prior series has no reusable embedded agent spec")
				return ExitRun
			}
			opts.spec = spec
			opts.workspace = wsRoot
		} else {
			started, err := readDriverStarted(dir)
			if err != nil {
				fmt.Fprintf(stderr, "agentrunner: %v\n", err)
				return ExitUsage
			}
			var spec driver.DriverSpec
			if err := json.Unmarshal(started.Spec, &spec); err != nil {
				fmt.Fprintf(stderr, "agentrunner: prior driver has no reusable spec: %v\n", err)
				return ExitRun
			}
			if spec.Agent == nil {
				fmt.Fprintln(stderr, "agentrunner: prior driver has no reusable embedded agent spec")
				return ExitRun
			}
			opts.spec = &spec
			opts.specPath = started.SpecPath
			opts.workspace = started.WorkspaceRoot
		}
	}
	return driveAgent(opts)
}

func driveAgent(opts driveOptions) int {
	ctx, _, stop := signalContext()
	defer stop()
	loadDotEnv(".env")

	spec := opts.spec
	if spec == nil {
		var err error
		spec, err = driver.LoadSpec(opts.specPath)
		if err != nil {
			fmt.Fprintln(opts.stderr, err)
			return ExitUsage
		}
	}
	loadDotEnv(filepath.Join(opts.workspace, ".env"))
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
	// Verifier adjudication (S7 还债①): merged user/project rules bind
	// first-match; the trailing allow is the DRIVER-TRUST layer — verifiers
	// are spec-declared config (same trust level as spec permissions), so an
	// unmatched verifier runs instead of hitting the interactive mode default.
	verifierPipe, _, err := buildPipeline(ws, []pipeline.PermissionRule{{Action: "allow"}}, "", 0, opts.stderr)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitRun
	}
	d := &driver.Driver{
		Spec:      spec,
		SpecPath:  absolutePath(opts.specPath),
		Store:     dStore,
		Clock:     clock.Real{},
		DriverID:  driverID,
		Exec:      exec,
		Judge:     prov,
		Approvals: approvals,
		Artifacts: artifacts,
		Pipeline:  verifierPipe,
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
		// Best-of-N (schedule=parallel): the attempt's whole face — executor
		// AND permission-gate path resolution — binds to its own worktree.
		Snapshots: snapshotStoreFor(ws, opts.stderr),
		NewChildAt: func(cs *store.EventStore, session string, iter, budgetTokens int, worktree string) *agent.Loop {
			frozen := *spec.Agent
			if budgetTokens > 0 {
				frozen.Budget.MaxTotalTokens = budgetTokens
			}
			wtWS, werr := workspace.New(worktree)
			if werr != nil {
				fmt.Fprintln(opts.stderr, werr)
				wtWS = ws // surfaced by the attempt's failure; never nil-deref
			}
			pipe, hooks, perr := buildPipeline(wtWS, frozen.Permissions, frozen.Mode,
				frozen.Budget.MaxTotalTokens, opts.stderr)
			if perr != nil {
				fmt.Fprintln(opts.stderr, perr)
			}
			fmt.Fprintf(opts.stderr, "attempt %d (%s) in %s\n", iter, session, worktree)
			return &agent.Loop{
				Spec:      &frozen,
				Provider:  prov,
				Exec:      &tool.Executor{WS: wtWS, Session: session},
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

	// Merged-stream is the DEFAULT for the shapes the series runner carries
	// (INC-80.2c flip; opt-in period was 2.2a, the webui cadence projection
	// now reads both forms). self_paced / parallel / retry stay on the
	// legacy stream until the runner grows them; --series insists and errors
	// instead of silently falling back.
	var res driver.Result
	var runErr error
	if opts.series && !d.SupportsSeries() {
		fmt.Fprintln(opts.stderr, "agentrunner: --series carries every shape except parallel with on_child_failure=retry — run this spec without --series")
		return ExitUsage
	}
	if d.SupportsSeries() {
		res, runErr = d.RunSeries(ctx)
	} else {
		res, runErr = d.Run(ctx)
	}
	if runErr != nil {
		fmt.Fprintf(opts.stderr, "drive failed: %v\n", runErr)
		return ExitRun
	}
	// "(best N)" only means something when an iteration actually met the bar:
	// the best-picker seeds on iteration 1, so a series where every iteration
	// scored 0 would otherwise brag "(best 1)" (QA Wave2 erin-03). Report the
	// winner only on success; on failure say plainly that none passed.
	if driveSucceeded(spec, res.Reason) {
		fmt.Fprintf(opts.stderr, "driver %s: %d iterations (best %d)\n",
			res.Reason, res.Iterations, res.BestIter)
		return ExitOK
	}
	fmt.Fprintf(opts.stderr, "driver %s: %d iterations (no iteration met the bar)\n",
		res.Reason, res.Iterations)
	return ExitRun
}

func absolutePath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
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
