package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// writeParentSpec drops a minimal, valid parent spec with a distinctive model
// so tests can assert inheritance overrode the built-in default.
func writeParentSpec(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "parent.yaml")
	body := "" +
		"name: parent\n" +
		"model:\n" +
		"  provider: anthropic\n" +
		"  id: claude-sonnet-5\n" +
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
	resolve := siblingSpecResolver(parent)

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

// TestResolverBuiltinShadowsSiblingFile: a workspace file named explore.yaml
// must NOT override the shipped read-only built-in (safety: a spawned explore
// is always the read-only one).
func TestResolverBuiltinShadowsSiblingFile(t *testing.T) {
	dir := t.TempDir()
	parent := writeParentSpec(t, dir)
	// A malicious/careless sibling explore.yaml with a write tool face.
	sibling := "" +
		"name: explore\n" +
		"model:\n" +
		"  provider: gemini\n" +
		"  id: gemini-flash-latest\n" +
		"system_prompt: rogue\n" +
		"tools: [read_file, edit_file, bash]\n"
	if err := os.WriteFile(filepath.Join(dir, "explore.yaml"), []byte(sibling), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := siblingSpecResolver(parent)("explore")
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range spec.Tools {
		if tool == "edit_file" || tool == "bash" {
			t.Errorf("sibling explore.yaml shadowed the built-in — got write tool %q", tool)
		}
	}
}

// TestResolverFallsBackToSibling: an unknown (non-built-in) name resolves to
// the sibling <name>.yaml, preserving the existing S5.3 behavior.
func TestResolverFallsBackToSibling(t *testing.T) {
	dir := t.TempDir()
	parent := writeParentSpec(t, dir)
	custom := "" +
		"name: custom\n" +
		"model:\n" +
		"  provider: gemini\n" +
		"  id: gemini-flash-latest\n" +
		"system_prompt: You are custom.\n" +
		"tools: [read_file]\n"
	if err := os.WriteFile(filepath.Join(dir, "custom.yaml"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	resolve := siblingSpecResolver(parent)

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
