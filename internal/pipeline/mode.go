package pipeline

import "github.com/ralphite/agentrunner/internal/event"

// Mode data model (3.6a): a mode is a tool-face filter plus per-class
// defaults (permission.go) plus a prompt suffix (owned by the agent
// layer). Everything here is a pure table.

// ClassAdvertised reports whether tools of a class are shown to the model
// at all in the given mode. Filtering the advertised face is the first
// line; the permission gate's mode defaults are the second (double door —
// a model hallucinating a hidden tool still gets denied).
func ClassAdvertised(mode, class string) bool {
	if mode == ModePlan {
		return class == "read" || class == "wait"
	}
	return true
}

// ValidTransition is the 3.6c rule table. Bypass is not a runtime
// transition — it can only be selected at process start (CLI flag).
func ValidTransition(from, to string) bool {
	switch [2]string{from, to} {
	case [2]string{ModePlan, ModeDefault}: // via approved exit_plan_mode
		return true
	case [2]string{ModeDefault, ModeAcceptEdits}, // user command (S4 interactive)
		[2]string{ModeAcceptEdits, ModeDefault}:
		return true
	}
	return false
}

// ValidMode reports whether the name is a known mode.
func ValidMode(mode string) bool {
	switch mode {
	case ModeDefault, ModePlan, ModeAcceptEdits, ModeBypass:
		return true
	}
	return false
}

// ValidAction reports whether a permission rule's action is one the gate
// understands. Used to reject a typo (e.g. "alow") at spec-load instead of
// silently at dispatch (QA Wave2 bob-05).
func ValidAction(action string) bool {
	switch action {
	case event.VerdictAllow, event.VerdictAsk, event.VerdictDeny:
		return true
	}
	return false
}
