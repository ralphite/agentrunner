package pipeline

import (
	"context"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
)

type fakeGate struct {
	name     string
	decision Decision
	called   *bool
}

func (g fakeGate) Name() string { return g.name }
func (g fakeGate) Check(context.Context, Effect) Decision {
	if g.called != nil {
		*g.called = true
	}
	return g.decision
}

func TestEvaluateAllAllow(t *testing.T) {
	p := &Pipeline{Gates: []Gate{
		fakeGate{name: "hooks", decision: Allow},
		fakeGate{name: "permission", decision: Allow},
		fakeGate{name: "budget", decision: Allow},
	}}
	out, err := p.Evaluate(context.Background(), Effect{ID: "eff-x"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != event.VerdictAllow || len(out.GateResults) != 3 {
		t.Fatalf("out = %+v", out)
	}
}

// Deny short-circuits: the gate after the denier never runs.
func TestEvaluateDenyShortCircuits(t *testing.T) {
	var budgetRan bool
	p := &Pipeline{Gates: []Gate{
		fakeGate{name: "hooks", decision: Allow},
		fakeGate{name: "permission", decision: Deny("path escapes workspace")},
		fakeGate{name: "budget", decision: Allow, called: &budgetRan},
	}}
	out, err := p.Evaluate(context.Background(), Effect{ID: "eff-x"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != event.VerdictDeny || len(out.GateResults) != 2 {
		t.Fatalf("out = %+v", out)
	}
	if budgetRan {
		t.Fatal("gate after deny must not run")
	}
	if out.GateResults[1].Reason != "path escapes workspace" {
		t.Errorf("results = %+v", out.GateResults)
	}
}

// Ask aggregates: evaluation continues, and a later deny still wins.
func TestEvaluateAskAggregatesAndDenyWins(t *testing.T) {
	p := &Pipeline{Gates: []Gate{
		fakeGate{name: "permission", decision: Ask("edit outside allow-list")},
		fakeGate{name: "budget", decision: Allow},
	}}
	out, err := p.Evaluate(context.Background(), Effect{ID: "eff-x"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != event.VerdictAsk || len(out.GateResults) != 2 {
		t.Fatalf("out = %+v", out)
	}

	p.Gates = append(p.Gates, fakeGate{name: "hooks", decision: Deny("blocked")})
	out, err = p.Evaluate(context.Background(), Effect{ID: "eff-x"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != event.VerdictDeny {
		t.Fatalf("deny after ask must win: %+v", out)
	}
}

func TestEvaluateNilPipelineAllows(t *testing.T) {
	var p *Pipeline
	out, err := p.Evaluate(context.Background(), Effect{ID: "eff-x"})
	if err != nil || out.Verdict != event.VerdictAllow {
		t.Fatalf("out = %+v err = %v", out, err)
	}
}

func TestEvaluateInvalidActionErrors(t *testing.T) {
	p := &Pipeline{Gates: []Gate{fakeGate{name: "rogue", decision: Decision{Action: "maybe"}}}}
	if _, err := p.Evaluate(context.Background(), Effect{}); err == nil {
		t.Fatal("invalid gate action must error")
	}
}
