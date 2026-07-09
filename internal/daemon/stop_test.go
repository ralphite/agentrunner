package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/protocol"
)

// stop tears a hosted run down (ctx cancel) and acks "stopping"; it leaves
// NO mark, so a later send revives the session (G12).
func TestStopTearsDownHostedRun(t *testing.T) {
	entered := make(chan struct{}, 1)
	returned := make(chan struct{}, 1)
	resume := func(ctx context.Context, _ ResumeRequest, _ protocol.Sink) error {
		entered <- struct{}{}
		<-ctx.Done() // an idle standby loop
		returned <- struct{}{}
		return ctx.Err()
	}
	marked := func(string) (bool, error) { return false, nil }
	sock, cancel, _ := revivalHarness(t, resume, marked)
	defer cancel()

	if reply, isErr := sendCmdTo(t, sock, "stop-me", "hi"); isErr {
		t.Fatalf("send refused: %s", reply)
	}
	<-entered

	var reply string
	var isErr bool
	if err := Dial(sock, Command{Cmd: "stop", Session: "stop-me"}, func(e protocol.Event) {
		reply, isErr = e.Text, e.Kind == protocol.KindError
	}); err != nil {
		t.Fatal(err)
	}
	if isErr || reply != "stopping" {
		t.Fatalf("stop reply = %q isErr=%v, want stopping", reply, isErr)
	}

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("hosted run not torn down after stop")
	}

	// No mark: a second stop finds no live run (teardown, not close).
	if err := Dial(sock, Command{Cmd: "stop", Session: "stop-me"}, func(e protocol.Event) {
		reply, isErr = e.Text, e.Kind == protocol.KindError
	}); err != nil {
		t.Fatal(err)
	}
	if !isErr {
		t.Fatalf("second stop should report no live run, got %q", reply)
	}
}

func TestStopUnknownSession(t *testing.T) {
	resume := func(ctx context.Context, _ ResumeRequest, _ protocol.Sink) error { return nil }
	marked := func(string) (bool, error) { return false, nil }
	sock, cancel, _ := revivalHarness(t, resume, marked)
	defer cancel()

	var reply string
	var isErr bool
	if err := Dial(sock, Command{Cmd: "stop", Session: "ghost"}, func(e protocol.Event) {
		reply, isErr = e.Text, e.Kind == protocol.KindError
	}); err != nil {
		t.Fatal(err)
	}
	if !isErr {
		t.Fatalf("stop on an unknown session should error, got %q", reply)
	}
}

// stop leaves no mark, so `send` lawfully revives the session afterward.
func TestStopThenSendRevives(t *testing.T) {
	entered := make(chan struct{}, 2)
	returned := make(chan struct{}, 1)
	resume := func(ctx context.Context, _ ResumeRequest, _ protocol.Sink) error {
		entered <- struct{}{}
		<-ctx.Done()
		returned <- struct{}{}
		return ctx.Err()
	}
	marked := func(string) (bool, error) { return false, nil }
	sock, cancel, _ := revivalHarness(t, resume, marked)
	defer cancel()

	sendCmdTo(t, sock, "revive-me", "hi")
	<-entered
	if err := Dial(sock, Command{Cmd: "stop", Session: "revive-me"}, func(protocol.Event) {}); err != nil {
		t.Fatal(err)
	}
	<-returned // teardown done; the run registry removal follows in a defer

	// Retry send until revival re-enters resume (removal races the defer).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		sendCmdTo(t, sock, "revive-me", "again")
		select {
		case <-entered:
			return // revived — no mark blocked it
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatal("send did not revive the session after stop")
}

// A drive series was previously unstoppable (it ran on the raw daemon ctx);
// stop now cancels its per-run context too (G12 handleDrive fix).
func TestStopTearsDownDriveSeries(t *testing.T) {
	entered := make(chan struct{}, 1)
	returned := make(chan struct{}, 1)
	drive := func(ctx context.Context, _ DriveRequest, _ protocol.Sink) error {
		entered <- struct{}{}
		<-ctx.Done()
		returned <- struct{}{}
		return ctx.Err()
	}
	sock := shortSock(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &Server{SocketPath: sock, NewID: func(string) string { return "drv" }, Drive: drive}
	go func() { _ = srv.ListenAndServe(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := Dial(sock, Command{Cmd: "ping"}, func(protocol.Event) {}); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// drive streams until the series ends — dial it in the background.
	go func() { _ = Dial(sock, Command{Cmd: "drive", SpecPath: "x.yaml"}, func(protocol.Event) {}) }()
	<-entered

	var reply string
	var isErr bool
	if err := Dial(sock, Command{Cmd: "stop", Session: "drv"}, func(e protocol.Event) {
		reply, isErr = e.Text, e.Kind == protocol.KindError
	}); err != nil {
		t.Fatal(err)
	}
	if isErr || reply != "stopping" {
		t.Fatalf("stop reply = %q isErr=%v, want stopping", reply, isErr)
	}
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("drive series not torn down by stop (per-run cancel missing)")
	}
}
