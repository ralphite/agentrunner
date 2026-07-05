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

