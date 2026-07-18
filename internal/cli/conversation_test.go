package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/daemon"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/store"
)

// v2 M1.2 (daemon twin of C1): a conversational session hosted by the
// daemon takes three inputs over the wire — the opening `run` plus two
// `send`s — each producing a turn, then a `close` ends it. Deterministic
// (scripted provider); the real-API gate is QA-01 at the M1 milestone exit.
func TestDaemonConversationalSendClose(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	specPath := writeSpec(t, dir)
	// One step per turn; each just answers and yields.
	fixture := `steps:
  - respond: [ { text: "answer one" }, { finish: end_turn } ]
  - respond: [ { text: "answer two" }, { finish: end_turn } ]
  - respond: [ { text: "answer three" }, { finish: end_turn } ]
`
	fixPath := filepath.Join(dir, "fix.yaml")
	if err := os.WriteFile(fixPath, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTRUNNER_SCRIPTED_FIXTURE", fixPath)

	sock := shortCLISocket(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var errOut bytes.Buffer
	broker := daemon.NewApprovalBroker()
	srv := &daemon.Server{
		SocketPath: sock,
		NewID:      func(prompt string) string { return runtime.NewSessionID(time.Now(), prompt) },
		Run:        hostRunFunc("test", &errOut, broker),
		Approvals:  broker,
	}
	go func() { _ = srv.ListenAndServe(ctx) }()
	waitDaemon(t, sock)

	ws := t.TempDir()
	// Open a conversational session and detach after RunStart.
	sid, err := dialUntilStart(sock, daemon.Command{
		Cmd: "run", SpecPath: specPath, Prompt: "first question", Workspace: ws,
	})
	if err != nil || sid == "" {
		t.Fatalf("new: sid=%q err=%v\nstderr: %s", sid, err, errOut.String())
	}
	// The store dir is created by the hosting goroutine concurrently with
	// RunStart; compute the path directly rather than scanning (which races).
	sdir, err := runtime.SessionDir(sid)
	if err != nil {
		t.Fatal(err)
	}

	waitAssistants := func(n int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			evs, _ := store.ReadEvents(sdir)
			c := 0
			for _, e := range evs {
				if e.Type == event.TypeAssistantMessage {
					c++
				}
			}
			if c >= n {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %d assistant messages\nstderr: %s", n, errOut.String())
	}
	sendOK := func(text string) {
		t.Helper()
		var ok bool
		if err := daemon.Dial(sock, daemon.Command{Cmd: "send", Session: sid, Text: text},
			func(e protocol.Event) { ok = e.Kind == protocol.KindMessage }); err != nil || !ok {
			t.Fatalf("send %q: ok=%v err=%v", text, ok, err)
		}
	}

	waitAssistants(1) // the opening message produced turn 1
	sendOK("second question")
	waitAssistants(2)
	sendOK("third question")
	waitAssistants(3)

	// Close: the idle loop resolves into its epilogue.
	var closing bool
	if err := daemon.Dial(sock, daemon.Command{Cmd: "close", Session: sid},
		func(e protocol.Event) { closing = e.Kind == protocol.KindMessage }); err != nil || !closing {
		t.Fatalf("close: ok=%v err=%v", closing, err)
	}

	// The journal: 3 user inputs, ≥3 turns, exactly one terminal, at the tail.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		evs, _ := store.ReadEvents(sdir)
		if len(evs) > 0 && evs[len(evs)-1].Type == event.TypeSessionClosed {
			var inputs, ends int
			for _, e := range evs {
				if e.Type == event.TypeInputReceived {
					inputs++
				}
				if e.Type == event.TypeSessionClosed {
					ends++
				}
			}
			if inputs != 3 {
				t.Fatalf("user inputs = %d, want 3", inputs)
			}
			if ends != 1 {
				t.Fatalf("session_closed count = %d, want exactly 1", ends)
			}
			dec, _ := event.DecodePayload(evs[len(evs)-1])
			if r := dec.(*event.SessionClosed).Reason; r != "closed" {
				t.Fatalf("terminal reason = %q, want closed", r)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session did not reach a terminal after close\nstderr: %s", errOut.String())
}

// INC-2 BB-me-4/5/6: the conversational path must SHOW the reply. `new`
// renders the opening turn's text and prints the continue hint; `send`
// renders the reply (and not the "delivered" ack) — both detach at idle,
// leaving the session running.
func TestNewAndSendRenderReply(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	specPath := writeSpec(t, dir)
	fixture := `steps:
  - respond: [ { text: "roses are red, violets are blue" }, { finish: end_turn } ]
  - respond: [ { text: "PONG" }, { finish: end_turn } ]
`
	fixPath := filepath.Join(dir, "fix.yaml")
	if err := os.WriteFile(fixPath, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTRUNNER_SCRIPTED_FIXTURE", fixPath)

	// The daemon listens where the CLI's own socketPath() looks, so
	// newCmd/sendCmd find it exactly like in production.
	sock, err := socketPath()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var errLog bytes.Buffer
	broker := daemon.NewApprovalBroker()
	srv := &daemon.Server{
		SocketPath:   sock,
		NewID:        func(prompt string) string { return runtime.NewSessionID(time.Now(), prompt) },
		Run:          hostRunFunc("test", &errLog, broker),
		Approvals:    broker,
		PersistInput: persistInputFunc(),
	}
	go func() { _ = srv.ListenAndServe(ctx) }()
	waitDaemon(t, sock)

	ws := t.TempDir()
	var out, errOut bytes.Buffer
	if code := newCmd([]string{"--workspace", ws, specPath, "write a poem"}, &out, &errOut); code != ExitOK {
		t.Fatalf("new: exit %d\nstdout: %s\nstderr: %s\nlog: %s", code, out.String(), errOut.String(), errLog.String())
	}
	if !strings.Contains(out.String(), "roses are red") {
		t.Fatalf("new stdout missing the reply:\n%s", out.String())
	}
	m := regexp.MustCompile(`session (\S+)`).FindStringSubmatch(errOut.String())
	if m == nil {
		t.Fatalf("new stderr missing the session line:\n%s", errOut.String())
	}
	sid := m[1]
	if !strings.Contains(errOut.String(), "agentrunner send "+sid) {
		t.Fatalf("new stderr missing the continue hint:\n%s", errOut.String())
	}

	out.Reset()
	errOut.Reset()
	if code := sendCmd([]string{sid, "say PONG"}, &out, &errOut); code != ExitOK {
		t.Fatalf("send: exit %d\nstdout: %s\nstderr: %s\nlog: %s", code, out.String(), errOut.String(), errLog.String())
	}
	if !strings.Contains(out.String(), "PONG") {
		t.Fatalf("send stdout missing the reply:\n%s", out.String())
	}
	if strings.Contains(out.String(), "delivered") {
		t.Fatalf("send stdout must not show the ack:\n%s", out.String())
	}

	// The session survived both detaches: close it cleanly.
	var closing bool
	if err := daemon.Dial(sock, daemon.Command{Cmd: "close", Session: sid},
		func(e protocol.Event) { closing = e.Kind == protocol.KindMessage }); err != nil || !closing {
		t.Fatalf("close: ok=%v err=%v", closing, err)
	}
}

// INC-2: --detach restores the fire-and-forget forms (id only / ack only).
func TestNewAndSendDetach(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	specPath := writeSpec(t, dir)
	fixture := `steps:
  - respond: [ { text: "hi" }, { finish: end_turn } ]
  - respond: [ { text: "ok" }, { finish: end_turn } ]
`
	fixPath := filepath.Join(dir, "fix.yaml")
	if err := os.WriteFile(fixPath, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTRUNNER_SCRIPTED_FIXTURE", fixPath)
	sock, err := socketPath()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var errLog bytes.Buffer
	broker := daemon.NewApprovalBroker()
	srv := &daemon.Server{
		SocketPath:   sock,
		NewID:        func(prompt string) string { return runtime.NewSessionID(time.Now(), prompt) },
		Run:          hostRunFunc("test", &errLog, broker),
		Approvals:    broker,
		PersistInput: persistInputFunc(),
	}
	served := make(chan struct{})
	go func() { _ = srv.ListenAndServe(ctx); close(served) }()
	// Drain BEFORE the TempDirs vanish (cleanups run LIFO, this one is
	// registered after them): a detached send's turn keeps journaling after
	// "delivered", and TempDir RemoveAll racing those writes was a real
	// gotest flake (audit-0717 F3, "directory not empty").
	t.Cleanup(func() {
		cancel()
		select {
		case <-served:
		case <-time.After(5 * time.Second):
		}
	})
	waitDaemon(t, sock)

	ws := t.TempDir()
	var out, errOut bytes.Buffer
	if code := newCmd([]string{"--detach", "--workspace", ws, specPath, "write a poem"}, &out, &errOut); code != ExitOK {
		t.Fatalf("new --detach: exit %d\nstderr: %s", code, errOut.String())
	}
	sid := strings.TrimSpace(out.String())
	if sid == "" || strings.Contains(sid, " ") {
		t.Fatalf("new --detach stdout should be just the id, got %q", out.String())
	}

	// The journal dir is created by the hosting goroutine after RunStart;
	// wait for it so the send's mailbox write has somewhere to land.
	sdir, err := runtime.SessionDir(sid)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if evs, err := store.ReadEvents(sdir); err == nil && len(evs) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session journal never appeared\nlog: %s", errLog.String())
		}
		time.Sleep(10 * time.Millisecond)
	}

	out.Reset()
	errOut.Reset()
	if code := sendCmd([]string{"--detach", sid, "more"}, &out, &errOut); code != ExitOK {
		t.Fatalf("send --detach: exit %d\nstderr: %s", code, errOut.String())
	}
	if strings.TrimSpace(out.String()) != "delivered" {
		t.Fatalf("send --detach stdout = %q, want delivered", out.String())
	}
	// Wait for the detached turn to SETTLE (second idle) so nothing is still
	// writing when the temp dirs are torn down.
	deadline = time.Now().Add(5 * time.Second)
	for {
		evs, err := store.ReadEvents(sdir)
		idles := 0
		if err == nil {
			for _, e := range evs {
				if e.Type == event.TypeWaitingEntered {
					idles++
				}
			}
		}
		if idles >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("detached send turn never settled\nlog: %s", errLog.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitDaemon(t *testing.T, sock string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := daemon.Dial(sock, daemon.Command{Cmd: "ping"}, func(protocol.Event) {}); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("daemon did not come up")
}

// TestScheduleAttachValidation pins that an unusable cadence is rejected at
// parse time, before any daemon round-trip (INC-74.3): both/neither of
// --every/--cron, an under-1s or unparseable duration, a bad cron expression,
// a negative --max-wakes, and a missing standing prompt are all usage errors.
func TestScheduleAttachValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"both cadences", []string{"s", "attach", "p", "--every", "30m", "--cron", "* * * * *"}, "not both"},
		{"no cadence", []string{"s", "attach", "p"}, "a cadence is required"},
		{"bad duration", []string{"s", "attach", "p", "--every", "banana"}, "not a duration"},
		{"sub-second", []string{"s", "attach", "p", "--every", "500ms"}, "not a duration of at least 1s"},
		{"bad cron", []string{"s", "attach", "p", "--cron", "* * *"}, "want 5 fields"},
		{"negative max-wakes", []string{"s", "attach", "p", "--every", "30m", "--max-wakes", "-1"}, "non-negative"},
		{"no prompt", []string{"s", "attach", "--every", "30m"}, "standing prompt is required"},
	}
	for _, tc := range cases {
		var out, errOut bytes.Buffer
		if code := scheduleCmd(tc.args, &out, &errOut); code != ExitUsage {
			t.Errorf("%s: code = %d, want ExitUsage (stderr: %s)", tc.name, code, errOut.String())
		}
		if !strings.Contains(errOut.String(), tc.want) {
			t.Errorf("%s: stderr = %q, want it to contain %q", tc.name, errOut.String(), tc.want)
		}
	}
}

// TestGoalMaxChecksValidation pins that a non-positive --max-checks is rejected
// at parse time (before any daemon round-trip), while an unset flag is left to
// the attach default / an update's existing budget (QA Wave7 olive-02).
func TestGoalMaxChecksValidation(t *testing.T) {
	for _, sub := range []string{"attach", "update"} {
		for _, bad := range []string{"0", "-3"} {
			var out, errOut bytes.Buffer
			code := goalCmd([]string{"somesession", sub, "do the thing", "--verify", "true", "--max-checks", bad}, &out, &errOut)
			if code != ExitUsage {
				t.Errorf("goal %s --max-checks %s: code = %d, want ExitUsage", sub, bad, code)
			}
			if !strings.Contains(errOut.String(), "must be a positive integer") {
				t.Errorf("goal %s --max-checks %s: stderr = %q, want the positive-integer error", sub, bad, errOut.String())
			}
		}
	}
}
