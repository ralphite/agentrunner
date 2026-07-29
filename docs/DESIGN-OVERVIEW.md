# An Actor-Model, Event-Sourced Agent Runtime and Harness

**AgentRunner — 高层设计（3 页）**

> `docs/DESIGN.md`（架构 source of truth）的高层浓缩：只讲 runtime 内核
> ——会话模型、输入通道、多 agent、turn 内机制、持久化、provider。扩展层
> （workspace 快照 / fork / 索引 / 生态接入 / surfaces）刻意略去。以 DESIGN.md 为准。

## 1. 本分

> **在一个长期存在的会话里，可靠地协调三方——用户、模型、并发的工作（工具
> 与子 agent）——任何一方随时可以说话，会话据此持续推进，直到用户离开。**

全部设计从这句话推导；不服务于它的机制降级为扩展层。**多轮交互、并发编排、
随时插话是日常动作，不是边缘特性**——它们必须是中心模型的直接推论，而不是
补丁；durability、effect 管线、安全都是服务这个内核的机制。

**四条原则**：一切可运行的是 actor；一切历史皆 event（state = journal 的纯
fold）；一切副作用是 activity，流经同一条 effect pipeline；一切行为由数据
定义（含 tool 定义本身，core 里不硬编码任何具体 agent）。

**明确的非目标**：确定性 code replay、整树确定性 replay、分布式执行、生产级
多租户。是取舍，不是遗漏（§5）。

## 2. 中心模型：Session = journal + 待命

整个 runtime 只有**一种活的东西**：Session = `id` + `inbox`（持久有序队列，
所有"说话"都进这里）+ `journal`（append-only event log）+ `state`（journal
的纯 fold，唯一工作内存）。它没有自己的循环，一生只有两句话：**平时待命**
（装着全部历史等下一条输入，等几秒或几天成本相同）；**每条输入触发一个 turn**：

```
输入到达（先落 journal，再被消费）
  → 跑 ONE turn（= 一遍 agentic loop）:
      loop:
        assemble(fold(journal)) → 调模型      # 一个 generation step
        有 tool call → 执行（前台并发；后台只启动，拿 handle 即返回）
                     → 回 loop 顶
                       # ← 安全边界：排队的插话 / 回执在此进入
        无 tool call → 这是 final generation，turn 结束
  → 回到待命
```

同一 session 同时只跑一个 turn，忙时到达的输入排队。**这就是全部执行模型**
——没有"run"概念、没有第二种运行形态、没有额外状态机。续聊、忙时排队、工作
完成激活新 turn、子 agent、中途改编排，都是这一个循环的推论。

**静止（quiescence）**是唯一的"结束"：最后一个 turn 收尾、无在飞工作、无未到
期的自触发——由 journal **形状**自明，不是事件、不是状态机。静止时跑一串固定
动作（publish outputs → 切 barrier → 有 parent 则投回执），任何"结束时要做的
事"挂进这个序列。静止可发生多次。

**自检**：加功能前先问——"它能不能是一条 Input，或一个 turn 内动作？"都不能，
先怀疑设计错了。

## 3. Inbox：一条通道，多种发送方

"任何一方对 session 说话"统一成"往 inbox 投一条 Input"——用户、子 agent、
timer、外部事件是同一个问题的四个发送方，不是四套机制。**Input 是弱类型的**：
对话面上就是纯内容 + 来源前缀，来源（user / agent / machine / timer / control）
只是 journal 元数据，模型不该看到类型系统。

**三条铁律**：投递与消费解耦（发送方从不阻塞在"agent 忙不忙"上）；
journal-inputs-first（先 fsync 进 CommandLog 再回执，崩溃不丢输入）；有序 +
幂等（输入 / interrupt / 审批 / kill 共用同一条 durable command 通道，稳定
`command_id` 保证重试不双执行）。

**两个正交的时机维度**，都锚在安全边界、都不打断执行中的 step：投递模式
`queue | steer`——前者进下个 turn，后者在安全边界以新 user 消息进对话、模型本
turn 内下个 generation 就看到，**仍是追加不是打断**；**interrupt 与输入分立**
——它先成为 durable command，再带外 cancel 当前 turn 的活动 ctx，把部分输出收尾
进 journal。安全推论：**权限判定永远看 principal/trust，绝不看内容措辞**，机器
来源恒记 untrusted，不能经由"诱导模型"拿到高于来源级别的权限。

## 4. 子 Agent：递归的 Session

**没有"子 agent"这个独立概念——它就是 parent 指针非空的 Session。** 生命周期
全是 inbox 动作，父子之间没有第二套通信机制：`spawn_agent` 创建子 session 并
**立即返回 handle**（非阻塞，父可以继续 spawn 或直接结束 turn 回待命）；子静止
时向**父 inbox** 投回执，父在安全边界看到并起新 turn——先完成的先处理，不等
全体；`kill{handle}` = 给子投一条 control 输入；父崩溃时对每个在飞 handle 查子
journal，已静止则从子 fold 结算，还在跑则重新挂接。

一条**模型可见面的契约要求**：`spawn_agent` / `output` 的描述必须显式声明
fire-and-yield（派完可结束 turn、完成会作为消息自动唤醒、无需轮询）。否则弱
模型会用 `output` 轮询 + `bash sleep` 自旋，把自动唤醒路径整个架空（实测同任务
轮询 13 → 0）。**编排的智能在模型，runtime 只提供"随时能投、能杀、能起"的原语。**

**树级约束**是真正的防线：审批沿 correlation id 冒泡到人——审批的永远是人，不是
parent agent；**权限继承拆两条规则**（mode 没有交集运算）——rules 做真交集并在
spawn 时**冻结**成不可变数据（子无法自行放宽，父事后跃迁不回溯），mode 不交集
但工具面先过冻结 rules；**树预算** = min(子限额, 父剩余)，reserve-at-spawn /
settle-at-child-idle，深度与扇出有数据化上限（spec 允许成环，上限是唯一防线）；
**子的意义在上下文隔离**——子烧自己的 window，只有符合 contract 的报告回流父。

## 5. Turn 内机制

**Context assembly 是一等组件**（`fold(journal) → 请求`）：system prompt 拼装
顺序固定，其中 tool/skill/**子 agent 目录**的注入是 multi-agent 可用的前提
——模型不知道 `summarizer` 存在就永远不会 spawn 它。**prefix 稳定是显式不变量**
（prompt caching 约 10x，没有它 agent loop 在经济上不可用）：环境变化一律**以
追加消息进入上下文，绝不改写 prefix**；工具面分两级——mode 过滤只作用于关卡侧
的 permitted 面，进 prefix 的 advertised 面 session 内稳定。压缩同理：journal
留全量结果（truth），**只有装配视图降级**，故 resume/rewind 语义天然良定义。

**Effect Pipeline**：每个副作用（模型调用、工具、spawn、发布）都是一个 Effect，
流经同一条管线——hooks、permission、审批、预算不是四个子系统：

```
effect → [1] Floor      硬底线（逃逸 / 凭据 / plan 模式）：纯判定，直接 deny
         [2] Spawn      结构限制：树深度、扇出、handoff 唯一性
         [3] Hooks pre  observe + block（不改写）
         [4] Permission allow / ask / deny —— policy 是数据
         [5] Budget     reserve-then-settle
         [6] Execute    以 activity 执行
         [7] Hooks post
```

- **纯判定的关卡排最前**，使必拒的 effect 绝不触发有副作用的 pre-hook。
- **判定落在记录边界之内**：结论在执行**之前**落盘（ask 路径先落
  `ApprovalRequested` 并携带已完成的关卡判定——pre-hook 可能已有副作用，这个
  事实必须先于可能挂几天的审批落盘）。恢复时读记录值，**不重跑 hook**。
- **预算 reserve-then-settle**：否则 N 个并行 call 对着同一个过期计数器放行，
  合起来超支 N 倍。
- **每种关卡结果都定义"模型看到什么"**：deny / block / 拒批 / 失败一律渲染成
  error tool result，**loop 继续**；只有 session 级预算超限才优雅收尾。给模型
  的错误与给用户的错误是两个 surface。
- **边界诚实**：path 规则只约束文件类 tool（一条 `sed -i` 就能改写 `src/**`），
  真正的路径边界由**强制 OS sandbox** 闭环，缺席时 fail closed、不降级裸跑。

**执行纪律**：并行 tool call 是常态（ask 挂起不阻塞已放行的 call）；token delta
只走 bus（显式 ephemeral），持久化的是组装完成的消息；后台 effect 的立即配对
结果就是 `{handle, running}`，完成时的终态兼任 pending input、在安全边界作为新
user 消息进对话；interrupt 触发 sweep 使所有未终态 call 得终态、未决审批作废且
迟到应答 no-op（否则 resume 后一条迟到的批准会执行用户已用 Esc 放弃的调用）。

## 6. 持久化与恢复

**最重要的取舍：不做确定性 code replay。** Temporal 式 replay 需要稳定 activity
id、确定性协程调度、divergence 检测——数周级的引擎项目；而 agent loop 的全部
状态不过是（消息列表、step 计数、待处理 tool call）。用三件更便宜的东西拿到同样
的用户可见能力：**外部输入 durable accepted**（先留事实再消费，崩溃时从 command
receipt 与 journal fact 的差集恢复）；**state 是纯 fold**（apply 不读时钟、不执行
副作用）；**snapshot-resume**（安全边界打 snapshot，resume 只读 `seq > N`；
snapshot 是可弃缓存，形状可疑就丢掉走全量 fold）。

**挂起是显式状态，不是任意点挂起**：等待种类只有 `WAITING_INPUT` 与
`WAITING_APPROVAL`，都发生在安全边界。等几分钟或几天成本相同，进程死了也一样
——**durable 的等待不需要 replay 引擎**。

**Activity**：`Started` 先落盘 → 执行 → `Completed/Failed`；结果过凭据
redaction；取消以**进程组**为准（确认组内全退才落 `Cancelled`，否则 `npm
install` 的孤儿会在"取消"后继续写盘）；timeout 走 durable timer，绝不在关卡
代码里读墙钟。**in-doubt 按 tool 类别数据化处置**（崩溃几乎必然砸中 in-flight
activity）：LLM 调用自动重发，read-class 与 `idempotent: true` 重跑，
execute/edit-class **不重跑**、渲染 `[interrupted by crash]` 让 loop 继续。
非幂等操作绝不静默重跑——它们根本不重跑，所以无人值守也不会卡在人工 triage。

**冷启动**：待命的 session 无事可做（待命跨进程存活，下一条 send 即接续）；
turn 中途崩的走 in-doubt 自愈；在飞子走 settle-from-child-fold。**恢复只住在一个
地方**（session resume），actor 崩溃不自动 restart，停在 failed 等人，不热循环。

## 7. 分层与 Provider

```
会话内核  Session actor · inbox · loop · turn · 子 session      ← 中心
Turn 机制 context assembly · effect pipeline · 工具
持久化    journal · fold · snapshot · CAS · in-doubt
扩展层    时间旅行 · goal/loop/best-of-N 驱动 · 生态接入
```

**core 是库**：一切 surface（CLI、headless、daemon、外部事件入口）都只是 inbox
的投递方 + 输出订阅方，是挂在 core 上的薄壳、也都是 actor，不存在"特权
frontend"。kernel 基座只有三件东西：actor（id + mailbox + behavior）、bus（进程
内、**ephemeral**，任何影响结果的输入必须先 journal 再消费）、envelope
（`command_id` 是外部幂等轴、`causation_id` 是 stream 内因果链，**两轴分立**才
使 command 可重试）。

**Provider 是薄接口**（`complete(request) → stream`，streaming 原生）：能力通用
且可选（caching / thinking / tools / structured output 以 provider 无关方式表达，
`capabilities()` 声明支持面，不支持时**明确降级或报错，绝不静默忽略**）；返回
**归一化**（token 计数含 cache read/write、finish reason 含各家独有的异常形态、
tool call、thinking 块统一成一套内部表示，管线与记账不感知 provider）；**opaque
signature 随 event 持久化**并原样回传（丢掉它，某些 provider 的多轮工具调用第二次
请求就 400；推论：mid-run 换 provider 必须在压缩边界重开）。主 Gemini、次
Anthropic——第二个实现的作用是**验证抽象不漏**。

**运行模式**（扩展层）：goal / loop / best-of-N 由 IterationDriver 表达，但会话内
形态不新增状态机——完成裁决与定时唤醒只是**静止序列或安全边界上的一格**，唤醒即
"以 program 源投一条输入"。这是 §2 自检的兑现。

## 8. 可证伪之处

**单进程假设**（bus 是进程内的；跨进程部署要分 ephemeral topic 与 guaranteed
send 两通道，重连方从 event log 对账而非靠 bus 补投）；**不保证整树确定性重现**
（保证的是 per-stream 可审计，不是跨 actor 消息交错的重现）；**软标记不计入安全
预算**——untrusted 框定只降低模型服从注入的概率，真正的缓解是 egress 控制、OS
sandbox、permission floor 这些与模型是否听话无关的硬防线；redaction 同理，是
文档化的残余风险，不是闭合的保证。
