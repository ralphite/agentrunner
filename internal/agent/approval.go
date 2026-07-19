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
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/tool"
)

// ApprovalRequest is what a resolver shows the human.
type ApprovalRequest struct {
	ApprovalID string
	CallID     string
	// Session is the exact asking member. A tree shares one resolver/sink,
	// so the transport cannot infer this from its hosted root.
	Session string
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
	protocol.CommandRef
	Approve  bool
	Reason   string
	Source   string // tty | env
	Remember bool   // INC-17 (G5): "allow and don't ask again" — persist a rule
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
		// This reason surfaces to the model AND to the user as the tool-result
		// error, so it must say how to proceed (T4/R2-B-2). Without it, a run
		// just prints "denied by policy" and cautious users conclude the tool
		// won't let the agent work — not realizing an interactive session asks.
		return ApprovalDecision{Approve: false, Source: "env",
			Reason: "needs approval, but this run is non-interactive so it was auto-denied. " +
				"Use `agentrunner new` for an interactive session that asks you, or auto-approve " +
				"with AGENTRUNNER_APPROVE=always, --mode bypass, or `permissions: [{action: allow}]` in the spec",
		}, nil
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
// Returns (allowed, denyReason, err): denyReason carries the resolver's
// denial reason so the loop can surface it in the model-visible tool result
// (empty when allowed).
func (l *Loop) requestApproval(ctx context.Context, ds *driveState, appendE AppendFunc,
	eff pipeline.Effect, outcome pipeline.Outcome) (bool, string, error) {

	// Standing approval (INC-62): an ask whose exact criterion the human
	// already always-allowed THIS SESSION is auto-answered approve — no
	// ApprovalRequested, no WAITING_APPROVAL, one EffectResolved that names
	// the standing answer. The criterion is extracted from the redacted args
	// (the same form awaitApproval journals), keeping both sides symmetric.
	// Escalation asks never reach here (their gate carries
	// ApprovalDenyFallback and a criterion never extracts for them), and a
	// child session folds its OWN journal — a parent's standing answer never
	// auto-approves a child's ask.
	if c, ok := standingCriterion(eff.ToolName, redact.FromEnv().JSON(eff.Args)); ok &&
		!eff.ApprovalDenyFallback && ds.s.Effects.HasStanding(c) {
		results := append(append([]event.GateResult{}, outcome.GateResults...), event.GateResult{
			Gate: "approval", Decision: event.VerdictAllow,
			Reason: "standing approval (this session): the user chose always-allow for this exact criterion",
		})
		if _, err := appendE(event.TypeEffectResolved, &event.EffectResolved{
			EffectID: eff.ID, CallID: eff.CallID,
			Verdict: event.VerdictAllow, GateResults: results,
			ReservedTokens: eff.EstTokens,
			Containment:    l.containment(eff),
		}); err != nil {
			return false, "", err
		}
		return true, "", nil
	}

	req := event.ApprovalRequested{
		ApprovalID:         "apr-" + eff.ID,
		EffectID:           eff.ID,
		CallID:             eff.CallID,
		GateResults:        outcome.GateResults,
		EstTokens:          eff.EstTokens,
		Containment:        l.containment(eff),
		DenyAllowsFallback: eff.ApprovalDenyFallback,
	}
	// Persist the exact reviewed operation even for tool calls. Recovery and
	// remote frontends must not depend on re-deriving authority details from
	// a possibly compacted conversation.
	req.ToolName = eff.ToolName
	req.Args = redact.FromEnv().JSON(eff.Args)
	// Plan approval payload (S5.7): the plan text publishes as a versioned
	// artifact BEFORE the request is journaled (blob-before-event), and the
	// approval fact pins the exact version ref it adjudicated — a rejected
	// plan's revision publishes v2 and the re-approval points there.
	if ref, err := l.publishApprovalPayload(eff, appendE); err != nil {
		return false, "", err
	} else if ref != "" {
		req.PayloadRef = ref
	}
	if _, err := appendE(event.TypeApprovalRequested, &req); err != nil {
		return false, "", err
	}
	detail, err := json.Marshal(req)
	if err != nil {
		return false, "", err
	}
	if _, err := appendE(event.TypeWaitingEntered, &event.WaitingEntered{
		Kind: event.WaitApproval, Detail: detail,
	}); err != nil {
		return false, "", err
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
	req event.ApprovalRequested) (bool, string, error) {

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

	for {
		select {
		case out := <-ch:
			if out.err != nil {
				return false, "", fmt.Errorf("approval %s: %w", req.ApprovalID, out.err)
			}
			decision := "deny"
			resolution := "denied"
			if out.d.Approve {
				decision = "approve"
				resolution = "approved"
			}
			responseAppend := appendE
			if out.d.CommandID != "" {
				responseAppend = l.commandAppender(ds, out.d.CommandID)
			}
			// "Allow and don't ask again" (INC-62): the standing criterion rides
			// the response FACT itself, so the fold — and any resume — knows this
			// session's later identical asks are already answered.
			var standing *event.StandingRule
			if out.d.Approve && out.d.Remember {
				if c, ok := standingCriterion(req.ToolName, req.Args); ok {
					standing = &c
				}
			}
			if _, err := responseAppend(event.TypeApprovalResponded, &event.ApprovalResponded{
				ApprovalID: req.ApprovalID, Decision: decision,
				Reason: out.d.Reason, Source: out.d.Source, Standing: standing,
			}); err != nil {
				return false, "", err
			}
			if _, err := responseAppend(event.TypeWaitingResolved, &event.WaitingResolved{
				Kind: event.WaitApproval, Resolution: resolution,
			}); err != nil {
				return false, "", err
			}
			// "Allow and don't ask again" (INC-17, G5): persist an exact allow rule
			// to the USER config so the NEXT session no longer asks. Best effort —
			// a writeback failure must never fail the approval (the user already
			// approved this call); it just does not persist.
			if out.d.Approve && out.d.Remember {
				l.rememberApproval(req)
			}
			ok, err := l.resolveEffectAfterApproval(ds, responseAppend, req, out.d.Approve, out.d.Reason)
			return ok, out.d.Reason, err

		case <-l.Interrupts:
			// Denied-by-interrupt (3.5): journal the interrupt (inputs first),
			// resolve the approval as a denial, render the call as interrupted,
			// and the loop CONTINUES — an interrupt is guidance, not shutdown.
			if _, err := appendE(event.TypeInputReceived, &event.InputReceived{
				Text: "[interrupt]", Source: "interrupt",
			}); err != nil {
				return false, "", err
			}
			if _, err := appendE(event.TypeApprovalResponded, &event.ApprovalResponded{
				ApprovalID: req.ApprovalID, Decision: "deny",
				Reason: "[interrupted by user]", Source: "interrupt",
			}); err != nil {
				return false, "", err
			}
			if _, err := appendE(event.TypeWaitingResolved, &event.WaitingResolved{
				Kind: event.WaitApproval, Resolution: WaitRules[event.WaitApproval].OnInterrupt,
			}); err != nil {
				return false, "", err
			}
			ok, err := l.resolveEffectAfterApproval(ds, appendE, req, false, "[interrupted by user]")
			return ok, "[interrupted by user]", err

		case ref := <-l.CommandInterrupts:
			cmdAppend := appendE
			if ref.CommandID != "" {
				cmdAppend = l.commandAppender(ds, ref.CommandID)
			}
			if _, err := cmdAppend(event.TypeInputReceived, &event.InputReceived{
				Text: "[interrupt]", Source: "interrupt",
			}); err != nil {
				return false, "", err
			}
			if _, err := cmdAppend(event.TypeApprovalResponded, &event.ApprovalResponded{
				ApprovalID: req.ApprovalID, Decision: "deny",
				Reason: "[interrupted by user]", Source: "interrupt",
			}); err != nil {
				return false, "", err
			}
			if _, err := cmdAppend(event.TypeWaitingResolved, &event.WaitingResolved{
				Kind: event.WaitApproval, Resolution: WaitRules[event.WaitApproval].OnInterrupt,
			}); err != nil {
				return false, "", err
			}
			ok, err := l.resolveEffectAfterApproval(ds, cmdAppend, req, false, "[interrupted by user]")
			return ok, "[interrupted by user]", err

		case in, open := <-l.UserInputs:
			// INC-70 Option B (G3 余项): a user-class message arriving at an
			// approval park is steering — the pending ask is superseded. Deny
			// it (the tool never runs) and feed the message at this SAME
			// boundary so the model turns with the user's words in context.
			// Everything else keeps waiting: machine/untrusted mail defers to
			// the turn end (G16 — untrusted content never drives an approval),
			// a revoked input is consumed AS revoked (INC-46), tree mail is
			// forwarded. The resolver goroutine stays alive across iterations.
			if !open {
				l.inboxClosed = true
				l.UserInputs = nil
				continue
			}
			l.drainRevokes()
			if in.CommandID != "" && l.revokedTargets[in.CommandID] {
				if err := l.journalInput(ds, appendE, in); err != nil {
					return false, "", err
				}
				continue
			}
			if in.DeliverySeq > 0 && in.DeliverySeq <= ds.s.Session.ConsumedInputSeq {
				continue // already-consumed replay
			}
			if in.Target != "" && in.Target != l.SessionID {
				if err := l.journalInput(ds, appendE, in); err != nil {
					return false, "", err
				}
				continue
			}
			if !protocol.UserClassSource(in.Source) {
				ds.deferredInputs = append(ds.deferredInputs, in)
				continue
			}
			cmdAppend := appendE
			if in.CommandID != "" {
				cmdAppend = l.commandAppender(ds, in.CommandID)
			}
			// Inputs first (the interrupt arm's doctrine) — and deferred
			// mail BEFORE this message: the delivery high-water is
			// monotonic, so a deferred earlier-seq input flushed after a
			// later-seq consume would be silently dropped as already
			// consumed. Same flush shape as drainSteer: a steer releases
			// the queue it jumped over.
			flush := ds.deferredInputs
			ds.deferredInputs = nil
			for _, fin := range flush {
				if err := l.journalInput(ds, appendE, fin); err != nil {
					return false, "", err
				}
			}
			if err := l.journalInput(ds, cmdAppend, in); err != nil {
				return false, "", err
			}
			const superseded = "[superseded by user message]"
			if _, err := cmdAppend(event.TypeApprovalResponded, &event.ApprovalResponded{
				ApprovalID: req.ApprovalID, Decision: "deny",
				Reason: superseded, Source: "user",
			}); err != nil {
				return false, "", err
			}
			if _, err := cmdAppend(event.TypeWaitingResolved, &event.WaitingResolved{
				Kind: event.WaitApproval, Resolution: WaitRules[event.WaitApproval].OnSteer,
			}); err != nil {
				return false, "", err
			}
			allowed, err := l.resolveEffectAfterApproval(ds, cmdAppend, req, false, superseded)
			return allowed, superseded, err
		}
	}
}

func (l *Loop) resolveEffectAfterApproval(ds *driveState, appendE AppendFunc,
	req event.ApprovalRequested, approved bool, reason string) (bool, error) {
	if approved && req.Containment != nil {
		if l.Exec == nil {
			approved = false
			reason = "approved effect lost its required OS sandbox"
		} else if _, err := l.Exec.SandboxInfo(); err != nil {
			approved = false
			reason = "approved effect cannot restore required OS sandbox: " + err.Error()
		}
	}

	allowed := approved || req.DenyAllowsFallback
	verdict := event.VerdictDeny
	decision := event.VerdictDeny
	if allowed {
		verdict = event.VerdictAllow
	}
	if approved {
		decision = event.VerdictAllow
	}
	results := append(append([]event.GateResult{}, req.GateResults...), event.GateResult{
		Gate: "approval", Decision: decision, Reason: reason,
	})
	if !approved && req.DenyAllowsFallback {
		results = append(results, event.GateResult{Gate: "authority_fallback",
			Decision: event.VerdictAllow, Reason: "escalation denied; continuing under parent∩child permissions"})
	}
	reserved := 0
	if allowed {
		reserved = req.EstTokens
	}
	if _, err := appendE(event.TypeEffectResolved, &event.EffectResolved{
		EffectID: req.EffectID, CallID: req.CallID,
		Verdict: verdict, GateResults: results,
		ReservedTokens: reserved,
		Containment:    approvalContainment(l, ds, req),
	}); err != nil {
		return false, err
	}
	return allowed, nil
}

func approvalContainment(l *Loop, ds *driveState, req event.ApprovalRequested) *event.Containment {
	if req.Containment != nil {
		if l.Exec == nil {
			return nil
		}
		info, err := l.Exec.SandboxInfo()
		if err != nil {
			return nil
		}
		return &event.Containment{Filesystem: info.Filesystem, Network: info.Network, Backend: info.Backend}
	}
	return l.containmentByCall(ds, req.CallID)
}

// containmentByCall recovers the containment stamp for an approval-path
// resolution, where only the journaled request (not the effect) survives a
// crash: the call's tool name comes from the fold (S7 模块 5).
func (l *Loop) containmentByCall(ds *driveState, callID string) *event.Containment {
	if callID == "" || l.Exec == nil {
		return nil
	}
	for _, m := range assistantMessages(ds.s) {
		for _, c := range toolCallsOf(m) {
			if c.CallID == callID {
				if c.Name == "bash" {
					return l.containment(pipeline.Effect{Kind: "tool_call", ToolName: "bash",
						Class: string(tool.ClassExecute)})
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
		Session:     l.SessionID,
		ToolName:    req.ToolName,
		Args:        req.Args,
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
