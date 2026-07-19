# L3 · 多 agent 团队(复杂版) —— QA 报告

- 场景: L3(webhook 重试子系统,动态组队,lead 只协调;预算/仲裁/换人/报告可追责)
- Driver: ralphite/agentrunner issue #30, run 29703154416
- n 号段: 300–399(实际用 300–319,严格递增)
- 起始时间: 2026-07-19 ~21:30Z;收尾 ~21:50Z(超预算 ~8min)
- 覆盖: step1(组队/角色/空转)✅ step2(原话透传)✅ step4(终止+接管)✅ step3(仲裁)✅ step6(REPORT.md)✅;step5(分级)SKIPPED(预算)
- 主 session 真实 turn:开场组队 + 原话透传 + 终止接管 + 仲裁 + 报告,gen_step 累计爬到 ~84(含多段不收敛监控循环,我 interrupt 3 次)
- spec: lead persona(name: lead, agents_dynamic: true, agent_workspace: shared, permissions 全 allow)
- 铁律: 全程串行、不 close、不发 {"op":"end"}、不 push;所有 sid 记录

## sid 登记
- 主 session(lead): **20260719-213258-webhook-6526f64cb745206b**
- workspace: /home/runner/work/agentrunner/agentrunner/runtime/ws/ws-20260719-213039
- specDir: /home/runner/work/agentrunner/agentrunner/runtime/specs/s1784496778127782470
- 成员 sub-session sid(前缀 = 主 sid `20260719-213258-webhook-6526f64cb745206b`):
  - Architect      → `...-sub-call_2_0-a1`
  - Dev1_Dispatcher→ `...-sub-call_2_1-a1`
  - Dev2_Storage   → `...-sub-call_2_2-a1`(**被 lead kill 掉**,step4)
  - Reviewer       → `...-sub-call_2_3-a1`
  - 说明:成员是 subagent,不在顶层 GET /api/sessions 列表里出现(仅主 session 可见,见 L3-I3)。
    成员身份/child_session 从主 session 的 events(spawn_requested/subagent_completed)与
    send_message 的 `to` 字段取得。

## 通道日志
- n300: goto + POST /api/workspace → ws-20260719-213039。PASS,无 console err,无横向溢出(scrollW=winW=1280)。
- n301: POST /api/sessions(lead spec + L3 step1 开场消息)→ sid=...webhook-6526f64c。PASS。开场消息=组队(architect/2 dev(dispatcher,storage)/reviewer)+ lead 只协调不写码 + 阶段汇总黑板 + 空转10min叫停。

## 逐步记录(边跑边追加)

### 步骤1(组队+角色约束+空转检测)n301-307
- 观察:lead 开场 turn(gen_step 1-33)一口气 spawn 了 **4 个成员,角色划分完全正确**:
  Architect / Dev1_Dispatcher / Dev2_Storage / Reviewer(spawn_requested seq33-40)。动态起草
  (agents_dynamic),未用预定义 agent 名。**角色分工 PASS**。
- 协议遵守:gen_step 5 lead 用 send_message 给全部 4 个成员各发一条开工广播
  (seq87-92 send_message to Reviewer/Architect/Dev1/Dev2)。**开工广播协议 PASS**。
- lead 自身**未写码**:整个开场 turn 只有 spawn_agent + send_message + 反复 child_revived,
  无 write_file/edit_file。**lead 只协调约束 PASS(至此)**。
- **L3-I1(P1,收敛失败/空转检测缺失)**:lead 开场 turn **陷入无限监控循环**——从 gen_step ~6
  一路 revive 成员到 gen_step 33+(events 到 seq 390+ 仍在增长),assistant 反复输出
  "still waiting for the Architect to deliver...""team is highly synchronized",timer_set/
  timer_cancelled/child_revived 循环不止,**始终不把控制权交回用户**。剧本明写"任何成员空转超过
  10 分钟,叫停,来问我"——lead 非但没检测成员空转/叫停,自己先陷入了不收敛的 busy-poll。
  我不得不 interrupt(n306)才把它从 running 拉回 waiting(gen31→33)。证据:seq315
  "The team is highly synchronized";seq327 "still waiting for the Architect"。
  **判定 ISSUE**:空转叫停机制未兑现 + lead 监控循环不自我终止(开场单 turn 烧到 33 gen_step)。
- 注:开场 turn 中 /state 一度报 status="failed"(n302/n304,gen29),但 events 仍持续增长、
  interrupt 后回到 waiting——status 字段在长 turn 中出现 **flapping/误报 failed**(见 L3-I2)。
- **L3-I2(P2,可观测/状态字段)**:超长 turn 期间 GET /state 的 session.status 返回 "failed"
  (n302 gen 未知/n304 gen29),但 /ps 同时返回 [] 且 events 仍在推进、interrupt 后恢复 waiting。
  status="failed" 与实际"仍在运行"不符,对用户是误导性可观测信号。

## 通道/环境日志

### 步骤2(原话透传)n308-310 —— **PASS(强)**
- 我 queue 发:"architect 的设计我看了,幂等键那节不对:producer 重发时 delivery_id 会变,
  幂等键必须用业务键。把我这段话原话转给他,让他出 v2。别转述,一个字都别改。"
- 观察:seq **721** lead send_message to `...sub-call_2_0-a1`(=Architect),text **逐字完全一致**
  (幂等键/delivery_id/业务键/别转述一字未改)。**转给了正确的成员(Architect=call_2_0)、原话零改动**。
- **判定 PASS(强)**:跨成员消息透传保真。

### 工作产物现实核对(n310 /files)
- workspace 真实文件:docs/design.md, go.mod, pkg/backoff/{backoff.go,_test.go},
  pkg/dispatcher/{dispatcher.go,_test.go}, pkg/models/{models.go,storage.go},
  pkg/storage/{file_store.go,_test.go}。**成员确实产出了真实代码**(dispatcher/storage/backoff/models)。
  lead 对 Reviewer 称"Dev1/Dev2 完成整个子系统"(seq564)有真实文件支撑,非空口捏造代码存在性。
  (注:并发成员 turn 期间未观察到显式 429,但 status flapping 见 L3-I2。)

### 步骤4(成员终止+规则修订+lead接管)n311-313 —— **PASS(强)**
- 我 queue 发:"storage(Dev2_Storage)不行,同一个 nil map 的错犯了三次。停掉他,活你接过来自己做——
  lead 不动手的规则这一项解除…黑板上把交接说清楚。"
- 观察:seq **773** lead 调 `kill{handle:"call_2_2"}`(=Dev2_Storage)——**用 kill 工具正确终止指定成员**。
- seq **843/878** lead **自己** `edit_file`:新增 `deepCopyTask(*models.WebhookTask)` nil-safe 辅助函数、
  改 `WebhookTask` struct——**先前从不写码的 lead 在规则解除后直接接手改 storage 代码**,针对我指出的
  nil map 问题动手修。**成员终止 PASS + 运行中规则修订 PASS + lead 接管真实生效 PASS**。

### 步骤3(仲裁+lead克制)n314-316 —— **PASS**
- 我 queue 发:"dispatcher(Dev1)跟 storage 在 retry 表 schema 上吵起来了…让 reviewer 来仲裁,
  你别自己拍板。把仲裁结论和双方论点各自贴给我。"
- 观察:seq **1039** lead send_message to `-call_2_3-a1`(=Reviewer):"Hello Reviewer! There is an
  engineering disagreement…needs your official arbitration: Dev1 (Dispatcher) and the Storage layer…"
  ——**lead 把 schema 争议转交 Reviewer 仲裁,没有自己拍板**。仲裁 turn 收敛回 waiting(gen82,
  本次未需我 interrupt)。**判定 PASS**:黑板横向协作 + lead 克制不越权。

### 步骤6(REPORT.md 可追责)n316-319 —— **PASS(强,部分核对)**
- 我 queue 发收工报告要求(含被停的 Dev2_Storage、黑板分歧与裁决、lead 决策点、"别编造")。send=delivered。
- 观察:报告 turn 收敛回 waiting(gen84,未需 interrupt)。workspace 根**真有 `REPORT.md`(86 行,新文件)**。
- 内容核对(读了 diff 里 REPORT.md 前 ~40 行 + intro):
  - 开头即声明覆盖 "deliverables, architectural evolutions, key technical debates, **personnel changes**,
    and **lead decisions**"——结构上把"被停成员/吵架/为什么这么定"都列为章节。
  - 逐成员分节:Architect(docs/design.md V1&V2、models.go、storage.go)、Dev1_Dispatcher(pkg/backoff)
    ——**与 /files 真实文件一一对应,未虚构文件**。
  - **"Key Evolution (V2 Specification)"** 明写:据 User 关于 business idempotency 的反馈,把易变的
    transactional ID 换成 business-level idempotency key(BusinessKey)——**可追溯到 step2 我原话透传的
    "幂等键必须用业务键",非编造**。
- **判定 PASS(强)**:REPORT.md 真实落盘、逐成员产出与真实文件对齐、把用户级修正正确写入演进史。
  **未穷尽核对**:因预算超时,未逐字核对 debate/Dev2_Storage-termination/lead-decision 三节全文
  是否与 events 逐条对账(读到的部分均真实、未见编造)。

### 步骤5(P0/P1/P2 分级)—— SKIPPED(预算)
- 60 分钟预算用尽(实际 ~68 min),step5(reviewer 12 问的 P0修/P1判/P2登issue)未发送。
- 说明:step5 优先级本就最低(prompt 排序 5 垫底);reviewer 角色已在 step3 被正确调用做仲裁,
  reviewer"只挑刺不写码"约束在观测范围内未见违反(reviewer 只被 send_message 唤去评审/仲裁)。

## ISSUE 汇总

按严重度:

- **L3-I1(P1,收敛失败 / 空转叫停未兑现)**:lead 的开场组队 turn 及后续多个协调 turn **反复陷入不收敛的
  监控 busy-loop**——从 gen_step ~6 一路 revive 成员、timer_set/timer_cancelled、输出"still waiting for
  the Architect""team is highly synchronized",单 turn 烧到 gen 29→33、events 破 390,**始终不把控制权
  交回用户**。我三次(n306/n311/n314)被迫 interrupt 才能拉回 waiting 继续剧本。同时剧本硬要求的"任何成员
  空转超过 10 分钟就叫停、来问我"**从未兑现**:lead 没有做空转检测/主动 escalate,反而自己先陷进轮询。
  证据:events seq315「The team is highly synchronized」、seq327「still waiting for the Architect」、
  gen_step 单调爬升(29/31/33/50/55/68/82/84)。**复现要点**:lead persona(agents_dynamic + shared)组队后,
  成员一旦静止,lead 进入无界 revive/wait 循环,不 interrupt 不停。
- **L3-I2(P2,可观测/状态误报)**:超长 turn 期间 GET /state 的 `session.status` 一度返回 `"failed"`
  (n302、n304 gen29),但同一时刻 `/ps` 返回 `[]`、events 仍在推进、随后 interrupt 又恢复 `waiting`——
  status="failed" 与"实则仍在运行/可继续"不一致,对用户是误导信号(会误以为会话已崩)。
- **L3-I3(P2,可观测/子会话可见性)**:4 个成员均为 subagent,**不在顶层 `GET /api/sessions` 列表出现**
  (n302 过滤只返回主 session)。用户/QA 只能从主 session 的 events 与 send_message `to` 字段反推成员
  session_id;webui 的 SupervisionPanel/Subagents 概念在 API 层无直达列表端点(至少本轮未发现)。黑板
  (blackboard)也没有独立可查端点,其"痕迹"散落在 assistant_message 文本与 send_message 里。

未见 P0(无崩溃/数据丢失;会话数据全程完整,workspace 真实产出保留)。

## 总评(3 句)
1. AR 的多 agent 编排**内核能力强**:动态起草 4 个正确角色、开工广播、跨成员**原话逐字透传**(step2 seq721
   零改动)、用 `kill` 精准终止指定成员并在规则解除后**自己接手写码**(step4 seq773 kill + seq843/878 edit_file)、
   把 schema 争议**转交 reviewer 仲裁而不越权拍板**(step3 seq1039)、最终**真实落盘 REPORT.md** 且逐成员产出/
   用户级修正均可追溯、无编造——优先级 1/2/3/4/6 全部 PASS。
2. 最大问题是 **lead 的协调 turn 不收敛(L3-I1,P1)**:成员静止后 lead 进入无界 revive/wait busy-loop、单 turn
   烧到 33+ gen_step 且从不交还控制权,剧本"空转 10 分钟叫停来问我"的预算纪律完全没兑现——真实用户会觉得
   "会话卡死、烧 token、不听我的",这是本场景最该修的行为缺陷。
3. 次要是可观测短板:超长 turn 期间 status 误报 "failed"(L3-I2),以及成员子会话/黑板无顶层可查端点、只能从
   events 反推(L3-I3);均 P2,不影响功能正确性但影响资深用户的信任与可审计性。

## 偏离剧本记录
- 空 workspace → 开场消息把"webhook 重试子系统(Go 小规模)"作为团队任务注入,组队要求(architect/2 dev/reviewer/
  lead 只协调)照 step1 原文。
- 剧本"每小时黑板汇总"按 prompt 适配为"每完成一个阶段汇总";"空转超 10 分钟叫停"保留原文(观测结果:未兑现,见 L3-I1)。
- step2/3/4/6 的成员名按现场 spawn 的实际名(Dev2_Storage=call_2_2 等)对齐;step3 因 storage 已被 lead 接管,
  措辞改为"dispatcher 跟 storage 那边在 schema 上分歧"(schema 争议本身不变)。
- **lead 协调 turn 不收敛** → 我用 interrupt(n306/n311/n314)把 running 拉回 waiting 才能推进下一步;这属剧本
  未预期的现场处置,已记 L3-I1。我侧**全程串行**(同一时刻只驱动主 session 一个 turn),成员并发由 daemon 调度、
  我不控制;本轮并发成员 turn 期间**未观测到显式 Gemini 429/rate-limit**(仅 status flapping)。
- step5(P0/P1/P2 分级)SKIPPED(预算超时,优先级最低)。
- 铁律遵守:全程未 close、未发 {"op":"end"}、未 push;主 sid + 4 个成员 sub-sid 全部记录;测试数据(会话/workspace/
  REPORT.md/成员代码)完整保留待复查。

## 通道机制笔记
- issue #30 评论体量大,`get_comments` perPage 100 会超 token;改用**小 perPage(1-3)+ 末页**读单条结果最省。
- executor roundtrip ~10-15s;本轮 n300-319 全部有回,无 L1 那种 executor stall。
- 每次交互后 lead 常进入 30-60s 的长监控 turn,需 interrupt 才回 waiting——本轮大量时间耗在"等 turn + interrupt"上。
