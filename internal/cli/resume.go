package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// resumeCmd implements `agentrunner resume <session-id-or-prefix>`: the
// spec and workspace root come from the session's RunStarted event, so no
// spec file argument is needed.
func resumeCmd(args []string, version string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner resume <session-id-or-prefix>")
		return ExitUsage
	}
	ctx, interrupts, stop := signalContext()
	defer stop()
	loadDotEnv(".env")

	dir, err := resolveSessionDir(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	sessionID := filepath.Base(dir)

	started, err := readRunStarted(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	if len(started.Spec) == 0 || started.WorkspaceRoot == "" {
		fmt.Fprintf(stderr, "agentrunner: session %s predates resumable metadata (no spec in run_started)\n", sessionID)
		return ExitRun
	}
	var spec agent.AgentSpec
	if err := json.Unmarshal(started.Spec, &spec); err != nil {
		fmt.Fprintf(stderr, "agentrunner: journaled spec: %v\n", err)
		return ExitRun
	}

	ws, err := workspace.New(started.WorkspaceRoot)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	prov, err := defaultProviderFactory(ctx, spec.Model.Provider)
	if err != nil {
		fmt.Fprintln(stderr, err)
		if errors.Is(err, errUnknownProvider) {
			return ExitUsage
		}
		return ExitRun
	}
	events, err := store.OpenEventStore(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	defer func() { _ = events.Close() }()

	fmt.Fprintf(stderr, "resuming session %s\n", sessionID)
	// The live mode comes from the fold; spec.Mode is only the gate's
	// static fallback. Journaled permission layers (S6) are the run's frozen
	// effective rules — a child session resumed standalone keeps its
	// parent's bounds; sessions predating the field fall back to the
	// config-merge path.
	var pipe *pipeline.Pipeline
	var hooks *hook.Runner
	if len(started.PermissionLayers) > 0 {
		var layers [][]pipeline.PermissionRule
		if err := json.Unmarshal(started.PermissionLayers, &layers); err != nil {
			fmt.Fprintf(stderr, "agentrunner: journaled permission layers: %v\n", err)
			return ExitRun
		}
		pipe, hooks, err = buildPipelineFromLayers(ws, layers, spec.Mode, spec.Budget.MaxTotalTokens, stderr)
	} else {
		pipe, hooks, err = buildPipeline(ws, spec.Permissions, spec.Mode, spec.Budget.MaxTotalTokens, stderr)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	loop := &agent.Loop{
		Spec:       &spec,
		Provider:   prov,
		Exec:       &tool.Executor{WS: ws, Session: sessionID},
		Store:      events,
		Clock:      clock.Real{},
		Out:        newTextRenderer(stdout),
		SessionID:  sessionID,
		Version:    version,
		Interrupts: interrupts,
		Pipeline:   pipe,
		Hooks:      hooks,
		Approvals:  approvalResolver(stderr),
	}
	result, runErr := loop.Resume(ctx)
	if runErr != nil {
		// An already-finished session is not a resume failure: report its
		// result, and exit 0 when it completed (nothing left to do).
		if result.Reason != "" {
			fmt.Fprintf(stderr, "%v\n", runErr)
			fmt.Fprintf(stderr, "run %s: %d turns, %d in / %d out tokens\n",
				result.Reason, result.Turns, result.Usage.InputTokens, result.Usage.OutputTokens)
			if result.Reason == "completed" {
				return ExitOK
			}
			return ExitRun
		}
		fmt.Fprintf(stderr, "resume failed: %v\n", runErr)
		return ExitRun
	}
	fmt.Fprintf(stderr, "run %s: %d turns, %d in / %d out tokens\n",
		result.Reason, result.Turns, result.Usage.InputTokens, result.Usage.OutputTokens)
	if result.Reason != "completed" {
		return ExitRun
	}
	return ExitOK
}

func readRunStarted(dir string) (*event.RunStarted, error) {
	events, err := store.ReadEvents(dir)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 || events[0].Type != event.TypeRunStarted {
		return nil, fmt.Errorf("session log does not begin with run_started")
	}
	decoded, err := event.DecodePayload(events[0])
	if err != nil {
		return nil, err
	}
	return decoded.(*event.RunStarted), nil
}

// sessionsCmd implements `agentrunner sessions list`: newest first, with
// the folded status.
func sessionsCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "list" {
		fmt.Fprintln(stderr, "usage: agentrunner sessions list")
		return ExitUsage
	}
	data, err := runtime.DataDir()
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	root := filepath.Join(data, "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		fmt.Fprintln(stdout, "no sessions")
		return ExitOK
	}
	type row struct {
		id, status string
		turns      int
		mtime      int64
	}
	var rows []row
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		r := row{id: e.Name(), status: "unreadable"}
		if info, err := e.Info(); err == nil {
			r.mtime = info.ModTime().UnixNano()
		}
		if events, err := store.ReadEvents(filepath.Join(root, e.Name())); err == nil {
			if s, err := state.Fold(events); err == nil {
				r.status = s.Run.Status
				if s.Waiting != nil {
					r.status = "waiting:" + s.Waiting.Kind
				}
				r.turns = s.Run.Turn
			}
		}
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "no sessions")
		return ExitOK
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].mtime > rows[j].mtime })
	fmt.Fprintf(stdout, "%-45s %-18s %s\n", "SESSION", "STATUS", "TURNS")
	for _, r := range rows {
		fmt.Fprintf(stdout, "%-45s %-18s %d\n", r.id, r.status, r.turns)
	}
	return ExitOK
}
