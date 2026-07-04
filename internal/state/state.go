// Package state defines the fold: state = fold(Apply, events). Apply is a
// pure function — it never mutates its input (containers are cloned on
// write) and never reads the clock. Everything the loop needs to decide
// its next move lives here, in namespaced sub-states.
package state

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
)

// SubStateVersions is the schema version of each namespace; the set is
// copied into RunStarted and into every snapshot header. Bump a version
// when a sub-state's shape changes incompatibly.
func SubStateVersions() map[string]int {
	return map[string]int{
		"conversation": 1,
		"activities":   1,
		"waiting":      1,
		"timers":       1,
		"run":          1,
		"effects":      1, // S3.2 (declared in the 2.4 table as an S3 addition)
		"mode":         1, // S3.6a
		"budget":       1, // S3.7a (reservations; settled usage lives in run)
		"compaction":   1, // S4.5 (context-compaction view)
	}
}

// Run statuses.
const (
	StatusRunning = "running"
	StatusWaiting = "waiting"
	StatusEnded   = "ended"
)

type State struct {
	Conversation Conversation `json:"conversation"`
	Activities   Activities   `json:"activities"`
	Waiting      *Waiting     `json:"waiting,omitempty"`
	Timers       Timers       `json:"timers"`
	Run          Run          `json:"run"`
	Effects      Effects      `json:"effects"`
	// Mode is the current run mode (3.6a); empty folds as "default".
	Mode string `json:"mode,omitempty"`
	// Budget holds live reservations (3.7a); settled usage is Run.Usage.
	Budget Budget `json:"budget"`
	// Compaction is the context-compaction view (S4.5): the summary that
	// replaces the message prefix and the boundary it replaces up to. The
	// full Conversation.Messages slice is kept intact (the log is truth);
	// assembly reads the compacted view through this.
	Compaction Compaction `json:"compaction"`
}

// Compaction is the folded result of ContextCompacted (S4.5): messages
// [0:Boundary] are replaced by Summary when assembling the provider request.
// Latest compaction wins — a second compaction re-summarizes (its summary
// already folds in the prior one) and advances the boundary.
type Compaction struct {
	Summary  string `json:"summary,omitempty"`
	Boundary int    `json:"boundary,omitempty"`
	UptoTurn int    `json:"upto_turn,omitempty"`
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
}

// EffectIDFromApprovalID recovers the effect id from an approval id
// (approval ids are minted as "apr-<effect_id>").
func EffectIDFromApprovalID(approvalID string) string {
	return strings.TrimPrefix(approvalID, "apr-")
}

// AwaitingApprovalEffect returns the effect id of the currently parked
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

// Timers is the pending set; resume reschedules whatever is still here.
type Timers map[string]event.TimerSet

// Waiting is the parked run (2.14): nil when not waiting.
type Waiting struct {
	Kind   string          `json:"kind"`
	Detail json.RawMessage `json:"detail,omitempty"`
	Since  int64           `json:"since"` // seq of WaitingEntered
}

type Run struct {
	Status    string         `json:"status"`
	SpecName  string         `json:"spec_name,omitempty"`
	Model     string         `json:"model,omitempty"`
	Task      string         `json:"task,omitempty"`
	Version   string         `json:"version,omitempty"`
	Turn      int            `json:"turn"`
	Reason    string         `json:"reason,omitempty"`
	Usage     provider.Usage `json:"usage"`
	LastCrash string         `json:"last_crash,omitempty"`
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
	// lifecycle as Env.
	Memory string `json:"memory,omitempty"`
	Skills string `json:"skills,omitempty"`
}

// New is the empty pre-RunStarted state.
func New() State {
	return State{
		Conversation: Conversation{ToolResults: map[string]ToolResult{}},
		Activities:   Activities{},
		Timers:       Timers{},
		Effects: Effects{
			Pending:   map[string]event.EffectRequested{},
			Allowed:   map[string]bool{},
			Decisions: map[string]string{},
		},
		Budget: Budget{Reserved: map[string]int{}},
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
	case *event.RunStarted:
		s.Run.Status = StatusRunning
		s.Run.SpecName, s.Run.Model, s.Run.Task, s.Run.Version = p.SpecName, p.Model, p.Task, p.Version
		s.Run.Env = p.Env
		s.Run.Memory, s.Run.Skills = p.Memory, p.Skills

	case *event.InputReceived:
		// Interrupts are journaled control inputs (journal-inputs-first),
		// not conversation content — they never become user messages.
		if p.Source != "interrupt" {
			s.Conversation = s.Conversation.withMessage(provider.Message{
				Role:  provider.RoleUser,
				Parts: []provider.Part{{Kind: provider.PartText, Text: p.Text}},
			})
		}

	case *event.TurnStarted:
		s.Run.Turn = p.Turn
		s.Run.MalformedRetries = 0

	case *event.AssistantMessage:
		s.Conversation = s.Conversation.withMessage(p.Message)
		s.Run.MalformedRetries = 0

	case *event.MalformedToolCall:
		s.Run.MalformedRetries++

	case *event.ToolsDiscovered:
		// Replace this server's tools (re-discovery wins), keep other
		// servers', and keep the whole face sorted by name — a stable face
		// keeps the advertised tool list (and thus the prompt) stable.
		kept := make([]event.MCPToolDef, 0, len(s.Run.MCPTools)+len(p.Tools))
		for _, t := range s.Run.MCPTools {
			if t.Server != p.Server {
				kept = append(kept, t)
			}
		}
		kept = append(kept, p.Tools...)
		sort.Slice(kept, func(i, j int) bool { return kept[i].Name < kept[j].Name })
		s.Run.MCPTools = kept

	case *event.ContextCompacted:
		// The full message log stays intact (truth); the boundary freezes at
		// the message count folded so far, and assembly reads only
		// messages[Boundary:] preceded by Summary. Latest compaction wins.
		s.Compaction = Compaction{
			Summary:  p.Summary,
			Boundary: len(s.Conversation.Messages),
			UptoTurn: p.UptoTurn,
		}

	case *event.ActivityStarted:
		s.Activities = s.Activities.with(p.ActivityID, *p)

	case *event.ActivityCompleted:
		started, inFlight := s.Activities[p.ActivityID]
		s.Activities = s.Activities.without(p.ActivityID)
		s.Effects = s.Effects.withoutAllowed(effectIDFor(started, p.ActivityID))
		s.Budget = s.Budget.release(effectIDFor(started, p.ActivityID))
		if p.Usage != nil {
			s.Run.Usage = addUsage(s.Run.Usage, *p.Usage)
		}
		if inFlight && started.Kind == event.KindTool && started.CallID != "" {
			s.Conversation = s.Conversation.withToolResult(started.CallID,
				ToolResult{Result: p.Result, IsError: p.IsError})
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
		if inFlight && started.Kind == event.KindTool && started.CallID != "" {
			// The rendered failure IS the call's model-visible result: the
			// loop continues, the model reacts (3.9).
			s.Conversation = s.Conversation.withToolResult(started.CallID,
				ToolResult{Result: errs.RenderForModel(errs.Class(p.Error.Class), p.Error.Message), IsError: true})
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
		if inFlight && started.Kind == event.KindTool && started.CallID != "" {
			result, _ := json.Marshal(map[string]string{
				"error":          "[interrupted by user]",
				"partial_output": p.PartialOutput,
			})
			s.Conversation = s.Conversation.withToolResult(started.CallID,
				ToolResult{Result: result, IsError: true})
		}

	case *event.TimerSet:
		s.Timers = s.Timers.with(p.TimerID, *p)

	case *event.TimerFired:
		s.Timers = s.Timers.without(p.TimerID)

	case *event.TimerCancelled:
		s.Timers = s.Timers.without(p.TimerID)

	case *event.WaitingEntered:
		s.Waiting = &Waiting{Kind: p.Kind, Detail: p.Detail, Since: env.Seq}
		s.Run.Status = StatusWaiting

	case *event.WaitingResolved:
		s.Waiting = nil
		s.Run.Status = StatusRunning

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
			s.Run.Status = StatusRunning
		}

	case *event.ModeChanged:
		s.Mode = p.To

	case *event.LimitExceeded:
		// Audit fact; the terminal state lands via RunEnded.

	case *event.TurnDiscarded:
		// Surface signal + audit only: no fold state to undo (the discarded
		// turn never produced a durable assistant_message).

	case *event.ActorCrashed:
		s.Run.LastCrash = p.Actor + ": " + p.Error

	case *event.RunEnded:
		s.Run.Status = StatusEnded
		s.Run.Reason = p.Reason
		s.Run.Turn = p.Turns

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
