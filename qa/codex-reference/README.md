# Codex 金标截图参照 (golden reference pixels)

这是 **OpenAI Codex 桌面 app**(`/Applications/ChatGPT.app`,bundle
`com.openai.codex`)各屏的**真实截图**,是 AgentRunner webui 对齐的 ground truth。
`/parity-drive` 每轮的第一步**必须**拿我们的 live `127.0.0.1:8809` 对着这些图逐屏
比对、排出差距、关闭最大的差距(详见 `docs/increments/INC-41-DRIVER.md` 的
「🎯 每轮第一步」)。

## ⚠️ 这些图为什么在这里、怎么刷新

- **headless 循环自己截不到 Codex app** —— launchd 里的 `claude -p` 无 GUI 会话、
  无 Computer Use、无录屏权限。所以金标图**不能**由循环自采,必须**带外捕获**后入库。
- 本批由 **Computer Use 交互 session** 于 2026-07-11 捕获(ChatGPT.app,full-height
  窗口,dark-on-light 的 "Codex" light 主题)。
- **刷新方式**:用一个能跑 Computer Use 的交互 session(或真人手动截图),按下面
  的文件名覆盖对应图,`git add qa/codex-reference/*.jpg && commit`。截图**入库**
  (这是参照 ground truth,不是临时测试产物;`qa/runs/` 里的 `*.png` 才是被
  gitignore 的临时件)。Codex app 升级或改版后应重截。

## 屏 → 参照文件

| 参照文件 | Codex 屏 | 我方 live(8809)对应 |
|---|---|---|
| `codex-diff-review.jpg` | **Diff / code review 分栏(最重要)**:左对话流 + 右满高语法高亮 diff(逐文件头、+/- 行数、unmodified 折叠、AgentRunner/Review tab、Commit or push) | Changes split / Review |
| `codex-task-thread.jpg` | 任务 thread / 详情:消息流、`Edited 31 files +980 -317` 变更卡(Undo/Review)、文件行 +/-、`Show N more files`、右侧 Environment 面板、底部 follow-up composer | 富会话 |
| `codex-thread-environment-panel.jpg` | thread + Environment 面板展开(Changes / Worktree / Create branch / Commit or push、Background processes、Browser、Sources) | 富会话右栏 |
| `codex-new-task-home.jpg` | New task / 项目首页空态:居中 "What should we build in agentrunner?" + 4 建议卡 + 底部 composer(repo/worktree/environment/branch chips + 模型选择) | home |
| `codex-scheduled.jpg` | Scheduled 任务:搜索、All/Active/Paused 筛选、任务行(cadence + next-run)、Suggestions | Scheduled |
| `codex-pull-requests.jpg` | Pull requests:All/Reviewing/Authored 筛选、PR 行(repo/branch + +/-) | Pull requests |
| `codex-plugins.jpg` | Plugins:搜索、Installed 行、Featured + 分类网格 + Install 按钮 | Plugins |
| `codex-sites-empty.jpg` | Sites 空态("No sites yet" + Create new site) | Sites |

**全局 chrome**(以上各图皆可见,是首要对齐面):左 sidebar =
New task / Scheduled / Plugins / Sites / Pull requests / Chat,然后 `Pinned`,
然后 `Projects`(按 repo 分组、每 repo 下缩进任务行、Show more/less),底部
`Tasks` 段,再底部账户行。
