// Package scripted implements the replay test provider (PLAN §0 测试基座):
// it serves recorded/hand-authored fixtures by sequence, with per-step
// request assertions that fail loudly on drift.
package scripted

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ralphite/agentrunner/internal/provider"
)

// Fixture is one session's scripted conversation.
type Fixture struct {
	Steps []Step `yaml:"steps"`
}

// Step pairs optional request assertions with the events to respond.
type Step struct {
	Expect  Expect  `yaml:"expect,omitempty"`
	Respond []Event `yaml:"respond"`
}

// Expect asserts on selected request fields (S1 执行包 matching contract).
type Expect struct {
	ToolsInclude        []string `yaml:"tools_include,omitempty"`
	LastMessageContains string   `yaml:"last_message_contains,omitempty"`
}

// Event is the YAML-friendly form of one StreamEvent.
type Event struct {
	Text     string         `yaml:"text,omitempty"`
	ToolCall *ToolCallEvent `yaml:"tool_call,omitempty"`
	Usage    *UsageEvent    `yaml:"usage,omitempty"`
	Finish   string         `yaml:"finish,omitempty"`
}

// ToolCallEvent scripts one tool call. CallID is optional — when empty the
// provider mints the deterministic harness id from (turn, index).
type ToolCallEvent struct {
	CallID string         `yaml:"call_id,omitempty"`
	Name   string         `yaml:"name"`
	Args   map[string]any `yaml:"args,omitempty"`
}

// UsageEvent scripts token accounting.
type UsageEvent struct {
	InputTokens  int `yaml:"input_tokens"`
	OutputTokens int `yaml:"output_tokens"`
}

// Provider serves a Fixture step by step.
type Provider struct {
	fixture Fixture
	source  string
	next    int
}

// Load reads a fixture file into a Provider.
func Load(path string) (*Provider, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("fixture %s: %w", path, err)
	}
	var f Fixture
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("fixture %s: %v", path, err)
	}
	return &Provider{fixture: f, source: path}, nil
}

// New builds a Provider from an in-memory fixture (hand-authored unit cases).
func New(f Fixture) *Provider {
	return &Provider{fixture: f, source: "<inline>"}
}

// Capabilities reports optional features (none).
func (p *Provider) Capabilities() provider.Capabilities {
	return provider.Capabilities{}
}

// Complete serves the next scripted step, asserting expectations first.
func (p *Provider) Complete(_ context.Context, req provider.CompleteRequest) iter.Seq2[provider.StreamEvent, error] {
	return func(yield func(provider.StreamEvent, error) bool) {
		if p.next >= len(p.fixture.Steps) {
			yield(provider.StreamEvent{}, fmt.Errorf(
				"scripted %s: fixture exhausted at request %d (have %d steps)",
				p.source, p.next+1, len(p.fixture.Steps)))
			return
		}
		step := p.fixture.Steps[p.next]
		stepNo := p.next + 1
		p.next++

		if err := step.Expect.check(req, p.source, stepNo); err != nil {
			yield(provider.StreamEvent{}, err)
			return
		}

		callIndex := 0
		for _, ev := range step.Respond {
			out, err := ev.toStreamEvent(req.Turn, &callIndex)
			if err != nil {
				yield(provider.StreamEvent{}, fmt.Errorf("scripted %s step %d: %w", p.source, stepNo, err))
				return
			}
			if !yield(out, nil) {
				return
			}
		}
	}
}

// Done errors unless every scripted step was consumed (test-teardown helper).
func (p *Provider) Done() error {
	if p.next != len(p.fixture.Steps) {
		return fmt.Errorf("scripted %s: %d of %d steps consumed",
			p.source, p.next, len(p.fixture.Steps))
	}
	return nil
}

func (e Expect) check(req provider.CompleteRequest, source string, stepNo int) error {
	for _, want := range e.ToolsInclude {
		found := slices.ContainsFunc(req.Tools, func(td provider.ToolDef) bool {
			return td.Name == want
		})
		if !found {
			return fmt.Errorf("scripted %s step %d: request drift: tool %q not offered (got %v)",
				source, stepNo, want, toolNames(req.Tools))
		}
	}
	if e.LastMessageContains != "" {
		text := lastMessageText(req.Messages)
		if !strings.Contains(text, e.LastMessageContains) {
			return fmt.Errorf("scripted %s step %d: request drift: last message %q does not contain %q",
				source, stepNo, truncate(text, 120), e.LastMessageContains)
		}
	}
	return nil
}

func (e Event) toStreamEvent(turn int, callIndex *int) (provider.StreamEvent, error) {
	switch {
	case e.Text != "":
		return provider.StreamEvent{Kind: provider.EventTextDelta, TextDelta: e.Text}, nil
	case e.ToolCall != nil:
		args, err := json.Marshal(e.ToolCall.Args)
		if err != nil {
			return provider.StreamEvent{}, fmt.Errorf("tool call %s args: %w", e.ToolCall.Name, err)
		}
		id := e.ToolCall.CallID
		if id == "" {
			id = provider.CallID(turn, *callIndex)
		}
		*callIndex++
		return provider.StreamEvent{Kind: provider.EventToolCall, ToolCall: &provider.ToolCall{
			CallID: id, Name: e.ToolCall.Name, Args: args,
		}}, nil
	case e.Usage != nil:
		return provider.StreamEvent{Kind: provider.EventUsage, Usage: &provider.Usage{
			InputTokens: e.Usage.InputTokens, OutputTokens: e.Usage.OutputTokens,
		}}, nil
	case e.Finish != "":
		return provider.StreamEvent{Kind: provider.EventFinish, Finish: provider.FinishReason(e.Finish)}, nil
	default:
		return provider.StreamEvent{}, fmt.Errorf("empty scripted event")
	}
}

func toolNames(defs []provider.ToolDef) []string {
	names := make([]string, len(defs))
	for i, td := range defs {
		names[i] = td.Name
	}
	return names
}

func lastMessageText(msgs []provider.Message) string {
	if len(msgs) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, p := range msgs[len(msgs)-1].Parts {
		if p.Kind == provider.PartText {
			sb.WriteString(p.Text)
		}
		if p.Kind == provider.PartToolResult {
			sb.Write(p.Result)
		}
	}
	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
