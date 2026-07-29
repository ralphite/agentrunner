package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func saveAgentCall(t *testing.T, args map[string]any) (payload map[string]string, isErr bool) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	res := runSaveAgentTool(raw)
	out := map[string]string{}
	if err := json.Unmarshal(res.Payload, &out); err != nil {
		t.Fatalf("payload not an object: %s", res.Payload)
	}
	return out, res.IsError
}

const validAgentYAML = "name: release-drafter\ndescription: Drafts release notes.\nsystem_prompt: Draft release notes from the git log.\ntools: [read_file, grep, bash]\n"

func TestSaveAgentWritesUserCatalog(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)

	out, isErr := saveAgentCall(t, map[string]any{"name": "release-drafter", "yaml": validAgentYAML})
	if isErr {
		t.Fatalf("save failed: %v", out)
	}
	target := filepath.Join(cfg, "agentrunner", "agents", "release-drafter.yaml")
	if out["path"] != target {
		t.Fatalf("path = %q, want %q", out["path"], target)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != validAgentYAML {
		t.Fatalf("landed content differs: %q", raw)
	}
	// No temp residue: the directory holds exactly the one saved file.
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("agents dir has %d entries, want 1", len(entries))
	}
}

func TestSaveAgentRejectsInvalidSpecWithoutLanding(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)

	out, isErr := saveAgentCall(t, map[string]any{"name": "broken", "yaml": "name: broken\ntools: [read_file]\n"})
	if !isErr {
		t.Fatalf("invalid spec accepted: %v", out)
	}
	// The loader error names the would-be file, not the temp path.
	if !strings.Contains(out["error"], "broken.yaml") || strings.Contains(out["error"], ".tmp-") {
		t.Fatalf("error should reference broken.yaml, got %q", out["error"])
	}
	if _, err := os.Stat(filepath.Join(cfg, "agentrunner", "agents", "broken.yaml")); !os.IsNotExist(err) {
		t.Fatal("invalid spec must not land on disk")
	}
	entries, _ := os.ReadDir(filepath.Join(cfg, "agentrunner", "agents"))
	if len(entries) != 0 {
		t.Fatalf("agents dir should be empty, has %d entries", len(entries))
	}
}

func TestSaveAgentRejectsNameMismatchAndBadNames(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)

	if out, isErr := saveAgentCall(t, map[string]any{"name": "other-name", "yaml": validAgentYAML}); !isErr ||
		!strings.Contains(out["error"], "must equal") {
		t.Fatalf("name mismatch not rejected: %v", out)
	}
	for _, bad := range []string{"../evil", "Evil", "a/b", ".hidden"} {
		if out, isErr := saveAgentCall(t, map[string]any{"name": bad, "yaml": validAgentYAML}); !isErr {
			t.Fatalf("bad name %q accepted: %v", bad, out)
		}
	}
}

func TestSaveAgentOverwriteSemantics(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)

	if out, isErr := saveAgentCall(t, map[string]any{"name": "release-drafter", "yaml": validAgentYAML}); isErr {
		t.Fatalf("first save failed: %v", out)
	}
	if out, isErr := saveAgentCall(t, map[string]any{"name": "release-drafter", "yaml": validAgentYAML}); !isErr ||
		!strings.Contains(out["error"], "already exists") {
		t.Fatalf("clobber without overwrite not rejected: %v", out)
	}
	updated := strings.Replace(validAgentYAML, "Draft release notes", "Draft crisp release notes", 1)
	if out, isErr := saveAgentCall(t, map[string]any{"name": "release-drafter", "yaml": updated, "overwrite": true}); isErr {
		t.Fatalf("overwrite failed: %v", out)
	}
	raw, err := os.ReadFile(filepath.Join(cfg, "agentrunner", "agents", "release-drafter.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "crisp") {
		t.Fatal("overwrite did not replace content")
	}
}

func TestSaveAgentRejectsModelField(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)

	yaml := "name: modelful\nsystem_prompt: hi\ntools: []\nmodel:\n  provider: gemini\n"
	if out, isErr := saveAgentCall(t, map[string]any{"name": "modelful", "yaml": yaml}); !isErr ||
		!strings.Contains(out["error"], "model is session input") {
		t.Fatalf("model field not rejected with migration hint: %v", out)
	}
}
