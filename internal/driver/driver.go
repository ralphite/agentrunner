package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
)

// carryExcerptBytes caps the inline carry kept in IterationCompleted; the
// full carry doc (later) lives in the ArtifactStore behind CarryRef.
const carryExcerptBytes = 512

// ChildFactory builds the fresh child run for one iteration. The driver owns
// the journal-before-send facts and the child's store (opened under sub/);
// the factory owns wiring — provider, pipeline, budget, workspace — so the
// same driver logic drives a scripted test child and a live-provider child.
// A fresh child per iteration is the DESIGN doctrine: same spec → byte-stable
// prefix, no compaction chain, failure isolation. budgetTokens is the
// min-aggregated allowance the factory must cap the child at (0 = unlimited).
type ChildFactory func(childStore *store.EventStore, childSession string, iter, budgetTokens int) *agent.Loop

// Driver is the IterationDriver actor. It has its own journal and pure fold;
// each iteration spawns a fresh child run and verifies its result.
type Driver struct {
	Spec     *DriverSpec
	Store    *store.EventStore
	Clock    clock.Clock
	DriverID string
	// Exec runs command verifiers against the (child-shared) workspace. The
	// driver never edits the workspace; a verifier is an adjudicable effect
	// that only reads/tests it.
	Exec     *tool.Executor
	NewChild ChildFactory
}

// Result summarizes a finished driver run.
type Result struct {
	Reason     string
	Iterations int
	BestIter   int
}

// appendFunc journals one driver-stream fact and folds it into the in-memory
// state — the single write path, mirroring the run loop's appender.
type appendFunc func(typ string, payload any) (event.Envelope, error)

// Run drives goal mode to a terminal DriverCompleted. Loop mode (schedules
// other than immediate) is not yet implemented and is refused explicitly.
func (d *Driver) Run(ctx context.Context) (Result, error) {
	if d.Clock == nil {
		d.Clock = clock.Real{}
	}
	if d.Spec.schedule() != ScheduleImmediate {
		return Result{}, fmt.Errorf("driver: only goal mode (schedule immediate) is implemented; got %q", d.Spec.Schedule)
	}
	if len(d.Spec.Verifiers) == 0 {
		return Result{}, fmt.Errorf("driver: goal mode requires at least one verifier")
	}
	if d.NewChild == nil {
		return Result{}, fmt.Errorf("driver: NewChild factory is required")
	}

	st := &State{Status: StatusRunning, DriverID: d.DriverID}
	appendE := func(typ string, payload any) (event.Envelope, error) {
		env, err := event.New(typ, payload)
		if err != nil {
			return env, err
		}
		env.CorrelationID = d.DriverID
		appended, err := d.Store.Append(env)
		if err != nil {
			return appended, err
		}
		st.apply(payload)
		return appended, nil
	}

	maxIter := d.Spec.maxIterations()
	for n := 1; ; n++ {
		if err := ctx.Err(); err != nil {
			return d.finish(appendE, st, "stopped", n-1)
		}
		if n > maxIter {
			slog.Warn("driver hit max_iterations", "driver", d.DriverID, "max", maxIter)
			return d.finish(appendE, st, "max_iterations", maxIter)
		}
		// Reserve-at-launch against the tree budget (DESIGN: the driver is the
		// budget root). A goal that has spent its whole allowance ends as
		// budget rather than launching a child that can do no useful work.
		allowance, ok := d.reserve(st)
		if !ok {
			slog.Warn("driver budget exhausted", "driver", d.DriverID, "spent", st.SpentTokens)
			return d.finish(appendE, st, "budget", n-1)
		}
		if _, err := appendE(event.TypeIterationScheduled, &event.IterationScheduled{
			DriverID: d.DriverID, Iter: n, Schedule: ScheduleImmediate,
		}); err != nil {
			return Result{}, err
		}
		childSession := fmt.Sprintf("%s-iter-%d", d.DriverID, n)
		if _, err := appendE(event.TypeIterationLaunched, &event.IterationLaunched{
			DriverID: d.DriverID, Iter: n, ChildSession: childSession,
		}); err != nil {
			return Result{}, err
		}

		childRes, childDir, cerr := d.runIteration(ctx, n, childSession, allowance)
		if cerr != nil {
			if ctx.Err() != nil {
				// A cancel of the driver reached the child: end as stopped, not
				// child_failed — the child did not fail on its own merits.
				if _, err := appendE(event.TypeIterationCompleted, &event.IterationCompleted{
					DriverID: d.DriverID, Iter: n, ChildSession: childSession,
					ChildReason: "canceled",
					Verdict:     event.IterationVerdict{Detail: "driver canceled"},
				}); err != nil {
					return Result{}, err
				}
				return d.finish(appendE, st, "stopped", n)
			}
			// The child run failed on its own merits (retries, if any, are
			// already exhausted inside runIteration). Record the failure as the
			// iteration's verdict — with the child's real spend so the budget
			// stays honest — then apply on_child_failure.
			if _, err := appendE(event.TypeIterationCompleted, &event.IterationCompleted{
				DriverID: d.DriverID, Iter: n, ChildSession: childSession,
				ChildReason: "error", Usage: childSpent(childDir),
				Verdict: event.IterationVerdict{
					Detail: "child failed: " + redact.FromEnv().String(cerr.Error())},
			}); err != nil {
				return Result{}, err
			}
			if d.Spec.OnChildFailure.Mode == OnFailSurface {
				// Resilient goal: a failed child is a spent iteration, but the
				// driver keeps trying the next one (bounded by max_iterations
				// and the budget).
				if d.stalled(st) {
					return d.finish(appendE, st, "stalled", n)
				}
				continue
			}
			return d.finish(appendE, st, "child_failed", n)
		}

		verdict := d.verify(ctx, childDir)
		if _, err := appendE(event.TypeIterationCompleted, &event.IterationCompleted{
			DriverID: d.DriverID, Iter: n, ChildSession: childSession,
			ChildReason: childRes.Reason, Verdict: verdict, Usage: childRes.Usage,
			Carry: excerpt(childReport(childDir)),
		}); err != nil {
			return Result{}, err
		}
		if verdict.Pass {
			return d.finish(appendE, st, "satisfied", n)
		}
		if d.stalled(st) {
			return d.finish(appendE, st, "stalled", n)
		}
	}
}

// finish journals the terminal DriverCompleted and returns the Result. The
// in-memory fold already carries BestIter.
func (d *Driver) finish(appendE appendFunc, st *State, reason string, iterations int) (Result, error) {
	if _, err := appendE(event.TypeDriverCompleted, &event.DriverCompleted{
		DriverID: d.DriverID, Reason: reason, Iterations: iterations, BestIter: st.BestIter,
	}); err != nil {
		return Result{}, err
	}
	return Result{Reason: reason, Iterations: iterations, BestIter: st.BestIter}, nil
}

// runIteration runs the iteration's child to completion, applying the
// on_child_failure retry policy: attempt 1 lands under sub/iter-N; each retry
// gets its own sub/iter-N-aM store so a re-run never appends onto a dead log.
// A ctx cancel stops retrying immediately. Returns the last attempt's result,
// its child dir, and its error (nil on the first success).
func (d *Driver) runIteration(ctx context.Context, n int, childSession string, allowance int) (agent.RunResult, string, error) {
	attempts := 1
	if d.Spec.OnChildFailure.Mode == OnFailRetry && d.Spec.OnChildFailure.Max > 0 {
		attempts += d.Spec.OnChildFailure.Max
	}
	var (
		res      agent.RunResult
		childDir string
		rerr     error
	)
	for a := 1; a <= attempts; a++ {
		childDir = filepath.Join(d.Store.Dir(), "sub", iterDir(n, a))
		childStore, err := store.OpenEventStore(childDir)
		if err != nil {
			return agent.RunResult{}, childDir, fmt.Errorf("driver: open child store: %w", err)
		}
		session := childSession
		if a > 1 {
			session = fmt.Sprintf("%s-a%d", childSession, a)
		}
		child := d.NewChild(childStore, session, n, allowance)
		res, rerr = child.Run(ctx, d.Spec.Task)
		_ = childStore.Close()
		if rerr == nil {
			return res, childDir, nil
		}
		if ctx.Err() != nil {
			return res, childDir, rerr // cancel is not a retryable failure
		}
		if a < attempts {
			slog.Warn("driver: child attempt failed, retrying",
				"driver", d.DriverID, "iter", n, "attempt", a, "err", rerr)
		}
	}
	return res, childDir, rerr
}

// iterDir names an iteration's child journal: sub/iter-N for the first
// attempt, sub/iter-N-aM for retries.
func iterDir(n, attempt int) string {
	if attempt <= 1 {
		return fmt.Sprintf("iter-%d", n)
	}
	return fmt.Sprintf("iter-%d-a%d", n, attempt)
}

// reserve computes the next child's min-aggregated allowance and reports
// whether there is any budget left to launch. Zero driver budget means
// unlimited (allowance 0 passes through to the child unclamped). With a
// budget, the allowance is the driver's remaining, further clamped by the
// child spec's own cap; an exhausted budget (remaining ≤ 0) refuses launch.
func (d *Driver) reserve(st *State) (allowance int, ok bool) {
	treeCap := d.Spec.Budget.MaxTotalTokens
	if treeCap <= 0 {
		return d.childCap(), true // unlimited tree: only the child spec caps
	}
	remaining := treeCap - st.SpentTokens
	if remaining <= 0 {
		return 0, false
	}
	if cc := d.childCap(); cc > 0 && cc < remaining {
		return cc, true
	}
	return remaining, true
}

// childCap is the child spec's own token cap (0 = unlimited).
func (d *Driver) childCap() int {
	if d.Spec.Agent == nil {
		return 0
	}
	return d.Spec.Agent.Budget.MaxTotalTokens
}

// stalled is pure fold: DESIGN's patience rule — this many consecutive most
// recent completed iterations with no score improvement over the best-so-far
// ends the run. Zero patience disables it.
func (d *Driver) stalled(st *State) bool {
	if d.Spec.Patience <= 0 {
		return false
	}
	best := 0.0
	if st.BestIter > 0 {
		best = st.Iterations[st.BestIter-1].Verdict.Score
	}
	// Count completed iterations after the best one; if that streak reaches
	// patience, no recent iteration improved on the best.
	sinceBest := 0
	for _, it := range st.Iterations {
		if !it.Completed {
			continue
		}
		if it.N > st.BestIter && it.Verdict.Score <= best {
			sinceBest++
		}
	}
	return sinceBest >= d.Spec.Patience
}

// verify runs every configured verifier; ALL must pass for the iteration to
// satisfy the goal. The aggregate score is the minimum across verifiers (the
// weakest gate), so stall detection tracks the true bottleneck — seeded from
// the first verifier so a single metric score above 1 is not clamped.
func (d *Driver) verify(ctx context.Context, childDir string) event.IterationVerdict {
	agg := event.IterationVerdict{Pass: true}
	for i, v := range d.Spec.Verifiers {
		vv := d.verifyOne(ctx, v)
		if i == 0 || vv.Score < agg.Score {
			agg.Score = vv.Score
		}
		agg.Verifier = vv.Verifier
		agg.Detail = vv.Detail
		if !vv.Pass {
			agg.Pass = false
			break // first failing gate settles the verdict
		}
	}
	return agg
}

func (d *Driver) verifyOne(ctx context.Context, v VerifierSpec) event.IterationVerdict {
	switch v.Kind {
	case VerifierCommand:
		return d.verifyCommand(ctx, v)
	default:
		return event.IterationVerdict{Verifier: v.Kind, Detail: "unknown verifier kind " + v.Kind}
	}
}

// verifyCommand runs a bash-class verifier against the workspace. With a
// metric regex, capture group 1 is parsed as the score (≥ threshold passes);
// otherwise exit code 0 passes. The command is driver-configured (trusted
// config, not a model-chosen effect), so it runs via the executor directly;
// full pipeline adjudication of verifiers is a later refinement.
func (d *Driver) verifyCommand(ctx context.Context, v VerifierSpec) event.IterationVerdict {
	if d.Exec == nil {
		return event.IterationVerdict{Verifier: VerifierCommand, Detail: "no executor for command verifier"}
	}
	args, _ := json.Marshal(map[string]string{"command": v.Command})
	res := d.Exec.Execute(ctx, "bash", args)
	var out struct {
		ExitCode int    `json:"exit_code"`
		Stdout   string `json:"stdout"`
	}
	_ = json.Unmarshal(res.Payload, &out)

	if v.MetricRegex != "" {
		re, err := regexp.Compile(v.MetricRegex)
		if err != nil {
			return event.IterationVerdict{Verifier: VerifierCommand, Detail: "bad metric_regex: " + err.Error()}
		}
		m := re.FindStringSubmatch(out.Stdout)
		if len(m) < 2 {
			return event.IterationVerdict{Verifier: VerifierCommand, Detail: "metric not found in output"}
		}
		score, perr := strconv.ParseFloat(m[1], 64)
		if perr != nil {
			return event.IterationVerdict{Verifier: VerifierCommand, Detail: "metric not a number: " + m[1]}
		}
		return event.IterationVerdict{
			Pass: score >= v.Threshold, Verifier: VerifierCommand, Score: score,
			Detail: fmt.Sprintf("metric=%g threshold=%g", score, v.Threshold),
		}
	}

	pass := out.ExitCode == 0
	score := 0.0
	if pass {
		score = 1
	}
	return event.IterationVerdict{
		Pass: pass, Verifier: VerifierCommand, Score: score,
		Detail: fmt.Sprintf("exit=%d", out.ExitCode),
	}
}

// childSpent folds the child journal for its settled usage — the truth even
// when the child aborted (RunResult carries zero on error paths), so a failed
// child's spend still counts against the tree budget.
func childSpent(childDir string) provider.Usage {
	events, err := store.ReadEvents(childDir)
	if err != nil {
		return provider.Usage{}
	}
	s, err := state.Fold(events)
	if err != nil {
		return provider.Usage{}
	}
	return s.Run.Usage
}

// childReport extracts the child's final assistant text from its journal —
// the carry excerpt a later iteration (and inspect) sees.
func childReport(childDir string) string {
	events, err := store.ReadEvents(childDir)
	if err != nil {
		return ""
	}
	s, err := state.Fold(events)
	if err != nil {
		return ""
	}
	var last string
	for _, m := range s.Conversation.Messages {
		if m.Role != provider.RoleAssistant {
			continue
		}
		for _, p := range m.Parts {
			if p.Kind == provider.PartText && p.Text != "" {
				last = p.Text
			}
		}
	}
	return last
}

// excerpt truncates a carry string to the inline cap, redacting credentials
// (the same doctrine as every other persisted payload).
func excerpt(s string) string {
	s = redact.FromEnv().String(s)
	if len(s) > carryExcerptBytes {
		return s[:carryExcerptBytes] + "…"
	}
	return s
}
