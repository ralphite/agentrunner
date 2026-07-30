package agent

import (
	"encoding/json"
	"testing"

	"github.com/ralphite/agentrunner/internal/command"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
)

func skillCallState(t *testing.T, calls ...struct {
	Name    string
	IsError bool
}) state.State {
	t.Helper()
	s := state.State{}
	s.Conversation.ToolResults = map[string]state.ToolResult{}
	for i, c := range calls {
		id := "call_" + string(rune('a'+i))
		args, _ := json.Marshal(map[string]string{"name": c.Name})
		s.Conversation.Messages = append(s.Conversation.Messages, provider.Message{
			Role: provider.RoleAssistant,
			Parts: []provider.Part{{
				Kind: provider.PartToolCall, CallID: id, ToolName: "skill", Args: args,
			}},
		})
		s.Conversation.ToolResults[id] = state.ToolResult{Result: json.RawMessage(`"body"`), IsError: c.IsError}
	}
	return s
}

// Loading a skill unlocks its frontmatter tools for the session; failed
// loads unlock nothing; unknown tool names in a frontmatter are dropped.
func TestSkillGatedToolUnlock(t *testing.T) {
	type call = struct {
		Name    string
		IsError bool
	}
	l := &Loop{}

	// No loads → no unlocks, and the base face is returned untouched.
	base := []provider.ToolDef{{Name: "read_file"}}
	if got := l.effectiveToolDefs(skillCallState(t), base); len(got) != 1 {
		t.Fatalf("no loads must keep the base face: %v", got)
	}

	// A successful create-agent load unlocks save_agent (shipped frontmatter).
	s := skillCallState(t, call{Name: "create-agent"})
	names := map[string]bool{}
	for _, d := range l.effectiveToolDefs(s, base) {
		names[d.Name] = true
	}
	if !names["save_agent"] || !names["read_file"] {
		t.Fatalf("create-agent load must unlock save_agent: %v", names)
	}
	if names["tool_config"] {
		t.Fatal("tool_config must stay locked until create-tool is loaded")
	}

	// Gate widens in step.
	l2 := &Loop{advertisedTools: map[string]bool{"read_file": true}}
	_ = l2.effectiveToolDefs(s, base)
	if !l2.advertisedTools["save_agent"] {
		t.Fatal("allowlist gate must admit the unlocked tool")
	}

	// A FAILED load unlocks nothing.
	sFail := skillCallState(t, call{Name: "create-tool", IsError: true})
	for _, d := range l.effectiveToolDefs(sFail, base) {
		if d.Name == "tool_config" {
			t.Fatal("failed load must not unlock")
		}
	}

	// The slash-expansion path unlocks too: a user message carrying the
	// "Loaded skill" header counts as a load (command.SkillLoadHeader).
	sSlash := state.State{}
	sSlash.Conversation.Messages = []provider.Message{{
		Role:  provider.RoleUser,
		Parts: []provider.Part{{Kind: provider.PartText, Text: command.SkillLoadHeader("create-tool") + "body\n\nargs"}},
	}}
	names = map[string]bool{}
	for _, d := range l.effectiveToolDefs(sSlash, base) {
		names[d.Name] = true
	}
	if !names["tool_config"] {
		t.Fatal("slash-expanded load must unlock tool_config")
	}
}
