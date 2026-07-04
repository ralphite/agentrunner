package daemon

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/protocol"
)

// An ask inside a hosted run parks on the broker, surfaces on the event
// stream, and an `approve` command from a SECOND connection resolves it —
// the cross-process approval round trip.
func TestDaemonApprovalRoundTrip(t *testing.T) {
	broker := NewApprovalBroker()
	answered := make(chan ApprovalAnswer, 1)
	run := func(ctx context.Context, req RunRequest, sink protocol.Sink) error {
		sink.Emit(protocol.Event{Kind: protocol.KindApprovalRequest,
			ApprovalID: "apr-1", Tool: "bash"})
		a, err := broker.Ask(ctx, req.SessionID, "apr-1")
		if err != nil {
			return err
		}
		answered <- a
		sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: "completed"})
		return nil
	}
	sock := filepath.Join(t.TempDir(), "d.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &Server{
		SocketPath: sock, Run: run, Approvals: broker,
		NewID: func(string) string { return "sess-a" },
	}
	go func() { _ = srv.ListenAndServe(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := Dial(sock, Command{Cmd: "ping"}, func(protocol.Event) {}); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Client 1 submits and watches; it must see the ask.
	sawAsk := make(chan struct{})
	runDone := make(chan []protocol.Event, 1)
	go func() {
		var got []protocol.Event
		_ = Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Task: "risky"},
			func(e protocol.Event) {
				got = append(got, e)
				if e.Kind == protocol.KindApprovalRequest && e.ApprovalID == "apr-1" {
					close(sawAsk)
				}
			})
		runDone <- got
	}()
	select {
	case <-sawAsk:
	case <-time.After(5 * time.Second):
		t.Fatal("the ask never surfaced on the run stream")
	}

	// A wrong id is refused, the right one resolves.
	var errText string
	_ = Dial(sock, Command{Cmd: "approve", Session: "sess-a", ApprovalID: "nope", Decision: "deny"},
		func(e protocol.Event) { errText = e.Text })
	if errText == "" || e2k(errText) {
		t.Fatalf("wrong-id answer should be refused, got %q", errText)
	}
	var okText string
	_ = Dial(sock, Command{Cmd: "approve", Session: "sess-a", ApprovalID: "apr-1",
		Decision: "approve", Reason: "looks fine"},
		func(e protocol.Event) { okText = e.Text })
	if okText != "answered apr-1: approve" {
		t.Fatalf("answer ack = %q", okText)
	}

	select {
	case a := <-answered:
		if !a.Approve || a.Reason != "looks fine" {
			t.Fatalf("answer = %+v", a)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the parked ask never resolved")
	}
	select {
	case got := <-runDone:
		last := got[len(got)-1]
		if last.Kind != protocol.KindRunEnd {
			t.Fatalf("run stream end = %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run never finished after approval")
	}
}

// e2k reports whether the text looks like a success ack (guards the
// wrong-id assertion above against accidentally matching).
func e2k(s string) bool { return s == "answered nope: deny" }
