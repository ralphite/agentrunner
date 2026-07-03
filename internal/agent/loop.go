package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
)

// Sink receives turn-granularity output for rendering (the CLI implements it).
// S2 is still turn-level; the streaming protocol arrives in S4.
type Sink interface {
	AssistantText(turn int, text string)
	ToolCall(turn int, call provider.ToolCall)
	ToolResult(turn int, callID string, result tool.Result)
}

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
	Sink      Sink
	SessionID string
	Version   string
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
	if inDoubt := collectInDoubt(s); len(inDoubt) > 0 {
		return RunResult{}, &InDoubtError{Activities: inDoubt}
	}

	// Timer sweep: expired pending timers fire now; future ones belong to
	// in-flight activities, which re-arm on their re-run.
	if _, err := FirePendingTimers(ds.s, l.Clock, appendE); err != nil {
		return RunResult{}, err
	}

	return l.drive(ctx, ds, appendE)
}

// InDoubtError reports non-idempotent activities whose outcome is unknown.
// S3 adds the per-tool-class resolution policy; until then the human
// inspects (agentrunner events) and decides.
type InDoubtError struct {
	Activities []event.ActivityStarted
}

func (e *InDoubtError) Error() string {
	names := make([]string, 0, len(e.Activities))
	for _, a := range e.Activities {
		names = append(names, fmt.Sprintf("%s (%s, attempt %d)", a.ActivityID, a.Name, a.Attempt))
	}
	return fmt.Sprintf("resume: %d activit%s in doubt — started but no terminal state, refusing to re-run: %s",
		len(e.Activities), plural(len(e.Activities), "y is", "ies are"), strings.Join(names, ", "))
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
			appended, err := appendE(event.TypeTurnStarted, &event.TurnStarted{Turn: act.turn})
			if err != nil {
				return RunResult{}, abort(act.turn, err)
			}
			// Turn boundary: serialize the fold (2.13). The snapshot is an
			// optimization — losing it costs a longer fold, nothing else.
			if err := store.WriteSnapshot(l.Store.Dir(), appended.Seq,
				state.SubStateVersions(), ds.s); err != nil {
				return RunResult{}, abort(act.turn, err)
			}

		case doLLM:
			var turn provider.Turn
			err := exec.Do(ctx, Activity{
				ID: fmt.Sprintf("llm-t%d", act.turn), Kind: event.KindLLM,
				Name: "complete", Idempotent: true,
				Run: func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
					req := provider.CompleteRequest{
						Model:     l.Spec.Model.ID,
						MaxTokens: l.Spec.Model.MaxTokens,
						System:    l.Spec.SystemPrompt,
						Messages:  assembleMessages(ds.s),
						Tools:     toolDefs,
						Turn:      act.turn,
					}
					collected, err := provider.CollectTurn(l.Provider.Complete(ctx, req))
					if err != nil {
						return nil, nil, false, err
					}
					turn = collected
					usage := collected.Usage
					return nil, &usage, false, nil
				},
			})
			if err != nil {
				return RunResult{}, abort(act.turn, fmt.Errorf("turn %d: %w", act.turn, err))
			}
			if _, err := appendE(event.TypeAssistantMessage, &event.AssistantMessage{
				Turn: act.turn, Message: turn.Message,
			}); err != nil {
				return RunResult{}, abort(act.turn, err)
			}
			if text := assistantText(turn.Message); text != "" && l.Sink != nil {
				l.Sink.AssistantText(act.turn, text)
			}

		case doTool:
			call := act.call
			if l.Sink != nil {
				l.Sink.ToolCall(act.turn, call)
			}
			var res tool.Result
			err := exec.Do(ctx, Activity{
				ID: "tool-" + call.CallID, Kind: event.KindTool,
				Name: call.Name, Args: call.Args, CallID: call.CallID,
				Idempotent: toolIdempotent(call.Name),
				Timeout:    toolTimeout(call.Name),
				Run: func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
					res = l.Exec.Execute(ctx, call.Name, call.Args)
					return res.Payload, nil, res.IsError, nil
				},
			})
			if err != nil {
				return RunResult{}, abort(act.turn, fmt.Errorf("turn %d: %s: %w", act.turn, call.Name, err))
			}
			if l.Sink != nil {
				l.Sink.ToolResult(act.turn, call.CallID, res)
			}

		case doWait:
			// Nothing in S2 produces waits mid-run; a parked session can
			// only be met here on resume. Resolution flows (approval UI,
			// interactive input) arrive in S3/S4 — refuse rather than spin.
			return RunResult{}, fmt.Errorf("session is waiting for %s; no resolver available yet", ds.s.Waiting.Kind)

		case doEnd:
			if act.reason == "max_turns" {
				slog.Warn("run hit max_turns", "max_turns", l.Spec.MaxTurns)
			}
			return runEpilogue(ctx, ds, appendE, act.reason, act.turn, false)
		}
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
	kind   int
	turn   int
	call   provider.ToolCall
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
	for _, c := range calls {
		if _, done := s.Conversation.ToolResults[c.CallID]; !done {
			return action{kind: doTool, turn: turn, call: c}
		}
	}
	if turn >= maxTurns {
		return action{kind: doEnd, turn: turn, reason: "max_turns"}
	}
	return action{kind: doTurn, turn: turn + 1}
}

// assembleMessages builds the provider-visible transcript from the fold:
// conversation messages in order, with each assistant message's fully
// resolved tool calls followed by one tool message (results by call_id).
// Pinned by testdata/request_assembly.golden.
func assembleMessages(s state.State) []provider.Message {
	var out []provider.Message
	for _, m := range s.Conversation.Messages {
		out = append(out, m)
		if m.Role != provider.RoleAssistant {
			continue
		}
		calls := toolCallsOf(m)
		if len(calls) == 0 {
			continue
		}
		toolMsg := provider.Message{Role: provider.RoleTool}
		complete := true
		for _, c := range calls {
			res, ok := s.Conversation.ToolResults[c.CallID]
			if !ok {
				complete = false
				break
			}
			toolMsg.Parts = append(toolMsg.Parts, provider.Part{
				Kind:     provider.PartToolResult,
				CallID:   c.CallID,
				ToolName: c.Name,
				Result:   res.Result,
				IsError:  res.IsError,
			})
		}
		if complete {
			out = append(out, toolMsg)
		}
	}
	return out
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

// toolIdempotent is the S2 placeholder policy: reads re-run safely on
// resume; edits and executions do not. S3 refines this per tool class.
func toolIdempotent(name string) bool {
	def, ok := tool.Get(name)
	return ok && def.Class == tool.ClassRead
}

// executeToolTimeout is the S1 default bash wall-clock limit, now owned by
// the durable-timer substrate (2.11) instead of the tool implementation.
const executeToolTimeout = 120 * time.Second

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
