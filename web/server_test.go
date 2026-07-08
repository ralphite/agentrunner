package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFakeAR installs a shell-script stand-in for the ar binary that emits
// canned outputs per subcommand and records every argv line to $AR_FAKE_LOG.
// Unit tests exercise the exec/parse layer only — never a real API.
func writeFakeAR(t *testing.T) (arPath, argvLog string) {
	t.Helper()
	dir := t.TempDir()
	argvLog = filepath.Join(dir, "argv.log")
	script := `#!/bin/sh
echo "$@" >> "$AR_FAKE_LOG"
case "$1" in
--version) echo "agentrunner test (go0)";;
sessions) printf 'SESSION                                       STATUS             TURNS\nsess-abc-123                                  waiting:input      3\n';;
events)
  case "$2" in
  --state) echo '{"session":{"status":"running"}}';;
  --json) printf '{"seq":1,"type":"session_started","payload":{"spec_name":"dev"}}\n{"seq":2,"type":"input_received","payload":{"text":"hi","source":"user"}}\n';;
  esac;;
new) echo "sess-new-456";;
agent) echo "agent switched to auditor";;
send|interrupt|kill|approve) echo ok;;
inspect) echo '{"tree":true}';;
ps) printf 'task-1\tspawn_agent\trunning agent=worker task=explore\n';;
attach) printf '{"kind":"idle","session":"%s"}\n' "$3"; sleep 3;;
esac
`
	arPath = filepath.Join(dir, "ar")
	if err := os.WriteFile(arPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AR_FAKE_LOG", argvLog)
	return arPath, argvLog
}

func newTestServer(t *testing.T) (*server, string) {
	t.Helper()
	ar, argvLog := writeFakeAR(t)
	rt := t.TempDir()
	for _, d := range []string{"specs", "uploads", "ws"} {
		if err := os.MkdirAll(filepath.Join(rt, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return &server{arPath: ar, runtimeDir: rt}, argvLog
}

func doJSON(t *testing.T, s *server, method, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestSessionsListParse(t *testing.T) {
	s, _ := newTestServer(t)
	code, body := doJSON(t, s, "GET", "/api/sessions", "")
	if code != 200 {
		t.Fatalf("code=%d body=%s", code, body)
	}
	var rows []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Turns  int    `json:"turns"`
	}
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "sess-abc-123" || rows[0].Status != "waiting:input" || rows[0].Turns != 3 {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestEventsAfterFilter(t *testing.T) {
	s, _ := newTestServer(t)
	code, body := doJSON(t, s, "GET", "/api/sessions/sess-abc-123/events?after=1", "")
	if code != 200 {
		t.Fatalf("code=%d body=%s", code, body)
	}
	var evs []struct {
		Seq  int64  `json:"seq"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(body), &evs); err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Seq != 2 || evs[0].Type != "input_received" {
		t.Fatalf("evs=%+v", evs)
	}
}

func TestNewSessionWritesSpecsAndParsesSid(t *testing.T) {
	s, argvLog := newTestServer(t)
	ws := t.TempDir()
	req := fmt.Sprintf(`{"spec":"name: dev\n","extraSpecs":[{"name":"worker.yaml","content":"name: worker\n"}],"workspace":%q,"message":"hello"}`, ws)
	code, body := doJSON(t, s, "POST", "/api/sessions", req)
	if code != 200 {
		t.Fatalf("code=%d body=%s", code, body)
	}
	var resp struct {
		Sid     string `json:"sid"`
		SpecDir string `json:"specDir"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Sid != "sess-new-456" {
		t.Fatalf("sid=%q", resp.Sid)
	}
	for _, f := range []string{"base.yaml", "worker.yaml"} {
		if _, err := os.Stat(filepath.Join(resp.SpecDir, f)); err != nil {
			t.Fatalf("spec file %s: %v", f, err)
		}
	}
	argv, _ := os.ReadFile(argvLog)
	line := strings.TrimSpace(string(argv))
	// --detach:INC-2 后 new 默认跟随渲染回复,驾驶舱要 ack-only 形式。
	if !strings.Contains(line, "new --detach --workspace "+ws) || !strings.Contains(line, "base.yaml hello") {
		t.Fatalf("argv=%q", line)
	}
}

func TestAgentSwitchWritesSpecAndCallsAR(t *testing.T) {
	s, argvLog := newTestServer(t)
	req := `{"spec":"name: auditor\n","extraSpecs":[{"name":"worker.yaml","content":"name: worker\n"}]}`
	code, body := doJSON(t, s, "POST", "/api/sessions/sess-abc-123/agent", req)
	if code != 200 {
		t.Fatalf("code=%d body=%s", code, body)
	}
	var resp struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "agent switched to auditor" {
		t.Fatalf("status=%q", resp.Status)
	}
	argv, _ := os.ReadFile(argvLog)
	line := strings.TrimSpace(string(argv))
	if !strings.HasPrefix(line, "agent sess-abc-123 ") || !strings.HasSuffix(line, "/agent.yaml") {
		t.Fatalf("argv=%q", line)
	}
	specPath := strings.TrimPrefix(line, "agent sess-abc-123 ")
	for _, f := range []string{specPath, filepath.Join(filepath.Dir(specPath), "worker.yaml")} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("spec file %s: %v", f, err)
		}
	}
}

func TestSendBuildsImageArgs(t *testing.T) {
	s, argvLog := newTestServer(t)
	img := filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(img, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := fmt.Sprintf(`{"text":"看图","images":[%q]}`, img)
	code, body := doJSON(t, s, "POST", "/api/sessions/sess-abc-123/send", req)
	if code != 200 {
		t.Fatalf("code=%d body=%s", code, body)
	}
	argv, _ := os.ReadFile(argvLog)
	want := "send --detach --image " + img + " sess-abc-123 看图"
	if got := strings.TrimSpace(string(argv)); got != want {
		t.Fatalf("argv=%q want %q", got, want)
	}
}

func TestKillValidatesHandle(t *testing.T) {
	s, argvLog := newTestServer(t)
	code, _ := doJSON(t, s, "POST", "/api/sessions/sess-abc-123/kill", `{"handle":"bad handle!"}`)
	if code != 400 {
		t.Fatalf("code=%d, want 400", code)
	}
	code, _ = doJSON(t, s, "POST", "/api/sessions/sess-abc-123/kill", `{"handle":"task-1"}`)
	if code != 200 {
		t.Fatalf("code=%d", code)
	}
	argv, _ := os.ReadFile(argvLog)
	if got := strings.TrimSpace(string(argv)); got != "kill sess-abc-123 task-1" {
		t.Fatalf("argv=%q", got)
	}
}

func TestApproveArgs(t *testing.T) {
	s, argvLog := newTestServer(t)
	code, body := doJSON(t, s, "POST", "/api/sessions/sess-abc-123/approve",
		`{"approvalId":"appr-1","decision":"deny","reason":"不行"}`)
	if code != 200 {
		t.Fatalf("code=%d body=%s", code, body)
	}
	argv, _ := os.ReadFile(argvLog)
	if got := strings.TrimSpace(string(argv)); got != "approve sess-abc-123 appr-1 deny 不行" {
		t.Fatalf("argv=%q", got)
	}
}

func TestPSParse(t *testing.T) {
	s, _ := newTestServer(t)
	code, body := doJSON(t, s, "GET", "/api/sessions/sess-abc-123/ps", "")
	if code != 200 {
		t.Fatalf("code=%d body=%s", code, body)
	}
	var tasks []struct{ Handle, Tool, Detail string }
	if err := json.Unmarshal([]byte(body), &tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Handle != "task-1" || tasks[0].Tool != "spawn_agent" {
		t.Fatalf("tasks=%+v", tasks)
	}
}

func TestSSEStreamFirstEvent(t *testing.T) {
	s, _ := newTestServer(t)
	ts := httptest.NewServer(s.routes())
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/sessions/sess-abc-123/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type=%q", ct)
	}
	var first string
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if line := sc.Text(); strings.HasPrefix(line, "data: ") {
			first = line
			break
		}
	}
	want := `data: {"kind":"idle","session":"sess-abc-123"}`
	if first != want {
		t.Fatalf("first sse line=%q want %q", first, want)
	}
}

func TestInvalidSessionID(t *testing.T) {
	s, _ := newTestServer(t)
	code, _ := doJSON(t, s, "GET", "/api/sessions/bad%20id/events", "")
	if code != 400 {
		t.Fatalf("code=%d, want 400", code)
	}
}
