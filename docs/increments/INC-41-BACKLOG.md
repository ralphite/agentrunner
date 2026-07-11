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
