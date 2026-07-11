package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// TestRevokedInputSkippedOnResume proves the §2 rev1 replay half (INC-46):
// a revoke behind an unconsumed queued input keeps suppressing it across a
// restart — the input is consumed AS REVOKED (InputRevoked advances the
// high-water, nothing is injected), a later queued input still runs, and a
// LATE revoke (target already consumed) is a silent no-op.
func TestRevokedInputSkippedOnResume(t *testing.T) {
	base := t.TempDir()
	sessDir := filepath.Join(base, "sess")
	root := filepath.Join(base, "ws")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	asst := event.AssistantMessage{GenStep: 1, Message: provider.Message{Role: provider.RoleAssistant,
		Parts: []provider.Part{{Kind: provider.PartText, Text: "答一"}}}}
	appendSynthetic(t, es, []struct {
		typ     string
		payload any
	}{
		{event.TypeSessionStarted, &event.SessionStarted{SpecName: "t",
			SubStateVersions: state.SubStateVersions()}},
		{event.TypeInputReceived, &event.InputReceived{Text: "问一", Source: "user", DeliverySeq: 1}},
		{event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}},
		{event.TypeAssistantMessage, &asst},
		{event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitInput}},
	})
	_ = es.Close()

	// Durable command log: seq1 consumed; seq2 queued then WITHDRAWN; seq3
	// queued and staying; plus a LATE revoke aimed at the consumed seq1.
	mustAppend := func(cmd protocol.SessionCommand) {
		t.Helper()
		if _, err := store.AppendCommand(sessDir, cmd); err != nil {
			t.Fatal(err)
		}
	}
	mustAppend(protocol.SessionCommand{Kind: protocol.CommandInput,
		CommandRef: protocol.CommandRef{CommandID: "cmd-1"},
		Input:      &protocol.UserInput{Text: "问一", CommandID: "cmd-1"}})
	mustAppend(protocol.SessionCommand{Kind: protocol.CommandInput,
		CommandRef: protocol.CommandRef{CommandID: "cmd-2"},
		Input:      &protocol.UserInput{Text: "要撤回的消息", CommandID: "cmd-2"}})
	mustAppend(protocol.SessionCommand{Kind: protocol.CommandInput,
		CommandRef: protocol.CommandRef{CommandID: "cmd-3"},
		Input:      &protocol.UserInput{Text: "留下的追问", CommandID: "cmd-3"}})
	mustAppend(protocol.SessionCommand{Kind: protocol.CommandRevoke,
		CommandRef: protocol.CommandRef{CommandID: "rv-2"},
		Revoke:     &protocol.Revoke{TargetCommandID: "cmd-2"}})
	mustAppend(protocol.SessionCommand{Kind: protocol.CommandRevoke,
		CommandRef: protocol.CommandRef{CommandID: "rv-late"},
		Revoke:     &protocol.Revoke{TargetCommandID: "cmd-1"}})

	es2, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	inputs := make(chan protocol.UserInput)
	close(inputs)
	l := &Loop{
		Spec: inDoubtSpec(),
		Provider: scripted.New(scripted.Fixture{Steps: []scripted.Step{
			{Expect: scripted.Expect{LastMessageContains: "留下的追问"},
				Respond: []scripted.Event{{Text: "答三"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
		}}),
		Exec: &tool.Executor{WS: ws}, Store: es2, SessionID: "rvk",
		UserInputs: inputs,
	}
	if _, err := l.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = es2.Close()

	events, err := store.ReadEvents(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	// The withdrawn text must never appear anywhere in the conversation.
	for _, m := range s.Conversation.Messages {
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "要撤回的消息") {
				t.Fatalf("revoked input leaked into the conversation: %q", p.Text)
			}
		}
	}
	// Exactly ONE InputRevoked (the late revoke at cmd-1 no-ops), carrying
	// cmd-2's delivery seq; the high-water covers every delivery.
	var revokedEvents []*event.InputRevoked
	for _, e := range events {
		if e.Type == event.TypeInputRevoked {
			dec, derr := event.DecodePayload(e)
			if derr != nil {
				t.Fatal(derr)
			}
			revokedEvents = append(revokedEvents, dec.(*event.InputRevoked))
		}
	}
	if len(revokedEvents) != 1 || revokedEvents[0].TargetCommandID != "cmd-2" || revokedEvents[0].DeliverySeq != 2 {
		t.Fatalf("want exactly one InputRevoked for cmd-2@2, got %+v", revokedEvents)
	}
	if s.Session.ConsumedInputSeq != 3 {
		t.Fatalf("high-water must cover the surviving input: got %d, want 3", s.Session.ConsumedInputSeq)
	}
	// A second resume replays nothing (convergence): the same fold state.
	es3, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es3.Close() }()
	inputs2 := make(chan protocol.UserInput)
	close(inputs2)
	l2 := &Loop{Spec: inDoubtSpec(),
		Provider: scripted.New(scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "ack2"}, {Finish: "end_turn"}}},
		}}),
		Exec: &tool.Executor{WS: ws}, Store: es3, SessionID: "rvk", UserInputs: inputs2}
	if _, err := l2.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	events2, _ := store.ReadEvents(sessDir)
	count := 0
	for _, e := range events2 {
		if e.Type == event.TypeInputRevoked {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("replay must converge: still one InputRevoked, got %d", count)
	}
}

// TestLiveRevokeConsumesQueuedInput proves the live half: a revoke folded
// into the set before the input is consumed makes journalInput record
// InputRevoked instead of injecting the message.
func TestLiveRevokeConsumesQueuedInput(t *testing.T) {
	base := t.TempDir()
	es, err := store.OpenEventStore(filepath.Join(base, "sess"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()
	ws, err := workspace.New(base)
	if err != nil {
		t.Fatal(err)
	}
	revokes := make(chan protocol.Revoke, 1)
	revokes <- protocol.Revoke{TargetCommandID: "cmd-x"}
	l := &Loop{Exec: &tool.Executor{WS: ws}, Store: es, SessionID: "lv", Revokes: revokes}

	var journaled []event.Envelope
	appendE := func(typ string, payload any) (event.Envelope, error) {
		env, err := event.New(typ, payload)
		if err == nil {
			journaled = append(journaled, env)
		}
		return env, err
	}
	ds := &driveState{}
	if err := l.journalInput(ds, appendE, protocol.UserInput{
		Text: "撤了我", CommandID: "cmd-x", DeliverySeq: 5}); err != nil {
		t.Fatal(err)
	}
	if len(journaled) != 1 || journaled[0].Type != event.TypeInputRevoked {
		t.Fatalf("want a single InputRevoked, got %+v", journaled)
	}
	// An unrevoked input journals normally — via commandAppender (it stamps
	// the command receipt), i.e. through the REAL store, not our collector.
	if err := l.journalInput(ds, appendE, protocol.UserInput{
		Text: "正常消息", CommandID: "cmd-y", DeliverySeq: 6}); err != nil {
		t.Fatal(err)
	}
	stored, err := store.ReadEvents(filepath.Join(base, "sess"))
	if err != nil {
		t.Fatal(err)
	}
	sawReceived := false
	for _, e := range stored {
		if e.Type == event.TypeInputReceived {
			sawReceived = true
		}
		if e.Type == event.TypeInputRevoked {
			t.Fatalf("the revoked event must have gone through the caller's appender, not the store")
		}
	}
	if !sawReceived {
		t.Fatal("unrevoked input must journal InputReceived through the command appender")
	}
}
