package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/state"
)

// memAppend collects appended events in memory.
type memAppend struct {
	events []event.Envelope
}

func (m *memAppend) append(typ string, payload any) (event.Envelope, error) {
	env, err := event.New(typ, payload)
	if err != nil {
		return env, err
	}
	env.Seq = int64(len(m.events) + 1)
	env.ID = event.EventID(env.Seq)
	m.events = append(m.events, env)
	return env, nil
}

func (m *memAppend) types() []string {
	var out []string
	for _, e := range m.events {
		out = append(out, e.Type)
	}
	return out
}

func testExecutor(m *memAppend) *ActivityExecutor {
	return &ActivityExecutor{
		Append: m.append,
		Clock:  clock.NewFake(time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)),
		Redact: redact.FromEnv(),
	}
}

// Credentials never reach the journal: args and results are redacted.
func TestActivityRedaction(t *testing.T) {
	t.Setenv("SNEAKY_API_KEY", "hunter2-secret-value")
	m := &memAppend{}
	x := testExecutor(m)
	x.Redact = redact.FromEnv() // rebuild after Setenv

	err := x.Do(context.Background(), Activity{
		ID: "tool-call_1_0", Kind: event.KindTool, Name: "bash",
		Args:   json.RawMessage(`{"command":"echo hunter2-secret-value"}`),
		CallID: "call_1_0",
		Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			return json.RawMessage(`{"output":"hunter2-secret-value done"}`), nil, false, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(m.events)
	if strings.Contains(string(raw), "hunter2-secret-value") {
		t.Fatalf("credential leaked into events: %s", raw)
	}
	if !strings.Contains(string(raw), "[REDACTED:SNEAKY_API_KEY]") {
		t.Fatalf("expected redaction marker: %s", raw)
	}
}

// A retryable failure retries with backoff through the Clock and each
// attempt is its own Started/Failed pair; success on attempt 2 completes.
func TestActivityRetryOnRetryable(t *testing.T) {
	m := &memAppend{}
	x := testExecutor(m)
	fake := clock.NewFake(time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC))
	x.Clock = fake

	attempts := 0
	done := make(chan error, 1)
	go func() {
		done <- x.Do(context.Background(), Activity{
			ID: "llm-t1", Kind: event.KindLLM, Name: "complete",
			Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
				attempts++
				if attempts == 1 {
					return nil, nil, false, errs.New(errs.ProviderRateLimit, "429")
				}
				return nil, nil, false, nil
			},
		})
	}()
	for fake.Waiters() == 0 { // parked on the 1s backoff
	}
	fake.Advance(time.Second)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	want := []string{"activity_started", "activity_failed", "activity_started", "activity_completed"}
	if got := m.types(); !equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	// Attempt numbers climb.
	var started event.ActivityStarted
	_ = json.Unmarshal(m.events[2].Payload, &started)
	if started.Attempt != 2 {
		t.Errorf("second attempt = %d", started.Attempt)
	}
}

// Non-retryable failures stop after one attempt.
func TestActivityNoRetryOnFatal(t *testing.T) {
	m := &memAppend{}
	x := testExecutor(m)
	err := x.Do(context.Background(), Activity{
		ID: "llm-t1", Kind: event.KindLLM, Name: "complete",
		Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			return nil, nil, false, errs.New(errs.ProviderAuth, "401")
		},
	})
	if err == nil {
		t.Fatal("fatal error must surface")
	}
	want := []string{"activity_started", "activity_failed"}
	if got := m.types(); !equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

// Retries exhaust at MaxAttempts even for retryable classes.
func TestActivityRetryExhaustion(t *testing.T) {
	m := &memAppend{}
	x := testExecutor(m)
	x.MaxAttempts = 2
	x.Backoff = []time.Duration{0}
	err := x.Do(context.Background(), Activity{
		ID: "llm-t1", Kind: event.KindLLM, Name: "complete",
		Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			return nil, nil, false, errs.New(errs.ProviderServer, "503")
		},
	})
	if err == nil {
		t.Fatal("exhausted retries must surface the error")
	}
	if got := len(m.events); got != 4 { // 2× (started, failed)
		t.Fatalf("events = %v", m.types())
	}
}

// The idempotent flag is journaled verbatim — the 2.15 in-doubt policy
// reads it from ActivityStarted at resume time.
func TestActivityIdempotentFlagJournaled(t *testing.T) {
	m := &memAppend{}
	x := testExecutor(m)
	if err := x.Do(context.Background(), Activity{
		ID: "tool-call_1_0", Kind: event.KindTool, Name: "read_file",
		CallID: "call_1_0", Idempotent: true,
		Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			return json.RawMessage(`{}`), nil, false, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	var started event.ActivityStarted
	if err := json.Unmarshal(m.events[0].Payload, &started); err != nil {
		t.Fatal(err)
	}
	if !started.Idempotent {
		t.Error("idempotent flag lost")
	}
}

// A model-visible error result (isError) is a SUCCESSFUL activity.
func TestActivityIsErrorIsCompletion(t *testing.T) {
	m := &memAppend{}
	x := testExecutor(m)
	if err := x.Do(context.Background(), Activity{
		ID: "tool-call_1_0", Kind: event.KindTool, Name: "read_file", CallID: "call_1_0",
		Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			return json.RawMessage(`"no such file"`), nil, true, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	var completed event.ActivityCompleted
	if err := json.Unmarshal(m.events[1].Payload, &completed); err != nil {
		t.Fatal(err)
	}
	if !completed.IsError {
		t.Error("is_error lost")
	}
}

// DiscardOnRetry (the S4 TurnDiscarded seam) fires before each retry.
func TestActivityDiscardSeam(t *testing.T) {
	m := &memAppend{}
	x := testExecutor(m)
	x.Backoff = []time.Duration{0}
	discards := 0
	x.DiscardOnRetry = func() error { discards++; return nil }
	attempts := 0
	if err := x.Do(context.Background(), Activity{
		ID: "llm-t1", Kind: event.KindLLM, Name: "complete",
		Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			attempts++
			if attempts < 3 {
				return nil, nil, false, errs.New(errs.Timeout, "slow")
			}
			return nil, nil, false, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if discards != 2 {
		t.Errorf("discards = %d, want 2", discards)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// 3.9 loop continuity: a tool activity that fails TERMINALLY resolves its
// call with the rendered error (fold), so decide() moves past it instead
// of re-running — the model reacts on its next turn.
func TestFinalToolFailureRendersAndResolves(t *testing.T) {
	m := &memAppend{}
	x := testExecutor(m)
	x.MaxAttempts = 1

	ds := &driveState{s: state.New()}
	x.Append = func(typ string, payload any) (event.Envelope, error) {
		env, err := m.append(typ, payload)
		if err != nil {
			return env, err
		}
		ds.s, err = state.Apply(ds.s, env)
		return env, err
	}

	err := x.Do(context.Background(), Activity{
		ID: "tool-call_1_0", Kind: event.KindTool, Name: "mcp_search",
		CallID: "call_1_0",
		Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			return nil, nil, false, errs.New(errs.ToolFailed, "backend unreachable")
		},
	})
	if err == nil {
		t.Fatal("terminal failure must surface to the caller")
	}
	tr, ok := ds.s.Conversation.ToolResults["call_1_0"]
	if !ok || !tr.IsError || !strings.Contains(string(tr.Result), "tool failed") {
		t.Fatalf("rendered result = %+v (ok=%v)", tr, ok)
	}
	if len(ds.s.Activities) != 0 {
		t.Fatalf("in-flight not drained: %+v", ds.s.Activities)
	}
}
