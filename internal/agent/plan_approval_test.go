package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// seqApprover answers approvals from a script, recording what it saw.
type seqApprover struct {
	answers []ApprovalDecision
	seen    []ApprovalRequest
}

func (s *seqApprover) Resolve(_ context.Context, req ApprovalRequest) (ApprovalDecision, error) {
	s.seen = append(s.seen, req)
	d := s.answers[0]
	if len(s.answers) > 1 {
		s.answers = s.answers[1:]
	}
	return d, nil
}

// S5.7 plan approval, the whole cycle: publish → review → REJECT (with a
// reason) → revised plan v2 → approve. Each ApprovalRequested pins the
// exact plan version it adjudicated via payload_ref.
func TestPlanApprovalFullFlow(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		// GenStep 1: propose plan v1.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "p1", Name: "exit_plan_mode",
				Args: map[string]any{"plan": "v1: wing it"}}},
			{Finish: "tool_use"},
		}},
		// GenStep 2 (still in plan mode after the denial): propose v2.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "p2", Name: "exit_plan_mode",
				Args: map[string]any{"plan": "v2: measured, reviewed steps"}}},
			{Finish: "tool_use"},
		}},
		// GenStep 3 (default mode now): wrap up.
		{Respond: []scripted.Event{{Text: "executing the approved plan"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Mode = pipeline.ModePlan
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		&pipeline.FloorGate{},
		&pipeline.PermissionGate{Rules: []pipeline.PermissionRule{{Action: "allow"}}, WS: l.Exec.WS},
	}}
	approver := &seqApprover{answers: []ApprovalDecision{
		{Approve: false, Reason: "too vague, add steps", Source: "tty"},
		{Approve: true, Reason: "looks solid", Source: "tty"},
	}}
	l.Approvals = approver

	res, err := l.Run(context.Background(), "plan then act")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.GenSteps != 3 {
		t.Fatalf("res = %+v", res)
	}

	// The plan stream holds both versions.
	streams, err := l.Artifacts.Streams()
	if err != nil {
		t.Fatal(err)
	}
	chain := streams["plan"]
	if len(chain) != 2 {
		t.Fatalf("plan versions = %+v", chain)
	}
	v1c, _ := l.Artifacts.Get(chain[0].Ref)
	v2c, _ := l.Artifacts.Get(chain[1].Ref)
	if !strings.Contains(string(v1c), "wing it") || !strings.Contains(string(v2c), "measured") {
		t.Errorf("plan contents = %q, %q", v1c, v2c)
	}

	// Each approval request pinned ITS version's ref.
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var requests []*event.ApprovalRequested
	var responses []*event.ApprovalResponded
	for _, e := range events {
		switch e.Type {
		case event.TypeApprovalRequested:
			dec, _ := event.DecodePayload(e)
			requests = append(requests, dec.(*event.ApprovalRequested))
		case event.TypeApprovalResponded:
			dec, _ := event.DecodePayload(e)
			responses = append(responses, dec.(*event.ApprovalResponded))
		}
	}
	if len(requests) != 2 || len(responses) != 2 {
		t.Fatalf("requests = %d, responses = %d", len(requests), len(responses))
	}
	if requests[0].PayloadRef != chain[0].Ref || requests[1].PayloadRef != chain[1].Ref {
		t.Errorf("payload refs do not pin the reviewed versions:\n req: %s / %s\n chain: %s / %s",
			requests[0].PayloadRef, requests[1].PayloadRef, chain[0].Ref, chain[1].Ref)
	}
	if responses[0].Decision != "deny" || !strings.Contains(responses[0].Reason, "too vague") {
		t.Errorf("first response = %+v, want the reasoned rejection", responses[0])
	}
	if responses[1].Decision != "approve" {
		t.Errorf("second response = %+v", responses[1])
	}

	// The denial kept plan mode; the approval transitioned out.
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if fold.CurrentMode() != pipeline.ModeDefault {
		t.Errorf("final mode = %q, want default after the approved exit", fold.CurrentMode())
	}
}
