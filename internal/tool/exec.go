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

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// S1 defaults pack limits.
const (
	readMaxLines       = 2000
	readMaxBytes       = 50 * 1024
	bashOutputBytes    = 30 * 1024 // combined budget: split across stdout+stderr
	bashKillGrace      = 5 * time.Second
	bashInterruptGrace = 500 * time.Millisecond
	bashPipeDeadline   = 2 * time.Second
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

// SessionEnvVar marks every process a session spawns, so cleanup
// assertions can find strays by marker instead of grepping global ps.
const SessionEnvVar = "AGENTRUNNER_SESSION"

// Executor runs built-in tools against a workspace. Wall-clock limits are
// NOT owned here (2.11): the activity executor arms a durable timer and
// cancels ctx with cause errs.ErrActivityTimeout; bash only reacts.
type Executor struct {
	WS *workspace.Workspace
	// Session tags spawned processes via SessionEnvVar (2.12).
	Session string
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
	case "schedule_next":
		return e.scheduleNext(args)
	case "finish_series":
		return e.finishSeries(args)
	default:
		return errResult("unknown tool %q", name)
	}
}

// scheduleNext and finishSeries are pure data-definition tools (S6 loop
// mode): the ack is the whole execution — the MEANING is read by the
// IterationDriver from this run's journal when the iteration ends.
func (e *Executor) scheduleNext(rawArgs json.RawMessage) Result {
	var args struct {
		After string `json:"after"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.After == "" {
		return errResult("schedule_next: invalid args: need {\"after\": duration}")
	}
	if _, err := time.ParseDuration(args.After); err != nil {
		return errResult("schedule_next: bad duration %q (want Go form like \"30m\", \"2h\")", args.After)
	}
	return okResult(map[string]any{
		"output": fmt.Sprintf("next iteration requested after %s (the driver clamps and applies it when this iteration ends)", args.After),
	})
}

func (e *Executor) finishSeries(rawArgs json.RawMessage) Result {
	var args struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Reason == "" {
		return errResult("finish_series: invalid args: need {\"reason\": string}")
	}
	return okResult(map[string]any{
		"output": "series completion claimed; a human verifier reviews it when this iteration ends",
	})
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

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("bash", "-c", args.Command)
	cmd.Dir = e.WS.Root()
	if e.Session != "" {
		cmd.Env = append(os.Environ(), SessionEnvVar+"="+e.Session)
	}
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
	reaped := make(chan struct{})
	go func() {
		err := cmd.Wait()
		close(reaped) // leader reaped: its pid is no longer safe to signal
		done <- err
	}()

	timedOut, canceled := false, false
	select {
	case <-done:
	case <-ctx.Done():
		// The command may have finished in the same instant; prefer done —
		// a completed command must not be journaled as canceled.
		select {
		case <-done:
		default:
			// The durable timer cancels with cause ErrActivityTimeout;
			// render that as a timeout, anything else as user cancellation.
			// A steering interrupt gets a much shorter kill grace than a
			// timeout — interactive cancellation must feel instant.
			grace := bashKillGrace
			if errors.Is(context.Cause(ctx), errs.ErrActivityTimeout) {
				timedOut = true
			} else {
				canceled = true
				if errors.Is(context.Cause(ctx), errs.ErrUserInterrupt) {
					grace = bashInterruptGrace
				}
			}
			killGroup(pgid, reaped, grace)
			<-done
		}
	}

	out := map[string]any{
		"stdout":    truncateHeadTail(stdout.String(), bashOutputBytes/2),
		"stderr":    truncateHeadTail(stderr.String(), bashOutputBytes/2),
		"exit_code": cmd.ProcessState.ExitCode(),
	}
	switch {
	case timedOut:
		out["timed_out"] = true
		out["error"] = "command killed after timeout"
	case canceled:
		out["canceled"] = true
		out["error"] = "command canceled"
	}
	payload, _ := json.Marshal(out)
	return Result{Payload: payload, IsError: timedOut || canceled || cmd.ProcessState.ExitCode() != 0}
}

// killGroup terminates the process group: SIGTERM, grace, then SIGKILL.
// Once `reaped` fires, the leader has been waited on and its pid may be
// recycled as an unrelated pgid — signaling it again could kill innocent
// processes, so we stop escalating (TERM-resistant grandchildren that
// outlive the leader escape the KILL; that beats shooting a stranger).
func killGroup(pgid int, reaped <-chan struct{}, grace time.Duration) {
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	deadline := time.After(grace)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			select {
			case <-reaped:
			default:
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			}
			return
		case <-reaped:
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
