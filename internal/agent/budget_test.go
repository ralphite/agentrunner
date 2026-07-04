package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// 3.7b: reservations enter the fold with the allow resolution and are
// released by the activity's terminal event; usage settles via Run.Usage.
func TestBudgetReserveSettleLifecycle(t *testing.T) {
	s := New3_7State(t)
	if got := s.Budget.ReservedTotal(); got != 0 {
		t.Fatalf("initial reserved = %d", got)
	}
	var err error
	s, err = state.Apply(s, mustEnvOf(t, event.TypeEffectResolved, &event.EffectResolved{
		EffectID: "eff-llm-t1", Verdict: event.VerdictAllow, ReservedTokens: 4096,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Budget.ReservedTotal(); got != 4096 {
		t.Fatalf("reserved = %d, want 4096", got)
	}
	s, err = state.Apply(s, mustEnvOf(t, event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: "llm-t1", Kind: event.KindLLM, Attempt: 1,
	}))
	if err != nil {
		t.Fatal(err)
	}
	s, err = state.Apply(s, mustEnvOf(t, event.TypeActivityCompleted, &event.ActivityCompleted{
		ActivityID: "llm-t1", Usage: &usage100,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Budget.ReservedTotal(); got != 0 {
		t.Fatalf("reservation not released: %d", got)
	}
	if got := s.Run.Usage.InputTokens + s.Run.Usage.OutputTokens; got != 100 {
		t.Fatalf("settled = %d, want 100", got)
	}
}

// 3.7d TOCTOU (synthetic): two effects adjudicated before either settles —
// the second MUST see the first's reservation. Serialized by a mutex to
// emulate the S4.3 concurrent adjudicators sharing one fold.
func TestBudgetTOCTOUSyntheticConcurrency(t *testing.T) {
	gate := &pipeline.BudgetGate{MaxTotalTokens: 1000}
	var mu sync.Mutex
	reserved := 0
	granted := 0

	adjudicate := func() {
		mu.Lock() // the loop's appendE serialization, in miniature
		defer mu.Unlock()
		d := gate.Check(context.Background(), pipeline.Effect{
			ID: "eff-x", Kind: "llm_call", EstTokens: 600,
			Budget: pipeline.BudgetView{SettledTokens: 0, ReservedTokens: reserved},
		})
		if d.Action == event.VerdictAllow {
			reserved += 600
			granted++
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); adjudicate() }()
	}
	wg.Wait()
	if granted != 1 {
		t.Fatalf("granted = %d, want exactly 1 (600+600 > 1000 must not double-commit)", granted)
	}
}

// 3.7c: budget exhaustion ends the run GRACEFULLY — LimitExceeded fact,
// epilogue, run_ended{limit_exceeded} — never a crash or a hard abort.
func TestBudgetGracefulEnding(t *testing.T) {
	// Turn 1 spends 900 of a 1000-token budget; turn 2's reservation
	// (max_tokens 200) cannot fit.
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: "reading"},
			{ToolCall: &scripted.ToolCallEvent{Name: "read_file", Args: map[string]any{"path": "a.txt"}}},
			{Usage: &scripted.UsageEvent{InputTokens: 500, OutputTokens: 400}},
			{Finish: "tool_use"},
		}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.Model.MaxTokens = 200
	l.Spec.Budget.MaxTotalTokens = 1000
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		&pipeline.BudgetGate{MaxTotalTokens: 1000},
	}}

	res, err := l.Run(context.Background(), "read a")
	if err != nil {
		t.Fatalf("budget ending must be graceful, got error: %v", err)
	}
	if res.Reason != "limit_exceeded" {
		t.Fatalf("res = %+v", res)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var sawLimit bool
	for _, e := range events {
		if e.Type == event.TypeLimitExceeded {
			sawLimit = true
			if !strings.Contains(string(e.Payload), `"limit":1000`) {
				t.Errorf("limit payload = %s", e.Payload)
			}
		}
	}
	if !sawLimit {
		t.Fatal("limit_exceeded fact missing")
	}
	if last := events[len(events)-1]; last.Type != event.TypeRunEnded ||
		!strings.Contains(string(last.Payload), "limit_exceeded") {
		t.Fatalf("last event = %s %s", last.Type, last.Payload)
	}
}

// S4.4c: the budget bills input + output − cache_read. A turn whose input
// is mostly a cached prefix must NOT be charged the full input rate, so a
// run that would overflow on raw input+output stays under budget once the
// cache read is discounted.
func TestBudgetBillsCacheReadDiscount(t *testing.T) {
	// Raw input+output = 900+200 = 1100 > 1000 budget; but 800 of the input
	// is a cache read, so billed = 1100 − 800 = 300. The run completes.
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: "cached hit"},
			{Usage: &scripted.UsageEvent{InputTokens: 900, OutputTokens: 200, CacheReadTokens: 800}},
			{Finish: "end_turn"},
		}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.Model.MaxTokens = 100
	l.Spec.Budget.MaxTotalTokens = 1000
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		&pipeline.BudgetGate{MaxTotalTokens: 1000},
	}}

	res, err := l.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("cache-discounted run must complete, got: %v", err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v (raw 1100 would exceed 1000; billed 300 must not)", res)
	}
	if got := res.Usage.Billed(); got != 300 {
		t.Errorf("billed = %d, want 300 (900+200−800)", got)
	}
}

// Billed clamps at zero and never credits the budget.
func TestUsageBilledClamp(t *testing.T) {
	u := provider.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 500}
	if got := u.Billed(); got != 0 {
		t.Errorf("billed = %d, want 0 (clamped)", got)
	}
}

// Bypass mode: the budget does not bind (3.6d).
func TestBudgetGateBypass(t *testing.T) {
	gate := &pipeline.BudgetGate{MaxTotalTokens: 100}
	d := gate.Check(context.Background(), pipeline.Effect{
		EstTokens: 10000, Mode: pipeline.ModeBypass,
	})
	if d.Action != event.VerdictAllow {
		t.Fatalf("bypass must not bind: %+v", d)
	}
}

// helpers

var usage100 = provider.Usage{InputTokens: 60, OutputTokens: 40}

func New3_7State(t *testing.T) state.State {
	t.Helper()
	return state.New()
}

func mustEnvOf(t *testing.T, typ string, payload any) event.Envelope {
	t.Helper()
	env, err := event.New(typ, payload)
	if err != nil {
		t.Fatal(err)
	}
	return env
}
