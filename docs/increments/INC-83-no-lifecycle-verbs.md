# INC-83 · 生命周期动词全面拆除：用户面没有"活着/关闭"

**日期**：2026-07-19 · **来源**：用户裁决（PLAN Phase 6，推翻 Phase 4
"打断/关闭两概念收敛"的方向）。本文件是 PROCESS.md §四 不变量变更流程
的"单独成文"件。

## 一、用户裁决原文（约束）

> 我们根本没有"活着"和"不是活着"这些概念呀，也根本没有"close 一个
> session"的概念呀。这些我已经讲了很多遍了，都不是我引入的东西，都不
> 应该存在。……这些全部都要删除呀！尤其是和用户相关的、与用户界面相关
> 的，更应该删除啊！

即：close / stop / kill / interrupt 这一族**生命周期动词不是产品概念**。
静止模型（决策 #31，用户认可的根设计）推到底：会话只有"在干活 / 在等
你"，永远可续——没有"关"，没有"死"，没有需要用户管理的生命周期。

## 二、旧不变量原文

DESIGN §15 决策 #30（标记+检查，2026-07-05 修订）：

> close/kill 是**标记**（含来源 user/parent），只被检查引用：自动路径
> 不唤醒、用户 kill 的子仅用户可复活；不挡用户显式 send。无终止状态、
> 无 session-completed 事件

以及其上生长的全部用户面：`ar close`/`ar stop`/`ar kill` 三命令、
webui "Close session" 菜单/Stop 语义/Background kill 按钮、
closed/stopped/killed 状态词汇（sessions/inspect/webui）、INC-82 刚
收敛的"打断/关闭"两概念表述、hook ingress 对 marked session 的 410。

## 三、为什么必须动

决策 #30 说对了一半（"终止无真实需求"——所以没有终态），却引入了
"标记"这个**面向用户的替代品**，后续每轮都在上面盖楼（stop 标记、
kill 来源、复活规则、INC-74/INC-82 的清标记之争），长出一整族用户
必须理解的相似概念。审计的判据（"UI 只承诺已接线的能力"、"driver
不是产品概念"）同样适用于此族：**每一个"停"的真实需求，都有比给
会话盖章更对的归属**——

| 真实需求 | 正确归属（已存在或本增量补齐） |
|---|---|
| 停止当前正在生成/执行的轮 | "Stop" 手势（Esc/按钮；affordance 不是概念） |
| 让挂着的 goal 别再驱动 | `goal cancel`（goal 自己的领域动词） |
| 让周期唤醒别再醒 | `schedule cancel`（schedule 自己的领域动词） |
| 停运行中的 loop/best-of-N 系列 | series 落 `SeriesEnded{cancelled}` 领域终态（本增量补齐） |
| 让 webhook 别再唤醒 | `ar hook revoke`（hook 自己的领域动词） |
| 收掉跑偏的后台子任务 | agent 的 `kill` 工具（模型内部事务，用户打断后吩咐即可） |

覆盖完这张表,"close 一个 session"没有剩余职责——不想聊了就不聊。

## 四、新表述（替换决策 #30 的用户面部分）

**用户面没有生命周期动词。** 会话的用户可见形态只有：working /
waiting（含 approval/ask 细分）/ failed（provider 类可重试失败）。
"停"的手势只有一个：**Stop 当前生成**（打断本轮活动，无任何标记，
idle 时 no-op）。自动驱动源（goal/schedule/series/hook）各自用领域
动词终止，终止是**该对象的事实**，不是会话的状态。

**内部机制保留但降级为实现细节**：(a) 旧 journal 的 SessionClosed
事件 fold 兼容读，状态投影折为中性形态（不再输出 closed/stopped/
killed 词汇）；(b) agent `kill` 工具对子会话落的 parent-kill 标记
保留（树内部纪律：被 kill 的子不被自动复活），它从不出现在用户面。

## 五、波及面（→ PLAN 6.1–6.6 队列）

| 面 | 处置 |
|---|---|
| `internal/cli`：close/stop/kill 三命令 | 删出命令面与 help；interrupt 保留、help 措辞改"stop what it's doing now" |
| daemon wire：close/stop/kill | webui transport 所需的内部保留（thin-shell），不宣传 |
| series 停止 | 新增 series cancel 路径：运行中 series 收到取消→`SeriesEnded{cancelled}`；drive sweep 不复活 cancelled 系列 |
| webui | "Close session" 菜单删、Background kill 按钮删、closed/stopped/killed 词汇清、Stop=停当前生成 |
| 状态投影 | sessions/inspect/Quiescence 词汇收敛；hook ingress 改为 hook revoke 即停（不查会话标记） |
| fold/内核 | SessionClosed 用户写侧删；INC-82 规则随之简化；child parent-kill 门保留为内部 |
| 文档 | DESIGN #30 重裁+§12 清词、SPEC/JOURNEYS/QA 清词、FEATURES v1.3、GAPS/LOG 记档 |

## 六、契约视角自审

- 旧 journal 里既有 SessionClosed 事件必须永远可读——fold 保留 case,
  只改投影词汇;不做任何 journal 迁移。
- webui thin-shell 教义不破:UI 需要的停止路径仍经 `ar` CLI(interrupt
  与 series cancel),只是不再以生命周期概念呈现。
- 决策 #31(静止模型)不动——本增量是把它贯彻到用户面的最后一步。
