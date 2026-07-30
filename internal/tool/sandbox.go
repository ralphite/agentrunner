package tool

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ralphite/agentrunner/internal/index"
)

// SandboxInfo is the OS boundary actually in force for bash/verifier
// subprocesses. BOTH dimensions are opt-in ratchets (决策 #34 修订): the
// default is terminal parity — Filesystem "host", Network "all", Backend
// "none" — and any spec in the tree asking for sandbox.filesystem=workspace
// or sandbox.network=none tightens it for everyone. The zero value is never
// an executable sandbox.
type SandboxInfo struct {
	Filesystem string
	Network    string
	Backend    string
}

// hostBackend is the Backend stamp when nothing wraps the child. It is honest
// evidence that the boundary is the operator's own machine — not a claim that
// containment happened (边界诚实, same clause as the bash containment stamp).
const hostBackend = "none"

// sandboxKey caches one probe per ratchet combination: a later child may
// tighten either dimension on the shared executor.
type sandboxKey struct{ fsNone, netNone bool }

type sandboxProbe struct {
	info SandboxInfo
	err  error
}

type sandboxDeny struct {
	Path    string
	Subpath bool
}

// sandboxPlan is the child command the platform backend must build. HostFS
// leaves the filesystem unconfined while still letting the network ratchet
// wrap the child: the two dimensions are independent, so `network: none` no
// longer drags credential isolation along with it.
type sandboxPlan struct {
	Root        string
	Command     string
	Writable    []string
	Denied      []sandboxDeny
	NetworkNone bool
	HostFS      bool
}

// SandboxInfo reports the boundary in force, probing the platform backend
// only when a ratchet actually asks for one. Terminal parity needs no
// backend, so the default path can never fail closed — an operator's shell
// does not depend on sandbox-exec/bwrap being installed.
func (e *Executor) SandboxInfo() (SandboxInfo, error) {
	if e == nil || e.WS == nil {
		return SandboxInfo{}, fmt.Errorf("bash requires a workspace")
	}
	key := sandboxKey{fsNone: e.FilesystemContained(), netNone: e.NetworkContained()}
	if !key.fsNone && !key.netNone {
		return SandboxInfo{Filesystem: "host", Network: "all", Backend: hostBackend}, nil
	}
	e.sandboxMu.Lock()
	defer e.sandboxMu.Unlock()
	if e.sandboxProbes == nil {
		e.sandboxProbes = map[sandboxKey]sandboxProbe{}
	}
	if cached, ok := e.sandboxProbes[key]; ok {
		return cached.info, cached.err
	}
	backend, err := platformSandboxProbe(key.netNone)
	if err == nil && e.ProbeSandbox != nil {
		err = e.ProbeSandbox(key.netNone)
	}
	info := SandboxInfo{Filesystem: "host", Network: "all", Backend: backend}
	if key.fsNone {
		info.Filesystem = "workspace"
	}
	if key.netNone {
		info.Network = "none"
	}
	if err != nil {
		info = SandboxInfo{}
	}
	e.sandboxProbes[key] = sandboxProbe{info: info, err: err}
	return info, err
}

// DoctorSandbox probes the platform OS sandbox backend for both network
// modes without needing a workspace-backed Executor. It powers `ar doctor`
// (INC-75). Since 决策 #34 修订 the boundary is opt-in, so a failing probe no
// longer blocks bash — it tells the operator which of sandbox.filesystem=
// workspace / sandbox.network=none this machine can actually honor, and how
// to fix it when it can't.
func DoctorSandbox() (backend string, openErr, restrictedErr error) {
	backend, openErr = platformSandboxProbe(false)
	_, restrictedErr = platformSandboxProbe(true)
	return backend, openErr, restrictedErr
}

// sandboxedBash builds the child command for a bash or command-tool effect.
//
// The DEFAULT is terminal parity (决策 #34 修订): an unwrapped shell whose cwd
// is the workspace root, carrying the operator's full environment and real
// HOME — so `gh`, `git`, `gcloud`, ssh-agent and every other on-disk or
// keychain credential works exactly as in a terminal window the user opened
// themselves. An agent whose shell cannot see the auth its tools need cannot
// do the work it was asked to do, and no amount of containment purity buys
// that back.
//
// A spec that ratchets sandbox.filesystem=workspace or sandbox.network=none
// gets the OS boundary instead, and capability absence then fails before any
// user command starts. The third return lists credential env var names
// withheld from the child — always empty under terminal parity.
func (e *Executor) sandboxedBash(command string) (*exec.Cmd, func(), []string, error) {
	info, err := e.SandboxInfo()
	if err != nil {
		return nil, func() {}, nil, err
	}
	root, err := filepath.EvalSymlinks(e.WS.Root())
	if err != nil {
		return nil, func() {}, nil, fmt.Errorf("resolve workspace: %w", err)
	}
	if info.Backend == hostBackend {
		cmd := exec.Command("bash", "-c", command)
		cmd.Dir = root
		cmd.Env = hostEnvironment(e.Session)
		return cmd, func() {}, nil, nil
	}
	plan := sandboxPlan{Root: root, Command: command,
		NetworkNone: info.Network == "none",
		HostFS:      info.Filesystem != "workspace",
	}
	env, withheld := hostEnvironment(e.Session), []string(nil)
	cleanup := func() {}
	if !plan.HostFS {
		tmp, err := os.MkdirTemp("", "agentrunner-sandbox-")
		if err != nil {
			return nil, func() {}, nil, fmt.Errorf("create sandbox temp: %w", err)
		}
		cleanup = func() { _ = os.RemoveAll(tmp) }
		resolvedTmp, err := filepath.EvalSymlinks(tmp)
		if err != nil {
			cleanup()
			return nil, func() {}, nil, fmt.Errorf("resolve sandbox temp: %w", err)
		}
		plan.Writable = append([]string{root, resolvedTmp}, gitMetadataPaths(root)...)
		// Paths the user approved reaching outside the workspace (LOG 2026-07-29).
		// Without this, an approved `edit_file` on ~/.zshrc would succeed while the
		// very next `bash` line touching the same file failed at the OS boundary —
		// the same intent, two different answers. The list is rebuilt per command,
		// so a grant made mid-session applies from the next call with no restart.
		// Moot under terminal parity, where nothing needed granting.
		plan.Writable = append(plan.Writable, e.GrantedPaths()...)
		plan.Denied = e.credentialDenies(root)
		env, withheld = sandboxEnvironment(resolvedTmp, e.Session, e.EnvPassthrough())
	}
	cmd, err := platformSandboxCommand(plan)
	if err != nil {
		cleanup()
		return nil, func() {}, nil, err
	}
	cmd.Env = env
	return cmd, cleanup, withheld, nil
}

// hostEnvironment is terminal parity: the operator's environment verbatim,
// real HOME and all, with only the session stamp replaced. No credential
// withholding — see sandboxedBash. What a child PROCESS sees was never the
// same question as what the journal stores: passed-through values are still
// value-redacted on every journaled surface.
func hostEnvironment(session string) []string {
	parent := os.Environ()
	env := make([]string, 0, len(parent)+1)
	for _, kv := range parent {
		if key, _, _ := strings.Cut(kv, "="); key == SessionEnvVar {
			continue
		}
		env = append(env, kv)
	}
	if session != "" {
		env = append(env, SessionEnvVar+"="+session)
	}
	return env
}

// sandboxEnvironment applies to the OPT-IN filesystem=workspace mode only: the
// parent environment with HOME/TMP isolated to the sandbox temp and credential
// variables withheld — unless the root spec's sandbox.env_passthrough names
// them (audit-0718 P0-2). It also returns the NAMES withheld so the tool
// result can say so instead of the command failing mysteriously (P0-3); names
// are not secrets, values are.
func sandboxEnvironment(home, session string, passthrough []string) (env, withheld []string) {
	allow := map[string]bool{}
	for _, name := range passthrough {
		allow[name] = true
	}
	env = make([]string, 0, len(os.Environ())+6)
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		// Sandbox-critical vars are always replaced below — passthrough can
		// never rescue them (spec validation also rejects those names).
		if key == "HOME" || key == "TMPDIR" || key == "TMP" || key == "TEMP" ||
			strings.HasPrefix(key, "XDG_") || key == SessionEnvVar {
			continue
		}
		upper := strings.ToUpper(key)
		secret := false
		for _, suffix := range []string{"_API_KEY", "_TOKEN", "_SECRET"} {
			if strings.HasSuffix(upper, suffix) {
				secret = true
				break
			}
		}
		if secret && !allow[key] {
			withheld = append(withheld, key)
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "HOME="+home, "TMPDIR="+home, "TMP="+home, "TEMP="+home,
		"XDG_CACHE_HOME="+filepath.Join(home, "cache"))
	if session != "" {
		env = append(env, SessionEnvVar+"="+session)
	}
	sort.Strings(withheld)
	return env, withheld
}

// maxCredentialDenies caps the deny list. Every entry becomes one
// `(deny file-read* …)` line inside the profile that platformSandboxCommand
// passes to `sandbox-exec -p` IN ARGV, so an unbounded list is not just slow —
// past ARG_MAX it fails the command outright with E2BIG. Hitting the cap is
// reported, never silent: the sandbox still denies the workspace-wide default,
// so this trims defense-in-depth entries, not the boundary itself.
const maxCredentialDenies = 512

// credentialDenies is the memoized credential deny list for this executor.
func (e *Executor) credentialDenies(root string) []sandboxDeny {
	e.credOnce.Do(func() { e.credPaths = credentialPaths(root) })
	return e.credPaths
}

func credentialPaths(root string) []sandboxDeny {
	var denied []sandboxDeny
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".ssh" || name == ".aws" {
				denied = append(denied, sandboxDeny{Path: path, Subpath: true})
				return fs.SkipDir
			}
			// Prune derived/vendored trees and dotdirs, in lockstep with the
			// index/grep/glob walks. Previously this walk pruned NOTHING but
			// .ssh/.aws — it descended .git and node_modules in full, once per
			// bash call. Nothing credential-shaped lives in a build output that
			// isn't already covered by the patterns below.
			if path != root && index.SkipDir(name) {
				return fs.SkipDir
			}
			return nil
		}
		if index.SkipFile(name) {
			if len(denied) >= maxCredentialDenies {
				slog.Warn("sandbox credential deny list truncated; workspace-wide default still applies",
					"cap", maxCredentialDenies, "root", root)
				return fs.SkipAll
			}
			denied = append(denied, sandboxDeny{Path: path})
		}
		return nil
	})
	return denied
}

// gitMetadataPaths preserves normal git operation for linked worktrees while
// exposing no sibling working tree. A regular in-workspace .git directory
// needs no extra grant.
func gitMetadataPaths(root string) []string {
	raw, err := os.ReadFile(filepath.Join(root, ".git"))
	if err != nil {
		return nil
	}
	line := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(line, "gitdir:") {
		return nil
	}
	dir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	dir, err = filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		return nil
	}
	out := []string{dir}
	if common, err := os.ReadFile(filepath.Join(dir, "commondir")); err == nil {
		c := strings.TrimSpace(string(common))
		if !filepath.IsAbs(c) {
			c = filepath.Join(dir, c)
		}
		if c, err = filepath.EvalSymlinks(filepath.Clean(c)); err == nil {
			out = append(out, c)
		}
	}
	return out
}
