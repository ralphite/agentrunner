package event

import (
	"encoding/json"
	"time"

	"github.com/ralphite/agentrunner/internal/provider"
)

// The S2 event type set. S3+ may only ADD types, never change these.
const (
	TypeSessionStarted    = "session_started"
	TypeInputReceived     = "input_received"
	TypeGenerationStarted = "generation_started"
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
	TypeSessionClosed     = "session_closed"

	// S3 additions (the S2 set above never changes).
	TypeEffectRequested   = "effect_requested"
	TypeEffectResolved    = "effect_resolved"
	TypeApprovalRequested = "approval_requested"
	TypeApprovalResponded = "approval_responded"
	TypeModeChanged       = "mode_changed"
	TypeLimitExceeded     = "limit_exceeded"

	// S4 additions.
	TypeGenerationDiscarded = "generation_discarded"
	TypeContextCompacted    = "context_compacted"
	TypeMalformedToolCall   = "malformed_tool_call"

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

	// 决策 #32 (2026-07-05): session 不绑 agent——运行中换 spec。
	TypeSpecChanged = "spec_changed"

	// INC-5: ask_user (wait-class) resolution. It pairs the pending
	// ask_user call as a tool result — answered (the user's inbox reply),
	// interrupted, or rejected (a second ask_user in one turn). The reply
	// arrives via the inbox but journals here, carrying its text, in the
	// same family as ApprovalResponded (a content-bearing reply event, not
	// a bare InputReceived).
	TypeAskResolved = "ask_resolved"

	// INC-D1 (决策 #21 拆分, G23/UJ-22): in-session goal. A goal hangs on the
	// conversational session; at the exchange boundary a verifier runs, and a
	// miss re-injects a program-source InputReceived so the SAME thread
	// continues IN CONTEXT (contrast the driver-goal's fresh-child-run form,
	// §13). These are run-fold events (a Goal sub-state), NOT the driver
	// stream. Control (attach/pause/resume/update/cancel) rides the same
	// out-of-band channel as compact/clear.
	TypeGoalAttached   = "goal_attached"
	TypeGoalUpdated    = "goal_updated"
	TypeGoalPaused     = "goal_paused"
	TypeGoalResumed    = "goal_resumed"
	TypeGoalCancelled  = "goal_cancelled"
	TypeGoalCheckpoint = "goal_checkpoint"
	TypeGoalAchieved   = "goal_achieved"
	TypeCommandHandled = "command_handled"
)

// CommandHandled is the semantic no-op/completion receipt for a durable
// command when no domain event would otherwise carry its CommandID.
type CommandHandled struct {
	CommandID  string `json:"command_id"`
	CommandSeq int64  `json:"command_seq,omitempty"`
	Kind       string `json:"kind"`
	Result     string `json:"result,omitempty"`
}

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
)

type SessionStarted struct {
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
	// SpecPath is the original spec file location (M5.1) — sibling
	// sub-agent specs resolve relative to it on a revived session.
	SpecPath string `json:"spec_path,omitempty"`
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
	// Images and Files are CAS refs of blobs attached to this input (v2
	// M4.1/M4.3). blob-before-event: the blob is in the CAS before this
	// event lands. Files carries folded long pastes and documents.
	Images []AttachmentRef `json:"images,omitempty"`
	Files  []AttachmentRef `json:"files,omitempty"`
	// DeliverySeq echoes the durable mailbox seq this input consumed (v2
	// 收口); 0 for non-mailbox sources. The fold's high-water mark drives
	// crash replay of the unconsumed mailbox tail.
	DeliverySeq int64 `json:"delivery_seq,omitempty"`
}

// AttachmentRef is one attached blob: its CAS ref and media type.
type AttachmentRef struct {
	Ref       string `json:"ref"`
	MediaType string `json:"media_type"`
}

type GenerationStarted struct {
	GenStep int `json:"gen_step"`
}

type AssistantMessage struct {
	GenStep int              `json:"gen_step"`
	Message provider.Message `json:"message"`
	// Finish records an abnormal normalized finish reason ("blocked") —
	// the audit fact that visibly truncates the turn (决策 #30). Empty on
	// normal finishes.
	Finish string `json:"finish,omitempty"`
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

// AskResolved pairs a parked ask_user call as its tool result (INC-5).
// Resolution: "answered" (Answer is the user's reply → {"answer": text},
// and the reply grants a fresh turn), "interrupted" or "rejected" (Answer
// is the model-visible error text → IsError). DeliverySeq echoes the
// durable mailbox seq an answered reply consumed (dedup), 0 otherwise.
type AskResolved struct {
	CallID      string `json:"call_id"`
	Resolution  string `json:"resolution"`
	Answer      string `json:"answer"`
	DeliverySeq int64  `json:"delivery_seq,omitempty"`
}

// ---- INC-D1: in-session goal (G23/UJ-22) ----

// GoalVerifier is one check that decides whether the goal is met. v0 supports
// the command kind (a bash command; exit 0 = pass) — the primary UJ-22 case
// ("run the tests N times"). Other kinds (llm_judge / human) are deferred.
type GoalVerifier struct {
	Kind    string `json:"kind"`              // command
	Command string `json:"command,omitempty"` // bash; exit 0 = pass
}

// GoalBudget bounds an in-session goal so a never-passing verifier still
// terminates (决策 #31 可见截断). v0 caps the number of checks; token/wallclock
// caps are deferred.
type GoalBudget struct {
	MaxChecks int `json:"max_checks,omitempty"`
}

// GoalAttached hangs a goal on the session. The verifier runs at the exchange
// boundary; the context continues across checks (contrast driver-goal, §13).
type GoalAttached struct {
	GoalID    string         `json:"goal_id"`
	Goal      string         `json:"goal"`
	Verifiers []GoalVerifier `json:"verifiers"`
	Budget    GoalBudget     `json:"budget"`
	Source    string         `json:"source"` // user
}

// GoalUpdated is change-as-event for a live goal (决策 #32 同族): a non-empty
// field replaces; verifiers replace wholesale when present.
type GoalUpdated struct {
	GoalID    string         `json:"goal_id"`
	Goal      string         `json:"goal,omitempty"`
	Verifiers []GoalVerifier `json:"verifiers,omitempty"`
	Budget    *GoalBudget    `json:"budget,omitempty"`
	Source    string         `json:"source"`
}

type GoalPaused struct {
	GoalID string `json:"goal_id"`
	Source string `json:"source"`
}

type GoalResumed struct {
	GoalID string `json:"goal_id"`
	Source string `json:"source"`
}

type GoalCancelled struct {
	GoalID string `json:"goal_id"`
	Reason string `json:"reason,omitempty"`
	Source string `json:"source"`
}

// GoalCheckpoint records one verifier evaluation at a quiescence boundary. A
// pass leads to GoalAchieved; a miss (with budget left) re-injects Feedback as
// a program-source input so the same thread continues. GenStep is the
// idempotency key: a resume that finds this gen step already checkpointed
// recovers instead of re-running the verifier (INC-D1 crash-recovery, R1/R2).
type GoalCheckpoint struct {
	GoalID   string `json:"goal_id"`
	GenStep  int    `json:"gen_step"`
	Check    int    `json:"check"` // 1-based
	Pass     bool   `json:"pass"`
	Detail   string `json:"detail,omitempty"`
	Feedback string `json:"feedback,omitempty"` // re-injected on a miss; kept for recovery
}

// GoalAchieved detaches the goal: reason satisfied (verifier passed), budget
// (checks exhausted → visible truncation), or cancelled.
type GoalAchieved struct {
	GoalID string `json:"goal_id"`
	Reason string `json:"reason"` // satisfied | budget | cancelled
	Checks int    `json:"checks"`
}

type ActorCrashed struct {
	Actor string `json:"actor"`
	Error string `json:"error"`
}

// SessionClosed is a close/kill MARK (决策 #30): the recorded fact that
// someone explicitly closed (reason "closed") or killed (reason "killed")
// the session, with the origin that did it. Marks are only ever CHECKED —
// automatic paths (timer sweep, boot sweep) do not wake a marked session,
// and a user-killed child revives only for the user — never a state: an
// explicit send lawfully continues any session, and the next generation
// step clears the mark. There is no terminal state and no delivery-receipt
// event; quiescence is a journal shape (决策 #31).
type SessionClosed struct {
	Reason   string         `json:"reason"`           // closed | killed
	Source   string         `json:"source,omitempty"` // user | parent
	GenSteps int            `json:"gen_steps"`
	Usage    provider.Usage `json:"usage"`
}

// SpecChanged swaps the session's agent spec mid-session (决策 #32): a
// session is NOT bound to an agent — the user switches specs with no
// confirmation (the switch itself is the intent; approval exists only for
// an agent escalating its own child, never for the user's own switch).
// The event carries the full new spec and the re-frozen prefix blocks: an
// EXPLICIT prefix generation change — the cache break is deliberate and
// journaled, never silent drift.
type SpecChanged struct {
	SpecName string          `json:"spec_name"`
	Model    string          `json:"model"`
	Spec     json.RawMessage `json:"spec"`
	SpecPath string          `json:"spec_path,omitempty"`
	Source   string          `json:"source"` // user
	// Re-frozen prefix blocks for the new generation (same lifecycle as
	// their SessionStarted originals).
	Env    string `json:"env,omitempty"`
	Memory string `json:"memory,omitempty"`
	Skills string `json:"skills,omitempty"`
	Agents string `json:"agents,omitempty"`
	// PermissionLayers replaces the journaled effective-rules layers for
	// resumes after the switch (same encoding as SessionStarted's).
	PermissionLayers json.RawMessage `json:"permission_layers,omitempty"`
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

// ApprovalRequested goes idle an ask-verdict effect for a human decision. It
// carries every gate's judgment (the prompt must show the full picture)
// and a reserved payload_ref for large payloads via the S7 ArtifactStore.
type ApprovalRequested struct {
	ApprovalID  string       `json:"approval_id"`
	EffectID    string       `json:"effect_id"`
	CallID      string       `json:"call_id,omitempty"`
	GateResults []GateResult `json:"gate_results,omitempty"`
	PayloadRef  string       `json:"payload_ref,omitempty"`
	// EstTokens preserves the budget reservation basis across a idle
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

// GenerationDiscarded marks an LLM turn whose partial stream was thrown away
// before a retry (S4.1): a durable companion to the ephemeral delta
// channel, telling a resuming surface to reopen the stream. The fold has
// no half-built assistant message to undo (assistant_message lands only on
// success), so this event is audit + surface-signal only.
type GenerationDiscarded struct {
	GenStep int    `json:"gen_step"`
	Reason  string `json:"reason,omitempty"`
}

// ContextCompacted records a compaction (S4.5): the output of a summarizer
// LLM call (a nondeterministic recorded activity) that REPLACES the
// conversation prefix folded so far with Summary. It changes subsequent
// fold results — fold to seq N and you get the pre- or post-compaction view
// depending on which side of this event N lands, which is what makes
// fork/rewind across the boundary well-defined. Summary is inlined in S4;
// SummaryRef (ArtifactStore) is reserved for later.
type ContextCompacted struct {
	UptoGenStep  int    `json:"upto_gen_step"`
	Summary      string `json:"summary"`
	DroppedTurns int    `json:"dropped_turns,omitempty"`
	SummaryRef   string `json:"summary_ref,omitempty"`
	// Cleared marks a /clear (G7): the boundary advances with an EMPTY
	// summary — the prior context is dropped outright, no summarizer runs.
	// Additive-optional (决策 #18: no schema bump); lets the timeline label
	// a clear honestly instead of as a compaction with an empty summary.
	Cleared bool `json:"cleared,omitempty"`
}

// MalformedToolCall records that a completed LLM call finished with an
// unparseable tool call (S4.6). It drives a bounded retry of the same turn
// (reusing the discard signal); Raw is the model's best-effort output for
// debugging, Error the parse failure.
type MalformedToolCall struct {
	GenStep int    `json:"gen_step"`
	Raw     string `json:"raw,omitempty"`
	Error   string `json:"error,omitempty"`
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
	GenSteps     int            `json:"gen_steps"`
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
// discipline + provenance, mirroring SessionStarted 2.17): the spec and fold
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
	// BaseRef pins the workspace snapshot a parallel attempt's worktree
	// materializes from (S7 best-of-N): every attempt of one round shares
	// the same base, and a resume re-materializes the SAME tree instead of
	// snapshotting whatever the workspace has drifted to.
	BaseRef string `json:"base_ref,omitempty"`
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
	GenStep   int    `json:"gen_step,omitempty"`
	// Vector maps stream (relative dir; "." = this run, "sub/<dir>" =
	// children) → last seq inside the cut. Terminal child streams' cut is
	// their whole journal.
	Vector map[string]int64 `json:"vector"`
	// SnapshotRef is the workspace snapshot (SnapshotStore-opaque). A
	// barrier is only taken when a snapshot succeeded — no ref, no barrier.
	SnapshotRef string `json:"snapshot_ref"`
	// Handles is the weakened-semantics vector: background work in flight
	// at the cut and how a fork disposes of it.
	Handles []BarrierHandle `json:"handles,omitempty"`
}

// BarrierHandle is one in-flight handle's fork-time disposition.
type BarrierHandle struct {
	Handle string `json:"handle"`
	Policy string `json:"policy"` // v0: cancel_at_fork — the fork renders it cancelled
}

// ForkedFrom is a forked session's genesis event (S7 模块 3, DESIGN
// §fork/rewind): the first fact in the new journal, ahead of the events
// copied from the parent's barrier cut. The parent session id is kept as
// provenance; WorkspaceRoot is the fork's OWN worktree (materialized from
// SnapshotRef — forks never share a directory with the original), and
// resume prefers it over the copied SessionStarted's stale root.
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
	// Containment records the OS-level containment in force for this
	// execution (S7 模块 5) — the journal states what actually bounded the
	// effect, not what was wished for. Absent = uncontained (pre-S7 runs
	// and runs without a sandbox spec).
	Containment *Containment `json:"containment,omitempty"`
}

// Containment is the effective sandbox scope applied to an execution.
type Containment struct {
	Network string `json:"network"`           // effective egress: none | all
	Backend string `json:"backend,omitempty"` // netns | none
}

// Registry maps every event type to a constructor for its payload struct.
// Decode helpers and the round-trip test are driven by this table.
var Registry = map[string]func() any{
	TypeSessionStarted:      func() any { return &SessionStarted{} },
	TypeInputReceived:       func() any { return &InputReceived{} },
	TypeGenerationStarted:   func() any { return &GenerationStarted{} },
	TypeAssistantMessage:    func() any { return &AssistantMessage{} },
	TypeActivityStarted:     func() any { return &ActivityStarted{} },
	TypeActivityCompleted:   func() any { return &ActivityCompleted{} },
	TypeActivityFailed:      func() any { return &ActivityFailed{} },
	TypeActivityCancelled:   func() any { return &ActivityCancelled{} },
	TypeTimerSet:            func() any { return &TimerSet{} },
	TypeTimerFired:          func() any { return &TimerFired{} },
	TypeTimerCancelled:      func() any { return &TimerCancelled{} },
	TypeWaitingEntered:      func() any { return &WaitingEntered{} },
	TypeWaitingResolved:     func() any { return &WaitingResolved{} },
	TypeActorCrashed:        func() any { return &ActorCrashed{} },
	TypeSessionClosed:       func() any { return &SessionClosed{} },
	TypeEffectRequested:     func() any { return &EffectRequested{} },
	TypeEffectResolved:      func() any { return &EffectResolved{} },
	TypeApprovalRequested:   func() any { return &ApprovalRequested{} },
	TypeApprovalResponded:   func() any { return &ApprovalResponded{} },
	TypeModeChanged:         func() any { return &ModeChanged{} },
	TypeLimitExceeded:       func() any { return &LimitExceeded{} },
	TypeGenerationDiscarded: func() any { return &GenerationDiscarded{} },
	TypeContextCompacted:    func() any { return &ContextCompacted{} },
	TypeMalformedToolCall:   func() any { return &MalformedToolCall{} },
	TypeToolsDiscovered:     func() any { return &ToolsDiscovered{} },
	TypeSpawnRequested:      func() any { return &SpawnRequested{} },
	TypeSubagentCompleted:   func() any { return &SubagentCompleted{} },
	TypeArtifactPublished:   func() any { return &ArtifactPublished{} },

	TypeDriverStarted:      func() any { return &DriverStarted{} },
	TypeIterationScheduled: func() any { return &IterationScheduled{} },
	TypeIterationLaunched:  func() any { return &IterationLaunched{} },
	TypeIterationCompleted: func() any { return &IterationCompleted{} },
	TypeIterationSkipped:   func() any { return &IterationSkipped{} },
	TypeDriverCompleted:    func() any { return &DriverCompleted{} },

	TypeNotificationSent: func() any { return &NotificationSent{} },

	TypeCheckpointBarrier: func() any { return &CheckpointBarrier{} },
	TypeForkedFrom:        func() any { return &ForkedFrom{} },
	TypeSpecChanged:       func() any { return &SpecChanged{} },
	TypeAskResolved:       func() any { return &AskResolved{} },

	TypeGoalAttached:   func() any { return &GoalAttached{} },
	TypeGoalUpdated:    func() any { return &GoalUpdated{} },
	TypeGoalPaused:     func() any { return &GoalPaused{} },
	TypeGoalResumed:    func() any { return &GoalResumed{} },
	TypeGoalCancelled:  func() any { return &GoalCancelled{} },
	TypeGoalCheckpoint: func() any { return &GoalCheckpoint{} },
	TypeGoalAchieved:   func() any { return &GoalAchieved{} },
	TypeCommandHandled: func() any { return &CommandHandled{} },
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
