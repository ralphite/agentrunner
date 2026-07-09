package main

import (
	"bytes"
	"context"
	"net/http"
	"os/exec"
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
		for _, line := range strings.Split(porcelain, "\n") {
			if strings.HasPrefix(line, "?? ") {
				untracked = append(untracked, strings.TrimSpace(line[3:]))
			}
		}
		resp["untracked"] = untracked
	}
	writeJSON(w, http.StatusOK, resp)
}
