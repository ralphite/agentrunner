package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
)

func assemblySpec() *AgentSpec {
	return &AgentSpec{
		Name: "a", Model: ModelSpec{ID: "m", MaxTokens: 100},
		SystemPrompt: "be helpful",
	}
}

// Fixed assembly order: system + mode suffix, filtered face, transcript
// with tool results by call_id. This is the byte-stable prefix (4c).
func TestAssembleOrderAndFace(t *testing.T) {
	defs := []provider.ToolDef{{Name: "read_file"}, {Name: "edit_file"}, {Name: "exit_plan_mode"}}

	// Default mode: full prompt, full face.
	s := state.New()
	req := Assemble(s, assemblySpec(), defs, 1)
	if req.System != "be helpful" {
		t.Errorf("default system = %q", req.System)
	}
	if len(req.Tools) != 3 {
		t.Errorf("default face = %d tools, want 3", len(req.Tools))
	}

	// Plan mode: suffix injected, edit filtered out.
	var err error
	if s, err = state.Apply(s, mustEnvOf(t, event.TypeModeChanged,
		&event.ModeChanged{To: pipeline.ModePlan, Cause: "startup"})); err != nil {
		t.Fatal(err)
	}
	req = Assemble(s, assemblySpec(), defs, 1)
	if !strings.Contains(req.System, "PLAN MODE") {
		t.Errorf("plan system missing suffix: %q", req.System)
	}
	for _, td := range req.Tools {
		if td.Name == "edit_file" {
			t.Errorf("plan mode advertised edit_file")
		}
	}
}

// A resolved tool call becomes a tool message right after its assistant
// message, keyed by call_id.
func TestAssembleToolResults(t *testing.T) {
	asst := provider.Message{Role: provider.RoleAssistant, Parts: []provider.Part{
		{Kind: provider.PartToolCall, CallID: "call_1_0", ToolName: "read_file",
			Args: json.RawMessage(`{"path":"a"}`)},
	}}
	s := state.New()
	var err error
	for _, e := range []event.Envelope{
		mustEnvOf(t, event.TypeInputReceived, &event.InputReceived{Text: "hi", Source: "cli"}),
		mustEnvOf(t, event.TypeAssistantMessage, &event.AssistantMessage{GenStep: 1, Message: asst}),
		mustEnvOf(t, event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: "tool-call_1_0", Kind: event.KindTool, Name: "read_file", CallID: "call_1_0"}),
		mustEnvOf(t, event.TypeActivityCompleted, &event.ActivityCompleted{
			ActivityID: "tool-call_1_0", Result: json.RawMessage(`{"content":"x"}`)}),
	} {
		if s, err = state.Apply(s, e); err != nil {
			t.Fatal(err)
		}
	}
	msgs := Assemble(s, assemblySpec(), nil, 2).Messages
	if len(msgs) != 3 {
		t.Fatalf("messages = %d, want user/assistant/tool", len(msgs))
	}
	if msgs[2].Role != provider.RoleTool || msgs[2].Parts[0].CallID != "call_1_0" {
		t.Errorf("tool message = %+v", msgs[2])
	}
}
