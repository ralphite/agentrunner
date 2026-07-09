# INC-12 多 agent 工程团队（动态组队 · 树内消息 · 子唤醒 · 提权审批 · 子会话可见）

> 状态：实现完成，待 QA-18 收口（2026-07-09 起）。用户裁决（2026-07-09）：动态生成的
> 复杂结构，**只要用户确认，权限可以放宽**——兑现决策 #32 既有政策
> 条款"请求超父必须用户 approve"，为其建表达面；决策 #20 的"冻结
> 交集"加用户确认例外，走 §五的不变量变更单。

## 动机与 journey 锚

模拟一个软件工程团队：主 agent 动态生成 PM / Designer / 架构师 /
SWE / Team Lead 等角色，成员之间互发消息、做 design review 与 code
review，目标统一，完成后结果回流主 agent；用户全程可见、可点开每个
成员的内部细节（像看主 agent 一样）。

- 新 journey：**UJ-23 工程团队模拟**（JOURNEYS.md，本增量落）。
- 底座引用：UJ-18（fan-out/fan-in 编排）的全部机制。
- 关闭/翻案的既有条目：GAPS **G10**（子进度实时镜像）、SPEC B 域
  🧊"子提权申请通道"（需求已出现）、🧊"子 agent 可被 steer"
  （`ar send <child-sid>` 打通后自然翻案——消息通道即 steer 通道）。

## 核心洞察（为什么这不是架构返工）

- DESIGN §3 已写明"回执可多次发生（子被再次唤醒、再次静止，再投
  一次）"；§6"send 是显式重开手势，对任何 session 成立"。**长期
  存在、可反复唤醒的团队成员 = 既有静止模型的直接推论**，静止模型
  不变量不动。
- "团队成员待命" = 子 session 静止。唤醒 = 向它的 durable inbox 投
  一条输入。**per-session durable inbox（`store.AppendInbox`，
  inbox.jsonl/CommandLog）对子 session 目录同样成立**——铁律 2
  （journal-inputs-first、崩溃不丢输入）零新机制闭环。
- 原则 7 自检通过：一切新能力都表达为"往某个 inbox 投一条输入"
  （树内消息）或"在 turn 里做一件事"（spawn role、send_message）。

## Spec delta（SPEC.md B 域，实现 commit 同步并回）

| 功能点 | 状态目标 | 验收锚 |
|---|---|---|
| 树内消息 `send_message{to,text}`（父→子续话、子→父中途消息、兄弟直发；durable per-session inbox；execute-class 过管线） | ✅ | 孪生 TestSendMessage* · QA-20 |
| 静止子唤醒（消息到达 → 父 re-host 子 resume，同一 journal 续 context；`ChildRevived` 事件；第二次 SubagentCompleted 回执） | ✅ | TestReviveQuiescentChild* · QA-20 |
| 用户→成员：`ar send <child-sid>`（daemon 经树根宿主转投；单写者纪律：子的宿主永远是树根进程） | ✅ | TestDaemonSendToChild* |
| 动态角色：`spawn_agent{role:{name,description,instructions,tools?,permissions?,escalate?}}`（与 `agent` 互斥；spec `agents_dynamic: true` 开面） | ✅ | TestSpawnDynamicRole* · QA-20 |
| 子提权用户审批（role/子 spec `escalate: true` → spawn 强制 ask，载提权清单；批准=子用自声明规则、不前置父 gates；拒绝=交集降级、告知模型）（翻案 B 域 🧊 行） | ✅ | TestEscalationApproval* |
| 子会话 live 镜像（子 loop 事件带 session 标签入树 hub；`ar attach <child-sid>` live；G10 收口） | ✅ | TestChildAttachLive* · webui 手验 |

## Design delta（DESIGN.md，实现 commit 同步并回）

### D1 §3 新小节「树内消息：agent 发送方」

- **投递**：树内任一 session 可调 `send_message{to, text}`（execute-
  class，过四关卡）。to = `"parent"` | 树内 session 全 id | 本 session
  spawn 出的 handle。执行 = 向目标 session 目录的 **durable inbox**
  （复用 `store.AppendInbox`：fsync-before-ack、command_id 幂等、
  单调 DeliverySeq）append 一条 `UserInput`，正文带来源前缀
  `[message from <agent> (<session>)]`；随后 best-effort 投目标的
  live 输入通道。**这是 §2"一条通道、多种发送方"的第四个发送方
  （agent），与 INC-D2 的机器发送方同族。**
- **消费**：目标运行中——安全边界 drainQueued/idle awaitInput 消费
  （既有机制，消息以 `InputReceived{source:"agent", delivery_seq}`
  入 journal，seq 去重）。目标静止——消息躺 durable inbox，唤醒走
  D2。**协议例外不适用**：agent 消息永远是新输入，不做 tool result
  配对。
- **单写者纪律**：inbox 文件的写者 = 树根宿主进程（TreeRouter，
  进程内互斥）；journal 的写者仍是各自 loop。daemon 对子会话的
  send 经树根 CommandLog（`UserInput.Target` 字段）→ 树根 loop 转投
  TreeRouter——**子的宿主永远是树根进程**，任何路径不产生第二个
  写者。
- **治理**：消息目标必须在同一 correlation 树内（session id 前缀
  校验）；消息风暴的防线 = 树级预算 + per-turn generation 预算
  （每条消息至多激活一个 turn，花费有界）。深度/扇出上限不变。
- **crash 语义**：消息 durable 于目标 inbox 后发送方才拿到成功
  结果；目标崩溃/静止竞态由 resume 对账（inbox 尾巴 vs fold 的
  ConsumedInputSeq）重放，恰好一次。

### D2 §3 新小节「静止子唤醒（revive）」

- 静止子收到消息 → 它的**直接父**负责 re-host：父 journal
  `ChildRevived{call_id, child_session, activity_id}`（fold：以合成
  background activity 重入 Handles——原 handle 不变，kill 仍凭它）
  → goroutine 里子 loop **Resume**（同 journal、同 context 延续）→
  消费 inbox 尾巴 → 跑 turn → 再静止 → **第二次 SubagentCompleted**
  + ActivityCompleted（settle 路径零改动，report 照常以 user-role
  消息进父对话）。
- 唤醒信号：TreeRouter 找目标注册口，未注册（静止）→ 投其父的
  revive 通道；父在安全边界/idle 处消费。父自身也静止（中间层）→
  逐层向上找第一个活宿主，resume 链自然层层展开（resume 时扫描
  `sub/` 名下子 inbox 尾巴 → 有尾巴即 re-host，crash 恢复同一路径）。
- **竞态收口**：子静止返回与消息到达并发时——子注销 TreeRouter 后
  检查自身 inbox 尾巴，有则通知父 revive；最多多一次静止-唤醒往返，
  消息不丢。
- **预算**：revive allowance = min(parent 剩余, child cap − child
  已花)；≤0 则拒绝 revive，消息保留在 inbox，父收到 error 结果
  （模型可见，决定加预算/放弃）。
- kill/close 标记的约束不变：**带 user-kill 标记的子不被 parent 的
  revive 路径唤醒**（裁决二 C：用户 kill 的仅用户可复活）。

### D3 §9 spec 与 spawn：动态角色

- spec 新字段 `agents_dynamic: true`：允许本 agent 以 inline role
  spawn 子（默认 false；工具面 advertise 条件扩为"白名单非空或
  agents_dynamic"）。子经 frozen spec 继承该位（团队成员可再组队，
  深度上限管制）。
- `spawn_agent` 参数集扩展：`agent` 与 `role` 二选一。
  `role{name, description, instructions, tools?, permissions?,
  escalate?}` → 运行时构造 AgentSpec：instructions → system prompt；
  tools 缺省继承父 spec 工具面（声明则取交集校验）；model/预算
  继承父；**hooks/MCP/skills 恒不可声明**（模型输出按不可信内容
  对待，决策 #19 同族条款）；`bypass` mode 恒非法。
- `SpawnRequested` 事件增 `RoleSpec`（序列化的构造后 spec）：动态
  role 无 yaml 文件，revive/审计/resume 从 journal 取真相。

### D4 §5/§15 提权审批（不变量变更，见 §五变更单）

- role/子 spec 可声明 `escalate: true` + `permissions`（显式请求制，
  不做规则包含关系自动推断）。spawn effect 判定遇 escalate →
  **强制 ask**（无论 rules 对 spawn_agent 怎么说），`ApprovalRequested`
  载荷 = 请求的规则清单原文 + 角色说明。
- 用户批准 → childLoop **不前置父 gates**，子管线 = 子声明规则 +
  预算 gate（预算仍切父树，**树预算与深度/扇出上限不放宽**；netns
  收容棘轮**不解除**——决策 #33 红线不碰）。拒绝 → 降级为冻结交集
  继续 spawn，拒绝理由进 spawn 结果（模型可见）。
- 审批沿既有 correlation 冒泡到 frontend——**审的永远是人**。

### D5 §12 可见性（G10 收口）

- 子 loop 继承带 session 标签的输出 sink：树内全部 protocol 事件
  （delta/工具/审批/idle）入树根 hub，`Event.Session` 区分来源。
- `ar attach <child-sid>`：journal 补读（既有 -sub- 解析）+ 订阅树
  hub 按 Session 过滤。webui 子会话视图接 live 流；时间线渲染
  send_message / ChildRevived。

### D6 §15 决策表 delta

- **#20 修订**（不变量变更单 §五）：权限冻结交集 + **用户确认的
  提权例外**（显式 escalate 请求、ApprovalRequested、批准后子用
  自声明规则；树预算/扇出/收容棘轮不在例外内）。
- **新 #34 树内消息**：agent 是 send 通道的第四种发送方；per-session
  durable inbox 树内复用；宿主单写者；树内寻址；风暴防线=预算。
- **新 #35 动态角色**：spawn 可 inline role（模型输出=不可信内容：
  无 hooks/MCP/skills、bypass 非法）；`agents_dynamic` 开面；
  RoleSpec 入 journal。

## 五、不变量变更单（决策 #20，PROCESS §四）

- **旧不变量原文**（§15 #20）："权限 rules 在 spawn 时冻结交集
  下传；预算 = min(child 限额, parent 剩余) 沿树聚合；深度/扇出有
  上限。"及 §3"child spec 无法放宽"。
- **为什么必须动**：工程团队场景中，主 agent 动态起草的成员可能
  正当地需要父面之外的权限（如父是只读评审角色、而 SWE 成员需要
  写码）。用户裁决（2026-07-09）：**用户确认后风险可控，必须支持
  放宽**。决策 #32 早已埋下政策条款"子 agent 默认权限不超父，请求
  超父必须用户 approve"，本变更是给它建表达面，非新政策。
- **新表述**："权限 rules 在 spawn 时冻结交集下传，child spec 无法
  自行放宽；**唯一例外：child 显式 `escalate` 请求经
  `ApprovalRequested` 由用户批准后，以其自声明规则替代交集**（批准
  事实 journaled、审批人恒为用户）。预算 min 聚合、深度/扇出上限、
  netns 收容棘轮**无例外**。"
- **波及面**：`internal/agent/spawn.go`（childLoop gates 分支）、
  `internal/agent/loop.go`（spawn effect 强制 ask）、spec loader
  （escalate/role 校验）、DESIGN §3/§5/§15、SPEC B 域行、本工作纸；
  测试 TestEscalationApproval*（批准/拒绝/interrupt 三态）。
- **review**：并入本增量收口的三视角对抗 review，契约视角必须
  覆盖本条。

## 验收

- **闸门 A（孪生，routing provider + crash 注入）**：
  1. 父 spawn 两子，子 A `send_message` 子 B（兄弟直发），B 运行中
     边界消费、回消息给 A；
  2. 子静止 → 父收回执 → 父 `send_message` 唤醒子 → 同 journal 续
     context（单 SessionStarted）→ 第二次回执；
  3. 子中途 `send_message parent`，父 idle 被唤醒起 turn；
  4. `ar send <child-sid>`（daemon 孪生）：durable、转投、去重；
  5. 动态 role spawn（无白名单命中）+ RoleSpec 入 journal + revive
     从 RoleSpec 重建；
  6. escalate 三态：批准（子写父面外路径成功）/ 拒绝（降级交集，
     deny 拦截）/ interrupt（审批作废）；
  7. crash 注入：消息 durable 后、目标 journal 前崩溃 → resume 重放
     恰好一次；revive 后子在飞崩溃 → settle-from-child-fold 不变。
- **闸门 B（真实 API）**：**QA-20**——真 Gemini，主 agent
  `agents_dynamic` 组队（pm / engineer / reviewer 三动态角色），
  互发消息完成一个小实现 + review 往复，结果回流；断言只钉 runtime
  红线：send_message activity 与目标 InputReceived{source:agent}
  配对存在、ChildRevived ≥1、同一子 journal 单 SessionStarted、
  SubagentCompleted ≥2（同一 call）、最终父报告非空。共享数据目录，
  数据保留，归档 `qa/runs/`。

## 实施步骤（一步 = 一个可合并提交）

1. ✅ **INC-12.0** 本工作纸 + JOURNEYS UJ-23（文档入口）。
2. ✅ **INC-12.1** 树内消息内核：TreeRouter、send_message def+分派、
   InputReceived{source:agent}、树内寻址校验、durable inbox 复用、
   运行中消费；孪生 1/3。
3. ✅ **INC-12.2** 静止子唤醒：ChildRevived 事件+fold、revive 执行、
   竞态收口、resume 扫尾巴、预算重核；孪生 2/7；DESIGN D1/D2 并回。
4. ✅ **INC-12.3** daemon 子会话 send：UserInput.Target、handleSend
   路由、CLI 放行子 id；孪生 4。
5. ✅ **INC-12.4** 动态角色：role 参数、spec agents_dynamic、
   RoleSpec 入 SpawnRequested、revive 兼容；孪生 5；DESIGN D3 并回。
6. ✅ **INC-12.5** 提权审批：escalate 强制 ask、childLoop gates 分支、
   拒绝降级；孪生 6；**DESIGN #20 修订与本步同 commit**（D4/D6）。
7. ✅ **INC-12.6** 可见性：子 sink 标签、daemon attach 子 live、webui
   （子会话流、时间线渲染、Subagents 状态）；DESIGN D5 并回。
8. **INC-12.7** QA-20 真实 API + 文档收口（SPEC/GAPS/QA/LOG 并回，
   工作纸归档）。

## review 裁决

里程碑级：**做三视角对抗 review**（正确性/并发、安全、契约——契约
基准 = DESIGN+QA），多轮至收敛（连续一轮无新 P0/P1）；不变量变更单
必须被契约视角显式覆盖。
