# INC-15 hooks 生命周期事件族第一批（G19）

## 动机与 journey 锚

GAPS **G19**（设计欠定·低，无 journey 直接压——目录覆盖债）+
CLAUDECODE-PARITY §2.08 #70-74：Claude Code hooks 已长到 30 事件 × 5
handler，我们只有 pre/post tool（observe+block，S2）。本增量补**生命周期
事件第一批**——都对齐既有 journal 点位、observe+block 语义不变、handler
仍 command-only（不引入 prompt/agent/http handler，那是后续）。

第一批事件（对齐 journal 产生点）：
- **SessionStart**（SessionStarted append 后）
- **SessionEnd**（SessionClosed / 显式 close）
- **UserPromptSubmit**（InputReceived 消费、turn 开始前）
- **Stop**（turn 收尾 / 静止前）
- **SubagentStart**（SpawnRequested）
- **SubagentStop**（SubagentCompleted）
- **PreCompact / PostCompact**（compactContext 前后）

## Spec delta

- SPEC D「hooks 生命周期扩展（session start/stop 等）」❌→✅（第一批），
  锚 `TestLifecycleHook*` + QA-24。
- CLAUDECODE-PARITY §2 #70 状态更新（部分：第一批事件 + command handler）。

## Design delta（不触不变量）

- 复用**现有 pre/post tool hook 机制**（决策 #11：hooks v0 只
  observe + block，是管线机件不是 effect）。加一个统一入口
  `fireLifecycleHook(kind, payload)`，在上列 journal 点位调用；命中的
  hook 以 command handler 执行（argv + JSON stdin，同现有 tool hook）。
- **observe vs block**：
  - observe-only 事件（SessionStart/SessionEnd/SubagentStart/SubagentStop/
    PostCompact）：hook 输出仅记录/通知，不改控制流。
  - block-capable 事件（UserPromptSubmit/Stop/PreCompact）：hook 非零退出
    = block（UserPromptSubmit block 拒绝该输入进入 turn；Stop block 让
    session 继续而非静止；PreCompact block 跳过本次压缩）——**与 pre-tool
    block 同语义**，不新增控制原语。
- **不触不变量**：hooks 是文档化 carve-out（同 notifier），不过四关卡、
  不 event 化副作用（决策 #11）；恢复不重放 hook（§8 决策 #8：hook 副作用
  不参与 fold）。第一批只在既有 journal 点位加**观测/否决**回调，不改
  session/turn 语义。matcher 复用现有 hook 配置形态。

## 验收

- 孪生：`TestLifecycleHookFires`（各事件在对应 journal 点位触发配置的
  hook command，收到正确 payload）/`TestUserPromptSubmitHookBlocks`（非零
  退出拒绝输入）/`TestStopHookBlocksQuiescence`（Stop block 让 session
  继续）/`TestPreCompactHookSkips`（block 跳过压缩）/`TestObserveHookDoesNotBlock`
  （observe 事件 hook 失败不改控制流）。
- 真实 API QA-24：spec 配 SessionStart + UserPromptSubmit + Stop hook
  （写标记文件 / 条件 block），真 Gemini 跑一轮，验证 hook 按点位触发、
  block 生效；`ar events` 归档 qa/runs/。
- `./scripts/check.sh` 全绿。

## 实施步骤

1. 探码确认现有 hook 机制与 journal 点位（探码 agent 进行中）。
2. hook 配置扩展（事件类型枚举）+ `fireLifecycleHook` 统一入口。
3. 各 journal 点位插调用（observe/block 分类）。
4. 孪生 + QA-24。
5. 文档行齐活（SPEC D/GAPS G19/CLAUDECODE §2.08/DESIGN §hooks/LOG/SPRINT）。

## review 裁决

做。M 号、复用既有 hook 机制、observe+block 语义不变（决策 #11 内）、
不触不变量、handler command-only（不扩 handler 类型）。第一批聚焦"事件
面对齐 journal 点位"，prompt/agent/http handler 与更多事件是后续增量。
