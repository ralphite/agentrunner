package agent

import "testing"

// writeTools that must NEVER appear on a built-in agent — they carry side
// effects. The whole safety argument for shipping explore/plan open is that
// their tool face is read-only.
var forbiddenBuiltinTools = map[string]bool{
	"edit_file":    true,
	"write_file":   true,
	"bash":         true,
	"apply_patch":  true,
	"spawn_agent":  true,
	"send_message": true,
}

// TestBuiltinSpecLoads confirms every shipped built-in loads, carries a
// description (a parent's agents directory shows it), and exposes only
// read-only tools.
func TestBuiltinSpecLoads(t *testing.T) {
	for _, name := range []string{"explore", "plan"} {
		t.Run(name, func(t *testing.T) {
			spec, ok := BuiltinSpec(name)
			if !ok {
				t.Fatalf("BuiltinSpec(%q) not found", name)
			}
			if spec.Name != name {
				t.Errorf("Name = %q, want %q", spec.Name, name)
			}
			if spec.Description == "" {
				t.Error("built-in agent must carry a Description (shown in the parent's agents directory)")
			}
			if len(spec.Tools) == 0 {
				t.Fatal("built-in agent must declare tools")
			}
			for _, tool := range spec.Tools {
				if forbiddenBuiltinTools[tool] {
					t.Errorf("built-in %q exposes side-effecting tool %q — must be read-only", name, tool)
				}
			}
			// LoadSpec-equivalent defaults must be applied (the embed path
			// skips LoadSpec).
			if spec.MaxGenerationSteps != DefaultMaxGenerationSteps {
				t.Errorf("MaxGenerationSteps = %d, want default %d", spec.MaxGenerationSteps, DefaultMaxGenerationSteps)
			}
			if spec.Model.MaxTokens != DefaultMaxTokens {
				t.Errorf("Model.MaxTokens = %d, want default %d", spec.Model.MaxTokens, DefaultMaxTokens)
			}
			if spec.AgentWorkspace != "isolated" {
				t.Errorf("AgentWorkspace = %q, want isolated", spec.AgentWorkspace)
			}
			// A shipped spec must pass the same validation a workspace spec does.
			if err := spec.validate("builtin/" + name + ".yaml"); err != nil {
				t.Errorf("built-in %q fails validate: %v", name, err)
			}
		})
	}
}

// TestBuiltinSpecUnknown confirms unknown names are not built-in (so the
// resolver falls through to sibling files) and that each call returns a fresh
// copy the caller can mutate (model inheritance rewrites spec.Model).
func TestBuiltinSpecUnknown(t *testing.T) {
	if _, ok := BuiltinSpec("nonexistent"); ok {
		t.Error("BuiltinSpec(nonexistent) = ok, want not found")
	}
	if IsBuiltinAgent("nonexistent") {
		t.Error("IsBuiltinAgent(nonexistent) = true")
	}
	if !IsBuiltinAgent("explore") {
		t.Error("IsBuiltinAgent(explore) = false")
	}
	a, _ := BuiltinSpec("explore")
	b, _ := BuiltinSpec("explore")
	if a == b {
		t.Error("BuiltinSpec returns a shared pointer; each call must be an independent copy")
	}
	a.Model.Provider = "mutated"
	if b.Model.Provider == "mutated" {
		t.Error("mutating one BuiltinSpec result leaked into another")
	}
}
