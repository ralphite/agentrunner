package state

import (
	"encoding/json"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
)

func applyInteraction(t *testing.T, s State, seq int64, typ string, payload any) State {
	t.Helper()
	env, err := event.New(typ, payload)
	if err != nil {
		t.Fatal(err)
	}
	env.Seq = seq
	next, err := Apply(s, env)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestTurnItemProjectionPreservesTypedIngressAndToolItems(t *testing.T) {
	caps := provider.Envelope("gemini", "model", provider.Capabilities{Images: true})
	s := New()
	s = applyInteraction(t, s, 1, event.TypeSessionStarted, &event.SessionStarted{
		ProviderCapabilities: &caps,
	})
	s = applyInteraction(t, s, 2, event.TypeInputReceived, &event.InputReceived{
		Text: "look", Source: "slack", Principal: "user:42", Trust: "external",
		TurnID: "turn-cmd-1", ItemID: "item-cmd-1",
		Content: []provider.Part{{Kind: provider.PartText, Text: "look"},
			{Kind: provider.PartImage, Ref: "sha256-image", MediaType: "image/png"}},
	})
	s = applyInteraction(t, s, 3, event.TypeAssistantMessage, &event.AssistantMessage{
		GenStep: 1, TurnID: "turn-cmd-1", ItemID: "item-assistant-1",
		Message: provider.Message{Role: provider.RoleAssistant, Parts: []provider.Part{{
			Kind: provider.PartToolCall, CallID: "c1", ToolName: "read_file", Args: json.RawMessage(`{"path":"a"}`),
		}}},
	})
	s = applyInteraction(t, s, 4, event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: "tool-c1", Kind: event.KindTool, Name: "read_file", CallID: "c1", Attempt: 1,
	})
	s = applyInteraction(t, s, 5, event.TypeActivityCompleted, &event.ActivityCompleted{
		ActivityID: "tool-c1", Result: json.RawMessage(`{"content":"ok"}`),
	})

	if s.Session.ProviderCapabilities == nil || s.Session.ProviderCapabilities.Provider != "gemini" {
		t.Fatalf("provider envelope = %+v", s.Session.ProviderCapabilities)
	}
	if len(s.Interactions.Turns) != 1 || len(s.Interactions.Turns[0].ItemIDs) != 4 {
		t.Fatalf("turns = %+v", s.Interactions.Turns)
	}
	in := s.Interactions.Items["item-cmd-1"]
	if in.Principal != "user:42" || in.Source != "slack" || in.Trust != "external" ||
		len(in.Content) != 2 || in.Content[1].Kind != provider.PartImage {
		t.Fatalf("typed input item = %+v", in)
	}
	result := s.Interactions.Items["item-tool-c1-result"]
	if result.CallID != "c1" || string(result.Result) != `{"content":"ok"}` {
		t.Fatalf("tool result item = %+v", result)
	}
}

func TestLegacyMessagesSynthesizeStableTurnItemsWithoutMutatingPriorState(t *testing.T) {
	base := New()
	afterInput := applyInteraction(t, base, 7, event.TypeInputReceived, &event.InputReceived{
		Text: "legacy", Source: "cli",
	})
	afterAssistant := applyInteraction(t, afterInput, 8, event.TypeAssistantMessage, &event.AssistantMessage{
		GenStep: 1, Message: provider.Message{Role: provider.RoleAssistant,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "ok"}}},
	})
	if len(base.Interactions.Items) != 0 || len(afterInput.Interactions.Items) != 1 {
		t.Fatal("Apply mutated the prior Turn/Item projection")
	}
	if len(afterAssistant.Interactions.Turns) != 1 ||
		afterAssistant.Interactions.Turns[0].TurnID != "turn-legacy-7" {
		t.Fatalf("legacy turn projection = %+v", afterAssistant.Interactions.Turns)
	}
	input := afterAssistant.Interactions.Items["item-legacy-7"]
	if input.Principal != "legacy-user" || input.Trust != "legacy" {
		t.Fatalf("legacy provenance defaults = %+v", input)
	}
}
