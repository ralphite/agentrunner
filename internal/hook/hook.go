// Package hook runs user-configured shell hooks (3.8). Pre-tool hooks can
// observe or BLOCK an effect (exit 2, stderr becomes the model-visible
// reason); post-tool hooks observe results and may attach a note to the
// activity's completion fact. Hooks are external processes — a real
// wall-clock timeout applies (this package is outside the forbidigo zone;
// exemption recorded in PROGRESS).
package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/redact"
)

// DefaultTimeout bounds each hook process.
const DefaultTimeout = 10 * time.Second

// BlockExitCode is the contract: a pre hook exiting 2 blocks the effect.
const BlockExitCode = 2

// Runner executes the configured hook commands.
type Runner struct {
	PreTool  []string
	PostTool []string
	Dir      string        // working directory (workspace root)
	Timeout  time.Duration // 0 = DefaultTimeout
}

// PreInput is the JSON a pre-tool hook receives on stdin.
type PreInput struct {
	ToolName string          `json:"tool_name"`
	Class    string          `json:"class"`
	Args     json.RawMessage `json:"args,omitempty"`
	CallID   string          `json:"call_id,omitempty"`
}

// PostInput is the JSON a post-tool hook receives on stdin.
type PostInput struct {
	ToolName string          `json:"tool_name"`
	CallID   string          `json:"call_id,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
	IsError  bool            `json:"is_error,omitempty"`
}

// PreResult aggregates the pre-hook chain's outcome.
type PreResult struct {
	Blocked bool
	Reason  string   // blocking hook's stderr
	Notes   []string // warnings from hooks that errored (observe + warn)
}

// RunPre executes every pre hook in order; the first block wins.
func (r *Runner) RunPre(ctx context.Context, in PreInput) PreResult {
	payload, _ := json.Marshal(in)
	var out PreResult
	for _, cmd := range r.PreTool {
		exit, _, stderr, err := r.runOne(ctx, cmd, payload)
		switch {
		case err != nil:
			out.Notes = append(out.Notes, fmt.Sprintf("pre hook %q error: %v", cmd, err))
		case exit == BlockExitCode:
			out.Blocked = true
			out.Reason = strings.TrimSpace(stderr)
			if out.Reason == "" {
				out.Reason = fmt.Sprintf("blocked by pre hook %q", cmd)
			}
			return out
		case exit != 0:
			// Not the block code: observe, warn, continue (a broken hook
			// must not silently veto work).
			out.Notes = append(out.Notes, fmt.Sprintf("pre hook %q exit %d (ignored)", cmd, exit))
		}
	}
	return out
}

// RunPost executes every post hook; their stdout lines become notes.
func (r *Runner) RunPost(ctx context.Context, in PostInput) []string {
	payload, _ := json.Marshal(in)
	var notes []string
	for _, cmd := range r.PostTool {
		exit, stdout, _, err := r.runOne(ctx, cmd, payload)
		if err != nil {
			notes = append(notes, fmt.Sprintf("post hook %q error: %v", cmd, err))
			continue
		}
		if exit != 0 {
			notes = append(notes, fmt.Sprintf("post hook %q exit %d", cmd, exit))
			continue
		}
		if s := strings.TrimSpace(stdout); s != "" {
			notes = append(notes, s)
		}
	}
	return notes
}

// scrubbedEnv is the parent environment minus credential variables.
func scrubbedEnv() []string {
	out := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		secret := false
		for _, suffix := range redact.Suffixes {
			if strings.HasSuffix(k, suffix) {
				secret = true
				break
			}
		}
		if !secret {
			out = append(out, kv)
		}
	}
	return out
}

// runOne executes a single hook command with the JSON payload on stdin.
func (r *Runner) runOne(ctx context.Context, command string, stdin []byte) (exit int, stdout, stderr string, err error) {
	timeout := r.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(hctx, "sh", "-c", command)
	cmd.Dir = r.Dir
	cmd.Stdin = bytes.NewReader(stdin)
	// Strip harness credentials from the hook's environment: a lint/audit
	// hook has no need for GEMINI_API_KEY etc., and the journal never sees
	// them either — the hook must not be a cleartext side channel.
	cmd.Env = scrubbedEnv()
	// Own process group so a forking hook's children die with it, and a
	// killed hook's grandchildren don't hold the output pipes hostage.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Start(); err != nil {
		return -1, "", "", err
	}
	pgid := cmd.Process.Pid
	runErr := cmd.Wait()
	if hctx.Err() == context.DeadlineExceeded {
		_ = syscall.Kill(-pgid, syscall.SIGKILL) // reap the whole group
		return -1, outBuf.String(), errBuf.String(), fmt.Errorf("timed out after %s", timeout)
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		return exitErr.ExitCode(), outBuf.String(), errBuf.String(), nil
	}
	if runErr != nil {
		return -1, outBuf.String(), errBuf.String(), runErr
	}
	return 0, outBuf.String(), errBuf.String(), nil
}

// Gate adapts the pre-hook chain into the effect pipeline. It declares
// side effects, so a crash inside its window is in-doubt (3.2).
type Gate struct {
	Runner *Runner
	// Notes receives observe-mode warnings (nil = dropped).
	Notes func(string)
}

func (g *Gate) Name() string { return "hooks" }

func (g *Gate) SideEffecting() bool { return len(g.Runner.PreTool) > 0 }

func (g *Gate) Check(ctx context.Context, eff pipeline.Effect) pipeline.Decision {
	if eff.Kind != "tool_call" || len(g.Runner.PreTool) == 0 {
		return pipeline.Allow
	}
	res := g.Runner.RunPre(ctx, PreInput{
		ToolName: eff.ToolName, Class: eff.Class,
		Args: eff.Args, CallID: eff.CallID,
	})
	for _, n := range res.Notes {
		if g.Notes != nil {
			g.Notes(n)
		}
	}
	if res.Blocked {
		return pipeline.Deny(res.Reason)
	}
	return pipeline.Allow
}
