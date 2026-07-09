package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// capsProvider wraps a provider to override the declared Capabilities, so the
// S4.7 downgrade path can be exercised without a real second provider.
type capsProvider struct {
	*capturingProvider
	caps provider.Capabilities
}

func (c *capsProvider) Capabilities() provider.Capabilities { return c.caps }

func runWithThinking(t *testing.T, caps provider.Capabilities) *capturingProvider {
	t.Helper()
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "ok"}, {Finish: "end_turn"}}},
	}}
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

	cap := &capturingProvider{inner: scripted.New(fix)}
	l := &Loop{
		Spec: &AgentSpec{
			Name: "think",
			Model: ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 2048,
				Thinking: ThinkingSpec{Enabled: true, BudgetTokens: 1024}},
			MaxGenerationSteps: 3,
		},
		Provider:  &capsProvider{capturingProvider: cap, caps: caps},
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)),
		SessionID: "think-sess",
	}
	if _, err := l.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	return cap
}

// S4.7: a provider that supports thinking receives the thinking config.
func TestThinkingPassedWhenSupported(t *testing.T) {
	cap := runWithThinking(t, provider.Capabilities{Thinking: true})
	requests := cap.Requests()
	if len(requests) == 0 {
		t.Fatal("no requests captured")
	}
	if !requests[0].Thinking.Enabled {
		t.Errorf("thinking config should reach a thinking-capable provider")
	}
}

// S4.7: a provider WITHOUT thinking gets the config stripped (explicit
// downgrade), not a request it cannot honor.
func TestThinkingDowngradedWhenUnsupported(t *testing.T) {
	cap := runWithThinking(t, provider.Capabilities{Thinking: false})
	requests := cap.Requests()
	if len(requests) == 0 {
		t.Fatal("no requests captured")
	}
	if requests[0].Thinking.Enabled {
		t.Errorf("thinking must be downgraded for a non-thinking provider: %+v", requests[0].Thinking)
	}
}
