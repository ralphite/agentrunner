package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
)

// collectProgress returns an AppendFunc that records journaled payloads.
func collectProgress(t *testing.T, got *[]*event.ProgressUpdated) AppendFunc {
	t.Helper()
	return func(typ string, payload any) (event.Envelope, error) {
		if typ != event.TypeProgressUpdated {
			t.Fatalf("unexpected event type %q", typ)
		}
		*got = append(*got, payload.(*event.ProgressUpdated))
		return event.Envelope{}, nil
	}
}

func TestProgressToolNormalizesAndJournals(t *testing.T) {
	var got []*event.ProgressUpdated
	args := json.RawMessage(`{"items":[
		{"id":"tests","title":"run the suite","status":"in_progress"},
		{"id":"docs","title":"update SPEC","status":"todo"},
		{"id":"ship","title":"push","status":"completed"}
	]}`)
	res := runProgressTool(args, collectProgress(t, &got))
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Payload)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 journaled event, got %d", len(got))
	}
	want := []string{"running", "pending", "done"}
	for i, w := range want {
		if got[0].Items[i].Status != w {
			t.Errorf("item %d status: want %q got %q", i, w, got[0].Items[i].Status)
		}
	}
	var out struct {
		Items int `json:"items"`
	}
	if err := json.Unmarshal(res.Payload, &out); err != nil || out.Items != 3 {
		t.Fatalf("result must carry the count: %s", res.Payload)
	}
	if strings.Contains(string(res.Payload), "run the suite") {
		t.Fatalf("result must not echo the table back: %s", res.Payload)
	}
}

func TestProgressToolRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"unknown status": `{"items":[{"id":"a","title":"x","status":"meh"}]}`,
		"duplicate id":   `{"items":[{"id":"a","title":"x","status":"done"},{"id":"a","title":"y","status":"done"}]}`,
		"empty title":    `{"items":[{"id":"a","title":"  ","status":"done"}]}`,
		"empty id":       `{"items":[{"id":"","title":"x","status":"done"}]}`,
		"not json":       `nope`,
	}
	for name, args := range cases {
		var got []*event.ProgressUpdated
		res := runProgressTool(json.RawMessage(args), collectProgress(t, &got))
		if !res.IsError {
			t.Errorf("%s: want model-visible error, got %s", name, res.Payload)
		}
		if len(got) != 0 {
			t.Errorf("%s: rejected input must not journal", name)
		}
	}
}

func TestProgressToolLimitsAndEmptyClear(t *testing.T) {
	// Over the item cap → error, nothing journaled.
	var rows []string
	for i := 0; i <= progressMaxItems; i++ {
		rows = append(rows, fmt.Sprintf(`{"id":"i%d","title":"t","status":"done"}`, i))
	}
	var got []*event.ProgressUpdated
	over := json.RawMessage(`{"items":[` + strings.Join(rows, ",") + `]}`)
	if res := runProgressTool(over, collectProgress(t, &got)); !res.IsError || len(got) != 0 {
		t.Fatal("want error above the item cap, nothing journaled")
	}
	// Empty list is a lawful clear.
	got = nil
	res := runProgressTool(json.RawMessage(`{"items":[]}`), collectProgress(t, &got))
	if res.IsError || len(got) != 1 || len(got[0].Items) != 0 {
		t.Fatalf("empty list must journal a clear: err=%v events=%v", res.IsError, got)
	}
	// Long id/title clamp, not error.
	got = nil
	long := strings.Repeat("z", 500)
	res = runProgressTool(json.RawMessage(`{"items":[{"id":"`+long+`","title":"`+long+`","status":"done"}]}`), collectProgress(t, &got))
	if res.IsError {
		t.Fatalf("long fields must clamp, not error: %s", res.Payload)
	}
	it := got[0].Items[0]
	if len(it.ID) != progressMaxID || len(it.Title) != progressMaxTitle {
		t.Fatalf("clamp sizes wrong: id=%d title=%d", len(it.ID), len(it.Title))
	}
}

// TestProgressFoldReplacesWholesale proves the journal → fold half: each
// ProgressUpdated replaces the whole table, an empty one clears it, and the
// projection round-trips through the real event envelope (decode included).
func TestProgressFoldReplacesWholesale(t *testing.T) {
	mustEnv := func(payload *event.ProgressUpdated) event.Envelope {
		e, err := event.New(event.TypeProgressUpdated, payload)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	s := state.New()
	var err error
	s, err = state.Apply(s, mustEnv(&event.ProgressUpdated{Items: []event.ProgressItem{
		{ID: "a", Title: "first", Status: "running"},
		{ID: "b", Title: "second", Status: "pending"},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Session.Progress) != 2 || s.Session.Progress[0].Status != "running" {
		t.Fatalf("fold missed the table: %+v", s.Session.Progress)
	}
	s, err = state.Apply(s, mustEnv(&event.ProgressUpdated{Items: []event.ProgressItem{
		{ID: "a", Title: "first", Status: "done"},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Session.Progress) != 1 || s.Session.Progress[0].Status != "done" {
		t.Fatalf("wholesale replace failed: %+v", s.Session.Progress)
	}
	s, err = state.Apply(s, mustEnv(&event.ProgressUpdated{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Session.Progress) != 0 {
		t.Fatalf("empty event must clear: %+v", s.Session.Progress)
	}
}
