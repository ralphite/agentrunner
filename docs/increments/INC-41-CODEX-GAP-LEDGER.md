# INC-41 Codex Parity Gap Ledger — 视觉/交互保真(2026-07-11 深比对)

**范围硬规则:** 只登记**已有后端功能**的 UI/UX 差距。新功能/需新后端集成的
(GitHub PR、插件注册表、站点托管)= out-of-scope,不在此表(已移除 Plugins/Sites/
Pull requests/Chat 壳页)。方法:24 张 Codex 金标图 × playwright live 截图,14 个并发
视觉保真 finder(spacing/typography/层级/状态/hover/密度/对齐/图标/空态/文案)。

状态:☐ 待做 · ▶ 本轮并发实现 · ✅ 已关闭

---

## 🌐 系统级(改一处提升多屏——最高杠杆)

| # | 差距 | 值 | 源 | 状态 |
|---|---|---|---|---|
| S1 | **选中态到处用蓝,Codex 是中性灰**(nav active、当前任务行、cmdk 选中、diff toggle) | blue-soft/accent → `--panel-2` 灰 + `--ink` 文字 | styles.css, styles.nav.css | ▶ G/Sidebar |
| S2 | **主文字偏灰**,对比不足 | `--ink` `#1f1f1f` → `#0d0d0d`(仅 light) | styles.css:9 | ▶ G |
| S3 | **两个蓝并存**碎片化 accent | `--blue` `#2f6bff` → 统一到 `#0169cc` | styles.css:16-17 | ▶ G |
| S4 | **sidebar 太宽** | 288px → ~248px | styles.css:125 | ▶ G |
| S5 | **整体偏小偏密**:字号/行高/间距全线小于 Codex(sidebar 13→15、cmdk、scheduled、change card、model 菜单) | 上调 type scale + 行高 | 多处 | ▶ G/各 |
| S6 | **卡片圆角不齐**(home 16 / task 12 / diff 10) | 统一 `--radius-card:12px` | styles.css, styles.home.css | ▶ G/Home |

## 🧹 去噪(Codex 没有的多余 chrome)

| # | 差距 | 源 | 状态 |
|---|---|---|---|
| N1 | composer chip 每个都挂 caret(4 个多余 ⌄) | Composer.tsx | ▶ Composer |
| N2 | sidebar 任务行**每行时间戳**挤压标题(Codex 无) | Sidebar.tsx | ▶ Sidebar |
| N3 | sidebar 每行**状态点** + repo header 前**caret**(Codex 无) | Sidebar.tsx | ▶ Sidebar |
| N4 | 助手消息包在**带边框气泡** + robot 头像(Codex 是纯正文) | Timeline.tsx / styles.css | ▶ Thread/G |
| N5 | "Worked" 折叠行画了**整条分隔线**(Codex 无) | styles.css `.worked-row` | ▶ G |
| N6 | env 面板**段头带图标 + 段间 hairline + 全大写**(Codex 纯文字句首大写、留白分段) | SupervisionPanel.tsx, styles.panel.css | ▶ Panel |
| N7 | cmdk 组头**全大写微标** + kbd 带边框 + 选中蓝 | CommandPalette.tsx, styles.css | ▶ G/CmdK |
| N8 | change card 每行**前导 FileCode 图标**、Undo **红字带框**、artifact 分离成独立卡 | ChangesOutcome.tsx | ▶ CmdK+Change |
| N9 | "Show N more" 带数字(Codex 只 "Show more") | Sidebar.tsx | ▶ Sidebar |
| N10 | diff toolbar **换行成两排**、按钮全带框/实心 primary(Codex 单排 ghost/icon-only) | DiffView.tsx, styles.rs.css | ▶ Diff |
| N11 | scheduled **page-eyebrow**、filter 带框段控 + 数字、markread 带框、每行 ArrowUpRight | Scheduled.tsx, styles.scheduled.css | ▶ Scheduled |
| N12 | approval kicker 全大写 + 大号填充图标 tile | ApprovalCard.tsx, styles.panel.css | ▶ Panel |

## 🎨 图标/色彩保真

| # | 差距 | 源 | 状态 |
|---|---|---|---|
| I1 | Scheduled 图标 calendar → **clock** | Sidebar.tsx | ▶ Sidebar |
| I2 | composer environment 图标 `</>` → **gear** | Composer.tsx | ▶ Composer |
| I3 | home Explore **binoculars → telescope**;Explore 蓝应为 **teal**(与选中蓝撞) | Home.tsx, styles.home.css | ▶ Home |
| I4 | diff 改动行**整行文字染绿/红**(Codex 只染 gutter+行号,正文近黑) | styles.css:1584-1591 | ▶ G |
| I5 | diff 行号不随改动染色;inline code 橙应为 **teal** | styles.rs.css, diffSummary.ts | ▶ Diff |
| I6 | model 菜单 Advanced chevron 方向反了;drill chevron 偏小;label 偏轻 | Composer.tsx, styles.composer.css | ▶ Composer |
| I7 | change card 头图标 Files → **±/GitDiff**;计数应恒显 `+N -0` | ChangesOutcome.tsx | ▶ CmdK+Change |

## 📐 密度/排版(逐组件,详见各 finder 原始报告)

home 卡片 `space-between` + padding 20 + min-h 150、headline 30→24/500、hero icon 透明放大;
composer chip 字号 12.5→14 + 色 ink-2→ink + gap 4→10、送出键 disabled 对比、model pill 去前导图标;
thread 消息动作行 icon-only + 👍/👎/share + "Goal achieved" 内联、标题去 FileText 图标;
env-row label 色 ink-2→ink、branch 行补 label;scheduled 搜索 pill 化 + 两排布局 + 状态用 outlined ring 图标 + 标题/副标题上调、Suggestions 标题/cadence 上调 + 第3图标 file-search;
add 菜单 title+desc **同行同号**、section 头放大、系统项图标放大。

## ⛔ out-of-scope(需新后端,移出 ledger)

GitHub PR 集成 · 插件/MCP 注册表 · 站点托管/发布 · 独立 Chat 后端 · Scheduled 的
cadence/next-run(需后端 next-run 计算)· model "Speed" 延迟档(provider 无此字段)。

---

## 本轮并发实现(9 个 implementer,touches 白名单两两无交集)

- **G** 系统 styles.css:S1-S6 的 styles.css 部分 + N5/N7 css + I4 + cmdk/worked/bubble/项目树排版
- **Sidebar** `Sidebar.tsx`+`styles.nav.css`:N2/N3/N9/I1 + nav 选中中性
- **Composer** `Composer.tsx`+`styles.composer.css`+`specs.ts`:N1/I2/I6 + chip 排版 + pill
- **Home** `Home.tsx`+`styles.home.css`:I3 + 卡片/headline/hero 排版
- **Diff** `DiffView.tsx`+`diffSummary.ts`+`styles.rs.css`:N10/I5 + 折叠带 + 状态字形色
- **Scheduled** `Scheduled.tsx`+`styles.scheduled.css`:N11 + 搜索/pill/行/建议排版
- **Panel** `SupervisionPanel.tsx`+`styles.panel.css`+`ApprovalCard.tsx`:N6/N12 + env-row
- **CmdK+Change** `CommandPalette.tsx`+`ChangesOutcome.tsx`:N7/N8/I7 tsx 部分
- **Thread** `Timeline.tsx`+`SessionView.tsx`:N4 tsx + 消息动作行 + 标题图标
