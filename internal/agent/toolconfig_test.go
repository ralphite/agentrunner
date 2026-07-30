package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func toolConfigCall(t *testing.T, args map[string]any) (raw json.RawMessage, isErr bool) {
	t.Helper()
	b, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	res := runToolConfigTool(b)
	return res.Payload, res.IsError
}

const validToolManifest = `{"name":"run-lint","description":"Run the linter.","command":"./scripts/lint.sh","timeout_s":60,"params":{"type":"object","properties":{"paths":{"type":"array","items":{"type":"string"}}}}}`

func TestToolConfigSaveListRemove(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)

	// save
	raw, isErr := toolConfigCall(t, map[string]any{"action": "save", "name": "run-lint", "manifest": validToolManifest})
	if isErr {
		t.Fatalf("save failed: %s", raw)
	}
	target := filepath.Join(cfg, "agentrunner", "tools", "run-lint.json")
	if disk, err := os.ReadFile(target); err != nil || string(disk) != validToolManifest {
		t.Fatalf("manifest not landed verbatim: %v %q", err, disk)
	}

	// clobber refused without overwrite; allowed with it
	if raw, isErr = toolConfigCall(t, map[string]any{"action": "save", "name": "run-lint", "manifest": validToolManifest}); !isErr ||
		!strings.Contains(string(raw), "already exists") {
		t.Fatalf("clobber not refused: %s", raw)
	}
	updated := strings.Replace(validToolManifest, "Run the linter.", "Run the linter fast.", 1)
	if raw, isErr = toolConfigCall(t, map[string]any{"action": "save", "name": "run-lint", "manifest": updated, "overwrite": true}); isErr {
		t.Fatalf("overwrite failed: %s", raw)
	}

	// list
	raw, isErr = toolConfigCall(t, map[string]any{"action": "list"})
	if isErr {
		t.Fatalf("list failed: %s", raw)
	}
	var listed struct {
		Tools []struct{ Name, Description string } `json:"tools"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 1 || listed.Tools[0].Name != "run-lint" ||
		listed.Tools[0].Description != "Run the linter fast." {
		t.Fatalf("list = %+v", listed.Tools)
	}

	// remove
	if raw, isErr = toolConfigCall(t, map[string]any{"action": "remove", "name": "run-lint"}); isErr {
		t.Fatalf("remove failed: %s", raw)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("manifest not removed")
	}
	if raw, isErr = toolConfigCall(t, map[string]any{"action": "remove", "name": "run-lint"}); !isErr ||
		!strings.Contains(string(raw), "does not exist") {
		t.Fatalf("missing remove not surfaced: %s", raw)
	}
}

func TestToolConfigSaveRejectsBadManifests(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)

	cases := []struct {
		name     string
		manifest string
		want     string
	}{
		{"bash", `{"name":"bash","command":"echo hi"}`, "built-in"},
		{"typo-field", `{"name":"typo-field","cmd":"echo hi"}`, "unknown field"},
		{"no-command", `{"name":"no-command"}`, "command is required"},
		{"bad-params", `{"name":"bad-params","command":"x","params":[1]}`, "params must be a JSON object"},
	}
	for _, c := range cases {
		raw, isErr := toolConfigCall(t, map[string]any{"action": "save", "name": c.name, "manifest": c.manifest})
		if !isErr || !strings.Contains(string(raw), c.want) {
			t.Fatalf("%s: want %q in error, got %s", c.name, c.want, raw)
		}
	}
	// Nothing landed, and no temp residue either.
	entries, _ := os.ReadDir(filepath.Join(cfg, "agentrunner", "tools"))
	if len(entries) != 0 {
		t.Fatalf("rejected manifests left files: %v", entries)
	}
	// Name/manifest mismatch.
	if raw, isErr := toolConfigCall(t, map[string]any{"action": "save", "name": "other", "manifest": validToolManifest}); !isErr ||
		!strings.Contains(string(raw), "must equal") {
		t.Fatalf("name mismatch not rejected: %s", raw)
	}
}
