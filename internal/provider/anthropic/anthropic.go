// Package anthropic adapts the normalized provider interface to the
// Anthropic Messages API via the official anthropic-sdk-go (S4.7). It is the
// second provider implementation; writing it against the SAME normalized
// Message/Part/Capabilities types is the test that the abstraction (built in
// S1 for the final shape) does not leak provider specifics.
package anthropic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/provider"
)

// extrasThinkingKey stores an Anthropic thinking block (its text + signature)
// on the assistant tool_call part, so a later turn can replay the block
// verbatim before the tool_use — Anthropic validates the signature against
// the thinking content, so it must round-trip byte-identically.
const extrasThinkingKey = "anthropic.thinking"

// minThinkingBudget is Anthropic's floor for an enabled thinking budget.
const minThinkingBudget = 1024

// Provider implements provider.Provider against the Anthropic Messages API.
type Provider struct {
	client sdk.Client
}

// New builds a Provider from ANTHROPIC_API_KEY in the environment.
func New(_ context.Context) (*Provider, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, errors.New("ANTHROPIC_API_KEY not set")
	}
	return &Provider{client: sdk.NewClient(option.WithAPIKey(key))}, nil
}

// Capabilities reports optional features (S4.7). Anthropic supports extended
// thinking, prompt caching (cache_control breakpoints), and parallel tool
// use.
func (p *Provider) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		Thinking:      true,
		PromptCaching: true,
		ParallelTools: true,
	}
}

// Complete streams one Anthropic call, normalizing to StreamEvents. Text
// deltas surface live; tool calls, usage, and the finish reason are derived
// from the accumulated message once the stream closes.
func (p *Provider) Complete(ctx context.Context, req provider.CompleteRequest) iter.Seq2[provider.StreamEvent, error] {
	return func(yield func(provider.StreamEvent, error) bool) {
		params, err := toParams(req)
		if err != nil {
			yield(provider.StreamEvent{}, err)
			return
		}

		stream := p.client.Messages.NewStreaming(ctx, params)
		var acc sdk.Message
		for stream.Next() {
			ev := stream.Current()
			if err := acc.Accumulate(ev); err != nil {
				yield(provider.StreamEvent{}, errs.Wrap(errs.Internal, err, "anthropic: accumulate"))
				return
			}
			// Only response text streams live; thinking deltas are internal
			// reasoning captured from the accumulated message, not surfaced.
			if ev.Type == "content_block_delta" && ev.Delta.Text != "" {
				if !yield(provider.StreamEvent{Kind: provider.EventTextDelta, TextDelta: ev.Delta.Text}, nil) {
					return
				}
			}
		}
		if err := stream.Err(); err != nil {
			yield(provider.StreamEvent{}, classify(err))
			return
		}

		if !emitAccumulated(acc, yield) {
			return
		}
		yield(provider.StreamEvent{Kind: provider.EventFinish, Finish: mapFinish(acc.StopReason)}, nil)
	}
}

// emitAccumulated yields tool calls (with any preceding thinking block's
// signature attached) and the usage event. Returns false if the consumer
// stopped early.
func emitAccumulated(acc sdk.Message, yield func(provider.StreamEvent, error) bool) bool {
	var pendingThinking json.RawMessage
	for _, block := range acc.Content {
		switch block.Type {
		case "thinking":
			// Hold the thinking block for the next tool_use so it can be
			// replayed before it on the following turn.
			pendingThinking, _ = json.Marshal(map[string]string{
				"thinking": block.Thinking, "signature": block.Signature,
			})
		case "tool_use":
			tc := &provider.ToolCall{CallID: block.ID, Name: block.Name, Args: block.Input}
			if len(pendingThinking) > 0 {
				tc.Extras = map[string]json.RawMessage{extrasThinkingKey: pendingThinking}
				pendingThinking = nil
			}
			if !yield(provider.StreamEvent{Kind: provider.EventToolCall, ToolCall: tc}, nil) {
				return false
			}
		}
	}
	// Normalize InputTokens to the TOTAL input including the cached prefix,
	// matching Gemini's convention (PromptTokenCount includes cached). The
	// Anthropic API instead reports input_tokens EXCLUDING cache_read and
	// cache_creation, so we add them back — otherwise Usage.Billed()
	// (input+output−cache_read) would double-discount the cache and a
	// warm-cache run would charge ~0 against the budget (S4 review P1).
	return yield(provider.StreamEvent{Kind: provider.EventUsage, Usage: &provider.Usage{
		InputTokens: int(acc.Usage.InputTokens + acc.Usage.CacheReadInputTokens +
			acc.Usage.CacheCreationInputTokens),
		OutputTokens:     int(acc.Usage.OutputTokens),
		CacheReadTokens:  int(acc.Usage.CacheReadInputTokens),
		CacheWriteTokens: int(acc.Usage.CacheCreationInputTokens),
	}}, nil)
}

// classify maps SDK errors onto the 2.8 taxonomy (S3.9 online-side, S4.7).
func classify(err error) error {
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		return errs.Wrap(errs.FromHTTPStatus(apiErr.StatusCode), err, "anthropic")
	}
	if class := errs.ClassOf(err); class == errs.Canceled || class == errs.Timeout {
		return errs.Wrap(class, err, "anthropic")
	}
	return errs.Wrap(errs.ProviderServer, err, "anthropic") // transport-level: retry
}

// mapFinish normalizes Anthropic stop reasons. Refusal is a safety block
// (surfaced as a user-visible error by the loop, S4.6).
func mapFinish(reason sdk.StopReason) provider.FinishReason {
	switch reason {
	case sdk.StopReasonEndTurn, sdk.StopReasonStopSequence:
		return provider.FinishEndTurn
	case sdk.StopReasonToolUse:
		return provider.FinishToolUse
	case sdk.StopReasonMaxTokens:
		return provider.FinishMaxTokens
	case sdk.StopReasonRefusal:
		return provider.FinishBlocked
	default:
		// pause_turn (server-side/long-running tools want the turn resumed)
		// and any unknown reason map to Other, which the loop surfaces as a
		// user-visible error — better than silently ending the run as
		// "completed" and truncating pending work (S4 review P2).
		return provider.FinishOther
	}
}

// toParams builds the Messages request: system (last block cache-marked),
// messages, tools, thinking config, token cap.
func toParams(req provider.CompleteRequest) (sdk.MessageNewParams, error) {
	msgs, err := toMessages(req.Messages)
	if err != nil {
		return sdk.MessageNewParams{}, err
	}
	params := sdk.MessageNewParams{
		Model:     sdk.Model(req.Model),
		MaxTokens: int64(req.MaxTokens),
		Messages:  msgs,
	}
	if req.System != "" {
		sys := sdk.TextBlockParam{Text: req.System}
		// Cache the frozen prefix (S4.4c prefix stability makes this pay off).
		sys.CacheControl = sdk.NewCacheControlEphemeralParam()
		params.System = []sdk.TextBlockParam{sys}
	}
	if tools := toTools(req.Tools); len(tools) > 0 {
		params.Tools = tools
	}
	if req.Thinking.Enabled {
		budget := int64(req.Thinking.BudgetTokens)
		if budget < minThinkingBudget {
			budget = minThinkingBudget
		}
		params.Thinking = sdk.ThinkingConfigParamOfEnabled(budget)
	}
	return params, nil
}

func toTools(defs []provider.ToolDef) []sdk.ToolUnionParam {
	out := make([]sdk.ToolUnionParam, 0, len(defs))
	for _, td := range defs {
		schema := sdk.ToolInputSchemaParam{}
		if len(td.InputSchema) > 0 {
			var parsed struct {
				Properties any      `json:"properties"`
				Required   []string `json:"required"`
			}
			if err := json.Unmarshal(td.InputSchema, &parsed); err == nil {
				schema.Properties = parsed.Properties
				schema.Required = parsed.Required
			}
		}
		tool := sdk.ToolUnionParamOfTool(schema, td.Name)
		if tool.OfTool != nil {
			tool.OfTool.Description = sdk.String(td.Description)
		}
		out = append(out, tool)
	}
	return out
}

// toMessages converts normalized messages to Anthropic MessageParams. Tool
// results live in USER-role messages (Anthropic's shape); an assistant turn
// replays its thinking block (if captured) before its tool_use blocks so the
// signature validates.
func toMessages(msgs []provider.Message) ([]sdk.MessageParam, error) {
	out := make([]sdk.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case provider.RoleUser:
			blocks, err := userBlocks(m)
			if err != nil {
				return nil, err
			}
			out = append(out, sdk.NewUserMessage(blocks...))
		case provider.RoleTool:
			blocks, err := toolResultBlocks(m)
			if err != nil {
				return nil, err
			}
			out = append(out, sdk.NewUserMessage(blocks...))
		case provider.RoleAssistant:
			blocks, err := assistantBlocks(m)
			if err != nil {
				return nil, err
			}
			out = append(out, sdk.NewAssistantMessage(blocks...))
		default:
			return nil, fmt.Errorf("anthropic: unsupported role %q", m.Role)
		}
	}
	return out, nil
}

func userBlocks(m provider.Message) ([]sdk.ContentBlockParamUnion, error) {
	var blocks []sdk.ContentBlockParamUnion
	for _, p := range m.Parts {
		switch p.Kind {
		case provider.PartText:
			blocks = append(blocks, sdk.NewTextBlock(p.Text))
		case provider.PartImage, provider.PartFile:
			// v2 M4.2: assembly already inflated the bytes from the CAS.
			if len(p.Data) == 0 {
				return nil, fmt.Errorf("anthropic: %s part %q has no bytes (not inflated)", p.Kind, p.Ref)
			}
			blocks = append(blocks, sdk.NewImageBlockBase64(p.MediaType,
				base64.StdEncoding.EncodeToString(p.Data)))
		default:
			return nil, fmt.Errorf("anthropic: user message part kind %q unsupported", p.Kind)
		}
	}
	return blocks, nil
}

func toolResultBlocks(m provider.Message) ([]sdk.ContentBlockParamUnion, error) {
	var blocks []sdk.ContentBlockParamUnion
	for _, p := range m.Parts {
		if p.Kind != provider.PartToolResult {
			return nil, fmt.Errorf("anthropic: tool message part kind %q unsupported", p.Kind)
		}
		blocks = append(blocks, sdk.NewToolResultBlock(p.CallID, string(p.Result), p.IsError))
	}
	return blocks, nil
}

func assistantBlocks(m provider.Message) ([]sdk.ContentBlockParamUnion, error) {
	// Thinking first (Anthropic requires it before tool_use), then text, then
	// tool_use — the fixed order the API validates.
	var thinking, text, tools []sdk.ContentBlockParamUnion
	for _, p := range m.Parts {
		switch p.Kind {
		case provider.PartText:
			text = append(text, sdk.NewTextBlock(p.Text))
		case provider.PartToolCall:
			if raw, ok := p.Extras[extrasThinkingKey]; ok && len(thinking) == 0 {
				var t struct{ Thinking, Signature string }
				if err := json.Unmarshal(raw, &t); err != nil {
					return nil, fmt.Errorf("anthropic: tool call %s thinking: %w", p.CallID, err)
				}
				thinking = append(thinking, sdk.NewThinkingBlock(t.Signature, t.Thinking))
			}
			var input any
			if len(p.Args) > 0 {
				if err := json.Unmarshal(p.Args, &input); err != nil {
					return nil, fmt.Errorf("anthropic: tool call %s args: %w", p.CallID, err)
				}
			}
			tools = append(tools, sdk.NewToolUseBlock(p.CallID, input, p.ToolName))
		default:
			return nil, fmt.Errorf("anthropic: assistant part kind %q unsupported", p.Kind)
		}
	}
	return append(append(thinking, text...), tools...), nil
}
