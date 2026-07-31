package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
)

// AppendFunc journals one event AND folds it into the caller's state —
// the loop owns both, the executor never touches the store directly.
type AppendFunc func(typ string, payload any) (event.Envelope, error)

// Activity describes one side-effecting unit: Started is journaled before
// execution, a terminal event after (the in-doubt window between the two
// is exactly what 2.15 surfaces on resume).
type Activity struct {
	ID         string // deterministic: llm-t<turn> | tool-<call_id>
	Kind       string // event.KindLLM | event.KindTool
	Name       string
	Args       json.RawMessage
	CallID     string
	Idempotent bool
	// Timeout arms a durable timer for each attempt (2.11): TimerSet is
	// journaled, and on fire the run ctx is canceled with cause
	// errs.ErrActivityTimeout. Zero means no timeout.
	Timeout time.Duration
	// Run performs the effect: (result, usage, isError, err). isError is a
	// model-visible failed result (tool_failed) — the activity SUCCEEDED;
	// err is an activity failure fed to the retry policy.
	Run func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error)
	// Progress is the optional ephemeral channel seam (S4 deltas, S6 background-work
	// tails). Never journaled; unused in S2.
	Progress func(delta string)
	// PostRun runs after a successful Run, before the terminal event; its
	// return value lands in ActivityCompleted.hook_note (3.8 post hooks).
	PostRun func(ctx context.Context, result json.RawMessage, isError bool) string
	// DiscardOnRetry (S4.1) runs before each retry — the LLM activity uses
	// it to journal GenerationDiscarded and signal the surface to reopen the
	// stream when deltas were already emitted.
	DiscardOnRetry func() error
	// Convert opts the activity into timeout-to-background (S6.2): the
	// timer firing journals ActivityBackgrounded and hands the still-running
	// attempt over instead of killing it. nil keeps the kill semantics.
	Convert *ConvertSpec
}

// ErrBackgrounded is Do's report that the attempt converted to background
// work instead of finishing (S6.2 timeout-to-background): no terminal event
// was journaled — the background settle path owns that — and the call is
// already paired with its handle placeholder in the fold. Callers treat it
// as "the batch is done with this call", never as a failure.
var ErrBackgrounded = errors.New("activity converted to background work")

// ConvertOutcome is a converted attempt's eventual terminal report,
// delivered on the channel ConvertSpec.HandOff received; the background
// runtime forwards it to the settle path, which journals the terminal the
// executor deliberately did not.
type ConvertOutcome struct {
	Result   json.RawMessage
	Usage    *provider.Usage
	IsError  bool
	Err      error
	Canceled bool
}

// ConvertSpec configures timeout-to-background for one activity (S6.2).
type ConvertSpec struct {
	// Base is the context the run derives from INSTEAD of the ctx passed to
	// Do — the run's lifetime, so converted work survives the batch. While
	// the attempt is still foreground the batch ctx cancels it all the same
	// (a steering interrupt kills as before, cause preserved); after the
	// conversion the work answers only to Base and the handle's kill.
	Base context.Context
	// Notice is journaled into ActivityBackgrounded and rendered into the
	// handle placeholder the model sees.
	Notice string
	// HandOff transfers the running attempt at conversion time: cancel
	// reaches the run (the handle's kill path), outcome yields its terminal
	// report. Called on the activity goroutine, exactly once.
	HandOff func(cancel context.CancelCauseFunc, outcome <-chan ConvertOutcome)
}

// ActivityExecutor is the single path every side effect takes (2.10).
type ActivityExecutor struct {
	Append AppendFunc
	Clock  clock.Clock
	Redact *redact.Redactor
	// MaxAttempts/Backoff default to 3 attempts with 1s/4s waits.
	MaxAttempts int
	Backoff     []time.Duration
}

// Do runs the activity: Started → execute → terminal, retrying retryable
// failures with backoff through the Clock. Args and results pass through
// credential redaction before journaling.
func (x *ActivityExecutor) Do(ctx context.Context, act Activity) error {
	maxAttempts := x.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 3
	}
	backoff := x.Backoff
	if backoff == nil {
		backoff = []time.Duration{time.Second, 4 * time.Second}
	}

	for attempt := 1; ; attempt++ {
		if _, err := x.Append(event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: act.ID,
			Kind:       act.Kind,
			Name:       act.Name,
			Args:       x.Redact.JSON(act.Args),
			CallID:     act.CallID,
			Idempotent: act.Idempotent,
			Attempt:    attempt,
		}); err != nil {
			return err
		}

		result, usage, isError, err, timedOut := x.runAttempt(ctx, act, attempt)
		if errors.Is(err, ErrBackgrounded) {
			// The attempt converted to background work (S6.2): the journal
			// holds Started + ActivityBackgrounded, the terminal settles
			// later through the background path. No terminal here, no retry
			// — report the conversion to the caller.
			return err
		}
		if timedOut && err != nil && isCancellation(err) {
			// The run surfaced OUR cancellation as an error: the true class
			// is timeout (retryable), not canceled. Errors that do not
			// descend from the cancellation (a 401 racing the timer, a
			// store failure) keep their own class — stamping them
			// retryable would retry the unretryable.
			err = errs.Wrap(errs.Timeout, err, "activity timeout")
		}
		if !timedOut && ctx.Err() != nil {
			// Canceled from above (2.12). The effect implementation has
			// already killed its process group and drained (bounded); the
			// terminal fact is ActivityCancelled with whatever partial
			// output survived — journaled only now, after the group died.
			// Usage the run managed to report settles (a steered child run's
			// spend is real, S5 review).
			if _, aerr := x.Append(event.TypeActivityCancelled, &event.ActivityCancelled{
				ActivityID:    act.ID,
				PartialOutput: string(x.Redact.JSON(result)),
				Usage:         usage,
			}); aerr != nil {
				return aerr
			}
			return errs.Wrap(errs.Canceled, context.Cause(ctx), act.Name)
		}
		if err == nil {
			var note string
			if act.PostRun != nil {
				note = act.PostRun(ctx, result, isError)
			}
			crash.Point(crash.PointAfterExecBeforeJournal)
			_, aerr := x.Append(event.TypeActivityCompleted, &event.ActivityCompleted{
				ActivityID: act.ID,
				Result:     x.Redact.JSON(result),
				Usage:      usage,
				IsError:    isError,
				HookNote:   x.Redact.String(note),
			})
			return aerr
		}

		class := errs.ClassOf(err)
		final := !class.Retryable() || attempt >= maxAttempts
		if _, aerr := x.Append(event.TypeActivityFailed, &event.ActivityFailed{
			ActivityID: act.ID,
			Attempt:    attempt,
			Error: event.ErrorInfo{
				Class:     string(class),
				Message:   x.Redact.String(err.Error()),
				Retryable: class.Retryable(),
			},
			Final: final,
		}); aerr != nil {
			return aerr
		}

		if final {
			return err
		}
		if act.DiscardOnRetry != nil {
			if derr := act.DiscardOnRetry(); derr != nil {
				return derr
			}
		}
		wait := backoff[min(attempt-1, len(backoff)-1)]
		if werr := x.Clock.WaitUntil(ctx, x.Clock.Now().Add(wait)); werr != nil {
			return werr
		}
	}
}

// runAttempt executes one attempt, racing it against the durable timeout
// timer when armed. All journal appends stay on this goroutine; the timer
// waiter only signals a channel. Returns timedOut=true when the timer
// fired before the run finished.
func (x *ActivityExecutor) runAttempt(ctx context.Context, act Activity, attempt int) (json.RawMessage, *provider.Usage, bool, error, bool) {
	if act.Timeout <= 0 {
		r, u, ie, err := act.Run(ctx)
		return r, u, ie, err, false
	}

	timerID := fmt.Sprintf("tm-%s-a%d", act.ID, attempt)
	fireAt := x.Clock.Now().Add(act.Timeout)
	if _, err := x.Append(event.TypeTimerSet, &event.TimerSet{
		TimerID: timerID, FireAt: fireAt, Purpose: "activity_timeout:" + act.ID,
	}); err != nil {
		return nil, nil, false, err, false
	}

	// A convertible attempt (S6.2) derives its run ctx from Convert.Base —
	// the run's lifetime — so the work survives the batch after a handoff.
	// While it is still foreground, the batch ctx cancels it all the same:
	// the watcher propagates the cancellation WITH its cause (a steering
	// interrupt's short kill grace depends on it) and is detached on return,
	// which is exactly the handoff boundary.
	base := ctx
	if act.Convert != nil && act.Convert.Base != nil {
		base = act.Convert.Base
	}
	handedOff := false
	runCtx, cancelRun := context.WithCancelCause(base)
	defer func() {
		if !handedOff {
			cancelRun(nil)
		}
	}()
	if base != ctx {
		stop := context.AfterFunc(ctx, func() { cancelRun(context.Cause(ctx)) })
		defer stop()
	}
	waitCtx, cancelWait := context.WithCancel(ctx)
	defer cancelWait()

	fired := make(chan struct{}, 1)
	go func() {
		if x.Clock.WaitUntil(waitCtx, fireAt) == nil {
			fired <- struct{}{}
		}
	}()

	type outcome struct {
		result  json.RawMessage
		usage   *provider.Usage
		isError bool
		err     error
	}
	outc := make(chan outcome, 1)
	go func() {
		r, u, ie, err := act.Run(runCtx)
		outc <- outcome{r, u, ie, err}
	}()

	select {
	case out := <-outc:
		cancelWait()
		if _, err := x.Append(event.TypeTimerCancelled, &event.TimerCancelled{TimerID: timerID}); err != nil {
			return nil, nil, false, err, false
		}
		return out.result, out.usage, out.isError, out.err, false
	case <-fired:
		if _, err := x.Append(event.TypeTimerFired, &event.TimerFired{TimerID: timerID}); err != nil {
			// Store failure, not a timeout: drain the run (cancelRun via
			// defer) and surface the append error with its own class.
			cancelRun(errs.ErrActivityTimeout)
			<-outc
			return nil, nil, false, err, false
		}
		if act.Convert != nil {
			// Timeout-to-background (S6.2): the window elapsing converts
			// the attempt instead of killing it. Journal the conversion
			// (the fold pairs the call with its handle placeholder on this
			// very append), hand the running attempt — its cancel and its
			// eventual outcome — to the background runtime, and return
			// without a terminal event. A journal that cannot record the
			// conversion falls back to the kill: the work must not outlive
			// the record of why it is still running.
			if _, err := x.Append(event.TypeActivityBackgrounded, &event.ActivityBackgrounded{
				ActivityID: act.ID, Notice: act.Convert.Notice,
			}); err != nil {
				cancelRun(errs.ErrActivityTimeout)
				<-outc
				return nil, nil, false, err, false
			}
			handedOff = true
			conv := make(chan ConvertOutcome, 1)
			go func() {
				out := <-outc
				conv <- ConvertOutcome{Result: out.result, Usage: out.usage,
					IsError: out.isError, Err: out.err, Canceled: runCtx.Err() != nil}
			}()
			act.Convert.HandOff(cancelRun, conv)
			return nil, nil, false, ErrBackgrounded, false
		}
		cancelRun(errs.ErrActivityTimeout)
		out := <-outc // bounded drain: effect impls kill their process groups on cancel
		return out.result, out.usage, out.isError, out.err, true
	}
}

// isCancellation reports whether err descends from a context cancellation
// (which is how our timeout reaches the run).
func isCancellation(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, errs.ErrActivityTimeout)
}
