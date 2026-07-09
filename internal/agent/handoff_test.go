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

// S5.4 handoff: control transfers to the successor (a fresh child run under
// the spawn discipline) and the CALLER's run ends with reason "handoff" —
// the parent never acts again.
func TestHandoffEndsParentRun(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		// Parent turn 1: hand off. NOTE: no further parent steps — the run
		// must end without another LLM call.
		{Respond: []scripted.Event{
			{Text: "this needs the specialist"},
			{ToolCall: &scripted.ToolCallEvent{CallID: "h1", Name: "handoff_agent",
				Args: map[string]any{"agent": "summarizer", "task": "finish the report"}}},
			{Usage: &scripted.UsageEvent{InputTokens: 30, OutputTokens: 10}},
			{Finish: "tool_use"},
		}},
		// Successor's run.
		{Respond: []scripted.Event{
			{Text: "FINAL REPORT: complete"},
			{Usage: &scripted.UsageEvent{InputTokens: 20, OutputTokens: 5}},
			{Finish: "end_turn"},
		}},
	}}
	l, cap := spawnLoop(t, fix, t.TempDir())

	res, err := l.Run(context.Background(), "do the work")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "handoff" || res.GenSteps != 1 {
		t.Fatalf("res = %+v, want reason handoff after one turn", res)
	}
	// Every fixture step consumed: the successor ran, the parent did NOT
	// take another turn.
	if err := cap.inner.(interface{ Done() error }).Done(); err != nil {
		t.Errorf("fixture: %v", err)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	// The handoff finishes the turn; the journal folds quiescent with
	// reason "handoff" (决策 #31: shape, not event). The successor is
	// journaled through the same spawn facts (it IS a child run in the tree).
	if fold, ferr := state.Fold(events); ferr != nil {
		t.Fatal(ferr)
	} else if q, reason := state.Quiescence(fold); !q || reason != "handoff" {
		t.Errorf("quiescence = %v %q, want true handoff", q, reason)
	}
	var completed *event.SubagentCompleted
	for _, e := range events {
		if e.Type == event.TypeSubagentCompleted {
			dec, _ := event.DecodePayload(e)
			completed = dec.(*event.SubagentCompleted)
		}
	}
	if completed == nil || completed.Agent != "summarizer" || completed.Reason != "completed" {
		t.Fatalf("subagent_completed = %+v", completed)
	}
	// The successor's usage settled into the (now ended) parent accounting.
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if got := fold.Session.Usage.InputTokens; got != 30+20 {
		t.Errorf("settled input = %d, want 50 (parent 30 + successor 20)", got)
	}
	// The successor's own journal holds its run.
	childEvents, err := store.ReadEvents(filepath.Join(l.Store.Dir(), "sub", "h1-a1"))
	if err != nil {
		t.Fatal(err)
	}
	childFold, err := state.Fold(childEvents)
	if err != nil {
		t.Fatal(err)
	}
	if q, reason := state.Quiescence(childFold); !q || reason != "completed" {
		t.Errorf("successor fold = %+v", childFold.Session)
	}
}

// S5.4: a failed handoff target is a model-visible error and the run
// CONTINUES — control only transfers on success.
func TestHandoffFailureContinuesRun(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "h1", Name: "handoff_agent",
				Args: map[string]any{"agent": "nobody", "task": "anything"}}},
			{Finish: "tool_use"},
		}},
		// The parent reacts to the error and finishes normally.
		{Respond: []scripted.Event{{Text: "fine, doing it myself"}, {Finish: "end_turn"}}},
	}}
	l, _ := spawnLoop(t, fix, t.TempDir())
	res, err := l.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.GenSteps != 2 {
		t.Fatalf("res = %+v, want a normal completion after the failed handoff", res)
	}
}

// S5.4 blackboard: notes published by the parent are visible to a child
// (and vice versa), reads land durably in each reader's own journal, and
// the tools are advertised across the tree.
func TestBlackboardCollaboration(t *testing.T) {
	// Routing by REQUEST SHAPE keeps the parent deterministic against
	// settle timing: once the child's receipt (containing "acknowledged")
	// is in the transcript, every later parent request matches that route
	// and replays its own read-back script — whether or not a handle-ack
	// turn ran in between.
	parentMain := scripted.Fixture{Steps: []scripted.Step{
		// Parent turn 1: publish a note.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "p1", Name: "publish_note",
				Args: map[string]any{"topic": "plan", "text": "focus on the API layer"}}},
			{Finish: "tool_use"},
		}},
		// Parent turn 2: spawn the child (non-blocking, handle pairs now).
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "task": "ACK-THE-PLAN please"}}},
			{Finish: "tool_use"},
		}},
		// Handle-ack turn (only reached when the receipt has not landed yet).
		{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
	}}
	afterReceipt := scripted.Fixture{Steps: []scripted.Step{
		// Read the topic back — sees both notes.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "p2", Name: "read_notes",
				Args: map[string]any{"topic": "plan"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	childFix := scripted.Fixture{Steps: []scripted.Step{
		// Child turn 1: read the notes, then publish its own.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "read_notes",
				Args: map[string]any{"topic": "plan"}}},
			{ToolCall: &scripted.ToolCallEvent{CallID: "c2", Name: "publish_note",
				Args: map[string]any{"topic": "plan", "text": "ack: API layer it is"}}},
			{Finish: "tool_use"},
		}},
		// Child turn 2: done.
		{Respond: []scripted.Event{{Text: "acknowledged"}, {Finish: "end_turn"}}},
	}}
	l, cap := routedSpawnLoop(t, parentMain, t.TempDir(),
		scripted.RoutePair{Key: "ACK-THE-PLAN", Fixture: childFix},
		scripted.RoutePair{Key: "acknowledged", Fixture: afterReceipt})

	res, err := l.Run(context.Background(), "coordinate")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}

	// Blackboard tools were advertised at the root.
	var names []string
	requests := cap.Requests()
	for _, td := range requests[0].Tools {
		names = append(names, td.Name)
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "publish_note") || !strings.Contains(joined, "read_notes") {
		t.Errorf("blackboard tools not advertised: %v", names)
	}

	// The child's journaled read saw the parent's note (durable influence).
	childEvents, err := store.ReadEvents(filepath.Join(l.Store.Dir(), "sub", "s1-a1"))
	if err != nil {
		t.Fatal(err)
	}
	childFold, err := state.Fold(childEvents)
	if err != nil {
		t.Fatal(err)
	}
	c1 := childFold.Conversation.ToolResults["c1"]
	if c1.IsError || !strings.Contains(string(c1.Result), "focus on the API layer") {
		t.Errorf("child read = %+v, want the parent's note", c1)
	}

	// The parent's read-back saw both notes, in publish order, with authors.
	fold, err := state.Fold(func() []event.Envelope {
		es, _ := store.ReadEvents(l.Store.Dir())
		return es
	}())
	if err != nil {
		t.Fatal(err)
	}
	p2 := fold.Conversation.ToolResults["p2"]
	body := string(p2.Result)
	if p2.IsError || !strings.Contains(body, "focus on the API layer") ||
		!strings.Contains(body, "ack: API layer it is") {
		t.Errorf("parent read-back = %s", body)
	}
	if !strings.Contains(body, `"from":"lead"`) || !strings.Contains(body, `"from":"summarizer"`) {
		t.Errorf("authors missing: %s", body)
	}
	if strings.Index(body, "focus on") > strings.Index(body, "ack:") {
		t.Errorf("publish order lost: %s", body)
	}
}

// S5.4: without a board (a run outside any collaboration face), the
// blackboard tools are not advertised; a fabricated call is model-visible.
func TestBlackboardAbsent(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}
	l, cap := spawnLoop(t, fix, t.TempDir())
	l.Spec.Agents = nil // no collaboration
	if _, err := l.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	requests := cap.Requests()
	for _, td := range requests[0].Tools {
		if td.Name == "publish_note" || td.Name == "read_notes" || td.Name == "handoff_agent" {
			t.Errorf("collaboration tool %s advertised without a whitelist", td.Name)
		}
	}
}
