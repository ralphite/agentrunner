package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
)

// statsEnv builds a journaled envelope with an explicit timestamp — the
// reporting projection reads envelope TS, which the store normally assigns.
func statsEnv(t *testing.T, ts time.Time, typ string, payload any) event.Envelope {
	t.Helper()
	e, err := event.New(typ, payload)
	if err != nil {
		t.Fatal(err)
	}
	e.TS = ts
	return e
}

func TestBuildStatsAggregates(t *testing.T) {
	t0 := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	at := func(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }
	editResult, _ := json.Marshal(map[string]any{"output": "edited x", "lines_added": 5, "lines_removed": 2})
	evs := []event.Envelope{
		// LLM activity 0–10s.
		statsEnv(t, at(0), event.TypeActivityStarted, &event.ActivityStarted{ActivityID: "llm-1", Kind: event.KindLLM, Name: "generate"}),
		statsEnv(t, at(10), event.TypeActivityCompleted, &event.ActivityCompleted{ActivityID: "llm-1"}),
		// Two OVERLAPPING tool calls 10–20s and 12–30s: merged span 10–30.
		statsEnv(t, at(10), event.TypeActivityStarted, &event.ActivityStarted{ActivityID: "t-1", Kind: event.KindTool, Name: "bash", CallID: "c1"}),
		statsEnv(t, at(12), event.TypeActivityStarted, &event.ActivityStarted{ActivityID: "t-2", Kind: event.KindTool, Name: "edit_file", CallID: "c2"}),
		statsEnv(t, at(20), event.TypeActivityCompleted, &event.ActivityCompleted{ActivityID: "t-1", IsError: true}),
		statsEnv(t, at(30), event.TypeActivityCompleted, &event.ActivityCompleted{ActivityID: "t-2", Result: editResult}),
		// Idle gap (would be standby) then a failed tool 100–103s.
		statsEnv(t, at(100), event.TypeActivityStarted, &event.ActivityStarted{ActivityID: "t-3", Kind: event.KindTool, Name: "bash", CallID: "c3"}),
		statsEnv(t, at(103), event.TypeActivityFailed, &event.ActivityFailed{ActivityID: "t-3", Final: true, Error: event.ErrorInfo{Class: "runtime", Message: "x"}}),
	}
	st := buildStats(evs)
	if st == nil {
		t.Fatal("want stats")
	}
	if st.ToolCalls != 3 || st.ToolFailures != 2 {
		t.Fatalf("tool counts: %+v", st)
	}
	if b := st.Tools["bash"]; b == nil || b.Calls != 2 || b.Fail != 2 || b.Success != 0 {
		t.Fatalf("bash stat: %+v", b)
	}
	if e := st.Tools["edit_file"]; e == nil || e.Success != 1 || e.DurationMS != 18000 {
		t.Fatalf("edit stat: %+v", e)
	}
	if st.LinesAdded != 5 || st.LinesRemoved != 2 {
		t.Fatalf("line delta: %+v", st)
	}
	// Merged active spans: 0–30 (LLM+overlapping tools) + 100–103 = 33s;
	// the 70s standby gap must NOT count.
	if st.ActiveSeconds != 33 {
		t.Fatalf("active seconds: want 33, got %v", st.ActiveSeconds)
	}
}

func TestBuildStatsEmptyAndNoTS(t *testing.T) {
	if st := buildStats(nil); st != nil {
		t.Fatalf("no events must yield nil stats, got %+v", st)
	}
	// Envelopes without timestamps (older journals) still count calls,
	// just contribute no duration.
	evs := []event.Envelope{
		statsEnv(t, time.Time{}, event.TypeActivityStarted, &event.ActivityStarted{ActivityID: "t-1", Kind: event.KindTool, Name: "grep", CallID: "c1"}),
		statsEnv(t, time.Time{}, event.TypeActivityCompleted, &event.ActivityCompleted{ActivityID: "t-1"}),
	}
	st := buildStats(evs)
	if st == nil || st.ToolCalls != 1 || st.ActiveSeconds != 0 {
		t.Fatalf("no-TS journal: %+v", st)
	}
}
