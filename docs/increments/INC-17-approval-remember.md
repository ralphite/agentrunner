# INC-17 审批"允许且不再问"（G5，取 A）

## 动机与 journey 锚

GAPS **G5**（设计缺失·中）+ UJ-08 步骤2「用户选'允许，且以后不再问'→
规则写回配置」。对标 CLAUDECODE-PARITY §2.06 #58。现状审批只单次批/拒
（`daemon.handleApprove` → resolve → journal `approval_responded`），无
"记住"。

## 两个开问的裁决

1. **本 run 何时生效**：**取 A（下次生效，不触不变量）**（INC-D5 已推荐）
   ——写回配置文件，**下次** session 拼 PermissionLayers 时读到；本 run
   该审批仍按本次应答（allow 这一次）。与"permission rules 冻结于
   SessionStarted"零冲突。取 B（PolicyChanged 本 run 生效）触不变量、
   放宽须限本 session 不下传已 spawn 子——推迟。
2. **写哪层**：**user 配置**（`runtime.UserConfigPath`）。裁决理由：
   project 配置的 allow 规则在 workspace 未 trust 时**降级为 ask**
   （config.Merge，决策 #19）——写回 project 会让"以后不再问"在未 trust
   的 workspace 里静默失效，违背用户意图。user 层恒生效、无 trust 纠缠、
   一定兑现。**取舍记档**：user 层是**全局**的，与 UJ-08"这个项目里"的
   project 作用域有偏差——精确到 project 的写回需 config 加 local 层或
   workspace-scoped 规则字段，列为**余项**（独立增量）。用精确命令/路径
   判据把超范围降到最小。

## 匹配粒度（安全）

写回一条**精确 allow**（不是宽通配）：
- bash：`{tool: bash, command: "<被审批的确切命令>", action: allow}`
  ——下次**完全相同**的命令免问（重复的 build/test 命令是主用例）。
  **不**做前缀/通配的智能提取（`git push` → `git *` 会放行
  `git reset --hard`，危险）——精确匹配最窄最安全，通配提取列余项。
- edit_file/write_file：`{tool: <tool>, path: "<确切路径>", action: allow}`。
- read-class（read_file/grep/glob）不触发审批，不涉及。
- **去重**：同规则已在则不重复 append（幂等，防重放/重复 approve）。

## Spec delta

- SPEC D「审批答复写回规则」❌→✅（取 A/user 层/精确匹配），锚
  `TestRememberRule*` + QA-26。
- CLAUDECODE-PARITY §2 #58 状态更新。

## Design delta（取 A，不触不变量）

DESIGN §审批/permission 加一小节「审批记忆（INC-17，G5，取 A）」：
- `ApprovalAnswer` 加 `Remember bool`；`ar approve <sid> <id> approve
  --always`。daemon `handleApprove`：Remember 时，除正常 resolve（本次
  allow）外，从被审批 effect 提取精确判据，**append 进 user 配置**为一条
  allow 规则（append-only、保留既有、去重）。**下次** session 生效。
- **不触不变量**：本 run 不改冻结的 PermissionLayers；写的是配置文件，
  下次读。DESIGN §15 决策表加一行（写 user 层 + 取 A + 精确匹配）。

## 验收

- 孪生：`TestRememberRuleFromEffect`（bash/edit effect → 精确 allow 规则）
  /`TestApproveAlwaysAppendsUserConfig`（handleApprove Remember → user
  settings.yaml 追加、保留既有、去重）/新 session 读到后同命令 pipeline
  直过（用 config.Merge + PermissionGate 断言）。
- 真实 API QA-26：UJ-08 流——ask → `ar approve --always` → 起新 session
  跑同命令 → 直过不 ask；`ar events` 归档 qa/runs/。
- `./scripts/check.sh` 全绿。

## 实施步骤

1. 探码 handleApprove/ApprovalAnswer/Effect 判据/config 写回路径。
2. `rememberRule(effect) (PermissionRule, ok)` 纯函数 + user 配置 append 助手。
3. ApprovalAnswer.Remember + handleApprove 接线 + CLI `approve --always`。
4. 孪生 + QA-26。
5. 文档行齐活。

## review 裁决

做。M 号、取 A 不触不变量、写 user 层可靠兑现。inline 三视角：
correctness（去重、append-only、判据提取）、security（精确匹配不宽通配、
写回是用户显式动作、user 层恒生效不绕 trust——但正因如此不放宽 project
未 trust 的 repo 规则）、contract（不改冻结 layers/审批语义）。
