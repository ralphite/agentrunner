package daemon

// QA v2sim L2-I1: a send accepted while its host was tearing down ("delivered"
// already acked, command log durable) used to go dormant — the pump fed it
// into an inbox nobody reads again, nothing re-hosted the session, and the
// user's message silently never started its turn until the NEXT daemon boot's
// pending-command sweep. The teardown defer now re-checks the pending suffix
// and re-hosts iff an unconsumed user input is there: a dangling message
// simply becomes the next turn.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/protocol"
)

func waitCount(t *testing.T, c *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for c.Load() < want && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if c.Load() < want {
		t.Fatalf("resume calls = %d, want >= %d", c.Load(), want)
	}
}

// A host that ends while an unconsumed user input sits in the command log
// must be re-hosted so that input becomes the next turn (the L2-I1 dead
// window). Without the teardown re-check this hangs at one resume call.
func TestTeardownDanglingInputRehosts(t *testing.T) {
	var resumes, scans atomic.Int32
	in := protocol.UserInput{Text: "砍掉这一路", Source: "unix-socket"}
	srv := &Server{
		Resume: func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
			resumes.Add(1)
			return nil // the host ends immediately — the teardown window itself
		},
		SessionMarked: func(string) (bool, error) { return false, nil },
		PendingCommands: func(string) ([]protocol.SessionCommand, error) {
			// The dangling input is pending until the second host consumes it.
			if scans.Add(1) <= 2 { // initial replay + teardown re-check
				return []protocol.SessionCommand{{
					CommandRef: protocol.CommandRef{CommandID: "cmd-dangling"},
					Kind:       protocol.CommandInput, Input: &in,
				}}, nil
			}
			return nil, nil
		},
		runs: map[string]*hostedRun{},
	}
	srv.hostResume(context.Background(), "s1", true)
	waitCount(t, &resumes, 2)
	srv.runsWG.Wait()
}

// Only a pending user INPUT re-hosts: an undeliverable approval answer (or
// any other stale command kind) must not resurrect the session in a loop.
func TestTeardownPendingApprovalDoesNotRehost(t *testing.T) {
	var resumes atomic.Int32
	srv := &Server{
		Resume: func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
			resumes.Add(1)
			return nil
		},
		SessionMarked: func(string) (bool, error) { return false, nil },
		PendingCommands: func(string) ([]protocol.SessionCommand, error) {
			return []protocol.SessionCommand{{
				CommandRef: protocol.CommandRef{CommandID: "cmd-stale-approval"},
				Kind:       protocol.CommandApproval,
				Approval:   &protocol.ApprovalCommand{ApprovalID: "apr-x", Decision: "approve"},
			}}, nil
		},
		runs: map[string]*hostedRun{},
	}
	srv.hostResume(context.Background(), "s2", true)
	srv.runsWG.Wait()
	time.Sleep(50 * time.Millisecond) // would-be rehost window
	if got := resumes.Load(); got != 1 {
		t.Fatalf("resume calls = %d, want exactly 1 (approval must not rehost)", got)
	}
}

// A failed resume marks the session (s.failed); the teardown re-check must
// not retry-storm it — the explicit send path stays the recovery gesture.
func TestTeardownRehostSkipsFailedResume(t *testing.T) {
	var resumes atomic.Int32
	in := protocol.UserInput{Text: "hello", Source: "unix-socket"}
	srv := &Server{
		Resume: func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
			resumes.Add(1)
			return errors.New("journal unreadable")
		},
		SessionMarked: func(string) (bool, error) { return false, nil },
		PendingCommands: func(string) ([]protocol.SessionCommand, error) {
			return []protocol.SessionCommand{{
				CommandRef: protocol.CommandRef{CommandID: "cmd-dangling"},
				Kind:       protocol.CommandInput, Input: &in,
			}}, nil
		},
		runs: map[string]*hostedRun{},
	}
	srv.hostResume(context.Background(), "s3", true)
	srv.runsWG.Wait()
	time.Sleep(50 * time.Millisecond)
	if got := resumes.Load(); got != 1 {
		t.Fatalf("resume calls = %d, want exactly 1 (failed resume must not retry-storm)", got)
	}
}
