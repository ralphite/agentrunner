package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

var scheduleT0 = time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)

// scheduleLoop is controlLoop plus the fake clock and a cancelable context —
// the INC-74 twins advance time and simulate crashes.
func scheduleLoop(t *testing.T, fix scripted.Fixture, dir string) (*store.EventStore,
	chan protocol.UserInput, chan protocol.Control, *clock.Fake, context.CancelFunc, chan error) {
	t.Helper()
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })

	inbox := make(chan protocol.UserInput, 4)
	controls := make(chan protocol.Control, 4)
	clk := clock.NewFake(scheduleT0)
	l := &Loop{
		Spec: &AgentSpec{
			Name:               "sched",
			Model:              ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			Tools:              []string{"bash"},
			MaxGenerationSteps: 10,
		},
		Provider:   scripted.New(fix),
		Exec:       &tool.Executor{WS: ws},
		Store:      es,
		Clock:      clk,
		SessionID:  "sched-sess",
		UserInputs: inbox,
		Controls:   controls,
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { _, e := l.Run(ctx, "first question"); done <- e }()
	return es, inbox, controls, clk, cancel, done
}

func scheduleAttach(id, interval, prompt string, maxWakes int) protocol.Control {
	return protocol.Control{Kind: protocol.ControlScheduleAttach, Schedule: &protocol.ScheduleControl{
		ScheduleID: id, Interval: interval, Prompt: prompt, MaxWakes: maxWakes,
	}}
}

func schedStep(text string) scripted.Step {
	return scripted.Step{Respond: []scripted.Event{{Text: text}, {Finish: "end_turn"}}}
}

// Attach arms a durable timer; when it fires at the idle, the loop journals
// the wake fact, re-injects the schedule prompt as a program input, runs a
// normal turn, and arms the next tick (INC-74 the core cycle).
func TestScheduleAttachWakesAndReinjects(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		schedStep("answer one"), schedStep("round done"),
		schedStep("filler a"), schedStep("filler b"), // tolerance for extra turns
	}}
	es, inbox, controls, clk, _, done := scheduleLoop(t, fix, filepath.Join(t.TempDir(), "sess"))

	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- scheduleAttach("sch-1", "30m", "巡检一轮构建状态", 0)
	waitForEvent(t, es, event.TypeTimerSet, 1)

	clk.Advance(31 * time.Minute)
	waitForEvent(t, es, event.TypeScheduleWake, 1)
	waitForEvent(t, es, event.TypeAssistantMessage, 2) // the wake ran a real turn
	waitForEvent(t, es, event.TypeTimerSet, 2)         // next tick armed after the wake

	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	evs, _ := store.ReadEvents(es.Dir())
	var wake *event.ScheduleWake
	var sawReinject bool
	fired := 0
	for _, e := range evs {
		switch e.Type {
		case event.TypeScheduleWake:
			dec, _ := event.DecodePayload(e)
			wake = dec.(*event.ScheduleWake)
		case event.TypeInputReceived:
			dec, _ := event.DecodePayload(e)
			in := dec.(*event.InputReceived)
			if in.Source == "program" && strings.Contains(in.Text, "<schedule>") {
				sawReinject = true
			}
		case event.TypeTimerFired:
			fired++
		}
	}
	if wake == nil || wake.Skipped || wake.N != 1 || !wake.Tick.Equal(scheduleT0.Add(30*time.Minute)) {
		t.Fatalf("wake fact = %+v, want served N=1 at t0+30m", wake)
	}
	if !sawReinject {
		t.Fatal("no program input carrying the schedule prompt was injected")
	}
	if fired != 1 {
		t.Fatalf("timer_fired = %d, want exactly 1 (the idle park fired the armed timer)", fired)
	}
}

// Pause cancels the pending timer (no wakes while paused); resume re-anchors
// the cadence at resume time — slots missed while paused are NOT compensated.
func TestSchedulePauseSkipsWakeAndResumeRebases(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{schedStep("answer one")}}
	es, inbox, controls, clk, _, done := scheduleLoop(t, fix, filepath.Join(t.TempDir(), "sess"))

	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- scheduleAttach("sch-1", "30m", "巡检", 0)
	waitForEvent(t, es, event.TypeTimerSet, 1)

	controls <- protocol.Control{Kind: protocol.ControlSchedulePause}
	waitForEvent(t, es, event.TypeSchedulePaused, 1)
	waitForEvent(t, es, event.TypeTimerCancelled, 1)

	clk.Advance(2 * time.Hour) // four slots pass while paused — none may wake

	controls <- protocol.Control{Kind: protocol.ControlScheduleResume}
	waitForEvent(t, es, event.TypeScheduleResumed, 1)
	waitForEvent(t, es, event.TypeTimerSet, 2)

	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if n := countEvents(t, es.Dir(), event.TypeScheduleWake); n != 0 {
		t.Fatalf("schedule_wake = %d, want 0 (paused slots are not compensated)", n)
	}
	evs, _ := store.ReadEvents(es.Dir())
	var lastTimer *event.TimerSet
	for _, e := range evs {
		if e.Type == event.TypeTimerSet {
			dec, _ := event.DecodePayload(e)
			lastTimer = dec.(*event.TimerSet)
		}
	}
	want := scheduleT0.Add(2*time.Hour + 30*time.Minute)
	if lastTimer == nil || !lastTimer.FireAt.Equal(want) {
		t.Fatalf("re-armed timer = %+v, want fire_at %s (resume re-anchors)", lastTimer, want)
	}
}

// Cancel clears the schedule and its pending timers; later turns run with no
// schedule machinery left behind.
func TestScheduleCancelStopsTimers(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{schedStep("answer one"), schedStep("answer two")}}
	es, inbox, controls, clk, _, done := scheduleLoop(t, fix, filepath.Join(t.TempDir(), "sess"))

	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- scheduleAttach("sch-1", "30m", "巡检", 0)
	waitForEvent(t, es, event.TypeTimerSet, 1)
	controls <- protocol.Control{Kind: protocol.ControlScheduleCancel}
	waitForEvent(t, es, event.TypeScheduleCancelled, 1)
	waitForEvent(t, es, event.TypeTimerCancelled, 1)

	clk.Advance(2 * time.Hour)
	inbox <- protocol.UserInput{Text: "again"}
	waitForEvent(t, es, event.TypeAssistantMessage, 2)

	close(inbox)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if n := countEvents(t, es.Dir(), event.TypeScheduleWake); n != 0 {
		t.Fatalf("schedule_wake = %d, want 0 after cancel", n)
	}
	if n := countEvents(t, es.Dir(), event.TypeTimerSet); n != 1 {
		t.Fatalf("timer_set = %d, want 1 (nothing re-armed after cancel)", n)
	}
	evs, _ := store.ReadEvents(es.Dir())
	s, err := state.Fold(evs)
	if err != nil {
		t.Fatal(err)
	}
	if s.Schedule != nil || len(s.Timers) != 0 {
		t.Fatalf("fold after cancel: schedule=%+v timers=%v, want both empty", s.Schedule, s.Timers)
	}
}

// A due tick on a busy session journals a skipped wake — never a mid-turn
// prompt injection — and still folds all missed slots into that one fact
// before arming the next tick (driver overlap:skip 同义).
func TestScheduleOverlapSkipsBusySession(t *testing.T) {
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()
	// now = t0+65m: slots t0+30m and t0+60m are both due; latest wins.
	l := &Loop{Clock: clock.NewFake(scheduleT0.Add(65 * time.Minute)), Store: es, SessionID: "busy"}
	ds := &driveState{s: state.New()}
	appendE := l.appender(ds)
	if _, err := appendE(event.TypeScheduleAttached, &event.ScheduleAttached{
		ScheduleID: "sch-1", Interval: "30m", Prompt: "round", Base: scheduleT0, Source: "user",
	}); err != nil {
		t.Fatal(err)
	}
	// A pending user input the model has not seen = a turn in progress.
	if _, err := appendE(event.TypeInputReceived, &event.InputReceived{
		Text: "working...", Source: "user",
	}); err != nil {
		t.Fatal(err)
	}

	if err := l.checkSchedule(ds, appendE); err != nil {
		t.Fatal(err)
	}

	evs, _ := store.ReadEvents(es.Dir())
	var wake *event.ScheduleWake
	var timer *event.TimerSet
	inputs := 0
	for _, e := range evs {
		switch e.Type {
		case event.TypeScheduleWake:
			dec, _ := event.DecodePayload(e)
			wake = dec.(*event.ScheduleWake)
		case event.TypeTimerSet:
			dec, _ := event.DecodePayload(e)
			timer = dec.(*event.TimerSet)
		case event.TypeInputReceived:
			inputs++
		}
	}
	if wake == nil || !wake.Skipped || !wake.Tick.Equal(scheduleT0.Add(60*time.Minute)) {
		t.Fatalf("wake = %+v, want skipped at t0+60m (missed slots folded)", wake)
	}
	if inputs != 1 {
		t.Fatalf("inputs = %d, want 1 (a skipped wake must NOT inject a prompt)", inputs)
	}
	if timer == nil || !timer.FireAt.Equal(scheduleT0.Add(90*time.Minute)) {
		t.Fatalf("next timer = %+v, want armed at t0+90m", timer)
	}

	// A second pass at the same clock is a no-op: the slot was served
	// (skipped) and the next timer is already armed — idempotent re-arming.
	if err := l.checkSchedule(ds, appendE); err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, es.Dir(), event.TypeScheduleWake); n != 1 {
		t.Fatalf("second pass journaled another wake (%d), want 1", n)
	}
	if n := countEvents(t, es.Dir(), event.TypeTimerSet); n != 1 {
		t.Fatalf("second pass re-armed the timer (%d), want 1", n)
	}
}

// A crash with an armed timer resumes cleanly: FirePendingTimers fires the
// expired timer, the safe point re-derives due-ness from LastTick, and ALL
// slots missed while down fold into exactly one catch-up wake (INC-54 教义).
func TestScheduleSurvivesRestartServesMissedSlotOnce(t *testing.T) {
	sessDir := filepath.Join(t.TempDir(), "sess")
	root := t.TempDir()

	// Phase 1: attach, arm, then crash (context cancel).
	ws1, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	es1, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	inbox1 := make(chan protocol.UserInput, 1)
	controls1 := make(chan protocol.Control, 1)
	spec := &AgentSpec{
		Name:               "sched",
		Model:              ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
		Tools:              []string{"bash"},
		MaxGenerationSteps: 10,
	}
	l1 := &Loop{
		Spec:     spec,
		Provider: scripted.New(scripted.Fixture{Steps: []scripted.Step{schedStep("answer one")}}),
		Exec:     &tool.Executor{WS: ws1}, Store: es1,
		Clock: clock.NewFake(scheduleT0), SessionID: "sched-restart",
		UserInputs: inbox1, Controls: controls1,
	}
	ctx1, cancel1 := context.WithCancel(context.Background())
	done1 := make(chan error, 1)
	go func() { _, e := l1.Run(ctx1, "first question"); done1 <- e }()
	waitForEvent(t, es1, event.TypeAssistantMessage, 1)
	controls1 <- scheduleAttach("sch-1", "30m", "巡检", 0)
	waitForEvent(t, es1, event.TypeTimerSet, 1)
	cancel1()
	<-done1 // crash-like end; the cancel cause is expected, not asserted
	_ = es1.Close()

	// Phase 2: resume 95 minutes later — slots 30/60/90 were all missed.
	ws2, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	es2, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es2.Close() }()
	inbox2 := make(chan protocol.UserInput, 1)
	l2 := &Loop{
		Spec:     spec,
		Provider: scripted.New(scripted.Fixture{Steps: []scripted.Step{schedStep("catch-up round"), schedStep("filler")}}),
		Exec:     &tool.Executor{WS: ws2}, Store: es2,
		Clock: clock.NewFake(scheduleT0.Add(95 * time.Minute)), SessionID: "sched-restart",
		UserInputs: inbox2, Controls: make(chan protocol.Control),
	}
	done2 := make(chan error, 1)
	go func() { _, e := l2.Resume(context.Background()); done2 <- e }()
	waitForEvent(t, es2, event.TypeScheduleWake, 1)
	waitForEvent(t, es2, event.TypeAssistantMessage, 2) // the catch-up turn ran

	close(inbox2)
	if err := <-done2; err != nil {
		t.Fatal(err)
	}

	evs, _ := store.ReadEvents(sessDir)
	var wakes []*event.ScheduleWake
	var lastTimer *event.TimerSet
	for _, e := range evs {
		switch e.Type {
		case event.TypeScheduleWake:
			dec, _ := event.DecodePayload(e)
			wakes = append(wakes, dec.(*event.ScheduleWake))
		case event.TypeTimerSet:
			dec, _ := event.DecodePayload(e)
			lastTimer = dec.(*event.TimerSet)
		}
	}
	if len(wakes) != 1 || wakes[0].Skipped || !wakes[0].Tick.Equal(scheduleT0.Add(90*time.Minute)) {
		t.Fatalf("wakes = %+v, want exactly one served catch-up at t0+90m", wakes)
	}
	if lastTimer == nil || !lastTimer.FireAt.Equal(scheduleT0.Add(120*time.Minute)) {
		t.Fatalf("next timer = %+v, want re-armed at t0+120m", lastTimer)
	}
}

// Close cancels the schedule's pending timers (决策 #30: the mark stops
// automatic paths) so the closed session reaches quiescence; the schedule
// itself survives the mark for a later explicit reopen.
func TestScheduleCloseCancelsTimersReachesQuiescence(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{schedStep("answer one")}}
	es, _, controls, _, _, done := scheduleLoop(t, fix, filepath.Join(t.TempDir(), "sess"))

	waitForEvent(t, es, event.TypeAssistantMessage, 1)
	controls <- scheduleAttach("sch-1", "30m", "巡检", 0)
	waitForEvent(t, es, event.TypeTimerSet, 1)
	controls <- protocol.Control{Kind: protocol.ControlClose}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	evs, _ := store.ReadEvents(es.Dir())
	s, err := state.Fold(evs)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Timers) != 0 {
		t.Fatalf("pending timers after close = %v, want none", s.Timers)
	}
	if s.Schedule == nil {
		t.Fatal("the schedule itself must survive the close mark (reopen re-arms)")
	}
	quiescent, reason := state.Quiescence(s)
	if !quiescent || reason != "closed" {
		t.Fatalf("quiescence = %v/%q, want true/closed", quiescent, reason)
	}
	if n := countEvents(t, es.Dir(), event.TypeScheduleWake); n != 0 {
		t.Fatalf("schedule_wake = %d, want 0", n)
	}
}
