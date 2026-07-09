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

我们**领先**于 Codex 可见承诺的面（报告的另一半事实）：durable mailbox
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
| 运行中 steering | mid-turn / queue 双默认 | ✅ 安全边界排队（QA-02/06） | 语义差异，已裁决 |
| interrupt | Esc 停 turn | ✅ 真停 + 部分输出保留（裁决 #11） | — |
| 消息队列 | queue-by-default | ✅ **领先** durable mailbox | — |
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
| 语义检索 | 无等价物 | ✅ **领先** semantic_search | — |
| web search / fetch | 默认开，cached/live | ❌ 未 spec | G18 余项（网络+注入面） |
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
| OS 级沙箱 | seatbelt/landlock/win | 🟡 网络=netns（Linux），mac fail-closed | macOS 收容 |
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
| worktree 一等公民 | 每 thread worktree | 🟡 fork/best-of-N 已用隔离 | 泛化为 new/submit 选项 |
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
| goal 长程目标 | 挂 thread、跑数天 | 🟡 goal driver ✅ 但 fresh-run，context 不延续 | **G23/UJ-22** |
| best-of-N | 云端多方案 | ✅ 隔离 worktree + verifier（S7） | 胜者晋升 G15 🧊 |
| verifier 管线 | 评分黑盒 | ✅ **领先** journaled + trust 规则层 | — |
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
| MCP client | 支持 | ✅ stdio 全生命周期（S5） | http+OAuth 🧊 |
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
| 自定义命令 / slash | G21 | ✅ 已实现 | INC-5 · TestExpand* · 真实 API |
| 手动 compact / clear | G7 | ✅ 已实现 | INC-6 · TestManualCompact/Clear · QA-12（真验捕获并修 idle-compact 空 summary bug） |
| 审批“允许且不再问” | G5 | ⏳ 计划中 | INC-7 |
| 记忆写回（# remember） | G9 | 📐 设计优先 | 触 prefix-freeze 不变量 |
| webui 改动视图白屏 + UI | — | ⏳ 计划中 | webui 增量（非三层） |
| web fetch / search | G18b | 📐 设计优先 | 触 network+注入面，先 DESIGN 增量 |
| 会话内 goal | G23 | 📐 不变量变更流程 | UJ-22，需 PROCESS §4 |
| 事件唤醒既有 session | G14 | 📐 设计优先 | 输入投递机器发送方 |
| 任务→diff 审阅门→PR | G13 | 📐 设计优先 | — |
| 云 workspace 生命周期 | G11 | 🧊 门槛/待裁 | XL，先裁"要不要云形态" |

图例：✅ 已实现 · ⏳ 计划本轮 · 📐 需先走设计/不变量流程 · 🧊 推迟。

---

## §4 填补路线图（四档）

- **P0 日用体感（多 S 号，本轮批量）**：G18 grep/glob（✅）→ G7 → G5 →
  G9 → G21 → G12 + webui 白屏/选择器/usage/markdown。
- **P1 Codex 核心工作流（M/L）**：G23 会话内 goal（不变量变更流程，最高优先）
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
