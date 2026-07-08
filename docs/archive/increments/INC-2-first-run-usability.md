# INC-2 新手第一公里:黑盒 QA 基础工作流修复

## 动机与 journey 锚

2026-07-07 黑盒 QA(零知识新用户视角,findings 见当次 QA 会话
recon-me.md,BB-me-1…7)确认 7 个基础工作流硬伤,全部落在"新用户拿到
二进制后的第一公里":

- 🔴 BB-me-1 `ar --help`/`-h` 报 unknown command(子命令支持、顶层不支持)。
- 🟠 BB-me-2 无 README;顶层 usage 是 20+ 子命令裸列表,零解释。
- 🔴 BB-me-3 spec 格式无从发现:无 init/模板/示例命令;瞎写 spec 报
  `field task not found in type agent.AgentSpec`(泄漏内部 Go 类型名,
  不给字段清单、不给示例)。
- 🔴 BB-me-4 对话回复完全不显示:`ar new` 只输出 session id,`ar send`
  只输出 "delivered",AI 的回答两处都看不到。
- 🟠 BB-me-5 读回复唯一的路是 `ar attach`,但 new/send 从不提示它、
  `attach --help` 不说明它会显示对话、且阻塞需 Ctrl-C。
- 🟠 BB-me-6 深层不一致:`run` 清晰显示输出,`new`/`send` 全部隐藏。
- 🟡 BB-me-7 daemon 是隐性前提,报错只说 "is the daemon running?"
  不告诉用户怎么启动。

journey 锚:UJ-01(即问即答)、UJ-03(结对续聊)第 1 步"用户提问 →
agent 答完"在 CLI 对话形态下事实不成立——用户根本看不到回答。其余
条目伤所有 journey 共同的进入门槛(帮助、spec、daemon 可发现性)。

## Spec delta(SPEC.md)

- 新增:CLI `help`(顶层 `--help`/`-h`/`help`,分组、每命令一行说明、
  Quick start 段)。
- 新增:CLI `init [path]`(生成带注释的示例 spec;拒绝覆盖已存在文件)。
- 修订:`ar new`/`ar send` 默认**跟随本轮直到 idle** 并渲染回复正文
  (与 `run` 同一 textRenderer);`--detach` 恢复纯异步(只打 id /
  delivered)。输出尾行提示 `send`/`attach` 后续动作。
- 修订:spec 解析错误不泄漏内部类型名;未知字段时附合法字段清单与
  `agentrunner init` 指引。
- 修订:daemon dial 失败的报错附启动指引(`start it with: agentrunner
  daemon &`)。
- 新增:README.md(用户入门:构建、init、run、daemon、new/send/attach)。

## Design delta(DESIGN.md)

- daemon 线协议:`Command` 增加 `follow`(仅 send 使用)。
  `handleSend` 在 follow 时**先订阅 hub、后投递**(订阅先于 post,
  杜绝回复事件漏在订阅前),ack("delivered")照旧发出,随后把 hub
  事件持续转发直到客户端断开——detach 即断连,与 attach 同一语义
  (订阅不改结果)。旧客户端不受影响(不带 follow 走原路径)。
- `new` 零协议变更:daemon 的 run 命令本就持续流式;客户端从
  "SessionStart 即 detach"改为"渲染至首个 `idle` 事件再 detach"。
- 不触任何 §15 不变量(可丢 delta、journal-first、订阅不改结果均保持)。

## 验收

- scripted 孪生(双闸门之一):
  - cli:`--help`/`-h`/`help` 走 ExitOK 且含 Quick start;unknown
    command 提示 help;`init` 生成可通过 LoadSpec 的 spec、已存在时拒绝;
    spec 未知字段错误含字段清单与 init 提示、不含 `agent.AgentSpec`;
    `new` 默认打印回复正文与提示行;`send` 默认打印回复正文。
  - daemon:send follow 语义(先订阅后投递;delivered 后事件持续转发)。
- 真实 API 端到端(双闸门之二):以新用户路径手工走一遍——`--help` →
  `init` → `run` → `daemon` → `new`(看到回答)→ `send`(看到回答)→
  提示行指向 attach;daemon 未起时报错含启动指引。

## 实施步骤

1. 可发现性包:顶层 help + usage 重写 + `init` 子命令 + spec 错误
   友好化 + daemon 报错指引 + attach help 文案(一个提交)。
2. 回复可见性包:daemon `follow` + `new`/`send` 跟随渲染 + 提示行
   (一个提交)。
3. README + 三层文档收口 + LOG 条目(一个提交)。

## review 裁决

小增量(纯 CLI 呈现层 + 一个只读线协议字段),裁掉三视角对抗 review;
以 scripted 孪生 + 真实 API 端到端双闸门为准。
