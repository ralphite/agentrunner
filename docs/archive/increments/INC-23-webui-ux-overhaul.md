# INC-23 webui UX 大扫查与整改(Round 1)

## 动机与 journey 锚

用户以真实使用者身份对 webui 提出根本性质疑:界面充斥费解信息
(纳秒时间戳项目名、裸路径标题、常驻空面板、永远为空的 Changes)、
"workspace / worktree 到底 work 不 work"。本增量以**挑剔用户 + UI/UX
专业审查**双视角对 webui 全量走查(真 daemon + 真 Gemini,QA 纪律),
登记全部问题并分批修复。

Journey 锚:UJ-WEBUI(webui 是 AgentRunner 的正式本机产品面,
`webui/README.md` 契约);不新增 journey,修复的是既有 journey 的
**验收质量**。少数行为变化项(默认权限、workspace 落位)单列 delta。

## 走查方法与证据

- 从 main@194deec 构建 `ar` + 前端 + `arwebui`,连**共享 daemon**
  (`~/.local/share/agentrunner/`),端口 8807,真实浏览器操作
  (Claude Preview 驱动),真 Gemini 跑新任务 + 审批 journey。
- 证据 session:`20260710-043858-task-44c3`(两人团队 hello.py,
  用户截图同源)、`20260710-050026-use-the-bash-tool-*-27f3`
  (本轮新建,UXR1-PROOF 审批链)。
- 结论先答用户两问:
  - **workspace work 吗?** 文件写入真实(hello.py / uxr1-proof.txt
    都在),但 Changes 视图对 auto-created workspace **静默失效**
    (W1),且命名/落位不可理喻(W2)——"看起来不 work"。
  - **worktree work 吗?** sub-agent 的 worktree 是快照复制,能隔离、
    能同步回主 workspace;但 spawn 时机早于队友产出时拿到**空快照**,
    该成员找不到文件空转,烧掉 80 步/195k tokens 才撞上限死掉,期间
    无人回收、Attention 还显示 "Nothing needs you"(W4/W35)。

## 问题清单

编号 W-n;分级 P0(骗人/坏)> P1(核心 UX 受阻)> P2(明显粗糙)>
P3(打磨)。标注根因文件。〔批次〕见实施步骤。

### P0 — 功能性欺骗与严重故障

- **W1 Changes 视图对 auto-created workspace 永远显示 "No changes"**。
  workspace 里明明有新文件(实测 uxr1-proof.txt),UI 却说无改动。
  根因 `webui/meta.go handleDiff`:`git rev-parse --is-inside-work-tree`
  对"嵌在 agentrunner 仓库 gitignored 目录里的 workspace"返回 true
  (命中**父仓库**),`git diff`/`status` 在父仓库语境下自然为空。
  修:isRepo 判定改为 `--show-toplevel` 归一化后 == workspace 本身;
  非 repo-root 时给出明确文案与行动(git init);`handleWorkspace`
  创建 auto workspace 时直接 `git init`(untracked 合成 diff 已有,
  Changes 从此真实可用)。〔B1〕
- **W2 auto-created workspace 纳秒命名 + 落位随 webui 进程走**。
  `ws1783659626368076000` 做项目名毫无意义;目录挂在
  `<webui cwd>/runtime/ws/` 下,用户实例在 `.codex/worktrees/b875`
  里起就落在那里(用户截图困惑的直接来源)。修:命名改
  `ws-<yyyymmdd-hhmmss>`(可读、可排序);侧栏对 runtime 下的
  workspace 显示友好标签;Environment 菜单说明落位。〔B1〕
- **W3 daemon `ps` 泄漏已死 spawn_agent handle**。call_6_0 的
  sub-agent 早已 quiescent(max_generation_steps),`ar ps` 仍报
  `running`,UI Background work 永远挂着(用户"右边一直显示"的
  第二半)。daemon 端 in-flight 表未在子 agent 异常终止时清理。
  修 Go runtime + UI 侧对无法核实的 handle 提供人话与清理动作。〔B2〕
- **W4 被抛弃的 sub-agent 无人回收,独自烧完 195k tokens**。主 agent
  抛弃 call_6_0 另起 call_15_0,没有 kill;弃子在空 worktree 里
  空转 80 步。runtime 层缺"父放弃即回收"或空转熔断;登记 GAPS,
  本轮先做 W35(Attention 上升)与 W6(错误人话)。〔GAPS+B2〕

### P1 — 核心 UX 受阻或误导

- **W5 Supervision 面板默认常开、切 session 重置、浮层遮挡 Changes**。
  `SessionView.tsx supervisionOpen=useState(true)`;无内容也占位,
  开着切到 Changes 还盖在 diff 上。修:默认关闭;有
  approval/goal/agents/background 时自动亮起;记住用户手动开关
  (localStorage);Changes 视图侧不遮挡。〔B2〕
- **W6 Subagents 行裸奔内部错误码** `max_generation_steps`。
  `pill.ts friendlyStatus` 不认识 → 原文上屏。修:映射为
  "stopped: step limit"(类似补 budget/killed 等),色用警示。〔B2〕
- **W7 Background work 行原始 kv 文本且断尾**:`spawn_agent · running
  agent=worker task=`(daemon detail 序列化丢 task 文本)。修 UI 文案
  组装 + daemon detail。〔B2〕
- **W8 侧栏 Projects 无排序、4s 轮询下列表跳动**。组=首次出现序、
  session=API 序,刷新即重排,用户找不到东西。修
  `viewModels.buildSidebarModel`:session 按 id 时间戳倒序,组按组内
  最新倒序,稳定可预期。〔B3〕
- **W9 任务无短标题:整条 prompt 截断当标题,7 条同名无法区分**。
  修:标题渲染去模板化(首句/截断优化),副行显示相对时间;
  tooltip 显示全文(现在是 sid)。〔B3〕
- **W10 时间线零时间戳**。任何消息/命令/回复都无时刻。journal
  envelope 带 ts(daemon 记录),fold 丢弃。修:气泡 hover 显示
  绝对时间,分组处显示相对时间标记。〔B3〕
- **W11 审批卡缺 "Always allow"**。CLI 已有 `approve --always`
  (INC-17),UI 只有 Deny/Approve once,精确规则能力不可发现。
  修:第三动作 "Always allow"(tooltip 讲清写 user 层规则),
  webui 后端透传。〔B4〕
- **W12 Scheduled/Runs 只见本实例发起的 run**。`runs.go` 记录在
  webui 本地 runtime/runs,daemon 侧 driver 会话不入列——换实例即
  "全部丢失"(本轮 Scheduled 页空、而 daemon 里明明有 driver)。
  短期:页面说明数据边界;登记 GAPS(runs 归 daemon)。〔B4+GAPS〕
- **W13 弹出菜单溢出视口顶部**。`Popover.tsx` 翻转阈值 hardcode
  360px,实际菜单 500px+;无 max-height 兜底。Home 页 model/Task
  options 菜单顶部被截。修:测量真实高度决定翻转 + panel
  max-height+滚动。〔B2〕
- **W14 Environment 选择器没有"最近 workspace"**。只有"新建空目录 /
  手输绝对路径",既有项目要人肉拷路径——一次性 workspace 泛滥的
  直接推手。修:列出最近 N 个已用 workspace(来自 sessions),
  一击选用。〔B3〕
- **W15 默认权限 Full access 且以警示橙呈现**。新任务默认"什么都
  不问",与产品 supervision 理念相反;颜色语义混乱(警示色当默认
  态)。修:默认 Ask to approve;记住上次选择;Full access 保持
  警示视觉。**行为变化,见 Spec delta**。〔B4〕
- **W16 New run 模态七宗罪**:kind 直接用 CLI 词 submit/drive;task
  默认值硬编码 demo 文案(应为 placeholder);spec.yaml 全文裸露
  (违背 progressive disclosure,应折叠 Advanced);"idem key" 黑话;
  按钮 "Start submit" 拼接语法;workspace 又是手输;缺最近列表。〔B4〕
- **W17 两个 "Background work" 同名异物**:侧栏 Scheduled → 页面
  标题 Background work(submit/drive runs);Supervision 分区
  Background work(session 内 in-flight 工具)。修:Scheduled 页
  统一 "Runs";Supervision 分区改 "Working now"。〔B3〕
- **W18 模态不响应 Esc**(实测 New run 模态)。修 Modals 统一
  keydown。〔B2〕
- **W19 agent 间消息渲染成用户气泡且裸内部 id**:
  `[message from worker (20260710-…-sub-call_6_0-a1)]` 以"你说的话"
  样式出现。修:fold 识别 from 源,渲染为标注来源的 agent 消息样式,
  id 缩写(worker · call_6_0,链接到子会话)。〔B3〕

### P2 — 明显粗糙

- **W20 项目组同名无法区分**(两个 `ws`、`workspace` 组):组标题
  只取 basename。修:重名组带缩略父路径副标签。〔B3〕
- **W21 "View run details" = 原始 JSON 弹窗 + 双图标**。至少命名
  诚实("Inspect data (JSON)"),图标留一;后续做结构化视图。〔B5〕
- **W22 Fork 模态**:空态文案语病("as the session checkpoints
  turns");无 checkpoint 时应内联 "Checkpoint now" 一键;
  "make empty workspace" 在 fork 语境误导(fork 本会造 worktree)。〔B5〕
- **W23 内部编号漏进产品文案**:persona 描述含 "(INC-12)"。删。〔B2〕
- **W24 "Spec-defined access" pill 术语晦涩**(session 内)。改
  "Access: from agent spec" 类人话 + 保留 tooltip。〔B5〕
- **W25 审批卡不显示 workspace 路径**("in the current workspace"
  但不知是哪)。加路径行。〔B4〕
- **W26 侧栏底部 daemon 信息浪费**:AgentRunner 品牌重复,版本藏
  title。改 "Connected · <version>"。〔B5〕
- **W27 状态点体系无图例、unread 蓝点+状态点+pin 三件套拥挤**。
  status dot title 已有;补充配色语义梳理(见 W33)+"最近活动"副行
  后拥挤感下降;保留。〔B3 附带〕
- **W28 `waiting:input` 映射绿色系点,与"运行中"混淆**。completed
  灰 / waiting 蓝 / running 绿脉冲 / appr 橙 / crash 红 分明。〔B3〕
- **W29 Home hint 文案空泛**("Workspace and run settings stay
  available when you need them.")。改指向实际控件。〔B5〕
- **W30 composer 无 Stop 控件**(运行中只能去顶栏)。运行中发送键
  变 Stop(Codex 惯例)。〔B4〕
- **W31 Deny 无 reason 输入的可发现性**(ApprovalCard Details 内?
  走查未见)。审查 ApprovalCard 补 reason 输入。〔B4〕
- **W32 `turn N` 分隔与 system 事件开发者开关**默认藏 OK,但
  "Show system events" 藏在 ⋯ 菜单底部无状态提示(开了容易忘)。
  菜单项加勾选态(已有 Check)+ 时间线顶部提示条。〔B5〕

### P3 — 打磨

- **W33 状态色板统一**(见 W28,含 supervision/sa-dot/pill 全部
  对齐一套语义色)。〔B3〕
- **W34 palette Sessions 结果无上下文**(项目/时间),同名难选。
  副行加 project + 相对时间。〔B5〕
- **W35 Attention 语义太窄**:只数 approvals。失败/step-limit/烧钱
  异常的 agent 应上升(W4 的 UI 半)。〔B2〕
- **W36 菜单无键盘导航**(role=menu 无 arrow/Home/End)。〔B6〕
- **W37 Close session 用 window.confirm 原生框**(store 已有
  openPrompt 自定义先例)。〔B6〕
- **W38 "generation steps" 徽标术语长**,可 "steps"(hover 全称)。〔B5〕
- **W39 "Show N more" 展开后无收起**。〔B5〕
- **W40 项目组无右键菜单**(整组归档/复制路径)。〔B6〕
- **W41 搜索框无范围提示**(搜标题/id/workspace)。placeholder 注明。〔B5〕
- **W42 fork 产物命名 `<ws>-fork-<id>` 继续污染项目列表**(与 W2
  同治:友好标签)。〔B3〕
- **W43 上传/附件错误路径未走查**(>10MB、非法类型的 toast 文案)。
  审查补齐。〔B6〕
- **W44 无 favicon/未读 title 徽标**(title.ts 只有静态标题?审查)。〔B6〕
- **W45 Composer 高级启动器(/goal /loop 等 slash)无发现引导**
  (菜单与 slash 双轨,slash 无提示)。〔B6〕

### daemon/runtime 层(另行 Go 侧修复或登记 GAPS)

- **W46 `ar ps` 泄漏(=W3 根因)**:子 agent 异常终止不摘除
  in-flight 条目。〔B2 Go〕
- **W47 `ar inspect` children 重复条目**(call_6_0 出现两次;UI 靠
  dedupe 掩盖)。〔B2 Go 或 GAPS〕
- **W48 sub-agent worktree 空快照时机**(=W4 根因):spawn 快照早于
  队友 sync-back,团队协作即坏。设计层,登记 GAPS/独立增量。〔GAPS〕
- **W49 spawn_agent detail 丢 task 文本**(=W7 根因半)。〔B2 Go〕
- **W50 孤儿 agent 熔断缺失**(=W4):父会话 quiescent 后子 agent
  仍可长跑无上报通道。登记 GAPS。〔GAPS〕

## Spec delta

- 「webui composer 权限 pill」:默认值 full → **ask**(W15);记住
  上次选择(localStorage)。SPEC webui 节补一行"默认 ask、可记忆"。
- 「webui Changes 视图」:验收锚增加"auto-created workspace 的新
  文件在 Changes 可见"(W1/W2)。
- 「webui 审批」:增加 always 动作(W11,复用 INC-17 CLI 能力,
  不新增 daemon 行为)。
- 其余条目均为既有功能点的缺陷修复/文案打磨,不动 SPEC 登记状态。

## Design delta

- 不触任何 DESIGN 不变量。W48/W50 涉及 runtime 设计的,只登记
  GAPS,本增量不改语义。
- `handleWorkspace` 落位与 git init(W1/W2)是 webui 便利层行为,
  DESIGN 无此层不变量;在 webui/README 更新说明。

## 验收

- 闸门 A:`./scripts/check.sh` 全绿(含 webui 前端 build+测试、Go 测)。
- 闸门 B(真实 API,共享 daemon):
  1. 新任务(默认 ask)→ 审批卡 approve → Changes 显示新文件 diff
     (W1/W2/W15 链验收);
  2. 复访 `20260710-043858-task-44c3`:Background work 不再挂僵尸、
     Subagents 显示人话状态、Attention 反映异常(W3/W6/W35);
  3. 侧栏排序稳定、新任务标题/时间可辨(W8/W9/W10)。
  产物归档 `qa/runs/2026-07-10-INC23/`。

## 实施步骤(批次 = 提交)

- B1 P0 workspace/Changes:W1 W2(meta.go/api.go + DiffView 文案)
- B2 supervision/弹层/裸码:W3 W5 W6 W7 W13 W18 W23 W35 W46 W47 W49
- B3 侧栏/时间线信息架构:W8 W9 W10 W14 W17 W19 W20 W28 W33 W42
- B4 composer/审批/run 模态:W11 W15 W16 W25 W30 W31
- B5 文案与小修:W21 W22 W24 W26 W29 W32 W34 W38 W39 W41
- B6 收尾打磨:W36 W37 W40 W43 W44 W45(酌情裁剪至 Round 2)
- 收口:GAPS 登记 W12 W48 W50;LOG 台账;QA 归档;工作纸归档。

## review 裁决

批量 UI 缺陷修复,不触不变量;裁掉三视角对抗 review,以闸门 B 真实
走查复验代替(每批修完即真浏览器复查)。W15(默认权限)行为变化已在
Spec delta 单列,随批 4 提交时在 LOG 记决策。

---

## 执行记录(2026-07-10 收口)

**完成**:B1(W1/W2)、B2(W5/W6/W7/W13/W18/W23/W35)、B3(W8/W9/W10/
W14/W19/W20/W28/W42 + chip 人话)、B4(W11/W15/W16/W25/W30)、B5
(W22/W24/W26/W29/W34/W38/W39/W41)。提交 INC-23.B1–B5;QA 归档
`qa/runs/2026-07-10-INC23/`。并发 session 依本工作纸完成另一路
B3–B6(Codex 结构 + restart-safe Scheduled,QA-32),其中 W12(runs
重启不可见)与 W17(双 Background work 同名)由其方案关闭;冲突合并
保留双方净改进。

**走查更正**:
- W3 撤回"ps 僵尸泄漏"定性——弃子 revive 后真实在跑,journal 折叠
  正确;真问题=弃子无人回收 → GAPS **G25**。
- W31 撤回——Deny 已有两步 reason 输入,走查初期未触发。
- W48/W50 → GAPS **G24/G25**;W47 → **G26**。

**Round 2 移交(B6 及剩余)**:W21(inspect 结构化视图)、W36(菜单
键盘导航)、W37(Close 确认改 app 风格)、W40(项目组右键)、W43
(附件错误路径)、W44(favicon/title 徽标)、W45(slash 引导)、W9
的"生成短标题"部分(需 LLM 摘要或 daemon 支持)、W33 全局色板审计
的剩余细项。连同对方 QA-32 未尽项,由下一轮走查开纸。
