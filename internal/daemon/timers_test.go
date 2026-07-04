package daemon

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/protocol"
)

// timerHarness starts a server whose sweeper sees the given index; resumed
// session ids land on the resumed channel.
func timerHarness(t *testing.T, clk *clock.Fake, scan func() ([]SessionTimer, error),
	resume func(ctx context.Context, id string, sink protocol.Sink) error) *Server {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	ctx, cancel := context.WithCancel(context.Background())
	srv := &Server{
		SocketPath: sock, Clock: clk,
		NewID:      func(string) string { return "x" },
		ScanTimers: scan,
		Resume:     resume,
	}
	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
	})
	// The sweeper parks on the fake clock once it has swept.
	deadline := time.Now().Add(5 * time.Second)
	for clk.Waiters() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if clk.Waiters() == 0 {
		t.Fatal("sweeper never parked on the clock")
	}
	return srv
}

// An expired timer triggers exactly ONE hosted resume; a future timer fires
// after the clock reaches it.
func TestTimerSweepResumesExpired(t *testing.T) {
	start := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)
	clk := clock.NewFake(start)
	var mu sync.Mutex
	resumed := map[string]int{}
	release := make(chan struct{})
	resume := func(ctx context.Context, id string, sink protocol.Sink) error {
		mu.Lock()
		resumed[id]++
		mu.Unlock()
		<-release // stay "hosted" so re-sweeps must skip it
		return nil
	}
	scan := func() ([]SessionTimer, error) {
		return []SessionTimer{
			{SessionID: "past", FireAt: start.Add(-time.Minute)},
			{SessionID: "future", FireAt: start.Add(30 * time.Second)},
		}, nil
	}
	timerHarness(t, clk, scan, resume)

	// First sweep: "past" resumes immediately; "future" not yet.
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return resumed["past"] == 1 })
	mu.Lock()
	if resumed["future"] != 0 {
		mu.Unlock()
		t.Fatal("future timer fired early")
	}
	mu.Unlock()

	// Advance past the future deadline: the sweeper wakes and resumes it.
	clk.Advance(31 * time.Second)
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return resumed["future"] == 1 })

	// Let more sweeps happen: still-hosted sessions are not double-resumed.
	waitParkedD(t, clk)
	clk.Advance(2 * sweepMaxInterval)
	waitParkedD(t, clk)
	mu.Lock()
	if resumed["past"] != 1 || resumed["future"] != 1 {
		t.Fatalf("double resume: %v", resumed)
	}
	mu.Unlock()
	close(release)
}

// A resume that errors is not retried on later sweeps (needs a human).
func TestTimerSweepDoesNotRetryFailedResume(t *testing.T) {
	start := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)
	clk := clock.NewFake(start)
	var mu sync.Mutex
	attempts := 0
	resume := func(ctx context.Context, id string, sink protocol.Sink) error {
		mu.Lock()
		attempts++
		mu.Unlock()
		return errors.New("in doubt")
	}
	scan := func() ([]SessionTimer, error) {
		return []SessionTimer{{SessionID: "stuck", FireAt: start.Add(-time.Minute)}}, nil
	}
	timerHarness(t, clk, scan, resume)

	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return attempts == 1 })
	// Several more sweep rounds: no further attempts.
	for i := 0; i < 3; i++ {
		waitParkedD(t, clk)
		clk.Advance(sweepMaxInterval + time.Second)
	}
	waitParkedD(t, clk)
	mu.Lock()
	if attempts != 1 {
		t.Fatalf("failed resume retried: attempts = %d", attempts)
	}
	mu.Unlock()
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition never became true")
}

// waitParkedD waits until the sweeper is parked on the fake clock again.
func waitParkedD(t *testing.T, clk *clock.Fake) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if clk.Waiters() > 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("sweeper never re-parked")
}
