# INC-D1 会话内目标（in-session goal，G23 / UJ-22）— 设计稿

> **状态：设计稿，走 PROCESS §4 不变量变更流程；未实现。** 本纸是"裁决"
> 闸门的输入——**触及 DESIGN 决策 #21 / §13 不变量**，须单独 review
> （至少契约视角）通过后，DESIGN 修订与实现同 commit 落地。

## 动机与 journey 锚
- GAPS **G23**（原始需求丢失记档）+ JOURNEYS **UJ-22**。硬性要求
  （2026-07-05 补登记）：**goal 的 context 必须延续——不起新 session、
  不起 fresh run；割裂不可接受。**
- 对标 Codex：goal mode 挂在 thread 上、composer 内编辑、跑数小时~数天，
  context 全程延续。我们现有 goal 是 driver + fresh child run，context 不延续。

## 现状（两个不相交的 goal 世界）
- **driver-goal**（批式/headless）：`internal/driver/driver.go` 每轮
  spawn 一个 fresh child `agent.Loop`（ChildFactory），跑 buildTask 全新
  对话、verify child 的 final report，miss 则重来、**无 context carry**
  （仅可选 series-memory 文件注入）。fresh-child-run 教义 = §13 + 决策 #21。
- **conversational session**：`agent.Loop` 一个 Loop 待命续聊、跨 send
  延续同一 fold。二者从不相遇（DESIGN §17）。
- **关键既有件**：exchange/quiescence 检查点已存在，正是 in-session
  verifier 该住的地方——`decide()` final generation 无待处理 → doIdle；
  `idleOrReturn` 跑固定静止序列（epilogue.go：auto_publish→barrier→
  parent 回执）**每个收尾 turn 一次**。verifier 是这个序列的**新一格**。

## 不变量变更载荷（PROCESS §4）
- **旧（原文）**：决策 #21「one-shot/goal/loop/best-of-N 是同一
  IterationDriver 的四种 schedule；每轮迭代 = fresh child session」；
  §13「driver…每轮迭代 spawn 一个 fresh child session」。
- **为何必须动**：UJ-22 硬要求 context 延续。fresh-run 买到 byte-stable
  prefix / 故障隔离，但**构造上丢弃对话 context**——agent 从零重启、
  重走对话已排除的死路。开发者已裁定（LOG 2026-07-05）：fresh-run 教义
  对 goal 形态不适用，保留给 best-of-N（隔离本就是其语义）与批式 loop。
- **拟新表述**：goal 分两形态。(i) **driver-goal**（批式/headless）保留
  fresh-child-run。(ii) **in-session goal** 挂在 conversational session：
  verifier 是**固定静止序列在 exchange 边界的新一格**（final generation
  收尾处，**绝不**在 model-call 级 turn 中途——LOG 消解 (a)/(b) 之争）；
  miss → verifier 输出作为**程序来源 input 回灌进同一 fold**（下一 turn
  同上下文继续）；pass → 达成回执 + 摘 goal + 待命。generation step
  永不被挟持，检查只住 turn 收尾。

## 机制草图
- **事件族**：`GoalAttached{goal_id, verifiers, budget, source}`、
  `GoalUpdated`（mirror SpecChanged）、`GoalPaused`/`GoalResumed`/
  `GoalCancelled`、`GoalCheckpoint{verdict}`、`GoalAchieved`。
- **state**：新 Goal 子状态 fold 自上述（active goal_id/verifiers/budget/
  spent/status），fold case 挨着 SpecChanged。
- **program 输入源**：新 input source `program`——**要**fold 进对话
  （对比 interrupt/control 在 state.go 被排除）：verifier 反馈以 program
  来源进 inbox → 同上下文下一 turn。
- **静止序列新 hook**：epilogue quiescentSequence 加一格,idle 前跑：
  有 active goal → 跑 verifier；miss → GoalCheckpoint + 回灌 program input
  （唤醒下一 turn）；pass → GoalAchieved + 摘 goal + 正常待命。
- **控制面** = control 输入：pause/resume/update/cancel 全走既有 send 通道
  （update 触及 spec 冻结不变量——goal 参数须定义为**可变 session 状态**
  即事件承载,非冻结 spec,与决策 #32「变更即事件」同族）。G12 远程 stop
  顺路收编。
- **预算**：per-turn `max_generation_steps`（已有,防 runaway）之上加
  **goal 级预算**（轮数/token/墙钟）；耗尽 = 可见截断（决策 #31 一致）。

## 波及面（代码/测试/文档）
- DESIGN：改决策 #21（scope 到 best-of-N + 批式 loop）+ §13 + §18.8
  iteration 术语 + §17 收敛注记；**走不变量变更流程**。
- 代码：event/types.go（goal 事件族）、state/state.go（Goal fold）、
  runtime/ingest.go（program 源 fold 进对话）、agent/epilogue.go（verifier
  hook 格）、agent/loop.go（goal checkpoint 接 idleOrReturn/doIdle、goal
  预算耗尽=可见截断）、driver（driver-goal 与 in-session goal 分形态）。
- SPEC F 行 goal 分两形态；GAPS G23 关闭注记；JOURNEYS UJ-22 收口。

## 验收
- 孪生：scripted provider——挂 goal（verifier=命令三态）→ miss → 反馈回灌
  同 fold（断言下一 turn request 含前文+反馈）→ pass → GoalAchieved + 待命；
  pause/update/cancel 走 send；goal 预算耗尽 = 可见截断。
- 真实 API QA：会话内挂"把 flaky test 修到连续 N 次全绿"（verifier 跑 N 次
  测试命令），验 context 全程延续（模型记得已排除方向）、达成回执入对话。

## review 裁决
里程碑级 + 不变量变更：**必须**三视角对抗 review（正确性/并发、安全、
契约=DESIGN+QA）。本纸仅设计,待开发者裁决 + 不变量 review 通过后实施。
