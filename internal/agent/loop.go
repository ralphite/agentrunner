package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/crash"
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

// Run drives the loop to completion for a single task.
func (l *Loop) Run(ctx context.Context, task string) (RunResult, error) {
	if l.Clock == nil {
		l.Clock = clock.Real{}
	}
	toolDefs, err := tool.ProviderDefs(l.Spec.Tools)
	if err != nil {
		return RunResult{}, err
	}

	// appendE journals one event and folds it — the single write path.
	// Causation is a linear chain: each event is caused by the previous.
	s := state.New()
	var lastID string
	appendE := func(typ string, payload any) (event.Envelope, error) {
		env, err := event.New(typ, payload)
		if err != nil {
			return env, err
		}
		env.CausationID = lastID
		env.CorrelationID = l.SessionID
		appended, err := l.Store.Append(env)
		if err != nil {
			return appended, err
		}
		lastID = appended.ID
		next, err := state.Apply(s, appended)
		if err != nil {
			return appended, err
		}
		s = next
		return appended, nil
	}
	// abort best-effort-journals a terminal event so a failed run's log is
	// distinguishable from a truncated one.
	abort := func(turn int, cause error) error {
		_, _ = appendE(event.TypeRunEnded, &event.RunEnded{
			Reason: "error", Turns: turn, Usage: s.Run.Usage,
		})
		return cause
	}

	if _, err := appendE(event.TypeRunStarted, &event.RunStarted{
		SpecName: l.Spec.Name, Model: l.Spec.Model.ID, Task: task,
		Version: l.Version, SubStateVersions: state.SubStateVersions(),
	}); err != nil {
		return RunResult{}, err
	}
	input, err := runtime.IngestInput(l.Store, l.SessionID, task, "cli")
	if err != nil {
		return RunResult{}, abort(0, err)
	}
	lastID = input.ID
	if s, err = state.Apply(s, input); err != nil {
		return RunResult{}, abort(0, err)
	}

	exec := &ActivityExecutor{Append: appendE, Clock: l.Clock, Redact: redact.FromEnv()}

	for {
		if err := ctx.Err(); err != nil {
			return RunResult{}, abort(s.Run.Turn, err)
		}
		act := decide(s, l.Spec.MaxTurns)
		switch act.kind {
		case doTurn:
			if _, err := appendE(event.TypeTurnStarted, &event.TurnStarted{Turn: act.turn}); err != nil {
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
						Messages:  assembleMessages(s),
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

		case doEnd:
			if act.reason == "max_turns" {
				slog.Warn("run hit max_turns", "max_turns", l.Spec.MaxTurns)
			}
			crash.Point(crash.PointBeforeRunEnd)
			// TODO(2.16): this becomes the run epilogue sequence
			// (quiesce → auto-publish → barrier → terminal event).
			if _, err := appendE(event.TypeRunEnded, &event.RunEnded{
				Reason: act.reason, Turns: act.turn, Usage: s.Run.Usage,
			}); err != nil {
				return RunResult{}, err
			}
			return RunResult{Reason: act.reason, Turns: act.turn, Usage: s.Run.Usage}, nil
		}
	}
}

// action kinds for decide.
const (
	doTurn = iota // journal TurnStarted for action.turn
	doLLM         // run the LLM activity for action.turn
	doTool        // run action.call
	doEnd         // journal RunEnded with action.reason
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
