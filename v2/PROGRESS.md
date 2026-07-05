# v2 实施台账（决策与偏差记录）

执行协议见 v2/PLAN.md §0。一步一条目，倒序不排，按时间追加。

---

## V2-M1.1 — conversational park — DONE

`Loop` 加 `Conversational bool` + `UserInputs <-chan string`（注意:
`Inputs` 名被 v1 的 artifact inputs 占用,故取 UserInputs）。decide()
自然结束分支前插 conversational 判定:**先给已到达的输入起 turn
（hasInputAfterLastAssistant),真空闲才 park**——顺序错了会把刚
journal 的输入再 park 一次(红测试当场抓住,修正后绿)。park =
journal WaitingEntered{input} → select{UserInputs/bg.done/Interrupts/
ctx}:收到输入 → journal InputReceived{source:user}(redact 过)→
WaitingResolved{input_received} → doTurn;通道关闭 → resolved{closed}
→ epilogue → RunEnded{closed}(新 reason);后台任务落定 →
resolved{task_settled} → 既有回灌路径;idle 时 interrupt = 关闭手势
(closed_by_interrupt)。protocol 加 KindIdle(前端 REPL 提示符信号)。

**记档**:①conversational 撞 maxTurns 时带着未消费输入也只能 park→
close(会话 turn 预算语义,M2 视需要再议);②task 模式零变化
(TestTaskModeStillEndsOnYield 断言),v1 全部测试+26 acceptance 回归绿。
三测试:三输入三 turn 一终态、close resolution、task 模式不变。
下一步:M1.2 外部投递(PostInput + daemon send + CLI new/send)。


## V2-M1.2 — 外部投递(daemon send + CLI new/send/close) — DONE

**设计冲突解决(记档)**:PLAN 原设想 send 经 store 直接 Append。但
loop 的 in-memory fold 是单写者(drive goroutine),daemon 直接写
store 会让 loop 的 ds.s 看不到该输入。改为 **send 经 hostedRun 的
inbox channel 投给 loop,由 loop 自己的 appender journal**——
journal-inputs-first 在**消费侧**保持(loop 收到即 journal,再被下个
turn 消费)。**send 侧崩溃窗口**(enqueue 后、loop journal 前进程死)
的 durable ack 留给 M5 记档。

**daemon**:Command 加 Conversational/Text;RunRequest 加
Conversational/Inbox;hostedRun 加 inbox chan(buffered 64,type-ahead)+
post()/closeInbox();新命令 send(查 runs 注册表→post)与 close
(关 inbox→parked loop 走 epilogue);finish 关 inbox 兜底。handleRun
按 Conversational 建 inbox 并接进 RunRequest;hostRunFunc 把
Conversational/Inbox 接到 Loop。send 是**投递入口的统一抽象**——
人/web/机器(webhook)将来都走这条(v2 DESIGN §2)。

**CLI**:new(起 conversational 会话,dialUntilStart 拿 RunStart 即
detach,会话在 daemon 续命)、send <sid> "msg"、close <sid>;
一问一答走既有 Dial。

**测试**:daemon 级 C1 孪生(three inputs over wire→3 turn→close,
断言 3 输入/1 终态/reason=closed;scripted 确定性)。全量 check +
race + stage 5/6 acceptance 回归绿。下一步:M1.3 park 恢复,然后
M1 出口闸门 QA-01 真实 API。
