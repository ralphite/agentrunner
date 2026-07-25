# INC-103 Loop mode 设计稿(设计先行——未经用户批准零实现)

> 教训与流程约束:INC-102 因未与用户确认"对标物的定义性语义"而返工并全部
> 移除。本稿先立**语义对照**,每一条与 Claude Code(下称 CC)的异同显式标注;
> 用户逐节批准后才进实施。

## 一、语义对照(CC loop 的定义性语义 → 本设计)

| # | CC loop 语义 | 本设计 | 异同 |
|---|---|---|---|
| 1 | `/loop <任务>` 默认**不带间隔**——**自定步调**:每轮结束由 agent 决定下次何时醒(按它在等什么),并给出 reason | 同。每轮 turn 末 agent 必须调 `schedule_next{delay, reason}` 或 `finish_loop{summary}` 之一 | **同** |
| 2 | **agent 自行终止**:任务干完自己停,无需 verifier/轮数上限 | 同。`finish_loop` 即收官(落终态事件,loop 摘除) | **同** |
| 3 | 固定间隔(`/loop 5m …`)只是可选变体 | 同。带间隔时不给 pace 工具,按钟醒 | **同** |
| 4 | 循环在**同一会话/上下文**里进行,轮间有全部记忆 | 同(挂在 conversational session 上,不起 fresh child) | **同** |
| 5 | 用户可随时插话/停止 | 同(composer 常开;`/loop stop` 或 UI Stop) | **同** |
| 6 | delay 有钳制(CC:60s–3600s clamp) | `pace_min`(默认 10s)/`pace_max`(默认 24h)钳制;忘调工具 = 视为 finish(防自旋) | 同思想,参数待定 |
| 7 | CC 无轮数上限概念 | 默认无上限;可选 `max_rounds` 作**安全阀**(非语义) | 弱化差异,显式声明 |

## 二、功能清单(用户逐条 ✓/✗)

1. **入口**:composer `/loop <任务>`(Home=新会话,会话内=挂当前会话)。启动器仅两个可选项:`Every`(留空=自定步调,默认)与 `Max rounds`(留空=无上限)。
2. **每轮形态**:普通 turn(全对话记忆);turn 末 agent 面上有 `schedule_next` / `finish_loop` 两工具(仅 loop 挂载期间暴露)。
3. **时间线渲染**:对话式。chips:`Loop started`、`Next round in 8m — <agent 的 reason>`、`Round N`、`Loop finished — <summary>`;composer 全程可用。
4. **控制面**:插话(steer,下一轮可见)、Pause/Resume、Stop(摘 loop 留对话);Scheduled 页列出进行中的 loop(显示 agent 定的 next round),点开即对话。
5. **韧性**:agent 选的 delay 落 durable timer;崩溃/重启按既有 timer-sweep 恢复;忘调工具按 `on_no_intent=finish` 收官并注记。
6. **预算护栏**:pace 钳制 + 可选 max_rounds + 既有 session token budget;三层都可见于 Scheduled 详情。

## 三、与现有底座的映射(实施时零新状态机)

- 复用 in-session schedule 事件族(attach/wake/cancel + durable timer)——只新增"下一 tick 由 `schedule_next` 写入"与"`finish_loop` → cancel"两条边;
- `schedule_next`/`finish_series` 工具语义已存在于 driver self_paced 形态,搬到 session 工具面并改名 `finish_loop`;
- best-of-N/goal/driver 不动。

## 四、开放问题(请裁决)

- Q1 `/loop` 不带任务直接跟上文干?(CC 允许"接着当前事继续循环")——建议:允许,任务缺省 = "继续当前工作"。
- Q2 pace 默认钳制值(10s–24h?)与 `on_no_intent`(finish vs 按 pace_min 续)。
- Q3 Scheduled 页是否也提供"新建 loop"入口,还是仅 composer。

## 五、验收草案(批准设计后细化)

真机:`/loop` 一个真实任务 → agent 每轮自报 next delay+reason、干完自行
finish;中途插话改向生效;崩溃重启后按 agent 定的 delay 照醒;`Every 5m`
变体按钟醒。
