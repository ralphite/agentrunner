# AgentRunner 设计能力审查 — 对照 Claude Code / Codex 功能全集

> 审查方法：8 个领域分析（agent loop、上下文、工具与 workspace、权限与 hooks、
> 多 agent、session 与 surfaces、MCP/skills/plugins 生态、provider 映射）逐项
> 对照 Claude Code（含 Agent SDK、云端会话）与 OpenAI Codex（CLI/cloud）的功能
> 面做 gap 分析，外加 5 个针对承重架构决策的压力测试。共覆盖 **166 个功能点**，
> 产出 43 条原始风险，去重归并后核实为下述结论。逐功能点的完整判定见
> [CAPABILITY-REVIEW-DETAILS.md](CAPABILITY-REVIEW-DETAILS.md)。

## 总结论

**架构骨架成立，没有发现 blocker。** L0-L2 的核心取舍（actor + journal 一切输入 +
snapshot-resume + 单一 effect pipeline）能撑住 Claude Code / Codex 级 agent 的
绝大部分核心与新功能：166 个功能点里 61 个被现有机制直接覆盖（clean）、87 个
顺着现有抽象即可长出（extension）、18 个存在 friction、0 个需要底层重构。

但 friction 不是均匀分布的——它们聚成 **少数几个同根的结构性问题**，全部集中在
几条"已定决策"的具体措辞上。共同特点：**现在改是改几行设计文本，M2/M3 之后改
是返工实现和测试。** 下面按"必须现在改"和"现在写一句话契约、以后就是普通扩展"
两档列出。

---

## 一、建议在写代码前修订的设计决策（7 项）

### 1. 决策 #7：「workspace 内 per-turn git commit」必须换成 shadow git repo　`严重`

这是全审查最高危的单点，三个领域分析和两个压力测试独立命中同一根因。
workspace 通常**就是用户的 git repo**，而 coding agent 的日常操作就是跑 git：

- **污染与破坏双向发生**：checkpoint commit 落在用户分支上 → agent 跑
  `git log/status` 看到失真状态；用户说"提交并 push" → 中间态快照（可能含误卷
  入的临时文件）被推上远端；agent 跑 `git rebase/reset/commit --amend` → 悬空
  掉 checkpoint commit，被 gc 回收——设计承诺的"一等状态、不可删除"被 agent 的
  完全正常操作静默摧毁。
- **rewind 会清掉用户手改**：`恢复到对应 commit` 在用户 repo 上即 reset --hard，
  用户在 IDE 里的并发修改无告警丢失；设计没有 dirty-state 检测，也没有
  "agent 改动 vs 用户改动"的任何区分维度。
- **覆盖度被 git 追踪语义绑死**：gitignored/untracked 文件拍不进快照但按 L1 的
  定义在 rewind 承诺内；submodule/嵌套 repo 内层完全不入快照；非 git 目录只能
  对用户目录偷偷 `git init`；巨型 monorepo 每 turn 全量 add 是秒级延迟。
- **同一 workspace 多 session 并发**（Claude Code 用户日常）在单条 git 历史上
  互相踩踏。

Claude Code 的真实实现正是 **shadow git repo**（独立 GIT_DIR + 私有 index，
共享 worktree，对用户 repo 零写入），就是为了消除以上全部冲突。

**最小改法**：决策 #7 的实现措辞改为 shadow repo；快照覆盖策略显式化（harness
自有 exclude 规则，untracked 默认入快照、配体积上限）；rewind 由 checkpoint
元数据里的"该 turn agent 实际写过的文件清单"驱动（edit-class 调用本就记录在
EffectResolved 里），restore 前 diff 出清单外差异时要求确认；决策 #3 补一句
shadow store 属于 workspace 持久状态。**语义承诺全部保留，改的只是载体；
"便宜、可 diff"不变。**

### 2. checkpoint barrier 的「全部子 agent 静默」前提，与后台/长驻任务互斥　`严重`

设计通篇假定 activity 在 turn 内有界完成，但 `run_in_background` bash、dev
server、长跑/长驻子 agent（pub/sub blackboard 模式的定义就是不结束）是
Claude Code 的既有核心功能。后果：

- 只要有一个后台任务/子 agent 活着，"全部子 agent 静默"永不成立 → **整段
  会话打不出一个 barrier，rewind/fork（M5 头牌功能）恰在用户最想用的时段整段
  不可用**，且失效是静默的。
- 即便放宽允许 fork，复制出的切面里含 Started 无 Completed 的长活 activity，
  新 run 一 resume 就触发 in-doubt 上浮——fork 出生即报错。
- 进程退出 = 后台 dev server 一起死，resume 时被当成事故（in-doubt）处理，
  而这是用户有意为之的后台任务。

**最小改法**：沿决策 #17（MCP server 带外状态）的先例，给 activity 语义增加
**detached 类别**——启动即以 handle（task id/pgid）作为结果 Completed 落盘，
后续输出读取是各自独立的短 activity；barrier 谓词把 detached 排除在外；
**解耦打点与静默**：workspace commit 与对话 snapshot 照常 per-turn 打点，
rewind 语义定义为"先对活跃子树执行 interrupt（协作取消机制现成）再恢复"；
仅纯 fork 保留一致切面要求。

### 3. 决策 #6：in-doubt 一刀切「上浮转人工」——崩溃恢复的主导形态会退化成人工 triage　`严重`

agent 的 wall-clock 几乎全部耗在 LLM 调用与 bash 里，任何非优雅退出（关终端、
休眠、OOM）**几乎必然**砸中一个 in-flight activity——in-doubt 不是设计文本暗示
的窄窗口，而是崩溃的主导形态。按现契约每次 resume 都以报错/人工确认开场；
headless/scheduler 无人值守 run 遇 in-doubt 直接卡死。对照：Claude Code 崩溃后
resume 把被打断的 tool 渲染成 interrupted 静默续行，无人工环节。

**最小改法**：把单一「上浮」改成**按 tool 类别的数据化 in-doubt 策略**（类别
标签已是 tool 定义数据）：LLM 调用 → 自动重发 + TurnDiscarded（机制现成）；
read-class → 直接重跑；execute/edit-class → 渲染 `[interrupted by crash]`
error tool_result 继续 loop（决策 #9 通道现成）。「绝不静默重跑」只保留给
非幂等类别，「转人工」只留给显式配置的高危工具。只改决策 #6 的措辞与一张
策略表，Started/Completed 契约不动。

### 4. Prefix 不变量与动态现实的矛盾：缺「消息流注入通道」和「两级工具面」两个概念　`严重`

三个同根表现：

- **环境块自相矛盾**：L3 把"git 状态、日期"排进 system prompt 固定段，同节又
  宣布 prefix 稳定是生死不变量——git 状态每个 edit turn 都变。且环境块/CLAUDE.md
  是未 journal 的外部输入，违反决策 #4，还会让"activity 缓存式 replay 测试"
  静默失真（重放时拼出与录制不同的请求）。
- **plan mode 若按字面实现打爆缓存**：决策 #10 的"工具面过滤"若作用于 API tools
  参数，每次 shift+tab 进出 = 全上下文 cache 失效（Anthropic 缓存层级
  tools→system→messages）。Claude Code 实际不改 tools 参数——收窄由 permission
  层 deny 实现，ExitPlanMode 常驻。
- **动态 tool 面没有经济落点**：MCP `list_changed`、deferred tool loading
  在"禁止或整体换代"的二元规则下，唯一合法动作是反复全量换代，缓存命中率
  趋近于零。

**最小改法**：(a) 环境块与 memory 层 session 开始时快照并 journal 成 event，
context assembly 只从 event 拼装；(b) 新增 **turn 边界合成注入通道**（复用
steering 的"journal 后在边界消费"机制），git 状态更新、CLAUDE.md 重载、将来的
hook additionalContext 全走这条通道进消息流而非改 prefix；(c) 区分
**advertised 工具面**（进 prefix，session 内稳定）与 **permitted 工具面**
（L2 关卡数据，随 mode 任意变），mode 的"工具面过滤"定义为后者；(d) tool
registry 分两级：prefix 级（静态 + 启动时已发现）与消息流级（mid-run 变更以
schema event 为源、注入消息流），整体换代只发生在 compaction/resume 等天然
重写点。

### 5. hooks 定位过窄：lifecycle hooks 无处可挂 + EffectResolved 单条 event 有时序漏洞　`严重`

Claude Code 的 hook 全家桶里，**Stop/SubagentStop（可拒绝停止、强制 loop 继续）、
UserPromptSubmit（注入上下文/block 用户输入）、SessionStart/SessionEnd/
PreCompact/Notification** 都不挂在任何 effect 上——现架构里它们没有执行点也没有
记录通道；若不 journal 注入内容，resume 后 fold 重建的对话与实际发给模型的内容
不一致，直接破坏"state 是纯 fold"不变量。此外决策 #8 的单条 EffectResolved
在 **ask 长挂起**场景有漏洞：pre-hook 已执行（有副作用）→ park 数天 → 进程
重启，hook 执行事实只存在于内存；恢复时重跑违反自家红线、跳过则审计记录缺失。

**最小改法**：(a) 把 hooks 从"L2 管线关卡"泛化为 **hook site 列表**（管线
pre/post 是其中两个 site；loop lifecycle——start/stop/compact/user-input——是
另外几个），所有 site 共享同一执行器与同一记录纪律（判定与注入内容 journal 成
event，恢复不重跑）；(b) ask 路径的 ApprovalRequested event 携带此前已完成
关卡的判定，EffectResolved 仍作终态汇总——只改一处 event payload 定义。

### 6. interrupt / steering 的精确语义没定：有一个安全级漏洞　`严重`

- **"turn"承载了 snapshot、per-turn commit、max_turns、steering 消费点、rewind
  barrier 至少五个机制，但全文未定义**。按不同读法，steering 的体感差一个数量级
  （Claude Code 是当前 tool 跑完立即注入，不等同轮其余 tool）。
- **安全漏洞**：Esc 时刻处于"已放行未启动"和"审批挂起中"的 call 没有终态
  event。若不落盘，fold 缺配对 tool_result 违反 API 约束；若留着
  ApprovalRequested 无终态，**crash-resume 后 run 重新进入 WAITING_APPROVAL，
  迟到的应答会执行一条用户已用 Esc 放弃的危险调用**（如 `git push --force`）。

**最小改法**：显式定义 turn = 一次 LLM 调用 + 其 tool 执行周期；把 **turn
sweep** 定为一等机制——InterruptReceived 的 fold 语义 = 该时刻所有未终态 call
一律合成 interrupted tool_result 并作废其 ApprovalRequested（补一条
EffectAbandoned 终态 event）；审批应答按 request id 对已作废请求 no-op；
steering 消费点改写为"最早可配对点"。

### 7. 决策 #15c：「凭据只从环境变量读」装不下三类主流认证　`严重`

Claude Code 主流用户走**订阅 OAuth**（refresh token 持久化 + 过期刷新 + 回写），
Codex 走 ChatGPT 登录，MCP streamable HTTP 需要 OAuth token 存储，Bedrock/Vertex
走云 SDK 凭据链——都不是"读一个静态环境变量"能表达的。失败场景：长跑 run 中
access token 过期，刷新后的 token 在 15c 下无处合法落盘，进程一死即丢。

**最小改法**：15c 改为「凭据经 **CredentialProvider 接口**解析；静态 env 是
其一种实现；受管 token store 是 event log 与 workspace 之外的第三个持久位置」，
保住真实意图"密钥绝不进 spec/event log/仓库"。

---

## 二、现在写一句话契约、以后就是普通 extension 的（8 项）

| # | 问题 | 一句话契约 |
|---|------|-----------|
| 8 | **durable timer 没有触发者**：进程不在时，timeout/cron/审批过期/离线唤醒全部哑火，"等几天成本相同"的承诺缺执行主体 | L4 命名一个 **supervisor 常驻角色**：维护 timer 派生索引、到期 journal TimerFired 并发起 resume、收容 scheduler；CLI-only 部署显式降级为"下次 resume 补火" |
| 9 | **决策 #18 跨版本 resume**：周更 CLI 作废全部存量会话；event 被 4 个独立 fold 消费，拖久了 migration 从纪律问题滑向结构问题 | 所有 fold 消费者只经 EventStore 单一读路径，该路径预留当前为恒等的 **upcast 阶段**；拒绝检查挂 event-schema 版本号而非代码版本 |
| 10 | **policy 热更新**（"always allow"写回 settings）是 log 外副作用：崩溃窗口丢失；settings 若在 workspace 内还会被 rewind 回滚——**已收紧的 deny 静默复活** | 新增 **PolicyChanged event**（先 journal 后写盘，幂等补做）；harness 配置路径显式排除出快照/rewind 范围 |
| 11 | **bash 无进程组语义**：crash 孤儿继续改 workspace，掏空"恢复不碰文件系统"的前提；Esc 杀不干净孙进程（端口占用） | activity 契约补：bash 以 setsid 新进程组启动，**pgid 随 ActivityStarted journal**；取消 = killpg；resume 先按 pgid 清算孤儿再判 in-doubt |
| 12 | **server 形态无隔离与停机故事**：单 loop 被大 fold 饿死；例行 deploy = 全部在飞 activity 变 in-doubt 转人工 | 推荐拓扑 **session-per-process**（core-as-library + 文件态持久状态天然支持）；定义优雅停机：SIGTERM → 协作取消全部在飞 activity（落 ActivityCancelled）→ snapshot → 退出 |
| 13 | **"分布式化是换 transport"低估契约**：ephemeral bus 跨进程后丢一条审批冒泡是"符合设计"的行为 | L0 bus 契约分**双通道**：ephemeral topic（可丢，delta 类）与 guaranteed send（接收方 journal 后 ack）；frontend 重连必须从 event log 对账未决状态 |
| 14 | **图片/附件没有存储归属**：base64 内联毁掉 JSONL 可读性，存临时路径则 resume/fork 后 dangling | event store 补 **content-addressed blob sidecar**（event 存 hash+mime+size），声明为 event log 存储的一部分；binary 豁免文本截断 |
| 15 | **event log 明文 secrets**：fold 完整性堵死事后擦除，唯一自洽的擦除点在写入之前，而管线里没有这个点 | L2 Execute 记录点之前预留（当前为空的）**scrub 阶段**；EventStore 接口预留 at-rest 加密位 |

## 三、Minor（记录在案即可，不必现在动设计）

- **Gemini 显式 cache 句柄**是有生命周期、按 token·hour 持续计费的带外资源，
  "挂起几天成本相同"对它不成立 → 原型期把 caching capability 限定为无状态
  请求内标记（Anthropic cache_control + Gemini 隐式缓存），显式句柄列为推迟
  能力并按决策 #17 先例预留（create/renew/delete 记 cost event）。
- **provider 侧执行的工具**（web_search、code_execution、MCP connector）副作用
  发生在 API 内部，per-call permission/pre-hook 无处插入 → 文档化一个"provider
  执行类工具"例外类别：请求期整体审批 + 响应期 post-hoc 观测记账。
- **跨 provider fallback** 不是 activity 级重试参数：签名 thinking 块不可移植、
  tool schema 方言不同，历史必须重新 fold/渲染 → fallback 决策上移到
  loop/context assembly 层；同 provider 内换档不受影响。
- **Codex on-failure 沙箱升级审批**（沙箱内失败 → 请求出沙箱重试）需要管线
  "执行失败后回到审批关卡"的回边 → loop 检测沙箱类失败后合成升级参数的新
  effect 重走管线，定义两条 EffectResolved 对同一 tool_use 的审计语义。
- **AskUserQuestion 类等输入工具**若实现为阻塞 activity，跨崩溃会被 in-doubt
  误杀 → tool 定义加 interactive/wait-class 标签，execute 走 WAITING_INPUT
  park 路径而非阻塞 activity。
- **deterministic workflow 编排**（Workflow 脚本式多 agent）：决策 #5 砍掉
  code replay 的隐藏代价是编排脚本必须手写成显式状态机（每步落 event）——能做，
  但要在 multi-agent 文档里写明这个纪律，别让 workflow 作者自己发现。
- **MCP sampling/elicitation**：server 在 activity 执行中途反向请求，等待不在
  任何边界上，durable park 承诺对它不成立 → 原型明确不支持，将来需要"activity
  内非持久等待"类别。
- **teleport 是架构红利**（两处持久状态 + snapshot 可弃 + MCP 带外，打包即迁移），
  仅缺环境指纹：RunStarted 记 cwd/平台指纹，resume 不匹配时 journal 一条
  EnvironmentChanged 作为显式换代点并向模型注入说明。
- **thinking 块 opaque passthrough**：归一化内部表示必须在**定型时**就预留
  provider 不透明字段（Anthropic signature/redacted 密文、OpenAI encrypted
  reasoning）并保证 event→fold→请求全程字节保真——事后补字段会导致已存日志
  无法合法回放给 API。

---

## 四、设计强项（审查确认可以放心押注的）

- **单一 effect pipeline** 是全设计最值钱的决定：并行 tool call 混合审批、
  失败面向模型的渲染（决策 #9）、permission modes 作为数据、审批不阻塞已放行
  调用——Claude Code 最难缠的这批行为都被四关卡 + EffectResolved 直接覆盖。
- **journal + fold + snapshot（不做 code replay）**用 ~10% 成本拿到了 crash
  恢复、长审批 durable park、steering 不丢、可审计时间线；166 个功能点里没有
  任何一个真正需要 Temporal 式确定性 replay。
- **capability 抽象（15b）**方向正确：thinking/caching/structured output/effort
  各家映射 + 显式降级，是支撑三 provider 的正确形状（需补 opaque passthrough）。
- **MCP schema 记录为 event、server 生命周期带外**（决策 #17）是准确的切分，
  并且是本审查多处修复建议的可复用先例。
- **skills 沿用 Claude Code 约定、tool 定义是数据、core 是库**：生态接入
  （plugins/slash commands/SDK in-process tools）全部顺着这三条长，无一 friction。

## 五、覆盖统计

| 领域 | clean | extension | friction | blocker |
|------|------:|----------:|---------:|--------:|
| agent loop | 13 | 8 | 1 | 0 |
| 上下文管理 | 8 | 14 | 1 | 0 |
| 工具与 workspace | 6 | 10 | 2 | 0 |
| 权限与 hooks | 6 | 11 | 4 | 0 |
| 多 agent | 8 | 8 | 2 | 0 |
| session 与 surfaces | 7 | 10 | 2 | 0 |
| 生态（MCP/skills/plugins） | 8 | 13 | 2 | 0 |
| provider 映射 | 5 | 13 | 4 | 0 |
| **合计（166）** | **61** | **87** | **18** | **0** |

> 注：18 个 friction 全部归并进上文第一、二、三节的条目；逐功能点判定与理由见
> [CAPABILITY-REVIEW-DETAILS.md](CAPABILITY-REVIEW-DETAILS.md)。
> 审查的对抗性核实阶段按提交人要求压缩，关键论断（shadow git、缓存层级失效
> 规则、plan mode 实现方式、steering 注入时机、Stop hook 语义、OAuth 主流路径、
> crash-resume 行为）已逐条对照两家产品的真实行为人工核实。
