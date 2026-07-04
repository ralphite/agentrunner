// Package driver is the IterationDriver (DESIGN §运行形态): a separate actor
// with its OWN event stream and pure fold that spawns a fresh child run per
// iteration and drives it toward a goal (goal mode) or on a schedule (loop
// mode, later). The driver never touches the LLM or the workspace itself —
// it orchestrates child runs and verifies their results.
package driver

import "github.com/ralphite/agentrunner/internal/agent"

// DefaultMaxIterations bounds a goal-mode driver that omits max_iterations —
// a goal that never verifies must still terminate.
const DefaultMaxIterations = 10

// Schedule kinds. v0 implements goal mode only (immediate); loop mode
// (interval/cron/self_paced) arrives with the scheduler module.
const (
	ScheduleImmediate = "immediate"
)

// Verifier kinds (DESIGN: a verifier is an effect through the four gates).
const (
	VerifierCommand = "command" // bash exit code / metric regex
	// VerifierLLMJudge and VerifierHuman arrive in a follow-up step.
)

// DriverSpec configures an IterationDriver. Goal mode = schedule immediate +
// at least one verifier; the driver re-iterates a fresh child run until a
// verifier is satisfied, the iteration budget is spent, or progress stalls.
type DriverSpec struct {
	Name string `yaml:"name"`
	// Schedule selects the mode; empty defaults to immediate (goal).
	Schedule string `yaml:"schedule,omitempty"`
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
}

// BudgetSpec caps token spend.
type BudgetSpec struct {
	MaxTotalTokens int `yaml:"max_total_tokens,omitempty"`
}

// VerifierSpec is one goal-mode gate. v0: command (bash). A metric regex with
// capture group 1 turns the command into a scored verifier (score ≥ threshold
// passes); without it, exit code 0 passes.
type VerifierSpec struct {
	Kind        string  `yaml:"kind"`
	Command     string  `yaml:"command,omitempty"`
	MetricRegex string  `yaml:"metric_regex,omitempty"`
	Threshold   float64 `yaml:"threshold,omitempty"`
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
