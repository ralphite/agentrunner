package agent

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ralphite/agentrunner/internal/command"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
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
	// Tree forwarding (INC-12.3): a Target names a descendant — this root
	// logged the command durably (daemon side) and now forwards it to the
	// member's inbox instead of consuming it. Idempotent end to end: the
	// member's inbox dedups by command id, and the CommandHandled receipt
	// keeps a daemon restart from re-waking this session for it.
	if in.Target != "" && in.Target != l.SessionID {
		return l.forwardToMember(ds, in)
	}
	var images, files []event.AttachmentRef
	var content []provider.Part
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
	putPart := func(kind provider.PartKind, mediaType string, data []byte) (provider.Part, error) {
		if err := l.ensureArtifacts(); err != nil {
			return provider.Part{}, err
		}
		ref, err := l.Artifacts.Put(data)
		if err != nil {
			return provider.Part{}, err
		}
		return provider.Part{Kind: kind, Ref: ref, MediaType: mediaType}, nil
	}
	if len(in.Content) > 0 {
		var legacyText []string
		for _, part := range in.Content {
			switch part.Kind {
			case provider.PartText:
				value := redact.FromEnv().String(part.Text)
				if len(value) > longPasteThreshold {
					stored, err := putPart(provider.PartFile, "text/plain", []byte(value))
					if err != nil {
						return err
					}
					files = append(files, event.AttachmentRef{Ref: stored.Ref, MediaType: stored.MediaType})
					head := value[:512]
					for len(head) > 0 && !utf8.ValidString(head) {
						head = head[:len(head)-1]
					}
					value = head + fmt.Sprintf("\n…[长文本已折叠为附件 %s,共 %d 字节,完整内容见 file part]", stored.Ref, len(value))
					content = append(content, provider.Part{Kind: provider.PartText, Text: value}, stored)
					legacyText = append(legacyText, value)
					continue
				}
				legacyText = append(legacyText, value)
				content = append(content, provider.Part{Kind: provider.PartText, Text: value})
			case provider.PartImage, provider.PartFile:
				stored, err := putPart(part.Kind, part.MediaType, part.Data)
				if err != nil {
					return err
				}
				content = append(content, stored)
				ref := event.AttachmentRef{Ref: stored.Ref, MediaType: stored.MediaType}
				if part.Kind == provider.PartImage {
					images = append(images, ref)
				} else {
					files = append(files, ref)
				}
			default:
				return fmt.Errorf("input content: unsupported kind %q", part.Kind)
			}
		}
		text = strings.Join(legacyText, "\n")
	}
	if len(in.Content) == 0 {
		for _, img := range in.Images {
			stored, err := putPart(provider.PartImage, img.MediaType, img.Data)
			if err != nil {
				return err
			}
			images = append(images, event.AttachmentRef{Ref: stored.Ref, MediaType: stored.MediaType})
			content = append(content, stored)
		}
		// Attached files (INC-9: PDF / any type) take the same blob-before-event
		// path as images — CAS-put the bytes, journal only the ref + real MIME.
		for _, f := range in.Files {
			stored, err := putPart(provider.PartFile, f.MediaType, f.Data)
			if err != nil {
				return err
			}
			files = append(files, event.AttachmentRef{Ref: stored.Ref, MediaType: stored.MediaType})
			content = append(content, stored)
		}
		content = append([]provider.Part{{Kind: provider.PartText, Text: text}}, content...)
	}
	// Long-paste folding (v2 M4.3): an oversized text body becomes a file
	// part plus a short head, AFTER redaction (the CAS copy must be as
	// redacted as the journal itself).
	if len(in.Content) == 0 && len(text) > longPasteThreshold {
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
		nonText := append([]provider.Part(nil), content[1:]...)
		content = []provider.Part{{Kind: provider.PartText, Text: text}, {
			Kind: provider.PartFile, Ref: ref, MediaType: "text/plain",
		}}
		content = append(content, nonText...)
	}
	inputAppend := appendE
	if in.CommandID != "" {
		inputAppend = l.commandAppender(ds, in.CommandID)
	}
	// Tree-internal messages (INC-12, send_message) arrive with
	// source="agent"; everything else defaults to a human sender. The source
	// is journal metadata — the conversation sees the sender as the text
	// prefix the sender wrote (weak-typed Input, 裁决 #9).
	source := in.Source
	if source == "" {
		source = "user"
	}
	principal := in.Principal
	if principal == "" {
		principal = "local-user"
	}
	trust := in.Trust
	if trust == "" {
		trust = "unknown"
	}
	turnID, itemID := in.TurnID, in.ItemID
	if turnID == "" {
		turnID = "turn-" + event.NewCommandID()
	}
	if itemID == "" {
		itemID = "item-" + event.NewCommandID()
	}
	_, err := inputAppend(event.TypeInputReceived, &event.InputReceived{
		Text: text, Source: source, Principal: principal, Trust: trust,
		TurnID: turnID, ItemID: itemID, Content: content, Images: images, Files: files,
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
		case in := <-l.peer:
			// A tree-internal message (INC-12) queues exactly like a user
			// one: journaled here, consumed by the next turn. A nil peer
			// channel (no router) never fires.
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
		case in := <-l.peer:
			// A tree-internal message wakes the idle exactly like a user
			// send (INC-12): journal it (plus any type-ahead) and start the
			// next turn.
			if err := l.journalInput(ds, appendE, in); err != nil {
				return false, err
			}
			if err := l.drainQueued(ds, appendE); err != nil {
				return false, err
			}
			return false, resolve("input_received")
		case sid := <-l.revive:
			// A quiescent child got mail while this session idled (INC-12.2):
			// wake, let the safe-point drain re-host it, and return to the
			// idle if nothing else follows.
			if err := l.reviveChild(ctx, ds, appendE, sid); err != nil {
				return false, err
			}
			return false, resolve("child_revived")
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
		case cmd := <-l.CommandCancels:
			if err := l.cancelDurableHandle(ds, appendE, cmd); err != nil {
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
		case ref := <-l.CommandInterrupts:
			cmdAppend := appendE
			if ref.CommandID != "" {
				cmdAppend = l.commandAppender(ds, ref.CommandID)
			}
			if _, err := cmdAppend(event.TypeInputReceived, &event.InputReceived{
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
		case in := <-l.peer:
			// A tree-internal message while parked on ask_user (INC-12): it
			// queues for the next turn but does NOT answer the question — the
			// answer must come from the user. decide() re-parks.
			if err := l.journalInput(ds, appendE, in); err != nil {
				return RunResult{}, true, err
			}
			return RunResult{}, false, nil
		case handle := <-l.Cancels:
			if err := l.cancelHandle(appendE, handle); err != nil {
				return RunResult{}, true, err
			}
			return RunResult{}, false, nil
		case cmd := <-l.CommandCancels:
			if err := l.cancelDurableHandle(ds, appendE, cmd); err != nil {
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
		case ref := <-l.CommandInterrupts:
			cmdAppend := appendE
			if ref.CommandID != "" {
				cmdAppend = l.commandAppender(ds, ref.CommandID)
			}
			if _, err := cmdAppend(event.TypeInputReceived, &event.InputReceived{
				Text: "[interrupt]", Source: "interrupt",
			}); err != nil {
				return RunResult{}, true, err
			}
			if err := l.journalAskResolved(cmdAppend, turn, d.CallID, "interrupted", "[interrupted by user]", 0); err != nil {
				return RunResult{}, true, err
			}
			if _, err := cmdAppend(event.TypeWaitingResolved, &event.WaitingResolved{
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
