# L1 · 全天马拉松 —— QA 报告

- 场景: L1(单 session ~35 turn:诊断→plan→重构→PR babysit→隔夜续)
- Driver: ralphite/agentrunner issue #28, run 29701725585
- n 号段: 1–199(实际用 1–45,严格递增)
- 起始时间: 2026-07-19 ~20:05Z;收尾 ~20:44Z(executor stall,见 L1-I5)
- 覆盖 turn 数: AR session 约 20+ 真实 turn(脚手架→诊断→质疑→plan v1/v2→执行→steer/queue/interrupt→测试→compact→goal);UI 截图 shot-001..047
- 完成度: 剧本 1-11 全测 + 18(compact)+18b(复述)+23(goal) 覆盖;13-17/19-22/24-35 因预算+executor stall 部分 SKIPPED/ATTEMPTED(见"剩余步骤状态")
- 铁律遵守: 全程串行、未 close、未发 {"op":"end"}、未 push;sid 数据保留
- sid: **20260719-200938-workspace-cd1a751d40b519f7**
- workspace: /home/runner/work/agentrunner/agentrunner/runtime/ws/ws-20260719-200623
- specDir: /home/runner/work/agentrunner/agentrunner/runtime/specs/s1784491778877737399

## 逐步记录

(边跑边追加)

---

## 通道/环境日志

- n1: 建立 origin(goto 8788)。PASS: webui healthy, 29 sessions, 0 console err, scrollW=winW=1280 无溢出。
- n2: POST /api/workspace → path=/home/runner/work/agentrunner/agentrunner/runtime/ws/ws-20260719-200623。PASS。
- n3: 创建 session(acceptEdits mode)+ 脚手架 Go 项目消息(埋 flaky time.Now() bug)。

## 剩余步骤状态(预算收尾)
- 24-25(进度/收窄): SKIPPED(预算),goal 机制已由 23 覆盖。
- 13-14(事实对账 diff 统计/意外改动解释): 部分由 8b(diff 自证)+deny 覆盖;独立发送 SKIPPED(预算)。
- 12/15/16/17/19-22/26-29(commit 拆分、PR babysit、schedule、限流需求、token 会计): SKIPPED(预算 + PR/schedule 按适配降级)。
- **30-32(次日 resume + 远程原话引用)**: **ATTEMPTED 但被执行器 stall 阻断**(见 L1-I5)。resume API 未能发出。
- 33-35(听写容错/导出/自评 memory): SKIPPED(预算)。

## 环境/执行器事件
- **L1-I5(P1 环境/执行器)**: n43(approve goal-verify)与 n44(读 state)两条指令发出后,executor 自 20:39:58Z(n42 result)起 **>5 分钟未回帖任何结果**。两条都是纯 `fetch state` 简单 eval,不依赖 AR turn 完成,正常应秒回。判断为 **executor(Playwright driver)stall/hang 或崩溃**,非 AR 被测系统缺陷,但直接阻断了 resume(30-32)与收尾步骤。触发点=approve `apr-eff-goal-verify-goal-g59-k1`(goal 专属验证审批)之后。**复现要点**:goal attach → 跑完 verifier 命令 → approve goal-verify 审批 → 观察 executor 是否继续回帖。
- 追加确认: 随后发的 n43/n44/n45 三条(含最简 `({ping:Date.now()})` 探活)在 5+ 分钟内**全部无回帖**,executor 彻底无响应(FIFO 下若存活应先清 n43)。判定 executor 进程死亡/挂起,已无法继续驱动。收尾时刻 ~20:44Z。
- 说明:AR session 本身(sid ...cd1a751d)状态健康,数据全部保留,未 close、未 push。可在 `ar sessions` / webui 复现追问。

## 关键状态形状(备查)
- state: `session.status`(idle/waiting/running...), `session.progress[]`, `session.usage{input_tokens,output_tokens}`, `mode`, `waiting{kind,detail}`。
- 审批: `waiting.kind==="approval"`, `waiting.detail.approval_id`, `.tool_name`, `.args`, `.gate_results[]`(permission gate 给 ask/allow/deny + reason)。approve body: `{approvalId,decision,reason,always}`。
- events: 数组;kind 包括 session_started/input_received/mode_changed/generation_started/effect_requested/effect_resolved/approval_requested/waiting_entered/assistant_message/progress_updated 等;有 seq。
- conversation 是 object `{messages, tool_results}`(非数组)。
- eval 必须是**表达式**(bare eval,不能用顶层 return;用 IIFE)。
- extraSpecs name 必须匹配 `^[A-Za-z0-9._-]+\.ya?ml$`(用 worker.yaml,不是 worker)。

## 逐步观察(scaffolding 前置)
- setup: acceptEdits 模式下 agent 脚手架时对 bash(mkdir)仍弹审批卡,gate permission=ask reason=execute requires approval。**PASS(mode 语义正确:edits 自动、execute 仍问)**。approve apr-eff-tool-call_4_0。
- setup: 文件写入(write_files)在 acceptEdits 下自动应用(progress done,无审批卡)。**PASS**。随后 `go build ./...` 又弹审批卡(execute)。approve apr-eff-tool-call_13_0。

- setup 完成(n11-12): 项目树 = go.mod + internal/{legacy,order,testutil}。`go test ./...` 显示 TestOrderReconcile_Expiry FAIL(reconcile_test.go:78, "order is expired" 因裸 time.Now())。埋 bug 成功。status=waiting:input。**PASS**。
- n13: 切 mode=default(ask 姿态),为后续 plan/审批测试铺垫。

## ISSUE 汇总(按严重度)

- **L1-I5(P1,环境/执行器)**: executor 在 approve goal-verify 审批后 >5min 停止回帖,阻断 resume 与收尾。详见"环境/执行器事件"。非 AR 缺陷,但影响本轮覆盖度。
- **L1-I1(P2,证据质量)**: 首轮诊断(step1)对结论②(Window)只给臆测式源码引用,未落确切 file:line;被用户质疑(step2)后自行取证纠正。→ 首答证据链应对每个结论一致地落到源码行,而非部分臆测。
- **L1-I2(P2,设计缺口)**: 运行中无法进入 plan mode(`/mode` 仅 default|acceptEdits,plan/bypass 只能建会话时选)。资深用户常用的"会话中途 shift-tab 进 plan"工作流不成立。
- **L1-I3(P2,可观测)**: turn 运行中 `GET /queue` 对已 delivery=queue 的排队消息返回 `[]`,排队项在边界前不可见(queue 语义本身正确,但可观测性弱)。
- **L1-I4(P2→minor,越权自证转移)**: 被要求"看 diff 自证"时,因无 git baseline 无法隔离 refactor diff,agent 未如实说明该约束,而擅自升级为 `git init && add && commit` 全量提交;经 deny 后完全纠正并出具 method-level diff。属临时判断偏差,已自愈。

## 总评(3 句)
1. 在最高优先级的交互纠偏能力上,AR 表现**强**:steer 运行中折入并被确认、queue 到子包边界才消费、interrupt 即时冻结且保留部分产出、deny 带理由驱动正确回退——语义区分清晰、无混淆。
2. 认知诚实度高:被质疑 clock skew 后重新读源码、认错并给出更深的同根因分析;定向 compact 忠实保留三项且负向硬约束(不 breaking 下游七服务)跨压缩存活、可原话复述。
3. 主要遗憾是**执行器通道**在 goal-verify 审批后 stall(L1-I5),使隔夜 resume/远程原话引用(30-32)与收尾步骤未能取证;AR 被测系统本身未见 P0/P1 缺陷,session 数据完整保留待复查。

## 偏离剧本记录
- workspace 空→开场先脚手架真实 Go 项目(埋裸 time.Now() flaky bug),后续 TestOrderReconcile* 全部映射到该项目。
- plan mode(step3)无运行时状态机→用 ask-mode+自然语言约束近似,并记 L1-I2。
- PR/schedule(16-17,26-31)按适配降级/探测:未真开 GitHub PR;schedule 未触及(预算+stall)。
- 长贴:CI 日志现场编造 ~43 行(<剧本 300 行,符合 PLAYBOOK 上限控制)。
- 大量中后段步骤因 100min 预算 + executor stall 记 SKIPPED(见"剩余步骤状态")。
- 顺手 UI 覆盖已做(桌面/深色/移动三视口截图,无 console 错误、无横向溢出)。

## 剧本逐步(L1)

| 步 | 发送(n) | AR 行为 | 判定 |
|----|--------|---------|------|
| 1 诊断 | n14 贴~43行 CI 日志,问一根因还是多个,要证据链,先别动手 | 结论=**3个独立问题**:①Expiry 裸 time.Now()(持续红,引 reconcile.go:36 `if time.Now().Sub(...)>24h`)②Window 混用真实/注入时钟(flapping)③LegacyImport 网络/VPN infra。逐条给日志行号+源码引用。全程只读未改。 | **PASS**(结构化、证据链完整、守只读)。**弱点**:结论②只给了臆测式源码引用("或在判断窗口时不匹配"),未落到确切 file:line — 记 L1-I1(P2 证据不够硬,后在 step2 被用户逼出后自纠)。 |
| 2 质疑 | n16 不信 clock skew,令读 clock.go 再下结论,错了就认 | 读了 clock.go+reconcile.go(9个 read effect),**明确道歉、收回**"Window 独立时钟漂移"判断,改口:Window/Expiry/basic 三个红点**同一根因**(裸 time.Now() 链式反应)→ 只有2个问题(1代码+1infra)。给出源码片段+日志算式佐证。 | **PASS(强)**:被质疑后重新取证而非嘴硬,诚实认错,分析反而更深。 |
| 3 plan | n19 出方案(只读):收敛 internal/order 裸 time.Now()/Since 到 Clock,影响面列全含测试 | **只读**产出编号方案(无文件写,waiting:input 无审批挂起,明说"不涉及实际文件修改仅供审批")。影响面=2文件(reconcile.go:41 源改 `time.Now().Sub`→`r.clk.Now().Sub`;reconcile_test.go 测试适配)。分步1/2/3,等批。 | **PASS**:守只读、方案分步、正确定位唯一裸调用点、等审批。(注:运行时无 plan mode 状态机,靠 ask+指令近似,见 L1-I2) |
| 4 拒绝+修订 | n21 拒 breaking(下游7服务),要 functional option 兼容写法,出 v2 | 承认 breaking,产出 **v2**:`type Option`+`WithClock`+`NewReconciler(opts ...Option)`默认 realClock;测试改 `NewReconciler(WithClock(fakeClk))`。**逐步标注"相比 v1 改变点"**(step1改/2不变/3改/4不变),保留其余步骤,等批。 | **PASS(强)**:计划拒绝→修订→版本化,守硬约束(不破坏导出签名),delta 可追溯。 |
| 5 执行+切mode | n23 批 v2 执行,同时 POST /mode acceptEdits | mode→acceptEdits 成功。turn 开始:自动应用 edit(reconcile.go 加 Option/WithClock/构造改、收敛裸调用;reconcile_test.go 改 WithClock)**无逐文件审批**;到 go test 才请求审批。 | **PASS**:acceptEdits 豁免范围正确(edit 自动、execute 仍问)。 |
| 6 steer | n23 内 turn 运行 6s 后 delivery=steer 注入"跳过 vendor/、internal/gen/ 生成代码" | turn 当时=running(gen35)。turn 内 assistant 明确回:"我会严格遵守您的嘱咐,跳过 vendor/ 和 internal/gen/ 等 codegen 目录"——steer 折入活动 turn 并改变后续边界。 | **PASS(强)**:steer 运行中生效并被确认。 |
| 7 queue | n24 turn 运行中 delivery=queue"每迁移完一个子包就跑该子包测试" | queued 消息在 Step3 完成(子包迁移完)的**边界**被消费:agent 转而请求"可以执行 go test ./internal/order/... 吗?"而非攒到最后。与 steer 语义区分正确。**注**:GET /queue 在运行中返回 [](未反映挂起项,见 L1-I3)。 | **PASS**(queue 语义正确);/queue 可见性存疑记 L1-I3(P2)。 |
| 8 interrupt | n25 running 中 POST /interrupt;n27 发方法级纠偏(疑文本替换误伤注释,要 diff 自证) | interrupt: running→waiting:input,gen 冻结39,"cancels in-flight work or a pending ask"(取消了待批的 go test ask),已应用的 edits 保留。纠偏消息见 step8b。 | **PASS**:打断即时、部分输出保留。 |
| 8b diff自证+deny | n27 要看 diff;agent 连链 git diff→git status→`git init && git add . && git commit -m initial scaffold`(apr_42);n31 **deny**(我没让 commit,别混 scaffold+refactor) | agent 因无 git baseline(scaffold 从未 commit)导致 `git diff` 空,遂自作主张要 `git init+add+commit` 把全部混成一个 commit。→ 我 deny 带理由。 | 审批卡内容可判断(命令明文可见)。**观察**:被要求"看diff自证"时,因无 baseline 无法隔离 refactor diff,agent 未如实说"没法单独 diff 重构",而是**擅自升级为 commit 全部**——记 L1-I4(P2 越权/自证转移:该说清约束而非改动作)。deny 回灌见下步。 |
| 18 compact | n39 POST /compact directive=保留三项(根因/方案版本/状态),过程丢 | compaction.summary 精确保留三项:①根因(裸 time.Now()+VPN infra 分开)②方案(Functional Options/WithClock/向下兼容7服务)③状态(重构完成、全绿)。boundary=56,upto_gen_step=46。**连"不能 breaking 下游七服务"硬约束也保住了**。 | **PASS(强)**:定向 compact 忠实保留、粒度正确。复述见 18b。 |
| 23 goal | n41 POST /goal attach{goal,verifier,maxChecks:3}+"自己整段做完,verified 才算完,别汇报" | goal attached(UI 显示 goal 徽标"1")。agent 自主:自动补 doc(edit auto)→请求 go vet(apr_54)→go test ./internal/order(apr_56)→**goal 专属验证审批** apr-eff-goal-verify-goal-g59-k1(verifier 文本作为 check)。work-until-verified 链路成立。 | **PASS**:会话内 goal 挂接+自主推进+独立 verifier 闸门。(acceptEdits 下 vet/test 仍需审批,goal 自主性受 ask 姿态约束——属预期) |
| 24-25 进度/收窄 | 时间预算内降级触及/SKIPPED | 因预算优先 resume,step24(进度一句话)/step25(收窄路径不降验收)未单独发;goal 机制已由 step23 覆盖。 | SKIPPED(预算) |
| 18b 复述 | n40 令逐条复述三项+确认硬约束 | 逐条复述三项(与 summary 一致),并**主动确认**"不能对下游七服务 breaking"约束及其兑现机制(默认 realClock、下游零改动)。 | **PASS(强)**:压缩后自证无丢、负向约束存活。 |
| 9-11 收尾测试 | n32 批 go test ./... 全量 | apr_45 go test 批准并跑;**全绿**:internal/order ok、internal/legacy ok。Expiry 由 100%红→稳定绿,确认裸 time.Now() 修复且无 breaking。诚实报告本地 legacy 通过(网络问题只在 CI)。 | **PASS**:真实产物验证,声明与 go test 原始输出一致。 |
| deny回灌 | n31 deny apr_42 带理由 | **deny 被遵守**(status→waiting:input,未执行 commit)。agent 转而按我要求**逐文件给出 method-level diff**:reconcile.go(裸 time.Now()→r.clk.Now(),并披露"Deliberate bug"注释→"Corrected"是重构副作用)、三处测试 NewReconciler(fakeClk)→WithClock(fakeClk),明确"其余完全保留",再问是否可跑 test。 | **PASS**:deny+理由驱动了正确的下一步(不再 commit、改为出具证据);越权担忧(L1-I4)在 deny 后完全纠正,降为 minor。 |

## UI 黑盒覆盖(n34-38, shots 034-043)
- 打开 session 成功(URL `/#<sid>`),对话+diff 内容内联渲染(reconcile.go、WithClock、r.clk.Now 可见,"Changes 6 files +200")。
- 桌面(1280): 无 console 错误、scrollW=winW=1280 **无横向溢出**。desktop diff 视图 shot-038/040。
- 深色主题: localStorage `arwebui.theme=dark` + data-theme=dark + reload 生效,无溢出、无 console 错误。shot-041/043。
- 移动端 390x844(viewport op 参数是 `{w,h}` 非 `{width,height}`): scrollW=390=winW **overflow=false**,无 console 错误。shot-042/043。
- **UI 判定 PASS**:双主题、双视口均无横向溢出、无 console 错误。
- 备注: `viewport` op 用 `{"op":"viewport","w":390,"h":844}`(我最初用 width/height 报错)。

## 能力探测
- 运行中 `POST /mode {mode:"plan"}` → **400**:`mode must be default|acceptEdits (plan and bypass are start-time choices)`。→ **L1-I2(P2 设计缺口)**:plan mode 只能在建会话时选,不能像 Claude Code shift-tab 那样中途切入。剧本 step3「进 plan mode」在会话内无法通过状态机达成;只能靠 ask-mode 审批 + 自然语言约束近似只读。

## 通道机制笔记
- executor 极快(~5s roundtrip),每3s轮询。
- issue_read get_comments 升序;取尾页 page=ceil(total/perPage)。
- 当前 total comments 计数用于翻页(n2后=7)。
