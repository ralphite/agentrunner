package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const oneShotTimeout = 60 * time.Second

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()

	// ---- API ----
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/daemon/start", s.handleDaemonStart)
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("POST /api/sessions", s.handleNewSession)
	mux.HandleFunc("POST /api/workspace", s.handleWorkspace)
	mux.HandleFunc("POST /api/worktree", s.handleWorktree)
	mux.HandleFunc("POST /api/upload", s.handleUpload)
	mux.HandleFunc("GET /api/uploads/{name}", s.handleServeUpload)
	mux.HandleFunc("POST /api/trust", s.handleTrust)

	mux.HandleFunc("GET /api/sessions/{sid}/events", s.handleEvents)
	mux.HandleFunc("GET /api/sessions/{sid}/state", s.handleState)
	mux.HandleFunc("GET /api/sessions/{sid}/inspect", s.handleInspect)
	mux.HandleFunc("GET /api/sessions/{sid}/artifact", s.handleArtifact)
	mux.HandleFunc("GET /api/sessions/{sid}/ps", s.handlePS)
	mux.HandleFunc("GET /api/sessions/{sid}/barriers", s.handleBarriers)
	mux.HandleFunc("POST /api/sessions/{sid}/barrier", s.handleBarrier)
	mux.HandleFunc("GET /api/sessions/{sid}/diff", s.handleDiff)
	mux.HandleFunc("GET /api/sessions/{sid}/files", s.handleFiles)
	mux.HandleFunc("GET /api/sessions/{sid}/file", s.handleSessionFile)
	mux.HandleFunc("POST /api/sessions/{sid}/commit", s.handleCommit)
	mux.HandleFunc("POST /api/sessions/{sid}/git-init", s.handleGitInit)
	mux.HandleFunc("GET /api/sessions/{sid}/stream", s.handleStream)

	mux.HandleFunc("POST /api/sessions/{sid}/send", s.handleSend)
	mux.HandleFunc("POST /api/sessions/{sid}/interrupt", s.handleInterrupt)
	mux.HandleFunc("POST /api/sessions/{sid}/resume", s.handleResume)
	mux.HandleFunc("POST /api/sessions/{sid}/retry", s.handleRetry)
	mux.HandleFunc("POST /api/sessions/{sid}/close", s.handleClose)
	mux.HandleFunc("POST /api/sessions/{sid}/stop", s.handleStop)
	mux.HandleFunc("POST /api/sessions/{sid}/kill", s.handleKill)
	mux.HandleFunc("POST /api/sessions/{sid}/approve", s.handleApprove)
	mux.HandleFunc("POST /api/sessions/{sid}/agent", s.handleAgentSwitch)
	mux.HandleFunc("POST /api/sessions/{sid}/fork", s.handleFork)
	mux.HandleFunc("POST /api/sessions/{sid}/compact", s.handleCompact)
	mux.HandleFunc("POST /api/sessions/{sid}/clear", s.handleClear)
	mux.HandleFunc("POST /api/sessions/{sid}/mode", s.handleMode)
	mux.HandleFunc("POST /api/sessions/{sid}/goal", s.handleGoal)

	// ---- workspace git (branch picker; cockpit-level, operates on the
	// session/new-task workspace exactly as a user would) ----
	mux.HandleFunc("GET /api/git/branches", s.handleGitBranches)
	mux.HandleFunc("POST /api/git/checkout", s.handleGitCheckout)

	// ---- background runs (submit / drive) ----
	mux.HandleFunc("GET /api/runs", s.handleRunsList)
	mux.HandleFunc("POST /api/runs", s.handleRunStart)
	mux.HandleFunc("GET /api/runs/{rid}/stream", s.handleRunStream)
	mux.HandleFunc("POST /api/runs/{rid}/stop", s.handleRunStop)

	// ---- static SPA ----
	mux.Handle("/", staticHandler())
	return mux
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// arFail maps a failed ar invocation onto the error convention: 502 with
// {error, stderr} so the UI can show the real failure.
func arFail(w http.ResponseWriter, what string, res arResult) {
	msg := what + " failed"
	if res.Err != nil {
		msg = fmt.Sprintf("%s: %v", what, res.Err)
	}
	// Some subcommands (notably `ar new`) print the actionable diagnostic to
	// stdout while stderr only carries the session id — surface both so the
	// UI toast shows the real reason.
	detail := strings.TrimSpace(res.Stderr)
	if out := strings.TrimSpace(res.Stdout); out != "" && out != detail {
		if detail != "" {
			detail += "\n"
		}
		detail += out
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{
		"error":  msg,
		"stderr": detail,
	})
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}

func sid(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("sid")
	if !validID(id) {
		badRequest(w, "invalid session id")
		return "", false
	}
	return id, true
}

func readBody(w http.ResponseWriter, r *http.Request, v any) bool {
	// Require an application/json Content-Type (INC-D1 review F2): a cross-origin
	// "simple request" (text/plain) needs no CORS preflight and would let a
	// malicious page the user visits drive this loopback server (e.g. attach a
	// goal whose verifier runs a shell command). Requiring application/json
	// forces a preflight the no-CORS server never answers, so the browser blocks
	// it — hardening every JSON endpoint (send/goal/git/…), not just this one.
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		badRequest(w, "Content-Type must be application/json")
		return false
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(v); err != nil {
		badRequest(w, "bad JSON body: "+err.Error())
		return false
	}
	return true
}

// writeSpecDir materialises a base.yaml plus any sibling specs into a fresh
// runtime/specs dir and returns the base.yaml path (CLI resolves siblings
// relative to it).
func (s *server) writeSpecDir(spec string, extras []specFile) (string, string, error) {
	specDir := filepath.Join(s.runtimeDir, "specs", fmt.Sprintf("s%d", time.Now().UnixNano()))
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(specDir, "base.yaml"), []byte(spec), 0o644); err != nil {
		return "", "", err
	}
	for _, ex := range extras {
		name := filepath.Base(strings.TrimSpace(ex.Name))
		if !specFileName.MatchString(name) || name == "base.yaml" {
			return "", "", fmt.Errorf("bad extra spec name: %s", ex.Name)
		}
		if err := os.WriteFile(filepath.Join(specDir, name), []byte(ex.Content), 0o644); err != nil {
			return "", "", err
		}
	}
	return specDir, filepath.Join(specDir, "base.yaml"), nil
}

type specFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// ---- daemon / health ----

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ver := "unknown"
	if res := s.runAR(r.Context(), 5*time.Second, "--version"); res.Err == nil {
		ver = strings.TrimSpace(res.Stdout)
	}
	managed, reachable, external := s.daemonStatus(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"ar":              s.arPath,
		"version":         ver,
		"daemonManaged":   managed,
		"daemonExternal":  external,
		"daemonUp":        reachable,
		"runtimeDir":      s.runtimeDir,
		"daemonLogPath":   filepath.Join(s.runtimeDir, "daemon.log"),
		"manageRequested": s.daemonManage,
	})
}

func (s *server) handleDaemonStart(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	alive := s.daemonAlive
	s.mu.Unlock()
	if alive {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already-running"})
		return
	}
	if err := s.spawnDaemon(); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (s *server) handleTrust(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dir string `json:"dir"`
	}
	if !readBody(w, r, &req) {
		return
	}
	dir := strings.TrimSpace(req.Dir)
	if dir == "" {
		badRequest(w, "dir is required")
		return
	}
	// Require an absolute path — don't resolve a relative one against arwebui's
	// CWD (that both surprises the user and leaks the server's directory).
	if !filepath.IsAbs(dir) {
		badRequest(w, "please provide an absolute path (starting with /): "+dir)
		return
	}
	res := s.runAR(r.Context(), oneShotTimeout, "trust", dir)
	if res.Err != nil {
		arFail(w, "ar trust", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(res.Stdout)})
}

// ---- sessions ----

func (s *server) handleSessions(w http.ResponseWriter, r *http.Request) {
	type row struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		Turns     int    `json:"turns"`
		Title     string `json:"title"`
		Workspace string `json:"workspace"`
		Kind      string `json:"kind"`
		Schedule  string `json:"schedule,omitempty"`
	}
	// Runtime metadata is authoritative for every session, including sessions
	// created by the CLI or another UI. The local meta store remains a fallback
	// for older AgentRunner binaries and preserves WebUI-created titles.
	res := s.runAR(r.Context(), 15*time.Second, "sessions", "list", "--json")
	rows := []row{}
	if res.Err == nil {
		if err := json.Unmarshal([]byte(res.Stdout), &rows); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "decode ar sessions --json: " + err.Error()})
			return
		}
		discovered := make(map[string]sessionMeta, len(rows))
		for _, runtimeRow := range rows {
			discovered[runtimeRow.ID] = sessionMeta{Workspace: runtimeRow.Workspace, Title: runtimeRow.Title}
		}
		s.meta.merge(discovered)
		for i := range rows {
			meta := s.meta.get(rows[i].ID)
			if meta.Title != "" {
				rows[i].Title = meta.Title
			} else if rows[i].Title == "" {
				rows[i].Title = titleFromID(rows[i].ID)
			}
			if rows[i].Workspace == "" {
				rows[i].Workspace = meta.Workspace
			}
		}
		writeJSON(w, http.StatusOK, rows)
		return
	}

	// Compatibility fallback for an older `ar` selected with --ar.
	res = s.runAR(r.Context(), 15*time.Second, "sessions", "list")
	if res.Err != nil {
		arFail(w, "ar sessions list", res)
		return
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || f[0] == "SESSION" || line == "no sessions" {
			continue
		}
		turns, _ := strconv.Atoi(f[len(f)-1])
		m := s.meta.get(f[0])
		title := m.Title
		if title == "" { // CLI-created session arwebui never saw (UX-03)
			title = titleFromID(f[0])
		}
		rows = append(rows, row{
			ID: f[0], Status: strings.Join(f[1:len(f)-1], " "), Turns: turns,
			Title: title, Workspace: m.Workspace, Kind: "session",
		})
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *server) handleNewSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Spec       string     `json:"spec"`
		ExtraSpecs []specFile `json:"extraSpecs"`
		Workspace  string     `json:"workspace"`
		Message    string     `json:"message"`
		Mode       string     `json:"mode"`
	}
	if !readBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Spec) == "" || strings.TrimSpace(req.Workspace) == "" ||
		strings.TrimSpace(req.Message) == "" {
		badRequest(w, "spec, workspace and message are required")
		return
	}
	ws, err := filepath.Abs(req.Workspace)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	if st, err := os.Stat(ws); err != nil || !st.IsDir() {
		badRequest(w, "workspace is not an existing directory: "+ws)
		return
	}
	specDir, basePath, err := s.writeSpecDir(req.Spec, req.ExtraSpecs)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	// --detach: return the session id immediately and let the daemon host the
	// opening turn. Blocking here would (a) hang until the turn idles and (b)
	// orphan any opening-turn approval when the request times out — the ask
	// belongs to the dying connection. Detached, the session lives in the
	// daemon and its approvals are answerable via the shared broker.
	args := []string{"new", "--detach", "--workspace", ws}
	if req.Mode != "" {
		args = append(args, "--mode", req.Mode)
	}
	args = append(args, basePath, req.Message)
	res := s.runAR(r.Context(), oneShotTimeout, args...)
	if res.Err != nil {
		arFail(w, "ar new", res)
		return
	}
	id := parseSessionID(res)
	if id == "" {
		arFail(w, "ar new (no session id)", res)
		return
	}
	// --detach prints an id and exits 0 even when the daemon then rejects the
	// spec (bad tool/model) — the session never registers. Confirm it actually
	// exists so a bad spec is a clear error, not a phantom the UI opens into.
	exists := false
	for i := 0; i < 5; i++ {
		if s.sessionExists(r.Context(), id) {
			exists = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !exists {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":  "session creation failed: the daemon rejected this spec (check tools / model / permissions)",
			"stderr": strings.TrimSpace(res.Stdout + "\n" + res.Stderr),
		})
		return
	}
	s.meta.set(id, ws, req.Message)
	writeJSON(w, http.StatusOK, map[string]string{"sid": id, "specDir": specDir, "workspace": ws})
}

// newRuntimeDir picks a fresh, human-readable directory name under
// runtime/<kind>/ — "ws-20260710-221530" reads as a scratch workspace and
// sorts chronologically, unlike the former raw-nanosecond names (W2). A
// same-second collision appends -2, -3, ….
func (s *server) newRuntimeDir(kind, prefix string) string {
	base := prefix + "-" + time.Now().Format("20060102-150405")
	dir := filepath.Join(s.runtimeDir, kind, base)
	for i := 2; ; i++ {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return dir
		}
		dir = filepath.Join(s.runtimeDir, kind, fmt.Sprintf("%s-%d", base, i))
	}
}

func (s *server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	dir := s.newRuntimeDir("ws", "ws")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// Make it a repository of its own so the Changes view can track it (W1).
	// runtime/ usually sits inside some parent repo that ignores it, where
	// `git diff` would otherwise silently show nothing. Best-effort: without
	// git the directory is still perfectly usable.
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "git", "-C", dir, "init", "-q").Run()
	writeJSON(w, http.StatusOK, map[string]string{"path": dir})
}

// handleWorktree creates a fresh git worktree of an existing repo and returns
// its path — Codex's "New worktree" run location: an isolated checkout so the
// agent's edits don't touch the user's working tree. Optional branch is
// created off HEAD; empty branch = detached worktree at HEAD.
func (s *server) handleWorktree(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Repo   string `json:"repo"`
		Branch string `json:"branch"`
		Ref    string `json:"ref"`
	}
	if !readBody(w, r, &req) {
		return
	}
	repo, err := filepath.Abs(strings.TrimSpace(req.Repo))
	if err != nil || repo == "" {
		badRequest(w, "repo is required")
		return
	}
	run := func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	if out, err := run("rev-parse", "--is-inside-work-tree"); err != nil || out != "true" {
		badRequest(w, "not a git repository: "+repo)
		return
	}
	dir := s.newRuntimeDir("ws", "wt")
	args := []string{"worktree", "add"}
	if b := strings.TrimSpace(req.Branch); b != "" {
		if !validID(b) {
			badRequest(w, "invalid branch name")
			return
		}
		args = append(args, "-b", b, dir)
	} else {
		args = append(args, "--detach", dir)
		if ref := strings.TrimSpace(req.Ref); ref != "" {
			// Prove the ref names a commit before git worktree receives it; the
			// end-of-options marker prevents option-like input from being parsed.
			if _, err := run("rev-parse", "--verify", "--end-of-options", ref+"^{commit}"); err != nil {
				badRequest(w, "invalid starting ref: "+ref)
				return
			}
			args = append(args, ref)
		}
	}
	if out, err := run(args...); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "git worktree add failed", "stderr": out})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": dir, "repo": repo})
}

func (s *server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		badRequest(w, "multipart parse: "+err.Error())
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		badRequest(w, "missing file field")
		return
	}
	defer func() { _ = f.Close() }()
	name := strings.Map(func(c rune) rune {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_' {
			return c
		}
		return '_'
	}, filepath.Base(hdr.Filename))
	dst := filepath.Join(s.runtimeDir, "uploads", fmt.Sprintf("%d-%s", time.Now().UnixNano(), name))
	out, err := os.Create(dst)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, io.LimitReader(f, 10<<20)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": dst, "name": name})
}

// handleServeUpload serves a previously uploaded file back for thumbnails —
// the journal keeps only a CAS ref, so the browser previews from the local
// uploads dir instead.
func (s *server) handleServeUpload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		badRequest(w, "invalid upload name")
		return
	}
	http.ServeFile(w, r, filepath.Join(s.runtimeDir, "uploads", name))
}

// ---- per-session reads ----

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	after := int64(0)
	if a := r.URL.Query().Get("after"); a != "" {
		after, _ = strconv.ParseInt(a, 10, 64)
	}
	res := s.runAR(r.Context(), 30*time.Second, "events", "--json", id)
	if res.Err != nil {
		arFail(w, "ar events", res)
		return
	}
	out := []json.RawMessage{}
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if after > 0 {
			var seq struct {
				Seq int64 `json:"seq"`
			}
			if json.Unmarshal([]byte(line), &seq) == nil && seq.Seq <= after {
				continue
			}
		}
		out = append(out, json.RawMessage(line))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	res := s.runAR(r.Context(), 30*time.Second, "events", "--state", id)
	if res.Err != nil {
		arFail(w, "ar events --state", res)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, res.Stdout)
}

func (s *server) handleInspect(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	res := s.runAR(r.Context(), 30*time.Second, "inspect", "--json", id)
	if res.Err != nil {
		arFail(w, "ar inspect", res)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, res.Stdout)
}

// handleArtifact serves one published artifact version's raw content
// (INC-40) — a thin wrapper over `ar artifacts read`, like everything else
// here. Text renders in the UI viewer; the CLI already refuses nothing, so
// binary just arrives as bytes the viewer shows a note for.
func (s *server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	stream := r.URL.Query().Get("stream")
	if stream == "" || strings.ContainsAny(stream, "\n\r") {
		badRequest(w, "stream query parameter required")
		return
	}
	spec := stream
	if v := r.URL.Query().Get("version"); v != "" {
		if _, err := strconv.Atoi(v); err != nil {
			badRequest(w, "version must be a number")
			return
		}
		spec = stream + "@v" + v
	}
	res := s.runAR(r.Context(), 30*time.Second, "artifacts", id, "read", spec)
	if res.Err != nil {
		arFail(w, "ar artifacts read", res)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, res.Stdout)
}

func (s *server) handlePS(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	res := s.runAR(r.Context(), 15*time.Second, "ps", id)
	if res.Err != nil {
		arFail(w, "ar ps", res)
		return
	}
	type task struct {
		Handle string `json:"handle"`
		Tool   string `json:"tool"`
		Detail string `json:"detail"`
	}
	tasks := []task{}
	for _, line := range strings.Split(res.Stdout, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "no tasks") {
			continue
		}
		f := strings.SplitN(line, "\t", 3)
		t := task{Handle: f[0]}
		if len(f) > 1 {
			t.Tool = f[1]
		}
		if len(f) > 2 {
			t.Detail = f[2]
		}
		tasks = append(tasks, t)
	}
	writeJSON(w, http.StatusOK, tasks)
}

// handleBarriers lists a session's fork points via `ar fork --list`.
func (s *server) handleBarriers(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	res := s.runAR(r.Context(), 15*time.Second, "fork", "--list", id)
	if res.Err != nil {
		arFail(w, "ar fork --list", res)
		return
	}
	names := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "no barriers") || strings.HasPrefix(line, "BARRIER") {
			continue
		}
		name := strings.Fields(line)[0]
		if seen[name] { // a barrier repeated across turns lists once
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	writeJSON(w, http.StatusOK, names)
}

// handleBarrier checkpoints the session now (`ar barrier`), creating a manual
// fork point. Note: barrier takes the session's exclusive lock, so it can be
// refused while a turn is in flight — the CLI error is surfaced as-is.
func (s *server) handleBarrier(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	res := s.runAR(r.Context(), oneShotTimeout, "barrier", id)
	if res.Err != nil {
		arFail(w, "ar barrier", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"barrier": parseBarrierID(res.Stdout)})
}

// parseBarrierID pulls the barrier id out of `ar barrier` output
// ("barrier <id>\nsnapshot <ref>").
func parseBarrierID(stdout string) string {
	for _, line := range strings.Split(stdout, "\n") {
		if rest, ok := strings.CutPrefix(line, "barrier "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// handleStream pipes `ar attach --json <sid>` as SSE: journal catch-up first,
// then live events. Client disconnect kills the child (ctx).
func (s *server) handleStream(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	fl, canFlush := w.(http.Flusher)
	if !canFlush {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	cmd := exec.CommandContext(r.Context(), s.arPath, "attach", "--json", id)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = cmd.Wait() }()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	fl.Flush()

	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			_, _ = io.WriteString(w, ": ping\n\n")
			fl.Flush()
		case line, more := <-lines:
			if !more {
				_, _ = io.WriteString(w, "event: end\ndata: {\"reason\":\"attach-exited\"}\n\n")
				fl.Flush()
				return
			}
			_, _ = io.WriteString(w, "data: "+line+"\n\n")
			fl.Flush()
		}
	}
}

// ---- per-session writes ----

func (s *server) handleSend(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	var req struct {
		Text   string   `json:"text"`
		Images []string `json:"images"`
		Files  []string `json:"files"`
		// Delivery is the per-message delivery mode (INC-43): "steer" folds the
		// message into the running turn at its next safe boundary; "" / "queue"
		// (default) queues it for the next turn. Only "steer" is honored; any
		// other value falls through to the default queue.
		Delivery string `json:"delivery"`
	}
	if !readBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		badRequest(w, "text is required")
		return
	}
	// --detach: deliver and return; the reply and any approval land in the
	// journal, which the UI already polls. Blocking would re-introduce the
	// orphaned-approval failure on follow-up turns (same as `ar new`).
	args := []string{"send", "--detach"}
	if req.Delivery == "steer" {
		args = append(args, "--steer")
	}
	for _, img := range req.Images {
		if st, err := os.Stat(img); err != nil || st.IsDir() {
			badRequest(w, "image not readable: "+img)
			return
		}
		args = append(args, "--image", img)
	}
	for _, f := range req.Files {
		if st, err := os.Stat(f); err != nil || st.IsDir() {
			badRequest(w, "file not readable: "+f)
			return
		}
		args = append(args, "--file", f)
	}
	args = append(args, id, req.Text)
	res := s.runAR(r.Context(), oneShotTimeout, args...)
	if res.Err != nil {
		arFail(w, "ar send", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(res.Stdout)})
}

func (s *server) oneShotHandler(what string, argsFor func(id string) []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := sid(w, r)
		if !ok {
			return
		}
		res := s.runAR(r.Context(), oneShotTimeout, argsFor(id)...)
		if res.Err != nil {
			arFail(w, what, res)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(res.Stdout)})
	}
}

func (s *server) handleInterrupt(w http.ResponseWriter, r *http.Request) {
	s.oneShotHandler("ar interrupt", func(id string) []string { return []string{"interrupt", id} })(w, r)
}

func (s *server) handleResume(w http.ResponseWriter, r *http.Request) {
	s.oneShotHandler("ar resume", func(id string) []string { return []string{"resume", id} })(w, r)
}

// handleRetry re-sends the session's last user message as a new turn
// (INC-44 §B): a thin wrapper over `ar retry --detach` — the derived
// command id makes double-clicks idempotent at the inbox.
func (s *server) handleRetry(w http.ResponseWriter, r *http.Request) {
	s.oneShotHandler("ar retry", func(id string) []string { return []string{"retry", "--detach", id} })(w, r)
}

// handleClose ends a session for good (ar close); handleStop tears down its
// hosted run but keeps it revivable — a later send brings it back (ar stop).
func (s *server) handleClose(w http.ResponseWriter, r *http.Request) {
	s.oneShotHandler("ar close", func(id string) []string { return []string{"close", id} })(w, r)
}

func (s *server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.oneShotHandler("ar stop", func(id string) []string { return []string{"stop", id} })(w, r)
}

func (s *server) handleKill(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	var req struct {
		Handle string `json:"handle"`
	}
	if !readBody(w, r, &req) {
		return
	}
	if !validID(req.Handle) {
		badRequest(w, "invalid handle")
		return
	}
	res := s.runAR(r.Context(), oneShotTimeout, "kill", id, req.Handle)
	if res.Err != nil {
		arFail(w, "ar kill", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(res.Stdout)})
}

func (s *server) handleApprove(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	var req struct {
		ApprovalID string `json:"approvalId"`
		Decision   string `json:"decision"`
		Reason     string `json:"reason"`
		// Always saves an exact allow rule to the user config so this call
		// stops asking (ar approve --always, INC-17) — approve only.
		Always bool `json:"always"`
	}
	if !readBody(w, r, &req) {
		return
	}
	if !validID(req.ApprovalID) || (req.Decision != "approve" && req.Decision != "deny") {
		badRequest(w, "need approvalId and decision approve|deny")
		return
	}
	args := []string{"approve", id, req.ApprovalID, req.Decision}
	if strings.TrimSpace(req.Reason) != "" {
		args = append(args, req.Reason)
	}
	if req.Always && req.Decision == "approve" {
		args = append(args, "--always")
	}
	res := s.runAR(r.Context(), oneShotTimeout, args...)
	if res.Err != nil {
		arFail(w, "ar approve", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(res.Stdout)})
}

// handleAgentSwitch swaps the session's agent spec (decision #32). The new
// spec (plus siblings) is materialised to runtime/specs first.
func (s *server) handleAgentSwitch(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	var req struct {
		Spec       string     `json:"spec"`
		ExtraSpecs []specFile `json:"extraSpecs"`
	}
	if !readBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Spec) == "" {
		badRequest(w, "spec is required")
		return
	}
	_, basePath, err := s.writeSpecDir(req.Spec, req.ExtraSpecs)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	res := s.runAR(r.Context(), oneShotTimeout, "agent", id, basePath)
	if res.Err != nil {
		arFail(w, "ar agent", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(res.Stdout)})
}

// handleFork branches a session at a barrier into a new session.
func (s *server) handleFork(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	var req struct {
		Barrier   string `json:"barrier"`
		Workspace string `json:"workspace"`
	}
	if !readBody(w, r, &req) {
		return
	}
	if !validID(req.Barrier) {
		badRequest(w, "invalid barrier name")
		return
	}
	args := []string{"fork"}
	if strings.TrimSpace(req.Workspace) != "" {
		ws, err := filepath.Abs(req.Workspace)
		if err != nil {
			badRequest(w, err.Error())
			return
		}
		args = append(args, "--workspace", ws)
	}
	args = append(args, id, req.Barrier)
	res := s.runAR(r.Context(), oneShotTimeout, args...)
	if res.Err != nil {
		arFail(w, "ar fork", res)
		return
	}
	newID := parseSessionID(res)
	if newID != "" {
		forkWS := strings.TrimSpace(req.Workspace)
		parent := s.meta.get(id)
		title := parent.Title
		if title != "" {
			title += " (fork @" + req.Barrier + ")"
		}
		s.meta.set(newID, forkWS, title)
	}
	writeJSON(w, http.StatusOK, map[string]string{"sid": newID, "status": strings.TrimSpace(res.Stdout + "\n" + res.Stderr)})
}
