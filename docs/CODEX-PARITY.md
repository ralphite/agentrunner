# AgentRunner — Codex 功能对照审计（CODEX-PARITY）

**这是什么**：以 OpenAI Codex（2026-07 全量功能面：CLI / IDE / cloud /
桌面 app / iOS / 集成）为标尺，对 AgentRunner 引擎（`internal/`）与 webui
驾驶舱（`webui/`，Codex 风格应用）逐项对照的**审计件**。它是 GAPS.md 的
一个外部视角切片——GAPS 以自家 22 条 journey 为标尺问"我们缺什么"，本文
以"前沿 coding agent 有什么"为标尺问"对标还差什么"。冲突时以三层活文档
（JOURNEYS/SPEC/DESIGN）为准；本文引用 GAPS 条目而不另立缺口编号。

**维护纪律**：每关闭一个对照项，更新 §3 实现进度表并把状态挂到对应
GAPS/SPEC 条目；不删行，只改状态。审计基线与 Codex 参照来源见文末。

---

## §1 结论速览

74 个对照项（§01–08 功能域，webui UI 项另计）：**齐平或领先 32（其中
11 项强于 Codex 可见承诺）· 部分 10 · 缺失 24 · 显式推迟/非目标 8**。

引擎的**会话内核 / 治理 / 编排 / 驱动 / 恢复**五域已达 Codex 同级，
durability / 审计 / 预算语义反超；差距集中在三个带上：**工具与上下文
薄层、Git 交付工作流、平台生态**。5 条曾卡死的 journey（续聊/多模态/
steering/事件唤醒/后台子 agent）中 4 条已在 v2 关闭，剩事件唤醒（G14）。

我们**领先**于 Codex 可见承诺的面（报告的另一半事实）：durable CommandLog
（确认即持久、恰好一次、跨 kill -9）、静止模型（session 无终态）、
in-doubt 崩溃纪律（非幂等绝不静默重跑）、barrier/fork/rewind 带 workspace
快照、树级预算 + 权限冻结交集、verifier journaled 管线、fail-closed 网络
棘轮 + 凭据硬排除、确定性孪生测试基建、Gemini/Anthropic 双 provider。

---

## §2 逐域对照矩阵

图例：✅ 齐平/领先 · 🟡 部分 · ❌ 缺失 · 🧊 显式推迟/非目标。

### 01 核心会话交互
| 功能 | Codex | AgentRunner | 缺口 |
|---|---|---|---|
| 续聊 | 线程常驻 | ✅ 静止模型，send 永远成立（QA-01） | — |
| 流式输出 | 全端打字流 | ✅ `attach --json` SSE；子会话仅轮询 | 子会话打字流（P1①） |
| 运行中 steering | mid-turn / queue 双默认 | ✅ 双模式对齐：per-message steer（turn 内安全边界注入）/ queue（下个 turn），webui composer `Queue\|Steer` 切换 + ⌘⏎ 反选、CLI `ar send --steer`（INC-43,QA-45） | 硬打断走既有 `interrupt`；排队为 durable 服务端队列（我方领先），不做客户端可撤回 |
| interrupt | Esc 停 turn | ✅ 真停 + 部分输出保留（裁决 #11） | — |
| 消息队列 | queue-by-default | ✅ **领先** durable CommandLog（全 session command、caller idempotency） | — |
| 图片输入 | 粘贴/相机 | ✅ CAS ref（QA-07）+ 缩略图 | — |
| 任意附件 / PDF | 任意类型 | ❌ 仅图片 | G1 余项 |
| 长文本折叠 | 内部消化 | ✅ >10KB 转 file part | — |
| 语音输入 | dictation | 🧊 非目标 | — |
| 会话标题管理 | 自动+改名 | 🟡 标题=开场消息，不可改 | webui rename |
| 会话搜索 | 搜内容+分支 | 🟡 仅标题客户端过滤 | 内容级搜索 |
| 会话归档 | archive | ❌ 平铺只增 | webui 归档 |

### 02 工具面与上下文
| 功能 | Codex | AgentRunner | 缺口 |
|---|---|---|---|
| 文件读/写/编辑 | 内置 | ✅（S1） | — |
| shell 前后台 | 内置+多终端 | ✅ bg + output/kill | UI 无终端面板 |
| **grep / glob** | ripgrep | **✅ INC-3（2026-07-09）** | — |
| 相关性检索 | 无等价物 | ✅ keyword_search（BM25 词法排名,非语义 embeddings） | — |
| web_fetch | 内置 | ✅ 并行线 INC-5（client read-class + network 数据位 + 收容 fail-closed + untrusted 标记）；程序争点待裁 | web_search 仍缺（需外部搜索 API） |
| ask_user（向用户提问） | 内置 | ✅ 并行线 INC-5.2（wait-class：park WAITING_INPUT，应答走 inbox 配对 tool result） | 原 G20 🧊，并行线解冻 |
| 自动 compaction | 长任务压缩 | ✅（S3） | — |
| 手动 compact / clear | 有 | ❌ 只有自动 | G7 |
| 项目指令注入 | AGENTS.md + /init | ✅ CLAUDE.md 合并 + `ar init` | — |
| 持久记忆系统 | Memories + Chronicle | ❌ 只读侧注入 | G9（写回第一步） |
| @ 引用/补全 | @文件/@skill | ❌ | 依赖 G21 + webui |
| 图像生成 | app 内建 | 🧊 建议非目标 | — |

### 03 治理：权限 / 审批 / 沙箱 / 信任（强区）
| 功能 | Codex | AgentRunner | 缺口 |
|---|---|---|---|
| 权限规则 | approval modes + sandbox 档 | ✅ tool/path/command/network（S2/S7） | — |
| 审批流 | 批/拒/parallel | ✅ ask→WAITING_APPROVAL→回灌 | — |
| “允许且不再问” | 写回 config | ❌ 单次批/拒 | G5（PolicyChanged 已设计） |
| 审批 agent 化 | reviewer agent 路由 | ❌ | 可建模为 policy/delegate |
| OS 级沙箱 | seatbelt/landlock/win | ✅ bash/verifier filesystem=workspace；macOS Seatbelt、Linux Bubblewrap；network 棘轮 | Windows backend 暂不支持并 fail-closed |
| 网络策略粒度 | 包管理器/全放行 | ✅ network 规则 + 收容棘轮 | per-env 归 G11 |
| 凭据红线 | 平台侧 | ✅ **领先** 硬排除 + redaction | 威胁模型成文 G16 |
| 信任模型 | trust_level | ✅ `ar trust` | — |
| hooks | rules & hooks | ✅ pre/post observe+block | 生命周期钩子 G19 |
| 无人值守审批 | never=全放行 | ✅ **领先** driver fail-closed | — |

### 04 Git 与交付（对 Codex 最大产品差距带）
| 功能 | Codex | AgentRunner | 缺口 |
|---|---|---|---|
| git 感知与操作 | 内置 git 工具 | 🟡 借 bash；webui 只读 diff | G13 一等公民化 |
| diff 审阅视图 | review 面板/inline 评论 | 🟡 webui「改动」标签（曾有 null 白屏 bug） | 已修 + hunk 折叠 |
| PR 创建/推送 | 产品化 | 🟡 bash+gh 可走（UJ-10 ✅） | G13 |
| 自动 code review | @codex PR review | ❌ 只读角色手工评审 | 依赖 G14+G13 |
| worktree 一等公民 | 每 thread worktree | 🟢 New worktree 落共享 `~/.local/share/agentrunner/worktrees/<repo>-<branch>-<ts>`；Changes 面板显 repo/branch + Apply to project（clean-or-nothing apply-back）+ Remove worktree（防呆+prune）（INC-49） | 位置/可见/apply-back/cleanup 四问齐；diff→PR 仍 G13 |
| 任务→审阅门→PR | 云任务标准流 | ❌ | G13 主体 |
| CI 值守/rebase | cloud follow-ups | ❌ | 依赖 G14 |

### 05 多 agent 编排（引擎语义领先）
| 功能 | Codex | AgentRunner | 缺口 |
|---|---|---|---|
| 后台并行子 agent | background subagents | ✅ spawn{background}（QA-04） | — |
| 编排控制 | 面较薄 | ✅ **领先** kill 带来源/回执激活父/七步（QA-05/09） | — |
| 树级预算/权限收窄 | 无公开等价 | ✅ **领先** reserve/settle + 冻结交集 | — |
| handoff/blackboard | 无公开等价 | ✅ **领先**（S4） | — |
| 子进度实时镜像 | app 内可见 | ❌ 靠 `ar ps` 轮询 | G10 |
| 子 agent steer | thread 均可 | 🧊 v0 否（杀+重起） | — |

### 06 驱动形态与无人值守（招牌区）
| 功能 | Codex | AgentRunner | 缺口 |
|---|---|---|---|
| 定时 automations | 模板/历史/自定模型 | ✅ loop driver（S6） | 历史/模板是 UI 糖 |
| goal 长程目标 | 挂 thread、跑数天 | ✅ INC-D1+INC-10：context 延续 + 自证完成（goal_status/goal_complete 工具面、结构化 continuation、边界裁决）+ 可选 command verifier（**强于 Codex 的硬证据形态**）+ /goal 一句话直启 + banner edit | 余项：token/墙钟预算、blocked/usage_limited 态（§6.2-④⑤，记档 defer） |
| best-of-N | 云端多方案 | ✅ 隔离 worktree + verifier（S7） | 胜者晋升 G15 🧊 |
| verifier 管线 | 评分黑盒 | ✅ **领先** in-session/driver 均 journaled + approval + OS containment evidence | — |
| 外部事件唤醒 | GitHub/Linear/Slack | ❌ inbox 原语备，投递壳缺 | **G14** |
| thread automations | 带上下文定时 | ❌ loop 是 fresh-run 批式 | 随 G23 收编 |
| cron 跨重启 | 云端天然 | 🧊 backlog | 随 G22 收编 |

### 07 持久化、恢复、时间旅行（护城河）
| 功能 | Codex | AgentRunner | 缺口 |
|---|---|---|---|
| resume/跨日 | sessions & resume | ✅ snapshot-resume + send 复活 | — |
| crash 恢复契约 | 黑盒 | ✅ **领先** journal+fold+in-doubt+矩阵（QA-08） | — |
| 重启自动接续 | 云端托管 | ❌ 躺到 send 才复活 | G22 boot sweep |
| 会话 fork | 任意消息 fork | ✅ **领先** barrier fork 带 workspace 快照 | 任意点=每 turn 落 barrier |
| rewind | 靠 fork | ✅ **领先**（S7） | — |
| 跨机续作 | 云端天然 | ❌ store 本机 | 归 G11 |

### 08 平台与生态（定位差异最大层，逐项裁决"追/不追")
| 功能 | Codex | AgentRunner | 缺口 |
|---|---|---|---|
| 云端执行环境 | 环境/secrets/缓存/并行 | 🧊 G11（S7 裁掉） | XL，P2 门槛项 |
| 移动端派活 | iOS 全功能 | ❌ 远程 attach/审批走 webui | 依赖 G11 |
| IDE 扩展 | VS Code + 同步 | 🧊 S7 cut line | MCP server 化替代 |
| GitHub 集成 | @codex/Action/review | ❌ | P2，依赖 G14 |
| Slack/Linear | @Codex/回写 | ❌ MCP 可桥读写 | P2/P3 |
| MCP client | 支持 | ✅ stdio + streamable HTTP、env OAuth bearer、resources/prompts/list_changed（INC-11.4） | 交互 OAuth 登录 UX 🧊 |
| MCP server（自暴露） | codex mcp-server | ❌ 未登记 | **建议新立** |
| skills | $语法/自动选/record&replay | 🟡 Claude Code 约定读侧注入（S5） | $调用+自动选 |
| plugins 捆绑 | 安装包 + marketplace | ❌ 原语都在 | P2（manifest+安装器） |
| SDK/无头 | TS SDK/Action | 🟡 CLI --json 事实无头可用 | 薄 SDK 包装 |
| 多 provider | OpenAI 系 | ✅ **领先** Gemini+Anthropic | — |
| 桌面 app 级 | 多窗/托盘/computer use/主题 | 🧊 webui 测试舱定位 | 需新 journey |
| 企业面 | RBAC/analytics | 🧊 非目标 | — |

---

## §3 实现进度（本轮 Codex-parity 冲刺，随增量更新）

| 项 | GAPS | 状态 | 增量 / 锚 |
|---|---|---|---|
| grep / glob 独立工具 | G18a | ✅ 已实现 | INC-3 · TestGrep*/TestGlob* · QA-11 |
| 远程 stop | G12 | ✅ 已实现 | INC-4 · TestStop* · 真 daemon 手验 |
| 自定义命令 / slash | G21 | ✅ 已实现 | INC-8 · TestExpand* · 真实 API |
| 手动 compact / clear | G7 | ✅ 已实现 | INC-6 · TestManualCompact/Clear · QA-12（真验捕获并修 idle-compact 空 summary bug） |
| web_fetch | G18b | ✅ 并行线已实现 | INC-5 · TestWebFetch* · QA-13（本会话 INC-D3 设计稿主张走 §4；程序争点 LOG 待裁） |
| ask_user（向用户提问） | G20 | ✅ 并行线已实现 | INC-5.2 · TestAskUser* · QA-13 |
| webui 改动视图白屏 + UX | — | ✅ 他会话已修 | diff 白屏 + UX-01..05（4e316de/672de7c）；余 UI 项（markdown/usage/搜索/归档）后续 |
| webui composer 对标 Codex | — | ✅ 已实现 | 24aeccb · Codex 风格 composer：权限模式/model/slash/`+`菜单/Goal·Loop 启动器/语音(Web Speech)/git 分支 pill；真 Gemini 验(新会话+turn、mid-session 换 model、Goal 达成、分支列举)。PDF 二进制附件仍待产品 file-part 增量 |
| 会话内 goal | G23 | ✅ v1 已实现（含自证完成） | INC-D1（v0）+ INC-10（§6 深潜缺口 ①②③⑥⑦ 全关：自证裁决/goal 工具面/结构化 continuation/UI 收敛/控制 revive）· QA-16+QA-17 真验 · ④⑤（token/墙钟预算、blocked/usage_limited）记档 defer |
| 事件唤醒既有 session | G14 | 📐 设计稿 | INC-D2（invariant-adjacent，机器发送方信任条款） |
| 记忆写回（# remember） | G9 | 📐 设计稿 | INC-D4（取 A 不触不变量；待裁 A/B） |
| 审批“允许且不再问” | G5 | 📐 设计稿 | INC-D5（取 A 下次生效不触不变量；待裁 A/B） |
| 任务→diff 审阅门→PR | G13 | 📐 设计优先 | 依赖 G14；未起草 |
| 云 workspace 生命周期 | G11 | 🧊 门槛/待裁 | XL，先裁"要不要云形态" |

图例：✅ 已实现 · 📐 设计稿（docs/increments/INC-D*，待裁决/不变量 review）· 🧊 推迟。

**本会话已实现（4 个引擎增量，双闸门全绿并推 main）**：grep/glob（INC-3）、
远程 stop（INC-4）、自定义命令（INC-8）、手动 compact/clear（INC-6，真验
捕获并修一个 idle-compact 空 summary bug）。**已起草设计稿（5 份，
docs/increments/INC-D1–D5）**：会话内 goal、事件唤醒、web 工具、记忆写回、
审批持久化——其中 D1/D3 触不变量须走 PROCESS §4，D2 引入 ingress 须安全
review，D4/D5 有"下次生效"的不触不变量最小路径待裁。

**并行 session 同期落地（非本会话，记录以保审计诚实）**：web_fetch +
ask_user（INC-5）、webui diff 白屏 + UX 修复。其中 web_fetch 与本会话
INC-D3 设计稿存**程序争点**（该改动是"收容棘轮不变量升级须走 §4"还是
"覆盖面扩展随实现修订"）——技术方向一致（egress 统一 fail-closed），
分歧纯在程序,LOG 已记档待开发者裁决。

---

## §4 填补路线图（四档）

- **P0 日用体感（多 S 号，本轮批量）**：G18 grep/glob（✅）→ G7 → G5 →
  G9 → G21 → G12 + webui 白屏/选择器/usage/markdown。
- **P1 Codex 核心工作流（M/L）**：G23 会话内 goal（✅ v0；余项 §6.2，其中
  ①语义洞 bug 级、②③为下一刀）
  + G14 事件唤醒 + G13 diff→PR + G22 boot sweep + G10 子进度 + skills 进阶
  + ar 自身 MCP server 化 + 内容级搜索/归档。
- **P2 平台化（L/XL，先裁再做）**：G11 云环境（门槛项）+ GitHub 集成 +
  plugins 捆绑 + SDK 薄包装 + MCP http+OAuth + macOS 沙箱 + memories 系统。
- **P3 明示裁决（记入 JOURNEYS「有意不覆盖」）**：computer use / 内建浏览器 /
  图像生成 / 语音 / 桌面 app 化 / 企业面 / marketplace / IDE 扩展。

---

## §5 审计基线与参照来源

- **我方基线**：origin/main（webui 与 web/ 已合流；grep/glob = INC-3）。
- **Codex 参照**：developers.openai.com/codex（docs 结构）、/codex/changelog
  （2025-09→2026-07 全量）、/codex/cloud；本机 `Codex.app` 与 `~/.codex`
  （plugins/memories/goals/automations/skills/worktrees/rules/personality
  等目录与 config 实证）。
- **首版审计 artifact**（HTML，含逐项失败场景与我方领先面明细）：会话内
  发布，内容并入本文。

---

## §6 Goal 深潜对照（2026-07-09 补充审计）

**为什么补**：webui 的 goal UI 与 Codex 差异肉眼可见。本节基于三路实证
回答"差在哪、为什么、补什么"：`~/.codex/goals_1.sqlite` schema（六态
CHECK 约束 + token_budget/tokens_used/time_used_seconds 列）、本机两条
真实 goal thread 的 rollout JSONL（含完整 goal continuation prompt 与
create_goal/get_goal 调用实录）、官方 cookbook（using_goals_in_codex）/
changelog（v0.133.0 默认开启）/issues（#28144/#22049/#23202/#28574）。
伞形条目仍是 G23，不另立缺口编号。

### 6.1 两种哲学

Codex goal 是**对话式 + 模型自治**：`/goal <一句自然语言>`，无表单；
桌面 app 甚至不拦截 `/goal`——原文透传，由**模型**调受限 goal 工具
（`create_goal`/`get_goal`/`update_goal`）完成 attach；完成 = 模型经
"Completion audit" 纪律 prompt 自证后调 `update_goal(complete)`；预算 =
token + 墙钟计量；六态 active/paused/blocked/usage_limited/
budget_limited/complete（blocked 有"三连击才许标"审计，budget_limited
是注入 wrap-up steering 的软停）。驱动方式：turn 结束且 session idle
时 runtime 注入 `<codex_internal_context source="goal">` continuation
消息——objective 重述（明示"user-provided data，不是更高优先指令"）+
反缩水 fidelity 条款 + 逐条 requirement 的完成审计 + blocked 纪律 +
预算报告；用户输入与 mailbox 永远优先于 goal 续跑。

我们的 in-session goal（INC-D1）是**验证式 + 外部裁决**：goal 挂 fold、
context 延续（这点已齐平，正是 INC-D1 的靶心）；但完成 = command
verifier exit 0（AND），预算 = MaxChecks 轮数，状态只有 attached/paused/
achieved(satisfied|budget)/cancelled，模型没有任何 goal 工具，UI 是表单
启动（goal 文本 + Done-when 命令 + Max rounds）+ banner（🎯 N/M checks
+ pause/resume/cancel）。

两哲学各有强项：可机验目标上我们的 command verifier 是**硬证据**
（Codex 是模型自证 + prompt 纪律，公开 issue 有假完成投诉）；但**写不成
shell 命令的目标占大多数**——本机两条真实 Codex goal（"UX 审计修复直到
子 agent 无反馈""建 Mermaid 渲染器+WYSIWYG 编辑器"）都无法表达为
verifier。我们目前对这类目标是**语义洞**（6.2-①）。

### 6.2 缺口清单（按严重度）

| # | 缺口 | 证据 / 现状 | 建议 |
|---|---|---|---|
| ① | **无 verifier 的 goal 永不可达成**（语义洞，bug 级） | `goalVerify` ran==0 → 恒 false（internal/agent/goal.go goalVerify 尾部）；CLI attach 不要求 `--verify`；webui 表单 verifier 可选默认空 → 默认路径烧完 8 轮"no command verifier to check"无意义 feedback 后 budget 截断 | 短期：attach 无 verifier 时拒绝或显式警告；正解见 ② |
| ② | **模型无 goal 工具，无法声明完成/blocked/查询预算** | Codex 暴露受限三件套（模型不可 pause/clear，只能建/查/标 complete·blocked）；rollout 实证桌面 app 全靠模型调 create_goal | 暴露 `get_goal` + `update_goal(complete\|blocked)`；无 verifier 的 goal 走模型自证 + 完成审计 prompt；有 verifier 时两者 AND（比 Codex 强的混合形态） |
| ③ | **continuation 回灌太薄** | 我们 miss 后只回灌 `` `cmd` exit=1 ``；Codex 注入整页协议（objective 重述/fidelity 反缩水/completion audit/blocked 纪律/预算报告） | checkpoint 回灌升级为结构化 continuation prompt——纯 prompt 工程，低成本高杠杆；全文已存档可参照 |
| ④ | 预算只有轮数，无 token/墙钟 | Codex：token_budget + time_used_seconds 逐 turn 报给模型；budget_limited 为 wrap-up 软停 | INC-D1 归档注记已 defer 的余项；落地时 banner 同步显示 elapsed + tokens |
| ⑤ | 无 blocked / usage_limited 态 | Codex blocked 有三连击审计；usage_limited 撞 5h 用量窗自动停（本机两条 goal 终态皆此） | blocked 可与我们的 ask_user（park WAITING_INPUT）打通——比 Codex 死停更好；usage_limited 对应 provider 429/quota 自动 pause + 窗口重置自动 resume（Codex 社区正在要这个，#28931） |
| ⑥ | UI：无 edit、无用量显示 | update 走 CLI/API 全就绪（前端 api.ts 已声明 "update" 但无组件调用）；banner 只有 N/M checks | banner 加 edit + elapsed/tokens；表单收敛为"一句话即走"，verifier/预算降为高级选项 |
| ⑦ | idle 会话 attach 不复活 | INC-D1 归档注记明列 | 已记 LOG 余项 |

### 6.3 webui goal UI 的对标结论

信息架构上我们并不落后：Codex 也没有 goal 列表页（CLI 是 footer 一行
chrome：objective + status + elapsed + tokens；桌面 app 原生 /goal 至今
是 open feature request #22049/#23202）——banner ≈ footer，齐平。真正的
差距不在"页面"而在**语义与启动形态**：表单三字段 vs 一句话；外部
verifier 强制 vs 模型自证默认。修 ①②③ 后把表单收敛成"输入目标即走"，
goal UI 的"很不同"即消失。

**收口（同日，INC-10）**：①②③⑥⑦ 已全部实现并双闸门验收
（TestInSessionGoal* 7 条 + TestGoalResumeCheck/TestGoalClaimFold +
QA-17 真 Gemini 自证达成 + webui Chrome 真跑）。现形态 = Codex 的对话式
自证（/goal 一句话、goal_complete 声明、审计式 continuation）**加**我们
独有的可选 command verifier 硬裁决——两哲学合流，verifier 存在时它仍是
唯一裁决者。④ token/墙钟预算与 ⑤ blocked/usage_limited 记档 defer
（LOG）。对抗 review 连带修复三处主干潜红（缓存掩盖的 red test、socket
路径超限、INC-D1 wake seam 空转），细节见 LOG 2026-07-09 INC-10 条。

### 6.4 新证据的连带发现（非 goal，登记备查）

rollout JSONL 里 Codex 模型侧工具面还有：`update_plan`（计划可见性，
goal continuation prompt 明文引用它做 progress visibility——我们无等价
物，webui 也无 plan/todo 呈现，矩阵未收此项，候补）、`spawn_agent/
wait_agent/close_agent`（子 agent 三件套，对应 §05 子进度 G10）、`js/
js_reset/js_add_node_module_dir`（node REPL 持久执行环境）、
`read_thread_terminal/write_stdin`（终端交互，对应 §02"UI 无终端面板"）、
`view_image`、`load_workspace_dependencies`。`~/.codex` 另见
ambient-suggestions / pets / memories_1.sqlite / transcription-history /
process_manager 等目录（记忆系统对应 G9）。goal 存储已从早期 `goals/`
目录演进为 `goals_1.sqlite`（thread_id 主键 = 一 thread 一 goal，与我们
"一 session 一 goal"同构）。

---

## §7 Web UI 全表面持续证据矩阵（INC-98 / QA-88）

**用途**：这是可执行 backlog，不是“看起来差不多”的结论。状态只允许
`UNTESTED / BLOCKED / GAP / PASS / INTENTIONAL`。`PASS` 必须有同批真实 Codex
实窗与 AgentRunner shared-store 交互证据；component test 或单侧截图只能作回归锚，
不能单独升级状态。证据日期过旧或产品变化时退回 `UNTESTED`。`GAP` 必须引用
GAPS；`INTENTIONAL` 必须说明不属于哪条 journey。

首批证据根：`qa/runs/2026-07-22-QA88-codex-ui-continuous-loop/`。其中 `07` 是
未稳定的 Pull Requests skeleton，明确拒收；稳定图为 `10`。以下未填证据的行就是
后续 loop 的执行队列，不能因同组另一行通过而批量判绿。

**98.3a 盘点**：79 行 = `PASS 18 / GAP 7 / INTENTIONAL 4 / BLOCKED 1 /
UNTESTED 49`。PASS 中 New session/Scheduled/Environment/Thread 各有多行交叉锚，因此它们
不是 7 个完整页面已测完；任何组内仍有 UNTESTED 就继续留在 loop。

### 7.1 Global shell 与 Codex-only 主入口

| ID | surface/state/action | 状态 | 最近证据 / 裁决 |
|---|---|---|---|
| GL-01 | New chat 默认桌面 shell | PASS | 2026-07-22：Codex `06` ↔ AgentRunner `13`（同为逻辑 1952×1465 light）；同为 sidebar + 单 composer + context strip |
| GL-02 | 当前 thread/session 桌面 shell | PASS | 2026-07-22 `QA88-98.3a-thread-actions/01..03/08`：Codex task thread/reference crop 与 AgentRunner 1280×720 completed thread；sidebar/header/thread/composer 同层，工具明细只在 disclosure 后展开 |
| GL-03 | command palette：open/search/empty/keyboard/Escape focus return | PASS | 2026-07-22 `QA88-98.2-global-new-session/05..09`：双侧真实 query；我方命令可见/可执行、no-match、↓至第 10 项仍在 viewport、Escape 回 Search trigger；390×844 边界内 |
| GL-04 | sidebar Search：标题/内容/项目/空结果/keyboard | GAP | 2026-07-22 标题/ID/workspace、no-match/键盘已真测；Codex `05/13` 可以命中消息正文并给 snippet，我方无 journal full-text backend/index，G44 |
| GL-05 | Pinned/Projects 分组、fold、resize、scroll、overflow | UNTESTED | 2026-07-22 `QA88-98.2b-sidebar/13/18..20/26/27/30` 已真测我方 220–480 resize、320 默认/刷新持久、fold、269 project overflow/scroll；Codex 尚缺同批 fold/overflow 全态，不能判 PASS |
| GL-06 | session row hover/focus/current/running/unread/attention | UNTESTED | 2026-07-22 `21..24` 已在 shared store 捕获我方 current/approval/failed/running 及 focus actions，stranded/unread 同屏；running QA session 永久保留；Codex 尚缺同批全状态，不能判 PASS |
| GL-07 | project/session context menu + Escape focus return | PASS | 2026-07-22 `05/06/14..16/28/29`：双侧右键菜单真测；project 六项同构；我方 session 四项为已裁决轻表面，右键/Shift+F10 同源且 Escape 回原 row；Codex Shift+F10 只落 focus、不弹菜单，我方键盘能力更完整 |
| GL-08 | user/account menu、update/status、sign-out 边界 | INTENTIONAL | 2026-07-22 Codex `09/10` 有 product switcher、Usage/Pet/Settings/Log out；我方 `17` 为本地单用户 Connected/Settings/Shortcuts/Theme，不承诺 account/sign-out/usage，更新走 UJ-25 CLI；若引入多租户/auth 必须另立 journey |
| GL-09 | Pull Requests list/search/filter/detail/loading/empty | GAP | 2026-07-22 Codex `10`；AgentRunner 缺 session→review→PR 产品面，G13；`07` loading 图拒收 |
| GL-10 | Sites empty/list/create/publish | INTENTIONAL | Codex `08`；网站托管不属于 UJ-01..25，除非另立 journey |
| GL-11 | Scheduled 主 shell/list/search/filter | PASS | 2026-07-22：Codex `05` ↔ AgentRunner `14`（同为逻辑 1952×1465 light）；只判 shell，细态见 SC 组 |
| GL-12 | Plugins installed/marketplace/search/install/update | GAP | Codex `09`；统一包生命周期缺失，G43 |
| GL-13 | 全局 loading/offline/reconnect/version mismatch | UNTESTED | 待不破坏 shared daemon 的可控路径；kill 类操作需单独授权 |

### 7.2 New session 与 composer

| ID | surface/state/action | 状态 | 最近证据 / 裁决 |
|---|---|---|---|
| NS-01 | projectless 默认首页、starter cards、首屏 geometry | PASS | 2026-07-22 Codex `06` ↔ AgentRunner `13`；只判同 viewport 默认视觉/层级 |
| NS-02 | Project picker：搜索/无结果/选择/清除/focus return | PASS | 2026-07-22 `QA88-98.2c-new-session/03..04/19..20`：双侧真实 project search/no-result；AgentRunner 选择/清除切换 headline+context controls，Escape 回 project opener |
| NS-03 | Local/New worktree + Environment + Branch：展开/搜索/invalid/ref | GAP | 2026-07-22 `05..06/13/21..24/55/58`：双侧 Local/New worktree 与 Branch open/main/no-match 当前实证通过；AgentRunner 缺 Codex `No environment → Create local environment` 与 Cloud/usage 生命周期，G11；不画假 chip |
| NS-04 | starter card：短 intent → 四条 suggestion、清空恢复、Send 前不创建 session | PASS | 2026-07-22 Codex `17/40/49/50` 四 intent ↔ AgentRunner 修后 `59..62`；`Home.starters.test.tsx` 钉 4 intents/16 suggestions/clear/no-send；root URL 保持 `/` |
| NS-05 | composer textarea：empty/multiline/overflow/IME/CJK | UNTESTED | 2026-07-22 `QA88-98.2e-input-attachments/19/30..33`：Codex driver 真实写入并自动清理 4/20 行 CJK；同逻辑 1952×1465 对照中我方由约 8 行放宽至约 15 行可见，Codex 约 18 行，controls 均在 viewport。真实 IME composition 尚缺，不能判 PASS |
| NS-06 | attachment：image/file/PDF/oversize/upload error/remove | UNTESTED | 2026-07-22 `QA88-98.2e-input-attachments/02..17`：Add→Files and folders 可打开原生 Open sheet；宿主 Codex 的 Computer Use 安全边界及 panel service 阻止可靠选择/移除，全部附件校准图拒收，未落半工作 driver，不能判 PASS/GAP |
| NS-07 | Add root + Automation/Agent/YAML 子页、outside/Escape | PASS | 2026-07-22 Codex `14` Add root ↔ AgentRunner `26..29`；真实 nested Automation→Agent→YAML，无 Send；outside 关闭，Escape 关闭并回 `Add and advanced options` opener |
| NS-08 | access/model/thinking picker：搜索/切换/disabled/error | GAP | 2026-07-22 `QA88-98.2d-model-access/01..10/20..34` 同逻辑 1952×1465：双侧 root/model/effort/speed/access 真实展开；我方 Ask→Full 可逆切换，Model/Effort/Advanced 可达；Codex 有真实 `Fast` service tier 与 `Ultra`，我方移除无行为的单项 `Speed→Standard` 并记 G45。disabled/error 仍须在后续该 GAP 内继续取证 |
| NS-09 | Goal/Plan/Loop/Best-of-N/background 启动表面 | PASS | 2026-07-22 `QA88-98.2f-goal-automation/08..24`：Codex Goal/Plan on-state ↔ AgentRunner 同 viewport；我方修复 Goal 双 composer 为 inline mode + options chip，Plan Add 可逆且恢复 prior access；Loop/Best-of-N/background/Agent→YAML 为 UJ-14/18/22 的 truthful Automation 子页，显式 Start 前均无 session/Send。98.2g 发现 Codex Plan 跨 New chat sticky，driver 改为从 Add 真 off 并验证 |
| NS-10 | Send：validation/submitting/streaming/failure/retry | UNTESTED | 2026-07-22 `QA88-98.2h-send-states/01..33`：双侧真实 Send、running、completed 与同逻辑 1952×1465 合并图；AgentRunner 另有 empty validation、draft-preserving submit、queued/Stop、invalid-model failure→换 model→Retry success，3 个 shared session 永久保留。Codex failure/retry 尚无可控同态证据，故不提前判 PASS |
| NS-11 | voice/dictation | INTENTIONAL | AgentRunner 核心 journey 不要求语音；Web Speech 既有实验入口不作为 parity 承诺 |
| NS-12 | desktop 1840/1280、mobile 390、light/dark | PASS | 2026-07-22 `QA88-98.2g-responsive-theme/01..18/21`：1280×720、1840×1000、1952×1465、390×844 的 home/settings/sidebar 均无横溢；`21` 为双侧同逻辑 1952×1465 clean New chat 合并图；mobile composer/controls 全在 viewport |

### 7.3 Thread、timeline 与运行中协作

| ID | surface/state/action | 状态 | 最近证据 / 裁决 |
|---|---|---|---|
| TH-01 | user/assistant/system/tool 消息层级与 streaming | PASS | 2026-07-22 `QA88-98.2h-send-states/19/20/28/31` + `QA88-98.3a-thread-actions/01..03`：双侧真实 `sleep 8` running/completed；user 右对齐、assistant prose、system 默认隐藏、tool 进 Worked，streaming 不改层级 |
| TH-02 | thinking/tool call/result：collapsed/expanded/long/error | UNTESTED | — |
| TH-03 | Markdown：heading/list/table/code/mermaid/math/link/media | GAP | 基础/mermaid 历史已测；math 缺 G38；其余需当前对标 |
| TH-04 | Worked duration、Copy、feedback、Open artifact | GAP | 2026-07-22 `QA88-98.3a-thread-actions/01..03/05..08`：Worked/tool 两级展开、message/tool Copy 内容、artifact Open/Download URL 与 file Review 实测通过；Codex 👍/👎 feedback 缺 backend event/identity/privacy/receipt，G46；不画假按钮 |
| TH-05 | message Continue：human/final/legacy/attachment-only | PASS | QA-82 + 2026-07-22 `QA88-98.3a-thread-actions/04`：human-before 生成零 turn dormant child + recorded draft；final-assistant-after 生成完整 cut dormant child；parent 不变，legacy/非 final/attachment-only 仍由 QA-82 锚 |
| TH-06 | queue/steer toggle、queued bubbles、⌘⏎、reorder 边界 | UNTESTED | — |
| TH-07 | running Stop/interrupt、partial output、recovery | UNTESTED | — |
| TH-08 | ask_user waiting、answer、reload、cancel 边界 | UNTESTED | — |
| TH-09 | approval：details/approve/deny/child approval/reload | UNTESTED | 不替用户决定真实高风险审批；使用 QA spec |
| TH-10 | provider/network/tool error、Retry 原位替换 | UNTESTED | — |
| TH-11 | completed/failed/recovery/continue terminal chrome | UNTESTED | — |
| TH-12 | long thread hydration/scroll anchor/new message badge | UNTESTED | — |
| TH-13 | goal banner：active/pause/edit/blocked/budget/complete | UNTESTED | — |
| TH-14 | plan/progress visibility 与完成真实性 | UNTESTED | Codex 有 update_plan；我方整体表 progress 需逐态比较 |

### 7.4 Environment、Agents 与 Changes

| ID | surface/state/action | 状态 | 最近证据 / 裁决 |
|---|---|---|---|
| EV-01 | Environment desktop 浮卡，不重排 thread/composer | PASS | 2026-07-22 QA-87：Codex 实窗 ↔ AgentRunner 1840/1280 |
| EV-02 | Environment mobile containment/scroll/close | PASS | QA-87 390×844 实窗几何；Codex desktop contract + 我方 mobile 实测 |
| EV-03 | Goal/Agents/Attention/Background empty/populated/overflow | UNTESTED | — |
| EV-04 | child member navigation/full timeline/back/reload | UNTESTED | — |
| EV-05 | child approval/recovery attention 与去重 | UNTESTED | — |
| EV-06 | Environment↔Changes/sidebar/settings overlay 互斥 | UNTESTED | QA-87/86 为我方候选证据，待全组合 |
| CH-01 | Changes empty/normal + Working tree/Last turn scopes | UNTESTED | — |
| CH-02 | added/modified/deleted/renamed/untracked/staged | UNTESTED | — |
| CH-03 | large/generated/binary/unavailable baseline disclosure | UNTESTED | — |
| CH-04 | file/hunk expand-collapse、scroll、syntax/CJK/wrap | UNTESTED | — |
| CH-05 | desktop split/mobile overlay + close/focus return | UNTESTED | QA-87 为我方候选证据，待 Codex 同状态 |
| CH-06 | Commit or push menu/validation/progress/error/success | UNTESTED | 不执行真实 push；可用专用 shared QA repo 做 commit |
| CH-07 | Apply to project/Remove worktree/dirty/conflict guards | UNTESTED | AgentRunner 特有强能力，需验证而非强行克隆 Codex |

### 7.5 Scheduled、Settings 与持久化

| ID | surface/state/action | 状态 | 最近证据 / 裁决 |
|---|---|---|---|
| SC-01 | Scheduled list/search/filter 主 shell | PASS | 2026-07-22 Codex `05` ↔ AgentRunner `14` |
| SC-02 | empty/large list/loading/error/pagination/scroll | UNTESTED | `12` 仅大列表静态首屏 |
| SC-03 | Create：one-shot/repeating/validation/cancel/success | UNTESTED | — |
| SC-04 | suggestions：launch/prefill/dismiss | UNTESTED | — |
| SC-05 | active/paused/finished/failed/overlap/retry | UNTESTED | 先对齐两侧状态语义，再判断 All/Active/Paused vs Finished 文案 |
| SC-06 | run detail/deep link/edit/next run/history | UNTESTED | — |
| SC-07 | restart 后 cadence/nextRun/status truthful | UNTESTED | shared daemon 安全 restart，不 kill -9 |
| ST-01 | Settings open/close/general/appearance | UNTESTED | — |
| ST-02 | theme light/dark/system + persistence/no flash | PASS | 2026-07-22 `QA88-98.2g-responsive-theme/01..15/22`：Settings 真实切 light/dark/system，desktop/mobile 截图；三态 reload 持久，System 无 `data-theme` 并跟随 media；新增 parser-blocking `theme-init.js` 在 body/main 首 paint 前恢复显式主题，theme-color 同步；`theme.test.ts` |
| ST-03 | shortcuts/config/worktrees/archived sessions | UNTESTED | — |
| ST-04 | desktop Done/mobile Back/Escape/outside focus return | UNTESTED | QA-80/86 为我方候选证据 |
| ST-05 | profile/voice/pets/personalization | INTENTIONAL | 非 AgentRunner runtime journey；若形成工程效益另立产品 delta |
| PS-01 | deep-link reload/back/forward/current selection | UNTESTED | — |
| PS-02 | rename/pin/archive/read/remove + persistence | UNTESTED | Archive/Remove 必须用专用 shared QA data，永久保留 journal |
| PS-03 | legacy/missing/corrupt session 与 projectless rows | UNTESTED | 不伪造 raw id/empty state |

### 7.6 Accessibility、viewport 与 resilience 横切

| ID | surface/state/action | 状态 | 最近证据 / 裁决 |
|---|---|---|---|
| AX-01 | 全主流程 keyboard-only、tab order、visible focus | UNTESTED | — |
| AX-02 | menu/dialog/listbox semantics + focus trap/return | UNTESTED | — |
| AX-03 | pointer/touch target ≥44px、hover 不作为唯一入口 | UNTESTED | — |
| AX-04 | text/icon/status contrast，light/dark/error/disabled | UNTESTED | — |
| AX-05 | 200% zoom/reflow、390×844、1280×720、1840×1353 | UNTESTED | QA-87 只覆盖 Environment/Changes geometry |
| AX-06 | reduced motion/animation/loading transitions | UNTESTED | — |
| AX-07 | VoiceOver accessible names/roles/state/announcements | BLOCKED | Codex Electron AX tree 本机仅暴露顶层 group；先用 keyboard/视觉证据，持续探测语义树 |
| AX-08 | CJK/emoji/long path/long title/RTL | UNTESTED | — |
| RS-01 | refresh/restart during loading/running/waiting | UNTESTED | destructive daemon kill 需另行授权；普通 restart 可测 |
| RS-02 | offline→online、API 4xx/5xx、version mismatch | UNTESTED | — |
| RS-03 | browser console warning/error、layout overflow | UNTESTED | 每批必存；首批 New/Scheduled logs=`[]` |
| RS-04 | screenshot driver PID/window/target/recovery fail-closed | PASS | 2026-07-22：CGWindow/CGEvent 实跑；`scripts/test-capture-codex-ui.sh` contract |
