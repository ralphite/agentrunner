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

## 2026-07-06 REVIEW-001 落地开工(开发者指令:全部裁决落码,文档代码同步)

不变量变更记录(开发者裁决即 review,PROCESS §四):
- 决策 #24/#29/#30 改写、#31 静止模型、#32 换 agent 与提权 新增;
  TaskCompleted 事件删除、"终止/terminal"词族废除、task 形态删除、
  恢复单一自愈、阻塞 spawn 删除(零 legacy)、待命处 interrupt=no-op。
- D1 本条:DESIGN §1/§2/§6/§12/§13/§18 + 决策表成文;SPEC 行改写;
  GAPS G8/G24 关闭。后续 D2-D5:核心手术/词汇清理/新能力/QA 复跑。

## 2026-07-07 INC-1 子会话寻址(观察面树形完备)

- 需求:驾驶舱用户需要"link to open subagent session"——在飞子 run
  的内部过程此前在观察面上完全不可寻址(resolveSessionDir 只扫顶层,
  inspect 树只收录已 settle 的子)。
- 落地:resolveSessionDir 支持 child_session 全 id(`-sub-` 分段映射
  `sub/` 目录,任意深度);events/--state/inspect/ps/attach-replay 全部
  自动生效。scripted:TestResolveChildSessionDir / TestEventsChildSession。
  工作纸 INC-1 归档。
- 记档:internal/tool 的 TestBashCancelLeavesNoSessionOrphans 在本增量
  之前的 main(4974932)上已 FAIL(D 系手术中间态,沙箱内外皆挂),
  与本增量无关,留待 D 系收口。
- 后续增量(已在 web/PROGRESS.md 提案区):P1② 子事件进 attach 流
  (childLoop 接 Out sink);P2 父/用户→在飞子的第二条消息(子 inbox)。

## 2026-07-07 INC-2 新手第一公里:黑盒 QA 基础工作流修复

**背景**:2026-07-07 黑盒 QA(零知识新用户视角)确认 7 个基础工作流
硬伤(BB-me-1…7,工作纸 archive/increments/INC-2 有全文):顶层
`--help` 报 unknown command、无 README、spec 格式无从发现且报错泄漏
内部类型名、**`ar new`/`ar send` 完全不显示 AI 回复**(只给 id /
"delivered")、attach 不可发现、run 与 new/send 输出行为相反、daemon
隐性前提报错不指路。UJ-01/03 的"用户提问 → agent 答完"在 CLI 对话
形态下事实不成立。

**动作**(三个提交):
1. 可发现性包:顶层 `help`/`--help`/`-h`(分组 + Quick start)、
   `ar init`(带注释示例 spec,拒绝覆盖,模板以测试钉住可加载)、
   spec 未知字段错误改写(去内部类型名、附合法字段清单与 init 指引,
   golden 更新)、daemon dial 失败附启动指引、attach usage 说明回放+
   跟随语义、裸 `sessions` 等价 `sessions list`。
2. 回复可见性包:`new`/`send` 默认跟随本轮渲染回复正文至 idle 再
   detach(尾行提示 send/attach);send 线协议加 `follow`(daemon 先
   订阅 hub 后投递,ack 照旧,随后转发直到客户端断开);`--detach`
   恢复纯异步。daemon.DialUntil 支持客户端側停止。
3. README.md(用户入门)+ 三层文档收口 + qa 脚本改用 `--detach`
   (脚本语义 = 投递即回,与忙时插话场景的时序假设一致)。

**决策记档**:
- `new`/`send` 的 stdout 从 id/"delivered" 变为回复正文——**面向人的
  默认**;脚本消费方一律 `--detach`(qa/ 已同步)。
- send follow 的订阅先于投递,保证回复事件零窗口;detach 即断连,
  与 attach 同一语义,订阅不改结果(§15 不变量零触碰)。
- 预存环境性测试失败(unix socket 路径超长,/var/folders 长随机段
  下 bind/dial invalid argument)与本增量无关,已另立任务修测试基建。

## 2026-07-08 REVIEW-001 落码完成(D2–D4:静止模型/词汇清理/新能力)

D1(2026-07-06 文档先行)之后,全部裁决落到代码,四个提交
(REVIEW-001.D2/D3a/D3b/D4a/D4c),每步 check.sh 全绿:

- **D2 静止模型核心手术**:Loop.Conversational 与 task 形态删除
  (decide 单一路径;"静止后 drive 返回还是继续待命"由 UserInputs 接线
  事实决定,非 session 属性);TaskCompleted 事件与 state.Terminal 删除,
  静止=journal 形状(新 `state.Quiescence`,driver 结算/crash 恢复/
  acceptance 校验/观察面共用);静止动作 = auto_publish→barrier(可重复),
  父回执由 launcher 从 drive 返回值投递;可见截断家族
  LimitExceeded{tokens|generation_steps|malformed_tool_call} +
  AssistantMessage.Finish=blocked,统一 step 异常处理(裁决 #5 消解),
  截断只被"截断后到达的输入"重启(每次唤醒一次尝试,TruncatedMsgCount
  精确判定);close/kill = SessionClosed{reason, source: user|parent}
  标记,kill 来源经 context cause(errs.KilledError)传递,自动路径查
  标记、显式 send 永远放行;待命处 interrupt = no-op(裁决 #11);恢复
  单一自愈(InDoubtError 仅剩 hooks 半跑窗口)。sub_state "session"
  版本 bump 2,旧 journal 按决策 #18 作废。
- **D3a 阻塞 spawn 删除**:spawn_agent 一律非阻塞(background 参数
  删除);handoff 保留同步执行(控制移交语义);同步可判定的 spawn
  失败立即以 error result 配对。scripted fixture YAML 支持 routes
  (G4 基建 YAML 化),并发子场景按请求形态路由,s5 acceptance 三场景
  改写。
- **D3b task 词汇全删**:工具 task_kill/task_output → kill/output;
  载荷 task_id → handle;fold Tasks → Handles;BarrierTask →
  BarrierHandle;对话前缀 [background task] → [background work]。
- **D4a 换 agent(决策 #32)**:SpecChanged 事件(新 spec + 重冻结
  prefix 块 + permission layers)+ `ar agent` 命令;daemon `agent`
  命令释放 hosted loop(per-run cancel);resume/revival 装配取最新
  SpecChanged。用户切换免确认。G8 真关闭。
- **D4c receipts 投递模式(裁决 #15)**:spec 字段 receipts:
  steer(默认)/turn_end,agent 配置层,不做 per-launch。
- **裁决一.2 子提权**:冻结交集使"超父"结构上不可能(比政策更强);
  提权申请通道无表达面、无 journey 压需求——政策条款入 DESIGN,
  通道 🧊 记档(自顶向下:无需求不设计)。
- **附带修复**:TestBashCancelLeavesNoSessionOrphans 跨平台化
  (pgrep 唯一时长,macOS 无 /proc,预存失败清偿);daemon socket
  测试路径超长为预存环境问题(TMPDIR 短路径下全绿,测试基建任务
  另立跟踪)。

**行为语义变化记档**(均 opt-in 于新语义,旧 journal 作废):
- final generation 时在飞后台工作不再被默认 cancel——session 待命等
  settle,善终后才静止(原 on_run_end: cancel|await 与 await_timeout
  spec 字段删除);
- 后台 spawn 预留期间父自身 LLM 可能被预算闸截断——可见截断 + 回执
  settle 后自然重试,树预算语义的正确后果;
- blocked/malformed 不再终结 session:可见截断 + 待命,send 可再试。

## 2026-07-08 REVIEW-001 收口:真实 API 全量复跑(B 闸)

静止模型手术后的第一次真实 API 全量回归(gemini-flash-latest,
真实工具,SHA 钉死仓库):**QA-01..09 全部 PASS**——kill 工具改名、
静止回执、标记语义、非阻塞 spawn、可见截断在真实链路零回归;
session_closed 恰好一次、crash 三态矩阵按新语义落盘。

新增 **QA-10 session 内换 agent**(qa/run-qa10.sh,首跑 PASS):
诗人 → `ar agent`(免确认)→ 审计员,同一 journal、上下文延续、
spec_changed 恰好一条——裁决 #1"session 不与 agent 绑定"的
端到端证词。运行留档 qa/runs/20260708-review001/(gitignored 本机)。

余项记档:①daemon socket 单测在超长 worktree 路径下 bind 失败为
预存环境问题(生产代码已有短路径回退;TMPDIR=/tmp 下全绿),测试
基建修复另立任务;②Anthropic 第二 provider 无凭据未测(与
2026-07-05 同状态)。

---

## 2026-07-09 · interrupt 真停 / 真转向(核心 bug 修复,对齐 DESIGN §1)

黑盒 QA(以 Claude Code 级别体验为标尺,真 repo + 真 Gemini 驱动 web
驾驶舱)发现:运行中 interrupt 只 cancel 当前活动就重跑同一 turn——
既停不下跑飞的 turn(实测 gin 逐文件长跑,连按两次 interrupt 仍跑到
gen_steps 预算 40 才停),也让忙时排队的 steer 要等整轮自然结束才
可见。与 DESIGN §1「interrupt cancel 当前 turn」「steer 下个 turn
模型看到它」相悖——是实现 bug,非不变量变更。

修(internal/agent/loop.go):新增 `finishInterrupt`——steering
interrupt 后落一条 `LimitExceeded{kind:"interrupted"}` 收尾 turn,
再 `drainQueued`。decide() 据 TruncatedAtGenStep 走 idle-或-重启:
有排队 steer(落在截断标记之后,len(Messages)>TruncatedMsgCount)
→ TruncationRestartable 重启转向;无 → doIdle 交还控制。LLM 相与
工具相两处 interrupt 汇合点统一走此路。复用既有可见截断机制,不新增
事件类型。

测:更新 TestSteeringInterruptDuringLLM(interrupt 现停 turn、模型不
被重跑、落 interrupted 截断)+ 新增 TestSteeringInterruptRedirects
(排队 steer 触发新 turn 转向,模型确实看到 steer)。真验(origin/main
+ 本修 + 真 Gemini):gin 逐文件长跑,gen=24 时 interrupt → 落
interrupted 截断 → 待命(gen 冻在 24,旧行为跑到 40);随后 send 一条
→ gen25 消费 steer 转向作答。

背景记档:本轮 web 驾驶舱另有一批黑盒缺陷(markdown 渲染、per-session
草稿、图片缩略图、stranded 可见性/恢复、会话列表可辨识),初版基于
过时 UI(7dd7d4c)做,而 origin/main 已对 arweb 做整体 overhaul
(见 web/UI-GAPS.md)。按用户裁决:核心 interrupt 修先落 origin/main
(此处),web 侧改为针对 overhaul 后的新 webui 重新黑盒 QA、只修真正
仍坏的项(另一轮)。

## 2026-07-09 INC-3 grep / glob 独立工具(G18a 关闭)

**动机**:Codex 功能对照审计(docs/CODEX-PARITY.md)——grep/glob 是前沿
coding agent 的日用检索底座。DESIGN §5 早已列 `glob/grep` 为内置套件、
标注"尚未一等化",纯实现缺口;借 bash 有三害(命中凭据文件无红线、
输出无 per-tool 截断、network=none 收容下被拦)。工作纸
archive/increments/INC-3。

**落地**:
- `internal/tool/defs/{grep,glob}.json`(class=read)+ `exec.go` 的
  `grep()`/`glob()` 执行器 + dispatch 两 case。grep = RE2 正则逐行扫,
  返回 {path,line,text};glob = `**` 深度匹配转 RE2,返回排序后 workspace
  相对路径。
- **凭据红线共享**:`internal/index` 导出 `SkipDir`/`SkipFile`,index +
  grep/glob 共用一份排除谓词(取代 index.go 注释里"手工保持 lockstep");
  snapshot 的 gitignore-pattern 机制是另一套,不动。grep 命中行过
  `redact.FromEnv()`,读文件遇 NUL 判二进制跳过,per-file 扫描封顶 1MiB,
  匹配数 cap 200 / glob cap 1000。
- 默认脚手架 `ar init` 的 tools 增 grep/glob/semantic_search。

**闸门**:A 孪生 grepglob_test.go(命中/凭据排除/vendored 排除/截断/
二进制跳过/路径范围/`**` 语义/literal bracket + 注册)全绿;更新
tool_test.go Names() 与 spec_errors/unknown_tool.golden 两处 known-tools
清单。B 真实 API QA-11(qa/run-qa11.sh,真 Gemini):模型自发调用
grep+glob、结果落盘、`.env` secret 零泄漏——PASS。

**决策/偏差**:
- glob 无"非法模式"路径(所有 regex 元字符转义),故无 bad-pattern 测试;
  移除初版误设的 TestGlobBadPattern,改测 literal bracket 匹配。
- glob pattern 相对**搜索根**匹配、输出**workspace 相对**(可直接喂
  read_file);path 参数缩小 walk 范围。
- read_file 有意**无**凭据排除(显式命名路径允许);排除只作用于
  bulk 扫描器(index/snapshot/grep/glob),与既有教义一致。

**环境记档**:本机默认 `$TMPDIR`(/var/folders/...)使 daemon 单测的
unix socket 路径越 macOS 104B 上限而 `bind: invalid argument`——与本
增量无关(daemon 单测不 import tool/index);`TMPDIR=/tmp/t ./scripts/check.sh`
全绿。QA 脚本自带短 base dir(/tmp/qa11.XXXXXX)规避。

**review 裁决**:小增量,inline 自审(正则/边界/排除/`**` 语义/截断);
裁掉三视角对抗 review。

## 2026-07-09 INC-4 远程 stop 命令(G12 关闭)

**动机**:Codex 对照审计——远程/云任务的 stop 是标配;我们
attach/审批/用量都有,线协议独缺 stop(interrupt 只"打断当前 turn",
待命处 no-op)。UJ-17 步骤4"点 stop→优雅取消"。

**落地**:
- daemon `stop` 命令(handleStop,复用 hostedRun.stopHosting() 的
  plain-teardown 原语——ctx cancel,**无标记、无终态**,session 落
  durable 待命、send 复活,镜像终端 SIGTERM)+ dispatch case + 更新
  unknown-command help。
- `ar stop <sid>` CLI(stopCmd,mirror interruptCmd)+ cli.go 分派 + help。
- **顺带修**:handleDrive 此前在裸 daemon ctx 上跑 s.Drive、无 per-run
  cancel,drive 系列不可 stop;加 `runCtx,runCancel:=WithCancel(ctx);
  hub.stop=runCancel`(mirror handleRun),drive 亦可 stop。

**决策**:stop = **teardown-no-mark**(推荐/最小)——与 close/kill 的
"留标记、自动恢复不越过"分立,与 interrupt 的"turn 级、待命 no-op"分立。
三条控制路径语义正交,DESIGN §交互协议记档。

**闸门**:A 孪生 stop_test.go(TestStopTearsDownHostedRun 拆 run+无
SessionClosed+二次 stop 报 no live run;TestStopUnknownSession;
TestStopThenSendRevives;TestStopTearsDownDriveSeries drive per-run
cancel)全绿。B 真 daemon 手验:长跑 bash 会话 `ar stop`→"stopping"、
journal 零 session_closed、`ar send` 复活并跑新 turn(gen 到 2)——PASS。

**review 裁决**:小增量(S),inline 自审(teardown 语义/drive cancel/
错误路径)。裁三视角 review。

## 2026-07-09 INC-8 自定义命令 / slash 面(G21 关闭)

**动机**:Codex 对照——slash / prompt 宏是团队姿势沉淀。UJ-19「/deploy-check
一键跑检查单」。G21 此前设计欠定(定义位/展开语义/与 skills 边界未定)。

**落地**:
- `internal/command`(mirror internal/skill):`Discover` 列
  `<root>/.claude/commands/*.md`(basename 命名,限 [A-Za-z0-9_-]);
  `Expand(root,text)→(str,ok)` 展开首 token 为 `/name` 的消息为命令体,
  `$ARGUMENTS` 替换/无占位则追加,未知与非 slash 原样透传,strip frontmatter。
- 两处唯一 ingest 入口接展开:`Loop.Run` 开场 task(展开后 re-redact,
  因命令体是 repo 内容)+ `conversation.journalInput` 每条 send(CLI/web/
  机器都经此)。**在 ingest 展开**是关键:journal 记展开后正文→fold 永不
  读 FS(决策 #3)、resume 自包含。

**决策**:
- 命令**对模型不可见**(无工具、无 prefix 注入)——纯用户侧宏,故**不涉
  prefix 稳定性不变量**(与 memory/skills 的模型侧注入不同)。
- 信任:.md 体是不可信 repo 内容(决策 #19),但只在用户显式 /invoke 时
  展开且只注入文本→与 memory/skills 同类,无需额外信任门。
- name 限 [A-Za-z0-9_-]+ 杜绝路径穿越。

**闸门**:A 孪生 command_test.go(参数替换/追加/未知透传/穿越拒绝/
frontmatter strip/前导空白/Discover)全绿。B 真实 API:workspace 放
`.claude/commands/locate.md`,`/locate authMiddleware` 经 new 与 send 两路
都展开进 journal 的 input_received——PASS。

**review 裁决**:小增量(S),inline 自审。裁三视角 review。

## 2026-07-09 INC-6 手动 compact / clear(G7 关闭)

**动机**:Codex 对照——手动 compact(带指示)/clear 是标配上下文控制。
UJ-09「嫌摘要丢了关键约束→手动 /compact 保留 API 决定」。DESIGN §18.2
早把"未来 pause/compact"列为预期 control 输入。

**落地**(触核心 select,中增量):
- `protocol.Control{Kind,Directive}` + `Loop.Controls` 通道(nil=不接);
  处理点唯一 = 安全边界 `drainControls`(exec 在此可用);待命处
  awaitInput 加 case 存 `ds.pendingControls`+resolve 唤醒→回安全边界处理
  →decide()doIdle 继续待命(compact/clear 不起 turn)。
- compact 复用 compactContext(参数化 directive/manual;manual 用独立
  activity-id 名字空间防撞自动)。clear = ContextCompacted{Summary:""}
  (assembly 见空 summary 跳摘要头)+ 事件 `Cleared` 标记(additive-optional,
  不 bump schema);退化保护:仅当有新内容越 boundary 才落。
- daemon `compact`/`clear` 命令(Command.Directive + hub.controls +
  handleControl,best-effort ack)+ `ar compact [指示]`/`ar clear` CLI;
  Controls 经 RunRequest/ResumeRequest 两路 wire 进 Loop。

**真验捕获的 bug(关键)**:idle 处 compact 时会话以 assistant 消息收尾,
Gemini 对"接自己的话"返回**空 summary**——而空 summary 会清空上下文
(assembly 只取 msgs[Boundary:])。这是 scripted 孪生抓不到的(scripted
恒返回非空)。**修法**:① summarizer 请求补一条 user 收尾消息(使请求
well-formed,不论在何处触发);② 空 summary 一律**不落** compaction、
护住上下文(auto compaction 同受益)。真验复核:compact 后 summary 含
两个暗号原文、模型正确复述——continuity 穿过 compaction 成立。

**闸门**:A 孪生 control_test.go(TestManualCompactControl 跑 summarizer
不起 turn;TestManualClearControl clear 空摘要+二次 no-op;
TestManualCompactEmptySummarySkipped 空 summary 不落)全绿。B 真实 API
QA-12(qa/run-qa12.sh):compact 落非空 summary、clear 落 cleared——PASS。

**review 裁决**:中增量触核心 select;inline 自审(双路 control 汇流/
manual 活动 id/clear 空摘要 assembly/nil channel/空 summary 护栏)。裁
三视角对抗 review(机制被孪生+真验双闸门覆盖,且真验已抓出并修掉主要
风险点)。

## 2026-07-09 设计稿:5 个 design-first 缺口(INC-D1..D5)

本轮 Codex 对照冲刺,凡触不变量或 outward-facing/design-undefined 且非
trivial 的缺口,一律**只出设计稿、不 slam code**(PROCESS §3.5/§4 纪律):
- INC-D1 会话内 goal(G23/UJ-22):原始丢失需求。**改决策 #21/§13 不变量**
  ——fresh-run 教义 scope 到 best-of-N+批式 loop;in-session goal 挂
  conversational session,verifier 是静止序列在 exchange 边界的新一格,
  miss 回灌 program 源 input 进同 fold。走 PROCESS §4。
- INC-D2 事件唤醒(G14/UJ-12):inbox 原语已就绪,缺投递壳;须先成文机器
  发送方信任/鉴权 + 来源前缀 + 幂等 id + 审批期投递语义。invariant-adjacent。
- INC-D3 web_fetch(G18b):进程内 net/http 出口不被 netns 覆盖→须把收容
  棘轮不变量从"bash fail-closed"升级为"egress 类统一 fail-closed",MVP 只
  做 web_fetch + host allowlist + 不可信标记。走 PROCESS §4 + G16 条款。
- INC-D4 记忆写回(G9):取 A(append-as-message,下次 session 生效)不触
  prefix 冻结不变量;取 B(MemoryChanged re-freeze)触之。待裁 A/B。
- INC-D5 审批持久化(G5):取 A(写 project 配置,下次生效)不触规则冻结;
  取 B(PolicyChanged 本 run 生效)触之。待裁 A/B + 写回层。

**webui**:用户已 spin off 独立 session 修 diff 白屏 blocker,本会话不动
webui/ 避免冲突;余 UI 项(markdown/usage/选择器/搜索/归档)随该线或后续。

---

## 2026-07-09 ⚠️ 冲突待裁:INC-D3(web_fetch design-first)× INC-5(web_fetch 已实现)

两条并行线对 web_fetch 给出**相反的程序裁决**,rebase 时正面撞上,记档
待开发者裁:

- **INC-D3(上条设计稿线)**:web_fetch 的进程内 net/http 出口不被 netns
  覆盖 → 判定为**收容棘轮不变量的升级**("bash fail-closed"→"egress 类
  统一 fail-closed"),按 PROCESS §3.5/§4「只出设计稿、不 slam code、走
  不变量变更流程」。
- **INC-5(下条实现线,本会话)**:已实现 web_fetch,并在 DESIGN §5 把
  network 资源类条款扩展为"带网 in-process read 工具收容下 fail-closed"。
  本线的论证是**覆盖面扩展、不反转旧语义**(无旧保证被削弱、边界诚实更
  完整),故按覆盖扩展落地、未单独走 §4。代码已 push origin/main、单测 +
  真实 API QA-13 PASS。

**争点**=同一改动是"不变量升级(须走 §4 停下单独 review)"还是"覆盖面
扩展(随实现同 commit 修订措辞)"。两线的**技术方向一致**(都主张 egress
统一 fail-closed),分歧纯在**程序**:是否必须先成文设计稿再落码。

**现状**:实现已在 main。若裁定 INC-D3 的程序要求优先,则需补一份
web_fetch 的不变量变更设计稿、对 DESIGN §5 的改法单独 review(实现可留、
补齐程序),而非回退代码。**在开发者裁决前,不再扩大 web_fetch 相关改动。**
(ask_user 无此争议——DESIGN §5 早已定义 wait-class 语义,属纯实现缺口。)

---

## 2026-07-09 INC-5:核心工具面补全——web_fetch + ask_user

**背景**:用户看过 Claude Code 的工具清单后问「你自己执行的工具能不能都
实现、做成可复用的」。研究确认 grep/glob 已由并行增量 INC-3 落地,本增量
承接剩下两个「自己执行、装得进数据模型」的缺口:web_fetch(G18 web 部分)
与 ask_user(G20)。工作纸 `increments/INC-5-web-fetch-ask-user.md`。

**编号撞车说明**:本仓库多 worktree 并行开发,INC-4/INC-5/INC-6 号被并行
session 分别占用(remote-stop / custom-commands / manual-compact)。本线
代码提交前缀已用 `INC-5.1/.2`(web_fetch/ask_user 同一增量两步),为内部
一致保留;工作纸让号至 `INC-5-web-fetch-ask-user`(与归档的
`INC-5-custom-commands` 语义独立)。QA 号让至 **QA-13**(QA-12 被 INC-6
手动 compact 占)。单人原型的并行噪音,记档不追改历史。

**web_fetch(客户端 read-class + `network` 数据位)**:`tool.Def` 新增
`network` 出口标签(数据位,强化决策 #13「tool 定义即数据」)。
`loop.networkScope` 从「只认 execute class」改为数据驱动:带 `network`
的工具未收容时恒带 `all`(permission network 规则可匹配),收容棘轮下
executor **fail closed**(in-process fetch 无法 netns 包裹,拒跑而非静默
出网——与 bash fail-closed、MCP「恒记 all 边界诚实」同族)。实现:
http/https 逐跳校验、重定向上限 5、读入 512KB/输出 50KB 截断、HTML→text
(剥 script/style/注释)、二进制/非文本拒绝、redaction、`untrusted_content`
标记(注入面 G16 第一道软防线)。

**ask_user(wait-class,落实 DESIGN §5 早定语义)**:提问=待命,park 到
`WAITING_INPUT` 携问题(`WaitingEntered{input, detail:{call_id,question}}`,
靠 Detail 与普通 standby idle 区分),不占 activity → 免 in-doubt 误杀。
补上 G20 缺的**应答路径 = inbox 本身**:新增 `AskResolved` 事件(携应答
文本,与 `ApprovalResponded` 同族——带内容的专用应答事件,不经
`InputReceived`),fold 配对为该 call 的 tool result `{answer:…}` 并授
turn budget → session 不 idle、续下一 gen step。一批限一问(第二个
`AskResolved{rejected}` 模型可见报错);interrupt→`{interrupted}`;
crash→park 持久、resume 补 `WaitingResolved` 自愈;headless→run 返回、
park 留 journal、`ar send` resume 应答。零新 CLI/daemon 动词(send 即应答)。

**设计取舍**:配对为什么走单一 `AskResolved` 而非改 `InputReceived` fold
——把「记录输入 + 配对 tool result + 授 budget」收进一个事件,配对**原子**,
crash 只可能落在 `AskResolved`(已 durable)与 `WaitingResolved` 之间,
自愈只需补后者(与既有 pending_input 自愈同构)。等待注册表两 kind
(决策 #31)不动:ask park 是「带未决问题的待命」,与 `WAITING_APPROVAL`
靠 Detail 携载荷同构。

**闸门 A**:web_fetch httptest 8 场景(文本/HTML 提取/重定向/重定向环/
非 http(s) 拒绝/超大截断/二进制拒绝/4xx 带 body)+ networkScope 数据驱动
+ 规则两种写法(tool 名 / network scope)拦截 + 收容 fail-closed;ask_user
`TestAskUser*` 六态(park→answer 续跑 / 第二个 rejected / park 中 interrupt /
headless 返回+resume 应答 / crash-resume 重 park / settle 不配对);event
round-trip sample、Names/golden、fold coverage 全绿。
**闸门 B**:真实 Gemini **QA-13**(qa/run-qa13.sh):模型自发 web_fetch 抓
本地页→HTML 正文回灌(script 噪音剥离)、ask_user park→`ar send` inbox
应答→按答案 write_file 落盘、零 crash——PASS,归档
`qa/runs/2026-07-09-QA-13/`。

**三层并回**:SPEC C(web_fetch ✅、web search 拆出单列、ask_user ✅、
finish 拆行)、D(network 规则覆盖带网 read 工具);DESIGN §5(network
资源类扩展、`network` 数据位、内置套件行)、§17(ask_user 一等化注记);
GAPS G18b(web_fetch 关闭、search 留开放)、G20 关闭、UJ-06 判定升 ✅;
JOURNEYS UJ-01 web_fetch 可选步。

**review 裁决**:中增量,ask_user 触 loop 等待/fold(并发+恢复敏感),
收口做一轮正确性/并发聚焦对抗 review(基准 = DESIGN §2/§5/§6 + 工作纸
语义表),见下条。

## 2026-07-09 让号(校正):自定义命令 INC-5→INC-8;QA-12 保留

并行 web_fetch/ask_user 线的 LOG 已记:其 QA 号**让至 QA-13**(明言"QA-12
被 INC-6 手动 compact 占"),即已尊重本会话的 QA-12(compact/clear)。故:
- **QA-12 不动**(compact/clear 本会话所有,对方已让 QA-13)。
- 自定义命令曾记 **INC-5**,与 web_fetch/ask_user 线的 INC-5.x 在 SPEC 上
  重号 → **让至 INC-8**(SPEC/GAPS/CODEX-PARITY/LOG 引用 + 归档工作纸
  INC-5-custom-commands.md→INC-8-custom-commands.md),消 SPEC 台账二义。
- INC-6(compact/clear)、INC-3(grep-glob)、INC-4(remote-stop)不撞、不动。
对方 LOG 中 `INC-5-custom-commands` 的历史指代按 append-only 纪律不追改。

## 2026-07-09 INC-5 收口对抗 review:1 P0 + 2 P2,全修

对 ask_user 做正确性/并发/恢复聚焦的对抗 review(7 个失效场景逐一追到
代码)。结论:4 项成立无缺陷,3 项有缺陷、已全修。

- **P0(测试盲区,已修)**:mailbox-crash 应答误配成孤儿 user 消息 →
  parked call 永久悬空 / headless 死锁。根因:daemon 对 send **先
  durable ack 再投 channel**(`AppendInbox` fsync 早于 `AskResolved`
  落盘),crash 落在这一窗口时,Resume 的 mailbox 重放(`loop.go`
  ~524)无差别 `journalInput`,把应答 fold 成独立 user 消息、call 不
  配对;headless 每次 resume 重复孤儿化 → 死锁。**全部 `TestAskUser*`
  都走 `UserInputs` channel,从不走 mailbox 重放,故盲区**。修:Resume
  的 mailbox 重放感知 ask-park——`Waiting` 为 ask-park 时,第一条未消费
  输入经 `journalAskResolved{answered}` + `WaitingResolved` 配对(镜像
  `awaitAnswer` 的 channel 分支),其余 type-ahead 才 `journalInput`。
  补 `TestAskUserMailboxReplyPairsAcrossCrash`(经 `store.AppendInbox`
  写应答、不经 channel,crash+Resume,断言配对而非孤儿)——禁用修复即
  红(`ok=false` 孤儿症状),修复后绿。
- **P2(已修)**:headless `awaitAnswer` 早退不等在飞 background handle
  (与 `idleOrReturn` 不一致,可丢 settlement)。修:早退加
  `len(Handles)==0 && len(Timers)==0` 前置,有在飞则落 select 等
  `bg.done`。
- **P2(已修)**:ask-park 崩溃自愈把 `WaitingResolved.Resolution` 硬编码
  `"answered"`,interrupt/reject 窗口下 audit 失真。修:从配对结果的
  `IsError` 反推(error → `"recovered"`)。
- **成立无缺陷(4)**:一批多 call(第一个 park、第二个 reject、指针取
  `&allowed[i]` 稳定,无双配/漏配)、AskResolved-已落自愈幂等、budget/
  decide 续跑正确、standby vs ask-park 判别(CallID 非空)稳健、interrupt
  两族路径(wait 上下文 vs activity 上下文)不相交。

**闸门复验**:`TestAskUser*` 七态(新增 mailbox-crash)+ check.sh 全绿。
P0/P1 清零,增量关闭。web_fetch 侧的程序争议(是否触收容棘轮不变量)
另见上方「⚠️ 冲突待裁」条,待开发者裁决,不含在本 review 范围。

## 2026-07-09 web_fetch 补程序:不变量变更(决策 #33)+ 安全对齐(M1/M2)

**开发者裁决**(前条「⚠️ 冲突待裁」的裁定):web_fetch 走「**实现留、补
PROCESS §4 程序**」——实现有效,补不变量变更设计稿 + 安全 review,而非
回退。本条即程序补齐。

**§4 不变量变更(决策 #33)**:收容棘轮从"bash fail-closed"升级为"**所有
egress 类 tool 统一 fail-closed under containment**"。旧不变量(§18.5)只
保 bash;冲突根因:web_fetch 用 `net/http` 在宿主 Go 进程内跑,出口**不被
`unshare -n` 覆盖**(netns 只包 bash 子进程),只保 bash fail-closed 会让它
在 `network=none` 下静默违反"收容=全树无出口"。新表述纳入 in-process 带网
工具(`def.network` 数据位),收容下执行期自我拒跑、containment 记账缺席
(自我拒跑非 netns)。波及:DESIGN §5/§15(#33)/§18.5、`tool.Def.network`、
`networkScope`、`containment` 守卫、`webfetch.go`。并入 INC-D3 设计稿(归档)。

## 2026-07-09 INC-9 PDF / 任意文件附件(G1 余项"PDF/附件泛化"关闭)

**决策**:file part 已是不变量枚举部件类型(DESIGN §18「消息 parts」),
part 模型/CAS/event(`InputReceived.Files`)/fold/inflate/**Gemini** provider
(inline_data 按 MIME 泛型)**均已泛化**——本增量**不新增部件类型、不触任何
不变量**(→ 不走 PROCESS §4)。改动面:`protocol.FileAttachment`+`UserInput.Files`、
`daemon.Command.Files`、`journalInput` 摄入 `in.Files`→CAS→`event.Files`、
`ar send --file`(sniff MIME,不拒非图片)、**Anthropic** provider 加
`application/pdf`→document block 分支(原先非 text 的 file part 一律发
image block,PDF 会误投)。驾驶舱:`/api/sessions/{sid}/send` 加 `files`→
`--file`;Composer `+`→File 走任意文件(≤10MB);`ar new` 开场不带附件的
非对称保留,驾驶舱以"建会话→立即跟一条带附件 send"补首条体验。

**偏差/记档**:附件字节沿用既有 CAS 通路、**不过 redaction**——这是
既有属性(长贴折叠、--image 同样),不在本增量改;若要附件级 redaction
另立增量。`ar new` 开场附件(§9.1 非对称)保留待反馈。>20MB / File-API
路径未做(inline base64,受驾驶舱 10MB 上传上限约束)。

**闸门**:A — 新增 `TestConversationalFileInputEndToEnd`(ref-not-bytes +
application/pdf)/`TestToPartFilePDF`(Gemini inline_data)/`TestUserBlocksFilePDF`
(Anthropic document block 非 image);check.sh 全绿(短 TMPDIR 规避 macOS
104B unix-socket 路径限)。B — **QA-15 真实 Gemini**:传含秘密词的 PDF、
问"只回秘密词",journal file part=sha256 ref+application/pdf、回复精确
=`ZEBRA-42-QUOKKA`(真读 PDF)。隔离实例跑(新二进制 daemon,不重启打扰
并发 session)。SPEC/DESIGN(§9.1)/GAPS(G1 余项)/QA-15、工作纸归档同步。
**review**:小增量、不触不变量、opt-in,裁掉三视角对抗 review(理由见
工作纸 `archive/increments/INC-9-*`)。

**安全视角对抗 review**(§4 要求的 review;审实现 vs 设计稿差异):
- **收容 fail-closed 无绕过**(6 点证据):check 在出网前、棘轮单调共享、
  独立于 permission gate(bypass 绕不过)、无旁路 dispatch、无 TOCTOU。✓
- **M1(P1,已修)**:web_fetch 原 read-class → default 模式**静默放行**,
  每次调用无审批出网(注入后可 exfil workspace/凭据),plan 模式也出网、
  in-doubt 会重跑。修:class **read→execute**(default 需审批;plan 拦住;
  in-doubt 不重跑)+ 同步 `containment()` 守卫(`def.network` 非空 → 记账
  缺席,否则 execute-class 会误记 netns——正是"class 翻转牵动记账诚实、须
  走 §4 而非 code-slam"的实证)。
- **M2(P1,云/CI 下 P0,已修)**:无 IP 过滤 → `web_fetch(169.254.169.254)`
  可窃云 IAM 凭据(redact 认不出非 env 密钥)。修:`http.Client` 装
  `Dialer.Control` egress 守卫拒连 link-local(`169.254.0.0/16`、`fe80::/10`),
  作用于**已解析 IP**、覆盖初始请求与**重定向每跳**——一处同时堵
  SSRF-via-redirect、DNS rebinding、十进制/IPv6 IP 混淆;零误报。
- **S1(建议,开发者待裁 = INC-D3 待裁点)裁定**:单机 dev 威胁模型下,M1
  的 execute 审批 + 审批面 URL 可见(随 `KindToolCall` 出到事件流)是可
  辩护的弱替代;完整 spec 级 host allowlist 记 **backlog**,与 pipeline
  `PermissionRule.Host` 字段(B2)、私网整体开关(B1,挂 G11 云形态)一并
  留待需求。`untrusted_content` 软标记保留但不计入 exfil 缓解。

**闸门**:A — `TestWebFetch*` + `TestWebFetchRefusesLinkLocalMetadata`/
`TestRefuseLinkLocalPredicate` + execute-class 后的 `TestNetworkRulesGateWebFetch`
(default ASK、allow 规则放行)/`TestWebFetchNetworkScope`(containment 守卫)
全绿;check.sh 全绿。B — **QA-14 真实 coding agent** execute-class 下三跑均
PASS(allow-all spec 命中放行,agent 照常抓规范→实现→测试绿),正常流程未
退化。DESIGN §5/§15/§18.5、SPEC C、GAPS、INC-D3(归档)同步。

## 2026-07-09 INC-D1 会话内 goal——不变量变更(决策 #21 拆分)+ G23/UJ-22 关闭

**§4 不变量变更(决策 #21 修订)**:旧「one-shot/goal/loop/best-of-N 是同一
IterationDriver 四种 schedule;每轮迭代=fresh child session」拆为:best-of-N/
批式 loop/one-shot/**driver-goal** 保 fresh-child-run;**goal 另有会话内形态**
(in-session goal)挂 conversational `agent.Loop`、context 全程延续。根因:UJ-22
硬要求 context 延续,fresh-run 构造上丢对话 context(开发者 2026-07-05 已裁定
fresh-run 教义不适用于 goal 形态)。DESIGN 决策 #21/§13/glossary 与实现同 commit。

**机制**:event goal 族(7 个)+ state.Goal 子状态 fold + `goal_verify` 作为
静止序列(决策 #24)**最后一格**(barrier 仍快照 pre-injection 干净边界)。
miss → `GoalCheckpoint` + program 源 `InputReceived` 回灌(state.go:332 天然
fold 进对话)→ idleOrReturn **wake seam**(`hasInputAfterLastAssistant` → 不
idle、返回 done=false 让 drive 重 decide → 同上下文续 turn)。pass →
`GoalAchieved{satisfied}` 摘 goal;`max_checks` 尽 → `GoalAchieved{budget}` =
可见截断(决策 #31)。控制面 attach/pause/resume/update/cancel 走 compact/clear
同 out-of-band control 通道(`ar goal`)。

**crash 安全(R1/R2)**:`GoalCheckpoint` 带 `GenStep`+`Feedback`;goal_verify
若本 gen step 已 checkpoint 则恢复(LastFeedback 缺则补灌),不重跑 verifier、
不双注入。verifier 命令须幂等(与 driver verifier 同契约)。程序输入直接
appendE(不过 mailbox,DeliverySeq=0),幂等键=CheckpointedGenStep(R3)。

**R6 resume 兼容**:加 "goal":1 sub-state 版本;`checkVersions` 从精确集合相等
放宽为 **superset-tolerant**(journal ⊆ binary,共享 namespace 版本须匹配;新增
namespace 从零 fold)——否则所有旧会话拒绝 resume。这是加 namespace 的可证加性
放宽。

**偏差/v0 余项**:llm_judge/human verifier、token/墙钟 goal 预算列余项(命令
verifier + max_checks 已覆盖 UJ-22 主场景);goal attach 需 live session(控制面
不复活 idle 会话,同 compact/clear);steer 与 goal 并行随既有插话排队天然成立。

**闸门**:A — `TestInSessionGoalContinuity`(单 SessionStarted 证 context 延续 +
miss→回灌→pass)/`BudgetTruncation`/`PauseCancel`;check.sh 全绿(核心 loop/
quiescence/mailbox/resume 无回归)。B — **QA-16 真实 Gemini**:挂 goal→真 agent
建 done.txt=FINISHED→真命令 verifier 通过→achieved,sessions=1。驾驶舱:`ar goal`
端点 + session `/goal` 挂 in-session goal(Home `/goal` 仍走 driver-goal)+ goal
banner(pause/cancel)+ inspect goal 摘要。DESIGN(#21/§13/§24/glossary)/SPEC F/
GAPS G23/JOURNEYS UJ-22/QA-16、工作纸归档同步。

**三视角对抗 review（里程碑+不变量变更,强制）**——三个独立 agent 各审
correctness/并发、安全、契约=DESIGN+QA,发现并全修:
- **正确性 Bug 1(CONFIRMED,关键)**:crash 恢复守卫在 resume 上是死代码——
  resume 时 shape 已静止,`quiesced` 起始 true,`idleOrReturn` 跳过
  `quiescentActions`(含 goal_verify),恢复分支永不执行 → checkpoint→
  follow-up 崩溃窗让 goal 自主推进停摆。**修**:新增 `goalRecover` 在 drive
  循环安全点每轮跑(不受 quiesced 门控),重发丢失的 GoalAchieved(pass/
  budget)/重灌丢失的 miss feedback,幂等。孪生 TestGoalRecover(三分支+
  不双注入)。
- **正确性 Bug 2(CONFIRMED)**:`ar goal update` 恒发 Budget(默认),只改
  verifier 的 update 会静默重置预算甚至立即截断。**修**:update 仅在显式
  `--max-checks` 时发 Budget;attach 才用默认 10。
- **正确性 Bug 3(PLAUSIBLE)**:MaxChecks==0 预算永不生效,driver 直连绕过
  默认可无界循环。**修**:`goalMaxChecks` 兜底 DefaultGoalMaxChecks=20。
- **安全 F3(CONFIRMED)**:goal 文本/feedback/detail 未过 redaction(凭据红线
  §18.5 不一致)。**修**:GoalAttached/Updated 的 goal 文本 + 回灌 program
  输入 + checkpoint detail 全过 `redact.FromEnv()`(verifier 命令须运行故保
  raw,同 bash 工具调用)。
- **安全 F2(PLAUSIBLE,largely 既有)**:webui `readBody` 不检 Content-Type →
  CORS simple-request CSRF(drive-by-localhost 触发 ungated bash)。**修**:
  readBody 要求 `application/json`,强制 preflight(no-CORS server 不应答→
  浏览器拦),硬化 goal + 既有 send/git 等全部 JSON 端点。
- **安全 F1 / 契约**:in-session verifier ungated(driver 其实 adjudicate)——
  修正误导注释,记档为 defensible(命令仅 operator 可设、网络仍收容)+
  pipeline 化 verifier 列 hardening 余项。契约 review 确认核心契约(verifier
  仅静止边界、generation 不被挟持、miss 回灌同 fold、achieved/cancel 非终态、
  goal 参数出冻结 spec、决策 #24/#31/#32 honored)全部成立。
- 三个 doc straggler(#24 加格、CODEX-PARITY goal 行、§13 opener)同修。
P0/P1 全修,收口。

## 2026-07-09 Codex goal 深潜审计:CODEX-PARITY 新增 §6(G23 余项开挖)

用户指出 webui goal UI 与 Codex 差异大,要求审计原因与缺口。三路实证
(~/.codex/goals_1.sqlite 六态 schema、两条真实 goal thread rollout JSONL
含完整 continuation prompt、官方 cookbook/changelog/issues)得出:差异根源
是两种哲学——Codex 对话式+模型自治(/goal 一句话、模型持 create_goal/
get_goal/update_goal 受限三件套、完成=Completion-audit prompt 下模型自证、
token+墙钟预算、六态含 blocked/usage_limited),我们验证式+外部裁决
(command verifier + MaxChecks 轮数)。**发现 bug 级语义洞:无 verifier 的
goal 恒不可达成**(goalVerify ran==0 恒 false,而 CLI/webui 都允许空
verifier 落地 → 烧完预算截断)。缺口清单+建议(模型自证与 verifier AND
的混合形态、continuation prompt 升级、blocked↔ask_user 打通等)见
CODEX-PARITY §6.2;连带发现 update_plan/终端交互/node REPL 等 Codex 模型
侧工具面差距登记 §6.4。§2-06 goal 行 ✅→🟡,§3 会话内 goal 行改
✅ v0+余项。纯审计与文档增量,未动代码。

## 2026-07-09 INC-11.1 runtime 基线与真实旧 store projection 修复

**真实复现**：`./scripts/check.sh` 卡在
`TestMalformedToolCallExhaustionErrors`；目标测试 15s timeout 的栈在
`idleOrReturn→Quiescence`。根因是 malformed finish 不落 assistant message，
原始 user message 永远满足 raw `hasInputAfterLastAssistant`，绕过
`TruncationRestartable` 的“一次 wake 一次尝试”契约而热循环。修为
`hasRunnableInput` 统一服从 truncation policy，并补 resume 回归测试。

同时修复三类基线漂移：quiescent 固定序列测试补上 INC-D1 已加入的
`goal_verify` 最后一格；所有直接绑定 Unix socket 的 daemon/CLI 测试使用
短临时路径，避免 macOS 104-byte `sun_path` 上限；timer sweep 的旧失败由
同一 socket bind 问题消除。`check.sh` 全绿。

**真实共享数据**：`~/.local/share/agentrunner` 中
`20260709-104551-sched-loop-5c56` 等 driver journal 之前被 CLI 错送
`state.Fold`，报 `registered event type has no fold case: driver_started`。
现在按首事件 stream header 分派 `driver.Fold`；`sessions`、`inspect`、
`events --state` 均能读取旧 journal，inspect 递归展示 `sub/iter-N` 子会话。
实测原 `unreadable` 行恢复为 `satisfied` / `max_iterations`。这是 projection
修复，不修改任何旧数据。
