package main

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// handleCompact folds the session's context into a summary (G7 · INC-6):
// `ar compact <sid>`. Exposed to the composer as the /compact slash command.
func (s *server) handleCompact(w http.ResponseWriter, r *http.Request) {
	s.oneShotHandler("ar compact", func(id string) []string { return []string{"compact", id} })(w, r)
}

// handleClear drops the session's context prefix (G7 · INC-6):
// `ar clear <sid>`. Exposed to the composer as the /clear slash command.
func (s *server) handleClear(w http.ResponseWriter, r *http.Request) {
	s.oneShotHandler("ar clear", func(id string) []string { return []string{"clear", id} })(w, r)
}

// handleGoal drives an in-session goal (INC-D1): attach/update/pause/resume/
// cancel via `ar goal <sid> <action> …`. The goal hangs on the conversational
// session and its context continues across the verifier's checks.
func (s *server) handleGoal(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	var req struct {
		Action    string `json:"action"` // attach | update | pause | resume | cancel
		Goal      string `json:"goal"`
		Verifier  string `json:"verifier"`
		MaxChecks int    `json:"maxChecks"`
	}
	if !readBody(w, r, &req) {
		return
	}
	args := []string{"goal", id, req.Action}
	switch req.Action {
	case "attach", "update":
		if req.Action == "attach" && strings.TrimSpace(req.Goal) == "" {
			badRequest(w, "goal statement is required")
			return
		}
		if v := strings.TrimSpace(req.Verifier); v != "" {
			args = append(args, "--verify", v)
		}
		if req.MaxChecks > 0 {
			args = append(args, "--max-checks", strconv.Itoa(req.MaxChecks))
		}
		if g := strings.TrimSpace(req.Goal); g != "" {
			args = append(args, g)
		}
	case "pause", "resume", "cancel":
		// verb-only
	default:
		badRequest(w, "action must be attach|update|pause|resume|cancel")
		return
	}
	res := s.runAR(r.Context(), oneShotTimeout, args...)
	if res.Err != nil {
		arFail(w, "ar goal", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(res.Stdout)})
}

// ---- workspace git (branch picker) ----
//
// The cockpit drives a workspace the same way a user with a terminal would;
// listing/switching branches is an ordinary git operation on that directory,
// not a product concept. These endpoints shell out to git directly (like the
// Diff view already does via meta.go's git()) and never touch the ar CLI.

// branchName is the same conservative charset git ref names live in; it keeps a
// crafted `dir`/`branch` from turning into flags or path traversal. git args are
// passed as discrete argv entries so there is no shell to inject into, but a
// name starting with '-' could still be read as a flag — reject those.
func validBranchName(b string) bool {
	b = strings.TrimSpace(b)
	if b == "" || strings.HasPrefix(b, "-") || len(b) > 200 {
		return false
	}
	for _, c := range b {
		if c <= ' ' || c == '~' || c == '^' || c == ':' || c == '?' || c == '*' || c == '[' || c == '\\' {
			return false
		}
	}
	// git also forbids these sequences in ref names.
	if strings.Contains(b, "..") || strings.HasSuffix(b, "/") || strings.HasSuffix(b, ".lock") {
		return false
	}
	return true
}

func absDir(w http.ResponseWriter, raw string) (string, bool) {
	dir := strings.TrimSpace(raw)
	if dir == "" {
		badRequest(w, "dir is required")
		return "", false
	}
	if !filepath.IsAbs(dir) {
		badRequest(w, "dir must be an absolute path")
		return "", false
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		badRequest(w, "not an existing directory: "+dir)
		return "", false
	}
	return dir, true
}

// handleGitBranches lists the workspace's branches for the branch picker, plus
// the current branch and a dirty-file count (mirrors Codex's "Uncommitted: N
// files" hint). A non-repo directory returns isRepo:false, not an error, so the
// picker can simply hide itself.
func (s *server) handleGitBranches(w http.ResponseWriter, r *http.Request) {
	dir, ok := absDir(w, r.URL.Query().Get("dir"))
	if !ok {
		return
	}
	resp := map[string]any{"isRepo": false, "current": "", "branches": []string{}, "dirty": 0}
	if _, isRepo := git(r.Context(), dir, "rev-parse", "--is-inside-work-tree"); !isRepo {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp["isRepo"] = true
	// `branch --show-current` is intentionally empty for detached worktrees.
	// `rev-parse --abbrev-ref HEAD` returns the misleading literal "HEAD".
	if cur, ok := git(r.Context(), dir, "branch", "--show-current"); ok {
		resp["current"] = strings.TrimSpace(cur)
	}
	branches := []string{}
	if out, ok := git(r.Context(), dir, "for-each-ref", "--format=%(refname:short)", "--sort=-committerdate", "refs/heads"); ok {
		for _, line := range strings.Split(out, "\n") {
			if b := strings.TrimSpace(line); b != "" {
				branches = append(branches, b)
			}
		}
	}
	resp["branches"] = branches
	if porcelain, ok := git(r.Context(), dir, "status", "--porcelain"); ok {
		n := 0
		for _, line := range strings.Split(porcelain, "\n") {
			if strings.TrimSpace(line) != "" {
				n++
			}
		}
		resp["dirty"] = n
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGitCheckout switches the workspace to a branch (optionally creating it),
// so a new task can start on the branch the user picked — the desktop-Codex
// "branch" affordance. Write path, so it does its own git exec (git() is
// read-only-by-convention and swallows errors).
func (s *server) handleGitCheckout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dir    string `json:"dir"`
		Branch string `json:"branch"`
		Create bool   `json:"create"`
	}
	if !readBody(w, r, &req) {
		return
	}
	dir, ok := absDir(w, req.Dir)
	if !ok {
		return
	}
	if !validBranchName(req.Branch) {
		badRequest(w, "invalid branch name")
		return
	}
	if _, isRepo := git(r.Context(), dir, "rev-parse", "--is-inside-work-tree"); !isRepo {
		badRequest(w, "not a git repository: "+dir)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	args := []string{"-C", dir, "checkout"}
	if req.Create {
		args = append(args, "-b")
	}
	args = append(args, req.Branch)
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":  "git checkout failed",
			"stderr": strings.TrimSpace(string(out)),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(string(out)), "branch": req.Branch})
}
