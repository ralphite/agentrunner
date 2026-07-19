# INC-80 driver 去 user-facing（E1 收敛收编）

## 动机与 journey 锚

用户裁决（2026-07-19，原话要旨）：**driver 最初只是 loop 和目标模式的
一种抽象，是后台的设计实现方式，与 user-facing feature 没有任何关系；
从未认可任何 user-facing 的 Driver 功能。** 目标态 = 用户面只有
「会话 + 挂在会话上的 goal / loop(schedule) / best-of-N」，不存在
user-facing 的 Driver / run / series 概念词。

Journey 锚：UJ-14（定时值守）、UJ-15（通宵冲目标）、UJ-16（三路并击）、
UJ-22（会话内 goal）、UJ-24（Web UI）。这些 journey 描述的是**能力**
（cron 值守、goal 迭代、best-of-N），从不要求"driver"作为用户概念存在；
UJ-22 的硬性要求（context 延续、不起 fresh run 割裂）正是收敛方向。

背景：docs/audit-2026-07-19-inventory/{FEATURES.md,PLAN.md}——双盲评审
交集第 1 条（双方 P0）：driver 与 in-session 双基座并存、≥4 个用户入口、
E1 收敛（DESIGN §17）只走到②就两边并行加功能。

## Spec delta

- F 表（驱动）：收敛完成后各条目改写为会话形态（goal/schedule/series
  挂会话），driver 独立形态行标 🧊 收编、锚迁移；"子执行收敛为递归
  session"（现 🟡）随本增量转 ✅。
- I 表：`Web UI 调度投影`（INC-41 行）随 webui/schedule.go 镜像删除
  改写为"cadence/nextRun 由 `ar sessions --json` 输出"（对应 PLAN 3.1）。
- 附录 CLI 子命令表：删 `drive`、`submit --drive`（步骤 4 后）；
  `record-fixture`/`accept` 等开发者命令不受影响。

## Design delta

**触及不变量，走 PROCESS §四流程**（本工作纸即"单独成文"载体）：

- 新决策（落 §15 决策表，与实现同 commit）：`driver 是内部实现抽象，
  无 user-facing 面`——所有调度/目标/并击能力以会话为宿主呈现；旧
  driver journal 永远只读可查（sessions/inspect/events 兼容读保留）。
- E1 收敛③④补完：driver stream 合流进 session 事实面；CLI 兼容层
  （旧 journal 可读、新建禁走 driver 流写侧）。
- 波及面：internal/driver（写侧退役、读侧保留）、internal/cli
  （drive/submit 命令）、webui（Scheduled/RunModal/runs.go/schedule.go）、
  docs（SPEC F/I 表、DESIGN §13/§17、QA 菜单相关场景）。

## 验收

- 枚举型交付物（逐项对锚，G29 教训）：driver 暴露面全集——
  `ar drive`、`ar submit --drive`、`POST /api/runs`（drive 分支）、
  webui Scheduled Create 菜单、RunModal drive 表单、`/loop` `/bestof`
  slash 落点——每项给出 in-session 等价物 + 迁移/删除的测试锚。
- 闸门 A：既有 driver 测试迁移为 in-session series 孪生；旧 journal
  兼容读回归（TestBuildInspectTreeUsesDriverFold 类保留）。
- 闸门 B：QA 场景——cron 会话值守跨 daemon 重启（对齐 QA-74）、
  best-of-N 会话形态真跑、旧 driver session 在新版本下可查看。

## 实施步骤（= audit PLAN Phase 2/3，一步一提交）

1. INC-80.1 盘点与映射：driver 暴露面全集 → in-session 等价表；核实
   in-session series（INC-77）真实完成度。完成标志：映射表入本文件。
2. INC-80.2 E1 ③④：stream 合流 + 兼容读；新建路径全部走 session 形态。
   完成标志：写侧无新 DriverStarted、旧 journal 读侧回归绿。
3. INC-80.3 webui 撤 driver 概念面：Scheduled = 带 schedule/goal 的
   会话视图；RunModal drive 分支移除。完成标志：前端无 driver/run 概念
   词、vitest 绿。
4. INC-80.4 删 `ar drive`/`ar submit --drive` + SPEC/DESIGN/QA 收口 +
   §15 决策落表（同 commit）。完成标志：check.sh 绿 + 附录命令表更新。
5. INC-80.5（原 PLAN 3.1）`ar sessions --json` 长出 cadence/nextRun，
   删 webui/schedule.go 镜像。完成标志：webui schedule_test 迁移绿。

## review 裁决

里程碑级：INC-80.4 收口时做三视角对抗 review（正确性/并发、安全、
契约——契约基准 = DESIGN + QA）。
