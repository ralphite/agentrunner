package state

import (
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
)

// INC-74.1: the Schedule sub-state's change-as-event lifecycle — attach,
// wake (served and skipped), pause/resume, cancel — folds copy-on-write.
func TestScheduleFoldLifecycle(t *testing.T) {
	apply := func(s State, typ string, p any) State {
		t.Helper()
		env, err := event.New(typ, p)
		if err != nil {
			t.Fatal(err)
		}
		next, err := Apply(s, env)
		if err != nil {
			t.Fatal(err)
		}
		return next
	}
	s := New()
	base := time.Date(2026, 7, 18, 5, 30, 0, 0, time.UTC)
	s = apply(s, event.TypeScheduleAttached, &event.ScheduleAttached{
		ScheduleID: "sch-1", Interval: "30m", Prompt: "巡检", MaxWakes: 5,
		Base: base, Source: "user"})
	if s.Schedule == nil || s.Schedule.Interval != "30m" || s.Schedule.Wakes != 0 ||
		!s.Schedule.LastTick.Equal(base) {
		t.Fatalf("after attach: %+v", s.Schedule)
	}

	tick1 := time.Date(2026, 7, 18, 6, 0, 0, 0, time.UTC)
	prev := s
	s = apply(s, event.TypeScheduleWake, &event.ScheduleWake{ScheduleID: "sch-1", N: 1, Tick: tick1})
	if s.Schedule.Wakes != 1 || s.Schedule.LastWakeN != 1 || !s.Schedule.LastTick.Equal(tick1) {
		t.Fatalf("after wake 1: %+v", s.Schedule)
	}
	if prev.Schedule.Wakes != 0 {
		t.Fatal("Apply mutated the prior state's Schedule (copy-on-write violated)")
	}

	// A skipped wake advances the cadence position but not the served count.
	tick2 := tick1.Add(30 * time.Minute)
	s = apply(s, event.TypeScheduleWake, &event.ScheduleWake{ScheduleID: "sch-1", N: 2, Tick: tick2, Skipped: true})
	if s.Schedule.Wakes != 1 || s.Schedule.LastWakeN != 2 || !s.Schedule.LastTick.Equal(tick2) {
		t.Fatalf("after skipped wake: %+v", s.Schedule)
	}

	s = apply(s, event.TypeSchedulePaused, &event.SchedulePaused{ScheduleID: "sch-1", Source: "user"})
	if !s.Schedule.Paused {
		t.Fatal("pause did not set Paused")
	}
	// Resume re-anchors LastTick to its Base: slots missed while paused are
	// not compensated (pausing was an explicit choice).
	rebase := tick2.Add(3 * time.Hour)
	s = apply(s, event.TypeScheduleResumed, &event.ScheduleResumed{
		ScheduleID: "sch-1", Base: rebase, Source: "user"})
	if s.Schedule.Paused || !s.Schedule.LastTick.Equal(rebase) {
		t.Fatalf("after resume: %+v", s.Schedule)
	}

	// Events for a different schedule id are ignored (stale control no-op).
	s = apply(s, event.TypeScheduleCancelled, &event.ScheduleCancelled{ScheduleID: "other", Source: "user"})
	if s.Schedule == nil {
		t.Fatal("cancel for a different id cleared the schedule")
	}
	s = apply(s, event.TypeScheduleCancelled, &event.ScheduleCancelled{ScheduleID: "sch-1", Source: "user"})
	if s.Schedule != nil {
		t.Fatal("cancel did not clear the schedule")
	}
}
