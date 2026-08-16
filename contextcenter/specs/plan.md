# Context Center — 技术方案（Plan）

对应 `spec.md` v0.7。三部分：交付分期、文件格式约定 v0.1（C1 的具体
化）、UI 设计基准与样例数据。

## 1. 交付分期

### Stage 0 — 高保真 HTML mock（已交付，`../mock/index.html`）

纯 HTML/CSS/原生 JS 单文件、零外部依赖。目的：钉死视觉与布局基线，让
后续所有 UI 讨论有共同参照。覆盖两个 Project 的文档树（无目录节点）、
七个手工页面（workstream/plan/task/loop/backlog/bug/project）+ 生成的
stub 页、右栏 Page Info + Chat 双区、真实文本选区 → 浮动入口 →
blockquote 文字引用进 Chat、Proposed update → Apply 等演示级交互。
（v0.8 重构前的四屏版对齐 `../reference/` 参考图；重构后布局以 v0.8
spec 为准，Journey 覆盖不变。）

### Stage 1 — 可交互 Web 原型

- 栈：React + Vite + TypeScript + Tailwind，纯前端。
- **组件底座（用户裁决 2026-08-16）：优先 shadcn/ui + Radix UI**。
  shadcn 组件源码拷入仓库、按 §4 的 Notion 基准 token 换肤（不保留其
  默认观感）；Radix primitives 提供弹层/菜单/焦点/键盘可达性语义。
  覆盖范围：下拉、context menu、select、popover、dialog、tooltip、
  toast、tabs、sheet（移动端右栏/树抽屉）等应用壳控件。
  **不覆盖**：块编辑器（gaps A1，另行选型）、文档树、画布渲染、选区
  引用——这些自研，行为按 spec。
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
- `store/`：SQLite 伴随库存取层，schema 见 §3.6。Stage 1 用内存实现
  （接口按 schema 设计，可选 sql.js 落 IndexedDB）；Stage 2 换真
  SQLite，接口不变。
- `ui/`：应用壳与控件优先 shadcn/ui + Radix（见 §1 裁决）；树、画布编
  辑器（技术选型待 gaps A1 裁决）、右栏 Metadata/Chat 双区、Widget 渲
  染、选区管理为自研组件。
- `agents/`：`AgentAdapter` 接口 + `MockAgent` 实现（延迟、流式假输
  出、提议更新、Attempt 写回）。
- 所有变更走 `core` 的单一写入口（改文件树 → 派生索引 → UI 响应），保
  证"应用只是文件之上的视图"不走样。

## 3. 文件格式约定 v0.1

### 3.1 落盘结构（没有目录对象）

**每个文档永远是一个 `.md` 文件。** 拥有子文档时，子文档放进与该文件同
名（去扩展名）的同级目录里；这个目录纯粹是嵌套的落盘形式，不是产品对
象，树里永远不出现"目录节点"。规则全树统一，Project 也不例外（根就是
vault 顶层的一个 `type: project` 文件）。目录的创建/清理由应用自动完成
（与惰性物化同思路）。

```
<vault>/
  aurora-ide.md                      # Project 根文档（type: project）
  aurora-ide/                        # ↑ 的子文档目录（仅落盘形式）
    overview.md
    editor-performance.md            # type: workstream
    ai-assistant.md                  # type: workstream
    session-recovery.md              # type: workstream，正文含任务镜像行（见 3.4）
    session-recovery/
      implement-automatic-recovery.md   # type: task（物化产物，不进树）
      implementation-plan.md            # type: plan
      implementation-plan/
        recovery-mvp-loop.md            # type: loop，队列在正文
      research-notes.md
      recovery-ux.md
    bugs.md
    notes.md
    plans.md
  atlas-deploy.md
  atlas-deploy/
    ...
```

物化的行级 Task/Bug 文档就是普通子文档（文件名 = 标题 slug），只是默认
不进左侧树（spec §4.1）；不再有 `tasks/` 之类的专用目录约定。

补充约定（gaps 裁决 2026-08-16）：

- **图片/附件**：粘贴时落盘到文档同级 `assets/`，正文插相对链接。
- **Trash**：软删移入 `<vault>/.contextcenter/trash/`（保留原相对路
  径），Trash 内永久删除才真删；指向已删文档的链接标 broken。
- **slug（待用户确认 ▷）**：文件名 = title 的 slug 化（中文保留原
  文）；rename 即改文件名并改写全部入链；frontmatter 不设 title 字段。

### 3.2 通用 frontmatter

```yaml
---
id: t-43            # 稳定 id，project 内唯一；创建时按 type 前缀递增
                    # d-/t-/bug-/loop-/les-；引用兜底锚
                    # display id（gaps E6 裁决）：task/普通条目 #N（
                    # project 内连续计数）、bug #BUG-N（独立计数）；
                    # 永不复用；跨 project 移动时重新分配并记 moved_from
type: task          # 可选；命中预定义集合才有预设 icon/字段/Action
tags: [reliability] # 可选；自由分类与检索，不影响 icon
icon: "🐛"          # 可选，手动覆盖 type 预设
---
# created/updated 时间戳由 SQLite 伴随库维护（§3.7），不进 frontmatter
```

### 3.3 各 type 的附加字段（全部可选，能省则省）

```yaml
# type: project（vault 顶层的根文档）
name: Aurora IDE
workspace: ~/dev/aurora-ide          # 代码仓路径
agents: [codex, claude-code]
default_agent: codex

# type: task（bug 同，外加 priority / found_in / environment）
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

# type: loop —— frontmatter 只放配置；运行态(status/current/进度/
# session 引用)住 SQLite(§3.7)，结束后的结果写回相关文档
loop:
  strategy: sequential
  on_failure: pause                  # pause | record_lesson_continue
  save_lessons: true
  stop_when: all_tasks_done
# 队列本体 = 正文里的有序 Task 链接列表（见 3.4），不在 frontmatter 重复

# type: lesson
sources: [codex:s-091, codex:s-097]  # 来源 session/attempt 引用
```

### 3.4 正文中的结构降级表示（C8）

- **未物化 to-do**：`- [ ] 修掉 flaky test`（纯 Markdown，无 id 无文件）。
- **物化 Task 镜像行**：`- [ ] [Implement automatic recovery](session-recovery/implement-automatic-recovery.md)`
  ——checkbox 状态由应用与 Task 文档 frontmatter 同步（文档为准）。
- **子文档链接**：`[Implementation Plan](session-recovery/implementation-plan.md)`，
  一行一个。
- **Loop 队列**：Loop 文档正文中的有序列表，每项一个 Task 链接；重排 =
  挪行。
- **Chat 引用**：选区以 Markdown blockquote（`> …`）出现在消息文字里，
  没有专用引用结构。
- **Lesson 引用**：普通链接，或带 💡 前缀的引用块。

### 3.5 链接与移动规则

- 正文引用一律相对路径链接（对外部读者诚实）；frontmatter `id` 是兜底
  锚。
- 应用内移动/重命名：应用负责改写全部入链（Obsidian 式）。
- 外部（agent/人）改动后路径失配：靠 id 扫描对账，修不了的标 broken
  link 露出给用户。
- 冲突策略（MVP）：文件即真相，last-write-wins，git 兜底。

### 3.6 SQLite 伴随库 v0.1

位置：`<vault>/.contextcenter/state.db`（gitignore；删库不失项目知识，
索引可重建——C1）。取舍速查：

| 信息 | 归属 |
|------|------|
| 正文、frontmatter（id/type/tags/status/task 字段/attempts 结果/lessons/loop 配置）、链接/镜像行、loop 队列（正文列表） | **文件** |
| Chat 线程与消息（含文字引用、proposed update 及其 apply 记录） | SQLite |
| 子文档在树中的显示顺序 | SQLite |
| 节点级应用状态（树展开/折叠、最近访问、上次滚动位置） | SQLite |
| 运行态（进行中委派/Loop 的 status/current/进度/外部 session 引用/预估） | SQLite（结束后结果写回文件） |
| created/updated 时间戳、display id 计数器 | SQLite |
| 派生索引（id→path、反向链接、任务清单） | SQLite（可重建 cache） |

表（草案）：

```sql
docs        (id PK, path, created_at, updated_at)            -- 节点信息+时间戳
child_order (parent_id, child_id, rank)                      -- 树内显示顺序
ui_state    (doc_id PK, collapsed, last_visited_at)          -- 节点级应用状态
counters    (scope PK, next)                                 -- display id 分配
threads     (id PK, doc_id, created_at)                      -- 每文档聊天线程
messages    (id PK, thread_id, author, agent, body_md,       -- 引用即 body 里的
             proposal_md, applied_at, created_at)            --   blockquote 文字
runtime_runs(id PK, kind, doc_id, status, current_ref,       -- 进行中委派/Loop
             progress, session_ref, started_at, est)
```

### 3.7 外部 Agent 读写契约（Stage 2 落地，格式即契约）

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

- **布局**：左树 ~280px（#f7f7f5，可折叠，顶部 Context Center
  branding）；中间画布白底，正文列 max-width ≈ 720px 居中，页首大标题
  + 灰色摘要行；右栏 ~360px **常驻双区**——上 Metadata（区头固定标注
  "Metadata"，渲染当前页 frontmatter + 伴随库信息，不重复页面标题），
  下 Chat（线程 + ChatGPT 式输入框：圆角容器内嵌多行输入、附件/Agent
  选择、圆形发送钮）；顶部面包屑栏 45px。没有文档底部 Composer dock。
- **树行为**：不设常驻 chevron 列；hover 时文档 icon 原位变为展开/折叠
  箭头（Notion 式），行尾浮现 `+`/`⋯`。
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
- **组件实现**：弹层/菜单/表单控件一律以 shadcn/ui（Radix）为底、用上
  述 token 定制；自研部分（编辑器/树/画布）与 shadcn 部分共享同一套
  token，肉眼不可区分出处。
- **快捷键（Notion 子集，gaps G3）**：cmd+K Quick Find、cmd+N 新页、
  cmd+\\ 收放侧栏、esc 关面板；编辑器内快捷键随 spec §4.2 输入行为。

## 5. 样例数据集（两个 Project，无目录节点）

- **Aurora IDE**（type: project）：Overview、Editor Performance
  (workstream)、AI Assistant (workstream)、Session Recovery
  (workstream)、Bugs、Notes、Plans 全部直接挂在 project 下。
  Session Recovery 含 Goal/Current Understanding/Linked docs/Tasks
  (#42 done、#43 in progress、#44 todo)/Open Questions；#43 是物化的
  task 文档（不进树），页内含 Scope/Acceptance Criteria/3 次 Attempt
  （failed→partial→promising+best）/Lesson；子文档 Implementation
  Plan (plan) 含 Milestones + 嵌入的 "Recovery MVP Loop"（type: loop
  文档，4 项队列，Run sequentially / Pause on failure / Save lessons）。
- **Atlas Deploy**（type: project）：Overview、Deployment Pipeline
  (workstream)、Infrastructure (workstream)、Bug Backlog (backlog)、
  Notes、Plans。Bug Backlog 含 Active(5, BUG-142 P1 in progress 3
  attempts)/Resolved(3)/Codex Bug Fix Loop(type: loop, running, 1 of
  5)/Learnings(2 lessons)；BUG-142 是物化的 bug 文档（不进树）。
