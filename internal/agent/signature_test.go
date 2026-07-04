package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// S4.4d: an opaque provider payload on a tool_call part (Gemini's
// thoughtSignature) must round-trip byte-identically — persisted into the
// event log on turn 1 and handed back VERBATIM in the turn-2 request the
// harness assembles. The harness never inspects or regenerates it.
func TestSignatureRoundTrip(t *testing.T) {
	sig := json.RawMessage(`"CiQAsig_opaque_blob_preserved_verbatim=="`)
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: "thinking, then acting"},
			{ToolCall: &scripted.ToolCallEvent{
				CallID: "call_1_0", Name: "bash",
				Args:   map[string]any{"command": "echo hi"},
				Extras: map[string]json.RawMessage{"thought_signature": sig},
			}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
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
			Name:     "sig",
			Model:    ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			Tools:    []string{"bash"},
			MaxTurns: 5,
		},
		Provider:  cap,
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)),
		SessionID: "sig-sess",
	}
	if _, err := l.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}

	// The turn-2 request the harness assembled must echo the signature back.
	if len(cap.requests) < 2 {
		t.Fatalf("expected 2 requests, got %d", len(cap.requests))
	}
	got := findToolCallExtra(cap.requests[1].Messages, "call_1_0", "thought_signature")
	if got == nil {
		t.Fatal("signature missing from assembled turn-2 request")
	}
	if !bytes.Equal(got, sig) {
		t.Errorf("signature not byte-identical:\n got: %s\nwant: %s", got, sig)
	}

	// And it is durable — the assistant_message event carries it verbatim.
	events, err := store.ReadEvents(es.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var persisted json.RawMessage
	for _, e := range events {
		if e.Type != event.TypeAssistantMessage {
			continue
		}
		dec, err := event.DecodePayload(e)
		if err != nil {
			t.Fatal(err)
		}
		am := dec.(*event.AssistantMessage)
		if x := findToolCallExtra([]provider.Message{am.Message}, "call_1_0", "thought_signature"); x != nil {
			persisted = x
		}
	}
	if persisted == nil || !bytes.Equal(persisted, sig) {
		t.Errorf("signature not persisted byte-identically: %s", persisted)
	}
}

func findToolCallExtra(msgs []provider.Message, callID, key string) json.RawMessage {
	for _, m := range msgs {
		for _, p := range m.Parts {
			if p.Kind == provider.PartToolCall && p.CallID == callID {
				return p.Extras[key]
			}
		}
	}
	return nil
}
