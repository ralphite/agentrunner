package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/state"
)

// rememberApprover answers every ask "allow and don't ask again" (INC-62).
type rememberApprover struct{}

func (rememberApprover) Resolve(context.Context, ApprovalRequest) (ApprovalDecision, error) {
	return ApprovalDecision{Approve: true, Remember: true, Source: "tty"}, nil
}

// denyApprover proves the resolver WAS consulted (exactness checks).
type denyApprover struct{}

func (denyApprover) Resolve(context.Context, ApprovalRequest) (ApprovalDecision, error) {
	return ApprovalDecision{Approve: false, Reason: "asked", Source: "tty"}, nil
}

// plainApprover approves once WITHOUT Remember.
type plainApprover struct{}

func (plainApprover) Resolve(context.Context, ApprovalRequest) (ApprovalDecision, error) {
	return ApprovalDecision{Approve: true, Source: "tty"}, nil
}

func standingHarness(t *testing.T) (*driveState, *memAppend, AppendFunc) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // rememberApproval writes user config
	ds := &driveState{s: state.New()}
	m := &memAppend{}
	appendE := func(typ string, payload any) (event.Envelope, error) {
		env, err := m.append(typ, payload)
		if err != nil {
			return env, err
		}
		ds.s, err = state.Apply(ds.s, env)
		return env, err
	}
	return ds, m, appendE
}

func askOutcome() pipeline.Outcome {
	return pipeline.Outcome{Verdict: event.VerdictAsk, GateResults: []event.GateResult{{
		Gate: "permission", Decision: event.VerdictAsk, Reason: "edit requires approval",
	}}}
}

func effectOf(id, tool string, args map[string]string) pipeline.Effect {
	raw, _ := json.Marshal(args)
	return pipeline.Effect{ID: id, Kind: "tool_call", ToolName: tool,
		Class: "edit", Args: raw, CallID: "call_" + id}
}

func lastResolved(t *testing.T, m *memAppend) event.EffectResolved {
	t.Helper()
	var out event.EffectResolved
	found := false
	for _, e := range m.events {
		if e.Type == event.TypeEffectResolved {
			if err := json.Unmarshal(e.Payload, &out); err != nil {
				t.Fatal(err)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no EffectResolved journaled")
	}
	return out
}

// The G35 contract: after one "allow and don't ask again", an identical
// criterion in the SAME session resolves without consulting any resolver —
// no ApprovalRequested, no WAITING, one EffectResolved naming the standing
// answer.
func TestStandingApprovalSameSession(t *testing.T) {
	ds, m, appendE := standingHarness(t)
	l := &Loop{Approvals: rememberApprover{}}

	allowed, _, err := l.requestApproval(context.Background(), ds, appendE,
		effectOf("eff-1", "write_file", map[string]string{"path": "notes.txt"}), askOutcome())
	if err != nil || !allowed {
		t.Fatalf("first approval: allowed=%v err=%v", allowed, err)
	}
	if n := countType(m.events, event.TypeApprovalRequested); n != 1 {
		t.Fatalf("asks after first call = %d, want 1", n)
	}

	// Same criterion again — the resolver must NOT be consulted.
	l.Approvals = mustNotAskResolver{t}
	allowed, _, err = l.requestApproval(context.Background(), ds, appendE,
		effectOf("eff-2", "write_file", map[string]string{"path": "notes.txt"}), askOutcome())
	if err != nil || !allowed {
		t.Fatalf("standing approval: allowed=%v err=%v", allowed, err)
	}
	if n := countType(m.events, event.TypeApprovalRequested); n != 1 {
		t.Fatalf("asks after standing call = %d, want still 1 (no re-ask)", n)
	}
	res := lastResolved(t, m)
	if res.Verdict != event.VerdictAllow {
		t.Fatalf("standing resolution = %+v, want allow", res)
	}
	last := res.GateResults[len(res.GateResults)-1]
	if last.Gate != "approval" || last.Decision != event.VerdictAllow {
		t.Fatalf("standing gate result = %+v", last)
	}

	// A DIFFERENT path still asks — exact criterion, no widening.
	l.Approvals = denyApprover{}
	allowed, _, err = l.requestApproval(context.Background(), ds, appendE,
		effectOf("eff-3", "write_file", map[string]string{"path": "other.txt"}), askOutcome())
	if err != nil || allowed {
		t.Fatalf("different path: allowed=%v err=%v, want denied (resolver consulted)", allowed, err)
	}
	if n := countType(m.events, event.TypeApprovalRequested); n != 2 {
		t.Fatalf("asks after different path = %d, want 2", n)
	}
}

// spawn_agent standing answers are tool-level (G35 裁定): approving one
// child's spawn with Remember silences the ask for the NEXT child too —
// three teammates, one question.
func TestStandingApprovalSpawnAgent(t *testing.T) {
	ds, m, appendE := standingHarness(t)
	l := &Loop{Approvals: rememberApprover{}}

	spawn := func(id, agent string) pipeline.Effect {
		e := effectOf(id, "spawn_agent", map[string]string{"agent": agent, "prompt": "t"})
		e.Class = "execute"
		return e
	}
	allowed, _, err := l.requestApproval(context.Background(), ds, appendE, spawn("eff-1", "worker-a"), askOutcome())
	if err != nil || !allowed {
		t.Fatalf("first spawn: allowed=%v err=%v", allowed, err)
	}
	l.Approvals = mustNotAskResolver{t}
	for i, agent := range []string{"worker-b", "worker-c"} {
		allowed, _, err = l.requestApproval(context.Background(), ds, appendE,
			spawn("eff-"+string(rune('2'+i)), agent), askOutcome())
		if err != nil || !allowed {
			t.Fatalf("spawn %s: allowed=%v err=%v (must be standing-approved)", agent, allowed, err)
		}
	}
	if n := countType(m.events, event.TypeApprovalRequested); n != 1 {
		t.Fatalf("asks = %d, want 1 — G35's three-spawn scenario must ask once", n)
	}
}

// The standing answer is a journal fact: a fresh fold (= resume) still
// answers without consulting a resolver.
func TestStandingApprovalSurvivesResume(t *testing.T) {
	ds, m, appendE := standingHarness(t)
	l := &Loop{Approvals: rememberApprover{}}
	if allowed, _, err := l.requestApproval(context.Background(), ds, appendE,
		effectOf("eff-1", "bash", map[string]string{"command": "npm test"}), askOutcome()); err != nil || !allowed {
		t.Fatalf("seed approval: allowed=%v err=%v", allowed, err)
	}

	refolded, err := state.Fold(m.events)
	if err != nil {
		t.Fatal(err)
	}
	ds2 := &driveState{s: refolded}
	m2 := &memAppend{}
	appendE2 := func(typ string, payload any) (event.Envelope, error) {
		env, err := m2.append(typ, payload)
		if err != nil {
			return env, err
		}
		ds2.s, err = state.Apply(ds2.s, env)
		return env, err
	}
	l.Approvals = mustNotAskResolver{t}
	allowed, _, err := l.requestApproval(context.Background(), ds2, appendE2,
		effectOf("eff-2", "bash", map[string]string{"command": "npm test"}), askOutcome())
	if err != nil || !allowed {
		t.Fatalf("post-resume standing: allowed=%v err=%v", allowed, err)
	}
	if n := countType(m2.events, event.TypeApprovalRequested); n != 0 {
		t.Fatalf("post-resume asks = %d, want 0", n)
	}
}

// A plain approve (no Remember) journals no standing criterion: the next
// identical ask still consults the human.
func TestPlainApproveDoesNotStand(t *testing.T) {
	ds, m, appendE := standingHarness(t)
	l := &Loop{Approvals: plainApprover{}} // approves WITHOUT Remember

	if allowed, _, err := l.requestApproval(context.Background(), ds, appendE,
		effectOf("eff-1", "write_file", map[string]string{"path": "notes.txt"}), askOutcome()); err != nil || !allowed {
		t.Fatalf("first approval: allowed=%v err=%v", allowed, err)
	}
	l.Approvals = denyApprover{}
	if allowed, _, _ := l.requestApproval(context.Background(), ds, appendE,
		effectOf("eff-2", "write_file", map[string]string{"path": "notes.txt"}), askOutcome()); allowed {
		t.Fatal("second call auto-approved without Remember")
	}
	if n := countType(m.events, event.TypeApprovalRequested); n != 2 {
		t.Fatalf("asks = %d, want 2", n)
	}
}
