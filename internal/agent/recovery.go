package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// reattachWaitingChildren preserves the one recoverable in-doubt spawn
// shape: a background child whose own journal is idle on an approval. The
// child did not die inside an unknown external effect, so Resume can safely
// re-enter its existing wait without replaying work. It retains the original
// parent activity/handle/lease; completion returns through bg.done and the
// ordinary single-writer settlement path.
func (l *Loop) reattachWaitingChildren(ctx context.Context, ds *driveState,
	acts []event.ActivityStarted) ([]event.ActivityStarted, error) {

	remaining := make([]event.ActivityStarted, 0, len(acts))
	for _, act := range acts {
		if act.Name != "spawn_agent" || !act.Background || act.CallID == "" ||
			!safeCallIDRe.MatchString(act.CallID) {
			remaining = append(remaining, act)
			continue
		}
		attempt := act.Attempt
		if attempt < 1 {
			attempt = 1
		}
		childDir := filepath.Join(l.Store.Dir(), "sub", fmt.Sprintf("%s-a%d", act.CallID, attempt))
		childSession := fmt.Sprintf("%s-sub-%s-a%d", l.SessionID, act.CallID, attempt)
		childState, err := childFoldState(childDir)
		if err != nil || childState.Waiting == nil || childState.Waiting.Kind != event.WaitApproval {
			remaining = append(remaining, act)
			continue
		}
		childSpec, err := childSpecFromJournal(childDir)
		if err != nil {
			slog.Warn("resume: approval-waiting child cannot be reconstructed; crash-settling",
				"child", childSession, "err", err)
			remaining = append(remaining, act)
			continue
		}
		childExec, err := l.childExecutorFromJournal(childDir, childSession)
		if err != nil {
			slog.Warn("resume: approval-waiting child workspace cannot be reopened; crash-settling",
				"child", childSession, "err", err)
			remaining = append(remaining, act)
			continue
		}
		cr, err := OpenChildRun(childDir)
		if err != nil {
			slog.Warn("resume: approval-waiting child store unavailable; crash-settling",
				"child", childSession, "err", err)
			remaining = append(remaining, act)
			continue
		}

		// The child must inherit the root's tree router before its Resume starts;
		// drive() normally creates it later, after crash reconciliation.
		l.ensureRouter()
		child := l.childLoopWithExec(childSpec, cr.Store(), childSession,
			childSpec.Budget.MaxTotalTokens, ds.s.CurrentMode(), childExec)
		l.ensureBackground()
		workCtx, cancel := context.WithCancelCause(ctx)
		l.bg.mu.Lock()
		l.bg.cancel[act.CallID] = cancel
		l.bg.mu.Unlock()

		agentName := spawnAgentNameOf(act.Args)
		if agentName == "" {
			agentName = childSpec.Name
		}
		baseline := reviveBaselineOf(act.Args)
		var assignment *event.TeamWorkspace
		for _, delegation := range ds.s.Team {
			if delegation.CallID == act.CallID {
				assignment = delegation.Workspace
				break
			}
		}
		go func(act event.ActivityStarted) {
			defer cr.Close()
			// The journal is non-empty and approval-parked (not quiescent),
			// so childRun's three-way decision lands on Resume — the same
			// substrate as a fresh spawn. total is the fold's CUMULATIVE
			// spend; the revive baseline is subtracted here at the caller.
			cres, total, cerr := cr.Run(workCtx, child, "")
			spent := subUsage(total, baseline)
			reason := cres.Reason
			if cerr != nil && reason == "" {
				reason = "error"
			}
			payload, _ := json.Marshal(map[string]any{
				"agent": agentName, "child_session": childSession,
				"reason": reason, "turns": cres.GenSteps,
				"report": childReport(childDir), "workspace": assignment,
				"recovered_approval_wait": true,
			})
			usage := spent
			l.bg.done <- bgOutcome{
				handle: act.CallID, activityID: act.ActivityID,
				result: payload, isError: reason == "error" || reason == "contract_violation",
				canceled: workCtx.Err() != nil, err: cerr, usage: &usage,
				subagent: &event.SubagentCompleted{
					CallID: act.CallID, Agent: agentName, ChildSession: childSession,
					Reason: reason, GenSteps: cres.GenSteps, Usage: spent,
				},
			}
		}(act)
	}
	return remaining, nil
}

// settleCrashInDoubt disposes of in-doubt activities on resume, per class
// (决策 #29 单一自愈): a long-lived session must self-heal after a crash
// instead of wedging on human triage. NOTHING re-runs — an execute/edit
// effect with an unknown outcome renders as an interrupted-by-crash error
// result the model sees and reacts to; a background child settles from its
// own journal (the child fold is the truth of how far it got).
func (l *Loop) settleCrashInDoubt(appendE AppendFunc, acts []event.ActivityStarted, timers state.Timers) error {
	for _, act := range acts {
		// The activity's durable timeout timer (2.11) outlived it: a crash
		// between TimerSet and the activity's own terminal left it pending.
		// The live path cancels it when the activity finishes (activity.go);
		// the crash-settle path must too, or the orphaned FUTURE timer keeps
		// the session from ever reaching quiescence — a resumed submitted run
		// idles on it forever instead of returning, so its stop is stuck (T1).
		// FirePendingTimers below can't help: it only fires EXPIRED timers, and
		// the loop's comment there assumes a re-armed re-run — but a non-
		// idempotent activity never re-runs. Keyed by purpose, so it cancels
		// only THIS activity's timer.
		if err := cancelActivityTimeout(appendE, timers, act.ActivityID); err != nil {
			return err
		}
		switch {
		case act.Name == "spawn_agent":
			if err := l.settleCrashedSpawn(appendE, act); err != nil {
				return err
			}
		case act.Background && act.CallID != "":
			// A background work's process died with the runtime; its
			// cancellation receipt reaches the model as a background outcome.
			if _, err := appendE(event.TypeActivityCancelled, &event.ActivityCancelled{
				ActivityID:    act.ActivityID,
				PartialOutput: "[interrupted by crash] the runtime died while this work ran; it was not re-run",
			}); err != nil {
				return err
			}
		default:
			if err := l.renderCrashFailure(appendE, act); err != nil {
				return err
			}
		}
	}
	return nil
}

// cancelActivityTimeout journals TimerCancelled for every pending timeout
// timer armed for an activity (activity.go names them "activity_timeout:<id>";
// there is at most one live per activity). Deterministic order keeps the
// settled journal stable across resumes.
func cancelActivityTimeout(appendE AppendFunc, timers state.Timers, activityID string) error {
	purpose := "activity_timeout:" + activityID
	ids := make([]string, 0, 1)
	for id, t := range timers {
		if t.Purpose == purpose {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, err := appendE(event.TypeTimerCancelled, &event.TimerCancelled{TimerID: id}); err != nil {
			return err
		}
	}
	return nil
}

// renderCrashFailure resolves one in-doubt foreground activity as a final,
// model-visible failure — the never-silently-re-run red line holds.
func (l *Loop) renderCrashFailure(appendE AppendFunc, act event.ActivityStarted) error {
	_, err := appendE(event.TypeActivityFailed, &event.ActivityFailed{
		ActivityID: act.ActivityID, Attempt: act.Attempt, Final: true,
		Error: event.ErrorInfo{Class: string(errs.Canceled),
			Message: "[interrupted by crash] the runtime died while this ran; the effect may or may not have happened and was NOT re-run"},
	})
	return err
}

// settleCrashedSpawn settles an in-doubt child (blocking or background)
// from the child's own journal — settle-from-child-fold (v2 M5.1): a child
// that FINISHED before the crash delivers its real receipt; one that died
// with the process settles as a crash cancellation carrying its true spend.
func (l *Loop) settleCrashedSpawn(appendE AppendFunc, act event.ActivityStarted) error {
	callID := act.CallID
	// The spawn-time CallID path guard (safeCallIDRe) runs INSIDE the spawn
	// closure — a crash in the Started→terminal window can leave an
	// unvalidated CallID in the journal. Re-apply it before deriving any
	// path from it (收口 security review): a malformed one renders as a
	// crash failure, never a directory read.
	if callID == "" || !safeCallIDRe.MatchString(callID) {
		return l.renderCrashFailure(appendE, act)
	}
	attempt := act.Attempt
	if attempt < 1 {
		attempt = 1
	}
	childDir := filepath.Join(l.Store.Dir(), "sub", fmt.Sprintf("%s-a%d", callID, attempt))
	childSession := fmt.Sprintf("%s-sub-%s-a%d", l.SessionID, callID, attempt)
	agentName := spawnAgentNameOf(act.Args)

	evs, rerr := store.ReadEvents(childDir)
	if rerr != nil || len(evs) == 0 {
		// The child never journaled anything: nothing happened downstream;
		// the spawn itself renders as a crash failure.
		return l.renderCrashFailure(appendE, act)
	}
	cf, ferr := state.Fold(evs)
	if ferr != nil {
		return fmt.Errorf("crash settle %s: child fold: %w", callID, ferr)
	}
	// A revive activity (INC-12.2) carries the child's settled spend at
	// revive time in its synthetic args — terminals report the DELTA so the
	// parent account never double-counts the child's earlier rounds.
	settled := subUsage(cf.Session.Usage, reviveBaselineOf(act.Args))

	if quiescent, reason := state.Quiescence(cf); quiescent {
		// The child reached quiescence before the crash — deliver the
		// receipt it already earned, with the reason read off its shape
		// (SubagentCompleted before the activity terminal, same order as
		// the live settle path).
		if _, err := appendE(event.TypeSubagentCompleted, &event.SubagentCompleted{
			CallID: callID, Agent: agentName, ChildSession: childSession,
			Reason: reason, GenSteps: cf.Session.GenStep, Usage: settled,
		}); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"agent": agentName, "child_session": childSession,
			"reason": reason, "turns": cf.Session.GenStep,
			"report": childReport(childDir),
		})
		usage := settled
		_, err := appendE(event.TypeActivityCompleted, &event.ActivityCompleted{
			ActivityID: act.ActivityID, Result: payload,
			IsError: reason == "error" || reason == "contract_violation",
			Usage:   &usage,
		})
		return err
	}

	// The child died with the process: settle as a crash cancellation with
	// the child's real settled spend (tree budget stays honest, S5).
	if _, err := appendE(event.TypeSubagentCompleted, &event.SubagentCompleted{
		CallID: callID, Agent: agentName, ChildSession: childSession,
		Reason: "crash", GenSteps: cf.Session.GenStep, Usage: settled,
	}); err != nil {
		return err
	}
	_, err := appendE(event.TypeActivityCancelled, &event.ActivityCancelled{
		ActivityID:    act.ActivityID,
		PartialOutput: "[interrupted by crash] the sub-agent died with the runtime; its journal holds the partial work",
		Usage:         &settled,
	})
	return err
}

// reviveBaselineOf extracts the revive baseline from a synthetic revive
// activity's args; zero for an ordinary spawn (delta = total).
func reviveBaselineOf(raw json.RawMessage) provider.Usage {
	var meta struct {
		Baseline provider.Usage `json:"baseline"`
	}
	_ = json.Unmarshal(raw, &meta)
	return meta.Baseline
}

// spawnAgentNameOf recovers the target agent name from the journaled
// (redacted) spawn args; best-effort — an unparsable blob yields "".
func spawnAgentNameOf(raw json.RawMessage) string {
	var args struct {
		Agent string `json:"agent"`
	}
	_ = json.Unmarshal(raw, &args)
	return args.Agent
}
