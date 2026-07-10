# INC-26 结构化输出（`ar new --json-schema`，#91）

## 动机与 journey 锚

CLAUDECODE-PARITY §2 #91：对标 Claude Code `--json-schema` → 校验最终
输出、失败重试、`structured_output` 字段。集成用途（脚本/CI 拿可解析
结果）+ verifier 用途（结构化裁决）两吃。现状 ❌：`ar` 无结构化输出面。

## 范围（干净形态：CLI 层编排，零核心 loop/provider 改动）

规避核心 loop 爆炸半径（INC-21 前例）。MVP 全在 CLI 层 + 一个纯包：

- 新纯包 `internal/structured`（zero 运行时依赖，纯函数）：
  - `Compile(raw []byte) (*Validator, error)`——unmarshal schema +
    `jsonschema.Schema.Resolve(nil)`（用已在依赖的 `github.com/google/
    jsonschema-go`）；坏 schema 早失败。
  - `Extract(answer string) (json.RawMessage, error)`——从模型答案抽 JSON：
    剥 ```json fences / 取首个平衡 {...} 或 [...]；答案夹叙述也能抽。
  - `(*Validator).Validate(raw []byte) error`——unmarshal 成 any +
    `Resolved.Validate`，返回可读错误。
- `ar new --json-schema <path> [--json-schema-max-retries N]`（默认 N=2）：
  - CLI 启动即 Compile schema（坏 → ExitUsage,不 spawn 幽灵会话）。
  - 前台跑,**捕获终答**（新增附加式 headless 捕获,**不改**已测的
    `followTurn`）。
  - Extract+Validate。合 → 打印规范 JSON（= structured_output）到 stdout,
    ExitOK。不合且有余次 → `send` 纠正消息（附校验错误,要求"只回一个
    验证通过的 JSON 值"）,重捕获重验。次数耗尽 → stderr 错误,非零退出。
  - `--json-schema` 与 `--detach` 互斥（需前台等终答）。

## Spec delta

- SPEC C（工具面/CLI）或 headless 段记 `ar new --json-schema`（校验+重试+
  structured_output）；锚 `internal/structured` 单元 + QA-33。
- CLAUDECODE-PARITY §2 #91 ❌→✅（headless 校验+重试；provider-native JSON
  mode 约束生成拆余项）。

## Design delta（不触不变量）

不改 DESIGN 不变量：不动 journal/fold/pipeline/loop/provider 接口。纯 CLI
编排 + 纯校验包。structured_output 是 CLI surface 概念,不入 durable 事件
（MVP）。provider-native JSON mode（gemini responseSchema 约束生成,免
re-prompt）与 durable structured_output 事件拆余项 #8b。

## 验收

- 孪生（纯包,快）：`TestStructuredCompile`（好/坏 schema）/
  `TestStructuredExtract`（fenced/raw/夹叙述/数组/无 JSON）/
  `TestStructuredValidate`（通过/类型错/缺 required,错误可读）。
- 真实 API QA-33：schema 要求 `{name:string, lines:integer}`；让 Gemini
  统计某文件返回结构化结果；校验通过打印 JSON；`ar events` 归档 qa/runs/。
  （真机若首答夹叙述,Extract 抽出;若不合 schema,重试纠正。）
- 绿门（排除已知环境测试）。

## 实施步骤

1. `internal/structured` 包 + 单元孪生。
2. `ar new --json-schema` flag + Compile 早校验 + headless 捕获终答 +
   validate/re-send 重试 + 打印 structured_output。
3. QA-33 真机。
4. 文档行齐活（SPEC/CLAUDECODE-PARITY/LOG/SPRINT）。

## review 裁决

做。干净 S（纯校验包 + CLI flag + 附加捕获函数,不改 followTurn/loop/
provider）。inline 自审：correctness（extract 边界/重试收敛/坏 schema 早
失败）、security（校验只读,无副作用；schema 来自本地文件）、contract
（--detach 互斥、既有 CLI 测试不触）。provider-native JSON mode 拆 #8b。
