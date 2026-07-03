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

## S2.4 fold/state — DONE

`internal/state`:`State{Conversation, Activities, Waiting, Timers, Run}`
+ `SubStateVersions()`(全部 v1,入 RunStarted 与 snapshot 头);
`Apply` 纯函数(copy-on-write helpers,输入 state 永不变);`Fold` =
从空态折叠。in-flight Activities 集合 = 钩子 3 落位(resume 时非空
即 in-doubt 信号);Timers = 未 fired 集合(2.11 resume 重调度依据);
Conversation 含 `ToolResults` map by call_id(2.10 assembly 的读取面)。

**Decisions**:
- **第五个 sub-state `timers`**(执行包只列了四个):2.11 resume 需要
  从 fold 读未 fired timer,归入 waiting 或 run 都语义不合,独立命名
  空间最干净。记为对执行包的加性偏离。
- ActivityFailed 一律移出 in-flight(该 attempt 已终结;retry 由新
  Started 重新加入)——in-doubt 语义因此简单:resume 时 in-flight
  非空 = Started 无终态。
- tool result 进 Conversation 的条件 = in-flight 里查到 kind=tool 且
  有 call_id(不解析 activity_id 字符串)。
- `TestApplyCoversRegistry`:Registry 每个类型零值过 Apply,漏写
  fold case 直接红——event 词汇表与 fold 的漂移在 CI 抓。
- `RunEnded` 把 `Run.Turn` 设为最终 turns(与 TurnStarted 同字段)。

## S2.5 调试工具 — DONE(按预授权提前至 2.4 之后)

`agentrunner events <session-id-or-prefix> [--state] [--json]`:美化
打印(seq/ts/type/compact payload 截断 100)、`--state` fold 转储
(缩进 JSON)、`--json` 原样 JSONL;session id 接受唯一前缀,歧义时
列出候选。`internal/state/statetest.AssertFoldEqual`:按命名空间
JSON 对比,**归一化**(空 map/slice、显式 null、缺键三者等价——
snapshot JSON round-trip 不得算分歧),失败报出具体 sub-state。

**Decisions**:
- AssertFoldEqual 放独立子包 `statetest`(不进生产依赖面)。
- events 的 bool flag 支持位置参数后置(partition 后再 Parse,
  stdlib flag 遇首个非 flag 即停)。
- `resolveSessionDir` 为 2.17 `sessions list`/`resume` 预留复用。

## S2.6+2.7 崩溃注入 harness + journal-inputs-first — DONE(按预授权合并)

`internal/crash`:两轨注入(`after:<EventType>:<n>` 计数谓词,挂在
EventStore.Append fsync 之后;`point:<name>` 命名点);S2 四个点注册
(`after_journal_input`/`after_exec_before_journal`/`after_snapshot_write`
/`before_run_end`);注册表封闭——未注册名 Point() panic、
`TestRegistryPinsS2Points` 钉死名单(删点即红);malformed env panic。
`runtime.IngestInput`:外部输入先 append(fsync)再消费,journaled
fact 以 cmd-id 为 causation。

**验证**:真子进程 harness(helper-process 模式)——armed 谓词处
exit 137;kill 后 ReadEvents 输入仍在、flock 随进程死亡自动释放、
store 可直接 reopen(2.7 崩溃场景 + harness 自测一体)。

**Decisions**:
- exit code 137(模拟 SIGKILL);`exit` var 可换(白盒测计数逻辑),
  子进程测试用真 os.Exit。
- 谓词 env 解析 sync.Once 缓存(进程内不变)。
- crash 包无 store 依赖(store → crash 单向)。

## S2.8 错误分类学 — DONE

`internal/errs`:8 类(执行包清单)+ `Class.Retryable()`(仅
rate_limit/server/timeout)+ `Error{Class,Msg,Err}` 可 wrap/Unwrap +
`ClassOf`(errors.As 提取,context 哨兵映射 canceled/timeout,默认
internal)+ `FromHTTPStatus`(429/5xx/401·403/4xx)。gemini 适配器
stream 错误经 `classify()` 上分类(genai.APIError 值类型 errors.As)。

**Decisions**:
- 传输层错误(非 APIError、非 context)分类为 `provider_server`
  ——连接重置类故障值得重试,比 internal 更符合语义。
- 分类学放 `internal/errs` 独立包(计划说 provider/base;provider
  包本身不该带分类政策,tool/timeout 类也要用——记为位置偏离)。
- ErrorInfo(event payload)与 errs.Error 的桥接留给 2.10
  (`ErrorInfo{Class: string(errs.ClassOf(err)), Retryable: ...}`)。

## S2.9 Clock 抽象 — DONE

`internal/clock`:`Clock{Now, WaitUntil(ctx, t)}`;`Real`(生产)、
`Fake`(手动 `Advance(d)`,按到期先后唤醒 waiter;`Waiters()` 供测试
同步)。过去目标立即返回;ctx 取消返回 ctx.Err()。48h 审批挂起场景
(3.5)一次 Advance 快进验证。

**Decisions**:
- 接口只两个方法——`Sleep` 不提供,一切等待都以绝对时刻表达
  (durable timer 的 `fire_at` 语义;相对时长在 resume 后会漂移)。
- `Fake.Waiters()` 暴露 parked 数供测试无 sleep 同步(spin+Gosched)。
- clock 包在 forbidigo 区外,是唯一合法 wall-clock 出口。

---

## S2.10 activity 执行器 + loop 重写主体 — DONE

S2 的核心步。`internal/agent/activity.go`:ActivityExecutor——一切
副作用的唯一通道:`ActivityStarted`(先落盘)→ 执行 → 终态落盘;
`crash.Point(after_exec_before_journal)` 卡在执行成功与终态落盘之间
(2.15 in-doubt 窗口);通用 retry/backoff(1s/4s 经 Clock,仅
Retryable 类,3 attempts,每 attempt 独立 Started/Failed 对);
`DiscardOnRetry` 接缝(S4 TurnDiscarded);`Progress` 字段(S4/S6
ephemeral 通道,S2 不用);args/results/错误消息全部过凭据 redaction
(新 `internal/redact` 包,`*_API_KEY/_TOKEN/_SECRET`)。

`loop.go` 重写:fold state 驱动——`decide(state, maxTurns) → action`
是唯一决策函数(doTurn/doLLM/doTool/doEnd),resume 用同一函数天然
续跑;`assembleMessages(state)` 从 Conversation.Messages + ToolResults
组装请求(**golden 测试未动一字节通过**——重写行为契约兑现);
appendE = journal+fold 单写入路径,causation 线性链;LLM/tool 全走
执行器;`before_run_end` 注入点落位;journal v0 删除(store/journal.go
及全部写入),events.jsonl 即 source of truth。

**迁移面**:CLI run 开 EventStore(传 SessionID/Version/Real clock);
acceptance `journal_valid` → `events_valid`(检查 envelope 形态 +
seq 无缝隙 + run_started 首 / run_ended 尾);场景 s1-02 改名
events-readable;e2e/loop 测试全部迁 EventStore;S1 集成测试在新
loop 上全绿。

**Decisions**:
- causation 链 = 线性(每 event 因于前一 event);kernel actor 拓扑
  暂不接入 loop(2.3 的 Bus 待 2.14+/S6 按需接),记 open question。
- LLM activity 标 `idempotent: true`(重跑安全,费用非正确性问题);
  tool 按 class:read=true,edit/execute=false(S3 细化)。
- tool 的 isError 结果 = 活动成功(模型可见错误),不是 activity
  失败——不触发 retry。
- LLM activity 的 Result 留空(消息走 AssistantMessage event),
  usage 挂 ActivityCompleted。
- 模型可见的 tool 结果也过 redaction(fold ToolResults 存的是
  redacted 版)——凭据不该回流进上下文,记为行为变更。
- state.addUsage 补 CacheWriteTokens(S1 会计口径 bug,顺手修)。

## S2.11 durable timer — DONE

executor 每 attempt 可挂 `Activity.Timeout`:`TimerSet`(fire_at 绝对
时刻)落盘 → WaitUntil goroutine 只发信号(**所有 append 留在 executor
goroutine**,无并发写)→ 到期 `TimerFired` 落盘 + runCtx 以
`errs.ErrActivityTimeout` 为 cause 取消;先完成则 `TimerCancelled`。
bash 墙钟超时迁移完成:tool.Executor 不再持有 timer(BashTimeout 字段
删除),按 `context.Cause` 区分 timed_out/canceled;loop 给 execute
类 tool 配 120s(常量归 agent 层)。`FirePendingTimers`:resume 扫
fold 未 fired timer,过期即刻 fire、未到期返回给 owner 重挂(2.13 用)。

**Decisions**:
- **新 event 类型 `timer_cancelled`**(S2 全集 14→15 的加性偏离):
  没有它,先完成的 activity 会在 fold 留 stale pending timer,resume
  会误触发。
- LLM 超时错误重分类:timer fired + Run 报 canceled → errs.Timeout
  (retryable);tool 超时是模型可见 IsError 结果(活动成功),与 S1
  行为一致,不触发 retry。
- timer id 确定性:`tm-<activity_id>-a<attempt>`。

## S2.12 进程组取消 — DONE

executor:非超时的 ctx 取消 → 等 run 有界 drain(bash killGroup:组
SIGTERM→5s 宽限→SIGKILL,ESRCH 确认组亡;WaitDelay 2s 兜底管道)→
**组退出后才落** `ActivityCancelled{partial_output}`(过 redaction)→
返回 canceled 类错误;挂着的 timer 先 TimerCancelled 清掉,不伪造
timeout。loop.abort 区分 reason:canceled 类 → `run_ended{canceled}`
(中断 ≠ 失败)。孤儿断言基础设施:`tool.SessionEnvVar`
(AGENTRUNNER_SESSION=<id>)标记 bash 子进程,CLI 注入 sessionID;
测试按 marker 扫 /proc(定向查找,不 grep 全局 ps),后台孙进程
一并断言死亡。

**Decisions**:
- ActivityCancelled 仅由"上层取消"触发;timeout 走 completed(tool)
  /failed(llm)路径——三种终态语义互斥。
- partial_output 存 string(event payload 已定 string 字段),tool
  结果 JSON 原样入内。

## S2.13 snapshot-resume — DONE

`store/snapshot.go`:`snapshots/<upto_seq>.json`(0600,temp+rename
原子写,`after_snapshot_write` 注入点);头含 sub_state_versions。
loop 在每个 turn 边界(TurnStarted 落盘后)写 snapshot。`Loop.Resume`:
最新 snapshot + seq 尾部 apply(无 snapshot 则全量 fold)→ 版本不
匹配拒绝(集合与逐版本都查)→ 已 ended 报错并附结果 → timer 扫
(`FirePendingTimers`)→ 进同一个 `drive()` 决策循环。Run/drive
重构:appendE/fold 状态收进 `driveState`,Run 与 Resume 共享 drive。

**验证**:真子进程崩溃场景——`after:turn_started:2` 处 kill(turn 1
的 read+edit 已落盘),父进程用**只含剩余 turn 的 fixture** resume:
任何 turn 1 重跑都会 drift/exhaust;断言 llm-t1 恰好 Started 一次、
2 turns 完成、文件改动幸存、log 以 run_ended 收尾。等价性质:真实
loop 产物上 fold(snapshot+尾)== fold(全量)(AssertFoldEqual)。

**Decisions**:
- snapshot 是优化不是事实源:丢失只导致更长的 fold。
- resume 时未到期 timer 不重挂(owner activity 重跑时自会重挂),
  过期的即刻 fire。
- Resume 对已 ended session 返回结果 + error(CLI 可打印结果并退出)。

## S2.14 等待状态注册表 — DONE

`agent/waiting.go`:`WaitRules` 封闭注册表,四变体一次画全
(INPUT@S4 / APPROVAL@S3 / TASKS@S6 / TIMER@S6),每行:可产生
stage、可中断性、中断决议名(approval → `denied_by_interrupt`,3.5
语义预埋)、非中断决议源。`CanProduce(kind, stage)` 供未来生产方守门
(S2 全部不可产生)。`ResolveWaitingOnInterrupt`:interrupt 先 journal
(`InputReceived{source: interrupt}`)再 `WaitingResolved{按表决议}`;
未知 kind 响亮报错。decide() 加 `doWait` 守卫:parked 状态下 drive
拒绝继续(S3/S4 才有 resolver),resume 不会越过等待乱跑。

**验证**:表驱动覆盖每格(合成 WaitingEntered);跨进程存活(S2 出口
标准的合成版:journal → close → reopen → fold 仍 parked → decide=
doWait);non-waiting no-op;interrupt 不进 conversation。

**Decisions**:
- interrupt 是控制输入不是会话内容:fold 对 `source=="interrupt"` 的
  InputReceived 不生成 user message(journal-inputs-first 仍满足)。
- 四 kind 目前全部 Interruptible=true;表结构保留 false 的表达力
  (S6 若有不可中断等待再启用)。

## S2.15 in-doubt — DONE

Resume 在 timer 扫和 drive 之前查 in-flight 集合(2.4 的钩子 3 兑现):
非 idempotent 的 Started-无终态 → 返回 `InDoubtError`(列出
activity_id/name/attempt,"refusing to re-run"),**不重跑**;
idempotent(read 类、LLM)→ 不算 in-doubt,decide() 自然重跑。S3
的 per-tool-class 决议政策来之前,人用 `agentrunner events` 检查后
自行处置。

**验证**:真子进程 `point:after_exec_before_journal:2` kill(bash 已
写 marker,终态未落盘)→ resume 拿到 InDoubtError、marker 恰一行
(重跑会变两行);合成 idempotent in-flight(read_file Started 无
终态)→ resume 重跑、结果落盘、in-flight 排干、2 turns 完成。

**Decisions**:
- crash harness 扩展:`point:<name>[:<n>]` 支持命中计数(该点在
  LLM 与 tool 活动都会经过,第 1 次命中是 llm-t1)——加性扩展,
  crash 包测试钉住。
- idempotent 重跑时 attempt 从 1 重新计(旧 Started 的 map 项被新
  Started 覆盖,终态后排干)——记为已知小瑕疵,不影响正确性。

## S2.16 run 收尾 epilogue 骨架 — DONE

`agent/epilogue.go`:固定序列 `quiesce → auto_publish → barrier →
RunEnded`(钩子 2 落位)。三个 slot S2 皆 no-op(quiesce 待 S6 并行
任务用 Activities sub-state 填;auto_publish/barrier 是 S7 预留位);
**此后所有 run 结束行为必须挂 slot,不得绕序列**。doEnd 与 abort
两条终态路径都走 `runEpilogue`:正常结束 hook 报错即中止(终态不落);
abort 路径 best-effort 硬推到底(能落 run_ended 就落)。
`before_run_end` 注入点收进 epilogue(barrier 之后、终态之前)。

**验证**:hook 顺序钉死;正常结束遇 hook 错误不写终态;best-effort
穿透错误仍写终态;三种 reason(completed/max_turns/error|canceled)
共用同一路径(既有 loop 测试覆盖)。

**Decisions**:
- epilogueSequence 为包级 var,测试以替换+恢复方式插桩(slot 体
  可换、序不可变的机械保证)。

## S2.17a CLI 收口 — DONE

`agentrunner resume <session-id-or-prefix>`:从 run_started 里 journal
的 **spec JSON + workspace_root** 重建 Loop(无需原 spec 文件;
RunStarted 加性扩展两字段),provider 走 defaultProviderFactory;
退出码语义同 run。`agentrunner sessions list`:mtime 倒序表格
(SESSION/STATUS/TURNS),status 来自 fold(waiting 显示
`waiting:<kind>`)。`after_journal_input` 命名点补上调用位
(IngestInput append 之后——2.6 注册时欠的 call site)。

**验证**:CLI 级真子进程 crash(`after:turn_started:2`)→ CLI resume
(唯一前缀)→ 2 turns 完成、文件修好、sessions list 显示 ended;
未知 session exit 2;空列表友好输出。

**Decisions**:
- spec 全文进 RunStarted(而非只记路径):resume 不依赖 spec 文件
  未被改动/移动;prototype 无隐私顾虑(spec 不含凭据,系统约定)。
- 旧 session(无 spec 字段)resume 明确报错,不猜。

## S2.17b 全崩溃注入矩阵(S2 出口门)— DONE

`TestCrashMatrix`:**10 行矩阵**,覆盖全部 4 个命名点 + 3 类计数谓词,
沿标准两 turn read+edit run 的事件序逐点 kill(真子进程 exit 137)后
resume:每行断言 completed/2 turns/文件修好/seq 无缝隙/run_ended 收尾
/fold ended/in-flight 空——"分毫不差";`edit-executed-unjournaled`
行断言 InDoubtError。行清单:input 落盘后(谓词+命名点两种)、llm
已执行未记账(幂等重跑)、assistant 落盘后、read 已执行未记账(幂等
重跑)、read 结果落盘后、edit 已执行未记账(**in-doubt**)、turn 2
边界、snapshot 写后(真 snapshot+空尾 resume)、run_end 之前(resume
零 LLM 调用直接收尾)。

S2 acceptance 场景包(4 个,`accept --stage 2` 全 PASS,e2e gate 扩到
双 stage):s2-01 崩溃 resume 端到端(binary 级)、s2-02 in-doubt 上浮
拒绝重跑(marker 恰一行)、s2-03 等待状态跨进程存活(合成 event +
sessions list 显示 waiting:approval)、s2-04 events 调试工具。

**S2 完成标志核对**:崩溃矩阵全绿 ✓;in-doubt 上浮 ✓;等待状态跨
进程存活(合成 event)✓。**Stage 2 实现完成**,待出口对抗式 review。

---

## Stage 2 出口对抗式 review — 三镜头(durability / concurrency / semantics)

三个并行 reviewer 共报 24 项;triage 后 **16 项修复**(本 commit)、
8 项记档递延。

**已修复(按镜头)**:
- [D-P1] `ActivityCancelled` 后崩溃 → resume 重跑半执行命令:fold 现在
  把取消的 tool call 解析为 `{"error":"[interrupted by user]",
  partial_output}` 的 IsError 结果,decide() 不再视为 pending。
- [D-P1] run_started 与 input_received 之间崩溃 → resume 空会话调模型:
  Resume 检测无 input 且 `Run.Task` 非空时从 RunStarted 重新 ingest;
  矩阵加行 `run-started-only`(现 12 行)。
- [D-P2] snapshot 无 fsync + 损坏 snapshot 卡死 resume:WriteSnapshot
  改 write+fsync+rename;LatestSnapshot 跳过不可读的、回退旧的;
  Resume 对 snapshot 层错误降级全量 fold(snapshot 永远只是优化)。
- [D-P2] events.jsonl 目录项无 fsync:open 时 fsync session dir。
- [D-P2] 全量 fold 路径不查版本:Resume 现在也校验 RunStarted 里的
  sub_state_versions。
- [C-P0] kernel bus 三方死锁(持 b.mu 阻塞投递):重写锁规则——b.mu
  只保护表,投递永不持锁;actor 循环 ctx 感知,Close 不关 channel;
  Register-after-Close panic。
- [C-P1] killGroup pid 复用:reaper 先 close(reaped) 再送 done;
  killGroup 观察到 leader 已被 reap 即停止向该 pgid 发信号(打错
  无辜进程 > 漏杀抗 TERM 孤儿)。
- [C-P1] timer fired 路径把任意错误盖章成 retryable Timeout:仅当
  错误源于我们的取消(isCancellation)才重分类;TimerFired append
  失败按 store 错误原样上浮并排干 run。
- [C-P2] Append 写失败后 torn 半行被下次 append 粘连:broken latch,
  写/fsync 失败后拒绝所有后续 append,重开修复。
- [C-P2] bash done-vs-cancel select 无偏向(完成的命令被记成
  canceled):cancel 臂先非阻塞查 done。
- [C-P2] Fake.WaitUntil ctx 取消泄漏 waiter 项:取消时移除。
- [S-P1] task 文本绕过 redaction(shell 展开凭据入 run_started/
  input_received/上下文/snapshot):appender 对**所有** payload 统一
  redact(assistant_message 一并覆盖)+ task 在 IngestInput 前先 scrub;
  回归测试断言凭据在全事件流无泄漏且有 marker。
- [S-P2] events_valid 弱断言:补 ts RFC3339 解析、id==evt-<seq>、
  payload 非空、type 对照 event.Registry。
- [S-P2] 矩阵缺 `after:activity_completed:1` 窗口:加行
  `llm-completed-unmessaged`。
- [S-P2] resume 已结束 session 不打印结果:CLI 打印结果行,completed
  → exit 0(无事可做 ≠ 失败),其余 exit 1。
- [S-P2] s2-04 场景 `--state` 空转:补 `"reason": "completed"` 断言。

**记档递延(均已核实,决策如下)**:
- [D-P2] abort 对 transient error/cancel 落 run_ended 致不可 resume
  (kill -9 反而可续):**接受现状**——S2 的 run 语义是单发;S4 交互
  session 引入 reopen 时统一解决。ActivityFailed 同窗口(非幂等 tool
  错误)在 S2 不可达,S3 tool 活动错误落地时必须回访(挂 S3 kickoff)。
- [D-P2] resume 后 attempt 从 1 重计(审计口径分歧 + 重试预算跨
  restart 重置):接受,fold 无感;S3 预算需要时再从 fold 推 attempt。
- [C-P2 latent] Run 闭包读 ds.s、LLM 活动禁配 Timeout(配了即数据
  竞争):已加代码注释;S4 流式重构时以类型手段消除。
- [C-P3] kernel seen 无界/crash 事件在 Close 竞态下可能丢;bus 尚无
  生产流量,S6 服务化时回访。
- [C-P3] 成功 bash 的后台孙进程存活(marker 只用于测试断言):
  session 级进程清扫挂 S6(quiesce slot 顺带)。
- [S-P2] LLM retryable 失败 attempt 的 usage 丢失(低报计费):挂
  S3.7b 预算结算时修(provider 需在 error 路径带出已收 usage)。
- [S-P2] binary version 漂移 resume 不设防:**决策**——兼容契约是
  sub_state_versions,binary version 仅信息;记档即可。
- [S-P2] record 录制器按单 delta redact(秘密跨 delta 分片可漏):
  S4 流式协议重构 recorder 时合并修。

---

**Stage 2 正式关闭**(实现 + 出口 review + 修复全部落地,
`accept --stage 1/2` 全绿,崩溃矩阵 12 行全绿)。

## S3 kickoff refinement — DONE

PLAN.md 新增 **S3 执行包**:包布局(pipeline/config/hook)、5 个加性
event 类型(effect_resolved/approval_requested/approval_responded/
mode_changed/limit_exceeded)、Effect 描述与 effect_id 方案、关卡序
与 deny 短路/ask 聚合语义、permissions YAML 形态(首条命中,三源
拼接序 user>project>spec,无命中按 mode 默认)、mode 跃迁表 +
exit_plan_mode 内置 tool、trust 注册表(不受信 project 的 allow
降级 ask)、budget 粗价目表与 farewell 挂点、hooks v0 协议(exit 2
= block,10s 真实 timer 豁免记档)、审批 CLI(非 TTY 走
AGENTRUNNER_APPROVE)、恢复语义(挂起 kill → resume 重提示;
between_gate_and_resolved → in-doubt)。回访项:非幂等 tool 的
ActivityFailed-重试窗口(S2 review 递延)。

下一步:S3.1 管线框架。

## S3.1 管线框架 — DONE

`internal/pipeline`(forbidigo 区,纯评估无 I/O):`Effect{ID, Kind,
ToolName, Class, Args, CallID, EstTokens}`、`Gate{Name, Check} →
Decision{allow|ask|deny, reason}`、`Pipeline.Evaluate`——deny 短路
(后续关不跑)、ask 聚合(继续评估,后续 deny 仍胜)、gate 返回
非法 action 报错、nil pipeline = 全放行。新 event `effect_resolved
{effect_id, call_id, verdict, gate_results}`;fold:**deny 即该 call
的模型可见结果**(`denied: <reason>` IsError 入 ToolResults——
decide() 不会重试被拒 effect,拒后崩溃 resume 也直接跳过)。loop:
`adjudicate()` 在 LLM 与 tool 活动前统一评估并落盘(判定终结后、
执行前);deny 的 tool 不产生任何 activity 事件,loop 继续;deny 的
LLM 暂 hard abort(TODO 3.7c 优雅收尾)。

**Decisions**:
- ask 在 3.5 前**显式降级 deny**:附加 gate_result(gate:
  "pipeline", reason 注明 "no approval flow yet (3.5)")——绝不静默
  放行、也绝不无解释拒绝;3.5 落地时替换此块。
- GateResult 类型放 event 包(pipeline 复用),event 保持叶子包。
- LLM effect 每 turn 都落 resolution(即使空 pipeline)——事实完整
  优先于日志体积。

**验证**:落盘时点单测——allow: resolution 先于 activity_started;
deny: 无 activity 事件、下一 turn 模型看到 denied 文本、run 正常完成;
ask: 降级链完整可见;llm: 每 turn 有 resolution。

## S3.2 in-doubt 扩展 — DONE

新 event `effect_requested{effect_id, call_id, side_effecting}`(进
关卡前落盘);第 6 个 fold sub-state `effects`(requested 加、
resolved 消,SubStateVersions 加键——**旧 session 无法用新 binary
resume,版本检查按设计拒绝**,原型可接受记档);命名注入点
`between_gate_and_resolved` 落位(Evaluate 之后、resolution 落盘
之前)。resume 语义:pending effect 且 `side_effecting`(管线含
hook 类关卡,`pipeline.SideEffectingGate` 接口声明)→ 并入
InDoubtError(Effects 字段,"mid-adjudication, hooks may have run");
纯关卡窗口 → 静默重评估(重新 adjudicate 覆盖旧 pending)。

**验证**:真子进程 kill 于该点(hit 2 = tool effect 窗口)×
{side-effecting → InDoubtError 含 1 个 effect;pure → resume 重评估
2 turns 完成}。

**Decisions**:
- "进关卡" 需要可观测事实 → effect_requested(执行包漏列,加性
  补充记档)。
- side_effecting 布尔由管线静态推导(任一 gate 实现
  SideEffecting()==true),journal 进事实供 resume 决策——resume
  时的管线配置可能不同,以崩溃时刻的事实为准。
