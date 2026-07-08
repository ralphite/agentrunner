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
tools: [read_file, write_file, edit_file, bash]

# --- optional ---------------------------------------------------------
# mode: plan                # default | plan | acceptEdits
# max_generation_steps: 40  # cap on model calls per turn
# budget:
#   max_total_tokens: 200000
# permissions:              # allow | ask | deny rules, first match wins
#   - tool: bash
#     command: "git *"
#     action: allow
# on_run_end: cancel        # cancel | await — what happens to background tasks
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
