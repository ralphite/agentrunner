# INC-38 Codex 式任务收尾与首屏真相

## 动机与 journey 锚

UJ-24 已具备 Codex 式骨架，但真实走查仍有三处会破坏任务主路径：deep link
首屏在 session 列表尚未返回时短暂显示 `No tasks yet` 和 raw session id；任务
完成后只有一枚状态，不先回答耗时、产出和下一步；New task 把 workspace /
运行位置 / branch 压在 composer 底部的一枚次要按钮里。用户提供的 Codex
参考图要求通用交互严格采用 Codex，本增量修订 UJ-24 第 1–3、6 步。

## Spec delta

- Web UI 产品面：首次 session fetch 完成前显示诚实 loading，不投影假空态或
  raw id；成功空列表才显示 `No tasks yet`。
- Web UI 任务收尾：每轮最终 assistant answer 前显示 `Worked for …`；完成态在
  answer 后显示真实 workspace Changes 摘要，并提供 `Review`；answer action
  提供 Copy 与复用既有 checkpoint fork 的 `Continue in new task`。
- Web UI progressive-disclosure composer：New task 将 workspace / Local /
  branch 提升为 composer 上缘环境条；具体 workspace、运行形态与 branch
  选择仍在同一 popover，不增加第二套配置真相。
- Web UI task navigation：sidebar task hover 同屏提供 pin/archive，并以非交互
  preview 显示完整标题、project、真实当前 branch（可查询时）与状态；键盘
  仍通过现有 context menu 获得同等操作。

不加入未有可靠持久语义的点赞和 `Undo`，避免伪交互。

## Design delta

补充 DESIGN Web UI projection：session 列表的首次 fetch readiness、turn duration
与 Changes outcome 都从现有公开 session/journal/diff 事实派生；不复制状态机，
不改 journal/API/daemon 或既有不变量。

## 验收

- frontend 单测：首屏 readiness 成功/失败语义；turn 最终 answer 耗时选择；
  numstat/untracked Changes 摘要。
- 真浏览器 QA-41（共享 store）：硬刷新 deep link 不出现假空态/raw id；真实
  completed session 显示 Worked、Copy、Continue、Changes Review；New task
  环境条在 desktop/mobile 可读可操作；Changes Review 进入原 diff；console
  error/warning=0。
- `npm --prefix webui/frontend test`、build、`./scripts/check.sh` 全绿。

## 实施步骤

1. 首屏 session readiness + loading 投影 + tests。
2. Worked / answer actions / Changes outcome + tests。
3. New task 环境条重排 + responsive/browser QA-fix。
4. Sidebar hover actions / metadata preview。
5. QA-41 证据、三层/QA/GAPS/LOG 收口，工作纸归档。

## review 裁决

纯前端 projection 与现有 action 重排，不触不变量；裁掉三视角对抗 review，
以真实共享 store、Codex 参考图同 viewport 对照及 desktop/mobile 黑盒 QA 代替。
任何 P0/P1/P2 在本增量内修完。
