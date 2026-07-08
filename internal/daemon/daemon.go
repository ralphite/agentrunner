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
	"sync"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/protocol"
)

// Command is one client→server line.
type Command struct {
	Cmd string `json:"cmd"` // ping | run | drive | attach | approve

	// run
	SpecPath  string `json:"spec_path,omitempty"`
	Task      string `json:"task,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Mode      string `json:"mode,omitempty"`

	// attach / approve / send
	Session string `json:"session,omitempty"`

	// send
	Text string `json:"text,omitempty"` // a user message for the session
	// Images are attachments riding a send (v2 M4.1); bytes are base64 on
	// the wire, the agent CAS-stores them before journaling the input.
	Images []protocol.ImageAttachment `json:"images,omitempty"`
	// Follow keeps the send connection open after the "delivered" ack,
	// streaming the session's live events until the client disconnects
	// (INC-2: the reply becomes visible on the send itself). Subscribe
	// happens BEFORE the input is posted so no reply event can slip
	// between delivery and the stream. Detach = close the connection,
	// same semantics as attach (订阅不改结果).
	Follow bool `json:"follow,omitempty"`

	// kill
	Handle string `json:"handle,omitempty"` // a child/task handle to cancel

	// approve
	ApprovalID string `json:"approval_id,omitempty"`
	Decision   string `json:"decision,omitempty"` // approve | deny
	Reason     string `json:"reason,omitempty"`

	// IdemKey makes run/drive submission idempotent within the daemon's
	// lifetime (DESIGN S6 修订): a retry with the same key attaches to the
	// session the first submission created instead of minting a new one.
	IdemKey string `json:"idem_key,omitempty"`
}

// RunRequest is what the daemon hands the injected runner. Every hosted
// session gets the live channels (决策 #31: only one session shape).
// Inbox delivers user inputs — the runner wires it to the Loop's
// UserInputs; closing it is the close gesture.
type RunRequest struct {
	SessionID  string
	SpecPath   string
	Task       string
	Workspace  string
	Mode       string
	Inbox      <-chan protocol.UserInput
	Interrupts <-chan struct{}
	Cancels    <-chan string
}

// RunFunc hosts one run to completion, emitting output events to sink. The
// CLI injects the real wiring (provider, pipeline, store); tests inject
// fakes. It MUST journal through the normal store so attach replay works.
type RunFunc func(ctx context.Context, req RunRequest, sink protocol.Sink) error

// ResumeRequest is what the daemon hands the resume runner. The channels
// are always provided and always wired (决策 #31: only one session shape).
type ResumeRequest struct {
	SessionID  string
	Inbox      <-chan protocol.UserInput
	Interrupts <-chan struct{}
	Cancels    <-chan string
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
	// SessionMarked reports whether a session's journal carries a
	// close/kill mark (决策 #30). AUTOMATIC revival (timer sweep) checks it
	// and skips marked sessions; an explicit send never does — any session
	// lawfully continues on a user's gesture. nil = never marked (tests).
	SessionMarked func(sessionID string) (marked bool, err error)
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
	// stop tears the hosted loop down (决策 #32 agent switch): a plain ctx
	// cancel — no mark, no ending; the journal simply stops mid-standby and
	// the next send revives it (with whatever spec the journal then names).
	stop context.CancelFunc
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
func (h *hostedRun) killHandle(handle string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done || h.cancels == nil {
		return false
	}
	select {
	case h.cancels <- handle:
		return true
	default:
		return false
	}
}

// signalInterrupt delivers one interrupt to the hosted loop (best-effort:
// a full buffer means one is already pending).
func (h *hostedRun) signalInterrupt() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done || h.interrupts == nil {
		return false
	}
	select {
	case h.interrupts <- struct{}{}:
		return true
	default:
		return true // one already pending — the loop will see it
	}
}

// post delivers a conversational input to the hosted session. The loop
// journals it on receipt (journal-inputs-first on the consume side); a
// crash between this enqueue and that journal loses the input — the
// durable send-side ack is a v2 M5 refinement (记档).
func (h *hostedRun) post(in protocol.UserInput) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done || h.inbox == nil {
		return false
	}
	select {
	case h.inbox <- in:
		return true
	default:
		return false // inbox full: the caller retries
	}
}

// Emit implements protocol.Sink: fan out to every subscriber. A slow
// subscriber's overflow is DROPPED (可丢 delta doctrine — the journal is the
// durable truth; the live stream is ephemeral rendering). Lifecycle events
// additionally tee to the notifier hook, outside the lock.
func (h *hostedRun) Emit(e protocol.Event) {
	e.Session = h.id
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
	defer h.mu.Unlock()
	h.done = true
	if h.inbox != nil {
		close(h.inbox) // unblock a idle conversational loop into close
		h.inbox = nil
	}
	for ch := range h.subs {
		close(ch)
		delete(h.subs, ch)
	}
}

// closeInbox is the `close` gesture for a conversational session: shut the
// inbox so the idle loop resolves into its epilogue. The run's own
// finish() then clears the registry.
func (h *hostedRun) closeInbox() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done || h.inbox == nil {
		return false
	}
	close(h.inbox)
	h.inbox = nil
	return true
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
		s.handleApprove(cmd, enc)
	case "send":
		s.handleSend(ctx, cmd, enc)
	case "close":
		s.handleClose(cmd, enc)
	case "interrupt":
		s.handleInterrupt(cmd, enc)
	case "kill":
		s.handleKill(cmd, enc)
	case "agent":
		s.handleAgent(cmd, enc)
	default:
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("unknown command %q (known: ping, run, attach, approve, send, close, interrupt, kill, agent)", cmd.Cmd)})
	}
}

// handleKill cancels one running child/task by handle (v2 M3.2): the user's
// direct kill path, distinct from the model calling kill.
func (s *Server) handleKill(cmd Command, enc *json.Encoder) {
	if cmd.Session == "" || cmd.Handle == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "kill needs session and handle"})
		return
	}
	s.mu.Lock()
	hub, ok := s.runs[cmd.Session]
	s.mu.Unlock()
	if !ok || !hub.killHandle(cmd.Handle) {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("no live session %s accepting kills", cmd.Session)})
		return
	}
	_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage,
		Text: "killing " + cmd.Handle, Session: cmd.Session})
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
func (s *Server) handleInterrupt(cmd Command, enc *json.Encoder) {
	if cmd.Session == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "interrupt needs session"})
		return
	}
	s.mu.Lock()
	hub, ok := s.runs[cmd.Session]
	s.mu.Unlock()
	if !ok || !hub.signalInterrupt() {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("no live interruptible session %s", cmd.Session)})
		return
	}
	_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage, Text: "interrupted", Session: cmd.Session})
}

// handleSend delivers a user message to a live conversational session
// (v2 M1.2). It is the machine/web/CLI-agnostic投递入口 — every sender
// (human at a terminal, web UI, webhook) posts through the same path.
func (s *Server) handleSend(ctx context.Context, cmd Command, enc *json.Encoder) {
	if cmd.Session == "" || cmd.Text == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "send needs session and text"})
		return
	}
	s.mu.Lock()
	hub, ok := s.runs[cmd.Session]
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
		delete(s.failed, cmd.Session) // an explicit send retries a failed resume
		s.mu.Unlock()
		// The revived run must live on the DAEMON's lifecycle (收口 review:
		// a Background ctx would wedge graceful shutdown in runsWG.Wait).
		s.hostResume(ctx, cmd.Session, true)
		s.mu.Lock()
		hub, ok = s.runs[cmd.Session]
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
	in := protocol.UserInput{Text: cmd.Text, Images: cmd.Images}
	if s.PersistInput != nil {
		// Durability before the ack (铁律 2): once "delivered" is on the
		// wire, a crash cannot lose this input — resume replays the
		// mailbox tail the journal has not consumed.
		var perr error
		if in, perr = s.PersistInput(cmd.Session, in); perr != nil {
			_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
				Text: "send not accepted (mailbox write failed): " + perr.Error()})
			return
		}
	}
	if !hub.post(in) {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("session %s is not accepting input (shutting down, or inbox full)", cmd.Session)})
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

// handleClose ends a conversational session gracefully (v2 M1.2): shutting
// the inbox resolves the idle loop into its epilogue.
func (s *Server) handleClose(cmd Command, enc *json.Encoder) {
	if cmd.Session == "" {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "close needs session"})
		return
	}
	s.mu.Lock()
	hub, ok := s.runs[cmd.Session]
	s.mu.Unlock()
	if !ok || !hub.closeInbox() {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("no live conversational session %s", cmd.Session)})
		return
	}
	_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage, Text: "closing", Session: cmd.Session})
}

// handleApprove routes a human's verdict to the idle ask.
func (s *Server) handleApprove(cmd Command, enc *json.Encoder) {
	if s.Approvals == nil {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError, Text: "daemon has no approval broker"})
		return
	}
	if cmd.Session == "" || cmd.ApprovalID == "" || (cmd.Decision != "approve" && cmd.Decision != "deny") {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: "approve needs session, approval_id and decision approve|deny"})
		return
	}
	ok := s.Approvals.Answer(cmd.Session, cmd.ApprovalID, ApprovalAnswer{
		Approve: cmd.Decision == "approve", Reason: cmd.Reason,
	})
	if !ok {
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("no pending approval %s on session %s", cmd.ApprovalID, cmd.Session)})
		return
	}
	_ = enc.Encode(protocol.Event{Kind: protocol.KindMessage,
		Text: "answered " + cmd.ApprovalID + ": " + cmd.Decision, Session: cmd.Session})
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
	hub := &hostedRun{id: id, notify: s.Notify, subs: map[chan protocol.Event]struct{}{}}
	hub.inbox = make(chan protocol.UserInput, 64) // type-ahead buffer
	hub.interrupts = make(chan struct{}, 1)
	hub.cancels = make(chan string, 8)
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
	hub := &hostedRun{id: id, notify: s.Notify, subs: map[chan protocol.Event]struct{}{}}
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

	go func() {
		defer s.runsWG.Done()
		defer func() {
			s.mu.Lock()
			delete(s.runs, id)
			s.mu.Unlock()
		}()
		defer hub.finish()
		if err := s.Drive(ctx, DriveRequest{
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
	s.mu.Lock()
	hub := s.runs[cmd.Session]
	s.mu.Unlock()

	// Subscribe BEFORE replay so no live event slips between the two; the
	// client may see an event twice around the seam, never a gap.
	var ch chan protocol.Event
	var cancel func()
	if hub != nil {
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
