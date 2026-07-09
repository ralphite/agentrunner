// Quiescent-child revive (INC-12, DESIGN §3 静止子唤醒): a team member that
// went quiescent is woken by tree mail. The DIRECT PARENT owns the re-host:
// it journals ChildRevived (the fold re-enters the handle through a synthetic
// background activity), resumes the child loop in place — same journal, same
// context — and the child's next quiescence settles through the ordinary
// background path with a SECOND SubagentCompleted. Marks are honored: a
// user-killed child revives only for user mail; a parent-killed one revives
// for tree mail too (裁决二 C — the parent process executes the revive).
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// childSegRe splits "<call>-a<attempt>" (one path segment under sub/).
var childSegRe = regexp.MustCompile(`^(.+)-a(\d+)$`)

// drainRevives handles every queued revive request without blocking.
// Drive-goroutine only (safe point / idle), like drainBackground.
func (l *Loop) drainRevives(ctx context.Context, ds *driveState, appendE AppendFunc) error {
	if l.revive == nil {
		return nil
	}
	for {
		select {
		case sid := <-l.revive:
			if err := l.reviveChild(ctx, ds, appendE, sid); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

// scanPendingChildMail enqueues a revive request for every DIRECT child
// whose inbox holds unconsumed mail (crash recovery / restart continuation:
// the durable truth outlives every dropped wake). Runs once as the drive
// starts; deeper descendants are found by the intermediate parent when its
// own resume runs this same scan.
func (l *Loop) scanPendingChildMail() {
	if l.Router == nil || l.revive == nil {
		return
	}
	subs, err := filepath.Glob(filepath.Join(l.Store.Dir(), "sub", "*"))
	if err != nil {
		return
	}
	for _, dir := range subs {
		sid := l.SessionID + "-sub-" + filepath.Base(dir)
		if !l.childHasMail(sid) {
			continue
		}
		select {
		case l.revive <- sid:
		default:
			slog.Warn("revive scan: queue full, child mail deferred to next resume", "child", sid)
		}
	}
}

// childHasMail reports unconsumed inbox entries for a tree member, folding
// its journal for the consumed high-water mark.
func (l *Loop) childHasMail(sid string) bool {
	if l.Router == nil {
		return false
	}
	dir, err := l.Router.DirOf(sid)
	if err != nil {
		return false
	}
	st, err := childFoldState(dir)
	if err != nil {
		return false
	}
	return l.Router.PendingMail(sid, st.Session.ConsumedInputSeq)
}

// reviveChild re-hosts one quiescent child. sid may name a deeper
// descendant — the revive walks the first hop under THIS session and lets
// the resumed intermediate parent's own scan carry it further. Every skip
// is deliberate and logged; genuine journal errors surface.
func (l *Loop) reviveChild(ctx context.Context, ds *driveState, appendE AppendFunc, sid string) error {
	child := firstHopChild(l.SessionID, sid)
	if child == "" {
		return nil // not one of ours
	}
	seg := strings.TrimPrefix(child, l.SessionID+"-sub-")
	m := childSegRe.FindStringSubmatch(seg)
	if m == nil || !safeCallIDRe.MatchString(m[1]) {
		return nil
	}
	callID := m[1]
	if _, inflight := ds.s.Handles[callID]; inflight {
		// Already running: the live wake (or its own resume replay) delivers.
		return nil
	}
	dir, err := l.Router.DirOf(child)
	if err != nil {
		return nil
	}
	st, err := childFoldState(dir)
	if err != nil {
		return fmt.Errorf("revive %s: child fold: %w", child, err)
	}
	tail, err := store.ReadInbox(dir, st.Session.ConsumedInputSeq)
	if err != nil || len(tail) == 0 {
		return nil // consumed meanwhile (or unreadable): nothing to do
	}

	// Marks gate the automatic path (决策 #30): user mail overrides every
	// mark (send-as-explicit-gesture); tree mail may revive a parent-killed
	// child (the parent executes this) but never a user-killed one.
	if mark := st.Session.Closed; mark != nil && mark.Source == "user" {
		hasUserMail := false
		for _, in := range tail {
			if in.Source == "" || in.Source == "user" {
				hasUserMail = true
				break
			}
		}
		if !hasUserMail {
			slog.Warn("revive skipped: user-killed child only revives for user mail",
				"child", child)
			return nil
		}
	}

	// Budget: the revive allowance tops the child's frozen cap up by what
	// the parent can still afford — min(parent remaining, original cap) over
	// the child's settled spend. Nothing left → the parent model hears it
	// (program source, INC-D1 family) and decides; the mail stays durable.
	spent := st.Session.Usage.Billed()
	cap := 0
	spec, specErr := childSpecFromJournal(dir)
	if specErr != nil {
		return fmt.Errorf("revive %s: %w", child, specErr)
	}
	if spec.Budget.MaxTotalTokens > 0 {
		cap = spec.Budget.MaxTotalTokens
	}
	allowance := l.reviveAllowance(ds.s, cap, spent)
	if allowance <= 0 {
		_, err := appendE(event.TypeInputReceived, &event.InputReceived{
			Text:   fmt.Sprintf("[revive of %s skipped: its token budget is exhausted; unread mail stays in its inbox]", child),
			Source: "program",
		})
		return err
	}

	if m[2] != "1" {
		// The synthetic activity records attempt 1 (the background-spawn
		// invariant settle-from-child-fold relies on); a handoff child (a2+)
		// is not revivable today — documented degraded mode.
		slog.Warn("revive skipped: non-attempt-1 child (handoff lineage)", "child", child)
		return nil
	}

	// Resources BEFORE the fact (deadlock guard): the store must be open
	// before ChildRevived lands — a journaled revive with no goroutine to
	// settle it would wedge every later abort drain. A transiently locked
	// child store (another opener racing) just defers: the mail is durable,
	// the next scan retries.
	childStore, err := store.OpenEventStore(dir)
	if err != nil {
		slog.Warn("revive deferred: child store unavailable", "child", child, "err", err)
		return nil
	}

	activityID := "revive-" + event.NewCommandID()
	baseline := st.Session.Usage
	if _, err := appendE(event.TypeChildRevived, &event.ChildRevived{
		CallID: callID, ActivityID: activityID, Agent: spec.Name,
		ChildSession: child, Reason: "message",
		BudgetTokens: allowance, BaselineUsage: baseline,
	}); err != nil {
		_ = childStore.Close()
		return err
	}
	// The child works under a cap covering its WHOLE journal: settled spend
	// plus this revive's allowance (its budget gate compares total usage).
	// Both sides unlimited → no cap (childLoop skips the gate on 0).
	frozenCap := 0
	if cap > 0 || l.Spec.Budget.MaxTotalTokens > 0 {
		frozenCap = spent + allowance
	}
	loop := l.childLoop(spec, childStore, child, frozenCap, ds.s.CurrentMode())

	l.ensureBackground()
	taskCtx, cancel := context.WithCancelCause(ctx)
	l.bg.mu.Lock()
	l.bg.cancel[callID] = cancel
	l.bg.mu.Unlock()

	agentName := spec.Name
	go func() {
		defer func() { _ = childStore.Close() }()
		cres, cerr := loop.Resume(taskCtx)
		after, aerr := childFoldState(dir)
		var total provider.Usage
		if aerr == nil {
			total = after.Session.Usage
		} else {
			total = baseline
		}
		delta := subUsage(total, baseline)
		reason := cres.Reason
		canceled := taskCtx.Err() != nil
		if cerr != nil && reason == "" {
			reason = "error"
		}
		payload, _ := json.Marshal(map[string]any{
			"agent": agentName, "child_session": child,
			"reason": reason, "turns": cres.GenSteps,
			"report": childReport(dir), "revived": true,
		})
		usage := delta
		l.bg.done <- bgOutcome{
			handle: callID, activityID: activityID,
			result: payload, isError: reason == "error" || reason == "contract_violation",
			canceled: canceled, usage: &usage,
			subagent: &event.SubagentCompleted{
				CallID: callID, Agent: agentName, ChildSession: child,
				Reason: reason, GenSteps: cres.GenSteps, Usage: delta,
			},
		}
	}()
	return nil
}

// reviveAllowance mirrors spawnAllowance for the revive path: what the
// parent can still afford, bounded by the child's own remaining headroom.
// Zero on both sides means unlimited (and the revive is never blocked).
func (l *Loop) reviveAllowance(s state.State, cap, spent int) int {
	parentRemaining := 0
	if l.Spec.Budget.MaxTotalTokens > 0 {
		parentRemaining = l.Spec.Budget.MaxTotalTokens - s.Session.Usage.Billed() - s.Budget.ReservedTotal()
		if parentRemaining < 1 {
			return 0 // parent exhausted: nothing to grant
		}
	}
	childRemaining := 0
	if cap > 0 {
		childRemaining = cap - spent
		if childRemaining < 1 {
			return 0 // child cap exhausted
		}
	}
	switch {
	case parentRemaining == 0 && childRemaining == 0:
		return 1 << 30 // both unlimited: effectively unbounded allowance
	case parentRemaining == 0:
		return childRemaining
	case childRemaining == 0:
		return parentRemaining
	default:
		return min(parentRemaining, childRemaining)
	}
}

// firstHopChild maps a descendant session id to the DIRECT child of parent
// on its path ("" when sid is not under parent).
func firstHopChild(parent, sid string) string {
	if !strings.HasPrefix(sid, parent+"-sub-") {
		return ""
	}
	rest := strings.TrimPrefix(sid, parent+"-sub-")
	if idx := strings.Index(rest, "-sub-"); idx >= 0 {
		rest = rest[:idx]
	}
	return parent + "-sub-" + rest
}

// childSpecFromJournal recovers the child's frozen spec from its
// SessionStarted — the truth for static AND dynamic (inline-role) children.
func childSpecFromJournal(dir string) (*AgentSpec, error) {
	events, err := store.ReadEvents(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		if e.Type != event.TypeSessionStarted {
			continue
		}
		decoded, derr := event.DecodePayload(e)
		if derr != nil {
			return nil, derr
		}
		started := decoded.(*event.SessionStarted)
		if len(started.Spec) == 0 {
			return nil, fmt.Errorf("child SessionStarted carries no spec")
		}
		var spec AgentSpec
		if err := json.Unmarshal(started.Spec, &spec); err != nil {
			return nil, fmt.Errorf("child spec unmarshal: %w", err)
		}
		return &spec, nil
	}
	return nil, fmt.Errorf("child journal has no SessionStarted")
}

// childFoldState folds a child journal (the settle/revive-side truth).
func childFoldState(dir string) (state.State, error) {
	events, err := store.ReadEvents(dir)
	if err != nil {
		return state.State{}, err
	}
	return state.Fold(events)
}

func subUsage(a, b provider.Usage) provider.Usage {
	clamp := func(n int) int {
		if n < 0 {
			return 0
		}
		return n
	}
	return provider.Usage{
		InputTokens:      clamp(a.InputTokens - b.InputTokens),
		OutputTokens:     clamp(a.OutputTokens - b.OutputTokens),
		CacheReadTokens:  clamp(a.CacheReadTokens - b.CacheReadTokens),
		CacheWriteTokens: clamp(a.CacheWriteTokens - b.CacheWriteTokens),
	}
}
