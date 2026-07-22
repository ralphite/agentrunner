# Design QA — INC-19 Web UI

## Source of truth

- 用户确认的母版：
  `/Users/yadong/.codex/generated_images/019f4935-7896-7c13-ab9e-a40abefdb07c/exec-1a047500-51f2-4330-8ae6-7eb8cb676e44.png`
- 视觉规则：Codex 通用 UI/UX；AgentRunner 品牌；AgentRunner 独有能力只在
  Codex 视觉语言上的 Supervision 扩展。

## Final implementation evidence

- 全屏 1554×1014：
  `qa/runs/2026-07-09-QA27/approval.png`
- 同图对照（source 左 / implementation 右）：
  `qa/runs/2026-07-09-QA27/design-compare-full.png`
- 主工作区 focused 对照：
  `qa/runs/2026-07-09-QA27/design-compare-focus.png`
- 状态截图：`home.png`、`supervision.png`、`changes.png`
- 响应式：`responsive-900.png`、`responsive-700.png`

## Comparison history

1. Full-view pass：发现 P1——真实 team session 的 revive/重复回执让
   Supervision 把同一 engineer/reviewer 各画两次。按 child session id 去重，
   保留最新 inspect 状态；行改为语义化 button。单测与真实 DOM 均验证每个
   成员恰一行。
2. Full-view recheck：三栏比例、安静 header、单一 thread、内联 approval、
   底部 composer 与右侧 Supervision 的层级和母版一致。0 P0/P1/P2。
3. Focused pass：approval 先显示动作/对象/scope，raw args/gates 折叠；
   Approve once 为主操作、Deny 分立；Attention 与 card 使用同一 pending
   真相。0 P0/P1/P2。
4. Responsive pass：900/700 下 Supervision 变可关闭浮层，thread/composer
   不被拆成另一套 IA。0 P0/P1/P2。

## Final verdict

PASS。实现忠于已确认的 Codex 母版，同时只把 AgentRunner 的 Goal / Agents /
Attention / Background work 作为原生 Supervision 扩展；无阻塞交付的视觉、
交互或信息架构偏差。

---

## INC-23 B3–B6 黑盒复核

- 母版不变；最终同尺寸 1554×1012 对照：
  `qa/runs/2026-07-10-QA34/29-reference-vs-latest.png`。
- 首轮证据暴露 7 个结构性 P1：窄窗遮挡、recovery 无入口且 Attention 撒谎、
  Scheduled 重启丢失、task 行不可键盘进入、非人类 input 冒充用户、raw
  launcher 主层泄漏、移动端 sidebar 覆盖内容。均已修复并用真实 session
  重走。
- 最终对照：sidebar/thread/approval/composer/Supervision 的比例、层级、边框、
  圆角、密度与 Codex 母版一致；AgentRunner 品牌与独有 supervision 数据不
  另造设计语言。审批 workspace 只在主层显示可辨识名称，完整临时路径不再
  抢占决策层。P0/P1/P2=0，PASS。

---

## INC-29 UX Round 3 黑盒复核

- 结构化 Run details：默认只呈现 status/waiting/overview/usage/activity/
  provider，raw inspect 收进折叠 advanced；approval 的 CLI answer command
  不进入决策层。
- QA-fix 发现并修复三处真相偏差：revive child 在详情重复计数、普通
  waiting:input 被误报为 Attention、stranded 被 stale inspect 显示 Running。
- 同前缀命令任务把真正差异前置；状态色在 sidebar/pill/Subagents 与
  light/dark 统一。最终 1554×1012 同图：
  `qa/runs/2026-07-10-QA36/07-reference-vs-latest.png`。

结论：AgentRunner 品牌、Codex 通用结构与 supervision 扩展仍使用同一视觉
语言；P0/P1/P2=0，PASS。

---

# INC-92 Design QA

## Evidence

- source visual truth paths:
  - `/var/folders/pv/h1nh3j1n7k94z_2nvdcz4rfc0000gn/T/codex-clipboard-d9f73125-f4d8-40fe-8b3b-95303d246d5e.png`
  - `/var/folders/pv/h1nh3j1n7k94z_2nvdcz4rfc0000gn/T/codex-clipboard-7e16d7f8-fb72-4e3c-aeff-8b81d769c7a5.png`
  - `/var/folders/pv/h1nh3j1n7k94z_2nvdcz4rfc0000gn/T/codex-clipboard-78da0713-a806-4250-99ac-dd17105ab16e.png`
- implementation screenshot paths:
  - `qa/runs/2026-07-22-QA83-sidebar-session-row-states/01-focus-full-row.png`
  - `qa/runs/2026-07-22-QA83-sidebar-session-row-states/03-selected-reload.png`
  - `qa/runs/2026-07-22-QA83-sidebar-session-row-states/06-worktree-resting.png`
- viewport: `1280 x 720` CSS px, desktop dark theme, `deviceScaleFactor=2`
- source pixels: `1284 x 448`, `776 x 581`, `1142 x 402`; implementation pixels:
  `2560 x 1440`, normalized to the focused comparison width/height before comparison
- density normalization: browser captures were downsampled from `@2x`; each source and
  implementation focus crop was normalized to equal pixel dimensions before side-by-side review
- state: managed-worktree resting, keyboard focus/quick actions, context menu, current after reload
- full-view comparison evidence: `qa/runs/2026-07-22-QA83-sidebar-session-row-states/01-focus-full-row.png`
- focused region comparison evidence:
  - `qa/runs/2026-07-22-QA83-sidebar-session-row-states/04-comparison-states.png`
  - `qa/runs/2026-07-22-QA83-sidebar-session-row-states/05-comparison-highlight.png`

## Findings

- fonts/typography: product-native typography, row hierarchy, weight and truncation remain
  consistent; no actionable mismatch.
- spacing/layout rhythm: 32px row geometry and trailing icon alignment are stable; hover/current
  background covers the complete title-and-icon row; no actionable mismatch.
- colors/tokens: hover/focus/current use the same existing `bg-panel-2`; source light/dark variation
  is an expected theme difference, not a fidelity defect.
- image quality: no raster product assets were introduced; all state/action icons use the existing
  Phosphor vector icon library at native size, with no placeholder or handcrafted SVG.
- copy/content: row ellipsis and duplicate path subtitles are absent; Pin/Archive and the complete
  context-menu wording match the existing product actions.
- behavior/accessibility: focus exposes quick actions; right-click and `Shift+F10` expose the same
  full menu; current state survives reload. Browser console warning/error count is zero.
- residual test gap: the shared store had no running session, so the live spinner state was not
  visually captured; component tests cover spinner rendering and coexistence with hover actions.
- P0/P1/P2: none. P3: none required for acceptance.

## Comparison history

- Iteration 1: reference showed row background stopping before trailing icons and duplicated menu /
  path surfaces in the earlier implementation. The row background moved to the outer wrapper,
  session row ellipsis and project path subtitle were removed, and status/quick-action layers were
  added. Post-fix evidence: `04-comparison-states.png` and `05-comparison-highlight.png`.
- Post-fix review found no actionable P0/P1/P2 differences. The blue outline in the focus capture is
  the expected keyboard focus ring; pointer hover shares the same background selector without it.

final result: passed
