export const DEFAULT_SPEC = `name: dev
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 4096 }
system_prompt: |
  你是一个严谨的编码助手。严格按用户指令行动;用户要求启动子 agent 时,
  用 spawn_agent 工具、数量与分工严格照做;要求取消时用 task_kill。
tools: [read_file, write_file, edit_file, bash, spawn_agent, kill]
agents: [worker]
permissions:
  - { action: allow }
`;

export const DEFAULT_WORKER = `name: worker
description: 执行父分派的调查/修改任务并汇报
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 4096 }
system_prompt: 完成任务后用简洁的要点汇报结论。
tools: [read_file, bash]
`;

// A driver.yaml is NOT an agent spec — it references a child agent spec via
// agent_spec (a sibling file) and carries the loop's task/verifiers/limits.
export const DEFAULT_DRIVER = `name: progress-loop
agent_spec: agent.yaml
task: 在 workspace 的 progress.txt 里追加一行本轮进展(用 bash 或 write_file),不要重复上一行。
max_iterations: 3
verifiers:
  - { kind: command, command: "test -f progress.txt" }
`;

// The child agent each iteration runs as. Written as the agent.yaml sibling.
export const DEFAULT_DRIVER_AGENT = `name: worker
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: 你在一个迭代循环里工作,每轮把任务推进一小步并自检,简洁汇报本轮做了什么。
tools: [read_file, write_file, bash]
permissions:
  - { action: allow }
`;
