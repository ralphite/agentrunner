# INC-103 Loop mode 功能设计(完整版,v2——纯自定步调,无任何固定间隔)

> 状态:设计稿,待用户批准;零实现。
> 用户裁决(2026-07-25):**固定间隔彻底移除,loop 永远不基于时间表**。
> 定时/cron 需求归 Scheduled runs(automations)那一族,与 loop 无关。

## 1. 定位

**Loop = 让 agent 在同一个对话里持续工作:每轮结束由 agent 自己决定
「立刻继续 / 睡多久再继续 / 已完成收官」。** 节奏与终点都是 agent 的判断,
不是用户预设的参数。对标 Claude Code `/loop` 的本体语义。

## 2. 定义性语义(五条,全部硬性)

1. `/loop <任务>` 启动;任务缺省 = 继续当前对话正在做的事。
2. 每轮 = 同一会话里的一个普通 turn,轮间全量对话记忆。
3. **turn 末 agent 必须二选一**:`schedule_next{delay, reason}`(何时继续+为什么)或 `finish_loop{summary}`(自行收官)。没有第三态。
4. 没有 interval、没有 cron、没有轮数概念。唯一的节奏来源是第 3 条。
5. 用户全程可插话(steer 进下一轮)、可 Pause/Resume、可 Stop;这些是控制面,不是语义。

## 3. 入口与命令面

- composer `/loop <任务>`:Home = 建新会话并开跑;会话内 = 当前会话开跑。
- **无 launcher 面板**——没有参数可填,回车即跑(与发普通消息同重量)。
- `/loop stop`(会话内)= Stop;Scheduled 页不提供新建入口(loop 不是"排程")。

## 4. 每轮生命周期

```
round N turn 开始(任务/上一轮延续 + 期间插话)
  → agent 干活(工具/输出,普通 turn)
  → turn 末:schedule_next{delay,reason} → 挂 durable timer,会话空闲
             finish_loop{summary}      → loop 摘除,落收官 chip,对话照常
  → timer 到 → round N+1(program 注入「继续」,含 reason 回显)
```

- **delay 钳制**:`[10s, 24h]`,越界收敛到边界并注记。
- **兜底**:turn 末两者都没调 = 视为 finish(注记 "loop ended — no continuation requested"),宁可早停不可自旋。
- **busy 语义**:timer 到时若会话正忙(用户在聊),该轮顺延到空闲边界,不叠加。

## 5. 工具契约(仅 loop 挂载期间暴露给模型)

- `schedule_next{delay: duration, reason: string}` —— "我在等什么/为什么这个节奏"。reason 必填,用户可见。
- `finish_loop{summary: string}` —— 收官陈词,用户可见。

## 6. UI / 渲染(全对话式)

- 时间线 chips:`Loop started`;每轮间 `Next round in 8m — 等 CI 跑完`(agent 的 reason 原文);`Round N` 分隔;`Loop finished — <summary>`。
- composer 常开;插话即 steer,下一轮开头可见。
- 会话头部/侧栏:进行中 loop 显示 pill(`Loop · next round in 8m`);无独立管理页。

## 7. 控制面

| 动作 | 语义 |
|---|---|
| 插话 | 进下一轮 context(不打断当前轮) |
| Pause | 冻结待挂的下一轮;Resume 恢复(delay 从 resume 起重算) |
| Stop | 摘 loop,对话保留;正在跑的轮走完 |
| Interrupt | 既有会话语义,打断当前轮(与 loop 正交) |

## 8. 韧性

- `schedule_next` 的 delay 落 **durable timer**(journal 事实);崩溃/daemon 重启后按既有 timer-sweep 照醒,漏 slot 恰好补一次。
- Pause/Stop/finish 均为 journal 事件,重放确定。

## 9. 护栏(全部非语义,防跑飞)

- pace 钳制(§4);
- 会话既有 token budget 照常封顶;
- 可选 `max_rounds` **安全阀**:默认关;开着也只是"到 N 轮强制 finish 并注记",不改变语义。

## 10. 事件模型(概念)

`loop_started{task}` / `loop_next{delay, reason}`(+durable timer)/
`loop_round{n}` / `loop_paused/resumed` / `loop_finished{by: agent|user|guard, summary}`。
实施时可复用 in-session schedule 事件族改语义,或新立 loop_* 族——实施阶段定,
不影响本设计。

## 11. 非目标(显式)

- ❌ 固定间隔 / cron / "Every" 字段——永不属于 loop;定时值守请用 Scheduled runs。
- ❌ fresh-child 迭代、verifier 判定(那是 best-of-N / goal 的领地)。
- ❌ Scheduled 页新建 loop。

## 12. 验收(概念级,批准后细化)

真机:`/loop` 一个真实多轮任务 → 每轮 chips 可见 agent 的 delay+reason;
等外部事(如 CI)时睡合适时长、没事等时立刻继续;干完自行 finish 且此后
不再醒;中途插话改向生效;kill -9 daemon 后按 agent 定的 delay 照醒。
