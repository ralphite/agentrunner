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

## INC-80.1 盘点结论（2026-07-19 完成）

**暴露面全集**：CLI `ar drive`（drive.go:233 走 legacy `d.Run`）、
`ar submit --drive`（daemon.go:886 hostDrive 同）、`ar init --driver`、
inspect/events 的 driver/iteration 展示词；webui `POST /api/runs` drive
分支（runs.go:290）、RunModal drive 表单、Scheduled run 行 +
schedule.go 的 driver-journal 投影（491-547）、RunView、`/loop`/`/bestof`
slash → `buildLoopDriver/buildBestOfNDriver` → `{kind:"drive"}`、
GoalLoopLauncher、runPreset。`/goal` 已是 in-session（非 driver）。

**E1 现状**：series runner（INC-77，driver/series.go RunSeries/
ResumeSeries，session journal 形态、不写 DriverStarted、fresh-child 走
agent.OpenChildRun）**代码+fold+测试齐备但零生产接线**——调用方只有
测试；今天一切入口仍写 DriverStarted（driver.go:235）。"写侧弃用"是
文档愿望非代码事实。runner 显式拒绝 self_paced/parallel/
on_child_failure=retry（series.go:61/69）。

**能力对照**：cron/interval/overlap/goal 三裁决/once headless ✅ 有
in-session 等价；❌ 缺 best-of-N（硬阻塞 `/bestof`）、retry、
self_paced、verifier metric_regex+human 两形态、loop fresh-child 隔离
（唯一实现即未接线 runner）。

**依赖序**：2.2a（goal/interval/cron 三类接线 merged stream + 观察面
收口）不被阻塞可先做 → 承接 `/loop` fresh-child 形态 → 2.2b runner 补
parallel/retry/self_paced → 撤 `/bestof` 与 webui 概念面（2.3）→ 删
CLI 入口（2.4）。

## 实施步骤（= audit PLAN Phase 2/3，一步一提交）

1. ~~INC-80.1 盘点与映射~~ ✅ 2026-07-19，结论见上节。
2. INC-80.2 E1 ③ 分两步：
   2a. goal/interval/cron 三类的写侧切 RunSeries/ResumeSeries（drive.go
       `d.Run`、daemon hostDrive/hostResumeDrive、boot sweep 兼容两种头）
       + inspect/events 对 series 会话单路径渲染（INC-77.4）；
       self_paced/parallel/retry 三类暂留 legacy（runner 未支持）。
   2b. runner 补 parallel(best-of-N)/retry/self_paced，legacy 写侧全退休。
   完成标志：2a 后三类新建不再写 DriverStarted；2b 后全类。
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

## 计划变更记录（2026-07-19，三视角 review 契约 P1-4 对账）

- 步骤 4 原文"删 `ar drive`/`submit --drive`"落地为**降级 transport +
  help 撤宣传**（物理保留）：thin-shell 教义要求某个 CLI 谓词承载
  webui 的 drive 提交；物理删除的前置条件是给 webui 另一条数据面
  （HTTP 壳 backlog）。决策 #41 与 LOG 记档一致，此处补工作纸对账。
- 步骤 3"前端无 driver/run 概念词"残余（RunView 兜底流、RunModal
  高级表单）已在收口 sweep 清除用户可见 driver 词；组件的存废随
  runs 面退役（同上 backlog）。
- E1④ 中"新建禁走 driver 流写侧"达成度：全形态默认 merged-stream，
  唯 parallel×retry 组合仍写 legacy 流（SPEC/DESIGN 显式声明），
  该组合补齐后 legacy 写侧全退休。
- 闸门 B 场景登记为 QA-77（五场景），真机轮跑后 GAPS 相关注记关闭。
