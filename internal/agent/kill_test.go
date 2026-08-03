package agent

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/kill"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// waitForKillable spins until the registry publishes id, so the kill lands
// while the call is genuinely in flight rather than racing its launch.
func waitForKillable(t *testing.T, r *kill.Registry, id string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		for _, tgt := range r.List() {
			if tgt.ID == id {
				return
			}
		}
		runtime.Gosched()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s never became killable", id)
}

func findEvent(evs []event.Envelope, typ string) (event.Envelope, bool) {
	for _, e := range evs {
		if e.Type == typ {
			return e, true
		}
	}
	return event.Envelope{}, false
}

// The load-bearing behaviour, end to end through a real batch and a real
// process group: killing ONE in-flight tool call cuts that call and nothing
// else — its sibling completes normally, the turn carries on to the next
// generation step, and the model is told the call was killed rather than
// that it was interrupted. Nothing about the SESSION is recorded: no mark,
// so a scheduled session killed mid-run still starts on its next tick.
func TestKillOneToolCallLeavesTheTurnRunning(t *testing.T) {
	root := t.TempDir()
	fix := scripted.Fixture{Steps: []scripted.Step{
		{
			Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{Name: "bash", Args: map[string]any{
					"command": "sleep 120"}}},
				{ToolCall: &scripted.ToolCallEvent{Name: "bash", Args: map[string]any{
					"command": "echo sibling-ran"}}},
				{Finish: "tool_use"},
			},
		},
		{
			Respond: []scripted.Event{{Text: "noted"}, {Finish: "end_turn"}},
		},
	}}
	l := testLoop(t, fix, root)
	l.Kills = kill.NewRegistry()

	go func() {
		// call_1_0 is the sleep: the scripted provider numbers calls by
		// turn and position.
		waitForKillable(t, l.Kills, "call_1_0")
		l.Kills.Kill("call_1_0")
	}()

	if _, err := l.Run(context.Background(), "run two commands"); err != nil {
		t.Fatalf("a killed tool call ended the whole run: %v", err)
	}

	evs, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	cancelled, ok := findEvent(evs, event.TypeActivityCancelled)
	if !ok {
		t.Fatal("no ActivityCancelled: the kill never reached the call")
	}
	var got event.ActivityCancelled
	if err := json.Unmarshal(cancelled.Payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.ActivityID != "tool-call_1_0" {
		t.Fatalf("cancelled %s; want the killed call", got.ActivityID)
	}
	if got.Reason != "killed" {
		t.Fatalf("cancel reason %q; want %q — the model must not read a targeted kill as a steering interrupt", got.Reason, "killed")
	}
	// The sibling ran to completion: a kill is not a batch-wide interrupt.
	var sawSibling bool
	for _, e := range evs {
		if e.Type != event.TypeActivityCompleted {
			continue
		}
		var done event.ActivityCompleted
		if json.Unmarshal(e.Payload, &done) == nil && done.ActivityID == "tool-call_1_1" {
			sawSibling = true
		}
	}
	if !sawSibling {
		t.Fatal("the sibling call did not complete: the kill cut more than its target")
	}
	// And the turn continued: a second generation step only happens if the
	// loop treated the killed call as an ordinary tool outcome.
	var genSteps int
	for _, e := range evs {
		if e.Type == event.TypeGenerationStarted {
			genSteps++
		}
	}
	if genSteps < 2 {
		t.Fatalf("generation steps = %d; want the turn to continue past the kill", genSteps)
	}
	// No lifecycle state, ever: that is what lets a killed scheduled session
	// run again on its next tick instead of being gated by a mark.
	if _, marked := findEvent(evs, event.TypeSessionClosed); marked {
		t.Fatal("a kill wrote a session mark; kills must leave no durable state")
	}
}

// Call ids repeat across the tree: a child's first tool call and its
// parent's first call are both "call_1_0". Keyed on the bare id, the
// child's call displaces the parent's handle FOR that child — and then
// "stop this agent" stops only whatever command it happened to be running
// while the agent itself carries on. Caught on the real stack (QA
// 2026-08-02), so the scoping is pinned here.
func TestChildCallIDDoesNotDisplaceParentHandle(t *testing.T) {
	root := &Loop{SessionID: "root", Depth: 0}
	child := &Loop{SessionID: "root-sub-call_1_0-a1", Depth: 1}
	if got := root.killID("call_1_0"); got != "call_1_0" {
		t.Fatalf("root killID = %q; want the bare id `ar ps` hands back", got)
	}
	if got := child.killID("call_1_0"); got == root.killID("call_1_0") {
		t.Fatalf("child killID collides with the parent's handle (%q)", got)
	}
	// And the two really do coexist in one table.
	reg := kill.NewRegistry()
	root.Kills, child.Kills = reg, reg
	_, cancelHandle := context.WithCancelCause(context.Background())
	_, cancelChildCall := context.WithCancelCause(context.Background())
	defer cancelHandle(nil)
	defer cancelChildCall(nil)
	reg.Register(kill.Target{
		ID: root.killID("call_1_0"), Kind: "agent", Name: "worker",
		Session: child.SessionID,
	}, cancelHandle)
	child.registerKill("call_1_0", "tool", "bash", cancelChildCall)

	if got := len(reg.List()); got != 2 {
		t.Fatalf("registry holds %d entries; want the handle AND the child's call", got)
	}
	hits := reg.KillSession(child.SessionID)
	if len(hits) != 2 {
		t.Fatalf("kill --agent hit %d units (%+v); want the agent and its in-flight call", len(hits), hits)
	}
}

// The model is told which of the two happened, in its own tool result.
func TestKilledCallRendersKilledToModel(t *testing.T) {
	evs := []event.Envelope{}
	appendEvent := func(typ string, payload any) {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		evs = append(evs, event.Envelope{Type: typ, Payload: raw})
	}
	appendEvent(event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: "tool-c1", Kind: event.KindTool, Name: "bash", CallID: "c1", Attempt: 1,
	})
	appendEvent(event.TypeActivityCancelled, &event.ActivityCancelled{
		ActivityID: "tool-c1", Reason: "killed",
	})
	s, err := state.Fold(evs)
	if err != nil {
		t.Fatal(err)
	}
	tr, ok := s.Conversation.ToolResults["c1"]
	if !ok {
		t.Fatal("the killed call never resolved for the model")
	}
	if !strings.Contains(string(tr.Result), "killed by user") {
		t.Fatalf("tool result = %s; want it to say the call was killed", tr.Result)
	}
}
