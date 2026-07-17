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
| 忙时投递排队（安全边界按序消费，不丢不乱序；默认 queue=下个 turn，steer 在 turn 内安全边界即消费——INC-43） | ✅ | UJ-07 | QA-02/06 · C2 |
| 运行中发消息投递模式（per-message `Delivery`：steer=当前 turn 下个安全边界注入 / queue=下个 turn，默认 queue；CLI `ar send --steer`、webui composer Queue\|Steer 切换 + ⌘⏎ 反选，对标 Codex） | ✅ | UJ-07 | INC-43 · TestSteerDeliversMidTurn/TestQueueDefersToTurnEnd/TestSteerFlushesQueuedBacklog/TestInboxDeliveryModeIsPartOfPayload · QA-45 |
| durable CommandLog（send/control/close/interrupt/approval/kill/**revoke**；command_id 幂等；principal/source/trust；确认即 accepted，跨 restart 自动重放；revoke 语义见 §2 撤回条款） | ✅ | 不变量 | INC-11.2/11.5 · TestInboxCommandIdempotency/TestInboxAppendRead/TestStartupResumesAndReplaysPendingDurableCommand（DESIGN §2）· INC-46 |
| 排队消息撤销（`ar queue` 列 pending+revoked 态 / `ar unqueue <sid> <cmd-id>` / daemon `unqueue`；live=revoke 通道+revoked 集+journalInput 消费前查集，resume=重放读全量 CommandLog 先跳被撤；`InputRevoked{target,delivery_seq}` 推 high-water（AskResolved 同模板）；迟到 no-op；仅 input 可撤；CLI 前置校验=UX 早拒非安全边界） | ✅ | UJ-07/24 | INC-46（HANDA #29，INC-44 §A rev1）· TestRevokedInputSkippedOnResume/TestLiveRevokeConsumesQueuedInput · 真验 2026-07-11（真 Gemini：忙时排队撤回+kill -9 重放不翻案 KEPT×4/DROPPED×0，qa/runs/2026-07-11-INC46）；webui 撤回按钮拆余项随 #7 |
| 外部事件唤醒/webhook ingress（`ar daemon --http <addr>` 起 `POST /hooks/<id>`（默认关，绑定地址落 `<data>/daemon.http`）；`ar hook create <sid> [--name]`/`list`/`revoke`——per-hook id+token，token 一次性打印仅哈希落盘不进 journal；载荷 source:"machine"+trust:"untrusted"+principal:"hook:<n>" 走同一条 durable send 通道；loop 侧强制隔离框定+trust 钳制+不做宏展开；未鉴权限流 429/body 上限 413/无存在性 oracle；machine 不越 close/kill 标记（410）、unmarked parked 正常 revive；`X-Command-Id` 幂等重投） | ✅ | UJ-12/13 | INC-50（HANDA #E2，G14，决策 #39，兑现 INC-D2）· TestHookIngress{DeliversMachineInput,AuthAndRateLimit,CannotReviveMarkedSession,BodyCap,IdempotentRedelivery}/TestHookRegistryHashesAndRevokes/TestMachineInputFramedAndTrustClamped · QA-50 |
| Turn/Item 交互投影（message/tool_call/tool_result；旧 Message/GenStep 日志兼容补投影） | ✅ | 不变量 | INC-11.5 · TestTurnItemProjectionPreservesTypedIngressAndToolItems/TestLegacyMessagesSynthesizeStableTurnItemsWithoutMutatingPriorState |
| typed ingress（text/image/file + principal/source/trust，CAS 后 ref-only 入 journal） | ✅ | UJ-01/04/12 | INC-11.5 · TestJournalInputPreservesTypedContentAndProvenance；`inspect --json` 暴露 turns/items/provider envelope |
| session 标识与 store 边界（64-bit 随机后缀、熵源失败 fail closed；CLI 只解析合法 basename/prefix，拒绝 `..`、final/intermediate symlink 越界；旧 4-hex ID 仍可读） | ✅ | UJ-01/17/24 | INC-67 · TestNewSessionID/TestSessionDirRejectsUnsafeID/TestResolveSessionDirRejectsTraversalAndSymlinkEscape · QA-67 |
| 静止模型（唯一 session 形态；静止=形状 `state.Quiescence`；静止动作 outputs→barrier→parent 回执；close/kill=标记+检查；预算耗尽=可见截断） | ✅ | 不变量 | 决策 #30/#31 · 2026-07-08 落码(D2) · TestResumeQuiescentIsLawful/TestQuiescentSequenceOrder/TestBackgroundWorkSettlesBeforeQuiescence |
| interrupt 与输入分立（Esc 杀活动 / 消息追加） | ✅ | UJ-07 | QA-02/06 · C8 · S3 |
| interrupt 永不结束 session（待命处 = no-op；close 是独立命令） | ✅ | UJ-03/07 | 裁决 #11 · 2026-07-08 落码(D2) · TestIdleInterruptIsNoOp |
| 图片输入（`ar send --image`，CAS ref、组装 inflate） | ✅ | UJ-04 | QA-07/03 · C9 · TestConversationalImageInputEndToEnd |
| 长贴折叠（>10KB 转 file part） | ✅ | UJ-04 | TestLongPasteFoldsToFilePart |
| `ar new` 开场消息折叠/带图（与 send 对称） | 🧊 | UJ-04 | 不对称记档（DESIGN §17），待真实使用反馈 |
| PDF/任意文件附件（`ar send --file`，sniff MIME、CAS ref、组装 inflate；Gemini inline_data / Anthropic document block） | ✅ | UJ-04 | INC-9 · TestConversationalFileInputEndToEnd/TestToPartFilePDF/TestUserBlocksFilePDF · QA-15（真实 Gemini 读 PDF 关键词） |
| provider capability envelope（版本、provider/model、modalities、stream/tools/thinking/cache/parallel） | ✅ | 不变量 | INC-11.5 · TestCapabilitiesMatrix；SessionStarted 冻结、inspect 可见 |
| WAITING_APPROVAL 挂起期间消息唤醒 | 🟡 | UJ-07 | 只排队不唤醒；GAPS G3 余项 |
| 手动 compact（带指示）/ clear | ✅ | UJ-09 | INC-6 · TestManualCompact/Clear/EmptySummarySkipped · TestHandleCompactForwardsDirective · QA-12（真实 API：compact 带指示落非空 summary、clear 落 cleared） |
| 自动 compaction（阈值触发） | ✅ | UJ-09 | S3 |
| microcompact（assembly 把久远可重算 read-class 工具结果渲染为占位符；不调 LLM；单调 micro boundary；先于 autocompact） | ✅ | UJ-09 | INC-13 · TestMicrocompact{AssemblyView,MonotonicFold,TriggeredInLoop,DisabledNoop} · QA-22（真 Gemini：micro 触发、无 compact、模型重跑工具复原被清结果） |
| 手动 barrier 打点（`ar barrier`，非运行中 session） | ✅ | UJ-15 | fork 全链路测试（S7 收口） |
| stdin 管道文本（`run/new/send` 文本参数缺省且 stdin 为管道时读取，显式 `-` 占位；非管道下 `-` 报错不阻塞；仅尾部换行 trim；附件 flags 不受影响） | ✅ | UJ-01/02 | INC-28（HANDA #32）· TestCompleteTextArg*/TestRunCmdPipedPromptSkipsUsage · 真验 2026-07-10（真 Gemini：管道开场+`-` 多行续聊，qa/runs/2026-07-10-INC28） |
| retry（conversation：`ar retry <sid>`/webui 重发最后一条 user-class 输入为新 turn，payload 纯函数重组、派生 command_id 幂等；driver：Scheduled Retry 从旧 `DriverStarted` 的 spec/workspace/spec_path **新建 series**，绝不向 driver journal 注入聊天消息） | ✅ | UJ-02/14/24 | INC-45+INC-66 · TestPlanRetryTargetsLastUserInput/TestRetryAttachmentsRoundTripCAS/TestParseDriverRetryInfo/TestCLIResumeDriverReportsDomainError |
| 服务端语音听写（`ar dictate <audio>`：provider `PartAudio` → Gemini inline_data 转写；`--context` 消歧专有名词/中英混合；`--max-bytes` 上限；webui 薄壳录音上传经 ar、SpeechRecognition fallback；**文本便利非模型 audio 模态**——loop 不组装 audio part、`InputModalities` 不含 audio） | ✅ | UJ-01/04 | INC-56（HANDA #18）· TestToPartAudio/TestDictateEncodesAudioPartAndContext/TestDictateRejectsOversizeAudio/TestHandleDictateRejectsNonUploadPath · 双闸门绿：A 闸孪生 + QA-57 真 Gemini（say 合成音频转写保留 kubelet/Artemis/rebase） |
| Prompt 优化（`ar optimize "draft"`：LLM 改写草稿、`--context` 解析模糊指代；webui 薄壳 composer Sparkles 按钮 + `/optimize` slash + 单步 undo（原稿留前端态）；一次性 provider 调用不碰 daemon/journal） | ✅ | UJ-02/24 | INC-56（HANDA #19）· TestOptimizeRewritesDraft/TestOptimizeSurfacesProviderError/TestHandleOptimizeForwardsAndGuardsDraft（前端 slash.test/composerOptimize.test）· 双闸门绿：A 闸孪生 + QA-57 真 Gemini（草稿改写为领域感知 prompt≠逐字） |

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
| durable delegation/DAG/lease + workspace assignment（生产默认隔离 worktree，显式 shared；revive 复用；isolated 快照语义入工具契约+子 session 注入，`spawn_agent.replaces` 显式回收前任，INC-30） | ✅ | UJ-16/18/23 | INC-11.6 · TestIsolatedTeamWorkspaceSurvivesRevive/TestDelegationDependencyPlan · INC-30 · TestIsolatedChildPromptCarriesSnapshotNotice/TestSpawnReplacesCancelsPredecessor · QA-INC30（真 Gemini 三场景） |
| 子 agent 实时进度镜像（成员事件带 session 标签入树根 hub;`ar attach <child-sid>` live 过滤;webui 子会话 SSE;CLI 前台锚定折叠）/ 子审批根宿主路由与 crash 重挂接 | ✅ | UJ-18/23 | INC-12.6 · TestDaemonAttachChildFiltersLive/TestCrashResumeReattachesApprovalWaitingChild · QA-20(G10 关闭) |
| 子执行收敛为递归 session | 🟡 | — | 阻塞路径已删(D3a);driver 独立待收敛,挂 UJ-22/G23（DESIGN §17） |
| 并发子 agent 确定性测试基建（routing provider） | ✅ | — | C3–C7 孪生在用（GAPS G4 关闭事实） |
| 内置只读 agent 库（explore/plan 随发行 embed;spec `agents:` 列名即 spawn 无需自带 spec;内置优先同名 sibling;model 继承父） | ✅ | UJ-18 | INC-25 · #78 · TestBuiltinSpecLoads/TestResolverPrefersBuiltinAndInheritsModel/TestResolverBuiltinShadowsSiblingFile · QA-32（真机 spawn 内置 explore、只读面、返值）;默认全自动可用拆 #16b |

## C · 工具面

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| read_file / write_file / edit_file（read_file 支持读图/PDF：media envelope+CAS ref,assembly 注入 image/file part,journal 恒 byte-free;5MB 上限;文本路径零变化；write/edit result 带 lines_added/removed 行统计,INC-43） | ✅ | UJ-02/05 | S1 · QA-03（write_file）· INC-33（TestReadFileImage*/TestReadFileImageEndToEnd · QA-38 真机:模型从像素读出截图内容） |
| bash 前台+后台（`output`/`kill` 凭 handle；SIGTERM→宽限→SIGKILL，以进程组实际消失而非 wrapper reaped 为取消终态） | ✅ | UJ-02/18 | S1/S3+INC-67 · TestBashCancelLeavesNoSessionOrphans/TestBashCancelKillsTermResistantGrandchild · QA-05/67 |
| 后台任务 notify 门（`notify: always\|on_fail\|none`；fold 从 journaled args 读门、resume 重放同裁决；none=终态只摘 handle 不回流（fire-and-forget）、on_fail=仅 IsError 回流；Cancelled 不过门；非法值宽容回退 always） | ✅ | UJ-18 | INC-39（HANDA #10 缩水版，唤醒与结构化载荷经勘误已存在）· TestBackgroundNotifyGate（10 例矩阵）· 真验 2026-07-10（真 Gemini 双场景：none 零回流零多余 turn / on_fail 回流+模型复述 exit 3，qa/runs/2026-07-10-INC39） |
| semantic_search（IndexStore，BM25） | ✅ | UJ-01 | S7 |
| publish_artifact（`outputs:` contract、审批载荷；manifest 跨 store instance/process 单写 + 原子替换，不丢并发版本） | ✅ | UJ-06/18 | S5+INC-67 · TestArtifactPublishSerializesAcrossStoreInstances |
| artifact 消费面（模型侧 artifacts_list/read：loop 内部 read 工具、fold `Published` 为真相（orphan blob 不可寻址）、read 分页 offset/max_bytes+next_offset+UTF-8 边界不切、二进制回 metadata、@version 历史寻址；CLI `ar artifacts <sid> list\|read <stream>[@vN]`；webui Supervision Artifacts 区+点击查看器） | ✅ | UJ-06/18/24 | INC-40（HANDA #11）· TestArtifactsList*/TestArtifactsRead*（分页重组/边界/orphan 不漏）· 真验 2026-07-11（真 Gemini publish→list→read 全链 READBACK 逐字命中+CLI+webui 查看器，qa/runs/2026-07-11-INC40） |
| exit_plan_mode（plan mode 跃迁） | ✅ | UJ-06/11 | S2/S3 · TestPlanApprovalFullFlow/TestPlanModeFullFlow/TestExitPlanModeDeniedStaysInPlan |
| schedule_next / finish_series（loop 自定步调） | ✅ | UJ-14 | S6 |
| progress_update（模型整表维护会话 checklist；loop 内部工具不过管线；status 归一 pending/running/done/failed、≤50 条/字段 clamp/redact；result 只回计数；`ProgressUpdated` 纯 fold 出 `state.Session.Progress`，`ar inspect` 文本+JSON 与 webui Supervision Progress 区消费） | ✅ | UJ-18/22/24 | INC-37（HANDA #9）· TestProgressTool*/TestProgressFoldReplacesWholesale + event 全类型 round-trip 守卫 · 真验 2026-07-10（真 Gemini 7 次自发调用+webui DOM 断言，qa/runs/2026-07-10-INC37） |
| grep / glob 独立工具 | ✅ | UJ-01 | INC-3 · TestGrep*/TestGlob* · QA-11（真实 API：模型自发调用 grep+glob，凭据红线守住） |
| grep 参数增强（case_insensitive、glob 文件过滤、output_mode: content/files_with_matches/count；-A/-B/-C context lines；multiline 跨行 regex；默认=旧行为） | ✅ | UJ-01 | INC-22（case/glob/output_mode·QA-30）+ INC-24（-A/-B/-C context·QA-31）+ INC-27（multiline (?sm) 整文件匹配·QA-35）· TestGrepCaseInsensitive/GlobFilter/OutputModes/ContextLines/Multiline |
| web_fetch（客户端执行,**execute-class**;HTML→text、重定向/大小上限、`network` 数据位、收容 fail-closed、**link-local/metadata 无条件封禁**、untrusted 标记;安全 review M1/M2 已对齐,host allowlist S1 待裁/backlog） | ✅ | UJ-01 | INC-5 · TestWebFetch*/TestRefuseLinkLocal* · QA-13 · QA-14（真实 coding agent 端到端） |
| web search | ❌ | UJ-01 | GAPS G18 余项（搜索后端选型 / provider 服务端工具例外类别，单独成增量） |
| ask_user（wait-class 提问：park WAITING_INPUT，应答走 inbox→配对 tool result，同 session 续跑；一批限一问、interrupt/crash/headless 全覆盖；**结构化形态**：questions[]≤4 问×2–4 选项/multi_select/allow_free_text，park detail 携带结构，`AskResolved.Answers` typed，`ar answer <sid> <q>:<n>`/`--skip`（cancelled 非错）+ daemon `answer` 通道，free-text send 兼容保留，crash 重放配对 pending answer；**webui 分步表单卡**（inspect.waiting.ask_questions 暴露结构，AskForm 单/多选+free-text，POST /answer 构 1-based specs，--skip）+ **queued 撤回按钮**（GET /queue+POST /unqueue）） | ✅ | UJ-06/24 | INC-5 · TestAskUser* · QA-13 · INC-47.1（步1，Test{ValidateAskQuestions,ValidateAskAnswers,AnswerCommandPairsAcrossRestart,ParseAnswerSpecs}）+ INC-47.2（步2 webui）· 真验 2026-07-11（真 Gemini：CLI typed 复述+skip（qa/runs/2026-07-11-INC47）；webui 表单点选 Banana/Dinner→choice.txt+queued 撤回端点（qa/runs/2026-07-11-INC47.2）） |
| finish（结束 turn 让 session 待命） | 🧊 | UJ-06 | 记档不预做（DESIGN §17：待命本身就是待命） |
| tool 输出截断（per-tool 上限） | ✅ | 不变量 | S3 |

## D · 权限与安全

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| rules（tool/path/command/network + realpath 归一） | ✅ | UJ-08/20 | S2 · S7（network） |
| bash 命令粒度匹配（复合命令逐段聚合取最严 + wrapper 剥离 + 只读集免提示；显式 deny 先于只读集；fail-safe 退整体） | ✅ | UJ-08 | INC-16 · TestSplitCompound/TestStripWrappers/TestIsReadOnlyCommand/TestCompound*/TestReadonlySetYieldsToExplicitRule · QA-25（真机：victim 存活证逐段 deny） |
| protected paths 写保护（acceptEdits 下 .git/.claude/rc/.mcp.json 等敏感写需审批；只收紧 mode default 自动放行，bypass/显式规则/hardFloor 不变；.claude/worktrees carve-out） | ✅ | UJ-08 | INC-18 · TestIsProtectedWritePath/TestAcceptEditsProtectedRequiresApproval/TestBypassIgnoresProtected/TestExplicitAllowOverridesProtected/TestProtectedWorktreeCarveout · QA-28（真机：.mcp.json 需审批且 pending 时未改写） |
| modes——面过滤/mode 默认/prompt 注入/plan→default 跃迁（default/plan/acceptEdits + bypass 不跳 hooks） | ✅ | UJ-06/11 | S2/S3 · TestAdvertisedToolsByMode/TestPermissionModeDefaults/TestPlanModeFullFlow/TestBypassRunsHooksButSkipsPermission |
| mode 运行中切换（default↔acceptEdits 用户命令；`ar mode`/webui `/mode` + pill 点击切换（选择器：Ask↔Auto-accept 可点，Full/Plan disabled 带原因）；plan 退出仍归 exit_plan_mode 审批、bypass 仅启动时；非法目标显式 rejected receipt；子 agent frozen-at-spawn 不变） | ✅ | UJ-06 | INC-42（G29 关闭）+ INC-58（pill 点击入口，原占 INC-54 撞号让号）· TestModeControlSwitchesToAcceptEdits/SwitchBack/RejectsInvalid/IdempotentReplay/TestReplayProjectsModeChanged + runtimeModeTarget 单测 · QA-44 + QA-51（真机 pill 点击切换六红线+webui playwright 真用户流） |
| 审批流（ask → WAITING_APPROVAL → 应答/拒绝理由回灌） | ✅ | UJ-08 | S2 · 远程审批 S6 |
| hooks（pre/post，observe+block） | ✅ | UJ-19 | S2 |
| OS 沙箱（bash/verifier 默认 filesystem=workspace；Seatbelt/Bubblewrap；network none 棘轮；能力缺失 fail-closed） | ✅ | UJ-20 | INC-11.3 · TestBashFilesystemSandbox/TestBashNetworkContainment/TestSandboxCapabilityMissingDeniesBeforeActivity |
| 凭据 redaction + 硬排除表（含 .netrc/.npmrc 等） | ✅ | UJ-20 | S2/S7 收口 |
| 信任模型（project 层 hooks 与 command tools 需显式 trust，`ar trust`；决策 #19 范畴不变量） | ✅ | UJ-20 | S2 · INC-55（command tools 同门）· TestTrustRegistry/TestMergeUntrustedProjectTightens/TestDiscoverProjectTrustGate/TestCommandToolProjectTrustGate |
| 审批"允许且不再问"：常设应答 + 规则写回（INC-62：同 session 内同判据 ask 由 journal 常设应答自动作答，不触层冻结；跨 session 走 user 层精确 allow 写回（取 A）；spawn_agent 为 tool 级判据入两侧；判据提取两侧共用 `standingCriterion`） | ✅ | UJ-08/18 | INC-17/INC-62 · TestStandingApprovalSameSession/TestStandingApprovalSpawnAgent/TestStandingApprovalSurvivesResume/TestPlainApproveDoesNotStand/TestRememberRuleFromEffect/TestAppendRuleIdempotentAndPreserving/TestRememberedRuleAllowsNextSession · QA-26（真机 bash 面）· QA-62（真 Gemini @ Actions run #2，2026-07-12：3 spawn 恰 1 ask + standing 判词在案 + 写回规则 + 新 session 零 ask，5/5；证据 artifact qa62-run）——G35 已关闭 |
| prompt injection 威胁模型成文 | 🟡 | UJ-20 | GAPS G16：统一信任分级条款已成文（DESIGN §5）+ web_fetch BEGIN/END 定界符落文本内（TestWebFetchPlainText/FollowsRedirects/TruncatesOversizedBody）；余项 host allowlist |
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
| daemon kill -9 后孤儿 bash 子进程清扫（pgid） | ✅ | — | audit-0717 B3 · daemon boot sweep（标记+init-parent 双证据）· TestSweepOrphanSessionProcessesKillsStrayGroup/TestParseProcStat/TestParsePSTableKeepsOnlyInitParented |
| shadow repo 并发 flock（同 GIT_DIR 的 init/Snapshot/ref push 跨 session/goroutine/process 单写，Diff private index 仍并发只读） | ✅ | UJ-15/16 | INC-66 · TestShadowRepoSerializesConcurrentInitAndSnapshots |
| session genesis 守卫（空/无 `SessionStarted|DriverStarted|ForkedFrom+SessionStarted` journal 不可 resolve/list/send/resume） | ✅ | 不变量 | INC-66 · TestResolveSessionDirRejectsEmptyJournalDirectory |

## F · 驱动（one-shot / goal / loop / best-of-N）

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| driver-goal（批式/headless，verifier 三态、停滞检测、carry；fresh child run） | ✅ | UJ-15 | S6 |
| **in-session goal（会话内，context 延续；command/llm_judge/self-cert 三种裁决；只有 pass=`GoalAchieved{satisfied}`；`max_checks` miss=`GoalExhausted{budget}`，goal 保留、update 可扩预算后同 context 继续；step-limit pass 有明确 `goal_satisfied` 收据）** | ✅ | UJ-22 | INC-D1+INC-10+INC-48+INC-66 · 决策 #21/#34/#40 · TestGoalUpdateRecoversExhaustedGoal/TestGoalExhaustionRetainsGoalAndUpdateRearmsIt + 既有 Goal suites |
| loop mode（interval fixed-rate/cron/self_paced、durable absolute tick、overlap skip/coalesce；每个 retry attempt parent-journaled；全 child error=`child_failed`） | ✅ | UJ-14 | S6+INC-66 · TestDriverIntervalOverlapPolicies/TestDriverChildFailRetryRecovers/TestDriverChildFailSurface |
| verifier 管线化（in-session/driver 均 journaled effect + Activity bracket + containment evidence；driver-trust 规则层） | ✅ | UJ-15/22 | S7 · INC-11.3 · TestVerifierActivityTrace |
| best-of-N（隔离 worktree、per-attempt 判定、胜者留盘） | ✅ | UJ-16 | S7 |
| overlap: interrupt | 🧊 | UJ-14 | backlog（与顺序执行同理推迟） |
| 胜者晋升（fork / apply diff） | 🧊 | UJ-16 | GAPS G15（v0 用户手动晋升，记档） |
| cron 跨重启唤醒（daemon **crash** 重启：boot sweep 重挂 running loop drive，missed cron slot 按 overlap 恰好补跑一次；durable tick + Driver.Resume backfill，幂等） | ✅ | UJ-14 | INC-54 · TestDriverCronResumeBackfillsMissedTicks/TestDriverCronResumeCoalescesMissedTicks/TestDriverCronResumeIsIdempotent · TestBootSweepResumesPendingDrives/TestBootSweepSkipsMarkedDrive/TestBootSweepSkipsHostedDrive · TestScanDriveSessionsGate · QA(B闸真实 daemon 重启，集中验) · 优雅停机保活 cron 未做（见 GAPS G22 注） |

## G · 时间旅行（barrier / fork / rewind）

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| CheckpointBarrier（安全边界/turn 收尾/手动，向量+快照 ref） | ✅ | UJ-15 | S7 |
| fork（单创世、处置向量落实、随行库复制、独立 worktree） | ✅ | UJ-15 | S7 收口 review 修复 + fork-of-fork 测试 |
| rewind（fork 后显式切换） | ✅ | UJ-15 | S7 |
| 多模态 blob 在 fork/rewind 下的归属语义 | 🟡 | — | GAPS G1 余项 |
| barrier 对在飞 background work 的处置语义 | 🟡 | — | GAPS G2 余项 |

## H · 生态接入

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| MCP（stdio/streamable HTTP、schema/list_changed、断连恢复、写审批） | ✅ | UJ-19 | INC-11.4；spec→所有 Loop 生产入口自动接线 |
| MCP resources/prompts、structured/multimodal result | ✅ | UJ-19 | INC-11.4；namespaced protocol tools，内容块保真 |
| MCP HTTP OAuth bearer（env 引用） | ✅ | UJ-19 | INC-11.4；token 不进 spec/journal |
| MCP 交互 OAuth 登录 / refresh-token 持久化 | 🧊 | UJ-19 | 凭据 UX；runtime 不持久化 secret |
| skills（Claude Code 约定：读侧目录注入 + 模型侧 invoke + context:fork 一次性子 agent 执行） | ✅ | UJ-19 | S5 · INC-20（`skill` 工具按 name 返回正文,去 frontmatter,WS 边界+防遍历;QA-29 真机）· INC-31（context:fork ingest 展开为 spawn_agent{role},动态角色全链复用,agents_dynamic 门控;TestForkSkill* · QA-37 真机七红线） |
| memory 文件读侧注入（CLAUDE.md 层级合并） | ✅ | UJ-09 | S3 |
| 记忆写回（`ar remember`，append 项目 CLAUDE.md；取 A：追加 program 输入本会话即遵循，文件供下次 session 冻结；并发 session 跨进程单写 + 原子替换） | ✅ | UJ-09 | INC-14+INC-67 · TestMemoryAppend*/TestMemoryAppendConcurrentWritersLoseNoNotes/TestRememberControl* · QA-23 |
| 自定义命令 / slash 面 | ✅ | UJ-19 | INC-8 · TestExpand*/TestDiscover · 真实 API（`.claude/commands/*.md` 的 `/name` 在 new+send 两路展开进 journal） |
| 自定义 command tools（manifest = name/description/command/timeout/params；user 层 `~/.config/agentrunner/tools` 恒载 + project 层 `<ws>/.claude/tools` 需 trust（决策 #19 同 hooks 门）；冻结进 `SessionStarted.command_tools`（resume 从 fold 重建）；撞内置拒载/user 压 project/`mcp__` 前缀拒载；每次调用 = execute-class command effect（`eff.Command`=固定命令过全管线，execute 默认 ask）+ 决策 #34 OS sandbox，args JSON 走 stdin） | ✅ | UJ-19 | INC-55（HANDA #4，决策 #19/#34）· commandtool.TestParseAndResolve/TestDiscover{UserLayer,ProjectTrustGate,BuiltinCollisionRejected,UserBeatsProject} · pipeline.TestCommandToolEffectAdjudication · tool.TestRunCommandTool{Stdin,FailsClosedWithoutSandbox,ExitCode} · agent.TestCommandTool{EndToEnd,ProjectTrustGate,FoldHelpers} · QA-59（真 Gemini PASS 2026-07-11，qa/runs/2026-07-11-INC55） |

## I · 观察与远程面

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| events / inspect（时间线、判定、子树、用量；**stats**：per-tool calls/success/fail/duration_ms、files lines_added/removed（自 write/edit result 载荷求和）、active_seconds（活动区间合并的墙钟，待命/审批挂起不计）——envelope TS 报表投影非核心 fold，文本+--json 两面） | ✅ | UJ-17 | S3/S6;INC-43（HANDA #31,TestBuildStatsAggregates/TestLineDeltaAccounting,真验 qa/runs/2026-07-11-INC43:+6/−1 与实际操作吻合）;INC-11.1 按 stream header 分派 run fold / driver fold，旧 goal/loop journal 可读并展开 iteration 子树；子会话寻址(child_session 全 id,`-sub-` 分段映射 `sub/` 目录,任意深度)INC-1 |
| `ar ps`（fold 的在飞 background work 列表，无 daemon 可用） | ✅ | UJ-18 | QA-05/09 实测 |
| attach/detach（journal 补读 + live 订阅） | ✅ | UJ-17 | S6 |
| 远程审批（daemon approve） | ✅ | UJ-17 | S6 |
| notifier（生命周期通知、跨重启去重） | ✅ | UJ-14 | S6 |
| 远程 stop command | ✅ | UJ-17 | INC-4 · TestStop*（daemon 孪生）· 手验（真 daemon：stop 拆 run、无标记、send 复活）；drive 系列亦可 stop（handleDrive 加 per-run cancel） |
| Web UI 产品面：New session/Scheduled runs/Pinned/Projects→sessions、单一 thread、Changes、deep link、responsive sidebar；首次 session readiness 不投影假空态/raw id；共享大历史首 40 条立即 ready、后台 80/页补齐且 refresh 不重入；session hover pin/archive+project/branch/status；自动标题去同质指令前缀 | ✅ | UJ-24 | INC-19/23/29/38/40/41/60/65 · conciseTitle/frontend view-model/timeline/store session tests · QA-27/34/36/41/42/43/61 |
| journal-backed session metadata（`sessions list --json` 输出 workspace/title/kind/schedule，Web UI cache 非真相源；title 优先 `RawTitle` 投影、回退首行；可选 `--limit/--offset` 先按 journal mtime 排候选再仅 fold 请求页，无 flag 仍全量兼容） | ✅ | UJ-24 | INC-19/23/52/60 · TestCLIResumeAfterCrash · TestCLISessionsJSONProjectsDriverMetadata · TestCLISessionsJSONSurfacesAutoTitle · TestCLISessionsPaginationNewestFirst · TestMetaStoreMerge* · QA-61 |
| LLM 自动会话标题（HANDA #14）：开局后异步一次 `llm_call` 维护调用把首条消息精简为短标题，落 `SessionTitled{source:auto}` journal 事件、fold `RawTitle` 投影；source 分立 auto/manual/fork，**auto 绝不覆盖 manual/fork**；不阻塞开局 turn、崩溃重放不重复生成、失败回退首行；`AutoTitle` 仅顶层托管 session（daemon）启用；webui manual rename 保留 localStorage、displayTitle 层胜出 | ✅ | UJ-24 | INC-52（缩水版 B3）· TestSessionTitledFoldProjection · TestAutoTitleGeneratesOnceAndFoldsProjection · TestAutoTitleDoesNotOverrideManual · TestAutoTitleReusesRecordedResultOnReplay · viewModels.test.ts（auto title/manual 胜出）· QA-53（真机 PASS：精简 auto title+SessionTitled{auto}+坏 key fail-closed） |
| Web UI 内联审批（人类摘要、Details 折叠、Approve once/Deny） | ✅ | UJ-08/17/24 | INC-19 · approval presentation tests · QA-27 真实 waiting:approval（不代用户决策） |
| Web UI Supervision（goal/agent tree 去重/approval+recovery attention/background，成员只读导航；结构化 Run details，raw inspect 仅 advanced） | ✅ | UJ-18/22/23/24 | INC-19/23/29 · dedupeInspectNodes/summarizeInspect tests · QA-27/34/36 真实父/子/recovery session |
| Web UI progressive-disclosure composer | ✅ | UJ-22/24 | INC-19/23/38/40/65 · 默认输入/附件/access/model/send；New session 上缘为独立 Project、Local/New worktree、Local environment、Branch，Project/Branch 可搜索，worktree 尊重 selected ref；Goal/Repeating/Best-of-N、persona 与 YAML 收入 Advanced |
| Web UI 输入边界（JSON 单值且 ≤4 MiB；上传 ≤10 MiB，超限 413 且不留截断/partial 文件；worktree 新建/checkout 共用 branch 校验；流扫描失败显式 error/failed） | ✅ | UJ-04/24 | INC-67 · TestReadBodyRejectsTrailingAndOversizedJSON/TestHandleUpload*/TestHandleWorktreeAcceptsSlashNamedBranch/TestRunRegistryFailsAndCancelsOnOversizedOutputLine · QA-67 |
| Web UI turn 收尾（最终 answer 前 journal-ts Worked duration；Copy；checkpoint/worktree-backed Continue in new session；可 fork 时顶栏按钮；真实 diff Changes 摘要→Review；Changes `Working tree / Last turn` durable 范围，缺 barrier truthful unavailable；generated/unbounded untracked output 有计数地隐藏，超大 diff 默认逐文件折叠且首 paint 前决定 disclosure） | ✅ | UJ-24 | INC-38/57/60/64/65 · TestShadowRepoDiffAgainstSnapshot · TestCLIDiffLastTurnJSON · TestHandleDiffLastTurn/TestHandleDiffNestedWorkspace · completedTurnDurations/summarizeChanges/shouldExpandDiffByDefault tests · SessionView.chrome.test.tsx fork button · QA-41/60/61 |
| Web UI 调度投影（Scheduled 行的 cadence + next run）：driver 的既有调度契约（`DriverSpec.schedule/interval/cron/n`）投影到 `/api/runs` 与 `/api/sessions` 的 `schedule` / `cadence`（人话：`Every 30m` / `Saturdays at 4:00 AM` / `Best of 3` / `Self-paced` / `Runs once`）/ `nextRunAt`（interval 锚在上次迭代开始、cron 自算下一次；**只在系列仍活着且能算出来时给**，终态 driver 不给）；webui 是零依赖独立 module，故 `webui/schedule.go` 是 `internal/driver/cadence.go` 的 stdlib 镜像，经 `ar events --json` 读 journal 的 `driver_started` spec + 最新 iteration tick（终态永久缓存、live 15s TTL）；不能表达的 cron 退回 `Cron <expr>`，绝不猜 | ✅ | UJ-24 | INC-41（CX-3）· TestCadence*/TestNextRun*（internal/driver）+ webui/schedule_test.go · live 8809 实测：28 个 driver session 带 cadence，Scheduled 行副标题 `Every 30m · Ran 9m ago · cx3-ws` |
| Web UI driver 状态正确性（interval `nextRunAt` 向前推进到严格晚于 now 的下一 slot；terminal 不留 running child/progress；denied tool 计 failure；Scheduled Retry 新建同 spec series；driver inspect 汇总已结算 iteration/attempt 的 raw/cache/billed usage） | ✅ | UJ-14/24 | INC-66 · schedule_test.go/TestSettleChildReportRemovesRunningProjection/TestBuildStatsCountsDeniedCallsAsFailures/TestParseDriverRetryInfo/TestBuildInspectTreeUsesDriverFold/TestDriverUsageReportIncludesSettledRetryBeforeIterationCompletion |
| Web UI worktree 一等公民化（New worktree 落共享数据根 `~/.local/share/agentrunner/worktrees/<repo>-<branch>-<ts>` 而非 webui cwd；Changes 面板显示所属 repo/branch + `Apply to project`（git 原生 clean-or-nothing：worktree add-A→write-tree→commit-tree→`diff --binary HEAD`→主 checkout `apply --check` 干跑通过才落，冲突零改动报错）+ `Remove worktree`（脏树防呆确认后 `--force`+`worktree prune`）；旧 runtime/ws worktree 不迁移仍可打开） | ✅ | UJ-10/24 | INC-49 · TestWorktreeInDataDir/TestApplyBackCleanApply/TestApplyBackConflictReported/TestWorktreeRemoveGuardsDirty/TestDiffReportsWorktreeMeta · QA-46 |
| Web UI 交互语义（session row/button、dialog/menu/listbox、Escape/方向键/focus return、popover viewport containment、移动端 sidebar scrim；Settings 与 Command palette dismissal 回到原 trigger/input；打开 Settings 或 mobile sidebar 同步收起另一 overlay/Environment；审批只在宽屏自动展开 Supervision；running/ready/attention/failed/terminal 统一状态色，具体 provider failure 优先于泛化 terminal notice；activity 数量/child link 在窄屏可读，child header 优先 inspect agent spec） | ✅ | UJ-24 | INC-23/29/40/41/60/65/68 · SessionView.chrome.test.tsx/Timeline.thread.test.tsx/viewModels.test.ts · QA-34/36/42/43/61/68 DOM/light/dark |
| Web UI project overlay + 系统 launcher（`webui-meta.json` 扩为 workspace-keyed overlay：自定义显示名/折叠态/last_opened，**装饰性、非分组真相源**，分组仍从 journal workspace 派生；`POST /api/open {workspace,app}` 系统 launcher：app 白名单→固定 per-OS argv（macOS `open -a`／Linux `code`/`xdg-open`）、workspace 必须是 journal 派生的已知 workspace、`exec.Command` 传参不过 shell；overlay 读写原子+向后兼容旧 flat 格式；phosphor 图标不引 lucide；注册/移除在派生模型无语义故裁掉） | ✅ | UJ-24 | INC-53（HANDA #24）· TestLaunchArgvWhitelist/TestOpenRejectsUnknownApp/TestOpenRejectsUnknownWorkspace/TestOpenLaunchesKnownWorkspace/TestMetaStoreProjectOverlayRoundTrip/TestMetaStoreLoadsLegacyFlatFile + 前端 vitest（projectDisplayName/visibleProjectSessions）· QA-56（真机 HTTP：off-whitelist app 400 / 未知 workspace 400 fail-closed / overlay 持久化 webui-meta.json）；真 open -a 靠 argv 孪生覆盖 |
| Web UI 用户消息折叠（>10 渲染行钳 10lh + Show more/less；按渲染高度测量故 wrap 长行也折叠、ResizeObserver 随宽重测；含 pending 队列气泡；纯前端态、copy 恒全文） | ✅ | UJ-04/24 | INC-36（HANDA #23）· frontend build+vitest · 真浏览器 DOM 断言（qa/runs/2026-07-10-INC36） |
| Web UI Markdown 渲染增强（react-markdown + remark-gfm 表格/删除线/任务列表 + 按需 highlight.js 语法高亮（core+19 语言，`common` tree-shaken）+ 每代码块 line-wrap 开关；token 配色映射主题变量双主题可用；**禁 raw HTML**：react-markdown 默认转义、无 rehype-raw、无注入面；对外 `<Markdown text>` prop 不变，Timeline 两调用点零改动） | ✅ | UJ-24 | INC-51（HANDA #20）· A 闸绿：frontend vitest `Markdown.test.tsx`（表格/高亮/line-wrap/raw-HTML 转义 4 断言）+ `tsc -b` + `vite build`；QA-55 真浏览器 DOM 断言（表头 Name/Role+Alice/Bob/Admin 单元格、hljs 高亮 span、字面 <script> 无注入 script 元素/作可见文本、wrap 开关）；mermaid 懒加载记余项 |
| HTTP/WS 壳 | 🧊 | UJ-13 | backlog；窄切片（单端点 webhook ingress）已由 INC-50 兑现，全 API 面仍 backlog |

## J · 运行形态与云

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| daemon 托管（owner-only 0600 socket，chmod 失败不服务；run idem_key + session command_id 幂等、优雅停机） | ✅ | UJ-17 | S6+INC-11.2+INC-67 · TestDaemonSocketIsOwnerOnly |
| CLI 第一公里可发现性（顶层 help/`init` 示例 spec/README/spec 错误附字段清单/daemon 报错附启动指引） | ✅ | UJ-01…（全 journey 进入门槛） | INC-2 · TestTopLevelHelp/TestInit* · spec_errors golden |
| 静止动作（outputs→barrier→goal_verify→parent 回执;ar run=开+发+等静止+读结果） | ✅ | UJ-02/14/15/22 | 决策 #31 · 2026-07-08 落码(D2) · INC-D1/INC-11.1 同步顺序测试 · TestQuiescentSequenceOrder · acceptance events_valid 改静止形状判定 |
| session 内换 agent（SpecChanged 事件 + `ar agent`,用户免确认,prefix 显式换代） | ✅ | UJ-11 | 裁决一 · 2026-07-08 落码(D4a) · QA-10 · TestAgentSwitchTakesEffectOnResume（G8 关闭） |
| 结构化输出（`ar new --json-schema <path>`：回复须为符合 schema 的 JSON,客户端校验+失败重发纠正,打印 canonical structured_output;`--json-schema-max-retries`、与 --detach 互斥） | ✅ | UJ-01 | INC-26 · #91 · TestStructured*（compile/extract/validate/canonical）/TestNewJSONSchema*（scripted 端到端重试+耗尽+usage）· QA-33（真机 Gemini 返值验证）;provider-native JSON mode 拆 #8b · **provider-native**（spec `output_schema`：gemini 原生约束 tool-less 轮生成免 re-prompt,anthropic downgrade;INC-35 · TestToConfig*/TestOutputSchema* · QA-39 真机裸 JSON） |
| curl 一行安装分发（install.sh 多平台探测/私有 repo token 路径/sha256 校验/版本化解包+symlink 切换；scripts/package-release.sh 单机交叉编译 4 target；release workflow tag/dispatch→构建→smoke→发布稳定命名资产；arwebui 优先兄弟 `ar`（避 binutils 同名冲突）；Windows 裁掉随其形态立项） | ✅ | UJ-25 | INC-63 · gate A 孪生 scripts/test-install.sh（5 场景，check.sh 常跑）+ TestARSiblingPreferredOverPATH · gate B=QA-63：workflow 构建+smoke 真跑绿（run 29182533118/29184948127）、v0.1.0 发布 17 资产齐、公网 `curl\|sh` 免 token 真装 v0.1.0 且 arwebui `/api/health` versionMatch=true（升级 symlink 切换由孪生场景 2 锚） |
| 云 workspace 生命周期 | 🧊 | UJ-13 | GAPS G11（S7 预授权裁掉，重启走新增量） |
| IDE 集成 | 🧊 | — | 同上裁决 |
| 多根 workspace（--add-dir 类） | ❌ | — | GAPS G17（待 journey 目录定版） |

---

## 附录 · 代码事实对照（2026-07-17 盘点，审计 audit-2026-07-17）

**CLI 子命令**（`internal/cli/cli.go`）：
`run` `drive` `submit` `resume` `new` `send` `close` `interrupt`
`stop`（INC-4）`compact`（INC-6）`clear`（INC-6）`remember`（INC-14）`kill` `agent`（决策 #32）`ps` `approve` `fork` `barrier`
`sessions` `trust` `attach` `daemon` `events` `inspect` `accept`
`record-fixture` `version` `help` `init`（INC-2）
`diff` `artifacts`（INC-40）`retry`（INC-45）`queue` `unqueue`（INC-46）
`hook`（INC-50）`answer`（INC-47）`mode`（INC-42）`goal`（INC-10）
`dictate`（INC-56）`optimize`（INC-56）

**daemon 线协议命令**（`internal/daemon/daemon.go`）：
`ping` `run` `drive` `attach` `approve` `send` `close` `interrupt`
`stop`（INC-4）`compact`（INC-6）`clear`（INC-6）`remember`（INC-14）`kill` `agent`
`mode`（INC-42）`unqueue`（INC-46）`answer`（INC-47）
`goal-attach` `goal-pause` `goal-resume` `goal-update` `goal-cancel`（INC-10）
（注：`dictate`/`optimize` 是前台一次性 CLI 不经 daemon；`hook` 走
HTTP ingress（INC-50），均非 wire 命令。）

**内置 tool 定义**（`internal/tool/defs/*.json`，26 个）：
`read_file` `write_file` `edit_file` `bash` `output` `kill`
`spawn_agent` `handoff_agent` `publish_artifact` `publish_note`
`read_notes` `semantic_search` `grep`（INC-3）`glob`（INC-3）`skill`（INC-20）
`exit_plan_mode` `schedule_next` `finish_series`
`ask_user`（INC-5）`web_fetch`（INC-5）`progress_update`（INC-37）
`send_message`（INC-12）`artifacts_list` `artifacts_read`（INC-40）
`goal_complete` `goal_status`（INC-10）
（注：`escalate` 无独立 def，提权走 spawn 路径强制人审。）
