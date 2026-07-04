# AgentRunner — Design

一个灵活的 agent runner/harness，目标能力对标 Claude Code 一类的 agent
harness。原型级实现：设计和代码尽量干净，零 legacy，不考虑 backward
compatibility。本文档是活的设计记录，随讨论逐步生长。

当前阶段：**高层设计经三方独立 review 后修订收口，细节按 roadmap 逐项展开。**

## 目标

- 通过声明式 spec 定义并运行一个或多个 LLM agent，agent 的一切行为皆可配置。
- 每次运行都是 durable 的：挺过进程死亡、可审计、可恢复、可 fork。
- 交互式：streaming 输出、运行中途 steering 与 interrupt、审批。
- 长任务形态：后台运行与 attach/detach、artifact 产出、
  goal/loop 驱动的迭代执行。
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
2. **一切历史皆 event。** 持久状态 = event log（历史与决策的 source
   of truth）+ workspace（世界状态）+ 接口后的 ref-addressed blob store
   （`SnapshotStore`、`ArtifactStore`、任务日志共用一个 CAS blob 模块）。
   fold 永不读 store；event 只引用 opaque ref，blob 先于引用它的 event
   落盘。除此之外的一切——bus 上的消息、token delta、内存中的 state
   ——都是 ephemeral 或可从 event log 重建的派生物。
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

- **Actor**：一个 id、一个 mailbox（channel）、一个 behavior。
  逐条处理消息，没有共享可变状态。并发来自"很多个 actor"。
- **Bus**：进程内 transport。`send(to, msg)` 点对点；`publish(topic, msg)`
  pub/sub 扇出。bus 是 ephemeral 的——**任何会影响 run 结果的输入，
  必须先 journal 成 event 再被消费**（见 L1），bus 只负责搬运。
  跨进程部署时 bus 契约分**双通道**：ephemeral topic（可丢，delta 类）
  与 guaranteed send（接收方 journal 后 ack）；frontend 重连必须从
  event log 对账未决状态，不依赖 bus 补投。
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

**等待状态是一个注册表，配一张可中断性表**：`WAITING_INPUT` /
`WAITING_APPROVAL` / `WAITING_TASKS`（后台任务未清）/ `WAITING_TIMER`
（driver 定时）都是同一个等待事件的 reason 变体。interrupt 在等待中
journal 进来即把 run 带出等待——未决审批按 denied-by-interrupt 解决，
对应 call 渲染为 `[interrupted by user]` 的 error 结果；**已配对的后台
任务例外**：它的 handle 已是唯一配对结果，取消通知走 task-notice 输入
通道，绝不发第二个 tool result。durable timeout 在等待中到期走同一条
路径。

### Activity 语义

- activity = 一次副作用执行的记录单元：`ActivityStarted` 先落盘 →
  执行 → `ActivityCompleted{result}` / `ActivityFailed` 落盘。
- **凭据 redaction**：结果落盘前，对进程已知的凭据值（`*_API_KEY` 类
  环境变量的字面值）替换为 `[REDACTED:VAR]`。harness 自身绝不把凭据写入
  spec/event；但 tool 输出可能携带任意 secret——redaction 是尽力而为的
  兜底，属文档化残余风险。log 文件权限 0600，永不入 git。落盘路径预留
  （当前为恒等的）**scrub 阶段**；`EventStore` 接口预留 at-rest 加密位
  ——fold 完整性堵死事后擦除，唯一自洽的擦除点在写入之前。
- **at-least-once + in-doubt 检测**：崩溃发生在"执行后、落盘前"时，
  恢复看到有 `Started` 无 `Completed` → in-doubt。崩溃几乎必然砸中
  in-flight activity（agent 的墙钟全在 LLM 调用和 bash 里），所以
  in-doubt 的处置是**按 tool 类别的数据化策略**，不是一刀切转人工：
  LLM 调用 → 自动重发（复用 `TurnDiscarded` 渲染）；read-class 与
  `idempotent: true` → 直接重跑；execute/edit-class → **不重跑**，
  渲染 `[interrupted by crash]` error 结果、loop 继续；"上浮转人工"
  只留给显式配置的高危工具。非幂等操作绝不静默重跑的红线不变——
  它们根本不重跑；headless/无人值守 run 也因此不会卡死在人工 triage。
- **retry 是 activity 的通用属性**：retry/backoff、rate limit 处理、
  model fallback 是 activity 级策略，所有副作用共享。
- **声明式幂等是 in-doubt 自动重跑的唯一通道**：tool/activity 定义可
  标注 `idempotent: true`（默认 false）——只读 verifier、artifact
  重发布等都引用这一个机制；未声明者 in-doubt 一律上浮，绝不静默重跑。
- **后台 activity**：`bash` / `spawn_agent` 支持 `background: true`。
  `ActivityStarted` 额外记录 task_id（= call id）、pgid、log_ref、
  `on_run_end: cancel|await`；输出重定向到 log_ref（完成时全量入 blob
  store，tail 截断后入 event）。取消、timeout、retry、redaction 语义
  与前台完全相同——不同的只是模型何时看到结果（见 L3 Agent loop）。
- **协作取消是 activity 的一等能力**：activity 持有 cancel signal，
  被打断时记录 `ActivityCancelled{partial_output}`。跑了 10 分钟的 bash
  必须能被 Esc 杀掉——interrupt 语义建立在这之上（见 L3/L4）。
- **取消的终态以进程组为准**：bash 以独立进程组启动
  （start_new_session），取消 = 对整组 SIGTERM → 宽限 → SIGKILL，
  **确认组内进程全部退出后**才 journal `ActivityCancelled`（管道以
  有界超时 drain 出 partial_output）。否则 `npm install` 的孤儿进程会在
  "取消"之后继续写 workspace，污染 barrier 和 rewind。MCP 的取消通知
  多数 server 不理会——按 best-effort 处理，journal 为
  cancelled-unconfirmed。
- **timeout 是 durable timer**，与 run 竞速的一条记录在案的定时器，
  绝不在关卡代码里读墙钟（重建时时间不同会得出不同结论）。

### Checkpoint 与 workspace

两种"快照"，语义完全不同，不混为一谈：

- **对话 state snapshot**：event log 的派生缓存，加速 resume。
  可随意丢弃——删掉只损失 fold 时间，不损失任何东西。
- **Workspace 快照**：**一等状态，不是派生物**。文件系统永远不可能从
  event log 重建（activity 结果被记录，但不重放）。快照藏在
  **`SnapshotStore` 接口**后，event 只引用 opaque 的 snapshot ref——
  上层语义不与任何具体机制耦合，只服务 **rewind/fork**，只在显式
  barrier 打点（见 L4 `CheckpointBarrier`）。快照 **pinned until
  explicit GC**——rewind 之后较新的快照不会变得不可达。
- **默认 backend 是 shadow repo**：独立的 `GIT_DIR` 放在 harness 数据
  目录下、`GIT_WORK_TREE` 指向 workspace——对用户自己的 repo 完全隐形：
  不污染 HEAD/index、不会被误 push，agent 通过 bash 做 `git checkout` /
  `git reset` 也打不断快照链。备选 backend：archive copy；`none`
  （rewind/fork 优雅不可用，其余功能不受影响）。git 只是默认实现，
  不是设计依赖。
- **排除策略显式化**：harness 级 exclude 列表（node_modules/venv/build
  类），被排除的路径文档化为 rewind 范围外。快照延迟在 S7（STAGES
  延迟批次）用真实大仓库基准测试后再定粒度。
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

- **关卡判定在记录边界之内，按持久化时点拆分**：pre-hook 结果 +
  permission 判定 + budget 判定在关卡判定终结后（放行或拦下——拦下时
  其后没有 `ActivityStarted`）、执行开始**之前**作为一条 `EffectResolved`
  event 落盘（ask 路径：`ApprovalRequested` **自身携带此前已完成关卡
  的判定**——pre-hook 可能已执行副作用，这个事实必须先于可能数天的
  挂起落盘；应答到达后 `EffectResolved` 作终态汇总并引用该 id）；post-hook 结果随 `ActivityCompleted`
  落盘。单一落盘点装不下整条管线——它跨越 durable 的 `ActivityStarted`
  和可能挂几天的审批。恢复时读记录值，不重跑 hook 脚本、不重读 policy
  文件——hook 是有副作用的外部脚本，绝不能在恢复路径上再执行一次；
  进了关卡但没有 `EffectResolved` 的 effect 与 activity 同等享受
  in-doubt 上浮，绝不静默重过关卡。happy path 下一个 effect 仍只有
  一条关卡 event，不淹没日志。
- **预算是 reserve-then-settle 的**：关卡时刻对预估成本（LLM 调用按
  max_tokens、tool 按类别估值）做原子预留，与已 fold 的消耗 + 未结清
  预留一起比对上限；`ActivityCompleted` 时按实际结算，预留集是 fold
  state 的一部分。否则 N 个并行 call 各自对着同一个过期计数器放行，
  合起来超支 N 倍。
- **hooks 是管线机件，不是 effect**——不递归进管线自身；执行记录随
  管线判定持久化（pre-hook 在 `EffectResolved`，post-hook 随
  `ActivityCompleted`）。v0 只支持 observe + block，改写输入（mutation）
  连同它带来的顺序与缓存问题一起推迟。
- **每种关卡结果都定义"模型看到什么"**。所有 provider 都要求 tool call
  与结果配对（Anthropic 按 call id、Gemini 按数量+位置且更严格），
  且 agent loop 在多数失败后应当继续：
  - deny → `tool_result{is_error: true, reason}`，loop 继续；
  - hook block → hook 的消息作为 error tool_result，loop 继续；
  - 审批被拒 → 同上，附拒绝理由；
  - budget 超限（run 级 token/cost/turns）→ 让模型收尾的最后一条消息 +
    优雅停止（`LimitExceeded` event），不是掐断；结构性限制（spawn
    深度/扇出，同在 budget 关卡校验）→ error 结果，loop 继续；
  - activity 失败（重试耗尽）→ error tool_result，loop 继续。
  "给模型的错误"和"给用户的错误"是两个 surface，分开设计。error 结果的
  线上形态由各 provider 定义（Anthropic 有 `is_error` 标志；Gemini 没有，
  约定为 `functionResponse.response` 内的 error 载荷）。
- **permission modes 是 loop 行为，不是 policy 枚举值**。每个 mode 是
  一组数据：工具面过滤 + prompt 注入 + 跃迁规则。例：`plan` = 只读工具面
  + 计划指令注入 + 专用 `ExitPlanMode` 工具（其审批通过即触发 mode 跃迁，
  跃迁本身是 event）；`acceptEdits` 依赖 tool 的**类别**标签
  （edit-class / execute-class / read-class，tool 定义数据的一部分）。
  hook 与 mode 的优先级明确：`bypass` 不跳过 hooks。
  **工具面分两级**：mode 的过滤作用于 **permitted 面**（L2 关卡数据，
  随 mode 任意变、deny 拦截）；**advertised 面**（进 prefix 的 tools
  参数与目录）session 内稳定——否则每次进出 plan mode 都打爆
  tools 级缓存。`ExitPlanMode` 常驻 advertised 面。
- **path 规则的边界诚实**：path 规则只约束文件类 tool；bash 天然是旁路
  （一条 `sed -i` 就能改写 `src/**`）。因此 rules schema 对 bash 提供
  **命令模式匹配**（`{tool: bash, command: "git *", action: allow}` 式），
  bash 的可写范围最终由 workspace 沙箱等级闭环（沙箱等级决定 bash
  可写路径，与 path 规则同源配置）。这层关系明文写出，不假装 path 规则
  覆盖一切。
- **路径匹配基于 realpath**：所有文件类 tool 的路径在 permission 匹配与
  边界检查前一律 resolve（symlink、`..` 归一化）；resolve 后落在
  workspace 外 → deny。`src/../../etc/passwd` 匹配不上 `src/**`，
  workspace 内指向外部的 symlink 也写不穿边界。

## L3 — Agent layer

### Agent spec

agent 完全由声明式 spec（YAML → 强类型 struct）定义，加载时校验、
坏 spec 报精确错误。spec 是模板，**agent instance** = spec + 运行时输入
（task、correlation id、parent）。

```yaml
# agents/researcher.yaml
name: researcher
description: Deep-dives a topic and reports findings.

model:
  provider: gemini             # 薄 provider 接口；gemini 为主、anthropic 次
  id: gemini-2.5-pro
  max_tokens: 8192
  thinking: { budget_tokens: 4096 }   # 通用能力，见 Provider；provider 各自映射
  # API key 只从环境变量读（如 GEMINI_API_KEY），绝不写进 spec/仓库

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
    - { tool: bash, command: "git status*", action: allow }
    - { tool: bash, action: ask }        # 兜底；path 规则约束不了 bash（见 L2）

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
  （read/edit/execute/**wait**-class——wait-class 即"向用户提问"类
  工具，execute = 进入 `WAITING_INPUT` park 而非阻塞 activity，
  跨崩溃不被 in-doubt 误杀；类别同时供 `acceptEdits` 等 mode 与
  in-doubt 策略使用）、per-tool 配置（bash timeout、输出截断上限）。
  内置 tool 以数据文件形式随包分发，spec 里的 `tools:` 是对这些定义
  的引用 + 收窄。
- 配置分层从简：**spec + user settings + project settings** 三个来源，
  标量覆盖、permission rules 按文档化顺序拼接（user > project > spec）；
  更细的合并语义等真实冲突出现再加。user settings 属于用户机器，
  project settings 随 repo 走——这个出身差异是信任模型的依据。
- **policy 热更新是 event**："always allow"类写回 settings 的操作先
  journal `PolicyChanged` event 再写盘（崩溃后幂等补做）；harness 配置
  路径显式排除出快照/rewind 范围——否则 rewind 会让已收紧的 deny
  静默复活。
- **信任模型**：spec 与 settings 等同于"你选择执行的代码"。可执行配置
  （hooks）只从 spec 与 user 层生效；**project 层（随 repo 走的
  文件）里的 hooks 被忽略**，除非用户对该 workspace 做过一次显式 trust
  确认——否则 clone 一个不受信任的 repo 就等于交出任意代码执行权，
  整个 permission 系统被绕过。memory 文件按不可信内容对待（只进 prompt，
  不获得任何执行权）。原型是单用户自担模式，但边界必须明文。

### Context assembly（一等组件）

上下文不是"一个 system prompt 文件 + 消息列表"，而是一个有名字的组件，
负责 `fold(event log) → provider 请求`：

- **System prompt 是拼装的**，顺序固定：harness 基础指令 → 环境块
  （cwd、git 状态、日期——**在 session start 冻结进 fold state**，之后的
  环境变化以追加消息进入上下文，绝不改写 prefix：git 状态每 turn 都变，
  不冻结的话 harness 会亲手打爆下面的 caching 不变量）→ memory 文件层
  （CLAUDE.md 按目录层级合并）→ tool/skill/子 agent 目录（模型不知道
  `summarizer` 存在就永远不会 spawn 它——目录注入是 multi-agent 可用的
  前提）→ spec 的 system prompt。
- **Prefix 稳定性是显式不变量**（prompt caching 的经济性约 10x，
  没有它 agent loop 在经济上不可用）：system prompt 与 tool schema 排序
  稳定，cache 断点由 loop 放置；任何会打爆 prefix 的操作
  （配置中途变更）要么禁止要么显式换代。context assembly 只负责保证
  prefix 稳定这个**与 provider 无关**的不变量；缓存怎么落地
  （Anthropic 的显式 `cache_control` 断点 vs. Gemini 的 context cache
  句柄）由各 provider 实现。LLM activity 的 event 记录归一化的
  cache_read/cache_write token，budget 关卡按真实计费口径记账。
- **Tool 结果截断**：per-tool 输出上限，超限截断并告知模型被截断了
  ——一条 `cat large.json` 不能毁掉上下文和预算。
- **Compaction 是 recorded activity**：它本身是一次 LLM 调用
  （非确定性副作用），产出 `ContextCompacted{summary, kept_range}` event，
  **改变后续 fold 的结果**。跨 compaction 边界的 fork/rewind 语义因此
  是良定义的：fold 到哪个 seq，就得到哪个视图。

### Agent loop

- agent loop 是一个普通的 goroutine 循环，其中每次 LLM 调用、每个 tool 执行、
  每次 spawn 都是走 L2 管线的 activity。durability 来自 L1 的
  journal + snapshot，loop 代码不背确定性纪律。
- **并行 tool call 是常态**：一条 assistant 消息含 N 个 tool call 时，
  每个 call 独立过管线；判定为 allow 的并发执行，判定为 ask 的按序等审批
  （审批挂起不阻塞已放行的 call）；完成 event 按到达顺序落盘。
  **call 的身份由 harness 生成的 call id 定义**（随 event 持久化，
  provider 各自映射到自家配对机制）；到达顺序只是日志事实——context
  assembly 在下一次 LLM 调用前收齐该 turn 全部结果，**按原 call 顺序
  重排**（Gemini 要求 functionResponse 与 functionCall 数量 1:1、
  按位置配对，乱序或缺失直接 400）。
- **异常终止形态是 loop 策略的一部分**：归一化 finish_reason 显式收录
  blocked / malformed_tool_call / recitation 等（Gemini 有一整类
  Anthropic 不存在的形态：MALFORMED_FUNCTION_CALL、SAFETY、零 candidate
  的 promptFeedback.blockReason）。策略：malformed_tool_call 走 activity
  retry（复用 `TurnDiscarded` 渲染路径）；safety/blocked 上浮为用户可见
  错误，不重试。
- **turn 是被定义的**：turn = 一次 LLM 调用 + 它返回的全部 tool call
  的执行周期。snapshot、max_turns、steering 消费点、barrier 候选点
  引用的都是同一个定义；steering 的消费点精确为**最早可配对点**
  （当前 call 结束后、下一次 LLM 调用前），不必等同 turn 的其余 call。
- **Steering 与 interrupt**：用户插话 journal 成 event 后在最早可配对
  点被 loop 消费；interrupt 触发 **turn sweep**——该时刻所有未终态的
  call 一律得到终态：执行中的走协作取消（`ActivityCancelled`）；
  已放行未启动与审批挂起中的落 `EffectAbandoned`（其
  `ApprovalRequested` 随之作废，迟到的应答按 request id no-op——
  否则 crash-resume 后一条迟到的批准会执行用户已用 Esc 放弃的危险
  调用）；全部渲染为 `[interrupted by user]` 呈现给模型的下一 turn。
- **Streaming 的持久化边界**：token delta 只走 bus（**显式 ephemeral**，
  这是原则 2 的正版应用而非违反）；持久化的是组装完成的 assistant
  message（一条 event）。LLM activity 重试发生在已流出部分输出之后时，
  发 `TurnDiscarded` event，前端据此渲染（"重试中"并重新开流），
  绝不静默替换用户已看到的文本。
- **后台 effect 不阻塞 loop**：background call 的立即配对结果就是
  `ActivityStarted` 的 fold 渲染（`{task_id, status: running}`）——
  Gemini 的 1:1 配对当场满足、永不再动；完成时 `ActivityCompleted`
  兼任 pending input，在 turn 边界以**新的 user-role 消息**进入 loop
  （与 steering 同路）。模型结束 turn 时仍有活任务且无其他输入，其处置
  由 `on_run_end` 决定（下一条 quiesce 同源）：`await` → 进入
  `WAITING_TASKS` park（等某个 task 终态、结果回流为新 user-role 消息后
  再决策，直到 task 清空才自然收尾）；默认 `cancel` → 直接走 run 收尾
  epilogue，由 quiesce 槽位协作取消残留 task（fire-and-forget）。被强制
  结束（max_turns 等）同样交 epilogue quiesce 按 `on_run_end` 处置——
  绝不为残留 task 阻塞 loop。`task_output`（读 log，read-class）/
  `task_kill`（协作取消，execute-class）是普通数据定义 tool；进度 tail
  走 ephemeral topic（与 token delta 同 doctrine）。
- **run 的收尾是固定 epilogue**：(1) 按 `on_run_end` quiesce 后台任务
  （`await` 是纯静默等待——完成只入 journal 不再进 loop，且必有
  durable timer 兜底）→ (2) 自动 publish `outputs:` 声明的交付物并
  检查 contract → (3) 切终态 `CheckpointBarrier` → (4) journal 终态
  event。任何给"run 结束"加步骤的 feature 都必须挂进这个序列，
  不得自行定义结束时序。

### Tools 与 workspace

- 内置 tool 套件（file read/write/edit、bash、glob/grep、web
  fetch/search）建立在 workspace 抽象上：工作目录、路径边界、bash 沙箱
  等级。worktree 级隔离支持多 agent 并行改文件。
- workspace 的持久化与 rewind 语义见 L1（`SnapshotStore` 快照，
  只在 `CheckpointBarrier` 打点；bash 逃逸不在承诺内）。

### Artifacts

- **`ArtifactStore` 是 SnapshotStore 模式的第二个实例**：接口后的
  content-addressed blob store（ref = `sha256:<hex>`）。一切语义
  （名字、版本、mime、provenance）都在 event log 里——per-session 的
  artifact 索引是 `ArtifactPublished` events 的纯 fold。目录型 artifact
  是一个 manifest（`{relpath, ref}` 列表，其自身 hash 即 ref）。
- **publish 是 tool，因此是 effect，因此是 activity**：内置
  `publish_artifact{name, path, …}` 走完整四关卡（DLP 类 pre-hook 可拦、
  file-class path 规则 + realpath 适用、per-publish 大小上限）。
  **发布即持久**（blob 先落盘、event 随后 append），与 run 是否结束
  无关。
- **版本按 publishing stream 本地排序**：version 是 (name, stream)
  内的序数，由该 stream 自己的 seq 决定——符合 per-stream 审计保证；
  session 级索引是展示层合并，跨 stream 同名不产生全局版本序。
- **`outputs:` 声明 = 交付物 contract**：spec 声明期望产出（name、
  path、required），run 收尾 epilogue 自动 publish 并检查 contract
  ——缺 required 输出渲染为 parent 的 error 结果，loop 继续。
  交付物 contract 与过程中的协调对象（plan 等）是两条路径，不混用。
- **审批载荷是 artifact ref**：`ApprovalRequested{payload_ref}` 引用
  一份版本化、可渲染的 artifact——plan 审批 = mid-run publish +
  带 ref 的审批请求 + `WAITING_APPROVAL`；被拒（附理由）→ 修订 →
  `plan@v2` → 再审，审批记录精确指向它审的是哪一版。
- **artifact 可作输入**：spawn 参数 / CLI 以 ref 传入，journal 进
  child 的 `RunStarted` 后由 materialize activity 物化进 workspace
  （in-doubt 语义随之而来）。driver 的跨迭代 carry 文档同样存这里。

### MCP

- **server 生命周期是带外运行时状态，不进 event 模型**：resume/重启后
  server 重新拉起；原型假定 MCP server 无状态（per-call stateless），
  这是文档化的契约。实现用官方 MCP Go SDK 管理 client/session。
- **发现的 tool schema 记录为 event**（它们进入 LLM 的 tool 列表，
  是影响 run 结果的外部输入）；`tools/list_changed` 同理。
- 只有 `McpToolCalled/Returned` 是 activity。spec schema 里保留
  `transport: http` + auth 字段，实现（OAuth 流程、凭据存储）推迟。
- **命名空间与类别**：MCP tool 在 permission rules 里只以全限定名
  `mcp__<server>__<tool>` 出现，与内置 tool 不可能撞名（server 上报
  一个叫 `read_file` 的 tool 不会命中内置规则）；动态发现的 tool 没有
  类别标签，一律按最保守的 execute-class 对待（plan 等只读 mode 默认
  排除），除非 spec 显式为其标注类别。

### Provider

- 薄接口（`complete(request) → stream`），streaming 原生。**Gemini 为主
  实现，Anthropic 为次**（同一接口的第二个实现，验证抽象不漏）。
- **能力是通用的、可选的**：请求以 provider 无关的方式携带 `caching`、
  `thinking`、`tools`、`max_tokens` 等意图；每个 provider 把它们映射到
  自家 API（Gemini 的 context caching / thinking config，Anthropic 的
  `cache_control` / extended thinking）。provider 用 `capabilities()`
  声明支持哪些能力，请求了不支持的能力时明确降级或报错，而不是静默忽略。
- **返回归一化**：token 计数（含 cache_read/cache_write）、finish
  reason（含异常形态，见 Agent loop）、tool call、thinking 块统一成一套
  内部表示，L2/L3 及记账不感知具体 provider。
- **opaque signature 随 event 持久化**：归一化的 assistant part 带一个
  per-provider 的 opaque extras/signature 字段（Gemini 的
  `thoughtSignature`、Anthropic 的 thinking signature），context
  assembly 回传时原样携带——丢掉它，Gemini 的多轮工具调用在第二次请求
  就 400。推论：mid-run 切换 provider 不能带着对方的 signature 历史，
  需在 compaction 边界（摘要天然无 signature）重新开始。
- **凭据经 `CredentialProvider` 接口解析**：静态环境变量（如
  `GEMINI_API_KEY`）是其一种实现；OAuth/订阅登录的 refresh token 走
  受管 token store（event log 与 workspace 之外的又一持久位置，0600，
  支持刷新回写）。意图不变：密钥绝不进 spec、event log 或仓库；
  tool 输出可能携带 secret，由 L1 的 redaction 兜底。

### Multi-agent

- 三种模式：**spawn/await**（子 agent 作为 activity，可扇出）、
  **handoff**（移交后退出）、**pub/sub 协作**（blackboard topic）。
- **子 agent 的意义在上下文隔离**：child 烧自己的 window，只有符合
  result contract 的最终报告回流 parent（contract 在子 agent spec 的
  `description`/输出约定里声明）。
- **审批路由**：child 的 `ask` 沿 correlation id 冒泡到 session 的
  frontend——审批的永远是人，不是 parent agent。
- **权限继承拆成两条规则**（mode 没有"交集"运算，不能笼统写 ∩）：
  (1) **rules 做真交集**——spawn 时由 parent 按当时的有效权限计算，
  冻结成不可变数据传给 child；child 的管线只认这份，child spec 无法
  放宽，parent 事后的 mode 跃迁也不回溯影响 child。(2) **mode 不交集**
  ——child 的 mode 独立，但工具面先经冻结 rules 过滤，mode 跃迁只能在
  冻结 rules 内移动；child spec 声明 `bypass` 非法。
- **树级预算与递归上限**：spawn 深度与并发扇出有数据化上限（budget
  关卡校验，超限渲染为 error 结果）——spec 白名单允许 A↔B 成环，
  上限是唯一防线。child 的有效预算 = min(child spec 限额, parent
  剩余额度)，沿 correlation 树聚合，与权限冻结同构；parent 的 token
  上限约束的是整棵树，不是单个 stream。
- 可审计性保证是 per-stream 的：每个 agent 的 stream 完整、
  causation/correlation 链路完整；跨 actor 的消息交错不保证确定性重现
  （见非目标）。

## L4 — Surfaces

### Session 管理

- session = correlation id + 它名下的 stream 闭包（含子 agent）。
- **list**：枚举 store。**resume**：snapshot + fold（见 L1）。
- **fork/rewind 的唯一合法目标是 `CheckpointBarrier` event**：barrier
  达成时（turn 边界 + 全部子 agent 静默——含 bash 进程组确认退出 +
  所有 worktree 快照完成）落一条 event，记录跨 stream 的一致切面：
  {stream → seq} 向量 + snapshot ref 集合。fork = 在新 run id 下复制
  该切面内的 events，以 `ForkedFrom{run, barrier}` 为创世 event
  （原 id 作为 provenance 保留），并从 snapshot 物化**自己的**
  worktree——fork 与原 run 不共享目录；rewind = fork 后用户显式切换
  并放弃原 run。被排除的路径（见 L1）在 fork 里天然缺席。
  任意 seq N 处的 fork 不提供——跨 stream 的因果一致切割不值得做。
  （注：barrier 的"全树静默"要求已确认会挡住长活后台任务场景，将在
  S7 按 dogfood 经验弱化为 consistent-enough cut——届时按不变量变更
  流程修订本节，见 STAGES.md S7。）

### 交互协议

- frontend 是普通 actor：订阅输出 topic，向 run 发输入（journal 后生效）。
- 输出事件流：turn 开始/结束、token delta（ephemeral）、tool call 及其
  permission 判定、`ApprovalRequested`、`TurnDiscarded`、后台任务进度
  topic。CLI 先做 turn 粒度渲染，token streaming 是纯增量，协议不变。
- `ApprovalRequested` 携带 `payload_ref` 时，frontend 渲染对应 artifact
  ——审批对象是一份版本化文档，不只是 tool call 参数。
- 协议预留（原型不实现）：slash command 调用、附件/图片消息类型。

### 运行形态与 background

- core 是库。CLI（`agentrunner run <spec> "task"`）、headless 单发、
  server（HTTP/WS 暴露同一协议）都是薄壳。
- **run 默认由常驻 runtime 托管**（server 壳 + 本地 socket），CLI 是
  attach/detach 的薄客户端：attach = 从 journal 补读到 seq N + 订阅
  live topic（错过的 token delta 按 doctrine 丢失，组装消息不丢）；
  detach **不产生任何事件**——订阅状态不影响 run 结果，无事可记。
  `runtime.daemon: never` 时降级为现有 durable park（下次进程启动时
  resume）。
- **常驻 runtime 也是 durable timer 的触发者**：维护 timer 的派生索引，
  到期 journal `TimerFired` 并发起 resume——timeout/cron/审批过期的
  "等几天成本相同"由它兑现；CLI-only 部署显式降级为"下次 resume 补火"。
- **优雅停机是定义好的**：SIGTERM → 协作取消全部在飞 activity
  （落 `ActivityCancelled`）→ snapshot → 退出——例行 deploy 不产生
  in-doubt。server 形态推荐 **session-per-process** 拓扑（core 是库 +
  文件态持久状态天然支持），单个大 fold 不会饿死其他 session。
- **notifier 是一个 L4 actor**：订阅 run/driver 生命周期 topic（终态、
  `WAITING_APPROVAL`、`IterationCompleted`…），按 user 层配置的通道发
  通知；`NotificationSent` 记在自己的 stream 里跨重启去重（启动时与
  store 对账）。通知通道是 surface 机件——与 hooks 同类的**文档化
  carve-out**，不过四关卡，只能来自 user 层配置。

### Scheduler 与 triggers

- scheduler 是发布 `RunAgent` command 的普通 actor；webhook 触发 =
  server 壳收到请求后发同一条 command。command 幂等（重试不会拉起
  重复 run）。（S6 修订：v0 无独立 scheduler actor——cadence 在
  driver 内、timer 唤醒在 daemon sweep；daemon 线协议的 run/drive
  提交以 `idem_key` 幂等（daemon 生命周期内），重试返回同一 session
  的流。独立 RunAgent command 家族随 webhook/壳 一并落地。）

### 运行模式：IterationDriver（one-shot / goal / loop）

- **goal 和 loop 是同一个 driver actor 的两种 schedule**，one-shot 是
  最平凡的情形。driver 有自己的 stream 和纯 fold 状态，每轮迭代 spawn
  一个 **fresh child run**（同 spec → prefix 逐字节稳定可跨迭代命中
  缓存、免 compaction 链、失败迭代不污染后续、迭代边界天然是 barrier
  候选点）；driver 自己从不碰 LLM 和 workspace——verifier 是这条线的
  **成文例外（S6 裁定、S7 管线化兑现）**：verifier 是 driver 规格里
  "用户可信配置"声明的效果，**作为 journaled、经管线判定的 effect 执行**
  （command = tool_call、llm_judge = llm_call；EffectRequested/Resolved
  + ActivityStarted/Completed 入 driver stream——event log 即 trace）。
  判定的规则层 = user/project 合并规则在前、driver-trust 的兜底 allow
  在后（显式 deny 约束 verifier，未命中即放行——verifier 与 spec
  permissions 同信任级）；ask 收紧为 deny（配置声明的效果无人应答）。
  花费计入迭代 usage、verdict journal 进 IterationCompleted。
- **统一事件族**：`IterationScheduled / Launched / Completed`、
  `DriverCompleted{reason: satisfied|stalled|max_iterations|budget|
  stopped|child_failed}`。launch 遵循 journal-before-send；崩溃后的
  重发幂等由**纯 fold 检查（st.at(n) 已在 journal 则不重发）+ 确定性
  child 目录（sub/iter-N，已终态则从其 fold 结算）**保证（S6 修订：
  等价于原 `Envelope.id = hash(driver_id, n)` 方案，且无需 command
  基础设施）。
- **Goal mode** = `schedule: immediate` + verifiers 必填。verifier 三态：
  `command`（bash-class，exit code / metric regex）、`llm_judge`
  （LLM 打分 + rubric + threshold）、`human`（就是现有 ask 路径，
  挂几天免费）。verdict journal 进 `IterationCompleted`；
  停滞检测是纯 fold——分数 patience 轮无改善（或 binary verifier 的
  失败指纹连续相同）→ stalled，附最佳迭代的 carry。
- **Best-of-N** = `schedule: parallel{n}`：N 个隔离 worktree 的并行
  尝试，选择即 human / llm_judge verifier，胜者晋升（fork 或 apply diff）。
- **Loop mode** = `schedule: interval|cron|self_paced` + verifiers
  选填。self_paced 靠两个数据定义的内置 tool：`schedule_next{after}`
  （过管线 → scheduler journal durable timer，min/max 钳位 +
  `on_no_intent` 兜底）与 `finish_series`（"自称完成"由 human
  verifier 把关，不另设 confirm 机制）。`overlap: skip|coalesce|
  interrupt`；跳过是 `IterationSkipped` event，不是沉默。
- **预算与失败策略共享**：driver 是树预算的根，reserve-at-launch /
  settle-at-completion；`on_reserve_failure: skip|stop`、
  `on_child_failure: stop|surface|retry{max, backoff}`——对**终态**
  失败 run 的策略性重试不是第二套恢复机制（恢复只关乎崩溃的 run
  找回自身状态，原则 6 不禁止 policy 级重试）。
- **跨迭代数据两条通道**：carry 文档（child report / verifier 输出
  摘要）存 `ArtifactStore`，`IterationCompleted` 只带 ref + 短摘录；
  series memory 是 workspace 里 agent 自管的文档，注入为 context
  assembly 的一层——**权威边界在注入时截断**（tool-gate 拒绝只是
  引导，bash 旁路条款同样适用）。`barrier_per_iteration` 可选；
  snapshot backend 为 `none` 时 `barrier_ref` 缺席，stall 呈现降级为
  carry + verdicts（无 fork 按钮）。
- **driver 依赖常驻 runtime**：没有它，interval/cron 只在进程活着时
  触发、human verifier 的审批无人接收——这是文档化的降级模式，
  不是默认。

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
  in-doubt activity、审批挂起恢复、interrupt、异常终止形态
  （空 candidate / malformed function call）各有专门的崩溃注入测试。

## 已定决策

| # | 决策点 | 选择 | 理由 |
|---|--------|------|------|
| 1 | 语言 | Go 1.23+ | goroutine/channel 与 actor/mailbox 天然同构；单静态 binary 跨平台分发；Gemini/Anthropic/MCP 官方 Go SDK 齐备；编译期检查利于迭代。（原选 Python 的论据是 SDK 成熟度，现已不构成差异。） |
| 2 | 进程模型 | 单进程，in-memory bus | 原型简单；边界清晰，分布式化是换 transport。 |
| 3 | 持久状态 | event log + workspace + 接口后的 ref-addressed blob store（SnapshotStore/ArtifactStore/任务日志共用 CAS 模块）；bus/delta ephemeral | fold 永不读 store；event 只引用 ref，blob 先于引用它的 event 落盘；"什么会丢"一目了然。 |
| 4 | 输入语义 | 一切外部输入先 journal 成 event 再消费 | 审批/steering 不丢、可审计；bus 才允许 ephemeral。 |
| 5 | Durability 模型 | journal + turn 边界 snapshot-resume + 显式等待状态；**不做** Temporal 式 code replay | 同样的用户可见能力（crash 恢复、长审批、fork），~10% 成本；loop 不背确定性纪律。 |
| 6 | Activity 语义 | Started/Completed 双落盘，at-least-once；in-doubt 按 tool 类别数据化处置（LLM 重发+TurnDiscarded、read/idempotent 重跑、execute/edit 渲染 interrupted 继续、高危显式转人工），协作取消，通用 retry，background 变体 | 崩溃必然砸中 in-flight；headless 不能靠人工 triage；非幂等者不重跑（而非转人工）。 |
| 7 | Checkpoint 语义 | 对话 snapshot 是可弃缓存；workspace 快照是一等状态，走 `SnapshotStore` 接口（event 只引用 opaque ref），默认 shadow-repo backend，只服务 rewind/fork | 文件系统不可从 log 重建；不与 git 耦合；用户 repo 与 agent 的 git 操作零污染。 |
| 8 | 副作用治理 | 单一 effect pipeline，四关卡；判定按持久化时点拆分——`EffectResolved` 落在 `ActivityStarted` 前，post-hook 随 `ActivityCompleted` | permission/审批/hooks/预算是一个机制；恢复不重放 hook 副作用；happy path 仍是单条关卡 event。 |
| 9 | 失败面向模型 | 每个 tool call 必有配对结果（harness call id，assembly 按原顺序重排）；error 渲染 per-provider 定义；超预算优雅收尾 | Gemini 按数量+位置严格配对且无 error 标志；agent 要能对失败自适应。 |
| 10 | Permission modes | mode = 工具面过滤（作用于 permitted 面；advertised 面 prefix 内稳定）+ prompt 注入 + 跃迁规则（数据） | plan/acceptEdits 是 loop 行为；mode 切换不得打爆 tools 级缓存。 |
| 11 | Hooks | v0 只 observe + block；是管线机件不是 effect | 改写带来顺序/缓存/重放问题，推迟；避免管线递归。 |
| 12 | 存储后端 | JSONL per stream，藏在 `EventStore` 接口后 | 可读可 diff；需要时换 SQLite。 |
| 13 | Spec 格式 | YAML → 强类型 struct + 校验；tool 定义也是数据 | 声明式、可 review；原则 4 落到 tool 层。 |
| 14 | 运行形态 | core 是库；CLI/headless/server 是薄壳 | 一套 core 支撑所有 surface。 |
| 15 | Provider | 薄接口 + 多 provider（Gemini 主、Anthropic 次），streaming 原生 | 两个实现验证抽象不漏；caching 是经济性前提。 |
| 15b | 能力抽象 | caching/thinking 等为 provider 无关的可选 capability，各 provider 映射到自家 API，请求归一化 | 上层不写死某家语义；不支持的能力显式降级/报错而非静默。 |
| 15c | 凭据 | `CredentialProvider` 接口（静态 env / 受管 token store 皆为实现）；harness 自身绝不写入 spec/event/仓库；落盘前 redaction；log 0600 | OAuth refresh token 需持久化+回写，"只读 env"表达不了；密钥不进受控内容的意图不变。 |
| 16 | Skill 格式 | 沿用 Claude Code 约定 | 生态兼容，不发明格式。 |
| 17 | MCP 生命周期 | 带外运行时状态；只有 tool 调用是 activity；发现的 schema 记录为 event | server 状态不可 event 化；schema 是影响结果的输入。 |
| 18 | Event schema 版本化 | 不 migration；`RunStarted` 记 event-schema 版本，不匹配拒绝 resume；所有 fold 消费者走 `EventStore` 单一读路径，预留恒等 upcast 阶段 | 原型 re-run 比 migrate 便宜；将来要 migration 时只有一个改动点。 |
| 19 | 信任模型 | 可执行配置（hooks）只认 spec 与 user 层；project 层需显式 trust；memory 文件按不可信内容对待 | clone 不受信 repo 不等于交出任意代码执行权。 |
| 20 | 树级约束 | 权限 rules 在 spawn 时冻结交集下传；预算 = min(child 限额, parent 剩余) 沿树聚合；深度/扇出有上限 | spawn 白名单可成环；树的总成本必须有界。 |
| 21 | 运行模式 | one-shot/goal/loop/best-of-N（`parallel{n}`）是同一 `IterationDriver` 的四种 schedule；每轮迭代 = fresh child run | 避免多套近似驱动机制；fresh run 保 prefix 稳定与故障隔离。 |
| 22 | Background | run 由常驻 runtime 托管，frontend 任意 attach/detach（detach 无事件）；后台 effect 的 handle 即其配对结果，完成是新的 user-role 输入 | 订阅状态不影响 run 结果；已配对的 call 不可二次触碰（Gemini 严格配对）。 |
| 23 | Artifacts | `ArtifactStore`（CAS，opaque ref）；publish 是过管线的 tool，发布即持久；`outputs:` 在 run 收尾自动 publish；审批载荷 = artifact ref；版本 per-stream | 交付物 contract 与过程协调对象分离；审批需要不可变锚点。 |
| 24 | Run 收尾 | 固定 epilogue：quiesce 后台任务 → auto-publish outputs → 终态 barrier → 终态 event | 多个 feature 都往 run 末尾加步骤，顺序必须唯一定义。 |

## Roadmap

以 walking skeleton 优先——让真实的 agent 负载塑造下层 API，
而不是给玩具 actor 造两个 milestone 的框架：

1. **M1 — Walking skeleton**：最小 spec loader、Gemini provider（凭
   `GEMINI_API_KEY` 从环境读）、朴素 agent loop、2-3 个内置 tool
   （read_file/edit_file/bash）、append-only JSONL event journal
   （先只记录，不做 source of truth）、最小 CLI。端到端跑通一个真 agent。
   provider 接口从第一天按多实现设计，Anthropic 作为第二实现在 M3
   context assembly（caching）阶段补上以验证能力抽象不漏。
2. **M2 — Durability + pipeline**：event fold 成为 state 的正源、
   turn 边界 snapshot、resume、显式等待状态；effect pipeline 四关卡 +
   拆分落盘的 `EffectResolved`；permission rules（含 bash 命令模式与
   realpath 匹配）+ 审批流；budgets（reserve-then-settle）；
   hooks（observe/block）；凭据 redaction；`SnapshotStore`
   （shadow-repo backend，用真实大仓库的延迟基准定快照粒度）。
   崩溃注入测试。
3. **M3 — 交互与上下文**：streaming 协议、steering/interrupt/协作取消、
   并行 tool call、context assembly（拼装、截断、compaction、caching）、
   session list/resume、后台 effect（handle 配对 + 完成输入 +
   `WAITING_TASKS`）、run 收尾 epilogue。
4. **M4 — 生态接入**：MCP（生命周期 + schema 记录）、skills、
   memory 文件、配置分层；`ArtifactStore`（`publish_artifact` +
   `outputs:` contract + 审批载荷）。
5. **M5 — Multi-agent + surfaces 收尾**：spawn/await、handoff、
   pub/sub；审批路由、权限冻结与树级预算；scheduler；
   fork/rewind（`CheckpointBarrier` 语义）；`inspect` 时间线；
   server 壳（常驻 runtime + attach/detach）+ notifier；
   `IterationDriver`（goal / loop）。
