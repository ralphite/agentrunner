# INC-41 Codex UI polish 冲刺（认领：Claude session 2026-07-10）

## 动机与 journey 锚

UJ-24（webui 驾驶舱）。用户裁决：以本机 Codex 桌面 app 为标尺把 webui
打磨到同等质感——双方都有的功能按 Codex 做法；核心差异功能不强行对齐；
我们独有功能沿用 Codex 风格。本轮为 INC-38/40 的续作：不再新增行为面，
以**结构与质感**为主（timeline 收纳、活动聚合、欢迎态、命名、review
工具栏、终态呈现）。

参照与审计证据：`qa/runs/2026-07-10-codex-ui-study/`
（CODEX-UI-REFERENCE.md = Codex 全功能实测规格；PIPELINE.md = W1–W11
执行清单与进度；screenshots/ 现状全景，gitignored 不入库）。

## Spec delta

- 修订 "Web UI 产品面"：New task 空态为项目感知欢迎语 + 居中 composer。
- 修订 "Web UI Codex 式任务收尾"：Worked for 为真折叠容器，收纳 turn 内
  活动（工具/审批结果），三级展开（聚合行→逐条→Shell 块）；待决审批
  恒在折叠外。消息操作（Copy/时间戳）hover 呈现。
- 修订 "Web UI 交互语义"：sidebar 状态点语义收敛（仅 attention/running
  着色）；项目名去 raw path、同名消歧；图片附件有 lightbox。
- Changes 视图：范围切换（Working tree|Last turn）+ 汇总 +N -N +
  样式化 file/hunk header。
- Goal banner：终态（achieved/cancelled）呈现不消失。

## Design delta

不触 §15 不变量；纯 webui 前端（少量 arwebui 只读 API 复用）。DESIGN
§12 Web UI product surface 增补：timeline 分层 = 「用户可见对话流」+
「Worked 折叠内活动流」，审批待决项属前者，已决归后者。

## 验收（QA-43）

证据 `qa/runs/2026-07-10-QA43-codex-ui-polish/`：

1. 共享 store 真实 session（diff/goal/多审批/图片各一）逐屏复核 W1–W10
   行为；不删除/关闭任何 session 或 workspace。
2. 1440/900/642/390 × light/dark 全景截图；console 0 error/warning。
3. 与 Codex 参考规格逐项对照（CODEX-UI-REFERENCE.md §8 清单画勾）。
4. scripted 孪生：frontend vitest 覆盖 fold 收纳/聚合分组/项目消歧/
   banner 终态；frontend build + webui Go tests + `./scripts/check.sh`
   全绿。

## 实施步骤

按 PIPELINE.md W1–W11 分批 commit（每批 check 绿 + 真浏览器复核 +
push origin/main），P0（W1–W4）→ P1（W5–W9）→ P2（W10–W11）收口归档。

## review 裁决

correctness / responsive / runtime-state 三视角；涉及 timeline 结构重排,
approval 可见性回归（待决卡不得被折叠吞掉）为 P0 红线。

## 2026-07-10 completion-audit design note（QA-45）

**沿用 pattern**：以 `INC-41-CODEX-UI-REFERENCE.md` 的 New task、turn 间态、
usage/error banner 与 Scheduled Create 为准，复用现有 `Menu`、inline alert、
composer 和 timeline 结构；不另造卡片语言。

**本批修订**：Home placeholder 精确改为 `Do anything`；生成中只显示一行
无气泡 `Thinking`；`limit_exceeded` / failed / recovery 在 thread 内给出人能
理解的原因和已有真实下一步；Scheduled 右上改为 `Create` 菜单，映射现有
one-time / Goal / Repeating / Best-of-N 入口。

**风险与数据**：approval 卡继续位于 fold 外且优先；不伪造 rate-limit reset、
context capacity、Last turn diff 或 OS `Open in` 能力；不改 journal/session/
workspace 数据，不清理 QA 会话。已有 composer draft 与 deep link 均保留。

**暂不宣称完成**：D1 需要 durable turn-boundary diff baseline；I2 缺 provider
context-window capacity contract；J2 的 OS app launcher 缺 server API。三项在
底座成立前保持未完成/裁决，不以 disabled 假控件冒充 parity。

**Archived tasks**：沿用现有 Settings 左栏与 project grouping，新增独立
Archived 视图；只提供已有、真实的 search / project grouping / Unarchive / open
task。永久删除没有 daemon contract，明确不画 `Delete all` 或垃圾桶假按钮。

**Document download**：J2 只接通真实、可验证的 `Download a copy`。新增的
session workspace file GET contract 必须把相对路径约束在该 session 的真实
workspace 内（拒绝 absolute / `..` / symlink escape / directory，并限制普通文件
大小）；前端文件 chip 直接下载该响应。VS Code / Finder / Terminal 等 OS launcher
仍无跨平台 contract，继续不画，不以 disabled `Open in` 菜单冒充能力。

**Queued goal edit**：goal control 可能排在待决 approval 后，HTTP 200 只表示
daemon 已接收，不代表 `goal_updated` 已落 journal。UI 保存后立即显示新文本并
标 `Update queued`；直到 inspect/event 反映同一目标才清标记。这样既不跳回旧文案，
也不把尚未跨 safe boundary 的更新伪装成已应用。

**Strict Codex New task correction**：用户后续明确裁决“common UI strictly use
Codex”。因此本条覆盖本工作纸早期 W1 的自创 robot hero / “What should we
build?” 欢迎语：New task 回到 Codex 的底部宽 composer，不再让品牌插画或标题
抢任务输入层级。AgentRunner 品牌只留在 sidebar；project menu 的宽度、行高、
字号与 composer 密度按用户提供的 Codex 同状态截图校准。

## 并发注记

Codex 侧 goal 线程（"重构 Web UI 体验"）usage 限额 17:59 恢复后可能
续跑同域工作；本工作纸已占号，恢复前以本认领为准，恢复后先 fetch
rebase 再动手。

## 2026-07-10 QA-45 completion audit（本批）

共享 store 真浏览器证据在
`qa/runs/2026-07-10-QA45-perfect-ux-audit/`（保留会话/工作区）：

- `16-thinking-fixed.png` + 实时 DOM `status "Thinking"`：生成间态无气泡、
  无重复 accessible name；真实 background session 同时验证 Background work。
- `14-limit-exceeded-fixed-stable.png`：撞限原因与 Continue 行动，不伪造 billing。
- `17-scheduled-create-menu.png`：Create 的 one-time/Goal/Repeating/Best-of-N
  四入口逐项选择回填。
- `20-goal-download.png` + `20-download-headers.txt`：document chip 的真实
  workspace-confined 下载（内容 `DONE`）；Go test 压 traversal/symlink escape。
- `21-home-daemon-offline.png` / `22-…-mobile.png`：Home 离线条与 390px。
- `23-supervision-environment.png` / `25-background-work.png`：真实 diff/branch/
  commit 入口与 live worker handle；未替用户点 stop。
- `24-goal-update-queued.png`：真实 goal edit 被 approval safe boundary 阻挡时
  保持新文本并诚实标 `Update queued`。
- `26/27-settings-archived*.png`：Archived 空态；另以真实 QA task 完成 archive
  → search match/no-match → Unarchive 的可逆全链。
- `29-home-project-picker-1728-fixed.png` 与用户 Codex 同状态截图同批视觉比较；
  `31-home-strict-codex-mobile-reload.png` 复验 390px；console warn/error = 0。

仍不伪造的底座缺口：D1 Last turn 需要 durable turn snapshot；I2 context
occupancy 需要 provider capacity + compaction 后 live usage。两者不阻塞本批
把已有能力做成 Codex UX，但不得在终局 Z1 前冒充已完成。
