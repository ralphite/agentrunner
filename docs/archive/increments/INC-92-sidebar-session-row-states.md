# INC-92 Sidebar session row 状态与动作

> 状态：✅ 已完成（2026-07-22，QA-83 PASS）。

## 动机与 journey 锚

修订 **UJ-24 Web UI 驾驶 AgentRunner** 的左栏 session item：现有每行常驻/
hover `…` 把“状态”与“管理入口”混在一起，也重复了 session title 已有的菜单。
目标是按参考图把行尾分成两个互斥层：

1. resting state 只投影事实：managed worktree 标记、running spinner、未读或异常状态；
2. desktop hover / keyboard focus 显示 Pin/Unpin 与 Archive/Unarchive 快捷动作，
   不改变行高、标题宽度或当前选中背景；running spinner 继续可见；hover/focus
   高亮覆盖整行（标题与所有尾随 icon），范围与 current selection 完全一致；
3. session row 不再显示 `…`；pointer 右键、`Shift+F10` / ContextMenu key 复用
   现有 Pin / Rename / read state / Archive 菜单；
4. mobile 不依赖 hover：点入 session 后从现有 title `…` 管理；context-menu capable
   设备仍可右键。项目 heading 的 `…` 与 New chat 不在本增量范围内；
5. project heading 不再常驻显示重名消歧副标题。不同 workspace 可显示相同 project
   名，完整路径只在现有 hover preview 与原生 tooltip 中披露。

**UI/UX 裁决**：复用 `Sidebar` 现有 32px row、Phosphor icon、current/hover token、
`ContextMenu` 与 session title menu；不新增弹窗、route、持久数据或 backend API。
Pin/Archive 仍调用既有 browser-local projection actions，不删除 journal/workspace。

## Spec delta

### JOURNEYS.md

UJ-24 step 1 将“session row 只露一个 `…` 管理菜单”改为：resting state 显示
worktree/live/attention 事实；hover/focus 显示 Pin/Archive；完整管理菜单由右键/
keyboard context menu 与 session title `…` 承担；project heading 只显示 project 名，
不为重名追加常驻 path hint。

### SPEC.md

Web UI 产品面条目改记 session row state/action disclosure、无行内 `…`、右键/
keyboard menu 等价，以及 duplicate project label 不显示 subtitle、hover 显示全路径；
验收锚为 `Sidebar.nav.test.tsx` 与 QA-83。

### DESIGN.md

§12 默认 surface 收敛修订 session row 投影规则：状态 icon 属于低噪事实，
hover/focus quick actions 仅 Pin/Archive；完整菜单不在 row 常驻，仍由 context/title
入口承载。managed worktree 标记仅由 AgentRunner shared worktree root 路径识别，
不对任意同名目录猜测。project grouping 继续以真实 workspace 为键，但 label 重名不
再增加常驻 subtitle；完整 workspace 只在 hover/tooltip 展示。

**不变量**：不触及。session/journal/workspace、pin/archive 语义、routing、权限与
durability contract 均不变，只调整已有前端 projection。

## 验收

### A 闸

`webui/frontend/src/components/Sidebar.nav.test.tsx`：

- managed worktree resting marker；普通 workspace 无 marker；
- running 显示可访问 spinner，hover actions 不吞掉 live status；
- row 无 `More actions` / `DotsThree` trigger；
- hover/focus quick actions 的 Pin↔Unpin、Archive↔Unarchive state/action；
- right-click 与 `Shift+F10` 打开既有 Pin / Rename / read state / Archive menu；
- current + hover class/DOM 结构保持稳定；mobile row 不恢复重复 menu。
- hover/focus 与 current 共用 `.project-session-wrap` 的全行背景，不出现只包标题的
  短高亮。
- duplicate project label 的 headings 可同名且无 `.project-hint`，两者 tooltip/hover
  preview 分别显示自己的完整 workspace。

全量：frontend vitest + production build + webui Go tests + `./scripts/check.sh`。

### B 闸：QA-83，共享真实环境

在真实 `http://127.0.0.1:8809/`、共享 `~/.local/share/agentrunner/`：

1. 用既有 managed-worktree session 验证 resting worktree icon；用 running session
   或可控 fixture 验证 spinner，不为截图启动新模型任务；
2. 真实 item hover/focus 显示 Pin/Archive 且无 row `…`；点击 Pin 后 icon/label
   切成 Unpin，再恢复原状态；不执行 Archive 以避免改变共享历史；
3. item 右键与 `Shift+F10` 打开完整 menu，选择之外不触发 navigation；
4. 找到两个同名 project，确认 heading 无副标题且各自 hover 显示正确全路径；
5. current/normal/hover、reload/restart 后均不 blank，console relevant error/warn=0；
6. 截图与参考图在同一 desktop viewport 做 design QA；证据留在
   `qa/runs/2026-07-22-QA83-sidebar-session-row-states/`，不清理共享数据。

running fixture 若真实共享历史当前没有 live session，则 component test 是 spinner
行为锚，B 闸明确记录该动态态未用新模型任务复现。

## 实施步骤

1. **INC-92.1**：`Sidebar`/CSS/action tests + 三层/QA/LOG 收口；全门通过；真实
   browser + visual QA；工作纸归档；commit 并 push `origin/main`。

## review 裁决

裁掉三视角对抗 review：局部 projection 改动，不改 backend/schema/权限/并发/持久
语义。保留 UI/UX pre-review、完整 component regression、真实浏览器 QA 与截图
design-QA gate；P0/P1/P2 视觉或交互问题修完才收口。
