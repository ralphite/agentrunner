# INC-93 Sidebar project row 整行高亮

> 状态：✅ 已完成（2026-07-22，QA-84 PASS）。

## 动机与 journey 锚

修订 **UJ-24 Web UI 驾驶 AgentRunner** 的 Projects rail：project heading hover / keyboard
focus 当前只给左侧 heading button 着色，右侧 `…` 与 New chat icons 浮在高亮之外；与
INC-92 已收口的 session row 整行状态不一致。目标是让 `.project-heading-row` 统一承担
hover/focus background，覆盖 project name、menu 与 New chat icons。

**UI/UX design note**：复用 INC-92 的 outer-wrapper highlight pattern、既有
`bg-panel-2` token、8px radius、Phosphor icons 与 action placement；不新增控件、文案、
状态或确认流程。menu / New chat 的动作与 browser-local project preference 均不变，
没有数据丢失或破坏性新路径，也没有未决产品问题。

## Spec delta

- `JOURNEYS.md`：UJ-24 step 1 补记 project hover/focus 背景覆盖 heading 与 actions。
- `SPEC.md`：Web UI 产品面补记 project row complete-wrapper highlight；锚
  `Sidebar.nav.test.tsx`、`buttonPress.test.js` 与 QA-84。
- `DESIGN.md`：§12 project controls 的 hover/focus background 与 session row 一样由
  outer row wrapper 承担，不让 icons 落在 highlight 外。
- **不变量**：不触及。仅调整 CSS projection，不改 backend、routing、project overlay
  或持久化语义。

## 验收

### A 闸

- CSS regression：`.project-heading-row:hover/:focus-within` 使用 `bg-panel-2`，
  `.project-heading` 不再单独使用 hover background。
- component regression：heading row 仍包含 menu / New chat，菜单六项与 New chat action
  不变；pointer hover 与 keyboard focus DOM geometry 不变。
- frontend targeted + full vitest + production build + `./scripts/check.sh`。

### B 闸：QA-84，共享真实环境

在 production `http://127.0.0.1:8809/`、共享 `~/.local/share/agentrunner/` 验证：

1. project heading focus/hover 的背景覆盖名称与两枚 actions；
2. `…` 六项菜单与 New chat control 仍可达，不执行会改变共享数据的动作；
3. 打开既有 session 后 reload，project/session rail 与 thread 不 blank；
4. console relevant warning/error=0；截图保留在
   `qa/runs/2026-07-22-QA84-sidebar-project-row-highlight/`。

## 实施步骤

1. **INC-93.1**：CSS + regression tests + 三层/QA/LOG 收口；真实浏览器 QA；部署、
   commit 并 push `origin/main`。

## review 裁决

裁掉三视角对抗 review：单一 CSS projection 修订，不改 schema/权限/并发/持久语义；
保留 UI/UX pre-review、component regression、真实浏览器 QA 与总闸门。
