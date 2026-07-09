# INC-4 远程 stop 命令（G12）

## 动机与 journey 锚
- **缺口**：GAPS **G12**（托管 run 远程控制面）——线协议有
  ping/run/drive/attach/approve/send/close/interrupt/kill/agent，**独缺
  stop**；interrupt 只绑"打断当前 turn"语义（裁决 #11，待命处 no-op），
  没有"优雅拆掉这个托管 run"的远程手势。
- **journey**：UJ-17 远程驾驶舱步骤 4「判断没救了 → 点 stop → 优雅取消」。
- **对标 Codex**：远程/云任务的 stop 是标配；我们 attach/审批/用量都有，
  独缺 stop。

## Spec delta
- SPEC I 行 `远程 stop command` ❌ → ✅（TestStop* · 手验）。
- SPEC 附录 daemon 线协议命令 + `stop`；CLI 子命令 + `stop`。

## Design delta
- **不触不变量**：stop 复用既有 plain-teardown 原语 `hostedRun.stopHosting()`
  （决策 #32 换 agent 用的同一机制）——ctx cancel，**无标记、无终态**，
  session 落 durable 待命，之后 `send` 合法复活。这正是 stop 想要的
  "优雅取消 + 可续"语义（对齐终端 SIGTERM）。
- DESIGN §867「协议预留（尚未实现）」删掉 `远程 stop command（GAPS G12）`；
  §交互协议加一行：stop = 远程硬取消（teardown-no-mark，镜像终端 SIGTERM）。
- **顺带修正**：`handleDrive` 当前在裸 daemon ctx 上跑 `s.Drive`（无 per-run
  cancel），drive 系列此前**不可 stop**；本增量给 handleDrive 加 per-run
  cancel（mirror handleRun 740-743），使 drive 也可 stop。

## 验收
- **闸门 A（scripted 孪生，internal/daemon）**：
  - `TestStopTearsDownHostedRun`：注入阻塞在 `<-ctx.Done()` 的 RunFunc，
    `{cmd:stop}` → ack `"stopping"` + 观察到 ctx cancel + **无** SessionClosed。
  - `TestStopUnknownSession`：stop 未知/已结束 session → error 行。
  - `TestStopThenSendRevives`：stop 后 send 合法复活（无标记不挡 send）。
  - `TestDrivePerRunCancel`：drive hub 有非 nil stop（可被 stop）。
- **闸门 B（手验）**：真 daemon 起一个长跑 run，`ar stop <sid>` → run 拆掉、
  journal 无 session_closed、随后 `ar send` 复活。

## 实施步骤
1. 一步（`INC-4: remote stop`）：
   - `internal/daemon/daemon.go`：`handleStop`（mirror handleInterrupt，调
     `hub.stopHosting()`）+ dispatch `case "stop"` + unknown-command help 加
     stop；`handleDrive` 加 `runCtx,runCancel := WithCancel(ctx); hub.stop=runCancel`
     并把 `s.Drive(runCtx,...)`。
   - `internal/cli/conversation.go`：`stopCmd`（mirror interruptCmd）。
   - `internal/cli/cli.go`：`case "stop"` + help 行。
   - 测试 + 文档行 + `./scripts/check.sh`（`TMPDIR=/tmp/t`）→ commit → push。

## review 裁决
小增量（S），inline 自审（teardown 语义、drive cancel、错误路径）。
裁三视角 review。
