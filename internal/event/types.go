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
	TypeTurnDiscarded     = "turn_discarded"
	TypeContextCompacted  = "context_compacted"
	TypeMalformedToolCall = "malformed_tool_call"

	// S5 additions.
	TypeToolsDiscovered   = "tools_discovered"
	TypeSpawnRequested    = "spawn_requested"
	TypeSubagentCompleted = "subagent_completed"
	TypeArtifactPublished = "artifact_published"

	// S6 additions (IterationDriver, DESIGN §运行形态). These belong to the
	// driver's OWN stream — folded by internal/driver, never by the run fold,
	// so they carry no run SubStateVersions entry.
	TypeDriverStarted      = "driver_started"
	TypeIterationScheduled = "iteration_scheduled"
	TypeIterationLaunched  = "iteration_launched"
	TypeIterationCompleted = "iteration_completed"
	TypeIterationSkipped   = "iteration_skipped"
	TypeDriverCompleted    = "driver_completed"

	// S6 模块⑤: the notifier's OWN stream (never in a run journal).
	TypeNotificationSent = "notification_sent"

	// S7 additions (world-state lifecycle).
	TypeCheckpointBarrier = "checkpoint_barrier"
	TypeForkedFrom        = "forked_from"
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
	// Memory and Skills are the rendered CLAUDE.md merge and the skills
	// directory block (S5.2), frozen at session start exactly like Env —
	// editing the files mid-run must not rewrite the prefix.
	Memory string `json:"memory,omitempty"`
	Skills string `json:"skills,omitempty"`
	// Agents is the rendered sub-agent directory block (S5.3), frozen like
	// Skills — the model spawns only what it can see.
	Agents string `json:"agents,omitempty"`
	// Inputs are artifact refs to materialize into the workspace before the
	// first turn (S5.8) — how a parent hands documents to a child.
	Inputs []ArtifactInput `json:"inputs,omitempty"`
	// PermissionLayers materializes the run's EFFECTIVE permission rules as
	// data (S6, S5 回访): a JSON [][]pipeline.PermissionRule, ordered
	// outermost (root) → innermost (this run). Chained first-match layers
	// cannot be flattened into one list, so the layers stay separate; a
	// cross-process resume rebuilds one gate per layer instead of chasing
	// in-memory gate pointers that no longer exist. Raw JSON because event
	// must not import pipeline (which imports event).
	PermissionLayers json.RawMessage `json:"permission_layers,omitempty"`
}

// ArtifactInput is one artifact handed to a run as input (S5.8).
type ArtifactInput struct {
	Ref  string `json:"ref"`
	Path string `json:"path"` // workspace-relative destination
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
	// Background marks a task-style activity (S6.1): the tool call pairs
	// with a handle result IMMEDIATELY (the fold renders it from this very
	// event) and the terminal event arrives later, rendered as a user-role
	// input. No separate Task event family — this flag IS the task fact.
	Background bool `json:"background,omitempty"`
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
	// Usage settles tokens the cancelled activity ALREADY spent (S5 review):
	// a steered/aborted child run burned real budget — losing it would let a
	// re-spawn over-grant against the tree cap.
	Usage *provider.Usage `json:"usage,omitempty"`
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

// MalformedToolCall records that a completed LLM call finished with an
// unparseable tool call (S4.6). It drives a bounded retry of the same turn
// (reusing the discard signal); Raw is the model's best-effort output for
// debugging, Error the parse failure.
type MalformedToolCall struct {
	Turn  int    `json:"turn"`
	Raw   string `json:"raw,omitempty"`
	Error string `json:"error,omitempty"`
}

// ToolsDiscovered journals one MCP server's discovered tool schemas (S5.1).
// The server CONNECTION is out-of-band runtime state — only the schemas are
// facts: resume reads them to know the run's tool face, then re-connects out
// of band and reconciles the live schemas against these (drift is refused,
// never silently absorbed — same discipline as 2.13 versioning).
type ToolsDiscovered struct {
	Server string       `json:"server"`
	Tools  []MCPToolDef `json:"tools"`
}

// MCPToolDef is one discovered MCP tool as journaled: fully-qualified name,
// the class the harness assigned (untagged → execute), and the schema.
type MCPToolDef struct {
	Server      string          `json:"server"`
	Name        string          `json:"name"` // fully-qualified mcp__<server>__<tool>
	Description string          `json:"description,omitempty"`
	Class       string          `json:"class"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// SpawnRequested records an adjudicated-allow sub-agent spawn (S5.3), right
// before the child run starts. ChildSession is the child's own journal —
// the parent log holds the REF, never the child's events (fresh child run,
// fault isolation). BudgetTokens is the frozen min-aggregated allowance.
type SpawnRequested struct {
	CallID       string `json:"call_id"`
	Agent        string `json:"agent"`
	Task         string `json:"task"`
	ChildSession string `json:"child_session"`
	Depth        int    `json:"depth"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// SubagentCompleted records the child run's terminal outcome in the PARENT
// log (S5.3): the ref plus the summary facts inspect needs to render the
// tree without opening every child journal.
type SubagentCompleted struct {
	CallID       string         `json:"call_id"`
	Agent        string         `json:"agent"`
	ChildSession string         `json:"child_session"`
	Reason       string         `json:"reason"`
	Turns        int            `json:"turns"`
	Usage        provider.Usage `json:"usage"`
}

// ArtifactPublished records one durable deliverable version (S5.5). The
// blob (and manifest) were fsynced BEFORE this event was appended — the ref
// always resolves; a crash in between leaves an orphan blob, never a
// dangling ref (mirror of the journal's fsync-before-ack).
type ArtifactPublished struct {
	Stream  string `json:"stream"`
	Version int    `json:"version"`
	Ref     string `json:"ref"`
	Bytes   int    `json:"bytes,omitempty"`
	// Source says what published it: "tool" (publish_artifact) or
	// "epilogue" (the outputs-contract auto-publish, S5.6).
	Source string `json:"source,omitempty"`
}

// DriverStarted is the driver stream's header fact (S7 还债: version
// discipline + provenance, mirroring RunStarted 2.17): the spec and fold
// version journaled at series start guard every resume — a fold-shape change
// refuses old streams instead of silently misreading them. Streams predating
// the header (S6) are accepted as fold version 1.
type DriverStarted struct {
	DriverID      string          `json:"driver_id"`
	SpecName      string          `json:"spec_name"`
	Spec          json.RawMessage `json:"spec,omitempty"`
	WorkspaceRoot string          `json:"workspace_root,omitempty"`
	FoldVersion   int             `json:"fold_version"`
}

// IterationScheduled marks one driver iteration as due (S6, DESIGN §运行
// 形态). Goal mode schedules immediately; loop mode via interval/cron/
// self_paced schedules the same fact from a timer.
type IterationScheduled struct {
	DriverID string `json:"driver_id"`
	Iter     int    `json:"iter"`
	Schedule string `json:"schedule,omitempty"`
}

// IterationLaunched records the fresh child run for an iteration, journaled
// BEFORE the child starts (journal-before-send): a crash mid-iteration is
// recoverable because the child session is a deterministic sub-path.
type IterationLaunched struct {
	DriverID     string `json:"driver_id"`
	Iter         int    `json:"iter"`
	ChildSession string `json:"child_session"`
}

// IterationVerdict is one iteration's verification outcome (S6). Score
// normalizes binary (0/1) and metric verifiers alike so stall detection can
// compare iterations on one axis.
type IterationVerdict struct {
	Pass     bool    `json:"pass"`
	Score    float64 `json:"score"`
	Verifier string  `json:"verifier,omitempty"`
	Detail   string  `json:"detail,omitempty"`
}

// IterationCompleted records the child's terminal outcome plus the verdict
// (S6: verdict journals here). CarryRef points at the ArtifactStore carry
// doc; Carry is a short inline excerpt kept for inspect without a blob read.
type IterationCompleted struct {
	DriverID     string           `json:"driver_id"`
	Iter         int              `json:"iter"`
	ChildSession string           `json:"child_session"`
	ChildReason  string           `json:"child_reason"`
	Verdict      IterationVerdict `json:"verdict"`
	Usage        provider.Usage   `json:"usage,omitzero"`
	CarryRef     string           `json:"carry_ref,omitempty"`
	Carry        string           `json:"carry,omitempty"`
}

// IterationSkipped records a schedule tick that did not launch (loop-mode
// overlap=skip) — a fact, never silence (S6, DESIGN §运行形态).
type IterationSkipped struct {
	DriverID string `json:"driver_id"`
	Iter     int    `json:"iter"`
	Reason   string `json:"reason"`
}

// DriverCompleted is the driver's terminal fact (S6). Reason ∈ satisfied |
// stalled | max_iterations | budget | stopped | child_failed. BestIter is
// the 1-based iteration the stall/summary presentation carries forward.
type DriverCompleted struct {
	DriverID   string `json:"driver_id"`
	Reason     string `json:"reason"`
	Iterations int    `json:"iterations"`
	BestIter   int    `json:"best_iter,omitempty"`
}

// CheckpointBarrier is the ONLY legal fork/rewind target (S7 模块 2,
// DESIGN §fork/rewind, weakened semantics): a consistent-enough cut taken
// at a turn boundary — NOT requiring whole-tree quiescence. It records the
// cross-stream cut vector, the workspace snapshot ref, and the in-flight
// background tasks with their fork-time disposition; a fork of this barrier
// treats those tasks per policy instead of pretending they never ran.
type CheckpointBarrier struct {
	BarrierID string `json:"barrier_id"`
	Turn      int    `json:"turn,omitempty"`
	// Vector maps stream (relative dir; "." = this run, "sub/<dir>" =
	// children) → last seq inside the cut. Terminal child streams' cut is
	// their whole journal.
	Vector map[string]int64 `json:"vector"`
	// SnapshotRef is the workspace snapshot (SnapshotStore-opaque). A
	// barrier is only taken when a snapshot succeeded — no ref, no barrier.
	SnapshotRef string `json:"snapshot_ref"`
	// Tasks is the weakened-semantics vector: background tasks in flight at
	// the cut and how a fork disposes of them.
	Tasks []BarrierTask `json:"tasks,omitempty"`
}

// BarrierTask is one in-flight task's fork-time disposition.
type BarrierTask struct {
	TaskID string `json:"task_id"`
	Policy string `json:"policy"` // v0: cancel_at_fork — the fork renders it cancelled
}

// ForkedFrom is a forked session's genesis event (S7 模块 3, DESIGN
// §fork/rewind): the first fact in the new journal, ahead of the events
// copied from the parent's barrier cut. The parent session id is kept as
// provenance; WorkspaceRoot is the fork's OWN worktree (materialized from
// SnapshotRef — forks never share a directory with the original), and
// resume prefers it over the copied RunStarted's stale root.
type ForkedFrom struct {
	ParentSession string `json:"parent_session"`
	BarrierID     string `json:"barrier_id"`
	SnapshotRef   string `json:"snapshot_ref,omitempty"`
	WorkspaceRoot string `json:"workspace_root,omitempty"`
}

// NotificationSent is the notifier's dedup fact (S6 模块⑤): one line per
// delivered (or fallback-delivered) notification, in the notifier's own
// stream. The Key is the dedup identity; startup reconciliation replays
// missed lifecycle moments against this set.
type NotificationSent struct {
	Key     string `json:"key"` // e.g. run_end/<session>, approval/<session>/<id>
	Kind    string `json:"kind"`
	Session string `json:"session,omitempty"`
	Text    string `json:"text,omitempty"`
	Channel string `json:"channel"` // command | stderr
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
	TypeMalformedToolCall: func() any { return &MalformedToolCall{} },
	TypeToolsDiscovered:   func() any { return &ToolsDiscovered{} },
	TypeSpawnRequested:    func() any { return &SpawnRequested{} },
	TypeSubagentCompleted: func() any { return &SubagentCompleted{} },
	TypeArtifactPublished: func() any { return &ArtifactPublished{} },

	TypeDriverStarted:      func() any { return &DriverStarted{} },
	TypeIterationScheduled: func() any { return &IterationScheduled{} },
	TypeIterationLaunched:  func() any { return &IterationLaunched{} },
	TypeIterationCompleted: func() any { return &IterationCompleted{} },
	TypeIterationSkipped:   func() any { return &IterationSkipped{} },
	TypeDriverCompleted:    func() any { return &DriverCompleted{} },

	TypeNotificationSent: func() any { return &NotificationSent{} },

	TypeCheckpointBarrier: func() any { return &CheckpointBarrier{} },
	TypeForkedFrom:        func() any { return &ForkedFrom{} },
}

// DriverStream lists the event types that belong to the IterationDriver's OWN
// stream (S6, DESIGN §运行形态): folded by internal/driver, never by the run
// fold. They never appear in a run journal, so the run fold legitimately has
// no case for them — the run-fold coverage test skips exactly this set, and
// internal/driver carries the matching coverage assertion.
var DriverStream = map[string]bool{
	TypeDriverStarted:      true,
	TypeIterationScheduled: true,
	TypeIterationLaunched:  true,
	TypeIterationCompleted: true,
	TypeIterationSkipped:   true,
	TypeDriverCompleted:    true,
}

// NotifierStream marks the notifier's own stream types (S6 模块⑤), excluded
// from the run fold for the same reason as DriverStream.
var NotifierStream = map[string]bool{
	TypeNotificationSent: true,
}
