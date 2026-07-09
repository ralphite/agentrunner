package agent

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/ralphite/agentrunner/internal/command"
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
// event, v2 M4.1): the journal carries only refs, never bytes. A mailbox-
// seq'd input at or below the fold's high-water mark is a duplicate
// delivery (the resume replay raced the channel copy) and is dropped —
// at-least-once + seq dedup = effectively once.
func (l *Loop) journalInput(ds *driveState, appendE AppendFunc, in protocol.UserInput) error {
	if in.DeliverySeq > 0 && in.DeliverySeq <= ds.s.Session.ConsumedInputSeq {
		return nil
	}
	var images, files []event.AttachmentRef
	// Custom-command expansion (G21): a /name send expands to its repo macro
	// body before redaction+journaling, so the journaled InputReceived is the
	// expanded prompt (fold stays pure; resume self-contained). Non-slash and
	// unknown /commands pass through unchanged.
	raw := in.Text
	if l.Exec != nil && l.Exec.WS != nil {
		if expanded, ok := command.Expand(l.Exec.WS.Root(), raw); ok {
			raw = expanded
		}
	}
	text := redact.FromEnv().String(raw)
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
		for len(head) > 0 && !utf8.ValidString(head) {
			head = head[:len(head)-1] // never split a multi-byte rune
		}
		text = head + fmt.Sprintf("\n…[长文本已折叠为附件 %s,共 %d 字节,完整内容见 file part]", ref, len(text))
	}
	_, err := appendE(event.TypeInputReceived, &event.InputReceived{
		Text: text, Source: "user", Images: images, Files: files,
		DeliverySeq: in.DeliverySeq,
	})
	return err
}

// drainQueued non-blockingly journals every ADDITIONAL input already queued
// on UserInputs, in arrival order (v2 M2.1 type-ahead): messages that piled
// up while a turn ran all enter the next turn's context together. Stops at
// the first empty read; a close seen here is remembered for the idle.
func (l *Loop) drainQueued(ds *driveState, appendE AppendFunc) error {
	for {
		select {
		case in, ok := <-l.UserInputs:
			if !ok {
				l.inboxClosed = true
				l.UserInputs = nil
				return nil
			}
			if err := l.journalInput(ds, appendE, in); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

// idleForInput waits at the standby idle. On the explicit close gesture it
// journals the close MARK (决策 #30) and returns done=true; on an input or
// settlement it returns done=false so the drive loop continues. Shared by
// the fresh idle (doIdle) and the resumed idle (doWait/WaitInput): neither
// re-journals WaitingEntered here.
func (l *Loop) idleForInput(ctx context.Context, ds *driveState, appendE AppendFunc,
	turn int) (RunResult, bool, error) {

	closed, err := l.awaitInput(ctx, ds, appendE, turn)
	if err != nil {
		// A cancelled context is a process teardown (crash/shutdown), NOT
		// an ending: the idle session must resume later, so it leaves
		// NOTHING. A genuine failure settles in-flight work best-effort and
		// leaves no terminal either — there is none to leave (决策 #31).
		if ctx.Err() == nil {
			l.settleOnAbort(ctx, ds, appendE)
		}
		return RunResult{}, true, err
	}
	if closed {
		res, cerr := l.closeSession(ctx, ds, appendE, turn)
		l.emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: res.Reason, N: res.GenSteps})
		return res, true, cerr
	}
	return RunResult{}, false, nil
}

// awaitInput is the standby idle (v2 M1.1, DESIGN §1): the turn is over
// and nothing is pending, so the session waits — for the next user input,
// a background settlement, a kill, or hard cancellation. An interrupt at
// idle is a NO-OP (裁决 #11, 2026-07-05: interrupt never ends a session —
// during a turn it cancels the current activity, at idle there is nothing
// to interrupt; close is its own explicit command). The
// WaitingEntered{input} fact is journaled by the caller (the resume path
// re-enters here without re-journaling it). Returns closed=true on the
// explicit close gesture (the input channel closing).
func (l *Loop) awaitInput(ctx context.Context, ds *driveState, appendE AppendFunc, turn int) (closed bool, err error) {
	l.ensureBackground()
	resolve := func(resolution string) error {
		_, rerr := appendE(event.TypeWaitingResolved, &event.WaitingResolved{
			Kind: event.WaitInput, Resolution: resolution,
		})
		return rerr
	}
	if l.inboxClosed {
		// drainQueued saw the channel close at a boundary: nothing left to
		// wait for — resolve the idle and close now.
		return true, resolve("closed")
	}
	l.emit(protocol.Event{Kind: protocol.KindIdle, N: turn})
	for {
		select {
		case in, ok := <-l.UserInputs: // nil channel blocks — settlements still wake
			if !ok {
				// Channel closed = the user is done: graceful close.
				return true, resolve("closed")
			}
			// journal-inputs-first: journal this input, then batch-drain any
			// others that queued behind it (type-ahead) so they all enter the
			// same next turn — then resolve the idle.
			if err := l.journalInput(ds, appendE, in); err != nil {
				return false, err
			}
			if err := l.drainQueued(ds, appendE); err != nil {
				return false, err
			}
			return false, resolve("input_received")
		case out := <-l.bg.done:
			// Background work settled while idle: its outcome is a user-role
			// input (S6.1), which decide() turns into the next turn.
			if err := l.settleBackground(appendE, out); err != nil {
				return false, err
			}
			return false, resolve("work_settled")
		case handle := <-l.Cancels:
			// A user's kill at idle: journal it and cancel the handle; the
			// cancelled child settles through bg.done, which wakes the next
			// turn.
			if err := l.cancelHandle(appendE, handle); err != nil {
				return false, err
			}
			return false, resolve("child_cancelled")
		case ctl := <-l.Controls:
			// A compact/clear at idle (G7): the summarizer can't run here, so
			// stash the control and wake the loop — the safe-point drain
			// applies it, then decide() returns to standby (no turn starts).
			ds.pendingControls = append(ds.pendingControls, ctl)
			return false, resolve("control")
		case <-l.Interrupts:
			// Idle interrupt = no-op: journal the signal (audit,
			// journal-inputs-first) and keep waiting. The session never
			// ends here.
			if _, err := appendE(event.TypeInputReceived, &event.InputReceived{
				Text: "[interrupt]", Source: "interrupt",
			}); err != nil {
				return false, err
			}
			l.emit(protocol.Event{Kind: protocol.KindMessage,
				Text: "interrupt at idle: nothing to interrupt (close is a separate command)"})
			continue
		case <-ctx.Done():
			return false, context.Cause(ctx)
		}
	}
}

// awaitAnswer parks on an ask_user question (INC-5). It mirrors awaitInput,
// but the inbox reply resolves the PENDING CALL (paired as its tool result
// via AskResolved) instead of starting a fresh turn — the session then
// continues in place. Returns done=true when the run should return (headless
// park, close, or hard cancel); done=false to continue the drive loop, with
// the park either resolved (answered/interrupted) or still set (a settlement
// woke us but the question stands — decide() re-parks).
func (l *Loop) awaitAnswer(ctx context.Context, ds *driveState, appendE AppendFunc, d askDetail) (RunResult, bool, error) {
	l.ensureBackground()
	if l.UserInputs == nil && !l.inboxClosed &&
		len(ds.s.Handles) == 0 && len(ds.s.Timers) == 0 {
		// Headless (one-shot) with NOTHING in flight: no live input source. The
		// park is durable in the journal — a later `ar send` resumes and
		// answers. Return like a standby idle does (idleOrReturn), WITHOUT
		// running quiescent actions (the turn is not over; a call is still
		// open). With in-flight background/timers we do NOT early-return: the
		// select below waits on bg.done so a settlement is journaled, not
		// dropped — matching idleOrReturn/idleForInput.
		res := RunResult{Reason: "waiting_input", GenSteps: ds.s.Session.GenStep, Usage: ds.s.Session.Usage}
		l.emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: res.Reason, N: res.GenSteps})
		return res, true, nil
	}
	turn := ds.s.Session.GenStep
	l.emit(protocol.Event{Kind: protocol.KindIdle, N: turn})
	for {
		select {
		case in, ok := <-l.UserInputs:
			if !ok {
				// The user closed instead of answering: graceful close. The
				// pending call is paired interrupted so the transcript is
				// consistent, then the session closes.
				if err := l.journalAskResolved(appendE, turn, d.CallID, "interrupted", "[interrupted by user]", 0); err != nil {
					return RunResult{}, true, err
				}
				if _, err := appendE(event.TypeWaitingResolved, &event.WaitingResolved{
					Kind: event.WaitInput, Resolution: "superseded_by_interrupt",
				}); err != nil {
					return RunResult{}, true, err
				}
				l.inboxClosed = true
				l.UserInputs = nil
				res, cerr := l.closeSession(ctx, ds, appendE, turn)
				l.emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: res.Reason, N: res.GenSteps})
				return res, true, cerr
			}
			// The reply answers the question: pair the call, resolve the park.
			// The reply text is redacted like any journaled input.
			answer := redact.FromEnv().String(in.Text)
			if err := l.journalAskResolved(appendE, turn, d.CallID, "answered", answer, in.DeliverySeq); err != nil {
				return RunResult{}, true, err
			}
			if _, err := appendE(event.TypeWaitingResolved, &event.WaitingResolved{
				Kind: event.WaitInput, Resolution: "answered",
			}); err != nil {
				return RunResult{}, true, err
			}
			return RunResult{}, false, nil
		case out := <-l.bg.done:
			// Background settled while parked: record it (a user-role message
			// next turn), but the question STANDS — decide() sees Waiting still
			// set and re-parks. The settlement does not answer the question.
			if err := l.settleBackground(appendE, out); err != nil {
				return RunResult{}, true, err
			}
			return RunResult{}, false, nil
		case handle := <-l.Cancels:
			if err := l.cancelHandle(appendE, handle); err != nil {
				return RunResult{}, true, err
			}
			return RunResult{}, false, nil
		case <-l.Interrupts:
			// Interrupt while parked: the question dies interrupted and the
			// loop continues (interrupt is guidance, not shutdown). Journal
			// the signal first (journal-inputs-first), then pair + resolve.
			if _, err := appendE(event.TypeInputReceived, &event.InputReceived{
				Text: "[interrupt]", Source: "interrupt",
			}); err != nil {
				return RunResult{}, true, err
			}
			if err := l.journalAskResolved(appendE, turn, d.CallID, "interrupted", "[interrupted by user]", 0); err != nil {
				return RunResult{}, true, err
			}
			if _, err := appendE(event.TypeWaitingResolved, &event.WaitingResolved{
				Kind: event.WaitInput, Resolution: "superseded_by_interrupt",
			}); err != nil {
				return RunResult{}, true, err
			}
			return RunResult{}, false, nil
		case <-ctx.Done():
			return RunResult{}, true, context.Cause(ctx)
		}
	}
}
