package tool

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ralphite/agentrunner/internal/index"
)

// SandboxInfo is the OS boundary actually available for bash/verifier
// subprocesses. Filesystem containment is mandatory; network is ratcheted by
// the agent spec. The zero value is never an executable sandbox.
type SandboxInfo struct {
	Filesystem string
	Network    string
	Backend    string
}

type sandboxProbe struct {
	info SandboxInfo
	err  error
}

type sandboxDeny struct {
	Path    string
	Subpath bool
}

// SandboxInfo probes the platform backend. The result is cached separately
// for network-open and network-none because a later child may tighten the
// shared executor's ratchet.
func (e *Executor) SandboxInfo() (SandboxInfo, error) {
	if e == nil || e.WS == nil {
		return SandboxInfo{}, fmt.Errorf("filesystem sandbox requires a workspace")
	}
	networkNone := e.NetworkContained()
	e.sandboxMu.Lock()
	defer e.sandboxMu.Unlock()
	if e.sandboxProbes == nil {
		e.sandboxProbes = map[bool]sandboxProbe{}
	}
	if cached, ok := e.sandboxProbes[networkNone]; ok {
		return cached.info, cached.err
	}
	backend, err := platformSandboxProbe(networkNone)
	if err == nil && e.ProbeSandbox != nil {
		err = e.ProbeSandbox(networkNone)
	}
	info := SandboxInfo{Filesystem: "workspace", Network: "all", Backend: backend}
	if networkNone {
		info.Network = "none"
	}
	if err != nil {
		info = SandboxInfo{}
	}
	e.sandboxProbes[networkNone] = sandboxProbe{info: info, err: err}
	return info, err
}

// DoctorSandbox probes the platform OS sandbox backend for both network
// modes without needing a workspace-backed Executor. It powers `ar doctor`
// (INC-75): the containment gates stay fail-closed (决策 #34) — this is the
// preflight that tells an operator, before any agent runs, whether bash and
// command tools will execute in this environment and how to fix it when
// they won't.
func DoctorSandbox() (backend string, openErr, restrictedErr error) {
	backend, openErr = platformSandboxProbe(false)
	_, restrictedErr = platformSandboxProbe(true)
	return backend, openErr, restrictedErr
}

// sandboxedBash constructs the mandatory OS-contained command and an isolated
// HOME/TMP. Capability absence fails before any user command starts. The
// third return lists credential env var names withheld from the child.
func (e *Executor) sandboxedBash(command string) (*exec.Cmd, func(), []string, error) {
	info, err := e.SandboxInfo()
	if err != nil {
		return nil, func() {}, nil, err
	}
	root, err := filepath.EvalSymlinks(e.WS.Root())
	if err != nil {
		return nil, func() {}, nil, fmt.Errorf("resolve workspace: %w", err)
	}
	tmp, err := os.MkdirTemp("", "agentrunner-sandbox-")
	if err != nil {
		return nil, func() {}, nil, fmt.Errorf("create sandbox temp: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	resolvedTmp, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		cleanup()
		return nil, func() {}, nil, fmt.Errorf("resolve sandbox temp: %w", err)
	}
	writable := []string{root, resolvedTmp}
	writable = append(writable, gitMetadataPaths(root)...)
	denied := credentialPaths(root)
	cmd, err := platformSandboxCommand(root, command, writable, denied, info.Network == "none")
	if err != nil {
		cleanup()
		return nil, func() {}, nil, err
	}
	env, withheld := sandboxEnvironment(resolvedTmp, e.Session, e.EnvPassthrough())
	cmd.Env = env
	return cmd, cleanup, withheld, nil
}

// sandboxEnvironment is the parent environment with HOME/TMP isolated to the
// sandbox temp and credential variables withheld — unless the root spec's
// sandbox.env_passthrough names them (audit-0718 P0-2). It also returns the
// NAMES withheld so the tool result can say so instead of the command
// failing mysteriously (P0-3); names are not secrets, values are.
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

func credentialPaths(root string) []sandboxDeny {
	var denied []sandboxDeny
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() && (name == ".ssh" || name == ".aws") {
			denied = append(denied, sandboxDeny{Path: path, Subpath: true})
			return fs.SkipDir
		}
		if !d.IsDir() && index.SkipFile(name) {
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
