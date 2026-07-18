# INC-77 stream 合流:驱动收敛为程序驱动的父 session(E1 步骤③,§四)

E1 四步的第三步。①(INC-74 in-session schedule)②(INC-76 子执行
基座)已收口。本步**触不变量**,按 PROCESS §四 单独成文:旧文/为什么
动/新表述/波及面,契约视角自查后 DESIGN 修订与实现同 commit 落地。

## 一、旧不变量原文(§四 第 2 条)

1. **决策 #21**(DESIGN §15):"best-of-N(`parallel{n}`)、批式
   loop、one-shot、driver-goal 是同一 `IterationDriver` 的 schedule,
   每轮迭代 = **fresh child session**(隔离/prefix 稳定是其语义)。"
2. **DESIGN §13**:"driver 有自己的 stream 和纯 fold 状态"。
3. **代码内条款**(internal/driver/state.go):"It is deliberately
   NOT a run sub-state (the driver stream is independent; DESIGN
   §运行形态)."
4. **DESIGN §13 目标形态注**(方向,非现状):"driver 是'一种特殊
   的、由程序而非人投递 inbox 的父 session'——当前实现仍为独立子
   系统,收敛挂 UJ-22/G23。"
5. **INC-11.1 在案裁定**:观察面按 stream header 分派 run fold /
   driver fold;旧 goal/loop journal 可读并展开 iteration 子树。

## 二、为什么必须动

- §3"一套机制"教义:driver 是最后一个独立"子执行"子系统。①②已把
  cadence(schedule 子状态)与子执行(ChildRun 基座)搬进会话侧,
  独立 stream 只剩"事实记在哪"这一层差异。
- 双 fold 双投影的持续成本是实测的:13 个消费文件(driver.go/
  types.go/daemon.go/cli×5/webui×2/accept/replay),每个观察面功能
  都要写两遍(INC-11.1 的分派层、webui schedule.go 的 stdlib 镜像、
  inspect 的 driver 分支);INC-66/71/72 各自双侧修正过状态正确性。
- 决策 #21 的**本质**是两条:轮次隔离(fresh child)与调度确定性
  (谁决定下一轮)。两者都不依赖"独立 stream"——①②之后,隔离由
  spawn 事实族+ChildRun 承担,确定性由"程序驱动"承担(见下)。

## 三、新表述(方案 A,采纳)

**驱动 = 程序驱动的父 session。** 修订后条款:

- 一个 drive series 是一个 **session journal**(头 `SessionStarted`,
  Spec 内嵌 DriverSpec 派生的 series 参数;`SubStateVersions` 增
  `"series":1`)。
- **轮次事实 = spawn 事实族**:每轮 child 以 `SpawnRequested` /
  `SubagentCompleted`(+Activity bracket)记账,child 经 ChildRun
  基座运行(②已备),目录 `sub/`、命名与 spawn 一致;verdict/
  carry/stall 折进新增 **Series 子状态**(`SeriesVerdict` 等少量
  新事件,change-as-event)。`DriverStarted/Iteration*/
  DriverCompleted` 事件族**退役**(新 stream 不再写入)。
- **cadence = schedule 子状态**(①已备):interval/cron 走
  `ScheduleAttached` 家族;self-paced 走 `schedule_next` 同款 timer。
- **调度确定性保持:模型不在环。** 父 session 没有 LLM 生成——
  驱动程序(原 driver 引擎收敛成的 series runner)在安全点合成
  program-source 事实:到点→spawn 下一轮→child 静止→journal 判定
  →按 schedule/verdict 决定续/停。这正是 §13 在案措辞"由程序而非
  人投递 inbox 的父 session"的字面兑现;决策 #21 的确定性(每轮必
  跑、判定纯 fold、重发幂等)原样保留,只是事实换了户口。
- **读侧兼容维持**(INC-11.1 裁定不动):旧 driver journal 依
  stream header 分派继续可读/可展开;零 legacy 指**不写**新旧混合
  流,不指销毁历史。FoldVersion 纪律:旧 driver stream 版本不变;
  新形态版本挂 session 的 SubStateVersions。

**方案 B(否决记档)**:program 输入引导**模型**去 spawn 每轮——
完全会话化但调度确定性交给模型自由度,纯 fold 判定、重发幂等、预算
上界全部失守;与决策 #21 的本质冲突,否决。

## 四、波及面(§四 第 2 条)

| 面 | 内容 | 处置 |
|---|---|---|
| event | Iteration*/Driver* 家族(20 处) | 保留类型与解码(读侧兼容),注记 deprecated-for-write;新增 Series 事件族 |
| state | 新 Series 子状态 + fold case;SubStateVersions "series":1 | 77.1 |
| driver 包 | 引擎改写为 series runner(写 session journal);spec.go 保留(输入格式不变) | 77.2 |
| cli/daemon | drive 宿主/boot sweep/timers:新形态走 session 侧既有 sweep(schedule timer 已通),旧 drive sweep 保留至 ④ 裁撤 | 77.3 |
| 观察面 | inspect/sessions/events/webui:新形态天然走 session fold 单路径;旧 stream 分派层保留 | 77.4 |
| CLI `ar drive` | 兼容层映射(= E1④,另纸) | ④ |
| SPEC/DESIGN | F 表全部行加形态注;§13/决策 #21 修订;§17 注记 | 随各步 |
| e2e/accept | driver 场景保旧路径;新形态 QA 场景另登记 | 77.3 B 闸 |

## 五、实施步骤(每步可合并、可回退)

1. ✅ INC-77.1:Series 子状态 + 事件族 + fold + round-trip 守卫
   (纯数据层,session 侧)。事件三件:`SeriesStarted{kind,
   max_iterations, patience, overlap}` / `SeriesIteration{n, call_id,
   child_session, reason, verdict, carry_ref, carry, skipped, tick,
   usage}`(IterationScheduled+Completed+Skipped 三合一,verdict 复用
   event.IterationVerdict)/ `SeriesEnded{reason, iterations,
   best_iter}`。fold:BestIter 最高分、平分取最早;SpentTokens =
   非 skip 迭代 billed 和(settle-at-completion);LastTick 取最大
   tick(INC-54 补跑锚);重复 N 重放原位覆盖不分叉;wrong-ID no-op;
   copy-on-write(Iterations 背板克隆)。SubStateVersions "series":1。
   孪生 TestSeriesFoldLifecycle。
2. INC-77.2:series runner(程序驱动 parent:写 session journal,
   轮次走 ChildRun+Spawn 事实,判定/stall/carry 折 Series)+ 孪生
   (one-shot/loop/goal 三形态各一,fake clock)。
3. INC-77.3:daemon 宿主接新形态(session 侧 sweep 复用)+ QA
   场景 B 闸(真 Gemini 新形态两轮 + 重启复活)。
4. INC-77.4:观察面收口(inspect/sessions/webui 新形态单路径)+
   DESIGN 修订(决策 #21/§13/§17)与 SPEC/GAPS/LOG 同 commit。
5. (=E1④,另纸)`ar drive`/`submit` 映射 + driver 独立宿主裁撤。

## 六、契约 review 自查(§四 第 3 条)

- **决策 #21 修订面**:改"事实记账户口"(独立 stream → session
  journal + Series 子状态),不改隔离(fresh child 语义由 spawn 事实
  +ChildRun 保持)、不改确定性(模型不在环)、不改判定纯 fold。
- **决策 #24/#31(静止)**:父 session 的静止 = series 终态或待下一
  tick(schedule 语义,①已把"带 pending schedule timer 不静止"定为
  有意语义);child 静止判定走既有 Quiescence,②已单点化。
- **决策 #29/#30(自愈/标记)**:series 重启恢复 = session resume +
  FirePendingTimers + ChildRun 三态,全部既有机制;close/kill 标记
  天然约束新形态(session 侧标记门已在 sweep 两处生效)。
- **预算**:spawn 事实族自带 reserve/settle(树预算);driver 原
  reserve-at-launch 语义由此承接,无双记账窗口(同一事实族)。
- **凭据红线**:series 参数与轮次 prompt 走 session journal 既有
  redact 路径,无新出口。
- **风险**:程序驱动 parent 是新会话形态(无 LLM 的 session),
  观察面对"无 assistant 消息的 session"的呈现需在 77.4 核对
  (sessions list/status 派生已在 ① 处理过 timer-pending 非静止)。

## 七、review 裁决

L 级、触不变量:本纸即 §四 单独成文;77.2 落码前以本纸"契约自查"
为 review 基线,若实施中发现自查遗漏的条款冲突,停下补记本纸再续。
方案 A/B 之择已记档(B 否决理由:确定性失守);如用户另有裁夺,以
用户裁夺为准(DECISIONS-PENDING 挂 FYI 条目)。
