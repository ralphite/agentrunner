# INC-87 Sidebar project controls

## 动机与 journey 锚

用户在 **UJ-24 Web UI 产品面**的 Projects 层需要与 Codex 桌面端一致的项目
管理入口：project row hover 时显示摘要与快捷操作；`…` 菜单提供 Pin project、
Reveal in Finder、Create permanent worktree、Rename project、Archive chats、
Remove；侧栏可拖拽改宽；Pinned / Projects 两个 section 可整体 fold/unfold。

现状已有 session hover preview/action、project 右键/移动端菜单、project rename、
Finder launcher、Archive all sessions、project group fold 与 overlay，本增量复用这些
模式并补齐缺口，不新建第二套管理流。

### UI/UX pre-implementation review

- **沿用模式**：浮层沿用 `.session-preview`；菜单沿用 `Menu` / `ContextMenu`
  同源 renderer；改名沿用 app-styled prompt；Remove 沿用现有 confirm modal；
  fold 继续用 caret + `aria-expanded`；偏好继续即时写入并跨刷新恢复。
- **提案**：project row 在 pointer hover / keyboard focus 时露出 `…` 与 pencil；
  preview 只显示 project name、chat count、workspace path、pin state。`…` 与右键
  渲染同一组六项动作。Pinned/Projects section label 变为可折叠 button。
- **风险态**：Archive chats 可逆；Remove 只写 `removed` 装饰偏好、从 rail 隐藏，
  不删除 session/journal/workspace，且 rail 提供 `Show removed projects` + Restore。
  永久 worktree 复用既有 `POST /api/worktree` 的 git/ref 校验，不新增任意 exec 面。
- **数据处理**：sidebar width、section folds 为 browser-local UI 偏好；project
  pinned/removed 进入现有 atomic `webui-meta.json` overlay。旧字段/旧文件兼容，
  既有 pin/archive/theme/sidebar/unread/rename 数据不迁移、不清理。
- **裁决**：Pin project 只把 project 稳定排到 Projects 顶部，不把其 chats 复制进
  session-level Pinned section；移动端保持既有 fixed drawer 宽度且不显示 resize
  handle。无未决产品问题。

## Spec delta

修订 `SPEC.md` 的 UJ-24 Web UI 产品面条目：

1. project hover preview + row actions；菜单六项均用户可达且与右键同源；
2. project overlay 增 `pinned` / `removed`，均为显示偏好；
3. desktop sidebar 220–480px pointer/keyboard resize，宽度跨刷新；
4. Pinned / Projects section fold/unfold，折叠态跨刷新；
5. Remove 不删数据，removed project 可显式恢复。

状态先记 ⚠️，A/B 双闸完成后同增量升 ✅。验收锚：
`TestMetaStoreProjectOverlayRoundTrip`、`TestProjectUpdatePinnedRemoved`、
`Sidebar.nav.test.tsx`（project preview/menu/sort/remove/section fold/resize）+ QA-78。

## Design delta

修订 `DESIGN.md` §12 Web UI product surface 的 project overlay 条款：新增
`pinned` / `removed` 两个装饰字段；`removed` 只影响 sidebar projection，
command palette 与 journal-backed session 真相仍完整可达。sidebar width 与
section fold 是 localStorage UI preference。

**是否触及不变量：否。** project grouping 仍严格从 journal workspace 派生，
overlay 不改变任何 session 的 group membership；Remove 不删除或改写 journal、
session、workspace，只隐藏一个 rail projection，并有恢复入口。永久 worktree
完全复用既有 `/api/worktree` 安全边界，不新增 host exec 契约。

## 验收

### A 闸（scripted / unit）

- Go：overlay `pinned` / `removed` round-trip、partial update、清空 default entry、
  legacy file load 保持；HTTP project update 两个新字段逐项对锚。
- Vitest：hover/focus action 可达；preview 含 name/count/path；project menu 六项；
  pin 稳定置顶/unpin 恢复；Remove confirm 后隐藏但 session 数据仍在、Show removed
  可恢复；Pinned/Projects section fold 持久化；resize pointer + keyboard clamp 与
  持久化；mobile class 保留 fixed drawer 且 handle 隐藏。
- 全树 `./scripts/check.sh` 全绿。

### B 闸（QA-78，共享真实环境）

1. desktop 真 sidebar hover 一个真实 project：preview 与 `…`/pencil 出现，菜单
   六项齐全；rename、pin/unpin、section fold 刷新后仍保持。
2. pointer drag 侧栏到最小/中间/最大，main 无横向溢出；刷新后宽度恢复；separator
   键盘 ArrowLeft/ArrowRight 可调。
3. Archive chats 后项目按既有 archive 语义消失且 Archived 可恢复；Remove 确认
   明示“不删 chats/files”，隐藏后 command palette 仍能找到其 session，Show
   removed projects 可恢复。
4. 对真实 git project 创建命名 worktree，路径存在、`git worktree list` 可见；
   无效 branch 显式报错；测试数据与 worktree 保留。
5. 390px mobile：sidebar drawer 不被持久 desktop width 改写，project `…` 菜单仍
   可达、无 resize handle；light/dark console error+warning 为 0。
6. 证据保存到 `qa/runs/2026-07-21-QA78-sidebar-project-controls/`，不清理共享数据。

## 实施步骤

1. overlay contract + Go tests：`pinned` / `removed` partial update/persistence。
2. frontend project controls + tests：preview、row actions、六项 menu、sort/remove/
   restore、permanent worktree。
3. frontend sidebar controls + tests：width resize、section folds、responsive/a11y。
4. A/B 双闸；并回三层/QA/GAPS/LOG；工作纸归档；commit 并 push `origin/main`。

## review 裁决

小型 Web UI 增量，不触不变量，裁掉里程碑级三视角对抗 review。数据安全风险
集中在 Remove 与 worktree：前者被裁决为可恢复的显示偏好并由 confirm + 真机
可达性验证兜底；后者复用已有、已测试的 git worktree contract，不新增执行面。
