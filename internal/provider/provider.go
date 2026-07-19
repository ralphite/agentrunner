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
	PartImage      PartKind = "image" // v2 M4.1: multimodal user input
	PartFile       PartKind = "file"  // v2 M4.1: attached document input
	// PartAudio carries an audio recording for a one-shot transcription
	// (INC-56 `ar dictate`). It is NOT a conversation modality: the agent loop
	// never assembles an audio part into a turn, and it is deliberately absent
	// from the capability envelope's InputModalities (which describe what the
	// conversation accepts — text/image/file). Only the out-of-loop dictate
	// helper builds it, so audio→text stays a composer text convenience, not a
	// model audio modality (DESIGN 非目标 line 36「语音输入」stands).
	PartAudio PartKind = "audio"
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
	// Image/file parts (v2 M4.1). The journal and fold carry ONLY the CAS
	// ref (blob-before-event); Data is inflated from the CAS at assembly
	// time and never serialized — a fold must never embed blob bytes.
	MediaType string `json:"media_type,omitempty"`
	Ref       string `json:"ref,omitempty"`
	Data      []byte `json:"-"`
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
// GenStep is the 1-based turn number of this call within the run; adapters use
// it to mint deterministic call ids via CallID.
type CompleteRequest struct {
	Model     string
	MaxTokens int
	System    string
	Messages  []Message
	Tools     []ToolDef
	GenStep   int
	// Thinking requests extended thinking (S4.7); providers map or downgrade.
	Thinking ThinkingConfig
	// ResponseSchema constrains the completion to JSON matching this schema
	// (INC-35, provider-native structured output). It applies ONLY to a
	// tool-less turn — JSON mode and tool calls are mutually exclusive, so a
	// provider MUST ignore it when Tools is non-empty. Empty = unconstrained.
	// A provider without StructuredOutput never sees it (the loop clears it).
	ResponseSchema json.RawMessage
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
	Thinking      bool `json:"thinking,omitempty"`       // extended thinking / reasoning tokens
	PromptCaching bool `json:"prompt_caching,omitempty"` // explicit prompt-cache breakpoints
	ParallelTools bool `json:"parallel_tools,omitempty"` // multiple tool calls in one assistant turn
	Images        bool `json:"images,omitempty"`         // image input
	Files         bool `json:"files,omitempty"`          // document/file input
	// StructuredOutput: the provider can constrain a tool-less completion to a
	// JSON schema natively (INC-35). Absent it, ResponseSchema is dropped and
	// the CLI validate/retry path (INC-26) is the only structured-output route.
	StructuredOutput bool `json:"structured_output,omitempty"`
}

// CapabilityEnvelope is the versioned, durable description of the provider
// contract a session started with. Core streaming/text/tool support follows
// from the Provider interface itself; optional abilities come from
// Capabilities. Journaling this envelope prevents provider/model identity and
// downgrade decisions from becoming invisible runtime assumptions.
type CapabilityEnvelope struct {
	SchemaVersion   int          `json:"schema_version"`
	Provider        string       `json:"provider"`
	Model           string       `json:"model"`
	Streaming       bool         `json:"streaming"`
	ToolCalls       bool         `json:"tool_calls"`
	InputModalities []PartKind   `json:"input_modalities"`
	Capabilities    Capabilities `json:"capabilities"`
}

// Envelope freezes one provider/model's normalized capability contract.
// NativeStructuredOutput reports whether the NAMED provider constrains a
// tool-less completion to a response schema natively. It is the static
// mirror of that provider's Capabilities().StructuredOutput for callers
// that must decide BEFORE constructing a provider (no credentials at hand)
// — e.g. the CLI choosing the spec output_schema fallback (PLAN 5.7). Keep
// in lockstep with the per-provider Capabilities methods.
func NativeStructuredOutput(providerName string) bool {
	return providerName == "gemini"
}

func Envelope(providerName, model string, caps Capabilities) CapabilityEnvelope {
	modalities := []PartKind{PartText}
	if caps.Images {
		modalities = append(modalities, PartImage)
	}
	if caps.Files {
		modalities = append(modalities, PartFile)
	}
	return CapabilityEnvelope{
		SchemaVersion: 1, Provider: providerName, Model: model,
		Streaming: true, ToolCalls: true, InputModalities: modalities,
		Capabilities: caps,
	}
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

// GenStep is the assembled result of one completed LLM call.
type GenStep struct {
	Message   Message // assistant message: text part (if any) + tool_call parts
	ToolCalls []ToolCall
	Usage     Usage
	Finish    FinishReason
}

// CollectTurn drains a stream into a GenStep. It exists for turn-granularity
// callers (the S1 loop); streaming callers consume the iterator directly.
func CollectTurn(stream iter.Seq2[StreamEvent, error]) (GenStep, error) {
	return CollectTurnStreaming(stream, nil)
}

// CollectTurnStreaming drains a stream into a GenStep, invoking onDelta for
// each text delta as it arrives (S4.1). onDelta may be nil. Deltas are
// ephemeral — only the assembled GenStep.Message is durable (GenerationDiscarded
// contract): if the call errors mid-stream, whatever was emitted to
// onDelta is discarded by the caller.
func CollectTurnStreaming(stream iter.Seq2[StreamEvent, error], onDelta func(string)) (GenStep, error) {
	var (
		turn GenStep
		text string
	)
	for ev, err := range stream {
		if err != nil {
			return GenStep{}, err
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
