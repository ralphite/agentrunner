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

## V2-M2.1 — 忙时排队(type-ahead 批量) — DONE

**设计定型(记档)**:排队消费的正确位置是 **awaitInput 内批量排空**,
不是 loop 顶部。初版把 drainInbox 放 loop 顶,与 Waiting 状态纠缠
(resume 时 Waiting≠nil 使 decide 走 doWait 而非 doTurn,drained 输入
被 journal 却直接 close——测试当场抓住)。改为:awaitInput 醒于第一条
输入 → journalInput → drainQueued 非阻塞排空其余(type-ahead 批量,
到达序)→ resolve → 下个 turn 一次看到全部。

**语义**:输入按到达序 journal;忙时(turn 在飞)到达的消息静躺 inbox
buffered channel(**从不打断在跑的 turn**——send 与 interrupt 是不同
channel),下个 park 批量入同一 turn 的上下文;spaced 到达(每条在
一个 park 期到)则自然一 turn 一条。inboxClosed 标志让 drain 中见到
的通道关闭在 park 时转为 close。

**记档**:journal-on-boundary(park 时)非 journal-on-arrival(send
瞬间)——后者需从 send 路径直写 fold,破单写者;durable-on-arrival
与 M1.2 的 send 侧 ack 同族,留 M5。

## V2-M2.2 — interrupt 与输入分立 — DONE

结构上已分立(send→inbox channel,turn 中不读;interrupt→Interrupts
channel,turn 的 interruptScope 中读)。新增 v2 语义:**idle 时
interrupt = 关闭会话**(交互惯例;turn 中 interrupt 仍是 steer/取消
活动,v1 既有)。resolution closed_by_interrupt。

测试:MultiInput 改反应式喂入(一 turn 一条,spaced 语义,3 turn)、
新增 TypeAheadBatches(两条排队入一 turn、到达序断言)、
IdleInterruptCloses。全量 check + race + stage 4/6 回归绿。
下一步:M2 出口闸门 QA-02/QA-06 真实 API。

## V2-M2 出口闸门(部分)— QA-02 真实 API GREEN

抽出 v2/qa/lib.sh(qa_setup/qa_spec/qa_daemon/qa_new/qa_wait_turns/
count_type)供各 QA 复用。**QA-02**:真实 ar+daemon+真实 Gemini,慢
bash(sleep 6)跑一个 turn,期间 send 两条 → 断言 bash 未被取消
(无 activity_cancelled)、3 输入按到达序(两 send 的 input_received
落在 bash activity_started 之后)、1 终态收尾。连续两次 PASS。
QA-06(interrupt)待 M2.3 daemon interrupt command 接线后跑。

## V2-M2.3 — daemon interrupt command — DONE

hostedRun 加 interrupts chan(buffered 1)+ signalInterrupt();Command
新增 interrupt 命令(查 runs → signalInterrupt);RunRequest 加
Interrupts,hostRunFunc 接进 Loop.Interrupts;CLI interrupt <sid>。
interrupt 与 send 是**不同 channel、不同语义**:turn 中 interrupt 经
interruptScope 取消当前活动(steer,续跑),idle 时 awaitInput 收到即
关闭——两个消费者但同一时刻只一个活跃(turn 跑 XOR park)。

## V2-M2 里程碑出口 — 双闸门 GREEN

- **闸门 A(scripted 孪生)**:TypeAheadBatches/IdleInterruptCloses +
  M1 全部 + daemon send/close 测试,check.sh 常绿。
- **闸门 B(真实 API)**:**QA-02**(忙时排队,bash 未取消/3 输入到达
  序/1 终态,×2 PASS)+ **QA-06**(interrupt 取消在跑 bash、进程被杀、
  会话续跑,×2 PASS)。

**C2、C8 达成**。全量 check + race + stage 1–7 全 26 acceptance 回归绿。
下一步:M3 后台子 agent(routing provider 前置 → QA-04/05,核心里程碑)。

## V2-M3.0 — routing provider — DONE

scripted.Router:按请求会话里出现的路由键(agent 的 system prompt +
user 消息文本)匹配各自的脚本子 provider,每个维护独立步进——并发
子 agent 的响应确定可复现(GAPS G4)。NewRouter(RoutePair...) 按
match-priority 序;无匹配显式报错(不静默给错脚本)。

## V2-M3.1 + M3.3 + M3.4 — 后台子 agent(并行 spawn + 回执激活 + 结算) — DONE

**spawn_agent{background:true}** 走 bg 机制(复用 bash background 的
launch/settle/cancel 骨架,一套机制不新建):launchBackgroundSpawn 在
drive goroutine journal SpawnRequested + ActivityStarted{Background}
(fold 立即配 handle {task_id,status:running},turn 不阻塞),注册
cancel,goroutine 跑 childLoop.Run;完成推 bgOutcome{subagent:
SubagentCompleted, usage, result:child report}。**settle**:先 journal
SubagentCompleted(树预算 usage + 子 stream provenance),再 activity
终态(ActivityCompleted{Usage} 结算预留 + 渲染 child report 为
user 消息 → 激活父 turn)。bgOutcome 加 subagent/usage 字段;
ActivityCancelled/Completed 带 usage。

**回执激活父 turn**:conversational park 的 awaitInput 已 select
bg.done——子结果落定即唤醒 park → 下 turn 消费(先回先处理),这是
"启动并行、消费结果"的自然归宿(v2 §3)。task 模式默认 onRunEnd=
cancel 会在收尾取消子 agent,故后台子 agent 归宿是 conversational
或 onRunEnd=await(记档)。

**kill**:task_kill 扩展 advertise 到 Agents 规格(task_id=handle,
走既有 cancel 注册表)——bash 任务与后台子 agent 共用同一取消原语
(DESIGN 的 cancel_child 语义由 task_kill 兑现,记档命名)。
spawn_agent schema 加 background 布尔。

测试:Router 路由的并行双子 agent e2e(2 spawn 同 turn1、2 bg-started、
2 SubagentCompleted + 2 ActivityCompleted、两 report 达模型、tasks
排空、两子 journal 存在),-race 绿。全量 check + stage 5/6 回归绿。
下一步:M3.2 用户直杀路径(daemon kill 命令 + CLI)+ M3 出口 QA-04/05。

## V2-M3.2 — 用户直杀路径 — DONE

Loop.Cancels <-chan string(带 handle 的取消通道,区别于整会话
interrupt);drainCancels 在 drive loop 安全点 + awaitInput select
arm 消费 → cancelHandle 查 bg.cancel 注册表触发;被杀子 agent 经
bg.done 结算为 canceled 回执,父下 turn 可见,其它子不受影响。
daemon:hostedRun.cancels chan + killHandle;kill 命令(查 runs →
killHandle);RunRequest.Cancels 接进 Loop.Cancels;CLI kill <sid>
<handle>。用户直杀与模型 task_kill 两条路径同触发 cancel 注册表。

测试:UserKill e2e(slow 子跑 sleep 30、kill by handle → slow 结算
canceled/error、fast completed 不受影响、SLOW_DONE 从未出现),
-race ×3 稳定;ParallelAndSettle 加 spare 步抗并发唤醒时序波动。
全量 check + stage 5/6 回归绿。下一步:M3 出口 QA-04/05 真实 API。

## V2-M3 出口(部分)— QA-04 真实 API GREEN + tool dedup 修复

**真实 bug 修复**:spec 显式列 spawn_agent + agents 触发自动追加 →
Gemini 收到重复 function 声明 → 400 INVALID_ARGUMENT。修:advertise
时对 extra 去重(against spec.Tools + extra 内部)。回归测试
TestNoDuplicateToolDeclaration。

**QA-04**:真实 ar+daemon+真实 Gemini,一个 turn 启动 3 个后台 worker
子 agent(background=true)→ 3 spawn_requested/3 subagent_completed/
3 子 journal、父跨 5 turn 消费结果、全在第一 turn 启动(非阻塞并行)。
PASS。

## V2-M3 里程碑出口 — 双闸门 GREEN

- **闸门 A(scripted 孪生)**:ParallelAndSettle(C3/C4)、UserKill(C5)、
  SteerChangesOrchestration(C6 模型 steer→cancel+spawn)、
  NoDuplicateToolDeclaration,-race 稳定。
- **闸门 B(真实 API)**:**QA-04**(真实 Gemini 一 turn 启 3 个后台子
  agent、3 完成、父跨 5 turn 消费,PASS)+ **QA-05**(用户 ar kill 直杀
  运行中子 agent by handle → canceled、其它存活、会话续跑,×2 PASS)。

**C3/C4/C5/C6 达成**——多 agent 编排核心真实跑通。记档:①assembly
把后台 spawn handle 的 tool-result 排在 steer user 消息之后(潜在
消息序疑问,QA-04 下真实模型处理正常,列 M3 review 观察项);②C6
真实 API 模型可靠性(自己 kill+spawn)为 best-effort,确定性由
scripted 孪生守,QA-05 真实 API 守确定性的用户直杀。

全量 check + race + stage 1–7 全 26 acceptance 回归绿。下一步:
M3 出口三视角对抗 review,然后 M4 多模态与工具面(→QA-03/07)。

## V2-M3 出口三视角对抗 review — 完成 + triage

三个独立 review agent(正确性/并发、安全、契约)对 635885a..e8b5a8b
全量审查。**已修复(commit a5a497d)**:

- **P1 正确性**:conversational 会话 mid-turn 被 ctx cancel(daemon
  重启/部署)时 abort() 写 RunEnded{canceled} → 永久不可 resume。
  修:abort 对 conversational + ctx cancel 不写 terminal(与 park
  路径同一崩溃纪律);Resume 靠 in-doubt 处置重入该 turn(LLM 幂等
  重发)。回归 TestConversationalMidTurnCancelResumes。
- **P1 正确性**:turn 预算累计计数 → 长会话到 40 turn 后静默吞输入
  (journal 了但永不应答,黑洞)。修:预算按 exchange 计
  (state.Run.LastInputTurn,自最近一次用户输入起算);预算耗尽且
  有 pending 输入时以 max_turns 可见结束。task 模式仍累计(v1 不变)。
  回归 BudgetPerExchange + TestDecideConversationalBudget。
- **P1 契约(wire bug)**:spawn_agent 工具描述教模型用不存在的
  cancel_child → unknown-tool(疑似 C6 真实 API best-effort 的根因
  之一)。修:描述改为 task_kill。
- **P1 契约**:用户 ar kill 无持久起源。修:cancelHandle 先 journal
  InputReceived{source:control}(journal-inputs-first,与 interrupt
  同纪律);fold 排除 control 进对话/授预算。回归 UserKill 断言。
- **P2 安全(defense-in-depth)**:CallID 进子目录名前校验
  ^[A-Za-z0-9_-]+$,路径语法解析为 model 可见错误。回归
  TestSpawnMalformedCallID。

**文档对账(本 commit)**:DESIGN §9.1 新增"M3 实现状态注记"(工具名
对照、inbox 字面统一度、三路径并存、idle-interrupt 语义)——目标态
与实现态的偏差不再只活在 PROGRESS;QA.md base.yaml/QA-05 更正为
实现工具名与修正后的通过标准。

**记档不修(P2/观察项,收口时再判)**:①kill-vs-complete 竞态下
reason 可能标 canceled(实际完成,payload 仍真);②settleBackground
的 out.err 分支不可达;③killHandle 满 buffer 返 false 与
signalInterrupt 语义不一致;④assembly 把后台 handle tool-result
排在 steer 消息后(真实模型处理正常);⑤child_result 字面进 inbox
与"一套机制收敛"列收口决策。

验证:go test 全绿(-race 含 bgspawn);v1 acceptance 26 场景全
PASS;QA-05 真实 API 复跑 PASS(kill 路径改动后)。
**M3 正式关闭。下一步:M4.1 消息 parts(多模态)。**

## V2-M4 图片输入 — QA-07 真实 API GREEN(C9 达成)

M4.1/M4.2 全链:CLI send --image(嗅探 media type)→ daemon base64
wire(命令行缓冲 32MB)→ inbox protocol.UserInput → journalInput 先
CAS Put 再落 InputReceived{images:[{ref,media_type}]}(blob-before-
event)→ fold ref-only image part → 组装时 inflateBlobs(copy-on-
write,fold 永不含字节)→ gemini inline_data / anthropic base64 block。

**QA-07**(真实 Gemini vision):build-error.png 截图三要素
(command.go/1234/EnableTraverseRunHooks2)全说对;journal 的
input_received 行 432 字节 ref-only、CAS blob 落盘;后续纯文本 turn
凭上下文检索到标识符(连续性)。PASS。

学到:QA 等待原语从"数 assistant_message"改为"数 waiting_entered
(idle park)"——一个 exchange 可能跨多个 assistant 步(模型先跑工具
再作答),数消息会提前放行(qa_wait_idle 进 lib.sh)。

下一步:M4.3 write_file 工具 + 长贴折叠 → QA-03。

## V2-M4.3 — write_file + 长贴折叠 — DONE;**M4 里程碑双闸门 GREEN**

**write_file**:一等核心工具(class=edit,建新文件/整文件覆盖不再借
edit_file 空 old 特例或 bash heredoc);registry 数据驱动加 defs JSON
+ Execute case。TestWriteFile(建/覆盖/边界逃逸拒绝/缺 content 报错/
空 content 合法)。

**长贴折叠**:journalInput 对 >10KB 文本(redaction 之后)CAS Put 全
文、journal 存 512B head + 折叠注记 + Files:[{ref,text/plain}];
event.AttachmentRef 统一 Images/Files;fold 出 PartFile;wire 层
text/* file part 两家都渲染为文本块(可移植),二进制走 inline_data/
image block。TestLongPasteFoldsToFilePart。

- **闸门 A**:C9 e2e 孪生 + 长贴折叠 + write_file + wire 映射单测,
  -race 绿;全量 go test 绿。
- **闸门 B**:**QA-07**(vision 三要素 + ref-not-bytes + 上下文连续,
  PASS)+ **QA-03**(真实 Gemini 修注入 bug:只改 calc.go 不改测试、
  自跑测试变绿;QA_NOTES.md 经 write_file 工具落盘。PASS)。

**C9 达成,核心工具 9 达成**。学到:QA-03 的 qa_inject 是 untracked
——断言用内容哈希对比,git diff 看不见注入文件。acceptance 1-7 全 26
场景回归绿。下一步:M5 恢复审计(crash 矩阵)→ QA-08。

## V2-M5 恢复审计 — QA-08 真实 API GREEN(C10 达成);**M5 双闸门 GREEN**

**QA-08 crash 矩阵**(真实 Gemini,单一 session 三连杀):
(a) idle park × kill -9 → 重启后 `ar send` 即复活,崩溃前上下文
(暗号)原样接续;(b) bash 在飞 × kill -9 → interrupted-by-crash
渲染落 journal、runtime 不重跑(结构断言:每次 qa_slow 启动都与
assistant tool_call 1:1 配对——模型看到 crash 结果后自行决定重跑属
合法 agency,红线只约束 runtime);(c) 2 个子 agent 在飞 × kill -9 →
重启后两张 subagent_completed 回执补投,会话继续;全程恰一个
run_ended(close 时)。

学到:①崩溃消息"may or may not have happened"会让模型主动重跑求证
——QA 断言必须只钉 runtime 红线(启动/模型调用 1:1),不能钉模型行为;
②daemon 就绪探针用 `ar sessions list`(裸 `sessions` 是 usage 错误);
③close 前先等 idle park,复活后的收尾 turn 有真实 API 延迟。

记档:daemon kill -9 会孤儿化在飞 bash 的子进程(进程组随 daemon 死
而失管;sleep 类自然退出,长驻型需重启后 pgid 清扫——列收口观察项)。

**M1–M5 全部关闭。剩收口:QA-09 压轴串联 + ps/events 观察面 +
文档同步(CORE/GAPS)+ 出口三视角 review。**
