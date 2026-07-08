package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

func eventTypes(t *testing.T, dir string) []string {
	t.Helper()
	events, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, e := range events {
		types = append(types, e.Type)
	}
	return types
}

// A provider error mid-run must surface wrapped with the turn number AND
// leave a terminal task_completed{error} event (a failed log must be
// distinguishable from a truncated one). The failure itself must be
// journaled as activity_failed before the terminal event.
func TestLoopProviderErrorWritesTerminalRecord(t *testing.T) {
	// One scripted step; the second Complete call exhausts the fixture.
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "bash", Args: map[string]any{"command": "true"}}},
			{Finish: "tool_use"},
		}},
	}}

	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sessDir := filepath.Join(t.TempDir(), "sess")
	es, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()

	loop := &Loop{
		Spec: &AgentSpec{Name: "t", Model: ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 10},
			SystemPrompt: "s", Tools: []string{"bash"}, MaxGenerationSteps: 5},
		Provider:  scripted.New(fix),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)),
		SessionID: "sess-err",
	}

	_, err = loop.Run(context.Background(), "loop until exhausted")
	if err == nil || !strings.Contains(err.Error(), "turn 2") {
		t.Fatalf("err = %v, want wrapped turn 2", err)
	}

	types := eventTypes(t, sessDir)
	if len(types) == 0 || types[len(types)-1] != event.TypeTaskCompleted {
		t.Fatalf("event types = %v, want terminal task_completed", types)
	}
	var sawFailed bool
	for _, typ := range types {
		if typ == event.TypeActivityFailed {
			sawFailed = true
		}
	}
	if !sawFailed {
		t.Errorf("event types = %v, want activity_failed for the exhausted provider call", types)
	}
}

// pumpBackoffs advances the fake clock past every retry backoff until the
// run under test finishes.
func pumpBackoffs(fake *clock.Fake, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		default:
			if fake.Waiters() > 0 {
				fake.Advance(5 * time.Second)
			}
		}
	}
}

func emptyCompletionLoop(t *testing.T, fix scripted.Fixture, sessDir string, fake *clock.Fake) *Loop {
	t.Helper()
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })
	return &Loop{
		Spec: &AgentSpec{Name: "t", Model: ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 10},
			SystemPrompt: "s", MaxGenerationSteps: 5},
		Provider:  scripted.New(fix),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     fake,
		SessionID: "sess-empty",
	}
}

// A TRUNCATED empty completion (no text, no tool calls, cut off at the token
// cap — the Gemini defect that poisoned session journals) must NOT land as an
// assistant_message. It is a transient provider failure: the activity retries
// and the next attempt's message is the only one journaled. (A clean empty
// end_turn is legitimate and ends the turn — see TestEmptyCandidateEndsCleanly.)
func TestLoopEmptyCompletionRetriedNotJournaled(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Finish: "max_tokens"}}}, // empty, truncated at cap → retried
		{Respond: []scripted.Event{{Text: "recovered"}, {Finish: "end_turn"}}},
	}}
	sessDir := filepath.Join(t.TempDir(), "sess")
	fake := clock.NewFake(time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC))
	loop := emptyCompletionLoop(t, fix, sessDir, fake)

	done := make(chan struct{})
	var runErr error
	go func() { _, runErr = loop.Run(context.Background(), "hi"); close(done) }()
	pumpBackoffs(fake, done)
	if runErr != nil {
		t.Fatalf("run failed: %v", runErr)
	}

	events, err := store.ReadEvents(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	var asstCount int
	var sawRetryableFailure bool
	for _, e := range events {
		switch e.Type {
		case event.TypeAssistantMessage:
			asstCount++
			var p event.AssistantMessage
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				t.Fatal(err)
			}
			if len(p.Message.Parts) == 0 {
				t.Errorf("empty assistant_message journaled — the poisoning this fix removes")
			}
		case event.TypeActivityFailed:
			var p event.ActivityFailed
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				t.Fatal(err)
			}
			if p.Error.Class == string(errs.ProviderServer) && !p.Final {
				sawRetryableFailure = true
			}
		}
	}
	if asstCount != 1 {
		t.Errorf("assistant_message count = %d, want 1 (the recovered attempt only)", asstCount)
	}
	if !sawRetryableFailure {
		t.Errorf("want a non-final provider_server activity_failed for the empty attempt")
	}
}

// When the model keeps returning TRUNCATED empty completions, retries exhaust
// and the run ends in error — but the journal still holds NO empty
// assistant_message, so a later revive re-runs the turn instead of dying on
// assembly.
func TestLoopEmptyCompletionExhaustionLeavesCleanJournal(t *testing.T) {
	empty := scripted.Step{Respond: []scripted.Event{{Finish: "max_tokens"}}} // truncated empty → retried
	fix := scripted.Fixture{Steps: []scripted.Step{empty, empty, empty}}
	sessDir := filepath.Join(t.TempDir(), "sess")
	fake := clock.NewFake(time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC))
	loop := emptyCompletionLoop(t, fix, sessDir, fake)

	done := make(chan struct{})
	var runErr error
	go func() { _, runErr = loop.Run(context.Background(), "hi"); close(done) }()
	pumpBackoffs(fake, done)
	if runErr == nil || !strings.Contains(runErr.Error(), "empty message") {
		t.Fatalf("err = %v, want empty-message failure after retry exhaustion", runErr)
	}
	for _, typ := range eventTypes(t, sessDir) {
		if typ == event.TypeAssistantMessage {
			t.Errorf("no assistant_message may land when every completion was empty")
		}
	}
}

// An event append failure aborts the run with an error (the best-effort
// terminal event may also fail — the error must still propagate).
func TestLoopEventAppendFailureAborts(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	_ = es.Close() // every subsequent append fails

	loop := &Loop{
		Spec: &AgentSpec{Name: "t", Model: ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 10},
			SystemPrompt: "s", MaxGenerationSteps: 3},
		Provider:  scripted.New(fix),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)),
		SessionID: "sess-closed",
	}
	if _, err := loop.Run(context.Background(), "hi"); err == nil {
		t.Fatal("expected error from failing event appends")
	}
}
