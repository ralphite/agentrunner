# INC-55 用户自定义 command tools（HANDA-PARITY #4，触信任模型 决策 #19）

> **编号冲突提示**：`docs/increments/INC-55-last-turn-diff.md`（Codex Changes
> 谱系，已合并、LOG.md/DESIGN.md 已引用）与本项（HANDA-PARITY #4，SPRINT
> 队列 🔧 认领号 INC-55）**跨 sprint 撞号**（INC-54 同样撞号：cron 跨重启 vs
> session-mode-pill）。系并发认领所致。本纸按 sprint 认领沿用 INC-55；合并进
> `origin/main` 时应择一改号（建议本项改 INC-57），请 reviewer 定夺。

**状态**：实现 + A 闸孪生已绿（2026-07-11）。判定 = **additive 兑现决策
#19**（详见 §四变更单），DESIGN 决策 #19 与 §9 信任段、§10 生态接入段的
additive 编辑与实现**同 commit**。B 闸（真 Gemini）待集中验（§验收）。

## 动机与 journey 锚

**UJ-19 生态接入**。现状：外接工具只有 MCP（out-of-process server）与
自定义 slash 命令（ingest 展开成 prompt 文本，对模型不可见）。缺一条
handa 已实现的能力（`tool_store.py` / `agent-command-tools.md`）：把**本地
命令**用一份 manifest 包成**模型可直接调用的工具**，参数以 JSON 从 stdin
传入。典型场景：项目里一段 `./deploy.sh`、一个 `kubectl` 包装、一个内部
CLI——用户想让 agent 像用内置 `bash` 一样按名调用它，但带上结构化参数面
（JSON schema）和固定命令（模型不能改命令行，只能填参数）。

对标 HANDA-PARITY §2 #4。规模 M。

## Spec delta

SPEC **C · 工具面** 增一行（MCP/自定义命令同族）：

- **自定义 command tools（INC-55）**：user 层 `~/.config/agentrunner/tools/*.json`
  + project 层 `<ws>/.claude/tools/*.json`；manifest =
  `{name, description, command, timeout_s, params(JSON schema)}`。发现在
  session 开始、**冻结进 `SessionStarted.command_tools`**（resume 从 fold
  重建、不重读文件系统，决策 #3）；**project 层 = 可执行配置，未 trust
  不加载**（决策 #19，同 hooks 门）；与内置撞名**拒载**、user 层压 project
  层（撞名优先级）、`mcp__` 前缀名拒载。每次调用 = **execute-class command
  effect**（`eff.Command` = manifest 固定命令，过 FloorGate→hooks→permission
  →budget 全管线，execute 默认 ask）+ **决策 #34 OS sandbox**（isolated
  HOME/TMP、凭据路径拒读、network ratchet，backend 缺失 fail closed、
  EffectResolved 载 containment），args JSON 从 stdin 传入。

**信任模型行**（SPEC C，原"project 层 hooks 需显式 trust"）扩为"project 层
hooks **与 command tools** 需显式 trust"。

## Design delta（触不变量 决策 #19，走 §四）

见下"§四变更单"。**决策 #34（OS sandbox）非新增枚举**——command tool 是
"bash/command verifier 之外的第三个 sandboxed subprocess"，落在决策 #34
既有的"execute-class 强制 OS sandbox"面内（本纸只把 `containment`/
`containmentError` 的 `bash` 判据泛化为"bash ∪ command tool"，是兑现不是
扩枚举）。

## §四变更单（决策 #19 信任模型）

**旧不变量**（DESIGN §15 决策 #19，与 §9 粗体信任段）：
> | 19 | 信任模型 | 可执行配置（hooks）只认 spec 与 user 层；project 层需
> 显式 trust；memory 文件按不可信内容对待 | clone 不受信 repo 不等于交出
> 任意代码执行权。 |

§9 正文：
> "**信任模型**：spec 与 settings 等同于'你选择执行的代码'。可执行配置
> （hooks）只从 spec 与 user 层生效；**project 层（随 repo 走的文件）里的
> hooks 被忽略**，除非用户对该 workspace 做过一次显式 trust 确认……memory
> 文件按不可信内容对待（只进 prompt，不获得任何执行权）。"

**判定：additive 兑现（非不变量翻转）**。依据：

1. **决策 #19 是范畴不变量，非 hooks 专属规则。** 其 reasoning 是范畴级
   （"clone 不受信 repo 不等于交出任意代码执行权"），"（hooks）"是**当时
   唯一的实例**的举例，不是封闭枚举。既有 DESIGN 已按范畴复用它：
   - **决策 #36**（动态角色）："信任面由结构封死（决策 #19/#20 **同族**）"
     ——把 #19 当**原则族**用到 inline role，不是 hooks-only。
   - **决策 #38**（approve --always）："project allow 未 trust 时降级为 ask
     （决策 #19）"——把 #19 用到 permission rules（非 hooks），证明 #19 已
     治理多个 project 层面。
   - §9 正文以原则起句（"spec 与 settings 等同于你选择执行的代码"），hooks
     是其例示。
2. **command tool 落在 #19 自己划的执行/文本分界的执行侧。** 决策 #19 同条
   已明确 memory=不可信内容（只进 prompt、零执行权）。command tool 运行
   `command` 且吃模型控制的 stdin——是最直接的"可执行配置"（甚至比 hooks
   更直接：hooks 是管线机件 observe+block，command tool 是模型侧一等执行面）。
   从不受信 clone 加载它，正是 #19 要禁的"交出任意代码执行权"。故 trust-gate
   它 = **应用**该不变量，不是改它。
3. **零新放行路径。** 实现复用既有 trust 门（`config.IsTrusted`，与 hooks
   同一 `trusted.yaml`）与既有 effect 管线（execute-class command effect +
   决策 #34 sandbox）。未 trust 的 project command tool 就是不进 fold、不
   advertise、不可 dispatch——与未 trust 的 project hook 被忽略同构。加载后
   的每次调用走 bash 同款管线与沙箱。信任不变量与 effect/sandbox 不变量都
   不被削弱。

**新表述**（决策 #19 additive 编辑，同 commit）：
> | 19 | 信任模型 | **可执行配置（hooks、command tools）** 只认 spec 与
> user 层；project 层需显式 trust；memory 文件按不可信内容对待 | clone 不
> 受信 repo 不等于交出任意代码执行权。 |

§9 正文对应把"可执行配置（hooks）"改为"可执行配置（hooks、command
tools——见 §10）"，其余不动（hooks/memory 的既有保证措辞一字不改）。

**四性/边界复核**：
- **崩溃安全**：发现在 session 开始一次性冻结进 `SessionStarted.command_tools`
  （journal-first，与 skills/MCP face 同）；resume 从 fold 重建、不重读
  manifest、不重查 trust——trust 判定被 journal 定格，rewind 不复活。
- **凭据红线**：manifest 只落命令字符串（用户自置配置，非凭据）；执行走
  `sandboxedBash` → 凭据路径 `credentialPaths` deny、secret env 剥离
  （复用 bash 沙箱，零新面）。
- **并发**：`l.commandTools` 是 per-Loop registry（非跨子共享），dispatch
  查表无锁；executor 保持无状态（`RunCommandTool` 只接命令+stdin）。
- **边界**：撞内置名拒载（内置赢）、`mcp__` 前缀拒载（与 MCP 面无撞名
  可能）、user 压 project、同层重名首个（文件名序）胜；名限
  `^[A-Za-z0-9_-]{1,64}$`（provider 函数名形，杜绝穿越/命名空间戏法）。
  malformed manifest 跳过并告警（不阻断 run，与 skills 同）。

**结论**：additive 兑现 → 完整实现 + 同 commit DESIGN 编辑；本纸 §四变更单
为佐证成文。回报里显式列出对决策 #19 的解读供 reviewer 复核；如 reviewer
认为"（hooks）"应作封闭枚举、扩为"（hooks、command tools）"属枚举翻转须
独立契约 review，则本 DESIGN 编辑回退为待 review、实现的 additive 发现/加载/
撞名/沙箱机制保留（不含不变量枚举翻转的落地）。

## 波及面

- 新包 `internal/commandtool`（发现/解析/trust 门/撞名/优先级，纯逻辑）。
- `internal/runtime/paths.go`：`UserToolsDir`/`ProjectToolsDir`。
- `internal/event/types.go`：`CommandToolDef` + `SessionStarted.CommandTools`。
- `internal/state/state.go`：`Session.CommandTools` + SessionStarted fold。
- `internal/pipeline`：`Effect.Command` 字段 + permission gate 用 `eff.Command`
  兜底 `args.command`（bash 行为不变，回归测试守）。
- `internal/tool/exec.go`：`RunCommandTool` + 抽出 `runSandboxed` 共享 bash
  运行机件（bash 语义不变）。
- `internal/agent/loop.go`：发现+冻结（SessionStarted 点，自动覆盖 run/
  daemon/children 三路）；drive 从 fold 建 face+registry；effect 带 Command；
  dispatch；`toolClassIn`/`toolTimeoutIn`/`commandToolIn`/`containment`/
  `containmentError` 认 command tool。
- 文档：SPEC C 加行、DESIGN 决策 #19+§9+§10、GAPS、LOG。

## 验收

**A 闸孪生（全绿）**：
- **manifest 解析**：`commandtool.TestParseAndResolve`（strict 解码拒未知
  键、缺 name/command 拒、`mcp__` 前缀拒、非对象 params 拒、负 timeout 拒）、
  `TestResolveDefaultsAndClamp`（默认对象 schema、timeout 上限钳）。
- **user/project 发现**：`commandtool.TestDiscoverUserLayer`（user 恒载、
  malformed 跳过告警）、`TestDiscoverMissingDirs`。
- **project 层未 trust 不加载**：`commandtool.TestDiscoverProjectTrustGate`
  （untrusted→0+告警 / trusted→载且 source=project）；
  `agent.TestCommandToolProjectTrustGate`（端到端：`config.Trust` 前后
  `SessionStarted.command_tools` 的差）。
- **撞名拒载（内置）与优先级（user/project）**：
  `commandtool.TestDiscoverBuiltinCollisionRejected`、
  `TestDiscoverUserBeatsProject`。
- **调用构造正确的 command effect（过管线、args JSON 走 stdin）**：
  `pipeline.TestCommandToolEffectAdjudication`（execute 默认 ask / tool-name
  allow / command glob allow 命中固定命令 / 固定命令 deny 压过模型 args /
  compound 分段 / plan 模式 floor deny）+ `TestBashStillUsesArgsCommand`
  （回归）；`tool.TestRunCommandToolStdin/EmptyArgs/ExitCode`（`cat` 回显
  stdin 证 args 到达、退出码传播）；`agent.TestCommandToolEndToEnd`（真沙箱
  端到端：advertise→调用→固定命令 effect allow→containment→stdin 到达）。
- **sandbox 生效**：`tool.TestRunCommandToolFailsClosedWithoutSandbox`
  （无 backend 拒跑）；`agent.TestCommandToolEndToEnd` 断言 EffectResolved.
  Containment（filesystem=workspace、backend 非空）；`agent.
  TestCommandToolFoldHelpers`（class=execute、manifest timeout）。

**B 闸（真 Gemini，待集中验）**——见 §QA 说明。

## 实施步骤（本轮一次成型，单 commit）

1. runtime paths + `commandtool` 包 + 单测。
2. event/state（CommandToolDef + fold）+ pipeline（Effect.Command）+ tool
   （RunCommandTool/runSandboxed）+ 各自单测。
3. loop 接线（发现/冻结/face/dispatch/class/timeout/containment）+ agent 孪生。
4. 文档 delta（本纸 + SPEC/DESIGN/GAPS/LOG）。
5. A 闸 `./scripts/check.sh` 全绿。

## review 裁决

- **契约视角**：本纸 §四变更单即契约自审（决策 #19 additive 兑现论证 +
  四性/边界复核）。**须 reviewer 复核对决策 #19 的解读**（additive 兑现
  vs 枚举翻转）——回报已显式提请。
- **安全视角**：信任门（未 trust project 不加载）、撞名拒载、沙箱 fail
  closed、凭据剥离均复用既有面并有孪生守；无新放行路径。裁为随本纸自审，
  不另起独立 review（M 规模、复用既有安全面）。
- **正确性/并发**：per-Loop registry 无锁、executor 无状态、fold 冻结
  resume 一致——孪生覆盖。裁为随本纸自审。

## QA 说明（B 闸，供集中验）

真 Gemini，共享 daemon/store（不隔离），测完保留会话：

1. **自定义 command tool 真调用**：user 层置
   `~/.config/agentrunner/tools/wordcount.json`
   （`{"name":"wordcount","description":"count words in the given text",
   "command":"wc -w","params":{"type":"object","properties":{"text":
   {"type":"string"}}}}`——注：`wc -w` 读 stdin，模型传的 args JSON 即
   stdin）。起会话让模型"用 wordcount 数一段文字的词数"；红线：模型 face
   见到 `wordcount`、调用产生 execute-class EffectResolved（含 containment）、
   `ar events` 见工具结果、`ar inspect` 见调用。
2. **未 trust 的 project 工具不加载**：在一个**未 trust** 的 workspace 放
   `<ws>/.claude/tools/x.json`，起会话；红线：stderr/日志见 untrusted 告警、
   `SessionStarted.command_tools` 不含 `x`、模型 face 无 `x`。`ar trust <ws>`
   后重起：`x` 出现。
3. **撞名拒载**：置 `~/.config/agentrunner/tools/bash.json`（name=`bash`）；
   红线：告警"collides with a built-in tool"、内置 `bash` 仍在、自定义未覆盖。

断言只钉 runtime 红线（face 成员/EffectResolved/containment/trust 门），
不钉模型措辞。结果归档 `qa/runs/<日期>-INC55/`。
