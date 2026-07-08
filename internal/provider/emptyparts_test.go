package provider

import "testing"

// TestCollectTurnNeverEmptyParts pins the C1 write-side guard: a generation
// step that emits no visible text and no tool calls (pure thought / dropped
// parts) must still assemble a non-empty assistant message, so it can never
// poison history with parts:null (audit 2026-07-07 C1).
func TestCollectTurnNeverEmptyParts(t *testing.T) {
	turn, err := CollectTurn(streamOf([]StreamEvent{
		{Kind: EventFinish, Finish: FinishEndTurn},
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if turn.Message.Role != RoleAssistant {
		t.Errorf("role = %q, want assistant", turn.Message.Role)
	}
	if len(turn.Message.Parts) != 1 {
		t.Fatalf("parts = %d, want 1 synthesized placeholder", len(turn.Message.Parts))
	}
	if p := turn.Message.Parts[0]; p.Kind != PartText || p.Text != EmptyGenerationPlaceholder {
		t.Errorf("part = %+v, want placeholder text %q", p, EmptyGenerationPlaceholder)
	}
}

// TestCollectTurnKeepsRealText pins the other half: real visible text is kept
// verbatim and never overwritten by the placeholder.
func TestCollectTurnKeepsRealText(t *testing.T) {
	turn, err := CollectTurn(streamOf([]StreamEvent{
		{Kind: EventTextDelta, TextDelta: "hello "},
		{Kind: EventTextDelta, TextDelta: "world"},
		{Kind: EventFinish, Finish: FinishEndTurn},
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.Message.Parts) != 1 {
		t.Fatalf("parts = %d, want 1", len(turn.Message.Parts))
	}
	if got := turn.Message.Parts[0].Text; got != "hello world" {
		t.Errorf("text = %q, want %q", got, "hello world")
	}
}
