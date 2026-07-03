package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

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
	e.BashTimeout = 300 * time.Millisecond

	// The marker file lets us find the grandchild's pid.
	cmd := fmt.Sprintf(`{"command":"echo $$ > %s/pgid.txt; sleep 30"}`, root)
	start := time.Now()
	m, isErr := run(t, e, "bash", cmd)
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
