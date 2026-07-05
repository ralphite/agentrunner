package agent

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/tool"
)

// bgOutcome is one background task's terminal report, produced on the task
// goroutine and SETTLED (journaled) only on the drive goroutine — the fold
// stays single-writer; the channel is the sole crossing point (S6.1).
type bgOutcome struct {
	taskID     string
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

// bgRuntime is the loop's ephemeral background-task machinery. Runtime
// state only: the DURABLE truth is the tasks sub-state folded from
// ActivityStarted{Background} and the terminal events.
type bgRuntime struct {
	mu     sync.Mutex
	cancel map[string]context.CancelFunc
	done   chan bgOutcome
}

func (l *Loop) ensureBackground() {
	if l.bg == nil {
		l.bg = &bgRuntime{cancel: map[string]context.CancelFunc{}, done: make(chan bgOutcome, 64)}
	}
}

// isBackgroundCall reports whether a tool call asks for task-style
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
// call with its handle and adds the task) and starts the task goroutine.
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
	taskCtx, cancel := context.WithCancel(ctx)
	l.bg.mu.Lock()
	l.bg.cancel[callID] = cancel
	l.bg.mu.Unlock()

	go func() {
		res := l.Exec.Execute(taskCtx, name, args)
		l.bg.done <- bgOutcome{
			taskID: callID, activityID: activityID,
			result: res.Payload, isError: res.IsError,
			canceled: taskCtx.Err() != nil,
		}
	}()
	return nil
}

// drainBackground settles every already-finished task without blocking.
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

// awaitBackground blocks until ONE task finishes (or an interrupt/cancel),
// then settles it, returning the WaitingResolved resolution (WaitRules
// vocabulary). The WAITING_TASKS idle (2.14): no pending calls, no new
// input, tasks in flight. An interrupt cancels every task — the user wants
// the run back; the cancellations settle through the same channel.
func (l *Loop) awaitBackground(ctx context.Context, appendE AppendFunc, turn int) (string, error) {
	l.ensureBackground()
	select {
	case out := <-l.bg.done:
		return "tasks_done", l.settleBackground(appendE, out)
	case <-l.Interrupts:
		if err := l.onSteeringInterrupt(appendE, turn); err != nil {
			return "", err
		}
		l.cancelAllBackground()
		// re-decide: tasks still nonempty → idle again → drain the cancellations
		return "tasks_cancelled_by_interrupt", nil
	case <-ctx.Done():
		return "", context.Cause(ctx)
	}
}

// settleBackground journals a finished task's terminal event; the fold
// removes it from tasks and renders the outcome as a user-role input.
func (l *Loop) settleBackground(appendE AppendFunc, out bgOutcome) error {
	l.bg.mu.Lock()
	delete(l.bg.cancel, out.taskID)
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
				Tool: "task", CallID: out.taskID,
				Result: compact(out.result), IsError: out.isError})
		}
		return err
	}
}

// drainCancels non-blockingly fires the cancel for every handle requested on
// the Cancels channel (v2 M3.2). An unknown handle is a no-op (the task may
// have already settled). The cancelled child/task settles through bg.done.
func (l *Loop) drainCancels(appendE AppendFunc) error {
	if l.Cancels == nil {
		return nil
	}
	for {
		select {
		case handle := <-l.Cancels:
			if err := l.cancelHandle(appendE, handle); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

// cancelHandle journals the user's kill as a control input (journal-inputs-
// first, DESIGN v2 §2 — the durable origin of the cancellation, same
// discipline as interrupts), then fires the handle's cancel if still live.
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
		cancel()
	}
	return nil
}

// cancelAllBackground fires every live task's cancel; terminals settle
// through the done channel.
func (l *Loop) cancelAllBackground() {
	if l.bg == nil {
		return
	}
	l.bg.mu.Lock()
	defer l.bg.mu.Unlock()
	for _, cancel := range l.bg.cancel {
		cancel()
	}
}

// quiesceTasks fills the epilogue quiesce slot (S6.1, 钩子 2): at a run
// ending, still-running background tasks are awaited or cancelled per the
// spec's on_run_end (default cancel), and every terminal settles BEFORE the
// terminal receipt — the log never ends with tasks in flight. The await
// wait is BOUNDED by a durable timer (S7 还债, DESIGN: await 必有 durable
// timer 兜底): the TimerSet fact makes a crashed-while-awaiting session
// visible to the daemon sweep, and an expired timer cancels the stragglers
// instead of awaiting forever.
func quiesceTasks(ctx context.Context, l *Loop, ds *driveState,
	appendE AppendFunc, _ *string) error {

	if l.bg == nil || len(ds.s.Tasks) == 0 {
		return nil
	}
	await := l.Spec != nil && l.Spec.OnRunEnd == "await"
	if !await {
		l.cancelAllBackground()
	}
	var timeoutCh chan struct{}
	const timerID = "tm-await-quiesce"
	if await {
		fireAt := l.Clock.Now().Add(l.Spec.awaitTimeout())
		if _, err := appendE(event.TypeTimerSet, &event.TimerSet{
			TimerID: timerID, FireAt: fireAt, Purpose: "await_quiesce",
		}); err != nil {
			return err
		}
		timeoutCh = make(chan struct{})
		tctx, tcancel := context.WithCancel(ctx)
		defer tcancel()
		go func() {
			if l.Clock.WaitUntil(tctx, fireAt) == nil {
				close(timeoutCh)
			}
		}()
	}
	fired := false
	for len(ds.s.Tasks) > 0 {
		select {
		case out := <-l.bg.done:
			if err := l.settleBackground(appendE, out); err != nil {
				return err
			}
		case <-timeoutCh:
			// The await bound expired: journal the firing, cancel what is
			// left, and keep draining — the cancellations settle normally.
			if _, err := appendE(event.TypeTimerFired, &event.TimerFired{TimerID: timerID}); err != nil {
				return err
			}
			fired = true
			timeoutCh = nil
			l.cancelAllBackground()
		case <-ctx.Done():
			// Hard cancel while quiescing: cancel everything, then BLOCK on
			// the next report — the killed tasks always report (killGroup
			// guarantees bash exits), and a default-branch here would spin
			// the CPU until they do (S6 review).
			l.cancelAllBackground()
			out := <-l.bg.done
			if err := l.settleBackground(appendE, out); err != nil {
				return err
			}
		}
	}
	if await && !fired {
		if _, err := appendE(event.TypeTimerCancelled, &event.TimerCancelled{TimerID: timerID}); err != nil {
			return err
		}
	}
	return nil
}

// runTaskTool executes task_output / task_kill against a fold snapshot of
// the task set (taken on the drive goroutine — the closure runs on an
// activity goroutine). Model-visible in every failure mode.
func (l *Loop) runTaskTool(tasks state.Tasks, name string, rawArgs json.RawMessage) tool.Result {
	var args struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.TaskID == "" {
		return errorResult(name + ": invalid args: need {\"task_id\"}")
	}
	if _, running := tasks[args.TaskID]; !running {
		return errorResult(name + ": no running task " + args.TaskID +
			" (a finished task's result arrived as a message)")
	}
	switch name {
	case "task_output":
		// v0: bash collects output at completion — the honest answer for a
		// running task is its status. A live tail arrives with streaming
		// tools (记档: 2.10 进度通道对 bash 未接).
		payload, _ := json.Marshal(map[string]string{
			"task_id": args.TaskID, "status": "running",
			"note": "output arrives as a message when the task finishes",
		})
		return tool.Result{Payload: payload}
	case "task_kill":
		l.bg.mu.Lock()
		cancel, ok := l.bg.cancel[args.TaskID]
		l.bg.mu.Unlock()
		if ok {
			cancel()
		}
		payload, _ := json.Marshal(map[string]string{
			"task_id": args.TaskID, "status": "cancelling",
			"note": "the cancellation notice arrives as a message",
		})
		return tool.Result{Payload: payload}
	default:
		return errorResult("unknown task tool " + name)
	}
}

// isTaskTool reports the task-management tools (advertised alongside bash).
func isTaskTool(name string) bool {
	return name == "task_output" || name == "task_kill"
}
