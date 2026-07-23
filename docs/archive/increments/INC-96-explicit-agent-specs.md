# INC-96 共享 Agent catalog 与独立模型选择

> 状态：✅ 2026-07-23 完成。共享 Agent catalog、独立 model input、
> CLI/Web UI 接线、旧 session 兼容、A/B 双闸均已验证。

## 动机与 journey 锚

UJ-01/UJ-11/UJ-24 都会选择 Agent，但当前产品有两套来源：

1. runtime 只内置 `explore` / `plan`，位于
   `internal/agent/builtin/*.yaml`；
2. Web UI 的 Dev / Team Lead / Auditor / Reviewer / Chat / Worker 位于
   `webui/frontend/src/agents/*.yaml`，CLI 与未来 TUI 无法复用。

此外，`model` 当前嵌在 Agent YAML 中。模型是一次 session 的执行选择，不是
Agent 的行为定义；同一个 Agent 应能由 Web UI、CLI 或未来 TUI 配不同模型运行。

本增量把 Agent 定义收敛为 runtime 共享 catalog，并把模型解析改成独立的
session input：

- shipped Agent：`internal/agent/builtin/*.yaml`，随 binary embed；
- user Agent：`~/.config/agentrunner/agents/*.yaml`，可新增或按同名覆盖
  shipped Agent；
- CLI 接受 Agent name 或显式 YAML path；
- Web UI 从 backend catalog 读取 Agent，不再 import/复制 Agent YAML；
- Agent YAML 不接受 `model` 字段；
- 自动 compaction 阈值是 Agent 的上下文行为，保留为顶层
  `compact_at_tokens` / `microcompact_at_tokens`；它们不属于 model selection；
- Web UI 每次创建/切换都显式提交 `model + effort`；
- CLI 可提交 `--model <provider>/<id> --effort <level>`；省略时读取
  `~/.config/agentrunner/settings.yaml` 的 `default_model`，仍缺失则使用
  compiled default `gemini/gemini-flash-latest + medium`。

### UI/UX design note

- **沿用模式**：保留 composer 现有 Agent picker、Model picker、Effort
  submenu 与 `Edit agent YAML…`，不新增 Settings 页面。
- **变化**：Agent picker 的数据来自 `/api/agents`；高级 YAML editor 只显示
  Agent 行为定义，不再显示/修改 model；模型始终由独立 pill 提交。
- **裁掉**：exact thinking-budget override。`max_tokens` 与 `thinking` 统一由
  `effort = light | medium | high | xhigh` 推导，避免三处表达同一选择。
- **风险态**：未知 Agent、非法 user override、非法 model/effort 均显式失败；
  不 silent fallback 到 Dev 或 shipped 同名定义。user override 是用户主动配置，
  因此可覆盖 `explore/plan`，仍经 strict loader 校验。
- **数据处理**：既有 journal 已冻结的 effective spec/model 照常 resume。
  旧磁盘 Agent YAML 的 `model` 对新 launch 给出迁移错误；既有 session 的
  legacy sibling 在 resume 时只为兼容读取并继承 session 已冻结模型，绝不重新采用
  YAML 中的 model。
- **未决问题**：无。catalog layer、CLI 形态与 default model 均经用户确认。

## Spec delta

- UJ-01：用户可以 `ar new dev "..."`，也可继续传显式 YAML path；CLI model
  input 缺省走 user/global default。
- UJ-11：session 内换 Agent 使用 catalog name 或 YAML path；未显式换 model
  时保持该 session 当前 model。
- UJ-24：Web UI Agent picker 与 YAML editor 消费 runtime catalog；model/effort
  是独立且必传的 session input。
- SPEC：
  - 新增 shared Agent catalog / user override / name-or-path resolution；
  - Agent definition schema 删除 `model`；
  - 新增 model input precedence 与 effort mapping；
  - Web UI progressive composer 补 catalog projection 契约。

## Design delta

- DESIGN §9：拆开 `AgentDefinition` 与 resolved runtime `AgentSpec`：
  - YAML 只解码行为定义；
  - session start 将 definition + model selection 解析成 effective spec；
  - `SessionStarted` / `SpecChanged` 继续冻结完整 effective spec 和 model。
- DESIGN §12：Web UI 是 catalog projection，不拥有 Agent 定义。
- DESIGN §15 决策 #13 仍是“YAML → 强类型 struct + strict 校验”，不改变。

**不变量裁决**：不触及既有 durability/journal-first/permission/provider
不变量。改变的是 session start 前的配置解析；解析完成后仍冻结为原有
`AgentSpec`，resume 不读取 live default model，不会被用户配置漂移改写。

## 验收

### A 闸

1. catalog：
   - shipped 八项 `dev/lead/auditor/reviewer/chat/worker/explore/plan` 可列出；
   - user 同名覆盖、user 新增、显式 path、unknown 与坏 YAML 均逐项测试；
   - child resolution 继承 parent effective model。
2. model：
   - `light/medium/high/xhigh` 的 thinking/max token 映射逐项测试；
   - precedence = explicit CLI/API > user `default_model` > compiled default；
   - Agent YAML 的 `model` 新 launch fail-fast，错误给出迁移方式；
   - 已冻结 legacy session resume 不受 live settings 漂移影响。
3. CLI：
   - `new/run/submit/drive/agent` 的 name/path 与 model/default 路径；
   - `agents --json` 是 backend/UI 共用 catalog contract。
4. Web UI：
   - `/api/agents` 映射 CLI catalog；
   - new/switch/run 请求显式携带 model/effort；
   - frontend 不再包含 Agent YAML；picker/editor round-trip 当前 definition。
5. targeted/full Go + frontend tests、production build、`./scripts/check.sh` 全绿。

### B 闸

共享 `~/.local/share/agentrunner/` 与 production `http://127.0.0.1:8809/`：

1. CLI `agents --json` 与 Web UI picker 显示同一 effective catalog；
2. 用 Web UI 选择 Agent + model 开新 session，journal 分别冻结正确 definition/model；
3. session 内只换 Agent，model 保持；只换 model，Agent prompt/tools 保持；
4. 用 CLI name 且省略 `--model`，命中 user/global default；显式 model 覆盖；
5. restart 后既有旧 session 与新 session 都可 reload/续聊；
6. browser URL/content/console/interaction/reload 均通过；测试 session、
   workspace、journal 与 `ar events`/workspace diff 全部保留并归档。

## 完成实证

- shipped catalog 已收敛为 runtime embed 的八项；CLI `ar agents --json` 与
  production `/api/agents` 返回同序、同 source、零 Agent `model` 字段。
- 真 Gemini shared-store session：
  - CLI 缺省选择冻结 `gemini/gemini-flash-latest + medium`
    （thinking 6144 / max 10240）；
  - CLI 显式 `light` 冻结 thinking 2048 / max 6144；
  - Web UI 真浏览器选择 Chat + Light，捕获到实际 POST 的独立
    `{provider,model,effort}`，Agent YAML 无 `model`。
- 同一 session 只换 Agent 时保留 Light effective model；随后只换 effort
  为 High 时 Agent 仍是 reviewer，effective model 更新为 thinking 12288 /
  max 16384。
- production 部署执行 guarded daemon/Web UI restart，既有 legacy session
  仍可由 CLI 与 Web API 读取；随后 Web UI 单独 restart 与新 session
  deep-link reload 均恢复。第二次 daemon restart 因共享环境存在两个
  running session 被安全门拒绝，未 force、未中断用户工作。
- 真浏览器 1100×700 的 catalog、model/effort、create、reply、reload 全链路
  PASS，warning/error 为空；证据保留于
  `qa/runs/2026-07-23-INC96-agent-catalog/`。
- A 闸：Go targeted/full tests、Web UI Go tests、frontend 75 files /
  776 tests、production build、`./scripts/check.sh` 全绿。

## 实施步骤

1. **INC-96.0**：重裁工作纸，明确共享 catalog、schema/model precedence、
   legacy/restart 与 UI 契约；commit/push。
2. **INC-96.1**：runtime catalog、user override、Agent YAML schema、model
   selection/default、CLI name/path/flags 与 scripted tests；check；commit/push。
3. **INC-96.2**：Web API/catalog + frontend picker/editor/model input，删除
   frontend Agent YAML 与 duplicated spec construction；全量 A 闸；commit/push。
4. **INC-96.3**：共享真实环境 B 闸，三层/QA/GAPS/LOG 收口，工作纸归档；
   check；commit/push。

## review 裁决

该增量跨 runtime config、CLI、Web API、frontend、legacy resume 与 restart，
规模达到架构契约级；实施后做 correctness/config-boundary/compatibility 三视角
review。P0/P1 全部闭合后才进入 B 闸。
