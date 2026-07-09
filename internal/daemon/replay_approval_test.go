package daemon

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/store"
)

type captureSink struct{ events []protocol.Event }

func (c *captureSink) Emit(e protocol.Event) { c.events = append(c.events, e) }

// TestReplayApprovalCarriesToolAndReason pins UX-02: a pending approval
// projected by ReplayJournal must name the gated tool + args and the ask
// reason (attach used to render "approval required:   (" — all blank —
// because the approval_request event only carried the call id).
func TestReplayApprovalCarriesToolAndReason(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	s, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	appendEv := func(typ string, payload any) {
		t.Helper()
		env, err := event.New(typ, payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Append(env); err != nil {
			t.Fatal(err)
		}
	}
	appendEv(event.TypeSessionStarted, &event.SessionStarted{})
	appendEv(event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1})
	appendEv(event.TypeAssistantMessage, &event.AssistantMessage{
		GenStep: 1,
		Message: provider.Message{Role: provider.RoleAssistant, Parts: []provider.Part{{
			Kind:     provider.PartToolCall,
			CallID:   "call_1_0",
			ToolName: "bash",
			Args:     json.RawMessage(`{"command":"rm -rf build/"}`),
		}}},
	})
	appendEv(event.TypeApprovalRequested, &event.ApprovalRequested{
		ApprovalID: "apr-eff-call_1_0",
		CallID:     "call_1_0",
		GateResults: []event.GateResult{
			{Gate: "floor", Decision: event.VerdictAllow},
			{Gate: "permission", Decision: event.VerdictAsk, Reason: "execute requires approval"},
		},
	})
	_ = s.Close()

	sink := &captureSink{}
	if err := ReplayJournal(dir, sink); err != nil {
		t.Fatal(err)
	}
	var ask protocol.Event
	found := false
	for _, e := range sink.events {
		if e.Kind == protocol.KindApprovalRequest {
			ask, found = e, true
		}
	}
	if !found {
		t.Fatal("no approval_request event projected")
	}
	if ask.ApprovalID != "apr-eff-call_1_0" {
		t.Errorf("ApprovalID = %q, want apr-eff-call_1_0", ask.ApprovalID)
	}
	if ask.Tool != "bash" {
		t.Errorf("Tool = %q, want bash", ask.Tool)
	}
	if ask.Args != `{"command":"rm -rf build/"}` {
		t.Errorf("Args = %q, want the gated command", ask.Args)
	}
	if ask.Text != "execute requires approval" {
		t.Errorf("Text (reason) = %q, want execute requires approval", ask.Text)
	}
}
