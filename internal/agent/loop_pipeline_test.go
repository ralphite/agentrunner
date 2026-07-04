package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
)

type policyGate struct {
	name  string
	check func(pipeline.Effect) pipeline.Decision
}

func (g policyGate) Name() string { return g.name }
func (g policyGate) Check(_ context.Context, eff pipeline.Effect) pipeline.Decision {
	return g.check(eff)
}

// Journal timepoint, allow path: effect_resolved{allow} lands after
// adjudication and BEFORE the activity's Started.
func TestEffectResolvedBeforeExecution(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "bash", Args: map[string]any{"command": "true"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		policyGate{name: "permission", check: func(pipeline.Effect) pipeline.Decision { return pipeline.Allow }},
	}}
	if _, err := l.Run(context.Background(), "run true"); err != nil {
		t.Fatal(err)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	resolvedAt, startedAt := -1, -1
	for i, e := range events {
		if e.Type == event.TypeEffectResolved && strings.Contains(string(e.Payload), "eff-tool-call_1_0") {
			resolvedAt = i
		}
		if e.Type == event.TypeActivityStarted && strings.Contains(string(e.Payload), "tool-call_1_0") {
			startedAt = i
		}
	}
	if resolvedAt < 0 || startedAt < 0 || resolvedAt > startedAt {
		t.Fatalf("effect_resolved at %d, activity_started at %d — resolution must precede execution", resolvedAt, startedAt)
	}
}

// Deny path: the resolution is the ONLY fact (no activity events for the
// call), the fold turns it into a model-visible error, and the loop
// continues — the model sees the denial on its next turn.
func TestDeniedEffectSkipsExecutionAndContinues(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "bash", Args: map[string]any{"command": "rm -rf /"}}},
			{Finish: "tool_use"},
		}},
		{
			Expect:  scripted.Expect{LastMessageContains: "denied: destructive command"},
			Respond: []scripted.Event{{Text: "understood"}, {Finish: "end_turn"}},
		},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		policyGate{name: "permission", check: func(eff pipeline.Effect) pipeline.Decision {
			if eff.Kind == "tool_call" && strings.Contains(string(eff.Args), "rm -rf") {
				return pipeline.Deny("destructive command")
			}
			return pipeline.Allow
		}},
	}}
	res, err := l.Run(context.Background(), "clean up")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.Turns != 2 {
		t.Fatalf("res = %+v", res)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Type == event.TypeActivityStarted && strings.Contains(string(e.Payload), "tool-call_1_0") {
			t.Fatalf("denied effect must not start an activity: %s", e.Payload)
		}
	}
}

// Ask path with the fail-closed env resolver (AGENTRUNNER_APPROVE unset):
// the ask escalates to an approval which auto-denies, recorded as an
// approval gate result — never a silent allow, never an unexplained deny.
func TestAskDowngradesToDenyUntilApprovalFlow(t *testing.T) {
	t.Setenv("AGENTRUNNER_APPROVE", "never")
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "edit_file", Args: map[string]any{
				"path": "a.txt", "old": "", "new": "x"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "ok"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		policyGate{name: "permission", check: func(eff pipeline.Effect) pipeline.Decision {
			if eff.Class == "edit" {
				return pipeline.Ask("edits need approval")
			}
			return pipeline.Allow
		}},
	}}
	if _, err := l.Run(context.Background(), "write a file"); err != nil {
		t.Fatal(err)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var resolved event.EffectResolved
	for _, e := range events {
		if e.Type == event.TypeEffectResolved && strings.Contains(string(e.Payload), "eff-tool-call_1_0") {
			if err := json.Unmarshal(e.Payload, &resolved); err != nil {
				t.Fatal(err)
			}
		}
	}
	if resolved.Verdict != event.VerdictDeny {
		t.Fatalf("resolved = %+v", resolved)
	}
	if len(resolved.GateResults) != 2 || resolved.GateResults[0].Decision != event.VerdictAsk ||
		resolved.GateResults[1].Gate != "approval" ||
		!strings.Contains(resolved.GateResults[1].Reason, "auto-denied") {
		t.Fatalf("gate results = %+v", resolved.GateResults)
	}
}

// LLM effects are adjudicated too; every turn journals a resolution.
func TestLLMEffectResolvedPerTurn(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	if _, err := l.Run(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var sawLLMResolution bool
	for _, e := range events {
		if e.Type == event.TypeEffectResolved && strings.Contains(string(e.Payload), "eff-llm-t1") {
			sawLLMResolution = true
		}
	}
	if !sawLLMResolution {
		t.Fatal("llm effect resolution missing from journal")
	}
}
