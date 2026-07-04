package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/runtime"
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
	// Mode is the STARTING mode (3.6): journaled as the first ModeChanged.
	// The live mode is fold state; empty means "default".
	Mode string
	// Hooks runs post-tool hooks (3.8); pre hooks live in the pipeline's
	// hook gate. nil = no hooks.
	Hooks *hook.Runner
}

// RunResult summarizes a completed run.
type RunResult struct {
	Reason string // "completed" | "max_turns"
	Turns  int
	Usage  provider.Usage
}

// driveState is the loop's working memory: the fold state plus the tip of
// the causation chain. drive() owns it; appendE mutates it.
type driveState struct {
	s      state.State
	lastID string
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
	l.emit(protocol.Event{Kind: protocol.KindDiscard, Turn: turn, Text: "interrupted by user"})
	return nil
}

// appender builds the single write path: journal one event, fold it, and
// advance the linear causation chain. EVERY payload passes through
// credential redaction here — args/results are also redacted upstream in
// the executor, but this blanket is what keeps run_started (task, spec),
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
	if _, err := appendE(event.TypeRunStarted, &event.RunStarted{
		SpecName: l.Spec.Name, Model: l.Spec.Model.ID, Task: task,
		Version: l.Version, SubStateVersions: state.SubStateVersions(),
		Spec: specJSON, WorkspaceRoot: wsRoot,
		Env: renderEnvBlock(wsRoot, l.Clock.Now()),
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
	l.emit(protocol.Event{Kind: protocol.KindRunStart, Mode: ds.s.CurrentMode()})

	return l.drive(ctx, ds, appendE)
}

// Resume rebuilds the fold — snapshot plus event tail when a snapshot
// exists, full fold otherwise — and re-enters the same drive loop. A
// sub-state version mismatch is refused, never silently migrated.
func (l *Loop) Resume(ctx context.Context) (RunResult, error) {
	if l.Clock == nil {
		l.Clock = clock.Real{}
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
	// wrong as a snapshot load.
	if events[0].Type == event.TypeRunStarted {
		if decoded, derr := event.DecodePayload(events[0]); derr == nil {
			if started := decoded.(*event.RunStarted); len(started.SubStateVersions) > 0 {
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
	if !ok {
		if s, err = state.Fold(events); err != nil {
			return RunResult{}, err
		}
	}

	if s.Run.Status == state.StatusEnded {
		return RunResult{Reason: s.Run.Reason, Turns: s.Run.Turn, Usage: s.Run.Usage},
			fmt.Errorf("resume: session already ended (%s)", s.Run.Reason)
	}

	ds := &driveState{s: s, lastID: events[len(events)-1].ID}
	appendE := l.appender(ds)

	// A crash between run_started and input_received leaves the task
	// durable in RunStarted but never journaled as input — re-ingest it
	// rather than silently calling the model with an empty conversation.
	if len(s.Conversation.Messages) == 0 && s.Run.Task != "" {
		input, err := runtime.IngestInput(l.Store, l.SessionID, s.Run.Task, "cli")
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
	// (decide() reaches them again); anything else surfaces to the human —
	// re-running an edit or a shell command on doubt is how state diverges.
	// 3.2 extends this to the adjudication window: an effect that entered
	// SIDE-EFFECTING gates (hooks) without an EffectResolved is equally in
	// doubt; pure-gate windows re-adjudicate on their own.
	inDoubt := collectInDoubt(s)
	pendingEffects := collectPendingSideEffecting(s)
	if len(inDoubt) > 0 || len(pendingEffects) > 0 {
		return RunResult{}, &InDoubtError{Activities: inDoubt, Effects: pendingEffects}
	}

	// Timer sweep: expired pending timers fire now; future ones belong to
	// in-flight activities, which re-arm on their re-run.
	if _, err := FirePendingTimers(ds.s, l.Clock, appendE); err != nil {
		return RunResult{}, err
	}

	return l.drive(ctx, ds, appendE)
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
	// An effect parked at an approval, or already answered, is NOT
	// in-doubt: reaching those states proves every side-effecting gate
	// (hooks) already completed (correctness #1/#3).
	parked := s.AwaitingApprovalEffect()
	var out []event.EffectRequested
	for id, eff := range s.Effects.Pending {
		if !eff.SideEffecting {
			continue
		}
		if id == parked {
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
	// abort routes a dying run through the same epilogue, best-effort, so
	// a failed log is distinguishable from a truncated one. User
	// cancellation is its own reason — an interrupted run is not a failed
	// one.
	abort := func(turn int, cause error) error {
		reason := "error"
		if errs.ClassOf(cause) == errs.Canceled {
			reason = "canceled"
		}
		_, _ = runEpilogue(ctx, ds, appendE, reason, turn, true)
		return cause
	}

	exec := &ActivityExecutor{Append: appendE, Clock: l.Clock, Redact: redact.FromEnv()}

	for {
		if err := ctx.Err(); err != nil {
			return RunResult{}, abort(ds.s.Run.Turn, err)
		}
		act := decide(ds.s, l.Spec.MaxTurns)
		switch act.kind {
		case doTurn:
			// Turn boundary is the compaction point (S4.5): summarize the
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
			appended, err := appendE(event.TypeTurnStarted, &event.TurnStarted{Turn: act.turn})
			if err != nil {
				return RunResult{}, abort(act.turn, err)
			}
			l.emit(protocol.Event{Kind: protocol.KindTurnStart, Turn: act.turn})
			// Turn boundary: serialize the fold (2.13). The snapshot is an
			// optimization — losing it costs a longer fold, nothing else.
			if err := store.WriteSnapshot(l.Store.Dir(), appended.Seq,
				state.SubStateVersions(), ds.s); err != nil {
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
				// A budget denial ends the run gracefully through the
				// epilogue (3.7c) — never mid-effect, never as a crash.
				if gate := denyingGate(outcome); gate == "budget" {
					used := ds.s.Run.Usage.Billed()
					if _, err := appendE(event.TypeLimitExceeded, &event.LimitExceeded{
						Kind: "tokens", Limit: l.Spec.Budget.MaxTotalTokens, Used: used,
					}); err != nil {
						return RunResult{}, abort(act.turn, err)
					}
					slog.Warn("token budget exhausted; ending run", "limit", l.Spec.Budget.MaxTotalTokens, "used", used)
					return runEpilogue(ctx, ds, appendE, "limit_exceeded", act.turn, false)
				}
				return RunResult{}, abort(act.turn, fmt.Errorf("turn %d: llm call denied by pipeline", act.turn))
			}
			actCtx, stopInt := l.interruptScope(ctx)
			var turn provider.Turn
			var streamed bool // any delta emitted this attempt?
			err := exec.Do(actCtx, Activity{
				ID: fmt.Sprintf("llm-t%d", act.turn), Kind: event.KindLLM,
				Name: "complete", Idempotent: true,
				DiscardOnRetry: func() error {
					// A retry after deltas were streamed: tell the surface to
					// throw away the partial stream and reopen (TurnDiscarded).
					if streamed {
						if _, err := appendE(event.TypeTurnDiscarded, &event.TurnDiscarded{
							Turn: act.turn, Reason: "llm retry after partial stream",
						}); err != nil {
							return err
						}
						l.emit(protocol.Event{Kind: protocol.KindDiscard, Turn: act.turn,
							Text: "partial stream discarded; retrying"})
						streamed = false
					}
					return nil
				},
				Run: func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
					req := Assemble(ds.s, l.Spec, toolDefs, act.turn)
					collected, err := provider.CollectTurnStreaming(
						l.Provider.Complete(ctx, req),
						func(delta string) {
							streamed = true
							l.emit(protocol.Event{Kind: protocol.KindTextDelta, Turn: act.turn, Text: delta})
						})
					if err != nil {
						return nil, nil, false, err
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
					Turn: act.turn, Raw: assistantText(turn.Message),
					Error: "provider could not parse tool call",
				}); err != nil {
					return RunResult{}, abort(act.turn, err)
				}
				l.emit(protocol.Event{Kind: protocol.KindDiscard, Turn: act.turn,
					Text: "malformed tool call; retrying"})
				if ds.s.Run.MalformedRetries > maxMalformedRetries {
					l.emit(protocol.Event{Kind: protocol.KindError, Turn: act.turn,
						Text: "model repeatedly returned malformed tool calls; giving up"})
					return runEpilogue(ctx, ds, appendE, "malformed_tool_call", act.turn, false)
				}
				continue
			}

			if _, err := appendE(event.TypeAssistantMessage, &event.AssistantMessage{
				Turn: act.turn, Message: turn.Message,
			}); err != nil {
				return RunResult{}, abort(act.turn, err)
			}
			if text := assistantText(turn.Message); text != "" {
				l.emit(protocol.Event{Kind: protocol.KindMessage, Turn: act.turn, Text: text})
			}

			// Blocked/safety finish (S4.6): the assistant text (if any) is
			// preserved above, but the model stopped for a policy reason —
			// surface a user-visible error and end the run gracefully.
			if turn.Finish == provider.FinishOther || turn.Finish == provider.FinishBlocked {
				l.emit(protocol.Event{Kind: protocol.KindError, Turn: act.turn,
					Text: "model stopped for a safety or policy reason (blocked)"})
				return runEpilogue(ctx, ds, appendE, "blocked", act.turn, false)
			}

		case doTool:
			if err := l.doTools(ctx, ds, appendE, abort, act); err != nil {
				return RunResult{}, err
			}
		case doWait:
			// A parked approval (fresh or resumed across a crash) re-enters
			// the same await path: the request payload lives in the fold's
			// Waiting.Detail. Other wait kinds have no resolver until their
			// stage (input S4, tasks/timer S6).
			if ds.s.Waiting.Kind == event.WaitApproval {
				var req event.ApprovalRequested
				if err := json.Unmarshal(ds.s.Waiting.Detail, &req); err != nil {
					return RunResult{}, abort(ds.s.Run.Turn, fmt.Errorf("waiting_approval detail: %w", err))
				}
				if _, err := l.awaitApproval(ctx, ds, appendE, req); err != nil {
					return RunResult{}, abort(ds.s.Run.Turn, err)
				}
				continue
			}
			return RunResult{}, fmt.Errorf("session is waiting for %s; no resolver available yet", ds.s.Waiting.Kind)

		case doEnd:
			if act.reason == "max_turns" {
				slog.Warn("run hit max_turns", "max_turns", l.Spec.MaxTurns)
			}
			res, err := runEpilogue(ctx, ds, appendE, act.reason, act.turn, false)
			l.emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: res.Reason, Turn: res.Turns})
			return res, err
		}
	}
}

// doTools runs one assistant turn's tool calls (S4.3). It is two-phase:
//
//  1. Adjudicate every call SERIALLY — asks park inline on the resolver, so
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
		call provider.ToolCall
		res  *tool.Result
	}
	var allowed []pending
	for _, call := range act.calls {
		l.emit(protocol.Event{Kind: protocol.KindToolCall, Turn: act.turn,
			Tool: call.Name, CallID: call.CallID, Args: compact(call.Args)})
		eff := pipeline.Effect{
			ID: toolEffectID(call.CallID), Kind: "tool_call",
			ToolName: call.Name, Class: toolClass(call.Name),
			Args: call.Args, CallID: call.CallID,
			Mode:      ds.s.CurrentMode(),
			EstTokens: pipeline.EstTokensForClass(toolClass(call.Name)),
			Budget:    budgetView(ds.s),
		}
		outcome, ok, err := l.adjudicate(ctx, ds, appendE, eff)
		if err != nil {
			return abort(act.turn, err)
		}
		if !ok {
			// The denial was journaled as the call's resolution (the fold
			// writes the model-visible error); nothing executes.
			dr := deniedResult(outcome)
			l.emit(protocol.Event{Kind: protocol.KindToolResult, Turn: act.turn,
				Tool: call.Name, CallID: call.CallID, Result: compact(dr.Payload), IsError: true})
			continue
		}
		allowed = append(allowed, pending{call: call, res: new(tool.Result)})
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
	var wg sync.WaitGroup
	for i, p := range allowed {
		wg.Add(1)
		go func(i int, p pending) {
			defer wg.Done()
			errsOut[i] = execP.Do(actCtx, Activity{
				ID: "tool-" + p.call.CallID, Kind: event.KindTool,
				Name: p.call.Name, Args: p.call.Args, CallID: p.call.CallID,
				Idempotent: toolIdempotent(p.call.Name),
				Timeout:    toolTimeout(p.call.Name),
				Run:        l.buildToolRun(p.call, p.res),
				PostRun:    l.buildPostRun(p.call),
			})
		}(i, p)
	}
	wg.Wait()
	stopInt()

	// All goroutines joined: ds.s is safe to read again. Process outcomes in
	// call order (surface ordering; the journal already holds arrival order).
	interrupted := steered(actCtx)
	for i, p := range allowed {
		err := errsOut[i]
		if err == nil {
			l.emit(protocol.Event{Kind: protocol.KindToolResult, Turn: act.turn,
				Tool: p.call.Name, CallID: p.call.CallID,
				Result: compact(p.res.Payload), IsError: p.res.IsError})
			continue
		}
		if interrupted {
			// A steering interrupt cancelled the whole batch: each cancelled
			// call already rendered [interrupted by user] in the fold. Emit it
			// and continue — the interrupt itself is journaled once, below.
			if tr, ok := ds.s.Conversation.ToolResults[p.call.CallID]; ok {
				l.emit(protocol.Event{Kind: protocol.KindToolResult, Turn: act.turn,
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
			l.emit(protocol.Event{Kind: protocol.KindToolResult, Turn: act.turn,
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
	doTurn = iota // journal TurnStarted for action.turn
	doLLM         // run the LLM activity for action.turn
	doTool        // run action.call
	doEnd         // journal RunEnded with action.reason
	doWait        // parked: the wait must resolve before anything else
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

// decide is THE loop policy: given only the fold state, what happens next.
// Resume re-enters here with the same state and therefore the same answer.
func decide(s state.State, maxTurns int) action {
	if s.Waiting != nil {
		return action{kind: doWait, turn: s.Run.Turn}
	}
	turn := s.Run.Turn
	if turn == 0 {
		return action{kind: doTurn, turn: 1}
	}
	assistants := assistantMessages(s)
	if len(assistants) < turn {
		return action{kind: doLLM, turn: turn}
	}
	calls := toolCallsOf(assistants[len(assistants)-1])
	if len(calls) == 0 {
		return action{kind: doEnd, turn: turn, reason: "completed"}
	}
	var unresolved []provider.ToolCall
	for _, c := range calls {
		if _, done := s.Conversation.ToolResults[c.CallID]; !done {
			unresolved = append(unresolved, c)
		}
	}
	if len(unresolved) > 0 {
		return action{kind: doTool, turn: turn, calls: unresolved}
	}
	if turn >= maxTurns {
		return action{kind: doEnd, turn: turn, reason: "max_turns"}
	}
	return action{kind: doTurn, turn: turn + 1}
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
		allowed, err := l.requestApproval(ctx, ds, appendE, eff, outcome)
		return outcome, allowed, err
	}
	reserved := 0
	if outcome.Verdict == event.VerdictAllow {
		reserved = eff.EstTokens
	}
	if _, err := appendE(event.TypeEffectResolved, &event.EffectResolved{
		EffectID: eff.ID, CallID: eff.CallID,
		Verdict: outcome.Verdict, GateResults: outcome.GateResults,
		ReservedTokens: reserved,
	}); err != nil {
		return outcome, false, err
	}
	return outcome, outcome.Verdict == event.VerdictAllow, nil
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
	})
	return approved, err
}

// budgetView snapshots the fold's accounting for the budget gate.
func budgetView(s state.State) pipeline.BudgetView {
	return pipeline.BudgetView{
		SettledTokens:  s.Run.Usage.Billed(),
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

func toolClass(name string) string {
	if def, ok := tool.Get(name); ok {
		return string(def.Class)
	}
	return ""
}

// toolIdempotent is the S2 placeholder policy: reads re-run safely on
// resume; edits and executions do not. S3 refines this per tool class.
func toolIdempotent(name string) bool {
	def, ok := tool.Get(name)
	// Reads and wait-class tools (exit_plan_mode) re-run safely on resume;
	// edits and executions do not (correctness #4).
	return ok && (def.Class == tool.ClassRead || def.Class == tool.ClassWait)
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

func toolTimeout(name string) time.Duration {
	if def, ok := tool.Get(name); ok && def.Class == tool.ClassExecute {
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
