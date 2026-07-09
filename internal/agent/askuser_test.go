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
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// newAskLoop builds a loop whose spec advertises ask_user, sharing an
// externally-owned store so resume tests can re-enter the same session.
func newAskLoop(t *testing.T, fix scripted.Fixture, root string, es *store.EventStore) *Loop {
	t.Helper()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	return &Loop{
		Spec: &AgentSpec{
			Name:               "test",
			Model:              ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
			SystemPrompt:       "be helpful",
			Tools:              []string{"read_file", "ask_user"},
			MaxGenerationSteps: 10,
		},
		Provider:  scripted.New(fix),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)),
		SessionID: "test-session",
	}
}

// waitAskPark blocks until an ask_user park (WaitingEntered{input} carrying
// a question detail) is journaled.
func waitAskPark(t *testing.T, dir string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		evs, _ := store.ReadEvents(dir)
		for _, e := range evs {
			if e.Type != event.TypeWaitingEntered {
				continue
			}
			var w event.WaitingEntered
			if json.Unmarshal(e.Payload, &w) == nil && w.Kind == event.WaitInput && len(w.Detail) > 0 {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for ask_user park")
}

func foldOf(t *testing.T, dir string) state.State {
	t.Helper()
	evs, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := state.Fold(evs)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func hasEvent(t *testing.T, dir, typ, contains string) bool {
	t.Helper()
	evs, _ := store.ReadEvents(dir)
	for _, e := range evs {
		if e.Type == typ && strings.Contains(string(e.Payload), contains) {
			return true
		}
	}
	return false
}

// askThenAnswer is the common fixture: turn 1 asks one question and stops;
// turn 2 (after the reply pairs the call) sees the answer and ends.
func askThenAnswer() scripted.Fixture {
	return scripted.Fixture{Steps: []scripted.Step{
		{
			Expect: scripted.Expect{LastMessageContains: "which db"},
			Respond: []scripted.Event{
				{Text: "let me check with you"},
				{ToolCall: &scripted.ToolCallEvent{Name: "ask_user", Args: map[string]any{"question": "Postgres or MySQL?"}}},
				{Finish: "tool_use"},
			},
		},
		{
			// The reply arrives as this call's tool result (not a new user
			// message): the answer text must be in the assembled request.
			Expect:  scripted.Expect{LastMessageContains: "postgres"},
			Respond: []scripted.Event{{Text: "going with postgres"}, {Finish: "end_turn"}},
		},
	}}
}

// askThenContinue is askThenAnswer with a turn-2 that accepts ANY prior
// message (used where the paired result is not the answer text, e.g. an
// interrupt renders "[interrupted by user]").
func askThenContinue() scripted.Fixture {
	f := askThenAnswer()
	f.Steps[1].Expect = scripted.Expect{}
	return f
}

// answerStep is JUST turn 2: a resumed process re-enters after the ask was
// journaled by the prior process, so its first LLM request IS the reply turn.
func answerStep() scripted.Fixture {
	return scripted.Fixture{Steps: []scripted.Step{
		{Expect: scripted.Expect{LastMessageContains: "postgres"},
			Respond: []scripted.Event{{Text: "going with postgres"}, {Finish: "end_turn"}}},
	}}
}

// The core loop: ask parks the session, the inbox reply pairs the ask_user
// call as its tool result, and the SAME session continues in a new turn.
func TestAskUserParkAndAnswer(t *testing.T) {
	root := t.TempDir()
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()

	inputs := make(chan protocol.UserInput)
	l := newAskLoop(t, askThenAnswer(), root, es)
	l.UserInputs = inputs
	go func() {
		waitAskPark(t, l.Store.Dir())
		inputs <- protocol.UserInput{Text: "use postgres", DeliverySeq: 1}
		waitAnswers(t, l.Store.Dir(), 2)
		close(inputs)
	}()

	res, err := l.Run(context.Background(), "which db should I use?")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" || res.GenSteps != 2 {
		t.Fatalf("res = %+v, want closed after 2 turns", res)
	}
	dir := l.Store.Dir()
	if !hasEvent(t, dir, event.TypeAskResolved, `"resolution":"answered"`) {
		t.Error("no answered AskResolved journaled")
	}
	if !hasEvent(t, dir, event.TypeWaitingResolved, `"resolution":"answered"`) {
		t.Error("park not resolved as answered")
	}
	tr, ok := foldOf(t, dir).Conversation.ToolResults["call_1_0"]
	if !ok || tr.IsError || !strings.Contains(string(tr.Result), "use postgres") {
		t.Fatalf("ask_user call not paired with the answer: %+v (ok=%v)", tr, ok)
	}
}

// A second ask_user in one turn is rejected model-visibly; the first parks.
func TestAskUserSecondRejected(t *testing.T) {
	root := t.TempDir()
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()

	fix := scripted.Fixture{Steps: []scripted.Step{
		{
			Expect: scripted.Expect{LastMessageContains: "go"},
			Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{Name: "ask_user", Args: map[string]any{"question": "A?"}}},
				{ToolCall: &scripted.ToolCallEvent{Name: "ask_user", Args: map[string]any{"question": "B?"}}},
				{Finish: "tool_use"},
			},
		},
		{Expect: scripted.Expect{}, Respond: []scripted.Event{{Text: "ok"}, {Finish: "end_turn"}}},
	}}
	inputs := make(chan protocol.UserInput)
	l := newAskLoop(t, fix, root, es)
	l.UserInputs = inputs
	go func() {
		waitAskPark(t, l.Store.Dir())
		inputs <- protocol.UserInput{Text: "answer to A", DeliverySeq: 1}
		waitAnswers(t, l.Store.Dir(), 2)
		close(inputs)
	}()
	if _, err := l.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	fold := foldOf(t, l.Store.Dir())
	first := fold.Conversation.ToolResults["call_1_0"]
	second, ok := fold.Conversation.ToolResults["call_1_1"]
	if !ok || !second.IsError || !strings.Contains(string(second.Result), "one ask_user question per turn") {
		t.Fatalf("second ask_user not rejected: %+v (ok=%v)", second, ok)
	}
	if first.IsError || !strings.Contains(string(first.Result), "answer to A") {
		t.Fatalf("first ask_user not answered: %+v", first)
	}
}

// An interrupt while parked kills the question (interrupted result) and the
// loop continues — interrupt is guidance, not shutdown.
func TestAskUserInterruptedWhileParked(t *testing.T) {
	root := t.TempDir()
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()

	inputs := make(chan protocol.UserInput)
	interrupts := make(chan struct{}, 1)
	l := newAskLoop(t, askThenContinue(), root, es)
	l.UserInputs = inputs
	l.Interrupts = interrupts
	go func() {
		waitAskPark(t, l.Store.Dir())
		interrupts <- struct{}{}
		waitAnswers(t, l.Store.Dir(), 2)
		close(inputs)
	}()
	if _, err := l.Run(context.Background(), "which db should I use?"); err != nil {
		t.Fatal(err)
	}
	dir := l.Store.Dir()
	if !hasEvent(t, dir, event.TypeWaitingResolved, "superseded_by_interrupt") {
		t.Error("park not resolved by interrupt")
	}
	tr := foldOf(t, dir).Conversation.ToolResults["call_1_0"]
	if !tr.IsError || !strings.Contains(string(tr.Result), "interrupted by user") {
		t.Fatalf("ask_user not rendered interrupted: %+v", tr)
	}
}

// Headless (no live input source): the park returns the run; a resume with
// an input source answers it. One question, two processes.
func TestAskUserHeadlessReturnsAndResumes(t *testing.T) {
	root := t.TempDir()
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()

	l := newAskLoop(t, askThenAnswer(), root, es)
	res, err := l.Run(context.Background(), "which db should I use?")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "waiting_input" {
		t.Fatalf("headless park res = %+v, want waiting_input", res)
	}
	if _, ok := foldOf(t, l.Store.Dir()).Conversation.ToolResults["call_1_0"]; ok {
		t.Fatal("call resolved before any answer")
	}

	// Resume with an input source and answer.
	inputs := make(chan protocol.UserInput, 1)
	l2 := newAskLoop(t, answerStep(), root, es)
	l2.UserInputs = inputs
	go func() {
		inputs <- protocol.UserInput{Text: "use postgres", DeliverySeq: 1}
		waitAnswers(t, l2.Store.Dir(), 2)
		close(inputs)
	}()
	if _, err := l2.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	tr, ok := foldOf(t, l2.Store.Dir()).Conversation.ToolResults["call_1_0"]
	if !ok || tr.IsError || !strings.Contains(string(tr.Result), "use postgres") {
		t.Fatalf("resumed answer not paired: %+v (ok=%v)", tr, ok)
	}
}

// P0 regression (adversarial review): the reply arrives via the durable
// MAILBOX (daemon acks a send before awaitAnswer journals AskResolved), then
// the process crashes. On resume the mailbox replay must PAIR the reply as
// the ask_user tool result — not orphan it as a standalone user message that
// leaves the call forever unpaired. Every other ask_user test drives replies
// through the UserInputs channel and never exercises this mailbox path.
func TestAskUserMailboxReplyPairsAcrossCrash(t *testing.T) {
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

	// Headless run parks on the question and returns (no input source).
	l := newAskLoop(t, askThenAnswer(), root, es)
	res, err := l.Run(context.Background(), "which db should I use?")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "waiting_input" {
		t.Fatalf("run res = %+v, want waiting_input park", res)
	}

	// The daemon durably enqueues the reply into the mailbox, but the crash
	// hit before awaitAnswer could journal AskResolved.
	if _, err := store.AppendInbox(sessDir, protocol.UserInput{Text: "use postgres"}); err != nil {
		t.Fatal(err)
	}
	_ = es.Close()

	// Fresh process resumes: the mailbox replay must pair, not orphan.
	es2, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es2.Close() }()
	l2 := newAskLoop(t, answerStep(), root, es2)
	if _, err := l2.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}

	fold := foldOf(t, sessDir)
	tr, ok := fold.Conversation.ToolResults["call_1_0"]
	if !ok || tr.IsError || !strings.Contains(string(tr.Result), "use postgres") {
		t.Fatalf("mailbox reply not paired to ask_user call: %+v (ok=%v)", tr, ok)
	}
	if !hasEvent(t, sessDir, event.TypeAskResolved, "answered") {
		t.Error("no AskResolved(answered) from mailbox replay")
	}
	// The reply must NOT also appear as a standalone user message (the orphan
	// bug): it is the call's result, nothing else.
	for _, m := range fold.Conversation.Messages {
		if m.Role != provider.RoleUser {
			continue
		}
		for _, p := range m.Parts {
			if strings.Contains(p.Text, "use postgres") {
				t.Fatal("reply orphaned as a user message instead of pairing the call")
			}
		}
	}
}

// A crash mid-park (ctx cancelled after the park is durable) self-heals on
// resume: the park is re-entered and answered, no re-ask.
func TestAskUserCrashResumeReparks(t *testing.T) {
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

	// Run until parked, then cancel (process teardown).
	ctx, cancel := context.WithCancel(context.Background())
	block := make(chan protocol.UserInput) // never sends: forces the park to wait
	l := newAskLoop(t, askThenAnswer(), root, es)
	l.UserInputs = block
	go func() {
		waitAskPark(t, l.Store.Dir())
		cancel()
	}()
	if _, err := l.Run(ctx, "which db should I use?"); err == nil {
		t.Fatal("want cancellation error from the parked run")
	}
	_ = es.Close()

	// The park is durable. A fresh process resumes and answers.
	es2, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es2.Close() }()
	if s := foldOf(t, sessDir); s.Waiting == nil || s.Waiting.Kind != event.WaitInput {
		t.Fatalf("park not durable across crash: waiting = %+v", s.Waiting)
	}
	inputs := make(chan protocol.UserInput, 1)
	l2 := newAskLoop(t, answerStep(), root, es2)
	l2.UserInputs = inputs
	go func() {
		inputs <- protocol.UserInput{Text: "use postgres", DeliverySeq: 1}
		waitAnswers(t, l2.Store.Dir(), 2)
		close(inputs)
	}()
	if _, err := l2.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	tr, ok := foldOf(t, sessDir).Conversation.ToolResults["call_1_0"]
	if !ok || tr.IsError || !strings.Contains(string(tr.Result), "use postgres") {
		t.Fatalf("resumed answer not paired after crash: %+v (ok=%v)", tr, ok)
	}
}
