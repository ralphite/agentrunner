# DESIGN.md 修订建议清单

> **⚠️ 已处理归档**：本清单基于旧版 DESIGN.md（`2bae06e` 之前）。
> 过时项与有效项的鉴定见 [CAPABILITY-REVIEW.md](CAPABILITY-REVIEW.md)
> 顶部注记；有效项已并入 DESIGN.md。本文件仅作历史留档。

> 来源：对照 Claude Code / Codex 166 个功能点的能力审查
> （[CAPABILITY-REVIEW.md](CAPABILITY-REVIEW.md)，逐项明细见
> [CAPABILITY-REVIEW-DETAILS.md](CAPABILITY-REVIEW-DETAILS.md)）。
> 用法：在后续 session 里按本清单逐条更新 DESIGN.md。每条给出
> **改哪里 / 为什么 / 改成什么**。A 组是结构性修订（建议全部采纳后再动手写
> 代码）；B 组是一句话契约补丁；C 组是备忘（roadmap 对应阶段再展开）。
> 采纳时可在条目前打勾追踪。

---

## A. 结构性修订（7 条）

### A1. 决策 #7 / L1「Checkpoint 与 workspace」：per-turn git commit 改为 shadow git repo

- [ ] **改哪里**：L1「Checkpoint 与 workspace」一节 +「已定决策」表 #7、#3。
- **为什么**：workspace 通常就是用户的 git repo，checkpoint commit 落在用户
  分支上会：污染 agent 看到的 `git status/log`、被用户 `git push` 推上远端、
  被 agent 的 `rebase/reset/amend` 悬空后 gc 回收（"一等状态不可删除"被正常
  操作静默摧毁）、与并发 git 操作竞争 index.lock；非 git 目录要偷偷
  `git init`；同 workspace 多 session 在同一条历史上互相踩踏。
- **改成什么**：
  - 实现载体改为 **harness 私有 shadow git repo**：独立 `GIT_DIR` + 私有
    `GIT_INDEX_FILE`，共享用户 worktree，checkpoint 引用存 shadow refs，
    **用户 repo 只读不写**；per-session 独立 shadow dir（顺带解决多 session
    并发）。
  - 显式写出**快照覆盖契约**：harness 自有 exclude 规则（untracked 默认入
    快照，配体积上限与黑名单如 node_modules）；声明 submodule/嵌套 repo 内层
    不在 rewind 承诺内；非 git 目录在 shadow 方案下天然支持。
  - **rewind 安全语义**：checkpoint 元数据附带该 turn agent 实际写过的文件
    清单（来自 edit-class 调用的 EffectResolved，零新机制），rewind 默认只
    回退清单内文件；restore 前 diff 出清单外差异（用户手改）时要求确认。
  - 决策 #3 补一句：shadow store 属于 workspace 持久状态的一部分。

### A2. L1 activity 语义 + L4 barrier：增加 detached 类别，解耦「打点」与「静默」

- [ ] **改哪里**：L1「Activity 语义」、L1/L4 的 checkpoint barrier 定义、
  L3 Multi-agent。
- **为什么**：`run_in_background` bash、dev server、长驻子 agent（pub/sub
  blackboard 模式）是既有核心功能，但现设计里 activity 隐含"turn 内有界完成、
  被宿主进程 await"。后果：任一后台任务存活期间"全部子 agent 静默"永不成立
  → 整段会话打不出 barrier，rewind/fork 静默失效；fork 出的切面含未完成
  activity，resume 即触发 in-doubt；进程退出连带杀死用户有意留下的后台任务，
  resume 时还被当事故处理。
- **改成什么**：
  - activity 增加 **detached 类别**（沿决策 #17 带外状态的先例）：启动即以
    handle（task id + pgid + 输出重定向路径）作为结果 Completed 落盘；后续
    输出读取是各自独立的短 activity；detached 豁免 in-doubt 上浮与孤儿
    reap；resume 时按 handle 探活，失效 handle 的读取按决策 #9 渲染 error
    tool_result。
  - **barrier 谓词排除 detached**；打点与静默解耦：workspace commit 与对话
    snapshot 照常 per-turn 打点；**rewind 语义 = 先对活跃子树执行 interrupt
    （协作取消现成）再恢复**；仅纯 fork（不打断原会话）保留一致切面要求。

### A3. 决策 #6：in-doubt 单一「上浮转人工」改为按 tool 类别的策略表

- [ ] **改哪里**：L1「Activity 语义」in-doubt 段 + 决策表 #6。
- **为什么**：agent 的 wall-clock 几乎全在 LLM 调用与 bash 里，任何非优雅
  退出几乎必然砸中 in-flight activity——in-doubt 是崩溃的主导形态而非窄窗口。
  按现契约每次 crash-resume 都以报错/人工确认开场，headless/scheduler 无人
  值守直接卡死。对照：Claude Code 崩溃恢复把被打断工具渲染成 interrupted
  静默续行。
- **改成什么**：in-doubt 处理由 tool 类别标签（read/edit/execute-class，
  已是 tool 定义数据）驱动：LLM 调用 → 自动重发 + TurnDiscarded（机制现成）；
  read-class → 直接重跑；execute/edit-class → 渲染 `[interrupted by crash]`
  error tool_result 继续 loop（决策 #9 通道）。「绝不静默重跑」只保留给
  非幂等类别，「转人工」只留给显式配置的高危工具。Started/Completed 契约不动。

### A4. L3 context assembly：环境快照 journal 化 + 消息流注入通道 + 两级工具面

- [ ] **改哪里**：L3「Context assembly」全节、L2/决策 #10 的 mode 定义、
  L3 MCP 节。
- **为什么**：三个同根矛盾——(1) 环境块（git 状态/日期）与 CLAUDE.md 被拼进
  system prompt 固定段，但它们是动态且未 journal 的外部输入：违反决策 #4、
  与 prefix 不变量自相矛盾、令 activity 缓存式 replay 测试静默失真。
  (2) 决策 #10 的"工具面过滤"若作用于 API tools 参数，plan mode 每次进出 =
  全上下文缓存失效（Anthropic 缓存层级 tools→system→messages）。(3) MCP
  `list_changed` / deferred tool loading 在"禁止或整体换代"二元规则下没有
  经济可行的落点。
- **改成什么**：
  - 环境块与 memory 层在 session 开始时**快照并 journal 成 event**
    （EnvSnapshot/MemoryLoaded 类），context assembly 只从 event 拼装。
  - 新增 **turn 边界合成注入通道**（复用 steering 的"journal 后在边界消费"
    机制）：git 状态更新、CLAUDE.md 重载、目录级 CLAUDE.md 按需加载、将来的
    hook additionalContext 全部作为消息流注入，不改 prefix。
  - 区分 **advertised 工具面**（进 prefix，session 内稳定）与 **permitted
    工具面**（L2 permission 关卡数据，随 mode 任意变）：mode 的"工具面过滤"
    定义为后者，模式外工具按决策 #9 渲染 deny error tool_result，
    ExitPlanMode 常驻工具列表。
  - tool registry 分两级：prefix 级（spec 静态 + 启动时已发现的 MCP schema，
    排序冻结）与消息流级（mid-run 发现/变更以 schema event 为源、注入消息流）；
    整体换代只发生在 compaction/resume 等天然重写点。

### A5. 决策 #11 / 原则 3：hooks 从「管线关卡」泛化为「hook site 列表」；EffectResolved 补 ask 路径

- [ ] **改哪里**：L2 管线一节、决策表 #8、#11。
- **为什么**：Claude Code hook 全家桶中 Stop/SubagentStop（可拒绝停止、强制
  继续）、UserPromptSubmit（注入/拦截用户输入）、SessionStart/SessionEnd/
  PreCompact/Notification 都不挂在任何 effect 上，现架构没有执行点与记录
  通道；注入内容不 journal 会破坏"state 是纯 fold"。另外决策 #8 的单条
  EffectResolved 在 ask 长挂起下有时序漏洞：pre-hook 已执行（有副作用）→
  park 数天 → 进程重启，hook 执行事实只在内存里；恢复时重跑违反自家红线、
  跳过则审计缺失。
- **改成什么**：
  - hooks 定义为 **hook site 列表**：L2 管线 pre/post 是其中两个 site，
    loop lifecycle（session-start/stop/subagent-stop/pre-compact/user-input）
    是另外几个；所有 site 共享同一执行器与记录纪律——判定与注入内容 journal
    成 event、恢复绝不重跑。
  - ask 路径的 **ApprovalRequested event 携带此前已完成关卡的判定**（hook
    结果等），resume 从该 event 续管线；EffectResolved 仍是快路径的单条终态
    汇总。只改 event payload 定义，管线结构不动。

### A6. L3 agent loop：定义 turn；把 interrupt 的「turn sweep」定为一等机制

- [ ] **改哪里**：L3「Agent loop」Steering/interrupt 段、L2 关卡结果枚举、
  L1 等待状态。
- **为什么**：(1) "turn"承载 snapshot、per-turn commit、max_turns、steering
  消费点、barrier 五个机制但全文未定义，不同读法下 steering 体感差一个数量级。
  (2) **安全漏洞**：Esc 时刻"已放行未启动"与"审批挂起中"的 call 没有终态
  event——若不落盘则 fold 缺配对 tool_result 违反 API 约束；若留着
  ApprovalRequested 无终态，crash-resume 后 run 重新进入 WAITING_APPROVAL，
  迟到的应答会执行用户已放弃的危险调用（如 force-push）。
- **改成什么**：
  - 显式定义 **turn = 一次 LLM 调用 + 其 tool 执行周期**；steering 消费点
    改写为"最早可配对点"（当前运行中 activity 结束即注入）。
  - **turn sweep**：InterruptReceived 的 fold 语义 = 该 seq 时刻所有未终态
    call 一律合成 interrupted tool_result，作废其 ApprovalRequested；新增
    终态 event（EffectAbandoned）覆盖"放行未启动/审批挂起"两类；审批应答按
    request id 对已作废请求 no-op。
  - WAITING_APPROVAL 降为 per-effect 状态，run 级状态改为派生。

### A7. 决策 #15c：凭据改走 CredentialProvider 抽象

- [ ] **改哪里**：决策表 #15c、L3 Provider 凭据段、L3 MCP auth 段。
- **为什么**：Claude Code 主流是订阅 OAuth（refresh token 持久化 + 过期刷新
  + 回写）、Codex 是 ChatGPT 登录、MCP streamable HTTP 需要 OAuth token
  存储、Bedrock/Vertex 走云 SDK 凭据链——"只读一个静态环境变量"装不下任何
  一个。长跑 run 中 token 过期后，刷新出的新 token 在现契约下无处合法落盘。
- **改成什么**：15c 改为「凭据经 **CredentialProvider 接口**解析：静态 env
  是其一种实现；受管 token store（OS keychain / 加密文件）是 event log 与
  workspace 之外的第三个持久位置」。保留底线原文：密钥绝不进
  spec/event log/仓库。

---

## B. 一句话契约补丁（8 条）

- [ ] **B1 supervisor 角色**（L4）：新增常驻 supervisor/session-manager——
  维护 durable timer 派生索引、到期 journal TimerFired 并发起目标 session 的
  resume、收容 scheduler、启动时"枚举未终态 run + re-arm 全部未到期 timer"。
  CLI-only 部署显式降级为"下次 resume 时补火"。（否则 timeout/cron/审批过期
  在进程不在时全部哑火，"等几天成本相同"缺执行主体。）
- [ ] **B2 event 版本化纪律**（决策 #12/#18）：所有 fold 消费者（state、
  context assembly、budget、inspect）只经 EventStore 单一读路径取 event，
  该路径预留当前为恒等函数的 upcast 阶段；resume 拒绝检查挂 event-schema
  版本号而非代码版本。（否则周更作废全部存量会话；四个独立 fold 消费面会让
  日后 migration 从纪律问题滑成结构问题。）
- [ ] **B3 PolicyChanged event**（L2/决策 #4）："always allow"等 settings
  写回先 journal 后落盘（崩溃可幂等补做）；harness 配置路径显式排除出
  workspace 快照/rewind 范围。（否则崩溃窗口丢用户决定；rewind 会回滚
  settings，已收紧的 deny 静默复活——安全问题。）
- [ ] **B4 bash 进程组语义**（L1 activity 契约）：bash 以 setsid 新进程组
  启动，pgid 随 ActivityStarted journal；协作取消 = SIGTERM→SIGKILL 整组；
  resume 的 in-doubt 处理先按 pgid 清算存活孤儿。（否则 crash 孤儿继续改
  workspace，掏空"恢复不碰文件系统"的前提；Esc 杀不掉孙进程。）
- [ ] **B5 server 拓扑与优雅停机**（L4/决策 #2）：server 形态推荐
  session-per-process（core-as-library + 文件态持久状态天然支持）；定义
  SIGTERM → 协作取消全部在飞 activity（落 ActivityCancelled）→ snapshot →
  退出。（否则大 fold 饿死全部 session；例行 deploy = 全体在飞 activity
  变 in-doubt 转人工。）
- [ ] **B6 bus 双通道契约**（L0）：ephemeral topic（可丢，仅 delta 类派生物）
  与 guaranteed send（接收方 journal 后 ack，发送意图作为发送方 stream 的
  event 可重试）；frontend 断线重连必须从 event log 对账未决状态。（否则
  "分布式化是换 transport"在跨进程那天丢审批冒泡还"符合设计"。）
- [ ] **B7 blob sidecar**（决策 #3/#12）：event store 补 content-addressed
  blob 存储（event 内只存 hash+mime+size），声明为 event log 存储的一部分，
  fork 按引用共享；binary payload 豁免 tool_output_limit 文本截断。（否则
  图片/附件要么 8MB base64 毁掉 JSONL，要么存临时路径 resume 后 dangling。）
- [ ] **B8 scrub 预留**（L2/决策 #15c）：L2 Execute 记录点之前预留（当前为空
  的）scrub 阶段作为未来脱敏/hook mutation 落点；EventStore 接口预留 at-rest
  加密位。（fold 完整性堵死事后擦除，唯一自洽的擦除点在写入之前。）

---

## C. 备忘（9 条，对应 roadmap 阶段再展开）

- [ ] **C1 thinking 块 opaque passthrough**（M1 定归一化 schema 时）：内部
  表示必须预留 provider 不透明字段（Anthropic signature/redacted 密文、
  OpenAI encrypted reasoning），event→fold→请求全程字节保真。事后补字段会
  让已存日志无法合法回放给 API。
- [ ] **C2 caching capability 限定**（M3）：15b 的 caching 原型期限定为
  无状态请求内标记（Anthropic cache_control + Gemini 隐式缓存）；Gemini
  显式句柄（有生命周期、按 token·hour 计费的带外资源）列为推迟能力，落地时
  按决策 #17 先例：create/renew/delete 记 cost event、WAITING_* 前后有
  生命周期回调。
- [ ] **C3 provider 执行类工具例外**（M3+）：web_search/code_execution/
  MCP connector 的副作用发生在 API 内部——文档化例外类别：请求期整体审批 +
  响应期 post-hoc 观测与记账；处理 pause_turn stop reason。
- [ ] **C4 跨 provider fallback 上移**（M3）：fallback 换 provider 不是
  activity 级重试参数（签名不可移植、schema 方言不同），决策上移到
  loop/context assembly 重新 fold 渲染；同 provider 换档不受影响。
- [ ] **C5 Codex on-failure 沙箱升级**（做沙箱时）：管线需要"执行失败 →
  回到审批关卡"的回边——loop 检测沙箱类失败后合成升级参数的新 effect 重走
  管线，定义同一 tool_use 两条 EffectResolved 的审计语义。
- [ ] **C6 wait-class 工具标签**（M3）：AskUserQuestion/ExitPlanMode 类
  "等用户输入"工具不实现为阻塞 activity（跨崩溃会被 in-doubt 误杀），tool
  定义加 interactive/wait-class 标签，execute 接 WAITING_INPUT park 路径。
- [ ] **C7 workflow 编排纪律**（M5）：决策 #5 砍掉 code replay 的隐藏代价是
  确定性编排脚本必须写成显式状态机（每步完成落 event、fold 出当前位置）——
  在 multi-agent 文档写明该纪律与辅助工具。
- [ ] **C8 MCP sampling/elicitation**（M4）：原型明确不支持并文档化；将来
  需要"activity 内非持久等待"类别（等待不在 turn/tool-call 边界上，durable
  park 承诺对它不成立）。
- [ ] **C9 teleport 环境指纹**（M5）：RunStarted 记环境指纹（cwd/平台/关键
  工具版本）；resume 不匹配不拒绝，但 journal EnvironmentChanged 作为显式
  prefix 换代点并向模型注入环境变更说明。（teleport 本身是架构红利：两处
  持久状态打包即迁移。）
