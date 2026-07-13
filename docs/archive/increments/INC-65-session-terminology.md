# INC-65 统一 session 产品模型并移除 task 概念

## 动机与 journey 锚

DESIGN 决策 #31 与 GAPS G27 已裁定：产品只有一种可持续对话、恢复、归档与
继续的 durable `session`，不存在另一种 `task` 形态。后续 INC-41 为追求
Codex UI parity，把参考产品的 `project/task` 信息架构原样写入 UJ-24、SPEC、
Web UI view model 和可见文案，重新制造了第二个产品实体：同一个 session 在
runtime 叫 session，在 sidebar、命令面板、归档页和 header 又叫 task。

本增量修订 UJ-24：用户创建、浏览、搜索、归档、恢复、fork 和继续的对象统一
为 `session`。一次执行叫 `run`；多 Agent 分工叫 `delegation`；后台执行单元叫
`background work`；交给模型或 run 的文本叫 `prompt`。不再用 `task` 同时指代
上述四种不同对象。

术语选择与主流用法：OpenAI Agents SDK 将跨多 turn 保存 conversation history、
可暂停恢复并可由多个 Agent 共享的持久对象称为 `session`；这与 AgentRunner 的
journal/fold 实体及既有 CLI `sessions` 完全同构。因此采用 `session`，不另造
`thread` 或 `conversation task`。参考：
<https://openai.github.io/openai-agents-python/sessions/>。

## Spec delta

- Web UI 信息架构由 `Projects -> task` 改为 `Projects -> sessions`；无 workspace
  的 session 在独立 `Sessions` 区平铺，不再进入 `tasks` collection。
- `New task`、task header/menu、task search、Archived tasks、Scheduled tasks
  等用户可见入口分别改为 `New session`、session、Archived sessions、
  Scheduled runs。
- command palette 与快捷键数据源统一为 `quickSwitchSessions`。
- session 内 `ps` 投影使用 `BackgroundWork`，多 Agent 协调投影使用
  `Delegation`，开场输入与 driver 输入使用 `prompt`。
- 旧 journal 与 driver YAML 一次性迁移到 canonical 字段；snapshot 是可重建
  cache，迁移时删除并由 journal 重建。daemon command 与 Web UI request 直接
  切换 schema，不保留双字段、fallback 或长期 legacy decoder。
- 新增产品术语 lint：活文档和 Web UI 产品面禁止再次引入 task 实体词汇；明确的
  legacy decoder 与 archive 不在禁区。

## Design delta

- DESIGN §12 Web UI projection 明确唯一实体为 session，并登记术语映射。
- DESIGN event/schema 兼容原则补充：按决策 #18 的破坏性升级路径执行一次性
  journal migration；迁移前留备份与校验，迁移后 runtime 只认识 canonical key。
- 不修改任何既定不变量。本增量是在实现层恢复决策 #31，而非改变它。

## UI/UX review

- **沿用 pattern**：保留现有 sidebar、project grouping、command palette、header
  menu、archive 和 Scheduled 布局与交互，只替换实体命名和对应 view model。
- **proposed UI**：`New session`；project 下和无 workspace 区均展示 session；
  palette 分组为 `Sessions` / `Needs attention`；归档为 `Archived sessions`；定时
  自动执行为 `Scheduled runs`；高级 composer 菜单简称 `Advanced`。
- **risky states**：旧 session 与旧 driver spec 经迁移后必须在 daemon 重启后
  继续可见、可打开、可继续；旧 client protocol 不承诺兼容。
- **data handling**：结构化改写 shared store journal/spec，迁移前备份；删除可
  重建 snapshot cache。用户消息、session id、workspace、archive/pin/rename
  本地状态原样保留。
- **unresolved questions**：无。`session` 已是 runtime、CLI、API 路由和状态机的
  主实体名，选择其他词反而制造第二次迁移。

## 验收

- scripted：event / snapshot / daemon / driver schema 测试，证明 canonical 数据只
  写 `opening_prompt` / `prompt` / `delegation_id`；迁移脚本 fixture 证明旧 journal
  被完整改写且用户消息内容不变。
- frontend unit：sidebar、palette、session header、archive、Scheduled、not-found、
  supervision 全部改为 canonical 术语；workspace-less session 仍可达。
- mechanical：`scripts/lint-product-terms.sh` 进入 `check.sh`，禁止活产品面再次出现
  `New task`、`Tasks` section、task header/menu、`TeamTask`、`TaskID` 等实体模型。
- real environment：共享 `~/.local/share/agentrunner/`，现有 session 列表、深链、
  project session、workspace-less session、归档页、Scheduled、创建新 session、
  daemon restart 后续聊均通过；截图归档到 `qa/runs/2026-07-12-INC65/`。
- 全量 `./scripts/check.sh` 通过。

## 实施步骤

1. 文档裁决：工作纸明确三层 delta、兼容边界和验收。
2. 产品面：修改 JOURNEYS/SPEC/DESIGN/QA/GAPS、Web UI/README、frontend model、
   文案、CSS selector 与测试；加入术语 lint。
3. runtime：将开场输入、driver 输入、daemon command、delegation、background
   work 的代码模型迁移到 canonical 术语；共享数据一次性改写，不保留 read alias。
4. 双闸门：全量自动化检查 + 共享真实环境 browser QA + daemon restart。
5. 收口：delta 并回活文档、LOG 追加决策，工作纸移入 archive/increments。

## 实施结果

- runtime / daemon / driver / Web API 直接切换到 `opening_prompt`、`prompt`、
  `delegation_id` 与 `delegations`；未保留旧字段 decoder、双写或 fallback。
- 共享 store 迁移前完成压缩备份与 SHA-256 校验；一次性改写 762 份 journal、
  41,945 行 event、1,171 个结构 key 与 2 份 driver spec，删除 590 份 event
  index 和 3,374 份可重建 snapshot。迁移后全部 JSONL 可解析，旧 schema key、
  index 与 snapshot 均为 0。
- 新 runtime 直接 fold 迁移前普通 session、driver session 与含 2 个 delegation
  的多 Agent session；daemon/Web UI 重启后健康检查 `daemonUp`、`versionMatch`
  均为 true。
- 共享真实 Web UI 验证 sidebar、command palette、Scheduled runs、一次性 run
  表单、Archived sessions、普通历史、多 Agent 历史和 driver 历史；禁止的旧产品
  label 计数为 0，console warning/error 为 0。截图与迁移审计证据保留在
  `qa/runs/2026-07-12-INC65/`。
- 收尾语义审计额外修复两处机械迁移错误：空 `ar ps` 输出被误投影为一条记录，
  以及 background tool timeline 状态误写为 `session`；均增加回归测试。
- `./scripts/check.sh` 全绿：全部 Go package、WebUI Go tests、520 个 frontend
  tests、production build 与 5 个 installer scenarios 通过。最终共享部署重启后
  再次通过 health、历史 session deep-link 与 browser console 检查。

## review 裁决

本增量触及跨层 schema compatibility、真实共享 store 和主要导航，执行正确性、
兼容性、契约三视角 review；P0/P1 全部修复后收口。UI 不新增布局或交互，视觉
专项 review 裁掉，理由是本次只统一既有对象名称与内部模型。
