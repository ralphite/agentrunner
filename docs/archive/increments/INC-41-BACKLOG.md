# AgentRunner webui — Codex parity backlog（细颗粒可并发任务库）

> **canonical 位置**：本文（`docs/increments/INC-41-BACKLOG.md`，已提交）。
> 工作纸见 `docs/increments/INC-41-codex-ui-polish.md`；截图证据在
> `qa/runs/2026-07-10-codex-ui-study/screenshots/`（gitignored，**不提交**）。

**这是什么**：以本机 Codex 桌面 app 实测规格（同目录
`INC-41-CODEX-UI-REFERENCE.md`，下称 REF，引用写 `REF §x`）为标尺，把
webui 打磨到同等质感的**穷举任务库**。比工作纸的 W1–W11 更细：每条
task 自足、带 behavior/截图/文件范围/验收/依赖，供多个子 Agent **并发**
认领。

**截图约定**：所有截图在 `qa/runs/2026-07-10-codex-ui-study/screenshots/`
（已 `.gitignore`，**不提交**）。"现状图"= 我方 webui 当前渲染；Codex
目标无法截图（宿主窗口受保护），以 REF 文字规格为准。新做的验证图也落
同目录，命名 `<task-id>-*.png`。

**功能对齐规则**（用户裁决）：双方都有的功能按 Codex 做；核心差异功能
不强行对齐；我方独有功能沿用 Codex 风格。每条 task 标了这条规则的适用。

**并发纪律**：
- 每条 task 列 `touches:` 文件；**同一文件被两条 task touch = 不可并发**，
  必须串行或合并认领。文末「并发分组」给出安全批次。
- `styles.css` 是全局热点：所有 task 的新样式**只追加到文件末尾**，用
  `/* ===== <task-id> ===== */` 注释块包裹，禁改中部既有规则——这样多条
  task 对 styles.css 的改动是 append-only、可无冲突合并。
- `viewModels.ts` / `timeline.ts` / `api.ts`：**只允许追加新导出**，不改
  既有函数签名。
- 不 commit / push（主线统一提交）；不动 `webui/frontend/dist`（验证后
  `git checkout HEAD -- webui/frontend/dist` 还原）。
- 真验：node 24 → `cd webui/frontend && npx vitest run && npm run build`
  → `cd webui && go build -o /tmp/arwebui-<id> .` → 起私有端口
  （8813+，避开 8809 主实例）跑 playwright（venv 见文末）→ 收工 kill。

**状态图例**：☐ 待做 · ◐ 进行中 · ✅ 完成(附 commit) · ✂ 裁掉(附理由) ·
🔒 有前置依赖(阻塞)。

---

## 已完成基线（勿重复）

INC-19..40 + INC-41.1（f9e9e65）已实现：Codex 框架/sidebar/项目分组/
New task 环境条四控件/审批卡/Changes→Review/命令面板/深浅主题/responsive/
Worked 真折叠(W2)/活动聚合+Shell 块(W3)/hover 时间戳/Home 欢迎态(W1)/
项目名消歧(W4)。工作区未提交(待整合)：W5 diff 行号+hunk+badge、W6 goal
终态 banner、W7 Scheduled 过滤、W8 命令面板 ⌘1-9、W9 lightbox、W10 滚底
浮钮+状态点收敛。**本 backlog 只列这些之外的**。

---

## A 组 · Timeline / 对话流（touches: Timeline.tsx / timeline.ts / Markdown.tsx / styles.css）

> ⚠ A 组多数 task 都 touch Timeline.tsx —— **组内不可并发**，须串行认领或
> 由同一 Agent 顺序做。可与 B/C/D/E/F/G 组并发。

### A1 ✅ 活动行图标化（REF §3 "活动行样式"）
**behavior**：Worked fold 展开后的活动聚合行（现为纯文字 "Ran commands /
Read files / Edited files"），每行左侧补一个 Codex 式小图标（terminal=
bash、doc=read、pencil=edit、magnifier=grep/glob/semantic、globe=
web_fetch、robot=spawn、chat=send_message）。单条工具行(step summary)已有
StepIcon 表状态；本 task 是**聚合行(act-group summary)**的类别图标，与状态
图标不同语义。
**现状图**：`screenshots/81-w2-fold-expanded.png`（展开后聚合行无图标）。
**touches**：`components/Timeline.tsx`（ActivityGroup/groupLabel）、
`styles.css`。
**验收**：多类别 turn（bash+read+edit）展开后每聚合行有对应图标；单类别
仍聚合；icon 与文字对齐；深浅主题；vitest 覆盖 groupLabel→icon 映射。

### A2 ✅ 工具专属 detail 渲染器（REF §3 三级展开）
**behavior**：当前非 bash 工具的三级 detail 一律 `<pre>{JSON}</pre>`。
按工具类型定制：
- `read_file`：显示 `path` + 若有 `line_range` 显示行段；结果折叠为
  "N lines" 而非糊一大坨 JSON。
- `edit_file` / `write_file`：把 args 里的 old/new 或 content 渲染成
  mini unified diff（复用 D 组 parseFileDiff），不显示裸 JSON。
- `grep` / `glob`：显示 pattern + "N matches in M files"，结果按文件分组。
- `web_fetch`：显示 URL（可点）+ 抓到的 title/字节数 + untrusted 标记。
- `semantic_search`：显示 query + top-N 结果路径。
- `spawn_agent`：显示 agent/role + task 摘要 + "open sub-session" 链接。
- `ask_user`：显示问题文本。
**现状图**：`screenshots/82-w3-shell-block.png`（只有 bash 有 Shell 块，
其余是裸 JSON）。
**touches**：`components/Timeline.tsx`（ToolCard/新增各 detail 组件）、
`diffSummary.ts`(复用,只读)、`styles.css`。
**依赖**：edit_file 渲染 🔒 依赖 D 组 `parseFileDiff` 已导出（已在
diffSummary.ts，可直接 import）。
**验收**：各工具类型有专属 detail；真实 session（有 read/edit/grep）逐个
断言；未知工具回落原 JSON；vitest 覆盖每种渲染器纯函数部分。

### A3 ✅ compaction 作为 fold 内活动行（REF §3 "Context automatically compacted"）
**behavior**：`context_compacted` 现渲染成独立 chip（现已 workChip 折进
fold）。Codex 把它渲成一条**活动行**样式（图标 + "Context automatically
compacted"），与工具聚合行同视觉层级，而非气泡 chip。
**touches**：`components/Timeline.tsx`、`styles.css`。
**验收**：有 compaction 的 session（qa store 里 goal/长 session）展开
fold 后 compaction 呈活动行样式；vitest 断言其 fold 归属不变。

### A4 ✅ "Thinking" 流式指示器对标（REF §3 turn 间态）
**behavior**：现 typing 态是 `.bubble.typing` 灰气泡。Codex 是一行不带
气泡的 "Thinking" 灰字（斜体/带微动效），流式产出后替换为正文。统一成
Codex 形态；running turn 的活动实时区（未折叠尾）保持可见。
**现状图**：无（需起真实 turn 抓；起一个 Gemini turn 时截）。
**touches**：`components/Timeline.tsx`、`styles.css`。
**验收**：真起一个 Gemini turn，流式期间显示 "Thinking" 行、产出后消失；
console 0。

### A5 ✅ "Sent as goal" 用户消息注记（REF §3 用户气泡下注记）
**behavior**：当某条 user 消息触发了 goal_attached（该消息即 goal 文本），
在该 user 气泡下方加一行灰字注记 `⚡ Sent as goal`（Codex 形态）。数据：
foldEvents 里 goal_attached 的 goal 文本与最近 user input 文本匹配即挂。
**现状图**：`screenshots/40-session-goal.png`（现为独立 "goal attached"
chip，未挂在用户气泡下）。
**touches**：`timeline.ts`（foldEvents 关联）、`components/Timeline.tsx`、
`styles.css`。
**验收**：goal session 里发起 goal 的那条 user 消息下显示注记；非 goal
消息无；vitest 覆盖关联逻辑。

### A6 ✅ Markdown 渲染 Codex 化（REF §3 正文）
**behavior**：assistant 正文 markdown 对标 Codex：内联 code = 灰底圆角
chip（部分已有,校准 padding/圆角/色）、代码块 = 带语言标签+复制按钮的
块、有序/无序列表间距、表格边框、链接色=accent。当前 Markdown.tsx 较素。
**现状图**：`screenshots/31-session-diff-bottom.png`（正文内联 code chip
可见,块级样式待校）。
**touches**：`components/Markdown.tsx`、`styles.css`。
**并发**：**不 touch Timeline.tsx**，可与 A 组其余并发（唯一例外）。
**验收**：渲染含代码块/表格/列表/内联 code 的消息,逐项视觉对标；代码块
有复制钮且可用；深浅主题；console 0。

### A7 ✅ 滚动锚定与 fold 展开态保持
**behavior**：现 fold 展开是本地 useState，poll 刷新（SSE 每次 fold
重算）后若 key 变可能塌回。确保 WorkedFold/ActivityGroup/step 的展开态
按稳定 key 保持,poll 不重置用户已展开的 fold。长 session 展开后滚动位置
不跳。
**touches**：`components/Timeline.tsx`。
**验收**：展开一个 fold,等一次 poll(2-3s),仍展开；DOM 断言。

---

## B 组 · 右侧上下文面板（touches: SessionView.tsx / SupervisionPanel.tsx / 新组件 / styles.css）

> Codex thread 视图右侧是常驻上下文面板（REF §4）：Environment /
> Background processes / Browser / Sources 四区。我们对应物是 Supervision
> 面板（Goal/Agents/Attention/Background）。本组把它对标 Codex 的分区
> 视觉,并补 Environment 区。**核心差异**：Browser/Sources 是 Codex 生态
> 件,裁掉。

### B1 ✅ Supervision 面板分区视觉对标（REF §4 分区样式）
**behavior**：现 Supervision 面板（`screenshots/40-session-goal.png` 右侧）
分区是 Goal/Agents/Attention/Run details。对标 Codex 右面板的分区样式：
小标题灰字 + 行条目 + 图标列对齐 + 分隔线。信息架构不变,只改视觉密度与
排版使其与 Codex 右面板一致。
**现状图**：`screenshots/40-session-goal.png`、`33-supervision.png`。
**touches**：`components/SupervisionPanel.tsx`、`styles.css`。
**验收**：goal/父子/attention 三种 session 下面板分区视觉对标；深浅主题；
console 0。

### B2 ✅ Environment 区（Changes/Worktree/branch 操作）（REF §4 Environment）
**behavior**：在 Supervision 面板顶部或独立区补 Codex 式 Environment 区：
`Changes +N -N`（点进 Changes 视图,复用现有 diff summary 数据）、当前
`Worktree`/`branch` 只读显示、`Create branch` 与 `Commit or push` 入口
（复用现有 AR.commit / git API；不可用时灰显）。
**现状图**：无对应物（我们无此区）。REF §4 是目标。
**touches**：`components/SupervisionPanel.tsx` 或新
`components/EnvironmentPanel.tsx`、`api.ts`(只读复用)、`styles.css`。
**依赖**：branch/diff API 已有（AR.gitBranches/AR.diff/AR.commit）。
**验收**：有改动的 session 显示 Changes 计数并可点入；branch 真实；
非 repo session 优雅退化；console 0。

### B3 ✅ Background processes 区（REF §4 Background processes）
**behavior**：Codex 右面板列运行中的后台进程（我们的 bash background
工具起的 bg 进程）。补一区列出当前 session 的 bg 活动（timeline 里
`background:true` 的 ToolItem status=running/task）+ 其 handle,可点 kill
（复用 task_kill 语义,但需确认现有 API 是否暴露；无则只读列出并记档缺口）。
**touches**：`components/SupervisionPanel.tsx`、`styles.css`。
**依赖**：🔒 kill 交互依赖后端 API（可能缺,先只读列出）。
**验收**：起一个 bg bash 的 session 显示该进程行；纯只读版可先落地。

### B4 ✂ Browser / Sources 区
**理由**：Codex 内嵌浏览器 tab 与 clipboard sources 是 ChatGPT 生态件,
PARITY §08 已裁 🧊。不做。

---

## C 组 · New task composer（touches: Composer.tsx / Popover.tsx / styles.css）

> 环境条四控件(W1/W4)已对标。本组补 composer 其余细节。**组内多数 touch
> Composer.tsx,不可并发**。

### C1 ✅ `+` 附件菜单对标（REF §2 `+` 菜单）
**behavior**：Codex `+` 菜单分组 `Add`（Files and folders / Goal—Set a
goal / Plan mode）+ `Plugins`（各带描述）。我们的 `+` 菜单
（`screenshots/15-add-menu.png`）对标：分组化 + 每项一句灰字描述；我们的
Goal/Loop/Best-of-N/persona 归入 Task options 分组（保留我方独有,套 Codex
菜单样式）。
**现状图**：`screenshots/15-add-menu.png`、`19-task-options.png`。
**touches**：`components/Composer.tsx`、`styles.css`。
**验收**：菜单分组+描述行对标；各项可用；键盘可达；console 0。

### C2 ✅ 权限模式菜单文案/图标对标（REF §2 权限模式菜单）
**behavior**：Codex 权限菜单有标题 "How should ChatGPT actions be
approved?" + 4 项(图标+标题+描述): Ask for approval / Approve for me /
Full access / Custom。我们的 access 菜单（`screenshots/16-access-menu.png`）
对标：加标题、每项图标+一句描述；映射我方权限模型（ask/auto/full/spec）。
**现状图**：`screenshots/16-access-menu.png`。
**touches**：`components/Composer.tsx`、`styles.css`。
**验收**：菜单标题+描述+图标对标；选择生效（真起 turn 验权限）；console 0。

### C3 ✅ model 菜单分组打磨（REF §2 模型选择器；保留菜单不改滑杆）
**behavior**：**核心差异**——我们是多 provider 离散模型（Gemini/Anthropic/
custom），不采 Codex effort 滑杆。但把现 model 菜单
（`screenshots/17-model-menu.png`）的分组视觉打磨到 Codex 质感：Model 组
（每项名+一句能力描述+✓）+ Reasoning effort 组（Off/Light/Medium/High/
Extra High + 描述+✓）+ Custom model id 入口。已有内容,只校排版/间距/图标。
**现状图**：`screenshots/17-model-menu.png`。
**touches**：`components/Composer.tsx`、`styles.css`。
**验收**：两组视觉对标；选择生效；console 0。

### C4 ✅ slash 命令菜单打磨（REF §2）
**behavior**：`/` 菜单（`screenshots/18-slash-menu.png`）对标 Codex 命令
列表样式：命令名 mono + 描述 + 右侧参数提示；键盘上下选择高亮。
**touches**：`components/Composer.tsx`、`styles.css`。
**验收**：`/` 触发菜单样式对标；键盘可选；console 0。

### C5 ✅ 附件缩略图 chip 样式（REF §2）
**behavior**：composer 已选附件的缩略图 chip（`.cx-att`）对标 Codex：
圆角、hover 显 × 删除、非图片显文件图标+名。
**touches**：`components/Composer.tsx`、`styles.css`。
**验收**：贴图/贴文件后 chip 样式对标；删除可用。

### C6 ✅ mobile 环境条换行与可达性回归（REF §2 390px）
**behavior**：390px 下环境条四控件换行、composer 底行 access/model/mic/
send 全可达（INC-40 曾复现裁切）。W1 已改 Home 布局,复验 mobile composer
未回归。
**现状图**：`screenshots/50-mobile-home.png`、`51-mobile-session.png`。
**touches**：`styles.css`（可能 Composer.tsx 微调）。
**验收**：375/390px 下所有控件可点、不裁切、console 0。

---

## D 组 · Diff / Review（touches: DiffView.tsx / ChangesOutcome.tsx / diffSummary.ts / styles.css）

> W5（行号+hunk+badge）已在工作区。本组是 W5 之外的 review 增强。

### D1 ✅ 范围切换 Working tree | Last turn（REF §5 范围下拉）
**behavior**：Codex review toolbar 有范围下拉（Unstaged/Staged/Commit/
Branch/Last Turn）。我们最小对标 `Working tree | Last turn` 两档：Working
tree=现有全量 diff；Last turn=最后一个 human turn 的改动。
**落地**（INC-57 / QA-60）：复用 loop-owned `bar-tN` snapshot（显式
`bar-m*`/`bar-final` 不可冒充开工 baseline），`SnapshotStore.Diff` 临时 index
只读比较；CLI/API 结构化 available/reason；前端范围 menu 全接通，Last turn
不显示会误提交全 workspace 的 Commit/Apply/Remove。desktop/mobile ×
light/dark、Escape/focus、历史 session unavailable、console 0 均真验。
**touches**：`internal/snapshot`、`internal/cli/diff.go`、`webui/meta.go`、
`components/DiffView.tsx`、`api.ts`、`types.ts`、`styles.css`。
**验收**：TestShadowRepoDiffAgainstSnapshot / TestCLIDiffLastTurnJSON /
TestHandleDiffLastTurn / frontend api test / QA-60。

### D2 ✅ 文件搜索 + 文件树切换（REF §5 toolbar 图标）
**behavior**：多文件 diff 时,toolbar 加文件搜索框（过滤文件段）+ 文件树/
列表切换图标。现 DiffView 是平铺 details 列表。
**现状图**：`screenshots/32-changes.png`（单文件,需多文件 session 验）。
**touches**：`components/DiffView.tsx`、`styles.css`。
**验收**：多文件 session（如改多文档的）搜索过滤生效；console 0。

### D3 ✅ diff 语法高亮（REF §5 "markdown 语法高亮保留"）
**behavior**：diff 行内按文件扩展名做轻量语法高亮（关键字/字符串/注释）。
最小实现：按扩展名选 highlighter,只高亮非 +/- 前缀后的内容,保留增删底色。
**touches**：`components/DiffView.tsx`、`styles.css`、可能新增轻量
highlighter 工具（无外部依赖,inline 规则）。
**验收**：.go/.ts/.md diff 有高亮；不引入外部包（CSP/体积）；console 0。

### D4 ✅ inline / split 视图切换（REF §5 分屏图标）
**behavior**：toolbar 加 inline(现状)↔split(左删右增)切换。split 用
parseFileDiff 的 old/new 行号对齐两列。
**touches**：`components/DiffView.tsx`、`diffSummary.ts`(只加导出)、
`styles.css`。
**验收**：切 split 后左右列对齐；窄屏回落 inline；console 0。

---

## E 组 · Sidebar（touches: Sidebar.tsx / viewModels.ts / styles.css）

> W4(项目名)、W10(状态点)已改。本组是其余 sidebar 细节。**组内 touch
> Sidebar.tsx,不可并发**。

### E1 ✅ Pinned 分组（REF §1 Pinned）
**behavior**：Codex sidebar 有独立 `Pinned` 分组(置顶任务扁平列表)在
Projects 之上。我们有 pin 能力(task-pin)但无独立 Pinned 区——把已 pin 的
task 提到顶部独立分组渲染。
**现状图**：`screenshots/10-home.png`（无 Pinned 分组）。
**touches**：`components/Sidebar.tsx`、`viewModels.ts`(加分组纯函数)、
`styles.css`。
**验收**：pin 一个 task 后它出现在 Pinned 分组；unpin 后回原项目；vitest
覆盖分组。

### E2 ✅ 账户底栏对标（REF §1 底部账户行 / §7）
**behavior**：Codex 底栏 = 圆形头像(缩写)+名字+右侧 `?` 帮助。我们底栏
现为 `● Connected · dev` + 主题切换。对标：左圆形徽标(可用 app 图标或
user 缩写)+ 名字/连接态 + 右侧 help/主题图标；daemon 状态并入徽标色。
**现状图**：`screenshots/10-home.png` 左下 "Connected · dev"。
**touches**：`components/Sidebar.tsx`、`styles.css`。
**验收**：底栏视觉对标；daemon 挂时红态可点重启不丢；深浅主题。

### E3 ✅ 导航项未读蓝点（REF §1 未读蓝点）
**behavior**：Codex 导航项(New task/Scheduled…)有新活动时右侧蓝点。我们
task 行有 unread,但顶部导航(Scheduled 等)无。给 Scheduled 等导航项在有
未读运行时加蓝点。
**touches**：`components/Sidebar.tsx`、`styles.css`。
**验收**：有未读 scheduled 运行时 Scheduled 导航项显蓝点；读后消。

### E4 ✅ hover 预览卡打磨（REF §1 预览卡）
**behavior**：现 hover 预览卡（`task-preview`）已有 title/time/project/
branch/status。对标 Codex 预览卡：圆角/阴影/间距/图标对齐校准,与 Codex
质感一致。
**现状图**：`screenshots/20-sidebar-hover.png`。
**touches**：`components/Sidebar.tsx`、`styles.css`。
**验收**：hover 任一 task 预览卡视觉对标；不越屏；console 0。

### E5 ✅ 项目折叠动画与 "Show N more"（REF §1）
**behavior**：项目分组展开/折叠加轻过渡；每项目超阈值(现"Show 49 more")
的 "Show N more" 样式对标 Codex(灰字行,点开增量)。
**touches**：`components/Sidebar.tsx`、`styles.css`。
**验收**：折叠/展开平滑；Show more 增量加载；console 0。

---

## F 组 · Scheduled（touches: Scheduled.tsx / styles.css）

> W7(过滤 tab+next-run)已在工作区。本组是其余。

### F1 ✂ Suggestions 模板卡
**理由**：Codex 的 Daily brief/Weekly review 模板是 ChatGPT 生态糖,与
AgentRunner driver/goal 语义不同,不硬凑。裁掉。

### F2 ✅ "Mark all as read" + 未读态（REF §6）
**behavior**：Scheduled 页顶加 `✓ Mark all as read`；未读运行行左侧蓝点,
点 mark 后清。
**touches**：`components/Scheduled.tsx`、`styles.css`。
**验收**：有未读运行时可 mark all；蓝点清除；console 0。

### F3 ✅ Create 下拉入口（REF §6 右上 Create）
**behavior**：Scheduled 页右上 `Create ⌄` 下拉(New schedule / 从模板)对标
Codex；至少 New schedule 入口样式化。
**touches**：`components/Scheduled.tsx`、`styles.css`。
**验收**：Create 下拉可用,New schedule 走现有流程。

---

## G 组 · Goal（touches: SessionView.tsx / timeline.ts / styles.css）

> W6(终态 banner)已在工作区。本组是其余 goal 细节。⚠ touch SessionView.tsx,
> 与 B 组注意错开。

### G1 ✅ banner elapsed 实时显示（REF §3 goal banner `· {elapsed}`）
**behavior**：活跃 goal banner 显示 elapsed(attach 起算,秒级 tick 或
分钟粒度)；终态显总耗时。W6 若只做终态,本 task 补活跃态 elapsed。
**touches**：`components/SessionView.tsx`、`timeline.ts`(deriveGoalState,
只加导出)、`styles.css`。
**验收**：活跃 goal(需真起或用现有 attached session)banner 显 elapsed;
终态显总时长；vitest 覆盖 elapsed 计算。

### G2 ✅ banner inline edit goal（REF §3 ✎ Edit goal）
**behavior**：banner 右侧 ✎ 点开可改 goal 文本(复用 AR goal update API,
api.ts 已声明 "update")。
**依赖**：api.ts 有 update 声明但"无组件调用"(见 CODEX-PARITY §6.2⑥)。
**touches**：`components/SessionView.tsx`、`api.ts`(接通现有声明)、
`styles.css`。
**验收**：点 ✎ 改文本→真实 update→banner 更新；console 0。

### G3 ✅ goal 进度/预算显示（REF §6.2④ 记档 defer 的余项）
**behavior**：banner 显示 N/M checks(有 verifier 时)或 token/墙钟预算(若
后端提供)。**核心差异+依赖**：token/墙钟预算后端未实现(INC-D1 defer),
本 task 只做 checks 计数显示,预算标 TODO。
**touches**：`components/SessionView.tsx`、`styles.css`。
**验收**：有 verifier 的 goal 显 N/M checks；无则只显状态。

---

## H 组 · Settings 页（大区,我方几乎空白；touches: 新 Settings*.tsx / App.tsx / styles.css）

> Codex Settings 是全窗接管的分组页(REF §7)。我们目前只有 sidebar 底栏
> 主题切换,无 Settings 页。本组从零搭。**核心差异**：Usage&billing/
> Account/Pets/Voice/Plugins marketplace 裁掉;只做与我方模型相关的。
> H1 是骨架,H2+ 依赖它 🔒。

### H1 ✅ Settings 页骨架 + 左导航分组（REF §7 左栏）
**behavior**：新增全窗 Settings 视图(route 或 modal),左栏分组: General /
Appearance / Keyboard shortcuts / Git / Worktrees / Configuration;顶部
"← Back to app" + 搜索框。入口:sidebar 底栏 help/齿轮 或 ⌘,。
**touches**：新 `components/Settings.tsx`、`App.tsx`(route)、`styles.css`。
**验收**：⌘, 或入口打开 Settings,左导航可切,Back 返回；深浅主题；console 0。

### H2 ✅ Appearance 设置（REF §7 Appearance / §0 tokens）
**behavior**：Theme 三选(System/Light/Dark,带迷你预览)、UI 字号、代码
字号、对比度滑杆、Diff markers(Color/+-)、Reduce motion。落到现有主题
系统(theme.ts)+ CSS 变量。
**依赖**：🔒 H1 骨架。
**touches**：`components/Settings.tsx`、`theme.ts`、`styles.css`。
**验收**：改主题/字号即时生效并持久(localStorage)；深浅主题；console 0。

### H3 ✅ Keyboard shortcuts 查看页（REF §7 Keyboard shortcuts）
**behavior**：列出当前键绑定(shortcuts.ts 已有映射)成只读清单(动作+
快捷键 chip)。重绑可后续,先只读展示。
**依赖**：🔒 H1。
**touches**：`components/Settings.tsx`、`shortcuts.ts`(只读导出)、`styles.css`。
**验收**：清单列全当前绑定；分组；console 0。

### H4 ✅ Git 设置（REF §7 Git）
**behavior**：branch prefix、PR merge method、commit/PR instructions 文本框。
**核心差异**：我们 git 走 bash,部分设置可能无落点——只做有真实落点的
(如 commit message 模板注入),其余记档。
**依赖**：🔒 H1 + 需确认后端有无落点。
**touches**：`components/Settings.tsx`、`api.ts`、`styles.css`。
**验收**：有落点的设置真实生效；无落点的标 TODO 不伪造。

### H5 ✅ Worktrees / Configuration 管理（REF §7）
**behavior**：Worktrees 列 + 删除(复用现有 fork/worktree 数据)；
Configuration 显示 approval policy/sandbox(只读或接通现有 trust/policy)。
**依赖**：🔒 H1。
**touches**：`components/Settings.tsx`、`api.ts`、`styles.css`。
**验收**：worktree 列真实；配置只读展示准确；console 0。

---

## I 组 · 框架 / chrome / 微件（touches: App.tsx / 各处 / styles.css）

### I1 ✅ thread 头部窗口图标（REF §1 右上窗口图标）
**behavior**：thread 视图右上补 Codex 式图标:toggle side panel(
Supervision 收放,我们有)、popout(可选)。至少 side panel 收放图标对标。
**touches**：`components/SessionView.tsx`、`styles.css`。
**验收**：图标可收放 Supervision；tooltip；深浅主题。

### I2 ✂ context window / usage 指示器（REF §7 "Show context window usage"）
**behavior**：composer 附近显示上下文用量(已用/上限,来自 journal usage
事件)。Codex 可选开关;我们默认显示一个细指示。
**touches**：`components/SessionView.tsx` 或 `Composer.tsx`、`styles.css`。
**依赖**：需 journal 有 token usage(有,activity usage/subagent usage)。
**验收**：真 session 显示用量;无数据时不显;console 0。
**裁决**：journal 只有累计 billed/input/output，没有当前 provider 的 context
capacity，也没有 compaction 后的 live context occupancy。现有 thread header 已
如实显示 billed tokens/steps；把它画成 context-window 进度会误导，故在后端
契约补齐前裁掉，不画假百分比。

### I3 ✅ 内联 usage/error 横幅样式（REF §3 usage 警示卡）
**behavior**：`limit_exceeded` / usage 撞限的 chip 升级为 Codex 式警示卡
（图标+标题+说明+行动钮)。我方无 billing,行动钮改为"稍后重试"提示,不画
Upgrade。
**touches**：`components/Timeline.tsx`(chip 渲染)或独立、`styles.css`。
**依赖**：⚠ 若 touch Timeline.tsx 与 A 组冲突,可抽为独立 chip 组件。
**验收**：撞限 session 显警示卡；无伪 Upgrade 钮；console 0。

### I4 ✅ Toast / 空态 / loading skeleton 打磨（REF §6 空态）
**behavior**：Toasts.tsx、各空态("No tasks yet"等)、sidebar loading
skeleton 对标 Codex 质感(圆角/间距/图标)。
**touches**：`components/Toasts.tsx`、`components/Sidebar.tsx`(空态)、
`styles.css`。
**验收**：触发 toast/空态/加载态,视觉对标；console 0。

### I5 ✂ 底部终端抽屉（⌘J）
**理由**：REF §1 的 bottom terminal drawer 是产品增量(PARITY §02 "UI 无
终端面板"),非 polish。留独立 INC,不进本 backlog。

### I6 ✅ 全局键盘快捷键补全（REF §7 Keyboard shortcuts 清单）
**behavior**：对标 Codex 键位补我方缺的全局快捷键(New task ⌘N 已有、
Toggle side panel、Next/Prev task 等),融入 shortcuts.ts。
**touches**：`shortcuts.ts`、`App.tsx`、`styles.css`(chip)。
**依赖**：⚠ W8 已动 shortcuts.ts(⌘1-9),需在其基础上加,避免冲突→W8 整合
后再认领。
**验收**：新增快捷键可用;不与既有冲突;console 0。

---

## J 组 · 补漏面（REF 逐节核查后新增；touches 见各条）

> 第二轮对照 REF §1–§7 逐面核查,补前一版 A–I 未覆盖的真实差异。

### J1 ✅ thread 头部 `…` 菜单对标（REF §3 头部 `…`）
**behavior**：Codex thread 头部标题右侧 `…` 菜单：Pin task / Rename task /
Archive task ┃ Open side task / Copy ▸ / Continue in… ▸ / Add scheduled
task… / Open in new window。我们 thread 头（`screenshots/30-session-diff-top.png`
右上 Changes/Supervision/`…`）的 `…` 菜单需对标：至少 Pin/Rename/Archive/
Copy link/Continue in new task（复用现有 rename/pin/archive/continue 能力）。
Codex 生态项(Open side task/new window)按需裁。
**现状图**：`screenshots/30-session-diff-top.png`。
**touches**：`components/SessionView.tsx`、`components/ContextMenu.tsx`(复用)、
`styles.css`。
**验收**：`…` 展开菜单项对标,rename/pin/archive/continue 真实可用；键盘可达。

### J2 ✅ 文件产物 chips 与 "Open in" 菜单（REF §3 文件产物 chips）
**behavior**：Codex 把 turn 产出的文档文件渲成卡片行（图标 + 文件名 +
"Document · MD"）+ 右侧 `Open in ▾`（VS Code/Cursor/Default app/Terminal/
Show in Finder/Download a copy）。我们 turn 产出文件现只在 Changes 视图见,
timeline 里无产物 chip。补：从 write_file/edit_file 活动或 Changes 摘要
提取"本 turn 新建/改动的文件",在最终答案下渲文件卡 + Open-in 菜单(复用
现有 Open-in 能力,CLI 侧已有 `Open in` 语义,见现状 sidebar/Changes)。
**现状图**：`screenshots/31-session-diff-bottom.png`（现只有 Edited files
汇总卡,无逐文件 Open-in chip）。
**touches**：`components/Timeline.tsx`(或独立 FileArtifacts 组件)、
`components/ChangesOutcome.tsx`、`api.ts`(open-in,若有)、`styles.css`。
**依赖**：⚠ 若 touch Timeline.tsx 与 A 组冲突→抽独立组件挂在 outcomeSlot。
Open-in 后端能力需确认(可能只 Download a copy 可直接做,IDE 打开需 CLI)。
**验收**：产出文件的 session 显文件卡；Download 可用；无落点的 Open-in 项
灰显或不列(不伪造);console 0。

### J3 ✅ 审批卡视觉对标（REF §3 审批;我方独有能力套 Codex 风格）
**behavior**：我们审批卡（`screenshots/30-session-diff-top.png` 里的
"Approved" 绿签 + write/git status 行,`ApprovalCard.tsx`）是**我方独有的
治理面**(Codex 审批模型不同)。按规则"独有功能沿用 Codex 风格":待决审批卡
的圆角/间距/按钮(Approve once/Deny)/Details 折叠/gate 说明对标 Codex 卡片
质感;已决审批的绿签/红签行样式统一。**不改审批语义,只改视觉**。
**现状图**：`screenshots/30-session-diff-top.png`（Approved 绿签行）。
**touches**：`components/ApprovalCard.tsx`、`styles.css`。
**验收**：真实 waiting:approval session(default 模式起一个)卡片视觉对标;
Approve/Deny 真实生效(不代用户决策,人工点);深浅主题;console 0。

### J4 ✅ 归档任务视图（REF §6 Archived tasks）
**behavior**：Codex 有归档任务页(搜索 + All tasks/All projects 过滤 + 按
项目分组 + Unarchive + Delete all)。我们有 archive 能力(sidebar "Show
archived"),但无独立归档管理视图。补一个轻量归档视图:列已归档 task(按
项目分组)+ Unarchive。Delete all **裁掉**(我方无 durable delete 契约,
INC-38 已定不画伪删除)。
**现状图**：`screenshots/10-home.png`（sidebar 底部 "Show archived · N"）。
**touches**：`components/Sidebar.tsx`(或新 Archived.tsx)、`App.tsx`、
`styles.css`。
**依赖**：⚠ 与 E 组 Sidebar.tsx 冲突→建议新 Archived.tsx 独立视图。
**验收**：归档视图列已归档 task 可 Unarchive;分组;console 0。

### J5 ✅ 通知条 / daemon 提示样式（REF §2 composer 上方通知条）
**behavior**：Codex composer 上方可叠通知条(圆角卡,如 rate-limit reset、
usage 警示)。我方对应物 = daemon 状态/连接提示。把现有 toast/inline 提示
在 Home composer 上方对标 Codex 通知条样式(圆角卡 + 图标 + 文案 + 可关)。
**核心差异**:不画 Upgrade/billing 相关(裁),只做 daemon/连接/系统提示。
**touches**：`components/Home.tsx`/`Composer.tsx`、`components/Toasts.tsx`、
`styles.css`。
**验收**：daemon 挂掉/恢复时 Home 上方显通知条;可关;console 0。

### 显式裁掉的生态件（登记备查,勿再当缺口）
- 浮动 Chat 面板（REF §1）：ChatGPT classic chat,生态件,🧊。
- Plugins/Skills marketplace（REF §6）：🧊。
- Sites（REF §6）：🧊。
- Pull requests 页（REF §6）：依赖 GitHub 集成(G13/P2),非 UI polish,🧊。
- Usage & billing / Credits（REF §7）：无 billing 模型,🧊。
- Pets / Voice(dictation 外) / Computer use / Image Gen（REF §7）：🧊。
- effort 滑杆（REF §2）：多 provider 离散模型不适配,保留菜单(见 C3),🧊。

---

## 终局 · QA-43 全景验收（依赖上述批次收口）

### Z1 ✅ 全景对照 + 归档
**behavior**：1554/1440/900/642/390 × light/dark 全量截图,与 REF 逐屏
对照(REF §8 清单画勾),console 0 error/warning;三层文档/GAPS/LOG/
CODEX-PARITY 收口;证据归档 `qa/runs/2026-07-10-QA43-codex-ui-polish/`。
**落地**：Home/rich thread/approval/Scheduled/Settings/Changes 六个主态均做
desktop/mobile × light/dark；另补 1554/900/642 三档。真浏览器全景发现并修复
mobile Settings 打开后 sidebar/scrim 未收起；修后以可见 DOM 断言侧栏消失，
Changes 以 scope trigger + 真实文件名确认面板已挂载后才截图。最终 console
error/warning=`[]`，contact sheets 与逐屏原图全保留。
**第二镜头独立确认（2026-07-11 headless 轮4）**：另跑一个 read-only finder
新眼复查 live 8809，6 面 × light/dark × 1440/390 共 19 稳态组合 + 3 复检
console error+warning 全 0，无新 P1/P2（sidebar 主题竞态疑点经 3 次复检排除）；
独立证据 `qa/runs/2026-07-11-QA43-endgame/EVIDENCE.md`。
**依赖**：✅ A–I 组已逐项完成或显式裁掉；QA-43 PASS。
**touches**：docs/*、qa/*。

---

## 并发分组（子 Agent 认领指南）

**可同时跑的批次**（组间 touches 不重叠）：

- **批 α**：A6(Markdown.tsx) + B1/B2(SupervisionPanel/新组件) + C1(Composer)
  + D2(DiffView) + E1(Sidebar) + F2(Scheduled) + H1(新 Settings)
  —— 7 条,文件互不重叠,可同时认领。
- **批 β**（批 α 的 Sidebar/Composer/DiffView 收工后）：A1→A2→A3→A4→A5→A7
  串行(都 touch Timeline.tsx,一个 Agent 顺序做) ‖ C2→C3→C4→C5(Composer
  串行) ‖ D1→D3→D4(DiffView 串行) ‖ E2→E3→E4→E5(Sidebar 串行) ‖
  G1→G2→G3(SessionView 串行,与 B 组错开 SessionView)。
- **批 γ**（H1 收工后）：H2 ‖ H3 ‖ H4 ‖ H5（都 touch Settings.tsx →
  其实需串行；或 H1 把 Settings 拆成 sub-panel 文件让 H2-5 各占一个文件
  即可并发——**建议 H1 落地时按 panel 拆文件**）。
- **批 δ**：I1(SessionView) I2 I3 I4 I6 —— 注意 I3/I6 与 A 组/W8 的
  Timeline/shortcuts 冲突,末批做。

**冲突热点**（同文件,须串行/合并）：
- `Timeline.tsx`：A1 A2 A3 A4 A5 A7 I3 → 单 Agent 串行。
- `Composer.tsx`：C1 C2 C3 C4 C5 → 单 Agent 串行。
- `SessionView.tsx`：B2(可选) G1 G2 G3 I1 I2 → 协调错开。
- `DiffView.tsx`：D1 D2 D3 D4 → 单 Agent 串行。
- `Sidebar.tsx`：E1 E2 E3 E4 E5 I4(空态) → 单 Agent 串行。
- `Settings.tsx`：H1 先行,拆 panel 后 H2-5 各占文件。
- `styles.css`：全体 append-only(注释块隔离),可无冲突合并。

## 验证环境（所有 task 共用）

- node 24：`export PATH="$(ls -d $HOME/.nvm/versions/node/v24* | tail -1)/bin:$PATH"`
- 前端：`cd webui/frontend && npx vitest run && npm run build`
- 后端：`cd webui && go build -o /tmp/arwebui-<id> .`
- 私有实例：`/tmp/arwebui-<id> --addr 127.0.0.1:<8813+> --ar /tmp/ar-claude --env-file /Users/yadong/dev2/agentrunner/.env`（共享 daemon/store,勿动 8809 主实例）
- playwright venv：`/private/tmp/claude-501/-Users-yadong-dev2-agentrunner/b84daf52-9db3-44c9-8c46-9a5d9f61a6df/scratchpad/pwenv/bin/python`（system Chrome channel="chrome" headless,dsf=2;session 页 goto 用 wait_until="domcontentloaded"+sleep,networkidle 被 SSE 卡死）
- 常用真实 session：diff=`20260710-213428-create-qa42-worktree-browser-t-d8ac`,goal=`20260710-062102-create-a-file-goal-r2-txt-in-t-0d1e`,图片=侧栏"工作区里有一张 CI"。
- 磁盘紧张(约 1GB):勿留大文件,构建产物用完清；截图落 screenshots/(gitignored)。

## 状态台账（只追加）

- 2026-07-10 16:xx 建库。已完成基线 INC-41.1-2(W1-W10 全部 push)。
  本库列 A–J 共 ~45 条细任务 + 并发分组 + 裁掉件登记,供多 Agent 认领。
- 2026-07-10 16:xx 第二轮 REF 逐节核查,补 J 组(thread `…` 菜单/文件产物
  chips/审批卡打磨/归档视图/通知条)+ 显式裁掉件清单。

- 2026-07-10 深夜 · 5 切片 worktree 并发批(conv/composer/panel/nav/rs)整合:
  上表 ✅ 共 29 条(A1-3/5-7、C1-6、B1、J1、J3、G3、E1-4、F2、I4、
  H1-5、D2-4、I6)一次合并落地。整树 90 vitest 绿、18/18 端到端断言、
  console 0;CSS 打包顺序修正(styles.css 先于切片 CSS)。
  仍开放:A4(Thinking 措辞,跨切片)、B2(Environment 区,功能增量)、
  B3(已由 Background work 区覆盖大半)、I1(Supervision 钮已覆盖)、
  I2(需 max-context 数据)、E5 整段高度动画、F3(单项下拉价值低)、
  J2(文件产物 chips)、J4(独立归档视图)、J5(通知条)、D1(需后端契约)、
  I3(缺真实 limit 数据)、Z1(终局 QA-43)。上述后续均已在本台账对应条目
  完成或显式裁掉，Z1 于 2026-07-11 收口。

## K 组 · 四镜头审查发现(R1-R4,2026-07-10 深夜)

四个只读审查子 Agent(R1 结构/R2 视觉/R3 交互/R4 边角真实性)驱动真实 app
逐面挑刺,约 40 条。**A 相(已修,主线亲手)**——诚实性/正确性集群:
- [R4-1] ✅ subagent 误判:`sid.includes("-sub-")` 命中标题含"sub"的顶层
  会话→假只读徽标+死链+藏 composer;改判 `-sub-call_` 真子会话格式。
- [R4-2] ✅ 后端 firstLine 按字节切中文→乱码 `��`;改按 rune 截断(runs.go)。
- [R4-3] ✅ driver 迭代 chip 泄漏裸 JSON verdict + "completed·completed"
  重复;新增 verdictLabel humanize + friendlyStatus 归一。
- [R4-4/R3-10] ✅ limit_exceeded chip 泄漏 `tokens: 0/500`/`generation_steps`
  裸枚举且自相矛盾;走 friendlyStatus + "capped at N" 措辞。
- [R4-6] ✅ "Worked for 90m 31s" 把排队/空闲计入;改从 generation_started
  起算(TurnItem.ts),且任何 input(user/runtime 注入如 unix-socket)重置
  turn 边界。真验 90m→3s。
- [R4-7] ✅ stranded 会话对 GUI 用户弹 CLI 专属报错;webui 侧 guiReason
  改写 auto-deny 文案(不动后端,免污染 CLI)。
- [R4-8] ✅ chip 泄漏裸活动 id `activity cancelled revive-cmd-...`→"Wake-up
  cancelled"。
- [R2-7] ✅ 成功 chip 灰、失败有色的不对称;goal check/achieved 走 .chip.good。
- [R3-3] ✅ 错误 chip 里 sha256 长串窄屏溢出;.chip overflow-wrap:anywhere。
- [R1-6b] ✅ header 文件夹图标→文档图标(FileText),对齐 Codex thread。

**B 相(待派 worktree 切片)** 视觉 CSS:R2-1 composer 卡暗色硬编码边框、
R2-2 env-strip 拼缝、R2-3 选中色统一、R2-4 弹窗 max-height、R2-5 show
system events 图标、R2-6 AGENTS 空态图标、R2-8~12、R3-1 右键/hover 互斥、
R3-2 "No all work" 语法、R3-5 closed composer 提示、R3-6 Commit 按钮文案、
R3-7/R2-10 预览卡全标题、R3-8 移动端菜单顶部遮挡、R3-11 cmdk 空态盒、
R1-5 session 权限 pill、R4-5 untracked 目录改动可复核、R4-10 空气泡、
R4-11 空会话空态、R2-1 box-shadow 暗色。
**C 相(结构,单独精修)**:R1-1 正文列宽/面板不跳栏 ✅、R1-2 Changes 整屏→
右 split ✅、R1-3 右面板默认常显 ✅、R1-4 Supervision 显已完成 goal ✅。

- 2026-07-10 深夜续 · C 相结构项收口:R1-1/R1-3/R1-4 已 push;R1-2
  (INC-41.11)把 Changes 从整屏接管改成右侧 split 面板——对话 timeline +
  composer 常驻左侧,DiffView 抽入 `.changes-panel`(宽 minmax(520px,52%),
  窄屏 <900 覆盖全宽)。真验:对话/diff/composer 三者共存、X 关闭返回、
  1440/390 两档 console 0、92/92 vitest 绿。

## L 组 · 加载/空态/错误态审查发现(轮7 finder,2026-07-11)

read-only finder(镜头:加载/骨架态 + 空态/错误态)对 live 8809(vDlkgc7S)
playwright 取证。4 条均已 `git log -S` 核查排除刻意决策(全为非刻意)。截图
在 `qa/runs/2026-07-10-codex-ui-study/screenshots/`(gitignored)。

### L1 ✅ 会话内容加载无骨架屏,有历史的会话闪出假空态 [P1]
**修**(轮8,`2a25877`):`Timeline.tsx:862` 把 `isEmpty` 拆成 `blank`(无内容)与
`isEmpty = blank && !loading`;`blank && loading` 渲染 `.tl-skeleton`(1 user +
2 assistant shimmer 气泡,复用 sidebar skeleton keyframes)。`SessionView.tsx:746`
传 `loading={!eventsReady}`,`eventsReady` 在首次 poll 的 `finally` 置位(成败都算
「已返回」)且永不回退 → 已有消息后不闪骨架。**live 验**:限速硬导航富会话,骨架
出现、假空态 0 次采样、稳态 29 nodes 骨架消失。

**behavior**:硬导航到富会话(限速)时,首帧 120-900ms 消息区错误显示终局
文案 `No messages yet · This task hasn't started`,~2.5s 后真实消息整块弹入,
造成误导 + 布局跳动。loading 与 genuinely-empty 无法区分。
**源码**:`Timeline.tsx:856`(`isEmpty = nodes.length===0`)渲染于 `:861-866`;
`SessionView.tsx` 首次 poll 未回时无 `loading` 标志传入 TimelineView。
**建议**:给 TimelineView 传 `loading`(首次 poll 未回),loading 渲染 2-3 条
骨架气泡;仅 `loading===false && nodes===0` 才显示空态文案。
**touches**:`SessionView.tsx` / `Timeline.tsx` / `styles.css`。
**刻意核查**:`git log -S"No messages yet"`→ab4ef19(R4-11 只为替代空白 void,
未决定加载中展示终局空态)→非刻意。

### L2 ✅ 非法/不存在 session id 无错误态,伪装成正常空会话 + Supervision 永久 spinner [P1]
**修**(轮8,`2a25877`):新增 `NotFound.tsx`(`SessionNotFound` 错误卡,图标+标题+
副文案+回列表 CTA,走既有 `select(null)`)。取证发现**后端不返 404**——`webui/api.go:104
arFail` 把 `ar` 失败统一包 502,真判据在 stderr `agentrunner: no session matches "<id>"`;
故 `SessionView.tsx:41 isSessionNotFound()` 只匹配这句(瞬时错误如 Failed to fetch 不
误入错误态,维持 best-effort 重试),命中后 `:596` 早返回渲染错误卡(无 composer)并用
`gone` ref 停掉两个轮询与 SSE 重连(原本每秒 spawn 一个 `ar` 子进程)。spinner 独立修:
`setInspectReady(true)` 由 try 内移到 `finally`(`:239`),inspect 无论成败都终结
`Checking…`。**live 验**:非法 id 出 not-found 卡、composer 0、Checking 0 次。

**behavior**:导航到不存在的 session id 得到完整可发消息的会话视图(无
"not found"、无返回),且右侧 Supervision 三个 `Checking…` spinner 稳态后仍
永久转圈(合法会话已解析为 No active goal/No subagents/Nothing needs you)。
**源码**:`SessionView.tsx:198` `setInspectReady(true)` 只在 inspect 成功
try 内;非法 id 时 inspect 一直抛错落 `:199` catch → `inspectReady` 永 false
→ SupervisionPanel `loading` 永 true(`SessionView.tsx:788`)。
**建议**:inspect/events 首次返回 404 时渲染 "Task not found" 错误卡(带回列表
CTA);pollTasks catch 里也 `setInspectReady(true)` 终止 spinner。
**touches**:`SessionView.tsx` / `SupervisionPanel.tsx` / 新错误态组件 / `styles.css`。
**刻意核查**:`git log -S"setInspectReady(true)"`→88c349a(INC-23 常规接线)→非刻意。

### L3 ✅ daemon 徽标首屏 health 未加载时误报红色 "Daemon offline" [P2]
**修**(轮8,`2a25877`):`Sidebar.tsx:369` 徽标改三态——`health===null` → `connecting`
(灰点、`Connecting…`、onClick 不触发 restart);`health && !daemonUp` → 红色 offline +
可点重启;`daemonUp` → 绿色 Connected。title/aria 三分支同步。**live 验**:冷加载 2s 内
逐帧采样,红色 "Daemon offline" 0 次出现,稳态 Connected(本机 health 快到,`Connecting…`
一帧即过;三态分支由 vitest 覆盖)。

**behavior**:每次冷加载 `/`,首帧 0-150ms 侧栏左下角红点 + `Daemon offline
— restart`,health(200,daemonUp:true)返回后才转绿。加载未知态被当确定故障态。
**源码**:`Sidebar.tsx:347-361`,`health` 初值 null(`store.ts:176`),
`health?.daemonUp` 对 null falsy → 直接渲染 offline。反例 `DaemonAlert.tsx:15`
正确地把 null 当"未知不报警"。两处对同一 health 的 null 处理不一致。
**建议**:徽标三态——`health===null` 显示中性 "Connecting…"(灰),仅
`health && !daemonUp` 才红色 offline。
**touches**:`Sidebar.tsx` / `styles.css`。
**刻意核查**:`git log -S"Daemon offline — restart"`→e4ed403(nav polish 只动
文案布局)→非刻意。

### L4 ✅ 空 Changes 面板是一行秃文字,与 timeline 空态规格不一致 [P3]
**收口(轮9,commit `31b68f7`)**:`DiffView.tsx` 病灶行换成已有的 `.diff-empty` 卡片
(`FileDashed` 图标 + `No changes yet` + 引导副文案;`last-turn` scope 另一份文案),
过滤无命中的秃空态一并改成 `.diff-empty`(`FileMagnifyingGlass` + 引导),并给面板内
另外三处只有文字没图标的 `.diff-empty` 补齐图标(否则同一 class 一半有图标一半没有,
等于把不一致挪了个位置)。`styles.css` **一行未动**(`.diff-empty` 已是居中 flex column,
与 `.tl-empty` 同一视觉语言)——与并发 session 的 styles.css 改动零冲突面。
新增 `DiffView.empty.test.tsx` 3 例。live 验:空态 svg 图标 1 个 + 标题 + 副文案,
有改动会话 `.diff-empty`=0 且文件列表正常,两页 console error+warning=0。
**behavior**:富会话点 Changes 且工作区无改动 → 仅一行灰字 `No changes in
the workspace.`,无图标/无引导;对比 timeline 空态有图标+标题+副文案。同一
app 内两类空态视觉规格不统一。
**源码**:`DiffView.tsx:413-416`(`<div className="dim" style={{padding:12}}>`);
对比 `Timeline.tsx:862-866` 空态有 ChatCircle 图标 + 标题 + 副说明。
**建议**:复用 timeline 空态样式,加图标 + 居中标题 + 一句副文案。
**touches**:`DiffView.tsx` / `styles.css`。
**刻意核查**:`git log -S"No changes in the workspace"`→9213fca(英文化批次)→非刻意。

### L5 ✅ not-found 判据靠 stderr 字符串匹配,后端应返真 404 [P2]
**behavior**(轮8 implementer 交回的风险点,非 UI 可见缺陷,属正确性/健壮性):
L2 的错误态判据是匹配 CLI 文案 `agentrunner: no session matches "<id>"`——因为
`webui/api.go:104 arFail` 把**所有** `ar` 失败统一包成 HTTP 502,前端拿不到 404。
一旦改了这句 CLI 文案,L2 会**静默退化**回旧行为(不误报,但错误卡不再出现)。
**建议**:`webui/api.go` 对 not-found 语义返真 404(或在错误体里带机器可读 code),
`api.ts` 把 HTTP status 附到 Error 上,`SessionView.tsx:41 isSessionNotFound()` 改判
status。**touches**:`webui/api.go` / `webui/frontend/src/api.ts` /
`SessionView.tsx`(+ 后端 handler 测试)。注:touches 含 Go 后端,与纯前端条目不重叠。

**收口(轮9,commit `100908a`)**:`webui/api.go` 新增 `arNotFound()`,`arFail()` 命中
not-found 语义 → **HTTP 404 + `{"code":"session_not_found"}`**,其余失败原样 502
(stale-binary 自诊断与 `ar new` stdout 合并逻辑未动)。`api.ts` 抛 `ApiError`
(带 `status`/`code`,message 表达式逐字不变故 toast 文案零变化);`isSessionNotFound()`
主判据改 `status===404 || code==="session_not_found"`,stderr 文案匹配降级为向后兼容
fallback(旧二进制)。字符串匹配现只剩后端一处(与 `ar` 同仓库同批构建,耦合可接受)。
新增 Go 测试 `TestArFailNotFoundIsMachineReadable` + 前端 4 例(其中一例把 CLI 文案
**完全改写**仍能判出 not-found —— 正是本条要防的静默退化)。live 验:`curl` ghost id
→ 404 + code,存在的 id → 200;前端 not-found 卡出现、composer 0 个;**10s 稳态窗内
针对 ghost 的请求 = 0**,阳性对照(活会话同窗 22 个轮询请求)证明探针有效。
遗留(小):`webui/ar.go:90 sessionExists()` 还有第二处同样的字符串匹配(后端内部
存在性探测,不违反本条目标),可后续复用 `arNotFound()` 收成一处。

> 注:finder 另确认空态实现良好、不构成 finding 者:侧栏空态
> (`Sidebar.tsx:269-274` 有 Tray 图标 + 区分无任务/搜索无果)、Scheduled 空态
> (`Scheduled.tsx:172-186` 双版)、Projects 骨架(`Sidebar.tsx:264-268` shimmer)。
> ✂ 刻意:Home 底部钉输入框留白布局 = QA-45 定,非空态缺陷。

## A 相 · 键盘可达与焦点可见性(轮9 finder,新镜头)

> 镜头:a11y——Tab 序 / 焦点环 / 浮层 Esc 与焦点归还 / onClick 语义 / 快捷键。
> 对 live 8809 playwright 取证(6 个探针,全 `wait_for_timeout`)+ JSX AST 扫描(217 处
> onClick)。finder 已排除的**假阳性**(别动):`Modals.tsx` 焦点陷阱/Esc/归还三件套齐全;
> `Menu.tsx`/`Popover.tsx` 方向键+Home/End+Esc+归还都做了;Settings 与 CommandPalette
> 的 Esc 与归还实测正常;Timeline 缩略图有 onKeyDown;Approval 卡是真 `<button>`;
> 全局焦点环 `tw.css:134` 对 button 生效良好(实测 solid 2px #2F6BFF)。

### A11Y-1 ✅ 会话页按 Tab 永远到不了对话区和 composer(侧栏堵死 Tab 路) [P1]
**behavior**:任一会话页从头按 Tab,焦点依次落进侧栏每个 project 标题和每条任务,
**按满 220 次仍未到达 composer**。纯键盘用户无法在合理次数内触达"读对话/发消息"这个
核心动作,只能先用鼠标点一下 composer。并会把 A11Y-3 的焦点丢失放大成灾难。
**证据**:实测 `focusable: total=794, sidebar=748`(**94% 可聚焦元素在侧栏**);
`composer reached at Tab # NEVER within 220`;地标探测 `{main:1, nav:1, skip:0, h1:0}`
——有 `<main>`/`<nav>` 但**无任何 skip link**。截图 `r9-a11y-session*.png`。
**源码**:`Sidebar.tsx`(每条 session 渲染成普通 `<button>`,全进 Tab 序)。
**建议**:(a) **skip link**(改动最小、收益最大):`App.tsx` 首部加视觉隐藏、`:focus` 显形的
`<a href="#main" class="skip-link">Skip to conversation</a>`,`<main id="main" tabIndex={-1}>`;
(b) roving tabindex(侧栏只留 1 个 Tab 停靠点,组内 ↑/↓ 移动,复用已有的 `⌘⌥↑/↓`)——更贴近
Codex 但改动面大。**先做 (a),(b) 另立条目**。
**touches**:`App.tsx` / `styles.css`(新增 `.skip-link`)。不碰 Sidebar.tsx。
**刻意核查**:`git log -S"skip-link"`/`-S"roving"` 对 webui 全无命中 → **非刻意,是洞**。

### A11Y-2 ✅ 三个搜索输入框聚焦后"零焦点指示"(outline/border/box-shadow 全空) [P2]
**behavior**:Tab 进侧栏搜索框 / Scheduled 搜索框 / Home 的 project 搜索框时,**屏幕上没有
任何视觉变化**,键盘用户不知道焦点在哪。对比同 app 普通按钮有清晰 2px 蓝环。
**证据**(聚焦态 computed style):`.side-search input` → `outline: none`、`borderWidth: 0px`、
`boxShadow: none`,外层 wrapper 边框与未聚焦时**完全相同**;`.sched-search input` 同上;
对照普通按钮 `solid 2px rgb(47,107,255)` ✅。截图 `r9-a11y-focus-*.png`。
**根因**:`tw.css:128-133` 的 `input:focus{outline:none;border-color:var(--blue)}` 特异性
(0,1,1) 压过同层 `:where(...):focus-visible{outline:2px solid}` 的 (0,1,0) → 全站 input 的
焦点提示**只靠 border-color**;而这三个是"wrapper 画边框 + 内部 input 无边框"结构
(`styles.css:4277`/`:3085-3092`/`:1713-1717` 显式 `border:0`),wrapper 又都**没有
`:focus-within`** → border-color 落在 0px 宽的边框上,什么都看不见。
**参照(仓库里做对的同类)**:`styles.rs.css:136-141` `.diff-filter input:focus{outline:none}`
+ `.diff-filter:focus-within{border-color:var(--rs-accent)}` —— 同一 wrapper 模式补了
`:focus-within`。
**建议**:照抄,给三个 wrapper 各加 `:focus-within{border-color:var(--blue)}`
(`.cx-project-search` 无 border,需补 1px 边框或改 box-shadow ring);顺带删掉
`styles.css:4277` 多余的 `border:0;outline:none`。
**touches**:`styles.css`(3 处,纯 CSS,零 TSX)。
**刻意核查**:`git log -S".sched-search input:focus"` → `0e97486`(Codex UI polish)是**视觉**
打磨(去双框感),无焦点可见性意图;而 `.diff-filter:focus-within`(`7512c6b`)证明团队后来
已认可正确写法,只是没回头补这三处。**非刻意,是遗漏**。

### A11Y-3 ✅ ⌘F 查找栏 Esc 关闭后焦点被丢到 `<body>` [P2]
**behavior**:会话页 ⌘F 打开对话内查找 → Esc 关闭 → 焦点不回到打开前的位置,而是掉进
`<body>`。再按 Tab 就从文档最顶端重来——叠加 A11Y-1,等于**要重新趟 748 个侧栏按钮**才能
回到正文。用一次 ⌘F 就"迷路"。
**证据**(锚定焦点在 "New task" 后开浮层):FindBar → Esc 后 `focus = BODY` ❌;
Settings(⌘,) → `BUTTON 'New task'` ✅;CommandPalette(⌘K) → ✅。焦点归还审计:
Modals ✅ / CommandPalette ✅ / Lightbox ✅ / **FindBar ❌(0 命中)**。
**源码**:`FindBar.tsx`(全文件无 activeElement 保存/恢复)。**参照**:`Modals.tsx:41`
`const previous = document.activeElement` + `:75` cleanup 里 `previous?.focus()`。
**建议**:把 Modals 那对四行搬进 FindBar 的 mount effect。
**touches**:`FindBar.tsx`(~5 行)。
**刻意核查**:`git log -S"fb-input"` → `1cc0675`(⌘F Codex parity)首次引入即无归还,而
`previous?.focus()` 是更早 `88c349a` 就有的既成模式 → **新组件漏用既有模式,非刻意**。

### A11Y-4 ☐ Composer 附件 chip 是 `<span onClick>`,键盘无法移除已添加附件 [P2]
**behavior**:composer 附加文件/图片后的 chip,点击即移除——但它是 `<span>`,**无 tabIndex、
无 role、无键盘事件**,Tab 会直接跳过。一旦误加附件,**没有任何键盘方式能删掉**(只能放弃
整条消息或改用鼠标);X 图标还标了 `aria-hidden`,屏幕阅读器也读不到"可移除"。
**证据**:JSX 扫描 217 处 onClick → 53 处挂在非 button 上 → 筛出**唯一真实病灶**
`Composer.tsx:1122 <span className="cx-att cx-att-codex" onClick={...}>`(其余是容器/背景
点击,非唯一入口,不算洞)。
**源码**:`Composer.tsx:1122`(+ `:1128` `<span className="cx-att-x" aria-hidden>`)。
**参照**:同文件其他 composer 控件(`cx-icon`/`cx-pill`/`cx-send`)都是真 `<button>`,实测在
Tab 序内且有焦点环。
**建议**:移除动作收进真 `<button className="cx-att-x" aria-label={`Remove ${a.name}`}>`
(去掉 aria-hidden),外层 span 退回纯展示容器、卸掉 onClick;CSS 补 `.cx-att-x` 的
`border:0;background:transparent;padding:0`(因 `tw.css:98` base button 会加边框内边距)。
**touches**:`Composer.tsx`(~6 行)/ `styles.css`(`.cx-att-x` 重置)。**与 A11Y-5 同文件,须同人做**。
**刻意核查**:`git log -S"cx-att"` → `24aeccb`/`a881f67`/`2e2eee8` 三个 commit 都在做**视觉**,
message 与 diff 均未提键盘/可达性,也无"故意不让键盘删附件"的理由 → **非刻意,是洞**。

### A11Y-5 ✅ Composer 的 8 个 pill 下拉缺 `aria-haspopup`/`aria-expanded` [P3]
**behavior**:composer 的 Access/Model/Mode 等 pill 点开是 `role="menu"` 面板(方向键、Esc、
焦点归还都好),但触发按钮**没告诉辅助技术"我是菜单、我现在开着"**。屏幕阅读器用户听到的
只是普通按钮。纯视觉/纯键盘用户不受影响,故 P3。
**证据**:`trigger focused: BUTTON.cx-pill 'Access: set by agent'` → Enter → `panel open: True`,
但按钮上无 `aria-expanded`。全仓 `aria-expanded|aria-haspopup` 仅 4 处命中(Menu ✅ /
DiffView ✅ / CommandPalette ✅ / Timeline ✅),**Popover 及其 8 个 Composer 调用点一个都没有**。
**源码**:`Popover.tsx`(trigger 由调用方渲染,Popover 不注入 aria)+ 调用点
`Composer.tsx:936/1003/1030/1049/1194/1240/1288/1322`。**参照**:`Menu.tsx:39` 把
`aria-haspopup="menu" aria-expanded={open}` 写在组件内部。
**建议**:给 8 个调用点的 button 各补 2 个属性(`open` 已作为参数传进 `trigger` 回调,零接线)。
**touches**:`Composer.tsx`。**与 A11Y-4 同文件,须同人做**。
**刻意核查**:`git log -S"aria-haspopup"` 对 Composer/Popover 无命中 → **新组件没沿用既有模式**。

### A11Y-6 ☐ 侧栏改 roving tabindex(A11Y-1 的彻底解法,skip link 之后再做) [P3]
从 A11Y-1 建议 (b) 拆出:侧栏列表整体只留 1 个 Tab 停靠点(选中项 `tabIndex={0}`,其余 -1),
组内用 ↑/↓ 移动(复用已有 `⌘⌥↑/↓` 逻辑)。更贴近 Codex,但改动面大 → 待 A11Y-1(a) 落地后再评估。
**touches**:`Sidebar.tsx`。
**轮10 补充**:A11Y-1(a) 已落地(skip link),侧栏 748 个按钮现在能一步跳过 → 本条的紧迫性
从 P1 降到真 P3(仍值得做:它让侧栏本身**能用键盘高效浏览**,而不只是"能被跳过")。

### A11Y-7 ☐ skip link 落进 #main 后,对话区 27 个 msg-copy 仍堵在 composer 前(40 次 Tab) [P2]
**behavior**:A11Y-1 的 skip link 把 748 个侧栏按钮一步跳过了(NEVER within 220 → 可达),但落点是
`#main` **顶部**,焦点接着要穿过整个对话区才能到 composer。富会话实测:skip 之后**还要按 40 次 Tab**
才落到输入框。而堵路的元素数量**随消息条数线性增长**——100 条消息的长会话会变成 100+ 次。
键盘用户"跳过侧栏 → 直接开始打字"这条最高频路径仍未打通。
**证据**(live `20260711-011831-what-is-the-project-297d`,轮10 实测):
```
焦点普查: {total:795, inMain:46, convBeforeComposer:40, composer(在#main内):0}
堵路构成: msg-copy ×27(每条消息一个复制按钮)、worked-row ×8、cx-icon ×2、cx-pill ×2
skip link 之后到达 composer 需 Tab: 40 次
```
**源码**:skip link 目标 `App.tsx` `<div className="main" id="main" tabIndex={-1}>`(轮10 `617fd68`);
堵路按钮是对话区每条消息的 `.msg-copy`。
**建议**:①**最直接**——再加一个 skip link「Skip to message input」直接锚到 composer 的 textarea
(两个 skip link 是 Codex/GitHub 的常见做法,Tab#1/Tab#2 各一个);或②把单个 skip link 的目标从
`#main` 改成 composer(但"跳到对话区读内容"也是合法诉求,故 ① 更优);③(治本,但面大)对话区
消息的 `.msg-copy` 改成**仅在消息 hover/focus-within 时进入 Tab 序**——**慎重:这可能反而伤害
纯键盘用户**(他们没有 hover),需先想清楚,别为了 Tab 次数把功能藏掉。
**touches**:`App.tsx` / `styles.css`(第二个 `.skip-link`)。
**刻意核查**:本条是轮10 修 A11Y-1 后**新暴露**的下一层,非既有刻意决策。

## P 相 · 性能(轴 B,2026-07-11 轮10 起)

**轮10 巡检 baseline(live 8809,index-BmBuR-u1.js)**:
| 指标 | 实测 |
|---|---|
| JS bundle | raw=895,097 B / gzip=251,312 B(单 chunk,零 code splitting) |
| CSS bundle | raw=133,732 B / gzip=24,858 B |
| 静态资源传输 | **无 gzip**,冷加载实传 ≈1.03 MB(本可 ≈276 KB) |
| 静态资源缓存 | **无 Cache-Control / 无 ETag / 无 Last-Modified** → 每次访问 200 全量重传 |
| `GET /`(index.html) | 0.8 ms |
| `GET /api/health` | 29 ms |
| ~~`GET /api/sessions` 242 ms——侧栏首屏要等它~~ | ⚠️ **本行是误判,已由轮10 性能 finder 订正** |

> **⚠️ baseline 订正(轮10 finder 实测)**:242ms 那个是**未分页**的 `/api/sessions`(152 KB),
> **前端从不调用它**——`store.ts:163/335` 首屏拉的是 `?limit=40`(11.4 KB,**中位 ~65ms**)。
> 侧栏首屏并不等 242ms。真正的洞是**别的**(见 PERF-4:7 个串行分页请求排成 604ms 瀑布)。
> 教训:**量 API 要量前端真实调用的那个 URL,别对着裸端点拍脑袋。**

> **⚠️ PERF-1 收益的诚实校准(轮10 finder 实测)**:loopback 上 895KB JS 的 resource timing
> `duration = 4ms` —— **本地场景下 gzip 几乎省不了传输时间**。PERF-1 的真实价值是
> ①**Cache-Control/ETag**(二次访问 200 全量 → 304 零字节,省掉重复传输**和重复 parse**)、
> ②**远程/弱网访问**(1.03MB → 271KB 是实打实的)。而首屏那 **~94ms 的 JS eval**
> (FCP 168ms − DCL 74ms)**gzip 治不了,要靠 PERF-3 的 code splitting**。别指望 gzip 改善本地 FCP。

### PERF-1 ✅ 静态资源零压缩、零缓存头:每次冷加载白下载 1.03 MB [P1]
**behavior**:webui 每次冷加载实打实传输 895 KB JS + 134 KB CSS,而这两份 gzip 后只有
251 KB + 25 KB(压缩率 72%/81%)。且因为**没有任何缓存头**,刷新页面 / 二次访问**照样
全量重传** —— 明明 Vite 产物文件名自带 content hash(`index-BmBuR-u1.js`),天然可以
`immutable` 永久缓存,却一个字节都没省。远程/弱网访问时首屏白等数秒。
**证据**(live 实测):
```
$ curl -s -o /dev/null -D - -H 'Accept-Encoding: gzip, deflate, br' \
    127.0.0.1:8809/assets/index-BmBuR-u1.js -w 'size_download=%{size_download}\n'
HTTP/1.1 200 OK
Content-Length: 895097        <-- 无 Content-Encoding、无 Cache-Control、无 ETag
size_download=895097
```
**源码**:`webui/embed.go` `staticHandler()` —— 直接用裸 `http.FileServer(http.FS(sub))`,
不加压缩也不加缓存头(embed.FS 的 modtime 是零值,FileServer 因此连 Last-Modified/304
都不发)。
**建议**:①**启动时预压缩**(不是每请求现压)`.js/.css/.html/.svg/.json` 并缓存在内存,
按 `Accept-Encoding` 协商发 gzip + `Vary`;②`/assets/*`(带 hash)→
`Cache-Control: public, max-age=31536000, immutable`,`index.html` → `no-cache`
(**边界:index.html 必须回源校验,否则用户永远拿不到新版**);③内容 hash 做 ETag,
`If-None-Match` 命中回 304(gzip 与非 gzip 变体 ETag 需区分)。
**touches**:`webui/embed.go` / `webui/embed_test.go`(新)。
**刻意核查**:`git log -S"FileServer"` / `-S"Cache-Control"` 对 webui 无"故意不压缩/不缓存"
的依据,注释里也只说 SPA fallback 语义 → **非刻意,是洞**。

### PERF-2 ☐ 空闲的已完成会话页仍在 183 请求/分钟;`/events` 每秒 dump 整份 journal 只为返回 `[]` [P1]
**behavior**:打开一个**已经跑完**的会话,什么都不做,浏览器每秒发 ~3 个请求;服务端每秒 fork 一次
`ar`、读+序列化**整份 journal**(193 KB),然后返回 `[]`。会话永不会再变,机器却一直空转、风扇一直转。
**证据**(富会话,严格 40s 空闲窗口,零交互 → **122 请求 = 183/min**):
| 端点 | 频率 | 响应体 | 说明 |
|---|---|---|---|
| `/events` | 61.5/min(1.0s) | **3 B**(`[]`) | 但服务端每次都做**全量**工作 |
| `/ps` / `/inspect` / `/queue` | 各 24/min(2.5s) | 3 B / 9.7 KB / 2 B | |
| `/api/sessions` / `/api/runs` | 各 15/min(4s) | 11 KB / 3 B | |
| `/api/health` / `/api/projects` | 12 / 7.5 per min | | |

**关键实测**:`/events`(全量 193,500 B)耗时 26.4/27.8/26.4 ms,`/events?after=999999`(返回 `[]`)
耗时 44/19/27/23/23 ms —— **两者几乎一样,证明服务端无论如何都做了全量工作**。主线程空闲开销
19.1 ms script/s(对照 about:blank = 0)。
**源码**:`webui/api.go:645` `s.runAR(ctx, 30s, "events", "--json", id)` —— **`after` 游标从不下传**,
`api.go:656-663` 才在 Go 里逐行 `json.Unmarshal` 出 `seq` 然后丢弃;`SessionView.tsx:313`
`setInterval(poll, 1000)`(而 `:320` 的 SSE `/stream` 已经在负责活跃会话了)。
**约束**:`ar events` CLI 只有 `-json`/`-state`,**没有 `-after`**,所以 webui 想增量也要不到。
**建议**(三选一或组合):(a) 给 `ar events` 加 `-after <seq>` 并透传;(b) **按 status 门控**——
completed/failed 会话把 1s 轮询降到 30s 或停掉(**改动最小、收益最大,建议先做**);
(c) webui 按 (sid, journal mtime+size) 缓存 folded journal。
**预估收益**:空闲会话页 `/events` **60/min → ~2/min**,总请求 **183/min → ~35/min**,
消掉 ~58 次 `ar` fork/min/tab 与 **~11 MB/min 的 journal 读+解析**。
**touches**:`SessionView.tsx`(方案 b)/ `webui/api.go` + `ar events` CLI(方案 a)。
**刻意核查**:`git log -S'"events", "--json", id'` → 只有 `8f817e3`(webui 初版)→ **非刻意,是遗留**。

### PERF-3 ☐ 零 code splitting:895 KB 单 chunk,其中 92.6 KB gzip 首屏根本用不到 [P1]
**behavior**:每次加载都要 parse+compile 895 KB JS 才出画面。首屏 FCP 168ms 里有 **~94ms 是
JS eval**(FCP 168 − DCL 74)。而 loopback 传输只花 4ms —— 所以**这几乎全是 parse 成本,
gzip(PERF-1)治不了它**。
**证据**(finder 真跑 vite build,不是估算):
| 组 | raw | **gzip** | 首屏必需? |
|---|---|---|---|
| `@phosphor-icons/react` | 260.0 KB | 53.8 KB | 部分 |
| app 源码 | 240.0 KB | 74.0 KB | 部分 |
| react-markdown + remark + micromark + unified | 155.7 KB | **47.0 KB** | ❌ 只在消息体渲染时用 |
| react + react-dom | 142.0 KB | 45.5 KB | ✅ |
| highlight.js + lowlight | 92.4 KB | **28.7 KB** | ❌ 只在代码块用 |

**lazy-load 实测(真 build)**:仅把 `Markdown` 改 `lazy()` → entry **251.79 → 174.25 KB gz(−31%)**;
再加 `Settings`/`Scheduled`/`CommandPalette`/`Shortcuts` → entry **159.23 KB gz(−36.8%,−92.6 KB)**。
**源码**:`vite.config.ts:9`(无 `rollupOptions.manualChunks`)、`App.tsx:5-13`(4 个路由级组件全静态
import)、`Markdown.tsx:3-6`(静态 import `react-markdown` + `./highlight`)。
**touches**:`vite.config.ts` / `App.tsx` / `Markdown.tsx`(需 `<Suspense>` 兜底态)。
**刻意核查**:`git log -S"manualChunks"` **零命中**,docs 里 `lazy|code split|chunk` 零命中 → **纯遗漏**。

### PERF-4 ☐ 初次 hydration 把 455 个 session 排成 7 个串行请求(604ms 瀑布) [P2]
**behavior**:home 首屏 `store.ts:341-347` 的 `for(;;)` 逐页 `await AR.sessions(80, offset)` 一直拉到
拉空。455 个 session = 1 + 6 = **7 个串行请求**,每个都 fork 一次 `ar`。
**证据**(playwright resource timing,home):session-list 请求依次落在 **t = 227 / 286 / 360 / 435 /
504 / 559 ms**,各 45–75ms,**到 ~604ms 才收尾**。
**源码**:`webui/frontend/src/store.ts:341-347`。
**建议**:老页面**并发**拉(`Promise.all`),或干脆**别在 hydration 里拉全量**(侧栏每个 project 只显示
6 条,还有搜索兜底)。**预估**:6 个请求并行 → **~370ms → ~75ms**。
**touches**:`store.ts`。

### PERF-5 ☐ 每个 webui 读接口都 fork 一个 40MB 的 `ar` 二进制(~20ms 地板价) [P2]
**behavior**:所有读接口都付一笔 ~20ms 的进程 fork + daemon 往返税。单看不明显,被 1Hz/2.5s 轮询
乘出来就是持续负载(实测**每秒 ~2 次 `ar` spawn,每个 tab**)。
**证据**(同机同进程对照):**fork 的** `/api/health` 29–44ms · `/events` 20–27ms · `/ps` 18–27ms ·
`/queue` 21–26ms · `/inspect` 22–39ms;**不 fork 的** `/api/runs` **0.6ms** · `/api/projects` **1.0ms**。
**差 20–40 倍。**
**源码**:`webui/ar.go:21-31` `runAR` → `exec.CommandContext(ctx, s.arPath, args...)`,被
`api.go:302/645/674/688/733/1076/…` 调用。
**建议**:webui **直连 daemon socket** 而非 shell out;或对读接口加 500ms–1s TTL 响应缓存
(零协议改动,能把绝大多数轮询请求折叠掉)。**与 PERF-2 有协同**,建议先做 PERF-2。
**touches**:`webui/ar.go` / `webui/api.go`。

### PERF-6 ☐ Phosphor 每个 icon 打包 6 种 weight,app 只用 4 种 [P2]
**behavior**:最大的单个依赖(占 gzip bundle 的 21%),其中一大半是永不渲染的字形。
**证据**:`@phosphor-icons/react@2.1.10` = 260.0 KB raw / **53.8 KB gzip**;app 用了 64 个 icon。
每个 icon 的 defs 模块(如 `defs/Check.es.js` = 1,825 B)含 **bold/duotone/fill/light/regular/thin
六个 weight** 的 SVG Map,而 `grep -rn "weight=" src` 显示 app 只用 **regular/fill/bold/light** ——
**thin 与 duotone 从未使用**,且 duotone 最胖(双 path + opacity)。
**建议**:vite plugin/alias 重写 `dist/defs/*` 只保留 4 个 weight,或让稀有 icon 随 PERF-3 的 lazy
chunk 走。**预估省 15–20 KB gzip 首屏**。
**刻意核查**:`git log -S"@phosphor-icons"` → `8d1edab`(Codex UX 重建)。**选这个库是刻意的**,
但"打进 6 个 weight"只是默认行为、不是决策 → 可改。

### PERF-7 ☐ 侧栏占 87% 的 DOM(3,595 / 4,154 节点),无虚拟化也无 memo [P3]
**证据**(富会话页):`document.querySelectorAll('*').length = 4,154`,`aside.sidebar` 内 **3,595** 个,
timeline 只有 445 个。侧栏已有每 project cap=6(`viewModels.ts:46`)+ "Show N more",但 455 个 session
摊到多个 project 上总量仍巨大。
**但必须说清(finder 主动澄清,防止误派工)**:20s 空闲窗口(含 5 次 `/api/sessions` 刷新)
MutationObserver 记录 **0 次 DOM mutation** —— React 协调后是 no-op,**没有 DOM churn、没有 layout
thrash**。所以这是**初次渲染 / 内存重量**问题,**不是 jank 问题** → 判 P3。
**建议**:侧栏行组件加 `React.memo`(避免 4s 刷新时白跑 455 次 render 函数);折叠区不挂载 DOM。
**touches**:`Sidebar.tsx`。

### ✂ 轮10 性能 finder 主动排除的假阳性(**别派 implementer 去改**)
- **✂ Timeline 列表虚拟化 —— 不需要**。滚完富会话整条 timeline(4,252px)实测:中位帧 **17.1ms**、
  p95 35.3ms、**超 50ms 的帧 = 0**;3s 滚动 ScriptDuration 仅 33ms。且 Timeline 本就在截断
  (`Timeline.tsx:190/441` `.slice(0,20000)`、`:330` `.slice(0,40)`、`:357` `.slice(0,12)`)。
  要出真 jank 得再长一个数量级。
- **✂ "markdown 每秒 re-parse / 每帧重算" —— 不成立**。`SessionView.tsx:381-385` 的
  `setInterval(()=>setNow(Date.now()),1000)` **有守卫** `if (!goalState || goalTerminal) return;`
  —— 已完成会话根本不启动这个 tick;`folded` 也已 memo(`:363`)。**这是刻意且正确的设计。**
- **✂ 首屏 long task —— 没抓到可靠信号**。`PerformanceObserver({type:'longtask',buffered:true})`
  覆盖首 3.5s:home 5 次里 4 次为 **0 个**,rich 5 次里 4 次为 **0 个**。不足以作为改进依据。

## A 相 · 轮10 axe 扫描新发现(A11Y-8..10)

轮10 巡检用 axe-core(WCAG 2.0/2.1 A+AA + best-practice)扫 home/rich/changes/scheduled 四页,
结果:**critical=0**、serious=1 类(color-contrast)、moderate=4 类。逐条核查如下。

### A11Y-8 ✅ 侧栏次要文字对比度 4.42:1,差 0.08 不达 WCAG AA — 一个色值波及 79 处 [P2]
**behavior**:侧栏的分组标题(Projects)、项目名、会话标题、"show more" 等**全部**次要文字,
在浅色主题下对比度只有 **4.42:1**,低于 WCAG AA 正文要求的 4.5:1。弱视/强光下读起来吃力。
**证据**(axe-core 实测,四页共 **79 处** violation,全是**同一个色对**):
```
fg=#737373  bg=#f7f7f8  →  contrastRatio = 4.42 : 1   (需 4.5:1)
命中: .section-label / .project-group > .project / .show-more / button[aria-label="…"] …
```
**关键**:79 处不是 79 个 bug——是**一个颜色 token** 用在 79 个地方。把 `#737373` 压深到
`#717171`(≈4.6:1)或 `#6e6e6e`(≈4.8:1)即可**一次修完**,视觉上几乎无感。
**源码**:`styles.nav.css:15` `.sidebar .section-label` / `styles.css:3109` `.section-label`
——先查这个灰值是不是某个 CSS 变量(如 `--muted`/`--text-2`),**改变量而不是逐处改**。
**dark 主题需另测**(axe 本轮只扫了 light)。
**刻意核查**:`git log -S".section-label"` → `e4ed403`(INC-41 nav Codex polish)、`8d1edab`
(rebuild UX around Codex)均是**视觉**打磨,无"刻意压低对比度"的依据;且 4.42 vs 4.5 明显是
**没量过**而非有意取舍 → **非刻意,是洞**。
**touches**:CSS 变量定义处(待定位)/ `styles.nav.css` / `styles.css`。

### A11Y-9 ✅ `<html lang="zh-CN">` 但整个 UI 是英文:屏幕阅读器用中文语音读英文 [P2]
**behavior**:`index.html` 声明 `lang="zh-CN"`,而 webui 的界面文案**全是英文**(New task /
Projects / Ask to approve …)。屏幕阅读器会据此挑**中文语音引擎**去读英文单词,发音不可懂;
搜索引擎与翻译工具也会误判。这是"一行 attribute 毁掉整个 a11y 语音层"的典型。
**证据**:`document.documentElement.lang === "zh-CN"`(live 实测);同页正文 100% 英文。
**源码**:`webui/frontend/index.html:2`。
**建议**:改 `lang="en"`。(若将来真做 i18n,应按用户语言动态设置——但**现在**没有任何中文 UI 文案,
`zh-CN` 就是纯错的。)
**刻意核查**:`git log -S'lang="zh-CN"'` → 只有 `8f817e3`(webui 初版,一次性建整个 React/Vite 骨架),
commit message 与 diff 都没提语言/i18n → **是初始模板遗留,非刻意**。
**touches**:`webui/frontend/index.html`(1 行)。

### A11Y-10 ☐ 缺 h1、home/scheduled 无 main landmark、内容游离在 landmark 之外 [P3]
**behavior**:屏幕阅读器用户靠标题(h1)和地标(landmark)快速定位;本 app **每一页都没有 h1**,
home/scheduled **没有 `<main>` 地标**,home 的 composer(`.cx-input-wrap`)整个**不在任何 landmark 内**,
changes 页两个 `<aside class="sidebar">` **重名地标**无法区分。
**证据**(axe,4 页):`page-has-heading-one` ×4、`landmark-one-main` ×2(home/scheduled)、
`region` ×4(游离内容,含 home composer)、`landmark-unique` ×1(changes 的两个 sidebar)。
**注**:轮10 的 A11Y-1 给 `App.tsx` 加的是 `<div className="main" id="main">`(**div,不是
`<main>` 元素**)——skip link 的锚点成立,但**没有**顺带补上 main 地标。真正的 `<main>` 只在
`SessionView.tsx:601/733`,所以 home/scheduled 才报缺。
**建议**:①`App.tsx` 的 `div.main` 换成 `<main id="main">`(**但要先确认不会和 SessionView 内部的
`<main>` 嵌套成两个** → 二选一,建议 SessionView 那个降级为 `<div>`);②每页补一个视觉隐藏的 h1
(复用 A11Y-1 的 `.sr-only`/skip-link 隐藏手法);③给两个 `<aside class="sidebar">` 各加
`aria-label` 区分。
**touches**:`App.tsx` / `SessionView.tsx` / `Home.tsx` / `Scheduled.tsx` / `styles.css`。
**刻意核查**:全仓 `git log -S"<main"` 无"刻意不加地标"的依据 → 非刻意,是从未做过。

### A11Y-11 ☐ assistant 消息的操作按钮 opacity:.62 把 #6e6e6e 稀释成 2.46:1(远低于 AA) [P2]
**behavior**:A11Y-8 把 `--dim` 压深后,axe 的 color-contrast 从 79 处降到 **6 处**——剩下这 6 处
根因**不同**:`.msg-copy`(Copy message / Continue in new task)的 computed color 已经是 `#6e6e6e`
(合格),但父元素 `.assistant .msg-actions` 的 **`opacity: .62`** 把**有效**前景色稀释成
`#a5a5a5` on `#ffffff` = **2.46:1**(AA 要 4.5:1,**差得最远的一条**)。11px 的小字 + 2.46:1,
几乎看不见。
**证据**(A11Y-8 上线后 axe 复测,rich/changes 页):`fg=#a5a5a5 bg=#ffffff ratio=2.46 需=4.5` ×6;
`getComputedStyle(.msg-copy).color === rgb(110,110,110)`(即 `#6e6e6e`,色值本身没问题)。
**源码**:`styles.css:2673` `.assistant .msg-actions { opacity: .62; }`(稀释源),
`styles.css:2674-2681` `.msg-copy { color: var(--dim); font-size: 11px }`。
**⚠️ 这条要慎重**:`opacity:.62` + `:2670` 的 `.msg:hover .msg-actions` 是**刻意的视觉低调设计**
(操作按钮平时退到背景里、hover 才浮现)。**不要直接删 opacity** —— 那会毁掉这个设计意图。
**建议**(需权衡,交给 implementer 决策并给 before/after 截图):
① 保留"低调"意图但换实现——把 `opacity:.62` 去掉、改用一个**本身就更淡但达标**的颜色
(如 `#767676` ≈ 4.54:1 on white),hover 再切到 `--ink`;
② 或把 opacity 提到 ~.85 并同时把 `--dim` 用于此处的色值再压深一档;
③ 注意 WCAG 对"纯装饰/禁用态"有豁免,但 **Copy message 是真实可用的功能按钮,不豁免**。
**touches**:`styles.css`(`.msg-actions` / `.msg-copy`)。
**刻意核查**:`git log -S"msg-actions"` 显示 opacity 是视觉打磨引入的 → **视觉意图是刻意的,
但"稀释后不达 AA"不是**(没人量过)。故判**修**,但要**换实现而非删设计**。

## M 相 · 移动端/响应式(轴 A,2026-07-11 轮10 新镜头)

轮10 派的「移动端触控 + 响应式断点」finder 是本循环**首次**测 **768 / 1024 / 1180 / 1280** 这些
中间断点(此前只测 1440 和 390)——**两条 P1 全在中间断点上**,印证了"没测过的地方就是洞在的地方"。

### MOB-1 ☐ 1024–1280 开 Changes 后,Send/mic/模型选择器跑出输入框、压在 diff 面板上 [P1]
**behavior**:iPad 横屏 / 小笔电(1024–1280px)打开 Changes 分栏,composer 底栏整排控件**溢出卡片
右缘**,发送键浮在 diff 面板上、麦克风被压成一条 **17px** 的竖条。用户看到的是"发送键长在别人家里"。
**证据**(live 实测,截图 `/tmp/mob/i-1024-changes.png`、`bar-{1024,1180,1280}.png`):
| 视口 | card 右缘 | `.cx-bar` scrollW/clientW | send 实测 x..right | Changes 面板 x | 结果 |
|---|---|---|---|---|---|
| 1024 | 556 | **433 / 234** | 721..754 | 584 | send **压面板 170px**,mic 宽 **17px** |
| 1180 | 654 | 433 / 333 | 721..754 | 683 | send 压面板 71px |
| 1280 | 732 | 433 / 410 | 721..754 | 760 | 溢出卡片 22px |
| 1440 | 815 | 493 / 493 | 773..806 | 843 | ✅ |

同时对话列被压到 **292px**(`grid-template-columns: 292px 440px`),正文一行只剩 4–5 个词。
**源码**:`styles.css:1833` `.cx-bar{display:flex}` **无 `flex-wrap`/`overflow`/`min-width:0`**;
`styles.css:1844` `.cx-icon` **缺 `flex-shrink:0`**(对比 `.cx-send` 在 `:1963` 就有);
`styles.css:4383` `@media(max-width:1180px)` 把对话列挤到 292px。
**建议**:仓库已有正确写法 `styles.css:3855-3857`(≤520 的 `.cx-bar{flex-wrap:wrap;row-gap:2px}`),
但那是 **viewport** query,而本 bug 的触发条件是**容器**变窄(视口 1024 但 composer 只有 236px)
→ 正确解法是 **container query**(`.cx` 加 `container-type:inline-size`,`@container (max-width:520px)`),
一处同时覆盖 390 和"1024+开面板";附带给 `.cx-icon` 补 `flex-shrink:0`。
**刻意核查**:`git log -S"cx-bar"` 无移动端专门提交;REFERENCE 全文只提一次 "responsive",无相反规定
→ **非刻意**。

### MOB-2 ☐ 521–880px 区间 Git 分支选择器被切出视口,完全不可见不可点 [P1]
**behavior**:iPad 竖屏(768)或横屏手机上新建任务,「分支」选择器**整个消失**——用户无法为新任务选
git branch,且**没有任何滚动/换行提示**告诉他还有东西。
**证据**:
| 视口 | `.cx-env-strip` clientW/scrollW | `.branch` x..right | 结论 |
|---|---|---|---|
| 390 | 366/366 | 170..263.8 | ✅ 换行(≤520 规则生效) |
| **768** | **470/589** | **756.6..862.8** | ❌ **超出视口 94.8px,看不见** |
| **820** | 522/589 | 756.6..862.8 | ❌ 超出 42.8px |
| 900 | 602/602 | — | ✅ |

**源码**:`styles.css:1667-1679` `.cx-env-strip{display:flex;overflow:visible}` 无 `flex-wrap`;
换行规则只在 `styles.css:3851`(`@media max-width:520px`)才开 → **521–880 这段没人管**。
**建议**:与 MOB-1 同源同招——常态 `flex-wrap:wrap;row-gap:2px`(复用 `:3851` 已验证的写法)或
container query;最低限度也要 `overflow-x:auto`(`styles.css:3879-3880` 已预留隐藏滚动条样式)。
**刻意核查**:`git log -S"cx-env-strip"` → `46345d0`/`e1080d5` 均为功能提交,无"窄屏刻意藏分支"
→ **非刻意**。

### MOB-3 ☐ 触控目标全面低于 44px(聚合条目,28 个选择器) [P2]
**behavior**:整个 App 在手机上**没有一个**主要操作达到 WCAG 2.5.5 / Apple HIG 的 44×44 下限;
发送、审批、开侧栏这些高频/高后果动作都在 25–36px,拇指易误触。
**证据**(390×844 实测,28 个选择器 <44px,按点击频率排序):
`.cx-send` **33×33** · `.cx-icon`/`.cx-mic` **32×32** · `.sidebar-show`(**手机开侧栏的唯一入口**)
**31×27** · `.topbar-tool` 34×32 · `.menu-trigger` 32×30 · `.approval-actions button` 高 **34** ·
`.cx-pill` 30–32 高 · `.gbar-btn` **25×25** · `.msg-copy.icon-only` 26×21 · `.sched-tab` 高 31.4 ·
`.worked-row` 高 30.4 · 抽屉内 Settings 齿轮/Hide 30×30 · diff 工具条 64.7×**25**。
**建议**:不改视觉尺寸,用**透明扩展命中区**——`position:relative` + `::after{inset:-6px}` 把 32 撑到 44;
或统一 `@media (pointer:coarse){ … min-width:44px;min-height:44px }`。
优先级:`.sidebar-show`(唯一导航入口)→ `.cx-send` → `.approval-actions button` → `.gbar-btn`。
**刻意核查**:逐个 `git log -S` 全是功能提交(`8d1edab`/`f2f1932`/`07286b8`…),**无一条提到 44px
触控下限或刻意压小**;REFERENCE 无触控目标规格 → **非刻意**。

### MOB-4 ☐ Goal bar「取消目标」是 25×25 垃圾桶,紧贴「暂停」,手机误触即毁 [P2]
**behavior**:390 下 goal 条右侧三钮(编辑/暂停/取消)各 **25×25**、间距 ~2px。**破坏性动作和
非破坏性动作挤在 27px 节距**里,而拇指宽约 45–57px —— 想暂停,大概率取消。
**源码**:`styles.css:4119` `.gbar-btn`。
**建议**:coarse pointer 下三钮撑到 44×44 且 danger 单独加 `margin-left:8px`;或 ≤680 折进"⋯"菜单
(仓库已有 `.menu-trigger`+`.pop-panel` 模式,`styles.css:625`/`:2062`)。
**刻意核查**:`git log -S"gbar-btn"` → `07286b8`(轮2 FA-2)刻意改的是**颜色**(危险钮静态中性化),
**没有**涉及尺寸/间距 → **尺寸非刻意**。

### MOB-5 ☐ Settings 在 390 下左导航吃掉 45vh 且从行中间硬切,无滚动提示 [P2]
**behavior**:手机上开 Settings,上半屏是一坨**被截断的导航列表**("Configuration" 从字中间切开),
下半屏才是内容;两个独立滚动区叠在 844px 屏上,用户不知道上面那块还能滚。
**源码**:`components/Settings.tsx:90` `max-[720px]:max-h-[45vh]`。
**建议**:≤720 改 **drill-down**(列表 → 详情 + 返回),或压成横向可滚 tab strip(仓库已有正确写法:
`.sched-tab`/`.sched-tabs` `styles.css:4285` + `.cx-bar` 隐藏滚动条 `:3879-3880`)。
**刻意核查**:`git log -S"45vh"` → `605691e`/`cb2874b` 是旧 CSS **等价迁移**产物(像素级 0 差异),
不是移动端设计决策 → **非刻意**。

### MOB-6 ☐ 滚动串联:侧栏抽屉列表滑到底会带着底下时间线一起动 [P3]
**证据**:抽屉打开时 `.project-list{overscroll-behavior:auto}` 与 `.timeline{overscroll-behavior:auto}`
完全重叠;全仓 `overscroll-behavior:contain` **只出现 1 次**(`.pop-panel` `styles.css:2067`)。
**建议**:给 `.timeline`/`.project-list`/`.changes-panel .diffwrap`/`.supervision-panel` 补
`overscroll-behavior:contain`,与 `.pop-panel` 对齐。**touches**:`styles.css`。

### MOB-7 ☐ 移动端零手势:侧栏只能靠角落 31×27 的按钮开 [P3]
全仓 `grep onTouchStart|touchstart|swipe|onPointer` **零命中**。关闭路径是通的(scrim 点击实测有效)。
低成本方案:先把 `.sidebar-show` 撑到 44×44(并入 MOB-3);边缘 swipe-to-open 可另立条目。

### MOB-8 ☐ Scheduled 页头在 390 下排版松垮("+ Create" 浮在说明文字中段) [P3]
`styles.css:4239` 的 `@media(max-width:520px)` 只处理了 `.page-action` 的换行(R2-9),没管标题区
flex 方向。**建议**:≤520 页头改 `flex-direction:column;align-items:flex-start`(参照 `styles.rs.css:692`
`.rs-archive-row` 在 ≤640 改 column 的写法)。

### ✂ 轮10 移动 finder 主动排除的假阳性(**别派 implementer 去改**)
- **✂ Home 底部钉底 composer** —— QA-45 供图刻意决策(`styles.css:4035` 注释写死 "the common
  composer is the product, not a custom landing hero")。**不碰**。
- **✂ 390 横向溢出** —— home/rich/appr/diff/sched/settings × light+dark **全部**
  `scrollWidth == innerWidth == 390`,零横向滚动。
- **✂ composer 遮挡最后一条消息** —— 富会话滚到底:最后一条气泡 bottom=603,composer top=691
  → **留白 88px,不遮挡**。
- **✂ Changes 面板在 390** —— `inset:12px` 全屏覆盖,`.diffwrap` scrollWidth==clientWidth==364,
  **diff 无横向溢出**,按钮自动换行成两排。
- **✂ 390 字号过小** —— 视口内可见文字**无 <11px**(仅 `.sched-tab-count`/`.msg-time` 为 11px,
  ≈iOS caption2,可接受)。
- **✂ 暗色模式 390** —— home/rich/appr/sched 暗色与亮色**逐像素同构**,无额外崩坏。

## C 相 · 轮12 Codex 金标并排比对新发现(2026-07-11)

**方法**:live 8809 逐屏 playwright 截图(home/rich/changes-split/scheduled/plugins/sites/prs ×
light/dark × 1440)对 `qa/codex-reference/*.jpg` 真像素金标并排比对。截图存
`qa/runs/2026-07-11-parity-r12/`。

### CX-1 ✅ 会话右栏(Changes / Supervision)是「浮动卡片」,Codex 是满高 flush 分栏 [P1]
**Codex 怎样**(`codex-diff-review.jpg`):右侧 diff 是一整列 **flush 贴到窗口右边缘、从 topbar 下沿
直落到窗口底**的分栏,与左侧对话之间只有一条 1px 分隔线;diff 内容因此拿到整个右半屏的宽度。
**我们怎样**(`qa/runs/2026-07-11-parity-r12/changes-split-light-1440.png`):`.changes-panel` 是一张
**带外边距 + 圆角 + 阴影的浮动卡片**,四周都能看见页面背景;卡片下半部在文件少时**大片空白**,而 diff
列被挤窄到长行被横向切断(`"resolved": "https://registry.npmjs.org/accepts/-/accepts-1.3.8` 直接截断)。
**差在哪**:最重要那块屏(diff/review)上最显眼的布局差 —— 我们看起来像「弹出一个面板盖在页面上」,
Codex 看起来像「这就是一个 review 工作台」。**为什么 Codex 更好**:review 时 diff 是主角,flush 分栏
把像素全给 diff;浮动卡片白白吃掉 ~60px 边距 + 阴影,还暗示「这东西是临时的、会消失」。
**关闭动作**:`.changes-panel` / Supervision 外壳去掉 margin/radius/shadow,改成 topbar 下满高列 +
1px 左边框;宽度按 Codex 比例给到右半屏。**Supervision 面板同一外壳,一起改。**

### CX-2 ✅ thread 里的折叠 turn 只写「Worked ›」,没有时长、没有步骤摘要,行间空隙巨大 [P1]
**Codex 怎样**(`codex-task-thread.jpg`):每个 turn 的折叠头是 **`Worked for 1h 37m 40s ›`** —— 带
**时长**,点开是完整步骤列表;turn 与 turn 之间靠 assistant markdown、artifact 卡、变更卡填满,thread
是**密的**。
**我们怎样**(`changes-split-light-1440.png` 左栏):todo-app 会话里连续 6 行 **`Worked ›`**(**完全没有
时长**)与 `Approved` 药丸交替,每行之间 **~140px 空隙**,中间**什么内容都没有** —— thread 看起来像
坏了 / 空的。富会话(`rich-light-1440.png`)倒是有 `Worked for 21s ›`,说明时长在**某些路径下丢失**。
**差在哪**:同样是「折叠的 turn」,Codex 一眼能看出「这一轮干了 1 小时 37 分」,我们一眼看不出任何
信息,且垂直空间浪费到 thread 显得空洞。
**关闭动作**:`Timeline.tsx` 折叠头一律渲染时长(缺时长时从 turn 首尾事件时间戳算,再不济退回步骤数
`N steps`);压缩 `Approved`/`Worked` 行之间的垂直节奏,让 thread 密起来。

### CX-3 ✅ Scheduled 每行不显示 cadence 与 next run —— 看不出「下次什么时候跑」[P1·跨层]
**已关闭(轮14, commit 760edb7)**:`internal/driver/cadence.go`(+单测)把 `DriverSpec` 投影成人话
cadence(`Every 30m` / `Saturdays at 4:00 AM` / `Best of 3` / `Self-paced` / `Runs once`)与 `NextRun`
(interval 锚上次迭代开始;五段 cron 自己解析,dom+dow 双限按 Vixie 的 OR 规则;不能表达的退回
`Cron <expr>`,**绝不猜**)。`webui/schedule.go` 是同一投影的 stdlib 镜像(arwebui 是零依赖独立
module,只经 `ar` CLI 契约对话)。`/api/runs` + `/api/sessions` 新增 `schedule`/`cadence`/`nextRunAt`。
**live 实测**:456 个 session 里 28 个 driver 带 cadence;Scheduled 行副标题
`Goal · drvmcp · 1d ago` → **`Every 30m · Ran 9m ago · cx3-ws`** / `Best of 3 · Ran 1d ago · drvbon`。
`nextRunAt` 对已终态 driver 不给(它们不会再跑了),前端诚实退回 `Ran Nd ago`,**不显示假的下次时间**。
**Codex 怎样**(`codex-scheduled.jpg`):每行副标题就是 **cadence + next run** ——
`Weekly status update draft` / **`Saturdays at 4:00 AM · Next run in 1 week`**;`cloc` / **`Daily at 6:00 AM`**。
外加底部 **Suggestions** 分区(3 张彩色图标建议:Daily brief / Weekly review / Follow-up monitor)与
「✓ Mark all as read」。
**我们怎样**(`qa/runs/2026-07-11-parity-r12/scheduled-light-1440.png`):行副标题是
`Goal · drvmcp · 1d ago` / `Best of N · drvbon · 1d ago` —— 只有**调度类型 · 项目 · 上次启动**,
**没有 cadence、没有 next run**。
**差在哪**:Scheduled 页的立身之本就是回答「这东西下次什么时候跑」。我们一个字都没答。用户盯着
27 条「Paused」却看不出任何一条的节律。
**为什么 Codex 更好**:cadence + next-run 是**唯一**能让 Scheduled 页区别于普通任务列表的信息。
**根因(已查证,不是前端偷懒)**:`Scheduled.tsx:72-75` 的注释诚实写着 "We have no cron/next-run
contract";`viewModels.ts:159 scheduleLabel()` 只能把 `schedule` 字段翻成 `Repeating`/`Scheduled`/
`Best of N`/`Goal` —— **API 根本没吐 cadence 表达式与下次触发时间**。
**关闭动作(跨层,须走 PROCESS 增量流程)**:daemon/CLI 暴露 driver 的调度规格(interval 秒数 / cron
表达式)与 `next_run_at` → webui API 透传 → 前端把行副标题换成 `<cadence> · Next run in <相对时间>`。
**⚠️ 这条要动后端契约,下轮开工前先写三层 delta,不要直接派前端 implementer。**
**轮14 更正(重要)**:「没有 cron/next-run 契约」这个前提**是错的**——后端一直有:
`internal/driver/spec.go` 定义了 `ScheduleImmediate/Interval/Cron/SelfPaced/Parallel` 五种 schedule,
`DriverSpec.Interval`(Go duration)与 `DriverSpec.Cron`(五段 cron)**都是既有字段**。缺的只是
**把它们从 spec 透传到 webui run API**,再加一个 cadence 人话描述器 + next-run 计算。
所以这不是"新增调度能力"的产品级增量,而是**暴露既有契约**的 delta(SPEC 加行,DESIGN 不变量不动)。
**教训**:代码注释里的「我们没有 X」是**写注释那一刻的认知**,不是事实。跨层条目开工前先查后端,
别把一句旧注释当成产品缺失。

### CX-4 ✅ 会话右栏 Environment 区缺常设操作行(Worktree / Create branch / Commit or push)[P1]
**已关闭(轮14, commit b2029e7)**:Environment 区改成 Codex 的**恒常四行**——
`Changes`(`+1` / `No changes`)· `Worktree`(路径尾段 `wt-20260710-143427`,展开出完整路径 + Copy path;
无 workspace 时 `—` 且禁用,**不隐藏**)· `Create branch`(右侧当前分支 / `No branch yet`;走**已有的**
`AR.gitCheckout(dir,name,create=true)`)· `Commit or push`(恒常可见;无变更时禁用 + `Nothing to commit`,
子会话标 `Sub-agent`)。**零新增 API**。implementer 真机建过分支验证(`git symbolic-ref HEAD` 确认)。
**Codex 怎样**(`codex-thread-environment-panel.jpg`):Environment 区**恒常**四行 —— `Changes` ·
`Worktree`(带展开箭头)· `Create branch` · `Commit or push`。**不管当前有没有变更、有没有分支,四行
永远在**,每行都是可点入口。
**我们怎样**(`qa/runs/2026-07-11-parity-r13/thread-light-1440.png`,`SupervisionPanel.tsx`
`EnvironmentSection`):无变更时整个区只剩 `Changes  No changes` + `No branch yet` 两行秃文字。
commit/push 动作**只在有变更时才浮现**;`Worktree` 行**不存在**;`Create branch` **不存在**。
**差在哪**:用户在会话里**没有建分支的入口**,也看不出自己在哪个 worktree 里干活。
**为什么 Codex 更好**:Environment 是「我这次改动落在哪、怎么收口」的控制台;把入口藏到"有变更之后"
= 只能事后补救,不能事前规划。恒常行还让面板有稳定的骨架,不会随状态忽长忽短。
**关闭动作**:`EnvironmentSection` 实现四行常设结构;Worktree 行显示 workspace 路径尾段 + 完整路径/
复制;Create branch 走**已有的** `AR.gitCheckout(dir, branch, create=true)`;Commit or push 恒常可见,
无变更时禁用 + 标 `Nothing to commit`(**不隐藏**)。**纯前端,已有 API 全够,不动后端。**

### ✂ 轮12 比对中主动排除的假阳性(**别派 implementer 去改**)
- **✂ sidebar 缺 `Pinned` 分区** —— 查证 `Sidebar.tsx:276-279` + `store.ts:73` + `viewModels.ts:228`:
  Pinned **已完整实现**(pin 按钮、右键菜单 Pin/Unpin、localStorage 持久化、置顶分区),只是当前
  测试数据里**没有 pinned 任务**所以不渲染。**功能在,不是差距。**
- **✂ diff 渲染质量** —— 展开后(`diff-expandall-light.png`)语法高亮、行号、+/- 底色、逐文件头
  `A package-lock.json new file +1284` **全部到位**,与 `codex-crop-diff-rendering.jpg` 同构。
  差的是**外壳布局**(见 CX-1),不是渲染。
- **✂ Home 空态** —— `home-light-1440.png` vs `codex-new-task-home.jpg`:图标 + "What should we
  build in {repo}?" + 4 张建议卡 + composer chips(project/worktree/environment/branch)+ 模型选择
  **全部对齐**。本屏无实质差距。
- **✂ Changes 面板头部动作** —— 我方 `Working tree ▾ / N files +X / 过滤 / unified|split / Expand
  all / Commit ▾ / Refresh` 实际比 Codex 头部**更全**,不需要补。
- **✂ Codex Environment 面板的 Background processes / Browser / Sources 三个分区** —— 属 Codex
  **核心差异功能**(它托管浏览器与后台进程),按对齐规则「核心差异功能不强凑」,**不做**。

## 驱动循环台账(/parity-drive,每30min一轮,只追加)

- 2026-07-11 轮1(循环启动):同步+部署 8809=index-CWvHKizj.js;收割 0(无存活
  agent);派工 finder×2(A=结构+视觉,B=交互+真实性,对 live 取证);push=
  parity-drive command + DRIVER 副本(a76f6e0)。下轮收割 findings 后派 implementer。

## F 相 · 驱动循环 finder 发现(2026-07-11 起,只追加)

- [FA-1] ✅ 右侧两面板圆角/阴影分叉(Supervision 16px双层 vs Changes 18px单层)
  ——.changes-panel 对齐 B1 卡样式;附带清 .supervision-panel 基础死声明。
- [FA-2] ✅ goal banner 删除图标被全局 button.danger 特异性压成常红框
  ——.gbar-btn.danger 静态中性化,hover 才红。
- [FA-3] ✅ goal banner 760px 比对话列/composer(720px)宽 20px 两侧突出
  ——.gbar max-width 改 720 对齐列宽。
- 2026-07-11 轮2(交互):收割 finder A(3 findings,P2×1/P3×2)→ 主线亲手修
  FA-1/2/3(styles.css)→ 5/5 playwright 断言绿+console 0 → push+部署 8809。
  finder B 在跑。
- [FB-1] ✅ Home slash 菜单向下展开整个掉出视口(composer QA-45 钉底)
  ——.cx-home .cx-slash 改向上展开,桌面/移动 6/6 项可见。
- [FB-2] ✅ 点单个 ✎ Edit goal 同时开两个编辑框且焦点落错侧
  ——goalEditSrc 标记发起处,编辑器只在被点的一侧渲染,焦点跟手。
- [FB-3] ✅ Run details 能力 chips Image/Images、File/Files 并列似重复
  ——dedupeCaps 单复数归一去重(带 vitest)。
- 2026-07-11 轮3(交互):收割 finder B(3 findings,P2×2/P3×1)→ 主线齐修
  FB-1/2/3 → tsc+104 vitest 绿 + playwright 双视口/双路径断言 + console 0
  → push + launchd 部署 8809。两镜头 finder 均已收割,F 相 6/6 全 ✅。
- 2026-07-11 轮4(headless):同步 ff→91f16eb、调和被杀轮 QA-51 PASS 证据、
  rebuild+kickstart 8809、跑同步 read-only finder 全景复查(19 稳态组合+3 复检
  console 全 0、新眼无 P1/P2,证据 qa/runs/2026-07-11-QA43-endgame/)。rebase 时
  并发 session 0a38b5a 已把 Z1 收成 ✅(补 1554/900/642+修 mobile Settings sidebar
  bug+QA-43 PASS);冲突裁决采纳对方 ✅,我方 finder 作第二镜头独立确认并入 Z1。
  push ad4567f(QA-51 调和)+本 commit。
- 2026-07-11 轮5(headless):同步 ff→0e5a529(并发 session 推 f2f1932 responsive
  +INC-60 progressive session list,live 已=index-CTcdOVfV.js)→ 判据 3 对旧 build 失效,遂
  scripts/panorama.py 全景重扫(19 面 × light/dark × 1440/390)→ error/warning/
  navfail 全 0、主题落实 19/19 一致 → 判据 3 对当前 live 重新达成。附带修 QA
  harness theme 假象(hash 导航不 reload,改 add_init_script,证据 theme_probe*.py
  排除 deep-link 主题回退 bug)。let路=无(工作区净)。push=本 commit;live=index-CTcdOVfV.js。
- [FC-1] ✅ `#/s/<sid>` 深链形式路由不剥前缀→标题栏泄漏原始 route 串
  `/s/<时间戳>…`+正文空(SSE 用带前缀错 sid)。app 自身只发裸 `#<sid>`,故
  仅 P3;但「标题泄漏内部 route 串」正是边角真实性忌讳。修:App.tsx route()
  入口 `raw0.replace(/^\/?s\//,"")` 归一,`/s/`、`s/`、裸 sid、`run:`、`scheduled`
  五形并存不误伤。真验:bare 与 /s/ 两形现完全一致渲染(标题 "what is the
  project?"、timeline 在、无泄漏、稳态 console 0)。
- 2026-07-11 轮6(headless):判据 1(全条目✅/✂,无开放☐)+判据3(轮5全景)+
  判据4(Z1/QA-43)已在册。核心:对**当前** live(index-CTcdOVfV.js,含并发
  session 新推 responsive f2f1932+INC-60 progressive session list——此前 finder
  只扫过旧 build)派同步双镜头 read-only finder → **无新 P1/P2,判据 2 对当前
  build 重新达成**(454 会话真触 progressive、390 responsive 不塌、19 面抽查
  console 0);顺手收其唯一 P3=FC-1(深链前缀泄漏)主线亲手修+真验。
  build 前补 `npm install`(jsdom 缺,并发改 package.json 后未装,memory 前科)
  →14 文件/134 vitest 绿。FC-1 产新 build vDlkgc7S 后,对**实际 live** 重跑
  panorama.py 全景(19 面×light/dark×1440/390)→ error/warning/navfail 全 0、
  主题 19/19 一致(证据 qa/runs/2026-07-11-QA43-endgame/panorama-vDlkgc7S.txt),
  判据 3 对 vDlkgc7S 亦达成。push=081e4e5+本 commit;live=index-vDlkgc7S.js。
  **四判据对当前 live 全齐,达终局,停循环(bootout headless timer)。**
- 2026-07-11 轮7(headless):收割0(工作区仅 parity-drive 自身基建脏文件,
  属循环域非并发 QA)、登记0、派工1(下述 finder)。核心:把已在 live launchd
  跑但未入库的**永不停基建**落库——watchdog.sh(清陈锁/恢复.stopped/重
  bootstrap)+cron 防自杀+DRIVER 去终局化,两 plist 与已装逐字节一致。
  **BACKLOG 真实开放条目=0**(此前 grep 的「2 个☐」经查全是图例行+台账
  「无开放☐」字样的假阳性);四闸门对当前 live(vDlkgc7S)仍全绿。按「达标
  即续航」派 read-only finder(镜头:加载/骨架态+空态/错误态)对 live 8809
  取证补弹药 → 收 4 findings(L1/L2 P1、L3 P2、L4 P3),全经 git log -S 核查
  为非刻意,登记 L 组。BACKLOG 新增开放 ☐×4。push=76708dc+本 commit;
  live=index-vDlkgc7S.js。
- 2026-07-11 轮8(headless):同步 ff→0605769(并发 session 推 DESIGN/SPEC 文档批,
  main 被 wt-20260710-230645 worktree 占用故本 checkout 走 detached HEAD +
  `push origin HEAD:main`)。收割 0(开轮无存活 agent、工作区净,让路=无)。
  **一件事**:把 L1+L2+L3 合成一个改动单元(同源于「加载态被当成确定态」、共享
  styles.css)派一个 worktree implementer 同步跑 → 交回 `2a25877`(6 文件:
  Timeline/SessionView/Sidebar/新 NotFound.tsx/styles.css 末尾追加块/新增 9 例测试;
  未提交 dist、未越白名单)。主线干净重建 dist(`rm dist/assets/*` 后 build,
  引用与 asset 两两一致)→ 143 vitest 绿 + tsc 0 → push `2a25877`+`f3ed6f3` →
  部署 8809 → **live playwright 复验 9/9 PASS**(骨架出现/假空态 0 次/29 nodes 稳态;
  not-found 卡出现/composer 0/Checking 0;冷加载红 offline 0 次采样;稳态 console
  error+warning=0)。BACKLOG:+✅×3(L1/L2/L3)、新增 ☐×1(L5,implementer 交回的
  后端 404 风险点)。开放 ☐ 剩 L4(P3 空 Changes 面板)+ L5(P2 真 404)。
  push=2a25877+f3ed6f3+本 commit;live=index-BVkYVAfh.js。
- 2026-07-11 轮9(headless):同步 HEAD=8a1581b 干净;收割 0(开轮无存活 agent);让路=无
  (注:`runtime/atmention-demo/` 是**未追踪**的本地遗留目录,它让根 module `go build ./...`
  失败——改动前的 8a1581b 上同样失败,与本轮无关;按让路纪律不 add/不删,改验
  `go build ./cmd/... ./internal/...` 全绿)。**一件事**:把剩余两条开放条目 L5(P2 后端
  真 404)+ L4(P3 Changes 空态)派两个 worktree implementer 同步跑(touches 完全不重叠:
  Go 后端/api.ts/SessionView vs DiffView/styles.css)→ 交回 `978f57b`/`cd4f229`,
  cherry-pick 成线性 `100908a`+`31b68f7`(无冲突)→ 主线 147 vitest 绿 + tsc 0 +
  webui module go test 绿 → 干净重建 dist(`rm dist/assets/*` 后 build,引用与 asset
  两两一致)→ push `e838814` → 重建 webui 二进制 + kickstart 8809 → **live 复验全 PASS**。
  ⚠️ **QA 方法学坑(重要,影响此前所有 console/请求计数)**:playwright **sync API 在
  `time.sleep()` 期间不 dispatch 事件**,回调要到下次调用 playwright API 才批量 flush。
  于是 `sleep → logs.clear() → sleep → 读计数` 这个写法里,clear() 清的是一个**尚未填充
  的空列表**,随后 flush 出来的是**整个生命周期**的事件(含加载期瞬态)——本轮一度据此
  误判"not-found 页轮询没停"(7s 窗见 5 条请求),而纯 sleep 不调 API 的探针又得出"0 条"
  自相矛盾。修正:稳态窗一律用 `page.wait_for_timeout()`(playwright API,会 pump 事件),
  并配**阳性对照**。修正后结论:ghost 页 10s 稳态窗针对 sid 请求 **0 个**、console
  error+warning **0**(加载期 3 个 404 是浏览器对 404 响应的原生日志,预期);活会话
  同窗 22 个轮询请求——对照成立,轮询确已停。此前 `panorama.py` 等用旧写法得到的
  "稳态 0" 实为"整个生命周期 0"(更严格,结论无害),但新写探针必须用 wait_for_timeout。
  探针脚本归档 `qa/runs/2026-07-11-QA43-endgame/scripts/{verify_l4_l5,probe_r9,probe_r9b,probe_r9c}.py`。
  BACKLOG:+✅×2(L4/L5),开放 ☐ 归零 → 按「达标即续航」本轮末派新镜头 finder 补弹药。
  push=100908a+31b68f7+e838814+本 commit;live=index-BmBuR-u1.js。
- 2026-07-11 轮9(补记·补弹药):L4/L5 收口后开放 ☐ 归零 → 按「达标即续航」派新镜头
  finder(**a11y:键盘可达 + 焦点可见性**,read-only,6 个 playwright 探针 + 217 处 onClick
  的 JSX 扫描)。交回 **A 相 A11Y-1..6**(1×P1 + 3×P2 + 2×P3),每条附 live 实测数据、
  file:line、参照实现与 `git log -S` 刻意核查(全部判定非刻意),并主动排除一批假阳性
  (Modals/Menu/Popover/CommandPalette 的键盘行为本已正确,别动)。**P1 = A11Y-1**:
  会话页 794 个可聚焦元素里 748 个在侧栏,按满 220 次 Tab 都到不了 composer——键盘用户
  根本无法开始工作。派工切分建议:A=A11Y-2(纯 CSS)、B=A11Y-1(App.tsx+skip link)、
  C=A11Y-3(FindBar)、D=A11Y-4+5(同为 Composer.tsx,须同人)。下轮消化。

- 2026-07-11 轮10(headless):同步 HEAD=5d2653a 干净(让路=无)。**巡检 live 8809** 两轴各下探针:
  轴A 全景 19 面(home/rich/approval/changes/changes-split/scheduled/settings × light/dark ×
  1440/390)稳态 console **error=0 warning=0 navfail=0**(闸门3 绿);轴B 量 baseline 时**当场
  挖到 P1**——静态资源零压缩零缓存(见 P 相 PERF-1)。**并发派工 6 个**(4 implementer worktree
  白名单两两无交集 + 2 read-only finder):
  - **PERF-1 ✅**(轴B,`webui/embed.go`):预压缩(启动时压一次缓存在内存,非每请求现压)+
    `assets/*` immutable 强缓存 + index.html `no-cache` + ETag/304。**live 实测 before→after**:
    JS **895,517→252,502 B**、CSS **133,732→24,657 B**、冷加载合计 **1.03 MB→271 KB(−73%)**、
    二次访问 **200 全量→304 零字节**;浏览器侧 gzip 2/2 命中且 React 正常挂载;SPA fallback 未破。
  - **A11Y-1 ✅**(`App.tsx`+`styles.css`):skip link。Tab#1 命中 skip link(可见、2px 蓝环)、
    Enter 落进 `#main`、hash 未被劫持。**748 个侧栏按钮一步跳过**(NEVER within 220 → 可达)。
  - **A11Y-2 ✅**(`styles.css` 末尾追加块):三个搜索框 `:focus-within` 焦点态(照抄
    `styles.rs.css:136-141` 的既有正确写法);`.cx-project-search` 用 box-shadow ring 避免布局位移。
  - **A11Y-3 ✅**(`FindBar.tsx`):⌘F→Esc 焦点从 `BODY` 归还到打开前的元素(实测回到 'New task')。
    implementer 顺带发现同 effect 里 `clearHighlights()` 在无 `CSS` 全局的环境会抛、且排在归还**之前**
    ——一抛就整个跳过归还,已加防御。
  - **A11Y-5 ✅**(`Composer.tsx`):8 个 Popover trigger 补 `aria-haspopup="menu"`/`aria-expanded`
    (值不是猜的:`Popover.tsx:113-116` 面板硬编码 `role="menu"`)。live 实测 7 个 home trigger
    全 false→true→false 闭环。
  - **A11Y-4 让路**(需动 `styles.css`,本轮被 A11Y-1/2 的 implementer 独占)→ 下轮做。
  **⚠️ QA 方法学坑(第二个,与轮9 的 sleep 坑同级)**:测 Tab 序必须用**全新 page**。本轮一度误判
  A11Y-1 回归(Tab#1 落到 topbar 而非 skip link),根因是探针污染:① hash 导航 `goto(BASE+"#"+sid)`
  **不重载 SPA**,焦点状态残留;② Chrome 除 `activeElement` 外还有独立的**顺序焦点导航起点**
  (记着上次**点击**的位置),`blur()` 只清前者、清不掉后者,`document.body.focus()` 更是空操作
  (body 无 tabindex)。换独立 page 后 Tab#1 立刻正确命中。**先怀疑探针,再怀疑产品。**
  **新暴露的下一层 → 登记 A11Y-7 [P2]**:skip link 落进 `#main` 顶部后,对话区 **27 个 msg-copy +
  8 个 worked-row** 仍堵在 composer 前,富会话实测**还要 40 次 Tab**,且随消息条数线性增长。
  BACKLOG:+✅×5(PERF-1/A11Y-1/2/3/5)、新增 ☐×1(A11Y-7);开放 ☐ = A11Y-4、A11Y-6、A11Y-7 + finder 待收割。
  push=7c853eb+f787d4a+d6b91a6+6baa8a6+19cb69d+863d5d2+dc57fac+本 commit;
  live=index-BV7f00Vi.js;perf 冷加载 1.03MB→271KB(−73%)、二次访问 1.03MB→0B(304)。

- 2026-07-11 轮10(补记·axe 巡检):本轮巡检补跑 **axe-core**(WCAG A+AA + best-practice,
  home/rich/changes/scheduled 四页,light):**critical=0**、serious 1 类、moderate 4 类。
  逐条核查后登记 **A11Y-8/9/10**(见 A 相):
  - **A11Y-8 [P2]** 79 处 color-contrast **全是同一个色对** `#737373` on `#f7f7f8` = **4.42:1**
    (需 4.5:1,**只差 0.08**)→ 改一个颜色 token 即可一次修完 79 处,性价比极高。
  - **A11Y-9 [P2]** `<html lang="zh-CN">` 而 UI 全英文(初版 `8f817e3` 模板遗留)→ 屏幕阅读器
    会用中文语音引擎读英文,1 行修复。
  - **A11Y-10 [P3]** 无 h1 / home+scheduled 无 `<main>` 地标 / home composer 游离在 landmark 外 /
    changes 两个 `<aside class="sidebar">` 重名。注:A11Y-1 加的是 `div#main` 不是 `<main>` 元素,
    skip link 锚点成立但没顺带补地标。
  axe 脚本归档 `qa/runs/2026-07-11-QA43-endgame/scripts/axe_r10.py`(axe.min.js 由
  `npm i axe-core --no-save` 装到 /tmp,**不污染仓库依赖**)。dark 主题的对比度**尚未扫**,下轮补。

- 2026-07-11 轮10(补记·性能 finder 收割):轴B 性能镜头 finder(read-only)交回 **7 条 findings +
  4 条主动排除的假阳性**,全部带实测数字与 file:line → 登记 **PERF-2..PERF-7**(见 P 相)。
  **两处订正了我(主驾驶)巡检时的判断,值得记住**:
  1. **baseline 的 242ms 是误判**:那是**未分页**的 `/api/sessions`(152KB),**前端从不调用**;
     首屏真正调的是 `?limit=40`(11.4KB,**~65ms**)。**量 API 要量前端真实调用的那个 URL。**
  2. **PERF-1 的收益要诚实校准**:loopback 上 895KB JS 的 resource timing `duration=4ms`,
     **本地 gzip 几乎省不了传输时间**;PERF-1 的真价值是 **ETag/304 + 远程弱网**。首屏那
     **~94ms JS eval** 得靠 **PERF-3 code splitting**(实测可减 92.6KB gz / −36.8%)才治得了。
  最猛的一条是 **PERF-2 [P1]**:空闲的**已完成**会话页仍在 **183 请求/分钟**,其中 `/events`
  每秒 fork 一次 `ar`、dump 整份 journal(193KB)**只为返回 `[]`**(实测全量与空返回耗时**相同**
  → 服务端无论如何都做全量工作)。方案 (b) 按 status 门控轮询改动最小、收益最大。
  **A11Y-9 ✅ 本轮顺手做掉**(主线亲手,1 行):`index.html` `lang="zh-CN"` → `"en"`,dist 干净重建 +
  部署 + live 复验 `lang="en"`。push=2decfc4。

- 2026-07-11 轮10(补记·移动 finder 收割 + 收轮):轴A「移动端触控 + 响应式断点」finder(本循环
  **新镜头**)交回 **8 条 findings + 6 条主动排除的假阳性** → 登记 **MOB-1..MOB-8**(见 M 相)。
  **最重要的方法学收获**:这是循环里**首次测 768/1024/1180/1280 中间断点**(此前只测 1440 和 390),
  而**两条 P1 全落在中间断点上** —— MOB-1(1024–1280 开 Changes 后 send 键压在 diff 面板上 170px)、
  MOB-2(521–880 区间 Git 分支选择器被切出视口、完全不可点)。**没测过的地方就是洞在的地方**;
  以后全景矩阵应常态包含中间断点。
  finder 还主动排除了 6 条假阳性(Home 钉底 composer 是 QA-45 刻意决策、390 零横向溢出、composer
  不遮挡末条消息、Changes 面板 390 正常、字号无 <11px、暗色与亮色逐像素同构)——**排除假阳性和
  确认真洞一样有价值**,省下下轮的无效派工。
  分派建议(finder 给的,已核 touches 无交集):MOB-1+MOB-2 同根因家族(flex 不换行 + overflow
  visible + 子项无 flex-shrink)合并给一个 implementer 用 container query 收口;MOB-3+MOB-4
  (触控目标,纯增量 CSS)给第二个;MOB-5(Settings.tsx + styles.rs.css)给第三个 —— 可三路并发。
  **轮10 收轮盘点**:push 9 个 commit(7c853eb 登记 → f787d4a PERF-1 → d6b91a6 A11Y-3 →
  6baa8a6 A11Y-5 → 19cb69d dist → 863d5d2 A11Y-1/2 → dc57fac 收 ✅ → b52d3fe 台账 →
  c5f4712 axe → 2decfc4 A11Y-9 → 996ca9c 性能登记 → 本 commit);中途 rebase 过一次并发 session
  的 `a33e988`(agent loop 重命名,与 webui 零重叠,无冲突)。
  **双轴均推进**:轴B = PERF-1 ✅(冷加载 1.03MB→271KB,二次访问→304 零字节)+ 登记 PERF-2..7;
  轴A = A11Y-1/2/3/5/9 五条 ✅ + 登记 A11Y-7/8/10 与 MOB-1..8。
  **开放 ☐ 共 17 条**:A11Y-4(让路)、A11Y-6、A11Y-7、A11Y-8、A11Y-10、PERF-2..PERF-7、MOB-1..MOB-8。
  **下轮首选**(高性价比排序):① **A11Y-8**(一个色值 `--dim:#737373`→`#6e6e6e` 修 79 处对比度,
  `styles.css:11`,dark 已另有 `#a0a0ad` 覆盖故只动 light);② **PERF-2**(方案 b:按 status 门控
  轮询,空闲页 183→35 req/min);③ **MOB-1+MOB-2**(两条 P1,同根因);④ **PERF-3**(code splitting,
  实测可减 92.6KB gz)。
  live=index-BV7f00Vi.js + lang="en";perf 冷加载 1.03MB→271KB(−73%)、二次访问 1.03MB→0B。

- 2026-07-11 轮10(补记·A11Y-8 收 ✅ + 残余登记):收轮后趁窗口把 **A11Y-8 ✅** 做掉(主线亲手,
  1 个色值):`styles.css:11` `--dim: #737373 → #6e6e6e`(4.42:1 → ~4.8:1);dark 另有 `#a0a0ad`
  覆盖故不受影响;151 vitest 绿 + tsc 0 + dist 干净重建 + 部署 + **axe 复测**。
  **before/after(axe live 实测,4 页)**:color-contrast **79 处 → 6 处(−92%)**。
  **剩余 6 处根因不同 → 登记 A11Y-11 [P2]**:`.assistant .msg-actions{opacity:.62}`
  (`styles.css:2673`)把已经合格的 `#6e6e6e` **稀释**成有效 `#a5a5a5` = **2.46:1**(全场差得最远)。
  ⚠️ 但 opacity+hover 浮现是**刻意的视觉低调设计**,**不能直接删** —— 条目里写清了三种换实现的
  路子,交下轮 implementer 权衡并出 before/after 截图。
  **这条很能说明问题**:79 处 violation 里藏着两个不同根因,**修掉大的才看见小的**——所以
  "扫描 → 修 → 再扫描"要成为常规动作,一次扫描的结论会被最大的那条噪声淹没。
  live=index-Bj4CtTwS.js + index-CvUaNrGE.css;开放 ☐ 共 17 条(A11Y-4/6/7/10/11 + PERF-2..7 + MOB-1..8)。

- 2026-07-11 轮12(headless):**比对 7 屏**(home/rich/changes-split/scheduled/plugins/sites/prs ×
  light/dark × 1440)对 `qa/codex-reference/*.jpg` 真像素金标并排。**关闭 2 个可见差距,派工 2 路并发**
  (worktree 隔离,touches 白名单零交集,两个 patch 合并时零冲突)。

  **CX-1 ✅ 会话右栏满高 flush 分栏**(最重要那块屏上最显眼的布局差):`.changes-panel` /
  `.supervision-panel` 从带 margin+radius+shadow 的**浮动卡片** → Codex 那样**从 topbar 下沿直落窗口
  底、贴右边缘、只留 1px 左分隔线**的分栏。真机实测 1440:右栏 y=54→bottom=1000、right=1440、
  radius 0 / shadow none / margin 0。**diff 列 56% 宽**,长行
  `"resolved": "https://registry.npmjs.org/accepts/-/accepts-1.3.8.tgz"` **从被横向切断变成完整显示**。
  左侧 thread 用 `calc(100% - 420px)` **把宽度地板焊进 grid track**(实测 505px)。≤900px 移动端
  overlay 在新 section 里显式重申(media query 不加特异性,不重申会被 flush 规则赢掉)——390 实测无回归。

  **CX-2 ✅ thread 折叠头永远带信息 + 密度**:裸 `Worked ›` 绝迹,三级阶梯(已存 duration → 从 fold 内
  事件时间戳自算 → 退回 `Worked · N steps`)。**根因值得记**:`completedTurnDurations()` 只给「到达了
  最终 assistant 回答」的 turn 记时长,而 todo-app 会话被 approval 切成 6 段 fold、最后死在
  `provider_server` 失败、**从头到尾没有过最终 assistant 回答** → durations 是空 Map → 6 个 fold 全退化
  成裸 `Worked`。富会话每轮正常收束所以有 `Worked for 21s`。**「只在 happy path 上填数据」的代码,在
  失败路径上就露出信息量为零的 UI。** 垂直节奏:折叠头 pitch **101px → 66px**(−35%),单行元素间隙
  18/26px → 7px。

  **方法学收获**:①「先看见差别再动手」救了本轮 —— 我原本要派 implementer 补 sidebar `Pinned` 分区,
  **查证后发现它早已完整实现**(`Sidebar.tsx:276` + `store.ts:73` + `viewModels.ts:228`),只是测试数据里
  没有 pinned 任务所以不渲染。**排除假阳性省下一整个 implementer。** ②截图脚本第一版用了
  `/#/sessions/<id>` 路由,而 app 用的是**裸 hash** `#<id>` —— 14 张截图全打偏、会话全渲染成
  "No messages yet",差点误判成重大回归。**截图 QA 必须先核对真实路由格式。**

  push=a2d1058(登记 CX-1/CX-2)+441975d(登记 CX-3)+5d8826a(CX-1+CX-2 实现);
  live=index-8uh6pCye.js;稳态 console err+warn=0(light+dark);151 vitest 绿 + tsc 0 + dist 干净。
  **开放 ☐ 新增 CX-3**(Scheduled 缺 cadence/next-run,**跨层,须先写三层 delta 再派工**)。

- 2026-07-11 轮14(headless,**接管被杀的轮13**):上一轮 20:06 开轮、20:16:41 派完 implementer 后
  **进程立刻退出**,子 agent 全被杀、零代码落地——**headless 轮必须在轮内前台阻塞等到子 agent 完成**,
  不能派完就收尾。本轮改用前台轮询 `git log --all | grep "Codex parity CX-N"` 死等两个 implementer 的
  commit,才进合并。**这是本轮最重要的流程修正。**
  **比对 3 屏**(scheduled / thread / thread-diff × light+dark)对 `qa/codex-reference/*.jpg`,
  **关闭 2 个可见差距,派工 2 路并发**(worktree 隔离,白名单零交集)。
  **CX-3 ✅ Scheduled 行显示 cadence + next run**(全栈):`Goal · drvmcp · 1d ago` →
  `Every 30m · Ran 9m ago · cx3-ws`。**最大收获是一条被写进注释的假前提**:`Scheduled.tsx:72` 长期
  写着 "We have no cron/next-run contract" —— 而 `internal/driver/spec.go` **一直有** schedule/
  interval/cron 字段。**代码注释里的「我们没有 X」是写注释那一刻的认知,不是事实**;整整一个屏的
  核心信息就因为没人回头查后端而缺席了。
  **CX-4 ✅ Environment 恒常四行**:Changes · Worktree · Create branch · Commit or push,
  不管有没有变更、有没有分支都在。此前无变更时右栏只剩两行秃文字,**用户在会话里根本没有建分支的入口**。
  **并发代价**:另一个 session 同时在重排 webui 视觉(8 个 commit,含 Scheduled 行样式与面板分区图标)。
  rebase 出一处 import 冲突(它删了分区图标 → 我方 union 后 `Crosshair`/`Package` 变孤儿,tsc 揪出)。
  **保双方语义 = 保留对方的删除 + 保留我方的新增**,不是无脑 union。
  push=ca23116(登记)+b2029e7(CX-4)+760edb7(CX-3)+08d2b4e(import 修);live=index-D3yKMQ9I.js;
  151 vitest 绿 + tsc 0 + go(driver+webui)绿 + 稳态 console err+warn=0(4 屏 × light/dark)。
  **补收(轮14 末)**:CX-3 implementer 的**终稿是 `10514c8`,而我合并的是它的中间态 `8c2e05b`**——
  轮询 commit 的那一刻它还在自查。终稿删掉了 `internal/driver/cadence.go`(**`check.sh` 的 lint-wiring
  闸门拒收**:webui 是零依赖独立 module、只经 `ar` CLI 契约对话,**无法 import `internal/`**,放那儿
  就是 main 不可达的死代码),并修了 cron `n/step` 的语义(= n..max,与 driver 真正在用的 `internal/cron`
  对齐)。已 apply 终稿差量 + push(`aa5d433`)+ 重建二进制重部署。**教训:轮询「commit 出现」只能证明
  它开始收尾,不等于它做完了——commit 出现后仍要等 agent 的完成回执,再核对终稿 sha。**
  **check.sh 全绿(exit 0)**,含 lint-docs/lint-wiring/golangci-lint/deadcode + 151 vitest。
  ⚠️ 跑 check.sh 时 **node24 必须排在 homebrew 前面**,否则 homebrew 的 node 遮住它、vitest 假失败
  10 条(`localStorage.clear is not a function`)——我本轮就被这个骗了一次。
  **后续正解(登记)**:cadence/next_run 应加进 `ar sessions list --json`(`internal/cli` + `internal/cron`),
  webui 那份镜像即可缩成读字段。

## R 组 · 轮15 Codex 金标四屏并排新发现(2026-07-11)

轮15 的第一步比对:4 个 finder 并发,分别把 live 8809 的 **Scheduled / 任务 thread /
Diff-Changes / New-task home+sidebar** 与 `qa/codex-reference/` 的金标真像素图逐屏并排。
截图存 `qa/runs/2026-07-11-round15/{scheduled,thread,diff,home}/`。
**只登记「已有后端支撑」的 UI/UX 差距**;需要新后端集成的(真 pause/resume、staged/commit
scope、行内 annotation、turn 级 diff、右栏 Background processes/Browser/Sources、后端生成
短标题)一律 out-of-scope,未登记。

### RD-1 ✅ 大 diff 的「全局折叠」让 Changes 打开后一行代码都看不到 [P0]
轮17 `f8a1834` 已修:`shouldExpandDiffByDefault`(全局)换成 `shouldExpandFileByDefault` +
`defaultOpenByPath`(逐文件,单文件 >500 行才折;整份 review 另有 5000 行首屏预算,按序展到用完),
`DiffView` 的 `allOpen` 改成 `override: boolean|null`(null = 各文件按自己的默认,Expand/Collapse all 仍可全局压过)。
**behavior**:`diffSummary.ts:409` `shouldExpandDiffByDefault` 是**全局**判据(最大文件 >500 行
→ **整个 review** 全折)。实测 `create a todo app` 会话 3 文件:只因 `package-lock.json` 有 1284
行,`package.json`(+12) 与 `server.js`(+110) 也被一起折成裸文件头 → **打开 Changes 看到三条横杠、
零行代码**。Codex 永远直接显示代码,靠 unmodified 折叠带控体量,从不把整个 review 折成文件名列表。
**证据**:`qa/runs/2026-07-11-round15/diff/light-1440x900-ff36-C1-open.png`
**刻意核查**:`f2f1932` 的注释自己声明目标是 "so 'Open Changes' shows CODE, not bare file
headers" —— 意图对、**粒度错**,非刻意 → 判据下沉到单文件。

### RD-2 ✅ 文件尾部(最后一个 hunk → EOF)没有 unmodified 折叠带 [P0]
轮17 `f8a1834` 已修:`hunkGaps(rows, {trailing})` 多吐一条 key = `rows.length` 的尾部 gap
(`end: null` = 到 EOF、长度未知——unified diff 从不给文件总行数),`FileBody` 在 rows 之后渲染尾带;
行数由 `AR.blob` 补(首屏对已展开、≤25 文件的 review 预取,拿到真数字「N unmodified lines」,
且当最后一个 hunk 本就到 EOF 时 n≤0 自动不渲染空带;超大 review 不预取,尾带先显示
「unmodified lines to end of file」,点开即解析——**不编造行数**)。diff 自证到 EOF
(尾行是 `\ No newline at end of file`)与 added/deleted 文件不出尾带。
**behavior**:`diffSummary.ts:101` `hunkGaps` 只算「每个 hunk 之前」的 gap,`DiffView.tsx:711`
的 band 也只在 `r.kind === "hunk"` 分支渲染 → 文件尾部区域**没有任何展开入口**。Codex 每个文件段尾
有 `⌄ N unmodified lines`,可一路展到 EOF。折叠功能只做了上半场(`251f986` commit message 自陈
范围是 "before the first hunk and between hunks")。
**证据**:`qa/runs/2026-07-11-round15/diff/light-1440x900-5849-B3-eof-no-trailing-band.png`

### RD-3 ✅ html/xml 无语法高亮 [P1]
轮17 `f8a1834` 已修:`EXT_LANG` 补 `html/htm/xhtml/xml/svg → "html"`,`LANGS` 加一条纯数据的
markup spec(标签名当 keyword、引号属性走既有 string 规则、`< > / =` 走 punctuation;**不引新依赖**,
复用现有 `highlightLine`)。刻意不给 `//` 行注释,否则 URL 里的 `//` 会把整行吞成注释。
**behavior**:`diffSummary.ts:246` 的 `EXT_LANG` 缺 html/xml → `dist/index.html` 的 diff 整片纯黑,
紧邻的 `.js` 满屏彩色。webui 自己的 diff 里 html 是高频文件。
**证据**:`qa/runs/2026-07-11-round15/diff/light-1440x900-5849-B2-band-expanded.png`

### RD-4 ✅ leading band 的 caret 方向反了 + 零值计数被隐藏 + 文件头计数被顶到最右 [P2]
轮17 `f8a1834` 已修:caret 一律指向被隐藏内容——leading `CaretUp`、interior `CaretUpDown`、
trailing `CaretDown`;文件头两个数无条件渲染(纯删除 = `+0 −176`);`styles.panel.css` 追加覆盖块
(`.session-view .filediff > summary.fd-head .fd-path{flex:0 1 auto}` + 新 `.fd-spacer{flex:1}`),
计数紧贴文件名、状态徽标靠右——**未动 styles.css**(归其它 slice 所有)。
`DiffView.tsx:653` leading band 用 `CaretDown`(应 `CaretUp`,caret 指向被隐藏内容的方向);
`DiffView.tsx:568` 的 `add > 0 &&` 守卫让纯删除文件只显示 `−176`(Codex `+0 -176`,且同 app 的
`ChangesOutcome` 本来就无条件渲染两个数,自相矛盾);`styles.css:1631` `.fd-path{flex:1}` 把 `+/-`
顶到面板另一端(Codex 是 `docs/DESIGN.md +8 -4` 紧贴文件名)。

### RT-1 ✅ assistant 产出的图片/截图在 thread 里完全不渲染 [P0]
**behavior**:Codex thread 把产出的截图**内联成缩略图**渲染进答案正文(单图/多图栅格),点开进
lightbox。我们三条路径全断:①`Markdown.tsx:101` 的 components map **没有 `img` renderer**,CSS 里
零条 `.md img` 规则 → `![](qa/shot.png)` 出 404 破图且无宽度约束;②`ChangesOutcome.tsx:11` 的
`DOC_KIND` 只认文档扩展名,`.png/.jpg` 掉进 "Edited N files" 纯文本行;③`Lightbox.tsx` 存在但 thread
里只有 `Thumbs` 能触发。
**后端已齐**:`GET /api/sessions/{sid}/file?path=…`(`webui/meta.go:331` / `AR.fileURL`)已能按正确
content-type 吐 workspace 任意文件(`ArtifactRow` 已在用)。**零后端改动。**

轮18 已修(三条路径全通,零后端改动):
- `Markdown.tsx` 新增 `img` renderer:workspace 相对路径(含 `./x.png` / `/x.png`)经 `AR.fileURL(sid, …)`
  解析到 session file 端点,`http(s):` / `data:` / `blob:` / `/api/…` 原样放行;sid 默认取 store 的
  `currentSid`(新增可选 `sid` prop,调用点 `Timeline.tsx` 不用改)。整段只含图片的段落渲染成
  `.md-img-grid`(单图独占、多图栅格),点击开 Lightbox——分组是该答案里 DOM 顺序的全部图片,
  `←/→` 可翻。加载失败降级成一行文件名链接(`.md-img-fallback`),不留破图。
- `Lightbox.tsx` 新增可选 `resolve` prop(默认仍是 `uploadURL`),thread 内联图与产出卡传
  workspace-file resolver;`images` 仍是原始 path,所以下载文件名依旧是真实文件名。
- `ChangesOutcome.tsx` 新增 `ImageArtifacts`:`.png/.jpg/.jpeg/.gif/.webp/.svg/.avif/.bmp/.ico` 的产出文件
  渲染成缩略图卡(> 6 张折叠),点开同一个 Lightbox;文档卡与 Edited-files 行保持原样。
- 样式在**新建**的 `styles.md.css`(`main.tsx` 里 import,未动 styles.css / conv / nav):图片限宽到正文列、
  限高 420px、圆角 + border + `cursor: zoom-in`。
**实测**(私有 8851 + 真实 daemon + 真实 Gemini 轮次,session `20260712-052313-bash-workspace-8637`,
workspace `/tmp/rt1-ws`):agent 真的用 python3 手写了 `shot.png`(蓝)/`chart.png`(红),答案正文写
`![shot](shot.png)` `![chart](chart.png)`。playwright 实测 `img.md-img` × 2,两图并排(288×181),
`naturalWidth=480` = 图片真的解码了(不是破图);`aria-label="Images produced this turn"` 的产出缩略图卡
× 2;点第二张开 Lightbox 显示 `2 / 2` 且加载的是 `chart.png`;稳态 console error+warning = 0。
vitest 179 passed(Markdown 新增 6 条 img 用例),`npm run build` 绿。
**证据**:`qa/runs/2026-07-11-round18/rt1-thread-inline-images.png`、`rt1-artifact-image-cards.png`、
`rt1-lightbox.png`(脚本 `rt1-shot.py`)。

### RT-2 ✅ Edited-files 卡与 artifact 卡左缘错位 34px [P1]
`styles.css:2829` `.changes-outcome{margin:16px 0 12px 34px}` 是**头像栏时代的遗留**(assistant
avatar 早已删)。实测 artifact 卡左缘 x=505(对齐正文),Edited-files 卡 x=553。Codex 两者与正文列
逐像素对齐。**证据**:`qa/runs/2026-07-11-round15/thread/changecard-dark-1440.png`
轮17 21d84f3 已修:`styles.conv.css` 追加 `.tl-inner > .changes-outcome{margin-left:0}`(不动 styles.css,
靠多一层祖先提升特异性)。真实会话实测 1440px:旧 bundle 变更卡 x=398 / 正文列 x=364(差 34px),
新 bundle 两者同为 x=364,与 `.worked-row`、`.tl-inner` 内容左缘同一条竖线。

### RT-3 ✅ 消息操作行常驻显示,Codex 是 hover 才浮现;内部工具名泄漏成活动行 [P2]
`styles.css:2672` `.assistant .msg-actions{opacity:.62}` → 每条 assistant 消息下常驻三图标+时间戳
(Codex 只在 hover 的消息末尾浮现)。`Timeline.tsx:196` `toolLabel` 的 default 分支把原始 tool name
当 verb → 折叠展开后出现裸的 `✓ goal_status` 行;Codex 的活动行从不出现内部标识符。
轮18 已修:
① 操作行——`styles.conv.css` **追加**块(不动 styles.css):`.msg .msg-col .msg-actions .msg-copy`
默认 `opacity:0; pointer-events:none`,`.msg:hover` 或 `.msg-actions:focus-within` 才 1 + 可点
(gate 的是 pointer-events 不是 visibility,按钮仍在 tab 序里,键盘 focus 落上即现);
`.msg.assistant .msg-col .msg-actions` 改回 `opacity:1` —— 金标
(`codex-crop-message-actions.jpg`)里常驻的是**结局**(`Goal achieved in N`)与时间戳,不是图标,
所以行本身不再半透,由图标承担 hover 态。
② 工具名——`toolLabel` 移入 `timeline.ts`(纯函数、可单测),为 `internal/tool/defs` 里**每一个**
工具配人话 verb(`goal_status`→"check goal progress"、`progress_update`→"update progress"、
`publish_artifact`→"publish"、`skill`→"run skill"…),未知工具降级成中性的 **"Ran a tool"**
而非裸标识符;`groupLabel` 的 default 同样从 `used <tool_name>` 改为 "used tools"。
实测(私有 8852 + 共享 store):`20260711-063844-workspace-app-tx-b7cd` 静息态三图标
computed opacity `0/0/0`、hover 同一条消息变 `1/1/1`(其余消息仍只剩时间戳);展开 3 个 fold /
26 条 step,step verb 全是人话(`update progress`、`check goal progress`、`$`、`write`),
`goal_status`/`progress_update` 等标识符**零出现**。稳态 console error+warning = 0。
**证据**:`qa/runs/2026-07-11-round18/rt3-{actions-rest,actions-hover,fold-expanded}.png`

### RT-4 ✅ 审批密集的 turn 在 timeline 里碎成「Approved / Worked · 1 step」阶梯 [P0]
**behavior**:Codex 一个 turn = **一条** `Worked for 1h 37m 40s ›` 折叠行,审批结果不作为顶层气泡
重复出现。我们:顶层出现 4 组 `Approved` 绿 chip + `Worked · 1 step ›` 垂直堆成阶梯,一个 9 步 turn
占 4 屏。**根因**:`timeline.ts:257` `foldable(it) && (it.kind === "tool" || finalAhead[i])` —— turn
未结算时 `finalAhead=false`,每个 work chip 都 `flush()` + 顶层输出,把一个 turn 的 fold 切成 N 段;
`Timeline.tsx:617` 的 `ActivityGroup` 只聚合**连续** tool,被 chip 一隔就退化成裸行,`Ran commands ×3`
这类聚合标签永远不出现。
**动作**:foldable chip 在 turn 未结算时也留在 `buf`(只对「答案之后」的审计 chip 保留顶层);
`ActivityGroup` 分组时跳过 chip。零后端改动。**轮15 因时间窗未派,列为下轮首选。**
**证据**:`qa/runs/2026-07-11-round15/thread/rich-workedfold-open-light-1440.png`
轮17 21d84f3 已修:`foldWork` 引入 post-answer 窗口(`answered`)取代 `finalAhead` 判据——work chip 在
整个 turn 的工作期内都留在 `buf`,只有紧跟最终答案的审计 chip 留顶层。**实测暴露 BACKLOG 未写的
真根因**:turn 常由**不可见的注入输入**(goal continuation → `runtime` item,被 feed 过滤)开启,
`foldWork` 根本看不到 user 边界,所以窗口还必须由 tool / outcome chip 关闭,否则上一轮的答案让整段
审批全漏到顶层。新增 `foldRuns()`(纯函数):chip 不再切断 tool 连续段,chip 随组内联。
`ActivityGroup` 改吃 `FoldRun`,label/count 只数 tool。
真实会话 `20260711-040811` 实测:顶层 `Approved` chip 7 → **0**,`Worked` 行 8(其中 7 条
「1 step」)→ **1 条**「Worked · 8 steps ›」;展开是一条 `Tracked progress, ran commands, read files 8`
聚合行,里面 8 tool + 7 审批 chip 按 journal 序齐全。富会话 `20260711-011831` 9 turn → 9 条 Worked 行,
顶层零 Approved,无回退。**证据**:`qa/runs/2026-07-11-round17/rt4-approval-{collapsed,expanded,group-open}.png`

### RT-5 ✅ 原始 provider 错误串直接当红 chip 抛给用户 [P1]
`timeline.ts:416` `workChip(seq, "activity failed: " + msg, "bad")` 把
`activity failed: provider_server: model returned an empty message (truncated at token cap…)`
原样贴脸。Codex 是**内联 banner + 人话 + 可执行出口**。`SessionView.tsx:791` 已有 `terminal-alert`
组件与样式,`AR.retry`(`POST /api/sessions/{sid}/retry`)后端已在 → 映射常见 provider 错误到人话 +
retry 按钮。零后端改动。
轮17 88a1c4f 已修:`timeline.ts` 新增 `explainFailure()` 把 errs.Class 全表(provider_rate_limit/
server/auth/invalid、timeout、tool_failed、canceled + token-cap 空回复与网络两条 message 启发)译成
一句人话 + 一条出路,未识别类落到通用标题但原文一字不丢;`foldEvents` 对 llm 活动失败改产
`Folded.failure`(FailureNotice,含 raw)——**被运行时自身重试救回**的降级成折叠里一条 warn
「…· retried automatically」,**没救回**的由 `SessionView` 渲染 `.turn-error` 内联 banner(人话标题 +
提示 + 「Technical details」折叠原始串 + 「Retry」调 `AR.retry`)。实测真实会话:
`20260708-224337-gin-0ea1`(rate_limit)/`20260709-091747-…-3fd8`(provider_invalid)出 banner,
`20260711-073559-create-a-todo-app-ff36`(已重试救回)出折叠 note,三者页面上都**不再**出现
`activity failed:` / `[provider_*]` 原始串。

### RT-6 ✅ 用户附件图片刷新后退化成 "×N attached" 文本 [P1]
`Timeline.tsx:798` 只有 `sentImages`(内存 Map,仅本 tab 本次发送)能出缩略图,否则渲染
`×N attached`;`timeline.ts:357` 只取了 `p.images.length`,**refs 就在手里没用**。数据 durable:
`input_received.payload.images[] = [{ref:"sha256-…"}]`,blob 实存于 `sessions/<sid>/artifacts/blobs/`。
缺的只是一条 webui ServeFile 路由(与 `handleServeUpload` 同形,`webui/api.go:659`,约 10 行,
**不碰 daemon/CLI**)。
轮18 已修:`webui/api.go` 新增 `GET /api/sessions/{sid}/image/{ref}` → 直读
`dataDir()/sessions/<sid>/artifacts/blobs/<ref>`。安全面三道:ref 走**白名单**正则
`^sha256-[0-9a-f]{64}$`(匹配即不可能含分隔符/`..`)、sid 额外钉 `filepath.Base(id)==id`(`validID`
允许 `.`/`-`,单靠它不成路径牢)、content-type **嗅探**而非信任 journal,非 `image/*` 一律 415
(同一 blob 库也装模型产出的 artifact,以 text/html 同源吐出等于给 run 的输出一个脚本执行面)。
内容寻址 → `immutable` 长缓存。前端:`timeline.ts` 的 `BubbleItem` 保留 `imageRefs`(+ 从 envelope
`correlation_id` 取 `sessionId`,投影仍是 journal 的纯函数,不必从 view 穿 sid),`Timeline.tsx`
优先 `sentImages`(本 tab 即时)→ 回落 blob URL 缩略图,点击进 Lightbox(`uploadURL` 对已是
`/api/` 的 URL 直通,Lightbox 一行不改);**全部图片都加载失败时**才回落 `×N attached` 文本。
实测(私有 8852 + 共享 store,全新 tab 从未上传过该图,再 reload):`20260710-050809-…-66c9`
用户气泡里缩略图仍在(src=`/api/sessions/…/image/sha256-eaf9…`,naturalWidth 720),`×N attached`
计数 **0**;点开 Lightbox 正常。**证据**:`qa/runs/2026-07-11-round18/rt6-{after-reload-full,
thumb-viewport,lightbox}.png`

### RT-7 ✅ 坏 deep-link 渲染成「假的空会话」而不是 Not found [P1]
`SessionView.tsx:40` 的 `isSessionNotFound` 只认 `404 || code==='session_not_found'`;非法 sid 走的是
**400** → 落进「瞬时错误」分支 → 永远轮询 + 渲染出一个可输入的空会话(标题还把 sid 拆成词)。
`SessionNotFound` 组件已存在,只需把 400/无法解析的 sid 也归到 not-found。
**证据**:`qa/runs/2026-07-11-round15/thread/bug-bad-deeplink-fake-empty.png`
轮17 88a1c4f 已修:`isSessionNotFound` 增设一条**窄**的 400 判据(须匹配 server `api.go sid()` 的
原话 `invalid session id`,别的 400 仍算瞬时、继续轮询);另加 `isValidSessionId()` 镜像 server 的
`ar.go idPattern`(`^[A-Za-z0-9._#-]+$`,≤200),语法上不可能的 sid **一个请求都不发**就落 Not found,
poll/inspect interval 与 EventSource 全部不起。实测 8846 私有 webui:`#/s/bad!id`(400)与
`#/s/this-is-not-a-real-session`(daemon 404)都立刻显示 Not found + 「Back to all tasks」,无 composer,
6s 稳态窗内该 sid 请求数 **0**;阳性对照真实会话同窗 12 个请求、正常轮询。

### RS-1 ✅ Scheduled 行标题不截断,整个列表失去节奏 [P0]
**behavior**:Codex 行标题恒为**单行**,行高一致,两行结构(title/sub-line)严格对齐,列表可垂直
扫读。我们把**原始 prompt 全文**当标题:1440 下换行 2 行,390 下换行 **4 行**,行高从 68px 膨胀到
150px+,cadence/next-run 被埋没。根因:只有 sub-line 的 `span` 有 nowrap/ellipsis,`.scheduled-copy b`
(`styles.css:3465`)没有。
**证据**:`qa/runs/2026-07-11-round15/scheduled/live-scheduled-light-390x844.png`
轮17 0105b46 已修:`.scheduled-copy b` 加 nowrap/overflow:hidden/text-overflow:ellipsis + 父 flex `min-width:0`(styles.scheduled.css 追加块);实测 1440/390 两宽度 28 行全部 rowH=68px、titleH=22px 单行。

### RS-2 ✅ 标题左缘参差 13px(去 glyph 改造留下的回归) [P1]
实测标题起始 x:有 glyph 的活跃行 449px vs settled 行 436px。`.sched-glyph` 宽 20px
(`styles.scheduled.css:14`)但占位的 `.sched-blank` 只有 **7px**(`styles.nav.css:395`)——还是老
「7px 圆点」时代的尺寸,`7c78d64` 去 glyph 时没跟着放大。**非刻意,是回归。**
轮17 0105b46 已修:`.sched-blank` 20×20px 对齐 `.sched-glyph`,并把行首未读点挪走;实测所有行(活跃/settled/未读)标题 x 恒为 429px(1440)/56px(390)。

### RS-3 ✅ 每行都有分隔线(表格感)+ 未读点在行首 + 工具条挤成一行 + 重型分段控件 [P1]
Codex:任务行之间**无分隔线**(靠留白分组,只有列表末尾一条线分隔 Suggestions);未读蓝点在行
**最右端**(左列专职状态、右端专职「有新东西」);搜索框**独占整行**、筛选行另起一行(左 tabs /
右 `✓ Mark all as read`);tabs 是**裸文字**、无外框无底色。
我们:每行 `border-bottom` → 满屏横线像日志转储;蓝点挤在行首状态列左边(还多吃 8px 缩进,是标题
左缘偏移的第二个来源);`.sched-toolbar` 单行 flex 把搜索框挤到左半边,`Mark all as read` 出现/消失
还顶动 tabs;`.sched-tabs` 是带 border+背景+box-shadow 的 iOS 式分段控件,被抬成页面第二重元素。
轮17 0105b46 已修:行 `border-bottom:0`(只留 `.scheduled-list` 末尾一条线分隔 Suggestions,hover 底色接管点击可供性);未读蓝点移到行最右(`.sched-trail` 常驻占位,读/未读不改标题宽度);`.sched-toolbar` 改 column——搜索框独占整行(pill),下面 `.sched-filters` 左 tabs / 右 Mark all as read(space-between,按钮出现消失时 tabs x 位移实测 0.0px);tabs 与 Mark all as read 均改裸文字(去 border/背景/shadow,选中态 = ink + 600)。

### RH-1 ✅ 首页标题与 project chip 互相矛盾,用户会把任务发错地方 [P0]
**behavior**:冷启动实测 —— 标题 `What should we build in cx3-ws?`,而 composer chip 显示
`Select project`,project 菜单的 ✓ 打在 **"Don't work in a project"** 上。两条独立数据源:标题来自
`Home.tsx:109` 的「最近一个非 Scratch 会话」兜底,composer 的实际工作区是 `Composer.tsx:182
useState("")`。**用户照标题的字面意思发出去,任务其实跑在无项目的 scratch 里——标题在撒谎。**
Codex 的 chip 与标题指同一个 repo,且默认已选中上次的 repo,打开即可直接发任务。
**证据**:`qa/runs/2026-07-11-round15/home/home-light-desktop.png` + `project-menu-light.png`
轮17 235c1a5 已修:标题不再自己猜——`Home.tsx` 删掉「最近非 Scratch 会话」兜底,只渲染 composer 经
`onProjectChange` 报上来的 `ws`(单一 source of truth),没有选中项目时退回中性 `What should we
build?`;composer 的 `ws` 初值改为 localStorage `arwebui.lastProject`(用户显式选择才是可靠意图;
`""` = 显式「不在项目里工作」,不会被再次 seed 覆盖),首次冷启动(键不存在)从历史里最近一个非
Scratch workspace seed 一次。实测冷启动:标题 `cx3-ws` = chip `cx3-ws` = 菜单 ✓ 打在 `cx3-ws`
(branch chip 自动解析到 `main`);换 project → 标题同步;选「Don't work in a project」→ 标题变
`What should we build?` + chip `Select project`。

### RH-2 ✅ 建议卡行与 composer 不是同一列 [P2]
1440 实测:composer `x=316 w=1100`,卡片行 `x=424 w=884`(`styles.home.css:38` `.home-empty
{max-width:900px}`)。两者同心但边缘差 108px,卡片视觉上「浮」在 composer 里面。Codex 的 4 张卡与
composer 卡左右边缘**逐像素对齐**。(修 `.home-empty` 宽度,**不缩 composer**——钉底大输入框是
QA-45 刻意决策。)
轮17 235c1a5 已修:`styles.home.css` 追加块去掉 `.home-empty` 的 900px 上限与 8px 左右内边距,让问候组
继承 hero(= composer)的列宽;composer 未动。1440 实测卡片行与 composer 卡 `x` 与 `width` 均为
316/1100,dx=dw=0。

### RH-3 ✅ 命令面板的 ⌘1..9 徽标实际永远看不见 [P1]
`CommandPalette.tsx:72` `quickNum: attention ? undefined : i+1` —— 本机 9 条 quickSwitchTasks 全是
attention,于是**整个面板一个徽标都没有**,`Tasks` 组根本不出现;可 ⌘1 仍绑到第一条 attention 任务
(`App.tsx:119`),即「能按、但没告诉你能按」。Codex(`codex-crop-command-palette.jpg`)的 Tasks 组
前 9 行每行都有 ⌘1…⌘9 徽标(有未读蓝点的行照样带),溢出的未读另开 `Unread tasks` 组。
**证据**:`qa/runs/2026-07-11-round18/before/live-palette-dark-1440.png`(零徽标)
轮18 已修:新增纯函数 `viewModels.nav.ts paletteTaskGroups()` —— `quick` 就是 `quickSwitchTasks()`
本身(`App.tsx` 的 ⌘digit 处理器索引的**同一个列表**,所以第 i 行的徽标 ⌘(i+1) 在定义上不可能说谎),
`unread` 是掉出九个数字之外的 attention 任务。`CommandPalette.tsx` 据此排出 Codex 的两组:`Tasks` 九行
**无条件**带 ⌘1…⌘9 徽标(蓝点只是多个点,不再夺走徽标)+ `Unread tasks` 组(无徽标——它们本来就没有
键可按)。单测钉住「徽标数字 = 真实绑定」:`viewModels.nav.test.ts` 5 例 + `CommandPalette.test.tsx`
6 例(含 12 条全 attention 的 live 形状)。
**实测**(8853 私有 webui 连共享 store,dark 1440):组 = `Tasks / Unread tasks / Commands`,徽标 =
`⌘1…⌘9` 九个;按 ⌘3 跳到的 sid `20260711-072852-acme-rocket-274f`,其标题与面板里标 ⌘3 的那行**逐字
相同**。截图 `qa/runs/2026-07-11-round18/rh345-palette-dark-panel.png`、`rh345-palette-groups-badges.png`、
`rh345-cmd3-jumped.png`(+ `rh345-cmd3-target.json`)。

### RH-4 ✅ 「New task」没有快捷键、nav 行没有 ⌘N 徽标 [P1]
`shortcuts.ts:48` 的 Global 组没有 New task,`App.tsx` 全局键也没有 `n` 分支 → 开新任务只能鼠标点。
Codex 的 New task 行尾常驻 `⌘N` 徽标。(需 `App.tsx` + `shortcuts.ts` + `Sidebar.tsx`,与轮15 的
implementer 白名单冲突 → 下轮做。)
轮18 已修,**绑定落在 ⌘⌥N 而不是 ⌘N**:Codex 是桌面 app,我们跑在浏览器 tab 里,而 ⌘N(新窗口)与
⇧⌘N(无痕)由 Chrome/Safari 在浏览器层吃掉——keydown **根本到不了页面**,`preventDefault` 也拦不住。
挂一个按不响的 `⌘N` 徽标就是 RH-3 那个谎的翻版,所以退到 app 已有的 ⌥⌘ 家族(⌥⌘↑/↓ 切任务)。三处
**共用同一份 token**:`shortcuts.ts` Global 组新增 `["mod","alt","N"] New task`(Settings 的快捷键表自动
带出)、`Sidebar.tsx` nav 行尾徽标由这份 token 渲染、`App.tsx` 真正触发。判键用 `e.code === "KeyN"`
(macOS 上 ⌥+N 是 dead key,`e.key` 会变成 `"Dead"`);同一分支也接受裸 ⌘N 形状,这样 Electron /
standalone PWA 等**能**投递 ⌘N 的壳里白拿 Codex 原键。在 input/textarea/contenteditable 里不劫持。
**实测**:nav 徽标 `⌘⌥N`;按 ⌘⌥N → hash 清空、New task 行 `active`。截图
`qa/runs/2026-07-11-round18/rh345-sidebar-newtask-kbd.png`、`rh345-cmdaltN-newtask.png`。

### RH-5 ✅ sidebar 放大镜打开的是内联过滤框,不是 ⌘K 命令面板 [P2]
`Sidebar.tsx:219` 的放大镜 toggle 出一条内联 `side-search` 过滤条,与 ⌘K 面板并存 → 两套搜索,⌘K 的
发现性被稀释。Codex 的 sidebar 放大镜**就是** ⌘K 面板(单一搜索入口)。
轮18 已修:放大镜改调 `onOpenPalette`(App.tsx 传入,与 ⌘K 同一个 opener);`side-search` 输入框、
`query`/`searching` state 及其在 sidebar 里的所有分支(空态文案、fold/expand 的 searching 覆盖)一并删除
—— sidebar 只负责列表,搜索只剩一个入口。`buildSidebarModel` 的 `query` 参数保留(Settings → Archived
仍在用);`styles.css` 里的 `.side-search` 死规则按白名单纪律**未动**(本轮不改 styles.css)。
**实测**:点放大镜直接开 ⌘K 面板,DOM 里 `.side-search` 计数 = 0;稳态 console error+warning = 0。
截图 `qa/runs/2026-07-11-round18/rh345-magnifier-opens-palette.png`。

### RX 组 · 轮15 判 ✂ / out-of-scope(不做,附理由)
- ✂ **首页居中 hero**:QA-45 供图定的底部钉底大输入框(`styles.css:4039` 注释),不动。
- ✂ **sidebar 行距比 Codex 松**(我们 38px pitch,Codex ~25px):`ecc95e7` 明写 "airier sidebar",刻意。
- ✂ **settled 行不画状态 glyph**:`Scheduled.tsx:47` 注释 + `7c78d64`,刻意。
- ✂ **流式 token 被隐藏成呼吸的 "Thinking"**:`styles.conv.css:515`(INC-41 A4)刻意,且 REF §3 记录
  Codex 流式态确实显示 `Thinking` 灰字。
- ✂ **👍/👎 缺失**:`Timeline.tsx:107` 注释——没有 feedback endpoint,接了就是死控件。
- ✂ **session composer 没有 chips 行**:核对金标,Codex 的 **thread** composer 同样没有 chips
  (chips 只属于 Home 的 composer)。**无差距**,我们已一致。
- ✂ **大 diff 折叠的性能意图**(`f2f1932` "responsive under real load"):保留,RD-1 只动粒度。
- ⊘ out-of-scope(需新后端):真 pause/resume;diff 的 Staged / Commit▸ / Branch scope;行内
  annotation;turn 级作用域 diff;右栏 Background processes / Browser / Sources;后端生成的人类可读
  短标题;GitHub PR / 插件 / 站点托管。

- 2026-07-11 轮16(headless)**开轮审计:上轮台账撒谎,12 条假 ✅ 已纠正回 ☐**。
  轮15 的 `effa3de` commit message 写「登记 R 组差距」,但它把 RD-1..4 / RT-1..3 / RS-1..3 /
  RH-1..2 **直接写成 ✅**——而 `git show --stat effa3de` 显示它**只改了 BACKLOG 一个文件、
  149 行纯新增、零行代码**,且 `git log -S "RD-1 ☐"` 零结果(从来没有过 ☐ 版本)。轮15 是
  「登记完就被 watchdog 杀」,implementer 一个没派、一行没改。逐条回代码核验,**12 条差距全部
  仍然成立**:`diffSummary.ts:246` EXT_LANG 仍无 html/xml;`diffSummary.ts:418` 仍是全局判据
  `largestFile <= 500`;`styles.home.css:38` 仍 `max-width:900px`;`styles.css:3465`
  `.scheduled-copy b` 仍无 nowrap/ellipsis(而同级 span 有)。
  **教训(比任何一条 UI 差距都重要):完成标记必须由代码 commit 背书,不能由「打算做」背书。**
  轮14 的教训是「commit 出现 ≠ 做完」,轮16 的教训是「✅ 出现 ≠ 有 commit」。今后登记差距一律
  写 ☐,✅ 只在 merge 进 main 且复验通过后由收轮那一步改写。

- 2026-07-11 轮17(headless)**三条并发线全部落地并复验**:比对 diff/thread/scheduled 三屏 vs 金标真像素,
  派工 3 个 implementer(worktree 隔离、白名单两两无交集)、全部自推 origin/main:
  **RD-1..4**(`f8a1834`)diff 判据下沉到单文件(大 review 不再全折成裸文件头)+ 文件尾 unmodified 折叠带
  展到 EOF + html/xml 高亮 + 文件头 `+n −m` 紧贴文件名(纯删除显示 `+0 −176`);
  **RT-4/RT-2**(`21d84f3`)一个 turn 收敛成**一条** `Worked · N steps ›`(审批会话顶层 Approved chip 7→0、
  Worked 行 8→1)+ 变更卡左缘与正文列对齐(x 398→364);
  **RS-1..3**(`0105b46`)Scheduled 标题单行截断(行高 150px+→68px 单一值)+ 标题左缘统一(429px)+
  去行分隔线/未读点移右端/搜索独占整行/tabs 裸文字。
  复验:live=`index-adzivI6A.js`,158 vitest 绿、build 绿,4 屏 × light/dark × 1440/390 稳态 console err+warn=0。
  截图 `qa/runs/2026-07-11-round17/{before,after}/`。
  **I2 的重要发现**(已写进代码注释):RT-4 根因诊断不完整——turn 常由**不可见的注入输入**(goal
  continuation → `input_received{source:program}`)开启,`foldWork` 看不到 user 边界,只按 BACKLOG 改会让
  post-answer 窗口永不关闭;窗口必须由 tool/outcome chip 关闭。**只有跑真实会话才抓得到。**
  **第二批**(同轮):**RH-1/RH-2**(`235c1a5`)首页标题与 composer chip 收敛成**单一 source of truth**
  (冷启动实测:标题 `What should we build in cx3-ws?` + chip `cx3-ws` + 菜单 ✓ 打在 cx3-ws,不再「标题说 A、
  chip 说 B」;换 project 标题同步变;localStorage 记住上次选择)+ 建议卡行与 composer 同列(dx=0/dw=0,
  原 424/884 vs 316/1100)。live=`index-CzTnO_8v.js`,console=0。
  **J2(RT-7 坏 deep-link + RT-5 provider 错误人话 banner)在轮末仍在跑,自推 origin/main,下轮开轮收割。**
  **J2 收割**(同轮末):**RT-7/RT-5**(`88a1c4f`)坏 deep-link 立判 not-found(语法非法 sid **零请求**、
  停轮询;窄 400 判据只认 server 原话 `invalid session id`,别的 400 仍算瞬时)+ provider 错误译成人话内联
  banner(`explainFailure()` 覆盖 errs 全 Class + token-cap/网络启发)+「Technical details」保留原始串 + Retry;
  **被运行时自动重试救回的失败降级为折叠内 note**(会话已往下走,红警报是撒谎)。173 vitest 绿。
  live=`index-UQM92LAm.js`。
  **轮17 合计关闭 11 条差距**(RD-1..4 / RT-2 / RT-4 / RT-5 / RT-7 / RS-1..3 / RH-1 / RH-2 —— 12 条),
  5 个 implementer 全部自推 origin/main,零 dist 提交。

- 2026-07-11 轮18(headless):比对 3 屏(thread / ⌘K 命令面板 / sidebar nav)对金标
  `codex-task-thread.jpg`+`codex-crop-command-palette.jpg`+`codex-crop-sidebar-nav.jpg`;实测我方面板
  **零个 ⌘ 徽标**、thread 图片全不渲染。关闭差距 **6 条**:**RT-1**(assistant 产出的图片内联渲染
  进正文 + 图片产出成缩略图卡 + Lightbox,P0)、**RT-6**(用户附件图片经新 blob 路由持久成缩略图,
  刷新不再退化成 `×N attached`)、**RT-3**(消息操作图标改 hover/focus 才浮现;工具名全表人话化,
  未知工具降级 "Ran a tool" 而非裸标识符)、**RH-3**(命令面板按 Codex 排 `Tasks`(⌘1..⌘9 徽标无条件)
  / `Unread tasks` 两组)、**RH-4**(New task 全局快捷键 + nav 行尾徽标;实测浏览器层吃掉 ⌘N,落在
  ⌘⌥N 并在 shortcuts.ts/徽标/handler 共用同一 token)、**RH-5**(sidebar 放大镜直接开 ⌘K 面板,内联
  side-search 删除)。派工 3 个(并发、worktree、白名单互斥,各自推)。push=`d6f604a`+`c15495b`
  +`5229b53`;8809 复验 live=`index-BV2FwMV7.js`:md-img×2 真解码、⌘1..⌘9 九个徽标全出、
  `.side-search`=0、`.msg-copy` 静息 opacity 0 → hover 1、稳态 console error+warning=0。
  截图 `qa/runs/2026-07-11-round18/{before,after}/`。**教训(两个 implementer 独立踩到)**:并发轮里
  **禁用 `git stash`**(stash 栈是全仓库共享的,跨 worktree 撞车);check.sh 前端测试须 **node24 优先于
  homebrew node25**,否则 `loadingStates.test.tsx` 10 条假失败。
  **开放 ☐ 剩**:R 组已清零;其余为 out-of-scope 的 a11y/perf/mob 组(本引擎不派)→ 下轮回第一步
  重新对着金标比对 live,开新一批差距。

## S 组 · 轮19 Codex 金标三面并排新发现(2026-07-11,sidebar / composer 菜单群 / diff-review 分栏)

三个并发 finder 截 live 8809 逐像素比对金标(`qa/runs/2026-07-11-round19/before/`)。
**登记一律 ☐;✅ 只在 merge 进 main 且复验通过后由收轮改写**(轮16 教训)。

### SB 相 · sidebar(金标 `codex-crop-sidebar-projects.jpg` / `codex-crop-sidebar-nav.jpg`)
- ✅ **SB-1(P0)打开一个任务后 sidebar 里根本没有它**。实测:current 行 `top=10262px` 而列表视口
  `153–842px`、`scrollTop` 恒为 0 → 永不露出;更硬的是某 project 里**第 7 条及以后**的任务
  `rowRendered: false` —— `visibleProjectSessions` 的 `cap=6`(`viewModels.ts:46`)把**你正在看的那条**
  藏进了 "Show more"。folded 组同理彻底消失。Codex 的 rail 永远回答「你在哪」。
  关闭:cap 截断后强制并入 current + folded 组含 current 时渲染豁免 + `scrollIntoView({block:'nearest'})`。
- ✅ **SB-2(P0)任务标题只拿到 rail 的 56%**(165px / 292px),行行省略号、相邻两行无法区分。根因
  `styles.css:3157` `.project-task{padding:6px 60px 6px 10px}` —— hover 才显形的 pin+archive 却
  **无条件常驻** 60px。Codex 标题铺到右缘(~83%),图标浮在标题上、status dot 贴右缘。
  非刻意决策:`6f7ef7e` 的意图是"加 hover 控件",不是"永久让出 60px"。
- ✅ **SB-3(P1)行高/标签/组间距比 Codex 胖 ~35%**,689px 视口只装 18 行(Codex 同高 26 行)。
  任务行 38px(`ecc95e7` 今日把 34 抬到 38,无 QA/INC 依据、方向与金标相反)、`.section-label` 34.9px
  (13.5px 字,几乎和任务文字一样响)、`.project-group` margin 16px。Codex 统一节奏 26–28px、标签 ~20px、组距 8px。
- ✅ **SB-4(P2)Projects 无段级折叠**:127 个组全渲染,`scrollHeight=14073px`。Codex 有 section 级
  `Show less` + 默认只放最近一批 repo。
- ✅ **SB-5(P2)rail 292px(20.3%)却比 Codex ~256px(17%)装得更少**;nested 缩进 21px vs Codex ~16px。
  必须在 SB-2 之后做(先还回 60px 再收窄)。同步 `.task-preview{left:296px}` 与移动端 `min(292px,86vw)`。

### CP 相 · composer 菜单群(金标 `codex-crop-add-menu.jpg` / `codex-crop-composer.jpg`)
- ✅ **CP-1(P0)「+」菜单是一堵墙**:实测 376×798px、12 行,占 900px 视口 **89% 高度**,盖死整屏。
  Codex 的 Add 只有 4 行全单行、desc 与标题**同一行**灰字、面板 ~200px。三个胖因:`pop-desc` 永远另起
  一行;Agent 分组把 5 个 persona 连 desc 平铺;Images/Files 拆两行(而 `pick()` 本就按 mime 自动分流)。
- ✅ **CP-3(P1)model 下拉少了 effort 滑杆**,改 reasoning 要 **3 次点击**。Codex pill 一点开就是 6 档
  圆点滑轨,`Model|Effort|Speed` 三行是 **Advanced 展开后**才有的——我们把 Codex 的 Advanced 页当成了根页。
  后端已就位(`specs.ts:100-108` 5 档 `EFFORT_LEVELS` + `chooseEffort`),不是壳。pill 也不显示当前档位。
- ✅ **CP-2(P1)空态 send 按钮几乎隐形**:`.cx-send:disabled` 底 `#efefef` 对卡片对比度 ≈**1.07:1**。
  Codex 空态是中灰实心圆 + 白箭头。app 最主要控件在默认态"消失"。
- ✅ **CP-4(P2)run-location chip 的菜单混装了 Task type**,且选了 Background 后 **chip 毫无变化**
  (用户以为自己在开可对话 session)。Codex 第二个 chip 只有一个意思:`Start in`。

### RV 相 · Diff/Review 分栏(金标 `codex-diff-review.jpg` / `codex-crop-diff-*.jpg`)
- ✅ **RV-1(P0)右栏被 172–206px「面板 chrome」吃掉**(占面板高 20–24%,首屏只剩 ~10 行 diff):
  `.changes-panel-head`「📄 Changes ✕」独占 48px 且与顶栏 `Changes` pill **标题重复两遍**;`.diffbar` 在
  worktree 会话里**换行成 2 行**(62px);再叠一张 616-hidden-files 双行说明卡。Codex 只有**一行**工具条。
  关闭:删 panel-head(顶栏 pill 已是 toggle)、`Apply to project…`/`Remove worktree…` 收进 `…` overflow。
- ✅ **RV-2(P0)每个文件是一张圆角卡片**(border+灰头带+14px gap+22px 内边距),Codex 是**满幅连续流**:
  文件头无背景带无边框、diff 行铺到面板边缘、文件间无 gap。代价:代码可读宽度净损 39px,`white-space:pre`
  下 `package.json` 的行直接被卡片右缘切断;纵向每文件多花 ~30px。这是与 Codex 观感差最远的一条。
- ✅ **RV-3(P1)折叠的文件只剩一条「空头」,零展开指示**(轮17 RD-1 引入的新态):`summary.fd-head`
  `list-style:none` 关掉了系统三角却没补自己的 caret → `A package-lock.json +1284 −0` 看上去像渲染失败的空卡片。
- ✅ **RV-4(P1)diff 行比 Codex 紧一档**:行高 18px(Codex 19.5)、行号与代码只隔 8px(Codex ~19px)、
  行号列固定 `3.2em`(5 位行号会溢出挤压代码列)。
- ✅ **RV-5(P2)文件头右端 `new file`/`deleted` 徽标冗余**(与最左的绿 A / 红 D 字形说同一件事),
  还把文件名挤成 `package-lock.js…`,1440 下砍掉路径 ~90px。

### 已排除(不登记)
- ✂ 标题 `Reply · …` 前缀与 92 字截断(`title.ts` `conciseTitle`,SPEC.md:173 挂 INC/QA 锚点)。
- ✂ `New task` 行常驻 `⌘⌥N` 键帽(RH-4 刻意;Codex 同样常驻)、hover 任务预览浮卡(E4,我方增益)。
- ✂ Home composer 比 thread composer 大一号(QA-45 拍板的 roomy bottom input)。
- ✂ thread mode pill 写 "Access: set by agent spec"(QA Round1 F-C3 诚实规则:live mode 未知时不撒谎)。
- ✂ model 菜单无 `Speed` 行 / add 菜单无 `Plugins` 分组(`ca2a249` 明确删除假壳)。
- ✂ 顶部 tab 条(Codex 那是 Electron 窗口 tab,我们的等价物是顶栏 `Changes`/`Supervision` pill,功能同构)。
- ✂ diff 的 add/del 底色深浅、左侧色条、`M↓` 字形、`N unmodified lines` 折叠带(轮17 已对齐)。
- ⊘ Codex 的 `Tasks` 底部分组(无 repo 归属的任务):我方每个 session 都带 workspace,**没有数据能填进去**。
- ⊘ `Attach Finder`、run-location 的 `Cloud`、`Create local environment`、`Last Turn` scope、Commit▸、
  行内 annotation:均无后端,不做壳。

## T 组 · 轮20 Codex 金标两面并排新发现(2026-07-11,Scheduled 屏 / thread 正文 + Environment 面板)

两个并发 finder 截 live 8809 逐像素比对金标(`qa/runs/2026-07-11-round20/before/`)。
**登记一律 ☐;✅ 只在 merge 进 main 且复验通过后由收轮改写**(轮16 教训)。

### SC 相 · Scheduled 屏(金标 `codex-scheduled.jpg` / `codex-crop-scheduled-{list,suggestions}.jpg`)
- ✅ **SC-1(P0)Scheduled 屏其实是「所有任务」倾倒**:实测 `rowCount=28`、列表高 1911px,26 行副行写着
  `Runs once` / `Best of 3` —— 一次性 submit run 与 best-of-N 根本不是 scheduled。真有节律的只有 1 行(`Every 30m`)。
  根因 `Scheduled.tsx:133` 全量收 runs + `:144` 主动给 `kind==="submit"` 编伪 cadence `"Runs once"`。
  后果:唯一真 scheduled 的行被淹没,Suggestions 被推到 y=2198(首屏外)。轮20 第二批派工中。
- ✅ **SC-2(P0)内容列 974px vs Codex ≈583px**:`max-width:980px` 在 1440 下从未生效(可用宽 974<980)。
  统一收到 640px(`styles.css` .page-heading/.scheduled-list/.sched-toolbar + `styles.scheduled.css` .sched-suggestions)。
- ✅ **SC-3(P1)28 行全 700 粗体长 prompt**(`.scheduled-copy b` 只设 size,weight 走 `<b>` 默认 700)→ 500。
- ✅ **SC-4(P1)副行三段事实 + cadence 被单独加重**:Codex 只有 `cadence · Next run …` 两段同档灰;
  我们多一段 project,还给 `.sched-cadence` 加 `--ink-2`+500,一行里三种视觉重量。
- ✅ **SC-6(P2)搜索框比行还重**:40.1px 高、有底色边框、input 13px(比副题还小);Codex ≈26px 细长 pill。
- ✅ **SC-7(P2)「Paused」是语义谎言**:代码注释自认 Paused == 非 active(已结束)行。文案改 `Finished`
  (不动后端;真 paused flag 见下 ⊘)。
- ✅ **SC-10/11(P3)** H1 走默认 700 在喊(→600)、390 下 `.page-heading` flex 挤压(→ 竖排)。
- ✅ **SC-5(P1)Suggestions 的 cadence 是空头支票**:卡片写 `Weekdays at 8:00 AM`,点开的 run 表单
  **没有任何 schedule 字段**(`store.ts:10` ModalKind 只有 `task?`/`preset?`)。要么把 interval/cron 传下去
  并在 `Modals.tsx` 取作初值,要么把文案改成我们真能落地的。
  **轮28 与 SC-18 一并落地 `71a8d6b`**(详见 SC-18)。
- ✅ **SC-9 + RT-1(实为 P0)`#/` 与 `#/scheduled` 深链都渲染 "Task not found"**:`App.tsx` 只剥
  `/s/` 前缀,不剥前导斜杠,于是 `"/"` / `"/scheduled"` 被当成 session id 去 `select()`。
  **不止 Scheduled——连首页 hash `#/` 都是死链**(书签、分享链接、手写 URL 全中;轮22 曾撞见但误判为
  "截图取证姿势不对",没修根因)。修:抽出 `routeHash.ts:normalizeRoute()` 统一剥前导斜杠 + 可选
  `s/` 前缀 + 尾斜杠,4 条单测钉住。轮23 落地 `5a141a5`,live 复验 `#/` → New task 首页、
  `#/scheduled` → Scheduled 页,console 0。
- ⊘ **SC-8 首屏一条 `Next run` 都没有**:`webui/schedule.go:64` 只在 future tick 可算时给 `nextRunAt`,
  已停/已完成 series 明确不给(`schedule_test.go:77` 就是这条断言)——行为诚实。SC-1 修完后需复核
  running interval driver 能否拿到;拿不到才是 bug。
- ⊘ **真 paused flag**:agent 层有(`internal/agent/goal.go:53` `ControlGoalPause`),但 webui API 未透出
  (`webui/api.go:288-299` / `webui/schedule.go:64` 无该字段)。要真 Paused 过滤须先补后端投影。
- ✂ settled 行左侧无状态点(`7c78d64` CHROME2 review sw-d-11 刻意:settled 灰点是噪音)。
- ✂ 无未读时 `Mark all as read` 整体消失(RS-3 刻意:不推动 tabs)。
- ⊘ 行标题短名化(Codex 有 LLM 生成的任务名,我方 title 即 prompt,无数据)。

### TH 相 · thread 正文 + 变更卡 + Environment 面板(金标 `codex-task-thread.jpg` / `codex-crop-{change-card,message-actions}.jpg`)
- ✅ **TH-1(P0)助手消息底部只剩一个「悬空时间戳」,缩进 94px 无锚点**:`.msg-actions` 盒子 x=350(对),
  但静息态只剩 `06:35 PM` 且实际 x=**444** —— `styles.conv.css:724-728` 给 `.msg-copy` 上的是 `opacity:0`
  而非不占位,3 个 26px 幽灵按钮把时间戳顶开。Codex 收尾行永远「图标 + 结论」左对齐正文左边缘。
  轮20 第二批派工中。
- ✅ **TH-2(P1)composer 比正文列宽 60px,竖直边线对不齐**:正文列 350→1010(660),
  `.cx-session .cx-card` 320→1040(720)。Codex 正文/变更卡/artifact/composer 共用同一条左右边线。
- ✅ **TH-4(P2)运行时 chip 不聚合**:同一条 `Agent changed · dev · gemini-flash-latest` 连着出现两遍还换行
  两排(`Timeline.tsx:846-857` 无相邻去重)。
- ✅ **TH-3(P1)Supervision 静息面板 = 三个「什么都没有」的空态块,占 236px**(面板高的 28%):
  `Goal`/`Agents`/`Attention` 各 78.6px 只装一行否定句,空态行还比真实数据行(28px)高。Codex 右栏没内容的组
  直接不出现。修法:空态收成 28px 单行 / 合并成一条 dim 摘要。**让路下轮**(涉 `SupervisionPanel.tsx` +
  `styles.panel.css`,与本轮 diff 面板 implementer 相邻)。
- ✅ **TH-5(P2)变更卡文件行不可点**(轮29 `45ea4af`:文件行 → `role=button`+`cursor:pointer`+hover 底,点它打开 Changes 面板并把该文件**强制展开**+滚入视口(store 新增一次性 `diffFocusPath`);live 复验:点 `qa42-worktree-browser.txt` → 面板开、它是唯一展开的卡,邻居 binary 仍折叠),没有「跳到这个文件的 diff」:结构已对齐金标(dim 目录 + 粗 basename +
  右侧 `+7 −0` + `Show N more files`),但每行是惰性 `<div>`,唯一出口是卡头 `Review`。需 diff 面板暴露
  `scrollToFile(path)` 锚点 → **排在 RV 组落地之后**。
- ✅ **TH-6(P3)变更卡角标字形语义不对**:用了 `GitDiff`(分叉箭头,读作"分支/合并"),Codex 是 `±` 方块。
  尺寸(38px/radius 10)已对。同理 `SupervisionPanel.tsx:522` 的 Changes 行。
- ✅ **TH-7(P3)变更卡拿不到 diff 时静默消失**:`ChangesOutcome.tsx:224-234` `.catch(() => setSummary(null))`
  + `:257` 空则 `return null` —— 后端抖一下整张「Edited N files」卡就无声蒸发,用户以为这轮没改文件;也无首帧
  skeleton。修法:失败保留卡壳 + `Couldn't load changes · Retry`。
- ✅ **TH-8(P3)折叠预览 6 行 vs 金标 3 行**(变更卡是摘要不是清单)。
- ✂ 消息 thumbs up/down(`Timeline.tsx:117-122`:无 feedback endpoint,会是死控件)。
- ✂ 静息保留时间戳 + verdict(RT-3 刻意决策;TH-1 只修它的落地 bug,不推翻决策)。
- ⊘ Environment 头部 `+`、Browser 组、Sources 组(无后端语义,不做壳);我方 `Background work` 已是对应物。

- 2026-07-11 23:5x 轮20(headless)**第一批 6 条 ✅ — diff/review 分栏满幅化 + model 菜单 effort 滑轨**。
  第一步比对:live 8809 逐屏截图 vs `qa/codex-reference/codex-diff-review.jpg` + `codex-crop-model-dropdown.jpg`
  (before 存 `qa/runs/2026-07-11-round20/before/`)。关闭的可见差距:
  **RV-1..RV-5**(右栏 chrome 从 **110px → 46px**:删 `.changes-panel-head` 重复标题带、`.diffbar` 收成单行
  nowrap、`Apply to project…`/`Remove worktree…`/refresh 收进 `…` overflow、616-hidden 说明卡 80px 双行 → 30px
  单行;文件卡片去边框/圆角/gap → **满幅连续流**,diff body 左边距 22px+border → 1px;折叠文件补 caret;
  行高 18→19.5px、行号-代码 8→18px、行号列 `3.2em` → `calc(5ch+27px)`;删冗余 `new file`/`deleted` 徽标)、
  **CP-3**(model 菜单从三级钻入压成**单根页**:模型列表 + 5 档 effort 圆点滑轨 + 次级 Advanced;改一档
  reasoning **3 击 → 2 击**,键盘 ←/→ 直接换档;pill 写 `<模型> <档位>`)。
  派工 2 个(并发、worktree、白名单互斥:A=DiffView/SessionView/styles.css/panel/rs · B=Composer/styles.composer/specs)。
  push=`4e0e2c2` + `f2754d3`;8809 复验 live=`index-DfW-iTSn.js`:`.changes-panel-head`=**不存在**、
  `.diffbar`=46px、面板顶→首个文件头=**46px**、`.filediff` border=none/radius=0、`.dl` line-height=19.5px、
  caret 出现、文件头徽标=0;model 菜单 `role=slider` + 5 个圆点、376×338;两屏稳态 console error+warning=**0**。
  截图 `qa/runs/2026-07-11-round20/{before,after}/`。vitest 21 files / 213 tests 全绿。
  **教训复现**:合并两个 worktree 后首跑 vitest 14 假失败——`PATH` 里 homebrew node25 排到了 node24 前面
  (已知陷阱,纠正后全绿)。
  同时 2 个 read-only finder 交回新弹药 → 登记 **T 组**:SC-1..SC-11(Scheduled)+ TH-1..TH-8(thread/变更卡/
  Environment)。第二批已派:C=Scheduled 收束(SC-1/2/3/4/6/7/10/11)、D=thread 收尾行+列边线+chip(TH-1/2/4)。
- 2026-07-12 00:2x 轮20 **第二批 11 条 ✅ — Scheduled 屏收束 + thread 正文收尾行/列边线**。
  关闭的可见差距:**SC-1**(Scheduled 不再是「所有任务倾倒」:新增 `hasRhythm()` 准入谓词——只收
  interval/cron/self_paced,排除 `immediate`(一次性)与 `parallel`(best-of-N);**行数 28 → 3、列表高
  1911px → 211px**、`Runs once`/`Best of 3` 字样从页面消失、**Suggestions 从 y=2198(首屏外)回到 y=490**)、
  **SC-2**(内容列 974px → **640px**;此前 `max-width:980px` 在 1440 下从未生效)、**SC-3/4/6/10/11**
  (行标题 700→500、副行删 project 段并让 cadence 回落同档灰(蓝色 next-run 成唯一强调)、搜索框
  40.1px/13px → 32px/14px 去底色、h1 700→600、390 页头竖排)、**SC-7**(`Paused` tab 是语义谎言 → 诚实改
  `Finished`,零后端)、**TH-1**(助手收尾行:`.msg-copy` 隐藏态由 `opacity:0` 改**零宽不占位**,
  静息时间戳 x **444 → 350 = 正文左边缘**,hover 图标浮现不推动正文,行高恒 19px;RT-3 决策未推翻)、
  **TH-2**(composer 与正文共用竖直边线:`.cx-card` 320→1040(720)**改为镜像 `.tl-inner` 几何** →
  350→1010(660)= `.msg-col`;收起 Supervision / 390 两态也对齐)、**TH-4**(相邻同文 chip 合并为 `×N`,
  真·重复的 `Agent changed · dev` 两条合成一条)。
  派工 2 个(并发、worktree、白名单互斥:C=Scheduled.tsx/styles.scheduled.css/styles.css ·
  D=Timeline.tsx/styles.conv.css)。push=`8af8ab8` + `dc4356a`(+ 台账 `a8a2337`)。
  8809 复验 live=`index-qVe4YxGi.js`:Scheduled rows=3 / listW=640 / suggTop=490 / tabs=All·Active·Finished /
  无 `Runs once`·`Best of`;thread `.msg-time` x=350=`.msg-col` x、`.cx-card` 350→1010=`.msg-col`、chip `×2` 合并;
  Scheduled(light/dark/390)+ thread 稳态 console error+warning=**0**。vitest 23 files / **230 tests 全绿**。
  截图 `qa/runs/2026-07-11-round20/{before,after}/`。
  **本轮合计 17 条 ✅**(第一批 RV-1..5 + CP-3 = 6 条;第二批 SC × 8 + TH × 3 = 11 条)。
  **开放 ☐ 剩**:SB-4(sidebar 段级折叠)、SC-5(Suggestions cadence prefill,跨 store/Modals)、SC-9(深链)、
  TH-3(Supervision 空态 236px)、TH-5(变更卡文件行不可点,依赖 RV 落地后的 `scrollToFile` 锚点)、
  TH-6/7/8(变更卡字形/失败态/预览行数)→ 下轮首选 TH-3 + TH-5/6/7(变更卡组)+ SB-4。

- 2026-07-12 00:5x 轮21(headless)**5 条 ✅ — Supervision 静息塌陷 + 变更卡三修 + sidebar 段级折叠**。
  比对 3 屏(thread / diff / home × light/dark × 1440/390)对 `codex-task-thread.jpg` /
  `codex-crop-change-card.jpg` / `codex-crop-sidebar-projects.jpg`。并发派 3 个 implementer
  (worktree 隔离、touches 白名单两两无交集)。关闭的可见差距:
  **TH-3** — Supervision 静息面板不再用三个「什么都没有」的空态块(`Goal`/`Agents`/`Attention`,
  **236px**)占掉面板 28% 高:空组直接不渲染,三者皆空时合并成**一条 27px dim 行**
  `Nothing needs you`(兼作 inspect 在途占位,加载→静息高度不跳);有 goal / subagent /
  approval 时各组照常(live 真机 `ar goal attach` 复验过 Goal 段 + settled 分支)。Run details 上移 209px。
  **TH-6** — 变更卡与 Environment `Changes` 行的角标从 `GitDiff`(分叉箭头,读作"分支/合并")
  改为 **±** 字形(timeline 卡手绘 24-grid `PlusMinusSquare`,Environment 行用 Phosphor `PlusMinus`;
  仓库无 lucide-react,不为一个图标加依赖)。
  **TH-7**(真 bug)— 变更卡拿不到 diff 时**整张卡无声蒸发**(用户以为这轮没改文件):改为显式三态
  `loading | ready | error` —— 加载中出 skeleton(同高不跳动)、失败**保留卡壳** + `Couldn't load
  changes` + `Retry ↻`(重发请求)、只有后端明确回「零改动」才不渲染。顺带修掉流式 refetch
  (`refreshKey = events.length`)会让卡片频闪回 skeleton 的隐患。
  **TH-8** — 折叠预览 6 行 → **3 行**,`Show N more files` 的 N = 总数 − 3。
  **SB-4** — sidebar Projects 段从**127 组全渲染**(`.project-list` scrollHeight **10505px**)收成
  **默认 8 组 / 776px**(`Show more · 119` / `Show less` 段级切换)+ **组头 chevron 折叠**
  (persist 到 `ar.sidebar.collapsedProjects`,并与 server overlay 合流:overlay 优先、否则回落
  本地镜像,冷启动首帧就正确)+ **当前会话所属组永远进渲染范围并强制展开**(超出上限则追加到尾部,
  不在用户脚下重排前 8 行)。
  push:`9e9d521`(TH-6/7/8)、`45e7071`(SB-4)、`037fc39`(TH-3/TH-6a)。
  live=`index-DZa2Gr9X.js`;vitest **255/255 绿**;全景复验 3 屏 × light/dark × 1440/390
  **console error+warning = 0**;截图 `qa/runs/2026-07-12-round21/{before,after}/`。
  **开放 ☐ 剩**:SC-5(Suggestions cadence prefill,跨 store/Modals)、SC-9(`#/scheduled` 深链)、
  TH-5(变更卡文件行不可点 → 需 diff 面板暴露 `scrollToFile(path)` 锚点)。


## U 组 · 轮22 Codex 金标 Diff/review 分栏并排新发现(2026-07-12)

finder 截 live 8809(`index-DZa2Gr9X.js`)Changes 分栏,对 `codex-diff-review.jpg` +
`codex-crop-diff-{header,rendering}.jpg` 逐项比对。**登记一律 ☐;✅ 只在 merge 进 main 且
复验通过后由收轮改写**(轮16 教训)。

结论:大结构已接近金标(满高 flush 右栏 56%、逐文件头 `A path +N −M`、sticky toolbar、
语法高亮、`N unmodified lines` 折叠条、add/del 边条都在)。剩余差距集中在**溢出与默认展开策略**。

- ✅ **DF-1(P0,功能被打断)diffbar 溢出,✕ 关闭键被推出面板**:1440×900 + worktree 会话 +
  多文件(= 我们最常见的真实会话)实测 `.diffbar` scrollWidth 692 > clientWidth 658;关闭按钮
  `right=1475` > 面板右界 1440 → **不可见,关不掉 Changes 分栏**;`.diff-viewtoggle` 被压成 **2px**
  (切不了 split);`.diff-filter` 压成 31px(输不了字)。`flex-wrap:nowrap` 把换行换成了静默裁切。
  金标 `codex-crop-diff-header.jpg` 一行装下全部控件且搜索是**图标**而非常驻 input。
  动作:`.diffbar` `min-width:0` + 关闭/切换/Commit `flex:0 0 auto` + 只让 worktree 徽章与过滤可压缩
  (或把过滤收进 `…` Popover,常驻控件 6→4)。touches:`DiffView.tsx`、`styles.css`(.diffbar 区)。
  → ✅ **已关闭**(`a68d784`)。`.diffwrap .diffbar > * { flex: 0 0 auto }` 写死一行契约,只有 `.spacer` 与
  worktree 徽章让位;常驻控件 **6 → 4**(`…` / 过滤图标 / split 切换 / Commit or push)+ ✕ —— 过滤 input 收进
  Popover(有 query 时触发器保持 active,过滤态不会被误读成空评审),Expand/Collapse-all 并入 `…`,
  徽章改 `worktree · <branch>`(≤1400px 退成纯图标 chip)。live `index-CHVun0rJ.js` 复验(1440):
  `.diffbar` 溢出 **34px → 0**、✕ right **1475(面板外,关不掉)→ 1430(面板内)**、split 切换 **2px → 63px**;
  1280 档溢出 **124px → 0**、✕ right 1404 → 1270;非 worktree/单文件对照无回退。新增 `DiffView.test.tsx` 6 条,
  vitest **268/268**;真机驱动过:过滤 4→1 文件、Collapse all、split 切换、**✕ 真的关掉了 Changes 分栏**。
- ✅ **DF-2(P0,一眼可见)构建产物默认展开,把 review 淹掉**:`D dist/assets/index-*.js +0 −176`
  **默认全展开**吐 176 行 minified React(单行 scrollWidth 1.9M px),真正要看的 `dist/index.html +2 −2`
  被埋在 4008px 之下。根因 `diffSummary.ts:463-468` `shouldExpandFileByDefault` **只看行数不看行宽/
  是否生成物**(176 < 阈值 → 展开;而 1284 行的 `package-lock.json` 反被正确折叠 → 口径不一致,
  越"宽而短"的产物越炸屏)。金标右栏是可走读的源码流。动作:加 `isGeneratedPath()` + 最长行 >500 字符
  判 minified → 默认折叠。touches:`diffSummary.ts` + 其 test。
  → ✅ **已关闭**(`5bd924b`)。`isGeneratedPath()`(dist/build/out/vendor/node_modules、`*.min.*`、
  lockfile、`assets/index-<hash>.*`)+ `longestContentLine() > 500 字符` 判 minified;生成物走 20 行小预算
  (故 `dist/index.html +2 −2` 这种 asset-hash bump 仍展开——比无脑全折更贴金标)。live `index-DJaT3qhw.js`
  复验(会话 `20260711-060645-what-agents-5849`):Changes 面板 scrollHeight **4008px → 846px(−79%)**,
  两个 dist bundle `details.open=false`(文件头与 `+0 −176` 仍在、可点开),`dist/index.html` **展开在第一屏**;
  回归会话 `…-todo-app-ff36` 2603px 完全不变;vitest **262/262**(+7 用例);console 0。
- ✅ **DF-3(P1)untracked 新文件是二等公民,两套视觉语言**:`new files (untracked) · 2` 是纯文本条
  (`DiffView.tsx:607-621`),下面两行裸路径——无 `A` 字形、无 `+N −0`、无行号、不可展开看内容,
  却排在所有真实文件之上。金标里新增文件与其它文件同款文件头。后端 `AR.blob(sid,path)` 已有
  (`DiffView.tsx:733` 已在用),**不需新后端**。动作:改成同款 `<details class="filediff">`。
- ✅ **DF-4(P1)长行被硬裁,每文件一条横滚条,无 Wrap 开关**:`.dl-text{white-space:pre}`
  (`styles.css:1586`)+ `.fd-body{overflow-x:auto}` → `"description": "A rich Todo Applicati…` 在右缘切断。
  讽刺的是**对话里的 markdown 代码块自己有 `↔ Wrap` 开关**,diff 里反而没有(同产品两套长行策略)。
  金标右栏通篇无横滚条、无半个词被切。动作:diffbar 加 wrap 开关 → `:root[data-diff-wrap] .dl-text
  {white-space:pre-wrap;overflow-wrap:anywhere}`。
- ✅ **DF-5(P2)`N unmodified lines` 折叠条没对齐代码栅格**:金标里 caret 装在占满行号沟槽宽的描边小格、
  label 从代码列起始处开始(读作"代码流的一部分");我们是 `px-[10px]` 的 flex 行(`DiffView.tsx:808-819`),
  与行号列/代码列都不对齐(读作外挂按钮)。动作:band 改 grid,首列复用 `.dl` 沟槽宽 `calc(5ch + 27px)`。
  **落地 `5d88d16`,轮28 端到端复验**(对照组 = 从 `5d88d16^` 源码构建的私有 arwebui 跑 :8866):有真实 gap 的
  会话(`…-what-agents-5849`,`@@ -5,8 +5,8 @@`)——caret 格左缘 x 792.45(偏 +10)→ **782.45**(= 行号沟槽
  782.45)、caret 格宽 18px 小圆角块 → **63.11px**(= 沟槽宽)、label 起始 x 818.45(偏 −27)→ **845.56**
  (= 代码列 845.56),**误差 0.00px**。展开/收起交互无回归(11 → 15 → 11 行)。
- ✅ **DF-6(P2)toolbar 摘要吞掉为零的一半**:金标是 `+649 -57` 并列;我们 `totalDel > 0 &&`
  (`DiffView.tsx:407-411`)只出 `+1406`,而**逐文件头**(`:651-654`)两个数字都渲染 → 同面板两套口径。
  动作:去掉 `> 0` 守卫。**落地 `5d88d16`,轮28 复验**:零删除会话(`…-d8ac`)toolbar `+1` → **`+1 −0`**,
  与逐文件头口径一致(两套 → 一套)。

## H 组 · 轮22 Codex 金标 New task 首页 / composer 并排新发现(2026-07-12)

finder 截 live 8809 home(**无 hash 的 `http://127.0.0.1:8809/`** —— `#/` 会被 hash 路由当 sid 解析成
"Task not found",App.tsx:182-201,过去几轮的 home 基线图因此全是错的)对 `codex-new-task-home.jpg` +
`codex-crop-{newtask-emptystate,composer,model-dropdown,add-menu}.jpg` 逐项比对。
**登记一律 ☐;✅ 只在 merge 进 main 且复验通过后由收轮改写。**

- ✅ **HM-1(P0,结构性)New task 整列没有最大宽度上限**:composer + 建议卡横跨 **1128px**,Codex 是
  **~640–720px 居中列**(金标 composer 占内容区 47%;`INC-41-CODEX-UI-REFERENCE.md:45` 也明写 640–720px)。
  后果:(1) 1126px 的单行输入 measure 远超可读行长;(2) `+ / Ask to approve` 与右侧 `Gemini Flash / mic / send`
  之间横着 ~1000px 空白,一条控制条被撕成两个孤岛;(3) 卡片被拉到 270px 后只有第 3 张折行 → 一排 4 张卡
  label 基线参差。根因 `styles.css:4117` `.home.home-welcome .hero { max-width: 1440px }`(commit 46345d0)。
  动作:改成 `max-width: 720px`(`.home` 已 `align-items:center`,一个值就自动居中;`styles.home.css:157-161`
  已把卡片列绑到 hero 宽度)。**不动** QA-45 的钉底/roomy input(那是纵向)。touches:`styles.css` + `styles.home.css`。
- ✅ **HM-2(P1)建议卡内部过空**:150px 高的卡里有 ~120px 死空白(icon 顶、label 底、中间全空);Codex 卡
  ≈140×89px、padding 14px、icon 与 2 行 label 间距 ~10px,是"紧凑标签"不是"内容卡片"。
  动作:`styles.home.css:99-102` `min-height 150→92`、`gap 20→12`、`padding 20→14`。**与 HM-1 同批做。**
- ✅ **HM-4(P2)headline 样式是死代码**:`styles.home.css:72-79` 写的 24px/500 **从未生效** —— 被
  `styles.css:3465` `.hero h2`(29px/580,特异性更高)覆盖。除了比金标更重更硬,更要紧的是这是条
  **沉默失效的样式**(后人调它不会有任何反应)。动作:选择器提权 + 定成 30px/500。**与 HM-1 同批。**
- ✅ **HM-6(P2)390 档:191px 的壳体包一个 88px 的输入框**(`.cx-env-strip` 95px 折 2 行 + `.cx-bar` 96px 折 2 行,
  `.cx-card` 287px = 屏高 34%)。动作:该断点 strip 改 `nowrap + overflow-x:auto`;`styles.css:3964-3966` 的
  `.cx-spacer{flex:0 0 100%}`(强制换行)存疑是否刻意 —— implementer 判断,可只做 strip 那半条。
  touches `styles.css:3960-3966` → **与 HM-1 同文件,不可并发**,并入或排下轮。

### ✂ 已确认刻意决策,不报为差距
- composer 钉底 + roomy input(`styles.css:4148-4157`):QA-45 供图定,注释与记忆在案。
- 模型菜单根页 = 模型列表 + effort 滑杆(而非 Codex 的 Model/Effort/Speed 三行摘要):INC-41 CP-3 刻意反向
  (3 次点击压成 1 次),`Composer.tsx:169-175` 有说明。
- model pill 在 effort=Off 时不写 effort 后缀:`Composer.tsx:1441-1442` 刻意。
- chip 条与输入卡合成一张卡(单外框 + 细分隔线)而非 Codex 的"灰条 + 白卡叠压":INC-41 P2 刻意
  (`Composer.tsx:1014-1019`,避免双圆角接缝)。

- ✅ **HM-3(P1)品牌标是个裸 `>_` 字符,Codex 是有轮廓的云朵团块**(`edd3981`):`Home.tsx` 内联
  `CloudMark` SVG(7 段外凸弧围成 lobed 云朵 + 内嵌 `>_`,viewBox 24、stroke currentColor、size 50),
  替换 `<Terminal size={34}/>`(原先 64px 盒子里只剩一个 34px 灰色字形,读起来像误入的标点)。
  对金标 crop 量化:chevron 高占标高 0.28(金标 0.27)、underscore 宽 0.20(0.21)、描边/标径 0.058(0.053)。
- ✅ **HM-5(P2)模型菜单 "EFFORT" 全大写,与同菜单其它 sentence-case 标题不一致**(`edd3981`):
  删掉 `styles.composer.css:184` 的 `text-transform: uppercase`(同文件 `:26` 的注释本来就写着
  *"group heading — quiet, sentence-case"*,自相矛盾)。Model / Effort / Advanced 三个标题现在同一档。

- 2026-07-12 01:4x 轮22(headless)**4 条 ✅ — Changes 分栏可用性回归 + home 品牌标/菜单体例**。
  强制第一步:截 live(`index-DZa2Gr9X.js`)Changes 分栏 + home + thread + Scheduled × light/dark,
  对 `codex-diff-review.jpg` / `codex-crop-diff-{header,rendering}.jpg` / `codex-new-task-home.jpg` /
  `codex-crop-{newtask-emptystate,composer,model-dropdown,add-menu}.jpg` 并排 → 登记 U 组(DF-1..6)+
  H 组(HM-1..6)。派 3 个 implementer(worktree 隔离,白名单两两无交集),**落一个推一个**。关闭的可见差距:
  **DF-1(P0,功能被打断)** — Changes 工具条在 worktree + 多文件(= 我们最主流的真实会话)下溢出面板:
  ✕ 关闭键被推到面板外(right 1475 > 1440)**根本关不掉分栏**、split 切换被压成 **2px**、过滤框压成 31px。
  改成一行契约 `.diffbar > * { flex:0 0 auto }` + 常驻控件 **6→4**(过滤 input 收进 Popover、Expand/Collapse
  并入 `…`、徽章可压缩)。live 复验:溢出 **34→0**、✕ right **1475→1430**、split **2→63px**;1280 档 124→0。
  **DF-2(P0,一眼可见)** — Changes 里构建产物默认全展开,176 行 minified React 把 review 淹掉,真正要看的
  `dist/index.html +2 −2` 被埋在 4008px 之下。加 `isGeneratedPath()` + 最长行 >500 字符判 minified → 默认折叠
  (生成物走 20 行小预算,故 asset-hash bump 这种仍展开)。live 复验:面板 **4008px → 846px(−79%)**,
  bundle 折叠、`index.html` 展开在第一屏。
  **HM-3** — home 品牌标从裸 `>_` 字形改成 Codex 的 lobed 云朵描边团块(内联 SVG,对金标量化 3 项比例吻合)。
  **HM-5** — 模型菜单 `EFFORT` 全大写 → sentence-case,与同菜单其它标题统一。
  **顺带修掉一个长期取证 bug**:home 的正确 URL 是**无 hash 的 `/`**,`#/` 被 hash 路由当 sid 解析成
  "Task not found" —— 过去几轮的 home 基线截图全是错屏。
  push:`5bd924b`(DF-2)、`a68d784`(DF-1)、`edd3981`(HM-3/HM-5)、`c07d372`+本 commit(台账)。
  live=`index-CHVun0rJ.js`;vitest **268/268 绿**;全景复验 4 屏 × light/dark × 1440/390 = 16 屏
  **console error+warning = 0**;截图 `qa/runs/2026-07-12-round22/{before,after,df1,df2,hm35,finder-*}/`。
  **开放 ☐ 剩**:**HM-1(P0 结构性,下轮首选)** + HM-2 + HM-4 + HM-6(同批,touches `styles.css`+`styles.home.css`)、
  DF-3(untracked 文件二等公民)、DF-4(长行硬裁无 Wrap 开关)、DF-5、DF-6、SC-5、SC-9、TH-5。

## V 组 · 轮23 Codex 金标 Scheduled 屏并排新发现(2026-07-12)

finder 对 `qa/codex-reference/codex-scheduled.jpg` 并排 live `#/scheduled`,取证含 live `/api/sessions`
实数据。截图 `qa/runs/2026-07-12-r23/finder-scheduled-*.png`。
**登记一律 ☐;✅ 只在 merge 进 main 且复验通过后由收轮改写。**

- ✅ **SC-10(P0)坏掉的 schedule 和健康的长得一模一样**:`Scheduled.tsx:322-332` 状态图标只有 3 档
  (空 / `PlayCircle` / `Circle` 兜底),`status.text` 只作 `title=` tooltip、**屏幕上一个字都没有**。
  于是 `crash`(Failed)与 `stranded`(Needs recovery)都落进灰 `Circle`,与健康 idle 行像素一致。
  live 实证:driver `20260712-033455-cx3-cadence-61b9` = `stranded` + `Every 30m` + `nextRunAt: null`,
  屏幕渲染成「灰圈 · Every 30m · Ran 4h ago」——自称 30 分钟节律、实际已死 4 小时,零提示。
  这个 hub 存在的意义就是回答"我的后台任务还活着吗",它对一具尸体回答"是"。
  动作:按 `status.cls` 分支出 warn 图标 + 把状态短语放到 next-run 位置(`Every 30m · Needs recovery`)。
  **轮23 已派 implementer SCHED。**
- ✅ **SC-11(P0)"Active" tab 结构性永远为空**:`Scheduled.tsx:141` `isActive = cls==="run"||cls==="appr"`
  —— 把 active 定义成"此刻正有一次迭代在执行",而周期任务在两次 tick 之间天然 idle。实测 live
  **All=3 / Active=0 / Finished=3**:三条活着的 `Every 30m` series 全被归进 Finished,Active tab 显示
  空态。Codex 的分法是 **series 级**(Active/Paused)。动作:判据改 series 级(有未来 tick 或 run/appr
  或 stranded/crash → Active;只有终态 → Finished)。`Paused→Finished` 的改名是刻意决策(✂,保留)。
  **轮23 已派 implementer SCHED。**
- ✅ **SC-12(P1)Scheduled 行没有任何操作,hub 是只读的**(`6caec2c`,菜单后由 `00223d7` 补 Resume/Retry/Stop/Close):hover 只有底色,右键无菜单,行的唯一行为是
  `select()`。而 `Sidebar.tsx` 早已有整套行 `ContextMenu`(rename/pin/archive/mark unread/open-in),
  `RunView.tsx:135` 有 Stop —— 唯独这个专门管长期任务的屏,不能停、不能重跑、不能改名。
  动作:`onContextMenu` + hover `⋯` 复用 `ContextMenu.tsx`,接已有 store actions。
- ✅ **SC-13(P2)行标题是原始 prompt 段落,不是任务名**(`6caec2c`,纯函数 `scheduledTitle.ts`:首句/去尾括号/48 字上限,
  raw prompt 进 `title=`;**残余**见 SC-21):`Scheduled.tsx:171/195` `title: run.label || run.id`
  = 整条提交的 prompt。live 三行里**两行都以同样的字开头**("Append one line with the current…"),
  列表无法扫读。Codex 是 2–4 词短名词("Weekly status update draft")。RS-1 的一行 clamp 只治了症状。
  动作:派生显示标题(截首句/去尾括号/~48 字上限,全文进 `title=`),或优先用 rename 名。
- ✅ **SC-14(P3)搜索命中屏幕上不存在的字段**(`6caec2c`,选了 chip 方案:`projectHit()` 命中才渲染 `.sched-project-chip`):`Scheduled.tsx:158` 的 `meta` 含 `project`,但 SC-4 已把
  project 从副行移除(✂)。实测搜 `scratch` 返回一行、而 `scratch` 在该行**任何可见文字里都没有**。
  动作:命中 project 时补一枚安静的尾部 chip,或把 project 移出 `meta`。
- ✂ **SC-21(轮31 新登记,out-of-scope)派生标题只是缓解,没真正达到金标**:SC-13 落地后,live 第 0/2 行
  仍以同样的 35 个字符开头(`Append one line with the current round number…` / `…iteration note…`),扫读时
  两行依旧「长得一样」——分歧点只是从被 clamp 掉挪进了可见区。金标的 `cloc` / `Weekly status update draft`
  是 **2–4 词的名字**,而从 prompt 首句派生天然是 6–8 词。**根因是数据层没有 title 字段**:要真对齐得让后端
  在创建 schedule 时产出短名(模型起名,或表单收一个 name)——**属新增能力,本引擎不做**。留档备真人决策。

- 2026-07-12 轮23(headless):比对 **home + Changes split + Scheduled** 三屏对金标
  (`codex-new-task-home.jpg` / `codex-diff-review.jpg` / `codex-scheduled.jpg`)。
  **关闭 9 条可见差距(含 3 条 P0)**:
  **RT-1/SC-9(P0)** `#/` 与 `#/scheduled` 深链都渲染 "Task not found" —— hash 路由不剥前导斜杠,
  **首页 hash 本身是死链**(轮22 撞见过但误判为"截图姿势不对");抽 `routeHash.ts:normalizeRoute()` 修根因。
  **HM-1(P0 结构性)** New task 从满宽 1128px 收进 **720px 居中窄列** —— headline / 4 卡 / composer
  实测 `l=492 r=1212` 逐像素对齐(此前 composer 被撑宽、chips 与模型选择器撕成左右孤岛);
  **HM-2** 卡片 150→**102px**(死空白真凶是 `justify-content: space-between`);**HM-4** headline 样式
  从死代码复活成 30px/500;**HM-6** 390 档 composer 壳体 191→**97px**(strip 51 + bar 46,均单行零溢出;
  没用横向滚动——`.pop-panel` 向上弹出,任何非 visible overflow 都会裁掉菜单)。
  **SC-10(P0)** 坏掉的 schedule 不再伪装健康:`stranded`/`crash` 换琥珀 `WarningCircle` + 状态短语
  上屏(`Every 30m · Needs recovery`);**live 实况是三具尸体齐声报"活着"**,现在三行全部报警。
  **SC-11(P0)** Active tab 判据从 tick 级改 series 级:**Active 0 → 3 行**,Finished 诚实地空了。
  **DF-3** untracked 文件不再是二等公民(同一套文件头 + `+N −0` + 可展开;后端早已把小文本内联,
  剩下的 binary/超大文件渲染成同款 A 卡 + binary badge);**DF-4** diff 工具条加 **Wrap 开关**
  (与对话代码块同图标/文案,`localStorage["ar.diff.wrap"]` 持久化;实测 2 个溢出 → 0)。
  派工:3 个 implementer 并发(worktree 隔离,白名单两两无交集:HOME=`styles.css`+`styles.home.css` /
  DIFF=`DiffView.tsx`+新建 `styles.diff.css` / SCHED=`Scheduled.tsx`+`styles.scheduled.css`)+ 1 个 finder;
  RT-1 主线自做。**落一个推一个**。
  push:`5a141a5`(RT-1)、`ee42560`(SC-10/11)、`37f1ab8`(HM 组)、`0e308df`(DF-3/4)、`cf121e4`+`7eb715a`+本 commit(台账)。
  live=`index-C8aYGFai.js`;vitest **282/282 绿**;全景 5 屏 × light/dark × 1440/390 = 20 屏
  **console error+warning = 0**;截图 `qa/runs/2026-07-12-r23/`。
  **开放 ☐ 剩**:SC-12(Scheduled 行零操作,hub 只读)、SC-13(行标题是原始 prompt 段落)、
  SC-14(搜索命中不可见字段)、DF-5、DF-6、SC-5、TH-5。

## W 组 · 轮24 finder 新弹药(thread/artifact + ⌘K 命令面板)(2026-07-12)

**登记一律 ☐;✅ 只在 merge 进 main 且复验通过后由收轮改写**(轮16 教训)。

- ✅ **TH-9(P0)同一对图片一屏渲染三遍,吃掉 617/723px 阅读区**:正文内联大图(`Markdown.tsx`
  `md-img-grid`)+ artifact 缩略卡(`ChangesOutcome.tsx:85` `ImageArtifacts` **无条件渲染**)+ 变更卡文件行
  —— 同两张 PNG 出现 3 层。Codex 的产出物只出现一次,artifact 行只用于**文档**,从不重复正文已画过的东西。
  修:`Markdown.tsx` 按 session 建内联图 path registry(挂载登记/卸载销号),`ImageArtifacts` 用
  `useSyncExternalStore` 订阅并滤掉正文已内联的 path,全内联则整行不渲染。
  轮24 落地 `a070dea`:图渲染 **4 → 2** 次、图片占高 **617 → 409px**、thread scrollHeight 1019 → 856px。
- ✅ **TH-13(P2)`Edited 2 files +0 −0` —— 卡头自己写了个假零,还和面板打架**:二进制/新建文件
  `countsKnown=false`,`ChangesOutcome.tsx:369-372` 直接吐 0 → 卡头说"改了 2 个文件但一行没动"(假话,
  还带绿/红着色);同屏 Supervision 却正确写 `Changes · 2 new`。修:± 只统计 `countsKnown` 的文件,
  未知的按面板口径报 `N new`(全未知 → `2 new`;部分已知 → `+4 −2 · 1 new`)。轮24 `a070dea`。
- ✅ **CP-5(P0)⌘K 键盘选中项滑出可视区,Enter 打开的是你看不见的任务**:`CommandPalette.tsx:113-125`
  的 `onKey` 只 `setIdx()`,全文件**无 `scrollIntoView`**。实测 `.cmdk-list` clientH=468 / scrollH=1149,
  ↓×16 后选中行 y=927–970 而 `scrollTop` 仍是 **0**(`inView:false`)—— 24 行里 14 行"键盘可达但不可见"。
  修:`useEffect(…scrollIntoView({block:"nearest"}),[idx])` + `onMouseEnter` 加 kbdNav 闸(静止鼠标下滚过
  的行不再抢选中)。轮24 落地 `719a034`:↓×16 后 `inView:true`(scrollTop 91)。
- ✅ **CP-6(P1)状态圆点在 rail 和 ⌘K 里含义不一致**:`CommandPalette.tsx:160-164` 硬编码
  `"status-dot" + (attention ? " unread" : "")`,把 `friendlyStatus(status).cls` 整个丢掉 —— rail 里
  `appr` 琥珀 `rgb(138,90,0)` 的任务在 ⌘K 里是 `unread` 蓝 `rgb(1,105,204)`,面板 18 个点全同一个蓝。
  而 `viewModels.ts:181-189` 的注释恰恰承诺"命令面板与 sidebar 的点色一致" —— **代码与自己的设计注释矛盾**;
  一个卡在等审批/已崩溃的任务被宣传成"有新活动"。修:`Item` 带 `dot` 走 `friendlyStatus`。轮24 `719a034`。
- ✅ **CP-7(P1)归档任务在 ⌘K 搜索里悄悄复活且无标记**:`CommandPalette.tsx:79-92` 的 query 分支直接 map
  `sessions`,从不查空查询分支所用的 `archived` 集合 → 归档后 rail 消失、空查询面板消失,一键入关键词它
  **原样回到 `Tasks` 分组**。修:归档命中进独立 `Archived` 分组排在活任务之后(保可达 + 诚实标注)。轮24 `719a034`。
- ✅ **CP-8(P2)⌘K 到不了 Scheduled**(app 仅有的另一个顶层目的地),也没有 `Open settings`:后端全在
  (`Page="home"|"scheduled"`、`showPage()`、Settings 整页 + ⌘,),只是没接线。修:`cmds` 加
  `Go to Scheduled` + `Open settings`(复用 App 的 `openSettings`,未另起 handler)。轮24 `719a034`。
- ✅ **CP-9(P2)面板浪费窗口**:`.cmdk` 560×523.8 在 900px 视口只占 58%,`.cmdk-list` 59% 滚出视野,
  行高 43.3px(金标 ~36–38)。修:`max-height: min(64vh,620px)`、行高 35px → 可见行 **10.8/24 → 16.5/26**。轮24 `719a034`。
- ✅ **TH-10(P1)助手消息收尾行静息态只剩孤零零一个时间戳,3 个功能入口不可见**(`30bd173`:`.msg-copy` 静息
  `opacity:.5`、hover 升到 1,不再 `width:0`):`styles.conv.css:753-765`
  把 `.msg-copy` 静息压成 `width:0/opacity:0`,只有 `.msg:hover` 才展开 —— Copy message / Copy link /
  **Continue in new task** 三个入口对不扫鼠标的用户**不存在**(触摸设备彻底没有)。金标两条不同消息的收尾行
  都是 `⧉ 👍 👎 ↗ + 时间戳/verdict`,**操作图标静息就在**。`styles.conv.css:737-739` 的注释把金标 verdict
  那枚 ✓ 圆圈误读成"整行只有一个图标"据此把 4 枚图标全藏了 —— **是误读金标,不是用户裁决**。
  轮24 已派 implementer。
- ✅ **TH-11(P1)终态 chrome 138px + 左边线三级台阶 282/320/350**:`.terminal-alert`(`styles.panel.css:397`
  `margin:0 18px`)x=282 w=796 h=85、`.gbar`(`styles.css:4330` `max-width:720px`)x=320 —— 而正文 `.msg-col`
  与 composer `.cx-card` 都是 x=350 w=660(TH-2 立起来的共享竖直边线)。两条横幅一左一右各捅出 68/30px,
  且固定占 138px(阅读区 −31%)。动作:`.terminal-alert` 改 `margin:0 auto; max-width:660px` + 压成单行;
  `.gbar` 720 → 660。**不推翻 QA-45「诚实异常终止 banner 必须存在」的决策,只对齐 + 压扁。**
  **主体轮27 落地 `3fe7bd7`,轮28 `e9e6145` 收尾**(按钮 `.terminal-alert-action` 的 6px 纵向 padding 让按钮
  33px 撑高整行 → 降为 4px + line-height 1.35)。**live 8809 实测**(富会话 297d,1440):`.terminal-alert`
  x=282 w=796 h=85 → **x=350 w=660 h=43**、`.gbar` x=320 w=720 → **x=350 w=660**,两条横幅与正文 `.msg-col`、
  composer `.cx-card`(x=350 w=660)**共用同一条竖直边线**;终态区总高 ~138px → **93px(含 gap)**。
  banner 与 `[Continue in new task]` 都还在(QA-45 决策未被推翻),只是不再撑高一行。
- ✅ **TH-12(P1)同一个事实一屏说 3–5 遍**:goal 取消说 3 遍(in-thread chip `timeline.ts:829` + `.gbar` +
  Supervision Goal 组)、step limit 说 2 遍(红 chip `timeline.ts:856` + `.terminal-alert`),外加一枚装着
  整句 goal 的 494px pill(`timeline.ts:816` fallback)。合计 5 处 ~300px 讲两件事。Codex 一个终态只说一次。
  动作:`SessionView` 已渲染 `.gbar`/`.terminal-alert` 时抑制重复 chip(`timeline.ts:799` 对 `goal attached`
  已有同类 `noted` 抑制逻辑,推广到 paused/cancelled/limit);fallback chip 的 goal 文本截断到 ~32 字。
  **轮27 落地 `3fe7bd7`,轮28 live 复验**:`suppressEchoedChips()`(`timeline.ts:520`)+ `SessionView.tsx:644`
  接线,`echo` 标记覆盖 goal cancelled / paused / step limit;`clipGoal()` 把 fallback goal pill 截到 32 字。
  live 实测:主列 "Goal cancelled" 出现次数 **3 → 1**。
- ✅ **SB-6(P1)Projects 树是倒的:子任务比父项目名还靠左 23px,父子字体完全同款**:`.project-heading span`
  x=**57**,而它自己的 `.project-task-title` x=**34**;两者 size/weight/color 三项全同(14.5px/400/`rgb(96,96,96)`)。
  根因:caret(11px)+ folder(16px)+ gap 都在文档流里,把组名推到了自己孩子的右边(`Sidebar.tsx:388-394`)。
  金标里 folder 图标吊在左 gutter,**项目名与其下嵌套任务标题起始 x 完全对齐**。动作:caret+folder 绝对定位
  进 34px gutter,heading 文字左边界拉到 34px;再给 heading 一档不同字重/墨色,让组名成为锚而非同辈。
  **轮25 落地 `9634450`**:caret+folder 绝对定位进 26px 左 gutter(`styles.nav.css` 高特异性覆盖,未碰
  `styles.css`),heading `padding-left:26px`;folder 常驻 gutter、hover 时 caret 顶替。live 复验:
  `.project-heading span` x **57 → 34** = `.project-task-title` x 34(**父子同列**),heading
  **400/`rgb(96,96,96)` → 600/`rgb(13,13,13)`**(组名成为锚),稳态 console 0。
- ☐ **SB-7(P2)rail 把 `appr`(能动手)和 `stranded`(已坏掉)涂成同一个琥珀 `rgb(138,90,0)`**,不可区分。
  (轮25 让路:色值定义在 `styles.css:3336` `.status-dot.stranded`,与本轮 TH-11 的 implementer 撞白名单。)
- ✅ **SC-15(P3)Scheduled 选中 tab 没有 pill 底色**:`styles.scheduled.css:222` RS-3 刻意去掉了 border+fill+
  shadow 三件套,但金标裁剪图(`codex-crop-scheduled-list.jpg`)显示 Codex 的选中 tab **确实有**一层浅灰 pill
  (只是没有边框和阴影)。动作:只补 fill,不补 border/shadow —— RS-3 的实质(不做 iOS segmented control)保留。
  **轮25 落地 `9634450`**:`.sched-tab.on` 只补 `color-mix(in srgb, var(--ink) 7%, transparent)` + radius 8px;
  live 复验 `border:0px` / `box-shadow:none` 保持,RS-3 实质未动。

- 2026-07-12 轮24(headless):比对 **Scheduled / Changes split / home / thread** 四屏对金标
  (`codex-scheduled.jpg` + `codex-crop-scheduled-list.jpg` / `codex-diff-review.jpg` / `codex-new-task-home.jpg` /
  `codex-task-thread.jpg` + `codex-crop-{message-actions,artifact-cards,command-palette,sidebar-projects}.jpg`)。
  **两批并发派工,落一个推一个。**
  第一批 2 个 implementer:**SC-12/13/14**(`6caec2c`)—— Scheduled 从只读列表变成可操作 hub:行标题由
  **整段原始 prompt(87 字,两行同头无法扫读)→ 派生短名(46 字,全文进 tooltip)**、行接 `onContextMenu`
  + hover `⋯` 复用已有 `ContextMenu`(Pin/Rename/Mark unread/Archive/Copy;run 行给 Stop)、搜索命中 project
  时行尾补一枚安静 chip(此前搜 `scratch` 命中的行里根本没有 `scratch`);**DF-5/DF-6**(`5d88d16`)——
  `N unmodified lines` 折叠条从"外挂按钮"落到代码栅格上(caret 格左缘 792.45 → **782.45 = `.dl-no` 左缘**、
  格宽 18 → **63.11px = 行号列宽**、label 左缘 818.45 → **845.56 = 代码列左缘**),工具条计数不再吞掉为零的
  一半(`totalDel=0` 时 before 不渲染 → after `−0`,与文件头 `+1 −0` 同口径)。
  第二批 3 个 implementer:**TH-9/TH-13**(`a070dea`)、**CP-5..CP-9**(`719a034`)、**TH-10**(在跑)。
  push=`5d88d16` `6caec2c` `a070dea` `719a034` + 台账;vitest **320/320 绿**;live=index-DKUBel51.js(第一批)。
  截图 `qa/runs/2026-07-12-r24/{before,after,finder-thread,finder-sidebar,impl-diff,impl-img}/`。
  BACKLOG:**+✅ 12 条**(SC-12/13/14 + DF-5/6 + TH-9/13 + CP-5..9)、新增 ☐ 5 条(TH-10 在跑、TH-11、TH-12、
  SB-6、SB-7、SC-15)。**下轮首选**:TH-11 + TH-12(终态 chrome 对齐 + 去重,同一个 implementer)、SB-6(Projects
  树缩进倒置)。

### 轮25 新弹药 · home/composer(finder H)+ Changes/diff(finder D)
**登记一律 ☐;✅ 只在 merge 进 main 且复验通过后由收轮改写**(轮16 教训)。
- ✅ **HM-P0(P0)dark 下 composer 卡描边是硬编码浅灰,整页最扎眼**:`styles.css:3986` 的
  `.cx-card { border-color: #dedede }` 是**裸字面量**、不在任何 `@media`/`[data-theme]` 作用域里,且排在
  `styles.css:1839` 的 `border: 1px solid var(--line)` 之后 → **两个主题都赢**。dark 下实测 `rgb(222,222,222)`,
  而同屏走 token 的建议卡是 `rgb(42,42,48)` —— 一屏两卡,描边差 180 个亮度点,页面上最大的元素套了一圈近白光圈
  (`.cx-card:focus-within` 反而是对的,所以**错的恰是你打开 New task 第一眼的静息态**)。
  **轮25 落地 `495b0ba`**:`#dedede` → `var(--line)`。live 复验 dark `rgb(222,222,222)` → **`rgb(42,42,48)`**
  = 与建议卡同边线;light `rgb(231,231,231)` 一致。
- ✅ **DF-D1(P0)Changes 文件头:文件名不是黑的,整条头都是 `--dim`**:我们自己的金标文字参照
  `INC-41-CODEX-UI-REFERENCE.md:167-168` 白纸黑字写着「目录灰、**文件名黑**」,但 `.fd-path`(`styles.css:1706`)
  不设 color → 继承 `details summary { color: var(--dim) }`(`styles.css:964`),与 `.fd-dir` **完全同色**
  `rgb(110,110,110)`。review 屏里最重要的标签(你正在读哪个文件)以 55% 灰呈现,读起来像 disabled 态。
  **轮25 落地 `495b0ba`**:`.fd-path { color: var(--ink) }`,`.fd-dir` 保持 `--dim`。live 复验
  light `rgb(110,110,110)` → **`rgb(13,13,13)`**、dark `rgb(160,160,173)` → **`rgb(236,236,241)`**。
- ✅ **DF-D2(P1)变更行把行号槽一起染色,DF-5 刚立的竖线一遇 add/del 就断**:`.dl.add`/`.dl.del` 的 background
  (`styles.css:1647/1650`)作用在**整个 grid row** 上,63.1px 行号槽跟着染红绿。轮20 DF-5 特意把折叠条 caret
  做成「行号槽」(白底 + 竖线),结果同一列在折叠条上是白+竖线、在变更行上是整片红绿 —— **那根竖线只要碰到一行
  add/del 就断**。Codex(`codex-crop-diff-rendering.jpg`)只染代码列,竖线从文件头贯到 EOF。
  **轮25 落地 `495b0ba`**:`.dl.add > .dl-no, .dl.del > .dl-no { background: var(--panel) }`(未搬底色到
  `.dl-text`——那会与 `styles.rs.css` signs 模式的 tint 叠成双重染色)。live 复验 `.dl.add .dl-no` 背景
  `rgba(0,0,0,0)` → light `rgb(255,255,255)` / dark `rgb(23,23,26)`,红绿从代码列左沿起。
- ✅ **HM-1(P1)New task 落地不 autofocus**:这一页唯一的工作就是打字,你却得先点一下。实测落地后
  `document.activeElement` = **`BODY`**,盲打 `"hello"` 后 textarea value 仍是 `""` —— 按键全部掉地上。
  `Composer.tsx:1221` 的 textarea 无 `autoFocus`;`⌘⌥N`(`App.tsx:139`,侧栏还挂着徽章)也只 `showPage("home")`
  **不聚焦** —— 官方快捷键把你送到一个还得再点一次的页面。Codex 的 New task 打开即可键入。
  动作:`variant==="home"` 时 autoFocus(或 Home 挂载后 `taRef.current?.focus()`)+ `App.tsx:139` 补 focus;
  注意 modal/popover 打开时别抢焦点。
- ✅ **HM-2(P1)composer 内部没有层级:环境 chip 和 placeholder 灰得几乎一样**:Codex 里 chip
  (`agentrunner`/`New worktree`/`main`)是**满墨 `rgb(0-8)`**、placeholder 是 `rgb(187)`,187 点分离度 ——
  因为 chip 是"这个任务将在哪儿跑"的 **launch 契约**,要你打字前先读到。我们 `.cx-env-control`
  (`styles.css:1769`)= `var(--ink-2)` → `rgb(96,96,96)`,placeholder `--dim` → `rgb(110,110,110)`,**只差 14 点**:
  launch 契约读起来像失效装饰,空输入框看起来像已填。动作:`.cx-env-control` `--ink-2` → `var(--ink)`(图标留
  `--ink-2` 做次级)。✂ **不动 placeholder**:`styles.css:12-13` 注释写明 `--dim` 是刻意从更浅值压到 `#6e6e6e`
  留对比度余量;只加深 chip 单边就能做出分离度,且方向是提高对比度、零 a11y 风险。
- ✅ **HM-3(P2)四个环境 chip 的 hover 反馈约等于不存在**:`.cx-env-control:hover` 给 `var(--panel)` = `#ffffff`,
  但 chip 躺在 env strip 里,strip 底色是 `#f8f8f8`(`styles.composer.css:566`)—— **Δ = 6/255 亮度**。四个可点的
  启动控件,鼠标扫过几乎没有回执,读起来像状态标签而非 popover 触发器。**必须与 HM-2 同批**(chip 一旦常驻满墨,
  这条 hover 只剩那 6 点底色变化,会**更**没反馈)。动作:hover 底色改成比 strip **更深**一档(浅色托盘上 hover
  要往下压不是往上提);dark 同理(strip `#1a1a1e` vs card `#17171a` 只差 3 点,更严重)。
- ✅ **HM-4(P2)「No environment」是个假控件**:`Composer.tsx:1124-1140` 的 popover 里**恰好一个** `PopItem`,
  硬编码 `active`,`onClick={close}` —— 点开只会关掉自己。它在 1440 下占 **145.4px**,是 chip 行 4 个控件里
  **最宽的**(project 79.6 / worktree 132.4 / **env 145.4** / branch 69.9),吃掉 20% 横向预算换回零个选择。
  (造 environments = 新后端,**out-of-scope**;但"渲染一个长得像可交互、点开什么都选不了的下拉"是纯 UI 诚实性
  问题——它在教用户"这排 chip 有的能点有的是骗人的",稀释另外三个**真**控件的可信度。)
  动作:(a) 摘掉整个 chip,把 145px 还给 project/branch;或 (b) 降级成非交互静态文字。倾向 (a) —— 顺带关闭 HM-5。
- ✅ **HM-5(P2)390 下 chip 被永久截断且无法读全**:`New worktree`(显示 101/需 78)与 `No environment`
  (112/90)被切成 `New workt…` / `No environ…`,而 strip `overflow-x: visible` + `nowrap` + 无 tooltip ——
  **既不换行、也不横滚、也没出口**,手机上永久读不全。而 `New worktree` vs `Local` 恰恰最不能猜错:截断后你
  无法确认这任务会不会直接写进你的工作目录。动作:先做 HM-4(a),释放 112px 后 `New worktree` 大概率自动放得下;
  仍不够则给 `.cx-env-strip` 在 ≤520 加 `overflow-x: auto`(`styles.composer.css:573-580` 已有该 media block)。
- ✅ **DF-D3(P2)binary 文件吐假的 `+0 −0`**:`A bin/ar +0 −0 [binary]` —— 二进制没有行,这数是凭空造的。
  `a070dea`(TH-9/TH-13)刚把「卡头不再吐假的 +0 −0」从产出卡删掉,同一条原则文件头没执行。
  动作:`DiffView.tsx:118` —— `badges.includes("binary")` 时不渲染 `.fd-counts`,让 binary 徽标独自说话。
- ✅ **DF-D4(P2)Supervision 面板仍留着 40px 的 "Supervision" 标题条,与顶栏 pill 二次重复**:
  `SupervisionPanel.tsx:212-215` + `styles.panel.css:192-204`,而顶栏的 `Supervision` pill(它本身就是开关)
  就在正上方 54px 处,两者永远同屏、文本一字不差。**RV-1 已为另一条右栏立过完全相同的规矩**:`DiffView.tsx:395-398`
  的注释白纸黑字写着「`.changes-panel-head` 是顶栏 Changes pill 的第二份拷贝,所以它没了,✕ 移到这里」。
  两条右栏坐同一个槽位,一条已拆标题条、一条还留着,规则不自洽。动作:删标题条,✕ 收到第一个分区标签行
  (`Environment`,`SupervisionPanel.tsx:550`)右端 —— 正是 Codex 放 `+` 的位置。
- ✅ **DF-D5(P3)Changes hidden 提示条的第二句永远读不到**:`white-space:nowrap`+`ellipsis`
  (`styles.rs.css:869-873`),657px 栏宽下 `Source files remain visible.` **在任何窗口宽度下都不出现**,
  且 div 无 `title`,截掉的字无从读起。动作:文案缩到一句能放下的,或加 `title`(`DiffView.tsx:796`)。
- ✅ **DF-D6(P3)binary 徽标被 spacer 顶到栏最右,离文件名 ~475px**:读起来像另一列的东西,不像这个文件的属性。
  动作:`DiffView.tsx:124-129` 把 badges 挪到 `.fd-counts` 之后、spacer 之前。
- ✅ **HM-6(P3)建议卡 hover 描边跳到 `--ink-2`**(`5a1d172`:hover 改 `color-mix(--dim 25%, --line)`=#c9c9c9 + `--panel-2` 底色,从「近黑描边把卡弹出来」变成同族深一档的原地微亮)(`styles.home.css:121-124`):从 `--line`(`#e7e7e7`)
  一步跳到 `#606060`,**跨 135 个亮度点** —— 四张软提示卡任一被 hover 都会突然变成整屏对比度最高的物体。
  Codex 的 hover 只轻微加深填充,描边基本不动。**无 Codex hover 态参照图,最后再做。**
- ⊘ **TH-3 需重新取证**:`SupervisionPanel.tsx:199-209` 注释显示空态块折叠(`resting` 分支)**已实现**,
  静息会话上重新量过再决定是否仍开放。
- ✂ home 建议卡几何(归一化后 23.3% vs 23.3%、卡高 82.6 vs 84、cloud mark 40 vs 38px)已收敛,别再动。
- ✂ split 视图无「N unmodified lines」折叠条(`DiffView.tsx:928-929` 明写为有意取舍:成对列模型无 per-row 锚点)。
- ✂ 长行硬裁 / 每文件横向滚动(DF-4 的 Wrap 开关是明确决定)。

- 2026-07-12 轮25(headless):比对 **thread/终态 + home + Changes/diff + sidebar** 对金标
  (`codex-task-thread.jpg` / `codex-new-task-home.jpg` + `codex-crop-{composer,model-dropdown,add-menu,newtask-emptystate}.jpg` /
  `codex-diff-review.jpg` + `codex-crop-{diff-header,diff-rendering}.jpg` / `codex-crop-sidebar-projects.jpg`);
  before 存 `qa/runs/2026-07-12-r25/before/`。**关闭 5 条可见差距(3 条 P0/P1)**:
  **SB-6**(侧栏 Projects 树**是倒的** → 正立:`.project-heading span` x **57 → 34** = 与其子任务
  `.project-task-title` **同列**;caret+folder 绝对定位进左 gutter;组名 **400/`rgb(96,96,96)` → 600/`rgb(13,13,13)`**
  成为锚而非同辈)、**SC-15**(Scheduled 选中 tab 补回浅灰 pill 底 7%,border/shadow 仍为 0,RS-3 实质保留)、
  **TH-11**(终态横幅落回正文边线:`.terminal-alert` x **282 w796 h85 → 350 w660 h49**、`.gbar` x **320 w720 → 350 w660**
  = 与 composer `.cx-card` **完全同轴**;终态 chrome 总高 **127 → 91px**;QA-45 的诚实 banner 一个没删,只对齐+压扁)、
  **TH-12**(一个终态只说一次:goal paused/cancelled/limit 的 echo chip 在横幅真的渲染时才抑制,**重复 chip 3 → 0**;
  fallback goal pill **494 → 294px**;interrupt chip 不抑制)、**HM-P0 + DF-D1 + DF-D2**(dark composer 描边
  `rgb(222,222,222)` → `rgb(42,42,48)` 不再套白光圈 / diff 文件名 `rgb(110,110,110)` → `rgb(13,13,13)` 回黑 /
  变更行不再染行号槽,DF-5 的竖线贯通)。
  派工 **3 个 implementer**(worktree 隔离、白名单两两互斥:B=Sidebar.tsx+styles.nav.css+styles.scheduled.css ·
  A=styles.css+styles.panel.css+timeline.ts+SessionView.tsx · C=styles.css)+ **2 个 read-only finder**(home/composer、
  Changes/diff)。push=`9634450` + `3fe7bd7` + `495b0ba`(+ 台账 `014bc37`);live=`index-4og8sgg5.js`。
  复验:home/thread/diff × light/dark 稳态 console error+warning = **0**;vitest 28 files / **329 tests** 全绿。
  截图 `qa/runs/2026-07-12-r25/{before,after,finder-home,finder-diff}/`。
  BACKLOG:**+✅ 5 条**、新增 ☐ **9 条**(HM-1..HM-6、DF-D3..DF-D6)。**下轮首选**:HM-1(New task 不 autofocus,
  一眼可见)+ **HM-2 + HM-3 同批**(composer 层级:chip 满墨 + hover 压深)+ **HM-4**(摘掉假的 No environment chip,
  顺带关 HM-5 的 390 截断)+ **DF-D4**(Supervision 标题条,RV-1 的规矩没自洽执行)。

## 轮26 新弹药(2026-07-12,2 个 read-only finder:Scheduled 屏 / thread 屏)

**Scheduled 屏**(截图 `qa/runs/2026-07-12-r26/finder-scheduled/`;已确认 parity、勿动:H1 28px、行距 68px、
标题左缘 36px、选中 tab 灰 pill、列表底 hairline、Suggestions 结构):

- ✅ **SC-16(P0)限额终态被画成「坏了」,还被算进 Active**:实测 **3/3 行**全是琥珀 `WarningCircle` + 琥珀文案。
  `pill.ts:24-26` 把 `max_iterations` / `max_generation_steps` / `budget` 全映射成 `cls:"stranded"`,`Scheduled.tsx:93`
  的 `ALERT_STATUS={crash,stranded}` 于是把「跑满配置的 N 次迭代、正常收工」和「驱动器崩了、等你救」画成同一片像素;
  `Scheduled.tsx:103` 的 `LIVE_STATUS` 也含 `stranded` → 实测 **All=3 / Active=3 / Finished=0(空态)**,两个永远不会
  再 fire 的系列被登记为"活着"。Codex(`codex-crop-scheduled-list.jpg`)全屏 **0 个告警色**,唯一强调是右端蓝 unread dot。
  告警色是稀缺资源,3/3 都在喊 = 没人喊;这屏唯一存在的理由(「我的后台还活着吗」)被自己的配色淹了。
  动作:新增 `LIMIT_STATUS`(判 raw status 含 `max_iterations|max_generation_steps|budget|limit_exceeded`,
  `:228`/`:255` 拿得到原文)→ 不进 `ALERT_STATUS`、不进 `LIVE_STATUS`、走 `SETTLED_STATUS`(`:85`)空槽 glyph,
  副行回落 `Ran 2d ago`。**别改 `friendlyStatus` 的 cls**(SessionView terminal banner 依赖它)。
  touches `Scheduled.tsx` + `Scheduled.list.test.tsx`。
- ✅ **SC-17(P1)行菜单里没有一个动作能处置这个 schedule**:⋯ 菜单 = Pin/Rename/Mark as unread/Archive/Copy ——
  **全是"整理",零个"处置"**。那一行白纸黑字写着 `Needs recovery`,而 `AR.resume`(`SessionView.tsx:532`)+ 后端
  `POST /api/sessions/{sid}/resume`(`webui/api.go:68`)**已存在**;retry(`:69`)/close(`:73`)/stop(`:74`)同理。
  长期后台工作的 hub 是只读的——诊断问题却要你点进任务才能解决。动作:`Scheduled.tsx:546-559` session 分支补
  Resume(仅 stranded/crash)/ Retry / Stop(仅 running)/ Close(非终态),照抄 `SessionView.tsx:524-566` 的 `act`。
  ✂ out-of-scope:Codex 的 pause / run-now / delete-schedule 需 daemon suspend/trigger/delete 接口,**后端没有**。
- ✅ **SC-18(P1)Suggestions 上写的 cadence 是装饰,点了不按那个节律排**:`Scheduled.tsx:579` 只传 task →
  `Modals.tsx:382-384` 用 preset 默认 `interval=5m` 开窗。**点「Daily brief · Weekdays at 8:00 AM」建出来的是
  每 5 分钟跑一次的任务** —— 卡片上的节律和产物的节律不一致,屏幕在撒谎。后端已支持(`webui/schedule.go` 的
  `cadenceOf` 能把 cron 渲染回人话,`schedule_test.go:72` 已有 `Saturdays at 4:00 AM` 用例)。动作:`store.ts:10`
  modal payload 加 `schedule?/interval?/cron?`;`SUGGESTIONS`(`Scheduled.tsx:33-55`)每条带 cron。
  **轮28 落地 `71a8d6b`**:`SUGGESTIONS` 每条带真 `CadenceSpec`(`{schedule,cron}`,就是 driver-spec 字段本身),
  **卡面人话由 `cadenceText(spec)` 渲染** → 单一事实源(改 cron 卡面跟着变,测试钉死);modal payload 加
  `cadence?`,`runFormDefaults(preset, cadence)` 拿它预填;`runPreset.ts` 新增 `cadenceText`/`cronPhrase` 作为
  `webui/schedule.go` `cadenceOf`/`cronPhrase` 的前端镜像(用例直接搬 `schedule_test.go` 钉住两边不漂移);
  modal 里加一行 cadence 回执(`0 8 * * 1-5` 没人校得动,「Weekdays at 8:00 AM」才校得动)。**后端一行未改。**
  live 复验三张卡:Daily brief `interval/5m` → **`cron` `0 8 * * 1-5`**、Weekly review → `0 16 * * 5`、
  Follow-up monitor → `0 */6 * * *`;稳态 console 0。
- ✅ **SC-19(P3)搜索框与状态 ring 是这屏仅剩的密度偏差**:搜索 pill **32px** vs 金标 ~27px
  (`styles.scheduled.css:277` padding `6px 14px`→`4px 14px`);行首 ring **16.0px** vs 金标 13.5px
  (`Scheduled.tsx:464-469` icon `size={20}`→`17`)。
- ✅ **SC-20(P1,部分 ✂)Create 按钮是全屏对比度最高的元素**(轮29 `d9031c1`:实心黑药丸 → ghost。live 实测 light bg `rgb(13,13,13)` → `rgba(0,0,0,0)`、color `rgb(255,255,255)` → `rgb(96,96,96)`;dark bg `rgb(236,236,241)` → 透明、color → `rgb(180,180,192)`;hover/菜单打开才给 6% 浅底 + `--line` 细边。**菜单内容(QA-45 `46345d0`)一字未动**):`.scheduled-create .menu-trigger`
  (`styles.css:3605-3618`)是 112×33 实心 `#0d0d0d` 药丸,坐在 640px 阅读列内;Codex 的 `Create ⌄` 是右上角
  无边框灰字,视觉重量近零。✂ 菜单内容是 QA-45 刻意设计(`46345d0`)**不动**;可关的只是把 fill 降级为 ghost/outline。

**thread 屏**(截图 `qa/runs/2026-07-12-r26/finder-thread/`;已确认 parity、勿动:变更卡左边线与正文/产出物同列
x=350、`Undo ↺`+`Review` 组合、文件路径「目录灰+文件名黑粗」+ 右齐 `+x −y`、产出物卡 `Document · MD`+`Open in ⌄`、
composer placeholder 逐字一致。✂ 已排除:👍/👎 无后端、`· N new` TH-13 刻意、per-turn 变更卡需新后端、
`Worked · N steps` 刻意):

- ✅ **TR-1(P0)turn 之间没有任何分隔——Codex 有一条贯穿正文列的 1px 细线**:金标像素扫描证实每个 turn 收尾
  (变更卡 → 动作条 → `Worked for …›`)后有一条 **1px 全列细线**(y=456 灰度均值 248.0/std 0.12,x 从 320 贯到 639
  = 整个 320px 正文列,比卡片还宽)。我们 `styles.css`/`styles.conv.css` 全文**无任何 turn 级分隔规则**,
  `Timeline.tsx:947-949` 只过滤掉 `kind==="turn"` 标记从不画线 —— 86 轮的会话读不出"这一轮结束了"。
  长 thread 里 turn 是唯一的导航单位。动作:每个非首个 `kind==="user"` 节点前插 `.turn-sep`
  (`height:1px; background:var(--line); margin:22px 0 18px`)。touches `Timeline.tsx`(~1038-1064)+ `styles.conv.css`。
- ✅ **TR-2(P1)消息时间戳只有时分,跨天完全读不出来**:`Timeline.tsx:66-71` `shortTime()` 只出 `11:31 PM`;
  实测富会话同屏出现 `11:31 PM` 与 `12:40 AM` = **跨午夜两天**,UI 毫无提示;diff 会话的三个时间戳实际跨 2 天。
  Codex 是 `Friday 10:14 PM`(星期+时间)。agent 任务动辄跑几小时到几天,时间戳的唯一价值就是定位"哪天的"。
  动作:加日期档位(今天→时分;7 天内→`Friday 10:14 PM`;更早→`Jul 3, 10:14 PM`),约 8 行。
- ✅ **TR-3(P1)变更卡的字号层级是【倒的】:文件行比卡片标题还大**:`.changes-outcome-title b` = **13px**/700,
  文件行 = **14px**,行尾 `small` = 11.67px → 标题 = 行的 **0.93×**。Codex 金标同阈值量测:标题 ≈ 行的 **1.05×**
  且加粗,`+7 −0` 与路径同号。卡片是**摘要**(TH-8 已定的方向),第一眼必须是"改了 5 个文件 +2 −179"。
  动作:`styles.css:2954` 13→15px、`:2968` 14→13px、`small` 11.67→13px。纯 CSS。
- ✅ **TR-4(P2)"Show N more files" 是颗错位的小按钮,且 N=1 时文案是 "Show 1 more file**s**"**:实测 rect
  x=361/w=140/h=29 —— 文字比文件路径右偏 **7px**、行高比文件行矮 **9px**、热区只有 140px 而非整行;
  `ChangesOutcome.tsx:438` 写死 `more files` → 4 文件时渲染出 **"Show 1 more files"**(可见语法错)。
  Codex 里它是卡内**同节拍的又一行**(文字左起 x=35 与路径 x=34 像素级对齐,行距与文件行同为 72 crop-px)。
  动作:`styles.css:2971` 改整行 flex(`width:100%; min-height:38px; padding:7px 14px`)+ 复数化。
- ✅ **TR-5(P2)user 消息的动作条在手机上【根本不存在】**:`styles.css:2765-2773` `opacity:0`+hover→1,
  `styles.conv.css:769-775` 再给 `.msg:not(.assistant)` 加 `pointer-events:none`。实测 1440 user 动作条
  `opacity:0` / assistant `1` → **390 无 hover 的设备上,user 消息既没 Copy、没任务链接、也没时间戳**。
  这正是 **TH-10 自己写下的理由**("hover-only icons do not exist AT ALL on touch"),但只落到 assistant 一侧;
  用户最常想复制的恰恰是自己发过的 prompt。动作:user 侧静止态改成与 assistant 同构(`opacity:1` + 图标 `.5`)。
- ✅ **TR-6(P3)没有内容的 `Worked for 2s` 死行 + 动作条图标间距比 Codex 松 40%**:富会话里 3 条
  `Worked for 2s/1s/3s` 实测 `disabled:true`(`fold.children.length===0`)—— 没 caret、点不开、点开也没东西,
  纯占位噪音。另 `.msg-copy.icon-only` 26px + `gap:4px` = 图标节拍 **30px**,Codex 约 **21.5px**。
  动作:`WorkedFold`(`Timeline.tsx:685-728`)空 fold `return null`;`styles.css:2788` width 26→22、gap 4→2。
- ✅ **DF-D7(P2)binary 文件仍会去 fetch `/blob` 并吃一条 400**(轮29 `45ea4af`:`api.ts` 新增 `isBinaryPath()`(只收「永远是字节」的扩展名,`.svg/.json/.map` 照常取)+ `AR.blob` 本地短路;live diff 屏 console **1×400 → 0**,binary 预取请求 **2 → 0**):轮26 复验 Changes 屏见 1 条
  `400 (Bad Request)` = `/blob?path=…asset.bin` → `{"error":"file is not text"}`。JS 侧已静默 catch(卡片显示
  "Content isn't shown…"),但 console 不干净、也白跑一次请求。动作:`DiffView` 在 `badges.includes("binary")`
  时**根本不发** blob 请求。touches `DiffView.tsx`。
- ✅ **(清理)`styles.css:3915-3936` 的 `.supervision-head` 规则已成死代码**(轮26 `74b9dbf` 删了 JSX,
  但该文件当时被并发 implementer 占用)。下轮顺手删。

- 2026-07-12 轮26:比对 **home / thread / Changes split / Scheduled** 4 屏 × light/dark(before 存
  `qa/runs/2026-07-12-r26/before/`)。**关闭 9 条可见差距(2 条 P1 + 5 条 P2 + 2 条 P3)**,派 **3 个 implementer**
  (worktree 隔离、白名单两两互斥:A=Composer.tsx+Home.tsx+App.tsx · B=styles.css · C=SupervisionPanel.tsx+
  DiffView.tsx+styles.panel.css+styles.rs.css)+ **2 个 read-only finder**(Scheduled / thread),**落一个推一个**。
  **HM-1**:New task 落地即可键入 —— `document.activeElement` **BODY → TEXTAREA**,不点击直接盲打 `"hello"`
  真的进了输入框(改前按键全部掉地上);⌘⌥N 从任意页切到 home 也把焦点送进 textarea。
  **HM-4 + HM-5**:摘掉「No environment」假 chip(popover 里恰好一个硬编码 active、onClick=close 的 PopItem ——
  点开只能关掉自己,零个可选项,却在 1440 下吃掉 **145.4px** = chip 行 20% 横向预算)。env chip **4 → 3**;
  释放的宽度让 **390 档 `New worktree` 从 `New workt…` 变回完整可读**(78/78,零截断)。
  **HM-2 + HM-3**:composer 环境 chip 成为真正的 launch 契约 —— color `rgb(96,96,96)` → **`rgb(13,13,13)`** 满墨
  (dark `rgb(180,180,192)` → `rgb(236,236,241)`),与 placeholder 的分离度 **14 → 97**(dark 19.7 → 73.3),图标
  留 `--ink-2` 作次级;hover 底色从 `--panel`(比 strip 还浅,Δ 仅 **6.6**)改成 `--line` → Δ **17.4**(dark 3.6 → 16.2),
  `.active` 再压一档 —— 四个可点的启动控件终于有了回执。**没动 placeholder / `--dim`**(styles.css:12-13 刻意值)。
  **DF-D4**:Supervision 面板不再抄顶栏 pill —— 删掉 40px 的 `Supervision` 标题条(它与正上方 54px 处的
  Supervision pill 文本一字不差),✕ 落到 `Environment` 标签行右端(Codex 放 `+` 的位置)。Environment 首行
  y **108 → 68**(−40px);两条右栏终于同轴(Supervision 首行 68 vs Changes scope 63)。**RV-1 早为 Changes 栏
  立过同一条规矩,这次让规则自洽了。**
  **DF-D3 + DF-D6**:binary 文件不再吐凭空造的 `+0 −0`(二进制没有行);`[binary]` 徽标从被 spacer 顶到栏最右
  (离文件名 **376px**)收回到文件名旁 **8px** —— 读作 `A qa-inc41-d4/asset.bin [binary]`。
  **DF-D5**:Changes hidden 提示条第二句不再被 `nowrap`+`ellipsis` 永久吃掉(scrollWidth 492>366 → **144=144**)+ 补 `title`。
  push:`d8f070b`(A)、`8b15fe6`(B)、`74b9dbf`(C)+ 本 commit(台账);live=`index-fhwDUPox.js`。
  复验:home/thread(Supervision)/Changes × light/dark 稳态 console **error+warning = 0**(Changes 屏 1 条 400 是
  C 造的 binary fixture 触发的 `file is not text` 拒绝路径 → 已登记 **DF-D7**);vitest **334 tests 全绿**。
  BACKLOG:**+✅ 9 条**、新增 ☐ **9 条**(SC-16..SC-20、TR-1..TR-6、DF-D7 + 1 条清理)。
  **下轮首选**:**SC-16(P0)**(Scheduled 3/3 行全琥珀告警 + 终态被算进 Active,一眼可见)+ **TR-1(P0)**
  (thread 没有 turn 分隔线)+ **TR-3**(变更卡字号倒挂)——三者 touches 互不重叠,可同批并发。

- 2026-07-12 03:5x 轮27(headless)**9 条 ✅(2 条 P0)— Scheduled 停止把「正常收工」画成「坏了」+ thread 有了 turn 分隔线**。
  第一步比对:live 8809(`index-fhwDUPox.js`)截 thread / Scheduled × light/dark(before 存
  `qa/runs/2026-07-12-r27/before/`)对 `codex-crop-scheduled-list.jpg` + `codex-task-thread.jpg`。
  并发派 **3 个 implementer**(worktree 隔离,白名单两两互斥:A=`Scheduled.tsx`+`styles.scheduled.css` ·
  B=`Timeline.tsx`+`styles.conv.css` · C=`styles.css`+`ChangesOutcome.tsx`),落一个推一个。
  关闭的可见差距:
  **SC-16(P0)** — Scheduled **3/3 行全琥珀告警** → **1/3**:新增 `isLimitStatus()`(判 raw status 原文
  `max_iterations|max_generation_steps|max_tokens|limit_exceeded|budget|step/token limit`),限额终态不进
  `ALERT_STATUS`(不涂琥珀/不配 WarningCircle)、不进 `LIVE_STATUS`(不算 Active),走 settled 空槽 glyph +
  中性副行 `Ran 3d ago`。tab 计数 **All=3/Active=3/Finished=0(空态)** → **All=3/Active=1/Finished=2**。
  `pill.ts` 的 `friendlyStatus` cls 一字未动(SessionView terminal banner 依赖)。
  **SC-17(P1)** — 行 ⋯ 菜单从「全是整理、零个处置」补上 **Resume**(仅真 stranded/crash)/ **Retry** /
  **Stop**(仅 running)/ **Close…**(带 confirm),照抄 `SessionView.tsx` 的 `act`;后端 `AR.resume` 等本就存在。
  pause/run-now/delete-schedule 后端无接口 → ✂ 不做壳。
  **SC-19(P3)** — 搜索 pill 32→**28px**、状态 ring 20→**17px**(金标 ~27/13.5)。
  **TR-1(P0)** — thread **`.turn-sep` = 0 → 9**(10 个 user turn):线挂在每个非首个 user 消息头上、
  作 `.tl-inner` 直接子元素继承正文列 content box → x=**350** w=**660** h=1,与 `.msg-col` 严丝合缝;
  色走 `--line`(light `#e7e7e7` / dark `#2a2a30`),首条 user 之上无线。86 轮的会话终于读得出"这一轮结束了"。
  **TR-2(P1)** — 时间戳加日期档位:今天→`10:14 PM`、1–6 天→`Friday 10:14 PM`、7 天+→`Jul 3, 10:14 PM`
  (日差按**本地日历日**算,否则"昨天 11:50 PM"会被 24h 制判成今天 —— 正是要修的跨午夜 bug 本身)。
  富会话跨午夜的 `11:31 PM → 12:40 AM` 现在读作 `Saturday 11:31 PM → 12:40 AM`。
  **TR-3(P1)** — 变更卡字号层级**从倒的扶正**:标题/文件行 13/14px(比 **0.93**)→ **15/13px**(比 **1.15**),
  行尾 `+x −y` 由 UA 默认 11.67px 提到与路径同 13px。卡片是摘要,第一眼必须是"改了 5 个文件 +2 −179"。
  **TR-4(P2)** — `Show N more files` 从错位小按钮(x=361/w=140/h=29,文字比路径右偏 7px)→ **卡内同节拍整行**
  (x=351/w=658/h=38,文字 x=365 = 路径 x=365);并修掉 N=1 时的 `Show 1 more file**s**` 语法错(+2 单测)。
  **TR-5(P2)** — user 消息动作条在触屏上**根本不存在**(`opacity:0`+hover 门 + `pointer-events:none`)→
  静息 `opacity:1`、`pointer-events:auto`;1440/390 下 `elementFromPoint` 均命中 Copy(无需 hover),
  行高逐像素不变(63.69px)。用户最常想复制的恰恰是自己发过的 prompt。
  **TR-6(P3)** — 空 `Worked for 2s` 死行(`fold.children.length===0`,没 caret 点不开)**2 → 0**;
  动作条图标节拍 30 → **24px**(金标 ~21.5)。顺手删掉 `styles.css` 里 `.supervision-head` 死代码。
  push=`e3d4d7c`(TR-1/2/6a)+ `b6d7054`(TR-3/4/5/6b)+ `00223d7`(SC-16/17/19);
  live=`index-BrQZq43D.js`;vitest **361 passed / 30 files**;复验 thread+Scheduled × light/dark × 1440/390
  稳态 console error+warning = **0**;截图 `qa/runs/2026-07-12-r27/{before,after}/`。
  **下轮首选**:TH-12(同一事实一屏说 3–5 遍)、TH-11(终态 chrome 138px + 左边线三级台阶)、
  SC-18(Suggestions cadence 是装饰,点了按 5m 排 —— 屏幕在撒谎)、DF-D7(binary 白吃一条 400)、
  清理 `styles.conv.css:769-775` 那条已被特异性对冲掉的 `pointer-events:none`。

## R 组 · 轮28 新弹药(2026-07-12)

- ☐ **PROC-1(P0,流程)台账与现实脱节:代码进了 main,BACKLOG 还标 ☐、live 还跑旧二进制**。轮28 派出的
  3 个 implementer 里有 **2 个(TH-11/TH-12、DF-5/DF-6)发现自己要做的活上一轮已经写完并推上 main**
  (`3fe7bd7`、`5d88d16`),但:(a) BACKLOG 里仍是 ☐,(b) live 8809 的二进制没重新部署 → **本轮开轮拍的
  "before" 截图拍的是旧界面**,比对失真,2/3 的算力花在补验补部署而不是关新差距。
  根因:上一轮跑到 55min 硬顶被截断,collect 阶段(打 ✅ + 部署 + 台账)没跑完。
  动作:轮末**先部署 live 再打 ✅**;开轮第一步在截 live 之前先 `curl 8809 | grep index-*.js` 与
  `git log --oneline -1` 对账,bundle hash 对不上就**先重建部署再截图**(否则整轮比对建立在幻觉上)。
- ☐ **INF-1(P2)vitest 在高负载下偶发 21 条红**:`localStorage.clear is not a function`,命中
  `Composer.effort` / `DiffView` / `Sidebar.nav` / `loadingStates` —— 多 jsdom 文件下 worker 复用 /
  全局 `localStorage` 被污染,对文件调度顺序极敏感(3 个 implementer 抢 CPU、load 12–17 时复现;
  空闲机器上同一棵树 **392/392 全绿**)。不是功能 bug,但会让并发轮的闸门读数不可信。
  动作:`vite.config.ts` 给 jsdom 环境固定 `environmentOptions` / 关 worker 复用,或在 setup 里
  重置 `globalThis.localStorage`。**跨共享 config,需单独一轮独占。**
- ◐ **DF-D8(P2)blob 预取对 binary/超大文件必然吃 400/413**(轮29 `45ea4af` **关掉一半**:binary 靠扩展名短路 = 零请求;`blobRefused` 记住 400/413 → **拒绝问第二遍**(重挂不重发)。**剩下的一半**:`/diff` 只给 `untracked: string[]`,前端**拿不到 size** → 413 无法预判;无扩展名的大二进制(`bin/ar`)首次仍吃 1 条。要彻底关须让 `webui/meta.go` 在 untracked 项上带 size/mime —— **跨前后端,须先写三层 delta**):`AR.blob` 对 untracked 文件做预取,
  碰到 binary(`asset.bin` → 400)和超大 blob(`dist/assets/index-*.js` → 413)必然失败。代码里
  **刻意 catch 掉**(卡片照常显示 "Content isn't shown"),但浏览器网络层仍记一条 console error
  (d8ac 1×400、5849 2×413)—— 稳态 console 0 的闸门因此在 Changes 屏读不干净。
  动作:预取前按 size/mime 短路(需动 `webui/meta.go` 或 `api.ts`)。与 DF-D7 同源,可并做。

- 2026-07-12 04:30 轮28:比对 4 屏(thread/diff/scheduled/home × light/dark)、关差距 **TH-11 + TH-12
  (终态区 138px → 93px,两条横幅落回正文边线 x=350 w=660 与 composer 同列;主列 "Goal cancelled" 3 → 1)
  + SC-18 + SC-5(Suggestion 卡片点什么就建什么:`interval/5m` → 真 cron `0 8 * * 1-5`,卡面人话与 spec
  同源)+ DF-5 + DF-6(折叠条落到代码栅格,误差 0.00px;toolbar `+1` → `+1 −0`)**、派工 3(并发,worktree
  隔离,白名单两两无交集)、push `e9e6145` + `71a8d6b`(+ 复验已在 main 的 `3fe7bd7` / `5d88d16`)、
  live=`index-DHsbKj6d.js`、vitest **392 passed / 31 files**、稳态 console 0、
  截图 `qa/runs/2026-07-12-r28/{before,after,before-df56,after-df56}/`。
  **本轮真实产出打折**:2/3 implementer 撞上「活已经干完但台账/live 没同步」(见 PROC-1)。
  **下轮首选**:TH-10(助手收尾 action row 静息不可见)、SC-12(Scheduled 行只读)、SB-7(appr 与
  stranded 同色不可分)、DF-D7/DF-D8(binary 白吃 400/413),开轮**先对账 bundle hash 再截图**。

## 轮29 新弹药(2026-07-12,3 个 read-only finder:Changes/Review 分栏 · Environment/Supervision 右栏 · sidebar)

**Changes/Review 分栏**(截图 `qa/runs/2026-07-12-r29/finder-review/`;已确认 parity、勿动:toolbar 形状、
`M↓ path +N −M` 文件头、unified 栅格、绿实心/红斜纹 channel bar、`N unmodified lines` 折叠条与 caret 方向、
空态、`Commit or push`。✂ 已排除刻意:split 无折叠条、长行不硬裁+Wrap、无 Changes 标题条(RV-1)、
DF-D5 生成文件横幅、DF-2/RD-1 默认折叠、DF-1 Expand-all 收进 `…`、DF-D3 binary 不吐 `+0 −0`、RV-3 caret):

- ✅ **RVW-1(P0)变更行的行号槽被我们刷白,金标是整行连续染色 —— 且 DF-D2 的立论经金标采样【是错的】**:
  金标像素采样(`gold-mid.png`)add 行号槽 `rgb(201,227,198)`、del `rgb(248,195,189)`、context `rgb(218,218,218)`
  —— **Codex 把红绿从 channel bar 一直刷到面板右缘,包含行号格**。而 `styles.css:1661-1662`(DF-D2,轮25 落地)
  写着「Codex 只染代码列、行号槽留中性面」并把 `.dl.add > .dl-no` 刷回 `var(--panel)` 纯白 —— **前提与金标不符**。
  后果:176 行的删除块,每一行都被 63px 白槽从中间劈断,读不成一条连续带。
  动作:删 `styles.css:1661-1662`(保留 1663-1664 数字的红绿墨色,金标也有);DF-D2 当初怕的"折叠条行号槽被染成
  色块"已由 `styles.diff.css:94-102` 的 `.fd-gap-caret{background:var(--panel)}` 独立兜住。
  ⚠ **这是对一条已落地刻意决策的翻案**,须先复核金标采样再动手。
  ✅ **轮30 `1520b48` 关闭**:删掉 styles.css 刷白行号槽的两条规则(DF-D2 翻案,金标采样为凭);实测 `.dl-no` 背景转 transparent → 整行连续染色,数字对比度 del 6.4:1 / add 7.7:1,折叠条仍中性面。
- ✅ **RVW-2(P1)split 视图硬裁长行:没有滚动条、没有省略号,内容【静默消失】**:`styles.rs.css:174-184`
  两半都是 `1fr` → grid 永远不超出容器 → `.fd-body{overflow-x:auto}` **永不出滚动条**;1440 下每半只剩 ~370px 代码,
  `var Xu=Object.defineProperty;var ed=` 直接断在词中间。**inline 视图同一行滚得好好的** —— 同一份 review,
  换个 toggle 就吃掉你的内容,且与本项目「长行不硬裁,用 Wrap 开关」的既定决策自相矛盾。
  动作:`.diffwrap .dls-half { overflow-x: auto }`(每半独立滚),或 `.dls` 两半改 `minmax(0,max-content)` 走
  `.fd-body` 已有的共享滚动条。渲染点 `DiffView.tsx:1099-1126`。
  ✅ **轮30 `1520b48` 关闭**:`.fd-split` 改共享 grid + 代码列 `minmax(max-content,1fr)`;maxScroll 0 → 1,901,162,263k 字符的 minified 行能滚到结尾;Wrap 打开时列回落 `minmax(0,1fr)` 正常换行。
- ✅ **RVW-3(P2)diff 没有任何「复制」出口**(`9d527b6`:`.diffbar` 加 `<Copy size={15}/>` ghost,写 `data.diff` 进剪贴板 + `diff copied` toast;实测拿到 301 字节真 unified diff。逐文件复制未做——`fd-head` 现在 sticky 且点击即 toggle,塞按钮会喧宾夺主):金标 `codex-crop-diff-header.jpg` review 头栏有 copy 图标;
  `DiffView.tsx` 全文零 copy handler(diffbar / `…` 菜单 / 逐文件都没有),而**同屏对话里每个代码块都有 Copy**。
  读完 diff 最常做的事就是把它贴进 issue/消息,今天唯一的路是在虚拟化栅格上拖选。
  动作:`.diffbar` 加 `<Copy size={15}/>` ghost 按钮,写 `data.diff` 进剪贴板 + 复用现有 toast。
- ✅ **RVW-4(P2)面板默认 scope 是 `working-tree`,金标默认 `Last Turn`**(`9d527b6`:默认 `last-turn`,`available===false` 且用户没显式选过 → 静默回落 `working-tree`(不落盘);显式切换持久化到 `ar.diff.scope`(全 try/catch);实测面板 `+2 −2` 与 thread 变更卡 `Edited 2 files +2 −2` 终于是同一份):`DiffView.tsx:159`
  `useState<DiffScope>("working-tree")` —— 无 INC/QA 出处,`git log -S` 查不到决策记录。症状:thread 变更卡说
  `Edited 5 files +2 −179`,点它的 `Review` 打开的面板可能 scope 到别的东西 —— **两个界面对同一次点击给出两份 diff**。
  动作:默认 `"last-turn"`,`data.available===false` 时静默回落 `working-tree`(回落分支 `:454-465` 已存在),
  用户显式切换后持久化。
- ✅ **RVW-5(P2)文件头随滚动消失,深入一个文件后不知道自己在哪个文件**:实测 `scrollTop=1500` 时前三个 `.fd-head`
  在 y = −1369/−1295/−1221,视口里 40 行 minified JS **屏幕上没有任何文件名**;`.fd-head` 是 `position:static`,
  而 `.diffbar` 已经是 `sticky top:0`(46px)。诚实标注:金标截于 scrollTop 0,无法证明 Codex 钉头 —— 但能证明我们
  滚起来就丢。动作:`.diffwrap .filediff > summary.fd-head { position:sticky; top:46px; z-index:3; background:var(--panel) }`。
  ✅ **轮30 `1520b48` 关闭**:`summary.fd-head` sticky top:46px(在 diffbar 之下),scrollTop=1500 时文件头钉在 y=100。
- ✅ **RVW-6(P3)载态是一句灰字,而 app 其余地方都用 skeleton**(`9d527b6`:`DiffSkeleton` = 3 张文件卡(头 glyph/path/counts + 12 行行号栅格),复用 `sidebar-shimmer`,`prefers-reduced-motion` 下停动画):`DiffView.tsx:452` `Loading changes…`;
  而 `.tl-skeleton`(styles.css:4844)、侧栏 skeleton(styles.nav.css:259)、甚至 thread 里那张 40px 的变更卡都画
  skeleton bar —— **摘要卡载得比它链去的 658px 面板更体面**。动作:换成 3–4 条文件头形 + ~12 行栅格 skeleton。

**Environment / Supervision 右栏**(截图 `qa/runs/2026-07-12-r29/finder-env/`;金标换算:面板 ≈236pt 宽 =16% 窗口、
行距 ~24pt、内容自适应高。✂ 已排除刻意:无 Supervision 标题条(DF-D4/RV-1)、满高贴边无卡片圆角(CX-1 `5d8826a`);
**不建议**造 Background processes / Browser / Sources 壳 —— 无数据源):

- ✅ **ENV-1(P1)四条 Environment 行【没有主文本】:label 与 value 同一档灰**(`605ad95`:`.env-row-label{color:var(--ink);font-weight:450}` + `.env-row{color:var(--dim)}`;实测 label 96→**13**、value 留 110;额外发现 disabled 行拿 `--ink` 后 `opacity:.55` 会落到 129 **比启用行还黑**,故 disabled label 单独降回 `--dim` → 实测 175,对上金标 180):金标 label 近黑(采样 92–117)、
  value 灰、**只有失效行才整行转灰**(180),是 ink → dim → header 三档。我们 `styles.nav.css:433-446`
  `.env-row{color:var(--ink-2)}` → label `rgb(96,96,96)` vs value `rgb(110,110,110)`,**只差 14 级** = 纯平;
  只有 hover(nav.css:475)或 `.active`(panel.css:601)才够到墨色。这四条 git 动作是面板的全部载荷,却以 metadata 灰呈现。
  动作:`.env-row-label{color:var(--ink);font-weight:450}` + `.env-row{color:var(--dim)}`(value 继承 dim),
  `.active`/`:hover`/`:disabled` 语义不变。
- ✅ **ENV-2(P1)每行 211px 的死沟:我们的行读起来像电子表格,金标像菜单**(`605ad95`:去 `.env-row-label{flex:1}`、`.env-row-val{margin-left:auto}`、右轨 344→**288**(≤1180 断点 304→264,不然越窄越宽);label↔value 实测 **211.3px → 16.2px**;label `flex:0 0 auto` / value `flex:0 1 auto;min-width:0`,免得窄轨下「Worktree」这个主文本先被截成 `Workt…`;Changes split 走另一条轨,实测仍 658.5px 未受影响):`.session-layout` 右轨
  `344px`(`styles.css:3796`)= 窗口 **23.9%**(金标 ~16%),且 `.env-row-label{flex:1}`(nav.css:451)把 51.5px 的
  「Changes」撑成 **253.9px** 的盒子 → label 与 value 之间实测 **211.3px 空白**(`+1` 被顶到面板右缘 x=1403.9)。
  label 与 value 是一个意思,被 211px 白隔开 → 每行一次扫视跳跃。
  动作:去掉 `.env-row-label` 的 `flex:1`,`.env-row-val` 改 `margin-left:auto`;右轨 `344px → 288px`
  (Changes 面板的轨在 `styles.css:4796`,另一条,不受影响)。
- ✅ **ENV-3(P2)面板右缘是一列否定句**:`SupervisionPanel.tsx:586/636/696` 的 `No changes` / `No branch yet` /
  `Nothing to commit` 右齐堆成一列 12px `--dim`(110)灰字,**和动作本身一样响**;`Nothing to commit` 更是冗余
  ——那行本来就 `disabled`。动作:删这三个 `.env-row-val`,让 disabled/inert 态自己说话(原因留 `title=`,已有)。
  ✅ **轮30 `232ea9b` 关闭**:三条右齐否定句删除(理由留 `title=`);有内容时 `+N` / worktree 路径照常显示。
- ✅ **ENV-4(P2)`Run details` 是面板里最重的文字,且脱离行栅格**:`styles.panel.css:297-303` 给它
  `font-weight:550 + --ink-2` → 全面板最黑最粗的字,压过所有真数据行;它**没有前导图标**,文字起于 x=1111 而四条
  env-row label 起于 x=1141 —— **30px 参差左缘**。金标的同位元素 `View all` 是**最淡**的一行(202)且带图标、
  与所有行同栅格。另 `Environment` 组标题起于 x=1112 vs 行图标 x=1118(6px 参差)。
  动作:`font-weight:400; color:var(--dim)` + 前导 14px 图标 + `padding-left:21px` 落到 label 列;
  `.supervision-label` 补 `padding-left:6px`。
  ✅ **轮30 `232ea9b` 关闭**:`Run details` 550→400 字重、`--ink-2`→`--dim`,补 14px 前导图标,图标 x=1118 / label x=1141 与 env-row 列 0px 对齐。
- ✅ **ENV-5(P2)右栏 60% 是空白**:rail 高 846px,内容底只到 376–390px → **510–524px 空面** × 344px 宽;
  dark 下 rail 底色 `rgb(23,23,26)` 与 thread 完全相同,那片空白读作"撕掉的一块"而非面板。
  (**不**回退成浮动卡片 —— CX-1 刻意。)动作:(a) `Run details` 下沉为真正的 footer 行
  (`margin-top:auto; border-top:1px solid var(--line)`,aside 改 flex column)→ 空白被"框"在内容与 footer 之间;
  (b) ENV-2 的 288px 收窄再砍掉 ~29% 空面积。
  ✅ **轮30 `232ea9b` 关闭**:aside 改 flex column,`Run details` 下沉为 footer(`margin-top:auto` + 1px 上边线),实测 top=856/bottom=900 贴死底部;未加假内容。
- ☐ **ENV-6(P3)Worktree 抽屉把路径撕成 4 行 mono**:`styles.panel.css:618-626` `overflow-wrap:anywhere` 把
  `…/wt-20260710-143427` 断成 **77.8px 高**的块,断在 `workt|rees`、`webu|i`、`wt-20260710-|143427` 词中间;
  金标里标识符**永远单行 + 尾部省略号**。动作:`white-space:nowrap; overflow:hidden; text-overflow:ellipsis`
  (保尾:`direction:rtl; text-align:left`),全路径已在 `title=`(`SupervisionPanel.tsx:610`)。

**sidebar**(截图 `qa/runs/2026-07-12-r29/finder-sidebar/`;已确认达标、勿动:SB-6 父子同列(heading x=34 = task x=34)、
SB-4 段级折叠 + Show more、SB-3 的 28px 行密度(金标 ~25px,3px 内不动)、RH-5 ⌘K 单一搜索入口、390 抽屉;
✂ Plugins/Sites/Chat/PR 无后端):

- ✅ **SB-8(P1)选中行是全 app 唯一的高彩度色块 —— 而同一根 rail 的 nav 已经是中性灰了**:
  `styles.nav.css:504-509` `.project-task-wrap.current{background:var(--rs-accent-soft)}` + 标题 `--rs-accent` 550
  → dark 实测选中底 `rgb(22,40,60)` 蓝、light 蓝底蓝粗字;而 `styles.nav.css:497` 的 `.primary-nav button.active`
  **已经**是中性 `color-mix(--ink 7%)` —— 一根 sidebar 里「选中」讲两种方言。金标采样:选中 pill `rgb(238,238,238)`
  落在 `rgb(251,251,251)` 上,**13 级灰、零色相**,文字仍近黑。且蓝在我们这里已有语义(`--blue` = unread dot),
  选中行现在长得像"未读"。
  **附带真 bug**:`styles.css:3373-3376` 给 `.current` 同时设 `background` 和 `--row-bg: var(--panel-2)`,
  nav.css:504 只覆盖了 `background` → hover 选中行时 `.project-task-wrap::after`(styles.css:3350)按 `--panel-2`
  淡出标题,在蓝 chip 上糊一条灰白渐变(dark 图里标题直接被吃掉)。
  动作:删 nav.css:504-509 的 R2-3 覆盖,回落 `styles.css:3373` 的中性 `--panel-2` 底 + `color:var(--ink); 550`。
  ✅ **轮30 `fce84b0` 关闭**:选中行去蓝 → 中性 `color-mix(--ink 10%)`(层级 rest 0% < hover 7% < selected 10%;实测 light Δ23 灰、R=G 零色相),`background` ≡ `--row-bg` 同源 → hover 渐变吃标题的真 bug 一并修掉。
- ✅ **SB-9(P1)墨色层级是倒的:内容(任务标题)是灰的,脚手架(组名)是漆黑加粗的**:金标采样 —— 子任务
  `rgb(37,37,37)`、组名 `rgb(48,48,48)`、`Projects`/`Show more` `rgb(160,160,160)`:**任务标题是 rail 里最黑的东西,
  组名比它还浅一档**。我们:任务标题 `rgb(96,96,96)`/400(`styles.css:3319` `--ink-2`),组名 `rgb(13,13,13)`/**600**
  (`styles.nav.css:590`)—— 父子墨差 **83 级且父更黑**(金标 11 级、子更黑)。8 个文件夹名在喊,14 个真正可点的
  任务行读起来像 disabled。(不推翻 SB-6:「组名是锚」靠**字重**继续成立。)
  动作:`.project-task-title{color:var(--ink)}`;`.sidebar .project-heading` → `color:var(--ink-2)`(字重 600 保留)。
  ✅ **轮30 `fce84b0` 关闭**:任务标题 `--ink`(light rgb(13,13,13))、组名回落 `--ink-2`(rgb(96,96,96),600 字重保留)→ 墨色层级翻正。
- ✅ **SB-10(P2)顶部 brand 块 64px vs 金标 ~38px**:`Sidebar.tsx:286` `min-h-[64px]` 硬撑(内容只有 30px 的
  `.brand-main`,上下各 ~17px 空白);多出来的 26px 是白送的一整行任务。动作:`min-h-[44px] pt-[6px] pb-[6px]`
  (同步 `styles.css:3165` 的 `.brand`)。
  ✅ **轮30 `fce84b0` 关闭**:brand 块 64px → 44px。
- ✅ **SB-11(P2)nav 块与列表打两个节拍(32 vs 28px),金标全 rail 一个节拍(~25px)**:
  `.primary-nav button{height:32px}`(`styles.css:3221`)+ `.primary-nav{padding:4px 10px 12px}`,而
  `.project-heading`/`.project-task-wrap` 都是 28px → 顶上两行比下面所有行高 4px,底下还压 12px 死带。
  动作:`height:28px` + `padding:4px 10px 8px`(字号 14px 不动)。
  ✅ **轮30 `fce84b0` 关闭**:`.primary-nav button` 32→28px、尾 padding 12→8px → 全 rail 一个 28px 节拍。
- ✅ **SB-12(P2,轮36 `29b7156` 收尾)底部账户行在重复品牌名,而不是标明「你是谁」;还塞了 3 个散装图标按钮**:`Sidebar.tsx:484-493`
  渲染 `<b>AgentRunner</b>` + `Connected · dev`,右侧挂 Settings/Help/Theme 三个 30px 按钮 —— 而 `AgentRunner`
  在 `Sidebar.tsx:289` 的 brand 行已经出现过一次(**同一根 264px rail 里产品名写了两遍**,却没有任何身份信息)。
  金标底部是一行 ~46px:头像 + 用户名 + 一个蓝点,别无他物。动作:`<b>` 换成账户/主机身份(daemon 状态并进副行),
  三个按钮收进 `···` 溢出菜单(复用 `Menu.tsx`),保留 daemon 状态点。
- ✅ **SB-13(P3,轮36 `29b7156`)没有 workspace 的任务被塞进假文件夹「Other sessions」;金标是平铺的 `Tasks` 段**:
  `viewModels.ts:95-101` `projectLabel()` 对空 workspace 返回 `"Other sessions"` → 在 `Sidebar.tsx:371-399` 被当成
  正常 project group 渲染(folder icon + caret + 缩进子行,live 上有 4 个命中)。folder icon 是一个断言
  (「这些任务属于磁盘上的一个项目」),对无 workspace 的任务这个断言是假的。
  动作:`projectLabel()` 空 workspace 返回 `null`;Projects 段之后加平铺 `Tasks` 段(与 Pinned 同缩进)。

- 2026-07-12 05:0x 轮29(headless)**4 条 ✅ + 1 条 ◐(2 P1)— 变更卡的文件行成了导航 + Scheduled 最响的东西不再是那扇门**。
  开轮先按 PROC-1 对账(`curl 8809 | grep index-*.js` vs `git log -1`)确认 live=main 再截图,比对 **diff / thread /
  home / Scheduled × light/dark**(before 存 `qa/runs/2026-07-12-r29/before/`,稳态 console 8/8 屏 = 0)对
  `codex-diff-review.jpg` / `codex-task-thread.jpg` / `codex-new-task-home.jpg` / `codex-crop-scheduled-list.jpg`。
  比对副产物:home 的 hero/建议卡/composer 几何**已收敛**(金标同为「卡组居中 + composer 钉底」),Scheduled 的
  cadence/next-run 副行**已达标** → 本轮最大可见差距落在**变更卡的死文件行**与**那颗黑 Create 药丸**上。
  并发派 **2 个 implementer**(worktree 隔离、白名单两两互斥:A=`Scheduled.tsx`+`styles.scheduled.css`+`styles.css` ·
  B=`ChangesOutcome.tsx`+`DiffView.tsx`+`api.ts`+`store.ts`+`SessionView.tsx`)+ **3 个 read-only finder**
  (Changes/Review 分栏 · Environment 右栏 · sidebar),落一个推一个。
  **TH-5(P1,本轮头号)** — thread 变更卡里的文件行**从标签变成导航**:`role=button` + `tabIndex=0` + `cursor:pointer`
  + hover 底(before 实测 `role=None / cursor=auto`,hover 透明,点下去**什么都不发生**);点一行 → Changes 面板打开、
  **该文件是唯一被强制展开的卡**(盖过默认折叠与 fold-all)并滚入视口,邻居保留用户自己的折叠状态。store 新增一次性
  `diffFocusPath`/`focusDiffFile`/`clearDiffFocus`;卡片右上 `Review` 行为不变。**零 CSS 改动**(CSS 本轮归 A 独占)。
  **SC-20(P1)** — Scheduled 屏 112×33 的实心黑 `+ Create` 药丸(全屏对比度最高的物体,坐在 640px 阅读列右上,
  金标那屏**根本没有实心黑按钮**)→ ghost:live 实测 light `rgb(13,13,13)`/白字 → **透明底 + `rgb(96,96,96)` 灰字**,
  dark `rgb(236,236,241)` → 透明 + `rgb(180,180,192)`;hover / 菜单打开才浮出 6% 浅底 + 细边。列表终于成为这屏最重的东西。
  **SC-21(P2,顺手真 bug)** — `✓ Mark all as read` 早就渲染在 tab 行右端(Codex 文案一字不差),但它读的是**全局**
  `scheduledUnread(sessions, unread)` —— 那个集合包含本页按 SC-1 刻意不列的 driver(一次性/goal)以及被当前 tab/搜索
  过滤掉的行 → 点一下会**静默清掉屏幕上根本不存在的行的未读态**。作用域收到 `filtered`(你看得见的那几行),
  按钮只在当前视图真有未读时渲染(+4 条单测)。侧栏圆点仍走全局(E3 语义不变)。
  **DF-D7 ✅ + DF-D8 ◐** — binary 文件的 blob 预取**根本不发**(`isBinaryPath()` 扩展名短路,`.svg/.json/.map` 照常取)
  + `blobRefused` 记住 400/413 拒绝问第二遍:live diff 屏 console **1×400 → 0**、binary 预取 **2 请求/4 error → 0/0**。
  剩下一半诚实登记为 ◐:`/diff` 只返回 `untracked: string[]`,前端拿不到 size → 413 无法预判(无扩展名的 10MB
  `bin/ar` 首次仍吃 1 条),要彻底关须让后端在 untracked 项上带 size/mime = 跨层,须先写三层 delta。
  push=`d9031c1`(A)+ `45ea4af`(B)+ `c6f6c0e`(finder 弹药)+ 本 commit;live=`index-CJhqgQRJ.js`;
  vitest **404 passed / 32 files**;复验 Scheduled/thread/diff × light/dark 稳态 console error+warning = **0**;
  截图 `qa/runs/2026-07-12-r29/{before,after,finder-review,finder-env,finder-sidebar}/`。
  BACKLOG:**+✅ 4 条、◐ 1 条**、新增 ☐ **18 条**(RVW-1..6 / ENV-1..6 / SB-8..13)。
  **下轮首选(finder 交回的最大三条,touches 天然互斥)**:
  **SB-9 + SB-8(P1,sidebar)** — 墨色层级是**倒的**:任务标题 `rgb(96,96,96)`/400、组名 `rgb(13,13,13)`/600
  (金标:子 `rgb(37,37,37)` 比父 `rgb(48,48,48)` **更黑**),8 个文件夹名在喊、14 个真正可点的行读起来像 disabled;
  外加选中行是全 app 唯一的蓝色块(而同一根 rail 的 nav 已是中性灰)+ `--row-bg` 错配导致 hover 时标题被糊掉。
  **ENV-1 + ENV-2(P1,右栏)** — 四条 Environment 行没有主文本(label/value 只差 14 级灰),且 label `flex:1` 撑出
  **211px 死沟**、右轨 344px = 窗口 23.9%(金标 16%)。
  **RVW-2(P1,diff)** — split 视图**硬裁长行且无滚动条**(两半都是 `1fr` → `.fd-body{overflow-x:auto}` 永不出条),
  与本项目「长行不硬裁」的既定决策自相矛盾。
  **RVW-1(P0,须先复核)** — finder 的金标像素采样称 Codex **连行号槽一起染色**(add 槽 `rgb(201,227,198)`),
  即轮25 DF-D2 的立论「Codex 只染代码列」**可能是错的**;这是对已落地刻意决策的翻案,**动手前先自己复核金标采样**。

- 2026-07-12 07:0x 轮30(headless)**10 条 ✅(P0×1 + P1×4)— 三个面同时朝金标收口:sidebar 讲回一种方言、
  行号槽跟着行一起染色、右栏不再念否定句**。比对 4 屏 × light/dark(before `qa/runs/2026-07-12-r30/before/`,
  after `.../after/`)。派工 3 个并发 implementer(worktree 隔离、白名单两两无交集:nav+Sidebar.tsx /
  styles.css+rs+diff+DiffView.tsx / panel.css+SupervisionPanel.tsx),落一个推一个部署一个。
  - **SB-8/9/10/11**(`fce84b0`,sidebar):选中行是全 app 唯一的蓝色块 → 中性灰药丸(金标 13 级灰、零色相);
    任务标题从灰(96)变成 rail 里最黑(13)、组名退为 96/600 → **墨色层级翻正**(此前父比子黑 83 级);
    brand 64→44px、nav 32→28px → 全 rail 一个节拍。附带修掉 `--row-bg` 与 `background` 不同源导致的
    hover 渐变吃标题真 bug。
  - **RVW-1(P0)/RVW-2/RVW-5**(`1520b48`,Changes/Review):**DF-D2 翻案** —— 金标采样证明 Codex 把红绿
    刷到行号槽(add 201,227,198 / del 248,195,189),我们的白槽把 176 行删除块每行劈成两半;删规则后整行
    连续染色。split 长行从「静默消失」变成能滚到结尾(maxScroll 0 → 1,901,162)。文件头 sticky,滚进
    大文件不再不知身在何处。
  - **ENV-3/4/5**(`232ea9b`,Environment 右栏):删掉 `No changes`/`No branch yet`/`Nothing to commit`
    三条与动作一样响的否定句;`Run details` 从面板最重的字降为最淡的一行、补图标、落到 label 列、下沉为
    footer(贴死 900 底,1px 上边线)→ 那片 510px 空白终于被框住而不是「撕掉的一块」。
  push `fce84b0` / `232ea9b` / `1520b48`;live=`index-DsX11mcr.js`;全景 4 屏 × light/dark 稳态 console 0。
  遗留:`what-agents-5849` 那类含超大 untracked 文件的会话仍吃 2×413(**已登记 ◐ DF-D8**,需 `/diff` 带 size,
  跨前后端、须先写三层 delta),非本轮引入。

- 2026-07-12 轮31(headless):比对 **thread(Environment 右栏)+ Changes/Review + Scheduled + home** 四屏
  对金标(`codex-thread-environment-panel.jpg` / `codex-crop-diff-header.jpg` / `codex-crop-scheduled-list.jpg` /
  `codex-new-task-home.jpg`)。**关闭 6 条可见差距(2 P1)+ 翻正 4 条撒谎的 ☐**:
  - **ENV-1/ENV-2(P1×2)**(`605ad95`):Environment 四行从**电子表格**变回**菜单**——label 从 96 灰拿回 **13** 墨色
    主文本、label↔value 的 **211.3px 死沟合拢到 16.2px**、右轨 344→288(≤1180 断点 304→264)。附带修掉两个只有
    实做才会撞见的坑:disabled label 拿 `--ink` 后反而比启用行还黑(129 vs 110);窄轨下主文本「Worktree」先于
    次文本被截。
  - **RVW-3/RVW-4/RVW-6**(`9d527b6`,Changes/Review):diff 终于有**复制出口**(金标头栏那颗 copy 图标);默认
    scope 从 `working-tree` 改回金标的 **Last Turn** —— 此前**变更卡说 `+2 −2`、点它的 Review 打开的却是另一份
    diff**,一次点击两份答案的分裂消失了;载态从一句灰字换成 3 文件 × 12 行的 skeleton。
  - **HM-6/HM-7**(`5a1d172`,home 空态):4 张建议卡从 **168×101.8 → 141×84**(金标实测 136×85)、gap 16→8、
    卡内图标 22→16、hero mark 50→35、整排占窗口 50%→41%(金标 39%);hover 描边从近黑降到 #c9c9c9。
    四张空旷大卡不再是首屏最重的东西。**composer 零变化**(QA-45 钉底决策)。
  - **台账翻正**(`bf5d5ce`):**SC-12/13/14 两轮前就进了 main、TH-10 轮24 就进了 main,勾全没翻** → 本轮据 ☐
    误派了一个 implementer(它核对后诚实拒绝造空 commit)。**PROC-1 复发,代价是一整个 implementer 名额。**
    下轮开轮**必须**先抽验 backlog 里排最前的 ☐ 是否真在代码里,再派工。新登记 SC-21 ✂(派生标题的残余差距,
    根因是数据层无 title 字段 → 属新增能力,不做)。
  push `bf5d5ce` / `605ad95` / `9d527b6` / `5a1d172`;live=`index-BzC0SdbM.js`;四屏 × light/dark 稳态
  console error+warning = **0**;vitest **413/413**。截图 `qa/runs/2026-07-12-r31/{before,after}/`。

---

## 轮32 本轮关闭(2026-07-12)

### DIFF-CP ✅ diff 顶栏的 `Commit or push` 在**默认 scope 下根本不存在**(`7075e04`)[P1]
- **Codex**:`codex-crop-diff-header.jpg` — `Last Turn ⌄ +649 −57 … ⋯ ⇅ 🔍 ▮▯ ⧉ │ ⊸ Commit or push ⌄`,
  提交胶囊**常驻**顶栏最右;不可用时**置灰**而非消失。
- **我们(before)**:按钮本身早已是一级按钮(`.diff-commit-btn`),但被 `scope === "working-tree"` 门控。
  轮31 的 RVW-4 把默认 scope 改成 `last-turn` 后,**用户默认打开 Changes 就一个提交出口都没有**——
  不在顶栏,也不在 `⋯` 里(Apply/Remove 同样 working-tree 门控,溢出菜单只剩 Refresh)。
  这道门什么也没保护:`AR.commit` 暂存的是 workspace,与你在读哪份 diff 无关。
- **关闭**:按钮在所有 scope 常驻;无改动时按金标置灰并说明原因。顶栏 <600px 退回纯图标(**仍常驻**,
  绝不退回 `⋯`)。宽度改由 **ResizeObserver 测顶栏**(不是窗口)—— 该面板是会话主列,1100px 窗口下
  它只有 415px,media query 看不见。
- **顺手修掉的潜伏 bug**:split/inline 切换器的 "split 需要空间" 规则也测在**窗口**上,于是 1100px 窗口
  下仍为 415px 的面板提供分栏(两列 ~190px 代码)。改测顶栏后它在真正用不了的地方退场,让出的 58px
  正好养活常驻按钮。13 档宽度 × 2 scope 对拍 main:溢出**每档 ≤ main**,working-tree 下按钮被挤出面板
  的 8 档**归零**。

### SCH-ICON ✅ Scheduled 每行都有状态图标,停跑的行整行降调(`8f576fe`)[P2]
- **Codex**:`codex-scheduled.jpg` — **每一行**左侧一个状态图标(active ○ / paused ⊘),paused 行整行灰。
- **我们(before)**:只有 needs-recovery 行有 ⚠,其余行左侧是**空槽**(看起来像图标没加载出来),
  三行标题一样黑,扫一眼分不出谁在跑、谁停了。
- **关闭**:`glyphFor(r)` 每行都渲染 glyph,全部由**已有**状态字段推导(broken ⚠ 保留 amber /
  running ▶ / settled ✓ / active ○ / dormant ⏸),删掉 `sched-blank` 空槽;`is-quiet` 行标题降到 `--dim`;
  Suggestions 行距 73→64px 与任务行同 pitch。
- **⚠ 本条推翻了一条旧决策**:`Scheduled.tsx:98-101`(sw-d-11 / SC-10 / SC-16)曾把「settled 行不画前导
  glyph、留空槽保对齐」定为刻意决策。**对着金标这条决策本身是反向的**——金标每行都有 glyph。SC-16
  当初剥掉 settled 行的 amber 是对的,但留下那个洞是错的。测试判据同步从「空槽」改成「中性 ✓ +
  无 `sched-warn` + `is-quiet`」,**语义没动**。
- **没有伪造 paused**:我们没有能挂起 driver 的 flag(所以 filter 第三个 tab 才叫 Finished 而非 Paused)。
  `PauseCircle` 给了真实存在的「无下次 tick 又未终结」态,不借金标的词。

---

## 轮32 新弹药(2026-07-12,3 个 read-only finder:thread / diff-review / sidebar+home+scheduled)

**登记一律 ☐;✅ 只在 merge 进 main 且复验通过后由收轮改写**(轮16 教训)。

### 【diff / review 分栏】(finder B,截图 `qa/runs/2026-07-12-r32/find-diff/`)

### RD-8 ✅ ≤1280 时 diff 顶栏溢出面板,**✕ 被裁到面板外 → 关不掉 Changes** [P1] — `bba2e91`(轮33)
**实测收口**:`BAR_TIGHT_PX` 600→640(按**面板**宽判定,不是窗口宽);面板 <640px 时 Copy + Wrap 一起降级进已有 `…` 菜单;`✕` 加 `.diff-closebtn { flex:0 0 auto; order:1 }` 永远最后一个、永不收缩。四档实测 `✕.right` vs `.diffwrap.right`:1440 1430/1440、1280 1270/1280、1152 1142/1152、**1024 改前 1051.9 出界 +27.9px → 改后 1014 ✅**。**诚实修正**:差距单里 1280/1152 的出界数字在当前 main 上**复现不出来**(那是 DIFF-CP 之前的状态),真正仍坏的只有 1024 档。
1280 下 `✕` x=1289–1317 整颗在面板外(面板 right=1280);1152 下出界 138px;1024 下 Wrap / inline-split
也一并出界。DF-1 声称修过,但 Commit 胶囊(150px 不收缩)回来后复发。DIFF-CP 已把溢出减轻(8 档归零、
余下两档 275/335 → 88/148)但未根治。**动作**:面板 <640px 时把 Wrap + inline/split 降级进 `…`;
`✕` 给 `flex:0 0 auto`。验收锚:1280/1152/1024 三档 `✕.right ≤ .diffwrap.right`。
touches:`DiffView.tsx` 顶栏区、`styles.rs.css` L774–830。

### RD-9 ✅ Split view 只渲染旧侧,新增行**整列在面板外** [P1] — `bba2e91`(轮33)
**实测收口**:`.fd-split` 两条代码轨 `minmax(max-content,1fr)` → `minmax(0,1fr)`,`.dls-half` 加 `min-width:0` + 省略号。1440 打开 split(会话 `20260711-060645`,7 个 `.fd-split`):改前最差轨 `63.1px 16px 63.1px 943464px` → 新增行面板内可见 **0/19**;改后 `63.1px 265.7px 63.1px 265.7px` → **19/19**。
`.fd-split` 实测 `grid-template-columns: 63.1px 1901680px 63.1px 16px` —— 左(旧)轨取 max-content 把右(新)
轨挤到 16px 且在面板右缘外。`22-light-split.png`:index.html 5–13 行只剩红色旧侧,**两条绿色新增行一行
都看不到**。DIFF-CP 已把 split 按钮的可用性改由面板宽判定(窄面板退场),但**宽面板下 max-content 撑爆
的根因仍在**。**动作**:面板 <900px 强制 inline 并 disable split;≥900px 时给两轨 `minmax(0,1fr)` 而非
`minmax(max-content,1fr)`。touches:`DiffView.tsx` L232–239、`styles.rs.css` L191–198。

### RD-10 ✅ hunk 分隔条是**蓝色**的 —— review 里唯一一块饱和蓝,且与灰折叠带信息重复 [P2] — `e5fef23`(轮35)
`.dl-hunk` 去饱和,并入 `.fd-gap` 折叠带的中性族。真机实测(会话
`20260711-011831-what-is-the-project-297d`,一个 3-hunk 的 `test-rig.ts` 真 diff;稳态,鼠标已移出):
light `.dl-hunk` bg `rgb(232,241,251)`(`--blue-soft`)/ 文字 `rgb(1,105,204)`(`--blue`)
→ **`rgb(244,244,244)` / `rgb(110,110,110)`**,与同屏 `.fd-gap` 的 `rgb(244,244,244)` / `rgb(110,110,110)`
**逐值相等**;dark `rgb(27,37,64)` / `rgb(111,155,255)` → **`rgb(28,28,32)` / `rgb(160,160,173)`**,同样与
`.fd-gap` 逐值相等(`--panel-2`/`--dim` 自带 dark 值,dark 白拿)。review 里最后一块饱和色回到 +/− 代码本身。
**取舍:`@@` 行保留,不删。** 金标里没有 `@@` 行,但 `diffSummary.ts:64` 表明该行渲染的**不是**
`@@ -a,b +c,d @@` 行号区间(行号早被解析进 `oldNo`/`newNo`,每行 gutter 都在印),而是 git 的
**section heading**(实测三条:`import fs, {…}`、`export function printDebugInfo(`、
`export function checkModelOutputContent(`)——这串「你在哪个函数里」全 UI 别无二处。金标那张恰是
Markdown 文件,git 对它不产 heading,故看不见;删掉它等于为对齐一张截图丢掉代码文件的真上下文。
无 heading 的 hunk 仍走 `.dl-hunk-blank` 细线(styles.rs.css),不受影响。零 markup 改动。
touches:`styles.css` L1671–1699。

### RD-11 ✅ 文件头坐在 #f5f5f5 灰底色带上 —— **实测不成立:原报告量到的是 :hover 态** [P3] — 轮34 复验,零代码改动
**诚实缩窄到 0**:`.fd-head` 的静态背景**早已**是 `var(--panel)`,与 `.diffbar` 同一个 token
(`styles.diff.css` RVW-5 的 sticky 规则本来就这么写)。真机 13 个文件头的背景去重后只剩一个值:
light `rgb(255,255,255)` == `.diffbar` 的 `rgb(255,255,255)`;dark `rgb(23,23,26)` == `rgb(23,23,26)`
(`qa/runs/2026-07-12-r34/after-rd1112/measurements.json` 的 `light_bg` / `dark_bg`)。
原报告的 srgb(0.962)=#f5f5f5 正是 `.diffwrap .filediff > summary.fd-head:hover` 的
`color-mix(in srgb, var(--ink) 4%, var(--panel))` —— `--ink:#0d0d0d` 按 4% 混白得 245 —— 即**鼠标停在
文件头上**时的合法 hover 底色,不是常态背景。截图 `1440-filelist-popover.png` 里 13 条文件头全白底、
只靠 `.filediff + .filediff` 的 hairline 分隔,没有任何灰实心带。故本条无债可还,**不做假改动**。

### RD-12 ✅ 缺「这次改了哪些文件」的总览入口;`N generated files hidden` 抢了第一屏 [P3] — `eec680c`(轮34)
**实测收口**:放大镜过滤 popover 升级成**文件列表 popover**(触发器 `TreeStructure`;常驻控件数不变 ——
1440 档 `.diffbar` 仍 10 个子元素、`scrollWidth−clientWidth = 0`,1024 档 7 个、溢出 0、✕ 在面板内)。
真机(`20260710-213428-…-d8ac`,last-turn):popover 列出 **13/13** 个改动文件(含 untracked),每行
`M/A/D 字形 + 路径 + +N −M`(binary 的 `asset.bin` 不编造计数,与其文件头 DF-D3 一致);点 `asset.bin`
→ `.diffwrap.scrollTop 0 → 407`,该文件头从 y=770(视口外)滚进视口且落在 sticky 工具栏**下方**
(新增 `.filediff { scroll-margin-top: 46px }`,顺带修好 TH-5 的落点),并自动展开。输入 `lcov-1` →
列表 13→3 行、面板内文件同步 3 个、标签 `3 of 13 files match`、触发器保持 active(过滤语义不变,
且从筛过的列表里点文件**不再清空 query**)。working-tree 档:`.diffwrap` 第一个子元素是 `.diffbar`、
**第二个就是第一个文件头**(`noteInStream: false`);`12 generated files hidden / Source files all still
shown.` 连同长句 tooltip 完整落在 popover 底部注脚。三档稳态 console error+warning **0**。
vitest 442 绿(基线 438 + 4 条新用例:列表渲染/点击跳转/过滤/空匹配)。

### 【thread 屏】(finder A,截图 `qa/runs/2026-07-12-r32/find-thread/`)

### TH-14 ✅ composer 上方**两条**常驻终态横幅堆叠,同一件事说三遍,读区被压到 70% [P1] — `8aa15be`(轮33)
**实测收口**:`goalBannerShown = goalLive && !terminalNotice`;goal 的 label/elapsed 并进 `.terminal-alert-meta`;`SupervisionPanel` 已结算 goal 压成 `.goal-settled-line`。1440 实测:常驻横幅 **2 → 1**,`.timeline` **630 → 678px(+48)**,右轨 Goal 段 123px 三行块 → 单行。
实测 `.timeline` 只有 **630px / 900**:`terminal-alert`「Step limit reached … Continue in new task」
(SessionView.tsx:884-900)+ `gbar`「Goal cancelled … 00:34 ✕」(:901-914)两条常驻,吃掉 216px;右轨 Goal 卡
**又**说一遍「Cancelled / 00:34 / 0 checks」。金标的终态是最后一条消息 action 行里的一段灰字
`⊘ Goal achieved in 3h 47m 26s`,**随内容滚走**,composer 上方零常驻 chrome(会话流占窗口 ~92%)。
TH-12 只做了「横幅在 ⇒ 砍 thread 里的 chip 回声」,没覆盖 alert 与 gbar 同屏、也没覆盖右轨回声。
**动作**:`SessionView.tsx:642` 的 `goalBannerShown` 加 `&& !terminalNotice`;goal 的 label/elapsed 并进
`terminal-alert` 的 meta 段;`SupervisionPanel` 的 Goal 段在 `terminal && bannerShown` 时压成单行。净收 ~46px。
touches:`SessionView.tsx`、`SupervisionPanel.tsx`、`styles.css` `.terminal-alert*` 段。

### TH-15 ✅ 按钮叫 "Supervision",打开的面板叫 "Environment";Changes 有两个入口 [P1] — `8aa15be`(轮33)
**实测收口**:顶栏 tool 按钮 `["Changes","Supervision"]` → `["Environment"]`(SlidersHorizontal,与轨内同图标);用户可见的 "Supervision" 字样全部改成 "Environment";Changes 只剩轨内首行 + `···` 兜底。**顺带修一个真 bug**:轨内 Changes 行原本靠 `document.querySelector('.task-topbar button[title="Review workspace changes"]').click()` **合成点击顶栏那枚按钮**来开 diff —— 删掉按钮后它会**静默失效**;已改成 `onOpenChanges` 回调。
顶栏两枚 tool 按钮(`Changes` :718 / `Supervision` :720-724),点 Supervision 打开的面板**唯一可见标题是
"Environment"**(DF-D4 已删掉 "Supervision" 标题条),轨内第一行又是 `Changes`,`···` 菜单里还有第三个
Changes 入口。金标里这条轨自始至终叫 **Environment**,`Changes` 是轨内第一行,顶栏**没有**这两枚按钮。
一个心智对象三个名字、两个入口。**动作**:顶栏按钮文案 `Supervision` → `Environment`、图标换
`SlidersHorizontal`(与轨内一致);删掉顶栏 Changes 按钮(`···` 已兜底,轨内行是主入口)。
touches:`SessionView.tsx`、`styles.nav.css` `.topbar-tool`。

### TH-16 ✅ 系统 chip 裸浮在消息之间,金标 thread 里一枚都没有 [P2]
`Agent changed · auditor · gemini-flash-latest` + `… · dev · … ×2`(实测 36px 高、合计 762px 宽)+
`goal attached · …` 是 `.tl-inner` 的**顶层**子元素,抢在正文层级、抢在第一条消息之前。金标整屏只有
prose / artifact 卡 / 变更卡 / `Worked for 1h 37m 40s ›` 折叠——所有非答复活动都在折叠里。
**动作**:`timeline.ts` 的 fold/group 归类里把 `agent_changed` / `goal_attached` 归入相邻 activity group
(`.act-body .chip` 样式已有,RT-4 给 approval chip 做过同一手),顶层只留终态 chip。
touches:`timeline.ts`、`timeline.test.ts`、`styles.conv.css`。

**已关闭(轮35 implB,sha 4859462)**:`ChipItem.system` 新标志——运维管线(换 agent / 挂 goal /
改 goal 文案)**永不**做顶层渲染节点。它比 `fold` 更强:`fold` chip 仍会让位给 post-answer window
(goal check 该贴在它解释的 goal 结果旁边),`system` chip 不让——`foldWork` 无条件把它 buffer 进
相邻 activity fold(照抄 RT-4 把 approval chip 塞进 step list 那条路径)。两处兜底:①只装着 system chip
的 buffer 不单独开折叠(否则「裸 chip」只换成「裸 Worked · 1 item 行」),而是顺延进**下一个** turn 的
折叠——那正是它描述的 turn(换 agent 就是为了那一轮);②journal 末尾无 turn 可顺延时,强制 flush 开一个
自己的活动组,绝不丢。live 实测(1440,会话 `20260711-011831-…-297d`,静息态):`.tl-inner` 顶层 chip
**4 → 0**(275/275/274/294px 那条 ~1118px 灰带子消失),展开折叠后 4 条内容/顺序/`×2` 聚合全部仍在;
console error+warning 0。vitest 453/453(+6)。证据:`qa/runs/2026-07-12-r35/impl-b/`。

### TH-17 ✅ composer 的 `Full access` 只有图标是橙的 —— **实测不成立:原报告量到的是 low 档的 pill** [P2] — 轮35 复验,零代码改动
**诚实缩窄到 0**:`styles.css:2063` 的 `.cx-mode.high { color: var(--amber) }` **早已存在**,把整条 pill
(label + 图标)一起染橙。真机复现(home composer,点开权限 popover 选 `Full access`,鼠标移出后稳态量):
pill class = `cx-pill cx-mode high`,**label `color: rgb(138,90,0)`、图标 `rgb(138,90,0)`——同为 `--amber`
(#8a5a00),整条橙,无差异**(`qa/runs/2026-07-12-r35/impl-a/high-before.json`,量的是**改前**的 live 8809)。
原报告的 `rgb(96,96,96)` 是 **low 档** pill(`Ask to approve`,class `cx-pill cx-mode low`)的中性色 ——
即 `.cx-pill { color: var(--ink-2) }` (#606060);而 TH-17 自己写明「low/medium/unknown 保持现状不变」,
所以量错了元素状态。同屏 `unknown` 档(`Access: set by agent spec`)同样是 `rgb(96,96,96)`,亦属预期。
按要求加 `risk-high` class + `.cx-pill.cx-mode.risk-high { color: var(--amber) }` 只会是一条与 2063 完全
等价的死规则,**故不做假改动**。(顺带记一处**未触发**的隐患:`styles.css:4096` `.cx-mode:disabled
{ color: var(--dim) }` 与 `.cx-mode.high` 特异性同为 (0,2,0) 而位置更靠后,若哪天 mode pill 被 disable,
label 会掉回灰而图标因 inline style 仍橙 —— 正是原报告描述的症状;但 `Composer.tsx` L1371/L1420 两处
pill 均**未**设 `disabled`,当前不可达,不预先改动。)

### TH-18 ☐ jump-to-latest 圆钮在**中途滚动**时压住正文 [P3]
`.tl-jump` `position:sticky; bottom:14px; margin:-32px auto 0`;实测 `ours-thread-scroll-2.png` 圆钮盖住
`routing/ & availability/ (智能路由机…` 一行的字。48px 底 padding 只在滚到底时救得了它。**动作**:
`.timeline` 底部加 40px `linear-gradient(transparent → var(--bg))` 遮罩,圆钮永远落在渐隐带上;
离底 <1 屏时隐藏。touches:`styles.css` `.timeline`/`.tl-jump`、`Timeline.tsx`。

### TH-19 ✅ thread 顶栏标题过重过长,抢走整条栏 [P3] — `e5fef23`(轮35)
`.tt-title`(定义在 `styles.css`,非原条目猜的 `styles.nav.css`/`.topbar-title`)降级。真机实测(长标题会话
`20260712-052313-…-8637`,稳态、鼠标已移出):**14px / 600 / `rgb(13,13,13)` / max-width 560px、实占 560px
且省略号截断** → **12.5px / 500 / `rgb(110,110,110)`(`--dim`)/ max-width 340px、实占 340px**,截断保留。
字号 −1.5px、字重 −100、从近黑压到 `--dim`,标题让位给控件。dark:`rgb(236,236,241)` → `rgb(160,160,173)`
(`--dim` dark 值,白拿)。**不塌陷不错位**:`.task-topbar` 高度 54px 不变、`overflow=false`、
`documentElement` 无横向溢出;390 窄屏走既有 media query 的 `max-width:42vw`(=163.8px,比 340 更紧,
仍在我这条之后生效)——实占 164px、截断正常、顶栏与文档均无溢出。稳态 console error+warning = 0。
touches:`styles.css` L3787–3800。

### 【sidebar / home / scheduled】(finder C,截图 `qa/runs/2026-07-12-r32/find-nav/`)

### SB-11 ✅ Projects 分组名和 "Show more" 一样灰,整棵树读不出层级 [P1] — `976b728`(轮33)
**实测收口**:`.project-heading` → `color-mix(in srgb, var(--ink) 78%, var(--ink-2))`(token 混色,不写死 hex)。亮度差 vs `.show-more`:light **14 → 78.7**(rgb(96,96,96)→rgb(31,31,31));dark **19.9 → 62.9**。`.show-more`/`--dim`/600 字重均未动。
`styles.nav.css:726` `.project-heading { color: var(--ink-2) }` = **#606060**,`.show-more` 是 `--dim` #6e6e6e
—— 只差 **14 级**,截图里 `rt1-ws` / `Scratch` 和 `Show more` 同一个灰。金标里分组名是近墨色 rgb(48,48,48),
与脚手架 rgb(160,160,160) 相差 **112 级**。SB-9 修反向倒置时**过冲**了,把分组名一路推进脚手架灰,
127 个组的树因此没有锚点。**动作**:`color-mix(in srgb, var(--ink) 78%, var(--ink-2))` ≈ #303030,
保留 600 字重;`.show-more` 不动。touches:`styles.nav.css`。

### HM-8 ✅ Home hero 标题 30px,金标 ≈23px —— 大了 33%,把刚瘦身的卡片又压回去了 [P1] — `6bf7086`(轮34)
用 HM-7 确立的 px→逻辑 px 映射反量金标:标题 360 逻辑 px / 36 字 = **10.0px per char**,字重 400;
我们 `413px / 31 字 = 13.3px per char`(`styles.home.css:85-92` 30px/500)。HM-4 的注释写着 "land on Codex's
lighter 30px/500" 但那次没有用映射反推,数字是拍的。轮31 已把 4 张卡收到金标同量级,**标题没跟**,
现在独自比周围重一号。**动作**:`font-size: 23px; font-weight: 400; letter-spacing: -0.2px`;390 断点 25→20px。
卡片/图标/composer 一律不动。touches:`styles.home.css`。
**实测(chrome headless dsf=2,私有 :8872,light/dark 一致)**:1440 档 `.hero h2` **30px/500 → 23px/400**,
标题宽 `413px/31 字 = 13.3` → **309px/31 字 = 9.97 px per char**(金标 10.0,误差 0.3%);390 档 **25 → 20px**
(styles.css:4283 的 24px 未夺回控制权——`.home-empty .home-empty-headline` (0,2,0) 仍压 `.hero h2` (0,1,1))。
卡片未动:141×84、row 588px、min-height 84px;composer 720px 未动(QA-45 钉底)。稳态 console error+warning = 0。
新增 `Home.headline.test.tsx`:把两张表按 import 序灌进 jsdom **读 computed 值**(不是 grep 声明),
钉死 23px/400/-0.2px —— HM-4 那种"写了但被特异性压成死代码"的失败模式这次会直接测红。截图
`qa/runs/2026-07-12-r34/after-hm8/`。vitest 438 → **443 绿**。

### SC-22 ✅ Scheduled 整页比金标大一号,首屏只装下 3 条任务 + 3 条建议 [P1] — `3b07a87`(轮34)
**实测收口**(真机 Chrome、1440×900 + 390×844 × light/dark,私有 :8871 接真实 driver 数据):
`.scheduled-row` 高/pitch **68 → 54px**(min-height 68→54、padding 12→9px,标题 leading 1.55→1.35、
副行 1.55→1.4 —— 字号一个没动,买回来的全是行距);建议行 pitch **~65 → 54.3px**(padding 10→8px、
head leading 1.25、desc 14→13px);页标题 **28 → 23px**、"Suggestions" **19 → 15px**、副标题 **14 → 13px**。
无截断/重叠/溢出(`.scheduled-page` 子树溢出探针 0 条),`Needs recovery` 橙字仍在(light `rgb(138,90,0)` /
dark `rgb(230,185,104)`)、`is-quiet` 降调 2 行、glyph 槽位齐,390 窄档不塌,稳态 console error+warning **0**。
**全局零污染**:本页标题是 `<h2>`(不是 h1),`.page-heading` 只此一处渲染,改动全部 scope 在
`.scheduled-page` 下;Settings 面板通篇没有 `<h1>/<h2>/<h3>`(其标题是 div),截图复验未变。
截图 `qa/runs/2026-07-12-r34/after-sc22/`。
逐项实测(金标标度 ÷2.48):列表行 pitch **68 vs 54**、"Suggestions" 标题 **19 vs 15**、H1 **28 vs 22**、
副标题 **14 vs 13**。(建议行 pitch 73→64 已由 SCH-ICON 关掉一半,金标是 54。)每项单独超 20–35%,
叠起来就是「我们的 Scheduled 是金标的放大版」。这屏的职责是**一眼扫完所有还在自己跑的东西**,
行高是这个职责的直接代价。**动作**:`styles.css:3723` `min-height 68→54`、`padding 12px→9px`;
`styles.scheduled.css:24` `19→15px`;`.scheduled-page` H1 加 scope `23px`(**别动全局 h1**,Settings 也吃它)。
touches:`styles.scheduled.css` + `styles.css` **3635–3740 段**(不得越界)。

### SB-12 ✅ Sidebar 页脚把产品名当账户名念了第二遍,单行的事占了两行 [P2] — `976b728`(轮33)
**实测收口**:删 `<b>AgentRunner</b>`,页脚 `AR AgentRunner / Connected · dev` → `AR Connected · dev`;`.side-foot` **58 → 41px**、`.account-badge` **40px 两行 → 28px 单行**。离线红字 + 可点重启路径由新增用例钉住,未丢。
`Sidebar.tsx:487` 硬编码 `<b>AgentRunner</b>` + `Connected · dev`,实测页脚 **40px 两行**;而
"AgentRunner" 这个词在 850px 之上的品牌行**已经出现过一次**。金标底部是**单行 ~26px**:头像 + 用户名
+ 一个蓝点。页脚的信息量只有一条(daemon 通不通)。**动作**:删掉 `<b>AgentRunner</b>`,`account-meta`
只留状态句(离线时仍是红字可点重启);页脚 40→~28px,还回一条任务行。3 个图标先留着。
touches:`Sidebar.tsx`、`styles.nav.css`。

### SB-13 ✅ Sidebar 品牌行 32px 黑色圆角 logo 方块,金标是纯文字 wordmark [P3] — `976b728`(轮33)
**实测收口**:品牌行去掉 accent 方块 + `Robot` 图标(连 import 一起),只剩文字 wordmark;`.brand-main svg` true → **false**。
黑方块是全屏最深的色块,把眼睛钉在一个不可点的装饰上。金标 `codex-crop-sidebar-nav.jpg` 只有
"ChatGPT Codex" 文字 + 右侧搜索图标,无图形块。**动作**:去掉方块底,只留 wordmark(或图标降为线性 16px)。
touches:`Sidebar.tsx`、`styles.nav.css`。

### SB-14 ✅ Sidebar 分组头多一个金标没有的次要后缀 [P3] — `976b728`(轮33)
**实测收口**:`.project-hint` 静息 `display:none`,hover/focus-visible 才 `inline`,`@media (hover:none)` 永不显示;`ws · qa53-013333` 不再占 264px 轨里的名字空间。
`ws · qa53-013333`(`Sidebar.tsx:400` `project-hint`)—— 金标分组头只有仓库名;后缀在 264px 的轨里吃掉了
名字的空间。**动作**:hint 降为 hover / `title=`。touches:`Sidebar.tsx`、`styles.nav.css`。

### CP-8 ☐ ⌘K 面板列表底部把一行字从中间切断,贴着圆角 [P3]
`styles.nav.css:692` `.cmdk-list { max-height: min(64vh, 620px) }` 与 36px 行高不整除,最后一行
"First use bash to run: pwd" 被面板圆角齐腰切断、无留白。**动作**:加 `padding-bottom: 6px`,
max-height 对齐行高格点。touches:`styles.nav.css`。

### 轮32 新登记的 ✂(核对代码/注释后判为刻意决策或 out-of-scope,不修)
- ✂ **diff 不画 `+`/`−` 符号列** —— `.dl-sign{display:none}`(styles.rs.css:98)。**像素采样金标证实
  Codex 也不画**:`codex-crop-diff-rendering` 里 1203 那个 `−` 是 markdown 列表符(该行无红底无红条);
  Codex 同样只靠底色 + 左条。
- ✂ **行号槽无竖分隔线** —— 采样金标 ctx 行 x≈111 亮度 253–255,Codex 也没有。
- ✂ **diff 面板宽度占比** —— 我们 658/1175 = 56%,金标 ≈51%,**我们已更宽**。
- ✂ **hover 行 inline 批注** —— 需新后端(批注),out-of-scope。
- ✂ **右轨 730px 空白虚空** —— `styles.panel.css:191-198` ENV-5 明确:不用假内容填(我们没有
  Background processes / Browser / Sources 数据源)。
- ✂ **每条消息常驻 action 行 + 时间戳**(金标是 hover 才出)—— `styles.conv.css:722-751` TH-1/TH-10
  明确推翻过 hover-only:"hover-only 意味着 Copy/Copy link 对触屏用户根本不存在"。
- ✂ **变更卡对未跟踪文件显示 `new` 而非 `+7 -0`** —— `ChangesOutcome.test.tsx:171`:后端 numstat 缺失时
  宁可不印,也不印 `+0 −0` 的假数字。补真数字要动 Go 端 → out-of-scope。
- ✂ **home composer 底部大输入框** —— QA-45 钉底决策。
- ✂ **Scheduled 第三个 tab 是 "Finished" 而非金标的 "Paused"** —— `Scheduled.tsx:17-22`:我们没有 paused
  语义,不借词。
- ✂ **sidebar 行高 28px vs 金标 25px** —— SB-3 钉过 "3px 内不动"。

### 轮32 抽验结论(应轮31 教训,开轮先核实老 ☐ 是否真在代码里)
- **SB-4(sidebar 段级折叠)= ✅ 属实** —— `Sidebar.tsx:42-53` / `:157-163` / `:373-441`,CSS `styles.nav.css:599-660`。
- **SC-5(Suggestion cadence 兑现)= ✅ 属实** —— `Scheduled.tsx:37-67` 每条带真 `CadenceSpec`,`Modals.tsx:126`
  交给 `runFormDefaults`(SC-18)。
- **SC-9(`#/scheduled` 深链)= ✅ 属实** —— `routeHash.ts:7-12` `normalizeRoute`;live 截图正常渲染。
- **Scheduled next-run = ✅ 早已实现**(主线自查):`Scheduled.tsx:89-90` 用后端 `nextRunAt`;finished 行显示
  "Ran 3d ago" 是**刻意的**(:121 注释)。**避免了又一次误派**。
- **sidebar Pinned 段 = ✅ 早已实现**:`Sidebar.tsx:328-332` + `store.ts:76` localStorage,只是当前无 pinned 会话。
- **"Mark all as read" = ✅ 早已实现**:`Scheduled.tsx:512`,无未读时不渲染。

- 2026-07-12 轮32:比对 4 屏(thread/diff/scheduled/home × light/dark)、关差距 **DIFF-CP(P1,默认 scope 下
  提交出口根本不存在)+ SCH-ICON(P2,Scheduled 空图标槽 + 停跑行不降调)**、派工 2 implementer(并发,
  worktree 隔离,白名单互斥:`DiffView.tsx`+`styles.rs.css` / `Scheduled.tsx`+`styles.scheduled.css`)+
  3 finder(read-only,thread/diff/nav);push `8f576fe` / `7075e04`;live=`index-LsNja6qZ.js`;
  四屏 × light/dark 稳态 console error+warning = **0**;vitest **426/426**(基线 413 + 8 + 3)。
  BACKLOG:+✅×2、新增 ☐×**15**(RD-8..12 / TH-14..19 / SB-11..14 / HM-8 / SC-22 / CP-8)、新增 ✂×10。
  截图 `qa/runs/2026-07-12-r32/{before,after,after-diffcp,after-schicon,find-*}/`。
  **本轮唯一的翻案**:SCH-ICON 推翻了 `Scheduled.tsx:98-101`(sw-d-11/SC-10/SC-16)「settled 行留空槽」的
  旧决策——对着金标那条决策本身是反向的。已在条目里诚实登记。

- 2026-07-12 轮33:比对 4 屏(thread/diff/home/scheduled × light|dark × 1440|1280,12 张 before/after)、
  关差距 **8 条(4 P1)**:**RD-8**(1024 档 ✕ 出界 27.9px,关不掉 Changes)+ **RD-9**(split 只渲染旧侧,
  新增行 0/19 可见 → 19/19)+ **TH-14**(终态两条横幅 → 一条,`.timeline` 630→678px)+ **TH-15**(Supervision/
  Environment 一物三名两门 → 一名一门,并修掉「删按钮后轨内 Changes 静默失效」的真 bug)+ **SB-11/12/13/14**
  (分组名亮度差 14→78.7、页脚 58→41px 单行、黑 logo 方块去掉、hint 收进 hover);派工 3 implementer
  (并发,worktree 隔离,白名单互斥:`DiffView.tsx`+`styles.rs.css` / `SessionView.tsx`+`SupervisionPanel.tsx`+
  `styles.css` / `Sidebar.tsx`+`styles.nav.css`);push `976b728` → `8aa15be` → `bba2e91`(落一个推一个);
  live=`index-C2n4EcAm.js`;四屏 × light/dark 稳态 console error+warning = **0**;vitest **438/438**。
  截图 `qa/runs/2026-07-12-r33/{before,after,after-rd89,after-th1415,after-sb}/`。
  **本轮两处诚实修正**:(a) RD-8 的 1280/1152 出界在当前 main 上**复现不出来**(DIFF-CP 已修),真坏的只有
  1024 档 —— 条目已据实缩范围;(b) 主线自己的截图脚本用了 `#/session/<id>` 路由,渲染成 "Task not found" ——
  webui 只认 `#<id>` / `#/s/<id>`,是**脚本的错不是产品的错**,已纠正并同步给三个子 agent。
  **下轮首选**:HM-8(P1,home hero 标题 30px vs 金标 23px,轮31 卡片瘦身后它独自大一号)+ SC-22(P1,
  Scheduled 整页大一号)+ TH-16(P2,系统 chip 裸浮在消息之间)。

- 2026-07-12 轮34:比对 3 屏(home/scheduled/diff × light|dark × 1440|390)、关差距 **4 条(2 P1)**:
  **SC-22**(P1,Scheduled 整页大一号 → 行 68→**54px**、页标题 28→**23px**、段标题 19→**15px**、建议行
  ~65→**54px**;行高不是砍字号买的,是 leading 1.55→1.35/1.4 + padding 12→9px,字号/省略号/颜色一个没动)
  + **HM-8**(P1,home 问候语 30px/500 → **23px/400**,`413px/31字=13.3 px/char` → **309px/31字=9.97**,金标 10.0;
  390 档 25→20px;卡片/composer/钉底一律未动)+ **RD-12**(review 终于有了「这次改了哪些文件」的入口:
  放大镜过滤 popover **1:1 升级**成文件列表 popover(`TreeStructure`),13/13 文件带 `M/A/D + 路径 + +N −M`、
  点击滚过去并展开(顺带修好 TH-5 的落点被 sticky 工具栏盖住)、过滤语义保留且**从筛过的列表点文件不清 query**;
  `12 generated files hidden` 从 review **第一行**降为 popover 底部注脚 → 第一行现在是第一个文件头;
  常驻控件数**不增**、1440/1024 `.diffbar` 溢出仍 0、✕ 仍在面板内)。
  派工 3 implementer(并发,worktree 隔离,白名单互斥:`Scheduled.tsx`+`styles.scheduled.css`+`styles.css`3721–3739 /
  `styles.home.css`+`components/Home.tsx` / `DiffView.tsx`+`styles.rs.css`);push `3b07a87`+`acbbf3e` → `6bf7086`+`beb630b`
  → `eec680c`+`7f8a719`(落一个推一个);live=`index-B5S9YBbI.js`;vitest **447/447**;三屏 × light/dark ×
  1440/390 稳态 console error+warning = **0**。截图 `qa/runs/2026-07-12-r34/{before,after,after-sc22,after-hm8,after-rd1112}/`。
  **本轮一处诚实翻案:RD-11 是个假差距 —— 撤回,零改动标 ✅**。原报告称 `.fd-head` 背景 srgb(0.962) 灰带;
  主线与 implementer 双向复验:**静息态 13 个文件头全是 `rgb(255,255,255)`,与 `.diffbar` 逐位相同**,0.962 只在
  `summary.fd-head:hover`(`color-mix(--ink 4%, --panel)`)出现。finder 当时的探针把鼠标留在了面板上,量到的是
  hover 态。**教训写进纪律**:量 computed style 前必须 `mouse.move(5,5)` 移出 + 阳性对照,否则 hover/focus 态
  会被误读成常态(与已有的「playwright sync sleep 不 dispatch 事件」同类)。
  **下轮首选**:TH-16(P2,系统 chip 裸浮在消息之间,金标 thread 里一枚都没有——live 实测 4 枚)+ RD-10(P2,
  hunk 分隔条是 review 里唯一一块饱和蓝,且与灰折叠带信息重复;`styles.css` 本轮被 SC-22 占用故让路)+
  TH-17/TH-19。

## 轮35 新弹药(2026-07-12,1 个 read-only finder:富会话右栏 vs Codex Environment 面板)

参照 `qa/codex-reference/codex-thread-environment-panel.jpg`;取证截图 `qa/runs/2026-07-12-r35/find-env/`。
finder 已逐条比对 `webui/api.go` 的 56 条路由,确认下列条目**全部有后端**。

### RD-A ✅ Environment 面板的 git 状态**只在挂载时取一次**,整轮跑下来永远是旧值 [P1] — 轮36 `3cfdb92`
30s 抓包(面板开,会话 `…-d8ac`):`S/events` 32 次、`S/ps` 13 次、`S/inspect` 12 次,而 **`S/diff` 只有 2 次
(都在挂载瞬间)**、`/api/git/branches` **1 次**。坐实:`SupervisionPanel.tsx:477` `const load = useCallback(…, [sid])`
+ `:516` `useEffect(() => load(), [load])` —— 只有 sid 变或面板自己 commit/push 后才重取。同屏的 thread 变更卡却是
活的(`ChangesOutcome.tsx:316` deps `[sid, refreshKey, bump]`,`refreshKey={events.length}`,`SessionView.tsx:890`)。
**后果:同一个屏幕上 thread 卡说「Edited 12 files」,右栏 Changes 行还是空的、`Commit or push` 还 disabled**,
关掉面板再打开才对。一个不刷新的状态面板比没有面板更糟——它会主动说谎。
**动作**:`EnvironmentSection` 加 `refreshKey` 入参(`:465`),`load` deps 改 `[sid, refreshKey]`(加 ≥2s 节流,
别把 `ar diff` 打成每事件一次);`SupervisionPanel` props 加 `refreshKey`(`:153-204`)并透传(`:248`);
`SessionView.tsx:1009` 传 `refreshKey={events.length}`(与 `:890` 同源)。
touches:`SupervisionPanel.tsx`、`SessionView.tsx`、`SupervisionPanel.test.tsx`。

### RD-B ✅ 右栏是一根满高布局列:**72% 是空的**,且一开一关把正文横甩 144px [P1] — 轮36 `3cfdb92`
实测 1440×900(鼠标已移出 + 400ms,light/dark 同值):`aside.supervision-panel` = **288×846px**,是 flex 兄弟节点、
吃布局宽度。内容(Environment 4 行 + settled goal)y=273 就结束,`Run details` 被 `margin-top:auto` 钉在 y=856
→ 中间 **583px 空白**(diff 会话 606px,占面板高 **72%**)。开/关面板:`main` 1176→888,正文列宽恒定 660px 但
左沿 **x=522 → x=378(横移 144px)**——每次「看一眼环境」,读到一半的整段话就横着滑走。
金标:Environment 是一张**贴内容长的浮层卡**(≈244×480,4 分区 12 行,填充率 ~100%),吊在 header 的 sliders
图标下,圆角+投影,盖在右侧留白上,thread 正文列**一动不动**。
**动作**:`styles.panel.css:188-202` 改浮层(`position:absolute; right:12px; top:56px; width:288px; height:auto;
max-height:calc(100% - 96px); overflow:auto`),`.session-view` 加 `position:relative`;去掉 `:326-338` 的
`.supervision-details { margin-top:auto }`(卡片贴内容后 `Run details` 自然跟在最后一行下 = Codex 的 `View all`);
`SessionView.tsx:999-1046` 面板不再与 `<main>` 争宽度(`view==="diff"` 的 `.changes-panel` 保持原样,那是真需要宽度的评审面)。
touches:`styles.panel.css`、`SessionView.tsx`、可能 `styles.css`(`.session-view` 布局)、`SessionView.chrome.test.tsx`。

### RD-C ✅ Worktree 行展开是个死胡同:只给一段折成 4 行的路径 + Copy path [P2]
**轮37 落地 `8f77ad9`**:抽出共享 `worktreeActions.ts`(`useWorktreeActions`,含 Remove 的确认弹窗与 force
二次确认),DiffView 改调它、UI/行为一字未动(5 个 DiffView 测试文件一行未改全绿);SupervisionPanel 的
Worktree 抽屉从「路径 + Copy path」变成动作抽屉:`Apply to project…` / `Open in VS Code` / `Remove worktree…`
(危险项在末位、走红色文字通道而非饱和红块)。`Open in VS Code` 复用 Sidebar 右键菜单那条 `openProjectIn`
store action(**不在 DiffView 里**,任务书写错了),自带错误 toast。`EnvState` 补 `worktree`/`mainRepo`。
live 实测抽屉里 `.env-wt-action` = `['Apply to project…','Open in VS Code','Remove worktree…']`。
遗留:抽屉里的路径仍被撕成多行 mono(**ENV-6**,P3)——动作行补上后这块的毛糙更显眼了,建议提到 P2。

<details><summary>原始条目</summary>
点开是 ~90px 灰块,里面一条 worktree 路径**硬折 4 行**,右边一个 `Copy path`——**能做的事只有复制**。而后端早有、
UI 却藏在别处的:`GET /api/sessions/{sid}/diff` 的响应已带 `worktree`/`mainRepo`/`branch`(面板拿到了却不显示);
**Apply to project**(`POST …/apply`)与 **Remove worktree**(`POST …/worktree/remove`)只活在 `DiffView.tsx:866-880`
的 `…` 菜单里,而打开 Changes 会把 Environment 面板**整个替换掉**(`SessionView.tsx:999-1007`)——两个入口互斥,
用户得先离开面板才能操作面板告诉他的那个 worktree;**Open in VS Code/Finder/Terminal**(`store.ts:309` → `POST /api/open`)
只在 `Sidebar.tsx:562-568` 的右键菜单里(发现率最低处)。
**动作**:改 `SupervisionPanel.tsx:652-667` 的 `.env-detail`:路径单行截断 + Copy 收成图标钮;补 `base`(mainRepo)/
`branch` 两行元数据(detached 留空,遵守 ENV-3);补动作行(Open in… / Apply to project… / Remove worktree…),
后两个把 `DiffView.tsx:435-505` 的 confirm 模态抽成共享 `worktreeActions.ts` 两边共用(别复制粘贴);
`EnvState`(`:435-443`)补 `mainRepo`/`worktree` 字段(`load()` 在 `:484-497` 已拿到完整 `d`,只是没存)。
touches:`SupervisionPanel.tsx`、`DiffView.tsx`、新增 `worktreeActions.ts`、`styles.panel.css`。
</details>

### RD-D ✅ 右栏 Changes 行:有改动时**未跟踪文件被吞掉**,且从不给文件数 [P3] — 轮36 `3cfdb92`
真实 payload:1 tracked(+1)、`untracked:["qa-inc41-d4/asset.bin"]`、`hiddenUntracked:12`;右栏只写 **`+1`**。
`SupervisionPanel.tsx:625-633`:`{env.untracked} new` 只在 `add===0 && del===0` 时渲染 —— 一有改动,新建文件就
彻底消失;文件数从来没渲染过(`EnvState.files` 在 `:492` 算了没用)。金标是 `Edited 31 files +980 −317`,**文件数在最前**。
**动作**:`:622-634` 值区改 `{files} files` + `+add −del` + `·{untracked} new`(untracked 独立渲染,不再被 add/del 门挡住)。
touches:`SupervisionPanel.tsx`、`SupervisionPanel.test.tsx`。

### RD-E ✅ Background work 排在整根栏的最后一格 [P3] — 轮36 `3cfdb92`
`SupervisionPanel.tsx:401-412` 排在 Goal/Progress/Artifacts/Agents/Attention **全部之后**;金标里 `Background processes`
紧跟 Environment 四行,是面板上**第二个**读到的东西(直接列原始命令行)。后端有(`GET …/ps` → `ar ps`,`api.go:856`)。
**动作**:把 `:401-412` 整块 `<section>` 上移到 `:248` 的 `<EnvironmentSection>` 之后。touches:`SupervisionPanel.tsx`。

### TH-20 ☐ TH-16 收口疑点:`goal attached` chip 折叠后在 11 个 fold 里都找不到 [P3]
轮35 主线复验 TH-16 时发现:富会话 `…-297d` 展开全部 11 个 `.worked` 折叠,3 条 `Agent changed` chip 都在,
但 `goal attached` 一条都找不到。**非硬丢失**——goal 的全文与终态由 goal banner 承载(实测 `.goal-settled-line`:
`Cancelled · TH-3 live check…`),且 `timeline.ts:898` 的 A5 路径本就优先在匹配的 user 气泡下标 `⚡ Sent as goal`
而不发 chip。但 implementer 声称的「末尾无 turn 可顺延时强制开一个自己的活动组」这条兜底**未在 live 上观察到生效**。
**动作**:查 `timeline.ts` 末尾 flush 的兜底分支是否真会被走到;补一个「journal 末尾只剩 system chip」的单测。
touches:`timeline.ts`、`timeline.test.ts`。

### 【out-of-scope 复核(轮35 finder,附证据)】
- **Browser 分区**(Codex 列 agent 开的浏览器标签):`grep -rniE "browser|chrome|cdp|devtools|tabs" webui/*.go` 只命中
  注释,**零 handler** → 不做,不建壳页。
- **Sources 分区**(上下文源列表):只有 `POST /api/upload` + `GET /api/uploads/{name}` + `GET …/files`(composer @-mention),
  **没有「这个会话挂了哪些上下文源」的模型或列表接口**;硬做只能拿 composer 附件伪造一个列表 = 壳页 → 不做。
- **Environment 标题右边的 `+`**(Codex:新建/切换 environment 沙箱):我们有 `POST /api/worktree`(建 git worktree),
  但**没有 "environment" 这个后端概念**(无 provisioning/镜像/远端沙箱);把 `+` 映射成「新建 worktree」是换语义,不是补差距 → 不做。
- PR 集成 / 插件注册表 / 站点托管:`api.go` 零路由、`docs/SPEC.md` 无条目 → 一律 out-of-scope。
- **已达 parity、无需再动**:Environment 四行(Changes/Worktree/Create branch/Commit or push)的行序、图标语义、
  disabled 语义、行高(28px vs ~25px)、面板宽度(288 vs ~244)——前几轮已关,本轮实测无差距。

- 2026-07-12 轮35:比对 2 屏(thread / diff-review × 静息态实测)、关差距 **3 条**(TH-16 P2 + RD-10 P2 + TH-19 P3),
  **撤回 1 条假差距(TH-17)**。
  **TH-16**(thread 顶层不再有裸系统 chip):`.tl-inner` 顶层 chip **4 → 0**(before 实测 4 枚:`Agent changed·auditor`×2、
  `Agent changed·dev ×2`、`goal attached·…`,合计 ~1118px 宽横在读者与第一条消息之间);after 顶层是干净的
  `msg user → .worked → msg assistant → turn-sep` 交替,**正是金标 thread 的结构**;3 条 `Agent changed` 展开
  `.worked` 折叠后原样可达(`×2` 聚合保留)。做法:新增 `ChipItem.system` 档(`spec_changed`/`goal_attached`/
  `goal_updated`)——比 RT-4 的 `fold` 更强一档,**永不**做顶层渲染节点,且不碰 RT-4 的 post-answer window 语义
  (原测试一字未改全绿)。vitest 447 → **453**(+6)。
  **RD-10**(review 里唯一的饱和蓝没了):`.dl-hunk` light `rgb(232,241,251)`/`rgb(1,105,204)` → **`rgb(244,244,244)`/
  `rgb(110,110,110)`**;dark `rgb(27,37,64)`/`rgb(111,155,255)` → **`rgb(28,28,32)`/`rgb(160,160,173)`** —— 两档都与
  同屏 `.fd-gap` 折叠带**逐值相等**(主线在 live 8809 上用 3-hunk 真实 diff 复验通过)。饱和色回归 +/− 代码本身。
  **`@@` 行保留不删**(推翻原条目的「直接不渲染」建议):`diffSummary.ts:64` 表明它渲染的不是行号区间(行号在每行
  gutter),而是 **git section heading**(实测 `export function printDebugInfo(` 等),全 UI 别无二处;金标那张恰是
  Markdown、git 不产 heading 才看不见——删它 = 为对齐一张截图丢掉代码文件的真上下文。
  **TH-19**(顶栏标题让位):`.tt-title` `14px/600/rgb(13,13,13)/max-560px`(长标题实占 560px 且截断)→
  **`12.5px/500/--dim/max-340px`**(live 实占 118px);顶栏高 54px 不变、无溢出;390 档沿用既有 42vw。
  注:真身在 `styles.css` 第二个 `.tt-title` 块(3778 行胜出),**不是**原条目猜的 `styles.nav.css`/`.topbar-title`。
  **⚠ TH-17 撤回 —— 第二个假差距(同轮34 RD-11 性质)**:`styles.css:2081` `.cx-mode.high { color: var(--amber) }`
  **早在 `24aeccb` 就存在**;`risk=high` 的 pill 实测 label 与图标同为 `rgb(138,90,0)`,**整条橙、零差异**。原条目量到的
  `rgb(96,96,96)` 是 **low/unknown 档**的中性色——而 TH-17 自己写明「low 保持不变」,即量错了元素状态。**零改动撤回,
  不做假改动。** 教训与 RD-11 同源:**量之前先确认自己量的是不是目标状态的那个元素**(不只是 hover 污染,还有档位混淆)。
  顺带记录一处当前**不可达**的真隐患:`styles.css:4123` `.cx-mode:disabled { color: var(--dim) }` 与 `.cx-mode.high`
  特异性同为 (0,2,0) 但位置更靠后 —— 若哪天 mode pill 被 `disabled`,label 会掉回灰而图标因 inline style 仍橙,
  正是原报告描述的症状;目前两处 pill 均未设 `disabled`,触发不了。
  派工 2 implementer(并发,worktree 隔离,白名单互斥:`timeline.ts`+`timeline.test.ts`+`styles.conv.css` /
  `styles.css`+`styles.diff.css`+`Composer.tsx`+`DiffView.tsx`)+ 1 read-only finder(右栏 vs Environment 面板)。
  push `4859462`+`80b1102`(TH-16)→ `e5fef23`+`05f9050`(RD-10/TH-19),**落一个推一个**;live=`index--xKf3VYL.js`;
  vitest **453/453**;复验屏稳态 console error+warning = **0**。截图 `qa/runs/2026-07-12-r35/{before,after-th16,after-main,impl-a,impl-b,find-env}/`。
  **BACKLOG:+✅×3、撤回 ✂×1(TH-17)、新增 ☐×6**(RD-A/RD-B 两条 **P1** + RD-C/RD-D/RD-E + TH-20)。
  **下轮首选**:**RD-A**(P1,Environment 面板 git 状态只在挂载时取一次 → 面板会主动说谎:thread 说「Edited 12 files」
  而右栏 Changes 空着、`Commit or push` 灰着)+ **RD-B**(P1,右栏是满高布局列、72% 空白,且开关一次把正文横甩 144px;
  金标是贴内容长的浮层卡)。两条 touches 有交集(都碰 `SupervisionPanel.tsx`/`SessionView.tsx`)→ **并进同一个 implementer 串行做**;
  另一个 implementer 可拿 RD-D+RD-E(同文件,也得并进 RD-A/RD-B 那份)或改拿 TH-18/TH-20 这类独立文件面。

- 2026-07-12 轮36:比对 2 屏(thread+Environment 面板 / sidebar,对 `codex-thread-environment-panel.jpg` +
  `codex-crop-sidebar-nav.jpg`),**关差距 6 条(2 条 P1)**,派工 2 implementer(并发、worktree 隔离、白名单互斥:
  A=`SupervisionPanel.tsx`+`SessionView.tsx`+`styles.panel.css`+`styles.css` · B=`Sidebar.tsx`+`viewModels.ts`+`styles.nav.css`)。
  **RD-B(P1,本轮头号)右栏从满高布局空柱 → 贴内容的浮层卡**:live 8809 实测 `aside.supervision-panel`
  **288×846 `position:static`(72% 空白)→ 244×265 `position:absolute`**(高由内容决定);开/关面板 `main` 恒 1176、
  正文列左沿恒 x=492 —— **横移 144px → 0**(原来每看一眼环境,读到一半的整段话就横着滑走);`Run details` 从
  `margin-top:auto` 钉底 y=856 → 贴内容 y=276(= 金标的 `View all`)。卡宽实测取 244(金标同值,正好等于正文列右侧
  留白 → 零遮挡);implementer 诚实反驳了条目原写的 288px(实测会盖住正文右缘 42px,把"横甩"换成"蒙眼")。
  `view==="diff"` 的 `.changes-panel` 分栏保持原样(那是真需要宽度的评审面)。
  **RD-A(P1)面板不再说谎**:`EnvironmentSection` 加 `refreshKey`(2s leading+trailing 节流),`SessionView.tsx:1010`
  传 `refreshKey={events.length}`(与 `:890` ChangesOutcome 同源)—— 真 Gemini turn 实测:面板不重开,3s 内 Changes 行
  自己从 `· 2 new` → `1 file +1 · 2 new`。此前 30s 抓包 `S/diff` 只有挂载瞬间那 2 次。
  **RD-D** Changes 值区改 `{files} files +add −del · {untracked} new`(untracked 不再被 `add===0&&del===0` 挡掉,文件数
  首次渲染;live 卡上现为 `2 files +4 −3` = 金标 `Edited 31 files +980 −317` 的信息序)。**RD-E** Background work 从整栏
  最后一格上移到 Environment 之后(= 金标第二读)。
  **SB-13** `projectLabel()` 空 workspace 不再编造 `"Other sessions"` 假文件夹(live 命中 **1 → 0**),这类任务平铺进新的
  `Tasks` 段(无 folder/无 caret,与 Pinned 同缩进 x=18;pinned 不重复出现)。**SB-12 收尾** 底部 3 个散装图标钮
  (Settings/Help/Theme)收进 `···` 溢出菜单(复用 `Menu.tsx`,向上弹出),底栏实测只剩 1 个触发器 + presence 点。
  **落一个推一个**:`29b7156`(侧栏)→ 部署复验 → `3cfdb92`(右栏)→ 部署复验。vitest **453 → 470**(+17)。
  live=`index-CrwmkD1V.js`;复验屏稳态 console error+warning = **0**。截图 `qa/runs/2026-07-12-r36/{before,after,after-sb}/`。
  **BACKLOG:+✅×6、新增 ☐×0**。
  **下轮首选**:**RD-C(P2)** Worktree 行展开是死胡同(只能 Copy path;`Apply to project` / `Remove worktree` /
  `Open in VS Code` 三个已有后端的动作分别锁在 DiffView 的 `…` 菜单与 sidebar 右键菜单里,与面板互斥)——现在面板成了
  浮层卡、不再与 Changes 分栏争地盘,正是补齐它动作行的时机;之后 TH-20(timeline 末尾 flush 兜底)。


---

### TH-21 ✅ thread 每条消息下方都常驻一行 action 图标 + 时间戳,Codex 只有末条有 [P1]
**轮37 落地 `4d80a58`。**
- **Codex 怎样**(本轮像素取证,`qa/codex-reference/codex-task-thread.jpg` 819×1456,把 y=1180..1420 与
  y=380..700 两段放大 3× 看过):中间的每一条 assistant / user 消息结束后**直接**接下一段内容——
  **没有 action 图标行、没有时间戳**。整个 thread **只有最后一条 assistant 消息**之后有一行
  `⧉ 👍 👎 ↗ │ ⊘ Goal achieved in 3h 47m 26s`,而且**这一行里也没有时间戳**。
- **我们(轮37 前)**:**每一条**消息(含 user bubble)下方常驻 3 个图标(opacity .5)+ 完整时间戳
  `Friday 06:21 PM`。一个 21 条消息的会话 = 21 条重复噪音带,每两三段散文之间插一次,把 thread 从
  「一条连贯的叙事」切成了「一叠带页脚的卡片」。证据 `qa/runs/2026-07-12-r37/before/thread-1440.png`。
- **改法**:中间消息的 `.msg-actions` 改 hover / `:focus-within` 才显(**只翻 `opacity`,不动任何盒模型属性**
  —— TH-1 的不 reflow 契约照守);末条 assistant 打 `.msg-last`(复用已有的 `lastAssistantKey`)豁免,常驻;
  所有消息不再常驻时间戳(末条的 `.msg-time` 走静态 `display:none`,因为 `Timeline.turns.test.tsx` 的 TR-2
  要 query 它)。新增 `Timeline.msgrow.test.tsx` 7 条(含注入真实样式表后 `getComputedStyle` 实测)。
- **⚠ 本轮推翻了 round 18/20/24 的 TH-1/TH-10 结论**(「图标常驻是对的、hover-only 是误读」):那轮只看了
  单张裁剪 `codex-crop-message-actions.jpg` 就推断"图标在静息态就有"——**那张 crop 拍的正是 thread 的最后
  一行**,不是中间消息。整屏金标证明中间消息零 action 行零时间戳。折中:末条常驻(保留 crop 的真相 + 触屏/
  键盘可达),中间 hover-only(对齐整屏真相)。旧注释块保留,顶部加了指引。
- **live 复验**(`index-NjS8Tk5S.js`):21 行 `.msg-actions`,20 行静息 `opacity=0` + 末条 `=1`;可见时间戳 **0**;
  hover 中间消息 → `opacity` 0→1 且 `height` 恒 19px(**不 reflow**);全景 console error+warning = 0。
  截图 `qa/runs/2026-07-12-r37/{before,after}/thread-1440.png`。
touches:`Timeline.tsx`、`styles.conv.css`、`Timeline.thread.test.tsx`、新增 `Timeline.msgrow.test.tsx`。

---

## 轮37 finder 新登记:Changes / Review 分栏 × `codex-diff-review.jpg`(5 条)

> finder 先更正了入口:**Changes 分栏没有顶栏按钮**(TH-15 刻意删,理由在 `SessionView.tsx:747-758`)。
> 三扇门:`···` 溢出菜单 → View → Changes;Environment 栏的 `Changes` 行;thread change card 的 `Review`。
> 总体结论:这一屏已很接近金标(逐文件头 + `+x −y`、M/A/D glyph、语法高亮、`N unmodified lines` 折叠带、
> hatched 左轨、sticky 文件头、`Commit or push` 都在),剩下 5 条。

### RVW-CLIP ✅ 面板第一个标签被切成 "Working tre",caret 掉出按钮外 [P1]
DOM 取证:`.diff-scope-trigger` `clientWidth=76` / `scrollWidth=97`(切掉 21px)、`text-overflow: clip`(连省略号
都没有)、caret `<svg>` 的 `right=886` 而按钮 `right=872.8` —— **caret 被裁到按钮外面**。bar 本身并没溢出
(`scrollW == clientW == 658`),纯属收缩分配错了对象:`styles.css:1559` / `styles.rs.css:828` 两处都没写 `flex`,
默认 `0 1 auto` 可收缩,而 bar 上其余控件全是 `flex: 0 0 auto`。`DiffView.tsx:314-316` 的 DF-1 注释还明确写着
worktree chip「is the one control allowed to give way」——实际让路的却是 scope 控件本身。Working tree 是持久化
偏好(`SCOPE_KEY`),选过一次就永远是这个断词的样子。
**动作**:`styles.rs.css:828` 的 `.diffwrap .diff-scope-trigger` 加 `flex: 0 0 auto; white-space: nowrap;`
(让 `.diff-wt-badge` 独自吸收收缩,它本来就有 `min-w-0 truncate`)。需要新后端:否。
**✅ 轮38 `c9f1bc9`**:`.pop-wrap:has(> .diff-scope-trigger)` 与 trigger 双双改 `flex: 0 0 auto; min-width: auto; overflow: visible`,收缩回落到 `.diff-wt-badge`(DF-1 契约的原意)。live 实测 `working-tree` 态 cw/sw **76/97 → 109/109**、文案完整、caret **886>872.8 → 898≤905**(收回按钮内)、bar 仍不溢出;@1024 窄宽同样不截断,让路的只有 worktree chip。Last turn 态无回退。

### RVW-PHANTOM ✅ 每份 review 的最后一个文件都多渲染一行「幽灵空行」 [P1]
`A rd-d-untracked-probe.txt +1 −0` 的正文渲染了**两行**(第 2 行是空的);原始 diff 是 `@@ -0,0 +1,1 @@` + 一条
`+` 行,**只有 1 行**。根因:`diffSummary.ts:156` `diff.split("\n")` 让末尾换行产生一个 `""`,被塞进**最后一个
文件**的 `lines`,再被 `diffSummary.ts:70` 的兜底分支当成 ctx 行(还消耗一个 `newNo` → 尾部 `hunkGaps` 的
`N unmodified lines` 计数有 off-by-one 风险)。review 唯一不能出错的东西就是 diff 本身。
**动作**:`diffSummary.ts:156` 丢弃尾部空行 + 补单测(payload 末尾带 `\n` 时最后一个文件的 rows 数)。需要新后端:否。
**✅ 轮38 `7c63a40`**:`splitDiff()` 新增 `diffLines()` 丢弃 payload **尾部**空行(正文里合法空行是 `" "`/`"+"`/`"-"`,永远不是 `""`,不会误伤)。尾部 band 的 off-by-one 一并归位(起点 14 → 13)。+5 条单测钉住(含「中间空行一行不少」的反向保护)。影响面是**每一份** review 的最后一个文件——git 的 diff payload 基本总以换行结尾——不只取证那个 probe。live 复验:`+1 −0` 的文件正文只剩 1 行。

### RVW-ORDER ✅ review 开屏先给两个打不开的 binary,真改动被顶到折叠线以下 [P2]
`DiffView.tsx:1184-1196` 先渲染 `shownUntracked` 再渲染 tracked,于是右栏顶部两行是 `A bin/ar [binary]` /
`A bin/arwebui [binary]`(点开只有「Content isn't shown」)。金标第一行就是第一个**有内容**的文件头。
**动作**:`DiffView.tsx:719/1184` 把 untracked 与 tracked 按 path 归并排序(或至少把 binary 沉到末尾),
文件列表 popover 用同一顺序。需要新后端:否。
**✅ 轮38 `39451ac`**:新增 `ReviewFile`/`cmpReviewFile`,untracked 与 tracked 归并成**一个** `shown` 数组(可读文件按 path 升序在前、binary 沉底),stream 与文件列表 popover **渲染同一个数组**(此前是两份独立数组,顺序一分叉点列表就跳错文件)。live 复验:开屏第一张卡是有内容的 `qa42-worktree-browser.txt`,`asset.bin` 沉到末尾。

### RVW-BINCOUNT ✅ 文件列表对 binary 报 `+… −0`,同一文件的头部却说「binary、不给计数」 [P2]
`DiffView.tsx:726` 的 `counts: !isBinaryPath(path)` 走扩展名判定,而 `bin/ar` **没有扩展名** → 判不出 binary →
popover 印 `+…`(永不兑现)、卡片还会为它发一次注定 400 的 blob 请求;正文里同一文件却是 `[binary]` 不带计数。
**动作**:`UntrackedFile`(`DiffView.tsx:1286`)已有 `failed`/`lines` 的真相,回调上报给 DiffView 覆盖扩展名猜测。需要新后端:否。
**✅ 轮38 `39451ac`**:`UntrackedFile` 新增 `knownBinary`/`onFact` —— 它是唯一知道真相的地方(blob 拿到 lines / 端点 400 拒绝),把 `{binary, add}` 回报给 DiffView 的 `facts`,真相**覆盖**扩展名猜测,作用于列表计数、排序、是否再问服务器三处(合并 monotone:remount 不会把已知 `+42` 退回 `+…`)。live 复验:binary 行不再印 `+…`,与它头部的 `[binary]` 一致;重复 mount 不再发注定 400 的 blob 请求。**诚实余量**:首次 mount 仍花一次请求——只有服务器能说「这不是文本」。

### RVW-HUNKBAND ☐ 每个 hunk 顶着两条全宽灰带(≈55px),金标只有一条 [P2]
`9 unmodified lines` 带(30px)**紧接着**再来一条 `.dl-hunk` 全宽灰带(25px,实测 `w=657.5 h=25 bg=#f4f4f4`),
内容是 `@@` 的 section heading,且**没有行号槽**,横向打断代码网格。
**注意**:保留 heading **文本**是刻意决策(`styles.css:1683-1689`:金标那个文件是 Markdown 才没 heading;
对代码文件这串字符是唯一告诉你身处哪个函数的东西)——本条只针对**两条带子叠在一起的形态**。
**动作**:把 heading 并进折叠 band 的右半边(GitHub 的做法):`DiffView.tsx:1562-1576` 在 `bandEl` 存在时不再
单独渲染 `header`,把 `r.text` 传进 `band()`;`styles.diff.css:75` 的 `.fd-gap` 网格加第三列。信息一条不丢,
每个 hunk 省 25px 且不再断网格。需要新后端:否。

### ✂ finder 排除的假差距(刻意决策,别派)
- ✂ **顶栏没有 `Review`/`Changes` tab**:TH-15 刻意删,`SessionView.tsx:747-758`(「三个名字、两扇门、一件事」)。
- ✂ **行首没有 `+`/`−` 号**:`styles.rs.css:97-115` 的 `data-diff-markers`,Settings › Appearance 可切,默认色带是决策。
- ✂ **长行不换行、横向滚动**:`DiffView.tsx:96-98`(DF-4)按 Codex 默认定的,且有 Wrap 开关。
- ✂ **hunk heading 文本本身**:见 RVW-HUNKBAND。
- **out-of-scope**:金标顶部的 `AgentRunner | (1) AgentRunner | …` 是 Codex 的**窗口级多标签**,我们没有对应后端/多窗口模型。

### RT-ROUTE ☐ 未知 hash 路由不落 NotFound,反而当成 session id 去打 3 个必 404 的 API [P3]
本轮探针误用 `#/settings`(Settings 其实是**模态框**、不是路由),结果 SessionView 把 `settings` 当 sid,
连发 `/api/sessions/settings/{events,ps,inspect}` 三个 404。`NotFound.tsx` 存在却没被路由到。
**动作**:`App.tsx:192` 的 `route()` 对不存在的 sid 走 NotFound(或先探一次 session 存在性再挂 SSE)。需要新后端:否。

---

- 2026-07-12 轮37:比对 5 屏(home/scheduled/thread/thread+panel/changes-split)× Codex 金标;关差距
  **TH-21(P1)** thread 中间消息的常驻 action 行 + 时间戳 → hover-only、仅末条常驻(21 行里 20 行静息
  opacity=0、可见时间戳 0、hover 不 reflow),**RD-C(P2)** Environment 面板 Worktree 抽屉从只读展示柜
  → 补齐 `Apply to project…` / `Open in VS Code` / `Remove worktree…`(复用共享 `worktreeActions.ts`,
  Remove 仍走确认弹窗;DiffView 5 个测试文件一行未改全绿);派工 3(2 implementer 并发 worktree 隔离 +
  1 finder);push `4d80a58` `8f77ad9`;live=`index-NjS8Tk5S.js`;全景 console error+warning = **0**
  (desktop/mobile × light/dark × 5 屏 + Settings 模态框)。截图 `qa/runs/2026-07-12-r37/{before,after}/`。
  **BACKLOG:+✅×2(1 P1)、新增 ☐×6**(RVW-CLIP/RVW-PHANTOM 两条 **P1** + RVW-ORDER/RVW-BINCOUNT/
  RVW-HUNKBAND + RT-ROUTE)。
  **下轮首选**:**RVW-PHANTOM(P1)** —— 每份 review 的最后一个文件都多渲染一行不存在的代码,且尾部折叠
  计数可能 off-by-one:review 屏是最重要的一屏,而 diff 的正确性是它唯一不能出错的东西;并行可打
  **RVW-CLIP(P1)**(scope 标签被切成 "Working tre"、caret 掉出按钮外,纯 CSS `flex` 一行修)。

## 轮38 finder 新弹药(2 个 finder:Home/Scheduled + thread/Environment)

### ART-SCRIM ✅ 点变更/产物卡的 `Open in ⌄` → **整个 app 变成一块灰色空白** [P0](轮39 落地 `dcbc46e`)
**已修**(`dcbc46e`):真根因不是 UA `buttonface`,是本项目 `tw.css` `@layer base` 给每个 `<button>` 上了不透明
`background: var(--panel)` + `:hover → var(--panel-2)` —— 裸遮罩 `<button class="fixed inset-0">` 于是铺了满屏白、光标停上面转灰。
加 `bg-transparent border-0` 后遮罩照旧接管「点外关菜单」但一个像素都不画。**顺带**揪出同一控件上更狠一条:菜单 74px 高却被两层之上
`ArtifactChips` 的 `overflow-hidden` 裁到只剩 9px,连命中测试一起没了 → `Open in` 此前 **100% 不可用**。同类根因(祖先 overflow 裁 popover)
由本轮 ENV-CLIP 的 portal 化根治。

<details><summary>原始 finder 证据</summary>
真机 headed Chrome(dsf=1)实测:点 `Open in` 后视口四个采样点(侧栏/正文/顶栏/品牌)**全部**变成 `rgb(244,244,244)`;
`elementFromPoint` 返回 `BUTTON.fixed inset-0 z-[5] cursor-default`。DOM 完好、console 干净——纯粹被一张**不透明全屏遮罩**
盖住。根因:`ChangesOutcome.tsx:161` 的关闭遮罩是**裸 `<button>`**,项目里没有 `button { background: transparent }` 复位,
它拿到 UA 默认的 `buttonface` = `#f4f4f4` 铺满 1440×900(全仓唯一一个 `fixed inset-0` 的裸 button)。用户第一反应是「app 崩了」,
而且它连自己弹出的菜单都盖掉一半。历轮 QA 只截静息态、从没点过这颗按钮,所以一直没被发现。
**动作**:`ChangesOutcome.tsx:161` 加 `bg-transparent`,并给全局 button 补透明底复位防复发。需要新后端:否。
证据:`qa/runs/2026-07-12-r38/finder-thread/headed-openin.png`。
</details>

### ENV-CLIP ✅ Environment 面板切掉自己的 `Commit or push` 菜单:3 个 git 写动作只有 1 个能点 [P1] — R51 finder live 复验:已修(`Popover.tsx:191` fixed 定位逃出 overflow,3 个写动作 hitSelf=true)
`.supervision-panel` = `absolute; height:265.3px; overflow:auto`(bottom=321);`Commit or push` 的 `.pop-panel` 是它的 **DOM 后代**,
rect bottom=446 → **125px(56%)被 `overflow:auto` 裁掉**。不只是看不见——**点不到**:`elementFromPoint` 打 `Commit & push`(1300,338)
与 `Push`(1300,405) 都命中背后的 `DIV.timeline`。根因:RD-B(轮36 `3cfdb92`)把面板改成浮层卡时加的 `styles.panel.css:250` `overflow:auto`,
而 `Popover.tsx:114` 的 `.pop-panel` 是**内联 absolute、不 portal**,z-index 逃不出祖先的 overflow 裁剪。后端 `/commit`、`/push` 早就有。
**动作**:`Popover.tsx:114` 把 `.pop-panel` `createPortal` 到 body + fixed 定位(`useLayoutEffect` 已在量位置),一并防住 `.diffwrap`/
`.timeline` 里的同类隐患;廉价替代:去掉 `styles.panel.css:250` 的 `overflow:auto`(面板实测 265px,从没滚动过)。需要新后端:否。

### HM-9 ✅ project 选择器只认 5 个 workspace,而 store 里有 202 个;"Search projects" 搜不到侧栏上看得见的项目 [P1] — R51 finder live 复验:已修(全量 285 workspace 去重,搜 qa57→4 条含 qa57-browser、搜 agentrunner→189 条;`Composer.tsx:143-153`)
`Composer.tsx:132-143` 的 `recentWorkspaces` 写死 `if (out.length >= 5) break;`,`:983` 的 `filteredProjects` **只在这 5 个里过滤**。
live `/api/sessions` = 457 session / **202 个不同 workspace**;搜 `qa57-browser` → `No projects found`,**而这个项目就在同一帧的侧栏里躺着**。
用户的日常主仓 `agentrunner` 不在那 5 个里 → 想在自己主仓开任务只能手打绝对路径。**搜索框在撒谎**(与 HM-4「No environment 假控件」
同类的诚实性问题,但这次挡的是主流程)。**动作**:去掉 `>= 5` 硬截断(无 query 仍只显示最近 5 条做默认视图,一有 query 就在**全量
workspace 集**上过滤);列表容器已是 `max-h-[180px] overflow-y-auto`。数据全来自 `allSessions`,需要新后端:否。

### FOLD-RUN ✅ 展开一个 `Worked for 4m 20s` = 6585px(9.7 屏):39 个 step 里 33 个没被聚合 [P1] — R51 finder live 复验:已修(`timeline.ts:492-577` 重写 foldRuns,40 step 聚合进 1 个 act-group,裸 step=0,body 仅 41px)
`.worked-body` 高 **6585.1px**(`.timeline` clientH 仅 678px);里面 `.act-group` = **1**、裸 `.step` = **33**。根因:`timeline.ts:478 foldRuns`
明确「planning narration … does break the run」,而 `Timeline.tsx:749-755` 只在 `run.tools.length > 1` 时才包 `ActivityGroup` —— Gemini 几乎每次
工具调用之间都吐一段 thinking,于是 39 个工具被切成 33 个「单工具 run」,全部退化成全宽裸行。金标展开是十来条**聚合活动行**
(`Ran commands` / `Read files` / `Edited files, read files, ran commands`),每条再自己点开;我们把整轮推理原文 + 每条命令一次性倒在正文列上,
**折叠的意义当场作废**。**动作**:让 run **吸收**中间夹的 narration(narration 照常按序渲染但不 close 当前 run),或按 category 跨整个 fold 聚合;
`groupLabel`(`Timeline.tsx:587`)已能吐多类标签。touches:`timeline.ts:478`、`Timeline.tsx:744-755` + `timeline.test.ts`。需要新后端:否。

### HM-10 ✅ 四张建议卡的行宽只有 composer 列的 82%,页面出现第三条左边线(金标齐平一列) [P2] — R51 finder live 复验:已修(cards 与 composer 逐像素齐平 w=712 left=494 right=1206,`tw.css:384 max-w-[720px]`)
金标**同图内**量(无标度):卡片行 308 图 px ÷ composer 318 图 px = **96.9%**,两者左右边缘齐平,整页只有一条内容边线。我们:`styles.home.css:129`
`max-width: 588px` → 1440 实测 composer `w=720`、卡片行 `w=588`(**左右各内缩 66px,只有列宽的 81.7%**),首页从上到下出现三条不同的边。
**⚠ 本条推翻 HM-7 的前提(诚实登记)**:`styles.home.css:113-123` 那句「金标整行 ≈565px = 窗口三分之一」是**用 Codex 的窗口宽度**归一化推的,
但 Codex 内容列只占其窗口 39%、我们占 50% —— 拿窗口做基准必然错位;轮25 那条 ✂「卡片几何已收敛 23.3%」则是把卡宽除以**各自的行宽**(自指恒等),
检测不到行宽本身偏窄。**动作**:`styles.home.css:129` 放开 `max-width` 继承 hero 列宽(720),卡片随 `1fr` 涨到 ≈166×84。**只放宽不加高**
(`min-height:84px`/`padding:12px`/12px 标签不动),composer 的 QA-45 钉底不碰。需要新后端:否。

### SC-23 ☐ Scheduled 建 schedule 没有项目选择器:默认丢进一次性 scratch 目录,想跑在真仓库要手打绝对路径 [P2]
`Create ⌄` 与三张 Suggestions 卡开的同一个 `Schedule a task` 弹窗里,Workspace 是**裸文本框**(placeholder `Leave blank for a new scratch
workspace`),旁边 `Use folder…`(`Modals.tsx:334`)点开的是 `openPrompt({title:"Choose workspace"})` —— **又一个手打路径的输入框**。整个建单流程
**没有任何一处列出已有项目**,而 app 自己知道 202 个 workspace,且 `Composer.tsx` 那个带 `Search projects` 的 popover **组件已经写好了**。
默认留空 = 点「Daily brief」建出的周期任务跑在一个空文件夹里对着空气工作。定时任务的价值全部来自「它在我的仓库里持续干活」。
**动作**:`Modals.tsx:331-338` 的 Workspace 行复用 composer 那个已存在的项目 popover(与 HM-9 共用同一份全量 workspace 列表),后端字段不变。需要新后端:否。

### ACT-RED ✅ N 步里有 1 步失败就把**整条聚合活动行**变砖红——全 thread 唯一的饱和色 [P2] — R51 finder live 复验:已修(聚合行 summary 中性灰 rgb(96,96,96)/dark rgb(208,208,219),`.act-group.error` 红规则已无)
`.act-group.error > summary` label = `rgb(192,57,43)`、图标 `rgb(179,38,30)`(`styles.css:2949` + `styles.conv.css:273`)。live 上
`Tracked progress, ran commands 6` 整行是红的,而那 6 步里失败的 2 步是 `ls -la ../../../` / `ls -la ../` 两条被沙箱挡掉的探路命令,
agent 当场换路继续——**这一组工作完全成功**。金标的活动行一律中性灰,状态只落在**单条 shell 块**上(右下角 `✓ Success`)。把「其中一步返回非零」
升级成「这一整段工作坏了」,是把告警色花在没坏的东西上(红色是这屏的稀缺资源,SC-16 刚立过同一条规矩)。
**动作**:删 `styles.css:2949` 与 `styles.conv.css:273`(单条 `.step.error` 自己的红 ✕ 保留),或退成一枚安静的 `2 failed` 计数 chip。需要新后端:否。

### TAIL-ROW ✅ thread 唯一常驻的收尾动作行,悬在最后一轮内容中间(下面还压着 3 个块) [P2] — R51 关闭(goal 判决移到 `.tl-inner` 末尾 turn-footer,在 outcome/changes 卡之后;`b904f1fe`)
`.tl-inner` 顶层子节点序:`msg-last` 的 `.msg-actions`(opacity=1)y=**420** → `worked` y=453 → 产物卡 y=491 → `changes-outcome` y=563 → 终态横幅 → composer。
TH-21 之后**全 thread 仅剩的那一行常驻图标**,落在离 thread 末尾还有 143px + 3 个块的地方,看上去像贴在某条中间消息上。金标里
`⧉ 👍 👎 ↗ │ ⊘ Goal achieved in …` 是**变更卡正下方、composer 正上方**的 thread **收尾行**,不是某条消息的页脚 —— TH-21 把中间消息改 hover-only 之后,
这一行的语义已从「这条消息的操作」变成「这条 thread 的收尾」,它就该长在末尾。**动作**:把末条的收尾行从 `.msg.msg-last` 内部(`Timeline.tsx:877`)
提出来,作为最后一个 turn 的尾节点渲染在 `.tl-inner` 末尾(fold/产物卡/变更卡之后);`goalVerdict` 已是现成入参。不推翻 QA-45 终态 banner,只挪位置。需要新后端:否。

### ENV-GOAL-LABEL ☐ 面板里已结算的 goal 是一条没有组标题的孤儿行 [P3]
`.supervision-panel` 里 `.supervision-label` 只有 1 个(`Environment`);第二个 `supervision-section`(`SupervisionPanel.tsx:299-300`,内容
`Cancelled · TH-3 live check…`)**没有 label**、也没有与上一组的分隔线 —— 而它所有兄弟 section 都有(`Background work`/`Goal`/`Artifacts`/`Agents`),
approval 会话里同一位置就正常写着 `Goal`。金标 Environment 面板每一组都是「dim 组标题 + 1px 分隔线 + 行」,读起来是四段目录。
**动作**:`SupervisionPanel.tsx:299` 补 `<div className="supervision-label">Goal</div>`,并给 `.supervision-section + .supervision-section` 补
`border-top: 1px solid var(--line)`。需要新后端:否。

### ✂ 轮38 finder 排除的假差距(别派)
- ✂ Scheduled 行 `⋯` 触屏不可达:`styles.scheduled.css:261` 有 `@media (hover:none)`,真机 touch profile 实测 `opacity=1` 且命中。
- ✂ Scheduled 不实时刷新:`App.tsx:188-189` 每 4s 轮询。
- ✂ 建议卡是死控件:`Home.tsx:158` → `prefillComposer`,实测点后 textarea 有值、焦点在 TEXTAREA。
- ✂ 未读蓝点与 `⋯` 打架:实测 dot `x=1131-1138`、`⋯` `x=1142-1168`,不重叠。
- ✂ `+` 菜单 / 权限菜单 / 三个 chip popover:已达标甚至超金标。
- ✂ 终态 amber banner 本身(QA-45 + TH-11/TH-14 已收口)、`Worked · N steps` 文案、`Access: set by agent spec` 中性色(TH-17 已撤回)。
- ✂ Environment 头部 `+` / Browser / Sources 分区:无后端(轮35 已核)。
- **已登记不重复**:ENV-6(Worktree 抽屉路径被撕成 7 行 mono,RD-C 补完动作行后更扎眼,建议提 P2)、TH-18(jump-to-latest 圆钮压正文,本轮再现)、MOB-8。

- 2026-07-12 轮38:比对 review/changes-split 屏 × Codex 金标 `codex-diff-review.jpg`(+ home/scheduled/thread 两个 finder 并行取证);
  关差距 **RVW-PHANTOM(P1)** 每份 review 最后一个文件的幽灵空行根除(`splitDiff()` 丢弃 payload 尾部换行,尾部 band off-by-one
  一并归位,+5 单测)、**RVW-CLIP(P1)** scope 标签不再被切("Working tre" → "Working tree",cw/sw 76/97 → 109/109,caret 从按钮外
  886 收回 898≤905,让路的改回 worktree chip)、**RVW-ORDER(P2)** review 开屏第一张卡从「打不开的 binary」变成有内容的文件
  (tracked/untracked 归并一条序、binary 沉底、列表与正文同序)、**RVW-BINCOUNT(P2)** binary 不再印永不兑现的 `+…`、不再重复发
  注定 400 的 blob 请求;派工 5(3 implementer 并发 worktree 隔离 + 2 finder);push `7c63a40` `39451ac` `c9f1bc9`;
  live=`index-DA8imJxu.js`;全景 console error+warning = **0**(home/scheduled/rich/approval/diff × light/dark × 1440/390)。
  截图 `qa/runs/2026-07-12-r38/{before,after,finder-home-sched,finder-thread}/`。
  **BACKLOG:+✅×4(2 P1)、新增 ☐×9**(**ART-SCRIM P0** + ENV-CLIP/HM-9/FOLD-RUN 三条 P1 + HM-10/SC-23/ACT-RED/TAIL-ROW 四条 P2 + ENV-GOAL-LABEL P3)。
  **下轮首选**:**ART-SCRIM(P0)** —— 点变更卡的 `Open in ⌄` 会让**整个 app 变成一块灰色空白**(裸 `<button class="fixed inset-0">`
  吃了 UA 默认的 `buttonface` 底色),改一个 class 就能修,是目前已知最严重的可见故障;并行可打 **ENV-CLIP(P1)**(Environment 面板
  `overflow:auto` 把自己的 `Commit or push` 菜单裁掉 56%,`Commit & push`/`Push` 两个 git 写动作**点不到**)与 **HM-9(P1)**
  (项目搜索框只认 5/202 个 workspace,搜不到侧栏上明明看得见的项目)—— 三条 touches 两两不重叠。

- 2026-07-21 轮39:比对 home/thread/diff/scheduled × Codex 金标(重建 live=index-CR_T__Jw.js 后逐屏并排);
  关差距 **HOME-REPOBOX(P1)** home hero 里 repo 名的幽灵点线矩形框根除(`.home-empty-repo` 的 `border-b border-dotted`
  会把四边都设 dotted、只有底边有显式 1px 宽、其余三边回退 UA `medium`=3px 成框;加 `border-0` 后只剩底部 1px 点线下划线,
  三边 0px,贴 Codex 纯文本 repo 名)、**SCHED-DENSITY(P2)** Scheduled 行从「重边框大卡片(~100px、一屏 ~5 条)」变
  「紧凑分隔清单(59px、细底分隔线、无卡片边框/圆角、透明底、glyph 32→28px,一屏 ~9 条)」贴 Codex 紧凑列表(`⋯`菜单/
  未读点/cadence 全保);派工 1(单 implementer,worktree 隔离,自验自推,仅动 tw.css);push `9e89fd6c`;
  live=`index-CR_T__Jw.js`;全景 console error+warning = **0**(home/scheduled × light/dark)。
  截图 `qa/runs/2026-07-20-r39/{live,after-live}/`。**注**:INC-41-BACKLOG.md 已随 INC-65.2 顺带归档至 docs/archive/,
  旧 backlog 条目多经 348 commit(含 Tailwind 迁移)洗牌,本轮以 live 重截图为准、未沿用旧条目。

- 2026-07-21 00:27 轮40:比对 4 屏(home/rich/scheduled/diff-review × light/dark × 1440/390)+ Codex 金标裁剪对齐。
  home 建议卡对 `codex-crop-newtask-emptystate.jpg` 真像素比对 → 尺寸/布局已一致(判 ✂,前几轮误判为差距);
  关差距 **SCHED-CREATE-BTN**:Scheduled 页 `Create` 按钮渲染畸形——`.menu-trigger` 强制 `w-8`(32px 图标宽),
  `.page-action` 只覆盖高/内边距不覆盖宽,故 pill 被钳到 32px、`Create` 文案连同 Plus/CaretDown 溢出边框外(看着像坏了)。
  加双类覆盖 `.menu-trigger.page-action { w-auto }` → 变回正常 `+ Create ⌄` pill(贴 Codex)。派工 1(inline,仅 tw.css);
  push `b4b03b22`;live=`index-DUF3e1iO.js`;复验 scheduled × light/dark console error+warning = **0**。
  截图 before `qa/runs/2026-07-21-r40/live/sched-header-crop.png`、after `qa/runs/2026-07-21-r40/after/sched-header-crop-light.png`。

- 2026-07-21 00:42 轮41:比对 4 屏(home/scheduled/rich/diff × light/dark × 1440)+ Codex 金标裁剪对齐。
  关差距 **EMPTY-STACK(P0 可见故障)**:`.tl-empty`(NotFound + timeline "No messages yet" 两个真实空态共用)
  只有 `text-center`、无 flex/grid → icon·`<b>`标题·`<span>`正文·CTA 全部 inline 流式排,渲染成
  "Session not foundNo session matches…"、Back 按钮嵌在句子中间(看着像坏了)。镜像已验证正确的兄弟
  `.empty-state`(`grid justify-items-center gap-1.5`)+ 加 `.tl-empty b {text-ink}` 深化标题 →
  两个空态都变干净竖排栈,贴 Codex。
  关差距 **SCHED-TABS(P1 Codex parity)**:Scheduled filter tab 是重蓝描边 pill(蓝字+蓝底+蓝边框),
  Codex 是纯文本 tab(inactive 灰字无边框、active 柔和中性 `panel-2` pill+深字);且 Codex 搜索占整行、
  tabs 在下一行(左 tabs·右 Mark-all)。改 `.sched-toolbar` flex-col 两行 + `.sched-filters` justify-between +
  `.sched-tab`/`.on` 去蓝改中性 + `.sched-markread` 去按钮 chrome 成纯灰文本。
  派工 1(inline,仅 tw.css,串行两条——共享文件单 implementer 规则);**让路** 并发 session(INC-85)未提交的
  specs.ts/Go 脏文件(其 specs.ts:243 反引号未转义炸 vitest transform)——只推我的 tw.css,用 origin/main 干净
  worktree 验证:vitest **602 全绿** + build 绿;push `d77ba3ab`;live=`index-B-munzsx.js`;
  复验 scheduled(tabs 已对齐)+ session-not-found(已竖排)× light/dark,scheduled/home 单屏 console = **0**
  (notfound 屏的 1 个 404 是故意导航不存在会话触发的 API 404,预期非回归)。
  截图 before `qa/runs/2026-07-21-r41/live/{scheduled,diff,rich}-light.png`、after `qa/runs/2026-07-21-r41/after/{scheduled,notfound}-{light,dark}.png`。

- 2026-07-21 01:03 轮42:比对 4 屏(home/rich-thread/scheduled/sidebar-projects × light/dark × 1440)+ Codex 金标裁剪对齐(旧参照会话 ID 全失效,改 UI 点击取有效会话)。
  关差距 **SIDE-SUBTITLE(P2 主对齐面·每屏可见)**:侧栏 `Projects` 同名 workspace 组头的消歧标签(`projectSubtitle` worktree lineage → `project.hint`)原以 `.project-hint { ml-auto }` 右对齐抢横向空间,把 repo 名挤成 "workspa..." 且 hint 换到第二行右对齐;Codex 组头是纯 repo 名完整突出、次要细节从属(`codex-crop-sidebar-projects.jpg`)。把 name+hint 包进 `.proj-heading-text` flex-col 列:名字第一行完整(不再因 hint 提前截断),hint 降为名字下方 text-[10px] text-dim 从属第二行、自身过长才 truncate、绝不抢名字空间;无 hint 组保持单行不变。**修回退**:两行列 flex-1(186px)宽于短名字,暴露 `<button>` UA 默认 `text-align:center` 致名字全部居中(退步),补 `.proj-heading-text text-left` 归正为 Codex 式左对齐清单。
  **顺带解 main 阻塞**:并发 INC-85(ae0f8df4)提交的 `specs.ts:242` 未转义反引号 `` `sleep` `` 提前闭合模板串(TS1005),**坏了 main 上所有人的 `npm run build`**——转义两个反引号(输出文本不变),build 恢复。
  派工 1 implementer(Sidebar.tsx+tw.css,worktree 隔离 clean checkout 自验自推)+ 2 finder 并发(diff/review+approval、mobile390+settings,补下轮 backlog)。
  push `b6591b29`(SIDE-SUBTITLE)、`dd8b1fc2`(build-fix specs.ts)、`9e68095a`(text-left 修回退);vitest **602 全绿** + build 绿;live=`index-CeaFSfIk.js`;复截侧栏 light/dark console error+warning = **0**。
  截图 before `qa/runs/2026-07-21-r42/live/home-{light,dark}.png`、after `qa/runs/2026-07-21-r42/after/sidebar2-{light,dark}.png`。
  **下轮线索**(留给 finder 定性后排):① 富 thread 里 **ChangesOutcome 变更卡渲染成空盒**(只 ± 徽章、无 "Edited N files"/无 +/-/无 Undo·Review,见 `qa/runs/2026-07-21-r42/live/crop-emptybox.png`)——疑 loading skeleton 卡死或 diff 请求失败,非确定性(第二次探测卡消失),需 finder 复现定性(loading-stuck vs data vs 真无变更);② diff/review 分栏屏(最重要屏)因无有效多变更会话本轮仍未可信对标,待 finder 交证。

- 2026-07-21 轮42 finder 收割(diff/review + approval,read-only):
  - **crop-emptybox 定性 = 加载骨架,非 bug(✂ 勿修主逻辑)**:`.changes-outcome` 是 `ChangesShell` loading 相(± 徽章 + `.changes-outcome-skel` 两条 shimmer,`ChangesOutcome.tsx:433-441`);两次 `/diff` 均 200,该会话 workspace(`editable_mermaid2`)非 git 仓库 → last-turn 只含被丢弃的 node_modules lockfile、working-tree `isRepo:false` → `phase=ready && !files.length → return null`,卡最终正确消失。"空盒"是被截到的 3-4s 过渡骨架。
  - ☐ **RVW-SKEL(P2)**:加载骨架读起来像"坏掉的空卡"——只有 ± 徽章 + 极淡 shimmer、**无可见 Loading 文案**(只 aria-label);且 `ChangesOutcome` 串行发两次 `/diff`(last-turn→working-tree),last-turn 含 node_modules lockfile 时 fetch+parse 慢,骨架挂 3-4s。动作:骨架加淡色 "Loading changes…" 占位文本或"文件行"形态 shimmer;可选 working-tree 回退只在 last-turn 判空后才发。`ChangesOutcome.tsx:433-441`。
  - ☐ **RVW-MARKER(P2)**:逐行 diff marker 比 Codex(`codex-crop-diff-rendering.jpg` 细左边条+极淡底色)**更重**——每改动行最左是整行高实心红/绿块;dark 新增行底色**高饱和亮绿**,一屏全绿刺眼。动作:左侧实心块收成细 accent 条 + 降行底色饱和(尤其暗色 diff add/del 背景 token)。截图 `qa/runs/2026-07-21-r42/finder-diff/{edit-diffsplit-light,bar-diffsplit-dark}.png`。
  - ✂ **DIFF-CP(P2 deliberate)**:1440 默认分栏 diff panel <640px→`barTight`→"Commit or push" 压成无标签 `-○-` 圆图标(`DiffView.tsx:817` 阈值,commit `7075e046`)。Codex 同宽给全标签胶囊,可发现性我方差;若面板可加宽/阈值可放宽建议让标签常驻。`diff-header-crop.png`。
  - ✂ P3-1 inline/split 切换 1440 默认隐藏(`DiffView.tsx:386/1035`,deliberate);☐ P3-2 改动行单行号列(Codex 旧/新双列);P3 approval 三钮主次层级可再拉开(内部打磨,无 Codex 参照)。
  - **已达标勿动**:ChangesOutcome 变更卡(`bar-card-light.png` vs `codex-crop-change-card.jpg` 近像素级一致)、语法高亮默认开、"N unmodified lines" 折叠条、逐文件折叠/Expand-all/文件导航/失败保留卡+Retry、approval 屏完成度高。**diff/review + approval 两屏功能层已基本对齐,无 P0。**

- 2026-07-21 轮42 finder 收割(mobile 390 + Settings,read-only):
  **总体:移动端(390)与 Settings 两条战线基本达 Codex parity,无 P0/P1 功能性回退**(经 40+ 轮 + 大量 mobile 专项提交)。横向溢出:home/富thread(mermaid+多agent)/侧栏抽屉/Settings 7分区处处 `scrollWidth==clientWidth` 无溢出;稳态 console 全 0(light+dark);composer +/model/mode 菜单、侧栏 ⋯ popover 全落视口内不裁;Settings 7 分区移动适配良好、分区 nav sticky;代码块 bordered+圆角+Wrap/Copy 与 Codex 一致。**独立复证空盒=ChangesOutcome loading skeleton 非 bug**(t+2/6/10s 已消失)。建议移动端/Settings 打磨**不排 backlog 前列**。
  - ☐ **MOB-BRANCHPILL(P3)**:home/composer @390 的 branch pill 用 `text-ellipsis` 头对齐截断(`Composer.tsx:1227`),长分支 `worktree-agent-a6e7…` 显示成 `worktree-a...`,**恰好隐藏区分度最高的尾部 hash**(pill 仅 107px)。Codex 分支名短(`main`)无此问题。动作:该 pill 改中段省略或保尾(`direction:rtl`+前导省略)让唯一后缀可见。`home-390-light.png`。
  - ☐ **SET-ROWGAP(P3)**:Settings desktop @1440 分区行 label 左/说明右两列,宽屏下相隔~760px(如 Appearance「Theme … Follow the system…」)视线跨度大。动作:说明加 max-width 或左靠。`settings-desktop-light-appearance.png`。
  - ✂ Home@390 内容顶对齐 + composer 下~180px 死白(commit `d238be5e` 刻意保 composer 可见)、suggestion card 竖排 `min-h-[76px]`(`Home.tsx:160` 刻意 mobile)。
  截图 `qa/runs/2026-07-21-r42/finder-mobile/`。

- 2026-07-21 01:33 轮43:比对 diff/review 分栏(最重要屏)× dark/light × 1600 + 移动端 home 390 + Codex `codex-crop-diff-rendering.jpg` 对齐。**一批 3 并发 implementer(worktree 隔离,白名单两两无交集),全落地。**
  关差距 **RVW-MARKER(P2 最重要屏·每屏可见)**:逐行 diff 左侧 marker 原是整条 18px 实心块(`.dl-marker`/`.fd-split .dls-marker.add` `@apply bg-green`;dark=亮薄荷 #6fd398),当整文件全是新增时右栏成一堵刺眼绿墙 + dark 行底 `bg-green-soft`(#16301f)饱和偏高。Codex(`codex-crop-diff-rendering.jpg`)是**极细左侧 accent 竖条 + 极淡行底 tint**。改:(a) marker 收成 3px `linear-gradient(to right, var(--color) 3px, transparent 3px)` 细条(18px 列宽不变、对齐不乱),del 45°斜纹铺满→3px 纯红细条;(b) 新增**专用** token `--diff-add-bg`/`--diff-del-bg`(light/dark 两处成对,dark 取贴近 --bg 的 #12211a/#241514),`.dl.add .dl-text`/del 由 `@apply bg-green-soft/red-soft` 换裸 `background-color: var(--diff-*-bg)`——**不动全局 --green-soft/--red-soft**(glyph/avatar/danger 共用,零波及)。派工 A(仅 `tw.css`,+26/-8);push `6a2ea307`。
  关差距 **RVW-SKEL(P2)**:富 thread「变更卡加载骨架」只有 ±徽章 + 两条淡 shimmer + 仅读屏器 aria-label,屏上无可见「加载中」文案,3-4s 加载期读起来像坏掉的空卡。在 `.changes-outcome-skel` 内、shimmer 前加可见淡色 `<div className="text-[12px] text-dim">Loading changes…</div>`(用 `<div>` 避开 `.changes-outcome-skel span` 的 shimmer 规则,纯 utility 无需改 tw.css)。派工 B(仅 `ChangesOutcome.tsx`);push `315e15e9`。
  关差距 **MOB-BRANCHPILL(P3)**:移动端 390 composer branch pill 用 `text-ellipsis` 头对齐尾截断,长分支 `worktree-agent-…d3d7655a7` 显示成 `worktree-a...`,恰好藏掉区分度最高的尾部 hash。给该 span 加 `[direction:rtl] text-left`(rtl 下从开头截断、保尾),`main` 等短名不受影响。**复验后 pill 现显 `…d3d7655a7`**(尾部 hash 可见)。派工 C(仅 `Composer.tsx`);push `8198886c`。
  三者 touches 白名单两两无交集(tw.css / ChangesOutcome.tsx / Composer.tsx);各自 node24 vitest **602 全绿** + build 绿、自 rebase 自推;dist 未提交。部署 8809 `live=index-CRHwC9o2.js`(200);playwright 复验 diff-split × dark/light + mob-home-390 稳态 console error+warning = **0**。
  截图 after `qa/runs/2026-07-21-r43/after/{diffsplit-dark,diffsplit-light,mob-home-390,thread-dark,thread-light}.png`;before 参照 `qa/runs/2026-07-21-r42/finder-diff/bar-diffsplit-dark.png` + Codex `qa/codex-reference/codex-crop-diff-rendering.jpg`。
  BACKLOG 状态:RVW-MARKER ✅、RVW-SKEL ✅、MOB-BRANCHPILL ✅(本轮 finder 收割的三条 P2/P3 关闭)。剩余开放:DIFF-CP(✂ deliberate)、SET-ROWGAP(P3 settings 行间距)、P3-2 双列行号。

- 2026-07-21 01:45 轮44 finder 收割(3 并发 read-only:rich-thread+env / diff-split / home,均以真实 diff 会话 20260711-011831-…297d 单次 goto 对标):
  **本轮做(implementer A · tw.css only · worktree)**:
  - ✅ **DIFF-MARKER-CTX(P1 回归·最重要屏)**:R43 RVW-MARKER 把 `.dl-marker` 基类默认设成绿渐变(`tw.css:746`),但**漏了把 inline context(无 .add/.del)行 marker 置空**→18 个未改动行左缘全挂绿条,整文件读起来像全新增。split 的 `.dls-marker`(:769-772)写法正确(基类无色、仅 .add/.del 上色)。修:`.dl-marker` 基类背景 transparent + 新增 `.dl.add .dl-marker` 绿细条,保留 `.dl.del` 红。
  - ✅ **ENV-FLAT(P2·最常见屏最大差距)**:Environment/Supervision 面板每行是独立 `rounded-[10px] border bg-panel` 描边盒(共享规则 `tw.css:670-672`)+ `gap-2` + 重 uppercase 标题(:667)+ section `mb-4` 无分隔线 → "一摞断开卡片";Codex 是一张卡内**无边框扁平行 + 发丝分隔线分组**。修(全 tw.css):共享行规则去 border/bg/rounded 改 `border-0 bg-transparent py-1.5 hover:bg-panel-2`(覆盖 base button 边框)、`.supervision-label` 去 uppercase/tracking、`.supervision-section` mb-4→`border-b border-line pb-3`(last 不加线)、`.env-rows/.artifact-list` gap-2→gap-y-0、`.supervision-details`(:1235)加 border-0 bg-transparent。状态靠 `.status-dot` 色不靠 box,扁平化不伤状态。
  **让路下轮(全在 tw.css,与本轮 A 冲突,按"共享文件一轮一 owner"顺延)**:
  - ☐ **HOME-CARD-DENSITY(P2)**:home 空态 4 建议卡 `min-h-[120px] px-5 py-5 justify-between`(`tw.css:376-378`)→卡高约 Codex 两倍且中间大空洞;Codex 紧凑卡(icon 左上 + 2 行标签紧贴)。改 min-h≈64、px-4 py-3、去 justify-between。
  - ☐ **HOME-REPO-UNDERLINE(P3)**:home 标题 repo 名 `.home-empty-repo`(`tw.css:374`)带 `border-b border-dotted` 虚线下划线,像链接/拼写错;Codex 无下划线。删该虚线边。
  - ☐ **HOME-CHIP-FLAT(P3)**:composer chips `.cx-env-control`(`tw.css:397-398`)是描边胶囊 + `.cx-env-strip`(:392)硬分隔线;Codex 无边框平铺。去 chip border/bg、去 strip border-b、放大 gap。**注意**会波及 session composer,需复验不回退。
  - ☐ **HOME-CARD-WIDTH(P3)**:`.home-empty-cards max-w-[690px]`(`tw.css:375`)比 composer `.cx-card max-w-[720px]` 窄 30px 不对齐→对齐 720px。
  - ☐ **DIFF-GUTTER(P3)**:行号 gutter `.dl-no`(`tw.css:750`)`bg-panel-2 + border-r` 灰装订线切断 changed 行底色;Codex 行号与代码同底、红/绿底通铺。去 `.dl-no` 的 bg/border-r 或让 add/del 行号格继承 `--diff-*-bg`。
  **✂ / 达标勿动**:diff split 视图探针确认无横向溢出、双列正常(44 halves);变更卡/artifact 卡/composer(Undo/Review/Open in/Access/model/mic/send)、model 下拉、add 菜单均已对齐 Codex;Environment 面板功能行(Changes/Worktree/Create branch/Commit or push/Background work=Codex Background processes/Artifacts=Sources)后端支撑齐全,仅视觉差;Codex Browser 行(端口扫描)、账户行改身份(SB-12 deliberate + 尾 sha 是 QA 部署信号)、命令面板 uppercase 标签均 out-of-scope/低价值不做。截图 `qa/runs/2026-07-21-r44/{live,finder-home,finder-thread,finder-diff}/`。
  - <2026-07-21 01:58> 轮44:比对 4 屏(home/scheduled/rich-thread+env/diff-split × light/dark,均以真实 diff 会话 297d 单次 goto 修 harness 后可信对标)。**关差距 DIFF-MARKER-CTX(P1 回归)+ENV-FLAT(P2 最常见屏最大差距)**。派工 1 implementer(tw.css only·worktree·vitest602 绿·build 绿·自 rebase 自推)。push:ledger `4f0612d0`、A `5986ac64`(env 面板扁平化 + inline context marker 置空)。live=`index-BC8dap8N.js`(200)。复验:inline context 行 marker 探针 ctxWithBg=0(18 行全 transparent)、add 绿/del 红细条照旧;env 面板 Changes/Worktree/Create branch/Commit or push/Progress/Agents/Run details 全成扁平行+发丝分隔(对标 Codex 一张卡内扁平清单),多 agent 会话 progress/sa 行不回退;thread+diff+multiagent × light/dark 稳态 console error+warning=**0**。截图 before `qa/runs/2026-07-21-r44/live/rich-UNIQUE-light.png`、after `qa/runs/2026-07-21-r44/after/{env-thread,diff,multiagent}-{light,dark}.png`。让路下轮(同 tw.css):HOME-CARD-DENSITY/REPO-UNDERLINE/CHIP-FLAT/CARD-WIDTH、DIFF-GUTTER。

- 2026-07-21 轮45(headless):比对 home 空态(× light/dark × 1440)对标 Codex `codex-new-task-home.jpg`,实测确认 4 条差距(卡高 143.875px / repo 虚线下划线 dotted / cards 690px vs composer 720px 不对齐 / inline diff gutter 灰装订线切断 changed 行底)。**一批 1 implementer(tw.css 独占 owner·worktree·vitest 602 绿·build 绿·自 fast-forward 推)**,串行关 4 条:
  - ✅ **HOME-CARD-DENSITY(P2·最常见屏可见)**:`.home-empty-card` `min-h-[120px]→76px`、`px-5 py-5→px-4 py-3.5`、`justify-between→justify-start`、`gap-3→gap-2.5`。padding 收紧 + 内容顶部聚拢(不再中间大空洞)。复验卡高 143.875→129.9px(129.9 是"Build a new feature…"3 行卡撑起的等高网格,与 Codex 等高行为一致,非 CSS 可再压)。
  - ✅ **HOME-REPO-UNDERLINE(P3)**:`.home-empty-repo` 删 `border-0 border-b border-dotted`。复验 `borderBottomStyle: dotted→none`(repo 名"workspace"无下划线)。
  - ✅ **HOME-CARD-WIDTH(P3)**:`.home-empty-cards` `max-w-[690px]→720px`,与 composer `.cx-card` 左右对齐。复验 `cardsW: 690px→720px`,截图卡片边缘与 composer 边缘齐平。
  - ✅ **DIFF-GUTTER(P3·代码正确+build 验证,未 live 截)**:`.dl-no` 去 `border-r border-line bg-panel-2`→`bg-transparent` + 新增 `.dl.add .dl-no`/`.dl.del .dl-no` 铺 `--diff-add/del-bg`;context 行号变 panel 同底(去灰装订线)、changed 行号与代码同底通铺。**注**:`.dl-no` 仅存在于 **inline** diff 视图(mobile/窄屏默认);split(1440 默认)行号无独立 bg 类、inline 切换 1440 默认隐藏 → 桌面默认屏看不到此变化;两个候选 diff 会话(d8ac/297d)本轮均已从 store 移除(Session not found),未能 live 截 inline diff,验证靠代码 diff 正确性 + vitest602 + build。
  已 grep 确认 `.dls-no` 不存在,未动 split 视图避免引入新差异。
  push:implementer A `75f4b3a2`(tw.css only,+7/-3)。部署 8809 `live=index-B-xS8HI2.js`(200)。复验 home × light/dark 稳态 console error+warning = **0**(diff 屏的 404 是被移除会话的 Session-not-found,良性)。截图 before `qa/runs/2026-07-21-r45/live/home-{light,dark}.png`、after `qa/runs/2026-07-21-r45/after/home-{light,dark}.png`。
  让路下轮(同 tw.css 需另 owner):HOME-CHIP-FLAT(P3·波及 session composer 需谨慎复验)、SET-ROWGAP(P3 settings 行间距)。DIFF-GUTTER 待有真实 inline diff 会话时补 live 截。

- 2026-07-21 轮46(headless):比对 home 空态 composer + settings modal（× light/dark × 1440）对标 Codex `codex-new-task-home.jpg`。实测确认 **HOME-CHIP-FLAT**：顶部 env strip chips（`workspace`/`New worktree`/`worktree-agent-…`）是描边 `rounded-full` 胶囊 + strip 底 `border-b border-line` 分隔线（`tw.css:392,397-398`）；Codex 同位置 chips（`agentrunner`/`New worktree`/`No environment`/`main`）**无边框扁平**、仅 hover 显底。
  **本轮做（主驾驶直接改 tw.css，单 owner，无并行冲突）**：
  - ✅ **HOME-CHIP-FLAT（P3·最常见屏可见）**：`.cx-env-strip` 去 `border-b border-line`；`.cx-env-control, .cx-pill`（env strip chips + 底部 mode/model pill）拆出 `border border-line bg-panel`→`border-0 bg-transparent`、px-[10px]→px-[9px]，hover 仍 `bg-panel-2`；active 去 `border-blue` 保 `bg-blue-soft`。`.cx-deliv`（Queue/Steer 分段开关）**单独保留描边**，两选项仍读作 segments。复验探针：chipBorderW=0px、chipBg=transparent、stripBorderB=0px（light+dark）；home + session composer（297d，底部 Access/model pill 亦扁平、Environment 面板不回退）稳态 console error+warning = **0**。
  push `f4eaffa4`（tw.css only,+12/-3,vitest 602 绿 + build 绿,fast-forward）。部署 8809 `live=index-Cn19v_0L.js`(200)。截图 before `qa/runs/2026-07-21-r46/live/{home,settings-modal}-{light,dark}.png`、after `qa/runs/2026-07-21-r46/after/{home-{light,dark},session-composer-light}.png`。
  **本轮 defer（诚实记录，非划水）**：☐ **SET-ROWGAP（P3）**——settings modal @1440 每行 label 左 / desc 右经 `.rs-row-head justify-between` 撑开（实测 label 右缘 528px、desc 左缘 896px、**368px 空档**）。但 `.rs-row-head` 是**共享类**（`.rs-wt-head` worktrees 行靠 justify-between 放右侧路径+控件、`.rs-archive-title` grid 覆盖），改共享类会破 worktrees 布局；正确修法需新增独立行类或组件级重构，**且无 Codex settings 金标参照** → 留待专门轮/交互 session 带参照再做。

- 2026-07-21 轮47（headless）：比对 6 屏（home / scheduled / thread / diff-review split / composer / sidebar × light/dark × 1440-1600，均以真实会话 297d 单次 goto）对标 Codex 金标 + crop（sidebar-nav/projects、composer、change-card、artifact-cards、diff-header/rendering、message-actions、scheduled-list）。**结论：diff/thread/home/composer/scheduled/artifact/change-card 均已高度对齐 Codex，无 P0/P1 功能回退**（经 40+ 轮打磨）。选定并关闭最清晰的可见差距：
  - ✅ **SIDE-SECTION-LABEL（P3·首要对齐面 sidebar 全屏可见）**：sidebar 段头 `Pinned`/`Projects`（`.section-label`）原 `text-[11px] font-bold uppercase tracking-[0.1em]` → 全大写粗体窄字距的"喊叫式"分隔；Codex（`codex-crop-sidebar-nav/projects.jpg`）用**标题式常规态**安静分隔（`Projects` 首字母大写、font-normal、无 tracking、稍大、dim）。修：仅给 `.section-label` 加独立 override `text-[12px] font-normal normal-case tracking-normal`（共享 base 的 flex/gap/px/py/text-dim 保留；repo 名 `.project-heading` 的 bold 处理**不受影响**，其自带 normal-case override）。**主驾驶直改**（tw.css 单文件·无并行冲突·比派 worktree 快）。探针实证：`textTransform uppercase→none`、`fontWeight 700→400`、`letterSpacing 0.1em→normal`、`fontSize 11px→12px`（light+dark）；scheduled + sidebar 稳态 console error+warning = **0**。
  push `176dfc1e`（tw.css only,fast-forward,vitest 602 绿 + build 绿）。部署 8809 `live=index-CaBFCu1j.js`(200)。截图 before `qa/runs/2026-07-21-r47/live/scheduled-{light,dark}.png`（PROJECTS 大写）、after `qa/runs/2026-07-21-r47/after/sidebar-{light,dark}.png`（Projects 标题式）。
  **新增 ☐ 候选（本轮发现，未做）**：☐ **SCHED-SUGGEST-LABEL（P3）**：Scheduled 内容区 `SUGGESTIONS` 分节标题仍全大写（不同选择器，非 sidebar `.section-label`）；Codex scheduled 该标题为标题式 `Suggestions`。留下轮/带 Codex scheduled 参照做。☐ **DIFF-SEARCH（out-of-scope？）**：Codex diff 工具栏有"搜索 diff 内容"放大镜图标，我方 diff header 无——需新后端/前端搜索能力，判 out-of-scope 除非确认已有 diff 文本搜索。
  - <2026-07-21 02:26> 轮47：比对 6 屏对标 Codex、关差距 SIDE-SECTION-LABEL（sidebar 段头标题式安静分隔）、派工 0（主驾驶直改 tw.css 单文件）、push 176dfc1e、live=index-CaBFCu1j.js

- 2026-07-21 轮48（headless）：比对 3 屏（home / scheduled / thread × light/dark × 1440）对标 Codex 金标（`codex-new-task-home.jpg` / `codex-scheduled.jpg` / `codex-crop-scheduled-suggestions.jpg`）。home（cards/composer chips/icons/底部构图 QA-45 刻意区）已高度对齐、thread 会话 297d 又被从 store 移除（Session not found，良性）。选定并关闭轮47 遗留候选：
  - ✅ **SCHED-SUGGEST-LABEL（P3·Scheduled 屏可见）**：Scheduled 内容区 `.sched-suggestions-title`（`tw.css:862`）原 `text-[12px] font-semibold tracking-[0.04em] uppercase`→喊叫式小标签；Codex（`codex-crop-scheduled-suggestions.jpg`）该标题为**安静标题式 ~15px 中等字重 dim 灰**（无大写无字距）。修：仅改 `.sched-suggestions-title` @apply→`text-[15px] font-medium normal-case tracking-normal text-dim`（独占选择器，`grep` 确认仅 Scheduled.tsx:476 一处引用，无共享类风险）。**主驾驶直改**（tw.css 单文件·比派 worktree 快，同轮47）。探针实证（light+dark）：`textTransform uppercase→none`、`fontWeight 600→500`、`letterSpacing 0.04em→normal`、`fontSize 12px→15px`、`textContent='Suggestions'`；scheduled × light/dark 稳态 console error+warning = **0**。
  push `4f44d019`（tw.css only,fast-forward,vitest 602 绿 + build 绿）。部署 8809 `live=index-DI00J4gG.js`(200)。截图 before `qa/runs/2026-07-21-r48/live/scheduled-{light,dark}.png`（SUGGESTIONS 大写）、after `qa/runs/2026-07-21-r48/after/scheduled-{light,dark}.png`（Suggestions 标题式）。
  **开放 ☐ 剩**：out-of-scope 的 a11y/perf/mob 组（本引擎不派）；DIFF-SEARCH（需新搜索后端，判 out-of-scope）。下轮回「🎯 第一步」重截 live、重对标 Codex，带新鲜眼睛找下一个已有后端功能的可见差距（thread 需先找一个仍在 store 的富会话截图对标 change-card/environment 面板）。
  - <2026-07-21 02:33> 轮48：比对 3 屏对标 Codex、关差距 SCHED-SUGGEST-LABEL（Scheduled Suggestions 标题式安静分隔）、派工 0（主驾驶直改 tw.css 单文件）、push 4f44d019、live=index-DI00J4gG.js

- 2026-07-21 轮49（headless）：**新鲜眼睛全屏重对标**（承认 R45-R48 漂移成 title-case 微修）——从 sidebar 点进真实富会话 `Building an Agent Runtime`（14 files +932 变更）逐屏对标 Codex 金标 thread/change-card/environment/diff-review。派 3 并发 read-only finder（thread结构 / diff分栏 / home+composer+菜单）。**结论：thread（变更卡/Worked-fold/env 面板/artifact 卡）与 home/composer/model下拉/命令面板均已高度对齐，无 P1/P2 结构缺口**（finder-thread 明确建议把预算投 thread 以外）。捞到并关闭 **2 个实质可见差距**（一批 2 并发 worktree implementer，白名单互斥，各自 vitest602+build 绿、自 rebase 自推）：
  - ✅ **DIFF-HEAD-COUNTS（P2·最重要屏 diff/review 全宽度可见 bug）**：diff 逐文件头的 `+59 −0` 计数**悬浮在行中央（~55% 宽）**而非紧贴文件名——Codex（`codex-crop-diff-header.jpg`）是 `docs/DESIGN.md +8 -4` 计数紧跟文件名。根因：`tw.css:749` 共享规则 `.diff-fileitem-path, .fd-path { flex-1 }` 让 diff 头的 `.fd-path` 与同排 `.fd-spacer`(:761,也 flex-1)各吃 50% 余量把 path 撑到中点；而 `DiffView.tsx:270-271` 注释**明确写 `.fd-path` 不该拉伸、`.fd-spacer` 才吸收余量**——CSS 与文档化意图相反。修（implementer A·仅 `tw.css`）：拆开共享规则，`.diff-fileitem-path` 保留 `flex-1`（文件列表 popover 右对齐计数是想要的），`.fd-path` 去 `flex-1` 只留 `min-w-0 truncate`（长路径靠默认 flex-shrink 仍截断）。**复验探针**：计数与文件名间距 gap=**8px**（贴名，之前 ~55% 中央）；light+dark 一致，console 0。push `0a64cae5`。
  - ✅ **ADD-MENU-DEDUPE（P2·移除 parity-theater 假表面）**：composer add(+) 菜单 "Attach Finder" 与上一行 "Files and folders" **onClick 完全相同**（都 `close(); anyRef.click()` 开同一隐藏 file input）——web 无原生 Finder 通道，是抄 Codex 标签没抄能力的假表面，同菜单两行一个行为。契合本项目「绝不做壳/假表面」硬规则，移除之。修（implementer B·`Composer.tsx` 删该 PopItem + 同步删 `Composer.addMenu.test.tsx` 里硬断言 "Attach Finder" 的期望值；`FolderIcon` 另有 3 处引用故保留）。**复验**：add 菜单现 Files-and-folders=1 / Attach-Finder=**0** / Goal=1，只剩 Files and folders·Goal·Plan mode·Automation，console 0。push `39f8a1b1`。
  两者 touches 白名单互斥（tw.css / Composer.tsx+test）。部署 8809 `live=index-tATGomwc.js`(200)。截图 before `qa/runs/2026-07-21-r49/live/diff-dark.png`(计数中央)、after `qa/runs/2026-07-21-r49/after/{diff-header-light,diff-header-dark,add-menu-after}.png`；finder 证据 `qa/runs/2026-07-21-r49/finder-{thread,diff,home}/`。
  **未做/让路**：finder-diff 差距2 DIFF-CP(1440 默认 split 下 Commit-or-push 压成无标签圆图标·`DiffView.tsx:205 BAR_TIGHT_PX=640`)——finder 明确标注该阈值是为修「✕ 挤出面板」精调过的，直接下调有回归风险，需单独增量评估，本轮不顺手改；finder-home model-pill-effort/speed-deadend(刻意/低价值)、finder-thread msg-action 图标对比度(属排除的 a11y 微修)。
  - <2026-07-21 02:5x> 轮49：新鲜眼睛全屏重对标(富会话 Building-an-Agent-Runtime)、3 finder 并发、关差距 DIFF-HEAD-COUNTS(diff头计数贴名) + ADD-MENU-DEDUPE(移除 Attach Finder 假表面)、派工 2(并发 worktree)、push 0a64cae5 + 39f8a1b1、live=index-tATGomwc.js

- 2026-07-21 轮50（headless）：**新鲜眼睛全屏重对标 + 3 并发 read-only finder**（diff-review 分栏 / thread+env+composer / home+菜单+sidebar，均以真实富会话 Building-an-Agent-Runtime 14 files +932 单次点击进入对标 Codex 金标 + crop）。**结论：thread（变更卡/env 面板/composer/model 下拉/add 菜单/message-actions）与 home（空态/chips/命令面板/sidebar 分组）均已近乎完全 Codex 对齐，finder-thread/finder-home 均明确返回"无 P1/P2、只剩 P3"**。捞到并关闭 finder-diff 的**唯一真 P2 可见 bug**（主驾驶直改 tw.css 单文件，比派 worktree 快）：
  - ✅ **DIFF-MARKER-NOTCH（P2·最重要屏 diff/review 全宽度、每个改动行可见）**：diff 行 18px marker cell 的渐变是 `var(--green) 3px, transparent 3px`——只染 3px accent 条、剩 ~15px 透明露出面板底色（light 白 / dark panel-dark），在 accent 条与已染色的 `.dl-no`/`.dl-text`（`--diff-add/del-bg`）之间劈出一道**白槽**，每个改动行读作"一条独立绿线 + 一块分离绿底"而非 Codex 的**整行连续染色**（`codex-crop-diff-rendering.jpg` y=458 金标扫描证一块连续绿底、accent 条嵌入其中）。根因是 RVW-MARKER 把 bar 收成 3px 后尾部留 transparent，与 RVW-1"整行连续染色"自述目标相悖（git 确认残留 bug 非刻意）。修（`tw.css:778/781` inline + `:805/806` split）：渐变尾色 `transparent`→`var(--diff-add-bg)`/`var(--diff-del-bg)`，主题 token 覆盖 light+dark。**before/after 探针**：before marker 3-18px = `rgba(0,0,0,0)` 透明（no/text 已染 light `rgb(231,246,236)`/dark `rgb(18,33,26)`）→ after marker 尾色 == no/text tint，跨行原始像素扫描 marker 区**白像素=0**（light+dark）；diff × light/dark 稳态 console error+warning=**0**。push `9aebb17b`（tw.css only,vitest 602 绿 + build 绿,fast-forward）。部署 8809 `live=index-DzS2Hhsy.js`(200)。截图 before `qa/runs/2026-07-21-r50/live/diff-{light,dark}.png`（白槽）、after `qa/runs/2026-07-21-r50/after/diff-{light,dark}.png`（连续染色）；finder 证据 `qa/runs/2026-07-21-r50/finder-thread/`。
  **新增 ☐ 候选（本轮 finder 发现，未做，留下轮 fresh-eyes 排）**：
  - ✅ **DIFF-SCOPE-CASE（P3）**：diff 工具栏 scope pill `Last turn`（`DiffView.tsx:567`）Codex 为 title-case `Last Turn`（`codex-crop-diff-header.jpg`）。R51 关闭：pill + 两个 PopItem 标签统一 Title Case（`Last Turn` / `Working Tree`，menu 内一致；prose "Last turn unavailable" 保留 sentence-case），同步 6 处测试断言；`886a4f01`。live 实测 pill="Last Turn"、menu=["Working Tree","Last Turn"]。
  - ☐ **DIFF-COLLAPSE-RESIDENT（P3·判断题，反 DF-1）**：Expand/Collapse-all 现埋在 `…` popover（`DiffView.tsx:837`），Codex 工具栏把折叠图标作常驻 icon（宽面板现已常驻 Copy+Wrap，有位置）。反转 DF-1 决策，需评估非清晰 bug。
  - ☐ **HOME-CARD-WRAP（P3）**：home 空态第 2 卡"Build a new feature, app, or tool"@1440 换 3 行（Codex 2 行），读起来更挤更高。属已落 CARD-WIDTH/DENSITY territory，需先探 line-count 再调（`tw.css:384-388`），别盲调。
  - ☐ **THREAD-ACTION-FOOTER（P3·可能被后端阻塞）**：末条消息操作行现在 prose 后、outcome 卡前渲染；Codex 作 turn footer 在卡之后。清关需 per-turn 卡后端（已排除）→ 大概率阻塞，仅记完整性。
  **out-of-scope 复确认**：DIFF-SEARCH（需新搜索后端）、richer scope（Staged/Commit/Branch 无后端）、per-line hover `+` 注释、composer context-window ring/credits（journal 无 provider 容量+无计费模型，`INC-41-BACKLOG.md:433`）、👍/👎 feedback（无后端）、sidebar Plugins/Sites/PR/Chat（Codex-only 后端）。
  - <2026-07-21 03:1x> 轮50：新鲜眼睛全屏重对标(富会话 297 变更)、3 finder 并发(diff/thread/home)、关差距 DIFF-MARKER-NOTCH(diff 行连续染色·消白槽)、派工 0(主驾驶直改 tw.css 单文件)、push 9aebb17b、live=index-DzS2Hhsy.js

- 2026-07-21 轮51（headless）：**新鲜眼睛全屏重对标 + 3 并发 read-only finder**（diff-review / thread+env / home+sidebar），并**重点复验挂账多轮的 P1/P2**（怀疑 R45-R50 漂进 title-case 微修而漏掉真 P1）。**最大结论：所有挂账 P1/多条 P2 早已被修，只是 ☐ 标记过期未更**——finder live 逐条复验并本轮改标 ✅：**ENV-CLIP(P1)**（`Popover.tsx:191` fixed 定位逃出 overflow，3 写动作可点）、**HM-9(P1)**（全量 285 workspace 去重+搜索，搜 qa57→含 qa57-browser、agentrunner→189）、**FOLD-RUN(P1)**（`timeline.ts:492-577` 重写 foldRuns，40 step→1 组、裸 step=0）、**HM-10/ACT-RED/RVW-SKEL/DIFF-GUTTER/HOME-CARD-DENSITY/HOME-CARD-WIDTH/HOME-REPO-UNDERLINE/HOME-CHIP-FLAT** 均 live 复验已修。捞到并关闭本轮 **2 个仍开放的实质差距**（一批 2 并发 worktree implementer，白名单互斥，各 vitest 602 + build 绿、自 rebase 自推）：
  - ✅ **TAIL-ROW（P2·thread 最常用屏结构差距）**：末条 assistant 的 goal 判决徽章("Goal achieved in Xs")渲染在 `.msg.msg-last` 的 `msg-actions` 行内，而这行之后还压着 worked-fold/artifact/changes-outcome 三块 → 判决徽章悬在末轮内容中间（finder 实测 msg-actions y=290、其后 3 块到 y=466）。Codex（`codex-task-thread.jpg`）把判决作 turn footer 放在该 turn 全部内容（含 change 卡）之后。修（implementer A·`Timeline.tsx`+`tw.css`）：`MsgActions`/`Item` 删 goalVerdict prop 与其分支（含 `.msg-actions-div`+`.msg-goal-verdict`），保留 copy/share/continue/timestamp；在 `.tl-inner` 末尾 `outcomeSlot` 之后新增 `.turn-footer`（CheckCircle fill + "Goal achieved in {elapsed}"，门控 `!active && !typing && pending.length===0 && goalVerdict`）；`tw.css` 仅追加 `.turn-footer` 规则。TH-10 测试按新结构重写。**live 复验**（goal-achieved 会话 `20260713-070200…inc66`）：turn-footer="Goal achieved in 00:18" 渲染在末条消息（y=488）**之后**（y=710）、`goalVerdictInsideActions=false`（已移出 msg-actions），DOM 顺序结构性保证在 change 卡之后，light+dark console 0。push `b904f1fe`。
  - ✅ **DIFF-SCOPE-CASE（P3）**：diff 工具栏 scope pill/菜单 sentence-case `Last turn`/`Working tree` → Codex title-case（`codex-crop-diff-header.jpg`）。修（implementer B·`DiffView.tsx`+3 测试文件）：pill(`:567`)+ 两个 PopItem 标签统一 Title Case `Last Turn`/`Working Tree`（menu 内一致），prose "Last turn unavailable" 等散文保留 sentence-case；同步 6 处测试断言。**live 复验**（diff 会话点 Review 开分栏）：pill="Last Turn"、menu=["Working Tree","Last Turn"]，light+dark console 0。push `886a4f01`。
  两者 touches 白名单互斥（Timeline.tsx+tw.css / DiffView.tsx+DiffView 测试）。部署 8809 `live=index-DyGWYeyW.js`(200)。截图 after `qa/runs/2026-07-21-r51/after/{turnfooter-,scope-pill-,scope-menu-}{light,dark}.png`；finder 证据 `qa/runs/2026-07-21-r51/finder-{diff,thread,home}/`。
  **仍开放 ☐（下轮 fresh-eyes 排）**：RVW-HUNKBAND(P2·判断题·风险最高·单独轮)、HOME-CARD-WRAP(P3·卡高/空洞根因·`tw.css:386`·需先探 line-count)、DIFF-GAP-CENTER(P3·新·折叠带计数居中→左对齐·`tw.css:823`)。后二者本轮因 `tw.css` 归 implementer A 独占（TAIL-ROW）而让路到下轮。
  - <2026-07-21 10:5x> 轮51：新鲜眼睛全屏重对标 + 复验挂账 P1（发现 ENV-CLIP/HM-9/FOLD-RUN 等早已修·改标 ✅）、3 finder 并发、关差距 TAIL-ROW(goal 判决移到 turn footer·change 卡之后) + DIFF-SCOPE-CASE(scope 标签 Title Case)、派工 2(并发 worktree)、push b904f1fe + 886a4f01、live=index-DyGWYeyW.js
  - ✅ **HOME-CARD-WRAP（P3）**：home 新任务空态 4 建议卡 label 用 `text-[15px]`（`tw.css:.home-empty-card`），最长的第 2 卡「Build a new feature, app, or tool」在 166px 卡宽下 ragged-wrap 成 **3 行**、其余卡 2 行 → grid 不齐、卡高 130px 比 Codex（`codex-new-task-home.jpg` 紧凑 2 行）高。修（单 implementer 内联·`tw.css` 仅 `.home-empty-card` 一处字号 15px→13px + 追加 HOME-CARD-WRAP 注释块）：label 降到 13px。**live 复验**（8809 index-ael979AY，playwright 量测）：4 卡全部 2 行、font-size=13px、卡高 130→104px、console error+warning=0。push `d4ab3581`。
  **本轮对标结论（诚实记录）**：新鲜眼睛全屏重截 home/thread/diff/scheduled × light/dark 并逐屏并排 Codex 金标——thread(change-card/goal-footer)、diff(header/change-card)、scheduled(Suggestions 已 Title Case) 均高 parity；**diff dark 逐像素采样 bg=#12211a（RVW-MARKER 已修的 subtle wash，非高饱和），无回退**。真差距落在 home 卡密度。
  - <2026-07-21 04:0x> 轮52：新鲜眼睛全屏重对标 4 屏×2 主题（自截 live+并排 codex-reference）、关差距 HOME-CARD-WRAP(卡 label 15px→13px·card2 三行→二行·卡高 130→104px 贴 Codex 密度)、派工 1(tw.css 内联·单文件无并发冲突)、push d4ab3581、live=index-ael979AY.js

- 2026-07-21 轮53（headless）：**新鲜眼睛全屏重对标 + 3 并发 read-only finder**（diff / thread / home）。**关键排障**：任务惯用会话 `297d`/`d8ac`/`6d0d` 直 hash 导航全报 "Session not found"——发现 SPA hash 路由是 **`#<id>`** 而非 `#/session/<id>`（R51/R52 可能受此误导）；改用存活富会话 `20260713-082616-session-6d0d93bfcf9daa38`（"Building an Agent Runtime" 14 files +932）重截 8 屏逐屏对标。四屏（home/thread/diff/scheduled）经 52 轮打磨均高 parity。捞到并关闭 **2 个实质可见差距**（主驾驶直改 tw.css 单文件·比派 worktree 快，同 R47/48/50/52 模式）：
  - ✅ **GAP-WORKED-BORDER（P1·thread 最常用屏、每个 turn 可见）**：折叠头 "Worked · N steps ›" + 中性状态 chip（"Mode changed · acceptEdits"）从共享带框规则（`tw.css:571`）继承 `rounded-12 border bg-panel px-3 py-2` → 长 thread 读作"一摞卡片"压过答案 prose；Codex（finder cx-worked-crop + `codex-task-thread.jpg`）渲染成**无框安静灰文本行**。修：`.chip, .worked-row` 从共享规则拆出降为 `my-2 text-[13px] text-dim`；`.worked-row` 是 `<button>`——移除显式 border 后**露出 UA 原生 button chrome**（探针实证 border 1px/bg buttonface/pad 5px 来自浏览器默认，非任何 author 规则），补 `border-0 bg-transparent p-0 text-left`（同 `.msg-copy` 既有重置模式）；`.chip.warn/.err` 保留彩框卡片（告警，Codex 也框）。**live 探针复验**（index-CinDMQNd）：worked-row border 0 / bg transparent / pad 0 / text-dim（light+dark）、neutral chip border 0、warn/err chip 仍 border 1px radius 12px；截图 before `qa/runs/2026-07-21-r53/finder-thread/thread-light-top.png`（白卡）、after `qa/runs/2026-07-21-r53/after/thread-top-{light,dark}.png`（灰字无框）。
  - ✅ **MENU-SECTION-LABEL-CASE（P2·composer +/env-chip 菜单可见·补一致性）**：菜单段头 "ADD"/"ADVANCED"/"START IN" 仍全大写字距（`.pop-section-label` @ `tw.css:447`，与 `.cx-slash-hd` 共享）——正是 R47/R48 已给 sidebar/scheduled 消过的喊叫式，此处漏了 → 内部不一致。修：拆开 `.cx-slash-hd`（slash 菜单保持）/ `.pop-section-label` 独立给 R47 治法 `text-[12px] normal-case tracking-normal`。**live 探针**：菜单 label "Start in" `text-transform:none / letter-spacing:normal / 12px`。
  两处 tw.css 单文件·vitest 602 绿 + build 绿·fast-forward。push `1663ba19`。部署 8809 `live=index-CinDMQNd.js`(200)。thread×light/dark + 菜单稳态 console error+warning=**0**（全景 6-context 扫描脚本超时属 tooling，改动只触 thread 折叠行/chip + 菜单标签两处，已定点验证）。
  **下轮 fresh-eyes 候选（本轮 finder 发现，未做）**：
  - ☐ **DIFF-GAP-CENTER（P2·最高价值·每个 modified diff 都犯·一行改）**：fold-band "N unmodified lines" 计数被居中在 code 列（`.fd-gap` 是 button，UA `text-align:center` 未覆盖），Codex 左对齐落在 code 列起点。修 `tw.css:845` `.fd-gap-label` 加 `text-left`（连带 P3-1 DIFF-GAP-CARET 一并）。
  - ☐ **RVW-HUNKBAND（P2）**：hunk @@ 上下文另起独立 `.dl-hunk` 灰带，与折叠带堆成**双灰带**；Codex 一个 gap 一条带。修 `DiffView.tsx:1621-1640` 把 @@ 上下文并入折叠带（别删上下文）+ `tw.css:819`。
  - ☐ **DIFF-SPLIT-ADDED（P2·边角但触发即内容全不可见）**：split 视图纯 added 文件恒空删除列仍占 1fr=1408px，新增代码被推到 x=2626 视口外。修 `DiffView.tsx:1580` split 分支塌陷空列 / 回退 inline + `tw.css:827`。
  - ☐ **SIDE-PROJECT-CARET（P2）**：项目行常驻 caret，Codex 静止只显 folder（代码自己 SB-6 注释就说 caret 应 hover-only 却没实现）；正确修法要 caret/folder 共用 overlap slot（fiddly，专门 implementer 做对，别 naive opacity 留空档）。`tw.css:326` + `Sidebar.tsx:496`。
  - ☐ **NAV-KBD-BADGE（P3）** / contested **HOME-COMPOSER-CENTERED**（QA-45 争议，需真人裁）：本轮 defer。
  - <2026-07-21 04:2x> 轮53：修正失效会话 ID（发现 hash 路由 #id）、新鲜眼睛全屏重对标（存活富会话 6d0d）、3 finder 并发、关差距 GAP-WORKED-BORDER（thread 折叠头/chip 无框安静灰行）+ MENU-SECTION-LABEL-CASE（菜单段头 title-case）、派工 0（主驾驶直改 tw.css 单文件）、push 1663ba19、live=index-CinDMQNd.js

- 2026-07-21 轮54（headless）：**新鲜眼睛对标 diff/review 屏**（存活富会话 `Building an Agent Runtime` 14 files +932）+ 关闭上轮 seed 的 2 个 diff P2（一批 2 并发 worktree implementer，白名单互斥 tw.css / DiffView.tsx，各 vitest + build 绿、自 rebase 自推）：
  - ✅ **DIFF-SPLIT-ADDED（P2·diff/review·触发即新增代码全部滚出视口不可见）**：split（side-by-side）视图对纯新增文件（`parsed.status==="added"`）仍渲染两列等宽网格（1fr 1fr），左（旧代码）列全空但占 ~1408px，把右列真正新增代码推到 **x=2441（视口 1600 外）**——用户只见空白左栏、须横滚才看到任何新代码；纯删除文件对称右列空。**live 铁证**（split 前）：`.dls-half` half0 x=996 w=1408 text=''、half1 x=2441 才是 `+# Agent Runtime…`。Codex 对单侧文件渲染单列。修（implementer B·`DiffView.tsx:1587`+`DiffView.review.test.tsx`）：split 守卫改 `if (effView==="split" && parsed.status!=="added" && parsed.status!=="deleted")`——纯 added/deleted 即使选 split 也回退下方 inline 单列路径（内容从左起可见），modified 不受影响仍 split；纯逻辑改动**无新 CSS**。新增 2 用例（added 切 split 断言无 `.dls`/有 `.dl` 单列且文本可见；modified 切 split 仍 `.dls`）。**live 复验**（index-wd_VAtMS）：全 added 会话 split 现 `.dls`=**0**、`.dl` 单列=933、代码首列 x=**996（视口内**，before 2441 出视口），light+dark 一致 console 0。push `5429e9a2`（vitest 604 绿）。
  - ✅ **DIFF-GAP-CENTER（P2·最重要屏·每个含折叠带的 modified diff 都犯·一行改）**：折叠带「N unmodified lines」计数标签 `.fd-gap-label` 被居中在 code 列——根因 `.fd-gap` 是 `<button>`（DiffView.tsx:1565·UA `text-align:center`）+ `display:grid`，`.fd-gap-label`（`tw.css:849`）无 text-align → 文字继承 button 的 UA center 推到 1fr code 列正中；Codex（`codex-crop-diff-rendering.jpg`/`codex-diff-review.jpg`）左对齐落在 code 列起点。修（implementer A·`tw.css` 仅 `.fd-gap-label` 加 `text-left`+注释块）：`@apply cursor-pointer px-2 py-2 text-left hover:text-ink;`，不动网格/caret/px-2 py-2。**复验**：部署 CSS bundle（index-ClxLta0q.css）实测 `.fd-gap-label{…;text-align:left}`（before 无 text-align 继承 center）；vitest 602 绿覆盖折叠带渲染。**诚实记录**：本轮 live 存活富会话皆为**纯新建文件**（无 unmodified 折叠带、`.fd-gap`=0），无法截 rendered before/after 折叠带——GAP-CENTER 由代码 + R53 finder + 部署 CSS 规则 + 单测三方确认；下轮若遇修改已有文件的会话补一张 rendered 折叠带 after 图。push `ff67848f`。
  两者 touches 白名单互斥（tw.css / DiffView.tsx+其测试）。部署 8809 `live=index-wd_VAtMS.js`(200)。截图 before `qa/runs/2026-07-21-r54/live/split-added.png`（新增代码 x=2441 出视口）、after `qa/runs/2026-07-21-r54/after/split-added-{light,dark}.png`（单列内容从左起可见）。diff×light/dark 稳态 console error+warning=**0**。
  **仍开放 ☐（下轮 fresh-eyes 排）**：RVW-HUNKBAND(P2·@@上下文与折叠带堆双灰带·`DiffView.tsx`+`tw.css` 需同 implementer)、SIDE-PROJECT-CARET(P2·项目行常驻 caret·`Sidebar.tsx`+`tw.css` fiddly 需 overlap slot)、DIFF-GAP-CARET(P3·折叠带 caret 对齐)。三者本轮均因与已派 implementer 的 tw.css/DiffView.tsx 白名单冲突而让路。
  - <2026-07-21 04:4x> 轮54：对标 diff/review 屏（富会话 +932）、关差距 DIFF-SPLIT-ADDED（纯单侧文件 split 回退单列·内容不再出视口）+ DIFF-GAP-CENTER（折叠带计数左对齐）、派工 2（并发 worktree·白名单互斥 tw.css/DiffView.tsx）、push ff67848f + 5429e9a2、live=index-wd_VAtMS.js

- 2026-07-21 轮55（headless）：**新鲜眼睛对标 diff-rendering + sidebar-projects 两屏**（逐屏并排 `codex-crop-diff-rendering.jpg` / `codex-crop-sidebar-projects.jpg`），关闭上轮 seed 的 2 个 P2（一批 2 并发 worktree implementer，白名单互斥 `DiffView.tsx`+tests / `Sidebar.tsx`+`tw.css`，各 vitest+build 绿、自 rebase 自推）：
  - ✅ **RVW-HUNKBAND（P2·diff/review 最重要屏·每个含折叠 gap 的 modified diff 都犯）**：一个被折叠的 gap 处堆了**两条灰带**——先 `.fd-gap` 折叠带（"N unmodified lines"），紧接着又渲染一条 `.dl-hunk` header 带（`@@` context 非空时是 context 文本、空时是 `.dl-hunk-blank` **空白灰带**），两条都 `bg-panel-2` 叠一起读作"重复灰带"；Codex 金标（`codex-crop-diff-rendering.jpg`：`∧ 1200 unmodified lines` / `⇕ 8 unmodified lines`）每个 gap **只一条**灰带、无独立 `@@` 带。修（implementer A·`DiffView.tsx`+`DiffView.chrome.test.tsx`，**未动 tw.css**）：给 `band()` 加可选第 4 参 `context`，把 `@@` 后上下文以 `.fd-gap-context`（`ml-2 text-[11px] opacity-70` 纯内联 Tailwind）塞进折叠带 `.fd-gap-label` 内计数之后；主循环里 `bandEl` 存在时把 `header` 置 null（`const header = bandEl ? null : …`）→ 有 gap 的 hunk 恰好一条 `.fd-gap` 带，绝不再堆 `.dl-hunk`/`.dl-hunk-blank`。无 fold band 的独立 hunk 保留 `.dl-hunk` 分隔不变；tail(RD-2)本无后随 header；split 不涉 fold band 均未动。新增 2 用例断言（单带 + `.fd-gap-context` 有文本 + 折叠带 wrapper 内无 `.dl-hunk`）。**验证**：vitest 606 绿（含 2 新）+ build 绿；部署 bundle（index-CMLz9Ugs.js）实测含 `.fd-gap-context`。**诚实记录**：live 现存活会话皆纯新建文件（`.fd-gap`=0、无 unmodified 折叠带），无法截 rendered before/after 折叠带 diff——RVW-HUNKBAND 由代码 + 2 新单测 + 部署 bundle 三方确认（同 R54 GAP-CENTER 处境）；下轮遇修改已有文件的会话补一张 rendered 折叠带 after 图。push `829e1de1`。
  - ✅ **SIDE-PROJECT-CARET（P2·sidebar 每个 Projects 行常驻可见）**：项目行**同时常驻** caret（`CaretRight`）+ folder 两个相邻图标（live 探针实测 8 caret + 8 folder 全可见）；代码 SB-6 注释自称 caret 应 hover-only 共用 overlap slot 却**从未实现**（`.proj-caret` 仅 `transition-transform`、`.proj-folder` 仅 `shrink-0 text-dim`，就是并排）；Codex 金标（`codex-crop-sidebar-projects.jpg`）静止**只显 folder**、caret 仅 hover/focus 现。修（implementer B·`Sidebar.tsx`+`tw.css`）：caret+folder 包进固定 16×16px `.proj-icon-slot`（`position:relative`），两图标 `absolute` + `-translate-1/2` 居中叠放，opacity cross-fade（默认 folder=1/caret=0，行 `:hover`/`:focus-visible` 反转），caret 保留 `.open` rotate-90；共占同一固定槽 → 名字文本列不随图标切换左右跳、与嵌套 session 标题缩进对齐（SB-6 原意）。**live 探针复验**（index-CMLz9Ugs）：REST caret_op=0/folder_op=1、HOVER caret_op=1/folder_op=0（light+dark 一致），console 0。push `47553aa8`。截图 before `qa/runs/2026-07-21-r55/before/sidebar-caret-folder-both.png`（双图标）、after `qa/runs/2026-07-21-r55/after/sidebar-folder-only-{light,dark}.png`（静止仅 folder）。
  两者 touches 白名单互斥（DiffView.tsx+tests / Sidebar.tsx+tw.css）。部署 8809 `live=index-CMLz9Ugs.js`(200)。全景 console：home+diff × light/dark 稳态 error+warning=**0**、sidebar × light/dark=**0**。
  **仍开放 ☐（下轮 fresh-eyes 排）**：DIFF-GAP-CARET(P3·折叠带 caret 对齐)、DIFF-COLLAPSE-RESIDENT(P3·判断题·Expand/Collapse-all 常驻 vs 埋 popover)、HOME-COMPOSER-CENTERED(QA-45 争议·需真人裁)。**out-of-scope 复确认**：DIFF-SEARCH/richer scope/per-line hover `+`/composer credits/👍👎/Plugins·Sites·PR·Chat 均无我方后端。
  - <2026-07-21 04:0x> 轮55：新鲜眼睛对标 diff-rendering + sidebar-projects、关差距 RVW-HUNKBAND（gap 合成单条折叠带·消双灰带·context 并入 label）+ SIDE-PROJECT-CARET（项目行静止仅 folder·caret hover/focus 现·共用 overlap slot）、派工 2（并发 worktree·白名单互斥 DiffView.tsx/Sidebar.tsx+tw.css）、push 829e1de1 + 47553aa8、live=index-CMLz9Ugs.js

- 2026-07-21 轮56（headless）：**新鲜眼睛全屏重对标 home/thread/diff × light/dark + 3 并发 read-only finder**（thread+env / diff-review / home+composer）。主驾驶自对标先确认 change 卡结构 / Review 描边按钮（base `button`=rounded-8 描边 pill）/ 文件行 dir 路径 dim（`var(--dim)`）均已 parity。捞到并关闭 **3 个实质可见差距**（一批 3 并发 worktree implementer，白名单两两互斥 `tw.css` / `timeline.ts` / `components/Timeline.tsx`，各 vitest+build 绿、自 rebase 自推）：
  - ✅ **COMPOSER-FOCUS-SEAMLESS（P1·每次进 home 都可见·最刺眼）**：home composer 落地即 autofocus，textarea 外漏出一圈蓝色 `ring-2` box-shadow（圆角方框），看起来像卡片里嵌了个搜索框；根因全局 `tw.css:238 input/textarea/select:focus { ring-2 ring-blue/30 }` 泄漏到本意无边框的 composer textarea（`tw.css:427-428 .cx-input-wrap textarea{ border-0 bg-transparent p-0 outline-none }` 没重置 ring），来自 tailwind 迁移非 composer 决策。Codex（`codex-crop-composer.jpg`）composer 无缝、focus 无任何 field box。修（implementer A·`tw.css` 仅追加 1 条 scoped 覆盖）：`.cx-input-wrap textarea:focus,:focus-visible { border-0 shadow-none ring-0 outline-none }`。**live 探针复验**（index-BUKWigjY）：textarea focus computed box-shadow 全 `0px 0px 0px 0px`（before 2px 蓝 ring）；Settings 等普通 input 的全局 ring **不受影响**（不在 `.cx-input-wrap` 内）。
  - ✅ **COMPOSER-CHIP-GROUP（P2）**：composer chip 条被拉裂——`workspace` chip 贴左 → 中间大片空白死点击区 → `New worktree`/`worktree-agent-…` 被推到右缘；根因 `tw.css:413 .cx-env-project-wrap{ flex-1 }`（来自 e0b41582 移动端长名截断，代价是桌面拉裂）让 project chip 抢占全部剩余宽度。Codex 四 chip 自然宽、紧贴、成组左对齐。修（implementer A·同 `tw.css`）：`.cx-env-project-wrap` 的 `flex-1` → `min-w-0 max-w-[240px]`，`.cx-env-control{ w-full }` 保留让长名仍 truncate，移动段媒体查询未动。**探针**：`.cx-env-project-wrap` 宽 314.8px→98px（chip 组左对齐、右侧留白），移动 390 无横向溢出。A push `8a7f2daf`（tw.css +8 −1，vitest 606 绿）。
  - ✅ **THREAD-TURN-FOLD-SPLIT + MODE-LABEL（P2·thread 最顶最先入眼）**：同一个 human turn 顶部堆了三行 `Worked · 5 steps ›` + `Mode changed · acceptEdits (user)` + `Worked for 6m 52s ›`——根因 `timeline.ts:1023` 的 `mode_changed` 走**裸 chip** 顶到 top-level 并 `flush()`，把一个 turn 的 work fold 从中间劈开：前半段 flush 时 durationMs undefined 于是 `workedLabel` fallback 成 step-count、后半段的 `6m 52s` 是整 turn 时长却只盖后半步骤（自相矛盾），且违背代码自申明的 "one turn ⇒ one fold"（`timeline.ts:386`）；同屏 composer pill 写 "Auto-accept edits" 这里却印内部枚举 `acceptEdits (user)`。Codex 一个 turn 只一行 `Worked for <dur>`、mode 变更折在内。修（implementer B·`timeline.ts`+`timeline.test.ts`，未碰 Timeline.tsx）：`mode_changed` 比照 `spec_changed` 改走 `sysChip`（foldable、TH-16 carry-forward、不 flush）；内联 `modeChipLabel()`（镜像 specs.ts ACCESS_LEVELS / inspectPresentation modeLabel：`acceptEdits→Auto-accept edits`、`bypass/full→Full access`、`plan→Plan · read-only`、`default→Ask`）+ `modeCauseLabel()`（`user→by you`、其余→automatic），文案变 `Mode changed · Auto-accept edits · by you`。**单测**（新增回归）：含 mid-turn mode_changed 的 turn 只产生**一个** fold（`["user","fold","assistant"]`）、`durationMs===412000`（非 step-count fallback）、chip 在 fold 内且不含裸枚举、无 chip 浮 top-level；vitest 608 绿。B push `a93088a7`。
  - ✅ **TAIL-ROW-ACTIONS（P3）**：末条 assistant 答复的操作行(copy/share/continue)内联渲染在 README artifact 卡 + Edited 14 files 卡**之前**、turn 中部，而 R51 已把完成态 goal 挪到 turn 最底；于是收尾操作被产物卡腰斩，用户读完卡还要回滚到卡上方找 copy/continue。Codex（`codex-crop-message-actions.jpg`）操作图标与 goal 同在 turn 全部产物卡**之后**的底行。修（implementer C·`components/Timeline.tsx`+测试，未碰 timeline.ts/tw.css，样式全内联 Tailwind）：`Item` 加 `deferActions` prop（末条 settled 时 bubble 内不渲染内联 MsgActions）；`.tl-inner` 底部在 `outcomeSlot` 之后新增 `.tl-tail-row` flex 行 = 末条 MsgActions（左）+ 分隔竖线 + 既有 turn-footer goal（右），门控 `settled && (lastAssistant||goalVerdict)`（没 goal 也渲染操作行在底部），run active 时保持内联持久行（TH-21 不变）。**单测**（新增 5 例 Timeline.tailrow.test.tsx）：末条+产物卡时 tail-row 的 `.msg-actions` DOM 顺序在 `.changes-outcome` 之后、goal 同底行、`.msg-last` 内不再有内联 msg-actions、中间 answer 内联 msg-actions 保持、active 时无 tail-row；vitest 613 绿。**live 探针复验**：`tailRow=True hasCard=True tailHasActions=True actionsAfterCard=True`。C push `85ebf2c2`。
  三者 touches 白名单两两互斥。部署 8809 `live=index-BUKWigjY.js`(200)。home+thread × light/dark 稳态 console error+warning=**0**。截图 before `qa/runs/2026-07-21-r56/live/{home,thread,diff-review}-*.png`、after `qa/runs/2026-07-21-r56/after/{home,thread,tail-row-crop}-*.png`；finder 证据 `qa/runs/2026-07-21-r56/finder-{thread,diff,home}/`。
  **仍开放 ☐（下轮 fresh-eyes 排·diff finder 本轮 seed）**：
  - ✅ **DIFF-SPLIT-TOGGLE-GONE（P2·最高价值·功能可达性缺陷）**〔R57 关闭〕：tight bar(`BAR_TIGHT_PX=640`，diff 面板 42vw → 1280–1512 全 <640 进 tight)下 inline/split 切换既不渲染也不进 `…` 菜单——用户在主流笔记本宽度**完全无法切到 split**。修（implementer A·`DiffView.tsx` only，+30/−2）：①解耦 `effView`（:388 `narrow||barTight?"inline":view`→`narrow?"inline":view`，只有窗口 ≤900 才强制 inline，barTight 中等面板尊重用户显式 `view`，默认 `view="inline"` 故只在主动切 split 时才改渲染、无被动回归）；②tight `…` 菜单(紧邻 Wrap/Copy 降级项)补单项 toggle PopItem，条件 `barTight && !empty && !narrow`（镜像 resident 按钮 `disabled={narrow}`），像 Wrap 一样指向另一视图（inline 时 `Split view`/`Columns`、split 时 `Inline view`/`Rows`）。**live 探针复验**(index-OS0gix6K)：1280 diff `…` 菜单含 `Split view` 项(与 Refresh changes 并列)，点击后菜单翻转成 `Inline view`(证 setView 生效、effView→split)；本会话全 added 文件故不显双列(:1595 added/deleted 单列渲染为正确行为)；新增 `DiffView.viewtoggle.test.tsx` 3 例(barTight+!narrow 菜单含项+点击 fd-split 渲染、narrow 不含项+恒 inline)；vitest 616 绿。A push `75c06eca`。
  - ✅ **DIFF-COMMIT-LABEL-1440（P2）**〔R57 关闭〕：主 CTA「Commit or push」在 1280–1512(含最主流 1440)退化成无标签的 `<GitCommit>` 点(barTight 640 阈值对 42vw 面板过激)。修（implementer B·`tw.css` only 单行）：`.session-layout.changes` 面板份额 `42vw→46vw`（:561，1440→662px>640 越阈恢复完整工具栏含 commit 标签+resident split toggle，46vw 留 22px 余量避 sub-pixel 抖动；1512→695 亦恢复；1280/1366 仍 <640 留后续）。**live 探针复验**：1440/1512 `commitLabel=1 residentToggle=1 consoleErr=0`。B push `1393c1a5`。
  - ☐ **DIFF-GAP-LABEL-XALIGN（P3·低置信·待 live 修改态确认）**：fold band「N unmodified lines」标签左缘比 code 列起点右移约 17px（`tw.css:842 .fd-gap` 首列 `calc(5ch + 35px)`，DiffView.tsx:1557 注释自述应是 `calc(5ch + 27px)`；DIFF-GAP-CENTER 只改了 text-align 没动水平起点）。需一份含修改已有文件的 diff 目视坐实。
  **out-of-scope 复确认**：DIFF-SEARCH/richer scope/per-line hover `+`/composer credits·context ring/👍👎 feedback/Plugins·Sites·PR·Chat 均无我方后端；桌面 diff 文件卡片形态(edgeToEdge 仅手机)、A/M/D 彩色字母徽标、tight bar Commit 收图标本体(DIFF-CP)均为刻意决策。
  - <2026-07-21 05:2x> 轮56：新鲜眼睛全屏重对标 home/thread/diff、关差距 COMPOSER-FOCUS-SEAMLESS(P1·composer 无缝无 focus box)+ COMPOSER-CHIP-GROUP(chip 组左对齐消拉裂)+ THREAD-TURN-FOLD-SPLIT(mode_changed 折进同 turn·人类标签)+ TAIL-ROW-ACTIONS(末条操作行下移到产物卡后)、派工 3(并发 worktree·白名单互斥 tw.css/timeline.ts/Timeline.tsx)、push 8a7f2daf + a93088a7 + 85ebf2c2、live=index-BUKWigjY.js

- 2026-07-21 轮57（headless）：**新鲜眼睛重截 live diff 分栏 1440/1280/1512 对标 Codex + 坐实上轮 seed 的 2 条 diff 工具栏 P2 可达性差距**（1440 diff 分栏工具栏 tight 模式实拍确认 split toggle+commit 标签双失）。关闭 **2 个实质可见功能可达性差距**（一批 2 并发 worktree implementer，白名单互斥 `DiffView.tsx` / `tw.css`，各 vitest+build 绿、自 rebase 自推）：
  - ✅ **DIFF-SPLIT-TOGGLE-GONE**（见上 ✅ 详情，implementer A·`DiffView.tsx`·解耦 effView + tight `…` 菜单补 split toggle·vitest 616·push `75c06eca`）——用户在 1280/1366 主流笔记本宽度重新可达 split view。
  - ✅ **DIFF-COMMIT-LABEL-1440**（见上 ✅ 详情，implementer B·`tw.css`·panel 42vw→46vw·push `1393c1a5`）——1440/1512 恢复完整 commit 工具栏。
  两者 touches 白名单互斥。部署 8809 `live=index-OS0gix6K.js`(200)。**四闸门全绿**：vitest 616 绿(+3)、build 绿；1440/1512 `commitLabel=1 residentToggle=1`、1280 `…` 菜单 `Split view` 项可点且翻转 `Inline view`；home/thread × light/dark 稳态 console error+warning=**0**。截图 before `qa/runs/2026-07-21-r57/live/split-{1440,1280,1512}.png`、after `qa/runs/2026-07-21-r57/after/{split-*,menu-1280,split-1280-applied}.png`。
  **仍开放 ☐（下轮 fresh-eyes 排）**：
  - ☐ **DIFF-GAP-LABEL-XALIGN（P3·低置信·待 live 修改态确认）**：fold band「N unmodified lines」标签左缘比 code 列起点右移约 17px（`tw.css:849 .fd-gap` 首列 `calc(5ch + 35px)`，DiffView.tsx 注释自述应是 `calc(5ch + 27px)`）。需一份含**修改已有文件**的 diff 目视坐实（本轮 QA 会话全 added 文件，无 fold band）。
  - <2026-07-21 05:4x> 轮57：重对标 diff 分栏 1440/1280/1512、关差距 DIFF-SPLIT-TOGGLE-GONE(P2·split 在 tight 宽度经 `…` 菜单可达)+ DIFF-COMMIT-LABEL-1440(P2·46vw 恢复 1440 commit 标签)、派工 2(并发 worktree·白名单互斥 DiffView.tsx/tw.css)、push 75c06eca + 1393c1a5、live=index-OS0gix6K.js

- 2026-07-21 轮58（headless）：**新鲜眼睛全屏重对标 home/thread/diff-Review-split × light/dark × 1440/390/1280 + 3 并发 read-only finder**（diff-split / thread-env / home-scheduled-sidebar）。主驾驶自对标先排除一批**刻意决策**（不修）：home-finder 的 composer-effort-inline（`Composer.tsx:1542-1543` effort≠off 时已内联显示，"Off 不写"是刻意）、sidebar 项目行双行（SIDE-SUBTITLE 消歧同名 workspace/Scratch）、Scheduled 行标题=完整 prompt（SC-13 `title`短名给菜单/`displayTitle`给行）、Scheduled Suggestions 插位（`SUGGESTION_INSERT_AFTER`）均属刻意。捞到并关闭 **2 个实质可见差距**（一批 2 并发 worktree implementer，白名单两两互斥 `ChangesOutcome.tsx` / `DiffView.tsx`，各 vitest+build 绿、自 rebase 自推）：
  - ✅ **ARTIFACT-CARD-SCALE（P2·每个含产物的 turn 都可见）**：同一 turn 内 `ArtifactRow`（"Open in" 文档卡）比其下 `Edited N files` 变更卡明显小一号——产物卡 icon `32×32 rounded-8`/glyph 18/title `13px`/subtitle `11px`/`py-8`，变更卡 icon `38×38 rounded-10`/title `15px`，尺度不统一、产物被视觉降级。Codex（`codex-crop-artifact-cards.jpg`+`codex-crop-change-card.jpg`）产物卡与变更卡**同一大尺度**。修（implementer A·`ChangesOutcome.tsx` only）：`ArtifactRow` icon→`38×38 rounded-10`、glyph→20、title→`15px`(留 font-550)、subtitle→`13px`、padding→`px-12 py-10`。**live 复验**(index-CdlNHX6Z→C3wdLSiL)：thread 屏 artifact tile 现与 "Edited 2 files" 变更卡同尺度、console 0。A push `c9863fe9`（vitest 616 绿）。before `qa/runs/2026-07-21-r58/live/thread-light-1440.png`、after `qa/runs/2026-07-21-r58/after/thread-artifact-{light,dark}.png`。
  - ✅ **DIFF-WRAP-DEFAULT-ON（P2·主流笔记本宽度功能可用性缺陷）**：Review/diff 分栏 line-wrap 默认**关**（`DiffView.tsx:135-142 loadWrap()` key 未设→false），长代码行被 `overflow:hidden` 硬裁、无横向滚动 affordance——`// r35 RD-10 probe hunk marker` 只显 `// r35`、`findMismatchedContent(result, expectedCon…` 被截；且 ≤1280(barTight) 把 wrap 开关降级进 `…` 菜单，主流笔记本宽度审查者看截断代码却找不到修复。**一个 review 工具把正被审查的字符藏起来=真实缺陷**。修（implementer B·`DiffView.tsx`+2 测试 only）：`loadWrap()` 未设→`true`（默认 wrap on）、显式 `"0"` 仍尊重、catch→true；barTight demotion 测试显式设 `"0"` 保原语义；新增回归 unset→on/显式"0"→off。split 列对齐核验：`.diff-wrap` 作用 inline(`.dl-text`)+split(`.dls-half` whitespace-normal)，grid `minmax(0,1fr)` 保双列对齐不破。**live 复验**：1280 barTight 下 line 13/91 尾注释 + fold band import 上下文**全部软换行完整可见**（before 裁成 `// r35`）、console 0。B push `7c75343c`（vitest 617 绿）。after `qa/runs/2026-07-21-r58/after/diff-wrap-{light-1440,dark-1440,light-1280}.png`。
  两者 touches 白名单互斥。部署 8809 `live=index-C3wdLSiL.js`(200)。**四闸门全绿**：vitest 617 绿、build 绿；A/B 目标差距 live 复比确关小无回退；全景 home/thread/diff × light/dark × 1440/390 稳态 console error+warning=**0**；qa/runs 存档 before/after + finder 证据（`finder-{diff,thread,home}/`）。
  **仍开放 ☐（下轮 fresh-eyes 排·本轮 finder seed）**：
  - ☐ **DIFF-REVIEW-MODE-LABEL（P3·diff-finder seed）**：diff 分栏打开时(`view==="diff"`)顶栏仍写 `Environment`、diff 工具栏只有 `Last Turn ▾` scope chip，无显式 "Review"/"Changes" 标签/pill 告知用户处于 Review 模式（Codex 有 `… | Review` 醒目 tab，兼作切回 affordance）。修点：`DiffView.tsx:~818-860` 工具栏行首加紧凑 "Review" 标签，或 `SessionView.tsx` topbar pill 反映 active mode。
  - ☐ **DIFF-GAP-LABEL-XALIGN（P3·低置信·本轮已获修改态样本可坐实）**：fold band「N unmodified lines」标签左缘 vs code 列起点对齐（`tw.css:849 .fd-gap` 首列 `calc(5ch + 35px)` vs 注释自述 `27px`）。本轮 mod 会话 diff-wrap 截图已含 fold band，下轮可目视精测。
  **out-of-scope 复确认**：composer effort-Off 隐藏/sidebar 双行/sched 行全 prompt/Suggestions 插位=刻意；Environment 面板 Background processes·Browser·Sources / composer credits·context ring / 👍👎 / Plugins·Sites·PR·Chat / DIFF-SEARCH·per-line hover `+`·richer scope / A/M/D 彩字徽标·文件卡形态=无后端或刻意；fold band 附 `@@` 上下文=我方或**领先** Codex（GitHub 式更有用），不动。
  - <2026-07-21 06:xx> 轮58：对标 home/thread/diff-Review-split（3 finder 并发）、关差距 ARTIFACT-CARD-SCALE(P2·产物卡对齐变更卡大尺度)+ DIFF-WRAP-DEFAULT-ON(P2·review 默认 wrap 长行不裁)、排除 4 条刻意决策、派工 2(并发 worktree·白名单互斥 ChangesOutcome.tsx/DiffView.tsx)、push c9863fe9 + 7c75343c、live=index-C3wdLSiL.js

- 2026-07-21 轮59（headless）：**新鲜眼睛全屏重对标 8 屏 + 逐组件裁图并排**（home/thread/diff-Review/scheduled × light/dark，外加 sidebar-projects / composer / add-menu / model-dropdown / diff-render / message-actions 逐组件裁图 vs `qa/codex-reference/codex-crop-*.jpg`）。**结论:parity 已很深**——add-menu(Files/Goal/Plan mode 文案逐字一致)、model-dropdown(Model/Effort/Speed/Advanced 结构完全一致)、composer、home(标题/卡 label 实测 fw500 非 bold)、sidebar 均高 parity。关闭 **2 个实质可见差距**（一批 2 并发 worktree implementer,白名单互斥 DiffView.tsx / Scheduled.tsx,各 vitest 617 绿 + build 绿,自 rebase 自推）:
  - ✅ **DIFF-REVIEW-LABEL（P2·最重要屏·diff-finder 多轮 seed）**：diff/review 面板工具栏此前只有 `Last Turn ▾ +932 −0 …`,**无任何"Review"模式标识**;Codex diff 面板头有醒目 `Review` pill 告知用户在 review 模式。修（单 implementer 内联·仅 `DiffView.tsx`·不碰 tw.css/不碰 topbar 以避 TH-15 冲突）：主 `.diffbar` 最前、`{scopeControl}` 之前新增非交互 `FileMagnifyingGlass + "Review"` subtle pill,`{!barTight && …}` 保证窄面板整块让位、`shrink-0` 不挤 `.diff-closebtn`。**live 复验**（8809 index-WYukE6BN,playwright 断言）：宽面板 light/dark 均现 Review pill、✕ 仍在面板内（closeRight 1428 ≤ barRight 1440）、console error+warning=0。push `5b5b275e`。
  - ✅ **SCHED-TITLE-WEIGHT（P3）**：Scheduled 列表行标题用裸 `<b>`（实测 fw**700** heavy bold）,Codex 列表/Suggestions 标题为较轻 semibold(~590/600)。修（单 implementer 内联·仅 `Scheduled.tsx` 两处 +`font-semibold`·不碰 tw.css）：列表行标题(`:662`)与 Suggestions 标题(`.sched-suggest-title :498`)字重 700→**600**,颜色/字号/行高/两行截断不变。**live 复验**：listFW=600、sugFW=600、console 0。push `76ee664d`。
  **本轮对标结论（诚实记录）**：8 屏 + 8 组件逐一并排,四主屏经 58 轮打磨均高 parity;真差距集中在 diff 面板缺 Review 模式标识(已关)与 scheduled 标题过重(已关)。**排除（刻意/无后端,不做）**：topbar Review pill=违 TH-15 刻意决策(「Codex topbar carries neither pill」);sidebar `.project-heading` 11px bold repo 名=r47 刻意;DIFF-SEARCH/Plugins/Sites/PR=无后端;composer effort/environments chip=provider/无后端。
  **仍开放 ☐（下轮 fresh-eyes 排）**：DIFF-GAP-LABEL-XALIGN(P3·低置信·需修改态会话样本,本轮 QA 会话全 added 文件无 fold band 无法坐实);diff 文件状态徽标(彩圈 Ⓐ vs Codex mono `M↕`,判断题·非清晰更优,记完整性)。
  - <2026-07-21 08:xx> 轮59：对标 8屏+8组件裁图(逐组件并排 codex-crop-*)、关差距 DIFF-REVIEW-LABEL(P2·最重要屏·diff 工具栏首 Review pill 对齐 Codex)+ SCHED-TITLE-WEIGHT(P3·scheduled/suggestion 标题 700→semibold600)、排除 topbar-pill(违TH-15)+sidebar-repo-bold(r47)等刻意/无后端项、派工 2(并发 worktree·白名单互斥 DiffView.tsx/Scheduled.tsx)、push 5b5b275e + 76ee664d、live=index-WYukE6BN.js

- 2026-07-21 轮60（headless）：**新鲜眼睛全屏重对标 home/rich-thread/diff(含修改文件)/scheduled × light/dark/390 + 逐组件裁图 + 3 并发 read-only finder**（home+composer / scheduled / diff-toolbar+render）。**结论:主屏经 59 轮打磨仍高 parity,但 finder 精确定位到 5 条有 file:line 证据的实质可见差距**——覆盖 4 屏、非 a11y/perf。关闭 **5 个实质可见差距**（一批 2 并发 worktree implementer,白名单两两互斥 `{tw.css,DiffView.tsx,Scheduled.tsx}` / `{Composer.tsx}`,各 vitest 617 绿 + build 绿,自 rebase 自推）:
  - ✅ **DIFF-GAP-LABEL-XALIGN（P1·最重要屏·多轮 seed·本轮像素坐实）**〔R60 关闭〕：fold band「N unmodified lines」计数标签左缘比代码列文本起点右移 **17px**（`.fd-gap` 首列 `calc(5ch + 35px)` + label px-2 8px = 5ch+43px,vs 代码 `.dl` 18px+5ch+px-2 8px = 5ch+26px）。修（implementer A·`tw.css:849` + `DiffView.tsx:1612` 注释）：首列 `calc(5ch + 35px)`→**`calc(5ch + 18px)`**（label 起点 = 5ch+18+8 = 5ch+26px 精确对齐代码列),并把 DF-5 注释里自相矛盾的 `calc(5ch + 27px)` 同步改为 `5ch + 18px`。**live 复验**(moddiff index-BI5hVgbw)：4 条 band(9/71/73/1,567 unmodified lines)计数左缘与代码 `import` 文本起点同 x、console 0。合入 A push `222dccda`。
  - ✅ **SCHED-SEARCH-FULLWIDTH（P1·Scheduled 屏最大差距·纯 CSS 布局 bug）**〔R60 关闭〕：`.sched-search` 只有 `w-full min-w-[220px]` 无 flex/边框/定位→放大镜浮框外、input 退回浏览器默认宽(~180px),Codex 是贯穿内容列全宽圆角 pill+图标内嵌。修（implementer A·`tw.css:921` only,markup 无需动）：容器 `flex w-full items-center gap-2 rounded-[10px] border border-line bg-panel px-3 h-9 text-dim focus-within:border-ink-2` + 新增 `.sched-search input { flex-1 w-full border-0 bg-transparent p-0 text-[13px] text-ink outline-none placeholder:text-dim }`。**live 复验**：搜索框贯穿内容列全宽 pill、放大镜内嵌左缘、light/dark 无溢出无游离图标。合入 A push `222dccda`。
  - ✅ **COMPOSER-WORKTREE-GLYPH（P2·两相邻 chip 同图标像渲染 bug）**〔R60 关闭〕：composer 的 "New worktree" chip 与 branch chip 都渲染同一 Phosphor `GitBranch`,丢失 worktree(隔离)vs branch(分支)语义;Codex 用不同字形(worktree=分叉,branch=git-branch)。修（implementer B·`Composer.tsx` only）：worktree chip(`runLocation!=="local"` 支)`<GitBranch size={17}/>`→`<GitFork size={17}/>`(顶部补 import),branch chip 仍 `GitBranch`,`Lightning`/`Desktop` 两支未动。**live 复验**：两 chip 字形明显不同、console 0。B push `d4c41df1`。
  - ✅ **COMPOSER-APPROVAL-CARET（P2·冗余 chrome）**〔R60 关闭〕：approval pill 尾部总渲染 `<Caret/>`(session `:1468` + home `:1508`),Codex approval pill 无 caret,只 model pill 有。修（implementer B·`Composer.tsx` only）：删两处 approval pill 的 `<Caret/>`,保留 model pill caret(`Caret` import 仍被 model pill 用故保留)。**live 复验**：home+session approval pill 无 caret、model pill "Gemini Flash ⌄" 仍有 caret。B push `d4c41df1`。
  - ✅ **HOME-CARD-JUSTIFY（P2·无回退对齐修正）**〔R60 关闭〕：Codex 四张建议卡 label 共享底部基线(icon 钉顶/label 钉底,单行卡 label 与两行卡第二行齐平,见 `codex-crop-newtask-emptystate.jpg`),我方 `.home-empty-card` 用 `justify-start` 使单行卡 label 浮顶。修（implementer A·`tw.css:391`）：`justify-start`→`justify-between`。注:1440 下四卡恰都换成 2 行、可见差异有限,属对齐 Codex 意图的无回退修正,窄标签/宽视口才显钉底效果。合入 A push `222dccda`。
  两批白名单互斥。部署 8809 `live=index-BI5hVgbw.js`(200)。**四闸门全绿**：vitest 617 绿、build 绿;A/B/C/E 目标差距 live 复比确关小无回退(D 无回退);home/scheduled/moddiff × light/dark 稳态 console error+warning=**0**;qa/runs 存档 `2026-07-21-r60/{live,after}/` before/after + 3 finder 证据(见 task 通知)。
  **排除的刻意/无后端项(诚实记录)**：Scheduled `Finished` tab(无 suspend 后端·SC-11)、行标题=完整 prompt(displayTitle·与 SC-13 有张力但有注释辩护·本轮不碰待评审)、状态图标丰富度 ✓/!/⏸(SCH-ICON 有意增强)、Mark-all-as-read(有后端仅无未读时不显)、Suggestions 文案/插位(产品适配)=刻意/parity;composer environment chip/full-access 权限模型/model effort-off/repo underline(75f4b3a2)/chip-flat(R46)/card 13px(R52)=刻意或无后端;diff A/M/D 彩色徽标·topbar 无 pill(TH-15)·search-in-diff·Plugins/Sites/PR·window chrome=刻意/无后端;fold band 附 @@ 上下文=我方领先。
  **本轮定性**:**非失败轮** — 一批关闭 5 个用户一眼可见的 Codex 差距,横跨 diff/scheduled/home/composer 四屏,含最重要屏(diff)的像素级对齐缺陷与 Scheduled 屏最大的搜索框布局 bug。经 60 轮打磨真差距日益细,但 finder 仍在 4 屏各挖到实质可见项。
  - <2026-07-21 06:5x> 轮60：对标 home/thread/diff(含修改文件)/scheduled(3 finder 并发)、关差距 DIFF-GAP-LABEL-XALIGN(P1·fold band 标签17px右移→精确对齐)+ SCHED-SEARCH-FULLWIDTH(P1·搜索框全宽pill+图标内嵌)+ COMPOSER-WORKTREE-GLYPH(worktree chip→GitFork)+ COMPOSER-APPROVAL-CARET(去冗余caret)+ HOME-CARD-JUSTIFY(卡label钉底)、排除 Finished-tab/displayTitle/status-icons/environment-chip/full-access/repo-underline 等刻意/无后端项、派工 2(并发 worktree·白名单互斥 tw.css+DiffView+Scheduled / Composer)、push 222dccda + d4c41df1、live=index-BI5hVgbw.js

- 2026-07-21 轮61（headless）：**新鲜眼睛全屏重对标 home/rich-thread/diff-Review-split/scheduled × light/dark + 逐组件裁图 + 3 并发 read-only finder**（diff-review / thread-env / home+sidebar+scheduled）。主驾驶自对标先确认四主屏经 60 轮打磨仍高 parity（home headline/卡 label、Review 分栏工具栏 Review pill+Last Turn+语法高亮+A 徽标+inline/split toggle+Commit or push、Scheduled 全宽搜索 pill、thread 变更卡/产物卡/tail-row 均对齐）。捞到并关闭 **3 个实质可见差距**（一批 2 并发 worktree implementer，白名单两两互斥 `{Sidebar.tsx,tw.css}` / `{SessionView.tsx}`，各 vitest 617 绿 + build 绿，自 rebase 自推）:
  - ✅ **NAV-KBD-BADGE（P1·sidebar 常驻可见·金标背离）**：primary-nav "New session" 行右端常驻渲染带边框 `⌘⌥N` 快捷键徽章（`Sidebar.tsx:406-408`，RH-4 注释自称 "Codex-style"）；但金标 `qa/codex-reference/codex-crop-sidebar-nav.jpg` 恰恰相反——Codex 所有 nav 行（New task/Scheduled/…）均**无常驻快捷键徽章**（Scheduled 仅右侧蓝色未读点）。RH-4 的 parity 前提在金标中不成立。修（implementer A·`Sidebar.tsx`+`tw.css`）：删 nav 行 `{keys && <span className="nav-kbd">}` 渲染（快捷键保留在 `title=` 与 ⌘K 面板），`tw.css:298` 合并选择器 `.nav-kbd, .menu-kbd`→仅留 `.menu-kbd`（命令面板仍用）消死类；耦合测试 `Sidebar.nav.test.tsx`（硬编码 RH-4 徽章存在）重写为断言 nav 无 `.nav-kbd` 且 title 仍携快捷键 token。**live 复验**(index-D84NSRHb)：New session 行无徽章、与金标一致、console 0。合入 A push `86cf64b6`。
  - ✅ **HOME-CARD-FLAT（P2·home 空态常见·双层 elevation 竞争）**：New task 空态四张 starter 卡带静止 `shadow-md`（`tw.css:391`），卡"浮"在页面上，再叠加 composer `shadow-xl` 形成两级 elevation 竞争；金标 `codex-crop-newtask-emptystate.jpg` 四卡近乎纯平（仅极淡 hairline 边框），elevation 独留给底部 composer。修（implementer A·`tw.css:391`）：`.home-empty-card` 去静止 `shadow-md`、hover 从 `hover:shadow-lg` 降 `hover:shadow-sm`，border/hover-border/布局/尺寸/字号不变，移动端媒体查询未动。**live 复验**：四卡静止纯平、唯 composer 抬起、console 0。合入 A push `86cf64b6`。
  - ✅ **THREAD-1-CONTINUE-DEDUP（P3·step-limit turn 可见·冗余控件）**：会话被 step-limit/cancel 异常终止时，composer 上方终止 banner 已带文字按钮 "Continue in new session"，thread 底部 tail-row `MsgActions` 又画一个无标签 continue 图标（`ArrowSquareOut`）——同一动作两入口，裸图标语义不明。TH-12/TH-14 dedup 只处理了 thread chip echo、没管 tail-row continue。根因 `SessionView.tsx:926` 无条件把 `onContinue` 传给 Timeline。修（implementer B·`SessionView.tsx` only）：`onContinue={terminalNotice?.action === "continue" ? undefined : () => openModal({kind:"fork",sid})}`——终止-continue banner 在场时不传给 Timeline，continue 只留 banner（带文字）；正常完成 turn 无 banner，tail-row continue 不变。**live 复验**：step-limit thread 的 tail-row 从 copy/share/continue 三图标→copy/share 两图标、"Step limit reached" 卡保留单一带文字 Continue 按钮、console 0。B push `7a9d41ed`。
  两批白名单互斥（A: Sidebar.tsx+tw.css；B: SessionView.tsx）。部署 8809 `live=index-D84NSRHb.js`(200)。**四闸门全绿**：vitest 617 绿、build 绿；G2/G1/Thread-1 目标差距 live 复比确关小无回退；全景 home/thread/scheduled × light/dark 稳态 console error+warning=**0**；qa/runs 存档 `2026-07-21-r61/{live,after}/` before/after + 3 finder 证据。
  **排除的刻意/无后端项(诚实记录)**：composer model pill 强调层级(G3·medium-low·下轮 seed)、diff 删除行 gutter 实心条 vs Codex hatch 斜纹(P3·borderline nit·下轮 seed)、diff 逐文件圆角卡 vs edge-to-edge(edgeToEdge=narrow 门控证明桌面卡形态刻意)、Scheduled Create 触发器边框(无专门裁图·低置信)、composer 分支 chip 显 worktree 名(仅 worktree workspace 边缘态·正常选 repo 显 main)、thread turn fold "Worked·1 step"(Thread-2·中断 turn 无 durationMs·较险·下轮 seed)；及既往 Finished-tab/displayTitle/status-icons/environment-chip/effort-off/A-M-D 彩徽标/topbar-pill(TH-15)/Plugins-Sites-PR 等刻意/无后端项。
  **本轮定性**:**非失败轮** — 一批关闭 3 个用户一眼可见的 Codex 差距，横跨 sidebar(常驻徽章金标背离)/home(卡 elevation)/thread(重复 continue)三屏。经 61 轮打磨真差距日益细，本轮亮点是发现并纠正了一条被 RH-4 当作"Codex parity"、实则与金标背离的常驻快捷键徽章。
  - <2026-07-21 07:xx> 轮61：对标 home/thread/diff-Review/scheduled(3 finder 并发)、关差距 NAV-KBD-BADGE(P1·去 nav 常驻⌘⌥N徽章·金标背离RH-4)+ HOME-CARD-FLAT(P2·home starter 卡去静止投影)+ THREAD-1-CONTINUE-DEDUP(P3·step-limit tail-row 去重复continue)、排除 model-pill-weight/diff-del-hatch/diff-filecard/Thread-2 等 seed 下轮及刻意/无后端项、派工 2(并发 worktree·白名单互斥 Sidebar.tsx+tw.css / SessionView.tsx)、push 86cf64b6 + 7a9d41ed、live=index-D84NSRHb.js

- 2026-07-21 轮62（headless）：**新鲜眼睛全屏重对标 diff-Review-split+model-pill / 富thread+Environment+turn-fold / home+sidebar+scheduled × light/dark × 1440/390 + 逐组件裁图 + 3 并发 read-only finder**。主驾驶自对标先排除刻意/无后端项。关闭 **4 个实质可见差距**（一批 2 并发 worktree implementer，白名单两两互斥 `{Composer.tsx,tw.css}` / `{timeline.ts,Timeline.tsx}`，各 vitest 绿 + build 绿，自 rebase 自推）:
  - ✅ **COMPOSER-MODEL-PILL-WEIGHT（P2·两 finder 交叉确认）**〔R62 关闭〕：composer 底栏 model pill 整体 `fw400/rgb(96,96,96)`(=ink-2 灰),模型名与 effort-sub 同灰同字重、毫无层次——模型名是这行最需一眼确认的事实却被压成 dim 灰。Codex(`codex-crop-composer.jpg`)模型名近黑加粗(~600)、effort 后缀退 dim 灰常规。修（implementer A·`Composer.tsx:1541` 把 `{modelLabel}` 包进 `<span className="cx-model-name">` + `tw.css:426` 拆出 `.cx-model-name {font-semibold text-ink}` 并给 `.cx-pill-sub` 加 `text-dim`；共享基类 `.cx-pill` 未动、access/env chip 保持扁平）。**live 复验**(index-DMDOJb-8)：`.cx-model-name` fw=**600** color=**rgb(13,13,13)**、console 0。合入 A push `0fc301bd`。
  - ✅ **NAV-ACTIVE-SHADOW（P2）**〔R62 关闭〕：`.primary-nav button.active`(`tw.css:297`)残留 `shadow-sm` 投影,active 药丸浮起一层灰影,与 R61 建立的整体扁平语言矛盾(同页 `.sched-tab.on` 已 box-shadow:none 自相矛盾)。Codex(`codex-crop-sidebar-nav.jpg`)active nav 为纯色浅灰圆角填充无投影。修（implementer A·`tw.css:297` 删 `shadow-sm`,保 `bg-panel-2 text-ink font-semibold`）。**live 复验**：active nav box-shadow=**none**、bg 仍浅灰填充、console 0。合入 A push `0fc301bd`。
  - ✅ **GBAR-META-GAP（P3·确诊 CSS bug）**〔R62 关闭〕：活动 goal banner 底部 meta 渲染成 `0/3 checks249h 50m`——`.gbar-meta`(`tw.css:691`)只有 `text-[12px] text-dim` 无 flex/gap,两裸 span(gbar-checks 与 elapsed)粘连读作一个乱码 token。修（implementer A·`tw.css:691` 改 `.gbar-meta` 为 `inline-flex items-center gap-2 text-[12px] text-dim`；SessionView.tsx 未碰）。**live 复验**：display=flex gap=**8px**、文本从粘连变 "0/3 checks | 249h 58m"、console 0。合入 A push `0fc301bd`。
  - ✅ **THREAD-2（P1/P2·seed 坐实·多步中断 turn 关闭）**〔R62 关闭〕：被 step-limit/approval 停摆的 turn,fold 头显 `Worked · N steps`(无耗时);只有 settled final answer turn 才显 `Worked for <耗时>`。live 实例：approval 停摆 turn 头 `Worked · 8 steps`。Codex(`codex-task-thread.jpg`)每个 turn 头恒 `Worked for <耗时>`,耗时是第一信号、长线程可扫读。修（implementer B·`timeline.ts` 给 `WorkFold` 增真实边界 `startMs`/`endMs`(=turn genStart→末次 gen/带 ts 活动,**非** durationMs、不编造),`foldWork` 消费透传的隐藏 turn 标记推进 spanEnd·flush 写边界；`Timeline.tsx` 新增 `foldInput` 保留 turn 标记喂 foldWork、`mergeAdjacentChips` 对 turn 标记透明、`workedLabel` 走 `durationMs → startMs..endMs → foldSpanMs → step 数` 阶梯）。**live 复验**(index-sHw0jI1t)：approval 停摆 turn `Worked · 8 steps`→**`Worked for 116m 23s`**、settled turn 文案不回退(rich 多条 `Worked for 16s/2s/…` 原样)、console 0。B push `ef5462f5`(vitest **626 绿**含 9 新回归)。**诚实注**：单步 step-limit turn 若终态 elapsed(如 `Goal cancelled 00:34`)由独立 terminal banner 渲染、不在 fold ts 路径,则优雅退回步数(rich 尾 `Worked · 1 step`,不编造 span)——多步中断的常见情形已关。
  两批白名单互斥(A: Composer.tsx+tw.css；B: timeline.ts+Timeline.tsx)。部署 8809 `live=index-sHw0jI1t.js`(200)。**四闸门全绿**：vitest 626 绿、build 绿；四目标差距 live 复比确关小无回退(settled turn 文案未退)；全景 home/rich/diff/scheduled × light/dark × 1440/390 稳态 console error+warning=**0**；qa/runs 存档 `2026-07-21-r62/{live,after}/` before(finder-{diff,thread,home,composer})/after(composer-modelpill-home/sidebar-nav-active/gbar-meta/thread2-{approval,rich})。
  **排除的刻意/无后端项(诚实记录)**：diff-del-hatch(P3·finder 判✂·QA-0718 刻意反转红斜纹 hatch 因暗色刺眼被换实心条,复原会重引入被真实用户 QA 拒绝的暗色刺眼图案·低价值)、home-repo-dotted-underline(纯样式补下划线会误导"可点"实则静态镜像·二选一须接控件·本轮不做)、Scheduled Create 按钮无边框化(裁图偏小·低置信·需高清 create 区裁图坐实)、home card radius 12→16(裁图缩放·像素差难锚·低置信)、fold 混类批次 ×8 单行聚合(INC-41 FOLD-RUN 刻意·无 Codex 展开态金标)；及既往 composer effort-off/A-M-D 彩徽标/topbar-pill(TH-15)/DIFF-SEARCH/Plugins-Sites-PR/Environment-Background-Browser-Sources/credits-context-ring/👍👎 等无后端或刻意项。
  **仍开放 ☐（下轮 fresh-eyes 排·本轮 seed）**：formatElapsed 多日不进位(`249h 50m` 应 `10d 9h`·`timeline.ts:1289`·P3·edge)；Thread-2 单步中断 turn 终态 elapsed 未进 fold 头(需把 terminal banner 的 cancel/limit 事件 ts 引进 fold endMs·跨 SessionView 数据流·较深·下轮评估)。
  **本轮定性**:**非失败轮** — 一批关闭 4 个用户一眼可见的 Codex 差距,横跨 composer(model 名强调层级)/sidebar(active 药丸扁平化)/goal-banner(meta 解粘连)/thread(中断 turn 显真实耗时)四屏。经 62 轮打磨真差距日益细,本轮亮点是把长会话里最影响可扫读的 turn 头耗时一致性(多步中断态)对齐了 Codex。
  - <2026-07-21 07:xx> 轮62：对标 diff+model-pill/thread+turn-fold/home+sidebar+scheduled(3 finder 并发)、关差距 COMPOSER-MODEL-PILL-WEIGHT(P2·model 名 fw600 提黑·effort 退灰)+ NAV-ACTIVE-SHADOW(P2·active 药丸去投影)+ GBAR-META-GAP(P3·goal meta 解粘连 gap8px)+ THREAD-2(P1/P2·多步中断 turn 头显真实 work-span)、排除 diff-del-hatch(✂)/home-repo-underline/Create-btn/card-radius 等刻意/低置信/无后端项、派工 2(并发 worktree·白名单互斥 Composer.tsx+tw.css / timeline.ts+Timeline.tsx)、push 0fc301bd + ef5462f5、live=index-sHw0jI1t.js

- 2026-07-21 轮63（headless）：**新鲜眼睛全屏重对标 home/rich-thread/diff-Review/scheduled × light/dark × 1440/390 + 逐组件裁图 + 3 并发 read-only finder**（diff-review / thread-env / home-sidebar-scheduled）。主驾驶自对标先确认四主屏经 62 轮打磨仍高 parity。关闭 **2 个实质可见差距**（一批 2 并发 worktree implementer，白名单两两互斥 `{Timeline.tsx,tw.css}` / `{Home.tsx}`，各 vitest 626 绿 + build 绿，自 rebase 自推）:
  - ✅ **THREAD-GOAL-VERDICT-TONE（P2·thread-finder 金标 crop 直证·每个 goal-achieved turn 尾可见）**〔R63 关闭〕：turn 尾 goal verdict 此前渲染成**绿色成功徽章**——`Timeline.tsx:1341` `<CheckCircle weight="fill"/>` 实心绿勾 + `tw.css:1278 .turn-footer text-green font-medium` 绿色 medium 字重，与同一 tail 行的 copy/share 图标(中性灰 `text-dim`)层级不一致、抢视觉。Codex 金标 `codex-crop-message-actions.jpg` verdict `Goal achieved in 3h 47m 26s` 是**描边圈勾 + 灰色常规字重**,和左侧动作图标完全同色同权重——verdict 是"元数据陈述"而非需庆祝的绿徽章。修（implementer A·`Timeline.tsx`+`tw.css` only）：`.turn-footer` 去 `text-green font-medium`→`text-dim`(字号 text-[13px]/inline-flex 保留)；`CheckCircle` 去 `weight="fill"`→描边。**live 复验**(index-D4XMT8eb·goal-achieved 会话 6027)：verdict `⊘ Goal achieved in 00:18` color=**rgb(85,85,85)** 灰(dark rgb(160,160,173)) fontWeight=**400**(原绿 fw500)、与 copy/share 同排同色、console 0。合入 A push `9ede6d72`（vitest 626 绿）。before `qa/runs/2026-07-21-r63/live/rich-*`(本轮 live 为取消态无 verdict,证据取金标 crop+源码+achieved 会话 after)、after `qa/runs/2026-07-21-r63/after/verdict-{light,dark}-1440.png`。
  - ✅ **HOME-COMPOSER-DOCK（P1·home 空态每次可见·金标背离）**〔R63 关闭〕：New task 首页把「云图标+标题+4卡+composer」整组用 `.hero` `justify-center` **垂直居中成一簇**,composer 紧贴卡片下方浮在页面中间,1440 下方/390 上下留大片死白(390 下方约 40% 空)。Codex 金标 `codex-new-task-home.jpg`:hero 居中偏上、**composer 钉视口底部**、卡片与 composer 间有明显留白——标准输入框钉底聊天布局。修（implementer B·`Home.tsx` only·纯内联 Tailwind、不碰 tw.css）：把 `.home-empty`(hero 三要素)+`<DaemonAlert/>` 包进新 `<div className="flex w-full min-h-0 flex-1 flex-col items-center justify-center gap-5">`,该 flex-1 子撑满 `.hero` 纵向空间把 composer(hero 末子)挤到底部;`.hero` `justify-center`/移动端 `justify-start` 因 flex-1 变无关,桌面+移动端 composer 皆钉底(顺带修 390 死白)。**保留** hero(未回退成无标题纯底部输入框·遵 QA-45「贴合供图」原则,记忆 home-newtask-bottom-pinned-qa45 只反对移除 hero)。**live 复验**：composerBottom=**868**/vh=900(钉底)、cards↔composer gap=**237px**(原紧贴)、light/dark+390 无溢出、console 0。合入 B push `effa056c`（vitest 626 绿）。before `qa/runs/2026-07-21-r63/live/home-{light,dark}-1440.png`+`home-390-*`、after `qa/runs/2026-07-21-r63/after/home-{light,dark}-{1440,390}.png`。
  两批白名单互斥(A: Timeline.tsx+tw.css；B: Home.tsx)。部署 8809 `live=index-D4XMT8eb.js`(200)。**四闸门全绿**：vitest 626 绿、build 绿；A/B 目标差距 live 复比确关小无回退；全景 home/rich/diff/scheduled × light/dark × 1440/390 稳态 console error+warning=**0**；qa/runs 存档 `2026-07-21-r63/{live,after}/` before/after + 3 finder 证据。
  **排除的刻意/无后端项(诚实记录)**：HOME-HEADLINE-WEIGHT(P3·home-finder·headline font-semibold vs Codex medium·跨字体感知·finder 自标低置信「verify/若匹配则不动」·本轮不做·下轮如仍显重再评)；Composer `Access: set by agent spec` fallback(thread-finder·Composer.tsx:1468·spec 不暴露具体 posture 时的刻意诚实·无后端·不谎报级别)；diff change card 零值红绿(finder A PIL 采样证 Codex `−0` 亦饱和红非灰化·两侧 parity·非差距)；及既往 Environment-Background/Browser/Sources、👍👎、THREAD-2 单步中断终态 ts(需跨 SessionView 引 terminal 事件 ts 进 fold endMs·较深·仍 seed)、formatElapsed 多日不进位(P3 edge·仍 seed)、topbar-pill(TH-15)/A-M-D 彩徽标/Plugins-Sites-PR/DIFF-SEARCH 等无后端或刻意项。
  **取证缺口(下轮补)**：finder A 指出本轮 live「diff-*」截图捕获的是 thread 内 change card 而非点 Review 后的 DiffView 渲染分栏(行号/fold band/逐文件 header/split)——已在轮内补截 `diffsplit-{light,dark}-1440.png`,但下轮采集应默认点开 Review 再对 `codex-crop-diff-rendering.jpg`/`codex-diff-review.jpg`。
  **本轮定性**:**非失败轮** — 一批关闭 2 个用户一眼可见的 Codex 差距:home 空态 composer 钉底(金标背离·纠正 W1 居中簇)+ thread goal verdict 从绿徽章降为中性灰元数据(层级对齐同排动作图标)。经 63 轮打磨真差距日益细,本轮两条均金标直证。
  - <2026-07-21 08:xx> 轮63：对标 home/thread/diff/scheduled(3 finder 并发)、关差距 THREAD-GOAL-VERDICT-TONE(P2·verdict 绿徽章→中性灰描边元数据)+ HOME-COMPOSER-DOCK(P1·home 空态 composer 钉视口底部·保留 hero)、排除 HOME-HEADLINE-WEIGHT(低置信)/Access-fallback/change-card-zero 等刻意/低置信/无后端项、派工 2(并发 worktree·白名单互斥 Timeline.tsx+tw.css / Home.tsx)、push 9ede6d72 + effa056c、live=index-D4XMT8eb.js

- 2026-07-21 轮64（headless）：**新鲜眼睛全屏重对标 home/rich-thread/diff-Review-split/scheduled × light/dark × 1440/390 + 逐组件裁图 + 3 并发 read-only finder**（scheduled / thread+environment / diff-split）。主驾驶自对标先坐实两条 Codex "候选差距"其实**均已端到端实现**（Scheduled 的 `Next run in…` 已由 `Scheduled.tsx:267 nextRunPhrase(nextRunAt)` 渲染、死 series 才诚实退回 `Ran 1w ago`；`✓ Mark all as read` 已在 `Scheduled.tsx:570` 且带蓝 unread 点——live 没显示只因 headless 浏览器 localStorage 空），排除为非差距。关闭 **3 个用户一眼可见的实质差距**（一批 **3 并发 worktree implementer**，白名单两两互斥 `{tw.css}` / `{DiffView.tsx}` / `{timeline.ts}`，各 vitest 绿 + build 绿，自 rebase 自推）:
  - ✅ **HOME-HEADLINE-WEIGHT（P2/P3·home 空态每次可见·金标高清裁图直证）**〔R64 关闭·seed 坐实〕：New task 首页标题 `.home-empty-headline`（`tw.css:382`）此前 `font-semibold`(600)，比 Codex 金标 `codex-crop-newtask-emptystate.jpg`（"What should we build in agentrunner?" 呈**轻/常规字重**、repo 名与句子等重）明显更粗；且我方 repo 名 `.home-empty-repo` 是 `font-medium`(500) 比句子更轻，方向反了。修（implementer A·`tw.css` only）：headline `font-semibold→font-normal`(400)（浏览器内实测 400 vs 500 与金标笔画比对，400 最贴），repo `font-medium→font-normal` 与句子等重，移动端 `tw.css:1124` 同改 400；未加点状下划线（R62 已排除·会误导可点）。**live 复验**(index-FxBH-3py)：headline computed fw=**400**（light/dark/390）、repo fw=**400**、笔画明显变轻、console 0。合入 A push `b452321a`（vitest 626 绿）。
  - ✅ **SCHED-MORE-HOVER（P2·Scheduled 每行常驻噪音·Tailwind 迁移回归）**〔R64 关闭·finder A 发现〕：Scheduled 每行右端常驻一个 `···` 操作按钮，但源码 `Scheduled.tsx:721-733` 注释明写"invisible at rest, appears on hover/focus"——CSS 在迁移 68a8a69b 时把隐藏样式丢了（`tw.css:948 .sched-more` 只剩 `shrink-0`），属回归。Codex 金标 `codex-crop-scheduled-list.jpg` 行静止只有 unread 蓝点、操作 hover 才现、列表干净。修（implementer A·`tw.css` only·纯 CSS，行 wrapper `.scheduled-row-wrap` 已存在无需碰 tsx）：`.sched-more` 加 `opacity-0 transition-opacity`，新增 `.scheduled-row-wrap:hover/:focus-within/.menu-open .sched-more { opacity-100 }`，触屏 `@media(hover:none)` 兜底常显。**live 复验**：静止 opacity=**0**、hover opacity=**1**、菜单打开/键盘 focus 也显、console 0。合入 A push `b452321a`。
  - ✅ **DIFF-FILENAME-BOLD（P2·每个 diff 可见·自我不一致）**〔R64 关闭·finder C 发现〕：Diff/Review 分栏逐文件 header（`DiffView.tsx:258-261 FileHead`）目录段 `.fd-dir` 灰暗但文件名 base 裸渲染、与目录同字重；Codex 金标 `codex-crop-diff-header.jpg` 文件名**实心加粗更深**给焦点。且我方自己的 "Edited N files" 变更卡 `ChangesOutcome.tsx:538` **已**把 base `<b style={{fontWeight:600}}>` 加粗——diff header 是遗漏。修（implementer B·`DiffView.tsx` only·镜像 ChangesOutcome 既有内联模式、不新增 CSS 类不碰 tw.css）：`DiffView.tsx:260`（FileHead）+ `:1051`（工具栏 file-list popover）把裸 `{base}` 包成 `<b style={{fontWeight:600,color:"var(--ink)"}}>{base}</b>`。**live 复验**：diff 文件名 fw=**600** color=ink（light rgb(13,13,13)/dark rgb(236,236,241)），目录段仍 dim，与变更卡一致、console 0。合入 B push `96b1ae37`（vitest 626 绿）。
  三 implementer 白名单互斥（A: tw.css；B: DiffView.tsx；C: timeline.ts）。部署 8809 `live=index-FxBH-3py.js`(200)。**四闸门全绿**：vitest 629 绿（C 加 3 回归）、build 绿；A/B 三目标差距 live 复比确关小无回退；全景 home/rich/diff/scheduled × light/dark（home→sched→diff→rich 顺序导航）稳态 console error+warning=**0**；qa/runs 存档 `2026-07-21-r64/{live,after}/`（live 8 张 before + after verify-{home,sched,diff,rich}-{light,dark} + headline/sched-more/diff-filename）。
  - ⚠️ **THREAD-2-SINGLESTEP（seed·机制落地但金标会话未可见翻转·诚实记录）**〔R64 部分·C push `fab85163`〕：按 finder B 规格把 terminal-event ts（`goal_cancelled`/`limit_exceeded`/budget-exhausted 家族）透传进 turn work-span（`ChipItem.ts` + `echoChip(ts)` + `noteTs` 消费 chip.ts），vitest **629 绿**含 3 条新回归（canonical 34s 间隔单步中断 fold 头产出 `Worked for 34s`、settled turn 仍走 durationMs）。**但 C 真机复验金标会话 `297d` 时 A/B（有/无改动·真实渲染管道）label histogram 逐字节相同——机制在该会话是 no-op**：末尾 `Worked · 1 step`（紧挨 "Goal cancelled 00:34" banner）不翻转，根因与 finder 模型不同——该 tail fold 的 `generation_started` 已被前一条 settled turn 的 durationMs flush 消费掉（genStart 重置），我的 fix 让终态 chip ts 抵达了 `endMs` 但**缺 startMs**→仍退步数。C 实测过一个能诚实翻转的最小扩展（给 `ToolItem` 加 ts + `noteTs` 用首个带 ts 项开 span，tail 从 `1 step`→`Worked for 2m`、其余 fold 零回退），但它改动 `foldSpanMs` 在 **Timeline.tsx** 明写的"tool/chip 不带 ts"契约、与本批并发 implementer 跨文件耦合，超白名单授权 → 正确地留给下轮。**定性：机制落地+单测证明+已推 main（安全无回退），但不算本轮"可见"胜利**。
  **仍开放 ☐（下轮 fresh-eyes 排·本轮 seed）**：
    - **THREAD-2-SINGLESTEP 收尾**（tool-dating 那半）：给 `ToolItem` 加 ts + `noteTs` 用首个带 ts 项开 `startMs`，让被 settled turn flush 消费掉 genStart 的 tail 中断 fold 也能显真实耗时；需与 `Timeline.tsx` 的 `foldSpanMs`"tool/chip 不带 ts"契约协调（跨 timeline.ts+Timeline.tsx，单 implementer 串行做）。C 已验证可翻转且其余 fold 零回退。
    - **ENV-RUNDETAILS-ALIGN**（P2/P3·finder B 发现·`tw.css:1328`）：Environment 面板底部 `ⓘ Run details` 居中，与上方 Changes/Worktree 等左对齐行割裂（`.supervision-details` 在 flex 列父下被 `<button>` 默认 text-align:center 居中）；ENV-4 注释自述意图是"ride the same icon+label grid, left-aligned"故属回归。修：`.supervision-details` 加 `flex items-center justify-start`（tw.css 单文件）。
    - **formatElapsed 多日不进位**（P3·edge·`timeline.ts:1339`）：`formatElapsed` 封顶 `Xh Ym`，10 天 goal 读作 `249h 58m` 应 `10d 9h`（仍 seed）。
  **排除的刻意/低置信/无后端项（诚实记录）**：Scheduled next-run/mark-all-read（**均已实现**·live 未显仅测试数据/localStorage 假象·非差距）、Scheduled All/Active/Finished vs Paused（`Scheduled.tsx:16-21` 注释坐实刻意·无 pause 后端）、diff 增删行行号数字仍中性灰（finder C·nit·低价值）、diff `.fd-glyph` 实心色块 chip vs Codex 裸字母（`DiffView.tsx:48` 注释显 chip 为有意设计·可能刻意）、diff 路径等宽 vs 无衬线（等宽对齐好·不改）；及既往变更卡/产物卡/message-actions 已 parity、Environment Background/Browser/Sources 无后端、👍👎、A-M-D 彩徽标、topbar-pill(TH-15)、diff-del-hatch(QA-0718)、edgeToEdge 圆角卡、DIFF-SEARCH、Plugins-Sites-PR 等无后端或刻意项。
  **本轮定性**:**非失败轮** — 一批 3 并发 implementer 关闭 **3 个用户一眼可见的 Codex 差距**，横跨 home(标题字重轻量化贴金标)/scheduled(行操作 hover-reveal 修回归·去每行常驻噪音)/diff(文件名加粗·修与变更卡的自我不一致)。第 4 个（THREAD-2 单步中断）机制安全落地+单测但金标会话未可见翻转，已诚实标注并把 tool-dating 收尾半 seed 下轮。经 64 轮打磨真差距日益细，本轮亮点是先证伪了两条"看似差距实则已实现"的候选（避免做无用功），再关掉三条真实可见差距。
  - <2026-07-21 08:xx> 轮64：对标 home/thread+env/diff-split/scheduled(3 finder 并发)、关差距 HOME-HEADLINE-WEIGHT(P2·标题 fw600→400 贴金标·repo 等重)+ SCHED-MORE-HOVER(P2·行 ··· 常驻→hover-reveal·修迁移回归)+ DIFF-FILENAME-BOLD(P2·diff 文件名加粗·修与变更卡不一致)、THREAD-2-SINGLESTEP 机制落地但金标会话未可见翻转(诚实·seed tool-dating 收尾)、证伪 next-run/mark-all-read(均已实现)、排除 diff-rowno-color/fd-glyph-chip 等 nit/刻意、派工 3(并发 worktree·白名单互斥 tw.css / DiffView.tsx / timeline.ts)、push b452321a + 96b1ae37 + fab85163、live=index-FxBH-3py.js；seeded THREAD-2 tool-dating 收尾 + ENV-RUNDETAILS-ALIGN + formatElapsed-day-rollover 下轮

- 2026-07-21 轮65（headless）：**新鲜眼睛全屏重对标 home/rich-thread/diff-Review-split/scheduled × light/dark × 1440/390 + 逐组件裁图 + 3 并发 read-only finder**（diff-split / thread+environment / home+sidebar+scheduled）。R64 三 seed（ENV-RUNDETAILS-ALIGN / HOME-EXPLORE-ICON-BLUE / THREAD-2 tool-dating）已于 R64→R65 间落地（HEAD=305cf802·live 已含），本轮部署最新 HEAD 后重新对标。主驾驶自对标先证伪两条候选（diff 增行斜体=注释语法高亮刻意`tw.css:1076`·非 bug；composer placeholder "Do anything" 与 Codex 一致·finder 坐实我方任务描述里"Ask anything"写错）。关闭 **2 个用户一眼可见的实质差距**（一批 2 并发 worktree implementer，白名单两两互斥 `{Scheduled.tsx,Scheduled.suggest.test.tsx}` / `{timeline.ts,Timeline.tsx,其测试}`，各 vitest 绿 + build 绿，自 rebase 自推）:
  - ✅ **SCHED-SUGGEST-BOTTOM（P1·Scheduled 每次可见·finder 逐源码+截图坐实为 bug）**〔R65 关闭〕：Scheduled 屏 "Suggestions"（Daily brief/Weekly review/Follow-up monitor 三 canned 模板）被**内联插入到列表第 2 行真实 run 之后**（`Scheduled.tsx:85 SUGGESTION_INSERT_AFTER=2` + `:473 suggestionsInline` + `:735` 内联渲染），其**下方继续渲染第 3…N 个真实 run**（"Reply exactly INC66…"、"Check the repository health · Every 2s · Failed" 等 ×20+），视觉上真实 run 归属到 "Suggestions" 标题下、读作建议——语义污染。设计注释误读金标（Codex `codex-scheduled.jpg` 只恰好 2 条真实 task，Suggestions 之后天然为空，并非主动劈分）。修（implementer A·`Scheduled.tsx`+`suggest.test.tsx`）：删 `SUGGESTION_INSERT_AFTER`/`suggestionsInline`/`:735` 内联插入，把 `{suggestions}` 移入 `.scheduled-list` 内 map 之后**无条件末尾渲染**（成为列表终结子元素）；重写 suggest 测试位置断言为置底（4 真实行在前、`scheduled-suggestions` 为 `list.lastElementChild`）。**live 复验**(index-Dq0TG9bD→Hbwip7Rj)：DOM 断言 Suggestions 为 `.scheduled-list` 最后子元素(idx 29/30)、其后真实 run 行数=**0**；截图所有真实 run 连续列在前、"Suggestions" 标题在最底、其下仅 3 canned·与金标一致、console 0。合入 A push `cc19850b`（vitest 630 绿）。
  - ✅ **THREAD-2-SINGLESTEP（P1·长会话尾可扫读一致性·R64 seed 坐实并真机翻转）**〔R65 关闭·终于可见翻转〕：被 step-limit/中断收编的**单步** turn fold 头显 timeless `Worked · 1 step`（紧挨 "Step limit reached / Goal cancelled 00:34" banner），Codex 每 turn 头恒显 `Worked for <耗时>`。**thread-finder 逐 journal 事件坐实真因（推翻 R64 seed 的"缺 env.ts"假设）**：297d 尾部 assistant planning 气泡被 flush 到顶层→尾 fold 只剩 1 个 bash tool；tool 的 `activity_completed`(ts=07:42:19·跑了2分钟)没写进 ToolItem；唯一带终止时刻的 `limit_exceeded` echoChip 在进 `foldWork` **之前**就被 `SessionView.tsx:698 suppressEchoedChips({terminalAlert:true})` 删掉（因 banner 在场）→ fold `startMs==endMs==tool start`、span=0→退步数。**305cf802/fab85163 假绿根因**：单测把带 ts 终止 chip 未经 suppress 直接喂 foldWork 故 629 绿，live 管线先 suppress 掉它 → dead-on-arrival。修（implementer B·`timeline.ts`+`Timeline.tsx`+测试·免疫 suppress 的更稳修法）：`ToolItem` 增 `endTs?`，`activity_completed/failed/cancelled` 分支写 `t.endTs=env.ts`；`Timeline.tsx:foldSpanMs` 遍历 children 时对 tool 子项**同时纳入 `ts`(start)+`endTs`(end)** 两瞬时→单步中断 fold 从 tool 自身生命周期(07:40:19→07:42:19≈2m)自证 span，对 chip suppress 完全免疫。**关键诚实点**：新增 `foldOfSuppressed` 回归照搬真实管线顺序(`foldEvents→suppressEchoedChips({terminalAlert:true})断言 limit chip 确已删→foldWork`)、driver-shape 无 env.ts 使 `startMs==endMs`，证明翻转纯来自 tool ts→endTs·不依赖任何被 suppress 的 chip——这正是前次假绿从未断言的。**live 复验**(index-Hbwip7Rj·金标会话 297d)：尾部单步中断 turn 头 `Worked · 1 step`→**`Worked for 2m ›`**、"Step limit reached / Goal cancelled 00:34" banner 保留、settled turn 文案未回退、console 0。合入 B push `b46ec059`（vitest **637 绿**含 8+ 新回归）。
  两批白名单互斥（A: Scheduled.tsx+suggest.test；B: timeline.ts+Timeline.tsx+turns.test）。部署 8809 `live=index-Hbwip7Rj.js`(200)。**四闸门全绿**：vitest 637 绿、build 绿；A/B 目标差距 live 复比确关小无回退（SCHED afterRows=0 / THREAD-2 fold 头翻转 2m·settled 未退）；全景 home/scheduled/rich/diff × light/dark × 1440/390 稳态 console error+warning=**0**；qa/runs 存档 `2026-07-21-r65/{live,after}/`（live 8 屏 + diffsplit + after sched-{light,dark} + thread2-297d-tail）+ 3 finder 报告。
  **让路**：并发 session 推 `0450b893`(INC-86·默认 medium thinking·runtime 逻辑非 webui-visual·不冲突)，未碰。
  **证伪/排除的刻意/低置信/无后端项（诚实记录）**：diff 增行 `// generated` 斜体（=`.hl-com`/`.hljs-comment` 注释语法高亮·GitHub/VSCode/highlight.js 通用默认·刻意·非 bug·`tw.css:1076`）、composer placeholder "Do anything"（与 Codex 金标一致·finder 证伪任务描述的"Ask anything"）、composer branch chip 显 worktree 名（真实分支名·数据差非 bug）、Environment 头 ×(关面板)vs +(add env)/Worktree →(导航)vs ⌄(展开)（结构性刻意）、approval dot "Ask to approve" vs Codex "Full access" glyph（默认 posture 不同·合法）、diff 逐文件圆角卡 vs edge-to-edge（edgeToEdge=narrow 门控·刻意）、`.fd-glyph` 彩 chip、diff 行号/符号中性灰；及既往 Environment Background/Browser/Sources、👍👎、Scheduled Paused-tab/mark-all-read(已实现)/nextRunPhrase(已实现)、Plugins/Sites/PR 等无后端或刻意项。
  **仍开放 ☐（下轮 fresh-eyes 排·本轮 seed）**：formatElapsed 多日不进位(`timeline.ts:1339`·`249h 58m` 应 `10d 9h`·P3·edge·仍 seed)。
  **本轮定性**:**非失败轮** — 一批 2 并发 implementer 关闭 **2 个用户一眼可见的 P1 差距**：Scheduled Suggestions 置底(修真实 run 混入建议区的语义污染)+ THREAD-2 单步中断 fold 头真实耗时(**R64 诚实标注"机制落地但金标未翻转"的那条·本轮由 thread-finder 逐 journal 坐实真因·换免疫 suppress 的修法·终于在金标会话 297d 可见翻转 `Worked for 2m`**)。经 64 轮打磨真差距日益细，本轮两条均金标/journal 直证、且 THREAD-2 补上了前两轮的假绿窟窿。
  - <2026-07-21 09:xx> 轮65：对标 diff-split/thread+env/home+sidebar+scheduled(3 finder 并发)、关差距 SCHED-SUGGEST-BOTTOM(P1·Suggestions 内联劈分致真实 run 混入建议区→无条件置底终结区块·金标直证)+ THREAD-2-SINGLESTEP(P1·单步中断 fold 头 `1 step`→`Worked for 2m`·tool endTs 免疫 suppress·终于金标可见翻转·补前两轮假绿)、证伪 diff-italic(=注释高亮刻意)/placeholder(=Do anything 一致)、排除 branch-chip/env-header/approval-glyph 等刻意/无后端、让路 INC-86 0450b893、派工 2(并发 worktree·白名单互斥 Scheduled.tsx+suggest.test / timeline.ts+Timeline.tsx+turns.test)、push cc19850b + b46ec059、live=index-Hbwip7Rj.js；seeded formatElapsed-day-rollover 下轮

- 2026-07-21 轮66（交互轮·主驾驶自对标 + 2 并发 read-only finder）：**新鲜眼睛全屏重对标 home/rich-thread/diff-Review-split/scheduled/approval × light/dark × 1440/390**。主驾驶自对标在 approval 会话 live 直接抓到两条**单位不进位**的时长格式化差距(fold 头 `Worked for 116m 23s`、goal `252h 13m`),与 Codex 金标 `Worked for 1h 37m 40s` 直接背离;finder A（sidebar+scheduled）坐实 Scheduled 图标漂移。关闭 **2 个用户一眼可见的实质差距**（本轮主线串行落 2·各 vitest 绿 + build 绿·落一个推一个）:
  - ✅ **ELAPSED-UNIT-ROLLOVER（P1·长会话/长 goal 每次可见·Codex 金标直证）**〔R66 关闭〕：`formatWorkDuration`（`timeline.ts:273`）封顶 `Xm Ys`——116 分钟读作 **`116m 23s`** 而非 `1h 56m 23s`；`formatElapsed`（`timeline.ts:1386`）封顶 `Xh Ym`——10 天 goal 读作 **`252h 13m`** 而非 `10d 12h`。二者单位都不向上进位，长会话/长 goal 变成无界的分/时计数，与 Codex fold 头恒显的粗粒度 `Worked for 1h 37m 40s`（金标 `codex-crop-diff-rendering`/thread 系列多处「Worked for 1h 37m 40s」「1h 9m 3s」）背离。修（主线·`timeline.ts`+`timeline.test.ts`）：`formatWorkDuration` ≥60m 进位为 `1h 56m 23s`（秒为 0 时按既有规则省略→`1h 0m`/`2h 0m`）；`formatElapsed` ≥24h 进位为 `10d 12h`（seeded canonical 249h58m→`10d 9h`），<24h 仍 `Xh Ym`、<1h 仍 mm:ss。+2 回归块（639 绿）。**live 复验**(index-59mWJTDl·approval 会话 b98f)：fold 头 `116m 23s`→**`Worked for 1h 56m 23s`**、goal `252h 13m`→**`10d 12h`**、body 无残留 `116m`/`252h`、rich 会话短时长(16s/2m/4m 20s)零回退、console 0。合入 `d9f9462b`。
  - ✅ **SCH-ICON-TOP（P1/HIGH·Scheduled 每次可见·finder A 发现·金标直证）**〔R66 关闭〕：Scheduled 每行前导状态图标（○/▷/✓/!）在标题被 `WebkitLineClamp:2` 撑成两行时**漂到两行标题中间**——行 `<button>` 内联 `items-center`（`Scheduled.tsx:622`）在 utilities 层压过 `.scheduled-row` 组件本身的 `items-start`（`tw.css:941`），28px 图标槽相对「标题2行+副标题」整块居中而下沉。Codex 金标 `codex-scheduled.jpg`/`codex-crop-scheduled-list.jpg` 前导图标恒贴标题**首行**、最左状态列可纵向扫读。修（主线·`Scheduled.tsx`+`Scheduled.list.test.tsx`·单组件）：行 `items-center`→`items-start`、glyph span 加 `-mt-1`（28px 环光学居中于 20px `leading-5` 首行）。+1 回归锁定 `items-start`/无 `items-center`/glyph 带 `-mt-1`（list 套件 32 绿）。**live 复验**(index-CgpLFIVp)：glyph 中心 vs 标题首行中心 **delta=0px**（两行行+单行行、light+dark 皆 0）、截图前两条长标题行图标已顶贴首行、console 0。合入 `b50261d6`。
  主线串行落 2（白名单天然互斥：`timeline.ts`+其 test / `Scheduled.tsx`+其 list.test）。部署 8809 `live=index-CgpLFIVp.js`(200)。**四闸门全绿**：vitest 639 绿、build 绿；两目标差距 live 复比确关小无回退（时长文案两处翻转正确·图标 delta=0）；全景 home/scheduled/rich/diff/approval × light/dark × 1440/390 稳态 console error+warning=**0**；qa/runs 存档 `2026-07-21-r66/{live,after}/` + 2 finder 报告。
  **让路**：并发 INC-86 session 推 `52f2849e`(INC-86.2 文档并回·loop mode 真机 PASS)、工作区留有未提交 `internal/provider/gemini/*`(其 provider 加固·非 webui-visual)——未碰。
  **证伪/排除的刻意/低置信/无后端项（诚实记录）**：diff 纯增文件头 `−0` 噪声（finder C·55% 置信·DF-6 刻意「both numbers always rendered」·但两张定标 crop `+8 -4`/`+649 -57` 均为两侧非零、**无纯增 Codex header 金标可证**→本轮不做·seed 需真人补一张纯增 header 复核）、diff 暗色增行底色近同 `--panel`（`tw.css:45/103` 注释坐实刻意「免糊成绿墙」）、sidebar 项目名字重（finder A·MEDIUM·自标可能刻意·类 R64 已调字重项·本轮不动）、sidebar 会话行红点（我方刻意状态信号非 unread）；及既往 `.fd-glyph` 彩 chip、逐文件圆角卡 vs edge-to-edge、注释斜体、行号/符号中性灰、路径等宽、Environment Background/Browser/Sources 无后端、👍👎、Scheduled Paused-tab/mark-all-read/nextRunPhrase(已实现)、Plugins/Sites/PR 等无后端或刻意项。
  **仍开放 ☐（下轮 fresh-eyes 排·本轮 seed）**：diff 纯增/纯删文件头 `−0` 噪声（需纯增 Codex header 金标复核 DF-6 是否该对纯增文件省略零侧）。
  **本轮定性**:**非失败轮** — 主线串行关闭 **2 个用户一眼可见的 P1 差距**：时长格式化单位进位（fold 头 `116m 23s`→`1h 56m 23s`、goal `252h 13m`→`10d 12h`·消 seeded formatElapsed-day-rollover + 主驾驶新抓的 formatWorkDuration-hour-rollover·Codex 金标直证）+ Scheduled 状态图标顶贴标题首行（消两行标题图标漂移·delta 归零）。经 65 轮打磨真差距日益细，本轮两条均金标直证、且清掉了 seeded 项。
  - <2026-07-21 10:xx> 轮66：对标 home/rich/diff/scheduled/approval(2 finder 并发+主驾驶自对标)、关差距 ELAPSED-UNIT-ROLLOVER(P1·formatWorkDuration 分→时 `1h 56m 23s`·formatElapsed 时→天 `10d 12h`·金标直证)+ SCH-ICON-TOP(P1·Scheduled 状态图标顶贴标题首行·delta=0)、证伪/让 diff `−0` 噪声(无纯增金标·seed)、让路 INC-86 52f2849e+gemini/*、主线串行落 2(白名单互斥 timeline.ts / Scheduled.tsx)、push d9f9462b + b50261d6、live=index-CgpLFIVp.js

- 2026-07-21 轮67（headless·主驾驶自对标 + 3 并发 read-only finder）：**新鲜眼睛全屏重对标 home/rich-thread/diff-Review-split/scheduled/approval × light/dark × 1440/390 + 逐组件裁图（composer/scheduled-list/diff-rendering/change-card 等）+ 3 finder（diff-split / thread+env+composer / home+sidebar+scheduled）**。三 finder 一致证实经 66 轮打磨三大屏（diff/thread/composer/env）已高度 parity——diff finder 仅剩 1 低价 nit（`.fd-gap-caret` 无边框·且 `.fd-gap` 簇被反复打磨过·风险高不做）、thread finder 唯一候选（WorkedFold 排在产物卡前/后）样本是**中断态**会话、finder 自评低-中置信并强烈建议用**完成态**会话复现再定性（避免误报）→ 本轮不做·seed。主线关闭 **1 个用户一眼可见的实质差距**（单条高置信·主线串行·vitest 640 绿 + build 绿）:
  - ✅ **SCHED-ROW-NAME（P1·Scheduled 每次可见·finder 逐源码+金标坐实为漂移回归）**〔R67 关闭〕：Scheduled 每行标题渲染 **`r.displayTitle`（整段 instruction prompt）** 塞进 `WebkitLineClamp:2` 的 `-webkit-box`（`Scheduled.tsx:665-675`）——live 整屏是一摞**两行加粗中文段落碎片**，前两行 prompt 前缀近乎相同、根本无法扫读，正是 SC-13（`scheduledTitle.ts`）当初为消灭而写的「文本墙」。Codex 金标 `codex-crop-scheduled-list.jpg` 每行是 **2–4 词短名·单行**。**自相矛盾坐实**：行内 `title=` 注释（`Scheduled.tsx:643-644`）明写「the derived name is what the row SHOWS」，且 `scheduledTitle` 早已算好短派生名 `r.title`，却只接到右键菜单标题（`:748`）与 aria（`:727`），可见的 `<b>` 反用未截断 prompt。git 溯源：commit `19326eac "Unify mobile scheduled run rows"`（2026-07-13）以「use available mobile width before truncating」为由把 `<b>{r.title}</b>` 换成 `{r.displayTitle}` + 2 行 clamp、并加测试锁定——是**刻意但与 Codex 金标 + SC-13 教义双背离**的决策，其「移动端宽度」关切已被 `scheduledTitle` 的 48 字符 cap（≈行 copy 列宽）覆盖。修（主线·`Scheduled.tsx`+`Scheduled.list.test.tsx`·单组件）：`<b>` 渲染 `{r.title}` 短名、`className` 从 line-clamp 内联样式换 `truncate`（单行 nowrap+ellipsis）；删已死的 `displayTitle` 字段（interface `:146` + Omit `:260` + assignment `:284` + 其局部 `full` const）；重写「renders the full source title…line-clamp:2」测试为断言 `scheduledTitle` 短名·无 line-clamp·tooltip+search 仍载全文。**live 复验**(index-BXgnePCE·8809)：DOM 6 行标题全 `lines=1.00`/`white-space:nowrap`/`text-overflow:ellipsis`/`webkitLineClamp=none`；截图前三条长中文 prompt 收成单行带省略号、"Reply exactly INC66-INTERVAL-OK and nothing else"/"Check the repository health and report the…" 等清爽短名、行高 uniform、与金标一致、console 0。合入 `c4900e5c`（vitest 640 绿）。
  部署 8809 `live=index-BXgnePCE.js`(200)。**四闸门全绿**：vitest 640 绿、build 绿；目标差距 live 复比确关小无回退（DOM lines=1.00·clamp none·截图两行墙→单行短名）；全景 home/rich/diff/scheduled/approval × light/dark 稳态 console error+warning=**0**（502 瞬态 0）；qa/runs 存档 `2026-07-21-r67/{live,after}/`（live 12 屏 before + after scheduled-{light,dark,390} + DOM 探针）+ 3 finder 报告。
  **让路**：开轮 `git status` 干净、无并发脏文件。
  **证伪/排除的刻意/低置信/无后端项（诚实记录）**：diff `.fd-gap-caret` 无边框（finder 低置信·可能是 hover 态非真边框·`.fd-gap` 簇 DF-5 反复打磨过·风险高不做）、WorkedFold 排产物卡前/后（finder 低-中置信·样本为中断态会话·须完成态复现·seed）、Home project 名无虚线下划（finder 自评倾向不做·虚线暗示内联切换器未接线·与 R64 HOME-HEADLINE-WEIGHT 部分冲突）、Scheduled 每行 border-b 分隔线 vs Codex 纯留白（finder 中置信·jpg 压缩存疑·且是 SCHED-ROW-NAME 的连带·本轮先只做确凿的标题·分隔线留待复核）、`+ Create` 带边框 vs Codex 无边框（纯 nit）、`Approved · goal_status` 暴露内部工具名（RT-3 相关·文案非布局·低置信）；及既往 `.fd-glyph` 彩 chip、逐文件圆角卡 vs edge-to-edge、注释斜体、行号/符号中性灰、路径等宽、Environment Background/Browser/Sources 无后端、👍👎、Scheduled Paused-tab/mark-all-read/nextRunPhrase(已实现·`nextRunPhrase` 完整·live 显 Ran 仅测试数据无 nextRunAt)、Plugins/Sites/PR 等无后端或刻意项。
  **仍开放 ☐（下轮 fresh-eyes 排·本轮 seed）**：WorkedFold 折叠工作条 vs 产物卡的相对次序（须用**完成态**会话复现确认后再定性）；Scheduled 行 border-b 分隔线是否该撤（须复核 Codex 金标有无淡分隔线）；diff 纯增文件头 `−0` 噪声（仍 seed·需纯增 Codex header 金标）。
  **本轮定性**:**非失败轮** — 主线关闭 **1 个用户一眼可见的 P1 差距**：Scheduled 行标题从「整段 prompt 两行文本墙」复原为「SC-13 短派生名单行」（对齐 Codex 短名单列表观感·消 near-identical 前缀无法扫读·并撤销 19326eac 对 SC-13+金标的双背离漂移）。经 66 轮打磨真差距日益细，本轮亮点是三 finder 一致确认 diff/thread/composer 三屏高度 parity（避免在已 parity 处空转）、诚实把两条低置信候选（WorkedFold 次序需完成态复现、border-b 需金标复核）seed 下轮而非勉强做，只落这一条金标+源码双证的确凿差距。
  - <2026-07-21 10:xx> 轮67：对标 diff-split/thread+env+composer/home+sidebar+scheduled(3 finder 并发+主驾驶自对标)、关差距 SCHED-ROW-NAME(P1·Scheduled 行 displayTitle 整段 prompt 两行墙→r.title SC-13 短名单行·撤 19326eac 漂移·金标+源码双证)、证伪/让 fd-gap-caret(低置信)/WorkedFold 次序(须完成态复现·seed)/border-b(须金标复核·seed)/Home 下划线(倾向不做)、开轮工作区干净无让路、主线串行落 1(单组件 Scheduled.tsx)、push c4900e5c、live=index-BXgnePCE.js；seeded WorkedFold-order + sched-border-b + diff 纯增 `−0` 下轮

- 2026-07-21 轮68（headless·主驾驶自对标 + 3 并发 read-only finder）：**新鲜眼睛全屏重对标 home/rich-thread/diff-Review-split/scheduled/approval × light/dark × 1440/390 + 逐组件裁图 + 3 finder（home+sidebar / thread+composer+env / diff-split）**。主驾驶自对标在 Scheduled 屏金标（`codex-crop-scheduled-list.jpg`/`codex-scheduled.jpg`）直接坐实 R67 seed 的 SCHED-BORDER-B（Codex 真实 task 行是纯留白分隔·无 per-row rule）；thread finder 在变更卡坐实 Review 按钮遗漏描边胶囊。一批 2 并发 worktree implementer（白名单两两互斥 `{tw.css .scheduled-row 块 + Scheduled.list.test}` / `{ChangesOutcome.tsx}`），各 vitest 641 绿 + build 绿，落一个推一个：
  - ✅ **SCHED-BORDER-B（P2/P1·Scheduled 每次可见·金标+源码双证·消 R67 seed）**〔R68 关闭〕：Scheduled 每条真实 run 行被 1px `border-b border-line` 灰线分隔（`tw.css:943` `.scheduled-row { @apply border-b border-line; }`，注释 `:937-939` 自称"Codex parity: rows split by a 1px rule"）——但 Codex 金标 `codex-crop-scheduled-list.jpg`/`codex-scheduled.jpg` 显示真实 task 行**纯留白分隔、无任何 per-row 分割线**，注释的"1px rule"前提是错的；我方整屏读作"被灰线切开的表格行"，Codex 是"通透清单"。修（implementer A·`tw.css` 仅 `.scheduled-row` 块 + `Scheduled.list.test.tsx`）：删 `:943` per-row `border-b border-line`；真实行纵向 padding py-2.5→py-3（在后置 `.scheduled-row { @apply py-3; }` 覆写，shared `.scheduled-row, .sched-suggest` 仍 py-2.5，故 `.sched-suggest` 不受影响）；改写 `:937-944` 注释说明金标是纯留白+hover 分隔、原前提错误、R68 移除；hover:bg-panel-2/glyph/SCH-ICON-TOP(items-start+glyph -mt-1)/SCH-MORE-HOVER 全未动；新增回归断言 `.scheduled-row`/`.scheduled-row-wrap` className 不得再含 `border-b`/`border-line`。**live 复验**(index-DgwBq1XJ)：DOM 前 5 行 `borderBottomWidth=0px`(原 1px)、`paddingTop/Bottom=12px`(原 10px)；截图整屏灰线消失、行间纯留白通透、与金标一致、console 0。合入 A push `6b5d3965`（vitest 641 绿）。
  - ✅ **CHANGE-CARD-REVIEW-BTN（P1·富会话变更卡每次可见·金标+同文件 sibling 双证）**〔R68 关闭〕：「Edited N files」变更卡右上 `Review`（进入完整 diff 审阅）在 `ChangesOutcome.tsx:503` 只有 `shrink-0 px-[10px]`——无 border/rounded/height/hover；因 tw.css @layer base 给每个 `<button>` 兜底 `background:var(--panel)`，Review 在同为 panel 底色的卡片上渲染成一块**与卡同色、无边框的隐形平板**，和纯文本 Undo 几乎无法区分。Codex 金标 `codex-crop-change-card.jpg` 里 Review 是**描边圆角胶囊**、Undo 纯文本，主/次分明。非刻意佐证：git log/注释仅有行为决策(`:460` jump-to-full-diff)无"borderless"样式决策，且同文件 sibling「Open in」(`:189`)正是这套胶囊——属遗漏。修（implementer B·仅 `ChangesOutcome.tsx:503`）：className 改为复用 `:189` 胶囊 `inline-flex items-center shrink-0 px-[11px] h-[30px] rounded-[8px] border border-line text-[13px] text-ink hover:bg-panel-2`；Undo(`:497` border-0 bg-transparent)保持纯文本维持层级。**live 复验**(index-DgwBq1XJ·297d 会话)：Review 按钮 computed `borderWidth=1px`/`borderRadius=8px`/`background=白`/`height=30px`/`borderColor=rgb(220,220,220)`=描边胶囊；截图变更卡 Review 描边胶囊紧邻纯文本 Undo ↺、主/次层级清晰、console 0。合入 B push `08e3795e`（vitest 641 绿）。
  两批白名单互斥（A: tw.css .scheduled-row 块 + Scheduled.list.test；B: ChangesOutcome.tsx）。部署 8809 `live=index-DgwBq1XJ.js`(200)。**四闸门全绿**：vitest 641 绿、build 绿；A/B 目标差距 live 复比确关小无回退（sched border 0px·row py 12px / Review pill border 1px radius 8px）；全景 home/rich/diff/scheduled/approval × light/dark × 1440/390 稳态 console error+warning=**0**；qa/runs 存档 `2026-07-21-r68/{live,after}/`（live 12 屏 + after sched-{light,dark} + rich + rich-changecard + DOM 探针）+ 3 finder 报告。
  **让路**：开轮 `git status` 干净、无并发脏文件。
  **证伪/排除的刻意/低置信/无后端项（诚实记录）**：diff-split finder 三 crop 逐像素比对判**高度 parity 无当轮可关差距**（文件头字母 chip=RV-5 刻意`DiffView.tsx:57-61`、文件头 caret=RV-3 刻意`:252-254`、Review 标位置=DIFF-REVIEW-LABEL 补位、band @@ 尾巴/`−` 统一 均刻意；唯一低置信 find-in-diff 未接 diff 面板`SessionView.tsx:876`属键盘落空非可见 UI·不建议）；thread finder 判三大屏高度 parity（tail「Goal achieved in N」内联验收=中断态样本不可比·`Timeline.tsx:1344-1355` 完成态逻辑已实现·须完成态复现·seed·低置信）；home finder 判 home 首页高度 parity 无可执行差距。及既往 composer `Access: set by agent spec`/Environment 头 ×/👍👎/`.fd-glyph` 彩 chip/逐文件圆角卡/注释斜体/行号中性灰/Environment Background-Browser-Sources 无后端/Scheduled Finished-tab/Plugins-Sites-PR 无后端 等刻意/无后端项。
  **仍开放 ☐（下轮 fresh-eyes 排·本轮 seed）**：SIDEBAR-INK-WEIGHT（home finder 中置信·sidebar nav/heading/session 静息态继承 `--ink-2`#606060 发灰 vs Codex 近黑#1a1a1a·`Sidebar.tsx:399-405/470-471/305-306`·可内联 text-ink 盖过·**但 a11y-adjacent 有被判对比度划水风险·下轮先判是否层级决策再定**）；SIDEBAR-FOLDER-ICON（home finder 中低置信·展开组用 FolderOpen vs Codex 恒闭合 Folder·`Sidebar.tsx:495`·细节）；`.project-session` 密度 `text-[13px] h-[32px]` vs Codex 更疏朗（tw.css base·可能刻意密度·须复核）；WorkedFold 折叠条 vs 产物卡次序（须完成态会话复现·seed 续）；diff 纯增文件头 `−0`（注：本轮变更卡金标 `codex-crop-change-card.jpg` 纯增文件显 `+7 -0`/`+8 -0`·证 `-0` 是 Codex 行为·我方一致·此 seed 或可撤·下轮复核 diff 面板专属金标）。
  **本轮定性**:**非失败轮** — 一批 2 并发 implementer 关闭 **2 个用户一眼可见的实质差距**：Scheduled 行分割线（一摞表格灰线→通透纯留白清单·对齐 Codex·消 R67 seed·金标坐实原注释前提错误）+ 变更卡 Review 描边胶囊（隐形平板→描边圆角按钮·恢复主/次层级 affordance·复用同文件 Open in pill）。三 finder 一致证实 diff/thread/home 三面已高度 parity（避免在已 parity 处空转），诚实把 home 两条中/中低置信候选 seed 下轮而非勉强做。
  - <2026-07-21 轮68>：对标 home+sidebar/thread+composer+env/diff-split(3 finder 并发+主驾驶自对标)、关差距 SCHED-BORDER-B(P2/P1·Scheduled 行 per-row border→纯留白分隔·消 R67 seed·金标+源码双证)+ CHANGE-CARD-REVIEW-BTN(P1·变更卡 Review 隐形平板→描边胶囊·复用 Open in pill·金标+sibling 双证)、证伪 diff-split 高度 parity/thread 高度 parity/home 首页无差距、开轮工作区干净无让路、一批 2 并发 worktree(白名单互斥 tw.css .scheduled-row 块+test / ChangesOutcome.tsx)、push 6b5d3965 + 08e3795e、live=index-DgwBq1XJ.js；seeded SIDEBAR-INK-WEIGHT + SIDEBAR-FOLDER-ICON + project-session 密度 + WorkedFold 次序 + diff `−0`(或可撤) 下轮

- 2026-07-21 轮69（headless·主驾驶自对标 + 3 并发 read-only finder）：**新鲜眼睛全屏重对标 home/rich-thread/diff-Review-split/scheduled × light/dark × 1440/390 + 逐组件裁图（sidebar-nav/sidebar-projects/composer 等）+ 3 finder（sidebar / composer+menu / diff-split+scheduled）**。主驾驶自对标在 sidebar-nav 金标（`codex-crop-sidebar-nav.jpg`）直接坐实 R68 seed 的 SIDEBAR-INK-WEIGHT——Codex sidebar 里 session/pinned/project 名恒为**近黑**，只有 Pinned/Projects 分节标签是灰；finder B 在 composer-crop 金标坐实 chip label 亦近黑。一批 2 并发 worktree implementer（白名单两两互斥 `{tw.css}` / `{Sidebar.tsx + Sidebar.nav.test.tsx}`），各 vitest 绿 + build 绿，落一个推一个：
  - ✅ **SIDEBAR-INK-WEIGHT + CHIP-INK（P1·sidebar 全树 + composer chip 每次可见·金标+双 finder 坐实·消 R68 seed）**〔R69 关闭〕：sidebar 里 `.project-session`（`tw.css:333`）与 `.project-heading`（`tw.css:309`）静息态继承 `--ink-2`=#606060 发灰——整棵 Projects 树读作"developer-preview 灰"，与 Codex 金标 `codex-crop-sidebar-nav.jpg`/`codex-crop-sidebar-projects.jpg`（session/pinned/project 名恒**近黑** ~#2b2b2b、清爽可扫读、只有分节标签灰）背离；同理 composer env chip label `.cx-env-value`（`tw.css:426`）随父 `.cx-env-control` text-ink-2 发灰，Codex `codex-crop-composer.jpg` 的 `agentrunner/main` 等 chip label 亦近黑（two-tone：label 黑、图标灰，与已实现的 `.cx-model-name` 同语言）。**非刻意佐证**：git log -S 显示 text-ink-2 源自最初"rebuild UX"泛化 commit，无 QA/INC/DF/RV 依据、无注释自称刻意；`:427-430` 旧注释"env chips unaffected"仅是 model-pill 加重时的 scope 说明、非"env 该留灰"决策。修（implementer A·仅 `tw.css`·3 处+注释刷新）：`.project-heading`/`.project-session`/`.cx-env-value` 的 text-ink-2→text-ink（近黑）；`.section-label` dim（分节标签）、`:331` unread `font-semibold text-ink`、chip 图标（父 `.cx-env-control` text-ink-2）、access pill（`.cx-pill` 中性）**均不动**——层级/two-tone 保留；像素校准判 #0d0d0d 比 Codex ~#2b2b2b 略深但读作正常高对比正文、非死黑，故保持最小改动不引 --ink-1。**live 复验**(index-DlbSlVP4)：computed color session/heading/chipVal **rgb(96,96,96)→rgb(13,13,13)**（dark #d0d0db→#ececf1）、sectionLabel rgb(85,85,85) 灰不变、chipCtrl(图标) rgb(96,96,96) 灰不变；截图 sidebar 树近黑可扫读、chip label mt-test/Local 变深、分节标签仍灰、light+dark 均不发灰不死黑、console 0。合入 A push `e0be6df3`（vitest 641 绿）。
  - ✅ **SIDEBAR-FOLDER-ICON（P2/P1·sidebar 每次可见·金标坐实·消 R68 seed）**〔R69 关闭〕：展开的 project 组用「打开的文件夹」`FolderOpen`（`Sidebar.tsx:495` `{!folded ? <FolderOpen> : <Folder>}`）——图标列在展开/折叠间**闪烁两种形状**，比 Codex 噪；Codex 金标 `codex-crop-sidebar-projects.jpg` 每组**恒闭合** `Folder`（已展开的 alphatrade2/agentrunner 仍闭合），展开态由左侧 caret 编码、图标列安静统一。修（implementer B·`Sidebar.tsx` + `Sidebar.nav.test.tsx`）：`:495` 去三元、无条件渲染闭合 `<Folder>`；保留 `:10` FolderOpen import（`:255` "Finder" open-in 仍用）、caret 逻辑不动；加回归断言"展开组与折叠组 `.proj-folder` innerHTML 全等（不再切 FolderOpen）+ 展开组有 `.proj-caret.open`"。**live 复验**(index-DlbSlVP4)：截图前 mt-test/workspace/editable_mermaid2 展开组前导图标全为闭合文件夹、与折叠组一致、console 0。合入 B push `65e8a749`（vitest 642 绿）。
  两批白名单互斥（A: `tw.css`；B: `Sidebar.tsx`+`Sidebar.nav.test.tsx`）。部署 8809 `live=index-DlbSlVP4.js`(200)。**四闸门全绿**：vitest 642 绿、build 绿；A/B 目标差距 live 复比确关小无回退（session/chip 近黑 rgb(13,13,13)·分节标签/图标层级保留·文件夹恒闭合）；home/rich × light/dark 稳态 console error+warning=**0**；qa/runs 存档 `2026-07-21-r69/{shoot,verify}.py + live 16 屏 + after-home/rich × light/dark + computed-color 探针`+3 finder 报告。
  **让路**：开轮 `git status` 干净、无并发脏文件。
  **证伪/撤销的 seed 与刻意/无后端项（诚实记录）**：finder C 逐像素 + 真 fold-band 样本（`diff-mod-light-1440.png`）**证伪并撤销 3 个 R66/R67 seed**——DIFF-GAP-LABEL-XALIGN（`.fd-gap` 现为 `calc(5ch+18px)` 与 `.dl` 18px+5ch 精确对齐，35px/27px 旧值 DF-5 已修；仅剩 context 行前导空 `.dl-sign` 致 label 字符左缘偏 ~1ch 的容差内残留，成因刻意统一 sign 列，不做）、DIFF-REVIEW-MODE-LABEL（Review pill 已实现 `DiffView.tsx:797-802`）、diff 纯增 `−0`（`FileHead` 两数恒显、与 Codex 一致）。finder B/C 判 composer/diff/scheduled 高度 parity。及既往刻意/无后端：composer env-chip 4th（无后端·`Composer.tsx:1111`）、`Ask to approve` vs `Full access`（DEFAULT_ACCESS 刻意·INC-23.B4）、add-menu Plugins 组（无后端·ca2a2494 已删壳）、model-dropdown Advanced（低置信）、send 按钮 disabled 暗盘 vs Codex 浅灰（finder B 中置信·seed 下轮）、Scheduled Finished-tab（SC-11 刻意）/Suggestions microcopy（email/calendar 无后端）、`.fd-gap-context` @@ 尾巴（RVW-HUNKBAND 刻意）、`.project-session` 密度（finder A 低置信·须等比裁图·不做）。
  **仍开放 ☐（下轮 fresh-eyes 排·本轮 seed）**：SEND-BTN-DISABLED（finder B 中置信·`.cx-send:disabled` `tw.css:449-450` bg-accent#0b0b0b+opacity-40=暗盘+白箭头 vs Codex 浅灰盘+灰箭头·金标 `codex-crop-composer.jpg` 坐实·可改 disabled 变体为 bg-panel-2 text-dim·下轮判是否值得做）；MODEL-DROPDOWN-ADVANCED（finder B 低置信·Advanced 页脚 caret 右对齐+跳子页 vs Codex 内联上 caret 就地展开·`Composer.tsx:1570`）；WorkedFold 折叠条 vs 产物卡次序（须完成态会话复现·seed 续）；`.dl-sign` fold-label 字符对齐（极低价·finder C 记录）。
  **本轮定性**:**非失败轮** — 一批 2 并发 implementer 关闭 **2 个用户一眼可见的实质差距**：sidebar 全树 + composer chip label 从灰(#606060)→近黑（"developer-preview 灰列表"→"清爽可扫读"·对齐 Codex ink 层级·消 R68 seed·双 finder+金标坐实非刻意）+ 项目组文件夹图标恒闭合（展开态图标闪烁→统一闭合·消 R68 seed·金标坐实）。三 finder 一致证实 diff/composer/scheduled 已高度 parity（避免在已 parity 处空转），并**证伪撤销了 3 个陈旧 seed**（DIFF-GAP-LABEL-XALIGN/REVIEW-MODE-LABEL/−0）而非勉强做，诚实把 send-btn-disabled 等中低置信候选 seed 下轮。
  - <2026-07-21 轮69>：对标 sidebar/composer+menu/diff-split+scheduled(3 finder 并发+主驾驶自对标)、关差距 SIDEBAR-INK-WEIGHT+CHIP-INK(P1·sidebar 全树+chip label 灰#606060→近黑·two-tone/分节层级保留·消 R68 seed·双 finder+金标坐实非刻意)+ SIDEBAR-FOLDER-ICON(P2/P1·项目组 FolderOpen→恒闭合 Folder·消 R68 seed·金标坐实)、证伪撤销 3 陈旧 seed(DIFF-GAP-LABEL-XALIGN/REVIEW-MODE-LABEL/diff −0)、开轮工作区干净无让路、一批 2 并发 worktree(白名单互斥 tw.css / Sidebar.tsx+nav.test)、push e0be6df3 + 65e8a749、live=index-DlbSlVP4.js；seeded SEND-BTN-DISABLED + MODEL-DROPDOWN-ADVANCED + WorkedFold 次序 下轮

- 2026-07-21 轮70（headless·主驾驶自对标 + 3 并发 read-only finder）：**新鲜眼睛全屏重对标 home/rich-thread/diff-Review-split/scheduled/approval × light/dark × 1440/390 + 逐组件裁图 + 3 finder（composer+model+add / diff-split / scheduled+thread+env）**。三 finder 一致判三大功能面总体高度 parity，各诚实交出低-中置信可关差距；主驾驶自对标在 diff-rendering 金标坐实 gutter 未染色、在 composer 金标坐实 send disabled 发虚 + chip 图标不齐。一批 2 并发 worktree implementer（白名单两两互斥 `{tw.css}` / `{Composer.tsx}`），各 vitest 642 绿 + build 绿，落一个推一个：
  - ✅ **DIFF-GUTTER-COLOR（P1·最重要屏·每个 modified diff 都可见·金标+源码双证）**〔R70 关闭〕：diff 视图里 changed 行只染背景，行号列 `.dl-no`（`tw.css:830`）与符号列 `.dl-sign`（`:836`）恒 `text-dim`（灰）——Codex 金标 `codex-crop-diff-rendering.jpg` 清晰显示**删除行行号(1204)+`-`号红、新增行行号(1204-1207)+`+`号绿、context 行号灰**，形成一条红/绿"配色脊"一眼扫出改动行；我方缺此脊。非刻意佐证：`:827-829` 注释只否定过删除行 **marker 斜纹**(repeating-gradient·深色刺眼)非行号文字色，且 `:1266-1267` `.cx-dl.add/.del`(composer 内联预览)本就染 sign/text 只是 `.dl`(DiffView)吃不到→属遗漏。修（implementer A·仅 `tw.css`·`.dl-sign` 后追加 8 行）：inline `.dl.add .dl-no, .dl.add .dl-sign { color: var(--green) }` / `.dl.del ... { color: var(--red) }`；**split 视图也一并染**（未留 follow-up）——符号落在带 add/del 的 `.dls-half` 内用 `.fd-split .dls-half.add/.del .dl-sign` 直选，裸行号紧邻其前用 `:has(+ .dls-half.add/.del)` 按右邻侧染，左右各自成立、context 侧无修饰类保持灰，全纯 CSS 未碰 DiffView.tsx；用与 `.dl-marker` 同源 `var(--green)`/`var(--red)`(light #167a3c/#b3261e·dark #6fd398/#f0938c)。**live 复验**(index-CUAabE4W·297d modified 会话 Review split)：computed delNo/delSign=rgb(179,38,30) 红·addNo/addSign=rgb(22,122,60) 绿·context 行号灰；截图删除行(13/91/171)行号+符号红、新增行绿、context 灰，与金标配色脊一致、console 0。合入 A push `f224b097`（vitest 642 绿）。
  - ✅ **SEND-BTN-DISABLED（P2·composer 每次可见·金标坐实·消 R69 seed）**〔R70 关闭〕：composer 送出按钮 `.cx-send`(`:454`)base `bg-accent`(近黑 rgb(11,11,11))，禁用态 `.cx-send:disabled { opacity-40 }`(`:455`)对**整盘**加 40% 透明——盘变中灰、白箭头 `text-accent-ink` 也淡到几乎不可读，affordance 塌掉。Codex 金标 `codex-crop-composer.jpg` 禁用送出是**浅灰盘 + 清晰可读灰箭头**(读作"未激活但在场")。修（implementer A·同批 `tw.css`·改 `:455` 一行·base 未动）：`opacity-40`→`border-line bg-panel-2 text-dim opacity-100 hover:bg-panel-2`=浅 panel-2 盘 + dim 箭头随主题自适应。**live 复验**：light send bg rgb(238,238,238)/箭头 rgb(85,85,85) 清晰·dark bg rgb(31,31,36)/箭头 rgb(160,160,173)·opacity=1(去发虚)；截图浅灰盘+清晰深灰箭头、启用态仍 accent 盘+白箭头未变、console 0。合入 A push `f224b097`（与 DIFF-GUTTER 同 commit）。
  - ✅ **COMPOSER-CHIP-ICON-SIZE（P2·composer chip 行每次可见·金标+实测双证）**〔R70 关闭〕：composer 上下文 chip 行图标尺寸不齐——项目 `FolderIcon`/`BranchIcon`(`Composer.tsx:1838-1839`)`size={13}`，中间 run-location chip(`:1193`)`size={17}`，实测 `.cx-env-strip` svg 宽 **[13,17,13]** 相邻差~30%，folder/branch 明显瘦小于中间。Codex 金标 `codex-crop-composer.jpg` 四 chip 图标统一~16px、读作一组。修（implementer B·仅 `Composer.tsx`·+4/-4）：**未** naive 把 const 改 17(那会连 popover 菜单项 FolderIcon`:1149/1173`、BranchIcon`:1246` 也放大、菜单里造出 17 vs 兄弟 15 的新不齐)——而是给 `FolderIcon`/`BranchIcon` 加可选 `size` prop 默认 13，只在两个 chip 触发处(`:1121`/`:1227`)传 `size={17}`，菜单项保持默认 15 不受影响。**live 复验**：`.cx-env-strip` svg 宽 [17,17,17](原 [13,17,13])、light+dark 一致；截图三 chip 图标等大、chip 行读作一组、console 0。合入 B push `e8a72936`（vitest 642 绿）。
  两批白名单互斥（A: `tw.css`；B: `Composer.tsx`）。部署 8809 `live=index-CUAabE4W.js`(200)。**四闸门全绿**：vitest 642 绿、build 绿；A/B/A 目标差距 live 复比确关小无回退（diff gutter 红/绿脊·send 浅盘清晰箭头 opacity1·chip 17px 统一）；全景 home/rich/diff/approval/scheduled × light/dark × 1440/390 稳态 console error+warning=**0**；qa/runs 存档 `2026-07-21-r70/{live 20 屏 + review-split + v-diff-gutter/v-sendbtn/v-chips + computed 探针 + 3 finder 报告(finder-composer/finder-diff/finder-sched)}`。
  **让路**：开轮 `git status` 干净、无并发脏文件。
  **证伪/排除的 seed 与刻意/无后端项（诚实记录）**：sched finder **证伪 SCHED-SUGGEST-LABEL**（`.sched-suggestions-title` 已 `normal-case`·`tw.css:926`·JSX 字面 `Suggestions`·live title-case·nothing to close）+ 判 Scheduled/thread+env 两屏高度 parity（Environment rail vs Codex 浮动 card=app 级 both-rails 约定刻意·Finished-tab=SC-11·mark-all-read 条件显=SC-21·microcopy=无 ChatGPT 品牌·Background/Browser/Sources=无后端）；diff finder 判 diff-split 高度 parity（文件头 M/A chip=RV-5·Review pill=DIFF-REVIEW-LABEL·纯增 `+0 −N`·fold band `calc(5ch+18px)` 已对齐·@@ band=RVW-HUNKBAND 均刻意·del 行 marker 斜纹已被 QA-0718 否决不做）；composer finder 判 add-menu Attach-Finder(desktop-shell 无后端)/Plugins(已删壳)/`Ask to approve`(INC-23.B4 刻意)/第4 env chip(无后端) 均排除。
  **仍开放 ☐（下轮 fresh-eyes 排·本轮 seed）**：SCHED-CREATE-PLUS（sched finder 低置信·Scheduled `+ Create ⌄` pill 带 `Plus` 图标 vs Codex `Create ⌄` 仅文本+caret·`Scheduled.tsx:518`·caret 已示菜单·`+` 冗余·可去 Plus·低价 nit）；MODEL-ROOT-LABEL-WEIGHT（composer finder 中低置信·model 下拉 Model/Effort/Speed 三 root 行 label regular vs Codex semibold·`Composer.tsx:1555-1568`+`.pop-title` 需 scope 到三 root 行不含 Model 列表·须对金标校准）；MODEL-ROOT-ADVANCED-CARET-ALIGN（composer finder 低置信·Advanced caret 经 `.pop-right ml-auto` 右对齐 vs Codex label 后内联·`Composer.tsx:1570-1572`·nit）；del 行 marker 斜纹(QA-0718 已否决·勿再 seed)；diff fold-band caret 无外框(diff finder 低价)；WorkedFold 折叠条 vs 产物卡次序(须完成态会话复现·seed 续)。
  **本轮定性**:**非失败轮** — 一批 2 并发 implementer 关闭 **3 个用户一眼可见的实质差距**：diff 行号/符号配色脊（最重要屏·灰 gutter→删红/增绿脊·inline+split 双落·金标坐实原是遗漏）+ 禁用送出按钮（opacity-40 发虚暗盘→浅灰盘清晰箭头·消 R69 seed）+ composer chip 图标统一 17px（[13,17,13]→[17,17,17]·消相邻 30% 不齐·且妥善 scope 避免连累菜单图标）。三 finder 一致证实 composer/diff/scheduled/thread 已高度 parity（避免在已 parity 处空转），并**证伪 SCHED-SUGGEST-LABEL** 陈旧 seed 而非勉强做，诚实把 3 条低-中置信候选 seed 下轮。
  - <2026-07-21 轮70>：对标 composer+model+add/diff-split/scheduled+thread+env(3 finder 并发+主驾驶自对标)、关差距 DIFF-GUTTER-COLOR(P1·最重要屏·changed 行号/符号灰→删红/增绿配色脊·inline+split 双落·金标+源码双证·遗漏非刻意)+ SEND-BTN-DISABLED(P2·禁用送出 opacity-40 发虚→浅灰盘清晰箭头·消 R69 seed·金标坐实)+ COMPOSER-CHIP-ICON-SIZE(P2·chip 图标 [13,17,13]→[17,17,17] 统一·scope 避连累菜单·金标+实测双证)、证伪 SCHED-SUGGEST-LABEL 陈旧 seed、判 scheduled/thread/diff-split 高度 parity、开轮工作区干净无让路、一批 2 并发 worktree(白名单互斥 tw.css / Composer.tsx)、push f224b097 + e8a72936、live=index-CUAabE4W.js；seeded SCHED-CREATE-PLUS + MODEL-ROOT-LABEL-WEIGHT + MODEL-ROOT-ADVANCED-CARET-ALIGN 下轮

- 2026-07-21 轮71（headless·主驾驶自对标 + 3 并发 read-only finder）：**新鲜眼睛重对标 + 主驾驶自截 live model 下拉/scheduled 直比金标 + 3 finder（thread+composer / diff-split / home+scheduled+sidebar）**。主驾驶自对标在 `codex-crop-model-dropdown.jpg` 金标直证 R70 seed 的两个 model 菜单差距（root label 该加粗、Advanced caret 该内联），在 `codex-scheduled.jpg` 金标直证 SCHED-CREATE-PLUS。一批 2 并发 worktree implementer（白名单两两互斥 `{Composer.tsx + tw.css + Composer.effort.test}` / `{Scheduled.tsx}`），各 vitest 绿 + build 绿，落一个推一个，关闭 **3 个用户可见的 model 菜单/scheduled 排版差距**：
  - ✅ **MODEL-ROOT-LABEL-WEIGHT（P1·model 下拉每次开都可见·金标+live 直证·消 R70 seed）**〔R71 关闭〕：composer model pill 弹出的下拉 root 页三行 Model/Effort/Speed 的 label 经 `.pop-title`（无 font-weight）渲染成 **regular（fontWeight 400）**，整菜单扁平无层级；Codex 金标 `codex-crop-model-dropdown.jpg` 三 root label 是**加粗（semibold）近黑**、值（Gemini Flash/Medium/Standard）灰右对齐，label/value 两级清晰。非刻意佐证：`.pop-title` 是全菜单共享的裸 truncate 类，无 QA/INC 依据要求 model root 该留 regular。修（implementer A·`Composer.tsx` root 分支把三 `<PopItem>` 包进新 `<div className="cx-model-roots">`·Advanced 留容器外 + `tw.css:492` 新增 `.cx-model-roots .pop-title { @apply font-semibold }`）：**只加粗 root 三行**，不连累全局 `.pop-title`/add-menu/model 子列表/Advanced。**live 复验**(index-DyhhBOrH)：three root `.pop-title` fontWeight **400→600**、Advanced 仍 400；截图 Model/Effort/Speed 加粗、与金标层级一致、light+dark console 0。合入 A push `4e75a397`（vitest 643 绿·含新断言）。
  - ✅ **MODEL-ROOT-ADVANCED-CARET-ALIGN（P2·model 下拉每次可见·金标+live 直证·消 R70 seed）**〔R71 关闭〕：同菜单底部 Advanced 行的 `^` caret 经 `right={...}` 落进 `.pop-right { ml-auto }` 被推到**行最右端**；Codex 金标里是「Advanced ^」——caret **紧跟 label 内联（左侧）**。修（implementer A·同批 `Composer.tsx:1571`）：把 CaretDown 从 `right` prop 移进 `title`（`<span className="inline-flex items-center gap-1">Advanced <CaretDown className="cx-model-adv-chev open"/></span>`），删 `right`，保留 `.cx-model-adv-chev.open` rotate（朝上=`^`）。**live 复验**：`.cx-model-advanced .pop-right` 不再存在(advHasRight=False)、caret 落在 `.pop-title` 内(advCaretInTitle=True)；截图 caret 紧跟 Advanced、与金标一致。合入 A push `4e75a397`（与 LABEL-WEIGHT 同 commit）。
  - ✅ **SCHED-CREATE-PLUS（P2·Scheduled 每次可见·金标+live 直证·消 R70 seed·home finder 高置信复证）**〔R71 关闭〕：Scheduled 屏右上 Create 按钮 `Scheduled.tsx:518` 渲染 `<Plus size={15}/> Create <CaretDown/>`＝「+ Create ⌄」；Codex 金标 `codex-scheduled.jpg`（home finder zoom 复证）是「Create ⌄」纯文字+caret、**无 Plus**——caret 已示菜单，`+` 冗余且暗示"单次 add"与 caret 的 disclosure 语义打架。修（implementer B·仅 `Scheduled.tsx`·:518 去 `<Plus/>` + :3 删 unused Plus import）。**live 复验**：Create 按钮 svg 数 2→1（仅 CaretDown）、text="Create"；截图纯文字+caret 对齐金标、console 0。合入 B push `f0041b6f`（vitest 642 绿）。
  两批白名单互斥（A: `Composer.tsx`+`tw.css`+`Composer.effort.test.tsx`；B: `Scheduled.tsx`）。部署 8809 `live=index-DyhhBOrH.js`(200)。**四闸门全绿**：vitest 643 绿、build 绿；A/A/B 目标差距 live 复比确关小无回退（root label 600·Advanced caret 内联·Create svg 1）；全景 home/rich/diff × light/dark 稳态 console error+warning=**0**；qa/runs 存档 `2026-07-21-r71/{live model-dropdown before + after model-dropdown/scheduled + verify/sweep 探针 + finder-home 报告}`。
  **让路**：开轮 `git status` 干净、无并发脏文件。
  **证伪/排除的刻意/低置信/无后端项（诚实记录）**：home finder 复证 SCHED-CREATE-PLUS 高置信（已关）+ 判 home/sidebar 高度 parity（folder 图标恒闭合 R69、近黑名 R69、section-label 灰刻意、home 空态卡 min-h-76/13px 是 R52 刻意密度·均不报）。
  **仍开放 ☐（下轮 fresh-eyes 排·本轮 seed）**：SCHED-SEARCH-PILL（home finder 中置信·Scheduled 搜索框 `.sched-search` `tw.css:957` `rounded-[10px]` h-9 方角矩形 vs Codex 金标全圆 pill 端·可 `rounded-full`·下轮判是否值得）；SCHED-TITLE-SIZE（home finder 低置信·`.page-heading h2` `tw.css:933` text-[22px] vs 金标略大~24px·heading:subtitle 1.7:1 vs 2:1·极低价 nit·须防回退 Create 垂直对齐）；WorkedFold 折叠条 vs 产物卡次序（须完成态会话复现·seed 续）；thread/diff finder 本轮跑至收轮未及回报·其发现下轮 fresh-eyes 重现。
  **本轮定性**:**非失败轮** — 一批 2 并发 implementer 关闭 **3 个用户可见的实质排版差距**：model 下拉 root label 加粗（扁平无层级→label/value 两级·消 R70 seed·金标+live 直证）+ Advanced caret 内联（右飘→紧跟 label·消 R70 seed）+ Scheduled Create 去冗余 Plus（+ Create ⌄→Create ⌄·消 R70 seed·三 finder 轮次金标复证）。主驾驶自对标金标直证三条陈旧 seed 全部确凿后一次清空，避免 seed 长期挂账;诚实把 home finder 新交的 sched-search pill / title-size 两 nit 与未及回报的 thread/diff finder 发现 seed 下轮。
  - <2026-07-21 轮71>：对标 model-dropdown/scheduled(主驾驶自对标金标+live 直证)+3 finder(thread/diff/home 并发)、关差距 MODEL-ROOT-LABEL-WEIGHT(P1·root label regular→semibold·scope cx-model-roots 不连累全局·消 R70 seed)+MODEL-ROOT-ADVANCED-CARET-ALIGN(P2·Advanced caret ml-auto 右飘→内联跟 label·消 R70 seed)+SCHED-CREATE-PLUS(P2·Create + Plus→纯文字+caret·消 R70 seed·金标复证)、判 home/sidebar 高度 parity、开轮工作区干净无让路、一批 2 并发 worktree(白名单互斥 Composer.tsx+tw.css+effort.test / Scheduled.tsx)、push 4e75a397 + f0041b6f、live=index-DyhhBOrH.js；seeded SCHED-SEARCH-PILL + SCHED-TITLE-SIZE + WorkedFold 次序 下轮

  - <2026-07-21 轮71 收轮后 finder 回报·登记下轮 seed>：thread finder + diff finder 收轮后交付(本轮未及纳入派工),诚实登记供下轮 fresh-eyes 优先:
    - **CHANGE-CARD-UNDO-INK（高置信·下轮领跑候选）**：富会话变更卡「Undo」label `ChangesOutcome.tsx:497` `text-ink-2`(#606060 灰)读作近禁用态,与同卡近黑标题(#030303)+描边 Review pill 失衡;Codex 金标 `codex-crop-change-card.jpg` 里 Undo 是近黑(#090909)、与 Review 读作对等 peer(只有 ↺ 图标安静)。Undo 是真实破坏性操作(丢弃全部改动),该给全 ink 权重。修:`text-ink-2`→`text-ink`(保 `hover:text-ink`)·同 R69 chip-ink 那类纯 className 改。**与 R70 已关的 CHANGE-CARD-REVIEW-BTN 同文件·下轮单独 implementer**。
    - COMPOSER-ACCESS-DOT-UNKNOWN（中置信·部分内部一致性）：session composer access chip risk="unknown" 时 `riskDot("unknown")` 无 `.risk-dot.unknown` CSS 规则(`tw.css:509-511` 仅 low/med/high)→7px dot 无背景不可见,"Access: set by agent spec" 前留幻影缩进;Codex access chip 恒有前导 glyph。修:加中性 `.risk-dot.unknown` 或 unknown 返回中性 icon。
    - MSG-COPY-OPACITY（中低·低价 nit）：常驻末条 message actions `.msg-copy` `tw.css:595` `text-dim opacity-60`(#999)比 Codex 常驻行(#848)淡;可仅对常驻行 `.msg.msg-last .msg-copy { opacity:1 }`。
    - DOC-CARD-RADIUS（低·nit）：doc/artifact 卡 `ChangesOutcome.tsx:250` `rounded-[8px]` vs 同 turn 变更卡 `rounded-[14px]` 不一致;可升 doc 卡到 14px。
    - DIFF-GUTTER-NUM-GAP（中/低·nit）：diff 行号到代码 glyph 间距 `.dl-no`/`.dl-text` 双 `px-2` + `.dl-sign` `px-[3px]` ≈19px 约 Codex 两倍;可收一侧 padding 到 ~10px。**注:diff finder 样本是 13 个单行 trivial 文件·未能深比 fold band/多 hunk/删除行·须真实多 hunk diff 会话复核**。
    - DIFF-FILEDIFF-SPACING（中·须先裁决卡模型）：`.filediff` `tw.css:809` `m-3`(24px 间距)比 Codex 密;仅收间距(mb-2)不动圆角卡·但与既定「逐文件圆角卡 vs edge-to-edge」刻意决策相邻·下轮先判卡模型是否已 settle。
    - diff finder 判 diff-split **高度 parity 无当轮可关高价差距**;两 finder 均确认 thread/composer/diff 经多轮打磨已高 parity。

  - <2026-07-21 轮72（headless·主驾驶自对标金标+live 直证·一批 2 并发 worktree implementer）>：**新鲜眼睛比对富会话变更卡对标 `codex-crop-change-card.jpg`——金标 Undo↺ 近黑、与 Review pill/标题对等 peer；live 直证我方 Undo=rgb(96,96,96)灰读作近禁用**，差距确凿。派工前逐条核实 R71 seed 与源码一致（非陈旧）。一批 2 并发 implementer（白名单两两互斥 `{ChangesOutcome.tsx}` / `{tw.css}`），各 vitest 643 绿 + build 绿，落一个推一个：
    - ✅ **CHANGE-CARD-UNDO-INK（P1·富会话每次可见·金标+live 双证·消 R71 领跑 seed）**〔R72 关闭〕：变更卡 Undo 按钮 `ChangesOutcome.tsx:497` `text-ink-2`(rgb96/#606060 灰) → `text-ink`(rgb13/#0d0d0d 近黑)，保 `hover:text-ink`。Undo 是真实破坏性操作（丢弃全部改动），应与 Review 对等、只留 ↺ 图标安静——金标正是如此。**live 复验**(index-BYnibrCN)：Undo/Review 同 rgb(13,13,13)、截图与金标一致、console 0。
    - ✅ **DOC-CARD-RADIUS（P3·nit·同文件搭车）**〔R72 关闭〕：documents 产物卡 `ChangesOutcome.tsx:250` `rounded-[8px]` → `rounded-[14px]`，与同 turn 变更卡圆角统一。live 复验 borderRadius=14px。合入 A push `c99e5cc9`。
    - ✅ **COMPOSER-ACCESS-DOT-UNKNOWN（P2·内部一致性 bug·消 R71 seed）**〔R72 关闭〕：composer access chip `risk="unknown"` 时前导 7px risk-dot 无 CSS 背景（`tw.css:509-511` 仅 low/med/high）→ dot 不可见、"Access: set by agent spec" 前留幻影缩进。追加 `.risk-dot.unknown { @apply bg-dim; }`（`bg-dim`/`--dim` 文件内已用），unknown 时 dot 有安静中性色。
    - ✅ **MSG-COPY-OPACITY（P3·nit·消 R71 seed）**〔R72 关闭〕：常驻末条 message copy 按钮 `.msg-copy`(`tw.css:596` `opacity-60`/#999) 比 Codex 常驻行淡。追加更具体规则 `.msg.msg-last .msg-copy { @apply opacity-100; }`（只提亮常驻末条，原全局 `.msg-copy` opacity-60 未动，非末条 hover 才亮的行为不变）。合入 B push `fdf41c1f`（初推被并发领先→rebase 干净→rebuild→推成功）。
  **本轮定性**:**非失败轮** — 一批 2 并发 implementer 关闭 **4 个用户可见的实质 UI/UX 差距**：变更卡 Undo 全 ink 权重（领跑·金标+live 双证·灰→近黑与 Review 对等）+ doc 卡圆角统一 14px + access chip unknown-risk dot 补中性色消幻影缩进 + 常驻末条 copy 常亮。四闸门绿：改动屏对金标复比确关小、全景 console(home/scheduled×1440/390) 0、qa/runs/2026-07-21-R72 存 before/after 截图。诚实：本轮四条含 2 个 nit（doc 圆角/msg-copy），但领跑 CHANGE-CARD-UNDO-INK 与 ACCESS-DOT-UNKNOWN 是清晰可见的实质差距，且 nit 与领跑同文件搭车零额外风险。
  - <2026-07-21 轮72>：对标变更卡金标(主驾驶自对标+live 直证)、关差距 CHANGE-CARD-UNDO-INK(P1·灰→近黑·消 R71 领跑 seed)+DOC-CARD-RADIUS(P3·8px→14px)+ACCESS-DOT-UNKNOWN(P2·补 .risk-dot.unknown·消 R71 seed)+MSG-COPY-OPACITY(P3·末条常亮·消 R71 seed)、开轮工作区干净无让路、一批 2 并发 worktree(白名单互斥 ChangesOutcome.tsx / tw.css)、push c99e5cc9 + fdf41c1f、live=index-BYnibrCN.js；diff/env finder(a6dc)后台跑供下轮 seed

  - <2026-07-21 轮72 收轮后 diff/env finder(a6dc)回报·登记 R73 seed>：diff/review split + Environment 面板镜头,供下轮 fresh-eyes 优先排序(read-only,均对着 codex-crop 金标):
    - **DIFF-SCOPE-CHROMELESS（高置信·下轮领跑候选）**：diff toolbar scope 选择器 `DiffView.tsx:568-583` + `tw.css:784-785` `.diff-scope-trigger` = `rounded-[6px] border border-line bg-panel-2 px-2 py-1` 描边填充 pill；Codex 金标 `codex-crop-diff-header.jpg` 是裸文字 `Last Turn ⌄`(无边无填、flush 左缘、只 label+caret)。toolbar 最显眼控件我方读作盒装按钮、Codex 读作纯标题,与真正盒装的 commit pill/viewtoggle 抢视觉重量。修:`.diff-scope-trigger` 去 `border border-line bg-panel-2`(留 text-ink+caret+hover tint),`.active` 仅 popover 开时给微 bg。源自 blanket tailwind 迁移、无 defending 注释。
    - DIFF-FD-GLYPH-BARE（中·须 fresh-eyes 确认·独立于卡模型裁决）：file header 状态 glyph `DiffView.tsx:255-257,49-55` + `tw.css:814-817` `.fd-glyph` = 18px `rounded-[5px]` 饱和 soft-chip(bg-green/red/blue-soft);Codex `codex-crop-diff-rendering.jpg` 是裸 mono-dark glyph `M↓`/`A`/`D`(无背景)。每个 file header 首元素我方是彩色 tile、Codex 是安静 inline glyph,且 `+8 −4` 计数已带增删色信号、tile 重复着色。修:剥 `.fd-glyph` 的 bg-*-soft/rounded/固定 18px 盒,渲成 `.fd-glyph-*` 着色的 inline mono 文字(可缀状态箭头)。
    - ENV-BGPROC-ICON（中）：Environment `Background processes` 行 `SupervisionPanel.tsx:302-304` + `tw.css:346-347` 用 7px 填充绿 `status-dot run`,打断相邻 Changes/Worktree/Create branch 用的 14px 线性 icon column;Codex `codex-thread-environment-panel.jpg` 该行用小 mono terminal glyph、与其他 env 行 icon 对齐。修:`status-dot`→Phosphor Terminal/Play `size={14}` 归到 env-row icon gutter。(附:label "Background work" vs Codex "Background processes" 次级 microcopy nit,低价。)
    - DIFF-MINUS-HYPHEN（中·须先裁决是否刻意）：删除计数与删除行前缀我方统一用 U+2212 `−`(偏宽偏高) `DiffView.tsx:126,269,813` + `SupervisionPanel.tsx:788` + `ChangesOutcome.tsx:486,546`;Codex 一律 ASCII hyphen `-`(与 `+` 光学成对)。修:`−`→`-` 全处。源自 "± counts" commit 的泛化选择,非 defended。**下轮先判是否有意用真减号**。
    - DIFF-FOLD-BAND-WEIGHT（中低·最软）：unmodified fold band `tw.css:886,888,893` `.fd-gap-label` = `text-[12px] text-dim`(最淡) 而 `.fd-gap-caret` = `text-ink font-medium`(最重),强调倒置(响 caret、怯 label);Codex `codex-crop-diff-rendering.jpg` 该 band 是可读中灰 count 行 + 从属细 caret。count 是信息却最淡、caret 最重误导视线。修:`.fd-gap-label` text-dim→text-ink-2;`.fd-gap-caret` text-ink font-medium→text-dim/常规。
    - ✂ 排除的刻意决策:ENV-SECTION-HAIRLINE(`tw.css:734-735` `.supervision-section border-b`,注释 `RVW: 发丝分隔分组` 有 defense——**但 finder 疑该注释前提"Codex 一张卡扁平清单"是在说 thread 变更卡、非 Environment rail;金标 env 面板分组无 inter-section rule,值下轮 fresh 裁决**);`.dl-hunk` 低调灰标题+删行红 accent bar(QA-0718 defended);DIFF-GUTTER-NUM-GAP/DIFF-FILEDIFF-SPACING(已 seed 未重报)。

  - <2026-07-21 轮73（headless·主驾驶自对标 codex-crop-diff-header + codex-crop-diff-rendering 金标 + live 双证·一批 2 并发 worktree implementer）>：**新鲜眼睛截 live diff split + Environment 面板对标金标——三处 diff 差距 + 一处 env 差距金标+live 双证确凿**。派工前逐条核实 R73 seed 源码一致（非陈旧）：`.diff-scope-trigger`(tw.css:786) 确为盒装 pill、`.fd-glyph`(816) 确为彩色 soft-chip tile、`.fd-gap-label/caret`(890/895) 确为强调倒置、bgproc 行(SupervisionPanel:303) 确为 7px status-dot。一批 2 并发 implementer（白名单两两互斥 `{tw.css}` / `{SupervisionPanel.tsx}`），各 vitest 643 绿 + build 绿，落一个推一个：
    - ✅ **DIFF-SCOPE-CHROMELESS（P1 领跑·diff toolbar 最显眼控件·金标+live 双证·消 R73 领跑 seed）**〔R73 关闭〕：diff toolbar scope 选择器 `.diff-scope-trigger`(tw.css:786) 去掉 `border border-line bg-panel-2`（保 text-ink+caret+px-2 py-1 命中区+rounded），hover tint 改 `hover:bg-panel-2`，`.active` 去 shadow-sm 只留微 bg。**live 复验**(index-C-Y2V-F9)：`Last Turn ⌄` 现读作裸文字标题、与右侧真盒装 Commit or push pill 拉开层级，与金标 codex-crop-diff-header 一致。合入 A push `51b71514`。
    - ✅ **DIFF-FD-GLYPH-BARE（P2·文件头首元素·消 R73 seed）**〔R73 关闭〕：`.fd-glyph`(816) 剥 `grid h-[18px] w-[18px] rounded-[5px] place-items-center` 盒→`inline-flex items-center text-[11px]`，`.fd-glyph-*`(817-819) 去 `bg-*-soft` 保 `text-green/red/blue`。**live 复验**：文件头 `A coverage/lcov-0.js` 的 `A` 现为裸绿 mono 字母、无彩色方块 tile，与金标 codex-crop-diff-rendering 一致（+N −M 计数已带增删色信号，tile 是重复着色）。合入 A push `51b71514`。
    - ✅ **DIFF-FOLD-BAND-WEIGHT（P2·折叠带强调纠倒·消 R73 seed）**〔R73 关闭·CSS 已改·可视待多 hunk 会话〕：`.fd-gap-label`(895) text-dim→`text-ink-2`(可读中灰 count)、`.fd-gap-caret`(890) 去 `text-ink font-medium`→`text-dim`(从属 caret)。修正当前 count 最淡/caret 最重的强调倒置，对齐金标（count 可读、caret 从属）。**注:当前 diff 样本是 trivial 单行文件无 unmodified fold band·可视复验待真实多 hunk diff 会话**;CSS token 交换低风险、vitest 绿。合入 A push `51b71514`。
    - ✅ **ENV-BGPROC-ICON（P2·Environment 面板 Background processes 行·消 R73 seed）**〔R73 关闭·JSX 已改·可视待有活跃后台任务的会话〕：`SupervisionPanel.tsx:303` `<span className="status-dot run"/>`(7px 绿点) → `<Terminal size={14}/>`（补 Phosphor import）。因 `.background-row` 与 `.env-row` 共享布局(tw.css:742)，14px 图标**自动落到 env-row 图标 gutter**、零 CSS——对齐金标 codex-thread-environment-panel（该行 mono terminal glyph 与其他 env 行对齐）。**注:该行仅 `backgroundWork.length>0` 时渲染·diff 会话无后台任务故本轮未可视到该行**;JSX icon 换 + vitest 643 绿 + 结构性 gutter 对齐。合入 B push `8d208f30`。次要文案改名 Background work→Background processes 被 B 按纪律丢弃（会破坏白名单外 SupervisionPanel.test.tsx 3 处断言，为低价 nit 改白名单外文件不划算）。
  **本轮定性**:**非失败轮** — 一批 2 并发 implementer 关闭 **4 个用户可见的实质 diff/env UI/UX 差距**，其中领跑 DIFF-SCOPE-CHROMELESS 是 diff toolbar 最显眼控件（盒装 pill→裸文字，金标+live 双证、每次进 diff 可见）。四闸门：改动屏对金标复比确关小（scope 裸文字化 + fd-glyph 裸字化 live 直证）、全景 console(home/scheduled×light/dark + diff split) 0、qa/runs/2026-07-21-R74 存 before/after diff toolbar 截图。诚实：FOLD-BAND-WEIGHT/ENV-BGPROC-ICON 两条已落代码但因样本/门控未能本轮可视，标注可视复验待多 hunk / 有后台任务的会话；均低风险 token/JSX 交换、vitest 绿。
  - <2026-07-21 轮73>：对标 diff split + Environment 面板(主驾驶自对标 codex-crop-diff-header/diff-rendering/thread-environment-panel 金标 + live 双证)、关差距 DIFF-SCOPE-CHROMELESS(P1 领跑·盒装 pill→裸文字·消 R73 领跑 seed)+DIFF-FD-GLYPH-BARE(P2·彩色 tile→裸 mono 字母)+DIFF-FOLD-BAND-WEIGHT(P2·count/caret 强调纠倒·CSS 已改可视待多 hunk)+ENV-BGPROC-ICON(P2·7px 点→14px Terminal glyph·JSX 已改可视待后台任务)、开轮工作区干净无让路、一批 2 并发 worktree(白名单互斥 tw.css / SupervisionPanel.tsx)、push 51b71514 + 8d208f30、live=index-C-Y2V-F9.js
    **仍开放 ☐（下轮 fresh-eyes 排·本轮 seed）**：DIFF-MINUS-HYPHEN（中·金标双证 Codex 用 ASCII hyphen `-`、我方用 U+2212 `−`·跨 DiffView.tsx/ChangesOutcome.tsx/SupervisionPanel.tsx 多文件·须一个 implementer 统一改全处或分文件·先判是否有意用真减号·本轮 diff live 仍见 `+1 −0` 宽减号）；FOLD-BAND-WEIGHT 可视复验（须多 hunk diff 会话确认 count 可读/caret 从属）；ENV-BGPROC-ICON 可视复验（须有活跃 background work 的会话确认 Terminal glyph 对齐 gutter）；ENV-SECTION-HAIRLINE fresh 裁决（`.supervision-section border-b` 注释前提"Codex 一张卡扁平清单"疑指 thread 变更卡非 Environment rail·金标 env 面板分组无 inter-section rule·下轮 fresh 判是否去发丝线）；DIFF-FILEDIFF-SPACING（须先裁决逐文件圆角卡 vs edge-to-edge 卡模型是否 settle）。

  - <2026-07-21 轮74>：对标 diff/变更卡/Environment 计数 glyph（主驾驶自对标金标 `codex-crop-change-card.jpg`「Edited 31 files **+980 -317** / +7 -0」+ `codex-crop-diff-rendering.jpg` 文件头 **+8 -4** + 逐行 del gutter 窄 `-`，均 **ASCII hyphen** 双金标坐实）、关差距 **DIFF-MINUS-HYPHEN（消 R73 领跑 seed）**、开轮工作区干净无让路、一批 2 并发 worktree(白名单互斥 {DiffView.tsx+其测试} / {ChangesOutcome.tsx+SupervisionPanel.tsx+各测试})、push e9cf3778 + 40545fea、live=index-CXnmoElU.js
    - ✅ **DIFF-MINUS-HYPHEN（中→P1 领跑·渲染态减号字符对齐·消 R73 seed）**〔R74 关闭·live 双证〕：渲染态 U+2212 `−`（偏宽）→ ASCII hyphen `-`，共 7 处渲染点跨 3 组件：`DiffView.tsx` :126(逐行 del gutter marker)/:269/:813/:1056(3 处 `.del` 计数)、`ChangesOutcome.tsx` :486(变更卡 totalDel)/:545(逐文件 del)、`SupervisionPanel.tsx` :789(env del)。同步改 3 个测试文件断言（`DiffView.chrome.test.tsx` 9 处 `toBe("-N")` + 正则 `/[-+]0/`、`ChangesOutcome.test.tsx` 4 处、`SupervisionPanel.test.tsx` 1 处）。注释里的 `−` 保留未动（不影响渲染）。**live 复验**：部署 index-CXnmoElU.js 后 DOM 探针 `.del` 文本 = `['-3','-3','-0','-3']`(rich)/`['-0'…]`(diff)、`has U+2212: False`——每张 diff/变更卡/env 计数现与金标 `+980 -317`/`+8 -4` 一致的窄 ASCII hyphen。两 implementer 各自 vitest 643 绿 + build 绿 + 自推。合入 A `e9cf3778`(DiffView) / B `40545fea`(Changes+Env)。
  **本轮定性**:**非失败轮** — 一批 2 并发 implementer 关闭 **1 个跨 3 组件、每张 diff/变更卡/env 面板都可见的实质字符对齐差距(R73 领跑 seed)**：Codex 全用窄 ASCII hyphen、我方旧用偏宽 U+2212，视觉重量与间距不一致；改后 live DOM 探针直证 `has U+2212: False`、`.del` 全 `-N`。四闸门：改动屏对金标复比确关小(DOM 探针 + before/after 截图直证)、稳态 console(rich+diff × light/dark) **0**、qa/runs/2026-07-21-r74 存 before/after + DOM 探针输出、三层文档行齐活。诚实：这是字符级 glyph 对齐(非布局重排)，但跨全部 diff 表面、每次看 diff 都可见，属实质 parity 收敛非划水 nit。
    **仍开放 ☐（下轮 fresh-eyes 排·本轮 seed）**：SettingsAppearance.tsx :127「+ / −」/:215 `−`（diff-style toggle 标签仍 U+2212·判是否随 glyph 决策一并 ASCII 化·低价）；FOLD-BAND-WEIGHT / ENV-BGPROC-ICON 可视复验仍待多 hunk / 有后台任务会话；ENV-SECTION-HAIRLINE fresh 裁决；DIFF-FILEDIFF-SPACING（卡模型 settle 前不动）。home/sidebar/scheduled finder(a8af9de9) 回报（home/Scheduled 均高 parity·真差距全在 **sidebar 项目栏**，金标+live×dsf2 实测）：**① SIDE-HEADING-WEIGHT（高·下轮领跑）**：repo 组头名我方 `font-bold(700)` vs Codex 常规字重(≈400-500·靠 folder 图标非字重区分层级)·`.project-heading`/`.proj-heading-name` 继承基规则 bold(`tw.css:305-306/324`)·加 `font-normal`/至多 `font-medium`·注 `tw.css:315` 注释"keeps bold repo-name"误判金标(金标非粗)·`{tw.css}`；**② SIDE-NEST-ALIGN（高）**：嵌套 session 标题左缘 x=65-66 比 repo 名列 x=80 左移 7px(金标三行严格同列 x=82)·`.project-session-wrap.nested ml-4`(`tw.css:329`)增到 `ml-[23px]`(23+8=31 与 repo 列齐)·SB-6 注释本意对齐但差 7px·`{tw.css}`；**③ NAV-NEWSESSION-ACTIVE-FILL（中）**：首页时 "New session" 被判 active→`bg-panel-2` 填充+加粗(rail 顶常驻实心块)·Codex 首页 New task chromeless·`Sidebar.tsx:401` active 判据 `!currentSid && currentPage===key` 应排除 `key==="home"`(只 scheduled 等真实页留填充)·`tw.css:297` 不动·git 无 QA/INC 锚支持首页高亮·`{Sidebar.tsx}`。**批次建议**：①②同改 tw.css 且同区→并一张串行；③只碰 Sidebar.tsx→与①②可并发。低优不排：ACCOUNT-FOOTER-SHA(SB-12 刻意·单用户本地无账户体系)。已确认无差距勿再 seed：home 4 卡/标题字重、Scheduled 全屏(All/Active/Finished 第三 tab SC-11 刻意、行密度 R68 定)。截图 `qa/runs/2026-07-21-r74/finder/`。

  - <2026-07-21 轮75（headless·主驾驶自对标 R74 finder(a8af9de9) sidebar 金标 seed + live×dsf2 探针双证·一批 2 并发 worktree implementer）>：**新鲜眼睛截 live sidebar 项目栏对标金标——R74 finder 三条 sidebar seed 全部主驾驶 DOM 探针坐实**（before 探针：`.proj-heading-name` fontWeight=**700**、`.project-session-wrap.nested` marginLeft=**16px**、home 时 "New session" nav 按钮 `active`=**True**）。派工前逐条核实源码一致（`.project-heading` 继承基规则 `font-bold`、`.nested { ml-4 }`、`Sidebar.tsx:401` 判据 `!currentSid && currentPage===key`——均非陈旧）。一批 2 并发 implementer（白名单两两互斥 `{tw.css}` / `{Sidebar.tsx + Sidebar.nav.test.tsx}`），各 vitest 绿 + build 绿，落一个推一个：
    - ✅ **SIDE-HEADING-WEIGHT（高·领跑·消 R74 finder seed·金标+live 双证）**〔R75 关闭〕：repo 组头名过重的 shouty `font-bold`(700) → `.project-heading` override 显式加 `font-medium`(500) 覆盖继承。Codex 金标 repo 组头是常规字重（≈400-500·靠 folder 图标而非字重区分层级）。同步修正 `tw.css:315` 附近误导注释（"keeps bold repo-name treatment"→"uses medium normal-weight repo-name treatment, folder icon distinguishes hierarchy"）。**live 复验**(index-c79NlHez)：`.proj-heading-name` fontWeight=**500**。合入 A push `8db8bbc5`。
    - ✅ **SIDE-NEST-ALIGN（高·消 R74 finder seed）**〔R75 关闭〕：嵌套 session 标题左缘比 repo 名列左移 7px → `.project-session-wrap.nested` `ml-4`(16px) → `ml-[23px]`(16+7=23px) 对齐 repo 名列。金标三行严格同列。**live 复验**：nested marginLeft=**23px**。合入 A push `8db8bbc5`。
    - ✅ **NAV-NEWSESSION-ACTIVE-FILL（中·消 R74 finder seed）**〔R75 关闭〕：首页时 "New session" nav 项被判 active → `bg-panel-2` 实心填充（rail 顶常驻高亮块）；Codex 首页 New task chromeless。`Sidebar.tsx:401` 判据加 `&& key !== "home"`（home 是 New-session landing 落地页·不该像被选中页那样常驻高亮；Scheduled 等真实页仍保填充）。加 2 条 Sidebar.nav.test 锁行为（home 无 active / scheduled 有 active）。**live 复验**：home 时 New session `active`=**False**、Scheduled `active`=**False**。合入 B push `5c5b1f8c`（vitest 645 绿=643+2）。
  **本轮定性**:**非失败轮** — 一批 2 并发 implementer 关闭 **3 个用户可见的实质 sidebar UI/UX 差距**（全景每次看 sidebar 都可见）：领跑 SIDE-HEADING-WEIGHT 是 repo 组头字重（700 shouty bold→500 medium·金标+live DOM 探针双证）+ 嵌套 session 对齐 repo 名列（+7px）+ 首页 New session 去掉常驻实心高亮块（chromeless 对齐 Codex）。四闸门绿：改动屏对金标复比确关小（before/after DOM 探针 700→500 / 16→23px / True→False 三项直证）、全景 console(home/scheduled×light/dark) **0**、qa/runs/2026-07-21-r75 存 before/after sidebar 截图（light+dark）+ 探针输出、三层文档行齐活。诚实：三条均为字重/边距/判据的精准 token 交换（非大布局重排），但都在 sidebar 主对齐面、每次看侧栏可见，属实质 parity 收敛。
  - <2026-07-21 轮75>：对标 sidebar 项目栏（主驾驶自对标 R74 finder(a8af9de9) 金标 seed + live×dsf2 DOM 探针双证）、关差距 SIDE-HEADING-WEIGHT(高领跑·700→500 font-medium·消 finder seed)+SIDE-NEST-ALIGN(高·ml-4→ml-[23px] 对齐 repo 名列)+NAV-NEWSESSION-ACTIVE-FILL(中·首页 New session chromeless·Sidebar:401 加 key!=="home")、开轮工作区干净无让路、一批 2 并发 worktree(白名单互斥 tw.css / Sidebar.tsx+nav.test)、push 8db8bbc5 + 5c5b1f8c、live=index-c79NlHez.js
    **仍开放 ☐（下轮 fresh-eyes 排·本轮 seed）**：SettingsAppearance.tsx :127「+ / −」/:215 `−`（diff-style toggle 标签仍 U+2212·判是否随 R74 glyph 决策一并 ASCII 化·低价）；FOLD-BAND-WEIGHT / ENV-BGPROC-ICON 可视复验仍待多 hunk / 有后台任务会话；ENV-SECTION-HAIRLINE fresh 裁决；DIFF-FILEDIFF-SPACING（卡模型 settle 前不动）。sidebar 三条 R74 seed 本轮全清，下轮宜换屏（回 diff/富会话/composer 金标重截 live 找新差距）。已确认无差距勿再 seed：home 4 卡/标题字重、Scheduled 全屏、sidebar section-label 字重(r47 已 defended)。截图 qa/runs/2026-07-21-r75/。
