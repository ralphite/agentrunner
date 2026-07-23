package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
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

// projectMeta is the workspace-keyed overlay added by INC-53 (HANDA #24): a
// user's cosmetic preferences for one project group — a custom display name,
// folded/pinned/removed presentation state, and when it was last opened in a
// system app via the launcher. It is DECORATIVE ONLY: project grouping still
// derives from the journal workspace (DESIGN §12 invariant), and an empty
// DisplayName falls back to the derived label. Removed hides only the sidebar
// projection; it never deletes or reassigns sessions, journals, or workspaces.
type projectMeta struct {
	DisplayName string `json:"displayName,omitempty"`
	Folded      bool   `json:"folded,omitempty"`
	Pinned      bool   `json:"pinned,omitempty"`
	Removed     bool   `json:"removed,omitempty"`
	LastOpened  int64  `json:"lastOpened,omitempty"` // unix millis; 0 = never
}

// metaFile is the on-disk shape of webui-meta.json since INC-53: the session
// cache plus the project overlay. Older files are a bare sid→sessionMeta map
// (no "sessions"/"projects" top-level keys); newMetaStore detects and reads
// those, then re-persists in this wrapper on the next write. The store stays a
// non-authoritative cache, so a transient cross-version read is self-healing.
type metaFile struct {
	Sessions map[string]sessionMeta `json:"sessions"`
	Projects map[string]projectMeta `json:"projects"`
}

type metaStore struct {
	mu       sync.Mutex
	m        map[string]sessionMeta
	projects map[string]projectMeta
	path     string // JSON persistence file; "" = in-memory only (tests)
}

// newMetaStore loads the persisted sid→meta cache and project overlay (if any)
// so a webui restart can render immediately while the journal-backed session
// list hydrates. It tolerates a missing file and the legacy flat format.
func newMetaStore(path string) *metaStore {
	s := &metaStore{m: map[string]sessionMeta{}, projects: map[string]projectMeta{}, path: path}
	if path == "" {
		return s
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	// Probe the top level: a wrapper file has "sessions"/"projects" keys; a
	// legacy file is a bare sid→sessionMeta map (session ids never collide with
	// those reserved words). This keeps the upgrade from silently dropping a
	// user's cached WebUI titles.
	var probe map[string]json.RawMessage
	if json.Unmarshal(b, &probe) == nil {
		_, hasSessions := probe["sessions"]
		_, hasProjects := probe["projects"]
		if hasSessions || hasProjects {
			var mf metaFile
			if json.Unmarshal(b, &mf) == nil {
				if mf.Sessions != nil {
					s.m = mf.Sessions
				}
				if mf.Projects != nil {
					s.projects = mf.Projects
				}
			}
			return s
		}
	}
	// Legacy flat format: the whole file is the session cache.
	_ = json.Unmarshal(b, &s.m)
	return s
}

// persistLocked writes the wrapper atomically (tmp+rename). Callers hold mu.
func (s *metaStore) persistLocked() {
	if s.path == "" {
		return
	}
	b, err := json.MarshalIndent(metaFile{Sessions: s.m, Projects: s.projects}, "", " ")
	if err != nil {
		log.Printf("ERROR: failed to marshal metadata: %v", err)
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		log.Printf("ERROR: failed to write metadata file %q: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		log.Printf("ERROR: failed to rename metadata file from %q to %q: %v", tmp, s.path, err)
	}
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

// allProjects returns a copy of the workspace-keyed overlay (INC-53) so the
// frontend can render custom names, folded state, and last-opened times.
func (s *metaStore) allProjects() map[string]projectMeta {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]projectMeta, len(s.projects))
	for k, v := range s.projects {
		out[k] = v
	}
	return out
}

// setProject applies a partial update to one project's overlay entry (INC-53,
// extended by INC-87):
// a nil pointer means "leave unchanged". An empty display name clears the
// override (the group reverts to its derived label), mirroring the session
// rename semantics. An entry that ends up entirely default is dropped so the
// file doesn't accrete empty overlays. Persists on any change.
func (s *metaStore) setProject(key string, displayName *string, folded, pinned, removed *bool) {
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.projects[key]
	before := cur
	if displayName != nil {
		cur.DisplayName = strings.TrimSpace(*displayName)
	}
	if folded != nil {
		cur.Folded = *folded
	}
	if pinned != nil {
		cur.Pinned = *pinned
	}
	if removed != nil {
		cur.Removed = *removed
	}
	if cur == before {
		return
	}
	if cur == (projectMeta{}) {
		delete(s.projects, key)
	} else {
		s.projects[key] = cur
	}
	s.persistLocked()
}

// touchProject records that a project was just opened in a system app
// (INC-53 launcher), so the sidebar can show a "last opened" hint.
func (s *metaStore) touchProject(key string) {
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.projects[key]
	cur.LastOpened = time.Now().UnixMilli()
	s.projects[key] = cur
	s.persistLocked()
}

// titleFromID recovers a readable title from a session id when arwebui has no
// recorded metadata (the session was created via the CLI, not this UI — UX-03).
// Ids look like "20260709-071306-find-the-function-that-disable-39bd": strip the
// leading date-time and the trailing entropy suffix, de-slugify the rest.
func titleFromID(id string) string {
	parts := strings.Split(id, "-")
	// Drop leading date + time segments (all-digit) …
	for len(parts) > 0 && isDigits(parts[0]) {
		parts = parts[1:]
	}
	// … and a trailing short hex suffix (the uniquifier).
	if n := len(parts); n > 1 {
		suffixLen := len(parts[n-1])
		if (suffixLen == 4 || suffixLen == 16) && isHex(parts[n-1]) {
			parts = parts[:n-1]
		}
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
	// core.quotePath=false: git otherwise octal-escapes non-ASCII path bytes
	// ("设计 稿.md" → "\350\256\276..." in diff headers), which the UI then
	// shows verbatim (QA-0719 S12 U1). The API is JSON — raw UTF-8 is safe.
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "--no-pager", "-c", "core.quotePath=false"}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return out.String(), true
}

// joinDiffText concatenates two diff (or numstat) outputs, skipping empties —
// used on an unborn branch where staged and unstaged changes need two git
// invocations (`--cached` vs plain) to cover.
func joinDiffText(a, b string) string {
	a, b = strings.TrimRight(a, "\n"), strings.TrimRight(b, "\n")
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "\n" + b
	}
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
	scope := r.URL.Query().Get("scope")
	if scope == "last-turn" {
		res := s.runAR(r.Context(), 30*time.Second, "diff", id, "--scope", "last-turn", "--json")
		if res.Err != nil {
			http.Error(w, strings.TrimSpace(res.Stderr), http.StatusBadGateway)
			return
		}
		var resp map[string]any
		if err := json.Unmarshal([]byte(res.Stdout), &resp); err != nil {
			http.Error(w, "invalid diff response from agentrunner", http.StatusBadGateway)
			return
		}
		workspace, _ := resp["workspace"].(string)
		resp["known"] = workspace != ""
		// Newer `ar diff` projections classify baseline-new large/binary files
		// as name-only and generated paths as hidden. Preserve those fields;
		// an older shared binary omitted them, so retain the stable empty/zero
		// compatibility shape instead of returning null to the frontend.
		if _, ok := resp["untracked"]; !ok {
			resp["untracked"] = []string{}
		}
		if _, ok := resp["hiddenUntracked"]; !ok {
			resp["hiddenUntracked"] = 0
		}
		if _, ok := resp["untrackedReasons"]; !ok {
			resp["untrackedReasons"] = map[string]string{}
		}
		// `ar diff --json` carries no repo fields, but every consumer of this
		// endpoint gates on isRepo/nested (the changes card's turn/workspace
		// split, qa/consistency). Leaving them absent made every last-turn
		// response read as "not a repo", so the Edited-N-files card silently
		// fell back to "Changes in workspace" on all turn edits (QA-0719).
		resp["isRepo"] = false
		resp["nested"] = false
		if workspace != "" {
			if top, insideRepo := git(r.Context(), workspace, "rev-parse", "--show-toplevel"); insideRepo {
				if root := strings.TrimSpace(top); samePath(root, workspace) {
					resp["isRepo"] = true
				} else {
					resp["nested"] = true
					resp["repoRoot"] = root
				}
			}
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if scope != "" && scope != "working-tree" {
		http.Error(w, "scope must be working-tree or last-turn", http.StatusBadRequest)
		return
	}
	meta := s.meta.get(id)
	resp := map[string]any{
		"scope": "working-tree", "workspace": meta.Workspace, "known": meta.Workspace != "",
		"isRepo": false, "nested": false, "repoRoot": "", "diff": "", "numstat": "",
		"untracked": []string{}, "untrackedReasons": map[string]string{}, "hiddenUntracked": 0,
		"worktree": false, "mainRepo": "", "branch": "",
	}
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
	// Staged changes are workspace changes. Bare `git diff` sees only
	// unstaged edits, and the untracked scan below only `?? ` lines — so a
	// staged file (porcelain `A `/`M `) vanished from the WHOLE surface:
	// rail said "Nothing to commit" while `git commit` would have committed
	// it (QA-0719 091500 用户真机实测:git add 4 文件后 Changes 全空、
	// Commit or push 灰死)。`git diff HEAD` covers staged+unstaged in one
	// pass; on an unborn branch (a scratch repo before its first commit —
	// exactly the 091500 case) HEAD is invalid, so fall back to
	// `git diff --cached` (index vs the empty tree) joined with the
	// unstaged diff.
	var diff, numstat string
	if _, hasHead := git(r.Context(), meta.Workspace, "rev-parse", "--verify", "-q", "HEAD"); hasHead {
		diff, _ = git(r.Context(), meta.Workspace, "diff", "HEAD")
		numstat, _ = git(r.Context(), meta.Workspace, "diff", "HEAD", "--numstat")
	} else {
		staged, _ := git(r.Context(), meta.Workspace, "diff", "--cached")
		unstaged, _ := git(r.Context(), meta.Workspace, "diff")
		diff = joinDiffText(staged, unstaged)
		stagedNum, _ := git(r.Context(), meta.Workspace, "diff", "--cached", "--numstat")
		unstagedNum, _ := git(r.Context(), meta.Workspace, "diff", "--numstat")
		numstat = joinDiffText(stagedNum, unstagedNum)
	}
	resp["diff"] = diff
	resp["numstat"] = numstat
	if porcelain, ok := git(r.Context(), meta.Workspace, "status", "--porcelain", "--untracked-files=all"); ok {
		untracked := []string{} // never nil — the UI does .length on this
		untrackedReasons := map[string]string{}
		var extra bytes.Buffer // synthetic new-file diffs for untracked content
		hiddenUntracked := 0
		inlineFiles := 0
		const maxInlineUntrackedBytes = 1 << 20
		for _, line := range strings.Split(porcelain, "\n") {
			if !strings.HasPrefix(line, "?? ") {
				continue
			}
			path := strings.TrimSpace(line[3:])
			if hiddenUntrackedPath(path) {
				hiddenUntracked++
				continue
			}
			if len(untracked)+inlineFiles >= 500 {
				hiddenUntracked++
				continue
			}
			// Inline the content of small, regular, text files as a new-file
			// diff so the UI shows it (git diff omits untracked entirely).
			full := filepath.Join(meta.Workspace, path)
			if info, err := os.Stat(full); err == nil && info.Mode().IsRegular() && info.Size() <= 256*1024 {
				if content, err := os.ReadFile(full); err == nil && !bytes.Contains(content, []byte{0}) {
					if extra.Len()+len(content) > maxInlineUntrackedBytes {
						untracked = append(untracked, path)
						continue
					}
					lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
					fmt.Fprintf(&extra, "diff --git a/%s b/%s\nnew file\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n",
						path, path, path, len(lines))
					for _, l := range lines {
						extra.WriteString("+" + l + "\n")
					}
					inlineFiles++
					continue
				}
			}
			untracked = append(untracked, path) // binary / large / unreadable: name only
			if info, err := os.Stat(full); err != nil || !info.Mode().IsRegular() {
				untrackedReasons[path] = "unavailable"
			} else if info.Size() > 256*1024 {
				untrackedReasons[path] = "large"
			} else if content, err := os.ReadFile(full); err != nil {
				untrackedReasons[path] = "unavailable"
			} else if bytes.Contains(content, []byte{0}) {
				untrackedReasons[path] = "binary"
			}
		}
		if extra.Len() > 0 {
			d, _ := resp["diff"].(string)
			if d != "" && !strings.HasSuffix(d, "\n") {
				d += "\n"
			}
			resp["diff"] = d + extra.String()
		}
		resp["untracked"] = untracked
		resp["untrackedReasons"] = untrackedReasons
		resp["hiddenUntracked"] = hiddenUntracked
	}
	writeJSON(w, http.StatusOK, resp)
}

func hiddenUntrackedPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		switch part {
		// "venv" (no dot) and site-packages: QA-0719 S8 — an agent-created
		// bare `venv/` slipped past this filter and 655 pip files were inlined
		// as synthetic diffs, exploding the Edited card, eating the 1MB inline
		// budget, and rendering LICENSE.txt/AUTHORS.txt as artifact cards.
		case ".git", ".venv", "venv", "site-packages", ".tox", ".eggs", ".cache", ".next", ".turbo", ".gradle", "node_modules", "vendor", "dist", "build", "out", "target", "coverage", "__pycache__":
			return true
		}
		if strings.HasSuffix(part, ".dist-info") || strings.HasSuffix(part, ".egg-info") {
			return true
		}
	}
	return false
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
	// Committing a clean tree isn't an error the user should see as a scary red
	// blob — it's a no-op. Report it as a friendly 200 (phone report class).
	if err != nil && (strings.Contains(out, "nothing to commit") || strings.Contains(out, "no changes added")) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "Nothing to commit — the working tree is clean."})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "git commit failed", "stderr": out})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": out})
}

// handlePush pushes the session workspace's current branch to its upstream (or
// origin) — the "push" half of Codex's "Commit or push". Failures are returned
// as STRUCTURED JSON ({error, stderr, kind}) so the UI can explain them instead
// of surfacing raw git prose: kind is one of no-remote, no-upstream, detached,
// rejected (non-fast-forward), auth, or failed. GIT_TERMINAL_PROMPT=0 keeps a
// credential-less remote from hanging the request on an interactive prompt.
func (s *server) handlePush(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
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
	if root := strings.TrimSpace(top); !samePath(root, ws) {
		badRequest(w, "workspace is inside another repository ("+root+"), refusing to push from there")
		return
	}
	branch, _ := git(r.Context(), ws, "rev-parse", "--abbrev-ref", "HEAD")
	branch = strings.TrimSpace(branch)
	if branch == "" || branch == "HEAD" {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "the workspace is on a detached HEAD — check out a branch before pushing",
			"kind":  "detached",
		})
		return
	}
	remotes, _ := git(r.Context(), ws, "remote")
	remoteList := strings.Fields(remotes)
	if len(remoteList) == 0 {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "this repository has no git remote configured — add one (e.g. `git remote add origin <url>`) before pushing",
			"kind":  "no-remote",
		})
		return
	}
	run := func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", ws}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	upstream, hasUpstream := git(r.Context(), ws, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	upstream = strings.TrimSpace(upstream)
	var out string
	var err error
	if !hasUpstream || upstream == "" {
		// No tracking branch yet: pick origin when present, else the sole/first
		// remote, and set upstream while pushing so subsequent pushes are bare.
		remote := remoteList[0]
		for _, rm := range remoteList {
			if rm == "origin" {
				remote = "origin"
				break
			}
		}
		out, err = run("push", "--set-upstream", remote, branch)
	} else {
		out, err = run("push")
	}
	if err != nil {
		low := strings.ToLower(out)
		kind := "failed"
		switch {
		case strings.Contains(low, "non-fast-forward") || strings.Contains(low, "fetch first") ||
			strings.Contains(low, "! [rejected]") || strings.Contains(low, "updates were rejected"):
			kind = "rejected"
		case strings.Contains(low, "has no upstream") || strings.Contains(low, "no upstream branch"):
			kind = "no-upstream"
		case strings.Contains(low, "could not read from remote") || strings.Contains(low, "repository not found") ||
			strings.Contains(low, "does not appear to be a git repository") || strings.Contains(low, "no such remote"):
			kind = "no-remote"
		case strings.Contains(low, "authentication failed") || strings.Contains(low, "permission denied") ||
			strings.Contains(low, "could not read username") || strings.Contains(low, "terminal prompts disabled"):
			kind = "auth"
		}
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "git push failed", "stderr": out, "kind": kind})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": out, "branch": branch})
}

// handleRevert discards the session's working-tree changes back to HEAD — the
// "Undo ↺" action on the change card. DESTRUCTIVE: it drops uncommitted edits
// and deletes the untracked files the agent introduced, so the frontend guards
// it behind an explicit confirm. An optional {path} scopes it to one file
// (per-row discard); absent, it reverts the whole workspace. Nested workspaces
// are refused so we never touch a parent repository's tree.
func (s *server) handleRevert(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if !readBody(w, r, &req) {
		return
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
	if root := strings.TrimSpace(top); !samePath(root, ws) {
		badRequest(w, "workspace is inside another repository ("+root+"), refusing to discard changes there")
		return
	}
	if path := strings.TrimSpace(req.Path); path != "" {
		clean := filepath.Clean(path)
		if filepath.IsAbs(path) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			badRequest(w, "path must be a file relative to the session workspace")
			return
		}
		// Unstage, restore tracked content from HEAD, then clean it if untracked.
		// checkout fails for a purely-untracked file, so the clean is what removes
		// a newly-created one — both are best-effort and the pathspec confines them.
		_, _ = gitIn(r.Context(), ws, nil, "reset", "-q", "--", clean)
		_, _ = gitIn(r.Context(), ws, nil, "checkout", "HEAD", "--", clean)
		if out, err := gitIn(r.Context(), ws, nil, "clean", "-fd", "--", clean); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "git clean failed", "stderr": out})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "discarded " + clean})
		return
	}
	// Whole workspace: unstage anything staged, restore all tracked files to HEAD,
	// then delete untracked files and dirs the change set introduced.
	_, _ = gitIn(r.Context(), ws, nil, "reset", "-q")
	if out, err := gitIn(r.Context(), ws, nil, "checkout", "--", "."); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "git checkout failed", "stderr": out})
		return
	}
	if out, err := gitIn(r.Context(), ws, nil, "clean", "-fd"); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "git clean failed", "stderr": out})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reverted"})
}

// handleBlob returns one workspace file's current text as {lines:[…]} so the
// Changes view can reveal the unmodified lines hidden between diff hunks (the
// "N unmodified lines" collapser bands). Same workspace jail as
// handleSessionFile — no absolute paths, traversal, directories, symlink
// escapes, oversized or binary files.
func (s *server) handleBlob(w http.ResponseWriter, r *http.Request) {
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
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	const maxBlob = 8 << 20
	if info.Size() > maxBlob {
		http.Error(w, "file is too large to expand", http.StatusRequestEntityTooLarge)
		return
	}
	content, err := os.ReadFile(target)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if bytes.Contains(content, []byte{0}) {
		badRequest(w, "file is not text")
		return
	}
	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
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
