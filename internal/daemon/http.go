// INC-50 (G14/UJ-12): the daemon's HTTP ingress — the machine sender's
// delivery shell. One narrow endpoint (POST /hooks/<id>) turns an external
// event into an ordinary durable send on the SAME channel as every human
// input; it is not an HTTP API shell (that stays backlog). Security posture
// (DESIGN §2 machine-sender clause, 决策 #39):
//   - off by default; explicit --http opt-in;
//   - per-hook capability: unguessable id + bearer token verified in
//     constant time against a stored hash;
//   - failed auth is rate-limited (budget-DoS), bodies are capped;
//   - the payload is journaled source:"machine" / trust:"untrusted" and the
//     loop frames it as untrusted data — never operator instructions;
//   - machine mail is not user-class: it cannot revive a session carrying a
//     user close/kill mark (决策 #30).
package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
)

// hookBodyMax caps an ingress payload. Webhook payloads are summaries, not
// artifacts; anything larger is a mistake or an attack on the token budget.
const hookBodyMax = 256 << 10

// dummyTokenHash gives unknown-hook requests the same verification cost as
// known-hook ones (never matches: sha256 output is hex, this is not).
const dummyTokenHash = "!novalidtoken!novalidtoken!novalidtoken!novalidtoken!novalidtok"

// failLimiter is a token bucket over FAILED authentications: sustained
// guessing gets 429 instead of free hash oracles. Successful requests are
// never throttled by it.
type failLimiter struct {
	mu     sync.Mutex
	tokens float64
	last   time.Time
	rate   float64 // tokens per second
	burst  float64
}

func newFailLimiter(perMinute float64) *failLimiter {
	return &failLimiter{tokens: perMinute, rate: perMinute / 60, burst: perMinute}
}

// allow consumes one failure budget token; false = the caller should 429.
func (l *failLimiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if !l.last.IsZero() {
		l.tokens += now.Sub(l.last).Seconds() * l.rate
		if l.tokens > l.burst {
			l.tokens = l.burst
		}
	}
	l.last = now
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}

// serveHTTP binds the ingress listener and serves until ctx is done. The
// daemon's root ctx (NOT per-request contexts) drives session lifecycles:
// a webhook client disconnecting must never cancel a revived session.
func (s *Server) serveHTTP(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.HTTPAddr)
	if err != nil {
		return fmt.Errorf("daemon http: %w", err)
	}
	if s.HTTPAddrFile != "" {
		if err := writeOwnerFile(s.HTTPAddrFile, ln.Addr().String()); err != nil {
			_ = ln.Close()
			return fmt.Errorf("daemon http: addr file: %w", err)
		}
	}
	mux := http.NewServeMux()
	limiter := newFailLimiter(10)
	mux.HandleFunc("POST /hooks/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.handleHook(ctx, limiter, w, r)
	})
	// Full read/write/idle timeouts (安全 review P1-1): ReadHeaderTimeout
	// alone leaves the BODY read unbounded — a slow-body client could pin
	// goroutines forever (slowloris).
	srv := &http.Server{Handler: mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	slog.Info("daemon http ingress listening", "addr", ln.Addr().String())
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Warn("daemon http ingress stopped", "err", err)
		}
	}()
	return nil
}

// handleHook authenticates one delivery and forwards it through the exact
// send path a human uses (durable command log, idempotent CommandID,
// send-as-resume) — with the machine sender's restrictions.
func (s *Server) handleHook(ctx context.Context, limiter *failLimiter, w http.ResponseWriter, r *http.Request) {
	fail := func(code int, msg string) {
		if code == http.StatusUnauthorized && !limiter.allow() {
			code, msg = http.StatusTooManyRequests, "too many failed attempts"
		}
		writeHookJSON(w, code, map[string]any{"error": msg})
	}
	// Authenticate BEFORE touching the body (安全 review P1-1): an
	// unauthenticated request never earns a 256KiB buffer.
	hook, ok, err := FindHook(s.HooksPath, r.PathValue("id"))
	if err != nil {
		writeHookJSON(w, http.StatusInternalServerError, map[string]any{"error": "hook registry unreadable"})
		return
	}
	// Require the documented "Bearer <token>" scheme. TrimPrefix used to leave
	// a bare, scheme-less token intact and accept it — looser than documented
	// (QA Wave3 judy-05). Only take the token when the Bearer prefix is
	// actually present; otherwise it stays empty and is rejected below.
	token := ""
	if after, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); found {
		token = after
	}
	// Unknown hook and bad token answer identically — status, body AND
	// hashing cost (a dummy verify keeps the timing flat; P2-1): no
	// existence oracle.
	target := Hook{TokenSHA256: dummyTokenHash}
	if ok {
		target = hook
	}
	if token == "" || !target.VerifyToken(token) || !ok {
		fail(http.StatusUnauthorized, "unknown hook or bad token")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, hookBodyMax))
	if err != nil {
		writeHookJSON(w, http.StatusRequestEntityTooLarge,
			map[string]any{"error": fmt.Sprintf("body exceeds %d bytes", hookBodyMax)})
		return
	}

	// Machine mail never overrides a user close/kill mark (决策 #30). The
	// pre-check gives the caller an honest 410; the send path enforces the
	// same rule (non-user-class resume) as defense in depth.
	host := hook.Session
	if s.SplitAddress != nil {
		if h, target := s.SplitAddress(hook.Session); target != "" {
			host = h
		}
	}
	s.mu.Lock()
	_, hosted := s.runs[host]
	s.mu.Unlock()
	if !hosted && s.SessionMarked != nil {
		if marked, merr := s.SessionMarked(host); merr == nil && marked {
			writeHookJSON(w, http.StatusGone,
				map[string]any{"error": "session was closed or killed by its user; a machine sender cannot revive it"})
			return
		}
	}

	text := hookText(r.Header.Get("Content-Type"), body)
	if strings.TrimSpace(text) == "" {
		writeHookJSON(w, http.StatusBadRequest, map[string]any{"error": "empty payload"})
		return
	}
	commandID := r.Header.Get("X-Command-Id")
	if commandID == "" {
		commandID = event.NewCommandID()
	}
	cmd := Command{
		Cmd: "send", Session: hook.Session, Text: text, CommandID: commandID,
		Source: protocol.SourceMachine, Trust: "untrusted", Principal: hook.Principal(),
	}
	// Reuse the socket send handler verbatim: its first (and, without
	// Follow, only) encoded event is the delivery verdict.
	var buf bytes.Buffer
	s.handleSend(ctx, cmd, json.NewEncoder(&buf))
	verdict := protocol.Event{}
	if sc := bufio.NewScanner(&buf); sc.Scan() {
		_ = json.Unmarshal(sc.Bytes(), &verdict)
	}
	if verdict.Kind == protocol.KindError || verdict.Kind == "" {
		msg := verdict.Text
		if msg == "" {
			msg = "delivery failed"
		}
		writeHookJSON(w, http.StatusBadGateway, map[string]any{"error": msg})
		return
	}
	writeHookJSON(w, http.StatusAccepted, map[string]any{
		"delivered": true, "session": hook.Session, "command_id": commandID,
	})
}

// hookText normalizes a payload into conversational text: a JSON body may
// carry {"text": ...}; anything else is taken verbatim.
func hookText(contentType string, body []byte) string {
	if strings.Contains(contentType, "application/json") {
		var probe struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(body, &probe); err == nil && probe.Text != "" {
			return probe.Text
		}
	}
	return string(body)
}

// writeOwnerFile persists a small owner-only rendezvous file (the bound
// ingress address, for `ar hook create` URL printing and QA discovery).
// Atomic (temp+rename) so a reader never sees a torn address.
func writeOwnerFile(path, content string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeHookJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
