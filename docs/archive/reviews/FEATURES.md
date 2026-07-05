# Claude Code / Codex 功能点清单（166 项速读版）

> 本清单是能力审查（[CAPABILITY-REVIEW.md](CAPABILITY-REVIEW.md)）过程中整理的
> Claude Code（含 Agent SDK、云端会话）与 OpenAI Codex（CLI/cloud）功能全集，
> 用于快速浏览。每项的完整判定理由见
> [CAPABILITY-REVIEW-DETAILS.md](CAPABILITY-REVIEW-DETAILS.md)。
>
> **判定**：✅ clean（现有设计直接覆盖）· 🔧 extension（顺着抽象新增代码即可）·
> ⚠️ friction（需局部修改某条已定决策，已归并进 review 的建议）
> **重要度**：★★★ core（产品没有它就不成立）· ★★ important（日常用到）· ★ nice-to-have

## 1. Agent loop 核心机制（22 项）

| 判定 | 功能点 | 说明 | 产品 | 重要度 |
|:--:|---|---|:--:|:--:|
| ✅ | token-level streaming output | token 级流式输出，delta 走总线、组装后消息持久化 | 两家 | ★★★ |
| ✅ | fine-grained tool parameter streaming | tool 调用参数也流式输出 | 两家 | ★★ |
| ✅ | extended thinking budget | 思考预算控制（含 per-message 动态调整） | CC | ★★★ |
| 🔧 | thinking block signature round-trip | thinking 块带 signature 原样回传 API（含 redacted） | CC | ★★★ |
| ✅ | interleaved thinking | 工具调用之间穿插思考块 | CC | ★★ |
| ✅ | parallel tool calls | 一条消息 N 个调用：混合审批、并发执行、乱序完成 | 两家 | ★★★ |
| ✅ | steering | 运行中插话，当前 tool 跑完立即注入 | 两家 | ★★★ |
| ✅ | interrupt (Esc) | 打断流式输出/运行中工具，打断后状态一致 | 两家 | ★★★ |
| ✅ | retry / 429 / 529 / mid-stream recovery | 请求重试、限流处理、断流恢复（TurnDiscarded） | 两家 | ★★★ |
| 🔧 | mid-session model switch (/model) | 中途换模型，含跨 provider 的历史序列化 | 两家 | ★★ |
| ✅ | reasoning effort levels & fast mode | 推理力度档位与 serving 档位切换 | 两家 | ★★ |
| 🔧 | structured output | 强制 JSON schema 输出（子 agent 结果契约依赖） | 两家 | ★★ |
| ✅ | stop reason handling | max_tokens 截断续写、prompt 过长处理 | 两家 | ★★★ |
| ✅ | automatic model fallback | 主模型不可用自动切备用 | CC | ★★ |
| ✅ | token counting & cost accounting | token/成本核算，cache read/write 分口径 | 两家 | ★★★ |
| 🔧 | Codex reasoning summaries & encrypted reasoning | 推理摘要 + 加密推理块（不透明往返） | Codex | ★★★ |
| 🔧 | Codex Responses API stateful conversation | Responses API 有状态对话 | Codex | ★★ |
| ✅ | prompt caching & prefix stability | 缓存断点由 loop 放置、前缀稳定 | 两家 | ★★★ |
| 🔧 | background tool execution | 后台工具执行，完成后异步注入结果 | CC | ★★ |
| 🔧 | mid-turn context injection (system-reminders) | turn 中途注入系统提醒（文件变更、todo 状态） | CC | ★★ |
| 🔧 | server-side tools & pause_turn | provider 侧执行的工具与 pause_turn 处理 | 两家 | ★★ |
| ⚠️ | OAuth/subscription auth | 订阅 OAuth、token 刷新、云凭据链 | 两家 | ★★★ |

## 2. 上下文管理（23 项）

| 判定 | 功能点 | 说明 | 产品 | 重要度 |
|:--:|---|---|:--:|:--:|
| ⚠️ | dynamic env block vs prompt cache | 环境块（git 状态/日期）动态性与缓存的矛盾 | 两家 | ★★★ |
| ✅ | system prompt assembly order | system prompt 分段拼装、顺序固定 | 两家 | ★★★ |
| 🔧 | CLAUDE.md hierarchy & imports | 多层级 CLAUDE.md 与 @import 语法 | CC | ★★★ |
| 🔧 | subdir CLAUDE.md on-demand injection | 进入子目录才注入该目录的 CLAUDE.md | CC | ★★ |
| ✅ | AGENTS.md convention | AGENTS.md 约定（Codex 主用，CC 兼容） | 两家 | ★★ |
| ✅ | auto-compaction threshold | 阈值触发自动压缩上下文 | 两家 | ★★★ |
| 🔧 | manual /compact & /clear | 手动压缩（带自定义指示）与清空 | 两家 | ★★ |
| 🔧 | microcompaction / context editing | 只清理旧 tool result 而非整体 summarize | CC | ★★ |
| ✅ | compaction rewind/fork semantics | 跨压缩边界的回退/分叉语义 | CC | ★★ |
| ✅ | tool result truncation & paged read | per-tool 输出截断、大文件分页读取 | 两家 | ★★★ |
| ✅ | prompt cache prefix stability & breakpoints | 缓存断点放置与前缀稳定不变量 | 两家 | ★★★ |
| 🔧 | cache TTL variants (5m/1h) | 两档缓存 TTL 的选择 | CC | ★ |
| 🔧 | mid-session tool list change vs cache | 中途工具列表变更与缓存的相容 | 两家 | ★★ |
| 🔧 | multimodal input & @-file refs | 图片/截图/PDF 输入、@文件引用展开 | 两家 | ★★ |
| 🔧 | long-context 1M window | 1M 上下文窗口（beta 头、计价档） | 两家 | ★ |
| 🔧 | context usage indicator | 上下文余量实时指示 | 两家 | ★★ |
| 🔧 | prompt-too-long auto recovery | prompt 过长时自动压缩重试 | 两家 | ★★ |
| ✅ | history fold across compaction | 跨压缩的会话历史重建（fold 良定义） | 两家 | ★★★ |
| ✅ | Codex AGENTS.md merge & history compression | Codex 的层级合并与历史压缩策略 | Codex | ★★ |
| 🔧 | memory write-back | # 开头快捷写入记忆文件 | CC | ★ |
| 🔧 | thinking block roundtrip in context | 上下文重建时 thinking 块无损往返 | 两家 | ★★ |
| 🔧 | hook output context injection | hook 输出注入上下文（additionalContext） | CC | ★ |
| 🔧 | system-reminder dynamic injection | 消息流内动态注入 system-reminder | CC | ★★ |

## 3. 工具与 workspace（18 项）

| 判定 | 功能点 | 说明 | 产品 | 重要度 |
|:--:|---|---|:--:|:--:|
| 🔧 | Read tool | 读文件：文本/分页/图片/PDF/notebook | 两家 | ★★★ |
| ✅ | Edit tool | 唯一匹配约束 + 文件被外部修改后拒编（Codex apply_patch） | 两家 | ★★★ |
| ✅ | Write tool | 写文件 | 两家 | ★★★ |
| ✅ | Glob / Grep | 文件名与内容搜索 | 两家 | ★★★ |
| 🔧 | NotebookEdit | Jupyter notebook 编辑 | CC | ★ |
| ✅ | Bash tool | cwd 跨调用持续、超时、输出截断 | 两家 | ★★★ |
| 🔧 | Background bash | run_in_background + 输出轮询 + kill + 完成唤醒 | CC | ★★ |
| 🔧 | OS sandbox | seccomp/landlock、bubblewrap/sandbox-exec、网络沙箱 | 两家 | ★★★ |
| 🔧 | Sandbox escalation retry | 沙箱内失败后请求出沙箱重试（审批） | Codex | ★★★ |
| 🔧 | Workspace boundary & --add-dir | 路径边界保护、多根目录 | 两家 | ★★ |
| ⚠️ | Checkpoint vs agent/user git ops | checkpoint 机制与 agent/用户自身 git 操作的冲突 | CC | ★★ |
| ⚠️ | Checkpoint on non-git / large repos | 非 git 目录、超大 repo 的 checkpoint | CC | ★★ |
| 🔧 | Worktree isolation | 多 agent worktree 隔离并行改同一 repo | 两家 | ★★ |
| ✅ | WebFetch / WebSearch | 网页抓取与搜索工具 | 两家 | ★★ |
| 🔧 | Plan file / scratchpad | 计划文件、草稿工作区 | 两家 | ★★ |
| ✅ | Rewind granularity | 回退粒度三选一：仅代码/仅对话/两者 | CC | ★★ |
| 🔧 | Fork workspace isolation | fork 出隔离 workspace 并行探索 | CC | ★ |
| 🔧 | Streaming tool output | 工具输出（bash stdout）实时流给前端 | 两家 | ★★ |

## 4. 权限与 hooks（21 项）

| 判定 | 功能点 | 说明 | 产品 | 重要度 |
|:--:|---|---|:--:|:--:|
| ✅ | permission modes | default/acceptEdits/plan/bypass 与模式跃迁 | CC | ★★★ |
| ✅ | PreToolUse observe + block | 工具执行前观察/拦截（exit code） | CC | ★★★ |
| 🔧 | permission rule syntax | Bash(git:*)、Edit(src/**)、mcp__x__y、deny>ask>allow | CC | ★★★ |
| 🔧 | five-layer settings | enterprise > CLI > local > project > user 五层设置 | 两家 | ★★ |
| 🔧 | always-allow write-back | "不再询问"写回 settings，运行时即时生效 | CC | ★★ |
| 🔧 | rich approval UI | diff 预览、改写输入后批准、批准并记住 | CC | ★★ |
| ✅ | programmatic/headless approval | canUseTool 回调、非交互兜底 | 两家 | ★★ |
| ✅ | parallel × permission | 已放行调用并发跑，审批挂起不阻塞 | 两家 | ★★ |
| ✅ | subagent permission inheritance | 子 agent 权限交集继承、审批路由到人 | CC | ★★ |
| 🔧 | PreToolUse updatedInput | hook 改写工具输入 | CC | ★★ |
| 🔧 | PreToolUse permissionDecision | hook 直接返回 allow/deny 短路审批 | CC | ★★ |
| 🔧 | PostToolUse feedback | 工具执行后给模型附加反馈 | CC | ★★ |
| ⚠️ | hook durability & EffectResolved timing | hook 执行结果的持久化时序（崩溃窗口） | CC | ★★ |
| ⚠️ | UserPromptSubmit hook | 用户输入侧 hook：注入上下文/拦截 | CC | ★★ |
| ⚠️ | Stop / SubagentStop hook | 拒绝停止、强制 loop 继续干活 | CC | ★★ |
| 🔧 | SessionStart / SessionEnd hooks | 会话生命周期 hook | CC | ★★ |
| ✅ | PreCompact hook | 压缩前 hook | CC | ★ |
| 🔧 | Notification hook | 通知类 hook | CC | ★ |
| 🔧 | hook timeout / parallel / async | hook 超时、并行执行、异步 hook | CC | ★★ |
| 🔧 | hook merging | 多层配置与 plugin 的 hook 合并 | CC | ★★ |
| ⚠️ | Codex approval policy | untrusted/on-failure/on-request/never + 沙箱升级审批 | Codex | ★★★ |

## 5. 多 agent（18 项）

| 判定 | 功能点 | 说明 | 产品 | 重要度 |
|:--:|---|---|:--:|:--:|
| ✅ | declarative subagent definition | markdown+frontmatter 声明式子 agent（tools/model/effort） | 两家 | ★★★ |
| ✅ | SDK programmatic agent definition | SDK 程序化定义 agent | 两家 | ★★ |
| ✅ | spawn/await fan-out | 扇出子 agent 并等待 | 两家 | ★★★ |
| 🔧 | background subagents notify-wake | 后台子 agent，完成后通知唤醒父 | CC | ★★ |
| 🔧 | resume child conversation | 续接已存在的子 agent 对话（SendMessage） | CC | ★★ |
| 🔧 | agent teams | peer 间直接通信、共享任务列表 | CC | ★★ |
| ✅ | handoff | 移交控制权后父退出 | Codex | ★★ |
| ✅ | approval bubbling & permission intersection | 审批冒泡到前端、权限交集继承 | 两家 | ★★★ |
| 🔧 | nesting depth & concurrency limits | 嵌套深度、并发上限 | 两家 | ★★ |
| 🔧 | shared token budget pool | 跨子 agent 共享 token 预算池 | 两家 | ★★ |
| 🔧 | per-agent worktree isolation | 每个子 agent 独立 worktree | 两家 | ★★ |
| 🔧 | structured result contract | 子 agent 结果的 JSON schema 强制 | 两家 | ★★ |
| ⚠️ | deterministic workflow orchestration | 确定性编排脚本（fan-out/pipeline 由代码驱动） | 两家 | ★★ |
| ✅ | subagent transcript observability | 子 agent transcript、父子 correlation 链 | 两家 | ★★ |
| ✅ | cascading cancellation | 父被打断时整棵子树级联取消 | 两家 | ★★★ |
| ⚠️ | multi-agent session fork/rewind | 有活跃子 agent 时的 fork/rewind | CC | ★ |
| 🔧 | dynamic runtime agent definition | 运行时动态定义新 agent | CC | ★ |
| ✅ | subagent directory prompt injection | 子 agent 目录注入 prompt（模型知道能 spawn 谁） | 两家 | ★★★ |

## 6. Session 与 surfaces（19 项）

| 判定 | 功能点 | 说明 | 产品 | 重要度 |
|:--:|---|---|:--:|:--:|
| ✅ | session list / resume / continue | 会话枚举、恢复、续跑 | 两家 | ★★★ |
| ⚠️ | cross-version resume | 周更 CLI 后仍能恢复旧会话 | 两家 | ★★★ |
| ✅ | rewind granularity | 恢复仅代码/仅对话/两者 | CC | ★★ |
| 🔧 | session teleport | 本地 ↔ 云端会话迁移、远程容器会话 | 两家 | ★★ |
| ⚠️ | multiple concurrent sessions per workspace | 同一 workspace 多个并发会话 | 两家 | ★★ |
| ✅ | headless -p mode | headless 单发、json/stream-json 输出、--resume | 两家 | ★★★ |
| 🔧 | Agent SDK | 进程内 query、canUseTool 回调、进程内自定义工具、hook 回调 | 两家 | ★★★ |
| ✅ | server mode (HTTP/WS) | server 形态暴露同一协议 | 两家 | ★★ |
| 🔧 | multi-client attach | 手机+桌面同时看/控同一会话 | 两家 | ★★ |
| 🔧 | IDE integration | diff 视图、编辑器选区上下文、@ 引用 | 两家 | ★★ |
| ✅ | GitHub Actions / @claude mention | GitHub 提及触发新 run | 两家 | ★★ |
| 🔧 | webhook into dormant session | webhook/PR 事件流入休眠会话 | CC | ★★ |
| 🔧 | scheduled/cron triggers & self wake-up | 定时触发、自唤醒；进程不在时的 timer | 两家 | ★★ |
| 🔧 | notifications & statusline | 桌面/推送通知、状态栏 | 两家 | ★★ |
| ✅ | /cost accounting | 成本查询（含 cache 命中口径） | 两家 | ★★ |
| 🔧 | OTel metrics/traces export | OTel 指标/追踪导出 | CC | ★ |
| ✅ | transcript export & audit | transcript 导出、审计合规 | 两家 | ★★ |
| 🔧 | parallel attempts / best-of-N | 同一任务并行多次尝试选优（Codex cloud） | Codex | ★ |
| 🔧 | slash commands over protocol | 协议层的 slash command 调用 | 两家 | ★★ |

## 7. MCP / skills / plugins / commands 生态（23 项）

| 判定 | 功能点 | 说明 | 产品 | 重要度 |
|:--:|---|---|:--:|:--:|
| ✅ | MCP stdio transport | stdio 传输 | 两家 | ★★★ |
| 🔧 | MCP streamable HTTP / SSE | 远程 HTTP/SSE 传输 | 两家 | ★★ |
| ⚠️ | MCP OAuth & token storage | OAuth 授权流程与 refresh token 存储 | 两家 | ★★ |
| ✅ | MCP server health & reconnect | 健康检查、重连、中途 crash 恢复 | 两家 | ★★ |
| ✅ | MCP tools/list_changed | server 中途变更工具集的通知 | CC | ★★ |
| 🔧 | MCP resources @-mention | @ 引用 MCP 资源内容 | CC | ★★ |
| 🔧 | MCP prompts as slash commands | MCP prompts 暴露为命令 | CC | ★ |
| ⚠️ | MCP sampling / elicitation | server 反向请求 LLM/用户输入 | CC | ★ |
| 🔧 | deferred tool loading (ToolSearch) | 几千个工具按需加载，不全进上下文 | CC | ★★ |
| ✅ | skills progressive disclosure | SKILL.md 列表常驻、body 触发时注入 | CC | ★★★ |
| 🔧 | skill scripts & allowed-tools | skill 携带脚本资源、工具白名单 | CC | ★★ |
| 🔧 | plugins marketplace bundle | 插件打包 commands/agents/skills/hooks/MCP | CC | ★★ |
| 🔧 | slash commands (markdown) | .claude/commands、$ARGUMENTS、frontmatter | 两家 | ★★★ |
| 🔧 | command bash pre-exec & file refs | 命令体内嵌 bash 预执行、@文件展开 | CC | ★★ |
| 🔧 | output styles switch | 切换 system prompt 人格 | CC | ★ |
| 🔧 | memory shortcut (#) | # 开头一键写入 CLAUDE.md | CC | ★★ |
| ✅ | memory files (CLAUDE.md/AGENTS.md) | 记忆指令文件注入 | 两家 | ★★★ |
| ✅ | Codex profiles & custom prompts | config.toml profiles、自定义 prompts 目录 | Codex | ★★ |
| 🔧 | hook lifecycle event matrix | hook 全事件矩阵（生命周期类） | CC | ★★ |
| 🔧 | hook input mutation | hook 改写输入 | CC | ★★ |
| ✅ | custom subagents as data | 自定义子 agent 即数据文件 | CC | ★★ |
| 🔧 | MCP dynamic server add/remove | 运行时增删 MCP server | CC | ★ |
| ✅ | MCP tool permission rules | mcp__server__tool 级权限规则 | 两家 | ★★ |

## 8. Provider 能力映射（22 项）

| 判定 | 功能点 | 说明 | 产品 | 重要度 |
|:--:|---|---|:--:|:--:|
| ✅ | Anthropic cache_control mapping | 断点数量/TTL/最小长度/层级失效规则 | CC | ★★★ |
| ✅ | dynamic tool surface cache invalidation | 工具列表变更 → 全缓存失效的规则 | 两家 | ★★ |
| 🔧 | thinking signature opaque roundtrip | 签名/加密推理块的不透明无损往返 | 两家 | ★★★ |
| 🔧 | interleaved thinking beta | beta header 开关 | CC | ★★ |
| ✅ | fine-grained tool streaming | 细粒度工具参数流式 | CC | ★ |
| 🔧 | structured outputs / response_format | 结构化输出各家方言 | 两家 | ★★ |
| 🔧 | tool_choice & parallel control | tool_choice、禁用并行调用 | 两家 | ★★ |
| 🔧 | long-context 1M pricing tier | 1M 上下文 beta 与计价档 | CC | ★★ |
| ⚠️ | server-side tools | web_search/code_execution 由 API 侧执行 | 两家 | ★★ |
| ⚠️ | MCP connector (API-side) | API 直连 MCP server，绕过本地管线 | 两家 | ★ |
| 🔧 | vision / PDF input | 图像与 PDF 输入映射 | 两家 | ★★ |
| 🔧 | citations | 引用标注 | CC | ★ |
| 🔧 | count_tokens endpoint | token 预计数 | 两家 | ★ |
| 🔧 | Batch API fan-out | 批处理降本（子 agent 扇出） | 两家 | ★ |
| 🔧 | OpenAI Responses API | stateful responses、reasoning items 回传、内建工具 | Codex | ★★★ |
| 🔧 | Gemini caching model | 显式 cache 句柄 + 存储计费 + 隐式缓存 | 两家 | ★★ |
| ✅ | Gemini thinking & function modes | thinking config、函数调用模式 | 两家 | ★★ |
| ✅ | stream retry & rate limit semantics | 流中断恢复、429/529/retry-after | 两家 | ★★★ |
| ⚠️ | cross-provider fallback history translation | 跨 provider 切换时历史转译（签名不可移植） | 两家 | ★★ |
| 🔧 | capability declaration operability | capability 声明 + 显式降级的逐项落地 | 两家 | ★★ |
| 🔧 | model pricing metadata | 模型价格元数据（记账用） | 两家 | ★★ |
| ⚠️ | OAuth subscription auth | 订阅 OAuth 认证（同 agent-loop 条目，provider 视角） | 两家 | ★★ |

---

**统计**：clean 61 · extension 87 · friction 18 · blocker 0（共 166 项）。
18 个 ⚠️ friction 全部同根归并为 [CAPABILITY-REVIEW.md](CAPABILITY-REVIEW.md)
的 7 条主修订 + 8 条一句话契约 + 9 条 minor，具体修改指令见
[DESIGN-SUGGESTIONS.md](DESIGN-SUGGESTIONS.md)。
