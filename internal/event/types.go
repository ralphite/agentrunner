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
	TypeWaitingEntered    = "waiting_entered"
	TypeWaitingResolved   = "waiting_resolved"
	TypeActorCrashed      = "actor_crashed"
	TypeRunEnded          = "run_ended"
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
	TypeWaitingEntered:    func() any { return &WaitingEntered{} },
	TypeWaitingResolved:   func() any { return &WaitingResolved{} },
	TypeActorCrashed:      func() any { return &ActorCrashed{} },
	TypeRunEnded:          func() any { return &RunEnded{} },
}
