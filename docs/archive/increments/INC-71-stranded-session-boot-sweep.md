# INC-71 mid-turn 崩溃 session 的 boot 自动接续（G22a，audit-0717 D4）

## 动机与 journey 锚

UJ-21(崩溃自愈):恢复语义齐(决策 #29 单一自愈、in-doubt 处置),
但**自动性**缺最后一块——daemon 崩溃时正在 turn 中的普通 agent
session,重启后要等人 `send` 才复活;在此之前 `inspect` 只能标
stranded。boot sweep 已覆盖 cron drive(INC-54)与 pending-command
session;本增量补第三类:mid-turn 无 pending 命令的 stranded session。

## Spec delta

SPEC E 表"crash 矩阵三态复活"行加注:daemon boot sweep 自动接续
mid-turn stranded session(标记不越、已托管跳过);GAPS G22 注 a 关闭。

## Design delta

DESIGN §12 boot sweep 段落加第三类。**不触不变量**——完全复用:
hostResume(explicit=false) 恒守决策 #30(标记不越)与已托管幂等;
resume 内部走既有决策 #29 in-doubt 自愈。新增仅是"谁来触发"。
扫描判据(cli 侧):journal 头 SessionStarted(排除 driver)、尾非
SessionClosed、fold Status==running(waiting=干净 park 不扰)、
无 live writer(防抢活宿主)。

## 验收

- 孪生:daemon 侧 TestBootSweepResumesStrandedSessions/
  TestBootSweepStrandedSkipsMarked(经 SessionMarked 门)/
  TestBootSweepStrandedSkipsHosted;cli 侧
  TestScanStrandedSessions(命中 mid-turn / 跳过 parked / 跳过 closed
  / 跳过 driver / 跳过 live-writer)。
- B 闸:随下次真实 daemon 重启 QA(与 INC-54 同法集中验)记档。

## 实施步骤

1. 单 commit:Server.ScanStranded + bootSweepStranded + cli
   scanStrandedSessions + 接线 + 孪生 + 文档行齐活。

## review 裁决

小增量、纯复用既有恢复语义:裁掉三视角 review,以孪生 + 决策 #30
门测试为界。
