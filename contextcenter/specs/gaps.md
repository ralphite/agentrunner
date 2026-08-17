# Context Center — 未定义清单（Gap Audit v2，Notion 基准裁决后）

v1 登记了 52 项行为层未定义。2026-08-16 用户裁决：**UX 基准 = Notion，
凡 Notion 有对应交互的一律照抄其体验**。据此逐项标注：

- ✅(Notion) = 按 Notion 对应交互解决，答案已写死在本项下
- ✅(自决) = 非 Notion 覆盖但足够明确，按"明确可自行确认"规则定案
- 🟡 = Notion/自决覆盖一部分，**残留**部分仍待裁决（残留已写明）
- ⬜ = Notion 不涉及，仍开放

统计：✅ 34 · 🟡 7 · ⬜ 11。**P0 已全部清零（2026-08-16 (4)），行为层
开发解冻**；剩余 🟡/⬜ 均为非阻塞项，后续以成套方案逐批裁决。
mock/app 与已解项不符的地方随 Stage 1 编辑器落地时统一清理（H2）。

---

## A. 编辑器

- **A1 编辑模型 ✅(Notion)**：block 编辑器——每行/每块一个 block，拖拽
  handle、块类型转换，硬约束：块集合必须能**无损写回 Markdown**
  （C1/C8）。编辑器库选型（TipTap/Lexical/自研）属工程决策，plan 定。
- **A2 Block 类型清单 ✅(Notion 裁剪)**：paragraph、H1–H3、bulleted /
  numbered list、to-do、quote、code(带语言)、divider、GFM table、
  image、子文档链接、callout(→ blockquote+emoji 降级)、Task/Loop
  Widget（降级表示已定）。超出此白名单的 Notion 块不引入。
- **A3 输入行为 ✅(Notion)**：markdown 快捷输入（`# ` `- ` `1. `
  `[] ` `> ` ```` ``` ````）、`/` slash 菜单（插入 A2 全部块型）、回车
  分块、Tab 缩进、拖拽 handle、多块选择。**to-do 由输入产生**；
  "Add task" 类按钮废弃（H2 清理）。
- **A4 粘贴/复制 ✅(Notion + 自决落盘)**：粘贴 markdown 解析成块、富文
  本转块、图片粘贴自动落盘到文档同级 `assets/` 并插入相对链接。
- **A5 保存与冲突 🟡**：已解——自动保存、无保存按钮、统一 undo 栈
  （Notion）。**残留**：写盘 debounce 参数与外部并发改动的合并策略
  （关联 C9，文件系统特有，Notion 云端无此问题）。
- **A6 标题↔文件名 ✅(裁决 2026-08-16 (4))**：标题即页首大字、直接编
  辑、树与面包屑实时跟随（Notion）；**文件名 = 标题**（中文保留原
  文），rename 即改文件名并改写全部入链，frontmatter 不设 title，同名
  加 `-2`，超长截断。
- **A7 引用锚定 ✅(裁决 2026-08-16 (3))**：智能匹配 + 降级——锚 =
  引用文字 + 就近标题路径，Apply 时模糊匹配定位；置信不足时**不硬
  改**，提示用户手动放置。不向 Markdown 注入 block id，文件保持纯净。
- **A8 空文档初始态 ✅(Notion)**：新页聚焦标题，标题占位 "Untitled"，
  正文占位 "Type '/' for commands…"。

## B. Frontmatter / Metadata 编辑

- **B1 Metadata 编辑 ✅(Notion 属性面板)**：字段点击即变控件——status/
  type→下拉（Radix Select/DropdownMenu）、tags→multi-select token 输
  入、日期→只读（DB 维护）、文本→行内输入、icon→emoji picker、
  workspace→路径输入。字段清单 = plan §3.3 各 type 字段。
- **B2 type 赋予/变更 🟡**：已解——入口 = Metadata Type 字段下拉 +
  ⋯ 菜单 "Turn into"（Notion 模式）。**残留**：降级副作用（task→doc
  时 attempts/lessons 字段何去何从；▷ 保留 frontmatter 不删，UI 不再
  渲染）。
- **B3 自定义字段 ✅(Notion properties)**：允许任意自定义 frontmatter
  key；Metadata 面板底部 "+ Add a property"；未知字段按文本渲染可编辑。
- **B4 view raw ✅(自决)**：⋯ 菜单 "View frontmatter"，只读抽屉起步
  （app topbar 菜单已放入口）。

## C. Agent 协议

- **C1 提议 vs 直改 ✅(裁决 2026-08-16 (3))**：**内置轻模型**对选区/局
  部小改 = Notion AI 形态（内联预览 + Accept/Discard）；**外部 coding
  agent = 直改 + 事后可撤**——直接落盘，应用实时高亮改动、chat 留痕、
  cmd+Z 可撤、通知提醒。不设提议缓冲层，Loop 连跑不中断。
- **C2 Agent 建文档 ✅(裁决 2026-08-16 (3))**：直接创建、无事前确认。
  默认建在其正在工作的文档之下（或 agent 判断的合理父级）；agent 起
  title，slug 自动；树上即时出现 + chat 给出链接 + 通知提醒。
- **C3 Proposed update 作用域 🟡**：已解——预览形态 = 原位内联新文本 +
  Accept/Discard（Notion AI），不做 diff 视图。**残留**：允许的作用域
  清单（选区/块/Section/整页/多文件）。
- **C4 Apply 事务/撤销 🟡**：已解——Apply 后可 cmd+Z（进统一 undo 栈，
  Notion）。**残留**：目标漂移失败的处理（关联 A7）；chat 留痕格式。
- **C5 slash command（chat 动作）⬜**：块插入类已归 A3；chat 里的
  /task /delegate /new-doc 等动作命令集仍开放。
- **C6 内置 vs 外部 agent 分工 ⬜**。
- **C7 委派生命周期 ✅(裁决 2026-08-16 (3))**：**轻确认**——点
  Delegate 弹紧凑卡片，列出将携带的上下文（Project 根文档、当前页、
  选区引用、关联 lesson），可增删，点 Start 开跑；运行中来源行
  spinner + Metadata 显示 Running；完成后通知 + Attempt/结果写回。
- **C8 View run 目标 ⬜**。
- **C9 外部改盘感知 🟡**：已解——体验照 Notion 协同：外部改动实时反映
  + 短暂高亮闪现，无弹窗打断。**残留**：机制（watcher/轮询）与正在编
  辑块被外部改动的合并（并 A5）。
- **C10 实体链接化 ✅(Notion mention)**：`@` 呼出页面 mention（插入普
  通链接文本，不引入特殊 widget，符合"文字引用"裁决）；渲染端自动
  linkify `#43` / `#BUG-142` 到对应文档。

## D. 树与文档管理

- **D1 新建流程 ✅(Notion)**：hover `+` → 立即创建 Untitled 子页并打
  开、光标落标题；type 默认无、事后在 Metadata 设。
- **D2 ⋯ 菜单 ✅(Notion + 两项自有)**：Rename、Change icon、
  Duplicate、Move to…、Copy link、Delete（红）；自有追加：Turn
  into（B2）、View frontmatter（B4）。
- **D3 删除语义 ✅(Notion)**：软删进 Trash（侧栏底部入口，可恢复/永久
  删）；指向已删页的链接标记 broken；落盘 ▷ 移入
  `<vault>/.contextcenter/trash/`。被 Loop 引用时删除弹确认并从队列
  移除。
- **D4 移动/重排 ✅(Notion)**：树内拖拽改父级+顺序（蓝线指示落点），
  顺序入 SQLite child_order；跨 project 拖拽允许（display id 迁移规则
  归 E6）。
- **D5 New project ✅(Notion 渐进式)**：即建即配——先创建空 project
  页，workspace/agents 在 Metadata 面板补全；未绑 workspace 时委派动
  作禁用并提示（G2）。
- **D6 搜索 ✅(Notion Quick Find)**：cmd+K 面板：标题+全文、最近访问、
  跨 project；行级 task 文档可被搜到。
- **D7 树高亮 ✅(自决)**：打开不在树中的行级文档时，高亮其最近可见祖
  先；面包屑给出精确位置。
- **D8 icon 设置 ✅(Notion)**：点击页 icon 或菜单 Change icon → emoji
  picker；写 frontmatter `icon`。
- **D9 折叠记忆/排序 ✅(Notion)**：折叠态与 project 顺序记忆
  （SQLite ui_state）；project 拖拽排序。

## E. Task / Loop / Bug 语义

- **E1 物化入口 ✅(Notion "Turn into")**：行 hover ⋯/拖柄菜单 →
  "Turn into task"；自动触发（点开/委派/被 Loop 引用）保留；物化反馈
  = 行尾浮现 #id。
- **E2 镜像行冲突 ✅(Notion 类比)**：改镜像行文字 = rename task（同步
  title，synced 语义）；勾选 checkbox = 改 status，即点即改无确认
  （Notion to-do）；删除镜像行 = 仅断开列表引用，task 文档仍在原处
  （经搜索/其他引用可达），不删除文档。
- **E3 状态机副作用 ⬜**：blocked 的呈现可照 pill 体系，但 loop 中改状
  态的副作用、done 重开语义仍开放。
- **E4 Loop 控制 ⬜**：启动校验、Pause/Stop 对进行中 session 的处理、
  失败暂停呈现。
- **E5 并发互斥 ⬜**。
- **E6 display id ✅(自决)**：task/普通条目 `#N`（project 内全局连续计
  数）；bug `#BUG-N`（独立计数）；id 一经分配永不复用；跨 project 移
  动时在目标 project 重新分配并在 frontmatter 记 `moved_from`。落
  plan §3.2。
- **E7 Lesson 工作流 ⬜**：落点/命名可类比（▷ 子文档 + type:
  lesson），但"后续委派如何选入 context"仍开放。

## F. Chat

- **F1 线程模型 ✅(Notion comments 裁剪)**：每文档单线程 + 滚动加载历
  史；不引入 resolve/多线程。
- **F2 消息操作 ✅(Notion)**：hover ⋯ → Edit / Delete / Copy text。
- **F3 agent 选择器作用域 ✅(自决)**：默认 = project 的 default_agent；
  thread 内记忆上次选择；每条消息可临时切换。
- **F4 跨页引用 ✅(Notion mention)**：`@page` 插入链接；选区 quote 仍
  限当前页，跨页内容以链接指入。
- **F5 跨页动静感知 ✅(裁决 2026-08-16 (4))**：顶栏铃铛（有新事件亮计
  数）+ popover 事件列表（loop 完成/失败、agent 回复、外部改动，点击
  跳转）+ 树行小蓝点标未读。popover 非独立页面，不违 C9。

## G. 系统级

- **G1 vault 选择/onboarding ⬜**：桌面/本地特有，Notion 不涉及。
- **G2 错误/空态 🟡**：已解——空态文案风格照 Notion（轻、灰、带一个动
  作）；未绑 workspace 委派禁用+提示（D5）。**残留**：agent 不可用
  （无 CLI/key）与坏 frontmatter 的呈现。
- **G3 快捷键 ✅(Notion 子集)**：cmd+K Quick Find、cmd+N 新页、
  cmd+Shift+H 返回、cmd+\\ 收侧栏、esc 关面板；编辑器内快捷键随 A3。
  清单落 plan 附录。
- **G4 多窗口同步 ⬜**：实现层（SQLite 并发 + 内存态失效）。
- **G5 后台活动指示 ✅(随 F5)**：全局"有东西在跑"= 铃铛处转圈 + 计数。
- **G6 移动端 🟡**：形态照 Notion mobile（抽屉树、底部工具条、选区评
  论工具条）；逐交互适配清单仍要产出。
- **G7 主题 ✅(自决)**：MVP light-only；dark 后置（token 体系已预留）。

## H. 交付流程

- **H1 验收标准↔Stage 1 映射 ⬜**：工作项，裁决后我产出对照表。
- **H2 mock/app 失真清理 ⬜→工作项**：A3/B1/D2 等已解项落地时，废弃
  "Add task" 按钮系列、假 contenteditable、只读 Metadata 等（清单见
  v1）。

---

## P0 状态：**已清零** 🎉

全部 14 项原 P0 均已关闭：A1、A3、B1、D1、D3、E1、E2、E6（Notion/自
决）；A7、C1、C2、C7（用户裁决 (3)）；A6、F5（用户裁决 (4)）。

剩余开放项已全部整理为成套建议方案，见 **`pending-decisions.md`**
（D1–D15 + 工作项 W1/W2）：A5+C9→D1、B2→D2、C3→D3、C4→D4、C5→D5、
C6→D6、C8→D7、E3→D8、E4→D9、E5→D10、E7→D11、G1→D12、G4→D13、
G2→D14、G6→D15、H1→W1、H2→W2。

---

## 裁决记录

- **2026-08-16（用户）**：Stage 1 组件底座 = shadcn/ui + Radix UI。
- **2026-08-16 (2)（用户）**：**UX 基准 = Notion**——凡 Notion 有对应交
  互的 gap 照抄其体验，逐项标注如上（✅28 / 🟡9 / ⬜15）；连带自决项
  一并定案（A4 assets、B4、D7、E6、F3、G7）。已同步 spec §4 与 plan
  §3。
- **2026-08-16 (3)（用户）**：P0 四项拍板——**A7** 智能匹配+降级（不注
  入 id）；**C1/C2** 外部 agent 直改+事后可撤（建文档同，直接建+通
  知）；**C7** 委派轻确认（上下文卡片→Start→运行态→写回）。已同步
  spec §4.4/§6。
- **2026-08-16 (4)（用户）**：最后两项确认——**A6** 标题即文件名（中文
  保留、rename 改写入链、无 frontmatter title）；**F5** 铃铛 +
  popover + 树行蓝点（G5 随之关闭）。**P0 清零，行为层解冻。**
