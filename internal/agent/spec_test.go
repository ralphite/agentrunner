package agent

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestLoadSpecErrors(t *testing.T) {
	files, err := filepath.Glob("testdata/spec_errors/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no error cases found")
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".yaml")
		t.Run(name, func(t *testing.T) {
			_, err := LoadSpec(f)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			golden := filepath.Join("testdata", "spec_errors", name+".golden")
			got := err.Error() + "\n"
			if *update {
				if werr := os.WriteFile(golden, []byte(got), 0o644); werr != nil {
					t.Fatal(werr)
				}
			}
			want, rerr := os.ReadFile(golden)
			if rerr != nil {
				t.Fatalf("missing golden (run with -update): %v", rerr)
			}
			if got != string(want) {
				t.Errorf("error mismatch\n got: %q\nwant: %q", got, string(want))
			}
		})
	}
}

func TestLoadSpecValid(t *testing.T) {
	spec, err := LoadSpec("testdata/valid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "hello" {
		t.Errorf("name = %q", spec.Name)
	}
	if spec.Model.Provider != "" || spec.Model.ID != "" {
		t.Errorf("definition unexpectedly resolved a model = %+v", spec.Model)
	}
	if spec.MaxGenerationSteps != DefaultMaxGenerationSteps {
		t.Errorf("max_generation_steps default = %d, want %d", spec.MaxGenerationSteps, DefaultMaxGenerationSteps)
	}
	if len(spec.Tools) != 3 {
		t.Errorf("tools = %v", spec.Tools)
	}
	if spec.AgentWorkspace != "isolated" {
		t.Errorf("agent_workspace default = %q, want isolated", spec.AgentWorkspace)
	}
}

func TestLoadSpecMCPTransports(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.yaml")
	raw := `name: mcp-agent
system_prompt: test
mcp:
  - name: local
    transport: stdio
    command: [mcp-server]
    env_from: {TOKEN: MCP_TOKEN}
    allowed_tools: [lookup]
  - name: remote
    transport: http
    url: https://example.test/mcp
    headers_from_env: {X-Tenant: MCP_TENANT}
    oauth: {access_token_env: MCP_ACCESS_TOKEN}
`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := LoadSpec(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.MCP) != 2 || spec.MCP[0].Command[0] != "mcp-server" || spec.MCP[1].OAuth.AccessTokenEnv != "MCP_ACCESS_TOKEN" {
		t.Fatalf("mcp config = %+v", spec.MCP)
	}
}

// sandbox.env_passthrough parses, and sandbox-critical names are rejected
// at load time (audit-0718 P0-2).
func TestLoadSpecSandboxEnvPassthrough(t *testing.T) {
	write := func(body string) (*AgentSpec, error) {
		path := filepath.Join(t.TempDir(), "spec.yaml")
		raw := "name: sbx\nsystem_prompt: test\n" + body
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		return LoadSpec(path)
	}
	spec, err := write("sandbox: {env_passthrough: [GEMINI_API_KEY, MY_TOKEN]}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Sandbox.EnvPassthrough) != 2 || spec.Sandbox.EnvPassthrough[0] != "GEMINI_API_KEY" {
		t.Fatalf("env_passthrough = %v", spec.Sandbox.EnvPassthrough)
	}
	for _, bad := range []string{"HOME", "XDG_DATA_HOME", "TMPDIR"} {
		if _, err := write("sandbox: {env_passthrough: [" + bad + "]}\n"); err == nil ||
			!strings.Contains(err.Error(), "sandbox-critical") {
			t.Fatalf("%s accepted or wrong error: %v", bad, err)
		}
	}
}

func TestLoadSpecPromptFile(t *testing.T) {
	spec, err := LoadSpec("testdata/valid_file.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if want := "You are a test agent.\n"; spec.SystemPrompt != want {
		t.Errorf("resolved prompt = %q, want %q", spec.SystemPrompt, want)
	}
	if spec.SystemPromptFile != "" {
		t.Errorf("SystemPromptFile should be cleared after resolution, got %q", spec.SystemPromptFile)
	}
}

func TestSpecRejectsBypassMode(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/spec.yaml"
	if err := os.WriteFile(path, []byte("name: x\nsystem_prompt: hi\nmode: bypass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSpec(path)
	if err == nil || !strings.Contains(err.Error(), "bypass cannot be set from a spec") {
		t.Fatalf("err = %v, want spec-bypass rejection", err)
	}
}

func TestBindModelValidatesResolvedBudget(t *testing.T) {
	spec := &AgentSpec{Name: "x", SystemPrompt: "hi", Budget: BudgetSpec{MaxTotalTokens: 5000}}
	err := BindModel(spec, ModelSpec{Provider: "gemini", ID: "m", MaxTokens: 6000}, "x.yaml")
	if err == nil || !strings.Contains(err.Error(), "below the resolved per-turn output cap") {
		t.Fatalf("BindModel error = %v", err)
	}
}

func TestBindModelCopiesAgentContextPolicy(t *testing.T) {
	spec := &AgentSpec{
		Name:                 "x",
		SystemPrompt:         "hi",
		CompactAtTokens:      12000,
		MicrocompactAtTokens: 9000,
	}
	model := ModelSpec{Provider: "gemini", ID: "m", MaxTokens: 10000}
	if err := BindModel(spec, model, "x.yaml"); err != nil {
		t.Fatal(err)
	}
	if spec.Model.CompactAtTokens != 12000 || spec.Model.MicrocompactAtTokens != 9000 {
		t.Fatalf("context policy not copied into effective spec: %+v", spec.Model)
	}
}
