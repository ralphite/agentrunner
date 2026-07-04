package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
)

// S4.4c: renderEnvBlock is deterministic and freezes the date — the same
// inputs render the same bytes, and only the date field reflects the frozen
// clock (never "now").
func TestRenderEnvBlockDeterministic(t *testing.T) {
	at := time.Date(2026, 7, 3, 14, 30, 0, 0, time.UTC)
	got := renderEnvBlock("/work/ws", at)
	want := "<env>\ncwd: /work/ws\ndate: 2026-07-03\n</env>"
	if got != want {
		t.Fatalf("env block = %q, want %q", got, want)
	}
	// A different wall-clock time on the SAME day must not change the block:
	// only the date is frozen in, so caching survives intra-day turns.
	if later := renderEnvBlock("/work/ws", at.Add(6*time.Hour)); later != got {
		t.Errorf("env block drifted within the day: %q vs %q", later, got)
	}
	if renderEnvBlock("", at) != "" {
		t.Errorf("empty cwd must yield no env block")
	}
}

// S4.4c: the cacheable prompt prefix (frozen env block + spec system prompt)
// is BYTE-IDENTICAL across turns as the conversation grows — the invariant
// that makes prompt caching economical. Only appended messages change.
func TestAssemblyPrefixByteStable(t *testing.T) {
	spec := &AgentSpec{
		Name:         "cache",
		Model:        ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
		SystemPrompt: "be precise",
		Tools:        []string{"read_file"},
	}
	env := renderEnvBlock("/work/ws", time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC))

	s := state.New()
	s = mustApply(t, s, event.TypeRunStarted, &event.RunStarted{
		SpecName: spec.Name, Model: spec.Model.ID, Env: env,
		SubStateVersions: state.SubStateVersions(),
	})
	s = mustApply(t, s, event.TypeInputReceived, &event.InputReceived{Text: "first task", Source: "cli"})
	s = mustApply(t, s, event.TypeTurnStarted, &event.TurnStarted{Turn: 1})

	req1 := Assemble(s, spec, nil, 1)

	// Grow the conversation by a couple of turns' worth of messages.
	s = mustApply(t, s, event.TypeAssistantMessage, &event.AssistantMessage{
		Turn: 1, Message: provider.Message{Role: provider.RoleAssistant,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "sure"}}}})
	s = mustApply(t, s, event.TypeTurnStarted, &event.TurnStarted{Turn: 2})
	s = mustApply(t, s, event.TypeInputReceived, &event.InputReceived{Text: "more", Source: "cli"})

	req2 := Assemble(s, spec, nil, 2)

	if req1.System != req2.System {
		t.Fatalf("system prefix drifted across turns:\n t1: %q\n t2: %q", req1.System, req2.System)
	}
	if !strings.HasPrefix(req1.System, env) {
		t.Errorf("env block is not the prefix: %q", req1.System)
	}
	// The prefix is stable, but the transcript is not — turn 2 sees more.
	if len(req2.Messages) <= len(req1.Messages) {
		t.Errorf("expected transcript to grow: t1=%d t2=%d", len(req1.Messages), len(req2.Messages))
	}
}

func mustApply(t *testing.T, s state.State, typ string, payload any) state.State {
	t.Helper()
	next, err := state.Apply(s, mustEnvOf(t, typ, payload))
	if err != nil {
		t.Fatal(err)
	}
	return next
}
