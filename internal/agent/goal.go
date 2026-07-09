package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/state"
)

// applyGoalControl journals the fold event for one in-session goal control
// (INC-D1), delivered out-of-band like compact/clear (never conversation
// content). On attach it also injects the goal statement as a program input so
// the agent starts working toward it in-context; the verifier then checks at
// each quiescence boundary.
func (l *Loop) applyGoalControl(ds *driveState, appendE AppendFunc, ctl protocol.Control) error {
	// The goal STATEMENT and any re-injected text are conversation content and
	// are redacted like every other journaled input (凭据红线 §18.5). The
	// verifier COMMANDS are NOT redacted — they must run verbatim (same as a
	// bash tool call's args); their journaled displays go through redaction.
	red := redact.FromEnv()
	switch ctl.Kind {
	case protocol.ControlGoalAttach:
		if ctl.Goal == nil {
			return nil
		}
		if _, err := appendE(event.TypeGoalAttached, &event.GoalAttached{
			GoalID: ctl.Goal.GoalID, Goal: red.String(ctl.Goal.Goal), Verifiers: ctl.Goal.Verifiers,
			Budget: goalBudgetOr(ctl.Goal.Budget), Source: "user",
		}); err != nil {
			return err
		}
		text := red.String("New goal to work toward: " + ctl.Goal.Goal +
			"\nKeep going until it's met; a verifier checks at each pause.")
		_, err := appendE(event.TypeInputReceived, &event.InputReceived{Text: text, Source: "program"})
		return err
	case protocol.ControlGoalPause:
		if ds.s.Goal == nil {
			return nil
		}
		_, err := appendE(event.TypeGoalPaused, &event.GoalPaused{GoalID: ds.s.Goal.GoalID, Source: "user"})
		return err
	case protocol.ControlGoalResume:
		if ds.s.Goal == nil {
			return nil
		}
		_, err := appendE(event.TypeGoalResumed, &event.GoalResumed{GoalID: ds.s.Goal.GoalID, Source: "user"})
		return err
	case protocol.ControlGoalUpdate:
		if ds.s.Goal == nil || ctl.Goal == nil {
			return nil
		}
		_, err := appendE(event.TypeGoalUpdated, &event.GoalUpdated{
			GoalID: ds.s.Goal.GoalID, Goal: red.String(ctl.Goal.Goal), Verifiers: ctl.Goal.Verifiers,
			Budget: ctl.Goal.Budget, Source: "user",
		})
		return err
	case protocol.ControlGoalCancel:
		if ds.s.Goal == nil {
			return nil
		}
		_, err := appendE(event.TypeGoalCancelled, &event.GoalCancelled{
			GoalID: ds.s.Goal.GoalID, Reason: "user", Source: "user",
		})
		return err
	}
	return nil
}

func goalBudgetOr(b *event.GoalBudget) event.GoalBudget {
	if b == nil {
		return event.GoalBudget{}
	}
	return *b
}

// DefaultGoalMaxChecks bounds an in-session goal whose budget omits max_checks
// (review Bug 3): a never-passing verifier must still terminate even if a
// driver dials the control-plane directly with Budget:nil / max_checks 0.
const DefaultGoalMaxChecks = 20

// goalMaxChecks is the effective goal-level check cap — the explicit budget, or
// the default backstop when unset (always > 0, so a miss loop always ends).
func goalMaxChecks(g *state.Goal) int {
	if g.Budget.MaxChecks > 0 {
		return g.Budget.MaxChecks
	}
	return DefaultGoalMaxChecks
}

// goalRecover repairs a crash that landed between a goal checkpoint and its
// follow-up event (INC-D1 R1/R2). It runs at the drive-loop safe point every
// iteration, NOT inside the quiescent sequence — because on resume the shape is
// already quiescent and idleOrReturn's !*quiesced gate skips quiescentActions
// (so goal_verify's own guard would be dead code, review Bug 1). It is a no-op
// unless a checkpoint at the CURRENT gen step is still missing its follow-up
// (in normal flow the follow-up is present, so nothing re-emits/re-injects).
func (l *Loop) goalRecover(ds *driveState, appendE AppendFunc) error {
	g := ds.s.Goal
	if g == nil || g.CheckpointedGenStep != ds.s.Session.GenStep {
		return nil
	}
	switch {
	case g.LastPass:
		_, err := appendE(event.TypeGoalAchieved, &event.GoalAchieved{
			GoalID: g.GoalID, Reason: "satisfied", Checks: g.Checks,
		})
		return err
	case g.Checks >= goalMaxChecks(g):
		_, err := appendE(event.TypeGoalAchieved, &event.GoalAchieved{
			GoalID: g.GoalID, Reason: "budget", Checks: g.Checks,
		})
		return err
	case g.LastFeedback != "" && !hasInputAfterLastAssistant(ds.s):
		_, err := appendE(event.TypeInputReceived, &event.InputReceived{
			Text: g.LastFeedback, Source: "program",
		})
		return err
	}
	return nil
}

// goalVerify runs an in-session goal's verifiers in the workspace (INC-D1).
// v0 supports command verifiers (exit 0 = pass); all must pass (AND). It runs
// the command through the loop's own executor. NOTE (review F1): unlike the
// driver's verifier — which IS adjudicated through the pipeline (driver.go
// verifyOne → adjudicateVerifier) — this v0 in-session verifier runs UNGATED
// (no permission/mode gate). Defensible because the command is operator-set
// only (control plane, not model/untrusted-reachable) and network containment
// still applies via the executor; but a locked-down session (plan mode / a deny
// rule on bash) does NOT gate it. Pipeline-adjudicating the goal verifier is a
// noted hardening follow-up (LOG). Returns pass + a short human detail.
func goalVerify(ctx context.Context, l *Loop, g *state.Goal) (bool, string) {
	if l.Exec == nil {
		return false, "no executor for goal verifier"
	}
	ran := 0
	for _, v := range g.Verifiers {
		if v.Kind != "command" || v.Command == "" {
			continue // v0: only command verifiers
		}
		ran++
		args, _ := json.Marshal(map[string]string{"command": v.Command})
		res := l.Exec.Execute(ctx, "bash", args)
		var out struct {
			ExitCode int    `json:"exit_code"`
			Stdout   string `json:"stdout"`
		}
		_ = json.Unmarshal(res.Payload, &out)
		if out.ExitCode != 0 {
			return false, fmt.Sprintf("`%s` exit=%d", v.Command, out.ExitCode)
		}
	}
	if ran == 0 {
		return false, "no command verifier to check"
	}
	return true, "all checks passed"
}

// goalCheckpoint is the goal_verify cell of the fixed quiescent sequence
// (INC-D1, 决策 #24/#21). At a graceful exchange boundary, with an active
// (unpaused) goal, it runs the verifier and:
//   - pass         → GoalCheckpoint{pass} + GoalAchieved{satisfied}, detach, idle.
//   - miss+budget  → GoalCheckpoint + a program-source InputReceived re-injects
//     the feedback so the SAME thread continues in-context (ds.goalContinue
//     tells idleOrReturn not to idle; decide sees the input → next turn).
//   - miss+spent   → GoalCheckpoint + GoalAchieved{budget} = visible truncation
//     (决策 #31), detach, idle.
//
// It NEVER hijacks a generation step — the check lives only at turn close.
func goalCheckpoint(ctx context.Context, l *Loop, ds *driveState, appendE AppendFunc, reason *string) error {
	g := ds.s.Goal
	if g == nil || g.Paused {
		return nil
	}
	// Only a graceful ending checks the goal; a dying/transferred turn does not.
	if *reason != "completed" && *reason != "max_generation_steps" {
		return nil
	}
	n := ds.s.Session.GenStep

	// Crash-recovery (R1/R2): if this gen step was already checkpointed, the
	// follow-up event didn't land before a crash — recover it (goalRecover also
	// runs at the drive-loop safe point for the resume case where this cell is
	// skipped). Never re-run the verifier at an already-checkpointed step.
	if g.CheckpointedGenStep == n {
		return l.goalRecover(ds, appendE)
	}

	pass, rawDetail := goalVerify(ctx, l, g)
	// The detail carries the verifier COMMAND string — redact it before it is
	// journaled / re-injected into the conversation (凭据红线 §18.5, review F3).
	detail := redact.FromEnv().String(rawDetail)
	check := g.Checks + 1
	budgetSpent := check >= goalMaxChecks(g)

	// Feedback is journaled ON the checkpoint (so recovery can re-inject it) and
	// is empty on a pass or on budget exhaustion (both detach, no continuation).
	feedback := ""
	if !pass && !budgetSpent {
		feedback = fmt.Sprintf(
			"[goal check %d not met] %s\nGoal: %s\nKeep working toward the goal; don't repeat approaches already ruled out.",
			check, detail, g.Goal)
	}

	if _, err := appendE(event.TypeGoalCheckpoint, &event.GoalCheckpoint{
		GoalID: g.GoalID, GenStep: n, Check: check, Pass: pass, Detail: detail, Feedback: feedback,
	}); err != nil {
		return err
	}

	switch {
	case pass:
		l.emit(protocol.Event{Kind: protocol.KindMessage, Text: "goal achieved"})
		_, err := appendE(event.TypeGoalAchieved, &event.GoalAchieved{
			GoalID: g.GoalID, Reason: "satisfied", Checks: check,
		})
		return err
	case budgetSpent:
		// Goal-level budget exhausted = visible truncation (决策 #31); detach,
		// no re-injection, session idles reopenable.
		l.emit(protocol.Event{Kind: protocol.KindError, Text: fmt.Sprintf(
			"goal not met after %d checks (budget spent) — truncating", check)})
		_, err := appendE(event.TypeGoalAchieved, &event.GoalAchieved{
			GoalID: g.GoalID, Reason: "budget", Checks: check,
		})
		return err
	default:
		// Miss with budget left: re-inject the feedback so the SAME thread
		// continues in-context (the wake seam in idleOrReturn sees this input).
		_, err := appendE(event.TypeInputReceived, &event.InputReceived{
			Text: feedback, Source: "program",
		})
		return err
	}
}
