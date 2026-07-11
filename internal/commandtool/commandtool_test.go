package commandtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, dir, file, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Manifest parsing: a well-formed manifest resolves; strict decoding rejects
// unknown keys; missing name/command and a mcp__ name are refused.
func TestParseAndResolve(t *testing.T) {
	good := `{"name":"deploy","description":"ship it","command":"./deploy.sh","timeout_s":45,
		"params":{"type":"object","properties":{"target":{"type":"string"}}}}`
	m, err := parseManifest([]byte(good))
	if err != nil {
		t.Fatalf("parse good: %v", err)
	}
	tool, err := resolve(m, "user")
	if err != nil {
		t.Fatalf("resolve good: %v", err)
	}
	if tool.Name != "deploy" || tool.Command != "./deploy.sh" || tool.TimeoutS != 45 || tool.Source != "user" {
		t.Fatalf("resolved wrong: %+v", tool)
	}
	if !strings.Contains(string(tool.InputSchema), "target") {
		t.Errorf("schema lost params: %s", tool.InputSchema)
	}

	// Strict decode: an unknown key is a hard error (a typo must fail loudly).
	if _, err := parseManifest([]byte(`{"name":"x","command":"y","tmeout_s":1}`)); err == nil {
		t.Error("unknown key must be rejected")
	}

	for _, bad := range []struct{ name, json string }{
		{"missing name", `{"command":"x"}`},
		{"missing command", `{"name":"x"}`},
		{"bad name chars", `{"name":"bad name","command":"x"}`},
		{"mcp prefix", `{"name":"mcp__x","command":"y"}`},
		{"non-object params", `{"name":"x","command":"y","params":[1,2]}`},
		{"negative timeout", `{"name":"x","command":"y","timeout_s":-3}`},
	} {
		m, perr := parseManifest([]byte(bad.json))
		if perr == nil {
			if _, rerr := resolve(m, "user"); rerr == nil {
				t.Errorf("%s: expected rejection", bad.name)
			}
		}
	}
}

// A parameterless manifest gets a valid default object schema, and an
// over-large timeout is clamped.
func TestResolveDefaultsAndClamp(t *testing.T) {
	m, err := parseManifest([]byte(`{"name":"ping","command":"echo pong","timeout_s":100000}`))
	if err != nil {
		t.Fatal(err)
	}
	tool, err := resolve(m, "user")
	if err != nil {
		t.Fatal(err)
	}
	if tool.TimeoutS != MaxTimeoutS {
		t.Errorf("timeout not clamped: %d", tool.TimeoutS)
	}
	if !strings.Contains(string(tool.InputSchema), `"type":"object"`) {
		t.Errorf("default schema missing: %s", tool.InputSchema)
	}
}

// User-layer tools always load; a malformed manifest is skipped with a
// warning, not a failure.
func TestDiscoverUserLayer(t *testing.T) {
	userDir := filepath.Join(t.TempDir(), "tools")
	writeManifest(t, userDir, "a.json", `{"name":"alpha","command":"echo a"}`)
	writeManifest(t, userDir, "b.json", `{"name":"beta","command":"echo b"}`)
	writeManifest(t, userDir, "broken.json", `{ not json`)

	tools, warnings := Discover(userDir, filepath.Join(t.TempDir(), "absent"), false, nil)
	if len(tools) != 2 || tools[0].Name != "alpha" || tools[1].Name != "beta" {
		t.Fatalf("tools = %+v", tools)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "broken.json") {
		t.Fatalf("warnings = %v", warnings)
	}
	for _, tl := range tools {
		if tl.Source != "user" {
			t.Errorf("wrong source: %+v", tl)
		}
	}
}

// The trust gate (决策 #19): project-layer tools load ONLY when the workspace
// is trusted. Untrusted → not loaded, and the presence of project manifests
// yields a warning pointing at `agentrunner trust`.
func TestDiscoverProjectTrustGate(t *testing.T) {
	userDir := filepath.Join(t.TempDir(), "userless") // empty/absent
	projectDir := filepath.Join(t.TempDir(), "proj")
	writeManifest(t, projectDir, "deploy.json", `{"name":"deploy","command":"./ci.sh"}`)

	// Untrusted: the project tool must NOT load.
	tools, warnings := Discover(userDir, projectDir, false, nil)
	if len(tools) != 0 {
		t.Fatalf("untrusted project tool loaded: %+v", tools)
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "untrusted") || !strings.Contains(joined, "trust") {
		t.Fatalf("missing untrusted warning: %v", warnings)
	}

	// Trusted: it loads, tagged as project.
	tools, _ = Discover(userDir, projectDir, true, nil)
	if len(tools) != 1 || tools[0].Name != "deploy" || tools[0].Source != "project" {
		t.Fatalf("trusted project tool = %+v", tools)
	}
}

// A name colliding with a built-in tool is refused (the built-in wins).
func TestDiscoverBuiltinCollisionRejected(t *testing.T) {
	userDir := filepath.Join(t.TempDir(), "tools")
	writeManifest(t, userDir, "bash.json", `{"name":"bash","command":"echo nope"}`)
	writeManifest(t, userDir, "ok.json", `{"name":"okay","command":"echo ok"}`)

	reserved := map[string]bool{"bash": true, "read_file": true}
	tools, warnings := Discover(userDir, "", false, reserved)
	if len(tools) != 1 || tools[0].Name != "okay" {
		t.Fatalf("built-in collision not rejected: %+v", tools)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "built-in") {
		t.Fatalf("missing collision warning: %v", warnings)
	}
}

// User precedence: when both layers define the same name, the user layer wins
// and the (trusted) project one is dropped with a shadow warning.
func TestDiscoverUserBeatsProject(t *testing.T) {
	userDir := filepath.Join(t.TempDir(), "user")
	projectDir := filepath.Join(t.TempDir(), "proj")
	writeManifest(t, userDir, "d.json", `{"name":"dup","command":"USER"}`)
	writeManifest(t, projectDir, "d.json", `{"name":"dup","command":"PROJECT"}`)
	writeManifest(t, projectDir, "extra.json", `{"name":"extra","command":"echo x"}`)

	tools, warnings := Discover(userDir, projectDir, true, nil)
	byName := map[string]Tool{}
	for _, tl := range tools {
		byName[tl.Name] = tl
	}
	if byName["dup"].Command != "USER" || byName["dup"].Source != "user" {
		t.Fatalf("user did not win: %+v", byName["dup"])
	}
	if _, ok := byName["extra"]; !ok {
		t.Fatalf("non-colliding project tool dropped: %+v", tools)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "shadowed") {
		t.Fatalf("missing shadow warning: %v", warnings)
	}
}

// A missing directory is "no tools", never an error.
func TestDiscoverMissingDirs(t *testing.T) {
	tools, warnings := Discover(filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "gone"), true, nil)
	if len(tools) != 0 || len(warnings) != 0 {
		t.Fatalf("tools=%+v warnings=%v", tools, warnings)
	}
}
