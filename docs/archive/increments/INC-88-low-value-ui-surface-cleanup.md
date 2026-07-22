# INC-88 低价值 UI surface 收敛

## 动机与 journey 锚

用户复核 **UJ-24 Web UI 产品面**后，要求删除审计清单 1–12：重复的
deep-link/ID/Stop/Continue/Environment 入口、无动作状态、前置条件不成立的
composer 控件、空菜单分组、重复 Settings 退出按钮与过密 sidebar 行操作。

目标不是删除底层能力，而是让默认产品面只保留完成真实任务所需的最小入口：
deep-link 路由继续支持 reload/bookmark；Stop 在有 composer 的 session 只留
composer、无 composer 的 RunView 仍留 header Stop；routine Continue/Fork 只留
Advanced 菜单（终态 recovery 告警的必要续跑动作例外）；内容型 Copy
（message/code/command/diff/path）全部保留。

### UI/UX pre-implementation review

- **沿用模式**：session/project 管理继续统一使用现有 `Menu`；状态相关项按当前
  state 条件渲染；Settings 继续用 720px 断点；composer 继续渐进披露；
  Environment 只显示当前可行动作。
- **提案**：普通用户面移除 Copy link、Session/Run ID；session row hover 只露一个
  `…`；删除装饰性 open glyph；有 composer 的 session 删除 header/menu Stop；
  Fork/Continue 只留 `Advanced → Continue in new session…`；connected daemon 变为
  安静状态，只有 offline 才是 Restart button；菜单只在有动作时显示 Run label，
  View 只显示可切换的目标；未选 project 时 composer 只显示 project picker；
  clean/sub-agent Environment 不画空 Changes 或 disabled Commit；Settings desktop
  只留 Done，mobile 只留 Back。
- **风险态**：Stop 与 Continue 都至少保留一个用户可达入口；RunView Stop 不动；
  deep-link router 不动；离线 restart 不动。不存在数据删除或不可逆操作。
- **数据/内容处理**：不迁移、不清理任何 localStorage、session、journal、workspace
  或 QA 数据；Copy message/code/command/diff/path 保留。
- **未决问题**：无。用户已明确授权清单 1–12。

## Spec delta

修订 `SPEC.md` 的 UJ-24 Web UI product surface、progressive composer、turn 收尾与
交互语义条目：

1. deep-link 是路由能力，不在普通消息/sidebar/Scheduled 菜单重复提供 link/ID；
2. 有 composer 的 session 以 composer Stop 为唯一直接停止入口；RunView 例外；
3. routine Continue/Fork 只在 Advanced 菜单保留，终态 recovery 例外；
4. sidebar session row 只露一个管理菜单；connected daemon 为非按钮状态；
5. menu label/current view/Environment/composer 控件按动作与前置条件条件渲染；
6. Settings 退出按 breakpoint 单一化。

验收锚：frontend component tests + QA-80 真实共享产品面。

## Design delta

修订 `DESIGN.md` §12 Web UI product surface 的默认 surface 规则：底层可寻址能力
与开发诊断标识不自动获得常驻 UI；同一动作在同一上下文只保留一个主要入口；
不可行动的 menu group、view item、Environment row 与前置条件未满足的 composer
控件不渲染。

**是否触及不变量：否。** deep-link/session/fork/interrupt/worktree contract 均不变；
只调整前端 projection 与入口密度。DESIGN 的 Stop 语义、journal-first、thin-shell
与数据保留条款均保持。

## 验收

### A 闸（scripted / component）

- Timeline：消息仍可 Copy，已无 Copy session link 与 answer-tail Continue。
- SessionView：无 topbar/menu Stop 与 topbar Fork；Advanced Continue 仍可达；View
  仅显示另一视图；Run label 只在至少一个 run action 存在时出现。
- Sidebar/Scheduled：普通菜单无 Session/Run ID/link；session row 只有一个 `…`；
  connected 状态非 button 且不显示 build id，offline Restart 仍可点击。
- Composer：未选 project 时不渲染 run-location/branch；选中后恢复。
- Environment：clean 不渲染 Changes/Commit，sub-agent 不渲染 Commit；dirty parent
  仍可 Review/Commit。
- Settings：desktop 只有 Done，mobile 只有 Back。
- frontend vitest/build 与 `./scripts/check.sh` 全绿。

### B 闸（QA-80，共享真实环境）

1. 当前 worktree 前端 `http://127.0.0.1:5188/` 连接真实
   `http://127.0.0.1:8809/` + `~/.local/share/agentrunner/`：Home、idle/failed/
   completed session、Scheduled、Settings、desktop sidebar 与 390px mobile 全部可用。
2. 直接 deep link/reload 仍打开原 session；session row/menu 仍可打开和管理。
3. 真实 running session 若当前环境已有则验证 composer Stop 唯一；若无，不为截图
   新启昂贵模型任务，改由 component state test 钉住，RunView Stop 另验 DOM。
4. console error/warning 为 0；截图和 DOM 证据归档到
   `qa/runs/2026-07-21-QA80-low-value-ui-cleanup/`，不清理共享数据。

## 实施步骤

1. 工作纸与产品裁决入档。
2. session/timeline/sidebar/Scheduled 清理 + tests。
3. composer/Environment/Settings 条件披露 + tests。
4. A/B 双闸；三层/QA/LOG 收口；工作纸归档。

## review 裁决

做一次契约视角收口 review：逐项证明唯一入口仍可达、底层 contract 未删、响应式
形态没有把仅存入口藏掉。改动不触并发、权限或不可逆数据面，裁掉里程碑级三视角
review。

## 实施结果

- 代码：12 项全部落地；不变的 router/interrupt/fork/worktree contract 均保留。
- A 闸：frontend 定向 138/138、全量 vitest 658/658、build 与
  `./scripts/check.sh` 全绿。
- B 闸：QA-80 PASS；会话
  `20260721-221631-say-hi-in-one-word-a4dd080497611f5d` 的 deep-link reload、
  desktop/mobile/Scheduled/Settings/clean Environment 全部通过，console 0 warning/error。
- 数据纪律：未创建、关闭、删除或清理共享 session/workspace/journal。
