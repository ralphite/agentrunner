package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
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
			Name:               "worker",
			Description:        "investigates a delegated topic and reports",
			Model:              ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			SystemPrompt:       "you investigate",
			Tools:              []string{"read_file"},
			MaxGenerationSteps: 3,
		},
	}
	return &Loop{
		Spec: &AgentSpec{
			Name: "lead", Model: ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			SystemPrompt: "you orchestrate", Tools: []string{"read_file"},
			MaxGenerationSteps: 10, Agents: agents,
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
			// Spare acks: settlement timing (which child wakes which turn) is
			// real-concurrency, so the parent may turn once more before close.
			{Respond: []scripted.Event{{Text: "still here"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "still here"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "ALPHA", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "ALPHA report: ok"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "BETA", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "BETA report: ok"}, {Finish: "end_turn"}}},
		}}},
	)
	l := bgSpawnLoop(t, router, []string{"worker"})
	// Conversational: the idle between turns waits on the children's results
	// (the idle selects bg.done), so each child completion wakes a turn —
	// the natural home for "launch parallel, consume results" (v2 §3). Close
	// once both children's reports are in the journal.
	inputs := make(chan protocol.UserInput)
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
		case event.TypeGenerationStarted:
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

// v2 M3.2 (C5): a user's out-of-band kill cancels a running sub-agent by
// handle; the cancelled child settles as a canceled outcome (partial output
// preserved) that the parent's next turn sees, and the other child is
// unaffected.
func TestBackgroundSpawnUserKill(t *testing.T) {
	// worker BLOCKS (a bash sleep) so the kill lands mid-run; the survivor
	// finishes normally.
	router := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "slow", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "task": "run SLOWJOB", "background": true}}},
				{ToolCall: &scripted.ToolCallEvent{CallID: "fast", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "task": "run FASTJOB", "background": true}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "a result arrived"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "another result"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "still here"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "both settled"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "SLOWJOB", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "bash",
					Args: map[string]any{"command": "sleep 30; echo SLOW_DONE"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "slow done"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "FASTJOB", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "FAST report: ok"}, {Finish: "end_turn"}}},
		}}},
	)
	l := bgSpawnLoop(t, router, []string{"worker"})
	l.Spec.Tools = []string{"read_file", "bash"} // worker needs bash for the sleep
	cancels := make(chan string, 1)
	inputs := make(chan protocol.UserInput)
	l.UserInputs = inputs
	l.Cancels = cancels

	go func() {
		// Once the slow child's bash is running, kill it by handle "slow".
		deadline := time.Now().Add(6 * time.Second)
		killed := false
		for time.Now().Before(deadline) {
			evs, _ := store.ReadEvents(l.Store.Dir() + "/sub/slow-a1")
			running := false
			for _, e := range evs {
				if e.Type == event.TypeActivityStarted && strings.Contains(string(e.Payload), `"name":"bash"`) {
					running = true
				}
			}
			if running && !killed {
				cancels <- "slow"
				killed = true
			}
			// Close once both children have settled (SubagentCompleted x2).
			pevs, _ := store.ReadEvents(l.Store.Dir())
			done := 0
			for _, e := range pevs {
				if e.Type == event.TypeSubagentCompleted {
					done++
				}
			}
			if done >= 2 {
				close(inputs)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		close(inputs)
	}()

	res, err := l.Run(context.Background(), "orchestrate SLOWJOB and FASTJOB, kill the slow one")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" {
		t.Fatalf("res = %+v", res)
	}

	events, _ := store.ReadEvents(l.Store.Dir())
	var slowReason, fastReason string
	for _, e := range events {
		if e.Type == event.TypeSubagentCompleted {
			dec, _ := event.DecodePayload(e)
			sc := dec.(*event.SubagentCompleted)
			if sc.CallID == "slow" {
				slowReason = sc.Reason
			}
			if sc.CallID == "fast" {
				fastReason = sc.Reason
			}
		}
	}
	// The killed child settled as canceled/error; the survivor completed.
	if slowReason == "" || slowReason == "completed" {
		t.Errorf("slow child reason = %q, want a cancellation (not completed)", slowReason)
	}
	if fastReason != "completed" {
		t.Errorf("fast child reason = %q, want completed (unaffected by the kill)", fastReason)
	}
	// No SLOW_DONE anywhere: the killed process never finished its sleep.
	for _, e := range events {
		if strings.Contains(string(e.Payload), "SLOW_DONE") {
			t.Error("killed child's command still completed")
		}
	}
	// The kill has a durable origin (M3 review, journal-inputs-first): an
	// InputReceived{control} fact — which must NOT surface as a user message.
	kills := 0
	for _, e := range events {
		if e.Type == event.TypeInputReceived && strings.Contains(string(e.Payload), `"source":"control"`) {
			kills++
		}
	}
	if kills != 1 {
		t.Errorf("control inputs = %d, want exactly 1 (the user kill)", kills)
	}
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range fold.Conversation.Messages {
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "[kill ") {
				t.Error("control input leaked into the conversation as a user message")
			}
		}
	}
}

// v2 M3 security review (P2, defense-in-depth): a provider-issued CallID
// carrying path syntax must not steer the child journal directory outside
// <session>/sub/ — the spawn resolves as a model-visible error instead.
func TestSpawnMalformedCallID(t *testing.T) {
	router := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "../evil", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "task": "investigate ALPHA", "background": true}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "saw the error, stopping"}, {Finish: "end_turn"}}},
		}}},
	)
	l := bgSpawnLoop(t, router, []string{"worker"})
	res, err := l.Run(context.Background(), "orchestrate")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	var sawError bool
	for _, e := range events {
		if e.Type == event.TypeActivityCompleted && strings.Contains(string(e.Payload), "malformed call id") {
			sawError = true
		}
	}
	if !sawError {
		t.Error("malformed CallID did not resolve as a model-visible error")
	}
	// The escape target must not exist: Join(dir, "sub", "../evil-a1")
	// would have landed at <session>/evil-a1.
	if _, err := os.Stat(filepath.Join(l.Store.Dir(), "evil-a1")); err == nil {
		t.Error("malformed CallID escaped sub/: child dir created outside")
	}
	if _, err := os.Stat(filepath.Join(l.Store.Dir(), "sub")); err == nil {
		entries, _ := os.ReadDir(filepath.Join(l.Store.Dir(), "sub"))
		if len(entries) > 0 {
			t.Errorf("malformed spawn still created a child dir: %v", entries)
		}
	}
}

// v2 M3 fix: a spec that explicitly lists an auto-added tool (spawn_agent)
// must NOT produce a duplicate wire declaration — some providers reject it.
func TestNoDuplicateToolDeclaration(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}
	cap := &capturingProvider{inner: scripted.New(fix)}
	l := bgSpawnLoop(t, scripted.NewRouter(), []string{"worker"})
	// Spec lists spawn_agent explicitly AND has agents (which auto-adds it).
	l.Spec.Tools = []string{"read_file", "spawn_agent"}
	l.Provider = cap

	if _, err := l.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, td := range cap.requests[0].Tools {
		seen[td.Name]++
	}
	for name, n := range seen {
		if n > 1 {
			t.Errorf("tool %q advertised %d times, want exactly 1", name, n)
		}
	}
	if seen["spawn_agent"] != 1 {
		t.Errorf("spawn_agent advertised %d times, want exactly 1", seen["spawn_agent"])
	}
}

// v2 M3.2 (C6): a steer message makes the MODEL change the orchestration —
// it cancels one running child (task_kill) and spawns a new one. Scripted
// twin (the real-API model-reliability of this is best-effort in QA).
func TestSteerChangesOrchestration(t *testing.T) {
	router := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			// GenStep 1: launch OLD (slow).
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "old", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "task": "investigate OLDTOPIC", "background": true}}},
				{Finish: "tool_use"},
			}},
			// GenStep 2 (woken by the steer): cancel OLD, spawn NEW. (No Expect:
			// assembly may order the spawn handle tool-result after the steer
			// user message; the structural asserts below prove causation.)
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "k", Name: "task_kill",
					Args: map[string]any{"task_id": "old"}}},
				{ToolCall: &scripted.ToolCallEvent{CallID: "new", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "task": "investigate NEWTOPIC", "background": true}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "reoriented"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "new done"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "still here"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "OLDTOPIC", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "s", Name: "bash",
					Args: map[string]any{"command": "sleep 30; echo OLD_DONE"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "old done"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "NEWTOPIC", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "NEW report: ok"}, {Finish: "end_turn"}}},
		}}},
	)
	l := bgSpawnLoop(t, router, []string{"worker"})
	l.Spec.Tools = []string{"read_file", "bash"}
	inputs := make(chan protocol.UserInput, 1)
	l.UserInputs = inputs
	go func() {
		// Once OLD's bash is running, steer to change course.
		deadline := time.Now().Add(6 * time.Second)
		steered := false
		for time.Now().Before(deadline) {
			evs, _ := store.ReadEvents(l.Store.Dir() + "/sub/old-a1")
			for _, e := range evs {
				if e.Type == event.TypeActivityStarted && strings.Contains(string(e.Payload), `"name":"bash"`) && !steered {
					inputs <- protocol.UserInput{Text: "change course: cancel OLDTOPIC and investigate NEWTOPIC instead"}
					steered = true
				}
			}
			pevs, _ := store.ReadEvents(l.Store.Dir())
			newDone := false
			for _, e := range pevs {
				if e.Type == event.TypeSubagentCompleted && strings.Contains(string(e.Payload), "new") {
					newDone = true
				}
			}
			if newDone {
				close(inputs)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		close(inputs)
	}()

	if _, err := l.Run(context.Background(), "orchestrate OLDTOPIC"); err != nil {
		t.Fatal(err)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	var oldReason, newReason string
	newSpawnedAfterOld := false
	sawOld := false
	for _, e := range events {
		if e.Type == event.TypeSpawnRequested {
			if strings.Contains(string(e.Payload), `"old"`) {
				sawOld = true
			}
			if strings.Contains(string(e.Payload), `"new"`) && sawOld {
				newSpawnedAfterOld = true
			}
		}
		if e.Type == event.TypeSubagentCompleted {
			dec, _ := event.DecodePayload(e)
			sc := dec.(*event.SubagentCompleted)
			if sc.CallID == "old" {
				oldReason = sc.Reason
			}
			if sc.CallID == "new" {
				newReason = sc.Reason
			}
		}
	}
	if oldReason == "completed" || oldReason == "" {
		t.Errorf("OLD child reason = %q, want cancelled by the steer", oldReason)
	}
	if !newSpawnedAfterOld {
		t.Error("NEW child was not spawned after OLD (steer did not change orchestration)")
	}
	if newReason != "completed" {
		t.Errorf("NEW child reason = %q, want completed", newReason)
	}
}
