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
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

func shortCLISocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ar-cli")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "d.sock")
}

// The daemon hosts a real run end to end: submit streams the events, the
// journal lands under the session dir, and a later attach replays the same
// story from the journal (补读).
func TestDaemonHostsRunAndAttachReplays(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	specPath := writeSpec(t, dir)
	fixture := `steps:
  - respond:
      - text: "hello from the daemon"
      - finish: end_turn
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
		NewID:      func(task string) string { return runtime.NewSessionID(time.Now(), task) },
		Run:        hostRunFunc("test", &errOut, broker),
		Approvals:  broker,
		Replay: func(sessionID string, sink protocol.Sink) error {
			d, err := resolveSessionDir(sessionID)
			if err != nil {
				return err
			}
			return daemon.ReplayJournal(d, sink)
		},
	}
	go func() { _ = srv.ListenAndServe(ctx) }()
	waitUp := time.Now().Add(5 * time.Second)
	for time.Now().Before(waitUp) {
		if err := daemon.Dial(sock, daemon.Command{Cmd: "ping"}, func(protocol.Event) {}); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Submit a run and watch it to its standby idle (决策 #31: the hosted
	// session never "ends" — followers detach at idle).
	var live []protocol.Event
	if err := daemon.DialUntil(sock, daemon.Command{
		Cmd: "run", SpecPath: specPath, Task: "wave", Workspace: t.TempDir(),
	}, func(e protocol.Event) bool {
		live = append(live, e)
		return e.Kind != protocol.KindIdle
	}); err != nil {
		t.Fatal(err)
	}
	if len(live) == 0 || live[0].Kind != protocol.KindSessionStart || live[0].Session == "" {
		t.Fatalf("live stream = %+v\nstderr: %s", live, errOut.String())
	}
	session := live[0].Session
	var sawMsg, sawIdle bool
	for _, e := range live {
		if e.Kind == protocol.KindMessage && e.Text == "hello from the daemon" {
			sawMsg = true
		}
		if e.Kind == protocol.KindIdle {
			sawIdle = true
		}
	}
	if !sawMsg || !sawIdle {
		t.Fatalf("live stream missing message/idle: %+v\nstderr: %s", live, errOut.String())
	}

	// The journal is on disk under the session id.
	if _, err := resolveSessionDir(session); err != nil {
		t.Fatalf("session dir: %v", err)
	}

	// Attach after the fact: the replay tells the same story, up to the
	// journaled standby idle.
	var replayed []protocol.Event
	if err := daemon.DialUntil(sock, daemon.Command{Cmd: "attach", Session: session},
		func(e protocol.Event) bool {
			replayed = append(replayed, e)
			return e.Kind != protocol.KindIdle
		}); err != nil {
		t.Fatal(err)
	}
	var reMsg, reIdle bool
	for _, e := range replayed {
		if e.Session != session {
			t.Errorf("replayed event missing session tag: %+v", e)
		}
		if e.Kind == protocol.KindMessage && e.Text == "hello from the daemon" {
			reMsg = true
		}
		if e.Kind == protocol.KindIdle {
			reIdle = true
		}
	}
	if !reMsg || !reIdle {
		t.Fatalf("replay = %+v, want the journal's message and idle", replayed)
	}
}

func TestPendingCommandsUsesJournalCommandReceipts(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir, err := runtime.SessionDir("pending-test")
	if err != nil {
		t.Fatal(err)
	}
	input, err := store.AppendCommand(dir, protocol.SessionCommand{
		CommandRef: protocol.CommandRef{CommandID: "cmd-input"}, Kind: protocol.CommandInput,
		Input: &protocol.UserInput{Text: "hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctl := protocol.Control{Kind: protocol.ControlClear}
	control, err := store.AppendCommand(dir, protocol.SessionCommand{
		CommandRef: protocol.CommandRef{CommandID: "cmd-control"}, Kind: protocol.CommandControl, Control: &ctl,
	})
	if err != nil {
		t.Fatal(err)
	}
	closeCtl := protocol.Control{Kind: protocol.ControlClose}
	if _, err := store.AppendCommand(dir, protocol.SessionCommand{
		CommandRef: protocol.CommandRef{CommandID: "cmd-close"}, Kind: protocol.CommandClose, Control: &closeCtl,
	}); err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	appendEvent := func(typ string, payload any, commandID string) {
		t.Helper()
		env, nerr := event.New(typ, payload)
		if nerr != nil {
			t.Fatal(nerr)
		}
		env.CommandID = commandID
		if _, nerr = es.Append(env); nerr != nil {
			t.Fatal(nerr)
		}
	}
	appendEvent(event.TypeSessionStarted, &event.SessionStarted{SubStateVersions: state.SubStateVersions()}, "")
	appendEvent(event.TypeInputReceived, &event.InputReceived{Text: "hello", DeliverySeq: input.CommandSeq}, input.CommandID)
	appendEvent(event.TypeContextCompacted, &event.ContextCompacted{Cleared: true}, control.CommandID)
	if err := es.Close(); err != nil {
		t.Fatal(err)
	}
	pending, err := pendingCommands("pending-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].CommandID != "cmd-close" || pending[0].Kind != protocol.CommandClose {
		t.Fatalf("pending = %+v", pending)
	}
	ids, err := scanPendingCommandSessions()
	if err != nil || len(ids) != 1 || ids[0] != "pending-test" {
		t.Fatalf("pending sessions = %v err=%v", ids, err)
	}
}
