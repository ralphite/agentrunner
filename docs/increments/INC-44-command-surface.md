# INC-44 命令面设计单元「命令身份·撤销·应答」（HANDA 2U：#16/#29/#7）

**状态：✅ 设计定稿（2026-07-11 契约 review「修订后放行」，rev1 已
吸收全部前置条件——见 §review 记录）。#29 触 DESIGN §2 铁律，实施时
DESIGN 修订与实现同 commit（PROCESS §四）。**

## 动机与 journey 锚

三项 HANDA 对照缺口同改 `protocol.SessionCommand` / daemon dispatch /
消费循环一片区（方案 review 的 D1/D2 裁定合并设计，防三趟改同处语义
打架）：

- **#16 turn retry**（UJ-02/24）：失败/中断轮一键原输入重发。
- **#29 排队消息撤销**（UJ-07/24）：忙时排队的消息可撤回/可编辑。
- **#7 结构化 ask_user**（UJ-06/24）：多问+选项表单式提问与应答。

## 代码事实基线（2026-07-11 核对）

- 命令种类 `input|control|interrupt|close|kill|approval`
  （protocol/input.go:119）；`AppendCommand` 按 command_id 幂等
  （同 id 同载荷回原 receipt，异载荷拒绝；inbox.go:67）；
  `ReadInbox` 只滤 `input` 种类；消费 high-water =
  `ConsumedInputSeq`（fold）。
- ask 面（INC-5）：一个 ask_user call = 一次 `WaitingEntered{input}`
  park；应答 = park 期间一条 send 经 `AskResolved{Answer string}` 配
  对为 tool result（不落 InputReceived）。
- `UserInput` 已带 `Content []ContentPart`（typed ingress 权威）与
  Images/Files（CAS ref 形态）——retry 重发可直接复用 journal 里的
  ref（blob 在同 session CAS）。

## 设计

### A. `revoke` 命令与消费语义（#29 · 触 §2 · 走 §四）

1. **协议**：新命令种类 `CommandRevoke = "revoke"`，载荷
   `Revoke{TargetCommandID string}`。与其它命令同 durable
   （AppendCommand fsync-then-ack、幂等、跨 restart 重放）。
2. **daemon 前置校验（rev1：UX 优化非安全边界）**：目标 command_id
   不存在 / 种类非 `input` / 已消费 → 同步报错不落账（"already
   being processed"）。已消费判定复用 pendingApproval 式**全量
   ReadEvents+Fold**（daemon 不持有 live fold）；在飞输入的 TOCTOU
   窗由 §A.3 消费守卫兜底——本校验只为尽早拒绝。
   interrupt/approval/close/kill/control 永不可撤。
3. **消费守卫（rev1，照抄 AskResolved 三件套）**：live 稳态的消费
   是 channel 逐条（daemon pump 逐条 forward、loop drainQueued/
   awaitInput 读 `UserInputs` channel），**不存在**"pending 批按 seq
   配对"——机制改为：
   - revoke 经 pump 进 loop 的专用通道，loop 维护 **revoked-target
     集**；`journalInput` 消费每条输入前查集，命中 → **落
     `InputRevoked{TargetCommandID, DeliverySeq}`**（带被撤输入的
     DeliverySeq！）而非 InputReceived；
   - **fold 加 `InputRevoked` 分支推进 `ConsumedInputSeq`**（镜像
     state.go AskResolved 分支——"消费了一条输入但不落
     InputReceived 仍推 high-water"的既有模板）；
   - **resume mailbox 重放**（loop.go:702 唯一按 seq 读 inbox 文件
     的点）改读 `ReadCommands`（或先建 revoked 集再滤），不再用只
     滤 input 的 `ReadInbox`——重放先跳被撤再注入；
   - revoke 晚于消费（目标已落 InputReceived）→ no-op（迟到审批
     先例 DESIGN §437-438 同族）。
4. **幂等**：重复 revoke 同目标（新 command_id）在 daemon 校验处
   遇"目标已撤/已消费"报错或 no-op，皆无害。
5. **UI**：webui queued 气泡加 撤回 / 编辑（=撤回+预填 composer
   重发）；CLI `ar queue <sid>`（列 pending input+撤回态）+
   `ar unqueue <sid> <command-id>`。

**§四变更单（DESIGN §2 durable CommandLog）**
- 旧不变量（§2 粗体，SPEC A 区第 4 行同文）："durable CommandLog
  （…；command_id 幂等；确认即 accepted，跨 restart 自动重放）"
  ——隐含"accepted 的对话输入必被消费注入"。
- 为什么必须动：排队消息的撤回/编辑（UJ-07 纠偏的自然延伸、HANDA
  #29）需要"accepted 但被显式撤回"的合法终局；否则 UI 只能假装
  撤回（体验谎言）或禁止撤回。
- 新表述（追加条款，不改既有语义）："`revoke` 是同等 durable 的
  命令：它把**尚未消费的对话输入**标记为撤回——消费循环跳过其对话
  注入、照常推进 ConsumedInputSeq（consumed-as-revoked），journal
  落 `InputRevoked` 使撤回可审计；已消费目标的 revoke 是 no-op。
  『不丢』语义不变：撤回是 durable 的显式用户意图，重放收敛到同一
  结果；乱序不可能（revoke 必在目标之后 append）。"
- 波及面：protocol（种类+载荷）、inbox 校验、消费循环（loop 安全
  边界 drain 与 idle wake 两处读 inbox 的地方）、event 新类型
  `InputRevoked` + fold（审计投影，不改会话内容）、daemon 动词
  `unqueue`、CLI、webui、孪生（撤回矩阵+crash 重放+竞态窗）、SPEC
  A 区行文、GAPS 无涉。

### B. retry 命令身份（#16 · 不触不变量）

- **纯 CLI/webui 组装，零协议变更**：`ar retry <sid>` 读 journal 定位
  最后一个 user 源 `InputReceived`（Text/Content/Images/Files ref
  原样），组装标准 send；**command_id 派生**：
  `retry:<原 command_id>`——同目标重复点击被 AppendCommand 幂等
  去重；retry 产生的命令有自己的 id，其失败后的再 retry 目标不同，
  天然可链（review B5 两难的解）。**rev1 硬约束：重组必须是纯函数
  ——逐字节复现原载荷、禁注入 TurnID/ItemID 等易变字段**
  （commandPayloadHash 不清这些字段，同 id 异 hash 是报错不是幂等）。
- 仅 session 待命时可用（忙时报错引导用 send/interrupt）；webui
  失败 turn 加 Retry 按钮（走同一 CLI 包装）。
- 附件：journal ref 直接复用（CAS blob 同 session 在盘），组装侧
  标注 ref 来源避免重传。

### C. 结构化 ask 应答（#7 · additive）

1. **工具面**：ask_user def 新增可选 `questions[]`（≤4 问；每问
   question/options 2–4[label+description]/multi_select/
   allow_free_text），与既有单问 `question` 参数互斥；park 机制不变
   （一个 call 一次 park，§17"一批至多一个 ask"不破）。
2. **事件**：`AskResolved` 载荷 additive 扩展
   `Answers []AskAnswer{Question int, Selected []string, Text string}`
   （旧 `Answer string` 保留=自由文本兼容形）。
3. **应答通道（rev1）**：新命令种类 `CommandAnswer`（park 应答类，
   不进对话）载 typed answers。**落地四触点缺一不可**：daemon pump
   switch 加 case（default 会静默丢未知 kind）、`validateCommand`
   加 case+载荷检查、`commandPayloadHash` 清零其 CommandRef、路由到
   **ask-park 解析器**（WaitInput + `AskResolved.DeliverySeq` 推
   high-water 路径——**不是** approval broker，那是 WaitApproval）。
   daemon 动词 `answer` + CLI `ar answer <sid> <q>:<choice>[,...]`；
   park 期间自由文本 send 的旧配对路径保留（headless 兼容）。
4. **webui**：waiting:input 时渲染分步表单卡（问题徽章 x/y、单选
   点击即答、multi_select、Other 自由文本、Skip=cancelled 回传）。

### 落地顺序（一设计三步，每步一 INC 提交组）

1. **B（#16）**：零协议，先行——也充当 journal 读回组装的样板。
2. **A（#29）**：本变更单过契约 review 后实施。
3. **C（#7）**：协议+工具+webui 表单（与 CLAUDECODE SPRINT #10 联动
   收口）。

## 验收（实施步随各步 INC 细化）

- A：撤回矩阵孪生（未消费撤/已消费 no-op/非 input 拒/crash 重放
  收敛/竞态窗跳过+high-water）+ 真验（webui 排队撤回、journal 无
  该输入、InputRevoked 在）。
- B：重复 retry 幂等孪生 + 真验（失败 turn retry 原文含附件重发）。
- C：多问/选项/multi_select/Skip 孪生 + 真 Gemini 结构化提问全流 +
  webui 表单。

## review 记录（PROCESS §四，2026-07-11）

独立契约 review（子 agent，对照 DESIGN §2 原文+inbox/loop/daemon 代码
取证）裁决：**§A 修订后放行；§B/§C 确认 additive**。BLOCKER×2 +
MAJOR×3 已全部吸收为本纸 rev1：
1. B1：InputRevoked 原设计不带 DeliverySeq、fold 无分支 → resume 重放
   会把撤回翻案（ReadInbox 只滤 input 看不到 revoke）——rev1 改为
   AskResolved 三件套（事件带 seq/fold 推 high-water/重放改读
   ReadCommands）。
2. B2："pending 批按 seq 配对"是 resume 才有的形态，live 是 channel
   逐条——rev1 改为 revoked-target 集 + journalInput 前查集。
3. M1：daemon 前置校验=全量重折 journal 的 UX 优化，非安全边界。
4. M2：retry 重组必须纯函数（异 hash 是报错不是幂等）。
5. M3：CommandAnswer 四触点（pump/validate/hash/park 路由），走
   WaitInput 非 approval broker。
四性复核：不乱序/accepted 达标；不丢与重放收敛由 rev1 三件套补齐。
