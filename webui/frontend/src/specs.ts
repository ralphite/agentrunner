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

export const DEFAULT_DRIVER = `name: reviewer-loop
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 4096 }
system_prompt: 你在一个 plan/verify 迭代循环里工作,每轮推进并自检。
tools: [read_file, write_file, bash]
max_iterations: 3
`;
