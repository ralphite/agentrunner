package cli

import (
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
)

func TestParseAnswerSpecs(t *testing.T) {
	qs := []event.AskQuestion{
		{Question: "color?", Options: []event.AskOption{{Label: "Red"}, {Label: "Blue"}}},
		{Question: "sizes?", Options: []event.AskOption{{Label: "S"}, {Label: "M"}, {Label: "L"}}, MultiSelect: true},
		{Question: "notes?"},
	}
	got, err := parseAnswerSpecs([]string{"1:2", "2:1,3", "3:text=看情况"}, qs)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Selected[0] != "Blue" || len(got[1].Selected) != 2 || got[1].Selected[1] != "L" || got[2].Text != "看情况" {
		t.Fatalf("parsed answers wrong: %+v", got)
	}
	for name, specs := range map[string][]string{
		"bad question":     {"9:1"},
		"bad option":       {"1:7"},
		"multi on single":  {"1:1,2"},
		"free text denied": {"1:text=nope"},
		"no colon":         {"garbage"},
	} {
		if _, err := parseAnswerSpecs(specs, qs); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
