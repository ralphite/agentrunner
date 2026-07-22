# Codex 金标截图参照 (golden reference pixels)

**OpenAI Codex 桌面 app**(`/Applications/ChatGPT.app`,bundle `com.openai.codex`)
各屏 + 组件的**真实截图**,是 AgentRunner webui 对齐的 ground truth。`/parity-drive`
每轮第一步**必须**拿 live `127.0.0.1:8809` 对着这些图逐屏/逐组件比对、排差距、
关最大的差距(见 `docs/increments/INC-41-DRIVER.md`「🎯 每轮第一步」)。

## ⚠️ 来源与刷新

- **纯 headless 循环仍截不到 Codex app**（无 GUI / 录屏权限）；但有 Screen Recording
  权限的 macOS 交互 session 可运行 `qa/capture-codex-ui.sh`，脚本按
  `com.openai.codex` PID 找到真实 layer-0 窗口，不会误截桌面或别的 app。
- **交互态**：`qa/capture-codex-ui.sh --command-palette --output <path>.png` 会用
  `Cmd+K` 打开命令面板、截图，再用 `Escape` 恢复；只做可逆 UI 导航，不发送消息。
- **刷新**：交互 session 先把 current / command-palette / 目标 screen 截到
  `qa/runs/<日期>-<QA号>/` 并逐张打开验图；确认无错窗/黑屏/裁切后，再按下方文件名
  覆盖、转成 jpg 并 `git add qa/codex-reference/*.jpg`。金标图入库，`qa/runs/`
  临时证据继续 gitignore。
- 2026-07-11 金标由 Computer Use 交互 session 捕获；2026-07-22 QA-87 已用上述
  系统窗口路径再次验证 current + command-palette 两态。

## 全屏参照

| 文件 | 屏 | 我方 live 对应 |
|---|---|---|
| `codex-diff-review.jpg` | **Diff/review 分栏(最重要)** | Changes split / Review |
| `codex-task-thread.jpg` | 任务 thread / 详情 | 富会话 |
| `codex-thread-environment-panel.jpg` | thread + Environment 面板展开 | 富会话右栏 |
| `codex-thread-alt-scroll.jpg` / `codex-thread-alt-scroll-2.jpg` | thread 其他滚动位 | 富会话 |
| `codex-new-task-home.jpg` | New task / 项目首页空态 | home |
| `codex-scheduled.jpg` | Scheduled 任务 | Scheduled |
| `codex-pull-requests.jpg` | Pull requests | Pull requests |
| `codex-plugins.jpg` | Plugins | Plugins |
| `codex-sites-empty.jpg` | Sites 空态 | Sites |

## 裁剪组件参照(高清,逐组件对齐用——**首选**)

| 文件 | 组件 | 我方 live 对应 |
|---|---|---|
| `codex-crop-sidebar-nav.jpg` | sidebar:logo "ChatGPT Codex"、nav(New task/Scheduled/Plugins/Sites/Pull requests/Chat)、Pinned | 左 sidebar 上半 |
| `codex-crop-sidebar-projects.jpg` | sidebar:Projects 树 — repo 文件夹图标、缩进任务行、深链箭头、Show more | 左 sidebar Projects |
| `codex-crop-newtask-emptystate.jpg` | 空态:图标 + "What should we build in {repo}?" + 4 建议卡(彩色图标) | home 空态 |
| `codex-crop-composer.jpg` | composer:输入上方 repo/worktree/environment/branch chips、"Do anything" 占位、+、Full access 橙徽、模型 "5.6 Sol Extra High"、mic、send | composer |
| `codex-crop-model-dropdown.jpg` | 模型下拉:Model / Effort / Speed 行 + 值 + chevron、Advanced | 模型选择 |
| `codex-crop-add-menu.jpg` | "+" 菜单:Files and folders / Attach Finder / Goal / Plan mode + Plugins 分区(Documents/PDF/Spreadsheets/Presentations/Template Creator) | composer + 菜单 |
| `codex-crop-diff-header.jpg` | diff 头:session tab(robot 图标)、Review tab、布局图标、"Last Turn ▾ +649 -57"、commit/push | Changes split 头 |
| `codex-crop-diff-rendering.jpg` | **diff 渲染**:逐文件头(M↓ path +8 -4)、"N unmodified lines" 折叠 chevron、行号、红/绿加删行 + 斜纹 gutter、语法高亮 | diff 视图 |
| `codex-crop-change-card.jpg` | 变更卡:"Edited 31 files / +980 -317"、Undo ↺、Review、逐文件 +N -N | 变更卡 |
| `codex-crop-artifact-cards.jpg` | artifact 卡:doc 图标、文件名、"Document · MD"、Open in ▾、Show 1 more | artifact 卡 |
| `codex-crop-message-actions.jpg` | 消息动作行(copy/👍/👎/share)+ "✓ Goal achieved in 3h 47m 26s" + follow-up composer | 消息动作行 |
| `codex-crop-command-palette.jpg` | 命令面板:"Search tasks or run a command"、Tasks 列(repo 标签 + ⌘1–⌘9)、Unread tasks(蓝点) | ⌘K 面板 |
| `codex-crop-scheduled-list.jpg` | Scheduled:标题/副标题、搜索、All/Active/Paused pill、任务行(cadence + next run、unread 点) | Scheduled 列表 |
| `codex-crop-scheduled-suggestions.jpg` | Scheduled:Suggestions 行(彩色图标 + 描述) | Scheduled 建议 |

**全局 chrome**(各图皆可见):左 sidebar = New task/Scheduled/Plugins/Sites/
Pull requests/Chat → `Pinned` → `Projects`(按 repo 分组 + 缩进任务行 + Show more/less)
→ `Tasks` → 账户行。
