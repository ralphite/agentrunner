package provider_test

import (
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
)

// TestContextLimitUnknownStaysZero is the load-bearing half of G57: the
// projection exists to stop a "72k / 258k" indicator from inventing its
// denominator, so an unrecognized model MUST report 0 rather than guess from
// a sibling family. If this ever starts returning a fallback, the honest
// "used, limit unknown" rendering silently becomes a confident lie.
func TestContextLimitUnknownStaysZero(t *testing.T) {
	unknown := []struct{ provider, model string }{
		{"gemini", "gemini-9.9-superflash"}, // plausible-looking future model
		{"anthropic", "sonnet-5"},           // right vendor, wrong id shape
		{"openai", "gpt-5"},                 // provider we do not model at all
		{"scripted", "m"},                   // the test double
		{"", ""},
	}
	for _, u := range unknown {
		if got := provider.ContextLimitTokens(u.provider, u.model); got != 0 {
			t.Errorf("ContextLimitTokens(%q, %q) = %d, want 0 (unknown must stay unknown)",
				u.provider, u.model, got)
		}
	}
}

func TestContextLimitKnownFamilies(t *testing.T) {
	rows := []struct {
		provider, model string
		want            int
	}{
		{"gemini", "gemini-flash-latest", 1_048_576}, // this repo's default model
		{"gemini", "gemini-2.5-pro", 1_048_576},
		{"gemini", "gemini-2.5-flash", 1_048_576},
		{"gemini", "gemini-1.5-pro-002", 2_097_152},
		{"gemini", "gemini-1.5-flash", 1_048_576},
		{"anthropic", "claude-sonnet-5", 200_000},
		{"anthropic", "claude-haiku-4-5-20251001", 200_000},
	}
	for _, r := range rows {
		if got := provider.ContextLimitTokens(r.provider, r.model); got != r.want {
			t.Errorf("ContextLimitTokens(%q, %q) = %d, want %d", r.provider, r.model, got, r.want)
		}
	}
}

// TestEnvelopeCarriesContextLimit pins that the limit rides the envelope
// frozen at SessionStarted — that is what makes it recoverable by a read-only
// observer with no provider credentials at hand.
func TestEnvelopeCarriesContextLimit(t *testing.T) {
	env := provider.Envelope("gemini", "gemini-2.5-pro", provider.Capabilities{})
	if env.ContextLimitTokens != 1_048_576 {
		t.Errorf("envelope ContextLimitTokens = %d, want 1048576", env.ContextLimitTokens)
	}
	unknown := provider.Envelope("scripted", "m", provider.Capabilities{})
	if unknown.ContextLimitTokens != 0 {
		t.Errorf("scripted envelope ContextLimitTokens = %d, want 0", unknown.ContextLimitTokens)
	}
}
