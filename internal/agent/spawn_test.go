package agent

import (
	"context"
	"encoding/json"
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
		Name:               "summarizer",
		Description:        "condenses findings into a short report",
		Model:              ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
		SystemPrompt:       "you summarize",
		Tools:              []string{"read_file", "edit_file", "bash"},
		MaxGenerationSteps: 3,
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

// spawnLoop builds a parent loop whitelisting the summarizer, serving one
// sequential fixture — for scenarios where NO child actually runs (denied
// or failing spawns) or where the successor is synchronous (handoff).
func spawnLoop(t *testing.T, fix scripted.Fixture, root string) (*Loop, *capturingProvider) {
	t.Helper()
	return spawnLoopWith(t, &capturingProvider{inner: scripted.New(fix)}, root)
}

// routedSpawnLoop wires the parent and each child to its OWN fixture:
// spawn is always non-blocking (零 legacy), so the scripts race — routing
// keeps them deterministic (GAPS G4). The parent fixture is the fallback
// route; give it spare zero-usage steps because how many turns run before
// the child settles is real-concurrency timing.
func routedSpawnLoop(t *testing.T, parentFix scripted.Fixture, root string,
	childRoutes ...scripted.RoutePair) (*Loop, *capturingProvider) {
	t.Helper()
	pairs := append(childRoutes, scripted.RoutePair{Key: "", Fixture: parentFix})
	return spawnLoopWith(t, &capturingProvider{inner: scripted.NewRouter(pairs...)}, root)
}

func spawnLoopWith(t *testing.T, cap *capturingProvider, root string) (*Loop, *capturingProvider) {
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
	return &Loop{
		Spec: &AgentSpec{
			Name:               "lead",
			Model:              ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			Tools:              []string{"read_file"},
			MaxGenerationSteps: 5,
			Agents:             []string{"summarizer"},
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
	parentFix := scripted.Fixture{Steps: []scripted.Step{
		// Parent turn 1: spawn — the handle pairs immediately.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "prompt": "summarize the findings"}}},
			{Usage: &scripted.UsageEvent{InputTokens: 30, OutputTokens: 10}},
			{Finish: "tool_use"},
		}},
		// Whether the receipt lands before or after the ack turn is timing;
		// the LAST consumed parent step carries the wrap-up usage and the
		// spares carry none, so the settled totals stay exact.
		{Respond: []scripted.Event{{Text: "waiting for the report"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"},
			{Usage: &scripted.UsageEvent{InputTokens: 8, OutputTokens: 2}}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"},
			{Usage: &scripted.UsageEvent{InputTokens: 8, OutputTokens: 2}}, {Finish: "end_turn"}}},
	}}
	childFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: "REPORT: all systems nominal"},
			{Usage: &scripted.UsageEvent{InputTokens: 20, OutputTokens: 5}},
			{Finish: "end_turn"},
		}},
	}}
	l, cap := routedSpawnLoop(t, parentFix, t.TempDir(),
		scripted.RoutePair{Key: "summarize the findings", Fixture: childFix})

	res, err := l.Run(context.Background(), "delegate the summary")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.GenSteps < 2 {
		t.Fatalf("res = %+v", res)
	}

	// Directory frozen into the prefix; spawn_agent advertised.
	requests := cap.Requests()
	sys := requests[0].System
	if !strings.Contains(sys, "<agents>") || !strings.Contains(sys, "summarizer: condenses findings") {
		t.Errorf("agents directory missing from prefix:\n%s", sys)
	}
	var toolNames []string
	for _, td := range requests[0].Tools {
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
	if q, _ := state.Quiescence(childFold); !q || childFold.Session.SpecName != "summarizer" {
		t.Errorf("child fold = %+v", childFold.Session)
	}

	// The call paired with a handle at once; the report re-entered as a
	// user-role message (决策 #27: 完成回执是父 inbox 输入).
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	tr := fold.Conversation.ToolResults["s1"]
	if tr.IsError || !strings.Contains(string(tr.Result), "running") {
		t.Errorf("spawn handle = %+v", tr)
	}
	var sawReport bool
	for _, m := range fold.Conversation.Messages {
		for _, part := range m.Parts {
			if strings.Contains(part.Text, "all systems nominal") {
				sawReport = true
			}
		}
	}
	if !sawReport {
		t.Error("child report never re-entered the parent conversation")
	}
	// …and the child's usage settled into the parent's accounting: spawn
	// turn 30 + child 20, plus 8 per wrap-up turn (turn count is settle
	// timing, the CHILD's 20 must be in regardless).
	if got := fold.Session.Usage.InputTokens; got < 50 || (got-50)%8 != 0 {
		t.Errorf("parent settled input = %d, want 50 + n×8 (child spend settled)", got)
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
	parentFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "prompt": "edit the file"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	// Child tries the edit the PARENT's rule denies.
	childFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "edit_file",
				Args: map[string]any{"path": "greet.txt", "old": "hello", "new": "HACKED"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "could not edit"}, {Finish: "end_turn"}}},
	}}
	l, _ := routedSpawnLoop(t, parentFix, root,
		scripted.RoutePair{Key: "edit the file", Fixture: childFix})
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

// S6 (S5 回访): the child's SessionStarted materializes the WHOLE permission
// intersection chain as data — parent layer first, child layer second — so a
// standalone resume of the child session rebuilds the same bounds without
// the parent process's gate pointers.
func TestSpawnMaterializesPermissionLayers(t *testing.T) {
	root := t.TempDir()
	parentFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "prompt": "small job"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "all done"}, {Finish: "end_turn"}}},
	}}
	childFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "child done"}, {Finish: "end_turn"}}},
	}}
	l, _ := routedSpawnLoop(t, parentFix, root,
		scripted.RoutePair{Key: "small job", Fixture: childFix})
	ws := l.Exec.WS
	parentRules := []pipeline.PermissionRule{
		{Tool: "edit_file", Action: "deny"},
		{Action: "allow"},
	}
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		&pipeline.SpawnGate{},
		&pipeline.PermissionGate{Rules: parentRules, WS: ws},
	}}
	childSpec := summarizerSpec()
	childSpec.Permissions = []pipeline.PermissionRule{
		{Tool: "bash", Action: "deny"},
		{Action: "allow"},
	}
	l.SubSpecs = staticResolver(map[string]*AgentSpec{"summarizer": childSpec})

	if _, err := l.Run(context.Background(), "delegate"); err != nil {
		t.Fatal(err)
	}

	layersOf := func(dir string) [][]pipeline.PermissionRule {
		t.Helper()
		events, err := store.ReadEvents(dir)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := event.DecodePayload(events[0])
		if err != nil {
			t.Fatal(err)
		}
		raw := decoded.(*event.SessionStarted).PermissionLayers
		if len(raw) == 0 {
			return nil
		}
		var layers [][]pipeline.PermissionRule
		if err := json.Unmarshal(raw, &layers); err != nil {
			t.Fatal(err)
		}
		return layers
	}

	parentLayers := layersOf(l.Store.Dir())
	if len(parentLayers) != 1 || parentLayers[0][0].Tool != "edit_file" {
		t.Fatalf("parent layers = %+v, want the single parent rule layer", parentLayers)
	}
	childLayers := layersOf(filepath.Join(l.Store.Dir(), "sub", "s1-a1"))
	if len(childLayers) != 2 {
		t.Fatalf("child layers = %+v, want [parent, child]", childLayers)
	}
	if childLayers[0][0].Tool != "edit_file" || childLayers[1][0].Tool != "bash" {
		t.Fatalf("child layers order = %+v, want parent (edit_file deny) then child (bash deny)", childLayers)
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
					Args: map[string]any{"agent": "summarizer", "prompt": "anything"}}},
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
		parentFix := scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
					Args: map[string]any{"agent": "summarizer", "prompt": "WIDEN-JOB: write it up"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
		}}
		childFix := scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "written up"}, {Finish: "end_turn"}}},
		}}
		l, _ := routedSpawnLoop(t, parentFix, t.TempDir(),
			scripted.RoutePair{Key: "WIDEN-JOB", Fixture: childFix})
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
	parentFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "prompt": "small job"}}},
			{Usage: &scripted.UsageEvent{InputTokens: 60, OutputTokens: 40}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"},
			{Usage: &scripted.UsageEvent{InputTokens: 10, OutputTokens: 5}}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"},
			{Usage: &scripted.UsageEvent{InputTokens: 10, OutputTokens: 5}}, {Finish: "end_turn"}}},
	}}
	childFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: "tiny report"},
			{Usage: &scripted.UsageEvent{InputTokens: 30, OutputTokens: 20}},
			{Finish: "end_turn"},
		}},
	}}
	l, _ := routedSpawnLoop(t, parentFix, t.TempDir(),
		scripted.RoutePair{Key: "small job", Fixture: childFix})
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
		if peak := s.Session.Usage.Billed() + s.Budget.ReservedTotal(); peak > 5000 {
			t.Fatalf("tree budget punctured: %d > 5000 after %s", peak, e.Type)
		}
	}
	// Final settled = parent spawn turn (100) + child (50) + 15 per
	// wrap-up turn (turn count is settle timing; the child's 50 is in
	// regardless, and the invariant above already proved the cap held).
	if got := s.Session.Usage.Billed(); got < 150 || (got-150)%15 != 0 {
		t.Errorf("settled = %d, want 150 + n×15", got)
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

func TestSpawnAllowanceFairSharesBatch(t *testing.T) {
	l := &Loop{Spec: &AgentSpec{Budget: BudgetSpec{MaxTotalTokens: 300}}}
	s := state.New()
	unlimited := &AgentSpec{}
	if got := l.spawnAllowance(s, unlimited, 3); got != 100 {
		t.Fatalf("first allowance = %d, want 100", got)
	}
	s.Budget.Reserved["first"] = 100
	if got := l.spawnAllowance(s, unlimited, 2); got != 100 {
		t.Fatalf("second allowance = %d, want 100", got)
	}
	s.Budget.Reserved["second"] = 100
	if got := l.spawnAllowance(s, unlimited, 1); got != 100 {
		t.Fatalf("last allowance = %d, want 100", got)
	}

	s = state.New()
	capped := &AgentSpec{Budget: BudgetSpec{MaxTotalTokens: 50}}
	if got := l.spawnAllowance(s, capped, 3); got != 50 {
		t.Fatalf("capped allowance = %d, want 50", got)
	}
	s.Budget.Reserved["small"] = 50
	if got := l.spawnAllowance(s, unlimited, 2); got != 125 {
		t.Fatalf("unused share was not redistributed: %d, want 125", got)
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
					Args: map[string]any{"agent": "summarizer", "prompt": "go deeper"}}},
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
		if fold.Session.Spawns != 0 {
			t.Errorf("denied spawn must not count: %d", fold.Session.Spawns)
		}
	})

	t.Run("fanout in one batch", func(t *testing.T) {
		parentFix := scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
					Args: map[string]any{"agent": "summarizer", "prompt": "job one"}}},
				{ToolCall: &scripted.ToolCallEvent{CallID: "s2", Name: "spawn_agent",
					Args: map[string]any{"agent": "summarizer", "prompt": "job two"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
		}}
		// Only ONE child runs (the second spawn is denied at adjudication).
		childFix := scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "only child"}, {Finish: "end_turn"}}},
		}}
		l, _ := routedSpawnLoop(t, parentFix, t.TempDir(),
			scripted.RoutePair{Key: "job one", Fixture: childFix})
		l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{&pipeline.SpawnGate{MaxSpawns: 1}}}
		if _, err := l.Run(context.Background(), "go"); err != nil {
			t.Fatal(err)
		}
		events, _ := store.ReadEvents(l.Store.Dir())
		fold, err := state.Fold(events)
		if err != nil {
			t.Fatal(err)
		}
		if fold.Session.Spawns != 1 {
			t.Errorf("spawns = %d, want exactly 1 (second denied in-batch)", fold.Session.Spawns)
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
				Args: map[string]any{"agent": "hacker", "prompt": "anything"}}},
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
	requests := cap2.Requests()
	for _, td := range requests[0].Tools {
		if td.Name == "spawn_agent" {
			t.Error("spawn_agent advertised without a whitelist")
		}
	}
}

// INC-12.4: agents_dynamic opens spawn_agent without a static directory and
// freezes a model-authored inline role into both the parent spawn fact and
// the child's SessionStarted spec. The role may only narrow the parent's
// explicit tool face and cannot smuggle MCP/hooks/skills capabilities.
func TestSpawnDynamicRole(t *testing.T) {
	parentFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "dyn", Name: "spawn_agent", Args: map[string]any{
				"prompt": "DYNAMIC-WORK", "role": map[string]any{
					"name": "reviewer", "description": "reviews changes",
					"instructions": "you review dynamically", "tools": []string{"read_file"},
				},
			}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	childFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "dynamic review complete"}, {Finish: "end_turn"}}},
	}}
	l, cap := routedSpawnLoop(t, parentFix, t.TempDir(),
		scripted.RoutePair{Key: "you review dynamically", Fixture: childFix})
	l.Spec.Agents = nil
	l.Spec.AgentsDynamic = true
	if _, err := l.Run(context.Background(), "assemble a dynamic team"); err != nil {
		t.Fatal(err)
	}

	request := cap.Requests()[0]
	if !strings.Contains(request.System, "Inline roles are allowed") {
		t.Fatalf("dynamic role instructions absent from system prefix:\n%s", request.System)
	}
	var sawSpawn bool
	for _, def := range request.Tools {
		if def.Name == "spawn_agent" {
			sawSpawn = true
		}
	}
	if !sawSpawn {
		t.Fatal("spawn_agent not advertised for agents_dynamic")
	}

	events, _ := store.ReadEvents(l.Store.Dir())
	var spawned *event.SpawnRequested
	for _, env := range events {
		if env.Type == event.TypeSpawnRequested {
			decoded, _ := event.DecodePayload(env)
			spawned = decoded.(*event.SpawnRequested)
		}
	}
	if spawned == nil || spawned.Agent != "reviewer" || len(spawned.RoleSpec) == 0 {
		t.Fatalf("dynamic spawn fact = %+v", spawned)
	}
	var frozen AgentSpec
	if err := json.Unmarshal(spawned.RoleSpec, &frozen); err != nil {
		t.Fatal(err)
	}
	if frozen.Name != "reviewer" || !frozen.AgentsDynamic || len(frozen.Tools) != 1 ||
		frozen.Tools[0] != "read_file" || len(frozen.MCP) != 0 {
		t.Fatalf("frozen dynamic spec = %+v", frozen)
	}
	childEvents, _ := store.ReadEvents(filepath.Join(l.Store.Dir(), "sub", "dyn-a1"))
	childFold, err := state.Fold(childEvents)
	if err != nil {
		t.Fatal(err)
	}
	if childFold.Session.SpecName != "reviewer" || childFold.Session.Agents == "" {
		t.Fatalf("child did not start from frozen dynamic spec: %+v", childFold.Session)
	}
}

// INC-12.5: escalate is an explicit human-reviewed exception. Approval
// drops the parent's permission layer for the child only; denial (including
// interrupt) still launches under the ordinary parent∩child chain and is
// visible in the immediate handle result.
func TestEscalationApproval(t *testing.T) {
	type expectation struct {
		approve, interrupt, edited bool
		outcome                    string
	}
	cases := map[string]expectation{
		"approved":  {approve: true, edited: true, outcome: "approved"},
		"denied":    {outcome: "denied"},
		"interrupt": {interrupt: true, outcome: "denied"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "greet.txt"), []byte("hello"), 0o644); err != nil {
				t.Fatal(err)
			}
			parentFix := scripted.Fixture{Steps: []scripted.Step{
				{Respond: []scripted.Event{
					{ToolCall: &scripted.ToolCallEvent{CallID: "esc", Name: "spawn_agent", Args: map[string]any{
						"prompt": "ESCALATED-EDIT", "role": map[string]any{
							"name": "engineer", "description": "edits the requested file",
							"instructions": "you are the escalated engineer",
							"tools":        []string{"edit_file"}, "escalate": true,
							"permissions": []map[string]any{
								{"tool": "edit_file", "action": "allow"}, {"action": "deny"},
							},
						},
					}}},
					{Finish: "tool_use"},
				}},
				{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
				{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
				{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
			}}
			childFix := scripted.Fixture{Steps: []scripted.Step{
				{Respond: []scripted.Event{
					{ToolCall: &scripted.ToolCallEvent{CallID: "edit", Name: "edit_file", Args: map[string]any{
						"path": "greet.txt", "old": "hello", "new": "ENGINEERED",
					}}}, {Finish: "tool_use"},
				}},
				{Respond: []scripted.Event{{Text: "edit attempt complete"}, {Finish: "end_turn"}}},
			}}
			l, _ := routedSpawnLoop(t, parentFix, root,
				scripted.RoutePair{Key: "ESCALATED-EDIT", Fixture: childFix})
			l.Spec.Agents = nil
			l.Spec.AgentsDynamic = true
			l.Spec.Tools = []string{"edit_file"}
			l.Spec.Sandbox.Network = "none"
			l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
				&pipeline.FloorGate{WS: l.Exec.WS}, &pipeline.SpawnGate{},
				&pipeline.PermissionGate{Rules: []pipeline.PermissionRule{
					{Tool: "spawn_agent", Action: "allow"},
					{Tool: "edit_file", Action: "deny"},
					{Action: "allow"},
				}, WS: l.Exec.WS},
			}}
			if want.interrupt {
				ready := make(chan struct{}, 1)
				interrupts := make(chan struct{}, 1)
				l.Approvals = blockingApprover{ready: ready}
				l.Interrupts = interrupts
				go func() { <-ready; interrupts <- struct{}{} }()
			} else if want.approve {
				t.Setenv("AGENTRUNNER_APPROVE", "always")
			} else {
				t.Setenv("AGENTRUNNER_APPROVE", "never")
			}

			if _, err := l.Run(context.Background(), "delegate with explicit authority review"); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(filepath.Join(root, "greet.txt"))
			if (string(got) == "ENGINEERED") != want.edited {
				t.Fatalf("file = %q, edited want %v", got, want.edited)
			}
			events, _ := store.ReadEvents(l.Store.Dir())
			var spawned *event.SpawnRequested
			var approval *event.ApprovalRequested
			for _, env := range events {
				switch env.Type {
				case event.TypeSpawnRequested:
					decoded, _ := event.DecodePayload(env)
					spawned = decoded.(*event.SpawnRequested)
				case event.TypeApprovalRequested:
					decoded, _ := event.DecodePayload(env)
					approval = decoded.(*event.ApprovalRequested)
				}
			}
			if spawned == nil || spawned.Escalation != want.outcome {
				t.Fatalf("spawn escalation = %+v", spawned)
			}
			if approval == nil || !approval.DenyAllowsFallback ||
				!strings.Contains(string(approval.Args), "permissions") {
				t.Fatalf("approval request lacks reviewed rules/fallback contract: %+v", approval)
			}
			fold, err := state.Fold(events)
			if err != nil {
				t.Fatal(err)
			}
			handle := fold.Conversation.ToolResults["esc"]
			if want.outcome == "denied" && !strings.Contains(string(handle.Result), "parent∩child") {
				t.Fatalf("denied fallback not model-visible: %+v", handle)
			}
			childEvents, _ := store.ReadEvents(filepath.Join(l.Store.Dir(), "sub", "esc-a1"))
			started, _ := event.DecodePayload(childEvents[0])
			var layers [][]pipeline.PermissionRule
			_ = json.Unmarshal(started.(*event.SessionStarted).PermissionLayers, &layers)
			wantLayers := 2
			if want.approve {
				wantLayers = 1
			}
			if len(layers) != wantLayers {
				t.Fatalf("permission layers = %+v, want %d", layers, wantLayers)
			}
			if !l.Exec.NetworkContained() {
				t.Fatal("authority approval widened the shared network containment ratchet")
			}
		})
	}
}
