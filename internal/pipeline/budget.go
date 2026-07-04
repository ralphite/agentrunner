package pipeline

import (
	"context"
	"fmt"
)

// Class price list (3.7b): coarse token-equivalents for tool accounting.
// LLM effects reserve their real max_tokens; these cover everything else.
var classEstTokens = map[string]int{
	"read":    500,
	"edit":    1000,
	"execute": 2000,
	"wait":    0,
}

// EstTokensForClass returns the reservation basis for a tool class.
func EstTokensForClass(class string) int {
	return classEstTokens[class]
}

// BudgetView is the live accounting the loop snapshots from the fold at
// adjudication time: settled usage plus outstanding reservations. The
// reserve-then-settle discipline makes concurrent adjudication safe — a
// second effect sees the first's reservation, not just its settled cost
// (the 3.7d TOCTOU property).
type BudgetView struct {
	SettledTokens  int
	ReservedTokens int
}

// BudgetGate denies effects whose reservation would break the run budget.
// Bypass mode does not bind (3.6d) — hooks still run, budget does not.
type BudgetGate struct {
	MaxTotalTokens int // 0 = unlimited
}

func (g *BudgetGate) Name() string { return "budget" }

func (g *BudgetGate) Check(_ context.Context, eff Effect) Decision {
	if g.MaxTotalTokens <= 0 || eff.Mode == ModeBypass {
		return Allow
	}
	projected := eff.Budget.SettledTokens + eff.Budget.ReservedTokens + eff.EstTokens
	if projected > g.MaxTotalTokens {
		return Deny(fmt.Sprintf("token budget exhausted: settled %d + reserved %d + est %d > limit %d",
			eff.Budget.SettledTokens, eff.Budget.ReservedTokens, eff.EstTokens, g.MaxTotalTokens))
	}
	return Allow
}
