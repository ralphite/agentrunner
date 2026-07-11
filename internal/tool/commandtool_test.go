package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// RunCommandTool (INC-55) runs a fixed command in the OS sandbox with the
// model's args delivered as JSON on stdin. `cat` echoes stdin, proving the
// args reach the process; the exit code is propagated.
func TestRunCommandToolStdin(t *testing.T) {
	e, _ := newExec(t)
	if _, err := e.SandboxInfo(); err != nil {
		t.Skipf("no OS sandbox backend here: %v", err)
	}
	args := json.RawMessage(`{"target":"prod","force":true}`)
	res := e.RunCommandTool(context.Background(), "cat", args)
	if res.IsError {
		t.Fatalf("cat errored: %s", res.Payload)
	}
	var m map[string]any
	if err := json.Unmarshal(res.Payload, &m); err != nil {
		t.Fatalf("payload not JSON: %s", res.Payload)
	}
	if got := m["stdout"].(string); !strings.Contains(got, `"target":"prod"`) {
		t.Errorf("stdin args did not reach the command: stdout=%q", got)
	}
	if m["exit_code"].(float64) != 0 {
		t.Errorf("exit_code = %v", m["exit_code"])
	}
}

// An empty/absent args object still delivers a valid JSON object on stdin.
func TestRunCommandToolEmptyArgs(t *testing.T) {
	e, _ := newExec(t)
	if _, err := e.SandboxInfo(); err != nil {
		t.Skipf("no OS sandbox backend here: %v", err)
	}
	res := e.RunCommandTool(context.Background(), "cat", nil)
	if res.IsError {
		t.Fatalf("errored: %s", res.Payload)
	}
	var m map[string]any
	_ = json.Unmarshal(res.Payload, &m)
	if strings.TrimSpace(m["stdout"].(string)) != "{}" {
		t.Errorf("empty args should deliver {} on stdin, got %q", m["stdout"])
	}
}

// A nonzero exit renders IsError with the code, like bash.
func TestRunCommandToolExitCode(t *testing.T) {
	e, _ := newExec(t)
	if _, err := e.SandboxInfo(); err != nil {
		t.Skipf("no OS sandbox backend here: %v", err)
	}
	res := e.RunCommandTool(context.Background(), "exit 5", nil)
	if !res.IsError {
		t.Fatalf("nonzero exit should be an error result: %s", res.Payload)
	}
	var m map[string]any
	_ = json.Unmarshal(res.Payload, &m)
	if m["exit_code"].(float64) != 5 {
		t.Errorf("exit_code = %v", m["exit_code"])
	}
}

// Fail closed: with no OS sandbox backend, a command tool refuses to run —
// the same hard boundary bash enforces (决策 #34). No user command executes.
func TestRunCommandToolFailsClosedWithoutSandbox(t *testing.T) {
	e, _ := newExec(t)
	e.ProbeSandbox = func(bool) error { return errors.New("sandbox disabled") }
	res := e.RunCommandTool(context.Background(), "echo should-not-run", nil)
	if !res.IsError {
		t.Fatalf("must refuse without a sandbox: %s", res.Payload)
	}
	if !strings.Contains(string(res.Payload), "sandbox unavailable") {
		t.Errorf("unexpected error: %s", res.Payload)
	}
}
