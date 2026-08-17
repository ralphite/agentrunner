# Context Center — 待决清单（Pending Decisions v1）

gaps.md 清零 P0 后剩余的**全部**未决之处，合并整理为 15 个决定（D1–
D15）+ 2 个工作项。每项给出**建议方案**（完整可直接采纳）与备选。裁决
方式：逐项拍板，或批量圈"按建议执行"的子集；裁决后写回 gaps/spec/plan。

附录 A 是**已决事项 review 清单**（一行一条），供下一轮回头审。

---

## D1 文件同步（原 A5+C9 残留）——写盘时机与外部改动合并

**问题**：编辑何时落盘？外部（agent/另一窗口）改了同一文件怎么合？

**建议**：
- 写盘：编辑停顿 ~800ms 或失焦即写；原子写（temp + rename）。
- 感知：Stage 2 用 file watcher；Stage 1 内存树无此问题。
- 合并：外部改动到达时——本地无未落盘编辑 → 直接刷新 + 改动处高亮闪
  现；正在编辑同一文档 → **块级合并**：不冲突的块自动合并，同一块两边
  都改时保留本地版本、块旁标"外部版本可用"，点击可查看/采纳。绝不弹
  阻断式对话框。
- undo 只回退自己的编辑，外部改动不进本地 undo 栈。

## D2 type 变更副作用（原 B2 残留）

**问题**：文档在 task/bug/普通 doc 之间转换时，结构化数据怎么办？

**建议**：升级（doc→task）补默认字段（status: todo），正文不动；降级
（task→doc）**frontmatter 全部保留不删**（数据无损），UI 停止渲染
task 专属区，父页镜像行退化为普通链接（去 checkbox），display id 保留
不回收；再升回来一切复原。

## D3 Proposed update 作用域（原 C3 残留）

**问题**：内置模型的"提议更新"允许针对哪些范围？

**建议**：选区 / 单块 / 单 Section / 整页，四档；**一次提议只针对一个
文档**，跨文档需求拆成并列多条提议卡片。（外部 agent 走直改不经提议，
所以此机制保持小。）

## D4 Apply 留痕（原 C4 残留）

**问题**：提议被 Apply 后，chat 里如何留痕？

**建议**：卡片折叠为一行"✓ Applied · 查看"（点击展开原文），文档改动
处高亮闪现一次；定位失败时卡片显示"未能定位"状态并提示手动放置（承
A7）。

## D5 Chat slash 命令集（原 C5）

**问题**：chat 输入框里支持哪些 `/` 动作命令？

**建议**：V1 只做四个高频项，全部等价于已有 UI 动作的快捷方式——
`/task`（引用/描述 → 新 task）、`/delegate`（委派当前页或引用）、
`/doc`（新建子文档）、`/lesson`（引用 → lesson）。参数自由文本由
agent 解析，回车前显示预览行。其余诉求走自然语言。

## D6 内置模型 vs 外部 Agent 分工（原 C6）

**问题**：agent 选择器里有谁？活怎么分？

**建议**：选择器 = **Assistant（内置轻模型）/ Codex / Claude Code**。
Assistant 管纯内容操作：选区改写、总结、生成 task 描述、提取 lesson、
页内问答——quote 改写类默认选它；一切涉及代码 workspace 的执行/研究走
外部 agent；**Delegate 动作永远是外部 agent**。

## D7 View run 跳转（原 C8）

**问题**：运行中/历史 attempt 的 "View run" 点开是什么？

**建议**：纯外链——打开外部系统自己的 session 页面（Codex web /
Claude Code session 链接），新窗口；拿不到链接就不显示按钮，Metadata
只留 session id 文本。产品内不做任何 run 浏览界面（守 C9）。

## D8 Task 状态机副作用（原 E3）

**问题**：blocked 怎么用？loop 运行中人工改状态怎么办？done 能重开吗？

**建议**：状态集 todo/in_progress/done/blocked。blocked 可手动设、
agent 失败后可建议设；**loop 遇 blocked 跳过并记录**，收尾时汇报。
loop 运行中人工把某项改 done → loop 视作完成继续下一项；改 todo/
blocked 不影响已在跑的 session（结果照记 attempt）。done 可直接改回
（attempts 历史保留，新 attempt 续号）。

## D9 Loop 控制细节（原 E4）

**问题**：启动校验、Pause/Stop 语义、失败暂停的界面。

**建议**：Start 时队列中未物化行自动物化；空队列/全 done 则 Start 置
灰。**Pause = 跑完当前项再停**；**Stop = 请求中断当前外部 session**
（断不了标 abandoned），已有结果保留。失败暂停：loop 页/widget 顶部红
条 "Paused on failure at #43 · Retry / Skip / Stop"，铃铛推事件；
Skip = 记 lesson 后跳下一项。

## D10 并发互斥（原 E5）

**问题**：同一 task 被两处同时委派？两个 loop 引用同一 task？

**建议**：**一个 task 同时至多一个进行中 attempt**。已在跑时 Delegate
按钮变 "View running attempt"；loop 到达一个正在跑的 task 时默认跳过
并记录（不排队等待）。同一文档"人编辑 + agent 改"并存走 D1 合并。

## D11 Lesson 进 context（原 E7）

**问题**：lesson 存哪、叫什么、后续委派怎么被带上？

**建议**：lesson = 普通子文档（type: lesson），默认建在来源 task/文档
同级，文件名=标题（承 A6）。委派时自动携带 = 当前 task frontmatter
`lessons:` 引用的 + 当前页/祖先页正文内联引用的 lesson；在轻确认卡片
（C7）里可见、可增删。不做全局自动相关性检索（Stage 2+ 再议）。

## D12 Vault 与 onboarding（原 G1）

**问题**：数据根目录哪来的？支持多 vault 吗？

**建议**：V1 **单 vault**：首启选目录（默认 `~/ContextCenter`），设置
可换；不做多 vault 切换器。New project 即 vault 顶层建文档（D5 已定）。

## D13 多窗口（原 G4）

**问题**：同一 vault 开两个窗口会怎样？

**建议**：允许。文件 + SQLite(WAL) 本就是共享真相，**把另一个窗口当作
"外部改动来源"处理**，完全复用 D1 的感知与合并机制，不做窗口间专用同
步协议。

## D14 错误与空态（原 G2 残留）

**问题**：agent 没配置、frontmatter 坏掉时界面什么样？

**建议**：agent 未配置——委派卡片顶部黄条"Codex CLI 未检测到 + 配置指
引"，Start 置灰。坏 frontmatter——文档照常打开可编辑，Metadata 区顶部
黄条"解析失败 · View raw 修复"，该文档结构化功能降级，不阻断阅读。

## D15 移动端范围（原 G6）

**问题**：移动端 V1 到底做到哪？

**建议**：**阅读优先 + 四个基本动作**：树抽屉浏览、文档阅读、chat 收
发、长按选区 → Quote 引用。编辑仅限纯文本级；hover/拖拽类交互一律用
⋯ 菜单替代；Metadata/Chat 为底部 Sheet（既有裁决）。超出此范围 V1 不
做。

---

## 工作项（不需要裁决，我来做）

- **W1（原 H1）**：验收标准 ↔ Stage 1 任务对照表，随 Stage 1 开工产出。
- **W2（原 H2）**：mock/app 失真清理（"Add task"按钮系列、假
  contenteditable、只读 Metadata 等），随编辑器与 Metadata 实装移除。

---

## 附录 A：已决事项 review 清单（供回头审）

宪法（constitution.md，修宪 3 次）：
- C1 文件即真相 + SQLite 伴随库两储分工；C2 一切皆文档、没有目录、
  type/tags 驱动 icon；C3 永远可编辑；C4 选择即上下文；C5 执行外包；
  C6 结果回流；C7 渐进结构（惰性物化）；C8 少而稳 Widget；C9 负空间
  清单。

结构与存储（spec §3、plan §3）：
- 任务即文档 + 惰性物化；镜像行 `- [ ] [标题](路径)`；文件+同名同级目
  录落盘（无 tasks/ 专用目录）；attempts 结果存 task frontmatter；多
  task 共享 session 各记各的 attempt；SQLite 七表（chat/顺序/ui 态/
  runtime/时间戳/计数器/索引）；display id `#N`/`#BUG-N` 永不复用；
  assets/ 图片落盘；trash 软删目录；**文件名=标题**（中文保留、rename
  改写入链、同名 -2）。

UX（Notion 基准批量裁决 + 后续拍板）：
- block 编辑器 + 块白名单 + markdown 快捷输入 + slash 插入 + 空态占位；
  to-do 由输入产生（无 Add 按钮）；Metadata 属性面板控件 + 自定义字段
  + view raw；树：hover icon 变箭头、即点即建、⋯ 菜单、拖拽移动、
  cmd+K 搜索、emoji picker、折叠记忆、行级文档高亮最近祖先；软删
  Trash；"Turn into task" 物化 + 镜像行三语义（改文字=rename、勾选=改
  状态、删行=断链不删档）；chat 单线程 + 消息编辑删除 + @page mention
  + 自动 linkify；快捷键 Notion 子集；MVP light-only。
- 三栏布局：无目录树 / 画布 / 右栏 Metadata+Chat；ChatGPT 式输入框；
  文字引用（blockquote）；项目 icon 与普通 icon 同风格；in-progress
  = spinner。

Agent 协议（P0 裁决）：
- 引用锚定=文字+标题路径模糊匹配，失配提示手动放置，不注入 block id；
  内置模型=内联 Accept/Discard；外部 agent=直改+事后可撤（高亮/留痕/
  undo/通知），建文档直接建；委派=轻确认卡片（上下文可增删）→ Start
  → 运行态 → 写回；动静=铃铛+popover+树行蓝点。

技术栈：
- React + Vite + TS + Tailwind v4；shadcn/ui + Radix 能用则用（源码拷
  入换肤）；lucide 图标；Stage 1 前端内存实现 store，Stage 2 真
  SQLite + watcher。
