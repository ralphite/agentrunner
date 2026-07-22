# INC-95 Web UI QA 契约与可访问性收口

> 状态：✅ 2026-07-22 完成。独立 CI 复核、A 闸与共享真实环境 QA-86 全部通过。

## 动机与 journey 锚

2026-07-22 对真实 `http://127.0.0.1:8809/` 做全量产品 QA 后，由独立 CI agent
只读复核出三项真实缺陷与一项验收契约误读，锚定 UJ-24 第 1–3、6 步：

1. 390×844 的 Changes overlay 覆盖 `Show sidebar`，但底层按钮仍可聚焦，点击命中
   `Change diff scope`；
2. sidebar session/project context menu 用 `Escape` 关闭后焦点落到 `body`，没有回到
   keyboard opener；
3. QA-60、QA-69 与当前已有明确 defending tests 的 Commit/Add 产品行为冲突；
4. QA-76 把可续聊的 `waiting:input` parent 误称“终态”，从而把模型最后一次
   `progress_update` 的陈旧 running 行误判成 runtime 状态 bug。

### UI/UX design note

- **沿用模式**：mobile Changes 继续是有自己关闭按钮的全屏 overlay；context menu 继续
  首项自动获焦并复用现有 `ContextMenu`；Add menu 继续采用 `Add / Advanced →
  Automation` 的渐进披露；Last Turn 继续使用现有 resident `Commit or push`。
- **提案**：Changes 打开时只在 `≤900px` 隐藏底层 `Show sidebar`，关闭后自然恢复；
  ContextMenu 只在 `Escape` dismissal 后恢复 mount 时的 opener，item click/outside/scroll
  dismissal 不抢后续动作焦点；活文档与 QA 脚本对齐现有产品行为。
- **风险态**：Changes 的 `✕` 始终可达，不制造无法退出的 overlay；focus return 只针对
  仍连接 DOM 的 opener；不把 `waiting:input` 的模型 checklist 私自改写成 done。
- **数据处理**：无 schema、API、journal、localStorage 或共享 store 迁移；不删除/关闭/
  清理任何 QA session、workspace 或 journal。
- **未决问题**：无。独立 CI 已逐项复现并裁决；progress 的模型质量提升若需要，应另起
  prompt 增量，不混入 truthful projection 修复。

## Spec delta

- `JOURNEYS.md` UJ-24：补 mobile Changes 独占 overlay 与 context-menu Escape focus return；
  把 composer IA 写准为 Goal/Plan root，Loop/Best-of-N/agent spec 在 Automation 子页。
- `SPEC.md`：Web UI product surface / composer / turn closure / interaction rows对齐上述行为；
  progress 行澄清 `waiting:input` 非终态，只有 settled child projection 会把未完成项标 failed。
- `DESIGN.md` §12：写明 mobile overlay 的底层控件抑制、context-menu focus-return 和
  Add/Automation 层级；不改变 backend/thin-shell 语义。

**不变量**：不触及。没有修改 journal-first、session liveness、diff scope、权限、持久化或
共享数据边界；只收紧前端 projection 与文档契约。

## 验收

### A 闸

- `App.mobile.test.ts`：CSS contract 钉住 mobile Changes 存在时 `Show sidebar` 不渲染。
- `Sidebar.nav.test.tsx`：session 与 project 的 keyboard context menu 在 Escape 后分别恢复
  原 row focus；菜单内容与动作不变。
- `Composer.addMenu.test.tsx` 保持 exact 四个 root 与 Automation 子页；
  `DiffView.commit.test.tsx` 保持 Last Turn resident Commit。
- `qa69-assert.mjs` 从陈旧“≥5 root”改为 exact root + Automation 子页真实浏览器断言。
- frontend targeted/full vitest + production build + webui Go tests + `./scripts/check.sh`。

### B 闸：QA-86，共享真实环境

在 production `:8809`、共享 `~/.local/share/agentrunner/`：

1. 390×844 打开既有真实 diff session 的 Changes，断言 `Show sidebar` 不可见/不可聚焦、
   scope trigger 可点击；关闭 Changes 后 sidebar trigger 恢复且可打开 drawer；
2. session 与 project row 分别以 keyboard context menu 打开，Escape 后焦点回 opener；
3. Last Turn resident Commit、Add exact 四 root 与 Automation 子页均可达；
4. 既有 stale-progress session 继续如实显示模型最后声明，不改 journal；
5. deep-link reload、health/versionMatch、console warning/error 均通过；证据保留
   `qa/runs/2026-07-22-QA86-webui-qa-corrections/`。

## 实施步骤

1. **INC-95.1**：CSS/focus 修复、component/QA script regression、三层与活 QA 契约收口；
   全量 gate、真实浏览器 QA；工作纸归档；commit 并 push `origin/main`。

## review 裁决

裁掉里程碑级三视角 review：改动限于前端 overlay/focus projection、测试与文档，无
backend/schema/并发/权限/破坏性数据面。保留独立 CI 事前复核、UI/UX pre-review、全量
component/build/check 与真实 shared-store browser gate。

## 完成记录

- mobile Changes 独占 surface，底层 `Show sidebar` 不可见且不进入 hit/focus surface；
- session/project context menu 的 Escape focus return 已由 component test 与真浏览器双锚；
- QA-60/69/76 与三层产品文档已对齐现有 Commit/Add/progress 事实；
- frontend 全量 65 files / 674 tests、production build、WebUI Go tests、
  `./scripts/check.sh` 全绿；
- QA-86 在 `:8809` + shared store PASS，证据保留于
  `qa/runs/2026-07-22-QA86-webui-qa-corrections/`，未清理任何真实数据。
