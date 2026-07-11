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
	TypeGenerationDiscarded   = "generation_discarded"
	TypeContextCompacted      = "context_compacted"
	TypeContextMicrocompacted = "context_microcompacted"
	TypeMalformedToolCall     = "malformed_tool_call"

	// S5 additions.
	TypeToolsDiscovered   = "tools_discovered"
	TypeSpawnRequested    = "spawn_requested"
	TypeSubagentCompleted = "subagent_completed"
	TypeChildRevived      = "child_revived"
	TypeArtifactPublished = "artifact_published"
	TypeProgressUpdated   = "progress_updated"
	TypeInputRevoked      = "input_revoked"

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
	TypeGoalAttached          = "goal_attached"
	TypeGoalUpdated           = "goal_updated"
	TypeGoalPaused            = "goal_paused"
	TypeGoalResumed           = "goal_resumed"
	TypeGoalCancelled         = "goal_cancelled"
	TypeGoalCheckpoint        = "goal_checkpoint"
	TypeGoalAchieved          = "goal_achieved"
	TypeGoalCompletionClaimed = "goal_completion_claimed"
	TypeCommandHandled        = "command_handled"
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
	// ProviderCapabilities freezes the normalized provider/model contract at
	// session creation. Empty means a legacy journal predating the envelope.
	ProviderCapabilities *provider.CapabilityEnvelope `json:"provider_capabilities,omitempty"`
}

// ArtifactInput is one artifact handed to a run as input (S5.8).
type ArtifactInput struct {
	Ref  string `json:"ref"`
	Path string `json:"path"` // workspace-relative destination
}

type InputReceived struct {
	Text   string `json:"text"`
	Source string `json:"source"`
	// Turn/Item identity and ingress provenance. Empty fields are accepted for
	// legacy journals and synthesized deterministically by the fold.
	TurnID    string `json:"turn_id,omitempty"`
	ItemID    string `json:"item_id,omitempty"`
	Principal string `json:"principal,omitempty"`
	Trust     string `json:"trust,omitempty"`
	// Content is the canonical typed form. Text/Images/Files remain as a
	// compatible projection for old journals and readers.
	Content []provider.Part `json:"content,omitempty"`
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
	TurnID  string           `json:"turn_id,omitempty"`
	ItemID  string           `json:"item_id,omitempty"`
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
	// Notice augments the immediate handle result for a background launch
	// (for example, escalation denied and the child is running under the
	// narrower fallback). It is model-visible and journaled.
	Notice string `json:"notice,omitempty"`
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
	CallID     string `json:"call_id"`
	Resolution string `json:"resolution"`
	Answer     string `json:"answer"`
	// Answers is the structured form (INC-47): per-question selections from
	// a questions[] ask. Answer stays as the free-text/legacy projection.
	Answers     []AskAnswer `json:"answers,omitempty"`
	DeliverySeq int64       `json:"delivery_seq,omitempty"`
}

// ---- INC-47: structured ask (HANDA #7 / CLAUDECODE #10) ----

// AskOption is one selectable choice of a structured ask question.
type AskOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// AskQuestion is one question of a structured ask_user call. No options =
// a free-text question; MultiSelect allows several selections.
type AskQuestion struct {
	Question      string      `json:"question"`
	Options       []AskOption `json:"options,omitempty"`
	MultiSelect   bool        `json:"multi_select,omitempty"`
	AllowFreeText bool        `json:"allow_free_text,omitempty"`
}

// AskAnswer answers one question by index: selected option labels and/or
// free text.
type AskAnswer struct {
	Question int      `json:"question"`
	Selected []string `json:"selected,omitempty"`
	Text     string   `json:"text,omitempty"`
}

// ---- INC-D1: in-session goal (G23/UJ-22) ----

// GoalVerifier is one check that decides whether the goal is met. v0 supports
// the command kind (a bash command; exit 0 = pass) — the primary UJ-22 case
// ("run the tests N times"). Other kinds (llm_judge / human) are deferred.
type GoalVerifier struct {
	Kind    string `json:"kind"`              // command | llm_judge (INC-48)
	Command string `json:"command,omitempty"` // bash; exit 0 = pass
	// Rubric is the llm_judge kind's grading rubric (INC-48): a strict LLM
	// verdict scores the session's work against it. Empty for command
	// verifiers and legacy journals.
	Rubric string `json:"rubric,omitempty"`
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

// GoalCompletionClaimed records the model's completion claim (INC-10,
// goal_complete tool). It lands mid-turn but is adjudicated only at the next
// quiescence boundary (the goal_verify cell, 决策 #24): with no command
// verifier the audited claim is accepted; with verifiers the verifier remains
// the sole adjudicator. Consumed (cleared from the fold) by the checkpoint
// that adjudicates it, and voided by a GoalUpdated (the objective changed).
type GoalCompletionClaimed struct {
	GoalID  string `json:"goal_id"`
	Summary string `json:"summary,omitempty"`
	Source  string `json:"source"` // model
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
	ApprovalID  string          `json:"approval_id"`
	EffectID    string          `json:"effect_id"`
	CallID      string          `json:"call_id,omitempty"`
	ToolName    string          `json:"tool_name,omitempty"`
	Args        json.RawMessage `json:"args,omitempty"`
	GateResults []GateResult    `json:"gate_results,omitempty"`
	PayloadRef  string          `json:"payload_ref,omitempty"`
	// EstTokens preserves the budget reservation basis across a idle
	// wait (the approval may resolve after a crash+resume).
	EstTokens int `json:"est_tokens,omitempty"`
	// Containment freezes the required OS boundary across a long approval wait;
	// recovery re-probes the backend and fails closed if it cannot restore it.
	Containment *Containment `json:"containment,omitempty"`
	// DenyAllowsFallback is true only for an explicit child-authority ask:
	// denial rejects the widening but permits the already-adjudicated spawn
	// to continue under parent∩child permissions.
	DenyAllowsFallback bool `json:"deny_allows_fallback,omitempty"`
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

// ContextMicrocompacted records a microcompact (INC-13): a NO-LLM context
// reclaim that advances a monotonic boundary; assembly renders re-runnable
// read-class tool results BEFORE the boundary as a short placeholder. The
// journal keeps every result intact (truth) — like the compaction boundary,
// only the assembled view changes, so fork/rewind/resume stay well-defined.
// Boundary is computed by the trigger (policy lives in the agent layer);
// fold applies it monotonically max-wins, which keeps the assembled prefix
// stable between triggers (prompt-cache friendly: bytes change only when
// this event lands, never per-turn).
type ContextMicrocompacted struct {
	Boundary        int `json:"boundary"`
	EstimatedTokens int `json:"estimated_tokens,omitempty"`
	Cleared         int `json:"cleared,omitempty"`
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
	// RoleSpec freezes the constructed child AgentSpec for an inline dynamic
	// role. Static directory spawns leave it empty; the child's own
	// SessionStarted.Spec remains the revive truth in both cases.
	RoleSpec json.RawMessage `json:"role_spec,omitempty"`
	// Escalated marks a USER-APPROVED permission escalation (INC-12.5): this
	// compatibility projection is true exactly when Escalation="approved".
	Escalated bool `json:"escalated,omitempty"`
	// Escalation records the explicit permission-exception outcome. Approved
	// uses the child's declared permission rules; denied keeps parent∩child.
	Escalation       string `json:"escalation,omitempty"`
	EscalationReason string `json:"escalation_reason,omitempty"`
	// Coordinator identity: TaskID is stable for the logical delegation,
	// DependsOn forms its DAG edges, and LeaseID names the active assignment.
	TaskID    string         `json:"task_id,omitempty"`
	DependsOn []string       `json:"depends_on,omitempty"`
	LeaseID   string         `json:"lease_id,omitempty"`
	Workspace *TeamWorkspace `json:"workspace,omitempty"`
	// Replaces records the predecessor handle this delegation retired
	// (spawn_agent.replaces, INC-30): audit-only — the cancel itself settles
	// through the predecessor's own terminal events.
	Replaces string `json:"replaces,omitempty"`
}

// TeamWorkspace is the durable filesystem assignment for one delegation.
// BaseRef pins the parent snapshot used to materialize an isolated child.
type TeamWorkspace struct {
	Mode    string `json:"mode"` // isolated | shared | shared_degraded
	Path    string `json:"path"`
	BaseRef string `json:"base_ref,omitempty"`
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

// ChildRevived re-hosts a quiescent child that received tree mail (INC-12,
// DESIGN §3 静止子唤醒): the parent journals it before resuming the child in
// place — same child journal, same context, original handle. The fold
// re-enters the handle into the in-flight set through a SYNTHETIC background
// activity (ActivityID here) WITHOUT re-pairing the original call; the
// child's next quiescence settles through the ordinary background path
// (second SubagentCompleted + the activity terminal rendering the report).
// BaselineUsage is the child's settled spend at revive time — terminals
// report the delta so the parent's account never double-counts.
type ChildRevived struct {
	CallID        string         `json:"call_id"`
	ActivityID    string         `json:"activity_id"`
	Agent         string         `json:"agent"`
	ChildSession  string         `json:"child_session"`
	Reason        string         `json:"reason"` // message
	BudgetTokens  int            `json:"budget_tokens,omitempty"`
	BaselineUsage provider.Usage `json:"baseline_usage"`
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

// InputRevoked records a queued conversational input consumed AS REVOKED
// (INC-46, §2 rev1): the user withdrew it before the loop injected it, so no
// InputReceived exists for that delivery — this event carries the delivery
// seq instead, and the fold advances ConsumedInputSeq exactly like
// AskResolved does (consume-without-inject template). A revoke arriving
// after the target was already journaled is a no-op and leaves no event.
type InputRevoked struct {
	TargetCommandID string `json:"target_command_id"`
	DeliverySeq     int64  `json:"delivery_seq"`
}

// ProgressUpdated replaces the session's model-maintained progress checklist
// wholesale (INC-37). The model owns the list's content and item identity;
// the fold keeps only the latest table — no merge, no history beyond the
// journal itself. Statuses are normalized at the tool seam, so the fold and
// every consumer read a closed enum.
type ProgressUpdated struct {
	Items []ProgressItem `json:"items"`
}

// ProgressItem is one checklist row. Status is one of
// pending|running|done|failed.
type ProgressItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
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
	Filesystem string `json:"filesystem,omitempty"` // workspace
	Network    string `json:"network"`              // effective egress: none | all
	Backend    string `json:"backend,omitempty"`    // sandbox-exec | bwrap
}

// Registry maps every event type to a constructor for its payload struct.
// Decode helpers and the round-trip test are driven by this table.
var Registry = map[string]func() any{
	TypeSessionStarted:        func() any { return &SessionStarted{} },
	TypeInputReceived:         func() any { return &InputReceived{} },
	TypeGenerationStarted:     func() any { return &GenerationStarted{} },
	TypeAssistantMessage:      func() any { return &AssistantMessage{} },
	TypeActivityStarted:       func() any { return &ActivityStarted{} },
	TypeActivityCompleted:     func() any { return &ActivityCompleted{} },
	TypeActivityFailed:        func() any { return &ActivityFailed{} },
	TypeActivityCancelled:     func() any { return &ActivityCancelled{} },
	TypeTimerSet:              func() any { return &TimerSet{} },
	TypeTimerFired:            func() any { return &TimerFired{} },
	TypeTimerCancelled:        func() any { return &TimerCancelled{} },
	TypeWaitingEntered:        func() any { return &WaitingEntered{} },
	TypeWaitingResolved:       func() any { return &WaitingResolved{} },
	TypeActorCrashed:          func() any { return &ActorCrashed{} },
	TypeSessionClosed:         func() any { return &SessionClosed{} },
	TypeEffectRequested:       func() any { return &EffectRequested{} },
	TypeEffectResolved:        func() any { return &EffectResolved{} },
	TypeApprovalRequested:     func() any { return &ApprovalRequested{} },
	TypeApprovalResponded:     func() any { return &ApprovalResponded{} },
	TypeModeChanged:           func() any { return &ModeChanged{} },
	TypeLimitExceeded:         func() any { return &LimitExceeded{} },
	TypeGenerationDiscarded:   func() any { return &GenerationDiscarded{} },
	TypeContextCompacted:      func() any { return &ContextCompacted{} },
	TypeContextMicrocompacted: func() any { return &ContextMicrocompacted{} },
	TypeMalformedToolCall:     func() any { return &MalformedToolCall{} },
	TypeToolsDiscovered:       func() any { return &ToolsDiscovered{} },
	TypeSpawnRequested:        func() any { return &SpawnRequested{} },
	TypeSubagentCompleted:     func() any { return &SubagentCompleted{} },
	TypeChildRevived:          func() any { return &ChildRevived{} },
	TypeArtifactPublished:     func() any { return &ArtifactPublished{} },
	TypeProgressUpdated:       func() any { return &ProgressUpdated{} },
	TypeInputRevoked:          func() any { return &InputRevoked{} },

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

	TypeGoalAttached:          func() any { return &GoalAttached{} },
	TypeGoalUpdated:           func() any { return &GoalUpdated{} },
	TypeGoalPaused:            func() any { return &GoalPaused{} },
	TypeGoalResumed:           func() any { return &GoalResumed{} },
	TypeGoalCancelled:         func() any { return &GoalCancelled{} },
	TypeGoalCheckpoint:        func() any { return &GoalCheckpoint{} },
	TypeGoalAchieved:          func() any { return &GoalAchieved{} },
	TypeGoalCompletionClaimed: func() any { return &GoalCompletionClaimed{} },
	TypeCommandHandled:        func() any { return &CommandHandled{} },
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
