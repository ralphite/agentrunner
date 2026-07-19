# INC-82 · 动词面收敛第一刀：维护手势不复活 close 标记

**日期**：2026-07-19 · **来源**：docs/audit-2026-07-19-inventory/PLAN.md
Phase 4.2（用户裁决：目标只有两个用户概念——**打断**（可被 send 复活）
与**关闭**（不被 compact/clear 复活）；stop 并入打断族，见 4.1 对账）。

本文件是 PROCESS.md §四 不变量变更流程的"单独成文"件。

## 一、旧不变量原文

DESIGN §恢复（INC-74，2026-07-18 落）：

> **任何显式重开都清 close/stop 标记（INC-74）**：两个对称信号——
> `GenerationStarted`（send 起新 turn）与 `WaitingEntered`
> （compact/clear/revive 无 turn 直接重新待命）——都清标记,故复活后
> 状态一律回到 `waiting:input`,不残留 "closed"（quinn-02）。

SPEC 登记簿（行 24）：

> close = 标记；**任何显式重开——send 或 compact/clear 复活——清标记，
> 状态回 waiting:input，不残留 closed**，INC-74

代码：`internal/state/state.go` fold 的 `WaitingEntered` 分支
`s.Session.Closed = nil`；钉子 `TestReopenAfterCloseClearsMark`。

INC-74 当时的裁决：quinn-01（compact/remember 复活关闭会话）=
by-design——"compact/remember 是显式用户命令，非自动路径，允许复活"。

## 二、为什么必须动

用户裁决（2026-07-19 评审收口）：close 的语义应当是"关闭，且**不被
compact/clear 复活**"。compact/clear 是**维护手势**——用户在整理一个
会话的上下文，不是在表达"让它重新活过来"的意图。INC-74 把两者混为
一谈：为了修状态撒谎（quinn-02，fold 报 closed 但 send 确能继续），
选择了"维护手势=重开"的宽口径。实际上状态诚实另有出路：`ar sessions`
/`inspect` 的 status 派生里 **Closed 标记本就优先于 waiting**
（resume.go:401），标记存活时报 "closed" 是真话——因为会话确实还关着。

## 三、新表述

**清 close/stop 标记的重开信号只有一个：`GenerationStarted`**——真实
输入起了新 turn（send / schedule-attach 后首个 tick 注入 / goal 回灌
/ 子会话 revive 邮件起 turn，殊途同归）。`WaitingEntered` 不再清标记：
维护手势（compact/clear）在 closed 会话上照常执行（整理上下文、落
journal），执行完**会话仍是 closed**——fold 报 closed 是诚实的，
send 随时可复活（决策 #30 越标记特权不变）。

决策 #30 本体（标记+检查、自动路径不越、send 显式越）**不动**；动的
只是 INC-74 加的"WaitingEntered 也清标记"对称条款——收回。

## 四、波及面

| 面 | 变更 | 核对结论 |
|---|---|---|
| `internal/state/state.go` WaitingEntered 分支 | 删 `s.Session.Closed = nil`，改注释 | GenerationStarted 分支（真重开）原样保留 |
| `TestReopenAfterCloseClearsMark` | 改写为 `TestMaintenanceAfterCloseKeepsMark`：compact→park 后标记仍在、Quiescence 报 "closed"；随后 send（InputReceived+GenerationStarted）清标记 | SPEC:24 锚同步换名 |
| status 派生 | 无需改 | resume.go:401 标记优先于 waiting，已诚实 |
| schedule | 无需改 | checkSchedule（schedule.go:208）已有 Closed 守卫：标记在则撤 timer 不武装；attach 复活流经 send/tick 起 turn 走 GenerationStarted |
| hook ingress / boot sweep / timer sweep | 无需改 | machine 与自动路径本就不越标记（决策 #30/#39） |
| 子会话 revive | 无需改 | revive 邮件起 turn → 子 journal GenerationStarted 清标记 |
| remember | 行为随信号走 | remember 注入 program input 若起 turn 则经 GenerationStarted 复活——与 send 同类（有真输入起了 turn），不在本刀收口范围 |
| DESIGN §恢复 | 重写 INC-74 条款为 INC-82 表述 | 同 commit |
| SPEC:24 | 改写行文+换锚 | 同 commit |
| e2e / qa / webui | 无引用 | grep 无依赖 compact 复活的场景 |

## 五、契约视角自审（单独 review 要求）

- **兼容性**：纯 fold 语义变更，不改事件 schema、不改 journal 写侧——
  旧 journal 重放到新 binary，close 后被 compact 过的会话从
  "waiting:input" 变回 "closed"。这正是本增量的意图（历史误复活的
  会话回归其真实语义），且 send 仍可继续，无功能损失。
- **单调性**：不涉 sub-state 版本（Session 无版本化 schema 变更，
  字段集不动）。
- **对称破缺是有意的**：GenerationStarted 清、WaitingEntered 不清——
  分界线是"是否有真实输入起 turn"，这比 INC-74 的"两信号对称"更接近
  用户意图模型。
