# INC-35 provider-native 结构化输出（spec output_schema，#91 余项，SPRINT #8b）

## 动机与关键约束

INC-26 拆出的 #8b。目标:让 provider **约束生成**直接产合规 JSON,免
INC-26 的 CLI re-prompt。

**关键工程发现（收窄范围的根据）**:gemini 的
`responseMimeType=application/json`+`responseSchema` 与
`FunctionDeclarations`（tools）**互斥**——JSON mode 强制整轮输出为一个
JSON 值,不能同轮 tool_call。故 provider-native 只能作用于**无 tools 的
轮**。且 `ar new --json-schema` 端到端需改 `daemon.Command`,而 HANDA 2U
正重做 daemon-dispatch（entanglement）。

**据此收窄为干净自包含切片**:走 **spec 级 `output_schema`**（不碰
daemon.Command）,且**仅在该轮 tools 为空时**下传 schema（有 tools 则
忽略,安全;INC-26 的 CLI 校验+重试对任意 run 仍是通用保底）。

## 范围（additive,不改既有语义）

- `provider.CompleteRequest.ResponseSchema json.RawMessage`（新可选字段）。
- `provider.Capabilities.StructuredOutput bool`（新能力位）。
- gemini `Capabilities().StructuredOutput = true`;`toConfig`:
  `req.ResponseSchema != "" && len(req.Tools)==0` → `ResponseMIMEType=
  "application/json"` + `ResponseJsonSchema=<unmarshaled any>`（genai 的
  raw-JSON schema 入口,同 tools 的 ParametersJsonSchema 路径）。有 tools
  则跳过（不能约束 tool 轮）。
- anthropic `StructuredOutput` 保持 false（无原生等价;downgrade）。
- `AgentSpec.OutputSchema json.RawMessage yaml:"output_schema"`。
- `Assemble` 设 `ResponseSchema: spec.OutputSchema`。
- loop caps-downgrade（同 Thinking 先例）:`if !caps.StructuredOutput {
  req.ResponseSchema = nil }`——无原生支持的 provider 静默降级,INC-26 CLI
  校验兜底,不静默"假装约束了"。

## Spec/PARITY delta

- SPEC J 结构化输出行:+ spec `output_schema` provider-native 约束
  （tool-less 轮;gemini 原生,anthropic downgrade）。
- CLAUDECODE-PARITY #91 备注:provider-native 余项 → ✅（spec output_schema,
  tool-less;`--json-schema` 端到端下传待 HANDA 2U 落定,拆 8c）。

## Design delta（不触不变量）

CompleteRequest/Capabilities 加字段,additive;downgrade 显式（决策"能力
缺失显式降级不静默"）。DESIGN §11 provider 能力段加 StructuredOutput 一
行。不动 journal/fold/pipeline/loop 语义。

## 验收

- 孪生:gemini adapter 单测 `TestToConfigResponseSchemaNoTools`（schema+无
  tools → ResponseMIMEType+ResponseJsonSchema 设）/`TestToConfigSchemaIgnoredWithTools`
  （schema+有 tools → 不设,tool 轮不破）;`TestGeminiCapabilitiesStructuredOutput`;
  agent `TestAssembleSetsOutputSchema` + `TestOutputSchemaDowngradedWhenUnsupported`
  （fake caps StructuredOutput=false → req.ResponseSchema 清空）。
- 真实 API QA-39（daemon-path→私有新二进制 daemon）:spec `output_schema`
  `{name,lines}` + tools:[]（纯抽取 agent）,让 gemini 抽取,验证**单轮
  无 re-prompt** 直接产合规 JSON（对比 QA-33 的 CLI 重试路径）。
- 绿门（排除已知环境测试）。

## 实施步骤

1. provider.go 加两字段。2. gemini toConfig+Capabilities。3. spec
   OutputSchema + Assemble + loop downgrade。4. 孪生 + QA-39。5. 文档。

## review 裁决

做。M 收窄为 S+（additive 字段 + gemini 映射 + spec/assembly 一处 +
downgrade 一处）。inline 自审:correctness（tool-less 门/downgrade 清空/
默认无 schema 零变化）、security（无新副作用,schema 只约束输出形状）、
contract（既有 provider/assembly 测试不触,新字段默认空;anthropic 走
downgrade）。`--json-schema` 端到端下传拆 8c（待 HANDA 2U）。
