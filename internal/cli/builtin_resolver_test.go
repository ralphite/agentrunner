package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/agent"
)

// writeParentSpec drops a minimal, valid parent spec with a distinctive model
// so tests can assert inheritance overrode the built-in default.
func writeParentSpec(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "parent.yaml")
	body := "" +
		"name: parent\n" +
		"system_prompt: You are the parent.\n" +
		"tools: [read_file]\n" +
		"agents: [explore]\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestResolverPrefersBuiltinAndInheritsModel: resolving a built-in name yields
// the shipped read-only spec, but with the PARENT's model (not the built-in's
// gemini default) so it runs on the provider the user chose.
func TestResolverPrefersBuiltinAndInheritsModel(t *testing.T) {
	dir := t.TempDir()
	parent := writeParentSpec(t, dir)
	model := agent.ModelSpec{Provider: "anthropic", ID: "claude-sonnet-5", MaxTokens: 10240}
	resolve := siblingSpecResolver(parent, model, false)

	for _, name := range []string{"explore", "plan"} {
		spec, err := resolve(name)
		if err != nil {
			t.Fatalf("resolve(%q): %v", name, err)
		}
		if spec.Name != name {
			t.Errorf("resolved Name = %q, want %q", spec.Name, name)
		}
		if spec.Model.Provider != "anthropic" || spec.Model.ID != "claude-sonnet-5" {
			t.Errorf("model not inherited from parent: got %s/%s, want anthropic/claude-sonnet-5",
				spec.Model.Provider, spec.Model.ID)
		}
		// Still the read-only built-in — inheritance touches only the model.
		for _, tool := range spec.Tools {
			if tool == "edit_file" || tool == "bash" || tool == "write_file" {
				t.Errorf("built-in %q leaked a write tool %q after inheritance", name, tool)
			}
		}
	}
}

// TestResolverExplicitSiblingCanOverrideShipped: an explicitly selected file
// definition may keep its own sibling catalog. The user named that file, so
// this is an explicit override rather than silent repository discovery.
func TestResolverExplicitSiblingCanOverrideShipped(t *testing.T) {
	dir := t.TempDir()
	parent := writeParentSpec(t, dir)
	// A malicious/careless sibling explore.yaml with a write tool face.
	sibling := "" +
		"name: explore\n" +
		"system_prompt: rogue\n" +
		"tools: [read_file, edit_file, bash]\n"
	if err := os.WriteFile(filepath.Join(dir, "explore.yaml"), []byte(sibling), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := siblingSpecResolver(parent, agent.ModelSpec{Provider: "gemini", ID: "m", MaxTokens: 10240}, false)("explore")
	if err != nil {
		t.Fatal(err)
	}
	if spec.SystemPrompt != "rogue" {
		t.Fatalf("explicit sibling did not override shipped explore: %+v", spec)
	}
}

// TestResolverFallsBackToSibling: an unknown (non-built-in) name resolves to
// the sibling <name>.yaml, preserving the existing S5.3 behavior.
func TestResolverFallsBackToSibling(t *testing.T) {
	dir := t.TempDir()
	parent := writeParentSpec(t, dir)
	custom := "" +
		"name: custom\n" +
		"system_prompt: You are custom.\n" +
		"tools: [read_file]\n"
	if err := os.WriteFile(filepath.Join(dir, "custom.yaml"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	resolve := siblingSpecResolver(parent, agent.ModelSpec{Provider: "gemini", ID: "gemini-flash-latest", MaxTokens: 10240}, false)

	spec, err := resolve("custom")
	if err != nil {
		t.Fatalf("resolve(custom): %v", err)
	}
	if spec.Name != "custom" {
		t.Errorf("Name = %q, want custom", spec.Name)
	}
	// Sibling keeps its own model — no built-in inheritance path.
	if spec.Model.Provider != "gemini" {
		t.Errorf("sibling model overwritten: got %q", spec.Model.Provider)
	}

	if _, err := resolve("nonexistent"); err == nil {
		t.Error("resolve(nonexistent) = nil error, want not-found")
	}
}

func TestResolverAllowsLegacySiblingOnlyForFrozenSession(t *testing.T) {
	dir := t.TempDir()
	parent := writeParentSpec(t, dir)
	legacy := "" +
		"name: legacy\n" +
		"model: {provider: gemini, id: old-model}\n" +
		"system_prompt: legacy child\n"
	if err := os.WriteFile(filepath.Join(dir, "legacy.yaml"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	model := agent.ModelSpec{Provider: "anthropic", ID: "new-model", MaxTokens: 10240}

	if _, err := siblingSpecResolver(parent, model, false)("legacy"); err == nil {
		t.Fatal("new launch accepted a legacy Agent model field")
	}
	spec, err := siblingSpecResolver(parent, model, true)("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Model.Provider != "anthropic" || spec.Model.ID != "new-model" {
		t.Fatalf("legacy child model = %s/%s, want frozen parent model", spec.Model.Provider, spec.Model.ID)
	}
}
