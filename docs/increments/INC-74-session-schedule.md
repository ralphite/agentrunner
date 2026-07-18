# INC-74 session 内 schedule:loop-mode 挂 conversational session（E1 步骤①）

E1(driver 收敛为递归 session,DESIGN §17 在案方向)四步拆步的第一步。
本步**不动**既有 driver:新能力与 driver loop-mode 并存,直到步骤③④
收敛完成后 driver 侧才裁撤(每步独立可合并、可回退)。

## 动机与 journey 锚

UJ-14(定时值守)+UJ-22(会话内目标)。in-session goal(INC-D1/10/48)
已证明"驱动状态挂 conversational session、context 延续"的形态优于
fresh-child driver:同一会话可中途纠偏、记忆延续、观察面统一。
loop-mode 是下一个:用户想要"这个会话每 30 分钟自己醒来做一轮",
而不是"另起一个 driver 系列、每轮 fresh child、聊天要去别处"。

## 目标形态（借 in-session goal 的全部先例）

- **事件族**(change-as-event,同 goal 家族模板):
  `ScheduleAttached{schedule_id, interval|cron, prompt, max_wakes,
  overlap}` / `SchedulePaused/Resumed/Cancelled` /
  `ScheduleWake{schedule_id, n, tick}`(每次唤醒的 journal 事实)。
  fold 出 `state.Session.Schedule` 子状态(可变 session 状态,非冻结
  spec——同决策 #32/#21 修订族)。
- **唤醒机制**:复用 daemon 既有 durable timer sweep(§12
  ScanTimers/sweepTimers 已存在):schedule 附着时 journal `TimerSet
  {purpose:"schedule_wake:<id>", fire_at}`;timer 到点 → daemon
  hostResume(automatic 路径,决策 #30 标记门天然生效)→ loop 安全点
  发现到期 schedule → journal `ScheduleWake` + 注入 program input
  (goalReinject 同模板:"Scheduled wake N — <prompt>")→ 正常 turn。
  turn 静止时若 schedule 仍活 → 依 interval/cron 计算下一 tick、
  journal 下一个 TimerSet。cron 补跑语义沿用 INC-54 定义(missed
  slot 按 overlap 恰好补一次——同一 fold 派生教义,从 ScheduleWake
  事实重建 lastTick)。
- **控制面**:`ar schedule attach <sid> --every 30m|--cron "…" [prompt]`
  / `pause|resume|cancel`(daemon `schedule-*` wire 命令,goal-* 同
  模板);webui 复用 goal 控制条模式(本步 CLI 先行,webui 拆后续)。
- **overlap**:唤醒到点而 session 忙 → skip(journal
  `ScheduleWake{skipped:true}`)——与 driver overlap:skip 同义;
  coalesce 等价于 queue 天然行为,不另做。
- **与 goal 的组合**:允许并存(schedule 唤醒的 turn 同样受 goal
  verify 支配);互不引用。

## 不变量核对

- 不触 §3"一套机制"(新增在 session 侧,driver 未动);
- prefix 冻结不触(注入走 program input,同 goal reinject 先例);
- 决策 #30:timer 唤醒是 automatic 路径,SessionMarked 门在
  hostResume 已复用;
- journal-first:TimerSet/ScheduleWake 全为 journal 事实,重启由
  timer sweep + fold 重建,无内存态。

## Spec delta

SPEC A 表新增"session 内 schedule(loop-mode 挂会话)"行(✅ 后);
F 表 loop mode 行加注"session 内形态见 A 表(INC-74),driver 形态
维持至 E1 收敛完成"。

## Design delta

DESIGN §13 加"in-session schedule"小节(goal 两形态段落的镜像);
§17 driver 收敛注记更新为"步骤① 已落(schedule),②③④ 待续"。

## 验收

孪生:TestScheduleAttachWakesAndReinjects(fake clock:attach 30m →
advance → ScheduleWake+program input+新 turn+下一 TimerSet)/
TestSchedulePauseSkipsWake/TestScheduleCancelStopsTimers/
TestScheduleSurvivesDaemonRestart(timer sweep 重建)/
TestScheduleOverlapSkipsBusySession/TestScheduleMarkedSessionNotWoken
(决策 #30)。B 闸:QA 场景(真 Gemini + 真 daemon:attach 1 分钟
cron → 两次自主唤醒各完成一 turn → pause 后不再醒)随实施登记。

## 实施步骤

1. ✅ INC-74.1:事件族 + fold 子状态 + round-trip 守卫(纯数据层)。
2. ✅ INC-74.2:loop 安全点的 schedule 检查(到期判定/Wake journal/
   reinject/下一 TimerSet)+ 孪生。
3. INC-74.3:CLI `ar schedule` + daemon wire `schedule-*` + 幂等
   重放 + 孪生;SPEC/DESIGN/GAPS/LOG 收口 + QA 场景 + B 闸。

## 74.2 实施中固化的设计判定

- **cadence 基准入事件,不入 envelope TS**:`ScheduleAttached.Base` /
  `ScheduleResumed.Base` 由 loop clock 盖章(关卡代码绝不读墙钟);
  fold 以 Base 初始化/重锚 `LastTick`,重放在任意时钟下纯。
- **resume 重锚,不补 paused 期间的 slot**:暂停是显式选择(区别于
  crash),恢复后第一个 tick = resume 时刻 + 周期;crash 漏 slot 仍按
  INC-54 教义折成恰好一次 catch-up wake(取最新到期 slot)。
- **timer 是唤醒提示,不是事实源**:到期判定每次安全点从 `LastTick`
  重推;timer 丢失/重复不丢 slot、不重复 wake(armScheduleTimer 收敛
  到恰好一个 pending timer,TimerID 由 slot 决定,重挂幂等)。
- **忙判定用对话形态,不用 Quiescence**:带 pending schedule timer 的
  session 永不静止(这语义上正确——它确实还会自己醒),故 overlap 判
  定镜像 decide() 的形态读取(pending input / 未解析 call / 欠下一步
  生成 = busy → `ScheduleWake{skipped}`,绝不中turn注入)。
- **close 撤 timer、留 schedule**(决策 #30):close 标记停自动路径,
  pending timer 一并撤销(否则关闭的会话因 timer 永不静止);schedule
  本体越标记存活,显式 send 重开后安全点自动重挂。
- **两条唤醒路径**:hosted 空闲 park(awaitInput 以 loop clock
  WaitUntil 等最早 schedule timer,到点 journal TimerFired 再走安全
  点)+ unhosted 由 daemon timer sweep hostResume(已有机制,74.3 接
  真线)。activity-timeout timer 永不从 idle 触发(归属 live executor)。

## review 裁决

中型增量,不触不变量(逐条核对如上):裁掉三视角 review,以孪生
六件套 + B 闸真验为界;若实施中发现须动既有条款,停下走 §四。
