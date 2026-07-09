package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// sessionMeta is what arwebui knows about a session that the CLI does not
// surface: the workspace it was launched against (for the Diff view) and the
// opening task text (for Codex-style task-card titles). It is best-effort —
// populated when *we* create a session; empty for externally-created ones.
type sessionMeta struct {
	Workspace string `json:"workspace"`
	Title     string `json:"title"`
}

type metaStore struct {
	mu sync.Mutex
	m  map[string]sessionMeta
}

func newMetaStore() *metaStore { return &metaStore{m: map[string]sessionMeta{}} }

func (s *metaStore) set(sid, ws, title string) {
	if sid == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.m[sid]
	if ws != "" {
		cur.Workspace = ws
	}
	if title != "" && cur.Title == "" {
		cur.Title = firstLine(title, 100)
	}
	s.m[sid] = cur
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

// handleDiff returns the workspace diff for a session — the closest analog to
// Codex's changed-files view. Tracked changes come from `git diff`; untracked
// files are listed from `git status --porcelain` and appended as synthetic
// "new file" diffs so the UI shows them too.
func (s *server) handleDiff(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	meta := s.meta.get(id)
	resp := map[string]any{"workspace": meta.Workspace, "known": meta.Workspace != "", "isRepo": false, "diff": "", "numstat": "", "untracked": []string{}}
	if meta.Workspace == "" {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if _, isRepo := git(r.Context(), meta.Workspace, "rev-parse", "--is-inside-work-tree"); !isRepo {
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
	if _, isRepo := git(r.Context(), ws, "rev-parse", "--is-inside-work-tree"); !isRepo {
		badRequest(w, "workspace is not a git repository")
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
