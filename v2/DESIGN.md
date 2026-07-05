# AgentRunner v2 — 设计

一个 coding agent runtime，目标能力对标 Claude Code / Codex。这是一次
**统筹全局的重写**：不从任何单一功能点（v1 从 actor+persistence）出发，
而是先确立"一个 agent runtime 的本分是什么"，让日常核心功能从**一个**
中心模型自然成立，再把 durability / 安全 / 驱动 / 云作为服务这个模型的
机制挂上去。

配套文件：`CORE.md`（核心清单与 v1 现状对照）。v1 的 DESIGN.md/GAPS.md/
JOURNEYS.md 是本设计的需求来源。

---

## 0. 本分（这是全文的锚）

> **一个 agent runtime 的本分：在一个长期存在的会话里，可靠地协调
> 三方——用户、模型、并发的工作（工具与子 agent）——任何一方随时可以
> 说话，会话据此持续推进，直到用户离开。**

一切设计从这句话推导。任何机制若不服务于它，降级为扩展层。

v1 的失败诊断：它把"本分"默认成了"把一个 task 跑到完成"，于是多轮
交互、并发编排、随时插话这些**日常动作**变成了要额外打补丁的边缘特性，
补丁之间不自洽，基本功能不 work。v2 把"持续的多方协调"放进内核。

---

## 1. 中心模型：Session 是一个持续消费 inbox 的 actor

整个 runtime 只有一种活的东西：**Session**。

```
Session:
  - id
  - inbox   : 一个持久、有序的输入队列（所有"说话"都进这里）
  - journal : 一个 append-only event log（这个 session 发生的一切）
  - state   : journal 的纯 fold（唯一工作内存）
  - loop    : 见下
```

**唯一的循环**（这是整个产品的心脏，务必看懂这 8 行）：

```
loop forever:
  drain inbox → journal 每条输入为 event（journal-inputs-first）
  s = fold(journal)
  if s 有未处理输入 or 有已就绪待处理的完成回执:
      run ONE turn:                    # 一个 turn = 一次模型调用 + 其工具
          assemble(s) → call model → 得到 assistant 消息(可能带 tool calls)
          执行 tool calls（前台并发；后台的只是启动，拿 handle 就返回）
          journal 全过程
  else:
      park：阻塞等 inbox 来新东西（可能等几秒，也可能等几天）
```

**这个循环直接给出核心十项里的九项**：

- **多次输入 / 续聊**（核心 1）：turn 跑完，循环回到顶部；没有新输入就
  park，有就再跑一个 turn。"答完 → 待命 → 再说话"是循环的默认行为，
  **不是**一个额外状态机。run 这个概念被消解——session 就是会话本身。
- **忙时投递排队**（核心 2）：inbox 是持久队列，写入与消费解耦。turn
  在飞时到达的输入静静排队，下一次 drain（turn 边界）被看见。天然的
  安全边界、天然的顺序、天然不打断。
- **回复激活新 turn**（核心 5）：一个工作（子 agent / 后台 bash）完成，
  它的完成回执**就是投进本 session inbox 的一条输入**。于是"子 agent
  回来"和"用户说话"是同一件事——都让循环起一个新 turn。先回来的先进
  inbox 先处理，不等全体。
- **子 agent = 递归的 Session**（核心 3/4/5）：启动子 agent = 创建一个
  子 Session + 把它挂到父 session 的"在飞工作"集合，**立即拿到 handle
  返回**（非阻塞）。子 session 有自己的 inbox/journal/loop。它结束时，
  向父 inbox 投一条完成输入。杀死 = 向子 session inbox 投一条 cancel
  输入（或直接 cancel 它的执行 ctx）——它会 park→收尾→给父投一条
  "被取消"的完成回执。**父子用同一套 inbox 机制通信**，没有第二套。
- **消息改变编排**（核心 6）：用户 steer 消息进父 inbox → 下个 turn
  模型看到它 → 模型自己决定发 `cancel_child(h2)` + `spawn_child(...)`
  工具调用。编排的智能在模型，runtime 只提供"随时能投、随时能杀、
  随时能起"的原语。
- **interrupt 与输入分立**（核心 7）：输入进 inbox（追加语义，不打断）；
  interrupt 是一个**带外信号**（不进 inbox），直接 cancel 当前 turn 的
  活动 ctx，把部分输出收尾成 journal，然后循环回顶——通常此时 inbox 里
  正躺着用户那条"改方向"的消息。两个通道，两种语义，同一个交汇点
  （turn 边界）。

**只剩多模态输入（核心 8）、前台工具（核心 9）、恢复（核心 10）不是
循环的直接推论**——它们是循环里"输入的形态"、"turn 里做什么"、"循环
怎么冷启动"，分别见 §4 / §5 / §6。

这一节是全文最重要的。**如果一个功能不能表达成"往某个 inbox 投一条
输入"或"在 turn 里做一件事"，先怀疑是不是设计错了。**

---

## 2. Inbox：统一输入投递（v1 缺口 G3/G6/G14 的共同答案）

inbox 是 v2 的关键新原语。v1 的病根是**没有输入通道**——只有 run 启动
时的一条 task。v2 把"任何一方对 session 说话"统一成"往 inbox 投一条
`Input`"。

**Input 的种类**（一个 tagged union，都 journal 为 `InputReceived`）：

| 种类 | 来源 | 例子 |
|---|---|---|
| `user_message` | 人 | 文本 + 附件（图片/长贴），终端或 web |
| `child_result` | 子 session | 完成/失败/被取消 的回执 + 产出摘要 |
| `tool_result` | 后台工具 | 后台 bash 的终态（复用同一路径）|
| `timer` | runtime | 定时/超时到期 |
| `control` | 人/系统 | cancel、pause（interrupt 是带外信号，不走这里）|

**三条铁律**：

1. **投递与消费解耦**。投递方（终端、web、子 session、timer、webhook）
   只管 append 到持久 inbox；消费方（loop）在 turn 边界 drain。发送方
   从不阻塞在"agent 现在忙不忙"上。
2. **journal-inputs-first**（承自 v1，唯一保留的输入纪律）：一条输入先
   落 journal 成 `InputReceived` event，再被 fold 看见、被 turn 消费。
   崩溃不丢输入。
3. **有序 + 至少一次**。inbox 是 per-session 的有序队列；投递去重靠
   幂等 id（发送方给，或 runtime 按内容+来源生成）。

**这一个原语关掉 v1 的三个高影响缺口**：G3（steering=投 user_message）、
G6（续聊=turn 后 park 等 inbox）、G14（外部事件=webhook 往既有 session
的 inbox 投 user_message，和人投的是同一种）。三者本就是"输入投递"的
三个发送方，v1 当成三个问题，v2 是一个。

---

## 3. 子 Agent：递归的 Session（核心 3/4/5/6）

**没有"子 agent"这个独立概念——子 agent 就是一个 parent 指针非空的
Session。** 这是 v2 相对 v1 的最大简化：v1 有 spawn 阻塞路径、后台
task 路径、driver 的 child 路径三套各自为政的"子执行"，v2 只有一套。

**生命周期全部是 inbox 动作**：

- **启动**（非阻塞）：父 turn 里模型调 `spawn_child{agent, task, budget}`
  工具 → runtime 创建子 Session（自己的 dir、inbox、journal，预算从父
  树预算切一块）→ 向子 inbox 投第一条 `user_message`（= task）→ 父侧
  journal 一条 `ChildSpawned{handle, child_id}` → **工具立即返回 handle**
  → 父 turn 继续（可以再 spawn、可以读码、可以结束 turn 去 park）。
- **并行**：N 个子 session 各自在自己的 loop 里跑，互不阻塞。父不"等"
  任何一个。
- **完成 → 激活父**：子 session 走到它的待命点且被标记为"一次性任务"
  （见下）时，向**父 inbox** 投一条 `child_result`。父 loop 下个 turn
  drain 到它 → 模型看到"h2 回来了，结论是……" → 起 turn 反应。先完成
  的先投先处理。
- **杀死**（核心 4）：父 turn 里模型调 `cancel_child{handle}`，或用户
  投一条 `control{cancel, handle}` → runtime cancel 子 session 的执行
  ctx → 子把在飞活动收尾（进程组确认退出、部分输出留存）→ 子向父 inbox
  投 `child_result{canceled, partial}`。**杀死不是特例，是给子 session
  投了一条 control 输入。**
- **改变编排**（核心 6）：steer → 模型在下个 turn 同时发
  `cancel_child{h2}` 和 `spawn_child{迁移文档}`。runtime 不需要懂
  "重定向"，它只提供杀和起。

**一次性任务 vs 持续会话**：子 session 默认是"一次性任务"——完成即向父
投回执并进入终态。但因为它就是个 Session，它**也可以**是持续的（父保持
它的 handle，多次投 `user_message` 复用它）。v2 不为这两者建两套东西，
只是子 session 的一个 flag：`report_to_parent_on_idle`。

**预算与权限沿树下切**（承自 v1，已验证）：树预算 reserve-at-spawn /
settle-at-child-idle；子权限只能等于或窄于父（mode 不放宽）。这部分
v1 设计正确，直接搬。

**父崩溃**：子 session 有独立 journal，父恢复时对每个"在飞 handle"
检查子 journal——子已终态则从子 fold 结算并合成一条 child_result 投回
父 inbox；子还在跑则重新挂接。承自 v1 的 settle-from-child-fold 纪律
（driver 已验证），推广到通用子 session。

---

## 4. Turn 与消息：模型看到什么、多模态（核心 8）

**Turn** = 一次模型调用 + 该调用产生的工具执行，是 journal 里的原子
推进单元，也是**唯一的中断/快照/审批边界**（承自 v1，正确）。

**消息模型**（修正 v1 G1）：一条消息由 parts 组成，part 种类：
`text` / `tool_call` / `tool_result` / **`image`** / **`file`**。
- 图片/文件的字节走 **CAS**（content-addressed blob store，承自 v1 的
  ArtifactStore）：journal 与 fold 只存 `ref + media_type`（blob 先于
  引用它的 event 落盘——承自 v1 blob-before-event），组装请求时才从 CAS
  inflate 成 wire 字节。fold 永不读 store 的纪律不破。
- 长粘贴文本：超阈值自动转成 `file` part（folded 显示为摘要+ref），
  不撑爆上下文。
- provider 适配层把 part 映射到各家 wire（Anthropic image block /
  Gemini inline_data）——一个薄适配，不是核心。

**上下文组装**（承自 v1，正确的部分）：固定顺序 assemble（frozen 前缀
→ 记忆/skills → 对话 → mode 后缀），保 prompt cache 稳定；compaction
在阈值触发折叠早期历史（v2 补手动触发 = 投一条 `control{compact,指示}`
——又是 inbox）。

---

## 5. Turn 里做什么：Effect Pipeline（承自 v1，重新定位）

turn 里每个副作用（模型调用、工具调用、spawn、发布 artifact）都是一个
**Effect**，流经同一条判定管线：`floor(硬红线) → hooks → permission →
budget → execute`。判定与执行按持久化时点拆分落盘（`EffectRequested`
→ 关卡 → `EffectResolved` → `ActivityStarted` → 终态）。

这套是 v1 最扎实的资产之一，**设计正确，直接保留**。v2 只做一件事：
明确它是"turn 内机制"，服务于 §1 的循环，不是设计的出发点。

- 前台工具：并发执行，全部 journal，turn 等它们完成。
- 后台工具（bash background / spawn_child）：只启动、拿 handle、立即
  返回；终态以 inbox 输入回来（§2）。
- 权限/审批/预算/hooks/redaction/in-doubt：全部承自 v1（这些的语义
  v1 写得对，实现也大多在）。**审批 = 一个 Effect 进 WAITING 并往需要
  应答的通道要答复**；应答（人或配置）作为一条输入回来。

**核心工具集**（v2 必须自带、必须 work，不能借 bash）：
`read_file` / `write_file` / `edit_file` / `bash`(前台+后台) /
`spawn_child` / `cancel_child` / `ask_user`(wait-class，向用户提问=park
等 inbox 里的 user_message) / `finish`(结束当前 turn 让 session 待命)。
v1 缺 write_file、cancel_child、ask_user 的应答路径——v2 视为核心工具，
一等实现。

---

## 6. 持久化与恢复：journal + fold + snapshot（承自 v1，重新定位）

- **journal 是唯一真相**：每个 session 一个 append-only event log。
  `state = fold(journal)`，fold 是纯函数、不读时钟、不执行副作用。
- **snapshot 是可弃缓存**：turn 边界给 state 打对话快照，加速恢复；
  丢了只损失 fold 时间。
- **恢复（核心 10）**：进程重启 → 对每个 session 读 journal + fold →
  - 空闲 session（上次 turn 后在 park）：直接回到 loop 顶部等 inbox。
    **续聊天然恢复**——因为待命就是循环的常态，不是特殊状态。
  - turn 中途崩溃：承自 v1 的 in-doubt 纪律（有 Started 无终态 → 按
    工具类别处置：LLM 重发、只读重跑、写/执行不重跑渲染
    `[interrupted by crash]`、非幂等绝不静默重跑）。
  - 在飞子 session：§3 的 settle-from-child-fold。
- **workspace 快照 / fork / rewind**：一等状态走 SnapshotStore（shadow
  repo backend），只在显式 barrier 打点。**这是扩展层**——核心十项不
  依赖它，冻结到核心绿灯之后。v1 这块设计与实现都在，不丢，但不在
  当前战线。

v2 与 v1 在持久化上的唯一实质差别：v1 的恢复围绕"未完成的 run"设计，
v2 的恢复围绕"长期存在的 session"设计——空闲续聊是一等公民，不是
"已完成、无事可做"。

---

## 7. 分层（对照 v1，看清什么被移动了）

```
┌─────────────────────────────────────────────────────────┐
│ 交互面   终端 / web / webhook —— 都只是 inbox 的投递方   │  ← v2 提升为一等
│          + 输出订阅（turn/delta/工具判定，ephemeral）    │
├─────────────────────────────────────────────────────────┤
│ 会话内核 Session actor · inbox · loop · turn · 子 session│  ← v2 的新中心
│          （§1–§3，核心十项住在这里）                     │
├─────────────────────────────────────────────────────────┤
│ Turn 机制 上下文组装 · effect pipeline(关卡) · 工具       │  ← 承自 v1
├─────────────────────────────────────────────────────────┤
│ 持久化   journal · fold · snapshot · CAS · in-doubt      │  ← 承自 v1
├─────────────────────────────────────────────────────────┤
│ 扩展层   workspace 快照/fork · goal/loop/best-of-N 驱动  │  ← 全部冻结
│          · 云环境 · Git 一等化 · 索引 · MCP · 通知       │     到核心绿灯后
└─────────────────────────────────────────────────────────┘
```

v1 的分层把"交互协议"塞在最上层 surfaces 里当薄壳，把 actor/durability
放在底层当地基——于是交互能力是"从地基往上够"，够不到。v2 把**会话
内核**放在正中，交互面和持久化都是它的两翼。actor model 依然在（session
就是 actor），但它是内核的实现手段，不是设计的起点。

---

## 8. 一个完整例子：走通 §11 的 C7（把中心模型跑给自己看）

用户开一个 session，跑一个多 agent + steer + 续聊的完整流程，看每一步
都是"inbox 投递 + turn 推进"：

```
1. 用户投 user_message("修这个 bug", +截图)      → inbox
   loop drain → turn1: 模型看图读码，发 3 个 spawn_child → 拿 h1/h2/h3
   → 3 个子 session 起跑 → turn1 结束 → park
2. h1(复现) 先跑完 → 向父 inbox 投 child_result(h1)
   loop drain → turn2: 模型"复现成功，继续等其它" → park
3. 用户投 user_message("别查依赖了，看迁移文档")  → inbox
   loop drain → turn3: 模型发 cancel_child(h2) + spawn_child(迁移)→h4
   → h2 收到 control→收尾→投 child_result(h2, canceled)
   → h4 起跑 → turn3 结束 → park
4. h3、h4 陆续完成 → 各投一条 child_result → 各激活一个 turn
5. 全部回来后某个 turn 模型调 finish → session 待命
6. 用户过一小时投 user_message("为什么这么修?")   → inbox
   loop drain → turn: 基于全部上下文作答 → park
7. 进程重启 → session fold → 在 park → 直接回 loop 顶等 inbox（续聊无缝）
```

全程没有一个"特殊状态机"：多输入、并行、杀死、回灌、续聊、恢复，
都是同一个循环消费同一个 inbox。**这就是 v2 要证明的东西。**

---

## 9. 保留 / 移动 / 新增（v1 → v2 账本）

**原样保留（设计正确，实现大多在）**：journal + 纯 fold、
journal-inputs-first、effect pipeline 四关卡与拆分落盘、CAS +
blob-before-event、redaction + 凭据红线、in-doubt 按类别处置、
turn 边界快照、权限 rules(path/command/network) + mode 阶梯、
树预算 reserve/settle、网络沙箱、workspace 快照/fork/rewind（降为
扩展层）、MCP/skills/memory 读侧、observability(inspect/事件链)。

**重新定位（从"设计出发点"降为"内核的服务机制"）**：actor model
（session 是唯一 actor 类型）、durability（围绕长期 session 而非
未完成 run）、driver（goal/loop/best-of-N 变成"一种特殊的、由程序而非
人投递 inbox 的父 session"——统一进 §3，不再是独立子系统）。

**新增（v2 的核心，v1 缺失）**：
- **inbox 原语**（§2）——统一输入投递，关掉 G3/G6/G14。
- **session-as-conversation 循环**（§1）——续聊是常态，关掉 G6。
- **子 agent = 递归 session**（§3）——一套机制取代 v1 三套，关掉 G2。
- **消息 parts 含 image/file**（§4）——多模态，关掉 G1。
- **核心工具一等化**：write_file / cancel_child / ask_user 应答路径
  （G18/G20）。

**冻结到核心绿灯之后（不删，不做）**：云 workspace 生命周期、Git/PR
一等化、best-of-N 晋升、语义索引、IDE、通知渠道、定时任务跨重启唤醒、
审批规则写回、记忆写回。

### 9.1 M3 实现状态注记（M3 出口 review 后追加，诚实对照）

本文档描述目标形态；fix-in-place 的 M1–M3 实现（MIGRATION.md）与
字面表述有以下已知偏差，记录在此、不做静默漂移：

- **工具名对照**：`spawn_child` → 实现名 `spawn_agent`；
  `cancel_child` → 实现名 `task_kill`（handle 即 task_id，与 bash
  后台任务共用取消原语，命名决策见 PROGRESS M3.1）。`ask_user` /
  `finish` 未实现（收口记档：idle park 本身就是"待命"，两者的
  增量价值待真实使用反馈，不预做）；`write_file` 已于 M4.3 一等化。
- **§2 inbox 字面统一度**：`user_message` 与 `control{kill}` 已按
  字面 journal 为 `InputReceived`（后者 source=control，不进对话）。
  `child_result`/`tool_result` 语义上是 inbox 输入，机制上暂由承自
  v1 的 background activity（`ActivityStarted{Background}`→终态，
  fold 渲染 user-role 消息）兑现——语义等价、事件形状不同；字面
  统一与否列收口决策。
- **§3 "一套机制取代三套"**：M3 阶段尚未收敛——阻塞 spawn、后台
  spawn、driver 子系统并存（阻塞路径与 driver 保留 v1 兼容）；
  收敛记收口任务。
- **§1 interrupt 语义**：实现新增"idle 处 interrupt = close 会话"
  （交互惯例）；turn 中 interrupt 仍是 steer（core-7 原文只定义
  后者）。

---

## 10. 非目标（原型阶段，承自 v1）

分布式多节点执行、向后兼容、多租户/团队共享会话、确定性 code replay、
生产级鉴权配额、语音输入。

---

## 11. 核心验收场景（C1–C10）——绿灯前不碰扩展层

这是 v2 的**完成定义**。每条是可执行的端到端场景（scripted 模型 +
routing provider，见 §13）。**全绿之前，扩展层一行不写。**

- **C1 多输入续聊**：一个 session，用户发 3 条消息（间隔 park），每条
  起一个 turn，同一上下文延续，session 从不 end。
- **C2 忙时排队**：turn 在飞时投 2 条消息 → 按序在后续 turn 边界消费，
  不丢不乱序，不打断在跑的活动。
- **C3 并行 spawn**：一个 turn 发 3 个 spawn_child → 3 个子 session
  并行跑 → 父 turn 不阻塞（拿 handle 即结束 turn）。
- **C4 子完成激活父**：子 session 完成 → child_result 进父 inbox →
  父起新 turn 看到结论；先完成先处理。
- **C5 杀子 agent**：cancel_child(handle) → 子收尾（部分输出留存）→
  投 canceled 回执 → 父 turn 可见；被杀的子不再影响后续。
- **C6 steer 改编排**：park 中投 user_message → 下个 turn 模型
  cancel 一个子 + spawn 一个新子 → 走通 C5+C3 组合。
- **C7 完整编排**：§8 的七步全程（多输入+并行+杀+回灌+续聊+恢复）
  端到端绿。
- **C8 interrupt vs 输入**：Esc 打断在跑 bash（部分输出留存）与投
  user_message（排队不打断）两条路径互不干扰。
- **C9 多模态**：user_message 带图片 → CAS 存 ref → 组装时 inflate →
  模型看到图；长贴自动转 file part。
- **C10 恢复**：(a) 空闲 session 进程重启后续聊无缝；(b) turn 中途崩溃
  按 in-doubt 恢复；(c) 在飞子 session 按 settle-from-child-fold 恢复。

---

## 12. 实施纪律（避免重蹈 v1 覆辙）

1. **核心优先，扩展冻结**：C1–C10 全绿之前，§7 扩展层一行不写、一句
   不讨论。这是硬约束。
2. **每个 C 场景先写验收测试，再写实现**（红→绿）。测试是 §11 的
   scripted 端到端，不是单元测试凑数。
3. **能表达成 inbox 投递的，不要发明新机制**（§1 的自检）。每加一个
   概念，问："它能不能是一条 Input 或一个 turn 内动作？"
4. **v1 资产按 §9 账本迁移**，不重写已验证正确的部分（fold/pipeline/
   CAS/redaction/in-doubt）；重写的是内核（inbox/loop/子 session）。
5. **一个能力没有对应的绿灯 C 场景（或扩展层场景），就不算完成**——
   不接受"设计了但不 work"。

---

## 13. 测试基建（v1 缺口 G4，v2 前置）

- **scripted provider**：按脚本产出模型响应，离线确定性。
- **routing provider**（v2 新增，C3–C7 前置）：按子 session 的
  id/task 路由各自的脚本，让并发子 agent 的响应确定、可复现。
- **fifo/barrier 编排**：测试侧控制子 session 完成顺序，复现"先回来
  先处理"、"杀死中途的"等时序。
- **crash 注入**：承自 v1，验 C10。

---

*本设计的成败只由一个标准衡量：C1–C10 是否全部真实走通。设计得多漂亮
都不算数——v1 的教训就是这个。*
