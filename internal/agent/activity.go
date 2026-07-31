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
	// Turn tags the generation step this activity belongs to, so a retry
	// notice lands under the right turn on the surface. Passed explicitly
	// rather than read off the fold: tool activities run on worker
	// goroutines, and the fold belongs to the drive goroutine.
	Turn int
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
	// MaxAttempts/Backoff default to 3 attempts with 1s/4s waits. They
	// govern the TRANSIENT classes (server errors, timeouts) — a rate limit
	// gets its own, far more patient policy below. MaxAttempts == 1 opts an
	// activity out of retrying entirely, rate limits included.
	MaxAttempts int
	Backoff     []time.Duration
	// RateLimitBackoff is the curve for provider_rate_limit. It exists
	// separately because a rate limit is not a transient glitch: the service
	// is telling us WHEN to come back, and 5 seconds of trying is an answer
	// to a different question. The last entry repeats, so the curve caps
	// rather than growing without bound.
	RateLimitBackoff []time.Duration
	// RateLimitMaxAttempts caps rate-limit retries; 0 means unlimited —
	// wait the quota out rather than failing a turn the user will only have
	// to re-send by hand. A rate-limited attempt costs no tokens, so an
	// unbounded wait cannot run away with the budget, and cancel/kill/
	// interrupt still cut through it (the wait honors ctx).
	RateLimitMaxAttempts int
	// Jitter returns a fraction in [0,1) used to spread synchronized
	// retries. A fleet of sibling agents hits the same quota wall in the
	// same instant; without jitter they would also come back in the same
	// instant, re-colliding forever. nil = no jitter (tests want exact
	// waits). Only ever lengthens a wait, never shortens it below what the
	// provider asked for.
	Jitter func() float64
	// OnRetry reports a scheduled retry to the surface. Without it a
	// long rate-limit wait is indistinguishable from a hung turn: the
	// stream simply goes quiet for minutes.
	OnRetry func(RetryNotice)
}

// RetryNotice is one scheduled retry, for the surface to render.
type RetryNotice struct {
	ActivityID string
	Kind       string
	Name       string
	Turn       int
	Attempt    int // the attempt that just failed
	Class      errs.Class
	Wait       time.Duration
	Err        error
}

// defaultRateLimitBackoff caps at 5 minutes: long enough that waiting out a
// daily quota costs a trivial number of probes, short enough that a
// per-minute window reopening is noticed promptly.
var defaultRateLimitBackoff = []time.Duration{
	5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second,
	80 * time.Second, 160 * time.Second, 5 * time.Minute,
}

// retryPlan decides whether a failed attempt gets another go, and how long
// to wait first. hint is the provider's own Retry-After, when it gave one.
func (x *ActivityExecutor) retryPlan(class errs.Class, attempt int, hint time.Duration) (time.Duration, bool) {
	if !class.Retryable() {
		return 0, false
	}
	maxAttempts := x.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 3
	}
	if maxAttempts == 1 {
		return 0, false // explicit opt-out; applies to every class
	}

	if class == errs.ProviderRateLimit {
		if x.RateLimitMaxAttempts > 0 && attempt >= x.RateLimitMaxAttempts {
			return 0, false
		}
		curve := x.RateLimitBackoff
		if curve == nil {
			curve = defaultRateLimitBackoff
		}
		wait := curve[min(attempt-1, len(curve)-1)]
		// The provider knows its own window; never come back sooner than it
		// asked. Our curve is the backstop for a service that says nothing,
		// or says something too optimistic to keep repeating.
		if hint > wait {
			wait = hint
		}
		if x.Jitter != nil {
			wait += time.Duration(x.Jitter() * 0.25 * float64(wait))
		}
		return wait, true
	}

	if attempt >= maxAttempts {
		return 0, false
	}
	backoff := x.Backoff
	if backoff == nil {
		backoff = []time.Duration{time.Second, 4 * time.Second}
	}
	return backoff[min(attempt-1, len(backoff)-1)], true
}

// Do runs the activity: Started → execute → terminal, retrying retryable
// failures with backoff through the Clock. Args and results pass through
// credential redaction before journaling.
func (x *ActivityExecutor) Do(ctx context.Context, act Activity) error {
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
		wait, again := x.retryPlan(class, attempt, errs.RetryAfter(err))
		final := !again
		failed := &event.ActivityFailed{
			ActivityID: act.ID,
			Attempt:    attempt,
			Error: event.ErrorInfo{
				Class:     string(class),
				Message:   x.Redact.String(err.Error()),
				Retryable: class.Retryable(),
			},
			Final: final,
		}
		if again {
			// Journal WHEN the next attempt lands, not just that there is
			// one: a reader of the log (or a UI replaying it) can then tell a
			// four-second blip from a five-minute quota wait.
			failed.RetryAt = x.Clock.Now().Add(wait)
		}
		if _, aerr := x.Append(event.TypeActivityFailed, failed); aerr != nil {
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
		if x.OnRetry != nil {
			x.OnRetry(RetryNotice{
				ActivityID: act.ID, Kind: act.Kind, Name: act.Name, Turn: act.Turn,
				Attempt: attempt, Class: class, Wait: wait, Err: err,
			})
		}
		if werr := x.Clock.WaitUntil(ctx, x.Clock.Now().Add(wait)); werr != nil {
			// Interrupted mid-backoff. A rate-limit wait runs to minutes, so
			// this is an ordinary way for a kill or a steering interrupt to
			// land — not a rare race. Journal the same terminal a cancel
			// during the ATTEMPT would have (ActivityFailed{non-final}
			// deliberately leaves the entry in flight, state.go), so the fold
			// is not left carrying a phantom in-flight activity.
			if _, aerr := x.Append(event.TypeActivityCancelled, &event.ActivityCancelled{
				ActivityID: act.ID,
			}); aerr != nil {
				return aerr
			}
			return errs.Wrap(errs.Canceled, context.Cause(ctx), act.Name)
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
