# INC-94 Sidebar 按 last update 排序

> 状态：✅ 已完成（2026-07-22，QA-85 PASS）。

## 动机与 journey 锚

修订 **UJ-24 Web UI 驾驶 AgentRunner** 的左栏 recency：CLI `/sessions` 已按每个
session 的 `events.jsonl` mtime 分页返回，但 `buildSidebarModel` 又按 session id（创建
时间）重排，导致旧 session 收到新活动后不会浮到所属 project 顶部，project 也不会
浮到 Projects 顶部。

目标：session、workspace-less Sessions、Pinned sessions 均按 durable journal last
update 降序；project 以其最新 session 的 last update 降序。Project overlay pin 继续
作为显式用户优先分区，分区内部仍按 last update。

**UI/UX design note**：复用现有 recency-first navigator，不新增控件、文案或设置。
后端以 RFC3339 `updated_at` 显式公开已经用于分页的 journal mtime；frontend 映射为
`updatedAt`。旧 backend / legacy row 缺字段时仅回退 session id 创建时间，保证兼容。
排序只改变 projection，不修改、迁移或删除 session/project 数据；没有破坏性状态或
未决产品问题。

## Spec delta

- `JOURNEYS.md`：UJ-24 step 1 明确 project 与 session 均按 last update newest-first。
- `SPEC.md`：Web UI 产品面与 journal-backed metadata 条目登记 `updated_at` / recency
  projection；锚 CLI/webui/frontend tests 与 QA-85。
- `DESIGN.md`：§12 明确 journal mtime 是 sidebar recency 单一来源；project recency =
  `max(member.updatedAt)`，overlay pin 只做稳定分区。
- **不变量**：不触及。journal 仍是真相源；字段 additive，旧行兼容，不改 durability、
  routing、permission 或存储 schema。

## 验收

### A 闸

- CLI：`sessions --json` 输出 `updated_at`，顺序与该值一致。
- WebUI：snake_case 映射为 `updatedAt`，不泄漏双 key。
- Frontend：创建时间更旧但 update 更新的 session 上浮；其 project 上浮；project 内
  sessions、workspace-less 与 Pinned 都 newest-first；相同/缺失时间以 id 稳定回退。
- targeted tests + frontend full vitest/build + `./scripts/check.sh`。

### B 闸：QA-85，共享真实环境

在 production `http://127.0.0.1:8809/`、共享 `~/.local/share/agentrunner/`：

1. API rows 具有 `updatedAt` 且全局降序；
2. 对可见 Projects 与每组 sessions 读取 DOM 顺序，对照 API workspace 的最大/成员
   update；Pinned project/session 只按已有 pin precedence 分区；
3. 打开既有 session 并 reload，顺序、deep link 与 thread 恢复；
4. console relevant warning/error=0，截图留在
   `qa/runs/2026-07-22-QA85-sidebar-last-update-order/`；不制造新 session 或触碰 journal
   mtime 只为测试。

## 实施步骤

1. **INC-94.1**：CLI/WebUI/frontend contract + regression tests + 三层/QA/LOG 收口；
   真实浏览器 QA；deploy、commit 并 push `origin/main`。

## review 裁决

裁掉三视角对抗 review：additive timestamp projection 与 pure sort，无并发写、权限或
schema migration；保留 API contract tests、分页/legacy fallback tests、真实共享数据
QA 与总闸门。
