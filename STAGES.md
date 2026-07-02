# AgentRunner — 分阶段实施设计（Stages）

本文档把 `DESIGN.md` 的架构拆成 **7 个从简单到复杂的实施阶段**。两个牵引目标：

1. **教育意义**：每个阶段都是一个能跑、能完全读懂、能独立消化的完整系统；
   每次迭代增加的复杂度有界。
2. **最终收敛**：七段走完，得到能支撑 General Coding Agent 大部分能力的
   完整设计（即 `DESIGN.md` 的全貌，含延迟批次）。

`DESIGN.md` 仍是架构的 source of truth——本文档只回答"按什么顺序、
切成什么块、每块的完成标志是什么"。两者在实施顺序上以本文档为准
（`DESIGN.md` 的旧 Roadmap 章节将来会收敛到这里，暂不修改）。

## 总览

| Stage | 名字 | 一句话 | 主要对应 DESIGN.md |
|-------|------|--------|--------------------|
| 1 | 会干活的 agent | walking skeleton：最小可用 coding agent | L3 loop/provider/tools 最小版 |
| 2 | 一切皆事实 | event-sourced 内核与 durability | L0 + L1 |
| 3 | 治理副作用 | effect pipeline 四关卡 | L2 |
| 4 | 交互与上下文 | 单 agent 完全体 | L3 context assembly + L4 交互协议 |
| 5 | 生态与多 agent | MCP/skills/子 agent/artifacts | L3 其余 |
| 6 | 服务化与运行模式 | 常驻 runtime/background/driver | L4 |
| 7 | 世界状态生命周期 | snapshot/fork/rewind/云/沙箱/索引（延迟批次） | L1 workspace + L4 session 深水区 |

**难度曲线是刻意设计的**：S1–S2 教内核思想，S3–S4 教生产现实，
S5–S6 教组合能力，S7 收官最难的教义问题。

## 贯穿 S1–S6 的硬约束（"四个钩子"）

S7 被有意延迟。为了让它届时是**加装**而非**返工**，前六段必须始终满足：

1. **所有文件类 tool 走 workspace 抽象**，不允许任何代码绕过它直接摸
   文件系统。（本来就是 permission/realpath 检查的要求，零额外成本。）
2. **run 收尾 epilogue 保留 barrier 槽位**，实现为显式 no-op：
   quiesce → auto-publish → **[barrier: no-op]** → 终态 event。
3. **活动静默记账不延迟**：live task 集合、进程组终态确认属于 activity
   语义（取消/interrupt 的正确性依赖它），S2 就位。S7 的 barrier 届时
   直接消费这份 fold state。
4. **event log 纪律不松动**：fold 纯度、per-stream seq、
   journal-inputs-first。这是内核本身。

另一条纪律：**每个阶段结束做一次对抗式 review**（与 DESIGN.md 定稿时
相同的多视角审查），审完才进下一段。

完成标志一律实现为**可执行的 acceptance 场景**（定义见 PLAN.md 0.6），
用 `agentrunner accept --stage N` 验收——TTY 下为 TUI 清单，非 TTY
纯文本 + JSON 报告。

---

## Stage 1 — 会干活的 agent（walking skeleton）

**目标**：最短路径打通 spec → LLM → tool → 结果，让真实负载从第一天
就开始塑造接口形状。

**范围**
- 最小 spec loader：YAML → 强类型 struct + 校验（name / model /
  system_prompt / tools 四个字段就够）。
- 薄 provider 接口 + Gemini 实现（`complete(request) → stream`，
  凭 `GEMINI_API_KEY` 从环境读；接口从第一天按多实现设计，但只做一个）。
- 朴素 agent loop：LLM turn → tool 执行 → 循环，turn 粒度输出。
- 三个内置 tool：`read_file` / `edit_file` / `bash`（tool 定义即数据；
  已经走一个最小 workspace 抽象——钩子 1 从这里开始）。
- append-only JSONL journal：**只记录，不作为 source of truth**
  （为 S2 铺路，先积累"哪些东西值得成为 event"的直觉）。
- 最小 CLI：`agentrunner run <spec> "task"`。

**教学重点**：agent loop 的完整解剖。一个人读完全部代码（一两千行量级），
能自己写出 mini coding agent。这一段没有任何"框架"，只有问题本身。

**完成标志**：agent 能在真实仓库里完成"读代码 → 改文件 → 跑测试"的
端到端任务；journal 里能看到全过程。

---

## Stage 2 — 一切皆事实（event-sourced 内核）

**目标**：把 S1 的裸 loop 放到 durable 内核上：进程死掉，run 不死。

**范围**
- L0 正式化：actor / mailbox（channel）/ bus（send + publish）/
  Envelope（id、causation、correlation）；command 按 `Envelope.id`
  幂等去重；actor 崩溃 → `ActorCrashed` event，无自动重启
  （恢复只住在 session resume 一个地方）。
- command/event 严格分离；**所有外部输入先 journal 成 event 再消费**。
- state = event log 的纯 fold；journal 从"只记录"升级为 source of truth。
- turn 边界 snapshot + resume（snapshot 是可弃缓存，fold 可全量重建）。
- 显式等待状态注册表：四个变体与完整可中断性表一次画全
  （`WAITING_INPUT/APPROVAL/TASKS/TIMER`，TASKS/TIMER 标注 S6 前不可产生）。
- run 收尾 epilogue 骨架（全 no-op：quiesce → publish → [barrier] →
  终态 event；**钩子 2 在此落位**，后续阶段填实槽位）。
- CLI `resume` / `sessions list` 收口。
- activity 语义全套：`ActivityStarted/Completed/Failed/Cancelled` 双落盘、
  at-least-once + in-doubt 上浮（绝不静默重跑；`idempotent: true` 是唯一
  例外通道）、通用 retry/backoff、**进程组级协作取消**（钩子 3）、
  durable timer、凭据 redaction、log 权限 0600。
- `RunStarted` 记录代码/spec 版本，不匹配拒绝 resume。

**教学重点**：不用 Temporal 也能拿到 durability——journal + fold +
snapshot 三件套；为什么"输入先落盘"消灭了一整类竞态；为什么恢复
只能住在一个地方。

**完成标志**：崩溃注入测试全绿——任意时刻 `kill -9`，resume 后对话
状态分毫不差、in-doubt activity 正确上浮、等待状态跨进程存活
（用合成 event 验证；"审批全流程挂起存活"是 S3 审批流步骤的验证项）。

---

## Stage 3 — 治理副作用（effect pipeline）

**目标**：所有副作用流经唯一管线；permission、审批、hooks、预算
作为四个关卡（而非四个子系统）落地。

**范围**
- 四关卡管线：hooks(pre) → permission → budget → execute → hooks(post)。
- `EffectResolved` 按持久化时点拆分落盘（判定终结后、执行开始前；
  ask 路径在审批应答后；post-hook 结果随 `ActivityCompleted`）；
  进关卡未出 `EffectResolved` 的 effect 享受 in-doubt 待遇。
- permission rules 作为数据：allow/ask/deny、path 规则（realpath 强制、
  workspace 边界检查）、bash 命令模式匹配、"path 规则约束不了 bash"
  的诚实条款。
- permission modes 作为 loop 行为：工具面过滤 + prompt 注入 + 跃迁规则
  （`plan` / `acceptEdits` / `bypass`；tool 类别标签 read/edit/execute）。
- 审批：ask → `ApprovalRequested` → `WAITING_APPROVAL`（durable）→
  应答 journal 后继续；denied-by-interrupt 路径。
- budget：reserve-then-settle（预留集入 fold state）；run 级资源限额 →
  优雅收尾；结构性限制 → error 结果。
- hooks v0：observe + block；是管线机件不是 effect。
- 配置分层（spec + user + project 三源合并，从 S5 提前——信任模型
  依赖它）+ 信任模型：可执行配置只认 spec 与 user 层，project 层需
  显式 trust；memory 文件按不可信内容对待。
- 每种关卡结果的模型可见渲染（error tool_result 家族）。

**教学重点**："policy 是数据"的完整示范；一条管线如何同时长出四个
产品级 feature；"给模型的错误"与"给用户的错误"是两个 surface。

**完成标志**：`plan` mode 全流程可用；审批挂两天后批准，run 原地继续；
预算 gate 级合成并发不超支（真实并行在 S4 复验）；不受信 repo 的
hooks 不执行。

---

## Stage 4 — 交互与上下文（单 agent 完全体）

**目标**：把 loop 打磨到生产现实：streaming、打断、并行、缓存经济学。

**范围**
- 交互协议：输出事件流（turn 边界、token delta ephemeral、
  `TurnDiscarded`）、steering 消息 turn 边界消费、interrupt 走
  activity 协作取消（`[interrupted by user]` 渲染）。
- 并行 tool call：harness call id（随 event 持久化）、per-call 过管线、
  ask 不阻塞已放行、assembly 收齐后按原 call 顺序重排
  （Gemini 1:1 严格配对）。
- context assembly 一等组件：拼装顺序（基础指令 → **冻结的**环境块 →
  memory 层 → tool/skill/子 agent 目录 → spec prompt）、prefix 稳定
  不变量与 cache 断点、tool 输出截断、compaction 作为 recorded activity
  （`ContextCompacted` 改变 fold）、opaque signature 透传
  （thoughtSignature）。
- 异常 finish reason 的 loop 策略（malformed_tool_call 重试、
  safety/blocked 上浮）。
- session UX 打磨（`show`、resume 的流式续接；`resume`/`sessions list`
  已在 S2 收口）。
- `inspect` v0：timeline、每个 call 的判定、token/cost/cache 列。
- Anthropic 作为第二 provider 实现落地，验证能力抽象不漏。

**教学重点**：生产级 loop 的全部脏现实——缓存的经济学（约 10x）、
主 provider 的严格配对与终止形态、被打断的流如何诚实呈现。

**完成标志**：体验接近"单 agent 版 Claude Code"的 CLI；缓存命中率
可在 `inspect` 中验证；Esc 能在 500ms 内杀掉任何 tool call；
双 provider scripted 矩阵全绿。

---

## Stage 5 — 生态与多 agent

**目标**：接入外部生态；multi-agent 作为"actor 发消息"落地；
交付物成为一等公民。

**范围**
- MCP：官方 SDK 管理生命周期（带外运行时状态）、发现的 schema 入
  event、`mcp__<server>__<tool>` 全限定名、无标签 tool 按 execute-class。
- skills（Claude Code 约定）、memory 文件层级合并（配置三源合并
  已提前至 S3）。
- 子 agent：spawn/await、handoff、pub/sub 三模式；权限 rules spawn 时
  冻结交集下传；mode 不交集；树级预算 min 聚合；深度/扇出上限；
  审批沿 correlation 冒泡到 frontend。
- **Artifacts**：`ArtifactStore`（CAS、opaque ref、blob 先于 event）、
  `publish_artifact` 过管线发布即持久、版本 per-stream、`outputs:`
  交付物 contract（收尾 epilogue 自动 publish）、
  `ApprovalRequested{payload_ref}`（plan 审批流）、artifact 作输入
  （materialize activity）。
- run 收尾 epilogue 的 **auto-publish 槽位填实**（骨架与钩子 2 已在
  S2 落位；quiesce 在 S6 填实，barrier 在 S7）。
- `inspect` 扩展：子 agent 树（correlation/causation 渲染）。

**教学重点**：multi-agent 不是子系统；contract（交付物）与协调对象
（plan）分离；"SnapshotStore 模式"如何复用为 ArtifactStore。

**完成标志**：一个 researcher 编队（parent + 2 子 agent）产出带
contract 检查的报告 artifact；plan 审批（发布 → 审 → 拒 → v2 → 批）
全流程走通；子 agent 无法越权、树预算不可击穿。

---

## Stage 6 — 服务化与运行模式

**目标**：从"一次 CLI 调用"变成"常驻服务上的运行形态家族"。

**范围**
- 常驻 runtime（server 壳 + 本地 socket）；CLI 变 attach/detach 薄客户端
  （attach = journal 补读 + 订阅；detach 无事件）；
  `runtime.daemon: never` 降级模式。
- notifier actor：订阅 run/driver 生命周期 topic、`NotificationSent`
  去重 stream、通道为文档化 carve-out（只认 user 层配置）。
- 后台 effect：`background: true`、handle = `ActivityStarted` 的 fold
  渲染（配对当场满足）、完成 = user-role 输入、`WAITING_TASKS`、
  `task_output` / `task_kill`、`on_run_end: cancel|await`。
- scheduler：cron/interval/webhook → 幂等 `RunAgent`。
- **IterationDriver**：统一事件族；goal mode（verifiers：command /
  llm_judge / human；停滞检测）；loop mode（self_paced 的
  `schedule_next` / `finish_series`；overlap 策略）；best-of-N
  （`schedule: parallel{n}`，**阶段内可延后项**，见 PLAN cut line）；
  跨迭代 carry 走 ArtifactStore、series memory 注入时截断；
  `on_child_failure` / `on_reserve_failure`。
- headless / server 壳补全（HTTP/WS 同一协议；**阶段内可延后项**，
  见 PLAN cut line——均不影响完成标志）。

**教学重点**：frontend 只是订阅者，"后台"不是执行模式而是 attach 问题；
one-shot / goal / loop / best-of-N 是同一个 driver 的四种 schedule。

**完成标志**：一个 series 在无人 attach 的情况下按 cron 跑过夜、
产出通知；一个 goal run 迭代三轮到 verifier 通过；CLI 关掉重开
attach 回同一个 run。

---

## Stage 7 — 世界状态生命周期（延迟批次）

**目标**：补上被有意延迟的最难教义：世界状态的快照、回退、迁移与边界。
此时手上有 S1–S6 的 dogfood 经验——尤其是真实的后台任务使用模式——
争议最大的语义（barrier 静默）在信息最多的时候设计。

**范围**（内部再分小步，顺序可调）
- `SnapshotStore` + shadow repo backend（独立 GIT_DIR，对用户 repo
  与 agent 的 git 操作隐形；排除策略；延迟基准）。
- `CheckpointBarrier`：**弱化版语义**（consistent-enough cut：barrier
  向量记录 in-flight 后台任务及其处置策略，不再要求全树静默）；
  epilogue 的 no-op 槽位填实。
- fork / rewind：事件切面与快照物化拆成两个正交轴（支持对话/代码
  独立回退）；`ForkedFrom` 创世与 id remapping。
- 云端 workspace 生命周期：provision → live → teardown；resume 可从
  外部源重建 workspace；store 外置；环境预备 prologue（setup 脚本
  走信任模型）。
- OS 沙箱 backend + 网络出口策略（rules 加 network 资源类；
  `EffectResolved` 记录生效的 containment）。※ 若安全优先级提前，
  此项可单独抽出提前到 S3 之后——它是 additive，不依赖本段其他内容。
- `IndexStore`（第四类状态：可从 workspace 重建的派生索引）+
  常驻 indexer actor + `semantic_search` tool。
- （可选，IDE 方向确定后）advisory-inference 旁路、IDE buffer overlay、
  shadow workspace 的 `promote_worktree_diff`、per-hunk 部分接受。

**教学重点**：为什么世界状态是 harness 里最难的部分；三次对抗审查中
争议最大的条目全部住在这里——以及延迟决策如何让它们变简单。

**完成标志**：rewind 到任意 barrier 且 workspace 与对话一致；fork 出的
分支在独立 worktree 上继续；（若做云）容器销毁后 resume 重建 workspace
继续跑。

---

## 与 DESIGN.md 的差异声明

- `DESIGN.md` 的 M1–M5 roadmap 由本文档取代（暂不修改原文，收敛时再改）。
- `DESIGN.md` 中以下内容在 S1–S6 期间处于"设计已定、实现延迟"状态：
  SnapshotStore 的非 `none` backend、`CheckpointBarrier`、fork/rewind、
  `barrier_ref` 相关字段。目标中的"可 fork"顺延至 S7。
- 四个钩子（上文）在 S1–S6 是硬性验收项，每阶段 review 检查。
