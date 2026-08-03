package agent

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
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
	if spec.MaxGenerationSteps != 0 {
		t.Errorf("max_generation_steps default = %d, want 0 (unlimited)", spec.MaxGenerationSteps)
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

// Spawn-by-default (2026-08-02): a spec silent on agents gets the builtin
// directory; an explicit `agents: []` opts out; agents_dynamic keeps its
// declared face; budgets stay off unless asked for.
func TestSpawnByDefault(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	silent, err := LoadSpec(write("silent.yaml", "name: silent\nsystem_prompt: x\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(silent.Agents, DefaultAgents) {
		t.Errorf("silent spec agents = %v, want default %v", silent.Agents, DefaultAgents)
	}

	optOut, err := LoadSpec(write("optout.yaml", "name: optout\nsystem_prompt: x\nagents: []\n"))
	if err != nil {
		t.Fatal(err)
	}
	if optOut.Agents == nil || len(optOut.Agents) != 0 {
		t.Errorf("explicit agents: [] should stay empty, got %v", optOut.Agents)
	}

	dyn, err := LoadSpec(write("dyn.yaml", "name: dyn\nsystem_prompt: x\nagents_dynamic: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if dyn.Agents != nil {
		t.Errorf("agents_dynamic spec should not gain a static directory, got %v", dyn.Agents)
	}

	// Builtin specs share the default: chat (silent on agents) gains the
	// directory; dev keeps its own declared whitelist.
	chat, ok := BuiltinSpec("chat")
	if !ok {
		t.Fatal("chat builtin missing")
	}
	if !slices.Equal(chat.Agents, DefaultAgents) {
		t.Errorf("chat agents = %v, want default %v", chat.Agents, DefaultAgents)
	}
	dev, ok := BuiltinSpec("dev")
	if !ok {
		t.Fatal("dev builtin missing")
	}
	if slices.Equal(dev.Agents, DefaultAgents) && len(dev.Agents) == 0 {
		t.Errorf("dev should keep its declared whitelist, got %v", dev.Agents)
	}
}

// Budget-off-by-default: max_generation_steps 0 means unlimited — a fresh
// input deep into a very long session still gets its turn instead of the
// visible truncation the same shape hits under a positive cap.
func TestDecideUnlimitedGenerationSteps(t *testing.T) {
	mk := func() state.State {
		s := state.New()
		s.Session.Status = state.StatusRunning
		s.Session.GenStep = 500
		s.Session.LastAssistantGenStep = 500
		s.Session.LastInputGenStep = 0
		msgs := []provider.Message{{Role: provider.RoleUser,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "q"}}}}
		for i := 0; i < 500; i++ {
			msgs = append(msgs, provider.Message{Role: provider.RoleAssistant,
				Parts: []provider.Part{{Kind: provider.PartText, Text: "a"}}})
		}
		msgs = append(msgs, provider.Message{Role: provider.RoleUser,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "next"}}})
		s.Conversation.Messages = msgs
		return s
	}
	if act := decide(mk(), 10); act.kind != doTruncate {
		t.Fatalf("positive cap control: kind = %v, want truncate", act.kind)
	}
	if act := decide(mk(), 0); act.kind != doTurn {
		t.Fatalf("maxGenerationSteps=0 must mean unlimited (doTurn), got %v", act.kind)
	}
}
