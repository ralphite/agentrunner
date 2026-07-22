# Shipped Web UI agents

这里是 Web UI `Automation → Agent` 显示的默认 agent 配置，也是它们的
source of truth：

- `dev.yaml` — 默认 coding agent；可 spawn `worker`
- `lead.yaml` — 动态组队、共享 workspace 的 Team Lead
- `auditor.yaml` — 只读短审计
- `reviewer.yaml` — 只读结构化 code review
- `chat.yaml` — 无工具对话
- `worker.yaml` — Dev 的 sibling sub-agent

修改 prompt、tools、agents、workspace 或其他 agent 行为时，直接编辑对应 YAML。
`specs.ts` 只保留 UI label/description，以及用户在 composer 选择的 model、effort、
access 覆盖逻辑；不要再把 agent prompt 或 tools 复制回 TypeScript。

这些文件会被 Vite 以 raw text 打进 Web UI bundle；修改后需要重新 build/deploy。
runtime 自带的只读 `explore` / `plan` sub-agent 另在
`internal/agent/builtin/*.yaml`，因为它们由 Go binary embed，不属于 Web UI persona。
