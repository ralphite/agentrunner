package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
)

// Assemble builds the provider request from the fold — the single place
// fold state becomes a wire request (S4.4a). The assembly order is fixed
// and byte-stable so a cached prefix stays cached (4c): frozen env block →
// spec system prompt → mode-injected suffix (3.6b,收编) → conversation
// transcript. The advertised tool face is filtered by the live mode (3.6a).
func Assemble(s state.State, spec *AgentSpec, toolDefs []provider.ToolDef, turn int) provider.CompleteRequest {
	mode := s.CurrentMode()
	return provider.CompleteRequest{
		Model:     spec.Model.ID,
		MaxTokens: spec.Model.MaxTokens,
		System:    assembleSystem(s.Run.Env, spec.SystemPrompt, mode),
		Messages:  assembleMessages(s),
		Tools:     advertisedTools(toolDefs, mode),
		Turn:      turn,
	}
}

// assembleSystem lays out the system prompt in DESIGN's fixed order: the
// frozen env block first (most-stable prefix), then the spec's own prompt,
// then the mode suffix. The env block was frozen at session start, so this
// prefix is byte-identical every turn; only the mode suffix moves, and only
// on an explicit mode transition (an accepted cache break, decision #10).
func assembleSystem(env, specPrompt, mode string) string {
	var b strings.Builder
	if env != "" {
		b.WriteString(env)
		b.WriteString("\n\n")
	}
	b.WriteString(specPrompt)
	b.WriteString(modePromptSuffix(mode))
	return b.String()
}

// renderEnvBlock freezes the environment into a stable block at session
// start (S4.4c). Only session-stable facts belong here — cwd and the date;
// per-turn-volatile state (git status) enters as appended messages instead,
// never rewriting this prefix. Git status is deferred until the workspace
// grows a git seam; cwd + date already pin the invariant DESIGN cares about.
func renderEnvBlock(cwd string, now time.Time) string {
	if cwd == "" {
		return ""
	}
	return fmt.Sprintf("<env>\ncwd: %s\ndate: %s\n</env>", cwd, now.Format("2006-01-02"))
}

// assembleMessages builds the provider-visible transcript from the fold:
// conversation messages in order, each assistant message's fully resolved
// tool calls followed by one tool message (results by call_id). A turn
// whose tool calls are not all resolved yet is emitted without its tool
// message (the loop only reaches here once results are in). Pinned by
// testdata/request_assembly.golden.
//
// Compaction (S4.5): when a boundary is set, messages[0:Boundary] are
// replaced by a single summary user message — the log keeps every message,
// but the model sees the summary plus everything after the boundary.
func assembleMessages(s state.State) []provider.Message {
	msgs := s.Conversation.Messages
	var out []provider.Message
	if b := s.Compaction.Boundary; b > 0 && b <= len(msgs) {
		out = append(out, provider.Message{Role: provider.RoleUser, Parts: []provider.Part{{
			Kind: provider.PartText,
			Text: "[conversation summary so far]\n" + s.Compaction.Summary,
		}}})
		msgs = msgs[b:]
	}
	for _, m := range msgs {
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

// advertisedTools filters the tool face by mode (3.6a): what the model
// cannot use, it does not see.
func advertisedTools(defs []provider.ToolDef, mode string) []provider.ToolDef {
	out := make([]provider.ToolDef, 0, len(defs))
	for _, d := range defs {
		if pipeline.ClassAdvertised(mode, toolClass(d.Name)) {
			out = append(out, d)
		}
	}
	return out
}
