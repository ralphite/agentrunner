# INC-102 loop 与 scheduled 对话化(/loop 改接 in-session schedule;Scheduled 点开进对话)

## 动机与 journey 锚

**用户产品要求(2026-07-24 原话归纳)**:composer 里用 loop 和目标模式,都应
像普通消息一样、以对话列表 + 各类消息的方式渲染;Scheduled 每个 item 点开,
内部也应是对话式 UI。

**锚:UJ-14(定时值守)+ UJ-24(webui 工作台)。** 现状(main dc2c58d2 对码):
- `/goal` ✅ 已对话化(Home 建普通会话再 attach、会话内直接 attach;
  ComposerController.tsx:885-922;渲染=气泡+"⚡ Sent as goal"+goal chips)。
- `/loop` ❌ 仍 `kind:"drive"` → fresh-child series 会话(958-982):时间线是
  iteration chip 流、composer 被禁("does not accept follow-up messages",
  SessionFeature.tsx:771;timeline.ts:741 注释自认 "not a conversation")。
- Scheduled 点开三分裂(useScheduledController.ts:290-345):裸 run→日志视图;
  series 会话→非对话 series 页;**in-session schedule(INC-74,背后是真对话
  会话)→ 只开设置面板,无进对话入口**。
- 底座已全备:INC-74 in-session schedule(standing prompt 注入同一对话跑普通
  turn、context 延续、durable、busy-skip,QA-74 PASS);webui 已有
  GET/POST /sessions/{sid}/schedule(detail/pause/resume/update),唯缺 attach。

**设计裁决背景(2026-07-24 与用户对齐)**:IterationDriver 的 fresh-child 语义
只对 best-of-N(并行 worktree 隔离+选优)是刚需;loop 用它构造上丢对话
context,本质退化为"重复的 scheduled run"(用户判定:无独立价值)。
CODEX-PARITY #112 早已把"带上下文定时"标为 ❌。

## 决策 #21 修订(不变量变更流程,PROCESS §四)

**旧文(DESIGN §15 决策 #21,节选)**:
> best-of-N(`parallel{n}`)、批式 loop、one-shot、driver-goal 是同一
> `IterationDriver` 的 schedule,每轮迭代 = fresh child session(隔离/prefix
> 稳定是其语义)。goal 另有会话内形态……

**为什么必须动**:fresh-child 对 loop 构造上丢对话 context;产品要求 loop 以
对话呈现并可插话。goal 已先例化(会话内形态为默认入口);loop 需同等待遇。

**新表述**:
> best-of-N(`parallel{n}`)与 one-shot 是 `IterationDriver` 的 schedule,每轮
> = fresh child session(隔离/prefix 稳定是其语义)——**driver 收窄为并行
> 选优/批式专用**。**loop 与 goal 的用户默认形态均为会话内**:goal =
> in-session goal(决策 #21 原文不变部分);loop = in-session schedule
> (INC-74:standing prompt 每 tick 以 program input 注入同一 fold 跑普通
> turn,context 延续)。driver 形态的批式 loop/driver-goal 保留为 legacy
> 兼容(旧 journal 照常投影),不再是新建入口。

**波及面**:webui composer /loop 提交路径、Scheduled 行点击与新建、timeline
schedule_* 投影、DESIGN #21/§13/IterationDriver 词条、SPEC C 表 loop 行、
CODEX-PARITY #112、GAPS(新条登记并同增量关闭)。runtime 零改动(底座已存在);
`internal/driver` 代码不动(best-of-N/兼容仍用)。

**review**:实施后三视角对抗 review(正确性/并发、安全、契约),用户点名
"review with another agent after"。DESIGN 修订与 /loop 改道实现同 commit 落地。

## Spec delta

- SPEC C 表(webui)loop 行:新建 `/loop` = 会话+schedule attach(对话形态);
  driver 批式标注 legacy。A 表 INC-74 行补:webui attach 入口(HTTP action:attach
  + composer /loop)。
- SPEC 新增/修订行锚 QA-0724(真机:/loop 落普通对话、wake 轮 context 延续、
  可插话;Scheduled 点开进对话)。

## Design delta

- 决策 #21 按上文修订;§13/词条表 IterationDriver 描述同步收窄;
  CODEX-PARITY #112 ❌→✅。

## 验收

**闸门 A(离线)**:
1. Go:webui attach action 单测(argv 映射 `schedule <sid> attach --every/--cron
   [--max-wakes] <prompt>`)+ 既有全绿。
2. 前端:`npm run build`(tsc+vite)+ vitest 全绿;新增/更新:
   startLoop 改道测试、Scheduled 行点击 → select(sid) 测试、timeline
   schedule_* chips 测试。

**闸门 B(真机,QA-0724)**:部署后真浏览器:
1. Home `/loop`(短 interval,2-3 rounds)→ **落普通对话会话**:开场消息即
   round 1,composer 可用;
2. wake 轮在**同一对话**里继续且**引用上一轮内容**(context 延续的行为证据);
3. 中途插话一条消息,agent 响应(可 steer);
4. Scheduled 页出现该 item,**点开直接进对话**;schedule 管理(pause 等)可达;
5. 旧 series 会话仍正常渲染(兼容不回归)。
证据归 `qa/runs/2026-07-24-QA-0724/`,会话保留共享 store。

## 实施步骤

1. **102.1** 后端:`handleScheduleControl` 增 `action:"attach"`(cadence 必填、
   prompt 必填、maxWakes 可选)+ Go 单测;check 相关腿绿;commit+push。
2. **102.2** 前端 + DESIGN 同 commit:api.scheduleAttach;startLoop 改道
   (Home:newSession(message=prompt 即 round 1)+attach(maxWakes=rounds-1,
   rounds≤1 则不 attach);会话内:attach 当前会话);Scheduled 行点击统一
   select(sid)、详情面板挪右键菜单"Schedule details";suggestion/新建 repeating
   预设走新流;timeline 补 schedule_attached/paused/resumed/cancelled sysChip +
   wake skipped chip;DESIGN #21/§13/词条 + CODEX-PARITY #112 同 commit;
   build+vitest 绿;commit+push。
3. **102.3** 部署 8809 → 闸门 B 真机(上列 5 条)取证归档;commit+push(证据
   路径 gitignored,findings 入 docs/QA)。
4. **102.4** 文档收口:SPEC/GAPS/LOG/QA 行齐活、工作纸归档;commit+push。
5. **102.5** **独立 agent 三视角对抗 review**(正确性/并发、安全、契约——
   契约基准 = 修订后 DESIGN + QA)全量 diff;P0/P1 修完复验再收口;commit+push。

## review 裁决

**做三视角**(用户点名)。规模=跨 前端×3 面+后端+不变量修订,达里程碑级。
