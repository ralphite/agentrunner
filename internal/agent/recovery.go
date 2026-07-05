package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// settleCrashInDoubt disposes of in-doubt activities on a CONVERSATIONAL
// resume, per class (v2 M5.1): a long-lived session must self-heal after a
// crash instead of wedging on human triage. NOTHING re-runs — an execute/
// edit effect with an unknown outcome renders as an interrupted-by-crash
// error result the model sees and reacts to; a background child settles
// from its own journal (the child fold is the truth of how far it got).
// Task-mode resume keeps v1's refuse-and-surface InDoubtError contract.
func (l *Loop) settleCrashInDoubt(appendE AppendFunc, acts []event.ActivityStarted) error {
	for _, act := range acts {
		switch {
		case act.Name == "spawn_agent":
			if err := l.settleCrashedSpawn(appendE, act); err != nil {
				return err
			}
		case act.Background && act.CallID != "":
			// A background task's process died with the runtime; its
			// cancellation receipt reaches the model as a task outcome.
			if _, err := appendE(event.TypeActivityCancelled, &event.ActivityCancelled{
				ActivityID:    act.ActivityID,
				PartialOutput: "[interrupted by crash] the runtime died while this task ran; it was not re-run",
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

	if cf.Run.Status == state.StatusEnded {
		// The child finished before the crash — deliver the receipt it
		// already earned (SubagentCompleted before the activity terminal,
		// same order as the live settle path).
		if _, err := appendE(event.TypeSubagentCompleted, &event.SubagentCompleted{
			CallID: callID, Agent: agentName, ChildSession: childSession,
			Reason: cf.Run.Reason, Turns: cf.Run.Turn, Usage: cf.Run.Usage,
		}); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"agent": agentName, "child_session": childSession,
			"reason": cf.Run.Reason, "turns": cf.Run.Turn,
			"report": childReport(childDir),
		})
		usage := cf.Run.Usage
		_, err := appendE(event.TypeActivityCompleted, &event.ActivityCompleted{
			ActivityID: act.ActivityID, Result: payload,
			IsError: cf.Run.Reason == "error" || cf.Run.Reason == "contract_violation",
			Usage:   &usage,
		})
		return err
	}

	// The child died with the process: settle as a crash cancellation with
	// the child's real settled spend (tree budget stays honest, S5).
	spent := cf.Run.Usage
	if _, err := appendE(event.TypeSubagentCompleted, &event.SubagentCompleted{
		CallID: callID, Agent: agentName, ChildSession: childSession,
		Reason: "crash", Turns: cf.Run.Turn, Usage: spent,
	}); err != nil {
		return err
	}
	_, err := appendE(event.TypeActivityCancelled, &event.ActivityCancelled{
		ActivityID:    act.ActivityID,
		PartialOutput: "[interrupted by crash] the sub-agent died with the runtime; its journal holds the partial work",
		Usage:         &spent,
	})
	return err
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
