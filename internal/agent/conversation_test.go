package agent

import (
	"context"
	"testing"

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
