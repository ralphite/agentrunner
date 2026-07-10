package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
	"os"
)

// applyToolTurn folds one assistant message carrying a single tool call plus
// its completed result — the repeated unit microcompact reasons about.
func applyToolTurn(t *testing.T, s state.State, callID, toolName string, result json.RawMessage, isErr bool) state.State {
	t.Helper()
	asst := provider.Message{Role: provider.RoleAssistant, Parts: []provider.Part{
		{Kind: provider.PartToolCall, CallID: callID, ToolName: toolName,
			Args: json.RawMessage(`{"path":"a"}`)},
	}}
	var err error
	for _, e := range []event.Envelope{
		mustEnvOf(t, event.TypeAssistantMessage, &event.AssistantMessage{GenStep: 1, Message: asst}),
		mustEnvOf(t, event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: "tool-" + callID, Kind: event.KindTool, Name: toolName, CallID: callID}),
		mustEnvOf(t, event.TypeActivityCompleted, &event.ActivityCompleted{
			ActivityID: "tool-" + callID, Result: result, IsError: isErr}),
	} {
		if s, err = state.Apply(s, e); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// INC-13: behind the micro boundary only big, successful, read-class results
// render as the placeholder; execute-class, errors, small results, and
// everything at/after the boundary stay verbatim. Pairing is untouched.
func TestMicrocompactAssemblyView(t *testing.T) {
	big := json.RawMessage(`"` + strings.Repeat("x", 400) + `"`)
	small := json.RawMessage(`"tiny"`)

	s := state.New()
	var err error
	if s, err = state.Apply(s, mustEnvOf(t, event.TypeInputReceived,
		&event.InputReceived{Text: "hi", Source: "cli"})); err != nil { // msg 0
		t.Fatal(err)
	}
	s = applyToolTurn(t, s, "call_1_0", "read_file", big, false)   // msg 1: elidable
	s = applyToolTurn(t, s, "call_2_0", "bash", big, false)        // msg 2: execute — keep
	s = applyToolTurn(t, s, "call_3_0", "read_file", big, true)    // msg 3: error — keep
	s = applyToolTurn(t, s, "call_4_0", "read_file", small, false) // msg 4: small — keep
	s = applyToolTurn(t, s, "call_5_0", "read_file", big, false)   // msg 5: beyond boundary — keep

	if s, err = state.Apply(s, mustEnvOf(t, event.TypeContextMicrocompacted,
		&event.ContextMicrocompacted{Boundary: 5})); err != nil {
		t.Fatal(err)
	}

	results := map[string]string{}
	for _, m := range Assemble(s, assemblySpec(), nil, 2).Messages {
		if m.Role != provider.RoleTool {
			continue
		}
		for _, p := range m.Parts {
			results[p.CallID] = string(p.Result)
		}
	}
	if len(results) != 5 {
		t.Fatalf("tool results assembled = %d, want 5 (pairing intact)", len(results))
	}
	if !strings.Contains(results["call_1_0"], "cleared to save context") {
		t.Errorf("old read result not elided: %q", results["call_1_0"])
	}
	for _, id := range []string{"call_2_0", "call_3_0", "call_4_0", "call_5_0"} {
		if strings.Contains(results[id], "cleared to save context") {
			t.Errorf("%s elided but must stay verbatim", id)
		}
	}
}

// Fold discipline: the micro boundary is monotonic (max-wins) and survives a
// later compaction event untouched.
func TestMicrocompactMonotonicFold(t *testing.T) {
	s := state.New()
	var err error
	for _, b := range []int{5, 3, 7} {
		if s, err = state.Apply(s, mustEnvOf(t, event.TypeContextMicrocompacted,
			&event.ContextMicrocompacted{Boundary: b})); err != nil {
			t.Fatal(err)
		}
	}
	if s.Compaction.MicroBoundary != 7 {
		t.Fatalf("MicroBoundary = %d, want monotonic 7", s.Compaction.MicroBoundary)
	}
	if s, err = state.Apply(s, mustEnvOf(t, event.TypeContextCompacted,
		&event.ContextCompacted{UptoGenStep: 1, Summary: "sum"})); err != nil {
		t.Fatal(err)
	}
	if s.Compaction.MicroBoundary != 7 {
		t.Fatalf("MicroBoundary lost across compaction: %d", s.Compaction.MicroBoundary)
	}
}

func microLoopFixture(readSteps int) scripted.Fixture {
	fix := scripted.Fixture{}
	for i := 0; i < readSteps; i++ {
		fix.Steps = append(fix.Steps, scripted.Step{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "read_file", Args: map[string]any{"path": "data.txt"}}},
			{Finish: "tool_use"},
		}})
	}
	fix.Steps = append(fix.Steps, scripted.Step{Respond: []scripted.Event{
		{Text: "done"}, {Finish: "end_turn"}}})
	return fix
}

func microLoop(t *testing.T, microAt int, fix scripted.Fixture) (*Loop, *capturingProvider, *store.EventStore) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.txt"),
		[]byte(strings.Repeat("payload line for microcompact threshold\n", 60)), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })
	cap := &capturingProvider{inner: scripted.New(fix)}
	return &Loop{
		Spec: &AgentSpec{
			Name: "micro",
			Model: ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100,
				MicrocompactAtTokens: microAt},
			Tools:              []string{"read_file"},
			MaxGenerationSteps: 14,
		},
		Provider:  cap,
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)),
		SessionID: "micro-sess",
	}, cap, es
}

// INC-13 end-to-end: with a low threshold and a stack of bulky read results,
// the loop journals ContextMicrocompacted at a step boundary (no LLM call for
// it), later requests carry the placeholder for the oldest read result, and
// no ContextCompacted ever fires (compaction stayed unnecessary).
func TestMicrocompactTriggeredInLoop(t *testing.T) {
	l, cap, es := microLoop(t, 1000, microLoopFixture(9))
	if _, err := l.Run(context.Background(), "read it all"); err != nil {
		t.Fatal(err)
	}

	events, err := store.ReadEvents(es.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var micro *event.ContextMicrocompacted
	for _, e := range events {
		switch e.Type {
		case event.TypeContextMicrocompacted:
			dec, derr := event.DecodePayload(e)
			if derr != nil {
				t.Fatal(derr)
			}
			micro = dec.(*event.ContextMicrocompacted)
		case event.TypeContextCompacted:
			t.Fatal("ContextCompacted journaled — micro was supposed to make it unnecessary")
		}
	}
	if micro == nil {
		t.Fatal("expected a ContextMicrocompacted event")
	}
	if micro.Boundary <= 0 || micro.Cleared <= 0 {
		t.Fatalf("micro event = %+v, want positive boundary and cleared count", micro)
	}

	requests := cap.Requests()
	last := requests[len(requests)-1]
	var placeholders, verbatim int
	for _, m := range last.Messages {
		for _, p := range m.Parts {
			if p.Kind != provider.PartToolResult {
				continue
			}
			if strings.Contains(string(p.Result), "cleared to save context") {
				placeholders++
			} else {
				verbatim++
			}
		}
	}
	if placeholders == 0 {
		t.Errorf("final request carries no placeholder result")
	}
	if verbatim == 0 {
		t.Errorf("recent-guard window all elided — guard not honored")
	}
}

// -1 disables microcompact outright: same bulky run, no event.
func TestMicrocompactDisabledNoop(t *testing.T) {
	l, _, es := microLoop(t, -1, microLoopFixture(9))
	if _, err := l.Run(context.Background(), "read it all"); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(es.Dir())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Type == event.TypeContextMicrocompacted {
			t.Fatal("microcompact fired despite microcompact_at_tokens: -1")
		}
	}
}
