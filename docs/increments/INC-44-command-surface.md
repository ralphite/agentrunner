# INC-44 命令面设计单元「命令身份·撤销·应答」（HANDA 2U：#16/#29/#7）

**状态：📐 设计稿（awaiting contract review）——#29 触 DESIGN §2 铁律，
按 PROCESS §四本纸先裁后码；#16/#7 与其共用协议/消费面，随单元一并
定稿、分步落地。**

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
2. **daemon 前置校验（尽早拒绝，不落无效账）**：目标 command_id 不
   存在 / 种类非 `input` / 已消费（target seq ≤ 当前 fold 的
   ConsumedInputSeq）→ 同步报错不落账（"already being processed"）。
   interrupt/approval/close/kill/control 永不可撤。
3. **消费守卫（校验-消费竞态窗）**：消费循环把 pending 命令按 seq
   排序配对——`input` 命令若其后存在指向它的 `revoke` 且自身尚未
   消费 → **跳过对话注入、照常推进 ConsumedInputSeq**
   （consumed-as-revoked），并 journal `InputRevoked{TargetCommandID}`
   （additive 事件，审计+webui"已撤回"显示）；revoke 晚于消费到达
   → no-op（复用迟到审批 no-op 先例，DESIGN §430/640 同族）。
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
  天然可链（review B5 两难的解）。
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
3. **应答通道**：新命令种类 `CommandAnswer`（approval 同族的 park
   应答类，不进对话）载 typed answers；daemon 动词 `answer` + CLI
   `ar answer <sid> <q>:<choice>[,...]`；**park 期间自由文本 send 的
   旧配对路径保留**（headless/旧客户端兼容）。
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

## review 裁决

**#29 部分按 PROCESS §四必须单独契约 review**（本纸 §A 变更单）；
B/C 为 additive，随单元一并送审。review 通过前不动代码。
