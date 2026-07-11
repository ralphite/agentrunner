package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestLaunchArgvWhitelist pins the OS-exec red line (INC-53): the app token is
// only a selection key into a fixed argv table, the directory is always the
// isolated trailing argument (never spliced through a shell), and an unknown
// token is refused. Enumerated per HANDA #24 launcher targets (G29 discipline).
func TestLaunchArgvWhitelist(t *testing.T) {
	const dir = "/known/workspace"

	// Unknown / injection-shaped tokens are rejected on every platform.
	for _, bad := range []string{"", "sh", "/bin/sh", "vscode; rm -rf /", "VSCode", "open"} {
		if argv, ok := launchArgv(bad, dir); ok {
			t.Fatalf("launchArgv(%q) unexpectedly allowed: %v", bad, argv)
		}
	}

	type want struct {
		app  string
		argv []string
	}
	var cases []want
	switch runtime.GOOS {
	case "darwin":
		cases = []want{
			{"vscode", []string{"open", "-a", "Visual Studio Code", dir}},
			{"finder", []string{"open", dir}},
			{"terminal", []string{"open", "-a", "Terminal", dir}},
		}
	case "linux":
		cases = []want{
			{"vscode", []string{"code", dir}},
			{"finder", []string{"xdg-open", dir}},
		}
		// terminal is intentionally unsupported on Linux.
		if _, ok := launchArgv("terminal", dir); ok {
			t.Fatalf("linux: terminal should be unsupported")
		}
	default:
		t.Skipf("no launcher table for GOOS=%s", runtime.GOOS)
	}

	for _, c := range cases {
		argv, ok := launchArgv(c.app, dir)
		if !ok {
			t.Fatalf("launchArgv(%q) rejected", c.app)
		}
		if !reflect.DeepEqual(argv, c.argv) {
			t.Fatalf("launchArgv(%q) = %v, want %v", c.app, argv, c.argv)
		}
		// Invariant across every target: the directory is the last argv element
		// and appears exactly once — it is never argv[0] (the executable).
		if argv[len(argv)-1] != dir {
			t.Fatalf("launchArgv(%q): dir must be the trailing arg, got %v", c.app, argv)
		}
		if argv[0] == dir {
			t.Fatalf("launchArgv(%q): dir must never be the executable", c.app)
		}
		count := 0
		for _, a := range argv {
			if a == dir {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("launchArgv(%q): dir appears %d times, want 1: %v", c.app, count, argv)
		}
	}
}

// openTestServer builds a server whose launcher exec is captured (never runs a
// real app) and whose known-workspace set is fixed, so /api/open can be driven
// offline.
func openTestServer(t *testing.T, known ...string) (*server, *[][]string) {
	t.Helper()
	var calls [][]string
	set := map[string]bool{}
	for _, w := range known {
		set[canonPath(w)] = true
	}
	s := &server{
		meta:       newMetaStore(filepath.Join(t.TempDir(), "meta.json")),
		workspaces: func(context.Context) map[string]bool { return set },
		launch: func(_ context.Context, argv []string) error {
			calls = append(calls, argv)
			return nil
		},
	}
	return s, &calls
}

func postOpen(t *testing.T, s *server, workspace, app string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"workspace": workspace, "app": app})
	req := httptest.NewRequest("POST", "/api/open", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleOpen(rec, req)
	return rec
}

// TestOpenRejectsUnknownApp: a non-whitelisted app token is refused and nothing
// is executed, even for a known workspace.
func TestOpenRejectsUnknownApp(t *testing.T) {
	ws := t.TempDir()
	s, calls := openTestServer(t, ws)
	rec := postOpen(t, s, ws, "/bin/sh")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(*calls) != 0 {
		t.Fatalf("launch was called for an unknown app: %v", *calls)
	}
}

// TestOpenRejectsUnknownWorkspace: an EXISTING directory that is not a known
// agent workspace is refused and nothing is executed — existence alone is not
// enough, it must be a known session workspace.
func TestOpenRejectsUnknownWorkspace(t *testing.T) {
	known := t.TempDir()
	stranger := t.TempDir() // exists, but not in the known set
	s, calls := openTestServer(t, known)
	rec := postOpen(t, s, stranger, "finder")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(*calls) != 0 {
		t.Fatalf("launch was called for an unknown workspace: %v", *calls)
	}
	// A path that does not exist at all is likewise refused.
	rec = postOpen(t, s, filepath.Join(known, "nope"), "finder")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("nonexistent path status = %d, want 400", rec.Code)
	}
	if len(*calls) != 0 {
		t.Fatalf("launch was called for a nonexistent path: %v", *calls)
	}
}

// TestOpenLaunchesKnownWorkspace: a known workspace + whitelisted app execs the
// correct argv exactly once and records last_opened in the overlay.
func TestOpenLaunchesKnownWorkspace(t *testing.T) {
	ws := t.TempDir()
	s, calls := openTestServer(t, ws)
	rec := postOpen(t, s, ws, "vscode")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(*calls) != 1 {
		t.Fatalf("launch called %d times, want 1", len(*calls))
	}
	argv := (*calls)[0]
	if argv[len(argv)-1] != ws {
		t.Fatalf("argv must end with the workspace dir: %v", argv)
	}
	want, _ := launchArgv("vscode", ws)
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	if got := s.meta.allProjects()[ws]; got.LastOpened == 0 {
		t.Fatalf("last_opened not recorded for %s: %+v", ws, got)
	}
}
