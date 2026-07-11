# INC-43 运行中发消息的投递模式（steer | queue，对标 Codex）

> **归档注记（2026-07-10 landed）**：本增量已落地，三层 delta 并回
> JOURNEYS（UJ-07）/ SPEC（A 域）/ DESIGN（§1 投递模式 + 术语表）/
> CODEX-PARITY（运行中 steering 行）/ QA（QA-45）/ LOG。孪生
> TestSteerDeliversMidTurn/TestQueueDefersToTurnEnd/TestSteerFlushesQueuedBacklog/
> TestInboxDeliveryModeIsPartOfPayload 与 QA-45 真机（steer seq10 / queue seq51）
> 全绿。原件只读封存。

状态：**landed**（PROCESS §二第 5 步收口；小增量 inline 自审，本工作纸即三层 delta 审）。
认领：Claude message-delivery-mode-a9751b，2026-07-10。
占号：**INC-43 / QA-45**（先 fetch 确认：INC-42 已占 mode 权限切换、QA-44 已占；
QA-43 为 INC-41 终局全景；INC-43/QA-45 空号）。

## 动机与 journey 锚

用户原话："we should be able to send msg in steer or queue mode. check codex for how it works in ui"

现状（核实）：向运行中 session 发消息**只有一种投递语义**——追加进 inbox、
在**下个 turn**才被模型看到（type-ahead，DESIGN §1「输入进对话 inbox，追加
语义，不打断」→ §128「下个 turn 模型看到它」）。要中途生效只能用 `interrupt`
（硬收尾当前 turn 再转向，DESIGN §132/§139-143）。而 Codex 提供**两种默认可选
的投递模式**：steer（把消息注入当前 turn，下个 step 边界生效，不硬打断）与
queue（排队到 turn 结束），并在 composer 暴露入口。CODEX-PARITY 第 42 行当年把
这条记为「语义差异，已裁决」——本增量把它升级为与 Codex 对齐的**双模式**。

**Codex UI 调研结论**（WebFetch/WebSearch + 仓库内 INC-41-CODEX-UI-REFERENCE.md
第 214-217 行实拍）：
- Codex composer 有「Follow-up behavior」设置：默认二选一——"Queue follow-ups
  while ChatGPT runs" 或 "steer the current run"；**`⌘⏎` 对单条消息做相反的那个**。
- TUI 版：turn 运行中，**Enter = 注入当前 turn（steer）**，**Tab = 排队到下个
  turn（queue）**。
- steer 的投递时机：**下个 step 边界**（当前模型输出 / 下一次工具调用之后），
  **不硬 interrupt**（HAPI #888：both native CLIs deliver steering at the next
  step boundary inside the running turn, without a hard interrupt）。另有 Esc
  可「interrupt 并立刻提交 steer」= 硬打断路径（对应我们既有 `ar interrupt`）。
- 排队消息可见、投递前可编辑/撤回（composer 保留待发文本，是本地待发态）。

对标取舍：本增量做 Codex 的**软 steer**（安全边界注入，不打断）+ queue（turn 末），
硬打断路径复用既有 `ar interrupt`（不动）。默认值 = **queue**（保持现有唯一行为，
PROCESS §三.3 opt-in 不破坏既有形态）；steer 为显式 opt-in。

**UJ-07 delta**：steering journey 增补一步——「用户运行中发消息可选投递模式：
`steer` 使其在**当前 turn** 的下个安全边界即进对话（模型本 turn 内看到、调整），
`queue`（默认）排队到 turn 结束进下个 turn；两种都是追加语义、都不打断在跑的
step。」覆盖功能标签增 `发消息投递模式(steer|queue)`。

## Spec delta

- SPEC A（会话交互域）新增功能点行「运行中发消息投递模式（steer 当前 turn
  安全边界 / queue 下个 turn，per-message，默认 queue）」**✅**，
  锚 `TestSteerDeliversMidTurn` + `TestQueueDefersToTurnEnd` +
  `TestSteerFlushesQueuedBacklog` + `TestDeliveryModeDurableIdempotent` + QA-45。
- SPEC 第 26 行「忙时投递排队（安全边界按序消费，不丢不乱序）」备注补一句
  「默认 queue；steer 模式在 turn 内安全边界即消费（INC-43）」。
- CODEX-PARITY 第 42 行「运行中 steering | mid-turn/queue 双默认 | ✅ 安全边界
  排队 | 语义差异，已裁决」→ 缺口列改为「✅ 双模式对齐（steer/queue per-message，
  webui+CLI，QA-45）」。

## Design delta

**新增语义（DESIGN §1/§18 投递模式）**：用户消息的 `UserInput.Delivery`
字段（`""`=queue 默认 / `steer` / `queue`）决定消费时机：
- `queue`（默认）：维持现状——追加进 inbox，idle（turn 末）由 awaitInput/
  drainQueued 消费，进下个 turn。这是今天唯一行为。
- `steer`：在 loop 安全边界（两 step 之间，drainBackground 同一 seam，loop.go
  ~1041）以新的 user-role 消息进对话，模型**本 turn 内下个 generation** 看到。
  **不 interrupt**（不 cancel 在跑 activity），只是可见时机从 turn 末提前到下个
  安全边界——与 receipts=`steer`（裁决 #15，背景回执 turn 内安全边界即进对话）
  完全对称。

**不变量核对（PROCESS §四）**：
- 触碰的粗体条款 = DESIGN §1「**interrupt 与输入分立**：输入进对话 inbox
  （追加语义，不打断）」。**判定：不破坏**。steer 仍是纯追加（不 cancel 在飞
  step、不落 `interrupted` 截断），与 interrupt（硬收尾当前 turn）分立如故；改变
  的只是**可见时机**（turn 末 → 下个安全边界），而「追加、不打断」的性质保持。
  interrupt 仍是唯一会收尾 turn 的通道。
- 触碰的散文 = §128/§224/§1489「用户 steer 消息…下个 turn 模型看到它」。此为
  **当时唯一模式的描述，非决策表(§15)行、非粗体不变量**。改为「queue（默认）
  下个 turn / steer 本 turn 安全边界」。既有 receipts steer/turn_end（裁决 #15）
  已确立「安全边界即进对话」为合法投递点与 steer/turn_end 词汇，本增量是对称扩展
  而非新不变量。
- **结论**：不走 §四 完整不变量变更流程（未动 §15 决策表行、未破坏粗体条款），
  但按「不悄悄绕」要求：本节显式登记散文 delta 与粗体条款核对，LOG 记档，最终
  汇报点名，供用户否决。

**落点**：
- DESIGN §1「统一原语」段后补一小节「投递模式（steer|queue，per-message，
  INC-43）」：定义两值、默认、与 receipts 的对称关系、seq 单调性保证（下）。
- §18.x 输入语义表 / 术语表「steering」条补「per-message Delivery：steer=安全
  边界 / queue=turn 末（默认）」。

**seq 单调性保证（正确性关口，写进 DESIGN 与孪生）**：`ConsumedInputSeq` 是
高水位（state.go:544；journalInput 丢 `DeliverySeq ≤ 高水位` 的重复）。steer 若
把高 seq 消息在低 seq queue 消息之前 journal，会把后者误判重复丢弃。规则：
**steer 触发时，先按 seq 序 flush 所有更早的待发 queue backlog（含跨 step 暂存
的），再 journal 本轮**——journal 的永远是当前最低连续 seq 段，暂存的永远是更高
seq 段。语义：一条 steer 把整个待发 backlog（含更早排队消息）一起带进当前 turn；
无 steer 尾随的纯 queue 消息才等到 turn 末。崩溃安全：暂存（高 seq）在内存丢失但
mailbox 仍 durable、未消费（高水位未越过），resume 经 mailbox 重投；已 journal 的
（≤ 高水位）resume 判重不重复。

**不触的边界**：
- interrupt/kill/close/goal/compact/clear/approval 全不动。
- 树内消息（peer/send_message，source=agent）维持 next-turn 语义，drainSteer 不
  读 l.peer。Delivery 仅作用于用户→session 的 `ar send`/webui send。
- 子 agent、权限、hooks、durable command 幂等（commandPayloadHash 已含 Input，
  Delivery 自动进 payload hash——不同 Delivery 的同 command_id 冲突 reject，
  完全正确，零改动）。

## 机制

- `protocol.UserInput.Delivery string`（omitempty；""/"queue"/"steer"，daemon 归一）。
- `daemon.Command.Delivery string` + handleSend 透传进 UserInput（daemon.go:1034）。
- `driveState.deferredInputs []protocol.UserInput`：drainSteer 暂存的 queue backlog。
- `loop.go` 安全边界（~1041，drainBackground 后）加 `l.drainSteer(ds, appendE)`：
  非阻塞 drain l.UserInputs 全部可用项 → 若本轮含 steer：`append(deferredInputs,
  buf...)` 全部按 seq 序 journal（flush backlog）、清空 deferredInputs；否则全部
  存入 deferredInputs。见 channel close → l.inboxClosed/l.UserInputs=nil（同
  drainQueued）。
- `awaitInput`（conversation.go）入口 select 前：`if len(ds.deferredInputs)>0`
  → 按序 journal + drainQueued（含新到 channel 项）+ resolve "input_received"
  返回启动下个 turn。保证纯 queue 全部经此在 turn 末进下个 turn。
- CLI `ar send --steer`（默认 queue）：`daemon.Command.Delivery="steer"`。
- webui：`api.go handleSend` 读 body `delivery` 透传 `ar send [--steer]`；
  frontend Composer 运行中显示 steer/queue 切换（默认 queue，`⌘⏎` 反选单条，
  对标 Codex），待发 pending bubble 标注模式、投递前可撤回（已有本地 pending 态）。

## 验收

孪生（闸门 A，进 check.sh；mirror TestReceiptsModeControlsSettlementTiming 形态，
mid-turn 注入 UserInputs）：
- `TestSteerDeliversMidTurn`：多 step turn 运行中投 `Delivery:"steer"` → 其
  InputReceived seq **早于**本 turn 末 assistant 生成（turn 内可见）。
- `TestQueueDefersToTurnEnd`：投 `Delivery:"queue"`（及默认 ""）→ InputReceived
  seq **晚于**当前 turn 末（下个 turn）。
- `TestSteerFlushesQueuedBacklog`：先投 queue（seq N）再投 steer（seq N+1）
  → 两条都在 turn 内 journal、seq 序不倒、无一丢弃（高水位单调）。
- `TestDeliveryModeDurableIdempotent`：同 command_id 同 Delivery 重放不双改；
  同 command_id 不同 Delivery 被 mailbox reject（payload hash 已覆盖）。
- crash 注入：steer journal 后崩溃 → resume 高水位=该 seq、暂存 queue（高 seq）
  经 mailbox 重投、不丢不重（fold 纯性）。

QA-45（闸门 B，真实 API；共享 daemon 与 store，不隔离沙箱）：
1. 起真实模型 session，发一条让它做多步工作的 prompt（turn 运行中）；
2. 运行中 webui 用 **queue** 发一条 → 验证 turn 末/下个 turn 才进对话；
3. 再跑一个 turn，运行中用 **steer** 发一条 → 验证当前 turn 内下个安全边界即被
   模型看到、并调整行为；
4. webui composer 复核：模式切换可见、`⌘⏎` 反选、pending 待发标注与撤回；
5. `ar send --steer <sid> "..."` CLI 路径同样生效。
   `ar events` 导出 + workspace diff 归档 `qa/runs/2026-07-10-QA-45/`；webui 强刷后验。

`./scripts/check.sh` 全绿（含 vitest、lint-docs/lint-wiring：新锚点名真实 Test/QA）。

## 实施步骤

1. **INC-43.1** protocol/daemon Delivery 字段 + drainSteer + deferredInputs +
   awaitInput flush + 四条孪生 + crash 孪生——一提交。
2. **INC-43.2** CLI `ar send --steer` + webui api/composer 透传 + frontend
   Composer steer/queue 切换 + pending 标注/撤回 + vitest；rebuild dist——一提交。
3. **INC-43.3** QA-45 真机 + 文档收口（SPEC/JOURNEYS/DESIGN/CODEX-PARITY/QA/LOG
   + 工作纸归档）——一提交。

## review 裁决

小增量，inline 自审（本工作纸即三层 delta 审）。correctness 关口三个：
(1) seq 单调性（flush-backlog-on-steer 规则 + 孪生 TestSteerFlushesQueuedBacklog
钉）；(2) steer 不打断在飞 step（drainSteer 只在安全边界跑，不 cancel activity——
与 interrupt 分立）；(3) durable 幂等（Delivery 进 payload hash，孪生钉）。安全面
零放宽：interrupt/kill/权限/hooks/protected 全不动；steer 是纯追加、不越过任何
审批或安全边界。
