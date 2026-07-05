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

## V2-M1.3 — park 恢复 — DONE

parkForInput 抽出(fresh park 与 resumed park 共用,均不重复 journal
WaitingEntered):close → epilogue 出 RunEnded{closed};**ctx 取消
(崩溃/shutdown)不落终态**——parked 会话必须能 resume,只有真 journal
错误才 best-effort 兜底终态(测试当场抓住:初版 ctx cancel 误落 RunEnded
导致 resume 前会话已 end)。drive 的 doWait 加 WaitInput 分支:折出
Waiting:input 的会话 resume 时直接 re-park(WaitingEntered 已在 fold)。
WaitingEntered{input} 折为 StatusWaiting(非 ended),resume 不拒。

记档:daemon 重启后重新托管 parked conversational 会话属 M5(QA-08)——
timer sweep 只管 WAITING_TIMER,input park 无 timer;foreground `resume`
一个 conversational 会话若不带 UserInputs 会永久 park(等 Ctrl-C),
daemon 路径是设计入口。

测试:in-process C10a 孪生(park→ctx 取消模拟崩溃→reopen→resume
re-park→一条输入续到 close,断言 2 输入/1 终态/answer-two 可见)。

## V2-M1 里程碑出口 — 双闸门 GREEN

- **闸门 A(scripted 孪生)**:TestConversationalMultiInput /
  CloseResolution / ParkResumes + daemon TestDaemonConversationalSendClose
  全绿,进 check.sh 常跑。
- **闸门 B(真实 API QA-01)**:v2/qa/run-qa01.sh 驱动真实 ar 二进制 +
  daemon + **真实 Gemini API**(gemini-flash-latest),三轮续聊,连续
  两次 PASS(1 session / 6 turns / 3 inputs / 1 terminal closed)。
  修 bug:set -e+pipefail 下 grep -c 无匹配返回 1 会中止脚本(count_type
  包装)。

**C1 达成**。全量 check + race + stage 1–7 全 26 acceptance 回归绿。
下一步:M2 inbox 完整化(忙时排队 + interrupt 分立 → QA-02/06)。
