# Context Center — 未定义清单（Gap Audit v1）

对 spec v0.9 + plan + mock 的全面审核：产品**行为层**的定义远落后于**结
构层**（内容模型/存储已定，"人和 Agent 具体怎么操作"大面积真空）。在
P0 项裁决之前，Stage 1 开发冻结。

标记：▷ = 我的倾向（仅供裁决参考，未裁决前不是决定）；【P0】= 不定就
不能开工。

审核发现的元问题：mock 在真空处自行发明了交互（如 "Add task" 按钮），
这类失真已登记在 §H2，待对应 gap 裁决后统一清理——**mock 不是规范，
以 spec 为准**。

---

## A. 编辑器——用户怎么编辑文档（最大的洞）

- **A1【P0】编辑模型未定**：Notion 式 block 编辑器 / Typora 式所见即所
  得 Markdown / 源码+预览？决定整个前端架构与序列化方案。
  ▷ block 编辑器，但 block 集合以"可无损写回 Markdown"为硬约束（C1/C8）。
- **A2 Block 类型清单**与 Markdown 映射白名单：heading/list/todo/
  quote/code/divider/table?/image?/子文档链接/Widget，各自的降级表示。
- **A3【P0】输入行为**：markdown 快捷输入（`# `、`- `、`[] `）、`/`
  slash 菜单项清单、回车/缩进/块转换/拖拽 handle。**to-do 由编辑器输入
  产生，不是 "Add task" 按钮**（用户已裁决方向；按钮是 mock 失真）。
- **A4 粘贴/复制语义**：markdown/富文本/图片粘贴（图片落盘位置约定，如
  `assets/`）、外链。
- **A5 保存与冲突**：写盘时机（debounce？）、编辑期间文件被外部（agent）
  修改的合并与提示、undo/redo 的边界（跨保存？跨 agent 改动？）。
- **A6【P0】标题与文件名**：title 的真相来源（文件名 slug / 首行 H1 /
  frontmatter title？）、rename 联动改名规则、中文与特殊字符的 slug 化。
  当前落盘格式里根本没有 title 字段——mock 页面标题的来源其实未定义。
- **A7【P0】选区引用的锚定**：纯 Markdown 无 block id，引用发出后文档被
  编辑，Apply 时"原位置"如何定位（文字匹配容错？段落指纹？失配降级？）。
- **A8 空文档初始态**与 placeholder（新建后光标落哪，"Type '/' for
  commands…"？）。

## B. Frontmatter / Metadata 编辑（明确是独立问题）

- **B1【P0】Metadata 面板的编辑能力**：哪些字段可改、控件形态（status
  下拉、tags token 输入、type picker、icon picker、workspace 路径选择、
  agent 绑定）。当前 mock 全部只读 + 假下拉箭头。
- **B2 type 的赋予与变更**：入口在哪；变更副作用（doc→task 获得哪些默
  认字段；task→doc 时 attempts/lessons 何去何从）。
- **B3 自定义 frontmatter 字段**：允许用户加任意 key 吗？未知字段在面板
  怎么显示/编辑？
- **B4 原始 frontmatter 视图**（view raw）：给不给 power user/排错入口。

## C. Agent 协议——Agent 怎么创建/修改内容

- **C1【P0】提议式 vs 直改式的边界**：Proposed update+Apply 与"直接落
  盘+可撤销"各适用于什么（按 agent 类型？改动大小？用户设置？）。mock
  里两种叙事并存但无规则。
- **C2【P0】Agent 创建文档的流程**：建在哪个父节点下、命名、type 赋予、
  创建后如何呈现给用户（树上出现？chat 里给链接？需要确认吗？）。
- **C3 Proposed update 的作用域模型**：目标=选区/块/Section/整页/多文
  件，各自允许吗；预览形态（不做 diff 的前提下：只展示新文本？新旧上下
  对照？）。
- **C4 Apply 的事务与撤销**：目标漂移时失败处理；Apply 后 undo；改动在
  chat 中的留痕格式。
- **C5 Chat 动作清单与 slash command**：/task /delegate /new-doc /loop
  …（原始需求提过 slash）——定义哪些、参数、是否需确认。
- **C6 内置轻模型 vs 外部 agent 的 UI 分工**：agent 选择器有哪些选项、
  quote 小改写默认给谁、切换粒度。
- **C7【P0】委派生命周期**：Delegate 点击后的完整链路——Context 预览
  （spec 承诺的"Agent 将看到什么"，UI 从未设计）→ 确认 → 运行中状态呈
  现在哪 → 完成通知 → Attempt 落地方式。目前完全未定义。
- **C8 外部 session 引用与 "View run"**：跳转外部（Codex web/终端）还是
  仅显示 id？与 C9"不做 session 浏览器"的边界写清。
- **C9 外部 agent 改盘的感知**：file watcher？UI 提示（"Codex updated
  this page"）？正在编辑的页被改怎么办（关联 A5）。
- **C10 Chat 文本中实体引用的链接化**：#43、文档名如何变成可点链接（渲
  染端识别 or agent 输出约定）。

## D. 树与文档管理操作

- **D1【P0】新建文档流程**：hover `+` 之后——立即创建 Untitled 并聚焦
  重命名？quick input？type 默认无、何时设？
- **D2 context menu（⋯）完整清单**：rename/move/duplicate/delete/copy
  link/change icon/change type…各项行为。
- **D3【P0】删除语义**：软删（trash+可恢复）vs 硬删；被 loop/链接引用时
  的警示与断链处理。（agentrunner 有"删除语义不可逆需用户裁决"的前车之鉴）
- **D4 移动/重排交互**：树内拖拽改父级+改顺序；跨 project 移动允许吗
  （id/display id 冲突怎么办）。
- **D5 New project 流程**：表单（name/workspace 路径/agents/默认 agent）
  与初始文档模板。
- **D6 搜索**：范围（标题/全文/跨 project）、入口与快捷键（cmd+k quick
  switcher？）、结果 UI。mock 只有一个放大镜图标。
- **D7 树的 active 高亮与行级 task 页**：打开 task-43 时树上高亮谁（最
  近可见祖先？）。mock 当前无高亮。
- **D8 icon 手动设置入口**（C2 定了优先级"手动>type 预设"，但设置 UI 不
  存在——emoji picker？）。
- **D9 project 折叠记忆与多 project 排序**。

## E. Task / Loop / Bug 语义细节

- **E1【P0】物化的用户入口**：自动触发之外，用户如何手动"把这行变成
  task"（hover 菜单/选中操作/slash 转换？）；物化瞬间的 UI 反馈（行尾出
  现 #id？）。
- **E2【P0】镜像行的编辑冲突**：直接改镜像行文字=rename task？勾
  checkbox=改 status（要确认吗）？删除镜像行=删 task 还是仅移出列表？
- **E3 状态机与副作用**：blocked 状态（spec 有、mock 从未出现）；loop
  运行中手动改 task 状态；done 任务重开。
- **E4 Loop 控制细节**：启动前校验（队列含未物化行？）、Pause/Stop 对进
  行中 session 的处理（中断/等完）、失败暂停的呈现与恢复路径。
- **E5 并发与互斥**：同一 task 同时被 loop 与手动委派；两个 loop 引用同
  一 task；运行中 task 被编辑。
- **E6【P0】display id 规则成文**：task=#N、bug=#BUG-N 是 mock 暗含的，
  plan 只定义了内部 id（t-43/bug-142）；两者映射与计数器规则要写进 plan。
- **E7 Lesson 工作流**：Save as lesson 的落点与命名、与来源 attempt 的
  双向引用、后续委派时 lesson 如何进 context（自动带关联？手动勾选？）。

## F. Chat

- **F1 线程模型**：每文档单线程够吗；历史分页/折叠。
- **F2 消息操作**：编辑/删除/重发、复制引用、proposed 卡片折叠。
- **F3 Agent 选择器作用域**：per-message / per-thread / project 默认。
- **F4 跨页引用**：quote 其他文档内容（@page）允许吗。
- **F5【P0】非当前页的动静感知**：C9 禁了全局 inbox，但"别的页有新回复
  /loop 完成"用户如何知道（树行 dot？项目根聚合？完全不提示？）——真空。

## G. 系统级

- **G1 vault 选择与多 vault**（onboarding 第一屏是什么）。
- **G2 错误/空态**：未绑 workspace 时的委派按钮、agent 不可用（无 CLI/
  key）、frontmatter 解析失败的呈现。
- **G3 快捷键体系**（cmd+k / cmd+n / cmd+enter / esc…）。
- **G4 多窗口同 vault**：内存态与 SQLite 的并发同步。
- **G5 后台活动指示**：别的项目有 loop 在跑，全局哪里看得出"有东西在跑"。
- **G6 移动端逐交互适配**：hover 依赖（树箭头/行操作）、拖拽、选区引用
  的触屏替代方案。
- **G7 主题外观**：dark mode 做不做、密度。

## H. 交付流程

- **H1 验收标准 ↔ Stage 1 任务映射**：19 条验收哪些 mock 可见、哪些只能
  Stage 1 验证——建立对照，避免 mock 与 spec 再脱节。
- **H2 mock 已知失真登记**（待对应 gap 裁决后统一清理）：
  - "Add task / Add bug / Add resolved bug / Add learning" 按钮行——应
    由编辑器输入行为替代（A3）；
  - 假 contenteditable（无 block 行为、不落盘）；
  - Metadata 面板只读 + 假下拉箭头（B1）；
  - Chat 发送无后果、Apply 只变按钮文案（C4）；
  - "View run" 无目标（C8）；
  - 树 active 高亮在 task 页缺失（D7）。

---

## P0 短名单（裁决完才能开 Stage 1）

| # | 项 | 一句话问题 |
|---|----|-----------|
| A1 | 编辑模型 | block 编辑器还是 WYSIWYG Markdown？ |
| A3 | 输入行为 | markdown 快捷输入 + slash 菜单定义（to-do 从这来） |
| A6 | 标题↔文件名 | title 真相来源与 rename 联动 |
| A7 | 引用锚定 | 无 block id 时 Apply 如何定位原位置 |
| B1 | Metadata 编辑 | 面板哪些字段可编辑、控件形态 |
| C1 | 提议 vs 直改 | Agent 改动两种模式的适用边界 |
| C2 | Agent 建文档 | 位置/命名/type/呈现/确认 |
| C7 | 委派生命周期 | Context 预览→确认→运行中→结果落地全链路 |
| D1 | 新建文档流程 | hover + 之后发生什么 |
| D3 | 删除语义 | 软删 vs 硬删 + 被引用时的处理 |
| E1 | 物化入口 | 用户手动把一行变成 task 的方式 |
| E2 | 镜像行冲突 | 改文字/勾选/删行的语义 |
| E6 | display id | #N 与 #BUG-N 规则成文 |
| F5 | 跨页动静感知 | 无 inbox 前提下的轻量提示方案 |

裁决方式建议：逐项拍板或批量圈掉"按我的 ▷ 倾向执行"的子集；每次裁决
写回本文件并同步 spec/plan 对应章节。

---

## 裁决记录

- **2026-08-16（用户）**：Stage 1 组件底座 = **shadcn/ui + Radix UI，
  能用则用**（落于 plan §1/§2/§4）。影响：B1/D2/D6/C7 等涉及下拉、
  菜单、弹层、对话框、toast 的项，其**实现底座已定**，待裁决的只剩字段
  与行为本身；F5 的提示载体多一个现成选项（Radix Toast）。**不因此关
  闭任何 gap**——A1 编辑器不在 shadcn/Radix 覆盖范围，仍是 P0。
