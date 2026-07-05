package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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

	sock := filepath.Join(t.TempDir(), "d.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var errOut bytes.Buffer
	broker := daemon.NewApprovalBroker()
	srv := &daemon.Server{
		SocketPath: sock,
		NewID:      func(task string) string { return runtime.NewSessionID(time.Now(), task) },
		Run:        hostRunFunc("test", &errOut, broker),
		Approvals:  broker,
	}
	go func() { _ = srv.ListenAndServe(ctx) }()
	waitDaemon(t, sock)

	ws := t.TempDir()
	// Open a conversational session and detach after RunStart.
	sid, err := dialUntilStart(sock, daemon.Command{
		Cmd: "run", Conversational: true, SpecPath: specPath, Task: "first question", Workspace: ws,
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
