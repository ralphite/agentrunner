# INC-20 skill 模型侧 invoke（核心，#45/§3.5）

## 动机与 journey 锚

CLAUDECODE-PARITY §2.05 #45 / §3.5 + UJ-19。现状 skills 是**读侧注入**：
目录（name+description+path）进 prompt prefix，body 由模型用 `read_file`
按需读（S5.2）。Claude Code 让 skill 成为**模型可调工具**（按 name
invoke，注入 SKILL.md 指令）。本增量补这半边——一个 `skill` 工具，模型
`skill(name:"deploy")` 直接拿到该 skill 的完整指令，无需知道文件路径。
维持"命令=用户宏"裁决**不动**（命令对模型不可见、ingest 时展开；skill
是模型侧能力，二者边界不变）。

## 范围（核心；fork 变体拆余项）

- **本增量（核心）**：`skill` 工具（read-class）——模型按 name 调用，
  返回该 skill 的 SKILL.md 正文（frontmatter 后）作为 tool result。等价
  于"read_file 那个 path"但按 name、更自然，且 skill 成为一等可调面。
- **余项 INC-20b（拆出）**：`context:fork`（skill 在一次性子 agent 里
  执行，= spawn_agent 一次性变体）——更大的活，独立增量。

## Spec delta

- SPEC H「skills」行增强：读侧注入 + **模型侧 invoke（`skill` 工具，按
  name 返回 SKILL.md 正文）**；tool 附录加 `skill`。锚
  `TestSkillTool*` + QA-29。
- CLAUDECODE-PARITY §2 #45 状态更新（invoke 核心 ✅，fork 余项）。

## Design delta（不触不变量）

DESIGN §10 Skills 补一段「模型侧 invoke（INC-20）」：`skill` 是 read-class
工具（无副作用、免审批同 read_file），执行 = 定位 `<ws>/.claude/skills/
<name>/SKILL.md`（WS.Resolve 边界内，name 校验拒 `/`/`..` 防遍历），返回
其正文（去 frontmatter）。目录注入仍是发现面（模型知道有哪些 skill），
invoke 是获取指令面。命令=用户宏、skill=模型侧能力的边界不变（决策
不动）。

## 验收

- 孪生（tool 包）：`TestSkillToolReturnsBody`（skill(name) → SKILL.md
  正文，去 frontmatter）/`TestSkillToolUnknownName`（未知 name → error
  result，非 panic）/`TestSkillToolPathTraversalRefused`（name 含
  `../` → 拒绝，不逃逸 skills 目录）。
- 真实 API QA-29：workspace 放一个 skill，spec 带 `skill` 工具，让模型
  调 `skill(name)` 并按其指令行动，验证 tool_result 含正文；`ar events`
  归档 qa/runs/。
- `./scripts/check.sh` 全绿（绿门排除已知环境测试）。

## 实施步骤

1. `internal/tool/defs/skill.json`（read-class，参数 name）。
2. exec.go 加 `case "skill"` + `skill(args)` 方法（复用 skill 目录约定 +
   WS.Resolve 边界 + 去 frontmatter）。
3. 孪生 + QA-29。
4. 文档行齐活（SPEC/CLAUDECODE-PARITY/DESIGN/LOG/SPRINT）。

## review 裁决

做。核心是 S（一个 def + 一个 read-class exec 方法），fork 变体拆余项。
inline 自审：correctness（去 frontmatter、未知 name 报错）、security
（read-class 免审批但 WS 边界 + name 防遍历，不读 skills 目录外）、
contract（不改 skills 读侧注入、不动命令=用户宏裁决）。
