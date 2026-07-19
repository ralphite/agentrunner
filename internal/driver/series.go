package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/cron"
	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// The merged-stream series form (INC-77, E1③): the SAME engine params and
// verifier machinery as the legacy driver, journaling into a SESSION
// journal — head SessionStarted, iterations as spawn facts
// (SpawnRequested/SubagentCompleted, children via the agent.ChildRun
// substrate), verdict/carry/stall folded into the Series sub-state. The
// parent is PROGRAM-DRIVEN: no LLM generation ever happens in this
// journal, so 决策 #21's scheduling determinism is intact — the facts just
// changed vocabulary. The legacy stream form stays alongside until E1④.

// seriesTickPurpose prefixes the durable wake-hint timers a cadenced series
// arms before each wait — the daemon sweep (77.3) resumes an unhosted
// series off them, exactly like schedule_wake timers.
const seriesTickPurpose = "series_tick:"

// SupportsSeries reports whether this spec runs in the merged-stream form:
// every legacy shape (INC-80.2b complete) except the parallel×retry combo,
// which keeps its legacy semantics until someone actually needs it —
// dispatchers use this to route.
func (d *Driver) SupportsSeries() bool {
	switch d.Spec.schedule() {
	case ScheduleImmediate:
		if len(d.Spec.Verifiers) == 0 {
			return false
		}
	case ScheduleInterval, ScheduleCron, ScheduleSelfPaced:
	case ScheduleParallel:
		return d.Spec.OnChildFailure.Mode != OnFailRetry
	default:
		return false
	}
	return true
}

// prepareSeries validates the spec the merged form carries — every legacy
// shape except parallel×retry (记档 INC-80 工作纸): refusing loudly beats
// silently changing its semantics.
func (d *Driver) prepareSeries() error {
	if d.Clock == nil {
		d.Clock = clock.Real{}
	}
	switch d.Spec.schedule() {
	case ScheduleImmediate:
		if len(d.Spec.Verifiers) == 0 {
			return fmt.Errorf("series: goal mode requires at least one verifier")
		}
	case ScheduleInterval:
		if _, err := d.Spec.interval(); err != nil {
			return fmt.Errorf("series: bad interval %q: %w", d.Spec.Interval, err)
		}
	case ScheduleCron:
		sched, err := cron.Parse(d.Spec.Cron)
		if err != nil {
			return fmt.Errorf("series: %w", err)
		}
		d.cronSched = sched
	case ScheduleSelfPaced:
		if _, _, err := d.Spec.paceBounds(); err != nil {
			return fmt.Errorf("series: %w", err)
		}
		switch d.Spec.OnNoIntent {
		case "", NoIntentFinish:
		case NoIntentContinue:
			if d.Spec.PaceMin == "" {
				return fmt.Errorf("series: on_no_intent=continue requires pace_min — a forgetful child must not spin the series")
			}
		default:
			return fmt.Errorf("series: on_no_intent %q unknown (known: finish, continue)", d.Spec.OnNoIntent)
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
			return fmt.Errorf("series: schedule parallel requires n >= 2 (got %d)", d.Spec.N)
		}
		if len(d.Spec.Verifiers) == 0 {
			return fmt.Errorf("series: schedule parallel requires verifiers — they select the winner")
		}
		if d.Snapshots == nil || d.NewChildAt == nil {
			return fmt.Errorf("series: schedule parallel requires a snapshot store and a worktree child factory")
		}
		if d.Spec.OnChildFailure.Mode == OnFailRetry {
			return fmt.Errorf("series: parallel with on_child_failure=retry stays on the legacy stream")
		}
	default:
		return fmt.Errorf("series: schedule %q not yet available in the merged-stream form (use the legacy driver)", d.Spec.schedule())
	}
	switch d.Spec.Overlap {
	case "", OverlapSkip, OverlapCoalesce:
	default:
		return fmt.Errorf("series: overlap %q unknown (known: skip, coalesce)", d.Spec.Overlap)
	}
	if d.Spec.OnChildFailure.Mode == OnFailRetry && d.Spec.OnChildFailure.Max < 0 {
		return fmt.Errorf("series: on_child_failure.max must be >= 0")
	}
	if d.NewChild == nil {
		return fmt.Errorf("series: NewChild factory is required")
	}
	return nil
}

// seriesAppendFunc is the single write path of the merged stream: append,
// fold into the session state, tee lifecycle events to watchers.
func (d *Driver) seriesAppendFunc(ss *state.State) appendFunc {
	r := redact.FromEnv()
	return func(typ string, payload any) (event.Envelope, error) {
		env, err := event.New(typ, payload)
		if err != nil {
			return env, err
		}
		// Blanket credential redaction on EVERY journaled payload — the
		// same invariant the agent appender enforces (loop.go): prompts and
		// series-memory excerpts ride SessionStarted/SpawnRequested here,
		// and the harness never journals a credential (INC-80 安全 review P1).
		env.Payload = r.JSON(env.Payload)
		env.CorrelationID = d.DriverID
		appended, err := d.Store.Append(env)
		if err != nil {
			return appended, err
		}
		next, err := state.Apply(*ss, appended)
		if err != nil {
			return appended, err
		}
		*ss = next
		switch p := payload.(type) {
		case *event.SeriesIteration:
			if !p.Skipped {
				d.emit(protocol.Event{Kind: protocol.KindIteration, N: p.N,
					Reason: p.Reason,
					Text: fmt.Sprintf("iteration %d %s (pass=%v score=%g)",
						p.N, p.Reason, p.Verdict.Pass, p.Verdict.Score)})
			}
		case *event.SeriesEnded:
			d.emit(protocol.Event{Kind: protocol.KindRunEnd, N: p.Iterations, Reason: p.Reason})
		}
		return appended, nil
	}
}

// RunSeries starts a fresh merged-stream series: session head, series
// opening, then the drive loop.
func (d *Driver) RunSeries(ctx context.Context) (Result, error) {
	if err := d.prepareSeries(); err != nil {
		return Result{}, err
	}
	ss := state.New()
	appendE := d.seriesAppendFunc(&ss)
	specJSON, _ := json.Marshal(d.Spec)
	wsRoot := ""
	if d.Exec != nil && d.Exec.WS != nil {
		wsRoot = d.Exec.WS.Root()
	}
	if _, err := appendE(event.TypeSessionStarted, &event.SessionStarted{
		SpecName: d.Spec.Name, Prompt: d.Spec.Prompt,
		SubStateVersions: state.SubStateVersions(),
		Spec:             redact.FromEnv().JSON(specJSON), WorkspaceRoot: wsRoot,
	}); err != nil {
		return Result{}, err
	}
	kind := d.Spec.schedule()
	baseRef := ""
	if kind == ScheduleParallel {
		// The round's shared base pins BEFORE the series opens
		// (blob-before-event): any crash after this point re-materializes
		// the SAME tree for every remaining attempt.
		kind = "best_of_n"
		ref, err := d.Snapshots.Snapshot(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("series: base snapshot for parallel round: %w", err)
		}
		baseRef = ref
	}
	if _, err := appendE(event.TypeSeriesStarted, &event.SeriesStarted{
		SeriesID: d.DriverID, Kind: kind,
		MaxIterations: d.Spec.maxIterations(), Patience: d.Spec.Patience,
		Overlap: d.Spec.Overlap, Source: "user", BaseRef: baseRef,
	}); err != nil {
		return Result{}, err
	}
	if d.Spec.schedule() == ScheduleParallel {
		return d.driveSeriesParallel(ctx, &ss, appendE, 1)
	}
	return d.driveSeries(ctx, &ss, appendE, 1)
}

// ResumeSeries rebuilds the series position from the SESSION fold and
// continues — the merged-stream mirror of Resume. Pending series tick
// timers are wake HINTS: cadence re-derives from Series.LastTick, so stale
// ones are cancelled rather than fired.
func (d *Driver) ResumeSeries(ctx context.Context) (Result, error) {
	events, err := store.ReadEvents(d.Store.Dir())
	if err != nil {
		return Result{}, err
	}
	ss, err := state.Fold(events)
	if err != nil {
		return Result{}, err
	}
	if ss.Series == nil {
		return Result{}, fmt.Errorf("series resume: journal has no series (not a merged-stream drive)")
	}
	if err := d.prepareSeries(); err != nil {
		return Result{}, err
	}
	appendE := d.seriesAppendFunc(&ss)
	for id, tm := range ss.Timers {
		if len(tm.Purpose) > len(seriesTickPurpose) && tm.Purpose[:len(seriesTickPurpose)] == seriesTickPurpose {
			if _, err := appendE(event.TypeTimerCancelled, &event.TimerCancelled{TimerID: id}); err != nil {
				return Result{}, err
			}
		}
	}
	sr := ss.Series
	if sr.Ended {
		return Result{Reason: sr.EndReason, Iterations: len(sr.Iterations), BestIter: sr.BestIter}, nil
	}
	// INC-54 backfill: anchor cadence at the last consumed slot so missed
	// ticks settle exactly once per the overlap policy.
	if !sr.LastTick.IsZero() {
		d.lastTick = sr.LastTick
	}
	// Goal mode: a crash between a terminal-deciding iteration and
	// SeriesEnded re-derives that decision instead of launching a
	// redundant child (GOAL MODE ONLY — in loop mode a pass is a quality
	// gate, never a terminal; S6 review P0 precedent).
	if d.Spec.schedule() == ScheduleImmediate {
		if last, ok := lastSeriesCompleted(sr); ok {
			if last.Verdict.Pass {
				return d.finishSeries(appendE, sr, "satisfied", last.N)
			}
			if seriesStalled(d.Spec.Patience, sr) {
				return d.finishSeries(appendE, sr, "stalled", last.N)
			}
		}
	}
	startN := 1
	for _, it := range sr.Iterations {
		if it.N >= startN {
			startN = it.N + 1
		}
	}
	// self_paced pace is runner memory — re-derive it from the last
	// completed child's journal so a resumed series honors the declared
	// pace instead of firing immediately (legacy Resume contract).
	if d.Spec.schedule() == ScheduleSelfPaced && startN > 1 {
		if last, ok := lastSeriesCompleted(sr); ok {
			floor, ceil, _ := d.Spec.paceBounds()
			tail := last.ChildSession
			if i := strings.LastIndex(tail, "-sub-"); i >= 0 {
				tail = tail[i+len("-sub-"):]
			}
			intent := childIntent(filepath.Join(d.Store.Dir(), "sub", tail))
			switch {
			case intent.has && !intent.finish:
				d.nextPace = clampPace(intent.after, floor, ceil)
			default:
				d.nextPace = floor
			}
		}
	}
	if d.Spec.schedule() == ScheduleParallel {
		return d.driveSeriesParallel(ctx, &ss, appendE, startN)
	}
	return d.driveSeries(ctx, &ss, appendE, startN)
}

// driveSeries is the merged-stream drive loop — the drive() mirror over the
// Series fold. Every decision reads the fold (journal, never memory).
func (d *Driver) driveSeries(ctx context.Context, ss *state.State, appendE appendFunc, startN int) (Result, error) {
	loopMode := d.Spec.schedule() != ScheduleImmediate
	maxIter := d.Spec.maxIterations()
	for n := startN; ; n++ {
		sr := ss.Series
		if ctx.Err() != nil {
			return d.seriesCancelTerminal(ctx, appendE, sr, n-1)
		}
		if n > maxIter {
			return d.finishSeries(appendE, sr, seriesLimitReason(sr), maxIter)
		}
		var tick time.Time
		if loopMode {
			run, t, terr := d.awaitSeriesTick(ctx, ss, appendE, n, n == 1)
			if terr != nil {
				if ctx.Err() != nil {
					return d.seriesCancelTerminal(ctx, appendE, ss.Series, n-1)
				}
				return Result{}, terr
			}
			if !run {
				continue // overlap=skip consumed n as a skipped iteration
			}
			tick = t
		}
		sr = ss.Series
		allowance, ok := d.reserveSeries(sr)
		if !ok {
			return d.finishSeries(appendE, sr, "budget", n-1)
		}

		callID := fmt.Sprintf("iter-%d", n)
		childRes, childDir, childSession, spent, cerr := d.runSeriesIteration(ctx, appendE, n, allowance)
		reason := childRes.Reason
		if cerr != nil {
			if ctx.Err() != nil {
				reason = "canceled"
			} else if reason == "" {
				reason = "error"
			}
		} else if reason == "" {
			reason = "completed"
		}

		if cerr != nil && ctx.Err() != nil {
			if _, err := appendE(event.TypeSeriesIteration, &event.SeriesIteration{
				SeriesID: d.DriverID, N: n, CallID: callID, ChildSession: childSession,
				Reason: "canceled", Usage: spent, Tick: tick,
				Verdict: event.IterationVerdict{Detail: "series canceled"},
			}); err != nil {
				return Result{}, err
			}
			return d.seriesCancelTerminal(ctx, appendE, ss.Series, n)
		}
		if cerr != nil {
			// on_child_failure (retry already exhausted its attempts inside
			// runSeriesIteration): surface keeps the series alive (a failed
			// child is a spent iteration); stop ends it.
			if _, err := appendE(event.TypeSeriesIteration, &event.SeriesIteration{
				SeriesID: d.DriverID, N: n, CallID: callID, ChildSession: childSession,
				Reason: "error", Usage: spent, Tick: tick,
				Verdict: event.IterationVerdict{Detail: "child failed: " + redact.FromEnv().String(cerr.Error())},
			}); err != nil {
				return Result{}, err
			}
			if d.Spec.OnChildFailure.Mode == OnFailSurface {
				if seriesStalled(d.Spec.Patience, ss.Series) {
					return d.finishSeries(appendE, ss.Series, "stalled", n)
				}
				continue
			}
			return d.finishSeries(appendE, ss.Series, "child_failed", n)
		}

		var verdict event.IterationVerdict
		if len(d.Spec.Verifiers) > 0 {
			var judgeUsage provider.Usage
			verdict, judgeUsage = d.verify(ctx, appendE, n, childDir, d.Exec)
			spent = addUsage(spent, judgeUsage)
		}
		carryText := childReport(childDir)
		if _, err := appendE(event.TypeSeriesIteration, &event.SeriesIteration{
			SeriesID: d.DriverID, N: n, CallID: callID, ChildSession: childSession,
			Reason: reason, Verdict: verdict, Usage: spent, Tick: tick,
			CarryRef: d.publishCarry(carryText), Carry: excerpt(carryText),
		}); err != nil {
			return Result{}, err
		}
		if !loopMode {
			if verdict.Pass {
				return d.finishSeries(appendE, ss.Series, "satisfied", n)
			}
			if seriesStalled(d.Spec.Patience, ss.Series) {
				return d.finishSeries(appendE, ss.Series, "stalled", n)
			}
		}
		if d.Spec.schedule() == ScheduleSelfPaced {
			done, res, err := d.applySeriesPaceIntent(ctx, appendE, ss, n, childDir)
			if err != nil {
				return Result{}, err
			}
			if done {
				return res, nil
			}
		}
	}
}

// applySeriesPaceIntent consumes the child's declared pace (INC-80.2b②) —
// the merged-stream mirror of applyPaceIntent: finish_series goes through
// the human gate (approve → satisfied, deny → floor pace and the series
// continues); schedule_next clamps into [pace_min, pace_max]; no intent
// follows on_no_intent.
func (d *Driver) applySeriesPaceIntent(ctx context.Context, appendE appendFunc, ss *state.State, n int, childDir string) (bool, Result, error) {
	intent := childIntent(childDir)
	floor, ceil, _ := d.Spec.paceBounds() // validated in prepare
	switch {
	case intent.finish:
		if d.confirmFinish(ctx, n, childDir) {
			res, err := d.finishSeries(appendE, ss.Series, "satisfied", n)
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
		res, err := d.finishSeries(appendE, ss.Series, "satisfied", n)
		return true, res, err
	}
}

// driveSeriesParallel is the best-of-N round in the merged stream
// (INC-80.2b③) — the driveParallel mirror: every attempt materializes the
// SAME base snapshot (pinned on SeriesStarted) into its own worktree, is
// judged in its own tree, and the selection (pass beats score, ties keep
// the earliest) rides SeriesEnded.BestIter as the fold authority.
func (d *Driver) driveSeriesParallel(ctx context.Context, ss *state.State, appendE appendFunc, startN int) (Result, error) {
	total := d.Spec.N
	baseRef := ss.Series.BaseRef
	if baseRef == "" {
		return Result{}, fmt.Errorf("series: parallel round has no pinned base snapshot")
	}
	// The ref crosses journal→git argv (Materialize): only a plain object
	// id may pass — a tampered journal value must not become a git option
	// (安全 review P2-1 hardening).
	if !isPlainObjectID(baseRef) {
		return Result{}, fmt.Errorf("series: journaled base ref %q is not a plain object id", baseRef)
	}
	for n := startN; n <= total; n++ {
		if ctx.Err() != nil {
			return d.seriesCancelTerminal(ctx, appendE, ss.Series, n-1)
		}
		allowance, ok := d.reserveSeries(ss.Series)
		if !ok {
			return d.finishSeries(appendE, ss.Series, "budget", n-1)
		}
		callID := fmt.Sprintf("att-%d", n)
		childSession := fmt.Sprintf("%s-sub-%s-a1", d.DriverID, callID)
		childDir := filepath.Join(d.Store.Dir(), "sub", callID+"-a1")
		worktree := filepath.Join(d.Store.Dir(), "wt", fmt.Sprintf("att-%d", n))
		if _, serr := os.Stat(worktree); os.IsNotExist(serr) {
			// Materialize is atomic (temp + rename): an existing dir IS a
			// complete tree, so existence doubles as the resume marker.
			if err := d.Snapshots.Materialize(ctx, baseRef, worktree); err != nil {
				return Result{}, fmt.Errorf("series: materialize attempt %d worktree: %w", n, err)
			}
			// A child journal without its worktree = the tree was lost
			// across a restart. Judging (or resuming) it on the fresh BASE
			// would silently roll back its own edits — fail the attempt
			// instead (S7 出口 review P1).
			if evs, rerr := store.ReadEvents(childDir); rerr == nil && len(evs) > 0 {
				if _, err := appendE(event.TypeSeriesIteration, &event.SeriesIteration{
					SeriesID: d.DriverID, N: n, CallID: callID, ChildSession: childSession,
					Reason:  "error",
					Verdict: event.IterationVerdict{Detail: "attempt worktree lost across restart; refusing to judge a rolled-back tree"},
				}); err != nil {
					return Result{}, err
				}
				continue
			}
		}
		if _, err := appendE(event.TypeSpawnRequested, &event.SpawnRequested{
			CallID: callID, Agent: agentNameOf(d.Spec), Prompt: d.buildPrompt(),
			ChildSession: childSession, Depth: 1, BudgetTokens: allowance,
			LeaseID: "lease-" + callID + "-a1",
		}); err != nil {
			return Result{}, err
		}
		childRes, spent, cerr := d.runSeriesChildAt(ctx, childDir, childSession, allowance, worktree)
		if cerr == nil && (childRes.Reason == "error" || childRes.Reason == "canceled" ||
			strings.HasPrefix(childRes.Reason, "failed")) {
			cerr = fmt.Errorf("child ended %s (settled from journal)", childRes.Reason)
		}
		reason := childRes.Reason
		if cerr != nil {
			if ctx.Err() != nil {
				reason = "canceled"
			} else if reason == "" {
				reason = "error"
			}
		} else if reason == "" {
			reason = "completed"
		}
		if _, err := appendE(event.TypeSubagentCompleted, &event.SubagentCompleted{
			CallID: callID, Agent: agentNameOf(d.Spec), ChildSession: childSession,
			Reason: reason, GenSteps: childRes.GenSteps, Usage: spent,
		}); err != nil {
			return Result{}, err
		}
		if cerr != nil {
			if _, err := appendE(event.TypeSeriesIteration, &event.SeriesIteration{
				SeriesID: d.DriverID, N: n, CallID: callID, ChildSession: childSession,
				Reason: reason, Usage: spent,
				Verdict: event.IterationVerdict{Detail: "attempt failed: " + redact.FromEnv().String(cerr.Error())},
			}); err != nil {
				return Result{}, err
			}
			if ctx.Err() != nil {
				return d.seriesCancelTerminal(ctx, appendE, ss.Series, n)
			}
			continue // a failed attempt is one spent slot; the round goes on
		}
		wtExec, werr := worktreeExecutor(worktree, childSession)
		if werr != nil {
			return Result{}, werr
		}
		verdict, judgeUsage := d.verify(ctx, appendE, n, childDir, wtExec)
		spent = addUsage(spent, judgeUsage)
		carryText := childReport(childDir)
		if _, err := appendE(event.TypeSeriesIteration, &event.SeriesIteration{
			SeriesID: d.DriverID, N: n, CallID: callID, ChildSession: childSession,
			Reason: reason, Verdict: verdict, Usage: spent,
			CarryRef: d.publishCarry(carryText), Carry: excerpt(carryText),
		}); err != nil {
			return Result{}, err
		}
	}
	// Selection: pass beats any score; among equals the higher score wins;
	// ties keep the earliest. The choice rides SeriesEnded.BestIter — the
	// fold applies it as the authority over its max-score default.
	best, bestPass, bestScore := 0, false, 0.0
	for _, it := range ss.Series.Iterations {
		if it.Skipped {
			continue
		}
		v := it.Verdict
		better := best == 0 ||
			(v.Pass && !bestPass) ||
			(v.Pass == bestPass && v.Score > bestScore)
		if better {
			best, bestPass, bestScore = it.N, v.Pass, v.Score
		}
	}
	reason := "stalled"
	if bestPass {
		reason = "satisfied"
	} else if seriesAllChildrenFailed(ss.Series) {
		reason = "child_failed"
	}
	if best > 0 {
		d.emit(protocol.Event{Kind: protocol.KindNote,
			Text: fmt.Sprintf("best-of-%d winner: attempt %d (worktree %s)",
				total, best, filepath.Join(d.Store.Dir(), "wt", fmt.Sprintf("att-%d", best)))})
	}
	if _, err := appendE(event.TypeSeriesEnded, &event.SeriesEnded{
		SeriesID: d.DriverID, Reason: reason, Iterations: total, BestIter: best,
	}); err != nil {
		return Result{}, err
	}
	return Result{Reason: reason, Iterations: total, BestIter: best}, nil
}

// isPlainObjectID accepts only a bare lowercase-hex git object id (40-64
// chars) — the shape ShadowRepo.Snapshot returns.
func isPlainObjectID(ref string) bool {
	if n := len(ref); n < 40 || n > 64 {
		return false
	}
	for _, c := range ref {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// seriesAllChildrenFailed reports whether every non-skipped iteration's
// child ended in error — the round's child_failed terminal condition.
func seriesAllChildrenFailed(sr *state.Series) bool {
	seen := false
	for _, it := range sr.Iterations {
		if it.Skipped {
			continue
		}
		seen = true
		if it.Reason != "error" && it.Reason != "canceled" {
			return false
		}
	}
	return seen
}

// runSeriesChildAt is runSeriesChild bound to an attempt worktree — the
// child's whole face (executor and permission paths) lives in its own tree.
func (d *Driver) runSeriesChildAt(ctx context.Context, childDir, childSession string, allowance int, worktree string) (agent.RunResult, provider.Usage, error) {
	cr, err := agent.OpenChildRun(childDir)
	if err != nil {
		return agent.RunResult{}, provider.Usage{}, err
	}
	defer cr.Close()
	if done, dres := agent.SettledChild(childDir); done {
		return dres, dres.Usage, nil
	}
	child := d.NewChildAt(cr.Store(), childSession, 0, allowance, worktree)
	return cr.Run(ctx, child, d.buildPrompt())
}

// runSeriesIteration runs iteration n through its attempt loop (INC-80.2b):
// each attempt is its own spawn fact pair (SpawnRequested/SubagentCompleted)
// with its own child journal `sub/iter-N-aM`, and the returned spend SUMS
// every attempt — a failed retry burned real tokens, the budget position
// must say so (same doctrine as the legacy stream). Returns the LAST
// attempt's dir/session for verify/carry.
func (d *Driver) runSeriesIteration(ctx context.Context, appendE appendFunc, n, allowance int) (agent.RunResult, string, string, provider.Usage, error) {
	attempts := 1
	if d.Spec.OnChildFailure.Mode == OnFailRetry && d.Spec.OnChildFailure.Max > 0 {
		attempts += d.Spec.OnChildFailure.Max
	}
	callID := fmt.Sprintf("iter-%d", n)
	var (
		res      agent.RunResult
		childDir string
		session  string
		spent    provider.Usage
		rerr     error
	)
	for a := 1; a <= attempts; a++ {
		suffix := fmt.Sprintf("-a%d", a)
		session = fmt.Sprintf("%s-sub-%s%s", d.DriverID, callID, suffix)
		childDir = filepath.Join(d.Store.Dir(), "sub", callID+suffix)
		// Idempotent across a crash: the delegation fact re-records the same
		// lease (fold replaces by id), and a recorded SeriesIteration N never
		// reaches this point (startN skips it).
		if _, err := appendE(event.TypeSpawnRequested, &event.SpawnRequested{
			CallID: callID, Agent: agentNameOf(d.Spec), Prompt: d.buildPrompt(),
			ChildSession: session, Depth: 1, BudgetTokens: allowance,
			LeaseID: "lease-" + callID + suffix,
		}); err != nil {
			return res, childDir, session, spent, err
		}
		var attemptUsage provider.Usage
		res, attemptUsage, rerr = d.runSeriesChild(ctx, childDir, session, allowance)
		spent = addUsage(spent, attemptUsage)
		// A SETTLED failure (the runner crashed after the child quiesced in
		// error) must count as a failed attempt, not a success — same
		// classification the legacy stream applies across a crash.
		if rerr == nil && (res.Reason == "error" || res.Reason == "canceled" ||
			strings.HasPrefix(res.Reason, "failed")) {
			rerr = fmt.Errorf("child ended %s (settled from journal)", res.Reason)
		}
		reason := res.Reason
		if rerr != nil {
			if ctx.Err() != nil {
				reason = "canceled"
			} else if reason == "" {
				reason = "error"
			}
		} else if reason == "" {
			reason = "completed"
		}
		if _, err := appendE(event.TypeSubagentCompleted, &event.SubagentCompleted{
			CallID: callID, Agent: agentNameOf(d.Spec), ChildSession: session,
			Reason: reason, GenSteps: res.GenSteps, Usage: attemptUsage,
		}); err != nil {
			return res, childDir, session, spent, err
		}
		if rerr == nil {
			return res, childDir, session, spent, nil
		}
		if ctx.Err() != nil {
			return res, childDir, session, spent, rerr // cancel is not retryable
		}
		if a < attempts {
			slog.Warn("driver: series child attempt failed, retrying",
				"driver", d.DriverID, "iter", n, "attempt", a, "err", rerr)
		}
	}
	return res, childDir, session, spent, rerr
}

// runSeriesChild runs one iteration's child through the ChildRun substrate
// (INC-76): three-way run decision + fold-settled spend, per-attempt dir.
func (d *Driver) runSeriesChild(ctx context.Context, childDir, childSession string, allowance int) (agent.RunResult, provider.Usage, error) {
	cr, err := agent.OpenChildRun(childDir)
	if err != nil {
		return agent.RunResult{}, provider.Usage{}, err
	}
	defer cr.Close()
	if done, dres := agent.SettledChild(childDir); done {
		return dres, dres.Usage, nil
	}
	child := d.NewChild(cr.Store(), childSession, 0, allowance)
	return cr.Run(ctx, child, d.buildPrompt())
}

// awaitSeriesTick mirrors awaitTick over the merged stream: skips journal
// SeriesIteration{Skipped} facts; the wait arms a durable series_tick
// timer (wake hint for the daemon sweep) and fires it on wake. Returns the
// consumed slot's tick for the iteration fact.
func (d *Driver) awaitSeriesTick(ctx context.Context, ss *state.State, appendE appendFunc, n int, first bool) (bool, time.Time, error) {
	skip := func(next time.Time) (bool, time.Time, error) {
		_, err := appendE(event.TypeSeriesIteration, &event.SeriesIteration{
			SeriesID: d.DriverID, N: n, Skipped: true, Tick: next,
			Reason: "overlap: tick " + next.UTC().Format(time.RFC3339) + " passed while an iteration ran or the host was down",
		})
		return false, time.Time{}, err
	}
	wait := func(next time.Time) error {
		timerID := fmt.Sprintf("series:%s:%d", d.DriverID, next.Unix())
		if _, err := appendE(event.TypeTimerSet, &event.TimerSet{
			TimerID: timerID, FireAt: next, Purpose: seriesTickPurpose + d.DriverID,
		}); err != nil {
			return err
		}
		if err := d.Clock.WaitUntil(ctx, next); err != nil {
			return err
		}
		_, err := appendE(event.TypeTimerFired, &event.TimerFired{TimerID: timerID})
		return err
	}
	now := d.Clock.Now()
	switch d.Spec.schedule() {
	case ScheduleInterval:
		every, _ := d.Spec.interval()
		if every <= 0 {
			return true, now, nil // back-to-back interval mode
		}
		if first {
			d.lastTick = now
			return true, now, nil
		}
		if d.lastTick.IsZero() {
			d.lastTick = now
		}
		next := d.lastTick.Add(every)
		if !next.After(now) {
			d.lastTick = next
			if d.Spec.Overlap == OverlapCoalesce {
				for !d.lastTick.Add(every).After(now) {
					d.lastTick = d.lastTick.Add(every)
				}
				return true, d.lastTick, nil
			}
			ok, tick, err := skip(next)
			return ok, tick, err
		}
		if err := wait(next); err != nil {
			return false, time.Time{}, err
		}
		d.lastTick = next
		return true, next, nil
	case ScheduleCron:
		if d.lastTick.IsZero() {
			d.lastTick = now
		}
		next, ok := d.cronSched.Next(d.lastTick)
		if !ok {
			return false, time.Time{}, fmt.Errorf("series: cron %q never fires", d.Spec.Cron)
		}
		if !next.After(now) {
			d.lastTick = next
			if d.Spec.Overlap == OverlapCoalesce {
				for {
					nn, ok := d.cronSched.Next(d.lastTick)
					if !ok || nn.After(now) {
						break
					}
					d.lastTick = nn
				}
				return true, d.lastTick, nil
			}
			okS, tick, err := skip(next)
			return okS, tick, err
		}
		if err := wait(next); err != nil {
			return false, time.Time{}, err
		}
		d.lastTick = next
		return true, next, nil
	case ScheduleSelfPaced:
		// The child's own schedule_next intent paces the series (legacy
		// 语义): nextPace is runner memory, re-derived on resume from the
		// last child's journal. The durable timer is only a daemon wake
		// hint; the returned tick stays zero — self-paced series have no
		// absolute timeline (state.Series.LastTick contract).
		if first || d.nextPace <= 0 {
			return true, time.Time{}, nil
		}
		if err := wait(now.Add(d.nextPace)); err != nil {
			return false, time.Time{}, err
		}
		return true, time.Time{}, nil
	}
	return false, time.Time{}, fmt.Errorf("series: schedule %q has no cadence", d.Spec.schedule())
}

// seriesCancelTerminal mirrors cancelTerminal (INC-72): a graceful host
// shutdown ends a cadenced series WITHOUT a terminal so the next boot
// revives it; every other cancel and every bounded schedule writes the
// stopped terminal.
func (d *Driver) seriesCancelTerminal(ctx context.Context, appendE appendFunc, sr *state.Series, n int) (Result, error) {
	if errors.Is(context.Cause(ctx), errs.ErrHostShutdown) {
		switch d.Spec.schedule() {
		case ScheduleInterval, ScheduleCron, ScheduleSelfPaced:
			// A graceful host shutdown leaves NO terminal — the next boot's
			// drive sweep revives the series (INC-72 semantics).
			return Result{Reason: "shutdown", Iterations: n, BestIter: sr.BestIter}, nil
		}
	}
	return d.finishSeries(appendE, sr, "stopped", n)
}

func (d *Driver) finishSeries(appendE appendFunc, sr *state.Series, reason string, iterations int) (Result, error) {
	if _, err := appendE(event.TypeSeriesEnded, &event.SeriesEnded{
		SeriesID: d.DriverID, Reason: reason, Iterations: iterations, BestIter: sr.BestIter,
	}); err != nil {
		return Result{}, err
	}
	return Result{Reason: reason, Iterations: iterations, BestIter: sr.BestIter}, nil
}

// reserveSeries is reserve() over the Series fold.
func (d *Driver) reserveSeries(sr *state.Series) (allowance int, ok bool) {
	treeCap := d.Spec.Budget.MaxTotalTokens
	if treeCap <= 0 {
		return d.childCap(), true
	}
	remaining := treeCap - sr.SpentTokens
	if remaining <= 0 {
		return 0, false
	}
	if cc := d.childCap(); cc > 0 && cc < remaining {
		return cc, true
	}
	return remaining, true
}

// seriesStalled is stalled() over the Series fold: patience consecutive
// completed iterations with no improvement over the best ends the run.
func seriesStalled(patience int, sr *state.Series) bool {
	if patience <= 0 {
		return false
	}
	best := 0.0
	if sr.BestIter > 0 {
		for _, it := range sr.Iterations {
			if it.N == sr.BestIter {
				best = it.Verdict.Score
				break
			}
		}
	}
	sinceBest := 0
	for _, it := range sr.Iterations {
		if it.Skipped {
			continue
		}
		if it.N > sr.BestIter && it.Verdict.Score <= best {
			sinceBest++
		}
	}
	return sinceBest >= patience
}

func seriesLimitReason(sr *state.Series) string {
	completed, failed := 0, 0
	for _, it := range sr.Iterations {
		if it.Skipped {
			continue
		}
		completed++
		if it.Reason == "error" || it.Reason == "canceled" {
			failed++
		}
	}
	if completed > 0 && completed == failed {
		return "child_failed"
	}
	return "max_iterations"
}

// lastSeriesCompleted returns the highest-N non-skipped iteration.
func lastSeriesCompleted(sr *state.Series) (state.SeriesIterationRec, bool) {
	for i := len(sr.Iterations) - 1; i >= 0; i-- {
		if !sr.Iterations[i].Skipped {
			return sr.Iterations[i], true
		}
	}
	return state.SeriesIterationRec{}, false
}

func agentNameOf(spec *DriverSpec) string {
	if spec.Agent != nil && spec.Agent.Name != "" {
		return spec.Agent.Name
	}
	return spec.Name
}
