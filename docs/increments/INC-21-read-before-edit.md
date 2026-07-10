# INC-21 read-before-edit 护栏（#32）— 📐 DEFERRED（测试适配成本）

> **状态：设计完成、实现验证可行，但 DEFER 到专轮。** 护栏实现是 S
> （Executor 一个 `readPaths sync.Map` + read/write/edit 成功记入 + edit
> 现有文件前检查），但**波及 ~10 个 scripted edit 测试**（TestEditFile、
> TestLoopMultiTurnEditsFile、TestCrashMatrix 三态、TestBarrierPerTurn、
> TestLoopRequestAssemblyGolden、TestIsolatedTeamWorkspaceSurviveRevive、
> TestEscalationApproval 等）——它们的 fixture 都是"直接 edit 现有文件、
> 无 read 步骤"，护栏一开全挂。批量给这些（含 crash matrix 等核心）
> fixture 加 read 步骤是独立的 M 工作、且改核心恢复测试风险高，不适合
> 快速 loop 轮。**放宽已验证**：read/write/edit 任一接触过即可后续 edit
> （只挡凭空 edit），但仍波及"os.WriteFile 直接建文件再 edit_file"的
> setup。专轮做时：先批量适配 fixture，再开护栏。
>
> 以下为原设计（保留供专轮实现）。

# INC-21 read-before-edit 护栏（#32）

## 动机与 journey 锚

CLAUDECODE-PARITY §2.05 #32 + UJ-02。对标 Claude Code Edit 的三检查之一：
**编辑现有文件前必须在本会话 Read 过它**。价值：防模型**盲改**——凭
幻觉对没看过的文件做 edit（改错行、覆盖不知道的内容）。现有 edit_file
已有精确匹配（old 不在文件里就报错），本增量补"先看再改"这层前置护栏。
纯工具层。

## 范围（最小形态；hash 检查拆余项）

- **本增量（核心）**：Executor 追踪本会话 read_file 成功读过的路径
  （`readPaths sync.Map[resolvedPath]bool`）；`edit_file` 修改**现有**
  文件（old != ""）前，要求该路径读过，否则返回 error result 提示先
  read_file。**创建新文件**（old == ""）不需读过（新文件本无内容）。
- **余项 INC-21b（拆出）**：磁盘未变检查（read 时存 content hash，edit
  时比对——防 read 后文件被外部改的 TOCTOU），需 hash 存储 + 精细语义。

## 关键设计（护栏是会话内软状态，不触不变量）

- read-before-edit 是 **Executor 内存态护栏**（sync.Map，并发安全，同
  index/netNone 的 per-process 内存态），**不进 journal/fold**、恢复不
  重放。它是 edit 前的一个前置检查（类精确匹配检查），不改 edit 的实际
  效果路径、不改 durability/恢复语义。
- **resume 后护栏重置**（内存态清空）：resume 后首次 edit 现有文件要求
  重新 read_file——**护栏语义可接受**（不丢数据、不破恢复；只是让模型
  在新进程里重新确认文件内容）。明示记档，同 harness-config 排除类。
- Executor 树内共享（agent 树同一 Executor）：readPaths 也共享——子
  agent read 过父可 edit（同一 workspace 视图），合理。

## Spec delta

- SPEC C「read_file/write_file/edit_file」行注记 read-before-edit 护栏
  （INC-21，会话内软护栏）；锚 `TestReadBeforeEdit*` + QA-30。
- CLAUDECODE-PARITY §2 #32 read-before-edit 状态更新（护栏 ✅，hash 余项）。

## Design delta（不触不变量）

DESIGN §5/工具面加一句：`edit_file` 对现有文件的编辑要求本会话
read_file 读过（会话内内存护栏，防盲改；不进 fold、resume 重置要求
重读）。不改 edit 效果、不改恢复语义。

## 验收

- 孪生（tool 包）：`TestReadBeforeEditAllowsAfterRead`（read → edit 现有
  文件成功）/`TestReadBeforeEditBlocksBlindEdit`（未 read 直接 edit 现有
  文件 → error，文件未变）/`TestEditNewFileNeedsNoRead`（old="" 创建新
  文件免读过）/`TestReadBeforeEditConcurrentSafe`（并发 read/edit 无
  race，`-race`）。
- 真实 API QA-30：spec 带 read_file+edit_file，让模型改一个现有文件，
  观察它先 read 再 edit（或未 read 时收到护栏 error 并纠正）；`ar events`
  归档 qa/runs/。
- `./scripts/check.sh` 全绿（绿门排除已知环境测试）。

## 实施步骤

1. Executor 加 `readPaths sync.Map`；readFile 成功后 Store。
2. editFile：old != "" 时检查 readPaths，缺则 error。
3. 孪生（含 -race）+ QA-30。
4. 文档行齐活。

## review 裁决

做。核心 S（一个 sync.Map + readFile 一行 + editFile 检查几行）。inline
自审：correctness（新文件免检、resume 重置记档）、concurrency（sync.Map
并发安全，-race 覆盖）、contract（护栏不进 fold、不改 edit 效果/恢复
语义、不触不变量）。hash 检查拆余项。
