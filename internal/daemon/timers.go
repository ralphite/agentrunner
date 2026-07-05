package daemon

import (
	"context"
	"log/slog"
	"time"

	"github.com/ralphite/agentrunner/internal/protocol"
)

// SessionTimer is one parked session's earliest pending timer, as derived
// from its journal (timer 派生索引 — the journal is the truth, this is a
// projection recomputed on every sweep).
type SessionTimer struct {
	SessionID string
	FireAt    time.Time
}

// sweepMaxInterval bounds how long the sweeper sleeps even with no known
// deadline: sessions parked by OTHER processes appear at the next rescan.
const sweepMaxInterval = time.Minute

// sweepTimers is the daemon's durable-timer trigger (S6 模块④, DESIGN:
// daemon 是 durable timer 的触发者): scan the parked sessions' pending
// timers, host a Resume for every expired one (the resume path itself
// journals TimerFired — 2.13's sweep), and sleep until the earliest future
// deadline or the rescan interval, whichever is sooner.
func (s *Server) sweepTimers(ctx context.Context) {
	for {
		entries, err := s.ScanTimers()
		if err != nil {
			slog.Warn("daemon: timer scan failed", "err", err)
		}
		now := s.Clock.Now()
		next := now.Add(sweepMaxInterval)
		for _, e := range entries {
			if s.resumeFailed(e.SessionID) {
				continue
			}
			if !e.FireAt.After(now) {
				s.hostResume(ctx, e.SessionID)
			} else if e.FireAt.Before(next) {
				next = e.FireAt
			}
		}
		if err := s.Clock.WaitUntil(ctx, next); err != nil {
			return // daemon shutting down
		}
	}
}

// resumeFailed reports whether an earlier timer-driven resume of this
// session errored — such sessions are skipped until the daemon restarts: an
// in-doubt session needs a human (inspect / events), and hammering it every
// sweep helps nobody.
func (s *Server) resumeFailed(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed[id]
}

func (s *Server) markResumeFailed(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed == nil {
		s.failed = map[string]bool{}
	}
	s.failed[id] = true
}

// hostResume hosts a session resume exactly like a submitted run: same hub,
// same runsWG, attach-able under the session id. A session already hosted
// (still running, or resumed by an earlier sweep) is left alone. The hub
// always carries conversational channels (v2 M5.1) — the resume runner
// wires them iff the journal says conversational, so a revived chat
// session accepts send/interrupt/kill exactly like a freshly hosted one.
func (s *Server) hostResume(ctx context.Context, id string) {
	// Shape first (v2 收口 review): an ended session must not be revived,
	// and a task-mode session must not grow conversational channels — a
	// post to a channel the loop never reads would false-ack "delivered".
	conversational := true
	if s.SessionShape != nil {
		var ended bool
		var err error
		conversational, ended, err = s.SessionShape(id)
		if err != nil || ended {
			return // handleSend reports "could not be resumed"
		}
	}
	s.mu.Lock()
	if s.stopping || s.runs[id] != nil {
		s.mu.Unlock()
		return
	}
	hub := &hostedRun{id: id, notify: s.Notify, subs: map[chan protocol.Event]struct{}{}}
	if conversational {
		hub.inbox = make(chan protocol.UserInput, 64)
		hub.interrupts = make(chan struct{}, 1)
		hub.cancels = make(chan string, 8)
	}
	s.runs[id] = hub
	s.runsWG.Add(1)
	s.mu.Unlock()

	slog.Info("daemon: resuming session", "session", id)
	go func() {
		defer s.runsWG.Done()
		defer func() {
			s.mu.Lock()
			delete(s.runs, id)
			s.mu.Unlock()
		}()
		defer hub.finish()
		if err := s.Resume(ctx, ResumeRequest{
			SessionID: id, Inbox: hub.inbox, Interrupts: hub.interrupts, Cancels: hub.cancels,
		}, hub); err != nil {
			slog.Warn("daemon: hosted resume failed", "session", id, "err", err)
			s.markResumeFailed(id)
			hub.Emit(protocol.Event{Kind: protocol.KindError, Text: "resume failed: " + err.Error()})
		}
	}()
}
