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

## 2026-07-09 INC-11.2 durable CommandLog 与幂等投递

把仅覆盖 user input 的 mailbox 扩成 typed per-session CommandLog，兼容沿用
`inbox.jsonl` 与旧行格式。send/control/close/interrupt/approval/kill 都先
redact+fsync，ack 只表示 durable accepted；调用方 mint 稳定 `command_id`，
同 payload 重试返回原 seq、冲突复用拒绝。event envelope 新增独立
`command_id` receipt，不污染线性 `causation_id`。

daemon 用单 FIFO 搬运所有已 accepted command，宿主内按 id 去重；append
后的 wake 失败不再反悔为客户端错误。启动扫描 CommandLog 与 journal
completion fact 的差集，自动 re-host/replay control、interrupt、approval 等
非输入命令。inbox append 索引改为启动时线性重建，消除逐次全表扫描的
O(n²)。agent 在应用输入/控制/中断/kill/审批时把 receipt 写入 semantic
event，无效果的重复 control 落 `CommandHandled`。

孪生覆盖跨 restart idempotency/冲突、legacy mixed read、200 条无界 FIFO、
宿主去重、startup pending replay、durable close/interrupt/approval receipt；
修复并发子 agent 共用 `capturingProvider` 测试桩的 slice race；相关四包
`go test -race` 与全量 `check.sh` 通过。

**真实共享 store + daemon restart**：在
`~/.local/share/agentrunner/sessions/20260709-212328-inc11-real-start-fdc4`
以真实 `daemon.sock` 写入 input/clear，确认 inbox 与 semantic event 各只有
一个同 id receipt，`command_id` 与 `causation_id` 分立。三次 SIGTERM 优雅
滚动到新 `/tmp/ar` 后重复原 wire command，journal 行数保持 23、无重复 wake。
该闸门先抓到两个仅跨 restart 出现的问题并修复：已完成旧 receipt 在新宿主
多唤醒一次；nested `Control.CommandRef` 未从 payload hash 规范化而误报冲突。
旧 driver `20260709-104551-sched-loop-5c56` 在同一重启后仍可完整 inspect。

## 2026-07-09 web/(arweb 原型驾驶舱)退役删除——webui 补齐六项缺口后收编

**决策**：删除 `web/` 整目录（用户拍板）。arweb 是 M0–M8 期的单文件
原型驾驶舱，`webui/`(React 版)功能面已成超集；本日补齐最后六项缺口并
真验后，双驾驶舱并存只剩维护成本。开发史（web/PROGRESS.md 台账、
web/DESIGN.md 铁律、web/UI-GAPS.md 盘点）在 git 历史中可考
（最后版本见本提交的父提交）。

**本日补齐的六项缺口（webui, 均真 Gemini 真验）**：手动 barrier 打点
（POST /sessions/{sid}/barrier + Checkpoint now 菜单）、图片缩略图
（GET /api/uploads/{name} + composer chip/已发气泡预览）、drive
best-of-N（schedule: parallel,/bestof + launcher + RunModal 预设）、
composer agent persona 模板（dev/auditor/reviewer/chat,经 ar agent
切换）、per-session 草稿恢复、RunView drive 富渲染（iteration 分隔 +
driver verdict 横幅）;另收编上一 session 遗留的批量审批 ⌘↵/⌘⌫。

**随 web/ 删除而指针失效、但仍有效的两个产品提案**（原载
web/PROGRESS.md 提案区,此处保留要点,需要时按 PROCESS 增量流程开挖）：
- **P1② 子事件进 attach 流**：`childLoop` 接 Out sink（tee 到父的 hub
  sink,`protocol.Event.Session` 填 childSession）,attach 一个父即得
  全树打字级实时流;轮询已覆盖功能面,纯流式装饰。
- **P2 父/用户→在飞子 agent 第二条消息**：子 run 是一次性 task,无
  inbox;增量 = 给 spawn 子挂 UserInputs（复用 conversational mailbox
  语义）+ 投递面（`ar send <child-id>`;可选父模型侧 send_to_agent
  工具）。动运行时核心语义,需完整增量流程。

LOG.md 历史行中对 web/PROGRESS.md、web/UI-GAPS.md 的引用（2026-07-07/08
两条）为历史记录,不回改;以 git 历史为准。

## 2026-07-09 INC-11.3 verifier 统一治理与 OS workspace sandbox

删除 in-session goal verifier 的 UNGATED 例外：每个 command check 使用确定性
effect/activity id，经现有 mode/permission/hooks/approval/budget 管线，落
`EffectRequested/Resolved` 与 `ActivityStarted/Completed`；deny 只形成失败
checkpoint、不启动命令。审批请求在无 provider call_id 时也持久化并展示
verifier tool/args/containment requirement。driver command verifier 同步记录
containment evidence，headless ask 继续收紧为 deny。
确定性 activity id 还关闭了 `ActivityCompleted → GoalCheckpoint` crash 窗：
恢复直接复用 journaled verifier result，不再次执行命令。

bash 与 command verifier 默认强制 OS filesystem=workspace：macOS 用
`sandbox-exec`/Seatbelt，Linux 用 Bubblewrap；`sandbox.network:none` 由同一
backend 收紧。sandbox 只给 workspace、隔离临时 HOME 与 linked-worktree git
metadata carve-out，显式遮蔽 workspace 内凭据形路径且不传 `_API_KEY/_TOKEN/
_SECRET` env。backend 不可用时 containment gate 在 Activity 前 fail closed；
不支持平台不裸跑。EffectResolved 记录 filesystem/network/backend 实证。

真实共享 store 对抗：`20260709-214651-exercise-sandbox-28ae` 中 sandboxed
bash 可读写 workspace，但读取 `/tmp/inc11-outside-real`、`.env` 均被 OS 拒绝，
敏感 env 为空且 journal 零泄漏；`20260709-214800-stand-by-for-goal-d657`
的会话内 verifier 产生完整 allow→Seatbelt containment→Activity→pass 链并
GoalAchieved。自动覆盖外部读写/凭据/env、linked worktree git、network deny、
capability fail-closed、goal policy deny、driver evidence；Linux 目标交叉编译通过。
## 2026-07-09 INC-10:goal 自证完成——G23 补全,CODEX-PARITY §6 缺口 ①②③⑥⑦ 关闭

**增量**(工作纸 INC-10,已归档):无 verifier 的 goal 由模型 `goal_complete`
声明完成,checkpoint 在静止边界裁决接受(GoalCompletionClaimed 事件,
change-as-event #32 同族,checkpoint fold 消费、GoalUpdated 作废);有
verifier 时 verifier 仍是唯一裁决者(claim 不越权,向后兼容)。模型工具面
goal_status/goal_complete(常驻 face extras,无生命周期/verifier 设置路径,
goalVerify 无门跑辩护前提保持);attach/miss 回灌升级为结构化 continuation
(<goal> 标签注入卫生+反缩水+完成路径+预算报告);webui /goal 一句话直启
(Home=新建会话+attach)、banner edit/self-certified/claim-pending。决策
#21 完成判据扩展走不变量变更流程(工作纸单独成文,契约 review 通过,
DESIGN/SPEC/UJ-22 与实现同 commit)。

**对抗 review(契约+正确性双 agent)**:核心主张全数核查成立;P0/P1 全修——
event 样本缺失(P0)、checkpoint 前 crash 窗漏裁(P1,新增 goalResumeCheck
安全点补裁)、resume/update 打 idle 会话停摆(P1,goalReinject 注入再武装)、
goal fold 就地 mutate 破 Apply 纯度(P1,copy-on-write)、SPEC 锚失真(P1)。
P2 采纳孤儿会话修复;double-attach last-wins/inspect max_checks 显示原始
值/Home task 双重注入记档接受。

**连带主干潜红(被 go test 缓存掩盖,本轮缓存失效暴露,当场修)**:
1. TestQuiescentSequenceOrder 期望缺 goal_verify 格(INC-D1 起潜红);
2. daemon/cli 测试 unix socket 路径超 macOS 104B(shortSock 统一;与并发
   session 的同因修复在 rebase 中合流);
3. INC-D1 wake seam × malformed 截断 drive 空转(TestMalformedToolCall
   ExhaustionErrors hang;修法与并发 session 的 hasRunnableInput 一致,
   rebase 取其实现)。

**闸门**:A 孪生 TestInSessionGoal{SelfCertify,ClaimDoesNotOverrideVerifier,
NoVerifierBudget,ResumeContinues}+TestGoalResumeCheck/TestGoalClaimFold/
TestGoalAttachRevivesSession,check.sh 全绿(rebase 合流后复验);B QA-17 真
Gemini PASS(共享 daemon/store:claimed=1(model)→checkpoint model-certified
→achieved satisfied,单 session,haiku 落盘)+ webui Chrome 真跑 PASS
(/goal 直启、banner、goal_complete 时间线、达成收敛;CLI 真验 update 作废
+resume 注入)。归档 qa/runs/2026-07-09-QA-17/。

**QA 备注**:共享 daemon 已切至 INC-10 二进制(/tmp/ar-inc10,原 /tmp/ar
优雅停止,当时无 running 会话);验证中曾误把守护 goal attach 到并发
session 的测试会话(...-ready-ad86),已即刻 cancel 并在归档记录。goal-*
控制的 revive 在 rebase 后由 INC-11.2 durable-command 统一路径结构性
提供(handleGoalControl 包装移除,TestGoalAttachRevivesSession 保留为
行为锚)。余项:goal token/墙钟预算、blocked/usage_limited 态、llm_judge
verifier、banner 用量显示(elapsed/tokens)——随 CODEX-PARITY §6.2-④⑤。

## 2026-07-09 INC-11.4：MCP 产品接线与协议面收口

`AgentSpec.mcp` 现可声明 stdio/streamable HTTP server，Loop 在前台、daemon、
resume、driver 与子 agent 等所有构造路径自动连接/关闭，不再依赖单测注入。
支持 env-only header/OAuth bearer、per-server/global allowlist、resources /
resource templates / prompts、structured content 与 image/audio/resource blocks；
SDK `list_changed` 在 loop 安全边界产生新的 `ToolsDiscovered` face。

断线只使当前调用返回 `outcome_unknown` 的模型可见错误，下一次操作重建
session；不会自动重放可能已产生副作用的调用。远端 `readOnlyHint` 仅用于
permission/UI class，MCP activity 无本地 idempotency contract 时恒为非幂等。
真实子进程 stdio 与 httptest streamable HTTP 覆盖协议面、OAuth/header、
通知及重连；`./scripts/check.sh` 全绿，`go test -race ./internal/mcp
./internal/agent` 全绿。共享 store 真实 Gemini 会话
`20260709-222150-mcp-e99b` 由 spec 自动拉起 stdio server，调用
`mcp__fixture__rich_result` 后完整收到 `structuredContent.answer=42` 与 image
block；journal 中该 MCP `ActivityStarted` 为 `idempotent:false`，配置仅含
环境变量名、无 secret 值。

---

## 2026-07-09 INC-11.5：Turn/Item、typed ingress 与 provider envelope

在不破坏既有 provider Message/GenStep 视图的前提下新增 `Interactions`
子状态：外部输入形成 Turn，user/assistant message、tool_call、tool_result
形成稳定 Item；新事件显式带 turn_id/item_id，旧日志按 seq/gen-step 确定性
补齐。旧 snapshot 若没有 interactions 版本会被当作可丢缓存，自动全量 fold，
避免只折 tail 导致历史 Item 消失。

统一 CommandLog 与 `InputReceived` 现保存 principal/source/trust 和 typed
content；binary 仍遵守 blob-before-event，journal/Turn Item 只含 CAS ref。
Unix socket 旧客户端获得明确 local 默认，CLI 主动标注 local-user/cli/local，
外部 connector 可传自己的身份、来源与信任级别。`inspect --json` 暴露
turn/item 数量与 provider capability envelope；SessionStarted 冻结 envelope
schema、provider/model、modalities、stream/tool-call 和可选能力。

自动锚：`TestTurnItemProjectionPreservesTypedIngressAndToolItems`、
`TestLegacyMessagesSynthesizeStableTurnItemsWithoutMutatingPriorState`、
`TestJournalInputPreservesTypedContentAndProvenance`、`TestCapabilitiesMatrix`。
`./scripts/check.sh` 全绿；`go test -race ./internal/state ./internal/store
./internal/daemon ./internal/agent` 全绿。共享 store 真实 Gemini 会话
`20260709-224222-task-0785` 的原始 journal 含 local-user/cli/local、typed text、
稳定 turn/item id 与 provider envelope；`inspect --json` 报告 1 turn / 5 items，
并完整显示 gemini modalities/capabilities。

---

## 2026-07-09 INC-11.7：event cursor、snapshot 真尾读与 schema 兼容

新增可弃的固定宽度 `events.idx`（每 event 的 seq、结束 byte offset、rolling
prefix hash）。journal fsync 仍是唯一 accepted 边界；索引写坏/写丢不反悔
事实，重启从 journal 自动重建。已有索引时只核验最后一条真实 journal
边界并扫描未索引尾，避免每次 OpenEventStore 全读历史。

fold snapshot 现记录 journal offset/hash。Resume 先读最多两个头事件做版本
守卫，再以 O(1) index record + snapshot hash 校验 cursor 和对应 journal 行，
seek 后仅解码 tail；任何 mismatch/旧 cursor-less snapshot 都安全回退全量
fold。兼容政策同步修订：additive 字段、旧 namespace 子集继续可读；旧
snapshot 缺新投影时只丢缓存全折，避免 interactions/team 等历史事实消失；
未知 namespace 或共享 namespace 版本冲突仍明确拒绝且不改原数据。

自动锚：`TestIndexedCursorReadsOnlyTailAndRejectsMismatch`、
`TestCorruptEventIndexRebuildsFromJournal`、`TestSnapshotTailEquivalence`、
`TestSchemaGuardAcceptsOlderNamespaceSubset`、
`TestResumeFullFoldsLegacySnapshotMissingNewProjection`。

---

## 2026-07-09 Claude Code 本地核心对照审计：新增 docs/CLAUDECODE-PARITY.md

以 Claude Code 本地 CLI/runtime 核心（2.1.x 至 2.1.205）为标尺的第三份
对照审计件，与 CODEX-PARITY 互为姊妹件。按开发者裁决只对标本地核心，
排除 cloud/IDE/桌面/生态外围（会话开场时的云端聚焦稿已按裁决整体撤销，
未入库）。三路证据：官方文档 ~25 页逐页抓取、本机 claude 2.1.144 binary
取证（28 内置工具 schema / 49 slash / 30 hooks 事件 / 386 env 全量 /
6 权限模式 / autocompact 阈值常量）、CHANGELOG 全量 4822 行 + 工程博客
+ 社区实测。调研由并行 sub-agent 完成，工作纸存会话 scratchpad。

结论：103 对照项 = 齐平/领先 37（10 项语义领先）· 部分 34 · 进行中 3
（INC-12）· 缺失 28 · 非目标 1。runtime 语义层同级或反超（durable
恰好一次 / 崩溃契约 / workspace 级时间旅行 / goal verifier / 树预算 /
OS 沙箱同代——对方 2.1.x 的 supervisor daemon/后台默认/SendMessage
resume 正向我们的形态收敛）；结构性差距集中在模型侧工程带：上下文
（microcompact 四级体系）、记忆（auto-memory）、hooks（30 事件×5
handler）、治理精度（auto mode 分类器/规则工程三件套/protected paths）、
生态（bundled skills/plugins/LSP）。五个深潜（§3）：checkpoint vs
barrier、上下文四级、auto mode 移植为 pipeline policy 源、agent teams
vs INC-12（恢复语义是我们反超点）、skill 模型侧 invoke。路线图 §4.2：
P0 三件（microcompact/G9 auto-memory/G19 hooks 扩展）全部不触不变量且
压在对方社区 top 抱怨带上。GAPS G9/G19 已挂参照注记。

## 2026-07-09 INC-11.8：runtime 基础加固收口（QA-21）

INC-11.1–11.7 的三层文档与验收锚全部并回。最终三视角 review 无剩余
P0/P1：correctness/concurrency 复核 durable command FIFO/receipt、snapshot
cursor fallback、team lease/revive 与单写者；security 复核 verifier/bash OS
sandbox、MCP egress/secret、isolated child worktree 与 approval escalation；
contract 复核 DESIGN/SPEC/QA、旧共享 session、CLI/inspect/WebUI。原
`TestParallelToolCalls` 的墙钟 ceiling 改成“全部 ActivityStarted 必须先于
第一条 terminal”的 journal ordering 断言，负载抖动不再制造假红。

**QA-21**：当前 main + 真 Gemini + 真实共享
`~/.local/share/agentrunner`，session
`20260710-000426-execute-the-team-task-now-exac-9c59`。动态 engineer 在
isolated worktree 写出精确 `REAL-ISOLATED-OK`，父 workspace 无泄漏；
inspect 展开 quiescent team task/member/workspace/base_ref；root/child 均有
`events.idx`，最新 snapshot 为 offset 14563 + rolling hash；当前二进制
resume rc=0 且 journal 48→48 行。当前 WebUI
`http://127.0.0.1:8788` 浏览器实测父 Subagents 面板/完成回执、子 read-only
详情与 write/message 链，console error/warning=0。共享 daemon 有活跃审批，
遵守数据纪律未重启；WebUI 用 `--no-daemon` 连接它。全部数据保留，归档
`qa/runs/2026-07-09-QA21/`。

---

## 2026-07-09 INC-12 多 agent 工程团队（UJ-23）：动态组队 · 树内消息 · 静止子唤醒 · 提权审批 · 子会话可见

**动机**：模拟软件工程团队——主 agent 动态生成 PM/架构师/SWE/reviewer
等角色,成员互发消息做 design/code review,目标统一、结果回流主 agent,
用户全程可点开每个成员（像看主 agent 一样）。用户裁决（2026-07-09）：
动态生成的复杂结构,**用户确认后权限可以放宽**——兑现决策 #32 政策
条款、修订决策 #20（不变量变更单见工作纸 §五）。

**落地**（新决策 #35/#36;决策 #20 修订;G10 关闭;本增量由两个并发
session 协作实施,以 origin/main 为汇合点）：
- **12.1 树内消息**：`send_message{to,text}`（to=parent/全 id/handle）
  → 目标 durable inbox（复用 store.AppendInbox:fsync+command_id 幂等+
  DeliverySeq 去重）;TreeRouter 树共享（与 Board 同族）,live wake
  best-effort、durable 为真相;发送者前缀进正文、source=agent 进元数据。
- **12.2 静止子唤醒**：ChildRevived 合成 background activity（原
  handle 不变、不二次配对、预算 reserve）→ 子 Resume 同 journal 续
  context → 第二次 SubagentCompleted;usage 按 baseline delta 结算
  （live/crash 同口径,防双计）;settle 后 PendingMail 收口+drive 入口
  scanPendingChildMail 兜重启;user-kill 标记仅 user-class 邮件可越。
- **12.3 用户直达成员**：`ar send <child-sid>` 经树根 CommandLog
  （UserInput.Target）→ 树根转投（CommandHandled{forwarded} 回执,
  对话零污染）——子的宿主永远是树根进程（单写者不破）。
- **12.4 动态角色**：`agents_dynamic` 开面;role=不可信模型输出（无
  hooks/MCP/skills 面、tools 仅父子集、沙箱棘轮继承）;构造 spec 冻结
  进 SpawnRequested.RoleSpec 与子 SessionStarted.Spec（revive 真相）。
- **12.5 提权审批**：escalate → spawn 无条件人审（allow 升 ask,
  escalation gate result 载请求规则）;批准=子管线以自声明 rules 替换
  父交集（树预算/深度扇出/工具子集/收容棘轮**无例外**）;拒绝/中断=
  降级交集继续并告知模型。
- **12.6 可见性（G10 关闭）**：成员事件带 session 标签入树根 hub;
  `ar attach <child-sid>`=成员 journal replay+hub 标签过滤 live;webui
  子会话 SSE、child_revived/forwarded/send_message 时间线渲染;CLI
  前台锚定主 session、成员事件折叠、成员审批带标注上浮。

**真验（QA-20,闸门 B）**：真 Gemini 共享 store+全局 daemon,lead 动态
起 engineer+reviewer,成员互发消息（含模型发错 id 又自纠的真实往复）、
协作期多次 revive（gen_steps 同 context 递增）、ar send 直达唤醒、单
SessionStarted context 延续——全 PASS,会话保留
（20260709-234601-task-381f）。**真验抓获三个孪生测不到的 bug**：
① CLI resolvePrefixLenient 对子 id 做 filepath.Base 截断全树地址;
② ensureRouter 原在 drive 内,Resume 的 mailbox replay 期转投遇
no-router 失败且 CommandHandled{forward_failed} 吞邮件;③ typed
ingress 的 source=cli 不在 user-class 白名单,转投归一/越标记判定不认
——全部修复并回归。

**记档**：QA 编号三次撞并发 session（17/18/19 均被占）,终号 QA-20;
qa/run-qa20.sh 按 CLAUDE.md QA 规则走共享 store（qa/lib.sh 的 XDG
隔离是旧惯例,新场景不再沿用）;handoff 血统子（a2+）不可 revive、
bash 进度 tail 仍在 G10 余项——均记档。工作纸归档
archive/increments/INC-12-agent-team.md。

## 2026-07-09 INC-13 microcompact——无 LLM 的轻量上下文回收（SPRINT #1）

CLAUDECODE-PARITY §4.2 P0① 落地：在 autocompact 之上加最省的一档，对标
Claude Code 四级压缩体系的 microcompact（§3.2）。context 估算跨过
`microcompact_at_tokens`（默认 = compact 阈值 3/4，先触发）时，
`ContextMicrocompacted{boundary}` 记一个单调边界；assembly 把边界之前的
可重算 read-class 工具结果渲染为占位符（"重跑工具即可"），execute/edit
类、错误、近窗（保护工作集 8 条）、小结果（≤200B）一律保留，tool call
与配对不动（决策 #9）。

**不触不变量**：journal 留全量结果（truth），只有装配视图降级——与
compaction boundary 同一 doctrine，故 fold 纯（决策 #3）、无 code replay
（#5）、fork/rewind/resume 天然良定义。单调 max-wins 保证装配前缀只在
事件落盘时变一次、不每 turn 抖（prefix-cache 友好）。触发在 step 边界、
compaction 之前：micro 先就地压小估算，compaction 常因此不再需要跑
（估算基于装配后视图，同 compaction 自终止）。DESIGN §4「Context
assembly」加 microcompact 段（additive，不改既有条款）。

**双闸门**：孪生 TestMicrocompact{AssemblyView（只降 read-class 大结果、
execute/错误/近窗/小结果保留、配对完整）,MonotonicFold（max-wins + 跨
compaction 存活）,TriggeredInLoop（loop 内触发、无 compact、末请求含
占位）,DisabledNoop（-1 关闭）}；event roundtrip 样本补齐。真实 API
QA-22（真 Gemini + 私有 daemon 隔离根 + 真 CLI send/attach）三红线全绿：
micro 触发、无 compact、模型见占位符后重跑 read_file（5→7）复原被清密钥
APERTURE-GRAPE-77。session 拷回共享 store、export 归档 qa/runs/。
`./scripts/check.sh` 全绿。

**并发协作**：QA 编号让路——INC-11.5 已占 QA-19，本增量让到 QA-22
（SPRINT SOP 的冲突避让）。
## 2026-07-09 INC-12 三视角对抗 review 收敛（安全/正确性/契约）

里程碑级增量的三视角对抗 review。安全（P0×1+P1×2）、契约（P1×2+P2×3）
两路先返回并修复；正确性/并发一路并行。**关键发现与修复**：

- **P0 路径穿越（安全）**：`send_message` 的 `to` 经 `TreeRouter.DirOf`
  时 `InTree` 只做前缀匹配、不拦 `..`,`filepath.Join(rootDir,"sub",
  "../../victim")` 可逃出树写入他会话甚至树外 inbox。修：`InTree` 逐
  `-sub-` 段过 `memberSegRe=^[A-Za-z0-9_-]+$`（禁 `.`/`/`）,与 spawn 侧
  `safeCallIDRe` 同源。测 TestTreeRouterRejectsPathTraversal。
- **P1 提权买断 hooks（安全+契约双发）**：`EscalationApproved` 分支只
  从父 pipeline 白名单取 Floor/SpawnGate,丢了 `hook.Gate`(pre) 且
  `childHooks` 仅 `!EscalationApproved` 构造(丢 post)——违反决策 #20
  "审批**只**替换 permission layers"（hooks 是并列机件,决策 #8,可
  deny）。修：escalated 分支保留除 PermissionGate 外的**全部**继承
  gate,post-hook 无条件继承。测 TestEscalationKeepsParentHooks
  （阻断 pre-hook 挡住提权子的 write_file）。
- **P1 `userClassSource` 回归（契约）**：INC-12.7 加的 helper 在某次
  rebase 冲突解决被对方旧版覆盖,`sendmsg.go`/`revive.go` 退回只认
  `{"","unix-socket"}`/`{"","user"}`,导致 `ar send`（cli 源）**无法
  唤醒 user-kill 的成员**（违反决策 #30"send 对任何 session 成立"）,
  且 LOG 已声称修复——典型 rebase 事故。修：补回 `userClassSource`
  （""/user/cli/unix-socket）统一两处。QA-20 因只 revive 未 kill 的
  成员而漏网;补正向测 TestReviveUserKilledOnCliMail。
- **P1 role 名注入（安全）**：动态 role.Name 是不可信模型输出,原样
  进"可信来源前缀"`[message from <name> (<sid>)]`,可用换行/`)]` 伪造
  二级 user 来源头。修：`roleNameRe=^[A-Za-z0-9_-]{1,64}$` 校验。测
  TestDynamicRoleNameSanitized。
- **P2 文档漂移（契约）**：QA-20 措辞补"cli∈user-class"、SPEC 删死锚
  `TestChildAttachLive*`、归档工作纸加更正注（决策号 #35/#36、测试名
  以活文档为准）。
- **P2 记档（安全,不修代码）**：默认 spec 无预算（0=无限）时消息风暴/
  revive 环仅靠 token 预算封顶,`AGENTRUNNER_APPROVE=always` 会自动批
  提权——无人值守跑动态角色树需显式配树预算与审批策略,列部署红线。

review 结论：P0/P1 全修并加回归测试,check.sh 全绿;P2 文档修讫 + 部署
红线记档。

## 2026-07-09 INC-12 三视角 review 收敛（2/2）：正确性 P0 grandchild relay

正确性/并发一路返回：**P0×1 + P2×2**。

- **P0 grandchild 投递保证破坏**：树 `R→C(静止)→G(静止)`,给 G 发消息时
  `Router.Send` 把邮件 append 进 G 的 inbox 并向上找活祖先 R、`revive←G`;
  但 `reviveChild` 把深层 sid 降级为 first-hop 直接子 C、读 **C** 的 inbox
  （空）→ no-op,G 的 durable 邮件永不投递,重启也不自愈（`scanPendingChildMail`
  只扫直接子）。修：①`reviveChild` 加 relay——深层目标读**收件人**inbox
  判定、re-host **first-hop 子**作中转,中转子 resume 后其 scan 接力下一跳;
  relay 中转子带 close/kill 标记则不穿过（决策 #30 保守）;②`scanPendingChildMail`
  改**递归扫整棵子树**。测 TestReviveGrandchildRelaysThroughParent（3 层
  R→C→G,restart 后 relay 链完整、孙恰好消费一次、单 SessionStarted,race 干净）。
- **P2 记档（不修/已随 P0 改善）**：①运行中被丢弃/推迟的 revive 请求
  （祖先 revive 通道满 / 子 store flock 未释放）无即时重试,靠下次
  重启的递归 scan 兜底（P0 修复已加强）;②树根每次 resume 会重读并空转
  转投 inbox 末尾的已处理 Target 命令（`forwardToMember` 只 journal
  CommandHandled、不推进根 `ConsumedInputSeq`）——inbox command_id 去重
  + relay revive 空转 → 无重复投递/双计,纯浪费,记 backlog。

三视角 review 全部收敛：安全 P0/P1、契约 P1、正确性 P0 全修并加回归
测试;各视角剩余项均为 P2 记档（部署红线 / backlog）。连续一轮无新
P0/P1。check.sh 全绿。
## 2026-07-09 INC-14 记忆写回核心——remember → 项目 CLAUDE.md（SPRINT #2，G9 取 A）

CLAUDECODE-PARITY §4.2 P0② 的写回核心落地，兑现 INC-D4 设计稿的**取 A**
（append-as-message，不触不变量）。`ar remember <sid> <text>` = durable
command（与 compact/clear 同 control 家族 / drainControls 路径）：
`memory.Append` 把 note append 到 **workspace-root CLAUDE.md**（append-only、
`## Remembered` 段、保留既有手写内容、**同 note 幂等去重**）+ 一条
program-source `InputReceived` 追加进当前对话（本会话即遵循，触发一次
确认续跑，与 goal 回灌同构）。文件供**下次** session start 被 memory
loader 冻结进 prefix——**不改冻结 prefix、不触任何 caching 不变量**。

**correctness 关口**：remember 有文件副作用，durable command 崩溃重放
（Append 后、journal receipt 前崩）可能双写——`memory.Append` 检测该 note
已在文件则 no-op，把重放吸收成幂等。孪生 TestRememberControlIsIdempotent
钉住。

**连带澄清**：memory 块在 session-start 冻结的 prefix 里，compact/
microcompact 只动 boundary 之后的消息——**记忆在压缩后永不丢**，我们
天然规避 Claude Code 的 top 抱怨 #29890（"压缩后不 consult memory"），
无需补丁（CLAUDECODE-PARITY #31 结论更新）。DESIGN 决策表加 #37 + §4
prose 命名 memory writeback 为允许操作（取 A 不翻转任一既有格）。

**双闸门**：memory 4 单元（Append 建/追加/保留手写/幂等去重/拒空）+
agent 2 孪生（remember → 文件 + program input + 确认 turn；重复同 note
文件不双写）；真实 API QA-23（真 Gemini + 私有 daemon 隔离根 + 真 CLI）
三红线全绿——note 写入 CLAUDE.md、session 1 见 program input、**全新
session 2 冻结遵循 pnpm 约束**（跨会话记忆生效，本增量靶心）。session
拷回共享 store、export 归档 qa/runs/2026-07-09-QA23/。`./scripts/check.sh`
全绿。

**余项（auto-memory 完整体，独立增量，挂 SPRINT #2 余项）**：MEMORY.md
索引（200 行/25KB）+ 主题文件按需读 + per-agent agent-memory + @import +
`.claude/rules` 条件加载（对标 Claude Code auto-memory）。

**并发协作**：QA-23 编号避让并发（origin 已用到 QA-22）；本轮 sync-in
干净、代码正交无冲突。

## 2026-07-09 INC-12 第二轮 review（扩展层交互）：fork × revive 隔离守卫

第二轮"扩展层交互 + 第一轮盲区"review 返回 **P1×1 + P2×3**（核心机制
第一轮已收敛稳固,本轮查新机制与扩展层的交互）。

- **P1 fork × revive 隔离击穿**：`internal/fork` 的 `os.CopyFS` 把 `sub/`
  verbatim 复制（含子 durable inbox + 子 journal）,子 `SessionStarted.
  WorkspaceRoot` 是**原 session 的绝对路径**（fork remap 不改 payload）。
  fork resume 时 `scanPendingChildMail` 按文件系统扫到复制来的静止子有
  pending mail → `reviveChild` → `childExecutorFromJournal` 读 stale
  WorkspaceRoot → `workspace.New(原路径)` → **fork 的被 revive 子写进原
  session 的 workspace**（跨会话数据污染,违反 DESIGN §12"fork 与原
  session 不共享目录"）。根因：fork"verbatim 复制 sub/ = 无害
  provenance"设计**早于** INC-12"静止子可被 revive 重新运行",叠加后
  provenance 里的 stale 绝对路径不再无害。修：`childExecutorFromJournal`
  加 fork 隔离守卫——合法子的 workspace 要么==父 WS root（shared）、要么
  在**本 session store 目录下**（isolated,`<store>/sub/<call>/worktree`）;
  其余是跨 fork stale 路径 → 返回 `errForeignWorkspace`,revive 优雅跳过
  （不 `workspace.New` 外部路径、邮件留 durable、warn）——mirrors
  cancel_at_fork（fork 是新起点,旧团队要续则 fork 后重新 spawn）。
  `underDir` 用 EvalSymlinks 归一（macOS /tmp↔/private/tmp）+ filepath.Rel
  边界（防 `/a/bc` 误判在 `/a/b` 下）。测 TestReviveRefusesForeignWorkspace
  /TestUnderDir;既有 TestIsolatedTeamWorkspaceSurvivesRevive 证合法
  isolated 子不误伤。
- **P2 记档/加固**：
  ① **trust taxonomy**（加固）：send_message 原只设 `Source:"agent"`,收件方
    ingest 缺省落 principal="local-user"/trust="unknown"——把 peer 误标成
    人类。虽 principal/trust 当前无消费点（dormant）,前瞻加固：显式设
    `Principal:"agent:<sid>"`/`Trust:"untrusted"`（决策 #19 精神:跨 agent
    内容不因来自树内而提权）,防未来 trust 用于注入防御时误抬权。
  ② **fork × send_message handle 寻址**（记档）：fork 父 journal 的
    `SpawnRequested.ChildSession` 仍是原树 id（remap 不改 payload）,fold
    出的 ChildSessions 全 stale → fork resume 后模型对继承成员
    `send_message{to:<handle>}` 报"not a handle you own"。与 P1 守卫一致
    ——fork 不延续旧团队,要协作则 fork 后重新组队。backlog。
  ③ **team task DAG × revive leaseID**（记档）：revive 把 task 从
    quiescent 翻回 leased,对其新建依赖会被 planSpawn 拒（语义可辩护:
    revive=重新活跃）;`teamRevive` 写的 leaseID 格式与 spawn 侧不一致但
    LeaseID 不参与 gating,cosmetic。backlog。
- **已核对正确不报**：barrier vector 覆盖 revive 的第二次回执/ChildRevived
  （去重开一次）;在飞 revive handle × cancel_at_fork 正确结算;compaction ×
  source=agent 消息正常摘要;handoff a2+ 不可 revive 已记档;driver/best-of-N
  各 iteration 独立树无冲突;webui SSE 过滤正常。

## 2026-07-09 INC-12 第二轮 review（修复回归+并发）：热循环守卫 + relay 软跳

第二轮"修复回归+并发"review 审第一轮修复本身,返回 **P1×1 + P2×2**
（path/escalated-hook/userClassSource 三项修复经探针核验无绕过/无回归）。

- **P1 无限 revive 热循环**：reviveChild re-host 一个"Resume 会**确定性
  失败**"的子（in-doubt 残留 side-effecting effect、MCP schema 漂移、
  sub-state 版本不匹配、materialize 失败——都在消费 mailbox **之前**
  返回 error）→ `settleBackground` 的竞态 close-out 检测 `childHasMail`
  仍真 → 把同一 sid 重新入队 `l.revive` → 下一轮 re-host → 又失败……
  **无 per-sid 上限,永续**（CPU peg + journal 无界 + 预算烧尽）。且第一轮
  relay 修复把 `scanPendingChildMail` 改递归,把触发面从直接子扩到任意
  深孙代。修：close-out 重入队加 `!out.isError` 守卫——**只在成功 settle
  后**重入队（真竞态:邮件在子出场时到达）;失败 revive 不重入队,邮件留
  durable 待重启 scan / 根因修复后显式 send。测 TestSettleDoesNotReenqueue
  FailedRevive（error settle 不入队、success+pending mail 才入队）。
- **P2 relay 收件人 fold 硬错误炸全树**：relay 分支 `childFoldState(rdir)`
  硬 error → `drainRevives` 上抛 → `drive` abort → 整棵 tree host 停摆。
  一个孙代 journal 瞬时损坏就够。修：软跳（warn + return nil,与相邻
  DirOf 出错一致）,单成员问题不炸全树。
- **P2 语义记档（有意,不改）**：`ar send <被杀中间父之下的孙>` 的
  user-class 邮件不穿过被杀的中间父（relay 遇任何 mark 即 return nil）
  ——与直接子的显式 reopen 不对称,但符合"kill 标记不被自动路径越过"
  （relay=自动 re-host 中间父）。用户须先显式 reopen 中间父。DESIGN §3
  revive 小节已述,此处补记为有意语义。
- **已核验无回归**：path 加固无绕过（daemon handleSend/CLI resolvePrefix
  最终都汇到 DirOf 地板）;escalated 保留 gate 无 double-add、hook.Gate
  浅拷贝并发安全、非提权路径行为未变;userClassSource 两处一致、agent 源
  不被归一。

---

## 2026-07-09 黑盒探索 QA Round 1：三路 agent 全面探索 + 修复批 #1

**方式**：三个探索型测试 agent（CLI 新手日常 / CLI 进阶编排 / webui
浏览器全流程）以真实用户身份、真实 Gemini API、共享 daemon 黑盒自主探索
（不读内部文档/源码），产出 40 条 findings（P1×3、P2×15、P3×22，
含正面验证清单）；报告归档 session scratchpad qa-round1/。主 agent
判重（对照 GAPS 已知缺口）后修复。本批修复（check.sh 全绿 + webui
tsc/build 绿）：

- **F-A01/F-B1 submit 挂死（P1）**：静止模型下 hosted session 不再
  "结束"、daemon 不关流，旧 Dial 等一个永不到来的 EOF。修：DialUntil +
  standby idle 即 detach（决策 #31 的 follower 契约），drive 仍等
  run_end；exit code 语义同步修正；session 行 announce 一次（曾双打）。
  钉 TestSubmitReturnsAtStandbyIdle。
- **F-B2 -sub- 寻址冲突（P1）**：顶层会话 slug 含 "-sub-"（自由任务
  文本铸 id，如 "spawn 3 sub-agents"）时被解析器误判为子会话地址，
  new 打印的 id 全家命令不可用。修：resolveSessionDir 顶层精确优先→
  子切分点从右向左枚举（磁盘验证）→顶层前缀兜底；resolvePrefixLenient
  以目录结构（父 hop 是否 "sub"）判子会话；daemon 新增 SplitAddress
  注入点（CLI 接 store-aware 解析），无注入时保持旧首切分。团队成员
  handle 是角色名（非 call_N_M），结构正则不可判——存在性判定是唯一
  完备解。钉 TestResolveSessionDirTopLevelWithSubInSlug。
- **F-A07/F-A09/F-B8 -h 系统性失效（P2）**：15 个位置参数命令把 -h
  当 sid/路径（init -h 写出名为 -h 的文件、trust -h 信任了它）。修：
  分发前统一拦截 + commandHelp 集中文案；flag 命令保留原 flag 帮助。
  钉 TestPositionalCommandsHonorHelpFlag。
- **F-A03 compact 遇图片破功（P2）**：summarizer 请求不 inflate CAS
  ref，provider 拒绝整个请求、压缩静默失败。修：compactContext 走
  inflateBlobs 同一管线。钉 TestManualCompactInflatesImageParts。
- **F-A02 new --detach 幽灵会话（P2）**：daemon 侧早期失败（坏
  workspace/spec）时 client 已带着 sid 离开、run 未落盘。修：new/submit
  发出前本地预检 LoadSpec + workspace stat（与 run 对齐）。
- **F-A04/F-B7 resume 凭据（P2）**：resume 只读 cwd .env；修为再读
  session 记录的 workspace 根 .env（cwd 优先，不覆盖）。
- **F-B3 no_op 谎报成功（P2）**：kill/goal 子命令/compact/clear/
  interrupt/close 的 ack 全部改为如实投递语义（"requested/delivered
  (a no-op unless …)"），close 提示 send 可复活（F-A18 一并收）。
  结果同步回传（等 command_handled）记为后续增量候选。
- **F-A05/F-A08 空消息（P2）**：run/new/submit 客户端拦空 task（曾
  透传成原始 gemini 400 + stranded 会话；new 曾报 "run needs…" 张冠
  李戴）。F-B6：submit 支持尾随 flag（reorderFlags 对齐 run）。
- **F-A06 坏 sid 报错（P2）**："(command log write failed)" 误导为
  磁盘故障，改透传真实原因（"no session matches…"）。
- **F-B5 barrier locked（P2）**：报错补出路（"quiesce it first:
  agentrunner stop <sid>"）。
- **F-A11 sessions TURNS 口径（P3）**：列值从 gen-steps 改为与 inspect
  同源的对话 turns。F-A13：歧义前缀 >5 匹配改汇总示例。F-A14：缺
  API key 报错补修复指引。F-B10：goal 进顶层 help。
- **webui F-C1（P2）**：5 处 window.prompt（workspace 路径/自定义
  model/新分支/commit message/worktree repo）全部换 app 风格
  PromptModal（独立 store 槽、可叠加在主 modal 上）——原生 prompt 同步
  冻结渲染主线程且与全站风格割裂。
- **webui F-C3（P2）**：会话 composer 权限 pill 曾对新载入 tab/fork
  会话谎报 "Ask to approve"——remembered 档位是 in-memory Map（刷新即
  丢）且 fallback 把 mode default 猜成 ask。修：sessionSpecs 持久化到
  localStorage（跨 tab/刷新一致）；default 模式 recall 不到时诚实显示
  "Spec-defined access"（灰点 + tooltip）；fork 继承源会话档位。

**未修记档**：F-B4 interrupt 不及后台任务边界透明度（interrupt 语义
提示已在 ack 文案带上，进一步的 per-handle 提示待设计）；F-B9 子会话
独立观测面（ar sessions 不列子会话）；F-C5 goal budget 截断的终态
展示（goal_achieved{reason:budget} 命名误导）；F-C4 无 goal 时模型
误调 goal_complete 的时间线红叉弱化；F-C6 /clear no_op toast（依赖
结果回传增量）；F-C7 Search 不过滤主网格——列 Round 2 观察或后续
增量。测试数据全部保留（37+ 会话在共享 store，sid 清单见各报告）。

## 2026-07-09 INC-15 hooks 生命周期事件族第一批（SPRINT #3，G19）

CLAUDECODE-PARITY §4.2 P0③ 落地——P0 三件（microcompact/记忆写回/hooks
扩展）至此全部完成。hooks 从 pre/post tool 扩到 8 个生命周期事件：
`hook.RunLifecycle`（复用 runOne 基建：sh -c + JSON stdin + 凭据剥离 +
超时 + pgid）+ settings `hooks.lifecycle`（event→commands，加载期校验
事件名，merge 纪律同 pre/post：user 恒生效、project 需 trust）+ loop
八个 journal 点位挂 `fireLifecycle`（nil-safe，notes 上 live 流）。

**语义分类**：observe-only（session_start/session_end/subagent_start/
subagent_stop/post_compact/stop——事实落 journal 后触发，任何退出码不
改控制流）；blockable（user_prompt_submit：exit 2 → 输入不落 journal
不起 turn；pre_compact：exit 2 → 跳过本次压缩）。**两个 correctness
关口**：①auto-compact 被否决后不得 `continue` 重试同一 due-check（会
无限自旋）——compactContext 改返回 (compacted bool, err)，否决/空
summary 走 fallthrough，孪生 TestPreCompactHookSkipsAndNoSpin 钉住；
②hooks 不重放——挂点只在 LIVE 跨越时触发，recovery 的 settle-from-
child-fold 路径**不**发 subagent_stop（与"恢复不重放 hook 副作用"教义
一致，决策 #8 同族）。决策 #11（observe+block、不改写）保持不动；
DESIGN §effect-pipeline 加生命周期事件族段。

**双闸门**：孪生 TestLifecycleHooksFire（stdin payload 断言）/
TestUserPromptSubmitHookBlocks（veto 不落 journal）/
TestPreCompactHookSkipsAndNoSpin（否决+不自旋）/
TestObserveHookFailureDoesNotBlock（坏 observe hook 无害，exit 2 在
observe 事件上惰性）；真实 API QA-24（真 Gemini + 私有 daemon +
user 层 settings.yaml）四红线全绿：session_start 触发、FORBIDDEN 输入
被 veto（无 journal 无 turn、session 存活）、后续正常输入照常问答、
stop 在静止触发。归档 qa/runs/2026-07-09-QA24/。check.sh 全绿。

**余项记档**：更多事件（Notification/FileChanged/ConfigChange 类）、
handler 类型扩展（prompt/agent/http）、改写类（决策 #11 明示推迟）;
journey 覆盖债仍在（无 journey 压 hooks）。

## 2026-07-09 INC-16 权限规则工程三件套（SPRINT #4，#53）

CLAUDECODE-PARITY §2.06 #53——权限疲劳（对方遥测 93% 反射式批准）的主解，
同时修一个安全弱点：`PermissionGate.Check` 旧行为对**整条 command** 匹配，
一条 `Bash(git *)` allow 会误放行 `git status && rm -rf x` 里搭便车的 rm。

三件套（`internal/pipeline/command.go` 三个纯函数）：①**复合命令逐段
匹配**——splitCompound 按顶层 &&/||/;/|/&/换行拆（引号内不拆、不平衡引号
退回整体），每段独立裁决聚合取**最严**（deny>ask>allow，未匹配段落 mode
default）；②**wrapper 剥离**——stripWrappers 剥白名单前缀（timeout 带
-k/-s 值/time/nice/nohup/stdbuf/裸 xargs），使 `timeout 60 npm test` 仍
匹配 `Bash(npm test)`，拿不准不剥；③**只读集**——isReadOnlyCommand 认
ls/cat/grep/find 等非执行内置，无规则时免提示 allow。

**安全立场（本增量核心）**：两件收紧（逐段=修 bug、wrapper 剥离单调更严）
+ 一件受控放松（只读集，本就无害 + OS sandbox 兜底）。**安全序修正**：
初版把只读集判在规则循环前（会让只读越过显式 `deny cat *`）——改为
**规则先行、只读兜底**，显式 deny/ask 永远先于放松（TestReadonlySet
YieldsToExplicitRule 钉住这条安全回归）。find -exec/-delete、含 >/`/$(
的段排除出只读集。fail-safe：拆分/剥离拿不准退回整体（只更严不更松）。

**双闸门**：9 个 pipeline 孪生（splitCompound/stripWrappers/isReadOnly
单元 + 逐段聚合/wrapper/只读/引号分隔/显式 deny 先于只读集的集成，含
安全案例）；真实 API QA-25（真 Gemini + 私有 daemon）——**文件系统硬
红线**：配 git-allow+rm-deny，让模型原样跑 `git status && rm -rf victim`，
**victim.txt 存活**（rm 段逐段 deny，旧整条匹配会删掉它），且 git 命令
正常执行（allow 半边生效）。归档 qa/runs/2026-07-09-QA25/。check.sh 全绿
（一次撞并发 golangci-lint 锁，重试即过——环境非代码）。DESIGN §5
permission 加「命令粒度匹配」段。

**余项**：参数级匹配 `Tool(param:value)`（#55）、path 规则 gitignore 风
锚点增强（#54）——独立小增量。
