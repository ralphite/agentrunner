package clock

import (
	"context"
	"runtime"
	"testing"
	"time"
)

var epoch = time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)

func TestFakeAdvanceWakesDueWaiter(t *testing.T) {
	f := NewFake(epoch)
	done := make(chan error, 1)
	go func() {
		done <- f.WaitUntil(context.Background(), epoch.Add(10*time.Minute))
	}()
	for f.Waiters() == 0 {
		runtime.Gosched() // spin until the waiter parks
	}
	f.Advance(9 * time.Minute)
	select {
	case err := <-done:
		t.Fatalf("woke early: %v (now=%s)", err, f.Now())
	default:
	}
	f.Advance(2 * time.Minute)
	if err := <-done; err != nil {
		t.Fatalf("WaitUntil = %v", err)
	}
	if got := f.Now(); !got.Equal(epoch.Add(11 * time.Minute)) {
		t.Errorf("now = %s", got)
	}
}

// A two-day approval hang (the 3.5 scenario) is a single Advance.
func TestFakeAdvanceTwoDays(t *testing.T) {
	f := NewFake(epoch)
	done := make(chan error, 1)
	go func() {
		done <- f.WaitUntil(context.Background(), epoch.Add(48*time.Hour))
	}()
	for f.Waiters() == 0 {
		runtime.Gosched()
	}
	f.Advance(48 * time.Hour)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestFakePastTargetReturnsImmediately(t *testing.T) {
	f := NewFake(epoch)
	if err := f.WaitUntil(context.Background(), epoch.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := f.WaitUntil(context.Background(), epoch); err != nil {
		t.Fatal(err)
	}
}

func TestFakeWaitCancellable(t *testing.T) {
	f := NewFake(epoch)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- f.WaitUntil(ctx, epoch.Add(time.Hour))
	}()
	for f.Waiters() == 0 {
		runtime.Gosched()
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestRealPastTargetNoSleep(t *testing.T) {
	r := Real{}
	start := r.Now()
	if err := r.WaitUntil(context.Background(), start.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if r.Now().Sub(start) > time.Second {
		t.Error("past target should not block")
	}
}

func TestRealWaitCancellable(t *testing.T) {
	r := Real{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := r.WaitUntil(ctx, r.Now().Add(time.Hour))
	if err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// A ctx-canceled WaitUntil must remove its parked entry — phantom waiters
// corrupt Waiters()-based synchronization.
func TestFakeCancelRemovesWaiter(t *testing.T) {
	f := NewFake(epoch)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.WaitUntil(ctx, epoch.Add(time.Hour)) }()
	for f.Waiters() == 0 {
		runtime.Gosched()
	}
	cancel()
	<-done
	for i := 0; f.Waiters() != 0 && i < 1e6; i++ {
		runtime.Gosched()
	}
	if got := f.Waiters(); got != 0 {
		t.Fatalf("waiters after cancel = %d, want 0", got)
	}
}
