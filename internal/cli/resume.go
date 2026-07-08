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
// spec and workspace root come from the session's SessionStarted event, so no
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

	started, err := readSessionStarted(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	if len(started.Spec) == 0 || started.WorkspaceRoot == "" {
		fmt.Fprintf(stderr, "agentrunner: session %s predates resumable metadata (no spec in session_started)\n", sessionID)
		return ExitRun
	}
	// A SpecChanged fact supersedes the opening spec (决策 #32).
	specJSON, permLayers := started.Spec, started.PermissionLayers
	specPath := started.SpecPath
	if changed, cerr := readLatestSpecChange(dir); cerr == nil && changed != nil {
		specJSON, specPath, permLayers = changed.Spec, changed.SpecPath, changed.PermissionLayers
	}
	var spec agent.AgentSpec
	if err := json.Unmarshal(specJSON, &spec); err != nil {
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
	if len(permLayers) > 0 {
		var layers [][]pipeline.PermissionRule
		if err := json.Unmarshal(permLayers, &layers); err != nil {
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
		Snapshots:  snapshotStoreFor(ws, stderr),
		SpecPath:   specPath,
	}
	if specPath != "" {
		loop.SubSpecs = siblingSpecResolver(specPath)
	}
	result, runErr := loop.Resume(ctx)
	if runErr != nil {
		// An already-finished session is not a resume failure: report its
		// result, and exit 0 when it completed (nothing left to do).
		if result.Reason != "" {
			fmt.Fprintf(stderr, "%v\n", runErr)
			fmt.Fprintf(stderr, "run %s: %d turns, %d in / %d out tokens\n",
				result.Reason, result.GenSteps, result.Usage.InputTokens, result.Usage.OutputTokens)
			if result.Reason == "completed" {
				return ExitOK
			}
			return ExitRun
		}
		fmt.Fprintf(stderr, "resume failed: %v\n", runErr)
		return ExitRun
	}
	fmt.Fprintf(stderr, "run %s: %d turns, %d in / %d out tokens\n",
		result.Reason, result.GenSteps, result.Usage.InputTokens, result.Usage.OutputTokens)
	if result.Reason != "completed" {
		return ExitRun
	}
	return ExitOK
}

func readSessionStarted(dir string) (*event.SessionStarted, error) {
	events, err := store.ReadEvents(dir)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("session log is empty")
	}
	// A forked session opens with its ForkedFrom genesis (S7.3); the copied
	// SessionStarted sits right behind it. The genesis' workspace root overrides
	// the copied one — the fork lives in its own materialized worktree, and
	// the SessionStarted root is parent provenance.
	var forked *event.ForkedFrom
	head := events[0]
	if head.Type == event.TypeForkedFrom && len(events) > 1 {
		if decoded, derr := event.DecodePayload(head); derr == nil {
			forked = decoded.(*event.ForkedFrom)
		}
		head = events[1]
	}
	if head.Type != event.TypeSessionStarted {
		return nil, fmt.Errorf("session log does not begin with session_started")
	}
	decoded, err := event.DecodePayload(head)
	if err != nil {
		return nil, err
	}
	started := decoded.(*event.SessionStarted)
	if forked != nil && forked.WorkspaceRoot != "" {
		started.WorkspaceRoot = forked.WorkspaceRoot
	}
	return started, nil
}

// sessionsCmd implements `agentrunner sessions [list]`: newest first, with
// the folded status. Bare `sessions` lists too — it is every first-timer's
// first guess (INC-2).
func sessionsCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) > 1 || (len(args) == 1 && args[0] != "list") {
		fmt.Fprintln(stderr, "usage: agentrunner sessions [list]")
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
				// The status column reads the SHAPE (决策 #31): a close/kill
				// mark or quiescence names the finish; a live wait shows its
				// kind; otherwise the liveness status.
				r.status = s.Session.Status
				if s.Waiting != nil {
					r.status = "waiting:" + s.Waiting.Kind
				}
				if s.Session.Closed != nil {
					r.status = s.Session.Closed.Reason
				} else if q, reason := state.Quiescence(s); q {
					r.status = reason
				}
				// A "running" session (mid-turn, not waiting/closed/quiescent)
				// whose host process is gone is STRANDED, not running: the
				// daemon crashed or was restarted and nothing is advancing it
				// (T1/T2b — 状态撒谎). resume recovers it. The probe reads the
				// lock's pid; it never takes the lock, so it cannot disturb a
				// live writer.
				if r.status == "running" && !store.HasLiveWriter(filepath.Join(root, e.Name())) {
					r.status = "stranded"
				}
				r.turns = s.Session.GenStep
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
