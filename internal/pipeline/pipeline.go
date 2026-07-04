// Package pipeline is the effect-governance layer (S3): every side effect
// is described as an Effect and adjudicated by an ordered gate sequence
// before execution. Evaluation is PURE — no journaling, no clock, no I/O;
// the loop owns durability (EffectResolved / ApprovalRequested facts).
//
// Gate order is fixed: pre-hooks → permission → budget. Deny short-
// circuits (later gates never run); ask aggregates (any ask with no deny
// escalates to approval, carrying every gate's judgment); all-allow
// executes.
package pipeline

import (
	"context"
	"fmt"

	"github.com/ralphite/agentrunner/internal/event"
)

// Effect describes one side effect awaiting adjudication.
type Effect struct {
	ID        string // eff-<call_id> | eff-llm-t<turn>
	Kind      string // tool_call | llm_call
	ToolName  string
	Class     string // tool class: read | edit | execute | wait
	Args      []byte
	CallID    string
	EstTokens int    // budget reservation basis (3.7)
	Mode      string // current run mode at adjudication time (3.6)
	// Budget is the fold's live accounting at adjudication time (3.7).
	Budget BudgetView
	// SpawnDepth/SpawnCount feed the spawn gate (S5.3): this run's depth in
	// the agent tree and the spawns it has already requested. Zero for
	// non-spawn effects. HandoffPending marks that an earlier call of the
	// SAME batch already transferred control (S5.4): once a handoff is
	// allowed, no further agent launch in that turn may run.
	SpawnDepth     int
	SpawnCount     int
	HandoffPending bool
}

// Decision is one gate's judgment.
type Decision struct {
	Action string // event.VerdictAllow | VerdictAsk | VerdictDeny
	Reason string
}

var (
	Allow = Decision{Action: event.VerdictAllow}
)

func Ask(reason string) Decision  { return Decision{Action: event.VerdictAsk, Reason: reason} }
func Deny(reason string) Decision { return Decision{Action: event.VerdictDeny, Reason: reason} }

// Gate adjudicates effects. Check must be pure and fast; anything slow or
// side-effecting (hook processes) runs before evaluation and feeds its
// conclusion in via the gate's own state.
type Gate interface {
	Name() string
	Check(ctx context.Context, eff Effect) Decision
}

// Outcome is the pipeline's aggregate verdict.
type Outcome struct {
	Verdict     string // allow | ask | deny
	GateResults []event.GateResult
}

// Pipeline is the ordered gate sequence.
type Pipeline struct {
	Gates []Gate
}

// SideEffectingGate marks gates whose Check has external side effects
// (hook processes). Their adjudication window is in-doubt after a crash;
// pure windows just re-adjudicate.
type SideEffectingGate interface {
	SideEffecting() bool
}

// SideEffecting reports whether any gate declares side effects.
func (p *Pipeline) SideEffecting() bool {
	if p == nil {
		return false
	}
	for _, g := range p.Gates {
		if se, ok := g.(SideEffectingGate); ok && se.SideEffecting() {
			return true
		}
	}
	return false
}

// Evaluate runs the gates in order. Deny short-circuits; ask is recorded
// and evaluation continues (an approval prompt must show every gate's
// judgment, and a later deny still wins).
func (p *Pipeline) Evaluate(ctx context.Context, eff Effect) (Outcome, error) {
	out := Outcome{Verdict: event.VerdictAllow}
	if p == nil {
		return out, nil
	}
	for _, g := range p.Gates {
		d := g.Check(ctx, eff)
		switch d.Action {
		case event.VerdictAllow, event.VerdictAsk, event.VerdictDeny:
		default:
			return Outcome{}, fmt.Errorf("pipeline: gate %q returned invalid action %q", g.Name(), d.Action)
		}
		out.GateResults = append(out.GateResults, event.GateResult{
			Gate: g.Name(), Decision: d.Action, Reason: d.Reason,
		})
		if d.Action == event.VerdictDeny {
			out.Verdict = event.VerdictDeny
			return out, nil // short-circuit: later gates never run
		}
		if d.Action == event.VerdictAsk {
			out.Verdict = event.VerdictAsk
		}
	}
	return out, nil
}
