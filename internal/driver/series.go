package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
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
// goal-with-verifiers / interval / cron, retry included (INC-80.2b step 1).
// self_paced / parallel stay on the legacy stream until the runner grows
// them — dispatchers use this to route.
func (d *Driver) SupportsSeries() bool {
	switch d.Spec.schedule() {
	case ScheduleImmediate:
		if len(d.Spec.Verifiers) == 0 {
			return false
		}
	case ScheduleInterval, ScheduleCron:
	default:
		return false
	}
	return true
}

// prepareSeries validates the spec subset the merged form supports.
// self_paced / parallel stay on the legacy stream for now (记档 INC-80
// 工作纸): refusing loudly beats silently changing their semantics.
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
	return func(typ string, payload any) (event.Envelope, error) {
		env, err := event.New(typ, payload)
		if err != nil {
			return env, err
		}
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
	if _, err := appendE(event.TypeSeriesStarted, &event.SeriesStarted{
		SeriesID: d.DriverID, Kind: d.Spec.schedule(),
		MaxIterations: d.Spec.maxIterations(), Patience: d.Spec.Patience,
		Overlap: d.Spec.Overlap, Source: "user",
	}); err != nil {
		return Result{}, err
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
	}
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
		case ScheduleInterval, ScheduleCron:
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
