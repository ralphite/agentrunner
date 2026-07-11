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

func twoQ() []event.AskQuestion {
	return []event.AskQuestion{
		{Question: "Which color?", Options: []event.AskOption{{Label: "Red"}, {Label: "Blue"}}},
		{Question: "Which sizes?", Options: []event.AskOption{{Label: "S"}, {Label: "M"}, {Label: "L"}}, MultiSelect: true},
	}
}

func TestValidateAskQuestions(t *testing.T) {
	opt := []event.AskOption{{Label: "A"}, {Label: "B"}}
	for name, tc := range map[string]struct {
		qs     []event.AskQuestion
		single string
		bad    bool
	}{
		"valid two":        {qs: twoQ()},
		"free-text only":   {qs: []event.AskQuestion{{Question: "why?"}}},
		"both forms":       {qs: twoQ(), single: "and this?", bad: true},
		"five questions":   {qs: make([]event.AskQuestion, 5), bad: true},
		"one option":       {qs: []event.AskQuestion{{Question: "q", Options: opt[:1]}}, bad: true},
		"five options":     {qs: []event.AskQuestion{{Question: "q", Options: []event.AskOption{{Label: "1"}, {Label: "2"}, {Label: "3"}, {Label: "4"}, {Label: "5"}}}}, bad: true},
		"empty label":      {qs: []event.AskQuestion{{Question: "q", Options: []event.AskOption{{Label: "A"}, {Label: " "}}}}, bad: true},
		"multi no options": {qs: []event.AskQuestion{{Question: "q", MultiSelect: true}}, bad: true},
	} {
		t.Run(name, func(t *testing.T) {
			if name == "five questions" {
				for i := range tc.qs {
					tc.qs[i].Question = "q"
				}
			}
			got := validateAskQuestions(tc.qs, tc.single)
			if (got != "") != tc.bad {
				t.Fatalf("validateAskQuestions = %q, want bad=%v", got, tc.bad)
			}
		})
	}
}

func TestValidateAskAnswers(t *testing.T) {
	qs := twoQ()
	for name, tc := range map[string]struct {
		answers []event.AskAnswer
		bad     bool
	}{
		"valid single":          {answers: []event.AskAnswer{{Question: 0, Selected: []string{"Red"}}}},
		"valid multi":           {answers: []event.AskAnswer{{Question: 1, Selected: []string{"S", "L"}}}},
		"index out of range":    {answers: []event.AskAnswer{{Question: 5, Selected: []string{"Red"}}}, bad: true},
		"multi on single":       {answers: []event.AskAnswer{{Question: 0, Selected: []string{"Red", "Blue"}}}, bad: true},
		"unknown label":         {answers: []event.AskAnswer{{Question: 0, Selected: []string{"Green"}}}, bad: true},
		"free text not allowed": {answers: []event.AskAnswer{{Question: 0, Text: "hmm"}}, bad: true},
		"empty answer":          {answers: []event.AskAnswer{{Question: 0}}, bad: true},
		"no answers":            {answers: nil, bad: true},
	} {
		t.Run(name, func(t *testing.T) {
			got := validateAskAnswers(qs, tc.answers)
			if (got != "") != tc.bad {
				t.Fatalf("validateAskAnswers = %q, want bad=%v", got, tc.bad)
			}
		})
	}
	if validateAskAnswers(nil, []event.AskAnswer{{Question: 0, Text: "x"}}) == "" {
		t.Fatal("a legacy park without structure must reject typed answers")
	}
}

// TestAnswerCommandPairsAcrossRestart proves the replay half (INC-47): a
// structured answer acked before a crash pairs the parked call on resume,
// with the typed selections journaled in AskResolved.Answers.
func TestAnswerCommandPairsAcrossRestart(t *testing.T) {
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
		Parts: []provider.Part{{Kind: provider.PartText, Text: "which?"},
			{Kind: provider.PartToolCall, CallID: "call_1_0", ToolName: "ask_user",
				Args: json.RawMessage(`{"questions":[{"question":"Which color?"}]}`)}}}}
	detail := askDetail{CallID: "call_1_0", Question: "Which color?", Questions: twoQ()}
	detailJSON, _ := json.Marshal(detail)
	appendSynthetic(t, es, []struct {
		typ     string
		payload any
	}{
		{event.TypeSessionStarted, &event.SessionStarted{SpecName: "t",
			SubStateVersions: state.SubStateVersions()}},
		{event.TypeInputReceived, &event.InputReceived{Text: "pick for me", Source: "user", DeliverySeq: 1}},
		{event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}},
		{event.TypeAssistantMessage, &asst},
		{event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitInput, Detail: detailJSON}},
	})
	_ = es.Close()

	// Mirror reality: the consumed seq-1 input exists in the mailbox too,
	// so the answer lands at seq 2 — above the fold's high-water.
	if _, err := store.AppendCommand(sessDir, protocol.SessionCommand{
		Kind:       protocol.CommandInput,
		CommandRef: protocol.CommandRef{CommandID: "cmd-1"},
		Input:      &protocol.UserInput{Text: "pick for me", CommandID: "cmd-1"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCommand(sessDir, protocol.SessionCommand{
		Kind:       protocol.CommandAnswer,
		CommandRef: protocol.CommandRef{CommandID: "ans-1"},
		Answer: &protocol.AnswerCommand{Answers: []event.AskAnswer{
			{Question: 0, Selected: []string{"Blue"}},
			{Question: 1, Selected: []string{"S", "M"}},
		}},
	}); err != nil {
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
	inputs := make(chan protocol.UserInput)
	close(inputs)
	l := &Loop{
		Spec: inDoubtSpec(),
		Provider: scripted.New(scripted.Fixture{Steps: []scripted.Step{
			{Expect: scripted.Expect{LastMessageContains: "Blue"},
				Respond: []scripted.Event{{Text: "noted"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
		}}),
		Exec: &tool.Executor{WS: ws}, Store: es2, SessionID: "ans",
		UserInputs: inputs,
	}
	if _, err := l.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	var resolved *event.AskResolved
	for _, e := range events {
		if e.Type == event.TypeAskResolved {
			dec, derr := event.DecodePayload(e)
			if derr != nil {
				t.Fatal(derr)
			}
			resolved = dec.(*event.AskResolved)
		}
	}
	if resolved == nil || resolved.Resolution != "answered" || len(resolved.Answers) != 2 ||
		resolved.Answers[0].Selected[0] != "Blue" {
		t.Fatalf("want typed AskResolved, got %+v", resolved)
	}
	s, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	tr, ok := s.Conversation.ToolResults["call_1_0"]
	if !ok || !strings.Contains(string(tr.Result), "Blue") {
		t.Fatalf("the call must pair with the typed result: %+v", tr)
	}
}
