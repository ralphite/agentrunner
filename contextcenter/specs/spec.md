# Context Center — 产品 Spec v0.8

上承 `../reference/product-record.html` 中的 v0.6 Spec。v0.7 并入两项结
构性决策（任务即文档、文件即真相）；v0.8 并入第二轮用户裁决：没有目录、
type/tags 驱动 icon、右栏改为常驻"Page Info + Chat"双区、选区引用改为纯
文字。与 `constitution.md` 冲突时以 constitution 为准。文末附变更清单。

## 1. 产品定义

Context Center 是一个 Document-first、Agent-aware 的项目工作空间。

用户和 AI 在同一棵可编辑的文档树中维护项目背景、Workstream、计划、
Task、Bug、结果和经验；需要执行代码工作时，把选中的 Task 或内容委派给
Codex、Claude Code 等 Coding Agent；执行结果与 Lesson 写回文档树，成为
下一次委派的 Context。

它不是新的 Coding Agent，不是通用项目管理系统，不是 Workflow Builder。

## 2. 用户与关键 Journey

目标用户：独立开发者 / 小团队中的单人，日常重度使用 Coding Agent，
需要一个比聊天记录更持久、比项目管理软件更轻的"项目记忆 + 委派台"。

四条关键 Journey（验收的主线）：

- **A. 选中内容并让 Agent 更新**：选择一段文字或 Task 行 → Composer 引
  用 → 输入要求 → Agent 回复或对原位置提议局部更新 → 一键 Apply。
- **B. 一个 Task 多次尝试**：委派 → Attempt 1 失败 → 调整/Retry →
  Attempt 2 部分成功 → Attempt 3 最佳 → 选择结果 → 保存 Lesson →
  Task 与文档更新。
- **C. 多 Task 自动执行**：文档内整理 Task 顺序 → 插入 Loop → 启动 →
  逐项委派 → 失败/需澄清暂停 → 状态结果持续写回。
- **D. 持续修复 Bug**：Bug Backlog 文档随手添 Bug → Bug Fix Loop 取下
  一项 → Codex 尝试修复 → 写回 Attempt/结果 → 成功标记完成，失败留
  Lesson。

## 3. 内容模型

### 3.1 Document（唯一的基础实体）

一切皆文档（C2）。文档 = Markdown 正文 + 可选 frontmatter + 可选子文档。

- **没有目录**。树里的每个节点都是文档；"Workstreams" 这类容器节点不存
  在——workstream 就是若干 `type: workstream` 的文档，直接挂在它们的父
  文档下。嵌套（文档含子文档）在磁盘上用目录落地，但目录只是存储形式，
  永远不是产品对象。
- `type`（可选 frontmatter 字段）标记语义：`project` / `workstream` /
  `task` / `bug` / `loop` / `lesson` / `plan` / `backlog` / `research`
  …。命中预定义集合时换预设 icon、默认字段与可用 Action；未知或缺省不
  改变任何行为。
- `tags`（可选 frontmatter 字段）：自由字符串列表，用于补充分类与检索，
  不影响 icon。
- Icon 解析顺序：用户手动设置 > `type` 预设 > 默认页面 icon。
- 子文档在正文中以**一行式链接**呈现，不用大卡片。
- 创建入口只有两个：树/正文行 hover 出现的 `+`（在其下新建子文档）与
  `⋯` context menu；不设独立的全局 create 按钮。

### 3.2 Project（特殊根文档）

左侧树的顶层节点。它是文档，额外绑定结构化数据：项目名称、代码
Workspace/Repository 路径、可用/默认 Coding Agent、少量配置。打开
Project 看到的仍是一篇可自由编辑的根页面。支持多 Project 并存。

### 3.3 Task（任务即文档，惰性物化）

**模型层：每个 Task 的身份是一个文档。** 呈现层保持渐进（C7）：

- **未物化**：就是父文档里的一行 `- [ ] 文字`，无 id、无文件。适合头脑
  风暴式的一次性 checklist，永远不会产生文件噪音。
- **物化触发**（自动、无感）：首次点开详情 / 委派给 Agent / 添加
  metadata / 对 Task 对象发起评论 / 被 Loop 引用。物化后父行变为
  `- [ ] [标题](相对路径)`——checkbox 是可读降级，链接是指针。
- **单一真相**：物化后，状态、metadata、Attempt 历史、Lesson 引用全部
  住在 Task 文档（frontmatter + 正文）里；父行的 checkbox 只是应用维护
  的镜像。在父页勾选 = 应用同时改写两处。
- **Bug 就是 Task**：`kind: bug` 的 Task 文档，多几个可选字段
  （priority、found_in、environment），拿 bug icon。
- Task 文档默认**不进左侧树**（见 4.1），经父页列表、引用与搜索到达。

### 3.4 Attempt

一次委派产生一个 Attempt——外部 Coding Agent Session 的**结果记录**，
存放在所属 Task 文档里。字段：外部 session 引用、agent、时间、outcome
（`failed / partial / promising / success`）、结果摘要、是否当前最佳。
刻意不含：完整 Transcript、Tool Calls、代码 Diff、执行 Timeline（C9）。

一个 Task 可以 Retry 产生多个 Attempt、并排比较摘要、标记一个为当前采
用结果、从失败 Attempt 提取 Lesson。

**多 Task 关联一个 Session**：一次委派可覆盖多个 Task（如整个 Loop 一
个 session）。落法：每个相关 Task 文档各记**自己的** Attempt 条目——引
用同一个外部 session id，但 outcome/摘要按本 Task 记（同一 session 可
能把 A 做完、B 只做一半）。摘要文字允许重复，换取每个 Task 文档自包含、
可单独移植。

### 3.5 Lesson

从一次或多次 Attempt 中提炼的可复用认识。可以是普通文档段落，也可以是
独立的 `kind: lesson` 文档；必须保留来源引用（session/attempt）。后续
Task、Loop、委派可显式引用 Lesson 进入 Context。

### 3.6 Loop

文档内的轻量 Widget，把一组 Task **按顺序**委派给 Coding Agent。

- **队列即文档**：Loop 的执行队列就是 Loop 文档正文里的有序 Task 链接
  列表；重排队列 = 挪动行。被引用的 Task 不必是 Loop 的子文档（Bug
  Fix Loop 引用住在 Backlog 下的 bug）。
- Loop 自身状态（running/paused、当前项、stop condition、retry 策略、
  是否自动存 Lesson）是 Loop 文档自己的 metadata。
- V1 行为：顺序执行；单 Task 可多次 Retry；失败或需要澄清时暂停；用户
  可 Stop；完成后继续下一项；状态、结果、Lesson 持续写回。
- 明确不是 DAG、不是 Workflow Builder（C9）。

### 3.7 Comment / Message

Message 不是独立导航对象。它永远锚定到：一段选中文字 / 一个 Block /
一个 Task 行 / 一个 Attempt / 一个 Loop / 整个当前页面。线程展示在锚点
附近（文档内联卡片或底部 Composer 区）。

## 4. 信息架构与主界面

### 4.1 左侧：Document Tree

只有一棵多 Project 文档树，没有 Tasks/Sessions/Runs/Workflows 等独立主
导航，也没有目录节点——每一行都是文档。紧凑、接近 Notion：展开/折叠、
hover 出现 `+`（新建子文档）与 `⋯`（context menu）、移动、重命名、删
除、切换 Project、搜索。创建一律经由 hover `+`，无独立 create 按钮。

**树只显示章节级文档**：物化出来的行级 Task/Bug 文档默认不出现在树里
（防止几百个微文档把树炸掉），它们经由父页列表、Loop 引用和搜索到达。

### 4.2 中间：Editable Document Canvas

- 始终可编辑（C3），无 Edit 按钮；
- 普通内容就是正文、标题、列表、引用、代码块；
- 子文档 = 一行链接；
- 只有 Task、Attempt、Loop 等需要交互的内容用 Widget；
- 面包屑显示 Project / 祖先 / 当前文档。

### 4.3 右侧：常驻双区栏（Page Info + Chat）

右栏常驻，上下两区，始终围绕**当前页面**：

- **上：Page Info**——当前文档的结构化数据（frontmatter 的人性化渲
  染）。在 project 页显示 name/workspace/agents；在 task 页显示
  status/attempts/关联文档与 Action；在 loop 页显示当前项/队列/stop
  condition/retry 策略；普通文档显示 type/tags/created 等基本项。不重
  复正文内容，不承担 Session/Diff 管理。
- **下：Chat**——当前页面的对话线程与输入框（见 4.4）。

点开哪个文档，右栏就换成谁的信息与对话；Task/Bug/Loop 都是文档，打开
它们的页面即看到各自的结构化数据——不再有"点击行才出现的对象
Inspector"。

### 4.4 Chat 与文字引用（核心交互）

1. 用户在正文中选择任意文本、Block、Task 行或 Widget 内容；
2. 出现浮动入口；确认后，选区以**纯文字引用**（Markdown blockquote
   `> …`）插入右栏 Chat 的输入框——不使用特殊引用 Widget，文字引用可
   编辑、可拼接、灵活性最高；
3. 用户附加 Comment/Instruction，选择绑定 Agent（Codex/Claude Code/内
   置轻量模型）；
4. 消息携带 Project、文档路径、引用文字与必要的周边上下文；
5. Agent 回复，或对当前页面**提议局部更新**（Proposed update 预览 +
   Copy / Apply update to document 一键应用）。

同一套交互适用于任何页面：正文、Task 页、Loop 页、Bug 页。

## 5. Context 组合

默认 Context：Project 根文档、当前页与祖先页、选区、当前 Task/Loop、显
式引用的子文档或 Section、已关联的 Lesson。

不做复杂 Context Pack 管理页；启动委派前只显示紧凑的"Agent 将看到什么"
预览，可增删引用。Agent 侧按 C7 渐进披露：先读根文档，沿相对链接展开。

## 6. Coding Agent 集成（双向）

### 6.1 出：从 Context Center 委派

从 Task、选区或 Loop 启动 Codex/Claude Code。产品负责组装 Context、发
起外部 session、接收结果摘要、把 Attempt/结果/Lesson 写回文档。

### 6.2 入：文件约定即 API

因为 C1（文件即真相），反向集成的第一形态就是**直接读写 Project 目录**：
外部 Agent 找到根文档 → 沿相对路径读子文档 → 更新 Markdown Section →
新增/更新 Widget 数据 → 追加 Attempt 结果 → 保存 Lesson → 创建子文档。
文件格式约定见 `plan.md` §3；配套提供说明文件（如项目目录内的
AGENTS.md/skill），让 Codex/Claude Code 开箱即会。应用对外部改动的态度：
文件就是真相，重建索引即可。

## 7. Responsive

- 桌面：树 + 画布 + 按需 Inspector；
- 移动：树变抽屉，Inspector 变底部 Sheet，Composer 固定底部，正文全宽
  可读；选择引用与 Comment 在移动端仍是一等交互。

## 8. MVP 范围

**必须包含**：多 Project；Notion 式嵌套文档树；始终可编辑的 Markdown
页面；一行式子文档链接；任意选择引用与 Agent Comment；Task（惰性物化
的任务即文档）与 Task List；Codex/Claude Code 委派；Task 多 Attempt；
Lesson 保存；顺序 Loop；Bug Fix Loop；文件落盘格式 + Agent 反向读写约
定；移动端基本可用。

**不在当前范围**：见 constitution C9 负空间清单。

## 9. 验收标准

v0.6 的 12 条全部保留，新增/改写以 ★ 标注：

1. 左侧可以同时展示两个 Project 及其嵌套文档。
2. 每个文档节点 hover 可新增子文档或打开更多操作。
3. 页面无需进入 Edit 模式即可直接修改。
4. 子文档可作为紧凑的一行链接插入正文。
5. 用户可选择任意文字、Block 或 Task 行，并以文字引用（blockquote）进
   入右栏 Chat 输入框。
6. Agent 回复与局部更新始终保留选择位置和文档上下文。
7. 一个 Task 可拥有多个 Attempt，并可标记当前采用结果。
8. 用户可从 Attempt 保存 Lesson，并在后续 Task 中引用。
9. 一个文档中的多 Task 可组成顺序 Loop。
10. Bug Backlog 可由持续 Loop 逐项处理并写回结果。
11. 不要求用户查看 Diff、Tool Call 或完整 Session Transcript。
12. Codex/Claude Code 可以通过集成读取并更新同一套 Project 文档。
13. ★ 一行 to-do 在被点开/委派/评论前不产生任何文件；物化后父行降级
    为 `- [ ] [标题](路径)`，勾选状态与 Task 文档保持一致。
14. ★ 把 Project 目录直接交给一个外部 Coding Agent（不经应用、不给
    API），它能读懂结构与任务状态，并能以纯文件编辑追加一条格式合规的
    Attempt 记录（C1 检验）。
15. ★ 移动/重命名文档后，正文链接与 Loop 队列引用不断（应用改写路径，
    frontmatter id 兜底）。
16. ★ 行级 Task 文档不出现在左侧树中，但可经父页与搜索到达。
17. ★ 右栏常驻两区：上区渲染当前文档的 frontmatter 结构化数据，下区是
    当前页面的 Chat；切换页面即切换右栏内容。
18. ★ 左侧树与正文中不存在目录节点——树里每一行都是可打开的文档；
    "Workstreams" 这类容器不存在。
19. ★ 文档创建只经由 hover `+` 与 context menu，无独立 create 按钮。

## 10. Open Questions

- **Comment 线程的文件表示**：候选——文档内联降级块 / 伴生
  `<doc>.comments.md` / 仅存应用侧（违 C1，倾向排除）。Stage 1 用内存
  模拟，落盘格式在 Stage 2 前定。
- **并发写**：应用与外部 Agent 同时改同一文件的对账策略（MVP 单人场
  景：last-write-wins + 依赖 git 兜底；是否够用待验证）。
- **Attempt 摘要的生成方**：委派结束时由被委派 Agent 按约定自行写回，
  还是产品内置模型读外部 session 产物后代写？MVP 先取前者。

## 11. 变更清单

### v0.7 → v0.8（第二轮用户裁决）

1. **没有目录**：树中不存在容器节点，一切节点皆文档；目录仅是嵌套的落
   盘形式（§3.1、constitution C2 修宪 2026-08-15 (2)）。样例里的
   "Workstreams" 节点取消，workstream 文档直接挂在 project 下。
2. **`kind` 更名为 `type`，新增 `tags`**：type 可选、命中预定义集合才
   换 icon 与默认字段；删除"按名字猜 icon"（§3.1）。
3. **右栏重构为常驻双区**：上区 Page Info 渲染当前页 frontmatter，下区
   Chat；取代"点击对象才出现的 Inspector"与文档底部 Composer dock
   （§4.3/§4.4）。
4. **选区引用改为纯文字**：blockquote 进 Chat 输入框，不用引用 Chip
   Widget（§4.4）。
5. 创建入口收敛为 hover `+` 与 context menu（§3.1/§4.1）。
6. 验收标准改写第 5 条，新增 17–19 条。

### v0.6 → v0.7

1. **任务即文档**：取代"checkbox → Widget → 子文档"的三级存储升级；渐
   进性移到呈现层，模型层统一为文档 + 惰性物化（§3.3）。
2. **文件即真相**升格为第一不变量（constitution C1），"文档映射文件系
   统"从"最好有"变为"必须"；反向集成第一形态改为文件约定（§6.2）。
3. 钉死五个实现边界：惰性物化触发、单一真相与镜像行、树不显示行级
   Task 文档、链接稳定性（路径 + id 兜底）、多 Task 共享 Session 的
   Attempt 落法（§3.3/§3.4/§4.1）。
4. 澄清：icon 只是默认装饰；Bug 就是 bug 类型的 Task（§3.1/§3.3）。
5. 验收标准新增 13–16 条。
