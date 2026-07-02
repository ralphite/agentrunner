# AgentRunner — High-Level Design

一个灵活的 agent runner/harness。原型级实现：设计和代码尽量干净，零 legacy，
不考虑 backward compatibility。本文档是活的设计记录，随讨论逐步生长。

当前阶段：**先定高层 feature，细节设计后续逐项展开。**

## 目标

- 通过声明式 spec 定义并运行一个或多个 LLM agent。
- 每次运行都是 durable、可审计、可 replay 的。
- 内核保持小而正交：少数几个 primitive，其余靠组合。

## 非目标（原型阶段）

- 分布式/多节点执行（设计上留出空间，实现上单进程）。
- spec、event、API 的向后兼容。
- 生产级加固（鉴权、多租户、跨用户配额）。

---

## 核心 primitives

系统由六个 primitive 构成，其余一切都是组合。

```
┌─────────────────────────────────────────────────────────────┐
│                          Runtime                            │
│                                                             │
│  ┌───────────┐   ┌───────────┐   ┌───────────┐              │
│  │  Actor A  │   │  Actor B  │   │  Actor C  │  ...         │
│  │ (agent)   │   │ (agent)   │   │ (workflow)│              │
│  └─────┬─────┘   └─────┬─────┘   └─────┬─────┘              │
│        │  mailboxes    │               │                    │
│  ══════╧═══════════════╧═══════════════╧══════ Message Bus  │
│        │                                                    │
│  ┌─────┴──────────┐      ┌────────────────┐                 │
│  │  Event Store   │◄─────┤  Checkpoints   │                 │
│  │ (append-only)  │      │  (snapshots)   │                 │
│  └────────────────┘      └────────────────┘                 │
└─────────────────────────────────────────────────────────────┘
```

### 1. Actor model

- 一切可运行的东西都是 **actor**：一个 id、一个 mailbox、一个 behavior。
  agent、workflow、系统服务（journal、scheduler）统一都是 actor。
- actor 从 mailbox 里逐条处理消息——没有共享可变状态，没有锁。
  并发来自"很多个 actor"，而不是单个 actor 内部的并行。
- actor 可以 `spawn` 子 actor，可以按 id 向任意 actor `send` 消息。
- **Supervision**：每个 actor 有 parent。actor 崩溃时通知 supervisor，
  由其执行重启策略（`restart`（从最近 checkpoint 恢复）、`resume`、`stop`、`escalate`）。

### 2. Message bus

- 单一进程内 bus，两种投递模式：
  - **send(to, msg)** — 点对点，进入某个 actor 的 mailbox。
  - **publish(topic, msg)** — pub/sub，扇出给所有 subscriber。
- 所有消息都是不可变的 **envelope**：

  ```
  Envelope {
    id            # 消息唯一 id
    causation_id  # 引发本消息的那条消息
    correlation_id# 串起整个会话/run
    sender, target (actor id 或 topic)
    type, payload
    ts
  }
  ```

- causation/correlation id 让 event log 天然具备 tracing 级别的因果链路。
- bus 只做 transport，本身是 ephemeral 的。持久化只发生在 event store，
  不在 bus。

### 3. Event sourcing

- 两套词汇，严格分开：
  - **Command** — 意图，在 bus 上流动（`RunAgent`、`CallTool`、`CancelRun`）。
  - **Event** — 不可变事实，append 进 store（`AgentStarted`、`LlmCalled`、
    `ToolReturned`、`RunCompleted`）。
- actor 的 state 永远不被直接修改，唯一路径是：
  `handle(command) → emit(events) → append to store → apply(event) to state`。
- event store 是 append-only 的，按 actor 分 stream，每个 stream 内
  sequence number 单调递增。
- 完整的 event log **就是** 审计日志、调试器和测试 fixture：
  任何一次 run 都可以从 events 重新推导出来。

### 4. Checkpoints

- 恢复 actor 时重放几千条 event 太浪费。**checkpoint** 是 actor state 在
  stream sequence `N` 处的 snapshot。
- 恢复 = 加载最新 snapshot + replay `seq > N` 的 events。
- 触发时机：每 K 条 event，以及 workflow step 边界。
- snapshot 是可丢弃的——删掉只损失 replay 时间，不损失正确性。
  event log 永远是唯一的 source of truth。

### 5. Durable workflow

- workflow 是确定性的编排代码，其**副作用被记录**（Temporal 风格）：
  - 有副作用的操作（LLM 调用、tool 调用、MCP 调用、sleep、调子 agent）
    以 **activity** 形式执行。
  - 每个 activity 的结果先持久化为 event，workflow 才继续。
  - 崩溃/重启后 replay 时，log 里已有结果的 activity 直接返回记录值，
    不重新执行。
- 结果：run 能挺过进程死亡。重启 runner，所有进行中的 run 从中断的那一步
  精确恢复——已完成的 LLM 调用不会重复付费。
- **agent loop 本身就是一个 durable workflow**：每个 LLM turn、每次 tool
  执行都是 activity。agent 的 durability 不需要任何额外机制。
- workflow 代码必须确定性：不读墙钟、不用 RNG、不在 activity 之外做 I/O。
  runtime 从构造上强制这一点（workflow 只拿到一个 `ctx`，其上只暴露
  activity、timer 和 messaging）。

### 6. Agent spec

agent 完全由声明式 spec（YAML）定义。一切皆可配置，runner 里不硬编码任何
agent 相关内容。

```yaml
# agents/researcher.yaml
name: researcher
description: Deep-dives a topic and reports findings.

model:
  provider: anthropic
  id: claude-sonnet-5
  max_tokens: 8192

system_prompt: |
  You are a meticulous researcher...
  # 或者: system_prompt_file: prompts/researcher.md

tools:                      # 内置 tool 白名单
  - read_file
  - web_search

mcp:                        # 该 agent 可用的 MCP server
  - name: github
    transport: stdio
    command: ["github-mcp-server"]
    allowed_tools: [search_code, get_file_contents]   # 可选收窄

skills:                     # markdown skill 目录，按需加载
  - ./skills/research

agents:                     # 允许 spawn 的子 agent（按 spec 名引用）
  - summarizer

limits:
  max_turns: 40
  max_tokens_total: 500_000
  timeout_s: 900
```

- spec 在加载时校验为强类型 model（pydantic），坏 spec 立刻报出精确错误。
- spec 是*模板*；**agent instance** 是"spec + 运行时输入（任务、
  correlation id、parent）"创建出的 actor。
- prompt、tools、MCP、skills、model、limits 全部是数据。新增一个 agent
  只需加一个 YAML 文件，不写代码。

### 7. Multi-agent

- agent 就是共享 bus 上的 actor，所以 multi-agent 不是独立子系统，
  就是 actor 之间发消息。runner 在其上提供几种模式：
  - **Spawn/await** — agent 以 activity 形式调用子 agent 并等待结果
    （扇出 N 个 child 同理）。
  - **Handoff** — agent 把会话/任务移交给另一个 agent 然后退出。
  - **Pub/sub 协作** — agent 订阅 topic（例如 blackboard topic），
    对彼此的产出做出反应。
- 子 agent 的结果和其他 activity 一样流经 parent 的 event log，
  因此整棵 multi-agent 树可以整体确定性 replay。
- spec 里的 `agents:` 列表即白名单——agent 只能 spawn spec 允许的对象。

---

## 横切 features

在核心 primitives 之上，以下六项确定纳入高层 feature 清单
（细节设计后续逐项展开）：

1. **可观测性（Observability）** — event log 本身就是 trace；提供
   `replay`/`inspect` CLI，把一次 run 渲染成时间线（turns、tool 调用、
   子 agent、token 消耗）。
2. **人工审批（Human-in-the-loop）** — 审批是一等公民：agent 发出
   `ApprovalRequested` event，workflow durable 地挂起，直到
   `ApprovalGranted` command 到达——几分钟或几天都行，durability 让这件事
   免费。
3. **预算限制（Budgets & limits）** — 由 runtime（而非 agent 自己）强制执行
   spec 里的 `limits:`，超限时发出 `LimitExceeded` 并停止 actor。
4. **触发调度（Triggers & scheduling）** — cron 或事件触发的 run；
   scheduler 就是一个发布 `RunAgent` command 的普通 actor。
5. **确定性回放测试（Deterministic replay testing）** — 录制一次 run，
   测试中用 stub activity replay；agent 行为变化直接体现为 event log 的 diff。
6. **上下文/记忆管理（Context & memory）** — compaction、summarization、
   跨 run memory，作为 spec 的可配置 section。

---

## 关键设计决策

| # | 决策点 | 选择 | 理由 |
|---|--------|------|------|
| 1 | 语言 | Python 3.12+, asyncio | async actor 天然映射到 task + queue；pydantic 做 spec；MCP + Anthropic SDK 成熟。 |
| 2 | 进程模型 | 单进程，in-memory bus | 原型简单。actor + event sourcing 的边界意味着日后分布式化只是换 transport，不是重新设计。 |
| 3 | Bus vs. store | bus 是 ephemeral transport；event store 是唯一持久化 | 避开"bus 要不要 durable"的泥潭。durability 只存在于一个地方。 |
| 4 | Command vs. event | 严格分离的类型 | 意图（可重试、可拒绝）与事实（不可变、可 replay）分开。 |
| 5 | 存储后端 | 每个 stream 一个 JSONL 文件，藏在 `EventStore` 接口后 | run 可读、可 diff；需要时换 SQLite。 |
| 6 | Durability 模型 | Temporal 风格的 activity record/replay | 让 agent loop 本身 crash-safe 且可恢复的最简模型。 |
| 7 | Spec 格式 | YAML → pydantic model | 声明式、可 review，新 agent 不需要写代码。 |

## 待拍板的问题

- **LLM provider 抽象**：原型只做 Anthropic，还是一开始就留一层薄的
  provider 接口？（倾向：薄接口 + 单实现。）
- **Skill 格式**：沿用 Claude Code 的 skill 约定（目录 + markdown +
  frontmatter），还是自定义最简格式？（倾向：沿用。）
- **Streaming**：v0 就把 token streaming 透出到 CLI，还是 turn 粒度输出
  就够？
- **Event schema 版本化**：原型阶段不做 migration，schema 变更即丢弃旧
  run 日志重跑——确认可接受？（倾向：接受。）

## Roadmap

1. **M1 — Kernel**：actor、bus、event store、checkpoint；toy actor 测试。
2. **M2 — Durable workflows**：activity record/replay，crash-resume 测试。
3. **M3 — Single agent**：spec loader、agent loop as workflow、内置 tools、CLI。
4. **M4 — MCP + skills**：MCP server 生命周期、skill 加载。
5. **M5 — Multi-agent**：spawn/await、handoff、pub/sub 模式；示例 fleet。
