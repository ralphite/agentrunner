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
- ☐ **SC-5(P1)Suggestions 的 cadence 是空头支票**:卡片写 `Weekdays at 8:00 AM`,点开的 run 表单
  **没有任何 schedule 字段**(`store.ts:10` ModalKind 只有 `task?`/`preset?`)。要么把 interval/cron 传下去
  并在 `Modals.tsx` 取作初值,要么把文案改成我们真能落地的。**跨 store/Modals,让路下轮。**
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
- ☐ **TH-5(P2)变更卡文件行不可点**,没有「跳到这个文件的 diff」:结构已对齐金标(dim 目录 + 粗 basename +
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
- ☐ **DF-5(P2)`N unmodified lines` 折叠条没对齐代码栅格**:金标里 caret 装在占满行号沟槽宽的描边小格、
  label 从代码列起始处开始(读作"代码流的一部分");我们是 `px-[10px]` 的 flex 行(`DiffView.tsx:808-819`),
  与行号列/代码列都不对齐(读作外挂按钮)。动作:band 改 grid,首列复用 `.dl` 沟槽宽 `calc(5ch + 27px)`。
- ☐ **DF-6(P2)toolbar 摘要吞掉为零的一半**:金标是 `+649 -57` 并列;我们 `totalDel > 0 &&`
  (`DiffView.tsx:407-411`)只出 `+1406`,而**逐文件头**(`:651-654`)两个数字都渲染 → 同面板两套口径。
  动作:去掉 `> 0` 守卫。

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
- ☐ **SC-12(P1)Scheduled 行没有任何操作,hub 是只读的**:hover 只有底色,右键无菜单,行的唯一行为是
  `select()`。而 `Sidebar.tsx` 早已有整套行 `ContextMenu`(rename/pin/archive/mark unread/open-in),
  `RunView.tsx:135` 有 Stop —— 唯独这个专门管长期任务的屏,不能停、不能重跑、不能改名。
  动作:`onContextMenu` + hover `⋯` 复用 `ContextMenu.tsx`,接已有 store actions。
- ☐ **SC-13(P2)行标题是原始 prompt 段落,不是任务名**:`Scheduled.tsx:171/195` `title: run.label || run.id`
  = 整条提交的 prompt。live 三行里**两行都以同样的字开头**("Append one line with the current…"),
  列表无法扫读。Codex 是 2–4 词短名词("Weekly status update draft")。RS-1 的一行 clamp 只治了症状。
  动作:派生显示标题(截首句/去尾括号/~48 字上限,全文进 `title=`),或优先用 rename 名。
- ☐ **SC-14(P3)搜索命中屏幕上不存在的字段**:`Scheduled.tsx:158` 的 `meta` 含 `project`,但 SC-4 已把
  project 从副行移除(✂)。实测搜 `scratch` 返回一行、而 `scratch` 在该行**任何可见文字里都没有**。
  动作:命中 project 时补一枚安静的尾部 chip,或把 project 移出 `meta`。

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
- ☐ **TH-10(P1)助手消息收尾行静息态只剩孤零零一个时间戳,3 个功能入口不可见**:`styles.conv.css:753-765`
  把 `.msg-copy` 静息压成 `width:0/opacity:0`,只有 `.msg:hover` 才展开 —— Copy message / Copy link /
  **Continue in new task** 三个入口对不扫鼠标的用户**不存在**(触摸设备彻底没有)。金标两条不同消息的收尾行
  都是 `⧉ 👍 👎 ↗ + 时间戳/verdict`,**操作图标静息就在**。`styles.conv.css:737-739` 的注释把金标 verdict
  那枚 ✓ 圆圈误读成"整行只有一个图标"据此把 4 枚图标全藏了 —— **是误读金标,不是用户裁决**。
  轮24 已派 implementer。
- ☐ **TH-11(P1)终态 chrome 138px + 左边线三级台阶 282/320/350**:`.terminal-alert`(`styles.panel.css:397`
  `margin:0 18px`)x=282 w=796 h=85、`.gbar`(`styles.css:4330` `max-width:720px`)x=320 —— 而正文 `.msg-col`
  与 composer `.cx-card` 都是 x=350 w=660(TH-2 立起来的共享竖直边线)。两条横幅一左一右各捅出 68/30px,
  且固定占 138px(阅读区 −31%)。动作:`.terminal-alert` 改 `margin:0 auto; max-width:660px` + 压成单行;
  `.gbar` 720 → 660。**不推翻 QA-45「诚实异常终止 banner 必须存在」的决策,只对齐 + 压扁。**
- ☐ **TH-12(P1)同一个事实一屏说 3–5 遍**:goal 取消说 3 遍(in-thread chip `timeline.ts:829` + `.gbar` +
  Supervision Goal 组)、step limit 说 2 遍(红 chip `timeline.ts:856` + `.terminal-alert`),外加一枚装着
  整句 goal 的 494px pill(`timeline.ts:816` fallback)。合计 5 处 ~300px 讲两件事。Codex 一个终态只说一次。
  动作:`SessionView` 已渲染 `.gbar`/`.terminal-alert` 时抑制重复 chip(`timeline.ts:799` 对 `goal attached`
  已有同类 `noted` 抑制逻辑,推广到 paused/cancelled/limit);fallback chip 的 goal 文本截断到 ~32 字。
- ☐ **SB-6(P1)Projects 树是倒的:子任务比父项目名还靠左 23px,父子字体完全同款**:`.project-heading span`
  x=**57**,而它自己的 `.project-task-title` x=**34**;两者 size/weight/color 三项全同(14.5px/400/`rgb(96,96,96)`)。
  根因:caret(11px)+ folder(16px)+ gap 都在文档流里,把组名推到了自己孩子的右边(`Sidebar.tsx:388-394`)。
  金标里 folder 图标吊在左 gutter,**项目名与其下嵌套任务标题起始 x 完全对齐**。动作:caret+folder 绝对定位
  进 34px gutter,heading 文字左边界拉到 34px;再给 heading 一档不同字重/墨色,让组名成为锚而非同辈。
- ☐ **SB-7(P2)rail 把 `appr`(能动手)和 `stranded`(已坏掉)涂成同一个琥珀 `rgb(138,90,0)`**,不可区分。
- ☐ **SC-15(P3)Scheduled 选中 tab 没有 pill 底色**:`styles.scheduled.css:222` RS-3 刻意去掉了 border+fill+
  shadow 三件套,但金标裁剪图(`codex-crop-scheduled-list.jpg`)显示 Codex 的选中 tab **确实有**一层浅灰 pill
  (只是没有边框和阴影)。动作:只补 fill,不补 border/shadow —— RS-3 的实质(不做 iOS segmented control)保留。

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
