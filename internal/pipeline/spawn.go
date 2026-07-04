package pipeline

import "context"

// Spawn caps (S5.3): hard bounds on the agent tree. Depth 0 is the root
// run, so MaxDepth 2 allows children and grandchildren but not deeper;
// MaxSpawns bounds one run's total spawn requests. Together they bound the
// whole tree. Exceeding a cap is a pipeline DENY — a model-visible result,
// never a crash.
const (
	DefaultMaxSpawnDepth = 2
	DefaultMaxSpawns     = 8
)

// SpawnGate denies agent-launching effects (spawn_agent and handoff_agent —
// both start a child run, S5.3/S5.4) past the depth or fan-out cap. It
// ignores every other effect.
type SpawnGate struct {
	MaxDepth  int // zero → DefaultMaxSpawnDepth
	MaxSpawns int // zero → DefaultMaxSpawns
}

func (g *SpawnGate) Name() string { return "spawn" }

func (g *SpawnGate) Check(_ context.Context, eff Effect) Decision {
	if eff.ToolName != "spawn_agent" && eff.ToolName != "handoff_agent" {
		return Allow
	}
	maxDepth := g.MaxDepth
	if maxDepth == 0 {
		maxDepth = DefaultMaxSpawnDepth
	}
	maxSpawns := g.MaxSpawns
	if maxSpawns == 0 {
		maxSpawns = DefaultMaxSpawns
	}
	if eff.SpawnDepth >= maxDepth {
		return Deny("agent tree depth limit reached")
	}
	if eff.SpawnCount >= maxSpawns {
		return Deny("spawn fan-out limit reached for this run")
	}
	return Allow
}
