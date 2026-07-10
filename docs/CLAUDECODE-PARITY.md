# AgentRunner — Claude Code 功能对照审计（CLAUDECODE-PARITY）

**这是什么**：以 **Anthropic Claude Code 本地 CLI/runtime 核心**（2026-07
实况，版本线 2.1.x 至 2.1.205）为标尺，对 AgentRunner 引擎（`internal/`）
与 webui 逐项对照的**审计件**——与 CODEX-PARITY（Codex 标尺）互为姊妹件。
按开发者裁决，本文**只对标本地核心**：会话管理、上下文与记忆、工具面、
权限/沙箱、hooks、编排、驱动、headless、配置与可观测；**排除** cloud/web、
Routines 云端形态、IDE/桌面/移动、GitHub App/Actions、Managed Agents API
等生态外围。冲突时以三层活文档（JOURNEYS/SPEC/DESIGN）为准；引用 GAPS
条目不另立缺口编号。

**为什么值得单独审一份**：AgentRunner 的多处约定直接承自 Claude Code
（skills 格式决策 #16、`.claude/commands` G21/INC-8、CLAUDE.md 层级合并
S3）——本文有"回到源头"的性质；且 Claude Code 是三个对标物（Codex/
Claude Code/自家 journey）中**模型侧工程**（上下文、记忆、权限分类器、
工具生态）最深的一个，恰好压在我们最薄的带上。

**维护纪律**：同 CODEX-PARITY——每关闭一个对照项更新 §2 状态并挂到对应
GAPS/SPEC 条目；不删行，只改状态。审计基线与参照来源见 §5。

---

## §1 结论速览

103 个对照项（§2，十二个域）：**齐平或领先 37（其中 10 项语义领先）·
部分 34 · 进行中 3（INC-12 团队线）· 缺失 28 · 显式非目标 1**（另有
4 项缺失在缺口列明示 P3 不追：TUI 定制面族、NotebookEdit、server 侧
context 管理）。

**总判断（三句话）**：

1. **runtime 语义层我们同级或反超。** 会话常驻/resume/托管 daemon/
   多 session 并行/后台子 agent/goal/loop/预算/恢复——Claude Code 在
   2.1.x 才长出的 supervisor daemon + roster + respawn（agent view），
   是我们 daemon/journal/静止模型的后来者形态；其 transcript 是"JSONL
   追加 + queue-operation"，无 durable 恰好一次投递、无 in-doubt 崩溃
   纪律、无 journaled 审计链——这一半我们是**结构性领先**（与
   CODEX-PARITY §07 结论一致）。
2. **模型侧工程是 Claude Code 最深的护城河，也是我们最集中的缺口带。**
   四级上下文压缩（含不调 LLM 的 microcompact）、auto-memory 体系
   （MEMORY.md 索引 + 主题文件 + per-agent 持久记忆）、30 事件 × 5
   handler 的 hooks 面、auto mode 权限分类器、权限规则的工程细节
   （复合命令拆分/wrapper 剥离/参数级匹配/protected paths）、每 prompt
   自动 checkpoint 的双轨回退——这五块（上下文/记忆/hooks/治理精度/
   checkpoint）我们要么只有雏形（G7 手动 compact、G19 双事件 hooks、
   G9 读侧记忆），要么形态不同（barrier/fork）。
3. **补齐路径是"移植模型侧工程到我们的内核上"，不是重写内核。**
   Claude Code 的这些能力全部是 harness 层机制（prompt 工程 + 文件
   约定 + 管线关卡），恰好能落进我们已有的 seam：microcompact 落
   compaction 管线、auto-memory 落 G9 写回、hooks 扩展落 G19 事件族、
   auto mode 落 effect pipeline 的 policy 源、checkpoint 语义并进
   barrier 族。P0 三件（上下文工程/记忆写回/hooks 扩展）见 §4。

**我们领先的面（审计的另一半事实）**：durable CommandLog 恰好一次
（对方两终端同 session resume = transcript 交织，文档原话）；崩溃恢复
契约（in-doubt 分类处置 vs 对方 changelog 里 daemon "每 50s 自杀"、
"stale lock 起不来"的修复史）；fork/rewind 带 **workspace git 快照**
（对方 checkpoint 明文不覆盖 bash 副作用、不是版本控制）；goal 的
verifier 硬证据 + 预算 + 控制面（对方 `/goal` 是纯 prompt 坚持条款）；
树级预算 reserve/settle（对方只有 per-run `--max-budget-usd` 停闸）；
无人值守 fail-closed（对方 `dontAsk` 同哲学但无 driver 语义）；OS 沙箱
基座已同代（决策 #34 Seatbelt/Bubblewrap，与其 sandbox-runtime 同技术
栈）+ link-local/metadata 无条件封禁（决策 #33，对方无）；双 provider
（对方锁 Anthropic 系）；事件语义级审计链（EffectResolved 判定链 vs
对方 OTEL 计量导出）。

---

## §2 逐域对照矩阵

图例：✅ 齐平/领先 · 🟡 部分 · ❌ 缺失 · 🧊 显式推迟/非目标。
Claude Code 列标注精确行为（版本号/默认值）；AgentRunner 列带锚
（DESIGN 决策号/SPEC/GAPS/INC）。

### 01 会话与生命周期

| # | 功能 | Claude Code | AgentRunner | 缺口 |
|---|---|---|---|---|
| 1 | 会话常驻/续聊 | transcript JSONL 追加，`-c`/`-r` resume | ✅ 静止模型 + send 复活（决策 #31，QA-01） | — |
| 2 | resume 选择器（名字/ID/picker/scope 拓宽） | picker 含搜索/预览/rename/跨 worktree/跨 project | 🟡 `ar sessions` 平铺 + id resume；无命名/搜索/分组 | webui rename/搜索（CODEX-PARITY §01 同项） |
| 3 | 会话命名（-n/`/rename`/plan 自动命名/默认名） | v2.1.196+ 全 session 有展示名 | ❌ 标题=开场消息 | 同上 |
| 4 | 分叉对话（`/branch`、`--fork-session`） | 任意当前点复制历史开新 | 🟡 fork 只到 CheckpointBarrier（含 workspace 快照，**更强**）；任意消息点 fork ❌（记档：任意点=每 turn 落 barrier） | 见 §3.1 |
| 5 | 多端同 session | 两终端同时 resume → 消息**交织**进同一 transcript（无仲裁） | ✅ **领先** daemon 单宿主 + durable CommandLog 序列化（INC-11.2/12.3 锁定单宿主回执） | — |
| 6 | PR↔session 绑定（`--from-pr`） | 按 PR 号/URL 找回 session | ❌ | G13 |
| 7 | 会话导出（`/export`） | 纯文本/剪贴板 | ✅ `ar events`/`inspect --json`（结构化，更强） | — |
| 8 | 保留期治理（`cleanupPeriodDays` 30 天） | 自动清理 | ❌ store 只增不清 | 小（gc 随 shadow-repo flock 一并做，backlog 已记） |
| 9 | 离开回来一行摘要（session recap/catch-up） | ≥3 分钟失焦后台生成 | ❌ | webui 糖，候补 |
| 10 | 后台会话面板（agent view：状态图标/attach/stop/respawn/rm/pin） | `claude agents` + supervisor daemon + roster.json（2.1.x 新长出） | ✅ **领先** `ar daemon`/`ps`/`sessions`/`attach`/`stop`/`kill`（S6/INC-4，先于对方存在）；差 UI 糖（分组/pin/rename） | webui 列表增强 |
| 11 | idle 资源回收（1h 未 attach 停进程、state 持久、resume 重拉） | supervisor 生命周期 | ✅ 同构且更强：静止=待命零成本（journal 即 state），无需"停进程"概念 | — |

### 02 Checkpointing / 时间旅行

| # | 功能 | Claude Code | AgentRunner | 缺口 |
|---|---|---|---|---|
| 12 | 自动打点 | **每个 user prompt 一个 checkpoint**（编辑前整文件快照，存 `~/.claude` 按 project_hash，非 git） | 🟡 CheckpointBarrier 在安全边界/静止/手动（S7）——密度低于每 prompt，但快照是 **workspace 级 git 快照** | 见 §3.1 |
| 13 | 双轨回退（code / conversation / both） | `/rewind` 菜单三选 + Summarize from/up-to here | 🟡 fork/rewind = 事件切面 + workspace 物化（**both**，S7）；无"只回对话/只回代码"拆分、无"从此点摘要" | §3.1 |
| 14 | 回退覆盖面 | **不含 bash 副作用**（rm/mv 不可回退）、不含手改/并发 session；"local undo 非版本控制" | ✅ **领先** shadow-repo 快照含全部 workspace 变化（含 bash 效果、排除表外的一切，决策 #7） | — |
| 15 | checkpoint 跨 session 持久 | 30 天随 session 清理 | ✅ snapshot pinned until GC | — |
| 16 | worktree 隔离（`--worktree`、EnterWorktree/ExitWorktree 工具、bg 自动 worktree） | 每后台 agent 编辑前进 `.claude/worktrees/` | 🟡 fork/best-of-N 已用隔离 worktree（S7）；无"进/出 worktree"工具与默认隔离策略 | G13 worktree 泛化（CODEX-PARITY §04 同项） |

### 03 上下文工程（对方最深带之一）

| # | 功能 | Claude Code | AgentRunner | 缺口 |
|---|---|---|---|---|
| 17 | 自动压缩 | auto-compact（阈值 ~92%，binary 常量 0.92；`autoCompactEnabled`、thrash 熔断连败 3 次停） | ✅ 自动 compaction（S3，阈值触发）+ 空 summary 保护（INC-6 真验修） | 熔断/thrash 检测候补 |
| 18 | **microcompact（不调 LLM 的轻量层）** | 先把可重算的旧工具结果换成 `[Old tool result cleared]`（Read/Shell/Grep/Glob/Web*），不动对话；四级体系 snip→microcompact→collapse→autocompact | ✅ **已实现 INC-13**（2026-07-09）：assembly 把久远可重算 read-class 结果渲染为占位符、不调 LLM、单调 boundary、先于 autocompact；journal 留全量（fork/resume 稳）；QA-22 真 Gemini 验 | —（P0① 关闭） |
| 19 | 手动 compact 带指示 / clear | `/compact [focus]`、`/clear`（旧对话可 resume） | ✅ INC-6（G7 关闭，QA-12） | — |
| 20 | compact 后关键上下文**重注入** | project-root CLAUDE.md 从磁盘重读重注；已 invoke skills 按预算前推（每个 5000/合计 25000 token） | 🟡 我们 assembly 每 turn 重组 prefix（CLAUDE.md 恒在，天然等价）；skills 注入无预算前推 | skills 预算化候补 |
| 21 | context 占用可视化（`/context` 彩格+建议） | 交互面板 | 🟡 `inspect` 有 token/cost；无占用分解视图 | webui usage 面板（CODEX-PARITY §01 余项） |
| 22 | 1M context | Sonnet 5/Fable 5 原生 1M 默认（2.1.197+），`[1m]` 后缀 | 🟡 provider capability envelope（INC-11.5）已建模；Gemini 天然 1M+；Anthropic 1M beta 未显式接 | 小（capability 映射一行） |
| 23 | 长输出治理 | Bash 输出 30K 字符上限（超存文件给路径）、Read PARTIAL 分页、tool 输出截断 | ✅ per-tool 截断（S3）+ >10KB 长贴折叠 file part | — |
| 24 | server 侧 context 管理 | `USE_API_CONTEXT_MANAGEMENT`（context-editing beta） | ❌ | 🧊 随 provider 能力观察 |

### 04 记忆（对方最深带之二；社区 top 抱怨也在此）

| # | 功能 | Claude Code | AgentRunner | 缺口 |
|---|---|---|---|---|
| 25 | CLAUDE.md 层级合并 | managed→user→project→local 四层，根→cwd 拼接，子目录按需加载 | ✅ 层级合并（S3；无 managed 层=非目标） | — |
| 26 | `@import` 语法 | `@path` 相对含文件处、4 跳上限、代码块跳过、外部 import 需批准 | ❌ | 小，随 G9 顺收 |
| 27 | `.claude/rules/*.md`（paths 条件加载） | glob `paths:` frontmatter，读匹配文件时注入 | ❌ | 候补（条件注入是记忆分层的正解之一） |
| 28 | **auto-memory** | v2.1.59+ 默认开：Claude 自写 `~/.claude/projects/<proj>/memory/MEMORY.md`（索引，每 session 载前 200 行/25KB）+ 主题文件按需 Read；`/memory` 管理；`/dream` 夜间巩固 | ❌ 只有读侧注入 | **G9**（P0②；对方机制=生产验证的设计输入，注记已挂 GAPS） |
| 29 | per-agent 持久记忆 | subagent frontmatter `memory: user/project/local` → `agent-memory/<name>/`（同 200 行/25KB 索引） | ❌ | G9 扩展 |
| 30 | 记忆写回（"记住 X"→文件） | 自然语言即写 auto-memory；"add to CLAUDE.md" 手编 | ✅ **已实现 INC-14**（取 A）：`ar remember` → append 项目 CLAUDE.md → 新 session 冻结生效；QA-23 真验 | —（写回核心 done；auto-memory 完整体余项） |
| 31 | 记忆×压缩闭环 | **他们的已知洞**：压缩后不自动 consult memory（#29890，148 赞 top 抱怨） | ❌（双方都未闭环） | G9 设计时直接补这个闭环=**后发反超点** |

### 05 工具面（对方 28 内置 vs 我们 22 defs）

| # | 功能 | Claude Code | AgentRunner | 缺口 |
|---|---|---|---|---|
| 32 | 文件读/写/编辑 | Read（图/PDF/ipynb 多模态、offset/limit、PARTIAL）/Write/Edit（read-before-edit 三检查、replace_all） | ✅ read/write/edit_file（S1）；read-before-edit 纪律 ❌；**工具读图/PDF ❌**（输入侧图片/PDF ✅ INC-9） | Read 多模态候补；read-before-edit 护栏候补 |
| 33 | Bash 前后台 | run_in_background、timeout 上限、`cd` 持久、5GB 上限、内存压力回收 | ✅ bash + output/kill（S1/S3） | — |
| 34 | **Monitor**（流式监听后台进程/WebSocket，每行输出即通知） | 内置 | ❌ output 是拉取轮询 | 候补（进度通道 backlog 同族，G10 相邻） |
| 35 | Glob/Grep | ripgrep、output_mode/-A/-B/-C/multiline/type | ✅ INC-3 + **INC-22 + INC-24**（2026-07-09）：case_insensitive/glob/output_mode(content/files_with_matches/count)/-A/-B/-C context lines，QA-30+QA-31 真机；默认=旧行为 | multiline 余项（#12c） |
| 36 | 语义检索 | ❌ 无等价物（LSP 是符号级） | ✅ **领先** semantic_search（S7） | — |
| 37 | **LSP**（go-to-def/find-refs/hover，2.0.74+） | 内置+plugin LSP server | ❌ | 候补 P2（大件） |
| 38 | WebFetch | HTML→MD 后**小模型跑提取 prompt**、15min 缓存、跨 host 重定向二次确认、domain 规则 | ✅ INC-5（G18b 关闭：execute-class+收容+metadata 封禁 **更硬**）；无"AI 提取"后处理 | AI 后处理候补 |
| 39 | WebSearch | Anthropic 搜索后端，≤8 次/调用 | ❌ | G18 余项（需外部后端，单独增量） |
| 40 | Task/subagent 工具 | Agent(subagent_type/model/run_in_background/isolation:worktree) | ✅ spawn_agent{background}（QA-04）；isolation:worktree 参数 ❌（fork 有机制） | G13 worktree 泛化 |
| 41 | TaskCreate/TaskList/TaskUpdate（依赖图 task board，TodoWrite 已废弃换代） | 会话/团队共享 task list，`blocks/blockedBy` | 🟡 blackboard（publish_note/read_notes，S4）语义近；无结构化 task/依赖/UI 呈现 | INC-12 团队线顺收（对齐 shared task list） |
| 42 | AskUserQuestion（结构化多选+Other） | 强约束"一切提问走此工具" | 🟡 ask_user 自由文本（INC-5，G20 关闭）；无结构化选项 | 选项化候补（webui 已有审批 UI 可复用） |
| 43 | SendMessage（teammate/subagent 定向消息+resume） | 停的 subagent 收信自动 resume | 🚧 INC-12.1-12.4 进行中（send_message/TreeRouter/ChildRevived/单宿主回执） | UJ-23 收口 |
| 44 | SendUserMessage（agent 主动通知用户，`--brief`） | 内置 | 🟡 notifier 事件通知（S6）；非模型工具 | 候补小 |
| 45 | Skill 工具（模型侧 invoke skill） | Skill(command)；`disable-model-invocation` 可关 | ✅ **核心已实现 INC-20**（2026-07-09）：`skill` 工具按 name 返回 SKILL.md 正文（去 frontmatter、WS 边界+防遍历），QA-29 真机；维持命令=用户宏裁决不动 | context:fork 余项（#7b/INC-20b） |
| 46 | SlashCommand 工具（模型侧执行命令） | 内置（`/init`/`/review` 等可编程调用） | ❌ G21 裁决命令=用户侧宏，模型不可见 | 裁决维持/翻转待定 |
| 47 | NotebookEdit | cell 级编辑 | ❌ | 🧊 低优先 |
| 48 | Artifact（发布可分享页） | 需订阅+login | 🟡 publish_artifact（决策 #23）本地 CAS；无分享面 | 平台层 |
| 49 | MCP 资源工具（ListMcpResources/ReadMcpResource、@server:res 引用） | 内置 | ✅ INC-11.4（resources/prompts 保真） | — |
| 50 | MCP tool search（deferred 加载省 context） | ENABLE_TOOL_SEARCH 默认开，>10% 阈值 | ❌ 全量注入 | 候补（大工具面时的 context 优化） |
| 51 | 工具并发上限 | `CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY` | ✅ 前台并发+背景机制（§4 turn 纪律） | — |

### 06 权限与治理

| # | 功能 | Claude Code | AgentRunner | 缺口 |
|---|---|---|---|---|
| 52 | 规则三档评估 | **deny → ask → allow**，同档首匹配；广 deny 盖窄 allow | ✅ rules 判定链（S2，EffectResolved 记理由） | — |
| 53 | Bash 规则工程细节 | 任意位通配、`:*`≡尾部 ` *`、**复合命令逐段匹配**（&&/;/\|）、**wrapper 剥离**（timeout/nice/xargs）、只读命令内置免提示集 | ✅ **已实现 INC-16**（2026-07-09）：三件套全落（逐段聚合取最严/wrapper 剥离/只读集，显式 deny 先于只读集，fail-safe 退整体）；QA-25 真机（victim 存活证逐段 deny） | — |
| 54 | 路径规则（gitignore 风、四种锚、symlink 双查） | Read/Edit 规则覆盖同类工具+Bash 文件命令 | ✅ path 规则 + realpath 归一（S2）；gitignore 风/锚点语法差异 | 语法增强候补 |
| 55 | 参数级匹配（`Tool(param:value)`，deny/ask） | `Agent(model:opus)`、`Bash(run_in_background:true)` | ❌ | 候补 |
| 56 | permission modes | 6 档：Manual/acceptEdits/plan/auto/dontAsk/bypass | ✅ default/plan/acceptEdits/bypass（S2/S3）+ headless fail-closed（=dontAsk 语义，决策 #34 收紧 driver ask→deny）；**auto ❌** | auto 见 §3.3 |
| 57 | **auto mode（分类器审批）** | 2.1.200 起默认：server 端注入探针 + Sonnet 4.6 transcript 分类器双层；黑白名单 `autoMode.{allow,soft_deny,hard_deny}`；连拒 3 次回退人审 | ❌ | §3.3（P1；作为 effect pipeline 的新 policy 源） |
| 58 | "允许且不再问"持久化 | Bash 按 project+命令永久、文件修改到 session 末；复合命令拆条存（≤5） | ✅ **已实现 INC-17**（2026-07-09，取 A）：`approve --always` 写 user 层精确 allow 规则、下次 session 生效、幂等；QA-26 真机 | —（project 精确作用域/本 run 立即生效余项） |
| 59 | protected paths（`.git`/`.claude`/rc 文件等写保护长表） | 除 bypass 外从不自动批 | ✅ **已实现 INC-18**（2026-07-09）：acceptEdits 对 protected 写强制 ask（只收紧 mode default，bypass/显式规则不变，.claude/worktrees carve-out）；QA-28 真机 | —（差异：我们的显式 allow 规则可放行 protected，对方 allow 不预批——记档） |
| 60 | workspace trust dialog | project settings 的 allow/additionalDirectories 需 trust | ✅ `ar trust`（决策 #19，project hooks 同门） | — |
| 61 | 权限规则热管理（`/permissions` 查规则+来源） | 交互面板 | 🟡 spec/config 静态；webui 无规则面板 | webui 候补 |
| 62 | PreToolUse hook 参与权限（allow/deny/ask/defer + updatedInput） | hook 可改写输入、deny 优先 allow 规则 | 🟡 hooks 可 block（S2）；不能改写输入/参与判定档位 | G19 扩展 |

### 07 沙箱（OS 级；双方 2026 同代化）

| # | 功能 | Claude Code | AgentRunner | 缺口 |
|---|---|---|---|---|
| 63 | OS 沙箱基座 | macOS Seatbelt / Linux bubblewrap+seccomp（sandbox-runtime 开源）；2.0.24 首发 | ✅ **同代** INC-11.3/决策 #34：bash/verifier 默认 Seatbelt/Bubblewrap workspace FS 收容，能力缺失 fail-closed | — |
| 64 | 沙箱内自动放行（auto-allow mode） | 沙箱命令免提示（`autoAllowBashIfSandboxed` 默认 true），不能沙箱的回退权限流 | ❌ 收容与审批是两层，未做"已收容→免审批"联动 | 候补 P1（对方 84% 提示降幅的来源） |
| 65 | FS 细粒度（allowWrite/denyRead/allowRead 数组跨 scope 合并） | 默认写=cwd+tmp、读=全机-denyRead | 🟡 默认 filesystem=workspace（写侧更严）；无 per-path 数组配置 | per-env 粒度随 G11 族 |
| 66 | 凭据沙箱（`sandbox.credentials.files/envVars` deny/**mask**，哨兵值+proxy 注入） | v2.1.187/199+ | 🟡 凭据路径与敏感 env 隔离已入决策 #34；mask/proxy 注入 ❌ | 候补（vault 化随 G11） |
| 67 | 网络沙箱形态 | host HTTP/SOCKS proxy + 域 allowlist、首访提示、tlsTerminate 可选 | ✅ netns/收容棘轮 fail-closed **更硬**（无出口 vs 过滤出口，决策 #33/#34）；无域级 allowlist/审计 | host allowlist（G18 记档 backlog） |
| 68 | 沙箱逃生舱治理 | `dangerouslyDisableSandbox` 参数 + `allowUnsandboxedCommands`（默认 true；false=Strict）+ `excludedCommands` | 🟡 fail-closed 无逃生舱（更严）；缺"显式逃生+审计"的中间档 | 候补 |
| 69 | 沙箱 UX（`/sandbox` 面板、违规指示） | 三 tab 面板 | ❌ spec 字段 | webui 候补 |

### 08 Hooks（对方演进最密集的域）

| # | 功能 | Claude Code | AgentRunner | 缺口 |
|---|---|---|---|---|
| 70 | 事件面 | **30 个事件**：session（Start/End）、turn（UserPromptSubmit/Stop/StopFailure）、tool（Pre/Post/PostToolUseFailure/PostToolBatch）、权限（PermissionRequest/PermissionDenied）、compact（Pre/Post）、agent（SubagentStart/Stop、TeammateIdle、TaskCreated/Completed）、环境（FileChanged/CwdChanged/ConfigChange/WorktreeCreate/Remove/InstructionsLoaded/Setup）、UI（Notification/MessageDisplay/Elicitation…） | 🟡 **INC-15 第一批已落**（2026-07-09）：pre/post tool + 8 个生命周期事件（session_start/end、user_prompt_submit、stop、subagent_start/stop、pre/post_compact；blockable=user_prompt_submit/pre_compact），QA-24 真验；余 20+ 事件待批 | G19 第一批关闭；余项随需求 |
| 71 | handler 类型 | 5 种：command/http/**mcp_tool**/**prompt**/**agent**（模型参与 hook 判定），`async`/`once`/`if` 条件 | 🟡 仅 command（argv） | G19 |
| 72 | 决策契约 | exit 0/2 + JSON（decision:block、permissionDecision allow/deny/ask/defer、**updatedInput**、**updatedToolOutput**、additionalContext、systemMessage） | 🟡 observe+block 二值 | G19（改写类推迟裁决已记档，决策 #11） |
| 73 | hook 挂载点 | settings/plugin/skill/agent frontmatter 四处，来源可见 | 🟡 spec + user 层（决策 #19） | G19 |
| 74 | 环境注入类 hook | SessionStart 的 additionalContext/initialUserMessage/watchPaths、`CLAUDE_ENV_FILE` | ❌ | G19（对齐我们 prefix 组装 seam） |
| 75 | statusline（stdin JSON 全字段：context %/cost/rate limits/PR 态/worktree） | 300ms debounce 脚本协议 | ❌ | 🧊 TUI 糖；webui 有等价信息面 |

### 09 编排：subagent / teams / 背景

| # | 功能 | Claude Code | AgentRunner | 缺口 |
|---|---|---|---|---|
| 76 | 子 agent 定义（`.claude/agents/*.md` frontmatter 10+ 字段） | tools/disallowedTools/model/permissionMode/maxTurns/skills/mcpServers/hooks/memory/background/effort/isolation/color/proactive/whenToUse | ✅ agents/*.yaml spec 同级（S4；含 permission/预算树约束**更严**，决策 #20）；缺 proactive/whenToUse/color 等发现性糖 | — |
| 77 | tools/model 继承 | 四规则（tools/disallowed 组合）+ `CLAUDE_CODE_SUBAGENT_MODEL` 解析序 | ✅ 冻结交集下传（结构上不可能超父，**更强**） | — |
| 78 | 内置 subagent 库（Explore/Plan/general-purpose，跳 CLAUDE.md、thoroughness 档） | 内置 + 可禁 | ❌ 无内置 agent 库（`ar init` 出样例 spec） | 候补小（发行内置 specs） |
| 79 | 默认后台 + 并行（2.1.198） | 子 agent 默认后台、完成通知、权限提示浮到主 session | ✅ spawn{background} + 回执激活父（QA-04/05，C3-C7）；子审批走同一 approval 流 | — |
| 80 | 子 agent 深度/并发闸 | 深度 5 级固定 | ✅ 深度/扇出上限（决策 #20，可配） | — |
| 81 | fork subagent（`/fork` 继承全对话） | 恒后台、不可再 fork | 🟡 handoff_agent（S4）语义近（移交非复制）；继承全对话的后台分身 ❌ | 候补 |
| 82 | subagent resume/steer（SendMessage 唤醒停的 subagent） | v2.1.x | 🚧 INC-12（ChildRevived/revive + 子会话 send 路由——**正在收口**） | UJ-23 |
| 83 | **agent teams**（lead+teammates 互通/共享 task list/mailbox/TeammateIdle hook/display modes） | 实验默认关（env gate）；不可嵌套；`/rewind` 不恢复 teammate | 🚧 INC-12 工程团队（send_message/TreeRouter/冻结动态角色/单宿主回执）——**同构且我们建在 durable 内核上**（他们 in-process teammate 不可恢复，我们 journal 化） | UJ-23 收口=结构性反超点 |
| 84 | 子 agent 观测（transcript 分文件/subagentStatusLine/tokenCount） | `subagents/agent-*.jsonl` | ✅ `sub/` 子 journal + 子会话寻址（INC-1）；实时进度镜像 ❌（G10） | G10 |

### 10 驱动与自动化（本地）

| # | 功能 | Claude Code | AgentRunner | 缺口 |
|---|---|---|---|---|
| 85 | goal（跨回合坚持） | `/goal [condition\|clear]`：prompt 级坚持条款，无 verifier/预算/状态机 | ✅ **领先** in-session goal：verifier 硬证据 ∧/∨ 模型自证（goal_complete）、结构化 continuation、预算=可见截断、控制面 attach/pause/resume/update/cancel/revive（INC-D1+INC-10，决策 #21/#34，QA-16/17） | — |
| 86 | loop（`/loop` 间隔/self-paced ScheduleWakeup） | session 开着时重复 | ✅ loop driver interval/cron/self_paced + overlap 语义（S6）+ durable timer（§12） | — |
| 87 | 本地 scheduled tasks（cron 编码进 SKILL.md、`/schedule` 本地形态、CronCreate 工具） | 依赖进程/daemon 存活 | 🟡 cron 在 driver；**跨重启唤醒 🧊 backlog**（G22 boot sweep 未落） | G22 |
| 88 | 自动化安全（scheduled 免打扰批准=不可交互即拒） | dontAsk 语义 | ✅ driver fail-closed + headless ask→deny（决策 #34 收紧） | — |

### 11 Headless / SDK 边界

| # | 功能 | Claude Code | AgentRunner | 缺口 |
|---|---|---|---|---|
| 89 | print 模式 | `-p` + stdin ≤10MB、`--max-turns`、退出等待后台任务收尾 | ✅ `ar run`（=开+发+等静止+读结果，决策 #31）；`ar drive` 系列 | — |
| 90 | 输出协议 | `--output-format json/stream-json`（system_init/assistant/result 事件、per-model cost、`--include-partial-messages` token delta） | ✅ `--json` 事件流 + attach SSE（决策 #22）；事件 schema 双方私有格式 | — |
| 91 | **结构化输出**（`--json-schema` → structured_output 字段） | schema 校验、失败重试 | ❌ | 候补 P1（verifier/集成两用；provider 已有 JSON mode 能力位） |
| 92 | 预算/回合闸 | `--max-budget-usd`（花超停）、`--max-turns` | ✅ **领先** 树级 reserve/settle 预算 + per-turn max_generation_steps + goal 级预算（决策 #20/#21） | — |
| 93 | `--bare`/`--safe-mode`（跳过一切 auto-discovery） | CI 推荐姿势 | 🟡 spec 显式化=天然 bare；无一键"忽略 project 层"开关 | 候补小 |
| 94 | SDK（Agent SDK：query() 暴露同一 harness、hooks 回调、V2 preview） | CLI 即 SDK 消费者 | 🟡 core 是库（决策 #14）同哲学；无对外 SDK 包装 | CODEX-PARITY §08 同项（薄包装 P2） |

### 12 配置、可观测与交互面

| # | 功能 | Claude Code | AgentRunner | 缺口 |
|---|---|---|---|---|
| 95 | settings 分层 | Managed>CLI>Local>Project>User，数组跨层合并，schema 化，热重载 | 🟡 user/project 两层 + spec（决策 #13）；无 local/managed 层、无热重载承诺 | 候补小（local 层顺手；managed=非目标） |
| 96 | 旋钮面（~80 settings 键 + 386 个 env 变量） | 全面可调 | 🟡 结构等价物在（spec/config）；旋钮密度低一个量级 | 成熟度指标，不逐项追 |
| 97 | 用量可观测（`/usage` 本地成本+plan 条、OTEL metrics/logs 全套、`prompt.id` 关联） | OTEL 导出 8 metrics+全事件 | 🟡 usage 归一化入 event + inspect（决策 #15b）✅；OTEL 导出 ❌ | OTEL P2（企业面） |
| 98 | 诊断（`/doctor` 体检、debug logs、`/status`） | 自修复向导 | 🟡 `ar init` 引导 + spec 错误字段清单（INC-2）；无体检命令 | 候补小 |
| 99 | @file 引用/`!`shell 直通/`#` 记忆前缀 | 行首触发三件套 | 🟡 webui @ 引用 ✅（d8f5ee0）；`!`/`#` ❌ | webui 糖 |
| 100 | 图片/PDF 粘贴输入 | Ctrl+V chip | ✅ `ar send --image/--file` + webui 拖拽（INC-9/G1 关闭） | — |
| 101 | plan mode 深化（/plan 直跑、plan 文件目录、Plan 子 agent、审批后自动命名） | plansDirectory 产物化 | 🟡 plan mode 全流程 ✅（S2/S3，UJ-06）；plan 产物文件/专用 agent ❌ | 候补 |
| 102 | 键位/vim/主题/输出风格（output styles 4 内置+自定义） | TUI 定制面 | 🧊 非目标（webui 是我们的 surface；output style≈`ar agent` 换 spec 已覆盖核心语义，决策 #32） | — |
| 103 | 命令/skill 生态（49 slash、bundled skills、plugins marketplace、热重载、`!` 动态注入、context:fork） | 命令=skill 统一（.claude/commands 兼容） | 🟡 自定义命令 ✅（INC-8/G21，同一约定）+ skills 读侧 ✅（S5）；bundled 库/热重载/fork 执行/动态注入 ❌；plugins ❌（CODEX-PARITY §08 已记 P2） | 见 §3.5 |

---

## §3 深潜对照（五个信息量最大的机制）

### 3.1 两种时间旅行：checkpoint vs barrier/fork

对方：**每个 user prompt 自动打点**，整文件快照（非 git、只覆盖 Claude
编辑工具动过的文件），`/rewind` 三选（code/conversation/both）+"从此点
摘要"；明文不覆盖 bash 副作用、手改、并发改动——"local undo，非版本
控制"。我们：barrier 打点在安全边界/静止/手动，快照是 **workspace 级
shadow-repo git 快照**（含 bash 效果与一切非排除变化，决策 #7），fork
物化独立 worktree，rewind=fork 后切换。

**互补结论**：我们的快照**覆盖面**与**一致性**强一代（这是引擎该有的
形态）；对方赢在**密度**（每 prompt）与**恢复拆分**（只回对话/只回
代码/摘要化）。补齐动作（不触不变量）：①把 barrier 打点密度提到
"每 turn 收尾"（机制已在，改打点策略）；②fork 增加"仅对话切面"变体
（不物化 worktree，等价"Restore conversation"）；③"Summarize from
here"=手动 compact 带范围指示，落 INC-6 的 compaction 管线。

### 3.2 上下文工程：四级体系 vs 两档

对方体系（社区实测+官方文档）：`snip`（丢整条）→ **microcompact**
（把可重算的旧工具结果原地替换为 `[Old tool result cleared]`，**不调
LLM**，覆盖 Read/Shell/Grep/Glob/Web*）→ `collapse`（选段摘要）→
`autocompact`（全量摘要，~92% 阈值，thrash 熔断）。哲学：**非必要不调
LLM**。另有：compact 后 CLAUDE.md 从磁盘重注、skills 按 token 预算
前推、`/context` 可视化。

我们只有两档（自动阈值摘要 + 手动 compact/clear，S3/INC-6）。**
microcompact 是全表最高杠杆的单项移植**：我们 journal/fold 架构做它
比对方还顺——工具结果本就是 journal 事件，assembly 时按"可重算类
（read-class 且文件未变）"降级为占位符即可，纯 assembly 策略，零事件
变更、零不变量触碰。挂 UJ-09。

### 3.3 auto mode：分类器审批 vs fail-closed

对方 2026 年最大行为变更：权限判定交给**双层模型系统**（server 端
prompt-injection 探针扫工具输出 + Sonnet 4.6 transcript 分类器两阶段
快筛/CoT），黑白名单可配（hard_deny 无条件），连拒 3 次回退人审；
动机=遥测显示 93% 提示被反射式批准。我们哲学相反：无人值守 fail-closed
（headless ask→deny，决策 #34）。

**移植路径（不推翻哲学）**：auto 不是"取消治理"而是**在 effect
pipeline 里加一个 policy 源**——四关卡中 permission 关卡的判定器从
"规则+人"扩为"规则+分类器+人"，分类器判定照常落 `EffectResolved`
（比对方强：他们的分类器判定不进审计链，我们的天然 journaled）。
前置件：G5（判定持久化）+ 规则工程三件套（#53）。P1。

### 3.4 agent teams vs INC-12 工程团队

对方（实验，默认关）：一 session = lead+teammates，各自独立 context、
**互相**直发消息（SendMessage）、共享 task list（文件锁）、mailbox
投递、TeammateIdle/TaskCreated/TaskCompleted hooks；限制：in-process
teammate **不能被 `/resume`/`/rewind` 恢复**、不可嵌套、lead 固定。
我们 INC-12（进行中）：send_message/TreeRouter 树内消息、ChildRevived/
revive 静止子唤醒、冻结动态团队角色、daemon 子会话 send 路由、单宿主
回执，UJ-23 承载。

**对齐清单**（INC-12 收口时逐项核）：①横向直发（非仅父子）——
TreeRouter 是否允许兄弟寻址；②共享 task board——blackboard（S4）语义
在，缺结构化 task+依赖解锁；③idle 钩子——TeammateIdle 等价物挂 G19
事件族；④**恢复语义是我们的反超点**：teammate=递归 session=journal
化，崩溃/重启后完整恢复，对方明文做不到。

### 3.5 skills/命令生态：读侧注入 vs 模型侧调用

对方已把 commands 并入 skills（同名 skill 优先），且是**双向**的：
用户 `/name` 调用 + 模型经 Skill 工具主动 invoke（`disable-model-
invocation` 可关）+ `context:fork` 丢给 subagent 执行 + `!`cmd``
预处理动态注入 + bundled skills 发行库 + 热重载。我们 G21/INC-8 落了
用户侧宏（ingest 时展开），S5 落了 skills 读侧注入，模型侧 invoke
显式裁决为"命令对模型不可见"。

**建议**：维持"命令=用户宏"裁决不动摇（它保 fold 纯与 resume 自
包含），但补"**skill 作为模型可调工具**"这半边——skill def 天然是
tool def（decision #13 tool=数据），invoke=把 SKILL.md 注入下一
generation 的受控形态；`context:fork`=spawn_agent 一次性变体，原语
全在。P1 小件。

---

## §4 结论与路线图

### 4.1 底盘评估

**资产**（对照后更确定）：journal/fold/durable CommandLog/静止模型/
恢复契约/树预算/OS 沙箱基座/goal 语义/双 provider——runtime 语义层
对 Claude Code 同级或反超，且对方 2.1.x 的演进方向（supervisor daemon、
后台默认、SendMessage resume、task board）持续向我们的形态收敛。

**结构性差距**（不是单个功能，是一整层工程）：模型侧工程带——上下文
（#18/20/21）、记忆（#26-31）、hooks（#70-74）、治理精度（#53/55/
57-59/64）、生态（#103）。这层不触任何不变量，全部是"在既有 seam 上
加机制"。

**哲学差异（保留不追）**：TUI 定制面（vim/keybindings/statusline/
output styles）=对方的 surface 投资，我们的 surface 是 webui+CLI，
语义等价物已在（🧊）；旋钮密度（386 env）不逐项追。

### 4.2 路线图（按杠杆排序，P0 三件都压在社区 top 抱怨带上）

- **P0（高杠杆、不触不变量）**：
  ① microcompact（§3.2，assembly 策略，S）
  ② G9 记忆写回 + auto-memory 机制（对方 MEMORY.md 索引/主题文件/
    agent-memory 为设计输入；顺手补"压缩后 consult memory"闭环=对方
    top 抱怨 #29890 的后发反超，M）
  ③ G19 hooks 事件族扩展（第一批：SessionStart/End、UserPromptSubmit、
    Stop、SubagentStart/Stop、PreCompact/PostCompact——恰好都对齐我们
    已有的 journal 事件点位，M）
- **P1（治理与编排精度）**：权限规则三件套（复合拆分/wrapper 剥离/
  只读集，#53）→ G5 允许持久化 → protected paths 写保护（#59）→
  auto-mode 作为 pipeline policy 源（§3.3，依赖前三，L）；INC-12 收口
  对齐 §3.4 清单；skill 模型侧 invoke + context:fork（§3.5，S/M）；
  结构化输出 `--json-schema`（#91，S）；checkpoint 密度与对话回退
  变体（§3.1，M）。
- **P2（生态与平台）**：LSP 工具（#37，L）；WebSearch 后端（#39，
  G18 余项）；plugins/bundled 库/SDK 薄包装/OTEL（随 CODEX-PARITY §08
  同批裁决）；MCP tool search 式 deferred 加载（#50）。
- **P3（明示不追，记档）**：TUI 定制面全族（#75/#102）、NotebookEdit
  （#47）、server 侧 context 管理（#24，随 provider 观察）。

### 4.3 GAPS 联动

本审计不新立缺口编号；已挂注记：G9（auto-memory 机制参照）、G19
（hooks 事件表参照）。候补新登记（待 journey 定版裁决）：microcompact
（挂 UJ-09）、结构化输出（挂 UJ-19 或新 headless journey）、Monitor
流式进度（并 G10）。

---

## §5 审计基线与参照来源

- **我方基线**：origin/main `2cc41cc`（INC-12.4，2026-07-09）——含
  INC-10（goal 自证）/INC-11.3（OS 沙箱，决策 #34）/INC-11.4（MCP
  http+OAuth）/INC-11.5（Turn Item/typed ingress）/INC-12.1-12.4
  （团队原语）。
- **Claude Code 参照（三路证据，2026-07-09）**：
  1. 官方文档 ~25 页逐页抓取（code.claude.com/docs：cli-reference/
     interactive-mode/commands/tools-reference/settings/hooks/
     permissions/permission-modes/sandboxing/memory/checkpointing/
     sub-agents/agent-teams/agent-view/sessions/model-config/headless/
     env-vars/mcp/skills/costs/monitoring-usage 等）；
  2. 本机 claude 2.1.144 binary 字符串取证（28 内置工具 schema、49
     slash 注册表、14 settings-hook 事件枚举、settings/sandbox 全键、
     6 权限模式、CLAUDE_CODE_*/CLAUDE_*/ANTHROPIC_* 三族 386 个 env
     全量、autocompact 阈值常量、teams mailbox 存储实证）；
  3. CHANGELOG 全量（0.2.x→2.1.205，4822 行）+ 官方工程博客（auto
     mode/sandboxing）+ 社区实测（context 四级体系与阈值、checkpoint
     全量快照机制、system prompt 逆向、top issues #29890 等）。
- 姊妹审计件：docs/CODEX-PARITY.md（Codex 标尺）；两文交叉引用处
  已注明，重复项以先登记者为准。
- 调研工作纸（3 份 agent 报告：官方文档/binary 取证/变更史社区）存于
  会话 scratchpad，结论已并入本文。
