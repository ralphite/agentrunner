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

	"github.com/ralphite/agentrunner/internal/protocol"
)

// Command is one client→server line.
type Command struct {
	Cmd string `json:"cmd"` // ping | run | attach

	// run
	SpecPath  string `json:"spec_path,omitempty"`
	Task      string `json:"task,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Mode      string `json:"mode,omitempty"`

	// attach
	Session string `json:"session,omitempty"`
}

// RunRequest is what the daemon hands the injected runner.
type RunRequest struct {
	SessionID string
	SpecPath  string
	Task      string
	Workspace string
	Mode      string
}

// RunFunc hosts one run to completion, emitting output events to sink. The
// CLI injects the real wiring (provider, pipeline, store); tests inject
// fakes. It MUST journal through the normal store so attach replay works.
type RunFunc func(ctx context.Context, req RunRequest, sink protocol.Sink) error

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

	mu   sync.Mutex
	runs map[string]*hostedRun
	ln   net.Listener
	wg   sync.WaitGroup
}

// hostedRun is one live run's broadcast hub.
type hostedRun struct {
	id   string
	mu   sync.Mutex
	subs map[chan protocol.Event]struct{}
	done bool
}

// Emit implements protocol.Sink: fan out to every subscriber. A slow
// subscriber's overflow is DROPPED (可丢 delta doctrine — the journal is the
// durable truth; the live stream is ephemeral rendering).
func (h *hostedRun) Emit(e protocol.Event) {
	e.Session = h.id
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
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
	for ch := range h.subs {
		close(ch)
		delete(h.subs, ch)
	}
}

// ListenAndServe binds the socket and serves until ctx is done. A stale
// socket file (no listener behind it) is removed; a LIVE daemon on the same
// path is an error — two daemons must not split the session space.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.runs == nil {
		s.runs = map[string]*hostedRun{}
	}
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
	s.ln = ln
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	slog.Info("daemon listening", "socket", s.SocketPath)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				s.wg.Wait() // drain in-flight connections before returning
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
	defer func() { _ = conn.Close() }()
	enc := json.NewEncoder(conn)
	var cmd Command
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
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
	case "attach":
		s.handleAttach(cmd, enc)
	default:
		_ = enc.Encode(protocol.Event{Kind: protocol.KindError,
			Text: fmt.Sprintf("unknown command %q (known: ping, run, attach)", cmd.Cmd)})
	}
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
	id := s.NewID(cmd.Task)
	hub := &hostedRun{id: id, subs: map[chan protocol.Event]struct{}{}}
	s.mu.Lock()
	s.runs[id] = hub
	s.mu.Unlock()

	ch, cancel, _ := hub.subscribe()
	defer cancel()

	// The run runs on the daemon's ctx (not the connection's): it survives
	// the client going away.
	go func() {
		defer hub.finish()
		if err := s.Run(ctx, RunRequest{
			SessionID: id, SpecPath: cmd.SpecPath, Task: cmd.Task,
			Workspace: cmd.Workspace, Mode: cmd.Mode,
		}, hub); err != nil {
			hub.Emit(protocol.Event{Kind: protocol.KindError, Text: "run failed: " + err.Error()})
		}
	}()

	// Tell the client which session it got, then stream until the run ends.
	_ = enc.Encode(protocol.Event{Kind: protocol.KindRunStart, Session: id})
	for e := range ch {
		if err := enc.Encode(e); err != nil {
			return // client went away; the run keeps going
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

// sinkFunc adapts a func to protocol.Sink.
type sinkFunc func(protocol.Event)

func (f sinkFunc) Emit(e protocol.Event) { f(e) }

// Dial connects to a daemon socket and issues one command, streaming the
// response events to onEvent until the server closes the stream.
func Dial(socketPath string, cmd Command, onEvent func(protocol.Event)) error {
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
		onEvent(e)
	}
	if err := sc.Err(); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}
