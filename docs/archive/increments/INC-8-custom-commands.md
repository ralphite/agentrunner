# INC-8 自定义命令 / slash 面（G21）

## 动机与 journey 锚
GAPS G21（设计欠定）；UJ-19 步骤2「自定义命令 `/deploy-check` 一键跑检查单」。
对标 Codex 的 slash 命令 / prompt 宏。

## Spec / Design delta
- SPEC H 行 `自定义命令 / slash 面` ❌→✅。
- DESIGN §10 新增「自定义命令」子节：定义位 `<root>/.claude/commands/<name>.md`；
  展开 = 注入 prompt 文本、ingest 时展开（fold 纯、resume 自包含）；与 skills
  边界 = 命令对模型不可见（用户宏）vs skills 模型侧能力；信任 = 决策 #19
  同类（用户显式 invoke + 只注入文本，无额外门）。**不触不变量**（命令不进
  prefix，不涉 prefix 稳定性）。

## 实施
- `internal/command`（mirror internal/skill）：`Discover` + `Expand(root,text)→(str,ok)`。
  name 限 `[A-Za-z0-9_-]+`（杜绝穿越）；`$ARGUMENTS` 替换/无占位则追加；
  未知/非 slash 原样透传；strip frontmatter。
- 两处 ingest 入口接展开：`Loop.Run` 开场 task（展开后 re-redact）+
  `conversation.journalInput` 每条 send。

## 验收
- A 孪生 command_test.go（$ARGUMENTS/追加/未知透传/穿越拒绝/frontmatter strip/
  前导空白/Discover 排序+basename+跳非法）全绿。
- B 真实 API：`.claude/commands/locate.md` 的 `/locate authMiddleware` 在
  new+send 两路都展开进 journal 的 input_received（验证 fold 纯 + 双入口）。

## review 裁决
小增量（S），inline 自审（穿越/透传/redaction 顺序/双入口）。裁三视角。
