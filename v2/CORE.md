# v2 — 核心功能清单与现状对照

**为什么有这个文件**：v1 的设计从 actor model + 持久化出发，把交互当成
"表面层"，结果是架构复杂而**日常使用的基本功能不 work**。v2 的纪律
反过来：先定义"什么是核心"，核心不绿灯，不讨论任何复杂功能。

**铁律**：下面的核心场景（C1–C10，正式定义见 DESIGN.md §11）全部
通过之前，不设计、不实现、不讨论扩展层的任何东西（云端、Git 一等化、
IDE、best-of-N、索引……全部冻结）。

---

## 一、最核心的功能（agent runtime 的本分）

一个 agent runtime 的本分是：**在一个长期存在的会话里，可靠地协调
用户、模型、和并发的工作（工具/子 agent），任何一方随时可以说话。**

具体是十件事：

1. **一个 session 里多次用户输入**。agent 答完不是结束，是回到
   待命；用户随时再说话，同一上下文继续。这是默认形态，不是特例。
2. **输入随时可投**。agent 忙的时候用户照样发消息——排队，在下一个
   安全边界生效；不丢、不乱序、不打断正在做的事（除非明确 interrupt）。
3. **一次启动多个子 agent，并行跑**。启动是非阻塞的：拿到 handle，
   主 agent 继续干自己的事。
4. **子 agent 可被杀死**。用户或主 agent 都能凭 handle 取消一个
   在跑的子 agent；部分产出留存，取消事实对模型可见。
5. **子 agent 的回复激活新 turn**。每个子 agent 完成时，结果作为
   输入回灌主 agent，触发新一轮思考——先回来的先处理，不等全体。
6. **用户消息可以改变编排**。"别查依赖了，去看迁移文档" → 主 agent
   据此杀掉一个子 agent、新起一个。
7. **interrupt 与输入分立**。Esc 打断当前活动（部分输出保留），
   发消息追加指令——两个手势，语义分明。
8. **多模态输入**。至少图片；粘贴长文本按附件折叠。
9. **前台工具闭环**。读/写/编辑文件 + bash，改了就能验证。
10. **会话可恢复**。进程重启后 session 还在：空闲的直接续聊，
    有在跑工作的按记录恢复或结算。

**明确不在核心里**（有价值，但都排在核心绿灯之后）：云端环境、
Git/PR 一等化、goal/loop/best-of-N 驱动、语义索引、IDE、通知、
定时任务。v1 在这些上花的功夫不浪费（见 DESIGN §12），但顺序错了。

---

## 二、现状对照（v1 设计 + 实现）

| # | 核心功能 | v1 现状 | 缺口 |
|---|---|---|---|
| 1 | 多次输入续聊 | ❌ **不存在**。run = task-to-completion，答完即 run_ended | GAPS G6 |
| 2 | 忙时投递排队 | ❌ 无输入通道；只有启动时一条 task | GAPS G3 |
| 3 | 并行子 agent | ❌ spawn 是阻塞的；"background:true" 只有一句承诺 | GAPS G2 |
| 4 | 杀死子 agent | ❌ task_kill 只对后台 bash 有效，而子 agent 进不了后台 | GAPS G2 |
| 5 | 回复激活新 turn | 🟡 机制存在（bash 任务的 outcome 回灌已通），子 agent 侧缺 | GAPS G2 |
| 6 | 消息改变编排 | ❌ 依赖 2+3+4，全卡 | G2+G3 |
| 7 | interrupt/输入分立 | 🟡 interrupt ✅（含部分输出）；输入侧缺 | GAPS G3 |
| 8 | 多模态输入 | ❌ 消息模型只有 text | GAPS G1 |
| 9 | 前台工具闭环 | ✅ 读/编辑/bash 完整；**write_file 缺**（建文件借 bash） | GAPS G18 |
| 10 | 会话可恢复 | 🟡 未完成 run 的 crash-resume 很扎实；但"续聊的空闲 session"这个形态本身不存在 | G6 连带 |

**结论**：十项核心里 1 项完整、3 项部分、6 项缺失或卡死。v1 最强的
恰恰是外围（durability、驱动、安全），最弱的是本分（交互内核）——
这就是"架构复杂但基本功能不 work"的病灶。

---

## 三、v2 怎么治

v2/DESIGN.md 用一个统摄性的模型让核心十项**自然成立**，而不是十个
补丁：**session = 长期会话 actor，一切输入进同一个 inbox，调度器在
"inbox 非空且无 turn 在飞"时起 turn；子 agent 就是 session（递归），
其完成是父 inbox 的一条输入**。多输入、排队、并行子 agent、杀死、
回复激活——全部是这一个模型的直接推论。

v1 已被验证的资产（journal-inputs-first、fold、effect 关卡、CAS、
redaction、in-doubt 纪律、快照/fork）全部保留——但重新定位为
**服务核心循环的机制**，不再是设计的出发点。

---

## 四、v2 收口对照（2026-07-05）——十项核心全部达成

| # | 核心功能 | v2 状态 | 闸门 |
|---|---|---|---|
| 1 | 多次输入续聊 | ✅ Conversational park + new/send/close | QA-01 |
| 2 | 忙时投递排队 | ✅ durable mailbox 确认即持久 + type-ahead,边界生效,不打断 | QA-02, QA-08(不丢) |
| 3 | 并行子 agent | ✅ spawn background 非阻塞,handle 立即配对 | QA-04 |
| 4 | 杀死子 agent | ✅ ar kill(QA-05 实测)/ task_kill(QA-09 实测+孪生),部分产出 best-effort | QA-05, QA-09 |
| 5 | 回复激活新 turn | ✅ 回执唤醒 park;先回先处理由孪生确定性背书 | QA-04 + scripted 孪生 |
| 6 | 消息改变编排 | ✅ steer → 模型 task_kill + spawn | QA-09(scripted 孪生 C6) |
| 7 | interrupt/输入分立 | ✅ 两通道两手势,互不串扰 | QA-06 |
| 8 | 多模态输入 | ✅ 图片 CAS ref + 长贴折叠 file part | QA-07 |
| 9 | 前台工具闭环 | ✅ read/edit/bash + write_file 一等化 | QA-03 |
| 10 | 会话可恢复 | ✅ 三态 crash 矩阵,send 即复活,排队输入不丢 | QA-08 |

**C7 压轴串联(QA-09)**：一个真实 session 里 图片输入 → 3 并行子
agent → 先回先处理 → steer 杀一换一 → 全回执 → 续聊 → kill -9 →
复活续聊 + write_file 落盘,PASS。

铁律解除条件已满足;扩展层(云端/Git/best-of-N/索引/IDE/通知/定时)
按 GAPS.md 余项另行排期。
