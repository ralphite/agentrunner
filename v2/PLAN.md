# v2 实施计划（loop 模式执行）

目标：按 MIGRATION.md 的路线在 v1 代码上动手术，达到 CORE.md 十项 /
DESIGN §11 C1–C10，以 QA.md 场景绿灯为唯一验收。

## §0 执行协议（loop 模式，沿用 v1 §0.5 纪律 + v2 修订）

1. **session 开始**：`git fetch origin main` + fast-forward。
2. **一步 = 一个可合并提交**：代码+测试+文档行齐活，`./scripts/check.sh`
   全绿才 commit；消息格式 `V2-M<里程碑>.<步>: <摘要>`；立即 push
   `origin/main`（只用 main）。
3. **台账**：决策/偏差/记档写 `v2/PROGRESS.md`（v2 的决策台账；v1 的
   PROGRESS.md 封存不再追加）。
4. **回归红线**：v1 的全部单元测试与 stage 1–7 的 26 个 acceptance
   场景**每一步都必须保持绿**——v2 的行为变化一律以 opt-in 方式落地
   （conversational 模式、background spawn flag），不破坏 task 模式。
5. **里程碑出口（双闸门）**：
   - 闸门 A：该里程碑的 **scripted 孪生测试**全绿（离线、确定性，进
     check.sh 常跑）；
   - 闸门 B：对应 **QA 场景真实 API 跑通**（.env 的 GEMINI_API_KEY，
     模型 gemini-flash-latest 控制成本；QA.md 的重跑规则适用；结果
     归档 `v2/qa/runs/`）。B 闸失败 → 归因（runtime bug / prompt /
     环境）→ runtime bug 必须修复后重跑。
6. **里程碑出口 review**：M3 与收口各做一次三视角对抗 review（正确性/
   并发、安全、契约——契约基准 = v2/DESIGN.md + QA.md），P0/P1 修完
   才进下一里程碑。
7. **设计冲突**：实现中发现 v2/DESIGN.md 语义走不通 → 停下，在
   v2/PROGRESS.md 写清冲突，修订 DESIGN 与实现同 commit 落地——
   禁止代码悄悄绕。
8. **loop 自续**：每 tick 收尾 `ScheduleWakeup`；一步做完即 commit，
   不留未推送状态过 tick。

## §1 里程碑与步骤

### M1 会话生命周期（→ 闸门 QA-01）

- **M1.1 conversational park**：`Loop` 加 `Conversational bool` 与
  `Inputs <-chan …` 通道。decide()：conversational 且 assistant 已
  yield 且无待办 → 不走 doEnd，journal `WaitingEntered{input}` 并 park
  （select：Inputs / Interrupts / ctx）；收到输入 → `WaitingResolved`
  → doTurn。显式 close（channel 语义或 control 输入）→ epilogue →
  `RunEnded{closed}`。**先写红测试**（scripted：3 条输入 3 个 turn、
  期间无 RunEnded、close 后才有终态），再动 decide()。task 模式
  （Conversational=false，默认）行为零变化。
- **M1.2 外部投递**：`PostInput`：经 store 的互斥 Append 直接落
  `InputReceived`（journal-inputs-first、崩溃不丢），channel 只做
  唤醒信号；loop 消费时 fold 该 envelope。daemon 加 `send` command
  （复用 hostedRun 的会话注册表），CLI 加 `send <sid> "text"` 与
  `new <spec> --workspace <dir>`（托管 conversational 会话，输出 sid）。
- **M1.3 park 恢复**：parked 会话 crash/重启 → resume 折出
  waiting:input → 重新 park；`sessions list` 显示该状态；旧 accept
  规则不受影响（v2 会话有自己的场景组）。
- **出口**：scripted C1 孪生绿 + **QA-01 真实 API 绿** + 回归红线。

### M2 inbox 完整化（→ QA-02, QA-06）

- **M2.1 忙时排队**：turn 在飞时 PostInput 照常 append（时间戳落在
  activity 区间内=断言依据）；drive 循环在 turn 边界统一把"journal 里
  已有、fold 已见但未消费"的输入按序纳入下一 turn（复用
  hasInputAfterLastAssistant 族机制，扩展为多条批量）。
- **M2.2 interrupt 分立回归**：同一在飞态下 send=排队、interrupt=取消
  （v1 机制已有）；补两条路径互不串扰的 scripted 测试。
- **出口**：C2/C8 孪生绿 + **QA-02、QA-06 真实 API 绿**。

### M3 后台子 agent（→ QA-04, QA-05；核心里程碑）

- **M3.0 routing provider**（前置，G4）：scripted 路由器——按子会话的
  task/session 匹配各自 fixture；并发下确定。
- **M3.1 background spawn**：`spawn_agent{background:true}` 走 bg
  机制：launch 时（drive goroutine）journal SpawnRequested +
  ActivityStarted{Background}，goroutine 跑 childLoop.Run；settle 时
  journal SubagentCompleted + 终态 + usage（child fold 结算）。阻塞
  模式保留（driver/兼容）。
- **M3.2 杀死**：task_kill 覆盖子会话 handle（cancel 注册表已通用）；
  daemon/CLI `kill <sid> <handle>` 用户直杀路径；子侧收尾语义 =
  ctx cancel → 子 journal 终态 → 父收 canceled 回执。
- **M3.3 回执激活**：bg 完成回执唤醒 conversational park（
  awaitBackground 已 select bg.done——把 park 与它合一），先回先处理。
- **M3.4 崩溃结算**：父重启时对在飞子会话 settle-from-child-fold
  （driver 先例推广）；孤儿进程检查。
- **出口**：C3/C4/C5/C6 孪生绿 + **QA-04、QA-05 真实 API 绿** +
  三视角 review。

### M4 多模态与工具面（→ QA-03, QA-07；可与 M3 并行度高）

- **M4.1 消息 parts**：provider.Part 加 image/file（MediaType/Ref/
  Data(json:"-")）；InputReceived.Images；fold 出 image parts；发送前
  从 CAS inflate（组装处，副本不动 fold）。
- **M4.2 wire 映射**：gemini inline_data + anthropic image block；
  scripted 断言 ref-not-bytes。
- **M4.3 write_file 工具** + 长贴折叠（阈值转 file part）。
- **出口**：C9 孪生绿 + **QA-03、QA-07 真实 API 绿**（QA-07 用
  qa/fixtures/build-error.png 验 vision 三要素）。

### M5 恢复审计（→ QA-08）

- **M5.1 crash 矩阵扩展**：三态（park 中 / turn 中 / 子在飞）× kill -9
  → resume；崩溃前已 journal 的排队输入重启后被消费。
- **出口**：C10 孪生绿 + **QA-08 真实 API 绿**。

### 收口（→ QA-09）

- **F.1 压轴串联**：QA-09 七步真实 API 全程；`ar ps`/`events` 观察面
  补齐（ps = fold 的在飞子会话列表）。
- **F.2 文档同步**：实现学到的语义回写 v2/DESIGN.md；CORE.md 对照表
  更新为"已达成"；GAPS.md 关闭 G1/G2/G3/G6（标注关闭位置）。
- **F.3 出口三视角 review** + 修复。

## §2 依赖与并行度

M1 → M2 → M3 严格串行（park 是回执激活的载体）；M4 与 M3 可交错；
M5 依赖 M1–M4 全部；收口最后。loop 每 tick 取当前里程碑的下一步，
一步一 commit。

## §3 风险登记（来自 MIGRATION §6，执行时对照）

- 反转"run 会结束"的连带面：动 decide() 前先 grep
  `RunEnded|doEnd|StatusEnded` 建立审计清单，逐点过。
- 真实 API 闸门的花费纪律：flash 模型、场景短促、失败先归因再重跑。
- conversational 与 driver/epilogue 的边界：driver 的 child 永远
  task 模式；quiesce/auto-publish 移到 close 路径时保 epilogue 顺序
  不变量（钩子 2）。
