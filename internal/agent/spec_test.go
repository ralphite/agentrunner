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
	if spec.Model.Provider != "gemini" || spec.Model.ID != "gemini-flash-latest" {
		t.Errorf("model = %+v", spec.Model)
	}
	if spec.MaxGenerationSteps != DefaultMaxGenerationSteps {
		t.Errorf("max_generation_steps default = %d, want %d", spec.MaxGenerationSteps, DefaultMaxGenerationSteps)
	}
	if spec.Model.MaxTokens != DefaultMaxTokens {
		t.Errorf("max_tokens default = %d, want %d", spec.Model.MaxTokens, DefaultMaxTokens)
	}
	if len(spec.Tools) != 3 {
		t.Errorf("tools = %v", spec.Tools)
	}
}

func TestLoadSpecMCPTransports(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.yaml")
	raw := `name: mcp-agent
model: {provider: scripted, id: model}
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
	if err := os.WriteFile(path, []byte("name: x\nmodel: {provider: scripted, id: y}\nsystem_prompt: hi\nmode: bypass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSpec(path)
	if err == nil || !strings.Contains(err.Error(), "bypass cannot be set from a spec") {
		t.Fatalf("err = %v, want spec-bypass rejection", err)
	}
}
