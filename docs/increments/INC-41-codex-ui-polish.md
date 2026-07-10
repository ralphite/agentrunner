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

## 并发注记

Codex 侧 goal 线程（"重构 Web UI 体验"）usage 限额 17:59 恢复后可能
续跑同域工作；本工作纸已占号，恢复前以本认领为准，恢复后先 fetch
rebase 再动手。
