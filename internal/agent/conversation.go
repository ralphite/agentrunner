package agent

import (
	"context"
	"fmt"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/redact"
)

// longPasteThreshold is the input size past which the text body folds into
// a file part (v2 M4.3, DESIGN §4): the journal line and the durable
// transcript stay small; the full bytes live in the CAS and reach the model
// through the same inflate path as any attachment.
const longPasteThreshold = 10 * 1024

// journalInput records one user message (journal-inputs-first, redacted).
// Attached blob bytes go into the CAS BEFORE the event lands (blob-before-
// event, v2 M4.1): the journal carries only refs, never bytes.
func (l *Loop) journalInput(appendE AppendFunc, in protocol.UserInput) error {
	var images, files []event.AttachmentRef
	text := redact.FromEnv().String(in.Text)
	for _, img := range in.Images {
		if err := l.ensureArtifacts(); err != nil {
			return err
		}
		ref, err := l.Artifacts.Put(img.Data)
		if err != nil {
			return err
		}
		images = append(images, event.AttachmentRef{Ref: ref, MediaType: img.MediaType})
	}
	// Long-paste folding (v2 M4.3): an oversized text body becomes a file
	// part plus a short head, AFTER redaction (the CAS copy must be as
	// redacted as the journal itself).
	if len(text) > longPasteThreshold {
		if err := l.ensureArtifacts(); err != nil {
			return err
		}
		ref, err := l.Artifacts.Put([]byte(text))
		if err != nil {
			return err
		}
		files = append(files, event.AttachmentRef{Ref: ref, MediaType: "text/plain"})
		head := text[:512]
		text = head + fmt.Sprintf("\n…[长文本已折叠为附件 %s,共 %d 字节,完整内容见 file part]", ref, len(text))
	}
	_, err := appendE(event.TypeInputReceived, &event.InputReceived{
		Text: text, Source: "user", Images: images, Files: files,
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
		case in, ok := <-l.UserInputs:
			if !ok {
				l.inboxClosed = true
				l.UserInputs = nil
				return nil
			}
			if err := l.journalInput(appendE, in); err != nil {
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
	case in, ok := <-l.UserInputs: // nil channel blocks — tasks/interrupt still wake
		if !ok {
			// Channel closed = the user is done: graceful close.
			return true, resolve("closed")
		}
		// journal-inputs-first: journal this input, then batch-drain any
		// others that queued behind it (type-ahead) so they all enter the
		// same next turn — then resolve the park.
		if err := l.journalInput(appendE, in); err != nil {
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
		// A user's kill at idle: journal it and cancel the handle; the
		// cancelled child settles through bg.done, which re-parks and wakes
		// the next turn.
		if err := l.cancelHandle(appendE, handle); err != nil {
			return false, err
		}
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
