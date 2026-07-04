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
// Turn is the 1-based turn number of this call within the run; adapters use
// it to mint deterministic call ids via CallID.
type CompleteRequest struct {
	Model     string
	MaxTokens int
	System    string
	Messages  []Message
	Tools     []ToolDef
	Turn      int
	// Thinking requests extended thinking (S4.7); providers map or downgrade.
	Thinking ThinkingConfig
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

// Billed is the budget-accounting caliber (S4.4c): input + output minus the
// cache-read portion, which was served from a warm prefix and does not cost
// the full input rate. This is the single source of truth for what the
// budget gate charges; raw Input/Output remain for display and telemetry.
// Clamped at zero — a provider reporting more cache reads than input (never
// expected) must not credit the budget.
func (u Usage) Billed() int {
	billed := u.InputTokens + u.OutputTokens - u.CacheReadTokens
	if billed < 0 {
		return 0
	}
	return billed
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
	// FinishMalformedToolCall: the provider emitted a tool call it could not
	// parse (S4.6). The loop records it and retries the turn.
	FinishMalformedToolCall FinishReason = "malformed_tool_call"
	// FinishBlocked: the model stopped for a safety/policy reason. The loop
	// surfaces it as a user-visible error and ends the run (S4.6).
	FinishBlocked FinishReason = "blocked"
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

// Capabilities declares optional provider abilities (S4.7). The loop reads
// these to decide whether to send a thinking config, place cache
// breakpoints, or rely on parallel tool calls — and to DOWNGRADE explicitly
// (never silently) when a spec asks for a feature a provider lacks.
type Capabilities struct {
	Thinking      bool // extended thinking / reasoning tokens
	PromptCaching bool // explicit prompt-cache breakpoints
	ParallelTools bool // multiple tool calls in one assistant turn
}

// ThinkingConfig is the normalized extended-thinking request (S4.7). Enabled
// with a token budget; providers map it (Anthropic thinking config, Gemini
// thinking budget) or downgrade when unsupported.
type ThinkingConfig struct {
	Enabled      bool
	BudgetTokens int
}

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
	return CollectTurnStreaming(stream, nil)
}

// CollectTurnStreaming drains a stream into a Turn, invoking onDelta for
// each text delta as it arrives (S4.1). onDelta may be nil. Deltas are
// ephemeral — only the assembled Turn.Message is durable (TurnDiscarded
// contract): if the call errors mid-stream, whatever was emitted to
// onDelta is discarded by the caller.
func CollectTurnStreaming(stream iter.Seq2[StreamEvent, error], onDelta func(string)) (Turn, error) {
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
			if onDelta != nil && ev.TextDelta != "" {
				onDelta(ev.TextDelta)
			}
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
