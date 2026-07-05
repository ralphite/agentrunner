# 能力审查明细 — 166 个功能点逐项判定

> 本文件由审查流程自动汇总。verdict 口径：**clean** = 现有机制直接覆盖；**extension** = 顺着现有抽象新增代码即可；**friction** = 需局部修改某条已定决策或层间契约（已归并进 [CAPABILITY-REVIEW.md](CAPABILITY-REVIEW.md)）；**blocker** = 需底层重构（本次审查未发现）。

## Agent loop 核心机制（22 项）

### token-level streaming output — `clean`

*core · both* · 依据：L3 agent loop / Streaming 的持久化边界；决策#3、#15

设计把 streaming 定为一等能力：provider 薄接口 `complete(request) → stream` 原生流式（决策#15），token delta 只走 bus、显式 ephemeral，持久化的是组装完成的 assistant message 一条 event（L3 "Streaming 的持久化边界"，明确说这是原则2的正版应用）。L4 输出事件流里也已列入 token delta，且 "token streaming 是纯增量，协议不变"。持久层与展示层切分干净，崩溃时丢什么一目了然，直接落 M3。

### fine-grained tool parameter streaming — `clean`

*important · both* · 依据：L3 agent loop / Streaming 的持久化边界；L4 交互协议

tool 参数流式只是 bus 上多一种 delta 类型，"delta ephemeral / 组装后 message 持久化" 的边界完全不变；tool 执行必须等 tool_use 块组装完整才进 L2 管线，与管线时序无冲突。唯一要处理的是 fine-grained streaming 下 max_tokens 截断产生半截 JSON 参数的情况，属 provider 层解析细节，不触碰任何契约。

### extended thinking budget — `clean`

*core · claude-code* · 依据：L3 agent spec model.thinking / 决策#15b

spec 已原生写入 `thinking: { budget_tokens: 4096 }`，决策#15b 把 thinking 定为 provider 无关的可选 capability，各 provider 自行映射（Anthropic extended thinking / Gemini thinking config），不支持时"明确降级或报错而不是静默忽略"。per-message 动态调 budget（ultrathink 类关键词）只是请求参数变化，不触碰 prefix 稳定性不变量。

### thinking block signature round-trip (incl. redacted thinking) — `extension`

*core · claude-code* · 依据：L3 Provider 返回归一化 / 决策#15b；L3 context assembly fold

Anthropic 要求 thinking/redacted_thinking 块连同 signature 字段逐字节原样回传，否则 thinking+tool use 的后续请求直接被 API 拒绝。设计的"返回归一化……thinking 块统一成一套内部表示，L2/L3 不感知具体 provider"（决策#15b）没有承诺保留 provider 专属的不透明字段——归一化表示必须显式加 opaque passthrough（signature、redacted 密文），并保证 event log 里的 assistant message event 与 context assembly 的 fold 全程字节保真。顺着现有抽象加字段即可、不动契约，但这个约束必须在归一化 schema 定型时就钉死：事后补字段会导致已存日志无法合法回放给 API。

### interleaved thinking — `clean`

*important · claude-code* · 依据：L3 Provider 返回归一化 / 决策#15b

归一化表示按 content block 组织（"tool_use、thinking 块统一成一套内部表示"），interleaved thinking 只是同一 assistant 消息内 thinking/text/tool_use 块的有序混排加一个 beta 开关，fold → provider 请求的路径天然保序。signature 保真由 signature round-trip 条目覆盖，此外无新增机制。

### parallel tool calls (mixed allow/ask, out-of-order completion) — `clean`

*core · both* · 依据：L3 agent loop 并行 tool call / L2 决策#8、#9

L3 明文"并行 tool call 是常态"：每个 call 独立过 L2 管线，判定 allow 的并发执行，判定 ask 的按序等审批且"审批挂起不阻塞已放行的 call"，完成 event 按到达顺序落盘——部分 allow 部分 ask 混合与乱序完成这两个难点都被点名设计。决策#9 保证每个 tool_use 都有配对 tool_result（deny/block/失败均渲染为 error tool_result），API 配对约束不会被并行破坏。

### steering (mid-turn user injection at tool-call boundary) — `clean`

*core · both* · 依据：L3 agent loop Steering / L1 "挂起是显式状态" / 决策#4

插话按决策#4 先 journal 成 event 再消费，崩溃不丢；L1 明文"审批、timer、人工输入全都发生在 turn/tool-call 边界"，已把 tool-call 边界写进挂起契约。L3 的表述是"在 turn 边界被 loop 消费"，但从 max_turns:40 与 per-turn snapshot/commit 看，本设计的 turn = 单次 LLM 调用 + 其 tool batch，该边界即等价于 Claude Code 的"当前 tool call 结束后立即注入"（下一次 LLM 调用之前）。不需要改契约，只需实现时钉死 turn 的定义、消除 L1 与 L3 两处措辞的歧义。

### interrupt (Esc) with consistent post-interrupt state — `clean`

*core · both* · 依据：L1 activity 协作取消 / 决策#6；L3 agent loop Steering 与 interrupt

协作取消是 activity 一等能力（决策#6）："跑了 10 分钟的 bash 必须能被 Esc 杀掉——interrupt 语义建立在这之上"；打断记 `ActivityCancelled{partial_output}`，被打断的 tool call 以 "[interrupted by user]" tool_result 呈现给下一 turn，满足 tool_use↔tool_result 配对。打断流式 LLM 调用同样是取消一个 activity——token delta 本就 ephemeral，一致性由"持久化的只有组装完成的 message"保证。测试策略里还专门为 interrupt 排了崩溃注入测试。

### retry / 429 / 529 overloaded / mid-stream disconnect recovery — `clean`

*core · both* · 依据：L1 activity 语义（retry）/ 决策#6；L3 agent loop TurnDiscarded

"retry/backoff、rate limit 处理、model fallback 是 activity 级策略"（决策#6），所有副作用共享一套重试语义；mid-stream 断流重试有专门的 `TurnDiscarded` event，前端据此渲染"重试中"并重新开流，"绝不静默替换用户已看到的文本"。一个实现口径要注意：决策#6 的 in-doubt "绝不静默重跑"字面上也覆盖崩溃时半途的 LLM 调用，恢复后应把它按 TurnDiscarded 处理、由用户继续触发新调用，而不是机械地转人工——这是策略细化，不是契约修改。

### mid-session model switch (/model) incl. cross-provider — `extension`

*important · both* · 依据：L3 context assembly Prefix 稳定性（"要么禁止要么显式换代"）/ 决策#4、#15b

需要新增一个 journal 化的运行时覆盖 event（ModelChanged 类，走决策#4 的输入语义）参与 fold，并走 prefix 不变量里明文预留的"显式换代"路径重建缓存前缀——两个钩子设计里都有。跨 provider 的历史序列化正是决策#15b 归一化内部表示的目标场景；要补的只是 provider 专属不透明块的降级规则（换家时 Anthropic thinking signature、Codex encrypted reasoning 必须丢弃）。/model 不是 spec 变更，不触发决策#18 的版本拒绝逻辑，属顺着骨架长的新代码。

### reasoning effort levels & fast mode (serving tier) — `clean`

*important · both* · 依据：L3 Provider capabilities / 决策#15b

决策#15b 把能力定义为"通用的、可选的"：reasoning effort（Codex 的 minimal→xhigh）与 serving 档位/service tier 只是再加两个 capability 字段，请求以 provider 无关方式携带意图，各家自行映射，不支持时"明确降级或报错"已有明文。中途切换与 /model 复用同一个运行时覆盖 event 机制。

### structured output (forced JSON schema; subagent result contract) — `extension`

*important · both* · 依据：L3 Provider 能力抽象 / 决策#15b；L3 Multi-agent result contract

请求侧走决策#15b 加一个 output_schema capability（Gemini responseSchema / OpenAI json_schema / Anthropic tool-forcing），是既有模式的直接套用。但 multi-agent 的 result contract 目前只"在子 agent spec 的 description/输出约定里声明"，是文本约定而非 schema 强制；要让 subagent 结果可靠回流，需在 spec 增加输出 schema 字段，loop 侧对最终输出做校验与修复重试。均为新增组件，不动 L0-L2 契约。

### stop reason handling: max_tokens continuation, prompt-too-long — `clean`

*core · both* · 依据：L3 Provider 返回归一化 / 决策#15b；context assembly Compaction / 决策#5、#9

finish reason 已在 provider 层归一化（决策#15b），max_tokens 截断续写是 loop 对归一化 stop reason 的分支处理。prompt too long 的事前防线是 compaction（trigger_ratio 0.8，recorded activity，产出 ContextCompacted 并"改变后续 fold 的结果"）；事后兜底是捕获 provider 错误、触发 compaction 后重试同一 activity——因为没有 code replay 纪律（决策#5），重试时按新 fold 重建请求完全合法。budget 类超限还有"让模型收尾的最后一条消息+优雅停止"的明文（决策#9）。

### automatic model fallback (primary unavailable) — `clean`

*important · claude-code* · 依据：L1 activity 语义（retry/fallback）/ 决策#6

"model fallback 是 activity 级策略"在 L1 activity 语义里被点名为所有副作用共享的通用属性（决策#6），529/overloaded 触发 fallback 就是 retry 策略的一个分支。同 provider 降档（opus→sonnet）零额外成本；跨 provider fallback 需叠加 model-switch 条目里的不透明块降级规则。

### token counting & cost accounting (cache read/write split) — `clean`

*core · both* · 依据：L3 context assembly（cache_read/write 归一化记账）/ L2 budget 关卡 / L4 Observability

L3 明文"LLM activity 的 event 记录归一化的 cache_read/cache_write token，budget 关卡按真实计费口径记账"，观测面 inspect 也列了 token/cost 含 cache 命中——cache 分口径正是题面要求的形态，且 budget 关卡（L2 第3关）直接消费这些数据。小注意：Gemini 显式 context cache 按存储小时计费而非纯 token 口径，记账 schema 留一个 provider 附加费用字段即可，不构成结构问题。

### Codex reasoning summaries & encrypted reasoning items — `extension`

*core · codex* · 依据：L3 Provider 返回归一化 / 决策#15、#15b

reasoning summary（可见摘要）走 bus 的 delta 流，encrypted_content（不透明密文）落进归一化 thinking 块——与 Anthropic signature 同构的 opaque passthrough 问题：event log 存原文，fold 时逐字节回传。OpenAI provider 作为第三个 provider 实现接入，决策#15 的"两个实现验证抽象不漏"正是为此铺路。前提同 signature 条目：归一化表示必须在定型时预留 provider 不透明字段。

### Codex Responses API stateful conversation — `extension`

*important · codex* · 依据：决策#3、#17（带外运行时状态先例）/ L3 Provider（Gemini context cache 句柄）

若把 previous_response_id/conversation 当 source of truth，会与决策#3"持久状态只有 event log 和 workspace 两处"冲突；但设计已有同类先例：MCP server 生命周期是"带外运行时状态，不进 event 模型"（决策#17），Gemini context cache 句柄也由 provider 层自行管理。provider 侧对话句柄同样可当可丢的带外缓存——句柄失效或 rewind/fork 时退回 store:false + 全量历史重发（encrypted reasoning items 保 reasoning 连续性，Codex 自身即以此模式支持 ZDR）。功能面无损，属 provider 实现内部的优化路径。

### prompt caching & prefix stability (loop-placed breakpoints) — `clean`

*core · both* · 依据：L3 context assembly Prefix 稳定性 / 决策#15、#15b

设计把 prefix 稳定性列为"显式不变量"并直言"没有它 agent loop 在经济上不可用"：system prompt 与 tool schema 排序稳定、cache 断点由 loop 放置、任何打爆 prefix 的操作要么禁止要么显式换代；缓存落地方式（Anthropic cache_control vs Gemini context cache 句柄）归 provider 实现，token 记账归一化。这是全设计中论证最充分的机制之一，M3 直接落地。

### background tool execution (run_in_background, async result injection) — `extension`

*important · claude-code* · 依据：L1 activity 语义 / 决策#4、#6；L4 fork/rewind barrier

可由现有 primitive 组合：bash activity 照常 Started 落盘，loop 立即合成"running in background"的 tool_result 满足配对并继续 turn，后续输出/完成按决策#4 journal 成输入 event 在 turn/tool-call 边界注入。需要补两处定义：fork/rewind 的 barrier 条件（现为"turn 边界+全部子 agent 静默+workspace commit"）要扩成"且无 in-flight background activity"；跨 turn 的后台 activity 在崩溃后按决策#6 的 in-doubt 语义上浮，现成可用。均为局部扩展，不动 L0-L2 契约。

### mid-turn context injection (system-reminders: file changes, todo state) — `extension`

*important · claude-code* · 依据：L0 bus 契约（journal 先于消费）/ 决策#4；L3 context assembly

Claude Code 重度依赖在 tool-call 边界注入 system-reminder（文件被外部修改、todo 状态、CLAUDE.md 提示）。在本设计里这些是"会影响 run 结果的输入"，按 L0 bus 契约/决策#4 必须先 journal 成 event，再由 context assembly 拼进请求尾部——机制现成，且注入发生在 suffix，不破坏 prefix 缓存不变量。需要新增的是产生 reminder 的探测组件（workspace 变更监测等），纯 L3/L4 新代码。

### server-side tools (provider-executed web search) & pause_turn — `extension`

*important · both* · 依据：L2 effect pipeline / 原则3；L3 Provider 返回归一化

Anthropic server-side web_search（含 pause_turn 自动续跑）与 OpenAI 内建 web_search 在 provider 采样过程中执行，原则3"一切副作用流经同一条 effect pipeline"对它们物理上够不着——L2 的 hooks/permission 无法逐次拦截，只能在请求构造时做工具面级 allow/deny。这与任何客户端 harness 的约束一致（Claude Code 同样不能逐次拦截 server tool），不构成能力差距：把 server tool 调用记录为 LLM activity 结果内的归一化块、loop 对 pause_turn stop reason 自动续请求即可。建议在设计里明文写下"provider 执行的工具不过 L2"这条例外，避免实现期把它硬塞进管线。

### OAuth/subscription auth & cloud credential chains (token refresh) — `friction`

*core · both* · 依据：决策#15c / L3 Provider 凭据

障碍是决策#15c 原文："凭据只从环境变量读（GEMINI_API_KEY 等），绝不进 spec/event/仓库"，provider 实现"从环境读取"。而 Claude Code 的主流路径是 claude.ai 订阅 OAuth（refresh token 持久化+过期刷新+回写凭据存储），Codex 同样以 ChatGPT 账号登录为主，Bedrock/Vertex 走云 SDK 凭据链——三者都不是"读一个静态环境变量"能表达的。具体失败场景：一个跑数小时的 run 中 access token 过期，provider 必须执行刷新并把新 token 持久化到凭据存储，纯环境变量模型下无处安放，刷新后的 token 随进程死亡即丢，run 恢复后无法再调 LLM。修法是把 #15c 局部改为"凭据经 CredentialProvider 接口解析，静态 env 是其一种实现"，保住"密钥不进 spec/event/仓库"的底线；改动不大，但决策原文构成直接障碍，故为 friction。

## 上下文管理（23 项）

### dynamic-env-block-vs-prompt-cache — `friction`

*core · both* · 依据：L3 Context assembly / 原文"环境块（cwd、git 状态、日期）"与"Prefix 稳定性是显式不变量"

L3 context assembly 的固定拼装顺序把"环境块（cwd、git 状态、日期）"放在 system prompt 第二段，同一节又宣布"Prefix 稳定性是显式不变量"、"没有它 agent loop 在经济上不可用（约 10x）"——两条互相矛盾：git 状态是每 turn 都变的动态内容。失败场景：agent 每 turn 编辑文件 → git status 变化 → system prompt 逐 turn 重渲染 → Anthropic cache_control 前缀每请求 miss，缓存经济性被自家拼装规则归零。现有护栏"任何会打爆 prefix 的操作（配置中途变更）要么禁止要么显式换代"只针对配置变更，未覆盖环境块的固有动态性。修复必须局部修改 L3 拼装契约：给环境块加每 session/每代冻结语义，或把 git 状态等动态项移出 system prompt、注入消息流（Claude Code 的实际做法），两者都不是现文本描述的机制。

### system-prompt-assembly-order — `clean`

*core · both* · 依据：L3 Context assembly / 原文"System prompt 是拼装的，顺序固定"

L3 明确把 context assembly 列为"一等组件"，system prompt 拼装顺序固定：harness 基础指令 → 环境块 → memory 文件层 → tool/skill/子 agent 目录 → spec system prompt，与 Claude Code 的实际分段一一对应。目录注入被点名为 multi-agent 可用的前提（"模型不知道 summarizer 存在就永远不会 spawn 它"），说明各段来源已被想清楚。落进 M3 roadmap 即可。

### claude-md-hierarchy-and-imports — `extension`

*core · claude-code* · 依据：L3 Agent spec / context.memory_files + "配置分层从简：两层起步" + M4

spec 已有 context.memory_files: true，L3 写明"CLAUDE.md 按目录层级合并"，M4 排期了 memory 文件——祖先目录链合并的骨架存在。enterprise/user/project/local 四级依赖配置分层，而设计明确"两层起步、三层与更细的合并语义等真实冲突出现再加"，属文档化推迟；@import 只是 memory 加载器内的解析逻辑。这些都顺着现有 memory 层抽象生长，不动任何契约。

### subdir-claude-md-on-demand-injection — `extension`

*important · claude-code* · 依据：L3 Context assembly memory 层 + 决策#4 输入语义

Claude Code 的子目录 CLAUDE.md 是运行中途（tool 首次触达该子树时）才注入的，不能走设计里的 memory 层——那是 system prompt 的一段，中途改动会打爆 prefix 不变量——必须走消息流注入。好在决策#4"一切外部输入先 journal 成 event 再消费"提供了正版通道：发现的文件内容 journal 成 event，fold 时渲染为消息流里的注入块，确定性与可审计性都保住。需要新增"发现触发器 + 消息流注入"两块代码，但完全顺着 journal+fold 抽象走；只是现文本"按目录层级合并"读起来是 session 启动时的静态合并，需在实现时补上按需通道，不构成契约冲突。

### agents-md-convention — `clean`

*important · both* · 依据：L3 Agent spec / context.memory_files + 决策#16

AGENTS.md（Codex 主用、Claude Code 已兼容）只是 memory 文件机制下多一个文件名发现规则，L3 的"CLAUDE.md 式指令文件注入"与"按目录层级合并"直接覆盖其层级合并语义（repo 根到 cwd 链）。决策#16"沿用 Claude Code 约定、生态兼容不发明格式"表明设计立场就是兼容既有约定。M4 memory 文件排期内即可带上。

### auto-compaction-threshold — `clean`

*core · both* · 依据：L3 Context assembly / 原文"Compaction 是 recorded activity" + 决策#6

spec 已有 compaction: { trigger_ratio: 0.8 }，L3 明确"Compaction 是 recorded activity"：它本身是一次 LLM 调用（非确定性副作用），产出 ContextCompacted{summary, kept_range} event 并改变后续 fold 结果。崩溃发生在 compaction 中途由 L1 的 in-doubt activity 语义兜底（有 Started 无 Completed 上浮，不静默重跑），阈值判断的输入（token 用量）来自 event 记录的归一化 cache_read/cache_write 计数。机制闭环，M3 排期内。

### manual-compact-and-clear — `extension`

*important · both* · 依据：L4 交互协议"协议预留：slash command" + L3 ContextCompacted 模式

手动 /compact 带自定义 instructions：slash command 在 L4 交互协议里是"协议预留（原型不实现）"，指令 journal 成 event 后触发同一个 compaction activity，instructions 作为其参数记录在 event 里，可审计可重建。/clear 在 append-only event log 下不能删历史，但 ContextCompacted 已确立"事件改变后续 fold 视图"的模式，加一个 ContextCleared 事件（fold 后消息视图清空、memory/环境保留）是同构的。都是新增代码、不动 L0-L2 契约。

### microcompaction-context-editing — `extension`

*important · claude-code* · 依据：L3 Context assembly / ContextCompacted event 模式

清理旧 tool result 而非整体 summarize：ContextCompacted{summary, kept_range} 的 schema 是摘要式、连续区间，microcompaction 需要按条目选择性擦除（非连续 id 列表），要新增一种 fold-affecting event 类型。但"事件改变 fold 视图"的模式直接泛化；且 microcompaction 是确定性规则不需要 LLM 调用，只需纯 event 不需要 activity，比 compaction 更简单。改写消息中段必然打一次 cache 前缀重写，这是该技术的固有成本（Claude Code 同样承担），不是设计缺陷。

### compaction-rewind-fork-semantics — `clean`

*important · claude-code* · 依据：L3 Context assembly / 原文"跨 compaction 边界的 fork/rewind 语义因此是良定义的" + L4 fork/rewind barrier

设计原文直接回答了这个问题："跨 compaction 边界的 fork/rewind 语义因此是良定义的：fold 到哪个 seq，就得到哪个视图"。rewind 到 compaction 之前的 barrier = fold 到更早 seq，得到未压缩视图；若该视图再次逼近窗口上限，trigger_ratio 会重新触发 compaction，自洽。L4 的 barrier 语义（turn 边界 + 子 agent 静默 + workspace commit）与 compaction event 落在 turn 边界天然对齐。

### tool-result-truncation-and-paged-read — `clean`

*core · both* · 依据：L3 Context assembly / 原文"Tool 结果截断" + spec context.tool_output_limit + 决策#13

L3 明确"Tool 结果截断：per-tool 输出上限，超限截断并告知模型被截断了——一条 cat large.json 不能毁掉上下文和预算"，spec 有 tool_output_limit: 30000，且 tool 定义作为数据包含 per-tool 截断上限（决策#13）。超大文件分页读取只是 read_file tool 定义里加 offset/limit 参数，tool 定义是数据文件，不涉及任何框架改动。

### prompt-cache-prefix-stability-and-breakpoints — `clean`

*core · both* · 依据：L3 Context assembly / 原文"Prefix 稳定性是显式不变量" + 决策#15/15b

缓存是设计里少数被当作经济性前提反复强调的点："Prefix 稳定性是显式不变量"，system prompt 与 tool schema 排序稳定、cache 断点由 loop 放置、provider 各自映射（Anthropic cache_control vs Gemini context cache 句柄），event 记录归一化 cache_read/cache_write token 供 budget 按真实计费口径记账。fold 是纯函数意味着 resume 后能字节级重建相同前缀，5m/1h TTL 内缓存甚至能跨进程重启存活——这是 event-sourcing 送的红利。tool 列表顺序耦合已被"排序稳定"显式覆盖。（环境块动态性问题单列为 friction 条目。）

### cache-ttl-variants-5m-1h — `extension`

*nice-to-have · claude-code* · 依据：决策#15b 能力抽象 + L3 Provider capabilities()

Anthropic 的 5m/1h TTL 选择是 cache_control 上的一个参数，决策#15b 已把 caching 定义为 provider 无关的可选 capability、由各 provider 映射到自家 API，TTL 只是该 capability 的一个字段。Gemini 隐式缓存无 TTL 控制的差异由"请求了不支持的能力时明确降级或报错，而不是静默忽略"覆盖。纯参数扩展。

### mid-session-tool-list-change-vs-cache — `extension`

*important · both* · 依据：决策#17 MCP schema 记录为 event + L3 "要么禁止要么显式换代"

MCP tools/list_changed 会改变进入 LLM 的 tool 列表，从而打爆 tool schema 前缀。决策#17 已把发现的 schema 和 list_changed 记录为 event（"它们进入 LLM 的 tool 列表，是影响 run 结果的外部输入"），L3 的"要么禁止要么显式换代"给出了处理框架：list_changed 触发一次显式换代，一次性 cache 重写后新前缀重新稳定。需要实现换代策略（何时应用变更、如何通知 fold），但两端机制都已预留。

### multimodal-input-and-at-file-refs — `extension`

*important · both* · 依据：L4 交互协议"协议预留：附件/图片消息类型" + 决策#4/#12/#15b

图片/截图/PDF 与 @-file 引用在 L4 交互协议里是"协议预留（原型不实现）：附件/图片消息类型"，按题设口径算 extension。输入侧走决策#4：附件内容 journal 成 InputReceived 类 event 再消费；provider 侧的"返回归一化"内部表示需扩展出 image/document content block，与决策#15b 的能力映射一致。隐藏成本是决策#12 的 JSONL per stream"可读可 diff"会被 base64 大 blob 破坏，但 EventStore 接口明确预留了换后端（SQLite/外置 blob）的位置，不构成结构障碍。

### long-context-1m-window — `extension`

*nice-to-have · both* · 依据：决策#15b + L3 Provider capabilities() + spec compaction.trigger_ratio

1M 窗口对 Gemini 是原生、对 Anthropic 是 beta header，正是决策#15b capabilities() 声明 + provider 各自映射的教科书用例。窗口大小需要成为 capability 的一部分（compaction trigger_ratio 0.8 的分母就是它），这是一个待补的接口字段而非结构问题。

### context-usage-indicator — `extension`

*important · both* · 依据：L3 Context assembly token 归一化记录 + L2 budget 关卡 + L4 交互协议

余量指示需要"当前上下文 token 数 / 窗口大小"：分子来自 LLM activity event 记录的归一化 token 计数（budget 关卡已从 event stream 统计同类数据），分母来自 provider capability。需要新增的只是把该派生值放进 L4 输出事件流供前端渲染（协议加字段），以及 Claude Code /context 式的分段占用明细——context assembly 作为一等组件天然知道各段大小。全部顺着现有抽象。

### prompt-too-long-auto-recovery — `extension`

*important · both* · 依据：L3 Agent loop + 决策#9 + ContextCompacted activity

API 返回 prompt too long 时自动 compact 后重试：这不是 L1 通用 retry（重发同一请求必然再失败），而是 loop 级恢复——捕获 LLM activity 的终态失败、跑一次 compaction activity、以新 fold 视图发起新 LLM activity。决策#9 的失败渲染枚举只覆盖了 tool 类失败（error tool_result），LLM 调用自身终态失败的 loop 行为未写明，但这是 agent loop 内的普通控制流，不触碰任何契约；compaction 作为 recorded activity 的机制现成可用。

### history-fold-across-compaction — `clean`

*core · both* · 依据：L1 Durability / 原文"State 是 event log 的纯 fold" + L3 ContextCompacted

fold 语义是良定义的：state = fold(apply, events)、apply 是纯函数（L1 原则 2），ContextCompacted 是流内的普通 event，"改变后续 fold 的结果"。同一份 log 可以派生两个视图——provider 请求视图（尊重 compaction event）和完整转录视图（全量 events，供 UI 显示压缩前历史）——都是纯派生，不需要额外持久状态。snapshot 只是 fold 的可弃缓存（决策#7），与 compaction event 的先后叠加无歧义。

### codex-agents-md-merge-and-history-compression — `clean`

*important · codex* · 依据：L3 Context assembly memory 层 + ContextCompacted event

Codex 的 AGENTS.md 层级合并（~/.codex 全局 → repo 根 → cwd 链）与设计的"memory 文件按目录层级合并"是同一机制的不同文件名与搜索路径；Codex 的 /compact 与自动历史压缩落在同一个 ContextCompacted recorded activity 模式上，压缩策略差异（保留哪些近期消息）只是 compaction activity 的参数/提示词。不需要任何 Codex 专属的框架分支。

### memory-write-back — `extension`

*nice-to-have · claude-code* · 依据：L2 effect pipeline / 决策#8 + 决策#4

Claude Code 的 # 快捷追加 CLAUDE.md 与自动 memory 目录：写 memory 文件就是一次普通的文件写副作用，走 L2 effect pipeline（hooks → permission → execute）并作为 activity 记录，与 edit_file 无异。触发入口（# 前缀消息）journal 成 event 后由 loop 或 frontend 解释。零新机制，只是新 tool/命令。

### thinking-block-roundtrip-in-context — `extension`

*important · both* · 依据：决策#15b / L3 Provider "返回归一化：thinking 块统一成一套内部表示"

Anthropic 与 Gemini 都要求后续请求原样回传带签名的 thinking 块，而决策#15b 把 thinking 块"统一成一套内部表示"——归一化表示必须无损携带 provider 专属的不透明字段（签名、原始 id），否则 fold 重建的请求会被 API 以 invalid signature 拒绝。这是实现归一化时给内部表示加一个 opaque provider_data 字段即可解决的事，设计没有禁止；model fallback 跨 provider 时签名不可转移，归一化表示反而让"丢弃/降级 thinking 块再重发"成为可能。属于需要在 M3 provider 归一化里写明的实现约束，非契约冲突。

### hook-output-context-injection — `extension`

*nice-to-have · claude-code* · 依据：决策#11 hooks v0 + L2 "hook 的消息作为 error tool_result" + 决策#8

Claude Code 的 hook additionalContext / PostToolUse 反馈注入模型上下文：决策#11 v0 只 observe + block，改写连同顺序与缓存问题一起推迟，属文档化推迟。且设计已有对称先例——"hook block → hook 的消息作为 error tool_result"，即 hook 输出进入模型视图的通道存在；成功路径的 feedback 注入同样经由 EffectResolved 记录、fold 时渲染，恢复时读记录值不重跑脚本的原则（决策#8）自动保住确定性。推迟的部分不藏结构问题。

### system-reminder-dynamic-injection — `extension`

*important · claude-code* · 依据：决策#4 输入语义 + L3 Context assembly "fold(event log) → provider 请求"

Claude Code 在用户 turn 里注入 system-reminder（todo 状态、文件被外部修改的通知、安全提示等）。在本设计里这类注入必须遵守决策#4：任何影响 run 结果的输入先 journal 成 event——todo 状态本身可从 event log 派生（纯 fold 内生成，无需额外 journal），外部文件变更通知则是外部输入需要 journal。fold 渲染时在消息流插入合成块即可，且不触碰 system prompt 前缀。需要新增"注入块"这一渲染概念和若干触发器，但与 journal+fold 契约完全同向。

## 工具与 workspace（18 项）

### Read tool (text/pagination/images/PDF/notebook) — `extension`

*core · both* · 依据：L3 Tools 与 workspace / 决策#13 / 决策#15b / 决策#12

read_file 在 M1 内置 tool 列表和 L3 '内置 tool 套件' 原文中。文本分页/notebook 解析是 tool 内部逻辑，顺着决策#13 'tool 定义是数据' 走。图片/PDF 需要 tool_result 携带多模态 content block：决策#15b 已把 provider 返回'统一成一套内部表示'并声明能力降级机制，给内部表示加 image block 类型是同一抽象的自然延伸，Gemini/Anthropic 都原生支持。隐藏成本是 JSONL event log（决策#12）里存 base64 图片会破坏'可读可 diff'属性，但 EventStore 接口后换存储是设计预留的出口，不构成结构问题。

### Edit tool (unique-match + freshness check, Codex apply_patch) — `clean`

*core · both* · 依据：L1 Durability 模型第2条 / 决策#7 / 决策#13

唯一匹配约束是 tool 内纯逻辑。freshness 检查（read-before-edit、文件被外部修改后拒绝编辑）需要的 per-file 状态天然可得：read/edit 都是 activity，其结果落 event log，而 'state 是 event log 的纯 fold'（L1 原则 2）意味着 '哪个文件在 seq N 被读过' 可从 fold 重建；mtime 对比是执行时的普通副作用读取，在 activity 内完成。'崩溃恢复绝不碰文件系统'（决策#7）与此无冲突——恢复后首次 edit 重新校验即可。apply_patch 只是同一 tool-as-data 槽位的另一实现。

### Write tool — `clean`

*core · both* · 依据：L3 Tools 与 workspace / 决策#10 / Roadmap M1

L3 原文明确列出 'file read/write/edit'。edit-class 类别标签（决策#10，tool 定义数据的一部分）让它直接接入 acceptEdits 等 permission mode 的工具面过滤，落进 M1 roadmap 即可。

### Glob/Grep tools — `clean`

*core · both* · 依据：L3 Tools 与 workspace / L3 Context assembly Tool 结果截断 / 决策#10

L3 原文明确列出 'glob/grep'。read-class 只读标签使其自动纳入 plan mode 的只读工具面（决策#10 'plan = 只读工具面'）；结果截断由 'per-tool 输出上限'（context assembly 的 tool_output_limit）现成覆盖。

### NotebookEdit tool — `extension`

*nice-to-have · claude-code* · 依据：决策#13 / L3 tool 定义是数据

新增一个 tool 定义数据文件 + 实现代码，.ipynb 的 cell 级读写是纯 tool 内逻辑，完全顺着决策#13 'tool 定义本身是数据'（description、schema、类别标签、per-tool 配置）走，打上 edit-class 标签即接入现有 mode 体系，不触碰任何层间契约。

### Bash tool (cwd persistence, timeout, output truncation) — `clean`

*core · both* · 依据：L1 Activity 语义 timeout/取消 / L3 tool 定义 per-tool 配置 / 决策#17

timeout 有双重现成机制：L3 tool 定义数据原文点名 'bash timeout' 为 per-tool 配置，L1 规定 'timeout 是 durable timer' 且 activity 有协作取消。输出截断由 tool_output_limit（'一条 cat large.json 不能毁掉上下文'）覆盖。cwd 跨调用持续性：cwd 变化记入 activity result event，state=fold 可重建，resume 无忧；env/shell 函数不承诺持久与 Claude Code 现状一致，若做长驻 shell 进程则按决策#17 '带外运行时状态、resume 后重新拉起' 的先例处理。

### Background bash (run_in_background + output polling + kill + wake-on-complete) — `extension`

*important · claude-code* · 依据：L0 actor/bus / L1 协作取消与 journal 输入（决策#4/#6）/ L4 fork-rewind barrier

各机制骨架都在：启动调用作为普通 activity 立即返回 task id；后台进程由专职 actor 持有（L0 actor 模型），输出增量走 bus（决策#3 ephemeral 的正版应用）；完成/退出按决策#4 journal 成输入 event（设计已列 'timer 到期' 为同类输入），loop 在 turn 边界被唤醒；kill 复用 L1 协作取消。需要新增的是后台任务 actor 与轮询 tool，并文档化两个语义空白：checkpoint barrier 的 '全部子 agent 静默' 要求需明确豁免后台任务（否则长驻 dev server 会让之后所有 turn 失去 rewind 点），其间的文件写入按 'bash 逃逸不在承诺内' 的既有精神处理；崩溃 resume 后进程已死，按决策#17 带外状态先例上浮。均不动 L0-L2 契约。

### OS sandbox (seccomp/landlock, bubblewrap/sandbox-exec, network sandbox + proxy) — `extension`

*core · both* · 依据：L3 Tools 与 workspace 'bash 沙箱等级' / L2 关卡[4] / 决策#8/#13

L3 明确把 'bash 沙箱等级' 列为 workspace 抽象的组成部分，per-tool 沙箱配置由决策#13 的 tool 定义数据承载。bubblewrap/seccomp/landlock 与网络代理都是 L2 execute 关卡内部的平台实现细节，实际执行等级可记入 EffectResolved（决策#8 的 event 本就携带全部关卡判定），审计路径现成。是体量不小的新增执行层代码，但完全在 execute 关卡内部，不触碰管线形状或任何已定决策。

### Sandbox escalation retry with approval (Codex-style) — `extension`

*core · codex* · 依据：L2 effect pipeline 关卡[2] / 决策#9 / L3 spec permissions.rules

Codex 的 '沙箱内失败 → 出沙箱重试需审批' 无需管线回跳：第一次 sandboxed 执行失败按决策#9 渲染为 error tool_result 且 loop 继续；升级重试是以 sandbox=off 参数重新提交的新 effect，完整再过一遍 hooks→permission→budget→execute，permission 关卡按 rules 判 ask 进入 WAITING_APPROVAL（L2 关卡[2] 原文机制）。permission rules 按沙箱等级参数匹配是现有 path pattern 匹配（spec 示例 'path: src/**'）的同类扩展。整条流程是现有关卡的组合，不是新机制。

### Workspace path boundary + --add-dir multi-root — `extension`

*important · both* · 依据：L3 Tools 与 workspace '路径边界' / L3 Context assembly prefix 稳定性 / L1 Checkpoint 与 workspace

'路径边界' 是 L3 workspace 抽象的原文组成。多目录需要把边界检查从单 root 扩展为前缀集合，permission rules 的 path pattern 天然支持多前缀。运行时 /add-dir 会改动 system prompt 环境块从而打爆 prompt cache prefix，但 L3 已给出出口：'任何会打爆 prefix 的操作要么禁止要么显式换代'——用户主动 add-dir 正是显式换代场景。需文档化的一点：per-turn git commit 只覆盖主 workspace，额外目录的 rewind 按 'bash 逃逸不在承诺内' 处理，与 Claude Code 行为一致。

### Checkpoint via in-workspace git: conflicts with agent/user git operations — `friction`

*important · claude-code* · 依据：决策#7 / L1 Checkpoint 与 workspace 'workspace 内 per-turn git commit' / L4 rewind 定义

障碍是决策#7 与 L1 原文 '实现为 workspace 内 per-turn git commit'——workspace 通常本身就是用户的 git repo，而 git status/commit/push 是 coding agent 的日常操作。具体失败场景：(1) agent 按用户要求 git commit 后，turn 边界的 checkpoint commit 落在同一 branch 上，用户 push 时把 harness 的检查点历史推上远端；(2) rewind = 'workspace 恢复到对应 commit'，在用户 repo 上即 reset --hard，会强移当前 branch ref，使 agent 期间创建的真实 commit 失去引用；(3) 并行 tool call 是常态（L3），harness 的 checkpoint commit 与 agent 的 git 命令竞争 .git/index.lock 随机失败。Claude Code 用 shadow repo（独立 GIT_DIR + 独立 index 共享 worktree）正是为了消除这三类冲突。修复不动 L0-L2 管线，但必须改写决策#7 的实现选择——典型 friction。

### Checkpoint on non-git directories and very large repos — `friction`

*important · claude-code* · 依据：决策#7 '不可随意删除' / L1 Checkpoint 与 workspace '便宜、可 diff'

同根因于决策#7 的 in-workspace git 选择。非 git 目录：per-turn commit 前提是 workspace 已是 git repo，否则必须对用户目录 git init——悄悄把普通目录变成 repo，.git 的出现会改变 IDE/构建工具行为，shadow GIT_DIR 方案则无此副作用。超大 repo：每 turn 全量 git add 扫描 worktree，monorepo 下每 turn 秒到十秒级延迟，设计原文 '便宜、可 diff' 低估了成本；且 checkpoint object 写进用户 repo 的 .git 会使其永久膨胀，而决策#7 又规定 workspace 快照 '不可随意删除'，连 gc 都不敢做。失败场景：用户在 5GB monorepo 上跑 agent，每个 turn 卡数秒且 .git 持续膨胀，唯一解法是改决策#7 换 shadow repo + 排除策略。

### Worktree isolation for parallel agents on same repo — `extension`

*important · both* · 依据：L3 Tools 与 workspace 'worktree 级隔离' / L4 fork/rewind barrier

L3 原文一句宣称 'worktree 级隔离支持多 agent 并行改文件'，schema 意义上已预留。需要新增：workspace 抽象的 per-agent-instance 实例化（spawn 时 git worktree add）、子 agent 结束后的合并/清理策略。git worktree 共享 object store 但 refs/index 独立，与 barrier 语义（'全部子 agent 静默'）配合自然。注意其 checkpoint ref 管理若叠加在 in-workspace git（前一条 friction）之上会更乱，但根因在决策#7 而非 worktree 机制本身，故本条判 extension。

### WebFetch / WebSearch tools — `clean`

*important · both* · 依据：L3 Tools 与 workspace / 决策#6 / L3 Context assembly 截断

L3 原文在内置套件里明确列出 'web fetch/search'。作为 activity 走 L2 管线，retry/backoff/rate-limit 由决策#6 'retry 是 activity 的通用属性' 覆盖，超大响应由 tool_output_limit 截断，URL 级管控由 permission rules 的参数匹配（同 path pattern）覆盖。全部机制现成。

### Plan file / scratchpad auxiliary workspace — `extension`

*important · both* · 依据：决策#10 / L3 Tools 与 workspace / L1 Checkpoint 与 workspace

plan mode 本体已由决策#10 完整覆盖（'plan = 只读工具面 + 计划指令注入 + ExitPlanMode 工具'，mode 跃迁是 event）。plan file/scratchpad 目录是 workspace 外的辅助根：路径边界需允许该目录（复用多 root 机制），并文档化它不受 per-turn commit 覆盖、rewind 不回滚 plan 文件（Claude Code 同样不回滚 scratchpad）。新增少量目录管理与配置代码，顺着 workspace 抽象走。

### Rewind granularity: code-only / conversation-only / both — `clean`

*important · claude-code* · 依据：L4 Session 管理 'rewind = fork + workspace 恢复' / 决策#7

L4 原文已把 rewind 定义为两个正交操作的显式组合：'rewind = fork + workspace 恢复到对应 commit'。三选一即暴露组合：只回对话 = fork 到 barrier 不恢复 workspace；只回代码 = workspace 恢复到 checkpoint commit 不动对话；两者 = 完整 rewind。barrier 粒度（turn 边界 + 子 agent 静默）与 Claude Code 的 per-user-message 回退粒度一致。机制正交且现成，只是 surface 层的命令组合。

### Fork workspace isolation (fork without rewind, parallel exploration) — `extension`

*nice-to-have · claude-code* · 依据：L4 Session 管理 fork / 决策#7 / L3 'worktree 级隔离'

L4 只说 'fork 复制 stream 闭包在 barrier 处的一致切面'，未定义 fork 出的新 run 的 workspace 指向——若与原 run 共用同一目录，两个活 run 并行推进会互踩文件。但补齐所需机制全部现成：barrier 处必有 workspace commit（决策#7），从该 commit 用 worktree（L3 已列）为新 run 实例化独立工作目录即可。属于语义空白 + 新增 fork 时的 workspace 实例化代码，不与任何决策冲突。

### Streaming tool output to frontend (live bash stdout) — `extension`

*important · both* · 依据：L3 Streaming 的持久化边界 / L1 ActivityCancelled{partial_output} / L4 交互协议

与 token delta 完全同构：增量走 bus、显式 ephemeral（设计自称这是 '原则 2 的正版应用而非违反'），持久化的只有最终 tool_result event；ActivityCancelled{partial_output} 已证明 activity 具备 partial output 概念，前端经 L4 交互协议订阅输出 topic 即可渲染。需新增的只是 activity 执行器向 bus publish stdout 增量的代码，零契约改动。

## 权限与 hooks（21 项）

### permission modes (default/acceptEdits/plan/bypassPermissions) + mode transitions — `clean`

*core · claude-code* · 依据：L2 effect pipeline mode 段 / 决策#10 / 决策#4 / L3 context assembly prefix 不变量

决策#10 把 mode 显式建模为数据：工具面过滤 + prompt 注入 + 跃迁规则，并逐一给出对应机制——plan =『只读工具面 + 计划指令注入 + 专用 ExitPlanMode 工具（其审批通过即触发 mode 跃迁，跃迁本身是 event）』，acceptEdits 依赖 tool 定义里的类别标签（edit/execute/read-class）。『hook 与 mode 的优先级明确：bypass 不跳过 hooks』直接对齐 Claude Code 语义。运行时 Shift+Tab 切 mode = 用户输入按决策#4 journal 后产生跃迁 event；mode 切换改变工具面/注入 prompt 会打 prefix，context assembly 段已规定此类变更『要么禁止要么显式换代』。落进 roadmap M2/M3 即可。

### PreToolUse hook: observe + block (exit code) — `clean`

*core · claude-code* · 依据：L2 关卡[1] / 决策#8 / 决策#9 / 决策#11 / roadmap M2

这正是 L2 关卡[1] 的 v0 能力：『v0: observe + block（exit code），不做改写』，hook block 面向模型渲染为 error tool_result（决策#9），执行记录进 EffectResolved 且恢复时不重跑 hook 脚本（决策#8）。roadmap M2 已排期 hooks（observe/block），spec 里也有 hooks.pre_tool_use 字段。

### permission rule syntax: Bash(git:*), Edit(src/**), mcp__server__tool, WebFetch(domain:...), deny>ask>allow precedence — `extension`

*core · claude-code* · 依据：L3 agent spec permissions 段 / 决策#8 / 决策#17

现有 spec schema 的 rules 只有 {tool, path, action} 三个字段（researcher.yaml 示例），要覆盖 Bash 的 command 前缀匹配（含复合命令拆分）、WebFetch 的 domain 匹配、MCP 的 server+tool 命名，需要给每类 tool 定义各自的 matcher 字段和一个 deny>ask>allow + 最长匹配的判定引擎。但『policy 是数据』（决策#8、原则4）和『permission rules 按文档化顺序拼接』的框架完全容纳新 matcher 种类——纯粹是丰富 rule schema 与匹配实现，不动任何层间契约。MCP tool 天然走同一管线（决策#17：McpToolCalled 是 activity），规则按名引用即可。

### five-layer settings: enterprise managed > CLI flags > local > project > user — `extension`

*important · both* · 依据：L3 配置分层段 / roadmap M4 / 决策#14

设计明说『配置分层从简：spec + 单个 project settings 文件两层起步……三层与更细的合并语义等真实冲突出现再加』，且已文档化了拼接顺序机制（local > project > spec，标量覆盖 + rules 按序拼接）。扩到五层就是往这个有序列表里加层：enterprise managed 的『不可被下层覆盖』= 该层的 deny/ask 在判定引擎里绝对优先，仍然是数据；CLI flags 由 L4 薄壳在加载时注入为一层，符合『core 是库、surface 是薄壳』（决策#14）。按判定口径这是明确推迟且路径已画出的 extension，没有藏结构性问题。

### 'always allow' — approval writes rule back to settings, runtime policy update takes immediate effect — `extension`

*important · claude-code* · 依据：L1 journal 一切输入 / 决策#4 / L1 fold 模型 / L2 关卡[2]

审批应答本来就是 journaled event（L1『审批应答以 event 到达后继续』、决策#4），给应答加 scope 字段（once/session/always），有效 policy = 静态配置 + fold 出的运行时增量，正是『state = fold(events)』的直接应用——resume 后增量规则随 fold 自动恢复，permission 关卡读 folded policy 即可即时生效。写回 settings 文件是一次普通副作用，由 L4 或走管线执行。唯一要定义的小语义是外部手改 settings 文件与 fold 增量的合并顺序，但不构成结构障碍。

### rich approval UI: diff preview, edit-input-then-approve, approve-and-remember — `extension`

*important · claude-code* · 依据：L4 交互协议 / 决策#9 / L3 compaction 是 recorded activity

diff 预览是 frontend 从 ApprovalRequested 携带的 effect payload（edit tool 的 old/new）本地计算，交互协议已列出 ApprovalRequested 与 permission 判定的输出事件流；『批准并记住』见 always-allow 条目。『修改输入后批准』需要一个明确模式：应答 event 携带 updatedInput，loop 以『原 effect 记 deny + 合成新 effect 重走完整管线（pre-hook 重新审视改后输入）』落地——compaction 已证明 loop 可以合成非模型发起的 effect，tool_result 仍按原 tool_use id 配对（决策#9），causation 链解释两条 EffectResolved 对一个 tool_use。全程顺着现有抽象，不改契约，但这个重入模式需要被显式设计而非顺手写出。

### programmatic/headless approval (canUseTool callback, permission-prompt-tool, non-interactive fallback) — `clean`

*important · both* · 依据：L1 挂起是显式状态 / L2 关卡[2] / L4 交互协议 / 原则5

设计把审批彻底事件化：ask ⇒ ApprovalRequested event + WAITING_APPROVAL 显式状态，『等几分钟或几天成本相同，进程死了也一样』；而『frontend 是普通 actor：订阅输出 topic，向 run 发输入』（原则5）意味着 SDK 回调或 permission-prompt-tool 就是一个订阅 ApprovalRequested 并发应答 event 的 actor，不需要任何特权通道。headless 单发要么 durable park 等待、要么挂一个自动应答 deny 的 actor，两者都是现有机制的直接组合。

### parallel tool calls × permission (allow runs concurrently, ask serialized without blocking allowed calls) — `clean`

*important · both* · 依据：L3 agent loop 并行 tool call 段 / roadmap M3

L3 agent loop 原文直接规定了这个语义：『一条 assistant 消息含 N 个 tool_use 时，每个 call 独立过管线；判定为 allow 的并发执行，判定为 ask 的按序等审批（审批挂起不阻塞已放行的 call）；完成 event 按到达顺序落盘』。这是 Claude Code 并行工具调用审批行为的逐句对应，roadmap M3 已排期。

### subagent permission inheritance & approval routing to human — `clean`

*important · claude-code* · 依据：L3 multi-agent 审批路由/权限继承 / roadmap M5

Multi-agent 节明文两条：『权限继承：child 的有效权限 = child spec ∩ parent 有效权限，子 agent 不能越过 parent 的边界』和『审批路由：child 的 ask 沿 correlation id 冒泡到 session 的 frontend——审批的永远是人，不是 parent agent』。∩ 语义与事件化审批 + correlation 链直接给出实现路径，M5 已排期。

### PreToolUse updatedInput (hook mutation of tool input) — `extension`

*important · claude-code* · 依据：决策#11 / L2 关卡顺序 / 决策#8 / L3 prefix 稳定性

决策#11 明确推迟 mutation 并点名了原因（顺序/缓存/重放问题），按判定口径先算 extension；关键是论证推迟处没藏结构问题：管线顺序 hooks(pre)→permission 恰好使改写发生在 permission 判定之前，判定的是改写后的输入，与 Claude Code 语义一致；EffectResolved 记录 {原输入, 改写链, 终输入}，恢复读记录值不重跑脚本（决策#8）；改写只影响执行参数，assistant 消息里的 tool_use 原文不变，不触碰 prefix 缓存不变量。三个被点名的问题在现契约内都有答案，长出来是顺的。

### PreToolUse permissionDecision short-circuit (hook returns allow/deny, skips ask) — `extension`

*important · claude-code* · 依据：L2 关卡[1]→[2] 顺序 / 决策#8 / L2 mode 段优先级句

pre-hook 关卡在 permission 关卡之前，hook 输出对下游关卡可见是管线内部的数据流；给 hook 输出协议加 permissionDecision 字段、permission 关卡尊重之，判定同样记入那条 EffectResolved。设计里『hook 与 mode 的优先级明确』一句说明关卡间优先级本来就是要显式定义的内容。纯管线内扩展，不动 L2 对外契约。

### PostToolUse hook feedback appended for the model — `extension`

*important · claude-code* · 依据：L2 关卡[5] / 决策#9 / 决策#8

关卡[5] 已在管线上，缺的是『hook 输出成为模型可见内容』这条通路：决策#9 已经为『给模型的错误』定义了 tool_result 渲染通道，post-hook feedback 附加到 tool_result 或作为 systemMessage 是同一通道的自然延伸；结果记录在 EffectResolved 里，fold→context assembly 渲染时带上即可，resume 后可精确重建。注意其持久性边界问题单列在 hook execution durability 条目。

### hook execution durability & EffectResolved write timing (crash window) — `friction`

*important · claude-code* · 依据：决策#8 单条 EffectResolved / 决策#11 hooks 不是 effect / 决策#6 in-doubt / L2『恢复不重跑 hook 脚本』

决策#8 规定整条管线对一个 effect 只产生一条 EffectResolved，『携带全部关卡判定（hook 结果、permission 判定、budget 判定）』——若包含 post-hook 结果，这条事件必须等 post-hooks 全部完成才能落盘，则 pre 关卡的自动放行判定在 execute 期间只存在于内存；而 hooks 又被决策#11 排除在 activity 之外，没有 Started/Completed 双落盘，决策#6 的 in-doubt 检测覆盖不到它们。失败场景：post-hook（如 formatter 脚本）执行到一半进程被 kill——activity 已 Completed，EffectResolved 永远没写；恢复路径规定『不重跑 hook 脚本』，于是 pre-hook 与 permission 的判定记录永久缺失，post-hook 处于既不能重跑（有副作用）也无任何事件可标记 in-doubt 的悬空状态。修法要么拆成 pre/post 两条 event（局部推翻单条事件的决策初衷），要么给 hook 引入 activity 式记录（局部推翻决策#11），设计需二选一。

### UserPromptSubmit hook (inject context / block user input) — `friction`

*important · claude-code* · 依据：原则3 / 决策#11 / 决策#8 / 决策#4 / L1 fold 模型

原则3 与决策#11 把 hooks 定位为『这条管线上的关卡，不是四个子系统』『管线机件不是 effect』，且 hook 执行的唯一记录通道是 EffectResolved——但用户 prompt 提交不是副作用、不进 effect 管线，UserPromptSubmit hook 在现架构里没有执行点也没有记录通道。要支持必须新增『管线外 hook site』概念：在 InputReceived journal 之后、turn 边界消费之前执行脚本，其判定与注入内容必须落成新种类的 event（注入内容改变 fold→context assembly 的结果，按决策#4 精神必须先 journal），并把『恢复不重跑 hook』的保证复制到新通道。失败场景：团队配置 UserPromptSubmit 注入当前 ticket 上下文——若注入结果不入 log，crash 后 resume 的 fold 重建出的对话与实际发给模型的内容不一致，直接破坏『state 是 event log 的纯 fold』不变量。这是对决策#11/原则3 hook 定位的局部修订，而非顺着抽象自然长出。

### Stop / SubagentStop hook (refuse stop, force loop to continue) — `friction`

*important · claude-code* · 依据：原则3 / 决策#11 / L3 agent loop 终止路径 / 设计目标『由组合得出而不是逐个特判』

同 UserPromptSubmit 的根因：『run 结束』不是 effect，Stop hook 无处可挂。且它比其它 lifecycle hook 更深地介入 loop 契约——hook block 后 loop 必须带着 hook 消息继续跑，等于合成了一个非用户来源的 turn 输入，现有 event 语汇（InputReceived 类）没有这个概念，而这个 verdict 不 journal 的话，resume 后无法解释 run 为什么没有在 model finish 处结束。可以把『停止』建模为伪 effect 走管线让 pre-hook 拦截，但这把管线的定义域从『所有副作用』悄悄扩成『所有需要关卡的动作』，同样是改原则3 的口径。失败场景：配置 Stop hook 强制『测试通过才许停』——现设计 agent loop 的终止条件与 L2 管线无接口，实现只能在 loop 里硬编码一个特判 hook 点，恰是设计自己反对的『逐个特判实现』。

### SessionStart / SessionEnd hooks — `extension`

*important · claude-code* · 依据：L1 snapshot-resume / 决策#4 / L3 context assembly / 原则6

执行点清晰（run 启动与 resume 之后、loop 开始之前；结束时），SessionEnd 是 observe-only 不影响 run，几乎零成本；SessionStart 的上下文注入需要 journal 成 event 进 fold（同决策#4 的『影响 run 结果的输入先落盘』），注入位置放在消息流而非 system prompt 就不碰 prefix 不变量。resume 时跑 SessionStart 是一次全新的、被记录的执行而非重放，不违反『恢复不重跑 hook』——fold 保持纯，副作用发生在 fold 完成后的 loop 里。前提是先有 lifecycle hook site 的通用概念（见 UserPromptSubmit 条目），但 SessionStart 本身的接入是顺的。

### PreCompact hook — `clean`

*nice-to-have · claude-code* · 依据：L3『Compaction 是 recorded activity』/ 原则3 / L2 关卡[1]

这是 lifecycle hooks 里唯一天然有挂点的：设计明确『Compaction 是 recorded activity——它本身是一次 LLM 调用（非确定性副作用）』，而原则3 规定所有副作用流经 effect 管线，所以 compaction 会经过关卡[1]，PreCompact 就是一个按 effect 类型（而非 tool 名）匹配的 pre-hook。只需 hook 配置的 matcher 支持 effect 类型过滤，记录、恢复语义全部复用 EffectResolved 现有机制。

### Notification hook — `extension`

*nice-to-have · claude-code* · 依据：L4 交互协议输出事件流 / 原则5 / L1 显式等待状态

通知时机（等审批、等输入、空闲）在设计里都是显式 event/状态（ApprovalRequested、WAITING_INPUT），而原则5 说 frontend 是订阅输出 topic 的普通 actor——Notification hook 就是一个订阅这些 event 并执行脚本的 actor，observe-only、输出不影响 run，因此不需要 journal，也不需要碰管线。只差把 hook 配置装配成订阅者的胶水代码。

### hook timeout, parallel execution, async hooks — `extension`

*important · claude-code* · 依据：L2 关卡[1]/决策#8 / L1 timeout 段 / 决策#4 / L3 steering

关卡内并发执行 N 个匹配脚本并合并判定是 gate 内部实现；hook timeout 用普通 wait_for 而非 durable timer 即可——L1『绝不在关卡代码里读墙钟』针对的是重建时会重算的判定，而 hook 结果一次性记录进 EffectResolved、恢复不重跑，墙钟超时无重放风险。async hook 若输出需回流模型，就是一个外部输入：按决策#4 journal 成 event、turn 边界消费，与 steering 完全同构。都不动契约。

### hook merging from config layers and plugins — `extension`

*important · claude-code* · 依据：L3 配置分层段 / L3 agent spec hooks 字段 / roadmap M4

配置分层已定义『标量覆盖、列表按文档化顺序拼接』的合并机制，hooks 数组与 permission rules 同样按层拼接即可；spec 里 hooks 已是数据（hooks.pre_tool_use 列表）。plugin 概念在设计中完全缺席，但 plugin 对 hooks 的贡献本质是又一个配置片段来源，落在同一拼接框架上——需要新增 plugin 发现/加载组件（属 M4 生态接入的自然延伸），不需要动 L0-L2。

### Codex approval policy (untrusted/on-failure/on-request/never) with sandbox-escape approval flow — `friction`

*core · codex* · 依据：L2 决策#8 四关卡线性顺序 / 决策#10 mode 三要素 / 决策#9 / L3 workspace『bash 沙箱等级』

untrusted（安全命令白名单放行、其余 ask）、on-request（模型带 escalation 参数请求，规则按参数匹配 ask）、never（无 ask）都能用决策#10 的 mode 数据 + rules 表达；卡住的是 on-failure：命令先在沙箱内执行、失败后才请求出沙箱重试的审批。决策#8 的四关卡是线性单向的（permission 在 execute 之前判定一次），没有『执行失败后回到审批关卡』的回边；决策#10 的 mode 三要素（工具面过滤 + prompt 注入 + 跃迁规则）也表达不了这种执行后审批策略。失败场景：on-failure 下 `cargo build` 在只读沙箱内写缓存失败，期望弹出『retry without sandbox』审批，但现设计只能按决策#9 渲染 error tool_result 给模型完事。可行修法是 loop 检测沙箱类失败后合成一个升级参数的新 effect 重走管线（产生第二条 EffectResolved 对同一 tool_use，需定义审计语义并抑制中间 tool_result），或在 execute 关卡内嵌第二次 ApprovalRequested——两者都要局部修改管线契约或关卡职责划分。

## 多 agent（18 项）

### subagent-definition-declarative — `clean`

*core · both* · 依据：L3 Agent spec（agents:/tools:/model: 字段）/ 决策#13 / 决策#15b

设计的 agent 完全由声明式 spec（YAML → pydantic）定义，spec 里已有 tools 白名单、model（provider/id/thinking budget，即 effort 类能力走 15b 能力抽象）、agents 子 agent 白名单、permissions。Claude Code 的 markdown+frontmatter 只是同一份数据的另一种载体，加一个 frontmatter loader 映射到同一 pydantic model 即可，决策#13『spec 是数据』直接覆盖。『agent instance = spec + 运行时输入（task、correlation id、parent）』的模板/实例分离也与子 agent 用法吻合。

### sdk-programmatic-agent-definition — `clean`

*important · both* · 依据：决策#14 / 设计原则 5 / L3 Agent spec

决策#14『core 是库，CLI/headless/server 是薄壳』意味着程序化入口是一等形态而非附加物；spec 是 pydantic model，SDK 用户直接构造 model 实例等价于加载 YAML。不存在特权 frontend（原则 5），所以程序化定义的 agent 与文件定义的 agent 走完全相同的 spawn/权限/审批路径。

### spawn-await-fanout — `clean`

*core · both* · 依据：L3 Multi-agent 三模式 / L2 effect pipeline / Roadmap M5

L3 Multi-agent 明确列出三种模式之一：『spawn/await（子 agent 作为 activity，可扇出）』，spawn 作为 effect 过 L2 管线、作为 activity 拿到决策#6 的 retry/取消/in-doubt 语义。并行 tool call 一节已定义 N 个 call 独立过管线、allow 的并发执行，扇出的并发骨架现成。Roadmap M5 已排期。

### background-subagents-notify-wake — `extension`

*important · claude-code* · 依据：L3 Multi-agent 三模式 / 决策#4 / L1『挂起是显式状态』

设计列的三模式里没有 detached spawn（父先返回、子完成后唤醒），需要新增第四种模式：spawn effect 立即返回 tool_result{task_id}，child 作为独立 actor 继续跑，完成消息按决策#4 journal 成 event、在父的 turn 边界被消费——这正是设计为 steering/timer 已铺好的输入路径。父在 WAITING_INPUT 显式挂起状态被子完成事件唤醒也正是 L1『durable park』的用法。全程不动 L0-L2 契约，只是把『子 agent 作为 activity』的一次性框架换成『actor + journaled 完成事件』的组合。

### resume-child-conversation — `extension`

*important · claude-code* · 依据：L3 Multi-agent『子 agent 作为 activity』/ L0 actor+mailbox / L4 session 定义

L3 把子 agent 框成一次性 activity（『只有符合 result contract 的最终报告回流 parent』），续对话需要 child 是可持续会话实体。但底座天然支持：child 本来就是有自己 stream 的 actor（L0），session 闭包含子 agent stream（L4），resume 是 per-stream 的；把『每次 SendMessage 交换』建模为一个 activity、child actor 跨交换存活即可，决策#6 的 Started/Completed 按交换粒度记录不冲突。需要新增 SendMessage effect 与 child 生命周期管理，属于顺着 actor 抽象长出来的组件。

### agent-teams-peer-messaging-shared-tasklist — `extension`

*important · claude-code* · 依据：L3 Multi-agent pub/sub 模式 / L0 Bus / 决策#3、#4 / L3 context assembly prefix 不变量

第三种模式『pub/sub 协作（blackboard topic）』就是为 peer 协作设计的；L0 的 send(to,msg) 点对点 + 决策#4『输入先 journal 再消费』给了 peer 直连消息的正确性基础。共享任务列表不能是共享可变状态（决策#3 持久状态只有 log 和 workspace），但建成一个 task-list actor（状态 = 自己 stream 的 fold）是教科书式的 actor 组合。需要补 teammate roster 的运行时注入——为不打爆 prefix 稳定性不变量，应走消息/工具而非 system prompt，实现上有讲究但不碰契约。

### handoff — `clean`

*important · codex* · 依据：L3 Multi-agent 三模式 / L4 Session 管理 / Roadmap M5

『handoff（移交后退出）』被明确列为三种模式之一，Roadmap M5 排期。session = correlation id + stream 闭包，接手方共享同一 correlation，frontend 作为普通 actor 订阅输出 topic 不需要感知移交；A→B→A 的回传就是再一次 handoff。现有机制直接覆盖。

### approval-bubbling-permission-intersection — `clean`

*core · both* · 依据：L3 Multi-agent 审批路由/权限继承 / L2 关卡[2] / Roadmap M5

两条都是原文明确设计：『child 的 ask 沿 correlation id 冒泡到 session 的 frontend——审批的永远是人，不是 parent agent』和『child 的有效权限 = child spec ∩ parent 有效权限』。审批挂起是 L1 显式等待状态（WAITING_APPROVAL），应答 journal 后继续，跨进程死亡也不丢（决策#4/#5）。唯一待细化点是交集语义对 mode（决策#10，mode 是行为数据不是 rule 集）如何定义，属 M5 细化而非契约修改。

### nesting-depth-concurrency-limits — `extension`

*important · both* · 依据：L2 关卡[3] Budget / L3 Multi-agent 可审计性 / L4 session 定义

spawn 是过 L2 管线的 effect，budget 关卡（关卡[3]）天然是放深度/并发判定的位置；嵌套深度可从 causation/correlation 链直接推导（L3 保证链路完整），spec 的 agents 白名单已静态约束扇出面。并发上限需要跨 stream 的会话级计数，session = correlation 闭包给出了聚合范围。是顺着关卡抽象加判定逻辑，不动契约。

### shared-token-budget-pool — `extension`

*important · both* · 依据：L2 关卡[3] / L3 spec limits / L3 context assembly（token 归一化记账）/ 决策#8

现设计 budget 关卡『turns/tokens/cost 从 event stream 统计』是 per-run 口径（spec limits.max_tokens_total 也是单 agent 的）；跨子 agent 池化需要会话级 ledger。并发 children 同时扣减需要单一序列化点以防双花——actor 模型免费提供（一个 budget actor，判定经 mailbox 串行），判定结果照旧记进各 effect 的 EffectResolved（决策#8），LLM activity 已归一化记录 cache_read/write token 使记账口径现成。新组件，但完全顺着 actor + 关卡抽象。

### per-agent-worktree-isolation — `extension`

*important · both* · 依据：L3 Tools 与 workspace / 决策#7 / L4 fork/rewind barrier 定义

隔离本身是原文承诺：『worktree 级隔离支持多 agent 并行改文件』（L3 Tools 与 workspace）。但只有这一句意图，子 agent 完工后结果 merge 回父 workspace、以及 workspace 快照（决策#7 per-turn git commit）在多 worktree 下的打点协调（barrier 需记录一组 worktree HEAD 而非单个 commit）都需要真实的新组件。好在 worktree 共享同一 git 对象库，merge/commit 都是普通 git 操作，不碰 L1 契约。

### structured-result-contract-json-schema — `extension`

*important · both* · 依据：L3 Multi-agent result contract / 决策#13 / 决策#6 retry

设计现状是软约定：『contract 在子 agent spec 的 description/输出约定里声明』，没有 schema 强制。但强制校验是顺手的延伸：tool 定义是携带 JSON schema 的数据（决策#13），给子 agent spec 加 output_schema 字段、在 spawn activity Completed 前校验、不合格走 activity 通用 retry（决策#6）即可；甚至可以像 Claude Code SDK 那样用一个 report 工具收口输出。不动任何契约。

### deterministic-workflow-orchestration — `friction`

*important · both* · 依据：决策#5 / L1 Durability 模型第 2、3 条 / 设计原则 1、6

障碍是决策#5（拒绝 Temporal 式 code replay）与 L1 契约『State 是 event log 的纯 fold』+『turn 边界 snapshot』的组合：这套 durability 是为『状态=消息列表+turn 计数+待处理 tool call』的 agent loop 量身定做的，而确定性编排脚本的状态是 Python 控制流位置和局部变量——既不是 event 的 fold，也没有天然 turn 边界。原则 1 宣称 workflow 也是统一模型下的 actor，但没说清它的 snapshot 边界是什么。失败场景：编排脚本扇出 5 个子 agent、await 全部后再跑 merge agent，进程在 3/5 完成时崩溃——按原则 6 只能走 snapshot+补放恢复，线性脚本无 snapshot 可打，除非作者把 workflow 手写成显式状态机（每步完成落 event、fold 出『当前在第几步』）。能做，但把 replay 引擎省下的成本转嫁给了每个 workflow 作者，是决策#5 未言明的隐藏代价。

### subagent-transcript-observability-correlation — `clean`

*important · both* · 依据：L0 Envelope / L3 Multi-agent 可审计性 / L4 Observability

这是设计的强项：每个 agent 一个 stream、per-stream 完整可审计、causation/correlation 链路完整是 L3 的明文保证；L4 Observability 明确 inspect CLI 渲染『子 agent 树（correlation/causation）、token/cost 消耗』。Envelope 从 L0 起就携带 causation_id/correlation_id，父子关联不是事后拼接而是内建。

### cascading-cancellation — `clean`

*core · both* · 依据：决策#6 / L1 Activity 语义（协作取消）/ L4 session 定义

协作取消是 activity 的一等能力（决策#6，『activity 持有 cancel signal』），而子 agent 就是父的一个 activity——取消 spawn activity 即取消 child，child 再对自己的 in-flight activity（含它的子 spawn）递归应用同一机制，级联是机制的直接组合。session = correlation 闭包还给出了『整棵树』的精确目标集，每层取消都以 ActivityCancelled{partial_output} 落盘可审计。M5 落地即可。

### multi-agent-session-fork-rewind — `friction`

*nice-to-have · claude-code* · 依据：L4 Session 管理 fork/rewind / 决策#7『只在显式 barrier 打点』/ L3 Multi-agent 三模式的组合效应

障碍是 L4 已定契约：『fork/rewind 只发生在 checkpoint barrier 上（turn 边界 + 全部子 agent 静默 + workspace commit 存在）』且明确『任意 seq N 处的 fork 不提供』，workspace 快照也『只在显式 barrier 打点』。对一次性 spawn/await 这够用（子 agent 很快静默），但与 background 子 agent、持续会话 child、agent teams 组合后，『全部子 agent 静默』的时刻可能长期不存在。失败场景：3 个 teammate 并行干一小时，用户想 rewind 到 20 分钟前——期间无任何全树静默点，一个 barrier 都没留下，整段时间不可回退。要支持需局部松动 barrier 契约（per-subtree barrier 或强制 quiesce 协议），属于对已定契约的修改。

### dynamic-runtime-agent-definition — `extension`

*nice-to-have · claude-code* · 依据：L3 context assembly prefix 不变量 / L3 Agent spec agents 白名单 / 决策#13

中途新定义一个子 agent 并 spawn，要更新 system prompt 里的子 agent 目录（『模型不知道 summarizer 存在就永远不会 spawn 它』），会撞 prefix 稳定性不变量——但设计已给出出口：『任何会打爆 prefix 的操作要么禁止要么显式换代』，付一次 cache 换代成本即可。spec 的 agents 白名单是数据，运行时扩展白名单走配置变更路径；新 spec 加载复用现有 pydantic 校验。不推翻决策，只需实现『显式换代』这条已预案的路径。

### subagent-directory-prompt-injection — `clean`

*core · both* · 依据：L3 Context assembly（system prompt 拼装顺序）/ L3 Agent spec description 字段

设计原文把它当作前提条件明确写进 context assembly 的拼装顺序：『tool/skill/子 agent 目录（模型不知道 summarizer 存在就永远不会 spawn 它——目录注入是 multi-agent 可用的前提）』。目录来自 spec 的 agents 白名单与各子 agent spec 的 description，拼装顺序固定以保 prefix 稳定。机制直接覆盖。

## Session 与 surfaces（19 项）

### session list / resume / continue — `clean`

*core · both* · 依据：L4 Session 管理 / L1 决策#5 / Roadmap M3

L4 Session 管理明确定义 session = correlation id + stream 闭包，list = 枚举 store，resume = snapshot + fold（L1 决策#5），且 M3 已排期 'session list/resume'。--continue 只是 'resume 最近一个 session' 的语法糖，靠 store 枚举 + 时间戳即可。挂起中的 session（WAITING_APPROVAL/WAITING_INPUT 是显式 event 状态）resume 后能原位继续等待，这正是设计的核心卖点。

### cross-version resume (weekly CLI upgrades) — `friction`

*core · both* · 依据：决策#18 / 非目标第 2 条 / L1 决策#5、#7

障碍是决策#18 本身：'RunStarted 记版本，不匹配拒绝 resume'，且非目标一节写明 'event schema 变更即丢弃旧 run 日志重跑，不做 migration'。失败场景：用户周更 CLI 后，所有历史 session（包括挂着审批等了三天的长任务）集体拒绝恢复——对真实产品这不可接受，Claude Code/Codex 都保证旧会话可 resume。后改需要推翻#18、给 event 建立版本化 + 读取路径 upcaster 纪律；好消息是架构对此友好——state 是 event log 的纯 fold、snapshot 是可弃缓存（决策#7），只需 upcast event 不需迁移 state，且 RunStarted 已记录版本号可作 upcast 起点。但原型期'随意改 schema'的自由一旦行使过，产品化后第一次兼容性承诺就要为所有历史变更补写迁移，成本随拖延递增。

### rewind granularity: code-only / conversation-only / both — `clean`

*important · claude-code* · 依据：L1 决策#7 / L4 fork-rewind barrier / L3 context assembly 'ContextCompacted'

决策#7 把两种快照严格分离：对话 state 是 fold 到某 seq 的派生物，workspace 是 barrier 处的 git commit。'两者' = 设计原文的 rewind（fork + workspace 恢复到对应 commit）；'仅对话' = 只 fork 不动文件；'仅代码' = 只 checkout barrier commit 不动 stream——三种粒度就是两个正交 primitive 的组合选择，无需新契约。且 L3 明确 compaction 是 recorded event、'跨 compaction 边界的 fork/rewind 语义因此是良定义的'，barrier（turn 边界）也与 Claude Code /rewind 按用户消息打点的 UX 对齐。

### session teleport (local <-> cloud) & remote container sessions — `extension`

*important · both* · 依据：决策#3、#17、#15c / L1 Checkpoint 与 workspace

决策#3 '持久状态只有 event log 和 workspace 两处' 使迁移面被精确枚举：搬 JSONL stream 闭包 + git workspace 即可在另一台机器 resume。MCP server 是带外运行时状态、resume 后重新拉起（决策#17），凭据只走环境变量不入 log（决策#15c），都为跨机迁移扫清了障碍；bash 逃逸 workspace 的副作用明确不在承诺内，与真实产品口径一致。需要新增的是打包/传输/远端 resume 编排组件，纯 L4 增量。唯一牵连是决策#18 要求本地与云端代码版本一致，这个成本记在 cross-version resume 条目下。

### multiple concurrent sessions on one workspace — `friction`

*important · both* · 依据：决策#7 / L1 'workspace 内 per-turn git commit' / L3 Tools 与 workspace

障碍是决策#7 的实现载体：workspace 快照'实现为 workspace 内 per-turn git commit'，隐含单写者假设。失败场景：同一目录开两个 session（Claude Code 用户的日常操作），A、B 的 per-turn commit 在同一条 git 历史上交错，A rewind 到自己的 barrier commit 会连带回滚 B 之后的所有修改，B 的 barrier 也不再是自己 run 的一致切面；同时 harness commit 污染用户自己的分支历史、与用户手工 git 操作互相踩踏。L3 提到的 'worktree 级隔离'只服务子 agent 并行，不解决用户明确想要两个 session 共享同一工作目录的场景。修法（每 session 独立 shadow git dir / 快照栈，Claude Code 即如此）语义上仍是'workspace 快照是一等状态'，但需局部改写决策#7 的机制并重新定义并发下的 rewind 承诺。

### headless -p mode, json/stream-json output, --resume — `clean`

*core · both* · 依据：决策#14 / L4 运行形态、交互协议 / Roadmap M1、M3

决策#14 'core 是库，CLI/headless/server 是薄壳'，M1 即含最小 CLI、L4 运行形态明确列出 headless 单发。json/stream-json 只是 frontend actor 对输出事件流（turn 事件、EffectResolved、assistant message）的另一种序列化渲染；token delta 走 bus 对进程内 frontend 直接可见，支持 partial streaming。--resume 续跑复用 M3 的 session resume 机制，headless 与交互式共享同一 core 路径，无特权 frontend（原则 5）。

### Agent SDK: in-process query, canUseTool callback approval, in-process custom tools, hook callbacks — `extension`

*core · both* · 依据：原则 4、5 / 决策#13、#17 / L2 关卡[2] ask 语义 / 决策#11

core-是-库（原则 5）使进程内 query 天然成立；spec 是 pydantic model，程序化构造绕过 YAML 无碍。canUseTool 式审批映射干净：L2 的 ask ⇒ ApprovalRequested event ⇒ 应答以 event 到达——SDK 回调就是一个立即应答的 frontend actor，应答照常 journal（决策#4），审计不破。进程内函数 tool 需要澄清'tool 定义是数据'（决策#13）的边界：定义（schema/类别标签）是数据，executor 本就是代码——内置 tool 已是'数据文件 + 包内实现'的配对，SDK 只需一个 name→callable 的 executor 注册表；resume 时宿主程序须先重注册 callable，决策#17 的 MCP '带外运行时状态'已提供同类先例。审批时改写输入（updatedInput）与 hook mutation 同属决策#11 推迟的改写域，schema 层面可在应答 event 里携带修改后输入并记入 EffectResolved，不破契约。

### server mode (HTTP/WS) — `clean`

*important · both* · 依据：L4 运行形态 / 决策#2、#14 / Roadmap M5

L4 运行形态明确 'server（HTTP/WS 暴露同一协议）' 是薄壳，M5 已排期 'server 壳'。交互协议本身以 event 流定义（turn 事件、ApprovalRequested、TurnDiscarded），frontend 是订阅输出 topic 的普通 actor，WS 桥接只是协议搬运。决策#2 也预留了'分布式化是换 transport'的边界。

### multi-client concurrent attach (phone + desktop on same session) — `extension`

*important · both* · 依据：原则 5 / L0 bus publish / 决策#4 / L4 交互协议

原则 5 '不存在特权 frontend' + L0 bus 的 publish 扇出使 N 个 frontend actor 同看同控天然成立；两端输入都按决策#4 journal 后串行进 stream，不会打架。需要新增的是中途 attach 的 catch-up 协议：从 event log 回放历史（log 即 source of truth，直接支持）再切到 live 订阅，以及'同一 ApprovalRequested 先答者生效'的应用层去重（Envelope.id 幂等只防同一 command 重试，不防两个客户端各发一条应答）。都是顺着现有抽象的 L4 代码，不动契约。

### IDE integration (diff view, editor selection as context, @-references) — `extension`

*important · both* · 依据：L4 交互协议'协议预留' / 决策#7 / L3 context assembly prefix 不变量

IDE 插件就是又一个 frontend actor。diff 视图直接受益于决策#7 的 per-turn git commit（原文'便宜、可 diff'）；editor selection / @ 文件引用是带结构 payload 的输入 event，L4 交互协议已'预留附件/图片消息类型'，说明消息类型系统本就打算扩展；注入到上下文由 L3 context assembly 承接，注意不打破 prefix 稳定不变量即可（selection 属于消息层不属于 system prompt 层，天然安全）。全部是协议与组件增量。

### GitHub Actions / @claude mention triggering new runs — `clean`

*important · both* · 依据：L4 Scheduler 与 triggers / L0 Envelope 幂等 / Roadmap M5

L4 Scheduler 与 triggers 已直接设计此路径：'webhook 触发 = server 壳收到请求后发同一条 RunAgent command'，且 command 按 Envelope.id 幂等（L0），webhook 平台的重试不会拉起重复 run——设计原文点名了这个坑。CI 内运行复用 headless 模式。M5 已排期 scheduler。

### webhook / follow-up events flowing into an existing (dormant) session — `extension`

*important · claude-code* · 依据：决策#4、#5 / L1 'InputReceived append 进该 run 的 stream' / L4 Session 管理

设计只写了 webhook 拉起新 run，流入已有 session 需要'唤醒休眠 session'编排：向休眠 run 的 stream append 一条 InputReceived event，再触发 resume（snapshot + fold seq>N 会把它折进状态，loop 在 turn 边界消费）。这恰好被决策#4'一切输入先 journal 再消费'和决策#5 的 resume 语义共同支撑——event store 本身就是休眠 session 的 durable mailbox。需要新增的是 session registry + lazy-resume 组件，纯 L4 增量，不动契约。

### scheduled/cron triggers, self wake-up (ScheduleWakeup), durable timer firing while process is down — `extension`

*important · both* · 依据：L1 'timeout 是 durable timer' / L4 Scheduler 与 triggers / 决策#2 / 原则 2

cron 到新 session 已被 scheduler actor + RunAgent command 覆盖；到已有 session 复用上条的休眠唤醒机制。关键问题'进程不在时 durable timer 由谁触发'设计未正面回答——L1 的 durable timer 是'记录在案的定时器'（event），但单进程模型（决策#2）下进程死了没人看表。补法不破契约：server 壳本就是常驻进程，加一个扫描各 stream 待决 timer 的 daemon（timer 索引是允许的派生物，原则 2），到期即走唤醒路径；纯 CLI 无常驻进程的用户则退化为 resume 时补触发过期 timer，这与 L1'恢复时 fold'语义一致。ScheduleWakeup 工具 = 写入一条 timer event，同一机制。

### notifications (desktop/push) and statusline — `extension`

*important · both* · 依据：原则 5 / L4 交互协议输出事件流 / L2 关卡[3]、mode 跃迁 event

通知就是一个订阅 ApprovalRequested / WAITING_INPUT / run 结束等 event 的 frontend actor，接桌面或推送通道——原则 5 下 frontend 无特权、可任意加。statusline 需要的会话摘要（当前 model、mode、token/cost）全部可从 event stream fold 出来（L2 budget 从 event stream 统计、mode 跃迁本身是 event）。都是纯新增订阅者组件。

### /cost and token/cache accounting — `clean`

*important · both* · 依据：L2 关卡[3] / L3 Provider 返回归一化 / L4 Observability

L2 budget 关卡明确'turns/tokens/cost 从 event stream 统计'，L3 provider 返回归一化 token 计数（含 cache_read/cache_write）且'budget 关卡按真实计费口径记账'，L4 observability 已列出'token/cost（含 cache 命中）消耗'的时间线渲染。/cost 只是对同一 fold 的 CLI 展示。

### OTel metrics/traces export — `extension`

*nice-to-have · claude-code* · 依据：L4 Observability / L0 Envelope causation-correlation / 决策#8

L4 observability 的立场是'event log 就是 trace'，且 causation/correlation 链路 per-stream 完整（L3 multi-agent 可审计性保证），到 OTel span 树的映射是机械的：correlation id → trace，causation → parent span，EffectResolved/Activity 事件 → span 属性。需要写一个订阅 bus 或尾随 event log 的 exporter actor，纯增量。

### transcript export and audit/compliance — `clean`

*important · both* · 依据：决策#4、#8、#12、#15c / L4 Observability / Roadmap M5

决策#4 保证一切输入先 journal（'历史完整可审计'是 L1 原文），决策#8 的单条 EffectResolved 记录每个副作用'为什么放行/拦下'的全部关卡判定，决策#12 JSONL '可读可 diff'，决策#15c 保证密钥不落 event log。导出 transcript 就是渲染 event log，inspect 时间线（M5）已是同一件事的 CLI 形态。

### parallel attempts / best-of-N (Codex cloud style) — `extension`

*nice-to-have · codex* · 依据：L0 Actor / L3 'worktree 级隔离' / L4 fork barrier

并发来自'很多个 actor'（L0），单进程内同时跑 N 个 run 天然成立；每个 attempt 用 L3 已有的 worktree 级隔离拿到独立文件视图，或从任务起点 barrier fork N 份（L4 fork 复制 stream 闭包一致切面）。需要新增的是编排/择优的上层组件与结果对比 UI，不动 L0-L2 契约。

### slash commands / custom commands over the protocol — `extension`

*important · both* · 依据：L4 交互协议'协议预留' / L3 ContextCompacted / L2 mode 跃迁 event

L4 交互协议已明确'协议预留（原型不实现）：slash command 调用'，按判定口径属 extension。slash command 本质是 frontend 侧把命令展开成输入 event 或控制指令（如触发 compaction、切 mode——两者都已是 event 化操作：ContextCompacted、mode 跃迁 event），custom command 文件展开成 prompt 注入也只在消息层，不碰 prefix 稳定不变量。

## MCP / skills / plugins / commands 生态（23 项）

### mcp-stdio-transport — `clean`

*core · both* · 依据：L3 MCP / 决策#17 / Roadmap M4

L3 MCP 节直接给出 spec 内 `transport: stdio` + `command` 的一等支持，决策#17 把 server 生命周期定为带外运行时状态、resume/重启后重新拉起，M4 已排期。tool 调用作为 activity 走 L2 管线，spec 的 `allowed_tools` 收窄与 permission rules（数据）天然衔接，无需任何新抽象。

### mcp-streamable-http-sse — `extension`

*important · both* · 依据：L3 MCP『spec schema 里保留 transport: http + auth 字段，实现推迟』/ 决策#17

spec schema 已保留 `transport: http` + auth 字段、实现推迟，按口径算 extension。带外生命周期契约（决策#17）对 transport 类型不敏感，换 transport 不触碰 event 模型。要注意 streamable HTTP 的 Mcp-Session-Id 带会话状态，而设计已文档化 'per-call stateless' 契约——重连/resume 后 server 端会话状态丢失是明示边界而非隐藏坑。

### mcp-oauth-token-storage — `friction`

*important · both* · 依据：决策#15c / L3 MCP『实现（OAuth 流程、凭据存储）推迟』

决策#15c 写死『凭据只从环境变量读，绝不进 spec/event/仓库』，而 OAuth token 是运行时动态获取、必须持久化 refresh token 的凭据，环境变量这一唯一渠道表达不了。需要局部修改 15c 引入第三个受管持久位置（本地 token store，在 event log 与 workspace 之外），并给带外的 server 启动流程加交互式授权路径。失败场景：接入需 OAuth 的 GitHub streamable HTTP server，浏览器授权拿到 refresh token 后无处合法落盘，进程每次重启都得重新走授权，或逼用户手工把短命 token 塞进环境变量、过期即断。L3 虽预留了 auth 字段，但预留的只是配置面，凭据存储这一结构性问题没被覆盖。

### mcp-server-health-reconnect — `clean`

*important · both* · 依据：决策#17 / 决策#6 / 决策#9

决策#17 明确 server 生命周期是带外运行时状态、resume/重启后重新拉起；中途 crash 时调用失败落到 activity 的通用 retry（决策#6『retry 是 activity 的通用属性』），重试耗尽则按决策#9 渲染 error tool_result 让 loop 继续。带外管理器重拉后 schema 若变化则记录新 schema event。server 状态不污染 event 模型正是该决策的设计意图。

### mcp-tools-list-changed — `clean`

*important · claude-code* · 依据：L3 MCP『tools/list_changed 同理』/ L3 context assembly prefix 不变量

L3 MCP 原文点名『tools/list_changed 同理』——变更后的 schema 作为影响 run 结果的输入记录为 event、进 fold；context assembly 的 prefix 不变量对此有显式出口：『任何会打爆 prefix 的操作要么禁止要么显式换代』。两个机制组合即得到良定义的中途工具集变更语义，resume/fork 时 fold 到对应 seq 就能重建当时的工具面。

### mcp-resources-at-mention — `extension`

*important · claude-code* · 依据：L2 effect pipeline / 决策#4 / 决策#17 / L4『协议预留：附件/图片消息类型』

resources/read 是一次 server 往返副作用，顺着 L2 管线做成新的 activity 类型即可；读到的内容作为外部输入按决策#4 journal 后进消息，L4 协议也已预留附件/图片消息类型，@ 引用展开可挂在同一输入处理路径。决策#17 措辞是『只有 tool 调用是 activity』，把 resource 读纳入 activity 集合是自然放宽而非推翻——activity 在 L1 本就是开放集合，该决策的理由（server 状态不可 event 化）并不排斥它。

### mcp-prompts-as-slash-commands — `extension`

*nice-to-have · claude-code* · 依据：L4『协议预留：slash command 调用』/ 决策#17

L4 明确预留 slash command 调用协议；MCP prompt 的发现与 tool schema 同构（记录为 event，决策#17），prompts/get 的往返做成 activity，返回消息 journal 后进对话。全程不碰 L0-L2 契约。

### mcp-sampling-elicitation — `friction`

*nice-to-have · claude-code* · 依据：L1『挂起是显式状态…turn/tool-call 边界』/ 决策#17 per-call stateless 契约 / L2 stage[4]

两条契约夹住了它：L1 规定『挂起是显式状态……全都发生在 turn/tool-call 边界』，而 sampling/elicitation 是 server 在管线 stage[4] 执行中途反向发起的请求，等待发生在 activity 内部、不在任何边界上；决策#17 又规定 server 会话是带外状态且 per-call stateless，崩溃后该会话与半途的反向交互不可重建。失败场景：server 在一次长 tool call 中 elicit 用户输入，进程死掉后 MCP session 重建、外层 call 变 in-doubt，用户应答无处投递，只能人工确认后整体重跑再被 elicit 一次——『等几天成本相同』的 durable park 承诺对这类等待不成立。要支持就得给 L1 新增『activity 内非持久等待』类别并给 in-doubt 语义开特例，属于局部修改层间契约；嵌套的 sampling LLM 调用还必须回流管线过 budget 关卡，虽有 spawn 先例，但 durable 性同样拿不到。

### deferred-tool-loading-toolsearch — `extension`

*important · claude-code* · 依据：决策#17 / 决策#10 / 决策#15b / L3 context assembly prefix 不变量

三个既有机制拼起来正好覆盖：决策#17 把发现的 schema 记录为 event（工具面因此是 fold 的派生物，resume/fork 自动正确重建当时的动态工具面）；决策#10 已把『工具面过滤』定义为数据；prefix 不变量提供『显式换代』出口——ToolSearch 载入新工具 = 一条工具面变更 event + 一次 prefix 换代，完全顺着现有抽象。隐藏成本需点名但非结构性：主 provider Gemini 没有 Anthropic 那种 API 级 deferred tools（后者可经决策#15b 的 capability 映射接入），客户端换代意味着每次载入重写整段对话缓存，频繁 ToolSearch 会侵蚀『caching 约 10x 经济性』前提；几千个 tool 的 schema 全量进 JSONL event log（决策#12）也需按内容去重防膨胀。

### skills-progressive-disclosure — `clean`

*core · claude-code* · 依据：决策#16 / L3 context assembly『tool/skill/子 agent 目录』/ Roadmap M4

决策#16 直接沿用 Claude Code skill 约定（目录 + markdown + frontmatter），spec 已有 `skills:` 字段；context assembly 固定拼装顺序里『tool/skill/子 agent 目录』常驻 system prompt，正是 progressive disclosure 的列表半边；body 触发时注入走消息流不碰 prefix，与 caching 不变量无冲突。M4 已排期。

### skill-scripts-and-allowed-tools — `extension`

*important · claude-code* · 依据：决策#10 / L3 multi-agent 权限继承 / 决策#16

skill 目录携带的脚本靠 bash/read tool 执行，天然走 L2 管线全关卡。frontmatter 的 allowed-tools 是 skill 激活期间的临时权限收窄，设计里有两个现成先例：决策#10 的『mode = 工具面过滤（数据）』和 multi-agent 的『child 有效权限 = child ∩ parent』交集语义。需要新写的只是 skill 作用域的 policy 激活/失效生命周期（一对 event），不动任何契约。

### plugins-marketplace-bundle — `extension`

*important · claude-code* · 依据：L3『配置分层从简…三层与更细的合并语义等真实冲突出现再加』/ 决策#13 / 原则 4

插件本质是『一包数据』——commands/agents/skills/hooks/MCP 在此设计里全部已是声明式数据（原则 4、决策#13），打包与安装是 run 之外的文件物料化，不进运行时契约。要补的是配置合并的 plugin 层，设计原文明确『三层与更细的合并语义等真实冲突出现再加』且已给出拼接顺序先例（local > project > spec），按口径这种显式推迟算 extension。风险点仅在合并语义细节，无结构障碍。

### slash-commands-markdown — `extension`

*core · both* · 依据：L4『协议预留：slash command 调用』/ 决策#4 / L3 Provider

L4 明确『协议预留（原型不实现）：slash command 调用』。命令展开（markdown 模板 + $ARGUMENTS → 用户消息）落在决策#4 的输入 journal 路径上，resume/fork 可重建；frontmatter 的 allowed-tools 复用 policy-as-data，model 覆盖走 provider 薄接口的 per-request 参数（缓存按模型隔离，切换即一次 miss，可接受）。全是顺着现有抽象的新代码。

### command-bash-preexec-and-file-refs — `extension`

*important · claude-code* · 依据：L2『所有副作用流经唯一管线』/ 决策#4

感叹号前缀的预执行 bash 是发生在 turn 之外的副作用，而 L2 管线声明自己是『所有副作用的唯一通道』且对 effect 来源不敏感（tool、MCP、LLM、bash……皆可），把命令展开期的 bash 作为 run stream 里先行的 activity 走管线即可，hooks/permission/budget 自动生效。@file 引用展开同理，展开结果按决策#4 journal 成 event。

### output-styles-switch — `extension`

*nice-to-have · claude-code* · 依据：L3 context assembly 拼装顺序 + prefix 不变量

切换 output style 改写的是拼装顺序里最靠前的『harness 基础指令』层，必然打爆整个 prefix；设计对此有显式预案『要么禁止要么显式换代』，实现时选换代——StyleChanged 记为 event，fold 后 context assembly 产出新一代 prefix。因为 context assembly 是 fold(event log) → request，切换点之后的 fork/rewind 语义自动良定义（『fold 到哪个 seq 就得到哪个视图』）。成本是一次全量 cache 重写，这是功能固有代价而非设计强加。

### memory-shortcut-hash-command — `extension`

*important · claude-code* · 依据：L3 context assembly memory 文件层 / 决策#4 / L2 管线

# 一键写入 = 一次 workspace 文件写（走 L2 管线）+ memory 层内容变更触发显式换代。有一个设计未写明但被原则覆盖的细节：CLAUDE.md 位于 prefix 的 memory 层，context assembly 若每 turn 直接读文件系统会破坏 fold 纯度，按决策#4『一切影响 run 结果的输入先 journal』应记录内容或 hash 再消费。属于实现时要点名的细节，不是契约修改。

### memory-files-claudemd-agentsmd — `clean`

*core · both* · 依据：L3 context assembly『memory 文件层』/ spec `context.memory_files` / Roadmap M4

spec 已有 `memory_files: true`，context assembly 固定顺序中有『memory 文件层（CLAUDE.md 按目录层级合并）』，M4 排期。Codex 的 AGENTS.md 只是文件名与合并根不同，同一机制直接覆盖。

### codex-profiles-custom-prompts — `clean`

*important · codex* · 依据：L3 agent spec『spec 是模板，agent instance = spec + 运行时输入』/ L3『配置分层从简』

Codex 的 config.toml profile 是『模型 + provider + 审批策略』的命名捆绑，而设计的 spec 本身就是这个捆绑（model/permissions/limits 同处一个 YAML），『agent instance = spec + 运行时输入』加两层 settings 覆盖即等价表达，profile 切换 = 换 spec/覆盖层。custom prompts 目录与 slash command 机制同构（见 slash-commands 条目）。

### hook-lifecycle-event-matrix — `extension`

*important · claude-code* · 依据：决策#11 / L2 管线 [1][5] / L3『Compaction 是 recorded activity』

设计的 hooks 只存在于 L2 管线的 [1]/[5] 两个 effect 级挂点，而 Claude Code 的 hook 矩阵有一半不是 effect 作用域（SessionStart/SessionEnd/Stop/SubagentStop/UserPromptSubmit/Notification）。其中 PreCompact 已被天然覆盖——『Compaction 是 recorded activity』意味着它走管线自带 pre/post hook；其余生命周期挂点需在 L3 loop 与 L4 session 代码里新增，但决策#11『hooks 是管线机件不是 effect』的定性不被触碰，observe+block 语义可原样搬用，判定结果照旧进对应 event。是新增挂点而非修改契约。

### hook-input-mutation — `extension`

*important · claude-code* · 依据：决策#11 / 决策#8 / L2『恢复时读记录值，不重跑 hook 脚本』

决策#11 明确把 mutation『连同它带来的顺序与缓存问题一起推迟』，按口径默认 extension；检查后确认推迟处未藏结构性问题：管线 stage[1] 本就位于 permission 之前，改写后的输入记进那条唯一的 `EffectResolved`（决策#8『关卡判定在记录边界之内』），恢复路径读记录值、绝不重跑 hook 脚本的原则对 mutation 同样成立。UserPromptSubmit 注入 additionalContext 走消息流，不碰 prefix 不变量。

### custom-subagents-as-data — `clean`

*important · claude-code* · 依据：决策#13 / L3 agent spec `agents:` / L3 context assembly 目录注入

设计把 agent 定义为一等声明式 spec（决策#13），spec 的 `agents:` 白名单控制可 spawn 集合，context assembly 注入子 agent 目录（原文：『模型不知道 summarizer 存在就永远不会 spawn 它——目录注入是 multi-agent 可用的前提』）。.claude/agents 的 markdown+frontmatter 只是另一种序列化格式，加个 loader 即可映射到既有 spec 模型。

### mcp-dynamic-server-add-remove — `extension`

*nice-to-have · claude-code* · 依据：决策#17 / L3 context assembly prefix 不变量

会话中途 add/remove server（/mcp reconnect、claude mcp add）= 带外生命周期操作（决策#17）+ 一条配置变更/schema 发现 event + prefix 显式换代，三个既有机制的组合。配置中途变更被 prefix 不变量点名『要么禁止要么显式换代』，实现时对这条路径选换代即可，不动任何决策。

### mcp-tool-permission-rules — `clean`

*important · both* · 依据：L3 agent spec `mcp.allowed_tools` + `permissions.rules` / L2 permission 关卡 / 决策#9

spec 的 mcp 条目自带 `allowed_tools` 收窄，permission rules 是数据（spec `permissions.rules`，policy 是数据），对 MCP tool 做 pattern 规则与内置 tool 无区别；MCP tool 调用作为 activity 过同一条 L2 管线，ask/deny 的模型面渲染由决策#9 统一定义（error tool_result，loop 继续）。

## Provider 能力映射（22 项）

### anthropic-cache-control-mapping — `clean`

*core · claude-code* · 依据：L3 context assembly "Prefix 稳定性" / 决策15、15b / Roadmap M3

L3 context assembly 把 prefix 稳定性定为"显式不变量"（原文承认没有 caching "agent loop 在经济上不可用"），并明确分工："缓存怎么落地（Anthropic 的显式 cache_control 断点 vs. Gemini 的 context cache 句柄）由各 provider 实现"，event 记录归一化 cache_read/cache_write，budget "按真实计费口径记账"。4 断点上限、5m/1h TTL、最小可缓存长度都是 Anthropic adapter 内部的断点放置策略，正好落在这个分工里；"system prompt 与 tool schema 排序稳定"直接覆盖 tools 块参与 prefix、顺序变化即失效的要求。M1 尾注排期 Anthropic 在 M3 caching 阶段作第二实现验证。

### dynamic-tool-surface-cache-invalidation — `clean`

*important · both* · 依据：L3 context assembly "Prefix 稳定性" / 决策10 / L3 MCP tools/list_changed

决策10 把 mode 定义为"工具面过滤"，plan→default 跃迁和 MCP tools/list_changed 都会改 tool 列表，按 Anthropic 语义打爆 tools 块之后的全部 cache。设计对此有显式政策："任何会打爆 prefix 的操作要么禁止要么显式换代"，且跃迁本身是 event，provider 可在换代点重置断点，代价有界（每次跃迁一次 prefix 重写）。若想学 Claude Code 保持 tool 列表稳定、改在 permission 层收紧以保 cache，也能在决策10 的"过滤"语义下选 permission-face 实现，不动契约。

### thinking-signature-opaque-roundtrip — `extension`

*core · both* · 依据：L3 Provider "返回归一化" / 决策15b / Roadmap M1 尾注

Provider 节写"thinking 块统一成一套内部表示"，但通篇未提 Anthropic signature、Gemini thoughtSignature、OpenAI encrypted reasoning item 这类必须逐字节原样回传的不透明字段——按字面实现，fold(event log)→请求重发时会丢 signature，多轮 thinking+tool use 直接被 API 以 400 拒绝。修法是给归一化 block 加 provider 打标的 opaque 透传字段并落进 event（L2/L3 不解读，不违反"不感知具体 provider"），纯增量、不动任何决策；M1 尾注让 Anthropic 作第二实现"验证能力抽象不漏"正是为抓这类漏预留的排期位，故按口径算 extension 而非 friction，但这是抽象最明确的一处漏，应在实现前把"opaque 无损往返"写成显式不变量。

### interleaved-thinking-beta — `extension`

*important · claude-code* · 依据：L3 Provider / 决策15b / agent spec model 块

需要三件增量：beta header 这类 provider 专有请求开关（15b 通用 capability 之外，spec 的 model 块加 provider 专属 passthrough 字段）；一条 assistant 消息内 thinking 与 tool_use 交错的有序 block 列表（归一化消息按 block 序列建模即可）；以及依赖 thinking-signature-opaque-roundtrip 条的签名透传。全部顺着现有抽象走，不碰 L0-L2。

### fine-grained-tool-streaming — `clean`

*nice-to-have · claude-code* · 依据：L3 Agent loop "Streaming 的持久化边界" / 原则2

设计的流式边界"token delta 只走 bus（显式 ephemeral），持久化的是组装完成的 assistant message"与 fine-grained tool streaming 完全同构：tool input 的 partial_json delta 走 bus 供前端提前渲染，组装完成的 tool_use 落 event 进 L2 管线。管线按完整 tool call 判定，所以只能"早渲染"不能"早执行"，这与关卡语义一致，不构成损失。

### structured-outputs-response-format — `extension`

*important · both* · 依据：决策15b / L3 Multi-agent result contract

请求侧"以 provider 无关的方式携带 caching、thinking、tools、max_tokens 等意图"是开放集合，加一个 output_schema/response_format capability 由各家映射（Anthropic structured outputs、OpenAI json_schema、Gemini responseSchema）即可，schema 子集差异走 capabilities() 声明+显式降级。与 multi-agent 的 result contract（"contract 在子 agent spec 的输出约定里声明"）天然互补，可把口头契约升级为 schema 校验。

### tool-choice-parallel-control — `extension`

*important · both* · 依据：决策15b / L3 Agent loop "并行 tool call 是常态"

tool_choice（auto/any/none/指定名）与 disable_parallel_tool_use 是教科书式的 15b 通用 capability：Anthropic tool_choice、OpenAI tool_choice/parallel_tool_calls、Gemini function calling mode（AUTO/ANY/NONE + allowed_function_names）互相可映射。loop 已把"并行 tool call 是常态"设计好（allow 并发、ask 串行等审批），请求侧开关只是归一化请求上的数据字段，不碰 loop 契约。

### long-context-1m-pricing-tier — `extension`

*important · claude-code* · 依据：L3 context assembly cache token 记账句 / spec context.compaction / 决策15b

1M beta 是 provider 专属开关（同 beta header passthrough 通道）；分档计价（≤200k 与 >200k 单价不同）意味着记账不能只存 token 数，但设计已说 budget"按真实计费口径记账"，让 provider 在 activity 完成时算好成本或把适用价档记进 event 即为纯数据扩展。compaction trigger_ratio 需要 per-model 窗口大小元数据，beta 开关改变窗口值，同属 provider 元数据表的事。

### server-side-tools — `friction`

*important · both* · 依据：原则3 / 决策8 / L2 effect pipeline

障碍是原则3+决策8："一切副作用是 activity，流经同一条 effect pipeline，四关卡"。web_search/code_execution 由 API 在一次 LLM activity 内部执行，副作用发生在 provider 侧，pre-hook 和 per-call permission 没有任何可插入时机。失败场景：用户写 {tool: web_search, action: ask}，模型流出 server_tool_use 时搜索已在服务端执行完毕，管线只能在组装请求时对整个 capability 整体放行/拒绝，无法逐次审批，pre_tool_use hook 对每次搜索永不触发；code_execution 的容器复用（container id）还是不可从 event fold 的 provider 侧状态，rewind/fork 无法恢复。需要给 L2 契约开一个文档化的"provider 执行类工具"例外类别（请求期整体审批+响应期 post-hoc 观测），外加按次计费记账与 pause_turn stop reason 的 loop 处理——属局部修改层间契约。

### mcp-connector-api-side — `friction`

*nice-to-have · both* · 依据：决策17 / L3 MCP / 决策8

与 server-side-tools 同源且更深：MCP connector 由 API 侧直连 MCP server，同时绕过决策17 的两条契约——"发现的 tool schema 记录为 event"（发现发生在 provider 侧）和"只有 McpToolCalled/Returned 是 activity"（调用不经本地 client，这些 activity 根本不产生）。失败场景：spec 的 mcp.allowed_tools 与 permission rules 对 API 侧直连失去强制力，只能翻译成请求里的 tool_configuration 求 provider 自律，审计时间线也缺失逐次调用的 EffectResolved。要支持须作为"provider 执行类工具"例外并接受 permission 语义降级为请求期整体判定。

### vision-pdf-input — `extension`

*important · both* · 依据：L4 交互协议"协议预留" / 决策12

L4 交互协议已显式预留"附件/图片消息类型"，按判定口径算 extension。增量在：归一化消息 block 支持 image/document 类型并由各 provider 映射（Anthropic image/document source、Gemini inlineData、OpenAI input_image/input_file）；event log 存二进制的策略——JSONL 内嵌 base64 会膨胀，但决策12 把存储藏在 EventStore 接口后，换 SQLite 或旁路文件引用即可，不动契约。

### citations — `extension`

*nice-to-have · claude-code* · 依据：L3 Provider "返回归一化" / 决策15b

citations 是响应侧新 block 形态（带 citation 标注的 text block）加请求侧 document block 的 citations 开关，落在"返回归一化"框架里加字段即可；后续 turn 的 fold→请求需原样回传带 citation 的块，复用 opaque 透传机制。与 server-side web_search 的引用结果联动时才触及 server-side-tools 条的例外类别。

### count-tokens-endpoint — `extension`

*nice-to-have · both* · 依据：决策15、15b / spec context.compaction

决策15 把 provider 接口定为"薄接口（complete(request) → stream）"单方法，count_tokens 是第二个操作，但作为 capabilities() 声明的可选方法加上是纯增量（Anthropic count_tokens、Gemini countTokens 有，OpenAI 无则显式降级为本地估算）。compaction 的 trigger_ratio 用上一轮响应 usage 也能驱动，所以不是硬依赖。

### batch-api-fanout — `extension`

*nice-to-have · both* · 依据：L1 Activity 语义（in-doubt）/ L1 "挂起是显式状态" / L2 关卡3

隐藏的坑在 L1 activity 语义：把最长 24h 的 batch job 建模成单个 activity，进程死亡后恢复会命中"有 Started 无 Completed → in-doubt 上浮、绝不静默重跑"，把本可按 batch_id 幂等续取的任务错误升级成人工处理。但现有原语足以正确分解：submit 为短 activity（记录 batch_id）→ 显式 WAITING + durable timer → poll/retrieve 各为独立 activity——"挂起是显式状态""timeout 走 durable timer"正是这种形态。顺着抽象组合即可，不动契约。

### openai-responses-api — `extension`

*core · codex* · 依据：原则2 / L3 context assembly / 决策15、15b

Codex 系模型只走 Responses API。stateful 模式（previous_response_id，历史存在 OpenAI 侧）与原则2"一切历史皆 event、state=fold(event log)"正面冲突——但无需采用：store:false + include reasoning.encrypted_content 的无状态模式让 harness 继续持有全量历史，每轮由 context assembly fold 成 items 数组，与现架构同构，且 OpenAI 自动 prefix caching 直接受益于"prefix 稳定性不变量"。前提是 opaque 透传落实（encrypted reasoning item 必须逐字节回传），其内建 tools 落入 server-side-tools 条的 friction。写第三个 provider adapter 即可，不动 L0-L2。

### gemini-caching-model — `extension`

*important · both* · 依据：L3 context assembly caching 落地句 / L2 关卡3 / 决策17 先例

设计原文点名"Gemini 的 context cache 句柄由 provider 实现"且 Gemini 是主 provider。implicit caching（cachedContentTokenCount→归一化 cache_read）完全落进现有记账。explicit CachedContent 有两个增量：句柄是带外运行时状态（同决策17 对 MCP server 的先例，resume 后重建/重挂）；存储计费是 $/token/小时的时间累积费，而 budget 关卡"从 event stream 统计"只看每次 activity——需要 provider 在创建/续期时合成计费 event 或把摊销成本记入调用 event，别扭但隔离在 Gemini adapter 与记账口径内；agent loop 场景 implicit 已够经济。

### gemini-thinking-function-modes — `clean`

*important · both* · 依据：agent spec model.thinking / 决策15b

spec 已用通用形态写 thinking: { budget_tokens } 并注明"provider 各自映射"，Gemini 映射到 thinkingConfig（thinkingBudget/includeThoughts），function calling mode 并入 tool_choice capability——这是 15b 的教科书用例，现有机制直接覆盖。唯一暗坑是 Gemini 2.5 函数调用的 thoughtSignature 同样要求原样回传，已并入 opaque 透传条。

### stream-retry-rate-limit-semantics — `clean`

*core · both* · 依据：L1 Activity 语义 / L3 Agent loop "Streaming 的持久化边界" / 决策6

这是设计覆盖最完整的一条：retry/backoff、rate limit 处理、model fallback 是"activity 的通用属性"；流中断重试后发 TurnDiscarded event，前端据此"重试中"重开流，"绝不静默替换用户已看到的文本"；429 retry-after/529 由 activity 重试策略消化，等待走 durable timer 不读墙钟。持久化边界只落组装完成的消息，半截流不会污染 event log。

### cross-provider-fallback-history-translation — `friction`

*important · both* · 依据：L1 Activity 语义 retry 句 / 决策6 / L3 context assembly

障碍是 L1 把"model fallback"归为与 retry/backoff 并列的"activity 级策略"，暗示换 provider 是重试参数级的便宜动作；实际跨 provider fallback 是 L3 context assembly 级工作：历史必须按目标 provider 重渲染——签名 thinking 块/encrypted reasoning 不可移植必须剥离、tool schema 方言（Gemini 的 OpenAPI 子集 vs Anthropic input_schema）重映射、system prompt 落位不同、cache 从零暖起。失败场景：Anthropic 529 风暴触发 fallback 到 Gemini，activity 层若持有已按 Anthropic 渲染的请求直接重发，带签名 thinking 块会被 Gemini 拒绝；正确做法要求 fallback 决策上移到 loop/context assembly 重新 fold，或改 activity 契约为持有归一化请求、execute 内重渲染——两者都要局部改写"fallback 是 activity 通用属性"这条 L1 表述。同 provider 内换 model 无此问题。

### capability-declaration-operability — `extension`

*important · both* · 依据：决策15b / L3 Provider capabilities()

15b 的 capabilities() + 显式降级对布尔型能力（有无 thinking/caching）可操作，但本领域大半差异是约束型的：4 个断点与最小可缓存长度、structured output 的 schema 子集、thinking 的回传规则、"隐式前缀 vs 显式句柄"是不同种而非有无。capabilities() 需从布尔集合长成带参数的元数据（限额、方言、计费口径），降级决策在 context assembly 组装请求时按元数据执行——纯增量演进，不推翻 15b。唯一兜不住的是"provider 侧执行"的权限语义差异，那属 L2 例外而非能力声明的表达力问题。

### model-pricing-metadata — `extension`

*important · both* · 依据：L2 关卡3 / L3 context assembly 记账句 / 原则4、决策13

budget"按真实计费口径记账"和 compaction trigger_ratio 都隐含需要一张 per-model 元数据表：窗口大小、分档单价、cache 写 1.25x/2x 与读 0.1x 系数、batch 5 折、server tool 按次费。设计没写这张表住哪，但原则4"tool 定义以数据文件随包分发"给了现成先例——model 元数据同样做成随包数据、provider 加载映射；或更稳妥地由 provider 在 activity 完成时算好成本记入 event，budget 只累加。两条路都不动契约。

### oauth-subscription-auth — `friction`

*important · both* · 依据：决策15c / L3 Provider 凭据段

决策15c 写死"凭据只从环境变量读"。Claude Code 主流用户走订阅 OAuth（token 需刷新、需持久存于 keychain 类凭据库），Codex 走 ChatGPT 登录，同为 OAuth 刷新流。失败场景：长跑 run 中 access token 过期，env var 是进程启动时的静态值，provider 无处取新 token，run 以鉴权失败告终；刷新后的 token 写回哪里在 15c 下没有合法答案。需把 15c 局部放宽为"凭据经 credential provider 抽象获取、绝不进 spec/event/仓库"（保留其真实意图"密钥永不落盘于受控内容"）——属修改一条已定决策的措辞，非架构性返工。

