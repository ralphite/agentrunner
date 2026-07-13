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

// S5.6: a declared output with a workspace path is AUTO-published by the
// epilogue when the run didn't publish it explicitly; the fact carries
// source=epilogue and the ref resolves.
func TestOutputsAutoPublishFromWorkspaceFile(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "bash",
				Args: map[string]any{"command": "echo 'the findings' > report.md"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.Outputs = []OutputSpec{{Name: "report", Path: "report.md", Required: true}}

	res, err := l.Run(context.Background(), "write the report file")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v (contract satisfied via auto-publish)", res)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var pub *event.ArtifactPublished
	for _, e := range events {
		if e.Type == event.TypeArtifactPublished {
			dec, _ := event.DecodePayload(e)
			pub = dec.(*event.ArtifactPublished)
		}
	}
	if pub == nil || pub.Stream != "report" || pub.Source != "epilogue" {
		t.Fatalf("artifact_published = %+v, want epilogue-sourced report", pub)
	}
	content, err := l.Artifacts.Get(pub.Ref)
	if err != nil || !strings.Contains(string(content), "the findings") {
		t.Fatalf("auto-published content = %q, %v", content, err)
	}
	// The publish is part of the quiescent actions; the journal folds to a
	// quiescent shape (决策 #31: no terminal event exists).
	if fold, ferr := state.Fold(events); ferr != nil {
		t.Fatal(ferr)
	} else if q, reason := state.Quiescence(fold); !q || reason != "completed" {
		t.Errorf("quiescence = %v %q, want true completed", q, reason)
	}
}

// S5.6: an explicit publish during the run satisfies the contract — the
// epilogue does not double-publish.
func TestOutputsExplicitPublishSatisfies(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "a1", Name: "publish_artifact",
				Args: map[string]any{"stream": "report", "content": "published by hand"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.Tools = append(l.Spec.Tools, "publish_artifact")
	l.Spec.Outputs = []OutputSpec{{Name: "report", Required: true}}

	res, err := l.Run(context.Background(), "publish it yourself")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	count := 0
	for _, e := range events {
		if e.Type == event.TypeArtifactPublished {
			count++
		}
	}
	if count != 1 {
		t.Errorf("published facts = %d, want 1 (no epilogue double-publish)", count)
	}
}

// S5.6: a missing required output downgrades the ending to
// contract_violation, with a user-visible error.
func TestOutputsMissingRequiredViolatesContract(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "forgot the deliverable"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.Outputs = []OutputSpec{{Name: "report", Path: "report.md", Required: true}}
	sink := &captureSink{}
	l.Out = sink

	res, err := l.Run(context.Background(), "do it")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "contract_violation" {
		t.Fatalf("res = %+v, want contract_violation", res)
	}
	if countKind(sink, "error") != 1 {
		t.Errorf("expected one user-visible error")
	}
	// contract_violation is the RunResult's observer reason; the journal
	// itself folds quiescent (决策 #31: no terminal fact).
	events, _ := store.ReadEvents(l.Store.Dir())
	if fold, ferr := state.Fold(events); ferr != nil {
		t.Fatal(ferr)
	} else if q, _ := state.Quiescence(fold); !q {
		t.Errorf("journal not quiescent after contract violation")
	}
	// An OPTIONAL missing output does not violate.
	fix2 := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "no outputs"}, {Finish: "end_turn"}}},
	}}
	l2 := testLoop(t, fix2, t.TempDir())
	l2.Spec.Outputs = []OutputSpec{{Name: "notes", Path: "notes.md"}}
	res2, err := l2.Run(context.Background(), "go")
	if err != nil || res2.Reason != "completed" {
		t.Fatalf("optional missing output must not violate: %+v, %v", res2, err)
	}
}

// S5.6: a contract-violating CHILD renders as the parent's error result —
// the parent's loop continues and reacts.
func TestOutputsChildViolationIsParentError(t *testing.T) {
	parentFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "prompt": "produce the report"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
		// Parent reacts to the error result and completes.
		{Respond: []scripted.Event{{Text: "delegation failed, wrapping up"}, {Finish: "end_turn"}}},
	}}
	// Child ends without its required output.
	childFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "did some thinking, no file"}, {Finish: "end_turn"}}},
	}}
	l, _ := routedSpawnLoop(t, parentFix, t.TempDir(),
		scripted.RoutePair{Key: "produce the report", Fixture: childFix})
	child := summarizerSpec()
	child.Outputs = []OutputSpec{{Name: "report", Path: "report.md", Required: true}}
	l.SubSpecs = staticResolver(map[string]*AgentSpec{"summarizer": child})

	res, err := l.Run(context.Background(), "delegate")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("parent res = %+v (loop must continue past the violation)", res)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	var sawViolation bool
	for _, m := range fold.Conversation.Messages {
		for _, part := range m.Parts {
			if strings.Contains(part.Text, "contract_violation") {
				sawViolation = true
			}
		}
	}
	if !sawViolation {
		t.Error("contract_violation never reached the parent conversation")
	}
	// The child's own journal folds quiescent — the downgrade is the
	// PARENT-side receipt's reason, never a child journal fact (决策 #31).
	childEvents, err := store.ReadEvents(filepath.Join(l.Store.Dir(), "sub", "s1-a1"))
	if err != nil {
		t.Fatal(err)
	}
	childFold, _ := state.Fold(childEvents)
	if q, _ := state.Quiescence(childFold); !q {
		t.Errorf("child journal not quiescent")
	}
}
