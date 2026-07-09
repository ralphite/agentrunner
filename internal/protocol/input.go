// v2 M4.1: the conversational input value that flows client → daemon →
// hosted Loop. Bytes ride the wire base64 (encoding/json's []byte codec);
// the agent puts them into the session CAS BEFORE journaling the input
// (blob-before-event), so the journal itself carries only refs.
package protocol

// ImageAttachment is one image attached to a user input.
type ImageAttachment struct {
	MediaType string `json:"media_type"`
	Data      []byte `json:"data"`
}

// UserInput is one conversational user message with optional attachments.
// DeliverySeq is the durable-mailbox sequence (v2 收口): 0 = not persisted
// (tests, direct wiring); >0 = the daemon fsynced it before acking, and the
// journal's InputReceived echoes it so a resume can replay the unconsumed
// tail exactly once.
type UserInput struct {
	Text        string            `json:"text"`
	Images      []ImageAttachment `json:"images,omitempty"`
	DeliverySeq int64             `json:"delivery_seq,omitempty"`
}

// Control is a non-conversational session-maintenance signal (G7): manual
// context compaction or clear. Like interrupt/kill it flows out of band
// (its own channel, not the inbox) and never enters the conversation; its
// EFFECT (a ContextCompacted event) is what lands in the journal.
type Control struct {
	Kind      string `json:"kind"`                // "compact" | "clear"
	Directive string `json:"directive,omitempty"` // optional focus for a compact
}

// Control kinds.
const (
	ControlCompact = "compact"
	ControlClear   = "clear"
)
