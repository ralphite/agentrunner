# AgentRunner — 增量与决策台账（LOG）

> 接续 v1/v2 两本 PROGRESS 台账(已封存于 archive/)。记录纪律:每个
> 增量、每个影响后续工作的决策,落地时在此追加一条;只追加,不改写。
> 条目格式与流程见 PROCESS.md。

---

## 2026-07-05 文档收敛:单目录 + 三层模型(本仓库文档体系重构)

**背景**:v2 计划关闭后,仓库积累了 11 份根目录/v2 目录混放的文档,
其中多数已完成使命;两份 DESIGN(v1 架构 + v2 中心模型)并存构成
最大的过时/冲突风险。开发者提出以三层结构管理产品定义:
① user journey(端到端,定义产品)→ ② spec(功能点,分层拆分)→
③ 架构设计;增量需求走"三层 delta 完全明确后再开发",以 journey 级
端到端测试验收。

**动作**:
1. 全部文档收敛进 `docs/`;已完成计划(v1 S1–S7、v2 M1–M5+收口)与
   旧审查件封存进 `docs/archive/`(头部加归档注记,正文不动)。
2. QA 脚本(可执行测试资产,非文档)从 `v2/qa/` 迁至顶层 `qa/`,
   路径引用同步修正。
3. v1/v2 两份 DESIGN 合并为唯一的 `docs/DESIGN.md`(v2 中心模型为
   骨架 + v1 仍有效的子系统契约逐字并入;原件归档)。
4. 新增 `docs/PROCESS.md`(三层模型与增量流程)、`docs/SPEC.md`
   (第二层功能点登记簿)、本台账;CLAUDE.md 指向新体系。

**决策记档**:
- 活文档收敛为 7 份:PROCESS / JOURNEYS / SPEC / DESIGN / QA / GAPS /
  LOG,职责与冲突裁决顺序成文于 PROCESS.md。不另立 INVARIANTS 文档
  ——不变量住在 DESIGN 的"已定决策"表与各章粗体条款,避免文档增殖。
- DESIGN 合并纪律:**只重组、不改语义**——契约条款逐字保留;v1 中被
  v2 取代的表述(run=task-to-completion、阻塞 spawn 为唯一形态、旧
  分层图)以 v2 文本为准重写,取代关系在合并处注明。不变量零变更,
  不触发不变量变更流程。
- 归档件内部相对引用不修复(保持历史原貌),纪律成文于 archive/README。

## 2026-07-05 需求登记:UJ-21 崩溃自愈与重启接续(G22,积累不实施)

开发者进入"功能点积累"模式:新 journey/功能点先登记,攒够一批再统一
排期实施——本条及后续同类登记只动第一层(JOURNEYS)与审计件(GAPS),
不动 DESIGN/SPEC(那是增量实施时的事)。

**需求原文(摘要)**:①子 actor crash 后 parent restart,状态是否
全保留、能否像没 crash 一样继续;②整个应用 crash/机器重启后,是否
自动接续未完成任务;③用户 kill 与 crash 是不同语义,kill 的不得自动
重启。

**分析结论(记档)**:
- 恢复语义已有(journal/fold 无损重建、in-doubt 纪律、settle-from-
  child-fold、send 复活形状把关、kill/close 终态判别,QA-08 验证);
  缺的是**自动性**:boot sweep、子 session crash 自动 resume、
  on_child_failure 泛化、屡崩升级策略。
- "像没 crash 一样"刻意不承诺——非幂等副作用绝不静默重跑(决策 #6)
  是红线;承诺为"不丢历史/不丢输入/安全边界继续/事实对模型可见"。
- **不采用** supervision tree 自动 restart(与原则 6 冲突);表述统一
  为 restart = resume。kill≠crash 由 journal 终态形状天然判别,机制
  已在,待成文为不变量。

登记位置:JOURNEYS UJ-21(+§5 索引四条)、GAPS §1 新行 + §2 G22
(监督与恢复分节)。

## 2026-07-05 需求登记:UJ-22 会话内目标(G23)+ 需求丢失事故记档 + turn 术语澄清

**事故记档(需求丢失)**:开发者指出"goal 挂在当前会话、context 必须
延续"是项目原始需求之一,但现有 goal mode 实现为 IterationDriver +
fresh child run(context 不延续),需求丢失。根因链:
1. 原始需求从未成文为 journey——JOURNEYS 目录是后来按"已实现/已设计
   形态"整理的,UJ-15(通宵冲目标)按 driver 形态倒写,倒果为因;
2. v1 没有 conversational 会话形态(G6 是 v2 才关的最大缺口)——goal
   在 S6 设计时,"attach 到会话"根本无处附着,只能长在 task 形态上,
   fresh-run 的工程理由(prompt cache/隔离/防污染)随决策 #21 固化;
3. GAPS 审计以 JOURNEYS 为标尺——需求不在标尺上,审计发现不了丢失,
   UJ-15 反而被判"最强区"。
**流程对策**:原始需求必须第一时间落 journey(第一层),否则处于审计
盲区——这正是三层流程要堵的洞;本次补登记即执行。

**开发者裁定(实施时走不变量变更流程)**:决策 #21 / DESIGN §13 的
fresh-run 教义对 goal 形态不适用;goal 的 context 必须延续。fresh-run
保留给 best-of-N(隔离是其语义)与批式 loop。

**术语澄清(记档,增量落地时在 DESIGN §4 成文)**:
- 设计/代码的 **turn** = 一次模型调用 + 该次调用返回的全部 tool call
  的执行(loop.go decide()/state.Run.Turn;TurnStarted 逐次落盘)。
  agent 连续调用工具 = 多个 turn:每次带工具结果回到模型是新 turn。
- 用户语义的"turn" = 一条用户输入 → agent 干到 yield 待命的整段,
  即代码里已有的 **exchange** 概念(conversational 的 max_turns 预算
  就是 per-exchange 计的,从 LastInputTurn 起算——防 runaway)。
- 两者都合法,词汇冲突已澄清;对话中说"turn"以用户语义为准,设计文
  档内部保持现定义并补 exchange 术语。in-session goal 的检查点在
  **exchange 边界**(yield 点),不挟持模型调用级 turn——用户方案 (b)
  ("不满足就不让 turn 结束")在 exchange 语义下正是目标形态,与
  (a)/(b) 之争消解。

登记位置:JOURNEYS UJ-22(硬性要求粗体写入)+ §5 索引三条;GAPS §1
新行 + §2 G23(驱动与时间旅行分节,含冲突声明与控制面/预算形态)。

## 2026-07-05 术语调研裁定:turn = 对话级(用户语义);内部单位改称 step

对 turn 一词做外部调研(记档,修订上一条的临时裁定"设计内部保持现
定义"):
- **对话级(经典、主流)**:对话分析(Sacks et al. 1974 turn-taking)
  ——turn = 一个说话人持续到让出话轮的整段贡献;LLM 语境的
  "multi-turn conversation" 全部沿用此义(user/assistant 交换)。
  用户的理解与此完全一致:被调用起 → 干到最后一条消息停下 = 一个 turn。
- **agent SDK 圈的挪用**:OpenAI Agents SDK 明文 "a turn is defined
  as a single interaction with the LLM, including any subsequent tool
  executions or handoffs"(= 本项目现定义);Claude Agent SDK 的
  maxTurns 只数 tool-use turns(最终纯文本响应不计)——SDK 之间自身
  就不一致。本项目 v1 借的是 SDK 义。
- **内部步进单位的业界通名是 step**(smolagents/LangGraph/AutoGPT)。

**裁定**:本项目术语与主流对齐——
- **turn**(对话级)= 一次输入激活 agent → 干到 yield 待命的整段
  (即代码现称 exchange 者);
- **step** = 一次模型调用 + 该调用返回的全部工具执行(即设计/代码
  现称 turn 者);
- spec 的 max_turns 语义即 "per-turn 的 step 预算"(代码已按
  per-exchange 计,语义不变,仅换名);
- **event/wire 名不改**(TurnStarted 等):改名作废全部旧 journal,
  违背决策 #18 的成本判断——作为 wire 遗留名记入 DESIGN §17 名词
  对照,与 spawn_agent/task_kill 同例。
- 文档层(DESIGN §4/§17、SPEC、QA)的措辞统一随下一个增量落地,
  不单独起一轮全文替换;此前对话与文档中的 turn 按上下文读。

## 2026-07-05 术语裁定修正:废除裸 "step",改用 generation step / tool step

上一条裁定把 step 定义为"一次模型调用+其全部工具执行"(捆绑),开发
者质疑;外部调研结果:**step 一词行业内同样分裂**——
- **捆绑派**:smolagents 的 ActionStep(一步 = thought/LLM completion
  + tool 执行 + observation,max_steps 数这个)、ReAct 轨迹的
  thought-action-observation 步。上一条裁定借的是这派,但并非共识。
- **分立派**:LangGraph 的 super-step(agent 节点=LLM 调用与 tools
  节点是**不同的** step,checkpoint 落在 super-step 边界)、tracing/
  observability 惯例(LLM span 与 tool span 分立)、RL 语境(env
  step = 动作执行)。开发者的理解与此派一致。
- 另注意:ML 推理文献里 generation/inference step 常指 **token 级**
  decoding step——agent 语境不用此义,但引用外部文献时须防混淆。

**修正裁定**:
- **裸词 "step" 不单独使用**(行业歧义),必须带限定词:
  - **generation step**(模型调用步)= 一次完整的模型调用(inference
    request);
  - **tool step**(工具步)= 一次工具执行(journal 里本就是一等
    activity)。
- 捆绑单位**不再赋予专名**;需要指称时写"一个 generation step 及其
  tool steps"。设计/代码旧词"turn(内部)"精确映射为 generation
  step——计数本就一致:state.Run.Turn 每次模型调用递增,工具执行不
  递增;"turn 边界"= generation step 之间的决策点(decide()/
  assemble,LangGraph 的 checkpoint-at-super-step-boundary 同构)。
- max_turns 语义 = **per-turn 的 generation step 预算**。
- turn(对话级)与 event/wire 名的裁定不变(见上一条)。

**流程教训(已入 PROCESS 执行协议)**:新术语(或改变既有术语含义)
入档前必须先做外部主流用法调研;与主流对齐,或写明偏离理由;裸词有
行业歧义时带限定词。本次 step 是反面教材:裁定 turn 时顺手引入,
未调研即入档,当天被推翻。

## 2026-07-05 DESIGN 新增 §18 术语表(canonical)

应开发者要求盘点系统全部核心概念,成文为 DESIGN §18(18.1–18.10,
含易混词踩坑表)。定位:全部术语的唯一定义处,与本文其余章节旧措辞
冲突时以 §18 为准;全文措辞统一仍随下一增量。纯文档新增,无语义
变更,不触发不变量变更流程。

## 2026-07-05 术语表 18.1 清理(开发者 review 四条意见落地)

1. **命名原则成文**:术语优先与产品功能关联;实现侧词汇(park/
   decide/WAITING_*)只作锚注,不充当术语。写入 §18.1 表头。
2. **session** 定义改为产品义领句(对标 Claude Code 的一次会话),
   actor 结构降为实现注。
3. **loop 歧义暴露并入踩坑表**:①会话循环(session 心脏)②loop
   mode(driver 定时系列 schedule)③代码 Loop 结构体。开发者对
   "决策点与 loop 的关系"的困惑源于 ①② 相撞——新增术语**会话循环**
   指 ①,裸词 loop 避免使用。
4. **yield/park 废除**(太 technical):yield → **final generation**
   (turn 的最后一个 generation step,不带 tool call,标志 turn 结束,
   开发者命名);park → **待命**。"决策点"更名 **安全边界**(产品义:
   一切外部影响只在 step 之间生效),decide() 降为实现锚注。
5. **新增 §18.1a step↔event 对照**:step 是执行模型词汇,event 是
   持久化词汇(event sourcing 记录单元);step 不是 event,是"产生
   某一小簇 event 的那段执行";逐 step 给出 event 簇对照表。

## 2026-07-05 大清理落地(开发者指令:零 legacy,文档与代码同步更新)

开发者裁定:基于当日已确认的全部概念裁定,大幅更新文档与代码,
**不保留任何 legacy、不做兼容 mapping**;旧 journal 直接作废(决策
#18 本就不做 migration,原型可重跑)。执行分四步(C1 文档/C2 代码
改名/C3 终止语义手术/C4 对账),每步 check 绿提交。

**C1(本条)文档总更新**:
- DESIGN 全文措辞统一至 §18 术语表:旧内部"turn"→ generation step;
  "turn 边界"→ 安全边界;yield/park/exchange → final generation/
  待命(全文清除);§1 重写(session = journal + 待命,每条输入触发
  一个 turn = 一遍 agentic loop;"会话循环"概念取消);§4 开头重写。
- 命名以实现名为 canonical:spawn_child/cancel_child/ChildSpawned →
  spawn_agent/task_kill/SpawnRequested,§17 的两张对照表删除。
- 事件改名(代码 C2 兑现):TurnStarted/TurnDiscarded →
  GenerationStarted/GenerationDiscarded;RunStarted → SessionStarted;
  spec 字段 max_turns → max_generation_steps;RunAgent → StartSession。
- **终止语义成文(不变量变更,开发者直接裁定即 review,PROCESS §四
  记录)**:待命是唯一静止状态;"结束"是 journal 形状不是状态;只记
  两类事实——意图(SessionClosed/kill,只挡自动恢复)与回执
  (TaskCompleted,交付时刻不封印);显式 send 对任何 session 都是
  合法重开。决策表 #24 改写、新增 #30;RunEnded 从设计中移除
  (C3 兑现)。§6 恢复/§12 session 管理/§18.1a/18.7 同步。
- SPEC/QA/JOURNEYS/GAPS 措辞同步(G23/UJ-22 的 yield/exchange 遗留
  用语更新;G8 SessionStarted)。

**C2 代码改名(行为不变)落地**:
- 事件:TurnStarted/turn_started → GenerationStarted/generation_started;
  TurnDiscarded → GenerationDiscarded;RunStarted/run_started →
  SessionStarted/session_started(RunEnded 随 C3 处置)。
- state:子状态 Run → Session(json "run"→"session",SubStateVersions
  键同步);字段 Turn → GenStep(json "gen_step")、LastInputTurn →
  LastInputGenStep;各事件载荷 Turns → GenSteps。
- spec:MaxTurns/max_turns → MaxGenerationSteps/max_generation_steps
  (fixture/golden 同步改名)。
- protocol:KindRunStart/run_start → KindSessionStart/session_start;
  KindTurnStart → KindGenerationStart/generation_start;Event.Turn →
  N(generation step 序号或 iteration 序号)。
- 词汇:park* → idle*(含测试名),doWaitInput → doIdle;CLI 展示
  turn → gen-step。注:Go range-over-func 的 yield 参数名是语言惯用词,
  保留(一次误替换已回滚)。
- 旧 journal 自此不可读(事件名/子状态键变更),按决策 #18 作废重跑,
  不做 migration。qa 脚本的 generation_started/session_started 同步;
  run_ended 断言随 C3 更新。全量 check + e2e 绿。

**C3 终止语义手术落地(决策 #30 兑现)**:
- event:RunEnded 拆除 → `TaskCompleted`(task 形态 epilogue 的交付
  回执)+ `SessionClosed`(close 的意图记录);fold 状态 ended 拆为
  completed/closed,新增 `state.Terminal()` 判定;GenerationStarted
  令 Status 回 running(重开后形状诚实)。
- **conversational 不再有终态事件**:per-turn step 预算耗尽从"终结
  session"改为**可见截断**——journal `LimitExceeded{kind:
  generation_steps}`(fold 借此重置预算基线,排队输入开新 turn,无
  silent-wedge)+ 回待命;decide 新增 doTruncate 分支。
- **显式重开**:daemon send 对 conversational session 一律成立,
  **含已 close 的**(意图只挡自动路径;timer sweep 对 terminal 一律
  跳过);loop resume 守卫只拒 task 形态。新测试:
  TestSendReopensClosedConversational / TestSendToCompletedTaskRefused。
- **记档余项(明日 review)**:①task session 的显式重开未落地(重开
  后形态如何——升格 conversational 还是原形态续跑,涉及 epilogue 重复
  执行语义,需要裁决);②`ar close` 对 task 形态、`interrupt` 在
  task 收尾语义不变;③CLI 子命令名(run/resume 等)沿用,"run"仅
  从叙述词汇中废除——命令面改名待裁决;④QA-01..09 真实 API 闸门
  在本环境无凭据未重跑,断言已同步(session_closed),回归以
  scripted 孪生 + acceptance 26 场景全绿为据。
- crash.PointBeforeRunEnd → PointBeforeTerminal;accept 框架终态校验
  改为 task_completed|session_closed。全量 check + e2e + 三包 -race 绿。

**C4 对账收尾**:
- SPEC:新增终止语义行(✅,决策 #30);"send 即复活"行改写为
  "显式重开"(🟡,task 形态待裁决);GAPS 新增 G24(task 显式重开的
  形态问题)。
- 残留扫描:代码/脚本/文档零 run_ended/RunEnded/max_turns/park/
  exchange 残留(archive 与 LOG 历史条目除外——归档纪律不改历史);
  acceptance 场景 fixture 的 sub_state_versions 键 run→session 修正。
- 教训记档:check.sh 经管道取尾时退出码失真,两次未拦住未格式化
  提交(已补两个 gofmt 修复提交)——后续闸门命令一律直跑取
  退出码,不接管道。
- **大清理四步(C1–C4)全部完成**:文档与代码零 legacy 对齐 §18 术语
  表与决策 #30。待明日 review:①术语表 18.2–18.10 逐节;②G24 task
  重开形态;③CLI 子命令名(run/resume 等)是否随词汇改;④QA-01..09
  真实 API 重跑(本环境无凭据)。

## 2026-07-05 Journey 真实验收(大清理后首次全量真机验证)

开发者提供 GEMINI_API_KEY(.env,永不提交),按 QA.md 纪律做 journey
级真实验收:真实 API(gemini-flash-latest)+ 真实工具 + SHA 钉死的
真实仓库(fatih/color、spf13/cobra+注入 bug、gin、blank)。

**结果:12/12 PASS**——QA-01..09 官方闸门全绿(QA-08 一条已记档
措辞级 WARN,不设闸)+ 三条补测:T1 UJ-01 即问即答(答案精确到
文件:函数并核验属实,零写操作)、T2 UJ-15 goal 驱动(1 轮迭代修复
注入 bug,verifier 独立复核绿)、T3 UJ-16 best-of-2(隔离 worktree
选优,胜者留盘)。

**验证意义**:这是术语重命名+终止语义手术后的第一次真实 API 全量
回归——session_closed 恰好一次、task_completed 回执(driver 子迭代
链路)、send 复活、崩溃三态矩阵全部按新语义在真实链路落盘,零回归。

**本轮发现**:①qa 脚本 .env 相对路径为搬家前旧值(已修复推送);
②优雅停机对在飞 LLM 的协作取消留下 activity_cancelled 实录(设计
承诺兑现);③QA spec 均 allow-all——审批/权限 journey(UJ-06/08)
未被真实压到,建议补 QA-10..12(审批/interval 值守/注入对抗);
④Anthropic 第二 provider 无凭据未测。报告 artifact:journey-验收-v1;
运行留档 qa/runs/20260705-real/(gitignored 本机)。
