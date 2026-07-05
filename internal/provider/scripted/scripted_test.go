package scripted

import (
	"context"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
)

func userMsg(text string) provider.Message {
	return provider.Message{Role: provider.RoleUser,
		Parts: []provider.Part{{Kind: provider.PartText, Text: text}}}
}

func twoStepFixture() Fixture {
	return Fixture{Steps: []Step{
		{
			Expect: Expect{ToolsInclude: []string{"read_file"}, LastMessageContains: "fix"},
			Respond: []Event{
				{Text: "reading"},
				{ToolCall: &ToolCallEvent{Name: "read_file", Args: map[string]any{"path": "a.go"}}},
				{Finish: "tool_use"},
			},
		},
		{
			Respond: []Event{{Text: "done"}, {Finish: "end_turn"}},
		},
	}}
}

func TestScriptedReplay(t *testing.T) {
	p := New(twoStepFixture())
	req := provider.CompleteRequest{
		GenStep:  1,
		Tools:    []provider.ToolDef{{Name: "read_file"}},
		Messages: []provider.Message{userMsg("please fix the bug")},
	}

	turn, err := provider.CollectTurn(p.Complete(context.Background(), req))
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].CallID != "call_1_0" {
		t.Fatalf("turn = %+v", turn)
	}

	turn2, err := provider.CollectTurn(p.Complete(context.Background(), provider.CompleteRequest{GenStep: 2}))
	if err != nil {
		t.Fatal(err)
	}
	if turn2.Finish != provider.FinishEndTurn {
		t.Errorf("finish = %q", turn2.Finish)
	}
	if err := p.Done(); err != nil {
		t.Errorf("Done() = %v", err)
	}
}

func TestScriptedDriftFailsLoudly(t *testing.T) {
	p := New(twoStepFixture())
	_, err := provider.CollectTurn(p.Complete(context.Background(), provider.CompleteRequest{
		GenStep:  1,
		Tools:    []provider.ToolDef{{Name: "bash"}}, // read_file missing
		Messages: []provider.Message{userMsg("please fix the bug")},
	}))
	if err == nil || !strings.Contains(err.Error(), "request drift") {
		t.Fatalf("err = %v, want request drift", err)
	}
}

func TestScriptedExhaustion(t *testing.T) {
	p := New(Fixture{Steps: []Step{{Respond: []Event{{Finish: "end_turn"}}}}})
	if _, err := provider.CollectTurn(p.Complete(context.Background(), provider.CompleteRequest{GenStep: 1})); err != nil {
		t.Fatal(err)
	}
	_, err := provider.CollectTurn(p.Complete(context.Background(), provider.CompleteRequest{GenStep: 2}))
	if err == nil || !strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("err = %v, want exhaustion", err)
	}
}

func TestScriptedDoneDetectsUnconsumed(t *testing.T) {
	p := New(twoStepFixture())
	if err := p.Done(); err == nil {
		t.Fatal("Done() should fail with unconsumed steps")
	}
}

// Pin: iterating the same returned stream twice consumes TWO fixture steps
// (consumption is per-iteration, not per-Complete-call). S2's activity
// executor must not blindly re-iterate a stream on retry. The provider is
// also single-goroutine by contract — no mutex on p.next.
func TestScriptedStreamConsumptionPerIteration(t *testing.T) {
	p := New(twoStepFixture())
	req := provider.CompleteRequest{
		GenStep:  1,
		Tools:    []provider.ToolDef{{Name: "read_file"}},
		Messages: []provider.Message{userMsg("please fix the bug")},
	}
	stream := p.Complete(context.Background(), req)

	if _, err := provider.CollectTurn(stream); err != nil {
		t.Fatal(err)
	}
	// Second iteration of the SAME seq serves step 2 (its expect has no
	// constraints, so it succeeds and consumes the fixture).
	if _, err := provider.CollectTurn(stream); err != nil {
		t.Fatalf("second iteration: %v", err)
	}
	if err := p.Done(); err != nil {
		t.Fatalf("both steps should be consumed: %v", err)
	}
}
