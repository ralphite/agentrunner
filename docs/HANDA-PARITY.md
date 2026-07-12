# AgentRunner — handa 功能对照审计（HANDA-PARITY）

**这是什么**：以 **handa**（`/Users/yadong/dev2/handa` 主仓 +
`public/` 发布仓，2026-06 实况；Python/Gemini/Web-first coding agent，
内置 orca 主 agent 与 browser 子 agent）为标尺，对 AgentRunner 逐项
对照的**审计件**——与 CODEX-PARITY / CLAUDECODE-PARITY 互为姊妹件
（第三份）。冲突时以三层活文档（JOURNEYS/SPEC/DESIGN）为准；引用
GAPS 条目不另立缺口编号。

**为什么值得单独审一份**：handa 与前两个标尺（Codex/Claude Code）
差异化明显——它长在**消费面与资产生态**：浏览器自动化（含人机
共驾）、Agent Config 持久化资产、自定义 command tools、reverse-spec、
Web 产品面的读侧功能（artifact 查看、context 细分、消息级操作）。
这些恰好压在我们「发布侧强、消费侧薄」的带上。

**审计方法**（2026-07-10）：5 路并行盘点（handa 文档面 43 份 feature
文档 / 模型侧工具与 agent 实现 / Web-API-CLI 面 / 运行时与发布面 /
AgentRunner webui-CLI 自身实况），产出 38 项对照；经用户逐项裁决
（实现 17 · 延期 17 · 不做 4）+ 独立子 agent 对实现方案的对抗 review
（6 处修正，含一处对我方现状的勘误，见 §4）。裁决与方案记录：
LOG 2026-07-10。

**维护纪律**：同姊妹件——每关闭一个 ✅(实现) 项更新 §2 状态并挂到
对应 SPEC/GAPS 条目；不删行，只改状态。冲刺总控见
`docs/increments/SPRINT-handa-parity.md`。

---

## §1 结论速览

38 项对照：**裁决实现 17 · 延期(defer) 17 · 不做 4**。另有一批
handa 有、我们同级或领先的面（§3），及 handa 自身也未做成的设计/
mock（计 4 项，全部裁「不做」）。

**总判断（三句话）**：

1. **内核我们全面领先**：持久化（journal/fold vs task.json+sqlite
   镜像）、权限安全（rules/审批/OS 沙箱/信任模型 vs 命令黑名单+
   确认问答）、多 agent（树消息/revive/escalate vs 父子 task 边界）、
   驱动形态（goal 控制面/loop/best-of-N vs goal judge 单形态）、
   实时性（SSE vs 轮询）。handa 的 permission/sandbox/memory 都停在
   设计或研究阶段。
2. **handa 领先的是六个整域 + 消费侧长尾**：浏览器自动化（人机
   共驾）、发布安装、Agent Config 资产生态、command tools 轻量扩展、
   reverse-spec、OTel 观测；加 Web 产品面的读侧（artifact 查看器、
   context 细分、消息级编辑/重试、结构化问答表单、Markdown 表格/
   高亮）与模型侧三件（LLM goal judge、progress 清单、后台任务
   完成通知门）。
3. **补齐路径全部落进已有 seam**：结构化 ask_user 落 INC-5 park
   原语、goal judge 落 goal_verify 格 + driver `verifyLLMJudge` 范例、
   artifact 读侧落 CAS+journal 投影、命令面三件（retry/unqueue/
   answer）合并一次 CommandLog 设计——无一需要重写内核；触不变量的
   两项（#8/#29）各自走 PROCESS §四。

---

## §2 对照矩阵

图例：✅ 已实现（挂锚）· 🔧 in-progress（INC-n）· ⬜ 裁决实现待做 ·
⏸ defer（延期，有价值不现在做）· 🚫 不做（记原因）。
编号沿用 2026-07-10 审计对话的决策清单（LOG 同日条目）。

### 域一 · 浏览器自动化

| # | 项 | handa 侧 | 我们现状 | 裁决 |
|---|---|---|---|---|
| 1 | 浏览器自动化整域：browser 子 agent + 9 工具（open/snapshot/click/type/keys/scroll/wait/screenshot/close）、per-session Playwright daemon（跨重启存活）、DOM snapshot（`e12` id+shadow DOM）、截图 + CDP 2x JPEG 帧流、webui 人机共驾（鼠标/键盘/粘贴/滚轮/视口自适应，共享 profile）、URL 白名单 | 已实现（`public/src/browser_*.py`、`docs/features/browser-environment.md`） | 无任何浏览器工具 | ⏸ defer（用户裁决 override，2026-07-10；将来做时整域立项） |

### 域二 · 发布与安装

| # | 项 | handa 侧 | 我们现状 | 裁决 |
|---|---|---|---|---|
| 2 | 自包含发布 + 一行安装 + 崩溃自愈 shim + release CI/smoke | 已实现（install.sh/ps1、package_*.sh、release.yml） | **INC-63 落地（2026-07-12）**：install.sh + package-release.sh（单机交叉编译 4 target）+ release.yml（tag→构建→smoke→发布）；崩溃自愈 shim 不搬——那是 handa 补 bundle 按 $0 定位的产物，Go symlink 即可，daemon 自愈另有 UJ-21 路线 | ✅（UJ-25/SPEC §J；gate B 全程随首个 release） |
| 2b | Windows 全栈支持（taskkill/msvcrt 锁/CI matrix；我们沙箱仅 Seatbelt/Bubblewrap，Windows fail-closed 不可用） | 已实现 | 缺失 | ⏸ defer（与 #2 同批裁决） |

### 域三 · agent 资产生态

| # | 项 | handa 侧 | 我们现状 | 裁决 |
|---|---|---|---|---|
| 3 | Agent Config 生态：模型侧 CRUD 工具、版本化不可变历史+lineage、Agent Editor UI（拖拽 sections/实时 prompt 预览/警告）、Config Viewer、catalog API、instruction sections 模板库、user 级全局 agents 目录 | 已实现（agent_store.py、AgentsPage.vue 等） | spec YAML 文件 + `agents_dynamic` inline（不落盘不复用）+ webui YAML 模态 + INC-25 内置只读 agent 库 | ⏸ defer（与 agents_dynamic 重叠；等 UJ-23 团队线真实使用反馈定形态） |
| 4 | 用户自定义 command tools：manifest（name/description/command/timeout/params JSON schema）把本地命令包成模型工具，args JSON stdin 传入 | 已实现（tool_store.py、agent-command-tools.md） | 无（外接工具只有 MCP） | ⬜ **实现**。方案（review 修订）：发现层 user 层 `~/.config/agentrunner/tools/*.json` + project 层 `<ws>/.claude/tools/`；**project 层 manifest = 可执行配置，trust 门与 hooks 同级（未 trust 不加载）**（决策 #19）；每次调用 = execute-class command effect 过完整权限管线 + 决策 #34 OS sandbox；与内置撞名拒载、user/project 撞名定优先级。规模 M。〔批 4〕 |

### 域四 · 驱动与裁决

| # | 项 | handa 侧 | 我们现状 | 裁决 |
|---|---|---|---|---|
| 5 | Reverse spec 生成器（SpecFold：evidence→atoms→L2–L4→system spec，每层 verify gate+repair，session 链驱动） | 已实现（reverse_spec_cli.py） | 无 | ⏸ defer（不在 journey 目录上；将来可作 skill/驱动形态而非内核） |
| 8 | LLM goal judge：三态裁决 achieved/continue/blocked、只认历史可见证据、citations、continue reason 回灌 | 已实现（goal_judge.py、specs/goal-stop-hook.md） | goal 只有 command verifier + `goal_complete` 自证；llm_judge/blocked 均为 SPEC F 区登记余项 | ⬜ **实现**（M/L，**触不变量走 PROCESS §四**）。方案（review 修订）：`ar goal attach --verify-llm "<rubric>"`；judge = **budget-gated 的 `llm_call` 管线 effect**（Activity-bracketed，接 driver `verifyLLMJudge`（driver.go:1228）进 Loop，恢复语义靠「Activity 已完成→复用 journaled result」）；**触发门控：仅在 `goal_complete` 声明时裁决**（镜像自证形态，杜绝每静止边界一炮的无界花费），`max_checks` 硬上限兜底；`verifiersHaveCommand` 二元判别升 command/llm_judge/self-cert 三态并定优先级；新增 blocked 终态（`GoalAchieved.Reason:"blocked"`）。**修订 §13:1184-1187/决策 #21（非决策 #34）**。〔批 3，单独 INC〕 |

### 域五 · 观测与统计

| # | 项 | handa 侧 | 我们现状 | 裁决 |
|---|---|---|---|---|
| 6 | OpenTelemetry/Phoenix tracing（invocation/子 run span，失败静默降级） | 已实现（observability.py） | 无 OTel（journal/events 自成体系） | ⏸ defer（随 CLAUDECODE-PARITY SPRINT #22 同批裁决） |
| 13 | Context usage inspector：composer 占用环（75/90% 阈值）+ 来源细分对话框（instruction/tools/messages/thought 分项+估算规则）+ 发送前静态估算 | 已实现（context-usage-inspector.md） | Timeline token 徽标 + inspect usage 汇总 | ⏸ defer（breakdown 需 assembly 侧配合，成本不低） |
| 31 | 结构化运行统计：tools{calls,success,fail,duration_ms}、files{lines_added,removed}、active_seconds（扣等待） | 已实现（cli-json-output.md） | 有 token/cache 审计；无工具成败/耗时/行增删/active 时长 | ⬜ **实现**。方案（review 修订）：success/fail 由 `ActivityCompleted.IsError` 纯 fold 聚合；**行增删在 edit/write 执行时算好写入 `ActivityCompleted` 载荷**（additive-optional，决策 #18；不在 fold 里 diff 已 redact 的 args）；duration/active_seconds 用 envelope `TS` 做**报表投影**（注明非核心 state fold）。出口 `ar inspect --json`/`ar run --json`。规模 **M**（含事件 schema+写路径）。〔批 1〕 |

### 域六 · 模型侧工具面

| # | 项 | handa 侧 | 我们现状 | 裁决 |
|---|---|---|---|---|
| 7 | 结构化 request_user_input：≤4 问、每问 2–4 选项、multi_select、free text；分步表单 UI、数字键快选、单问单选即提交、Skip=cancelled | 已实现（request-user-input.md、UserInputForm.vue） | `ask_user` 单问纯文本（INC-5） | ⬜ **实现**（= CLAUDECODE-PARITY SPRINT **#10**，两边状态联动）。方案（review 核验）：park/配对复用成立（一个 call 一次 park，questions[] 装进一个 call 不违反「一批一 ask」§17:1360）；`AskResolved.Answer` 扩成 typed payload（additive）+ 新 `ar answer` 语法/daemon 命令，free-text `ar send` 兼容归并；webui 分步表单卡。〔批 2，命令面设计单元〕 |
| 9 | progress_update 进度清单：模型整表维护 checklist（去重/归一/不回显），右栏 Progress 区 | 已实现（progress.py、session-progress-right-sidebar.md） | 无模型侧 todo 面；Supervision 无 checklist 区 | ⬜ **实现**。方案（review 修订）：**不造 "state-class" 术语**——按 `goal_complete`/`publish_note` 先例做 loop 内处理的内部工具，journal `ProgressUpdated`、fold 出 `state.Progress`（对照 `s.Session.Published` 先例），不过管线；消费面 `ar inspect` + SupervisionPanel Progress 区。规模 S/M。〔批 1〕 |
| 10 | 后台任务完成通知门：任务终态→幂等通知→空闲会话注入 turn；可抑制 | 已实现（background_task_manager.md） | **勘误（§4）**：唤醒已存在——idle `awaitInput` 监听 `bg.done`（conversation.go:311），bash 与子 agent 共用 settle seam，`TestBackgroundTaskSettlesBeforeQuiescence` 钉住 | ⬜ **实现（缩水为 S）**。真 delta 仅两件：spawn 参数 `notify: always\|on_fail\|none` 抑制门 + settle 载荷结构化（exit code/输出 tail；注意 DESIGN §4 规定后台终态是 **user-role** 消息回流）。唯一 DESIGN 补语义：`notify:none` 时 settle 不产唤醒输入、handle 摘除后可静止——additive 不触粗体不变量。〔批 1〕 |
| 11 | artifact 消费面：模型侧 artifacts_list/read（窗口分页/metadata_only）+ CLI 子命令 + webui Artifacts 面板/查看器 | 已实现（orca/tools.py:333-405 等） | `publish_artifact` 只有发布半边：`ArtifactPublished` 已 journaled 已 fold（state.go:697 `Published`），CAS 读 API 齐，但无模型读回/CLI/webui 消费 | ⬜ **实现**（review CONFIRMED 纯 additive）。(a) `artifacts_list`/`artifacts_read`（read-class，分页 offset/max_bytes）；(b) `ar artifacts <sid> list\|read <name>[@vN]`；(c) webui 右栏 Artifacts 区+查看器。规模 M。〔批 1〕 |
| 12 | 内置 skills 库（chat-session-analysis/qa/vcs-jj）+ user 级全局 skills 目录 + GitHub 导入脚本 | 已实现（system-skills.md） | skills 仅 workspace `.claude/skills`（skill.go:36）、零内置、无导入链 | ⏸ defer（机制已在 INC-20，内容库等生态需求） |

### 域七 · 会话与输入

| # | 项 | handa 侧 | 我们现状 | 裁决 |
|---|---|---|---|---|
| 14 | LLM 自动会话标题（fallback 首行→异步 LLM 精简；auto/manual/fork 来源分立，不覆盖手动） | 已实现（session-title-generation.md） | title=首条消息首行（meta.go:71）；manual rename 在 localStorage | ✅ **已实现（INC-52 缩水版 B3，A 闸绿；B 闸真机 QA-51 待验）**。**auto-title 走 `SessionTitled{source:auto}` journal 事件**（journal-backed metadata 教义，fold `RawTitle` 投影）；生成为 harness `llm_call` 维护调用（compaction/judge 同族），不阻塞开局 turn、崩溃重放不重复；**manual rename 保留 localStorage 原样**（§12:1092 禁止迁移）；`AutoTitle` 仅顶层托管 session 启用。承接 INC-23 W9 移交。与 CLAUDECODE SPRINT #17（webui rename）避让。工作纸 `INC-52-auto-title.md`。〔批 5〕 |
| 15 | 用户消息编辑 rewrite（truncate_session 截断历史+原附件重发） | 已实现（user-message-edit.md） | 无；与 append-only journal 教义冲突 | ⏸ defer（若做应设计为「从消息点 fork」变体，单独设计轮） |
| 16 | 失败 turn 一键 retry（原地复用 input+附件重跑） | 已实现（`POST /api/turns/{id}/retry`） | 无（resume 只救崩溃） | ⬜ **实现**。方案（review 修订，B5）：`ar retry <sid>` + webui 按钮，从 journal 读原始 `InputReceived`（Text/Images/Files ref）重发为新 turn；**command_id 用派生的确定性 id（`retry:<turn-id>`）**——同 turn 重复点击被幂等去重，retry 与原输入是不同命令（随机 id=重复投递、复用原 id=被 `PreviouslyAccepted` 短路，两头都错）。仅待命时可用。〔批 2，命令面设计单元〕 |
| 17 | 消息级操作条与消息级 fork（hover：时间戳/Fork/Edit/Copy；从任意 completed turn 分叉、fork 预填 composer） | 已实现（session-fork.md） | 消息 hover 仅 Copy；fork 是 barrier 级模态 | ⏸ defer（barrier 级 fork 已可用，消息锚点是交互增强） |
| 29 | 排队消息管理（pending 队列可编辑/取消） | 已实现（composer queue） | 忙时排队在（inbox），UI 乐观气泡不可改撤 | ⬜ **实现**（M，**触 §2 CommandLog 铁律走 PROCESS §四**）。方案（review 修订，M1 五点语义）：daemon `unqueue <command_id>` 追加 **durable revoke**（否则 crash 后原命令重放）；**已消费竞态 no-op**（point-of-no-return = `InputReceived` 落账，复用迟到审批 no-op 先例 DESIGN §430/640）；**作用域只限排队的对话输入**（不得撤 interrupt/approval/close/kill）；revoke 幂等；被撤 seq 仍推进 `ConsumedInputSeq`（consumed-as-revoked）。webui 气泡加编辑（=撤销+重发）/取消；`ar queue <sid>` 列出+撤销。〔批 2，命令面设计单元〕 |
| 32 | stdin 管道 prompt（`echo … \| handacli`） | 已实现（cli-json-output.md） | 任务文本必须是参数 | ⬜ **实现**。`ar run/new/send` 任务参数缺省且 stdin 非 tty（或显式 `-`）时读 stdin；注意与 `--image/--file` 共存（附件只在 send 路径，§17:1366）。规模 S。〔批 1〕 |

### 域八 · webui 消费面

| # | 项 | handa 侧 | 我们现状 | 裁决 |
|---|---|---|---|---|
| 18 | 服务端语音听写（音频→Gemini 转写，session/project 上下文消歧专有名词，中英混合） | 已实现（routes/dictate.py） | useVoice = 浏览器 SpeechRecognition（无上下文、兼容受限） | ⬜ **实现**（用户裁决 override）。方案（review 修订，B4）：**走 `ar dictate` 命令**（provider 调用落 harness 凭据层，守 §12:1075「webui 只经 ar/daemon 契约」+ 决策 #15c，webui 仍薄壳）；provider 层补 audio 输入 part（provider.go 现仅 image/file）→ 规模 **M**；定性为 composer 文本便利（非模型 audio 模态，不撞 DESIGN 非目标「语音输入」line 36）；音频大小上限。前端录音+上传，SpeechRecognition 留 fallback。〔批 5〕 |
| 19 | Prompt 优化（LLM 改写草稿+上下文解析模糊指代+undo） | 已实现（routes/optimize_prompt.py） | 无 | ⬜ **实现**（用户裁决 override）。方案（review 修订）：**走 `ar optimize` 命令**（同 #18 路线，不让 webui 直调 provider）；composer Sparkles + `/optimize` slash + 单步 undo（原稿留内存）。规模 S（搭 #18 的车）。〔批 5〕 |
| 20 | Markdown 渲染增强：表格、语法高亮、Mermaid、line-wrap/copy 工具栏、diff fence | 已实现（markdown-*.md 两份） | 自研极简 Markdown——**无表格**、无高亮、无 mermaid（Markdown.tsx） | ⬜ **实现**（review CONFIRMED）。react-markdown + remark-gfm（表格）+ highlight.js core 按需语言；**保持禁 raw HTML**（react-markdown 默认转义，与现状一致）；line-wrap 开关；mermaid 懒加载后置为可选尾巴；注意 bundle 预算。规模 M。〔批 5〕 |
| 21 | 模型输出 HTML 卡片渲染（html_output 指令+trusted/safe policy） | 已实现（html-output-prompt-guidance.md） | 无（纯 Markdown） | ⏸ defer（注入面安全考量，价值集中演示场景） |
| 22 | 图片 lightbox（缩放/适应/新标签） | 已实现（user-message-image-preview.md） | 仅缩略图 | ⏸ defer |
| 23 | 用户消息折叠（>10 行 Show more/less，按真实 DOM 高度） | 已实现（user-message-collapse.md） | 无（Timeline 用户气泡裸渲染） | ⬜ **实现**（用户裁决 override；review CONFIRMED 纯前端）。Timeline 用户气泡按渲染高度折叠，纯前端态。规模 S。〔批 1〕 |
| 24 | Project 一等实体 + 系统 launcher（注册/重命名/移除/last_opened；VS Code/Finder 直开带系统图标） | 已实现（web_projects 表、launcher API） | 按 workspace 目录名分组；无 launcher | 🔧 **A 闸绿（INC-53，B 待验）**（用户裁决 override）。方案（review 修订）：**不建服务端注册表**——扩展 `webui-meta.json` 为 workspace-keyed **overlay**（display name/折叠/last_opened；分组仍从 journal workspace 派生，守 §12「grouping 以 workspace 为键」，不删 localStorage key）；launcher `POST /api/open {workspace, app}` 后端 `open -a`/`xdg-open`（薄后端新 OS-exec surface，localhost+用户驱动，app 白名单+已知-workspace 门，已记档）；**图标用现有 phosphor（不引 lucide）**，不做 .icns 提取。注册/移除在派生模型无语义故裁掉。规模 S/M。〔批 5〕 |
| 25 | UI 元数据服务端化（pin/rename/unread/archive/settings 入服务端跨设备） | 已实现（sqlite settings） | 全 localStorage | ⏸ defer（与 §12:1092 教义张力，等多设备需求） |
| 26 | 归档管理页 + 会话软删除 | 已实现 | 仅 Show/Hide archived 开关 | ⏸ defer |
| 27 | API key 管理 UI（遮罩/来源/保存移除） | 已实现（SettingsPanel.vue） | `--env-file`/环境变量 | ⏸ defer |
| 30 | UI polish 杂项包（统一 tooltip 系统、过程折叠单行耗时摘要、工具 outcome 判定、files_list 分组展示等） | 已实现（tooltips.md 等） | 部分已被 INC-23 顺手覆盖（相对时间副行、状态色板） | ⏸ defer（攒批做 polish 轮，与 INC-23 Round 2 合流） |

### 域九 · 无人值守

| # | 项 | handa 侧 | 我们现状 | 裁决 |
|---|---|---|---|---|
| 28 | Automated tasks 任务实体产品面（任务定义管理/Run now/每任务运行历史/IANA 时区/LLM 任务名） | 已实现（automated_tasks/） | loop/cron 是 driver 会话形态（能力等价+）；无任务实体管理面 | ⏸ defer（webui Scheduled 已承担运行列表；实体面等需求） |
| 28b | cron 跨重启唤醒（backfill 补排+错过 slot 恰好补跑一次） | 已实现（dispatcher.py backfill/dedup_key） | **crash-重启支已实现（INC-54，A 闸绿·B 闸待验）**：durable tick+Driver.Resume backfill+bootSweepDrives，missed slot 按 overlap 恰一次，不越 close 标记 | 🟡 **实现中**（= CLAUDECODE-PARITY SPRINT **#15**）。crash 支落地；**优雅停机保活 cron** 另立增量（GAPS G22 注b，触 driver 终态语义走 §四）。〔批 3〕 |
| E2 | 外部事件唤醒既有 session（webhook→inbox） | UI 占位未实现（event trigger "Soon"） | GAPS **G14**（inbox 原语已备缺投递壳；§2:190 已授权同通道投递） | ⬜ **实现**（M；本就是我们仅存 journey 卡死项 UJ-12）。方案（review 修订，M3）：daemon HTTP ingress `POST /hooks/<sid>`（per-hook token；token 不进 journal/明文）→ durable send 通道，载荷 `source:"machine"`（净新常量；`userClassSource` 已明确排除机器方 sendmsg.go:114）+ `trust:"untrusted"`；**硬条件：核实 untrusted 标记真正驱动 assembly 隔离框定**（若仅元数据须补，否则注入防御是纸面的）；未鉴权限流防预算 DoS；子 session 目标走决策 #35 revive；HTTP 壳与 backlog「HTTP/WS 壳」合并考量。〔批 3〕 |

### 域十 · 裁定不做（handa 亦未做成或我们已有等价物）

| # | 项 | 理由 |
|---|---|---|
| E1 | NL workflow 图/workflow library | 🚫 handa 仅 Storybook Future mock；我们 spawn/DAG/team 已覆盖实质 |
| E3 | Ralph loop 形态（planner 确认→builder/verifier 循环） | 🚫 handa native 运行时已无实现（文档遗存）；我们 driver-goal 即其已实现等价物 |
| E4 | 权限审批卡/OS 沙箱/memory 对齐项 | 🚫 handa 均为设计/研究阶段，我们已全面领先，无动作 |
| E5 | YOLO mode 类占位 | 🚫 handa 自身即 Not implemented 占位 |

---

## §3 已确认齐平或领先（防误判，2026-07-10 双侧核验）

全局搜索（⌘K+sidebar）、未读（自动+手动）、pin/star、archive、
rename（localStorage）、附件粘贴/拖拽、per-session 草稿、系统通知、
主题三态、**SSE 实时打字流**（handa 反而全靠 900ms–5s 轮询）、
**Changes/diff 审阅+本地 commit**（handa 无 diff 面）、worktree/分支
管理、多 provider（Gemini+Anthropic vs 仅 Gemini）、reasoning 档位、
权限规则/审批流/OS 沙箱/凭据红线/信任模型、hooks（pre/post tool +
8 生命周期事件 vs 5 hook 点）、多 agent 树（兄弟互发/revive/escalate/
树预算 vs 父子 task 边界）、goal 控制面（pause/update/resume vs
set/clear/cancel）、fork/rewind/barrier、compact/clear/microcompact、
MCP、记忆写回、crash 恢复矩阵（vs 孤儿收敛+boot 清理）、CLI/Web
共享 store、best-of-N、每 session 串行+跨 session 并发、CLI 附件
（handa CLI 不支持附件）。

---

## §4 审计与 review 记录

**方案对抗 review**（2026-07-10，独立子 agent 对照 DESIGN 原文 +
代码取证）：6 处放行前修正，已全部吸收进 §2 方案列——

1. **#10 勘误（对我方现状的误判）**：初版审计称「bash 后台任务完成
   不唤醒待命会话」——**错**。idle `awaitInput` 本就监听 settle
   （conversation.go:311-317，与子 agent 回执共用 seam，
   TestBackgroundTaskSettlesBeforeQuiescence 钉住）。#10 由 M+触
   不变量缩水为 S 级 notify 门。
2. **#8**：初版引错不变量（决策 #34 → 应为 §13:1184-1187/决策 #21）；
   judge 必须是管线化 `llm_call` effect；触发必须门控（否则每静止
   边界一次 LLM 调用无界花费）；三态/blocked 是净新。
3. **#14**：「rename 迁移」撞 §12:1092 粗体条款，删除；拆为
   auto-title（journal）+ manual rename（localStorage 原样）。
4. **#18/#19**：webui 直调 provider 破 §12:1075 projection 教义 +
   决策 #15c 凭据面；改走 `ar dictate`/`ar optimize`。
5. **#16**：幂等叙述自相矛盾（随机 id=重复、复用 id=被短路）；改
   派生确定性 command_id。
6. **#29**：补五点 revoke 语义（durable/已消费 no-op/作用域/幂等/
   high-water），走 §四。

**结构性修正**：#16+#29+#7 三项同改 protocol/daemon-dispatch/消费侧，
合并为一个「命令身份·撤销·应答」设计单元（一次设计分步落地）。

## §5 审计基线

- handa：`/Users/yadong/dev2/handa`（主仓 docs/storybook/website）+
  `public/`（发布仓 src/，Python 3.11+/FastAPI/Vue3/google-genai/
  Playwright），git 快照 2026-06 下旬；内置 agent 实为 orca+browser
  （ralph 仅存文档与前端 mock）。
- AgentRunner：main@13f65e6（INC-26 后）。
- 盘点系数：5 路并行子 agent 穷尽扫描 + 4 项人工代码复核
  （artifact 消费面/Markdown 表格/skills 目录/后台唤醒——最后一项
  盘点结论错误，由方案 review 纠正，见 §4）。
