package event

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/provider"
)

// One populated sample per registered type. The round-trip test refuses to
// pass if a registered type has no sample here — adding an event type
// forces adding its sample.
var samples = map[string]any{
	TypeRunStarted: &RunStarted{SpecName: "hello", Model: "gemini-flash-latest",
		Task: "fix it", Version: "dev", SubStateVersions: map[string]int{"conversation": 1}},
	TypeInputReceived: &InputReceived{Text: "please fix", Source: "cli"},
	TypeTurnStarted:   &TurnStarted{Turn: 3},
	TypeAssistantMessage: &AssistantMessage{Turn: 3, Message: provider.Message{
		Role: provider.RoleAssistant,
		Parts: []provider.Part{{Kind: provider.PartToolCall, CallID: "call_3_0",
			ToolName: "read_file", Args: json.RawMessage(`{"path":"a.go"}`)}},
	}},
	TypeActivityStarted: &ActivityStarted{ActivityID: "tool-call_3_0", Kind: KindTool,
		Name: "read_file", Args: json.RawMessage(`{"path":"a.go"}`),
		CallID: "call_3_0", Idempotent: true, Attempt: 1},
	TypeActivityCompleted: &ActivityCompleted{ActivityID: "llm-t3",
		Result: json.RawMessage(`{"ok":true}`),
		Usage:  &provider.Usage{InputTokens: 5, OutputTokens: 7}},
	TypeActivityFailed: &ActivityFailed{ActivityID: "llm-t3",
		Error: ErrorInfo{Class: "provider_server", Message: "503", Retryable: true}, Attempt: 2},
	TypeActivityCancelled: &ActivityCancelled{ActivityID: "tool-call_3_1", PartialOutput: "partial"},
	TypeTimerSet: &TimerSet{TimerID: "tm-1",
		FireAt: time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC), Purpose: "activity_timeout"},
	TypeTimerFired:      &TimerFired{TimerID: "tm-1"},
	TypeTimerCancelled:  &TimerCancelled{TimerID: "tm-1"},
	TypeWaitingEntered:  &WaitingEntered{Kind: WaitApproval, Detail: json.RawMessage(`{"call_id":"call_3_1"}`)},
	TypeWaitingResolved: &WaitingResolved{Kind: WaitApproval, Resolution: "approved"},
	TypeActorCrashed:    &ActorCrashed{Actor: "session", Error: "boom"},
	TypeEffectResolved: &EffectResolved{EffectID: "eff-call_3_1", CallID: "call_3_1",
		Verdict: VerdictDeny, GateResults: []GateResult{
			{Gate: "permission", Decision: VerdictDeny, Reason: "path escapes workspace"}}},
	TypeRunEnded: &RunEnded{Reason: "completed", Turns: 4, Usage: provider.Usage{InputTokens: 10}},
}

func TestRoundTripAllTypes(t *testing.T) {
	if len(samples) != len(Registry) {
		t.Fatalf("samples = %d, registry = %d — every registered type needs a sample", len(samples), len(Registry))
	}
	for typ, sample := range samples {
		t.Run(typ, func(t *testing.T) {
			env, err := New(typ, sample)
			if err != nil {
				t.Fatal(err)
			}
			line, err := json.Marshal(env)
			if err != nil {
				t.Fatal(err)
			}
			var back Envelope
			if err := json.Unmarshal(line, &back); err != nil {
				t.Fatal(err)
			}
			decoded, err := DecodePayload(back)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded, sample) {
				t.Errorf("round trip mismatch:\n got %#v\nwant %#v", decoded, sample)
			}
		})
	}
}

func TestNewRejectsUnregisteredType(t *testing.T) {
	if _, err := New("hologram", struct{}{}); err == nil {
		t.Fatal("unregistered type must be rejected")
	}
}

func TestDecodeUnknownTypeIsError(t *testing.T) {
	_, err := DecodePayload(Envelope{Seq: 7, Type: "from_the_future", Payload: json.RawMessage(`{}`)})
	if err == nil || !strings.Contains(err.Error(), "from_the_future") {
		t.Fatalf("err = %v, want unknown-type error naming the type", err)
	}
}

func TestChildOfPropagation(t *testing.T) {
	parent := Envelope{ID: "evt-9", CorrelationID: "20260703-120000-fix-abcd"}
	child, err := New(TypeTurnStarted, &TurnStarted{Turn: 1})
	if err != nil {
		t.Fatal(err)
	}
	child = child.ChildOf(parent)
	if child.CausationID != "evt-9" {
		t.Errorf("causation = %q, want parent id", child.CausationID)
	}
	if child.CorrelationID != parent.CorrelationID {
		t.Errorf("correlation = %q, want inherited", child.CorrelationID)
	}
}

func TestCommandIDFormat(t *testing.T) {
	a, b := NewCommandID(), NewCommandID()
	if !strings.HasPrefix(a, "cmd-") || len(a) != len("cmd-")+8 {
		t.Errorf("id = %q, want cmd-<8hex>", a)
	}
	if a == b {
		t.Errorf("two ids collided: %q", a)
	}
}

func TestEventID(t *testing.T) {
	if got := EventID(42); got != "evt-42" {
		t.Errorf("EventID = %q", got)
	}
}

// Envelope JSON must keep the wire field names fixed — S2's on-disk format.
func TestEnvelopeWireFields(t *testing.T) {
	env := Envelope{Seq: 1, ID: "evt-1", CausationID: "cmd-aabbccdd",
		CorrelationID: "sess", Sender: "kernel", Target: "session",
		Type: TypeTurnStarted, Payload: json.RawMessage(`{"turn":1}`),
		TS: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"seq":1`, `"id":"evt-1"`, `"causation_id"`,
		`"correlation_id"`, `"sender"`, `"target"`, `"type"`, `"payload"`, `"ts"`} {
		if !strings.Contains(string(raw), field) {
			t.Errorf("wire form missing %s: %s", field, raw)
		}
	}
}
