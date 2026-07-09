package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// closeWhen closes the inputs channel once cond(parent events) holds — the
// scripted-twin idiom for "the user leaves when the collaboration is done".
func closeWhen(l *Loop, inputs chan protocol.UserInput, cond func([]event.Envelope) bool) {
	go func() {
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			evs, _ := store.ReadEvents(l.Store.Dir())
			if cond(evs) {
				close(inputs)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		close(inputs)
	}()
}

func countType(evs []event.Envelope, typ string) int {
	n := 0
	for _, e := range evs {
		if e.Type == typ {
			n++
		}
	}
	return n
}

// INC-12.1/12.2 (工作纸闸门 A-2): the parent messages a QUIESCENT child —
// ChildRevived re-hosts it in place (same journal, ONE SessionStarted,
// context continues), the message re-enters through the durable inbox, and
// the child's second quiescence delivers a SECOND SubagentCompleted for the
// same call. The send_message target here is the HANDLE (resolution path).
func TestSendMessageRevivesQuiescentChild(t *testing.T) {
	router := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "a", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "task": "investigate ALPHA"}}},
				{Finish: "tool_use"},
			}},
			// Woken by receipt 1: message the (now idle) child by HANDLE.
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "m1", Name: "send_message",
					Args: map[string]any{"to": "a", "text": "please also check BETA"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "ALPHA", Fixture: scripted.Fixture{Steps: []scripted.Step{
			// Round 1: report and go quiescent.
			{Respond: []scripted.Event{{Text: "ALPHA report: v1 done"}, {Finish: "end_turn"}}},
			// Round 2 (revived by the parent's message): acknowledge it.
			{Respond: []scripted.Event{{Text: "BETA follow-up: also done"}, {Finish: "end_turn"}}},
		}}},
	)
	l := bgSpawnLoop(t, router, []string{"worker"})
	inputs := make(chan protocol.UserInput)
	l.UserInputs = inputs
	closeWhen(l, inputs, func(evs []event.Envelope) bool {
		return countType(evs, event.TypeSubagentCompleted) >= 2
	})

	res, err := l.Run(context.Background(), "orchestrate ALPHA, then follow up")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" {
		t.Fatalf("res = %+v, want closed", res)
	}

	evs, _ := store.ReadEvents(l.Store.Dir())
	if n := countType(evs, event.TypeSubagentCompleted); n != 2 {
		t.Fatalf("SubagentCompleted = %d, want 2 (original + revived round)", n)
	}
	if n := countType(evs, event.TypeChildRevived); n != 1 {
		t.Fatalf("ChildRevived = %d, want 1", n)
	}
	// Both receipts name the SAME call — the handle survives the revive.
	for _, e := range evs {
		if e.Type == event.TypeSubagentCompleted {
			dec, _ := event.DecodePayload(e)
			if sc := dec.(*event.SubagentCompleted); sc.CallID != "a" {
				t.Errorf("receipt call = %q, want a", sc.CallID)
			}
		}
	}
	// Context continuity: the child journal has exactly ONE SessionStarted
	// and carries the delivered message as an agent-source input.
	childEvs, err := store.ReadEvents(l.Store.Dir() + "/sub/a-a1")
	if err != nil {
		t.Fatal(err)
	}
	if n := countType(childEvs, event.TypeSessionStarted); n != 1 {
		t.Fatalf("child SessionStarted = %d, want 1 (context continues, no fresh run)", n)
	}
	var sawMsg bool
	for _, e := range childEvs {
		if e.Type == event.TypeInputReceived && strings.Contains(string(e.Payload), `"source":"agent"`) &&
			strings.Contains(string(e.Payload), "please also check BETA") {
			sawMsg = true
		}
	}
	if !sawMsg {
		t.Fatal("child journal lacks the delivered agent message")
	}
	// The child fold sees both rounds in ONE conversation.
	cf, err := state.Fold(childEvs)
	if err != nil {
		t.Fatal(err)
	}
	var sawV1, sawV2 bool
	for _, m := range cf.Conversation.Messages {
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "v1 done") {
				sawV1 = true
			}
			if strings.Contains(p.Text, "also done") {
				sawV2 = true
			}
		}
	}
	if !sawV1 || !sawV2 {
		t.Errorf("child context continuity broken: v1=%v v2=%v", sawV1, sawV2)
	}
	// The parent got BOTH reports as user-role messages, and its handle set
	// drained (the revive settled).
	pf, err := state.Fold(evs)
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.Handles) != 0 {
		t.Errorf("parent handles not drained: %+v", pf.Handles)
	}
	var sawRevivedReport bool
	for _, m := range pf.Conversation.Messages {
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "also done") {
				sawRevivedReport = true
			}
		}
	}
	if !sawRevivedReport {
		t.Error("revived round's report never reached the parent conversation")
	}
}

// INC-12.1 (工作纸闸门 A-3): a running child messages its PARENT mid-work —
// the parent's idle wakes on the tree message and the text (with the sender
// prefix) enters the parent conversation before the child's receipt.
func TestSendMessageChildToParent(t *testing.T) {
	router := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "g", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "task": "investigate GAMMA"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "noted the progress"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "GAMMA", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "p1", Name: "send_message",
					Args: map[string]any{"to": "parent", "text": "progress: half way"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "GAMMA report: done"}, {Finish: "end_turn"}}},
		}}},
	)
	l := bgSpawnLoop(t, router, []string{"worker"})
	inputs := make(chan protocol.UserInput)
	l.UserInputs = inputs
	closeWhen(l, inputs, func(evs []event.Envelope) bool {
		return countType(evs, event.TypeSubagentCompleted) >= 1
	})

	if _, err := l.Run(context.Background(), "orchestrate GAMMA with progress reports"); err != nil {
		t.Fatal(err)
	}

	evs, _ := store.ReadEvents(l.Store.Dir())
	var msgSeq, receiptSeq int64
	for _, e := range evs {
		if e.Type == event.TypeInputReceived && strings.Contains(string(e.Payload), `"source":"agent"`) &&
			strings.Contains(string(e.Payload), "progress: half way") && msgSeq == 0 {
			msgSeq = e.Seq
		}
		if e.Type == event.TypeSubagentCompleted && receiptSeq == 0 {
			receiptSeq = e.Seq
		}
	}
	if msgSeq == 0 {
		t.Fatal("parent journal lacks the child's mid-work message")
	}
	if receiptSeq == 0 || msgSeq > receiptSeq {
		t.Fatalf("message (seq %d) should precede the receipt (seq %d) — it was mid-work", msgSeq, receiptSeq)
	}
	// The sender prefix reached the parent MODEL (weak-typed Input).
	pf, err := state.Fold(evs)
	if err != nil {
		t.Fatal(err)
	}
	var sawPrefixed bool
	for _, m := range pf.Conversation.Messages {
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "[message from worker") && strings.Contains(p.Text, "progress: half way") {
				sawPrefixed = true
			}
		}
	}
	if !sawPrefixed {
		t.Error("prefixed message did not reach the parent conversation")
	}
}

// INC-12.1: target guards — messaging yourself, an off-tree id, and an
// unknown handle each resolve as model-visible errors; nothing is delivered.
func TestSendMessageTargetGuards(t *testing.T) {
	router := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "m1", Name: "send_message",
					Args: map[string]any{"to": "lead", "text": "hi me"}}},
				{ToolCall: &scripted.ToolCallEvent{CallID: "m2", Name: "send_message",
					Args: map[string]any{"to": "other-tree-sub-x-a1", "text": "hi outsider"}}},
				{ToolCall: &scripted.ToolCallEvent{CallID: "m3", Name: "send_message",
					Args: map[string]any{"to": "parent", "text": "hi parent"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "saw the errors"}, {Finish: "end_turn"}}},
		}}},
	)
	l := bgSpawnLoop(t, router, []string{"worker"})
	res, err := l.Run(context.Background(), "try bad targets")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}
	evs, _ := store.ReadEvents(l.Store.Dir())
	fold, err := state.Fold(evs)
	if err != nil {
		t.Fatal(err)
	}
	for call, want := range map[string]string{
		"m1": "cannot message yourself",
		"m2": "neither",
		"m3": "no parent",
	} {
		tr, ok := fold.Conversation.ToolResults[call]
		if !ok || !tr.IsError {
			t.Errorf("call %s: want a model-visible error result, got %+v", call, tr)
			continue
		}
		if !strings.Contains(string(tr.Result), want) {
			t.Errorf("call %s error %q lacks %q", call, string(tr.Result), want)
		}
	}
}

// INC-12.2 (工作纸闸门 A-7 变体): mail delivered while NOBODY hosts the tree
// (process down) is picked up by the restart continuation scan — the resumed
// parent revives the child, which consumes the durable inbox tail exactly
// once.
func TestReviveScanAfterRestart(t *testing.T) {
	router := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "d", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "task": "investigate DELTA"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "round one done"}, {Finish: "end_turn"}}},
			// After restart: woken by the revived child's second receipt.
			{Respond: []scripted.Event{{Text: "late mail receipt seen"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "DELTA", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "DELTA report: done"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "late mail handled"}, {Finish: "end_turn"}}},
		}}},
	)
	l := bgSpawnLoop(t, router, []string{"worker"})
	inputs := make(chan protocol.UserInput)
	l.UserInputs = inputs
	closeWhen(l, inputs, func(evs []event.Envelope) bool {
		return countType(evs, event.TypeSubagentCompleted) >= 1
	})
	if _, err := l.Run(context.Background(), "orchestrate DELTA"); err != nil {
		t.Fatal(err)
	}

	// The process is gone (release the store lock as a real exit would).
	// Another process (a sibling tree member's host, or the daemon routing a
	// user send) delivers durable mail to the child.
	dir := l.Store.Dir()
	_ = l.Store.Close()
	childDir := dir + "/sub/d-a1"
	if _, err := store.AppendInbox(childDir, protocol.UserInput{
		Text: "[message from lead (lead)]\nlate follow-up", Source: "agent",
		CommandID: event.NewCommandID(),
	}); err != nil {
		t.Fatal(err)
	}

	// Restart: a fresh loop over the same store resumes; the scan finds the
	// child's unconsumed mail and revives it.
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()
	l2 := &Loop{
		Spec: l.Spec, Provider: l.Provider, Exec: l.Exec, Store: es,
		Clock: l.Clock, SessionID: "lead", SubSpecs: l.SubSpecs,
	}
	inputs2 := make(chan protocol.UserInput)
	l2.UserInputs = inputs2
	closeWhen(l2, inputs2, func(evs []event.Envelope) bool {
		return countType(evs, event.TypeSubagentCompleted) >= 2
	})
	if _, err := l2.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}

	evs, _ := store.ReadEvents(dir)
	if n := countType(evs, event.TypeChildRevived); n != 1 {
		t.Fatalf("ChildRevived = %d, want 1 (restart scan)", n)
	}
	if n := countType(evs, event.TypeSubagentCompleted); n != 2 {
		t.Fatalf("SubagentCompleted = %d, want 2", n)
	}
	childEvs, _ := store.ReadEvents(childDir)
	if n := countType(childEvs, event.TypeSessionStarted); n != 1 {
		t.Fatalf("child SessionStarted = %d, want 1", n)
	}
	// Exactly-once: ONE journaled InputReceived for the late mail.
	late := 0
	for _, e := range childEvs {
		if e.Type == event.TypeInputReceived && strings.Contains(string(e.Payload), "late follow-up") {
			late++
		}
	}
	if late != 1 {
		t.Fatalf("late mail journaled %d times, want exactly once", late)
	}
}

// INC-12.2: a user-killed child never revives for tree mail (裁决二 C) —
// the mail stays durable; only user mail may wake it.
func TestReviveHonorsUserKillMark(t *testing.T) {
	router := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "e", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker", "task": "investigate EPSILON"}}},
				{Finish: "tool_use"},
			}},
			// Woken by receipt 1: just idle (the mark lands meanwhile).
			{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
			// Woken by the steer AFTER the user-kill mark landed: try to wake
			// the killed child.
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "m1", Name: "send_message",
					Args: map[string]any{"to": "e", "text": "wake up please"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "EPSILON", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "EPSILON report: done"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "should not run"}, {Finish: "end_turn"}}},
		}}},
	)
	l := bgSpawnLoop(t, router, []string{"worker"})
	inputs := make(chan protocol.UserInput, 1)
	l.UserInputs = inputs

	// Deterministic order: receipt 1 → user-kill mark lands → steer makes the
	// parent message the killed child → the revive path must refuse.
	childDir := l.Store.Dir() + "/sub/e-a1"
	go func() {
		deadline := time.Now().Add(8 * time.Second)
		marked, steered := false, false
		for time.Now().Before(deadline) {
			evs, _ := store.ReadEvents(l.Store.Dir())
			if !marked && countType(evs, event.TypeSubagentCompleted) >= 1 {
				ces, err := store.OpenEventStore(childDir)
				if err == nil {
					_, _ = ces.Append(event.Envelope{
						Type:    event.TypeSessionClosed,
						Payload: mustJSON(&event.SessionClosed{Reason: "killed", Source: "user"}),
					})
					_ = ces.Close()
					marked = true
				}
			}
			if marked && !steered {
				inputs <- protocol.UserInput{Text: "now send the follow-up message to the worker"}
				steered = true
			}
			if steered {
				for _, e := range evs {
					if e.Type == event.TypeActivityCompleted && strings.Contains(string(e.Payload), "tool-m1") {
						time.Sleep(100 * time.Millisecond) // let any (wrong) revive start
						close(inputs)
						return
					}
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		close(inputs)
	}()

	if _, err := l.Run(context.Background(), "orchestrate EPSILON then message it"); err != nil {
		t.Fatal(err)
	}
	evs, _ := store.ReadEvents(l.Store.Dir())
	if n := countType(evs, event.TypeChildRevived); n != 0 {
		t.Fatalf("ChildRevived = %d, want 0 (user-kill mark honored)", n)
	}
	// The mail is durable, unconsumed — a later USER send may still revive.
	tail, err := store.ReadInbox(childDir, 0)
	if err != nil || len(tail) == 0 {
		t.Fatalf("mail should stay durable in the child inbox: %v (%d)", err, len(tail))
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
