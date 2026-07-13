# Codex 桌面 app UI 全面参照（2026-07-10 实测）

**方法**：Computer Use 实操本机 `/Applications/ChatGPT.app`（bundle
`com.openai.codex`，版本 26.709.11516），逐屏浏览 + 放大取证。只读浏览，
未发送消息/未改设置。截图因宿主进程无录屏权限无法落盘，本文以结构化
文字规格代替像素存档（本目录 .gitignore 已排除未来任何截图）。

**用途**：AgentRunner webui 对齐 Codex 的 ground truth。配合
`docs/CODEX-PARITY.md`（功能对照）使用；本文偏 UI 结构/microcopy/交互。

---

## 0. 设计 tokens（Settings → Appearance 实测值）

- Light theme 预设名 "Codex"：Accent `#0169CC`，Background `#FFFFFF`，
  Foreground `#0D0D0D`。
- UI font：`-apple-system, Blink…`（系统栈）14px 基准；Code font：
  `ui-monospace, "SFM…"` 12px 基准。两者字号均可调（px 步进）。
- Theme 三选卡：System / Light / Dark（带迷你窗口预览图）。
- Contrast 滑杆（0-100，当前 45）；Translucent sidebar 开关（开）。
- Diff markers 二选一：Color 或 `+/-`；Reduce motion System/On/Off；
  Font smoothing 开关；Use pointer cursors 开关（默认关——即默认箭头，
  不是手型！）；Dock icon 二选。
- 主内容白底；sidebar 半透明浅灰；卡片/输入区圆角大（~12-16px），
  边框极浅；主按钮黑底白字胶囊（"Upgrade to Pro"/"Save"），次按钮
  白底细边框胶囊（"Review"/"Reset usage"）。
- 状态红用于警示徽标（红色 (!) 圆圈）与危险按钮（Delete/Reinstall 红字）。

## 1. 应用框架

- **左 sidebar**（~135px 窄档实测；半透明）：
  - 头部：`ChatGPT Codex`（Codex 字样淡紫/灰）+ 搜索图标（打开命令面板）。
  - 导航：New task（⌘N 徽标显示在行尾）/ Scheduled / Plugins / Sites /
    Pull requests / Chat。未读的导航项右侧有**蓝点**。
  - `Pinned` 分组：置顶任务扁平列表。
  - `Projects` 分组：项目（📁 名称）→ 其下任务缩进列表；每项目最多
    ~5 条 + `Show more`；无任务项目显示灰字 `No tasks`。
  - 任务行 hover：右端浮出 **pin/unpin + archive** 两图标；已 pin 行
    是 unpin 图标。行内状态徽标：红 (!) 圆=需要注意（usage limited 等），
    ⇗ 小图标=有 worktree/运行中标记。
  - 任务行 hover ~1s 出**预览卡**：标题全文 + 相对时间（右上），
    📁 project 行，⑂ worktree/branch 行。
  - 底部账户行：圆形头像（姓名缩写）+ 名字 + `?` 帮助按钮（无齿轮；
    Settings 走 ⌘, 或应用菜单）。
- **主内容列**：单列 thread/页面，内容最大宽度居中（~640-720px 视觉）。
- **右上窗口图标**（thread 视图）：⤢ 弹出/全屏、▭ toggle bottom panel、
  ◫ toggle side panel（tooltip 带快捷键）。
- **右侧上下文面板**（thread 视图默认显示，可折叠）：见 §4。
- **Review 分栏**（点 Edited files 卡的 Review 后）：右半屏变 tab 化
  面板：tab 条 `(75) AgentRunner`（内嵌浏览器 tab，带 favicon）+
  `Review` tab + `+`。见 §5。
- **底部终端抽屉**（⌘J）：tab 化终端（📁 项目名 tab + `+` + 右端 ×），
  Settings 可选终端出现在 Bottom 或 Right。
- **浮动 Chat 面板**（sidebar Chat 或 hover 子菜单 Recent chats）：
  右下角浮窗 "New chat"，头部 Recent chats + 弹出 + 最小化；空态文案
  "Ask quick questions with Chat — Use the classic Chat experience to get
  answers and explore ideas. Start a new chat or pick up a past ChatGPT
  conversation here." + View chat history；底部小 composer
  （"Message ChatGPT"，模型档显示 "Instant"）。
- **命令面板**（sidebar 搜索图标/⌘K）：居中 modal，"Search tasks or run
  a command"；分组 Tasks / Unread tasks；行=标题 + 右侧项目名灰字 +
  ⌘1..9 快捷键徽标。

## 2. New task 视图（首页）

- 居中笑脸图标 + 大标题 **"What should we build in {project}?"**
  （项目感知；无项目时想必变体）。
- composer 上方可叠**通知条**（圆角卡片）：如 "You have a new rate
  limit reset available … [See resets] [×]"、usage 警示条（见 §7）。
- **Composer**（大圆角卡、双行布局）：
  - 上缘环境条（浅灰内嵌条，含四个独立 chip）：
    `📁 agentrunner` `⑂ New worktree` `◎ No environment` `⑂ main`。
  - 输入区 placeholder：**"Do anything"**（新任务）/
    **"Ask for follow-up changes"**（thread 内跟进）。
  - 底行：`+` 附件菜单 · `⚠ Full access`（权限模式，橙色字）·
    （thread 内多一个 `Goal` chip）· 右侧：模型 pill
    `5.6 Sol Extra High ▾` · 🎤 · 圆形黑底 ↑ send。
- **四个环境 chip 的 popover**（全部有界高度、可键盘、Escape 关闭）：
  1. Project：搜索框 "Search projects" + 项目列表（selected ✓）+
     `+ New project ▸` + `× Don't work in a project`。
  2. 运行位置：标题 "Start in"，选项 `Work locally` / `New worktree ✓` /
     `Cloud`；分隔线下 `Usage remaining 0%`。
  3. Environment：标题 "Local environment"，`No environment ✓`、
     空态 "No environments found"、`⧉ Create local environment`。
  4. Branch：搜索框 "Search branches" + `Branches` 分组列表（main ✓ +
     其他分支，长名截断）。
- **`+` 菜单**：分组 `Add`：Files and folders / Attach Finder(?) /
  `Goal — Set a goal to keep pursuing` / `Plan mode — Turn plan mode on`；
  分组 `Plugins`：Documents / PDF / Spreadsheets / Presentations /
  Template Creator（各带一句描述）。
- **权限模式菜单**：标题 "How should ChatGPT actions be approved?" +
  Learn more；选项（图标+标题+描述）：
  - Ask for approval — Always ask to edit external files and use the internet
  - Approve for me — Only ask for actions detected as potentially unsafe
  - Full access — Unrestricted access to the internet and any file on your
    computer ✓
  - Custom (config.toml) — Uses permissions defined in config.toml
- **模型选择器**：pill 点开 = **Effort 滑杆**（6 档圆点轨道，蓝色填充 +
  白色圆钮）+ 头部 `Advanced ▸` 与 ⚡ 图标；Advanced 展开三行：
  `Model | 5.6 Sol ▸`、`Effort | Extra High ▸`、`Speed | Standard ▸`。
  Settings→Configuration 有 "Available reasoning efforts (5 selected)"
  控制滑杆档位可见性。

## 3. Thread（任务会话）视图

- **头部**：📄 图标 + 标题 + `…` 菜单：Pin task / Rename task /
  Archive task ┃ Open side task / Copy ▸ / Continue in… ▸ /
  Add scheduled task… / Open in new window。
- **消息流**（单列，用户消息右对齐浅灰气泡、assistant 左对齐无气泡）：
  - 用户气泡下可挂注记行：`⚡ Sent as goal`（该消息作为 goal 发送）。
  - 图片附件：气泡下缩略图（多图为一行栅格）；点开 = lightbox（全屏暗
    背景 + 底部 `− 100% +` 缩放条 + 右上 下载/×）。
  - **Worked for N 折叠**（每个 turn 的工作段）：`Worked for 33m 35s ⌄`
    灰字行，展开后 = **活动行 + 叙述文本交错**：
    - 活动行样式：小图标 + 聚合标签，如 `Viewed an image`、
      `Ran commands`、`Read files`、`Edited files, read files, ran
      commands`、`Used the browser, ran a command`、
      `Context automatically compacted`（compaction 也是一条活动行！）。
    - 活动行可再点开：逐条命令行（终端小图标 + `Ran {cmd 截断}`）；
      单条再点开 = **Shell 块**：灰底圆角，左上角 "Shell" 小标，
      `$ 命令`（等宽、可换行）+ 空行 + 输出，右下角 `✓ Success` 状态。
      部分行是自然语言小结（如浏览器操作组："刷新真实共享数据"）。
  - **最终回答**：正文 markdown（列表、内联 code chip 灰底圆角）。
  - **文件产物 chips**：卡片行（图标 + 文件名 + "Document · MD"）+
    右端 `Open in ▾` 菜单：VS Code / Cursor / Default app / Terminal /
    iTerm2 / Warp / Xcode ┃ Show in Finder / Download a copy；
    多文件折叠 "Show 4 more ⌄"。
  - **Edited files 卡**：头部 `⊞ Edited N files` + 绿红 `+606 -121`，
    右侧 `Undo ↺` 与 `Review`（描边胶囊）；展开列表每行
    `docs/DESIGN.md ... +9 -2`（路径灰、文件名黑、数字右对齐绿红），
    折叠 "Show 15 more files ⌄"。Review 打开后卡片副题变
    `Review changes ↗`。
  - **消息 hover 操作行**（消息组末尾浮现）：copy / 👍 / 👎 /
    分叉(share?) 图标 + 时间戳 `2:53 PM`。
  - **turn 间态**：`Thinking` 灰字（流式时）；错误/系统横幅内联：
    `⊙ You've hit your usage limit. Upgrade your plan or add credits to
    continue, or try again at 5:59 PM.`（两侧细分隔线）。
  - **滚到底浮钮**：内容不在底部时中下方浮现圆形 ↓。
- **Goal banner**（composer 上方常驻，浅色圆角条）：
  `◎ Goal {status} {goal 文本} · {elapsed}`，右端三个小图标：
  ✎ Edit goal / ▶(resume?) / 🗑 删除。status 实测 "usage limited"
  （黑体）+ goal 文本灰字。
- **Usage 警示卡**（撞限时 composer 上方）：
  `⊙ You're out of Codex and Work usage — Your rate limit resets on
  5:59 PM. Upgrade or use one of your rate limit resets now.` +
  黑底 `Upgrade to Pro` + 描边 `Reset usage`。

## 4. 右侧上下文面板（thread 视图）

分区小标题灰字 + 行条目：
- **Environment** `+`：`⊞ Changes  +0 -0`（绿红计数）/ `⑂ Worktree ⌄`
  （可展开）/ `⑂ Create branch` / `⊙ Commit or push`（灰=不可用）。
- **Background processes**：`▣ /tmp/arwebui-audit-latest -addr…`
  （运行中的后台进程行）。
- **Browser**：`🤖 (75) AgentRunner  127.0.0.1:8788`（内嵌浏览器 tab +
  地址）。
- **Sources** `+`：`codex-clipboard-{uuid}` 截图/剪贴板来源列表 +
  `View all`。

## 5. Review diff 面板

- 右半屏 tab 化：`(75) AgentRunner`（browser tab）| `Review` | `+`。
- 工具栏：**范围下拉** `Last Turn ⌄`（选项：Unstaged / Staged /
  Commit ▸ / Branch / Last Turn ✓）+ `+607 -121` 总计 + `…` +
  折叠图标 + 文件搜索图标 + 分屏/inline 切换图标 + 文件树图标 +
  `Commit or push ⌄`（灰胶囊，不可用时禁用）。
- 文件段头：`M↓ docs/archive/increments/INC-40-….md +70 -0`
  （目录灰、文件名黑）。
- diff 主体：行号列 + 绿底新增/红底删除，markdown 语法高亮保留；
  hover 行出现 `+`（inline annotation/建议工具——assistant 曾提
  "suggest and make updates with the annotation tool"）。

## 6. 各枢纽页

- **Scheduled tasks**：标题 + 副题 "Ask ChatGPT to schedule tasks, set
  reminders, or monitor for updates"；搜索框；过滤 tab `All Active
  Paused` + 右侧 `✓ Mark all as read`；任务行（名称 + "Saturdays at
  4:00 AM · Next run in 13 hours"，未读蓝点，暂停项灰显）；
  **Suggestions** 分组：Daily brief / Weekly review / Follow-up monitor
  模板卡（图标 + 名 + 时刻 + 一句描述）；右上 `Create ⌄`。
- **Plugins**：顶部子 tab `Plugins | Skills`；标题 + "Work with ChatGPT
  across your favorite tools"；搜索；**Installed** 图标行 + ⚙；
  `Public | Personal` 切换 + 筛选；分区 Featured / Productivity /
  Creativity / Developer Tools…，卡片 = 图标 + 名 + 一句描述 +
  `Install`/`…`；"See …, and N more" 展开行。
- **Skills**：同布局；Installed 卡片（Image Gen / OpenAI Docs / Plugin
  Creator / Skill Creator / Skill Installer + 用户自装 Real Env Risk QA、
  UI|UX Product Review）；子 tab `System | Personal | Recommended`。
- **Sites**："Turn your ideas into live websites"；空态：图标 +
  "No sites yet" + `Create new site` 按钮；右上 Create。
- **Pull requests**："Review and track work across GitHub as ralphite."；
  搜索 + 筛选；tab `All | Reviewing | Authored`；行 = PR 图标 + 标题 +
  repo · branch + 相对时间 + `+21 -23`。
- **Archived tasks**（Settings 内）：搜索 + `All tasks ⌄` + `All
  projects ⌄`；按项目分组（📁 名 + "N task(s)"），行 = 标题 + 归档时间
  + 🗑 + `Unarchive`；右上红 `Delete all`。

## 7. Settings（⌘, 全窗接管）

左栏分组：Personal（General/Profile/Appearance/Voice/Configuration/
Personalization/Pets/Keyboard shortcuts/Usage & billing/Account↗）、
Integrations（Appshots/Plugins/Browser/Computer use）、Coding
（Hooks/Connections/Git/Environments/Worktrees）、Archived（Archived
tasks）；顶部 "← Back to app" + "Search settings…"。

关键页面（与我们相关）：
- **General**：Permissions 三行（Default permissions / Auto-review /
  Full access，各带风险描述 + Learn more + 开关）；Default file open
  destination（VS Code ⌄）；Language（Auto detect）；Show in menu bar；
  Bottom panel 开关；Default terminal location `Bottom | Right`；
  Prevent sleep while running；**Speed**（Standard ⌄，"Choose how
  quickly ChatGPT runs across tasks, subagents, and compaction"）；
  Suggested prompts 开关；Import work from other AI apps；Open source
  licenses。**Composer** 组：Show context window usage / Send shortcut
  （Enter 行为）/ Follow-up behavior（"Queue follow-ups while ChatGPT
  runs or steer the current run. Press ⌘⏎ to do the opposite for one
  message"）。**Popout Window** 组：hotkey + Default to projectless task。
- **Appearance**：见 §0（含 Import / Copy theme / 预设下拉 `Aa Codex`）。
- **Keyboard shortcuts**：全量可重绑（每行动作 + 快捷键 chip + 编辑/
  删除；可多绑定如 Back = ⌘[ + Mouse Back；`Reset all to defaults`）。
  动作含：New task ⌘N、New chat、Archive task、New projectless task、
  Open side task、Open in new window、Toggle pin、Focus browser address
  bar ⌘L、Back/Forward、Next/Previous recently viewed task ⌃Tab、
  Next/Previous tab、Next/Previous task、Open browser tab ⌘T、Open
  review tab、Toggle bottom panel ⌘J…
- **Usage & billing**：Your plan（Pro）+ View plans；Credits balance
  （$0 + Buy credits + Set up auto-reload）；**General usage limits**
  （5-hour：进度条 + "Resets 5:59 PM" + "0% left"；Weekly：82% left +
  "Resets Jul 17"）；**GPT-5.3-Codex-Spark usage limits**（同构双行）；
  **Usage limit resets**（"Full reset (Weekly + 5 hr) — Expires 7/26"
  + 黑底 `Use reset` 按钮 ×2）；Cancel plan。
- **Git**：Branch prefix（`codex/`）；PR merge method `Merge | Squash`；
  Always force push（--force-with-lease）；Create draft PRs 开关；
  Review delivery `Inline | Detached`；Commit instructions 大文本框 +
  Save；Pull request instructions 同。
- **Worktrees**：Worktree root（Default）；Automatically delete old
  worktrees 开关 + 说明 "ChatGPT snapshots worktrees before deleting,
  so pruned worktrees should always be restorable"；Auto-delete limit
  （15）；下面按 repo 分组列 worktree 卡（路径 + Conversations 关联
  任务名 / "No conversations linked" + 红字 Delete）。
- **Configuration**：Custom config.toml settings（scope 下拉 `User
  config ⌄` + `Open config.toml ↗`）；Approval policy（Never ⌄）；
  Sandbox settings（Full access ⌄）；Model features：Available
  reasoning efforts（5 selected ⌄）；Workspace Dependencies：Codex
  dependencies 开关（bundled Node.js/Python）、Diagnose、Reset and
  install Workspace（红）+ "Current version: 26.709.11516"。
- **Hooks**：空态 "No hooks found — Configured hooks will appear here"
  + "Manage lifecycle hooks from config and enabled plugins" + 刷新。

## 8. 与 AgentRunner webui 的 UI 差距速记（详见 PIPELINE.md）

我们已齐平（INC-19..40 已做）：Codex 式框架/sidebar/项目分组/New task
环境条四控件/审批卡/Changes→Review/Worked for 折叠/task hover
pin+archive+预览/命令面板/深浅主题/responsive。

本轮观察到的可对齐余项（候选，待 PIPELINE 细化）：
1. Worked 段展开的**活动行聚合**形态（图标 + "Ran commands/Read files"
   分组 → 二级命令行 → 三级 Shell 块带 ✓ Success 尾注）。
2. **消息 hover 操作**（copy/👍👎/时间戳）与用户消息 "Sent as goal"
   注记。
3. **Review 面板**：范围下拉（Unstaged/Staged/Commit/Branch/Last
   Turn）、+N -N 工具栏、inline/split 切换、文件段头 M 图标。
4. **右侧上下文面板**：Environment(Changes/Worktree/branch 操作)、
   Background processes、Browser、Sources 四区。
5. **Goal banner** 三图标（edit/resume/delete）+ elapsed 展示形态；
   usage 警示卡形态。
6. **文件产物 chips**（Open in ▾ 菜单）。
7. **Scheduled** 页的 Suggestions 模板卡 + Mark all as read + 未读蓝点。
8. **命令面板**的 ⌘1..9 + Unread 分组。
9. **图片 lightbox**（缩放条 + 下载）。
10. **Settings 信息架构**（分组左栏 + Appearance tokens/字号/对比度、
    Keyboard shortcuts 可重绑清单、usage 进度条页）。
11. **底部终端抽屉**（⌘J、tab 化）——对应我们 bash bg 工具的 UI 面。
12. 滚到底浮钮、Thinking 行、compaction 活动行等微部件。

Codex 有而我们**明确不追**（DESIGN/PARITY 已裁）：Cloud 运行位置、
Sites、Chat 浮窗（ChatGPT 生态）、Plugins marketplace、语音、Pets。
