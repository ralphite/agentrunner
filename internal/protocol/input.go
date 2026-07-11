// v2 M4.1: the conversational input value that flows client → daemon →
// hosted Loop. Bytes ride the wire base64 (encoding/json's []byte codec);
// the agent puts them into the session CAS BEFORE journaling the input
// (blob-before-event), so the journal itself carries only refs.
package protocol

import (
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
)

// ContentPart is one typed ingress part before blob materialization. Binary
// Data rides only the client→daemon command; the agent stores it in CAS and
// journals a ref-only provider.Part.
type ContentPart struct {
	Kind      provider.PartKind `json:"kind"`
	Text      string            `json:"text,omitempty"`
	MediaType string            `json:"media_type,omitempty"`
	Data      []byte            `json:"data,omitempty"`
	Ref       string            `json:"ref,omitempty"`
}

// ImageAttachment is one image attached to a user input.
type ImageAttachment struct {
	MediaType string `json:"media_type"`
	Data      []byte `json:"data"`
}

// FileAttachment is one arbitrary-type file attached to a user input (INC-9:
// PDF / any file, generalising the image path). Same wire shape as an image —
// bytes ride base64, the agent puts them in the session CAS before journaling,
// and the journal carries only the ref. The MediaType (sniffed at the CLI)
// drives the provider mapping (Gemini inline_data / Anthropic document block).
type FileAttachment struct {
	MediaType string `json:"media_type"`
	Data      []byte `json:"data"`
}

// UserInput is one conversational user message with optional attachments.
// DeliverySeq is the durable-mailbox sequence (v2 收口): 0 = not persisted
// (tests, direct wiring); >0 = the daemon fsynced it before acking, and the
// journal's InputReceived echoes it so a resume can replay the unconsumed
// tail exactly once.
type UserInput struct {
	Text   string            `json:"text"`
	Images []ImageAttachment `json:"images,omitempty"`
	Files  []FileAttachment  `json:"files,omitempty"`
	// Content is the canonical typed ingress. The three fields above remain
	// wire-compatible shorthands; when Content is non-empty it is authoritative.
	Content []ContentPart `json:"content,omitempty"`
	// Principal identifies who sent the command; Source names the transport or
	// integration ("" / "user" for humans, "agent" for tree-internal messages
	// — INC-12 send_message); Trust is the sender's trust classification.
	// Empty legacy values are normalized at the daemon/agent boundary, never
	// discarded.
	Principal string `json:"principal,omitempty"`
	Source    string `json:"source,omitempty"`
	Trust     string `json:"trust,omitempty"`
	TurnID    string `json:"turn_id,omitempty"`
	ItemID    string `json:"item_id,omitempty"`
	// Target addresses a DESCENDANT session inside the recipient's tree
	// (INC-12.3, `ar send <child-sid>`): the daemon durably logs the command
	// on the TREE ROOT (single host, single writer) and the root loop
	// forwards it to the member's inbox instead of journaling it itself.
	// Empty = the message is for the receiving session.
	Target string `json:"target,omitempty"`
	// CommandID is minted by the caller and remains stable across retries.
	// The durable mailbox rejects reuse with a different payload and returns
	// the original DeliverySeq for an exact retry.
	CommandID   string `json:"command_id,omitempty"`
	DeliverySeq int64  `json:"delivery_seq,omitempty"`
	// Delivery is the per-message delivery mode for a send to a RUNNING session
	// (INC-43, Codex parity): "" / "queue" (default) appends to the inbox and is
	// consumed at the idle — the message enters the NEXT turn (type-ahead);
	// "steer" is consumed at the loop's next safe boundary WITHIN the current
	// turn, so the model sees it this turn — a pure append that does NOT
	// interrupt the in-flight step (symmetric to Spec.Receipts="steer" for
	// background settlements, 裁决 #15). interrupt (a separate command) remains
	// the only channel that cuts a turn.
	Delivery string `json:"delivery,omitempty"`
}

// DeliverySteer is the opt-in mid-turn delivery mode; "" and DeliveryQueue are
// the default next-turn behavior. Normalized at the daemon boundary.
const (
	DeliveryQueue = "queue"
	DeliverySteer = "steer"
)

// CommandRef is the durable receipt carried from the command log to the
// semantic event that applies it.
type CommandRef struct {
	CommandID  string `json:"command_id"`
	CommandSeq int64  `json:"command_seq"`
}

// CancelCommand is an out-of-band kill request with its durable receipt.
type CancelCommand struct {
	CommandRef
	Handle string `json:"handle"`
}

// ApprovalCommand is the durable user response to one pending approval.
type ApprovalCommand struct {
	ApprovalID string `json:"approval_id"`
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
	// Remember (INC-17, G5): "allow and don't ask again" — on approve, persist
	// the effect's criterion as a user-level allow rule for the NEXT session.
	Remember bool `json:"remember,omitempty"`
}

// SessionCommand is the typed durable command-log record. Input is optional
// only for non-input kinds; control/approval/handle carry the other payloads.
type SessionCommand struct {
	CommandRef
	// Target routes a command accepted in one tree member's durable log
	// through the tree root's single hosted process. Empty means the hosted
	// session itself. INC-12.6 uses it for child approval answers.
	Target    string `json:"target,omitempty"`
	Principal string `json:"principal,omitempty"`
	Source    string `json:"source,omitempty"`
	Trust     string `json:"trust,omitempty"`
	// PreviouslyAccepted is response metadata, never persisted. It tells the
	// daemon an exact retry returned an existing receipt, so a completed
	// command is acknowledged without another live wake after restart.
	PreviouslyAccepted bool             `json:"-"`
	Kind               string           `json:"kind"`
	Input              *UserInput       `json:"input,omitempty"`
	Control            *Control         `json:"control,omitempty"`
	Approval           *ApprovalCommand `json:"approval,omitempty"`
	Handle             string           `json:"handle,omitempty"`
}

const (
	CommandInput     = "input"
	CommandControl   = "control"
	CommandInterrupt = "interrupt"
	CommandClose     = "close"
	CommandKill      = "kill"
	CommandApproval  = "approval"
)

// Control is a non-conversational session-maintenance signal (G7 · INC-D1):
// manual context compaction/clear, or in-session goal control. Like
// interrupt/kill it flows out of band (its own channel, not the inbox) and
// never enters the conversation as user content; its EFFECT (a ContextCompacted
// or Goal* event) is what lands in the journal.
type Control struct {
	CommandRef
	Kind      string       `json:"kind"`                // compact | clear | goal_*
	Directive string       `json:"directive,omitempty"` // optional focus for a compact
	Goal      *GoalControl `json:"goal,omitempty"`      // payload for goal_attach / goal_update
}

// GoalControl carries the parameters of an in-session goal control (INC-D1).
// It reuses the event-layer verifier/budget shapes (event does not import
// protocol, so this is not a cycle).
type GoalControl struct {
	GoalID    string               `json:"goal_id"`
	Goal      string               `json:"goal,omitempty"`
	Verifiers []event.GoalVerifier `json:"verifiers,omitempty"`
	Budget    *event.GoalBudget    `json:"budget,omitempty"`
}

// Control kinds.
const (
	ControlCompact    = "compact"
	ControlClear      = "clear"
	ControlRemember   = "remember"
	ControlMode       = "mode"
	ControlGoalAttach = "goal_attach"
	ControlGoalPause  = "goal_pause"
	ControlGoalResume = "goal_resume"
	ControlGoalUpdate = "goal_update"
	ControlGoalCancel = "goal_cancel"
	ControlClose      = "close"
)
