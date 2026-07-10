# INC-14 记忆写回（remember → 项目 CLAUDE.md，G9）— 取 A 落地

## 动机与 journey 锚

GAPS **G9**（设计欠定·中）+ UJ-09 步骤3「记住：这个项目一律用 pnpm →
写入项目记忆文件，之后的会话生效」。对标 CLAUDECODE-PARITY §2.04
#28-31（Claude Code auto-memory）。现状只有**读侧注入**（S3，memory 块
session start 冻结进 prefix）；写回缺失。本增量落地 INC-D4 设计稿的
**取 A（append-as-message）**——不触不变量的最小路径。

## Spec delta

- SPEC H「记忆写回（# remember → CLAUDE.md）」❌→✅，锚
  `TestMemoryAppend*`/`TestRememberControl*` + QA-23。
- CLAUDECODE-PARITY §2 #30（记忆写回）状态更新。

## Design delta（取 A，不触不变量）

按 INC-D4：
- **写哪个文件**：workspace-root `CLAUDE.md` only（项目记忆；WS 边界内，
  禁 ancestor/user-global）；**append-only，永不 overwrite**（保留既有）。
- **本 run 何时生效**：**取 A**——remember 的内容作为一条 program-source
  `InputReceived` 追加进当前对话（honors DESIGN §4「环境变化以追加消息
  进入上下文，绝不改写 prefix」，与 goal 回灌 goal.go:48 同构）；文件
  持久化供**下次** session start 冻结时读进 prefix。**不动 prefix →
  不触不变量**。
- **rewind/snapshot 交互**：CLAUDE.md 是 workspace 内容、非 journaled
  fold state → rewind 不 un-write（接受项，同 harness-config 排除，
  明示记档）。
- DESIGN §15 决策表加一行（记忆写回 = 项目 CLAUDE.md append + 取 A）；
  §4 prose 命名 memory writeback 为允许操作并说明如何守 prefix 稳定
  （取 A **不翻转**任何既有格）。

## 机制

- `internal/memory/memory.go` 加 `Append(root, note) error`：定位
  workspace-root CLAUDE.md（不存在则建），append 一行/段（前置换行分隔、
  trim），保留既有内容。
- `protocol.ControlRemember = "remember"`，复用 `Control.Directive` 载 note。
- `Loop.drainControls` 加 `case ControlRemember`：`memory.Append(wsRoot, note)`
  （`l.Exec.WS.Root()`）+ `appendE(InputReceived{Text:"[记忆] 已记入项目
  CLAUDE.md：<note>", Source:"program"})`。副作用（文件写）+ 对话可见
  （追加消息）。remember 与 compact/clear 同一 control 家族、走同一
  durable command / drainControls 路径。
- daemon `case "remember"` → `handleControl(Control{Kind:ControlRemember,
  Directive:cmd.Directive}, "remembering", enc)`。
- CLI `ar remember <sid> <text>`。

## 余项（G9 完整体，记档不在本增量）

auto-memory 的 MEMORY.md 索引（200 行/25KB）+ 主题文件按需读 +
per-agent agent-memory + @import 语法 + `.claude/rules` 条件加载——是
对标 Claude Code 的增强层，独立增量（挂 SPRINT #2 余项）。本增量只做
写回**核心手势**（remember → 项目 CLAUDE.md → 下次生效 + 本轮追加可见）。

## 验收

- 孪生：`TestMemoryAppendCreatesAndPreserves`（建/追加/保留既有）/
  `TestRememberControlWritesFileAndMessage`（control → 文件含该行 + 对话
  见 program input）/`TestRememberIdempotentCommand`（durable command_id
  幂等，跨 restart 不双写——remember 有文件副作用，须 in-doubt 安全）。
- 真实 API QA-23：`ar remember` 写入 pnpm 约束 → CLAUDE.md 含该行 →
  起**新** session → 注入生效（模型对话中受该约束）；`ar events` 归档
  qa/runs/。
- `./scripts/check.sh` 全绿。

## review 裁决

做。取 A = 小增量 inline 自审（INC-D4 已裁不触不变量）。写文件副作用
须走 durable command 幂等（避免崩溃重放双写）——这是本增量唯一的
correctness 关口，孪生 TestRememberIdempotentCommand 钉住。
