package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// A run is a background one-shot: `ar submit` (daemon-hosted) or `ar drive`
// (foreground iteration driver). Both emit `--json` event lines that we buffer
// and fan out over SSE so the UI can follow them like a session timeline.
type run struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"` // submit | drive
	Label     string   `json:"label"`
	Workspace string   `json:"workspace"`
	Status    string   `json:"status"` // running | done | failed | stopped
	StartedAt string   `json:"startedAt"`
	Args      []string `json:"-"`

	mu     sync.Mutex
	lines  []string
	subs   map[chan string]struct{}
	done   bool
	cancel context.CancelFunc
}

func (r *run) append(line string) {
	r.mu.Lock()
	r.lines = append(r.lines, line)
	for ch := range r.subs {
		select {
		case ch <- line:
		default:
		}
	}
	r.mu.Unlock()
}

func (r *run) subscribe() (chan string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan string, 256)
	if r.subs == nil {
		r.subs = map[chan string]struct{}{}
	}
	r.subs[ch] = struct{}{}
	backlog := append([]string(nil), r.lines...)
	return ch, backlog
}

func (r *run) unsubscribe(ch chan string) {
	r.mu.Lock()
	delete(r.subs, ch)
	r.mu.Unlock()
}

func (r *run) finished() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.done
}

type runRegistry struct {
	mu   sync.Mutex
	seq  int
	runs map[string]*run
	list []*run
}

func newRunRegistry() *runRegistry { return &runRegistry{runs: map[string]*run{}} }

func (rr *runRegistry) get(id string) *run {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	return rr.runs[id]
}

func (rr *runRegistry) snapshot() []*run {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	out := make([]*run, len(rr.list))
	copy(out, rr.list)
	return out
}

func (rr *runRegistry) stopAll() {
	for _, r := range rr.snapshot() {
		if r.cancel != nil {
			r.cancel()
		}
	}
}

// start launches an ar process and streams its stdout lines into a fresh run.
func (rr *runRegistry) start(arPath, kind, label, workspace string, args []string, logDir string) *run {
	rr.mu.Lock()
	rr.seq++
	id := fmt.Sprintf("run%d", rr.seq)
	rr.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	r := &run{
		ID: id, Kind: kind, Label: label, Workspace: workspace,
		Status: "running", StartedAt: time.Now().Format(time.RFC3339),
		Args: args, cancel: cancel,
	}
	rr.mu.Lock()
	rr.runs[id] = r
	rr.list = append([]*run{r}, rr.list...) // newest first
	rr.mu.Unlock()

	logf, _ := os.OpenFile(filepath.Join(logDir, id+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	cmd := exec.CommandContext(ctx, arPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.Status = "failed"
		r.append(fmt.Sprintf(`{"kind":"error","text":%q}`, err.Error()))
		r.done = true
		cancel()
		return r
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		r.Status = "failed"
		r.append(fmt.Sprintf(`{"kind":"error","text":%q}`, err.Error()))
		r.done = true
		cancel()
		return r
	}
	go func() {
		defer func() {
			if logf != nil {
				_ = logf.Close()
			}
		}()
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
		for sc.Scan() {
			line := sc.Text()
			r.append(line)
			if logf != nil {
				_, _ = io.WriteString(logf, line+"\n")
			}
		}
		err := cmd.Wait()
		r.mu.Lock()
		r.done = true
		if r.Status == "running" {
			if ctx.Err() != nil {
				r.Status = "stopped"
			} else if err != nil {
				r.Status = "failed"
			} else {
				r.Status = "done"
			}
		}
		st := r.Status
		for ch := range r.subs {
			close(ch)
		}
		r.subs = nil
		r.mu.Unlock()
		r.append(fmt.Sprintf(`{"kind":"end","status":%q}`, st))
	}()
	return r
}

// ---- HTTP ----

func (s *server) handleRunsList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.runs.snapshot())
}

func (s *server) handleRunStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind       string     `json:"kind"` // submit | drive
		Spec       string     `json:"spec"` // base.yaml (submit) or driver.yaml (drive)
		ExtraSpecs []specFile `json:"extraSpecs"`
		Task       string     `json:"task"`
		Workspace  string     `json:"workspace"`
		Mode       string     `json:"mode"`
		Idem       string     `json:"idem"`
	}
	if !readBody(w, r, &req) {
		return
	}
	if req.Kind != "submit" && req.Kind != "drive" {
		badRequest(w, "kind must be submit or drive")
		return
	}
	if strings.TrimSpace(req.Spec) == "" || strings.TrimSpace(req.Workspace) == "" {
		badRequest(w, "spec and workspace are required")
		return
	}
	if req.Kind == "submit" && strings.TrimSpace(req.Task) == "" {
		badRequest(w, "task is required for submit")
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
	_, basePath, err := s.writeSpecDir(req.Spec, req.ExtraSpecs)
	if err != nil {
		badRequest(w, err.Error())
		return
	}

	var args []string
	var label string
	if req.Kind == "submit" {
		args = []string{"submit", "--json", "--workspace", ws}
		if req.Mode != "" {
			args = append(args, "--mode", req.Mode)
		}
		if strings.TrimSpace(req.Idem) != "" {
			args = append(args, "--idem", req.Idem)
		}
		args = append(args, basePath, req.Task)
		label = firstLine(req.Task, 60)
	} else {
		args = []string{"drive", "--json", "--workspace", ws, basePath}
		label = "driver: " + filepath.Base(basePath)
	}

	run := s.runs.start(s.arPath, req.Kind, label, ws, args, filepath.Join(s.runtimeDir, "runs"))
	writeJSON(w, http.StatusOK, map[string]string{"runId": run.ID})
}

func (s *server) handleRunStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("rid")
	run := s.runs.get(id)
	if run == nil {
		http.Error(w, "unknown run", http.StatusNotFound)
		return
	}
	fl, canFlush := w.(http.Flusher)
	if !canFlush {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	fl.Flush()

	ch, backlog := run.subscribe()
	defer run.unsubscribe(ch)
	for _, line := range backlog {
		_, _ = io.WriteString(w, "data: "+line+"\n\n")
	}
	fl.Flush()
	if run.finished() {
		// backlog already carries the end marker; nothing more will arrive.
		return
	}
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			_, _ = io.WriteString(w, ": ping\n\n")
			fl.Flush()
		case line, more := <-ch:
			if !more {
				return
			}
			_, _ = io.WriteString(w, "data: "+line+"\n\n")
			fl.Flush()
		}
	}
}

func (s *server) handleRunStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("rid")
	run := s.runs.get(id)
	if run == nil {
		http.Error(w, "unknown run", http.StatusNotFound)
		return
	}
	if run.cancel != nil {
		run.cancel()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}

func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}
