package driver

import "github.com/ralphite/agentrunner/internal/event"

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
	Verdict      event.IterationVerdict `json:"verdict,omitzero"`
	CarryRef     string                 `json:"carry_ref,omitempty"`
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
	case *event.IterationScheduled:
		if v.Iter < 1 {
			return
		}
		s.ensure(v.Iter)
		if s.DriverID == "" {
			s.DriverID = v.DriverID
		}
	case *event.IterationLaunched:
		if v.Iter < 1 {
			return
		}
		s.ensure(v.Iter)
		it := &s.Iterations[v.Iter-1]
		it.ChildSession = v.ChildSession
		it.Launched = true
	case *event.IterationCompleted:
		if v.Iter < 1 {
			return
		}
		s.ensure(v.Iter)
		it := &s.Iterations[v.Iter-1]
		it.Completed = true
		it.ChildReason = v.ChildReason
		it.Verdict = v.Verdict
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
