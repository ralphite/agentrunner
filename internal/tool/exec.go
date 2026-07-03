package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ralphite/agentrunner/internal/workspace"
)

// S1 defaults pack limits.
const (
	readMaxLines     = 2000
	readMaxBytes     = 50 * 1024
	bashOutputBytes  = 30 * 1024 // head+tail combined
	defaultBashTO    = 120 * time.Second
	bashKillGrace    = 5 * time.Second
	bashPipeDeadline = 2 * time.Second
)

// Result is a tool execution outcome. IsError results render as error
// tool_results for the model (决策 #9); the loop continues either way.
type Result struct {
	Payload json.RawMessage
	IsError bool
}

func errResult(format string, args ...any) Result {
	msg, _ := json.Marshal(map[string]any{"error": fmt.Sprintf(format, args...)})
	return Result{Payload: msg, IsError: true}
}

func okResult(v any) Result {
	payload, _ := json.Marshal(v)
	return Result{Payload: payload}
}

// Executor runs built-in tools against a workspace.
// NOTE(S1): bash timeout uses the wall clock; migrates to the durable-timer
// substrate in S2.11 (declared provisional in PLAN).
type Executor struct {
	WS          *workspace.Workspace
	BashTimeout time.Duration
}

// Execute dispatches one tool call. Unknown tools and malformed args are
// model-visible errors, not harness failures.
func (e *Executor) Execute(ctx context.Context, name string, args json.RawMessage) Result {
	switch name {
	case "read_file":
		return e.readFile(args)
	case "edit_file":
		return e.editFile(args)
	case "bash":
		return e.bash(ctx, args)
	default:
		return errResult("unknown tool %q", name)
	}
}

func (e *Executor) readFile(rawArgs json.RawMessage) Result {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Path == "" {
		return errResult("read_file: invalid args: need {\"path\": string}")
	}
	path, err := e.WS.Resolve(args.Path)
	if err != nil {
		return errResult("read_file: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return errResult("read_file: %v", err)
	}

	content := string(raw)
	truncated := false
	if len(content) > readMaxBytes {
		content = content[:readMaxBytes]
		truncated = true
	}
	if lines := strings.Split(content, "\n"); len(lines) > readMaxLines {
		content = strings.Join(lines[:readMaxLines], "\n")
		truncated = true
	}
	if truncated {
		content += fmt.Sprintf("\n[truncated: file is %d bytes, showing at most %d lines / %d bytes]",
			len(raw), readMaxLines, readMaxBytes)
	}
	return okResult(map[string]any{"content": content, "truncated": truncated})
}

func (e *Executor) editFile(rawArgs json.RawMessage) Result {
	var args struct {
		Path string `json:"path"`
		Old  string `json:"old"`
		New  string `json:"new"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Path == "" {
		return errResult("edit_file: invalid args: need {\"path\", \"old\", \"new\"}")
	}
	path, err := e.WS.Resolve(args.Path)
	if err != nil {
		return errResult("edit_file: %v", err)
	}

	if args.Old == "" {
		if _, err := os.Stat(path); err == nil {
			return errResult("edit_file: %s exists; empty \"old\" creates new files only", args.Path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return errResult("edit_file: %v", err)
		}
		if err := os.WriteFile(path, []byte(args.New), 0o644); err != nil {
			return errResult("edit_file: %v", err)
		}
		return okResult(map[string]any{"output": fmt.Sprintf("created %s", args.Path)})
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return errResult("edit_file: %v", err)
	}
	content := string(raw)
	switch n := strings.Count(content, args.Old); n {
	case 1:
		// fallthrough to replace
	case 0:
		return errResult("edit_file: old string not found in %s (0 matches, need exactly 1)", args.Path)
	default:
		return errResult("edit_file: old string matches %d times in %s, need exactly 1", n, args.Path)
	}
	content = strings.Replace(content, args.Old, args.New, 1)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return errResult("edit_file: %v", err)
	}
	return okResult(map[string]any{"output": fmt.Sprintf("edited %s", args.Path)})
}

func (e *Executor) bash(ctx context.Context, rawArgs json.RawMessage) Result {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Command == "" {
		return errResult("bash: invalid args: need {\"command\": string}")
	}

	timeout := e.BashTimeout
	if timeout == 0 {
		timeout = defaultBashTO
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("bash", "-c", args.Command)
	cmd.Dir = e.WS.Root()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Background children inherit the output pipes; don't let them hold
	// Wait hostage after the direct child exits.
	cmd.WaitDelay = bashPipeDeadline

	if err := cmd.Start(); err != nil {
		return errResult("bash: %v", err)
	}
	pgid := cmd.Process.Pid

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	timedOut := false
	select {
	case <-done:
	case <-time.After(timeout):
		timedOut = true
		killGroup(pgid)
		<-done
	case <-ctx.Done():
		timedOut = true
		killGroup(pgid)
		<-done
	}

	out := map[string]any{
		"stdout":    truncateHeadTail(stdout.String()),
		"stderr":    truncateHeadTail(stderr.String()),
		"exit_code": cmd.ProcessState.ExitCode(),
	}
	if timedOut {
		out["timed_out"] = true
		out["error"] = fmt.Sprintf("command killed after timeout %s", timeout)
	}
	payload, _ := json.Marshal(out)
	return Result{Payload: payload, IsError: timedOut || cmd.ProcessState.ExitCode() != 0}
}

// killGroup terminates the process group: SIGTERM, grace, then SIGKILL.
func killGroup(pgid int) {
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	deadline := time.After(bashKillGrace)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			return
		case <-tick.C:
			// Signal 0 probes for existence; ESRCH means the group is gone.
			if err := syscall.Kill(-pgid, syscall.Signal(0)); err != nil {
				return
			}
		}
	}
}

func truncateHeadTail(s string) string {
	if len(s) <= bashOutputBytes {
		return s
	}
	half := bashOutputBytes / 2
	return s[:half] +
		fmt.Sprintf("\n[... truncated %d bytes ...]\n", len(s)-bashOutputBytes) +
		s[len(s)-half:]
}
