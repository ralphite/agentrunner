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
| `GET /api/sessions` | **242 ms**(中位/5 次)——侧栏首屏要等它 |

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
