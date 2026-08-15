# Context Center — 技术方案（Plan）

对应 `spec.md` v0.7。三部分：交付分期、文件格式约定 v0.1（C1 的具体
化）、UI 设计基准与样例数据。

## 1. 交付分期

### Stage 0 — 高保真 HTML mock（已交付，`../mock/index.html`）

纯 HTML/CSS/原生 JS 单文件、零外部依赖。目的：钉死视觉与布局基线，让
后续所有 UI 讨论有共同参照。覆盖四个关键屏（对齐 `../reference/` 四张
参考图）+ 树展开折叠、文档切换、Task/Attempt/Bug/Loop → Inspector、真
实文本选区 → 浮动入口 → Composer 引用 Chip 等演示级交互。

### Stage 1 — 可交互 Web 原型

- 栈：React + Vite + TypeScript + Tailwind，纯前端。
- **数据层直接采用 §3 文件格式**：样例数据就是一棵真实的 `.md` 文件树
  （构建时打包/fetch 进浏览器），前端解析 frontmatter + Markdown 渲染；
  编辑写回内存文件树，可整树导出。从第一天起 C1 可验证——把样例目录丢
  给真实 Codex/Claude Code 读写（验收标准 14）。
- Agent 响应用 mock adapter 模拟（脚本化回复/提议更新/Attempt 产出），
  接口形状按 Stage 2 真适配器设计。
- 范围：`tasks.md` 的 F0–F11。

### Stage 2 — 真落盘 + 真 Agent

- 轻量本地后端（形态待定：本地服务或桌面壳）把文件树落到真实文件系
  统；文件监听 + 索引重建。
- Agent adapter：Codex CLI / Claude Code（headless/SDK）双向：出——组
  装 Context 发起 session、收结果；入——项目目录内置 AGENTS.md/skill
  说明文件约定，外部 agent 直接读写目录。
- Open questions（spec §10）在本阶段前收口。

## 2. 原型架构要点（Stage 1）

- `core/`：文件树模型（path → {frontmatter, body}）、Markdown/frontmatter
  解析与序列化、链接解析（相对路径 + id 兜底）、物化/镜像行改写、索引
  （id → path、task 列表、反向链接）。**这一层不依赖 React**，未来直接
  移植 Stage 2 后端。
- `ui/`：树、画布（contenteditable 起步，不引重编辑器框架）、Widget 渲
  染、Inspector、Composer、选区管理。
- `agents/`：`AgentAdapter` 接口 + `MockAgent` 实现（延迟、流式假输
  出、提议更新、Attempt 写回）。
- 所有变更走 `core` 的单一写入口（改文件树 → 派生索引 → UI 响应），保
  证"应用只是文件之上的视图"不走样。

## 3. 文件格式约定 v0.1

### 3.1 目录结构

一个 Project = 一个目录；文档 = 单文件，或（拥有子文档时）目录 +
`index.md`。文件→目录的升级由应用自动完成（与惰性物化同思路）。

```
aurora-ide/                          # Project 根目录
  project.md                         # 根文档（kind: project）
  overview.md
  workstreams/
    index.md                         # "Workstreams" 文档本体
    editor-performance.md
    session-recovery/
      index.md                       # 正文含任务镜像行（见 3.4）
      tasks/
        implement-automatic-recovery.md    # 行级 Task 文档（物化产物）
      implementation-plan.md
      research-notes.md
      recovery-ux.md
  bugs.md
  notes.md
  plans.md
```

约定（非强制）：物化的行级 Task 文档放在父文档同级的 `tasks/` 下；文件
名 = 标题 slug。行级 Task 文档不进左侧树（spec §4.1）。

### 3.2 通用 frontmatter

```yaml
---
id: t-43            # 稳定 id，project 内唯一；创建时按 kind 前缀递增
                    # d-/t-/bug-/loop-/les-；引用兜底锚
kind: task          # 可选；无 kind = 普通文档
icon: "🐛"          # 可选，覆盖 kind/名字预设
created: 2025-05-12 # 可选，应用维护
updated: 2025-05-12
---
```

### 3.3 各 kind 的附加字段（全部可选，能省则省）

```yaml
# kind: project（project.md）
name: Aurora IDE
workspace: ~/dev/aurora-ide          # 代码仓路径
agents: [codex, claude-code]
default_agent: codex

# kind: task（bug 同，外加 priority / found_in / environment）
status: in_progress                  # todo | in_progress | done | blocked
depends_on: [t-42]
attempts:                            # 追加式列表，一次委派一条
  - session: codex:s-091             # 外部 session 引用
    agent: codex
    at: 2025-05-11
    outcome: partial                 # failed | partial | promising | success
    summary: Restores small files; large files exceed timeout.
  - session: codex:s-097
    agent: codex
    at: 2025-05-12
    outcome: promising
    summary: Handles large files via streaming; minor gaps remain.
    best: true                       # 当前采用结果，至多一条
lessons: [les-7]                     # 或相对路径链接

# kind: loop
loop:
  status: running                    # idle | running | paused | done
  current: t-43                      # 队列位置
  strategy: sequential
  on_failure: pause
  save_lessons: true
# 队列本体 = 正文里的有序 Task 链接列表（见 3.4），不在 frontmatter 重复

# kind: lesson
sources: [codex:s-091, codex:s-097]  # 来源 session/attempt 引用
```

### 3.4 正文中的结构降级表示（C8）

- **未物化 to-do**：`- [ ] 修掉 flaky test`（纯 Markdown，无 id 无文件）。
- **物化 Task 镜像行**：`- [ ] [Implement automatic recovery](tasks/implement-automatic-recovery.md)`
  ——checkbox 状态由应用与 Task 文档 frontmatter 同步（文档为准）。
- **子文档链接**：`[Implementation Plan](implementation-plan.md)`，一行
  一个。
- **Loop 队列**：Loop 文档正文中的有序列表，每项一个 Task 链接；重排 =
  挪行。
- **Lesson 引用**：普通链接，或带 💡 前缀的引用块。

### 3.5 链接与移动规则

- 正文引用一律相对路径链接（对外部读者诚实）；frontmatter `id` 是兜底
  锚。
- 应用内移动/重命名：应用负责改写全部入链（Obsidian 式）。
- 外部（agent/人）改动后路径失配：靠 id 扫描对账，修不了的标 broken
  link 露出给用户。
- 冲突策略（MVP）：文件即真相，last-write-wins，git 兜底。

### 3.6 外部 Agent 读写契约（Stage 2 落地，格式即契约）

外部 Coding Agent 对 Project 目录可做的事及其"API"：

| 意图 | 文件操作 |
|------|---------|
| 了解项目 | 读 `project.md`，沿相对链接渐进展开 |
| 读任务 | 读 Task 文档 frontmatter + 正文 |
| 更新任务状态 | 改 frontmatter `status`（镜像行由应用对账） |
| 写回执行结果 | 向 `attempts:` 追加一条（不改历史条目） |
| 保存经验 | 新建 `kind: lesson` 文档并在来源处链接 |
| 更新内容 | 直接编辑对应 Markdown Section |
| 新增子文档 | 创建文件 + 在父文档插入一行链接 |

项目目录内置一份 AGENTS.md（模板由产品生成）向 agent 说明以上契约。

## 4. UI 设计基准（Stage 0/1 共用）

以 Notion 为直接参照，细节对齐 `../reference/mock-*.png`：

- **布局**：左树 260px（#f7f7f5，可折叠）；中间画布白底，正文列
  max-width ≈ 720px 居中，页首大标题 + 灰色摘要行；右 Inspector 340px，
  按需出现；顶部面包屑栏 45px。
- **字体**：系统栈（-apple-system, "Segoe UI", "PingFang SC" …）；正文
  15px/1.6，页标题 38–40px/700，节标题 20px/600；树行 13.5px，行高
  28px。
- **颜色**：ink `#37352f`、muted `rgba(55,53,47,.65)`、边线
  `rgba(55,53,47,.09)`、hover `rgba(0,0,0,.04)`、accent（选区/激活/主按
  钮）`#6759dc`；状态 pill：Done 绿 / In progress 蓝紫 / Todo 灰 /
  Failed 红 / Partial 琥珀 / Promising 绿 / P1 红 P2 灰。
- **组件习惯**：hover 才浮现的行内操作（+、⋯）；pill 圆角 4–6px 小号字；
  Widget 用 1px 边框圆角卡片，标题行 + 行列表 + 页脚动作行；Inspector
  为"标签 + 值"两列的松散表单，Action 列表带图标。
- 交互密度、留白、图标风格"几乎照抄 Notion"，禁止 SaaS Dashboard 化
  （阴影卡片堆、大色块统计）。

## 5. 样例数据集（两个 Project，对齐参考图）

- **Aurora IDE**：Overview；Workstreams{Editor Performance、AI
  Assistant、Session Recovery}；Bugs；Notes；Plans。Session Recovery
  含 Goal/Current Understanding/Linked docs/Tasks(#42 done、#43 in
  progress、#44 todo)/Open Questions，t-43 带 3 次 Attempt
  （failed→partial→promising+best）与 1 条 Lesson；子文档
  Implementation Plan 含 Milestones + "Recovery MVP Loop"（4 项，
  Run sequentially / Pause on failure / Save lessons）。
- **Atlas Deploy**：Overview；Workstreams{Deployment Pipeline、
  Infrastructure}；Bugs{Bug Backlog}；Notes；Plans。Bug Backlog 含
  Active(5, BUG-142 P1 in progress 3 attempts)/Resolved(3)/Codex Bug
  Fix Loop(running, 1 of 5)/Learnings(2 lessons)。
