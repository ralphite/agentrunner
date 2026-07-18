package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/blackboard"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/config"
	"github.com/ralphite/agentrunner/internal/daemon"
	"github.com/ralphite/agentrunner/internal/driver"
	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/notify"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// sockPathMax caps the socket path length. The kernel's sockaddr_un.sun_path
// is 104 bytes on darwin, 108 on Linux; 100 leaves headroom on both and the
// bind is what actually enforces it — this only decides whether to fall back.
const sockPathMax = 100

// socketPath is the daemon's rendezvous, fixed under the data dir so every
// client finds the same runtime. The dir is created here: the daemon may be
// the first agentrunner process this machine ever ran.
//
// unix sockets cap paths at ~104 bytes (darwin sun_path), so an extravagant
// XDG_DATA_HOME would make the natural path un-bindable ("bind: invalid
// argument"). When that happens we fall back to a short, stable path under
// the temp dir, derived by hashing the data dir — deterministic, so the
// daemon and every client compute the SAME fallback and still rendezvous.
func socketPath() (string, error) {
	data, err := runtime.DataDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(data, 0o700); err != nil {
		return "", fmt.Errorf("daemon: %w", err)
	}
	sock := filepath.Join(data, "daemon.sock")
	if len(sock) <= sockPathMax {
		return sock, nil
	}
	h := sha256.Sum256([]byte(data))
	return filepath.Join(os.TempDir(), "ar-"+hex.EncodeToString(h[:8])+".sock"), nil
}

// daemonCmd runs the resident runtime (S6 模块④): `agentrunner daemon`.
// Hosted runs' asks idle on the approval broker and resolve over the socket
// (`agentrunner approve`); a run cancelled before its ask is answered
// resolves denied through the loop's normal ctx path.
func daemonCmd(args []string, version string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(stderr)
	detach := fs.Bool("detach", false, "start the daemon in the background, detached from this terminal, then return")
	httpAddr := fs.String("http", "", "enable the webhook ingress on this TCP address (INC-50, e.g. 127.0.0.1:4177); empty = off")
	if err := fs.Parse(reorderFlags(fs, args)); err != nil {
		return ExitUsage
	}
	if len(fs.Args()) != 0 {
		fmt.Fprintln(stderr, "usage: agentrunner daemon [--detach] [--http addr]")
		return ExitUsage
	}
	if *detach {
		// Real backgrounding (T3): `daemon &` dies with SIGHUP when its shell
		// exits — a new user closes the terminal and every send/new/sessions
		// then reports "no daemon". --detach re-execs as a session leader so
		// the runtime outlives this terminal.
		return daemonDetach(*httpAddr, stdout, stderr)
	}
	sigCtx, _, stop := signalContext()
	defer stop()
	// A SIGTERM/second-Ctrl-C here is a GRACEFUL HOST SHUTDOWN, not a user
	// stopping any particular work: re-cancel with the ErrHostShutdown cause
	// (INC-72, G22b) so loop-mode drivers end without a terminal and the next
	// boot's drive sweep revives them. The cause rides the ctx tree into
	// every hosted run.
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	go func() {
		<-sigCtx.Done()
		cancel(errs.ErrHostShutdown)
	}()
	loadDotEnv(".env")

	sock, err := socketPath()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	// A live daemon already owns the socket: a second foreground start would
	// only collide on the store/notifier lock and surface a cryptic
	// "notifier: session locked: held by pid N" (QA Wave3 judy-06). Report it
	// plainly instead, mirroring --detach's idempotent probe.
	if conn, derr := net.Dial("unix", sock); derr == nil {
		_ = conn.Close()
		fmt.Fprintf(stderr, "daemon already running on %s\n", sock)
		return ExitOK
	}
	broker := daemon.NewApprovalBroker()
	notifier, notifyTee, err := buildNotifier(ctx, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	defer func() { _ = notifier.Close() }()
	srv := &daemon.Server{
		SocketPath:   sock,
		NewID:        func(prompt string) string { return runtime.NewSessionID(time.Now(), prompt) },
		Run:          hostRunFunc(version, stderr, broker),
		SplitAddress: splitSessionAddress,
		Replay: func(sessionID string, sink protocol.Sink) error {
			dir, err := resolveSessionDir(sessionID)
			if err != nil {
				return err
			}
			return daemon.ReplayJournal(dir, sink)
		},
		ScanTimers:                 scanSessionTimers,
		Resume:                     hostResumeFunc(version, stderr, broker),
		ScanDrives:                 scanDriveSessions,
		ScanStranded:               scanStrandedSessions,
		ResumeDrive:                hostResumeDriveFunc(version, stderr, broker),
		PersistCommand:             persistCommandFunc(),
		PendingCommands:            pendingCommands,
		ScanPendingCommandSessions: scanPendingCommandSessions,
		SessionMarked:              sessionMarked,
		PendingApproval:            pendingApproval,
		Drive:                      hostDriveFunc(version, stderr, broker),
		Approvals:                  broker,
		IdemPath:                   filepath.Join(filepath.Dir(sock), "idem.json"),
		Notify:                     notifyTee,
	}
	if *httpAddr != "" {
		// Webhook ingress (INC-50, G14): strictly opt-in. The registry and
		// the bound-address rendezvous live beside the socket in the data
		// dir, so `ar hook` finds them without asking the daemon.
		srv.HTTPAddr = *httpAddr
		srv.HTTPAddrFile = hookAddrPath()
		srv.HooksPath = hooksPath()
	}
	reconcileNotifications(notifier)
	fmt.Fprintf(stderr, "daemon on %s\n", sock)
	if err := srv.ListenAndServe(ctx); err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	return ExitOK
}

// daemonDetach re-execs `agentrunner daemon` as a new SESSION LEADER (setsid),
// with stdio redirected to a log file, so it survives this terminal closing
// (T3). Go cannot fork+setsid in-process — the runtime is multi-threaded — so
// the idiomatic daemonize is a re-exec of self. The parent waits for the
// socket to accept, reports where the daemon lives, and returns; the child
// keeps running, now with no controlling terminal to SIGHUP it.
func daemonDetach(httpAddr string, stdout, stderr io.Writer) int {
	sock, err := socketPath()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	// Idempotent start: a live daemon on the socket is success, not a
	// duplicate — starting the runtime twice must not split the session space.
	if conn, derr := net.Dial("unix", sock); derr == nil {
		_ = conn.Close()
		fmt.Fprintf(stderr, "daemon already running on %s\n", sock)
		return ExitOK
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	logPath := filepath.Join(filepath.Dir(sock), "daemon.log")
	logF, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	defer func() { _ = logF.Close() }()

	daemonArgs := []string{"daemon"}
	if httpAddr != "" {
		daemonArgs = append(daemonArgs, "--http", httpAddr)
	}
	cmd := exec.Command(exe, daemonArgs...)
	cmd.Stdin = nil
	cmd.Stdout = logF
	cmd.Stderr = logF
	cmd.Env = os.Environ()                               // carry GEMINI_API_KEY etc.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from the tty
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "agentrunner: start daemon: %v\n", err)
		return ExitRun
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release() // do not reap; the child outlives us

	// Wait for the socket to actually accept before declaring success, so a
	// spec/bind failure surfaces here instead of silently later.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if conn, derr := net.Dial("unix", sock); derr == nil {
			_ = conn.Close()
			fmt.Fprintf(stdout, "daemon started (pid %d)\n", pid)
			fmt.Fprintf(stderr, "daemon on %s  (logs: %s ; stop: kill %d)\n", sock, logPath, pid)
			return ExitOK
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Fprintf(stderr, "agentrunner: daemon did not come up within 5s — see %s\n", logPath)
	return ExitRun
}

// buildNotifier opens the notifier stream, reads the USER-level channel
// config (carve-out: project settings never redirect notifications), and
// returns the non-blocking tee the daemon calls from run emit paths — a
// buffered queue drained by one goroutine; overflow drops (the journal +
// startup reconciliation are the safety net for the moments that matter).
func buildNotifier(ctx context.Context, stderr io.Writer) (*notify.Notifier, func(protocol.Event), error) {
	userPath, err := runtime.UserConfigPath()
	if err != nil {
		return nil, nil, err
	}
	user, err := config.LoadFile(userPath)
	if err != nil {
		return nil, nil, err
	}
	data, err := runtime.DataDir()
	if err != nil {
		return nil, nil, err
	}
	notifier, err := notify.Open(filepath.Join(data, "notifier"), user.Notify.Command, stderr)
	if err != nil {
		return nil, nil, err
	}
	ch := make(chan protocol.Event, 64)
	go func() {
		for {
			select {
			case e := <-ch:
				notifier.Notify(toNotification(e))
			case <-ctx.Done():
				// Shutdown: deliver what is already queued (a run_end has no
				// startup reconciliation — dropping it here loses the moment
				// permanently; S6 review).
				for {
					select {
					case e := <-ch:
						notifier.Notify(toNotification(e))
					default:
						return
					}
				}
			}
		}
	}()
	tee := func(e protocol.Event) {
		select {
		case ch <- e:
		default:
		}
	}
	return notifier, tee, nil
}

// toNotification maps a lifecycle event to its deduplicated notification.
// The keys MUST match reconcileNotifications' keys for the same moment.
func toNotification(e protocol.Event) notify.Notification {
	switch e.Kind {
	case protocol.KindIteration:
		return notify.Notification{
			Key:  fmt.Sprintf("iteration/%s/%d", e.Session, e.N),
			Kind: "iteration", Session: e.Session,
			Text: fmt.Sprintf("%s: %s", e.Session, e.Text),
		}
	case protocol.KindApprovalRequest:
		return notify.Notification{
			Key:  "approval/" + e.Session + "/" + e.ApprovalID,
			Kind: "approval", Session: e.Session,
			Text: fmt.Sprintf("approval needed on %s: %s %s (agentrunner approve %s %s approve|deny)",
				e.Session, e.Tool, truncate(e.Args, 80), e.Session, e.ApprovalID),
		}
	default: // run_end
		return notify.Notification{
			Key:  "run_end/" + e.Session,
			Kind: "run_end", Session: e.Session,
			Text: fmt.Sprintf("run %s ended: %s", e.Session, e.Reason),
		}
	}
}

// reconcileNotifications is the startup sweep (启动对账): sessions idle on
// an approval get their notification (re)sent unless the journaled sent set
// already has it — a daemon that died between the ask and the notify never
// loses the moment. Ended-run reconciliation is deliberately NOT done: on
// first adoption it would replay every historical session's ending as a
// fresh notification (记档).
func reconcileNotifications(notifier *notify.Notifier) {
	data, err := runtime.DataDir()
	if err != nil {
		return
	}
	entries, err := os.ReadDir(filepath.Join(data, "sessions"))
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(data, "sessions", e.Name())
		events, err := store.ReadEvents(dir)
		if err != nil {
			continue
		}
		s, err := state.Fold(events)
		if err != nil || s.Waiting == nil || s.Waiting.Kind != event.WaitApproval {
			continue
		}
		var req event.ApprovalRequested
		if err := json.Unmarshal(s.Waiting.Detail, &req); err != nil {
			continue
		}
		notifier.Notify(notify.Notification{
			Key:  "approval/" + e.Name() + "/" + req.ApprovalID,
			Kind: "approval", Session: e.Name(),
			Text: fmt.Sprintf("approval waiting on %s (idle; resume or approve %s %s)",
				e.Name(), e.Name(), req.ApprovalID),
		})
	}
}

// socketApprovals adapts the daemon's ApprovalBroker to the agent's
// resolver seam. It EMITS the ask onto the hosted run's event stream before
// going idle — child loops are silent (no Out sink) but share this resolver,
// so a child's ask surfaces on the attach stream too (上卷). req.Agent says
// WHO is asking.
type socketApprovals struct {
	broker  *daemon.ApprovalBroker
	session string
	sink    protocol.Sink
}

func (s socketApprovals) Resolve(ctx context.Context, req agent.ApprovalRequest) (agent.ApprovalDecision, error) {
	origin := req.Session
	if origin == "" {
		origin = s.session
	}
	// Register FIRST: concurrent sibling asks can carry identical
	// deterministic ids, and the broker de-dupes with a suffix — the id we
	// SURFACE must be the one an answer can address (S6 review).
	id, ch := s.broker.Register(origin, req.ApprovalID)
	s.sink.Emit(protocol.Event{
		Kind: protocol.KindApprovalRequest, ApprovalID: id,
		Tool: req.ToolName, CallID: req.CallID,
		Args: string(req.Args), Text: req.Agent, Session: origin,
	})
	a, err := s.broker.Wait(ctx, origin, id, ch)
	if err != nil {
		return agent.ApprovalDecision{}, err
	}
	return agent.ApprovalDecision{CommandRef: a.CommandRef, Approve: a.Approve, Reason: a.Reason, Source: "socket", Remember: a.Remember}, nil
}

// hostRunFunc is the daemon's real run wiring — the same assembly as a
// foreground `run` minus the tty concerns (no interrupts; asks idle on the
// approval broker and resolve over the socket).
func hostRunFunc(version string, stderr io.Writer, broker *daemon.ApprovalBroker) daemon.RunFunc {
	return func(ctx context.Context, req daemon.RunRequest, sink protocol.Sink) error {
		spec, err := agent.LoadSpec(req.SpecPath)
		if err != nil {
			return err
		}
		wsRoot := req.Workspace
		if wsRoot == "" {
			wsRoot = "."
		}
		ws, err := workspace.New(wsRoot)
		if err != nil {
			return err
		}
		prov, err := defaultProviderFactory(ctx, spec.Model.Provider)
		if err != nil {
			return err
		}
		sessionDir, err := runtime.SessionDir(req.SessionID)
		if err != nil {
			return err
		}
		events, err := store.OpenEventStore(sessionDir)
		if err != nil {
			return err
		}
		defer func() { _ = events.Close() }()

		mode := spec.Mode
		if req.Mode != "" {
			mode = req.Mode
		}
		pipe, hooks, err := buildPipeline(ws, spec.Permissions, mode, spec.Budget.MaxTotalTokens, stderr)
		if err != nil {
			return err
		}
		loop := &agent.Loop{
			Spec:              spec,
			Provider:          prov,
			Judge:             prov,
			Exec:              &tool.Executor{WS: ws, Session: req.SessionID},
			Store:             events,
			Clock:             clock.Real{},
			Out:               sink,
			SessionID:         req.SessionID,
			Version:           version,
			Pipeline:          pipe,
			Mode:              mode,
			Hooks:             hooks,
			Approvals:         socketApprovals{broker: broker, session: req.SessionID, sink: sink},
			SubSpecs:          siblingSpecResolver(req.SpecPath),
			SpecPath:          req.SpecPath,
			Snapshots:         snapshotStoreFor(ws, stderr),
			UserInputs:        req.Inbox,
			Interrupts:        req.Interrupts,
			Cancels:           req.Cancels,
			Controls:          req.Controls,
			CommandInterrupts: req.CommandInterrupts,
			CommandCancels:    req.CommandCancels,
			Revokes:           req.Revokes,
			Answers:           req.Answers,
			// A top-level hosted session gets the auto session title (INC-52).
			AutoTitle: true,
			// Blackboard publishes mirror onto the attach stream (S6 模块⑤
			// 回访): watchers see the tree's collaboration live; the board
			// stays the read-back truth.
			BoardMirror: func(n blackboard.Note) {
				sink.Emit(protocol.Event{Kind: protocol.KindNote,
					Text: fmt.Sprintf("[%s] %s: %s", n.Topic, n.From, n.Text)})
			},
		}
		_, runErr := loop.Run(ctx, req.Prompt)
		return runErr
	}
}

// persistInputFunc makes sends durable before the ack (v2 收口): redact,
// then append + fsync to the session's mailbox. Redaction here keeps the
// mailbox file as credential-free as the journal it feeds.
func persistInputFunc() func(string, protocol.UserInput) (protocol.UserInput, error) {
	return func(sessionID string, in protocol.UserInput) (protocol.UserInput, error) {
		dir, err := resolveSessionDir(sessionID)
		if err != nil {
			return in, err
		}
		in.Text = redact.FromEnv().String(in.Text)
		return store.AppendInbox(dir, in)
	}
}

func persistCommandFunc() func(string, protocol.SessionCommand) (protocol.SessionCommand, error) {
	return func(sessionID string, cmd protocol.SessionCommand) (protocol.SessionCommand, error) {
		dir, err := resolveSessionDir(sessionID)
		if err != nil {
			return cmd, err
		}
		raw, err := json.Marshal(cmd)
		if err != nil {
			return cmd, err
		}
		raw = redact.FromEnv().JSON(raw)
		if err := json.Unmarshal(raw, &cmd); err != nil {
			return cmd, err
		}
		return store.AppendCommand(dir, cmd)
	}
}

func pendingCommands(sessionID string) ([]protocol.SessionCommand, error) {
	dir, err := resolveSessionDir(sessionID)
	if err != nil {
		return nil, err
	}
	// A child session hosts no tree: only a top-level session (its dir sits
	// directly under sessions/, not under a "sub" hop) hoists descendant
	// approvals. The string "-sub-" alone cannot tell the two apart — a
	// top-level slug may contain it (QA Round1 F-B2).
	pending, err := pendingCommandsInDir(dir)
	if err != nil || filepath.Base(filepath.Dir(dir)) == "sub" {
		return pending, err
	}
	// A child approval is persisted in the exact asking child's CommandLog,
	// but the collaboration has one host: the tree root. On daemon restart,
	// fold all descendant approval tails into the root's replay queue and
	// retain Target so the broker answers the child's rendezvous. Other child
	// commands are deliberately not hoisted; user sends already persist on
	// the root with UserInput.Target, preserving the single-writer route.
	childPending, err := pendingChildApprovals(sessionID, dir)
	if err != nil {
		return nil, err
	}
	return append(pending, childPending...), nil
}

func pendingCommandsInDir(dir string) ([]protocol.SessionCommand, error) {
	events, err := store.ReadEvents(dir)
	if err != nil {
		return nil, err
	}
	handled := map[string]bool{}
	var consumedInputSeq int64
	for _, env := range events {
		if env.Type == event.TypeInputReceived {
			if decoded, derr := event.DecodePayload(env); derr == nil {
				if seq := decoded.(*event.InputReceived).DeliverySeq; seq > consumedInputSeq {
					consumedInputSeq = seq
				}
			}
		}
		if env.CommandID == "" {
			continue
		}
		switch env.Type {
		case event.TypeInputReceived, event.TypeContextCompacted,
			event.TypeGoalAttached, event.TypeGoalUpdated, event.TypeGoalPaused,
			event.TypeGoalResumed, event.TypeGoalCancelled, event.TypeGoalAchieved,
			event.TypeGoalExhausted,
			event.TypeSessionClosed, event.TypeLimitExceeded,
			event.TypeApprovalResponded, event.TypeCommandHandled:
			handled[env.CommandID] = true
		}
	}
	commands, err := store.ReadCommands(dir, 0)
	if err != nil {
		return nil, err
	}
	var pending []protocol.SessionCommand
	for _, cmd := range commands {
		if cmd.Kind == protocol.CommandInput && cmd.CommandSeq <= consumedInputSeq {
			continue
		}
		if cmd.CommandID != "" && handled[cmd.CommandID] {
			continue
		}
		pending = append(pending, cmd)
	}
	return pending, nil
}

func pendingChildApprovals(parentID, parentDir string) ([]protocol.SessionCommand, error) {
	entries, err := os.ReadDir(filepath.Join(parentDir, "sub"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []protocol.SessionCommand
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		childID := parentID + "-sub-" + entry.Name()
		childDir := filepath.Join(parentDir, "sub", entry.Name())
		pending, perr := pendingCommandsInDir(childDir)
		if perr != nil {
			return nil, perr
		}
		for _, cmd := range pending {
			if cmd.Kind != protocol.CommandApproval {
				continue
			}
			cmd.Target = childID
			out = append(out, cmd)
		}
		deeper, derr := pendingChildApprovals(childID, childDir)
		if derr != nil {
			return nil, derr
		}
		out = append(out, deeper...)
	}
	return out, nil
}

func scanPendingCommandSessions() ([]string, error) {
	data, err := runtime.DataDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(data, "sessions"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pending, perr := pendingCommands(entry.Name())
		if perr == nil && len(pending) > 0 {
			ids = append(ids, entry.Name())
		}
	}
	return ids, nil
}

// sessionMarked reports whether a session's journal carries a close/kill
// mark (决策 #30): automatic revival paths check it; explicit sends never
// ask.
func sessionMarked(sessionID string) (bool, error) {
	dir, err := resolveSessionDir(sessionID)
	if err != nil {
		return false, err
	}
	events, err := store.ReadEvents(dir)
	if err != nil {
		return false, err
	}
	s, err := state.Fold(events)
	if err != nil {
		return false, err
	}
	return s.Session.Closed != nil, nil
}

// pendingApproval reports the approval id a session's journal shows it idle on
// (waiting:approval), if any — the daemon's seam for M2 self-heal: after a
// restart lost the in-memory ask, `approve` uses this to confirm the session
// really is waiting on that id before reviving it to re-arm the ask.
func pendingApproval(sessionID string) (string, bool, error) {
	dir, err := resolveSessionDir(sessionID)
	if err != nil {
		return "", false, err
	}
	events, err := store.ReadEvents(dir)
	if err != nil {
		return "", false, err
	}
	s, err := state.Fold(events)
	if err != nil {
		return "", false, err
	}
	if s.Waiting == nil || s.Waiting.Kind != event.WaitApproval {
		return "", false, nil
	}
	var req event.ApprovalRequested
	if err := json.Unmarshal(s.Waiting.Detail, &req); err != nil {
		return "", false, err
	}
	return req.ApprovalID, true, nil
}

// scanSessionTimers derives the pending-timer index from the session
// journals (timer 派生索引): every unmarked session with pending timers
// reports its earliest fire time (a close/kill mark gates this automatic
// path, 决策 #30). Unreadable or unfoldable sessions are skipped — the
// sweep must not die on one corrupt log.
func scanSessionTimers() ([]daemon.SessionTimer, error) {
	data, err := runtime.DataDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(data, "sessions"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []daemon.SessionTimer
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(data, "sessions", e.Name())
		events, err := store.ReadEvents(dir)
		if err != nil {
			continue
		}
		s, err := state.Fold(events)
		if err != nil || s.Session.Closed != nil || len(s.Timers) == 0 {
			continue
		}
		var earliest time.Time
		for _, tm := range s.Timers {
			if earliest.IsZero() || tm.FireAt.Before(earliest) {
				earliest = tm.FireAt
			}
		}
		out = append(out, daemon.SessionTimer{SessionID: e.Name(), FireAt: earliest})
	}
	return out, nil
}

// hostResumeFunc is the daemon's timer-driven resume wiring — the same
// assembly as a foreground `resume` minus the tty: spec and workspace come
// from the journaled SessionStarted, permissions from the journaled layers.
func hostResumeFunc(version string, stderr io.Writer, broker *daemon.ApprovalBroker) func(context.Context, daemon.ResumeRequest, protocol.Sink) error {
	return func(ctx context.Context, req daemon.ResumeRequest, sink protocol.Sink) error {
		sessionID := req.SessionID
		dir, err := resolveSessionDir(sessionID)
		if err != nil {
			return err
		}
		started, err := readSessionStarted(dir)
		if err != nil {
			return err
		}
		if len(started.Spec) == 0 || started.WorkspaceRoot == "" {
			return fmt.Errorf("session %s predates resumable metadata", sessionID)
		}
		// The CURRENT agent may differ from the opening one (决策 #32): a
		// SpecChanged fact supersedes the SessionStarted spec, spec path and
		// permission layers — the revival runs the agent the journal names.
		specJSON, specPath, permLayers := started.Spec, started.SpecPath, started.PermissionLayers
		if changed, cerr := readLatestSpecChange(dir); cerr == nil && changed != nil {
			specJSON, specPath, permLayers = changed.Spec, changed.SpecPath, changed.PermissionLayers
		}
		var spec agent.AgentSpec
		if err := json.Unmarshal(specJSON, &spec); err != nil {
			return fmt.Errorf("journaled spec: %w", err)
		}
		ws, err := workspace.New(started.WorkspaceRoot)
		if err != nil {
			return err
		}
		prov, err := defaultProviderFactory(ctx, spec.Model.Provider)
		if err != nil {
			return err
		}
		events, err := store.OpenEventStore(dir)
		if err != nil {
			return err
		}
		defer func() { _ = events.Close() }()

		var pipe *pipeline.Pipeline
		var hooks *hook.Runner
		if len(permLayers) > 0 {
			var layers [][]pipeline.PermissionRule
			if err := json.Unmarshal(permLayers, &layers); err != nil {
				return fmt.Errorf("journaled permission layers: %w", err)
			}
			pipe, hooks, err = buildPipelineFromLayers(ws, layers, spec.Mode, spec.Budget.MaxTotalTokens, stderr)
		} else {
			pipe, hooks, err = buildPipeline(ws, spec.Permissions, spec.Mode, spec.Budget.MaxTotalTokens, stderr)
		}
		if err != nil {
			return err
		}
		loop := &agent.Loop{
			Spec:      &spec,
			Provider:  prov,
			Judge:     prov,
			Exec:      &tool.Executor{WS: ws, Session: sessionID},
			Store:     events,
			Clock:     clock.Real{},
			Out:       sink,
			SessionID: sessionID,
			Version:   version,
			Pipeline:  pipe,
			Hooks:     hooks,
			Approvals: socketApprovals{broker: broker, session: sessionID, sink: sink},
			Snapshots: snapshotStoreFor(ws, stderr),
		}
		// Every revived session gets the live channels (决策 #31: only one
		// session shape) — it accepts send/interrupt/kill like a freshly
		// hosted one.
		loop.UserInputs = req.Inbox
		loop.Interrupts = req.Interrupts
		loop.Cancels = req.Cancels
		loop.Controls = req.Controls
		loop.CommandInterrupts = req.CommandInterrupts
		loop.CommandCancels = req.CommandCancels
		loop.Revokes = req.Revokes
		loop.Answers = req.Answers
		// A revived top-level session still auto-titles if not yet titled (INC-52).
		loop.AutoTitle = true
		loop.SpecPath = specPath
		if specPath != "" {
			loop.SubSpecs = siblingSpecResolver(specPath)
		}
		_, runErr := loop.Resume(ctx)
		return runErr
	}
}

// attachCmd follows a session hosted by the daemon: journal catch-up first,
// then the live stream. `agentrunner attach [--json] <session-id-or-prefix>`.
func attachCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("attach", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit the event stream as JSON lines instead of rendered text")
	replayOnly := fs.Bool("replay-only", false, "replay the recorded history and exit, without following live output")
	fs.BoolVar(replayOnly, "no-follow", false, "alias for --replay-only")
	if err := fs.Parse(reorderFlags(fs, args)); err != nil {
		return ExitUsage
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(stderr, `usage: agentrunner attach [--json] [--replay-only] <session-id-or-prefix>
replays the whole conversation, then follows live output; Ctrl-C detaches
(the session keeps running). --replay-only prints the history and exits.`)
		return ExitUsage
	}
	// Resolve prefixes locally so the wire carries the full id.
	dir, err := resolveSessionDir(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	session := filepath.Base(dir)

	sock, err := socketPath()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	var sink protocol.Sink
	if *jsonOut {
		sink = protocol.NewJSONSink(stdout)
	} else {
		sink = newTextRenderer(stdout)
	}
	if err := daemon.Dial(sock, daemon.Command{Cmd: "attach", Session: session, ReplayOnly: *replayOnly}, sink.Emit); err != nil {
		daemonDialErr(stderr, err)
		return ExitRun
	}
	return ExitOK
}

// childLifecycleFilter strips a child run's lifecycle FRAMING (run_start /
// run_end) from the shared hub stream: within a hosted series the DRIVER
// owns the lifecycle (iteration / run_end events) — a child's run_end would
// otherwise consume the notifier's run_end/<session> dedup key and shadow
// the series' real ending (found by the s6-05 scenario). The child's work
// (turns, messages, tool calls) still streams.
type childLifecycleFilter struct{ inner protocol.Sink }

func (f childLifecycleFilter) Emit(e protocol.Event) {
	if e.Kind == protocol.KindRunEnd || e.Kind == protocol.KindSessionStart {
		return
	}
	f.inner.Emit(e)
}

// hostDriveFunc is the daemon's IterationDriver wiring — the same assembly
// as a foreground `drive` minus the tty: asks (human verifier,
// finish_series) route over the approval broker, and the driver's lifecycle
// tees to watchers and the notifier through the hub sink.
func hostDriveFunc(version string, stderr io.Writer, broker *daemon.ApprovalBroker) func(context.Context, daemon.DriveRequest, protocol.Sink) error {
	return func(ctx context.Context, req daemon.DriveRequest, sink protocol.Sink) error {
		spec, err := driver.LoadSpec(req.SpecPath)
		if err != nil {
			return err
		}
		d, cleanup, err := assembleHostedDriver(ctx, version, req.SpecPath, spec,
			req.Workspace, req.SessionID, broker, sink, stderr)
		if err != nil {
			return err
		}
		defer cleanup()
		_, runErr := d.Run(ctx)
		return runErr
	}
}

// hostResumeDriveFunc is the boot-sweep counterpart of hostDriveFunc (INC-54,
// G22): it rebuilds the SAME driver assembly from the journal (the spec and
// workspace root ride DriverStarted, mirroring how hostResumeFunc reconstructs
// an agent session from SessionStarted) and calls Driver.Resume — whose cron
// cadence backfills the slots missed while the daemon was down. specPath is
// unknown after a restart, so sibling sub-specs resolve against builtins/CWD.
func hostResumeDriveFunc(version string, stderr io.Writer, broker *daemon.ApprovalBroker) func(context.Context, daemon.DriveRequest, protocol.Sink) error {
	return func(ctx context.Context, req daemon.DriveRequest, sink protocol.Sink) error {
		sessionDir, err := runtime.SessionDir(req.SessionID)
		if err != nil {
			return err
		}
		started, err := readDriverStarted(sessionDir)
		if err != nil {
			return err
		}
		if len(started.Spec) == 0 {
			return fmt.Errorf("drive %s predates resumable metadata", req.SessionID)
		}
		var spec driver.DriverSpec
		if err := json.Unmarshal(started.Spec, &spec); err != nil {
			return fmt.Errorf("journaled driver spec: %w", err)
		}
		d, cleanup, err := assembleHostedDriver(ctx, version, "", &spec,
			started.WorkspaceRoot, req.SessionID, broker, sink, stderr)
		if err != nil {
			return err
		}
		defer cleanup()
		_, runErr := d.Resume(ctx)
		return runErr
	}
}

// assembleHostedDriver builds the daemon's IterationDriver over an already
// loaded spec — the shared assembly behind a fresh drive (hostDriveFunc → Run)
// and a boot-sweep resume (hostResumeDriveFunc → Resume). It returns the driver
// and a cleanup that closes the opened store; the caller defers it after
// Run/Resume returns. specPath is "" on the resume path (the spec came from the
// journal, not a file), so sibling sub-specs resolve against builtins/CWD.
func assembleHostedDriver(ctx context.Context, version, specPath string, spec *driver.DriverSpec,
	wsRoot, sessionID string, broker *daemon.ApprovalBroker, sink protocol.Sink, stderr io.Writer) (*driver.Driver, func(), error) {
	if wsRoot == "" {
		wsRoot = "."
	}
	ws, err := workspace.New(wsRoot)
	if err != nil {
		return nil, nil, err
	}
	prov, err := defaultProviderFactory(ctx, spec.Agent.Model.Provider)
	if err != nil {
		return nil, nil, err
	}
	sessionDir, err := runtime.SessionDir(sessionID)
	if err != nil {
		return nil, nil, err
	}
	dStore, err := store.OpenEventStore(sessionDir)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = dStore.Close() }
	artifacts, err := store.OpenArtifactStore(filepath.Join(sessionDir, "artifacts"))
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	approvals := socketApprovals{broker: broker, session: sessionID, sink: sink}
	exec := &tool.Executor{WS: ws, Session: sessionID}
	// Same verifier-adjudication construction as the foreground drive:
	// user/project rules first, trailing driver-trust allow.
	verifierPipe, _, err := buildPipeline(ws, []pipeline.PermissionRule{{Action: "allow"}}, "", 0, stderr)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	d := &driver.Driver{
		Spec:      spec,
		SpecPath:  specPath,
		Store:     dStore,
		Clock:     clock.Real{},
		DriverID:  sessionID,
		Exec:      exec,
		Judge:     prov,
		Approvals: approvals,
		Artifacts: artifacts,
		Out:       sink,
		Pipeline:  verifierPipe,
		NewChild: func(cs *store.EventStore, session string, iter, budgetTokens int) *agent.Loop {
			frozen := *spec.Agent
			if budgetTokens > 0 {
				frozen.Budget.MaxTotalTokens = budgetTokens
			}
			pipe, hooks, perr := buildPipeline(ws, frozen.Permissions, frozen.Mode,
				frozen.Budget.MaxTotalTokens, stderr)
			if perr != nil {
				fmt.Fprintln(stderr, perr)
			}
			return &agent.Loop{
				Spec:      &frozen,
				Provider:  prov,
				Judge:     prov,
				Exec:      &tool.Executor{WS: ws, Session: session},
				Store:     cs,
				Clock:     clock.Real{},
				Out:       childLifecycleFilter{inner: sink},
				SessionID: session,
				Version:   version,
				Pipeline:  pipe,
				Mode:      frozen.Mode,
				Hooks:     hooks,
				Approvals: approvals,
				SubSpecs:  siblingSpecResolver(specPath),
			}
		},
		// Best-of-N (schedule=parallel): attempt face binds to its worktree.
		Snapshots: snapshotStoreFor(ws, stderr),
		NewChildAt: func(cs *store.EventStore, session string, iter, budgetTokens int, worktree string) *agent.Loop {
			frozen := *spec.Agent
			if budgetTokens > 0 {
				frozen.Budget.MaxTotalTokens = budgetTokens
			}
			wtWS, werr := workspace.New(worktree)
			if werr != nil {
				fmt.Fprintln(stderr, werr)
				wtWS = ws
			}
			pipe, hooks, perr := buildPipeline(wtWS, frozen.Permissions, frozen.Mode,
				frozen.Budget.MaxTotalTokens, stderr)
			if perr != nil {
				fmt.Fprintln(stderr, perr)
			}
			return &agent.Loop{
				Spec:      &frozen,
				Provider:  prov,
				Judge:     prov,
				Exec:      &tool.Executor{WS: wtWS, Session: session},
				Store:     cs,
				Clock:     clock.Real{},
				Out:       childLifecycleFilter{inner: sink},
				SessionID: session,
				Version:   version,
				Pipeline:  pipe,
				Mode:      frozen.Mode,
				Hooks:     hooks,
				Approvals: approvals,
				SubSpecs:  siblingSpecResolver(specPath),
			}
		},
	}
	return d, cleanup, nil
}

// readDriverStarted reads a drive session's stream header — the journaled
// DriverSpec + workspace root a boot-sweep resume rebuilds the assembly from
// (INC-54; mirrors readSessionStarted for agent sessions).
func readDriverStarted(dir string) (*event.DriverStarted, error) {
	events, err := store.ReadEvents(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		if e.Type == event.TypeDriverStarted {
			decoded, derr := event.DecodePayload(e)
			if derr != nil {
				return nil, derr
			}
			return decoded.(*event.DriverStarted), nil
		}
	}
	return nil, fmt.Errorf("session %s has no DriverStarted header", filepath.Base(dir))
}

// scanDriveSessions derives the boot-sweep drive index (INC-54, G22): every
// LOOP-MODE drive (interval/cron/self_paced) whose journal shows it still
// running. A terminal DriverCompleted is a drive's explicit-end mark (决策 #30:
// a finished or explicitly-stopped series left one, and automatic paths must
// not cross it), so it excludes the session. Goal-mode / parallel drives are
// bounded runs, out of the cron boot-sweep scope. Unreadable or agent-shaped
// sessions are skipped — the sweep must not die on one non-drive log.
func scanDriveSessions() ([]string, error) {
	data, err := runtime.DataDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(data, "sessions"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(data, "sessions", e.Name())
		events, err := store.ReadEvents(dir)
		if err != nil || len(events) == 0 || events[0].Type != event.TypeDriverStarted {
			continue // unreadable, empty, or an agent session (SessionStarted header)
		}
		started, err := readDriverStarted(dir)
		if err != nil {
			continue
		}
		var spec driver.DriverSpec
		if err := json.Unmarshal(started.Spec, &spec); err != nil {
			continue
		}
		switch spec.Schedule {
		case driver.ScheduleInterval, driver.ScheduleCron, driver.ScheduleSelfPaced:
		default:
			continue // immediate (goal) / parallel: out of the cron boot-sweep scope
		}
		st, err := driver.Fold(events)
		if err != nil || st.Status == driver.StatusEnded {
			continue // ended = the drive's explicit-end mark (决策 #30)
		}
		out = append(out, e.Name())
	}
	return out, nil
}

// scanStrandedSessions derives the stranded-session boot-sweep index
// (INC-71, G22a): top-level agent sessions whose journal folds to RUNNING
// with no live writer — the previous host died mid-turn. Cleanly parked
// (waiting) sessions are NOT stranded: nothing was in flight, an input will
// revive them. Marks are re-checked by hostResume's automatic path (决策
// #30); unreadable or non-agent journals are skipped — the sweep must not
// die on one bad log.
func scanStrandedSessions() ([]string, error) {
	data, err := runtime.DataDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(data, "sessions"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(data, "sessions", e.Name())
		events, err := store.ReadEvents(dir)
		if err != nil || len(events) == 0 || events[0].Type != event.TypeSessionStarted {
			continue // unreadable, empty, or a driver stream
		}
		if events[len(events)-1].Type == event.TypeSessionClosed {
			continue // terminal — cheap skip before folding
		}
		st, err := state.Fold(events)
		if err != nil || st.Session.Status != state.StatusRunning {
			continue // parked (waiting) or terminal: not stranded
		}
		if store.HasLiveWriter(dir) {
			continue // another host is live — not ours to touch
		}
		out = append(out, e.Name())
	}
	return out, nil
}

// approveCmd answers a daemon-hosted ask: `agentrunner approve
// <session-id-or-prefix> <approval-id> <approve|deny> [reason]`.
func approveCmd(args []string, stdout, stderr io.Writer) int {
	// `--always` (INC-17, G5): on approve, remember an allow rule for next
	// session. Strip it wherever it appears; the rest are positional.
	remember := false
	positional := args[:0:0]
	for _, a := range args {
		if a == "--always" {
			remember = true
			continue
		}
		positional = append(positional, a)
	}
	args = positional
	if len(args) < 3 || len(args) > 4 {
		fmt.Fprintln(stderr, "usage: agentrunner approve <session-id-or-prefix> <approval-id> <approve|deny> [reason] [--always]")
		return ExitUsage
	}
	session, err := resolveApprovalSession(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	decision := args[2]
	if decision != "approve" && decision != "deny" {
		fmt.Fprintln(stderr, "agentrunner: decision must be approve or deny")
		return ExitUsage
	}
	reason := ""
	if len(args) == 4 {
		reason = args[3]
	}
	sock, err := socketPath()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	code := ExitOK
	err = daemon.Dial(sock, daemon.Command{
		Cmd: "approve", Session: session, ApprovalID: args[1],
		Decision: decision, Reason: reason, Remember: remember, CommandID: event.NewCommandID(),
	}, func(e protocol.Event) {
		if e.Kind == protocol.KindError {
			code = ExitRun
		}
		fmt.Fprintln(stdout, e.Text)
	})
	if err != nil {
		daemonDialErr(stderr, err)
		return ExitRun
	}
	return code
}

func resolveApprovalSession(arg string) (string, error) {
	if _, err := resolveSessionDir(arg); err != nil {
		return "", err
	}
	return resolvePrefixLenient(arg), nil
}

// submitCmd hands a run — or, with --drive, an IterationDriver series — to
// the daemon and streams it until it ends; the work survives this client.
// `agentrunner submit [flags] <spec.yaml> "prompt"` /
// `agentrunner submit --drive [flags] <driver.yaml>`.
func submitCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceDir := fs.String("workspace", ".", "workspace root (default: current directory)")
	mode := fs.String("mode", "", "run mode: default|plan|acceptEdits (overrides spec)")
	jsonOut := fs.Bool("json", false, "emit the event stream as JSON lines")
	drive := fs.Bool("drive", false, "submit a driver spec (prompt lives in the spec)")
	idem := fs.String("idem", "", "idempotency key: a retried submit with the same key reattaches instead of starting a duplicate")
	if err := fs.Parse(reorderFlags(fs, args)); err != nil {
		return ExitUsage
	}
	rest := fs.Args()
	if (*drive && len(rest) != 1) || (!*drive && len(rest) != 2) {
		fmt.Fprintln(stderr, "usage: agentrunner submit [flags] <spec.yaml> \"prompt\"  |  submit --drive [flags] <driver.yaml>")
		return ExitUsage
	}
	if !*drive && rest[1] == "" {
		fmt.Fprintln(stderr, "agentrunner: submit needs a non-empty prompt")
		return ExitUsage
	}
	specPath, err := filepath.Abs(rest[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}
	wsAbs, err := filepath.Abs(*workspaceDir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}
	// Same preflight as `new`: a daemon-side early failure surfaces as an
	// error event, but validating here fails fast and never mints a session
	// for a run that cannot start (QA Round1 F-A02).
	if !*drive {
		loaded, err := agent.LoadSpec(specPath)
		if err != nil {
			fmt.Fprintf(stderr, "agentrunner: %v\n", err)
			return ExitUsage
		}
		// Validate the provider up front too, like `new` does — otherwise an
		// unknown provider mints a session that fails at runtime (QA Wave1
		// alice-04 / dave-01).
		if !knownProviderName(loaded.Model.Provider) {
			fmt.Fprintf(stderr, "agentrunner: unknown provider %q (available: gemini, anthropic, scripted)\n", loaded.Model.Provider)
			return ExitUsage
		}
	}
	if st, err := os.Stat(wsAbs); err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "agentrunner: workspace root %s is not a directory\n", wsAbs)
		return ExitUsage
	}
	sock, err := socketPath()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	var sink protocol.Sink
	if *jsonOut {
		sink = protocol.NewJSONSink(stdout)
	} else {
		sink = newTextRenderer(stdout)
	}
	reason := ""
	sawIdle := false
	announced := false
	sid := ""
	cmd := daemon.Command{Cmd: "run", SpecPath: specPath, Workspace: wsAbs, Mode: *mode, IdemKey: *idem}
	if *drive {
		cmd = daemon.Command{Cmd: "drive", SpecPath: specPath, Workspace: wsAbs, IdemKey: *idem}
	} else {
		cmd.Prompt = rest[1]
	}
	err = daemon.DialUntil(sock, cmd, func(e protocol.Event) bool {
		if sid == "" && e.Session != "" {
			sid = e.Session
		}
		if e.Kind == protocol.KindSessionStart && e.Session != "" {
			// Announced once: the daemon's ack and the loop's own emit both
			// carry this kind (same dedup as followTurn).
			if !announced {
				fmt.Fprintf(stderr, "session %s\n", e.Session)
				announced = true
			}
			sink.Emit(e)
			return true
		}
		// A tree member's live event (INC-12.6) is not this run's lifecycle:
		// its idle/run_end must not end the follow.
		if sid != "" && e.Session != "" && e.Session != sid {
			sink.Emit(e)
			return true
		}
		switch e.Kind {
		case protocol.KindRunEnd:
			reason = e.Reason
			sink.Emit(e)
			return false // terminal event (failure/close, or drive series end)
		case protocol.KindIdle:
			if !*drive {
				// 决策 #31: a one-shot run parks at standby instead of
				// ending. The work is done — detach; the session stays
				// resident and `send` revives it.
				sawIdle = true
				sink.Emit(e)
				return false
			}
		}
		sink.Emit(e)
		return true
	})
	if err != nil {
		daemonDialErr(stderr, err)
		return ExitRun
	}
	if *drive {
		// No run_end in the stream means the series died before its terminal
		// event (e.g. spec failure inside the daemon) — that is a failure.
		dspec, derr := driver.LoadSpec(specPath)
		if derr != nil || !driveSucceeded(dspec, reason) {
			return ExitRun
		}
		return ExitOK
	}
	// Standby idle IS completion under the quiescence model; run_end appears
	// only on failure or close. Neither in the stream means the run died
	// before its first idle (e.g. spec/provider failure inside the daemon).
	if sawIdle || reason == "completed" {
		return ExitOK
	}
	return ExitRun
}
