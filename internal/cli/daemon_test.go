package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/daemon"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/runtime"
)

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

	sock := filepath.Join(t.TempDir(), "d.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var errOut bytes.Buffer
	srv := &daemon.Server{
		SocketPath: sock,
		NewID:      func(task string) string { return runtime.NewSessionID(time.Now(), task) },
		Run:        hostRunFunc("test", &errOut),
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

	// Submit a run and watch it to completion.
	var live []protocol.Event
	if err := daemon.Dial(sock, daemon.Command{
		Cmd: "run", SpecPath: specPath, Task: "wave", Workspace: t.TempDir(),
	}, func(e protocol.Event) { live = append(live, e) }); err != nil {
		t.Fatal(err)
	}
	if len(live) == 0 || live[0].Kind != protocol.KindRunStart || live[0].Session == "" {
		t.Fatalf("live stream = %+v\nstderr: %s", live, errOut.String())
	}
	session := live[0].Session
	var sawMsg, sawEnd bool
	for _, e := range live {
		if e.Kind == protocol.KindMessage && e.Text == "hello from the daemon" {
			sawMsg = true
		}
		if e.Kind == protocol.KindRunEnd && e.Reason == "completed" {
			sawEnd = true
		}
	}
	if !sawMsg || !sawEnd {
		t.Fatalf("live stream missing message/end: %+v\nstderr: %s", live, errOut.String())
	}

	// The journal is on disk under the session id.
	if _, err := resolveSessionDir(session); err != nil {
		t.Fatalf("session dir: %v", err)
	}

	// Attach after the fact: the replay tells the same story.
	var replayed []protocol.Event
	if err := daemon.Dial(sock, daemon.Command{Cmd: "attach", Session: session},
		func(e protocol.Event) { replayed = append(replayed, e) }); err != nil {
		t.Fatal(err)
	}
	var reMsg, reEnd bool
	for _, e := range replayed {
		if e.Session != session {
			t.Errorf("replayed event missing session tag: %+v", e)
		}
		if e.Kind == protocol.KindMessage && e.Text == "hello from the daemon" {
			reMsg = true
		}
		if e.Kind == protocol.KindRunEnd && e.Reason == "completed" {
			reEnd = true
		}
	}
	if !reMsg || !reEnd {
		t.Fatalf("replay = %+v, want the journal's message and run_end", replayed)
	}
}
