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
