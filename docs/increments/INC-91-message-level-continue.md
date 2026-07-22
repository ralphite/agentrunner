# INC-91 从任一消息继续到新 session

> 状态：rev2 方案已通过独立 counter-review（Revised Go），尚未实施。本文固定
> journey/spec/design delta、风险模型与双闸验收；代码阶段需另行启动。

## 动机与 journey 锚

修订 **UJ-24 Web UI 驾驶 AgentRunner** 的会话分叉路径。用户阅读历史消息时，
应能直接从该消息所在的语义位置创建一个独立 session，而不必先理解或手选
`CheckpointBarrier`：

1. runtime 对每条已持久化的**人类用户消息**尝试建立 safe checkpoint；成功后可执行
   `Continue in new session`：新 session
   停在该消息之前，composer 预填这条消息的完整已记录内容；用户可编辑，只有再次
   点击 Send 才把它作为新 session 的下一条输入提交。
2. 对每次 agent loop 的**最后一条可见 final assistant message**执行该动作：
   新 session 停在这条回答之后，composer 为空且自动 focus，等待用户输入下一条消息。
3. 图片、文件、attachment-only message、long-paste 折叠文件与普通文本具有相同语义；
   fork 后的预填 attachment 可见、可删除、可追加，reload 后仍能恢复。
4. parent session、journal 与 workspace 完全不变；child 使用自己的 journal、artifact CAS
   与 workspace。浏览器 Back 可回到 parent。

这里的“最后一条 assistant message”严格指：**当前打开的 conversational session
自身一次 generation 中，不带待执行 tool call、会结束本轮的可见
`AssistantMessage`**；normal final 与 blocked final 可用。带 tool call 的 assistant、
handoff tool-call message、tool result 后直接 generation-limit 的旧 assistant、partial/reasoning、
program input、driver summary、peer merge row、approval/control row均不可用。queued input、active
background handle/timer 不会取消刚结束 answer 的资格，因此 anchor 不能依赖“随后是否进入
quiescence”。空内容 assistant 没有可点的消息行，也不显示 action。

scope 覆盖任何在 Web UI 中**直接打开**且自身 journal 有 message anchor 的 conversational
session，包括只读 sub-session；从 sub-session 继续时，新 child 提升为 top-level session，并从
sub-session 自己的 spec/workspace snapshot 继承。parent timeline 内的 peer merge row 仍不可点，
用户需先打开对应 sub-session，避免在错误 journal 上解析 item id。driver/series 不属于本功能。

本增量延续现有 UJ-15 的 checkpoint/fork 能力，但把日常入口从“先选 checkpoint”提升为
“点消息”；`Advanced → Continue in new session…` 仍保留给需要手选 checkpoint 或检查
workspace cut 的高级场景。

### 产品交互契约

| 点击对象 | child 的 event/workspace cut | child 初始 composer | 自动执行 | parent |
|---|---|---|---|---|
| 人类 user message | 该 `InputReceived` 之前的 anchored barrier | 预填该 event 的已记录 text + images + files | 否；Send 后才 journal/运行 | 不变 |
| loop-final assistant message | 紧跟该 final answer 的 safe-boundary anchored barrier | 空，自动 focus | 否；等待下一条用户输入 | 不变 |
| 非 final assistant / tool / control / peer / driver / optimistic row | 不可用，不显示 action | — | — | — |
| 无 anchored barrier 的 legacy message | 不可用，不伪造 workspace 历史；仍可用 Advanced | — | — | — |
| snapshot backend unavailable/本次 snapshot 失败 | 消息照常运行但无 anchored cut | 不显示 action | 不影响聊天 | 不变 |

### UI/UX pre-implementation review

- **沿用现有模式**：在 message hover/focus action row 里复用当前 `Copy` 的尺寸、间距、
  tooltip 与 keyboard focus 规则，加入 `GitFork` 图标和 accessible name
  `Continue in new session`；不新增常驻卡片、checkpoint modal 或解释页。
- **尾部动作**：最后一条 eligible assistant 的 bottom action row 与 Copy 一样常驻；历史
  中间消息仍只在 hover/focus（mobile 为现有 tap/focus-within）时显示，避免 timeline
  每行增加永久噪音。
- **直接动作**：点击后显示该 action 的局部 pending/disabled 状态，创建成功才 route/select
  child；user-message fork 预填并 focus，assistant-message fork 清空并 focus。
- **内容保真**：预填使用 parent journal 中已经 redacted、long-paste-folded 且 CAS 化的
  canonical content，不回捞浏览器原始上传、不重新暴露 secret，也不把 file 当纯文本展开。
- **错误恢复**：创建失败留在 parent、保留可重试 action，并给出 inline/toast error；不能先
  跳到一个半创建 child。双击、网络重试或 response 丢失只得到同一个 child。
- **导航**：child 进入现有 session route/selection；Back 返回 parent。全局 Advanced fork
  入口与 recovery actions 不移动。
- **可访问性**：icon button 有稳定 label、tooltip、focus ring 与最小触控面积；pending 时
  `aria-busy`，失败文案可被 live region 读出。

### 明确不做

- 不从任意 raw event seq、tool call、partial output 或未经 checkpoint 的视觉位置 fork。
- 不支持 driver series，也不从 parent timeline 的 sub-agent/peer merge row 直接 fork；直接打开
  sub-session 后可从它自身 eligible assistant message 继续到新的 top-level session。
- 不回填 feature 上线前缺少 message anchor 的 legacy message。
- 不把 composer 的后续逐字符编辑持久化；只持久化初始 fork draft，首次成功 send 后消费。
- 不改变普通 `ar fork` 的运行语义或删除手动 checkpoint 选择器。
- 不复制 parent 在 fork cut 之后的 inbox、goal verification、schedule tick、workspace edit 或
  event。

## Spec delta

### JOURNEYS.md

修订 UJ-24 当前“routine Continue/Fork 只在 Advanced”步骤：

- message action row 提供 message-scoped `Continue in new session`；
- Advanced 仍承担 raw checkpoint 选择；
- 明确 user/assistant 两种 cut 与 composer 行为；
- 明确 multimodal、reload、idempotent retry 与 parent 不变。

### SPEC.md

在 Web UI turn 收尾 / fork 功能点拆出并登记以下可独立验收的子项：

1. **message anchor**：每条可继续的人类 `InputReceived` 有 before-user barrier；每个
   loop-final `AssistantMessage` 有 after-assistant barrier，UI 只按 anchor 判定 eligibility。
2. **message-scoped fork API**：客户端只提交 `parent_session + item_id + request_id`；服务端
   authoritative resolve exact event seq/barrier，不接受客户端指定任意 barrier。
3. **dormant child**：message fork 创建后不 auto-resume；active goal/schedule 在 child 明确
   paused，copied timers/handles 被取消，直到用户 Send。
4. **durable multimodal fork draft**：user-message fork 的 recorded text/images/files/name/order
   信息进入 child genesis draft，child reload 可恢复，首次成功 send 后不再恢复。
5. **attachment-only send**：composer/Web API/CLI-runtime path 接受“text 为空但至少一个合法
   attachment”的输入。
6. **message row action**：desktop hover/focus、mobile tap/focus-within、tail assistant 常驻；
   loading/error/a11y 与现有 Copy action 一致。
7. **idempotent creation**：同一 `request_id` 无论双击、timeout retry 或 response 丢失都解析为
   同一 child；不同目标复用 request id 明确 conflict。

实施前全部标 `❌`；完成时每条分别挂 A 闸 `Test*`/frontend test 与 QA-82，禁止用
“INC-91”叙述代替可执行锚。

## Design delta

预计修订 `DESIGN.md` 的 conversation input、checkpoint/fork、event schema、Web UI product
surface 与 crash matrix 章节。

### 不变量裁决

**不改变既有不变量；实现必须在现有不变量内收敛。**

| 既有不变量 | INC-91 的保持方式 |
|---|---|
| fork/rewind 唯一合法目标是 `CheckpointBarrier` | message 先解析到带 anchor 的 barrier，再以该 barrier 的 exact `Seq` 调用 fork；绝不按任意 message seq 截 journal |
| journal-inputs-first | 外部输入仍先成为 durable mailbox command；before-user barrier 是消费前 checkpoint，Send 后仍由 `InputReceived` 作为第一个“已消费”事实 |
| blob-before-event | attachment bytes 先进入 parent/child CAS，引用它们的 `InputReceived`/`ForkedFrom.Draft` 才可见 |
| fork 有独立 workspace | child 从 anchor 的 `SnapshotRef` 物化自己的 worktree；绝不共享 parent root |
| fork cut 的 copied events 保持 provenance，active handle 在 child cancel | 普通 copied payload 不改写；message-specific draft/anchor 是 child genesis 的附加 provenance；handle disposition 沿用现有 cancel |
| quiescent 固定顺序 auto-publish → barrier → goal_verify | message anchor 是 generation final 后的普通 safe-boundary barrier；既有 quiescent sequence 原样保留，三个 slot 不插入、不重排 |
| snapshot 不可用时其余功能继续 | message barrier 继续 best-effort；缺 snapshot 的消息正常运行但无 Continue action，不把 fork 能力升级为聊天可用性的硬依赖 |
| 新 session 对外可见前必须有合法 genesis | 整个 staged session directory 的 atomic rename 是唯一 visibility commit；半成品不可 resolve/list |

需要改变的是**事件字段与 barrier 生成频率**，不是上述语义：新增向后兼容的 optional anchor/
draft metadata；旧 journal 继续可 fold，只是没有 message action。

### 1. Canonical message identity 与 anchor

`InputReceived.ItemID`、`AssistantMessage.ItemID` 是 UI/API 的 canonical message identity。
扩展 `CheckpointBarrier`：

```text
MessageAnchor {
  side: "before_user" | "after_assistant"
  item_id: string
  turn_id: string
}
CheckpointBarrier.message_anchor?: MessageAnchor
```

- `BarrierID` 只用于显示/legacy CLI；message API 以 authoritative fold 得到的 barrier
  `Seq` 为准，彻底避开重复 `bar-final` 的 last-wins 歧义。
- `{side,item_id}` 在一个 parent journal 中唯一。相同 anchor 的 retry 返回既有 barrier；
  如果同一 item 出现两个不同 seq 的 anchor，resolver fail closed 并报 journal corruption。
- `before_user` barrier 必须位于目标 `InputReceived` 之前，且二者之间只允许本次输入的
  intake bookkeeping；`after_assistant` barrier 必须紧跟 eligible final `AssistantMessage` 的
  safe boundary，不要求 session 随后 quiescent。二者都必须携带 materializable snapshot ref。
- timeline action 只有在 server/client fold 同时能建立上述唯一映射时显示；后端再次解析，
  不能信任 UI 的布尔值。

### 2. Opening input 统一为 durable mailbox input

当前新 session 同时把 opening prompt 放入 `SessionStarted.Prompt` 和 `InputReceived`，并在
`SessionStarted` 后直接 ingest；crash recovery 通过前者补写后者，却没有 durable opening
command identity。本增量把**可执行输入**统一到 mailbox，同时保持旧 title/consumer 兼容：

1. `ar new`/daemon/Web UI 先分配稳定 `CommandID/TurnID/ItemID`，把 opening text + bytes
   作为 durable mailbox input 保存；直接 CLI 也走同一 durable seam。
2. `SessionStarted.Prompt` 暂时保留为 redacted display/title hint，并新增 optional
   `OpeningCommandID/OpeningItemID`（或等价 marker）；它不再是带 marker 新会话的 executable
   recovery source。旧 journal 无 marker 时继续走现有 Prompt fallback。
3. `SessionStarted`、effective startup mode、frozen tools/inputs 等 bootstrap
   facts 落盘并完成 materialization 后，创建 `before_user` barrier，再走统一
   `journalInput` intake（hook/command expansion/redaction/CAS/InputReceived）。
4. session title fallback/auto-title 继续可读 `SessionStarted.Prompt`；conversation assembly 永远
   只读 `InputReceived`。带 marker 的 resume 从 mailbox 重放，绝不再次从 Prompt 合成 input。
5. crash 在 mailbox durable 后、`InputReceived` 前发生时，resume 重放相同 command identity；
   `ensureMessageBarrier` 按 item id 复用或补齐 barrier，不丢 input、不重复消费。

迁移矩阵必须逐项测试：daemon-hosted `ar new`、foreground `ar run`、Web New chat、direct CLI、
legacy no-marker resume；spawned child/driver 未接 message-continue 时继续 legacy opening path，不能
被全局修改 `Loop.Run` 静默改变。后续若要给 spawned child 的 opening input 也提供 user action，
另行把它迁入 durable mailbox。

如此第一条与后续 user message 使用同一协议，不需要在 fork 时改写 copied
`SessionStarted`，也不破坏普通 fork 的 byte-identical copied payload 约束。
从第一条 user message 之前 fork 时，child 可继承该 non-conversational display hint/title，但模型
上下文与 runnable input 中没有原消息；composer draft只来自目标 `InputReceived`，用户编辑后发送
的内容才会进入 conversation。

### 3. User input intake 的 before barrier

把所有**会形成可见 human `InputReceived`**的路径收束为一个
`ingestHumanInputWithBarrier` seam：opening、idle input、`drainQueued`、`flushDeferred` 与
mid-turn steer batch 均逐条调用。

每条消息的顺序：

```text
durable mailbox command
  → dedup/revoke/forward 分类
  → ensure before_user barrier(item_id, turn_id)
  → UserPromptSubmit hook
  → command expansion + redaction
  → attachment CAS put
  → InputReceived
  → command handled/high-water advance
```

- revoke、forward、interrupt、machine/program/peer input 不生成 message action；它们走现有
  非 human path。
- hook veto 可能留下一个没有对应 `InputReceived` 的 orphan anchor；它不可解析为 action，
  retry 按 item id 复用，允许后续 GC 但不要求本增量清理。
- queued batch 中每条消息前都有独立 barrier。第二条的 cut 包含第一条已经 journaled 的
  message，但不包含第二条；即使 workspace snapshot ref 因无改动而内容寻址去重，barrier
  vector/seq 仍不同。
- message snapshot 失败/`SnapshotStore` unavailable 时沿用既有 graceful degradation：消息照常
  journal/运行，但本条没有 anchor，因此不显示 Continue；记录可诊断的内部 reason/metric，不
  把 rewind backend 故障升级为发送失败。
- snapshot 创建成功而 barrier append 前 crash 只留下可回收 CAS；barrier append 后 crash
  由 item-id dedup 复用该 barrier。

### 4. Loop-final assistant 的 after barrier

after anchor 是 **final-generation safe boundary**，不复用也不替代 `quiescentBarrier`：

1. provider result 收齐后，runtime 先以统一 classifier 判断：message 可见、无待执行 tool call，
   且 finish 为 normal final 或 blocked final；其他 finish/tool-call shape 不 eligible。
2. eligible 时先从当前 prefix fold捕获 `snapshot_ref + child vector + handles`，作为 optional
   `AssistantMessage.ContinuationCheckpoint` 与 assistant event 一起持久化；main stream 的 vector
   `"."` 在 barrier 时固定为该 AssistantMessage 的实际 seq。snapshot failure则照常落 assistant、
   无 action。
3. 正常路径在 AssistantMessage 后立即追加带 `{after_assistant,item_id,turn_id}` 的
   `CheckpointBarrier`。AssistantMessage append 后、barrier append 前 crash 时，resume/boot 读取
   journal 后的**第一条 append 必须是 repair barrier**：在 in-doubt settlement、timer/goal
   reconciliation、mailbox drain、任何 hook/observer fact 之前执行。repair fold只到该 assistant
   prefix，使用 receipt 中当时捕获的 child vector/handles，并令 `vector["."] = assistant.Seq`；不读
   crash 后 workspace重拍。已有唯一 anchor时幂等跳过。
4. queued input、active handles/timers 不影响此 checkpoint，因此每个真正 final answer 都可覆盖。
   background handles 进入 barrier disposition，message fork 时取消。
5. 既有 quiescent sequence 仍独立执行 `auto-publish → bar-final → goal_verify`；message cut 在它
   之前，因而不声称包含后续 auto-publish/goal_verify events。goal miss 后的新 loop 产生自己的
   final-message anchor。

`ContinuationCheckpoint` 只是 crash-recovery receipt，不是合法 fork target；唯一合法 target
仍是补齐后的 `CheckpointBarrier`。snapshot 必须先于引用它的 assistant event（blob-before-event），
snapshot 成功但 assistant append 失败只留下可回收 orphan；GC 在 receipt 未 reconcile 为 barrier
前也必须把该 ref 视为 pinned。

### 5. Message-scoped fork request 与 resolver

新增 core/service operation（命名暂定 `ContinueFromMessage`）与 Web endpoint：

```text
POST /api/sessions/{parent}/continue-from-message
{
  "item_id": "item-...",
  "request_id": "client-generated stable id"
}

201/200 {
  "session_id": "...",
  "source_item_id": "item-...",
  "draft_id": "..." | null,
  "draft": { "text": "...", "images": [...], "files": [...] } | null
}
```

后端事务：

1. 锁定/读取 parent 当前 journal，按 item id 找唯一 message event；验证它是 human user 或
   loop-final assistant。
2. 按 `{side,item_id}` 找唯一 anchored barrier，校验 event ordering、snapshot ref、workspace
   root 与 conversational session 类型；nested session 用自己的 resolved dir/journal，结果仍创建
   为 top-level child。
3. 使用 barrier 的 exact `Seq` 生成 cut；绝不把 client barrier id 传给 `ar fork` 再做
   last-match。
4. user target 从**目标 `InputReceived` event 本身**构造 recorded draft；assistant target
   draft 为空。
5. 校验 bounded、全局唯一的 request id（长度/字符集），以 **request id 单独作为 registry/lock
   key**，并让 payload hash 覆盖 `parent + item_id`。相同 request 重试返回
   同一 child；同 request 指向另一 parent/item 返回 `409 conflict`。
6. child 不启动 agent process。完成 staged publish 后才返回并允许 UI route。

CLI 可新增内部 JSON-safe command 面（例如 `ar continue-message <sid> --item <id>
--request-id <id> --json`），但 Web handler 不拼 shell 文本；参数必须经 argv/core API 传递。
普通 `ar fork <sid> <barrier>` 保持兼容。

### 6. Child genesis、draft 与 dormant 状态

扩展 `ForkedFrom` 的 optional metadata（旧 decoder 忽略缺省）：

```text
request_id?: string
source_item_id?: string
source_side?: "before_user" | "after_assistant"
draft?: ForkDraft {
  draft_id: string
  text: string
  content: ordered ref-only parts
  images: AttachmentRef[]
  files: AttachmentRef[]
}
```

`AttachmentRef`、`protocol.ContentPart`、legacy `ImageAttachment/FileAttachment`、需要持久化 order/name
的 `provider.Part` 与 UI/API DTO 全链新增 optional `name/part_id`；旧记录 fallback 为 media type +
ref 短 hash。filename 是不可信显示数据：去路径、控制字符并限长；重复同 ref、不同 name 的 part
仍是两个条目。draft 只含已 journaled/redacted 的 text 与 CAS ref，绝不含 raw bytes。

message fork 在 copied cut/handle cancellation 后追加 child-only dormant normalization：

- 新增 `ForkAwaitingInput{request_id,draft_id?}`（命名可调整）作为**独立 durable park**；它不伪装
  `WaitingEntered`，不覆盖 cut 中可能尚未闭合的 generation/ask/wait。
- state/inspect/list 把该 park 投影为 `waiting:input`；boot stranded sweep、timer sweep、goal/schedule
  自动路径全部跳过。只有显式 human Send 可以解除。
- active goal → `GoalPaused{source:"message_fork"}`；active schedule →
  `SchedulePaused{source:"message_fork"}`。
- 为**全部** pending timers 追加 `TimerCancelled`，包括 schedule/self-wake 与 activity timeout；所有
  copied background handles 继续走现有 fork cancellation。publish 前最终 fold 必须满足
  `handles=0,timers=0,automation paused,fork park active`。
- 第一条成功 journal 的 human `InputReceived` 原子解除 fork park；opening/mid-turn steer cut 会从
  原有未闭合 state 加上新 input 再交给 `decide`，而不是在 child 可见时偷偷续跑旧 generation。
  hook veto/revoke/crash 未形成 `InputReceived` 时 park 保持。

普通 Advanced fork 不采用该 park/pause policy，保持既有语义。

### 7. 原子创建与 blob-before-genesis

现有 `fork.Cut` 先写 journal、后复制 `artifacts/`；当 genesis draft 引用 attachment 时会产生
“event 已可见、blob 尚未复制”的窗口。message fork 必须重构为 staged publish：

1. 以 **request id hash** 取得跨进程 `flock`；在同一 request-id key 的 durable registry 原子创建
   `{request_id,payload_hash(parent,item_id),child_id,state}`。child id 首次 reserve 后固定；同
   request 异 payload 直接 409，不能因 parent/item 不同而落入另一把锁。
2. registry 状态至少为 `reserved → workspace_ready → session_published`。每次 retry 在锁内以
   final child genesis 的 tuple 为最终真相，可修复“directory 已发布、receipt 尚未更新”的窗口，
   绝不再分配第二个 id。
3. 在同一 filesystem 的 `sessions/.staging/<request-hash>/` 创建整个 child staging directory；
   list/resolve 明确排除 `.staging`，最终 rename 才进入合法 session-id namespace。从 barrier
   snapshot 物化 child workspace 到 temp sibling，校验后 rename 到最终独立 root。final workspace 可先存在，但没有
   published session journal 时不可被产品发现，只是可识别 orphan。
4. publish 前 pin barrier 以及 cut 内继承的全部 snapshot refs。`artifacts/` 作为 immutable CAS
   先复制/校验；每个 draft ref 必须存在且 hash/media metadata 匹配。
5. `sub/` 不对 live JSONL 做裸 `os.CopyFS`：逐 stream 通过 EventStore 的一致读取 seam 取得完整
   envelopes，至少覆盖 barrier vector，并写入 staging；vector 之后读到的完整 suffix 仍按既有
   DESIGN 作为 harmless provenance 保留，torn line 一律失败重试。
6. 在 staging 中写齐 inbox watermark 与完整 journal：单一 `ForkedFrom` genesis、exact cut、
   handle/timer cancellations、goal/schedule pause、`ForkAwaitingInput`；fold 校验 genesis、barrier、
   draft refs、workspace root、无 handle/timer 且 park active。
7. fsync 后以**整个 staging session directory rename 到 `sessions/<child_id>`**作为唯一 visibility
   commit；随后把 registry 标 `session_published`。crash 在二者之间时，retry 用 final genesis
   修复 receipt并返回同一 child。
8. 任一阶段失败都不暴露可 list/resolve 的 session；重试只继续/回收自己 request 的 staging，
   不删除 parent、其他 request 或已发布 child。registry corruption 必须 fail closed，不降级为
   “再创建一个”。

普通 fork 也应复用重构后的 staged copier，至少保证 side stores 在 journal 可见前就绪；这是
修复新 draft 暴露出的既有写序风险，不改变它的用户语义。

### 8. Draft 恢复与首次 Send

- child fold 暴露 pending `ForkDraft` 与 `ForkAwaitingInput`；标准 child session read payload 返回
  draft，只允许该 child 自己 genesis 中授权的 refs。
- `Composer` attachment model 支持两种来源：新 upload path 与 durable child CAS ref；显示、
  remove、preview/download 统一，不能把 parent URL 继续绑在 child 上。
- user-message fork route 后以 `draft_id` 做 one-shot seed。reload 或换设备打开 child 且尚无
  consuming input 时，从 server rehydrate；不会依赖 React memory/localStorage。
- Send body 在 pending draft 时总带 `draft_id + send_request_id`，即使用户已经改写 text/移除
  全部 seeded attachment。server 只允许引用 draft 白名单内的 CAS ref；新 upload 走既有 upload
  validation。任意其他 ref 返回 400，防止把任意 child CAS blob 注入下一次模型输入。
- pending draft 不另建先于 inbox 的 claim store。完整 `CommandInput` 本身扩展
  `ForkDraftAttempt{draft_id,send_request_id,payload_hash}`；它已携带完整 text/content bytes/ref
  selection 与稳定 command id。在 child inbox 的同一 file lock/atomic append 下，检查该 draft
  没有未 settle attempt并把**完整 command append 作为唯一 durable claim**：append失败即没有
  claim，append成功后的任意 crash都由 CommandLog重放。exact retry返回同一 command receipt；
  另一 tab 的不同 attempt在前一 attempt未 settle时得到409。
- server 读取经授权 ref bytes并交给现有 input/CAS seam（相同内容 hash 去重）；成功落下的
  `InputReceived` 记录 optional `ForkDraftID/BasedOnItemID`，同时解除 fork park、消费 draft。
  hook veto 必须新增/复用带 `CommandID`、推进 consumed high-water 的 durable `InputRejected`/release
  fact；`InputRevoked` 同样 settle attempt并释放 draft。daemon crash 保留完整 inbox command并重放；
  不存在“claim 已落但 command 不存在”的状态。没有形成 InputReceived 的失败不能消费 draft。
- CLI/其他 surface 在 pending fork park 上发送无 `draft_id` 的第一条 human input时，同样解除
  park，并把 draft durable 标为 `discarded_by_new_input`，reload 不再恢复旧 seed。
- text 为空但合法 images/files 非空是有效输入；text 与 attachments 都为空仍拒绝。
- 未编辑、未删 part 时按 recorded `content[]` 原 order bit-for-bit replay。现有 composer 仍以
  单 textarea + attachment strip 展示；一旦用户编辑 text/选择 attachment，就 canonicalize 为
  “编辑后单一 text part + 剩余 attachment 原相对顺序”。此规则显式覆盖 interleaved legacy
  content，不能暗称任意编辑后仍保留原 interleave。
- child image/file read 必须校验 ref 对当前 journal/draft 可达；file 下载使用 sanitized
  `Content-Disposition: attachment` 与 `X-Content-Type-Options: nosniff`。

### 9. Frontend data flow

1. `timeline.ts` fold 暴露 durable `turnId/itemId`，并从 anchored barriers 导出
   `continueSide`; 不通过“它是最后一行”猜 assistant eligibility。
2. `Timeline.MsgActions` 接收完整 bubble/continue callback，不再因 `text==""` 而整行返回
   null；attachment-only user bubble 也有 Copy 以外的 Continue action。
3. `TimelineView` 在普通 row、retry/work fold 与 tail action row 传递同一 canonical item；
   parent timeline 的 peer/driver/program rows无 callback；直接打开 sub-session 后按它自己的
   anchors 渲染。
4. `SessionView` 持有 request-scoped pending/error，调用 message endpoint；成功后 select child，
   **只通过标准 child session/state read** 读取 effective spec/access at cut，不能从 endpoint 与
   parent localStorage 各维护一份，也不能把 parent 最新 setting 误写到历史 cut。
5. 复用 Project New chat 的 request-id seed/focus pattern；selection 完成后再 focus，避免
   remount 丢 draft。child attachments 的 image/file URL 一律使用 child session id。
6. attachment-only 的所有 UI guard 统一为 `hasContent = text.trim() || attachments.length`，覆盖
   Send disabled、Enter、⌘Enter、reset 与 error rollback；API 失败必须恢复 text 和 attachment。

## 风险与 race matrix

| 场景 | 必须结果 |
|---|---|
| 新 user message 因 snapshot failure 无 before barrier | message 正常运行；该行无 action并留诊断，不能阻断聊天 |
| assistant event 已落盘，进程在 after barrier 前 crash | resume 第一条 append 用 event receipt 的 snapshot/vector/handles 补 anchor；不得先 settle/reconcile或重拍 |
| crash 在 snapshot 后、barrier append 前 | input command 未消费；retry 可新建/去重 snapshot，无重复 visible message |
| crash 在 barrier 后、InputReceived 前 | 相同 item id 复用 barrier，再消费同一 mailbox command |
| queued/steer batch 有 N 条 user input | N 个顺序 before anchor；第 k 个 child 包含前 k-1 条，不含第 k 条及之后 |
| hook veto / revoke / forward | 不生成 visible action；orphan anchor 不被 resolver 采用 |
| 两次快速点击同一 action | 相同 request 只创建一个 child；按钮 pending 禁用只是 UX，server idempotency 才是 correctness |
| response 在 child commit 后丢失 | retry 返回既有 child，不再创建第二个 |
| 两个 Web 进程同时创建同 request | cross-process request lock + payload hash 只发布一个 final child dir |
| 同 request id 攻击性复用到另一 item | 409，不返回已有 child 内容 |
| parent 在 fork 解析后继续写 events/workspace | child 固定在已验证 barrier seq/snapshot；不混入后续内容 |
| draft 引用 blob 尚未复制时 crash | journal 未 publish，child 不可 resolve/list |
| seeded ref 被客户端替换为任意 CAS hash | server 因不在 draft whitelist 拒绝 |
| opening/mid-turn steer cut 看起来 running | `ForkAwaitingInput` 覆盖 boot/decide 自动路径；显式 Send 前零 LLM call |
| child 有 active goal/schedule/activity timer | pause/cancel facts在 publish 前写齐，最终 fold 零 timer/handle，不 auto-run |
| attachment-only draft | bubble/action 可见，composer 可发送；空 text + 零 attachment 才拒绝 |
| reload before first Send | 从 child genesis 恢复 draft；成功 Send 后 reload 不恢复 |
| 两个 tab 同时发送 pending draft | first durable attempt claim wins；exact retry复用，不同 payload/attempt 409；veto/revoke后释放 |
| crash 在 draft claim 与 inbox 之间 | 此窗口不存在：包含完整 payload 的 CommandLog append 本身就是 claim |
| filename 缺失的旧 attachment | 生成稳定 fallback，不阻断 fork |
| snapshot store unavailable | 不伪造 continue；message 正常运行但无 action，Advanced 同样按现有能力显示 |
| live parent sub-stream 正在 append | 经 EventStore 一致读取复制完整 envelope；不复制 torn JSONL，至少覆盖 barrier vector |

## 验收

### A 闸：单元 / 集成 / scripted 孪生

计划新增或扩展以下可执行锚；最终以真实测试名回填 SPEC：

#### Anchor 与 runtime

1. `TestHumanInputBarrierPrecedesEveryMessage`：opening、idle、queued、deferred、steer 每条均有
   唯一 before anchor，且 exact cut 不含目标 input。
2. `TestQueuedInputBarriersPreservePrefix`：三条 batch 的第 2 个 cut 含第 1 条、不含第 2/3 条。
3. `TestInputBarrierCrashRetryReusesAnchor`：barrier 后 crash/replay 不重复 barrier/input。
4. `TestInputBarrierSnapshotFailureDoesNotBlockInput`：snapshot failure 仍落 input/high-water，但无
   message anchor/action。
5. `TestFinalAssistantSafeBoundaryAnchor`：normal/blocked final 在 queued input、active handle/timer 下
   仍有 immediate anchor；tool-call/handoff/generation-limit old assistant无 anchor。
6. `TestFinalAssistantAnchorCrashRepairIsFirstAppend`：assistant 后 crash 用 event receipt补一个 exact
   anchor；即使同时有 in-doubt handle/timer/mailbox尾巴，barrier仍是第一条恢复 append、vector 点到
   assistant prefix且不重拍 workspace；snapshot failure仍保留 final message。
7. `TestQuiescentSequenceUnaffectedByMessageAnchor`：独立证明 auto-publish → bar-final → goal_verify
   顺序未改，message cut 不声称包含这些后续 events。
8. `TestOpeningInputUsesMailboxMarkerAndLegacyPromptFallback`：hosted new/foreground run/Web/direct CLI
   走 durable opening；new marker resume不从 Prompt合成，legacy no-marker仍 recovery；title不回归。

#### Resolver / fork / crash

9. `TestContinueFromUserMessageCutsBeforeInput`：child events/workspace精确到 before barrier，genesis
   draft 来自目标 event。
10. `TestContinueFromAssistantCutsAfterLoopFinal`：包含回答，不含 queued next input、auto-publish 或
    goal_verify 后续。
11. `TestContinueResolverRejectsLegacyNonFinalAndDuplicateAnchor`：legacy/non-final/peer merge/duplicate fail
    closed，不退化成任意 seq。
12. `TestContinueRequestIsIdempotentAcrossProcessesAndCrash`：双 Web 进程、reservation 后 crash、
    directory publish 后 receipt 前 crash、response 丢失都返回同一 child；payload冲突409。
13. `TestContinueForkPublishesBlobsSnapshotsAndDirectoryAtomically`：draft ref/pinned snapshot在 child
    可见时已可读；阶段 crash 不产生可 resolve/list session。
14. `TestContinueForkCapturesLiveSubStreamsWithoutTornJournal`：一致读取至少覆盖 barrier vector，
    并发 append 不产生半行。
15. `TestContinueForkParkBlocksBootAndResumeUntilHumanInput`：opening/idle/queued/steer/assistant cut 在
    daemon restart、boot sweep 与 timer tick 下零 LLM call；第一条 human input才解除。
16. `TestContinueForkPausesAutomationAndCancelsEveryTimer`：goal/schedule paused，activity/schedule/self-
    wake timers 与 handles 全清；parent active state不变。
17. `TestContinueUsesBarrierSeqNotRepeatedBarrierID`：多个 `bar-final` 时仍选 item 对应 exact seq。
18. `TestContinueFromNestedSessionCreatesTopLevelChild`：direct-open sub-session 用自身 spec/snapshot，
    parent peer merge row不可解析。
19. 保留并扩展现有 `fork.Cut`、snapshot materialize、inbox watermark、nested fork/handle cancel
    tests，证明普通 Advanced fork 不回归。

#### Multimodal / API

20. `TestContinueDraftPreservesRecordedMultimodalContent`：text、image、file、part order、重复 ref 不同
    filename、media type、long-paste 与 attachment-only全覆盖；不含 raw bytes/secret。
21. `TestForkDraftRefAuthorization`：只接受本 draft ref；跨 child/任意 hash拒绝；file download
    强制 attachment+nosniff。
22. `TestForkDraftAttemptIsCommandLogAtomic`：完整 CommandInput append即 claim；双 tab first claim、
    exact retry、不同 payload 409、append失败无 claim、append后 crash replay、hook veto/revoke
    durable release、成功 consume/CLI discard全覆盖。
23. `TestAttachmentOnlySend`：Web/daemon/runtime canonical input path 在空 text + attachment 时成功，
    全空仍 400。
24. `TestHandleContinueFromMessage`：201/200/400/404/409、nested session、bounded request id 与 DTO。

#### Frontend

25. `Timeline.msgrow.test.tsx`：eligible user/final assistant 显示 action；non-final、peer、legacy、
    optimistic 不显示；attachment-only row仍显示。
26. `Timeline.tailrow.test.tsx`：最后 eligible assistant 的 Continue 与 Copy 常驻且引用正确 item，
    不能误用 timeline 最后一行。
27. mobile keyboard/a11y tests：touch discoverability、focus-within、label、tooltip、`aria-busy`、
    error live region。
28. `SessionView.chrome.test.tsx`：pending guard、失败完整恢复 text/attachments并留 parent、成功 route、
    Back history不被二次 select污染、assistant空 focus、user durable seed、标准 child read提供 spec/access。
29. `Composer` tests：seeded CAS attachment preview/remove/add、attachment-only 的 button/Enter/⌘Enter、
    interleaved unedited replay/canonicalized edit、reload seed 与首次 successful send consumption。

每一步仍跑完整 `./scripts/check.sh`，不得只跑定向 tests。

### B 闸：QA-82，共享真实环境

新增 `qa/run-qa82-message-level-continue.sh`，严格使用
`~/.local/share/agentrunner/` 的全局 daemon/store，测试数据与证据全部保留在
`qa/runs/<日期>-QA82-message-level-continue/`：

1. 创建真实 API session，首条 human message 含 text + image + file；等待 final assistant。
2. 从首条 user message 点 Continue：只出现一个 child，parent hash/journal/workspace不变；child
   composer 预填 text/image/file/name，agent未运行。
3. reload child 后 draft仍在；删除一个 attachment、编辑 text、保留另一个并 Send；断言 child
   新 `InputReceived` provenance、模型可见保留内容、draft不再恢复。
4. 从 final assistant 点 Continue：child 包含该 answer 和当时 workspace，排除后续 parent
   message；composer空且 focus，未 auto-run。
5. 连续向 parent queue 两条 user message，从第二条 fork；child timeline 含第一条、不含第二条，
   draft为第二条。
6. 用 attachment-only message重复 user fork/Send，证明无文本也完整可达。
7. 人为复用同 request id/模拟 response retry，证明 session id相同；更换 item id得到409。
8. active goal、schedule、background activity timer 场景 fork，等待跨过 tick，child无自动生成；
   parent继续按原状态运行。
9. 直接打开一个 sub-session，从其 final assistant fork；结果是 top-level child 且使用 sub-session
   spec/workspace。parent 的 peer merge row无 action；legacy/non-final/tool/driver row无 action，
   Advanced checkpoint fork仍可用。
10. desktop hover/focus、mobile tap、keyboard、loading/error（text/attachments完整恢复）、Back
    navigation 与 console 0
    error/warn；保存 screenshots、DOM、API responses、`ar events`、workspace diff。

破坏性 crash 注入只在 A 闸进行；QA-82 不 kill 全局 daemon。若必须验证真实 restart，先取得
用户时间窗。测试结束不 close/delete session，不清理 workspace/journal/store。

### 枚举交付物对锚

| 枚举 | 值 | 锚 |
|---|---|---|
| message side | before_user / after_assistant | A1/A5/A9/A10 + QA-82.2/.4 |
| assistant finish | normal / blocked / tool-call / handoff / limit-old-message / empty | A5/A6/A11 + QA-82.4/.9 |
| content | text / image / file / long-paste / attachment-only / interleaved | A20/A23/A25/A29 + QA-82.1/.3/.6 |
| intake | opening / idle / queued / deferred / steer | A1/A2/A8/A15 + QA-82.1/.5 |
| request outcome | create / retry-existing / invalid / missing / conflict | A12/A24 + QA-82.7 |
| child source | top-level / direct-open nested | A18/A24/A25 + QA-82.9 |
| child automation | goal / schedule / activity timer / self-wake / handle | A15/A16 + QA-82.8 |
| UI state | hidden / hover-focus / tail-persistent / pending / error-rollback / success-focus | A25–A29 + QA-82.10 |

## 实施步骤

每步均为一个可合并提交，必须 `./scripts/check.sh` 全绿后立即 push `origin/main`：

1. **INC-91.1 — message identity/anchor schema**
   扩展 optional event fields/state fold；实现 authoritative resolver 与 backward-compatible tests。
   完成标志：legacy journals全绿，重复/错序 anchor fail closed。
2. **INC-91.2 — unified human input barrier seam**
   opening input改 durable mailbox；统一 opening/idle/queue/deferred/steer；补 crash/failure tests。
   完成标志：snapshot 可用时每条新 human message 有 exact before anchor；不可用时消息仍正常运行；
   新 opening marker 与 legacy Prompt recovery/title兼容。
3. **INC-91.3 — final assistant anchor**
   final classifier、assistant snapshot receipt、immediate after barrier 与 crash reconciliation。
   完成标志：queued/handles/timers下仍锚 normal/blocked final；tool-call shapes不误锚；quiescent
   sequence tests证明未重排。
4. **INC-91.4 — atomic message fork core**
   cross-process request registry、whole-directory staged publish、snapshot pin/live sub copy、exact-seq
   cut、`ForkAwaitingInput`、automation pause与全 timer cancel；普通 fork复用 copier并回归。
   完成标志：所有阶段 crash/并发 tests无半 session/blob gap；opening/steer child 重启也零 auto-run。
5. **INC-91.5 — durable multimodal draft/send**
   genesis draft、全链 name/order、authorized CAS rehydrate、single-attempt consume/release、
   attachment-only input。
   完成标志：text/image/file/long-paste/attachment-only/interleaved全枚举通过，跨 tab/crash收敛且
   arbitrary ref被拒。
6. **INC-91.6 — Web API + timeline action**
   endpoint、timeline fold、MsgActions/tail row、SessionView route/focus/error、Composer seeded refs。
   完成标志：frontend/component/API tests全绿，Advanced入口不回归。
7. **INC-91.7 — 双闸、对抗 review 与收口**
   跑 QA-82；修完 P0/P1；delta 并回 JOURNEYS/SPEC/DESIGN/QA/GAPS，LOG追加，工作纸归档。
   完成标志：证据归档、`check.sh` 全绿、共享数据保留、文档锚全部可执行。

若任一步发现必须改“barrier-only target / journal-first / blob-before-event / quiescent fixed order /
independent workspace”任一原文，立即停止实施，按 PROCESS §四另起不变量变更段和单独契约
review；不得在代码里先绕。

## review 裁决

**必须做里程碑级三视角对抗 review，不裁。** 理由：本增量同时触及 input durability、
checkpoint 时序、fork atomicity、CAS authorization、goal/schedule concurrency、Web navigation 与
multimodal composer；任一“看起来能用”的 happy path 都不足以证明 crash/race/contract 正确。

实施前先做一次独立 counter-review，要求按 P0/P1/P2 给出**具体失败场景 + 必须修改的方案**，
至少覆盖：

- 正确性：user/assistant exact cut、opening prompt、batch prefix、final-message判定、legacy。
- crash/并发/安全：barrier retry、fork publish窗口、idempotency、CAS ref authorization、parent并写。
- 产品契约：click/route/focus/reload/back、mobile/a11y、draft edit/consume、automation dormant。
- DESIGN 契约：不变量是否真的未改，特别是 journal-first 与 fixed quiescent sequence。

P0/P1 必须在本文修订后才能进入 INC-91.1；P2 要么纳入，要么在 review 结果中写明接受理由。

### 独立 counter-review 结果（2026-07-21）

两名只读 reviewer 对初稿均给出 **No-Go**。rev1 吸收全部首轮 P0/P1；不是把 finding 降级：

| finding | 初稿失败场景 | rev1 裁决 |
|---|---|---|
| P0 assistant 只绑 quiescence | queued input/handle/timer 使 final answer 不进 quiescent actions | 改为 immediate final-generation safe-boundary anchor；assistant event带 prepared snapshot receipt，quiescent sequence独立不动 |
| P0 child 不真正 dormant | opening/steer cut fold 为 running，boot sweep 自动继续旧 generation | 新增独立 durable `ForkAwaitingInput` gate；boot/decide/timer/goal均尊重，只有 human InputReceived 解除 |
| P0 snapshot fail-closed 违约 | nested git/backend unavailable 会让所有消息发送失败 | 恢复 best-effort：消息正常运行、该行无 action，保持 DESIGN graceful degradation |
| P0 staged publish/idempotency 不闭合 | final dir/journal 与 receipt 之间 crash 可能暴露半 child/重复 child | cross-process request registry + payload hash + whole staging dir rename；final genesis 可修 receipt |
| P1 opening Prompt 迁移漏消费者 | Resume/title/auto-title/spawn child 依赖 Prompt | 保留 Prompt 为 display hint，新增 durable opening marker；只迁 top-level入口，列 legacy/title矩阵 |
| P1 final classifier 含糊 | handoff/tool-call/blocked/limit shape 会误锚 | 枚举 normal/blocked eligible；tool-call/handoff/limit-old/empty 不 eligible并逐项测试 |
| P1 draft 首次 Send 有双 tab race | 两 tab 都过 whitelist、都入 inbox | durable single-attempt claim；exact retry复用，冲突409，veto/revoke release，crash replay |
| P1 live sub/timer/snapshot ref 不完整 | `os.CopyFS` 可能拷 torn child journal；activity timer自唤醒；refs发布后才 pin | EventStore 一致读取、publish 前 pin、全部 timer cancel、最终 fold 零 timer/handle |
| P1 multimodal name/order 不闭合 | 只改 AttachmentRef 无法保 filename/interleave | 扩全链 schema；定义 duplicate ref、legacy fallback、未编辑 exact replay与编辑后 canonicalization |
| P1 sub-session scope 含糊 | “每个 agent loop”却单方面排除只读 sub-session | direct-open conversational sub-session 可继续并提升 top-level；parent peer merge row仍排除 |

reviewer 的 P2 也已纳入：统一 `hasContent`、send error 恢复 attachments、Back history 测试、child
spec/access 单一 read source、mobile touch discoverability。方向正确且保留的部分包括 exact barrier
`Seq`、legacy fail-closed、parent 不变、draft ref whitelist 与 blob-before-genesis。

rev1 复核仍指出 3 个 P1；rev2 进一步固定：(1) `request_id` 是唯一 registry/flock key，
parent/item 进入 payload hash；(2) 完整 `CommandInput` 的 atomic CommandLog append 就是 draft claim；
(3) prepared assistant barrier repair 是 resume 后第一条 append，vector 锁定 assistant prefix。独立
reviewer 最终裁决 **Revised Go**，无剩余 blocker。

**本文现在可作为实现输入；实施收口仍必须再做一次三视角 review。**
