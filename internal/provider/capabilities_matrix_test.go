package provider_test

import (
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/anthropic"
	"github.com/ralphite/agentrunner/internal/provider/gemini"
)

// TestCapabilitiesMatrix pins each provider's declared optional features
// (S4.7). The abstraction's value is that the loop reads these flags rather
// than branching on provider identity — so every provider must answer the
// same three questions. A zero-value Provider is enough: Capabilities()
// reports static support, no client needed.
func TestCapabilitiesMatrix(t *testing.T) {
	rows := []struct {
		name string
		caps provider.Capabilities
	}{
		{"gemini", (&gemini.Provider{}).Capabilities()},
		{"anthropic", (&anthropic.Provider{}).Capabilities()},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			// Both current providers support all three; the test exists to
			// force a conscious declaration when a third provider (or a
			// downgraded model) reports false.
			if !r.caps.Thinking {
				t.Errorf("%s: Thinking not declared", r.name)
			}
			if !r.caps.PromptCaching {
				t.Errorf("%s: PromptCaching not declared", r.name)
			}
			if !r.caps.ParallelTools {
				t.Errorf("%s: ParallelTools not declared", r.name)
			}
			if !r.caps.Images || !r.caps.Files {
				t.Errorf("%s: input modalities not declared: %+v", r.name, r.caps)
			}
			env := provider.Envelope(r.name, "model", r.caps)
			if env.SchemaVersion != 1 || !env.Streaming || !env.ToolCalls || len(env.InputModalities) != 3 {
				t.Errorf("%s: capability envelope = %+v", r.name, env)
			}
		})
	}
}
