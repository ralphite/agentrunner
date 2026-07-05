package agent

import (
	"context"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// blockingLLM streams a delta then blocks until ctx is cancelled (a stuck
// model call the user interrupts), then on the SECOND attempt completes.
type blockingLLM struct {
	mu      sync.Mutex
	attempt int
	entered chan struct{}
}

func (p *blockingLLM) Capabilities() provider.Capabilities { return provider.Capabilities{} }
func (p *blockingLLM) Complete(ctx context.Context, _ provider.CompleteRequest) iter.Seq2[provider.StreamEvent, error] {
	return func(yield func(provider.StreamEvent, error) bool) {
		p.mu.Lock()
		p.attempt++
		a := p.attempt
		p.mu.Unlock()
		if a == 1 {
			yield(provider.StreamEvent{Kind: provider.EventTextDelta, TextDelta: "thinking..."}, nil)
			select {
			case p.entered <- struct{}{}:
			default:
			}
			<-ctx.Done() // stuck until interrupted
			yield(provider.StreamEvent{}, ctx.Err())
			return
		}
		yield(provider.StreamEvent{Kind: provider.EventTextDelta, TextDelta: "done"}, nil)
		yield(provider.StreamEvent{Kind: provider.EventFinish, Finish: provider.FinishEndTurn}, nil)
	}
}

// S4.2: a steering interrupt during a stuck LLM call cancels it and the run
// CONTINUES (re-runs the turn), rather than aborting.
func TestSteeringInterruptDuringLLM(t *testing.T) {
	root := t.TempDir()
	l := testLoop(t, scripted.Fixture{}, root)
	prov := &blockingLLM{entered: make(chan struct{}, 1)}
	l.Provider = prov
	interrupts := make(chan struct{}, 1)
	l.Interrupts = interrupts
	sink := &captureSink{}
	l.Out = sink

	done := make(chan error, 1)
	var res RunResult
	go func() {
		var err error
		res, err = l.Run(context.Background(), "answer")
		done <- err
	}()
	<-prov.entered           // LLM is streaming and stuck
	interrupts <- struct{}{} // Ctrl-C
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("steering interrupt must not fail the run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not complete after steering interrupt")
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var sawCancel, sawInterrupt bool
	for _, e := range events {
		if e.Type == event.TypeActivityCancelled {
			sawCancel = true
		}
		if e.Type == event.TypeInputReceived && strings.Contains(string(e.Payload), "interrupt") {
			sawInterrupt = true
		}
	}
	if !sawCancel || !sawInterrupt {
		t.Fatalf("expected cancelled activity + interrupt input (cancel=%v interrupt=%v)", sawCancel, sawInterrupt)
	}
	kinds := strings.Join(sink.kinds(), ",")
	if !strings.Contains(kinds, "discard") {
		t.Errorf("surface should see the interrupt discard: %s", kinds)
	}
}

// S4.2 + 500ms: a steering interrupt kills a running bash tool quickly and
// the run continues; the model sees [interrupted by user].
func TestSteeringInterruptKillsBashFast(t *testing.T) {
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })

	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "bash",
				Args: map[string]any{"command": "echo $$ > pid.txt; sleep 30"}}},
			{Finish: "tool_use"},
		}},
		{
			Expect:  scripted.Expect{LastMessageContains: "[interrupted by user]"},
			Respond: []scripted.Event{{Text: "ok, stopped"}, {Finish: "end_turn"}},
		},
	}}
	interrupts := make(chan struct{}, 1)
	l := &Loop{
		Spec: &AgentSpec{Name: "b", Model: ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 50},
			SystemPrompt: "s", Tools: []string{"bash"}, MaxGenerationSteps: 5},
		Provider:   scripted.New(fix),
		Exec:       &tool.Executor{WS: ws},
		Store:      es,
		SessionID:  "interrupt-bash",
		Interrupts: interrupts,
	}

	done := make(chan error, 1)
	go func() { _, err := l.Run(context.Background(), "run something slow"); done <- err }()

	// Wait for the bash pid marker, then interrupt and time the kill.
	pidPath := filepath.Join(root, "pid.txt")
	for {
		if _, err := os.Stat(pidPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	interrupts <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("interrupt took %s — 500ms grace not applied", elapsed)
	}

	events, err := store.ReadEvents(es.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var sawCancel bool
	for _, e := range events {
		if e.Type == event.TypeActivityCancelled {
			sawCancel = true
		}
	}
	if !sawCancel {
		t.Fatal("bash activity should be cancelled")
	}
}
