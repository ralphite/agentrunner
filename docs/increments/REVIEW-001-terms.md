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
