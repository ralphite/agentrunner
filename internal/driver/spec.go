// Package driver is the IterationDriver (DESIGN §运行形态): a separate actor
// with its OWN event stream and pure fold that spawns a fresh child run per
// iteration and drives it toward a goal (goal mode) or on a schedule (loop
// mode, later). The driver never touches the LLM or the workspace itself —
// it orchestrates child runs and verifies their results.
package driver

import (
	"fmt"
	"time"

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
	// Agent is the spec each iteration runs as a fresh child (same spec →
	// byte-stable prefix across iterations).
	Agent *agent.AgentSpec `yaml:"-"`
	// Task is the instruction every child iteration receives.
	Task string `yaml:"task"`
	// MaxIterations caps goal mode; zero means DefaultMaxIterations.
	MaxIterations int `yaml:"max_iterations,omitempty"`
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
