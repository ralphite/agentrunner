package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/store"
)

const resumeSpecYAML = `name: fixer
model: { provider: scripted, id: x }
system_prompt: fix things
tools: [read_file, edit_file]
permissions:
  - { action: allow }
`

const crashFixtureYAML = `steps:
  - respond:
      - tool_call: { name: read_file, args: { path: greet.txt } }
      - tool_call: { name: edit_file, args: { path: greet.txt, old: hello world, new: HELLO WORLD } }
      - finish: tool_use
  - respond:
      - text: unreachable before crash
      - finish: end_turn
`

const resumeFixtureYAML = `steps:
  - respond:
      - text: all done
      - finish: end_turn
`

// End-to-end CLI crash + resume: `run` dies at the turn-2 boundary in a
// subprocess; `resume <prefix>` reconstructs the loop from session_started
// (spec + workspace root journaled) and finishes the run.
func TestCLIResumeAfterCrash(t *testing.T) {
	if os.Getenv("GO_CRASH_HELPER") == "1" {
		os.Exit(Run([]string{"run", "--workspace", os.Getenv("CRASH_WS"),
			os.Getenv("CRASH_SPEC"), "make it loud"}, "dev", os.Stdout, os.Stderr))
	}

	base := t.TempDir()
	xdg := filepath.Join(base, "xdg")
	ws := filepath.Join(base, "ws")
	if err := os.Mkdir(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "greet.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(base, "spec.yaml")
	if err := os.WriteFile(specPath, []byte(resumeSpecYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	crashFix := filepath.Join(base, "crash.yaml")
	if err := os.WriteFile(crashFix, []byte(crashFixtureYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCLIResumeAfterCrash")
	cmd.Env = append(os.Environ(),
		"GO_CRASH_HELPER=1",
		"XDG_DATA_HOME="+xdg,
		"CRASH_WS="+ws,
		"CRASH_SPEC="+specPath,
		"AGENTRUNNER_SCRIPTED_FIXTURE="+crashFix,
		crash.EnvVar+"=after:generation_started:2",
	)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != crash.ExitCode {
		t.Fatalf("run subprocess: err = %v, out = %s", err, out)
	}

	// Resume in-process through the CLI with the remaining fixture.
	t.Setenv("XDG_DATA_HOME", xdg)
	resumeFix := filepath.Join(base, "resume.yaml")
	if err := os.WriteFile(resumeFix, []byte(resumeFixtureYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTRUNNER_SCRIPTED_FIXTURE", resumeFix)
	t.Setenv(crash.EnvVar, "")

	var stdout, stderr bytes.Buffer
	// The session id begins with the date; a bare prefix that matches the
	// single session is enough.
	sessions, err := os.ReadDir(filepath.Join(xdg, "agentrunner", "sessions"))
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %v (err %v)", sessions, err)
	}
	code := Run([]string{"resume", sessions[0].Name()[:8]}, "dev", &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("resume exit = %d\nstderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "run completed: 2 turns") {
		t.Errorf("stderr = %s", stderr.String())
	}
	got, _ := os.ReadFile(filepath.Join(ws, "greet.txt"))
	if string(got) != "HELLO WORLD" {
		t.Errorf("file = %q", got)
	}

	// sessions list shows the delivery receipt.
	stdout.Reset()
	if code := Run([]string{"sessions", "list"}, "dev", &stdout, &stderr); code != ExitOK {
		t.Fatalf("sessions list exit = %d", code)
	}
	if !strings.Contains(stdout.String(), "completed") {
		t.Errorf("sessions list:\n%s", stdout.String())
	}

	// The product surfaces need durable workspace/title metadata for every
	// session, not only sessions they created themselves.
	stdout.Reset()
	if code := Run([]string{"sessions", "list", "--json"}, "dev", &stdout, &stderr); code != ExitOK {
		t.Fatalf("sessions json exit = %d: %s", code, stderr.String())
	}
	var listed []struct {
		ID        string `json:"id"`
		Workspace string `json:"workspace"`
		Title     string `json:"title"`
		Kind      string `json:"kind"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatalf("decode sessions json: %v\n%s", err, stdout.String())
	}
	wantWorkspace, err := filepath.EvalSymlinks(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Workspace != wantWorkspace || listed[0].Title != "make it loud" || listed[0].Kind != "session" {
		t.Fatalf("sessions json = %+v, want workspace %q and journaled title", listed, wantWorkspace)
	}
}

func TestCLIResumeUnknownSession(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var out, errOut bytes.Buffer
	if code := Run([]string{"resume", "nope"}, "dev", &out, &errOut); code != ExitUsage {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
}

func TestCLISessionsListEmpty(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var out, errOut bytes.Buffer
	if code := Run([]string{"sessions", "list"}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "no sessions") {
		t.Errorf("out = %q", out.String())
	}
	out.Reset()
	if code := Run([]string{"sessions", "--json"}, "dev", &out, &errOut); code != ExitOK || strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("empty json exit = %d, out = %q, err = %q", code, out.String(), errOut.String())
	}
}

func TestCLISessionsJSONProjectsDriverMetadata(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	dir := filepath.Join(xdg, "agentrunner", "sessions", "driver-1")
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := json.Marshal(map[string]any{
		"Name": "nightly",
		"Task": "Audit dependencies every night",
	})
	for _, item := range []struct {
		typ string
		v   any
	}{
		{event.TypeDriverStarted, &event.DriverStarted{DriverID: "driver-1", SpecName: "nightly", Spec: spec, WorkspaceRoot: "/tmp/project", FoldVersion: 1}},
		{event.TypeDriverCompleted, &event.DriverCompleted{DriverID: "driver-1", Reason: "satisfied", Iterations: 1}},
	} {
		if _, err := es.Append(mkEnv(t, item.typ, item.v)); err != nil {
			t.Fatal(err)
		}
	}
	if err := es.Close(); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"sessions", "--json"}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("sessions exit=%d stderr=%s", code, errOut.String())
	}
	var rows []struct {
		ID       string `json:"id"`
		Kind     string `json:"kind"`
		Schedule string `json:"schedule"`
		Title    string `json:"title"`
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "driver-1" || rows[0].Kind != "driver" || rows[0].Schedule != "immediate" || rows[0].Title != "Audit dependencies every night" {
		t.Fatalf("driver row = %+v", rows)
	}
}

func TestCLITrust(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	ws := t.TempDir()
	var out, errOut bytes.Buffer
	if code := Run([]string{"trust", ws}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "trusted") {
		t.Errorf("out = %q", out.String())
	}
	if code := Run([]string{"trust"}, "dev", &out, &errOut); code != ExitUsage {
		t.Fatalf("no-arg exit = %d", code)
	}
}
