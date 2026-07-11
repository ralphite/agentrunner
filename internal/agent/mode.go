package agent

import (
	"strconv"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/protocol"
)

// modePromptSuffix is the 3.6b injection: appended to the system prompt's
// tail. S4.4a folds this into the assembly pipeline (declared planned
// migration in PLAN).
func modePromptSuffix(mode string) string {
	if mode == pipeline.ModePlan {
		return "\n\nYou are in PLAN MODE: read and analyze only. Editing and " +
			"executing tools are unavailable. When your plan is ready, call " +
			"exit_plan_mode with a summary to request approval to proceed."
	}
	return ""
}

// applyModeControl is the user-command mode transition (INC-42, G29) — the
// third trigger in {startup, approved exit_plan_mode, user command}. Validity
// is the 3.6c table narrowed to user sovereignty over approvals only:
// default↔acceptEdits. Bypass stays a process-start choice, and leaving plan
// stays exit_plan_mode's approval flow even though the table would allow
// plan→default (widening that is a separate ruling, not a side effect here).
// The permitted face follows automatically — effects carry the live fold mode
// (PermissionGate.effectiveMode) — and neither side of default↔acceptEdits
// differs in advertised face or prompt suffix, so the prefix stays stable.
func (l *Loop) applyModeControl(ds *driveState, appendE AppendFunc, ctl protocol.Control) error {
	from := ds.s.CurrentMode()
	target := ctl.Directive
	switch {
	case !pipeline.ValidMode(target):
		return l.rejectModeControl(appendE, ctl, "unknown mode "+strconv.Quote(target))
	case target == from:
		return nil // idempotent re-request → the generic no_op receipt
	case from == pipeline.ModePlan:
		return l.rejectModeControl(appendE, ctl, "leave plan mode via exit_plan_mode approval")
	case !pipeline.ValidTransition(from, target):
		return l.rejectModeControl(appendE, ctl, from+" → "+target+" is not a valid runtime transition")
	}
	if _, err := appendE(event.TypeModeChanged, &event.ModeChanged{
		To: target, Cause: "user",
	}); err != nil {
		return err
	}
	l.emit(protocol.Event{Kind: protocol.KindModeChanged, Mode: target})
	return nil
}

// rejectModeControl records why a mode request did not switch: an explicit
// receipt for durable commands (so "why didn't it change" is answerable from
// the journal alone) and a live message either way.
func (l *Loop) rejectModeControl(appendE AppendFunc, ctl protocol.Control, reason string) error {
	l.emit(protocol.Event{Kind: protocol.KindMessage, Text: "mode: " + reason})
	if ctl.CommandID == "" {
		return nil
	}
	_, err := appendE(event.TypeCommandHandled, &event.CommandHandled{
		CommandID: ctl.CommandID, CommandSeq: ctl.CommandSeq,
		Kind: ctl.Kind, Result: "rejected: " + reason,
	})
	return err
}
