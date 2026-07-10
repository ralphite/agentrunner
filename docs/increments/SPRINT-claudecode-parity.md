# SPRINT — CLAUDECODE-PARITY 补齐冲刺（loop 总控 · 活文档）

**这是什么**：把 docs/CLAUDECODE-PARITY.md §4.2 路线图队列化的**冲刺
总控**。一个自步调 loop 逐轮执行：每轮完成一项（或一项的一个阶段），
全程与并发 session 保持 sync。它不是 INC 工作纸——每个具体项开工时
仍按 PROCESS §二 另立 `INC-<n>-<slug>.md`；本文件只管**队列、认领、
状态、节律**。完成后按归档纪律移入 archive。

**并发协作约定（本 sprint 的存在理由之一）**：
- **认领 = push**。开工一项的第一个动作是把该项状态改 `🔧 in-progress
  (INC-<n>)` 并 push 本文件——其他 session 看到即避让；反之每轮选题前
  先 pull 本文件与 `git log`，已被认领/已被做掉的项直接跳过。
- **避让区**：INC-12 团队线（send_message/teams/webui 团队可见性）及
  其 webui 连带——另有 session 活跃；矩阵 #41/43/82/83 不在本队列。
- 冲突宁可让路换题，不抢同一文件区。

## 每轮 SOP（loop 每次 wakeup 执行一轮）

1. **Sync-in**：`git fetch origin main` → rebase/fast-forward；读最近
   commits 与本文件最新状态（别的 session 可能已关掉某项）。
2. **选题**：按队列序取第一个 `⬜ open` 且非避让、依赖已满足的项。
   - 触不变量的项：本轮只产出 PROCESS §4 变更单（或设计稿），状态标
     `📐 awaiting-review`，**不实现**，直接进入下一项。
   - 需外部凭据/服务的项（如 WebSearch 后端）：标 `⏸ blocked-external`
     并在轮报里向用户提需求，跳过。
3. **认领**：更新状态表 + 立 `INC-<n>` 工作纸（动机/journey 锚/Spec
   delta/Design delta/验收/实施步骤）→ **立即 commit+push**。
4. **实现 + 双闸门**：scripted 孪生测试 + 真实 API QA 场景（共享
   daemon/store，测试数据保留，`ar events` 导出归档 `qa/runs/`；一律
   真实端到端验证，不以单测代替）。
5. **文档行齐活**：SPEC.md 功能点、GAPS.md 状态、CLAUDECODE-PARITY §2
   对应行改 ✅（不删行）、涉及处同步 CODEX-PARITY、LOG.md 增量条目。
6. **Sync-out**：`git fetch` → rebase 最新 main → `./scripts/check.sh`
   全绿 → `git push origin HEAD:main`。
   - push 被拒 → fetch+rebase+check 再 push，最多 3 个来回；仍失败则
     标记本轮 `⚠ push-stuck` 并汇报。
   - rebase 冲突：docs 类（LOG/SPEC/GAPS/PARITY 状态行）按"两边都
     保留、状态取更先进者"合并；代码冲突语义合并后**必须**重跑
     check.sh 与相关孪生；**绝不 force push、绝不改写他人提交**。
7. **轮报**：更新本文件状态表（含轮次日志一行）并随同 push；向用户
   简报本轮产出。大项一轮未竟：状态表记阶段进度，下轮续做。

**终止条件**：队列全部 `✅/📐/⏸/🚫` → 停 loop 总结（📐/⏸ 项列清单
交用户裁决）；或用户叫停。

## 队列与状态

图例：⬜ open · 🔧 in-progress · ✅ done · 📐 awaiting-review（变更单/
设计稿待裁）· ⏸ blocked-external · 🚫 skipped（记原因）。
（矩阵号 = CLAUDECODE-PARITY §2 行号。）

### 第一梯队（P0）

| # | 项 | 矩阵/GAPS 锚 | 规模 | 状态 | 备注 |
|---|---|---|---|---|---|
| 1 | microcompact：assembly 层将可重算旧工具结果降级为占位符（read-class 且来源未变），不调 LLM | #18 · UJ-09 | S | ✅ done (INC-13) | 纯 assembly 策略，零事件变更；阈值先于 autocompact 触发；QA-22 真验 |
| 2 | G9 记忆写回核心（remember → 项目 CLAUDE.md，取 A） | #26-31 · G9 · INC-D4 | M | ✅ done (INC-14) | 写回核心 QA-23 真验；auto-memory 完整体拆为 #2b 余项 |
| 2b | auto-memory 完整体（MEMORY.md 索引 200 行/25KB + 主题文件 + @import + .claude/rules 条件加载） | #26-31 · G9 余项 | L | ⬜ | 对标 Claude Code auto-memory；独立增量 |
| 3 | G19 hooks 事件族第一批（SessionStart/End、UserPromptSubmit、Stop、SubagentStart/Stop、PreCompact/PostCompact），observe+block 语义不变 | #70-74 · G19 | M | ✅ done (INC-15) | 8 事件+2 blockable；QA-24 真验；P0 三件全部完成 |

### 第二梯队（P1）

| # | 项 | 矩阵/GAPS 锚 | 规模 | 状态 | 备注 |
|---|---|---|---|---|---|
| 4 | 权限规则工程三件套：复合命令逐段匹配、wrapper 剥离（timeout/nice/xargs 等）、只读命令免提示集 | #53 | M | ⬜ | 权限疲劳主解；纯 rules 层 |
| 5 | G5 审批"允许且不再问"（下次生效路径） | #58 · G5 · INC-D5 | M | ⬜ | INC-D5 已裁不触不变量路径 |
| 6 | protected paths 写保护集（.git/.claude/rc 文件等） | #59 | S | ⬜ | 配 acceptEdits 才安全 |
| 7 | skill 模型侧 invoke + context:fork（skill def 即 tool def；fork=spawn_agent 一次性变体） | #45 · §3.5 | M | ⬜ | 维持"命令=用户宏"裁决不动 |
| 8 | 结构化输出（`ar run --json-schema`，provider JSON mode 能力位） | #91 | S | ⬜ | verifier/集成两用 |
| 9 | checkpoint 增强：barrier 打点密度提至每 turn 收尾 + "仅对话"fork 变体 + compact 范围指示（Summarize-from-here 等价） | #12/13 · §3.1 | M | ⬜ | §3.1 已论证不触不变量 |
| 10 | ask_user 结构化选项（多选 + Other，向 AskUserQuestion 对齐） | #42 | S | ⬜ | webui 审批 UI 可复用 |
| 11 | read-before-edit 护栏（edit_file 要求本会话 Read 过且未变） | #32 | S | ⬜ | 纯工具层检查 |
| 12 | Grep/Glob 参数增强（output_mode/-A/-B/-C/multiline/type） | #35 | S | ⬜ | INC-3 延伸 |
| 13 | Read 工具多模态（读图/PDF 入 context，复用 CAS/part 管线） | #32 | M | ⬜ | 输入侧已通（INC-9），补工具侧 |
| 14 | Monitor 流式后台进度（每行输出即通知；并 G10 进度通道） | #34 · G10 | M | ⬜ | 与 bash output 拉取并存 |
| 15 | G22 boot sweep + cron 跨重启唤醒 | #87 · G22 | M | ⬜ | 无人值守自动性下半场 |
| 16 | 内置 agent 库（Explore/Plan 类只读 spec 随发行） | #78 | S | ⬜ | `ar init` 延伸 |
| 17 | webui 会话 rename/归档/内容搜索 | #2/3/7 | M | ⬜ | **注意避让**：webui 区与他 session 协调后再动 |
| 18 | auto mode 设计稿（分类器作为 effect pipeline 的 policy 源） | #57 · §3.3 | M(设计) | ⬜ | 设计先行 → 📐 待裁；依赖 #4/#5 |

### 第三梯队（P2，轮到时再评估）

| # | 项 | 矩阵锚 | 规模 | 状态 | 备注 |
|---|---|---|---|---|---|
| 19 | MCP tool search 式 deferred 加载 | #50 | M | ⬜ | 大工具面 context 优化 |
| 20 | LSP 工具 | #37 | L | ⬜ | 大件，届时先出 INC 设计 |
| 21 | WebSearch | #39 · G18 余项 | M | ⏸ | 需外部搜索 API 凭据——届时向用户要 |
| 22 | SDK 薄包装 / OTEL / plugins | #94/97/103 | L | ⬜ | 随 CODEX-PARITY §08 同批裁决，默认后置 |

## 轮次日志（每轮一行，追加）

| 轮 | 日期 | 项 | 结果 | commit |
|---|---|---|---|---|
| 3 | 2026-07-09 | #3 G19 hooks 事件族第一批 (INC-15) | ✅ 双闸门全绿（4 孪生 + QA-24 真 Gemini 四红线）；8 事件+2 blockable；P0 三件全部完成 | (见 push) |
| 2 | 2026-07-09 | #2 G9 记忆写回核心 (INC-14) | ✅ 双闸门全绿（memory 4 单元 + remember 2 孪生 + QA-23 真 Gemini：新 session 冻结遵循记住的约束）；auto-memory 完整体余项记档 | (见 push) |
| 1 | 2026-07-09 | #1 microcompact (INC-13) | ✅ 双闸门全绿（4 孪生 + QA-22 真 Gemini）；QA 编号连让 19/20/21 已被并发占→落 QA-22 | (见 push) |
