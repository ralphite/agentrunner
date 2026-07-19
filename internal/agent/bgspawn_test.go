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
					Args: map[string]any{"agent": "worker", "prompt": "investigate ALPHA", "background": true}}},
				{ToolCall: &scripted.ToolCallEvent{CallID: "b", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "prompt": "investigate BETA", "background": true}}},
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
	// Background work drained; both child journals exist under sub/.
	if len(fold.Handles) != 0 {
		t.Errorf("background handles not drained: %+v", fold.Handles)
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
					Args: map[string]any{"agent": "worker", "prompt": "run SLOWJOB", "background": true}}},
				{ToolCall: &scripted.ToolCallEvent{CallID: "fast", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "prompt": "run FASTJOB", "background": true}}},
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
		// The deadline is give-up time, not expected latency: on a loaded
		// machine (check.sh 六腿并行) the child can take >6s to reach its
		// bash, and giving up early means the kill never fires and the
		// child settles "completed" — the exact flake this guards against.
		// Normal runs exit this loop on the both-settled check below.
		deadline := time.Now().Add(60 * time.Second)
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
		cevs, _ := store.ReadEvents(l.Store.Dir() + "/sub/slow-a1")
		for _, e := range cevs {
			t.Logf("slow child event %s: %.300s", e.Type, e.Payload)
		}
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
					Args: map[string]any{"agent": "worker", "prompt": "investigate ALPHA", "background": true}}},
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
	requests := cap.Requests()
	for _, td := range requests[0].Tools {
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
// it cancels one running child (kill) and spawns a new one. Scripted
// twin (the real-API model-reliability of this is best-effort in QA).
func TestSteerChangesOrchestration(t *testing.T) {
	router := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			// GenStep 1: launch OLD (slow).
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "old", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "prompt": "investigate OLDTOPIC", "background": true}}},
				{Finish: "tool_use"},
			}},
			// GenStep 2 (woken by the steer): cancel OLD, spawn NEW. (No Expect:
			// assembly may order the spawn handle tool-result after the steer
			// user message; the structural asserts below prove causation.)
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "k", Name: "kill",
					Args: map[string]any{"handle": "old"}}},
				{ToolCall: &scripted.ToolCallEvent{CallID: "new", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "prompt": "investigate NEWTOPIC", "background": true}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "reoriented"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "new done"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "still here"}, {Finish: "end_turn"}}},
			// Receipt boundaries are scheduling-dependent under host
			// contention: the kill receipt and NEW's completion can land in
			// one wake or two (audit-0717 F3 flake). The turn COUNT is
			// incidental — the structural asserts below are the red lines —
			// so keep two tolerant fillers for the split case.
			{Respond: []scripted.Event{{Text: "noted"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "noted"}, {Finish: "end_turn"}}},
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

// 决策 #30/裁决二: an explicit kill leaves a SOURCED mark in the child's
// journal — the parent model's kill tool marks source=parent; the user's
// out-of-band kill (ar kill → Cancels) marks source=user. Automatic paths
// check the mark; a lawful reopen clears it.
func TestKillLeavesSourcedMark(t *testing.T) {
	childFix := func() scripted.Fixture {
		return scripted.Fixture{Steps: []scripted.Step{
			// The child parks on a slow foreground bash — a wide window for
			// the kill to land while it is genuinely mid-work.
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "cz", Name: "bash",
					Args: map[string]any{"command": "sleep 5"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "never reached"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "never reached"}, {Finish: "end_turn"}}},
		}}
	}
	slowWorker := func() map[string]*AgentSpec {
		return map[string]*AgentSpec{"worker": {
			Name: "worker", Description: "runs a long job",
			Model:        ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			SystemPrompt: "you work slowly", Tools: []string{"bash"},
			MaxGenerationSteps: 3,
		}}
	}

	readMark := func(t *testing.T, dir string) *state.CloseMark {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			evs, _ := store.ReadEvents(dir)
			if s, err := state.Fold(evs); err == nil && s.Session.Closed != nil {
				return s.Session.Closed
			}
			time.Sleep(10 * time.Millisecond)
		}
		return nil
	}

	t.Run("parent kill marks source=parent", func(t *testing.T) {
		router := scripted.NewRouter(
			scripted.RoutePair{Key: "SLOW-JOB", Fixture: childFix()},
			// Once the cancellation receipt is in the transcript, every later
			// parent request lands here (routing by shape beats settle timing).
			scripted.RoutePair{Key: "canceled]", Fixture: scripted.Fixture{Steps: []scripted.Step{
				{Respond: []scripted.Event{{Text: "killed it"}, {Finish: "end_turn"}}},
			}}},
			scripted.RoutePair{Key: "", Fixture: scripted.Fixture{Steps: []scripted.Step{
				{Respond: []scripted.Event{
					{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
						Args: map[string]any{"agent": "worker", "prompt": "SLOW-JOB run"}}},
					{Finish: "tool_use"},
				}},
				{Respond: []scripted.Event{
					{ToolCall: &scripted.ToolCallEvent{CallID: "k1", Name: "kill",
						Args: map[string]any{"handle": "s1"}}},
					{Finish: "tool_use"},
				}},
				{Respond: []scripted.Event{{Text: "waiting for the cancellation"}, {Finish: "end_turn"}}},
				{Respond: []scripted.Event{{Text: "waiting for the cancellation"}, {Finish: "end_turn"}}},
			}}},
		)
		l := bgSpawnLoop(t, router, []string{"worker"})
		l.SubSpecs = staticResolver(slowWorker())
		if _, err := l.Run(context.Background(), "spawn then kill"); err != nil {
			t.Fatal(err)
		}
		mark := readMark(t, filepath.Join(l.Store.Dir(), "sub", "s1-a1"))
		if mark == nil || mark.Reason != "killed" || mark.Source != "parent" {
			t.Fatalf("child mark = %+v, want killed/parent", mark)
		}
		events, _ := store.ReadEvents(l.Store.Dir())
		for _, e := range events {
			if e.Type != event.TypeSubagentCompleted {
				continue
			}
			dec, _ := event.DecodePayload(e)
			if sc := dec.(*event.SubagentCompleted); sc.CallID == "s1" && sc.Reason != "canceled" {
				t.Fatalf("parent child receipt reason = %q, want canceled", sc.Reason)
			}
		}
	})

	t.Run("user kill marks source=user", func(t *testing.T) {
		router := scripted.NewRouter(
			scripted.RoutePair{Key: "SLOW-JOB", Fixture: childFix()},
			scripted.RoutePair{Key: "canceled]", Fixture: scripted.Fixture{Steps: []scripted.Step{
				{Respond: []scripted.Event{{Text: "user killed it"}, {Finish: "end_turn"}}},
			}}},
			scripted.RoutePair{Key: "", Fixture: scripted.Fixture{Steps: []scripted.Step{
				{Respond: []scripted.Event{
					{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
						Args: map[string]any{"agent": "worker", "prompt": "SLOW-JOB run"}}},
					{Finish: "tool_use"},
				}},
				{Respond: []scripted.Event{{Text: "waiting for the kill"}, {Finish: "end_turn"}}},
				{Respond: []scripted.Event{{Text: "waiting for the kill"}, {Finish: "end_turn"}}},
			}}},
		)
		cancels := make(chan string, 1)
		l := bgSpawnLoop(t, router, []string{"worker"})
		l.SubSpecs = staticResolver(slowWorker())
		l.Cancels = cancels
		go func() {
			// The user's ar kill: fire once the handle exists (a kill for an
			// unknown handle is a no-op by design).
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				evs, _ := store.ReadEvents(l.Store.Dir())
				for _, e := range evs {
					if e.Type == event.TypeSpawnRequested {
						cancels <- "s1"
						return
					}
				}
				time.Sleep(5 * time.Millisecond)
			}
		}()
		if _, err := l.Run(context.Background(), "spawn; the user kills"); err != nil {
			t.Fatal(err)
		}
		mark := readMark(t, filepath.Join(l.Store.Dir(), "sub", "s1-a1"))
		if mark == nil || mark.Reason != "killed" || mark.Source != "user" {
			t.Fatalf("child mark = %+v, want killed/user", mark)
		}
		events, _ := store.ReadEvents(l.Store.Dir())
		for _, e := range events {
			if e.Type != event.TypeSubagentCompleted {
				continue
			}
			dec, _ := event.DecodePayload(e)
			if sc := dec.(*event.SubagentCompleted); sc.CallID == "s1" && sc.Reason != "canceled" {
				t.Fatalf("parent child receipt reason = %q, want canceled", sc.Reason)
			}
		}
	})
}

// INC-30.2 (G25): spawn_agent.replaces retires the predecessor before the
// successor starts — the abandoned member is cancelled (parent-sourced, same
// as the kill tool) instead of running its budget out. The 44c3 incident:
// a replaced reviewer kept spinning for 70+ steps / 195k tokens because
// nothing ever cancelled it.
func TestSpawnReplacesCancelsPredecessor(t *testing.T) {
	router := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			// Turn 1: the stuck member.
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "old", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "prompt": "run STUCKJOB", "background": true}}},
				{Finish: "tool_use"},
			}},
			// Turn 2: re-assign to a fresh member, retiring the old handle.
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "new", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "prompt": "run RETRYJOB", "replaces": "old", "background": true}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "result noted"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "another result"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "wrapping"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "STUCKJOB", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "bash",
					Args: map[string]any{"command": "sleep 30; echo STUCK_DONE"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "stuck done"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "RETRYJOB", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "RETRY report: ok"}, {Finish: "end_turn"}}},
		}}},
	)
	l := bgSpawnLoop(t, router, []string{"worker"})
	l.Spec.Tools = []string{"read_file", "bash"}
	inputs := make(chan protocol.UserInput)
	l.UserInputs = inputs

	go func() {
		// Wait for the stuck member's bash to be genuinely running, then let
		// the parent's next turn (the replacing spawn) fire by feeding one
		// input; close once both children settled.
		deadline := time.Now().Add(8 * time.Second)
		nudged := false
		for time.Now().Before(deadline) {
			evs, _ := store.ReadEvents(l.Store.Dir() + "/sub/old-a1")
			running := false
			for _, e := range evs {
				if e.Type == event.TypeActivityStarted && strings.Contains(string(e.Payload), `"name":"bash"`) {
					running = true
				}
			}
			if running && !nudged {
				inputs <- protocol.UserInput{Text: "the old one is stuck, re-assign"}
				nudged = true
			}
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

	if _, err := l.Run(context.Background(), "orchestrate STUCKJOB, you may re-assign later"); err != nil {
		t.Fatal(err)
	}

	events, _ := store.ReadEvents(l.Store.Dir())
	var oldReason, newReason, journaledReplaces string
	for _, e := range events {
		switch e.Type {
		case event.TypeSubagentCompleted:
			dec, _ := event.DecodePayload(e)
			sc := dec.(*event.SubagentCompleted)
			if sc.CallID == "old" {
				oldReason = sc.Reason
			}
			if sc.CallID == "new" {
				newReason = sc.Reason
			}
		case event.TypeSpawnRequested:
			dec, _ := event.DecodePayload(e)
			sr := dec.(*event.SpawnRequested)
			if sr.CallID == "new" {
				journaledReplaces = sr.Replaces
			}
		}
	}
	if oldReason == "" || oldReason == "completed" {
		t.Errorf("replaced child reason = %q, want a cancellation (not completed)", oldReason)
	}
	if newReason != "completed" {
		t.Errorf("replacing child reason = %q, want completed", newReason)
	}
	if journaledReplaces != "old" {
		t.Errorf("SpawnRequested.Replaces = %q, want %q (audit fact)", journaledReplaces, "old")
	}
	for _, e := range events {
		if strings.Contains(string(e.Payload), "STUCK_DONE") {
			t.Error("replaced child's command still completed")
		}
	}
}

// INC-30.2: replaces pointing at a finished or never-existing handle is a
// silent no-op — the new delegation proceeds normally.
func TestSpawnReplacesUnknownHandleIsNoop(t *testing.T) {
	router := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "solo", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "prompt": "run SOLOJOB", "replaces": "ghost-handle", "background": true}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "noted"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "SOLOJOB", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "SOLO report: ok"}, {Finish: "end_turn"}}},
		}}},
	)
	l := bgSpawnLoop(t, router, []string{"worker"})
	inputs := make(chan protocol.UserInput)
	l.UserInputs = inputs
	closeWhen(l, inputs, func(events []event.Envelope) bool {
		return countType(events, event.TypeSubagentCompleted) >= 1
	})
	if _, err := l.Run(context.Background(), "orchestrate SOLOJOB"); err != nil {
		t.Fatal(err)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	for _, e := range events {
		if e.Type != event.TypeSubagentCompleted {
			continue
		}
		dec, _ := event.DecodePayload(e)
		if sc := dec.(*event.SubagentCompleted); sc.CallID == "solo" && sc.Reason != "completed" {
			t.Fatalf("solo child reason = %q, want completed despite ghost replaces", sc.Reason)
		}
	}
}
