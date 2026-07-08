# INC-001 · 术语与裁决落地实施说明（terms & rulings implementation instruction）

> **本文是什么**：把 `REVIEW-001-terms.md` 收集并裁决的**全部**术语更新，
> 连同它们在统一模型下的涌现推论、相关审计 critical、以及跨文档一致性
> 修复，汇成**唯一的、可执行的落地清单**。REVIEW-001 是"意见+裁决"的
> 原始记录；本文是"据此要改什么、改到哪一行、为什么"的施工说明。
>
> **本文不是什么**：不是新的裁决场（裁决已在 REVIEW-001 定案），也不是
> 实现本身。凡遇 DESIGN 不变量改动，仍须走 `PROCESS.md` 的不变量变更
> 流程；本文只把"改哪些不变量"列全。
>
> **性质与纪律**：改文档/代码前先读 `PROCESS.md`。全程受 Part III 的元
> 纪律（MR-1..MR-5）约束。命名以 §18 术语表 + 实现名为 canonical，不设
> "设计名→实现名"对照层（决策 2026-07-05：零 legacy mapping）。

---

## Part 0 · 三态标注与全局现状（读本文前先看）

落地是**三态**的，本文每一项都带一个标注，务必分清（这是本 session 反复
踩坑之处：文档把目标态写成"✅ 已落地"，代码却是旧两形态模型）：

| 标注 | 含义 |
|---|---|
| 〔码文皆改〕 | 文档与代码都已在 `main` 兑现，仅列作账不需再动。 |
| 〔文改·码未改〕 | 某文档已按目标态改写（甚至标 ✅），但**代码仍是旧模型**，是本 INC 的主战场。 |
| 〔待落地〕 | 文档与代码都还没有，全新增量。 |
| 〔文自相矛盾〕 | 文档 A 与文档 B（或同文档两处）互相打架，须校准到单一口径。 |

**全局现状判定（2026-07-08 盘点，基于 `main` @ `a5a84f2` 与真实 code grep）**：

- **DESIGN 决策表**已加入 #30（标记+检查）/#31（静止模型）/#32（换 agent
  提权），高层裁决在决策层**已成文**。
- **DESIGN §18 术语表、决策 #25、§17 实现注记**仍大面积残留**旧两形态
  模型**文字（interrupt=close、Input tagged-union、后台任务(task)、spawn
  两形态、"conversational vs task 两形态"）。
- **SPEC.md** 多行已改写为目标态并标 **✅ "落地"**——但对应**代码并未改**
  （`Loop.Conversational` 仍在、`TaskCompleted` 事件仍在、`SpecChanged`
  事件不存在）。即 SPEC **过度声称**。
- **GAPS.md** §2 关闭了 G8/G24/G22 等，但 **§1 速览仍给 UJ-03/18 等打
  ❌**（未随 §2 重打分），且 G6 关闭注仍引用即将被删的 `Loop.Conversational`。
- **代码**：静止模型手术**完全未做**。`Conversational` 标志遍布
  `internal/agent/loop.go`（:66/:266/:428/:438/:475/:489/:703/:738/:1363）、
  `epilogue.go:130`、`event.SessionStarted`、~10 个测试文件；`TaskCompleted`
  事件仍在（`internal/event/types.go:26/248`）；`SpecChanged` 事件**不存在**。
- **C1 空-parts 修复**：写入侧（`provider.go` `CollectTurnStreaming`）与请求
  侧（`gemini.go` `toContent`）代码**已改且编译通过**，随本 INC 一并提交。

一句话：**决策已定、部分前沿文档已抢先写成目标态、代码几乎全是旧模型**。
本 INC 的使命 = 让代码追上、让残留文档改齐、让自相矛盾归一。

---

## Part I · 统一概念模型（一切裁决收敛于此的五支柱 + 涌现推论）

REVIEW-001 的八条术语意见与四组裁决，**不是八个孤立修补**，而是同一个更
简单模型的不同切面。先立模型，再逐项落地才不散。

### 五支柱

1. **静态 session 模型（决策 #31）**——只有一种 session，**没有运行形态**。
   "跑完交卷"不是一种形态，而是 session 走到**静止（quiescence）**这个
   *形状*。`ar run` 退化为便捷命令："开 session + 发消息 + 等静止 + 读结果"。
   消解 REVIEW-001 的裁决三/四、#追加(task 质疑)。

2. **标记 + 检查（决策 #30）**——**删除一切"终止/terminal 状态"**。close /
   kill 只是 journal 里的**标记**（带来源 `user|parent`），**只被自动路径
   读取**（timer sweep / boot sweep / 父自动复活判定）。**用户显式 `send`
   永远重开任何 session**，无视标记。消解 #13、#11。

3. **session ≠ agent（决策 #32 / 意见 #1）**——session 的本质是"持续跟随
   的 event log"，**不与某个 agent 绑死**。spec 是**可变的 session 属性**，
   经 `SpecChanged` 事件换代，而非冻结于 SessionStarted。消解 #1、G8。

4. **Input 弱类型化（意见 #9）**——对话面 = **内容 + 来源前缀**，不靠强
   类型字段。类型只作为 **journal/log 层的审计元数据**存在，**不进给模型
   的对话内容**。**唯一例外（协议红线）**：前台 `tool_call` 与 `tool_result`
   必须严格配对（Gemini `functionResponse`↔`functionCall` 按数量+位置、
   Anthropic `tool_use`↔`tool_result` 按 id），弱类型化**绝不能破坏这个
   配对**。

5. **task → handle（意见 #追加）**——"后台工作"一律叫 **handle**（实现名
   `task_id` 是 wire 细节）；"task 形态"义**整体删除**（并入支柱 1）。

### 涌现推论（从五支柱自然导出，实施时必须一并落地，逐条）

> 这些**不在** REVIEW-001 字面裁决里，但是裁决在统一模型下的**逻辑后承**，
> 漏掉任何一条，落地就会自相矛盾或留下死角。

- **R1 · 回执可多次**：静止是可重复到达的*形状*（静止→被 send 唤醒→再
  静止），故**子回执可发生多次**（每次静止一次），非一次性终态。已入决策 #24。
- **R2 · 换 agent ⇒ 重置 provider 签名 + cache epoch**：Gemini 的
  `thoughtSignature`（`internal/provider/gemini/gemini.go` `extrasSignatureKey`）
  是 per-agent/per-model 的；`SpecChanged` 换 agent/model 时，旧签名对新
  模型无效，且 prefix 缓存必须**显式换代**（决策 #10 "mode 切换不打爆
  tools 级缓存"的同族问题，但换 agent 是**有意**作废缓存）。落地须定义
  "prefix 换代点"。
- **R3 · 删两形态 ⇒ `Conversational` 标志整体消失**：静态模型下没有
  `Conversational=true/false` 的二分。删除该字段，**连带修复审计 C4**
  （fork 吞消息的根因正是 fork 未置 `Conversational=true`——字段没了，
  bug 自然消失）。
- **R4 · final generation 的"假收尾" ⇒ 统一 generation-step 异常路径**：
  意见 #5 担心"最后一个 generation 带 tool call、执行完模型就不生成了"。
  裁决：final generation 是**形状判定**（不带 tool call 才算收尾），带
  tool call 必然回到模型，不存在"执行完就停"。但**若某个 generation step
  真的产出异常形状**（空 parts / malformed tool call / blocked），应走
  **统一的 step 异常处理**，不为 final generation 单独特判——**这正是审计
  C1（空-parts）的概念归宿**：空 parts 是"generation step 产出异常"的一
  个实例，统一在写入侧合成占位 + 请求侧修补。
- **R5 · 删 `TaskCompleted` 是破坏性事件变更 ⇒ event-schema 版本 bump**：
  决策 #18（不 migration，`SessionStarted` 记 schema 版本，不匹配拒绝
  resume）。删除 wire 事件类型是破坏性的（旧 journal 有 `task_completed`），
  须 bump 版本；additive 的 `SpecChanged` / `SessionClosed.Source` 若设计为
  optional 可不 bump——**删除类**才 bump。落地须明确版本号与判定。
- **R6 · parent → 已存在子 的消息原语缺失**：裁决二（parent 复活自己
  kill 的子）与"任何 session 任何时候可续发消息"要求 parent 能**向一个
  已存在的子 session 投消息**。现有原语只有 `spawn`（新建子）、`kill`
  （杀子）、子→父的 `SubagentCompleted` 回执——**没有** parent→existing-child
  的 send 通道。这是**新原语**（〔待落地〕），是裁决二可实施的前提。
- **R7 · CLI run/new/submit 语义塌缩**：静态模型下 `ar run` = 便捷命令；
  `ar new`（开 session）+ `ar send`（发消息）+ 等静止 + 读结果，三者语义
  收拢。`submit`/`resume` 的角色需按"只有一种 session + 显式 send 永远
  重开"重新表述。属 §18.10 run 踩坑行的代码兑现。

---

## Part II · 变更后的不变量（DESIGN 级；改动须走 PROCESS 不变量流程）

落地会新增/修订下列 DESIGN 不变量。**每一条都是"改 DESIGN §不变量"**，
按 `PROCESS.md` 的不变量变更流程处理（停下、写清冲突、单独 review）。

| 编号 | 不变量 | 出处裁决 | 现状 |
|---|---|---|---|
| INV-Q1 | **静止定义**：静止 =（final generation 已出 ∧ inbox 空 ∧ 无在飞前台/后台/子 handle ∧ 无未到期 durable timer ∧ 无 pending 审批）。 | #31 | 决策 #31 已述形状，须提升为显式不变量并逐项枚举 |
| INV-Q2 | **静止动作时序**：静止时固定顺序 auto-publish outputs → CheckpointBarrier → 向 parent 投 `SubagentCompleted`（若有 parent）；**可重复发生**。 | #24/#31, R1 | 决策 #24 已成文 |
| INV-M1 | **标记只被自动路径读**：close/kill 是带来源标记；timer sweep / boot sweep / 父自动复活判定读它；**显式 `send` 无视标记重开**。无终止状态。 | #30/#13/#11 | 决策 #30 已成文，§18/§17 残留旧文字 |
| INV-M2 | **来源复活**：用户 kill 的子仅用户可复活；parent kill 的子 parent 可复活。 | 裁决二 C | 决策 #30 含此，需 R6 原语支撑 |
| INV-P1 | **提权审批**：子 agent 权限默认 ⊆ 父；请求超父必须**用户 approve**；用户自己切 agent **免确认**。 | 裁决一 | 决策 #32 已成文，码未落 |
| INV-X1 | **配对红线**：前台 `tool_call`↔`tool_result` 严格配对，Input 弱类型化不得破坏（Gemini/Anthropic 协议约束）。 | #9 例外 | 决策 #9 已在，须在弱类型化落地时显式保护 |
| INV-C1 | **空-parts 不毒化**：任何 assistant 消息 `parts` 恒非空（写入侧合成占位 + 请求侧修补）。 | 审计 C1, R4 | **代码已改**（本 INC 提交） |
| INV-R1 | **restart = resume**：无 supervision 自动重启；恢复只住一个地方。 | G22 | 决策 #29 已成文 |

---

## Part III · 元纪律（MR-1..MR-5，贯穿一切改动，违反即返工）

| 编号 | 纪律 | 来源 | 要点 |
|---|---|---|---|
| **MR-1** | **零 legacy，删就删干净** | REVIEW-001 硬性 | 禁止"因为 X 在用所以保留"式论证；项目无发布、无兼容义务。"阻塞 spawn 保留 v1 兼容""Conversational=false 的 task mode"这类残留在落地时一并清除。 |
| **MR-2** | **一切设计自顶向下** | REVIEW-001 硬性 | 每个技术选择必须可追溯到 JOURNEYS/SPEC 层需求；呈现给开发者的是高层影响；无需求不设计（over-design 是 REVIEW-001 反复点名的病）。 |
| **MR-3** | **术语纪律** | §18 命名原则 | 裸 `step`/`task`/`run`/`loop` 禁用；一律用 §18 canonical 词（generation step / tool step / handle / agentic loop / loop mode）。代码与文档同名。 |
| **MR-4** | **离线验证 + git 内容寻址交叉核对** | 本 session 血教训 | 工具输出的散文（`ls`、prose）**可能被伪造**；只信 `git hash-object` / `git ls-tree` / `wc -l` 的数值型输出。凡"文件是否在库"用内容寻址核，不用散文核。 |
| **MR-5** | **改完即 commit + push** | CLAUDE.md + 本 session 事故 | 未提交文件会被**并发 session 清掉**——本 INC 文档曾整份丢失一次即因此。不留未推送提交、不留未提交工作区改动。 |

---

## Part IV · 逐项落地（每条 REVIEW-001 术语一节）

> 体例：**开发者原话**（verbatim，摘自 REVIEW-001）→ **裁决** → **文档改动**
> （精确到条目/行）→ **代码改动**（精确到 `文件:符号`）→ **标注** → **验收**。

### A · #1 session（session≠agent；换 agent；id 澄清）

**开发者原话**：
> "这个 id 是 session id 还是 agent(id)?" · "一个 session 必须可以更换
> agent（session 生命周期内 agent 可变）。" · "session 不应该与 agent
> 绑定。session 的本质是持续跟随的 event log；把它和某个 agent 绑死可能
> 出问题。"

**裁决**（决策 #32）：session 内可换 agent，用 `SpecChanged` 事件、prefix
显式换代；**用户切换免确认**。id = session id（agent 不是一等身份，spec
才是 session 的可变属性）。

**文档改动**：
- DESIGN §18.9 `spec / instance` 词条："冻结于 SessionStarted" → "**初始**
  spec 于 SessionStarted 立；session 内经 `SpecChanged` 可换代（决策 #32）"。
- DESIGN §18.1 `session` 词条：补一句"session 不绑定 agent；agent/spec 是
  可换代的 session 属性"。
- 决策 #25 文字（见 Part VII）：删"绑定"暗示。
- GAPS G8：〔码文皆改·文档侧〕已标 ✅ 关闭，保留。

**代码改动**〔待落地〕：
- `internal/event/types.go`：**新增** `SpecChanged` 事件（当前 grep 确认
  不存在）：`{NewSpecName/NewSpecPath, PrefixEpoch, By(user|agent), 权限交
  集重算标记}`。登记进 `decoderRegistry`。
- 换代点须触发 **R2**：重置 provider 签名（清 `extrasSignatureKey` 累积）
  + prefix cache epoch++（`internal/agent/assembly.go` 的 prefix 冻结逻辑
  须认 epoch）。
- CLI `ar agent <sid> <spec>` 命令（SPEC J 已声称落地，code 需补）。

**标注**：〔文改·码未改〕（SPEC J/决策 #32/GAPS G8 已写目标态；`SpecChanged`
事件、`ar agent` 命令、prefix 换代逻辑均未在 code）。

**验收**：真实 API——起只读 agent 分析→`ar agent` 换成写权限 agent→同一
session 同一上下文继续、prefix 换代可见、无确认弹窗；子 agent 提权路径见 I。

---

### B · #5 final generation（统一 generation-step 异常；连带审计 C1）

**开发者原话**：
> "有没有可能……最后一个 generation 带着 tool call，我们执行完这个 tool
> 之后，模型就不再生成了？" · "如果这属于错误，那它是一个异常；而如果是
> 异常，所有的 step 都可能出错——可以考虑把'step 出错'统一作为一套异常
> 处理，不为 final generation 单独特判。"

**裁决**：final generation 是**形状判定**（不带 tool call 即收尾），带
tool call 必然回到模型，**不存在"执行完就停"**（此疑虑消解）。但"step
产出异常"确应**统一处理**（R4）——**审计 C1 空-parts 即其一个实例**。

**文档改动**：
- DESIGN §18.1 `final generation` 词条：补一句"若某 generation step 产出
  异常形状（空 parts / malformed_tool_call / blocked），走统一 step 异常
  路径（决策 #6 的异常终止形态族），不为 final generation 特判"。
- DESIGN §4 "异常终止形态"：把"空 parts"列为一类 in-doubt/异常，指向 C1 修复。

**代码改动**〔码已改·随本 INC 提交〕（审计 C1，写入侧 + 请求侧 + 可修复）：
- **写入侧** `internal/provider/provider.go` `CollectTurnStreaming`：组装后
  若 `len(turn.Message.Parts)==0`，合成 `Part{Kind:PartText, Text:
  EmptyGenerationPlaceholder}`；新增 `const EmptyGenerationPlaceholder =
  "(no visible output)"`。**绝不落 `parts:null`**。
- **请求侧** `internal/provider/gemini/gemini.go` `toContent`：遇 `len(msg.
  Parts)==0` 时**先设 role 再补占位 part 并返回**，替换原来的硬 error
  `message with role %q has no parts`——让任何历史坏 journal 变可重放（防
  御纵深）。
- **回归测试** `internal/provider/emptyparts_test.go`（随本 INC 新增）：
  `TestCollectTurnNeverEmptyParts`（纯 thought/无 delta 也非空）、
  `TestCollectTurnKeepsRealText`（有正文不被占位覆盖）。

**标注**：〔码已改·待提交〕代码已在工作区且编译通过；文档 §18.1/§4 的补
句〔文待改〕。

**验收**：真实 API 深轮次（20+）生成型任务（原 C1 复现脚本：写游戏→加
功能→转 web app→headless 检查），不再 `has no parts` 死会话；重开可续。

---

### C · #9 inbox / Input（弱类型化；来源前缀；前台配对是协议例外）

**开发者原话**：
> "反对 Input 的显式类型分类"（理由：扩展性——每加一种来源要全链路认识
> 新类型；类型必要性——Input 本质只是给大语言模型看的内容，不需要强
> 类型）· "来源信息用内容前缀表达，不用类型字段"（tool call 结果正文前
> 加说明；子 agent 返回像普通文本前缀标明 agent/session id/返回了什么）·
> "作为整个系统的 log，可以有类型；但 Input 本身是给 agent 的东西，类型
> 放进这个 event 里完全不需要。"

**裁决**：对话面弱类型化——内容 + 来源前缀；类型**只留 journal/log 层**做
审计。**红线例外**：前台 `tool_result` 与 `tool_call` 的严格配对是
provider 协议要求（Gemini `functionResponse` 按数量/位置、Anthropic 按
id），**不可退化为纯文本前缀**（否则 400 / 配对崩）。

**文档改动**：
- DESIGN §18.2 `Input` 词条：由"tagged union 五种（user_message/child_
  result/tool_result/timer/control），全部 journal 为 InputReceived" 改为
  "对话面 = 内容 + 来源前缀；来源/类型仅作 journal 审计元数据，不进对话
  上下文。**例外**：前台 tool_result 保持与 tool_call 的协议级配对（决策
  #9），不走纯文本前缀"。
- DESIGN §2/§18.2 `control 输入`：保留（control{kill,close} 本就不进对话），
  但明确它是"journal 层类型"而非"对话面类型"。

**代码改动**〔待落地〕：
- `child_result` / `tool_result`（后台义）的**对话面渲染**改为"user-role
  消息 + 来源前缀"（现状已接近：§17 记后台终态"fold 渲染 user-role
  消息"）；把仍显式分叉的类型判定收敛为"渲染时加前缀"。
- **前台 tool_result 路径不动**（`internal/provider/*` 的 `PartToolResult`
  配对保持）——这是 INV-X1。
- journal 事件 `InputReceived` 仍可带 `source` 元字段（log 层类型保留）。

**标注**：〔文改·码未改〕（DESIGN §18.2 仍是 tagged-union 旧文字；对话面
渲染收敛未做）。

**验收**：子 agent 返回、后台 tool 结果在父对话里以带前缀 user-role 文本
出现；前台工具调用多轮不 400（配对不破）。

---

### D · #11 interrupt（idle 处 = no-op；拒绝任何 interrupt→end 语义）

**开发者原话**：
> "对 interrupt 这个词对应的用户功能不理解。" · "杀死 ≠ session 结束，
> 完全不是。最常见的需求是：我看到它在做的事完全不是我想要的 → kill 掉
> → 再写一条消息让它继续执行。" · "任何 session 在任何时候都可以继续发
> 消息、继续执行。" · "不接受现行定义里 interrupt 导致 session 被 end /
> turn 被 end 的情况（含'待命处 interrupt = close'这条交互惯例）。"

**裁决**（裁决 #11 / 决策 #30）：turn 中 interrupt = 打断当前活动（部分
输出留存），**会话继续**；**idle 处 interrupt = no-op**（无在跑活动可
打断，什么都不做）——**删除"待命处 interrupt = close"惯例**。close 是
独立命令、且只是可重开标记。

**文档改动**：
- DESIGN §18.2 `interrupt` 词条：由"idle 处 = close 会话" 改为 "**idle 处
  = no-op**（无在跑活动，不产生任何状态变化）；close 是独立命令且为可重开
  标记"。
- DESIGN §17 实现注记："实现新增'待命处 interrupt = close'" 那条**删除**。
- SPEC A "interrupt 永不结束 session（待命处 = no-op；close 是独立命令）"
  〔文改·已写目标态〕保留，但**当前标 ✅ 属过度声称**（见下码改），改为
  🟡 或补"码待落地"。
- 审计"三、真实通过"里"idle interrupt=close"是**旧行为**，落地后此行为
  **改变**，QA 用例须改。

**代码改动**〔文改·码未改〕：
- `internal/agent/conversation_test.go:328` `TestConversationalIdleInterrupt
  Closes` → 改为 `TestIdleInterruptIsNoOp`（断言 idle interrupt 不产生
  `SessionClosed`、session 仍待命）。
- idle 处收到 interrupt 的分支（现产生 close 意图）改为 no-op。

**标注**：〔文改·码未改〕+ 一处〔文自相矛盾〕（§18.2 idle=close vs SPEC A
idle=no-op）。

**验收**：idle 时发 interrupt → 无 `SessionClosed`、`sessions list` 仍
waiting:input；随后 send 正常续聊。

---

### E · #12 fold / state（state = history 部分 + runtime 记账，拆写）

**开发者原话**：
> "state 指的究竟是什么？它是不是针对大语言模型的一个 request？" · "一个
> request 由 system instruction、history、tools 组成——其中 system
> instruction 与 tools 更多是从 agent spec build 出来的；state 看起来更
> 像其中的 history 那一部分。"

**裁决**：state ≠ LLM request。state = **纯 fold(journal)**，其中**只有
history 部分喂给模型**；其余（预算用量 / 在飞 handle / 等待状态 / 权限
mode / mailbox 高水位）是**runtime 记账**。LLM request = `assemble(state,
spec)`——system/tools 出自 spec，history 出自 state。

**文档改动**：
- DESIGN §18.3 `fold / state` 词条：补"state 含两类——① 喂模型的 history
  部分；② runtime 记账（预算/在飞 handle/等待/mode）。LLM request =
  assemble(state, spec)：system instruction 与 tools 出自 spec，history 出自
  state（回应意见 #12）"。
- DESIGN §18.9 `context assembly` 词条：显式点出"history 来自 state 的
  history 部分，非 state 全体"。

**代码改动**：纯文档澄清，**无代码改动**（现有分层已如此，只是术语表没
写清）。

**标注**：〔文待改〕（仅术语表补写）。

**验收**：术语表读来能明确回答"state 里哪些给模型、哪些给 runtime"。

---

### F · #13 终止语义三件套（删"终止/terminal"；kill=标记+检查）

**开发者原话**：
> "为什么要提供'终止'？不理解——想不到任何一个地方需要终止语义。" ·
> "用户 kill 掉一个 session 或一次执行，只需要标记它被 kill 过，不意味着
> 被终止；用户还可以继续给它发消息。" · "子 agent 被用户 kill 后，parent
> 允不允许重启它/继续给它发消息——这确实是个问题，取决于具体状态。" ·
> "不需要一个'终止状态'：只要有 kill 标记；如果裁决 kill 后不许 parent
> 再发消息，那就基于标记直接做检查拒绝即可，不需要状态机。" · "三件套
> 疑似过度设计、过于复杂。"

**裁决**（决策 #30 / 裁决二）：**删除一切终止/terminal 状态与 `TaskCompleted`
独立事件**。close/kill = 带来源标记，只被自动路径检查。parent 能否复活
被 kill 的子按**来源**定（裁决二 C：用户 kill→仅用户复活；parent kill→
parent 复活），基于标记做检查，**无状态机**。

**文档改动**：
- DESIGN §18.7 `crash vs kill` / `显式重开` / `静止动作`〔码文皆改·文档
  侧〕已成文，保留。
- 决策 #25 "task 跑到完成是另一种形态"（见 Part VII）须标被 #31 取代。

**代码改动**〔待落地〕（详见 Part VI wire 变更）：
- **删** `event.TaskCompleted`（`types.go:26/248/583`）；其三个消费者
  （#15 通知时刻、driver 迭代判分、headless 退出码）改由**静止 +
  `SubagentCompleted` 回执**承载（裁决四）。
- `event.SessionClosed` **加 `Source{user|parent}`**（`types.go:257`）。
- `internal/agent/loop.go` 的 `state.Terminal(s.Session.Status)` 检查
  （:438）与终态判定改为"读标记"，删 `StatusCompleted` 终态语义。
- 自动路径（timer sweep / boot sweep）读标记 + 来源做复活判定；`send`
  路径无视标记。

**标注**：〔文改·码未改〕。

**验收**：真实 API——用户 kill 子→仅用户 send 能复活、parent 自动复活被
拒；parent kill 子→parent 可复活。无任何"terminal"状态出现在 fold。

---

### G · #追加 · yield/park（已废）与 task 概念（后台→handle；形态删除）

**开发者原话**：
> "yield/park 一族：如果 turn/session 里发消息、收到之后的状态本身就能
> 表征，那就够了——不需要引入这类额外概念。" · "task 概念被整体质疑：
> 什么是 task？不理解。session 和 turn 已经足够表征……为什么还要有 task？
> 有大量重复的嫌疑，而且 task 的确切定义从未说清。" · "简化，不引入冗余
> 概念。"

**裁决**：
- yield → final generation，park → 待命（〔码文皆改〕已废，§18.1 已记）。
- task **双义全删**：**后台义 → handle**（实现名 `task_id` 是 wire 细节）；
  **形态义 → 删除**（并入静止模型，决策 #31）。

**文档改动**：
- DESIGN §18.4 `后台任务（task）` 词条 → 改名 **`handle（后台工作）`**：
  "`background:true` 的 activity：handle（实现名 task_id = call id）立即
  配对返回；终态回流为带前缀 user-role 输入"。
- DESIGN §18.7 `等待注册表`：`WAITING_TASKS` → `WAITING_HANDLES`。
- DESIGN §18.10 踩坑表 `task` 行〔码文皆改·文档侧〕已改，保留；`run`/
  `loop` 行校准（见 Part VII）。
- SPEC B "spawn 一律非阻塞（阻塞路径已删除，零 legacy）✅" 与 DESIGN §17
  "阻塞 spawn……保留 v1 兼容"**直接矛盾**（〔文自相矛盾〕）——按 MR-1 删
  阻塞路径，§17 该句删除，SPEC ✅ 待码兑现后成立。

**代码改动**〔待落地〕：
- 删除**阻塞 spawn** 路径（若 code 仍存；grep 未见显著残留，实施时先核）。
- `state.Tasks` → `state.Handles`（字段改名，`internal/agent/*` 与
  `event` 状态）；`WAITING_TASKS` → `WAITING_HANDLES`。
- 工具名 `task_output`/`task_kill` 的去留：对外可保留（wire 兼容无义务，
  MR-1；但改名牵动 defs/*.json 与 CLI，属"能改则改"，列为可选清理项）。

**标注**：〔文改·码未改〕+〔文自相矛盾〕（SPEC B vs DESIGN §17 阻塞 spawn）。

**验收**：spawn 一律非阻塞；fold 里无 `task` 形态字样；`WAITING_HANDLES`
出现在等待注册表。

---

### H · #15 子 agent / 回执——投递时机（steer 同路；spec 级默认+override；不做 per-launch）

**开发者原话**：
> "既然已支持 steer……那么子 agent / background task 完成时应当能经同一
> steer 通道影响父的当前 turn……很多时候不希望等一个很长的 turn 结束后，
> 才处理早已完成的后台结果。" · "优先级：回执投递优先走 steer 式还是
> pending message 式？" · "如果要求 agent 在启动时逐个指定投递 mode，过于
> 复杂。" · "建议：agent 配置层选定一种 / 允许自主选择（默认值+override）。"

**裁决**：回执走 **steer 同路**（安全边界插入当前 turn，不等 turn 结束）
——**现状即如此**（§17 记"回执在安全边界进入"）。投递模式做成 **spec 级
默认值 + 可 override**，**不做 per-launch** 开关（太复杂）。

**文档改动**：
- DESIGN §18.6 `child_result` 词条：补"投递走 steer 同路（安全边界插入
  当前 turn）；投递模式为 spec 级默认 + 可 override，不做 per-launch"。
- SPEC B "完成回执激活父 turn（先回先处理）✅" 保留；补 spec 级 override
  字段登记（若做）。

**代码改动**〔部分已在·增量小〕：
- steer 同路投递〔码文皆改〕已在。
- **新增** spec 字段 `subagent_receipt_mode: steer|pending`（默认 steer），
  `internal/agent` 消费；〔待落地·小增量〕。

**标注**：〔码文皆改（主体）〕+〔待落地（spec 级 override 开关）〕。

**验收**：长 turn 进行中子完成→回执在下个安全边界即插入；改 spec 默认为
pending 时排队等 turn 结束。

---

### I · 裁决一（用户换 agent 免确认；子 agent 提权须用户 approve）

**开发者原话**（裁决一，已裁）：
> "原来给出的选项全部不要，按真实需求来"（并批评多处 over-design）·
> "用户切换 agent：不需要任何 review/确认。" · "子 agent 权限：默认不得
> 超过父；若一定要起权限超过父的子，必须通知用户 approve。"

**裁决**（决策 #32）：权限放宽的审批**只存在于"agent 提权自己的子"**，
**不存在于"用户自己的切换动作"**。

**文档改动**：DESIGN 决策 #32〔码文皆改·文档侧〕已成文；SPEC B "权限默认
不超父（请求超父须用户 approve）✅ 裁决一.2"、SPEC J "换 agent……用户免
确认 ✅ 裁决一" 保留（码兑现见下）。

**代码改动**〔文改·码未改〕：
- 用户切 agent（`ar agent`）路径**不弹审批**。
- spawn 子时若请求权限 ⊄ 父有效权限 → 触发**用户** `ApprovalRequested`
  （非父自动放行）；`internal/agent` spawn + `internal/pipeline/permission.go`
  的权限冻结交集（§18.6 `权限冻结交集`）加"超父→用户 approve"分支。

**标注**：〔文改·码未改〕。

**验收**：用户切写权限 agent 无弹窗；子请求超父权限→用户审批弹出、拒绝则
子降级到父权限交集。

---

### J · 裁决二（被 kill 的子按来源复活）

**开发者原话**（裁决二，已裁 C）：
> "按 kill 来源分：用户 kill 的，仅用户可复活；parent kill 的，parent 可
> 复活。"

**裁决**：见 INV-M2；实现依赖 **R6**（parent→existing-child 消息原语）。

**文档改动**：DESIGN §18.7 `crash vs kill`〔码文皆改·文档侧〕已含来源；
SPEC B "parent 可复活自己 kill 的子 ✅ 裁决二 C" 保留（码兑现见下）。

**代码改动**〔待落地〕：
- `SessionClosed.Source` / 取消事实带 `Source`（Part VI）。
- **R6 新原语**：parent 向已存在子 session 投消息（复活/续发）——现无此
  通道，须新增（否则裁决二无法实施）。
- 自动复活判定读 Source：用户 kill→拒绝 parent 自动复活；parent kill→
  允许。

**标注**：〔文改·码未改〕，且依赖 R6〔待落地〕新原语。

**验收**：同 F 的验收。

---

### K · 裁决三/四（task 形态删除；无独立完成事件——静止回执承载）

**开发者原话**（开发者给出的统一模型，消解裁三/四）：
> "不管以何形态启动，当 session 的最后一个 turn 结束、无在飞工作、且无
> 定时自触发（scheduled retrigger 未到期 = 未结束）——即没有别人会触发
> 它、它自己也不会再执行——就认为它结束了，此时应通知它的 parent。"

**裁决**：
- **裁决三**：task 形态概念**整体删除**。只有一种 session；"跑完交卷"不是
  形态；`ar run` = 便捷命令（R7）。
- **裁决四**：**不需要独立完成事件**。静止时通知 parent 用既有
  `SubagentCompleted` 回执承载；顶层无 parent 无通知，退出码由观察者从
  静止状态读出；**回执可多次发生**（R1）。outputs 发布/退出码挂在静止
  时刻，是 spec 声明的行为而非形态属性。

**文档改动**：
- DESIGN 决策 #31〔码文皆改·文档侧〕已成文；§18.1a event 对照表"静止
  回执"行已含"既有子回执，非新事件"，保留。
- SPEC "task 模式"相关行删除、"阻塞 spawn/await（v1 保留）"行删除（部分
  SPEC 已改，须逐行核）；driver 行文改"parent 靠静止回执判分"。
- GAPS G24〔码文皆改·文档侧〕已消解，保留。

**代码改动**〔待落地〕（静止模型核心手术，最大一块）：
- 删 `Loop.Conversational`（`loop.go:66` 及全部引用 :266/:428/:438/:475/
  :489/:703/:738/:1363、`epilogue.go:130`、`event.SessionStarted.
  Conversational`、~10 测试文件）——**只剩一种 session**（连带 R3 修复
  审计 C4）。
- `decide()`（`loop.go:738` 签名带 `conversational bool`）去掉该参数，
  统一为静止判定（INV-Q1）。
- epilogue（`epilogue.go`）的"task-form 自动交付"改为**静止动作**（INV-Q2
  时序），挂在静止时刻而非 `!Conversational` 分支。
- 删 `TaskCompleted`（Part VI）。

**标注**：〔文改·码未改〕（决策/SPEC/GAPS 已写目标态，代码全是旧两形态）。

**验收**：C1–C10 孪生 + QA-01..09 真实 API 在**单形态**下全绿；fork 续跑
（原审计 C4）可用；无 `Conversational` 符号残留。

---

## Part V · 审计 critical 在术语范围内的落地

| 审计项 | 与术语的关系 | 落地 | 标注 |
|---|---|---|---|
| **C1 空-parts 毒化** | = #5 统一 step 异常的实例（R4）；INV-C1 | 写入侧 `provider.go` 合成占位 + 请求侧 `gemini.go` 修补 + 回归测试 | **码已改·随本 INC 提交** |
| **C2 bash 越界** | 独立安全红线（与术语弱相关，但同批落地） | `internal/pipeline/permission.go` `hardFloor` 现只解析 file 工具 `args.Path`（:92-102），**须对 bash 的 `args.Command` 也做工作区逃逸检查** | 〔待落地〕 |
| **C3 凭据泄漏** | 独立安全红线；与决策 #15c 凭据红线矛盾 | `internal/tool/exec.go:148` `redact.FromEnv()` 只按环境变量值 redact，对文件内 token 无效；须接入硬排除表（.netrc/.npmrc）到 `read_file` 路径 | 〔待落地〕 |
| **C4 fork 吞消息** | **被静态模型手术连带消解**（R3：删 `Conversational` 后根因消失） | 无需单独修，随 K 的 `Conversational` 删除自动解决；须补 fork 续跑真实 API 验收 | 〔随 K 落地〕 |

> C5（daemon socket 路径）、C6（MCP 不可达）与术语无关，**超本 INC 范围**，
> 仅索引至 `docs/audit-2026-07-07/AUDIT.md`，另行排期。M1–M9 / m1–m13 同理。

---

## Part VI · 事件 wire 变更清单（决策 #18 版本化）

| 事件 | before | after | 破坏性？ |
|---|---|---|---|
| `TaskCompleted`（`types.go:26/248`） | 独立完成事件，driver/退出码/#15 三消费者 | **删除**；由静止 + `SubagentCompleted` 承载（裁决四） | **是** → 须 bump schema 版本（R5） |
| `SessionClosed`（`types.go:257`） | `{Reason, GenSteps}` | **加 `Source {user|parent}`**（INV-M2） | 否（additive-optional，可不 bump） |
| `SpecChanged` | 不存在 | **新增** `{NewSpecName/Path, PrefixEpoch, By(user|agent)}`（决策 #32） | 否（additive） |
| `SessionStarted`（`.Conversational`） | 带 `Conversational bool` | **删 `Conversational`**；记 event-schema 版本号 | **是** → bump |
| `state.Tasks` | 在飞任务列表 | 改名 `state.Handles`（#追加） | 内部状态，非 wire |
| 状态 `StatusCompleted` | 终态 | **删除**（无终止状态，决策 #30） | 内部状态 |

**版本 bump 具体号 = 开放设计点 D2（Part VIII）**，须开发者拍。原则：删除
类事件才 bump（决策 #18："additive-optional 字段不 bump"）；bump 后旧
journal 拒绝 resume（MR-1 零 legacy：原型无兼容义务，旧 journal 丢弃）。

---

## Part VII · DESIGN §18 术语表 + 决策表 全量编辑清单（逐条 before→after）

> 这是"改哪一行"的**精确 diff 指令**。行号以 `main` @ `a5a84f2` 的
> `docs/DESIGN.md` 为准（实施前重核，文档会漂）。

| 位置 | before（现文） | after（目标） | 出处 |
|---|---|---|---|
| §18.2 `interrupt`（~1140） | "idle 处 = close 会话" | "idle 处 = **no-op**（无在跑活动，无状态变化）" | #11 / D |
| §18.2 `Input`（~1137） | "tagged union 五种……全部 journal 为 InputReceived" | "对话面 = 内容 + 来源前缀；类型仅 journal 审计元数据。例外：前台 tool_result 保协议配对" | #9 / C |
| §18.3 `fold / state`（~1148） | "state = 纯 fold(journal)……" | 补"含 history 部分（喂模型）+ runtime 记账；LLM request = assemble(state, spec)" | #12 / E |
| §18.4 `后台任务（task）`（~1166） | "后台任务（task）：background:true 的 activity……task_id" | 词条改名 `handle（后台工作）`；"task_id" 标注为 wire 实现名 | #追加 / G |
| §18.6 `spawn`（~1187） | "阻塞与 background 两形态" | "**仅 background**（阻塞路径已删，MR-1）；立即返回 handle" | #追加 / G |
| §18.7 `等待注册表`（~1199） | "WAITING_INPUT / APPROVAL / TASKS / TIMER" | "……/ **HANDLES** / TIMER" | #追加 / G |
| §18.9 `spec / instance`（~1224） | "冻结于 SessionStarted" | "初始 spec 于 SessionStarted 立；经 SpecChanged 换代（决策 #32）" | #1 / A |
| §18.10 `task` 行（~1236） | 已改（"已废……形态删除"） | 保留，仅核对措辞 | #追加 |
| §18.10 `run` 行（~1233） | "已废：ar run 仅是便捷命令名" | 保留；补 R7 塌缩（run/new/send 语义） | R7 |
| 决策 #25（~1018） | "续聊是循环的默认形态(conversational)，task 跑到完成是另一种形态" | **标注被决策 #31 取代**；删"两形态"表述 | #31 / K |
| §17 实现注记 · interrupt（~1078） | "实现新增'待命处 interrupt = close'" | **整条删除**（idle=no-op） | #11 / D |
| §17 实现注记 · 三套子执行（~1076） | "阻塞 spawn、后台 spawn、driver 子系统并存（阻塞路径……保留 v1 兼容）" | **删阻塞路径**表述；收敛为"后台 spawn + driver" | MR-1 / G |
| §17 实现注记 · inbox 字面（~1070） | child_result/tool_result "机制上暂由 background activity 兑现……字面统一待定" | 按 #9 弱类型化收敛为"user-role + 前缀"，更新此注 | #9 / C |

---

## Part VIII · 跨文档一致性修复（doc-vs-doc / doc-vs-code 矛盾清单）

本 session 盘点发现的**过度声称与陈旧行**，逐条校准（这些是"文档说 ✅、
代码没有"的信任裂缝，MR-4 的直接产物）：

1. **SPEC 过度声称 ✅**：SPEC A/B/J 多行标 ✅ "落地"（静止模型 / spawn 非
   阻塞 / interrupt no-op / 换 agent / 静止动作），但对应代码未改
   （`Conversational` 在、`TaskCompleted` 在、`SpecChanged` 无）。
   **修复**：这些行降级为 🟡 "设计落地·码待落地"，或在完成对应代码手术后
   再置 ✅。**不许 code 未动时挂 ✅**（SPEC 维护纪律：验收锚必须指到真实
   测试）。
2. **SPEC B ↔ DESIGN §17 阻塞 spawn 矛盾**：SPEC "阻塞路径已删除 ✅" vs
   §17 "保留 v1 兼容"。**修复**：按 MR-1 删阻塞路径，§17 该句删，SPEC ✅
   待码兑现。
3. **GAPS §1 速览陈旧**：UJ-03/UJ-04/UJ-11/UJ-18/UJ-22 在 §1 仍标 ❌，但
   §2 对应 G6/G1/G8/G2/G23 多已 ✅ 关闭或登记。**修复**：§1 随 §2 **重
   打分**（登记纪律要求"§1 速览随每轮设计修订重打分"）。
4. **GAPS G6 关闭注引用将删符号**：G6 关闭位置写 `Loop.Conversational` /
   `RunStarted.Conversational`——正是静态模型要删的字段。**修复**：G6 关闭
   注改为静止模型口径（"唯一 session 形态；答完待命；close 才标记"），不
   引 `Conversational`。
5. **SpecChanged 三处声称落地、code 无**：决策 #32 / SPEC J / GAPS G8 都
   写 `SpecChanged` 事件已落地，但 code grep 确认**不存在**。**修复**：
   要么实现 `SpecChanged`（A 的代码改动），要么把这三处口径改为"设计定案·
   待实现"。**二者择一，不许悬空**。

---

## Part IX · 开放设计点（实施前需开发者裁决）

以下**不在** REVIEW-001 已裁范围，是落地时才浮现的选择题，须开发者拍板
（MR-2 自顶向下：呈现高层影响，等裁决再动）：

- **D1 · 回执投递默认值**：#15 的 `subagent_receipt_mode` 默认 `steer` 还是
  `pending`？（本文暂设 steer。）
- **D2 · event-schema 版本号**：删 `TaskCompleted` 后 bump 到几？旧 journal
  处置（MR-1 = 直接丢弃）确认？
- **D3 · R6 parent→child 原语的工具形态**：是新 tool（如 `send_to_child
  {handle}`）还是 CLI-only（`ar send <child-sid>`）还是二者？影响模型可否
  自主向子续发。
- **D4 · R7 CLI 塌缩范围**：`ar run`/`submit`/`resume` 保留几个？`ar run`
  是否直接实现为"new+send+wait-quiesce+read"的组合壳？
- **D5 · R2 prefix 换代点**：`SpecChanged` 换 agent 时 cache epoch 与
  thoughtSignature 作废的精确时机（换代事件落盘时？下一 assembly 时？）。
- **D6 · task_output/task_kill 工具改名**：是否连带改为 handle_output/
  handle_kill（MR-1 能改则改）还是保留 wire 名？
- **D7 · goal 会话内形态（G23/UJ-22）**：与静止模型的耦合——goal 检查点
  住在"final generation 该出现处"，verifier 不满足→反馈作为程序来源 input
  进 inbox→同一上下文下一 turn。此为**独立增量**（G23，走不变量流程），
  本 INC 仅标注其与静止模型的接口，不在此实施。

---

## Part X · 执行顺序与验收门

**建议顺序**（每步过 `./scripts/check.sh` 全绿 + 相关文档行齐活，MR-5 即
时 commit+push）：

1. **C1 空-parts**（码已改）+ 回归测试 → 提交。**先堵会话死亡主干裂缝。**
2. **C2 bash 越界 + C3 凭据泄漏** → 安全红线，独立可做。
3. **词汇与清理（G）**：task→handle 字段改名、删阻塞 spawn、`WAITING_
   HANDLES`；纯机械，先行降噪。
4. **静止模型核心手术（K）**：删 `Conversational`、`decide()` 去参、epilogue
   改静止动作、删 `TaskCompleted`、schema bump（连带修复 C4）。**最大一块，
   走不变量流程。**
5. **标记+检查复活（F/J）**：`SessionClosed.Source`、来源复活判定、R6 新
   原语。
6. **换 agent + 提权（A/I）**：`SpecChanged` 事件、`ar agent`、prefix 换代
   （R2）、子提权→用户 approve。
7. **Input 弱类型化（C）**：对话面渲染收敛（守住 INV-X1 配对红线）。
8. **#15 投递 override（H）**：spec 级 `subagent_receipt_mode`。
9. **§18 术语表 + 决策 #25 + §17 全量编辑（Part VII）** + **跨文档校准
   （Part VIII）** → 文档终检。
10. **验收门**：C1–C10 scripted 孪生全绿 + QA-01..09 真实 Gemini API 全绿
    （尤其新增"深轮次 20+ / 生成型任务 / fork 续跑"用例，补审计盲区）+
    §18/SPEC/GAPS 三文档无残留、无过度声称、无自相矛盾。

**双闸门测试纪律**（PROCESS.md）：每项落地既过 scripted 孪生（确定性闸门）
又过真实 API（QA 闸门）；两门齐绿才算"一步完成"。

---

*本文与 REVIEW-001 的分工：REVIEW-001 = 意见与裁决的原始台账（归档前只读
证据）；本 INC = 据裁决的施工说明（落地完成后与 REVIEW-001 一并归档
`docs/archive/`）。落地进度以 SPEC/GAPS/DESIGN 三活文档为准，本文不重复
记状态，只定"改什么、为什么、验收锚"。*
