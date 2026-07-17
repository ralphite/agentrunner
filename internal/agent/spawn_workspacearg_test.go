package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSpawnRejectsWorkspaceArg pins that a spawn_agent call trying to set the
// child's workspace via an arg is refused with guidance, instead of having its
// intent silently dropped (QA Wave2 erin-01). The child's workspace is fixed
// by the sub-agent spec's agent_workspace field.
func TestSpawnRejectsWorkspaceArg(t *testing.T) {
	l := &Loop{Spec: &AgentSpec{Agents: []string{"worker"}}}
	for _, key := range []string{"workspace", "agent_workspace", "shared", "isolated"} {
		args, _ := json.Marshal(map[string]any{"agent": "worker", "prompt": "go", key: "shared"})
		_, _, _, problem := l.resolveSpawnTarget("spawn_agent", args)
		if problem == "" || !strings.Contains(problem, key) {
			t.Errorf("arg %q: expected a problem naming it, got %q", key, problem)
		}
		if !strings.Contains(problem, "agent_workspace") {
			t.Errorf("arg %q: problem should point at the spec's agent_workspace, got %q", key, problem)
		}
	}
	// A normal spawn (no workspace arg) must still be accepted this far — it
	// fails only later on the missing sub-spec resolver, not on arg parsing.
	ok, _ := json.Marshal(map[string]any{"agent": "worker", "prompt": "go"})
	if _, _, _, problem := l.resolveSpawnTarget("spawn_agent", ok); strings.Contains(problem, "not a spawn arg") {
		t.Errorf("a plain spawn was wrongly rejected as a bad arg: %q", problem)
	}
}
