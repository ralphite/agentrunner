# AgentRunner — 实施计划（Implementation Plan）

本文档是 `STAGES.md` 七阶段的 **step-by-step 实施计划**。已经过三视角
对抗式 review（依赖顺序 / 覆盖忠实度 / 可执行性，共 32 条发现）修订。

粒度约定：**S1–S3 细到单步**（每步一个可验证交付物），**S4–S6 细到
模块序列**，**S7 保持里程碑级**（延迟批次，届时 kickoff refinement）。

执行方式：每个 stage 用 loop 式迭代实现（实现一步 → 测试 → 小结 →
下一步）；stage 结束跑对抗式 review；进入 stage N 前做 kickoff
refinement（只许细化步骤，不许动 DESIGN.md 不变量——要动必须停下、
写清冲突、单独 review）。

## 预期返工声明（全局，计划内而非事故）

1. **S2 重写 S1 loop 的编排**到 activity + fold state 之上
   （provider/tool/workspace 接口不动——S1 的接口从第一天按 S4 的
   最终形态设计，见 1.2）。
2. **S4.3 把 loop 从顺序执行翻成并发执行**（到达序落盘 + assembly
   重排）。缓解：从 2.10 起 tool 结果一律按 call_id map 存储，
   不存有序列表——重排从来都是 assembly 的事。
3. **1.6 的 bash 墙钟超时是临时的**，2.11 durable timer 落地后迁移。

---

## 0. 技术栈与工程基座（S1 第 0 步，此后不变）

| 项 | 选择 | 说明 |
|----|------|------|
| 语言 | **Go 1.23+** | goroutine/channel 与 actor/mailbox 天然同构；单静态 binary 跨平台分发；编译期检查利于 AI 迭代 |
| SDK | `google.golang.org/genai`（S1）、`anthropic-sdk-go`（S4）、`modelcontextprotocol/go-sdk`（S5） | 三个关键依赖均有官方 Go SDK |
| 配置/序列化 | `gopkg.in/yaml.v3` + 手写校验；event 用 `encoding/json` | 坏 spec 的精确报错由 1.1 黄金测试逼出 |
| 测试 | `go test` + `go-cmp`；三层：unit / integration / crash | |
| 静态检查 | `golangci-lint`（含 forbidigo：kernel/state/pipeline 内禁用 `time.Now`/`time.Sleep`，强制走 Clock） | |
| 日志 | `log/slog`，`AGENTRUNNER_DEBUG=1` 提升级别；日志文件在 data dir，与 journal 分离，过 redaction | |
| 平台 | **Linux/macOS only**；Windows 原型阶段显式不支持（进程组/flock 全 POSIX） | |
| 内置数据 | tool 定义等数据文件用 `go:embed` 打进 binary | |

仓库布局（Go 惯例，逐阶段长出来）：

```
cmd/agentrunner/          # main
internal/
  kernel/    # S2: actor, mailbox, bus, envelope
  event/     # event/command 类型注册（单一出处）
  store/     # S2: EventStore(JSONL); S5: ArtifactStore(CAS)
  state/     # S2: fold/apply(分命名空间 sub-state)、snapshot、resume
  clock/     # S2: Clock 接口 + FakeClock
  errs/      # S2: 错误分类学
  pipeline/  # S3: 四关卡
  agent/     # spec、loop、context assembly、multi-agent
  provider/  # base + gemini(S1) + anthropic(S4) + scripted(测试)
  workspace/ # 路径边界（S1 起）
  tool/      # 内置 tool 定义（数据）+ 实现
  runtime/   # 装配、session、epilogue；S6: daemon、notifier、scheduler、driver
  cli/
testdata/fixtures/        # 录制的 provider 应答、样例 repo（版本入库）
specs/                    # 示例 agent spec
scripts/check.sh          # golangci-lint + go test 全绿 = 一步完成
```

跨阶段测试基座：
- **ScriptedProvider**（1.3a 建）：按 session 内**序列**匹配回放，
  每条 fixture 可附对请求关键字段的断言（tool 名集合、末条消息含 X），
  漂移即响亮失败；fixture 由录制工具生成（见 1.3a），刻意的 prompt
  变更 ⇒ 重录，小单测可手写 YAML fixture。
- **样例 repo fixture**：小 Go 工程（含可跑的失败测试），版本入库于
  `testdata/`，**每个测试复制到 tmp workspace 再操作**，绝不弄脏库内副本。
- **真实仓库 testbed**（中等规模、可复现）：`scripts/testbed.sh` 把钉死
  的外部仓库 clone 到 scratch 目录——默认
  `gin-gonic/gin@v1.10.1`（`b5af7796…`，约 2 万行，MIT，测试齐全）；
  更大任务用 `caddyserver/caddy`（约 6 万行，Apache-2.0）作第二档。
  **可复现的任务构造**：clone 钉死 commit → 应用 `testdata/testbed/`
  里的已知 bug patch → agent 修复 → 跑该仓库测试。testbed 场景不进
  单测 CI，只挂 acceptance（`requires: [testbed]`），从 S1 出口检查点
  起用于真实环境验收，S4/S6 起用于 dogfood。
- **本地凭据**：repo 根 `.env`（已 gitignore、0600）存
  `GEMINI_API_KEY` 等；CLI 与测试启动时若存在 `.env` 则加载。
  **绝不提交、绝不进 fixture/journal**（redaction 兜底）。

---

## 0.5 Loop-mode 执行协议（执行 agent 的操作契约）

本计划由 coding agent 在 session 内以 loop 方式逐步执行。契约如下：

1. **启动顺序**：读 PLAN.md §0 + 当前 stage 的步骤表 → STAGES.md 对应
   段 → 步骤引用到的 DESIGN.md 章节 → `PROGRESS.md` 定位当前步。
2. **进度台账**：repo 根维护 `PROGRESS.md`，每步一节：状态、
   **所做的每个决定**（凡计划未指定而自行选择的，必须记录为 decision）、
   留给 stage review 的 open questions。随该步一起提交。
3. **一步一 commit，commit 即 push**：格式 `S<stage>.<step>:
   <交付物摘要>`，body 列 decisions/deviations；`scripts/check.sh`
   全绿才 commit（1.0 自举例外：该步创建 check.sh）。**每次 commit
   后立即 push 到 `origin/main`**——只用 main，不开分支，session
   开始先 fetch + fast-forward（项目硬规则，见 CLAUDE.md）。
4. **不可验证项**：验证列需要环境缺失的资源（如 `GEMINI_API_KEY`、
   人工验收）→ 照常实现 + 单测，验证标 `DEFERRED: <原因>` 记入
   PROGRESS.md，继续前进——**绝不停等人类**；stage 出口的人工检查点
   集中补验全部 DEFERRED 项。
5. **阻塞分流**：*欠规格*（计划没说）→ 选合理默认、记录、继续，
   review 裁决；*违反 DESIGN.md 不变量* → 停下该步，按不变量变更流程
   写清冲突，**终止 loop 并以冲突为最终报告**。
6. **前向依赖**：步 N 依赖 M>N 的产物 → 用最小 stub + `TODO(<M>)`
   标记，M 的出口清单加"替换 stub"；相邻步可对调，记录于 PROGRESS.md；
   跨 stage 顺序不得自行变更。

## 0.6 Acceptance tests 与验收 UI

**原则：每个 stage 的完成标志逐句映射为可执行的 acceptance 场景**——
完成标志里的每一句话都必须有对应场景 id，否则该句视为不可验收。

- **场景即数据**：`testdata/acceptance/s<stage>/*.yaml`：
  `{id, title(人话描述，用户读得懂), requires: [live?], steps: [命令/操作],
  expect: [断言：退出码/文件内容/journal 含记录/输出匹配]}`。
- **Runner**：`agentrunner accept --stage <n>`。TTY 下为 **bubbletea
  TUI**：清单式进度（pending / spinner / PASS / FAIL + 耗时），失败项
  展开命令输出与日志路径；非 TTY 降级为纯文本逐行，**总是**写
  `acceptance-report.json`（loop-mode agent 靠它自判结果）。
- **SKIPPED 语义**：`requires: [live]`（需凭据）或 `requires: [testbed]`
  （需外网 clone testbed）的场景在条件缺失时标 SKIPPED（区别于 FAIL）；
  stage 出口要求 FAIL=0，SKIPPED 项归入人工检查点。
- **演进**：v0 在 S1（步 1.11）落地，支持命令执行 + 退出码/文件/journal
  断言；S2 加崩溃注入场景包装，S4 加流式输出断言，逐 stage 生长。

---

## Stage 1 — 会干活的 agent（walking skeleton）

| # | 步骤 | 交付物 | 验证 |
|---|------|--------|------|
| 1.0 | 工程基座 | go module、golangci-lint（含 forbidigo 规则）、`scripts/check.sh`、slog 日志约定、`agentrunner --version` | check.sh 全绿 |
| 1.1 | 最小 spec | `AgentSpec{name, model{provider,id,max_tokens}, system_prompt|_file, tools[], max_turns?}` + loader + 校验；unknown-tool 先对硬编码 `knownTools`（`TODO(1.5)` 换注册表） | 坏 spec 黄金错误测试（缺字段/未知 tool/文件不存在，断言全文，格式见执行包） |
| 1.2 | provider 接口（按最终形态设计） | `Complete(ctx, req) → 流（iter.Seq[StreamEvent]）` + `CollectTurn()` 帮手（S1 loop 用）；归一化 `Message/Part`，**`Part` 自带 harness call_id 与 opaque `Extras/Signature` 字段**（S4 的 thoughtSignature 落位处）；`Capabilities()` 先返回空 | 类型单测；接口在 S2/S4 不再变更是本步的验收承诺 |
| 1.3 | Gemini provider | env 读 key、请求映射、流式实现、functionCall↔call_id 映射、usage 提取 | live 冒烟测试（`-tags live`；**无 key 时 t.Skip 并标 DEFERRED**，见 0.5 第 4 条） |
| 1.3a | ScriptedProvider + 录制工具 | 同接口回放实现（序列匹配 + 字段断言）；`agentrunner record-fixture` 包装 live run 生成 fixture（过凭据 redaction） | 录一个 fixture 并回放通过 |
| 1.4 | workspace 抽象 | root、`Resolve(path)`（realpath + `..` 归一 + 边界检查，越界拒） | symlink/`..` 逃逸全拒（钩子 1 生效） |
| 1.5 | tool 定义即数据 | `ToolDef{name, desc, json_schema, class}`，内置定义 `go:embed` | schema 渲染进 provider 请求 |
| 1.6 | 三个 tool 实现 | read_file（截断）、edit_file（old/new 精确替换）、bash（`Setpgid`、**临时墙钟超时**、组 kill、输出截断） | 各自单测；超时杀干净子进程 |
| 1.7 | journal v0 | append-only JSONL（记录类型见执行包）；**只记录不读回**。**先执行 1.7a 定目录再做本步**（相邻步对调，按 0.5 第 6 条记录） | 逐行可解析 |
| 1.7a | 数据目录与命名 | `$XDG_DATA_HOME/agentrunner/sessions/<id>/`；session id = `YYYYMMDD-HHMMSS-<slug>`；user 配置 `$XDG_CONFIG_HOME/agentrunner/settings.yaml`、project 配置 `.agentrunner/settings.yaml`、trust 注册表在 user data dir | 单测：路径解析与创建（0700 目录） |
| 1.8 | agent loop | turn 循环：LLM（CollectTurn）→ N 个 call 顺序执行 → 按 call_id 回填 → 继续；max_turns | ScriptedProvider 集成测试：多 turn 修文件 |
| 1.9 | CLI | `agentrunner run <spec> "task"`，turn 粒度打印 | 手动验收 |
| 1.10 | E2E | 样例 repo：agent 修一个失败测试；scripted fixture **手写**（3 turn：read→edit→bash，不依赖录制工具） | scripted 版入 CI 层；live 版标 DEFERRED 至出口检查点 |
| 1.11 | acceptance harness v0 | `agentrunner accept --stage 1`（0.6 的 runner：TUI + 纯文本 + report.json）+ S1 场景包（≥3：e2e-fix-test(scripted)、journal-readable、workspace-escape-denied） | `accept --stage 1` FAIL=0；TUI 手动查看一次 |

**S1 完成标志**：`accept --stage 1` FAIL=0（live 场景可 SKIPPED，
出口人工检查点补跑）；journal 可读。

### S1 执行包（预定默认值——执行 agent 不得再猜；偏离须记入 PROGRESS.md）

- **module**：`github.com/ralphite/agentrunner`；version：`dev`
  （`-ldflags -X main.version` 覆盖），`--version` 打印
  `agentrunner <version> (<go version>)`。
- **check.sh**：`gofmt -l` 检查 + `go vet` + `golangci-lint run` +
  `go test ./...`（不含 live tag）；`.golangci.yml` = 默认 linter 集 +
  forbidigo 规则 `time\.(Now|Sleep)` 按路径限定
  `internal/(kernel|state|pipeline)/`（目录出现即生效）。
- **provider 类型**：`StreamEvent{TextDelta | ToolCall | Usage |
  FinishReason}`；`Part{Kind: text|tool_call|tool_result, Text, CallID,
  ToolName, Args/Result json.RawMessage, Extras map[string]json.RawMessage}`；
  roles `system|user|assistant|tool`；
  `CollectTurn → (Message, []ToolCall, Usage, FinishReason, error)`。
- **call_id**：harness 生成 `call_<turn>_<index>`（确定性，回放友好）；
  Gemini 适配层按位置映射回 functionResponse。
- **spec 细则**：`system_prompt` 与 `system_prompt_file` 恰一；
  `model.id` 必填（示例与 live 测试用 `gemini-2.5-flash`）；
  `max_turns` 可选、默认 40；错误格式
  `spec <path>: field <name>: <problem>`。
- **fixture 格式**：每 session 一个 YAML：
  `steps: [{expect: {tools_include, last_message_contains}, respond: [StreamEvents]}]`；
  record-fixture CLI：`agentrunner record-fixture <spec> "task" -o <file>`（过 redaction）。
- **workspace**：root = `run` 的 cwd（1.9 加 `--workspace` 覆盖）；
  越界错误 `path escapes workspace: <requested> -> <resolved>`，读写皆拒。
- **tool 细则**：定义文件 `internal/tool/defs/*.json` + `go:embed`；
  class ∈ `read|edit|execute`；read_file 上限 2000 行 / 50KB；
  edit_file 的 `old` 必须**恰好匹配一次**（0 或 N 次报错并说明次数）；
  bash：cwd = workspace root、默认 timeout 120s、SIGTERM→5s→SIGKILL
  （pgid）、输出 head+tail 共 30KB 截断带标记。
- **journal**：`sessions/<id>/journal.jsonl`；记录类型
  `run_meta{spec_name, model, task, version}` / `assistant_message{turn,
  message}` / `tool_call{turn, call_id, name, args}` / `tool_result{turn,
  call_id, result, is_error}` / `run_end{reason, turns, usage}`。
- **路径**：`$XDG_DATA_HOME`（未设则 `~/.local/share`，macOS 同规则，
  不用 `~/Library`）；session id = `YYYYMMDD-HHMMSS-<slug>`
  （slug = task 前 30 字符，小写、非字母数字 → `-`）。
- **loop 终止**：assistant 消息零 tool call 即完成；或达 max_turns →
  journal `run_end{reason: max_turns}`。
- **CLI**：`agentrunner run <spec> "task"`（spec 位置参数）；退出码
  0 = 完成 / 1 = 运行失败 / 2 = 用法或 spec 错误；启动时若 cwd 有
  `.env` 则加载（仅本地便利，不覆盖已存在的环境变量）。

---

## Stage 2 — 一切皆事实（event-sourced 内核）

核心动作：journal 升级为 source of truth，loop 重写到 activity 之上
（重写主体在 **2.10**，2.13 起依赖它）。

| # | 步骤 | 交付物 | 验证 |
|---|------|--------|------|
| 2.1 | event/command 类型 | Envelope 字段全数落定：`id/causation_id/correlation_id/sender/target/type/payload/ts` + **传播规则**（child.causation = parent.id，correlation 继承）；`RunStarted{versions}`、`InputReceived`、`AssistantMessage`、`Activity*`、`WaitingState*`、`ActorCrashed`… | 序列化 round-trip |
| 2.2 | EventStore | 接口 + JSONL backend：per-stream seq 单调、原子 append + fsync、**文件 0600/目录 0700**、尾部半行容错、**per-session flock**（写者持锁，`run`/`resume` 撞锁即报 "held by pid N"，含 stale 检测；读者免锁） | 并发 append、截断容错、锁冲突、权限位各一测 |
| 2.3 | kernel | Actor(goroutine+channel)、bus（send/publish）、command 按 `Envelope.id` 幂等去重、`ActorCrashed`（无自动重启） | 重复 command 只处理一次；**causation/correlation 链路断言** |
| 2.4 | fold/state | `state = fold(apply, events)`，apply 纯函数；**fold state 分命名空间 sub-state**（conversation / waiting / in-flight activities / …，各带 schema 版本，版本汇入 snapshot 头）；**in-flight activity 集合作为 sub-state 即钩子 3 落位**；后续阶段新增 sub-state 在本表声明：S3 加 effects(3.2)/mode(3.6)/budget(3.7,即 reservations)、S4 加 compaction 视图、S6 加 tasks | 性质测试：fold 幂等；fold(全量)==fold(snapshot+尾部) |
| 2.5 | 调试工具 | `agentrunner events <session>` 美化打印 + `--state` fold 转储；测试帮手 `AssertFoldEqual`（结构化 diff） | 自测；后续所有 fold 断言经它 |
| 2.6 | 崩溃注入 harness 骨架 | 子进程跑 runner；**命名谓词注入**：`CRASH_AFTER=ActivityStarted:2`（第 2 条该类型 event 后 abort）+ 代码级命名注入点注册表（`after_exec_before_journal`、`between_gate_and_resolved`…，删点即测试响亮失败） | harness 自测（注入点触发与断言） |
| 2.7 | journal-inputs-first | 一切外部输入先 append 再消费 | 崩溃注入：输入落盘后 kill，resume 输入仍在 |
| 2.8 | 错误分类学 | `internal/errs`：typed 层级 + `Retryable` 标志；provider 错误映射（HTTP 状态/Gemini 码 → 分类）入 provider/base | 表驱动单测；3.9 只消费分类 |
| 2.9 | Clock 抽象 | `Clock{Now, WaitUntil}` 接口经 runtime 注入；`FakeClock.Advance()`；forbidigo 已在 1.0 拦直接调用 | FakeClock 快进单测 |
| 2.10 | activity 执行器 + loop 重写主体 | Started 先落盘 → 执行 → 终态落盘；通用 retry/backoff（**retry 可发 discard 标记**——S4 `TurnDiscarded` 的接缝）；**可选 ephemeral 进度通道**（S2 不用，S4 delta/S6 task tail 复用）；`idempotent` 标志；凭据 redaction；**tool 结果按 call_id map 存**；LLM/tool 全部改走执行器，loop 编排改为 fold state 驱动 | redaction/retry/幂等各一测；S1 集成测试在新 loop 上重跑 |
| 2.11 | durable timer | `TimerSet`/`TimerFired` event + runtime 调度（走 Clock）；activity timeout、1.6 的 bash 墙钟超时迁移至此 | FakeClock 快进 + 崩溃后 timer 仍到期 |
| 2.12 | 进程组取消 | cancel signal → 组 SIGTERM→宽限（timer）→SIGKILL → 确认组退出才落 `ActivityCancelled{partial_output}`（有界 drain） | 孤儿断言：**按 session 标记的 pgid/env marker** 查，不 grep 全局 ps |
| 2.13 | snapshot-resume | turn 边界序列化 fold state（JSON，snapshot 头含各 sub-state 版本）；resume = snapshot + fold 尾部 + 继续 loop；版本不匹配拒绝 | 等价测试；崩溃 harness 扩展 resume 断言 |
| 2.14 | 等待状态注册表 | **四个变体一次画全**：`WAITING_INPUT/APPROVAL/TASKS/TIMER` + 完整可中断性表（TASKS/TIMER 行已定义、标注"S6 前不可产生"）；interrupt journal 后带出等待 | 表驱动测试覆盖每格（暂不可产生的用合成 event） |
| 2.15 | in-doubt | resume 见 Started-无终态 → 上浮（`idempotent: true` 除外） | 崩溃注入 `after_exec_before_journal`：不重跑、上浮 |
| 2.16 | run 收尾 epilogue 骨架 | 固定序列落位：quiesce(no-op) → auto-publish(no-op) → **[barrier no-op]** → 终态 event；**此后任何加 run 结束步骤的 feature 必须挂进此序列**（钩子 2 验收点从此在这里） | 终态路径单测 |
| 2.17 | CLI 收口 + 出口矩阵 | `resume <session>`、`sessions list`；S1 E2E 场景在新内核重跑；**全崩溃注入矩阵 = S2 出口门** | 矩阵全绿 |

**S2 完成标志**：崩溃矩阵全绿——任意命名注入点 kill 后 resume 分毫
不差、in-doubt 上浮、等待状态跨进程存活（**用合成 event 验证；
"审批全流程挂起存活"是 3.5 的验证项**——STAGES.md 已同步勘误）。

### S2 执行包（kickoff refinement 预定默认值——偏离须记入 PROGRESS.md）

- **包布局**：`internal/event`（Envelope + 全部 event/command payload
  类型 + 类型注册表）、`internal/store` 升级为 EventStore（journal v0
  同包共存到 2.10，loop 切换后删除 journal 写入）、`internal/kernel`
  （actor/bus，forbidigo 生效区）、`internal/state`（fold + sub-states，
  forbidigo 生效区）、`internal/errs`（2.8）、`internal/clock`
  （Clock/FakeClock，在 forbidigo 区外——唯一合法 wall-clock 出口）、
  `internal/crash`（命名注入点注册表；生产构建为 no-op，含
  `AGENTRUNNER_CRASH` 时 `os.Exit(137)`）。
- **Envelope 线上形态**（events.jsonl 每行一个）：
  `{seq, id, causation_id, correlation_id, sender, target, type,
  payload, ts}`；`seq` 由 EventStore 追加时赋值（per-session 单调，
  从 1 起）；event id = `evt-<seq>`（追加后确定）；command id 由发送方
  生成 `cmd-<8hex>`（crypto/rand——command 属外部输入，先 journal 再
  消费，不破坏回放确定性）。传播：`child.causation_id = parent.id`、
  `correlation_id` 全链继承（根 = run 的 correlation，即 session id）。
- **文件布局**：`sessions/<id>/events.jsonl`（0600）、
  `sessions/<id>/lock`（flock + 写入 `pid\n`，撞锁读 pid 报
  `session held by pid N`；持锁进程不存在（`kill -0` 失败）即 stale，
  抢占并覆写）、`sessions/<id>/snapshots/<upto_seq>.json`（0600，
  snapshot 不是 event，不进流）。读者（`events`/`accept` 检查）免锁。
- **event 类型清单（S2 全集，S3+ 只加不改）**：
  `RunStarted{spec_name, model, task, version, sub_state_versions}` /
  `InputReceived{text, source}` / `TurnStarted{turn}` /
  `AssistantMessage{turn, message}` /
  `ActivityStarted{activity_id, kind: llm|tool, name, args, call_id?,
  idempotent, attempt}` /
  `ActivityCompleted{activity_id, result, usage?, is_error}` /
  `ActivityFailed{activity_id, error{class, message, retryable}, attempt}` /
  `ActivityCancelled{activity_id, partial_output}` /
  `TimerSet{timer_id, fire_at, purpose}` / `TimerFired{timer_id}` /
  `WaitingEntered{kind: input|approval|tasks|timer, detail}` /
  `WaitingResolved{kind, resolution}` / `ActorCrashed{actor, error}` /
  `RunEnded{reason, turns, usage}`。payload 一律独立 struct + 注册表
  `map[type]func() any`（round-trip 测试逐类型驱动）。
- **activity id 确定性**：LLM = `llm-t<turn>`；tool = `tool-<call_id>`
  （call_id 已确定性）；重试不换 id，靠 `attempt` 区分。
- **fold state 形态**：`state.State{Conversation, Activities, Waiting,
  Run}` 四个命名空间 sub-state，各实现 `Version() int`（全部从 1 起）；
  snapshot 头 = `{upto_seq, sub_state_versions map[string]int}`；
  `Activities` = in-flight 集合（钩子 3）：`map[activity_id]StartedInfo`，
  终态 event 移除；`Run` = 状态机 `running|waiting|ended` + turn 计数 +
  usage 累计。apply 纯函数：`func Apply(s State, e event.Envelope)
  (State, error)`——未知 event type 是 error（拒绝静默丢失实）。
- **崩溃注入两轨**：(a) 计数谓词 `AGENTRUNNER_CRASH=after:<EventType>:<n>`
  ——EventStore append 成功后检查；(b) 命名点
  `AGENTRUNNER_CRASH=point:<name>`——代码内 `crash.Point("<name>")`。
  S2 注册点：`after_journal_input`、`after_exec_before_journal`、
  `after_snapshot_write`、`before_run_end`（S3 加
  `between_gate_and_resolved`）。harness 断言注册表含全部期望名——
  删点即测试响亮失败。
- **Clock**：`clock.Clock{Now() time.Time; WaitUntil(ctx, t) error}`；
  `FakeClock.Advance(d)` 唤醒到期 waiter；runtime 注入，kernel/state
  只经接口。durable timer：`TimerSet` 落盘 → 调度器 `WaitUntil` →
  `TimerFired` 落盘；resume 时扫 fold 里未 fired 的 timer 重新调度
  （过期即刻 fire）。
- **错误分类学**：`errs.Class` ∈ `provider_rate_limit | provider_server |
  provider_auth | provider_invalid | tool_failed | timeout | canceled |
  internal`；`Retryable()`：rate_limit/server/timeout = true，其余 false；
  Gemini 映射：429→rate_limit、5xx→server、401/403→auth、400→invalid。
- **retry/backoff**：仅 `Retryable` 类重试；上限 3 次尝试，backoff
  1s/4s（经 Clock，可 FakeClock 快进）；每次尝试都是新
  `ActivityStarted{attempt: n}`。
- **CLI**：`agentrunner events <session-id> [--state] [--json]`（美化
  打印默认；`--state` 输出 fold 终态 JSON）；`agentrunner resume
  <session-id>`；`agentrunner sessions list`（按 mtime 倒序表格：id、
  状态（读 fold）、turns）。session-id 参数接受唯一前缀。
- **顺序微调预授权**：2.5（调试工具）可提前到 2.4 之后立即做（fold
  断言全靠它）；2.6 与 2.7 可合并为一个 commit（harness 骨架的自测
  就是 journal-inputs-first 的第一个用例）。其余顺序不变。

---

## Stage 3 — 治理副作用（effect pipeline）

| # | 步骤 | 交付物 | 验证 |
|---|------|--------|------|
| 3.1 | 管线框架 | effect 描述、四关卡接口、`EffectResolved`（判定终结后——放行或拦下都落盘——执行前；ask 路径在应答后）；post-hook 结果挂 `ActivityCompleted` | 每条路径落盘时点单测 |
| 3.2 | in-doubt 扩展 | 进关卡无 `EffectResolved` → in-doubt | 注入点 `between_gate_and_resolved` |
| 3.3 | permission rules | tool/path（经 workspace.Resolve）/bash command 模式；allow/ask/deny；序 user > project > spec | 表驱动；`src/../../etc` 拒绝 |
| 3.4 | 配置分层 + 信任 | spec + user + project 三源合并（**注：从 S5 提前，S5 只剩 skills/memory 合并**）；project 层 hooks 默认忽略，`agentrunner trust <dir>` 显式信任（注册表在 1.7a 的位置） | 不受信 repo 的 hook 不执行 |
| 3.5 | 审批流 | ask → `ApprovalRequested`（**预留 `payload_ref`**）→ `WAITING_APPROVAL` → 应答 journal → 继续/拒绝渲染；**denied-by-interrupt**：等待中 interrupt → 审批按拒绝解决、call 渲染 `[interrupted by user]`、loop 继续 | 崩溃注入：挂起中 kill → resume → 批准继续（S2 出口欠的验证在此补齐）；FakeClock 挂两天；interrupt 路径测试 |
| 3.6a | mode 数据模型 | mode = 工具面过滤（按 ToolDef.class）+ 数据描述；`default/plan/acceptEdits/bypass` | 过滤表驱动测试 |
| 3.6b | mode prompt 注入 | S3 注入点：system prompt 尾部追加段（**S4.4a 会把它收编进 assembly 拼装序**，声明为计划内迁移） | 注入内容断言 |
| 3.6c | 跃迁 | `ExitPlanMode` tool + 审批通过 → `ModeChanged` event + 跃迁规则表 | plan→default 集成测试 |
| 3.6d | 优先级 | bypass 不跳 hooks 的精确语义 | 组合测试 |
| 3.7a | 预算 sub-state | reservation 集合入 fold state（2.4 声明的 S3 sub-state） | fold 等价测试更新 |
| 3.7b | 预留与结算 | LLM 按 max_tokens 预留、tool 按类别估值；`ActivityCompleted` 实结 | 单测 |
| 3.7c | 优雅收尾 | 资源超限 → 收尾消息 + `LimitExceeded`，**挂进 2.16 epilogue 序列**；结构限制 → error 结果 | 终态路径测试 |
| 3.7d | TOCTOU（合成） | **gate 级合成并发测试**（真实并行 S4.3 才存在，届时复验——见 S4.3） | N 合成并发不超支 |
| 3.8 | hooks v0 | pre/post 执行器（observe + block by exit code） | 恢复路径不重跑 hook（崩溃注入） |
| 3.9 | 错误渲染表 | error 分类（2.8）→ 模型可见渲染的统一函数（**归一化形态；per-provider 线上形态的映射在 S4.7**） | 每行一测；loop 继续性断言 |
| 3.10 | CLI 审批 UI | 终端交互批准/拒绝（附理由） | 手动 + scripted |

**S3 完成标志**：plan mode 全流程；审批挂两天（FakeClock）后批准原地
继续；合成 TOCTOU 不超支；不受信 hooks 不执行。

### S3 执行包（kickoff refinement 预定默认值——偏离须记入 PROGRESS.md）

- **包布局**：`internal/pipeline`（effect 描述 + 四关卡 + 管线,
  forbidigo 生效区）、`internal/config`（三源合并 + trust 注册表）、
  `internal/hook`（3.8 执行器）。mode/budget 的 fold sub-state 进
  `internal/state`（2.4 已声明:S3 加 `reservations` + `mode`,
  SubStateVersions 加两键）。
- **新 event 类型（加性,S2 的 15 个不动）**:
  `effect_resolved{effect_id, activity_id, verdict: allow|deny,
  gate_results: [{gate, decision, reason}]}`（判定终结后、执行前落盘;
  deny 也落盘再渲染）/
  `approval_requested{approval_id, call_id, effect_id, gate_results,
  payload_ref?}`（payload_ref 预留字符串,S2 无 ArtifactStore 留空）/
  `approval_responded{approval_id, decision: approve|deny,
  reason?, source}`（外部输入,journal-inputs-first;WaitingResolved
  仍负责解除 WAITING_APPROVAL）/
  `mode_changed{from, to, cause}` /
  `limit_exceeded{kind: tokens, limit, used}`。
- **effect 描述**:`Effect{Kind: tool_call|llm_call, ToolName, Class,
  Args, CallID, EstTokens}`;effect_id = `eff-<call_id>` / `eff-llm-t<n>`。
- **关卡语义**:序 = pre-hooks → permission → budget;每关
  `Decision{Allow|Ask|Deny, Reason}`;**deny 短路**（后续关不跑）,
  ask 聚合（任一 ask 且无 deny → 审批,携带全部 gate_results——
  review 已并入的决定）;全 allow → `effect_resolved{allow}` → 执行。
  bypass mode 跳过 permission/budget 的 ask/deny 但 **hooks 照跑**
  （3.6d 语义）。
- **permission rules YAML**（settings.yaml 的 `permissions:` 列表,
  首条匹配即生效——顺序即优先级;三源拼接序 user > project > spec）:
  `- {tool: bash, command: "go test *", action: allow}`（command 用
  path.Match 风格 glob,对整条命令）/ `- {tool: edit_file,
  path: "src/**", action: ask}`（path 相对 workspace root,经
  workspace.Resolve 归一后匹配,`**` 支持）。**无规则命中时按 mode
  默认**:default mode = read:allow, edit:ask, execute:ask;
  acceptEdits = edit 升 allow;plan = edit/execute 一律 deny(工具面
  已过滤,双保险);bypass = 全 allow。
- **mode**:spec 可设 `mode:`,CLI `--mode` 覆盖;跃迁规则表:
  plan→default(经 ExitPlanMode 审批)、default↔acceptEdits(用户
  命令)、任意→bypass 仅 CLI 启动时可设。`exit_plan_mode` 是 S3 新
  内置 tool(class wait)。
- **trust**:`$XDG_DATA_HOME/agentrunner/trusted.yaml`(列 realpath
  目录);`agentrunner trust <dir>` 写入;project 层 settings 的
  hooks 段仅在 workspace root 受信时生效(permissions 段可用——
  只收紧不放宽:不受信 project 的 allow 降级为 ask)。
- **budget**:spec `budget: {max_total_tokens: N}`(可选,缺省无限);
  LLM 活动预留 `max_tokens`、tool 按类估值(read 500 / edit 1000 /
  execute 2000 tokens 记账口径,S3 粗价目表);`ActivityCompleted`
  实结(usage 抵预留);超限 → 3.7c 优雅收尾(epilogue quiesce 前
  加 farewell slot——2.16 序列的既定挂点)。
- **hooks v0**:settings `hooks: {pre_tool: [cmd], post_tool: [cmd]}`;
  pre 以 JSON(stdin: effect)调用,exit 0 = observe/allow、exit 2 =
  block(渲染 stderr 给模型),其他 exit = hook 错误(observe + 警告);
  post 收 result JSON,输出挂 ActivityCompleted 的 `hook_note` 字段
  (加性)。超时 10s(经 Clock 不可行——hook 是外部进程,用真实
  timer,记为 forbidigo 豁免点,hook 包不在禁区)。
- **审批 CLI**:TTY 下交互 `[y]es/[n]o + reason`;非 TTY(loop-mode)
  读 `AGENTRUNNER_APPROVE=always|never`(acceptance 用),缺省 never。
- **恢复语义**:挂起中 kill → resume 重现 WAITING_APPROVAL(fold 已
  支持)→ CLI 重新提示;审批经过关卡但 crash 于 `effect_resolved`
  前(`between_gate_and_resolved` 注入点)→ in-doubt 报告该 effect。
- **S3 kickoff 回访项**(出口 review 递延):tool 活动可返回 activity
  错误后,ActivityFailed-重试窗口的非幂等重跑保护(见 S2 review 记录)。

---

## Stage 4 — 交互与上下文（模块序列）

1. **协议 v1 + streaming**：输出事件流类型定稿；CLI 流式渲染；delta 走
   2.10 的 ephemeral 进度通道；**`TurnDiscarded` 接线**——LLM activity
   在已流出 delta 后 retry → 发 discard 标记 → 前端重开流（scripted
   partial-stream-retry 测试）；**用户可见错误 surface**（与模型可见
   渲染分离）。
2. **steering/interrupt**：输入 journal 后 turn 边界消费；Esc → 协作
   取消;2.14 可中断性表的 interrupt 列在此端到端复验。
3. **并行 tool call**（预期返工 #2 落地）：并发执行 allow 的 call
   （ask 不阻塞）；到达序落盘；assembly 按 call_id 原序重排回填；
   **3.7d 的 TOCTOU 在真实并行下复验**。
4a. **assembly 组件 + 拼装序**：fold → 请求的独立模块；固定拼装顺序；
   env 块 session start 冻结；3.6b 的 mode 注入收编至此。
4b. **tool 输出截断**（per-tool 上限 + 告知模型被截断）。
4c. **prefix 稳定 + caching**：byte-stability 回归测试；cache 断点；
   **cache_read/write 归一化入 usage event，budget 结算按真实计费口径**。
4d. **signature 往返**：`Part.Extras` 持久化进 event、assembly 原样
   回传（Gemini thoughtSignature 多轮测试）。
5. **compaction**：`ContextCompacted` recorded activity 改变 fold 视图
   （2.4 sub-state）；**fold 等价性质测试跨 compaction 边界**。
6. **finish reason 策略**：归一化枚举；malformed_tool_call → retry
   （复用 discard 路径）；safety/blocked 上浮；空 candidate 注试。
7. **Anthropic provider + capabilities 矩阵**：第二实现；**`thinking`
   进 spec model 块并按 provider 映射/显式降级**；**per-provider error
   线上形态**（Anthropic `is_error` / Gemini functionResponse error
   载荷）；同一 scripted 矩阵跑双 provider。
8. **session UX + inspect v0**：`sessions list/show`、resume 流式续接；
   `agentrunner inspect <session>`：turns、每个 call 的 `EffectResolved`
   判定、token/cost/cache 列（2.5 的 events 命令进化版）。

**S4 完成标志**：单 agent 体验接近 Claude Code；**inspect v0** 可见
缓存命中；Esc 500ms 内杀掉任意 tool call；双 provider 矩阵全绿。

### S4 执行包（kickoff refinement 预定默认值——偏离须记入 PROGRESS.md）

- **包布局**:`internal/protocol`(输出事件流类型 + 编码)、
  `internal/agent/assembly.go`(4a 独立 assembly,把现 `assembleMessages`
  抽出)、provider 下加 `anthropic/`(4.7)。streaming 渲染在 cli。
- **加性 event 类型(S2/S3 不改)**:
  `TurnDiscarded{turn, reason}`(LLM 已流出 delta 后 retry;fold 丢弃
  该 turn 的半成品 assistant 累积——但 assistant_message 只在成功后
  落盘,故 fold 层无半成品,TurnDiscarded 主要驱动**前端重开流**信号,
  记为 ephemeral-伴生 durable 标记) /
  `ContextCompacted{upto_turn, summary_ref, dropped_turns}`(5;summary_ref
  预留 ArtifactStore,S4 先内联 summary 字段) /
  `MalformedToolCall{turn, raw, error}`(6;驱动 retry,复用 discard 路径)。
- **输出协议(protocol 包)**:`StreamEvent` 输出侧(区别于 provider 输入
  侧的同名类型——protocol 是**面向 surface** 的):`{kind, turn, ...}`,
  kind ∈ `text_delta | tool_call | tool_result | turn_start | turn_end |
  approval_request | mode_changed | run_end | error | discard`。JSON 行
  编码(`--json` 流)+ 人读渲染两套 sink。**用户可见错误**(protocol
  error kind)与**模型可见错误**(errs.RenderForModel,3.9)分离:前者进
  stream 给人看,后者进 fold 给模型看。
- **ephemeral 进度通道兑现**:2.10 预留的 `Activity.Progress` 在 S4.1
  接线——LLM activity 的 text delta 经 Progress 回调 → protocol
  text_delta,**不落 journal**(delta 是 ephemeral,成功后的 assembled
  message 才落 assistant_message,TurnDiscarded 契约)。
- **steering/interrupt(4.2)**:复用 3.5 的 `Loop.Interrupts` 通道 +
  2.14 可中断性表。turn 边界消费:interrupt 在 turn 中到达 → journal
  `InputReceived{source: interrupt}` → 当前 activity 取消(2.12 路径)→
  turn 边界把 interrupt 文本作为新 user input 注入。Esc = 首个 interrupt
  (协作取消,run 继续);第二 Esc/SIGTERM = 硬取消(3.10 signalContext
  已实现,S4 扩展 interrupt 语义)。**500ms 杀 tool call** = 现 killGroup
  路径(SIGTERM→5s 宽限太慢,S4 把交互取消的宽限缩到 500ms,与 timeout
  取消区分:交互取消急、超时取消可宽限)。
- **并行 tool call(4.3,预期返工 #2)**:同一 assistant turn 的多个 allow
  的 tool call **并发执行**(经 `parallel`-式 goroutine + errgroup 风格
  收集);ask 的 call 不阻塞其他(但 ask 本身串行审批,避免多弹窗竞争
  ——S4 决定:一个 turn 内多个 ask 顺序审批)。到达序落盘(ActivityCompleted
  按完成先后),assembly 按 call_id **原序**重排回填(fold 的 ToolResults
  是 map,assembly 按 assistant message 里 tool_call 的顺序读)。**3.7d
  TOCTOU 真实并行复验**:BudgetGate 的 reserve-then-settle 在真并发下不
  超支(现合成测试升级为真 goroutine)。**appendE 串行化**:并行 activity
  的 journal 写必须过单一 appendE(mutex 或 channel 序列化)——fold 是
  单线程折叠,这是 S4.3 的核心不变量。
- **assembly(4a)**:`assembly.Build(state.State, mode, tools) → 
  CompleteRequest`,固定拼装序:system prompt → mode 注入(3.6b 收编)→
  env 块(session start 冻结,byte-stable)→ conversation。golden 测试
  从 loop_golden 迁至 assembly 包。
- **caching(4c)**:usage event 已有 cache_read/write 字段(S1 provider
  类型),S4 确保 budget 结算按 `input + output - cache_read` 真实计费
  口径(现 budget 用 input+output,4c 修正);prefix byte-stability 回归
  (env 块冻结是关键)。
- **finish reason(6)**:`provider.FinishReason` 归一枚举已在 S1
  (end_turn/tool_use/max_tokens/other);S4 加 `malformed_tool_call`
  (provider 解析 tool call 失败)→ MalformedToolCall event + retry;
  safety/blocked → FinishOther 已有,S4 上浮为用户可见 error。
- **Anthropic provider(4.7)**:`internal/provider/anthropic`,`anthropic-sdk-go`;
  capabilities 矩阵(thinking/cache/parallel_tools 各 provider 声明);
  thinking 进 `spec.model.thinking`(bool/budget)按 provider 映射;
  per-provider error 线上形态映射到 errs.Class(3.9 归一化的线上侧,
  S4.7 补齐)。同一 scripted 矩阵双 provider 跑。
- **inspect(4.8)**:`agentrunner inspect <session>`——events 命令进化:
  turns 表 + 每 call 的 EffectResolved 判定 + token/cost/cache 列(从
  fold 的 usage + effect_resolved 读)。
- **顺序微调预授权**:4a(assembly 抽取)可提前到 4.1 之前做(streaming
  依赖 assembly 已是独立模块更干净);4c(caching)依赖 4a 的 env 块冻结,
  顺序不变。
- **回访项**:S3 记档的 AGENTRUNNER_APPROVE footgun 在 4.2 interrupt
  落地后复审(交互式审批 UI 已在 3.10,loop-mode 的 auto-approve 语义
  是否收紧);record recorder 按单 delta redact 的跨分片泄漏(S2 review
  递延)在 4.1 流式协议重构 recorder 时合并修。

---

## Stage 5 — 生态与多 agent（模块序列）

1. **MCP client**：官方 Go SDK、生命周期带外、schema 入 event、
   `mcp__<server>__<tool>` 命名、无标签按 execute-class、
   **spec `allowed_tools` 收窄（含否定测试）**。
2. **skills + memory 文件**：目录发现、frontmatter、按需加载；CLAUDE.md
   层级合并入 assembly；**skill 目录注入 assembly 层（prefix 稳定）**。
3. **spawn/await**：子 agent 作为 activity；**子 agent 目录注入 system
   prompt**（不注入模型不知道能 spawn 谁）；权限 rules spawn 冻结交集；
   树预算 min 聚合 + 深度/扇出上限；审批沿 correlation 冒泡。
4. **handoff + pub/sub**：移交语义；blackboard topic。
5. **ArtifactStore**：CAS、`publish_artifact`、per-stream 版本、manifest。
6. **outputs contract**：2.16 epilogue 的 auto-publish 槽位**填实**；
   缺 required → parent error 结果。
7. **审批载荷**：3.5 预留的 `payload_ref` 启用；plan 审批全流程
   （发布→审→拒→v2→批）。
8. **artifact 输入**：spawn/CLI 传 ref、materialize activity。
9. **inspect 扩展**：子 agent 树（correlation/causation 渲染）。

**S5 完成标志**：researcher 编队产出带 contract 检查的报告；plan 审批
全流程；越权/预算击穿否定测试全绿。

---

## Stage 6 — 服务化与运行模式（模块序列）

1. **daemon**：本地 socket server 托管 runtime；CLI attach/detach
   （journal 补读 + 订阅）；`runtime.daemon: never` 降级。
2. **notifier**：生命周期 topic、`NotificationSent` 去重 stream、
   启动对账；通道 = user 层配置（文档化 carve-out）。
3. **background effects**：`background: true`、handle = ActivityStarted
   渲染、完成 = user-role 输入、`WAITING_TASKS` 行激活（2.14 表已定义）、
   `task_output/kill`、`on_run_end`；epilogue quiesce 槽位填实；
   task tail 复用 2.10 进度通道。
4. **scheduler**：cron/interval（走 Clock）→ 幂等 `RunAgent`；webhook。
5. **IterationDriver**：driver actor + 统一事件族；goal（三种 verifier +
   停滞检测）；loop（`schedule_next`/`finish_series`、overlap）；
   carry 走 ArtifactStore、series memory 注入时截断。
6. **HTTP/WS 壳**：同一协议远程暴露；headless 收口。

**阶段内可延后项（cut line）**：best-of-N（`parallel{n}`）与 HTTP/WS
壳可移出 S6 收口、随后补——不影响完成标志。

**S6 完成标志**：series 无人 attach 跑过夜出通知；goal 三轮迭代到
verifier 通过；CLI 重开 attach 回同一 run。

---

## Stage 7 — 世界状态生命周期（里程碑级）

按 `STAGES.md`。进入前 kickoff refinement + 单独 review，此处不预写。

---

## 横切纪律

- **一步 = 一个可合并提交单元**（代码+测试+文档行），`scripts/check.sh`
  全绿才算完（提交与台账契约见 0.5）。
- **stage 收口**：acceptance 场景（0.6）FAIL=0（SKIPPED 项人工检查点
  补验）→ 三视角对抗 review → 修复 → 下一段 kickoff refinement。
  每个后续 stage 在收口前把自己的完成标志场景化并入
  `testdata/acceptance/s<n>/`。
- **四个钩子验收点**：钩子 1 = 1.4（workspace 强制）；钩子 2 = **2.16**
  （epilogue 骨架，5.6/6.3 填实）；钩子 3 = **2.4 + 2.12**（in-flight
  集合入 fold + 进程组终态）；钩子 4 = 2.1–2.7（event 纪律）。每次
  stage review 显式检查未被绕过。
- **规模预期**（Go 口径，校准信号而非承诺）：S1 ≈ 1.5–2k 行 + 测试；
  S2 ≈ 2.5–3k；S3 ≈ 2.5–3k；S4 ≈ 3.5–4k；S5 ≈ 3.5–4k；S6 ≈ 4.5–5k
  （含 cut line 项）。单 stage 超预估 50% = 计划信号，回头审视切分。
- **不变量变更流程**：实现中发现设计不变量站不住 → 停下该步 → 写清
  冲突（现象、涉及不变量、备选）→ 单独 review 后改 DESIGN.md →
  再继续。禁止代码里先绕。
