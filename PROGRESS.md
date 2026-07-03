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
