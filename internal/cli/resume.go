package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/driver"
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
	// A merged-stream series session (INC-80.2a) is program-driven — an
	// agent-loop resume would misinterpret it. Same refusal as the legacy
	// driver stream: the daemon's drive sweep recovers it.
	if _, _, ok := readSeriesSpec(dir); ok {
		fmt.Fprintf(stderr, "agentrunner: %s is a scheduled series session, not a conversation; the daemon recovers live series automatically, or use `agentrunner drive --retry %s` to start a new series\n", sessionID, sessionID)
		return ExitUsage
	}
	if events, rerr := store.ReadEvents(dir); rerr == nil && len(events) > 0 && events[0].Type == event.TypeDriverStarted {
		folded, ferr := driver.Fold(events)
		if ferr != nil {
			fmt.Fprintf(stderr, "agentrunner: driver journal: %v\n", ferr)
			return ExitRun
		}
		if folded.Status == driver.StatusEnded {
			fmt.Fprintf(stderr, "agentrunner: driver %s already ended (%s); use retry to start a new series\n", sessionID, folded.Reason)
		} else {
			fmt.Fprintf(stderr, "agentrunner: driver %s is a scheduled series, not a conversation; the daemon recovers live series automatically, or use retry to start a new series\n", sessionID)
		}
		return ExitUsage
	}

	// A session with a live writer is hosted by the running daemon — it is
	// NOT stranded, so there is nothing to resume, and opening the event store
	// here would only collide with the daemon's lock and surface a scary
	// "session locked: held by pid N" (QA Wave1 alice-08/dave-05). Point the
	// user at the gestures that actually continue a live session instead.
	if store.HasLiveWriter(dir) {
		fmt.Fprintf(stderr, "session %s is live under the running daemon — it isn't stranded, so there is nothing to resume.\n", sessionID)
		fmt.Fprintf(stderr, "  continue it:  agentrunner send %s \"<message>\"\n", sessionID)
		fmt.Fprintf(stderr, "  watch it:     agentrunner attach %s\n", sessionID)
		fmt.Fprintln(stderr, "resume recovers sessions stranded by a daemon crash or restart (status 'stranded' in `agentrunner sessions`).")
		return ExitUsage
	}

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
	// The session's own workspace supplies credentials too (QA Round1
	// F-A04/F-B7): resume is issued from anywhere, while the .env
	// convention lives at the workspace root — "durable, resumable" must
	// not hinge on the caller's cwd. The cwd .env (loaded above) wins;
	// loadDotEnv never overrides what is already set.
	loadDotEnv(filepath.Join(started.WorkspaceRoot, ".env"))
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

	// Don't announce "resuming" for a session that is already terminal — the
	// resume below immediately reports its closed/stopped result, and a leading
	// "resuming session …" before "session is closed" reads as a lie (QA Wave7
	// nate-05). A resumable (waiting/stranded) session still gets the notice.
	resumable := true
	if evs, ferr := store.ReadEvents(dir); ferr == nil {
		if s, serr := state.Fold(evs); serr == nil && s.Session.Closed != nil {
			resumable = false
		}
	}
	if resumable {
		fmt.Fprintf(stderr, "resuming session %s\n", sessionID)
	}
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
		Out:        newTextRenderer(stdout).anchor(sessionID),
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
	return sessionStartedFromEvents(events)
}

func sessionStartedFromEvents(events []event.Envelope) (*event.SessionStarted, error) {
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

// sessionsCmd implements `agentrunner sessions [list] [--json] [--limit N]
// [--offset N]`: newest first, with
// the folded status. Bare `sessions` lists too — it is every first-timer's
// first guess (INC-2).
func sessionsCmd(args []string, stdout, stderr io.Writer) int {
	jsonOutput := false
	seenList := false
	limit, offset := 0, 0
	usage := func() int {
		fmt.Fprintln(stderr, "usage: agentrunner sessions [list] [--json] [--limit N] [--offset N]")
		return ExitUsage
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "list":
			if seenList {
				return usage()
			}
			seenList = true
		case "--json":
			jsonOutput = true
		case "--limit", "--offset":
			if i+1 >= len(args) {
				return usage()
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 0 {
				return usage()
			}
			if arg == "--limit" {
				limit = n
			} else {
				offset = n
			}
			i++
		default:
			return usage()
		}
	}
	data, err := runtime.DataDir()
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	root := filepath.Join(data, "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		if jsonOutput {
			fmt.Fprintln(stdout, "[]")
		} else {
			fmt.Fprintln(stdout, "no sessions")
		}
		return ExitOK
	}
	type row struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		Turns     int    `json:"turns"`
		Workspace string `json:"workspace,omitempty"`
		Title     string `json:"title,omitempty"`
		Kind      string `json:"kind"`
		Schedule  string `json:"schedule,omitempty"`
		// Cadence/NextRunAt are the engine-computed schedule projection
		// (PLAN 3.1): the human phrase ("Every 30m") and the RFC3339 next
		// tick — only when a live series can actually fire again. webui
		// consumes these verbatim instead of re-deriving them.
		Cadence   string `json:"cadence,omitempty"`
		NextRunAt string `json:"next_run_at,omitempty"`
		mtime     int64
	}
	now := time.Now()
	type candidate struct {
		entry os.DirEntry
		mtime int64
	}
	candidates := make([]candidate, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !validSessionDir(filepath.Join(root, e.Name())) {
			continue
		}
		mtime := int64(0)
		// A session directory's own mtime usually stays at creation time while
		// events.jsonl keeps advancing. Sort by the journal mtime so an old session
		// with new activity returns to the first page, matching the UI's notion
		// of recent work.
		if info, statErr := os.Stat(filepath.Join(root, e.Name(), "events.jsonl")); statErr == nil {
			mtime = info.ModTime().UnixNano()
		} else if info, infoErr := e.Info(); infoErr == nil {
			mtime = info.ModTime().UnixNano()
		}
		candidates = append(candidates, candidate{entry: e, mtime: mtime})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].mtime > candidates[j].mtime })
	if offset >= len(candidates) {
		candidates = nil
	} else if offset > 0 {
		candidates = candidates[offset:]
	}
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}

	var rows []row
	for _, candidate := range candidates {
		e := candidate.entry
		r := row{ID: e.Name(), Status: "unreadable", Kind: "session"}
		r.mtime = candidate.mtime
		if events, err := store.ReadEvents(filepath.Join(root, e.Name())); err == nil {
			if isDriverJournal(events) {
				r.Kind = "driver"
				live := false
				lastTick := time.Time{}
				if s, derr := driver.Fold(events); derr == nil {
					r.Status = string(s.Status)
					if s.Status == driver.StatusEnded && s.Reason != "" {
						r.Status = s.Reason
					}
					if s.Status == driver.StatusRunning && !store.HasLiveWriter(filepath.Join(root, e.Name())) {
						r.Status = "stranded"
					}
					r.Turns = len(s.Iterations)
					live = s.Status != driver.StatusEnded
					lastTick = s.LastTick
				}
				if len(events) > 0 && events[0].Type == event.TypeDriverStarted {
					if decoded, derr := event.DecodePayload(events[0]); derr == nil {
						started := decoded.(*event.DriverStarted)
						r.Workspace = started.WorkspaceRoot
						r.Title = started.SpecName
						var spec driver.DriverSpec
						if json.Unmarshal(started.Spec, &spec) == nil {
							r.Schedule = spec.Schedule
							if r.Schedule == "" {
								r.Schedule = driver.ScheduleImmediate
							}
							if prompt := strings.TrimSpace(strings.SplitN(spec.Prompt, "\n", 2)[0]); prompt != "" {
								r.Title = prompt
							}
							r.Cadence = spec.Cadence()
							if live {
								if t, ok := spec.NextRunAt(lastTick, now); ok {
									r.NextRunAt = t.Format(time.RFC3339)
								}
							}
						}
					}
				}
			} else if s, err := state.Fold(events); err == nil {
				// The status column reads the SHAPE (决策 #31): a close/kill
				// mark or quiescence names the finish; a live wait shows its
				// kind; otherwise the liveness status.
				r.Status = s.Session.Status
				if s.Waiting != nil {
					r.Status = "waiting:" + s.Waiting.Kind
				}
				if s.Session.Closed != nil {
					r.Status = s.Session.Closed.Reason
				} else if q, reason := state.Quiescence(s); q {
					// A conversational session whose final generation is quiet is
					// ready for another message, not a completed unit of work. Preserve
					// its durable input wait; abnormal/goal terminal reasons still win.
					if s.Waiting == nil || s.Waiting.Kind != event.WaitInput || reason != "completed" {
						r.Status = reason
					}
				}
				// A "running" session (mid-turn, not waiting/closed/quiescent)
				// whose host process is gone is STRANDED, not running: the
				// daemon crashed or was restarted and nothing is advancing it
				// (T1/T2b — 状态撒谎). resume recovers it. The probe reads the
				// lock's pid; it never takes the lock, so it cannot disturb a
				// live writer.
				if r.Status == "running" && !store.HasLiveWriter(filepath.Join(root, e.Name())) {
					r.Status = "stranded"
				}
				// Same yardstick as inspect: conversation TURNS, not
				// gen-steps (QA Round1 F-A11 — the two disagreed here).
				r.Turns = len(s.Interactions.Turns)
				if started, serr := sessionStartedFromEvents(events); serr == nil {
					r.Workspace = started.WorkspaceRoot
					r.Title = strings.TrimSpace(strings.SplitN(started.Prompt, "\n", 2)[0])
				}
				// The auto/manual/fork title projection (INC-52, HANDA-PARITY
				// #14) is a journal fact: it wins over the opening first line
				// when present. Fallback stays the first line for legacy journals.
				if s.Session.RawTitle != "" {
					r.Title = s.Session.RawTitle
				}
				// A merged-stream series session (INC-80.2a) projects like a
				// driver row: same kind, schedule and engine-computed cadence
				// — the UI keys Scheduled rows off these fields.
				if s.Series != nil {
					r.Kind = "driver"
					r.Schedule = s.Series.Kind
					r.Turns = len(s.Series.Iterations)
					if s.Series.Ended && s.Series.EndReason != "" {
						r.Status = s.Series.EndReason
					}
					if spec, _, ok := readSeriesSpec(filepath.Join(root, e.Name())); ok {
						r.Cadence = spec.Cadence()
						if !s.Series.Ended {
							if t, ok := spec.NextRunAt(s.Series.LastTick, now); ok {
								r.NextRunAt = t.Format(time.RFC3339)
							}
						}
					}
				}
			}
		}
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		if jsonOutput {
			fmt.Fprintln(stdout, "[]")
		} else {
			fmt.Fprintln(stdout, "no sessions")
		}
		return ExitOK
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].mtime > rows[j].mtime })
	if jsonOutput {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rows); err != nil {
			fmt.Fprintf(stderr, "agentrunner: encode sessions: %v\n", err)
			return ExitRun
		}
		return ExitOK
	}
	fmt.Fprintf(stdout, "%-45s %-18s %s\n", "SESSION", "STATUS", "TURNS")
	for _, r := range rows {
		fmt.Fprintf(stdout, "%-45s %-18s %d\n", r.ID, r.Status, r.Turns)
	}
	return ExitOK
}
