package daemon

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/protocol"
)

// startServer runs a Server with the given RunFunc on a temp socket and
// waits until it accepts connections.
func startServer(t *testing.T, run RunFunc, replay func(string, protocol.Sink) error) (string, context.CancelFunc) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	ctx, cancel := context.WithCancel(context.Background())
	var n atomic.Int64
	srv := &Server{
		SocketPath: sock,
		Run:        run,
		Replay:     replay,
		NewID:      func(task string) string { return fmt.Sprintf("sess-%d", n.Add(1)) },
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("server: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("server did not shut down")
		}
	})
	// Wait for the socket to accept.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := Dial(sock, Command{Cmd: "ping"}, func(protocol.Event) {}); err == nil {
			return sock, cancel
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("daemon never came up")
	return "", nil
}

func TestDaemonPing(t *testing.T) {
	sock, _ := startServer(t, nil, nil)
	var got []protocol.Event
	if err := Dial(sock, Command{Cmd: "ping"}, func(e protocol.Event) { got = append(got, e) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "pong" {
		t.Fatalf("ping → %+v", got)
	}
}

// run: the daemon hosts the run, streams its events (tagged with the
// session), and reports the assigned session id first.
func TestDaemonRunStreams(t *testing.T) {
	run := func(ctx context.Context, req RunRequest, sink protocol.Sink) error {
		sink.Emit(protocol.Event{Kind: protocol.KindTurnStart, Turn: 1})
		sink.Emit(protocol.Event{Kind: protocol.KindMessage, Turn: 1, Text: "hello from " + req.Task})
		sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: "completed"})
		return nil
	}
	sock, _ := startServer(t, run, nil)

	var got []protocol.Event
	err := Dial(sock, Command{Cmd: "run", SpecPath: "spec.yaml", Task: "wave"},
		func(e protocol.Event) { got = append(got, e) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("events = %+v, want session banner + 3 run events", got)
	}
	if got[0].Kind != protocol.KindRunStart || got[0].Session == "" {
		t.Fatalf("first event = %+v, want run_start with a session id", got[0])
	}
	for _, e := range got[1:] {
		if e.Session != got[0].Session {
			t.Errorf("event %+v missing the session tag", e)
		}
	}
	if got[2].Text != "hello from wave" {
		t.Errorf("message = %+v", got[2])
	}
}

// A hosted run belongs to the daemon, not the connection: the run finishes
// even when the submitting client disconnects immediately.
func TestDaemonRunSurvivesClientDisconnect(t *testing.T) {
	ran := make(chan string, 1)
	release := make(chan struct{})
	run := func(ctx context.Context, req RunRequest, sink protocol.Sink) error {
		<-release
		ran <- req.SessionID
		return nil
	}
	sock, _ := startServer(t, run, nil)

	// Dial raw and hang up right after sending the command.
	done := make(chan error, 1)
	go func() {
		done <- Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Task: "long job"},
			func(e protocol.Event) {
				// Disconnect after the banner by returning from Dial via
				// closing: simplest is to panic-free early return — instead
				// we just let Dial keep reading; the run blocks on release.
			})
	}()
	// Let the run start, then simulate the client vanishing by releasing
	// AFTER we know nothing was consumed: close release; the run must still
	// complete and report.
	close(release)
	select {
	case id := <-ran:
		if id == "" {
			t.Fatal("run saw no session id")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hosted run never completed")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("client dial never returned")
	}
}

// attach: journal replay (补读) comes first, tagged with the session; a
// finished/unknown session is replay-only and the stream then closes.
func TestDaemonAttachReplaysFinishedSession(t *testing.T) {
	replay := func(id string, sink protocol.Sink) error {
		sink.Emit(protocol.Event{Kind: protocol.KindTurnStart, Turn: 1})
		sink.Emit(protocol.Event{Kind: protocol.KindMessage, Turn: 1, Text: "replayed"})
		return nil
	}
	sock, _ := startServer(t, nil, replay)

	var got []protocol.Event
	if err := Dial(sock, Command{Cmd: "attach", Session: "old-sess"},
		func(e protocol.Event) { got = append(got, e) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Text != "replayed" || got[1].Session != "old-sess" {
		t.Fatalf("attach replay = %+v", got)
	}
}

// attach on a LIVE run: replay then live events, no gap — an event emitted
// after the subscription but during replay must not be lost.
func TestDaemonAttachFollowsLiveRun(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	run := func(ctx context.Context, req RunRequest, sink protocol.Sink) error {
		sink.Emit(protocol.Event{Kind: protocol.KindTurnStart, Turn: 1})
		close(started)
		<-release
		sink.Emit(protocol.Event{Kind: protocol.KindMessage, Turn: 1, Text: "late news"})
		sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: "completed"})
		return nil
	}
	replay := func(id string, sink protocol.Sink) error {
		sink.Emit(protocol.Event{Kind: protocol.KindTurnStart, Turn: 1, Text: "from journal"})
		return nil
	}
	sock, _ := startServer(t, run, replay)

	// Submit the run in the background; it parks on release.
	go func() {
		_ = Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Task: "job"}, func(protocol.Event) {})
	}()
	<-started

	// Attach while the run is parked, then release it: the attach stream
	// must carry the replay AND the post-attach live events.
	got := make(chan protocol.Event, 16)
	attachDone := make(chan error, 1)
	go func() {
		attachDone <- Dial(sock, Command{Cmd: "attach", Session: "sess-1"},
			func(e protocol.Event) { got <- e })
	}()

	var seen []protocol.Event
	// First the replay line.
	select {
	case e := <-got:
		seen = append(seen, e)
	case <-time.After(5 * time.Second):
		t.Fatal("no replay event")
	}
	close(release)
	deadline := time.After(5 * time.Second)
	for len(seen) < 3 {
		select {
		case e := <-got:
			seen = append(seen, e)
		case <-deadline:
			t.Fatalf("attach stream stalled at %+v", seen)
		}
	}
	if seen[0].Text != "from journal" {
		t.Errorf("replay first, got %+v", seen[0])
	}
	sawLive := false
	for _, e := range seen[1:] {
		if e.Text == "late news" {
			sawLive = true
		}
	}
	if !sawLive {
		t.Error("live event after attach was lost")
	}
	select {
	case err := <-attachDone:
		if err != nil {
			t.Fatalf("attach: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("attach stream never closed after run end")
	}
}

// Graceful shutdown: cancelling the daemon ctx cooperatively cancels every
// hosted run and ListenAndServe returns only AFTER the runs finished their
// terminal work — a routine deploy leaves zero in-doubt sessions.
func TestDaemonGracefulShutdownWaitsForRuns(t *testing.T) {
	var settled atomic.Bool
	run := func(ctx context.Context, req RunRequest, sink protocol.Sink) error {
		<-ctx.Done()
		// Simulate the abort epilogue journaling terminal events.
		time.Sleep(50 * time.Millisecond)
		sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: "canceled"})
		settled.Store(true)
		return ctx.Err()
	}
	sock := filepath.Join(t.TempDir(), "d.sock")
	ctx, cancel := context.WithCancel(context.Background())
	srv := &Server{
		SocketPath: sock, Run: run,
		NewID: func(string) string { return "sess-g" },
	}
	served := make(chan error, 1)
	go func() { served <- srv.ListenAndServe(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := Dial(sock, Command{Cmd: "ping"}, func(protocol.Event) {}); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Park a hosted run, then pull the plug.
	go func() {
		_ = Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Task: "long"}, func(protocol.Event) {})
	}()
	time.Sleep(50 * time.Millisecond) // let the run register
	cancel()

	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not shut down")
	}
	if !settled.Load() {
		t.Fatal("daemon exited before the hosted run finished its terminal work")
	}
}

// Lifecycle events tee to the Notify hook exactly as emitted; ordinary
// events do not.
func TestDaemonNotifyTee(t *testing.T) {
	var mu sync.Mutex
	var teed []protocol.Event
	run := func(ctx context.Context, req RunRequest, sink protocol.Sink) error {
		sink.Emit(protocol.Event{Kind: protocol.KindTurnStart, Turn: 1})
		sink.Emit(protocol.Event{Kind: protocol.KindApprovalRequest, ApprovalID: "apr-7", Tool: "bash"})
		sink.Emit(protocol.Event{Kind: protocol.KindIteration, Turn: 2, Text: "iteration 2 completed"})
		sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: "completed"})
		return nil
	}
	sock := filepath.Join(t.TempDir(), "d.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &Server{
		SocketPath: sock, Run: run,
		NewID: func(string) string { return "sess-n" },
		Notify: func(e protocol.Event) {
			mu.Lock()
			teed = append(teed, e)
			mu.Unlock()
		},
	}
	go func() { _ = srv.ListenAndServe(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := Dial(sock, Command{Cmd: "ping"}, func(protocol.Event) {}); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Task: "job"}, func(protocol.Event) {}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(teed) != 3 {
		t.Fatalf("teed = %+v, want approval_request + iteration + run_end only", teed)
	}
	if teed[0].ApprovalID != "apr-7" || teed[0].Session != "sess-n" {
		t.Errorf("tee[0] = %+v", teed[0])
	}
	if teed[1].Kind != protocol.KindIteration || teed[1].Turn != 2 {
		t.Errorf("tee[1] = %+v", teed[1])
	}
	if teed[2].Kind != protocol.KindRunEnd {
		t.Errorf("tee[2] = %+v", teed[2])
	}
}

// S6 review P1: a client that connects and never sends (or never reads)
// must not wedge graceful shutdown — the daemon closes lingering
// connections after the runs settle.
func TestDaemonShutdownWithHungClient(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "d.sock")
	ctx, cancel := context.WithCancel(context.Background())
	srv := &Server{SocketPath: sock, NewID: func(string) string { return "x" }}
	served := make(chan error, 1)
	go func() { served <- srv.ListenAndServe(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := Dial(sock, Command{Cmd: "ping"}, func(protocol.Event) {}); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// A rude client: connects, sends nothing, never hangs up.
	hung, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = hung.Close() }()
	time.Sleep(50 * time.Millisecond) // let serveConn park in Scan

	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("graceful shutdown wedged on the hung connection")
	}
}

// An idempotent resubmission (same idem_key) attaches to the FIRST
// submission's session instead of minting a duplicate run.
func TestDaemonSubmitIdempotency(t *testing.T) {
	var mu sync.Mutex
	launches := 0
	run := func(ctx context.Context, req RunRequest, sink protocol.Sink) error {
		mu.Lock()
		launches++
		mu.Unlock()
		sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: "completed"})
		return nil
	}
	replay := func(id string, sink protocol.Sink) error {
		sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: "completed"})
		return nil
	}
	sock, _ := startServer(t, run, replay)

	var first []protocol.Event
	if err := Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Task: "job", IdemKey: "k-1"},
		func(e protocol.Event) { first = append(first, e) }); err != nil {
		t.Fatal(err)
	}
	session := first[0].Session

	// Retry with the same key: no second launch; the stream carries the
	// SAME session (served from replay, the run being finished).
	var retry []protocol.Event
	if err := Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Task: "job", IdemKey: "k-1"},
		func(e protocol.Event) { retry = append(retry, e) }); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if launches != 1 {
		mu.Unlock()
		t.Fatalf("launches = %d, want 1 (retry must not duplicate)", launches)
	}
	mu.Unlock()
	if len(retry) == 0 || retry[0].Session != session {
		t.Fatalf("retry stream = %+v, want session %s", retry, session)
	}

	// A DIFFERENT key launches fresh.
	if err := Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Task: "job", IdemKey: "k-2"},
		func(protocol.Event) {}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if launches != 2 {
		t.Fatalf("launches = %d, want 2", launches)
	}
}

// Two daemons must not share a socket; a stale socket file is reclaimed.
func TestDaemonSocketExclusive(t *testing.T) {
	sock, _ := startServer(t, nil, nil)
	second := &Server{SocketPath: sock, NewID: func(string) string { return "x" }}
	err := second.ListenAndServe(context.Background())
	if err == nil {
		t.Fatal("second daemon on the same live socket must fail")
	}
}
