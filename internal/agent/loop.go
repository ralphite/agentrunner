package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
)

// Sink receives turn-granularity output for rendering (the CLI implements it).
// S1 is turn-level; the streaming protocol arrives in S4.
type Sink interface {
	AssistantText(turn int, text string)
	ToolCall(turn int, call provider.ToolCall)
	ToolResult(turn int, callID string, result tool.Result)
}

// Loop runs the S1 agent loop: LLM turn → execute tool calls in order →
// feed results back → repeat until the model stops or max_turns is hit.
//
// NOTE(S1): this orchestration is deliberately naive (no activities, no
// fold state). S2.10 rewrites the body onto the activity executor and
// event-sourced state; the collaborators (provider, tools, journal) keep
// their interfaces.
type Loop struct {
	Spec     *AgentSpec
	Provider provider.Provider
	Exec     *tool.Executor
	Journal  *store.Journal
	Sink     Sink
}

// RunResult summarizes a completed run.
type RunResult struct {
	Reason string // "completed" | "max_turns"
	Turns  int
	Usage  provider.Usage
}

// Run drives the loop to completion for a single task.
func (l *Loop) Run(ctx context.Context, task string) (RunResult, error) {
	toolDefs, err := tool.ProviderDefs(l.Spec.Tools)
	if err != nil {
		return RunResult{}, err
	}

	messages := []provider.Message{{
		Role:  provider.RoleUser,
		Parts: []provider.Part{{Kind: provider.PartText, Text: task}},
	}}

	var total provider.Usage
	for turn := 1; turn <= l.Spec.MaxTurns; turn++ {
		req := provider.CompleteRequest{
			Model:     l.Spec.Model.ID,
			MaxTokens: l.Spec.Model.MaxTokens,
			System:    l.Spec.SystemPrompt,
			Messages:  messages,
			Tools:     toolDefs,
			Turn:      turn,
		}

		result, err := provider.CollectTurn(l.Provider.Complete(ctx, req))
		if err != nil {
			return RunResult{}, fmt.Errorf("turn %d: %w", turn, err)
		}
		accumulate(&total, result.Usage)

		if text := assistantText(result.Message); text != "" && l.Sink != nil {
			l.Sink.AssistantText(turn, text)
		}
		if err := l.Journal.RecordAssistantMessage(turn, result.Message); err != nil {
			return RunResult{}, err
		}
		messages = append(messages, result.Message)

		// No tool calls → the model produced a final answer.
		if len(result.ToolCalls) == 0 {
			return l.finish("completed", turn, total)
		}

		// Execute each call in order; feed all results back as one tool message.
		toolMsg := provider.Message{Role: provider.RoleTool}
		for _, call := range result.ToolCalls {
			if l.Sink != nil {
				l.Sink.ToolCall(turn, call)
			}
			if err := l.Journal.RecordToolCall(turn, call); err != nil {
				return RunResult{}, err
			}

			res := l.Exec.Execute(ctx, call.Name, call.Args)
			if l.Sink != nil {
				l.Sink.ToolResult(turn, call.CallID, res)
			}
			if err := l.Journal.RecordToolResult(turn, call.CallID, call.Name, res.Payload, res.IsError); err != nil {
				return RunResult{}, err
			}

			toolMsg.Parts = append(toolMsg.Parts, provider.Part{
				Kind:     provider.PartToolResult,
				CallID:   call.CallID,
				ToolName: call.Name,
				Result:   res.Payload,
				IsError:  res.IsError,
			})
		}
		messages = append(messages, toolMsg)
	}

	slog.Warn("run hit max_turns", "max_turns", l.Spec.MaxTurns)
	return l.finish("max_turns", l.Spec.MaxTurns, total)
}

func (l *Loop) finish(reason string, turns int, usage provider.Usage) (RunResult, error) {
	if err := l.Journal.RecordRunEnd(reason, turns, usage); err != nil {
		return RunResult{}, err
	}
	return RunResult{Reason: reason, Turns: turns, Usage: usage}, nil
}

func assistantText(msg provider.Message) string {
	for _, p := range msg.Parts {
		if p.Kind == provider.PartText {
			return p.Text
		}
	}
	return ""
}

func accumulate(total *provider.Usage, u provider.Usage) {
	total.InputTokens += u.InputTokens
	total.OutputTokens += u.OutputTokens
	total.CacheReadTokens += u.CacheReadTokens
	total.CacheWriteTokens += u.CacheWriteTokens
}
