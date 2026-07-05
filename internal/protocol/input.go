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
type UserInput struct {
	Text   string            `json:"text"`
	Images []ImageAttachment `json:"images,omitempty"`
}
