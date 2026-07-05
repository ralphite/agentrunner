package agent

import (
	"context"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/redact"
)

// journalInput records one user message (journal-inputs-first, redacted).
func (l *Loop) journalInput(appendE AppendFunc, text string) error {
	_, err := appendE(event.TypeInputReceived, &event.InputReceived{
		Text: redact.FromEnv().String(text), Source: "user",
	})
	return err
}

// drainQueued non-blockingly journals every ADDITIONAL input already queued
// on UserInputs, in arrival order (v2 M2.1 type-ahead): messages that piled
// up while a turn ran all enter the next turn's context together. Stops at
// the first empty read; a close seen here is remembered for the park.
func (l *Loop) drainQueued(appendE AppendFunc) error {
	for {
		select {
		case text, ok := <-l.UserInputs:
			if !ok {
				l.inboxClosed = true
				l.UserInputs = nil
				return nil
			}
			if err := l.journalInput(appendE, text); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

// parkForInput waits at the conversational idle and, on close, runs the
// epilogue and returns done=true with the terminal result. On an input or
// task settlement it returns done=false so the drive loop continues. Shared
// by the fresh park (doWaitInput) and the resumed park (doWait/WaitInput):
// neither re-journals WaitingEntered here.
func (l *Loop) parkForInput(ctx context.Context, ds *driveState, appendE AppendFunc,
	turn int) (RunResult, bool, error) {

	closed, err := l.awaitInput(ctx, appendE, turn)
	if err != nil {
		// A cancelled context is a process teardown (crash/shutdown), NOT an
		// ending: the parked session must resume later, so it leaves NO
		// terminal. Only a genuine journal failure gets a best-effort
		// terminal so the log still closes.
		if ctx.Err() == nil {
			_, _ = l.runEpilogue(ctx, ds, appendE, "error", turn, true)
		}
		return RunResult{}, true, err
	}
	if closed {
		res, eerr := l.runEpilogue(ctx, ds, appendE, "closed", turn, false)
		l.emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: res.Reason, Turn: res.Turns})
		return res, true, eerr
	}
	return RunResult{}, false, nil
}

// awaitInput is the conversational idle (v2 M1.1, DESIGN v2 §1): the model
// yielded and nothing is pending, so the session waits — for the next user
// input, a background settlement, an interrupt (the close gesture at idle),
// or hard cancellation. The WaitingEntered{input} fact is journaled by the
// caller (the resume path re-enters here without re-journaling it).
// Returns closed=true when the session should end via the epilogue.
func (l *Loop) awaitInput(ctx context.Context, appendE AppendFunc, turn int) (closed bool, err error) {
	l.ensureBackground()
	resolve := func(resolution string) error {
		_, rerr := appendE(event.TypeWaitingResolved, &event.WaitingResolved{
			Kind: event.WaitInput, Resolution: resolution,
		})
		return rerr
	}
	if l.inboxClosed {
		// drainInbox saw the channel close at a boundary: nothing left to
		// wait for — resolve the park and close now.
		return true, resolve("closed")
	}
	l.emit(protocol.Event{Kind: protocol.KindIdle, Turn: turn})
	select {
	case text, ok := <-l.UserInputs: // nil channel blocks — tasks/interrupt still wake
		if !ok {
			// Channel closed = the user is done: graceful close.
			return true, resolve("closed")
		}
		// journal-inputs-first: journal this input, then batch-drain any
		// others that queued behind it (type-ahead) so they all enter the
		// same next turn — then resolve the park.
		if err := l.journalInput(appendE, text); err != nil {
			return false, err
		}
		if err := l.drainQueued(appendE); err != nil {
			return false, err
		}
		return false, resolve("input_received")
	case out := <-l.bg.done:
		// A background task settled while idle: its outcome is a user-role
		// input (S6.1), which decide() turns into the next turn.
		if err := l.settleBackground(appendE, out); err != nil {
			return false, err
		}
		return false, resolve("task_settled")
	case handle := <-l.Cancels:
		// A user's kill at idle: cancel the handle; the cancelled child
		// settles through bg.done, which re-parks and wakes the next turn.
		l.cancelHandle(handle)
		return false, resolve("child_cancelled")
	case <-l.Interrupts:
		// Ctrl-C at the idle prompt closes the session (the interactive
		// convention); during a turn it steers, only at idle it closes.
		if err := l.onSteeringInterrupt(appendE, turn); err != nil {
			return false, err
		}
		return true, resolve("closed_by_interrupt")
	case <-ctx.Done():
		return false, context.Cause(ctx)
	}
}
