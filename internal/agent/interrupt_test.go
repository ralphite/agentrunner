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
	"github.com/ralphite/agentrunner/internal/protocol"
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

// redirectLLM blocks on attempt 1 (interrupted); on attempt 2 it records
// whether the request carries the steer text, proving the model saw it.
type redirectLLM struct {
	mu       sync.Mutex
	attempt  int
	sawSteer bool
	entered  chan struct{}
}

func (p *redirectLLM) Capabilities() provider.Capabilities { return provider.Capabilities{} }
func (p *redirectLLM) Complete(ctx context.Context, req provider.CompleteRequest) iter.Seq2[provider.StreamEvent, error] {
	return func(yield func(provider.StreamEvent, error) bool) {
		p.mu.Lock()
		p.attempt++
		a := p.attempt
		p.mu.Unlock()
		if a == 1 {
			select {
			case p.entered <- struct{}{}:
			default:
			}
			<-ctx.Done()
			yield(provider.StreamEvent{}, ctx.Err())
			return
		}
		for _, m := range req.Messages {
			for _, part := range m.Parts {
				if strings.Contains(part.Text, "other file instead") {
					p.mu.Lock()
					p.sawSteer = true
					p.mu.Unlock()
				}
			}
		}
		yield(provider.StreamEvent{Kind: provider.EventTextDelta, TextDelta: "ok, redirected"}, nil)
		yield(provider.StreamEvent{Kind: provider.EventFinish, Finish: provider.FinishEndTurn}, nil)
	}
}

// S4.2 / DESIGN §1: a steering interrupt during a stuck LLM call cancels it and
// ENDS THE TURN — with nothing queued the session idles (the user has regained
// control), the model is NOT re-invoked. interrupt never fails or ends the
// session.
func TestSteeringInterruptDuringLLM(t *testing.T) {
	root := t.TempDir()
	l := testLoop(t, scripted.Fixture{}, root)
	prov := &blockingLLM{entered: make(chan struct{}, 1)}
	l.Provider = prov
	interrupts := make(chan protocol.CommandRef, 1)
	l.CommandInterrupts = interrupts
	// A live (empty) input source: the loop parks at idle rather than
	// returning, mirroring a hosted session — the model must not be re-run.
	inputs := make(chan protocol.UserInput)
	l.UserInputs = inputs
	sink := &captureSink{}
	l.Out = sink

	done := make(chan error, 1)
	go func() {
		_, err := l.Run(context.Background(), "answer")
		done <- err
	}()
	<-prov.entered // LLM is streaming and stuck
	interrupts <- protocol.CommandRef{CommandID: "cmd-interrupt", CommandSeq: 3}
	// The turn ends and the session idles; close the input channel so the
	// idle resolves into a clean return.
	time.Sleep(200 * time.Millisecond)
	close(inputs)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("steering interrupt must not fail the run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not complete after steering interrupt")
	}
	// The model was NOT re-run: interrupt stopped the turn (the old behavior
	// silently re-ran and completed on attempt 2).
	prov.mu.Lock()
	attempts := prov.attempt
	prov.mu.Unlock()
	if attempts != 1 {
		t.Fatalf("model re-invoked after interrupt (attempts=%d): interrupt must stop the turn", attempts)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var sawCancel, sawInterrupt, sawTrunc bool
	for _, e := range events {
		if e.Type == event.TypeActivityCancelled {
			sawCancel = true
		}
		if e.Type == event.TypeInputReceived && strings.Contains(string(e.Payload), "interrupt") {
			sawInterrupt = true
		}
		if e.Type == event.TypeLimitExceeded && strings.Contains(string(e.Payload), "interrupted") {
			if e.CommandID != "cmd-interrupt" {
				t.Fatalf("interrupt truncation command receipt = %q", e.CommandID)
			}
			sawTrunc = true
		}
	}
	if !sawCancel || !sawInterrupt || !sawTrunc {
		t.Fatalf("expected cancelled activity + interrupt input + interrupted truncation (cancel=%v interrupt=%v trunc=%v)", sawCancel, sawInterrupt, sawTrunc)
	}
	kinds := strings.Join(sink.kinds(), ",")
	if !strings.Contains(kinds, "discard") {
		t.Errorf("surface should see the interrupt discard: %s", kinds)
	}
}

// DESIGN §1: a steering interrupt with a queued steer REDIRECTS — the
// interrupted turn ends and a fresh turn consumes the steer, which the model
// sees. Proves the mid-run redirect path (busy-send steer becomes visible).
func TestSteeringInterruptRedirects(t *testing.T) {
	root := t.TempDir()
	l := testLoop(t, scripted.Fixture{}, root)
	prov := &redirectLLM{entered: make(chan struct{}, 1)}
	l.Provider = prov
	interrupts := make(chan struct{}, 1)
	l.Interrupts = interrupts
	inputs := make(chan protocol.UserInput, 1)
	l.UserInputs = inputs

	done := make(chan error, 1)
	var res RunResult
	go func() {
		var err error
		res, err = l.Run(context.Background(), "answer")
		done <- err
	}()
	<-prov.entered // turn 1 LLM stuck
	// Queue the steer, THEN interrupt: finishInterrupt drains it after the
	// truncation mark, so decide() restarts a fresh turn consuming it.
	inputs <- protocol.UserInput{Text: "go read the other file instead", DeliverySeq: 1}
	interrupts <- struct{}{}
	// Wait for the fresh turn to run and see the steer, then close the input
	// channel so the post-redirect idle resolves into a clean return.
	deadline := time.After(10 * time.Second)
	for {
		prov.mu.Lock()
		saw := prov.sawSteer
		prov.mu.Unlock()
		if saw {
			break
		}
		select {
		case <-deadline:
			t.Fatal("model never saw the steer on a fresh turn: redirect broken")
		case <-time.After(20 * time.Millisecond):
		}
	}
	close(inputs)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("redirect must not fail: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return after redirect")
	}
	_ = res
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
