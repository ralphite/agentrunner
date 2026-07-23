// Package snapshot is the SnapshotStore seam (S7 模块 1, DESIGN L1): events
// reference only OPAQUE snapshot refs, so no upper layer couples to the
// mechanism. Snapshot materialization serves rewind/fork; opaque refs may
// also be compared read-only for review surfaces. Snapshots are taken only
// at explicit barriers (module 2), and stay pinned until explicit GC.
//
// The default backend is a SHADOW REPO: a separate GIT_DIR in the harness
// data directory, invisible to the user's repo AND to the agent's own git
// commands (which see only the workspace's .git). backend=none degrades
// gracefully — rewind/fork become unavailable, nothing else is affected;
// a missing git binary degrades the same way.
package snapshot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// ErrUnavailable marks a store that cannot snapshot (backend=none, git
// missing). Callers record barriers WITHOUT refs — fork/rewind of those
// barriers is then refused, gracefully.
var ErrUnavailable = errors.New("snapshot backend unavailable")

// DiffResult is a read-only comparison from an opaque snapshot ref to the
// workspace as it exists when Diff is called.
type DiffResult struct {
	Diff             string            `json:"diff"`
	Numstat          string            `json:"numstat"`
	Untracked        []string          `json:"untracked"`
	UntrackedReasons map[string]string `json:"untracked_reasons"`
	HiddenUntracked  int               `json:"hidden_untracked"`
}

// Store captures and reconstructs workspace states.
type Store interface {
	// Snapshot captures the workspace now and returns an opaque ref. Two
	// snapshots of an identical tree may return the SAME ref (dedup).
	Snapshot(ctx context.Context) (string, error)
	// Materialize reconstructs ref's file tree into dir (created if needed,
	// must be empty) — forks never share directories with the original.
	Materialize(ctx context.Context, ref, dir string) error
	// Diff compares ref with the current workspace without mutating either.
	// Backends that cannot provide a safe comparison return ErrUnavailable.
	Diff(ctx context.Context, ref string) (DiffResult, error)
}

// None is the degraded backend.
type None struct{}

func (None) Snapshot(context.Context) (string, error) { return "", ErrUnavailable }
func (None) Materialize(context.Context, string, string) error {
	return ErrUnavailable
}
func (None) Diff(context.Context, string) (DiffResult, error) {
	return DiffResult{}, ErrUnavailable
}

// hardExcludes are paths NEVER snapshotted (DESIGN: 凭据路径显式排除出
// 快照/rewind 范围 — a rewind must not resurrect deleted credentials).
// Written to the shadow repo's info/exclude; .gitignore semantics.
var hardExcludes = []string{
	".env",
	".env.*",
	".envrc",
	"*.pem",
	"*.key",
	"id_rsa*",
	"id_ed25519*",
	".git-credentials",
	".netrc",
	".npmrc",
	".pypirc",
	"credentials.json",
	// Unanchored (**): a nested subproject's credentials are just as much
	// credentials (S7 出口 review — the bare pattern anchored to the root).
	"**/.aws/credentials",
	".ssh/",
}

// ShadowRepo snapshots via a separate GIT_DIR. The workspace's own .git is
// never tracked (git refuses paths containing a .git component), so the
// user's repo and the agent's git operations stay invisible in both
// directions. Embedded repos deeper in the tree (vendor/x/.git) degrade to
// gitlinks and are NOT materialized — documented limit.
type ShadowRepo struct {
	gitDir string
	work   string
}

// Open builds the default store for a workspace: a shadow repo under
// dataDir, or None (with ErrUnavailable at use) when git is missing.
func Open(dataDir, workspaceRoot string) (Store, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return None{}, nil
	}
	gitDir := filepath.Join(dataDir, "shadow.git")
	s := &ShadowRepo{gitDir: gitDir, work: workspaceRoot}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

// NewShadowRepo opens a shadow repo at an explicit GIT_DIR (tests).
func NewShadowRepo(gitDir, workspaceRoot string) (*ShadowRepo, error) {
	s := &ShadowRepo{gitDir: gitDir, work: workspaceRoot}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *ShadowRepo) init() error {
	return withRepoLock(context.Background(), s.gitDir, func() error {
		if _, err := os.Stat(filepath.Join(s.gitDir, "HEAD")); err == nil {
			return s.writeExcludes() // already initialized; refresh excludes
		}
		// A bare init takes no --work-tree; run it raw.
		cmd := exec.Command("git", "init", "--bare", "-q", s.gitDir)
		var errb bytes.Buffer
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("snapshot: git init: %v: %s", err, strings.TrimSpace(errb.String()))
		}
		return s.writeExcludes()
	})
}

func withRepoLock(ctx context.Context, gitDir string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(gitDir), 0o700); err != nil {
		return fmt.Errorf("snapshot: create lock directory: %w", err)
	}
	f, err := os.OpenFile(gitDir+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("snapshot: open repository lock: %w", err)
	}
	defer func() { _ = f.Close() }()
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			return fmt.Errorf("snapshot: acquire repository lock: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	return fn()
}

func (s *ShadowRepo) writeExcludes() error {
	info := filepath.Join(s.gitDir, "info")
	if err := os.MkdirAll(info, 0o700); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	content := "# agentrunner hard excludes — credentials never enter snapshots\n" +
		strings.Join(hardExcludes, "\n") + "\n"
	return os.WriteFile(filepath.Join(info, "exclude"), []byte(content), 0o600)
}

// git runs one git command against the shadow GIT_DIR with a pinned
// identity and no global/user config interference.
func (s *ShadowRepo) git(ctx context.Context, args ...string) (string, error) {
	return s.gitWithEnv(ctx, nil, args...)
}

func (s *ShadowRepo) gitWithEnv(ctx context.Context, extraEnv []string, args ...string) (string, error) {
	out, err := s.gitRawWithEnv(ctx, extraEnv, args...)
	return strings.TrimSpace(string(out)), err
}

func (s *ShadowRepo) gitRawWithEnv(ctx context.Context, extraEnv []string, args ...string) ([]byte, error) {
	// core.quotePath=false: without it git octal-escapes non-ASCII path bytes
	// in diff/numstat headers (`"a/\345\233\276.md"`), so the Last-turn review
	// card renders CJK filenames as garbage. The working-tree diff path already
	// pins this (webui meta.go git()); the shadow snapshot backend that feeds
	// `ar diff --scope last-turn` needs the same pin (QA-0719 t11 真机实证).
	full := append([]string{"--git-dir=" + s.gitDir, "--work-tree=" + s.work, "-c", "core.quotePath=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=agentrunner", "GIT_AUTHOR_EMAIL=snapshot@agentrunner",
		"GIT_COMMITTER_NAME=agentrunner", "GIT_COMMITTER_EMAIL=snapshot@agentrunner",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"HOME="+s.gitDir, // keep hooks/config lookups inside the shadow
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("snapshot: git %s: %v: %s", args[0], err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

var snapshotRefPattern = regexp.MustCompile(`^[0-9a-f]{40}(?:[0-9a-f]{24})?$`)

// Diff compares a durable barrier snapshot with the current workspace. It
// uses a private temporary index: the running agent may take another snapshot
// concurrently, but this review never reads or mutates the shadow HEAD/index.
// `git add -A` makes untracked/deleted files visible and reuses info/exclude,
// including the hard credential exclusions.
func (s *ShadowRepo) Diff(ctx context.Context, ref string) (DiffResult, error) {
	if !snapshotRefPattern.MatchString(ref) {
		return DiffResult{}, fmt.Errorf("snapshot: invalid snapshot ref")
	}
	f, err := os.CreateTemp(s.gitDir, "review-index-*")
	if err != nil {
		return DiffResult{}, fmt.Errorf("snapshot: create review index: %w", err)
	}
	index := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(index)
		return DiffResult{}, fmt.Errorf("snapshot: close review index: %w", err)
	}
	// Git expects a missing path when initializing an index with read-tree;
	// an existing zero-byte file is an invalid index.
	if err := os.Remove(index); err != nil {
		return DiffResult{}, fmt.Errorf("snapshot: prepare review index: %w", err)
	}
	defer func() {
		_ = os.Remove(index)
		_ = os.Remove(index + ".lock")
	}()
	env := []string{"GIT_INDEX_FILE=" + index}
	if _, err := s.gitWithEnv(ctx, env, "read-tree", ref); err != nil {
		return DiffResult{}, err
	}
	if _, err := s.gitWithEnv(ctx, env, "add", "-A", "."); err != nil {
		return DiffResult{}, err
	}
	untracked, untrackedReasons, hiddenUntracked, err := s.quietNewReviewFiles(ctx, env, ref)
	if err != nil {
		return DiffResult{}, err
	}
	diff, err := s.gitWithEnv(ctx, env, "diff", "--cached", "--no-ext-diff", "--no-color", "--find-renames", ref, "--")
	if err != nil {
		return DiffResult{}, err
	}
	numstat, err := s.gitWithEnv(ctx, env, "diff", "--cached", "--numstat", ref, "--")
	if err != nil {
		return DiffResult{}, err
	}
	return DiffResult{
		Diff: diff, Numstat: numstat, Untracked: untracked,
		UntrackedReasons: untrackedReasons, HiddenUntracked: hiddenUntracked,
	}, nil
}

// quietNewReviewFiles applies the same review-density contract as the Web UI's
// Working Tree projection, but only to paths added after the durable baseline.
// The temporary index remains the sole mutation target: snapshots and workspace
// files stay byte-identical, while generated paths disappear and large/binary
// additions become name-only cards instead of multi-hundred-kilobyte patches.
func (s *ShadowRepo) quietNewReviewFiles(ctx context.Context, env []string, ref string) ([]string, map[string]string, int, error) {
	raw, err := s.gitRawWithEnv(ctx, env, "diff", "--cached", "--name-only", "--diff-filter=A", "-z", ref, "--")
	if err != nil {
		return nil, nil, 0, err
	}
	untracked := []string{}
	reasons := map[string]string{}
	hidden := 0
	visible := 0
	const maxVisible = 500
	const maxInlineBytes = 256 * 1024
	for _, item := range bytes.Split(raw, []byte{0}) {
		if len(item) == 0 {
			continue
		}
		path := string(item)
		hide := reviewHiddenUntrackedPath(path) || visible >= maxVisible
		if hide {
			hidden++
		} else {
			visible++
			full := filepath.Join(s.work, filepath.FromSlash(path))
			info, statErr := os.Stat(full)
			if statErr != nil || !info.Mode().IsRegular() {
				untracked = append(untracked, path)
				reasons[path] = "unavailable"
				hide = true
			} else if info.Size() > maxInlineBytes {
				untracked = append(untracked, path)
				reasons[path] = "large"
				hide = true
			} else if content, readErr := os.ReadFile(full); readErr != nil {
				untracked = append(untracked, path)
				reasons[path] = "unavailable"
				hide = true
			} else if bytes.Contains(content, []byte{0}) {
				untracked = append(untracked, path)
				reasons[path] = "binary"
				hide = true
			}
		}
		if hide {
			if _, err := s.gitWithEnv(ctx, env, "update-index", "--force-remove", "--", path); err != nil {
				return nil, nil, 0, err
			}
		}
	}
	return untracked, reasons, hidden, nil
}

func reviewHiddenUntrackedPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		switch part {
		case ".git", ".venv", "venv", "site-packages", ".tox", ".eggs", ".cache", ".next", ".turbo", ".gradle", "node_modules", "vendor", "dist", "build", "out", "target", "coverage", "__pycache__":
			return true
		}
		if strings.HasSuffix(part, ".dist-info") || strings.HasSuffix(part, ".egg-info") {
			return true
		}
	}
	return false
}

// Snapshot: stage the whole workspace (info/exclude applies), write the
// tree, and commit it — deduplicating on an unchanged tree (same state,
// same ref). Plumbing only: no hooks, no porcelain "nothing to commit".
func (s *ShadowRepo) Snapshot(ctx context.Context) (string, error) {
	var snapshot string
	err := withRepoLock(ctx, s.gitDir, func() error {
		if _, err := s.git(ctx, "add", "-A", "."); err != nil {
			return err
		}
		tree, err := s.git(ctx, "write-tree")
		if err != nil {
			return err
		}
		head, headErr := s.git(ctx, "rev-parse", "HEAD")
		if headErr == nil {
			if prevTree, err := s.git(ctx, "rev-parse", "HEAD^{tree}"); err == nil && prevTree == tree {
				snapshot = head
				return nil
			}
		}
		args := []string{"commit-tree", tree, "-m", "agentrunner snapshot"}
		if headErr == nil {
			args = append(args, "-p", head)
		}
		commit, err := s.git(ctx, args...)
		if err != nil {
			return err
		}
		if _, err := s.git(ctx, "update-ref", "HEAD", commit); err != nil {
			return err
		}
		snapshot = commit
		return nil
	})
	return snapshot, err
}

// PushRefs copies snapshot commits into another shadow GIT_DIR, pinning
// each under refs/pinned/<ref> (S7.3): a fork's inherited barriers stay
// materializable from the fork workspace's OWN store, so a fork of a fork
// never reaches back into the original's repo. Local-path push moves the
// full object closure; an already-present ref is a cheap no-op.
func (s *ShadowRepo) PushRefs(ctx context.Context, dstGitDir string, refs []string) error {
	return withRepoLock(ctx, dstGitDir, func() error {
		for _, ref := range refs {
			if ref == "" {
				continue
			}
			if _, err := s.git(ctx, "push", "--quiet", dstGitDir, ref+":refs/pinned/"+ref); err != nil {
				return err
			}
		}
		return nil
	})
}

// GitDir exposes the store's GIT_DIR for ref transfer between stores.
func (s *ShadowRepo) GitDir() string { return s.gitDir }

// Materialize extracts ref into dir via `git archive` — no index or HEAD
// mutation, no linked-worktree metadata to clean up. Extraction is ATOMIC
// at the directory level: it lands in a temp sibling and renames into
// place, so a crash mid-extraction leaves dir ABSENT, never truncated —
// callers may treat an existing dir as a complete tree (S7 出口 review).
func (s *ShadowRepo) Materialize(ctx context.Context, ref, dir string) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("snapshot: materialize target %s is not empty", dir)
	}
	tmp, err := os.MkdirTemp(parent, ".materialize-*")
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }() // no-op after a successful rename

	arch := exec.CommandContext(ctx, "git", "--git-dir="+s.gitDir, "archive", "--format=tar", ref)
	tarCmd := exec.CommandContext(ctx, "tar", "-x", "-C", tmp)
	pipe, err := arch.StdoutPipe()
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	tarCmd.Stdin = pipe
	var archErr, tarErr bytes.Buffer
	arch.Stderr = &archErr
	tarCmd.Stderr = &tarErr
	if err := arch.Start(); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	if err := tarCmd.Start(); err != nil {
		// Nothing will drain git's stdout: kill it or Wait blocks forever
		// on a tree larger than the pipe buffer (S7 出口 review).
		_ = arch.Process.Kill()
		_ = arch.Wait()
		return fmt.Errorf("snapshot: %w", err)
	}
	tErr := tarCmd.Wait()
	aErr := arch.Wait()
	if aErr != nil {
		return fmt.Errorf("snapshot: git archive %s: %v: %s", ref, aErr, strings.TrimSpace(archErr.String()))
	}
	if tErr != nil {
		return fmt.Errorf("snapshot: tar extract: %v: %s", tErr, strings.TrimSpace(tarErr.String()))
	}
	// An existing-but-empty target was allowed above; clear it for the rename.
	_ = os.Remove(dir)
	if err := os.Rename(tmp, dir); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	return nil
}
