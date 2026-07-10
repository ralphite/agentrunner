// Package state defines the fold: state = fold(Apply, events). Apply is a
// pure function — it never mutates its input (containers are cloned on
// write) and never reads the clock. Everything the loop needs to decide
// its next move lives here, in namespaced sub-states.
package state

import (
	"encoding/json"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
)

// SubStateVersions is the schema version of each namespace; the set is
// copied into SessionStarted and into every snapshot header. Bump a version
// when a sub-state's shape changes incompatibly.
func SubStateVersions() map[string]int {
	return map[string]int{
		"conversation": 1,
		"activities":   1,
		"waiting":      1,
		"timers":       1,
		"session":      2, // 2: 静止模型(决策 #31)——终态字段让位标记(Closed)与截断记账
		"effects":      1, // S3.2 (declared in the 2.4 table as an S3 addition)
		"mode":         1, // S3.6a
		"budget":       1, // S3.7a (reservations; settled usage lives in run)
		"compaction":   1, // S4.5 (context-compaction view)
		"handles":      1, // S6.1 (background task set)
		"barriers":     1, // S7.2 (checkpoint barriers — fork/rewind targets)
		"goal":         1, // INC-D1 (in-session goal — G23/UJ-22)
		"interactions": 1, // INC-11.5 (Turn/Item typed interaction projection)
		"team":         1, // INC-11.6 durable task/DAG/lease/workspace projection
	}
}

// Barrier is one folded checkpoint-barrier record (S7.2): everything a
// fork/rewind needs to locate the cut without re-reading the raw event.
type Barrier struct {
	BarrierID   string                `json:"barrier_id"`
	Seq         int64                 `json:"seq"` // the barrier event's own seq
	GenStep     int                   `json:"gen_step,omitempty"`
	SnapshotRef string                `json:"snapshot_ref"`
	Vector      map[string]int64      `json:"vector"`
	Handles     []event.BarrierHandle `json:"handles,omitempty"`
}

// Session liveness statuses. There is no terminal status (决策 #30/#31):
// close/kill are MARKS (Session.Closed), quiescence is a SHAPE (Quiescence).
const (
	StatusRunning = "running"
	StatusWaiting = "waiting"
)

type State struct {
	Conversation Conversation `json:"conversation"`
	Activities   Activities   `json:"activities"`
	Waiting      *Waiting     `json:"waiting,omitempty"`
	Timers       Timers       `json:"timers"`
	Session      Session      `json:"session"`
	Effects      Effects      `json:"effects"`
	// Mode is the current run mode (3.6a); empty folds as "default".
	Mode string `json:"mode,omitempty"`
	// Budget holds live reservations (3.7a); settled usage is Run.Usage.
	Budget Budget `json:"budget"`
	// Handles is the in-flight background work set (S6.1): handle → the ActivityStarted fact.
	Handles Handles `json:"handles"`
	// Barriers are the fork/rewind targets taken so far (S7.2), in order.
	Barriers []Barrier `json:"barriers,omitempty"`
	// Compaction is the context-compaction view (S4.5): the summary that
	// replaces the message prefix and the boundary it replaces up to. The
	// full Conversation.Messages slice is kept intact (the log is truth);
	// assembly reads the compacted view through this.
	Compaction Compaction `json:"compaction"`
	// Goal is the in-session goal hanging on this session (INC-D1, G23/UJ-22);
	// nil = none. Its verifier runs at the exchange boundary (epilogue) and a
	// miss re-injects a program-source input so the thread continues.
	Goal *Goal `json:"goal,omitempty"`
	// Interactions is the durable Turn/Item projection. Conversation remains
	// the provider-compatible view; both are folded from the same events.
	Interactions Interactions `json:"interactions"`
	// Team is the durable coordinator view: logical delegation task → DAG,
	// active lease, assigned member, workspace and last settlement.
	Team map[string]TeamTask `json:"team,omitempty"`
}

type TeamTask struct {
	TaskID      string               `json:"task_id"`
	CallID      string               `json:"call_id"`
	Description string               `json:"description"`
	DependsOn   []string             `json:"depends_on,omitempty"`
	LeaseID     string               `json:"lease_id,omitempty"`
	AssignedTo  string               `json:"assigned_to,omitempty"`
	Workspace   *event.TeamWorkspace `json:"workspace,omitempty"`
	Status      string               `json:"status"` // leased | quiescent | failed | cancelled
	LastReason  string               `json:"last_reason,omitempty"`
	Settlements int                  `json:"settlements,omitempty"`
}

func teamWith(in map[string]TeamTask, task TeamTask) map[string]TeamTask {
	out := make(map[string]TeamTask, len(in)+1)
	for id, old := range in {
		out[id] = old
	}
	out[task.TaskID] = task
	return out
}

func teamSettle(in map[string]TeamTask, callID, reason string) map[string]TeamTask {
	out := make(map[string]TeamTask, len(in))
	for id, task := range in {
		if task.CallID == callID {
			task.Status = "quiescent"
			if reason == "error" || reason == "contract_violation" || reason == "crash" {
				task.Status = "failed"
			}
			if reason == "cancelled" || reason == "killed" {
				task.Status = "cancelled"
			}
			task.LastReason = reason
			task.Settlements++
			task.LeaseID = ""
		}
		out[id] = task
	}
	return out
}

func teamRevive(in map[string]TeamTask, callID, leaseID string) map[string]TeamTask {
	out := make(map[string]TeamTask, len(in))
	for id, task := range in {
		if task.CallID == callID {
			task.Status, task.LeaseID = "leased", leaseID
		}
		out[id] = task
	}
	return out
}

const (
	ItemMessage    = "message"
	ItemToolCall   = "tool_call"
	ItemToolResult = "tool_result"
)

type Turn struct {
	TurnID    string   `json:"turn_id"`
	StartedAt int64    `json:"started_at"`
	ItemIDs   []string `json:"item_ids"`
}

type Item struct {
	ItemID     string          `json:"item_id"`
	TurnID     string          `json:"turn_id"`
	Kind       string          `json:"kind"`
	Role       provider.Role   `json:"role,omitempty"`
	Principal  string          `json:"principal,omitempty"`
	Source     string          `json:"source,omitempty"`
	Trust      string          `json:"trust,omitempty"`
	Content    []provider.Part `json:"content,omitempty"`
	ActivityID string          `json:"activity_id,omitempty"`
	CallID     string          `json:"call_id,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	IsError    bool            `json:"is_error,omitempty"`
	Seq        int64           `json:"seq"`
}

type Interactions struct {
	ActiveTurnID string          `json:"active_turn_id,omitempty"`
	Turns        []Turn          `json:"turns,omitempty"`
	Items        map[string]Item `json:"items,omitempty"`
}

func (x Interactions) withItem(item Item) Interactions {
	out := Interactions{ActiveTurnID: x.ActiveTurnID, Items: make(map[string]Item, len(x.Items)+1)}
	for id, old := range x.Items {
		out.Items[id] = old
	}
	_, existed := out.Items[item.ItemID]
	out.Items[item.ItemID] = item
	out.Turns = make([]Turn, len(x.Turns))
	copy(out.Turns, x.Turns)
	found := false
	for i := range out.Turns {
		out.Turns[i].ItemIDs = append([]string(nil), x.Turns[i].ItemIDs...)
		if out.Turns[i].TurnID == item.TurnID {
			found = true
			if !existed {
				out.Turns[i].ItemIDs = append(out.Turns[i].ItemIDs, item.ItemID)
			}
		}
	}
	if !found {
		out.Turns = append(out.Turns, Turn{TurnID: item.TurnID,
			StartedAt: item.Seq, ItemIDs: []string{item.ItemID}})
	}
	return out
}

func legacyPrincipal(source string) string {
	switch source {
	case "program", "control", "interrupt":
		return "runtime"
	case "parent", "background":
		return "agent"
	default:
		return "legacy-user"
	}
}

func legacyTrust(source string) string {
	switch source {
	case "program", "control", "interrupt":
		return "system"
	case "parent", "background":
		return "delegated"
	default:
		return "legacy"
	}
}

func (x Interactions) withToolResult(started event.ActivityStarted, activityID string,
	result json.RawMessage, isError bool, seq int64) Interactions {
	turnID := x.ActiveTurnID
	if call, ok := x.Items["item-"+activityID+"-call"]; ok {
		turnID = call.TurnID
	}
	if turnID == "" {
		turnID = "turn-unassigned"
	}
	return x.withItem(Item{
		ItemID: "item-" + activityID + "-result", TurnID: turnID,
		Kind: ItemToolResult, Role: provider.RoleTool,
		Principal: "runtime", Source: "tool", Trust: "tool",
		ActivityID: activityID, CallID: started.CallID,
		Result: result, IsError: isError, Seq: seq,
	})
}

// Goal is the folded in-session goal (INC-D1). change-as-event (决策 #32):
// Attached sets it, Updated mutates it, Paused/Resumed flip Paused, Checkpoint
// counts a check, Cancelled/Achieved clear it.
type Goal struct {
	GoalID    string               `json:"goal_id"`
	Goal      string               `json:"goal"`
	Verifiers []event.GoalVerifier `json:"verifiers"`
	Budget    event.GoalBudget     `json:"budget"`
	Checks    int                  `json:"checks"`
	Paused    bool                 `json:"paused,omitempty"`
	// CheckpointedGenStep + LastPass + LastFeedback are the crash-recovery guard
	// (INC-D1 R1/R2): if a resume re-runs the goal_verify hook at a gen step
	// already checkpointed, it recovers the recorded verdict instead of
	// re-running the verifier — LastPass → re-emit GoalAchieved{satisfied},
	// budget-spent → GoalAchieved{budget}, else re-inject LastFeedback iff
	// absent. So a crash between the checkpoint and the achieved/re-inject event
	// neither re-runs the verifier, double-injects, nor drops the receipt.
	CheckpointedGenStep int    `json:"checkpointed_gen_step,omitempty"`
	LastPass            bool   `json:"last_pass,omitempty"`
	LastFeedback        string `json:"last_feedback,omitempty"`
	// Claimed + ClaimSummary carry a pending goal_complete claim (INC-10)
	// until the next quiescence boundary adjudicates it. A GoalCheckpoint
	// consumes the claim; a GoalUpdated voids it (the objective changed).
	Claimed      bool   `json:"claimed,omitempty"`
	ClaimSummary string `json:"claim_summary,omitempty"`
}

// Compaction is the folded result of ContextCompacted (S4.5): messages
// [0:Boundary] are replaced by Summary when assembling the provider request.
// Latest compaction wins — a second compaction re-summarizes (its summary
// already folds in the prior one) and advances the boundary.
type Compaction struct {
	Summary     string `json:"summary,omitempty"`
	Boundary    int    `json:"boundary,omitempty"`
	UptoGenStep int    `json:"upto_gen_step,omitempty"`
	// MicroBoundary (INC-13, additive-optional per 决策 #18): messages before
	// this index render re-runnable read-class tool results as a placeholder
	// at assembly time. Monotonic — max-wins across events — so the
	// assembled prefix only changes when a ContextMicrocompacted lands.
	MicroBoundary int `json:"micro_boundary,omitempty"`
}

// Budget is the reservation set: effect_resolved{allow, reserved_tokens}
// adds, the activity's terminal event releases. The budget gate sees
// settled + reserved — the reserve-then-settle discipline is what makes
// concurrent adjudication (S4.3) TOCTOU-safe.
type Budget struct {
	Reserved map[string]int `json:"reserved,omitempty"`
}

// ReservedTotal sums outstanding reservations.
func (b Budget) ReservedTotal() int {
	total := 0
	for _, n := range b.Reserved {
		total += n
	}
	return total
}

// CurrentMode returns the effective mode ("default" when unset).
func (s State) CurrentMode() string {
	if s.Mode == "" {
		return "default"
	}
	return s.Mode
}

// Effects tracks adjudication state (3.2/3.5). Pending: entered the gates,
// no resolution yet (resume in-doubt signal for side-effecting pipelines).
// Allowed: resolved allow but the execution has not reached its terminal
// event — after a crash, adjudication is NOT repeated (an approval already
// granted must not be re-asked). Decisions: the durable human answer to an
// approval, keyed by effect id — set the instant ApprovalResponded is
// journaled, so a crash between the response and EffectResolved never
// re-asks (the answer is authoritative from the moment it is a fact).
type Effects struct {
	Pending   map[string]event.EffectRequested `json:"pending,omitempty"`
	Allowed   map[string]bool                  `json:"allowed,omitempty"`
	Decisions map[string]string                `json:"decisions,omitempty"`
	// Authorities preserves an explicit escalation approval/denial while
	// its allowed spawn has not started/settled. It closes the crash window
	// between EffectResolved and SpawnRequested.
	Authorities map[string]string `json:"authorities,omitempty"`
}

// EffectIDFromApprovalID recovers the effect id from an approval id
// (approval ids are minted as "apr-<effect_id>").
func EffectIDFromApprovalID(approvalID string) string {
	return strings.TrimPrefix(approvalID, "apr-")
}

// AwaitingApprovalEffect returns the effect id of the currently idle
// approval, if any. Reaching a WAITING_APPROVAL means every gate — hooks
// included — already ran, so this effect is NOT in-doubt.
func (s State) AwaitingApprovalEffect() string {
	if s.Waiting == nil || s.Waiting.Kind != event.WaitApproval {
		return ""
	}
	var req event.ApprovalRequested
	if err := json.Unmarshal(s.Waiting.Detail, &req); err != nil {
		return ""
	}
	return req.EffectID
}

// Conversation is the transcript plus tool results keyed by call_id —
// the 2.10 request assembly reads exactly this.
type Conversation struct {
	Messages    []provider.Message    `json:"messages"`
	ToolResults map[string]ToolResult `json:"tool_results"`
}

type ToolResult struct {
	Result  json.RawMessage `json:"result,omitempty"`
	IsError bool            `json:"is_error,omitempty"`
}

// Activities is the in-flight set (standing hook 3): ActivityStarted adds,
// any terminal event removes. An entry present at resume time IS the
// in-doubt signal (2.15).
type Activities map[string]event.ActivityStarted

// Handles is the in-flight background work set (S6.1, the handles
// sub-state): handle (= the launching call_id) → the ActivityStarted fact.
// Folded from ActivityStarted{Background}; the activity's terminal event
// removes it and renders the outcome as a user-role input.
type Handles map[string]event.ActivityStarted

func (t Handles) with(id string, v event.ActivityStarted) Handles {
	out := make(Handles, len(t)+1)
	for k, vv := range t {
		out[k] = vv
	}
	out[id] = v
	return out
}

func (t Handles) without(id string) Handles {
	if _, ok := t[id]; !ok {
		return t
	}
	out := make(Handles, len(t))
	for k, vv := range t {
		if k != id {
			out[k] = vv
		}
	}
	return out
}

// Timers is the pending set; resume reschedules whatever is still here.
type Timers map[string]event.TimerSet

// Waiting is the idle run (2.14): nil when not waiting.
type Waiting struct {
	Kind   string          `json:"kind"`
	Detail json.RawMessage `json:"detail,omitempty"`
	Since  int64           `json:"since"` // seq of WaitingEntered
}

type Session struct {
	Status   string `json:"status"`
	SpecName string `json:"spec_name,omitempty"`
	Model    string `json:"model,omitempty"`
	Task     string `json:"task,omitempty"`
	Version  string `json:"version,omitempty"`
	GenStep  int    `json:"gen_step"`
	// Closed is the close/kill mark (决策 #30): set by SessionClosed,
	// cleared by the next GenerationStarted (a lawful reopen). Automatic
	// paths CHECK it (timer/boot sweep skip marked sessions; a user-killed
	// child revives only for the user); it never blocks an explicit send.
	Closed *CloseMark `json:"closed,omitempty"`
	// TruncatedAtGenStep/TruncatedKind record a budget truncation at this
	// gen step (决策 #30 可见截断): the turn ended by LimitExceeded, so the
	// resolved-calls shape at this step counts as finished (Quiescence).
	// TruncatedMsgCount pins the transcript length at the truncation:
	// messages beyond it arrived AFTER — the only inputs that lawfully
	// restart a truncated session (plus the queued-input generation_steps
	// case, whose baseline reset grants the fresh budget).
	TruncatedAtGenStep int            `json:"truncated_at_gen_step,omitempty"`
	TruncatedKind      string         `json:"truncated_kind,omitempty"`
	TruncatedMsgCount  int            `json:"truncated_msg_count,omitempty"`
	Usage              provider.Usage `json:"usage"`
	LastCrash          string         `json:"last_crash,omitempty"`
	// MalformedRetries counts consecutive malformed_tool_call finishes on the
	// current turn (S4.6). Reset when a turn starts or an assistant message
	// lands; the loop escalates to a user-visible error past a bound.
	MalformedRetries int `json:"malformed_retries,omitempty"`
	// Env is the frozen environment block (S4.4c): rendered once at session
	// start and injected verbatim into the prompt prefix on every turn, so
	// the cacheable prefix stays byte-stable as the conversation grows.
	Env string `json:"env,omitempty"`
	// MCPTools is the journaled MCP tool face (S5.1): the schemas discovered
	// at session start. The connections themselves are out-of-band runtime
	// state; the fold only knows the facts a resume needs to rebuild the
	// advertised face and reconcile a re-connect. Sorted by Name.
	MCPTools []event.MCPToolDef `json:"mcp_tools,omitempty"`
	// Memory and Skills are the frozen prompt-prefix blocks (S5.2), same
	// lifecycle as Env. Agents is the sub-agent directory block (S5.3).
	Memory string `json:"memory,omitempty"`
	Skills string `json:"skills,omitempty"`
	Agents string `json:"agents,omitempty"`
	// Spawns counts SpawnRequested facts (S5.3): the fan-out gate's input.
	Spawns int `json:"spawns,omitempty"`
	// Published maps stream → latest published version (S5.5): the outputs
	// contract (S5.6) checks required streams against it at the epilogue.
	Published map[string]int `json:"published,omitempty"`
	// Inputs are the artifact refs to materialize (S5.8, from SessionStarted);
	// Materialized records that the materialize activity completed, so a
	// crash-resume knows whether to (re-)run it (it is idempotent anyway).
	Inputs       []event.ArtifactInput `json:"inputs,omitempty"`
	Materialized bool                  `json:"materialized,omitempty"`
	// ChildSessions lists completed child runs' sessions in completion order
	// (S7.2, additive): the barrier's cross-stream vector reads it.
	ChildSessions []string `json:"child_sessions,omitempty"`
	// ForkedFrom is a forked session's provenance (S7.3, additive): set by
	// the genesis event, nil for a run born from `run`.
	ForkedFrom *ForkOrigin `json:"forked_from,omitempty"`
	// LastInputGenStep is the turn at which the latest conversation-visible
	// user input landed (v2 M3 triage): the conversational turn budget is
	// per turn, counted from here — a cumulative cap would wedge a
	// long-lived session once GenStep passed max_generation_steps.
	LastInputGenStep int `json:"last_input_gen_step,omitempty"`
	// ConsumedInputSeq is the mailbox high-water mark (v2 收口): the
	// largest DeliverySeq among journaled inputs. Resume replays mailbox
	// entries above it — 崩溃不丢输入 becomes literally true.
	ConsumedInputSeq     int64                        `json:"consumed_input_seq,omitempty"`
	ProviderCapabilities *provider.CapabilityEnvelope `json:"provider_capabilities,omitempty"`
}

// ForkOrigin records where a forked session came from.
type ForkOrigin struct {
	ParentSession string `json:"parent_session"`
	BarrierID     string `json:"barrier_id"`
}

// CloseMark is the folded close/kill mark (决策 #30): who marked the
// session and how. A mark is checked, never a state machine.
type CloseMark struct {
	Reason string `json:"reason"`           // closed | killed
	Source string `json:"source,omitempty"` // user | parent
}

// New is the empty pre-SessionStarted state.
func New() State {
	return State{
		Conversation: Conversation{ToolResults: map[string]ToolResult{}},
		Activities:   Activities{},
		Timers:       Timers{},
		Effects: Effects{
			Pending:     map[string]event.EffectRequested{},
			Allowed:     map[string]bool{},
			Decisions:   map[string]string{},
			Authorities: map[string]string{},
		},
		Budget:       Budget{Reserved: map[string]int{}},
		Handles:      Handles{},
		Interactions: Interactions{Items: map[string]Item{}},
		Team:         map[string]TeamTask{},
	}
}

// Fold folds all events over the empty state.
func Fold(events []event.Envelope) (State, error) {
	s := New()
	for _, e := range events {
		var err error
		if s, err = Apply(s, e); err != nil {
			return State{}, err
		}
	}
	return s, nil
}

// Apply folds one event into the state. Pure: the input state is never
// mutated. Unknown event types are an error — facts must not be dropped.
func Apply(s State, env event.Envelope) (State, error) {
	decoded, err := event.DecodePayload(env)
	if err != nil {
		return State{}, err
	}
	switch p := decoded.(type) {
	case *event.SessionStarted:
		s.Session.Status = StatusRunning
		s.Session.SpecName, s.Session.Model, s.Session.Task, s.Session.Version = p.SpecName, p.Model, p.Task, p.Version
		s.Session.Env = p.Env
		s.Session.Memory, s.Session.Skills, s.Session.Agents = p.Memory, p.Skills, p.Agents
		s.Session.Inputs = p.Inputs
		if p.ProviderCapabilities != nil {
			caps := *p.ProviderCapabilities
			s.Session.ProviderCapabilities = &caps
		}

	case *event.InputReceived:
		// Interrupts and control signals (user kill) are journaled control
		// inputs (journal-inputs-first), not conversation content — they
		// never become user messages and never grant turn budget.
		if p.DeliverySeq > s.Session.ConsumedInputSeq {
			s.Session.ConsumedInputSeq = p.DeliverySeq
		}
		if p.Source != "interrupt" && p.Source != "control" {
			parts := append([]provider.Part(nil), p.Content...)
			if len(parts) == 0 {
				parts = []provider.Part{{Kind: provider.PartText, Text: p.Text}}
				// Attached images/files fold as ref-only parts (v2 M4.1/M4.3):
				// bytes stay in the CAS; assembly inflates them before the wire.
				for _, img := range p.Images {
					parts = append(parts, provider.Part{
						Kind: provider.PartImage, Ref: img.Ref, MediaType: img.MediaType,
					})
				}
				for _, f := range p.Files {
					parts = append(parts, provider.Part{
						Kind: provider.PartFile, Ref: f.Ref, MediaType: f.MediaType,
					})
				}
			}
			s.Conversation = s.Conversation.withMessage(provider.Message{
				Role:  provider.RoleUser,
				Parts: parts,
			})
			s.Session.LastInputGenStep = s.Session.GenStep
			turnID := p.TurnID
			if turnID == "" {
				turnID = "turn-legacy-" + strconv.FormatInt(env.Seq, 10)
			}
			itemID := p.ItemID
			if itemID == "" {
				itemID = "item-legacy-" + strconv.FormatInt(env.Seq, 10)
			}
			principal, trust := p.Principal, p.Trust
			if principal == "" {
				principal = legacyPrincipal(p.Source)
			}
			if trust == "" {
				trust = legacyTrust(p.Source)
			}
			s.Interactions.ActiveTurnID = turnID
			s.Interactions = s.Interactions.withItem(Item{ItemID: itemID,
				TurnID: turnID, Kind: ItemMessage, Role: provider.RoleUser,
				Principal: principal, Source: p.Source, Trust: trust,
				Content: parts, Seq: env.Seq})
		}

	case *event.GenerationStarted:
		s.Session.GenStep = p.GenStep
		// A lawful reopen (决策 #30): the new generation step clears the
		// close/kill mark and any truncation record — the shape is live again.
		s.Session.Status = StatusRunning
		s.Session.Closed = nil
		s.Session.TruncatedAtGenStep = 0
		s.Session.TruncatedKind = ""
		s.Session.TruncatedMsgCount = 0
		s.Session.MalformedRetries = 0

	case *event.AssistantMessage:
		s.Conversation = s.Conversation.withMessage(p.Message)
		turnID := p.TurnID
		if turnID == "" {
			turnID = s.Interactions.ActiveTurnID
		}
		if turnID == "" {
			turnID = "turn-legacy-gen-" + strconv.Itoa(p.GenStep)
		}
		itemID := p.ItemID
		if itemID == "" {
			if env.Seq == 0 {
				itemID = "item-assistant-g" + strconv.Itoa(p.GenStep)
			} else {
				itemID = "item-legacy-" + strconv.FormatInt(env.Seq, 10)
			}
		}
		s.Interactions.ActiveTurnID = turnID
		s.Interactions = s.Interactions.withItem(Item{ItemID: itemID,
			TurnID: turnID, Kind: ItemMessage, Role: provider.RoleAssistant,
			Principal: "model", Source: "provider", Trust: "model",
			Content: append([]provider.Part(nil), p.Message.Parts...), Seq: env.Seq})
		s.Session.MalformedRetries = 0
		if p.Finish != "" {
			// An abnormal finish (blocked) visibly truncates the turn
			// (决策 #30): the session will idle rather than continue.
			s.Session.TruncatedAtGenStep = s.Session.GenStep
			s.Session.TruncatedKind = p.Finish
			s.Session.TruncatedMsgCount = len(s.Conversation.Messages)
		}

	case *event.MalformedToolCall:
		s.Session.MalformedRetries++

	case *event.SpawnRequested:
		s.Session.Spawns++
		taskID := p.TaskID
		if taskID == "" {
			taskID = "task-" + p.CallID
		}
		s.Team = teamWith(s.Team, TeamTask{TaskID: taskID, CallID: p.CallID,
			Description: p.Task, DependsOn: append([]string(nil), p.DependsOn...),
			LeaseID: p.LeaseID, AssignedTo: p.ChildSession, Workspace: p.Workspace,
			Status: "leased"})

	case *event.CheckpointBarrier:
		// Copy-on-write: barriers append into a fresh slice.
		barriers := make([]Barrier, 0, len(s.Barriers)+1)
		barriers = append(barriers, s.Barriers...)
		s.Barriers = append(barriers, Barrier{
			BarrierID: p.BarrierID, Seq: env.Seq, GenStep: p.GenStep,
			SnapshotRef: p.SnapshotRef, Vector: p.Vector, Handles: p.Handles,
		})

	case *event.ForkedFrom:
		// Genesis of a forked session (S7.3): provenance only — every other
		// aspect of the state comes from the copied cut that follows.
		s.Session.ForkedFrom = &ForkOrigin{ParentSession: p.ParentSession, BarrierID: p.BarrierID}

	case *event.SubagentCompleted:
		// The parent's accounting settles through the spawn activity's
		// ActivityCompleted, never here; the fold only records the child
		// stream's existence for the barrier vector (S7.2, copy-on-write).
		// A revived child completes AGAIN (INC-12) — the list stays deduped
		// so the barrier vector opens each child stream once.
		if !slices.Contains(s.Session.ChildSessions, p.ChildSession) {
			children := make([]string, 0, len(s.Session.ChildSessions)+1)
			children = append(children, s.Session.ChildSessions...)
			s.Session.ChildSessions = append(children, p.ChildSession)
		}
		s.Team = teamSettle(s.Team, p.CallID, p.Reason)

	case *event.ChildRevived:
		// A quiescent child re-enters the in-flight set (INC-12, DESIGN §3
		// 静止子唤醒) through a SYNTHETIC background activity: the original
		// handle keeps working for kill/output, the child's next terminal
		// settles through the ordinary ActivityCompleted path (which renders
		// the report as a user-role message), and the ORIGINAL call is NOT
		// re-paired — its tool result landed when the spawn first returned.
		// Args carry the agent name + revive baseline for the crash-settle
		// path (settle-from-child-fold reports the delta, never the total).
		args, _ := json.Marshal(map[string]any{
			"agent": p.Agent, "revive": true, "baseline": p.BaselineUsage,
		})
		started := event.ActivityStarted{
			ActivityID: p.ActivityID, Kind: event.KindTool, Name: "spawn_agent",
			Args: args, CallID: p.CallID, Attempt: 1, Background: true,
		}
		s.Activities = s.Activities.with(p.ActivityID, started)
		s.Handles = s.Handles.with(p.CallID, started)
		if p.BudgetTokens > 0 {
			// Reserve-then-settle holds for revives too: the allowance
			// reserves up front under the same effect id the activity
			// terminal releases.
			s.Budget = s.Budget.withReservation(effectIDFor(started, p.ActivityID), p.BudgetTokens)
		}
		s.Team = teamRevive(s.Team, p.CallID, p.ActivityID)

	case *event.ArtifactPublished:
		// Copy-on-write: Apply is pure, the input map must not mutate.
		published := make(map[string]int, len(s.Session.Published)+1)
		for k, v := range s.Session.Published {
			published[k] = v
		}
		published[p.Stream] = p.Version
		s.Session.Published = published

	case *event.ToolsDiscovered:
		// Replace this server's tools (re-discovery wins), keep other
		// servers', and keep the whole face sorted by name — a stable face
		// keeps the advertised tool list (and thus the prompt) stable.
		kept := make([]event.MCPToolDef, 0, len(s.Session.MCPTools)+len(p.Tools))
		for _, t := range s.Session.MCPTools {
			if t.Server != p.Server {
				kept = append(kept, t)
			}
		}
		kept = append(kept, p.Tools...)
		sort.Slice(kept, func(i, j int) bool { return kept[i].Name < kept[j].Name })
		s.Session.MCPTools = kept

	case *event.ContextCompacted:
		// The full message log stays intact (truth); the boundary freezes at
		// the message count folded so far, and assembly reads only
		// messages[Boundary:] preceded by Summary. Latest compaction wins.
		// MicroBoundary survives: indices before Compaction.Boundary never
		// reach assembly anyway, and a stale micro boundary inside the kept
		// suffix keeps rendering the same placeholders (stable view).
		s.Compaction = Compaction{
			Summary:       p.Summary,
			Boundary:      len(s.Conversation.Messages),
			UptoGenStep:   p.UptoGenStep,
			MicroBoundary: s.Compaction.MicroBoundary,
		}

	case *event.ContextMicrocompacted:
		// Monotonic max-wins (INC-13): the boundary never retreats, so the
		// assembled view of any prefix is stable across resumes and forks.
		if p.Boundary > s.Compaction.MicroBoundary {
			s.Compaction.MicroBoundary = p.Boundary
		}

	case *event.ActivityStarted:
		s.Activities = s.Activities.with(p.ActivityID, *p)
		if p.Kind == event.KindTool && p.CallID != "" {
			turnID := s.Interactions.ActiveTurnID
			if turnID == "" {
				turnID = "turn-legacy-gen-" + strconv.Itoa(s.Session.GenStep)
			}
			s.Interactions = s.Interactions.withItem(Item{
				ItemID: "item-" + p.ActivityID + "-call", TurnID: turnID,
				Kind: ItemToolCall, Role: provider.RoleAssistant,
				Principal: "model", Source: "provider", Trust: "model",
				Content: []provider.Part{{Kind: provider.PartToolCall, CallID: p.CallID,
					ToolName: p.Name, Args: p.Args}}, ActivityID: p.ActivityID,
				CallID: p.CallID, Seq: env.Seq,
			})
		}
		if p.Background && p.CallID != "" {
			// The handle IS this event's fold rendering (S6.1): the call
			// pairs immediately, and the task enters the tasks sub-state.
			s.Handles = s.Handles.with(p.CallID, *p)
			handlePayload := map[string]string{
				"handle": p.CallID, "status": "running",
			}
			if p.Notice != "" {
				handlePayload["note"] = p.Notice
			}
			handle, _ := json.Marshal(handlePayload)
			s.Conversation = s.Conversation.withToolResult(p.CallID,
				ToolResult{Result: handle})
		}

	case *event.ActivityCompleted:
		started, inFlight := s.Activities[p.ActivityID]
		s.Activities = s.Activities.without(p.ActivityID)
		s.Effects = s.Effects.withoutAllowed(effectIDFor(started, p.ActivityID))
		s.Budget = s.Budget.release(effectIDFor(started, p.ActivityID))
		if p.Usage != nil {
			s.Session.Usage = addUsage(s.Session.Usage, *p.Usage)
		}
		if p.ActivityID == "materialize" {
			s.Session.Materialized = true // artifact inputs are in the workspace (S5.8)
		}
		if inFlight && started.Background && started.CallID != "" {
			// A background task's outcome arrives as a USER-role input
			// (S6.1): the handle already paired the call at start; the
			// result becomes conversation the model sees next turn.
			s.Handles = s.Handles.without(started.CallID)
			s.Conversation = s.Conversation.withMessage(handleOutcomeMessage(
				started.CallID, "completed", string(p.Result)))
		} else if inFlight && started.Kind == event.KindTool && started.CallID != "" {
			s.Conversation = s.Conversation.withToolResult(started.CallID,
				ToolResult{Result: p.Result, IsError: p.IsError})
		}
		if inFlight && started.Kind == event.KindTool && started.CallID != "" {
			s.Interactions = s.Interactions.withToolResult(started, p.ActivityID,
				p.Result, p.IsError, env.Seq)
		}
		// The mode transition is folded from exit_plan_mode's OWN completion
		// so it is atomic — a crash can never leave the tool result saying
		// "now in default mode" while s.Mode is still "plan" (correctness
		// review #2). The gate already guarantees this only fires from plan.
		if inFlight && started.Name == "exit_plan_mode" && !p.IsError {
			s.Mode = ""
		}

	case *event.ActivityFailed:
		if !p.Final {
			// Mid-retry: the entry STAYS in flight — a crash in the backoff
			// window must surface as in-doubt for non-idempotent activities
			// instead of silently re-running (S3 回访项); the next Started
			// overwrites it.
			break
		}
		started, inFlight := s.Activities[p.ActivityID]
		s.Activities = s.Activities.without(p.ActivityID)
		s.Budget = s.Budget.release(effectIDFor(started, p.ActivityID))
		if inFlight && started.Background && started.CallID != "" {
			s.Handles = s.Handles.without(started.CallID)
			s.Conversation = s.Conversation.withMessage(handleOutcomeMessage(
				started.CallID, "failed",
				string(errs.RenderForModel(errs.Class(p.Error.Class), p.Error.Message))))
		} else if inFlight && started.Kind == event.KindTool && started.CallID != "" {
			// The rendered failure IS the call's model-visible result: the
			// loop continues, the model reacts (3.9).
			s.Conversation = s.Conversation.withToolResult(started.CallID,
				ToolResult{Result: errs.RenderForModel(errs.Class(p.Error.Class), p.Error.Message), IsError: true})
		}
		if inFlight && started.Kind == event.KindTool && started.CallID != "" {
			s.Interactions = s.Interactions.withToolResult(started, p.ActivityID,
				errs.RenderForModel(errs.Class(p.Error.Class), p.Error.Message), true, env.Seq)
		}

	case *event.ActivityCancelled:
		// A cancelled tool call resolves to a model-visible error result:
		// decide() must never see it as "still pending" — a crash after
		// this event would otherwise re-run a provably half-executed
		// effect on resume. The rendering matches the 3.5 contract.
		started, inFlight := s.Activities[p.ActivityID]
		s.Activities = s.Activities.without(p.ActivityID)
		s.Effects = s.Effects.withoutAllowed(effectIDFor(started, p.ActivityID))
		s.Budget = s.Budget.release(effectIDFor(started, p.ActivityID))
		if p.Usage != nil {
			// Tokens spent before the cancellation are real spend (S5).
			s.Session.Usage = addUsage(s.Session.Usage, *p.Usage)
		}
		if inFlight && started.Background && started.CallID != "" {
			s.Handles = s.Handles.without(started.CallID)
			s.Conversation = s.Conversation.withMessage(handleOutcomeMessage(
				started.CallID, "canceled", p.PartialOutput))
		} else if inFlight && started.Kind == event.KindTool && started.CallID != "" {
			result, _ := json.Marshal(map[string]string{
				"error":          "[interrupted by user]",
				"partial_output": p.PartialOutput,
			})
			s.Conversation = s.Conversation.withToolResult(started.CallID,
				ToolResult{Result: result, IsError: true})
			s.Interactions = s.Interactions.withToolResult(started, p.ActivityID,
				result, true, env.Seq)
		}

	case *event.TimerSet:
		s.Timers = s.Timers.with(p.TimerID, *p)

	case *event.TimerFired:
		s.Timers = s.Timers.without(p.TimerID)

	case *event.TimerCancelled:
		s.Timers = s.Timers.without(p.TimerID)

	case *event.WaitingEntered:
		s.Waiting = &Waiting{Kind: p.Kind, Detail: p.Detail, Since: env.Seq}
		s.Session.Status = StatusWaiting

	case *event.WaitingResolved:
		s.Waiting = nil
		s.Session.Status = StatusRunning

	case *event.AskResolved:
		// Pair the parked ask_user call as its tool result (INC-5). A crash
		// between this and the WaitingResolved that clears the park is safe:
		// the call is resolved and durable here, so doWait's ask branch just
		// re-journals the resolution instead of re-parking.
		if p.DeliverySeq > s.Session.ConsumedInputSeq {
			s.Session.ConsumedInputSeq = p.DeliverySeq
		}
		var result json.RawMessage
		if p.Resolution == "answered" {
			result, _ = json.Marshal(map[string]string{"answer": p.Answer})
			// The reply is fresh user input: grant it a turn so decide()
			// continues instead of truncating against the pre-ask baseline.
			s.Session.LastInputGenStep = s.Session.GenStep
		} else {
			result, _ = json.Marshal(p.Answer)
		}
		s.Conversation = s.Conversation.withToolResult(p.CallID,
			ToolResult{Result: result, IsError: p.Resolution != "answered"})

	case *event.EffectRequested:
		s.Effects = s.Effects.withPending(p.EffectID, *p)

	case *event.EffectResolved:
		s.Effects = s.Effects.withoutPending(p.EffectID).withoutDecision(p.EffectID)
		if p.Verdict == event.VerdictAllow {
			s.Effects = s.Effects.withAllowed(p.EffectID)
			if p.ReservedTokens > 0 {
				s.Budget = s.Budget.withReservation(p.EffectID, p.ReservedTokens)
			}
		}
		var authorityAsk, authorityDecision string
		for _, result := range p.GateResults {
			if result.Gate == "authority_escalation" {
				authorityAsk = result.Decision
			}
			if result.Gate == "approval" {
				authorityDecision = result.Decision
			}
		}
		if authorityAsk != "" && authorityDecision != "" {
			s.Effects = s.Effects.withAuthority(p.EffectID, authorityDecision)
		}
		// A denial IS the call's model-visible outcome: journaling it
		// resolves the call_id, so decide() never re-attempts a denied
		// effect (and a post-deny crash resumes past it).
		if p.Verdict == event.VerdictDeny && p.CallID != "" {
			reason := deniedReason(p.GateResults)
			result, _ := json.Marshal(map[string]string{"error": "denied: " + reason})
			s.Conversation = s.Conversation.withToolResult(p.CallID,
				ToolResult{Result: result, IsError: true})
		}

	case *event.ApprovalRequested:
		// The request itself is audit; the wait it enters carries the state.

	case *event.ApprovalResponded:
		// The human answer is authoritative the moment it is a fact: record
		// it and clear the approval wait here, so a crash before the derived
		// waiting_resolved / effect_resolved never re-asks (correctness #1/#3).
		s.Effects = s.Effects.withDecision(EffectIDFromApprovalID(p.ApprovalID), p.Decision)
		if s.Waiting != nil && s.Waiting.Kind == event.WaitApproval {
			s.Waiting = nil
			s.Session.Status = StatusRunning
		}

	case *event.ModeChanged:
		s.Mode = p.To

	case *event.SpecChanged:
		// The session switches agents (决策 #32): identity and every frozen
		// prefix block move to the new generation — assembly picks them up
		// on the next generation step (an explicit, journaled cache break).
		s.Session.SpecName, s.Session.Model = p.SpecName, p.Model
		s.Session.Env, s.Session.Memory, s.Session.Skills, s.Session.Agents =
			p.Env, p.Memory, p.Skills, p.Agents

	case *event.LimitExceeded:
		// A visible truncation fact (决策 #30): the turn ends here, the
		// session idles, reopenable as ever. Generation-step exhaustion
		// additionally resets the per-turn budget baseline so a queued input
		// starts a fresh turn instead of wedging against the spent budget.
		s.Session.TruncatedAtGenStep = s.Session.GenStep
		s.Session.TruncatedKind = p.Kind
		s.Session.TruncatedMsgCount = len(s.Conversation.Messages)
		if p.Kind == "generation_steps" {
			s.Session.LastInputGenStep = s.Session.GenStep
		}

	case *event.GenerationDiscarded:
		// Surface signal + audit only: no fold state to undo (the discarded
		// turn never produced a durable assistant_message).

	case *event.CommandHandled:
		// Durable command receipt only; the domain event carries state.

	case *event.ActorCrashed:
		s.Session.LastCrash = p.Actor + ": " + p.Error

	case *event.SessionClosed:
		// A close/kill MARK, not a state transition (决策 #30): liveness
		// fields stay untouched; automatic paths check the mark, the next
		// GenerationStarted clears it.
		s.Session.Closed = &CloseMark{Reason: p.Reason, Source: p.Source}

	// ---- INC-D1: in-session goal (G23/UJ-22) ----
	case *event.GoalAttached:
		s.Goal = &Goal{GoalID: p.GoalID, Goal: p.Goal, Verifiers: p.Verifiers, Budget: p.Budget}

	// The goal cases below are copy-on-write like every other sub-state
	// (INC-10 review): s.Goal is a pointer shared with the caller's previous
	// State value, so mutating in place would break Apply's purity contract.
	case *event.GoalUpdated:
		if s.Goal != nil && s.Goal.GoalID == p.GoalID {
			g := *s.Goal
			if p.Goal != "" {
				g.Goal = p.Goal
			}
			if p.Verifiers != nil {
				g.Verifiers = p.Verifiers
			}
			if p.Budget != nil {
				g.Budget = *p.Budget
			}
			// The objective (or its judge) changed — a pending completion
			// claim no longer speaks for it (INC-10).
			g.Claimed = false
			g.ClaimSummary = ""
			s.Goal = &g
		}

	case *event.GoalPaused:
		if s.Goal != nil && s.Goal.GoalID == p.GoalID {
			g := *s.Goal
			g.Paused = true
			s.Goal = &g
		}

	case *event.GoalResumed:
		if s.Goal != nil && s.Goal.GoalID == p.GoalID {
			g := *s.Goal
			g.Paused = false
			s.Goal = &g
		}

	case *event.GoalCheckpoint:
		// The check count advances and the gen step + feedback are recorded so
		// a resume that re-enters this gen step recovers instead of re-running
		// the verifier (crash-recovery R1/R2).
		if s.Goal != nil && s.Goal.GoalID == p.GoalID {
			g := *s.Goal
			g.Checks = p.Check
			g.CheckpointedGenStep = p.GenStep
			g.LastPass = p.Pass
			g.LastFeedback = p.Feedback
			// The boundary adjudicated any pending claim — consume it.
			g.Claimed = false
			g.ClaimSummary = ""
			s.Goal = &g
		}

	case *event.GoalCancelled:
		if s.Goal != nil && s.Goal.GoalID == p.GoalID {
			s.Goal = nil
		}

	case *event.GoalAchieved:
		if s.Goal != nil && s.Goal.GoalID == p.GoalID {
			s.Goal = nil
		}

	case *event.GoalCompletionClaimed:
		if s.Goal != nil && s.Goal.GoalID == p.GoalID {
			g := *s.Goal
			g.Claimed = true
			g.ClaimSummary = p.Summary
			s.Goal = &g
		}

	default:
		// A type registered in event.Registry but missing here.
		return State{}, &UnhandledEventError{Type: env.Type}
	}
	return s, nil
}

// UnhandledEventError means event.Registry and Apply drifted apart.
type UnhandledEventError struct{ Type string }

func (e *UnhandledEventError) Error() string {
	return "state: registered event type has no fold case: " + e.Type
}

// handleOutcomeMessage renders a background task's terminal outcome as the
// user-role input the model sees next turn (S6.1).
func handleOutcomeMessage(taskID, status, body string) provider.Message {
	return provider.Message{Role: provider.RoleUser, Parts: []provider.Part{{
		Kind: provider.PartText,
		Text: "[background work " + taskID + " " + status + "]\n" + body,
	}}}
}

func deniedReason(results []event.GateResult) string {
	for _, r := range results {
		if r.Decision == event.VerdictDeny {
			if r.Reason != "" {
				return r.Reason
			}
			return "blocked by " + r.Gate
		}
	}
	return "policy"
}

func addUsage(a, b provider.Usage) provider.Usage {
	a.InputTokens += b.InputTokens
	a.OutputTokens += b.OutputTokens
	a.CacheReadTokens += b.CacheReadTokens
	a.CacheWriteTokens += b.CacheWriteTokens
	return a
}

// --- copy-on-write helpers (Apply purity) ---

func (c Conversation) withMessage(m provider.Message) Conversation {
	msgs := make([]provider.Message, len(c.Messages), len(c.Messages)+1)
	copy(msgs, c.Messages)
	c.Messages = append(msgs, m)
	return c
}

func (c Conversation) withToolResult(callID string, r ToolResult) Conversation {
	results := make(map[string]ToolResult, len(c.ToolResults)+1)
	for k, v := range c.ToolResults {
		results[k] = v
	}
	results[callID] = r
	c.ToolResults = results
	return c
}

func (a Activities) with(id string, v event.ActivityStarted) Activities {
	out := make(Activities, len(a)+1)
	for k, x := range a {
		out[k] = x
	}
	out[id] = v
	return out
}

func (a Activities) without(id string) Activities {
	if _, ok := a[id]; !ok {
		return a
	}
	out := make(Activities, len(a))
	for k, x := range a {
		if k != id {
			out[k] = x
		}
	}
	return out
}

func (b Budget) withReservation(id string, tokens int) Budget {
	out := make(map[string]int, len(b.Reserved)+1)
	for k, v := range b.Reserved {
		out[k] = v
	}
	out[id] = tokens
	b.Reserved = out
	return b
}

func (b Budget) release(id string) Budget {
	if _, ok := b.Reserved[id]; !ok {
		return b
	}
	out := make(map[string]int, len(b.Reserved))
	for k, v := range b.Reserved {
		if k != id {
			out[k] = v
		}
	}
	b.Reserved = out
	return b
}

// effectIDFor recovers the effect id from an activity's identity (the
// eff-<call_id> / eff-llm-t<n> convention).
func effectIDFor(started event.ActivityStarted, activityID string) string {
	if started.CallID != "" {
		return "eff-tool-" + started.CallID
	}
	return "eff-" + activityID
}

func (e Effects) withPending(id string, v event.EffectRequested) Effects {
	out := make(map[string]event.EffectRequested, len(e.Pending)+1)
	for k, x := range e.Pending {
		out[k] = x
	}
	out[id] = v
	e.Pending = out
	return e
}

func (e Effects) withoutPending(id string) Effects {
	if _, ok := e.Pending[id]; !ok {
		return e
	}
	out := make(map[string]event.EffectRequested, len(e.Pending))
	for k, x := range e.Pending {
		if k != id {
			out[k] = x
		}
	}
	e.Pending = out
	return e
}

func (e Effects) withDecision(id, decision string) Effects {
	out := make(map[string]string, len(e.Decisions)+1)
	for k, v := range e.Decisions {
		out[k] = v
	}
	out[id] = decision
	e.Decisions = out
	return e
}

func (e Effects) withoutDecision(id string) Effects {
	if _, ok := e.Decisions[id]; !ok {
		return e
	}
	out := make(map[string]string, len(e.Decisions))
	for k, v := range e.Decisions {
		if k != id {
			out[k] = v
		}
	}
	e.Decisions = out
	return e
}

func (e Effects) withAllowed(id string) Effects {
	out := make(map[string]bool, len(e.Allowed)+1)
	for k := range e.Allowed {
		out[k] = true
	}
	out[id] = true
	e.Allowed = out
	return e
}

func (e Effects) withAuthority(id, decision string) Effects {
	out := make(map[string]string, len(e.Authorities)+1)
	for k, v := range e.Authorities {
		out[k] = v
	}
	out[id] = decision
	e.Authorities = out
	return e
}

func (e Effects) withoutAllowed(id string) Effects {
	if _, ok := e.Allowed[id]; !ok {
		return e
	}
	out := make(map[string]bool, len(e.Allowed))
	for k := range e.Allowed {
		if k != id {
			out[k] = true
		}
	}
	e.Allowed = out
	if _, ok := e.Authorities[id]; ok {
		authorities := make(map[string]string, len(e.Authorities))
		for k, v := range e.Authorities {
			if k != id {
				authorities[k] = v
			}
		}
		e.Authorities = authorities
	}
	return e
}

func (t Timers) with(id string, v event.TimerSet) Timers {
	out := make(Timers, len(t)+1)
	for k, x := range t {
		out[k] = x
	}
	out[id] = v
	return out
}

func (t Timers) without(id string) Timers {
	if _, ok := t[id]; !ok {
		return t
	}
	out := make(Timers, len(t))
	for k, x := range t {
		if k != id {
			out[k] = x
		}
	}
	return out
}

// Quiescence reports whether the fold shape is quiescent (决策 #31): the
// last turn finished — final generation, visible truncation, completed
// handoff, or a close/kill mark — with nothing in flight and no pending
// timer. Nobody else will trigger the session and it will not run again by
// itself. reason names the finishing shape ("completed",
// "max_generation_steps", "tokens", "handoff", "closed", "canceled");
// observers (driver settle, exit codes, sweeps) read it off the shape —
// quiescence is never an event and never a state machine.
func Quiescence(s State) (quiescent bool, reason string) {
	if len(s.Activities) > 0 || len(s.Handles) > 0 || len(s.Timers) > 0 {
		return false, ""
	}
	if s.Waiting != nil && s.Waiting.Kind != event.WaitInput {
		return false, "" // waiting on an approval/settlement: work in flight
	}
	if s.Session.Closed != nil {
		if s.Session.Closed.Reason == "killed" {
			return true, "canceled"
		}
		return true, s.Session.Closed.Reason
	}
	// A visible truncation finishes the turn regardless of message shape
	// (决策 #30). It restarts only on input that would actually run —
	// TruncationRestartable mirrors decide(); otherwise the session is
	// quiescent under the truncation's name.
	if s.Session.TruncatedAtGenStep > 0 && s.Session.TruncatedAtGenStep == s.Session.GenStep {
		if TruncationRestartable(s) {
			return false, ""
		}
		switch s.Session.TruncatedKind {
		case "generation_steps":
			return true, "max_generation_steps"
		case "tokens":
			return true, "limit_exceeded"
		default:
			return true, s.Session.TruncatedKind
		}
	}
	if hasInputAfterLastAssistant(s) {
		return false, "" // pending input: a turn is owed
	}
	msgs := s.Conversation.Messages
	var last *provider.Message
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleAssistant {
			last = &msgs[i]
			break
		}
	}
	if last == nil {
		return false, "" // no turn yet: a fresh (or crashed-at-start) session
	}
	hasCalls, handoffOK := false, false
	for _, part := range last.Parts {
		if part.Kind != provider.PartToolCall {
			continue
		}
		hasCalls = true
		tr, done := s.Conversation.ToolResults[part.CallID]
		if !done {
			return false, "" // unresolved call: mid-turn
		}
		if part.ToolName == "handoff_agent" && !tr.IsError {
			handoffOK = true
		}
	}
	if !hasCalls {
		return true, "completed" // final generation shape
	}
	if handoffOK {
		return true, "handoff" // control moved on; this agent acts no more
	}
	return false, "" // resolved calls owe the model a next generation step
}

// TruncationRestartable reports whether a truncated session (决策 #30 可见
// 截断) has input that lawfully restarts it: any message that arrived
// AFTER the truncation (a settlement or a fresh send — one attempt per
// wake, so a broken model never hot-loops), or a queued-before-truncation
// input under a generation_steps truncation, whose baseline reset grants
// the fresh budget.
func TruncationRestartable(s State) bool {
	if len(s.Conversation.Messages) > s.Session.TruncatedMsgCount {
		return true
	}
	if s.Session.TruncatedKind != "generation_steps" {
		return false
	}
	return hasInputAfterLastAssistant(s) && len(s.Conversation.Messages) > 0
}

// hasInputAfterLastAssistant reports a user-role message newer than the
// model's last message — pending input the model has not seen.
func hasInputAfterLastAssistant(s State) bool {
	msgs := s.Conversation.Messages
	for i := len(msgs) - 1; i >= 0; i-- {
		switch msgs[i].Role {
		case provider.RoleAssistant:
			return false
		case provider.RoleUser:
			return true
		}
	}
	return false
}
