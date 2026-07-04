package driver

import (
	"testing"

	"github.com/ralphite/agentrunner/internal/agent"
)

// reserve is the min-aggregation of the tree remaining and the child spec cap
// (DESIGN: the driver is the tree budget root). This nails the math without a
// full run.
func TestReserveAllowance(t *testing.T) {
	cases := []struct {
		name          string
		treeBudget    int
		childCap      int
		spent         int
		wantAllowance int
		wantOK        bool
	}{
		{"unlimited tree, unlimited child", 0, 0, 0, 0, true},
		{"unlimited tree, capped child", 0, 500, 9999, 500, true},
		{"tree cap, no child cap", 1000, 0, 0, 1000, true},
		{"child cap tighter", 1000, 300, 0, 300, true},
		{"tree remaining tighter", 1000, 300, 800, 200, true},
		{"exactly exhausted", 1000, 0, 1000, 0, false},
		{"over budget", 1000, 0, 1200, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := &Driver{Spec: &DriverSpec{
				Agent:  &agent.AgentSpec{Budget: agent.BudgetSpec{MaxTotalTokens: c.childCap}},
				Budget: BudgetSpec{MaxTotalTokens: c.treeBudget},
			}}
			got, ok := d.reserve(&State{SpentTokens: c.spent})
			if got != c.wantAllowance || ok != c.wantOK {
				t.Errorf("reserve = (%d, %v), want (%d, %v)", got, ok, c.wantAllowance, c.wantOK)
			}
		})
	}
}
