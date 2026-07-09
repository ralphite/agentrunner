package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// specTemplate is the commented example spec `agentrunner init` writes
// (INC-2 BB-me-3): the spec schema's discoverable form. It must always pass
// LoadSpec — TestInitSpecLoads pins that.
const specTemplate = `# agentrunner agent spec — the declarative definition of one agent.
# Required: name, model.provider, model.id, and exactly one of
# system_prompt / system_prompt_file. Everything else is optional.
name: my-agent

model:
  provider: gemini          # gemini | anthropic (needs GEMINI_API_KEY / ANTHROPIC_API_KEY)
  id: gemini-flash-latest   # any model id the provider serves
  # max_tokens: 8192        # per-turn output cap (default 8192)

system_prompt: >
  You are a helpful coding agent. Answer in plain text; use tools only
  when the task requires reading or changing files or running commands.
# system_prompt_file: prompt.md   # or load the prompt from a file (not both)

# Tools the agent may use; omit for a chat-only agent.
tools: [read_file, write_file, edit_file, bash, grep, glob, semantic_search]

# Heads-up: with NO permissions block below, edits (write_file/edit_file)
# and shell commands (bash) PAUSE for your approval every time — reads are
# free, side effects ask. Answer a pending ask with:
#   agentrunner approve <session> <id> approve|deny
# Uncomment permissions to pre-authorize what you trust (first match wins):
permissions:
  - { tool: read_file, action: allow }   # reads never touch anything
  # - { tool: bash, command: "git *", action: allow }  # trust git
  # - { action: allow }                  # trust everything (single-user dev box)

# --- optional ---------------------------------------------------------
# mode: plan                # default | plan | acceptEdits
# max_generation_steps: 40  # cap on model calls per turn
# budget:
#   max_total_tokens: 200000
# agents: [worker]              # sibling worker.yaml specs allowed to spawn
# agents_dynamic: true          # also allow inline role definitions
# agent_workspace: isolated     # isolated (default) | shared

# MCP servers are connected automatically in run/resume/daemon/driver paths.
# Secrets are referenced by environment-variable NAME, never embedded here.
# mcp:
#   - name: github
#     transport: stdio
#     command: [github-mcp-server]
#     env_from: { GITHUB_PERSONAL_ACCESS_TOKEN: GITHUB_TOKEN }
#     allowed_tools: [search_code, get_file_contents]  # bare server names
#   - name: remote
#     transport: http
#     url: https://mcp.example.test/mcp
#     oauth: { access_token_env: MCP_ACCESS_TOKEN }
`

// initCmd writes the example spec: `agentrunner init [path]` (default
// spec.yaml). It refuses to overwrite — the user's spec is theirs.
func initCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: agentrunner init [path]")
		return ExitUsage
	}
	path := "spec.yaml"
	if len(args) == 1 {
		path = args[0]
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			fmt.Fprintf(stderr, "agentrunner: %s already exists — not overwriting\n", path)
		} else {
			fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		}
		return ExitUsage
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(specTemplate); err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	fmt.Fprintf(stdout, "wrote %s\n", path)
	fmt.Fprintf(stderr, "next: agentrunner run %s \"say hello\"\n", path)
	return ExitOK
}
