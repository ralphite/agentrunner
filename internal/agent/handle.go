package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/tool"
)

// bgOutcome is one background work's terminal report, produced on the worker
// goroutine and SETTLED (journaled) only on the drive goroutine — the fold
// stays single-writer; the channel is the sole crossing point (S6.1).
type bgOutcome struct {
	handle     string
	activityID string
	result     json.RawMessage
	isError    bool
	err        error
	canceled   bool
	// subagent is set for a BACKGROUND SPAWN (v2 M3.1): settling it journals
	// SubagentCompleted (provenance + tree-budget usage) alongside the
	// activity terminal that renders the child's report as a user message.
	subagent *event.SubagentCompleted
	usage    *provider.Usage
}

// bgRuntime is the loop's ephemeral background-work machinery. Runtime
// state only: the DURABLE truth is the handles sub-state folded from
// ActivityStarted{Background} and the terminal events. Cancels carry a
// CAUSE (决策 #30): an explicit kill records who asked (errs.KilledError),
// so a killed child journals its mark; teardown cancels carry none.
type bgRuntime struct {
	mu     sync.Mutex
	cancel map[string]context.CancelCauseFunc
	logs   map[string]*bgLog
	done   chan bgOutcome
}

func (l *Loop) ensureBackground() {
	if l.bg == nil {
		l.bg = &bgRuntime{cancel: map[string]context.CancelCauseFunc{}, logs: map[string]*bgLog{}, done: make(chan bgOutcome, 64)}
	}
}

// bgLog is a running background work's live output tail (2.10 进度通道,
// audit-0717 B9). Bounded and ephemeral by design: the journal's durable
// record stays the completion result; this exists so `output` can answer a
// running handle with real progress instead of only "running".
type bgLog struct {
	mu    sync.Mutex
	tail  []byte
	total int
}

const bgLogTailCap = 16 * 1024

func (b *bgLog) append(chunk []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.total += len(chunk)
	b.tail = append(b.tail, chunk...)
	if over := len(b.tail) - bgLogTailCap; over > 0 {
		b.tail = append(b.tail[:0:0], b.tail[over:]...)
	}
}

func (b *bgLog) snapshot() (tail string, total int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.tail), b.total
}

// isBackgroundCall reports whether a tool call asks for background
// execution (S6.1): bash with args.background == true. Only bash supports
// it today — the schema advertises the flag there and nowhere else.
func isBackgroundCall(name string, rawArgs json.RawMessage) bool {
	if name != "bash" {
		return false
	}
	var args struct {
		Background bool `json:"background"`
	}
	_ = json.Unmarshal(rawArgs, &args)
	return args.Background
}

// launchBackground journals ActivityStarted{Background} (the fold pairs the
// call with its handle and adds the work item) and starts its goroutine.
// Runs on the drive goroutine; appendE may be the batch-serialized appender.
func (l *Loop) launchBackground(ctx context.Context, appendE AppendFunc,
	callID, name string, args json.RawMessage) error {

	l.ensureBackground()
	activityID := "tool-" + callID
	if _, err := appendE(event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: activityID, Kind: event.KindTool, Name: name,
		Args: redact.FromEnv().JSON(args), CallID: callID,
		Attempt: 1, Background: true,
	}); err != nil {
		return err
	}
	workCtx, cancel := context.WithCancelCause(ctx)
	log := &bgLog{}
	l.bg.mu.Lock()
	l.bg.cancel[callID] = cancel
	l.bg.logs[callID] = log
	l.bg.mu.Unlock()

	// Progress tail (B9): chunks land in the bounded live log for `output`
	// and mirror to the surface as ephemeral bg_output events — the same
	// journal-free doctrine as token deltas. Redaction runs per chunk: the
	// live path must never show what the completion result would redact.
	workCtx = tool.WithLiveOutput(workCtx, func(chunk []byte) {
		log.append(chunk)
		l.emit(protocol.Event{Kind: protocol.KindBgOutput, CallID: callID,
			Text: redact.FromEnv().String(string(chunk))})
	})

	go func() {
		res := l.Exec.Execute(workCtx, name, args)
		l.bg.done <- bgOutcome{
			handle: callID, activityID: activityID,
			result: res.Payload, isError: res.IsError,
			canceled: workCtx.Err() != nil,
		}
	}()
	return nil
}

// drainBackground settles every already-finished work without blocking.
// Drive-goroutine only.
func (l *Loop) drainBackground(appendE AppendFunc) error {
	if l.bg == nil {
		return nil
	}
	for {
		select {
		case out := <-l.bg.done:
			if err := l.settleBackground(appendE, out); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

// settleBackground journals a finished work's terminal event; the fold
// removes it from the live handle set and renders the outcome as a user-role input.
func (l *Loop) settleBackground(appendE AppendFunc, out bgOutcome) error {
	l.bg.mu.Lock()
	delete(l.bg.cancel, out.handle)
	delete(l.bg.logs, out.handle) // the completion result supersedes the live tail
	l.bg.mu.Unlock()

	// A background spawn also journals SubagentCompleted (v2 M3.1): tree-budget
	// usage + the child-stream provenance the barrier vector reads. It rides
	// BEFORE the activity terminal so a crash between them still leaves a
	// coherent "child done" fact; the activity terminal below settles the
	// reservation and renders the child's report as a user message.
	if out.subagent != nil {
		if _, err := appendE(event.TypeSubagentCompleted, out.subagent); err != nil {
			return err
		}
		l.fireLifecycle(context.Background(), hook.EventSubagentStop, map[string]string{
			"agent": out.subagent.Agent, "child_session": out.subagent.ChildSession,
			"reason": out.subagent.Reason}, false)
		// Quiescence race close-out (INC-12.2): mail that landed while the
		// child was on its way out (delivered to a port nobody was reading)
		// is durable in its inbox — queue the revive now, after the receipt.
		// ONLY when this round actually CONSUMED the mailbox (INC-12 review
		// P1/P2): a Resume that errored BEFORE consuming (reason "error" —
		// in-doubt, MCP drift, version mismatch) or a canceled child did not,
		// so re-enqueuing would hot-loop (re-host → fail → close-out → …). A
		// child that ran to quiescence (completed OR contract_violation) DID
		// consume it, so a still-pending inbox reflects genuinely NEW mail
		// worth waking. A failed member's mail stays durable for the next
		// restart scan / explicit send once the root cause is fixed.
		if reviveConsumedMailbox(out) && l.Router != nil && l.revive != nil && l.childHasMail(out.subagent.ChildSession) {
			select {
			case l.revive <- out.subagent.ChildSession:
			default:
				slog.Warn("revive queue full at settle; child mail deferred to next resume",
					"child", out.subagent.ChildSession)
			}
		}
	}

	switch {
	case out.canceled:
		_, err := appendE(event.TypeActivityCancelled, &event.ActivityCancelled{
			ActivityID:    out.activityID,
			PartialOutput: string(redact.FromEnv().JSON(out.result)),
			Usage:         out.usage,
		})
		return err
	case out.err != nil:
		class := errs.ClassOf(out.err)
		_, err := appendE(event.TypeActivityFailed, &event.ActivityFailed{
			ActivityID: out.activityID, Attempt: 1, Final: true,
			Error: event.ErrorInfo{Class: string(class),
				Message: redact.FromEnv().String(out.err.Error())},
		})
		return err
	default:
		_, err := appendE(event.TypeActivityCompleted, &event.ActivityCompleted{
			ActivityID: out.activityID,
			Result:     redact.FromEnv().JSON(out.result),
			IsError:    out.isError,
			Usage:      out.usage,
		})
		if err == nil {
			l.emit(protocol.Event{Kind: protocol.KindToolResult,
				Tool: "handle", CallID: out.handle,
				Result: compact(out.result), IsError: out.isError})
		}
		return err
	}
}

// reviveConsumedMailbox reports whether a settled revive actually consumed
// its mailbox this round (INC-12 review P1/P2). A Resume that errored before
// consuming (reason "error") or a canceled child did not — re-enqueuing them
// would hot-loop. A child that ran to quiescence (completed / contract_violation)
// consumed its mailbox, so a still-pending inbox is genuinely new mail.
func reviveConsumedMailbox(out bgOutcome) bool {
	if out.canceled || out.subagent == nil {
		return false
	}
	return out.subagent.Reason != "error"
}

// drainCancels non-blockingly fires the cancel for every handle requested on
// the Cancels channel (v2 M3.2). An unknown handle is a no-op (the work may
// have already settled). The cancelled child/background work settles through bg.done.
func (l *Loop) drainCancels(ds *driveState, appendE AppendFunc) error {
	if l.Cancels == nil && l.CommandCancels == nil {
		return nil
	}
	for {
		select {
		case handle := <-l.Cancels:
			if err := l.cancelHandle(appendE, handle); err != nil {
				return err
			}
		case cmd := <-l.CommandCancels:
			if err := l.cancelDurableHandle(ds, appendE, cmd); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func (l *Loop) cancelDurableHandle(ds *driveState, appendE AppendFunc, cmd protocol.CancelCommand) error {
	// Journal the audit intent first, then fire the ephemeral cancel, and
	// only then record completion. A crash on either side replays the command;
	// cancellation is idempotent and an accepted kill is never lost.
	if err := l.cancelHandle(appendE, cmd.Handle); err != nil {
		return err
	}
	if cmd.CommandID == "" {
		return nil
	}
	cmdAppend := l.commandAppender(ds, cmd.CommandID)
	_, err := cmdAppend(event.TypeCommandHandled, &event.CommandHandled{
		CommandID: cmd.CommandID, CommandSeq: cmd.CommandSeq,
		Kind: protocol.CommandKill, Result: "cancel_requested",
	})
	return err
}

// cancelHandle journals the user's kill as a control input (journal-inputs-
// first, DESIGN v2 §2 — the durable origin of the cancellation, same
// discipline as interrupts), then fires the handle's cancel if still live.
// The cause records the USER as the kill origin (决策 #30 裁决二): a
// killed child journals SessionClosed{killed, source:"user"} from it.
func (l *Loop) cancelHandle(appendE AppendFunc, handle string) error {
	if _, err := appendE(event.TypeInputReceived, &event.InputReceived{
		Text: "[kill " + handle + "]", Source: "control",
	}); err != nil {
		return err
	}
	if l.bg == nil {
		return nil
	}
	l.bg.mu.Lock()
	cancel, ok := l.bg.cancel[handle]
	l.bg.mu.Unlock()
	if ok {
		cancel(&errs.KilledError{Source: "user"})
	}
	return nil
}

// cancelAllBackground fires every live background work's cancel with the given cause
// (nil = teardown, no mark); terminals settle through the done channel.
func (l *Loop) cancelAllBackground(cause error) {
	if l.bg == nil {
		return
	}
	l.bg.mu.Lock()
	defer l.bg.mu.Unlock()
	for _, cancel := range l.bg.cancel {
		cancel(cause)
	}
}

// settleOnAbort settles in-flight background work on a dying execution —
// close, kill, teardown or harness error: cancel everything (no kill
// cause: the dying run tears down, it does not kill on anyone's behalf)
// and drain the terminals so the journal never ends with orphans. The
// drain counts LIVE background work (the cancel registry), not the fold's handle
// set: a handle whose goroutine never started (a failed revive, a crash
// artifact awaiting settle-from-child-fold) must not wedge the drain.
// Best effort by nature: journal failures stop the drain, nothing more
// can be done.
func (l *Loop) settleOnAbort(ctx context.Context, ds *driveState, appendE AppendFunc) {
	if l.bg == nil || len(ds.s.Handles) == 0 {
		return
	}
	l.cancelAllBackground(nil)
	live := func() int {
		l.bg.mu.Lock()
		defer l.bg.mu.Unlock()
		return len(l.bg.cancel)
	}
	for live() > 0 {
		out := <-l.bg.done
		if err := l.settleBackground(appendE, out); err != nil {
			return
		}
	}
}

// runHandleTool executes output / kill against a fold snapshot of
// the handle set (taken on the drive goroutine — the closure runs on an
// activity goroutine). Model-visible in every failure mode.
func (l *Loop) runHandleTool(handles state.Handles, name string, rawArgs json.RawMessage) tool.Result {
	var args struct {
		Handle string `json:"handle"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Handle == "" {
		return errorResult(name + ": invalid args: need {\"handle\"}")
	}
	if _, running := handles[args.Handle]; !running {
		return errorResult(name + ": no running background work " + args.Handle +
			" (a finished result arrived as a message)")
	}
	switch name {
	case "output":
		// Progress tail (2.10 进度通道, audit-0717 B9): a running work
		// answers with its live output so far — bounded, already redacted
		// chunk-wise on the way in. The completion message stays the full,
		// durable record.
		out := map[string]any{
			"handle": args.Handle, "status": "running",
			"note": "tail of the output so far; the full result arrives as a message when the work finishes",
		}
		l.bg.mu.Lock()
		log := l.bg.logs[args.Handle]
		l.bg.mu.Unlock()
		if log != nil {
			tail, total := log.snapshot()
			out["output_tail"] = redact.FromEnv().String(tail)
			out["output_bytes_total"] = total
		}
		payload, _ := json.Marshal(out)
		return tool.Result{Payload: payload}
	case "kill":
		l.bg.mu.Lock()
		cancel, ok := l.bg.cancel[args.Handle]
		l.bg.mu.Unlock()
		if ok {
			// The parent model asked: record it (裁决二 — a parent-killed
			// child may be revived by the parent, a user-killed one only by
			// the user).
			cancel(&errs.KilledError{Source: "parent"})
		}
		payload, _ := json.Marshal(map[string]string{
			"handle": args.Handle, "status": "cancelling",
			"note": "the cancellation notice arrives as a message",
		})
		return tool.Result{Payload: payload}
	default:
		return errorResult("unknown background-work tool " + name)
	}
}

// isHandleTool reports background-work management tools.
func isHandleTool(name string) bool {
	return name == "output" || name == "kill"
}
