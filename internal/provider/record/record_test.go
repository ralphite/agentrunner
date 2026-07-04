package record

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
)

func TestRecordAndReplayRoundTrip(t *testing.T) {
	t.Setenv("FAKE_API_KEY", "sekret-value")

	source := scripted.New(scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: "key is sekret-value ok"},
			{ToolCall: &scripted.ToolCallEvent{Name: "bash", Args: map[string]any{"command": "ls"}}},
			{Finish: "tool_use"},
		}},
	}})

	rec := New(source)
	req := provider.CompleteRequest{
		Turn:  1,
		Tools: []provider.ToolDef{{Name: "bash"}},
		Messages: []provider.Message{{Role: provider.RoleUser,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "run ls for me please"}}}},
	}
	turn, err := provider.CollectTurn(rec.Complete(context.Background(), req))
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("turn = %+v", turn)
	}

	path := filepath.Join(t.TempDir(), "session.yaml")
	if err := rec.WriteFixture(path); err != nil {
		t.Fatal(err)
	}

	// Replay the recorded fixture with the same request: must succeed and
	// must not contain the secret.
	replay, err := scripted.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	turn2, err := provider.CollectTurn(replay.Complete(context.Background(), req))
	if err != nil {
		t.Fatal(err)
	}
	text := turn2.Message.Parts[0].Text
	if strings.Contains(text, "sekret-value") {
		t.Errorf("secret leaked into fixture: %q", text)
	}
	if !strings.Contains(text, "[REDACTED:FAKE_API_KEY]") {
		t.Errorf("redaction marker missing: %q", text)
	}
	if err := replay.Done(); err != nil {
		t.Error(err)
	}
}

func TestRecordedExpectCatchesDrift(t *testing.T) {
	source := scripted.New(scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}})
	rec := New(source)
	req := provider.CompleteRequest{
		Turn: 1,
		Messages: []provider.Message{{Role: provider.RoleUser,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "original prompt"}}}},
	}
	if _, err := provider.CollectTurn(rec.Complete(context.Background(), req)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "session.yaml")
	if err := rec.WriteFixture(path); err != nil {
		t.Fatal(err)
	}

	replay, err := scripted.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	drifted := req
	drifted.Messages = []provider.Message{{Role: provider.RoleUser,
		Parts: []provider.Part{{Kind: provider.PartText, Text: "completely different"}}}}
	_, err = provider.CollectTurn(replay.Complete(context.Background(), drifted))
	if err == nil || !strings.Contains(err.Error(), "request drift") {
		t.Fatalf("err = %v, want request drift", err)
	}
}

// A credential split across two text deltas must still be redacted — the
// recorder accumulates deltas and redacts the whole (S2 review item). The
// scripted provider emits one delta per text event, so two text events
// split the secret across delta boundaries.
func TestRecorderRedactsCrossDeltaSecret(t *testing.T) {
	t.Setenv("SPLIT_API_KEY", "sk-abcdef123456")
	source := scripted.New(scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: "the key is sk-abc"},
			{Text: "def123456 ok"},
			{Finish: "end_turn"},
		}},
	}})
	rec := New(source)
	if _, err := provider.CollectTurn(rec.Complete(context.Background(), provider.CompleteRequest{})); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fix.yaml")
	if err := rec.WriteFixture(path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "sk-abcdef123456") {
		t.Fatalf("cross-delta secret leaked into fixture:\n%s", raw)
	}
	if !strings.Contains(string(raw), "[REDACTED:SPLIT_API_KEY]") {
		t.Fatalf("expected redaction marker:\n%s", raw)
	}
}
