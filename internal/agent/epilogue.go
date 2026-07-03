package agent

import (
	"context"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
)

// epilogueHook is one slot in the fixed run-ending sequence (standing
// hook 2). Later stages replace slot BODIES — the order never changes,
// and new end-of-run behavior MUST land in a slot, not around it.
type epilogueHook struct {
	name string
	run  func(ctx context.Context, ds *driveState, appendE AppendFunc, reason string) error
}

// epilogueSequence: quiesce → auto-publish → barrier → (terminal event).
//   - quiesce: wait for in-flight activity quiescence. No-op in S2: the
//     drive loop is serial, nothing is in flight at an ending. S6
//     (parallel tasks) fills this in using the Activities sub-state.
//   - auto_publish: publish run outputs. No-op until S7 world-state.
//   - barrier: the S7 run-end snapshot barrier slot, reserved no-op.
//
// S3.7c's LimitExceeded farewell message hooks in as a quiesce-slot
// predecessor per PLAN (挂进此序列).
var epilogueSequence = []epilogueHook{
	{name: "quiesce", run: noopEpilogueHook},
	{name: "auto_publish", run: noopEpilogueHook},
	{name: "barrier", run: noopEpilogueHook},
}

func noopEpilogueHook(context.Context, *driveState, AppendFunc, string) error { return nil }

// runEpilogue drives every run ending: the fixed hook sequence, then the
// terminal RunEnded fact. bestEffort (abort paths) presses on through hook
// and journal errors so a dying run still leaves a terminal marker if it
// possibly can.
func runEpilogue(ctx context.Context, ds *driveState, appendE AppendFunc,
	reason string, turns int, bestEffort bool) (RunResult, error) {

	for _, h := range epilogueSequence {
		if err := h.run(ctx, ds, appendE, reason); err != nil && !bestEffort {
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
