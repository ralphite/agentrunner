// INC-50 (G14/UJ-12) twins: the webhook ingress delivers external events
// into a session's durable inbox as untrusted machine input — with the
// machine sender's restrictions (auth, rate limit, body cap, no revive of
// user-marked sessions, idempotent redelivery).
package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/protocol"
)

type ingressEnv struct {
	base      string // http://addr
	hooksPath string
	cancel    context.CancelFunc
}

// ingressHarness starts a daemon with the HTTP ingress on an ephemeral port.
func ingressHarness(t *testing.T, resume func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error,
	marked func(string) (bool, error)) ingressEnv {
	t.Helper()
	tmp := shortTempDir(t)
	sock := filepath.Join(tmp, "d.sock")
	hooks := filepath.Join(tmp, "hooks.json")
	addrFile := filepath.Join(tmp, "http.addr")
	ctx, cancel := context.WithCancel(context.Background())
	srv := &Server{
		SocketPath: sock, NewID: func(string) string { return "x" },
		Resume: resume, SessionMarked: marked,
		HTTPAddr: "127.0.0.1:0", HTTPAddrFile: addrFile, HooksPath: hooks,
	}
	go func() { _ = srv.ListenAndServe(ctx) }()
	t.Cleanup(cancel)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(addrFile); err == nil && len(raw) > 0 {
			if err := Dial(sock, Command{Cmd: "ping"}, func(protocol.Event) {}); err == nil {
				return ingressEnv{base: "http://" + strings.TrimSpace(string(raw)),
					hooksPath: hooks, cancel: cancel}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("ingress daemon never came up")
	return ingressEnv{}
}

func postHook(t *testing.T, env ingressEnv, hookID, token, body string, hdr map[string]string) (int, string) {
	t.Helper()
	req, err := http.NewRequest("POST", env.base+"/hooks/"+hookID, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(res.Body)
	return res.StatusCode, buf.String()
}

// The happy path: an authenticated POST revives the parked session and its
// inbox receives the payload as source:"machine" / trust:"untrusted" with
// the hook's principal — the exact contract journalInput frames on.
func TestHookIngressDeliversMachineInput(t *testing.T) {
	got := make(chan protocol.UserInput, 8)
	resume := func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
		for {
			select {
			case in := <-req.Inbox:
				got <- in
			case <-ctx.Done():
				return nil
			}
		}
	}
	env := ingressHarness(t, resume, func(string) (bool, error) { return false, nil })
	hk, token, err := CreateHook(env.hooksPath, "sess-1", "ci")
	if err != nil {
		t.Fatal(err)
	}

	code, body := postHook(t, env, hk.ID, token, `{"text":"CI run 42 failed on main"}`,
		map[string]string{"Content-Type": "application/json"})
	if code != http.StatusAccepted {
		t.Fatalf("delivery = %d %s", code, body)
	}
	var ack struct {
		Delivered bool   `json:"delivered"`
		CommandID string `json:"command_id"`
	}
	if err := json.Unmarshal([]byte(body), &ack); err != nil || !ack.Delivered || ack.CommandID == "" {
		t.Fatalf("ack = %s", body)
	}
	select {
	case in := <-got:
		if in.Source != protocol.SourceMachine || in.Trust != "untrusted" ||
			in.Principal != "hook:ci" || in.Text != "CI run 42 failed on main" {
			t.Fatalf("delivered input = %+v", in)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("payload never reached the inbox")
	}
}

// Bad token and unknown hook answer identically (401, no existence oracle)
// and deliver nothing; sustained failures trip the rate limiter (429).
func TestHookIngressAuthAndRateLimit(t *testing.T) {
	var resumed atomic.Int32
	resume := func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
		resumed.Add(1)
		<-ctx.Done()
		return nil
	}
	env := ingressHarness(t, resume, func(string) (bool, error) { return false, nil })
	hk, _, err := CreateHook(env.hooksPath, "sess-1", "ci")
	if err != nil {
		t.Fatal(err)
	}

	if code, _ := postHook(t, env, hk.ID, "wrong-token", "x", nil); code != http.StatusUnauthorized {
		t.Fatalf("bad token = %d, want 401", code)
	}
	if code, _ := postHook(t, env, "no-such-hook", "whatever", "x", nil); code != http.StatusUnauthorized {
		t.Fatalf("unknown hook = %d, want 401", code)
	}
	// The failure budget is 10/min: hammering bad tokens must flip to 429.
	saw429 := false
	for i := 0; i < 15 && !saw429; i++ {
		code, _ := postHook(t, env, hk.ID, "wrong-token", "x", nil)
		saw429 = code == http.StatusTooManyRequests
	}
	if !saw429 {
		t.Fatal("sustained auth failures never rate-limited")
	}
	if resumed.Load() != 0 {
		t.Fatal("an unauthenticated request revived a session")
	}
}

// A machine sender cannot revive a session its user closed or killed
// (决策 #30: the override privilege is user-class only) — honest 410.
func TestHookIngressCannotReviveMarkedSession(t *testing.T) {
	var resumed atomic.Int32
	resume := func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
		resumed.Add(1)
		<-ctx.Done()
		return nil
	}
	env := ingressHarness(t, resume, func(string) (bool, error) { return true, nil })
	hk, token, err := CreateHook(env.hooksPath, "sess-killed", "ci")
	if err != nil {
		t.Fatal(err)
	}
	code, body := postHook(t, env, hk.ID, token, "wake up", nil)
	if code != http.StatusGone {
		t.Fatalf("machine delivery to marked session = %d %s, want 410", code, body)
	}
	if resumed.Load() != 0 {
		t.Fatal("machine mail revived a user-marked session")
	}
}

// Oversized payloads are refused (413) — the ingress is a summary channel,
// not an artifact upload, and the cap protects the token budget.
func TestHookIngressBodyCap(t *testing.T) {
	env := ingressHarness(t, func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
		<-ctx.Done()
		return nil
	}, func(string) (bool, error) { return false, nil })
	hk, token, err := CreateHook(env.hooksPath, "sess-1", "")
	if err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("a", hookBodyMax+1)
	code, _ := postHook(t, env, hk.ID, token, huge, nil)
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body = %d, want 413", code)
	}
}

// Webhook retries with the same X-Command-Id are idempotent end to end:
// both POSTs ack, the session sees the event once (铁律 3).
func TestHookIngressIdempotentRedelivery(t *testing.T) {
	got := make(chan protocol.UserInput, 8)
	resume := func(ctx context.Context, req ResumeRequest, sink protocol.Sink) error {
		for {
			select {
			case in := <-req.Inbox:
				got <- in
			case <-ctx.Done():
				return nil
			}
		}
	}
	env := ingressHarness(t, resume, func(string) (bool, error) { return false, nil })
	hk, token, err := CreateHook(env.hooksPath, "sess-1", "ci")
	if err != nil {
		t.Fatal(err)
	}
	hdr := map[string]string{"X-Command-Id": "hook-evt-7"}
	for i := 0; i < 2; i++ {
		if code, body := postHook(t, env, hk.ID, token, "same event", hdr); code != http.StatusAccepted {
			t.Fatalf("redelivery %d = %d %s", i, code, body)
		}
	}
	select {
	case in := <-got:
		if in.CommandID != "hook-evt-7" {
			t.Fatalf("command id = %q", in.CommandID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("payload never reached the inbox")
	}
	select {
	case in := <-got:
		t.Fatalf("duplicate delivery reached the loop: %+v", in)
	case <-time.After(300 * time.Millisecond):
	}
}

// The registry stores only a token hash — the plaintext exists once, on the
// create response — and revoke removes the capability.
func TestHookRegistryHashesAndRevokes(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "hooks.json")
	hk, token, err := CreateHook(path, "sess-1", "ci")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), token) {
		t.Fatal("registry persisted the plaintext token")
	}
	if !hk.VerifyToken(token) || hk.VerifyToken("not-the-token") {
		t.Fatal("token verification broken")
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("registry must be owner-only, got %v (%v)", fi.Mode(), err)
	}
	found, err := RevokeHook(path, hk.ID)
	if err != nil || !found {
		t.Fatalf("revoke = %v %v", found, err)
	}
	if _, ok, _ := FindHook(path, hk.ID); ok {
		t.Fatal("revoked hook still resolves")
	}
	if found, _ := RevokeHook(path, hk.ID); found {
		t.Fatal("double revoke reported a hit")
	}
}

func TestHookRegistryConcurrentCreatesLoseNoHooks(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	path := filepath.Join(shortTempDir(t), "hooks.json")
	const writers = 24
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, _, err := CreateHook(path, "sess", fmt.Sprintf("hook-%02d", i)); err != nil {
				t.Errorf("CreateHook: %v", err)
			}
		}(i)
	}
	wg.Wait()
	hooks, err := LoadHooks(path)
	if err != nil || len(hooks) != writers {
		t.Fatalf("hooks = %d, err = %v; want %d", len(hooks), err, writers)
	}
}

// Non-JSON bodies land verbatim; a JSON body may carry {"text": ...}.
func TestHookTextNormalization(t *testing.T) {
	if got := hookText("application/json", []byte(`{"text":"hello"}`)); got != "hello" {
		t.Fatalf("json text = %q", got)
	}
	if got := hookText("application/json", []byte(`{"other":"x"}`)); got != `{"other":"x"}` {
		t.Fatalf("json passthrough = %q", got)
	}
	if got := hookText("text/plain", []byte("raw body")); got != "raw body" {
		t.Fatalf("raw = %q", got)
	}
}

// Guard: the limiter refills over time and never exceeds its burst.
func TestFailLimiterRefill(t *testing.T) {
	l := newFailLimiter(60) // 1/s for test speed
	for i := 0; i < 60; i++ {
		if !l.allow() {
			t.Fatalf("burst budget exhausted early at %d", i)
		}
	}
	if l.allow() {
		t.Fatal("over-burst failure allowed")
	}
	time.Sleep(1100 * time.Millisecond)
	if !l.allow() {
		t.Fatal("limiter never refilled")
	}
}
