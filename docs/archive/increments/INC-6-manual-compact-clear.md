# INC-6 手动 compact / clear（G7）

## 动机与 journey 锚
- **缺口**：GAPS **G7**（手动上下文操作，设计欠定）——只有自动阈值
  compaction；`/compact 带指示`、`/clear` 缺。DESIGN §18.2 已把
  "未来 pause/compact 等"列为**预期的 control 输入**——本增量把它落实。
- **journey**：UJ-09 步骤2「嫌摘要丢了关键约束 → 手动 `/compact 保留 API
  设计的所有决定`」。
- **对标 Codex**：手动 compact（含指示）/ clear 是标配上下文控制。

## Spec / Design delta
- SPEC A 行 `手动 compact（带指示）/ clear` ❌ → ✅。
- DESIGN §4 context assembly：手动 compact = 与自动同一 ContextCompacted
  机制、**无条件**、directive 附加进 harness summarizer prompt；`/clear`
  = 兄弟 control，复用 ContextCompacted{Summary:""}（assembly 见空 summary
  跳过摘要头，view = msgs[Boundary:] 为空）+ 事件加 `Cleared bool`（
  additive-optional，不 bump schema，供 timeline 诚实区分）。
- **不触不变量**：control 输入是 DESIGN §18.2 已预留的族；compact 复用
  既有 compactContext（journaled LLM activity、idempotent、不过 permission
  管线的 harness 维护调用）；clear 只挪 boundary、不调 LLM。

## 机制（transport）
- `protocol.Control{Kind, Directive}`；`Loop.Controls <-chan Control`
  （nil = 不接，select 永阻，与 UserInputs/Cancels 同）。
- 处理点唯一 = 安全边界 `drainControls`（exec 在此可用）：非阻塞排空
  channel + `ds.pendingControls`，逐条 apply。
- 待命处（awaitInput idle select）加 `case ctl := <-l.Controls`：存入
  `ds.pendingControls` + `resolve("control")` 唤醒 → 回安全边界
  drainControls 处理 → decide()→doIdle 继续待命（compact/clear 不起 turn）。
- daemon `compact`/`clear` 命令（Command.Directive）→ hub.controls，
  ack 即返回（best-effort，与 interrupt/kill 同）；`ar compact [指示]`/
  `ar clear` CLI。
- 退化保护：仅当 `len(Messages) > Compaction.Boundary` 才落事件（空会话
  compact/clear 是 no-op）。

## 验收
- **闸门 A（孪生，internal/agent）**：
  - `TestManualCompact`：跑几轮 → post Control{compact, directive} → 到
    边界恰好一条 ContextCompacted、summary 非空、下一 turn request 丢弃
    compaction 前的体量。
  - `TestManualClear`：post Control{clear} → ContextCompacted{Cleared,
    Summary:""}；assembly 后 view = msgs[Boundary:]（空摘要头不出现）。
  - `TestCompactAtIdle`：待命中 post control → 处理后仍待命（不起 turn）。
  - `TestEmptyCompactNoop`：空会话 compact/clear 不落事件。
- **闸门 B（真实 API）**：真 daemon，聊两轮 → `ar compact 只保留 API 决定`
  → journal 出现 context_compacted、后续答复仍连续（QA-12）。

## 实施步骤
1. 一步（`INC-6: manual compact/clear`）：protocol.Control；event Cleared；
   assembly 空摘要跳头；compaction 参数化 directive/manual + clear 助手；
   loop Controls+drainControls+driveState.pendingControls；awaitInput case；
   daemon Directive+controls+handleCompact/Clear+dispatch；CLI 两命令；
   测试 + 文档 + check.sh（TMPDIR=/tmp/t）→ commit → push。

## review 裁决
中增量（M，触核心 select）：inline 自审重点——安全边界与待命两路的
control 汇流、compact 活动 id 不撞自动、clear 空摘要 assembly、nil channel
安全。裁三视角对抗 review（记档：机制被孪生+真验双闸门覆盖）。
