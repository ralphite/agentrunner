# AgentRunner — Design

一个灵活的 agent runner/harness，目标能力对标 Claude Code 一类的 agent
harness。原型级实现：设计和代码尽量干净，零 legacy，不考虑 backward
compatibility。本文档是活的设计记录，随讨论逐步生长。

当前阶段：**高层设计经三方独立 review 后修订收口，细节按 roadmap 逐项展开。**

## 目标

- 通过声明式 spec 定义并运行一个或多个 LLM agent，agent 的一切行为皆可配置。
- 每次运行都是 durable 的：挺过进程死亡、可审计、可恢复、可 fork。
- 交互式：streaming 输出、运行中途 steering 与 interrupt、审批。
- 内核小而正交：少数几个 primitive，Claude Code 级的 feature 由组合得出，
  而不是逐个特判实现。

## 非目标（原型阶段）

- 分布式/多节点执行（设计上留出空间，实现上单进程）。
- spec、event、API 的向后兼容。event schema 变更即丢弃旧 run 日志重跑，
  不做 migration。`RunStarted` 记录代码/spec 版本，resume 时版本不匹配
  直接报清晰错误拒绝恢复，而不是 replay 到一半发散。
- 生产级加固（鉴权、多租户、跨用户配额）。
- **确定性 code replay**（Temporal 式）。见"Durability 模型"——这是
  有意的取舍，不是遗漏。
- **整树确定性 replay**。多 agent 树保证的是 per-stream 可审计
  （causation/correlation 链路完整），不是跨 actor 消息交错的确定性重现。

---

## 设计原则

1. **一切可运行的是 actor。** agent、workflow、scheduler、frontend——
   统一模型，统一生命周期。
2. **一切历史皆 event。** 持久状态只有两处：event log（历史与决策的
   source of truth）和 workspace（世界状态，git 管理）。除此之外的一切
   ——bus 上的消息、token delta、内存中的 state——都是 ephemeral 或
   可从 event log 重建的派生物。
3. **一切副作用是 activity，流经同一条 effect pipeline。** hooks、
   permission、审批、预算是这条管线上的关卡，不是四个子系统。
4. **一切行为由数据定义。** spec + 配置决定 agent 的全部行为——包括
   tool 定义本身。core 里不硬编码任何具体 agent。
5. **core 是库。** CLI、headless、server、scheduler 都是挂在 core 上的
   薄壳（也都是 actor），不存在"特权 frontend"。
6. **恢复只住在一个地方。** 崩溃后的恢复统一走 session resume
   （snapshot + 事件补放）；不存在与之竞争的第二套恢复机制。

## 架构分层

```
┌──────────────────────────────────────────────────────────────┐
│ L4  Surfaces      CLI · headless · server · scheduler        │
│                   session 管理 · 交互协议 · observability    │
├──────────────────────────────────────────────────────────────┤
│ L3  Agent layer   spec · agent loop · context assembly ·     │
│                   tools/workspace · MCP · skills · provider  │
│                   · multi-agent                              │
├──────────────────────────────────────────────────────────────┤
│ L2  Effect        hooks → permission → budget → execute      │
│     pipeline      （所有副作用的唯一通道，关卡判定入 log）   │
├──────────────────────────────────────────────────────────────┤
│ L1  Durability    event store · snapshot-resume ·            │
│                   activity 语义 · workspace 快照(rewind 用)  │
├──────────────────────────────────────────────────────────────┤
│ L0  Kernel        actor · mailbox · bus · envelope           │
└──────────────────────────────────────────────────────────────┘
```

上层只依赖下层。L0/L1 对"agent"一无所知；L2 对"LLM"一无所知。

---

## L0 — Kernel

- **Actor**：一个 id、一个 mailbox（`asyncio.Queue`）、一个 behavior。
  逐条处理消息，没有共享可变状态。并发来自"很多个 actor"。
- **Bus**：进程内 transport。`send(to, msg)` 点对点；`publish(topic, msg)`
  pub/sub 扇出。bus 是 ephemeral 的——**任何会影响 run 结果的输入，
  必须先 journal 成 event 再被消费**（见 L1），bus 只负责搬运。
- **Envelope**：不可变，携带 `id / causation_id / correlation_id / sender /
  target / type / payload / ts`。command 处理按 `Envelope.id` 幂等去重
  （actor 在自己的 stream 里记录已处理的 command id），"command 可重试"
  才成立。
- **失败处理**：actor 未捕获异常 → 发 `ActorCrashed` event → run 标记
  failed。**没有自动 restart 策略**——恢复统一走 session resume（原则 6），
  避免两套恢复机制互相竞争。反复崩溃的 run 停在 failed 状态等人工处理，
  不会热循环。

## L1 — Durability

### Durability 模型：journal 一切输入，snapshot-resume，不做 code replay

这是全设计最重要的取舍。Temporal 式确定性 code replay 需要稳定 activity
id、确定性协程调度、divergence 检测——一个数周级的引擎项目；而 agent loop
的全部状态不过是（消息列表、turn 计数、待处理 tool call）。我们用三件更
便宜的东西拿到同样的用户可见能力：

1. **所有外部输入 journal 成 event，先落盘再消费。** 用户消息、steering、
   interrupt、审批应答、timer 到期——任何 workflow 能观察到的输入，
   都以 `InputReceived` 类 event append 进该 run 的 stream，然后才进入
   处理。崩溃时不丢审批、不丢插话；历史完整可审计。
2. **State 是 event log 的纯 fold。** `state = fold(apply, events)`，
   apply 是纯函数、不执行任何代码副作用。因此对话状态永远可从 log 重建。
3. **Snapshot-resume。** 在 turn 边界给 actor state 打 snapshot；
   resume = 加载最新 snapshot + fold `seq > N` 的 events + 继续 loop。
   不重放代码路径，没有确定性纪律要负担。

**挂起是显式状态，不是任意点 park。** 审批、timer、人工输入全都发生在
turn/tool-call 边界。run 进入 `WAITING_APPROVAL` / `WAITING_INPUT` 状态
（本身是 event），待等的输入作为 event 到达后 loop 继续。等几分钟或几天
成本相同，进程死了也一样——这正是原设计想要的"durable park"，
但不需要 replay 引擎。

### Activity 语义

- activity = 一次副作用执行的记录单元：`ActivityStarted` 先落盘 →
  执行 → `ActivityCompleted{result}` / `ActivityFailed` 落盘。
- **at-least-once + in-doubt 检测**：崩溃发生在"执行后、落盘前"时，
  恢复看到有 `Started` 无 `Completed`，将其标记为 in-doubt 并上浮
  （报错或转人工），**绝不静默重跑**——bash 不幂等，LLM 调用要花钱。
- **retry 是 activity 的通用属性**：retry/backoff、rate limit 处理、
  model fallback 是 activity 级策略，所有副作用共享。
- **协作取消是 activity 的一等能力**：activity 持有 cancel signal，
  被打断时记录 `ActivityCancelled{partial_output}`。跑了 10 分钟的 bash
  必须能被 Esc 杀掉——interrupt 语义建立在这之上（见 L3/L4）。
- **timeout 是 durable timer**，与 run 竞速的一条记录在案的定时器，
  绝不在关卡代码里读墙钟（重建时时间不同会得出不同结论）。

### Checkpoint 与 workspace

两种"快照"，语义完全不同，不混为一谈：

- **对话 state snapshot**：event log 的派生缓存，加速 resume。
  可随意丢弃——删掉只损失 fold 时间，不损失任何东西。
- **Workspace 快照**：**一等状态，不是派生物**。文件系统永远不可能从
  event log 重建（activity 结果被记录，但不重放）。实现为 workspace 内
  per-turn `git commit`（便宜、可 diff），只服务 **rewind/fork**，
  只在显式 barrier 打点（turn 边界 + 子 agent 静默）。**不可随意删除**
  ——删掉即永久失去该回退点。
- **崩溃恢复绝不碰文件系统。** 单进程下崩溃时文件系统本来就活着、
  已在 head 附近；恢复只重建对话 state。回滚文件系统是 rewind 的
  用户主动行为，不是恢复的一部分。
- **bash 可以逃逸 workspace**（pip install、网络调用、写外部路径）。
  rewind 回退的是 workspace 内的文件，逃逸的副作用明确不在承诺内。

## L2 — Effect pipeline

所有副作用（tool、MCP、LLM、spawn 子 agent、bash……）流经唯一管线：

```
effect
  │
  ▼
[1] Hooks (pre)      # v0: observe + block（exit code），不做改写
  ▼
[2] Permission       # allow / ask / deny（policy 是数据）
  │                  #   ask ⇒ ApprovalRequested event，run 进
  │                  #   WAITING_APPROVAL，应答以 event 到达后继续
  ▼
[3] Budget           # turns/tokens/cost 从 event stream 统计；
  │                  # timeout 走 durable timer（见 L1）
  ▼
[4] Execute          # 以 activity 执行（retry/cancel 语义见 L1）
  ▼
[5] Hooks (post)
```

- **关卡判定在记录边界之内**：整条管线对一个 effect 产生**一条**
  `EffectResolved` event，携带全部关卡判定（hook 结果、permission 判定、
  budget 判定）。恢复时读记录值，不重跑 hook 脚本、不重读 policy 文件
  ——hook 是有副作用的外部脚本，绝不能在恢复路径上再执行一次。
  单条 event 也避免每个 tool call 4-5 条官僚 event 淹没日志。
- **hooks 是管线机件，不是 effect**——不递归进管线自身；其执行记录在
  `EffectResolved` 里。v0 只支持 observe + block，改写输入（mutation）
  连同它带来的顺序与缓存问题一起推迟。
- **每种关卡结果都定义"模型看到什么"**。Anthropic API 要求每个
  `tool_use` 都有对应 `tool_result`，且 agent loop 在多数失败后应当继续：
  - deny → `tool_result{is_error: true, reason}`，loop 继续；
  - hook block → hook 的消息作为 error tool_result，loop 继续；
  - 审批被拒 → 同上，附拒绝理由；
  - budget 超限 → 让模型收尾的最后一条消息 + 优雅停止（`LimitExceeded`
    event），不是掐断；
  - activity 失败（重试耗尽）→ error tool_result，loop 继续。
  "给模型的错误"和"给用户的错误"是两个 surface，分开设计。
- **permission modes 是 loop 行为，不是 policy 枚举值**。每个 mode 是
  一组数据：工具面过滤 + prompt 注入 + 跃迁规则。例：`plan` = 只读工具面
  + 计划指令注入 + 专用 `ExitPlanMode` 工具（其审批通过即触发 mode 跃迁，
  跃迁本身是 event）；`acceptEdits` 依赖 tool 的**类别**标签
  （edit-class / execute-class / read-class，tool 定义数据的一部分）。
  hook 与 mode 的优先级明确：`bypass` 不跳过 hooks。

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
  thinking: { budget_tokens: 4096 }   # reasoning 预算是一等配置

system_prompt_file: prompts/researcher.md   # 只是拼装的一层，见下

tools: [read_file, edit_file, bash, web_search]   # 引用 tool 定义（数据）

mcp:
  - name: github
    transport: stdio           # schema 同时定义 http + auth，实现推迟
    command: ["github-mcp-server"]
    allowed_tools: [search_code, get_file_contents]

skills:                        # Claude Code skill 约定：目录 + markdown + frontmatter
  - ./skills/research

agents: [summarizer]           # 允许 spawn 的子 agent 白名单

permissions:
  mode: default                # mode 是 loop 行为的数据描述（见 L2）
  rules:
    - { tool: read_file, action: allow }
    - { tool: edit_file, path: "src/**", action: allow }
    - { tool: bash, action: ask }

hooks:
  pre_tool_use: ["./hooks/lint-check.sh"]   # v0: observe + block

context:
  compaction: { trigger_ratio: 0.8 }   # 见 context assembly
  tool_output_limit: 30000             # 每个 tool result 的截断上限
  memory_files: true                   # CLAUDE.md 式指令文件注入

limits:
  max_turns: 40
  max_tokens_total: 500_000
  timeout_s: 900
```

- **tool 定义本身是数据**：description、JSON schema、类别标签
  （read/edit/execute-class，供 `acceptEdits` 等 mode 使用）、per-tool
  配置（bash timeout、输出截断上限）。内置 tool 以数据文件形式随包分发，
  spec 里的 `tools:` 是对这些定义的引用 + 收窄。
- 配置分层从简：**spec + 单个 project settings 文件**两层起步，
  标量覆盖、permission rules 按文档化顺序拼接（local > project > spec）；
  三层与更细的合并语义等真实冲突出现再加。

### Context assembly（一等组件）

上下文不是"一个 system prompt 文件 + 消息列表"，而是一个有名字的组件，
负责 `fold(event log) → provider 请求`：

- **System prompt 是拼装的**，顺序固定：harness 基础指令 → 环境块
  （cwd、git 状态、日期）→ memory 文件层（CLAUDE.md 按目录层级合并）→
  tool/skill/子 agent 目录（模型不知道 `summarizer` 存在就永远不会
  spawn 它——目录注入是 multi-agent 可用的前提）→ spec 的 system prompt。
- **Prefix 稳定性是显式不变量**（prompt caching 的经济性约 10x，
  没有它 agent loop 在经济上不可用）：system prompt 与 tool schema 排序
  稳定，cache breakpoint 由 loop 放置；任何会打爆 prefix 的操作
  （配置中途变更）要么禁止要么显式换代。LLM activity 的 event 记录
  cache_read/cache_write token，budget 关卡按真实计费口径记账。
- **Tool 结果截断**：per-tool 输出上限，超限截断并告知模型被截断了
  ——一条 `cat large.json` 不能毁掉上下文和预算。
- **Compaction 是 recorded activity**：它本身是一次 LLM 调用
  （非确定性副作用），产出 `ContextCompacted{summary, kept_range}` event，
  **改变后续 fold 的结果**。跨 compaction 边界的 fork/rewind 语义因此
  是良定义的：fold 到哪个 seq，就得到哪个视图。

### Agent loop

- agent loop 是一个普通的 async loop，其中每次 LLM 调用、每个 tool 执行、
  每次 spawn 都是走 L2 管线的 activity。durability 来自 L1 的
  journal + snapshot，loop 代码不背确定性纪律。
- **并行 tool call 是常态**：一条 assistant 消息含 N 个 `tool_use` 时，
  每个 call 独立过管线；判定为 allow 的并发执行，判定为 ask 的按序等审批
  （审批挂起不阻塞已放行的 call）；完成 event 按到达顺序落盘。
- **Steering 与 interrupt**：用户插话 journal 成 event 后在 turn 边界
  被 loop 消费；interrupt 则通过 activity 协作取消立即生效
  （`ActivityCancelled`），被打断的 tool call 以
  `tool_result: "[interrupted by user]"` 呈现给模型的下一 turn。
- **Streaming 的持久化边界**：token delta 只走 bus（**显式 ephemeral**，
  这是原则 2 的正版应用而非违反）；持久化的是组装完成的 assistant
  message（一条 event）。LLM activity 重试发生在已流出部分输出之后时，
  发 `TurnDiscarded` event，前端据此渲染（"重试中"并重新开流），
  绝不静默替换用户已看到的文本。

### Tools 与 workspace

- 内置 tool 套件（file read/write/edit、bash、glob/grep、web
  fetch/search）建立在 workspace 抽象上：工作目录、路径边界、bash 沙箱
  等级。worktree 级隔离支持多 agent 并行改文件。
- workspace 的持久化与 rewind 语义见 L1（per-turn git commit，
  bash 逃逸不在承诺内）。

### MCP

- **server 生命周期是带外运行时状态，不进 event 模型**：resume/重启后
  server 重新拉起；原型假定 MCP server 无状态（per-call stateless），
  这是文档化的契约。实现用官方 MCP Python SDK 管理 client/session。
- **发现的 tool schema 记录为 event**（它们进入 LLM 的 tool 列表，
  是影响 run 结果的外部输入）；`tools/list_changed` 同理。
- 只有 `McpToolCalled/Returned` 是 activity。spec schema 里保留
  `transport: http` + auth 字段，实现（OAuth 流程、凭据存储）推迟。

### Provider

- 薄接口（`complete(request) → stream`），streaming 原生，原型只有
  Anthropic 实现。请求对象携带 cache breakpoint 与 thinking 配置。

### Multi-agent

- 三种模式：**spawn/await**（子 agent 作为 activity，可扇出）、
  **handoff**（移交后退出）、**pub/sub 协作**（blackboard topic）。
- **子 agent 的意义在上下文隔离**：child 烧自己的 window，只有符合
  result contract 的最终报告回流 parent（contract 在子 agent spec 的
  `description`/输出约定里声明）。
- **审批路由**：child 的 `ask` 沿 correlation id 冒泡到 session 的
  frontend——审批的永远是人，不是 parent agent。
- **权限继承**：child 的有效权限 = child spec ∩ parent 有效权限，
  子 agent 不能越过 parent 的边界。
- 可审计性保证是 per-stream 的：每个 agent 的 stream 完整、
  causation/correlation 链路完整；跨 actor 的消息交错不保证确定性重现
  （见非目标）。

## L4 — Surfaces

### Session 管理

- session = correlation id + 它名下的 stream 闭包（含子 agent）。
- **list**：枚举 store。**resume**：snapshot + fold（见 L1）。
- **fork/rewind 只发生在 checkpoint barrier 上**（turn 边界 + 全部子
  agent 静默 + workspace commit 存在）：fork 复制 stream 闭包在 barrier
  处的一致切面；rewind = fork + workspace 恢复到对应 commit。
  任意 seq N 处的 fork 不提供——跨 stream 的因果一致切割不值得做。

### 交互协议

- frontend 是普通 actor：订阅输出 topic，向 run 发输入（journal 后生效）。
- 输出事件流：turn 开始/结束、token delta（ephemeral）、tool call 及其
  permission 判定、`ApprovalRequested`、`TurnDiscarded`。CLI 先做 turn
  粒度渲染，token streaming 是纯增量，协议不变。
- 协议预留（原型不实现）：slash command 调用、附件/图片消息类型。

### 运行形态

- core 是库。CLI（`agentrunner run <spec> "task"`）、headless 单发、
  server（HTTP/WS 暴露同一协议）都是薄壳。

### Scheduler 与 triggers

- scheduler 是发布 `RunAgent` command 的普通 actor；webhook 触发 =
  server 壳收到请求后发同一条 command。command 按 Envelope.id 幂等，
  重试不会拉起重复 run。

### Observability

- event log 就是 trace。`inspect` CLI 渲染时间线：turns、每个 tool call
  的 `EffectResolved`（为什么放行/拦下）、子 agent 树
  （correlation/causation）、token/cost（含 cache 命中）消耗。

---

## 测试策略

- **Activity 结果缓存式 replay 测试**：录一次真实 run，测试中 activity
  直接返回记录值——不打 LLM、不碰网络，毫秒级重走整个 loop。这只需要
  "activity 结果被记录"，不需要确定性 code replay 引擎。
- agent 行为变化体现为 event log 的 diff，review 的是决策序列。
- kernel/L1/L2 普通单测；spec loader 用坏 spec 的错误信息做黄金测试；
  in-doubt activity、审批挂起恢复、interrupt 各有专门的崩溃注入测试。

## 已定决策

| # | 决策点 | 选择 | 理由 |
|---|--------|------|------|
| 1 | 语言 | Python 3.12+, asyncio | actor 映射到 task + queue；pydantic；MCP + Anthropic SDK 成熟。 |
| 2 | 进程模型 | 单进程，in-memory bus | 原型简单；边界清晰，分布式化是换 transport。 |
| 3 | 持久状态 | 只有 event log 和 workspace 两处；bus/delta ephemeral | durability 语义集中，"什么会丢"一目了然。 |
| 4 | 输入语义 | 一切外部输入先 journal 成 event 再消费 | 审批/steering 不丢、可审计；bus 才允许 ephemeral。 |
| 5 | Durability 模型 | journal + turn 边界 snapshot-resume + 显式等待状态；**不做** Temporal 式 code replay | 同样的用户可见能力（crash 恢复、长审批、fork），~10% 成本；loop 不背确定性纪律。 |
| 6 | Activity 语义 | Started/Completed 双落盘，at-least-once + in-doubt 上浮，协作取消，通用 retry | bash 不幂等、LLM 要花钱，静默重跑不可接受；interrupt 建立在取消上。 |
| 7 | Checkpoint 语义 | 对话 snapshot 是可弃缓存；workspace 快照是一等状态（per-turn git commit），只服务 rewind/fork | 文件系统不可从 log 重建；崩溃恢复不碰文件系统。 |
| 8 | 副作用治理 | 单一 effect pipeline，四关卡，一条 `EffectResolved` event，关卡在记录边界内 | permission/审批/hooks/预算是一个机制；恢复不重放 hook 副作用；日志不被官僚 event 淹没。 |
| 9 | 失败面向模型 | deny/block/失败渲染为 error tool_result，loop 继续；超预算优雅收尾 | API 要求 tool_use↔tool_result 配对；agent 要能对失败自适应。 |
| 10 | Permission modes | mode = 工具面过滤 + prompt 注入 + 跃迁规则（数据） | plan/acceptEdits 是 loop 行为，枚举值表达不了。 |
| 11 | Hooks | v0 只 observe + block；是管线机件不是 effect | 改写带来顺序/缓存/重放问题，推迟；避免管线递归。 |
| 12 | 存储后端 | JSONL per stream，藏在 `EventStore` 接口后 | 可读可 diff；需要时换 SQLite。 |
| 13 | Spec 格式 | YAML → pydantic；tool 定义也是数据 | 声明式、可 review；原则 4 落到 tool 层。 |
| 14 | 运行形态 | core 是库；CLI/headless/server 是薄壳 | 一套 core 支撑所有 surface。 |
| 15 | Provider | 薄接口 + 仅 Anthropic，streaming 原生，cache/thinking 一等 | 不过度抽象；caching 是经济性前提。 |
| 16 | Skill 格式 | 沿用 Claude Code 约定 | 生态兼容，不发明格式。 |
| 17 | MCP 生命周期 | 带外运行时状态；只有 tool 调用是 activity；发现的 schema 记录为 event | server 状态不可 event 化；schema 是影响结果的输入。 |
| 18 | Event schema 版本化 | 不 migration；`RunStarted` 记版本，不匹配拒绝 resume | 原型 re-run 比 migrate 便宜；失败要响亮不要发散。 |

## Roadmap

以 walking skeleton 优先——让真实的 agent 负载塑造下层 API，
而不是给玩具 actor 造两个 milestone 的框架：

1. **M1 — Walking skeleton**：最小 spec loader、Anthropic provider、
   朴素 asyncio agent loop、2-3 个内置 tool（read_file/edit_file/bash）、
   append-only JSONL event journal（先只记录，不做 source of truth）、
   最小 CLI。端到端跑通一个真 agent。
2. **M2 — Durability + pipeline**：event fold 成为 state 的正源、
   turn 边界 snapshot、resume、显式等待状态；effect pipeline 四关卡 +
   `EffectResolved`；permission rules + 审批流；budgets；
   hooks（observe/block）。崩溃注入测试。
3. **M3 — 交互与上下文**：streaming 协议、steering/interrupt/协作取消、
   并行 tool call、context assembly（拼装、截断、compaction、caching）、
   session list/resume。
4. **M4 — 生态接入**：MCP（生命周期 + schema 记录）、skills、
   memory 文件、配置分层（两层）。
5. **M5 — Multi-agent + surfaces 收尾**：spawn/await、handoff、
   pub/sub；审批路由与权限继承；scheduler；fork/rewind（barrier 语义）；
   `inspect` 时间线；server 壳。
