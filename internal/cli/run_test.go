package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
)

func writeSpec(t *testing.T, dir string) string {
	t.Helper()
	useScriptedDefaultModel(t, dir)
	path := filepath.Join(dir, "spec.yaml")
	spec := `name: t
system_prompt: help
tools: [read_file, edit_file, bash]
permissions:
  - { action: allow }
`
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func useScriptedDefaultModel(t *testing.T, dir string) {
	t.Helper()
	root := filepath.Join(dir, "config")
	t.Setenv("XDG_CONFIG_HOME", root)
	settings := filepath.Join(root, "agentrunner", "settings.yaml")
	if err := os.MkdirAll(filepath.Dir(settings), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte("default_model: { provider: scripted, id: x, effort: medium }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func scriptedFactory(fix scripted.Fixture) providerFactory {
	return func(_ context.Context, name string) (provider.Provider, error) {
		return scripted.New(fix), nil
	}
}

func TestRunAgentEndToEnd(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "f.txt"), []byte("small world"), 0o644); err != nil {
		t.Fatal(err)
	}

	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: "editing"},
			{ToolCall: &scripted.ToolCallEvent{Name: "edit_file", Args: map[string]any{
				"path": "f.txt", "old": "small", "new": "BIG"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done!"}, {Finish: "end_turn"}}},
	}}

	var out, errOut bytes.Buffer
	code := runAgent(runOptions{
		specPath:  writeSpec(t, dir),
		prompt:    "embiggen",
		workspace: ws,
		factory:   scriptedFactory(fix),
		stdout:    &out,
		stderr:    &errOut,
	})
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "done!") || !strings.Contains(out.String(), "edit_file") {
		t.Errorf("stdout = %q", out.String())
	}
	if !strings.Contains(errOut.String(), "run completed: 2 turns") {
		t.Errorf("stderr = %q", errOut.String())
	}

	content, _ := os.ReadFile(filepath.Join(ws, "f.txt"))
	if string(content) != "BIG world" {
		t.Errorf("file = %q", content)
	}

	// event log exists under the session dir
	matches, _ := filepath.Glob(filepath.Join(os.Getenv("XDG_DATA_HOME"), "agentrunner", "sessions", "*", "events.jsonl"))
	if len(matches) != 1 {
		t.Errorf("event logs = %v", matches)
	}
}

func TestRunAgentSpecErrorExits2(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runAgent(runOptions{
		specPath: "does-not-exist.yaml",
		prompt:   "x",
		factory:  scriptedFactory(scripted.Fixture{}),
		stdout:   &out, stderr: &errOut,
	})
	if code != ExitUsage {
		t.Fatalf("exit = %d", code)
	}
}

func TestRunCmdUsageErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runCmd([]string{"only-spec.yaml"}, false, "dev", &out, &errOut); code != ExitUsage {
		t.Errorf("missing prompt: exit = %d", code)
	}
	if code := runCmd([]string{"s.yaml", "prompt"}, true, "dev", &out, &errOut); code != ExitUsage {
		t.Errorf("record-fixture without -o: exit = %d", code)
	}
	// `run -o` is refused loudly instead of silently ignored (PLAN 5.4).
	errOut.Reset()
	if code := runCmd([]string{"-o", "f.json", "s.yaml", "prompt"}, false, "dev", &out, &errOut); code != ExitUsage {
		t.Errorf("run with -o: exit = %d, want ExitUsage", code)
	}
	if !strings.Contains(errOut.String(), "record-fixture") {
		t.Errorf("run -o refusal should point at record-fixture, got %q", errOut.String())
	}
	// A whitespace-only prompt is rejected like an empty one — it must not
	// create a junk session (QA Wave1 cli-life-09).
	for _, p := range []string{"", "   ", "\t\n"} {
		out.Reset()
		errOut.Reset()
		if code := runCmd([]string{"s.yaml", p}, false, "dev", &out, &errOut); code != ExitUsage {
			t.Errorf("blank prompt %q: exit = %d, want ExitUsage", p, code)
		}
	}
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("CLI_TEST_VAR=from-dotenv\n# comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLI_TEST_VAR", "") // registers cleanup; empty counts as unset for loadDotEnv
	_ = os.Unsetenv("CLI_TEST_VAR")
	loadDotEnv(path)
	if got := os.Getenv("CLI_TEST_VAR"); got != "from-dotenv" {
		t.Errorf("var = %q", got)
	}

	// existing values win
	t.Setenv("CLI_TEST_VAR", "already-set")
	loadDotEnv(path)
	if got := os.Getenv("CLI_TEST_VAR"); got != "already-set" {
		t.Errorf("var = %q, want already-set", got)
	}
}
