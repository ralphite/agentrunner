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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

// derivedDirs are directory names whose contents are MACHINE-REGENERABLE: a
// package manager, build, or test run puts them there, and the same command
// puts them back. They are excluded from snapshots (DESIGN §6: harness 级
// exclude 列表（node_modules/venv/build 类 + 凭据文件硬排除表）) and are
// therefore documented as OUTSIDE REWIND SCOPE — a rewind restores your source,
// not your `node_modules`.
//
// Membership is deliberately conservative, because the two lists that consume
// this set carry very different risk. Hiding a path from a REVIEW CARD costs a
// tidier diff at worst. Excluding it from a SNAPSHOT means a rewind will not
// bring it back — so anything a project might legitimately commit as source
// must not be here. That is why `vendor` is NOT in this set (Go and PHP
// projects commit it routinely, and DESIGN does not name it) even though the
// review projection still hides it; see reviewOnlyHiddenDirs.
var derivedDirs = map[string]bool{
	".git":         true, // git refuses these paths anyway; listed for intent
	"node_modules": true, "__pycache__": true, "site-packages": true,
	".venv": true, "venv": true, ".tox": true, ".eggs": true,
	".next": true, ".turbo": true, ".gradle": true, ".cache": true,
	"dist": true, "build": true, "out": true, "target": true, "coverage": true,
}

// reviewOnlyHiddenDirs are hidden from review cards but STILL SNAPSHOTTED.
// Splitting them out is the whole point: a display filter may be aggressive,
// a durability filter may not.
var reviewOnlyHiddenDirs = map[string]bool{"vendor": true}

// derivedSuffixes are path components matched by suffix rather than by name.
var derivedSuffixes = []string{".dist-info", ".egg-info"}

// derivedPath reports whether any component of path is machine-regenerable.
func derivedPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if derivedDirs[part] {
			return true
		}
		for _, suffix := range derivedSuffixes {
			if strings.HasSuffix(part, suffix) {
				return true
			}
		}
	}
	return false
}

// derivedExcludePatterns renders derivedDirs/derivedSuffixes as .gitignore
// lines. A bare `name/` matches that directory at ANY depth, which is exactly
// the component-wise semantics derivedPath implements — so the file written to
// info/exclude and the Go predicate cannot drift.
func derivedExcludePatterns() []string {
	names := make([]string, 0, len(derivedDirs))
	for name := range derivedDirs {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic info/exclude content
	out := make([]string, 0, len(names)+len(derivedSuffixes))
	for _, name := range names {
		out = append(out, name+"/")
	}
	for _, suffix := range derivedSuffixes {
		out = append(out, "*"+suffix+"/")
	}
	return out
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
			// Already initialized: refresh excludes, then converge an index
			// that predates them (see pruneDerivedFromIndex).
			if err := s.writeExcludes(); err != nil {
				return err
			}
			return s.pruneDerivedFromIndex(context.Background())
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
	content := "# machine-regenerable trees — DOCUMENTED AS OUTSIDE REWIND SCOPE.\n" +
		"# The workspace's own .gitignore still does most of this work; these are\n" +
		"# the floor, for the workspace that has no .gitignore at all.\n" +
		strings.Join(derivedExcludePatterns(), "\n") + "\n"
	return os.WriteFile(filepath.Join(info, "exclude"), []byte(content), 0o600)
}

// pruneDerivedFromIndex drops already-tracked derived paths from the index.
//
// .gitignore semantics only govern UNTRACKED files, so a shadow repo created
// before these excludes existed would keep staging the `node_modules` it had
// already picked up — the exclude would silently never take effect there. This
// is the convergence step that makes the new floor apply to existing repos too.
//
// Deliberately filtered by derivedPath rather than by `git ls-files -i
// --exclude-standard`: the latter would also untrack anything the USER's
// .gitignore happens to list, which is a decision this function has no business
// making. Index-only walk, so cost is proportional to tracked files, and it is
// idempotent — after the first pass nothing matches. Callers hold the repo lock.
func (s *ShadowRepo) pruneDerivedFromIndex(ctx context.Context) error {
	if _, err := os.Stat(filepath.Join(s.gitDir, "index")); err != nil {
		return nil // nothing tracked yet
	}
	raw, err := s.gitRawWithEnv(ctx, nil, "ls-files", "-z")
	if err != nil {
		return err
	}
	var stale []string
	for _, item := range bytes.Split(raw, []byte{0}) {
		if len(item) == 0 {
			continue
		}
		if path := string(item); derivedPath(path) {
			stale = append(stale, path)
		}
	}
	if len(stale) == 0 {
		return nil
	}
	// Batch through stdin: a monorepo that predates the excludes can have tens
	// of thousands of these, which would blow argv as separate arguments.
	cmd := exec.CommandContext(ctx, "git",
		"--git-dir="+s.gitDir, "--work-tree="+s.work,
		"update-index", "--force-remove", "-z", "--stdin")
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"HOME="+s.gitDir)
	cmd.Stdin = strings.NewReader(strings.Join(stale, "\x00") + "\x00")
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("snapshot: prune derived paths: %v: %s", err, strings.TrimSpace(errb.String()))
	}
	return nil
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
// `git add -A` makes untracked/deleted files visible and reuses the
// machine-regenerable-tree excludes.
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
	if err := s.seedReviewIndex(ctx, index, ref, env); err != nil {
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

// seedReviewIndex prepares the private index that Diff's `git add -A` builds on.
//
// The obvious seed — `read-tree <ref>` — is a performance trap: an index built
// that way carries NO stat cache, so the `add -A` that follows cannot take the
// mtime/size shortcut and must re-read and re-hash every file in the workspace.
// Measured on a 300k-file tree: 29.0s, against 796ms for the snapshot path's
// stat-cache hit. A 36x gap on the code path behind `ar diff --scope last-turn`
// and the webui DIFF screen — which refetches on every streamed event against a
// 30s timeout, so at that size the screen simply stopped resolving (GAPS G61).
//
// Copying the repo's PERSISTENT index carries its stat cache over, so `add -A`
// hashes only what actually changed. The seed's content is irrelevant: every
// consumer names <ref> explicitly (`diff --cached <ref>`), so the index only has
// to end up representing the working tree — which `add -A` guarantees from
// either seed, deletions included.
//
// Trusting the stat cache here introduces no new risk: Snapshot() already trusts
// the same cache for the DURABLE snapshot, so a cache that could miss a change
// would mean wrong snapshots long before it meant a wrong diff. Git's own
// racily-clean handling is what makes both safe.
//
// Concurrency is unchanged and still lock-free — DESIGN's "Diff 使用 private
// index，仍可并发只读" holds because the target index is still private. Git
// writes its index by lock-and-rename, so a concurrent Snapshot is never
// observed half-written by the copy.
//
// Falls back to read-tree when there is no persistent index yet (no snapshot has
// been taken in this repo). The fallback is silent because it IS the previous
// behavior — correct, just slower.
func (s *ShadowRepo) seedReviewIndex(ctx context.Context, index, ref string, env []string) error {
	if err := copyIndexFile(filepath.Join(s.gitDir, "index"), index); err == nil {
		return nil
	}
	_, err := s.gitWithEnv(ctx, env, "read-tree", ref)
	return err
}

// copyIndexFile copies src to dst, which must not exist.
func copyIndexFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}

// DiffSnapshots compares two durable cuts without consulting the live
// workspace. This distinction is essential for completed-turn attribution:
// files changed after the assistant's final cut must not be charged to that
// already-finished turn.
func (s *ShadowRepo) DiffSnapshots(ctx context.Context, fromRef, toRef string) (DiffResult, error) {
	if !snapshotRefPattern.MatchString(fromRef) || !snapshotRefPattern.MatchString(toRef) {
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
	if err := os.Remove(index); err != nil {
		return DiffResult{}, fmt.Errorf("snapshot: prepare review index: %w", err)
	}
	defer func() {
		_ = os.Remove(index)
		_ = os.Remove(index + ".lock")
	}()
	env := []string{"GIT_INDEX_FILE=" + index}
	if _, err := s.gitWithEnv(ctx, env, "read-tree", toRef); err != nil {
		return DiffResult{}, err
	}
	untracked, untrackedReasons, hiddenUntracked, err :=
		s.quietSnapshotReviewFiles(ctx, env, fromRef, toRef)
	if err != nil {
		return DiffResult{}, err
	}
	diff, err := s.gitWithEnv(ctx, env, "diff", "--cached", "--no-ext-diff",
		"--no-color", "--find-renames", fromRef, "--")
	if err != nil {
		return DiffResult{}, err
	}
	numstat, err := s.gitWithEnv(ctx, env, "diff", "--cached", "--numstat", fromRef, "--")
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

// quietSnapshotReviewFiles is quietNewReviewFiles' durable-cut counterpart.
// It reads additions from toRef itself, never from the possibly-newer live
// workspace.
func (s *ShadowRepo) quietSnapshotReviewFiles(ctx context.Context, env []string,
	fromRef, toRef string) ([]string, map[string]string, int, error) {
	raw, err := s.gitRawWithEnv(ctx, env, "diff", "--cached", "--name-only",
		"--diff-filter=A", "-z", fromRef, "--")
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
			object := toRef + ":" + path
			sizeRaw, sizeErr := s.gitRawWithEnv(ctx, nil, "cat-file", "-s", object)
			size, parseErr := strconv.ParseInt(strings.TrimSpace(string(sizeRaw)), 10, 64)
			if sizeErr != nil || parseErr != nil {
				untracked = append(untracked, path)
				reasons[path] = "unavailable"
				hide = true
			} else if size > maxInlineBytes {
				untracked = append(untracked, path)
				reasons[path] = "large"
				hide = true
			} else if content, readErr := s.gitRawWithEnv(ctx, nil, "cat-file", "blob", object); readErr != nil {
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

// reviewHiddenUntrackedPath is the DISPLAY filter: everything machine-regenerable
// plus the review-only extras. It shares derivedDirs with the snapshot excludes
// so the two can never silently disagree about what "derived" means, while
// keeping its own strictly-larger set for paths that must stay snapshotted.
func reviewHiddenUntrackedPath(path string) bool {
	if derivedPath(path) {
		return true
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if reviewOnlyHiddenDirs[part] {
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
