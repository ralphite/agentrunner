package agent

import (
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
)

// Assemble builds the provider request from the fold — the single place
// fold state becomes a wire request (S4.4a). The assembly order is fixed
// and byte-stable so a cached prefix stays cached (4c): system prompt →
// mode-injected suffix (3.6b,收编) → conversation transcript. The
// advertised tool face is filtered by the live mode (3.6a).
func Assemble(s state.State, spec *AgentSpec, toolDefs []provider.ToolDef, turn int) provider.CompleteRequest {
	mode := s.CurrentMode()
	return provider.CompleteRequest{
		Model:     spec.Model.ID,
		MaxTokens: spec.Model.MaxTokens,
		System:    spec.SystemPrompt + modePromptSuffix(mode),
		Messages:  assembleMessages(s),
		Tools:     advertisedTools(toolDefs, mode),
		Turn:      turn,
	}
}

// assembleMessages builds the provider-visible transcript from the fold:
// conversation messages in order, each assistant message's fully resolved
// tool calls followed by one tool message (results by call_id). A turn
// whose tool calls are not all resolved yet is emitted without its tool
// message (the loop only reaches here once results are in). Pinned by
// testdata/request_assembly.golden.
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
