package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
)

func writeDriverSpecs(t *testing.T, dir, driverYAML string) string {
	t.Helper()
	worker := `name: worker
model: { provider: scripted, id: x }
system_prompt: make progress
tools: [bash]
permissions:
  - { action: allow }
`
	if err := os.WriteFile(filepath.Join(dir, "worker.yaml"), []byte(worker), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "driver.yaml")
	if err := os.WriteFile(path, []byte(driverYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// drive: a goal-mode driver reaches satisfied through the CLI seam — the
// scripted provider is shared across iterations, so the fixture scripts the
// whole series in order.
func TestDriveGoalEndToEnd(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	ws := t.TempDir()
	specPath := writeDriverSpecs(t, dir, `name: fill-progress
agent_spec: worker.yaml
prompt: add a line
max_iterations: 3
verifiers:
  - { kind: command, command: "test $(wc -l < progress.txt) -ge 2" }
`)

	fix := scripted.Fixture{Steps: []scripted.Step{
		// iteration 1
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "bash",
				Args: map[string]any{"command": "echo tick >> progress.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "one line in"}, {Finish: "end_turn"}}},
		// iteration 2
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "bash",
				Args: map[string]any{"command": "echo tick >> progress.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "two lines in"}, {Finish: "end_turn"}}},
	}}

	var out, errOut bytes.Buffer
	code := driveAgent(driveOptions{
		specPath:  specPath,
		workspace: ws,
		factory:   scriptedFactory(fix),
		stdout:    &out,
		stderr:    &errOut,
	})
	if code != ExitOK {
		t.Fatalf("exit = %d\nstderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "driver satisfied: 2 iterations") {
		t.Errorf("stderr = %q, want the satisfied summary", errOut.String())
	}
	raw, err := os.ReadFile(filepath.Join(ws, "progress.txt"))
	if err != nil || strings.Count(string(raw), "tick") != 2 {
		t.Errorf("progress.txt = %q, err %v", raw, err)
	}
}

// drive: a goal that never verifies exits nonzero (max_iterations is not
// success in goal mode).
func TestDriveGoalUnsatisfiedExitsNonzero(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	specPath := writeDriverSpecs(t, dir, `name: never
agent_spec: worker.yaml
prompt: try
max_iterations: 1
verifiers:
  - { kind: command, command: "false" }
`)
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "tried"}, {Finish: "end_turn"}}},
	}}
	var out, errOut bytes.Buffer
	code := driveAgent(driveOptions{
		specPath: specPath, workspace: t.TempDir(),
		factory: scriptedFactory(fix), stdout: &out, stderr: &errOut,
	})
	if code != ExitRun {
		t.Fatalf("exit = %d, want %d (goal not reached)\nstderr: %s", code, ExitRun, errOut.String())
	}
}

// drive: a bounded loop-mode series ending at max_iterations exits zero.
func TestDriveLoopBoundedSeriesExitsZero(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	specPath := writeDriverSpecs(t, dir, `name: rounds
agent_spec: worker.yaml
schedule: interval
prompt: do a round
max_iterations: 2
`)
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "round 1"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "round 2"}, {Finish: "end_turn"}}},
	}}
	var out, errOut bytes.Buffer
	code := driveAgent(driveOptions{
		specPath: specPath, workspace: t.TempDir(),
		factory: scriptedFactory(fix), stdout: &out, stderr: &errOut,
	})
	if code != ExitOK {
		t.Fatalf("exit = %d\nstderr: %s", code, errOut.String())
	}
}

// drive: spec errors are usage errors.
func TestDriveSpecErrors(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("name: x\nprompt: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := driveAgent(driveOptions{
		specPath: path, workspace: t.TempDir(),
		factory: scriptedFactory(scripted.Fixture{}), stdout: &out, stderr: &errOut,
	})
	if code != ExitUsage || !strings.Contains(errOut.String(), "agent_spec") {
		t.Fatalf("exit = %d, stderr = %q — want usage error naming agent_spec", code, errOut.String())
	}
}

// drive --series (INC-80.2a opt-in): the SAME goal series journals as a
// SESSION — head SessionStarted + SeriesStarted, terminal SeriesEnded, no
// DriverStarted anywhere — and still reaches satisfied through the CLI seam.
func TestDriveSeriesGoalJournalsMergedStream(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	dir := t.TempDir()
	ws := t.TempDir()
	specPath := writeDriverSpecs(t, dir, `name: fill-progress
agent_spec: worker.yaml
prompt: add a line
max_iterations: 3
verifiers:
  - { kind: command, command: "test $(wc -l < progress.txt) -ge 1" }
`)
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "bash",
				Args: map[string]any{"command": "echo tick >> progress.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "one line in"}, {Finish: "end_turn"}}},
	}}
	var out, errOut bytes.Buffer
	code := driveAgent(driveOptions{
		specPath: specPath, workspace: ws,
		factory: scriptedFactory(fix), stdout: &out, stderr: &errOut,
		series: true,
	})
	if code != ExitOK {
		t.Fatalf("exit = %d\nstderr: %s", code, errOut.String())
	}
	sessions, err := os.ReadDir(filepath.Join(xdg, "agentrunner", "sessions"))
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %v, err %v", sessions, err)
	}
	events, err := store.ReadEvents(filepath.Join(xdg, "agentrunner", "sessions", sessions[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Type != event.TypeSessionStarted {
		t.Fatalf("journal head = %s, want session_started (merged stream)", events[0].Type)
	}
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Type] = true
	}
	if !seen[event.TypeSeriesStarted] || !seen[event.TypeSeriesEnded] {
		t.Fatalf("series facts missing: started=%v ended=%v", seen[event.TypeSeriesStarted], seen[event.TypeSeriesEnded])
	}
	if seen[event.TypeDriverStarted] {
		t.Fatal("merged-stream series journaled a DriverStarted — legacy stream leaked")
	}
}

// drive --series refuses the one spec shape the merged form does not carry
// (parallel × on_child_failure=retry) instead of silently changing
// semantics.
func TestDriveSeriesRefusesUnsupportedSpec(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	specPath := writeDriverSpecs(t, dir, `name: bestof
agent_spec: worker.yaml
prompt: go
schedule: parallel
n: 3
on_child_failure: { mode: retry, max: 1 }
verifiers:
  - { kind: command, command: "true" }
`)
	var out, errOut bytes.Buffer
	code := driveAgent(driveOptions{
		specPath: specPath, workspace: t.TempDir(),
		factory: scriptedFactory(scripted.Fixture{}), stdout: &out, stderr: &errOut,
		series: true,
	})
	if code != ExitUsage {
		t.Fatalf("exit = %d, want usage refusal\nstderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "--series carries") {
		t.Errorf("stderr = %q, want the routing hint", errOut.String())
	}
}

// A merged-stream series session refuses the conversational resume path —
// the daemon's drive sweep owns it (INC-80.2a).
func TestCLIResumeRefusesSeriesSession(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeSeriesJournal(t, "series-conv-guard", "interval", false)
	var out, errOut bytes.Buffer
	code := resumeCmd([]string{"series-conv-guard"}, "test", &out, &errOut)
	if code != ExitUsage {
		t.Fatalf("exit = %d, want usage refusal\nstderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "series session") {
		t.Errorf("stderr = %q, want the series refusal", errOut.String())
	}
}

// INC-80.2c flip: with NO --series flag, an eligible spec journals the
// merged stream by default — the legacy DriverStarted form is now reserved
// for the shapes the runner does not carry (self_paced/parallel/retry).
func TestDriveDefaultsToMergedStreamForEligibleSpec(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	dir := t.TempDir()
	specPath := writeDriverSpecs(t, dir, `name: quick
agent_spec: worker.yaml
prompt: touch
max_iterations: 1
verifiers:
  - { kind: command, command: "true" }
`)
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	var out, errOut bytes.Buffer
	if code := driveAgent(driveOptions{
		specPath: specPath, workspace: t.TempDir(),
		factory: scriptedFactory(fix), stdout: &out, stderr: &errOut,
	}); code != ExitOK {
		t.Fatalf("exit = %d\nstderr: %s", code, errOut.String())
	}
	sessions, err := os.ReadDir(filepath.Join(xdg, "agentrunner", "sessions"))
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %v, err %v", sessions, err)
	}
	events, err := store.ReadEvents(filepath.Join(xdg, "agentrunner", "sessions", sessions[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Type != event.TypeSessionStarted {
		t.Fatalf("default journal head = %s, want session_started (the 2.2c flip)", events[0].Type)
	}
}

// drive --retry seeds from EITHER journal form: a merged-stream series
// session's spec rides its SessionStarted head.
func TestDriveRetryReadsSeriesHead(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	specPath := writeDriverSpecs(t, dir, `name: seed
agent_spec: worker.yaml
prompt: touch
max_iterations: 1
verifiers:
  - { kind: command, command: "true" }
`)
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done again"}, {Finish: "end_turn"}}},
	}}
	factory := scriptedFactory(fix)
	var out, errOut bytes.Buffer
	if code := driveAgent(driveOptions{
		specPath: specPath, workspace: t.TempDir(),
		factory: factory, stdout: &out, stderr: &errOut,
	}); code != ExitOK {
		t.Fatalf("seed exit = %d\nstderr: %s", code, errOut.String())
	}
	// The seed journaled as a series session; retry must reload its spec
	// from the SessionStarted head (embedded agent rides the spec).
	code := driveCmd([]string{"--retry", "seed"}, "dev", &out, &errOut)
	if strings.Contains(errOut.String(), "no reusable") || strings.Contains(errOut.String(), "no DriverStarted") {
		t.Fatalf("retry could not read the series head: %s", errOut.String())
	}
	_ = code // provider wiring differs on this path; the head-read is the contract under test
}
