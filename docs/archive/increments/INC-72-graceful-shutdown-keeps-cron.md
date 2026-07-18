# INC-72 优雅停机保活 cron（G22b，audit-0717 D5）——不变量变更流程（PROCESS §四）

## 旧不变量原文

DESIGN §13"统一事件族":`DriverCompleted{reason: …|stopped|…}` 为系列
终态;§12 ScanDrives 注释与 GAPS G22 表述"drive 的终态 DriverCompleted
= 其显式结束标记(决策 #30),scanDriveSessions 排除"。**实际语义**:
任何 ctx 取消(用户 stop、优雅 SIGTERM、换 agent teardown)都落
`DriverCompleted{stopped}` 终态——优雅 deploy 后 cron 系列永久死亡,
而 kill -9(无终态)反而能被 boot sweep 复活。优雅比崩溃更糟,倒挂。

## 为什么必须动

UJ-14(定时值守)的存在前提是系列跨 deploy 存活。GAPS G22 注 b 明确
"须区分 shutdown-teardown 与用户 stop,另立增量走 §四评估"——意图
在案,本纸即评估与落法。

## 新表述

**loop-mode(interval/cron/self_paced)系列的终态 `DriverCompleted`
只由两类事实产生:用户显式 stop,或系列自然终点(max_iterations/
budget/finish_series/child_failed)。优雅停机(host shutdown)对
loop-mode 系列是无终态 teardown——journal 与 crash 同形,下一次
boot sweep 经既有 Driver.Resume 幂等重挂、按 overlap 补跑错过的
slot。** goal/parallel(bounded)系列不变:shutdown 仍落
stopped 终态(它们不被 boot sweep 重挂,无终态会永久 stranded,
记档)。

机制:取消 **cause** 区分——daemon 优雅停机以
`errs.ErrHostShutdown` 为 cause 取消宿主 ctx(cause 沿 ctx 树传播);
用户 stop 保持既有 `errs.ErrSessionStopped`。driver 在取消分支查
cause:shutdown 且 loop-mode → 返回不 journal 终态(mid-iteration
时仍照常 journal `IterationCompleted{canceled}` 事实,只略去终态)。

## 波及面

- `internal/errs`:新增 `ErrHostShutdown` sentinel。
- `internal/cli/daemon.go`:daemonCmd 把 signalContext 的取消包一层
  WithCancelCause(ErrHostShutdown) 后传给 ListenAndServe。
- `internal/driver/driver.go`:loop-mode 可达的 3 个 finish("stopped")
  站点改走 `cancelTerminal`(cause 判定 helper);parallel 路径 2 个
  站点同 helper(schedule 门自然排除,行为不变)。
- DESIGN §13 统一事件族段 + §12 ScanDrives 注释补一句新语义;GAPS
  G22 注 b 关闭;SPEC F 表 cron 行备注更新。
- 测试:TestDriverShutdownCutLeavesNoTerminal(shutdown cause →
  无 DriverCompleted、fold 非 Ended、可被 scan 选中)/
  TestDriverUserStopStillWritesTerminal/
  TestDriverGoalShutdownStillWritesTerminal(bounded 不变)。
  既有 TestBootSweepResumesPendingDrives/TestDriverCronResumeBackfills*
  锚定重挂半边,不重写。

## 契约视角 review(单独成文)

- 决策 #30(标记不越)不受影响:无终态≠无标记语义——close/kill 标记
  另有事件,scan/hostResumeDrive 的 SessionMarked 门原样。
- journal 真实性:shutdown 不写"stopped"恰恰**更**诚实——系列没有被
  任何人停,是宿主没了;与 crash 同形是事实同形。
- 幂等:重挂路径全部复用 INC-54 已验机制(fold 派生 tick、consumed
  slot 去重),无新状态。
- 风险:旧二进制读新 journal 无新事件类型,零 schema 变化;唯一行为
  差异是优雅停机后 loop 系列会在下次 boot 复活——这正是需求本身,
  且用户随时可 `ar stop` 落真终态。

## 实施步骤

1. 单 commit:sentinel + cli 接线 + driver cause 判定 + 3 测试 +
   DESIGN/GAPS/SPEC/LOG 齐活,check.sh 全绿。
