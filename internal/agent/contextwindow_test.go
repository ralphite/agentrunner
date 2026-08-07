package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
)

func cwSpec(compactAt, microAt int) *AgentSpec {
	return &AgentSpec{Model: ModelSpec{
		Provider: "scripted", ID: "m",
		MaxTokens:            100, // the OUTPUT cap — must never leak into the window
		CompactAtTokens:      compactAt,
		MicrocompactAtTokens: microAt,
	}}
}

// G57's core contract: the projection measures the ASSEMBLED view, so a
// microcompact that elides an old read result SHRINKS it. Cumulative billed
// usage can only grow, which is exactly why it cannot stand in for this
// number — this test pins the difference rather than the arithmetic.
func TestContextWindowTracksAssembledViewNotCumulativeUsage(t *testing.T) {
	big := json.RawMessage(`"` + strings.Repeat("x", 4000) + `"`)

	s := state.New()
	var err error
	if s, err = state.Apply(s, mustEnvOf(t, event.TypeInputReceived,
		&event.InputReceived{Text: "hi", Source: "cli"})); err != nil {
		t.Fatal(err)
	}
	s = applyToolTurn(t, s, "call_1_0", "read_file", big, false)
	for i := 0; i < microcompactRecentGuard+2; i++ {
		s = applyToolTurn(t, s, "pad_"+string(rune('a'+i)), "read_file", json.RawMessage(`"s"`), false)
	}

	before := ProjectContextWindow(s, cwSpec(10000, 0))
	if before.EstimatedTokens < 500 {
		t.Fatalf("estimate %d too small to be measuring the big result", before.EstimatedTokens)
	}

	if s, err = state.Apply(s, mustEnvOf(t, event.TypeContextMicrocompacted,
		&event.ContextMicrocompacted{Boundary: 3})); err != nil {
		t.Fatal(err)
	}
	after := ProjectContextWindow(s, cwSpec(10000, 0))

	if after.EstimatedTokens >= before.EstimatedTokens {
		t.Errorf("estimate did not shrink after microcompact: before=%d after=%d "+
			"(the projection must follow the assembled view, not a monotonic total)",
			before.EstimatedTokens, after.EstimatedTokens)
	}
}

// The output cap and the input window are different quantities; conflating
// them is the specific misreport G57 forbids.
func TestContextWindowNeverReportsOutputCapAsLimit(t *testing.T) {
	s := state.New()
	cw := ProjectContextWindow(s, cwSpec(10000, 0))
	if cw.LimitTokens == 100 {
		t.Fatal("LimitTokens took ModelSpec.MaxTokens (an OUTPUT cap) as the context window")
	}
	// scripted is an unmodelled provider and no envelope is folded here, so the
	// only honest answer is "unknown".
	if cw.LimitTokens != 0 {
		t.Errorf("LimitTokens = %d, want 0 (unknown)", cw.LimitTokens)
	}
	if cw.Estimator != contextEstimator {
		t.Errorf("Estimator = %q, want %q — the method must be self-described", cw.Estimator, contextEstimator)
	}
}

// The limit comes from the envelope frozen at SessionStarted, which is what
// lets a credential-less reader (inspect) report it.
func TestContextWindowReadsLimitFromFrozenEnvelope(t *testing.T) {
	env := provider.Envelope("gemini", "gemini-2.5-pro", provider.Capabilities{})
	s := state.New()
	var err error
	if s, err = state.Apply(s, mustEnvOf(t, event.TypeSessionStarted,
		&event.SessionStarted{SpecName: "x", Model: "gemini-2.5-pro",
			ProviderCapabilities: &env})); err != nil {
		t.Fatal(err)
	}
	cw := ProjectContextWindow(s, cwSpec(10000, 0))
	if cw.LimitTokens != 1_048_576 {
		t.Errorf("LimitTokens = %d, want 1048576 from the frozen envelope", cw.LimitTokens)
	}
}

// Thresholds are reported EFFECTIVE (post-defaulting), so a consumer showing
// "next compaction at …" uses the same numbers the loop triggers on.
func TestContextWindowReportsEffectiveThresholds(t *testing.T) {
	s := state.New()

	// micro unset defaults to 3/4 of compact.
	if cw := ProjectContextWindow(s, cwSpec(12000, 0)); cw.MicrocompactAtTokens != 9000 {
		t.Errorf("defaulted MicrocompactAtTokens = %d, want 9000", cw.MicrocompactAtTokens)
	}
	// micro disabled (-1) projects as 0, matching microcompactAt.
	if cw := ProjectContextWindow(s, cwSpec(12000, -1)); cw.MicrocompactAtTokens != 0 {
		t.Errorf("disabled MicrocompactAtTokens = %d, want 0", cw.MicrocompactAtTokens)
	}
	if cw := ProjectContextWindow(s, cwSpec(12000, 500)); cw.MicrocompactAtTokens != 500 {
		t.Errorf("explicit MicrocompactAtTokens = %d, want 500", cw.MicrocompactAtTokens)
	}
	// A spec-less observer still gets the estimate; only thresholds go unknown.
	if cw := ProjectContextWindow(s, nil); cw.CompactAtTokens != 0 || cw.Estimator == "" {
		t.Errorf("nil spec projection = %+v, want zero thresholds but a live estimator", cw)
	}
}
