# AgentRunner — 缺口登记簿（GAPS）

> **v2 收口注记（2026-07-05）**：G1/G2/G3/G6 已在 v2 M1–M5 关闭
> （标注见各条目），G18 的 write_file 部分关闭；C1–C10 全部达成
> （DESIGN.md §核心验收，QA-01..09 真实 API 闸门全绿）。其余条目仍为
> 扩展层缺口（v2 核心绿灯已达成、铁律已解除——见 archive/v2/CORE.md，
> 逐条按 PROCESS.md 的增量流程排期）。

**这是什么**：以 JOURNEYS.md 的 20 条 user journey 为标尺，对 AgentRunner
（DESIGN.md 的设计 + S1–S7 的实现）做的支持度审计。journey 目录回答
"产品要做什么"（JOURNEYS.md），本文件回答"我们缺什么"。

**分级**：
- ❌ **设计缺失**——DESIGN.md 没有这个东西，或只有一个名词。
- ⚠️ **设计欠定**——有承诺/有一句话，语义没写清，照着做会做出
  互不兼容的实现。
- 🔧 **纯实现缺口**——设计已覆盖、实现没跟上。

**影响**：高 = 不补则主流程走不通；中 = 常用能力缺失但有绕路；低 =
薄层/边缘。

---

## §1 逐 journey 支持度速览

判定：✅ 现在能走通 · 🟡 主体能走、部分步骤缺（列缺口）· ❌ 关键步骤
卡死。

| Journey | 判定 | 卡点 |
|---|---|---|
| UJ-01 即问即答 | ✅ | 续聊 ✅（G6 关闭）；grep/glob ✅（INC-3，G18 收口）；语义检索 ✅ |
| UJ-02 小修快跑 | ✅ | —（S1 验收场景即此） |
| UJ-03 结对续聊 | ❌ | **G6 续聊形态不存在**——答完即 run_ended |
| UJ-04 贴图贴日志 | ❌ | **G1 多模态输入全链路缺失**（图片+长贴折叠） |
| UJ-05 从零起项目 | ✅ | write_file ✅（M4.3）、grep/glob ✅（INC-3） |
| UJ-06 大重构走计划 | 🟡 | plan/审批/修订再批全通；"agent 主动提问"G20 |
| UJ-07 中途纠偏 | 🟡 | interrupt ✅；steer 消息与队列 G3 |
| UJ-08 权限日常 | 🟡 | 规则/审批/审计 ✅；"允许且不再问"写回 G5 |
| UJ-09 长会话续命 | 🟡 | 自动 compaction/跨日 resume ✅；整条以 G6 为前提；手动 compact G7、记忆写回 G9、跨机 G11 |
| UJ-10 提交流水 | ✅ | 全程借 bash+gh 可走；一等公民化 G13 |
| UJ-11 代码评审员 | 🟡 | 只读约束(plan mode)✅；角色切换 G8、续聊 G6 |
| UJ-12 PR 保姆 | ❌ | **G14 外部事件唤醒既有 session 不存在**；插话 G3 |
| UJ-13 手机派活 | ❌ | 环境生命周期 G11、diff 审阅门 G13、follow-up G6；HTTP 壳 🔧backlog |
| UJ-14 定时值守 | ✅ | cron/overlap/carry/通知/fail-closed 全通；跨重启唤醒 🔧backlog |
| UJ-15 通宵冲目标 | ✅ | goal/verifier/停滞/预算/时间线/rewind 全通（最强区） |
| UJ-16 三路并击 | 🟡 | 并行隔离+选优 ✅；胜者晋升 G15 |
| UJ-17 远程驾驶舱 | 🟡 | attach/远程审批/用量 ✅；stop ✅（INC-4）；远程 steer G3 |
| UJ-18 多 agent 编排 | ❌ | **G2 后台子 agent 未实现**（阻塞 spawn 无编排窗口）；steer G3、子进度 G10、图片 G1 |
| UJ-19 生态接入 | ✅ | MCP/skills/写审批/断连恢复 ✅；自定义命令 ✅（INC-5） |
| UJ-20 不受信审计 | ✅ | 信任/沙箱/凭据红线/审计全通；注入威胁模型成文 G16 |
| UJ-21 崩溃自愈与重启接续 | 🟡 | 恢复语义✅（resume/in-doubt/终态把关，QA-08）；**自动性缺**：boot sweep、子 crash 自动 resume（G22）（2026-07-05 新增行） |
| UJ-22 会话内目标 | ❌ | **G23 形态不存在**——goal 只有 driver+fresh run 形态，context 不延续（原始需求丢失，2026-07-05 补登记） |

**汇总**：6 通 · 9 部分 · 5 卡死。5 条卡死全部落在同一族——**交互与
输入投递**（续聊 G6、多模态 G1、steering G3、事件唤醒 G14、后台子
agent G2）。durability/驱动/安全这一半（UJ-02/05/10/14/15/20）是
扎实的；交互外壳是明确的前沿。

---

## §2 缺口登记表

### 交互与输入投递（5 条卡死 journey 的根源）

**G1 多模态用户输入（图片/PDF/附件/长贴折叠） — ✅ 已关闭（v2 M4，2026-07-05）**
关闭位置：provider.Part image/file + event.InputReceived.Images/Files
(AttachmentRef) + CAS blob-before-event + 组装 inflate + gemini
inline_data/anthropic image block + `ar send --image` + 长贴 >10KB 折叠
file part。闸门：QA-07(vision 三要素+ref-not-bytes) + QA-03 真实 API
PASS;孪生 TestConversationalImageInputEndToEnd/TestLongPasteFolds。
**余项**：PDF/附件泛化;blob 在 fork/rewind 下的归属语义;`ar new`
开场消息不折叠/不带图(不对称,DESIGN §9.1 记档)。原文:
`provider.Part` 只有 text/tool_call/tool_result；`InputReceived` 只有
Text；协议仅一行"附件/图片消息类型预留"。缺：消息模型、CAS 存放教义
（blob-before-event、fold 只带 ref、发送时 inflate）、Anthropic/Gemini
wire 映射、长粘贴按附件折叠、redaction/fork 语义。
→ UJ-04, UJ-18

**G2 并行/后台子 agent（spawn background + 可杀） — ✅ 已关闭（v2 M3/M5，2026-07-05）**
关闭位置：spawn_agent{background:true} 走 bg 机制(SpawnRequested+
ActivityStarted{Background} 立即配 handle)、SubagentCompleted 先于
activity 终态、task_kill/ar kill 双杀路径(kill 有 InputReceived
{control} 起源)、崩溃 settle-from-child-fold(M5.1)。闸门：QA-04/05/
08/09 真实 API PASS。**余项**：barrier tasks 对"任务=子 agent"的处置
语义(fork/rewind 扩展层连带);daemon kill -9 孤儿化在飞 bash 子进程
(重启后 pgid 清扫未做)。原文:
L1 一句承诺（bash/spawn_agent 支持 background:true）；bash 侧机制完整，
spawn 侧未定：SpawnRequested/SubagentCompleted 与后台终态的事件序、
usage 结算时点、task_kill 杀子 run 的终态与部分产出归属、父崩溃时
in-flight 子 run 的 settle-from-child-fold、barrier tasks 处置对
"任务=子 agent"的语义。实现完全缺失。
→ UJ-18

**G3 运行中 steering 文本消息 — ✅ 已关闭（v2 M2+收口，2026-07-05）**
关闭位置：daemon send command + durable mailbox(确认即持久,收口
F.3) + inbox(64 type-ahead) + 忙时排队边界生效 + interrupt 分立
(闸门 QA-02/06/08 真实 API PASS)。子 agent 不可 steer(v0 明确否,
父 kill+re-spawn 代替)。**余项**：机器发送方(G14);WAITING_APPROVAL
park 期间消息只排队不唤醒(审批答复才解栈,唤醒语义待定)。原文:
目标有、消费点提了一句、机制全缺：传输通道（daemon 线协议无
steer/input command）、park（WAITING_TASKS/APPROVAL）被消息唤醒、
type-ahead 队列语义、steer 与 interrupt 的组合手势、子 agent 是否可
steer（v0 明确"否"也要写下）。
→ UJ-07, UJ-12, UJ-17, UJ-18

**G6 会话续聊形态 — ✅ 已关闭（v2 M1/M5，2026-07-05）**
关闭位置：Loop.Conversational(答完 park 待命,close 才终结)+ `ar new/
send/close` + 重启后 send 即复活(M5.1,RunStarted.Conversational)。
闸门：QA-01/08 真实 API PASS;孪生 TestConversationalMultiInput/
ParkResumes/MidTurnCancelResumes。原文:
run 是 task-to-completion：end_turn 即 run_ended。"答完 → 等下一条
用户消息 → 同一上下文继续"这个 coding agent 的**默认交互形态**不存在
——resume 只续未完成 run，续聊既不是 resume 也不是新 run。需要 session
与 run 生命周期解耦（session = 对话链）或 run 常态化进 WAITING_INPUT。
与 G3/G14 共用"输入投递"地基。
→ UJ-01, UJ-03, UJ-09, UJ-11, UJ-13

**G14 外部事件唤醒既有 session — ❌ 设计缺失 · 高（云形态）**
scheduler 只有"webhook → RunAgent command"（新起 run）一行；"CI 结果/
PR 评论作为 InputReceived 投进在跑或 parked 的 session"没有设计。它是
G3 通道的机器发送方——一条输入投递设计应同时覆盖终端用户/web 用户/
机器三种来源。
→ UJ-12

### 治理与交互薄层

**G5 审批答复的规则持久化（"允许且不再问"） — ❌ 设计缺失 · 中**
permission rules 只有三个静态来源；审批现场写回 user/project 配置的
路径未设计（写哪层、如何表达为规则、对当前 run 何时生效）。
→ UJ-08

**G7 手动上下文操作（/compact 带指示、/clear） — ⚠️ 设计欠定 · 低**
只有自动阈值触发。
→ UJ-09

**G8 运行中 spec 变更（换模型/角色切换） — ✅ 已关闭（2026-07-05,决策 #32）**
关闭位置:SpecChanged 事件 + `ar agent` 命令,用户切换免确认,prefix 显式换代。原文:
spec 冻结于 SessionStarted（对的），但缺一个显式变更事件族（如 ModeChanged
之于 mode）承载换模型、权限面切换。
→ UJ-11（评审→动手的角色切换）

**G9 记忆写回（# remember → CLAUDE.md/项目记忆） — ⚠️ 设计欠定 · 中**
只设计了读侧注入（prefix 冻结）；写回哪个文件、本 run 何时生效未定。
→ UJ-09

**G16 prompt injection 威胁模型成文 — ⚠️ 设计欠定 · 中**
硬防线（沙箱/凭据/权限）✅；"workspace 内容不可信"的信任分级与可疑
重定向呈现没有成文条款。
→ UJ-20

**G19 hooks 生命周期事件族 — ⚠️ 设计欠定 · 低**
只有 pre/post tool（observe+block）。session start/stop、用户输入提交、
通知类钩子未设计。**注意：20 条 journey 无一压到 hooks——目录本身在
此处覆盖不足**。
→ （无 journey 覆盖）

**G20 agent 主动提问（wait-class 工具，ask_user） — ⚠️ 设计欠定 · 中**
wait-class 词汇与 WAITING_INPUT park、免 in-doubt 误杀的类别语义已设计；
但没有任何 wait-class 工具定义，CLI/daemon 也无应答路径（approve 只答
审批）。实现侧 loop 对 WaitInput 直接报"no resolver"。
→ UJ-06

**G21 自定义命令 / slash 面 — ✅ 已关闭（INC-5，2026-07-09）**
关闭位置：`internal/command` 包（mirror skill）+ DESIGN §10「自定义命令」
子节。定义位 `<root>/.claude/commands/<name>.md`（Claude Code 约定）；
展开语义 = **注入 prompt 文本**、在 **ingest 时**（落 journal 前）于两处
唯一入口（`Loop.Run` 开场 task + `journalInput` 每条 send）展开，
`$ARGUMENTS` 占位替换、fold 保持纯、resume 自包含；与 skills 边界 = 命令
对模型不可见（用户侧宏），skills 是模型侧能力。闸门：TestExpand*/
TestDiscover + 真实 API（new+send 两路展开进 journal 验证）。原文：
协议一行预留；命令的定义位、展开语义、与 skills 的边界未设计。
→ UJ-19

### 工具与检索面

**G18 内置工具面完整性 — 🟡 部分关闭（write_file ✅ v2 M4.3；grep/glob ✅ INC-3；web 仍开放）**
DESIGN 列名"file read/**write**/edit、bash、**glob/grep**、**web
fetch/search**"，其中：write_file ✅（v2 M4.3）；**grep/glob ✅（INC-3，
2026-07-09）**——独立 read-class 工具，与 semantic_search 共用凭据/
vendored 排除谓词（`index.SkipDir/SkipFile`）、命中过 redaction、per-tool
截断；闸门 TestGrep*/TestGlob* + QA-11 真实 API。**余项**：web fetch/search
完全未 spec（牵动 network 资源类与注入面，见 G16；设计草图见
docs/increments/INC-web-tools 评估）。
→ UJ-01, UJ-05

**G10 子 agent/后台任务实时进度 — ⚠️ 设计欠定 · 中**
协议列了"后台任务进度 topic"；2.10 进度通道对 bash 未接（已记
backlog）；子 agent 的 turn 级进度镜像无统一设计。
→ UJ-18

### 云与远程

**G11 云 workspace 生命周期展开 — ⚠️ 设计欠定 · 高（云形态）**
S7 被裁 cut line，只有一段草图。缺：环境配置模型、setup 脚本信任、
secrets 注入、镜像/缓存、per-env 网络策略与 sandbox.network 的关系、
store 外置（journal/CAS 离机）、环境回收后 follow-up 的重建语义、
并行任务的环境隔离。
→ UJ-13, UJ-09（跨机续作）

**G12 托管 run 远程控制面（stop command） — ✅ 已关闭（INC-4，2026-07-09）**
关闭位置：daemon 线协议加 `stop` 命令 + `ar stop` CLI；stop =
teardown-no-mark（复用换 agent 的 plain-teardown 原语），session 落
durable 待命、send 复活，镜像 SIGTERM；顺带修 handleDrive 加 per-run
cancel（drive 系列此前不可 stop）。闸门：TestStop*（孪生，含 drive）+
真 daemon 手验。原文：线协议只有 ping/run/drive/attach/approve；stop 缺失，
interrupt 语义只绑终端信号。（steer 并入 G3。）
→ UJ-17

**G13 SCM/PR 工作流一等公民化 — ⚠️ 设计欠定 · 中**
bash+gh 全程可走（UJ-10 判✅）；缺的是 Codex 式"任务产出=可审阅
diff→批准→PR→元数据回填 session"的组装设计（diff 审阅门、审阅通过
才 push 的约束表达）。积木（artifacts/审批载荷/outputs 契约）都在。
→ UJ-10, UJ-13

### 驱动与时间旅行

**G23 会话内目标（in-session goal）+ goal 控制面 — ❌ 设计缺失 · 高（原始需求，2026-07-05 补登记）**
**需求丢失记档**："goal 挂在当前会话、context 必须延续"是项目原始
需求之一，但从未成文为 journey；S6 在"run=task-to-completion 是唯一
形态"的时代把 goal 建成 IterationDriver + fresh child run（决策
#21），UJ-15 按已实现形态倒写，而本审计以 JOURNEYS 为标尺——需求不
在标尺上，审计永远发现不了它丢了。根因与流程对策见 LOG 2026-07-05。
**与现设计的冲突（实施时必须走 PROCESS 不变量变更流程）**：开发者
裁定 DESIGN §13 "每轮迭代 = fresh child run"与决策 #21 对 **goal
形态不适用**——目标模式的 context 必须延续，割裂不可接受；fresh-run
教义保留给 best-of-N（隔离本就是其语义）与批式 loop；UJ-15 通宵形态
的归属届时一并裁决。
**目标形态（UJ-22）**：goal 状态挂在 conversational session 上；
检查点 = final generation 该出现处（本要回待命处）——先跑
verifier（journaled、过管线的 effect），不满足 → 反馈作为程序来源的
input 进 inbox → **同一上下文**下一 turn；满足 → 达成回执 + 摘 goal
+ 待命。generation step 永不被挟持，检查只住在 turn 的收尾处。
**控制面** = control 输入：pause / resume / update / cancel 全走既有
send 通道；update 触及"spec 冻结于 SessionStarted"不变量——goal 参数需
定义为可变更的 session 状态（事件承载）而非冻结 spec，与 G8"变更即
事件"同族；G12 远程 stop 顺路收编。**预算**：per-turn 的 max_generation_steps
（已有，防 runaway）之上需要 goal 级预算（轮数/token/墙钟）。
→ UJ-22；关联 G8 / G12 / G20（human verifier 即 ask 路径）

**G15 best-of-N 胜者晋升语义 — ⚠️ 设计欠定 · 低**
"晋升（fork 或 apply diff）"四个字；apply-diff 的冲突处理、fork 接管
的交接未设计（v0 留盘由用户晋升，已记档）。
→ UJ-16

### 监督与恢复

**G24 task 形态的显式重开 — ✅ 已消解（2026-07-05,静止模型决策 #31）**
"task 形态"概念删除,交付非终点,任何 session 可继续——问题不复存在。原文:
conversational 的显式重开（含已 close）已落地；task session 被 send
重开后的形态未定——升格 conversational 还是原形态续跑、epilogue 是否
重复执行、outputs 重发布语义——暂拒绝（TestSendToCompletedTaskRefused
钉住现状）。
→ UJ-03/09 连带

**G22 监督语义：崩溃自动恢复与重启接续 — ⚠️ 设计欠定 · 中（无人值守形态的地基，2026-07-05 登记）**
已有的一半（全部在，勿重复设计）：恢复语义本身——journal/fold 状态
无损重建、in-doubt 按类别处置、settle-from-child-fold、send 即复活的
journal 形状把关、kill/close/interrupt 的**终态判别**（DESIGN §6/§17，
QA-08 crash 矩阵）。缺的另一半是**自动性**：
① **boot sweep**——daemon 启动无"未完成工作扫描"，中断在 turn 中途
的 session 躺到有人 send 才复活；cron 跨重启唤醒（backlog）同族，
应一并收编；
② **子 session 崩溃的自动 resume**——daemon 存活时单个子 session
crash → `ActorCrashed` 标 dead（kernel 明确 no auto-restart），无
自动拉起路径；driver 的 `on_child_failure: retry{max,backoff}` 未
泛化到 spawn_agent 子 session；屡崩升级（避免热循环）策略未定；
③ **kill/crash 语义成文**——"显式 kill/close 产生带来源的标记,任何
自动恢复不得越过标记"已成文为决策 #30(2026-07-08 措辞随静止模型
校准;机制 TestAutomaticResumeSkipsMarkedSession 钉住)。
**明确不做**：Erlang 式 supervision tree 自动 restart——与原则 6
（恢复只住在一个地方）冲突；表述统一为 **restart = resume**。
→ UJ-21
"像没 crash 一样"刻意不承诺：非幂等副作用绝不静默重跑是红线
（决策 #6），承诺的是"不丢历史/不丢输入/从最近安全边界继续/崩溃
事实对模型可见"。

### 其他

**G4 并发子 agent 确定性测试基建（routing scripted provider） — 🔧 · 中**
→ 多 agent 类 e2e 测试的前提。

**G17 多根 workspace（--add-dir 类） — ❌ 设计缺失 · 低**
单根是各处隐含前提（路径边界/快照/索引）。**当前 20 条 journey 目录
未包含此场景**——保留自旧审计，纳入与否待目录定版。

---

## §3 已确认覆盖（防重复登记）

对照 20 条 journey 核实、设计与实现俱在：编辑-执行闭环与失败自纠
（UJ-02）、空 workspace 生成（UJ-05）、plan mode 全流程含拒绝-修订-
再批（UJ-06）、interrupt 协作取消与部分输出（UJ-07）、规则/审批/
拒绝理由回灌/判定审计（UJ-08）、自动 compaction 与跨日 resume
（UJ-09）、git/PR 借 bash 全程（UJ-10）、只读角色约束（UJ-11）、
cron/overlap/carry/静默通知/fail-closed（UJ-14）、goal/verifier/
停滞/预算/时间线/barrier/rewind/fork（UJ-15）、并行隔离与选优
（UJ-16）、attach/远程审批/用量审计（UJ-17）、树预算/权限只窄不宽/
blackboard/artifacts（UJ-18 的编排底座）、MCP 全生命周期含断连恢复
与写审批、skills（UJ-19）、信任模型/网络沙箱/凭据红线/全程审计
（UJ-20）。

已记 PROGRESS backlog 的实现项（不再重复）：HTTP/WS 壳、cron 跨重启
唤醒、output 进度 tail、shadow repo 并发 flock、overlap:interrupt。

---

## §4 修订优先级建议

1. **G6 续聊**——单条卡死最多（5 条 journey），交互模型的地基；
   session/run 生命周期解耦是其它输入类缺口的载体。
2. **G3 + G14 合并为"输入投递设计"**——一条通道、三种发送方（终端/
   web/机器）、一套消费语义（park 唤醒、边界消费、队列）；G12 的
   stop/interrupt command 顺路收编。
3. **G2 后台子 agent**——多 agent 编排（UJ-18）的开关，L1 承诺兑现。
4. **G1 多模态输入**——独立性好，可并行设计。
5. **G11 云 workspace 展开**——云形态门槛（UJ-13 整条）。
6. 薄层批：G5 / G7 / G8 / G9 / G10 / G13 / G15 / G20 / G21 在上述
   定型后按需排入；G16/G19 是文档条款可随时补；G17 待目录定版。

**目录自身的覆盖债**（审计的副产品）：hooks 无 journey 覆盖（G19）、
web fetch/search 无 journey 直接压（G18）、多根 workspace 未入目录
（G17）——下轮修目录时决定补场景还是明示不覆盖。

---

*登记纪律：新缺口一律入 §2 并注明来源 journey；缺口被设计修订关闭时，
标注关闭它的 DESIGN 章节与日期，不删除行。§1 速览随每轮设计修订重打分。*
