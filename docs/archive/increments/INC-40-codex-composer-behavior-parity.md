# INC-40 Codex composer 行为对齐与真实浏览器验收

## 动机与 journey 锚

修订 UJ-24 第 2/4/5/6 步。INC-38 只按截图把 workspace、运行位置与
branch 合并进一个 `Environment` 菜单，视觉近似但操作模型错误；窄屏还会
隐藏 access，Supervision 会遮住 Changes，完成状态插入时间线的位置也错误。

本增量以用户提供的 Codex 截图和本机 ChatGPT/Codex `app.asar` 中的真实
composer 模块为行为基准，不再凭截图猜测：

- Project、运行位置（Local/New worktree）、local environment、branch 是四个
  独立控件；各自只负责一种选择。
- Project picker 自带 search、selected check、New project 与
  Don't work in a project；菜单高度有界、可键盘操作。
- AgentRunner 品牌与 Supervision 保留，但不得盖住审批或 Changes 主任务。
- desktop、642px 与 390px 都必须保留 access、model、send 的可达性。

## Spec delta

- 修订 “Web UI progressive-disclosure composer”：上缘环境条由四个独立语义
  控件组成；Project picker 支持搜索、清除、新建/选择项目。
- 修订 “Web UI 交互语义”：窄屏不隐藏权限；popover 受 viewport 约束；
  Supervision 在窄屏默认关闭且不遮挡 Changes。
- 修订 “Web UI Codex 式任务收尾”：terminal 状态不得插在历史事件之前。

## Design delta

修订 DESIGN §12 Web UI product surface：INC-38 的“同一 popover”实现约束
改为四个独立 composer control，共享同一提交 state。没有触及 §15 不变量。

现有 pattern 与提案：

- 现有：单个 535px 高菜单混合 recent workspace、运行类型与 branch。
- 提案：严格采用 Codex 的独立控件、32px 行高、有限高度 menu；只把文案、
  provider 和 AgentRunner 独有 Task options/Supervision 换成本产品语义。
- 风险状态：无项目、新 scratch、非 Git、空 branch、worktree 创建失败、无
  local environment、窄屏 approval/Changes、terminal/recovery。
- 数据处理：继续使用现有 shared store、`/workspace`、`/worktree`、
  `/git/branches` 与 composer spec；不迁移或删除用户数据。

## 验收

新增 QA-42，证据保留在 `qa/runs/2026-07-10-QA42-codex-ux-full/`：

1. 同 viewport 将 Codex reference 与实现截图放在一次视觉对照中。
2. New task 逐一打开 Project/运行位置/Environment/Branch，验证搜索、选择、
   清除、Escape、键盘方向键与 viewport containment。
3. 用共享 store 创建真实 session，完成 approval → completed → Changes →
   Continue in new task；不删除/关闭 session 或 workspace。
4. 1554px、900px、642px、390px 检查 access/model/send、sidebar、
   Supervision 与 Changes；console error/warning 为 0。
5. deep link reload 与 Web UI restart 后复核同一真实 session。

scripted 孪生：frontend tests 覆盖 project filtering、空 branch label、terminal
status placement 与 responsive state policy；frontend build、Web UI Go tests、
`./scripts/check.sh` 全绿。

## 实施步骤

1. 从本机 `app.asar` 提取行为证据，完成现状全流程黑盒审计。
2. 重构 composer 环境条与 popover 交互，补单测与 responsive policy。
3. 修复 Supervision/Changes 与 terminal timeline 顺序，补回归测试。
4. 重启真实 Web UI，按 QA-42 全流程复测并做同图对照。
5. 三层/QA/GAPS/LOG 收口，归档工作纸，check、commit、push `origin/main`。

## review 裁决

做 correctness、responsive/a11y、runtime-state 三视角 review。此次改变 New
task 主路径并涉及真实共享状态，不能裁掉；P0/P1/P2 必须在同轮清零。
