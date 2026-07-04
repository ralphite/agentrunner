package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/daemon"
	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// socketPath is the daemon's rendezvous, fixed under the data dir so every
// client finds the same runtime. The dir is created here: the daemon may be
// the first agentrunner process this machine ever ran. (unix sockets cap
// paths at ~108 bytes — an extravagant XDG_DATA_HOME surfaces as a bind
// error, which ListenAndServe reports verbatim.)
func socketPath() (string, error) {
	data, err := runtime.DataDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(data, 0o700); err != nil {
		return "", fmt.Errorf("daemon: %w", err)
	}
	return filepath.Join(data, "daemon.sock"), nil
}

// daemonCmd runs the resident runtime (S6 模块④): `agentrunner daemon`.
// Approvals resolve through AGENTRUNNER_APPROVE (fail-closed when unset) —
// interactive approval routing over the socket is a later step.
func daemonCmd(args []string, version string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if len(fs.Args()) != 0 {
		fmt.Fprintln(stderr, "usage: agentrunner daemon")
		return ExitUsage
	}
	ctx, _, stop := signalContext()
	defer stop()
	loadDotEnv(".env")

	sock, err := socketPath()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	srv := &daemon.Server{
		SocketPath: sock,
		NewID:      func(task string) string { return runtime.NewSessionID(time.Now(), task) },
		Run:        hostRunFunc(version, stderr),
		Replay: func(sessionID string, sink protocol.Sink) error {
			dir, err := resolveSessionDir(sessionID)
			if err != nil {
				return err
			}
			return daemon.ReplayJournal(dir, sink)
		},
		ScanTimers: scanSessionTimers,
		Resume:     hostResumeFunc(version, stderr),
	}
	fmt.Fprintf(stderr, "daemon on %s\n", sock)
	if err := srv.ListenAndServe(ctx); err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	return ExitOK
}

// hostRunFunc is the daemon's real run wiring — the same assembly as a
// foreground `run` minus the tty concerns (no interrupts, env-only
// approvals; the daemon is headless).
func hostRunFunc(version string, stderr io.Writer) daemon.RunFunc {
	return func(ctx context.Context, req daemon.RunRequest, sink protocol.Sink) error {
		spec, err := agent.LoadSpec(req.SpecPath)
		if err != nil {
			return err
		}
		wsRoot := req.Workspace
		if wsRoot == "" {
			wsRoot = "."
		}
		ws, err := workspace.New(wsRoot)
		if err != nil {
			return err
		}
		prov, err := defaultProviderFactory(ctx, spec.Model.Provider)
		if err != nil {
			return err
		}
		sessionDir, err := runtime.SessionDir(req.SessionID)
		if err != nil {
			return err
		}
		events, err := store.OpenEventStore(sessionDir)
		if err != nil {
			return err
		}
		defer func() { _ = events.Close() }()

		mode := spec.Mode
		if req.Mode != "" {
			mode = req.Mode
		}
		pipe, hooks, err := buildPipeline(ws, spec.Permissions, mode, spec.Budget.MaxTotalTokens, stderr)
		if err != nil {
			return err
		}
		loop := &agent.Loop{
			Spec:      spec,
			Provider:  prov,
			Exec:      &tool.Executor{WS: ws, Session: req.SessionID},
			Store:     events,
			Clock:     clock.Real{},
			Out:       sink,
			SessionID: req.SessionID,
			Version:   version,
			Pipeline:  pipe,
			Mode:      mode,
			Hooks:     hooks,
			Approvals: &agent.EnvApprovals{},
			SubSpecs:  siblingSpecResolver(req.SpecPath),
		}
		_, runErr := loop.Run(ctx, req.Task)
		return runErr
	}
}

// scanSessionTimers derives the pending-timer index from the session
// journals (timer 派生索引): every non-ended session with pending timers
// reports its earliest fire time. Unreadable or unfoldable sessions are
// skipped — the sweep must not die on one corrupt log.
func scanSessionTimers() ([]daemon.SessionTimer, error) {
	data, err := runtime.DataDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(data, "sessions"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []daemon.SessionTimer
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(data, "sessions", e.Name())
		events, err := store.ReadEvents(dir)
		if err != nil {
			continue
		}
		s, err := state.Fold(events)
		if err != nil || s.Run.Status == state.StatusEnded || len(s.Timers) == 0 {
			continue
		}
		var earliest time.Time
		for _, tm := range s.Timers {
			if earliest.IsZero() || tm.FireAt.Before(earliest) {
				earliest = tm.FireAt
			}
		}
		out = append(out, daemon.SessionTimer{SessionID: e.Name(), FireAt: earliest})
	}
	return out, nil
}

// hostResumeFunc is the daemon's timer-driven resume wiring — the same
// assembly as a foreground `resume` minus the tty: spec and workspace come
// from the journaled RunStarted, permissions from the journaled layers.
func hostResumeFunc(version string, stderr io.Writer) func(context.Context, string, protocol.Sink) error {
	return func(ctx context.Context, sessionID string, sink protocol.Sink) error {
		dir, err := resolveSessionDir(sessionID)
		if err != nil {
			return err
		}
		started, err := readRunStarted(dir)
		if err != nil {
			return err
		}
		if len(started.Spec) == 0 || started.WorkspaceRoot == "" {
			return fmt.Errorf("session %s predates resumable metadata", sessionID)
		}
		var spec agent.AgentSpec
		if err := json.Unmarshal(started.Spec, &spec); err != nil {
			return fmt.Errorf("journaled spec: %w", err)
		}
		ws, err := workspace.New(started.WorkspaceRoot)
		if err != nil {
			return err
		}
		prov, err := defaultProviderFactory(ctx, spec.Model.Provider)
		if err != nil {
			return err
		}
		events, err := store.OpenEventStore(dir)
		if err != nil {
			return err
		}
		defer func() { _ = events.Close() }()

		var pipe *pipeline.Pipeline
		var hooks *hook.Runner
		if len(started.PermissionLayers) > 0 {
			var layers [][]pipeline.PermissionRule
			if err := json.Unmarshal(started.PermissionLayers, &layers); err != nil {
				return fmt.Errorf("journaled permission layers: %w", err)
			}
			pipe, hooks, err = buildPipelineFromLayers(ws, layers, spec.Mode, spec.Budget.MaxTotalTokens, stderr)
		} else {
			pipe, hooks, err = buildPipeline(ws, spec.Permissions, spec.Mode, spec.Budget.MaxTotalTokens, stderr)
		}
		if err != nil {
			return err
		}
		loop := &agent.Loop{
			Spec:      &spec,
			Provider:  prov,
			Exec:      &tool.Executor{WS: ws, Session: sessionID},
			Store:     events,
			Clock:     clock.Real{},
			Out:       sink,
			SessionID: sessionID,
			Version:   version,
			Pipeline:  pipe,
			Hooks:     hooks,
			Approvals: &agent.EnvApprovals{},
		}
		_, runErr := loop.Resume(ctx)
		return runErr
	}
}

// attachCmd follows a session hosted by the daemon: journal catch-up first,
// then the live stream. `agentrunner attach [--json] <session-id-or-prefix>`.
func attachCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("attach", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit the event stream as JSON lines")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner attach [--json] <session-id-or-prefix>")
		return ExitUsage
	}
	// Resolve prefixes locally so the wire carries the full id.
	dir, err := resolveSessionDir(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	session := filepath.Base(dir)

	sock, err := socketPath()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	var sink protocol.Sink
	if *jsonOut {
		sink = protocol.NewJSONSink(stdout)
	} else {
		sink = newTextRenderer(stdout)
	}
	if err := daemon.Dial(sock, daemon.Command{Cmd: "attach", Session: session}, sink.Emit); err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v (is the daemon running?)\n", err)
		return ExitRun
	}
	return ExitOK
}

// submitCmd hands a run to the daemon and streams it until it ends —
// `run`, hosted: the run survives this client. `agentrunner submit [flags]
// <spec.yaml> "task"`.
func submitCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceDir := fs.String("workspace", ".", "workspace root (default: current directory)")
	mode := fs.String("mode", "", "run mode: default|plan|acceptEdits (overrides spec)")
	jsonOut := fs.Bool("json", false, "emit the event stream as JSON lines")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	rest := fs.Args()
	if len(rest) != 2 {
		fmt.Fprintln(stderr, "usage: agentrunner submit [flags] <spec.yaml> \"task\"")
		return ExitUsage
	}
	specPath, err := filepath.Abs(rest[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}
	wsAbs, err := filepath.Abs(*workspaceDir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}
	sock, err := socketPath()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	var sink protocol.Sink
	if *jsonOut {
		sink = protocol.NewJSONSink(stdout)
	} else {
		sink = newTextRenderer(stdout)
	}
	reason := ""
	err = daemon.Dial(sock, daemon.Command{
		Cmd: "run", SpecPath: specPath, Task: rest[1], Workspace: wsAbs, Mode: *mode,
	}, func(e protocol.Event) {
		if e.Kind == protocol.KindRunStart && e.Session != "" {
			fmt.Fprintf(stderr, "session %s\n", e.Session)
		}
		if e.Kind == protocol.KindRunEnd {
			reason = e.Reason
		}
		sink.Emit(e)
	})
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v (is the daemon running?)\n", err)
		return ExitRun
	}
	// No run_end in the stream means the run died before its terminal event
	// (e.g. spec/provider failure inside the daemon) — that is a failure,
	// same contract as a foreground run.
	if reason != "completed" {
		return ExitRun
	}
	return ExitOK
}
