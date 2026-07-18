package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
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
	return runCtx(t, context.Background(), e, name, args)
}

func runCtx(t *testing.T, ctx context.Context, e *Executor, name, args string) (map[string]any, bool) {
	t.Helper()
	res := e.Execute(ctx, name, json.RawMessage(args))
	var m map[string]any
	if err := json.Unmarshal(res.Payload, &m); err != nil {
		t.Fatalf("payload not JSON: %s", res.Payload)
	}
	return m, res.IsError
}

// cancelWhenFileReady waits until the sandboxed shell has definitely
// started before canceling it. A fixed sleep races sandbox startup under a
// loaded host and can cancel before the shell writes the PID marker, which
// proves nothing about process-group cleanup.
func cancelWhenFileReady(path string, cancel func()) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(raw))) > 0 {
			cancel()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Bound the test even if startup itself is broken. The caller's marker
	// assertion will report the more useful failure after Execute returns.
	cancel()
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
	marker := filepath.Join(root, "pgid.txt")

	// 2.11: the wall-clock limit is owned by the durable timer, which
	// cancels ctx with cause ErrActivityTimeout; bash renders timed_out.
	ctx, cancel := context.WithCancelCause(context.Background())
	go func() {
		cancelWhenFileReady(marker, func() { cancel(errs.ErrActivityTimeout) })
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
	raw, err := os.ReadFile(marker)
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
	marker := filepath.Join(root, "pgid.txt")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		cancelWhenFileReady(marker, cancel)
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

	raw, err := os.ReadFile(marker)
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

// findMarkedSleeps pgreps for live processes whose command line carries
// the unique sleep duration — a cross-platform orphan probe (macOS cannot
// read another process's environ; a fractional duration is unique enough).
func findMarkedSleeps(t *testing.T, marker string) []string {
	t.Helper()
	out, err := exec.Command("pgrep", "-f", "sleep "+marker).Output()
	if err != nil {
		// pgrep exits 1 on no match — that IS the clean answer.
		return nil
	}
	return strings.Fields(string(out))
}

// 2.12 orphan assertion: after a cancel kills the group, no process from
// the command survives — including background grandchildren.
func TestBashCancelLeavesNoSessionOrphans(t *testing.T) {
	e, _ := newExec(t)
	e.Session = "orphan-test-" + fmt.Sprint(os.Getpid())
	marker := fmt.Sprintf("6%d.%06d", os.Getpid()%10, time.Now().Nanosecond()%1000000)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	cmd := fmt.Sprintf(`{"command":"sleep %s & sleep %s & sleep %s"}`, marker, marker, marker)
	res := e.Execute(ctx, "bash", json.RawMessage(cmd))
	if !res.IsError {
		t.Fatalf("result = %s", res.Payload)
	}

	if pids := findMarkedSleeps(t, marker); len(pids) != 0 {
		t.Fatalf("orphans survived cancel: pids %v", pids)
	}
}

// A sandbox wrapper/leader can exit on TERM while a grandchild ignores it.
// Cancellation must wait for the process group itself, then escalate, rather
// than treating the reaped leader as proof that the whole group is gone.
func TestBashCancelKillsTermResistantGrandchild(t *testing.T) {
	e, root := newExec(t)
	marker := fmt.Sprintf("7%d.%06d", os.Getpid()%10, time.Now().Nanosecond()%1000000)
	ready := filepath.Join(root, "ready")
	ctx, cancel := context.WithCancelCause(context.Background())
	go cancelWhenFileReady(ready, func() { cancel(errs.ErrUserInterrupt) })

	command := fmt.Sprintf(
		`trap 'exit 0' TERM; (trap '' TERM; exec sleep %s) & echo ready > %s; wait`,
		marker, ready,
	)
	res := e.Execute(ctx, "bash", json.RawMessage(fmt.Sprintf(`{"command":%q}`, command)))
	if !res.IsError {
		t.Fatalf("result = %s, want canceled", res.Payload)
	}

	if pids := findMarkedSleeps(t, marker); len(pids) != 0 {
		t.Fatalf("TERM-resistant grandchildren survived cancel: pids %v", pids)
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

// Network containment is enforced by the platform OS sandbox. Even a local
// listener is unreachable, the ratchet never widens, and capability absence
// fails closed.
func TestBashNetworkContainment(t *testing.T) {
	e, _ := newExec(t)
	e.ContainNetwork()
	if _, err := e.SandboxInfo(); err != nil {
		t.Skipf("no OS sandbox backend here: %v", err)
	}
	if !e.NetworkContained() {
		t.Fatal("ratchet did not hold")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	command := fmt.Sprintf("echo leaked > /dev/tcp/127.0.0.1/%d", ln.Addr().(*net.TCPAddr).Port)
	out, isErr := run(t, e, "bash", fmt.Sprintf(`{"command":%q}`, command))
	if !isErr {
		t.Fatalf("contained bash reached local network: %v", out)
	}
}

func TestBashFailsClosedWithoutOSSandbox(t *testing.T) {
	e, _ := newExec(t)
	e.ProbeSandbox = func(bool) error { return errors.New("sandbox disabled") }
	e.ContainNetwork()
	out, isErr := run(t, e, "bash", `{"command":"echo should not run"}`)
	if !isErr {
		t.Fatalf("bash ran despite uncontainable host: %v", out)
	}
	if msg := out["error"].(string); !strings.Contains(msg, "refusing to run") {
		t.Errorf("error = %q", msg)
	}
}

func TestBashFilesystemSandbox(t *testing.T) {
	e, root := newExec(t)
	t.Setenv("INC11_API_KEY", "ENV-SECRET")
	if _, err := e.SandboxInfo(); err != nil {
		t.Skipf("no OS sandbox backend here: %v", err)
	}
	outside := filepath.Join(filepath.Dir(root), "outside-secret")
	if err := os.WriteFile(outside, []byte("OUTSIDE"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("INSIDE-SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := fmt.Sprintf("cat ok.txt; echo changed > made.txt; cat %q; cat .env; printf -- \"$INC11_API_KEY\"", outside)
	out, isErr := run(t, e, "bash", fmt.Sprintf(`{"command":%q}`, cmd))
	// Denial surfaces per backend: Seatbelt reports EPERM ("Operation not
	// permitted"); bwrap masks paths via tmpfs / /dev/null binds, so reads
	// fail with ENOENT or EACCES instead. The invariant is that the reads
	// are denied — the errno wording is backend-specific.
	stderr, _ := out["stderr"].(string)
	deniedRead := strings.Contains(stderr, "Operation not permitted") ||
		strings.Contains(stderr, "Permission denied") ||
		strings.Contains(stderr, "No such file or directory")
	if !isErr && !deniedRead {
		t.Fatalf("sandbox did not report denied reads: %v", out)
	}
	if !strings.Contains(out["stdout"].(string), "inside") {
		t.Fatalf("workspace read failed: %v", out)
	}
	if raw, err := os.ReadFile(filepath.Join(root, "made.txt")); err != nil || strings.TrimSpace(string(raw)) != "changed" {
		t.Fatalf("workspace write = %q err=%v", raw, err)
	}
	if strings.Contains(out["stdout"].(string), "OUTSIDE") || strings.Contains(out["stdout"].(string), "INSIDE-SECRET") ||
		strings.Contains(out["stdout"].(string), "ENV-SECRET") {
		t.Fatalf("sandbox leaked credential/outside content: %v", out)
	}
}

func TestBashFilesystemSandboxAllowsLinkedWorktreeGitMetadata(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	base := t.TempDir()
	// Host git working does not imply git works under the OS sandbox: on
	// Xcode.app-only machines the /usr/bin/git shim resolves via
	// xcrun+xcodebuild, which the sandbox profile blocks (GAPS G32). Probe
	// git inside the same sandbox with a command that does not depend on
	// the linked-worktree metadata grants under test.
	probe := filepath.Join(base, "probe")
	if err := os.Mkdir(probe, 0o755); err != nil {
		t.Fatal(err)
	}
	pws, err := workspace.New(probe)
	if err != nil {
		t.Fatal(err)
	}
	if out, isErr := run(t, &Executor{WS: pws}, "bash", `{"command":"git --version"}`); isErr ||
		!strings.Contains(out["stdout"].(string), "git version") {
		t.Skipf("git unusable inside the OS sandbox on this host (GAPS G32): %v", out)
	}
	repo := filepath.Join(base, "repo")
	wt := filepath.Join(base, "worktree")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "qa@example.com"},
		{"config", "user.name", "QA"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit := exec.Command("git", "add", "tracked.txt")
	commit.Dir = repo
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	commit = exec.Command("git", "commit", "-qm", "base")
	commit.Dir = repo
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	add := exec.Command("git", "worktree", "add", "-qb", "qa-worktree", wt)
	add.Dir = repo
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v: %s", err, out)
	}
	ws, err := workspace.New(wt)
	if err != nil {
		t.Fatal(err)
	}
	e := &Executor{WS: ws}
	out, isErr := run(t, e, "bash", `{"command":"git status --porcelain && echo git-ok"}`)
	if isErr || !strings.Contains(out["stdout"].(string), "git-ok") {
		t.Fatalf("sandboxed linked-worktree git failed: %v", out)
	}
}

// v2 M4.3: write_file creates (with parents) and overwrites whole files;
// the workspace boundary holds; empty content is valid, missing content is
// a model-visible arg error.
func TestWriteFile(t *testing.T) {
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &Executor{WS: ws}

	res := e.Execute(context.Background(), "write_file",
		json.RawMessage(`{"path":"docs/NOTES.md","content":"第一版\n"}`))
	if res.IsError {
		t.Fatalf("create: %s", res.Payload)
	}
	raw, err := os.ReadFile(ws.Root() + "/docs/NOTES.md")
	if err != nil || string(raw) != "第一版\n" {
		t.Fatalf("content = %q, %v", raw, err)
	}
	res = e.Execute(context.Background(), "write_file",
		json.RawMessage(`{"path":"docs/NOTES.md","content":"第二版"}`))
	if res.IsError {
		t.Fatalf("overwrite: %s", res.Payload)
	}
	raw, _ = os.ReadFile(ws.Root() + "/docs/NOTES.md")
	if string(raw) != "第二版" {
		t.Fatalf("overwrite content = %q", raw)
	}
	res = e.Execute(context.Background(), "write_file",
		json.RawMessage(`{"path":"../escape.txt","content":"x"}`))
	if !res.IsError {
		t.Fatal("workspace escape allowed")
	}
	res = e.Execute(context.Background(), "write_file", json.RawMessage(`{"path":"a.txt"}`))
	if !res.IsError {
		t.Fatal("missing content accepted")
	}
	res = e.Execute(context.Background(), "write_file",
		json.RawMessage(`{"path":"a.txt","content":""}`))
	if res.IsError {
		t.Fatalf("empty content rejected: %s", res.Payload)
	}
}
