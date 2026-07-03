// Package event defines the Envelope wire shape and the full S2 payload
// type set. Events are facts (appended by the EventStore, which assigns
// seq/id/ts); commands are requests (id minted by the sender before the
// input is journaled). Fold determinism rests on event ORDER — nothing in
// this package reads the clock.
package event

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Envelope is one line of events.jsonl. For events, Seq/ID/TS are assigned
// by the EventStore at append time (ID = "evt-<seq>"); a not-yet-appended
// event has Seq 0 and empty ID. Commands carry a sender-minted ID.
type Envelope struct {
	Seq           int64           `json:"seq,omitempty"`
	ID            string          `json:"id,omitempty"`
	CausationID   string          `json:"causation_id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Sender        string          `json:"sender,omitempty"`
	Target        string          `json:"target,omitempty"`
	Type          string          `json:"type"`
	Payload       json.RawMessage `json:"payload"`
	TS            time.Time       `json:"ts,omitzero"`
}

// New builds an unappended envelope for a registered payload type.
func New(typ string, payload any) (Envelope, error) {
	if _, ok := Registry[typ]; !ok {
		return Envelope{}, fmt.Errorf("event: unregistered type %q", typ)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("event: marshal %s payload: %w", typ, err)
	}
	return Envelope{Type: typ, Payload: raw}, nil
}

// ChildOf stamps causation/correlation propagation: the child is caused by
// the parent, and the correlation (root = session id) is inherited.
func (e Envelope) ChildOf(parent Envelope) Envelope {
	e.CausationID = parent.ID
	e.CorrelationID = parent.CorrelationID
	return e
}

// NewCommandID mints a random command id. Commands are external inputs —
// journaled before consumption — so randomness does not break replay.
func NewCommandID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("event: crypto/rand unavailable: %v", err))
	}
	return "cmd-" + hex.EncodeToString(b[:])
}

// EventID derives the deterministic id for an appended event.
func EventID(seq int64) string {
	return fmt.Sprintf("evt-%d", seq)
}

// DecodePayload unmarshals the payload into its registered struct and
// returns a pointer to it. Unknown types are an error — silently dropping
// facts is forbidden.
func DecodePayload(e Envelope) (any, error) {
	mk, ok := Registry[e.Type]
	if !ok {
		return nil, fmt.Errorf("event: unknown type %q (seq %d)", e.Type, e.Seq)
	}
	p := mk()
	if err := json.Unmarshal(e.Payload, p); err != nil {
		return nil, fmt.Errorf("event: decode %s (seq %d): %w", e.Type, e.Seq, err)
	}
	return p, nil
}
