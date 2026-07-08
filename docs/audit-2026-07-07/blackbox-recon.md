# 协调者本人冷启动 recon(黑盒,模拟零知识新用户)

产品当前 main(含我之前的 C1/C3/C4/C5/M8 修复)。全程只用工具自身输出,不读源码/文档。

## 确认的基础工作流问题

### [BB-me-1] `ar --help` / `ar -h` 报 "unknown command" 🔴
用户第一反应就是求助,但顶层 `--help`/`-h` 直接报错:
```
$ ar --help
agentrunner: unknown command "--help"
usage: agentrunner <run|drive|daemon|new|send|...20+ 命令...>
```
(子命令支持 --help,但顶层不支持——不一致且第一步就撞墙。)

### [BB-me-2] 无 README / 无"从这开始",顶层 usage 是 20+ 命令的裸列表 🟠
仓库根只有 CLAUDE.md(内部约定,非用户文档)。`ar` 无参数只吐一行 20+ 子命令,零解释、无"新手从 run 开始"之类引导。用户不知道该用哪个。

### [BB-me-3] spec 格式无从发现,报错泄漏内部类型名 🔴
要跑任何东西都得手写一个 YAML spec,但:
- 没有 `ar init` / 模板 / 示例命令(子命令列表里没有)。
- `run --help` 只列 flag,不说 spec 该含什么。
- 瞎写一个 spec →
```
$ ar run guess.yaml "..."
spec guess.yaml: yaml: unmarshal errors:
  line 1: field task not found in type agent.AgentSpec
```
泄漏内部 Go 类型 `agent.AgentSpec`,不给合法字段列表、不给示例。新用户彻底卡住:它到底要什么字段?

### [BB-me-4] 对话流的回复完全不显示——new/send 只给 id/"delivered" 🔴
```
$ ar new s.yaml "write a two-line poem about rain"
20260708-...-3d0c
session 20260708-...-3d0c (send: agentrunner send <sid> "...")     ← 诗在哪??
$ ar send <sid> "now say PING"
delivered                                                          ← AI 说 PING 了吗??
```
用户问了要诗,AI 也真写了,但 `new` 和 `send` 的输出里**都没有回复正文**。

### [BB-me-5] 读回复唯一的路是 `ar attach`,但它不可发现、help 不说明、还阻塞 🟠
- `ar attach <sid>` 其实能干净回放对话(显示 "BANANA"),但:
  - `new`/`send` 从不提示"用 attach 看回复"(只提示怎么继续 send)。
  - `attach --help` 只说 `-json emit the event stream as JSON lines`,**根本没说 attach 会显示对话**。"attach"这词也不像"看回复"。
  - 它是 live-follow,会阻塞,用户得知道去 Ctrl-C。
- 用户可能瞎试的替代:
  - `ar events <sid>` → 内部事件流(`effect_requested`/`waiting_entered`/`gate_results`/`verdict`),回复正文被**截断成 `…`**。
  - `ar inspect <sid>` → 只有 token 数和 budget 判定,**没有回复正文**。

### [BB-me-6] 深层不一致:run 显示输出,new/send 隐藏输出 🟠
```
$ ar run s.yaml "say hello"     → [gen-step 1] 你好！...  run completed   ← 输出可见、可读
$ ar new s.yaml "say hello"     → 只有一个 session id                    ← 输出消失
```
同一个产品两种相反行为。先用 run 觉得好用的人,一转到 new 做对话,输出就没了,会懵。

### [BB-me-7] daemon 是隐性前提,报错只暗示不指路 🟡
```
$ ar new s.yaml "..."   (没起 daemon)
agentrunner: daemon dial: ... connect: no such file or directory (is the daemon running?)
```
"is the daemon running?" 暗示有个 daemon,但不告诉用户:要先 `ar daemon` 启动、也不说 new/send 依赖它。新用户看到会想"什么 daemon?怎么起?"
