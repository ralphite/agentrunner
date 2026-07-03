// Package clock is the harness's only legal wall-clock outlet. Kernel,
// state, and pipeline code never call time.Now/time.Sleep (forbidigo);
// they receive a Clock from the runtime. Durable timers (2.11) and
// activity timeouts run through WaitUntil so tests fast-forward them
// with FakeClock.Advance.
package clock

import (
	"context"
	"sort"
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
	// WaitUntil blocks until the clock reaches t or ctx is done. A target
	// in the past returns immediately.
	WaitUntil(ctx context.Context, t time.Time) error
}

// Real is the production clock.
type Real struct{}

func (Real) Now() time.Time { return time.Now() }

func (Real) WaitUntil(ctx context.Context, t time.Time) error {
	d := time.Until(t)
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Fake is a manually advanced clock for tests.
type Fake struct {
	mu      sync.Mutex
	now     time.Time
	waiters []fakeWaiter
}

type fakeWaiter struct {
	at time.Time
	ch chan struct{}
}

func NewFake(start time.Time) *Fake {
	return &Fake{now: start}
}

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *Fake) WaitUntil(ctx context.Context, t time.Time) error {
	f.mu.Lock()
	if !t.After(f.now) {
		f.mu.Unlock()
		return nil
	}
	w := fakeWaiter{at: t, ch: make(chan struct{})}
	f.waiters = append(f.waiters, w)
	f.mu.Unlock()

	select {
	case <-w.ch:
		return nil
	case <-ctx.Done():
		// Remove the parked entry — a phantom waiter would corrupt
		// Waiters()-based test synchronization forever after.
		f.mu.Lock()
		for i := range f.waiters {
			if f.waiters[i].ch == w.ch {
				f.waiters = append(f.waiters[:i], f.waiters[i+1:]...)
				break
			}
		}
		f.mu.Unlock()
		return ctx.Err()
	}
}

// Advance moves the clock forward and wakes every waiter whose deadline
// has been reached, earliest first.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	var due, rest []fakeWaiter
	for _, w := range f.waiters {
		if !w.at.After(f.now) {
			due = append(due, w)
		} else {
			rest = append(rest, w)
		}
	}
	f.waiters = rest
	sort.Slice(due, func(i, j int) bool { return due[i].at.Before(due[j].at) })
	f.mu.Unlock()
	for _, w := range due {
		close(w.ch)
	}
}

// Waiters reports how many WaitUntil calls are currently parked — tests
// use it to synchronize before Advance.
func (f *Fake) Waiters() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.waiters)
}
