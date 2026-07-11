package daemon

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/ralphite/agentrunner/internal/protocol"
)

// INC-54 (G22): the boot sweep re-hosts every drive ScanDrives reports still
// running — a cron/interval series survives a daemon restart instead of dying
// with the process.
func TestBootSweepResumesPendingDrives(t *testing.T) {
	resumed := make(chan string, 2)
	srv := &Server{
		NewID:         func(string) string { return "x" },
		ScanDrives:    func() ([]string, error) { return []string{"cron-1"}, nil },
		SessionMarked: func(string) (bool, error) { return false, nil },
		ResumeDrive: func(ctx context.Context, req DriveRequest, sink protocol.Sink) error {
			resumed <- req.SessionID
			return nil
		},
		runs: map[string]*hostedRun{},
	}
	srv.bootSweepDrives(context.Background())
	srv.runsWG.Wait()
	select {
	case id := <-resumed:
		if id != "cron-1" {
			t.Fatalf("resumed %q, want cron-1", id)
		}
	default:
		t.Fatal("boot sweep did not resume the pending drive")
	}
}

// INC-54: re-running the sweep must not double-host — a drive already live in
// the registry is left alone (the idempotency the crash-safe backfill leans
// on: at-most-one host, exactly-once slots from the journal).
func TestBootSweepSkipsHostedDrive(t *testing.T) {
	var calls int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	srv := &Server{
		NewID:         func(string) string { return "x" },
		ScanDrives:    func() ([]string, error) { return []string{"d1"}, nil },
		SessionMarked: func(string) (bool, error) { return false, nil },
		ResumeDrive: func(ctx context.Context, req DriveRequest, sink protocol.Sink) error {
			atomic.AddInt32(&calls, 1)
			started <- struct{}{}
			<-release // stay hosted so the re-run must skip it
			return nil
		},
		runs: map[string]*hostedRun{},
	}
	srv.bootSweepDrives(context.Background()) // hosts d1
	<-started                                 // d1 is now live in runs
	// The re-run's guard is synchronous: an already-hosted drive is skipped
	// without launching a second goroutine, so no second ResumeDrive can occur.
	srv.bootSweepDrives(context.Background())
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("ResumeDrive calls = %d, want 1 (re-run must not double-host)", got)
	}
	close(release)
	srv.runsWG.Wait()
}

// INC-54: an empty scan is a clean no-op — nothing hosted, nothing resumed.
func TestBootSweepNoDrivesNoSideEffect(t *testing.T) {
	var calls int32
	srv := &Server{
		NewID:      func(string) string { return "x" },
		ScanDrives: func() ([]string, error) { return nil, nil },
		ResumeDrive: func(ctx context.Context, req DriveRequest, sink protocol.Sink) error {
			atomic.AddInt32(&calls, 1)
			return nil
		},
		runs: map[string]*hostedRun{},
	}
	srv.bootSweepDrives(context.Background())
	srv.runsWG.Wait()
	if calls != 0 {
		t.Fatalf("ResumeDrive called %d times with no eligible drives", calls)
	}
	if len(srv.runs) != 0 {
		t.Fatalf("a session was hosted with no eligible drives: %v", srv.runs)
	}
}

// 决策 #30: the automatic boot sweep never crosses a close/kill mark — the same
// SessionMarked gate the timer sweep uses. (A drive's authoritative end-marker
// is its terminal DriverCompleted, which ScanDrives already excludes; this is
// the shared belt-and-suspenders门.)
func TestBootSweepSkipsMarkedDrive(t *testing.T) {
	var calls int32
	srv := &Server{
		NewID:         func(string) string { return "x" },
		ScanDrives:    func() ([]string, error) { return []string{"marked-drive"}, nil },
		SessionMarked: func(string) (bool, error) { return true, nil },
		ResumeDrive: func(ctx context.Context, req DriveRequest, sink protocol.Sink) error {
			atomic.AddInt32(&calls, 1)
			return nil
		},
		runs: map[string]*hostedRun{},
	}
	srv.bootSweepDrives(context.Background())
	srv.runsWG.Wait()
	if calls != 0 {
		t.Fatal("a marked drive was resumed by the boot sweep")
	}
	if len(srv.runs) != 0 {
		t.Fatal("a marked drive was hosted by the boot sweep")
	}
}
