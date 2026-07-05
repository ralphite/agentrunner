package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// S5 review P0: concurrent publishes into the tree-shared store must not
// lose manifest versions or tear the file.
func TestArtifactStoreConcurrentPublish(t *testing.T) {
	a, err := store.OpenArtifactStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const n = 24
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := a.Publish("shared", fmt.Appendf(nil, "content-%d", i)); err != nil {
				t.Errorf("publish %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	streams, err := a.Streams()
	if err != nil {
		t.Fatal(err)
	}
	chain := streams["shared"]
	if len(chain) != n {
		t.Fatalf("chain = %d versions, want %d (lost updates)", len(chain), n)
	}
	for i, v := range chain {
		if v.Version != i+1 {
			t.Fatalf("version chain not dense at %d: %+v", i, v)
		}
	}
}

// S5 review P2: republishing the same content at the chain tip (the
// crash-resume path) returns the SAME version, not a duplicate.
func TestArtifactStoreRepublishSameContentDedups(t *testing.T) {
	a, err := store.OpenArtifactStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v1, _ := a.Publish("plan", []byte("the plan"))
	v2, _ := a.Publish("plan", []byte("the plan"))
	if v2.Version != v1.Version || v2.Ref != v1.Ref {
		t.Fatalf("re-publish minted a duplicate: %+v vs %+v", v1, v2)
	}
	// Different content still advances.
	v3, _ := a.Publish("plan", []byte("revised plan"))
	if v3.Version != 2 {
		t.Fatalf("v3 = %+v", v3)
	}
}

// S5 review P1: a FAILED child's real spend settles into the parent — the
// tree budget cannot be punctured through the failure path.
func TestSpawnFailedChildUsageSettles(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "task": "burn and die"}}},
			{Usage: &scripted.UsageEvent{InputTokens: 10, OutputTokens: 5}},
			{Finish: "tool_use"},
		}},
		// Child turn 1 spends real tokens…
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "read_file",
				Args: map[string]any{"path": "x.txt"}}},
			{Usage: &scripted.UsageEvent{InputTokens: 500, OutputTokens: 100}},
			{Finish: "tool_use"},
		}},
		// …then its turn 2 hits an impossible drift Expect: the scripted
		// provider errors (non-retryable) and the child ABORTS having spent.
		{
			Expect:  scripted.Expect{LastMessageContains: "IMPOSSIBLE-SENTINEL"},
			Respond: []scripted.Event{{Text: "never served"}, {Finish: "end_turn"}},
		},
		{Respond: []scripted.Event{{Text: "parent reacts to the failure"}, {Finish: "end_turn"}}},
	}}
	l, _ := spawnLoop(t, fix, t.TempDir())

	res, err := l.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v (failed child is model-visible, parent continues)", res)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	tr := fold.Conversation.ToolResults["s1"]
	if !tr.IsError || !strings.Contains(string(tr.Result), "failed") {
		t.Fatalf("spawn result = %+v", tr)
	}
	// Parent settled its own 15 + the dead child's 600.
	if got := fold.Session.Usage.InputTokens + fold.Session.Usage.OutputTokens; got != 15+600 {
		t.Errorf("settled = %d, want 615 (failed child's spend must count)", got)
	}
	// And the SubagentCompleted fact carries the real spend for inspect.
	for _, e := range events {
		if e.Type == event.TypeSubagentCompleted {
			dec, _ := event.DecodePayload(e)
			if u := dec.(*event.SubagentCompleted).Usage; u.InputTokens != 500 {
				t.Errorf("subagent_completed usage = %+v", u)
			}
		}
	}
}

// S5 review P2: once a handoff is allowed in a turn, every further agent
// launch in the SAME turn is denied — control transfer is exclusive.
func TestHandoffExclusiveInBatch(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "h1", Name: "handoff_agent",
				Args: map[string]any{"agent": "summarizer", "task": "take over"}}},
			{ToolCall: &scripted.ToolCallEvent{CallID: "h2", Name: "handoff_agent",
				Args: map[string]any{"agent": "summarizer", "task": "also take over"}}},
			{Finish: "tool_use"},
		}},
		// Exactly ONE successor runs.
		{Respond: []scripted.Event{{Text: "successor done"}, {Finish: "end_turn"}}},
	}}
	l, _ := spawnLoop(t, fix, t.TempDir())
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{&pipeline.SpawnGate{}}}

	res, err := l.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "handoff" {
		t.Fatalf("res = %+v", res)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if fold.Session.Spawns != 1 {
		t.Errorf("spawns = %d, want 1 (second handoff denied)", fold.Session.Spawns)
	}
	r2 := fold.Conversation.ToolResults["h2"]
	if !r2.IsError || !strings.Contains(string(r2.Result), "already transferred") {
		t.Errorf("second handoff = %+v, want exclusive-transfer deny", r2)
	}
}

// S5 review: a child spec MAY be narrower than the parent (plan child under
// a default parent stays in plan mode).
func TestSpawnChildNarrowerModeKept(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "task": "plan only"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "planned"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	l, _ := spawnLoop(t, fix, t.TempDir())
	child := summarizerSpec()
	child.Mode = pipeline.ModePlan // narrower than the parent's default
	l.SubSpecs = staticResolver(map[string]*AgentSpec{"summarizer": child})

	if _, err := l.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	childEvents, err := store.ReadEvents(filepath.Join(l.Store.Dir(), "sub", "s1-a1"))
	if err != nil {
		t.Fatal(err)
	}
	childFold, _ := state.Fold(childEvents)
	if got := childFold.CurrentMode(); got != pipeline.ModePlan {
		t.Errorf("child mode = %q, want plan (narrower spec mode kept)", got)
	}
}

// S5 review P1: materialize passes the pipeline — a path-scoped deny rule
// binds an artifact-input write exactly like an edit_file.
func TestMaterializeDeniedByPathRule(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "task": "go",
					"inputs": []map[string]any{{"ref": "REF", "path": ".github/workflows/x.yml"}}}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "spawn failed, fine"}, {Finish: "end_turn"}}},
	}}
	l, _ := spawnLoop(t, fix, t.TempDir())
	if err := l.ensureArtifacts(); err != nil {
		t.Fatal(err)
	}
	ref, err := l.Artifacts.Put([]byte("workflow content"))
	if err != nil {
		t.Fatal(err)
	}
	fix.Steps[0].Respond[0].ToolCall.Args["inputs"] = []map[string]any{
		{"ref": ref, "path": ".github/workflows/x.yml"}}
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		&pipeline.SpawnGate{},
		&pipeline.PermissionGate{Rules: []pipeline.PermissionRule{
			{Tool: "materialize", Action: "deny"},
			{Action: "allow"},
		}, WS: l.Exec.WS},
	}}

	res, err := l.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	fold, _ := state.Fold(events)
	tr := fold.Conversation.ToolResults["s1"]
	if !tr.IsError || !strings.Contains(string(tr.Result), "failed") {
		t.Errorf("spawn with denied materialize = %+v, want error result", tr)
	}
}
