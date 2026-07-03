package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/ralphite/agentrunner/internal/workspace"
)

// S1 defaults pack limits.
const (
	readMaxLines     = 2000
	readMaxBytes     = 50 * 1024
	bashOutputBytes  = 30 * 1024 // combined budget: split across stdout+stderr
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
		content = trimToValidUTF8(content[:readMaxBytes])
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
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return errResult("edit_file: %v", err)
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				return errResult("edit_file: %s exists; empty \"old\" creates new files only", args.Path)
			}
			return errResult("edit_file: %v", err)
		}
		_, werr := f.WriteString(args.New)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return errResult("edit_file: %v", werr)
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

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	timedOut, canceled := false, false
	select {
	case <-done:
	case <-timer.C:
		// The command may have finished in the same instant; prefer done.
		select {
		case <-done:
		default:
			timedOut = true
			killGroup(pgid)
			<-done
		}
	case <-ctx.Done():
		canceled = true
		killGroup(pgid)
		<-done
	}

	out := map[string]any{
		"stdout":    truncateHeadTail(stdout.String(), bashOutputBytes/2),
		"stderr":    truncateHeadTail(stderr.String(), bashOutputBytes/2),
		"exit_code": cmd.ProcessState.ExitCode(),
	}
	switch {
	case timedOut:
		out["timed_out"] = true
		out["error"] = fmt.Sprintf("command killed after timeout %s", timeout)
	case canceled:
		out["canceled"] = true
		out["error"] = "command canceled"
	}
	payload, _ := json.Marshal(out)
	return Result{Payload: payload, IsError: timedOut || canceled || cmd.ProcessState.ExitCode() != 0}
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
			// Signal 0 probes for existence; only ESRCH means the group is
			// gone (EPERM = alive but unsignalable — keep escalating).
			if err := syscall.Kill(-pgid, syscall.Signal(0)); errors.Is(err, syscall.ESRCH) {
				return
			}
		}
	}
}

func truncateHeadTail(s string, budget int) string {
	if len(s) <= budget {
		return s
	}
	half := budget / 2
	head := trimToValidUTF8(s[:half])
	tail := s[len(s)-half:]
	for len(tail) > 0 && !utf8.ValidString(tail) {
		tail = tail[1:]
	}
	return head +
		fmt.Sprintf("\n[... truncated %d bytes ...]\n", len(s)-budget) +
		tail
}

// trimToValidUTF8 drops at most 3 trailing bytes to avoid a torn rune.
func trimToValidUTF8(s string) string {
	for i := 0; i < 4 && len(s) > 0 && !utf8.ValidString(s); i++ {
		s = s[:len(s)-1]
	}
	return s
}
