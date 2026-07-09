package agent

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/redact"
)

// quiescentHook is one slot in the fixed quiescent-actions sequence
// (决策 #24). Later features replace slot BODIES — the order never
// changes, and new at-quiescence behavior MUST land in a slot, not around
// it. A slot may REWRITE the finishing reason (the outputs contract
// downgrades "completed" to "contract_violation") but never reorders
// siblings.
type quiescentHook struct {
	name string
	run  func(ctx context.Context, l *Loop, ds *driveState, appendE AppendFunc, reason *string) error
}

// quiescentSequence: auto-publish → barrier. Runs at EVERY quiescence
// (决策 #31 — a session can quiesce, wake, and quiesce again; the actions
// repeat). The third fixed action — the parent receipt — is posted by
// whoever launched this session, from drive's return: receipts live in the
// PARENT's stream. Nothing here journals a terminal fact; quiescence is a
// shape, not an event.
var quiescentSequence = []quiescentHook{
	{name: "auto_publish", run: autoPublishOutputs},
	{name: "barrier", run: quiescentBarrier},
	// goal_verify (INC-D1) is LAST so the barrier snapshots a genuinely
	// quiescent, pre-injection turn boundary (clean fork/rewind + crash
	// anchor). On a miss it re-injects a program input; the wake seam in
	// idleOrReturn then continues the thread instead of idling.
	{name: "goal_verify", run: goalCheckpoint},
}

// quiescentBarrier fills the barrier slot: one CheckpointBarrier over the
// quiescent state (outputs published, nothing in flight). Feature-gated
// like every barrier — no snapshot store, no barrier — and it never
// rewrites the finishing reason.
func quiescentBarrier(ctx context.Context, l *Loop, ds *driveState,
	appendE AppendFunc, _ *string) error {
	return l.takeBarrier(ctx, ds, appendE, "bar-final", 0)
}

// autoPublishOutputs fills the auto-publish slot (S5.6): on a GRACEFUL
// ending, every spec-declared output that was not explicitly published
// during the run is auto-published from its workspace file; a required
// output that still cannot be satisfied downgrades the ending to
// "contract_violation" — the deliverable contract is a hard check, never a
// silent success. Non-graceful endings (error/canceled/handoff/blocked)
// skip the slot: a dying or transferred run owes no deliverables.
func autoPublishOutputs(_ context.Context, l *Loop, ds *driveState,
	appendE AppendFunc, reason *string) error {

	if len(l.Spec.Outputs) == 0 {
		return nil
	}
	if *reason != "completed" && *reason != "max_generation_steps" {
		return nil
	}
	var missing []string
	for _, out := range l.Spec.Outputs {
		if ds.s.Session.Published[out.Name] > 0 {
			continue // explicitly published during the run — satisfied
		}
		if out.Path != "" && l.Exec != nil && l.Exec.WS != nil && l.Artifacts != nil {
			if content, ok := readWorkspaceFile(l, out.Path); ok {
				// Same redaction-before-persist as publish_artifact (S5).
				content = []byte(redact.FromEnv().String(string(content)))
				v, err := l.Artifacts.Publish(out.Name, content)
				if err != nil {
					return err // disk failure is a harness error
				}
				crash.Point(crash.PointAfterBlobBeforeEvent)
				if _, err := appendE(event.TypeArtifactPublished, &event.ArtifactPublished{
					Stream: v.Stream, Version: v.Version, Ref: v.Ref, Bytes: v.Bytes,
					Source: "epilogue",
				}); err != nil {
					return err
				}
				continue
			}
		}
		if out.Required {
			missing = append(missing, out.Name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		*reason = "contract_violation"
		l.emit(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("outputs contract violated: missing required %s",
				strings.Join(missing, ", "))})
	}
	return nil
}

// readWorkspaceFile resolves a declared output path inside the workspace.
func readWorkspaceFile(l *Loop, path string) ([]byte, bool) {
	resolved, err := l.Exec.WS.Resolve(path)
	if err != nil {
		return nil, false
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return nil, false
	}
	return content, true
}

// quiescentActions runs the fixed at-quiescence sequence (决策 #24/#31).
// reason may be rewritten by a slot (contract_violation). Never journals a
// terminal fact — quiescence is a journal shape, and the session stays
// reopenable forever.
func (l *Loop) quiescentActions(ctx context.Context, ds *driveState, appendE AppendFunc,
	reason *string) error {

	for _, h := range quiescentSequence {
		if err := h.run(ctx, l, ds, appendE, reason); err != nil {
			return err
		}
	}
	return nil
}

// closeSession journals the user's explicit close MARK (决策 #30) after
// settling any in-flight background work — the journal never ends with
// orphans. The mark gates automatic paths only; a later send reopens.
func (l *Loop) closeSession(ctx context.Context, ds *driveState, appendE AppendFunc,
	turns int) (RunResult, error) {

	l.settleOnAbort(ctx, ds, appendE)
	crash.Point(crash.PointBeforeCloseMark)
	if _, err := appendE(event.TypeSessionClosed, &event.SessionClosed{
		Reason: "closed", Source: "user", GenSteps: turns, Usage: ds.s.Session.Usage,
	}); err != nil {
		return RunResult{}, err
	}
	return RunResult{Reason: "closed", GenSteps: turns, Usage: ds.s.Session.Usage}, nil
}
