package pipeline

import (
	"context"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
)

func TestSpawnGateDefaultsUnlimitedAndHonorsExplicitCaps(t *testing.T) {
	eff := Effect{ToolName: "spawn_agent", SpawnDepth: 100, SpawnCount: 1000}
	if got := (&SpawnGate{}).Check(context.Background(), eff); got.Action != event.VerdictAllow {
		t.Fatalf("default spawn gate = %+v, want unlimited allow", got)
	}
	if got := (&SpawnGate{MaxDepth: 2}).Check(context.Background(), eff); got.Action != event.VerdictDeny {
		t.Fatalf("explicit depth cap = %+v, want deny", got)
	}
	if got := (&SpawnGate{MaxSpawns: 8}).Check(context.Background(), eff); got.Action != event.VerdictDeny {
		t.Fatalf("explicit fan-out cap = %+v, want deny", got)
	}
}
