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
	TypeSessionStarted: &SessionStarted{SpecName: "hello", Model: "gemini-flash-latest",
		Task: "fix it", Version: "dev", SubStateVersions: map[string]int{"conversation": 1},
		Spec: json.RawMessage(`{"name":"hello"}`), WorkspaceRoot: "/w",
		Env: "<env>\ncwd: /w\n</env>", Memory: "<memory>rules</memory>",
		Skills: "<skills>- s</skills>", Agents: "<agents>- a</agents>",
		Inputs:           []ArtifactInput{{Ref: "sha256-aa", Path: "in.md"}},
		PermissionLayers: json.RawMessage(`[[{"tool":"edit_file","action":"deny"}]]`)},
	TypeInputReceived:     &InputReceived{Text: "please fix", Source: "cli"},
	TypeGenerationStarted: &GenerationStarted{GenStep: 3},
	TypeAssistantMessage: &AssistantMessage{GenStep: 3, Message: provider.Message{
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
	TypeActivityCancelled: &ActivityCancelled{ActivityID: "tool-call_3_1", PartialOutput: "partial",
		Usage: &provider.Usage{InputTokens: 40, OutputTokens: 10}},
	TypeTimerSet: &TimerSet{TimerID: "tm-1",
		FireAt: time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC), Purpose: "activity_timeout"},
	TypeTimerFired:      &TimerFired{TimerID: "tm-1"},
	TypeTimerCancelled:  &TimerCancelled{TimerID: "tm-1"},
	TypeWaitingEntered:  &WaitingEntered{Kind: WaitApproval, Detail: json.RawMessage(`{"call_id":"call_3_1"}`)},
	TypeWaitingResolved: &WaitingResolved{Kind: WaitApproval, Resolution: "approved"},
	TypeActorCrashed:    &ActorCrashed{Actor: "session", Error: "boom"},
	TypeEffectRequested: &EffectRequested{EffectID: "eff-call_3_1", CallID: "call_3_1", SideEffecting: true},
	TypeApprovalRequested: &ApprovalRequested{ApprovalID: "apr-eff-call_3_1", EffectID: "eff-call_3_1",
		CallID: "call_3_1", GateResults: []GateResult{{Gate: "permission", Decision: VerdictAsk, Reason: "edit"}},
		PayloadRef: "sha256-planref", EstTokens: 1000},
	TypeApprovalResponded: &ApprovalResponded{ApprovalID: "apr-eff-call_3_1", Decision: "approve",
		Reason: "looks safe", Source: "tty"},
	TypeModeChanged:         &ModeChanged{From: "plan", To: "default", Cause: "exit_plan_mode approved"},
	TypeLimitExceeded:       &LimitExceeded{Kind: "tokens", Limit: 10000, Used: 10250},
	TypeGenerationDiscarded: &GenerationDiscarded{GenStep: 3, Reason: "llm retry after partial stream"},
	TypeContextCompacted:    &ContextCompacted{UptoGenStep: 4, Summary: "user asked X; did Y", DroppedTurns: 3},
	TypeMalformedToolCall:   &MalformedToolCall{GenStep: 3, Raw: "{bad json", Error: "unexpected end of input"},
	TypeToolsDiscovered: &ToolsDiscovered{Server: "demo", Tools: []MCPToolDef{{
		Server: "demo", Name: "mcp__demo__peek", Description: "read-only peek",
		Class: "read", InputSchema: json.RawMessage(`{"type":"object"}`)}}},
	TypeSpawnRequested: &SpawnRequested{CallID: "call_2_0", Agent: "summarizer",
		Task: "summarize the findings", ChildSession: "sess-sub-call_2_0", Depth: 1, BudgetTokens: 4000},
	TypeSubagentCompleted: &SubagentCompleted{CallID: "call_2_0", Agent: "summarizer",
		ChildSession: "sess-sub-call_2_0", Reason: "completed", GenSteps: 2,
		Usage: provider.Usage{InputTokens: 100, OutputTokens: 50}},
	TypeArtifactPublished: &ArtifactPublished{Stream: "report", Version: 2,
		Ref: "sha256-deadbeef", Bytes: 512, Source: "tool"},
	TypeEffectResolved: &EffectResolved{EffectID: "eff-call_3_1", CallID: "call_3_1",
		Verdict: VerdictDeny, GateResults: []GateResult{
			{Gate: "permission", Decision: VerdictDeny, Reason: "path escapes workspace"}},
		Containment: &Containment{Network: "none", Backend: "netns"}},
	TypeTaskCompleted: &TaskCompleted{Reason: "completed", GenSteps: 4, Usage: provider.Usage{InputTokens: 10}},
	TypeSessionClosed: &SessionClosed{Reason: "closed", GenSteps: 4},
	TypeDriverStarted: &DriverStarted{DriverID: "drv-1", SpecName: "nightly",
		Spec: json.RawMessage(`{"name":"nightly"}`), WorkspaceRoot: "/w", FoldVersion: 1},
	TypeIterationScheduled: &IterationScheduled{DriverID: "drv-1", Iter: 2, Schedule: "immediate", BaseRef: "0badc0de"},
	TypeIterationLaunched:  &IterationLaunched{DriverID: "drv-1", Iter: 2, ChildSession: "drv-1-iter-2"},
	TypeIterationCompleted: &IterationCompleted{DriverID: "drv-1", Iter: 2, ChildSession: "drv-1-iter-2",
		ChildReason: "completed", Verdict: IterationVerdict{Pass: true, Score: 1, Verifier: "command", Detail: "exit=0"},
		Usage: provider.Usage{InputTokens: 30, OutputTokens: 12}, CarryRef: "sha256-carry", Carry: "wrote 3 lines"},
	TypeIterationSkipped: &IterationSkipped{DriverID: "drv-1", Iter: 3, Reason: "overlap"},
	TypeDriverCompleted:  &DriverCompleted{DriverID: "drv-1", Reason: "satisfied", Iterations: 2, BestIter: 2},
	TypeNotificationSent: &NotificationSent{Key: "run_end/sess-1", Kind: "run_end",
		Session: "sess-1", Text: "run completed", Channel: "command"},
	TypeCheckpointBarrier: &CheckpointBarrier{BarrierID: "bar-t3", GenStep: 3,
		Vector: map[string]int64{".": 41, "sub/s1-a1": 12}, SnapshotRef: "0badc0de",
		Tasks: []BarrierTask{{TaskID: "bg1", Policy: "cancel_at_fork"}}},
	TypeForkedFrom: &ForkedFrom{ParentSession: "20260703-120000-fix-abcd",
		BarrierID: "bar-t3", SnapshotRef: "0badc0de", WorkspaceRoot: "/w-fork"},
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
	child, err := New(TypeGenerationStarted, &GenerationStarted{GenStep: 1})
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
		Type: TypeGenerationStarted, Payload: json.RawMessage(`{"turn":1}`),
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
