# INC-55 durable Last turn diff（Codex Changes 范围）

## 动机与 journey 锚

UJ-24 第 3/6 步要求 Changes 是 runtime 真相的 review 面。当前只有
`Working tree`（当前 workspace 相对用户 repo HEAD 的全量 diff），无法回答
Codex Changes 最常用的另一个问题：**最后一条 human input 之后，workspace
发生了什么变化**。INC-41 D1 因“无 per-turn diff 契约”保持锁定。

runtime 其实已经在每个 `GenerationStarted` 前 journal 一个
`CheckpointBarrier`，其 opaque `SnapshotRef` 指向该 generation 开始前的
workspace。缺的不是新快照，而是一个不物化、不改 workspace 的只读比较契约。

本增量把 D1 解锁。UI 沿用 Codex 的 `Working tree | Last turn` 范围选择；
AgentRunner 品牌与 Supervision 不变。

## Spec delta

- Web UI Changes 增 `Working tree | Last turn` 两档真实范围：
  - `Working tree` 保持现有“workspace 相对 repo HEAD”的语义；
  - `Last turn` = 当前 workspace 相对**最新 human-class `InputReceived` 之后
    第一条带 snapshot ref 的 barrier**的变化。
- `Last turn` 是时间窗，不伪装成文件改动归因：若人或别的进程在该 barrier
  后修改 workspace，也会如实出现。UI 辅助说明写明
  “Changes in the workspace since the latest human turn began”。
- 历史 session 无 human input、输入后尚未开始 generation、snapshot backend
  不可用、snapshot ref 不合法/不可读时，返回结构化 `available:false + reason`；
  UI 显示不可用原因，不回退成 Working tree、不伪造空 diff。
- 新增只读命令面：
  `agentrunner diff <session> --scope last-turn --json`。scope 是显式枚举；
  本增量命令只承诺 `last-turn`，Working tree 继续由 Web UI 既有 repo API 提供。

## Design delta

触及 DESIGN §6 “Workspace 快照”粗体条款、§15 决策 #7 与 glossary
“只服务 rewind/fork[/best-of-N base]”，属于不变量变更，按 PROCESS §四执行。

### §四变更单

**旧不变量原文**：

> “快照藏在 `SnapshotStore` 接口后，event 只引用 opaque 的 snapshot ref——
> 上层语义不与任何具体机制耦合，只服务 **rewind/fork 与 best-of-N 的
> base 物化**。”

> 决策 #7：“workspace 快照是一等状态，走 `SnapshotStore` 接口（event 只
> 引用 opaque ref），默认 shadow-repo backend，只服务 rewind/fork。”

> glossary：“workspace 快照……只服务 rewind/fork/best-of-N base。”

**为什么必须动**：Last turn review 必须比较“turn 开始时的 durable 文件树”
与“现在的 workspace”。重新造一套 per-turn 文件日志既不能覆盖 bash/外部
进程改动，也会形成第二真相；现有 barrier snapshot 正是唯一完整、durable、
凭据排除一致的 baseline。只读比较不扩大 rewind/fork 的物化授权。

**新表述**：workspace snapshot 的**物化/状态恢复**仍只服务
rewind/fork/best-of-N base；`SnapshotStore` 另允许基于 opaque ref 做只读
workspace comparison，供 review surface 使用。comparison 不物化、不移动
shadow HEAD、不改 shadow index、不改用户 workspace；默认 backend 仍是
shadow repo，其他 backend 可明确返回 unavailable。

**波及面**：

- `internal/snapshot`: `Store` 增只读 `Diff` 能力，shadow backend 用临时
  `GIT_INDEX_FILE` 组装当前树；`None` 返回 `ErrUnavailable`；ref 严格校验；
- `internal/cli`: journal 纯函数选择 last human input 与其后第一条 barrier；
  新 `diff --scope last-turn --json` 命令；
- `webui`: `/api/sessions/{sid}/diff?scope=last-turn` 代理结构化命令；
  既有无 scope/working-tree 行为不变；
- frontend: Changes toolbar 的 Codex 式范围 menu、loading/unavailable/empty
  状态，切 session 时取消旧结果投影；
- DESIGN/SPEC/JOURNEYS/QA/GAPS/LOG 与 INC-41 D1 同步收口。

### 契约边界

1. baseline 选择是 journal 的纯函数：倒序找到最新 human-class
   `InputReceived`（`""|user|cli|unix-socket`），再正序取其后第一条
   `CheckpointBarrier{SnapshotRef != ""}`。program/agent/machine/control 输入
   不开新 Last turn 窗。
2. barrier 在 `GenerationStarted` 后、任何该 generation 的 LLM/tool activity
   前生成，因此是该 human turn 开工前的 workspace；steer 被消费后亦在下一
   generation barrier 建立新 baseline。
3. 若 input 后没有 barrier，语义是“该 turn 尚未建立 durable baseline”，
   不是空改动。
4. comparison 将 baseline ref 读入临时 index，再把当前 workspace `add -A`
   到该临时 index，以 cached diff 产出 tracked/untracked/deleted/rename 与
   numstat；沿用 shadow repo `info/exclude`，凭据永不进入结果。
5. comparison 可与 agent 运行并发：ref 固定，临时 index 独立；不读取或
   修改 shadow HEAD/index。当前 workspace 本身可变化，结果是一次只读观察，
   不宣称原子冻结。

## 验收

### 闸门 A：确定性测试

- `TestShadowRepoDiffAgainstSnapshot`：modified/new/deleted/rename 与 numstat；
- `TestShadowRepoDiffRejectsInvalidRef`：option/ref 注入不可达；
- `TestShadowRepoDiffKeepsCredentialsExcluded`：`.env` 不进入 diff；
- `TestPlanLastTurnDiffBaseline`：human source 枚举逐项、program/agent/machine
  排除、同批 input、steer 后下一 barrier、无 barrier/unavailable；
- `TestCLIDiffLastTurnJSON`：真实 shadow snapshot + journal + workspace 变化；
- `TestHandleDiffLastTurn`：Web API 成功/unavailable/CLI 失败，默认
  Working tree 回归不变；
- frontend：scope URL、切换、不可用状态、文件统计/过滤继续正确。

### 闸门 B：共享真实环境 QA-54

1. 在共享 store 创建并保留真实 multi-turn session；第一轮修改 A，第二轮
   修改 A 并新增 B；导出 journal/events 与 workspace diff；
2. live Web UI 打开 Changes：Working tree 显示两轮累计；Last turn 只显示
   第二轮时间窗；来回切换不串数据；
3. 已有 legacy/no-baseline session 显示 truthful unavailable；
4. desktop 1440×900 与 mobile 390×844、light/dark 截图归档；键盘 menu、
   Escape/focus return、loading/empty/error；稳态 console error+warning=0；
5. 所有 session/workspace/journal 保留，证据落
   `qa/runs/2026-07-11-QA54-last-turn-diff/`。

## 实施步骤

1. `INC-55.0`：工作纸 + 独立契约 review，裁决通过后才写代码。
2. `INC-55.1`：snapshot 只读 comparison + CLI + DESIGN 同 commit；A 闸核心
   测试与 `./scripts/check.sh` 全绿，立即 push `origin/main`。
3. `INC-55.2`：Web API + frontend scope UX + scripted UI tests；build/check
   全绿，立即 push。
4. `INC-55.3`：部署 live 8809，共享数据 QA-54 全景浏览器验收；三层/支撑件
   收口，D1 解锁，工作纸移 `docs/archive/increments/`，立即 push。

## review 裁决

**独立契约 pass（2026-07-11，专门 review 轮；未写实现代码）**：逐项对照
DESIGN §6、§12 barrier、§15 #7、glossary 与 `loop.go:doTurn/takeBarrier`、
`snapshot.go`、`protocol.UserClassSource` 后放行。

- 不新增 snapshot 时点，不改变 legal fork/rewind target，不改变 pinned/GC；
- 上层只见 opaque ref，comparison 是 `SnapshotStore` 能力，不让 CLI/Web UI
  知道 shadow gitDir；
- 临时 index 消除 review 与运行中 snapshot 对 shadow index 的竞争，也保证
  用户 repo HEAD/index 零污染；
- 明确“时间窗而非 agent 因果归因”，避免 UI 比后端契约承诺更多；
- unavailable 是正常能力降级，不把历史 session 错画成“0 changes”；
- 凭据 exclude、workspace 边界、bash 逃逸范围均不削弱。

review 唯一修订要求已吸收：原提案曾考虑让 Web UI 直接读 shadow repo，
这会泄漏 backend 机制并违反 opaque ref seam；裁决改为公开只读 CLI 契约，
Web UI 只消费结构化结果。规模为小增量，不另做三视角对抗 review；安全与
契约风险已由上述 ref 校验、凭据测试、并发边界和独立契约轮覆盖。
