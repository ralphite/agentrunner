# INC-D4 记忆写回（# remember → CLAUDE.md，G9）— 设计稿

> **状态：设计稿，走 PROCESS §4 不变量变更流程；未实现。** 触碰 §4
> prefix 稳定性（环境块 session start 冻结）不变量。

## 动机与 journey 锚
GAPS **G9**（设计欠定·中）+ UJ-09 步骤3「记住：这个项目一律用 pnpm →
写入项目记忆文件,之后的会话生效」。对标 Codex：Memories（多区域 GA）+
Chronicle。我们只有读侧注入。

## 现状（读侧已建，正是"冻结"的样子）
- Loader：`internal/memory/memory.go` `Collect(root)` 从内向外走到 git
  root、`Render` 出 byte-stable `<memory>` 块（每段 `# CLAUDE.md (path)`,
  outermost first,nearest wins）。
- **冻结点**：`assembly.renderContextBlocks(wsRoot)` 在 session start
  调一次（loop.go），存进 `SessionStarted.Memory`,fold 进 `Session.Memory`。
- Assembly：`assembleSystem` 写固定 prefix 序 env→Memory→Skills→Agents→
  specPrompt→modeSuffix,**session start 冻结,逐 turn byte-identical**。

## 不变量与两个开问（须先裁）
1. **写哪个文件**：**workspace-root CLAUDE.md only**（项目记忆；WS.Resolve
   边界禁 ancestor/user-global；`~/.claude` 出信任+workspace 边界）；
   **append,永不 overwrite**。
2. **本 run 何时生效 vs 冻结不变量**——二选一定为 canonical：
   - **(A) append-as-message（推荐）**：把 remember 的内容当作一条**追加
     消息**进上下文（honors DESIGN §4「环境变化以追加消息进入上下文,
     绝不改写 prefix」）；文件持久化供**下次** session 冻结时读到。
     不动 prefix → **不触不变量**,最小。
   - **(B) MemoryChanged re-freeze 事件**：mirror 决策 #32——一次刻意的、
     journaled 的 cache 断裂,重冻 Memory 块,本 run 立即生效。触不变量,
     需走变更流程。
- **rewind/snapshot 交互**：CLAUDE.md 是 workspace 内容、非 journaled fold
  state → rewind **不会**un-write 它（明示为接受项,同 harness-config
  排除）。

## 机制草图（取 A，最小）
- 触发面：一条 `# remember <text>` 用户手势（或 `ar remember <sid> <text>`
  control）——把 `<text>` append 进 workspace-root CLAUDE.md（不存在则建,
  append-only,保留既有内容）,并作为一条追加消息进当前对话（"已记入项目
  记忆:<text>"）。下次 session start 冻结时该行进 prefix。
- `internal/memory/memory.go` 加 `Append(root, note)` 助手。
- 若日后要 B：加 MemoryChanged 事件 + state fold overwrite Session.Memory
  （mirror SpecChanged）。

## 波及面
- DESIGN §15 决策表加一行（写哪个文件 + 取 A/B）+ §4 prose 命名 memory
  writeback 为允许操作及如何守 prefix 稳定；取 A 则**不翻转**决策表任一
  既有格,只新增说明；取 B 走不变量变更流程。
- 代码：memory.Append；触发面（# remember 识别或 ar remember control,
  与 INC-6 control 家族同构）。
- SPEC H 行 G9；GAPS G9；QA 场景（remember → 文件含该行 → 新 session 注入）。

## 验收
- 孪生：Append 建/追加/保留既有；# remember → 文件含该行 + 当前对话见追加
  消息。
- 真实 API QA：remember pnpm 约束 → 起新 session → 注入生效（模型行为
  受约束）。

## review 裁决
取 A = 小增量 inline 自审；取 B = 走不变量变更流程 + 契约 review。本纸
仅设计,待裁 A/B。
