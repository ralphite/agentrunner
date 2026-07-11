package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// launchArgv maps a UI app token to the EXACT argv used to open a directory in
// a system application (INC-53, HANDA #24 launcher). This is the hard gate on
// the new OS-exec surface: `app` is only ever a SELECTION KEY into this fixed
// per-OS table — user input never names the executable and never reaches
// argv[0]. The directory is always the trailing, isolated argument, passed via
// exec.Command (no shell), so a path can't be reinterpreted as flags or command
// text. An unknown token, or one unsupported on this OS, returns ok=false and
// the caller rejects the request without executing anything.
func launchArgv(app, dir string) (argv []string, ok bool) {
	switch runtime.GOOS {
	case "darwin":
		switch app {
		case "vscode":
			return []string{"open", "-a", "Visual Studio Code", dir}, true
		case "finder":
			return []string{"open", dir}, true
		case "terminal":
			return []string{"open", "-a", "Terminal", dir}, true
		}
	case "linux":
		switch app {
		case "vscode":
			return []string{"code", dir}, true
		case "finder":
			return []string{"xdg-open", dir}, true
			// "terminal" is intentionally unsupported on Linux: there is no
			// reliable, portable "open a terminal here" command. Better to
			// return an honest error than guess a fragile x-terminal-emulator.
		}
	}
	return nil, false
}

// runLaunch executes a launcher argv. Direct argv, no shell — matching runAR.
// The command is fire-and-forget style but we wait briefly so a failed spawn
// (e.g. the app isn't installed) surfaces as an error rather than a silent 200.
func runLaunch(ctx context.Context, argv []string) error {
	if len(argv) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	return cmd.Run()
}

// canonPath resolves symlinks then cleans a path so /tmp and /private/tmp
// (macOS) compare equal for known-workspace membership.
func canonPath(p string) string {
	if r, err := filepath.EvalSymlinks(filepath.Clean(p)); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// knownWorkspacesFromAR derives the set of directories the launcher may target
// from the live journal-backed session list — exactly the workspaces the
// sidebar shows. This is fail-closed: if `ar` can't be reached or its output
// can't be parsed, the set is empty and every /api/open is refused.
func (s *server) knownWorkspacesFromAR(ctx context.Context) map[string]bool {
	set := map[string]bool{}
	res := s.runAR(ctx, 15*time.Second, "sessions", "list", "--json")
	if res.Err != nil {
		return set
	}
	var rows []struct {
		Workspace string `json:"workspace"`
	}
	if json.Unmarshal([]byte(res.Stdout), &rows) != nil {
		return set
	}
	for _, row := range rows {
		if w := strings.TrimSpace(row.Workspace); w != "" {
			set[canonPath(w)] = true
		}
	}
	return set
}

// knownSet returns the launcher's allowed-workspace set. The `workspaces` field
// lets tests inject a fixed set without a real `ar` binary; nil = query ar.
func (s *server) knownSet(ctx context.Context) map[string]bool {
	if s.workspaces != nil {
		return s.workspaces(ctx)
	}
	return s.knownWorkspacesFromAR(ctx)
}

// handleOpen opens a session's workspace directory in a system application
// (INC-53 launcher). Both inputs are gated: `app` must be a whitelisted token
// (launchArgv), and `workspace` must be a KNOWN agent workspace (derived from
// the live session list), never an arbitrary host path. See the OS-exec
// security clause in docs/increments/INC-53-project-launcher.md.
func (s *server) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Workspace string `json:"workspace"`
		App       string `json:"app"`
	}
	if !readBody(w, r, &req) {
		return
	}
	ws := strings.TrimSpace(req.Workspace)
	if ws == "" {
		badRequest(w, "workspace is required")
		return
	}
	// Gate 1: the workspace must be a directory that actually exists AND is a
	// known agent session workspace — never an arbitrary path the caller names.
	st, err := os.Stat(ws)
	if err != nil || !st.IsDir() {
		badRequest(w, "not an existing directory: "+ws)
		return
	}
	if !s.knownSet(r.Context())[canonPath(ws)] {
		badRequest(w, "unknown workspace: not a known agent session workspace")
		return
	}
	// Gate 2: the app token must be whitelisted; the returned argv carries the
	// directory as an isolated trailing argument (no shell).
	argv, ok := launchArgv(req.App, ws)
	if !ok {
		badRequest(w, "unsupported app: "+req.App)
		return
	}
	run := s.launch
	if run == nil {
		run = runLaunch
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := run(ctx, argv); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "open failed", "stderr": err.Error()})
		return
	}
	// Record the open so the sidebar can show a "last opened" hint (INC-53).
	s.meta.touchProject(ws)
	writeJSON(w, http.StatusOK, map[string]string{"status": "opened"})
}

// handleProjectsList returns the workspace-keyed overlay (INC-53): custom
// display names, folded state, and last-opened times the sidebar renders on top
// of the journal-derived project groups.
func (s *server) handleProjectsList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.meta.allProjects())
}

// handleProjectUpdate applies a partial overlay update for one project group
// (INC-53): a custom display name and/or folded state. The key is the project
// group's identity (a workspace path, or the "__scratch__" aggregate) — purely
// cosmetic metadata, so it is length-capped but not required to be a known
// workspace (folding the Scratch aggregate must work, and toggling must be
// snappy without an `ar` round-trip). It never execs anything.
func (s *server) handleProjectUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Workspace   string  `json:"workspace"`
		DisplayName *string `json:"displayName"`
		Folded      *bool   `json:"folded"`
	}
	if !readBody(w, r, &req) {
		return
	}
	key := strings.TrimSpace(req.Workspace)
	if key == "" {
		badRequest(w, "workspace is required")
		return
	}
	if len(key) > 512 || strings.ContainsAny(key, "\n\r\x00") {
		badRequest(w, "invalid workspace key")
		return
	}
	if req.DisplayName != nil && len(*req.DisplayName) > 200 {
		badRequest(w, "display name too long")
		return
	}
	s.meta.setProject(key, req.DisplayName, req.Folded)
	writeJSON(w, http.StatusOK, s.meta.allProjects())
}
