package agentcatalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListShippedCatalog(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	entries, err := List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"dev", "lead", "auditor", "reviewer", "chat", "worker", "explore", "plan"}
	if len(entries) != len(want) {
		t.Fatalf("entries=%d, want %d", len(entries), len(want))
	}
	for i, name := range want {
		if entries[i].Name != name || entries[i].Source != "shipped" {
			t.Fatalf("entry[%d]=%+v, want shipped %s", i, entries[i], name)
		}
		if stringContainsModel(entries[i].YAML) {
			t.Fatalf("shipped Agent %s still contains model", name)
		}
	}
}

func TestUserAgentOverridesShippedAndAddsNames(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	dir := filepath.Join(root, "agentrunner", "agents")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, prompt string) {
		t.Helper()
		body := "name: " + name + "\ndescription: custom\nsystem_prompt: " + prompt + "\ntools: []\n"
		if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("dev", "overridden")
	write("research", "research")

	dev, path, err := Resolve("dev")
	if err != nil {
		t.Fatal(err)
	}
	if dev.SystemPrompt != "overridden" || filepath.Base(path) != "dev.yaml" {
		t.Fatalf("user override not resolved: spec=%+v path=%s", dev, path)
	}
	entries, err := List()
	if err != nil {
		t.Fatal(err)
	}
	var sources = map[string]string{}
	for _, entry := range entries {
		sources[entry.Name] = entry.Source
	}
	if sources["dev"] != "user" || sources["research"] != "user" {
		t.Fatalf("sources=%v", sources)
	}
}

func TestAgentModelFieldFailsWithMigrationHint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.yaml")
	if err := os.WriteFile(path, []byte("name: old\nmodel: {provider: gemini, id: x}\nsystem_prompt: old\ntools: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Resolve(path); err == nil || !stringContains(err.Error(), "session input") {
		t.Fatalf("Resolve legacy model error=%v", err)
	}
}

func TestResolveExplicitPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "path-agent.yaml")
	if err := os.WriteFile(path, []byte("name: path-agent\nsystem_prompt: explicit\ntools: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec, source, err := Resolve(path)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "path-agent" || source != path {
		t.Fatalf("Resolve(path) = name %q, source %q", spec.Name, source)
	}
}

func stringContainsModel(s string) bool {
	return stringContains(s, "\nmodel:") || stringContains(s, "model:") && len(s) >= 6 && s[:6] == "model:"
}
func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
