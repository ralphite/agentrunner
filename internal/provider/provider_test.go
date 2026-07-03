package provider

import (
	"encoding/json"
	"errors"
	"iter"
	"reflect"
	"testing"
)

func streamOf(events []StreamEvent, err error) iter.Seq2[StreamEvent, error] {
	return func(yield func(StreamEvent, error) bool) {
		for _, ev := range events {
			if !yield(ev, nil) {
				return
			}
		}
		if err != nil {
			yield(StreamEvent{}, err)
		}
	}
}

func TestCollectTurnAssembles(t *testing.T) {
	extras := map[string]json.RawMessage{"thought_sig": json.RawMessage(`"abc"`)}
	events := []StreamEvent{
		{Kind: EventTextDelta, TextDelta: "I'll read "},
		{Kind: EventTextDelta, TextDelta: "the file."},
		{Kind: EventToolCall, ToolCall: &ToolCall{
			CallID: CallID(1, 0), Name: "read_file",
			Args: json.RawMessage(`{"path":"a.go"}`), Extras: extras,
		}},
		{Kind: EventToolCall, ToolCall: &ToolCall{
			CallID: CallID(1, 1), Name: "bash",
			Args: json.RawMessage(`{"command":"go test"}`),
		}},
		{Kind: EventUsage, Usage: &Usage{InputTokens: 10, OutputTokens: 20}},
		{Kind: EventFinish, Finish: FinishToolUse},
	}

	turn, err := CollectTurn(streamOf(events, nil))
	if err != nil {
		t.Fatal(err)
	}
	if turn.Finish != FinishToolUse {
		t.Errorf("finish = %q", turn.Finish)
	}
	if turn.Usage.InputTokens != 10 || turn.Usage.OutputTokens != 20 {
		t.Errorf("usage = %+v", turn.Usage)
	}
	if len(turn.ToolCalls) != 2 {
		t.Fatalf("tool calls = %d, want 2", len(turn.ToolCalls))
	}
	if turn.ToolCalls[0].CallID != "call_1_0" || turn.ToolCalls[1].CallID != "call_1_1" {
		t.Errorf("call ids = %q, %q", turn.ToolCalls[0].CallID, turn.ToolCalls[1].CallID)
	}

	if turn.Message.Role != RoleAssistant {
		t.Errorf("role = %q", turn.Message.Role)
	}
	if len(turn.Message.Parts) != 3 {
		t.Fatalf("parts = %d, want 3 (text + 2 tool calls)", len(turn.Message.Parts))
	}
	if turn.Message.Parts[0].Kind != PartText || turn.Message.Parts[0].Text != "I'll read the file." {
		t.Errorf("text part = %+v", turn.Message.Parts[0])
	}
	if !reflect.DeepEqual(turn.Message.Parts[1].Extras, extras) {
		t.Errorf("extras not preserved: %+v", turn.Message.Parts[1].Extras)
	}
}

func TestCollectTurnTextOnly(t *testing.T) {
	turn, err := CollectTurn(streamOf([]StreamEvent{
		{Kind: EventTextDelta, TextDelta: "done"},
		{Kind: EventFinish, Finish: FinishEndTurn},
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.ToolCalls) != 0 || len(turn.Message.Parts) != 1 {
		t.Errorf("turn = %+v", turn)
	}
}

func TestCollectTurnError(t *testing.T) {
	wantErr := errors.New("stream broke")
	_, err := CollectTurn(streamOf([]StreamEvent{
		{Kind: EventTextDelta, TextDelta: "partial"},
	}, wantErr))
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestPartJSONRoundTrip(t *testing.T) {
	p := Part{
		Kind: PartToolResult, CallID: "call_2_0", ToolName: "bash",
		Result: json.RawMessage(`{"out":"ok"}`), IsError: true,
		Extras: map[string]json.RawMessage{"sig": json.RawMessage(`"x"`)},
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var back Part
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, back) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", back, p)
	}
}
