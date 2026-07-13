package agent

import (
	"fmt"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
)

// WaitRule is one row of the waiting-state registry (2.14). All four
// variants are defined NOW so later stages slot into an existing table
// instead of growing an ad-hoc one; ProducibleStage says which stage may
// first journal a WaitingEntered of this kind.
type WaitRule struct {
	Kind            string
	ProducibleStage int
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
		Kind: event.WaitInput, ProducibleStage: 4, Interruptible: true,
		OnInterrupt: "superseded_by_interrupt", ResolvedBy: "input",
	},
	event.WaitApproval: {
		Kind: event.WaitApproval, ProducibleStage: 3, Interruptible: true,
		// 3.5 denied-by-interrupt: the approval resolves as a denial and
		// the call renders "[interrupted by user]".
		OnInterrupt: "denied_by_interrupt", ResolvedBy: "approval_response",
	},
}

// CanProduce reports whether a stage may journal WaitingEntered of kind.
func CanProduce(kind string, stage int) bool {
	rule, ok := WaitRules[kind]
	return ok && stage >= rule.ProducibleStage
}

// ResolveWaitingOnInterrupt handles a user interrupt against a idle run:
// the interrupt is journaled FIRST (journal-inputs-first), then the wait
// resolves per its registry row. A nil Waiting is a no-op; an unknown kind
// is corruption and errors loudly.
func ResolveWaitingOnInterrupt(s state.State, appendE AppendFunc) error {
	if s.Waiting == nil {
		return nil
	}
	rule, ok := WaitRules[s.Waiting.Kind]
	if !ok {
		return fmt.Errorf("waiting: unknown kind %q", s.Waiting.Kind)
	}
	if !rule.Interruptible {
		return fmt.Errorf("waiting: kind %q is not interruptible", s.Waiting.Kind)
	}
	if _, err := appendE(event.TypeInputReceived, &event.InputReceived{
		Text: "[interrupt]", Source: "interrupt",
	}); err != nil {
		return err
	}
	_, err := appendE(event.TypeWaitingResolved, &event.WaitingResolved{
		Kind: s.Waiting.Kind, Resolution: rule.OnInterrupt,
	})
	return err
}
