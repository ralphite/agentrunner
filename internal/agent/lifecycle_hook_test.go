package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// lifecycleLoop builds a conversational loop with lifecycle hooks configured
// (INC-15). Hooks run with cwd = workspace root, so relative marker files
// land inside root.
func lifecycleLoop(t *testing.T, root string, lifecycle map[string][]string, fix scripted.Fixture) (*Loop, *store.EventStore, chan protocol.UserInput, chan error) {
	t.Helper()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })
	inbox := make(chan protocol.UserInput, 4)
	l := &Loop{
		Spec: &AgentSpec{
			Name:               "lc",
			Model:              ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			Tools:              []string{"bash"},
			MaxGenerationSteps: 6,
		},
		Provider:   scripted.New(fix),
		Exec:       &tool.Executor{WS: ws},
		Store:      es,
		Clock:      clock.NewFake(time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)),
		SessionID:  "lc-sess",
		UserInputs: inbox,
		Hooks:      &hook.Runner{Lifecycle: lifecycle, Dir: root},
	}
	done := make(chan error, 1)
	go func() { _, e := l.Run(context.Background(), "first question"); done <- e }()
	return l, es, inbox, done
}

// Observe events fire at their journal points with the event JSON on stdin.
func TestLifecycleHooksFire(t *testing.T) {
	root := t.TempDir()
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answer"}, {Finish: "end_turn"}}},
	}}
	_, es, inbox, done := lifecycleLoop(t, root, map[string][]string{
		hook.EventSessionStart: {"cat > started.json"},
		hook.EventStop:         {"cat > stopped.json"},
	}, fix)

	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	started, err := os.ReadFile(filepath.Join(root, "started.json"))
	if err != nil {
		t.Fatalf("session_start hook never fired: %v", err)
	}
	if !strings.Contains(string(started), `"event":"session_start"`) ||
		!strings.Contains(string(started), `"session":"lc-sess"`) {
		t.Errorf("session_start payload = %s", started)
	}
	stopped, err := os.ReadFile(filepath.Join(root, "stopped.json"))
	if err != nil {
		t.Fatalf("stop hook never fired: %v", err)
	}
	if !strings.Contains(string(stopped), `"event":"stop"`) {
		t.Errorf("stop payload = %s", stopped)
	}
}

// A user_prompt_submit hook exiting 2 vetoes the input: it never journals
// and no turn runs for it.
func TestUserPromptSubmitHookBlocks(t *testing.T) {
	root := t.TempDir()
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answer one"}, {Finish: "end_turn"}}},
	}}
	_, es, inbox, done := lifecycleLoop(t, root, map[string][]string{
		// Veto anything mentioning "forbidden"; allow the rest (the first
		// prompt must pass or the run never starts).
		hook.EventUserPromptSubmit: {`grep -q forbidden && { echo "policy: forbidden topic" >&2; exit 2; } || exit 0`},
	}, fix)

	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	inbox <- protocol.UserInput{Text: "this is forbidden content"}
	// Give the veto a beat, then close; a journaled second input would start
	// turn 2 and exhaust the fixture (which would fail the run).
	time.Sleep(300 * time.Millisecond)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, es.Dir(), event.TypeInputReceived); n != 1 {
		t.Fatalf("InputReceived = %d, want 1 (blocked prompt must not journal)", n)
	}
	if n := countEvents(t, es.Dir(), event.TypeGenerationStarted); n != 1 {
		t.Fatalf("generation_started = %d, want 1", n)
	}
}

func TestDurableOpeningHookVetoDoesNotStartEmptyTurn(t *testing.T) {
	root := t.TempDir()
	l := testLoop(t, scripted.Fixture{}, root)
	l.DurableOpening = true
	l.Hooks = &hook.Runner{Lifecycle: map[string][]string{
		hook.EventUserPromptSubmit: {"echo blocked >&2; exit 2"},
	}, Dir: root}
	res, err := l.Run(context.Background(), "forbidden opening")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "waiting_input" {
		t.Fatalf("result = %+v, want waiting_input", res)
	}
	if n := countEvents(t, l.Store.Dir(), event.TypeInputReceived); n != 0 {
		t.Fatalf("InputReceived = %d, want 0", n)
	}
	if n := countEvents(t, l.Store.Dir(), event.TypeGenerationStarted); n != 0 {
		t.Fatalf("GenerationStarted = %d, want 0", n)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	folded, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if folded.Session.ConsumedInputSeq != 1 {
		t.Fatalf("consumed input seq = %d, want vetoed command settled", folded.Session.ConsumedInputSeq)
	}
	if !folded.Session.OpeningRejected {
		t.Fatal("opening veto is not durably parked")
	}
	res, err = l.Resume(context.Background())
	if err != nil || res.Reason != "waiting_input" {
		t.Fatalf("resume after veto = %+v err=%v", res, err)
	}
	if n := countEvents(t, l.Store.Dir(), event.TypeGenerationStarted); n != 0 {
		t.Fatalf("GenerationStarted after resume = %d, want 0", n)
	}
}

// A pre_compact veto skips the compaction AND the loop keeps making progress
// (the auto path must not spin on a standing veto).
func TestPreCompactHookSkipsAndNoSpin(t *testing.T) {
	root := t.TempDir()
	big := strings.Repeat("verbose text that inflates the running context a lot. ", 120) // ~6KB
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: big},
			{ToolCall: &scripted.ToolCallEvent{Name: "bash", Args: map[string]any{"command": "true"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })
	l := &Loop{
		Spec: &AgentSpec{
			Name: "pc",
			Model: ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100,
				CompactAtTokens: 500, MicrocompactAtTokens: -1},
			Tools:              []string{"bash"},
			MaxGenerationSteps: 4,
		},
		Provider:  scripted.New(fix),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)),
		SessionID: "pc-sess",
		Hooks:     &hook.Runner{Lifecycle: map[string][]string{hook.EventPreCompact: {"exit 2"}}, Dir: root},
	}
	if _, err := l.Run(context.Background(), "please elaborate"); err != nil {
		t.Fatal(err) // a spin would exhaust the fixture and error here
	}
	if n := countEvents(t, es.Dir(), event.TypeContextCompacted); n != 0 {
		t.Fatalf("ContextCompacted = %d, want 0 (vetoed)", n)
	}
	if n := countEvents(t, es.Dir(), event.TypeAssistantMessage); n < 2 {
		t.Fatalf("assistant messages = %d, want the run to have progressed past the veto", n)
	}
}

// A broken observe hook (non-zero exit) must never veto work.
func TestObserveHookFailureDoesNotBlock(t *testing.T) {
	root := t.TempDir()
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "answer"}, {Finish: "end_turn"}}},
	}}
	_, es, inbox, done := lifecycleLoop(t, root, map[string][]string{
		hook.EventSessionStart: {"exit 7"},
		hook.EventStop:         {"exit 2"}, // even the block code is inert on observe events
	}, fix)
	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, es.Dir(), event.TypeAssistantMessage); n != 1 {
		t.Fatalf("assistant messages = %d, want 1 (run unaffected by broken observe hooks)", n)
	}
}
