package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/cron"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/snapshot"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
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

// WorktreeChildFactory builds a child whose workspace (executor AND
// permission-gate path resolution) is an isolated worktree — the best-of-N
// launch path (S7). The caller owns materializing worktreeRoot first.
type WorktreeChildFactory func(childStore *store.EventStore, childSession string,
	iter, budgetTokens int, worktreeRoot string) *agent.Loop

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
	// NewChildAt and Snapshots serve best-of-N (schedule=parallel): each
	// attempt runs in its own worktree materialized from one base snapshot.
	// Both are required for parallel and unused otherwise.
	NewChildAt WorktreeChildFactory
	Snapshots  snapshot.Store
	// Judge is the LLM behind llm_judge verifiers (a single scoring call per
	// iteration, not an agent loop). nil → an llm_judge verifier fails
	// model-visibly rather than silently passing.
	Judge provider.Provider
	// Approvals resolves human verifiers via the same ask path the agent
	// loop uses. nil → EnvApprovals (fail-closed when unset).
	Approvals agent.ApprovalResolver
	// Artifacts is the deliverable CAS (S5.5) the carry docs land in: each
	// completed iteration's full report is published to the "carry" stream
	// and IterationCompleted keeps only the ref + a short excerpt (DESIGN).
	// nil → carry stays inline-only.
	Artifacts *store.ArtifactStore
	// Out receives the driver's LIFECYCLE as output events (S6 模块⑤):
	// iteration completions and the terminal — what a hosting surface tees
	// to watchers and the notifier. nil = silent (journal stays the truth).
	Out protocol.Sink
	// Pipeline adjudicates verifier effects (S7 还债①: verifier 过四关卡,
	// 收回 S6 的直连例外): a command verifier is a tool_call effect, a
	// judge an llm_call. The CLI builds it with the merged user/project
	// rules ahead of a trailing driver-trust allow — explicit user denies
	// bind, config-declared verifiers otherwise run. nil = allow-all.
	Pipeline *pipeline.Pipeline

	// Loop-mode cadence runtime state (never fold state: cron ticks are
	// absolute wall times, recomputable from the clock; the self_paced pace
	// re-derives from the last child journal).
	cronSched *cron.Schedule
	lastTick  time.Time
	nextPace  time.Duration
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

// emit sends a lifecycle output event (nil-safe).
func (d *Driver) emit(e protocol.Event) {
	if d.Out != nil {
		d.Out.Emit(e)
	}
}

// prepare validates the spec and builds the single write path over st (the
// folded in-memory state). Shared by Run (fresh state) and Resume (folded).
func (d *Driver) prepare(st *State) (appendFunc, error) {
	if d.Clock == nil {
		d.Clock = clock.Real{}
	}
	switch d.Spec.schedule() {
	case ScheduleImmediate:
		// Goal mode: a verifier is what decides "done".
		if len(d.Spec.Verifiers) == 0 {
			return nil, fmt.Errorf("driver: goal mode requires at least one verifier")
		}
	case ScheduleInterval:
		// Loop mode: verifiers are optional; the cadence must parse.
		if _, err := d.Spec.interval(); err != nil {
			return nil, fmt.Errorf("driver: bad interval %q: %w", d.Spec.Interval, err)
		}
	case ScheduleCron:
		sched, err := cron.Parse(d.Spec.Cron)
		if err != nil {
			return nil, fmt.Errorf("driver: %w", err)
		}
		d.cronSched = sched
	case ScheduleSelfPaced:
		if _, _, err := d.Spec.paceBounds(); err != nil {
			return nil, fmt.Errorf("driver: %w", err)
		}
		switch d.Spec.OnNoIntent {
		case "", NoIntentFinish:
		case NoIntentContinue:
			if d.Spec.PaceMin == "" {
				return nil, fmt.Errorf("driver: on_no_intent=continue requires pace_min — a forgetful child must not spin the series")
			}
		default:
			return nil, fmt.Errorf("driver: on_no_intent %q unknown (known: finish, continue)", d.Spec.OnNoIntent)
		}
		// The child paces the series: put the pace tools on its face.
		if d.Spec.Agent != nil {
			for _, name := range []string{"schedule_next", "finish_series"} {
				if !slices.Contains(d.Spec.Agent.Tools, name) {
					d.Spec.Agent.Tools = append(d.Spec.Agent.Tools, name)
				}
			}
		}
	case ScheduleParallel:
		// Best-of-N: the verifiers ARE the selection; isolation needs the
		// snapshot store and a worktree-aware child factory.
		if d.Spec.N < 2 {
			return nil, fmt.Errorf("driver: schedule parallel requires n >= 2 (got %d)", d.Spec.N)
		}
		if len(d.Spec.Verifiers) == 0 {
			return nil, fmt.Errorf("driver: schedule parallel requires verifiers — they select the winner")
		}
		if d.Snapshots == nil || d.NewChildAt == nil {
			return nil, fmt.Errorf("driver: schedule parallel requires a snapshot store and a worktree child factory")
		}
	default:
		return nil, fmt.Errorf("driver: schedule %q not implemented", d.Spec.Schedule)
	}
	switch d.Spec.Overlap {
	case "", OverlapSkip, OverlapCoalesce:
	default:
		return nil, fmt.Errorf("driver: overlap %q unknown (known: skip, coalesce)", d.Spec.Overlap)
	}
	if d.NewChild == nil {
		return nil, fmt.Errorf("driver: NewChild factory is required")
	}
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
		// Lifecycle tee (S6 模块⑤): the single write path is the one place
		// every journal site passes through, so watchers and the notifier
		// see EVERY iteration terminal and the driver's ending — including
		// the failure and cancel paths.
		switch p := payload.(type) {
		case *event.IterationCompleted:
			d.emit(protocol.Event{Kind: protocol.KindIteration, N: p.Iter,
				Reason: p.ChildReason,
				Text: fmt.Sprintf("iteration %d %s (pass=%v score=%g)",
					p.Iter, p.ChildReason, p.Verdict.Pass, p.Verdict.Score)})
		case *event.DriverCompleted:
			d.emit(protocol.Event{Kind: protocol.KindRunEnd, N: p.Iterations, Reason: p.Reason})
		}
		return appended, nil
	}
	return appendE, nil
}

// Run drives goal mode from scratch to a terminal DriverCompleted. Loop mode
// (schedules other than immediate) is not yet implemented and is refused.
func (d *Driver) Run(ctx context.Context) (Result, error) {
	st := &State{Status: StatusRunning, DriverID: d.DriverID}
	appendE, err := d.prepare(st)
	if err != nil {
		return Result{}, err
	}
	// The stream header (S7 还债): spec + fold version guard every resume,
	// and the spec provenance makes a future spec-less resume possible
	// (mirrors SessionStarted 2.17). Redacted like every persisted payload.
	specJSON, _ := json.Marshal(d.Spec)
	wsRoot := ""
	if d.Exec != nil && d.Exec.WS != nil {
		wsRoot = d.Exec.WS.Root()
	}
	if _, err := appendE(event.TypeDriverStarted, &event.DriverStarted{
		DriverID: d.DriverID, SpecName: d.Spec.Name,
		Spec: redact.FromEnv().JSON(specJSON), WorkspaceRoot: wsRoot,
		FoldVersion: FoldVersion,
	}); err != nil {
		return Result{}, err
	}
	if d.Spec.schedule() == ScheduleParallel {
		return d.driveParallel(ctx, st, appendE, 1)
	}
	return d.drive(ctx, st, appendE, 1)
}

// Resume rebuilds the driver fold from its journal and continues — an ended
// driver returns its recorded result; otherwise the drive loop picks up at the
// first not-yet-completed iteration, and runIteration recovers any in-flight
// child (resume it, or settle it from its own terminal fold).
func (d *Driver) Resume(ctx context.Context) (Result, error) {
	events, err := store.ReadEvents(d.Store.Dir())
	if err != nil {
		return Result{}, err
	}
	folded, err := Fold(events)
	if err != nil {
		return Result{}, err
	}
	// Version discipline: a header carrying a different fold version refuses
	// the resume (never silently migrated); a headerless S6 stream is v1.
	if len(events) > 0 && events[0].Type == event.TypeDriverStarted {
		if decoded, derr := event.DecodePayload(events[0]); derr == nil {
			if started := decoded.(*event.DriverStarted); started.FoldVersion != FoldVersion {
				return Result{}, fmt.Errorf("driver resume: stream fold version %d does not match binary version %d",
					started.FoldVersion, FoldVersion)
			}
		}
	}
	st := &folded
	st.DriverID = d.DriverID
	appendE, err := d.prepare(st)
	if err != nil {
		return Result{}, err
	}
	if st.Status == StatusEnded {
		return Result{Reason: st.Reason, Iterations: len(st.Iterations), BestIter: st.BestIter}, nil
	}
	// A crash can land between a terminal-deciding IterationCompleted and the
	// DriverCompleted that records it. Re-derive that decision from the fold
	// so resume does not launch a redundant iteration — GOAL MODE ONLY: in
	// loop mode a passing verdict is a quality gate, never a terminal, and
	// re-deriving it here would kill a healthy series on every restart
	// (S6 review P0). max_iterations and budget re-check at the top of
	// drive(), so only satisfied/stalled need re-deriving.
	if d.Spec.schedule() == ScheduleImmediate {
		if last, ok := st.lastCompleted(); ok {
			if last.Verdict.Pass {
				return d.finish(appendE, st, "satisfied", last.N)
			}
			if d.stalled(st) {
				return d.finish(appendE, st, "stalled", last.N)
			}
		}
	}
	// Completed AND skipped iterations are consumed slots; resume at the
	// first untouched number — re-running a skipped iteration would violate
	// the overlap policy across the crash (S6 review).
	startN := 1
	for i := range st.Iterations {
		if st.Iterations[i].Completed || st.Iterations[i].Skipped {
			startN = st.Iterations[i].N + 1
		}
	}
	// self_paced: the pace the last child declared must survive the restart
	// (the field comment promises re-derivation; S6 review). A declared
	// finish was either approved (driver ended) or denied (floor pace).
	if d.Spec.schedule() == ScheduleSelfPaced && startN > 1 {
		if last, ok := st.lastCompleted(); ok {
			floor, ceil, _ := d.Spec.paceBounds()
			intent := childIntent(filepath.Join(d.Store.Dir(), "sub", iterDir(last.N, 1)))
			switch {
			case intent.has && !intent.finish:
				d.nextPace = clampPace(intent.after, floor, ceil)
			default:
				d.nextPace = floor
			}
		}
	}
	if d.Spec.schedule() == ScheduleParallel {
		return d.driveParallel(ctx, st, appendE, startN)
	}
	return d.drive(ctx, st, appendE, startN)
}

// drive is the goal loop shared by Run and Resume. On resume it does not
// re-journal an iteration's Scheduled/Launched facts that the fold already
// holds — the write path stays append-only and idempotent across a crash.
func (d *Driver) drive(ctx context.Context, st *State, appendE appendFunc, startN int) (Result, error) {
	loopMode := d.Spec.schedule() != ScheduleImmediate
	maxIter := d.Spec.maxIterations()
	for n := startN; ; n++ {
		if err := ctx.Err(); err != nil {
			return d.finish(appendE, st, "stopped", n-1)
		}
		if n > maxIter {
			slog.Warn("driver hit max_iterations", "driver", d.DriverID, "max", maxIter)
			return d.finish(appendE, st, "max_iterations", maxIter)
		}
		// Loop-mode cadence: interval fires iteration 1 now and fixed-delays
		// the rest; cron waits for each absolute tick, applying the overlap
		// policy to ticks the previous iteration ran past. A cancel during
		// the wait ends the series.
		if loopMode {
			// "first" means the SERIES' first iteration, not the first after
			// a resume: a restart must respect the pace/tick, not fire early.
			run, terr := d.awaitTick(ctx, appendE, n, n == 1)
			if terr != nil {
				if ctx.Err() != nil {
					return d.finish(appendE, st, "stopped", n-1)
				}
				return Result{}, terr
			}
			if !run {
				continue // overlap=skip consumed n as a skipped iteration
			}
		}
		// Reserve-at-launch against the tree budget (DESIGN: the driver is the
		// budget root). A goal that has spent its whole allowance ends as
		// budget rather than launching a child that can do no useful work.
		allowance, ok := d.reserve(st)
		if !ok {
			slog.Warn("driver budget exhausted", "driver", d.DriverID, "spent", st.SpentTokens)
			return d.finish(appendE, st, "budget", n-1)
		}
		session := fmt.Sprintf("%s-iter-%d", d.DriverID, n)
		if _, inFold := st.at(n); !inFold {
			if _, err := appendE(event.TypeIterationScheduled, &event.IterationScheduled{
				DriverID: d.DriverID, Iter: n, Schedule: ScheduleImmediate,
			}); err != nil {
				return Result{}, err
			}
		}
		if existing, inFold := st.at(n); !inFold || !existing.Launched {
			if _, err := appendE(event.TypeIterationLaunched, &event.IterationLaunched{
				DriverID: d.DriverID, Iter: n, ChildSession: session,
			}); err != nil {
				return Result{}, err
			}
		} else {
			session = existing.ChildSession // in-flight: reuse the recorded session
		}

		childRes, childDir, spent, cerr := d.runIteration(ctx, n, session, allowance, "")
		if cerr != nil {
			if ctx.Err() != nil {
				// A cancel of the driver reached the child: end as stopped, not
				// child_failed — the child did not fail on its own merits.
				if _, err := appendE(event.TypeIterationCompleted, &event.IterationCompleted{
					DriverID: d.DriverID, Iter: n, ChildSession: session,
					ChildReason: "canceled", Usage: spent,
					Verdict: event.IterationVerdict{Detail: "driver canceled"},
				}); err != nil {
					return Result{}, err
				}
				return d.finish(appendE, st, "stopped", n)
			}
			// The child run failed on its own merits (retries, if any, are
			// already exhausted inside runIteration). Record the failure as the
			// iteration's verdict — with EVERY attempt's real spend so the
			// budget stays honest — then apply on_child_failure.
			if _, err := appendE(event.TypeIterationCompleted, &event.IterationCompleted{
				DriverID: d.DriverID, Iter: n, ChildSession: session,
				ChildReason: "error", Usage: spent,
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

		// Loop mode verifies only when verifiers are configured (they gate
		// quality, they do not decide "done"); goal mode always verifies.
		// A judge's LLM spend joins the iteration's usage — verifier tokens
		// are real tree spend (S6 review).
		var verdict event.IterationVerdict
		if len(d.Spec.Verifiers) > 0 {
			var judgeUsage provider.Usage
			verdict, judgeUsage = d.verify(ctx, appendE, n, childDir, d.Exec)
			spent = addUsage(spent, judgeUsage)
		}
		carryText := childReport(childDir)
		if _, err := appendE(event.TypeIterationCompleted, &event.IterationCompleted{
			DriverID: d.DriverID, Iter: n, ChildSession: session,
			// spent sums EVERY attempt's folded usage (a retried iteration's
			// failed attempts burned real tokens — S6 review P1); on the
			// no-retry happy path it equals childRes.Usage.
			ChildReason: childRes.Reason, Verdict: verdict, Usage: spent,
			CarryRef: d.publishCarry(carryText), Carry: excerpt(carryText),
		}); err != nil {
			return Result{}, err
		}
		// Goal mode ends when the goal is met or progress stalls; loop mode
		// runs on cadence until max_iterations / budget / cancel — or, in
		// self_paced, an approved finish_series / no-intent finish.
		if !loopMode {
			if verdict.Pass {
				return d.finish(appendE, st, "satisfied", n)
			}
			if d.stalled(st) {
				return d.finish(appendE, st, "stalled", n)
			}
		} else if d.Spec.schedule() == ScheduleSelfPaced {
			done, res, perr := d.applyPaceIntent(ctx, appendE, st, n, childDir)
			if perr != nil {
				return Result{}, perr
			}
			if done {
				return res, nil
			}
		}
	}
}

// driveParallel is best-of-N (S7, schedule=parallel): N attempts, each a
// fresh child in an isolated worktree materialized from ONE base snapshot,
// judged by the verifiers IN ITS OWN TREE; the best verdict wins (pass
// beats score, ties keep the earliest attempt). Attempts run sequentially
// (v0 — deterministic and resume-friendly; the isolation is the semantics,
// wall-clock concurrency a deferred optimization) and a failed attempt is
// a spent slot, never the round's end — other attempts are the point.
func (d *Driver) driveParallel(ctx context.Context, st *State, appendE appendFunc, startN int) (Result, error) {
	total := d.Spec.N
	// One base for the whole round, pinned in attempt 1's Scheduled fact so
	// a resume re-materializes the SAME tree, not the drifted workspace.
	baseRef := ""
	if it, ok := st.at(1); ok {
		baseRef = it.BaseRef
	}
	if baseRef == "" {
		ref, err := d.Snapshots.Snapshot(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("driver: base snapshot for parallel round: %w", err)
		}
		baseRef = ref
	}
	for n := startN; n <= total; n++ {
		if err := ctx.Err(); err != nil {
			return d.finish(appendE, st, "stopped", n-1)
		}
		allowance, ok := d.reserve(st)
		if !ok {
			slog.Warn("driver budget exhausted mid-round", "driver", d.DriverID, "spent", st.SpentTokens)
			return d.finish(appendE, st, "budget", n-1)
		}
		session := fmt.Sprintf("%s-att-%d", d.DriverID, n)
		if _, inFold := st.at(n); !inFold {
			if _, err := appendE(event.TypeIterationScheduled, &event.IterationScheduled{
				DriverID: d.DriverID, Iter: n, Schedule: ScheduleParallel, BaseRef: baseRef,
			}); err != nil {
				return Result{}, err
			}
		}
		worktree := filepath.Join(d.Store.Dir(), "wt", fmt.Sprintf("att-%d", n))
		_, serr := os.Stat(worktree)
		wtMissing := os.IsNotExist(serr)
		if wtMissing {
			// Materialize is atomic (temp + rename): an existing dir IS a
			// complete tree, so existence doubles as the resume marker.
			if err := d.Snapshots.Materialize(ctx, baseRef, worktree); err != nil {
				return Result{}, fmt.Errorf("driver: materialize attempt %d worktree: %w", n, err)
			}
			// A child journal without its worktree = the tree was lost across
			// a restart. Resuming the child (or judging a settled one) on the
			// fresh BASE would silently roll back its own edits — fail the
			// attempt instead (S7 出口 review P1).
			if evs, rerr := store.ReadEvents(filepath.Join(d.Store.Dir(), "sub", iterDir(n, 1))); rerr == nil && len(evs) > 0 {
				if _, err := appendE(event.TypeIterationCompleted, &event.IterationCompleted{
					DriverID: d.DriverID, Iter: n, ChildSession: session,
					ChildReason: "error",
					Verdict:     event.IterationVerdict{Detail: "attempt worktree lost across restart; refusing to judge a rolled-back tree"},
				}); err != nil {
					return Result{}, err
				}
				continue
			}
		}
		if existing, inFold := st.at(n); !inFold || !existing.Launched {
			if _, err := appendE(event.TypeIterationLaunched, &event.IterationLaunched{
				DriverID: d.DriverID, Iter: n, ChildSession: session,
			}); err != nil {
				return Result{}, err
			}
		} else {
			session = existing.ChildSession
		}

		childRes, childDir, spent, cerr := d.runIteration(ctx, n, session, allowance, worktree)
		if cerr != nil {
			if _, err := appendE(event.TypeIterationCompleted, &event.IterationCompleted{
				DriverID: d.DriverID, Iter: n, ChildSession: session,
				ChildReason: failReason(ctx), Usage: spent,
				Verdict: event.IterationVerdict{
					Detail: "attempt failed: " + redact.FromEnv().String(cerr.Error())},
			}); err != nil {
				return Result{}, err
			}
			if ctx.Err() != nil {
				return d.finish(appendE, st, "stopped", n)
			}
			continue // a failed attempt is one spent slot; the round goes on
		}

		wtExec, werr := worktreeExecutor(worktree, session)
		if werr != nil {
			return Result{}, werr
		}
		verdict, judgeUsage := d.verify(ctx, appendE, n, childDir, wtExec)
		spent = addUsage(spent, judgeUsage)
		carryText := childReport(childDir)
		if _, err := appendE(event.TypeIterationCompleted, &event.IterationCompleted{
			DriverID: d.DriverID, Iter: n, ChildSession: session,
			ChildReason: childRes.Reason, Verdict: verdict, Usage: spent,
			CarryRef: d.publishCarry(carryText), Carry: excerpt(carryText),
		}); err != nil {
			return Result{}, err
		}
	}

	// Selection: pass beats any score; among equals the higher score wins;
	// ties keep the earliest. The choice overrides the fold's max-score
	// BestIter and rides DriverCompleted (the fold applies it on resume).
	best, bestPass, bestScore := 0, false, 0.0
	for n := 1; n <= total; n++ {
		it, ok := st.at(n)
		if !ok || !it.Completed {
			continue
		}
		v := it.Verdict
		better := best == 0 ||
			(v.Pass && !bestPass) ||
			(v.Pass == bestPass && v.Score > bestScore)
		if better {
			best, bestPass, bestScore = n, v.Pass, v.Score
		}
	}
	st.BestIter = best
	reason := "stalled"
	if bestPass {
		reason = "satisfied"
	}
	if best > 0 {
		d.emit(protocol.Event{Kind: protocol.KindNote,
			Text: fmt.Sprintf("best-of-%d winner: attempt %d (worktree %s)",
				total, best, filepath.Join(d.Store.Dir(), "wt", fmt.Sprintf("att-%d", best)))})
	}
	return d.finish(appendE, st, reason, total)
}

// failReason distinguishes a driver cancel from a child's own failure.
func failReason(ctx context.Context) string {
	if ctx.Err() != nil {
		return "canceled"
	}
	return "error"
}

// worktreeExecutor builds the per-attempt verifier executor: a command
// verifier must test the attempt's OWN tree, not the shared workspace.
func worktreeExecutor(root, session string) (*tool.Executor, error) {
	ws, err := workspace.New(root)
	if err != nil {
		return nil, fmt.Errorf("driver: worktree workspace: %w", err)
	}
	return &tool.Executor{WS: ws, Session: session}, nil
}

// applyPaceIntent reads the finished child's schedule_next / finish_series
// declaration (from its journal) and either ends the series or stashes the
// next pace for awaitTick. finish_series is human-gated (DESIGN: "自称完成"
// 由 human verifier 把关,不另设 confirm 机制): approved → satisfied; denied
// → the series continues at the pace floor. No intent → on_no_intent
// (default finish: a child that stops asking is done).
func (d *Driver) applyPaceIntent(ctx context.Context, appendE appendFunc, st *State, n int, childDir string) (bool, Result, error) {
	intent := childIntent(childDir)
	floor, ceil, _ := d.Spec.paceBounds() // validated in prepare
	switch {
	case intent.finish:
		if d.confirmFinish(ctx, n, childDir) {
			res, err := d.finish(appendE, st, "satisfied", n)
			return true, res, err
		}
		d.nextPace = floor
		return false, Result{}, nil
	case intent.has:
		d.nextPace = clampPace(intent.after, floor, ceil)
		return false, Result{}, nil
	default:
		if d.Spec.OnNoIntent == NoIntentContinue {
			d.nextPace = floor
			return false, Result{}, nil
		}
		res, err := d.finish(appendE, st, "satisfied", n)
		return true, res, err
	}
}

func clampPace(after, floor, ceil time.Duration) time.Duration {
	if after < floor {
		return floor
	}
	if ceil > 0 && after > ceil {
		return ceil
	}
	return after
}

// confirmFinish runs the human gate on a finish_series claim through the
// same ask path as every approval (fail-closed under EnvApprovals unset).
func (d *Driver) confirmFinish(ctx context.Context, n int, childDir string) bool {
	resolver := d.Approvals
	if resolver == nil {
		resolver = &agent.EnvApprovals{}
	}
	args, _ := json.Marshal(map[string]string{
		"claim":  "the series is complete",
		"report": excerpt(childReport(childDir)),
	})
	dec, err := resolver.Resolve(ctx, agent.ApprovalRequest{
		ApprovalID: fmt.Sprintf("finish-%s-i%d", d.DriverID, n),
		Agent:      d.Spec.Name + " (series completion)",
		ToolName:   "finish_series",
		Args:       args,
	})
	return err == nil && dec.Approve
}

// paceIntent is a self_paced child's declared intent.
type paceIntent struct {
	finish bool          // finish_series succeeded
	after  time.Duration // schedule_next's requested delay
	has    bool          // any intent at all (the LAST declaration wins)
}

// childIntent folds the child journal and extracts the last successful
// schedule_next / finish_series call — unanswered or error-result calls
// carry no intent.
func childIntent(childDir string) paceIntent {
	events, err := store.ReadEvents(childDir)
	if err != nil {
		return paceIntent{}
	}
	s, err := state.Fold(events)
	if err != nil {
		return paceIntent{}
	}
	var intent paceIntent
	for _, m := range s.Conversation.Messages {
		if m.Role != provider.RoleAssistant {
			continue
		}
		for _, p := range m.Parts {
			if p.Kind != provider.PartToolCall ||
				(p.ToolName != "schedule_next" && p.ToolName != "finish_series") {
				continue
			}
			tr, ok := s.Conversation.ToolResults[p.CallID]
			if !ok || tr.IsError {
				continue
			}
			if p.ToolName == "finish_series" {
				intent = paceIntent{finish: true, has: true}
				continue
			}
			var args struct {
				After string `json:"after"`
			}
			_ = json.Unmarshal(p.Args, &args)
			if dur, derr := time.ParseDuration(args.After); derr == nil {
				intent = paceIntent{after: dur, has: true}
			}
		}
	}
	return intent
}

// awaitTick blocks until iteration n's tick is due (loop mode). It returns
// false when overlap=skip consumed n as a skipped iteration (the missed tick
// is journaled, never silent — DESIGN §运行形态).
func (d *Driver) awaitTick(ctx context.Context, appendE appendFunc, n int, first bool) (bool, error) {
	switch d.Spec.schedule() {
	case ScheduleInterval:
		// Fixed delay after the previous completion: the first iteration (of
		// a run or a resume) fires immediately, and — being sequential with
		// no absolute timeline — an interval series cannot miss a tick.
		if first {
			return true, nil
		}
		every, _ := d.Spec.interval() // validated in prepare
		if err := d.Clock.WaitUntil(ctx, d.Clock.Now().Add(every)); err != nil {
			return false, err
		}
		return true, nil

	case ScheduleCron:
		// Absolute timeline: every iteration (including the first) waits for
		// a real tick — a nightly job started at 10:00 first runs at 03:00.
		now := d.Clock.Now()
		if d.lastTick.IsZero() {
			d.lastTick = now
		}
		next, ok := d.cronSched.Next(d.lastTick)
		if !ok {
			return false, fmt.Errorf("driver: cron %q never fires", d.Spec.Cron)
		}
		if !next.After(now) {
			// The tick passed while the previous iteration ran.
			d.lastTick = next
			if d.Spec.Overlap == OverlapCoalesce {
				// Fold EVERY due tick into one immediate iteration.
				for {
					nn, ok := d.cronSched.Next(d.lastTick)
					if !ok || nn.After(now) {
						break
					}
					d.lastTick = nn
				}
				return true, nil
			}
			// skip (default): one skipped iteration per missed tick.
			if _, err := appendE(event.TypeIterationSkipped, &event.IterationSkipped{
				DriverID: d.DriverID, Iter: n,
				Reason: "overlap: tick " + next.UTC().Format(time.RFC3339) + " passed while an iteration ran",
			}); err != nil {
				return false, err
			}
			return false, nil
		}
		if err := d.Clock.WaitUntil(ctx, next); err != nil {
			return false, err
		}
		d.lastTick = next
		return true, nil

	case ScheduleSelfPaced:
		// The pace was stashed by applyPaceIntent at the previous iteration's
		// end; the first iteration fires immediately.
		if first {
			return true, nil
		}
		if d.nextPace > 0 {
			if err := d.Clock.WaitUntil(ctx, d.Clock.Now().Add(d.nextPace)); err != nil {
				return false, err
			}
		}
		return true, nil

	default:
		return false, fmt.Errorf("driver: schedule %q has no cadence", d.Spec.schedule())
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
// A ctx cancel stops retrying immediately. Returns the last attempt's result
// and dir, the SUMMED spend across every attempt (failed retries burned real
// tokens — the budget must see them; S6 review), and the error (nil on the
// first success).
func (d *Driver) runIteration(ctx context.Context, n int, childSession string, allowance int, worktree string) (agent.RunResult, string, provider.Usage, error) {
	newChild := func(cs *store.EventStore, sess string) *agent.Loop {
		if worktree != "" {
			return d.NewChildAt(cs, sess, n, allowance, worktree)
		}
		return d.NewChild(cs, sess, n, allowance)
	}
	attempts := 1
	if d.Spec.OnChildFailure.Mode == OnFailRetry && d.Spec.OnChildFailure.Max > 0 {
		attempts += d.Spec.OnChildFailure.Max
	}
	var (
		res      agent.RunResult
		childDir string
		spent    provider.Usage
		rerr     error
	)
	for a := 1; a <= attempts; a++ {
		childDir = filepath.Join(d.Store.Dir(), "sub", iterDir(n, a))
		childStore, err := store.OpenEventStore(childDir)
		if err != nil {
			return agent.RunResult{}, childDir, spent, fmt.Errorf("driver: open child store: %w", err)
		}
		session := childSession
		if a > 1 {
			session = fmt.Sprintf("%s-a%d", childSession, a)
		}
		// A pre-existing journal means the driver crashed with this child
		// in-flight (only attempt 1 can carry prior events — retries always get
		// a fresh dir). If that child already reached a terminal state, settle
		// from its fold — an error/canceled ending settles as a FAILURE so the
		// on_child_failure policy applies identically across the crash (S6
		// review); otherwise resume it (its own in-doubt discipline guards
		// correctness) rather than duplicating a fresh run.
		if childStore.LastSeq() > 0 {
			child := newChild(childStore, session)
			if done, dres := settledChild(childDir); done {
				_ = childStore.Close()
				spent = addUsage(spent, dres.Usage)
				if dres.Reason == "error" || dres.Reason == "canceled" {
					res, rerr = dres, fmt.Errorf("child ended %s (settled from journal)", dres.Reason)
					if ctx.Err() != nil {
						return res, childDir, spent, rerr
					}
					continue
				}
				return dres, childDir, spent, nil
			}
			res, rerr = child.Resume(ctx)
		} else {
			child := newChild(childStore, session)
			res, rerr = child.Run(ctx, d.buildTask())
		}
		_ = childStore.Close()
		spent = addUsage(spent, childSpent(childDir))
		if rerr == nil {
			return res, childDir, spent, nil
		}
		if ctx.Err() != nil {
			return res, childDir, spent, rerr // cancel is not a retryable failure
		}
		if a < attempts {
			slog.Warn("driver: child attempt failed, retrying",
				"driver", d.DriverID, "iter", n, "attempt", a, "err", rerr)
		}
	}
	return res, childDir, spent, rerr
}

// addUsage sums token accounting across attempts.
func addUsage(a, b provider.Usage) provider.Usage {
	return provider.Usage{
		InputTokens:      a.InputTokens + b.InputTokens,
		OutputTokens:     a.OutputTokens + b.OutputTokens,
		CacheReadTokens:  a.CacheReadTokens + b.CacheReadTokens,
		CacheWriteTokens: a.CacheWriteTokens + b.CacheWriteTokens,
	}
}

// settledChild reports whether a child journal is already QUIESCENT (决策
// #31 — the driver settles from the shape, no receipt event exists) and,
// if so, its result folded from that journal — the recovery path for a
// crash between the child quiescing and the driver recording
// IterationCompleted.
func settledChild(childDir string) (bool, agent.RunResult) {
	events, err := store.ReadEvents(childDir)
	if err != nil {
		return false, agent.RunResult{}
	}
	s, err := state.Fold(events)
	if err != nil {
		return false, agent.RunResult{}
	}
	quiescent, reason := state.Quiescence(s)
	if !quiescent {
		return false, agent.RunResult{}
	}
	return true, agent.RunResult{Reason: reason, GenSteps: s.Session.GenStep, Usage: s.Session.Usage}
}

// seriesMemoryMaxBytes caps the injected series memory: the authority
// boundary is AT injection (DESIGN: 权威边界在注入时截断) — an agent that
// lets its own doc grow cannot bloat the next iteration's context.
const seriesMemoryMaxBytes = 8 * 1024

// buildTask renders one iteration's task: the spec task plus the truncated
// series-memory block when configured. A missing file is simply no block —
// the first iteration has nothing to remember yet.
func (d *Driver) buildTask() string {
	task := d.Spec.Task
	if d.Spec.SeriesMemory == "" || d.Exec == nil || d.Exec.WS == nil {
		return task
	}
	path, err := d.Exec.WS.Resolve(d.Spec.SeriesMemory)
	if err != nil {
		return task
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return task
	}
	mem := string(raw)
	truncated := false
	if len(mem) > seriesMemoryMaxBytes {
		mem = mem[:seriesMemoryMaxBytes]
		truncated = true
	}
	block := "\n\n<series-memory path=\"" + d.Spec.SeriesMemory + "\">\n" + mem
	if truncated {
		block += "\n[truncated at " + strconv.Itoa(seriesMemoryMaxBytes) + " bytes — keep this file short]"
	}
	block += "\n</series-memory>"
	return task + block
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
func (d *Driver) verify(ctx context.Context, appendE appendFunc, n int, childDir string, exec *tool.Executor) (event.IterationVerdict, provider.Usage) {
	agg := event.IterationVerdict{Pass: true}
	var spent provider.Usage
	for i, v := range d.Spec.Verifiers {
		vv, usage := d.verifyOne(ctx, appendE, n, i, v, childDir, exec)
		spent = addUsage(spent, usage)
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
	return agg, spent
}

// verifyOne runs one verifier as a JOURNALED, ADJUDICATED effect (S7 还债①):
// EffectRequested/Resolved record the gate verdict, ActivityStarted/Completed
// bracket the execution — the event log is the trace (DESIGN §Observability).
// A denial fails the gate verdict, never a silent pass; the human verifier IS
// the ask path already, so it gets the activity trace without re-adjudication.
func (d *Driver) verifyOne(ctx context.Context, appendE appendFunc, n, idx int, v VerifierSpec, childDir string, exec *tool.Executor) (event.IterationVerdict, provider.Usage) {
	actID := fmt.Sprintf("verify-i%d-k%d", n, idx)
	failed := func(detail string) (event.IterationVerdict, provider.Usage) {
		return event.IterationVerdict{Verifier: v.Kind, Detail: detail}, provider.Usage{}
	}
	switch v.Kind {
	case VerifierCommand:
		args, _ := json.Marshal(map[string]string{"command": v.Command})
		ok, reason, err := d.adjudicateVerifier(ctx, appendE, "eff-"+actID, "bash", "execute", "tool_call", args, 0, exec)
		if err != nil {
			return failed("verifier journal: " + err.Error())
		}
		if !ok {
			return failed("denied: " + reason)
		}
		if _, err := appendE(event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: actID, Kind: event.KindTool, Name: "verifier:command",
			Args: redact.FromEnv().JSON(args), Attempt: 1, Idempotent: true,
		}); err != nil {
			return failed("verifier journal: " + err.Error())
		}
		vv := d.verifyCommand(ctx, v, exec)
		d.completeVerifier(appendE, actID, vv, nil)
		return vv, provider.Usage{}
	case VerifierLLMJudge:
		ok, reason, err := d.adjudicateVerifier(ctx, appendE, "eff-"+actID, "", "", "llm_call", nil, 1024, nil)
		if err != nil {
			return failed("verifier journal: " + err.Error())
		}
		if !ok {
			return failed("denied: " + reason)
		}
		if _, err := appendE(event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: actID, Kind: event.KindLLM, Name: "verifier:llm_judge",
			Attempt: 1, Idempotent: true,
		}); err != nil {
			return failed("verifier journal: " + err.Error())
		}
		vv, usage := d.verifyLLMJudge(ctx, v, childDir)
		d.completeVerifier(appendE, actID, vv, &usage)
		return vv, usage
	case VerifierHuman:
		if _, err := appendE(event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: actID, Kind: event.KindTool, Name: "verifier:human",
			Attempt: 1, Idempotent: true,
		}); err != nil {
			return failed("verifier journal: " + err.Error())
		}
		vv := d.verifyHuman(ctx, n, v, childDir)
		d.completeVerifier(appendE, actID, vv, nil)
		return vv, provider.Usage{}
	default:
		return failed("unknown verifier kind " + v.Kind)
	}
}

// adjudicateVerifier runs a verifier effect through the pipeline, journaling
// the request and resolution into the driver stream. ask TIGHTENS to deny:
// a verifier is config-declared, nobody sits behind it to answer (记档).
func (d *Driver) adjudicateVerifier(ctx context.Context, appendE appendFunc,
	effID, toolName, class, kind string, args json.RawMessage, estTokens int,
	exec *tool.Executor) (bool, string, error) {

	if _, err := appendE(event.TypeEffectRequested, &event.EffectRequested{EffectID: effID}); err != nil {
		return false, "", err
	}
	network := ""
	var containment *event.Containment
	if toolName == "bash" {
		if exec == nil {
			reason := "command verifier requires an executor-backed OS sandbox"
			gates := []event.GateResult{{Gate: "containment", Decision: event.VerdictDeny, Reason: reason}}
			_, err := appendE(event.TypeEffectResolved, &event.EffectResolved{
				EffectID: effID, Verdict: event.VerdictDeny, GateResults: gates,
			})
			return false, reason, err
		}
		info, err := exec.SandboxInfo()
		if err != nil {
			reason := "required OS sandbox unavailable: " + err.Error()
			gates := []event.GateResult{{Gate: "containment", Decision: event.VerdictDeny, Reason: reason}}
			_, jerr := appendE(event.TypeEffectResolved, &event.EffectResolved{
				EffectID: effID, Verdict: event.VerdictDeny, GateResults: gates,
			})
			return false, reason, jerr
		}
		network = info.Network
		containment = &event.Containment{
			Filesystem: info.Filesystem, Network: info.Network, Backend: info.Backend,
		}
	}
	outcome, err := d.Pipeline.Evaluate(ctx, pipeline.Effect{
		ID: effID, Kind: kind, ToolName: toolName, Class: class,
		Args: args, EstTokens: estTokens, Network: network,
	})
	if err != nil {
		return false, "", err
	}
	verdict := outcome.Verdict
	reason := ""
	if verdict == event.VerdictAsk {
		verdict = event.VerdictDeny
		reason = "ask tightened to deny for a config-declared verifier"
	}
	if verdict == event.VerdictDeny && reason == "" {
		for _, g := range outcome.GateResults {
			if g.Decision == event.VerdictDeny {
				reason = g.Gate + ": " + g.Reason
			}
		}
	}
	if _, err := appendE(event.TypeEffectResolved, &event.EffectResolved{
		EffectID: effID, Verdict: verdict, GateResults: outcome.GateResults,
		Containment: containment,
	}); err != nil {
		return false, "", err
	}
	return verdict == event.VerdictAllow, reason, nil
}

// completeVerifier journals the verifier activity's terminal with the verdict
// as its result (and the judge's usage when present). A journal failure here
// is logged, not fatal — the verdict itself still rides IterationCompleted.
func (d *Driver) completeVerifier(appendE appendFunc, actID string, vv event.IterationVerdict, usage *provider.Usage) {
	result, _ := json.Marshal(vv)
	if _, err := appendE(event.TypeActivityCompleted, &event.ActivityCompleted{
		ActivityID: actID, Result: redact.FromEnv().JSON(result), Usage: usage,
	}); err != nil {
		slog.Warn("driver: verifier activity terminal journal failed", "activity", actID, "err", err)
	}
}

// verifyCommand runs a bash-class verifier against the workspace. With a
// metric regex, capture group 1 is parsed as the score (≥ threshold passes);
// otherwise exit code 0 passes. The caller has already journaled pipeline
// adjudication and OS containment evidence; the executor enforces that
// boundary again and fails closed if the backend disappears.
func (d *Driver) verifyCommand(ctx context.Context, v VerifierSpec, exec *tool.Executor) event.IterationVerdict {
	if exec == nil {
		return event.IterationVerdict{Verifier: VerifierCommand, Detail: "no executor for command verifier"}
	}
	args, _ := json.Marshal(map[string]string{"command": v.Command})
	res := exec.Execute(ctx, "bash", args)
	var out struct {
		ExitCode *int   `json:"exit_code"`
		Stdout   string `json:"stdout"`
	}
	_ = json.Unmarshal(res.Payload, &out)
	if out.ExitCode == nil {
		return event.IterationVerdict{Verifier: VerifierCommand,
			Detail: "verifier execution failed: " + redact.FromEnv().String(string(res.Payload))}
	}

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

	pass := *out.ExitCode == 0
	score := 0.0
	if pass {
		score = 1
	}
	return event.IterationVerdict{
		Pass: pass, Verifier: VerifierCommand, Score: score,
		Detail: fmt.Sprintf("exit=%d", *out.ExitCode),
	}
}

// verifyLLMJudge scores the child's result against a rubric with a single LLM
// call (DESIGN: llm_judge = LLM activity + rubric + threshold). The judge is
// asked for a strict JSON verdict; an explicit "pass" wins, otherwise score ≥
// threshold. A judge that cannot be reached or parsed fails the gate — never
// a silent pass.
func (d *Driver) verifyLLMJudge(ctx context.Context, v VerifierSpec, childDir string) (event.IterationVerdict, provider.Usage) {
	if d.Judge == nil {
		return event.IterationVerdict{Verifier: VerifierLLMJudge, Detail: "no judge provider configured"}, provider.Usage{}
	}
	model, maxTokens := "", 1024
	if d.Spec.Agent != nil {
		model = d.Spec.Agent.Model.ID
	}
	req := provider.CompleteRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System: v.Rubric + "\n\nYou are a strict verifier. Respond with ONLY a JSON object " +
			`{"score": <number 0-1>, "pass": <true|false>, "reason": <short string>}.`,
		Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{
			{Kind: provider.PartText, Text: "Result to verify:\n" + childReport(childDir)}}}},
	}
	turn, err := provider.CollectTurnStreaming(d.Judge.Complete(ctx, req), func(string) {})
	if err != nil {
		return event.IterationVerdict{Verifier: VerifierLLMJudge,
			Detail: "judge call failed: " + redact.FromEnv().String(err.Error())}, turn.Usage
	}
	var j struct {
		Score  float64 `json:"score"`
		Pass   *bool   `json:"pass"`
		Reason string  `json:"reason"`
	}
	raw := firstJSONObject(assistantText(turn.Message))
	if err := json.Unmarshal([]byte(raw), &j); err != nil {
		return event.IterationVerdict{Verifier: VerifierLLMJudge, Detail: "judge output not parseable"}, turn.Usage
	}
	pass := j.Score >= v.Threshold
	if j.Pass != nil {
		pass = *j.Pass
	}
	return event.IterationVerdict{
		Pass: pass, Verifier: VerifierLLMJudge, Score: j.Score,
		Detail: redact.FromEnv().String(j.Reason),
	}, turn.Usage
}

// verifyHuman asks a person whether the iteration meets the goal, reusing the
// agent's ask path (DESIGN: human verifier = the existing ask path; it may
// hang for days for free). Approve passes; deny or an unset non-interactive
// resolver fails closed.
func (d *Driver) verifyHuman(ctx context.Context, n int, v VerifierSpec, childDir string) event.IterationVerdict {
	resolver := d.Approvals
	if resolver == nil {
		resolver = &agent.EnvApprovals{}
	}
	args, _ := json.Marshal(map[string]string{
		"goal":   v.Rubric,
		"result": excerpt(childReport(childDir)),
	})
	dec, err := resolver.Resolve(ctx, agent.ApprovalRequest{
		ApprovalID: fmt.Sprintf("verify-%s-i%d", d.DriverID, n),
		Agent:      d.Spec.Name + " (driver goal check)",
		ToolName:   "verify_goal",
		Args:       args,
	})
	if err != nil {
		return event.IterationVerdict{Verifier: VerifierHuman, Detail: "human verify failed: " + redact.FromEnv().String(err.Error())}
	}
	score := 0.0
	if dec.Approve {
		score = 1
	}
	return event.IterationVerdict{
		Pass: dec.Approve, Verifier: VerifierHuman, Score: score,
		Detail: redact.FromEnv().String(dec.Reason),
	}
}

// firstJSONObject returns the substring from the first '{' to the last '}'
// (inclusive), so a judge that wraps its verdict in prose still parses. The
// whole string is returned when no braces are present (json.Unmarshal then
// reports the real error).
func firstJSONObject(s string) string {
	start, end := strings.IndexByte(s, '{'), strings.LastIndexByte(s, '}')
	if start < 0 || end < start {
		return s
	}
	return s[start : end+1]
}

// assistantText returns the first text part of a message (the judge's verdict).
func assistantText(m provider.Message) string {
	for _, p := range m.Parts {
		if p.Kind == provider.PartText {
			return p.Text
		}
	}
	return ""
}

// publishCarry stores the full carry doc in the CAS and returns its ref (empty
// when there is no store or no text). The full text lives in the blob; only
// the ref + a short excerpt ride IterationCompleted, keeping the log lean
// (DESIGN: carry 文档存 ArtifactStore). Redaction precedes the write, as with
// every persisted payload.
func (d *Driver) publishCarry(text string) string {
	if d.Artifacts == nil || text == "" {
		return ""
	}
	v, err := d.Artifacts.Publish("carry", []byte(redact.FromEnv().String(text)))
	if err != nil {
		slog.Warn("driver: carry publish failed", "driver", d.DriverID, "err", err)
		return ""
	}
	return v.Ref
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
	return s.Session.Usage
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
