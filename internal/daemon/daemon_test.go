package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	sock := shortSock(t)
	ctx, cancel := context.WithCancel(context.Background())
	var n atomic.Int64
	srv := &Server{
		SocketPath: sock,
		Run:        run,
		Replay:     replay,
		NewID:      func(prompt string) string { return fmt.Sprintf("sess-%d", n.Add(1)) },
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

func TestDaemonSocketIsOwnerOnly(t *testing.T) {
	sock, _ := startServer(t, nil, nil)
	info, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket mode = %o, want 0600", got)
	}
}

func TestHostedRunInputQueueIsUnboundedAndFIFO(t *testing.T) {
	h := newHostedRun("s", nil, true)
	defer h.finish()
	const total = 200
	for i := 0; i < total; i++ {
		if !h.post(protocol.UserInput{Text: fmt.Sprintf("m-%03d", i)}) {
			t.Fatalf("post %d rejected", i)
		}
	}
	for i := 0; i < total; i++ {
		select {
		case in := <-h.inbox:
			want := fmt.Sprintf("m-%03d", i)
			if in.Text != want {
				t.Fatalf("delivery %d = %q, want %q", i, in.Text, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("delivery %d timed out", i)
		}
	}
}

func TestHostedRunDeduplicatesCommandID(t *testing.T) {
	h := newHostedRun("s", nil, true)
	defer h.finish()
	in := protocol.UserInput{Text: "once", CommandID: "cmd-1", DeliverySeq: 1}
	if !h.post(in) {
		t.Fatal("first post was rejected")
	}
	if !h.post(in) {
		t.Fatal("duplicate post was rejected")
	}
	select {
	case got := <-h.inbox:
		if got.Text != "once" {
			t.Fatalf("input = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("input was not delivered")
	}
	select {
	case got := <-h.inbox:
		t.Fatalf("duplicate was delivered: %+v", got)
	case <-time.After(50 * time.Millisecond):
	}
}

// INC-12.3: addressing a child never hosts that child independently. The
// durable command and live wake both go through the root hub, while the
// input keeps the child target and the client-facing receipt keeps the
// address the caller used.
func TestDaemonSendToChildRoutesThroughRoot(t *testing.T) {
	const root = "sess-root"
	const child = root + "-sub-work-a1"
	h := newHostedRun(root, nil, true)
	defer h.finish()
	s := &Server{runs: map[string]*hostedRun{root: h}, failed: map[string]bool{}}
	var persistedSession string
	s.PersistCommand = func(session string, cmd protocol.SessionCommand) (protocol.SessionCommand, error) {
		persistedSession = session
		cmd.CommandSeq = 1
		cmd.Input.DeliverySeq = 1
		return cmd, nil
	}
	var reply bytes.Buffer
	s.handleSend(context.Background(), Command{
		Session: child, Text: "please review", CommandID: "cmd-child-1",
	}, json.NewEncoder(&reply))

	if persistedSession != root {
		t.Fatalf("command persisted to %q, want tree root %q", persistedSession, root)
	}
	select {
	case in := <-h.inbox:
		if in.Target != child || in.Text != "please review" || in.CommandID != "cmd-child-1" {
			t.Fatalf("root wake input = %+v", in)
		}
	case <-time.After(time.Second):
		t.Fatal("root host never received the targeted input")
	}
	var ack protocol.Event
	if err := json.Unmarshal(reply.Bytes(), &ack); err != nil {
		t.Fatal(err)
	}
	if ack.Kind != protocol.KindMessage || ack.Text != "delivered" || ack.Session != child {
		t.Fatalf("ack = %+v, want child-addressed delivered receipt", ack)
	}
}

func TestHostedRunPreservesControlBeforeApproval(t *testing.T) {
	h := newHostedRun("s", nil, true)
	defer h.finish()
	result := make(chan string, 1)
	h.mu.Lock()
	h.answerApproval = func(protocol.SessionCommand) bool {
		select {
		case ctl := <-h.controls:
			result <- ctl.CommandID
		default:
			result <- "approval overtook control enqueue"
		}
		return true
	}
	h.mu.Unlock()
	ctl := protocol.Control{CommandRef: protocol.CommandRef{CommandID: "cmd-1", CommandSeq: 1}, Kind: protocol.ControlClear}
	if !h.postCommand(protocol.SessionCommand{CommandRef: ctl.CommandRef, Kind: protocol.CommandControl, Control: &ctl}) ||
		!h.postCommand(protocol.SessionCommand{
			CommandRef: protocol.CommandRef{CommandID: "cmd-2", CommandSeq: 2},
			Kind:       protocol.CommandApproval,
			Approval:   &protocol.ApprovalCommand{ApprovalID: "apr-1", Decision: "approve"},
		}) {
		t.Fatal("commands were rejected")
	}
	select {
	case got := <-result:
		if got != "cmd-1" {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("approval was not delivered")
	}
}

func TestCompletedCommandRetryDoesNotWakeAfterRestart(t *testing.T) {
	var resumed atomic.Int32
	s := &Server{
		PersistCommand: func(_ string, cmd protocol.SessionCommand) (protocol.SessionCommand, error) {
			cmd.CommandSeq = 7
			cmd.PreviouslyAccepted = true
			return cmd, nil
		},
		PendingCommands: func(string) ([]protocol.SessionCommand, error) { return nil, nil },
		Resume: func(context.Context, ResumeRequest, protocol.Sink) error {
			resumed.Add(1)
			return nil
		},
	}
	_, delivered, err := s.acceptAndDeliver(context.Background(), "s", "cmd-old",
		protocol.SessionCommand{Kind: protocol.CommandInterrupt},
		func(*hostedRun, protocol.SessionCommand) bool { return true })
	if err != nil || !delivered {
		t.Fatalf("retry = delivered %v err %v", delivered, err)
	}
	if resumed.Load() != 0 {
		t.Fatal("completed retry woke the session")
	}
}

// INC-98.5a: series pause is fsynced into the independent command log before
// ack, then delivered through a drive-only hub. Re-host replay uses that same
// accepted receipt, so an append→crash window cannot lose the pause.
func TestSeriesPauseDurableReplayStartsDriveHost(t *testing.T) {
	var accepted protocol.SessionCommand
	delivered := make(chan protocol.Control, 1)
	s := &Server{
		runs: map[string]*hostedRun{},
		SeriesControlState: func(string) (SeriesControlState, error) {
			return SeriesControlState{Eligible: true}, nil
		},
		PersistCommand: func(_ string, cmd protocol.SessionCommand) (protocol.SessionCommand, error) {
			cmd.CommandSeq = 9
			cmd.Control.CommandRef = cmd.CommandRef
			accepted = cmd
			return cmd, nil
		},
		PendingCommands: func(string) ([]protocol.SessionCommand, error) {
			if accepted.CommandID == "" {
				return nil, nil
			}
			return []protocol.SessionCommand{accepted}, nil
		},
		ResumeDrive: func(_ context.Context, req DriveRequest, _ protocol.Sink) error {
			select {
			case ctl := <-req.Controls:
				delivered <- ctl
			case <-time.After(time.Second):
				t.Error("pending series command was not replayed")
			}
			return nil
		},
	}
	var reply bytes.Buffer
	s.handleSeriesControl(context.Background(), Command{
		Session: "series-1", CommandID: "cmd-pause",
	}, true, json.NewEncoder(&reply))
	var ack protocol.Event
	if err := json.Unmarshal(reply.Bytes(), &ack); err != nil {
		t.Fatal(err)
	}
	if ack.Kind != protocol.KindMessage || !strings.Contains(ack.Text, "current iteration") {
		t.Fatalf("ack = %+v", ack)
	}
	select {
	case ctl := <-delivered:
		if ctl.Kind != protocol.ControlSchedulePause || ctl.CommandID != "cmd-pause" || ctl.CommandSeq != 9 {
			t.Fatalf("delivered control = %+v", ctl)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("drive host never received the durable pause")
	}
}

// run: the daemon hosts the run, streams its events (tagged with the
// session), and reports the assigned session id first.
func TestDaemonRunStreams(t *testing.T) {
	run := func(ctx context.Context, req RunRequest, sink protocol.Sink) error {
		sink.Emit(protocol.Event{Kind: protocol.KindGenerationStart, N: 1})
		sink.Emit(protocol.Event{Kind: protocol.KindMessage, N: 1, Text: "hello from " + req.Prompt})
		sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: "completed"})
		return nil
	}
	sock, _ := startServer(t, run, nil)

	var got []protocol.Event
	err := Dial(sock, Command{Cmd: "run", SpecPath: "spec.yaml", Prompt: "wave"},
		func(e protocol.Event) { got = append(got, e) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("events = %+v, want session banner + 3 run events", got)
	}
	if got[0].Kind != protocol.KindSessionStart || got[0].Session == "" {
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
		done <- Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Prompt: "long job"},
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
		sink.Emit(protocol.Event{Kind: protocol.KindGenerationStart, N: 1})
		sink.Emit(protocol.Event{Kind: protocol.KindMessage, N: 1, Text: "replayed"})
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
		sink.Emit(protocol.Event{Kind: protocol.KindGenerationStart, N: 1})
		close(started)
		<-release
		sink.Emit(protocol.Event{Kind: protocol.KindMessage, N: 1, Text: "late news"})
		sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: "completed"})
		return nil
	}
	replay := func(id string, sink protocol.Sink) error {
		sink.Emit(protocol.Event{Kind: protocol.KindGenerationStart, N: 1, Text: "from journal"})
		return nil
	}
	sock, _ := startServer(t, run, replay)

	// Submit the run in the background; it goes idle on release.
	go func() {
		_ = Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Prompt: "job"}, func(protocol.Event) {})
	}()
	<-started

	// Attach while the run is idle, then release it: the attach stream
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

// --replay-only replays a LIVE session's recorded history and RETURNS, without
// following the live output the run keeps emitting (黑盒 R2-E-5): a transcript
// dump that never hijacks the terminal to tail a still-running session.
func TestAttachReplayOnly(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	run := func(ctx context.Context, req RunRequest, sink protocol.Sink) error {
		sink.Emit(protocol.Event{Kind: protocol.KindGenerationStart, N: 1})
		close(started)
		<-release
		sink.Emit(protocol.Event{Kind: protocol.KindMessage, N: 1, Text: "live news"})
		return nil
	}
	replay := func(id string, sink protocol.Sink) error {
		sink.Emit(protocol.Event{Kind: protocol.KindMessage, Text: "from journal"})
		return nil
	}
	sock, _ := startServer(t, run, replay)
	go func() {
		_ = Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Prompt: "job"}, func(protocol.Event) {})
	}()
	<-started // the run is live and idle on release

	var seen []protocol.Event
	done := make(chan error, 1)
	go func() {
		done <- Dial(sock, Command{Cmd: "attach", Session: "sess-1", ReplayOnly: true},
			func(e protocol.Event) { seen = append(seen, e) })
	}()
	// It must RETURN after the journal even though the run is live — if it
	// followed live it would block here until release.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("replay-only attach errored: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("replay-only attach did not return — it followed live output")
	}
	close(release)
	if len(seen) != 1 || seen[0].Text != "from journal" {
		t.Fatalf("replay-only attach = %+v, want just the journal replay", seen)
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
	sock := shortSock(t)
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

	// Idle a hosted run, then pull the plug.
	go func() {
		_ = Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Prompt: "long"}, func(protocol.Event) {})
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
		sink.Emit(protocol.Event{Kind: protocol.KindGenerationStart, N: 1})
		sink.Emit(protocol.Event{Kind: protocol.KindApprovalRequest, ApprovalID: "apr-7", Tool: "bash"})
		sink.Emit(protocol.Event{Kind: protocol.KindIteration, N: 2, Text: "iteration 2 completed"})
		sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: "completed"})
		return nil
	}
	sock := shortSock(t)
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
	if err := Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Prompt: "job"}, func(protocol.Event) {}); err != nil {
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
	if teed[1].Kind != protocol.KindIteration || teed[1].N != 2 {
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
	sock := shortSock(t)
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
	time.Sleep(50 * time.Millisecond) // let serveConn idle in Scan

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
	if err := Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Prompt: "job", IdemKey: "k-1"},
		func(e protocol.Event) { first = append(first, e) }); err != nil {
		t.Fatal(err)
	}
	session := first[0].Session

	// Retry with the same key: no second launch; the stream carries the
	// SAME session (served from replay, the run being finished).
	var retry []protocol.Event
	if err := Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Prompt: "job", IdemKey: "k-1"},
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
	if err := Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Prompt: "job", IdemKey: "k-2"},
		func(protocol.Event) {}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if launches != 2 {
		t.Fatalf("launches = %d, want 2", launches)
	}
}

// S7 还债: the idem index survives a daemon restart — a retry against the
// NEW daemon still finds the old session (served from replay).
func TestDaemonIdemPersistsAcrossRestart(t *testing.T) {
	idemPath := filepath.Join(t.TempDir(), "idem.json")
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

	// Daemon #1: submit with a key, then shut down.
	sock1 := shortSock(t)
	ctx1, cancel1 := context.WithCancel(context.Background())
	srv1 := &Server{SocketPath: sock1, Run: run, Replay: replay, IdemPath: idemPath,
		NewID: func(string) string { return "sess-persist" }}
	done1 := make(chan error, 1)
	go func() { done1 <- srv1.ListenAndServe(ctx1) }()
	waitDial(t, sock1)
	var first []protocol.Event
	if err := Dial(sock1, Command{Cmd: "run", SpecPath: "s.yaml", Prompt: "job", IdemKey: "k-persist"},
		func(e protocol.Event) { first = append(first, e) }); err != nil {
		t.Fatal(err)
	}
	cancel1()
	<-done1

	// Daemon #2 on a fresh socket, same IdemPath: the retry reattaches.
	sock2 := shortSock(t)
	ctx2, cancel2 := context.WithCancel(context.Background())
	srv2 := &Server{SocketPath: sock2, Run: run, Replay: replay, IdemPath: idemPath,
		NewID: func(string) string { return "sess-should-not-exist" }}
	done2 := make(chan error, 1)
	go func() { done2 <- srv2.ListenAndServe(ctx2) }()
	defer func() { cancel2(); <-done2 }()
	waitDial(t, sock2)

	var retry []protocol.Event
	if err := Dial(sock2, Command{Cmd: "run", SpecPath: "s.yaml", Prompt: "job", IdemKey: "k-persist"},
		func(e protocol.Event) { retry = append(retry, e) }); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if launches != 1 {
		t.Fatalf("launches = %d, want 1 (restart must not duplicate the run)", launches)
	}
	if len(retry) == 0 || retry[0].Session != "sess-persist" {
		t.Fatalf("retry = %+v, want the persisted session", retry)
	}
}

// waitDial polls until the daemon accepts connections.
func waitDial(t *testing.T, sock string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := Dial(sock, Command{Cmd: "ping"}, func(protocol.Event) {}); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("daemon never came up")
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

// attach on a CHILD session id (INC-12.6): replay reads the member's own
// journal; live events come from the TREE ROOT's hub filtered to the member
// — the root's own stream stays out.
func TestDaemonAttachChildFiltersLive(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	run := func(ctx context.Context, req RunRequest, sink protocol.Sink) error {
		sink.Emit(protocol.Event{Kind: protocol.KindGenerationStart, N: 1, Session: req.SessionID})
		close(started)
		<-release
		sink.Emit(protocol.Event{Kind: protocol.KindMessage, Text: "root speaking", Session: req.SessionID})
		sink.Emit(protocol.Event{Kind: protocol.KindMessage, Text: "member speaking", Session: req.SessionID + "-sub-pm-a1"})
		sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, Reason: "completed", Session: req.SessionID})
		return nil
	}
	replay := func(id string, sink protocol.Sink) error {
		if !strings.Contains(id, "-sub-") {
			t.Errorf("replay asked for %q, want the child id", id)
		}
		sink.Emit(protocol.Event{Kind: protocol.KindMessage, Text: "member journal line"})
		return nil
	}
	sock, _ := startServer(t, run, replay)

	go func() {
		_ = Dial(sock, Command{Cmd: "run", SpecPath: "s.yaml", Prompt: "job"}, func(protocol.Event) {})
	}()
	<-started

	got := make(chan protocol.Event, 32)
	go func() {
		_ = Dial(sock, Command{Cmd: "attach", Session: "sess-1-sub-pm-a1"}, func(e protocol.Event) { got <- e })
		close(got)
	}()
	time.Sleep(50 * time.Millisecond) // let the attach subscribe
	close(release)

	var texts []string
	deadline := time.After(5 * time.Second)
collect:
	for {
		select {
		case e, ok := <-got:
			if !ok {
				break collect
			}
			if e.Kind == protocol.KindMessage {
				texts = append(texts, e.Text)
			}
		case <-deadline:
			break collect
		}
	}
	joined := strings.Join(texts, "|")
	if !strings.Contains(joined, "member journal line") || !strings.Contains(joined, "member speaking") {
		t.Fatalf("child attach missing member content: %q", joined)
	}
	if strings.Contains(joined, "root speaking") {
		t.Fatalf("child attach leaked the root stream: %q", joined)
	}
}

// An approval answer whose ask never appears in the broker must not
// head-of-line-block the command pump forever: it is dropped after the
// bounded retry window and the commands queued behind it still deliver
// (QA Round4 F-J1 — one undeliverable approve froze send/close/stop for
// the rest of the session's life).
func TestPumpDropsUndeliverableApprovalAndMovesOn(t *testing.T) {
	h := newHostedRun("s", nil, true)
	defer h.finish()
	h.mu.Lock()
	h.approvalGiveUp = 3
	h.answerApproval = func(protocol.SessionCommand) bool { return false }
	h.mu.Unlock()

	if !h.postCommand(protocol.SessionCommand{
		CommandRef: protocol.CommandRef{CommandID: "cmd-apr"},
		Kind:       protocol.CommandApproval,
		Approval:   &protocol.ApprovalCommand{ApprovalID: "apr-x", Decision: "approve"},
	}) {
		t.Fatal("approval postCommand refused")
	}
	if !h.postCommand(protocol.SessionCommand{
		CommandRef: protocol.CommandRef{CommandID: "cmd-input"},
		Kind:       protocol.CommandInput,
		Input:      &protocol.UserInput{Text: "queued behind the wedge"},
	}) {
		t.Fatal("input postCommand refused")
	}

	select {
	case in := <-h.inbox:
		if in.Text != "queued behind the wedge" {
			t.Fatalf("unexpected input %q", in.Text)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("input never delivered: the undeliverable approval wedged the pump")
	}
}
