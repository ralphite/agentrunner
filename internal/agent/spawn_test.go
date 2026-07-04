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
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

func summarizerSpec() *AgentSpec {
	return &AgentSpec{
		Name:         "summarizer",
		Description:  "condenses findings into a short report",
		Model:        ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
		SystemPrompt: "you summarize",
		Tools:        []string{"read_file", "edit_file", "bash"},
		MaxTurns:     3,
	}
}

func staticResolver(specs map[string]*AgentSpec) SubSpecResolver {
	return func(name string) (*AgentSpec, error) {
		spec, ok := specs[name]
		if !ok {
			return nil, os.ErrNotExist
		}
		return spec, nil
	}
}

// spawnLoop builds a parent loop whitelisting the summarizer. The scripted
// fixture is SHARED between parent and child (same provider instance):
// spawn blocks, so the step order is parent turn → child turns → parent turn.
func spawnLoop(t *testing.T, fix scripted.Fixture, root string) (*Loop, *capturingProvider) {
	t.Helper()
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
			Name:     "lead",
			Model:    ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			Tools:    []string{"read_file"},
			MaxTurns: 5,
			Agents:   []string{"summarizer"},
		},
		Provider:  cap,
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)),
		SessionID: "lead-sess",
		SubSpecs:  staticResolver(map[string]*AgentSpec{"summarizer": summarizerSpec()}),
	}, cap
}

// S5.3 e2e: the agents directory is frozen into the prefix, spawn_agent is
// advertised, the spawn runs a fresh child in its own journal, the child's
// report returns as the tool result, and the child's usage settles into the
// parent's accounting.
func TestSpawnEndToEnd(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		// Parent turn 1: spawn.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "task": "summarize the findings"}}},
			{Usage: &scripted.UsageEvent{InputTokens: 30, OutputTokens: 10}},
			{Finish: "tool_use"},
		}},
		// Child turn 1 (same provider, spawn blocks).
		{Respond: []scripted.Event{
			{Text: "REPORT: all systems nominal"},
			{Usage: &scripted.UsageEvent{InputTokens: 20, OutputTokens: 5}},
			{Finish: "end_turn"},
		}},
		// Parent turn 2.
		{Respond: []scripted.Event{{Text: "done"},
			{Usage: &scripted.UsageEvent{InputTokens: 8, OutputTokens: 2}}, {Finish: "end_turn"}}},
	}}
	l, cap := spawnLoop(t, fix, t.TempDir())

	res, err := l.Run(context.Background(), "delegate the summary")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.Turns != 2 {
		t.Fatalf("res = %+v", res)
	}

	// Directory frozen into the prefix; spawn_agent advertised.
	sys := cap.requests[0].System
	if !strings.Contains(sys, "<agents>") || !strings.Contains(sys, "summarizer: condenses findings") {
		t.Errorf("agents directory missing from prefix:\n%s", sys)
	}
	var toolNames []string
	for _, td := range cap.requests[0].Tools {
		toolNames = append(toolNames, td.Name)
	}
	if !strings.Contains(strings.Join(toolNames, ","), "spawn_agent") {
		t.Errorf("spawn_agent not advertised: %v", toolNames)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var spawned *event.SpawnRequested
	var completed *event.SubagentCompleted
	for _, e := range events {
		switch e.Type {
		case event.TypeSpawnRequested:
			dec, _ := event.DecodePayload(e)
			spawned = dec.(*event.SpawnRequested)
		case event.TypeSubagentCompleted:
			dec, _ := event.DecodePayload(e)
			completed = dec.(*event.SubagentCompleted)
		}
	}
	if spawned == nil || spawned.Agent != "summarizer" || spawned.Depth != 1 {
		t.Fatalf("spawn_requested = %+v", spawned)
	}
	if completed == nil || completed.Reason != "completed" ||
		completed.Usage.InputTokens != 20 {
		t.Fatalf("subagent_completed = %+v", completed)
	}

	// The child journal is a real, fresh run under the parent's dir.
	childDir := filepath.Join(l.Store.Dir(), "sub", "s1-a1")
	childEvents, err := store.ReadEvents(childDir)
	if err != nil {
		t.Fatalf("child journal: %v", err)
	}
	childFold, err := state.Fold(childEvents)
	if err != nil {
		t.Fatal(err)
	}
	if childFold.Run.Status != state.StatusEnded || childFold.Run.SpecName != "summarizer" {
		t.Errorf("child fold = %+v", childFold.Run)
	}

	// The report reached the parent's fold as the tool result…
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	tr := fold.Conversation.ToolResults["s1"]
	if tr.IsError || !strings.Contains(string(tr.Result), "all systems nominal") {
		t.Errorf("spawn result = %+v", tr)
	}
	// …and the child's usage settled into the parent's accounting.
	if got := fold.Run.Usage.InputTokens; got != 30+20+8 {
		t.Errorf("parent settled input = %d, want 58 (own 38 + child 20)", got)
	}
	if fold.Budget.ReservedTotal() != 0 {
		t.Errorf("spawn reservation not released: %d", fold.Budget.ReservedTotal())
	}
}

// S5.3 no-escalation: the parent's deny rule binds the child — the child
// pipeline is parent gates + child gates, so the child can only be narrower.
func TestSpawnChildCannotEscalatePermissions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "greet.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "task": "edit the file"}}},
			{Finish: "tool_use"},
		}},
		// Child tries the edit the PARENT's rule denies.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "edit_file",
				Args: map[string]any{"path": "greet.txt", "old": "hello", "new": "HACKED"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "could not edit"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	l, _ := spawnLoop(t, fix, root)
	ws := l.Exec.WS
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		&pipeline.SpawnGate{},
		&pipeline.PermissionGate{Rules: []pipeline.PermissionRule{
			{Tool: "edit_file", Action: "deny"},
			{Action: "allow"},
		}, WS: ws},
	}}

	if _, err := l.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	// The file survived: the parent's deny bound the child.
	got, _ := os.ReadFile(filepath.Join(root, "greet.txt"))
	if string(got) != "hello" {
		t.Fatalf("child escaped the parent's deny rule: file = %q", got)
	}
	// And the child journal shows the deny.
	childEvents, err := store.ReadEvents(filepath.Join(l.Store.Dir(), "sub", "s1-a1"))
	if err != nil {
		t.Fatal(err)
	}
	sawDeny := false
	for _, e := range childEvents {
		if e.Type == event.TypeEffectResolved && strings.Contains(string(e.Payload), `"deny"`) {
			sawDeny = true
		}
	}
	if !sawDeny {
		t.Error("child journal missing the deny resolution")
	}
}

// S5.3 mode non-widening. Two halves: (a) plan mode cannot spawn AT ALL —
// spawn_agent is execute-class and the hard floor is unbypassable (spawning
// does work); (b) a child spec that asks for bypass is ignored — the child
// starts in the parent's live mode.
func TestSpawnModeNonWidening(t *testing.T) {
	t.Run("plan mode cannot spawn", func(t *testing.T) {
		fix := scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
					Args: map[string]any{"agent": "summarizer", "task": "anything"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "ok"}, {Finish: "end_turn"}}},
		}}
		l, _ := spawnLoop(t, fix, t.TempDir())
		l.Mode = pipeline.ModePlan
		l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
			&pipeline.FloorGate{},
			&pipeline.SpawnGate{},
			&pipeline.PermissionGate{Rules: []pipeline.PermissionRule{{Action: "allow"}}, WS: l.Exec.WS},
		}}
		if _, err := l.Run(context.Background(), "go"); err != nil {
			t.Fatal(err)
		}
		events, _ := store.ReadEvents(l.Store.Dir())
		fold, err := state.Fold(events)
		if err != nil {
			t.Fatal(err)
		}
		tr := fold.Conversation.ToolResults["s1"]
		if !tr.IsError {
			t.Errorf("plan-mode spawn = %+v, want hard-floor deny", tr)
		}
		if _, err := os.Stat(filepath.Join(l.Store.Dir(), "sub")); err == nil {
			t.Error("a child journal exists despite the deny")
		}
	})

	t.Run("child spec cannot widen mode", func(t *testing.T) {
		fix := scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
					Args: map[string]any{"agent": "summarizer", "task": "report"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "report"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
		}}
		l, _ := spawnLoop(t, fix, t.TempDir())
		child := summarizerSpec()
		child.Mode = pipeline.ModeBypass // the child spec TRIES to widen
		l.SubSpecs = staticResolver(map[string]*AgentSpec{"summarizer": child})
		if _, err := l.Run(context.Background(), "go"); err != nil {
			t.Fatal(err)
		}
		childEvents, err := store.ReadEvents(filepath.Join(l.Store.Dir(), "sub", "s1-a1"))
		if err != nil {
			t.Fatal(err)
		}
		childFold, err := state.Fold(childEvents)
		if err != nil {
			t.Fatal(err)
		}
		if got := childFold.CurrentMode(); got != pipeline.ModeDefault {
			t.Errorf("child mode = %q, want default (spec bypass ignored)", got)
		}
	})
}

// S5.3 tree budget: the spawn reserves the min-aggregated allowance up
// front, so the parent's budget can never be double-committed by a spawn —
// and the whole tree's settled+reserved never exceeds the parent cap.
func TestSpawnBudgetMinAggregation(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "task": "small job"}}},
			{Usage: &scripted.UsageEvent{InputTokens: 60, OutputTokens: 40}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{
			{Text: "tiny report"},
			{Usage: &scripted.UsageEvent{InputTokens: 30, OutputTokens: 20}},
			{Finish: "end_turn"},
		}},
		{Respond: []scripted.Event{{Text: "done"},
			{Usage: &scripted.UsageEvent{InputTokens: 10, OutputTokens: 5}}, {Finish: "end_turn"}}},
	}}
	l, _ := spawnLoop(t, fix, t.TempDir())
	l.Spec.Budget.MaxTotalTokens = 5000
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		&pipeline.SpawnGate{},
		&pipeline.BudgetGate{MaxTotalTokens: 5000},
	}}

	if _, err := l.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	// The spawn's reservation = parent remaining at adjudication, and it was
	// released on settlement; replay proves settled+reserved ≤ cap always.
	s := state.New()
	for _, e := range events {
		if s, err = state.Apply(s, e); err != nil {
			t.Fatal(err)
		}
		if peak := s.Run.Usage.Billed() + s.Budget.ReservedTotal(); peak > 5000 {
			t.Fatalf("tree budget punctured: %d > 5000 after %s", peak, e.Type)
		}
	}
	// Final settled = parent own (100 + 15) + child (50).
	if got := s.Run.Usage.Billed(); got != 165 {
		t.Errorf("settled = %d, want 165", got)
	}
	// The journaled allowance was the min aggregation: parent remaining
	// (5000 − 100 settled) since the child spec is unlimited.
	for _, e := range events {
		if e.Type == event.TypeSpawnRequested {
			dec, _ := event.DecodePayload(e)
			if got := dec.(*event.SpawnRequested).BudgetTokens; got != 4900 {
				t.Errorf("allowance = %d, want 4900", got)
			}
		}
	}
}

// S5.3 caps: depth and fan-out deny via the pipeline (model-visible), never
// crash. Depth: a run already at the cap cannot spawn. Fan-out: the second
// spawn of one turn sees the first's in-batch count.
func TestSpawnDepthAndFanoutCaps(t *testing.T) {
	t.Run("depth", func(t *testing.T) {
		fix := scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
					Args: map[string]any{"agent": "summarizer", "task": "go deeper"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "ok"}, {Finish: "end_turn"}}},
		}}
		l, _ := spawnLoop(t, fix, t.TempDir())
		l.Depth = pipeline.DefaultMaxSpawnDepth // already at the cap
		l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{&pipeline.SpawnGate{}}}
		if _, err := l.Run(context.Background(), "go"); err != nil {
			t.Fatal(err)
		}
		events, _ := store.ReadEvents(l.Store.Dir())
		fold, err := state.Fold(events)
		if err != nil {
			t.Fatal(err)
		}
		tr := fold.Conversation.ToolResults["s1"]
		if !tr.IsError || !strings.Contains(string(tr.Result), "depth") {
			t.Errorf("depth-capped spawn = %+v, want deny mentioning depth", tr)
		}
		if fold.Run.Spawns != 0 {
			t.Errorf("denied spawn must not count: %d", fold.Run.Spawns)
		}
	})

	t.Run("fanout in one batch", func(t *testing.T) {
		fix := scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
					Args: map[string]any{"agent": "summarizer", "task": "job one"}}},
				{ToolCall: &scripted.ToolCallEvent{CallID: "s2", Name: "spawn_agent",
					Args: map[string]any{"agent": "summarizer", "task": "job two"}}},
				{Finish: "tool_use"},
			}},
			// Only ONE child runs (the second spawn is denied at adjudication).
			{Respond: []scripted.Event{{Text: "only child"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
		}}
		l, _ := spawnLoop(t, fix, t.TempDir())
		l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{&pipeline.SpawnGate{MaxSpawns: 1}}}
		if _, err := l.Run(context.Background(), "go"); err != nil {
			t.Fatal(err)
		}
		events, _ := store.ReadEvents(l.Store.Dir())
		fold, err := state.Fold(events)
		if err != nil {
			t.Fatal(err)
		}
		if fold.Run.Spawns != 1 {
			t.Errorf("spawns = %d, want exactly 1 (second denied in-batch)", fold.Run.Spawns)
		}
		r1, r2 := fold.Conversation.ToolResults["s1"], fold.Conversation.ToolResults["s2"]
		if r1.IsError {
			t.Errorf("first spawn should succeed: %+v", r1)
		}
		if !r2.IsError || !strings.Contains(string(r2.Result), "fan-out") {
			t.Errorf("second spawn = %+v, want fan-out deny", r2)
		}
	})
}

// S5.3: an agent outside the whitelist is a model-visible error, and with no
// whitelist the tool is not advertised at all.
func TestSpawnWhitelist(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "hacker", "task": "anything"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "ok"}, {Finish: "end_turn"}}},
	}}
	l, _ := spawnLoop(t, fix, t.TempDir())
	if _, err := l.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	tr := fold.Conversation.ToolResults["s1"]
	if !tr.IsError || !strings.Contains(string(tr.Result), "not in this agent's directory") {
		t.Errorf("off-whitelist spawn = %+v", tr)
	}

	// Without a whitelist, spawn_agent is not advertised.
	fix2 := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}
	l2, cap2 := spawnLoop(t, fix2, t.TempDir())
	l2.Spec.Agents = nil
	if _, err := l2.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	for _, td := range cap2.requests[0].Tools {
		if td.Name == "spawn_agent" {
			t.Error("spawn_agent advertised without a whitelist")
		}
	}
}
