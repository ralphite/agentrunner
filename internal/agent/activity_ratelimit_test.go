package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
)

// drainBackoffs advances the fake clock past每 pending backoff wait until the
// activity finishes, so a test never depends on wall time.
func drainBackoffs(t *testing.T, f *clock.Fake, done <-chan error, step time.Duration) error {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case err := <-done:
			return err
		case <-deadline:
			t.Fatal("activity did not finish")
			return nil
		default:
		}
		if f.Waiters() > 0 {
			f.Advance(step)
		}
		time.Sleep(time.Millisecond)
	}
}

// A rate limit outlives the ordinary 3-attempt budget: the harness keeps
// waiting the quota out instead of failing the turn. This is the whole point
// of the separate policy — a 429 is a "come back later", not a glitch.
func TestRateLimitRetriesPastOrdinaryBudget(t *testing.T) {
	m := &memAppend{}
	f := clock.NewFake(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC))
	x := testExecutor(m)
	x.Clock = f

	attempts := 0
	done := make(chan error, 1)
	go func() {
		done <- x.Do(context.Background(), Activity{
			ID: "llm-t1", Kind: event.KindLLM, Name: "complete", Idempotent: true,
			Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
				attempts++
				if attempts < 9 { // far past the 3-attempt transient budget
					return nil, nil, false, errs.New(errs.ProviderRateLimit, "429")
				}
				return json.RawMessage(`{"ok":true}`), nil, false, nil
			},
		})
	}()

	if err := drainBackoffs(t, f, done, 10*time.Minute); err != nil {
		t.Fatalf("rate limit must be waited out, not surfaced: %v", err)
	}
	if attempts != 9 {
		t.Fatalf("attempts = %d, want 9", attempts)
	}
	if last := m.types()[len(m.types())-1]; last != "activity_completed" {
		t.Fatalf("last event = %s, want activity_completed", last)
	}
}

// A non-rate-limit transient class keeps the tight 3-attempt budget: server
// errors are glitches, and retrying one for an hour helps nobody.
func TestServerErrorKeepsShortBudget(t *testing.T) {
	m := &memAppend{}
	f := clock.NewFake(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC))
	x := testExecutor(m)
	x.Clock = f

	attempts := 0
	done := make(chan error, 1)
	go func() {
		done <- x.Do(context.Background(), Activity{
			ID: "llm-t1", Kind: event.KindLLM, Name: "complete",
			Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
				attempts++
				return nil, nil, false, errs.New(errs.ProviderServer, "503")
			},
		})
	}()

	if err := drainBackoffs(t, f, done, time.Minute); err == nil {
		t.Fatal("exhausted server-error retries must surface")
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3 (the transient budget is unchanged)", attempts)
	}
}

// The provider's own Retry-After wins whenever it is longer than our curve:
// the service knows when its window reopens, and coming back early can
// extend the penalty.
func TestRateLimitHonorsProviderRetryAfter(t *testing.T) {
	x := &ActivityExecutor{}
	wait, again := x.retryPlan(errs.ProviderRateLimit, 1, 90*time.Second)
	if !again {
		t.Fatal("rate limit must retry")
	}
	if wait != 90*time.Second {
		t.Fatalf("wait = %s, want the provider's 90s (curve would say 5s)", wait)
	}
	// A hint SHORTER than the curve does not shorten the wait: a provider
	// repeating "1s" on an exhausted daily quota must not become a hot loop.
	wait, _ = x.retryPlan(errs.ProviderRateLimit, 5, time.Second)
	if wait != 80*time.Second {
		t.Fatalf("wait = %s, want the curve's 80s", wait)
	}
}

// The curve caps instead of growing without bound.
func TestRateLimitBackoffCaps(t *testing.T) {
	x := &ActivityExecutor{}
	for _, attempt := range []int{7, 20, 500} {
		wait, again := x.retryPlan(errs.ProviderRateLimit, attempt, 0)
		if !again {
			t.Fatalf("attempt %d must still retry (unbounded by default)", attempt)
		}
		if wait != 5*time.Minute {
			t.Fatalf("attempt %d wait = %s, want the 5m cap", attempt, wait)
		}
	}
}

// MaxAttempts:1 opts out of retrying entirely — including rate limits. The
// autotitle path relies on this: a background nicety must never hold a
// session open waiting for a quota.
func TestMaxAttemptsOneOptsOutOfRateLimitRetry(t *testing.T) {
	x := &ActivityExecutor{MaxAttempts: 1}
	if _, again := x.retryPlan(errs.ProviderRateLimit, 1, 0); again {
		t.Fatal("MaxAttempts:1 must not retry a rate limit")
	}
}

// RateLimitMaxAttempts caps the wait when a caller wants one.
func TestRateLimitMaxAttemptsCap(t *testing.T) {
	x := &ActivityExecutor{RateLimitMaxAttempts: 2}
	if _, again := x.retryPlan(errs.ProviderRateLimit, 1, 0); !again {
		t.Fatal("attempt 1 must retry")
	}
	if _, again := x.retryPlan(errs.ProviderRateLimit, 2, 0); again {
		t.Fatal("attempt 2 must be final under a cap of 2")
	}
}

// Jitter only ever lengthens a wait. Sibling agents hit the same wall in the
// same instant; spreading them is the point, but never at the cost of coming
// back sooner than the provider allowed.
func TestJitterOnlyLengthens(t *testing.T) {
	x := &ActivityExecutor{Jitter: func() float64 { return 0.99 }}
	wait, _ := x.retryPlan(errs.ProviderRateLimit, 1, 0)
	if wait < 5*time.Second {
		t.Fatalf("wait = %s, must never fall below the 5s curve entry", wait)
	}
	if wait > 7*time.Second {
		t.Fatalf("wait = %s, jitter must stay within 25%%", wait)
	}
	withHint, _ := x.retryPlan(errs.ProviderRateLimit, 1, 60*time.Second)
	if withHint < 60*time.Second {
		t.Fatalf("wait = %s, must never undercut the provider's 60s", withHint)
	}
}

// A non-final failure records WHEN the next attempt lands, so a reader can
// tell a four-second blip from a five-minute quota wait.
func TestRetryAtJournaled(t *testing.T) {
	m := &memAppend{}
	f := clock.NewFake(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC))
	x := testExecutor(m)
	x.Clock = f

	done := make(chan error, 1)
	go func() {
		done <- x.Do(context.Background(), Activity{
			ID: "llm-t1", Kind: event.KindLLM, Name: "complete",
			Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
				return nil, nil, false, errs.New(errs.ProviderRateLimit, "429")
			},
		})
	}()
	// Let the first attempt fail and park in its backoff.
	for i := 0; i < 500 && f.Waiters() == 0; i++ {
		time.Sleep(time.Millisecond)
	}

	var failed event.ActivityFailed
	found := false
	for _, e := range m.events {
		if e.Type == event.TypeActivityFailed {
			if err := json.Unmarshal(e.Payload, &failed); err != nil {
				t.Fatal(err)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no activity_failed journaled")
	}
	if failed.Final {
		t.Fatal("a rate limit must not be final")
	}
	if want := f.Now().Add(5 * time.Second); !failed.RetryAt.Equal(want) {
		t.Fatalf("retry_at = %s, want %s", failed.RetryAt, want)
	}

	// Do not leave the goroutine parked in an unbounded retry.
	_ = drainBackoffs
}

// A retry notice reaches the surface before the wait, so a five-minute
// backoff does not read as a hung turn.
func TestOnRetryFiresBeforeWaiting(t *testing.T) {
	m := &memAppend{}
	f := clock.NewFake(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC))
	x := testExecutor(m)
	x.Clock = f
	notices := make(chan RetryNotice, 8)
	x.OnRetry = func(n RetryNotice) { notices <- n }

	go func() {
		_ = x.Do(context.Background(), Activity{
			ID: "llm-t7", Kind: event.KindLLM, Name: "complete", Turn: 7,
			Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
				return nil, nil, false, errs.New(errs.ProviderRateLimit, "429")
			},
		})
	}()

	select {
	case n := <-notices:
		if n.Class != errs.ProviderRateLimit {
			t.Fatalf("class = %s", n.Class)
		}
		if n.Turn != 7 {
			t.Fatalf("turn = %d, want 7", n.Turn)
		}
		if n.Wait != 5*time.Second {
			t.Fatalf("wait = %s, want 5s", n.Wait)
		}
		if n.Attempt != 1 {
			t.Fatalf("attempt = %d, want 1", n.Attempt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no retry notice reached the surface")
	}
}

// Cancelling during a backoff wait is an ORDINARY path now that waits run to
// minutes: it must journal the same terminal a cancel during the attempt
// would, so the fold is not left carrying a phantom in-flight activity.
func TestCancelDuringBackoffJournalsCancelled(t *testing.T) {
	m := &memAppend{}
	f := clock.NewFake(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC))
	x := testExecutor(m)
	x.Clock = f

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- x.Do(ctx, Activity{
			ID: "llm-t1", Kind: event.KindLLM, Name: "complete",
			Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
				return nil, nil, false, errs.New(errs.ProviderRateLimit, "429")
			},
		})
	}()
	for i := 0; i < 500 && f.Waiters() == 0; i++ {
		time.Sleep(time.Millisecond)
	}
	cancel()

	select {
	case err := <-done:
		if errs.ClassOf(err) != errs.Canceled {
			t.Fatalf("class = %s, want canceled", errs.ClassOf(err))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not cut through the backoff wait")
	}
	if last := m.types()[len(m.types())-1]; last != "activity_cancelled" {
		t.Fatalf("last event = %s, want activity_cancelled (events: %v)", last, m.types())
	}
}
