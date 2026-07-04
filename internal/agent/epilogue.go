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

// epilogueHook is one slot in the fixed run-ending sequence (standing
// hook 2). Later stages replace slot BODIES — the order never changes,
// and new end-of-run behavior MUST land in a slot, not around it. A slot
// may REWRITE the ending reason (the outputs contract downgrades a
// graceful ending to "contract_violation") but never reorders siblings.
type epilogueHook struct {
	name string
	run  func(ctx context.Context, l *Loop, ds *driveState, appendE AppendFunc, reason *string) error
}

// epilogueSequence: quiesce → auto-publish → barrier → (terminal event).
//   - quiesce: background tasks settle or cancel per on_run_end before the
//     terminal event (S6.1 填实 — the log never ends with tasks in flight).
//   - auto_publish: publish the spec's declared outputs and check the
//     deliverable contract (S5.6).
//   - barrier: the S7 run-end snapshot barrier slot, reserved no-op.
//
// S3.7c's LimitExceeded farewell message hooks in as a quiesce-slot
// predecessor per PLAN (挂进此序列).
var epilogueSequence = []epilogueHook{
	{name: "quiesce", run: quiesceTasks},
	{name: "auto_publish", run: autoPublishOutputs},
	{name: "barrier", run: noopEpilogueHook},
}

func noopEpilogueHook(context.Context, *Loop, *driveState, AppendFunc, *string) error {
	return nil
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
	if *reason != "completed" && *reason != "max_turns" {
		return nil
	}
	var missing []string
	for _, out := range l.Spec.Outputs {
		if ds.s.Run.Published[out.Name] > 0 {
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

// runEpilogue drives every run ending: the fixed hook sequence, then the
// terminal RunEnded fact. bestEffort (abort paths) presses on through hook
// and journal errors so a dying run still leaves a terminal marker if it
// possibly can.
func (l *Loop) runEpilogue(ctx context.Context, ds *driveState, appendE AppendFunc,
	reason string, turns int, bestEffort bool) (RunResult, error) {

	for _, h := range epilogueSequence {
		if err := h.run(ctx, l, ds, appendE, &reason); err != nil && !bestEffort {
			return RunResult{}, err
		}
	}
	crash.Point(crash.PointBeforeRunEnd)
	if _, err := appendE(event.TypeRunEnded, &event.RunEnded{
		Reason: reason, Turns: turns, Usage: ds.s.Run.Usage,
	}); err != nil && !bestEffort {
		return RunResult{}, err
	}
	return RunResult{Reason: reason, Turns: turns, Usage: ds.s.Run.Usage}, nil
}
