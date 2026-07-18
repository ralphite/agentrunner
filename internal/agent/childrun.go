package agent

import (
	"context"

	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// childRun owns one child-session attempt directory (INC-76, E1②): the
// store handle, the three-way run decision, and the settled spend. It is
// the single implementation of "run a child session to quiescence" — the
// spawn paths, the approval-wait reattach, and driver.runIteration all
// drive their children through it instead of hand-rolling the discipline.
//
// The Loop itself is built by the CALLER and passed in: construction reads
// parent state (pipeline gates, mode, allowance) and must happen on the
// caller's goroutine; only the run is background-safe.
type ChildRun struct {
	dir string
	cs  *store.EventStore
}

// OpenChildRun opens the attempt's store. Callers keep their existing
// open-position in the sequence (before any journal fact referencing the
// child) so failure shapes are unchanged.
func OpenChildRun(dir string) (*ChildRun, error) {
	cs, err := store.OpenEventStore(dir)
	if err != nil {
		return nil, err
	}
	return &ChildRun{dir: dir, cs: cs}, nil
}

func (c *ChildRun) Store() *store.EventStore { return c.cs }
func (c *ChildRun) Close()                   { _ = c.cs.Close() }

// SettledChild reports whether the child journal at dir is already
// QUIESCENT, and if so its result folded from that shape (决策 #31 — no
// receipt event exists; the shape is the truth). Exposed for callers whose
// settled-path handling differs from a live run's (driver retry
// classification) — the substrate's Run uses the same check internally.
func SettledChild(dir string) (bool, RunResult) {
	events, err := store.ReadEvents(dir)
	if err != nil {
		return false, RunResult{}
	}
	s, err := state.Fold(events)
	if err != nil {
		return false, RunResult{}
	}
	quiescent, reason := state.Quiescence(s)
	if !quiescent {
		return false, RunResult{}
	}
	return true, RunResult{Reason: reason, GenSteps: s.Session.GenStep, Usage: s.Session.Usage}
}

func (c *ChildRun) settled() (bool, RunResult) { return SettledChild(c.dir) }

// run drives the attempt to quiescence with the three-way decision:
//
//   - journal already quiescent → settle from the shape, NEVER re-run
//     (决策 #29 单一自愈: a completed child re-run would duplicate its
//     side effects);
//   - journal non-empty but not quiescent → Resume (the child's own
//     in-doubt discipline guards correctness);
//   - fresh journal → Run(prompt).
//
// spent is always read from the child fold — the truth even when
// RunResult carries zero on an abort (S5/S6 review: an unsettled failed
// child would let a re-spawn over-grant against the tree cap). It is the
// CUMULATIVE fold usage; callers that need a delta (revive baseline)
// subtract at their site.
func (c *ChildRun) Run(ctx context.Context, child *Loop, prompt string) (RunResult, provider.Usage, error) {
	if c.cs.LastSeq() > 0 {
		if done, sres := c.settled(); done {
			return sres, sres.Usage, nil
		}
		res, err := child.Resume(ctx)
		return res, childFoldUsage(c.dir), err
	}
	res, err := child.Run(ctx, prompt)
	return res, childFoldUsage(c.dir), err
}
