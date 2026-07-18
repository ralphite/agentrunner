// Package driver is the IterationDriver (DESIGN §运行形态): a separate actor
// with its OWN event stream and pure fold that spawns a fresh child run per
// iteration and drives it toward a goal (goal mode) or on a schedule (loop
// mode, later). The driver never touches the LLM or the workspace itself —
// it orchestrates child runs and verifies their results.
package driver

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ralphite/agentrunner/internal/agent"
)

// DefaultMaxIterations bounds a goal-mode driver that omits max_iterations —
// a goal that never verifies must still terminate.
const DefaultMaxIterations = 10

// Schedule kinds. immediate = goal mode; interval / cron = loop mode on a
// fixed cadence; self_paced = loop mode where the CHILD declares the pace
// via the schedule_next / finish_series built-in tools.
const (
	ScheduleImmediate = "immediate"
	ScheduleInterval  = "interval"
	ScheduleCron      = "cron"
	ScheduleSelfPaced = "self_paced"
	// ScheduleParallel is best-of-N (S7, DESIGN §运行形态): N attempts in
	// isolated worktrees materialized from ONE base snapshot, judged by the
	// verifiers; the best verdict wins (pass beats score, ties keep the
	// earliest). v0 executes the attempts sequentially — the isolation is
	// the semantics, wall-clock concurrency is a deferred optimization.
	ScheduleParallel = "parallel"
)

// Overlap policies for a loop-mode tick that fires while the previous
// iteration is still running (or, sequentially, ticks that passed during it):
// skip journals IterationSkipped per missed tick and waits for the next
// future one; coalesce folds all missed ticks into ONE immediate iteration.
// interrupt (cancel the running iteration) needs concurrent launches and is
// deferred to the daemon module.
const (
	OverlapSkip     = "skip"
	OverlapCoalesce = "coalesce"
)

// Verifier kinds (DESIGN: a verifier is an effect through the four gates).
const (
	VerifierCommand  = "command"   // bash exit code / metric regex
	VerifierLLMJudge = "llm_judge" // LLM scores the result against a rubric
	VerifierHuman    = "human"     // a person answers via the ask path
)

// DriverSpec configures an IterationDriver. Goal mode = schedule immediate +
// at least one verifier; the driver re-iterates a fresh child run until a
// verifier is satisfied, the iteration budget is spent, or progress stalls.
type DriverSpec struct {
	Name string `yaml:"name"`
	// Schedule selects the mode; empty defaults to immediate (goal).
	Schedule string `yaml:"schedule,omitempty"`
	// AgentSpecPath names the child agent spec file, resolved relative to
	// the driver spec at load time into Agent.
	AgentSpecPath string `yaml:"agent_spec,omitempty"`
	// Interval is the loop-mode cadence (schedule=interval), a Go duration
	// string like "5m". Empty/zero runs iterations back to back.
	Interval string `yaml:"interval,omitempty"`
	// Cron is the loop-mode cadence (schedule=cron), five fields:
	// minute hour dom month dow.
	Cron string `yaml:"cron,omitempty"`
	// Overlap says what happens to ticks that pass while an iteration is
	// still running: skip (default; each missed tick journals
	// IterationSkipped) or coalesce (missed ticks fold into one immediate
	// iteration).
	Overlap string `yaml:"overlap,omitempty"`
	// PaceMin / PaceMax clamp a self_paced child's schedule_next request
	// (Go durations). Empty = unclamped on that side.
	PaceMin string `yaml:"pace_min,omitempty"`
	PaceMax string `yaml:"pace_max,omitempty"`
	// OnNoIntent is the self_paced fallback when an iteration ends without a
	// schedule_next or an approved finish_series: "finish" (default — a
	// child that stops asking is done) or "continue" (re-run at PaceMin,
	// which must then be set: a forgetful child must not spin the series).
	OnNoIntent string `yaml:"on_no_intent,omitempty"`
	// SeriesMemory is a workspace-relative path to the agent-managed series
	// document (DESIGN: series memory). When set, each iteration's prompt
	// carries the file's content — TRUNCATED at injection (the authority
	// boundary is here, not in the agent's discipline). Missing file = no
	// block (a first iteration has nothing to remember).
	SeriesMemory string `yaml:"series_memory,omitempty"`
	// Agent is the spec each iteration runs as a fresh child (same spec →
	// byte-stable prefix across iterations).
	Agent *agent.AgentSpec `yaml:"-"`
	// Prompt is the instruction every child iteration receives.
	Prompt string `yaml:"prompt"`
	// MaxIterations caps goal mode; zero means DefaultMaxIterations.
	MaxIterations int `yaml:"max_iterations,omitempty"`
	// N is the best-of-N attempt count (schedule=parallel); must be >= 2.
	N int `yaml:"n,omitempty"`
	// Verifiers are the goal-mode gates; ALL must pass for an iteration to
	// satisfy the goal. Required in goal mode.
	Verifiers []VerifierSpec `yaml:"verifiers,omitempty"`
	// Patience is stall detection: this many consecutive iterations with no
	// score improvement ends the run as stalled. Zero disables it.
	Patience int `yaml:"patience,omitempty"`
	// Budget caps the WHOLE driver tree (DESIGN: the driver is the tree
	// budget root; reserve-at-launch / settle-at-completion). Zero =
	// unlimited. Each iteration's child is capped at the min-aggregation of
	// the driver's remaining and the child spec's own cap.
	Budget BudgetSpec `yaml:"budget,omitempty"`
	// OnChildFailure is the policy for a child RUN that errors (a transport
	// failure or crash — not a verification miss, which is a normal
	// re-iterate). DESIGN: policy retry of a terminal failed run is not a
	// second recovery mechanism (principle 6 permits it).
	OnChildFailure FailurePolicy `yaml:"on_child_failure,omitempty"`
}

// BudgetSpec caps token spend.
type BudgetSpec struct {
	MaxTotalTokens int `yaml:"max_total_tokens,omitempty"`
}

// Child-failure policy modes.
const (
	OnFailStop    = "stop"    // default: end the driver as child_failed
	OnFailSurface = "surface" // record the failure, continue to the next iteration
	OnFailRetry   = "retry"   // re-run the same iteration up to Max extra times
)

// FailurePolicy governs a failed child run. Empty Mode is stop. Backoff is
// deferred to the scheduler module (it needs the durable-timer substrate);
// retry here is immediate and count-bounded.
type FailurePolicy struct {
	Mode string `yaml:"mode,omitempty"`
	Max  int    `yaml:"max,omitempty"`
}

// VerifierSpec is one goal-mode gate.
//   - command: a bash effect. A metric regex with capture group 1 turns it
//     into a scored verifier (score ≥ threshold passes); without it, exit
//     code 0 passes.
//   - llm_judge: an LLM scores the child's result against Rubric and returns
//     {score, pass, reason}; pass, if present, wins, else score ≥ threshold.
//   - human: a person answers the ask path with Rubric as the question.
type VerifierSpec struct {
	Kind        string  `yaml:"kind"`
	Command     string  `yaml:"command,omitempty"`
	MetricRegex string  `yaml:"metric_regex,omitempty"`
	Threshold   float64 `yaml:"threshold,omitempty"`
	Rubric      string  `yaml:"rubric,omitempty"`
}

// driverSpecFields lists DriverSpec's top-level yaml keys for the
// unknown-field hint. Keep in sync with the struct tags.
const driverSpecFields = "name, schedule, agent_spec, interval, cron, overlap, " +
	"pace_min, pace_max, on_no_intent, series_memory, prompt, max_iterations, n, " +
	"verifiers, patience, budget, on_child_failure"

// decodeHint rewrites a yaml decode error for a user who has never seen the
// Go structs behind the spec (same courtesy as agent.LoadSpec — QA Round2
// F-E3: it used to leak "type driver.DriverSpec" and name no valid fields).
const verifierSpecFields = "kind, command, metric_regex, threshold, rubric"

func decodeHint(err error) string {
	msg := err.Error()
	// Capture BOTH the field and the type it wasn't found in, so a stray key
	// inside a verifiers[] item names verifier fields, not the top-level driver
	// fields (QA Wave2 erin-04). The type name is stripped from the surfaced
	// text either way.
	if m := typeNameRe.FindStringSubmatch(msg); m != nil {
		fields := driverSpecFields
		where := "top-level driver"
		if strings.Contains(m[2], "VerifierSpec") {
			fields, where = verifierSpecFields, "verifier item"
		}
		msg = typeNameRe.ReplaceAllString(msg, `unknown field "$1"`)
		msg += fmt.Sprintf("\n  valid %s fields: %s", where, fields)
	}
	return msg
}

var typeNameRe = regexp.MustCompile(`field (\S+) not found in type (\S+)`)

// LoadSpec reads and validates a driver spec, resolving agent_spec into
// Agent (relative paths anchor at the driver spec's directory). Error format
// mirrors agent.LoadSpec: "driver spec <path>: field <name>: <problem>".
func LoadSpec(path string) (*DriverSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("driver spec %s: %w", path, err)
	}
	var spec DriverSpec
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("driver spec %s: %v", path, decodeHint(err))
	}
	fail := func(field, problem string) error {
		return fmt.Errorf("driver spec %s: field %s: %s", path, field, problem)
	}
	if spec.Name == "" {
		return nil, fail("name", "required")
	}
	if spec.Prompt == "" {
		return nil, fail("prompt", "required")
	}
	if spec.AgentSpecPath == "" {
		return nil, fail("agent_spec", "required")
	}
	// A negative max_iterations is a user error, not a budget: 0 is the
	// documented "use DefaultMaxIterations" sentinel, but a negative value used
	// to be silently coerced to the default too (QA Wave7 olive-02) — reject it
	// like agent.LoadSpec rejects a negative max_generation_steps.
	if spec.MaxIterations < 0 {
		return nil, fail("max_iterations", fmt.Sprintf("must be >= 0 (0 = default %d; got %d)", DefaultMaxIterations, spec.MaxIterations))
	}
	// n only means anything under schedule: parallel (best-of-N). Setting it on
	// any other schedule used to silently run the default loop, ignoring n with
	// no diagnostic — the reverse (parallel without n>=2) already errors, so the
	// validation was one-directional (QA Wave8 ravi-04).
	if spec.N != 0 && spec.schedule() != ScheduleParallel {
		return nil, fail("n", fmt.Sprintf("only applies to schedule: parallel (best-of-N); schedule is %q — set schedule: parallel or remove n", spec.schedule()))
	}
	// Verifier kinds resolve at PARSE time (QA Round2 F-E1: an empty kind
	// used to fail every check at runtime with the reason buried in
	// verdict.detail — the loop burned all its iterations in silence). A
	// bare `command:` can only mean kind command; anything else unknown is
	// an error here, not a silent never-pass.
	for i := range spec.Verifiers {
		v := &spec.Verifiers[i]
		if v.Kind == "" && v.Command != "" {
			v.Kind = VerifierCommand
		}
		field := fmt.Sprintf("verifiers[%d]", i)
		switch v.Kind {
		case VerifierCommand:
			if v.Command == "" {
				return nil, fail(field+".command", "required for kind command")
			}
		case VerifierLLMJudge, VerifierHuman:
			// runtime-served kinds; rubric/threshold are their own defaults
		case "":
			return nil, fail(field+".kind", "required (command | llm_judge | human)")
		default:
			return nil, fail(field+".kind", fmt.Sprintf("unknown kind %q (valid: command, llm_judge, human)", v.Kind))
		}
	}
	agentPath := spec.AgentSpecPath
	if !filepath.IsAbs(agentPath) {
		agentPath = filepath.Join(filepath.Dir(path), agentPath)
	}
	child, err := agent.LoadSpec(agentPath)
	if err != nil {
		return nil, fmt.Errorf("driver spec %s: field agent_spec: %v", path, err)
	}
	spec.Agent = child
	return &spec, nil
}

// schedule returns the effective schedule (immediate default).
func (s *DriverSpec) schedule() string {
	if s.Schedule == "" {
		return ScheduleImmediate
	}
	return s.Schedule
}

// maxIterations returns the effective cap.
func (s *DriverSpec) maxIterations() int {
	if s.MaxIterations <= 0 {
		return DefaultMaxIterations
	}
	return s.MaxIterations
}

// interval parses the loop-mode cadence; empty is zero (back-to-back).
func (s *DriverSpec) interval() (time.Duration, error) {
	if s.Interval == "" {
		return 0, nil
	}
	return time.ParseDuration(s.Interval)
}

// OnNoIntent modes (self_paced).
const (
	NoIntentFinish   = "finish"
	NoIntentContinue = "continue"
)

// paceBounds parses the self_paced clamp; empty sides are unbounded.
func (s *DriverSpec) paceBounds() (min, max time.Duration, err error) {
	if s.PaceMin != "" {
		if min, err = time.ParseDuration(s.PaceMin); err != nil {
			return 0, 0, fmt.Errorf("pace_min %q: %w", s.PaceMin, err)
		}
	}
	if s.PaceMax != "" {
		if max, err = time.ParseDuration(s.PaceMax); err != nil {
			return 0, 0, fmt.Errorf("pace_max %q: %w", s.PaceMax, err)
		}
	}
	if max > 0 && min > max {
		return 0, 0, fmt.Errorf("pace_min %s exceeds pace_max %s", s.PaceMin, s.PaceMax)
	}
	return min, max, nil
}
