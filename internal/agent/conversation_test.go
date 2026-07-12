package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// waitAnswers blocks until the session journal holds >= n assistant
// messages (turn synchronization for reactive input feeders).
func waitAnswers(t *testing.T, dir string, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		evs, _ := store.ReadEvents(dir)
		c := 0
		for _, e := range evs {
			if e.Type == event.TypeAssistantMessage {
				c++
			}
		}
		if c >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("timed out waiting for %d assistant messages", n)
}

// v2 M1.1 (C1): a conversational session answers, PARKS, and continues on
// the next user input — three inputs, three turns, one session, and the
// terminal event appears only at close.
func TestConversationalMultiInput(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{
			Expect:  scripted.Expect{LastMessageContains: "first question"},
			Respond: []scripted.Event{{Text: "answer one"}, {Finish: "end_turn"}},
		},
		{
			// The follow-up must arrive as a NEW user message in the SAME
			// conversation (context continuity is what C1 is about).
			Expect:  scripted.Expect{LastMessageContains: "second question"},
			Respond: []scripted.Event{{Text: "answer two"}, {Finish: "end_turn"}},
		},
		{
			Expect:  scripted.Expect{LastMessageContains: "third question"},
			Respond: []scripted.Event{{Text: "answer three"}, {Finish: "end_turn"}},
		},
	}}
	// Spaced sends: each follow-up is delivered only AFTER the prior turn's
	// answer is journaled — one input per turn, the QA-01 timing.
	inputs := make(chan protocol.UserInput)
	l := testLoop(t, fix, t.TempDir())
	l.UserInputs = inputs
	go func() {
		waitAnswers(t, l.Store.Dir(), 1)
		inputs <- protocol.UserInput{Text: "second question"}
		waitAnswers(t, l.Store.Dir(), 2)
		inputs <- protocol.UserInput{Text: "third question"}
		waitAnswers(t, l.Store.Dir(), 3)
		close(inputs)
	}()

	res, err := l.Run(context.Background(), "first question")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" || res.GenSteps != 3 {
		t.Fatalf("res = %+v, want closed after 3 turns", res)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var userInputs, idles, resolves, ends int
	var lastType string
	for _, e := range events {
		switch e.Type {
		case event.TypeInputReceived:
			userInputs++
		case event.TypeWaitingEntered:
			idles++
		case event.TypeWaitingResolved:
			resolves++
		case event.TypeSessionClosed:
			ends++
		}
		lastType = e.Type
	}
	// 1 initial + 2 follow-ups; 3 goes idle (after each yield), 3 resolutions
	// (2 inputs + 1 close); exactly ONE terminal, and it is the tail.
	if userInputs != 3 {
		t.Errorf("user inputs = %d, want 3", userInputs)
	}
	if idles != 3 || resolves != 3 {
		t.Errorf("idles/resolves = %d/%d, want 3/3", idles, resolves)
	}
	if ends != 1 || lastType != event.TypeSessionClosed {
		t.Errorf("session_closed count=%d tail=%s — exactly one close intent, at close", ends, lastType)
	}
}

// The idle resolution vocabulary: closing the channel resolves the idle as
// "closed"; the session ends via the epilogue with reason "closed".
func TestConversationalCloseResolution(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}
	inputs := make(chan protocol.UserInput)
	close(inputs)
	l := testLoop(t, fix, t.TempDir())
	l.UserInputs = inputs

	res, err := l.Run(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" || res.GenSteps != 1 {
		t.Fatalf("res = %+v", res)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	var resolution string
	for _, e := range events {
		if e.Type == event.TypeWaitingResolved {
			dec, _ := event.DecodePayload(e)
			resolution = dec.(*event.WaitingResolved).Resolution
		}
	}
	if resolution != "closed" {
		t.Errorf("resolution = %q, want closed", resolution)
	}
}

func TestJournalInputCarriesDurableCommandReceipt(t *testing.T) {
	l := testLoop(t, scripted.Fixture{}, t.TempDir())
	ds := &driveState{s: state.New()}
	appendE := l.appender(ds)
	if _, err := appendE(event.TypeSessionStarted, &event.SessionStarted{
		SubStateVersions: state.SubStateVersions(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := l.journalInput(ds, appendE, protocol.UserInput{
		Text: "once", CommandID: "cmd-send-1", DeliverySeq: 1,
	}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	got := events[len(events)-1]
	if got.Type != event.TypeInputReceived || got.CommandID != "cmd-send-1" {
		t.Fatalf("input receipt = type %s command %q", got.Type, got.CommandID)
	}
	if got.CausationID == "cmd-send-1" {
		t.Fatal("command receipt must not replace the linear event causation chain")
	}
}

func TestJournalInputPreservesTypedContentAndProvenance(t *testing.T) {
	l := testLoop(t, scripted.Fixture{}, t.TempDir())
	ds := &driveState{s: state.New()}
	appendE := l.appender(ds)
	if err := l.journalInput(ds, appendE, protocol.UserInput{
		CommandID: "cmd-webhook-1", DeliverySeq: 4,
		TurnID: "turn-webhook-1", ItemID: "item-webhook-1",
		Principal: "service:buildbot", Source: "webhook", Trust: "external",
		Content: []protocol.ContentPart{
			{Kind: provider.PartText, Text: "inspect"},
			{Kind: provider.PartFile, MediaType: "application/json", Data: []byte(`{"ok":true}`)},
		},
	}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := event.DecodePayload(events[len(events)-1])
	if err != nil {
		t.Fatal(err)
	}
	in := decoded.(*event.InputReceived)
	if in.Principal != "service:buildbot" || in.Source != "webhook" || in.Trust != "external" ||
		in.TurnID != "turn-webhook-1" || in.ItemID != "item-webhook-1" || len(in.Content) != 2 ||
		in.Content[1].Ref == "" || len(in.Content[1].Data) != 0 {
		t.Fatalf("journaled typed input = %+v", in)
	}
	item := ds.s.Interactions.Items["item-webhook-1"]
	if item.Principal != "service:buildbot" || len(item.Content) != 2 {
		t.Fatalf("folded typed item = %+v", item)
	}
}

// Task mode (Conversational=false, the v1 default) is untouched: yield
// still completes the run — every existing caller keeps its contract.
func TestTaskModeStillEndsOnYield(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	res, err := l.Run(context.Background(), "do the thing")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.GenSteps != 1 {
		t.Fatalf("res = %+v, want v1 completed semantics", res)
	}
}

// v2 M1.3 (C10a): a conversational session that idle for input, then had
// its process die, RESUMES back into the idle idle and continues on the
// next input — "answer, wait" survives a restart with no special action.
func TestConversationalIdleResumes(t *testing.T) {
	root := t.TempDir()
	sessDir := filepath.Join(t.TempDir(), "sess")

	// Phase 1: open, take one turn, idle — then cancel (simulated crash).
	fix1 := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answer one"}, {Finish: "end_turn"}}},
	}}
	es1, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	l1 := testLoop(t, fix1, root)
	l1.Store = es1
	l1.UserInputs = make(chan protocol.UserInput) // never fed: the session goes idle and stays
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel once the idle is durable (WaitingEntered{input} in the journal).
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			evs, _ := store.ReadEvents(sessDir)
			for _, e := range evs {
				if e.Type == event.TypeWaitingEntered {
					dec, _ := event.DecodePayload(e)
					if dec.(*event.WaitingEntered).Kind == event.WaitInput {
						cancel()
						return
					}
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
	}()
	_, _ = l1.Run(ctx, "first question") // returns with the cancel cause
	_ = es1.Close()

	// The journal idle and did NOT end.
	evs, _ := store.ReadEvents(sessDir)
	for _, e := range evs {
		if e.Type == event.TypeSessionClosed {
			t.Fatal("idle session ended before resume")
		}
	}

	// Phase 2: reopen on the SAME dir and resume — re-idle, then one input
	// continues the conversation and closes.
	fix2 := scripted.Fixture{Steps: []scripted.Step{
		{
			Expect:  scripted.Expect{LastMessageContains: "second question"},
			Respond: []scripted.Event{{Text: "answer two"}, {Finish: "end_turn"}},
		},
	}}
	es2, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es2.Close() }()
	inputs := make(chan protocol.UserInput, 1)
	inputs <- protocol.UserInput{Text: "second question"}
	close(inputs)
	l2 := testLoop(t, fix2, root)
	l2.Store = es2
	l2.UserInputs = inputs

	res, err := l2.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" {
		t.Fatalf("res = %+v, want closed after the resumed turn", res)
	}
	final, _ := store.ReadEvents(sessDir)
	var inputsN, ends int
	var sawAnswerTwo bool
	for _, e := range final {
		switch e.Type {
		case event.TypeInputReceived:
			inputsN++
		case event.TypeSessionClosed:
			ends++
		case event.TypeAssistantMessage:
			dec, _ := event.DecodePayload(e)
			for _, p := range dec.(*event.AssistantMessage).Message.Parts {
				if p.Text == "answer two" {
					sawAnswerTwo = true
				}
			}
		}
	}
	if inputsN != 2 || ends != 1 || !sawAnswerTwo {
		t.Fatalf("inputs=%d ends=%d answerTwo=%v — resume must re-idle then continue to close", inputsN, ends, sawAnswerTwo)
	}
}

// v2 M2.1 (C2 core): messages that queue while a turn is in flight all
// enter the NEXT turn together (batch drain), in arrival order — type-ahead
// never splits into extra turns or reorders.
func TestConversationalTypeAheadBatches(t *testing.T) {
	// GenStep 1 runs a tool (a beat during which two messages queue); the batch
	// turn must see BOTH queued messages.
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "bash",
				Args: map[string]any{"command": "echo working"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "acknowledged one"}, {Finish: "end_turn"}}},
		{
			// The batch turn: BOTH queued messages must be present, in order.
			Expect: scripted.Expect{LastMessageContains: "queued two"},
			Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "c2", Name: "bash",
					Args: map[string]any{"command": "echo both seen"}}},
				{Finish: "tool_use"},
			},
		},
		{Respond: []scripted.Event{{Text: "handled both"}, {Finish: "end_turn"}}},
	}}
	inputs := make(chan protocol.UserInput, 2)
	l := testLoop(t, fix, t.TempDir())
	l.UserInputs = inputs
	go func() {
		// After turn 1's answer, queue two messages back-to-back BEFORE the
		// loop goes idle-and-drains, then close.
		waitAnswers(t, l.Store.Dir(), 1)
		inputs <- protocol.UserInput{Text: "queued one"}
		inputs <- protocol.UserInput{Text: "queued two"}
		waitAnswers(t, l.Store.Dir(), 2)
		close(inputs)
	}()

	res, err := l.Run(context.Background(), "start working")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" {
		t.Fatalf("res = %+v", res)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	// Both queued inputs are journaled as consecutive user inputs (arrival
	// order), and they entered ONE batch turn — assert the two InputReceived
	// for the queued messages are adjacent (nothing interleaved).
	var texts []string
	for _, e := range events {
		if e.Type == event.TypeInputReceived {
			dec, _ := event.DecodePayload(e)
			texts = append(texts, dec.(*event.InputReceived).Text)
		}
	}
	// start + queued one + queued two = 3, in order.
	if len(texts) != 3 || texts[1] != "queued one" || texts[2] != "queued two" {
		t.Fatalf("input order = %v, want [start..., queued one, queued two]", texts)
	}
}

// v2 M2.2 (C8): interrupt and input are distinct gestures in a
// 裁决 #11 (2026-07-05): interrupt NEVER ends a session. At idle it is a
// no-op — the signal is journaled for audit, the session keeps waiting,
// and a later input continues the conversation; close is its own command.
func TestIdleInterruptIsNoOp(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answered, now idle"}, {Finish: "end_turn"}}},
		{
			Expect:  scripted.Expect{LastMessageContains: "still there?"},
			Respond: []scripted.Event{{Text: "still here"}, {Finish: "end_turn"}},
		},
	}}
	interrupts := make(chan struct{}, 1)
	inputs := make(chan protocol.UserInput, 1)
	l := testLoop(t, fix, t.TempDir())
	l.UserInputs = inputs
	l.Interrupts = interrupts
	go func() {
		waitAnswers(t, l.Store.Dir(), 1) // idle reached
		interrupts <- struct{}{}         // no-op: nothing to interrupt
		// Wait for the audit fact so the interrupt is consumed AT IDLE
		// before the next input races it into a running turn.
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			evs, _ := store.ReadEvents(l.Store.Dir())
			seen := false
			for _, e := range evs {
				if e.Type == event.TypeInputReceived {
					dec, _ := event.DecodePayload(e)
					if dec.(*event.InputReceived).Source == "interrupt" {
						seen = true
					}
				}
			}
			if seen {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		inputs <- protocol.UserInput{Text: "still there?"}
		waitAnswers(t, l.Store.Dir(), 2)
		close(inputs) // the explicit close gesture ends the wait
	}()

	res, err := l.Run(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" || res.GenSteps != 2 {
		t.Fatalf("res = %+v, want closed after TWO turns (interrupt closed nothing)", res)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	var sawInterruptAudit bool
	for _, e := range events {
		if e.Type == event.TypeInputReceived {
			dec, _ := event.DecodePayload(e)
			in := dec.(*event.InputReceived)
			if in.Source == "interrupt" {
				sawInterruptAudit = true
			}
		}
		if e.Type == event.TypeWaitingResolved {
			dec, _ := event.DecodePayload(e)
			if dec.(*event.WaitingResolved).Resolution == "closed_by_interrupt" {
				t.Fatal("idle interrupt resolved the wait — the close-at-idle convention must be gone")
			}
		}
	}
	if !sawInterruptAudit {
		t.Error("idle interrupt left no audit fact")
	}
}

// v2 M3 review fix: a ctx cancel MID-TURN (daemon shutdown/deploy while the
// LLM call is in flight) leaves NO terminal — the same crash discipline as
// the idle path — so the conversation resumes and re-runs the turn.
func TestConversationalMidTurnCancelResumes(t *testing.T) {
	root := t.TempDir()
	sessDir := filepath.Join(t.TempDir(), "sess")
	prov := &blockingLLM{entered: make(chan struct{}, 1)}

	// Phase 1: the model call blocks; cancel the loop ctx mid-turn.
	es1, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	l1 := testLoop(t, scripted.Fixture{}, root)
	l1.Store = es1
	l1.Provider = prov
	l1.UserInputs = make(chan protocol.UserInput) // never fed
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-prov.entered
		cancel()
	}()
	if _, err := l1.Run(ctx, "first question"); err == nil {
		t.Fatal("run survived a ctx cancel")
	}
	_ = es1.Close()
	evs, _ := store.ReadEvents(sessDir)
	for _, e := range evs {
		if e.Type == event.TypeSessionClosed {
			t.Fatal("mid-turn cancel journaled a terminal — session is unresumable")
		}
	}

	// Phase 2: resume re-enters the turn (LLM attempt 2 completes), then a
	// close ends the session normally.
	es2, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es2.Close() }()
	inputs := make(chan protocol.UserInput)
	close(inputs)
	l2 := testLoop(t, scripted.Fixture{}, root)
	l2.Store = es2
	l2.Provider = prov
	l2.UserInputs = inputs
	res, err := l2.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" {
		t.Fatalf("res = %+v, want closed after the resumed turn", res)
	}
	final, _ := store.ReadEvents(sessDir)
	var answered bool
	for _, e := range final {
		if e.Type == event.TypeAssistantMessage && strings.Contains(string(e.Payload), "done") {
			answered = true
		}
	}
	if !answered {
		t.Error("resumed session never answered the interrupted turn")
	}
}

// v2 M3 review fix: the conversational turn budget is PER EXCHANGE, not
// cumulative — a session whose total turns exceed max_generation_steps keeps answering
// as long as each turn stays within budget. (The old cumulative cap
// silently wedged the session: inputs journaled, never answered.)
func TestConversationalBudgetPerExchange(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answer one"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "answer two"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "answer three"}, {Finish: "end_turn"}}},
	}}
	inputs := make(chan protocol.UserInput)
	l := testLoop(t, fix, t.TempDir())
	l.Spec.MaxGenerationSteps = 2 // < total turns (3): cumulative budgeting would wedge
	l.UserInputs = inputs
	go func() {
		waitAnswers(t, l.Store.Dir(), 1)
		inputs <- protocol.UserInput{Text: "second question"}
		waitAnswers(t, l.Store.Dir(), 2)
		inputs <- protocol.UserInput{Text: "third question"}
		waitAnswers(t, l.Store.Dir(), 3)
		close(inputs)
	}()
	res, err := l.Run(context.Background(), "first question")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" || res.GenSteps != 3 {
		t.Fatalf("res = %+v, want closed after 3 turns (per-turn budget)", res)
	}
}

// decide's conversational budget arithmetic, directly on crafted folds:
// budget counts from the last user input; exhaustion over a pending input
// truncates visibly (LimitExceeded + idle), never a silent idle.
func TestDecideConversationalBudget(t *testing.T) {
	mk := func(turn, lastInput int, pending bool) state.State {
		s := state.New()
		s.Session.Status = state.StatusRunning
		s.Session.GenStep = turn
		s.Session.LastInputGenStep = lastInput
		var msgs []provider.Message
		msgs = append(msgs, provider.Message{Role: provider.RoleUser,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "q"}}})
		for i := 0; i < turn; i++ {
			msgs = append(msgs, provider.Message{Role: provider.RoleAssistant,
				Parts: []provider.Part{{Kind: provider.PartText, Text: "a"}}})
		}
		if pending {
			msgs = append(msgs, provider.Message{Role: provider.RoleUser,
				Parts: []provider.Part{{Kind: provider.PartText, Text: "next"}}})
		}
		s.Conversation.Messages = msgs
		return s
	}
	cases := []struct {
		name            string
		turn, lastInput int
		pending         bool
		wantKind        int
		wantReason      string
	}{
		// Deep into a long conversation (turn 40 > maxGenerationSteps) a fresh input
		// still gets its turn — the old cumulative cap returned doIdle
		// here, wedging the session.
		{"fresh input late in session", 40, 39, true, doTurn, ""},
		// A pending input with the turn budget spent truncates visibly
		// (决策 #30): LimitExceeded + idle, never a terminal state.
		{"exhausted turn truncates", 40, 0, true, doTruncate, ""},
		// Truly idle goes idle regardless of totals.
		{"idle stays idle", 40, 39, false, doIdle, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			act := decide(mk(c.turn, c.lastInput, c.pending), 10)
			if act.kind != c.wantKind {
				t.Fatalf("kind = %v, want %v", act.kind, c.wantKind)
			}
			if c.wantReason != "" && act.reason != c.wantReason {
				t.Fatalf("reason = %q, want %q", act.reason, c.wantReason)
			}
		})
	}
}

// INC-50 hard condition: a machine-delivered input (webhook ingress) is
// framed AS SEEN BY THE MODEL — the untrusted classification drives the
// conversation content, not just metadata — and its journaled trust can
// never rise above untrusted, whatever the delivery shell claimed.
func TestMachineInputFramedAndTrustClamped(t *testing.T) {
	l := testLoop(t, scripted.Fixture{}, t.TempDir())
	ds := &driveState{s: state.New()}
	appendE := l.appender(ds)
	if err := l.journalInput(ds, appendE, protocol.UserInput{
		Text: "CI run 42 failed on main", CommandID: "cmd-hook-1", DeliverySeq: 1,
		Source: protocol.SourceMachine, Trust: "local", // a buggy shell over-claims
		Principal: "hook:ci", TurnID: "turn-h1", ItemID: "item-h1",
	}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := event.DecodePayload(events[len(events)-1])
	if err != nil {
		t.Fatal(err)
	}
	in := decoded.(*event.InputReceived)
	if in.Trust != "untrusted" {
		t.Fatalf("machine trust journaled as %q, want untrusted", in.Trust)
	}
	if !strings.HasPrefix(in.Text, "[external event from hook:ci") ||
		!strings.Contains(in.Text, "treat it as data") ||
		!strings.Contains(in.Text, "CI run 42 failed on main") {
		t.Fatalf("machine input not framed: %q", in.Text)
	}
	// The fold's conversation — what assembly sends to the provider —
	// carries the frame too.
	msgs := ds.s.Conversation.Messages
	if len(msgs) == 0 || len(msgs[len(msgs)-1].Parts) == 0 ||
		!strings.HasPrefix(msgs[len(msgs)-1].Parts[0].Text, "[external event from hook:ci") {
		t.Fatalf("folded conversation lacks the isolation frame: %+v", msgs)
	}
	item := ds.s.Interactions.Items["item-h1"]
	if item.Trust != "untrusted" || item.Source != protocol.SourceMachine {
		t.Fatalf("folded item provenance = %+v", item)
	}
}

// 安全 review P2-3 regression: a machine input arriving as typed Content
// parts (a future machine transport) still carries the isolation frame —
// framing happens after content assembly, not only on the plain-text shape.
func TestMachineTypedContentGetsFrame(t *testing.T) {
	l := testLoop(t, scripted.Fixture{}, t.TempDir())
	ds := &driveState{s: state.New()}
	appendE := l.appender(ds)
	if err := l.journalInput(ds, appendE, protocol.UserInput{
		CommandID: "cmd-hook-2", DeliverySeq: 1, ItemID: "item-h2",
		Source: protocol.SourceMachine, Principal: "hook:ci",
		Content: []protocol.ContentPart{{Kind: provider.PartText, Text: "typed payload"}},
	}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := event.DecodePayload(events[len(events)-1])
	if err != nil {
		t.Fatal(err)
	}
	in := decoded.(*event.InputReceived)
	if !strings.HasPrefix(in.Text, "[external event from hook:ci") {
		t.Fatalf("typed-content machine input text lost the frame: %q", in.Text)
	}
	if len(in.Content) == 0 || in.Content[0].Kind != provider.PartText ||
		!strings.HasPrefix(in.Content[0].Text, "[external event from hook:ci") {
		t.Fatalf("typed-content parts lost the frame: %+v", in.Content)
	}
}
