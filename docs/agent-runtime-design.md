# An Actor-Model, Event-Sourced Agent Runtime

**架构设计（3 页）**

> 本文讲一个通用 agent runtime 的内核该长什么样：会话模型、输入通道、
> 多 agent、turn 内机制、持久化与恢复、provider 抽象。目标是能长期驻留、
> 挺过进程死亡、支持多方随时插话的执行环境，而不是一次性的 LLM 调用循环；
> 不预设领域——写码、研究、运维、数据工作共用同一个内核。具体工具集、领域
> 状态的快照与回滚、索引、生态接入、各类 surface 属于扩展层，本文略去。

## 1. 本分

> **在一个长期存在的会话里，可靠地协调三方——用户、模型、并发的工作（工具
> 与子 agent）——任何一方随时可以说话，会话据此持续推进，直到用户离开。**

全部设计从这句话推导；不服务于它的机制降级为扩展层。**多轮交互、并发编排、
随时插话是日常动作，不是边缘特性**——它们必须是中心模型的直接推论，而不是
补丁。一旦把本分默认成"把一次 run 跑到完成"，这些日常动作就会变成互不自洽
的补丁堆；durability、effect 管线、安全都只是服务这个内核的机制。

**四条原则**：一切可运行的是 actor；一切历史皆 event（state = journal 的纯
fold）；一切副作用是 activity，流经同一条 effect pipeline；一切行为由数据
定义（含 tool 定义本身，内核里不硬编码任何具体 agent）。

**明确的非目标**：确定性 code replay、整树确定性 replay、分布式执行、生产级
多租户。这些是取舍，不是遗漏（§6）。

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
——没有"run"这个概念、没有第二种运行形态、没有额外状态机。续聊、忙时排队、
工作完成激活新 turn、子 agent、中途改编排，都是这一个循环的推论。

**静止（quiescence）**是唯一的"结束"：最后一个 turn 收尾、无在飞工作、无未到
期的自触发——由 journal **形状**自明，不是事件、不是状态机。静止时跑一串固定
动作（发布产出 → 切 checkpoint barrier → 有 parent 则投回执），任何"结束时要
做的事"挂进这个序列。静止可发生多次：再被唤醒、再静止，动作再跑一遍。

**自检**：加功能前先问——"它能不能是一条 Input，或一个 turn 内动作？"都不能，
先怀疑设计错了。

## 3. Inbox：一条通道，多种发送方

"任何一方对 session 说话"统一成"往 inbox 投一条 Input"——用户、子 agent、
timer、外部事件是同一个问题的四个发送方，不是四套机制。**Input 是弱类型的**：
对话面上就是纯内容 + 来源前缀，来源（user / agent / machine / timer / control）
只是 journal 元数据，模型不该看到类型系统。

**三条铁律**：投递与消费解耦（发送方从不阻塞在"agent 忙不忙"上）；
journal-inputs-first（先 fsync 进 durable command log 再回执，崩溃不丢输入）；
有序 + 幂等（输入 / interrupt / 审批 / kill 共用同一条 durable command 通道，
调用方 mint 稳定 `command_id`，同 id 同 payload 返回原回执，重试不双执行）。

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

一条**模型可见面的契约要求**：`spawn_agent` 与读后台输出的工具，其描述必须显式
声明 fire-and-yield（派完可结束 turn、完成会作为消息自动唤醒、无需轮询）。否则
较弱的模型会用"读输出 + sleep"自旋当等待手段，把自动唤醒路径整个架空——实测中
补上这句声明后，同一任务的空转轮询从十余次降到零。**编排的智能在模型，runtime
只提供"随时能投、能杀、能起"的原语。**

**树级约束**是真正的防线：审批沿 correlation id 冒泡到人——审批的永远是人，不是
parent agent；**权限继承拆两条规则**（mode 没有交集运算）——rules 做真交集并在
spawn 时**冻结**成不可变数据（子无法自行放宽，父事后的 mode 跃迁不回溯），mode
不交集但工具面先过冻结 rules；**树预算** = min(子限额, 父剩余)，reserve-at-spawn
/ settle-at-child-idle，深度与扇出有数据化上限（spec 允许 A↔B 成环，上限是唯一
防线）；**子的意义在上下文隔离**——子烧自己的 window，只有符合 contract 的报告
回流父。

## 5. Turn 内机制

**Context assembly 是一等组件**（`fold(journal) → 请求`）：system prompt 拼装
顺序固定，其中 tool / skill / **子 agent 目录**的注入是 multi-agent 可用的前提
——模型不知道某个子 agent 存在就永远不会 spawn 它。**prefix 稳定是显式不变量**
（prompt caching 的经济性约 10x，没有它 agent loop 在经济上不可用）：环境变化
一律**以追加消息进入上下文，绝不改写 prefix**；工具面因此分两级——mode 过滤只
作用于关卡侧的 permitted 面，进 prefix 的 advertised 面在 session 内稳定。压缩
同理：journal 留全量结果（truth），**只有装配视图降级**，故 resume 与 rewind
的语义天然良定义。

**Effect Pipeline**：每个副作用（模型调用、工具、spawn、发布产出）都是一个
Effect，流经同一条管线——hooks、permission、审批、预算不是四个子系统：

```
effect → [1] Floor      硬底线（越界 / 凭据 / 只读模式）：纯判定，直接 deny
         [2] Spawn      结构限制：树深度、扇出、handoff 唯一性
         [3] Hooks pre  observe + block（不改写）
         [4] Permission allow / ask / deny —— policy 是数据
         [5] Budget     reserve-then-settle
         [6] Execute    以 activity 执行
         [7] Hooks post
```

- **纯判定的关卡排最前**，使必拒的 effect 绝不触发有副作用的 pre-hook。
- **判定落在记录边界之内**：结论在执行**之前**落盘（ask 路径先落一条审批请求
  并携带此前已完成的关卡判定——pre-hook 可能已产生副作用，这个事实必须先于
  可能挂几天的审批落盘）。恢复时读记录值，**不重跑 hook、不重读 policy**。
- **预算 reserve-then-settle**：否则 N 个并行 call 各自对着同一个过期计数器
  放行，合起来超支 N 倍。
- **每种关卡结果都定义"模型看到什么"**：deny / block / 拒批 / 执行失败一律渲染
  成 error 形态的 tool result，**loop 继续**；只有 session 级预算耗尽才让模型
  收尾后优雅停止。给模型的错误与给用户的错误是两个 surface，分开设计。
- **边界诚实**：参数级规则只约束能被结构化解析的工具调用；执行类工具（shell、
  解释器、浏览器）的实际行为无法从参数可靠推断——一条命令就能绕过一切参数
  规则。真正的边界必须由执行环境的**强制隔离**（OS sandbox / 容器 / 网络出口
  控制）闭环，隔离缺席时 fail closed、不降级裸跑。不假装规则覆盖了执行类工具。

**执行纪律**：并行 tool call 是常态（ask 挂起不阻塞已放行的 call，下次调模型前
按原 call 顺序收齐结果）；token delta 只走 bus（显式 ephemeral），持久化的是
组装完成的消息；后台 effect 的立即配对结果就是 `{handle, running}`，完成时的
终态兼任 pending input、在安全边界作为新 user 消息进对话；interrupt 触发 sweep
使所有未终态 call 得终态、未决审批作废且迟到应答按 id no-op（否则崩溃恢复后一条
迟到的批准会执行用户早已放弃的危险调用）。

## 6. 持久化与恢复

**最重要的取舍：不做确定性 code replay。** Temporal 式 replay 需要稳定 activity
id、确定性协程调度、divergence 检测——一个数周级的引擎项目；而 agent loop 的全部
状态不过是（消息列表、step 计数、待处理 tool call）。用三件更便宜的东西拿到同样
的用户可见能力：**外部输入 durable accepted**（先留事实再消费，崩溃时从 command
receipt 与 journal fact 的差集恢复）；**state 是纯 fold**（apply 不读时钟、不执行
副作用，对话状态永远可从 log 重建）；**snapshot-resume**（安全边界打 snapshot 并
记 journal offset，resume 只读 `seq > N`；snapshot 是可弃缓存，形状可疑就丢掉走
全量 fold）。

**挂起是显式状态，不是任意点挂起**：等待种类只有"等输入"与"等审批"两个，都发生
在安全边界。等几分钟或几天成本相同，进程死了也一样——**durable 的等待不需要
replay 引擎**。

**Activity**：`Started` 先落盘 → 执行 → `Completed/Failed`；结果落盘前过凭据
redaction；取消以**进程组**为准（确认组内进程全部退出才落 `Cancelled`，否则被
"取消"的子进程会继续产生副作用、污染后续状态）；timeout 走 durable timer，绝不在
关卡代码里读墙钟。**in-doubt（有 Started 无 Completed）按 tool 类别数据化处置**
——崩溃几乎必然砸中 in-flight activity，因为 agent 的墙钟全在模型调用和子进程里：
模型调用自动重发；read-class 与显式 `idempotent: true` 重跑；execute / edit-class
**不重跑**，渲染 `[interrupted by crash]` 让 loop 继续；转人工只留给显式配置的
高危工具。非幂等操作绝不静默重跑——它们根本不重跑，所以无人值守的运行也不会卡在
人工 triage。

**冷启动**：待命的 session 无事可做（待命跨进程存活，下一条输入即接续）；turn
中途崩的走 in-doubt 自愈后继续；在飞子从子 journal 的静止形状结算。**恢复只住在
一个地方**（session resume），不存在与之竞争的第二套机制；actor 崩溃不自动
restart，停在 failed 等人处理，不热循环。

## 7. 分层与 Provider

```
会话内核  Session actor · inbox · loop · turn · 子 session      ← 中心
Turn 机制 context assembly · effect pipeline · 工具             ← 即 "harness"
持久化    journal · fold · snapshot · CAS · in-doubt
扩展层    时间旅行 · 迭代驱动（goal / 周期 / best-of-N）· 生态接入
```

**内核是库**：一切 surface（交互式前端、无人值守批处理、常驻服务、外部事件入口）
都只是 inbox 的投递方 + 输出订阅方，是挂在内核上的薄壳、也都是 actor，不存在
"特权 frontend"。kernel 基座只有三件东西：actor（id + mailbox + behavior）、bus
（进程内 transport、**ephemeral**，任何影响结果的输入必须先 journal 再消费）、
envelope（`command_id` 是外部幂等轴、`causation_id` 是 stream 内因果链，**两轴
分立**才使"command 可重试"成立）。

**Provider 是薄接口**（`complete(request) → stream`，streaming 原生）：能力通用
且可选——caching、thinking、tools、structured output 以 provider 无关的方式表达，
`capabilities()` 声明支持面，请求了不支持的能力**明确降级或报错，绝不静默忽略**；
返回**归一化**——token 计数（含 cache read / write）、finish reason（含各家独有的
异常形态，如畸形 tool call、安全拦截、零候选）、tool call、thinking 块统一成一套
内部表示，管线与记账不感知具体 provider；**opaque signature 随 event 持久化**并
原样回传——丢掉它，某些 provider 的多轮工具调用在第二次请求就会失败，推论是
mid-run 换 provider 必须在压缩边界重开。**至少实现两个 provider**：第二个实现的
唯一作用是验证抽象没漏。

**运行模式**（扩展层）：目标驱动、周期驱动、best-of-N 由迭代驱动器表达，但它们的
会话内形态不新增状态机——完成裁决与定时唤醒只是**静止序列或安全边界上的一格**，
唤醒即"以 program 源投一条输入"。这是 §2 自检的兑现。

## 8. 边界与已知局限

**单进程假设**：bus 是进程内的；跨进程部署时契约要分 ephemeral topic 与
guaranteed send 两通道，重连方必须从 event log 对账未决状态，不依赖 bus 补投。
**不保证整树确定性重现**：保证的是 per-stream 可审计（causation / correlation
链完整），不是跨 actor 消息交错的重现。**软标记不计入安全预算**：untrusted 框定
只降低模型服从注入的概率，真正的缓解是 egress 控制、OS sandbox、permission floor
这些与模型是否听话无关的硬防线，两类防线不得混记；凭据 redaction 同理——runtime
自身绝不写入凭据，但工具输出可能携带任意 secret，这是文档化的残余风险，不是闭合
的保证。
