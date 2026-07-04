package event

import (
	"encoding/json"
	"time"

	"github.com/ralphite/agentrunner/internal/provider"
)

// The S2 event type set. S3+ may only ADD types, never change these.
const (
	TypeRunStarted        = "run_started"
	TypeInputReceived     = "input_received"
	TypeTurnStarted       = "turn_started"
	TypeAssistantMessage  = "assistant_message"
	TypeActivityStarted   = "activity_started"
	TypeActivityCompleted = "activity_completed"
	TypeActivityFailed    = "activity_failed"
	TypeActivityCancelled = "activity_cancelled"
	TypeTimerSet          = "timer_set"
	TypeTimerFired        = "timer_fired"
	TypeTimerCancelled    = "timer_cancelled"
	TypeWaitingEntered    = "waiting_entered"
	TypeWaitingResolved   = "waiting_resolved"
	TypeActorCrashed      = "actor_crashed"
	TypeRunEnded          = "run_ended"

	// S3 additions (the S2 set above never changes).
	TypeEffectRequested   = "effect_requested"
	TypeEffectResolved    = "effect_resolved"
	TypeApprovalRequested = "approval_requested"
	TypeApprovalResponded = "approval_responded"
	TypeModeChanged       = "mode_changed"
	TypeLimitExceeded     = "limit_exceeded"

	// S4 additions.
	TypeTurnDiscarded    = "turn_discarded"
	TypeContextCompacted = "context_compacted"
)

// Effect verdicts and gate decisions.
const (
	VerdictAllow = "allow"
	VerdictAsk   = "ask"
	VerdictDeny  = "deny"
)

// Activity kinds.
const (
	KindLLM  = "llm"
	KindTool = "tool"
)

// Waiting kinds (the full 2.14 registry; tasks/timer cannot be produced
// before S6 but the vocabulary is fixed now).
const (
	WaitInput    = "input"
	WaitApproval = "approval"
	WaitTasks    = "tasks"
	WaitTimer    = "timer"
)

type RunStarted struct {
	SpecName         string         `json:"spec_name"`
	Model            string         `json:"model"`
	Task             string         `json:"task"`
	Version          string         `json:"version"`
	SubStateVersions map[string]int `json:"sub_state_versions"`
	// Spec and WorkspaceRoot let `resume <session>` reconstruct the run
	// without the original spec file (2.17).
	Spec          json.RawMessage `json:"spec,omitempty"`
	WorkspaceRoot string          `json:"workspace_root,omitempty"`
	// Env is the environment block (cwd, date) rendered and FROZEN at session
	// start (S4.4c / DESIGN §context-assembly): volatile data captured once
	// so it never rewrites the cacheable prompt prefix on later turns.
	Env string `json:"env,omitempty"`
}

type InputReceived struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

type TurnStarted struct {
	Turn int `json:"turn"`
}

type AssistantMessage struct {
	Turn    int              `json:"turn"`
	Message provider.Message `json:"message"`
}

type ActivityStarted struct {
	ActivityID string          `json:"activity_id"`
	Kind       string          `json:"kind"` // llm | tool
	Name       string          `json:"name"`
	Args       json.RawMessage `json:"args,omitempty"`
	CallID     string          `json:"call_id,omitempty"`
	Idempotent bool            `json:"idempotent,omitempty"`
	Attempt    int             `json:"attempt"`
}

type ActivityCompleted struct {
	ActivityID string          `json:"activity_id"`
	Result     json.RawMessage `json:"result,omitempty"`
	Usage      *provider.Usage `json:"usage,omitempty"`
	IsError    bool            `json:"is_error,omitempty"`
	// HookNote carries post-tool hook output (3.8): audit-only, additive.
	HookNote string `json:"hook_note,omitempty"`
}

// ErrorInfo is the journaled form of a classified error (2.8 taxonomy).
type ErrorInfo struct {
	Class     string `json:"class"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type ActivityFailed struct {
	ActivityID string    `json:"activity_id"`
	Error      ErrorInfo `json:"error"`
	Attempt    int       `json:"attempt"`
	// Final marks the attempt that exhausted the retry policy (3.9): the
	// activity is over, and for tool calls the fold renders the failure
	// as the call's model-visible result.
	Final bool `json:"final,omitempty"`
}

type ActivityCancelled struct {
	ActivityID    string `json:"activity_id"`
	PartialOutput string `json:"partial_output,omitempty"`
}

type TimerSet struct {
	TimerID string    `json:"timer_id"`
	FireAt  time.Time `json:"fire_at"`
	Purpose string    `json:"purpose"`
}

type TimerFired struct {
	TimerID string `json:"timer_id"`
}

// TimerCancelled clears a pending timer whose purpose completed before it
// fired (e.g. the activity finished inside its timeout).
type TimerCancelled struct {
	TimerID string `json:"timer_id"`
}

type WaitingEntered struct {
	Kind   string          `json:"kind"`
	Detail json.RawMessage `json:"detail,omitempty"`
}

type WaitingResolved struct {
	Kind       string `json:"kind"`
	Resolution string `json:"resolution"`
}

type ActorCrashed struct {
	Actor string `json:"actor"`
	Error string `json:"error"`
}

type RunEnded struct {
	Reason string         `json:"reason"`
	Turns  int            `json:"turns"`
	Usage  provider.Usage `json:"usage"`
}

// EffectRequested marks entry into the gate sequence (3.2): an effect
// with this fact but no EffectResolved crashed mid-adjudication. When the
// pipeline contains side-effecting gates (hooks), that window is in-doubt;
// pure-gate windows simply re-adjudicate on resume.
type EffectRequested struct {
	EffectID      string `json:"effect_id"`
	CallID        string `json:"call_id,omitempty"`
	SideEffecting bool   `json:"side_effecting,omitempty"`
}

// ApprovalRequested parks an ask-verdict effect for a human decision. It
// carries every gate's judgment (the prompt must show the full picture)
// and a reserved payload_ref for large payloads via the S7 ArtifactStore.
type ApprovalRequested struct {
	ApprovalID  string       `json:"approval_id"`
	EffectID    string       `json:"effect_id"`
	CallID      string       `json:"call_id,omitempty"`
	GateResults []GateResult `json:"gate_results,omitempty"`
	PayloadRef  string       `json:"payload_ref,omitempty"`
	// EstTokens preserves the budget reservation basis across a parked
	// wait (the approval may resolve after a crash+resume).
	EstTokens int `json:"est_tokens,omitempty"`
}

// ApprovalResponded is the journaled human decision (an external input:
// journal-inputs-first applies).
type ApprovalResponded struct {
	ApprovalID string `json:"approval_id"`
	Decision   string `json:"decision"` // approve | deny
	Reason     string `json:"reason,omitempty"`
	Source     string `json:"source"` // tty | env | interrupt
}

// ModeChanged records a run-mode transition (3.6c). Cause names the
// authority: "startup" | "exit_plan_mode approved" | "user".
type ModeChanged struct {
	From  string `json:"from,omitempty"`
	To    string `json:"to"`
	Cause string `json:"cause"`
}

// LimitExceeded records a resource-budget breach (3.7c): the run then
// ends gracefully through the epilogue, never mid-effect.
type LimitExceeded struct {
	Kind  string `json:"kind"` // tokens
	Limit int    `json:"limit"`
	Used  int    `json:"used"`
}

// TurnDiscarded marks an LLM turn whose partial stream was thrown away
// before a retry (S4.1): a durable companion to the ephemeral delta
// channel, telling a resuming surface to reopen the stream. The fold has
// no half-built assistant message to undo (assistant_message lands only on
// success), so this event is audit + surface-signal only.
type TurnDiscarded struct {
	Turn   int    `json:"turn"`
	Reason string `json:"reason,omitempty"`
}

// ContextCompacted records a compaction (S4.5): the output of a summarizer
// LLM call (a nondeterministic recorded activity) that REPLACES the
// conversation prefix folded so far with Summary. It changes subsequent
// fold results — fold to seq N and you get the pre- or post-compaction view
// depending on which side of this event N lands, which is what makes
// fork/rewind across the boundary well-defined. Summary is inlined in S4;
// SummaryRef (ArtifactStore) is reserved for later.
type ContextCompacted struct {
	UptoTurn     int    `json:"upto_turn"`
	Summary      string `json:"summary"`
	DroppedTurns int    `json:"dropped_turns,omitempty"`
	SummaryRef   string `json:"summary_ref,omitempty"`
}

// GateResult is one gate's judgment inside an effect resolution.
type GateResult struct {
	Gate     string `json:"gate"`
	Decision string `json:"decision"` // allow | ask | deny
	Reason   string `json:"reason,omitempty"`
}

// EffectResolved journals the pipeline's final verdict — allow or deny,
// with every gate's judgment — AFTER adjudication and BEFORE execution
// (the ask path resolves to one of these after the approval response).
type EffectResolved struct {
	EffectID    string       `json:"effect_id"`
	CallID      string       `json:"call_id,omitempty"`
	Verdict     string       `json:"verdict"` // allow | deny
	GateResults []GateResult `json:"gate_results,omitempty"`
	// ReservedTokens is the budget reservation granted with an allow
	// (3.7b); released when the activity reaches a terminal event.
	ReservedTokens int `json:"reserved_tokens,omitempty"`
}

// Registry maps every event type to a constructor for its payload struct.
// Decode helpers and the round-trip test are driven by this table.
var Registry = map[string]func() any{
	TypeRunStarted:        func() any { return &RunStarted{} },
	TypeInputReceived:     func() any { return &InputReceived{} },
	TypeTurnStarted:       func() any { return &TurnStarted{} },
	TypeAssistantMessage:  func() any { return &AssistantMessage{} },
	TypeActivityStarted:   func() any { return &ActivityStarted{} },
	TypeActivityCompleted: func() any { return &ActivityCompleted{} },
	TypeActivityFailed:    func() any { return &ActivityFailed{} },
	TypeActivityCancelled: func() any { return &ActivityCancelled{} },
	TypeTimerSet:          func() any { return &TimerSet{} },
	TypeTimerFired:        func() any { return &TimerFired{} },
	TypeTimerCancelled:    func() any { return &TimerCancelled{} },
	TypeWaitingEntered:    func() any { return &WaitingEntered{} },
	TypeWaitingResolved:   func() any { return &WaitingResolved{} },
	TypeActorCrashed:      func() any { return &ActorCrashed{} },
	TypeRunEnded:          func() any { return &RunEnded{} },
	TypeEffectRequested:   func() any { return &EffectRequested{} },
	TypeEffectResolved:    func() any { return &EffectResolved{} },
	TypeApprovalRequested: func() any { return &ApprovalRequested{} },
	TypeApprovalResponded: func() any { return &ApprovalResponded{} },
	TypeModeChanged:       func() any { return &ModeChanged{} },
	TypeLimitExceeded:     func() any { return &LimitExceeded{} },
	TypeTurnDiscarded:     func() any { return &TurnDiscarded{} },
	TypeContextCompacted:  func() any { return &ContextCompacted{} },
}
