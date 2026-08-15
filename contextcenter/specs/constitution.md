# Context Center — Constitution（产品不变量）

本文是 Context Center 的第一原则清单。所有 spec、plan、实现与后续讨论都
受它约束：与本文冲突的设计默认是错的，除非先修宪。

修宪流程：任何条款的修改/删除/新增，必须在本文末尾"修宪记录"追加一条
（日期、动机、影响面），并同步修订受影响的 spec/plan。不允许"代码先绕
过去，文档以后再说"。

---

## C1. 文件即真相（Files are the source of truth）

一个 Project 的全部数据以 Markdown 文件 + YAML frontmatter 落盘在一个普
通目录树里。Context Center 应用只是这棵文件树之上的**视图与索引**；任何
数据库/缓存必须可以从文件完整重建。

**检验**：把 Project 目录直接交给一个从未见过 Context Center 的人或
Coding Agent，不借助任何 API，就能读懂项目结构、任务状态与历史，并能用
纯文件编辑正确地追加结果。

## C2. 一切皆文档（Document-first）

产品里没有"模块"，只有文档。Bug、Workstream、Plan、Backlog 等都不是预
设类型，而是用户随手创建的普通文档；任何文档可在任何位置创建、任意嵌套。
所谓"特殊文档" = 普通文档 + 一点可选的 frontmatter metadata（如 Project
绑定 workspace 路径与 agent）。Icon 只是按名字/kind 预设的默认装饰。

## C3. 永远可编辑（Always editable）

文档画布默认且始终处于可编辑态。不存在 Edit 按钮、不存在编辑模式切换。

## C4. 选择即上下文（Selection is context）

页面上任何东西——几个词、一段话、一个 Block、一个 Task 行、一个 Widget
——都可以被选中，成为 Composer 里的引用，连同文档路径与周边上下文发给
Agent。这是产品最核心的交互，适用于一切内容，不是某个对象类型的专属功能。

## C5. 执行外包（Execution is delegated）

Context Center 不是 Coding Agent。复杂研究、代码实现、长期执行一律委派
给外部 Coding Agent（Codex、Claude Code 等）；产品内置模型只做局部、简
单、可控的内容操作（改写一段、总结、提取 Task）。

## C6. 结果回流（Results flow back）

闭环是产品存在的理由：Session 产生经验 → 沉淀为 Project Memory →
从 Context 生成 Task → Task + 合适 Context 委派给 Agent → 结果与
Lesson 写回文档树。失败的 Attempt 不是废品，是 Lesson 的原料。

## C7. 渐进结构（Progressive structure）

轻的东西必须保持轻。结构只在需要状态、查询或 Action 时局部出现：

- **呈现层**渐进：行内 to-do → 展开详情 → 打开整页。
- **模型层**统一：每个 Task 的身份是一个文档；但**惰性物化**——一行
  to-do 在第一次需要身份（点开、委派、加 metadata、对象级评论）之前，
  只是父文档里的一行 Markdown，不生成文件。
- Agent 侧渐进披露：从 Project 根文档开始，沿相对链接按需读取子文档。

## C8. 少而稳的 Widget + 可读降级

Widget 只保留少数几种（Task/Task List、Attempt、Loop 及必要的轻量
Action）。每种 Widget 在原始 Markdown 里必须有人类可读的降级表示；
普通用户永远不需要直接面对 YAML/JSON。

## C9. 负空间（刻意不做）

以下能力被明确排除，出现在设计里即违宪：

- 用户、团队、权限、Share 体系；
- 代码 Diff 管理与 Code Review；
- Session 内部执行浏览器（完整 Transcript、Tool Calls、Run Timeline）；
- 通用 Workflow Builder / DAG；
- 复杂 Dashboard；
- 强制完整 Task Schema、一切内容 Widget 化。

---

## 修宪记录

- 2026-08-15：初版（C1–C9）。来源：ChatGPT 设计 session 的 16 轮决策
  （见 `../reference/product-record.html`）+ 本仓库 session 追加的两项
  决策：任务即文档（task-as-document）、文件即真相升格为第一不变量。
