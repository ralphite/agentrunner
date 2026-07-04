package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/workspace"
)

func newExec(t *testing.T) (*Executor, string) {
	t.Helper()
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	return &Executor{WS: ws}, ws.Root()
}

func run(t *testing.T, e *Executor, name, args string) (map[string]any, bool) {
	t.Helper()
	res := e.Execute(context.Background(), name, json.RawMessage(args))
	var m map[string]any
	if err := json.Unmarshal(res.Payload, &m); err != nil {
		t.Fatalf("payload not JSON: %s", res.Payload)
	}
	return m, res.IsError
}

func TestReadFile(t *testing.T) {
	e, root := newExec(t)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, isErr := run(t, e, "read_file", `{"path":"a.txt"}`)
	if isErr || m["content"] != "hello" {
		t.Fatalf("m=%v isErr=%v", m, isErr)
	}

	_, isErr = run(t, e, "read_file", `{"path":"missing.txt"}`)
	if !isErr {
		t.Error("missing file should be an error result")
	}

	_, isErr = run(t, e, "read_file", `{"path":"../../etc/passwd"}`)
	if !isErr {
		t.Error("escape should be an error result")
	}
}

func TestReadFileTruncation(t *testing.T) {
	e, root := newExec(t)
	big := strings.Repeat("line\n", 3000)
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	m, isErr := run(t, e, "read_file", `{"path":"big.txt"}`)
	if isErr || m["truncated"] != true {
		t.Fatalf("m truncated=%v isErr=%v", m["truncated"], isErr)
	}
	if !strings.Contains(m["content"].(string), "[truncated:") {
		t.Error("truncation marker missing")
	}
}

func TestEditFile(t *testing.T) {
	e, root := newExec(t)
	path := filepath.Join(root, "code.go")
	if err := os.WriteFile(path, []byte("aaa bbb aaa"), 0o644); err != nil {
		t.Fatal(err)
	}

	// exactly-once replacement
	if _, isErr := run(t, e, "edit_file", `{"path":"code.go","old":"bbb","new":"XXX"}`); isErr {
		t.Fatal("single match should succeed")
	}
	content, _ := os.ReadFile(path)
	if string(content) != "aaa XXX aaa" {
		t.Fatalf("content = %q", content)
	}

	// zero and multiple matches fail with counts
	m, isErr := run(t, e, "edit_file", `{"path":"code.go","old":"zzz","new":"q"}`)
	if !isErr || !strings.Contains(m["error"].(string), "0 matches") {
		t.Errorf("zero-match error = %v", m)
	}
	m, isErr = run(t, e, "edit_file", `{"path":"code.go","old":"aaa","new":"q"}`)
	if !isErr || !strings.Contains(m["error"].(string), "2 times") {
		t.Errorf("multi-match error = %v", m)
	}
}

func TestEditFileCreate(t *testing.T) {
	e, root := newExec(t)
	if _, isErr := run(t, e, "edit_file", `{"path":"new/dir/f.txt","old":"","new":"fresh"}`); isErr {
		t.Fatal("create should succeed")
	}
	content, err := os.ReadFile(filepath.Join(root, "new", "dir", "f.txt"))
	if err != nil || string(content) != "fresh" {
		t.Fatalf("content=%q err=%v", content, err)
	}

	// creating over an existing file is refused
	if _, isErr := run(t, e, "edit_file", `{"path":"new/dir/f.txt","old":"","new":"clobber"}`); !isErr {
		t.Error("create over existing file should fail")
	}
}

func TestBashBasics(t *testing.T) {
	e, _ := newExec(t)
	m, isErr := run(t, e, "bash", `{"command":"echo hi && pwd"}`)
	if isErr || m["exit_code"].(float64) != 0 {
		t.Fatalf("m=%v isErr=%v", m, isErr)
	}
	if !strings.Contains(m["stdout"].(string), "hi") {
		t.Errorf("stdout = %q", m["stdout"])
	}

	m, isErr = run(t, e, "bash", `{"command":"exit 3"}`)
	if !isErr || m["exit_code"].(float64) != 3 {
		t.Errorf("nonzero exit: m=%v isErr=%v", m, isErr)
	}
}

func TestBashTimeoutKillsProcessGroup(t *testing.T) {
	e, root := newExec(t)

	// 2.11: the wall-clock limit is owned by the durable timer, which
	// cancels ctx with cause ErrActivityTimeout; bash renders timed_out.
	ctx, cancel := context.WithCancelCause(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel(errs.ErrActivityTimeout)
	}()

	// The marker file lets us find the grandchild's pid.
	cmd := fmt.Sprintf(`{"command":"echo $$ > %s/pgid.txt; sleep 30"}`, root)
	start := time.Now()
	res := e.Execute(ctx, "bash", json.RawMessage(cmd))
	var m map[string]any
	if err := json.Unmarshal(res.Payload, &m); err != nil {
		t.Fatalf("payload not JSON: %s", res.Payload)
	}
	isErr := res.IsError
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("took %s, kill path did not engage", elapsed)
	}
	if !isErr || m["timed_out"] != true {
		t.Fatalf("m=%v isErr=%v", m, isErr)
	}

	// The shell's process group must be gone.
	raw, err := os.ReadFile(filepath.Join(root, "pgid.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	if _, err := fmt.Sscanf(string(raw), "%d", &pid); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(-pid, syscall.Signal(0)); err == nil {
		t.Errorf("process group %d still alive after timeout kill", pid)
	}
}

func TestBashOutputTruncation(t *testing.T) {
	e, _ := newExec(t)
	m, _ := run(t, e, "bash", `{"command":"head -c 100000 /dev/zero | tr '\\0' 'x'"}`)
	out := m["stdout"].(string)
	if !strings.Contains(out, "... truncated") {
		t.Errorf("truncation marker missing (len=%d)", len(out))
	}
	if len(out) > bashOutputBytes+100 {
		t.Errorf("output too long: %d", len(out))
	}
}

func TestUnknownTool(t *testing.T) {
	e, _ := newExec(t)
	if _, isErr := run(t, e, "teleport", `{}`); !isErr {
		t.Error("unknown tool should be an error result")
	}
}

// Context cancellation (the Esc/interrupt path) must kill the process group
// promptly and render as canceled — not as a fabricated timeout.
func TestBashContextCancelKillsGroup(t *testing.T) {
	e, root := newExec(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	res := e.Execute(ctx, "bash", json.RawMessage(
		fmt.Sprintf(`{"command":"echo $$ > %s/pgid.txt; sleep 30"}`, root)))
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("took %s, cancel path did not engage", elapsed)
	}

	var m map[string]any
	if err := json.Unmarshal(res.Payload, &m); err != nil {
		t.Fatal(err)
	}
	if !res.IsError || m["canceled"] != true || m["timed_out"] == true {
		t.Fatalf("m=%v isErr=%v, want canceled without timed_out", m, res.IsError)
	}

	raw, err := os.ReadFile(filepath.Join(root, "pgid.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	if _, err := fmt.Sscanf(string(raw), "%d", &pid); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(-pid, syscall.Signal(0)); err == nil {
		t.Errorf("process group %d still alive after cancel", pid)
	}
}

// findSessionProcs scans /proc for live processes carrying the session
// env marker — targeted lookup, not a global ps pattern match.
func findSessionProcs(t *testing.T, session string) []string {
	t.Helper()
	needle := SessionEnvVar + "=" + session
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatal(err)
	}
	var pids []string
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		environ, err := os.ReadFile(filepath.Join("/proc", e.Name(), "environ"))
		if err != nil {
			continue // gone or not ours
		}
		for _, kv := range strings.Split(string(environ), "\x00") {
			if kv == needle {
				pids = append(pids, e.Name())
				break
			}
		}
	}
	return pids
}

// 2.12 orphan assertion: after a cancel kills the group, no process tagged
// with the session marker survives — including background grandchildren.
func TestBashCancelLeavesNoSessionOrphans(t *testing.T) {
	e, _ := newExec(t)
	e.Session = "orphan-test-" + fmt.Sprint(os.Getpid())

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	res := e.Execute(ctx, "bash", json.RawMessage(
		`{"command":"sleep 60 & sleep 60 & sleep 60"}`))
	if !res.IsError {
		t.Fatalf("result = %s", res.Payload)
	}

	if pids := findSessionProcs(t, e.Session); len(pids) != 0 {
		t.Fatalf("session orphans survived cancel: pids %v", pids)
	}
}

// The marker reaches spawned processes.
func TestBashSessionMarkerSet(t *testing.T) {
	e, _ := newExec(t)
	e.Session = "marker-test"
	m, isErr := run(t, e, "bash", `{"command":"echo -n $AGENTRUNNER_SESSION"}`)
	if isErr || m["stdout"] != "marker-test" {
		t.Fatalf("m=%v isErr=%v", m, isErr)
	}
}

// semantic_search (S7 模块 4): the executor lazily builds the derived
// index and returns ranked hits; snippets are redacted like every output.
func TestSemanticSearch(t *testing.T) {
	e, root := newExec(t)
	if err := os.WriteFile(filepath.Join(root, "auth.go"),
		[]byte("package auth\nfunc CheckToken(t string) bool { return t != \"\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.txt"),
		[]byte("nothing relevant\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, isErr := run(t, e, "semantic_search", `{"query":"token check"}`)
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	hits, ok := out["hits"].([]any)
	if !ok || len(hits) != 1 {
		t.Fatalf("hits = %#v, want one", out["hits"])
	}
	hit := hits[0].(map[string]any)
	if hit["path"] != "auth.go" || !strings.Contains(hit["snippet"].(string), "CheckToken") {
		t.Errorf("hit = %#v", hit)
	}
	if out["indexed_files"].(float64) != 2 {
		t.Errorf("indexed_files = %v", out["indexed_files"])
	}

	// The index refreshes incrementally within one executor lifetime.
	future := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(filepath.Join(root, "auth.go"),
		[]byte("package auth\n// the old logic moved away\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "auth.go"), future, future); err != nil {
		t.Fatal(err)
	}
	out, _ = run(t, e, "semantic_search", `{"query":"CheckToken"}`)
	if hits := out["hits"].([]any); len(hits) != 0 {
		t.Errorf("stale hits = %#v", hits)
	}

	if out, isErr := run(t, e, "semantic_search", `{"query":""}`); !isErr {
		t.Errorf("empty query accepted: %v", out)
	}
}

// Network containment (S7 模块 5): a contained executor runs bash in a
// fresh netns — only loopback is visible; the ratchet never widens; a
// host that cannot contain FAILS CLOSED instead of running with egress.
func TestBashNetworkContainment(t *testing.T) {
	e, _ := newExec(t)
	if err := e.netNSAvailable(); err != nil {
		t.Skipf("no unprivileged netns here: %v", err)
	}
	e.ContainNetwork()
	if !e.NetworkContained() {
		t.Fatal("ratchet did not hold")
	}
	out, isErr := run(t, e, "bash", `{"command":"tail -n +3 /proc/net/dev | cut -d: -f1 | tr -d ' '"}`)
	if isErr {
		t.Fatalf("contained bash failed: %v", out)
	}
	if got := strings.TrimSpace(out["stdout"].(string)); got != "lo" {
		t.Errorf("interfaces inside netns = %q, want only lo", got)
	}
}

func TestBashFailsClosedWithoutNetNS(t *testing.T) {
	e, _ := newExec(t)
	e.ProbeNetNS = func() error { return errors.New("namespaces disabled") }
	e.ContainNetwork()
	out, isErr := run(t, e, "bash", `{"command":"echo should not run"}`)
	if !isErr {
		t.Fatalf("bash ran despite uncontainable host: %v", out)
	}
	if msg := out["error"].(string); !strings.Contains(msg, "refusing to run") {
		t.Errorf("error = %q", msg)
	}
}
