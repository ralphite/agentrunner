# AgentRunner — 实施计划（Implementation Plan）

本文档是 `STAGES.md` 七阶段的 **step-by-step 实施计划**：每一步做什么、
产出什么、怎么验证、以什么顺序。原则：**计划先行、review 定稿、然后开工**
——代码写完再调结构比在计划阶段理清贵得多。

粒度约定：**S1–S3 细到单步**（每步一个可验证交付物），**S4–S6 细到
模块序列**，**S7 保持里程碑级**（延迟批次，届时带着 dogfood 经验再细化）。
这是有意的：越远的步骤信息越少，过度细化远期步骤是伪精确。

执行方式：每个 stage 用 loop 式迭代实现（实现一步 → 测试 → 小结 →
下一步）；stage 结束跑对抗式 review（多视角 agent 审查），审过才进
下一个 stage；进入 stage N 前，先用 stage N-1 的经验把 N 的步骤
再细化一轮（**kickoff refinement**，只许细化步骤，不许动 DESIGN.md
不变量——要动不变量必须显式提出、单独 review）。

---

## 0. 技术栈与工程基座（S1 第 0 步，此后不变）

| 项 | 选择 | 说明 |
|----|------|------|
| 语言 | Python 3.12+ | asyncio 全程 |
| 包管理 | uv | `pyproject.toml`，锁定依赖 |
| 校验 | pydantic v2 | spec、event、config 全部强类型 |
| 测试 | pytest + pytest-asyncio | 单测/集成/崩溃注入三层 |
| 静态检查 | ruff（lint+format）+ mypy（core 目录 strict） | 提交前本地跑 |
| LLM SDK | `google-genai`（S1）、`anthropic`（S4）、`mcp`（S5） | 官方 SDK |
| 入口 | `agentrunner` console script | `uv run agentrunner …` |

仓库最终布局（逐阶段长出来，此处为全貌）：

```
agentrunner/
  kernel/        # S2: actor, mailbox, bus, envelope
  events.py      # event/command 类型注册（逐阶段增长，单一出处）
  store/         # S2: EventStore(JSONL); S5: ArtifactStore(CAS); S7: SnapshotStore
  state/         # S2: fold/apply、state snapshot、resume
  pipeline/      # S3: 四关卡、EffectResolved、permission、budget、hooks
  agent/         # spec、loop、context assembly、multi-agent
  providers/     # base + gemini(S1) + anthropic(S4)
  workspace/     # workspace 抽象、路径边界（S1 起）
  tools/         # 内置 tool 定义（数据）+ 实现
  runtime/       # 装配、session；S6: daemon、notifier、scheduler、driver
  cli/
tests/
  unit/  integration/  crash/   # crash/ 为崩溃注入 harness（S2 起）
  fixtures/                     # 录制的 provider 应答、样例 repo
specs/                          # 示例 agent spec
```

跨阶段测试基座（S1 就建）：
- **ScriptedProvider**：从 fixture 回放 provider 应答的假 provider——
  即设计中"activity 结果缓存式 replay 测试"的最小形态。所有集成测试
  默认用它；真 Gemini 测试放 `@pytest.mark.live`，本地凭 env 变量跑。
- **样例 repo fixture**：一个小 Python 工程（含可跑的失败测试），
  作为 agent 的操作对象。

---

## Stage 1 — 会干活的 agent（walking skeleton）

目标回顾：spec → LLM → tool → 结果端到端；几百行、全部可读懂。
**S1 刻意不做**：actor、event 作为 source of truth、管线、审批、并行
tool call、streaming 渲染（turn 粒度输出即可）。

| # | 步骤 | 交付物 | 验证 |
|---|------|--------|------|
| 1.0 | 工程基座 | pyproject/uv/ruff/mypy/pytest 就绪，`agentrunner --version` | CI 式本地脚本 `scripts/check.sh` 全绿 |
| 1.1 | 最小 spec | `agent/spec.py`：`AgentSpec{name, model{provider,id,max_tokens}, system_prompt|_file, tools[]}` + loader | 坏 spec 黄金错误测试（缺字段/未知 tool/文件不存在） |
| 1.2 | provider 类型 | `providers/base.py`：归一化 `Message/Part(text|tool_call|tool_result)/ToolDef/CompleteRequest/TurnResult`；**tool_call 自带 harness call_id**（Gemini 配对从第一天按 id 设计） | 类型单测 |
| 1.3 | Gemini provider | `providers/gemini.py`：env 读 key、请求映射、流式收集为整 turn、functionCall↔call_id 映射、usage 提取 | ScriptedProvider 同接口；live 冒烟测试 |
| 1.4 | workspace 抽象 | `workspace/`：root、`resolve(path)`（realpath + `..` 归一 + 边界检查，越界即拒） | 单测：symlink/`..` 逃逸全拒（钩子 1 从这里生效） |
| 1.5 | tool 定义即数据 | `tools/defs.py`：`ToolDef{name, desc, json_schema, klass: read|edit|execute}`（内置定义以数据声明） | schema 能渲染进 provider 请求 |
| 1.6 | 三个 tool 实现 | read_file（截断上限）、edit_file（old/new 精确替换）、bash（subprocess、`start_new_session=True`、超时 kill 进程组、输出截断） | 各自单测；bash 超时杀干净子进程 |
| 1.7 | journal v0 | `store/journal.py`：append-only JSONL，记录 run 元信息、每 turn 的请求摘要/assistant 消息/tool 调用与结果 | 文件逐行可解析；**只记录，不读回** |
| 1.8 | agent loop | `agent/loop.py`：turn 循环——LLM → N 个 tool_call 顺序执行 → 按原顺序回填 tool_result → 继续；max_turns 上限 | ScriptedProvider 集成测试：多 turn 修文件场景 |
| 1.9 | CLI | `cli/`：`agentrunner run <spec> "task"`，turn 粒度打印 | 手动验收 |
| 1.10 | E2E | 样例 repo：让 agent 修一个失败测试 | scripted 版入 CI 层；live 版手动跑通 |

**S1 完成标志**（= STAGES.md）：真实仓库端到端 + journal 可读。
**预期返工声明**：S2 会把 loop 的内部重写到 activity 之上——1.2–1.6 的
接口（provider/tool/workspace）保持不变，loop 的编排代码允许重写。
这是计划内的，不是事故。

---

## Stage 2 — 一切皆事实（event-sourced 内核）

进入条件：S1 review 通过。核心动作：**journal 从记录升级为 source of
truth，loop 重写到 activity 之上**。

| # | 步骤 | 交付物 | 验证 |
|---|------|--------|------|
| 2.1 | event/command 类型 | `events.py`：Envelope、Command/Event 基类、`RunStarted{versions}`、`InputReceived`、`AssistantMessage`、`ActivityStarted/Completed/Failed/Cancelled`、`WaitingStateEntered/Exited`、`ActorCrashed`… 全部 pydantic | 序列化 round-trip 单测 |
| 2.2 | EventStore | `store/event_store.py`：接口 + JSONL backend；per-stream、seq 单调、append 原子（写临时行+fsync）、读迭代 | 单测：并发 append、崩溃截断行的容错（尾部半行丢弃并告警） |
| 2.3 | kernel | `kernel/`：Actor(task+Queue)、bus（send/publish）、Envelope 投递、command 按 `Envelope.id` 幂等去重（已处理 id 入自身 stream）、未捕获异常 → `ActorCrashed` | 单测：重复 command 只处理一次；崩溃不重启 |
| 2.4 | fold/state | `state/fold.py`：`state = fold(apply, events)`，apply 纯函数；agent 对话 state 的 fold 实现 | 性质测试：任意事件序列 fold 两遍结果相同；fold(全量) == fold(snapshot+尾部) |
| 2.5 | journal-inputs-first | 用户输入/审批应答/timer 到期一律先 append `InputReceived` 类 event 再消费 | 集成测试：输入在崩溃后仍在 |
| 2.6 | activity 执行器 | `state/activity.py`：Started 先落盘 → 执行 → 终态落盘；通用 retry/backoff；`idempotent` 标志；**凭据 redaction**（落盘前替换已知 env 值）；LLM 调用与 tool 执行全部改走它 | 单测：redaction；retry 次数；幂等标志行为 |
| 2.7 | 进程组取消 | bash activity：cancel signal → 组 SIGTERM→宽限→SIGKILL → 确认组退出 → 才落 `ActivityCancelled{partial_output}`（有界 drain） | 测试：孤儿进程不存活（`ps` 断言）；钩子 3 落位 |
| 2.8 | durable timer | timer = `TimerSet` event + runtime 侧调度，到期 append `TimerFired` 再消费；timeout 用它实现 | 崩溃后 timer 仍到期 |
| 2.9 | snapshot-resume | turn 边界序列化 fold state（JSON）；`resume(session)` = 最新 snapshot + fold 尾部 + 继续 loop；版本不匹配拒绝并报错 | fold 全量 == snapshot+尾部 等价测试 |
| 2.10 | 等待状态注册表 | `WAITING_INPUT/WAITING_APPROVAL` 变体 + 可中断性表（表驱动）；interrupt journal 后带出等待 | 单测覆盖表中每格 |
| 2.11 | in-doubt | resume 时 Started-无终态 → 标记 in-doubt 上浮（除 `idempotent: true` 自动重跑） | 崩溃注入：执行中 kill，resume 上浮不重跑 |
| 2.12 | 崩溃注入 harness | `tests/crash/`：子进程跑 runner，按注入点（env 变量指定"append 第 N 条 event 后 abort"）kill，再 resume 断言 | S2 完成标志的载体 |
| 2.13 | loop 重写收口 | S1 loop 的编排全部改为 activity + fold state；CLI 加 `agentrunner resume <session>`、`sessions list` | S1 的 E2E 场景在新内核上重跑通过 |

**S2 完成标志**：崩溃注入矩阵全绿——每个注入点 kill -9 后 resume，
对话状态分毫不差、in-doubt 正确上浮、等待跨进程存活。

---

## Stage 3 — 治理副作用（effect pipeline）

| # | 步骤 | 交付物 | 验证 |
|---|------|--------|------|
| 3.1 | 管线框架 | `pipeline/`：effect 描述对象、四关卡接口、`EffectResolved`（判定终结后、执行前落盘；拦下也落盘）；post-hook 结果挂 `ActivityCompleted` | 单测：每条路径的落盘时点 |
| 3.2 | in-doubt 扩展 | 进关卡无 `EffectResolved` → in-doubt（复用 2.11 机制） | 崩溃注入点扩展到关卡间 |
| 3.3 | permission rules | 规则引擎：tool/path（基于 workspace.resolve）/bash command 模式；allow/ask/deny；规则序 user > project > spec | 表驱动单测；`src/../../etc` 拒绝案例 |
| 3.4 | 配置分层 + 信任 | spec + user + project 三源合并（标量覆盖、rules 拼接）；**project 层 hooks 默认忽略**，`agentrunner trust <dir>` 显式信任 | 不受信 repo 的 hook 不执行（测试） |
| 3.5 | 审批流 | ask → `ApprovalRequested`（预留 `payload_ref` 字段）→ `WAITING_APPROVAL` → 应答 journal → 继续/拒绝渲染 | 崩溃注入：挂起中 kill，resume 后批准继续 |
| 3.6 | modes | mode = 数据（工具面过滤 + prompt 注入 + 跃迁规则）；`default/plan/acceptEdits/bypass`；`ExitPlanMode` 工具 + 跃迁 event；bypass 不跳 hooks | plan 全流程集成测试 |
| 3.7 | budget | reserve-then-settle（预留集入 fold state）；LLM 按 max_tokens 预留、tool 按类别估值；资源超限 → 收尾消息 + `LimitExceeded`；结构限制 → error 结果 | 并发 TOCTOU 测试：N 并发不超支 |
| 3.8 | hooks v0 | pre/post 执行器（observe + block by exit code）；结果入 `EffectResolved`/`ActivityCompleted` | 恢复路径不重跑 hook（崩溃注入） |
| 3.9 | 错误渲染表 | deny/hook block/审批拒/activity 失败/超预算 → 模型可见渲染的统一函数 | 每行一测；loop 继续性断言 |
| 3.10 | CLI 审批 UI | 挂起时终端交互批准/拒绝（附理由） | 手动验收 + scripted 测试 |

**S3 完成标志**：plan mode 全流程；审批挂两天（模拟时钟）后批准原地
继续；TOCTOU 不超支；不受信 hooks 不执行。

---

## Stage 4 — 交互与上下文（模块序列）

1. **交互协议 v1**：输出事件流类型定稿（turn 边界、delta、
   `TurnDiscarded`、审批请求）；CLI 改为流式渲染；delta 只走 bus。
2. **steering/interrupt**：输入 journal 后 turn 边界消费；Esc →
   活动 activity 协作取消 → `[interrupted by user]` 渲染。
3. **并行 tool call**：loop 内并发执行 allow 的 call（ask 不阻塞其余）；
   完成按到达落盘；assembly 按原 call 顺序重排回填。
4. **context assembly 组件化**：fold → 请求的独立模块；拼装顺序落地
   （env 块 session start 冻结）；tool 输出截断；prefix 稳定 +
   cache 断点；**opaque signature 透传**（event 里持久化）。
5. **compaction**：`ContextCompacted` recorded activity，改变 fold 视图；
   阈值触发；跨边界 resume 测试。
6. **finish reason 策略**：归一化枚举 + malformed_tool_call 重试
   （`TurnDiscarded` 路径）、safety/blocked 上浮；空 candidate 注入测试。
7. **Anthropic provider**：第二实现 + `capabilities()` 协商；同一
   scripted 测试矩阵跑两个 provider，验证抽象不漏。
8. **session UX**：`sessions list/show`，resume 的流式续接。

**S4 完成标志**：单 agent 体验接近 Claude Code；`inspect` 可见缓存
命中；Esc 500ms 内杀掉任意 tool call；双 provider 测试矩阵全绿。

---

## Stage 5 — 生态与多 agent（模块序列）

1. **MCP client**：官方 SDK、生命周期带外、发现 schema 入 event、
   `mcp__<server>__<tool>` 命名、无标签按 execute-class。
2. **skills + memory 文件**：目录发现、frontmatter 解析、按需加载；
   CLAUDE.md 层级合并入 assembly。
3. **spawn/await**：子 agent 作为 activity；权限 rules spawn 时冻结
   交集；树预算 min 聚合 + 深度/扇出上限；审批沿 correlation 冒泡。
4. **handoff + pub/sub**：移交语义；blackboard topic 模式。
5. **ArtifactStore**：CAS（sha256、原子写）、`publish_artifact` tool、
   per-stream 版本、目录 manifest。
6. **outputs contract + epilogue**：run 收尾序列成形（quiesce 占位 →
   auto-publish → **[barrier no-op]** → 终态）；缺 required →
   parent error 结果。
7. **审批载荷**：`payload_ref` 启用（3.5 已预留字段）；plan 审批
   全流程（发布→审→拒→v2→批）。
8. **artifact 输入**：spawn/CLI 传 ref、materialize activity。

**S5 完成标志**：researcher 编队（parent+2 子）产出带 contract 检查的
报告；plan 审批全流程；越权/预算击穿的否定测试全绿。

---

## Stage 6 — 服务化与运行模式（模块序列）

1. **daemon**：本地 socket server 托管 runtime；CLI attach/detach
   （journal 补读 + 订阅）；`runtime.daemon: never` 降级路径。
2. **notifier**：生命周期 topic 订阅、`NotificationSent` 去重 stream、
   启动对账；通道 = user 层配置的命令/webhook（文档化 carve-out）。
3. **background effects**：`background: true`、handle = ActivityStarted
   渲染、完成 = user-role 输入、`WAITING_TASKS`、`task_output/kill`、
   `on_run_end`；epilogue 的 quiesce 占位落实。
4. **scheduler**：cron/interval → 幂等 `RunAgent`；webhook 入口。
5. **IterationDriver**：driver actor + 统一事件族；goal（verifiers:
   command/llm_judge/human + 停滞检测）；loop（self_paced 的
   `schedule_next`/`finish_series`、overlap）；best-of-N
   （`parallel{n}` + 选择 verifier）；carry 走 ArtifactStore、
   series memory 注入时截断。
6. **HTTP/WS 壳**：同一交互协议的远程暴露；headless 模式收口。

**S6 完成标志**：series 无人 attach 跑过夜出通知；goal 三轮迭代到
verifier 通过；CLI 重开 attach 回同一 run。

---

## Stage 7 — 世界状态生命周期（里程碑级，进入前做 kickoff refinement）

按 `STAGES.md`：SnapshotStore(shadow repo) → CheckpointBarrier
（弱化语义，届时设计）→ fork/rewind（双轴）→ 云 workspace 生命周期 →
OS 沙箱 backend（可提前）→ IndexStore →（可选）IDE 方向条目。
本计划不预写步骤——进入前基于 S1–S6 dogfood 数据细化并单独 review。

---

## 横切纪律

- **每步的定义**：一步 = 一个可合并的提交单元（代码 + 测试 + 必要的
  文档行），`scripts/check.sh`（ruff+mypy+pytest）全绿才算完。
- **stage 收口**：完成标志全绿 → 对抗式 review（实现 vs DESIGN.md
  一致性、代码质量、测试盲区三视角）→ 修复 → 下一 stage 的 kickoff
  refinement。
- **四个钩子的验收点**：1.4（workspace 强制）、2.7（静默记账）、
  5.6（epilogue 槽位）、2.1–2.5（event 纪律）；每次 stage review
  显式检查未被绕过。
- **规模预期**（粗估，用于校准而非承诺）：S1 ≈ 0.8–1.2k 行 + 测试；
  S2 ≈ 1.5–2k；S3 ≈ 1.5k；S4 ≈ 2k；S5 ≈ 2.5k；S6 ≈ 2.5k。
  单个 stage 若明显超出预估 50%，视为计划信号，回头审视切分。
- **不变量变更流程**：实现中发现设计不变量站不住 → 停下该步，写清
  冲突（现象、涉及不变量、备选方案）→ 单独讨论/review 后改
  DESIGN.md → 再继续。禁止"代码里先绕过去"。
