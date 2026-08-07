package agent

import (
	"github.com/ralphite/agentrunner/internal/state"
)

// ContextWindow is the honest answer to "how full is this session's context?"
// (G57). It exists because the numbers a session already carries CANNOT
// answer that question: `Session.Usage` is a CUMULATIVE BILLED history (it
// only grows, and double-counts every resent prefix), and `ModelSpec.MaxTokens`
// is an OUTPUT cap. Neither is the size of the next assembled request, which
// is the only quantity a "72k / 258k used" indicator may claim to show.
//
// It is DERIVED, never journaled. The estimate is a pure function of the fold
// plus the spec, so computing it on read is both cheaper (no event per turn on
// the hottest path in the system) and strictly fresher than any stamped
// snapshot could be — there is no staleness window to disclose because the
// value is measured at the moment it is asked for.
type ContextWindow struct {
	// EstimatedTokens sizes the ASSEMBLED view — post-compaction,
	// post-microcompact, i.e. what the next request would actually carry.
	EstimatedTokens int `json:"estimated_tokens"`
	// LimitTokens is the model's input context window; 0 = this binary does
	// not know it. A consumer that gets 0 must render the used count WITHOUT
	// a ratio or a percentage — inventing a denominator is the one failure
	// this projection exists to prevent.
	LimitTokens int `json:"limit_tokens,omitempty"`
	// CompactAtTokens / MicrocompactAtTokens are the EFFECTIVE thresholds
	// after spec defaulting (micro defaults to 3/4 of compact; either can be
	// disabled), so a consumer can show "next compaction at …" using the same
	// numbers the loop will actually trigger on. 0 = that tier is disabled.
	CompactAtTokens      int `json:"compact_at_tokens,omitempty"`
	MicrocompactAtTokens int `json:"microcompact_at_tokens,omitempty"`
	// Estimator names the measurement method rather than hiding it. The
	// estimate is a coarse ~4-bytes-per-token count over assembled parts, NOT
	// a provider tokenizer result; a consumer that presents it as exact is
	// misreporting it.
	Estimator string `json:"estimator"`
}

// contextEstimator is the estimator's stable public name — bumping the method
// must bump this string so a consumer never silently reinterprets the number.
const contextEstimator = "assembled-bytes/4"

// ProjectContextWindow measures the session's context occupancy right now.
//
// Scope decisions (G57 asked for these explicitly):
//   - MODEL SWITCH: none exists mid-session, so the limit comes from the
//     capability envelope frozen at SessionStarted — the same provider/model
//     the assembled request will go to. If switching is ever added, the limit
//     becomes a per-turn fact and must move with it.
//   - TOOL PAYLOAD: counted. Tool args and results are assembled into the
//     request, so excluding them would understate the very growth that drives
//     compaction.
//   - CACHE TOKENS: not subtracted. A cached prefix still occupies the
//     context window; caching changes what it COSTS, not what it takes up.
//   - SUBAGENT / DRIVER: per-session. A child has its own context and its own
//     projection; parent and child windows are never summed.
func ProjectContextWindow(s state.State, spec *AgentSpec) ContextWindow {
	cw := ContextWindow{
		EstimatedTokens: estimateContextTokens(s),
		Estimator:       contextEstimator,
	}
	if spec != nil {
		cw.CompactAtTokens = spec.Model.CompactAtTokens
		cw.MicrocompactAtTokens = microcompactAt(spec)
	}
	if env := s.Session.ProviderCapabilities; env != nil {
		cw.LimitTokens = env.ContextLimitTokens
	}
	return cw
}
