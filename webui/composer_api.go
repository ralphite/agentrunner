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
// `ar compact <sid> [directive]`. Exposed to the composer as the /compact
// slash command, including the optional focus directive.
func (s *server) handleCompact(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	var req struct {
		Directive string `json:"directive"`
	}
	if !readBody(w, r, &req) {
		return
	}
	args := []string{"compact", id}
	if d := strings.TrimSpace(req.Directive); d != "" {
		args = append(args, d)
	}
	res := s.runAR(r.Context(), oneShotTimeout, args...)
	if res.Err != nil {
		arFail(w, "ar compact", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(res.Stdout)})
}

// handleClear drops the session's context prefix (G7 · INC-6):
// `ar clear <sid>`. Exposed to the composer as the /clear slash command.
func (s *server) handleClear(w http.ResponseWriter, r *http.Request) {
	s.oneShotHandler("ar clear", func(id string) []string { return []string{"clear", id} })(w, r)
}

// handleMode switches the session's permission mode at its next safe boundary
// (INC-42, G29): `ar mode <sid> <default|acceptEdits>`. Exposed to the
// composer as the /mode slash command. Runtime switching covers the
// user-sovereignty pair only — plan exits via exit_plan_mode approval and
// bypass is a start-time choice — and the loop owns final validity; this
// handler pre-rejects only the obviously invalid.
func (s *server) handleMode(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if !readBody(w, r, &req) {
		return
	}
	m := strings.TrimSpace(req.Mode)
	if m != "default" && m != "acceptEdits" {
		badRequest(w, "mode must be default|acceptEdits (plan and bypass are start-time choices)")
		return
	}
	res := s.runAR(r.Context(), oneShotTimeout, "mode", id, m)
	if res.Err != nil {
		arFail(w, "ar mode", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(res.Stdout)})
}

// helperTimeout bounds a composer-helper provider call (dictate/optimize).
// Longer than the plain oneShotTimeout because a transcription or rewrite is a
// real model round-trip that can be slower than a local daemon command.
const helperTimeout = 120 * time.Second

// handleDictate transcribes an uploaded audio recording via `ar dictate`
// (INC-56, HANDA-PARITY #18). The webui stays a thin shell: it uploads the
// recording (existing /api/upload), then hands the path to the `ar` command,
// which owns the provider call and credential resolution (DESIGN §12:1075 +
// 决策 #15c). The browser never talks to a provider. The transcript is a
// composer text convenience — it lands in the textarea as an ordinary prompt.
func (s *server) handleDictate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`    // an /api/upload path
		Context string `json:"context"` // optional disambiguation hint (session/draft text)
	}
	if !readBody(w, r, &req) {
		return
	}
	// Only files THIS server produced (the uploads dir) may be transcribed: the
	// path becomes a positional arg to `ar dictate`, so we never let a crafted
	// path steer the webui into reading an arbitrary file off disk.
	uploads := filepath.Join(s.runtimeDir, "uploads")
	clean := filepath.Clean(strings.TrimSpace(req.Path))
	if clean == "." || !underDir(uploads, clean) {
		badRequest(w, "audio must be an uploaded file")
		return
	}
	if st, err := os.Stat(clean); err != nil || st.IsDir() {
		badRequest(w, "audio not readable")
		return
	}
	args := []string{"dictate"}
	if c := strings.TrimSpace(req.Context); c != "" {
		args = append(args, "--context", c)
	}
	args = append(args, clean)
	res := s.runAR(r.Context(), helperTimeout, args...)
	if res.Err != nil {
		arFail(w, "ar dictate", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": strings.TrimSpace(res.Stdout)})
}

// handleOptimize rewrites a draft prompt via `ar optimize` (INC-56,
// HANDA-PARITY #19) — same thin-shell discipline as dictate. The original
// draft is never mutated server-side; the frontend keeps it for a single-step
// undo. The draft rides after a "--" so a draft that happens to start with "-"
// is never mistaken for a flag.
func (s *server) handleOptimize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Draft   string `json:"draft"`
		Context string `json:"context"`
	}
	if !readBody(w, r, &req) {
		return
	}
	draft := strings.TrimSpace(req.Draft)
	if draft == "" {
		badRequest(w, "draft is required")
		return
	}
	args := []string{"optimize"}
	if c := strings.TrimSpace(req.Context); c != "" {
		args = append(args, "--context", c)
	}
	args = append(args, "--", draft)
	res := s.runAR(r.Context(), helperTimeout, args...)
	if res.Err != nil {
		arFail(w, "ar optimize", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": strings.TrimSpace(res.Stdout)})
}

// underDir reports whether path is dir itself or a file inside it. Both are
// cleaned/absolute already; a "../" escape resolves to a Rel starting with "..".
func underDir(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
	resp := map[string]any{"isRepo": false, "current": "", "branches": []string{}, "dirty": 0, "hasCommits": false}
	if _, isRepo := git(r.Context(), dir, "rev-parse", "--is-inside-work-tree"); !isRepo {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp["isRepo"] = true
	// A fresh `git init` sits on an UNBORN branch: `branch --show-current`
	// still reports "master"/"main", but that ref names no commit, so a
	// worktree at it fails with git's raw "invalid starting ref" (phone
	// report 2026-07-12). hasCommits is the authoritative "can make a
	// worktree" signal — HEAD resolves to a commit — so the picker can guard
	// the option instead of letting the user hit the scary git error.
	_, hasCommits := git(r.Context(), dir, "rev-parse", "--verify", "--quiet", "HEAD")
	resp["hasCommits"] = hasCommits
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
// so a new session can start on the branch the user picked — the desktop-Codex
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
		// Classify the common cases into friendly guidance instead of raw git
		// plumbing prose (phone report class).
		raw := strings.TrimSpace(string(out))
		msg := "Couldn’t switch branch."
		switch {
		case strings.Contains(raw, "already exists"):
			msg = "A branch named “" + req.Branch + "” already exists — pick it from the list instead of creating it."
		case strings.Contains(raw, "would be overwritten") || strings.Contains(raw, "Please commit your changes") || strings.Contains(raw, "local changes"):
			msg = "You have uncommitted changes — commit or discard them before switching branch."
		case strings.Contains(raw, "did not match") || strings.Contains(raw, "invalid reference") || strings.Contains(raw, "pathspec"):
			msg = "No branch named “" + req.Branch + "”."
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": msg, "stderr": raw})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(string(out)), "branch": req.Branch})
}
