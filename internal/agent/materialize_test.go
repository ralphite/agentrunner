package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// S5.8: a parent publishes an artifact, spawns a child passing the ref as
// input, and the child finds the MATERIALIZED file in its workspace — the
// materialize activity journaled in the child's log before its first turn.
func TestArtifactInputMaterializedForChild(t *testing.T) {
	parentFix := scripted.Fixture{Steps: []scripted.Step{
		// Parent turn 1: publish the briefing.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "a1", Name: "publish_artifact",
				Args: map[string]any{"stream": "briefing", "content": "focus: the auth module"}}},
			{Finish: "tool_use"},
		}},
		// Parent turn 2: spawn with the ref as input. The ref is not known
		// statically — the test rewrites this step after computing it (the
		// ToolCall pointer is shared with the provider's copy).
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "task": "CONFIRM-BRIEFING now",
					"inputs": []map[string]any{{"ref": "PLACEHOLDER", "path": "briefing.md"}}}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
		// Parent: done.
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	childFix := scripted.Fixture{Steps: []scripted.Step{
		// Child turn 1: read the materialized file.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "read_file",
				Args: map[string]any{"path": "briefing.md"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "confirmed: auth module"}, {Finish: "end_turn"}}},
	}}

	// Compute the ref the same way the store will (content-addressed), then
	// patch the fixture — deterministic by CAS design.
	root := t.TempDir()
	l, _ := routedSpawnLoop(t, parentFix, root,
		scripted.RoutePair{Key: "CONFIRM-BRIEFING", Fixture: childFix})
	child := summarizerSpec()
	child.Tools = []string{"read_file"}
	l.SubSpecs = staticResolver(map[string]*AgentSpec{"summarizer": child})
	l.Spec.Tools = append(l.Spec.Tools, "publish_artifact")
	if err := l.ensureArtifacts(); err != nil {
		t.Fatal(err)
	}
	ref, err := l.Artifacts.Put([]byte("focus: the auth module"))
	if err != nil {
		t.Fatal(err)
	}
	parentFix.Steps[1].Respond[0].ToolCall.Args["inputs"] = []map[string]any{
		{"ref": ref, "path": "briefing.md"}}

	res, err := l.Run(context.Background(), "brief then delegate")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}

	// The child's journal: SessionStarted carries the input, a materialize
	// activity completed BEFORE the first turn, and the read saw the content.
	childDir := filepath.Join(l.Store.Dir(), "sub", "s1-a1")
	childEvents, err := store.ReadEvents(childDir)
	if err != nil {
		t.Fatal(err)
	}
	var matSeq, turnSeq int64
	for _, e := range childEvents {
		if e.Type == event.TypeActivityCompleted && strings.Contains(string(e.Payload), "materialize") {
			matSeq = e.Seq
		}
		if e.Type == event.TypeGenerationStarted && turnSeq == 0 {
			turnSeq = e.Seq
		}
	}
	if matSeq == 0 || turnSeq == 0 || matSeq > turnSeq {
		t.Fatalf("materialize (seq %d) must complete before turn 1 (seq %d)", matSeq, turnSeq)
	}
	childFold, err := state.Fold(childEvents)
	if err != nil {
		t.Fatal(err)
	}
	if !childFold.Session.Materialized || len(childFold.Session.Inputs) != 1 {
		t.Errorf("child fold: materialized=%v inputs=%+v", childFold.Session.Materialized, childFold.Session.Inputs)
	}
	c1 := childFold.Conversation.ToolResults["c1"]
	if c1.IsError || !strings.Contains(string(c1.Result), "focus: the auth module") {
		t.Errorf("child read = %+v, want the materialized briefing", c1)
	}
}

// S5.8: a dangling input ref is the parent model's mistake — model-visible
// error, no child run starts.
func TestArtifactInputDanglingRefRejected(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "task": "go",
					"inputs": []map[string]any{{"ref": "sha256-doesnotexist", "path": "x.md"}}}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "ok, without inputs then"}, {Finish: "end_turn"}}},
	}}
	l, _ := spawnLoop(t, fix, t.TempDir())
	res, err := l.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	tr := fold.Conversation.ToolResults["s1"]
	if !tr.IsError || !strings.Contains(string(tr.Result), "does not resolve") {
		t.Errorf("dangling ref spawn = %+v", tr)
	}
}
