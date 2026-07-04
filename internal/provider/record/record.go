// Package record wraps any Provider and captures its traffic as a scripted
// fixture (PLAN 1.3a). The record-fixture CLI wires this up in 1.9; the
// middleware itself is provider-agnostic and unit-testable now.
package record

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
)

// redactEnvSuffixes marks env vars whose values must never reach a fixture.
var redactEnvSuffixes = []string{"_API_KEY", "_TOKEN", "_SECRET"}

// Recorder is a pass-through Provider that accumulates fixture steps.
type Recorder struct {
	inner   provider.Provider
	redact  map[string]string // secret value → replacement
	fixture scripted.Fixture
}

// New wraps inner, harvesting credential values from the environment for
// redaction (S1 执行包: fixtures pass through redaction).
func New(inner provider.Provider) *Recorder {
	redact := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		for _, suffix := range redactEnvSuffixes {
			if strings.HasSuffix(k, suffix) && v != "" {
				redact[v] = "[REDACTED:" + k + "]"
			}
		}
	}
	return &Recorder{inner: inner, redact: redact}
}

// Capabilities delegates to the wrapped provider.
func (r *Recorder) Capabilities() provider.Capabilities {
	return r.inner.Capabilities()
}

// Complete passes the call through while capturing a fixture step.
// Consecutive text deltas are ACCUMULATED and redacted as one string, so a
// credential split across two deltas (which no single-delta redaction would
// catch) is still scrubbed before it reaches the fixture (S2 review item).
func (r *Recorder) Complete(ctx context.Context, req provider.CompleteRequest) iter.Seq2[provider.StreamEvent, error] {
	return func(yield func(provider.StreamEvent, error) bool) {
		step := scripted.Step{Expect: r.deriveExpect(req)}
		var textBuf strings.Builder
		flushText := func() {
			if textBuf.Len() > 0 {
				step.Respond = append(step.Respond,
					scripted.Event{Text: r.redactString(textBuf.String())})
				textBuf.Reset()
			}
		}
		for ev, err := range r.inner.Complete(ctx, req) {
			if err != nil {
				yield(provider.StreamEvent{}, err)
				return
			}
			if ev.Kind == provider.EventTextDelta {
				textBuf.WriteString(ev.TextDelta)
			} else {
				flushText()
				if rec, ok := r.toEvent(ev); ok {
					step.Respond = append(step.Respond, rec)
				}
			}
			if !yield(ev, nil) {
				return
			}
		}
		flushText()
		r.fixture.Steps = append(r.fixture.Steps, step)
	}
}

// WriteFixture serializes the captured session to a YAML fixture file.
func (r *Recorder) WriteFixture(path string) error {
	data, err := yaml.Marshal(r.fixture)
	if err != nil {
		return fmt.Errorf("record: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("record: %w", err)
	}
	return nil
}

// deriveExpect auto-fills drift assertions: offered tool names plus a snippet
// of the last message.
func (r *Recorder) deriveExpect(req provider.CompleteRequest) scripted.Expect {
	e := scripted.Expect{}
	for _, td := range req.Tools {
		e.ToolsInclude = append(e.ToolsInclude, td.Name)
	}
	if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1]
		for _, p := range last.Parts {
			if p.Kind == provider.PartText && p.Text != "" {
				snippet := r.redactString(p.Text)
				if len(snippet) > 60 {
					snippet = snippet[:60]
				}
				e.LastMessageContains = snippet
				break
			}
		}
	}
	return e
}

func (r *Recorder) toEvent(ev provider.StreamEvent) (scripted.Event, bool) {
	switch ev.Kind {
	case provider.EventTextDelta:
		// Text is accumulated and flushed in Complete (cross-delta
		// redaction); toEvent is never called for text deltas.
		return scripted.Event{}, false
	case provider.EventToolCall:
		var args map[string]any
		if len(ev.ToolCall.Args) > 0 {
			_ = json.Unmarshal([]byte(r.redactString(string(ev.ToolCall.Args))), &args)
		}
		return scripted.Event{ToolCall: &scripted.ToolCallEvent{
			CallID: ev.ToolCall.CallID, Name: ev.ToolCall.Name, Args: args,
			Extras: ev.ToolCall.Extras,
		}}, true
	case provider.EventUsage:
		return scripted.Event{Usage: &scripted.UsageEvent{
			InputTokens: ev.Usage.InputTokens, OutputTokens: ev.Usage.OutputTokens,
			CacheReadTokens: ev.Usage.CacheReadTokens,
		}}, true
	case provider.EventFinish:
		return scripted.Event{Finish: string(ev.Finish)}, true
	default:
		return scripted.Event{}, false
	}
}

func (r *Recorder) redactString(s string) string {
	for secret, replacement := range r.redact {
		s = strings.ReplaceAll(s, secret, replacement)
	}
	return s
}
