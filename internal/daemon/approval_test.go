package daemon

import (
	"bytes"
	"context"
	"encoding/json"
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
		_ = Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Prompt: "risky"},
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
	// DIFFERENT session keys (a parent and its child) sharing a
	// deterministic approval id are NOT a collision: each keeps the
	// original id and answers route by the full (session, id) key. The
	// historic global suffix here handed "#2" to whichever session
	// registered second while its journal kept the original id — on a
	// shared daemon, N concurrently-parked sessions meant one survivor
	// and N-1 permanently wedged approves (QA Round4 F-J1).
	id1, ch1 := b.Register("sess", "apr-eff-tool-call_1_0")
	id2, ch2 := b.Register("sess-sub-child-a1", "apr-eff-tool-call_1_0")
	if id1 != "apr-eff-tool-call_1_0" || id2 != "apr-eff-tool-call_1_0" {
		t.Fatalf("distinct-key registrations renamed: %q, %q", id1, id2)
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
		a, err := b.Wait(context.Background(), "sess-sub-child-a1", id2, ch2)
		res2 <- got{a, err}
	}()

	if !b.Answer("sess-sub-child-a1", id2, ApprovalAnswer{Approve: false, Reason: "second"}) {
		t.Fatal("answer to the child key was refused")
	}
	if !b.Answer("sess", id1, ApprovalAnswer{Approve: true, Reason: "first"}) {
		t.Fatal("answer to the parent key was refused")
	}
	g1 := <-res1
	g2 := <-res2
	if g1.err != nil || !g1.a.Approve || g1.a.Reason != "first" {
		t.Fatalf("waiter 1 got %+v", g1)
	}
	if g2.err != nil || g2.a.Approve || g2.a.Reason != "second" {
		t.Fatalf("waiter 2 got %+v", g2)
	}

	// A SAME-key re-register (the only true collision) still suffixes.
	id3, _ := b.Register("sess-same", "apr-1")
	if id4, _ := b.Register("sess-same", "apr-1"); id3 == id4 {
		t.Fatalf("same-key registrations share id %q", id3)
	}
}

// INC-12.6: a child approval is durable in the child's command log but is
// delivered through the root hub. The root process remains the sole host,
// while the child journal can prove the command receipt.
func TestChildApprovalRoutesThroughRootHost(t *testing.T) {
	const root = "sess-root"
	const child = root + "-sub-swe-a1"
	b := NewApprovalBroker()
	s := &Server{Approvals: b, runs: map[string]*hostedRun{}, failed: map[string]bool{}}
	h := s.newHostedRun(root, true)
	defer h.finish()
	s.runs[root] = h
	var persistedSession string
	s.PersistCommand = func(session string, cmd protocol.SessionCommand) (protocol.SessionCommand, error) {
		persistedSession = session
		cmd.CommandSeq = 4
		return cmd, nil
	}
	s.PendingApproval = func(session string) (string, bool, error) {
		return "apr-child", session == child, nil
	}
	_, waiting := b.Register(child, "apr-child")

	var reply bytes.Buffer
	s.handleApprove(context.Background(), Command{
		Session: child, ApprovalID: "apr-child", Decision: "approve", CommandID: "cmd-child-approval",
	}, json.NewEncoder(&reply))
	select {
	case answer := <-waiting:
		if !answer.Approve || answer.CommandID != "cmd-child-approval" || answer.CommandSeq != 4 {
			t.Fatalf("child approval answer = %+v", answer)
		}
	case <-time.After(time.Second):
		t.Fatal("root host never delivered the child approval")
	}
	if persistedSession != child {
		t.Fatalf("approval persisted to %q, want child log %q", persistedSession, child)
	}
	var ack protocol.Event
	if err := json.Unmarshal(reply.Bytes(), &ack); err != nil {
		t.Fatal(err)
	}
	if ack.Kind != protocol.KindMessage || ack.Session != child {
		t.Fatalf("ack = %+v", ack)
	}
}

// Two SESSIONS parking on the same deterministic approval id must both keep
// the ORIGINAL id: the historic global suffix de-dupe handed the second one
// "#2" in the broker while its journal kept the original, so the surfaced
// `approve` command could never match (QA Round4 F-J1). Uniqueness now
// lives on the full (session, id) key; a same-key re-register still
// suffixes.
func TestRegisterKeepsIDAcrossSessions(t *testing.T) {
	b := NewApprovalBroker()
	idA, chA := b.Register("sess-a", "apr-eff-tool-call_1_0")
	idB, chB := b.Register("sess-b", "apr-eff-tool-call_1_0")
	if idA != "apr-eff-tool-call_1_0" || idB != "apr-eff-tool-call_1_0" {
		t.Fatalf("cross-session ids = %q, %q; want both original", idA, idB)
	}
	if !b.Answer("sess-b", idB, ApprovalAnswer{Approve: true}) {
		t.Fatal("answer for sess-b did not match")
	}
	select {
	case a := <-chB:
		if !a.Approve {
			t.Fatal("sess-b got the wrong answer")
		}
	default:
		t.Fatal("sess-b channel empty")
	}
	select {
	case <-chA:
		t.Fatal("sess-a must not receive sess-b's answer")
	default:
	}
	// Same-key collision still de-dupes with a suffix.
	if id2, _ := b.Register("sess-a", "apr-eff-tool-call_1_0"); id2 != "apr-eff-tool-call_1_0#2" {
		t.Fatalf("same-key re-register = %q, want #2 suffix", id2)
	}
}
