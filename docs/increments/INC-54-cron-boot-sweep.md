# INC-54 cron 跨重启唤醒 + boot sweep（GAPS G22 / HANDA-PARITY #28b）

## 动机与 journey 锚

UJ-14（loop mode / 定时系列）+ UJ-21（崩溃自愈与重启接续）。

现状缺口（GAPS **G22 ①** boot sweep 同族 / SPEC 行「cron 跨重启唤醒」🧊
backlog）：loop-mode driver（`schedule: interval|cron|self_paced`）的下一
tick 是 `awaitTick` 里的**进程内** `Clock.WaitUntil`；`hostDriveFunc` 永远
调 `d.Run` 且 `handleDrive` 每次 mint 新 id——**daemon 重启后没有任何路径
重挂一个在跑的 drive**，cron 系列直接死掉。即便手工 resume，cron 的
`d.lastTick` 是**非 fold 的 runtime state**（driver.go 旧注释「recomputable
from the clock」），resume 时归零→设为 `now`，宕机期间错过的 slot 被**静默
跳过**。

DESIGN 已承诺此行为，本增量是**兑现未落地的机制（additive）**，非改不变量：
- §运行形态 1225-1227：「常驻 runtime 也是 durable timer 的触发者……
  timeout/**cron**/审批过期的『等几天成本相同』由它兑现」。
- §13 1362-1364：「driver 依赖常驻 runtime：没有它，interval/cron **只在
  进程活着时触发**……这是文档化的**降级模式，不是默认**」——即有 daemon
  时 cron 应跨重启存活。

## Spec delta

- SPEC「cron 跨重启唤醒」行：🧊 backlog → ✅，锚 INC-54 +
  `TestDriverCronResumeBackfillsMissedTicks` /
  `TestBootSweepResumesPendingDrives`。
- SPEC「loop mode」行补注：跨重启接续由 daemon boot sweep 重挂
  （INC-54）。

## Design delta

**不触不变量**。落在既有决策的 additive 兑现：
- **决策 #30（crash vs kill）**：drive 的「显式结束标记」= 终态
  `DriverCompleted`（Status==ended）。crash 什么都不留 → Status==running、
  无终态 → **有恢复资格**。boot sweep 只重挂 Status==running 的 loop-mode
  drive，天然不越已 close/stop（已落 DriverCompleted）的系列。
- **durable timer 触发者**（§运行形态）：cron 的「下一 tick」成为
  journal 派生事实——把触发每次迭代的**墙钟 tick** 记进
  `IterationScheduled.Tick` / `IterationSkipped.Tick`（additive 事件字段，
  omitempty，不 bump FoldVersion，同 BaseRef 纪律），driver fold 派生
  `State.LastTick`。resume 从 `LastTick` 恢复 `d.lastTick`，既有
  `awaitTick` 的 overlap 逻辑即幂等 backfill。
- driver.go `lastTick` 注释更新：resume 时从 journal（`State.LastTick`）
  恢复以支持跨重启 backfill；运行中仍为进程内墙钟（不每 tick 刷 journal）。

**相邻张力（本增量不做，显式上报 / GAPS 记）**：优雅停机（SIGTERM）目前
让 idle loop drive 走 `finish("stopped")` 落 `DriverCompleted`，于是**优雅
重启**会把 cron 系列标 ended、boot sweep 不再重挂。使优雅 deploy 也保 cron
需改 driver 终态语义（shutdown→待命而非 terminal，区分 shutdown-teardown
与用户 stop），属 driver 终态语义变更，另立增量走 DESIGN §四评估。本增量
scope 锚在**崩溃重启**路径（纯决策 #30「crash 无标记→有资格」），不改
driver 终态语义。

## 验收

孪生（A 闸，全部离线确定性）：

| 交付面 | 孪生 | 锚 |
|---|---|---|
| missed-tick 恰好补跑一次（skip） | 宕机跨多 slot resume，每错过 tick 恰一条 IterationSkipped，续正常 cadence | `TestDriverCronResumeBackfillsMissedTicks` |
| missed-tick（coalesce） | 错过多 slot 折成一次 catch-up run，无 IterationSkipped | 同上（coalesce 子用例） |
| 幂等：重跑不重复 fire | 已消费 slot（fold 里 Skipped/Completed）resume 不重跑 | `TestDriverCronResumeIsIdempotent` |
| boot sweep 重挂 pending drive | ScanDrives 返回的 drive 经 ResumeDrive 重挂一次 | `TestBootSweepResumesPendingDrives` |
| 幂等：已托管/重跑不双挂 | 已在 runs 里的 drive 不被 boot sweep 再挂 | `TestBootSweepSkipsHostedDrive` |
| 无 pending 无副作用 | ScanDrives 空 → 零 ResumeDrive 调用 | `TestBootSweepNoDrivesNoSideEffect` |
| 决策 #30：ended drive 不重挂（drive 形态的标记门） | scanDriveSessions 排除 ended / goal-mode / 非 drive | `TestScanDriveSessionsGate` |
| boot sweep 重挂已有 pending **timer** | 既有 sweepTimers 首轮即 boot 重挂+补火 expired（本增量复用，不新写） | 既有 `TestTimerSweepResumesExpired` |

B 闸（真实，交由集中验，须私有新二进制 daemon）：
1. 起 daemon，`ar drive` 一个 `schedule: cron`、`cron: "*/1 * * * *"`（每分钟）
   的 spec，等它跑过 ≥1 次迭代（journal 有 IterationCompleted + tick）。
2. `kill -9 <daemon-pid>`（崩溃，非优雅——优雅停机的 cron 保活是相邻增量）。
3. 隔过 ≥2 个 cron slot（sleep >2 min）后用**同一私有二进制**重启 daemon。
4. 断言：boot sweep 重挂该 drive（`ar sessions` 见其仍 running / `ar attach`
   见续跑）；错过的每个 slot 恰一条 IterationSkipped（overlap=skip 默认）
   或一次 catch-up（overlap=coalesce）——**不重复、不丢**；此后回正常 cadence。
5. 反例：`ar close`（或跑完 max_iterations）后的 drive 重启不被重挂。
   导出 `ar events` + workspace diff 归档 `qa/runs/<日期>-INC54/`。

## 实施步骤

1. **event 字段**：`IterationScheduled.Tick` / `IterationSkipped.Tick`（additive）。
2. **driver fold**：`State.LastTick`，apply 里对两事件取 max。
3. **driver**：awaitTick 盖 tick（run/coalesce 用 `d.lastTick`，skip 用 `next`）；
   drive() journal IterationScheduled 时带 `Tick`；Resume 从 `st.LastTick`
   恢复 `d.lastTick`；更新 lastTick 注释。孪生：driver backfill/幂等三例。
4. **daemon**：`ScanDrives`/`ResumeDrive` 两个 seam；`bootSweepDrives`
   一次性启动扫描（ListenAndServe 里，紧邻 resumePendingCommandSessions）；
   `hostResumeDrive`（非交互 hub、runs 注册去重、runsWG、per-run cancel）。
   孪生：boot sweep 三例。
5. **CLI 接线**：`readDriverStarted`；`scanDriveSessions`（Status==running &&
   loop-mode）；`hostResumeDriveFunc`（读 DriverStarted → 重建装配 → d.Resume）；
   Server 挂 ScanDrives/ResumeDrive。单测：`TestScanDriveSessionsGate`。
6. **文档收口**：SPEC 行、GAPS G22 注、LOG 台账；本纸归档由收口者做。

每步 `./scripts/check.sh` 全绿。

## review 裁决

小增量、锚在崩溃重启的 additive 兑现，不触不变量。裁掉三视角对抗 review，
理由：改动限于 (a) additive 事件字段+fold、(b) 复用既有 scan-at-boot +
resume 模式（resumePendingCommandSessions/sweepTimers 同构）、(c) 幂等靠
journal 事实（consumed slot fold + runs 注册），无并发新面、无凭据面、
契约面即 DESIGN 已承诺项。**相邻的 driver 终态语义（优雅停机保活 cron）
显式上报另立增量**，不在本纸悄悄改。
