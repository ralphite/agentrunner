package state

import (
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
)

// INC-77.1: the Series sub-state's fold — start, settled/skipped
// iterations (best-iter ties keep the earliest, spend sums billed usage,
// LastTick advances), duplicate-N replay overwrites in place, wrong-ID
// no-op, terminal — all copy-on-write.
func TestSeriesFoldLifecycle(t *testing.T) {
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
	s = apply(s, event.TypeSeriesStarted, &event.SeriesStarted{
		SeriesID: "ser-1", Kind: "goal", MaxIterations: 5, Patience: 2, Source: "user"})
	if s.Series == nil || s.Series.Kind != "goal" || s.Series.MaxIterations != 5 {
		t.Fatalf("after start: %+v", s.Series)
	}

	tick1 := time.Date(2026, 7, 18, 14, 0, 0, 0, time.UTC)
	prev := s
	s = apply(s, event.TypeSeriesIteration, &event.SeriesIteration{
		SeriesID: "ser-1", N: 1, CallID: "c1", Reason: "completed",
		Verdict: event.IterationVerdict{Pass: false, Score: 0.5},
		Tick:    tick1, Usage: provider.Usage{InputTokens: 100, OutputTokens: 50}})
	if len(s.Series.Iterations) != 1 || s.Series.BestIter != 1 ||
		s.Series.SpentTokens != 150 || !s.Series.LastTick.Equal(tick1) {
		t.Fatalf("after iter 1: %+v", s.Series)
	}
	if len(prev.Series.Iterations) != 0 {
		t.Fatal("Apply mutated the prior state's Series (copy-on-write violated)")
	}

	// Equal score: ties keep the EARLIEST iteration as best.
	s = apply(s, event.TypeSeriesIteration, &event.SeriesIteration{
		SeriesID: "ser-1", N: 2, CallID: "c2", Reason: "completed",
		Verdict: event.IterationVerdict{Pass: false, Score: 0.5},
		Usage:   provider.Usage{InputTokens: 40, OutputTokens: 10}})
	if s.Series.BestIter != 1 || s.Series.SpentTokens != 200 {
		t.Fatalf("tie handling: %+v", s.Series)
	}

	// A skipped slot advances LastTick but neither spend nor best.
	tick3 := tick1.Add(time.Hour)
	s = apply(s, event.TypeSeriesIteration, &event.SeriesIteration{
		SeriesID: "ser-1", N: 3, Skipped: true, Tick: tick3})
	if s.Series.SpentTokens != 200 || s.Series.BestIter != 1 || !s.Series.LastTick.Equal(tick3) {
		t.Fatalf("after skip: %+v", s.Series)
	}

	// Higher score takes best; duplicate-N replay overwrites in place.
	s = apply(s, event.TypeSeriesIteration, &event.SeriesIteration{
		SeriesID: "ser-1", N: 4, Reason: "completed",
		Verdict: event.IterationVerdict{Pass: true, Score: 0.9},
		Usage:   provider.Usage{InputTokens: 10}})
	if s.Series.BestIter != 4 || len(s.Series.Iterations) != 4 {
		t.Fatalf("after iter 4: %+v", s.Series)
	}
	s = apply(s, event.TypeSeriesIteration, &event.SeriesIteration{
		SeriesID: "ser-1", N: 4, Reason: "completed",
		Verdict: event.IterationVerdict{Pass: true, Score: 0.9},
		Usage:   provider.Usage{InputTokens: 10}})
	if len(s.Series.Iterations) != 4 {
		t.Fatalf("duplicate N forked the list: %+v", s.Series.Iterations)
	}

	// Pause/resume is change-as-event. Resume's explicit Base replaces the
	// cadence anchor, discarding slots elapsed during the pause.
	beforePause := s
	s = apply(s, event.TypeSeriesPaused, &event.SeriesPaused{SeriesID: "ser-1", Source: "user"})
	if !s.Series.Paused || beforePause.Series.Paused {
		t.Fatalf("pause fold/copy-on-write: before=%+v after=%+v", beforePause.Series, s.Series)
	}
	resumeBase := tick3.Add(3 * time.Hour)
	s = apply(s, event.TypeSeriesResumed, &event.SeriesResumed{
		SeriesID: "ser-1", Base: resumeBase, Source: "user"})
	if s.Series.Paused || !s.Series.LastTick.Equal(resumeBase) {
		t.Fatalf("resume did not re-anchor: %+v", s.Series)
	}

	// Wrong-ID facts are stale-control no-ops.
	s = apply(s, event.TypeSeriesPaused, &event.SeriesPaused{SeriesID: "other", Source: "user"})
	if s.Series.Paused {
		t.Fatal("wrong-id pause changed the series")
	}
	s = apply(s, event.TypeSeriesEnded, &event.SeriesEnded{SeriesID: "other", Reason: "stopped"})
	if s.Series.Ended {
		t.Fatal("wrong-id terminal ended the series")
	}
	s = apply(s, event.TypeSeriesEnded, &event.SeriesEnded{
		SeriesID: "ser-1", Reason: "goal_satisfied", Iterations: 4, BestIter: 4})
	if !s.Series.Ended || s.Series.EndReason != "goal_satisfied" {
		t.Fatalf("after end: %+v", s.Series)
	}
}
