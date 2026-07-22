# INC-90 选中 session 时 project 仍可折叠

## 动机与 journey 锚

用户在 **UJ-24 Web UI 驾驶 AgentRunner** 的 Projects→sessions 导航中发现：
当前选中 session 属于某 project 时，点击该 project heading 无法 collapse。

真实 `http://127.0.0.1:8809/` 已复现：`mt-test` 的 session
`20260721-221631-say-hi-in-one-word-a4dd080497611f5d` 被选中时，点击
project 后 `aria-expanded` 立即仍为 `true`，session row 仍在。

根因是 SB-1 旧裁决把 active session 当成 rail anchor，并在两层覆盖用户
fold：`Sidebar` 以 `persistedFold && !holdsCurrent` 强制展开，
`visibleProjectSessions` 又让 current session 穿透 folded 状态。这导致已有
collapse affordance 对当前 project 成为无效按钮，违反用户对 disclosure
控件的基本预期。

### UI/UX pre-implementation review

- **沿用模式**：继续使用现有 project heading/caret、server overlay 与
  localStorage mirror，不新增控件、文案或确认。
- **正确交互**：active session 只决定选中态、初次定位与 project group 在
  section cap 之外时仍保留 heading；用户点击 collapse 后，session rows 全部
  隐藏，选中 session 和中央内容不变，再次展开时恢复原高亮行。
- **风险态**：deep link/⌘K 进入超出首 8 个 project 的 session 时，project
  heading 仍强制进入 rail，但 persisted fold 可以隐藏其 session rows；用户可随时
  从 heading 重新展开。
- **数据/内容**：只更新现有 `folded` 偏好，不改 session/project 归属，
  不删 journal/workspace，不改中央会话。
- **未决问题**：无；用户已明确否定“active session 锁定 project expanded”。

## Spec delta

修订 `SPEC.md` 的 Web UI product surface：project fold 是用户偏好，不被
active session 覆盖；active group 超出 section cap 时只保证 heading 可见。

验收锚：`Sidebar.nav.test.tsx`、`viewModels.test.ts`、QA-81 共享真实产品面。

## Design delta

修订 `DESIGN.md` §Web UI product surface 的 rail 折叠规则：selection 可以把
group heading 带入有限 project 列表，但不得让 collapse control 失效；folded
始终隐藏该 group 的 session rows。

**是否触及不变量：否。** 不改 journal-first、project grouping、deep-link、
session selection 或 persisted overlay contract；只纠正前端 disclosure 优先级。

## 验收

### A 闸（component / pure model）

1. selected session 所在 project 点击 heading 后立即 collapse，写入
   localStorage 并调用 `toggleProjectFolded(key, true)`。
2. active session 不再穿透 folded group；重新展开后仍高亮原 session。
3. active project 超出首 8 个 group 时，heading 仍渲染，但 fold 时无
   session row。
4. 非 active group、show more/less、project section fold 和 mobile drawer 不回归。

### B 闸（QA-81，共享真实环境）

1. 在 `:8809` 选中现有 `mt-test` session，点 project heading；立即
   `aria-expanded=false`，session rows 消失但 hash/central thread 不变。
2. 等待至少一次 4s refresh 后仍 folded；reload 后仍 folded。
3. 再点 heading 展开，原 session row 恢复 current highlight；console 0 error/warn。
4. 截图/DOM/console 证据入
   `qa/runs/2026-07-21-QA81-selected-session-project-fold/`，共享数据不清理。

## 实施步骤

1. 工作纸与产品裁决入档。
2. 移除 active-session-overrides-fold 两层逻辑，更新注释与 tests。
3. A/B 双闸、三层/QA/LOG 收口、工作纸归档并更新本机服务。

## review 裁决

做一次契约视角收口 review：证明 fold 优先后 deep-link/current highlight/
section cap 仍可用。改动仅前端布尔条件与回归断言，无并发、权限、
数据迁移或不可逆面，裁掉里程碑级三视角 review。
