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
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/daemon/start", s.handleDaemonStart)
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("POST /api/sessions", s.handleNewSession)
	mux.HandleFunc("POST /api/workspace", s.handleWorkspace)
	mux.HandleFunc("POST /api/upload", s.handleUpload)
	mux.HandleFunc("GET /api/sessions/{sid}/events", s.handleEvents)
	mux.HandleFunc("GET /api/sessions/{sid}/state", s.handleState)
	mux.HandleFunc("GET /api/sessions/{sid}/inspect", s.handleInspect)
	mux.HandleFunc("GET /api/sessions/{sid}/ps", s.handlePS)
	mux.HandleFunc("GET /api/sessions/{sid}/stream", s.handleStream)
	mux.HandleFunc("POST /api/sessions/{sid}/send", s.handleSend)
	mux.HandleFunc("POST /api/sessions/{sid}/interrupt", s.handleInterrupt)
	mux.HandleFunc("POST /api/sessions/{sid}/kill", s.handleKill)
	mux.HandleFunc("POST /api/sessions/{sid}/approve", s.handleApprove)
	mux.HandleFunc("POST /api/sessions/{sid}/agent", s.handleAgent)
	mux.HandleFunc("POST /api/sessions/{sid}/barrier", s.handleBarrier)
	mux.HandleFunc("GET /api/sessions/{sid}/barriers", s.handleBarriers)
	mux.HandleFunc("POST /api/sessions/{sid}/fork", s.handleFork)
	mux.HandleFunc("POST /api/drive", s.handleDrive)
	return mux
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	page, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// arFail maps a failed ar invocation onto the error convention (DESIGN §4):
// 502 with {error, stderr} so the UI can show the real failure.
func arFail(w http.ResponseWriter, what string, res arResult) {
	msg := what + " failed"
	if res.Err != nil {
		msg = fmt.Sprintf("%s: %v", what, res.Err)
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{
		"error":  msg,
		"stderr": strings.TrimSpace(res.Stderr),
	})
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}

// sid extracts and validates the {sid} path segment.
func sid(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("sid")
	if !validID(id) {
		badRequest(w, "invalid session id")
		return "", false
	}
	return id, true
}

func readBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(v); err != nil {
		badRequest(w, "bad JSON body: "+err.Error())
		return false
	}
	return true
}

type extraSpec struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// writeSpecDir lands a main spec plus sibling specs in a fresh
// runtime/specs/ dir and returns the main file's path. The CLI resolves
// `agents: [...]` names against the main spec's siblings, so extras must
// live in the same directory.
func (s *server) writeSpecDir(prefix, main, spec string, extras []extraSpec) (string, error) {
	dir := filepath.Join(s.runtimeDir, "specs", fmt.Sprintf("%s%d", prefix, time.Now().UnixNano()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, main), []byte(spec), 0o644); err != nil {
		return "", err
	}
	for _, ex := range extras {
		name := filepath.Base(strings.TrimSpace(ex.Name))
		if !specFileName.MatchString(name) || name == main {
			return "", fmt.Errorf("bad extra spec name: %s", ex.Name)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(ex.Content), 0o644); err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, main), nil
}

// ---- daemon / health ----

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ver := "unknown"
	if res := s.runAR(r.Context(), 5*time.Second, "--version"); res.Err == nil {
		ver = strings.TrimSpace(res.Stdout)
	}
	managed, reachable := s.daemonStatus(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"ar":              s.arPath,
		"version":         ver,
		"daemonManaged":   managed,
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

// ---- sessions ----

func (s *server) handleSessions(w http.ResponseWriter, r *http.Request) {
	res := s.runAR(r.Context(), 15*time.Second, "sessions", "list")
	if res.Err != nil {
		arFail(w, "ar sessions list", res)
		return
	}
	type row struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Turns  int    `json:"turns"`
	}
	rows := []row{}
	for _, line := range strings.Split(res.Stdout, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || f[0] == "SESSION" || line == "no sessions" {
			continue
		}
		turns, _ := strconv.Atoi(f[len(f)-1])
		rows = append(rows, row{ID: f[0], Status: strings.Join(f[1:len(f)-1], " "), Turns: turns})
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *server) handleNewSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Spec       string      `json:"spec"`
		ExtraSpecs []extraSpec `json:"extraSpecs"`
		Workspace  string      `json:"workspace"`
		Message    string      `json:"message"`
		Mode       string      `json:"mode"`
		Trust      bool        `json:"trust"`
		Oneshot    bool        `json:"oneshot"`
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
	specPath, err := s.writeSpecDir("s", "base.yaml", req.Spec, req.ExtraSpecs)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	// Opt-in trust (UJ-20): mark the workspace trusted before the run so
	// project-layer hooks/settings take effect.
	if req.Trust {
		if res := s.runAR(r.Context(), 10*time.Second, "trust", ws); res.Err != nil {
			arFail(w, "ar trust", res)
			return
		}
	}
	// One-shot (UJ-02/13): `ar submit` hands a run to the daemon and streams
	// to completion. We read just the session id off the stream, then drop
	// the streaming client — the run keeps going in the daemon, and the
	// timeline poll renders it like any other session.
	if req.Oneshot {
		id, serr := s.startOneShot(specPath, ws, req.Mode, req.Message)
		if serr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "ar submit: " + serr.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"sid": id, "specDir": filepath.Dir(specPath), "workspace": ws})
		return
	}
	// --detach: INC-2 made `ar new` follow the first turn and print the
	// reply for humans; the cockpit is a programmatic consumer — it wants
	// the ack-only form (stdout = session id) and reads the reply from the
	// journal like everything else.
	args := []string{"new", "--detach", "--workspace", ws}
	if req.Mode != "" {
		args = append(args, "--mode", req.Mode)
	}
	args = append(args, specPath, req.Message)
	res := s.runAR(r.Context(), oneShotTimeout, args...)
	if res.Err != nil {
		arFail(w, "ar new", res)
		return
	}
	id := strings.TrimSpace(strings.SplitN(res.Stdout, "\n", 2)[0])
	if id == "" {
		arFail(w, "ar new (no session id)", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sid": id, "specDir": filepath.Dir(specPath), "workspace": ws})
}

// startOneShot spawns `ar submit --json` (which needs the daemon and requires
// flags BEFORE positionals), reads the session id off the first streamed
// event, then lets the run finish in the BACKGROUND. Unlike `new` (whose
// conversational session stays resident once started), submit's run is bound
// to the client connection — dropping the client cancels the run. So we keep
// the process alive, detached from this HTTP request, draining its output so
// the pipe never blocks; the timeline poll renders progress from the journal.
// A generous cap reaps a hung run.
func (s *server) startOneShot(specPath, ws, mode, task string) (string, error) {
	args := []string{"submit", "--json", "--workspace", ws}
	if mode != "" {
		args = append(args, "--mode", mode)
	}
	args = append(args, specPath, task)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	cmd := exec.CommandContext(ctx, s.arPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return "", err
	}
	var errb strings.Builder
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		cancel()
		return "", err
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	sid := ""
	for sc.Scan() {
		var ev struct {
			Session string `json:"session"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) == nil && ev.Session != "" {
			sid = ev.Session
			break
		}
	}
	if sid == "" {
		cancel()
		_ = cmd.Wait()
		if e := strings.TrimSpace(errb.String()); e != "" {
			return "", fmt.Errorf("no session id in stream: %s", e)
		}
		return "", fmt.Errorf("submit produced no session id")
	}
	go func() {
		defer cancel()
		_, _ = io.Copy(io.Discard, stdout) // keep the pipe drained so the run isn't blocked
		_ = cmd.Wait()
	}()
	return sid, nil
}

// handleBarrier checkpoints the session (`ar barrier`). Note: barrier takes
// the session's exclusive lock, so a daemon-hosted (live) session is refused
// with "a live session cannot be barriered externally" — surfaced as-is.
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
	bid := ""
	for _, line := range strings.Split(res.Stdout, "\n") {
		if strings.HasPrefix(line, "barrier ") {
			bid = strings.TrimSpace(strings.TrimPrefix(line, "barrier "))
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"barrier": bid})
}

// handleBarriers lists a session's barriers by parsing `ar fork <sid> --list`
// (a fixed-width table, not JSON) into structured rows.
func (s *server) handleBarriers(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	res := s.runAR(r.Context(), 15*time.Second, "fork", id, "--list")
	if res.Err != nil {
		arFail(w, "ar fork --list", res)
		return
	}
	type bar struct {
		ID       string `json:"id"`
		Turn     int    `json:"turn"`
		Seq      int    `json:"seq"`
		Snapshot string `json:"snapshot"`
	}
	bars := []bar{}
	for _, line := range strings.Split(res.Stdout, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || f[0] == "BARRIER" || strings.HasPrefix(line, "no barriers") {
			continue
		}
		turn, _ := strconv.Atoi(f[1])
		seq, _ := strconv.Atoi(f[2])
		snap := ""
		if len(f) > 3 {
			snap = f[3]
		}
		bars = append(bars, bar{ID: f[0], Turn: turn, Seq: seq, Snapshot: snap})
	}
	writeJSON(w, http.StatusOK, bars)
}

// handleFork branches a session at a barrier into a new one (`ar fork`).
func (s *server) handleFork(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	var req struct {
		Barrier string `json:"barrier"`
	}
	if !readBody(w, r, &req) {
		return
	}
	if !validID(req.Barrier) {
		badRequest(w, "invalid barrier id")
		return
	}
	res := s.runAR(r.Context(), oneShotTimeout, "fork", id, req.Barrier)
	if res.Err != nil {
		arFail(w, "ar fork", res)
		return
	}
	newid := ""
	for _, line := range strings.Split(res.Stdout, "\n") {
		if strings.HasPrefix(line, "session ") {
			newid = strings.TrimSpace(strings.TrimPrefix(line, "session "))
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"sid": newid})
}

func (s *server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Join(s.runtimeDir, "ws", fmt.Sprintf("ws%d", time.Now().UnixNano()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": dir})
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

// handleStream pipes `ar attach --json <sid>` as SSE: journal catch-up
// first, then live events. Client disconnect kills the child (ctx).
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
	}
	if !readBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		badRequest(w, "text is required")
		return
	}
	// --detach for the same reason as `new`: deliver-and-ack, the timeline
	// poll renders the reply. Without it send follows until idle (INC-2).
	args := []string{"send", "--detach"}
	for _, img := range req.Images {
		if st, err := os.Stat(img); err != nil || st.IsDir() {
			badRequest(w, "image not readable: "+img)
			return
		}
		args = append(args, "--image", img)
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
	s.oneShotHandler("ar interrupt", func(id string) []string {
		return []string{"interrupt", id}
	})(w, r)
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

// handleAgent switches the session's agent mid-conversation (决策 #32):
// the new spec text lands next to a fresh specs/ dir and `ar agent` does
// the rest (SpecChanged event, re-frozen prefix, same journal).
func (s *server) handleAgent(w http.ResponseWriter, r *http.Request) {
	id, ok := sid(w, r)
	if !ok {
		return
	}
	var req struct {
		Spec       string      `json:"spec"`
		ExtraSpecs []extraSpec `json:"extraSpecs"`
	}
	if !readBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Spec) == "" {
		badRequest(w, "spec is required")
		return
	}
	specPath, err := s.writeSpecDir("a", "agent.yaml", req.Spec, req.ExtraSpecs)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	res := s.runAR(r.Context(), oneShotTimeout, "agent", id, specPath)
	if res.Err != nil {
		arFail(w, "ar agent", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(res.Stdout)})
}

// yamlStr renders a Go string as a YAML scalar by borrowing JSON's quoting
// (YAML is a JSON superset, so a JSON double-quoted string is valid YAML and
// safely escapes colons, quotes, and newlines in user-supplied task/command).
func yamlStr(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// buildDriverYAML assembles an iteration-driver spec (internal/driver/spec.go)
// from the cockpit's driver form. Modes map to the schedule enum: goal →
// immediate (empty), loop → interval, best-of-N → parallel.
func buildDriverYAML(req driveReq, agentFile string) string {
	var b strings.Builder
	b.WriteString("name: web-driver\n")
	fmt.Fprintf(&b, "agent_spec: %s\n", agentFile)
	fmt.Fprintf(&b, "task: %s\n", yamlStr(req.Task))
	switch req.Mode {
	case "loop":
		b.WriteString("schedule: interval\n")
		if req.Interval != "" {
			fmt.Fprintf(&b, "interval: %s\n", yamlStr(req.Interval))
		}
	case "parallel":
		b.WriteString("schedule: parallel\n")
		if req.N >= 2 {
			fmt.Fprintf(&b, "n: %d\n", req.N)
		}
	}
	if req.MaxIterations > 0 {
		fmt.Fprintf(&b, "max_iterations: %d\n", req.MaxIterations)
	}
	if req.Patience > 0 {
		fmt.Fprintf(&b, "patience: %d\n", req.Patience)
	}
	if req.Budget > 0 {
		fmt.Fprintf(&b, "budget: { max_total_tokens: %d }\n", req.Budget)
	}
	if strings.TrimSpace(req.VerifierCommand) != "" {
		b.WriteString("verifiers:\n")
		fmt.Fprintf(&b, "  - kind: command\n    command: %s\n", yamlStr(req.VerifierCommand))
		if req.MetricRegex != "" {
			fmt.Fprintf(&b, "    metric_regex: %s\n    threshold: %g\n", yamlStr(req.MetricRegex), req.Threshold)
		}
	}
	return b.String()
}

type driveReq struct {
	Mode            string  `json:"mode"` // goal | loop | parallel
	Task            string  `json:"task"`
	Spec            string  `json:"spec"` // child agent spec YAML
	Workspace       string  `json:"workspace"`
	Interval        string  `json:"interval"`
	N               int     `json:"n"`
	MaxIterations   int     `json:"maxIterations"`
	Budget          int     `json:"budget"`
	Patience        int     `json:"patience"`
	VerifierCommand string  `json:"verifierCommand"`
	MetricRegex     string  `json:"metricRegex"`
	Threshold       float64 `json:"threshold"`
}

// handleDrive runs `ar drive --json` in the foreground and streams its stdout
// (one protocol.Event per line) back as NDJSON. `drive` runs in-process (no
// daemon); the child runs and the driver's iteration/run_end lifecycle events
// all arrive on this stream. Client disconnect cancels the driver (ctx).
func (s *server) handleDrive(w http.ResponseWriter, r *http.Request) {
	var req driveReq
	if !readBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Task) == "" || strings.TrimSpace(req.Spec) == "" || strings.TrimSpace(req.Workspace) == "" {
		badRequest(w, "task, spec and workspace are required")
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
	// agent.yaml and driver.yaml must be siblings: agent_spec resolves
	// relative to the driver spec's directory.
	dir := filepath.Join(s.runtimeDir, "specs", fmt.Sprintf("drive%d", time.Now().UnixNano()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		badRequest(w, err.Error())
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte(req.Spec), 0o644); err != nil {
		badRequest(w, err.Error())
		return
	}
	driverPath := filepath.Join(dir, "driver.yaml")
	if err := os.WriteFile(driverPath, []byte(buildDriverYAML(req, "agent.yaml")), 0o644); err != nil {
		badRequest(w, err.Error())
		return
	}
	fl, canFlush := w.(http.Flusher)
	if !canFlush {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	cmd := exec.CommandContext(r.Context(), s.arPath, "drive", "--json", "--workspace", ws, driverPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var errb strings.Builder
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = cmd.Wait() }()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fl.Flush()
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		_, _ = io.WriteString(w, line+"\n")
		fl.Flush()
	}
	// The driver's terminal reason/errors ride stderr; surface it as a final
	// synthetic line so the cockpit can render a conclusion (the text renderer
	// drops run_end in text mode, but --json emits it — this is the backstop).
	if e := strings.TrimSpace(errb.String()); e != "" {
		last := e
		if i := strings.LastIndexByte(last, '\n'); i >= 0 {
			last = last[i+1:]
		}
		b, _ := json.Marshal(map[string]string{"kind": "driver_stderr", "text": last})
		_, _ = io.WriteString(w, string(b)+"\n")
		fl.Flush()
	}
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
	res := s.runAR(r.Context(), oneShotTimeout, args...)
	if res.Err != nil {
		arFail(w, "ar approve", res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": strings.TrimSpace(res.Stdout)})
}
