package agent

import (
	"path/filepath"
	"testing"
)

// The Web UI ships these files as raw YAML. Loading every one through the
// runtime's strict parser keeps the editable source of truth honest: a typo in
// a prompt/tool/config edit fails CI instead of surfacing only when a user
// starts a session.
func TestShippedWebUIAgentSpecsLoad(t *testing.T) {
	dir := filepath.Join("..", "..", "webui", "frontend", "src", "agents")
	for _, name := range []string{"dev", "lead", "auditor", "reviewer", "chat", "worker"} {
		t.Run(name, func(t *testing.T) {
			spec, err := LoadSpec(filepath.Join(dir, name+".yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if spec.Name != name {
				t.Fatalf("name = %q, want %q", spec.Name, name)
			}
		})
	}
}
