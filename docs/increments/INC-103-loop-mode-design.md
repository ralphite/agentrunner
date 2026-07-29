# INC-103 Loop mode 功能设计 v3(plan 驱动;设计先行,零实现)

> 用户裁决史:v1 固定间隔=cron-lite ✗;v2 纯自步调=与单次大 run 无区别 ✗。
> v3 核心修正:**loop 的定义单位是一份 step-by-step plan(持久 todo 清单),
> loop 的职责 = 把 plan 逐步推进到全部完成**。对标 Claude Code 的
> plan mode + todo list + 循环续作三件套。

## 1. 一句话定义

`/loop <目标>` = **先把目标拆成一份带状态的分步计划,然后在同一对话里
一步一轮地执行它,每步打勾、全绿收官**。

**与"一个 run 直接干"的区别(本设计的存在理由)**:
| 单 run | loop |
|---|---|
| 过程隐式,黑盒到结束 | **plan 是显式持久状态**,每步 pending→in_progress→completed 可见 |
| 一口气吃完,context 无界膨胀 | **每轮只领一步**,轮边界收敛 context |
| 中断=丢进度 | plan 落 journal,**崩溃/重启从下一未完步续** |
| 只能整体催/停 | **按步 steer**:插话改某步、跳某步、加步 |
| "做完了"=模型口说 | **终止=清单全绿**(每步有完成注记/证据) |

## 2. 流程(两阶段)

**阶段 A · 出计划(round 0)**
- agent 把目标拆成有序步骤清单(每步:title + 完成判据一句话),落
  journal 成为会话的 **Plan 状态**;时间线渲染成 checklist 卡片。
- 默认直接进入执行;用户可在任何时刻插话改计划(增/删/改步)。
  (可选启动开关 `需确认`:出完计划先停,等用户批准再执行——默认关。)

**阶段 B · 逐步执行(round 1..N)**
```
每轮:领取下一 pending 步(或续 in_progress 步)
  → 标 in_progress → 干活(普通 turn,全对话记忆)
  → 完成:标 completed + 一句完成注记(做了什么/证据)
    受阻:标 blocked + 原因;计划外发现:plan_update 加步
  → 清单还有 pending → 继续下一轮(默认立即;确需等外部事才 defer,带 reason)
    清单全绿      → finish:收官 summary(逐步对账),loop 摘除
```

## 3. 状态模型(会话上的持久 Plan 子状态)

`Plan = { steps: [{id, title, criteria?, status: pending|in_progress|completed|skipped|blocked, note?}], round }`
——journal 事件驱动(plan_created / step_started / step_done / plan_updated /
loop_finished),重放确定;崩溃恢复 = 读 Plan 找下一步,不靠模型记忆。

## 4. 工具面(仅 loop 挂载期间暴露)

- `plan_update{add/complete/skip/block/edit …}` —— 计划是活的,但每次变更
  落 journal、用户可见;
- `defer_next{delay, reason}` —— **仅当真在等外部事**(如 CI)才允许推迟
  下一轮,reason 必填、钳制 [10s,24h];默认路径是立即续;
- `finish_loop{summary}` —— 仅当无 pending/in_progress 步时可调(runtime
  校验,防"口头完工");有 blocked 步时 finish 必须逐条说明。

## 5. UI(全对话式)

- **Checklist 卡片**常驻更新:☑/▶/☐/⚠ 每步一行,点开看完成注记;
- 每轮分隔 chip:`Round 3 · ▶ 修复 parser 边界`;defer 时:`Next round in 8m — 等 CI`;
- 会话 pill:`Loop 3/7`;composer 常开,插话进下一轮;
- 收官 chip:`Loop finished · 7/7 done — <summary>`。

## 6. 控制面

插话(改向/改计划)· Pause/Resume · Stop(摘 loop 留对话与 plan)·
跳过某步/重开某步(经对话说即可,agent 用 plan_update 落实)。

## 7. 韧性与护栏

- Plan 与轮边界全部 journal 化:kill -9 / daemon 重启后从下一未完步续跑;
- 每轮一步 = 天然 context 收敛;compaction 兜底长计划;
- 护栏(非语义):defer 钳制、会话 token budget、可选 max_rounds 安全阀(默认关)。

## 8. 非目标

❌ 固定间隔/cron(归 Scheduled runs)❌ fresh-child ❌ verifier 门(goal 领地)
❌ 无计划的"自由续跑"(v2 已废——没有 plan 就没有 loop)。

## 9. 验收(概念级,批准后细化)

真机:`/loop 把 X 重构完` → round 0 出 checklist;每轮恰领一步、卡片逐步
变绿;中途插话"第 4 步改成 Y"生效;kill -9 后从未完步续;全绿自动收官,
summary 逐步对账;`finish_loop` 在有 pending 步时被 runtime 拒绝。
