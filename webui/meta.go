package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// sessionMeta caches workspace/title values from the journal-backed CLI
// contract. Values recorded while arwebui creates a session are a compatibility
// fallback only; this store is never the source of runtime truth.
type sessionMeta struct {
	Workspace string `json:"workspace"`
	Title     string `json:"title"`
}

type metaStore struct {
	mu   sync.Mutex
	m    map[string]sessionMeta
	path string // JSON persistence file; "" = in-memory only (tests)
}

// newMetaStore loads the persisted sid→meta cache (if any) so a webui restart
// can render immediately while the journal-backed session list hydrates.
func newMetaStore(path string) *metaStore {
	s := &metaStore{m: map[string]sessionMeta{}, path: path}
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(b, &s.m)
		}
	}
	return s
}

// persistLocked writes the map atomically (tmp+rename). Callers hold mu.
func (s *metaStore) persistLocked() {
	if s.path == "" {
		return
	}
	b, err := json.MarshalIndent(s.m, "", " ")
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
}

func (s *metaStore) set(sid, ws, title string) {
	if sid == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.m[sid]
	before := cur
	if ws != "" {
		cur.Workspace = ws
	}
	if title != "" && cur.Title == "" {
		cur.Title = firstLine(title, 100)
	}
	s.m[sid] = cur
	if cur == before {
		return
	}
	s.persistLocked()
}

// merge hydrates metadata discovered from AgentRunner's journal-backed
// `sessions --json` contract. It persists once for the whole list so the
// 4-second session refresh does not rewrite the metadata file per row.
func (s *metaStore) merge(entries map[string]sessionMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for sid, incoming := range entries {
		if sid == "" {
			continue
		}
		cur := s.m[sid]
		before := cur
		if incoming.Workspace != "" {
			cur.Workspace = incoming.Workspace
		}
		if incoming.Title != "" && cur.Title == "" {
			cur.Title = firstLine(incoming.Title, 100)
		}
		if cur != before {
			s.m[sid] = cur
			changed = true
		}
	}
	if changed {
		s.persistLocked()
	}
}

func (s *metaStore) get(sid string) sessionMeta {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[sid]
}

// titleFromID recovers a readable title from a session id when arwebui has no
// recorded metadata (the session was created via the CLI, not this UI — UX-03).
// Ids look like "20260709-071306-find-the-function-that-disable-39bd": strip the
// leading date-time and the trailing 4-hex suffix, de-slugify the rest.
func titleFromID(id string) string {
	parts := strings.Split(id, "-")
	// Drop leading date + time segments (all-digit) …
	for len(parts) > 0 && isDigits(parts[0]) {
		parts = parts[1:]
	}
	// … and a trailing short hex suffix (the uniquifier).
	if n := len(parts); n > 1 && len(parts[n-1]) <= 4 && isHex(parts[n-1]) {
		parts = parts[:n-1]
	}
	if len(parts) == 0 {
		return id
	}
	return strings.Join(parts, " ")
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// git runs a read-only git command in dir and returns trimmed stdout ("" on
// any error — the caller treats a non-repo / missing dir as "no changes").
func git(ctx context.Context, dir string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "--no-pager"}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return out.String(), true
}

// handleFiles lists workspace files for the composer's @-mention picker
// (Codex's file references). Case-insensitive substring filter via ?q=, capped
// scan so a huge tree can't wedge the request; dot-dirs and dependency dirs
// are skipped.
func (s *server) handleFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	ws := s.meta.get(id).Workspace
	resp := map[string]any{"workspace": ws, "known": ws != "", "files": []string{}}
	if ws == "" {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	q := strings.ToLower(r.URL.Query().Get("q"))
	const maxScan, maxResults = 20000, 50
	skip := map[string]bool{".git": true, "node_modules": true, "dist": true, ".venv": true, "__pycache__": true}
	files := []string{}
	scanned := 0
	_ = filepath.WalkDir(ws, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip, keep walking
		}
		if scanned++; scanned > maxScan || len(files) >= maxResults {
			return filepath.SkipAll
		}
		name := d.Name()
		if d.IsDir() {
			if path != ws && (skip[name] || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(ws, path)
		if err != nil {
			return nil
		}
		if q == "" || strings.Contains(strings.ToLower(rel), q) {
			files = append(files, rel)
		}
		return nil
	})
	resp["files"] = files
	writeJSON(w, http.StatusOK, resp)
}

// handleSessionFile downloads one regular file from the session's workspace.
// The browser never receives an arbitrary host path: absolute paths, traversal,
// directories, and symlinks escaping the workspace are rejected. This is the
// truthful subset of Codex's document "Open in" menu that the web surface can
// support without pretending to have a desktop-app launcher.
func (s *server) handleSessionFile(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	ws := s.meta.get(id).Workspace
	if ws == "" {
		http.Error(w, "session workspace is unavailable", http.StatusNotFound)
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("path"))
	clean := filepath.Clean(raw)
	if raw == "" || filepath.IsAbs(raw) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		badRequest(w, "path must be a file relative to the session workspace")
		return
	}
	root, err := filepath.EvalSymlinks(filepath.Clean(ws))
	if err != nil {
		http.Error(w, "session workspace is unavailable", http.StatusNotFound)
		return
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, clean))
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		badRequest(w, "file is outside the session workspace")
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	const maxDownload = 64 << 20
	if !info.Mode().IsRegular() {
		badRequest(w, "path is not a regular file")
		return
	}
	if info.Size() > maxDownload {
		http.Error(w, "file is too large to download", http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(target)}))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, target)
}

// samePath reports whether two paths name the same directory once symlinks
// (macOS /tmp → /private/tmp) and trailing separators are resolved.
func samePath(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(filepath.Clean(a))
	rb, err2 := filepath.EvalSymlinks(filepath.Clean(b))
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// worktreeInfo reports whether dir is a LINKED git worktree (as opposed to a
// repo's main working tree) and, if so, the main checkout path and the
// worktree's current branch ("" when detached). A linked worktree has a
// per-worktree git-dir distinct from the shared common-dir. Used to surface
// "worktree of <repo> on <branch>" and the Apply/Remove controls (INC-49).
func worktreeInfo(ctx context.Context, dir string) (isWorktree bool, mainRepo, branch string) {
	out, ok := git(ctx, dir, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
	if !ok {
		return false, "", ""
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || samePath(strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])) {
		return false, "", "" // main working tree (git-dir == common-dir)
	}
	// First `worktree <path>` line of the porcelain listing is the main checkout.
	if list, ok := git(ctx, dir, "worktree", "list", "--porcelain"); ok {
		for _, l := range strings.Split(list, "\n") {
			if p, found := strings.CutPrefix(l, "worktree "); found {
				mainRepo = strings.TrimSpace(p)
				break
			}
		}
	}
	if b, ok := git(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD"); ok {
		if b = strings.TrimSpace(b); b != "HEAD" {
			branch = b
		}
	}
	return true, mainRepo, branch
}

// handleDiff returns the workspace diff for a session — the closest analog to
// Codex's changed-files view. Tracked changes come from `git diff`; untracked
// files are listed from `git status --porcelain` and appended as synthetic
// "new file" diffs so the UI shows them too.
//
// isRepo means the workspace is a repository ROOT of its own. A workspace
// that merely sits inside some other repo (e.g. an auto-created dir under a
// gitignored runtime/) reports nested:true instead — running `git diff` there
// would silently diff the parent repo and always come back empty (W1).
func (s *server) handleDiff(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	meta := s.meta.get(id)
	resp := map[string]any{"workspace": meta.Workspace, "known": meta.Workspace != "", "isRepo": false, "nested": false, "repoRoot": "", "diff": "", "numstat": "", "untracked": []string{}, "worktree": false, "mainRepo": "", "branch": ""}
	if meta.Workspace == "" {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	top, insideRepo := git(r.Context(), meta.Workspace, "rev-parse", "--show-toplevel")
	if !insideRepo {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if root := strings.TrimSpace(top); !samePath(root, meta.Workspace) {
		resp["nested"] = true
		resp["repoRoot"] = root
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp["isRepo"] = true
	// A linked worktree is its own toplevel (so isRepo is true), but it belongs
	// to a main checkout the UI names and offers Apply/Remove against (INC-49).
	if isWt, mainRepo, branch := worktreeInfo(r.Context(), meta.Workspace); isWt {
		resp["worktree"] = true
		resp["mainRepo"] = mainRepo
		resp["branch"] = branch
	}
	diff, _ := git(r.Context(), meta.Workspace, "diff")
	numstat, _ := git(r.Context(), meta.Workspace, "diff", "--numstat")
	resp["diff"] = diff
	resp["numstat"] = numstat
	if porcelain, ok := git(r.Context(), meta.Workspace, "status", "--porcelain", "--untracked-files=all"); ok {
		untracked := []string{} // never nil — the UI does .length on this
		var extra bytes.Buffer  // synthetic new-file diffs for untracked content
		for _, line := range strings.Split(porcelain, "\n") {
			if !strings.HasPrefix(line, "?? ") {
				continue
			}
			path := strings.TrimSpace(line[3:])
			// Inline the content of small, regular, text files as a new-file
			// diff so the UI shows it (git diff omits untracked entirely).
			full := filepath.Join(meta.Workspace, path)
			if info, err := os.Stat(full); err == nil && info.Mode().IsRegular() && info.Size() <= 256*1024 {
				if content, err := os.ReadFile(full); err == nil && !bytes.Contains(content, []byte{0}) {
					lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
					fmt.Fprintf(&extra, "diff --git a/%s b/%s\nnew file\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n",
						path, path, path, len(lines))
					for _, l := range lines {
						extra.WriteString("+" + l + "\n")
					}
					continue
				}
			}
			untracked = append(untracked, path) // binary / large / unreadable: name only
		}
		if extra.Len() > 0 {
			d, _ := resp["diff"].(string)
			if d != "" && !strings.HasSuffix(d, "\n") {
				d += "\n"
			}
			resp["diff"] = d + extra.String()
		}
		resp["untracked"] = untracked
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleCommit stages and commits the workspace changes — Codex's review→commit
// step, closing the loop after the Diff view. Local commit only (no push); if
// the repo has no git identity, a cockpit fallback is used for that one commit.
func (s *server) handleCommit(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if !readBody(w, r, &req) {
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		msg = "changes from agent session " + id
	}
	ws := s.meta.get(id).Workspace
	if ws == "" {
		badRequest(w, "arwebui doesn't know this session's workspace")
		return
	}
	top, insideRepo := git(r.Context(), ws, "rev-parse", "--show-toplevel")
	if !insideRepo {
		badRequest(w, "workspace is not a git repository")
		return
	}
	// A nested workspace must never commit: `git add -A` would stage (and
	// commit) the PARENT repository's tree, not this workspace.
	if root := strings.TrimSpace(top); !samePath(root, ws) {
		badRequest(w, "workspace is inside another repository ("+root+"), refusing to commit there")
		return
	}
	run := func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", ws}, args...)...)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	if out, err := run("add", "-A"); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "git add failed", "stderr": out})
		return
	}
	out, err := run("commit", "-m", msg)
	if err != nil && (strings.Contains(out, "user.email") || strings.Contains(out, "empty ident") ||
		strings.Contains(out, "Please tell me who you are") || strings.Contains(out, "author identity")) {
		out, err = run("-c", "user.name=AgentRunner", "-c", "user.email=agent@agentrunner.local", "commit", "-m", msg)
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "git commit failed", "stderr": out})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": out})
}

// handleGitInit turns the session's workspace into its own git repository so
// the Changes view can track it — the recovery action offered when the diff
// endpoint reports a non-repo or nested workspace (W1). The path comes from
// session metadata only, never from the request.
func (s *server) handleGitInit(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	ws := s.meta.get(id).Workspace
	if ws == "" {
		badRequest(w, "arwebui doesn't know this session's workspace")
		return
	}
	if top, insideRepo := git(r.Context(), ws, "rev-parse", "--show-toplevel"); insideRepo && samePath(strings.TrimSpace(top), ws) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already a repository"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", ws, "init", "-q").CombinedOutput()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "git init failed", "stderr": strings.TrimSpace(string(out))})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "initialized"})
}

// gitIn runs a git command in dir, optionally feeding stdin, and returns
// combined output. A shared 20s timeout keeps a wedged git from hanging the
// request.
func gitIn(ctx context.Context, dir string, stdin []byte, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// handleApply applies a worktree session's changes back onto its main checkout —
// Codex's "Apply changes" (INC-49). The mechanism is git-native and
// clean-or-nothing: capture the full worktree change set (including untracked)
// as a commit object in the shared object DB, produce a binary patch, dry-run
// `git apply --check` on the main checkout, and only apply when it verifies.
// A conflict is reported verbatim and the main working tree is left untouched —
// never a silent half-merge. Applied changes land UNSTAGED so the user reviews
// and commits them in their own checkout.
func (s *server) handleApply(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	ws := s.meta.get(id).Workspace
	if ws == "" {
		badRequest(w, "arwebui doesn't know this session's workspace")
		return
	}
	isWt, mainRepo, _ := worktreeInfo(r.Context(), ws)
	if !isWt || mainRepo == "" {
		badRequest(w, "this session's workspace is not a git worktree of a project")
		return
	}
	// Stage everything (tracked edits + untracked new files, .gitignore honored)
	// and turn it into a commit object WITHOUT moving the worktree's HEAD, so the
	// patch below carries the complete change set.
	if out, err := gitIn(r.Context(), ws, nil, "add", "-A"); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "git add failed", "stderr": out})
		return
	}
	tree, err := gitIn(r.Context(), ws, nil, "write-tree")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "git write-tree failed", "stderr": tree})
		return
	}
	commit, err := gitIn(r.Context(), ws, nil, "commit-tree", tree, "-p", "HEAD", "-m", "apply-back from agent worktree")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "git commit-tree failed", "stderr": commit})
		return
	}
	// Restore the worktree's index to HEAD (mixed reset) so the changes go back to
	// their pre-apply UNSTAGED state — otherwise the `git add -A` above would leave
	// them staged and the Changes view (which shows unstaged + untracked) would
	// blank out right after Apply. Everything downstream compares commit objects,
	// not the index, so this is safe.
	_, _ = gitIn(r.Context(), ws, nil, "reset", "-q")
	patch, err := gitIn(r.Context(), ws, nil, "diff", "--binary", "HEAD", commit)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "git diff failed", "stderr": patch})
		return
	}
	if strings.TrimSpace(patch) == "" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "no changes to apply", "applied": ""})
		return
	}
	pb := []byte(patch + "\n")
	// Dry run first: if the patch does not apply cleanly we report the conflict
	// and change NOTHING in the main checkout.
	if out, err := gitIn(r.Context(), mainRepo, pb, "apply", "--check", "-"); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":  "changes do not apply cleanly onto " + mainRepo + " — resolve the divergence (e.g. commit the worktree branch and merge it) and try again",
			"stderr": out,
		})
		return
	}
	if out, err := gitIn(r.Context(), mainRepo, pb, "apply", "-"); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "git apply failed", "stderr": out})
		return
	}
	files, _ := gitIn(r.Context(), ws, nil, "diff", "--name-only", "HEAD", commit)
	writeJSON(w, http.StatusOK, map[string]string{"status": "applied to " + mainRepo, "mainRepo": mainRepo, "applied": files})
}

// handleWorktreeRemove deletes a session's worktree checkout and prunes the
// stale registry entry (INC-49 cleanup). A worktree with uncommitted/untracked
// changes is refused unless the request sets force:true — the UI turns that
// refusal into an explicit "you have unapplied changes, delete anyway?" prompt.
func (s *server) handleWorktreeRemove(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	var req struct {
		Force bool `json:"force"`
	}
	if !readBody(w, r, &req) {
		return
	}
	ws := s.meta.get(id).Workspace
	if ws == "" {
		badRequest(w, "arwebui doesn't know this session's workspace")
		return
	}
	isWt, mainRepo, _ := worktreeInfo(r.Context(), ws)
	if !isWt || mainRepo == "" {
		badRequest(w, "this session's workspace is not a git worktree of a project")
		return
	}
	args := []string{"worktree", "remove"}
	if req.Force {
		args = append(args, "--force")
	}
	args = append(args, "--", ws)
	if out, err := gitIn(r.Context(), mainRepo, nil, args...); err != nil {
		// Dirty/untracked worktrees are refused without --force. Surface that as a
		// structured signal so the UI can ask for confirmation rather than error.
		if !req.Force && (strings.Contains(out, "use --force") || strings.Contains(out, "contains modified or untracked")) {
			writeJSON(w, http.StatusConflict, map[string]any{"dirty": true, "error": "worktree has unapplied changes", "stderr": out})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "git worktree remove failed", "stderr": out})
		return
	}
	_, _ = gitIn(r.Context(), mainRepo, nil, "worktree", "prune")
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed", "mainRepo": mainRepo})
}
