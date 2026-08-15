# Context Center — Constitution（产品不变量）

本文是 Context Center 的第一原则清单。所有 spec、plan、实现与后续讨论都
受它约束：与本文冲突的设计默认是错的，除非先修宪。

修宪流程：任何条款的修改/删除/新增，必须在本文末尾"修宪记录"追加一条
（日期、动机、影响面），并同步修订受影响的 spec/plan。不允许"代码先绕
过去，文档以后再说"。

---

## C1. 文件即真相（Files are the source of truth）

**项目知识**——正文、结构、任务、结果、经验及其元数据——以 Markdown +
YAML frontmatter 落盘在普通目录树里，文件是它唯一的真相。

应用另配一个 **SQLite 伴随库**，存放不适合文件的辅助信息：对话线程、子
文档顺序、节点级应用状态、运行时状态、派生索引。取舍规则：**能放文件的
放文件；放不进文件的进 SQLite**。项目知识永远不允许只活在 SQLite 里——
对话中需要沉淀的结论必须回流成文档（C6）。

**检验**：把 Project 目录（不带 SQLite）交给一个从未见过 Context
Center 的人或 Coding Agent，不借助任何 API，就能读懂项目结构、任务状态
与历史，并能用纯文件编辑正确地追加结果；删掉 SQLite 不损失任何项目知
识，索引类内容可从文件重建。

## C2. 一切皆文档（Document-first）

产品里没有"模块"，也没有**目录**：树里的每个节点都是文档（文件）。
Bug、Workstream、Plan、Backlog 等都不是独立对象，而是用户随手创建的普
通文档；任何文档可在任何位置创建、任意嵌套——嵌套在磁盘上如何落地只是
存储细节，目录永远不是产品对象，不会作为节点出现在树或正文里。

所谓"特殊文档" = 普通文档 + 可选 frontmatter：`type`、`tags`，以及某些
type 的附加字段（如 project 绑定 workspace 路径与 agent）。`type` 命中
预定义集合（project / workstream / task / bug / loop / lesson / plan /
backlog / research …）时换预设 icon、默认字段与可用 Action；未知或缺省
的 type 不改变任何行为。Icon 解析：手动设置 > type 预设 > 默认页面 icon。

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
- 2026-08-15 (2)：C2 增补（用户裁决）：明确**没有目录**——一切皆文件，
  目录仅为嵌套的落盘形式；icon 改由可选 frontmatter `type`（预定义集
  合）驱动，辅以 `tags`，手动可覆盖；删除"按名字猜 icon"。
- 2026-08-15 (3)：C1 增补（用户裁决）：引入 **SQLite 伴随库**，两储分
  工——能放文件的放文件，放不进文件的（对话线程、子文档顺序、节点级应
  用状态、运行态、索引）进 SQLite；项目知识不许只活在 DB 里，检验条款
  相应加强（不带 DB 仍可读懂续作、删 DB 不失知识）。
