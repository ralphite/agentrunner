# AgentRunner — 功能点登记簿（SPEC，第二层）

**这是什么**：产品功能点的**正面清单**——从 JOURNEYS.md 的 journey
拆出，按功能域组织。每条登记：状态、journey 来源、验收锚（哪条测试
证明它 work）。镜像的负面清单（缺口审计）见 GAPS.md；功能点如何成立
见 DESIGN.md（实现偏差以其 §17 为准）。

**维护纪律**（PROCESS.md §二）：增量落地时更新对应条目；验收锚必须
指到真实存在的测试；状态变化不删行、只改标。

**状态图例**：✅ 已实现且有验收锚 · 🟡 部分实现（备注列缺口）·
❌ 未实现（引 GAPS 条目）· 🧊 显式推迟/裁掉（有记档）。

**验收锚缩写**：`QA-xx` = qa/run-qaxx.sh（真实 API，QA.md 菜单）；
`C1–C10` = DESIGN §16 核心验收（scripted 孪生）；`S1–S7` = v1
acceptance 26 场景（e2e/，按阶段）；具名测试 = Go 测试名。

---

## A · 会话与输入（内核）

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| 续聊（答完待命；close = 标记） | ✅ | UJ-01/03/09 | QA-01 · C1 · 孪生（见 e2e） |
| 回复就地可见（`new`/`send` 默认跟随本轮渲染正文至 idle，尾行提示 send/attach；`--detach` 恢复异步） | ✅ | UJ-01/03 | INC-2 · TestNewAndSendRenderReply/Detach |
| 忙时投递排队（安全边界按序消费，不丢不乱序） | ✅ | UJ-07 | QA-02/06 · C2 |
| durable CommandLog（send/control/close/interrupt/approval/kill；command_id 幂等；principal/source/trust；确认即 accepted，跨 restart 自动重放） | ✅ | 不变量 | INC-11.2/11.5 · TestInboxCommandIdempotency/TestInboxAppendRead/TestStartupResumesAndReplaysPendingDurableCommand（DESIGN §2） |
| Turn/Item 交互投影（message/tool_call/tool_result；旧 Message/GenStep 日志兼容补投影） | ✅ | 不变量 | INC-11.5 · TestTurnItemProjectionPreservesTypedIngressAndToolItems/TestLegacyMessagesSynthesizeStableTurnItemsWithoutMutatingPriorState |
| typed ingress（text/image/file + principal/source/trust，CAS 后 ref-only 入 journal） | ✅ | UJ-01/04/12 | INC-11.5 · TestJournalInputPreservesTypedContentAndProvenance；`inspect --json` 暴露 turns/items/provider envelope |
| 静止模型（唯一 session 形态；静止=形状 `state.Quiescence`；静止动作 outputs→barrier→parent 回执；close/kill=标记+检查；预算耗尽=可见截断） | ✅ | 不变量 | 决策 #30/#31 · 2026-07-08 落码(D2) · TestResumeQuiescentIsLawful/TestQuiescentSequenceOrder/TestBackgroundTaskSettlesBeforeQuiescence |
| interrupt 与输入分立（Esc 杀活动 / 消息追加） | ✅ | UJ-07 | QA-02/06 · C8 · S3 |
| interrupt 永不结束 session（待命处 = no-op；close 是独立命令） | ✅ | UJ-03/07 | 裁决 #11 · 2026-07-08 落码(D2) · TestIdleInterruptIsNoOp |
| 图片输入（`ar send --image`，CAS ref、组装 inflate） | ✅ | UJ-04 | QA-07/03 · C9 · TestConversationalImageInputEndToEnd |
| 长贴折叠（>10KB 转 file part） | ✅ | UJ-04 | TestLongPasteFoldsToFilePart |
| `ar new` 开场消息折叠/带图（与 send 对称） | 🧊 | UJ-04 | 不对称记档（DESIGN §17），待真实使用反馈 |
| PDF/任意文件附件（`ar send --file`，sniff MIME、CAS ref、组装 inflate；Gemini inline_data / Anthropic document block） | ✅ | UJ-04 | INC-9 · TestConversationalFileInputEndToEnd/TestToPartFilePDF/TestUserBlocksFilePDF · QA-15（真实 Gemini 读 PDF 关键词） |
| provider capability envelope（版本、provider/model、modalities、stream/tools/thinking/cache/parallel） | ✅ | 不变量 | INC-11.5 · TestCapabilitiesMatrix；SessionStarted 冻结、inspect 可见 |
| 外部事件唤醒既有 session（webhook → inbox，机器发送方） | ❌ | UJ-12 | GAPS G14（inbox 原语已备，缺投递壳） |
| WAITING_APPROVAL 挂起期间消息唤醒 | 🟡 | UJ-07 | 只排队不唤醒；GAPS G3 余项 |
| 手动 compact（带指示）/ clear | ✅ | UJ-09 | INC-6 · TestManualCompact/Clear/EmptySummarySkipped · QA-12（真实 API：compact 带指示落非空 summary、clear 落 cleared） |
| 自动 compaction（阈值触发） | ✅ | UJ-09 | S3 |
| microcompact（assembly 把久远可重算 read-class 工具结果渲染为占位符；不调 LLM；单调 micro boundary；先于 autocompact） | ✅ | UJ-09 | INC-13 · TestMicrocompact{AssemblyView,MonotonicFold,TriggeredInLoop,DisabledNoop} · QA-22（真 Gemini：micro 触发、无 compact、模型重跑工具复原被清结果） |
| 手动 barrier 打点（`ar barrier`，非运行中 session） | ✅ | UJ-15 | fork 全链路测试（S7 收口） |

## B · 子 agent 与编排

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| 后台 spawn（非阻塞拿 handle，`spawn_agent{background}`） | ✅ | UJ-18 | QA-04 · C3 |
| 静止回执激活父 turn（先回先处理,可多次;投递模式 receipts: steer 默认/turn_end,spec 层） | ✅ | UJ-18 | QA-04/05 · C4 · 裁决 #15 · TestReceiptsModeControlsSettlementTiming |
| 杀死子 agent（`kill` 工具 / `ar kill`,标记记来源 user/parent;用户 kill 的仅用户可复活） | ✅ | UJ-18 | QA-05/09 · C5 · 裁决二 C · TestKillLeavesSourcedMark |
| steer 改变编排（杀一个、起一个） | ✅ | UJ-07/18 | QA-06/09 · C6 |
| 完整编排七步（多输入+并行+杀+回灌+续聊+恢复） | ✅ | UJ-18 | QA-09 · C7 |
| 父崩溃 settle-from-child-fold | ✅ | 不变量 | QA-08(c) · C10(c) |
| spawn 一律非阻塞（阻塞路径已删除,零 legacy） | ✅ | UJ-18 | 2026-07-08 落码(D3a) · TestSpawnEndToEnd(后台形态) · s5 场景 routes 化 |
| handoff（`handoff_agent`）/ blackboard（`publish_note`/`read_notes`） | ✅ | UJ-18 | S4 |
| 树预算 / 权限默认不超父（冻结交集） / 深度扇出上限 | ✅ | UJ-18/20/23 | S4 · INC-12.5 · TestEscalationApproval（预算/上限/收容无例外） |
| 子提权申请通道（`escalate` 强制人审；批准用 child rules，拒绝/interrupt 降级交集） | ✅ | UJ-23 | INC-12.5 · TestEscalationApproval |
| 动态角色（`agents_dynamic` + inline role；冻结 RoleSpec；无 hooks/MCP/skills，工具仅父子集） | ✅ | UJ-23 | INC-12.4 · TestSpawnDynamicRole |
| 树内 durable 消息 / 静止子唤醒 / 用户直达子 steer | ✅ | UJ-18/23 | INC-12.1–12.3 · TestSendMessage*/TestRevive*/TestSendForwardsTargetToChild/TestDaemonSendToChildRoutesThroughRoot · QA-20（真 Gemini 团队协作+revive） |
| durable team task/DAG/lease + workspace assignment（生产默认隔离 worktree，显式 shared；revive 复用） | ✅ | UJ-16/18/23 | INC-11.6 · TestIsolatedTeamWorkspaceSurvivesRevive/TestTeamTaskDependencyPlan |
| 子 agent 实时进度镜像（成员事件带 session 标签入树根 hub;`ar attach <child-sid>` live 过滤;webui 子会话 SSE;CLI 前台锚定折叠）/ 子审批根宿主路由与 crash 重挂接 | ✅ | UJ-18/23 | INC-12.6 · TestDaemonAttachChildFiltersLive/TestCrashResumeReattachesApprovalWaitingChild · QA-20(G10 关闭) |
| 子执行收敛为递归 session | 🟡 | — | 阻塞路径已删(D3a);driver 独立待收敛,挂 UJ-22/G23（DESIGN §17） |
| 并发子 agent 确定性测试基建（routing provider） | ✅ | — | C3–C7 孪生在用（GAPS G4 关闭事实） |

## C · 工具面

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| read_file / write_file / edit_file | ✅ | UJ-02/05 | S1 · QA-03（write_file） |
| bash 前台+后台（`output`/`kill` 凭 handle、进程组取消） | ✅ | UJ-02/18 | S1/S3 · QA-05 |
| semantic_search（IndexStore，BM25） | ✅ | UJ-01 | S7 |
| publish_artifact（`outputs:` contract、审批载荷） | ✅ | UJ-06 | S5 |
| exit_plan_mode（plan mode 跃迁） | ✅ | UJ-06/11 | S2/S3 |
| schedule_next / finish_series（loop 自定步调） | ✅ | UJ-14 | S6 |
| grep / glob 独立工具 | ✅ | UJ-01 | INC-3 · TestGrep*/TestGlob* · QA-11（真实 API：模型自发调用 grep+glob，凭据红线守住） |
| web_fetch（客户端执行,**execute-class**;HTML→text、重定向/大小上限、`network` 数据位、收容 fail-closed、**link-local/metadata 无条件封禁**、untrusted 标记;安全 review M1/M2 已对齐,host allowlist S1 待裁/backlog） | ✅ | UJ-01 | INC-5 · TestWebFetch*/TestRefuseLinkLocal* · QA-13 · QA-14（真实 coding agent 端到端） |
| web search | ❌ | UJ-01 | GAPS G18 余项（搜索后端选型 / provider 服务端工具例外类别，单独成增量） |
| ask_user（wait-class 提问：park WAITING_INPUT，应答走 inbox→配对 tool result，同 session 续跑；一批限一问、interrupt/crash/headless 全覆盖） | ✅ | UJ-06 | INC-5 · TestAskUser*（park/answer/reject/interrupt/headless/crash-resume）· QA-13 |
| finish（结束 turn 让 session 待命） | 🧊 | UJ-06 | 记档不预做（DESIGN §17：待命本身就是待命） |
| tool 输出截断（per-tool 上限） | ✅ | 不变量 | S3 |

## D · 权限与安全

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| rules（tool/path/command/network + realpath 归一） | ✅ | UJ-08/20 | S2 · S7（network） |
| modes（default/plan/acceptEdits + bypass 不跳 hooks） | ✅ | UJ-06/11 | S2/S3 |
| 审批流（ask → WAITING_APPROVAL → 应答/拒绝理由回灌） | ✅ | UJ-08 | S2 · 远程审批 S6 |
| hooks（pre/post，observe+block） | ✅ | UJ-19 | S2 |
| OS 沙箱（bash/verifier 默认 filesystem=workspace；Seatbelt/Bubblewrap；network none 棘轮；能力缺失 fail-closed） | ✅ | UJ-20 | INC-11.3 · TestBashFilesystemSandbox/TestBashNetworkContainment/TestSandboxCapabilityMissingDeniesBeforeActivity |
| 凭据 redaction + 硬排除表（含 .netrc/.npmrc 等） | ✅ | UJ-20 | S2/S7 收口 |
| 信任模型（project 层 hooks 需显式 trust，`ar trust`） | ✅ | UJ-20 | S2 |
| 审批答复写回规则（"允许且不再问"） | ❌ | UJ-08 | GAPS G5（PolicyChanged 事件已设计） |
| prompt injection 威胁模型成文 | 🟡 | UJ-20 | GAPS G16（硬防线在，条款未成文） |
| hooks 生命周期事件族（8 事件：session_start/end、user_prompt_submit、stop、subagent_start/stop、pre/post_compact；observe+block，blockable=user_prompt_submit/pre_compact；settings `hooks.lifecycle`，事件名加载期校验；hooks 不重放） | ✅ | — | INC-15 · TestLifecycleHooksFire/TestUserPromptSubmitHookBlocks/TestPreCompactHookSkipsAndNoSpin/TestObserveHookFailureDoesNotBlock · QA-24（真 Gemini：四红线） |

## E · 持久化与恢复

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| journal + 纯 fold + indexed snapshot-resume（offset/hash 校验、真尾读；索引可重建） | ✅ | 不变量 | INC-11.7 · TestIndexedCursorReadsOnlyTailAndRejectsMismatch/TestSnapshotTailEquivalence |
| schema 兼容 reader（additive/旧 namespace 子集；缺新投影的旧 snapshot 自动全量 fold；破坏性冲突拒绝且不改源） | ✅ | 不变量 | INC-11.7 · TestSchemaGuardAcceptsOlderNamespaceSubset/TestResumeFullFoldsLegacySnapshotMissingNewProjection |
| in-doubt 按类别处置（LLM 重发/只读重跑/执行不重跑） | ✅ | 不变量 | S2 · QA-08(b) |
| crash 矩阵三态复活（idle/在飞 bash/在飞子 agent） | ✅ | UJ-09 | QA-08 · C10 |
| 显式重开（send 对任何 session 成立，含带标记的；自动路径受标记约束） | ✅ | UJ-09/03 | TestSendReopensMarkedSession · TestAutomaticResumeSkipsMarkedSession · TestSendRevivalDiesWithDaemon |
| 恢复单一自愈（in-doubt 处置后渲染 interrupted-by-crash,session 继续） | ✅ | 不变量 | QA-08 · 决策 #29(2026-07-05 单一化) |
| workspace 快照（shadow repo、排除表、pinned） | ✅ | UJ-15 | S2/S7 |
| daemon kill -9 后孤儿 bash 子进程清扫（pgid） | 🟡 | — | 记档观察项（DESIGN §17） |
| shadow repo 并发 flock（daemon 多 session） | 🧊 | — | backlog（加 gc 前必须先做，LOG/v2 台账记档） |

## F · 驱动（one-shot / goal / loop / best-of-N）

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| driver-goal（批式/headless，verifier 三态、停滞检测、carry；fresh child run） | ✅ | UJ-15 | S6 |
| **in-session goal（会话内，context 延续；完成裁决在静止边界：command verifier 唯一裁决且经过 effect pipeline/approval/OS sandbox，或无 verifier 时模型 `goal_complete` 自证；结构化 continuation 回灌、goal 级预算=可见截断；模型工具面 goal_status/goal_complete；控制面 attach/pause/resume/update/cancel，非 hosted 会话 revive）** | ✅ | UJ-22 | INC-D1+INC-10+INC-11.3 · 决策 #21/#34 · TestInSessionGoal{Continuity,SelfCertify,ClaimDoesNotOverrideVerifier,VerifierPipelineDenyBinds,VerifierCompletedResultIsNotRerun} + TestGoalRecover/TestGoalResumeCheck/TestGoalAttachRevivesSession · QA-16/17 |
| loop mode（interval/cron/self_paced、overlap skip/coalesce） | ✅ | UJ-14 | S6 |
| verifier 管线化（in-session/driver 均 journaled effect + Activity bracket + containment evidence；driver-trust 规则层） | ✅ | UJ-15/22 | S7 · INC-11.3 · TestVerifierActivityTrace |
| best-of-N（隔离 worktree、per-attempt 判定、胜者留盘） | ✅ | UJ-16 | S7 |
| overlap: interrupt | 🧊 | UJ-14 | backlog（与顺序执行同理推迟） |
| 胜者晋升（fork / apply diff） | 🧊 | UJ-16 | GAPS G15（v0 用户手动晋升，记档） |
| cron 跨重启唤醒 | 🧊 | UJ-14 | backlog |

## G · 时间旅行（barrier / fork / rewind）

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| CheckpointBarrier（安全边界/task 收尾/手动，向量+快照 ref） | ✅ | UJ-15 | S7 |
| fork（单创世、处置向量落实、随行库复制、独立 worktree） | ✅ | UJ-15 | S7 收口 review 修复 + fork-of-fork 测试 |
| rewind（fork 后显式切换） | ✅ | UJ-15 | S7 |
| 多模态 blob 在 fork/rewind 下的归属语义 | 🟡 | — | GAPS G1 余项 |
| barrier tasks 对"任务=子 agent"的处置语义 | 🟡 | — | GAPS G2 余项 |

## H · 生态接入

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| MCP（stdio/streamable HTTP、schema/list_changed、断连恢复、写审批） | ✅ | UJ-19 | INC-11.4；spec→所有 Loop 生产入口自动接线 |
| MCP resources/prompts、structured/multimodal result | ✅ | UJ-19 | INC-11.4；namespaced protocol tools，内容块保真 |
| MCP HTTP OAuth bearer（env 引用） | ✅ | UJ-19 | INC-11.4；token 不进 spec/journal |
| MCP 交互 OAuth 登录 / refresh-token 持久化 | 🧊 | UJ-19 | 凭据 UX；runtime 不持久化 secret |
| skills（Claude Code 约定） | ✅ | UJ-19 | S5 |
| memory 文件读侧注入（CLAUDE.md 层级合并） | ✅ | UJ-09 | S3 |
| 记忆写回（`ar remember`，append 项目 CLAUDE.md；取 A：追加 program 输入本会话即遵循，文件供下次 session 冻结） | ✅ | UJ-09 | INC-14 · TestMemoryAppend*/TestRememberControl* · QA-23（真 Gemini：写 CLAUDE.md → 新 session 冻结遵循 pnpm 约束） |
| 自定义命令 / slash 面 | ✅ | UJ-19 | INC-8 · TestExpand*/TestDiscover · 真实 API（`.claude/commands/*.md` 的 `/name` 在 new+send 两路展开进 journal） |

## I · 观察与远程面

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| events / inspect（时间线、判定、子树、用量） | ✅ | UJ-17 | S3/S6;INC-11.1 按 stream header 分派 run fold / driver fold，旧 goal/loop journal 可读并展开 iteration 子树；子会话寻址(child_session 全 id,`-sub-` 分段映射 `sub/` 目录,任意深度)INC-1 |
| `ar ps`（fold 的在飞任务列表，无 daemon 可用） | ✅ | UJ-18 | QA-05/09 实测 |
| attach/detach（journal 补读 + live 订阅） | ✅ | UJ-17 | S6 |
| 远程审批（daemon approve） | ✅ | UJ-17 | S6 |
| notifier（生命周期通知、跨重启去重） | ✅ | UJ-14 | S6 |
| 远程 stop command | ✅ | UJ-17 | INC-4 · TestStop*（daemon 孪生）· 手验（真 daemon：stop 拆 run、无标记、send 复活）；drive 系列亦可 stop（handleDrive 加 per-run cancel） |
| HTTP/WS 壳 | 🧊 | UJ-13 | backlog |

## J · 运行形态与云

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| daemon 托管（socket、run idem_key + session command_id 幂等、优雅停机） | ✅ | UJ-17 | S6 · INC-11.2 |
| CLI 第一公里可发现性（顶层 help/`init` 示例 spec/README/spec 错误附字段清单/daemon 报错附启动指引） | ✅ | UJ-01…（全 journey 进入门槛） | INC-2 · TestTopLevelHelp/TestInit* · spec_errors golden |
| 静止动作（outputs→barrier→goal_verify→parent 回执;ar run=开+发+等静止+读结果） | ✅ | UJ-02/14/15/22 | 决策 #31 · 2026-07-08 落码(D2) · INC-D1/INC-11.1 同步顺序测试 · TestQuiescentSequenceOrder · acceptance events_valid 改静止形状判定 |
| session 内换 agent（SpecChanged 事件 + `ar agent`,用户免确认,prefix 显式换代） | ✅ | UJ-11 | 裁决一 · 2026-07-08 落码(D4a) · QA-10 · TestAgentSwitchTakesEffectOnResume（G8 关闭） |
| 云 workspace 生命周期 | 🧊 | UJ-13 | GAPS G11（S7 预授权裁掉，重启走新增量） |
| IDE 集成 | 🧊 | — | 同上裁决 |
| 多根 workspace（--add-dir 类） | ❌ | — | GAPS G17（待 journey 目录定版） |

---

## 附录 · 代码事实对照（2026-07-05 盘点）

**CLI 子命令**（`internal/cli/cli.go`）：
`run` `drive` `submit` `resume` `new` `send` `close` `interrupt`
`stop`（INC-4）`compact`（INC-6）`clear`（INC-6）`remember`（INC-14）`kill` `agent`（决策 #32）`ps` `approve` `fork` `barrier`
`sessions` `trust` `attach` `daemon` `events` `inspect` `accept`
`record-fixture` `version` `help` `init`（INC-2）

**daemon 线协议命令**（`internal/daemon/daemon.go`）：
`ping` `run` `drive` `attach` `approve` `send` `close` `interrupt`
`stop`（INC-4）`compact`（INC-6）`clear`（INC-6）`remember`（INC-14）`kill` `agent`

**内置 tool 定义**（`internal/tool/defs/*.json`）：
`read_file` `write_file` `edit_file` `bash` `output` `kill`
`spawn_agent` `handoff_agent` `publish_artifact` `publish_note`
`read_notes` `semantic_search` `grep`（INC-3）`glob`（INC-3）
`exit_plan_mode` `schedule_next` `finish_series`
