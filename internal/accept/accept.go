// Package accept is the acceptance-test harness (PLAN 0.6): stage
// completion criteria as executable, human-readable scenarios with a TUI /
// plain renderer and a JSON report.
package accept

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ralphite/agentrunner/internal/event"
)

//go:embed scenarios/s*/*.yaml
var scenariosFS embed.FS

// Scenario is one executable acceptance case (data, human-readable title).
type Scenario struct {
	ID       string            `yaml:"id"`
	Title    string            `yaml:"title"`
	Requires []string          `yaml:"requires,omitempty"` // "live" | "testbed"
	Files    map[string]string `yaml:"files,omitempty"`    // written into SCRATCH before steps
	Steps    []Step            `yaml:"steps"`
	Expect   []Expect          `yaml:"expect"`
}

// Step runs one shell command in the scratch dir.
type Step struct {
	Run string `yaml:"run"`
}

// Expect is one assertion; exactly one field is set.
type Expect struct {
	ExitCode       *int        `yaml:"exit_code,omitempty"`       // of the last step
	OutputContains string      `yaml:"output_contains,omitempty"` // across all steps
	FileContains   *FileExpect `yaml:"file_contains,omitempty"`
	EventsValid    string      `yaml:"events_valid,omitempty"` // glob under SCRATCH
}

// FileExpect asserts file content under SCRATCH.
type FileExpect struct {
	Path string `yaml:"path"`
	Text string `yaml:"text"`
}

// Status of one scenario run.
type Status string

const (
	StatusPass    Status = "PASS"
	StatusFail    Status = "FAIL"
	StatusSkipped Status = "SKIPPED"
	StatusAborted Status = "ABORTED" // scenario never ran (user quit the TUI)
)

// Result is the outcome of one scenario.
type Result struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Status   Status        `json:"status"`
	Duration time.Duration `json:"duration_ns"`
	Detail   string        `json:"detail,omitempty"` // failure reason / skip reason / output tail
}

// LoadStage reads the embedded scenarios for one stage, sorted by id.
func LoadStage(stage int) ([]Scenario, error) {
	dir := fmt.Sprintf("scenarios/s%d", stage)
	entries, err := fs.ReadDir(scenariosFS, dir)
	if err != nil {
		return nil, fmt.Errorf("no acceptance scenarios for stage %d", stage)
	}
	var scenarios []Scenario
	for _, e := range entries {
		raw, err := scenariosFS.ReadFile(dir + "/" + e.Name())
		if err != nil {
			return nil, err
		}
		s, err := parseScenario(e.Name(), raw)
		if err != nil {
			return nil, err
		}
		scenarios = append(scenarios, s)
	}
	sort.Slice(scenarios, func(i, j int) bool { return scenarios[i].ID < scenarios[j].ID })
	return scenarios, nil
}

// parseScenario decodes strictly (unknown keys are errors — a typo'd expect
// key must never become a silently-passing zero assertion) and validates
// that every Expect sets exactly one field.
func parseScenario(name string, raw []byte) (Scenario, error) {
	var s Scenario
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return Scenario{}, fmt.Errorf("scenario %s: %v", name, err)
	}
	if s.ID == "" || s.Title == "" || len(s.Steps) == 0 || len(s.Expect) == 0 {
		return Scenario{}, fmt.Errorf("scenario %s: id/title/steps/expect are required", name)
	}
	for i, exp := range s.Expect {
		if n := exp.fieldsSet(); n != 1 {
			return Scenario{}, fmt.Errorf("scenario %s: expect[%d] must set exactly one assertion, has %d", name, i, n)
		}
	}
	return s, nil
}

func (e Expect) fieldsSet() int {
	n := 0
	if e.ExitCode != nil {
		n++
	}
	if e.OutputContains != "" {
		n++
	}
	if e.FileContains != nil {
		n++
	}
	if e.EventsValid != "" {
		n++
	}
	return n
}

// Runner executes scenarios against a built agentrunner binary.
type Runner struct {
	Bin string // path to the agentrunner binary (os.Executable for the CLI)
}

// Run executes one scenario in a fresh scratch dir.
func (r *Runner) Run(s Scenario) Result {
	start := time.Now()
	res := Result{ID: s.ID, Title: s.Title}

	if reason, skip := r.shouldSkip(s); skip {
		res.Status = StatusSkipped
		res.Detail = reason
		res.Duration = time.Since(start)
		return res
	}

	scratch, err := os.MkdirTemp("", "accept-"+s.ID+"-")
	if err != nil {
		return r.fail(res, start, "scratch dir: "+err.Error(), "")
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	for path, content := range s.Files {
		full := filepath.Join(scratch, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return r.fail(res, start, err.Error(), "")
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return r.fail(res, start, err.Error(), "")
		}
	}

	var allOutput strings.Builder
	lastExit := 0
	for i, step := range s.Steps {
		cmd := exec.Command("sh", "-c", step.Run)
		cmd.Dir = scratch
		cmd.Env = append(os.Environ(),
			"BIN="+r.Bin,
			"SCRATCH="+scratch,
			"XDG_DATA_HOME="+filepath.Join(scratch, "xdg"),
		)
		out, err := cmd.CombinedOutput()
		allOutput.Write(out)
		lastExit = cmd.ProcessState.ExitCode()
		if err != nil && i < len(s.Steps)-1 {
			// Intermediate steps must succeed; only the last step's exit
			// code is subject to expectations.
			return r.fail(res, start, fmt.Sprintf("step %d failed: %v", i+1, err), allOutput.String())
		}
	}

	for _, exp := range s.Expect {
		if msg := checkExpect(exp, scratch, allOutput.String(), lastExit); msg != "" {
			return r.fail(res, start, msg, allOutput.String())
		}
	}

	res.Status = StatusPass
	res.Duration = time.Since(start)
	return res
}

func (r *Runner) shouldSkip(s Scenario) (string, bool) {
	for _, req := range s.Requires {
		switch req {
		case "live":
			if os.Getenv("GEMINI_API_KEY") == "" {
				return "requires live credentials (GEMINI_API_KEY unset)", true
			}
		case "testbed":
			if os.Getenv("AGENTRUNNER_TESTBED") == "" {
				return "requires testbed (AGENTRUNNER_TESTBED unset)", true
			}
		}
	}
	return "", false
}

func (r *Runner) fail(res Result, start time.Time, msg, output string) Result {
	res.Status = StatusFail
	res.Detail = msg
	if output != "" {
		res.Detail += "\n--- output ---\n" + tail(output, 2000)
	}
	res.Duration = time.Since(start)
	return res
}

func checkExpect(exp Expect, scratch, output string, lastExit int) string {
	switch {
	case exp.ExitCode != nil:
		if lastExit != *exp.ExitCode {
			return fmt.Sprintf("exit_code = %d, want %d", lastExit, *exp.ExitCode)
		}
	case exp.OutputContains != "":
		if !strings.Contains(output, exp.OutputContains) {
			return fmt.Sprintf("output does not contain %q", exp.OutputContains)
		}
	case exp.FileContains != nil:
		raw, err := os.ReadFile(filepath.Join(scratch, exp.FileContains.Path))
		if err != nil {
			return fmt.Sprintf("file_contains: %v", err)
		}
		if !strings.Contains(string(raw), exp.FileContains.Text) {
			return fmt.Sprintf("%s does not contain %q", exp.FileContains.Path, exp.FileContains.Text)
		}
	case exp.EventsValid != "":
		return checkEvents(filepath.Join(scratch, exp.EventsValid))
	}
	return ""
}

// checkEvents verifies every matched event log is complete: ≥1 line, every
// line a well-formed envelope with gapless seq, first event session_started and
// last event run_ended (a truncated log must not pass).
func checkEvents(glob string) string {
	matches, err := filepath.Glob(glob)
	if err != nil || len(matches) == 0 {
		return fmt.Sprintf("events_valid: no files match %s", glob)
	}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err.Error()
		}
		lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
		if len(lines) == 0 || lines[0] == "" {
			return fmt.Sprintf("events_valid: %s is empty", path)
		}
		var types []string
		for i, line := range lines {
			var rec struct {
				Seq     int64           `json:"seq"`
				ID      string          `json:"id"`
				Type    string          `json:"type"`
				TS      string          `json:"ts"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := json.Unmarshal([]byte(line), &rec); err != nil || rec.Type == "" || len(rec.Payload) == 0 {
				return fmt.Sprintf("events_valid: %s line %d malformed: %s", path, i+1, line)
			}
			if _, err := time.Parse(time.RFC3339Nano, rec.TS); err != nil {
				return fmt.Sprintf("events_valid: %s line %d bad ts %q", path, i+1, rec.TS)
			}
			if rec.Seq != int64(i+1) {
				return fmt.Sprintf("events_valid: %s line %d seq = %d, want %d (gapless)", path, i+1, rec.Seq, i+1)
			}
			if want := fmt.Sprintf("evt-%d", rec.Seq); rec.ID != want {
				return fmt.Sprintf("events_valid: %s line %d id = %q, want %q", path, i+1, rec.ID, want)
			}
			if _, known := event.Registry[rec.Type]; !known {
				return fmt.Sprintf("events_valid: %s line %d unknown event type %q", path, i+1, rec.Type)
			}
			types = append(types, rec.Type)
		}
		// Three journal shapes share the format: a RUN journal opens with
		// session_started (a FORKED run with its forked_from genesis, S7.3) and
		// closes with run_ended; a DRIVER stream opens with driver_started
		// (S7 header; S6 streams opened with the first iteration_scheduled)
		// and closes with driver_completed.
		first, last := types[0], types[len(types)-1]
		switch first {
		case "session_started", "forked_from":
			if last != "run_ended" {
				return fmt.Sprintf("events_valid: %s last event is %q, want run_ended (truncated?)", path, last)
			}
		case "driver_started", "iteration_scheduled":
			if last != "driver_completed" {
				return fmt.Sprintf("events_valid: %s last event is %q, want driver_completed (truncated?)", path, last)
			}
		default:
			return fmt.Sprintf("events_valid: %s first event is %q, want session_started, forked_from, driver_started or iteration_scheduled", path, first)
		}
	}
	return ""
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
