package agent

import (
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider"
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

// advertisedTools filters the tool face by mode (3.6a): what the model
// cannot use, it does not see. The permission gate's mode defaults are
// the second door behind this one.
func advertisedTools(defs []provider.ToolDef, mode string) []provider.ToolDef {
	out := make([]provider.ToolDef, 0, len(defs))
	for _, d := range defs {
		if pipeline.ClassAdvertised(mode, toolClass(d.Name)) {
			out = append(out, d)
		}
	}
	return out
}
