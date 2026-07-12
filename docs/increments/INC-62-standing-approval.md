# INC-62 审批常设应答（standing approval）— 同 session「始终批准」生效 + spawn_agent 写回（G35）

> **状态：已裁决（用户 2026-07-12 选定方案一），实施随本增量落地。**
> 不触不变量：PermissionLayers 冻结语义零改动（见「与 INC-D5/决策 #38
> 的关系」）。

## 动机与 journey 锚

GAPS **G35**（用户现场：一个 session 内三次 spawn_agent，webui「始终
批准」三次全部重问）+ UJ-08（权限日常）/UJ-18（多 agent 编排）。用户
裁定：**同 session 内「始终批准」必须生效，这是最基本的 UX 要求**。

G35 查明的三层根因：① `rememberRule` 白名单静默排除 spawn_agent（规则
永不写回，跨 session 永远重问）；② 决策 #38 取 A 本就只承诺「下次
session 生效」；③ webui toast 无条件谎报「已保存」。

## 与 INC-D5 / 决策 #38 的关系

INC-D5 当年只摆了两个选项：(A) 写回配置下次生效（已取，不触不变量）、
(B) `PolicyChanged` 事件实时并入活层（触「层冻结」不变量，走变更流程，
已推迟）。本增量走的是当年未摆出的**第三条路**：不动 permission 层，
在**审批层**记住用户的常设应答——

> 权限闸门照旧裁定 ask；改变的不是「要不要问」，而是「这个 ask 由谁
> 来答」。用户点过「始终批准」的精确判据，本 session 内后续同判据的
> ask 由 journal 里的常设应答自动作答 approve，不再上浮给人。

这是既有教义的顺延一步：「`ApprovalResponded` 一旦成为事实即权威，
崩溃后绝不重问」（决策 #8/§6）→「同一判据本 session 内后续的 ask 也
被这个事实回答」。语义是「用户的常设应答」，不是「规则变了」——
PermissionLayers 一个字节不动，决策 #20 冻结交集、收容棘轮全部无恙。
决策 #38 因此**扩展**（取 A 保留管跨 session + 常设应答管本 session），
非推翻；不变量变更流程不适用。

## 机制

1. **判据提取统一**：`standingCriterion(toolName, args)` 抽取精确判据
   （bash=精确 command；edit_file/write_file/notebook_edit=精确 path；
   **spawn_agent=tool 级**——「始终批准 spawn」的用户意图就是别再为
   起子 agent 问我，且 PermissionRule 无 agent 维度，tool 级即诚实
   表达；web_fetch/MCP 等仍不提取，判据形态另议——沿 INC-17 原判断）。
   写回规则 `rememberRule` 与常设应答共用这一个提取函数，**结构性
   保证两套记忆永不歧义**（否则会出现"本 session 不问、下 session
   又问"的反向失真）。
2. **落盘**：approve 且 Remember 时，把提取的判据作为
   `ApprovalResponded.Standing`（新增可选字段，additive）随应答事实
   一起落 journal。fold（`Effects.Standing` 集合，去重追加）零解析
   逻辑，纯数据搬运。**无需 bump effects 子状态版本**：旧 journal 里
   不存在 Standing 事实，旧 snapshot + 新 binary 的尾部 fold 不会丢
   任何历史投影（决策 #18 的丢尾场景不成立），纯加法兼容，旧 session
   照常 resume。
3. **免问**：`requestApproval` 开头，对当前 effect 用同一
   `standingCriterion` 提取判据（与落盘侧同样先过 redaction，保证
   对称），命中 `Effects.Standing` 即直接落
   `EffectResolved{allow, gate:"approval", reason:"standing approval
   (this session)…"}`——不落 `ApprovalRequested`、不进 WAITING，
   审计链完整（EffectResolved 里写明由常设应答自动作答）。
4. **树语义（裁定）**：常设应答住在**各 session 自己的 fold** 里，
   子 session 的 ask 读子自己的 fold——父的常设应答**不**自动放行子
   的同类调用（审批经 correlation 上浮到人是树约束的一部分；父批过
   ≠ 子可扩权）。该语义由结构免费给出，无需额外代码。
5. **rewind/fork 语义免费正确**：常设应答是 fold 的一部分，rewind 到
   barrier 之前自然消失；fork 出的新 session 从 fork 点的 fold 继承。
6. **跨 session**：`rememberRule` 补 `spawn_agent` case（写
   `{tool: spawn_agent, action: allow}` 进 user 层），修复 G35 ①。
7. **webui 诚实化**：toast 不再无条件宣称「已保存规则」；改为中性
   「已批准（始终）」，权威反馈以 loop 侧既有的 `remembered:` 流消息
   为准（写回成功才发）。

## 波及面

- `internal/event/types.go`：`StandingRule{Tool,Path,Command}` +
  `ApprovalResponded.Standing`（可选，additive）。
- `internal/state/state.go`：`Effects.Standing` + fold case +
  `HasStanding`；版本不 bump（理由见上）。
- `internal/agent/approval_remember.go`：提取函数拆出共用；
  spawn_agent 入白名单。
- `internal/agent/approval.go`：requestApproval 免问短路；
  awaitApproval 落 Standing。
- `webui/frontend` SessionView toast 文案（dist 重建随包）。
- SPEC D 行、GAPS G35、DESIGN §5（常设应答成文）+ 决策 #38 修订注记、
  LOG。

## 验收

- 孪生（gate A）：
  - `TestStandingApprovalSameSession`：同判据第二次调用零
    `ApprovalRequested`，`EffectResolved` 带 standing 判词；
  - `TestStandingApprovalSurvivesResume`：常设应答 fold 重建后仍免问；
  - `TestStandingApprovalExactCriterion`：不同判据（另一 path/command）
    仍上浮询问——精确性不放宽；
  - `TestRememberRuleFromEffect` 扩展：spawn_agent 产出 tool 级 allow；
    web_fetch 仍不写回。
- 真实 API QA（gate B，**待在用户环境跑**——本容器无法全量）：UJ-08
  流内三连 spawn_agent，「始终批准」一次后两次直过；新 session spawn
  直过（写回规则生效）。跑绿前 SPEC 行保持 🟡。
