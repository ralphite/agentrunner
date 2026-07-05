> **[归档 2026-07-05]** v2 迁移路线,已执行完毕。封存只读。

# v2 落地评估：在 v1 基础上修，还是重写？

**结论：能修，且应该修——不推倒重写。** v2 的中心模型（session/inbox/
递归子 session）可以作为对 v1 的一次**定点心脏手术**落地：约 70% 的
代码原样保留，手术集中在 `internal/agent` 的生命周期与输入路径上。
QA 菜单（C1–C10）在这条路线上全部可达。

---

## 1. 为什么能修：v2 内核需要的零件 v1 基本都有

逐条对照 v2 DESIGN §1 的循环，看 v1 手里已有什么：

| v2 内核需要 | v1 现状 | 差距 |
|---|---|---|
| 输入先落 journal 再消费 | `InputReceived` + journal-inputs-first ✅（S2 起就有，测试完备） | 无 |
| park 等待新输入 | `awaitBackground` 已实现"阻塞等完成回执/中断/超时"的 park ✅ | 只差多一个 select arm：等用户输入通道 |
| 完成回执激活新 turn | S6.1 的 task outcome 回灌 + `hasInputAfterLastAssistant → doTurn` ✅（review 修过、有回归测试） | 无——这正是 v2 "child_result 激活 turn"的现成实现 |
| 子 agent = 有独立 journal 的完整会话 | `childLoop` 本来就构造一个**完整的 Loop**（独立 store/spec/pipeline）✅ | 只差从"阻塞等它跑完"改成"后台跑 + handle" |
| 杀死子任务 | `task_kill` + cancel 注册表 + 进程组终态确认 ✅ | 只差让子 agent 进这个注册表 |
| 崩溃恢复 | fold + snapshot + in-doubt 纪律 + settle-from-child-fold（driver 已验证）✅ | 复用，但要适配新的"待命"状态 |
| 输出订阅 | daemon hub + attach ✅ | 无 |
| 关卡/预算/权限/redaction | pipeline 四关卡 ✅ | 无 |

**v1 缺的不是机件，是接线方式**：机件为"跑完一个 task"服务，v2 只是
把同样的机件接成"永续会话"。这就是能修的根本原因。

## 2. 手术清单（必须动的地方）

### 2a. 三处语义反转（危险点，动之前先写测试）

1. **yield ≠ 结束**（G6 的根）：`decide()` 里"assistant 结束 turn 且无
   新输入 → doEnd"改为"→ park 等 inbox"。`RunEnded` 只在显式 close/
   错误/预算终态时发生。**连带审计**：epilogue 时机（quiesce/auto-publish
   移到 close 时）、`resume` 对"已结束"的拒绝、`sessions list` 状态、
   accept 的 head/tail 规则、driver 对 child「跑完」的判定（driver 的
   child 用"一次性任务"flag 保持旧语义）。
2. **spawn 非阻塞化**（G2）：`buildSpawnRun` 的"等 child.Run 返回"改为
   后台任务形态（复用 bg 机制：launch 时 journal SpawnRequested +
   ActivityStarted{Background}，goroutine 跑 child，回执经既有 done
   channel 在 drive goroutine 结算 SubagentCompleted+终态）。阻塞模式
   保留为 flag（driver 与部分场景仍要它）。
3. **会话可被外部写入**：v1 的 store 被跑着的进程独占（flock），"外人
   投消息"只能经 daemon。daemon 增加 `send`（含 image ref）与
   `interrupt` command，投进 Loop 的输入通道；本地交互模式（`ar` REPL）
   是同进程 channel，不经 socket。

### 2b. 五处增量（无反转，风险低）

4. **输入通道 + park arm**（G3）：`Loop.Inputs <-chan UserInput`；
   `awaitBackground` 加一个 arm（收到 → journal InputReceived → 返回
   → decide → doTurn）；turn 边界非阻塞 drain（type-ahead 队列）。
5. **多模态 parts**（G1）：Part 加 image/file（CAS ref + 发送时
   inflate），InputReceived 加 Images，两个 provider wire 各加一段
   映射。纯增量（此前已试探过一次，半天量级）。
6. **write_file 工具 + ask_user（wait-class）**（G18/G20）。
7. **CLI 契约层**：QA §0.3 的 `ar new/send/attach/interrupt/ps/kill/
   events/close`——大多是现有命令改名/薄封装，新的是 send/ps/kill。
8. **测试基建**（G4）：routing scripted provider + fifo 编排。

## 3. 原样保留（不碰，≈70% 代码 + 全部既有测试）

store、event、pipeline、tool（加法除外）、provider wire（加法除外）、
workspace、redact、artifact CAS、snapshot/fork、index、cron、notify、
mcp、kernel。driver 与 accept 只做适配性小改。

## 4. 冻结（能力在库里，不在战线上）

goal/loop/best-of-N 驱动、fork/rewind、语义索引、网络沙箱、通知、
daemon 定时唤醒——**已实现且有测试，不删除**；QA C1–C10 绿灯前不再
投入，也不允许它们的存在阻碍手术（如 driver 需要适配就适配）。

## 5. 里程碑（每个 = 若干 QA 场景真实 API 绿灯）

| 里程碑 | 内容 | 绿灯 |
|---|---|---|
| M1 会话生命周期 | 反转 1 + 通道/park arm + daemon send + `ar send` | QA-01 |
| M2 inbox 完整化 | 队列语义 + interrupt 分立 + drain 时机 | QA-02, QA-06 |
| M3 后台子 agent | 反转 2 + kill + 回执激活 + routing provider | QA-04, QA-05 |
| M4 多模态与工具面 | parts + wire + write_file | QA-03, QA-07 |
| M5 恢复审计 | 待命态/子在飞/turn 中三态过 crash 矩阵 | QA-08 |
| 收口 | 压轴串联 | QA-09 |

顺序有依赖：M1 是一切的前提；M3 依赖 M1（回执激活复用 park）；
M2/M4 可与 M3 并行。

## 6. 风险与对策

- **最大风险**：反转 1 的连带面（"run 会结束"这个假设散布在
  epilogue/resume/accept/driver/daemon 各处）。对策：先写 C1 的
  scripted 失败测试，再全库 grep `RunEnded`/`doEnd`/`StatusEnded`
  逐点审计，一次 commit 一个连带点。
- **次风险**：并发子 agent 与共享 scripted provider 的测试不确定
  （G4）——routing provider 必须先于 M3 落地。
- **明确不做的**：不把 v1 代码搬进 v2/ 新目录重长一遍。重写会重付
  1.7 万行里 1.2 万行的搬运税，还会丢掉七个阶段攒下的回归测试网。
  v2/ 目录保持为**规格**（DESIGN/CORE/QA），实现继续在 internal/
  演进，以 QA 绿灯为唯一验收。

---

*判定依据：本评估基于对 v1 全库的逐包核对（S1–S7 全程实现于同一
session，包体量：agent 11.1k / cli 4.2k / driver 3.6k / daemon 1.7k /
其余各 <1.1k，非测试合计 ~16.7k 行）。*
