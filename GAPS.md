# AgentRunner — 设计缺口登记簿（GAPS）

**这是什么**：以"完全支持 Claude Code / Codex（cloud）级 coding agent"为
目标，对 DESIGN.md 做的对抗性审计记录。**只审设计，不审实现**——一个
条目被标为缺口，意味着 DESIGN.md 里没有（或没写清）它的语义，照着设计
文档做一个新实现也做不出这个能力。实现层面的欠账继续记在 PROGRESS.md
的 backlog，不在此重复。

**方法**：先登记第一个多 agent 测试用例暴露的缺口（§1），再构造七条
覆盖 Claude Code 终端形态与 Codex cloud 异步形态的复杂 User Journey
（§2），逐步对照 DESIGN.md 打分，把新暴露的缺口并入登记表（§3），最后
给出设计修订的优先级建议（§4）。

**分级**：
- ❌ **设计缺失**——DESIGN.md 完全没有这个东西，或只有一个名词。
- ⚠️ **设计欠定**——有承诺/有一句话，但语义没写清，照着做会做出
  互不兼容的实现。
- 🔧 **纯实现缺口**——设计已覆盖、实现没跟上（登记指针，正文在
  PROGRESS backlog）。

**影响**：高 = 不补则日常主流程走不通；中 = 常用能力缺失但有绕路；
低 = 锦上添花或边缘场景。

---

## §1 第一个测试用例暴露的缺口

测试用例（多 agent + steer 编排）：用户输入文字+图片 → agent 第一个
turn 启动多个子 agent → 子 agent 完成后消息回灌 → 期间用户以 steer
消息介入 → agent 据此杀死部分子 agent、启动新的 → 全部完成后主 agent
收口。

六步里三步现有设计可支撑（多 turn 机制、task outcome 回灌、run 收尾
quiesce），三步暴露缺口：

### G1 多模态用户输入（图片/附件） — ❌ 设计缺失 · 影响高

DESIGN 交互协议一节只有一行"协议预留（原型不实现）：slash command
调用、附件/图片消息类型"。没有任何语义设计：
- `provider.Part` 的消息模型只有 text / tool_call / tool_result，没有
  image 类型；
- 图片字节的存放没有教义决定——按既有原则应走 CAS（blob-before-event，
  journal 只存 opaque ref，fold 永不读 store，发送时 inflate），但这
  条推导没有写进 DESIGN；
- provider wire 映射（Anthropic image block / Gemini inline_data）、
  尺寸/格式约束、redaction 是否适用于图片、fork 复制时随 artifacts
  走——全部未定。
- 同族缺失：PDF/任意文件附件、粘贴的长文本按附件处理（Claude Code
  的 paste 折叠）。

### G2 并行/后台子 agent（spawn background + 可杀） — ⚠️ 设计欠定 · 影响高

DESIGN L1 有一句承诺："后台 activity：`bash` / `spawn_agent` 支持
`background: true`"，且 handle 配对、outcome 以 user-role 消息回灌、
`task_kill`、run 末尾 quiesce 的机制对 bash 已设计完整。但 spawn 特有
的语义全部未定：
- `SpawnRequested` / `SubagentCompleted` 与后台 activity 终态事件的
  顺序与配对关系；
- 子 run 的 usage 结算时点（后台模式下预留何时释放、失败/取消的
  子 run 从子 fold 结算的规则是否沿用阻塞 spawn 的 S5 修订）；
- `task_kill` 杀子 agent 的语义：child ctx cancel → 子 journal 的终态
  是什么、父侧渲染什么、部分产出（子 blackboard 笔记/artifacts）算不算
  数；
- crash-resume：父崩溃时 in-flight 后台子 run 的 in-doubt 处置（子 有
  自己的 journal，可以 settle-from-child-fold——driver 的 settledChild
  已有先例，但 run 内后台 spawn 没有写）；
- barrier 的 tasks 处置向量对"任务是一个子 agent"的情形是否仍然
  cancel_at_fork 一刀切。

### G3 运行中 steering 文本消息 — ⚠️ 设计欠定 · 影响高

DESIGN 目标列了"运行中途 steering"，交互协议有一行"frontend……向
run 发输入（journal 后生效）"，agent loop 一节把"steering 消费点"列在
turn 边界。但机制未定：
- 传输通道（daemon 线协议里没有 steer/input command；本地进程内没有
  输入通道抽象）；
- 消费语义：mid-turn 到达的消息何时可见（下一 turn 边界？）；
  `WAITING_TASKS` / `WAITING_APPROVAL` park 是否被 steer 消息唤醒
  （本测试用例必须：park 中收到 steer → 立即起新 turn 反应）；
- type-ahead 队列：agent 忙时用户连发多条，是排队逐条消费还是合并；
- steer 与 interrupt 的关系（steer = 追加指令不打断在跑活动；interrupt
  = 取消当前活动。两者组合"打断并改方向"是否一个手势）；
- 子 agent 是否可被 steer（v0 可明确"不可"，但要写下来）。

### G4 并发子 agent 的确定性测试基建 — 🔧 纯实现缺口（测试工具） · 影响中

scripted provider 是单一顺序脚本；并发后台子 agent 竞争消费步骤会让
测试不确定。需要按 子 agent（session/task）路由脚本的 routing provider
+ 用 fifo/信号控制子 agent 完成顺序的编排手段。不动产品设计。

---

## §2 User Journeys（以 Claude Code / Codex cloud 为标尺）

打分记号：✅ 设计可支撑 · ⚠️ 设计欠定 · ❌ 设计缺失。
每条 journey 末尾列出它命中的缺口编号（登记表在 §3）。

### UJ-1 交互式结对编程（Claude Code 终端的一天）

> 开发者在终端里与 agent 结对：贴一段报错 + 一张截图（①），agent 读码
> 后进 plan mode 给方案，用户批准（②）；agent 边改边跑测试，用户中途
> 补一句"顺便把日志级别改成 debug"（③），又连发两条消息排队（④）；
> 一条 rm 命令触发审批，用户选"允许且以后不再问"（⑤）；改完后 agent
> 汇报，用户**继续追问**"为什么选这个方案"（⑥）；会话拉长后手动
> /compact 保留要点（⑦）；用户中途 /model 切到更强的模型跑难题（⑧）；
> 晚上合上电脑，第二天 resume 继续，让 agent"记住这个项目用 pnpm"
> 写进项目记忆（⑨）。

| 步骤 | 判定 | 依据 |
|---|---|---|
| ① 文字+截图输入 | ❌ | G1 |
| ② plan mode + 审批 | ✅ | 3.6/S5.7 计划审批载荷，设计完整 |
| ③ 运行中补一句 | ⚠️ | G3 |
| ④ type-ahead 排队 | ⚠️ | G3（队列语义未定） |
| ⑤ "允许且不再问" | ❌ | G5：审批答复写回 user settings 的规则持久化未设计——permission rules 只有三个静态来源（spec/project/user），没有运行时写回路径 |
| ⑥ 完成后继续追问 | ❌ | **G6：会话续聊形态未设计**。run 是 task-to-completion 模型，end_turn 即 run_ended；`WAITING_INPUT` 只由 wait-class 工具（agent 主动提问）进入。"agent 答完 → 等下一条用户消息 → 同一上下文继续"这个 Claude Code 的**默认交互形态**在设计里不存在——resume 只续未完成的 run，续聊既不是 resume 也不是新 run |
| ⑦ 手动 /compact | ⚠️ | G7：compaction 只有自动阈值触发（trigger_ratio），手动触发/带指示压缩/clear 未设计 |
| ⑧ 中途换模型 | ❌ | G8：spec 冻结于 RunStarted，模型/预算等的运行中变更未设计（冻结是对的，但需要一个显式的"变更即事件"路径，如 ModeChanged 之于 mode） |
| ⑨ 记忆写回 | ⚠️ | G9：memory 文件只设计了读侧注入（S5.2 冻结进 prefix）；"# 记住 X"类写回（写哪个文件、何时生效——本 run prefix 已冻结）未设计 |

### UJ-2 多 agent 并行研究与编排（§1 测试用例的完整形态）

> 用户丢一个 bug 报告 + 截图：主 agent 拆解后**并行**启动三个子 agent
> （复现、查 git 历史、查依赖版本），自己继续读代码（①）；子 agent 的
> 进度实时可见（②）；第一个子 agent 返回后用户 steer："别查依赖了，
> 直接看 v2.3 的迁移文档"——主 agent 杀掉依赖子 agent、新起一个（③）；
> 全部返回后主 agent 汇总（④），把结论发布为 artifact 并在树共享
> blackboard 上留档（⑤）。

| 步骤 | 判定 | 依据 |
|---|---|---|
| ① 并行子 agent + 主 agent 继续工作 | ⚠️ | G2（阻塞 spawn 下主 agent 无法"自己继续读代码"） |
| ② 子 agent 进度实时可见 | ⚠️ | G10：协议列了"后台任务进度 topic"，2.10 进度通道对 bash 未接是已记 backlog；对子 agent 的 turn 级进度镜像（daemon 侧有 blackboard 镜像先例）没有统一设计 |
| ③ steer → 杀 A 起 B | ⚠️ | G2 + G3 |
| ④ outcome 回灌与收敛 | ✅ | S6.1 task outcome + await 机制，设计完整 |
| ⑤ artifact + blackboard | ✅ | S5.4/S5.5，设计完整 |

### UJ-3 云端异步任务（Codex cloud 形态）

> 用户在手机上对着 repo 提交任务"把这个 flaky 测试修了"（①）；平台
> 为任务 provision 一个容器：clone 仓库、跑 setup 脚本、按环境策略
> 收窄网络（②）；agent 在云端跑，用户中途在网页上看到它走偏，发一条
> steer 纠偏（③），也可以直接停掉（④）；agent 产出 diff，用户在网页
> 上审阅后让它开 PR（⑤）；第二天用户回来说"改成用 t.Parallel"，
> **在同一环境/分支上继续**（⑥）；一切用量与审计可查（⑦）。

| 步骤 | 判定 | 依据 |
|---|---|---|
| ① 远程提交任务 | ✅ | daemon submit + idem_key；HTTP/WS 壳是已记 backlog（🔧） |
| ② 环境 provision/setup/网络策略 | ⚠️ | G11：云 workspace 生命周期是 S7 被裁的 cut line，只有一段草图（provision→live→teardown、store 外置、setup prologue 走信任模型）；环境配置模型、secrets 注入、镜像/缓存、per-env 网络策略与 sandbox.network 的关系全部未展开 |
| ③ 网页 steer | ⚠️ | G3（daemon 线协议无 steer command） |
| ④ 远程停止 | ⚠️ | G12：托管 run 的远程控制面不完整——线协议只有 ping/run/drive/attach/approve，没有 stop/interrupt；interrupt 语义设计只绑在终端信号上 |
| ⑤ diff 审阅 → 开 PR | ⚠️ | G13：SCM/PR 工作流零设计——bash+gh 能凑合跑，但"任务产出=可审阅的 diff→PR"这条 Codex 的一等公民路径（diff 呈现、审阅通过才推送、PR 元数据回填 session）没有设计位置（artifacts + 审批载荷是现成的积木，组装方式未写） |
| ⑥ 同环境 follow-up | ❌ | G6（续聊形态）+ G11（环境复用/重建语义——resume 时容器已回收，workspace 从外部源重建是草图里的一句话） |
| ⑦ 用量/审计 | ✅ | usage 结算 + inspect 时间线，设计完整 |

### UJ-4 PR 保姆（提交后自动跟进到绿灯）

> 用户让 agent"盯着这个 PR 直到能合"：CI 失败事件到达（①），agent 被
> 唤醒、拉日志诊断、推修复（②）；reviewer 留了评论（③），agent 逐条
> 处理并回复；期间用户插话"第 3 条评论不用理，回复说明原因即可"（④）；
> 绿灯 + 评论清零后通知用户（⑤）。

| 步骤 | 判定 | 依据 |
|---|---|---|
| ① 外部事件唤醒既有 session | ❌ | G14：外部事件源 → **既有** session 的输入投递未设计。scheduler 一节只有"webhook 触发 = 收到请求后发 RunAgent command"（新起 run）；"CI 结果/PR 评论作为 InputReceived 投进在跑/parked 的 session"没有设计（它其实就是 G3 steering 通道的机器发送方——两者应一并设计） |
| ② 诊断修复循环 | ✅ | driver loop mode / 普通 run 均可承载 |
| ③④ 评论处理 + 用户插话 | ⚠️ | G3 + G14 |
| ⑤ 完成通知 | ✅ | notifier（S6 模块⑤），设计完整 |

### UJ-5 通宵 goal 驱动 + 晨间复盘

> 睡前提交"把测试覆盖率提到 80%"，goal 驱动 + command verifier +
> 预算上限 + 停滞检测（①）；夜里某次迭代把代码改坏，verifier 分数
> 下跌，用户早上看时间线定位那次迭代（②），rewind 到它之前的 barrier
> 再让 agent 换个思路（③）；或者直接 best-of-3 并行重试，选优晋升（④）。

| 步骤 | 判定 | 依据 |
|---|---|---|
| ① goal 驱动 | ✅ | IterationDriver 设计完整（S6/S7） |
| ② 时间线定位 | ✅ | inspect + 迭代事件族 |
| ③ rewind/fork | ✅ | S7 CheckpointBarrier + fork，设计完整 |
| ④ best-of-N + 晋升 | ⚠️ | G15：胜者晋升（fork 或 apply diff）在 DESIGN 里是四个字，v0 已记档推迟；晋升的具体语义（diff 应用回主 workspace 的冲突处理、或以 fork 接管）未设计 |

### UJ-6 安全红线（不受信 repo + 注入对抗）

> 用户 clone 了一个不受信的 repo 让 agent 分析：首次进入触发信任
> 决策（①）；repo 的 README 里埋了 prompt injection 让 agent 外传
> `.env`（②）；agent 的 bash 被网络沙箱收容、凭据路径全程不可达（③）；
> 用户事后审计"它到底动过什么"（④）。

| 步骤 | 判定 | 依据 |
|---|---|---|
| ① 信任模型 | ✅ | trust + project settings 出身区分 |
| ② 注入对抗 | ⚠️ | G16：DESIGN 没有"来自 workspace 内容的指令不可信"的成文条款——permission/sandbox 是硬防线（✅），但注入防御的软层（工具结果与用户指令的信任分级、可疑重定向的呈现）完全没提。至少应作为文档化的威胁模型条目 |
| ③ 沙箱 + 凭据 | ✅ | S7 模块 5 + 硬排除表（出口 review 后已加宽） |
| ④ 审计 | ✅ | event log + inspect + EffectResolved 判定链 |

### UJ-7 大仓库长会话（多日、跨工具）

> 百万行 monorepo：agent 用 semantic_search 定位（①）；会话跨三天、
> 自动 compaction 多次仍不迷失（②）；中途要同时看另一个共享库的源码
> ——第二个根目录（③）；通过 MCP 连内部 ticket 系统读需求（④）；用户
> 在两台机器间切换继续同一会话（⑤）。

| 步骤 | 判定 | 依据 |
|---|---|---|
| ① semantic_search | ✅ | S7 模块 4（IndexStore 第四类状态） |
| ② 多次 compaction | ✅ | S4.5 设计完整（含跨 compaction fork 语义） |
| ③ 多根 workspace | ❌ | G17：workspace = 单根是各处的隐含前提（路径边界、快照、索引全绑单根）；--add-dir 类多根未设计 |
| ④ MCP | ✅ | S5.1 生命周期 + schema 记录 |
| ⑤ 跨机器续会话 | ⚠️ | G11 的近亲：store 外置（journal/CAS 不在本机）只有 cut line 一句话 |

---

## §3 缺口登记表（汇总）

| # | 缺口 | 级别 | 影响 | 命中 journey |
|---|---|---|---|---|
| G1 | 多模态用户输入（图片/PDF/附件）：消息模型、CAS 存放、wire 映射、redaction/fork 语义 | ❌ | 高 | 测试用例, UJ-1/2/3 |
| G2 | 并行/后台子 agent：spawn background 的事件序、usage 结算、task_kill 杀子 run、crash-resume、barrier 处置 | ⚠️ | 高 | 测试用例, UJ-2 |
| G3 | 运行中 steering 文本消息：通道、park 唤醒、type-ahead 队列、与 interrupt 的组合、子 agent 是否可 steer | ⚠️ | 高 | 测试用例, UJ-1/2/3/4 |
| G4 | 并发子 agent 的确定性测试基建（routing scripted provider） | 🔧 | 中 | 测试用例 |
| G5 | 审批答复的规则持久化（"允许且不再问"写回 user settings） | ❌ | 中 | UJ-1/6 |
| G6 | **会话续聊形态**：answer 后等待下一条用户消息、同一上下文/同一 session 继续——Claude Code 的默认交互模型 | ❌ | **高** | UJ-1/3 |
| G7 | 手动上下文操作：/compact（可带指示）、/clear | ⚠️ | 低 | UJ-1/7 |
| G8 | 运行中 spec 变更（换模型等）的"变更即事件"路径 | ❌ | 中 | UJ-1 |
| G9 | 记忆写回（# remember → CLAUDE.md/项目记忆） | ⚠️ | 中 | UJ-1/7 |
| G10 | 子 agent/后台任务的实时进度呈现（2.10 进度通道的统一设计） | ⚠️ | 中 | UJ-2/3 |
| G11 | 云 workspace 生命周期展开：环境配置模型、secrets 注入、store 外置、环境复用/重建（S7 被裁项的正式设计） | ⚠️ | 高（云形态）| UJ-3/7 |
| G12 | 托管 run 的远程控制面：stop/interrupt command（steer 并入 G3） | ⚠️ | 中 | UJ-3/4 |
| G13 | SCM/PR 工作流：diff 审阅→PR→元数据回填的组装设计（积木已有） | ⚠️ | 中 | UJ-3/4 |
| G14 | 外部事件源 → 既有 session 的输入投递（webhook/CI/PR 评论作为 InputReceived；与 G3 同一通道的机器发送方） | ❌ | 高（云形态）| UJ-4 |
| G15 | best-of-N 胜者晋升语义（apply diff 的冲突处理 / fork 接管） | ⚠️ | 低 | UJ-5 |
| G16 | prompt injection 威胁模型成文（workspace 内容的信任分级） | ⚠️ | 中 | UJ-6 |
| G17 | 多根 workspace（--add-dir 类） | ❌ | 低 | UJ-7 |
| G18 | 内置工具面完整性：glob/grep/web fetch/search 在 DESIGN 列名未 spec（web 类牵动 network 资源类与注入面） | ⚠️ | 中 | UJ-1/7 |
| G19 | hooks 生命周期事件族：只有 pre/post tool，无 session start/stop、用户输入提交、通知类钩子 | ⚠️ | 低 | UJ-1/6 |

**已明确不算缺口的**（核对后设计已覆盖，防止重复登记）：审批与计划
载荷（S5.7）、compaction 自动触发与跨界 fork（S4.5/S7）、rewind/fork
（S7）、goal/loop/parallel 驱动（S6/S7）、notifier（S6⑤）、MCP
（S5.1）、信任模型、预算 reserve-then-settle、凭据 redaction 与快照
硬排除、sandbox network、观测（inspect/事件链）。

---

## §4 设计修订的优先级建议

按"补一个解锁一片"的杠杆排序，供讨论，不是结论：

1. **G6 会话续聊**——交互模型的地基。建议方向：run 终态与 session
   生命周期解耦（session = 多个 run 的对话链，或 run 增加
   `WAITING_INPUT` 的常态化入口），它同时是 UJ-1⑥ 和 UJ-3⑥ 的答案，
   并影响 G3（steer 与续聊共用输入投递）与 G14。
2. **G3 + G14 统一为"输入投递设计"**——一条通道三种发送方（终端
   用户、web 用户、机器/webhook），一套消费语义（park 唤醒、turn
   边界、队列）。journal-inputs-first 教义现成，缺的是通道与消费点
   的正式化。
3. **G2 后台子 agent**——L1 承诺的兑现设计，多 agent 编排的前提。
4. **G1 多模态输入**——独立性好，可并行设计；CAS 教义现成推导。
5. **G11 云 workspace 展开**——云形态的门槛，原 cut line 重启为
   正式设计（含 G12 远程控制面）。
6. 其余（G5/G7/G8/G9/G10/G13/G15/G16/G17/G18/G19）在上述五块定型后
   按需排入，多数是薄层。

---

*登记纪律：新缺口一律入 §3 表格并注明来源 journey；缺口被设计修订
关闭时，在条目上标注关闭它的 DESIGN 章节与日期，不删除行。*
