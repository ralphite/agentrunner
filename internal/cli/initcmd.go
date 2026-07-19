package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
  # compact_at_tokens: 60000     # auto-summarize the context past this size
  # microcompact_at_tokens: 0    # clear old tool results past this size (default 3/4 of compact_at_tokens; -1 disables)

system_prompt: >
  You are a helpful coding agent. Answer in plain text; use tools only
  when the prompt requires reading or changing files or running commands.
# system_prompt_file: prompt.md   # or load the prompt from a file (not both)

# Tools the agent may use; omit for a chat-only agent.
tools: [read_file, write_file, edit_file, bash, grep, glob, keyword_search]

# Heads-up: with NO permissions block below, edits (write_file/edit_file)
# and shell commands (bash) PAUSE for your approval every time — reads are
# free, side effects ask. (Common read-only commands — ls, pwd, cat, git
# status… — are pre-approved and never ask.) Answer a pending ask with:
#   agentrunner approve <session> <id> approve|deny
# Uncomment permissions to pre-authorize what you trust (first match wins):
permissions:
  - { tool: read_file, action: allow }   # reads never touch anything
  # - { tool: bash, command: "git *", action: allow }  # trust git
  # - { action: allow }                  # trust everything (single-user dev box)

# --- optional ---------------------------------------------------------
# mode: plan                # default | plan | acceptEdits
# max_generation_steps: 200 # cap on model calls per turn
# budget:
#   max_total_tokens: 200000
# agents: [worker]              # sibling worker.yaml specs allowed to spawn
# agents_dynamic: true          # also allow inline role definitions
# agent_workspace: isolated     # isolated (default) | shared
# sandbox:
#   network: none               # remove bash egress (a ratchet: children can never widen it)
#   env_passthrough: [GEMINI_API_KEY]  # credential env vars bash/hooks may see
#                               # (default: every *_API_KEY/_TOKEN/_SECRET is withheld;
#                               #  the tool result lists what was withheld by name)

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

// driverTemplate is the commented example driver spec `agentrunner init
// --driver` writes (QA Round2 F-E9: the driver schema was undiscoverable —
// no template, no docs). It must always pass driver.LoadSpec next to a
// default spec.yaml — TestInitDriverSpecLoads pins that.
const driverTemplate = `# agentrunner driver spec — an iteration driver: it runs a fresh child
# agent per iteration until the verifiers pass (goal mode) or on a
# schedule. Required: name, prompt, agent_spec. Run: agentrunner drive <this file>
name: my-driver
prompt: Make the test suite pass          # the instruction EVERY iteration receives
agent_spec: spec.yaml                   # child agent spec, relative to this file (agentrunner init writes one)

max_iterations: 5     # goal-mode cap (default 10)
verifiers:            # ALL must pass for an iteration to satisfy the goal
  - kind: command     # command | llm_judge | human (a bare command: implies kind command)
    command: "test -f done.txt"         # exit 0 = pass
    # metric_regex: 'coverage: (\d+)'   # capture group 1 becomes a score
    # threshold: 80                     # score >= threshold passes

# --- optional ---------------------------------------------------------
# schedule: immediate   # immediate (goal) | interval | cron | self_paced | parallel (best-of-N)
# interval: 5m          # loop cadence (schedule: interval)
# cron: "0 2 * * *"     # loop cadence (schedule: cron)
# overlap: skip         # skip | coalesce — ticks firing while an iteration runs
# n: 3                  # attempt count for schedule: parallel (best-of-N)
# patience: 3           # stop after this many iterations with no score improvement
# series_memory: NOTES.md   # workspace file injected into every iteration's prompt
# budget:
#   max_total_tokens: 500000
# on_child_failure: { mode: stop }   # stop | surface | retry (with max: N)
`

// initCmd writes the example spec: `agentrunner init [path]` (default
// spec.yaml), or `agentrunner init --driver [path]` for a driver spec
// (default driver.yaml). It refuses to overwrite — the user's spec is theirs.
func initCmd(args []string, stdout, stderr io.Writer) int {
	driver := false
	rest := args[:0:0]
	for _, a := range args {
		if a == "--driver" || a == "-driver" {
			driver = true
			continue
		}
		rest = append(rest, a)
	}
	args = rest
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: agentrunner init [path]")
		return ExitUsage
	}
	path := "spec.yaml"
	template := specTemplate
	if driver {
		path = "driver.yaml"
		template = driverTemplate
	}
	if len(args) == 1 {
		path = args[0]
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			fmt.Fprintf(stderr, "agentrunner: %s already exists — not overwriting\n", path)
		} else if errors.Is(err, os.ErrNotExist) {
			// A path in a missing directory otherwise leaks a raw
			// "open …: no such file or directory" (QA Wave1 alice-05).
			fmt.Fprintf(stderr, "agentrunner: cannot write %s — the directory %s does not exist (create it first, or pass a path in an existing directory)\n", path, filepath.Dir(path))
		} else {
			fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		}
		return ExitUsage
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(template); err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	fmt.Fprintf(stdout, "wrote %s\n", path)
	if driver {
		fmt.Fprintf(stderr, "next: agentrunner drive %s   (it iterates spec.yaml — agentrunner init writes that)\n", path)
	} else {
		fmt.Fprintf(stderr, "next: agentrunner run %s \"say hello\"\n", path)
	}
	return ExitOK
}
