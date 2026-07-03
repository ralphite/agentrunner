// Package gemini adapts the normalized provider interface to the Gemini API
// via the official google.golang.org/genai SDK.
package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"

	"google.golang.org/genai"

	"github.com/ralphite/agentrunner/internal/provider"
)

// extrasSignatureKey stores Gemini thought signatures in Part/ToolCall Extras.
// The bytes must round-trip untouched: dropping them 400s multi-turn tool use.
const extrasSignatureKey = "gemini.thought_signature"

// Provider implements provider.Provider against the Gemini API.
type Provider struct {
	client *genai.Client
}

// New builds a Provider from GEMINI_API_KEY in the environment.
func New(ctx context.Context) (*Provider, error) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		return nil, errors.New("GEMINI_API_KEY not set")
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  key,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}
	return &Provider{client: client}, nil
}

// Capabilities reports optional features (stub until S4).
func (p *Provider) Capabilities() provider.Capabilities {
	return provider.Capabilities{}
}

// Complete streams one Gemini call, normalizing chunks to StreamEvents.
func (p *Provider) Complete(ctx context.Context, req provider.CompleteRequest) iter.Seq2[provider.StreamEvent, error] {
	return func(yield func(provider.StreamEvent, error) bool) {
		contents, err := toContents(req.Messages)
		if err != nil {
			yield(provider.StreamEvent{}, err)
			return
		}
		config, err := toConfig(req)
		if err != nil {
			yield(provider.StreamEvent{}, err)
			return
		}

		st := newStreamState(req.Turn)
		for resp, err := range p.client.Models.GenerateContentStream(ctx, req.Model, contents, config) {
			if err != nil {
				yield(provider.StreamEvent{}, fmt.Errorf("gemini: %w", err))
				return
			}
			for _, ev := range st.mapResponse(resp) {
				if !yield(ev, nil) {
					return
				}
			}
		}
		yield(provider.StreamEvent{Kind: provider.EventFinish, Finish: st.finish()}, nil)
	}
}

// streamState tracks per-call accumulation across stream chunks.
type streamState struct {
	turn         int
	callIndex    int
	sawToolCall  bool
	finishReason genai.FinishReason
}

func newStreamState(turn int) *streamState {
	return &streamState{turn: turn}
}

// mapResponse converts one stream chunk into zero or more normalized events.
func (st *streamState) mapResponse(resp *genai.GenerateContentResponse) []provider.StreamEvent {
	var events []provider.StreamEvent
	if len(resp.Candidates) > 0 {
		cand := resp.Candidates[0]
		if cand.FinishReason != "" {
			st.finishReason = cand.FinishReason
		}
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if ev, ok := st.mapPart(part); ok {
					events = append(events, ev)
				}
			}
		}
	}
	if resp.UsageMetadata != nil {
		events = append(events, provider.StreamEvent{
			Kind: provider.EventUsage,
			Usage: &provider.Usage{
				InputTokens: int(resp.UsageMetadata.PromptTokenCount),
				// Thinking tokens bill as output (真实计费口径).
				OutputTokens:    int(resp.UsageMetadata.CandidatesTokenCount + resp.UsageMetadata.ThoughtsTokenCount),
				CacheReadTokens: int(resp.UsageMetadata.CachedContentTokenCount),
			},
		})
	}
	return events
}

func (st *streamState) mapPart(part *genai.Part) (provider.StreamEvent, bool) {
	switch {
	case part.FunctionCall != nil:
		st.sawToolCall = true
		args, err := json.Marshal(part.FunctionCall.Args)
		if err != nil {
			args = json.RawMessage(`{}`)
		}
		tc := &provider.ToolCall{
			CallID: provider.CallID(st.turn, st.callIndex),
			Name:   part.FunctionCall.Name,
			Args:   args,
		}
		if len(part.ThoughtSignature) > 0 {
			sig, _ := json.Marshal(part.ThoughtSignature) // []byte → base64 JSON string
			tc.Extras = map[string]json.RawMessage{extrasSignatureKey: sig}
		}
		st.callIndex++
		return provider.StreamEvent{Kind: provider.EventToolCall, ToolCall: tc}, true
	case part.Text != "":
		return provider.StreamEvent{Kind: provider.EventTextDelta, TextDelta: part.Text}, true
	default:
		return provider.StreamEvent{}, false
	}
}

func (st *streamState) finish() provider.FinishReason {
	switch st.finishReason {
	case genai.FinishReasonStop, "":
		if st.sawToolCall {
			return provider.FinishToolUse
		}
		return provider.FinishEndTurn
	case genai.FinishReasonMaxTokens:
		return provider.FinishMaxTokens
	default:
		return provider.FinishOther
	}
}

// toConfig builds the request config: system instruction, token cap, tools.
func toConfig(req provider.CompleteRequest) (*genai.GenerateContentConfig, error) {
	config := &genai.GenerateContentConfig{
		MaxOutputTokens: int32(req.MaxTokens),
	}
	if req.System != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: req.System}},
		}
	}
	if len(req.Tools) > 0 {
		tool := &genai.Tool{}
		for _, td := range req.Tools {
			var schema any
			if len(td.InputSchema) > 0 {
				if err := json.Unmarshal(td.InputSchema, &schema); err != nil {
					return nil, fmt.Errorf("gemini: tool %s schema: %w", td.Name, err)
				}
			}
			tool.FunctionDeclarations = append(tool.FunctionDeclarations, &genai.FunctionDeclaration{
				Name:                 td.Name,
				Description:          td.Description,
				ParametersJsonSchema: schema,
			})
		}
		config.Tools = []*genai.Tool{tool}
	}
	return config, nil
}

// toContents converts normalized messages to Gemini contents. Tool results
// must appear in the same count and order as the functionCalls of the
// preceding model turn — the loop/assembly guarantees ordering; this function
// converts faithfully.
func toContents(msgs []provider.Message) ([]*genai.Content, error) {
	var contents []*genai.Content
	for _, msg := range msgs {
		content, err := toContent(msg)
		if err != nil {
			return nil, err
		}
		contents = append(contents, content)
	}
	return contents, nil
}

func toContent(msg provider.Message) (*genai.Content, error) {
	content := &genai.Content{}
	switch msg.Role {
	case provider.RoleUser, provider.RoleTool:
		content.Role = genai.RoleUser
	case provider.RoleAssistant:
		content.Role = genai.RoleModel
	default:
		return nil, fmt.Errorf("gemini: unsupported message role %q", msg.Role)
	}

	for _, p := range msg.Parts {
		part, err := toPart(p)
		if err != nil {
			return nil, err
		}
		content.Parts = append(content.Parts, part)
	}
	return content, nil
}

func toPart(p provider.Part) (*genai.Part, error) {
	switch p.Kind {
	case provider.PartText:
		return &genai.Part{Text: p.Text}, nil

	case provider.PartToolCall:
		var args map[string]any
		if len(p.Args) > 0 {
			if err := json.Unmarshal(p.Args, &args); err != nil {
				return nil, fmt.Errorf("gemini: tool call %s args: %w", p.CallID, err)
			}
		}
		part := &genai.Part{FunctionCall: &genai.FunctionCall{Name: p.ToolName, Args: args}}
		if sig, ok := p.Extras[extrasSignatureKey]; ok {
			var raw []byte
			if err := json.Unmarshal(sig, &raw); err != nil {
				return nil, fmt.Errorf("gemini: tool call %s thought signature: %w", p.CallID, err)
			}
			part.ThoughtSignature = raw
		}
		return part, nil

	case provider.PartToolResult:
		return &genai.Part{FunctionResponse: &genai.FunctionResponse{
			Name:     p.ToolName,
			Response: toResponseMap(p),
		}}, nil

	default:
		return nil, fmt.Errorf("gemini: unsupported part kind %q", p.Kind)
	}
}

// toResponseMap renders a tool result into Gemini's response map. Object
// results pass through; non-object results wrap as {"output": …}; errors
// wrap as {"error": …} — Gemini has no is_error flag, this is our error
// payload convention (决策 #9).
func toResponseMap(p provider.Part) map[string]any {
	var value any
	if len(p.Result) > 0 {
		if err := json.Unmarshal(p.Result, &value); err != nil {
			value = string(p.Result)
		}
	}
	if p.IsError {
		return map[string]any{"error": value}
	}
	if obj, ok := value.(map[string]any); ok {
		return obj
	}
	return map[string]any{"output": value}
}
