// Package daemon is the resident runtime (S6 模块④, DESIGN §运行形态):
// a unix-socket server hosting runs so they outlive any single client. The
// wire protocol is the protocol package's JSON lines — the SAME encoding as
// `run --json` — bidirectionally: client→server lines are commands,
// server→client lines are output events. The daemon owns no run semantics:
// it hosts loops built by an injected RunFunc and multiplexes their output.
package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
)

// Command is one client→server line.
type Command struct {
	Cmd string `json:"cmd"` // ping | run | drive | attach | approve
	// CommandID is caller-minted and stable across retries. Session-mutating
	// commands use it as their durable idempotency key.
	CommandID string `json:"command_id,omitempty"`

	// run
	SpecPath  string `json:"spec_path,omitempty"`
	Task      string `json:"task,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Mode      string `json:"mode,omitempty"`

	// attach / approve / send
	Session string `json:"session,omitempty"`
	// unqueue (INC-46): the queued input command to withdraw.
	TargetCommandID string `json:"target_command_id,omitempty"`

	// send
	Text      string                 `json:"text,omitempty"` // a user message for the session
	Principal string                 `json:"principal,omitempty"`
	Source    string                 `json:"source,omitempty"`
	Trust     string                 `json:"trust,omitempty"`
	Content   []protocol.ContentPart `json:"content,omitempty"`
	// Images are attachments riding a send (v2 M4.1); bytes are base64 on
	// the wire, the agent CAS-stores them before journaling the input.
	Images []protocol.ImageAttachment `json:"images,omitempty"`
	// Files are arbitrary-type attachments riding a send (INC-9: PDF / any
	// file). Same wire/CAS treatment as Images; MediaType drives the provider
	// mapping.
	Files []protocol.FileAttachment `json:"files,omitempty"`
	// Delivery is the per-message delivery mode (INC-43): "" / "queue" (default,
	// next turn) or "steer" (current turn's next safe boundary). Threaded onto
	// the durable UserInput; see protocol.UserInput.Delivery.
	Delivery string `json:"delivery,omitempty"`
	// Goal carries the parameters of a goal-attach / goal-update control
	// (INC-D1). pause/resume/cancel need only the command verb.
	Goal *protocol.GoalControl `json:"goal,omitempty"`
	// Follow keeps the send connection open after the "delivered" ack,
	// streaming the session's live events until the client disconnects
	// (INC-2: the reply becomes visible on the send itself). Subscribe
	// happens BEFORE the input is posted so no reply event can slip
	// between delivery and the stream. Detach = close the connection,
	// same semantics as attach (订阅不改结果).
	Follow bool `json:"follow,omitempty"`

	// ReplayOnly (attach) replays the recorded history and stops, instead of
	// following live output after catch-up — so `attach --replay-only` dumps a
	// session's transcript without hijacking the terminal to tail a still-live
	// one (黑盒 R2-E-5).
	ReplayOnly bool `json:"replay_only,omitempty"`

	// kill
	Handle string `json:"handle,omitempty"` // a child/task handle to cancel

	// compact (G7): an optional focus for the manual summarizer.
	Directive string `json:"directive,omitempty"`

	// approve
	ApprovalID string `json:"approval_id,omitempty"`
	Decision   string `json:"decision,omitempty"` // approve | deny
	Reason     string `json:"reason,omitempty"`
	Remember   bool   `json:"remember,omitempty"` // INC-17: allow-and-don't-ask-again

	// IdemKey makes run/drive submission idempotent within the daemon's
	// lifetime (DESIGN S6 修订): a retry with the same key attaches to the
	// session the first submission created instead of minting a new one.
	IdemKey string `json:"idem_key,omitempty"`
}

func attributedCommand(cmd Command, command protocol.SessionCommand) protocol.SessionCommand {
	command.Principal, command.Source, command.Trust = cmd.Principal, cmd.Source, cmd.Trust
	if command.Principal == "" {
		command.Principal = "local-user"
	}
	if command.Source == "" {
		command.Source = "unix-socket"
	}
	if command.Trust == "" {
		command.Trust = "local"
	}
	return command
}

// RunRequest is what the daemon hands the injected runner. Every hosted
// session gets the live channels (决策 #31: only one session shape).
// Inbox delivers user inputs — the runner wires it to the Loop's
// UserInputs; closing it is the close gesture.
type RunRequest struct {
	SessionID         string
	SpecPath          string
	Task              string
	Workspace         string
	Mode              string
	Inbox             <-chan protocol.UserInput
	Interrupts        <-chan struct{}
	Cancels           <-chan string
	Controls          <-chan protocol.Control
	CommandInterrupts <-chan protocol.CommandRef
	CommandCancels    <-chan protocol.CancelCommand
	Revokes           <-chan protocol.Revoke
}

// RunFunc hosts one run to completion, emitting output events to sink. The
// CLI injects the real wiring (provider, pipeline, store); tests inject
// fakes. It MUST journal through the normal store so attach replay works.
type RunFunc func(ctx context.Context, req RunRequest, sink protocol.Sink) error

// ResumeRequest is what the daemon hands the resume runner. The channels
// are always provided and always wired (决策 #31: only one session shape).
type ResumeRequest struct {
	SessionID         string
	Inbox             <-chan protocol.UserInput
	Interrupts        <-chan struct{}
	Cancels           <-chan string
	Controls          <-chan protocol.Control
	CommandInterrupts <-chan protocol.CommandRef
	CommandCancels    <-chan protocol.CancelCommand
	Revokes           <-chan protocol.Revoke
}

// DriveRequest is a hosted IterationDriver series (S6 完成标志①: a series
// runs unattended, its lifecycle reaching watchers and the notifier).
type DriveRequest struct {
	SessionID string
	SpecPath  string
	Workspace string
}

// NewSessionID mints a session id for a hosted run; injected for
// deterministic tests.
type NewSessionID func(task string) string

// Server hosts runs behind a unix socket.
type Server struct {
	SocketPath string
	Run        RunFunc
	NewID      NewSessionID
	// SplitAddress resolves a session address into (host, target): target
	// non-empty means a child session hosted by its tree root `host`. The
	// CLI wires a store-aware resolver — a TOP-LEVEL slug may itself
	// contain "-sub-" (ids are minted from free task text, QA Round1
	// F-B2), which the naive split misreads as child addressing. nil
	// falls back to the first "-sub-" split.
	SplitAddress func(session string) (host, target string)
	// Replay renders a session's journal as output events for attach
	// catch-up (补读). nil = attach serves live events only.
	Replay func(sessionID string, sink protocol.Sink) error
	// ScanTimers derives the idle sessions' pending-timer index from
	// their journals; Resume hosts a session resume (same wiring as a
	// foreground `resume`). Both non-nil → the daemon runs the durable
	// timer sweeper: expired timers trigger a hosted resume, whose own
	// sweep journals TimerFired. Resume also serves send-driven revival
	// (v2 M5.1): the request carries conversational channels which the
	// runner wires iff the journal says the session is conversational —
	// the daemon itself stays free of run semantics. Clock nil = real time.
	ScanTimers func() ([]SessionTimer, error)
	Resume     func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error
	// PersistInput makes a send durable BEFORE the ack (v2 收口, 铁律
	// "崩溃不丢输入"): it appends to the session's mailbox, fsyncs, and
	// returns the input stamped with its DeliverySeq. nil = no durability
	// (tests); the ack then only means enqueued.
	PersistInput func(sessionID string, in protocol.UserInput) (protocol.UserInput, error)
	// PersistCommand is the unified durable command log. When present it is
	// used for input, control, close, interrupt, approval, and kill; the
	// legacy PersistInput seam remains for older embedders/tests.
	PersistCommand func(sessionID string, cmd protocol.SessionCommand) (protocol.SessionCommand, error)
	// PendingCommands derives unhandled accepted commands from command log
	// receipts versus journal Envelope.CommandID facts.
	PendingCommands            func(sessionID string) ([]protocol.SessionCommand, error)
	ScanPendingCommandSessions func() ([]string, error)
	// SessionMarked reports whether a session's journal carries a
	// close/kill mark (决策 #30). AUTOMATIC revival (timer sweep) checks it
	// and skips marked sessions; an explicit send never does — any session
	// lawfully continues on a user's gesture. nil = never marked (tests).
	SessionMarked func(sessionID string) (marked bool, err error)
	// PendingApproval reports the approval id a session's journal shows it
	// idle on (waiting:approval), if any. handleApprove uses it to tell a
	// genuinely-stuck approval — the daemon restarted and the in-memory broker
	// lost the ask (M2) — from a stale one, so it only revives-to-re-arm the
	// former. nil = the self-heal is off (an unanswerable ask just reports
	// "no pending approval", the pre-fix behavior).
	PendingApproval func(sessionID string) (approvalID string, ok bool, err error)
	// Drive hosts an IterationDriver series (nil = the drive command is
	// refused). Same hub/registry semantics as Run.
	Drive func(ctx context.Context, req DriveRequest, sink protocol.Sink) error
	Clock clock.Clock
	// Approvals is the cross-process ask rendezvous: hosted runs idle here
	// (the CLI's resolver adapter calls Ask) and `approve` commands answer.
	// nil = the approve command is refused.
	Approvals *ApprovalBroker
	// IdemPath persists the idem_key → session index (S7 还债: a submit
	// retry AFTER a daemon restart still finds its session). Empty = the
	// index lives for the daemon's lifetime only.
	IdemPath string
	// Notify receives LIFECYCLE events (run_end, approval_request) from
	// every hosted run — the notifier's live tee (S6 模块⑤). It MUST NOT
	// block: the caller is the run's emit path.
	Notify func(protocol.Event)

	mu     sync.Mutex
	runs   map[string]*hostedRun
	failed map[string]bool   // sessions whose timer-driven resume errored
	idem   map[string]string // idem_key → session (daemon lifetime)
	conns  map[net.Conn]struct{}
	ln     net.Listener
	wg     sync.WaitGroup
	// runsWG tracks HOSTED RUNS (not connections): graceful shutdown waits
	// for every run to reach its terminal journal event after the ctx
	// cancel propagates — a routine deploy leaves zero in-doubt sessions
	// (DESIGN §运行形态: 优雅停机). Add happens under mu against stopping,
	// so a late connection can never Add after the shutdown Wait began.
	runsWG   sync.WaitGroup
	stopping bool
	// commandMu serializes durable append, recovery replay and live enqueue.
	// Therefore command-log sequence is also the single delivery order.
	commandMu sync.Mutex // serializes durable append + live enqueue order
}

// hostedRun is one live run's broadcast hub.
type hostedRun struct {
	id     string
	notify func(protocol.Event)
	mu     sync.Mutex
	subs   map[chan protocol.Event]struct{}
	done   bool
	// inbox delivers conversational user inputs to the hosted Loop (v2
	// M1.2). Buffered so `send` never blocks on the loop's turn; nil for a
	// task run. Closed at `close`/shutdown.
	inbox chan protocol.UserInput
	// interrupts carries the out-of-band interrupt signal (v2 M2.3): a
	// during-turn interrupt steers (cancels the current activity); an idle
	// interrupt closes. Buffered 1 like the terminal Ctrl-C channel.
	interrupts chan struct{}
	// cancels delivers handles to kill out of band (v2 M3.2): `kill <handle>`
	// cancels one running child/task. Buffered for non-blocking delivery.
	cancels chan string
	// controls delivers session-maintenance signals out of band (G7): manual
	// compact/clear. Buffered for non-blocking delivery.
	controls          chan protocol.Control
	commandInterrupts chan protocol.CommandRef
	commandCancels    chan protocol.CancelCommand
	// revokes delivers queued-input withdrawals (INC-46): the loop keeps a
	// revoked-target set and consumes matching inputs as InputRevoked.
	revokes         chan protocol.Revoke
	pendingCommands []protocol.SessionCommand
	postedCommands  map[string]struct{}
	commandWake     chan struct{}
	commandStop     chan struct{}
	commandPumpWG   sync.WaitGroup
	answerApproval  func(protocol.SessionCommand) bool
	// approvalGiveUp caps delivery attempts per approval answer before the
	// pump drops it (0 = the ~10s default); tests shrink it.
	approvalGiveUp int
	// stop tears the hosted loop down (决策 #32 agent switch): a plain ctx
	// cancel — no mark, no ending; the journal simply stops mid-standby and
	// the next send revives it (with whatever spec the journal then names).
	stop context.CancelFunc
}

func newHostedRun(id string, notify func(protocol.Event), interactive bool) *hostedRun {
	h := &hostedRun{id: id, notify: notify, subs: map[chan protocol.Event]struct{}{}}
	if !interactive {
		return h
	}
	h.inbox = make(chan protocol.UserInput, 64)
	h.interrupts = make(chan struct{}, 1)
	h.cancels = make(chan string, 8)
	h.controls = make(chan protocol.Control, 8)
	h.commandInterrupts = make(chan protocol.CommandRef, 1)
	h.commandCancels = make(chan protocol.CancelCommand, 8)
	h.revokes = make(chan protocol.Revoke, 8)
	h.commandWake = make(chan struct{}, 1)
	h.commandStop = make(chan struct{})
	h.postedCommands = map[string]struct{}{}
	h.commandPumpWG.Add(1)
	go h.pumpCommands()
	return h
}

func (h *hostedRun) pumpCommands() {
	defer h.commandPumpWG.Done()
	// approvalTries counts delivery attempts per approval command: an answer
	// whose ask never (re)appears in the broker must not head-of-line-block
	// the queue forever (QA Round4 F-J1 — one undeliverable approve froze
	// every later command, close included). The pump is one goroutine, so a
	// plain map is safe.
	approvalTries := map[string]int{}
	for {
		h.mu.Lock()
		if h.done {
			h.mu.Unlock()
			return
		}
		if len(h.pendingCommands) == 0 {
			wake, stop := h.commandWake, h.commandStop
			h.mu.Unlock()
			select {
			case <-wake:
				continue
			case <-stop:
				return
			}
		}
		cmd := h.pendingCommands[0]
		stop := h.commandStop
		inbox := h.inbox
		controls := h.controls
		interrupts := h.commandInterrupts
		cancels := h.commandCancels
		revokes := h.revokes
		answerApproval := h.answerApproval
		h.mu.Unlock()

		delivered := false
		switch cmd.Kind {
		case protocol.CommandInput:
			if cmd.Input == nil {
				delivered = true
				break
			}
			select {
			case inbox <- *cmd.Input:
				delivered = true
			case <-stop:
				return
			}
		case protocol.CommandControl, protocol.CommandClose:
			if cmd.Control == nil {
				delivered = true
				break
			}
			ctl := *cmd.Control
			select {
			case controls <- ctl:
				delivered = true
			case <-stop:
				return
			}
		case protocol.CommandRevoke:
			if cmd.Revoke == nil {
				delivered = true
				break
			}
			select {
			case revokes <- *cmd.Revoke:
				delivered = true
			case <-stop:
				return
			}
		case protocol.CommandInterrupt:
			select {
			case interrupts <- cmd.CommandRef:
				delivered = true
			case <-stop:
				return
			}
		case protocol.CommandKill:
			select {
			case cancels <- protocol.CancelCommand{CommandRef: cmd.CommandRef, Handle: cmd.Handle}:
				delivered = true
			case <-stop:
				return
			}
		case protocol.CommandApproval:
			if cmd.Approval == nil || answerApproval == nil {
				delivered = true
				break
			}
			delivered = answerApproval(cmd)
			if !delivered {
				// The retry window covers the loop still on its way to park
				// (the ask registers moments after the command can arrive).
				// Past it the ask simply is not there — a wrong or stale id —
				// and retrying forever would wedge every command behind it.
				limit := h.approvalGiveUp
				if limit == 0 {
					limit = 400 // ~10s at 25ms
				}
				approvalTries[cmd.CommandID]++
				if approvalTries[cmd.CommandID] >= limit {
					delete(approvalTries, cmd.CommandID)
					slog.Warn("daemon: dropping undeliverable approval answer",
						"session", h.id, "approval", cmd.Approval.ApprovalID)
					h.Emit(protocol.Event{Kind: protocol.KindError, Session: h.id,
						Text: fmt.Sprintf("approval answer %s could not be delivered (no matching pending ask) — dropped; inspect shows the current ask", cmd.Approval.ApprovalID)})
					delivered = true
					break
				}
				timer := time.NewTimer(25 * time.Millisecond)
				select {
				case <-timer.C:
				case <-stop:
					timer.Stop()
					return
				}
			} else {
				delete(approvalTries, cmd.CommandID)
			}
		default:
			// PersistCommand rejects unknown kinds; keep the live pump safe if
			// an embedding supplies one anyway.
			slog.Warn("daemon: dropping unknown live command", "session", h.id, "kind", cmd.Kind)
			delivered = true
		}
		if delivered {
			h.mu.Lock()
			h.pendingCommands = h.pendingCommands[1:]
			h.mu.Unlock()
		}
	}
}

func (h *hostedRun) postCommand(cmd protocol.SessionCommand) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done || h.commandWake == nil {
		return false
	}
	if cmd.CommandID != "" {
		if _, duplicate := h.postedCommands[cmd.CommandID]; duplicate {
			return true
		}
		h.postedCommands[cmd.CommandID] = struct{}{}
	}
	h.pendingCommands = append(h.pendingCommands, cmd)
	select {
	case h.commandWake <- struct{}{}:
	default:
	}
	return true
}

// stopHosting cancels the hosted loop's context (teardown, not a close).
func (h *hostedRun) stopHosting() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done || h.stop == nil {
		return false
	}
	h.stop()
	return true
}

// killHandle requests cancellation of one running child/task by handle.
func (h *hostedRun) killHandle(cmd protocol.SessionCommand) bool {
	return h.postCommand(cmd)
}

// postControl queues a compact/clear control behind earlier commands.
func (h *hostedRun) postControl(cmd protocol.SessionCommand) bool {
	return h.postCommand(cmd)
}

// signalInterrupt queues one durable interrupt behind earlier commands.
func (h *hostedRun) signalInterrupt(cmd protocol.SessionCommand) bool {
	return h.postCommand(cmd)
}

// post queues a conversational input for the hosted session. The caller has
// already durably persisted it; this queue is only the best-effort wake path.
func (h *hostedRun) post(in protocol.UserInput) bool {
	return h.postCommand(protocol.SessionCommand{
		CommandRef: protocol.CommandRef{CommandID: in.CommandID, CommandSeq: in.DeliverySeq},
		Kind:       protocol.CommandInput, Input: &in,
	})
}

// Emit implements protocol.Sink: fan out to every subscriber. A slow
// subscriber's overflow is DROPPED (可丢 delta doctrine — the journal is the
// durable truth; the live stream is ephemeral rendering). Lifecycle events
// additionally tee to the notifier hook, outside the lock.
func (h *hostedRun) Emit(e protocol.Event) {
	if e.Session == "" {
		// Tree members stamp their own id (INC-12.6); only untagged events
		// default to the hosted root.
		e.Session = h.id
	}
	h.mu.Lock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
	h.mu.Unlock()
	if h.notify != nil &&
		(e.Kind == protocol.KindRunEnd || e.Kind == protocol.KindApprovalRequest ||
			e.Kind == protocol.KindIteration) {
		h.notify(e)
	}
}

// subscribe registers a live-event channel; the returned cancel removes it.
// A finished run returns ok=false — there is nothing live to subscribe to.
func (h *hostedRun) subscribe() (ch chan protocol.Event, cancel func(), ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done {
		return nil, nil, false
	}
	ch = make(chan protocol.Event, 256)
	h.subs[ch] = struct{}{}
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}, true
}

// finish marks the run done and closes every subscriber channel.
func (h *hostedRun) finish() {
	h.mu.Lock()
	if h.done {
		h.mu.Unlock()
		return
	}
	h.done = true
	if h.commandStop != nil {
		close(h.commandStop)
	}
	inbox := h.inbox
	h.inbox = nil
	for ch := range h.subs {
		close(ch)
		delete(h.subs, ch)
	}
	h.mu.Unlock()
	h.commandPumpWG.Wait()
	if inbox != nil {
		close(inbox)
	}
}

// ListenAndServe binds the socket and serves until ctx is done. A stale
// socket file (no listener behind it) is removed; a LIVE daemon on the same
// path is an error — two daemons must not split the session space.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.runs == nil {
		s.runs = map[string]*hostedRun{}
	}
	if s.conns == nil {
		s.conns = map[net.Conn]struct{}{}
	}
	if s.Clock == nil {
		s.Clock = clock.Real{}
	}
	s.loadIdem()
	ln, err := net.Listen("unix", s.SocketPath)
	if err != nil {
		if conn, derr := net.Dial("unix", s.SocketPath); derr == nil {
			_ = conn.Close()
			return fmt.Errorf("daemon: already running on %s", s.SocketPath)
		}
		_ = os.Remove(s.SocketPath) // stale socket from a dead daemon
		ln, err = net.Listen("unix", s.SocketPath)
		if err != nil {
			return fmt.Errorf("daemon: %w", err)
		}
	}
	// Owner-only, explicitly: anyone who can connect can submit runs and
	// answer approvals, so the socket must not lean on the umask or the
	// parent dir alone (S6 review).
	_ = os.Chmod(s.SocketPath, 0o600)
	s.ln = ln
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	if s.ScanTimers != nil && s.Resume != nil {
		go s.sweepTimers(ctx)
	}
	if s.ScanPendingCommandSessions != nil && s.Resume != nil {
		go s.resumePendingCommandSessions(ctx)
	}
	slog.Info("daemon listening", "socket", s.SocketPath)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				// Graceful shutdown: the ctx cancel is already propagating
				// into every hosted run (cooperative cancel → terminal
				// events). Refuse new runs, wait for the live ones to finish
				// journaling, then CLOSE the remaining connections — a
				// client idle in a read/write must not wedge the deploy
				// (S6 review P1) — and drain the handlers.
				s.mu.Lock()
				s.stopping = true
				s.mu.Unlock()
				s.runsWG.Wait()
				s.mu.Lock()
				for c := range s.conns {
					_ = c.Close()
				}
				s.mu.Unlock()
				s.wg.Wait()
				return nil
			}
			return fmt.Errorf("daemon accept: %w", err)
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serveConn(ctx, conn)
		}()
	}
}

func (s *Server) resumePendingCommandSessions(ctx context.Context) {
	ids, err := s.ScanPendingCommandSessions()
	if err != nil {
		slog.Warn("daemon: pending command scan failed", "err", err)
		return
	}
	for _, id := range ids {
		s.hostResume(ctx, id, true)
	}
}

func (s *Server) replayPendingCommands(ctx context.Context, session string, hub *hostedRun) {
	if s.PendingCommands == nil {
		return
	}
	commands, err := s.PendingCommands(session)
	if err != nil {
		slog.Warn("daemon: pending command replay failed", "session", session, "err", err)
		return
	}
	for _, cmd := range commands {
		_ = hub.postCommand(cmd)
	}
}

func (s *Server) newHostedRun(id string, interactive bool) *hostedRun {
	hub := newHostedRun(id, s.Notify, interactive)
	if interactive && s.Approvals != nil {
		hub.answerApproval = func(cmd protocol.SessionCommand) bool {
			if cmd.Approval == nil {
				return true
			}
			target := cmd.Target
			if target == "" {
				target = id
			}
			return s.Approvals.Answer(target, cmd.Approval.ApprovalID, ApprovalAnswer{
				CommandRef: cmd.CommandRef,
				Approve:    cmd.Approval.Decision == "approve",
				Reason:     cmd.Approval.Reason,
				Remember:   cmd.Approval.Remember,
			})
		}
	}
	return hub
}

// serveConn handles one connection: ONE command line in, an event stream
// out (v0 — interactive multiplexing arrives with approval routing).
func (s *Server) serveConn(ctx context.Context, conn net.Conn) {
	s.mu.Lock()
	s.conns[conn] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		_ = conn.Close()
	}()
	enc := json.NewEncoder(conn)
	var cmd Command
	sc := bufio.NewScanner(conn)
	// Command lines can carry base64 image attachments (v2 M4.1) — allow
	// well past the practical screenshot size instead of the event-line 1MB.
	sc.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)
	if !sc.Scan() {
		return
	}
	if err := json.Unmarshal(sc.Bytes(), &cmd); err != nil {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "bad command: " + err.Error()})
		return
	}
	switch cmd.Cmd {
	case "ping":
		_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage, Text: "pong"})
	case "run":
		s.handleRun(ctx, cmd, enc)
	case "drive":
		s.handleDrive(ctx, cmd, enc)
	case "attach":
		s.handleAttach(cmd, enc)
	case "approve":
		s.handleApprove(ctx, cmd, enc)
	case "send":
		s.handleSend(ctx, cmd, enc)
	case "close":
		s.handleClose(ctx, cmd, enc)
	case "interrupt":
		s.handleInterrupt(ctx, cmd, enc)
	case "stop":
		s.handleStop(cmd, enc)
	case "compact":
		s.handleControl(ctx, cmd, protocol.Control{Kind: protocol.ControlCompact, Directive: cmd.Directive}, "compact requested — the journal records the outcome", enc)
	case "clear":
		s.handleControl(ctx, cmd, protocol.Control{Kind: protocol.ControlClear}, "clear requested — the journal records the outcome", enc)
	case "remember":
		s.handleControl(ctx, cmd, protocol.Control{Kind: protocol.ControlRemember, Directive: cmd.Directive}, "remember requested — the journal records the outcome", enc)
	case "mode":
		s.handleControl(ctx, cmd, protocol.Control{Kind: protocol.ControlMode, Directive: cmd.Directive}, "mode change requested — the journal records the outcome", enc)
	// goal-* controls revive a non-hosted session like send does (INC-10):
	// structural since the durable-command unification — handleControl's
	// delivery path (commandHubCommandLocked) resumes the session first.
	case "goal-attach":
		s.handleControl(ctx, cmd, protocol.Control{Kind: protocol.ControlGoalAttach, Goal: cmd.Goal}, "goal attached", enc)
	case "goal-pause":
		s.handleControl(ctx, cmd, protocol.Control{Kind: protocol.ControlGoalPause}, "goal pause requested (a no-op unless a goal is attached)", enc)
	case "goal-resume":
		s.handleControl(ctx, cmd, protocol.Control{Kind: protocol.ControlGoalResume}, "goal resume requested (a no-op unless a goal is paused)", enc)
	case "goal-update":
		s.handleControl(ctx, cmd, protocol.Control{Kind: protocol.ControlGoalUpdate, Goal: cmd.Goal}, "goal update requested (a no-op unless a goal is attached)", enc)
	case "goal-cancel":
		s.handleControl(ctx, cmd, protocol.Control{Kind: protocol.ControlGoalCancel}, "goal cancel requested — applies at the session's next boundary (interrupt cuts the current turn); a no-op unless a goal is attached", enc)
	case "kill":
		s.handleKill(ctx, cmd, enc)
	case "unqueue":
		s.handleUnqueue(ctx, cmd, enc)
	case "agent":
		s.handleAgent(cmd, enc)
	default:
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("unknown command %q (known: ping, run, drive, attach, approve, send, close, interrupt, stop, compact, clear, remember, goal-*, kill, agent)", cmd.Cmd)})
	}
}

func (s *Server) acceptCommand(session, commandID string, cmd protocol.SessionCommand) (protocol.SessionCommand, bool, error) {
	if commandID == "" {
		commandID = event.NewCommandID()
	}
	cmd.CommandID = commandID
	if s.PersistCommand == nil {
		return cmd, false, nil
	}
	accepted, err := s.PersistCommand(session, cmd)
	return accepted, err == nil, err
}

// commandHubCommandLocked returns or starts the live hub. The caller holds
// commandMu, so recovery replay cannot interleave with a new append.
func (s *Server) commandHubCommandLocked(ctx context.Context, session string) (*hostedRun, bool) {
	s.mu.Lock()
	hub := s.runs[session]
	s.mu.Unlock()
	if hub != nil {
		return hub, true
	}
	if s.Resume == nil {
		return nil, false
	}
	s.mu.Lock()
	delete(s.failed, session)
	s.mu.Unlock()
	s.hostResumeCommandLocked(ctx, session, true)
	s.mu.Lock()
	hub = s.runs[session]
	s.mu.Unlock()
	return hub, hub != nil
}

func (s *Server) acceptAndDeliver(ctx context.Context, session, commandID string,
	cmd protocol.SessionCommand, post func(*hostedRun, protocol.SessionCommand) bool) (protocol.SessionCommand, bool, error) {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	accepted, durable, err := s.acceptCommand(session, commandID, cmd)
	if err != nil {
		return accepted, false, err
	}
	if !s.commandNeedsDelivery(session, accepted) {
		return accepted, true, nil
	}
	hub, _ := s.commandHubCommandLocked(ctx, session)
	return accepted, s.acceptedDelivery(ctx, session, hub, durable,
		func(h *hostedRun) bool { return post(h, accepted) }), nil
}

// acceptAndDeliverVia separates the command-log owner from the live host.
// Child commands remain durable in the child's log (so its journal receipt
// proves completion) but wake through the tree root's single process.
func (s *Server) acceptAndDeliverVia(ctx context.Context, commandSession, hostSession,
	commandID string, cmd protocol.SessionCommand,
	post func(*hostedRun, protocol.SessionCommand) bool) (protocol.SessionCommand, bool, error) {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	accepted, durable, err := s.acceptCommand(commandSession, commandID, cmd)
	if err != nil {
		return accepted, false, err
	}
	if !s.commandNeedsDelivery(commandSession, accepted) {
		return accepted, true, nil
	}
	hub, _ := s.commandHubCommandLocked(ctx, hostSession)
	return accepted, s.acceptedDelivery(ctx, hostSession, hub, durable,
		func(h *hostedRun) bool { return post(h, accepted) }), nil
}

// commandNeedsDelivery distinguishes an old, already-completed receipt from
// an old receipt that is still pending. Failure to derive the projection
// falls back to at-least-once delivery: accepted commands must never vanish.
func (s *Server) commandNeedsDelivery(session string, accepted protocol.SessionCommand) bool {
	if !accepted.PreviouslyAccepted || s.PendingCommands == nil {
		return true
	}
	pending, err := s.PendingCommands(session)
	if err != nil {
		slog.Warn("daemon: cannot verify retried command completion; redelivering",
			"session", session, "command_id", accepted.CommandID, "err", err)
		return true
	}
	for _, cmd := range pending {
		if cmd.CommandID == accepted.CommandID {
			return true
		}
	}
	return false
}

func (s *Server) acceptedDelivery(ctx context.Context, session string, hub *hostedRun,
	durable bool, post func(*hostedRun) bool) bool {
	if hub != nil && post(hub) {
		return true
	}
	if durable {
		s.reviveAcceptedAfter(ctx, session, hub)
		return true
	}
	return false
}

// handleKill cancels one running child/task by handle (v2 M3.2): the user's
// direct kill path, distinct from the model calling kill.
func (s *Server) handleKill(ctx context.Context, cmd Command, enc *json.Encoder) {
	if cmd.Session == "" || cmd.Handle == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "kill needs session and handle"})
		return
	}
	_, delivered, err := s.acceptAndDeliver(ctx, cmd.Session, cmd.CommandID, attributedCommand(cmd, protocol.SessionCommand{
		Kind: protocol.CommandKill, Handle: cmd.Handle,
	}), func(h *hostedRun, accepted protocol.SessionCommand) bool { return h.killHandle(accepted) })
	if err != nil {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "kill not accepted: " + err.Error()})
		return
	}
	if !delivered {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("no live session %s accepting kills", cmd.Session)})
		return
	}
	_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage,
		Text: "kill requested for " + cmd.Handle + " (a no-op if the handle is done or unknown)", Session: cmd.Session})
}

// handleUnqueue withdraws a QUEUED conversational input (INC-46, §2 rev1).
// The precheck here is a UX courtesy, not the safety boundary (it re-folds
// the journal like pendingApproval does and can race an in-flight consume):
// the loop's consume-side guard is what actually decides, and a late revoke
// is a no-op there.
func (s *Server) handleUnqueue(ctx context.Context, cmd Command, enc *json.Encoder) {
	if cmd.Session == "" || cmd.TargetCommandID == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "unqueue needs session and target_command_id"})
		return
	}
	// Target validation lives in the CLI (it owns store access; the daemon
	// stays semantics-free) — and either way it is only a UX courtesy: the
	// loop's consume-side guard is the safety boundary, a late revoke no-ops.
	_, delivered, err := s.acceptAndDeliver(ctx, cmd.Session, cmd.CommandID, attributedCommand(cmd, protocol.SessionCommand{
		Kind: protocol.CommandRevoke, Revoke: &protocol.Revoke{TargetCommandID: cmd.TargetCommandID},
	}), func(h *hostedRun, accepted protocol.SessionCommand) bool { return h.postCommand(accepted) })
	if err != nil {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "unqueue not accepted: " + err.Error()})
		return
	}
	if !delivered {
		// Durable is enough: an unhosted session's resume replay applies it.
		_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage, Session: cmd.Session,
			Text: "unqueue recorded; it applies when the session next runs"})
		return
	}
	_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage, Session: cmd.Session,
		Text: "unqueue delivered for " + cmd.TargetCommandID + " (a no-op if already processed)"})
}

// handleAgent prepares an agent switch (决策 #32): it tears the hosted
// loop down (plain teardown — no mark, journal untouched) and waits for
// the journal lock to free, so the CLI can append SpecChanged and the next
// send revives the session under the new spec. A session not hosted here
// needs no preparation.
func (s *Server) handleAgent(cmd Command, enc *json.Encoder) {
	if cmd.Session == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "agent needs session"})
		return
	}
	s.mu.Lock()
	hub, ok := s.runs[cmd.Session]
	s.mu.Unlock()
	if !ok || !hub.stopHosting() {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage,
			Text: "not hosted", Session: cmd.Session})
		return
	}
	// Wait for the loop to actually exit (flock release) before acking.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		_, live := s.runs[cmd.Session]
		s.mu.Unlock()
		if !live {
			_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage,
				Text: "released", Session: cmd.Session})
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
		Text: "session did not release in time"})
}

// handleInterrupt delivers an out-of-band interrupt to a live session
// (v2 M2.3): distinct from `send` — it steers a running turn or closes an
// idle one, it does not enter the conversation.
func (s *Server) handleInterrupt(ctx context.Context, cmd Command, enc *json.Encoder) {
	if cmd.Session == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "interrupt needs session"})
		return
	}
	_, delivered, err := s.acceptAndDeliver(ctx, cmd.Session, cmd.CommandID, attributedCommand(cmd, protocol.SessionCommand{
		Kind: protocol.CommandInterrupt,
	}), func(h *hostedRun, accepted protocol.SessionCommand) bool { return h.signalInterrupt(accepted) })
	if err != nil {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "interrupt not accepted: " + err.Error()})
		return
	}
	if !delivered {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("no live interruptible session %s", cmd.Session)})
		return
	}
	_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage, Text: "interrupt delivered — cancels in-flight work or a pending ask; a no-op when the session is idle", Session: cmd.Session})
}

// handleStop is the remote hard-cancel (G12): it tears the hosted run down
// via the plain-teardown primitive (the same ctx cancel the agent switch
// uses) — NO mark, NO ending. The session lands in durable standby and a
// later `send` lawfully revives it, mirroring how a terminal run reacts to
// SIGTERM. Distinct from `interrupt` (which only cancels the current turn's
// activity and is a no-op at idle) and from `close`/`kill` (which leave a
// mark that the automatic-resume path must not cross).
func (s *Server) handleStop(cmd Command, enc *json.Encoder) {
	if cmd.Session == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "stop needs session"})
		return
	}
	s.mu.Lock()
	hub, ok := s.runs[cmd.Session]
	s.mu.Unlock()
	if !ok || !hub.stopHosting() {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("no live hosted run %s to stop", cmd.Session)})
		return
	}
	_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage, Text: "stopping", Session: cmd.Session})
}

// handleControl posts a compact/clear maintenance signal to a hosted session
// (G7). The command is durable before the ack; the semantic event records
// when the loop applies it. A parked session is woken by the control.
func (s *Server) handleControl(ctx context.Context, cmd Command, ctl protocol.Control, ack string, enc *json.Encoder) {
	if cmd.Session == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: ctl.Kind + " needs session"})
		return
	}
	_, delivered, err := s.acceptAndDeliver(ctx, cmd.Session, cmd.CommandID, attributedCommand(cmd, protocol.SessionCommand{
		Kind: protocol.CommandControl, Control: &ctl,
	}), func(h *hostedRun, accepted protocol.SessionCommand) bool {
		accepted.Control.CommandRef = accepted.CommandRef
		return h.postControl(accepted)
	})
	if err != nil {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: ctl.Kind + " not accepted: " + err.Error()})
		return
	}
	if !delivered {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("no live session %s to %s", cmd.Session, ctl.Kind)})
		return
	}
	_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage, Text: ack, Session: cmd.Session})
}

// splitAddress resolves a session address into (host, target); target != ""
// means a child session hosted by its tree root. The wired resolver knows
// the store; the structural fallback keeps the historic first-"-sub-"
// split for addresses the store does not know.
func (s *Server) splitAddress(session string) (host, target string) {
	if s.SplitAddress != nil {
		return s.SplitAddress(session)
	}
	if idx := strings.Index(session, "-sub-"); idx > 0 {
		return session[:idx], session
	}
	return session, ""
}

// handleSend delivers a user message to a live conversational session
// (v2 M1.2). It is the machine/web/CLI-agnostic投递入口 — every sender
// (human at a terminal, web UI, webhook) posts through the same path.
func (s *Server) handleSend(ctx context.Context, cmd Command, enc *json.Encoder) {
	if cmd.Session == "" || (cmd.Text == "" && len(cmd.Content) == 0 && len(cmd.Images) == 0 && len(cmd.Files) == 0) {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "send needs session and content"})
		return
	}
	// A child-session address (INC-12.3, `ar send <child-sid>`) routes
	// through the TREE ROOT: the root is the single host and single mailbox
	// writer for its tree, so the command logs durably on the root and the
	// root loop forwards it to the member's own inbox. The Target rides the
	// UserInput; everything else (revive-as-resume, idempotency) is the
	// ordinary send path.
	hostSession, target := s.splitAddress(cmd.Session)
	s.mu.Lock()
	hub, ok := s.runs[hostSession]
	s.mu.Unlock()
	if !ok {
		// v2 M5.1 (QA.md §0.3): a send to a non-hosted session REVIVES it —
		// after a daemon restart, `ar send` IS the resume gesture; no
		// special action. The resume runner refuses ended/unknown sessions
		// and the failure reaches watchers through the hub.
		if s.Resume == nil {
			_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
				Text: fmt.Sprintf("no live session %s and no resume runner", cmd.Session)})
			return
		}
		s.mu.Lock()
		delete(s.failed, hostSession) // an explicit send retries a failed resume
		s.mu.Unlock()
		// The revived run must live on the DAEMON's lifecycle (收口 review:
		// a Background ctx would wedge graceful shutdown in runsWG.Wait).
		s.hostResume(ctx, hostSession, true)
		s.mu.Lock()
		hub, ok = s.runs[hostSession]
		s.mu.Unlock()
		if !ok {
			_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
				Text: fmt.Sprintf("no live session %s and it could not be resumed", cmd.Session)})
			return
		}
	}
	// Follow subscribes BEFORE the post: the turn the input triggers must
	// not be able to emit anything the follower misses (INC-2).
	var follow chan protocol.Event
	if cmd.Follow {
		if ch, cancel, ok := hub.subscribe(); ok {
			follow = ch
			defer cancel()
		}
	}
	if cmd.CommandID == "" {
		cmd.CommandID = event.NewCommandID() // older clients remain accepted
	}
	in := protocol.UserInput{Text: cmd.Text, Images: cmd.Images, Files: cmd.Files,
		Content: cmd.Content, Principal: cmd.Principal, Source: cmd.Source,
		Trust: cmd.Trust, CommandID: cmd.CommandID, Target: target,
		Delivery: cmd.Delivery}
	in.TurnID, in.ItemID = "turn-"+cmd.CommandID, "item-"+cmd.CommandID
	// Normalize the delivery mode at the boundary: only "steer" opts into the
	// mid-turn safe-boundary drain; anything else is the default next-turn
	// queue. Persisting the canonical value keeps the payload hash stable.
	if in.Delivery != protocol.DeliverySteer {
		in.Delivery = ""
	}
	if in.Principal == "" {
		in.Principal = "local-user"
	}
	if in.Source == "" {
		in.Source = "unix-socket"
	}
	if in.Trust == "" {
		in.Trust = "local"
	}
	durable := false
	s.commandMu.Lock()
	if s.PersistCommand != nil {
		accepted, perr := s.PersistCommand(hostSession, attributedCommand(cmd, protocol.SessionCommand{
			CommandRef: protocol.CommandRef{CommandID: cmd.CommandID},
			Kind:       protocol.CommandInput, Input: &in,
		}))
		if perr != nil {
			s.commandMu.Unlock()
			// The persist error is usually the story itself ("no session
			// matches …") — a "command log write failed" wrap would misread
			// as disk trouble (QA Round1 F-A06).
			_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
				Text: "send not accepted: " + perr.Error()})
			return
		}
		in = *accepted.Input
		durable = true
		if !s.commandNeedsDelivery(hostSession, accepted) {
			s.commandMu.Unlock()
			_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage, Text: "delivered", Session: cmd.Session})
			return
		}
	} else if s.PersistInput != nil {
		// Durability before the ack (铁律 2): once "delivered" is on the
		// wire, a crash cannot lose this input — resume replays the
		// mailbox tail the journal has not consumed.
		var perr error
		if in, perr = s.PersistInput(hostSession, in); perr != nil {
			s.commandMu.Unlock()
			_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
				Text: "send not accepted: " + perr.Error()})
			return
		}
		durable = true
	}
	posted := hub.post(in)
	s.commandMu.Unlock()
	if !posted {
		if durable {
			// Accepted is irrevocable once the mailbox fsync succeeds. A hub
			// that began shutting down in the append→wake window cannot turn
			// that success into a client-visible failure (which would invite a
			// duplicate retry); revive it after the old host releases its lock.
			s.reviveAcceptedAfter(ctx, hostSession, hub)
			_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage, Text: "delivered", Session: cmd.Session})
			return
		}
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("session %s is not accepting input (shutting down)", cmd.Session)})
		return
	}
	_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage, Text: "delivered", Session: cmd.Session})
	if follow == nil {
		return
	}
	for e := range follow {
		if err := enc.Encode(e); err != nil {
			return // client detached; the session keeps going
		}
	}
}

func (s *Server) reviveAcceptedAfter(ctx context.Context, session string, old *hostedRun) {
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			s.mu.Lock()
			current := s.runs[session]
			s.mu.Unlock()
			if current == nil || current != old {
				s.hostResume(ctx, session, true)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// handleClose ends a conversational session gracefully (v2 M1.2): shutting
// the inbox resolves the idle loop into its epilogue.
func (s *Server) handleClose(ctx context.Context, cmd Command, enc *json.Encoder) {
	if cmd.Session == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "close needs session"})
		return
	}
	ctl := protocol.Control{Kind: protocol.ControlClose}
	_, delivered, err := s.acceptAndDeliver(ctx, cmd.Session, cmd.CommandID, attributedCommand(cmd, protocol.SessionCommand{
		Kind: protocol.CommandClose, Control: &ctl,
	}), func(h *hostedRun, accepted protocol.SessionCommand) bool {
		accepted.Control.CommandRef = accepted.CommandRef
		return h.postControl(accepted)
	})
	if err != nil {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "close not accepted: " + err.Error()})
		return
	}
	if !delivered {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("no live conversational session %s", cmd.Session)})
		return
	}
	_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage, Text: "closing (a later send revives it)", Session: cmd.Session})
}

// handleApprove routes a human's verdict to the idle ask. If the ask is no
// longer live in the (in-memory) broker but the session's journal still shows
// it idle on exactly this approval, the daemon was restarted and lost the
// pending ask (M2): reviveAndAnswer re-hosts the session so its resumed loop
// re-registers the ask, then answers it — the stuck approval self-heals
// instead of dead-ending at "no pending approval".
func (s *Server) handleApprove(ctx context.Context, cmd Command, enc *json.Encoder) {
	if s.Approvals == nil {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "daemon has no approval broker"})
		return
	}
	if cmd.Session == "" || cmd.ApprovalID == "" || (cmd.Decision != "approve" && cmd.Decision != "deny") {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: "approve needs session, approval_id and decision approve|deny"})
		return
	}
	if s.PersistCommand != nil && s.PendingApproval != nil {
		pending, ok, err := s.PendingApproval(cmd.Session)
		if err != nil || !ok || pending != cmd.ApprovalID {
			_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
				Text: fmt.Sprintf("no pending approval %s on session %s", cmd.ApprovalID, cmd.Session)})
			return
		}
	}
	if s.PersistCommand != nil {
		hostSession, target := s.splitAddress(cmd.Session)
		_, delivered, err := s.acceptAndDeliverVia(ctx, cmd.Session, hostSession, cmd.CommandID, attributedCommand(cmd, protocol.SessionCommand{
			Target: target,
			Kind:   protocol.CommandApproval,
			Approval: &protocol.ApprovalCommand{
				ApprovalID: cmd.ApprovalID, Decision: cmd.Decision, Reason: cmd.Reason, Remember: cmd.Remember,
			},
		}), func(h *hostedRun, accepted protocol.SessionCommand) bool { return h.postCommand(accepted) })
		if err != nil {
			_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "approval not accepted: " + err.Error()})
			return
		}
		if !delivered {
			_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
				Text: fmt.Sprintf("no live session %s accepting approvals", cmd.Session)})
			return
		}
		_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage,
			Text: "answered " + cmd.ApprovalID + ": " + cmd.Decision, Session: cmd.Session})
		return
	}

	accepted, durable, err := s.acceptCommand(cmd.Session, cmd.CommandID, attributedCommand(cmd, protocol.SessionCommand{
		Kind: protocol.CommandApproval,
		Approval: &protocol.ApprovalCommand{
			ApprovalID: cmd.ApprovalID, Decision: cmd.Decision, Reason: cmd.Reason, Remember: cmd.Remember,
		},
	}))
	if err != nil {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "approval not accepted: " + err.Error()})
		return
	}
	answer := ApprovalAnswer{CommandRef: accepted.CommandRef, Approve: cmd.Decision == "approve", Reason: cmd.Reason, Remember: cmd.Remember}
	if s.Approvals.Answer(cmd.Session, cmd.ApprovalID, answer) ||
		s.reviveAndAnswer(ctx, cmd.Session, cmd.ApprovalID, answer) {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage,
			Text: "answered " + cmd.ApprovalID + ": " + cmd.Decision, Session: cmd.Session})
		return
	}
	if durable {
		// The response is accepted even if the in-memory rendezvous vanished
		// in this instant. Startup pending-command sweep/revive will re-arm it.
		_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage,
			Text: "accepted " + cmd.ApprovalID + ": " + cmd.Decision, Session: cmd.Session})
		return
	}
	_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
		Text: fmt.Sprintf("no pending approval %s on session %s", cmd.ApprovalID, cmd.Session)})
}

// reviveAndAnswer recovers a pending approval the in-memory broker lost to a
// daemon restart (M2): when the session is NOT hosted but its journal shows it
// idle on exactly this approval, re-host it — the resumed loop re-enters the
// wait and re-registers the ask on the fresh broker — and answer once the ask
// reappears. The journal guard means a non-approval target (wrong id, an
// already-ended session) is never spuriously revived.
func (s *Server) reviveAndAnswer(ctx context.Context, session, approvalID string, a ApprovalAnswer) bool {
	if s.Resume == nil || s.PendingApproval == nil {
		return false
	}
	s.mu.Lock()
	hosted := s.runs[session] != nil
	s.mu.Unlock()
	if hosted {
		return false // hosted but not pending = the ask already resolved (stale)
	}
	pid, ok, err := s.PendingApproval(session)
	if err != nil || !ok || pid != approvalID {
		return false // the journal does not show this session idle on this ask
	}
	s.mu.Lock()
	delete(s.failed, session) // an explicit answer retries a failed resume
	s.mu.Unlock()
	s.hostResume(ctx, session, true)
	// The resumed loop re-registers the ask asynchronously; poll the broker
	// until it reappears, bounded so a session that settles some other way
	// (e.g. resume error) does not hang the client.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.Approvals.Answer(session, approvalID, a) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(25 * time.Millisecond):
		}
	}
	return false
}

// handleRun hosts a new run and streams its events to the submitting client
// until the run ends. The run belongs to the DAEMON's lifetime, not the
// connection's: a client that disconnects mid-run only stops watching.
func (s *Server) handleRun(ctx context.Context, cmd Command, enc *json.Encoder) {
	if s.Run == nil {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "daemon has no runner configured"})
		return
	}
	if cmd.SpecPath == "" || cmd.Task == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "run needs spec_path and task"})
		return
	}
	// bypass is a workstation-only escape hatch (spec validation refuses it
	// too); it must not be reachable over the wire (S6 review).
	switch cmd.Mode {
	case "", "default", "plan", "acceptEdits":
	default:
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("mode %q not allowed over the daemon (known: default, plan, acceptEdits)", cmd.Mode)})
		return
	}
	// Idempotent resubmission: the same key returns the FIRST submission's
	// session — live, that means following its stream; finished, its replay.
	if cmd.IdemKey != "" {
		if existing, ok := s.idemSession(cmd.IdemKey); ok {
			s.handleAttach(Command{Cmd: "attach", Session: existing}, enc)
			return
		}
	}
	id := s.NewID(cmd.Task)
	hub := s.newHostedRun(id, true)
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "daemon is shutting down"})
		return
	}
	s.runs[id] = hub
	s.runsWG.Add(1)
	if cmd.IdemKey != "" {
		s.registerIdemLocked(cmd.IdemKey, id)
	}
	s.mu.Unlock()

	ch, cancel, _ := hub.subscribe()
	defer cancel()

	// The run runs on the daemon's ctx (not the connection's): it survives
	// the client going away. A per-run cancel (hub.stop) lets an agent
	// switch tear just this loop down. The registry entry is removed when
	// the run finishes — attach then serves replay only, and a long-lived
	// daemon's map does not grow unboundedly (S6 review).
	runCtx, runCancel := context.WithCancel(ctx)
	hub.stop = runCancel
	go func() {
		defer runCancel()
		defer s.runsWG.Done()
		defer func() {
			s.mu.Lock()
			delete(s.runs, id)
			s.mu.Unlock()
		}()
		defer hub.finish()
		if err := s.Run(runCtx, RunRequest{
			SessionID: id, SpecPath: cmd.SpecPath, Task: cmd.Task,
			Workspace: cmd.Workspace, Mode: cmd.Mode,
			Inbox: hub.inbox, Interrupts: hub.interrupts, Cancels: hub.cancels,
			Controls: hub.controls, CommandInterrupts: hub.commandInterrupts,
			CommandCancels: hub.commandCancels, Revokes: hub.revokes,
		}, hub); err != nil {
			hub.Emit(protocol.Event{Kind: protocol.KindError, Text: "run failed: " + err.Error()})
		}
	}()

	// Tell the client which session it got, then stream until the run ends.
	_ = enc.Encode(protocol.Event{Kind: protocol.KindSessionStart, Session: id})
	for e := range ch {
		if err := enc.Encode(e); err != nil {
			return // client went away; the run keeps going
		}
	}
}

// handleDrive hosts an IterationDriver series exactly like a run: it belongs
// to the daemon's lifetime, its lifecycle events tee to the notifier, and a
// disconnected client only stops watching — the series keeps its cadence
// (S6 完成标志①: 无人 attach 跑过夜).
func (s *Server) handleDrive(ctx context.Context, cmd Command, enc *json.Encoder) {
	if s.Drive == nil {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "daemon has no driver runner configured"})
		return
	}
	if cmd.SpecPath == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "drive needs spec_path"})
		return
	}
	if cmd.IdemKey != "" {
		if existing, ok := s.idemSession(cmd.IdemKey); ok {
			s.handleAttach(Command{Cmd: "attach", Session: existing}, enc)
			return
		}
	}
	id := s.NewID("drive")
	hub := s.newHostedRun(id, false)
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "daemon is shutting down"})
		return
	}
	s.runs[id] = hub
	s.runsWG.Add(1)
	if cmd.IdemKey != "" {
		s.registerIdemLocked(cmd.IdemKey, id)
	}
	s.mu.Unlock()

	ch, cancel, _ := hub.subscribe()
	defer cancel()

	// A per-run cancel makes the drive series stoppable (G12): without it
	// `s.Drive` ran on the raw daemon ctx and a `stop` had nothing to cancel.
	runCtx, runCancel := context.WithCancel(ctx)
	hub.stop = runCancel

	go func() {
		defer runCancel()
		defer s.runsWG.Done()
		defer func() {
			s.mu.Lock()
			delete(s.runs, id)
			s.mu.Unlock()
		}()
		defer hub.finish()
		if err := s.Drive(runCtx, DriveRequest{
			SessionID: id, SpecPath: cmd.SpecPath, Workspace: cmd.Workspace,
		}, hub); err != nil {
			hub.Emit(protocol.Event{Kind: protocol.KindError, Text: "drive failed: " + err.Error()})
		}
	}()

	_ = enc.Encode(protocol.Event{Kind: protocol.KindSessionStart, Session: id})
	for e := range ch {
		if err := enc.Encode(e); err != nil {
			return // client went away; the series keeps going
		}
	}
}

// handleAttach replays a session's journal (补读) and then follows the live
// stream if the run is still hosted. Detach is just closing the connection —
// zero events are produced by detaching (订阅不改结果).
func (s *Server) handleAttach(cmd Command, enc *json.Encoder) {
	if cmd.Session == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "attach needs session"})
		return
	}
	// A child session (INC-12.6) is hosted by its TREE ROOT: live events flow
	// through the root's hub tagged with each member's id, so attaching to a
	// member = subscribe to the root, filter by origin. Replay still reads
	// the member's own journal.
	hubID, filter := s.splitAddress(cmd.Session)
	s.mu.Lock()
	hub := s.runs[hubID]
	s.mu.Unlock()

	// Subscribe BEFORE replay so no live event slips between the two; the
	// client may see an event twice around the seam, never a gap. --replay-only
	// skips the live subscription entirely: replay is the whole response.
	var ch chan protocol.Event
	var cancel func()
	if hub != nil && !cmd.ReplayOnly {
		if c, cn, ok := hub.subscribe(); ok {
			ch, cancel = c, cn
			defer cancel()
		}
	}
	if s.Replay != nil {
		if err := s.Replay(cmd.Session, sinkFunc(func(e protocol.Event) {
			e.Session = cmd.Session
			_ = enc.Encode(e)
		})); err != nil {
			_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
				Text: "replay: " + err.Error(), Session: cmd.Session})
			return
		}
	}
	if ch == nil {
		return // not hosted (finished or unknown): replay was everything
	}
	for e := range ch {
		if filter != "" && e.Session != filter {
			continue
		}
		if err := enc.Encode(e); err != nil {
			return
		}
	}
}

// idemSession looks up an idempotency key's session.
func (s *Server) idemSession(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.idem[key]
	return id, ok
}

// registerIdem records a key → session binding and, when IdemPath is set,
// persists the whole index atomically (tiny map, whole-file rewrite is
// simplest-correct). Caller holds s.mu.
func (s *Server) registerIdemLocked(key, id string) {
	if s.idem == nil {
		s.idem = map[string]string{}
	}
	s.idem[key] = id
	if s.IdemPath == "" {
		return
	}
	raw, err := json.Marshal(s.idem)
	if err != nil {
		return
	}
	tmp := s.IdemPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		slog.Warn("daemon: idem index write failed", "err", err)
		return
	}
	if err := os.Rename(tmp, s.IdemPath); err != nil {
		slog.Warn("daemon: idem index rename failed", "err", err)
	}
}

// loadIdem restores the persisted index; a missing or corrupt file is an
// empty index (idempotency degrades to daemon-lifetime, never an error).
func (s *Server) loadIdem() {
	if s.IdemPath == "" {
		return
	}
	raw, err := os.ReadFile(s.IdemPath)
	if err != nil {
		return
	}
	idx := map[string]string{}
	if err := json.Unmarshal(raw, &idx); err != nil {
		slog.Warn("daemon: idem index unreadable, starting empty", "err", err)
		return
	}
	s.mu.Lock()
	s.idem = idx
	s.mu.Unlock()
}

// sinkFunc adapts a func to protocol.Sink.
type sinkFunc func(protocol.Event)

func (f sinkFunc) Emit(e protocol.Event) { f(e) }

// Dial connects to a daemon socket and issues one command, streaming the
// response events to onEvent until the server closes the stream.
func Dial(socketPath string, cmd Command, onEvent func(protocol.Event)) error {
	return DialUntil(socketPath, cmd, func(e protocol.Event) bool {
		onEvent(e)
		return true
	})
}

// DialUntil is Dial with a client-side stop: onEvent returning false closes
// the connection (detach — the hosted session keeps running) and DialUntil
// returns nil. INC-2: `new`/`send` follow the turn they triggered and detach
// at its idle.
func DialUntil(socketPath string, cmd Command, onEvent func(protocol.Event) bool) error {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("daemon dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if err := json.NewEncoder(conn).Encode(cmd); err != nil {
		return err
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e protocol.Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return fmt.Errorf("daemon: bad event line: %w", err)
		}
		if !onEvent(e) {
			return nil
		}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}
