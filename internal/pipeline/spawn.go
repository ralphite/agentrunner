package pipeline

import "context"

// SpawnGate optionally bounds agent-launching effects (spawn_agent and
// handoff_agent — both start a child run, S5.3/S5.4). Zero means unlimited:
// the production default must not silently amputate recursive delegation.
// Tests/embedders can still install explicit bounds.
type SpawnGate struct {
	MaxDepth  int // zero = unlimited
	MaxSpawns int // zero = unlimited
}

func (g *SpawnGate) Name() string { return "spawn" }

func (g *SpawnGate) Check(_ context.Context, eff Effect) Decision {
	if eff.ToolName != "spawn_agent" && eff.ToolName != "handoff_agent" {
		return Allow
	}
	if eff.HandoffPending {
		return Deny("control already transferred by an earlier handoff this turn")
	}
	if g.MaxDepth > 0 && eff.SpawnDepth >= g.MaxDepth {
		return Deny("agent tree depth limit reached")
	}
	if g.MaxSpawns > 0 && eff.SpawnCount >= g.MaxSpawns {
		return Deny("spawn fan-out limit reached for this run")
	}
	return Allow
}
