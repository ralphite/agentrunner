# INC-41 Codex Parity Gap Ledger（2026-07-11 全景比对）

**方法**:24 张真实 Codex 金标截图(`qa/codex-reference/`)× playwright 截取的
我方 live 8809(`qa/runs/2026-07-11-parity-capture/ours/`),13 个并发 finder 逐屏/
逐组件比对。仅收**实质功能/UI/UX 差距**,已剔除 a11y/性能。按用户可见价值排序。

状态:☐ 未做 · ▶ 本轮并发实现中 · ✅ 已关闭

---

## P1 — 最高价值(最刺眼、最影响"好不好用")

| # | 组件 | 差距(Codex 有 / 我们缺) | 源 | 状态 |
|---|---|---|---|---|
| 1 | new-task home | 主区**全空白**:缺 "What should we build in {project}?" 大标题 + 4 张 suggestion 卡(彩色图标)+ 品牌图标。CSS 骨架(`.home-hero-icon`/`.hero h2`)已存在却无元素使用 | Home.tsx, styles | ▶ A |
| 2 | review / diff | 大 diff **默认全部折叠**成裸文件头(>10 文件或 >500 行即触发),"打开 Changes 只看到文件名、看不到代码" | diffSummary.ts, DiffView.tsx | ▶ B |
| 3 | diff render | 无 "N unmodified lines" 上下文折叠带 / 无完整文件上下文(需后端按需上下文接口) | DiffView.tsx, meta.go, api.ts | ☐ 后端 |
| 4 | diff / panel | 无 push:只有本地 `Commit changes…`,无 "Commit or push"/开 PR(前后端都缺 push 端点) | api.ts, meta.go, DiffView/SupervisionPanel | ☐ 后端 |
| 5 | change card | 无 Undo/还原(整轮或逐文件 revert;后端无 revert 端点) | ChangesOutcome.tsx, api.ts | ☐ 后端 |
| 6 | scheduled | 列表是**一次性运行历史**而非重复计划;行 sub-line 无 cadence + next-run(需后端 cadence 字段) | Scheduled.tsx, api.go, spec.go | ☐ 后端 |
| 7 | global IA | 一级导航只有 2 个目的地(New task/Scheduled),Codex 有 6(缺 Pull requests/Chat/Plugins/Sites) | Sidebar.tsx, store.ts, App.tsx | ☐ 大 |
| 8 | diff render | 修改文件头无 M/A/D 状态字形(最常见的"修改"无任何徽标) | diffSummary.ts, DiffView.tsx | ▶ B |

## P2 — 高价值

| # | 组件 | 差距 | 源 | 状态 |
|---|---|---|---|---|
| 9 | change card | "Show N more files" 跳去 Review 而非**就地展开**;文件行未 dim-dir/bold-base;计数不右对齐 | ChangesOutcome.tsx | ▶ E |
| 10 | artifact card | 只有 Download,缺 "Open in ▾";无 "Show N more" 折叠 | ChangesOutcome.tsx | ▶ E |
| 11 | command palette | 无未读蓝点;⌘1–9 编号给了 attention 而非最近任务(与 Codex 相反);空 query 应任务优先 | CommandPalette.tsx | ▶ C |
| 12 | scheduled | 缺 "Suggestions" 区(模板任务);筛选应为 All/Active/**Paused** 而非 Completed | Scheduled.tsx | ▶ D |
| 13 | composer | chip 区是独立灰面板,与白卡形成**双描边接缝**(应合成一张卡);模型 pill 默认藏了 effort 维度 | Composer.tsx, styles | ☐ |
| 14 | model selector | 缺 Speed 行 + Advanced 折叠;应改成 label+值+chevron 的 drill-in(现在两长列全展开占屏) | Composer.tsx, specs.ts | ☐ |
| 15 | add menu | 文件选择不支持多选/文件夹;缺 Plugins 区(Documents/PDF/Spreadsheets/…) | Composer.tsx | ☐ |
| 16 | thread panel | 分支行只读(有 `gitCheckout`/`gitBranches` 却没接);缺 Browser/Sources 区;消息行无 👍/👎/share | SupervisionPanel.tsx, Timeline.tsx | ☐ |
| 17 | diff header | 缺 copy-diff、overflow "…"、放大/全屏、文件排序、多会话 tab 条 | DiffView.tsx, SessionView.tsx | ☐ |
| 18 | diff render | hunk 间空白 hairline 无信息;changed 行无 hatched gutter 标记;inline 双行号列应合一 | DiffView.tsx, styles.rs.css | ▶ B |

## P3 — 打磨(择机)

sidebar 项目图标不分 repo 类型 · 行内时间戳挤压标题 · 行尾统一 pop-out 无语义 ·
composer chip 多余 caret + environment 图标错(应 gear)· 高危 access 用点而非警告字形 ·
command palette 分组大写/命名 · goal-achieved 未挂到消息行 · 词级 intra-line diff 高亮。
(详见各 finder 原始报告;本 ledger 只登记,择机由循环消化。)

---

## 本轮并发实现(5 个 implementer,touches 白名单**两两无交集**)

- **A** — new-task 空状态(P1#1):`Home.tsx` + 新 `styles.home.css`
- **B** — diff 保真(P1#2,#8,P2#18):`DiffView.tsx` + `diffSummary.ts` + `styles.rs.css`
- **C** — 命令面板(P2#11):`CommandPalette.tsx`
- **D** — Scheduled 建议+筛选(P2#12):`Scheduled.tsx` + 新 `styles.scheduled.css`
- **E** — 变更/artifact 卡(P2#9,#10):`ChangesOutcome.tsx`

**本轮不做(留给循环,多为后端或共享文件):** push/revert 端点(#4,#5)、
unmodified 折叠带后端(#3)、新一级目的地(#7)、composer/model/add-menu(#13-15,
同一 `Composer.tsx` 只能串行)、thread panel 后端接线(#16)、diff header 工具栏(#17)。
