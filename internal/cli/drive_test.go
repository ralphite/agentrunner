package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider/scripted"
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
task: add a line
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
task: try
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
task: do a round
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
	if err := os.WriteFile(path, []byte("name: x\ntask: t\n"), 0o644); err != nil {
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
