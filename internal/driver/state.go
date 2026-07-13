package driver

import (
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
)

// FoldVersion is the driver fold's shape version, journaled in the stream
// header (DriverStarted) and checked on resume — a shape change refuses old
// streams instead of silently misreading them (S7 还债: version discipline,
// 与 run 的 SubStateVersions 同纪律). Streams predating the header (S6) are
// accepted as version 1.
const FoldVersion = 1

// Status is the driver's lifecycle position.
type Status string

const (
	StatusRunning Status = "running"
	StatusEnded   Status = "ended"
)

// Iteration is one folded iteration record.
type Iteration struct {
	N            int                    `json:"n"`
	ChildSession string                 `json:"child_session,omitempty"`
	ChildReason  string                 `json:"child_reason,omitempty"`
	Launched     bool                   `json:"launched,omitempty"`
	Completed    bool                   `json:"completed,omitempty"`
	Skipped      bool                   `json:"skipped,omitempty"`
	Verdict      event.IterationVerdict `json:"verdict,omitzero"`
	Usage        provider.Usage         `json:"usage,omitzero"`
	CarryRef     string                 `json:"carry_ref,omitempty"`
	// BaseRef is the parallel attempt's worktree base (S7 best-of-N,
	// additive — same fold version discipline as run sub-states).
	BaseRef  string    `json:"base_ref,omitempty"`
	Attempts []Attempt `json:"attempts,omitempty"`
}

// Attempt is one physical child run inside a logical iteration.
type Attempt struct {
	N            int            `json:"n"`
	ChildSession string         `json:"child_session,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Usage        provider.Usage `json:"usage,omitzero"`
	Error        string         `json:"error,omitempty"`
	Started      bool           `json:"started,omitempty"`
	Completed    bool           `json:"completed,omitempty"`
}

// State is the driver's pure fold — the sole working memory, rebuilt from the
// driver stream on resume exactly like the run fold. It is deliberately NOT a
// run sub-state (the driver stream is independent; DESIGN §运行形态).
type State struct {
	DriverID   string      `json:"driver_id,omitempty"`
	Status     Status      `json:"status"`
	Reason     string      `json:"reason,omitempty"`
	Iterations []Iteration `json:"iterations,omitempty"`
	// BestIter is the 1-based iteration with the highest verdict score so
	// far (ties keep the earliest); 0 means none completed yet.
	BestIter int `json:"best_iter,omitempty"`
	// SpentTokens is the settled tree spend: the sum of every completed
	// iteration's billed usage (DESIGN: settle-at-completion). Pure fold, so
	// resume recovers the exact budget position.
	SpentTokens int `json:"spent_tokens,omitempty"`
	// LastTick is the wall time of the latest consumed cron/loop slot (INC-54,
	// max over every IterationScheduled/IterationSkipped Tick). Resume seeds
	// the driver's in-memory `lastTick` from it so a cron series backfills the
	// slots missed while the daemon was down, exactly once per the overlap
	// policy. Zero for schedules without an absolute timeline.
	LastTick time.Time `json:"last_tick,omitzero"`
}

// Fold rebuilds driver state from its event stream.
func Fold(events []event.Envelope) (State, error) {
	s := &State{Status: StatusRunning}
	for _, e := range events {
		p, err := event.DecodePayload(e)
		if err != nil {
			return State{}, err
		}
		s.apply(p)
	}
	return *s, nil
}

// apply folds one decoded payload. Driver stream only; unrelated types (a
// child's own events never land here) are ignored. Iterations are 1-based; a
// malformed Iter < 1 is skipped rather than allowed to panic the fold.
func (s *State) apply(p any) {
	switch v := p.(type) {
	case *event.DriverStarted:
		if s.DriverID == "" {
			s.DriverID = v.DriverID
		}
	case *event.IterationScheduled:
		if v.Iter < 1 {
			return
		}
		s.ensure(v.Iter)
		if s.DriverID == "" {
			s.DriverID = v.DriverID
		}
		if v.BaseRef != "" {
			s.Iterations[v.Iter-1].BaseRef = v.BaseRef
		}
		if v.Tick.After(s.LastTick) {
			s.LastTick = v.Tick
		}
	case *event.IterationLaunched:
		if v.Iter < 1 {
			return
		}
		s.ensure(v.Iter)
		it := &s.Iterations[v.Iter-1]
		it.ChildSession = v.ChildSession
		it.Launched = true
	case *event.IterationAttemptStarted:
		if v.Iter < 1 || v.Attempt < 1 {
			return
		}
		s.ensure(v.Iter)
		it := &s.Iterations[v.Iter-1]
		ensureAttempt(it, v.Attempt)
		attempt := &it.Attempts[v.Attempt-1]
		attempt.ChildSession = v.ChildSession
		attempt.Started = true
	case *event.IterationAttemptCompleted:
		if v.Iter < 1 || v.Attempt < 1 {
			return
		}
		s.ensure(v.Iter)
		it := &s.Iterations[v.Iter-1]
		ensureAttempt(it, v.Attempt)
		attempt := &it.Attempts[v.Attempt-1]
		attempt.ChildSession = v.ChildSession
		attempt.Reason = v.Reason
		attempt.Usage = v.Usage
		attempt.Error = v.Error
		attempt.Started = true
		attempt.Completed = true
	case *event.IterationCompleted:
		if v.Iter < 1 {
			return
		}
		s.ensure(v.Iter)
		it := &s.Iterations[v.Iter-1]
		it.Completed = true
		it.ChildReason = v.ChildReason
		it.Verdict = v.Verdict
		it.Usage = v.Usage
		it.CarryRef = v.CarryRef
		if s.BestIter == 0 || v.Verdict.Score > s.Iterations[s.BestIter-1].Verdict.Score {
			s.BestIter = v.Iter
		}
		// Settle-at-completion: accumulate this iteration's billed spend into
		// the tree total (DESIGN: the driver is the tree budget root). One
		// IterationCompleted per iteration number, so no double count.
		s.SpentTokens += v.Usage.Billed()
	case *event.IterationSkipped:
		if v.Iter < 1 {
			return
		}
		s.ensure(v.Iter)
		s.Iterations[v.Iter-1].Skipped = true
		if v.Tick.After(s.LastTick) {
			s.LastTick = v.Tick
		}
	case *event.DriverCompleted:
		s.Status = StatusEnded
		s.Reason = v.Reason
		if v.BestIter > 0 {
			s.BestIter = v.BestIter
		}
	}
}

// ensure grows the dense iteration slice so index n-1 exists.
func (s *State) ensure(n int) {
	for len(s.Iterations) < n {
		s.Iterations = append(s.Iterations, Iteration{N: len(s.Iterations) + 1})
	}
}

// at returns iteration n (1-based) by value and whether it is in the fold —
// the resume cursor consults it to avoid re-journaling facts already durable.
func (s *State) at(n int) (Iteration, bool) {
	if n < 1 || n > len(s.Iterations) {
		return Iteration{}, false
	}
	return s.Iterations[n-1], true
}

func ensureAttempt(it *Iteration, n int) {
	for len(it.Attempts) < n {
		it.Attempts = append(it.Attempts, Attempt{N: len(it.Attempts) + 1})
	}
}

func (s *State) attempt(iter, attempt int) (Attempt, bool) {
	it, ok := s.at(iter)
	if !ok || attempt < 1 || attempt > len(it.Attempts) {
		return Attempt{}, false
	}
	return it.Attempts[attempt-1], true
}

// lastCompleted returns the highest-numbered completed iteration and whether
// one exists — the resume anchor for re-deriving an already-decided terminal.
func (s *State) lastCompleted() (Iteration, bool) {
	for i := len(s.Iterations) - 1; i >= 0; i-- {
		if s.Iterations[i].Completed {
			return s.Iterations[i], true
		}
	}
	return Iteration{}, false
}
