export const DEFAULT_SPEC = `name: dev
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 4096 }
system_prompt: |
  You are a rigorous coding assistant. Follow the user's instructions
  exactly; when asked to start sub-agents, use the spawn_agent tool with
  the exact count and division of labor requested; use kill to cancel.
tools: [read_file, write_file, edit_file, bash, spawn_agent, kill]
agents: [worker]
permissions:
  - { action: allow }
`;

export const DEFAULT_WORKER = `name: worker
description: carries out investigation/edit tasks assigned by the parent and reports back
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 4096 }
system_prompt: When the task is done, report your conclusions as concise bullet points.
tools: [read_file, bash]
`;

// A driver.yaml is NOT an agent spec — it references a child agent spec via
// agent_spec (a sibling file) and carries the loop's task/verifiers/limits.
export const DEFAULT_DRIVER = `name: progress-loop
agent_spec: agent.yaml
task: Append one line describing this round's progress to progress.txt in the workspace (use bash or write_file); do not repeat the previous line.
max_iterations: 3
verifiers:
  - { kind: command, command: "test -f progress.txt" }
`;

// The child agent each iteration runs as. Written as the agent.yaml sibling.
export const DEFAULT_DRIVER_AGENT = `name: worker
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: You work in an iteration loop; each round advance the task one small step, self-check, and report concisely what you did this round.
tools: [read_file, write_file, bash]
permissions:
  - { action: allow }
`;
