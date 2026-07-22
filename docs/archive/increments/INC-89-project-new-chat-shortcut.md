# INC-89 Project new-chat shortcut

## 动机与 journey 锚

用户纠正 INC-87 的 project row 铅笔快捷键语义：它不是 Rename project，
而是“在这个 project 里新建 chat”；并要求所有 button 在按下时不改变尺寸。
修订 UJ-24 第 1 步；Rename 继续只住在 project `…` 菜单，避免同一动作重复
占两个高频入口。

### UI/UX pre-implementation review

- **沿用模式**：复用侧栏 `New session` → Home composer 的既有路径和同一
  project picker state；保留用户截图中的铅笔视觉。
- **交互**：快捷键 tooltip / accessible name 为 `New chat in <project>`；点击
  打开 Home、把 project workspace 设为 composer 当前选择并聚焦输入框。
- **数据**：不创建 session，直到用户自己发送第一条消息；不清空 draft、附件、
  access、model 或其它 composer 状态。project 选择继续写既有
  `arwebui.lastProject` preference。
- **pressed state**：删除全局 `button:active { scale-95 }`；按下反馈继续由既有
  background/border/focus 样式承担。所有 button 的 visual/layout size 均稳定。
- **风险/问题**：无破坏态，无未决产品问题；Rename 仍可从 `…` 菜单到达。

## Spec delta

修订 UJ-24 Web UI 产品面条目：project hover/focus 的铅笔快捷入口是
project-scoped New chat；Rename 仅在六项菜单内；button pressed state 不改变
尺寸。验收锚：`Sidebar.nav.test.tsx` + `Composer.projects.test.tsx` +
`buttonPress.test.js` + QA-79。

## Design delta

Home composer 接受一个 in-memory、带 request id 的 project seed。Sidebar
快捷键发出 seed 并切到 Home；Composer 在首次挂载或 request id 变化时复用
既有 project selection side effects（remember、branch discovery、headline、focus）。
不新建 API，不改变 session/journal 真相，不触 DESIGN 不变量。

## 验收

### A 闸

- Sidebar：快捷键名称/tooltip 正确；点击传入项目真实 workspace、进入 Home；
  Rename prompt 只从 menu item 打开。
- Composer：在已位于 Home 与从 session 新挂载两种情况下都消费 project seed，
  chip/headline 指向目标 project，输入焦点就绪；其它 composer state 不 remount。
- CSS：不存在全局或局部 button active scale；clicked button 尺寸保持不变。
- frontend vitest/build + `./scripts/check.sh` 全绿。

### B 闸（QA-79，共享真实环境）

在 `http://127.0.0.1:8809/` 的真实 `mt-test` project 点击铅笔：落 Home、
project chip/headline 为 `mt-test`、输入框聚焦；不发送消息、不创建 session；
`…` 菜单仍含 Rename project；pointerdown/click 前后按钮 bounding box 不变。
刷新后目标 project 仍为 last project；console error/warn=0。证据进
`qa/runs/2026-07-21-QA79-project-new-chat/`。

## 实施步骤

1. store/composer one-shot seed + Sidebar 接线 + frontend tests。
2. A/B 双闸；并回 JOURNEYS/SPEC/DESIGN/QA/LOG；工作纸归档；push main。

## 实施与验收结果

- store 新增带 request id 的 `newSessionForProject` intent；Home/Composer 原地消费
  seed，重复点同一 project 也会重新聚焦，draft 不丢。
- project row 铅笔改为 `New chat in <project>`，Rename 只在 `…` menu；全局
  `button:active scale-95` 删除，按下尺寸稳定。
- A 闸：针对性 49/49、完整 frontend vitest 656/656、production build、webui
  Go tests、全树 `./scripts/check.sh` 均绿。
- B 闸：QA-79 真实 `:8809` + shared store PASS；`mt-test` headline/chip/focus、
  reload persistence、Rename menu、24×24 bounding box、session count 605→605、
  console 0 error/warn 均对锚。
- GAPS 对账：用户纠正已有 UI 语义，无新架构缺口；无 GAPS delta。

## review 裁决

小型语义纠正，复用既有 New session 与 project picker，不增后端/数据/权限面，
裁掉里程碑级三视角 review；由 UI/UX pre-review、component tests 与真实浏览器
QA 覆盖。
