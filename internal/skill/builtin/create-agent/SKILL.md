---
name: create-agent
description: Create or update a custom agent in the user catalog (~/.config/agentrunner/agents) from a description of what it should do; the saved agent is immediately usable in the webui agent picker and via `agentrunner new <name>`.
---

# Creating a custom agent

You are turning the user's description of an assistant they want into a
saved agent definition. The flow is: understand → draft → save with
`save_agent` → present for review. Do not write the YAML into the workspace;
`save_agent` is the only channel to the user catalog.

## 1. Understand what they want

Work out from the conversation:

- **name** — a short bare identifier (lowercase, digits, `-`/`_`), e.g.
  `release-drafter`. Suggest one if the user didn't name it.
- **job** — what the agent does, in one or two sentences. This becomes the
  `description` and drives the system prompt.
- **behavior** — tone, output shape, constraints, domain rules.
- **capabilities** — what it must be able to do (read code? edit? run
  commands? search the web? delegate?).

If something essential is genuinely unclear AND wrong guesses would be
costly, ask once with ask_user (batch all questions into that one call).
Otherwise pick sensible defaults and note them in the final summary.

## 2. Draft the spec YAML

Template — include only the fields you need:

```yaml
name: <name>                # MUST equal the save_agent name argument
description: <one line>     # shown in agent pickers and to parent agents
system_prompt: |
  <persona and rules: what it is, how it responds, what it never does.
   Write it like a good prompt: identity first, then behavior bullets.>
tools: [<subset>]
# Optional, when the job calls for them:
# max_generation_steps: 24      # cap a scoped agent's turn length
# mode: plan                    # start read-only (user can override per run)
# permissions:
#   - { tool: bash, action: ask }   # actions: allow | ask | deny
# agents: [worker, explore, plan]   # sub-agents it may spawn
# agents_dynamic: true              # or let it draft roles at run time
# output_schema: { ... }            # constrain replies to JSON
# skills: [create-agent, /abs/path/to/skill-dir]  # bundle skills: shipped
#                                  # names or dirs containing a SKILL.md
```

Choosing `tools` (least privilege — grant what the job needs, no more):

- read/analyze code: `read_file, grep, glob, keyword_search`
- edit files: add `edit_file, write_file`
- run commands/tests/git: add `bash`
- fetch web pages: add `web_fetch`
- ask the user mid-run: add `ask_user`
- delegate/parallelize: add `spawn_agent, kill` (+ `agents:` whitelist)
- follow workspace skills: add `skill`
- a pure conversational agent: `tools: []`

Hard rules (save_agent enforces most of them):

- NO `model:` field — the model is chosen per session, not per agent.
- `name:` must equal the filename/name argument.
- `mode: bypass` is not allowed in a spec.
- Every tool name must be a real tool (the error will list valid names).
- Keep the system prompt focused; workspace conventions already reach the
  agent via CLAUDE.md, so don't restate them.

## 3. Save it

Call `save_agent{name, yaml}`. If validation fails, fix the YAML from the
error and retry. To update an existing agent, call again with
`overwrite: true` — but only after telling the user you are replacing it.

## 4. Present for review

Show the user:

1. The full saved YAML (they should be able to review every line).
2. Where it landed (the `path` from the result).
3. How to use it: pick it in the webui's New session agent selector, or
   `agentrunner new <name> "..."`.
4. How to iterate: describe the change and you will re-save with
   `overwrite: true`.

Offer a quick test run as a natural next step, but don't start one
unprompted.
