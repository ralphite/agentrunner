package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
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
	inputs := make(chan string)
	l := testLoop(t, fix, t.TempDir())
	l.Conversational = true
	l.UserInputs = inputs
	go func() {
		waitAnswers(t, l.Store.Dir(), 1)
		inputs <- "second question"
		waitAnswers(t, l.Store.Dir(), 2)
		inputs <- "third question"
		waitAnswers(t, l.Store.Dir(), 3)
		close(inputs)
	}()

	res, err := l.Run(context.Background(), "first question")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" || res.Turns != 3 {
		t.Fatalf("res = %+v, want closed after 3 turns", res)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var userInputs, parks, resolves, ends int
	var lastType string
	for _, e := range events {
		switch e.Type {
		case event.TypeInputReceived:
			userInputs++
		case event.TypeWaitingEntered:
			parks++
		case event.TypeWaitingResolved:
			resolves++
		case event.TypeRunEnded:
			ends++
		}
		lastType = e.Type
	}
	// 1 initial + 2 follow-ups; 3 parks (after each yield), 3 resolutions
	// (2 inputs + 1 close); exactly ONE terminal, and it is the tail.
	if userInputs != 3 {
		t.Errorf("user inputs = %d, want 3", userInputs)
	}
	if parks != 3 || resolves != 3 {
		t.Errorf("parks/resolves = %d/%d, want 3/3", parks, resolves)
	}
	if ends != 1 || lastType != event.TypeRunEnded {
		t.Errorf("run_ended count=%d tail=%s — the session must end exactly once, at close", ends, lastType)
	}
}

// The park resolution vocabulary: closing the channel resolves the park as
// "closed"; the session ends via the epilogue with reason "closed".
func TestConversationalCloseResolution(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}
	inputs := make(chan string)
	close(inputs)
	l := testLoop(t, fix, t.TempDir())
	l.Conversational = true
	l.UserInputs = inputs

	res, err := l.Run(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" || res.Turns != 1 {
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
	if res.Reason != "completed" || res.Turns != 1 {
		t.Fatalf("res = %+v, want v1 completed semantics", res)
	}
}

// v2 M1.3 (C10a): a conversational session that parked for input, then had
// its process die, RESUMES back into the idle park and continues on the
// next input — "answer, wait" survives a restart with no special action.
func TestConversationalParkResumes(t *testing.T) {
	root := t.TempDir()
	sessDir := filepath.Join(t.TempDir(), "sess")

	// Phase 1: open, take one turn, park — then cancel (simulated crash).
	fix1 := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answer one"}, {Finish: "end_turn"}}},
	}}
	es1, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	l1 := testLoop(t, fix1, root)
	l1.Store = es1
	l1.Conversational = true
	l1.UserInputs = make(chan string) // never fed: the session parks and stays
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel once the park is durable (WaitingEntered{input} in the journal).
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

	// The journal parked and did NOT end.
	evs, _ := store.ReadEvents(sessDir)
	for _, e := range evs {
		if e.Type == event.TypeRunEnded {
			t.Fatal("parked session ended before resume")
		}
	}

	// Phase 2: reopen on the SAME dir and resume — re-park, then one input
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
	inputs := make(chan string, 1)
	inputs <- "second question"
	close(inputs)
	l2 := testLoop(t, fix2, root)
	l2.Store = es2
	l2.Conversational = true
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
		case event.TypeRunEnded:
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
		t.Fatalf("inputs=%d ends=%d answerTwo=%v — resume must re-park then continue to close", inputsN, ends, sawAnswerTwo)
	}
}

// v2 M2.1 (C2 core): messages that queue while a turn is in flight all
// enter the NEXT turn together (batch drain), in arrival order — type-ahead
// never splits into extra turns or reorders.
func TestConversationalTypeAheadBatches(t *testing.T) {
	// Turn 1 runs a tool (a beat during which two messages queue); the batch
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
	inputs := make(chan string, 2)
	l := testLoop(t, fix, t.TempDir())
	l.Conversational = true
	l.UserInputs = inputs
	go func() {
		// After turn 1's answer, queue two messages back-to-back BEFORE the
		// loop parks-and-drains, then close.
		waitAnswers(t, l.Store.Dir(), 1)
		inputs <- "queued one"
		inputs <- "queued two"
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
// conversational session. At IDLE, an interrupt closes the session (the
// interactive convention); a queued input during a turn never cancels the
// running activity (structural: separate channels).
func TestConversationalIdleInterruptCloses(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answered, now idle"}, {Finish: "end_turn"}}},
	}}
	interrupts := make(chan struct{}, 1)
	l := testLoop(t, fix, t.TempDir())
	l.Conversational = true
	l.UserInputs = make(chan string) // never fed
	l.Interrupts = interrupts
	go func() {
		waitAnswers(t, l.Store.Dir(), 1) // wait until it parks at idle
		interrupts <- struct{}{}
	}()

	res, err := l.Run(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" {
		t.Fatalf("res = %+v, want closed (idle interrupt = close)", res)
	}
	events, _ := store.ReadEvents(l.Store.Dir())
	var resolution string
	for _, e := range events {
		if e.Type == event.TypeWaitingResolved {
			dec, _ := event.DecodePayload(e)
			resolution = dec.(*event.WaitingResolved).Resolution
		}
	}
	if resolution != "closed_by_interrupt" {
		t.Errorf("resolution = %q, want closed_by_interrupt", resolution)
	}
}

// v2 M3 review fix: a ctx cancel MID-TURN (daemon shutdown/deploy while the
// LLM call is in flight) leaves NO terminal — the same crash discipline as
// the park path — so the conversation resumes and re-runs the turn.
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
	l1.Conversational = true
	l1.UserInputs = make(chan string) // never fed
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
		if e.Type == event.TypeRunEnded {
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
	inputs := make(chan string)
	close(inputs)
	l2 := testLoop(t, scripted.Fixture{}, root)
	l2.Store = es2
	l2.Provider = prov
	l2.Conversational = true
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
// cumulative — a session whose total turns exceed max_turns keeps answering
// as long as each exchange stays within budget. (The old cumulative cap
// silently wedged the session: inputs journaled, never answered.)
func TestConversationalBudgetPerExchange(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answer one"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "answer two"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "answer three"}, {Finish: "end_turn"}}},
	}}
	inputs := make(chan string)
	l := testLoop(t, fix, t.TempDir())
	l.Spec.MaxTurns = 2 // < total turns (3): cumulative budgeting would wedge
	l.Conversational = true
	l.UserInputs = inputs
	go func() {
		waitAnswers(t, l.Store.Dir(), 1)
		inputs <- "second question"
		waitAnswers(t, l.Store.Dir(), 2)
		inputs <- "third question"
		waitAnswers(t, l.Store.Dir(), 3)
		close(inputs)
	}()
	res, err := l.Run(context.Background(), "first question")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "closed" || res.Turns != 3 {
		t.Fatalf("res = %+v, want closed after 3 turns (per-exchange budget)", res)
	}
}

// decide's conversational budget arithmetic, directly on crafted folds:
// budget counts from the last user input; exhaustion over a pending input
// ends visibly (max_turns), never a silent park.
func TestDecideConversationalBudget(t *testing.T) {
	mk := func(turn, lastInput int, pending bool) state.State {
		s := state.New()
		s.Run.Status = state.StatusRunning
		s.Run.Turn = turn
		s.Run.LastInputTurn = lastInput
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
		// Deep into a long conversation (turn 40 > maxTurns) a fresh input
		// still gets its turn — the old cumulative cap returned doWaitInput
		// here, wedging the session.
		{"fresh input late in session", 40, 39, true, doTurn, ""},
		// A pending input with the exchange budget spent ends visibly.
		{"exhausted exchange ends", 40, 0, true, doEnd, "max_turns"},
		// Truly idle parks regardless of totals.
		{"idle parks", 40, 39, false, doWaitInput, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			act := decide(mk(c.turn, c.lastInput, c.pending), 10, "", true)
			if act.kind != c.wantKind {
				t.Fatalf("kind = %v, want %v", act.kind, c.wantKind)
			}
			if c.wantReason != "" && act.reason != c.wantReason {
				t.Fatalf("reason = %q, want %q", act.reason, c.wantReason)
			}
		})
	}
}
