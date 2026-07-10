package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	resp := map[string]any{"workspace": meta.Workspace, "known": meta.Workspace != "", "isRepo": false, "nested": false, "repoRoot": "", "diff": "", "numstat": "", "untracked": []string{}}
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
	diff, _ := git(r.Context(), meta.Workspace, "diff")
	numstat, _ := git(r.Context(), meta.Workspace, "diff", "--numstat")
	resp["diff"] = diff
	resp["numstat"] = numstat
	if porcelain, ok := git(r.Context(), meta.Workspace, "status", "--porcelain"); ok {
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
