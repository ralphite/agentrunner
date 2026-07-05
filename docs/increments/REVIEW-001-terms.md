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
