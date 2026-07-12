# AgentRunner — 缺口登记簿（GAPS）

> **v2 收口注记（2026-07-05）**：G1/G2/G3/G6 已在 v2 M1–M5 关闭
> （标注见各条目），G18 的 write_file 部分关闭；C1–C10 全部达成
> （DESIGN.md §核心验收，QA-01..09 真实 API 闸门全绿）。其余条目仍为
> 扩展层缺口（v2 核心绿灯已达成、铁律已解除——见 archive/v2/CORE.md，
> 逐条按 PROCESS.md 的增量流程排期）。

**这是什么**：以 JOURNEYS.md 的 24 条 user journey 为标尺，对 AgentRunner
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
| UJ-03 结对续聊 | ✅ | G6 已关闭：同 session 续聊、park/resume、跨空闲保持 |
| UJ-04 贴图贴日志 | ✅ | G1 已关闭；图片/任意文件/长贴折叠全链路可用 |
| UJ-05 从零起项目 | ✅ | write_file ✅（M4.3）、grep/glob ✅（INC-3） |
| UJ-06 大重构走计划 | ✅ | plan/审批/修订再批全通；agent 主动提问 ✅（ask_user，INC-5，G20 关闭） |
| UJ-07 中途纠偏 | ✅ | interrupt 与 steering/type-ahead 分立；G3 已关闭 |
| UJ-08 权限日常 | ✅ | 规则/审批/审计 + “允许且不再问”写回（INC-17） |
| UJ-09 长会话续命 | 🟡 | 自动/手动 compaction（G7 ✅ INC-6）、跨日 resume、续聊 ✅；记忆写回 G9、跨机 G11 |
| UJ-10 提交流水 | ✅ | 全程借 bash+gh 可走；一等公民化 G13 |
| UJ-11 代码评审员 | ✅ | 只读约束、角色切换、续聊均已覆盖（G8/G6 关闭） |
| UJ-12 PR 保姆 | 🟡 | G14 外部事件唤醒 ✅（INC-50 webhook ingress）；GitHub 具体集成（PR 评论/CI 状态归一）借 bash+gh 可走 |
| UJ-13 手机派活 | ❌ | 环境生命周期 G11、diff 审阅门 G13、follow-up G6；HTTP 壳 🔧backlog |
| UJ-14 定时值守 | ✅ | cron/overlap/carry/通知/fail-closed 全通；跨重启唤醒 🔧backlog |
| UJ-15 通宵冲目标 | ✅ | goal/verifier/停滞/预算/时间线/rewind 全通（最强区） |
| UJ-16 三路并击 | 🟡 | 并行隔离+选优 ✅；胜者晋升 G15 |
| UJ-17 远程驾驶舱 | ✅ | attach/远程审批/用量/stop/steer 全通；Web UI 产品面见 UJ-24 |
| UJ-18 多 agent 编排 | ✅ | G2/G3/G10/G1 均关闭；后台子 agent、进度、kill、回灌全通 |
| UJ-19 生态接入 | ✅ | MCP/skills/写审批/断连恢复 ✅；自定义命令 ✅（INC-8）；自定义 command tools ✅（INC-55，HANDA #4，trust 门=hooks 级/全管线/OS sandbox） |
| UJ-20 不受信审计 | ✅ | 信任/沙箱/凭据红线/审计全通；注入威胁模型成文 G16 |
| UJ-21 崩溃自愈与重启接续 | 🟡 | 恢复语义✅（resume/in-doubt/终态把关，QA-08）；**自动性缺**：boot sweep、子 crash 自动 resume（G22）（2026-07-05 新增行） |
| UJ-22 会话内目标 | ✅ | **G23 已关闭（INC-D1）**——in-session goal 挂会话、context 延续；决策 #21 拆两形态 |
| UJ-23 工程团队模拟 | ✅ | INC-12：动态角色、横向消息、revive、用户直达与子会话 live 全通 |
| UJ-24 Web UI 驾驶 AgentRunner | ✅ | INC-19/23/29/38/40/41/57/60：Codex 式信息架构 + truthful progressive hydration（454 session 首屏分页、后台补齐、refresh 串行）+ 四控件 environment composer/selected-ref worktree + Worked/Changes outcome + durable `Working tree / Last turn` review scope + 大 Diff 有界 disclosure + hover actions + 内联审批 + responsive Supervision/recovery + restart-safe Scheduled + structured Run details + responsive/a11y/focus return；QA-27/34/36/41/42/43/60/61 |

**汇总（2026-07-11 更新）**：20 通 · 3 部分 · 1 卡死。G14 已关闭
（INC-50 webhook ingress），UJ-12 转部分；剩余卡死集中在云环境
（UJ-13，G11/G13）；主线本机交互、编排与 Web UI 已可走通。

---

## §2 缺口登记表

### 交互与输入投递（5 条卡死 journey 的根源）

**G1 多模态用户输入（图片/PDF/附件/长贴折叠） — ✅ 已关闭（v2 M4，2026-07-05）**
关闭位置：provider.Part image/file + event.InputReceived.Images/Files
(AttachmentRef) + CAS blob-before-event + 组装 inflate + gemini
inline_data/anthropic image block + `ar send --image` + 长贴 >10KB 折叠
file part。闸门：QA-07(vision 三要素+ref-not-bytes) + QA-03 真实 API
PASS;孪生 TestConversationalImageInputEndToEnd/TestLongPasteFolds。
**余项**：~~PDF/附件泛化~~（✅ 已收 INC-9：`ar send --file` 任意类型，
sniff MIME → file part，Gemini inline_data / Anthropic document block；
QA-15 真 Gemini 读 PDF 关键词 PASS）;blob 在 fork/rewind 下的归属语义;
`ar new` 开场消息不折叠/不带图(不对称,DESIGN §9.1 记档)。原文:
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
关闭位置：daemon send command + durable CommandLog(确认即 accepted，
INC-11.2 已把 control/close/interrupt/approval/kill 同收编) + FIFO wake +
忙时排队边界生效 + interrupt 分立
(闸门 QA-02/06/08 真实 API PASS)。子 agent 不可 steer(v0 明确否,
父 kill+re-spawn 代替)。**余项**：WAITING_APPROVAL
park 期间消息只排队不唤醒(审批答复才解栈；INC-D2/INC-50 定案为
"排队不解栈")。机器发送方已由 INC-50 关闭(G14)。原文:
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

**G14 外部事件唤醒既有 session — ✅ 已关闭（INC-50，2026-07-11）**
关闭位置：daemon HTTP ingress（`ar daemon --http` + `POST /hooks/<id>`
+ `ar hook create/list/revoke`）把外部事件作为 `source:"machine"` /
`trust:"untrusted"` 的 InputReceived 投进既有 session 的 durable
inbox——机器发送方与终端/web 用户走同一条输入投递通道（§2 三类归一
兑现，INC-D2 设计稿落地）。信任/鉴权条款成文为 DESIGN 决策 #39
（per-hook token 仅哈希落盘、未鉴权限流/body 上限、loop 侧强制隔离
框定+trust 钳制+不做宏展开、machine 不越 user-kill、X-Command-Id
幂等）。闸门：TestHookIngress*/TestMachineInputFramedAndTrustClamped
孪生 + QA-50 真机。原文（历史）：scheduler 只有"webhook → RunAgent
command"（新起 run）一行；"CI 结果/PR 评论作为 InputReceived 投进
在跑或 parked 的 session"没有设计。
→ UJ-12

### 治理与交互薄层

**G5 审批答复的规则持久化（"允许且不再问"） — ✅ 已关闭（INC-17，2026-07-09，取 A）**
关闭位置：`ApprovalDecision.Remember`（贯穿 CLI `approve --always` →
protocol/daemon/agent 三层）+ agent `rememberApproval`（Approve && Remember
时从被审批 effect 提取**精确**判据 `rememberRule` → `config.AppendRule`
写 **user 配置**，幂等去重、保留既有、best-effort 不阻断审批）。**裁决**
（两个开问）：①**取 A**（下次生效，不触不变量）——本 run 该审批照常
应答，规则写文件供**下次** session 拼 PermissionLayers 时读到；②**写
user 层**——project 层的 allow 在未 trust 时降级为 ask（决策 #19），写
project 会静默失效，故写 user（恒生效）。**精确匹配**（bash=确切命令、
edit/write=确切路径，不宽通配——`git push` 不放宽成 `git *`）把 user
层"全局"的超范围降到最小。闸门：孪生 TestRememberRule*/TestAppendRule*/
TestRememberedRuleAllowsNextSession + QA-26（真机 UJ-08 全流，捕获并修一个
persist 主路径漏传 Remember 的 bug）。**余项**：project 精确作用域（需
config 加 local 层或 workspace-scoped 规则字段）；取 B 本 run 立即生效
（PolicyChanged，触不变量，留待需求）。原文（历史）：permission rules
只有三个静态来源；审批现场写回路径未设计。
→ UJ-08

**G7 手动上下文操作（compact 带指示、clear） — ✅ 已关闭（INC-6，2026-07-09）**
关闭位置：`protocol.Control{compact|clear}` control 输入（DESIGN §18.2 早
预留）+ `Loop.Controls` 通道 + 安全边界/待命双路 `drainControls` + daemon
`compact`/`clear` 命令 + `ar compact [指示]`/`ar clear` CLI。compact 复用
compactContext（无条件、directive 附加进 summarizer prompt）；clear 复用
ContextCompacted{Summary:""}（assembly 见空 summary 跳摘要头）+ 事件
`Cleared` 标记。**真验捕获并修复一个 bug**：idle 处 compact 时会话以
assistant 消息收尾，Gemini 对"接自己的话"返回空 summary → 空 summary 会
清空上下文；修法 = summarizer 请求补一条 user 收尾消息 + 空 summary 一律
不落 compaction（护上下文）。闸门：TestManualCompact/Clear/
EmptySummarySkipped + QA-12 真实 API。原文：只有自动阈值触发。
→ UJ-09

**G8 运行中 spec 变更（换模型/角色切换） — ✅ 已关闭（2026-07-05,决策 #32）**
关闭位置:SpecChanged 事件 + `ar agent` 命令,用户切换免确认,prefix 显式换代。原文:
spec 冻结于 SessionStarted（对的），但缺一个显式变更事件族（如 ModeChanged
之于 mode）承载换模型、权限面切换。
→ UJ-11（评审→动手的角色切换）

**G9 记忆写回（# remember → CLAUDE.md/项目记忆） — ✅ 写回核心已关闭（INC-14，2026-07-09，取 A）**
关闭位置（写回核心）：`memory.Append(root, note)`（workspace-root CLAUDE.md、
append-only、`## Remembered` 段、同 note 幂等去重防重放双写）+
`protocol.ControlRemember` control（与 compact/clear 同 durable command /
drainControls 家族）+ `Loop.remember`（Append + program-source
`InputReceived` 追加，本会话确认续跑）+ daemon `remember` 命令 + CLI
`ar remember <sid> <text>`。**取 A**（INC-D4）：追加 program 输入本会话即
遵循、文件供**下次** session start 冻结进 prefix——**不动 prefix、不触
不变量**。闸门：孪生 TestMemoryAppend*/TestRememberControl* + QA-23（真
Gemini：remember 写 CLAUDE.md → 新 session 冻结遵循 pnpm 约束）。
**余项（auto-memory 完整体，独立增量）**：MEMORY.md 索引（200 行/25KB）+
主题文件按需读 + per-agent agent-memory + @import + `.claude/rules` 条件
加载（对标 Claude Code，CLAUDECODE-PARITY §2.04/§4.2）；其"压缩后不
consult memory"洞我们天然规避（memory 冻结在 prefix，compact 只动
boundary 后消息，memory 永在——比对方强，记档）。
→ UJ-09

**G16 prompt injection 威胁模型成文 — ⚠️ 设计欠定 · 中(web_fetch 面部分收口)**
硬防线（沙箱/凭据/权限）✅;"workspace 内容不可信"的信任分级未成文。
**web_fetch 面已落硬控(INC-5 安全 review)**:egress 需审批(execute-class,
不静默出网)、link-local/metadata 封禁堵 exfil-to-metadata、重定向逐跳
egress 守卫;`untrusted_content` 软标记降低服从注入概率(不计入 exfil
缓解——真正的防御是 egress 控制)。**余项**:统一的"不可信来源信任分级"
成文条款、BEGIN/END 定界符(现为 JSON 兄弟布尔)、host allowlist(S1)。
→ UJ-20

**G19 hooks 生命周期事件族 — ✅ 第一批已关闭（INC-15，2026-07-09）**
关闭位置（第一批 8 事件）：`hook.RunLifecycle`（复用 runOne：sh -c +
JSON stdin + 凭据剥离 + 超时）+ settings `hooks.lifecycle`（event →
commands，加载期校验事件名，merge 同 pre/post：user 恒生效、project 需
trust）+ loop 各 journal 点位挂 `fireLifecycle`。observe-only =
session_start/session_end/subagent_start/subagent_stop/post_compact/stop
（事实落 journal 后触发，坏 hook 只 warn）；blockable =
user_prompt_submit（exit 2 → 输入不落 journal）/pre_compact（exit 2 →
跳过本次压缩，auto 路径防自旋）。**hooks 不重放**：resume/recovery
settle 不触发。闸门：TestLifecycleHooksFire/TestUserPromptSubmitHookBlocks
/TestPreCompactHookSkipsAndNoSpin/TestObserveHookFailureDoesNotBlock +
QA-24（真 Gemini 四红线）。**余项**：更多事件（Notification/FileChanged/
ConfigChange 类）与 handler 类型扩展（prompt/agent/http）、hook 改写
输入输出（决策 #11 明示推迟）——对照面见 CLAUDECODE-PARITY §2.08
（对方 30 事件 × 5 handler）。**journey 覆盖债仍在**（无 journey 压
hooks，目录修订时裁）。原文（历史）：只有 pre/post tool；session
start/stop、用户输入提交、通知类钩子未设计。
→ （无 journey 覆盖）

**G20 agent 主动提问（wait-class 工具，ask_user） — ✅ 已关闭（INC-5，2026-07-09）**
关闭位置：`internal/tool/defs/ask_user.json`（wait-class def）+ loop 的
park/应答落地（`doTools` 批末 park、`awaitAnswer`、`AskResolved` 配对）。
应答路径 = **inbox 本身**（approve 只答审批的缺口就此补上）：park 期间
一条 `ar send` 经 `AskResolved` 配对为该 call 的 tool result，session
同步续跑,无新 CLI/daemon 动词。免 in-doubt 误杀成立(park 无 activity)。
interrupt/crash-resume/headless 全覆盖(TestAskUser* 六态)。ask park
不再落入 doWait 的 "no resolver" 兜底(该兜底仅留给真正未知的 wait
kind)。→ UJ-06

**G21 自定义命令 / slash 面 — ✅ 已关闭（INC-8，2026-07-09）**
关闭位置：`internal/command` 包（mirror skill）+ DESIGN §10「自定义命令」
子节。定义位 `<root>/.claude/commands/<name>.md`（Claude Code 约定）；
展开语义 = **注入 prompt 文本**、在 **ingest 时**（落 journal 前）于两处
唯一入口（`Loop.Run` 开场 task + `journalInput` 每条 send）展开，
`$ARGUMENTS` 占位替换、fold 保持纯、resume 自包含；与 skills 边界 = 命令
对模型不可见（用户侧宏），skills 是模型侧能力。闸门：TestExpand*/
TestDiscover + 真实 API（new+send 两路展开进 journal 验证）。原文：
协议一行预留；命令的定义位、展开语义、与 skills 的边界未设计。
→ UJ-19

**G29 运行中 mode 切换入口缺失（default↔acceptEdits 用户命令） — ✅ 已关闭（INC-42，2026-07-10）**
关闭位置：`protocol.ControlMode` 入 compact/clear/remember durable command
家族 + `Loop.applyModeControl`（ValidTransition 校验、`ModeChanged{Cause:
"user"}`、非法目标显式 rejected receipt）+ `ar mode` CLI + webui `/mode`
与 pill live 化（含清除 "display only"/"can't change mid-session" 两处
固化遗迹）。闸门：TestModeControl* 孪生 + QA-44 真机（CLI 六红线 + webui
playwright 真用户流）。lint-wiring 当场报 ValidTransition"已接线"并从
deadcode 基线移除——接线的机械证明闭环。原文（历史复盘，保留）：
v1 PLAN S3.6c 白纸黑字的交付物是三条跃迁边:"plan→default(经 ExitPlanMode
审批)、default↔acceptEdits(用户命令)、任意→bypass 仅 CLI 启动时可设"——
但"用户命令"这条边从未接线:`pipeline.ValidTransition` 零生产调用方
（deadcode 可证）,daemon 命令面/CLI/webui 三处均无 mode 命令,webui
Composer 反把缺失固化成注释 "the session's fixed approval mode (display
only)"。已接线的只有 plan→default（exit_plan_mode 审批、原子 fold）与
startup 设定;事件底座全部现成（`ModeChanged` 含 Cause:"user" 枚举、fold、
replay、CLI render）。
**为什么丢了（五道闸皆漏,对策=PROCESS §五登记簿真实性）**:S3.6c 验收锚
窄于交付物（只锚 plan→default 集成测试）→ SPEC 收编以档期名"S2/S3"代锚、
部分完成四舍五入记 ✅ → JOURNEYS 无承载步骤（UJ-06 只有 plan 跃迁）→
PARITY #56 按 mode 清单对表、不按生命周期问"可变更吗"→
TestModeTransitionTable 断言表格三条边全绿,测试绿掩盖零接线。
**关闭路径**:`protocol.Control` 家族加 mode 命令（与 compact/clear/
remember 同 durable command 族）+ `ValidTransition` 校验（bypass 仍拒）+
`ModeChanged{Cause:"user"}` + `ar mode <sid> <mode>` CLI + webui composer
入口;UJ-06 补"中途切 acceptEdits/切回"步骤;QA 场景真机验。缓存无障碍
（mode 只过滤 permitted 面,DESIGN #10）。单独成增量,走三层 delta。
→ UJ-06/11

### 工具与检索面

**G18 内置工具面完整性 — 🟡 部分关闭（write_file ✅；grep/glob ✅ INC-3；web_fetch ✅ INC-5；web search 仍开放）**
DESIGN 列名"file read/**write**/edit、bash、**glob/grep**、**web
fetch/search**"，其中：write_file ✅（v2 M4.3）；**grep/glob ✅（INC-3）**
——独立 read-class 工具，与 semantic_search 共用凭据/vendored 排除谓词
（`index.SkipDir/SkipFile`）、命中过 redaction、per-tool 截断；闸门
TestGrep*/TestGlob* + QA-11 真实 API。**web_fetch ✅（INC-5,2026-07-09,
G18b 关闭）**——execute-class（default 需审批,不静默出网)+ `def.network`
数据位 + 收容 fail-closed + **link-local/metadata 无条件封禁**(安全 review
M1/M2)+ HTML→text + 截断 + redact + untrusted 标记;不变量升级走 §4
(决策 #33);闸门 TestWebFetch*/TestRefuseLinkLocal* + QA-13 + QA-14(真实
coding agent 端到端)。**余项**:**web search**(需外部搜索 API/凭据,或
provider 服务端工具例外类别,单独成增量);host allowlist(S1)= backlog。
→ UJ-01, UJ-05

**G10 子 agent/后台任务实时进度 — ✅ 子 agent 侧已关闭（INC-12.6，2026-07-09）**
关闭位置：成员 loop 继承树根 Out sink 且事件恒带来源 session 标签；
daemon hub 保留非空标签；`ar attach <child-sid>` = 成员 journal replay +
树根 hub 按标签过滤 live；webui 子会话视图接 SSE（按标签隔离打字流,
成员审批全树上浮）;CLI 前台渲染锚定主 session、成员事件折叠。闸门：
TestDaemonAttachChildFiltersLive + QA-20 真实团队会话手验。**余项**：
bash 后台任务的进度 tail（2.10 进度通道,backlog 不变）。原文:
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

**G13 SCM/PR 工作流一等公民化 — ⚠️ 设计欠定 · 中（worktree 子面已关闭 INC-49）**
bash+gh 全程可走（UJ-10 判✅）；缺的是 Codex 式"任务产出=可审阅
diff→批准→PR→元数据回填 session"的组装设计（diff 审阅门、审阅通过
才 push 的约束表达）。积木（artifacts/审批载荷/outputs 契约）都在。
**worktree 一等公民子面 INC-49 已关闭**：New worktree 落稳定共享位置
`~/.local/share/agentrunner/worktrees/<repo>-<branch>-<ts>`、Changes 面板显示
所属 repo/branch、`Apply to project`（git 原生 clean-or-nothing apply-back）+
`Remove worktree`（防呆确认+prune）生命周期闭环。**仍欠定**：diff 审阅门→PR
主体（依赖 G14）、审阅通过才 push 的约束表达。
→ UJ-10, UJ-13

### 驱动与时间旅行

**G23 会话内目标（in-session goal）+ goal 控制面 — ✅ 已关闭（INC-D1，2026-07-09）**
关闭位置：event goal 族（GoalAttached/Updated/Paused/Resumed/Cancelled/
Checkpoint/Achieved）+ state.Goal 子状态 fold + `goal_verify` 静止序列新格
（决策 #24）+ program 源 InputReceived 回灌（state.go:332 天然 fold 进对话）
+ idleOrReturn wake seam（hasInputAfterLastAssistant → 不 idle、续 turn）+
goal 级预算 max_checks=可见截断 + 控制面走 compact/clear 同 out-of-band
通道（`ar goal attach|update|pause|resume|cancel`）+ checkVersions 放宽为
superset（旧会话 resume 不破，R6）。DESIGN 决策 #21/§13/glossary 走不变量
变更流程修订（与实现同 commit）。闸门：孪生 TestInSessionGoalContinuity
（单 SessionStarted 证 context 延续）/BudgetTruncation/PauseCancel；真实
API QA-16。crash 安全：GoalCheckpoint 带 GenStep+Feedback 幂等守卫，resume
不重跑 verifier、不双注入（R1/R2）。**INC-10 补全（同日）**：自证完成
（无 verifier goal 由模型 `goal_complete` 声明、边界裁决——关闭"无
verifier 恒不可达成"的 v0 语义洞）+ goal_status/goal_complete 模型工具面
+ 结构化 continuation + resume/update 注入再武装 + goalResumeCheck 补裁
checkpoint 前 crash 窗 + goal 控制 revive；QA-17 真验；余项 token/墙钟
预算与 blocked/usage_limited 态记档（CODEX-PARITY §6.2-④⑤）。
原始缺口记档（历史）：
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

**G27 task 形态的显式重开 — ✅ 已消解（2026-07-05,静止模型决策 #31）**
（编号更正:本条曾误编 G24,与"团队成员 worktree 快照时机"重号;2026-07-10
更正为 G27。历史 LOG 中 2026-07-05 条目所称"G24 task"指本条。）
"task 形态"概念删除,交付非终点,任何 session 可继续——问题不复存在。原文:
conversational 的显式重开（含已 close）已落地；task session 被 send
重开后的形态未定——升格 conversational 还是原形态续跑、epilogue 是否
重复执行、outputs 重发布语义——暂拒绝（TestSendToCompletedTaskRefused
钉住现状）。
→ UJ-03/09 连带

**G22 监督语义：崩溃自动恢复与重启接续 — ⚠️ 设计欠定 · 中（无人值守形态的地基，2026-07-05 登记；①cron/drive 半兑现 INC-54 2026-07-11）**
已有的一半（全部在，勿重复设计）：恢复语义本身——journal/fold 状态
无损重建、in-doubt 按类别处置、settle-from-child-fold、send 即复活的
journal 形状把关、kill/close/interrupt 的**终态判别**（DESIGN §6/§17，
QA-08 crash 矩阵）。缺的另一半是**自动性**：
① **boot sweep**——daemon 启动无"未完成工作扫描"，中断在 turn 中途
的 session 躺到有人 send 才复活；cron 跨重启唤醒（backlog）同族，
应一并收编；
   **进度（INC-54，2026-07-11）**：cron/drive 的 crash-重启一支落地——
   daemon 启动一次性 `bootSweepDrives` 扫描 running loop-mode drive 并经
   `Driver.Resume` 重挂；cron 的 tick 成 journal 派生事实
   （`IterationScheduled/Skipped.Tick` → `State.LastTick`），resume 从中
   恢复 `lastTick`，错过的 slot 按 overlap 恰好补跑一次（幂等靠 fold 里
   consumed slot + runs 注册去重，不靠内存态）；不越 close 标记（drive 的
   终态 `DriverCompleted` = 其显式结束标记，`scanDriveSessions` 排除，
   决策 #30）。**仍缺**：(a) 中断在 turn 中途的 **agent session** 无 send
   自动接续（本支只做 drive）；(b) **优雅停机保活 cron**——SIGTERM 目前让
   idle loop drive 落 `DriverCompleted "stopped"`（终态），于是优雅重启
   会标 ended、boot sweep 不再重挂；使优雅 deploy 也保 cron 需改 driver
   终态语义（shutdown→待命而非 terminal，区分 shutdown-teardown 与用户
   stop），属 driver 终态语义变更，**另立增量走 DESIGN §四评估**，不在
   INC-54 悄悄改。
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

**G28 树预算下无 cap 子 agent 的预留串行化 — 🟡 engine sharp edge · 中(2026-07-10 真验发现)**
（编号更正:本条曾误编 G25,与已关闭的"弃子回收"重号;2026-07-10 更正为
G28。LOG 2026-07-10 真验条目所称"记 GAPS G25"指本条。）
`spawnAllowance` 对"父有树预算、子无 cap"返回 parentRemaining——一次 spawn
预留父几乎全部剩余预算,lead 下个 LLM 调用被拒(limit_exceeded),团队被串行化、
lead 无法协调(见 UJ-23 web 试跑,LOG 2026-07-10)。无预算(默认)时并行正常。
**workaround**:团队场景不设树预算(交互试跑),或给**每个成员**设 per-member
`budget.max_total_tokens` 使预留有界。**潜在修**:父有预算+子无 cap 时给子一个
默认预算切片(如 parentRemaining/预期扇出)而非全部,或改 reserve-est+settle-up
(弱化 N 并行子的 overspend 保证,需权衡)。单独成增量,走三层 delta。
→ UJ-18/23

**G17 多根 workspace（--add-dir 类） — ❌ 设计缺失 · 低**
单根是各处隐含前提（路径边界/快照/索引）。**当前 24 条 journey 目录
未包含此场景**——保留自旧审计，纳入与否待目录定版。

**G24 团队成员 worktree 快照时机 — ✅ 已关闭（INC-30，2026-07-10）**
真相比初判更深：isolated 子产出**本无回流机制**（"sync-back"不存在，
44c3 里父 workspace 的文件是父 agent write_file 手抄自救），成员 B 的
快照来自父 workspace，队友产出天然不在其中。关闭方式=接通已有设计而
非改隔离语义：①`spawn_agent`/`kill` schema 讲清快照/无回流/制品正道;
②isolated 子开场注入 `[workspace note]`（缺文件即报告,不再空转）;
③webui Dev/Team Lead persona 注明 isolated vs shared 差别（协作用
Team Lead=shared）。闸门：TestIsolatedChildTaskCarriesSnapshotNotice
+ QA-INC30 场景 1/2（isolated 知情转运 12 步 53k tok vs 事故 200k+;
shared 成员直写父树、父零转运）。原文（历史）见上段描述。→ UJ-23

**G25 弃子回收缺失 — ✅ 已关闭（INC-30，2026-07-10）**
关闭位置：`spawn_agent.replaces:<旧 handle>`——启动替代者前经既有
kill(parent) 路径回收前任（未知/已静止幂等 no-op），
`SpawnRequested.Replaces` 留审计；schema 文案引导"重派前回收"；webui
worker spec 步限 40→24 兜底减损。闸门：
TestSpawnReplacesCancelsPredecessor/TestSpawnReplacesUnknownHandleIsNoop
+ QA-INC30 场景 3（真 Gemini 显式指示下 replaces 落盘、sleep 90 的旧
子秒级终止、新子 completed）。裁掉：自动 idle 熔断/父静止杀子（后台
子合法长跑无法与空转区分，须新不变量——留待真实需求）。原文（历史）
见上段描述。→ UJ-23/18

**G26 `ar inspect` children 含重复条目 — 🟡 实现瑕疵 · 低（INC-23 走查）**
被 revive 的 child 每次完成都追加一条 children 记录（同一 call_6_0
出现两次），下游须按 call_id 去重取最新（webui 已做）。→ UJ-23

**G30 SPEC 弱锚存量燃尽 — 🟡 登记簿债务 · 中（2026-07-10 登记）**
`scripts/lint-docs.sh` 落地时,存量 31 行 ✅ 功能点的验收锚只写档期名
（S2/S3/S6/S7 等）,不点名 Test/QA——这是 G29 类缺口的温床:部分交付可以
躲在档期名后面记 ✅。基线 `scripts/spec-anchor-debt.txt` 只减不增
（linter 强制:新增弱锚行拒绝提交,已清偿行必须从基线删除）;每还一行 =
找到真锚,或如实降级状态并挂 GAP。
→ 横切（登记簿本身）

**G31 deadcode 存货甄别 — 🟡 接线审计存货 · 中（2026-07-10 登记）**
`scripts/lint-wiring.sh`（deadcode,main 可达性）基线里 19 个不可达导出
待逐项甄别:`clock.Fake*`/`scripted.New*` 类疑似测试基建（合理,注记后留
基线）;但 `command.Discover`/`parseFrontmatter`、`driver.Resume`、
`agent.ResolveWaitingOnInterrupt`、`mcp.NewConn`、`blackboard.Board.Topics`
等疑似 G29 同类"设计了未接线"或已死。甄别三选一:接线 / 删除 / 基线注记
理由;逐项落 LOG。webui 模块（独立 main,不 import 根模块）当前零死码,
已并入同一 lint。
→ 横切

**G32 纯 Xcode.app 机器 OS 沙箱内 git 不可用 — 🟡 环境兼容缺口 · 中（2026-07-10 登记）**
host 无完整 CommandLineTools（SDK-only 壳不算,xcselect 验证安装）、
xcode-select 指向 Xcode.app 时,Seatbelt 内 /usr/bin/git shim 解析
developer dir 失败（"No developer tools were found"）——profile 只读
白名单不含 /Applications。该形态机器上 bash 工具内 git 整体不可用。
sandbox-exec 忠实复刻 profile 的实验（LOG 2026-07-10）证实候选方案
"放行 /var/db/xcode_select_link + Xcode.app 只读"不成立:放行后 shim
不直接 exec Xcode 内 git,转走 xcrun 完整解析链——要写 per-user
DARWIN_USER_TEMP_DIR（xcrun_db 缓存,无视 TMPDIR env）,并拉起
xcodebuild（fs event stream / result bundle 均被沙箱拒,rc=72）;
所需放行面从"系统只读工具链"膨胀为用户级可写路径 + 系统服务,
且每次 git 调用背一次 xcodebuild 启动税。测试侧已加同沙箱
`git --version` 探测守卫,不可用则 skip 指向本条
（TestBashFilesystemSandboxAllowsLinkedWorktreeGitMetadata）。
环境侧解法:装完整 CLT（`xcode-select --install`）,沙箱内 xcselect
探测 Xcode.app 受阻后回落 /Library 下 CLT（已在白名单）。产品侧闭环
（如 PATH 截击 shim 直指 toolchain git、host 侧 git 代理）触
sandbox env/PATH 语义,须走增量流程另行设计。→ UJ-10/20

**G33 共享环境跑陈旧二进制致新功能假失败 — 🟡 部署/验收流程缺口 · 中（2026-07-10 登记，第二次栽）**
增量在私有 daemon + 私有新二进制上验（QA 纪律要求隔离新 daemon-path
功能），但**收口未把新二进制部署回用户日常共享环境并复验**——共享的
`ar`/daemon/webui 服务端仍是旧二进制。表现:webui 前端 dist 是新的
（Queue|Steer 控件在），调用的共享 `ar` 却是 pre-INC-43，Steer 发消息
`ar send: exit status 2 / flag provided but not defined: -steer`。此为
**第二次**同类事故（首次见 MEMORY「QA 新 daemon-path 功能须私有新
二进制 daemon」）。根因两层:(a) 部署缺一步——增量收口没有"部署回
共享环境"动作；(b) 二进制无版本身份——`ar/arwebui --version` 一律
印 `dev`,新旧不可辨,skew 无从被机械发现。机械加固（随本 bugfix
落地,非新 journey）:①`scripts/deploy.sh` 固化 build→版本化安装（绝不
原地覆盖运行中二进制）→守活跃 turn→重启 daemon/webui;②`-ldflags
-X main.version=<commit>` 给 `ar` 与 `arwebui` 打同一 commit 戳;
③webui 启动 + `/api/health` 做 ar↔webui 版本一致性核对,skew 打
WARNING;④webui send 失败含 `flag provided but not defined` 时把
toast 改写为"ar 二进制过期,scripts/deploy.sh 重新部署"（可诊断,
替代 exit status 2）。测试:TestVersionMatch/TestArFailFlagsStaleBinary。
复盘见 LOG 2026-07-10。→ UJ-13/UJ-16（webui 产品面）

**G34 provider thinking 预算无上限致空消息饿死 — ✅ 已关闭（QA-52，2026-07-11）**
Gemini 的 thought token 从 `MaxOutputTokens` 里扣。旧 gemini provider 在
`req.Thinking.Enabled` 且 budget≤0 时把 `ThinkingBudget` 留空 = "让模型
自己决定"（dynamic，无硬上限），思考可吃光整个 max_tokens，正文/tool
call 颗粒无收 → 红条 `model returned an empty message (truncated at token
cap...)`。前序 508f0e2 只堵了 `!Enabled` 默认思考（发 budget 0），
Enabled 分支的无上限/过大预算两个洞仍在。Anthropic 侧同类：extended
thinking 亦从 max_tokens 扣且要求 `budget_tokens < max_tokens`，旧代码只
floor 到 1024、不按 cap 上钳，过大 budget 非法/饿死。关闭位置：gemini
`resolveThinkingBudget(maxTokens, requested)`——永远发正的、钳过的 budget，
预留 `max(maxTokens/4, 1024)` 给正文；budget≤0 用默认 8192（Gemini 自家
dynamic cap）而非无上限；cap 太小放不下思考时关闭 thinking（budget 0，
整份 cap 给正文）；anthropic 对称 clamp（放不下即不发 thinking）。
loop.go 空消息兜底保留为防御（tool-call 过大等）。与决策 15b 一致
（provider 各自映射 thinking，本条修正 Gemini 映射）。诚实边界：用户
现场 session `20260711-073559-create-a-todo-app-ff36` 的 spec 是
`Thinking:{Enabled:false}`（effort off），那条具体红条是大 tool-call
输出撞 4096 cap（loop.go 已兜底重试恢复），非思考饿死；本条关闭的是
思考饿死向量，真实 API 独立复现（QA-52）。→ UJ-01/UJ-18（provider 层）

**G35 审批「always allow」：spawn_agent 静默不写回 + 同 session 不生效 — ✅ 已关闭（INC-62 + QA-62，2026-07-12；原 ❌ 高，2026-07-11 登记，用户现场）**
用户现场：一个 session 内三次 spawn_agent，webui 审批卡点「始终批准」，
三次全部继续重问。根因两层，且叠加一处 UI 谎报：
(a) **spawn_agent 被白名单静默排除**——`--always` 信号从 webui 按钮
（ApprovalCard→SessionView→`POST .../approve {always:true}`）→
`ar approve --always` → daemon `ApprovalAnswer{Remember:true}` →
`rememberApproval` 七跳全对，最后在
`internal/agent/approval_remember.go` `rememberRule` 的 switch 丢弃：
白名单仅 bash/edit_file/write_file/notebook_edit，spawn_agent 落
default 返回 false——规则永不写入，**下个乃至每个 session 都重问**
（对照 bash 有 TestRememberedRuleAllowsNextSession 证明次 session 直过）。
该审批出自 PermissionGate 的 mode-default ask（execute 类默认审批），
非 SpawnGate/escalation，故规则本可消音；若写回，谓词会是宽泛的
`{tool: spawn_agent, allow}`（rememberRule 只读 command/path，不含
per-child 特定值），能匹配全部子 agent——失败在"没写"，不在"不匹配"。
(b) **同 session 内本就不生效是决策 #38 取 A 的既定语义**（写 user 层、
冻结 PermissionLayers 本 run 不可变）——但**用户已裁定：同 session 内
「始终批准」必须生效，这是最基本的 UX 要求**。取 A 不满足该需求，
决策 #38 需扩展/修订（候选：审批 broker/loop 层的 session 级
auto-approve 记忆，不动冻结层不变量；或走不变量变更流程改层冻结语义）。
(c) **webui toast 无条件谎报**（SessionView.tsx `always&&approve` 即
toast「已保存精确 allow 规则，此调用不会再询问」）——不查写回结果；
spawn 路径连 journal 的 remembered 提示都不会落。
登记簿失真同修（G29 族）：SPEC「审批答复写回规则」行原 ✅ 无覆盖面
限定、锚测试仅 bash/edit 面（TestRememberRuleFromEffect 甚至把
"其他 execute 类不记忆"断言为正确），已降 🟡 指向本条。修复面：
①rememberRule 补 spawn_agent（谓词范围待定：tool 级宽泛 vs 按 agent
名限定）；②同 session 生效机制（需增量流程，触决策 #38）；③toast
依据真实写回信号；④补 spawn 面锚测试与 QA 场景后方可回 ✅。
**收口（INC-62，2026-07-12，用户裁定方案一=审批层常设应答）**：
①spawn_agent 入 `standingCriterion` 白名单（tool 级——用户意图即"别再
为起子 agent 问"，且 PermissionRule 无 agent 维度）；②同 session 生效
落地为 standing approval（判据随 `ApprovalResponded.Standing` 落
journal → fold `Effects.Standing` → `requestApproval` 同判据免问直落
`EffectResolved{allow}`；不触层冻结，决策 #38 扩展非推翻，详
INC-62 工作纸）；③webui toast 改为只声明 always 意图（"本会话对同类
操作不再询问"），写回成功与否以 loop 的 `remembered:` 流消息为权威；
④锚：TestStandingApprovalSameSession/SpawnAgent/SurvivesResume +
TestPlainApproveDoesNotStand + TestRememberRuleFromEffect（spawn 行）。
**gate B 已绿（2026-07-12）**：QA-62 于 GitHub Actions runner 以真
Gemini 跑通 5/5（run #2，commit 26e0178）——3 spawn 恰 1 ask、standing
判词在案、user 配置得 spawn_agent 规则、新 session 零 ask；journal 导出
存 workflow artifact `qa62-run`。附带收获两枚 QA 基建教训（run #1 假绿
复盘，已修）：①`grep -c || echo 0` 零匹配输出两行值，整数守卫报错被
静默跳过（QA-26 同款潜伏已一并修）；②Actions 默认 shell 无 pipefail，
`| tee` 吞脚本退出码——workflow 步骤须显式 `shell: bash`。本条与 SPEC
行同步回 ✅。
→ UJ-08/UJ-18

**G36 webui 移动/错误 UX 缺口批 — 🟡 大部已修，余项低优先（2026-07-12，用户手机现场 + 两路审计 + 黑盒 QA）**
用户从手机（phone-webui）连续现场暴露一类缺陷 + 两路 audit agent（webui
后端/前端）+ 真浏览器黑盒 QA 系统清扫。**根因**：api.ts 把 ApiError.message
拼成 error+"\n"+stderr,~40 处 toast 全甩原始 git/CLI 输出("吓人红 toast"
类)；配套后端多处 `badRequest` 直接回 git/CLI 原文或 server-CWD resolve
后的绝对路径。**已修（fcc1547/e0d5d7a/2fadd8c/bb0c705/c0ec63a）**：①api.ts
message 只取友好 error、stderr 落 .details；②后端 daemon-down→503+code
+友好文案（原每次点击甩"is the daemon running"）、resolveWorkspace 统一
校验、git commit 干净树/checkout 分类/worktree ref-repo 友好化、worktree
unborn-branch(hasCommits)守卫、workspace bare 名守卫；③Toasts 死 CSS 复
活+safe-area+error 不自动消失；④iOS：输入 focus 16px 消自动缩放、PWA 可
安装、拦下拉刷新整页重载、safe-area、tap-highlight/:active/touch-callout/
44px 命中盒、visualViewport 键盘避让；⑤row-flex wrap、RunModal 标题、
sa-name ellipsis、DiffView 失败重试。**验收**：前端 306 测试；真浏览器
黑盒 `qa-blackbox`（qa/blackbox/drive.mjs，手机+桌面双端、真 Gemini turn、
daemon-down journey，机器判据=无 console 错误/无原始错误文案/无横向溢出/
逐步截图）run #4 首次全绿（4/4，2026-07-12）。**余项（低）**：schedule
interval/cron 内联校验、错误 .details 披露 UI、更多 Settings/审批卡交互
黑盒覆盖——待排期,非阻塞。三轮黑盒真机产品级新发现已归零(曲线压平)。
→ UJ-01/UJ-24（webui 产品面）

---

## §3 已确认覆盖（防重复登记）

对照 24 条 journey 核实、设计与实现俱在：编辑-执行闭环与失败自纠
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
