# PROGRESS — 实施决策台账（PLAN §0.5 规定）

每步一节：状态、所做决定（凡计划未指定而自行选择的）、留给 stage
review 的 open questions。

## S1.0 — 工程基座　✅

- **状态**：完成。go module + CLI 骨架（`--version`）+ slog 约定 +
  `.golangci.yml` + `scripts/check.sh`，check 全绿。
- **决定**：
  - Go 1.24（环境预装 1.24.7，满足 PLAN 的 1.23+）。
  - golangci-lint 环境预装 **v2.5.0** → 配置用 **v2 格式**
    （`version: "2"`，非 PLAN 隐含的 v1 格式）；forbidigo 用
    `exclusions.rules[].path-except` 实现"只在
    internal/(kernel|state|pipeline) 生效"。
  - CLI 骨架放 `internal/cli`（`Run(args, version, stdout, stderr) int`
    可测试），main 只做转发；`run` 子命令留给 1.9。
  - 日志走 stderr（stdout 留给 run 输出），`AGENTRUNNER_DEBUG=1` 提级。
  - `.gitignore` 清理了 Python 时代残留（`__pycache__`），加编译产物。
  - errcheck 排除 `fmt.Fprint/Fprintf/Fprintln`（CLI 输出返回值检查是
    噪音；项目级惯例）。
- **DEFERRED**：无。
- **Open questions**：无。

## S1.1 — 最小 spec loader　✅

- **状态**：完成。`internal/agent/spec.go` + 8 个黄金错误用例 +
  3 个正例（默认值、prompt file 解析）；`specs/hello.yaml` 示例。
- **决定**：
  - 校验返回**第一个**错误（字段定义序），不聚合——黄金测试断言全文，
    单错误最稳定。
  - `model.max_tokens` 可选，默认 8192（执行包未定，取 DESIGN 示例值）。
  - `system_prompt_file` 相对路径**相对 spec 文件所在目录**解析；
    加载后内容并入 `SystemPrompt` 并清空 file 字段（下游只看一个字段）。
  - 未知 YAML 字段用 `KnownFields(true)` 严格拒绝，错误保留 yaml 库
    原文（含行号），一样进黄金测试。
  - 空 `tools` 合法（纯对话 agent）。
  - 黄金测试带 `-update` 再生成机制。
- **DEFERRED**：无。
- **Open questions**：`model.provider` 目前只查非空，不查已知 provider
  名单——provider 注册表 1.3 才出现，届时是否收紧留给 stage review。

## S1.2 — provider 接口（最终形态）　✅

- **状态**：完成。`internal/provider`：归一化类型全套 + `Provider`
  接口（流式）+ `CollectTurn` + `CallID` 帮手，4 组单测含 Extras
  round-trip。
- **决定**：
  - 流用 **`iter.Seq2[StreamEvent, error]`**（Go 1.23 迭代器；错误随
    流内联，终止即停）——执行包写的 `iter.Seq`，带错误通道的 Seq2 更
    准确，记为偏离。
  - `Part.IsError` 作为归一化的 tool_result 错误标志（线上形态由各
    provider 映射，S4 落地）。
  - `FinishReason` 枚举 S1 先放 4 个常规值（end_turn/tool_use/
    max_tokens/other），异常形态 S4 扩展——类型从第一天存在，event
    形状不变。
  - `provider.ToolDef` 是 wire 级最小定义（name/desc/schema）；1.5 的
    数据化注册表持有富定义并向下转换，避免 import 环。
  - `CollectTurn` 把 text delta 合并为单个 text part，tool_call parts
    按到达序附加（Extras 原样保留）。
- **DEFERRED**：无。
- **验收承诺**：本接口在 S2/S4 不再变更（1.2 的验证列）。

## S1.3 — Gemini provider　✅

- **状态**：完成。`internal/provider/gemini`：官方 genai SDK 适配、
  流式映射、functionCall↔call_id、thoughtSignature 进出 Extras、
  usage/finish 归一化。5 组纯函数单测 + **live 冒烟已实跑通过**
  （无需 DEFERRED——本环境 `.env` 有 key 且网络可达）。
- **决定**：
  - tool schema 用 SDK 的 **`ParametersJsonSchema` 直通**而非执行包
    预设的手写 Schema converter——SDK 原生支持 raw JSON schema，
    直通严格更优（偏离已记）。
  - **默认模型改为 `gemini-flash-latest`**：执行包写的
    `gemini-2.5-flash` 在本 key 上 404（该 key 的模型清单无裸 2.5-flash，
    有 latest 别名/2.5-flash-lite/3-preview 系）。示例与测试全部
    改用 latest 别名。
  - `CompleteRequest` 增加 `Turn` 字段（加性变更，call id 生成需要；
    不违反 1.2 的稳定承诺）。
  - Gemini 无 error 标志 → 错误结果约定为
    `functionResponse.response = {"error": …}`；对象结果直通，
    标量包 `{"output": …}`（决策 #9 的 Gemini 侧落地）。
  - live 测试自带 `.env` 加载（不覆盖已有 env），`//go:build live`
    隔离,check.sh 不编译。
- **DEFERRED**：无。

## S1.3a — ScriptedProvider + 录制器　✅

- **状态**：完成。`internal/provider/scripted`（序列匹配 + expect 断言 +
  Done() 消费校验）与 `internal/provider/record`（Provider 中间件式
  录制器：自动派生 expect、凭据 redaction、WriteFixture）。6 组测试
  含录制→回放 round-trip 与 drift 检测。
- **决定**：
  - **录制器做成 Provider 中间件**而非 CLI 子命令——`record-fixture`
    CLI 需要 agent loop（1.9 才有），中间件现在就可单测；CLI 接线
    推迟到 1.9（记入其出口清单）。
  - 录制时自动派生 expect：tools 全名单 + 末条消息首个 text part 的
    前 60 字符；redaction 覆盖 `*_API_KEY/_TOKEN/_SECRET` 的环境值。
  - scripted 的 tool_call 事件 `call_id` 可省略——默认按
    `CallID(req.Turn, index)` 铸造，手写 fixture 更省事。
  - `Expect.LastMessageContains` 对 tool_result part 也匹配其 Result
    原文（下一轮请求的"末条消息"往往是 tool 结果）。
- **DEFERRED**：无。

## S1.4 — workspace 抽象　✅（钩子 1 落位）

- **状态**：完成。`internal/workspace`：realpath + `..` 归一 + 边界
  检查；**不存在的路径解析最深已存在祖先**（新文件写在 out-of-tree
  symlink 目录后面同样拒绝）。6 个测试覆盖 `..`/绝对路径/symlink
  已存在与新文件目标/root 自身。
- **决定**：root 在 New 时即做 abs + EvalSymlinks（边界比较在完全
  解析的空间里进行）；错误格式按执行包
  `path escapes workspace: <requested> -> <resolved>`。
- **DEFERRED**：无。

## S1.5 — tool 定义即数据　✅

- **状态**：完成。`internal/tool`：三个内置定义（`defs/*.json` +
  go:embed）、类别标签（含预留的 wait）、注册表（启动时校验：完整性/
  重名 panic）、`ProviderDefs` 向 wire 级转换。1.1 的 knownTools stub
  已按出口清单换成注册表（`TODO(1.5)` 关闭，unknown_tool 黄金重生成）。
- **决定**：
  - `Names()` 排序输出（embed FS 按文件名序，显式排序更稳）。
  - edit_file 语义在 schema 描述里锁定：`old` 恰好匹配一次；
    **空 `old` + 不存在的 path = 创建新文件**（执行包只说了替换，
    创建语义是补充决定——没有创建能力 agent 无法新增文件）。
  - registry 校验失败用 panic（embed 的定义坏 = 程序坏，不是运行时错）。
- **DEFERRED**：无。

## S1.6 — 三个 tool 实现　✅

- **状态**：完成。`internal/tool/exec.go`：Executor（绑定 workspace）+
  read_file（2000 行/50KB 截断）+ edit_file（恰好一次替换，报 0/N 次；
  空 old 创建）+ bash（Setpgid、120s 默认超时、SIGTERM→5s→SIGKILL 组杀、
  30KB 头尾截断）。9 组测试含**进程组死亡断言**（timeout 后 kill -0
  探测组已消失）与转义拒绝。
- **决定**：
  - 统一 `Result{Payload, IsError}` 返回——tool 级错误全部是模型可见的
    error 结果（决策 #9），Go error 只留给 harness 自身故障（S1 无）。
  - bash 非零退出码 → IsError（对齐 Claude Code 行为）。
  - `cmd.WaitDelay = 2s` 解决后台子进程持有管道导致 Wait 悬挂的经典
    问题（`bash -c 'x &'` 场景）。
  - bash 超时是墙钟（PLAN 已声明 provisional，S2.11 迁移 durable timer）。
- **DEFERRED**：无。

## S1.7a — 数据目录与命名　✅

- **状态**：完成。`internal/runtime/paths.go`：XDG data dir（macOS 同
  规则）、session 目录 0700、session id（`YYYYMMDD-HHMMSS-<slug>`，
  slug 30 字节小写规整）、user/project 配置路径。
- **决定**：slug 截断按字节（超长任务名截 30 字节，UTF-8 断字符的
  残片被过滤规则自然丢弃）；空 slug 兜底为 `task`。按台账既定顺序
  先于 1.7 执行。

## S1.7 — journal v0　✅

- **状态**：完成。`internal/store/journal.go`：append-only JSONL、
  0600、五种记录类型（run_meta / assistant_message / tool_call /
  tool_result / run_end）、只写不读。测试验证逐行可解析、类型序、
  权限位。
- **决定**：行形状用 `{"type","ts","data":{…}}` **嵌套 data**（执行包
  未定平铺 vs 嵌套；嵌套无歧义且前向可解析）。journal v0 用
  `time.Now`——store 包 S2 才进 Clock 纪律，v0 本来就会被 EventStore
  替换。
- **DEFERRED**：无。

## S1.8 — agent loop　✅

- **状态**：完成。`internal/agent/loop.go`：turn 循环组装 provider +
  tool executor + journal + Sink（turn 粒度输出接口）。4 组集成测试
  用 ScriptedProvider：多 turn 改文件、纯文本停止、tool 错误后续跑、
  max_turns。
- **决定**：
  - loop 终止：assistant 消息零 tool call = 完成；否则并行 call 顺序
    执行、结果合成一条 `RoleTool` 消息回填（S1 顺序执行，S4.3 才并发）。
  - `Sink` 接口把渲染与循环解耦（CLI 1.9 实现；测试传 nil）。
  - usage 逐 turn 累加进 RunResult。
  - **明确标注**：本 orchestration 是 S1 naive 版，S2.10 会重写到
    activity + fold state 之上（接口不变）——预期返工 #1 的落点。
- **DEFERRED**：无。

## S1.9 — CLI run 命令　✅

- **状态**：完成。`run` / `record-fixture` 子命令、`--workspace` /
  `--max-turns` / `-o` 旗标、`.env` 加载（不覆盖已有 env）、textSink
  turn 粒度渲染、session 创建 + journal 接线。5 组测试（scripted 端到端、
  退出码、dotenv 语义）+ **live 手动验收通过**（真 Gemini 3 turn 修文件）。
- **决定**：
  - `record-fixture` 与 `run` 共用一条执行路径（recordMode 包装
    provider），1.3a 遗留的 CLI 接线在此关闭。
  - 人机信息（session id、run 摘要、fixture 路径）走 **stderr**，
    stdout 只留 agent 输出——脚本可管道消费。
  - max_turns 停止按正常完成处理（exit 0）。
  - provider 工厂可注入（测试用 scripted 工厂）；未知 provider 报
    usage 错（exit 2）。
- **DEFERRED**：无。

## S1.10 — 样例 repo E2E　✅

- **状态**：完成。`e2e/`：samplerepo（含故意失败的测试）+ 手写 4-step
  fixture（read → edit → go test → 收尾）+ 端到端测试：先断言原始 repo
  测试失败，跑完断言修复落地且 repo 自身测试转绿、fixture 全消费。
- **决定**：testdata 放 `e2e/testdata/`（Go 惯例包内 testdata；PLAN
  写的根目录 testdata/，记为偏离）；samplerepo 是独立 go module，
  testdata 目录天然被 go 工具忽略；每次测试复制到 tmp，从不弄脏库内副本。
- **DEFERRED**：live 版 E2E（真 Gemini 修 samplerepo）留 stage 出口
  人工检查点——scripted 版已入 CI 层。

## S1.11 — acceptance harness v0　✅（Stage 1 收官步）

- **状态**：完成。`internal/accept`（场景模型/runner/plain 渲染/JSON
  报告/bubbletea TUI）+ `agentrunner accept --stage N` 子命令 +
  3 个 S1 场景（e2e-fix-file / journal-readable / workspace-escape）。
  **实跑 `accept --stage 1`：3 PASS / 0 FAIL**，report.json 生成。
- **决定**：
  - 场景 **go:embed 进 binary**（自包含，任何目录可跑）；场景自带
    `files:` 段生成输入，不依赖外部 fixture 路径。
  - CLI provider 工厂加 `scripted`（`AGENTRUNNER_SCRIPTED_FIXTURE`
    env）——acceptance 经真 CLI 跑 scripted fixture 的测试接缝。
  - runner 给每个场景独立 scratch dir + 独立 `XDG_DATA_HOME`，注入
    `BIN`/`SCRATCH` env；中间步必须成功，末步 exit code 归 expect 管。
  - expect 四种：exit_code / output_contains / file_contains /
    journal_valid（内建逐行 JSON 校验）。
  - 非 TTY 自动降级 plain（本环境即此路径）；`--plain` 可强制。
- **DEFERRED**：TUI 的人工目视验收（本环境无 TTY）→ stage 出口检查点。

## Stage 1 状态：**全部 12 步完成**

出口条件：`accept --stage 1` FAIL=0 ✅（3 PASS）；journal 可读 ✅。
待办：对抗式 stage review（PLAN 收口纪律）+ 出口人工检查点
（TUI 目视 + live E2E）。

## Stage 1 出口对抗式 review　✅（三视角,35 条发现,修复批已落地)

**修复的真缺陷**(quality/fidelity 两审):TUI 中止不再假 PASS
(`StatusAborted` + `Report.Green()` 门);acceptance 场景严格解析
(未知键报错、每条 expect 恰一断言——空转断言不可能再出现);
`journal_valid` 要求首 `run_meta` 尾 `run_end`;Ctrl-C 经
`signal.NotifyContext` 传导到工具进程组;bash 的取消与超时分离渲染
(不再伪造 timeout)、done/timer 竞态偏向 done、killGroup 只认 ESRCH、
stdout/stderr 各 15KB(合计 30KB 对齐执行包);read_file/截断不再撕裂
UTF-8;edit_file 创建走 O_EXCL(消 TOCTOU);录制器 expect 片段过
redaction(修密钥泄漏)、tool_call Extras 进 fixture schema;Gemini
thinking tokens 计入 output(真实计费口径);loop 失败路径 best-effort
写 `run_end{reason:error}`;record-fixture 在 run 失败时也写 fixture;
`run_meta.Version` 接通 ldflags;session id 加 4hex 熵防同秒碰撞;
dotenv 支持引号/export;provider 构造失败 exit 1(区别于未知名 exit 2)。

**决策修订**:max_turns 强制停止改为 **exit 1**(推翻 S1.9 的 exit 0
——脚本/CI 不应把卡死的 agent 当成功)。

**记档的已知缺口**(不修,S4 处理):text part 上的 thoughtSignature
无法经 StreamEvent 携带(计划 S4 给 StreamEvent 加可选 Extras——
加性变更);CollectTurn 以 struct 返回(语义等价执行包的四元组,
正式记为偏离);journal tool_result 多一个 name 字段(保留)。

**待办队列 ✅ 已清空(用户指示立即恢复,未等定时)**:
钉住测试批全部落地——请求组装 golden(`testdata/request_assembly.golden`,
S2.10 重写的行为契约)、`accept --stage 1` 进 go test(e2e 包构建真
binary 执行,S1 完成标志可在 CI 复现)、Report.Green 门测试、scenario
严格解析测试(typo 键/空断言/双断言全拒)、journal 终态校验测试、loop
错误路径(provider 错误 → turn 包装 + 终态 run_end;journal 写失败
中止)、record-fixture CLI 往返 + 写失败 exit 1、provider 构造失败
退出码、gemini 转换错误表 + 空 parts 校验(新增:零 part 消息报错,
Gemini 会 400)+ Complete 流内错误、scripted 每次迭代消费一步的语义
钉住、workspace root-symlink/兄弟前缀、bash ctx-cancel(canceled 而非
伪 timeout + 进程组死亡断言)。新场景 **s1-04-e2e-fix-test**(经 CLI
全链路修 Go 工程失败测试,含 bash go test)入 suite——S1 acceptance
现为 4 场景。

**Stage 1 正式关闭**。下一步:S2 kickoff refinement。

---

## S2 kickoff refinement — DONE

按 §0.5 惯例在进入 Stage 2 前细化步骤(只动 PLAN.md Stage 2 段,
不触 DESIGN.md 不变量)。产出:PLAN.md 新增 **S2 执行包**,把 2.1–2.17
里所有"实现时才会遇到"的欠规格项预先钉死。关键决定:

- **包布局**:`internal/event`(类型+注册表)、store 升级 EventStore
  (journal v0 共存到 2.10)、`internal/kernel`+`internal/state`
  (forbidigo 生效区)、`internal/clock`(区外,唯一 wall-clock 出口)、
  `internal/crash`(注入点注册表)。
- **id 方案**:event id = `evt-<seq>`(append 后确定,seq per-session
  单调);command id = `cmd-<8hex>` 随机(外部输入先 journal 再消费,
  不破坏回放);activity id 确定性(`llm-t<turn>` / `tool-<call_id>`),
  重试不换 id 靠 attempt 区分。
- **文件布局**:`events.jsonl` + `lock`(flock+pid,stale 检测=
  kill -0)+ `snapshots/<upto_seq>.json`(snapshot 不进 event 流)。
- **event 全集 14 个类型**(S3+ 只加不改),payload 独立 struct +
  注册表驱动 round-trip 测试;apply 遇未知 type 报错(拒绝静默丢失实)。
- **崩溃注入两轨**:计数谓词 `after:<EventType>:<n>`(store 层检查)
  + 命名点 `point:<name>`(`crash.Point()`);S2 注册 4 个点。
- **错误分类学 8 类 + retry 政策**(仅 retryable,3 次,1s/4s 经 Clock)。
- **顺序微调预授权**:2.5 可提至 2.4 后;2.6+2.7 可合并 commit。

Open questions 留给 stage review:kernel 的 actor 粒度(单 session
单 actor 还是 per-concern 多 actor)在 2.3 实现时按最小可用决定并记档。

## S2.1 event/command 类型 — DONE

`internal/event`:Envelope(wire 形态 `{seq,id,causation_id,
correlation_id,sender,target,type,payload,ts}`)+ 14 个 payload struct
+ `Registry` 表 + `DecodePayload`(未知 type 报错)+ `ChildOf` 传播
helper + `NewCommandID`/`EventID`。

**Decisions**:
- `New()` 拒绝未注册 type——事实的词汇表封闭,加类型必须过注册表。
- round-trip 测试要求 samples 表与 Registry 等长——加 event 类型时
  漏写测试样本会直接 fail。
- `ts` 用 json `omitzero`(Go 1.24):未 append 的 envelope 不带 ts。
- `WaitingEntered.Detail` 用 `json.RawMessage`(各 kind 结构不同,
  S3 审批 payload 落这里)。
- ErrorInfo 提前定义(2.8 的 journaled 形态),ActivityFailed 即用。

## S2.2 EventStore — DONE

`internal/store/eventstore.go`:JSONL backend,per-session flock 独占
写者(`ErrLocked: held by pid N`)、append = seq++/id/ts 赋值 + 单行写
+ fsync、`ReadEvents` 免锁读、torn tail 容错(读者跳过;写者 open 时
truncate 修复——该 event 从未被 ack,丢弃安全)。

**Decisions**:
- stale lock 无需 pid 探活:flock 由内核在持有者死亡时自动释放,
  lock 文件里的 pid 纯为诊断信息(撞锁报错用)。执行包里的
  "kill -0 stale 检测"因此不需要——语义更强,记为简化偏离。
- 换行结尾的坏行 = 真损坏 → 读写都响亮报错;只有无换行的尾部
  是 torn tail(崩溃中断写)→ 修复。
- `crashAfter()` stub 落在 store(TODO(2.6)),append fsync 成功后
  调用——计数谓词的正确注入时点先钉住。
- Append 失败(write/fsync)不回滚 seq:文件可能已有半行,下次
  open 会修复;marshal 失败(未写盘)回滚 seq。

## S2.3 kernel — DONE

`internal/kernel`:Actor = goroutine + 64-buffer mailbox;
`Bus{Register, Subscribe, Send, Publish, Close}`;handler 返回子
envelope,actor 负责 `ChildOf` 盖章后路由(有 Target → send,
无 → publish by type);command 按 Envelope.ID 去重;handler
error/panic → actor 标 dead + publish `ActorCrashed`(以肇事
envelope 为 causation),不自动重启,后续 Send 报错。

**Decisions**:
- actor 粒度(kickoff open question):kernel 不预设,Bus 支持任意
  个;2.10 loop 重写时按最小可用定拓扑。
- dedup 集合在内存(非 fold):回放期的去重由 fold 天然给出,
  mailbox 级 dedup 只防运行期重复投递。
- mailbox 满时 send 持锁阻塞——原型级死锁风险,记档;S6 服务化
  若需要再换无锁投递。
- 子 envelope 路由失败(目标不存在/已死)按 crash 处理——静默
  丢子事实不可接受。
- forbidigo 从本步起在 internal/kernel 生效(测试也不用 sleep,
  全部靠 channel 同步 + mailbox FIFO 序断言)。
