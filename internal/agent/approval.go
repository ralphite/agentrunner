package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
)

// ApprovalRequest is what a resolver shows the human.
type ApprovalRequest struct {
	ApprovalID  string
	CallID      string
	ToolName    string
	Args        json.RawMessage
	GateResults []event.GateResult
}

// ApprovalDecision is the human's answer.
type ApprovalDecision struct {
	Approve bool
	Reason  string
	Source  string // tty | env
}

// ApprovalResolver blocks until a decision arrives or ctx is done.
type ApprovalResolver interface {
	Resolve(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error)
}

// EnvApprovals is the non-interactive resolver: AGENTRUNNER_APPROVE=
// always|never (unset = never — loop-mode must fail closed, not hang).
type EnvApprovals struct{}

func (EnvApprovals) Resolve(_ context.Context, _ ApprovalRequest) (ApprovalDecision, error) {
	switch os.Getenv("AGENTRUNNER_APPROVE") {
	case "always":
		return ApprovalDecision{Approve: true, Reason: "AGENTRUNNER_APPROVE=always", Source: "env"}, nil
	default:
		return ApprovalDecision{Approve: false, Reason: "auto-denied (AGENTRUNNER_APPROVE unset or never)", Source: "env"}, nil
	}
}

// requestApproval journals the ask (ApprovalRequested + WAITING_APPROVAL)
// and blocks on the resolver. awaitApproval is split out so resume can
// re-enter a parked wait without re-journaling the request.
func (l *Loop) requestApproval(ctx context.Context, ds *driveState, appendE AppendFunc,
	eff pipeline.Effect, outcome pipeline.Outcome) (bool, error) {

	req := event.ApprovalRequested{
		ApprovalID:  "apr-" + eff.ID,
		EffectID:    eff.ID,
		CallID:      eff.CallID,
		GateResults: outcome.GateResults,
		// PayloadRef reserved: large payloads move to the ArtifactStore (S7).
	}
	if _, err := appendE(event.TypeApprovalRequested, &req); err != nil {
		return false, err
	}
	detail, err := json.Marshal(req)
	if err != nil {
		return false, err
	}
	if _, err := appendE(event.TypeWaitingEntered, &event.WaitingEntered{
		Kind: event.WaitApproval, Detail: detail,
	}); err != nil {
		return false, err
	}
	return l.awaitApproval(ctx, ds, appendE, req)
}

// awaitApproval races the resolver against a user interrupt. Every exit
// journals: response (external input first), waiting resolution, effect
// resolution. Returns whether the effect may execute.
func (l *Loop) awaitApproval(ctx context.Context, ds *driveState, appendE AppendFunc,
	req event.ApprovalRequested) (bool, error) {

	resolver := l.Approvals
	if resolver == nil {
		resolver = EnvApprovals{}
	}
	rctx, rcancel := context.WithCancel(ctx)
	defer rcancel()

	type outcome struct {
		d   ApprovalDecision
		err error
	}
	// The prompt is built HERE, not in the goroutine: ds belongs to the
	// drive goroutine, and the interrupt arm mutates it concurrently.
	prompt := l.approvalPrompt(ds, req)
	ch := make(chan outcome, 1)
	go func() {
		d, err := resolver.Resolve(rctx, prompt)
		ch <- outcome{d, err}
	}()

	select {
	case out := <-ch:
		if out.err != nil {
			return false, fmt.Errorf("approval %s: %w", req.ApprovalID, out.err)
		}
		decision := "deny"
		resolution := "denied"
		if out.d.Approve {
			decision = "approve"
			resolution = "approved"
		}
		if _, err := appendE(event.TypeApprovalResponded, &event.ApprovalResponded{
			ApprovalID: req.ApprovalID, Decision: decision,
			Reason: out.d.Reason, Source: out.d.Source,
		}); err != nil {
			return false, err
		}
		if _, err := appendE(event.TypeWaitingResolved, &event.WaitingResolved{
			Kind: event.WaitApproval, Resolution: resolution,
		}); err != nil {
			return false, err
		}
		return l.resolveEffectAfterApproval(appendE, req, out.d.Approve, out.d.Reason)

	case <-l.Interrupts:
		// Denied-by-interrupt (3.5): journal the interrupt (inputs first),
		// resolve the approval as a denial, render the call as interrupted,
		// and the loop CONTINUES — an interrupt is guidance, not shutdown.
		if _, err := appendE(event.TypeInputReceived, &event.InputReceived{
			Text: "[interrupt]", Source: "interrupt",
		}); err != nil {
			return false, err
		}
		if _, err := appendE(event.TypeApprovalResponded, &event.ApprovalResponded{
			ApprovalID: req.ApprovalID, Decision: "deny",
			Reason: "[interrupted by user]", Source: "interrupt",
		}); err != nil {
			return false, err
		}
		if _, err := appendE(event.TypeWaitingResolved, &event.WaitingResolved{
			Kind: event.WaitApproval, Resolution: "denied_by_interrupt",
		}); err != nil {
			return false, err
		}
		return l.resolveEffectAfterApproval(appendE, req, false, "[interrupted by user]")
	}
}

func (l *Loop) resolveEffectAfterApproval(appendE AppendFunc,
	req event.ApprovalRequested, approved bool, reason string) (bool, error) {

	verdict := event.VerdictDeny
	decision := event.VerdictDeny
	if approved {
		verdict = event.VerdictAllow
		decision = event.VerdictAllow
	}
	results := append(append([]event.GateResult{}, req.GateResults...), event.GateResult{
		Gate: "approval", Decision: decision, Reason: reason,
	})
	if _, err := appendE(event.TypeEffectResolved, &event.EffectResolved{
		EffectID: req.EffectID, CallID: req.CallID,
		Verdict: verdict, GateResults: results,
	}); err != nil {
		return false, err
	}
	return approved, nil
}

// approvalPrompt enriches the journaled request with the call's tool name
// and args (recovered from the fold) for display.
func (l *Loop) approvalPrompt(ds *driveState, req event.ApprovalRequested) ApprovalRequest {
	out := ApprovalRequest{
		ApprovalID:  req.ApprovalID,
		CallID:      req.CallID,
		GateResults: req.GateResults,
	}
	if req.CallID == "" {
		return out
	}
	for _, m := range assistantMessages(ds.s) {
		for _, c := range toolCallsOf(m) {
			if c.CallID == req.CallID {
				out.ToolName = c.Name
				out.Args = c.Args
			}
		}
	}
	return out
}
