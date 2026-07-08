package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ralphite/agentrunner/internal/blackboard"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/mcp"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/snapshot"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
)

// Loop is the S2 event-sourced agent loop: every input and side effect is
// journaled as an event, the fold state is the only working memory, and
// each step is decided from that state alone — which is exactly what makes
// snapshot-resume (2.13) a restart of the same decision function.
type Loop struct {
	Spec      *AgentSpec
	Provider  provider.Provider
	Exec      *tool.Executor
	Store     *store.EventStore
	Clock     clock.Clock
	Out       protocol.Sink
	SessionID string
	Version   string
	// Pipeline adjudicates every effect before execution (S3). nil is an
	// empty pipeline: everything allowed, resolutions still journaled.
	Pipeline *pipeline.Pipeline
	// Approvals resolves ask verdicts (3.5). nil = EnvApprovals.
	Approvals ApprovalResolver
	// Interrupts delivers user interrupts (first Ctrl-C). A receive during
	// WAITING_APPROVAL resolves the approval as denied-by-interrupt and
	// the run continues. nil = never fires.
	Interrupts <-chan struct{}
	// SpecPath is the spec file's location, journaled into SessionStarted so a
	// revived session can resolve sibling sub-agent specs (v2 M5.1). Empty
	// for spec-injected callers (tests).
	SpecPath string
	// UserInputs delivers live user inputs. Each received text is journaled
	// as InputReceived{source:"user"} and wakes a new turn; CLOSING the
	// channel is the close gesture (SessionClosed{closed} mark). nil means
	// no live input source is wired (headless run, spawned child, driver
	// iteration): the session still idles over in-flight background work,
	// but at quiescence drive() RETURNS instead of idling — standby lives in
	// the journal, and a later send/resume continues the same session.
	// There is only ONE session shape (决策 #31); this field is process
	// wiring, never a session property, and is not journaled.
	UserInputs <-chan protocol.UserInput
	// inboxClosed records that a boundary drain saw UserInputs close, so the
	// next idle closes the session instead of waiting (v2 M2.1).
	inboxClosed bool
	// Cancels delivers handles to cancel out of band (v2 M3.2): a user's
	// `kill <handle>` cancels one running child/task without entering the
	// conversation. Consumed at drive-loop safe points and during the idle.
	Cancels <-chan string
	// Mode is the STARTING mode (3.6): journaled as the first ModeChanged.
	// The live mode is fold state; empty means "default".
	Mode string
	// Hooks runs post-tool hooks (3.8); pre hooks live in the pipeline's
	// hook gate. nil = no hooks.
	Hooks *hook.Runner
	// MCP is the connected MCP tool face (S5.1). The connections are
	// out-of-band runtime state owned by the caller; the loop journals the
	// discovered schemas (ToolsDiscovered), dispatches mcp__ calls here, and
	// on resume reconciles the live face against the journaled one. nil =
	// no MCP tools.
	MCP MCPManager
	// SubSpecs resolves the spec.Agents whitelist to child specs (S5.3);
	// nil = spawning unavailable. Depth is this run's position in the agent
	// tree (0 = root); the spawn gate caps it.
	SubSpecs SubSpecResolver
	Depth    int
	// Board is the agent tree's shared blackboard (S5.4): created at the
	// root when the spec whitelists agents, inherited by every child. The
	// store is ephemeral runtime state — durable influence flows through
	// each run's journaled read_notes results, never the store itself.
	Board *blackboard.Board
	// BoardMirror, when set, receives every note the tree publishes (S6
	// 模块⑤: a hosting surface like the daemon forwards notes to attached
	// watchers). Wired into the board at creation; the tool face is
	// unaffected — it still depends on the spec's agents whitelist only.
	BoardMirror func(blackboard.Note)
	// Artifacts is the tree-shared deliverable CAS (S5.5): opened lazily at
	// the ROOT session (Store.Dir()/artifacts), inherited by children so
	// refs resolve tree-wide. Blob durability precedes the ArtifactPublished
	// fact, always.
	Artifacts *store.ArtifactStore
	// Inputs are artifact refs to materialize into the workspace before the
	// first turn (S5.8): journaled into SessionStarted, written by an idempotent
	// materialize activity. Set by a spawning parent (or a future CLI flag).
	Inputs []event.ArtifactInput
	// Snapshots is the workspace SnapshotStore (S7.2): barriers are taken
	// only when it is present AND a snapshot succeeds — no ref, no barrier
	// (backend=none degrades to zero barriers, nothing else changes).
	Snapshots snapshot.Store
	// bg is the background-task runtime (S6.1): cancel handles + the done
	// channel. Ephemeral — the durable truth is the tasks sub-state.
	bg *bgRuntime
}

// MCPManager is the slice of mcp.Manager the loop needs (an interface so
// tests can fake a face without live transports).
type MCPManager interface {
	SetAllowed(names []string)
	Discover(ctx context.Context) ([]mcp.DiscoveredTool, error)
	Call(ctx context.Context, qualified string, args json.RawMessage) (json.RawMessage, bool, error)
}

var _ MCPManager = (*mcp.Manager)(nil)

// RunResult summarizes the session shape at the moment drive() returned —
// quiescence (决策 #31) or a close mark. Reason names the finishing shape
// ("completed" | "max_generation_steps" | "limit_exceeded" | "blocked" |
// "malformed_tool_call" | "handoff" | "contract_violation" | "closed"); it
// is an observer value, never a journal fact.
type RunResult struct {
	Reason   string
	GenSteps int
	Usage    provider.Usage
}

// driveState is the loop's working memory: the fold state plus the tip of
// the causation chain. drive() owns it; appendE mutates it.
type driveState struct {
	s      state.State
	lastID string
	// quiesceReason names the latest quiescent shape (observer value for
	// RunResult / the parent receipt; never journaled).
	quiesceReason string
}

// compact renders raw JSON on one line, dropping surrounding whitespace.
func compact(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// emit sends an output event to the surface (nil-safe).
func (l *Loop) emit(e protocol.Event) {
	if l.Out != nil {
		l.Out.Emit(e)
	}
}

// interruptScope derives a per-activity context cancelled with cause
// errs.ErrUserInterrupt when a steering interrupt (S4.2, first Ctrl-C)
// arrives. stop() must be called after the activity. A hard cancel of the
// parent ctx (second Ctrl-C / SIGTERM) propagates unchanged and keeps its
// own cause, so the loop can tell steering (continue) from quit (abort).
func (l *Loop) interruptScope(ctx context.Context) (context.Context, func()) {
	if l.Interrupts == nil {
		return ctx, func() {}
	}
	actCtx, cancel := context.WithCancelCause(ctx)
	done := make(chan struct{})
	go func() {
		select {
		case <-l.Interrupts:
			cancel(errs.ErrUserInterrupt)
		case <-done:
		}
	}()
	return actCtx, func() { close(done); cancel(nil) }
}

// steered reports whether an activity ended because of a steering interrupt
// (as opposed to a hard cancel or a normal error).
func steered(actCtx context.Context) bool {
	return errors.Is(context.Cause(actCtx), errs.ErrUserInterrupt)
}

// onSteeringInterrupt journals the interrupt as a control input (audit;
// journal-inputs-first) and emits a surface notice. The interrupt is NOT
// a conversation message (the fold drops source=="interrupt").
func (l *Loop) onSteeringInterrupt(appendE AppendFunc, turn int) error {
	if _, err := appendE(event.TypeInputReceived, &event.InputReceived{
		Text: "[interrupt]", Source: "interrupt",
	}); err != nil {
		return err
	}
	l.emit(protocol.Event{Kind: protocol.KindDiscard, N: turn, Text: "interrupted by user"})
	return nil
}

// appender builds the single write path: journal one event, fold it, and
// advance the linear causation chain. EVERY payload passes through
// credential redaction here — args/results are also redacted upstream in
// the executor, but this blanket is what keeps session_started (task, spec),
// input_received, and assistant messages (a model echoing a secret it
// read) out of the durable log and, via the fold, out of snapshots and
// later provider requests.
func (l *Loop) appender(ds *driveState) AppendFunc {
	r := redact.FromEnv()
	return func(typ string, payload any) (event.Envelope, error) {
		env, err := event.New(typ, payload)
		if err != nil {
			return env, err
		}
		env.Payload = r.JSON(env.Payload)
		env.CausationID = ds.lastID
		env.CorrelationID = l.SessionID
		appended, err := l.Store.Append(env)
		if err != nil {
			return appended, err
		}
		ds.lastID = appended.ID
		next, err := state.Apply(ds.s, appended)
		if err != nil {
			return appended, err
		}
		ds.s = next
		return appended, nil
	}
}

// Run drives the loop to completion for a single task.
func (l *Loop) Run(ctx context.Context, task string) (RunResult, error) {
	if l.Clock == nil {
		l.Clock = clock.Real{}
	}
	l.ensureBoard()
	l.ensureApprovals()
	l.applySandbox()
	if err := l.ensureArtifacts(); err != nil {
		return RunResult{}, err
	}
	// The task is external input and may carry a shell-expanded credential;
	// IngestInput appends via the store directly (not the appender), so it
	// must be scrubbed here.
	task = redact.FromEnv().String(task)
	ds := &driveState{s: state.New()}
	appendE := l.appender(ds)

	specJSON, err := json.Marshal(l.Spec)
	if err != nil {
		return RunResult{}, err
	}
	var wsRoot string
	if l.Exec != nil && l.Exec.WS != nil {
		wsRoot = l.Exec.WS.Root()
	}
	memoryBlock, skillsBlock := renderContextBlocks(wsRoot)
	if _, err := appendE(event.TypeSessionStarted, &event.SessionStarted{
		SpecName: l.Spec.Name, Model: l.Spec.Model.ID, Task: task,
		Version: l.Version, SubStateVersions: state.SubStateVersions(),
		Spec: specJSON, WorkspaceRoot: wsRoot,
		SpecPath: l.SpecPath,
		Env:      renderEnvBlock(wsRoot, l.Clock.Now()),
		Memory:   memoryBlock, Skills: skillsBlock,
		Agents: renderAgentsDirectory(l.Spec.Agents, l.SubSpecs),
		Inputs: l.Inputs,
		// The effective permission rules, materialized as data (S6): a child
		// pipeline holds the parent's gates, so this captures the WHOLE
		// intersection chain — a standalone resume of this session rebuilds
		// the same gates without the parent process.
		PermissionLayers: marshalPermissionLayers(l.Pipeline),
	}); err != nil {
		return RunResult{}, err
	}
	input, err := runtime.IngestInput(l.Store, l.SessionID, task, "cli")
	if err != nil {
		return RunResult{}, err
	}
	ds.lastID = input.ID
	if ds.s, err = state.Apply(ds.s, input); err != nil {
		return RunResult{}, err
	}
	if l.Mode != "" && l.Mode != pipeline.ModeDefault {
		if !pipeline.ValidMode(l.Mode) {
			return RunResult{}, fmt.Errorf("unknown mode %q", l.Mode)
		}
		if _, err := appendE(event.TypeModeChanged, &event.ModeChanged{
			To: l.Mode, Cause: "startup",
		}); err != nil {
			return RunResult{}, err
		}
	}
	if err := l.discoverMCP(ctx, appendE); err != nil {
		return RunResult{}, err
	}
	if err := l.materializeInputs(ctx, ds, appendE); err != nil {
		return RunResult{}, err
	}
	l.emit(protocol.Event{Kind: protocol.KindSessionStart, Mode: ds.s.CurrentMode()})

	return l.drive(ctx, ds, appendE)
}

// discoverMCP journals the MCP tool face at session start (S5.1): the
// spec's allowed_tools narrowing is applied first, then each server's
// discovered schemas land as one ToolsDiscovered fact. The connections
// themselves stay out-of-band.
func (l *Loop) discoverMCP(ctx context.Context, appendE AppendFunc) error {
	if l.MCP == nil {
		return nil
	}
	l.MCP.SetAllowed(l.Spec.AllowedTools)
	tools, err := l.MCP.Discover(ctx)
	if err != nil {
		return fmt.Errorf("mcp discovery: %w", err)
	}
	for _, group := range groupByServer(tools) {
		if _, err := appendE(event.TypeToolsDiscovered, &group); err != nil {
			return err
		}
	}
	return nil
}

// groupByServer buckets discovered tools into per-server ToolsDiscovered
// payloads, in server order (input is already name-sorted).
func groupByServer(tools []mcp.DiscoveredTool) []event.ToolsDiscovered {
	byServer := map[string]*event.ToolsDiscovered{}
	var order []string
	for _, t := range tools {
		g, ok := byServer[t.Server]
		if !ok {
			g = &event.ToolsDiscovered{Server: t.Server}
			byServer[t.Server] = g
			order = append(order, t.Server)
		}
		g.Tools = append(g.Tools, event.MCPToolDef{
			Server: t.Server, Name: t.Name, Description: t.Description,
			Class: t.Class, InputSchema: t.InputSchema,
		})
	}
	sort.Strings(order)
	out := make([]event.ToolsDiscovered, 0, len(order))
	for _, s := range order {
		out = append(out, *byServer[s])
	}
	return out
}

// Resume rebuilds the fold — snapshot plus event tail when a snapshot
// exists, full fold otherwise — and re-enters the same drive loop. A
// sub-state version mismatch is refused, never silently migrated.
func (l *Loop) Resume(ctx context.Context) (RunResult, error) {
	if l.Clock == nil {
		l.Clock = clock.Real{}
	}
	// A resumed collaboration gets a FRESH board: notes are ephemeral by
	// doctrine (what mattered was journaled as read results), and the face
	// must match the original run's. The artifact store is durable and
	// simply reopens.
	l.ensureBoard()
	l.ensureApprovals()
	l.applySandbox()
	if err := l.ensureArtifacts(); err != nil {
		return RunResult{}, err
	}
	dir := l.Store.Dir()
	events, err := store.ReadEvents(dir)
	if err != nil {
		return RunResult{}, err
	}
	if len(events) == 0 {
		return RunResult{}, fmt.Errorf("resume: session has no events")
	}

	// The versions journaled at run start guard EVERY resume, snapshot or
	// not — a full fold across an incompatible sub-state shape is just as
	// wrong as a snapshot load. A forked session's SessionStarted sits right
	// behind its ForkedFrom genesis (S7.3) and guards the fork the same way.
	head := events[0]
	if head.Type == event.TypeForkedFrom && len(events) > 1 {
		head = events[1]
	}
	if head.Type == event.TypeSessionStarted {
		if decoded, derr := event.DecodePayload(head); derr == nil {
			if started := decoded.(*event.SessionStarted); len(started.SubStateVersions) > 0 {
				if err := checkVersions(started.SubStateVersions); err != nil {
					return RunResult{}, err
				}
			}
		}
	}

	var s state.State
	snap, ok, err := store.LatestSnapshot(dir)
	if err != nil {
		// Snapshots are an optimization, never a source of truth: a
		// corrupt one degrades to the full fold instead of blocking.
		slog.Warn("resume: ignoring unreadable snapshot, folding from scratch", "err", err)
		ok = false
	}
	if ok {
		if err := checkVersions(snap.SubStateVersions); err != nil {
			return RunResult{}, err
		}
		if err := json.Unmarshal(snap.State, &s); err != nil {
			slog.Warn("resume: snapshot state unreadable, folding from scratch", "err", err)
			ok = false
		} else {
			for _, e := range events {
				if e.Seq <= snap.UptoSeq {
					continue
				}
				if s, err = state.Apply(s, e); err != nil {
					return RunResult{}, err
				}
			}
		}
	}
	// A snapshot written by a binary that predates Run.LastInputGenStep folds
	// it as 0, which would mis-compute the per-turn budget (收口
	// review). Snapshots are disposable caches: on the suspicious shape,
	// discard and fold from scratch — the fold recomputes the field exactly.
	if ok && s.Session.LastInputGenStep == 0 && s.Session.GenStep > 0 {
		ok = false
		s = state.State{}
	}
	if !ok {
		if s, err = state.Fold(events); err != nil {
			return RunResult{}, err
		}
	}

	// No resume refusal here (决策 #30/#31): there is no terminal state.
	// A close/kill mark gates AUTOMATIC paths at their call sites; this
	// explicit gesture lawfully continues any session.

	ds := &driveState{s: s, lastID: events[len(events)-1].ID}
	appendE := l.appender(ds)

	// A crash between session_started and input_received leaves the task
	// durable in SessionStarted but never journaled as input — re-ingest it
	// rather than silently calling the model with an empty conversation.
	if len(s.Conversation.Messages) == 0 && s.Session.Task != "" {
		input, err := runtime.IngestInput(l.Store, l.SessionID, s.Session.Task, "cli")
		if err != nil {
			return RunResult{}, err
		}
		ds.lastID = input.ID
		if ds.s, err = state.Apply(ds.s, input); err != nil {
			return RunResult{}, err
		}
	}

	// In-doubt (2.15): Started without a terminal event means the effect
	// may or may not have happened. Idempotent activities simply re-run
	// (decide() reaches them again); anything else NEVER re-runs — the
	// session SELF-HEALS (决策 #29, 单一自愈): the doubt renders as an
	// interrupted-by-crash result / a child settles from its own fold, and
	// the session continues. 3.2's adjudication window (side-effecting
	// gates entered, no EffectResolved) stays human — hooks may have
	// half-run.
	inDoubt := collectInDoubt(s)
	pendingEffects := collectPendingSideEffecting(s)
	if len(pendingEffects) > 0 {
		return RunResult{}, &InDoubtError{Activities: inDoubt, Effects: pendingEffects}
	}
	if len(inDoubt) > 0 {
		if err := l.settleCrashInDoubt(appendE, inDoubt); err != nil {
			return RunResult{}, err
		}
	}

	// Mailbox replay (v2 收口, 铁律 "崩溃不丢输入"): every delivery the
	// daemon acked durably but the journal never consumed (crash between
	// enqueue and the consume-side journal) re-enters now, in order,
	// journal-inputs-first. The seq high-water mark makes this effectively
	// once — an input both journaled and in the mailbox replays never.
	pending, perr := store.ReadInbox(l.Store.Dir(), ds.s.Session.ConsumedInputSeq)
	if perr != nil {
		return RunResult{}, fmt.Errorf("resume: mailbox: %w", perr)
	}
	for _, in := range pending {
		if err := l.journalInput(ds, appendE, in); err != nil {
			return RunResult{}, err
		}
	}

	// MCP re-connect reconciliation (S5.1): the journaled schemas are the
	// run's tool face; the live face must still honor them. Drift is
	// refused, never silently absorbed (2.13 version discipline).
	if err := l.reconcileMCP(ctx, ds.s); err != nil {
		return RunResult{}, err
	}

	// Timer sweep: expired pending timers fire now; future ones belong to
	// in-flight activities, which re-arm on their re-run.
	if _, err := FirePendingTimers(ds.s, l.Clock, appendE); err != nil {
		return RunResult{}, err
	}

	// A crash between session_started and the materialize completion leaves the
	// inputs unwritten — re-run (idempotent: same refs, same bytes).
	if err := l.materializeInputs(ctx, ds, appendE); err != nil {
		return RunResult{}, err
	}

	return l.drive(ctx, ds, appendE)
}

// reconcileMCP verifies every journaled MCP tool still exists live with the
// same class and schema. Extra live tools are ignored (the journaled face is
// this run's truth); a missing or drifted tool refuses the resume.
func (l *Loop) reconcileMCP(ctx context.Context, s state.State) error {
	if len(s.Session.MCPTools) == 0 {
		return nil
	}
	if l.MCP == nil {
		return fmt.Errorf("resume: session uses MCP tools but no MCP servers are connected")
	}
	l.MCP.SetAllowed(l.Spec.AllowedTools)
	live, err := l.MCP.Discover(ctx)
	if err != nil {
		return fmt.Errorf("resume: mcp discovery: %w", err)
	}
	byName := make(map[string]mcp.DiscoveredTool, len(live))
	for _, t := range live {
		byName[t.Name] = t
	}
	for _, want := range s.Session.MCPTools {
		got, ok := byName[want.Name]
		if !ok {
			return fmt.Errorf("resume: mcp tool %q journaled but not offered by the live server", want.Name)
		}
		if got.Class != want.Class {
			return fmt.Errorf("resume: mcp tool %q class drifted (%s → %s)", want.Name, want.Class, got.Class)
		}
		if compact(want.InputSchema) != compact(got.InputSchema) {
			return fmt.Errorf("resume: mcp tool %q schema drifted from the journaled face", want.Name)
		}
	}
	return nil
}

// InDoubtError reports non-idempotent activities — and side-effecting
// adjudication windows — whose outcome is unknown. The human inspects
// (agentrunner events) and decides.
type InDoubtError struct {
	Activities []event.ActivityStarted
	Effects    []event.EffectRequested
}

func (e *InDoubtError) Error() string {
	var names []string
	for _, a := range e.Activities {
		names = append(names, fmt.Sprintf("%s (%s, attempt %d)", a.ActivityID, a.Name, a.Attempt))
	}
	for _, eff := range e.Effects {
		names = append(names, fmt.Sprintf("%s (mid-adjudication, hooks may have run)", eff.EffectID))
	}
	n := len(names)
	return fmt.Sprintf("resume: %d item%s in doubt — no terminal state, refusing to re-run: %s",
		n, plural(n, " is", "s are"), strings.Join(names, ", "))
}

func collectPendingSideEffecting(s state.State) []event.EffectRequested {
	// An effect idle at an approval, or already answered, is NOT
	// in-doubt: reaching those states proves every side-effecting gate
	// (hooks) already completed (correctness #1/#3).
	idle := s.AwaitingApprovalEffect()
	var out []event.EffectRequested
	for id, eff := range s.Effects.Pending {
		if !eff.SideEffecting {
			continue
		}
		if id == idle {
			continue
		}
		if _, decided := s.Effects.Decisions[id]; decided {
			continue
		}
		out = append(out, eff)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EffectID < out[j].EffectID })
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func collectInDoubt(s state.State) []event.ActivityStarted {
	var out []event.ActivityStarted
	for _, a := range s.Activities {
		if !a.Idempotent {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ActivityID < out[j].ActivityID })
	return out
}

func checkVersions(got map[string]int) error {
	want := state.SubStateVersions()
	if len(got) != len(want) {
		return fmt.Errorf("resume: snapshot sub-state set %v does not match binary %v", got, want)
	}
	for name, v := range want {
		if got[name] != v {
			return fmt.Errorf("resume: sub-state %q version %d does not match binary version %d",
				name, got[name], v)
		}
	}
	return nil
}

// drive is the decision loop shared by Run and Resume.
func (l *Loop) drive(ctx context.Context, ds *driveState, appendE AppendFunc) (RunResult, error) {
	toolDefs, err := tool.ProviderDefs(l.Spec.Tools)
	if err != nil {
		return RunResult{}, err
	}
	// The MCP face comes from the FOLD, not the live manager: what was
	// journaled at discovery is what the run advertises — resume gets the
	// identical face without re-negotiation (S5.1).
	for _, mt := range ds.s.Session.MCPTools {
		toolDefs = append(toolDefs, provider.ToolDef{
			Name: mt.Name, Description: mt.Description, InputSchema: mt.InputSchema,
		})
	}
	// The multi-agent face (S5.3/S5.4): spawn/handoff advertise whenever
	// the spec whitelists agents; the blackboard tools whenever the run is
	// part of a collaboration (own whitelist, or an inherited board in a
	// child). The face must depend on journaled-spec-or-tree facts only —
	// resume rebuilds the same face; a missing resolver/board surfaces as a
	// model-visible error at execution instead.
	var extra []string
	if len(l.Spec.Agents) > 0 {
		extra = append(extra, "spawn_agent", "handoff_agent")
	}
	// bash can launch background tasks (S6.1) — the management tools ride
	// along so the model can inspect/cancel what it started.
	if slices.Contains(l.Spec.Tools, "bash") || len(l.Spec.Agents) > 0 {
		// bash background tasks (S6.1) and background sub-agents (v2 M3.1)
		// share kill: cancel a running child/task by its handle.
		extra = append(extra, "output", "kill")
	}
	if l.Board != nil || len(l.Spec.Agents) > 0 {
		extra = append(extra, "publish_note", "read_notes")
	}
	if len(extra) > 0 {
		// Dedup against the spec's own tools AND within extra itself: a spec
		// that already lists an auto-added tool (e.g. spawn_agent) must not
		// produce a DUPLICATE wire declaration — some providers reject it
		// (Gemini 400 "duplicate function declaration"). v2 M3 fix.
		seen := make(map[string]bool, len(l.Spec.Tools))
		for _, n := range l.Spec.Tools {
			seen[n] = true
		}
		deduped := extra[:0]
		for _, n := range extra {
			if !seen[n] {
				seen[n] = true
				deduped = append(deduped, n)
			}
		}
		if len(deduped) > 0 {
			extraDefs, derr := tool.ProviderDefs(deduped)
			if derr != nil {
				return RunResult{}, derr
			}
			toolDefs = append(toolDefs, extraDefs...)
		}
	}
	// abort is every dying execution's exit ramp. There is NO terminal
	// event (决策 #30/#31): an explicit kill leaves its SessionClosed
	// {killed, source} mark; teardown (daemon shutdown, deploy) and genuine
	// errors leave nothing — Resume re-enters the turn later, the same
	// discipline as a crash. In-flight background work is settled
	// best-effort either way so the journal never ends with orphans.
	abort := func(turn int, cause error) error {
		if src := errs.KillSource(ctx); src != "" {
			_, _ = appendE(event.TypeSessionClosed, &event.SessionClosed{
				Reason: "killed", Source: src,
				GenSteps: ds.s.Session.GenStep, Usage: ds.s.Session.Usage,
			})
		}
		l.settleOnAbort(ctx, ds, appendE)
		return cause
	}

	exec := &ActivityExecutor{Append: appendE, Clock: l.Clock, Redact: redact.FromEnv()}

	// Capability downgrade (S4.7): if the spec asks for thinking but the
	// provider can't do it, drop it — explicitly and once, never silently.
	var caps provider.Capabilities
	if l.Provider != nil {
		caps = l.Provider.Capabilities()
	}
	if l.Spec.Model.Thinking.Enabled && !caps.Thinking {
		slog.Warn("provider lacks thinking; downgrading (request will omit thinking config)",
			"provider", l.Spec.Model.Provider)
	}

	// quiesced tracks whether the current finished-turn shape already ran
	// its quiescent actions (决策 #24: outputs → barrier). Reset when a new
	// assistant message lands; a resume of an already-quiescent shape starts
	// true — the actions ran before the crash, or their loss is accepted
	// (barriers/outputs are best-effort across a crash; the parent receipt
	// is the launcher's job either way).
	quiesced, quiescedReason := state.Quiescence(ds.s)
	if quiesced {
		ds.quiesceReason = quiescedReason
	}

	for {
		if err := ctx.Err(); err != nil {
			return RunResult{}, abort(ds.s.Session.GenStep, err)
		}
		// Finished background work settles at the loop's safe point (S6.1):
		// their outcomes become user-role inputs before the next decision.
		// receipts: "steer" (default) lands them mid-turn at this boundary;
		// "turn_end" defers to the idle — the turn finishes undisturbed and
		// the settlement wakes the next one (裁决 #15).
		if l.Spec.Receipts != "turn_end" {
			if err := l.drainBackground(appendE); err != nil {
				return RunResult{}, abort(ds.s.Session.GenStep, err)
			}
		}
		// Out-of-band kills (v2 M3.2): fire any requested handle cancels here,
		// at a safe point; the cancelled child settles through bg.done as a
		// canceled outcome on a subsequent iteration.
		if err := l.drainCancels(appendE); err != nil {
			return RunResult{}, abort(ds.s.Session.GenStep, err)
		}
		act := decide(ds.s, l.Spec.MaxGenerationSteps)
		switch act.kind {
		case doTurn:
			// GenStep boundary is the compaction point (S4.5): summarize the
			// context before assembling the next turn's request, so the LLM
			// call already sees the compacted view. Runs at most once per
			// boundary — the fresh summary drops the estimate below the
			// threshold, so the next decide() no longer finds it due.
			if act.turn > 1 && compactionDue(ds.s, l.Spec) {
				if err := l.compactContext(ctx, ds, appendE, exec, act.turn); err != nil {
					return RunResult{}, abort(act.turn, err)
				}
				continue
			}
			appended, err := appendE(event.TypeGenerationStarted, &event.GenerationStarted{GenStep: act.turn})
			if err != nil {
				return RunResult{}, abort(act.turn, err)
			}
			l.emit(protocol.Event{Kind: protocol.KindGenerationStart, N: act.turn})
			// GenStep boundary: serialize the fold (2.13). The snapshot is an
			// optimization — losing it costs a longer fold, nothing else.
			if err := store.WriteSnapshot(l.Store.Dir(), appended.Seq,
				state.SubStateVersions(), ds.s); err != nil {
				return RunResult{}, abort(act.turn, err)
			}
			// GenStep boundaries are barrier points (S7.2): a workspace snapshot
			// plus the cut vector make this turn a legal fork/rewind target.
			if err := l.takeBarrier(ctx, ds, appendE,
				fmt.Sprintf("bar-t%d", act.turn), act.turn); err != nil {
				return RunResult{}, abort(act.turn, err)
			}

		case doLLM:
			if outcome, allowed, err := l.adjudicate(ctx, ds, appendE, pipeline.Effect{
				ID: fmt.Sprintf("eff-llm-t%d", act.turn), Kind: "llm_call",
				EstTokens: l.Spec.Model.MaxTokens,
				Mode:      ds.s.CurrentMode(),
				Budget:    budgetView(ds.s),
			}); err != nil {
				return RunResult{}, abort(act.turn, err)
			} else if !allowed {
				// A budget denial is a VISIBLE TRUNCATION (决策 #30), never a
				// terminal: the fact lands, the turn ends here, the session
				// idles — reopenable as ever (the gate will speak again).
				if gate := denyingGate(outcome); gate == "budget" {
					used := ds.s.Session.Usage.Billed()
					if _, err := appendE(event.TypeLimitExceeded, &event.LimitExceeded{
						Kind: "tokens", Limit: l.Spec.Budget.MaxTotalTokens, Used: used,
					}); err != nil {
						return RunResult{}, abort(act.turn, err)
					}
					slog.Warn("token budget exhausted; truncating turn", "limit", l.Spec.Budget.MaxTotalTokens, "used", used)
					l.emit(protocol.Event{Kind: protocol.KindError, N: act.turn,
						Text: "token budget exhausted; turn truncated, session is idle"})
					continue
				}
				return RunResult{}, abort(act.turn, fmt.Errorf("turn %d: llm call denied by pipeline", act.turn))
			}
			actCtx, stopInt := l.interruptScope(ctx)
			var turn provider.GenStep
			var streamed bool // any delta emitted this attempt?
			err := exec.Do(actCtx, Activity{
				ID: fmt.Sprintf("llm-t%d", act.turn), Kind: event.KindLLM,
				Name: "complete", Idempotent: true,
				DiscardOnRetry: func() error {
					// A retry after deltas were streamed: tell the surface to
					// throw away the partial stream and reopen (GenerationDiscarded).
					if streamed {
						if _, err := appendE(event.TypeGenerationDiscarded, &event.GenerationDiscarded{
							GenStep: act.turn, Reason: "llm retry after partial stream",
						}); err != nil {
							return err
						}
						l.emit(protocol.Event{Kind: protocol.KindDiscard, N: act.turn,
							Text: "partial stream discarded; retrying"})
						streamed = false
					}
					return nil
				},
				Run: func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
					req := Assemble(ds.s, l.Spec, toolDefs, act.turn)
					// Image/file parts fold as CAS refs; the wire needs
					// bytes (v2 M4.1). Inflation copies — the fold and
					// journal stay byte-free.
					if err := l.inflateBlobs(req.Messages); err != nil {
						return nil, nil, false, err
					}
					if !caps.Thinking {
						req.Thinking = provider.ThinkingConfig{}
					}
					collected, err := provider.CollectTurnStreaming(
						l.Provider.Complete(ctx, req),
						func(delta string) {
							streamed = true
							l.emit(protocol.Event{Kind: protocol.KindTextDelta, N: act.turn, Text: delta})
						})
					if err != nil {
						return nil, nil, false, err
					}
					// A TRUNCATED empty completion — no text, no tool calls, cut
					// off at the token cap — is the Gemini defect: thoughts (or a
					// tiny cap) ate the budget before any answer. Journaled as an
					// assistant_message it would poison every later assembly
					// (adapters reject part-less messages), so fail the attempt as
					// transient — the retry re-runs the call. A CLEAN empty finish
					// (the model chose to say nothing, end_turn) is legitimate and
					// ends the turn normally (S4.6). The root fix is upstream: the
					// provider disables default thinking so thoughts never starve
					// the answer.
					if len(collected.Message.Parts) == 0 && collected.Finish == provider.FinishMaxTokens {
						return nil, nil, false, errs.New(errs.ProviderServer,
							"model returned an empty message (truncated at token cap, no text or tool calls)")
					}
					turn = collected
					usage := collected.Usage
					return nil, &usage, false, nil
				},
			})
			if err != nil {
				if steered(actCtx) {
					stopInt()
					if ierr := l.onSteeringInterrupt(appendE, act.turn); ierr != nil {
						return RunResult{}, abort(act.turn, ierr)
					}
					continue // re-decide: turn N has no assistant message → re-run
				}
				stopInt()
				return RunResult{}, abort(act.turn, fmt.Errorf("turn %d: %w", act.turn, err))
			}
			stopInt()

			// Malformed tool call (S4.6): the call finished with a tool call
			// the provider could not parse. Record it, signal the surface to
			// discard the partial stream, and retry the SAME turn (no
			// assistant message is journaled, so decide() re-runs the LLM) —
			// bounded, then escalated to a user-visible error.
			if turn.Finish == provider.FinishMalformedToolCall {
				if _, err := appendE(event.TypeMalformedToolCall, &event.MalformedToolCall{
					GenStep: act.turn, Raw: assistantText(turn.Message),
					Error: "provider could not parse tool call",
				}); err != nil {
					return RunResult{}, abort(act.turn, err)
				}
				l.emit(protocol.Event{Kind: protocol.KindDiscard, N: act.turn,
					Text: "malformed tool call; retrying"})
				if ds.s.Session.MalformedRetries > maxMalformedRetries {
					// Retry budget exhausted: one visible truncation, same as
					// every step failure (统一 step 异常处理) — the turn ends,
					// the session idles, a later send retries lawfully.
					l.emit(protocol.Event{Kind: protocol.KindError, N: act.turn,
						Text: "model repeatedly returned malformed tool calls; turn truncated"})
					if _, err := appendE(event.TypeLimitExceeded, &event.LimitExceeded{
						Kind: "malformed_tool_call", Limit: maxMalformedRetries,
						Used: ds.s.Session.MalformedRetries,
					}); err != nil {
						return RunResult{}, abort(act.turn, err)
					}
				}
				continue
			}

			// Blocked/safety finish (S4.6) journals WITH the message: the
			// finish reason is the audit fact that ends this turn (the fold
			// records the truncation), the partial text is preserved, and
			// the session idles — no terminal, reopenable as ever.
			blocked := turn.Finish == provider.FinishOther || turn.Finish == provider.FinishBlocked
			finish := ""
			if blocked {
				finish = "blocked"
			}
			if _, err := appendE(event.TypeAssistantMessage, &event.AssistantMessage{
				GenStep: act.turn, Message: turn.Message, Finish: finish,
			}); err != nil {
				return RunResult{}, abort(act.turn, err)
			}
			if text := assistantText(turn.Message); text != "" {
				l.emit(protocol.Event{Kind: protocol.KindMessage, N: act.turn, Text: text})
			}
			if blocked {
				l.emit(protocol.Event{Kind: protocol.KindError, N: act.turn,
					Text: "model stopped for a safety or policy reason (blocked)"})
			}
			quiesced = false

		case doTool:
			if err := l.doTools(ctx, ds, appendE, abort, act); err != nil {
				return RunResult{}, err
			}
		case doWait:
			// A idle approval (fresh or resumed across a crash) re-enters
			// the same await path: the request payload lives in the fold's
			// Waiting.Detail. Other wait kinds have no resolver until their
			// stage (input S4, tasks/timer S6).
			if ds.s.Waiting.Kind == event.WaitApproval {
				var req event.ApprovalRequested
				if err := json.Unmarshal(ds.s.Waiting.Detail, &req); err != nil {
					return RunResult{}, abort(ds.s.Session.GenStep, fmt.Errorf("waiting_approval detail: %w", err))
				}
				if _, _, err := l.awaitApproval(ctx, ds, appendE, req); err != nil {
					return RunResult{}, abort(ds.s.Session.GenStep, err)
				}
				continue
			}
			if ds.s.Waiting.Kind == event.WaitInput {
				// A session idle for input, resumed across a restart (v2
				// M1.3): re-enter the wait WITHOUT re-journaling
				// WaitingEntered — the fact is already in the fold. But first
				// (收口 review): an input that became durable without its
				// WaitingResolved (crash in that window), or a crash receipt
				// settled by settleCrashInDoubt, is ALREADY waiting in the
				// fold — resolve the stale idle instead of blocking over it,
				// or the model never sees it until the next external poke.
				if hasInputAfterLastAssistant(ds.s) {
					if _, err := appendE(event.TypeWaitingResolved, &event.WaitingResolved{
						Kind: event.WaitInput, Resolution: "pending_input",
					}); err != nil {
						return RunResult{}, abort(ds.s.Session.GenStep, err)
					}
					continue
				}
				if res, done, err := l.idleOrReturn(ctx, ds, appendE, ds.s.Session.GenStep, &quiesced, false); done {
					return res, err
				}
				continue
			}
			return RunResult{}, fmt.Errorf("session is waiting for %s; no resolver available yet", ds.s.Waiting.Kind)

		case doIdle:
			if res, done, err := l.idleOrReturn(ctx, ds, appendE, act.turn, &quiesced, true); done {
				return res, err
			}
			continue

		case doTruncate:
			// 决策 #30: per-turn budget exhaustion is a visible truncation
			// fact, not a terminal state. LimitExceeded resets the per-turn
			// baseline in the fold; a queued input starts a fresh turn, and
			// with nothing queued the session goes idle, reopenable as ever.
			slog.Warn("turn truncated: max_generation_steps", "max_generation_steps", l.Spec.MaxGenerationSteps)
			if _, err := appendE(event.TypeLimitExceeded, &event.LimitExceeded{
				Kind: "generation_steps", Limit: l.Spec.MaxGenerationSteps, Used: act.turn - ds.s.Session.LastInputGenStep,
			}); err != nil {
				return RunResult{}, abort(act.turn, err)
			}
			l.emit(protocol.Event{Kind: protocol.KindError, Text: "turn truncated: max_generation_steps reached; session is idle"})
			continue
		}
	}
}

// idleOrReturn is the standby seam shared by a fresh idle (doIdle) and a
// resumed one (doWait/WaitInput). At a QUIESCENT shape (nothing in flight,
// no pending timer) it first runs the fixed quiescent actions ONCE per
// finished turn (决策 #24: outputs → barrier; the parent receipt is posted
// by the launcher from drive's return). Then: with a live input source it
// idles (journaling WaitingEntered when fresh); without one drive RETURNS —
// standby lives in the journal, a later send/resume continues the session.
func (l *Loop) idleOrReturn(ctx context.Context, ds *driveState, appendE AppendFunc,
	turn int, quiesced *bool, fresh bool) (RunResult, bool, error) {

	if len(ds.s.Handles) == 0 && len(ds.s.Timers) == 0 {
		if !*quiesced {
			reason := quiesceReason(ds.s)
			if err := l.quiescentActions(ctx, ds, appendE, &reason); err != nil {
				return RunResult{}, true, err
			}
			*quiesced = true
			ds.quiesceReason = reason
		}
		if l.UserInputs == nil && !l.inboxClosed {
			// No live input source ever wired: standby lives in the journal.
			// (A CLOSED input channel is the close GESTURE instead — it
			// resolves through idleForInput into the close mark below.)
			res := RunResult{Reason: ds.quiesceReason, GenSteps: ds.s.Session.GenStep, Usage: ds.s.Session.Usage}
			l.emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: res.Reason, N: res.GenSteps})
			return res, true, nil
		}
	}
	if fresh {
		if _, err := appendE(event.TypeWaitingEntered, &event.WaitingEntered{
			Kind: event.WaitInput,
		}); err != nil {
			return RunResult{}, true, err
		}
	}
	return l.idleForInput(ctx, ds, appendE, turn)
}

// quiesceReason names the finishing shape for observers (RunResult / the
// parent receipt). The fold's Quiescence answers except on the abnormal
// paths it cannot see mid-flight; default is "completed".
func quiesceReason(s state.State) string {
	if q, r := state.Quiescence(s); q && r != "" {
		return r
	}
	return "completed"
}

// doTools runs one assistant turn's tool calls (S4.3). It is two-phase:
//
//  1. Adjudicate every call SERIALLY — asks idle inline on the resolver, so
//     a turn's multiple asks are approved one at a time (no multi-prompt
//     race), and each allow's budget reservation is folded before the next
//     adjudication reads the budget (reserve-then-settle stays correct under
//     the fold, no TOCTOU — this is why adjudication is not parallelized).
//  2. Execute the allow-verdict calls CONCURRENTLY. The fold is single-
//     threaded, so every concurrent journal write funnels through one
//     mutex-serialized appendE (the S4.3 core invariant); terminal events
//     therefore land in arrival order, and assembly reorders results by the
//     assistant message's call_id sequence. One interruptScope covers the
//     whole batch.
//
// Returns nil to continue the drive loop, or the (already-epilogued) abort
// error to stop it.
func (l *Loop) doTools(ctx context.Context, ds *driveState, appendE AppendFunc,
	abort func(int, error) error, act action) error {

	// Phase 1 — serial adjudication.
	type pending struct {
		call      provider.ToolCall
		res       *tool.Result
		allowance int // spawn only: the frozen min-aggregated child budget
	}
	var allowed []pending
	// batchSpawns counts spawns allowed THIS batch: SpawnRequested only
	// lands at execution, so the gate would otherwise see a stale count
	// for the second spawn of one turn (fan-out TOCTOU). handoffAllowed
	// makes control transfer EXCLUSIVE: once a handoff passes, no further
	// agent launch in the same turn may run (S5 review — two successors
	// would otherwise execute concurrently).
	batchSpawns := 0
	handoffAllowed := false
	for _, call := range act.calls {
		l.emit(protocol.Event{Kind: protocol.KindToolCall, N: act.turn,
			Tool: call.Name, CallID: call.CallID, Args: compact(call.Args)})
		class := toolClassIn(ds.s, call.Name)
		eff := pipeline.Effect{
			ID: toolEffectID(call.CallID), Kind: "tool_call",
			ToolName: call.Name, Class: class,
			Args: call.Args, CallID: call.CallID,
			Mode:      ds.s.CurrentMode(),
			EstTokens: pipeline.EstTokensForClass(class),
			Budget:    budgetView(ds.s),
			Network:   l.networkScope(class, call.Name),
		}
		allowance := 0
		if isAgentLaunch(call.Name) {
			// Spawn and handoff both launch a child run (S5.3/S5.4): the
			// effect reserves the child's WHOLE allowance up front (min
			// aggregation) and feeds the tree caps to the spawn gate. An
			// unresolvable target keeps the class default; execution
			// reports the problem to the model.
			eff.SpawnDepth = l.Depth
			eff.SpawnCount = ds.s.Session.Spawns + batchSpawns
			eff.HandoffPending = handoffAllowed
			if _, _, childSpec, problem := l.resolveSpawnTarget(call.Name, call.Args); problem == "" {
				allowance = l.spawnAllowance(ds.s, childSpec)
				eff.EstTokens = allowance
			}
		}
		outcome, ok, err := l.adjudicate(ctx, ds, appendE, eff)
		if err != nil {
			return abort(act.turn, err)
		}
		if !ok {
			// The denial was journaled as the call's resolution (the fold
			// writes the model-visible error); nothing executes.
			dr := deniedResult(outcome)
			l.emit(protocol.Event{Kind: protocol.KindToolResult, N: act.turn,
				Tool: call.Name, CallID: call.CallID, Result: compact(dr.Payload), IsError: true})
			continue
		}
		if isAgentLaunch(call.Name) {
			batchSpawns++
			if call.Name == "handoff_agent" {
				handoffAllowed = true
			}
		}
		allowed = append(allowed, pending{call: call, res: new(tool.Result), allowance: allowance})
	}
	if len(allowed) == 0 {
		return nil
	}

	// Phase 2 — concurrent execution behind one serialized write path.
	actCtx, stopInt := l.interruptScope(ctx)
	var mu sync.Mutex
	serialAppend := func(typ string, payload any) (event.Envelope, error) {
		mu.Lock()
		defer mu.Unlock()
		return appendE(typ, payload)
	}
	execP := &ActivityExecutor{Append: serialAppend, Clock: l.Clock, Redact: redact.FromEnv()}
	errsOut := make([]error, len(allowed))
	// Activities are BUILT on this goroutine (their config reads ds.s, which
	// the concurrent serialAppend mutates) and only RUN concurrently. Spawn
	// closures journal through serialAppend and capture the parent's live
	// mode NOW — frozen-at-spawn semantics.
	parentMode := ds.s.CurrentMode()
	handlesSnapshot := ds.s.Handles
	acts := make([]Activity, 0, len(allowed))
	actIdx := make([]int, 0, len(allowed))
	for i, p := range allowed {
		// Background launches never join the batch (S6.1): the handle pairs
		// the call via the Started fold, the work outlives this turn, and
		// the terminal settles later at a drive-loop safe point. The task
		// context is the RUN's, not the batch's interrupt scope.
		if isBackgroundCall(p.call.Name, p.call.Args) {
			if err := l.launchBackground(ctx, serialAppend, p.call.CallID, p.call.Name, p.call.Args); err != nil {
				stopInt()
				return abort(act.turn, err)
			}
			if tr, ok := ds.s.Conversation.ToolResults[p.call.CallID]; ok {
				l.emit(protocol.Event{Kind: protocol.KindToolResult, N: act.turn,
					Tool: p.call.Name, CallID: p.call.CallID, Result: compact(tr.Result)})
			}
			continue
		}
		if p.call.Name == "spawn_agent" {
			// spawn is ALWAYS non-blocking (零 legacy 2026-07-05): launch
			// detached, the handle pairs the call now, the report re-enters
			// as a message later. ctx is the RUN's — a parent cancel reaches
			// the child; the batch interrupt scope does not.
			if err := l.launchBackgroundSpawn(ctx, serialAppend, p.call, p.allowance, parentMode); err != nil {
				stopInt()
				return abort(act.turn, err)
			}
			if tr, ok := ds.s.Conversation.ToolResults[p.call.CallID]; ok {
				l.emit(protocol.Event{Kind: protocol.KindToolResult, N: act.turn,
					Tool: p.call.Name, CallID: p.call.CallID, Result: compact(tr.Result)})
			}
			continue
		}
		run := l.buildToolRun(p.call, p.res)
		if p.call.Name == "handoff_agent" {
			run = l.buildHandoffRun(p.call, p.res, serialAppend, p.allowance, parentMode)
		}
		if p.call.Name == "publish_artifact" {
			run = l.buildPublishRun(p.call, p.res, serialAppend)
		}
		if isHandleTool(p.call.Name) {
			// Task tools read the fold snapshot taken NOW, on the drive
			// goroutine — the closure runs on an activity goroutine while
			// serialAppend mutates ds.s.
			call := p.call
			res := p.res
			run = func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
				*res = l.runHandleTool(handlesSnapshot, call.Name, call.Args)
				return res.Payload, nil, res.IsError, nil
			}
		}
		acts = append(acts, Activity{
			ID: "tool-" + p.call.CallID, Kind: event.KindTool,
			Name: p.call.Name, Args: p.call.Args, CallID: p.call.CallID,
			Idempotent: toolIdempotentIn(ds.s, p.call.Name),
			Timeout:    toolTimeoutIn(ds.s, p.call.Name),
			Run:        run,
			PostRun:    l.buildPostRun(p.call),
		})
		actIdx = append(actIdx, i)
	}
	var wg sync.WaitGroup
	for j := range acts {
		wg.Add(1)
		go func(j int) {
			defer wg.Done()
			errsOut[actIdx[j]] = execP.Do(actCtx, acts[j])
		}(j)
	}
	wg.Wait()
	stopInt()

	// All goroutines joined: ds.s is safe to read again. Process outcomes in
	// call order (surface ordering; the journal already holds arrival order).
	interrupted := steered(actCtx)
	for i, p := range allowed {
		err := errsOut[i]
		if err == nil {
			l.emit(protocol.Event{Kind: protocol.KindToolResult, N: act.turn,
				Tool: p.call.Name, CallID: p.call.CallID,
				Result: compact(p.res.Payload), IsError: p.res.IsError})
			continue
		}
		if interrupted {
			// A steering interrupt cancelled the whole batch: each cancelled
			// call already rendered [interrupted by user] in the fold. Emit it
			// and continue — the interrupt itself is journaled once, below.
			if tr, ok := ds.s.Conversation.ToolResults[p.call.CallID]; ok {
				l.emit(protocol.Event{Kind: protocol.KindToolResult, N: act.turn,
					Tool: p.call.Name, CallID: p.call.CallID,
					Result: compact(tr.Result), IsError: true})
			}
			continue
		}
		// A terminally-failed tool whose call resolved in the fold (rendered
		// error result) is model-visible: the loop continues and the model
		// reacts. Cancellation and harness failures still abort.
		if tr, resolved := ds.s.Conversation.ToolResults[p.call.CallID]; resolved &&
			errs.ClassOf(err) != errs.Canceled {
			l.emit(protocol.Event{Kind: protocol.KindToolResult, N: act.turn,
				Tool: p.call.Name, CallID: p.call.CallID,
				Result: compact(tr.Result), IsError: true})
			continue
		}
		return abort(act.turn, fmt.Errorf("turn %d: %s: %w", act.turn, p.call.Name, err))
	}
	if interrupted {
		if ierr := l.onSteeringInterrupt(appendE, act.turn); ierr != nil {
			return abort(act.turn, ierr)
		}
	}
	return nil
}

// buildToolRun is the per-call Run closure, writing its outcome into *res.
// exit_plan_mode is a harness-level transition: the approved mode change IS
// the effect, so it has no executor call.
func (l *Loop) buildToolRun(call provider.ToolCall, res *tool.Result) func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
	if call.Name == "exit_plan_mode" {
		return func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			*res = tool.Result{Payload: json.RawMessage(
				`{"output":"plan approved; now in default mode"}`)}
			return res.Payload, nil, false, nil
		}
	}
	// Blackboard tools (S5.4) act on the tree-shared board; a missing board
	// (e.g. a resumed run before any collaboration) is model-visible.
	if call.Name == "publish_note" || call.Name == "read_notes" {
		return func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			*res = l.runBlackboardTool(call.Name, call.Args)
			return res.Payload, nil, res.IsError, nil
		}
	}
	// mcp__ calls dispatch to the out-of-band MCP face (S5.1). A tool-level
	// IsError is a model-visible result; a transport error is an activity
	// failure (retry policy applies, final failure renders for the model).
	// With no manager connected the call falls through to the executor,
	// whose unknown-tool error is equally model-visible.
	if l.MCP != nil && strings.HasPrefix(call.Name, "mcp__") {
		return func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			payload, isErr, err := l.MCP.Call(ctx, call.Name, call.Args)
			if err != nil {
				return nil, nil, false, err
			}
			*res = tool.Result{Payload: payload, IsError: isErr}
			return res.Payload, nil, res.IsError, nil
		}
	}
	return func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
		*res = l.Exec.Execute(ctx, call.Name, call.Args)
		return res.Payload, nil, res.IsError, nil
	}
}

// buildPostRun wires post-tool hooks (3.8) for a call, or nil when none.
func (l *Loop) buildPostRun(call provider.ToolCall) func(context.Context, json.RawMessage, bool) string {
	if l.Hooks == nil || len(l.Hooks.PostTool) == 0 {
		return nil
	}
	return func(ctx context.Context, result json.RawMessage, isError bool) string {
		notes := l.Hooks.RunPost(ctx, hook.PostInput{
			ToolName: call.Name, CallID: call.CallID,
			Result: result, IsError: isError,
		})
		return strings.Join(notes, "; ")
	}
}

// action kinds for decide.
const (
	doTurn     = iota // journal GenerationStarted for action.turn
	doLLM             // run the LLM activity for action.turn
	doTool            // run action.call
	doWait            // a durable wait must resolve before anything else
	doIdle            // the turn is over: quiesce/standby for the next input
	doTruncate        // per-turn budget exhausted: journal the truncation, then re-decide
)

type action struct {
	kind int
	turn int
	// calls carries EVERY unresolved tool call of the current assistant turn
	// (S4.3): the allow-verdict ones execute concurrently. One call is the
	// common case; the slice degenerates to it without a separate path.
	calls  []provider.ToolCall
	reason string
}

// decide is THE loop policy: given only the fold state (plus the spec
// constant maxTurns, fixed for the run), what happens next. Resume
// re-enters here with the same state and therefore the same answer. There
// is only ONE session shape (决策 #31): a turn runs to its final
// generation, then the session idles; nothing here ever "ends" a session.
func decide(s state.State, maxTurns int) action {
	if s.Waiting != nil {
		return action{kind: doWait, turn: s.Session.GenStep}
	}
	turn := s.Session.GenStep
	// A truncated turn never continues by itself (决策 #30 可见截断): only
	// input that arrived after the truncation restarts it — one attempt per
	// wake, so a tokens truncation re-runs the gate when a reservation may
	// have settled, and a malformed-model session never hot-loops. The
	// queued-input generation_steps case restarts too: its baseline reset
	// granted the fresh budget.
	if turn > 0 && s.Session.TruncatedAtGenStep == turn {
		if state.TruncationRestartable(s) {
			return action{kind: doTurn, turn: turn + 1}
		}
		return action{kind: doIdle, turn: turn}
	}
	if turn == 0 {
		return action{kind: doTurn, turn: 1}
	}
	assistants := assistantMessages(s)
	if len(assistants) < turn {
		return action{kind: doLLM, turn: turn}
	}
	last := assistants[len(assistants)-1]
	calls := toolCallsOf(last)
	if len(calls) > 0 {
		var unresolved []provider.ToolCall
		for _, c := range calls {
			if _, done := s.Conversation.ToolResults[c.CallID]; !done {
				unresolved = append(unresolved, c)
			}
		}
		if len(unresolved) > 0 {
			return action{kind: doTool, turn: turn, calls: unresolved}
		}
		// A completed successful handoff finishes the turn (S5.4): control
		// moved to the successor, this agent does not act again ON ITS OWN —
		// the session idles (quiescent, reason "handoff") and only fresh
		// input would make it act again. A denied/failed handoff resolves as
		// an error result and the loop continues normally.
		handoffOK := false
		for _, c := range calls {
			if c.Name == "handoff_agent" {
				if tr, ok := s.Conversation.ToolResults[c.CallID]; ok && !tr.IsError {
					handoffOK = true
				}
			}
		}
		if handoffOK && !hasInputAfterLastAssistant(s) {
			return action{kind: doIdle, turn: turn}
		}
		// Resolved results owe the model its next generation step, bounded
		// by the per-turn budget (anti-runaway, counted from the last input
		// — a cumulative cap would silently wedge a long-lived session).
		if turn-s.Session.LastInputGenStep >= maxTurns {
			return action{kind: doTruncate, turn: turn}
		}
		return action{kind: doTurn, turn: turn + 1}
	}
	// Final generation: the turn is over. An input that already arrived
	// (journaled while the turn ran) gets its turn FIRST — with budget; only
	// a truly quiet session idles. The idle also wakes on background
	// settlements, so live background work needs no extra branch.
	if hasInputAfterLastAssistant(s) {
		if turn-s.Session.LastInputGenStep < maxTurns {
			return action{kind: doTurn, turn: turn + 1}
		}
		return action{kind: doTruncate, turn: turn}
	}
	return action{kind: doIdle, turn: turn}
}

// takeBarrier journals a CheckpointBarrier at the current cut (S7.2,
// weakened semantics): no whole-tree quiescence — the vector records this
// stream's seq, every completed child stream's final seq, and the in-flight
// background tasks with their fork-time disposition. No snapshot, no
// barrier: a barrier without a materializable workspace would promise a
// rewind it cannot deliver.
func (l *Loop) takeBarrier(ctx context.Context, ds *driveState, appendE AppendFunc,
	barrierID string, turn int) error {

	if l.Snapshots == nil {
		return nil
	}
	ref, err := l.Snapshots.Snapshot(ctx)
	if err != nil {
		if errors.Is(err, snapshot.ErrUnavailable) {
			return nil // degraded backend: run on, without barriers
		}
		slog.Warn("barrier skipped: snapshot failed", "barrier", barrierID, "err", err)
		return nil
	}
	vector := map[string]int64{".": l.Store.LastSeq()}
	for _, childSession := range ds.s.Session.ChildSessions {
		dir, rel := childDirOf(l.Store.Dir(), childSession)
		if dir == "" {
			continue
		}
		events, rerr := store.ReadEvents(dir)
		if rerr != nil || len(events) == 0 {
			continue
		}
		vector[rel] = events[len(events)-1].Seq
	}
	var handles []event.BarrierHandle
	for id := range ds.s.Handles {
		handles = append(handles, event.BarrierHandle{Handle: id, Policy: "cancel_at_fork"})
	}
	sort.Slice(handles, func(i, j int) bool { return handles[i].Handle < handles[j].Handle })
	_, err = appendE(event.TypeCheckpointBarrier, &event.CheckpointBarrier{
		BarrierID: barrierID, GenStep: turn,
		Vector: vector, SnapshotRef: ref, Handles: handles,
	})
	return err
}

// childDirOf maps a child session id back to its journal dir. Child sessions
// are "<parent>-sub-<call>-a<n>" living at sub/<call>-a<n>; anything else is
// unmappable (skipped from the vector).
func childDirOf(parentDir, childSession string) (dir, rel string) {
	idx := strings.LastIndex(childSession, "-sub-")
	if idx < 0 {
		return "", ""
	}
	suffix := childSession[idx+len("-sub-"):]
	return filepath.Join(parentDir, "sub", suffix), "sub/" + suffix
}

// marshalPermissionLayers renders the pipeline's permission gates as ordered
// rule layers (outermost first). Empty-rule gates carry only mode defaults —
// identical for every gate of the run — so they add no layer; a pipeline
// with NO rules still marshals an explicit empty array: the journaled field
// must always exist for a run that had a pipeline, so resume takes the
// frozen-layers path instead of re-merging LIVE config (S6 review: config
// drift between run and resume must never widen a rule-less run). nil only
// when there was no pipeline at all.
func marshalPermissionLayers(p *pipeline.Pipeline) json.RawMessage {
	if p == nil {
		return nil
	}
	layers := [][]pipeline.PermissionRule{}
	for _, g := range p.Gates {
		if pg, ok := g.(*pipeline.PermissionGate); ok && len(pg.Rules) > 0 {
			layers = append(layers, pg.Rules)
		}
	}
	raw, err := json.Marshal(layers)
	if err != nil {
		return nil
	}
	return raw
}

// isAgentLaunch reports whether a tool launches a child run (spawn keeps
// control and awaits; handoff transfers control and ends the caller).
func isAgentLaunch(name string) bool {
	return name == "spawn_agent" || name == "handoff_agent"
}

// ensureBoard creates the shared blackboard at a collaboration root (S5.4);
// children inherit the parent's through childLoop.
func (l *Loop) ensureBoard() {
	if l.Board == nil && len(l.Spec.Agents) > 0 {
		l.Board = blackboard.New()
		l.Board.Mirror = l.BoardMirror
	}
}

// ensureApprovals pins ONE fallback resolver for the whole tree: the
// AGENTRUNNER_APPROVE answer-sequence position is resolver state, so a
// per-ask instance would replay the first answer forever (S6 还债③).
func (l *Loop) ensureApprovals() {
	if l.Approvals == nil {
		l.Approvals = &EnvApprovals{}
	}
}

// hasInputAfterLastAssistant reports a user-role message newer than the
// model's last message — pending input the model has not seen.
func hasInputAfterLastAssistant(s state.State) bool {
	msgs := s.Conversation.Messages
	for i := len(msgs) - 1; i >= 0; i-- {
		switch msgs[i].Role {
		case provider.RoleAssistant:
			return false
		case provider.RoleUser:
			return true
		}
	}
	return false
}

func assistantMessages(s state.State) []provider.Message {
	var out []provider.Message
	for _, m := range s.Conversation.Messages {
		if m.Role == provider.RoleAssistant {
			out = append(out, m)
		}
	}
	return out
}

func toolCallsOf(m provider.Message) []provider.ToolCall {
	var out []provider.ToolCall
	for _, p := range m.Parts {
		if p.Kind == provider.PartToolCall {
			out = append(out, provider.ToolCall{CallID: p.CallID, Name: p.ToolName, Args: p.Args})
		}
	}
	return out
}

// adjudicate runs the effect through the pipeline and journals the
// resolution — allow or deny — AFTER adjudication, BEFORE execution. An
// ask verdict downgrades to deny until the 3.5 approval flow exists (the
// downgrade is itself recorded as a gate result, never silent).
func (l *Loop) adjudicate(ctx context.Context, ds *driveState, appendE AppendFunc, eff pipeline.Effect) (pipeline.Outcome, bool, error) {
	// Already resolved allow (e.g. approval granted, then crash before the
	// activity's terminal event): never re-ask, never re-journal.
	if ds.s.Effects.Allowed[eff.ID] {
		return pipeline.Outcome{Verdict: event.VerdictAllow}, true, nil
	}
	// The human already answered this approval before a crash (the decision
	// is durable from the moment ApprovalResponded was journaled): resolve
	// from the recorded answer instead of re-asking (correctness #1/#3).
	if dec, ok := ds.s.Effects.Decisions[eff.ID]; ok {
		allowed, err := l.resolveFromDecision(appendE, eff, dec)
		return pipeline.Outcome{Verdict: verdictFor(dec)}, allowed, err
	}
	if _, err := appendE(event.TypeEffectRequested, &event.EffectRequested{
		EffectID: eff.ID, CallID: eff.CallID,
		SideEffecting: l.Pipeline.SideEffecting(),
	}); err != nil {
		return pipeline.Outcome{}, false, err
	}
	outcome, err := l.Pipeline.Evaluate(ctx, eff)
	if err != nil {
		return outcome, false, err
	}
	crash.Point(crash.PointBetweenGateAndResolved)
	if outcome.Verdict == event.VerdictAsk {
		allowed, denyReason, err := l.requestApproval(ctx, ds, appendE, eff, outcome)
		if err != nil {
			return outcome, false, err
		}
		if !allowed {
			// Carry the approval's real denial reason into the outcome so the
			// model-visible tool result explains it (and, for a non-interactive
			// run, how to proceed) instead of a bare "denied by policy" (T4).
			outcome.GateResults = append(outcome.GateResults, event.GateResult{
				Gate: "approval", Decision: event.VerdictDeny, Reason: denyReason,
			})
		}
		return outcome, allowed, nil
	}
	reserved := 0
	if outcome.Verdict == event.VerdictAllow {
		reserved = eff.EstTokens
	}
	if _, err := appendE(event.TypeEffectResolved, &event.EffectResolved{
		EffectID: eff.ID, CallID: eff.CallID,
		Verdict: outcome.Verdict, GateResults: outcome.GateResults,
		ReservedTokens: reserved,
		Containment:    l.containment(eff),
	}); err != nil {
		return outcome, false, err
	}
	return outcome, outcome.Verdict == event.VerdictAllow, nil
}

// networkScope is the egress an execute-class effect would run with: "all"
// when uncontained, "" once the tree's executor is ratcheted (S7 模块 5).
// MCP tools execute in an out-of-process server the netns wrapper never
// covers — they carry "all" EVEN under the ratchet, so network rules keep
// matching them (S7 出口 review: 边界诚实 must not stop at bash).
func (l *Loop) networkScope(class, toolName string) string {
	if class != string(tool.ClassExecute) {
		return ""
	}
	if strings.HasPrefix(toolName, "mcp__") {
		return "all"
	}
	if l.Exec != nil && l.Exec.NetworkContained() {
		return ""
	}
	return "all"
}

// containment records the OS containment in force for an execute effect;
// nil for uncontained runs (absence = uncontained, the pre-S7 shape) AND
// for MCP tools — the sandbox never bounded their server process, and the
// journal must not claim it did.
func (l *Loop) containment(eff pipeline.Effect) *event.Containment {
	if eff.Kind != "tool_call" || eff.Class != string(tool.ClassExecute) {
		return nil
	}
	if strings.HasPrefix(eff.ToolName, "mcp__") {
		return nil
	}
	if l.Exec == nil || !l.Exec.NetworkContained() {
		return nil
	}
	return &event.Containment{Network: "none", Backend: "netns"}
}

// applySandbox ratchets the shared executor per this loop's spec (S7 模块
// 5); Run and Resume both pass through here, and children re-apply their
// own spec on entry — tightening only, the ratchet never widens.
func (l *Loop) applySandbox() {
	if l.Spec != nil && l.Spec.Sandbox.Network == "none" && l.Exec != nil {
		l.Exec.ContainNetwork()
	}
}

func verdictFor(decision string) string {
	if decision == "approve" {
		return event.VerdictAllow
	}
	return event.VerdictDeny
}

// resolveFromDecision journals the EffectResolved implied by a durable
// approval answer, without re-prompting. Used only on the recovery path.
func (l *Loop) resolveFromDecision(appendE AppendFunc, eff pipeline.Effect, decision string) (bool, error) {
	approved := decision == "approve"
	verdict, gate := event.VerdictDeny, event.VerdictDeny
	reason := "recovered denial"
	reserved := 0
	if approved {
		verdict, gate = event.VerdictAllow, event.VerdictAllow
		reason = "recovered approval"
		reserved = eff.EstTokens
	}
	_, err := appendE(event.TypeEffectResolved, &event.EffectResolved{
		EffectID: eff.ID, CallID: eff.CallID, Verdict: verdict,
		GateResults:    []event.GateResult{{Gate: "approval", Decision: gate, Reason: reason}},
		ReservedTokens: reserved,
		Containment:    l.containment(eff),
	})
	return approved, err
}

// budgetView snapshots the fold's accounting for the budget gate.
func budgetView(s state.State) pipeline.BudgetView {
	return pipeline.BudgetView{
		SettledTokens:  s.Session.Usage.Billed(),
		ReservedTokens: s.Budget.ReservedTotal(),
	}
}

// denyingGate names the gate that produced the deny, if any.
func denyingGate(outcome pipeline.Outcome) string {
	for _, r := range outcome.GateResults {
		if r.Decision == event.VerdictDeny {
			return r.Gate
		}
	}
	return ""
}

func deniedResult(outcome pipeline.Outcome) tool.Result {
	payload, _ := json.Marshal(map[string]string{"error": "denied by policy"})
	for _, r := range outcome.GateResults {
		if r.Decision == event.VerdictDeny {
			payload, _ = json.Marshal(map[string]string{"error": "denied: " + r.Reason})
			break
		}
	}
	return tool.Result{Payload: payload, IsError: true}
}

// toolClassIn resolves a tool's permission class from the built-in registry
// or, for mcp__ names, from the fold's journaled MCP face (S5.1). Unknown
// names return "" and every consumer fails closed on it.
func toolClassIn(s state.State, name string) string {
	if def, ok := tool.Get(name); ok {
		return string(def.Class)
	}
	if mt, ok := mcpToolIn(s, name); ok {
		return mt.Class
	}
	return ""
}

func mcpToolIn(s state.State, name string) (event.MCPToolDef, bool) {
	for _, mt := range s.Session.MCPTools {
		if mt.Name == name {
			return mt, true
		}
	}
	return event.MCPToolDef{}, false
}

// toolIdempotentIn: reads and wait-class tools re-run safely on resume;
// edits and executions do not (correctness #4). MCP read-class tools carry
// the server's ReadOnlyHint ("does not modify its environment"), so they
// re-run safely too; everything else MCP is execute and does not.
func toolIdempotentIn(s state.State, name string) bool {
	if def, ok := tool.Get(name); ok {
		return def.Class == tool.ClassRead || def.Class == tool.ClassWait
	}
	if mt, ok := mcpToolIn(s, name); ok {
		return mt.Class == string(tool.ClassRead)
	}
	return false
}

// toolEffectID namespaces tool effects away from LLM effects (eff-llm-t<n>),
// so a model-chosen call_id can never collide with an LLM effect's id.
func toolEffectID(callID string) string { return "eff-tool-" + callID }

// executeToolTimeout is the S1 default bash wall-clock limit, now owned by
// the durable-timer substrate (2.11) instead of the tool implementation.
const executeToolTimeout = 120 * time.Second

// maxMalformedRetries bounds consecutive malformed_tool_call retries on one
// turn before the run ends with a user-visible error (S4.6).
const maxMalformedRetries = 2

func toolTimeoutIn(s state.State, name string) time.Duration {
	if isAgentLaunch(name) {
		// A child run is bounded by its own max_generation_steps and budget, not a
		// wall clock — 120s would kill legitimate children (S5.3/S5.4).
		return 0
	}
	if def, ok := tool.Get(name); ok {
		if def.Class == tool.ClassExecute {
			return executeToolTimeout
		}
		return 0
	}
	if _, ok := mcpToolIn(s, name); ok {
		// Every MCP call crosses a process/network boundary; even a
		// read-class tool can hang on a stuck server, so all get the
		// execute wall clock.
		return executeToolTimeout
	}
	return 0
}

// FirePendingTimers is the resume-side timer sweep (2.13 calls it): every
// timer still pending in the fold whose fire_at has passed is fired NOW;
// future timers are returned for their owners to re-arm.
func FirePendingTimers(s state.State, clk clock.Clock, appendE AppendFunc) ([]event.TimerSet, error) {
	now := clk.Now()
	var future []event.TimerSet
	for _, tm := range s.Timers {
		if tm.FireAt.After(now) {
			future = append(future, tm)
			continue
		}
		if _, err := appendE(event.TypeTimerFired, &event.TimerFired{TimerID: tm.TimerID}); err != nil {
			return nil, err
		}
	}
	return future, nil
}

func assistantText(msg provider.Message) string {
	for _, p := range msg.Parts {
		if p.Kind == provider.PartText {
			return p.Text
		}
	}
	return ""
}
