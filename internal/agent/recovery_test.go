package agent

import (
	"context"
	"encoding/json"
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

// appendSynthetic journals a crash-shaped prefix: each pair lands verbatim.
func appendSynthetic(t *testing.T, es *store.EventStore, pairs []struct {
	typ     string
	payload any
}) {
	t.Helper()
	for _, pair := range pairs {
		env, err := event.New(pair.typ, pair.payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := es.Append(env); err != nil {
			t.Fatal(err)
		}
	}
}

// v2 M5.1 (C10b, conversational): a session that died mid-bash self-heals
// on resume — the in-doubt command is NOT re-run, it renders as an
// interrupted-by-crash error the model reacts to, and the conversation
// continues in the same session.
func TestConversationalCrashRendersInDoubtAndContinues(t *testing.T) {
	base := t.TempDir()
	sessDir := filepath.Join(base, "sess")
	root := filepath.Join(base, "ws")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	asst := event.AssistantMessage{Turn: 1,
		Message: providerAssistantToolCall("call_1_0", "bash", `{"command":"sleep 30"}`)}
	appendSynthetic(t, es, []struct {
		typ     string
		payload any
	}{
		{event.TypeRunStarted, &event.RunStarted{SpecName: "t", Conversational: true,
			SubStateVersions: state.SubStateVersions()}},
		{event.TypeInputReceived, &event.InputReceived{Text: "跑个慢命令", Source: "user"}},
		{event.TypeTurnStarted, &event.TurnStarted{Turn: 1}},
		{event.TypeAssistantMessage, &asst},
		{event.TypeActivityStarted, &event.ActivityStarted{ActivityID: "tool-call_1_0",
			Kind: event.KindTool, Name: "bash", CallID: "call_1_0", Attempt: 1}},
	})
	_ = es.Close() // kill -9

	es2, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es2.Close() }()
	inputs := make(chan protocol.UserInput, 1)
	inputs <- protocol.UserInput{Text: "刚才的命令什么状态?"}
	close(inputs)
	prov := scripted.New(scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "命令被崩溃打断了,没有重跑"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "如上,它被打断了"}, {Finish: "end_turn"}}},
	}})
	l := &Loop{
		Spec:           inDoubtSpec(),
		Provider:       prov,
		Exec:           &tool.Executor{WS: ws},
		Store:          es2,
		SessionID:      "crash-chat",
		Conversational: true,
		UserInputs:     inputs,
	}
	res, err := l.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" {
		t.Fatalf("res = %+v, want closed (session continued after crash)", res)
	}

	evs, _ := store.ReadEvents(sessDir)
	var crashRendered, bashStarts int
	for _, e := range evs {
		if e.Type == event.TypeActivityFailed && strings.Contains(string(e.Payload), "interrupted by crash") {
			crashRendered++
		}
		if e.Type == event.TypeActivityStarted && strings.Contains(string(e.Payload), `"name":"bash"`) {
			bashStarts++
		}
	}
	if crashRendered != 1 {
		t.Errorf("crash-rendered failures = %d, want 1", crashRendered)
	}
	if bashStarts != 1 {
		t.Errorf("bash ActivityStarted = %d — the in-doubt command must NOT re-run", bashStarts)
	}
}

// crashParentPrefix journals a parent that died with one background spawn
// in flight (SpawnRequested + ActivityStarted{Background}, no terminal).
func crashParentPrefix(t *testing.T, es *store.EventStore, callID string) {
	t.Helper()
	asst := event.AssistantMessage{Turn: 1, Message: providerAssistantToolCall(
		callID, "spawn_agent", `{"agent":"worker","task":"investigate","background":true}`)}
	appendSynthetic(t, es, []struct {
		typ     string
		payload any
	}{
		{event.TypeRunStarted, &event.RunStarted{SpecName: "lead", Conversational: true,
			SubStateVersions: state.SubStateVersions()}},
		{event.TypeInputReceived, &event.InputReceived{Text: "去调查", Source: "user"}},
		{event.TypeTurnStarted, &event.TurnStarted{Turn: 1}},
		{event.TypeAssistantMessage, &asst},
		{event.TypeSpawnRequested, &event.SpawnRequested{CallID: callID, Agent: "worker",
			Task: "investigate", ChildSession: "p-sub-" + callID + "-a1", Depth: 1}},
		{event.TypeActivityStarted, &event.ActivityStarted{ActivityID: "tool-" + callID,
			Kind: event.KindTool, Name: "spawn_agent", CallID: callID, Attempt: 1,
			Background: true, Args: json.RawMessage(`{"agent":"worker","task":"investigate","background":true}`)}},
	})
}

func resumeCrashedParent(t *testing.T, sessDir, root string) (RunResult, []event.Envelope) {
	t.Helper()
	es2, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es2.Close() }()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	inputs := make(chan protocol.UserInput)
	close(inputs)
	prov := scripted.New(scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "收到子 agent 的结局"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
	}})
	l := &Loop{
		Spec:           inDoubtSpec(),
		Provider:       prov,
		Exec:           &tool.Executor{WS: ws},
		Store:          es2,
		SessionID:      "p",
		Conversational: true,
		UserInputs:     inputs,
	}
	res, err := l.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	evs, _ := store.ReadEvents(sessDir)
	return res, evs
}

// v2 M5.1 (C10c): a child that FINISHED before the crash delivers its real
// receipt on resume — settle-from-child-fold, no work lost.
func TestCrashSettleSpawnFromChildFold(t *testing.T) {
	base := t.TempDir()
	sessDir := filepath.Join(base, "sess")
	root := filepath.Join(base, "ws")
	es, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	crashParentPrefix(t, es, "c1")
	_ = es.Close() // kill -9

	// The child ran to completion in its own journal before the crash.
	childDir := filepath.Join(sessDir, "sub", "c1-a1")
	ces, err := store.OpenEventStore(childDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "cws"), 0o755); err != nil {
		t.Fatal(err)
	}
	cws, err := workspace.New(filepath.Join(base, "cws"))
	if err != nil {
		t.Fatal(err)
	}
	child := &Loop{
		Spec: &AgentSpec{Name: "worker", Model: ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
			SystemPrompt: "w", Tools: []string{"read_file"}, MaxTurns: 3},
		Provider: scripted.New(scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "ALPHA finding: all clear"}, {Finish: "end_turn"}}},
		}}),
		Exec: &tool.Executor{WS: cws}, Store: ces, SessionID: "p-sub-c1-a1",
	}
	if _, err := child.Run(context.Background(), "investigate"); err != nil {
		t.Fatal(err)
	}
	_ = ces.Close()

	res, evs := resumeCrashedParent(t, sessDir, root)
	if res.Reason != "closed" {
		t.Fatalf("res = %+v", res)
	}
	var subReason string
	var receiptOK bool
	for _, e := range evs {
		if e.Type == event.TypeSubagentCompleted {
			dec, _ := event.DecodePayload(e)
			subReason = dec.(*event.SubagentCompleted).Reason
		}
		if e.Type == event.TypeActivityCompleted && strings.Contains(string(e.Payload), "ALPHA finding") {
			receiptOK = true
		}
	}
	if subReason != "completed" {
		t.Errorf("subagent reason = %q, want completed (child finished pre-crash)", subReason)
	}
	if !receiptOK {
		t.Error("child report never delivered to the parent")
	}
}

// v2 M5.1 (C10c): a child that died WITH the process settles as a crash
// cancellation carrying its real spend; the parent continues.
func TestCrashSettleSpawnChildDiedToo(t *testing.T) {
	base := t.TempDir()
	sessDir := filepath.Join(base, "sess")
	root := filepath.Join(base, "ws")
	es, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	crashParentPrefix(t, es, "c2")
	_ = es.Close()

	// The child journaled a start but never a terminal (it died too).
	childDir := filepath.Join(sessDir, "sub", "c2-a1")
	ces, err := store.OpenEventStore(childDir)
	if err != nil {
		t.Fatal(err)
	}
	appendSynthetic(t, ces, []struct {
		typ     string
		payload any
	}{
		{event.TypeRunStarted, &event.RunStarted{SpecName: "worker",
			SubStateVersions: state.SubStateVersions()}},
		{event.TypeInputReceived, &event.InputReceived{Text: "investigate", Source: "cli"}},
	})
	_ = ces.Close()

	res, evs := resumeCrashedParent(t, sessDir, root)
	if res.Reason != "closed" {
		t.Fatalf("res = %+v", res)
	}
	var subReason string
	var cancelled bool
	for _, e := range evs {
		if e.Type == event.TypeSubagentCompleted {
			dec, _ := event.DecodePayload(e)
			subReason = dec.(*event.SubagentCompleted).Reason
		}
		if e.Type == event.TypeActivityCancelled && strings.Contains(string(e.Payload), "interrupted by crash") {
			cancelled = true
		}
	}
	if subReason != "crash" {
		t.Errorf("subagent reason = %q, want crash", subReason)
	}
	if !cancelled {
		t.Error("no crash-cancellation receipt for the dead child")
	}
}

// v2 收口 review: a park that lost its WaitingResolved to a crash (input
// durable, resolution not) must NOT re-block on resume — the pending input
// resolves the stale park and gets its turn immediately.
func TestResumedParkSeesPendingInput(t *testing.T) {
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
	asst := event.AssistantMessage{Turn: 1, Message: provider.Message{Role: provider.RoleAssistant,
		Parts: []provider.Part{{Kind: provider.PartText, Text: "答一"}}}}
	appendSynthetic(t, es, []struct {
		typ     string
		payload any
	}{
		{event.TypeRunStarted, &event.RunStarted{SpecName: "t", Conversational: true,
			SubStateVersions: state.SubStateVersions()}},
		{event.TypeInputReceived, &event.InputReceived{Text: "问一", Source: "user"}},
		{event.TypeTurnStarted, &event.TurnStarted{Turn: 1}},
		{event.TypeAssistantMessage, &asst},
		{event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitInput}},
		// The input landed durably; the crash hit before WaitingResolved.
		{event.TypeInputReceived, &event.InputReceived{Text: "问二", Source: "user"}},
	})
	_ = es.Close()

	es2, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es2.Close() }()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	inputs := make(chan protocol.UserInput)
	close(inputs)
	l := &Loop{
		Spec: inDoubtSpec(),
		Provider: scripted.New(scripted.Fixture{Steps: []scripted.Step{
			{Expect: scripted.Expect{LastMessageContains: "问二"},
				Respond: []scripted.Event{{Text: "答二"}, {Finish: "end_turn"}}},
		}}),
		Exec: &tool.Executor{WS: ws}, Store: es2, SessionID: "pend",
		Conversational: true, UserInputs: inputs,
	}
	res, err := l.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" {
		t.Fatalf("res = %+v", res)
	}
	evs, _ := store.ReadEvents(sessDir)
	var answered, resolvedPending bool
	for _, e := range evs {
		if e.Type == event.TypeAssistantMessage && strings.Contains(string(e.Payload), "答二") {
			answered = true
		}
		if e.Type == event.TypeWaitingResolved && strings.Contains(string(e.Payload), "pending_input") {
			resolvedPending = true
		}
	}
	if !answered {
		t.Error("pending input never answered — resumed park blocked over it")
	}
	if !resolvedPending {
		t.Error("stale park not resolved as pending_input")
	}
}

// v2 收口 (铁律 "崩溃不丢输入"): mailbox entries the journal never consumed
// replay on resume, exactly once — consumed ones never duplicate.
func TestMailboxReplayOnResume(t *testing.T) {
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
	asst := event.AssistantMessage{Turn: 1, Message: provider.Message{Role: provider.RoleAssistant,
		Parts: []provider.Part{{Kind: provider.PartText, Text: "答一"}}}}
	appendSynthetic(t, es, []struct {
		typ     string
		payload any
	}{
		{event.TypeRunStarted, &event.RunStarted{SpecName: "t", Conversational: true,
			SubStateVersions: state.SubStateVersions()}},
		{event.TypeInputReceived, &event.InputReceived{Text: "问一", Source: "user", DeliverySeq: 1}},
		{event.TypeTurnStarted, &event.TurnStarted{Turn: 1}},
		{event.TypeAssistantMessage, &asst},
		{event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitInput}},
	})
	_ = es.Close()
	// The mailbox holds seq 1 (consumed) and seq 2 (acked, never journaled —
	// the crash ate it between enqueue and consume).
	if _, err := store.AppendInbox(sessDir, protocol.UserInput{Text: "问一"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendInbox(sessDir, protocol.UserInput{Text: "排队的问二"}); err != nil {
		t.Fatal(err)
	}

	es2, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es2.Close() }()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	// The channel ALSO carries a duplicate of seq 2 (the delivery that was
	// in flight when the process died raced the mailbox replay): the seq
	// dedup must journal it exactly once.
	inputs := make(chan protocol.UserInput, 1)
	inputs <- protocol.UserInput{Text: "排队的问二", DeliverySeq: 2}
	close(inputs)
	l := &Loop{
		Spec: inDoubtSpec(),
		Provider: scripted.New(scripted.Fixture{Steps: []scripted.Step{
			{Expect: scripted.Expect{LastMessageContains: "排队的问二"},
				Respond: []scripted.Event{{Text: "答二:补上了"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
		}}),
		Exec: &tool.Executor{WS: ws}, Store: es2, SessionID: "mbox",
		Conversational: true, UserInputs: inputs,
	}
	res, err := l.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" {
		t.Fatalf("res = %+v", res)
	}
	evs, _ := store.ReadEvents(sessDir)
	q1, q2, answered := 0, 0, false
	for _, e := range evs {
		if e.Type == event.TypeInputReceived {
			if strings.Contains(string(e.Payload), "问一") {
				q1++
			}
			if strings.Contains(string(e.Payload), "排队的问二") {
				q2++
			}
		}
		if e.Type == event.TypeAssistantMessage && strings.Contains(string(e.Payload), "补上了") {
			answered = true
		}
	}
	if q1 != 1 {
		t.Errorf("consumed mailbox entry journaled %d times, want 1 (no duplicate replay)", q1)
	}
	if q2 != 1 || !answered {
		t.Errorf("unconsumed mailbox entry: journaled %d times, answered=%v — 输入丢了", q2, answered)
	}
}

// v2 收口 security review: a malformed CallID journaled in the crash window
// must never steer the crash-settle read outside sub/ — it renders as a
// crash failure and any decoy journal at the escape target stays unread.
func TestCrashSettleSpawnMalformedCallID(t *testing.T) {
	base := t.TempDir()
	sessDir := filepath.Join(base, "sess")
	root := filepath.Join(base, "ws")
	es, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	crashParentPrefix(t, es, "../evil")
	_ = es.Close()
	// A decoy "child journal" at the escape target <sess>/evil-a1.
	decoy, err := store.OpenEventStore(filepath.Join(sessDir, "evil-a1"))
	if err != nil {
		t.Fatal(err)
	}
	appendSynthetic(t, decoy, []struct {
		typ     string
		payload any
	}{
		{event.TypeRunStarted, &event.RunStarted{SpecName: "decoy",
			SubStateVersions: state.SubStateVersions()}},
		{event.TypeInputReceived, &event.InputReceived{Text: "secret", Source: "cli"}},
		{event.TypeRunEnded, &event.RunEnded{Reason: "completed", Turns: 1}},
	})
	_ = decoy.Close()

	res, evs := resumeCrashedParent(t, sessDir, root)
	if res.Reason != "closed" {
		t.Fatalf("res = %+v", res)
	}
	for _, e := range evs {
		if e.Type == event.TypeSubagentCompleted {
			t.Error("malformed CallID produced a subagent receipt — decoy journal was read")
		}
		if strings.Contains(string(e.Payload), "secret") {
			t.Error("decoy journal content leaked into the session")
		}
	}
	var crashRendered bool
	for _, e := range evs {
		if e.Type == event.TypeActivityFailed && strings.Contains(string(e.Payload), "interrupted by crash") {
			crashRendered = true
		}
	}
	if !crashRendered {
		t.Error("malformed-CallID spawn not rendered as a crash failure")
	}
}
