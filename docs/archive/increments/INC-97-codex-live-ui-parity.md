# INC-97 Codex 实窗对标与 Environment 浮层修正

> 状态：✅ 2026-07-22 完成。QA-87 通过；frontend 681/681、production build、
> WebUI Go tests、`./scripts/check.sh` 全绿；共享测试数据与证据全部保留。

## 动机与 journey 锚

2026-07-22 用 macOS 系统窗口捕获直接取得当前 Codex 桌面 app，并通过
`Cmd+K` / `Escape` 实际操作命令面板；同时在共享 `:8809`、共享 store 的既有
`QA-FULL-20260722` session 上按相同 1840×1353 viewport 捕获 AgentRunner。
本轮对比锚定 UJ-24 第 3、5、6 步：

1. Codex 的 `Environment` 是右上浮动卡，开关时 thread 与 composer 不改宽、不横移；
   AgentRunner 的 JSX 与 component test 也声明了相同契约，但 desktop CSS 仍给
   `.session-layout.environment` 分配 300–360px grid track，真实页面会挤压 thread。
2. `SessionView.chrome.test.tsx` 只断言 `single` class，没有钉 CSS geometry，导致
   “测试说浮层、产品仍分栏”的漂移长期漏过。
3. 现有 `qa/codex-reference/README.md` 声称自动流程拿不到 Codex app；本轮已验证在
   有 Screen Recording / Accessibility 权限的 macOS 交互 session 中，可用窗口 PID +
   CGWindow id 精确截图，并可做低风险快捷键交互。应固化为可重复 QA 工具。

### UI/UX design note

- **沿用模式**：复用现有 `Environment` topbar pill、右上 close、圆角/边框/阴影与
  `≤900px` overlay；`Changes` 继续使用真实双栏/移动端独占 overlay。
- **提案**：所有 viewport 的 `Environment` 都采用 `position:absolute` 右上浮动卡，
  `session-layout` 始终单列；卡片按内容自然高度，超长时在 viewport 内独立滚动。
- **风险态**：浮层可能覆盖右侧内容，但不改变阅读位置；close 始终可达，最大高度受
  viewport 约束。`Changes` 的 z-index 高于 Environment，二者仍不同时显示。
- **数据处理**：无 API/schema/journal/localStorage/store 迁移；QA 只读既有 session，
  不 Send/Archive/Remove/Commit，不清理任何共享数据。
- **未决问题**：无。浮层契约已存在于源码注释、component test 与 Codex 当前实窗；
  本增量只让 desktop CSS 和既有产品决定重新一致。

## Spec delta

- `JOURNEYS.md` UJ-24 第 5 步：写明 Environment 是不重排 thread 的浮动卡，
  `Changes` 才使用分栏审阅。
- `SPEC.md` Web UI interaction row：补 Environment 浮层 geometry 与 Changes 分栏边界。
- `DESIGN.md` §12 Web UI product surface：把“Supervision 叠加层”收紧为全 viewport
  absolute card、内容高度/滚动/层级契约；不改变 thin projection 或状态语义。
- `QA.md` 新增 QA-87：Codex 实窗 × AgentRunner 同尺寸对比与共享真实环境回归。
- `qa/codex-reference/README.md`：登记可重复 macOS 捕获/交互办法，替代“只能带外手工捕获”
  的过时绝对表述。

**不变量**：不触及。改动只有前端布局投影与 QA 工具；journal-first、session liveness、
diff scope、权限、持久化和共享数据边界均不变。

## 验收

### A 闸

- 新增 CSS contract test：desktop `.session-layout.environment` 不再产生第二 track；
  `.supervision-panel.session-side` 在 base rule 即为 absolute/right-top/viewport-bounded/
  independently-scrollable card；mobile 不另维护一份漂移规则。
- 保持 `SessionView.chrome.test.tsx`：Environment 开关时 `single` class 不变，Changes
  仍切到 `changes`。
- `bash -n qa/capture-codex-ui.sh`，并在当前 Codex app 实跑 current + command-palette
  两态，截图保存后逐张检查。
- frontend targeted/full vitest + production build + webui Go tests + `./scripts/check.sh`。

### B 闸：QA-87，共享真实环境

在 production `http://127.0.0.1:8809/`、共享
`~/.local/share/agentrunner/`、既有 `QA-FULL-20260722` session：

1. 1840×1353 light 打开/关闭 Environment，断言 `session-primary` 的 x/width 不变；
   panel 为右上浮动卡、全部 rect 在 viewport、close 可达；
2. Environment → Changes，断言浮卡消失、Changes 获得真实 split column；返回后可重开；
3. 1280×720 与 390×844 复测 overlay/close/无横向 overflow；
4. deep-link reload 与 Web UI restart 后同一 session/偏好/共享历史仍可用；
5. health `daemonUp/versionMatch=true`，browser warning/error 为空；
6. Codex 与 AgentRunner 同尺寸截图、DOM geometry、health/events/workspace diff 证据保留到
   `qa/runs/2026-07-22-QA87-codex-live-ui-parity/`，不关闭或删除 session。

## 实施步骤

1. **INC-97.1**：固化 Codex 窗口捕获工具；修正 Environment base CSS 与 contract test；
   完成三层/QA/LOG 收口、全量 gate 与共享真实环境 QA；工作纸归档；commit 并 push
   `origin/main`。

## review 裁决

裁掉里程碑级三视角 review：改动限于已由源码注释与现有测试裁决的 CSS 漂移、只读
macOS QA 工具和文档，无 backend/schema/并发/权限/破坏性数据面。保留 UI/UX pre-review、
component/build/check 与真实 shared-store browser gate。

## 完成记录

- Codex current + command-palette 两态已由真实 `com.openai.codex` 窗口捕获并验图；
  可重复工具为 `qa/capture-codex-ui.sh`。
- AgentRunner 1840×1353、1280×720、390×844 的 Environment geometry、desktop
  Working Tree diff 与 mobile Changes overlay 均通过；browser warning/error=`[]`。
- 新建并保留共享只读 QA session
  `20260722-181007-qa-87-ui-7a9174ac616dfb29`；未清理 session/workspace/journal。
- 证据：`qa/runs/2026-07-22-QA87-codex-live-ui-parity/`。
