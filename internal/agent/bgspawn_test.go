package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// bgSpawnLoop wires a parent whose provider is a Router: the parent, worker
// A, and worker B each run their OWN fixture, so two parallel children are
// deterministic under real concurrency (v2 M3.0).
func bgSpawnLoop(t *testing.T, router *scripted.Router, agents []string) *Loop {
	t.Helper()
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(root + "/sess")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })
	specs := map[string]*AgentSpec{
		"worker": {
			Name:         "worker",
			Description:  "investigates a delegated topic and reports",
			Model:        ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			SystemPrompt: "you investigate",
			Tools:        []string{"read_file"},
			MaxTurns:     3,
		},
	}
	return &Loop{
		Spec: &AgentSpec{
			Name: "lead", Model: ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			SystemPrompt: "you orchestrate", Tools: []string{"read_file"},
			MaxTurns: 10, Agents: agents,
		},
		Provider:  router,
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)),
		SessionID: "lead",
		SubSpecs:  staticResolver(specs),
	}
}

// v2 M3.1 + M3.4 (C3/C4): a turn launches TWO background sub-agents; both
// pair a handle immediately (turn ends without blocking), both run in
// parallel, and each completion re-enters as a message that the parent's
// later turns consume —先回先处理 — before a final summary.
func TestBackgroundSpawnParallelAndSettle(t *testing.T) {
	router := scripted.NewRouter(
		// Parent: turn 1 spawns A and B in background, then yields; later
		// turns (woken by each child's result) acknowledge, then finish.
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "a", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "task": "investigate ALPHA", "background": true}}},
				{ToolCall: &scripted.ToolCallEvent{CallID: "b", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "task": "investigate BETA", "background": true}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "a child came back, still waiting"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "both children back: ALPHA and BETA done"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "ALPHA", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "ALPHA report: ok"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "BETA", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "BETA report: ok"}, {Finish: "end_turn"}}},
		}}},
	)
	l := bgSpawnLoop(t, router, []string{"worker"})
	// Conversational: the park between turns waits on the children's results
	// (the park selects bg.done), so each child completion wakes a turn —
	// the natural home for "launch parallel, consume results" (v2 §3). Close
	// once both children's reports are in the journal.
	inputs := make(chan string)
	l.Conversational = true
	l.UserInputs = inputs
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			evs, _ := store.ReadEvents(l.Store.Dir())
			a, b := 0, 0
			for _, e := range evs {
				if e.Type == event.TypeSubagentCompleted {
					if strings.Contains(string(e.Payload), "ALPHA") || strings.Contains(string(e.Payload), "\"a\"") {
						a++
					}
					if strings.Contains(string(e.Payload), "BETA") || strings.Contains(string(e.Payload), "\"b\"") {
						b++
					}
				}
			}
			if a >= 1 && b >= 1 {
				close(inputs)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		close(inputs)
	}()

	res, err := l.Run(context.Background(), "orchestrate ALPHA and BETA in parallel")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" {
		t.Fatalf("res = %+v, want closed", res)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var spawns, started, subCompleted, actCompleted, turns int
	spawnsBeforeSecondTurn := 0
	for _, e := range events {
		switch e.Type {
		case event.TypeSpawnRequested:
			spawns++
			if turns <= 1 {
				spawnsBeforeSecondTurn++
			}
		case event.TypeActivityStarted:
			if strings.Contains(string(e.Payload), `"background":true`) {
				started++
			}
		case event.TypeSubagentCompleted:
			subCompleted++
		case event.TypeActivityCompleted:
			if strings.Contains(string(e.Payload), `"tool-a"`) || strings.Contains(string(e.Payload), `"tool-b"`) {
				actCompleted++
			}
		case event.TypeTurnStarted:
			turns++
		}
	}
	if spawns != 2 || started != 2 {
		t.Fatalf("spawns=%d bg-started=%d, want 2/2 (both launched in parallel)", spawns, started)
	}
	if spawnsBeforeSecondTurn != 2 {
		t.Errorf("turn 1 launched %d spawns, want both 2 (non-blocking parallel)", spawnsBeforeSecondTurn)
	}
	if subCompleted != 2 || actCompleted != 2 {
		t.Fatalf("subagent_completed=%d activity_completed=%d, want 2/2", subCompleted, actCompleted)
	}

	// Both children's reports reached the model as user-role messages.
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	var sawAlpha, sawBeta bool
	for _, m := range fold.Conversation.Messages {
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "ALPHA report") {
				sawAlpha = true
			}
			if strings.Contains(p.Text, "BETA report") {
				sawBeta = true
			}
		}
	}
	if !sawAlpha || !sawBeta {
		t.Errorf("child reports reached model: alpha=%v beta=%v", sawAlpha, sawBeta)
	}
	// Tasks drained; both child journals exist under sub/.
	if len(fold.Tasks) != 0 {
		t.Errorf("tasks not drained: %+v", fold.Tasks)
	}
	for _, sub := range []string{"a-a1", "b-a1"} {
		if _, err := store.ReadEvents(l.Store.Dir() + "/sub/" + sub); err != nil {
			t.Errorf("child journal %s missing: %v", sub, err)
		}
	}
}
