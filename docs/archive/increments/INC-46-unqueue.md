# INC-46 排队消息撤销（HANDA #29，INC-44 §A rev1 实施）

## 动机与 journey 锚

忙时排队的消息不可撤/不可改（UJ-07 纠偏、UJ-24 webui 队列管理）。
设计与 §2 变更单已过契约 review（INC-44 rev1，AskResolved 三件套）。

## Spec delta

- SPEC A 区 durable CommandLog 行追加 revoke 条款 + 新行：排队消息
  撤销（`ar queue`/`ar unqueue`/daemon `unqueue`；revoke 同 durable；
  消费守卫落 `InputRevoked{target, delivery_seq}` 推 high-water；
  resume 重放改读 ReadCommands 先跳被撤；迟到 no-op）。

## Design delta（触 §2，修订与实现同 commit）

- §2 追加 rev1 条款（变更单文本见 INC-44 §A，含四性论证）。

## 验收

- 孪生：fold InputRevoked 推 high-water；validate/hash 稳定；
  journalInput 集命中落 revoked 不落 received；resume 重放矩阵
  （撤/迟到 no-op/被撤不冒充 ask 答案）；revoke 非 input 目标拒。
- B 闸（真 Gemini）：忙时排队两条→unqueue 第二条→settle 只消费第
  一条、journal 有 InputRevoked；kill daemon 重启 resume 不翻案。

## 实施步骤

1. 事件+fold+协议+inbox 校验/hash+守卫+resume+daemon+CLI+DESIGN §2
   修订 → 孪生 → check 绿 → 真验 → 文档 → commit（一组）。

## review 裁决

设计+变更单已过独立契约 review（INC-44 rev1），实施轮不重复。
**余项**：webui 队列撤回按钮（需 send API 返回 command_id，随 #7
webui 表单批一并做）。

---

## 执行记录（2026-07-11 收口）

rev1 三件套全实施，DESIGN §2 撤回条款同 commit。偏差记档：daemon
前置校验按"daemon 无 store 依赖"移到 CLI 侧（语义等同，更贴架构）。
B 闸含 kill -9 crash 重放实证（B1 场景）：KEPT 进、DROPPED 零、
InputRevoked 在账、二次 resume 收敛。证据 qa/runs/2026-07-11-INC46。
SPEC 两处；SPRINT #29 ✅。
