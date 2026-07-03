// Package provider defines the normalized LLM provider interface.
//
// The shapes here are the FINAL ones (S1 执行包 / PLAN 1.2): streaming-native,
// opaque per-provider extras on parts (thought signatures land there in S4),
// and a Capabilities stub — so the interface survives unchanged into S2/S4.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
)

// Role is a normalized message role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// PartKind discriminates Part variants.
type PartKind string

const (
	PartText       PartKind = "text"
	PartToolCall   PartKind = "tool_call"
	PartToolResult PartKind = "tool_result"
)

// Part is one content part of a message. CallID pairs tool_call parts with
// their tool_result parts; providers map it to their native pairing scheme
// (Gemini: positional; Anthropic: id-based). Extras carries opaque
// provider-specific payloads (e.g. thought signatures) that must round-trip
// through the event log byte-identically.
type Part struct {
	Kind     PartKind                   `json:"kind"`
	Text     string                     `json:"text,omitempty"`
	CallID   string                     `json:"call_id,omitempty"`
	ToolName string                     `json:"tool_name,omitempty"`
	Args     json.RawMessage            `json:"args,omitempty"`
	Result   json.RawMessage            `json:"result,omitempty"`
	IsError  bool                       `json:"is_error,omitempty"`
	Extras   map[string]json.RawMessage `json:"extras,omitempty"`
}

// Message is a normalized conversation message.
type Message struct {
	Role  Role   `json:"role"`
	Parts []Part `json:"parts"`
}

// ToolDef is the wire-level tool definition a provider advertises to the
// model. The richer data-driven tool registry (1.5) converts into this.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// CompleteRequest is a normalized, provider-agnostic completion request.
type CompleteRequest struct {
	Model     string
	MaxTokens int
	System    string
	Messages  []Message
	Tools     []ToolDef
}

// ToolCall is one tool invocation requested by the model.
type ToolCall struct {
	CallID string                     `json:"call_id"`
	Name   string                     `json:"name"`
	Args   json.RawMessage            `json:"args"`
	Extras map[string]json.RawMessage `json:"extras,omitempty"`
}

// Usage is normalized token accounting. Cache fields stay zero until S4.
type Usage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// FinishReason is the normalized termination shape of one LLM call.
// The abnormal variants (blocked, malformed_tool_call, …) get loop policy
// in S4; the type exists from day one so events never change shape.
type FinishReason string

const (
	FinishEndTurn   FinishReason = "end_turn"
	FinishToolUse   FinishReason = "tool_use"
	FinishMaxTokens FinishReason = "max_tokens"
	FinishOther     FinishReason = "other"
)

// StreamEventKind discriminates StreamEvent variants.
type StreamEventKind string

const (
	EventTextDelta StreamEventKind = "text_delta"
	EventToolCall  StreamEventKind = "tool_call"
	EventUsage     StreamEventKind = "usage"
	EventFinish    StreamEventKind = "finish"
)

// StreamEvent is one element of a completion stream (S1 执行包 event set).
type StreamEvent struct {
	Kind      StreamEventKind
	TextDelta string
	ToolCall  *ToolCall
	Usage     *Usage
	Finish    FinishReason
}

// Capabilities declares optional provider abilities. Stub in S1; caching,
// thinking, and structured-output negotiation land here in S4 (决策 15b).
type Capabilities struct{}

// Provider is the thin, streaming-native LLM interface (决策 15).
type Provider interface {
	// Complete streams one model call. The iterator yields events until the
	// stream ends; a non-nil error terminates the stream.
	Complete(ctx context.Context, req CompleteRequest) iter.Seq2[StreamEvent, error]
	// Capabilities reports what optional features this provider supports.
	Capabilities() Capabilities
}

// CallID builds the harness-generated deterministic call id
// (call_<turn>_<index>, S1 执行包) used for tool call/result pairing.
func CallID(turn, index int) string {
	return fmt.Sprintf("call_%d_%d", turn, index)
}

// Turn is the assembled result of one completed LLM call.
type Turn struct {
	Message   Message // assistant message: text part (if any) + tool_call parts
	ToolCalls []ToolCall
	Usage     Usage
	Finish    FinishReason
}

// CollectTurn drains a stream into a Turn. It exists for turn-granularity
// callers (the S1 loop); streaming callers consume the iterator directly.
func CollectTurn(stream iter.Seq2[StreamEvent, error]) (Turn, error) {
	var (
		turn Turn
		text string
	)
	for ev, err := range stream {
		if err != nil {
			return Turn{}, err
		}
		switch ev.Kind {
		case EventTextDelta:
			text += ev.TextDelta
		case EventToolCall:
			if ev.ToolCall != nil {
				turn.ToolCalls = append(turn.ToolCalls, *ev.ToolCall)
			}
		case EventUsage:
			if ev.Usage != nil {
				turn.Usage = *ev.Usage
			}
		case EventFinish:
			turn.Finish = ev.Finish
		}
	}

	turn.Message.Role = RoleAssistant
	if text != "" {
		turn.Message.Parts = append(turn.Message.Parts, Part{Kind: PartText, Text: text})
	}
	for _, tc := range turn.ToolCalls {
		turn.Message.Parts = append(turn.Message.Parts, Part{
			Kind:     PartToolCall,
			CallID:   tc.CallID,
			ToolName: tc.Name,
			Args:     tc.Args,
			Extras:   tc.Extras,
		})
	}
	return turn, nil
}
