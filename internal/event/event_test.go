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
		Prompt: "fix it", Version: "dev", SubStateVersions: map[string]int{"conversation": 1},
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
	TypeAskResolved:     &AskResolved{CallID: "call_2_0", Resolution: "answered", Answer: "yes, use postgres", DeliverySeq: 7},

	TypeGoalAttached:   &GoalAttached{GoalID: "g1", Goal: "make tests pass", Verifiers: []GoalVerifier{{Kind: "command", Command: "go test ./..."}}, Budget: GoalBudget{MaxChecks: 5}, Source: "user"},
	TypeGoalUpdated:    &GoalUpdated{GoalID: "g1", Goal: "make tests pass 3x", Budget: &GoalBudget{MaxChecks: 8}, Source: "user"},
	TypeGoalPaused:     &GoalPaused{GoalID: "g1", Source: "user"},
	TypeGoalResumed:    &GoalResumed{GoalID: "g1", Source: "user"},
	TypeGoalCancelled:  &GoalCancelled{GoalID: "g1", Reason: "user cancelled", Source: "user"},
	TypeGoalCheckpoint: &GoalCheckpoint{GoalID: "g1", Check: 2, Pass: false, Detail: "1 test still failing"},
	TypeGoalAchieved:   &GoalAchieved{GoalID: "g1", Reason: "satisfied", Checks: 3},
	TypeGoalExhausted:  &GoalExhausted{GoalID: "g1", Reason: "budget", Checks: 5},
	TypeGoalCompletionClaimed: &GoalCompletionClaimed{GoalID: "g1",
		Summary: "suite green 3x, artifact rendered", Source: "model"},

	TypeActorCrashed:    &ActorCrashed{Actor: "session", Error: "boom"},
	TypeEffectRequested: &EffectRequested{EffectID: "eff-call_3_1", CallID: "call_3_1", SideEffecting: true},
	TypeApprovalRequested: &ApprovalRequested{ApprovalID: "apr-eff-call_3_1", EffectID: "eff-call_3_1",
		CallID: "call_3_1", GateResults: []GateResult{{Gate: "permission", Decision: VerdictAsk, Reason: "edit"}},
		PayloadRef: "sha256-planref", EstTokens: 1000},
	TypeApprovalResponded: &ApprovalResponded{ApprovalID: "apr-eff-call_3_1", Decision: "approve",
		Reason: "looks safe", Source: "tty"},
	TypeModeChanged:           &ModeChanged{From: "plan", To: "default", Cause: "exit_plan_mode approved"},
	TypeLimitExceeded:         &LimitExceeded{Kind: "tokens", Limit: 10000, Used: 10250},
	TypeGenerationDiscarded:   &GenerationDiscarded{GenStep: 3, Reason: "llm retry after partial stream"},
	TypeContextCompacted:      &ContextCompacted{UptoGenStep: 4, Summary: "user asked X; did Y", DroppedTurns: 3},
	TypeContextMicrocompacted: &ContextMicrocompacted{Boundary: 12, EstimatedTokens: 9000, Cleared: 5},
	TypeMalformedToolCall:     &MalformedToolCall{GenStep: 3, Raw: "{bad json", Error: "unexpected end of input"},
	TypeToolsDiscovered: &ToolsDiscovered{Server: "demo", Tools: []MCPToolDef{{
		Server: "demo", Name: "mcp__demo__peek", Description: "read-only peek",
		Class: "read", InputSchema: json.RawMessage(`{"type":"object"}`)}}},
	TypeSpawnRequested: &SpawnRequested{CallID: "call_2_0", Agent: "summarizer",
		Prompt: "summarize the findings", ChildSession: "sess-sub-call_2_0", Depth: 1, BudgetTokens: 4000},
	TypeSubagentCompleted: &SubagentCompleted{CallID: "call_2_0", Agent: "summarizer",
		ChildSession: "sess-sub-call_2_0", Reason: "completed", GenSteps: 2,
		Usage: provider.Usage{InputTokens: 100, OutputTokens: 50}},
	TypeChildRevived: &ChildRevived{CallID: "call_2_0", ActivityID: "revive-cmd_1",
		Agent: "summarizer", ChildSession: "sess-sub-call_2_0-a1", Reason: "message",
		BudgetTokens: 2000, BaselineUsage: provider.Usage{InputTokens: 100, OutputTokens: 50}},
	TypeArtifactPublished: &ArtifactPublished{Stream: "report", Version: 2,
		Ref: "sha256-deadbeef", Bytes: 512, Source: "tool"},
	TypeProgressUpdated: &ProgressUpdated{Items: []ProgressItem{
		{ID: "tests", Title: "run the suite", Status: "running"}}},
	TypeInputRevoked: &InputRevoked{TargetCommandID: "cmd-9", DeliverySeq: 4},
	TypeEffectResolved: &EffectResolved{EffectID: "eff-call_3_1", CallID: "call_3_1",
		Verdict: VerdictDeny, GateResults: []GateResult{
			{Gate: "permission", Decision: VerdictDeny, Reason: "path escapes workspace"}},
		Containment: &Containment{Filesystem: "workspace", Network: "none", Backend: "sandbox-exec"}},
	TypeSessionClosed: &SessionClosed{Reason: "killed", Source: "user", GenSteps: 4},
	TypeDriverStarted: &DriverStarted{DriverID: "drv-1", SpecName: "nightly",
		Spec: json.RawMessage(`{"name":"nightly"}`), WorkspaceRoot: "/w", FoldVersion: 1},
	TypeIterationScheduled:      &IterationScheduled{DriverID: "drv-1", Iter: 2, Schedule: "immediate", BaseRef: "0badc0de"},
	TypeIterationLaunched:       &IterationLaunched{DriverID: "drv-1", Iter: 2, ChildSession: "drv-1-iter-2"},
	TypeIterationAttemptStarted: &IterationAttemptStarted{DriverID: "drv-1", Iter: 2, Attempt: 2, ChildSession: "drv-1-iter-2-a2"},
	TypeIterationAttemptCompleted: &IterationAttemptCompleted{DriverID: "drv-1", Iter: 2, Attempt: 2,
		ChildSession: "drv-1-iter-2-a2", Reason: "error", Error: "provider failed"},
	TypeIterationCompleted: &IterationCompleted{DriverID: "drv-1", Iter: 2, ChildSession: "drv-1-iter-2",
		ChildReason: "completed", Verdict: IterationVerdict{Pass: true, Score: 1, Verifier: "command", Detail: "exit=0"},
		Usage: provider.Usage{InputTokens: 30, OutputTokens: 12}, CarryRef: "sha256-carry", Carry: "wrote 3 lines"},
	TypeIterationSkipped: &IterationSkipped{DriverID: "drv-1", Iter: 3, Reason: "overlap"},
	TypeDriverCompleted:  &DriverCompleted{DriverID: "drv-1", Reason: "satisfied", Iterations: 2, BestIter: 2},
	TypeNotificationSent: &NotificationSent{Key: "run_end/sess-1", Kind: "run_end",
		Session: "sess-1", Text: "run completed", Channel: "command"},
	TypeCheckpointBarrier: &CheckpointBarrier{BarrierID: "bar-t3", GenStep: 3,
		Vector: map[string]int64{".": 41, "sub/s1-a1": 12}, SnapshotRef: "0badc0de",
		Handles: []BarrierHandle{{Handle: "bg1", Policy: "cancel_at_fork"}}},
	TypeForkedFrom: &ForkedFrom{ParentSession: "20260703-120000-fix-abcd",
		BarrierID: "bar-t3", SnapshotRef: "0badc0de", WorkspaceRoot: "/w-fork"},
	TypeSpecChanged: &SpecChanged{SpecName: "reviewer", Model: "gemini-x",
		Spec: json.RawMessage(`{"name":"reviewer"}`), SpecPath: "/specs/reviewer.yaml",
		Source: "user", Env: "<env>cwd: /w</env>", Agents: "<agents>helper</agents>"},
	TypeCommandHandled: &CommandHandled{CommandID: "cmd-1", CommandSeq: 7,
		Kind: "compact", Result: "no_op"},
	TypeSessionTitled: &SessionTitled{Title: "Fix the auth boundary", Source: TitleSourceAuto},
	TypeScheduleAttached: &ScheduleAttached{ScheduleID: "sch-1", Interval: "30m",
		Prompt: "巡检一次构建状态", MaxWakes: 8,
		Base: time.Date(2026, 7, 18, 6, 0, 0, 0, time.UTC), Source: "user"},
	TypeSchedulePaused: &SchedulePaused{ScheduleID: "sch-1", Source: "user"},
	TypeScheduleResumed: &ScheduleResumed{ScheduleID: "sch-1",
		Base: time.Date(2026, 7, 18, 7, 0, 0, 0, time.UTC), Source: "user"},
	TypeScheduleCancelled: &ScheduleCancelled{ScheduleID: "sch-1", Reason: "user", Source: "user"},
	TypeScheduleWake: &ScheduleWake{ScheduleID: "sch-1", N: 3,
		Tick: time.Date(2026, 7, 18, 6, 30, 0, 0, time.UTC), Skipped: true},
	TypeSeriesStarted: &SeriesStarted{SeriesID: "ser-1", Kind: "goal",
		MaxIterations: 8, Patience: 3, Overlap: "skip", Source: "user"},
	TypeSeriesIteration: &SeriesIteration{SeriesID: "ser-1", N: 2, CallID: "call-7",
		ChildSession: "s-sub-call-7-a1", Reason: "completed",
		Verdict:  IterationVerdict{Pass: true, Score: 0.9, Verifier: "command"},
		CarryRef: "carry@v2", Carry: "两条断言修复",
		Tick:  time.Date(2026, 7, 18, 14, 30, 0, 0, time.UTC),
		Usage: provider.Usage{InputTokens: 100, OutputTokens: 50}},
	TypeSeriesEnded: &SeriesEnded{SeriesID: "ser-1", Reason: "goal_satisfied",
		Iterations: 2, BestIter: 2},
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
