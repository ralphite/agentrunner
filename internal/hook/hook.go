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
	"sort"
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
	// Lifecycle maps a lifecycle event name (the Event* constants) to its
	// hook commands (INC-15, G19). Same command-only contract as pre/post.
	Lifecycle map[string][]string
	Dir       string        // working directory (workspace root)
	Timeout   time.Duration // 0 = DefaultTimeout
	// Credential env passthrough (audit-0718 P0-2), sealed first-wins by the
	// root loop before any hook fires — same face as the bash sandbox. Plain
	// fields, no lock: the Runner is struct-copied for children (spawn), and
	// the single seal happens at loop entry before any concurrent use.
	envPassthrough []string
	envSealed      bool
}

// SealEnvPassthrough fixes the credential env vars hooks may see. The first
// seal wins; a child loop re-applying its own spec is a no-op (a copied
// child Runner inherits the parent's seal).
func (r *Runner) SealEnvPassthrough(names []string) {
	if r.envSealed {
		return
	}
	r.envSealed = true
	r.envPassthrough = append([]string(nil), names...)
}

// Lifecycle event names (INC-15, G19 first batch). Each fires at its journal
// point in the loop; only the *blockable* ones honor exit 2.
const (
	EventSessionStart     = "session_start"      // observe — after SessionStarted
	EventSessionEnd       = "session_end"        // observe — after SessionClosed (close/kill)
	EventUserPromptSubmit = "user_prompt_submit" // BLOCKABLE — before InputReceived lands
	EventStop             = "stop"               // observe — at quiescence (turn wrapped up)
	EventSubagentStart    = "subagent_start"     // observe — after SpawnRequested
	EventSubagentStop     = "subagent_stop"      // observe — after SubagentCompleted
	EventPreCompact       = "pre_compact"        // BLOCKABLE — before a compaction runs
	EventPostCompact      = "post_compact"       // observe — after ContextCompacted
)

// LifecycleEvents is the registry of known event names (config validation).
var LifecycleEvents = map[string]bool{
	EventSessionStart: true, EventSessionEnd: true,
	EventUserPromptSubmit: true, EventStop: true,
	EventSubagentStart: true, EventSubagentStop: true,
	EventPreCompact: true, EventPostCompact: true,
}

// LifecycleInput is the JSON a lifecycle hook receives on stdin.
type LifecycleInput struct {
	Event   string          `json:"event"`
	Session string          `json:"session,omitempty"`
	Detail  json.RawMessage `json:"detail,omitempty"`
}

// LifecycleResult aggregates one event's hook chain outcome. Blocked is only
// ever set when the caller declared the event blockable.
type LifecycleResult struct {
	Blocked bool
	Reason  string
	Notes   []string
}

// RunLifecycle executes the hooks registered for one lifecycle event.
// blockable events honor the exit-2 contract (first block wins, stderr is
// the reason — same as RunPre); observe events treat every exit code as
// observe+warn (same as RunPost): a broken hook must never veto work it was
// only meant to watch.
func (r *Runner) RunLifecycle(ctx context.Context, in LifecycleInput, blockable bool) LifecycleResult {
	cmds := r.Lifecycle[in.Event]
	if len(cmds) == 0 {
		return LifecycleResult{}
	}
	payload, _ := json.Marshal(in)
	_, withheld := r.scrubbedEnv()
	var out LifecycleResult
	for _, cmd := range cmds {
		exit, stdout, stderr, err := r.runOne(ctx, cmd, payload)
		switch {
		case err != nil:
			out.Notes = append(out.Notes, fmt.Sprintf("%s hook %q error: %v%s", in.Event, cmd, err, withheldNote(withheld)))
		case blockable && exit == BlockExitCode:
			out.Blocked = true
			out.Reason = strings.TrimSpace(stderr)
			if out.Reason == "" {
				out.Reason = fmt.Sprintf("blocked by %s hook %q", in.Event, cmd)
			}
			return out
		case exit != 0:
			out.Notes = append(out.Notes, fmt.Sprintf("%s hook %q exit %d (ignored)", in.Event, cmd, exit))
		default:
			if s := strings.TrimSpace(stdout); s != "" {
				out.Notes = append(out.Notes, s)
			}
		}
	}
	return out
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
	_, withheld := r.scrubbedEnv()
	var out PreResult
	for _, cmd := range r.PreTool {
		exit, _, stderr, err := r.runOne(ctx, cmd, payload)
		switch {
		case err != nil:
			out.Notes = append(out.Notes, fmt.Sprintf("pre hook %q error: %v%s", cmd, err, withheldNote(withheld)))
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
	_, withheld := r.scrubbedEnv()
	var notes []string
	for _, cmd := range r.PostTool {
		exit, stdout, _, err := r.runOne(ctx, cmd, payload)
		if err != nil {
			notes = append(notes, fmt.Sprintf("post hook %q error: %v%s", cmd, err, withheldNote(withheld)))
			continue
		}
		if exit != 0 {
			notes = append(notes, fmt.Sprintf("post hook %q exit %d%s", cmd, exit, withheldNote(withheld)))
			continue
		}
		if s := strings.TrimSpace(stdout); s != "" {
			notes = append(notes, s)
		}
	}
	return notes
}

// scrubbedEnv is the parent environment minus credential variables — unless
// the root spec's sandbox.env_passthrough names them (audit-0718 P0-2). It
// also returns the NAMES withheld so a failing hook can say so instead of
// dying mysteriously (P0-3).
func (r *Runner) scrubbedEnv() (env, withheld []string) {
	allow := map[string]bool{}
	for _, name := range r.envPassthrough {
		allow[name] = true
	}
	env = make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		secret := false
		for _, suffix := range redact.Suffixes {
			if strings.HasSuffix(k, suffix) {
				secret = true
				break
			}
		}
		if secret && !allow[k] {
			withheld = append(withheld, k)
			continue
		}
		env = append(env, kv)
	}
	sort.Strings(withheld)
	return env, withheld
}

// withheldNote renders the explicit-withholding suffix appended to a failing
// hook's note: names only, never values.
func withheldNote(withheld []string) string {
	if len(withheld) == 0 {
		return ""
	}
	return fmt.Sprintf(" (%d credential env vars withheld from hooks: %s — list needed ones in spec sandbox.env_passthrough)",
		len(withheld), strings.Join(withheld, ", "))
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
	// them either — the hook must not be a cleartext side channel. The root
	// spec's sandbox.env_passthrough names the exceptions.
	env, _ := r.scrubbedEnv()
	cmd.Env = env
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
