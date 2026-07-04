package agent

import (
	"github.com/ralphite/agentrunner/internal/pipeline"
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
