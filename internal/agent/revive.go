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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
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

// scanPendingChildMail enqueues a revive request for every DESCENDANT
// (direct or deeper) whose inbox holds unconsumed mail — crash recovery /
// restart continuation: the durable truth outlives every dropped wake.
// Walks the WHOLE subtree because a grandchild's mail lives only in the
// grandchild's own inbox; enqueuing the descendant sid lets reviveChild
// relay through the first hop (INC-12 正确性 review P0). Runs once as the
// drive starts.
func (l *Loop) scanPendingChildMail() {
	if l.Router == nil || l.revive == nil {
		return
	}
	var walk func(baseDir, baseSid string)
	walk = func(baseDir, baseSid string) {
		subs, err := filepath.Glob(filepath.Join(baseDir, "sub", "*"))
		if err != nil {
			return
		}
		for _, d := range subs {
			sid := baseSid + "-sub-" + filepath.Base(d)
			if l.childHasMail(sid) {
				select {
				case l.revive <- sid:
				default:
					slog.Warn("revive scan: queue full, mail deferred to next resume", "member", sid)
				}
			}
			walk(d, sid) // recurse: a grandchild's mail is only in its own inbox
		}
	}
	walk(l.Store.Dir(), l.SessionID)
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
		// A single unreadable/corrupt member journal must never abort the
		// whole tree host (INC-12 review P1): reviveChild runs from
		// drainRevives, whose error propagates to drive()→abort, tearing down
		// every healthy sibling. All CHILD-journal reads here are best-effort
		// per-member — skip softly, the mail stays durable. Only PARENT-side
		// appendE failures (below) are fatal.
		slog.Warn("revive skipped: member fold unreadable", "member", child, "err", err)
		return nil
	}
	// The mail may be for CHILD itself (sid == child) or for a DEEPER
	// descendant that only CHILD can re-host (sid != child). In the deep
	// case CHILD is a RELAY: we re-host it so its OWN scanPendingChildMail
	// carries the wake one hop further (DESIGN §3 "孙由中间父 resume 时同一
	// 扫描接力"). The mail lives in the ACTUAL recipient's inbox — reading
	// CHILD's would find nothing and strand a grandchild's durable mail
	// (INC-12 正确性 review P0). So resolve the tail from the recipient.
	relay := sid != child
	mailDir, mailConsumed := dir, st.Session.ConsumedInputSeq
	if relay {
		rdir, rerr := l.Router.DirOf(sid)
		if rerr != nil {
			return nil
		}
		rst, rerr := childFoldState(rdir)
		if rerr != nil {
			// A single unreadable/corrupt DESCENDANT journal must not abort
			// the whole tree host (INC-12 review P2): skip this relay softly,
			// exactly like the DirOf failure above. The mail stays durable.
			slog.Warn("revive relay: recipient fold unreadable; skipping", "recipient", sid, "err", rerr)
			return nil
		}
		mailDir, mailConsumed = rdir, rst.Session.ConsumedInputSeq
	}
	tail, err := store.ReadInbox(mailDir, mailConsumed)
	if err != nil || len(tail) == 0 {
		return nil // consumed meanwhile (or unreadable): nothing to do
	}

	// Marks gate the automatic path (决策 #30). For a RELAY parent, ANY
	// close/kill mark blocks the automatic hop: a descendant's mail cannot
	// resurrect an explicitly-terminated intermediate parent — the mail
	// stays durable until CHILD is reopened by an explicit send, whose own
	// scan then carries it on. For the DIRECT recipient, user-kill only
	// yields to user-class mail (the explicit reopen gesture).
	if mark := st.Session.Closed; mark != nil {
		if relay {
			slog.Warn("revive skipped: relay parent is marked; descendant mail stays durable",
				"relay", child, "recipient", sid)
			return nil
		}
		if mark.Source == "user" {
			hasUserMail := false
			for _, in := range tail {
				if userClassSource(in.Source) {
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
	}

	// Budget: the revive allowance tops the child's frozen cap up by what
	// the parent can still afford — min(parent remaining, original cap) over
	// the child's settled spend. Nothing left → the parent model hears it
	// (program source, INC-D1 family) and decides; the mail stays durable.
	spent := st.Session.Usage.Billed()
	cap := 0
	spec, specErr := childSpecFromJournal(dir)
	if specErr != nil {
		// Best-effort per-member read (see fold guard above): a corrupt /
		// specless member journal skips softly, never aborts the tree host.
		slog.Warn("revive skipped: member spec unreadable", "member", child, "err", specErr)
		return nil
	}
	if spec.Budget.MaxTotalTokens > 0 {
		cap = spec.Budget.MaxTotalTokens
	}
	childExec, err := l.childExecutorFromJournal(dir, child)
	if err != nil {
		if errors.Is(err, errForeignWorkspace) {
			// Fork isolation guard (INC-12 交互 review P1): a forked session
			// copies sub/ verbatim, so an inherited child's journaled
			// WorkspaceRoot is the ORIGINAL session's absolute path. Reviving
			// it would write into the original workspace (cross-session
			// corruption). Refuse the automatic path; the mail stays durable,
			// and a fork that wants the team continued re-spawns it (mirrors
			// cancel_at_fork, DESIGN §12).
			slog.Warn("revive skipped: child workspace is outside this session (fork provenance); mail stays durable",
				"child", child)
			return nil
		}
		// Any other executor-open failure (unreadable journal, missing
		// workspace) skips softly — a single member never aborts the host.
		slog.Warn("revive skipped: member executor unavailable", "member", child, "err", err)
		return nil
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
	loop := l.childLoopWithExec(spec, childStore, child, frozenCap, ds.s.CurrentMode(), childExec)

	l.ensureBackground()
	workCtx, cancel := context.WithCancelCause(ctx)
	l.bg.mu.Lock()
	l.bg.cancel[callID] = cancel
	l.bg.mu.Unlock()

	agentName := spec.Name
	go func() {
		defer func() { _ = childStore.Close() }()
		cres, cerr := loop.Resume(workCtx)
		after, aerr := childFoldState(dir)
		var total provider.Usage
		if aerr == nil {
			total = after.Session.Usage
		} else {
			total = baseline
		}
		delta := subUsage(total, baseline)
		reason := cres.Reason
		canceled := workCtx.Err() != nil
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

func (l *Loop) childExecutorFromJournal(dir, session string) (*tool.Executor, error) {
	events, err := store.ReadEvents(dir)
	if err != nil {
		return nil, err
	}
	root := ""
	for _, env := range events {
		if env.Type != event.TypeSessionStarted {
			continue
		}
		decoded, derr := event.DecodePayload(env)
		if derr != nil {
			return nil, derr
		}
		root = decoded.(*event.SessionStarted).WorkspaceRoot
		break
	}
	// Shared with the parent (the common case): reuse the parent executor.
	// Compare canonically — the journaled root may be un-normalized while
	// workspace.New canonicalized the parent's (macOS /var↔/private/var), so
	// a plain string compare would spuriously fall through to the fork guard.
	if root == "" || (l.Exec != nil && l.Exec.WS != nil && sameDir(root, l.Exec.WS.Root())) {
		return l.Exec, nil
	}
	// A legit isolated child of THIS tree has its worktree UNDER this
	// session's store dir (spawn.go: <store>/sub/<call>/worktree). A root
	// ELSEWHERE is stale fork provenance (INC-12 交互 review P1): a forked
	// session copies sub/ verbatim carrying the ORIGINAL absolute path.
	// Refuse — never workspace.New() an outside path (which would create /
	// write into the original session's workspace).
	if !underDir(root, l.Store.Dir()) {
		return nil, errForeignWorkspace
	}
	ws, err := workspace.New(root)
	if err != nil {
		return nil, fmt.Errorf("reopen isolated workspace: %w", err)
	}
	exec := &tool.Executor{WS: ws, Session: session}
	if l.Exec != nil {
		exec.ProbeSandbox = l.Exec.ProbeSandbox
		if l.Exec.NetworkContained() {
			exec.ContainNetwork()
		}
	}
	return exec, nil
}

// errForeignWorkspace marks a child whose journaled WorkspaceRoot is outside
// this session's managed tree — stale fork provenance (INC-12 交互 review
// P1). The automatic revive path refuses it; the durable mail is untouched.
var errForeignWorkspace = errors.New("child workspace outside this session (fork provenance)")

// canonical resolves symlinks in the LONGEST EXISTING prefix of a path and
// keeps the (possibly missing) tail. This canonicalizes both sides of a
// comparison CONSISTENTLY even when one leaf is absent (INC-12 review P2):
// plain EvalSymlinks on each side independently would mix a resolved
// /private/var with a literal /var when one path's leaf is missing, and a
// symlinked prefix (macOS /var→/private/var, or a symlinked XDG_DATA_HOME)
// would then produce a spurious mismatch. Resolving the existing prefix
// makes the comparison stable regardless of which leaves exist.
func canonical(p string) string {
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p // reached the root; nothing more to resolve
	}
	return filepath.Join(canonical(parent), filepath.Base(p))
}

// sameDir reports whether two paths denote the same directory (a journaled
// root may be un-normalized while the live workspace root was canonicalized
// at open).
func sameDir(a, b string) bool {
	return canonical(a) == canonical(b)
}

// underDir reports whether path is dir or a descendant of it, canonicalizing
// both consistently so /tmp vs /private/tmp (macOS) — or any symlinked prefix
// — does not cause a false negative even when path's leaf is missing.
func underDir(path, dir string) bool {
	rel, err := filepath.Rel(canonical(dir), canonical(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
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
