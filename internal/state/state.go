// Package state defines the fold: state = fold(Apply, events). Apply is a
// pure function — it never mutates its input (containers are cloned on
// write) and never reads the clock. Everything the loop needs to decide
// its next move lives here, in namespaced sub-states.
package state

import (
	"encoding/json"

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
}

// Effects tracks adjudication state (3.2/3.5). Pending: entered the gates,
// no resolution yet (resume in-doubt signal for side-effecting pipelines).
// Allowed: resolved allow but the execution has not reached its terminal
// event — after a crash, adjudication is NOT repeated (an approval already
// granted must not be re-asked).
type Effects struct {
	Pending map[string]event.EffectRequested `json:"pending,omitempty"`
	Allowed map[string]bool                  `json:"allowed,omitempty"`
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
}

// New is the empty pre-RunStarted state.
func New() State {
	return State{
		Conversation: Conversation{ToolResults: map[string]ToolResult{}},
		Activities:   Activities{},
		Timers:       Timers{},
		Effects: Effects{
			Pending: map[string]event.EffectRequested{},
			Allowed: map[string]bool{},
		},
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

	case *event.AssistantMessage:
		s.Conversation = s.Conversation.withMessage(p.Message)

	case *event.ActivityStarted:
		s.Activities = s.Activities.with(p.ActivityID, *p)

	case *event.ActivityCompleted:
		started, inFlight := s.Activities[p.ActivityID]
		s.Activities = s.Activities.without(p.ActivityID)
		s.Effects = s.Effects.withoutAllowed(effectIDFor(started, p.ActivityID))
		if p.Usage != nil {
			s.Run.Usage = addUsage(s.Run.Usage, *p.Usage)
		}
		if inFlight && started.Kind == event.KindTool && started.CallID != "" {
			s.Conversation = s.Conversation.withToolResult(started.CallID,
				ToolResult{Result: p.Result, IsError: p.IsError})
		}

	case *event.ActivityFailed:
		// The attempt concluded; a retry re-adds via a fresh Started.
		s.Activities = s.Activities.without(p.ActivityID)

	case *event.ActivityCancelled:
		// A cancelled tool call resolves to a model-visible error result:
		// decide() must never see it as "still pending" — a crash after
		// this event would otherwise re-run a provably half-executed
		// effect on resume. The rendering matches the 3.5 contract.
		started, inFlight := s.Activities[p.ActivityID]
		s.Activities = s.Activities.without(p.ActivityID)
		s.Effects = s.Effects.withoutAllowed(effectIDFor(started, p.ActivityID))
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
		s.Effects = s.Effects.withoutPending(p.EffectID)
		if p.Verdict == event.VerdictAllow {
			s.Effects = s.Effects.withAllowed(p.EffectID)
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

	case *event.ApprovalRequested, *event.ApprovalResponded:
		// State lives in the surrounding waiting_entered/resolved and
		// effect_resolved facts; these are the audit trail.

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

// effectIDFor recovers the effect id from an activity's identity (the
// eff-<call_id> / eff-llm-t<n> convention).
func effectIDFor(started event.ActivityStarted, activityID string) string {
	if started.CallID != "" {
		return "eff-" + started.CallID
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
