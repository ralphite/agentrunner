package agent

import (
	"github.com/ralphite/agentrunner/internal/event"
)

// WaitRule is one row of the waiting-state registry (2.14). The registry is
// the single source of each wait kind's interrupt semantics — production
// resolution paths read their WaitingResolved.Resolution literal FROM here
// (INC-69 wired the sites; hardcoding the strings again is the ad-hoc table
// this registry exists to prevent).
type WaitRule struct {
	Kind string
	// Interruptible: a user interrupt may resolve this wait.
	Interruptible bool
	// OnInterrupt is the WaitingResolved.Resolution an interrupt produces.
	OnInterrupt string
	// ResolvedBy names the non-interrupt resolution source.
	ResolvedBy string
}

// WaitRules is the closed registry: input (the standby idle — background
// settlements wake it too, 决策 #31) and approval. There is no handles/timer
// wait kind — in-flight work parks the session in the SAME input idle, and
// timers belong to the daemon sweep.
var WaitRules = map[string]WaitRule{
	event.WaitInput: {
		Kind: event.WaitInput, Interruptible: true,
		OnInterrupt: "superseded_by_interrupt", ResolvedBy: "input",
	},
	event.WaitApproval: {
		Kind: event.WaitApproval, Interruptible: true,
		// 3.5 denied-by-interrupt: the approval resolves as a denial and
		// the call renders "[interrupted by user]".
		OnInterrupt: "denied_by_interrupt", ResolvedBy: "approval_response",
	},
}
