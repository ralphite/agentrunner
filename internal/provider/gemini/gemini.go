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
	"strings"

	"google.golang.org/genai"

	"github.com/ralphite/agentrunner/internal/errs"
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
		return nil, errors.New("GEMINI_API_KEY not set — export it, or put GEMINI_API_KEY=<key> in a .env at the workspace root")
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

// Capabilities reports optional features (S4.7). Gemini supports thinking
// (thought tokens), context caching, and parallel function calls.
func (p *Provider) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		Thinking:         true,
		PromptCaching:    true,
		ParallelTools:    true,
		Images:           true,
		Files:            true,
		StructuredOutput: true,
	}
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

		st := newStreamState(req.GenStep)
		for resp, err := range p.client.Models.GenerateContentStream(ctx, req.Model, contents, config) {
			if err != nil {
				yield(provider.StreamEvent{}, classify(err))
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

// classify maps SDK errors onto the 2.8 taxonomy. Downstream retry and
// rendering consume only the class.
func classify(err error) error {
	var ae genai.APIError
	if errors.As(err, &ae) {
		if ae.Code == 404 {
			// A retired or misspelled model id 404s. The SDK names the id but
			// not a fix — point at a stable alias so the user isn't left
			// guessing which ids still exist (黑盒 R2 minor: gemini-2.5-flash
			// was retired; gemini-flash-latest tracks the current one).
			return errs.Wrap(errs.FromHTTPStatus(ae.Code), err,
				"gemini: model not found — use a current id such as `gemini-flash-latest` or `gemini-2.5-pro`")
		}
		return errs.Wrap(errs.FromHTTPStatus(ae.Code), err, "gemini")
	}
	if class := errs.ClassOf(err); class == errs.Canceled || class == errs.Timeout {
		return errs.Wrap(class, err, "gemini")
	}
	return errs.Wrap(errs.ProviderServer, err, "gemini") // transport-level: worth a retry
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

// Gemini thinking sizing constants. Thought tokens are drawn from
// MaxOutputTokens, so an enabled thinking request must leave answer room.
const (
	// geminiMaxThinkingBudget is the largest thought budget any 2.5 Flash
	// model accepts (Pro allows more, but this ceiling is safe everywhere).
	geminiMaxThinkingBudget = 24576
	// geminiDefaultThinkingBudget is used when thinking is enabled without an
	// explicit budget — mirrors Gemini's own dynamic-thinking cap (8192)
	// instead of leaving the budget unbounded.
	geminiDefaultThinkingBudget = 8192
	// geminiMinAnswerRoom is the floor of output tokens always reserved for the
	// answer + tool calls, so thinking can never consume the whole cap.
	geminiMinAnswerRoom = 1024
)

// resolveThinkingBudget picks the thinkingBudget to send for an ENABLED
// thinking request, clamped so the answer always keeps room within maxTokens.
// requested is the caller's spec budget (0 ⇒ unspecified). It returns
// (budget, ok); ok=false means the cap is too small to afford any thinking, so
// the caller disables it (budget 0 ⇒ full cap to the answer) rather than
// letting thoughts starve the answer to an empty message.
func resolveThinkingBudget(maxTokens, requested int) (int32, bool) {
	// Reserve at least a quarter of the cap (floor geminiMinAnswerRoom) for the
	// answer; thinking may use the rest.
	reserve := maxTokens / 4
	if reserve < geminiMinAnswerRoom {
		reserve = geminiMinAnswerRoom
	}
	ceiling := maxTokens - reserve
	if ceiling <= 0 {
		return 0, false // cap too small for any thinking
	}
	budget := requested
	if budget <= 0 {
		budget = geminiDefaultThinkingBudget
	}
	if budget > geminiMaxThinkingBudget {
		budget = geminiMaxThinkingBudget
	}
	if budget > ceiling {
		budget = ceiling
	}
	return int32(budget), true
}

// toConfig builds the request config: system instruction, token cap, tools.
func toConfig(req provider.CompleteRequest) (*genai.GenerateContentConfig, error) {
	config := &genai.GenerateContentConfig{
		MaxOutputTokens: int32(req.MaxTokens),
	}
	// gemini-flash-latest (and Pro) now REJECT thinkingBudget:0 with
	// INVALID_ARGUMENT (2026-07-21 — the "latest" alias moved to a model that
	// thinks by default and cannot be fully disabled). So we NEVER send a zero
	// budget. A request that doesn't explicitly enable thinking still gets a
	// positive, clamped budget (default toward the dynamic cap, reserved against
	// the answer — the 会话死亡 empty-message defense stays intact via
	// resolveThinkingBudget); thought SUMMARIES surface only when the caller set
	// Thinking.Enabled. When the cap is too small for any clamped budget, we
	// leave the model to its own minimum floor rather than an invalid explicit 0.
	requested := req.Thinking.BudgetTokens
	if requested <= 0 {
		requested = geminiDefaultThinkingBudget
	}
	if budget, ok := resolveThinkingBudget(req.MaxTokens, requested); ok {
		config.ThinkingConfig = &genai.ThinkingConfig{IncludeThoughts: req.Thinking.Enabled, ThinkingBudget: &budget}
	} else {
		config.ThinkingConfig = &genai.ThinkingConfig{IncludeThoughts: req.Thinking.Enabled}
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
	// Native structured output (INC-35): constrain the completion to a JSON
	// schema — but ONLY on a tool-less turn. Gemini's JSON mode forces the
	// whole output to be one JSON value, which is mutually exclusive with
	// function calling, so a turn that offers tools must ignore the schema
	// (the CLI validate/retry path stays the fallback for tool-using runs).
	if len(req.ResponseSchema) > 0 && len(req.Tools) == 0 {
		var schema any
		if err := json.Unmarshal(req.ResponseSchema, &schema); err != nil {
			return nil, fmt.Errorf("gemini: response schema: %w", err)
		}
		config.ResponseMIMEType = "application/json"
		config.ResponseJsonSchema = schema
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
	if len(msg.Parts) == 0 {
		return nil, fmt.Errorf("gemini: message with role %q has no parts", msg.Role)
	}
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

	case provider.PartImage, provider.PartFile, provider.PartAudio:
		// v2 M4.2: for loop-assembled image/file parts, assembly already
		// inflated the bytes from the CAS; a ref without bytes here means the
		// inflate step was skipped. PartAudio (INC-56 `ar dictate`) is built
		// directly by the dictate helper with Data already populated — same
		// inline_data encoding, no CAS round-trip.
		if len(p.Data) == 0 {
			return nil, fmt.Errorf("gemini: %s part %q has no bytes (not inflated)", p.Kind, p.Ref)
		}
		// Text attachments (folded long pastes, M4.3) render as text — the
		// most portable mapping; binary media (images, audio) rides inline_data.
		if p.Kind == provider.PartFile && strings.HasPrefix(p.MediaType, "text/") {
			return &genai.Part{Text: "[attached file " + p.Ref + "]\n" + string(p.Data)}, nil
		}
		return &genai.Part{InlineData: &genai.Blob{MIMEType: p.MediaType, Data: p.Data}}, nil

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
