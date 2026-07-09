# INC-D2 外部事件唤醒既有 session（G14 / UJ-12）— 设计稿

> **状态：设计稿，design-first；未实现。** invariant-adjacent（引入
> outward-facing ingress，须先成文机器发送方的信任/鉴权条款）。

## 动机与 journey 锚
- GAPS **G14**（设计缺失·高，云形态）+ JOURNEYS **UJ-12**（PR 保姆：
  CI 失败 webhook → 唤醒既有 session → 拉日志分类修复）、UJ-13。
- 对标 Codex：GitHub/Linear/Slack 事件触发既有任务。这是"输入投递"通道
  的**第三个发送方**——机器（前两个：终端用户、web 用户）。

## 现状（inbox 原语已就绪,缺投递壳）
- 投递入口 `daemon.handleSend`：查 hub.runs[session] 命中即活跃、未命中经
  Resume 复活（send-as-resume）→ PersistInput 先 fsync 落 durable mailbox
  再 ack → hub.post 投进 loop inbox。
- durable mailbox = `store/inbox.go`（inbox.jsonl,单调 delivery_seq,
  fsync-before-ack）。消费侧 journal-inputs-first + DeliverySeq 去重 =
  恰好一次；resume 重放未消费尾巴。
- **park→wake**：WaitRules 只两种（WaitInput / WaitApproval）。
  WAITING_INPUT idle 的 `awaitInput` select 于 UserInputs/bg.done/Cancels/
  Interrupts——**机器投递进 parked-at-idle 的 session 无需改动即唤醒起
  turn**。但 WAITING_APPROVAL 的 `awaitApproval` 只 select 审批 resolver +
  Interrupts,**不** select UserInputs——审批挂起期机器投递只排队不唤醒
  （G3 余项,本纸定案为"排队不解栈审批"）。

## 须先成文（invariant-adjacent，扩展/补全既有不变量,非翻转决策表）
1. **机器发送方的信任/鉴权**：一个接受外部网络输入的 ingress surface,其
   鉴权与 source 归属未在决策 #19（信任模型）或 notifier/hooks surface
   carve-out 里覆盖。须写清:机器投递壳属哪个信任级、如何认证、是否过四
   关卡。**建议**：投递壳是独立进程/端点,持 per-session 投递 token；
   投进来的内容按**不可信**对待（与 memory/web 同级,加来源标记,不因
   "来自 CI"而提权）。
2. **弱类型 Input 的机器来源前缀**：§2 已铺垫 source=journal 元数据 +
   内容前缀,但只落实 user/child/tool/timer/control——须落实
   webhook/ci/pr-comment 来源。
3. **发送方幂等 id**：兑现铁律 3（§2,设计有实现无）——机器高频重投不
   产生重复 turn。
4. **WAITING_APPROVAL park 期间机器投递** = 写死"排队不唤醒/不解栈审批"
   （G3 余项定案）。

## 机制草图（最小投递壳）
- `protocol.UserInput` 增 `Source string` 与 `IdemKey string`（现仅
  Text/Images/DeliverySeq）。
- `daemon.Command` 增 Source、IdemKey；`handleSend` 透传进
  protocol.UserInput；IdemKey 去重（已投过同 id → 幂等 ack,不重投）。
- 投递壳（独立 surface,不进核心）：一个把 webhook/CI payload 归一成
  `send{session, text=渲染后的事件摘要, source=ci|webhook|pr-comment,
  idem_key}` 的薄壳；鉴权持 per-session token。**MVP 可先做一个通用
  `ar notify <session> --source ci "payload"` CLI/HTTP 入口**,把"机器
  发送方"跑通,GitHub/Slack 具体集成后续。
- 消费无需改动（inbox + awaitInput 已覆盖 parked-at-idle 唤醒）。

## 波及面
- DESIGN §Scheduler/triggers + §2 Inbox：机器发送方 carve-out（信任/鉴权
  模型 + 来源前缀 + 幂等 id + 审批期投递语义）。
- 代码：protocol/input.go（Source/IdemKey）、daemon（Command + handleSend
  透传 + idem 去重）、投递壳（新 surface）。
- SPEC A 行 G14；GAPS G14/G3 关闭注记；QA 场景（CI payload → parked
  session → 真实 LLM turn 诊断）。

## 验收
- 孪生：scripted——parked-at-idle session,机器 send{source=ci, idem_key}
  → 唤醒起 turn；同 idem_key 重投 → 幂等不重复；WAITING_APPROVAL 期间机器
  send → 排队不解栈。
- 真实 API QA：webhook/CI payload 投进 parked session → 真实 LLM 诊断
  （UJ-12 step 2）。

## review 裁决
引入 outward-facing ingress：安全视角 review 必做（鉴权/信任/注入）。
本纸仅设计。
