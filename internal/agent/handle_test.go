package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// S6.1 e2e: a background bash pairs with a handle immediately (turn 1
// continues without blocking), its result arrives as a user-role message,
// and the model sees it on a later turn.
func TestBackgroundWorkHandleAndOutcome(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		// GenStep 1: launch a background work, then keep working (text) — proves
		// the handle paired without blocking on the sleep.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "bg1", Name: "bash",
				Args: map[string]any{"command": "sleep 0.2; echo done-work", "background": true}}},
			{Finish: "tool_use"},
		}},
		// GenStep 2: model acknowledges the handle (the work is still running or
		// just finished) and yields with text; the loop then goes idle on the
		// session (waiting for background work) if it hasn't settled.
		{
			Expect:  scripted.Expect{LastMessageContains: "handle"},
			Respond: []scripted.Event{{Text: "started it, waiting"}, {Finish: "end_turn"}},
		},
		// GenStep 3: the background outcome has arrived as a user message; wrap up.
		{
			Expect:  scripted.Expect{LastMessageContains: "done-work"},
			Respond: []scripted.Event{{Text: "work finished, all done"}, {Finish: "end_turn"}},
		},
	}}
	l := testLoop(t, fix, t.TempDir())

	res, err := l.Run(context.Background(), "run a background job")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	// The Started fact is Background; the Completed settles it.
	var sawBgStart, sawComplete bool
	for _, e := range events {
		if e.Type == event.TypeActivityStarted && strings.Contains(string(e.Payload), `"background":true`) {
			sawBgStart = true
		}
		if e.Type == event.TypeActivityCompleted && strings.Contains(string(e.Payload), "tool-bg1") {
			sawComplete = true
		}
	}
	if !sawBgStart || !sawComplete {
		t.Fatalf("bg start=%v complete=%v", sawBgStart, sawComplete)
	}

	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	// The work drained out of the handles sub-state at the end.
	if len(fold.Session.Env) == 0 && len(fold.Handles) != 0 {
		t.Errorf("background handles not drained: %+v", fold.Handles)
	}
	if len(fold.Handles) != 0 {
		t.Errorf("handles sub-state not empty at end: %+v", fold.Handles)
	}
	// The handle paired the call, and the outcome is a user message.
	tr := fold.Conversation.ToolResults["bg1"]
	if !strings.Contains(string(tr.Result), "running") {
		t.Errorf("handle = %s, want running status", tr.Result)
	}
	var sawOutcome bool
	for _, m := range fold.Conversation.Messages {
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "background work bg1 completed") &&
				strings.Contains(p.Text, "done-work") {
				sawOutcome = true
			}
		}
	}
	if !sawOutcome {
		t.Error("background outcome did not arrive as a user-role message")
	}
}

// S6.1: kill cancels running background work; the cancellation lands as a
// message and the handle set empties.
func TestBackgroundWorkKill(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "bg1", Name: "bash",
				Args: map[string]any{"command": "sleep 30", "background": true}}},
			{Finish: "tool_use"},
		}},
		// GenStep 2: kill it.
		{
			Expect: scripted.Expect{LastMessageContains: "handle"},
			Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "k1", Name: "kill",
					Args: map[string]any{"handle": "bg1"}}},
				{Finish: "tool_use"},
			},
		},
		// GenStep 3: the cancellation arrived as a message; done.
		{
			Expect:  scripted.Expect{LastMessageContains: "canceled"},
			Respond: []scripted.Event{{Text: "killed it"}, {Finish: "end_turn"}},
		},
	}}
	l := testLoop(t, fix, t.TempDir())

	res, err := l.Run(context.Background(), "start then kill")
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
	if len(fold.Handles) != 0 {
		t.Errorf("background work not removed after kill: %+v", fold.Handles)
	}
	// The kill tool paired normally; the cancellation is a user message.
	kr := fold.Conversation.ToolResults["k1"]
	if !strings.Contains(string(kr.Result), "cancelling") {
		t.Errorf("kill result = %s", kr.Result)
	}
}

// 决策 #31: a final generation over in-flight background work is NOT
// quiescence — the session idles, the settlement feeds back as a user-role
// input that earns one more turn, and only then does the session quiesce.
func TestBackgroundWorkSettlesBeforeQuiescence(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "bg1", Name: "bash",
				Args: map[string]any{"command": "sleep 0.2; echo awaited-output", "background": true}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "ending now, but await the work"}, {Finish: "end_turn"}}},
		// The awaited outcome arrived as a user message → one more turn.
		{
			Expect:  scripted.Expect{LastMessageContains: "awaited-output"},
			Respond: []scripted.Event{{Text: "saw the result, done"}, {Finish: "end_turn"}},
		},
	}}
	l := testLoop(t, fix, t.TempDir())

	res, err := l.Run(context.Background(), "await on end")
	if err != nil {
		t.Fatal(err)
	}
	if res.GenSteps != 3 {
		t.Fatalf("turns = %d, want 3 (the outcome earned a turn)", res.GenSteps)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	var sawCompleteWithOutput, sawWaitEntered, sawWaitResolved bool
	for _, e := range events {
		if e.Type == event.TypeActivityCompleted && strings.Contains(string(e.Payload), "awaited-output") {
			sawCompleteWithOutput = true
		}
		if e.Type == event.TypeWaitingEntered && strings.Contains(string(e.Payload), `"input"`) {
			sawWaitEntered = true
		}
		if e.Type == event.TypeWaitingResolved && strings.Contains(string(e.Payload), "work_settled") {
			sawWaitResolved = true
		}
	}
	if !sawCompleteWithOutput {
		t.Error("await must let the background work finish with real output, not cancel it")
	}
	if !sawWaitEntered || !sawWaitResolved {
		t.Errorf("waiting for background work idle must be journaled: entered=%v resolved=%v", sawWaitEntered, sawWaitResolved)
	}
}

// 裁决 #15: receipts 是 agent 配置层的投递模式。默认 steer——早到的
// 后台结果在 turn 内的安全边界即进对话;turn_end——推迟到 turn 收尾,
// 由回执唤醒下一个 turn。断言锚在 journal 顺序:bg 终态 vs 该 turn 的
// final generation。
func TestReceiptsModeControlsSettlementTiming(t *testing.T) {
	run := func(t *testing.T, receipts string) (bgSettleSeq, finalGenSeq int64) {
		t.Helper()
		fix := scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "bg1", Name: "bash",
					Args: map[string]any{"command": "echo early-result", "background": true}}},
				{ToolCall: &scripted.ToolCallEvent{CallID: "fg1", Name: "bash",
					Args: map[string]any{"command": "sleep 0.5; echo slow-foreground"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "turn over"}, {Finish: "end_turn"}}},
			{
				Expect:  scripted.Expect{LastMessageContains: "early-result"},
				Respond: []scripted.Event{{Text: "saw it"}, {Finish: "end_turn"}},
			},
		}}
		l := testLoop(t, fix, t.TempDir())
		l.Spec.Receipts = receipts
		if _, err := l.Run(context.Background(), "race the receipt"); err != nil {
			t.Fatal(err)
		}
		events, err := store.ReadEvents(l.Store.Dir())
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range events {
			switch e.Type {
			case event.TypeActivityCompleted:
				if strings.Contains(string(e.Payload), "early-result") && bgSettleSeq == 0 {
					bgSettleSeq = e.Seq
				}
			case event.TypeAssistantMessage:
				if strings.Contains(string(e.Payload), "turn over") {
					finalGenSeq = e.Seq
				}
			}
		}
		if bgSettleSeq == 0 || finalGenSeq == 0 {
			t.Fatalf("missing anchors: settle=%d final=%d", bgSettleSeq, finalGenSeq)
		}
		return bgSettleSeq, finalGenSeq
	}

	t.Run("steer lands mid-turn", func(t *testing.T) {
		settle, final := run(t, "")
		if settle > final {
			t.Fatalf("steer: settle seq %d after final generation %d — receipt missed the in-turn boundary", settle, final)
		}
	})
	t.Run("turn_end defers to the idle", func(t *testing.T) {
		settle, final := run(t, "turn_end")
		if settle < final {
			t.Fatalf("turn_end: settle seq %d before final generation %d — receipt interrupted the turn", settle, final)
		}
	})
}
