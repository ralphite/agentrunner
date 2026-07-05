# REVIEW-001 · 术语逐条 review 意见收集（工作纸）

> 性质：开发者对 §18 术语(按 20 项优先级清单)逐条提出的问题与看法,
> **只记录、不响应、不处理**;攒够后统一分析与裁决,落地后本纸归档。
> 记录纪律:每条尽量保留原意;我方不加评注(有歧义只标[?])。
> 优先级清单与状态见 2026-07-05 对话(1–7 已 review,8–20 待过)。

---

## #1 session（复审意见,2026-07-05）

现行定义:session = 一次会话,长期存在;实现 = id + inbox + journal + state。

开发者意见/问题:

1. **id 指什么?** 这个 id 是 session id 还是 agent(id)?
2. **需求:一个 session 必须可以更换 agent**(session 生命周期内
   agent 可变)。
3. agent 对应的是一个 actor,一个 actor[?]有 inbox;**journal 在这个
   结构里指什么不清楚**——开发者后面要自己看。
4. **想法/疑虑:session 不应该与 agent 绑定**。session 的本质是
   持续跟随的 event log(keeps following the event log);把它和某个
   agent 绑死可能出问题。

（待分析时的关联线索,仅索引不评述:GAPS G8「运行中 spec 变更/角色
切换」;DESIGN §9 spec 冻结于 SessionStarted;§18.1 session 词条。）

## #5 final generation（复审意见,2026-07-05）

现行定义:turn 的最后一个 generation step,不带 tool call;标志 turn 结束。

开发者意见/问题:

1. 现在要求 final generation 不带 tool call。**有没有可能模型出问题**
   ——比如"最后一个 generation 带着 tool call,我们执行完这个 tool
   之后,模型就不再生成了"?这种情况会不会发生?不确定,可能与模型
   库(SDK/provider)怎么处理有关,也可能与模型本身的稳定性有关;
   **不知道要不要特殊处理**。
2. 倾向:如果这属于错误,那它是一个**异常**;而如果是异常,**所有的
   step 都可能出错**——可以考虑把"step 出错"统一作为一套异常处理,
   不为 final generation 单独特判。

（待分析线索,仅索引:DESIGN §4"异常终止形态"条款(malformed_tool_call
/safety/blocked 的既有策略)、§5"每种关卡结果都定义模型看到什么"、
决策 #9 配对红线——分析时对照"模型不再生成"与这些机制的覆盖关系。）

## #9 inbox / Input（复审意见,2026-07-05）

现行定义:inbox = per-session 持久有序输入队列;Input = tagged union
五种(user_message / child_result / tool_result / timer / control),
全部 journal 为 InputReceived。

开发者意见/问题:

1. **反对 Input 的显式类型分类**,理由有二:
   - **扩展性**:后续扩展会有各处"支持度"的问题(每加一种来源就要
     全链路认识这个新类型);
   - **类型必要性**:Input 本质上只是给大语言模型看的内容,**不需要
     强类型**——文本即可,多模态也一样能表达。
2. 替代方案:**来源信息用内容前缀(prefix)表达,不用类型字段**——
   - tool call 的结果:正文前加一段说明"这是哪个 tool call 的结果";
   - 子 agent 的返回:像普通文本一样处理,前缀标明是哪个 agent、
     哪个 session id、返回了什么;
   - 其他来源同理。
   这样灵活得多,因为本质上不需要强类型。
3. 边界澄清:**作为整个系统的 log(event 记录/审计),可以有类型**;
   但 Input 本身是"给 agent 的东西",类型放进这个 event 里完全不需要。

（待分析线索,仅索引:provider tool_result 配对红线(决策 #9:
Gemini 要求 functionResponse 与 functionCall 严格配对,tool 结果作为
纯文本 prefix 是否破坏配对,需分析);control{kill} 不进对话的现行
语义;§17"inbox 字面统一度"记档。）

## #11 interrupt（复审意见,2026-07-05）

现行定义:带外信号(不进 inbox)——turn 中 = 打断当前活动(部分输出
保留);待命处 = close(记 SessionClosed 意图)。

开发者意见/问题:

1. **对 interrupt 这个词对应的用户功能不理解**——从用户功能出发,
   它到底对应什么?
2. 能想到的对应物:用户(或其他 agent 经由 parent)把当前 agent
   session 的在跑工作杀死,或把 running tool / background task 杀死。
3. **杀死 ≠ session 结束,完全不是**。最常见的需求是:"我看到它在做
   的事完全不是我想要的 → kill 掉 → 再写一条消息让它继续执行。"
4. **不需要"完全停下来"的情况**:任何 session 在任何时候都可以继续
   发消息、继续执行。
5. 结论:**不接受现行定义里 interrupt 导致 session 被 end / turn 被
   end 的情况**(含"待命处 interrupt = close"这条交互惯例)。

（待分析线索,仅索引:"turn 中 interrupt = 打断活动+部分输出保留+
会话继续"与开发者第 3 条描述一致,分歧点集中在"待命处=close"惯例
与措辞;close 已是可重开意图(决策 #30);kill 工具族(task_kill/
ar kill)与 interrupt 的关系待统一梳理。）

## #12 fold / state（复审意见,2026-07-05）

现行定义:state = 纯函数 fold(journal),不读时钟/外部存储/无副作用;
session 唯一的工作内存。

开发者意见/问题:

1. **state 指的究竟是什么?** 具体地问:它是不是"针对大语言模型的
   一个 request"?
2. 开发者的对照理解(以 Gemini 为例):一个 request 由 system
   instruction、history、tools 等组成——其中 system instruction 与
   tools 更多是从 agent spec build 出来的;**state 看起来更像其中的
   history(对话历史)那一部分**。

（待分析线索,仅索引:现行 state 内容 ≠ 仅 history——fold 出的
Session 子状态还含预算用量/在飞任务/等待状态/权限 mode/mailbox 高水位
等 runtime 记账,"LLM request = assemble(state, spec)"是现行分层
(context assembly 词条 §18.9);分析时需回答"state 里哪些是给模型
的、哪些是给 runtime 的",并对照 #1 意见 4(session=event log 本体)
与 #9(Input 弱类型化)一并裁。）

## #13 终止语义三件套（复审意见,2026-07-05）

现行定义:TaskCompleted 回执 / SessionClosed 意图 / 显式重开;除两类
事实外无"结束"事件。

开发者意见/问题:

1. **为什么要提供"终止"?不理解**——想不到任何一个地方需要终止语义。
2. 用户 kill 掉一个 session 或一次执行,**只需要标记"它被 kill 过"**,
   不意味着被终止;用户还可以继续给它发消息。
3. 承认一个真实的待决问题:**子 agent 被用户 kill 后,parent 允不允许
   重启它/继续给它发消息**——这确实是个问题,取决于具体状态。
4. 但即便如此也**不需要一个"终止状态"**:只要有 kill 标记;如果裁决
   "kill 后不许 parent 再发消息",那就基于标记直接做检查拒绝即可,
   不需要状态机。
5. 总评:三件套**疑似过度设计、过于复杂**,与最终需求没有直接关联。

（待分析线索,仅索引:与 #11 同族,合并裁;现行"意图只挡自动恢复、
不挡显式重开"在机制上已接近"标记+检查"而非封印,分歧点可能在
①TaskCompleted 回执是否该作为概念存在(其消费者:driver 判迭代/父
结算/headless 退出码)②"终止/terminal"这组命名本身;kill 后父可否
re-spawn 同名子任务 = 现状允许(handle 已终、模型可自行重启,fork
处置向量记档同语义)。）
