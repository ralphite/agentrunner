---
name: create-tool
description: Create, update, or remove a command tool (a local command packaged as a model-callable tool) in the user config (~/.config/agentrunner/tools) — validated manifest, immediately available to new sessions.
tools: [tool_config]
---

# Managing command tools

You are turning the user's description of a helper command into a saved
command-tool manifest (or updating/removing one). Use the `tool_config`
tool for every read and write; file tools cannot reach the config dir.

A command tool wraps ONE local command as a tool the model can call: the
manifest declares name, description, the command line, and a JSON Schema
for arguments; at call time the arguments arrive as JSON on the command's
stdin, and every invocation runs as an execute-class effect through the
permission pipeline plus the OS sandbox.

## 1. See what exists

Call `tool_config{action:"list"}` first — it returns every user-layer
manifest. Update and remove must name an existing tool; create must not
collide (built-in tool names are refused by validation anyway).

## 2. Draft the manifest (create/update)

```json
{
  "name": "run_lint",
  "description": "Run the project linter. Pass paths to narrow the scan.",
  "command": "./scripts/lint.sh",
  "timeout_s": 120,
  "params": {
    "type": "object",
    "properties": {
      "paths": { "type": "array", "items": { "type": "string" }, "description": "Files or dirs to lint; empty = whole repo." }
    }
  }
}
```

Rules (validation enforces most):

- `name`: bare identifier (lowercase, digits, `-`/`_`); equals the
  filename `<name>.json`; built-in names and the `mcp__` prefix are
  refused.
- `command` is required — an executable on PATH, an absolute path, or a
  path relative to the session's workspace. Remember the command reads
  its arguments as JSON from stdin.
- `params` is optional; when present it must be a JSON **object** schema.
  Omit it for a parameterless tool.
- `timeout_s` is optional (`0` or omitted = unlimited; a positive value is
  honored exactly).
- The description is what the model sees when deciding to call the tool —
  write it like a good tool description: what it does, when to use it.

## 3. Save / remove

- Create: `tool_config{action:"save", name, manifest}`.
- Update: same, plus `overwrite:true` — but only after telling the user
  you are replacing the existing manifest.
- Remove: confirm with the user which tool, then
  `tool_config{action:"remove", name}`.
- If validation rejects the manifest, fix it from the error and retry.

## 4. Present for review

Show the user:

1. The full saved manifest JSON (every line reviewable), or what was
   removed.
2. Where it landed (the `path` from the result).
3. That it takes effect in NEW sessions — the current session's tool set
   was frozen at start.
4. How to iterate: describe the change and you will re-save with
   `overwrite:true`.
