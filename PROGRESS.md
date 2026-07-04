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

## S3.3 permission rules — DONE

`pipeline.PermissionGate`:规则表(首条命中即生效——顺序即优先级)
+ 无命中按 mode 默认表(default/plan/acceptEdits/bypass × 四 class
全表);规则 = `{tool?, path?, command?, action}`,多条件合取;
path glob(`*`/`?` 不跨 `/`,`**` 跨)对 workspace 相对路径
(先经 WS.Resolve 归一),command glob(`*` 匹配任意含空格)对
整条命令;**越界路径无条件 deny——先于规则、先于 mode、bypass
也不豁免**(钩子 1 的关卡层复检,`src/../../etc` 表驱动钉住)。

**Decisions**:
- command glob 的 `*` 不带 path 语义(匹配任意字符)——"go test *"
  要能配 "go test ./..."(执行包已定,实现记档确认)。
- 规则合取:path 条款对无 path 参数的 tool(bash)永不匹配。
- malformed args 在关卡层按空值处理(执行层会给模型可见错误)。
- **CLI 尚未接线 PermissionGate**:ask 在 3.5 前会降级 deny,接线会
  打破 S1/S2 acceptance(edit 全拒);3.5 审批流落地后随 3.6 mode
  一起接入 CLI。3.3 验收 = 表驱动单测(计划原文如此)。

## S3.4 配置分层 + 信任 — DONE(从 S5 提前,计划原文注明)

`internal/config`:`Settings{permissions, hooks{pre_tool, post_tool}}`
严格解析(未知键/非法 action 拒);`Merge(user, project, spec, trusted)`
——规则拼接序 user > project > spec(首条命中配合 = user 优先);
**不受信 project 的 allow 降级 ask(只收紧不放宽),hooks 整段丢弃**;
spec 永不携带 hooks(可移植内容 ≠ 工作站策略)。trust 注册表
`trusted.yaml`(0600,realpath 存储,symlink 同判);CLI
`agentrunner trust <dir>`(幂等)。AgentSpec 加 `permissions:` 字段
(最低优先级源)。

**Decisions**:
- spec 不设 hooks 字段:hooks 是本机策略,spec 是可分享内容——
  不受信 spec 经 hooks 提权的面根本不开。
- trust 判定用 EvalSymlinks 双向归一(注册与查询都 realpath)。

## S3.5 审批流 — DONE

事件链:ask → `approval_requested{approval_id, effect_id, call_id,
gate_results, payload_ref 预留}` → `waiting_entered{approval, detail=
完整请求}` → resolver 应答(**先 journal** `approval_responded`)→
`waiting_resolved` → `effect_resolved{allow|deny, gates+approval 关
判定}` → 放行执行 / 拒绝渲染。`ApprovalResolver` 接口;默认
`EnvApprovals`(AGENTRUNNER_APPROVE=always|never,缺省 **fail
closed** 拒绝——loop-mode 不悬挂);TTY 交互版在 3.10。

**Effects sub-state 扩展**:`{Pending, Allowed}`——resolved-allow 到
activity 终态之间的窗口进 `Allowed`,adjudicate 先查:**已批准的
effect 崩溃后 resume 绝不重新问一遍**(activity 终态经 call_id/
activity_id 约定回收 Allowed 项)。3.2 的 sub-state 形态就地改
(同 stage 内迭代,版本仍 1,无线上会话,记档)。

**denied-by-interrupt**:`Loop.Interrupts` 通道;等待中 interrupt →
journal input(interrupt) → approval_responded{deny, interrupt} →
waiting_resolved{denied_by_interrupt} → effect_resolved{deny,
"[interrupted by user]"} → **loop 继续**(拒绝结果模型可见)。CLI:
`signalContext()`——首个 Ctrl-C = interrupt,第二个/SIGTERM = 硬取消
(run 与 resume 都接线)。

**验证**:approve 全链事实序断言 + 文件真落盘;FakeClock 挂 48h 后
批准原地继续(S3 完成标志句);interrupt 路径(模型下一 turn 看到
"[interrupted by user]"、run 完成、文件未写);**崩溃注入
after:waiting_entered:1(挂起中 kill)→ resume 重新提示 → 批准 →
继续完成**(S2 出口欠的验证在此补齐,计划原文)。全部 -race 通过。

**Decisions**:
- resolver 在 goroutine 跑、prompt 在 drive goroutine 预构(ds 不
  跨 goroutine——race detector 抓过一次,修法记档)。
- resume 到 doWait(approval)复用 awaitApproval(请求 payload 存于
  Waiting.Detail,不重复 journal approval_requested)。

## S3.6a–d mode — DONE(+ CLI 全管线接线)

- **3.6a 数据模型**:mode = fold sub-state(第 7 个,`mode_changed`
  event 驱动,空 = default);工具面过滤 `ClassAdvertised`(plan 只
  广告 read/wait——**双门**:面过滤 + 关卡 mode 默认,模型幻觉隐藏
  tool 也会被拒);Effect.Mode 携带活 mode(mode 可中途变,gate 构造
  时快照不可靠)。
- **3.6b prompt 注入**:plan mode 在 system prompt 尾部追加段
  (S4.4a 收编,计划内迁移);默认模式零注入(golden 不受扰)。
- **3.6c 跃迁**:内置 tool `exit_plan_mode`(class wait,defs JSON);
  permission gate 视之为跃迁政策(plan → Ask,非 plan → Deny);
  批准后 harness 级执行(不进 tool.Executor)+ `mode_changed{plan→
  default, cause: exit_plan_mode approved}`;跃迁规则表
  `ValidTransition`(any→bypass 仅 CLI 启动)。
- **3.6d bypass 语义**:bypass 在 permission/budget 层全 allow,但
  hooks 关卡照跑(组合测试钉住);**越界拒绝连 bypass 也不豁免**
  (3.3 已定)。
- **CLI 接线**(3.3 递延项落地):`buildPipeline` = 三源 config 合并 +
  PermissionGate;`--mode` flag;spec `mode:` 字段(校验);resume 重建
  管线(活 mode 走 fold);不受信 project 有配置时 stderr 提示。
- **迁移面**:CLI 级测试 spec 与 s1/s2 acceptance 场景 spec 加
  `permissions: [{action: allow}]`(关卡激活后 edit/execute 默认 ask
  → env 拒);tool 注册表钉住测试 + spec golden 更新(exit_plan_mode
  入册)。

**验证**:过滤表全 mode×class;跃迁表 allowed/denied 双向;plan 全流程
集成(turn 1 过滤面+注入 → exit_plan_mode 审批 → mode_changed → turn 2
全面无注入);拒绝路径留在 plan;bypass+hooks 组合。

## S3.7a–d 预算 — DONE

- **3.7a sub-state**:第 8 个 fold 命名空间 `budget`(reservations
  map;settled = Run.Usage 既有口径)。`effect_resolved{allow}` 加性
  字段 `reserved_tokens` 记入,activity 终态(Completed/Cancelled)
  经 effectIDFor 释放。
- **3.7b 预留与结算**:LLM 按 `model.max_tokens` 预留;tool 按类价目
  (read 500 / edit 1000 / execute 2000 / wait 0);ApprovalRequested
  携带 `est_tokens`(挂起跨 crash 后批准仍能预留)。
- **3.7c 优雅收尾**:BudgetGate deny(LLM)→ `limit_exceeded{tokens,
  limit, used}` → runEpilogue(reason "limit_exceeded")——不是 error、
  不是 crash;tool 的预算拒绝走普通 deny(模型可见,可收尾);spec
  `budget: {max_total_tokens}`(0=无限)+ 校验;CLI 管线加 BudgetGate。
- **3.7d TOCTOU 合成**:8 goroutine 经互斥串行 adjudicate(模拟
  S4.3 共享 fold 的并发裁决),reserve-then-settle 使第二个 600 token
  请求看见第一个的预留 → 恰好 1 个放行(600+600>1000 不双越)。
  S4.3 真并行落地时按计划复验。

**Decisions**:
- BudgetView 由 loop 从 fold 快照进 Effect(gate 保持纯函数,不持
  状态引用)。
- bypass mode 预算不 bind(3.6d 语义延伸,gate 内判)。
- LLM 预算拒绝时的 farewell 消息:S3 先以 slog + limit_exceeded 事实
  + 终态 reason 呈现;面向模型的告别 turn 留 S4 流式协议一并做(记档)。

## S3.8 hooks v0 — DONE

`internal/hook`:`Runner{pre_tool, post_tool}`——pre 以 JSON(effect
描述)stdin 调用,**exit 0 = observe、exit 2 = block(stderr 即模型
可见理由)、其他 = observe + 警告**(坏 hook 不得静默否决);首个
block 短路后续;post 收结果 JSON,stdout 汇成 `ActivityCompleted.
hook_note`(加性字段,过 redaction);10s 真实超时(hook 是外部进程,
forbidigo 区外,豁免记档)+ WaitDelay 2s 防孙进程扣住管道。
`hook.Gate` 实现 pipeline.Gate + SideEffecting(有 pre hook 即声明)
——3.2 的 in-doubt 机制自动覆盖。executor 加 `PostRun` 接缝(成功
执行后、终态落盘前)。CLI:hook gate 列关卡序首位(pre-hooks →
permission → budget,执行包既定序),post runner 进 Loop.Hooks。

**验证**:协议表驱动(observe/block/警告/首 block 短路/超时=警告);
gate 适配(llm 效应直通、无 pre hook 不声明副作用);**恢复路径不重
跑 hook**:真 pre hook 写 marker,kill 于 between_gate_and_resolved
→ resume InDoubtError、marker 恰一行(计划的崩溃注入验证);post
note 入 journal 断言。

## S3.9 错误渲染表 + S3 回访项 — DONE

`errs.RenderForModel(class, detail)`:8 类各一行的归一化模型可见
渲染(未知类归 internal;per-provider 线上形态映射留 S4.7,计划
原文)。`ActivityFailed` 加性字段 `final`(retry 政策耗尽的那次);
fold:**final 的 tool 失败渲染为该 call 的模型可见错误结果**(loop
继续、模型反应),同时释放预算预留;loop 的 doTool 对"已在 fold
解析的终局失败"continue 而非 abort(cancel/harness 失败仍 abort)。

**S3 回访项关账**:non-final 失败**保留 in-flight 项**——重试
backoff 窗口崩溃时,非幂等活动经 2.15 in-doubt 上浮而非静默重跑
(此前 Failed 一律移出 in-flight,该窗口是盲区)。

**验证**:渲染表每行一测(含未知类);finality fold 测试(non-final
保留/final 渲染+排干);executor 级连续性(终局失败 → call 解析 →
decide 不再重试)。

**Decisions**:
- 渲染在 fold 内调用(确定性纯函数,与 fold 契约相容)——渲染表
  变更会改变历史 fold 结果,记为已知耦合,S4.7 重构渲染层时回访。

## S3.10 CLI 审批 UI + S3 acceptance 场景包 — DONE

`ttyApprovals`:展示 tool/args/全 gate 判定,读 `y`/`n [reason]`;
读取在 goroutine 与 ctx 竞速(终端读不可取消,进程退出时回收);
**真终端检测用 term.IsTerminal**(初版用 ModeCharDevice,acceptance
跑挂——/dev/null 也是字符设备,修正记档);非 TTY 回退 fail-closed
EnvApprovals。run/resume 双接线(提示走 stderr,stdout 属 run 输出)。
手动 TTY 验收 DEFERRED(出口人工检查点,与 S1 TUI 同批)。

`accept --stage 3` 三场景全 PASS,e2e gate 扩三 stage:
- s3-01 plan mode 全流程(完成标志句 1)
- s3-02 审批拒绝渲染+run 继续(3.5/3.10 行为)
- s3-03 不受信 hooks 不执行、trust 后执行(完成标志句 4)

**0.6 偏离记档**:完成标志句 2(FakeClock 挂 48h)与句 3(合成
TOCTOU)本质是合成时间/合成并发,无法经 binary 场景表达,映射到
命名 go test(TestApprovalHangsTwoDaysThenApproved /
TestBudgetTOCTOUSyntheticConcurrency),作为"合成场景"记入台账。

**Stage 3 实现完成**,待出口对抗式 review。

---

## Stage 3 出口对抗式 review — 三镜头(correctness / security×2 / contract)

四份报告(security 跑了两份,高度一致)。triage 后 **13 项修复**、
若干记档。

**已修复**:
- [C-P1] 挂起审批 + 真 hook 管线崩溃后不可 resume(pending side-effecting
  effect 被误判 in-doubt):`AwaitingApprovalEffect` + Decisions 排除——
  到达 WAITING_APPROVAL 证明所有关卡(含 hook)已跑完,不算 in-doubt。
  既有测试用无 hook 管线掩盖了此洞,新增 `TestApprovalWithHooksSurvivesCrash`。
- [C-P1] exit_plan_mode 跃迁非原子(ActivityCompleted 与 ModeChanged 之间
  崩溃 → 卡在 plan):**跃迁改由 fold 从 exit_plan_mode 自身的
  ActivityCompleted 派生**(单事件原子),删除 loop 的独立 ModeChanged 发射
  (startup mode 仍用 ModeChanged)。
- [C-P2] 审批答复不跨 WaitingResolved→EffectResolved 崩溃(会重问):
  **ApprovalResponded 折叠进 Effects.Decisions 并清 Waiting**(答复即权威),
  adjudicate 见 Decisions 直接从答复解析不重问;`TestApprovalDecisionDurableAcrossResolveGap`
  (resolver 被调用即 fail)。
- [C-P2] exit_plan_mode 标非幂等致崩溃后伪 in-doubt:toolIdempotent 纳入
  ClassWait(无副作用 stub 重跑安全)。
- [S-HIGH] plan mode 的 edit/execute 硬拒可被 allow 规则绕过:新
  `hardFloor` + `FloorGate`——工作区越界 + plan 硬拒在**规则表与 mode
  默认之前**判定,任何规则不可覆盖;FloorGate 列关卡序首位(hooks 之前),
  被拒 effect 不再触发副作用 hook。
- [S-HIGH] spec 绕过 trust:**禁止 spec 设 mode: bypass**(bypass 是关卡
  杀手开关,仅 --mode 可选)。spec 权限规则**不**收紧(信任非对称记档:
  project settings.yaml 是静默 repo 内容→收紧;spec 由用户显式命名→放行)。
- [S-P1/HIGH] command deny 规则换行绕过:globMatch 加 `(?s)`(`.` 跨行),
  path glob 加 `(?i)`(大小写不敏感文件系统);`TestCommandDenyResistsNewlineEvasion`。
- [S-P2] hook 前于 permission 致被拒 effect 仍跑 hook:FloorGate 硬拒
  短路(见上)。
- [S-P2] hook 拿到凭据 env:`scrubbedEnv` 剥离 `*_API_KEY/_TOKEN/_SECRET`。
- [S-P2] hook 无进程组:Setpgid + 超时 kill -pgid(对齐 bash)。
- [S-P2/LOW] 未知 tool class 失败开放:modeDefault 未知 class 一律 fail
  closed(plan→deny,其余→ask);`TestUnknownClassFailsClosed`。
- [S-LOW] effect-id 命名空间(call_id 模型可控可撞 eff-llm-t<n>):tool
  effect 改 `eff-tool-<call_id>`,与 LLM 空间不相交。
- [T-P2] s3-01/s3-02 场景弱于标题:scripted 加 `tools_exclude`,plan-mode
  场景断言 turn1 排除 edit_file/含 exit_plan_mode、turn2 含 edit_file;
  approval-deny 场景断言 turn2 last_message_contains "denied"。
- [T-P2] PLAN.md sub-state 文档漂移:§2.4 与 S3 执行包更新为
  effects/mode/budget 三个(勘误"两键")。

**记档裁定(不改)**:
- [T] S2 struct 加 omitempty 字段(HookNote/Final):加性、round-trip 安全,
  沿用 S2.17(RunStarted.Spec)先例,判"只加不改"允许。
- [T] 完成标志句 2(48h)/句 3(TOCTOU)映射命名 go-test 而非 binary
  场景:合成时间/并发无法 binary 表达,§0.6 carve-out 批准(已记 S3.10)。
- [S] AGENTRUNNER_APPROVE=always 抵消不受信收紧:loop-mode footgun,
  文档级——auto-approve 是显式选择。
- [S] hook stdin args 不 redact:hook 是受信代码(仅受信 project/user
  hook 执行),需真实 args 才能审计/拦截;与 journal redaction 的非对称
  是有意的(env 凭据已剥离)。

**Stage 3 正式关闭**。下一步:S4 kickoff refinement。

## S4 kickoff refinement — DONE

PLAN.md 新增 **S4 执行包**:细化交互与上下文的 8 模块序列。关键决定:
- 包布局:`internal/protocol`(输出事件流)、`agent/assembly.go`(4a 抽出)、
  `provider/anthropic`(4.7)。
- 加性 event:`TurnDiscarded`(delta 流出后 retry 的前端重开流信号)、
  `ContextCompacted`(5)、`MalformedToolCall`(6)。
- **输出 protocol** 区别于 provider 输入侧:面向 surface 的 StreamEvent
  (text_delta/tool_call/.../error/discard),用户可见错误(protocol)与
  模型可见错误(errs.RenderForModel)分离。
- ephemeral 通道兑现:2.10 的 Activity.Progress 在 4.1 接线,delta 不落
  journal(TurnDiscarded 契约)。
- 并行 tool call(4.3):同 turn 多 allow call 并发、到达序落盘、assembly
  原序重排;**appendE 串行化是核心不变量**(fold 单线程);一个 turn 内
  多 ask 顺序审批;3.7d TOCTOU 真并发复验。
- interrupt(4.2)复用 Loop.Interrupts + 500ms 交互取消宽限(区别于
  timeout 取消的 5s 宽限)。
- caching(4c):budget 结算修正为 input+output-cache_read 真实计费口径。
- 顺序微调预授权:4a 可提前到 4.1 前(streaming 依赖独立 assembly 更干净)。

回访项:AGENTRUNNER_APPROVE footgun(4.2 后复审)、recorder 单-delta
redact 跨分片泄漏(4.1 流式重构 recorder 时合并修)。

下一步:S4.1(先做 4a assembly 抽取)。

## S4.4a assembly 组件 + 拼装序 — DONE(按预授权提前到 4.1 前)

`agent/assembly.go`:`Assemble(state, spec, toolDefs, turn) →
CompleteRequest`——fold → 请求的唯一模块。固定拼装序:system prompt →
mode 注入(3.6b 收编)→ conversation transcript;工具面按活 mode 过滤
(3.6a)。`assembleMessages`/`advertisedTools` 从 loop.go/mode.go 迁入。
loop 的 doLLM 改调 `Assemble()`。**request_assembly.golden 一字节未改
通过**——纯抽取,行为保持。

**Decisions**:
- Assemble 为包级函数(非方法):assembly 是 fold→wire 的纯变换,不持
  Loop 状态,便于 4c 的 byte-stability 独立测试。
- env 块(session start 冻结)留 4c;S4.4a 先落拼装序骨架。

## S4.1 协议 v1 + streaming — DONE

`internal/protocol`:**输出事件流**(面向 surface,区别于 provider 输入
侧 StreamEvent):`Event{kind, turn, text, tool, ...}`,kind ∈ run_start/
turn_start/text_delta/message/tool_call/tool_result/approval_request/
mode_changed/discard/error/run_end;`JSONSink`(每行一 JSON,并发安全)
+ `Discard`。`provider.CollectTurnStreaming(stream, onDelta)`:边收边回调
text delta。loop 接线:doLLM 经 onDelta 发 text_delta(**ephemeral,不落
journal**——成功后 assistant_message 才落,TurnDiscarded 契约);turn/
tool/run 各 emit;旧 `agent.Sink` 接口删除,统一 `Loop.Out protocol.Sink`。
CLI:`textRenderer`(delta 内联流式渲染)+ `--json`(JSONSink);旧
textSink/compactJSON 删除。

**TurnDiscarded event(加性)**:LLM 已流出 delta 后 retry → 经 Activity
的 `DiscardOnRetry`(从 executor 字段移到 per-activity)journal
`turn_discarded` + emit discard(前端重开流信号);fold 无半成品可撤
(assistant_message 只在成功后落)。

**回访项关账**:recorder 单-delta redact 跨分片泄漏(S2 review 递延)——
Complete 现**累积连续 text delta 再整体 redact**,密钥跨 delta 边界仍
被 scrub;`TestRecorderRedactsCrossDeltaSecret`。

**验证**:JSONSink 行编码/sparse;流式 delta 序(2 delta→message);
TurnDiscarded 全链(partial→discard→final,final 消息干净、turn_discarded
入日志);cross-delta redaction。用户可见错误(protocol error)与模型
可见错误(errs.RenderForModel)分离——protocol 注释钉死。

**Decisions**:
- text delta 用回调而非 channel(executor 单线程,回调最简);Progress
  字段(2.10 预留)暂未用——delta 走 CollectTurnStreaming 更直接,
  Progress 留 S6 task tail。
- TurnDiscarded retry 测试用 Real clock(FakeClock 会阻塞 backoff)。

## S4.2 steering/interrupt — DONE

**interrupt 语义分层**(CLI signalContext 改 send-once 缓冲):首个 Ctrl-C
= **steering interrupt**(一次,缓冲送)→ 取消当前 activity、run 继续;
第二个 Ctrl-C / SIGTERM = 硬取消 → ctx cancel → abort。新 cancel cause
`errs.ErrUserInterrupt`(canceled 类)。

- `Loop.interruptScope(ctx)`:每个 LLM/tool activity 外包一层——interrupt
  到达 → `cancel(ErrUserInterrupt)`;`steered(actCtx)` 判定。LLM 被 steer
  → journal InputReceived{interrupt}(audit,source==interrupt 不进
  conversation)→ **continue**(decide 见 turn 无 assistant msg → 重跑,
  interrupt 已消费不循环)。tool 被 steer → ActivityCancelled 已渲染
  `[interrupted by user]`(S3 fold)→ journal interrupt → continue,模型
  下一 turn 反应。
- **500ms 交互取消宽限**:bash killGroup 按 cause 选 grace——
  ErrUserInterrupt → `bashInterruptGrace`(500ms),timeout → 5s;
  killGroup 签名加 grace 参数。
- awaitApproval 的 denied-by-interrupt(3.5)与 steering 互斥(审批在
  adjudicate 内、activity 之前,parked 时无 interruptScope 活动)。

**验证**:steering during LLM(卡住的 model call → 中断 → 取消 activity
+ interrupt input + discard surface → 重跑完成);steering during bash
(pid marker → 中断 → 5s 内杀掉、ActivityCancelled、模型见
[interrupted by user] → 完成);测试改 send-once 语义(blockingApprover
加 ready 信号,parked 后再送 interrupt)。

**回访项结论**:
- AGENTRUNNER_APPROVE=always footgun(S3 递延,4.2 后复审):**维持
  文档级,不改**。交互路径是 3.10 TTY resolver;EnvApprovals 专为
  非-TTY loop-mode,`=always` 是显式 opt-in(CI/自动化的自觉选择),
  与 --mode bypass 同类"你明确要求了才生效"。复审确认非对称收紧
  (project allow→ask)对**交互**用户仍有效,footgun 仅在显式
  auto-approve 下失效,可接受。

**Decisions**:
- interrupt 通道 send-once(缓冲 1)而非 close-once:steering 需可消费
  的单次事件,close 会让所有后续 activity 立即取消(closed channel 恒
  可读)。
- steering LLM 不把 interrupt 放进 conversation(source==interrupt 语义
  沿用 S2.14):CLI 无 steering 文本;带文本的 steering 属交互 WAIT_INPUT,
  留后续。

## S4.3 并行 tool call — DONE(预期返工 #2 落地)

**同一 assistant turn 的多个 allow 的 tool call 并发执行。** `decide()`
不再逐个返回 tool call,而是把当前 turn 全部未决 call 一次性返回
(`action.calls []provider.ToolCall`);单 call 是退化情形,无独立路径。

`Loop.doTools` 两阶段:
- **阶段一 串行裁决**:每个 call 顺序过 `adjudicate`——ask 就地 park
  在 resolver 上(一个 turn 的多个 ask 顺序审批,不抢弹窗);每个 allow
  的预算预留在下一个裁决读预算前已折进 fold。**裁决串行正是
  reserve-then-settle 在真并行下不超支的原因(3.7d TOCTOU 复验):不并行化
  裁决,就没有共享 fold 的读改写竞态。** deny 就地渲染 model-visible
  错误结果,不执行。
- **阶段二 并发执行**:allow 的 call 各起一个 goroutine 跑 `exec.Do`。
  **fold 单线程,所以并发的 journal 写全部过单一 mutex 串行化的 appendE
  ——这是 S4.3 的核心不变量。** 因此 ActivityCompleted 按**到达序**落盘;
  assembly 从 fold 的 ToolResults(map,call_id 键)按 assistant message
  的 tool_call **原序**重排回填。一个 `interruptScope` 覆盖整批:steering
  中断取消整批,每个被取消的 call 已在 fold 渲染 `[interrupted by user]`,
  中断本身在批次 join 后 journal 一次。

**验证**(`parallel_test.go`):
- `TestParallelToolCalls`:三个 `sleep 0.3` bash 并发跑完 <0.7s(串行需
  ~0.9s),三个结果都进 fold 且非错误——墙钟即并发证明。
- `TestParallelToolArrivalOrder`:发起序 c1/c2/c3,完成序 c2/c3/c1
  (0.1/0.3/0.5s 错开);断言 journal 的 ActivityCompleted 顺序 = 完成序
  (到达序),而 `Assemble` 回填的 tool_result 顺序 = 发起序(call_id 键
  map 不受落盘序扰动)。
- `TestParallelToolBudgetNoOverspend`:一个 turn 三个 execute call(各
  2000),预算 5000——串行裁决使 b1+b2(4000)通过、b3(将达 6100)被拒;
  逐事件 replay 断言 settled+reserved 全程 ≤5000。3.7d 的合成并发测试
  (mutex 模拟)在此升级为真 goroutine 并行。

**Decisions**:
- **统一路径,不留单 call 快路径**:N=1 走同一 goroutine+mutex 批处理
  (mutex 无争用)。理由:并发机制被最常见的 N=1 路径充分覆盖,而非仅在
  罕见多 call turn 才走到;两条路径的维护成本与漏测风险更大。
- **裁决串行、执行并行**(而非全程并行):预算预留在裁决阶段发生,串行
  裁决让 reserve-then-settle 无 TOCTOU;执行阶段无预算门,可安全并行。
  符合执行包"ask 本身串行审批"且把 3.7d 不变量落到"裁决不并行"。
- **crash matrix 改一 turn 一 tool**:原 matrix 的 read+edit 同 turn 在
  S4.3 下并发,race 了 `after_exec_before_journal` 的每活动计数器(两
  goroutine 命中序不定),且非幂等 edit 在 read 崩溃点变 in-doubt。为保
  matrix 作为**确定性顺序**崩溃门,改为 read(t1)/edit(t2)/done(t3)
  一 turn 一 tool;并发多 tool 行为由 `TestParallelToolCalls` 单独覆盖。
- **golden fixture read/edit 改指不同文件**:`TestLoopRequestAssemblyGolden`
  原 read 与 edit 同文件,并发下 read 结果 race edit(可能读到改后
  内容),golden 不确定;改为 read `notes.txt`、edit `greet.txt`,拼装
  形状(两 call 两 result、角色与序)覆盖不减而结果确定。
- **同 turn 同资源的 tool call 竞态属模型责任**:模型同时发起 read+edit
  同一文件即声明二者独立;harness 并发执行(与主流 agent 一致),不做
  隐式排序。

## S4.4c caching — DONE

**两部分:计费口径 + prefix 字节稳定。**

- **计费口径 `input + output − cache_read`**:`provider.Usage.Billed()`
  成为预算计费单一真相(clamp 于 0,cache_read 多于 input 时不倒贴预算);
  `budgetView.SettledTokens` 与 LimitExceeded 的 used 均改用 `Billed()`。
  raw Input/Output 仍供 CLI 展示与遥测。scripted/record 的 UsageEvent 补
  `cache_read_tokens` 字段以便 fixture 驱动缓存场景。
  验证 `TestBudgetBillsCacheReadDiscount`:raw 1100 会超 1000 预算,但
  800 为 cache_read → billed 300,run 正常完成;`TestUsageBilledClamp`。
- **env 块 session-start 冻结(DESIGN §context-assembly 不变量)**:cwd +
  date 在 session start 渲染成 `<env>…</env>` 冻进 `RunStarted.Env` →
  fold 进 `Run.Env`;`Assemble` 按 DESIGN 固定序把它放 system prompt 最前
  (env → spec prompt → mode suffix)。date 用 Clock.Now() 冻结,故同日多
  turn 的 prefix 逐字节稳定(prompt caching 经济性所系)。
  验证 `TestRenderEnvBlockDeterministic`(同日不同时刻不变、空 cwd 空块)、
  `TestAssemblyPrefixByteStable`(跨 turn conversation 增长而 system 前缀
  字节相同)。golden 用 FakeClock 固定日期 + normalizeRequests 归一化
  `<env>…</env>`(cwd=tempdir 属环境特定)。

**Decisions**:
- **`Run.Env` 加字段不 bump run 版本**:sub-state 版本规则原文是"shape
  changes **incompatibly** 才 bump"。`Env`(omitempty)是加性可选字段——
  旧 snapshot 无该字段 fold 成 `Env=""`(恰为零值),旧 binary 丢弃未知
  字段,双向兼容,非不兼容变更,故不 bump。保持已有 8 个 sub-state 均
  version 1。
- **env 块只放会话稳定项(cwd+date),不含 git 状态**:DESIGN 举例含 git
  状态,但 git 每 turn 变、需冻结才稳定;当前 workspace 抽象无 git seam,
  贸然跑 git 既 scope creep 又对非 git 目录脆弱。cwd+date 已锁住 DESIGN
  真正在意的不变量(volatile 数据 session-start 冻结);git 状态待
  workspace 长出 git 接缝再补(追加消息进上下文,不改 prefix)。
- **env 块置于 system 最前、mode suffix 殿后**:DESIGN 序为 harness base
  → env → memory → tool dirs → spec prompt;harness base / memory / tool
  dirs 尚未落地,当前实为 env → spec prompt → mode suffix。mode suffix 仅
  在显式 mode 跃迁时变(决策 #10 接受的 cache 断裂),放最后使 env+spec
  前缀最大化稳定。

## S4.4d signature 往返 — DONE(基础设施 S1 已备,本步补验证)

**opaque provider payload(Gemini thoughtSignature)逐字节往返。** 链路 S1
起即备齐:`Part.Extras map[string]json.RawMessage` → `CollectTurnStreaming`
把 `ToolCall.Extras` 拷进 assistant message 的 tool_call part →
`AssistantMessage` event 持久化(JSON round-trip)→ fold 存入
`Conversation.Messages` → `assembleMessages` 原样 append 整条 assistant
message,故下一 turn 请求里 tool_call part 的 Extras 逐字节回传。harness
从不解析或再生成签名。

`toolCallsOf`(decide/adjudicate/execute 用)丢弃 Extras 属正确:签名只在
assembly 回传时需要,而 assembly 读整条 message,不经 toolCallsOf。

验证 `TestSignatureRoundTrip`:turn1 tool_call 带 Extras{thought_signature},
断言(a)turn2 assembled 请求的 tool_call part Extras 与原值 `bytes.Equal`,
(b)assistant_message event 持久化的 Extras 亦逐字节一致。

**Decisions**:
- 本步无生产代码改动——4d 是"接口按最终形态设计(1.2)"承诺的兑现点:
  S1 就把 Extras/Signature 落位,S4 只需驱动多轮测试证明不变量成立。

## S4.5 compaction — DONE

**ContextCompacted 作为 recorded activity 改变 fold 视图 + 跨边界 fold 等价。**

- **event**:`ContextCompacted{upto_turn, summary, dropped_turns, summary_ref}`
  ——S4 内联 summary,summary_ref 预留 ArtifactStore(计划原文)。注册进
  Registry + 加进 round-trip sample。
- **新 sub-state `compaction`(version 1,2.4 声明的 S4 新增)**:
  `Compaction{Summary, Boundary, UptoTurn}`。fold ContextCompacted 时
  `Boundary = len(当前 messages)`(冻结当下消息数),full message log 保持
  完整(log 是 truth),latest compaction wins(二次压缩的 summary 已含前
  一次,boundary 前推)。
- **assembly 视图**:`assembleMessages` 在 Boundary>0 时以单条 summary user
  message 取代 messages[0:Boundary],其后消息照常拼装。
- **recorded activity `compactContext`**:以当前(可能已压缩的)视图为输入
  跑 summarizer LLM(harness-owned system prompt),idempotent,产出
  ContextCompacted。**不过 permission pipeline**(harness 内部维护调用,
  非模型指令),但其 usage 结算进预算。
- **触发**:turn 边界(doTurn 且 turn>1),`compactionDue` 纯判定——
  `estimateContextTokens(assembled view) > spec.model.compact_at_tokens`
  且 messages 超出 Boundary+1(保证每次压缩吃掉≥2 条新消息、不逐 turn 抖)。
  基于**已压缩视图**估算 → 新 summary 使估算落回阈值下,压缩自终止。

**验证**:`TestCompactionFoldView`(边界后视图=summary+边界后消息,前缀内容
不泄漏)、`TestCompactionFoldEquivalence`(在每个 seq 切开 prefix→snapshot→
tail,跨 ContextCompacted 边界 fold 等价)、`TestCompactionTriggeredInLoop`
(低阈值 + 大 turn1 → ContextCompacted 落盘、turn2 请求带 summary 不带原文)。
另修 `statetest.AssertFoldEqual`:补齐 effects/mode/budget/compaction 四个
此前**漏比**的 sub-state(等价断言此前静默不覆盖它们)。

**Decisions**:
- **触发阈值用绝对 `compact_at_tokens` 而非 DESIGN 的 trigger_ratio×window**:
  ratio 需 per-model context window,尚未建模;v0 用估算 token 绝对阈值,
  记为 trigger_ratio 的占位简化。估算用 bytes/4 粗口径(provider 无关,
  压缩触发只需数量级信号)。
- **compaction 不过 permission/budget pipeline**:它是 harness 内部维护
  调用,模型从未指令;usage 仍结算进预算(压缩耗 token 计费),但不被
  budget gate 拦(v0 简化,记档)。
- **compact activity idempotent**:崩溃在 Started 与 ContextCompacted 之间
  → resume 重跑 summarizer(非 in-doubt);ContextCompacted 仅在 activity
  完成后落盘,重跑收敛(代价:崩溃时 summarizer usage 轻微重复计,可接受)。
- **新 sub-state 使版本集长度 8→9**:checkVersions set-length 变化 → 旧
  session 不可 resume。属计划内(2.4 明列"S4 加 compaction 视图"),原型
  无持久 session,可接受。

## S4.6 finish reason 策略 — DONE

**异常 finish reason 的 loop 策略。** `provider.FinishReason` 加两枚:
`malformed_tool_call`、`blocked`。

- **malformed_tool_call → MalformedToolCall event + 有界重试**:LLM 调用
  完成但 Finish==malformed → 落 `MalformedToolCall{turn, raw, error}`
  (raw 取累积 assistant text)、发 KindDiscard(复用 discard 信号)、
  **不落 AssistantMessage** → `continue` 使 decide 见 turn 无 assistant msg
  → 重跑同 turn。durable 计数 `Run.MalformedRetries`(fold:
  MalformedToolCall++,TurnStarted/AssistantMessage 归零)超
  `maxMalformedRetries=2`(即第 3 次)→ 发 KindError、epilogue 收尾
  reason `malformed_tool_call`。
- **blocked(safety)→ 用户可见 error 收尾**:Finish==blocked 或 other →
  先落 AssistantMessage 保留已有文本 → 发 KindError → epilogue reason
  `blocked`。DESIGN/PLAN 口径:provider 把 safety/blocked 映射到
  FinishOther/blocked,S4 上浮为用户可见 error。
- **空 candidate**:无 text 无 tool call 的 end_turn → assistant message 空
  → decide 见无 tool call → doEnd(completed),干净收尾不空转。

**验证**(`finish_test.go`):malformed 重试后成功(1 个 event、completed)、
malformed 耗尽(3 个 event、reason malformed_tool_call、1 个 KindError、
末事件 run_ended)、blocked 收尾(reason blocked、KindError + 保留 message)、
空 candidate 单 turn completed。

**Decisions**:
- **malformed 重试在 loop 层而非 activity retry**:malformed 不是 provider
  error(流成功完成、Finish 标注),故不能走 ActivityExecutor 的 class-based
  retry;在 doLLM 收到 turn 后判 Finish,不落 assistant message + continue
  即天然重跑,durable 计数保证有界且跨 resume(resume 后计数从 fold 恢复)。
- **FinishOther 与 FinishBlocked 同等上浮**:PLAN 明言 safety/blocked 用
  既有 FinishOther 表示;为让线上 provider 可显式标 blocked,加 FinishBlocked
  枚举,二者都触发用户可见 error 收尾。
- **Run.MalformedRetries 加性字段不 bump 版本**:与 Env 同理,omitempty
  加性可选,双向兼容。

## S4.7 Anthropic provider + capabilities 矩阵 — DONE

**第二 provider 实现 + capabilities 抽象验证。**

- **`internal/provider/anthropic`(anthropic-sdk-go v1.56.0)**:New(从
  ANTHROPIC_API_KEY)、Capabilities、Complete(streaming)。Complete 用
  SDK 的 `Message.Accumulate` 累积完整消息:response text 逐 delta 实时
  流出(thinking delta 不外露——内部推理),流闭合后从累积消息派生
  tool call / usage / finish。转换:normalized Message/Part ↔ Anthropic
  blocks(tool_result 进 user 角色;assistant 按 thinking→text→tool_use
  固定序,API 校验此序)。
- **thinking 签名往返**:Anthropic thinking block(text+signature)按
  签名校验内容,故必须逐字节回放。捕获进 assistant tool_call 的
  `Extras["anthropic.thinking"]={thinking,signature}`,回传时在 tool_use
  前 `NewThinkingBlock(sig,thinking)` 复原。
- **capabilities 矩阵**:`Capabilities{Thinking, PromptCaching, ParallelTools}`
  三旗标(此前空 struct);gemini/anthropic 均声明三者 true。
  `internal/provider/capabilities_matrix_test.go`(package provider_test)
  跨 provider 断言矩阵——零值 Provider 即可(Capabilities 静态,无需 client)。
- **thinking 进 spec.model + 显式降级**:`ModelSpec.Thinking{Enabled,
  BudgetTokens}` → `CompleteRequest.Thinking` → Assemble 注入。各 provider
  自映射(gemini `ThinkingConfig`、anthropic `ThinkingConfigParamOfEnabled`,
  budget floor 1024);**provider !Capabilities.Thinking 时 loop 显式降级**
  (drive 起始 slog.Warn 一次 + doLLM 清空 req.Thinking),非静默。
- **per-provider error 映射**:anthropic `*sdk.Error.StatusCode` →
  `errs.FromHTTPStatus`(3.9 归一化的线上侧);transport 层归 ProviderServer
  (可重试)。cache_control:system block 打 ephemeral 断点(4c prefix
  稳定使其生效)。
- **CLI 接线**:provider factory 加 `anthropic` 分支。

**验证**:anthropic 单测(capabilities、mapFinish、classify 状态映射、
toParams thinking+cache、toTools schema、thinking 往返序、emitAccumulated
tool call 带 thinking、坏 part 拒绝、转换错误经 stream 上抛);capabilities
矩阵;loop thinking 降级(supported 透传 / unsupported 清空);live 冒烟
(pong + 双 turn tool+thinking 往返,`-tags live`,无 key 则 skip)。

**Decisions**:
- **thinking delta 不作为 text 外露**:内部推理不污染 response 文本;
  签名往返从累积消息取,不需流式外露。summarized thinking 的用户可见
  呈现留后续。
- **Anthropic tool_use CallID 用 provider 原生 block.ID**(id-based 配对,
  与 Gemini 的 positional call_<turn>_<idx> 相对);send-back 的 tool_result
  引用同 id,天然一致。
- **降级在 loop 而非 Assemble**:Assemble 保持纯 + 无 caps 依赖(签名不动、
  测试不改);loop 查一次 caps,doLLM 清 req.Thinking。both provider 均支持
  thinking,降级路径靠 scripted(caps 全 false)与 caps 覆盖 wrapper 测试。
- **新增依赖 anthropic-sdk-go v1.56.0**(go.mod/go.sum);go mod tidy 干净。

## S4.8 inspect v0 — DONE

**`agentrunner inspect <session> [--json]`——events 命令的人读进化。** events
dump 原始日志,inspect 按人理解 run 的方式渲染:

- **头部**:spec / model / mode / status(带 reason)/ turns。
- **timeline**:按 turn 分组,每条 activity 一行——`kind name call_id
  verdict[gate] tokens`。llm/compact/tool 三类(按 ActivityID 前缀 llm-t /
  compact-t / 其余判)。
- **每 call 判定**:从 EffectResolved 建索引(tool 按 CallID、llm 按
  eff-llm-tN),`decidingGate` 取产生该 verdict 的 gate(deny 取首个 deny
  gate,否则末 gate)+ reason。
- **token/cost/cache 列**:per-call 从 ActivityCompleted.Usage 读
  input/output/cache_read;汇总从 fold 的 Run.Usage(input/output/
  cache_read/write + Billed())+ Budget.ReservedTotal()。
- `--json` 输出结构化 inspectReport。CLI dispatch + usage 加 inspect。

**实现要点**:ActivityCompleted 无 name/call_id(在 ActivityStarted 上),
故走一遍把 ActivityStarted 按 ActivityID 建索引,ActivityCompleted 查回
name/callID。纯函数 `buildInspectReport(events, state)` 便于单测。

**验证**:`TestBuildInspectReport`(craft 事件日志→llm allow+tokens、tool
deny+permission gate、billed=input+output−cache_read)、`TestRenderInspect`
(timeline/turn/billed/reason 出现在人读输出)。

**Decisions**:
- **无 $ cost 列,用 billed token**:代码库无 per-model 价目表(预算用
  token-equivalent),故 "cost" 呈现为 billed token(input+output−cache_read,
  4c 口径);真实 $ 定价留后续价目表。

## Stage 4 出口对抗式 review — 三镜头(correctness/concurrency · security · contract/DESIGN)

三个并行 reviewer 覆盖 S4 全量 diff(3ef3578..HEAD)。核心不变量确认成立:
S4.3 并行 tool call 无 data race(所有并发 journal 写过单一 mutex 化
serialAppend,ds.s/ds.lastID 读改写全在临界区,ds.s 仅 wg.Wait() 后读);
S4.5 compaction 自终止且崩溃安全;S4.6 malformed 有界(第 3 次逃逸,无
off-by-one);sub-state 版本 8→9 + checkVersions 正确;statetest.AssertFoldEqual
覆盖全 9 个 sub-state;capability 降级在 doLLM 与 compactContext 两条装配路径
均不漏;assembly 序与 prefix 稳定成立;protocol 用户/模型可见错误分道。

**修复(3 项)**:
- **[P1] Anthropic Usage.Billed() 少记预算**(correctness + contract 双报):
  `Billed()=input+output−cache_read` 依 Gemini 口径(PromptTokenCount 含
  cached)。Anthropic `input_tokens` **不含** cache_read/cache_creation,原
  adapter 逐字透传 → 双重折扣,暖 cache run 计费趋 0、可能永不触
  LimitExceeded。修:adapter 把 InputTokens 归一为**总输入**(+
  CacheReadInputTokens + CacheCreationInputTokens),与 Gemini 口径一致;
  Billed 变 uncached+creation+output(cache_read 折扣、creation 计费)。
  测试断言归一(input 18 / billed 19)。
- **[P1] recorder Extras 未脱敏**(security):`toEvent` 逐字拷 tool call
  Extras,而 Anthropic adapter 往 `Extras["anthropic.thinking"]` 塞模型
  完整推理文本(可回显读到的凭据),经 WriteFixture 写入**提交的 fixture**
  = credential-to-repo。修:新增 `redactExtras`,每个 Extras 值过
  redactString。主 journal 路径不受影响(appender 整 payload text-redact)。
- **[P2] Anthropic pause_turn → end_turn 静默截断**(latent):pause_turn
  语义是"续跑本 turn",映射到 end_turn 会让无 tool call 的暂停响应被
  decide 判为 completed 提前结束。修:pause_turn 归 FinishOther → loop 上浮
  为用户可见 error,不静默截断。

**记档为 v0 已知限制(不改)**:
- **[P2 latent] Anthropic interleaved 多 thinking block**:`assistantBlocks`
  的 `len(thinking)==0` guard 使回放只重发首个 thinking block。仅在
  interleaved-thinking beta header 下才有多 thinking block,当前 adapter 不
  启用该 beta(响应恒单前导 thinking block),故不触发。启用该 beta 属后续。
- **[LOW] blocked finish 非持久**:blocked/other 收尾先落 assistant_message
  再走 epilogue(reason "blocked"),二者间崩溃则 resume 把无 tool call 的
  assistant message 判为 completed,丢失 blocked reason 与 KindError。窗口
  窄、run 仍终止;与 malformed(有持久 MalformedToolCall event)不对称。
  彻底修需新持久 event 或 fold 标记,代价不成比例,v0 记档接受。

**Stage 4 正式关闭**。下一步:S5 kickoff refinement。

## S5 kickoff refinement — DONE

PLAN.md 新增 **S5 执行包**:细化生态与多 agent 的 9 模块序列。四条跨切
硬线:①权限交集冻结绝不上扬(mode 不交集)②树预算 min 聚合不可击穿
③artifact blob 先于 event(镜像 journal fsync-先于-ack,ref 永不悬空)
④prefix 稳定 under skill/子 agent/memory 目录注入(session start 冻结)。

关键决定:MCP 生命周期带外(仅 schema 入 event,resume 带外重连+对账);
子 agent = fresh child run 的 activity(prefix 稳定、故障隔离);审批沿
correlation 冒泡(预期返工:跨 actor waiting 路由);ArtifactStore 复用
SnapshotStore 的 CAS 模式;outputs contract 填实 2.16 epilogue auto-publish
槽位(缺 required → parent error);payload_ref 兑现(plan 存 artifact、审批
带 ref);**S5 不新增 fold sub-state**(骑既有 Activities+correlation,tasks
是 S6)。acceptance:s5_fleet / s5_plan_approval / s5_no_escalation(否定)/
s5_budget_seal(否定)。

三文档一致性:S5 pack 仅细化 PLAN,引用既有 DESIGN 不变量(multi-agent
即 actor、prefix 稳定、CAS 复用、权限非上扬),与 STAGES Stage 5 范围一致,
与 2.4 表"S6 加 tasks、S5 无新 sub-state"一致,未动 DESIGN 不变量。

下一步:S5.1 MCP client(官方 go-sdk、生命周期带外、schema 入 event、
mcp__server__tool 命名、无标签 execute-class、allowed_tools 收窄+否定测试)。

## S5.1 MCP client — 进行中(part 1:client wrapper 落地)

新增 `internal/mcp`(官方 `modelcontextprotocol/go-sdk` v1.6.1):

- **生命周期带外**:`Conn` 包一个已连接的 `*sdk.ClientSession`(连接
  transport——stdio/in-memory——是调用方职责,不进 event log)。`clientSession`
  接口便于测试替身。
- **发现 + 归一**:`Discover` → ListTools → `DiscoveredTool{Server, Tool,
  Name(mcp__srv__tool), Description, InputSchema, Class}`,按 Name 排序(稳定
  tool face → 稳定 prefix)。
- **class 映射**:ReadOnlyHint → read;**无标签 → execute**(最保守默认)。
- **命名**:`QualifiedName`/`SplitName`(只首个 `__` 分割,tool 名可含 `__`)。
- **Call 分发**:`Conn.Call` 渲染 content(TextContent 拼接)+ 透传 MCP
  tool-level IsError(失败是 model-visible 结果,与 built-in 同契约)。
- **Manager**:多 server 联合发现 + **allowed_tools 收窄**;`Call` 对未列
  工具**防御性拒绝**(即便模型伪造调用未 advertise 的 MCP tool 也不执行)。
  重复 server 名拒绝(命名 namespace 工具,冲突则分发歧义)。

**验证**(in-memory MCP server,双工具 peek[read-only]/run[untagged]):
命名+class 默认、Call 分发+IsError 透传、Manager allowed 收窄+越权 Call
拒绝(否定测试)、SplitName 边界、重复 server 拒绝。

**S5.1 剩余(下一步)**:①discovered schema **入 event**(resume 知 tool
face + 带外重连对账);②MCP tool 接入 loop 的 advertised 面 + permission
class + assembly(mcp__ 工具进 tool registry/ProviderDefs);③resume 时按
journaled schema 带外重连+漂移检测。本步先落纯 client 抽象(与 agent 内核
解耦,可单测),集成留后续步。

新增依赖:modelcontextprotocol/go-sdk v1.6.1(+ jsonschema-go、uritemplate、
oauth2、segmentio/encoding 传递依赖);go mod tidy 干净。

## S5.1 MCP client — DONE(part 2:schema 入 event + loop 集成 + resume 对账)

- **`ToolsDiscovered{server, tools[]}` event(S5 首个新 event)**:
  `MCPToolDef{Server, Name, Description, Class, InputSchema}`。Registry +
  round-trip sample 齐。fold 进 `Run.MCPTools`(加性 omitempty 字段,同 Env
  先例不 bump 版本;**未新增 sub-state**,信守 S5 执行包):per-server 替换、
  全脸按 Name 排序(稳定 face → 稳定 prompt)。
- **发现在 Run() journaled**:`discoverMCP`——SetAllowed(spec.allowed_tools)
  → Discover → 逐 server 落 ToolsDiscovered。连接本身仍带外。
- **loop 集成**:toolDefs = builtin + **fold 的** MCPTools(resume 拿到与原
  run 逐字节相同的 face,无需再协商);`toolClassIn/toolIdempotentIn/
  toolTimeoutIn`(state-aware,取代旧 free functions)——mcp read(源自
  ReadOnlyHint)幂等、其余不幂等;**所有 mcp call 均给 execute 墙钟**(跨
  进程边界,read 也可能挂死);`advertisedTools` 过 fold 取 class(mode 过滤
  与 builtin 同款:plan mode 隐藏 execute-class MCP tool,测试锁定);
  `buildToolRun` 按 `mcp__` 前缀分发到 Manager.Call(tool-level IsError =
  model-visible,transport error = activity failure 走 retry/final 渲染)。
- **S4.3 并发不变量守护**:Activity 配置读 ds.s,原先在 goroutine 内构造
  会与 serialAppend 的 ds.s 突变竞争——改为**主 goroutine 先构造全部
  Activity**、goroutine 只执行(race 在写测试时发现并修)。
- **resume 对账**:`reconcileMCP`——journaled face 是本 run 的真相;缺
  tool / class 漂移 / schema 漂移(compact JSON 比对)/ 无 manager 均**拒绝
  resume**(2.13 版本纪律同款,绝不静默吸收)。
- **spec `allowed_tools`**:AgentSpec 新字段(fully-qualified mcp 名单,
  builtin 不受影响);缩窄作用于 advertise + journal + Call 三层。
- **MCPManager 接口**(SetAllowed/Discover/Call)+ 编译期断言
  `*mcp.Manager` 实现之;测试用 fakeMCP。

**验证**:e2e(发现落盘、face 并入 advertise、mcp call 分发、结果入 fold)、
allowed_tools 否定(不 advertise、不入 journal、伪造调用被拒且 run 继续)、
plan mode face 过滤、resume 四例(匹配通过 / schema 漂移拒 / 缺 tool 拒 /
无 manager 拒)。全量 check + race 绿。

**Decisions**:
- **face 从 fold 读而非 live manager**:resume 语义要求 face 与原 run 一致;
  live 只用于执行与对账。
- **typed-nil 陷阱**:MCPManager 参数必须以接口传递(*fakeMCP nil 指针装进
  接口非 nil),测试注释记档。
- **CLI 的真实 stdio server 接线留后续**(spec 声明 mcp server 命令 + 启动
  transport 属配置层,harness 接缝已完备可测)。

## S5.2 skills + memory 文件 — DONE

- **`internal/skill`**:Claude Code 约定发现(`.claude/skills/<name>/SKILL.md`
  + YAML frontmatter{name, description});**目录只进 name/description/相对
  path,body 绝不进 prefix**(按需加载:模型经 read_file 读 body——不加新
  tool 的 v0 兑现)。name 缺省回落目录名;malformed skill 跳过并列名报错
  (不阻断 run,loop 层降级为 warn);目录块 `<skills>` 按名排序 byte-stable。
- **`internal/memory`**:CLAUDE.md 层级合并——从 workspace root 向上走到
  **git root(含)**收集,**无 enclosing repo 则只取 workspace 自身**(不
  吸上层无关目录);渲染 outermost first、**近者最后**(近者优先语义:后
  出现覆盖前面),`<memory>` 块每段标注相对路径。
- **冻结与注入**:RunStarted 加 `Memory`/`Skills`(与 Env 同生命周期,加性
  omitempty 不 bump 版本)→ fold 进 Run → `assembleSystem` 固定序:
  **env → memory → skills → spec prompt → mode suffix**(DESIGN §context
  序;harness base 与 tool/子 agent 目录段留后续模块)。中途改文件不扰
  prefix(session start 冻结)。经 appendE 落盘 → 全量 redaction 覆盖
  (memory/skill 文本里的凭据被脱敏)。

**验证**:skill(发现+排序+name 回落+相对路径、body 不进目录、无目录
nil、malformed 跳过存好)、memory(三层级顺序、near-last 渲染、无 repo 停
在 root)、assembly(五段固定序)、e2e(CLAUDE.md+skill 冻进 RunStarted 入
prefix、body 不漏、**中途 bash 改 CLAUDE.md 后 turn2 prefix 逐字节不变**)。

**Decisions**:
- **按需加载 v0 = read_file 路径引用**,不做专用 skill tool:目录携相对
  path,模型自读;避免新 tool face 扰动。
- **user-level skills(~/.claude/skills)与 user CLAUDE.md 留后续**:三源
  merge 骨架在 3.4,本步只做 project 层(workspace + 祖先);user 层注入
  属加性扩展,记档。
- **memory 无大小上限(v0)**:prefix 膨胀风险记档,S5 出口 review 复查。
