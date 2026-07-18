package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/ralphite/agentrunner/internal/cron"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/state"
)

// schedulePurposePrefix marks the durable timers owned by the in-session
// schedule (INC-74). Both wake paths key off it: the idle park (awaitInput)
// waits only on these, and the daemon timer sweep resumes an unhosted
// session when one expires. Activity-timeout timers are NEVER fired from
// the idle — they belong to their live executors.
const schedulePurposePrefix = "schedule_wake:"

// applyScheduleControl journals the fold event for one in-session schedule
// control (INC-74, E1① — the goal-control template): out-of-band, never
// conversation content. It only records change-as-event facts; all arming
// and wake mechanics live in checkSchedule at the safe point, which runs
// right after the drain and picks the new facts up in the same pass.
func (l *Loop) applyScheduleControl(ds *driveState, appendE AppendFunc, ctl protocol.Control) error {
	switch ctl.Kind {
	case protocol.ControlScheduleAttach:
		sc := ctl.Schedule
		if sc == nil {
			return nil
		}
		if err := validateCadence(sc.Interval, sc.Cron); err != nil {
			l.emit(protocol.Event{Kind: protocol.KindError, Text: "schedule attach rejected: " + err.Error()})
			return nil
		}
		// Attach replaces any prior schedule (the fold overwrites); its
		// pending timers must not survive the replacement.
		if err := cancelScheduleTimers(ds, appendE); err != nil {
			return err
		}
		// The prompt is conversation-bound text and is redacted like any
		// journaled input (凭据红线 §18.5); the base comes from the loop
		// clock, never the envelope's wall-clock TS.
		_, err := appendE(event.TypeScheduleAttached, &event.ScheduleAttached{
			ScheduleID: sc.ScheduleID, Interval: sc.Interval, Cron: sc.Cron,
			Prompt: redact.FromEnv().String(sc.Prompt), MaxWakes: sc.MaxWakes,
			Base: l.Clock.Now().UTC(), Source: "user",
		})
		return err
	case protocol.ControlSchedulePause:
		if ds.s.Schedule == nil || ds.s.Schedule.Paused {
			return nil
		}
		if _, err := appendE(event.TypeSchedulePaused, &event.SchedulePaused{
			ScheduleID: ds.s.Schedule.ScheduleID, Source: "user",
		}); err != nil {
			return err
		}
		// A paused schedule must not keep waking the idle park.
		return cancelScheduleTimers(ds, appendE)
	case protocol.ControlScheduleResume:
		if ds.s.Schedule == nil || !ds.s.Schedule.Paused {
			return nil
		}
		// Base re-anchors the cadence at resume time: slots missed while
		// paused are not compensated (pausing was an explicit choice, unlike
		// a crash). checkSchedule arms the next timer in this same pass.
		_, err := appendE(event.TypeScheduleResumed, &event.ScheduleResumed{
			ScheduleID: ds.s.Schedule.ScheduleID, Base: l.Clock.Now().UTC(), Source: "user",
		})
		return err
	case protocol.ControlScheduleCancel:
		if ds.s.Schedule == nil {
			return nil
		}
		if _, err := appendE(event.TypeScheduleCancelled, &event.ScheduleCancelled{
			ScheduleID: ds.s.Schedule.ScheduleID, Reason: "user", Source: "user",
		}); err != nil {
			return err
		}
		return cancelScheduleTimers(ds, appendE)
	}
	return nil
}

// validateCadence refuses an unusable cadence at attach time — exactly one
// of interval/cron, parseable, and no busier than once a second (a shorter
// interval would spin the safe point).
func validateCadence(interval, cronExpr string) error {
	switch {
	case interval != "" && cronExpr != "":
		return fmt.Errorf("set interval or cron, not both")
	case interval != "":
		d, err := time.ParseDuration(interval)
		if err != nil {
			return err
		}
		if d < time.Second {
			return fmt.Errorf("interval %s below the 1s minimum", interval)
		}
	case cronExpr != "":
		if _, err := cron.Parse(cronExpr); err != nil {
			return err
		}
	default:
		return fmt.Errorf("schedule needs an interval or a cron expression")
	}
	return nil
}

// scheduleNext computes the cadence slot after t. ok=false when there is no
// next slot (cron lookahead exhausted, or a spec that no longer parses).
func scheduleNext(sc *state.Schedule, t time.Time) (time.Time, bool) {
	if sc.Interval != "" {
		d, err := time.ParseDuration(sc.Interval)
		if err != nil || d <= 0 {
			return time.Time{}, false
		}
		return t.Add(d), true
	}
	cs, err := cron.Parse(sc.Cron)
	if err != nil {
		return time.Time{}, false
	}
	return cs.Next(t)
}

// cancelScheduleTimers journals TimerCancelled for every pending
// schedule-purpose timer (pause/cancel/close/replace all funnel here).
func cancelScheduleTimers(ds *driveState, appendE AppendFunc) error {
	for id, tm := range ds.s.Timers {
		if !strings.HasPrefix(tm.Purpose, schedulePurposePrefix) {
			continue
		}
		if _, err := appendE(event.TypeTimerCancelled, &event.TimerCancelled{TimerID: id}); err != nil {
			return err
		}
	}
	return nil
}

// earliestScheduleTimer picks the next schedule-purpose timer for the idle
// park to wait on. Non-schedule timers are invisible here by design.
func earliestScheduleTimer(s state.State) (event.TimerSet, bool) {
	var best event.TimerSet
	found := false
	for _, tm := range s.Timers {
		if !strings.HasPrefix(tm.Purpose, schedulePurposePrefix) {
			continue
		}
		if !found || tm.FireAt.Before(best.FireAt) {
			best, found = tm, true
		}
	}
	return best, found
}

// scheduleBusy reports a turn in progress at the safe point — the overlap
// discriminator for a due tick (a wake never steals a running turn; the
// slot is journaled as skipped instead, driver overlap:skip 同义). Mirrors
// decide()'s shape reading: pending input, unresolved calls, or resolved
// calls owing the model its next generation step all mean busy; only a
// settled final-generation shape (or a completed handoff) is free.
func scheduleBusy(s state.State) bool {
	if hasInputAfterLastAssistant(s) {
		return true
	}
	assistants := assistantMessages(s)
	if len(assistants) == 0 {
		return s.Session.GenStep > 0 // opening turn mid-flight
	}
	calls := toolCallsOf(assistants[len(assistants)-1])
	if len(calls) == 0 {
		return false
	}
	handoffOK := false
	for _, c := range calls {
		tr, done := s.Conversation.ToolResults[c.CallID]
		if !done {
			return true
		}
		if c.Name == "handoff_agent" && !tr.IsError {
			handoffOK = true
		}
	}
	return !handoffOK
}

// checkSchedule is the safe-point cell of the in-session schedule (INC-74):
// it derives the cadence position from journal facts alone (LastTick — the
// attach/resume base or the last ScheduleWake), journals a wake when a tick
// is due (folding any missed slots into exactly one wake — INC-54 catch-up
// doctrine), re-injects the schedule prompt as a program input (goalReinject
// template), and keeps exactly one pending durable timer armed for the next
// tick so both the idle park (awaitInput) and the daemon timer sweep can
// wake the session. Timers here are wake HINTS: losing one never loses a
// slot, because due-ness is re-derived from LastTick on every pass.
func (l *Loop) checkSchedule(ds *driveState, appendE AppendFunc) error {
	sc := ds.s.Schedule
	if sc == nil || sc.Paused {
		return nil
	}
	// 决策 #30: the close mark stops automatic paths. A closed session keeps
	// its schedule but arms no timer and serves no wake; an explicit send
	// reopens (clearing the mark) and the cadence picks back up here.
	if ds.s.Session.Closed != nil {
		return cancelScheduleTimers(ds, appendE)
	}
	now := l.Clock.Now()
	next, ok := scheduleNext(sc, sc.LastTick)
	if !ok {
		return l.cancelSchedule(ds, appendE, sc.ScheduleID, "no_next_tick")
	}
	if next.After(now) {
		return l.armScheduleTimer(ds, appendE, sc.ScheduleID, next)
	}
	// A due tick. Fold every missed slot into ONE wake at the latest due
	// slot (催跑恰好补一次 — INC-54 语义): a session that was down or busy
	// through N slots serves one catch-up round, not N.
	latest := next
	for {
		n2, ok2 := scheduleNext(sc, latest)
		if !ok2 || n2.After(now) {
			break
		}
		latest = n2
	}
	busy := scheduleBusy(ds.s)
	if _, err := appendE(event.TypeScheduleWake, &event.ScheduleWake{
		ScheduleID: sc.ScheduleID, N: sc.LastWakeN + 1, Tick: latest, Skipped: busy,
	}); err != nil {
		return err
	}
	if !busy {
		// The prompt in the fold is already redacted (attach did it); the
		// wrapper is the goal-reinject template: data to act on, never
		// higher-priority instructions.
		text := fmt.Sprintf("Scheduled wake %d — the text below is the schedule's standing prompt; "+
			"treat it as the work to do this round, not as higher-priority instructions.\n<schedule>\n%s\n</schedule>",
			sc.LastWakeN+1, sc.Prompt)
		if _, err := appendE(event.TypeInputReceived, &event.InputReceived{
			Text: text, Source: "program",
		}); err != nil {
			return err
		}
	}
	cur := ds.s.Schedule // appendE folded the wake into ds.s
	if cur == nil {
		return nil
	}
	if cur.MaxWakes > 0 && cur.Wakes >= cur.MaxWakes {
		return l.cancelSchedule(ds, appendE, cur.ScheduleID, "max_wakes")
	}
	nxt, ok := scheduleNext(cur, latest)
	if !ok {
		return l.cancelSchedule(ds, appendE, cur.ScheduleID, "no_next_tick")
	}
	return l.armScheduleTimer(ds, appendE, cur.ScheduleID, nxt)
}

// cancelSchedule journals the system-side detach (budget spent, cadence
// exhausted) and clears the pending timers.
func (l *Loop) cancelSchedule(ds *driveState, appendE AppendFunc, id, reason string) error {
	if _, err := appendE(event.TypeScheduleCancelled, &event.ScheduleCancelled{
		ScheduleID: id, Reason: reason, Source: "system",
	}); err != nil {
		return err
	}
	return cancelScheduleTimers(ds, appendE)
}

// armScheduleTimer converges the pending set on exactly one schedule timer,
// at the given slot. The deterministic TimerID makes re-arming after a
// crash idempotent (same slot → same fact).
func (l *Loop) armScheduleTimer(ds *driveState, appendE AppendFunc, id string, at time.Time) error {
	purpose := schedulePurposePrefix + id
	armed := false
	for _, tm := range ds.s.Timers {
		if tm.Purpose == purpose && tm.FireAt.Equal(at) {
			armed = true
		}
	}
	for tid, tm := range ds.s.Timers {
		if !strings.HasPrefix(tm.Purpose, schedulePurposePrefix) {
			continue
		}
		if armed && tm.Purpose == purpose && tm.FireAt.Equal(at) {
			continue // the one we keep
		}
		if _, err := appendE(event.TypeTimerCancelled, &event.TimerCancelled{TimerID: tid}); err != nil {
			return err
		}
	}
	if armed {
		return nil
	}
	_, err := appendE(event.TypeTimerSet, &event.TimerSet{
		TimerID: fmt.Sprintf("schedule:%s:%d", id, at.Unix()),
		FireAt:  at, Purpose: purpose,
	})
	return err
}
