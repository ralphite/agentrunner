package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
)

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
	inputs := make(chan string, 2)
	inputs <- "second question"
	inputs <- "third question"
	close(inputs) // after the queue drains, the user is done

	l := testLoop(t, fix, t.TempDir())
	l.Conversational = true
	l.UserInputs = inputs

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
