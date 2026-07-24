# INC-100：全分支取舍与原子开场附件收口

状态：COMPLETE（Gate A + Gate B）

## 三层 delta

- JOURNEYS：不新增 journey；补齐 UJ-04「从 Home 创建的首个 user turn 可同时携带
  text/image/file」的真实 Web UI 路径。
- SPEC：把 Web UI Home 从「new 纯文本 + send 附件」改为一次 create request，
  attachment-only 使用中性 caption；验收锚新增 INC-100 / QA-90。
- DESIGN：无不变量变更；复用 §17 已有 durable opening attachment、
  blob-before-event、typed ingress 与 CAS ref-only journal 契约。

## 审计方法与总账

2026-07-23 fetch 全部 remote 后，枚举 `refs/heads` 与
`refs/remotes/origin` 共 229 个 tip：

- 203 个 tip 已是 `origin/main` 祖先，无待合并代码。
- 26 个非祖先 tip 逐一执行 reachability、`git cherry origin/main <tip>`、
  diff/commit intent、当前代码/测试/history 对照。
- 11 个 tip 的全部提交为 patch-equivalent；不再合并。
- 14 个 tip 的 patch id 不同，但产品语义已被 main 的拆分/重写实现和后续修复覆盖；
  合并旧实现会回退现有架构或 UI，全部弃置。
- 仅 `origin/codex/conduct-comprehensive-quality-assurance-testing@893bb11f`
  留下一个未覆盖意图：Web UI 新会话的首条消息直接携带附件。该意图已按当前架构
  重做于 `cadd08e7`，不 cherry-pick 旧实现。

## 11 个 patch-equivalent tip

`git cherry` 的 `+` 均为 0，故不存在 main 未拥有的补丁：

| ref@tip | `+ / -` | 裁决 |
|---|---:|---|
| `main@6390e595` | `0 / 5` | patch-equivalent，弃置旧 ref |
| `worktree-agent-a1ef081866287793e@617fd68f` | `0 / 1` | 同上 |
| `worktree-agent-a674b41301cc14a4e@978f57b0` | `0 / 1` | 同上 |
| `worktree-agent-a757ca8418bb11f55@feb83b11` | `0 / 1` | 同上 |
| `worktree-agent-aa11a5a50cc060c94@cd4f2290` | `0 / 1` | 同上 |
| `worktree-agent-ac1924598b7435221@811c7810` | `0 / 1` | 同上 |
| `worktree-agent-ad573dc79915416e5@d47873ed` | `0 / 1` | 同上 |
| `worktree-agent-aeeffe4db3e385fb7@38a98976` | `0 / 1` | 同上 |
| `worktree-agent-af18d41241fe4432b@0dbf1ccb` | `0 / 1` | 同上 |
| `origin/claude/codex-blackbox-testing-8w3ky5@8772ba0b` | `0 / 26` | 同上 |
| `origin/claude/zen-colden-fbf3ea@c9d0f23a` | `0 / 3` | 同上 |

## 14 个语义已覆盖 tip

每行都给出当前 main 的 history 与仍存在的 code/test 锚；旧 tip 不合并。

| ref@tip | 旧改动意图 | 当前 main 覆盖证据与裁决 |
|---|---|---|
| `save-my-1245@6f42f5bc` | 动态团队角色 + child 提权审批合并实现 | `2cc41cc1` 动态角色、`c9307237`/`863e6b36` 审批安全、`df631099` child approval 可发现；`internal/agent/spawn_test.go`、`role_test.go`、`SupervisionPanel.test.tsx` 均在。main 已拆分且补全递归 child 路由，弃置旧合并提交。 |
| `worktree-agent-a0058b0d051f3b64c@c6047ab6` | assistant 图片栅格 + Edited-files 对齐 | `d6f604a5` inline image、`21d84f3b` 变更卡对齐、`a070dea0` 产出图去重；`Markdown.test.tsx` 与 `ChangesOutcome.tsx` 在。弃置旧两提交。 |
| `worktree-agent-a24fbf600b61b1f09@e7ccdce1` | 用户自定义 command tools | `2ca5b5df` 已落实现，`1246dd67` 已过真实 B 闸；`internal/commandtool/commandtool_test.go` 与 pipeline/tool tests 在。弃置旧 tip。 |
| `worktree-agent-a30c6f5b0c143e673@163bc0e9` | React Markdown/GFM/highlight/wrap | `05031849` 已落，`4ad204db` 真浏览器验收，`fb66858a` 后续补 Mermaid；`Markdown.tsx`/`Markdown.test.tsx` 在。弃置旧 tip。 |
| `worktree-agent-a3d7377001c4f2600@10514c89` | Scheduled cadence + next run | `760edb78` 已落；`27829a8b` 又把 cadence 权威收回 engine/CLI，删除 Web 自研解析；`internal/driver/cadence_test.go`、`Scheduled.list.test.tsx` 在。合并旧 Web 解析会架构回退，弃置。 |
| `worktree-agent-a6dcb51063ad6352a@b9a2bbbc` | LLM 自动标题 | `1cba5c0c` 已落，`018e1673` 真机验收；`internal/agent/autotitle.go`/`autotitle_test.go` 在。弃置旧 tip。 |
| `worktree-agent-a7d2aa6f71c473ef5@76994c06` | Scheduled 标题、左缘、无线条、筛选密度 | `0105b469`、`8af8ab82`、`3b07a87f` 已分批落地并继续收密度；`Scheduled.tsx`/`Scheduled.list.test.tsx` 在。弃置旧四提交。 |
| `worktree-agent-a968279c3066e0e13@a2c8cb2e` | diff caret、折叠判据、计数 | `f8a1834f` 与 `5d88d166` 已落并继续修正；`DiffView.tsx`、`DiffView.test.tsx`、`diffSummary.test.ts` 在。弃置旧两提交。 |
| `worktree-agent-abcf80fd5ee475c47@ee70ae9f` | project overlay + system launcher | `815ae1be` 已落，`018e1673` 真机验收，`60086773` 扩展现有 project controls；`webui/open_test.go` 与 Sidebar tests 在。弃置旧 tip。 |
| `worktree-agent-ac85db78a7de2aeb7@a7b32df4` | cron restart boot sweep | `34d3215e` 已落，`1246dd67` 真机验收，`27ef6d12` 后续补 graceful shutdown 保活；`internal/daemon/drivesweep_test.go`、`internal/driver/drivershutdown_test.go` 在。弃置旧 tip。 |
| `worktree-agent-ac922202c9d3b5b0a@ef1fe5ad` | `ar dictate` + `ar optimize` 及 Web 薄壳 | `da06347e` 已落，`91f16eb6` 双闸门；`internal/cli/dictate_test.go`、`optimize_test.go` 与 Web helper tests 在。弃置旧 tip。 |
| `worktree-agent-afbd234569a188d4f@94396536` | Environment 常设 worktree/branch/action rows | `b2029e7d` 已落，`8f77ad96` 补 Apply/Remove/Open；`SupervisionPanel.test.tsx` 在。弃置旧 tip。 |
| `origin/claude/continue-session-21kjss@299cdbab` | 修空 parts，另有反复增删 INC-001 文档 | `508f0e20` 已以当前 loop/provider 结构修复；`internal/agent/assembly_test.go`、`loop_error_test.go`、Gemini tests 在。分支文档提交最终净删除且已过时，弃置全部。 |
| `origin/claude/webui-tailwind-migration-bxmxe3@36e146ce` | Tailwind 迁移含两个 WIP 快照 | `68a8a69b` 已基于更新后的组件树完成迁移；当前只保留 `webui/frontend/src/tw.css`，旧 `styles*.css` 已不存在，frontend tests/build 全绿。合并 WIP/旧树会回退，弃置六提交。 |

## 唯一恢复的产品价值

`origin/codex/conduct-comprehensive-quality-assurance-testing@893bb11f` 仍有 1 个
`git cherry` 正项。旧分支的底层 opening attachment 已被 main 的 DESIGN §17 契约替代，
但当前 Home composer 仍先 `new` 纯文本、再 `send` 附件：

- 同一用户意图被拆为两个 turn；
- attachment-only 会在空 opening message 处先失败；
- reload/journal 会永久保留不真实的两次输入。

`cadd08e7` 只恢复该净价值：

- `/api/sessions` 接收并校验 `images/files`，转发 `ar new --image/--file`；
- Home 一次提交 text/images/files；attachment-only 仅补中性 caption；
- 删除创建后的附件 `send`；
- 保留现有 CAS、journal、recovery、普通 follow-up 附件不变量；
- 同批修复 AppShell 提前请求 Agent catalog 的隔离回归和 terminology lint。

Gate A：`TestHandleNewSessionForwardsOpeningAttachments`、
`Composer.createLifecycle` attachment-only case、frontend 85 files / 830 tests、
production build、`go vet ./...`、`go test ./...`、`./scripts/check.sh` 全绿。

Gate B：QA-90 在 `http://127.0.0.1:8809/`、production
`cadd08e7-215130`、全局 daemon/shared store 上完成。两条真实浏览器 session 的
opening journal 均恰有一次 `input_received`，typed content/files 同 event 携带附件；
无第二次 send，console warning/error 为 0，workspace diff 为空。证据见
`qa/runs/2026-07-23-QA90-INC100-branch-reconciliation/`。

## 非目标与保留

- 不删除任何 branch/ref；本次只裁决其代码是否进入 main。
- 不覆盖原 checkout `codex/ui` 的用户未提交修改；其 typed human attention 意图已由
  main 的递归 child answer/approval 投影与后续修复语义覆盖，原修改原样保留。
- 不重启共享 daemon：QA 时已有定时 session 正在运行；仅 guarded build + Web UI
  restart，并验证该 session 仍为 `running`。
- QA 新建 session、worktree、journal、截图全部保留，不 close、不删除。
