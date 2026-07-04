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
	// Progress is the optional ephemeral channel seam (S4 deltas, S6 task
	// tails). Never journaled; unused in S2.
	Progress func(delta string)
	// PostRun runs after a successful Run, before the terminal event; its
	// return value lands in ActivityCompleted.hook_note (3.8 post hooks).
	PostRun func(ctx context.Context, result json.RawMessage, isError bool) string
}

// ActivityExecutor is the single path every side effect takes (2.10).
type ActivityExecutor struct {
	Append AppendFunc
	Clock  clock.Clock
	Redact *redact.Redactor
	// MaxAttempts/Backoff default to 3 attempts with 1s/4s waits.
	MaxAttempts int
	Backoff     []time.Duration
	// DiscardOnRetry is the S4 TurnDiscarded seam: called before each
	// retry so the caller can journal a discard mark. Nil in S2.
	DiscardOnRetry func() error
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
			if _, aerr := x.Append(event.TypeActivityCancelled, &event.ActivityCancelled{
				ActivityID:    act.ID,
				PartialOutput: string(x.Redact.JSON(result)),
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
		if _, aerr := x.Append(event.TypeActivityFailed, &event.ActivityFailed{
			ActivityID: act.ID,
			Attempt:    attempt,
			Error: event.ErrorInfo{
				Class:     string(class),
				Message:   x.Redact.String(err.Error()),
				Retryable: class.Retryable(),
			},
		}); aerr != nil {
			return aerr
		}

		if !class.Retryable() || attempt >= maxAttempts {
			return err
		}
		if x.DiscardOnRetry != nil {
			if derr := x.DiscardOnRetry(); derr != nil {
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

	runCtx, cancelRun := context.WithCancelCause(ctx)
	defer cancelRun(nil)
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
