package daemon

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/protocol"
)

// revivalHarness starts a server with a controllable Resume and SessionShape.
func revivalHarness(t *testing.T, resume func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error,
	shape func(string) (bool, bool, error)) (string, context.CancelFunc, chan error) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	ctx, cancel := context.WithCancel(context.Background())
	srv := &Server{
		SocketPath:   sock,
		NewID:        func(string) string { return "x" },
		Resume:       resume,
		SessionShape: shape,
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
		<-ctx.Done() // a idle conversational loop: only ctx ends it
		return ctx.Err()
	}
	shape := func(string) (bool, bool, error) { return true, false, nil }
	sock, cancel, errCh := revivalHarness(t, resume, shape)

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

// v2 收口 review P1: a task-mode session must never false-ack a send —
// the revived hub grows no inbox, so the client hears the refusal.
func TestSendToTaskModeSessionRefused(t *testing.T) {
	var resumed atomic.Int64
	resume := func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
		resumed.Add(1)
		if req.Inbox != nil {
			t.Error("task-mode revival got an inbox")
		}
		return nil // task resume finishes on its own
	}
	shape := func(string) (bool, bool, error) { return false, false, nil } // task mode
	sock, cancel, _ := revivalHarness(t, resume, shape)
	defer cancel()

	reply, isErr := sendCmdTo(t, sock, "task-sess", "hi")
	if !isErr {
		t.Fatalf("send to task-mode session acked %q — input would be silently dropped", reply)
	}
}

// v2 收口 review: an ended session is not revivable and says so.
func TestSendToEndedSessionRefused(t *testing.T) {
	resume := func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
		t.Error("ended session must not be resumed")
		return errors.New("unreachable")
	}
	shape := func(string) (bool, bool, error) { return true, true, nil } // ended
	sock, cancel, _ := revivalHarness(t, resume, shape)
	defer cancel()

	if reply, isErr := sendCmdTo(t, sock, "ended-sess", "hi"); !isErr {
		t.Fatalf("send to ended session acked %q", reply)
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
	sock := filepath.Join(t.TempDir(), "d.sock")
	ctx, cancel := context.WithCancel(context.Background())
	srv := &Server{
		SocketPath:   sock,
		NewID:        func(string) string { return "x" },
		Resume:       resume,
		SessionShape: func(string) (bool, bool, error) { return true, false, nil },
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
