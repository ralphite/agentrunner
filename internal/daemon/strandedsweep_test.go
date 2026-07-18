package daemon

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/ralphite/agentrunner/internal/protocol"
)

// INC-71 (G22a): the boot sweep resumes every mid-turn stranded session
// ScanStranded reports — a crashed daemon's sessions self-heal on the next
// boot instead of waiting for a human send.
func TestBootSweepResumesStrandedSessions(t *testing.T) {
	resumed := make(chan string, 2)
	srv := &Server{
		NewID:         func(string) string { return "x" },
		ScanStranded:  func() ([]string, error) { return []string{"s-mid-turn"}, nil },
		SessionMarked: func(string) (bool, error) { return false, nil },
		Resume: func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
			resumed <- req.SessionID
			return nil
		},
		runs: map[string]*hostedRun{},
	}
	srv.bootSweepStranded(context.Background())
	srv.runsWG.Wait()
	select {
	case id := <-resumed:
		if id != "s-mid-turn" {
			t.Fatalf("resumed %q, want s-mid-turn", id)
		}
	default:
		t.Fatal("boot sweep did not resume the stranded session")
	}
}

// 决策 #30: the stranded sweep is an AUTOMATIC path — it must not cross a
// close/kill mark. (An explicit send still lawfully revives a marked session.)
func TestBootSweepStrandedSkipsMarked(t *testing.T) {
	var calls int32
	srv := &Server{
		NewID:         func(string) string { return "x" },
		ScanStranded:  func() ([]string, error) { return []string{"marked"}, nil },
		SessionMarked: func(string) (bool, error) { return true, nil },
		Resume: func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
			atomic.AddInt32(&calls, 1)
			return nil
		},
		runs: map[string]*hostedRun{},
	}
	srv.bootSweepStranded(context.Background())
	srv.runsWG.Wait()
	if calls != 0 {
		t.Fatal("a marked session was resumed by the automatic sweep")
	}
	if len(srv.runs) != 0 {
		t.Fatal("a marked session was hosted by the automatic sweep")
	}
}

// An already-hosted session is left alone — the sweep re-uses hostResume's
// registry guard, so a live host is never doubled.
func TestBootSweepStrandedSkipsHosted(t *testing.T) {
	var calls int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	srv := &Server{
		NewID:         func(string) string { return "x" },
		ScanStranded:  func() ([]string, error) { return []string{"s1"}, nil },
		SessionMarked: func(string) (bool, error) { return false, nil },
		Resume: func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
			atomic.AddInt32(&calls, 1)
			started <- struct{}{}
			<-release
			return nil
		},
		runs: map[string]*hostedRun{},
	}
	srv.bootSweepStranded(context.Background())
	<-started
	srv.bootSweepStranded(context.Background())
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("Resume calls = %d, want 1 (re-run must not double-host)", got)
	}
	close(release)
	srv.runsWG.Wait()
}
