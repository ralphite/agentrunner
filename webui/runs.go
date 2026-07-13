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
	"regexp"
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
	SessionID string   `json:"sessionId,omitempty"`
	Status    string   `json:"status"` // running | done | failed | stopped
	StartedAt string   `json:"startedAt"`
	Args      []string `json:"-"`

	mu    sync.Mutex
	lines []string
	subs  map[chan string]struct{}
	done  bool
	// spec is the drive run's driver spec (nil for submit): the source of the
	// cadence / next-run projection the Scheduled page shows (CX-3). lastIter
	// anchors an interval cadence — the driver announces each iteration on its
	// stderr, which we merge into the run's stream.
	spec     *driverSpec
	lastIter time.Time
	cancel   context.CancelFunc
}

// iterationLine matches the driver's own iteration/attempt announcement
// ("iteration 3 (drv-…-iter-3)" / "attempt 2 (…) in <worktree>"), the only
// live signal we have for when the last iteration started.
var iterationLine = regexp.MustCompile(`^(?:iteration|attempt) \d+ \(`)

// runView is the wire shape of a run: the stored facts plus the derived
// schedule projection. nextRunAt is computed per request because it moves with
// the clock — a cached string would be stale the moment it is written.
type runView struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Workspace string `json:"workspace"`
	SessionID string `json:"sessionId,omitempty"`
	Status    string `json:"status"`
	StartedAt string `json:"startedAt"`
	scheduleView
}

func (r *run) view(now time.Time) runView {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := runView{
		ID: r.ID, Kind: r.Kind, Label: r.Label, Workspace: r.Workspace,
		SessionID: r.SessionID, Status: r.Status, StartedAt: r.StartedAt,
	}
	if r.spec == nil {
		return v
	}
	// Before the first iteration announcement, the run's own start is the
	// honest anchor for an interval cadence (the driver launches immediately).
	last := r.lastIter
	if last.IsZero() {
		if t, err := time.Parse(time.RFC3339, r.StartedAt); err == nil {
			last = t
		}
	}
	v.scheduleView = scheduleFor(r.spec, last, r.Status == "running", now)
	return v
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

func (r *run) setSessionID(id string) {
	if id == "" {
		return
	}
	r.mu.Lock()
	if r.SessionID == "" {
		r.SessionID = id
	}
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
// onSession fires once with the daemon-assigned session id (parsed from the
// event stream) so the caller can record workspace/title metadata for it.
// spec is the drive run's driver spec (nil for submit) — it carries the
// cadence the Scheduled page reports.
func (rr *runRegistry) start(arPath, kind, label, workspace string, args []string, logDir string, spec *driverSpec, onSession func(sid string)) *run {
	rr.mu.Lock()
	rr.seq++
	id := fmt.Sprintf("run%d", rr.seq)
	rr.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	r := &run{
		ID: id, Kind: kind, Label: label, Workspace: workspace,
		Status: "running", StartedAt: time.Now().Format(time.RFC3339),
		Args: args, spec: spec, cancel: cancel,
	}
	rr.mu.Lock()
	rr.runs[id] = r
	rr.list = append([]*run{r}, rr.list...) // newest first
	rr.mu.Unlock()

	// O_TRUNC, not O_APPEND: run ids (run1, run2…) restart per process, so a
	// reused id must start a clean log, not append to a prior run's output.
	logf, _ := os.OpenFile(filepath.Join(logDir, id+".log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
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
		notified := false
		for sc.Scan() {
			line := sc.Text()
			r.append(line)
			if logf != nil {
				_, _ = io.WriteString(logf, line+"\n")
			}
			// A submit run's process stays attached after its initial turn goes
			// idle (a conversational agent never "ends"), so it would otherwise
			// show "running" forever. Reaching idle means the one-shot run is
			// done — reconcile the run status with the session's (QA r3-#2).
			if r.Kind == "submit" && strings.Contains(line, `"kind":"idle"`) {
				r.mu.Lock()
				if r.Status == "running" {
					r.Status = "done"
				}
				r.mu.Unlock()
			}
			// Anchor the interval cadence on the iteration the driver just
			// started (CX-3): next run = this + interval.
			if r.spec != nil && iterationLine.MatchString(line) {
				r.mu.Lock()
				r.lastIter = time.Now()
				r.mu.Unlock()
			}
			if m := sessionIDLine.FindString(line); m != "" {
				r.setSessionID(m)
				if !notified && onSession != nil {
					notified = true
					onSession(m)
				}
			}
		}
		scanErr := sc.Err()
		if scanErr != nil {
			// Stop a child that may otherwise remain blocked writing the pipe
			// after Scanner rejects an oversized line.
			cancel()
			r.append(fmt.Sprintf(`{"kind":"error","text":%q}`, "read run output: "+scanErr.Error()))
		}
		err := cmd.Wait()
		r.mu.Lock()
		r.done = true
		if r.Status == "running" {
			if scanErr != nil {
				r.Status = "failed"
			} else if ctx.Err() != nil {
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
	now := time.Now()
	snap := s.runs.snapshot()
	out := make([]runView, 0, len(snap))
	for _, run := range snap {
		out = append(out, run.view(now))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) handleRunStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind       string     `json:"kind"` // submit | drive
		Spec       string     `json:"spec"` // base.yaml (submit) or driver.yaml (drive)
		ExtraSpecs []specFile `json:"extraSpecs"`
		Prompt     string     `json:"prompt"`
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
	if req.Kind == "submit" && strings.TrimSpace(req.Prompt) == "" {
		badRequest(w, "prompt is required for submit")
		return
	}
	ws, ferr := resolveWorkspace(req.Workspace)
	if ferr != "" {
		badRequest(w, ferr)
		return
	}
	_, basePath, err := s.writeSpecDir(req.Spec, req.ExtraSpecs)
	if err != nil {
		badRequest(w, err.Error())
		return
	}

	// A drive run's spec IS its schedule contract: parse it once at launch so
	// every /api/runs poll can report the cadence and the next tick (CX-3).
	var spec *driverSpec
	if req.Kind == "drive" {
		spec = specFromYAML(req.Spec)
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
		args = append(args, basePath, req.Prompt)
		label = firstLine(req.Prompt, 60)
	} else {
		args = []string{"drive", "--json", "--workspace", ws, basePath}
		if name := yamlName(req.Spec); name != "" {
			label = "drive: " + name
		} else {
			label = "drive: " + filepath.Base(basePath)
		}
	}

	title := label
	if req.Kind == "submit" {
		title = req.Prompt
	}
	run := s.runs.start(s.arPath, req.Kind, label, ws, args, filepath.Join(s.runtimeDir, "runs"), spec,
		func(sid string) { s.meta.set(sid, ws, title) })
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
	if run.finished() {
		// Signal end so the client closes instead of auto-reconnecting and
		// re-replaying the whole backlog forever.
		_, _ = io.WriteString(w, "event: end\ndata: {\"reason\":\"run-finished\"}\n\n")
		fl.Flush()
		return
	}
	fl.Flush()
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
				_, _ = io.WriteString(w, "event: end\ndata: {\"reason\":\"run-finished\"}\n\n")
				fl.Flush()
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
	// Truncate by rune, not byte: a byte slice can cut through a multibyte
	// UTF-8 rune (e.g. a CJK char) and produce U+FFFD "" in the title, which
	// then leaks into /api/sessions, the sidebar and the CLI table (R4-2).
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}

// yamlName pulls the top-level `name:` value out of a spec (best-effort; used
// only to label a drive run with its driver name rather than "base.yaml").
func yamlName(spec string) string {
	for _, line := range strings.Split(spec, "\n") {
		t := strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(t, "name:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
