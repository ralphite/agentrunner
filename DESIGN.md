# AgentRunner — Design

一个灵活的 agent runner/harness，目标能力对标 Claude Code 一类的 agent
harness。原型级实现：设计和代码尽量干净，零 legacy，不考虑 backward
compatibility。本文档是活的设计记录，随讨论逐步生长。

当前阶段：**高层设计已收口，细节设计按 roadmap 逐项展开。**

## 目标

- 通过声明式 spec 定义并运行一个或多个 LLM agent，agent 的一切行为皆可配置。
- 每次运行都是 durable 的：挺过进程死亡、可审计、可 replay、可 fork。
- 交互式：streaming 输出、运行中途 steering、审批、interrupt。
- 内核小而正交：少数几个 primitive，Claude Code 级的 feature 全部由
  primitive 组合出来，而不是逐个特判实现。

## 非目标（原型阶段）

- 分布式/多节点执行（设计上留出空间，实现上单进程）。
- spec、event、API 的向后兼容。event schema 变更即丢弃旧 run 日志重跑，
  不做 migration。
- 生产级加固（鉴权、多租户、跨用户配额）。

---

## 设计原则

整个系统由五条原则推导，每个 feature 都应能回答"我是哪条原则的组合"：

1. **一切可运行的是 actor。** agent、workflow、scheduler、frontend、
   journal——统一模型，统一生命周期，统一 supervision。
2. **一切持久的是 event。** append-only event log 是唯一的 source of
   truth；state、checkpoint、trace、session、测试 fixture 都是它的派生物。
3. **一切副作用是 activity，且流经同一条 effect pipeline。**
   hooks、permission、审批、预算不是四个子系统，而是这条管线上的四个关卡。
4. **一切行为由数据定义。** spec + 分层配置决定 agent 的全部行为，
   core 里不硬编码任何具体 agent。
5. **core 是库。** CLI、headless、server、scheduler 都是挂在 core 上的
   薄壳（也都是 actor），不存在"特权 frontend"。

## 架构分层

```
┌──────────────────────────────────────────────────────────────┐
│ L4  Surfaces      CLI · headless · server · scheduler        │
│                   session 管理 · 交互协议 · observability    │
├──────────────────────────────────────────────────────────────┤
│ L3  Agent layer   spec · agent loop · tools/workspace · MCP  │
│                   skills · provider · context/memory ·       │
│                   multi-agent                                │
├──────────────────────────────────────────────────────────────┤
│ L2  Effect        hooks → permission policy → budget →       │
│     pipeline      execute（所有副作用的唯一通道）            │
├──────────────────────────────────────────────────────────────┤
│ L1  Durability    event store · durable workflow ·           │
│                   checkpoints（含 workspace snapshot）       │
├──────────────────────────────────────────────────────────────┤
│ L0  Kernel        actor · mailbox · supervision · bus        │
└──────────────────────────────────────────────────────────────┘
```

上层只依赖下层。L0/L1 对"agent"一无所知；L2 对"LLM"一无所知；
换掉 L3 可以跑完全不同类型的 durable 工作负载。

---

## L0 — Kernel

- **Actor**：一个 id、一个 mailbox、一个 behavior。从 mailbox 逐条处理
  消息——没有共享可变状态，没有锁。并发来自"很多个 actor"。
- **Bus**：进程内 transport，两种投递——`send(to, msg)` 点对点进 mailbox；
  `publish(topic, msg)` pub/sub 扇出。bus 是 ephemeral 的，持久化只发生在
  L1 的 event store。
- **Envelope**：所有消息不可变，携带 `id / causation_id / correlation_id /
  sender / target / type / payload / ts`。causation/correlation 让 event
  log 天然具备 tracing 级因果链路。
- **Supervision**：每个 actor 有 parent；崩溃时由 supervisor 执行策略
  （`restart`（从最近 checkpoint 恢复）、`resume`、`stop`、`escalate`）。

## L1 — Durability

### Event sourcing

- 两套词汇严格分开：**Command** 是意图，在 bus 上流动（`RunAgent`、
  `CallTool`、`CancelRun`）；**Event** 是不可变事实，append 进 store
  （`AgentStarted`、`LlmCalled`、`ToolReturned`、`RunCompleted`）。
- state 变更的唯一路径：`handle(command) → emit(events) → append → apply`。
- store 按 actor 分 stream，stream 内 sequence 单调递增。
  原型后端：每 stream 一个 JSONL 文件，藏在 `EventStore` 接口后。

### Durable workflow

- workflow 是确定性编排代码，副作用全部以 **activity** 执行（Temporal
  风格）：结果先持久化为 event，workflow 才前进；crash 后 replay 时，
  已有结果的 activity 直接返回记录值，不重新执行。
- **retry 是 activity 的通用属性**：retry/backoff、rate limit 处理、
  model fallback 都是 activity 级重试策略，所有副作用共享同一套健壮性语义。
- workflow 可以 **durable 地挂起**（park）等待某条 command——审批、
  timer、人工输入都靠这一个能力，等几分钟或几天成本相同。
- 确定性由构造保证：workflow 只拿到 `ctx`，其上只暴露 activity、timer、
  messaging；不读墙钟、不用 RNG、不做裸 I/O。

### Checkpoints

- checkpoint = actor state 在 stream seq `N` 处的 snapshot，恢复 =
  最新 snapshot + replay `seq > N`。
- **checkpoint 语义覆盖 workspace**：对话状态 + 文件系统快照一起打点，
  这是 rewind/fork 能回退"世界状态"而不只是回退对话的前提。
- snapshot 可丢弃——删掉只损失 replay 时间，不损失正确性。

## L2 — Effect pipeline

所有副作用（tool 调用、MCP 调用、LLM 调用、spawn 子 agent、bash……）
流经唯一一条管线，四个关卡按序执行：

```
effect command
   │
   ▼
[1] Hooks (pre)     # 观察 / 改写 / 阻断；PreToolUse、SessionStart…
   │
   ▼
[2] Permission      # policy check → allow / ask / deny
   │                #   ask ⇒ 发 ApprovalRequested，workflow park，
   │                #         等 ApprovalGranted / Denied command
   ▼
[3] Budget          # limits 检查（turns/tokens/cost/timeout）
   │                #   超限 ⇒ LimitExceeded event，停 actor
   ▼
[4] Execute         # 作为 activity 执行（带 retry 语义），结果入 log
   │
   ▼
[5] Hooks (post)    # PostToolUse、Stop…
```

由此"免费"得到的 feature：

- **Permission 系统** — per-tool allow/ask/deny、路径级规则（可读但写需
  审批）、permission modes（default / acceptEdits / plan / bypass）。
  policy 是数据，来自 spec + 分层配置。
- **人工审批** — 只是 policy 判定为 `ask` 时的自然结果：durable park +
  一条等待中的 command。不是独立子系统。
- **Hooks** — 挂在管线固定拦截点上的订阅者，能观察、改写、阻断。
  拦截点从第一天就在管线里，不是后补的。
- **预算与记账** — runtime（而非 agent 自己）强制 `limits:`；
  token/cost 记账从 LLM activity 的 events 里统计，per-run、per-agent 可查。

管线本身也是事件源的：每个关卡的判定都是 event，审计时能看到
"这个 tool call 为什么被放行/拦下"。

## L3 — Agent layer

### Agent spec

agent 完全由声明式 spec（YAML → pydantic model）定义，加载时校验、
坏 spec 报精确错误。spec 是模板，**agent instance** = spec + 运行时输入
（task、correlation id、parent）。

```yaml
# agents/researcher.yaml
name: researcher
description: Deep-dives a topic and reports findings.

model:
  provider: anthropic          # 薄 provider 接口，原型只有 anthropic 实现
  id: claude-sonnet-5
  max_tokens: 8192

system_prompt_file: prompts/researcher.md   # 或内联 system_prompt: |

tools: [read_file, edit_file, bash, web_search]   # 内置 tool 白名单

mcp:
  - name: github
    transport: stdio
    command: ["github-mcp-server"]
    allowed_tools: [search_code, get_file_contents]

skills:                        # Claude Code skill 约定：目录 + markdown + frontmatter
  - ./skills/research

agents: [summarizer]           # 允许 spawn 的子 agent 白名单

permissions:
  mode: default                # default / acceptEdits / plan / bypass
  rules:
    - tool: read_file
      action: allow
    - tool: edit_file
      path: "src/**"
      action: allow
    - tool: bash
      action: ask

hooks:
  pre_tool_use: ["./hooks/lint-check.sh"]

context:
  compaction: auto             # 上下文压缩策略
  memory_files: true           # 注入 CLAUDE.md 式 memory 文件

limits:
  max_turns: 40
  max_tokens_total: 500_000
  timeout_s: 900
```

### 配置分层与 memory 文件

- settings 三层合并：user / project / local，语义在 loader 里一次定清。
- CLAUDE.md 式项目指令文件按目录层级发现、合并、注入 system prompt。
- spec 里所有 section 都可被分层配置覆盖——spec 定义 agent，
  配置定义环境。

### Agent loop

- **agent loop 就是一个 durable workflow**：LLM turn、tool 执行、
  spawn 子 agent 都是 activity，全部走 L2 管线。agent 的
  durability、permission、记账不需要任何专门机制。
- loop 在 turn 边界消费 mailbox 里的 **steering 消息**（用户插话、
  interrupt、cancel）——交互能力是 loop 的形状决定的，从第一天设计进去。
- **context/memory 管理**由 loop 调用：compaction、summarization 按
  spec 配置触发；跨 run memory 是持久化的普通数据。

### Tools 与 workspace

- 内置 tool 套件（file read/write/edit、bash、glob/grep、web
  fetch/search）建立在 **workspace 抽象**上：工作目录、路径边界、
  bash 沙箱等级。
- worktree 级隔离支持多 agent 并行改文件不打架。
- workspace 快照参与 checkpoint（见 L1），rewind 回退的是"世界状态"。

### MCP 与 skills

- MCP server 生命周期由 runner 按 spec 管理（启动、握手、工具发现、
  关闭）；`allowed_tools` 可收窄暴露面。MCP tool 调用同样走 L2 管线。
- skills 沿用 Claude Code 约定（目录 + markdown + frontmatter），
  按需加载进上下文。

### Provider

- 薄 provider 接口（`complete(messages, tools, …) → stream`），
  原型只有 Anthropic 一个实现。接口以 streaming 为原生形态。

### Multi-agent

- multi-agent 不是子系统，就是 actor 发消息。三种模式：
  **spawn/await**（子 agent 作为 activity，可扇出 N 个）、
  **handoff**（移交任务后退出）、**pub/sub 协作**（订阅 blackboard topic）。
- 子 agent 结果流经 parent 的 event log，整棵树可整体确定性 replay。
- spec 的 `agents:` 是 spawn 白名单。

## L4 — Surfaces

### Session 管理

- session = correlation id + 它名下的 streams。天然获得：
  **list**（枚举 store）、**resume**（replay，免费）、
  **fork**（复制 stream 到 seq N，开新分支）、
  **rewind**（fork + workspace 快照回退）。

### 交互协议

- frontend 是普通 actor：订阅输出 topic，向 agent mailbox 发输入。
- **输出从第一天就是流**：agent 的输出以事件流形式 publish
  （turn 开始/结束、token delta、tool call、审批请求……）。
  CLI 先做 turn 粒度渲染，token streaming 是纯增量，协议不变。
- steering / interrupt / cancel / 审批应答都是发进 mailbox 的 command。

### 运行形态

- core 是库。`agentrunner run <spec> "task"`（CLI）、headless 单发、
  server（HTTP/WS 暴露同一交互协议）都是薄壳。

### Scheduler 与 triggers

- cron 或事件触发的 run：scheduler 是一个发布 `RunAgent` command 的
  普通 actor。webhook 触发 = server 壳收到请求后发同一条 command。

### Observability

- event log 本身就是 trace。`replay` / `inspect` CLI 把 run 渲染成时间线：
  turns、tool 调用及其 permission 判定、子 agent 树、token/cost 消耗。
- 无需埋点——想看的一切本来就在 log 里。

---

## 测试策略

- **确定性 replay 测试**：录制一次真实 run，测试中 activity 直接返回
  记录值——不打 LLM、不碰网络，毫秒级跑完整个 agent 行为。
- agent 行为变化体现为 event log 的 diff，review 的是"决策序列"
  而不是 mock 断言。
- kernel/L1/L2 用普通单测；spec loader 用坏 spec 的错误信息做黄金测试。

## 已定决策

| # | 决策点 | 选择 | 理由 |
|---|--------|------|------|
| 1 | 语言 | Python 3.12+, asyncio | actor 映射到 task + queue；pydantic 做 spec；MCP + Anthropic SDK 成熟。 |
| 2 | 进程模型 | 单进程，in-memory bus | 原型简单；actor + event sourcing 边界让分布式化只是换 transport。 |
| 3 | Bus vs. store | bus ephemeral，event store 是唯一持久化 | durability 只存在于一个地方。 |
| 4 | Command vs. event | 严格分离 | 意图（可重试、可拒绝）与事实（不可变、可 replay）分开。 |
| 5 | 副作用治理 | 单一 effect pipeline：hooks → permission → budget → execute | permission/审批/hooks/预算是一个机制的四个关卡，不是四个子系统。 |
| 6 | Durability 模型 | Temporal 风格 activity record/replay + durable park | 让 agent loop crash-safe、可恢复、可挂起等审批的最简模型。 |
| 7 | Checkpoint 语义 | 对话状态 + workspace 快照 | rewind/fork 要回退"世界状态"。 |
| 8 | 存储后端 | JSONL per stream，藏在 `EventStore` 接口后 | 可读可 diff；需要时换 SQLite。 |
| 9 | Spec 格式 | YAML → pydantic model | 声明式、可 review；新 agent 不写代码。 |
| 10 | 运行形态 | core 是库；CLI/headless/server 是薄壳 | 一套 core 支撑所有 surface。 |
| 11 | Provider | 薄接口 + 仅 Anthropic 实现，streaming 原生 | 不过度抽象，但不锁死。 |
| 12 | Skill 格式 | 沿用 Claude Code 约定 | 生态兼容，不发明格式。 |
| 13 | Streaming | 协议从第一天按事件流设计；CLI 先 turn 粒度渲染 | token streaming 是纯增量，不改协议。 |
| 14 | Event schema 版本化 | 不做 migration，schema 变更丢弃旧日志 | 原型阶段 re-run 比 migrate 便宜。 |

## Roadmap

1. **M1 — Kernel + durability 骨架**：actor、bus、supervision；event
   store、command/event 纪律、checkpoint。toy actor 的崩溃恢复测试。
2. **M2 — Durable workflow + effect pipeline**：activity
   record/replay、retry、durable park；管线四关卡及拦截点（hooks、
   permission、budget 先做最简实现）。crash-resume 测试。
3. **M3 — Single agent**：spec loader + 配置分层、provider（Anthropic）、
   workspace + 内置 tools、agent loop（含 steering/interrupt）、
   CLI（含事件流渲染）、session resume。
4. **M4 — 生态接入**：MCP 生命周期、skills、memory 文件、
   context compaction。
5. **M5 — Multi-agent + surfaces 收尾**：spawn/await、handoff、pub/sub；
   scheduler；session fork/rewind；`inspect` 时间线；server 壳。
