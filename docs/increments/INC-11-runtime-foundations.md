# INC-11 通用 coding-agent runtime 基础加固

> **状态：已裁决，实施中。** 来源：2026-07-09 对 Agent runtime 的全栈
> 审查。该增量按依赖顺序关闭一致性、安全、扩展能力与长期运行缺口；每
> 一步独立过 `check.sh`、真实共享 store 与真实 daemon/Web 闸门后提交。

## 动机与 journey 锚

- UJ-03/07/08/09/12/13/15/16/17/18/19/20/21/22：输入不丢不重、控制
  可恢复、审批与 verifier 不越权、长期会话可迁移、MCP/多 agent/云任务
  能在同一核心模型上成立。
- 真实共享 store 已复现：driver journal 被 run fold 读取而显示
  `unreadable`；当前 daemon 的 `send` 在 mailbox fsync 后若内存队列满仍
  返回失败，客户端重试会形成两个可执行输入。
- 审查确认：control/close/interrupt/approval 只以内存投递确认；in-session
  goal verifier 绕过 effect pipeline；filesystem sandbox 为空；MCP 未从
  spec/CLI 生产路径建立连接；Turn/Item、typed ingress、provider capability、
  durable multi-agent coordination 与长期存储迁移均欠缺。

## Spec delta

1. A/E：所有外部 session command 使用调用方 `command_id` 幂等提交；ack
   只表示 durable accepted，内存唤醒永远是 best-effort。旧 mailbox 自动
   兼容读取。
2. D/F：所有 verifier 都是 effect，经过 mode/permission/approval/budget/
   containment；filesystem/network sandbox 是可执行的 OS 边界，能力缺失时
   fail closed。
3. H：MCP server 可在 spec 声明 stdio/streamable HTTP；支持 tools、
   resources、prompts、动态能力变化、结构化/多模态结果。annotation 仅用于
   UI/默认分类，不得作为重放安全证明。
4. A/H：持久模型补稳定 `turn_id`、`item_id` 与 typed ingress
   (`principal/source/trust/content-type`)；未知 item/capability 可透传。
5. B/E：多 agent 增 durable task/message/lease/workspace 记录；默认隔离
   worktree，权限与预算只窄不宽。
6. E：event/inbox 读取有索引/游标；snapshot 真正减少 replay IO；版本升级
   通过显式 migration 或兼容 reader，不因增加可选字段拒绝旧 session。
7. I：run 与 driver journal 使用各自 fold/inspect projection，真实旧数据
   不再显示 `unreadable`。

## Design delta 与不变量变更

### 旧不变量冲突

- 旧实现把 durable mailbox append 与内存 enqueue 合成一次“成功/失败”，
  与“输入先持久化、accepted 后不丢不重”冲突。
- §1 把 interrupt 定义为不进 inbox 的带外瞬时信号；这不足以支撑“已确认
  控制跨 crash 生效”。新定义区分 **durable command fact** 与 **ephemeral
  wake signal**：interrupt 内容/顺序是 fact，cancel ctx 是 wake/effect。
- DESIGN 非目标“event schema 变化丢弃旧日志、不做 migration”与跨日、
  跨版本长期 session 冲突。新不变量为“已 accepted 的 journal 必须由兼容
  reader 或显式 migration 恢复；不支持的破坏性版本给出可操作错误且不改
  原数据”。
- MCP `readOnlyHint` 是不可信 annotation；删除“readOnly 即可安全重放”的
  推论。重放只依据本地 policy 中显式声明的 idempotency key/contract。

### 新骨架

```text
Durable CommandLog(command_id, seq, principal, source, trust)
  -> Session -> Turn(turn_id) -> Item(item_id, typed content)
  -> Effect/Activity(effect_id, replay policy, sandbox evidence)
```

- CommandLog append 成功即 ack；重复 `command_id` 返回原 receipt；consumer
  先 journal command fact 再执行，完成 fact 使 crash-resume 幂等。
- Provider、MCP、Device、Hook、Policy/Sandbox、MultiAgent Coordinator 是
  versioned capability provider，不把 browser/computer/audio 等硬编码进 loop。
- snapshot 记录 journal offset/hash；resume 从 offset 后读取并校验前缀。

### 波及面

`protocol`、`store`、`event/state`、`daemon`、`agent/driver`、`pipeline/tool`、
`mcp`、`provider`、CLI/webui，以及 DESIGN/SPEC/GAPS/CODEX-PARITY/QA/LOG。

## 验收

- scripted/crash：append 成功 + wake 失败仍 ack accepted；同 command_id 重试
  只执行一次；每种 command 在 accepted/handled 两侧 crash 均不丢不重。
- security：goal/driver verifier 均产生 EffectRequested/Resolved 与 sandbox
  evidence；filesystem/network 越界在 macOS/Linux 下被 OS 层拒绝。
- MCP：真实 stdio 与 streamable HTTP server；断连重建、list_changed、
  resource/prompt、structured/image result；伪造 readOnlyHint 不获重放权。
- schema：旧共享 store 的 run/driver/goal/child/approval 日志均可 inspect；
  新旧数据跨 restart 可续。
- performance：10k command/event 的 append/replay 为线性；snapshot resume
  只读取尾部。
- 全量：`./scripts/check.sh`、相关 QA/UQ；真实共享
  `~/.local/share/agentrunner`、真实 daemon、`http://127.0.0.1:8788` 浏览器路径
  在 restart 前后通过。

## 实施步骤

1. ✅ INC-11.1：修 run/driver journal projection 与现有测试超时/顺序/timer/
   socket 路径基线；真实旧 store 可读。`check.sh` 全绿；共享 store 中旧
   driver 已由 `unreadable` 恢复为 `satisfied/max_iterations`，inspect 可
   展开 iteration 子树。
2. ✅ INC-11.2：统一 durable command/receipt/idempotency；`inbox.jsonl` 兼容
   读取 legacy input，append 索引从 O(n²) 降为每次启动一次线性重建；覆盖
   send/control/close/interrupt/approval/kill。调用方 `command_id` 与 event
   causation 分轴；所有 command 单 FIFO wake、宿主内去重；daemon 启动扫描
   CommandLog/journal 差集并 re-host。`check.sh`、race 与真实重启闸门通过。
3. ✅ INC-11.3：in-session/driver command verifier 统一 journaled effect +
   Activity bracket；mode/permission/hooks/approval/budget/containment 全绑定。
   bash/verifier 默认强制 filesystem=workspace，macOS Seatbelt、Linux
   Bubblewrap，`network:none` 同 backend 收紧；凭据路径/敏感 env 隔离，能力
   缺失在 Activity 前 fail closed。共享 store 真实 session
   `20260709-214651-exercise-sandbox-28ae` 与
   `20260709-214800-stand-by-for-goal-d657` 通过。
4. ✅ INC-11.4：MCP spec 自动接入所有 Loop 生产路径；stdio + streamable
   HTTP、环境变量 bearer/header、resources/prompts、structured/multimodal、
   list_changed 与断线后新 session；远端 `readOnlyHint` 不再授予重放权。
5. INC-11.5：Turn/Item + typed ingress + provider capability envelope，兼容旧
   Message/GenStep reader。
6. INC-11.6：durable multi-agent task/message/workspace coordinator。
7. INC-11.7：索引、snapshot offset、schema migration/compatibility。
8. INC-11.8：三层文档收口、对抗 review、全自动与真实环境 QA。

## review 裁决

这是里程碑级且改变 durability/security/versioning 不变量的增量。每个触及
不变量的步骤必须做 correctness/concurrency、security、contract 三视角
review；P0/P1 清零后才能关闭。用户要求实现全部审查项，视为对上述 delta
的实施裁决；若实现发现语义冲突，按 PROCESS §4 先修本工作纸与 DESIGN，
不得先在代码里绕过。
