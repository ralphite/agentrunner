# INC-D5 审批"允许且不再问"（G5）— 设计稿

> **状态：设计稿，design-first；未实现。** 触及"permission rules 冻结于
> SessionStarted"（PermissionLayers 随 spawn 冻结下传）——本 run 生效路径
> 触不变量,取"下次生效"则不触。

## 动机与 journey 锚
GAPS **G5**（设计缺失·中）+ UJ-08 步骤2「用户选'允许,且这个项目里以后
不再问'→ 规则写回项目配置」。对标 Codex：审批"允许且不再问"写回 config。

## 现状
- 审批流：`daemon.handleApprove` → `ApprovalAnswer{Approve, Reason}` →
  resolver → journal `approval_responded`。**只单次批/拒**,无"记住"。
- 权限规则：user > project > spec 拼接,materialize 进
  `SessionStarted.PermissionLayers`（spawn 时冻结交集下传,决策 #20）。
- **`PolicyChanged` 事件不存在**（SPEC/GAPS 曾称"已设计",实为未实现）。

## 两个开问（须先裁）
1. **写哪层**：user 还是 project？UJ-08 说"这个项目里以后不再问"→
   **project 配置**（`<root>/.agentrunner/` 或既有 project rules 文件——
   实施时定位）。project 层需已 trust（决策 #19）。
2. **本 run 何时生效 vs 规则冻结**——二选一：
   - **(A) 下次生效（推荐,最小,不触不变量）**：写回 project 配置文件,
     **下次** session 拼 PermissionLayers 时读到；本 run 该审批仍按本次
     应答（allow 这一次）。简单、与冻结不变量零冲突。
   - **(B) 本 run 立即生效**：需 `PolicyChanged` 事件承载新规则,fold 进
     一个"运行时追加规则层"（叠加在冻结 layers 之上,只收窄?不,是放宽——
     须谨慎:放宽须限本 session 且不下传已 spawn 的子,与决策 #20 冻结交集
     语义交互）。触不变量,走变更流程。

## 机制草图（取 A）
- 审批答复扩展：`ApprovalAnswer` 加 `Remember bool`（+ scope=project）；
  `ar approve <sid> <id> approve --always`（webui 审批卡加"允许且不再问"）。
- daemon handleApprove：Remember 时,除正常 resolve 外,把该 tool/command/
  path/network 判据**append 进 project 配置**为一条 allow 规则（append-only,
  保留既有）。
- 生效：下次 session 读到。本 run 该次照常 allow。

## 波及面
- DESIGN §permission/审批：写回路径（写哪层/如何表达为规则/何时生效）
  成文；取 A 不触不变量,取 B 走变更流程 + 定义 PolicyChanged。
- 代码：ApprovalAnswer.Remember；handleApprove 写回 project 配置；CLI
  `approve --always` + webui 审批卡（webui 侧,注意另有 session 在改 webui）。
- SPEC D 行 G5；GAPS G5；QA 场景（approve --always → 同类命令下次直过）。

## 验收
- 孪生：approve --always → project 配置追加一条 allow 规则（断言文件内容）；
  起新 session → 同类命令 pipeline 直过不 ask。
- 真实 API QA：UJ-08 流（ask → approve --always → 后续同类直过）。

## review 裁决
取 A = 小/中增量 inline 自审（写回边界/append-only/trust 前置）；取 B =
走不变量变更流程。本纸仅设计,待裁 A/B + 写回层。
