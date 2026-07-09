package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/protocol"
)

// shortSock returns a unix-socket path under a SHORT temp dir. macOS caps
// sockaddr_un.sun_path at ~104 bytes, and t.TempDir() embeds the (long) test
// name — a descriptive test would otherwise fail to bind ("invalid argument").
func shortTempDir(t *testing.T) string {
	t.Helper()
	tmp, err := os.MkdirTemp("", "ar")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })
	return tmp
}

func shortSock(t *testing.T) string {
	t.Helper()
	return filepath.Join(shortTempDir(t), "d.sock")
}

// revivalHarness starts a server with a controllable Resume and SessionMarked.
func revivalHarness(t *testing.T, resume func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error,
	marked func(string) (bool, error)) (string, context.CancelFunc, chan error) {
	t.Helper()
	sock := shortSock(t)
	ctx, cancel := context.WithCancel(context.Background())
	srv := &Server{
		SocketPath:    sock,
		NewID:         func(string) string { return "x" },
		Resume:        resume,
		SessionMarked: marked,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := Dial(sock, Command{Cmd: "ping"}, func(protocol.Event) {}); err == nil {
			return sock, cancel, errCh
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("daemon never came up")
	return "", nil, nil
}

func sendCmdTo(t *testing.T, sock, sid, text string) (reply string, isErr bool) {
	t.Helper()
	err := Dial(sock, Command{Cmd: "send", Session: sid, Text: text}, func(e protocol.Event) {
		reply = e.Text
		isErr = e.Kind == protocol.KindError
	})
	if err != nil {
		t.Fatal(err)
	}
	return reply, isErr
}

// v2 收口 review P1: a send-driven revival must live on the DAEMON's
// lifecycle — graceful shutdown cancels it; runsWG.Wait must not wedge.
func TestSendRevivalDiesWithDaemon(t *testing.T) {
	entered := make(chan struct{}, 1)
	resume := func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
		entered <- struct{}{}
		<-ctx.Done() // an idle standby loop: only ctx ends it
		return ctx.Err()
	}
	marked := func(string) (bool, error) { return false, nil }
	sock, cancel, errCh := revivalHarness(t, resume, marked)

	if reply, isErr := sendCmdTo(t, sock, "idle-sess", "hi"); isErr {
		t.Fatalf("send refused: %s", reply)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("revival never started")
	}
	// Graceful shutdown must complete: the revived run's ctx is the
	// daemon's, so runsWG.Wait() returns.
	cancel()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("graceful shutdown wedged on a send-revived session")
	}
}

// 决策 #30: an explicit send REOPENS a marked session (close/kill marks
// gate automatic paths only; there is no session a send cannot continue).
func TestSendReopensMarkedSession(t *testing.T) {
	var resumed atomic.Int32
	resume := func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
		resumed.Add(1)
		if req.Inbox == nil {
			t.Error("reopen must wire an inbox")
		}
		<-ctx.Done()
		return nil
	}
	marked := func(string) (bool, error) { return true, nil } // close-marked
	sock, cancel, _ := revivalHarness(t, resume, marked)
	defer cancel()

	if reply, isErr := sendCmdTo(t, sock, "closed-sess", "hi"); isErr {
		t.Fatalf("send to marked session refused: %q", reply)
	}
	if resumed.Load() == 0 {
		t.Fatal("marked session was not reopened by the explicit send")
	}
}

// 决策 #30: the AUTOMATIC path (timer sweep) never wakes a marked session
// — the same hostResume seam that lets an explicit send through. White-box:
// the sweep is the only automatic caller, and it goes through hostResume
// with explicit=false.
func TestAutomaticResumeSkipsMarkedSession(t *testing.T) {
	srv := &Server{
		NewID: func(string) string { return "x" },
		Resume: func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
			t.Error("automatic path resumed a marked session")
			return errors.New("unreachable")
		},
		SessionMarked: func(string) (bool, error) { return true, nil },
		runs:          map[string]*hostedRun{},
	}
	srv.hostResume(context.Background(), "marked-sess", false)
	srv.runsWG.Wait()
	if len(srv.runs) != 0 {
		t.Fatal("marked session was hosted by the automatic path")
	}
}

// v2 收口 (铁律 2): with PersistInput wired, the "delivered" ack means the
// input hit the mailbox BEFORE the ack; a persist failure refuses the send.
func TestSendPersistsBeforeAck(t *testing.T) {
	persisted := make(chan protocol.UserInput, 2)
	resume := func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
		<-ctx.Done()
		return ctx.Err()
	}
	sock := shortSock(t)
	ctx, cancel := context.WithCancel(context.Background())
	srv := &Server{
		SocketPath:    sock,
		NewID:         func(string) string { return "x" },
		Resume:        resume,
		SessionMarked: func(string) (bool, error) { return false, nil },
		PersistInput: func(sid string, in protocol.UserInput) (protocol.UserInput, error) {
			if in.Text == "poison" {
				return in, errors.New("disk full")
			}
			in.DeliverySeq = 7
			persisted <- in
			return in, nil
		},
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe(ctx) }()
	defer func() { cancel(); <-errCh }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := Dial(sock, Command{Cmd: "ping"}, func(protocol.Event) {}); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if reply, isErr := sendCmdTo(t, sock, "s1", "hello"); isErr {
		t.Fatalf("send refused: %s", reply)
	}
	select {
	case in := <-persisted:
		if in.DeliverySeq != 7 || in.Text != "hello" {
			t.Fatalf("persisted = %+v", in)
		}
	default:
		t.Fatal("ack sent without persist")
	}
	if _, isErr := sendCmdTo(t, sock, "s1", "poison"); !isErr {
		t.Fatal("persist failure still acked delivered")
	}
}

// approvalHarness starts a server wired for the approval self-heal (M2):
// a real broker, a Resume that re-registers the ask like socketApprovals, and
// a controllable PendingApproval guard.
func approvalHarness(t *testing.T, broker *ApprovalBroker,
	resume func(context.Context, ResumeRequest, protocol.Sink) error,
	pending func(string) (string, bool, error)) (string, context.CancelFunc, chan error) {
	t.Helper()
	sock := shortSock(t)
	ctx, cancel := context.WithCancel(context.Background())
	srv := &Server{
		SocketPath:      sock,
		NewID:           func(string) string { return "x" },
		Resume:          resume,
		Approvals:       broker,
		SessionMarked:   func(string) (bool, error) { return false, nil },
		PendingApproval: pending,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := Dial(sock, Command{Cmd: "ping"}, func(protocol.Event) {}); err == nil {
			return sock, cancel, errCh
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("daemon never came up")
	return "", nil, nil
}

func approveVia(t *testing.T, sock, sid, id string) (reply string, isErr bool) {
	t.Helper()
	err := Dial(sock, Command{Cmd: "approve", Session: sid, ApprovalID: id, Decision: "approve"},
		func(e protocol.Event) { reply, isErr = e.Text, e.Kind == protocol.KindError })
	if err != nil {
		t.Fatal(err)
	}
	return reply, isErr
}

// M2: an approval the in-memory broker lost to a daemon restart is recovered
// by `approve` itself — it re-hosts the session (whose resumed loop re-arms
// the ask) and answers it, instead of dead-ending at "no pending approval".
func TestApproveRevivesApprovalLostToRestart(t *testing.T) {
	broker := NewApprovalBroker()
	got := make(chan ApprovalAnswer, 1)
	// The resumed loop re-registers the ask and blocks on the verdict, exactly
	// like the real socketApprovals.Resolve.
	resume := func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
		id, ch := broker.Register("stuck", "apr-1")
		a, err := broker.Wait(ctx, "stuck", id, ch)
		if err != nil {
			return err
		}
		got <- a
		<-ctx.Done()
		return ctx.Err()
	}
	sock, cancel, errCh := approvalHarness(t, broker, resume,
		func(string) (string, bool, error) { return "apr-1", true, nil })
	defer func() { cancel(); <-errCh }()

	// The ask is NOT live in the broker (the daemon "restarted"): approve must
	// revive the session and answer the re-armed ask.
	reply, isErr := approveVia(t, sock, "stuck", "apr-1")
	if isErr || reply != "answered apr-1: approve" {
		t.Fatalf("approve reply = %q isErr=%v, want answered", reply, isErr)
	}
	select {
	case a := <-got:
		if !a.Approve {
			t.Error("revived ask answered deny, want approve")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("revived loop never received the answer")
	}
}

// The self-heal is guarded: a session the journal does NOT show idle on this
// approval (wrong id, already ended) is never revived — approve reports no
// pending approval and Resume is never called.
func TestApproveDoesNotReviveNonApprovalSession(t *testing.T) {
	broker := NewApprovalBroker()
	var resumed atomic.Int32
	resume := func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
		resumed.Add(1)
		<-ctx.Done()
		return ctx.Err()
	}
	sock, cancel, errCh := approvalHarness(t, broker, resume,
		func(string) (string, bool, error) { return "", false, nil }) // not waiting on any ask
	defer func() { cancel(); <-errCh }()

	reply, isErr := approveVia(t, sock, "whatever", "apr-1")
	if !isErr {
		t.Fatalf("approve reply = %q, want an error (nothing to answer)", reply)
	}
	if resumed.Load() != 0 {
		t.Error("a non-approval session was revived by approve")
	}
}

// 决策 #32: the `agent` command releases a hosted loop (plain teardown,
// no mark) so the CLI can append SpecChanged; the next send revives under
// the new spec.
func TestAgentCommandReleasesHostedLoop(t *testing.T) {
	entered := make(chan struct{}, 1)
	resume := func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
		entered <- struct{}{}
		<-ctx.Done() // an idle standby loop
		return ctx.Err()
	}
	marked := func(string) (bool, error) { return false, nil }
	sock, cancel, _ := revivalHarness(t, resume, marked)
	defer cancel()

	// Host the session via a send-driven revival.
	if reply, isErr := sendCmdTo(t, sock, "switch-me", "hi"); isErr {
		t.Fatalf("send refused: %s", reply)
	}
	<-entered

	// The agent command tears the hosted loop down and acks the release.
	var reply string
	var isErr bool
	if err := Dial(sock, Command{Cmd: "agent", Session: "switch-me"}, func(e protocol.Event) {
		reply, isErr = e.Text, e.Kind == protocol.KindError
	}); err != nil {
		t.Fatal(err)
	}
	if isErr || reply != "released" {
		t.Fatalf("agent reply = %q isErr=%v, want released", reply, isErr)
	}

	// Not hosted anymore: a second agent command reports so.
	if err := Dial(sock, Command{Cmd: "agent", Session: "switch-me"}, func(e protocol.Event) {
		reply, isErr = e.Text, e.Kind == protocol.KindError
	}); err != nil {
		t.Fatal(err)
	}
	if isErr || reply != "not hosted" {
		t.Fatalf("second agent reply = %q isErr=%v, want not hosted", reply, isErr)
	}
}
