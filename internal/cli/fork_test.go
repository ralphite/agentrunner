package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/store"
)

const forkSpecYAML = `name: writer
model: { provider: scripted, id: x }
system_prompt: write things
tools: [edit_file]
permissions:
  - { action: allow }
`

const forkParentFixtureYAML = `steps:
  - respond:
      - tool_call: { name: edit_file, args: { path: note.txt, old: v1, new: v2 } }
      - finish: tool_use
  - respond:
      - text: bumped to v2
      - finish: end_turn
`

const forkResumeFixtureYAML = `steps:
  - respond:
      - tool_call: { name: edit_file, args: { path: note.txt, old: v1, new: FORKED } }
      - finish: tool_use
  - respond:
      - text: forked path done
      - finish: end_turn
`

// S7.3 e2e (s7-01/s7-02 semantics): run two turns (v1 → v2), fork at
// bar-t1, and the fork's worktree holds the PRE-EDIT state while the
// original keeps v2; resuming the fork continues in its own worktree and
// never touches the original.
func TestCLIForkRewindsAndContinues(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg"))
	base := t.TempDir()
	ws := filepath.Join(base, "ws")
	if err := os.Mkdir(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "note.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(base, "spec.yaml")
	if err := os.WriteFile(specPath, []byte(forkSpecYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	fix := filepath.Join(base, "fix.yaml")
	if err := os.WriteFile(fix, []byte(forkParentFixtureYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTRUNNER_SCRIPTED_FIXTURE", fix)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--workspace", ws, specPath, "bump the note"}, "dev", &stdout, &stderr); code != ExitOK {
		t.Fatalf("run exit = %d\n%s", code, stderr.String())
	}
	var parent string
	for _, line := range strings.Split(stderr.String(), "\n") {
		if rest, ok := strings.CutPrefix(line, "session "); ok {
			parent = rest
			break
		}
	}
	if parent == "" {
		t.Fatalf("no session line in:\n%s", stderr.String())
	}

	// --list shows the turn barriers plus the terminal one.
	stdout.Reset()
	if code := Run([]string{"fork", "--list", parent}, "dev", &stdout, &stderr); code != ExitOK {
		t.Fatalf("fork --list exit = %d\n%s", code, stderr.String())
	}
	for _, want := range []string{"bar-t1", "bar-t2", "bar-final"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("--list missing %s:\n%s", want, stdout.String())
		}
	}

	// Fork at bar-t1: the cut before the edit.
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"fork", parent, "bar-t1"}, "dev", &stdout, &stderr); code != ExitOK {
		t.Fatalf("fork exit = %d\n%s", code, stderr.String())
	}
	var forkSession, forkWS string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if rest, ok := strings.CutPrefix(line, "session "); ok {
			forkSession = rest
		}
		if rest, ok := strings.CutPrefix(line, "workspace "); ok {
			forkWS = rest
		}
	}
	if forkSession == "" || forkWS == "" {
		t.Fatalf("fork output missing session/workspace:\n%s", stdout.String())
	}

	// Rewind semantics: the fork's worktree is the barrier state (v1); the
	// original run's worktree still has the edit (v2).
	got, err := os.ReadFile(filepath.Join(forkWS, "note.txt"))
	if err != nil || string(got) != "v1" {
		t.Fatalf("fork note.txt = %q err=%v, want v1 (rewound)", got, err)
	}
	if got, _ := os.ReadFile(filepath.Join(ws, "note.txt")); string(got) != "v2" {
		t.Fatalf("original note.txt = %q, want v2 (untouched)", got)
	}

	// The fork continues from turn 1 in its OWN worktree.
	resumeFix := filepath.Join(base, "resume.yaml")
	if err := os.WriteFile(resumeFix, []byte(forkResumeFixtureYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTRUNNER_SCRIPTED_FIXTURE", resumeFix)
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"resume", forkSession}, "dev", &stdout, &stderr); code != ExitOK {
		t.Fatalf("resume fork exit = %d\n%s", code, stderr.String())
	}
	if got, _ := os.ReadFile(filepath.Join(forkWS, "note.txt")); string(got) != "FORKED" {
		t.Errorf("fork note.txt after resume = %q, want FORKED", got)
	}
	if got, _ := os.ReadFile(filepath.Join(ws, "note.txt")); string(got) != "v2" {
		t.Errorf("original note.txt after fork resume = %q, want v2", got)
	}

	// The fork journal: forked_from genesis, run_ended tail.
	forkDir, err := resolveSessionDir(forkSession)
	if err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(forkDir)
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Type != event.TypeForkedFrom {
		t.Errorf("fork journal head = %s", events[0].Type)
	}
	if events[len(events)-1].Type != event.TypeRunEnded {
		t.Errorf("fork journal tail = %s", events[len(events)-1].Type)
	}
}

func TestCLIForkUnknownBarrier(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var out, errOut bytes.Buffer
	if code := Run([]string{"fork", "nope", "bar-t1"}, "dev", &out, &errOut); code != ExitUsage {
		t.Fatalf("exit = %d", code)
	}
}

// S7 出口 review: the explicit barrier entry (PLAN 模块 2 承诺) — barrier
// an ENDED session at its current workspace state, then fork that barrier.
func TestCLIManualBarrierThenFork(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg"))
	base := t.TempDir()
	ws := filepath.Join(base, "ws")
	if err := os.Mkdir(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "note.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(base, "spec.yaml")
	if err := os.WriteFile(specPath, []byte(forkSpecYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	fix := filepath.Join(base, "fix.yaml")
	if err := os.WriteFile(fix, []byte(forkParentFixtureYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTRUNNER_SCRIPTED_FIXTURE", fix)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--workspace", ws, specPath, "bump"}, "dev", &stdout, &stderr); code != ExitOK {
		t.Fatalf("run exit = %d\n%s", code, stderr.String())
	}
	var session string
	for _, line := range strings.Split(stderr.String(), "\n") {
		if rest, ok := strings.CutPrefix(line, "session "); ok {
			session = rest
			break
		}
	}

	// The run ended with note.txt = v2; edit it OUT OF BAND to v3, then cut
	// a manual barrier — it must capture the PRESENT state.
	if err := os.WriteFile(filepath.Join(ws, "note.txt"), []byte("v3"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if code := Run([]string{"barrier", session}, "dev", &stdout, &stderr); code != ExitOK {
		t.Fatalf("barrier exit = %d\n%s", code, stderr.String())
	}
	var barrierID string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if rest, ok := strings.CutPrefix(line, "barrier "); ok {
			barrierID = rest
			break
		}
	}
	if !strings.HasPrefix(barrierID, "bar-m") {
		t.Fatalf("barrier id = %q", barrierID)
	}

	stdout.Reset()
	if code := Run([]string{"fork", "--workspace", filepath.Join(base, "forkws"), session, barrierID}, "dev", &stdout, &stderr); code != ExitOK {
		t.Fatalf("fork exit = %d\n%s", code, stderr.String())
	}
	got, err := os.ReadFile(filepath.Join(base, "forkws", "note.txt"))
	if err != nil || string(got) != "v3" {
		t.Errorf("fork note.txt = %q err=%v, want the manual barrier's v3", got, err)
	}
}
