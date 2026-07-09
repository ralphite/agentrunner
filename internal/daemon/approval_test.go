package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/protocol"
)

// An ask inside a hosted run goes idle on the broker, surfaces on the event
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
	sock := shortSock(t)
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
		t.Fatal("the idle ask never resolved")
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

// S6 review: two concurrent sibling asks with IDENTICAL deterministic ids
// must both be individually addressable — the second registration gets a
// suffixed id, and each answer reaches its own waiter.
func TestApprovalBrokerCollision(t *testing.T) {
	b := NewApprovalBroker()
	id1, ch1 := b.Register("sess", "apr-eff-tool-call_1_0")
	id2, ch2 := b.Register("sess", "apr-eff-tool-call_1_0")
	if id1 == id2 {
		t.Fatalf("colliding registrations share id %q", id1)
	}

	type got struct {
		a   ApprovalAnswer
		err error
	}
	res1 := make(chan got, 1)
	res2 := make(chan got, 1)
	go func() {
		a, err := b.Wait(context.Background(), "sess", id1, ch1)
		res1 <- got{a, err}
	}()
	go func() {
		a, err := b.Wait(context.Background(), "sess", id2, ch2)
		res2 <- got{a, err}
	}()

	if !b.Answer("sess", id2, ApprovalAnswer{Approve: false, Reason: "second"}) {
		t.Fatal("answer to the suffixed id was refused")
	}
	if !b.Answer("sess", id1, ApprovalAnswer{Approve: true, Reason: "first"}) {
		t.Fatal("answer to the original id was refused")
	}
	g1 := <-res1
	g2 := <-res2
	if g1.err != nil || !g1.a.Approve || g1.a.Reason != "first" {
		t.Fatalf("waiter 1 got %+v", g1)
	}
	if g2.err != nil || g2.a.Approve || g2.a.Reason != "second" {
		t.Fatalf("waiter 2 got %+v", g2)
	}
}
