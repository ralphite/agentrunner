package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/tool"
)

// ApprovalRequest is what a resolver shows the human.
type ApprovalRequest struct {
	ApprovalID string
	CallID     string
	// Agent identifies WHO is asking (spec name + session): sub-agent asks
	// bubble to the same frontend, and the human must be able to tell a
	// child's destructive edit from the parent's harmless read (S5 review).
	Agent       string
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

// EnvApprovals is the non-interactive resolver. AGENTRUNNER_APPROVE=
//   - always / never (unset = never — loop-mode must fail closed, not hang)
//   - a comma-separated SEQUENCE of approve|deny[:reason] (S6 还债③):
//     answers are consumed in order and the last repeats once exhausted,
//     which makes multi-approval flows (plan 拒→v2→批) scriptable in
//     acceptance scenarios. The sequence position is resolver state — the
//     loop keeps ONE instance per tree (children share it).
type EnvApprovals struct {
	mu  sync.Mutex
	idx int
}

func (e *EnvApprovals) Resolve(_ context.Context, _ ApprovalRequest) (ApprovalDecision, error) {
	v := os.Getenv("AGENTRUNNER_APPROVE")
	switch v {
	case "always":
		return ApprovalDecision{Approve: true, Reason: "AGENTRUNNER_APPROVE=always", Source: "env"}, nil
	case "", "never":
		return ApprovalDecision{Approve: false, Reason: "auto-denied (AGENTRUNNER_APPROVE unset or never)", Source: "env"}, nil
	}
	parts := strings.Split(v, ",")
	e.mu.Lock()
	i := e.idx
	if i >= len(parts) {
		i = len(parts) - 1 // exhausted: the last answer repeats
	}
	e.idx++
	e.mu.Unlock()
	verb, reason, _ := strings.Cut(strings.TrimSpace(parts[i]), ":")
	switch verb {
	case "approve", "y", "yes":
		return ApprovalDecision{Approve: true, Reason: reason, Source: "env"}, nil
	default:
		if reason == "" {
			reason = "denied by AGENTRUNNER_APPROVE sequence"
		}
		return ApprovalDecision{Approve: false, Reason: reason, Source: "env"}, nil
	}
}

// requestApproval journals the ask (ApprovalRequested + WAITING_APPROVAL)
// and blocks on the resolver. awaitApproval is split out so resume can
// re-enter a idle wait without re-journaling the request.
func (l *Loop) requestApproval(ctx context.Context, ds *driveState, appendE AppendFunc,
	eff pipeline.Effect, outcome pipeline.Outcome) (bool, error) {

	req := event.ApprovalRequested{
		ApprovalID:  "apr-" + eff.ID,
		EffectID:    eff.ID,
		CallID:      eff.CallID,
		GateResults: outcome.GateResults,
		EstTokens:   eff.EstTokens,
	}
	// Plan approval payload (S5.7): the plan text publishes as a versioned
	// artifact BEFORE the request is journaled (blob-before-event), and the
	// approval fact pins the exact version ref it adjudicated — a rejected
	// plan's revision publishes v2 and the re-approval points there.
	if ref, err := l.publishApprovalPayload(eff, appendE); err != nil {
		return false, err
	} else if ref != "" {
		req.PayloadRef = ref
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

// publishApprovalPayload stores an approval's large payload in the
// ArtifactStore (S5.7). Today that is the exit_plan_mode plan text (stream
// "plan"); other asks carry their args inline. Returns "" when there is
// nothing to publish.
func (l *Loop) publishApprovalPayload(eff pipeline.Effect, appendE AppendFunc) (string, error) {
	if eff.ToolName != "exit_plan_mode" || l.Artifacts == nil {
		return "", nil
	}
	var args struct {
		Plan string `json:"plan"`
	}
	if err := json.Unmarshal(eff.Args, &args); err != nil || args.Plan == "" {
		return "", nil // no plan text: nothing to anchor
	}
	v, err := l.Artifacts.Publish("plan", []byte(redact.FromEnv().String(args.Plan)))
	if err != nil {
		return "", err
	}
	crash.Point(crash.PointAfterBlobBeforeEvent)
	if _, err := appendE(event.TypeArtifactPublished, &event.ArtifactPublished{
		Stream: v.Stream, Version: v.Version, Ref: v.Ref, Bytes: v.Bytes,
		Source: "approval",
	}); err != nil {
		return "", err
	}
	return v.Ref, nil
}

// awaitApproval races the resolver against a user interrupt. Every exit
// journals: response (external input first), waiting resolution, effect
// resolution. Returns whether the effect may execute.
func (l *Loop) awaitApproval(ctx context.Context, ds *driveState, appendE AppendFunc,
	req event.ApprovalRequested) (bool, error) {

	resolver := l.Approvals
	if resolver == nil {
		resolver = &EnvApprovals{}
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
		return l.resolveEffectAfterApproval(ds, appendE, req, out.d.Approve, out.d.Reason)

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
		return l.resolveEffectAfterApproval(ds, appendE, req, false, "[interrupted by user]")
	}
}

func (l *Loop) resolveEffectAfterApproval(ds *driveState, appendE AppendFunc,
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
	reserved := 0
	if approved {
		reserved = req.EstTokens
	}
	if _, err := appendE(event.TypeEffectResolved, &event.EffectResolved{
		EffectID: req.EffectID, CallID: req.CallID,
		Verdict: verdict, GateResults: results,
		ReservedTokens: reserved,
		Containment:    l.containmentByCall(ds, req.CallID),
	}); err != nil {
		return false, err
	}
	return approved, nil
}

// containmentByCall recovers the containment stamp for an approval-path
// resolution, where only the journaled request (not the effect) survives a
// crash: the call's tool name comes from the fold (S7 模块 5).
func (l *Loop) containmentByCall(ds *driveState, callID string) *event.Containment {
	if callID == "" || l.Exec == nil || !l.Exec.NetworkContained() {
		return nil
	}
	for _, m := range assistantMessages(ds.s) {
		for _, c := range toolCallsOf(m) {
			if c.CallID == callID {
				if toolClassIn(ds.s, c.Name) == string(tool.ClassExecute) &&
					!strings.HasPrefix(c.Name, "mcp__") {
					return &event.Containment{Network: "none", Backend: "netns"}
				}
				return nil
			}
		}
	}
	return nil
}

// approvalPrompt enriches the journaled request with the call's tool name
// and args (recovered from the fold) for display.
func (l *Loop) approvalPrompt(ds *driveState, req event.ApprovalRequested) ApprovalRequest {
	out := ApprovalRequest{
		ApprovalID:  req.ApprovalID,
		CallID:      req.CallID,
		GateResults: req.GateResults,
	}
	if l.Spec != nil {
		out.Agent = fmt.Sprintf("%s (%s)", l.Spec.Name, l.SessionID)
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
