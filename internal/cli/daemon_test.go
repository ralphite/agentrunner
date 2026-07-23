package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/daemon"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/fork"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// The shared daemon resolver must preserve the asking member's identity on
// both the live event and broker key; otherwise child attach can display an
// approval that cannot be answered.
func TestSocketApprovalsPreserveChildSession(t *testing.T) {
	b := daemon.NewApprovalBroker()
	var out bytes.Buffer
	resolver := socketApprovals{broker: b, session: "root", sink: protocol.NewJSONSink(&out)}
	done := make(chan error, 1)
	go func() {
		_, err := resolver.Resolve(context.Background(), agent.ApprovalRequest{
			ApprovalID: "apr-1", Session: "root-sub-c-a1", Agent: "child",
		})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for !b.Answer("root-sub-c-a1", "apr-1", daemon.ApprovalAnswer{Approve: true}) {
		if time.Now().After(deadline) {
			t.Fatal("child approval was not registered under the child session")
		}
		time.Sleep(time.Millisecond)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"session":"root-sub-c-a1"`) {
		t.Fatalf("approval event lost child tag: %s", out.String())
	}
}

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
		NewID:      func(prompt string) string { return runtime.NewSessionID(time.Now(), prompt) },
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
		Cmd: "run", SpecPath: specPath, Prompt: "wave", Workspace: t.TempDir(),
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

func TestPendingCommandsHoistsChildApprovalToRootHost(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	rootID := "pending-root"
	rootDir, err := runtime.SessionDir(rootID)
	if err != nil {
		t.Fatal(err)
	}
	rootStore, err := store.OpenEventStore(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	env, _ := event.New(event.TypeSessionStarted,
		&event.SessionStarted{SubStateVersions: state.SubStateVersions()})
	if _, err := rootStore.Append(env); err != nil {
		t.Fatal(err)
	}
	_ = rootStore.Close()

	childID := rootID + "-sub-call_1_0-a1"
	childDir := filepath.Join(rootDir, "sub", "call_1_0-a1")
	childStore, err := store.OpenEventStore(childDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := childStore.Append(env); err != nil {
		t.Fatal(err)
	}
	_ = childStore.Close()
	if _, err := store.AppendCommand(childDir, protocol.SessionCommand{
		CommandRef: protocol.CommandRef{CommandID: "cmd-child-approval"},
		Kind:       protocol.CommandApproval, Approval: &protocol.ApprovalCommand{
			ApprovalID: "apr-child", Decision: "approve",
		},
	}); err != nil {
		t.Fatal(err)
	}

	pending, err := pendingCommands(rootID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].CommandID != "cmd-child-approval" ||
		pending[0].Target != childID || pending[0].Kind != protocol.CommandApproval {
		t.Fatalf("root replay pending = %+v", pending)
	}
	ids, err := scanPendingCommandSessions()
	if err != nil || len(ids) != 1 || ids[0] != rootID {
		t.Fatalf("pending roots = %v err=%v", ids, err)
	}
}

// A checkpoint fork seeds its fresh mailbox at the cut's consumed-input
// high-water mark. That mark may come from a revoked input rather than an
// InputReceived event. The daemon boot scan must treat the seed as inert:
// otherwise a normal restart resumes the fork without user authority.
func TestCheckpointForkWatermarkDoesNotAutoResumeOnDaemonRestart(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parentID := "checkpoint-parent"
	parentDir, err := runtime.SessionDir(parentID)
	if err != nil {
		t.Fatal(err)
	}
	parentStore, err := store.OpenEventStore(parentDir)
	if err != nil {
		t.Fatal(err)
	}
	appendParent := func(typ string, payload any) {
		t.Helper()
		env, nerr := event.New(typ, payload)
		if nerr != nil {
			t.Fatal(nerr)
		}
		if _, nerr = parentStore.Append(env); nerr != nil {
			t.Fatal(nerr)
		}
	}
	appendParent(event.TypeSessionStarted, &event.SessionStarted{
		SpecName: "test", SubStateVersions: state.SubStateVersions(),
	})
	appendParent(event.TypeInputReceived, &event.InputReceived{
		Text: "run", Source: "user", DeliverySeq: 18,
	})
	appendParent(event.TypeInputRevoked, &event.InputRevoked{
		TargetCommandID: "cmd-withdrawn", DeliverySeq: 19,
	})
	appendParent(event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1})
	appendParent(event.TypeCheckpointBarrier, &event.CheckpointBarrier{
		BarrierID: "bar-t1", GenStep: 1, Vector: map[string]int64{".": 4},
	})
	if err := parentStore.Close(); err != nil {
		t.Fatal(err)
	}
	parentEvents, err := store.ReadEvents(parentDir)
	if err != nil {
		t.Fatal(err)
	}
	parentFold, err := state.Fold(parentEvents)
	if err != nil {
		t.Fatal(err)
	}

	childID := "checkpoint-child"
	childDir, err := runtime.SessionDir(childID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fork.Cut(fork.Options{
		ParentDir: parentDir, ParentSession: parentID,
		NewDir: childDir, NewSession: childID,
		Barrier: parentFold.Barriers[0], WorkspaceRoot: t.TempDir(),
		Now: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	pending, err := pendingCommands(childID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("fresh checkpoint fork has phantom pending commands: %+v", pending)
	}
	pendingSessions, err := scanPendingCommandSessions()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range pendingSessions {
		if id == childID {
			t.Fatalf("daemon boot scan would auto-resume untouched fork: %v", pendingSessions)
		}
	}
	stranded, err := scanStrandedSessions()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range stranded {
		if id == childID {
			t.Fatalf("stranded boot scan would auto-resume untouched fork: %v", stranded)
		}
	}

	accepted, err := store.AppendInbox(childDir, protocol.UserInput{
		Text: "continue explicitly", Source: "web",
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.DeliverySeq != 20 {
		t.Fatalf("first explicit fork input seq = %d, want 20", accepted.DeliverySeq)
	}
	pending, err = pendingCommands(childID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Input == nil ||
		pending[0].Input.Text != "continue explicitly" {
		t.Fatalf("explicit fork input is not recoverable: %+v", pending)
	}
}

// 决策 #31 下 submit 的退出契约：一次性任务跑到 standby idle 即返回成功
// （曾经挂死：静止模型的 hosted session 不再"结束"、daemon 不关流，旧的
// Dial 等一个永不到来的 EOF——QA Round1 F-A01）。同时钉住 session 行只
// announce 一次（daemon ack 与 loop 自身都发 KindSessionStart）。
func TestSubmitReturnsAtStandbyIdle(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	specPath := writeSpec(t, dir)
	fixture := "steps:\n  - respond: [ { text: \"SUBMIT DONE\" }, { finish: end_turn } ]\n"
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
		SocketPath: sock,
		NewID:      func(prompt string) string { return runtime.NewSessionID(time.Now(), prompt) },
		Run:        hostRunFunc("test", &errLog, broker),
		Approvals:  broker,
	}
	go func() { _ = srv.ListenAndServe(ctx) }()
	waitDaemon(t, sock)

	ws := t.TempDir()
	var out, errOut bytes.Buffer
	done := make(chan int, 1)
	go func() { done <- submitCmd([]string{"--workspace", ws, specPath, "do the thing"}, &out, &errOut) }()
	select {
	case code := <-done:
		if code != ExitOK {
			t.Fatalf("submit: exit %d\nstdout: %s\nstderr: %s\nlog: %s",
				code, out.String(), errOut.String(), errLog.String())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("submit did not return after the run parked at standby idle")
	}
	if !strings.Contains(out.String(), "SUBMIT DONE") {
		t.Fatalf("submit stdout missing the reply:\n%s", out.String())
	}
	if got := strings.Count(errOut.String(), "session 2"); got != 1 {
		t.Fatalf("session announced %d times, want exactly 1:\n%s", got, errOut.String())
	}
}
