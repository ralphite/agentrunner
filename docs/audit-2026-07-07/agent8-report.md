# Agent 8 报告:daemon 协议与远程面

daemon 协议层(web UI 的地基)整体**非常健壮**:14 种畸形输入注入、并发风暴、优雅停机、跨重启去重全部扛住,**全程零 panic、零 daemon 崩溃、零数据丢失**。**用户"几条消息搞崩产品"几乎可排除协议层**——问题更可能在 web server 层或前端(或 no-parts bug 冒泡)。发现 2 个 major 语义缺陷 + 若干 minor。

侦察:web server(`web/api.go`)**不直接对 socket 说协议**,而是 shell 出 `ar` CLI(`exec.CommandContext(arPath,"send"...)`);CLI(`internal/cli/conversation.go:197`)才 net.Dial socket。CLI 客户端**无 auto-spawn**,daemon 不在就报错。故两条路都测。

## 一、测试记录

| 场景 | 关键命令 | 结果 | 备注 |
|---|---|---|---|
| 1 协议 smoke | `ping`→`run(conv)`→`send` 协议直发 | PASS | pong/你好/再见,journal 独立验证 |
| 2 协议健壮性 | 14 种畸形输入 + ping 验活 | PASS | 全部体面拒绝,daemon 存活,无 panic |
| 2+ 并发压力 | 60 并发 + 200 快连关 + 30 挂起 | PASS | FD 无泄漏,无死锁 |
| 3 idem_key(run) | 同 key run×2/不同 key | PASS | 同 key→同会话,idem.json 持久化 |
| 3 idem_key(send) | 同 idem_key send×2 | **FAIL** | send 的 idem_key 被忽略→重复投递(A8-1) |
| 4 notifier 生命周期 | close→run_end→通知 | PASS | journal-before-send,stderr fallback |
| 4 跨重启去重 | kill-9→重启→diff notifier | PASS | 零重复(diff IDENTICAL),核心承诺兑现 |
| 5 优雅停机 SIGTERM | 运行中 turn 时 SIGTERM | PASS | cooperative cancel,0.6s 退出,exit 0,socket 清理 |
| 5 kill-9 对比 | kill-9→重启恢复 | PASS | stale socket 自动重建,不丢数据 |
| 5c CLI 停机报错 | daemon down 时发 CLI | PASS | `(is the daemon running?)` 清晰;但见 A8-2 |
| 6 attach 补读→live | 协议 attach 长 turn | PASS | 补读在前 live 在后,无缝 |
| 6 多订阅+detach | 双 attach,一个提前 detach | PASS | detach 不影响另一订阅与会话 |
| 7 anthropic 次 provider | `printenv ANTHROPIC_API_KEY` | **BLOCKED** | key 长度 0,无凭据,跳过 |
| 8 远程审批协议 | 协议 approve + 4 种错误 approve | PASS | 与 CLI 等效,错误/重复 approve 不崩 |
| 9 interrupt 协议命令 | 协议 interrupt 运行中/idle/unknown | PASS | 实际支持(证伪 G12),见备注 |
| 边缘 | send/close/kill 打 ghost/closed 会话 | PASS | 体面报错 + M5.1 revive;但见 A8-3 |
| 并发 send 风暴 | 12 并发 send 同一会话 | PASS | 12/12 delivered,零丢失(排除"搞崩") |

## 二、发现的问题

### [A8-1] send 的 `idem_key` 字段被完全忽略——重复投递 🟠 major
- 复现(协议原文,同一 idle 会话连发两次相同 idem_key):
  ```json
  {"cmd":"send","session":"...-74b4","text":"幂等探针:回OK","idem_key":"send-idem-777"}
  {"cmd":"send","session":"...-74b4","text":"幂等探针:回OK","idem_key":"send-idem-777"}
  ```
- 期望: 同 idem_key 第二次应去重(类比 run 的幂等)。Command 结构体确实声明了 IdemKey(daemon.go:58)。
- 实际: 两次都回 delivered,journal input_received 从 1→3(**+2**),"幂等探针"出现 2 次。根因: `handleSend`(daemon.go:465-516)**从不读 cmd.IdemKey**——idem 只在 handleRun/handleDrive 实现。且 `ar send` CLI 层根本不暴露 idem_key。网络抖动/重试 → 消息重复 → agent 重复响应+重复计费。

### [A8-2] daemon 死后被中断的会话在 sessions list 永久显示 `running`(僵尸)🟠 major
- 复现: 运行中 turn 时 kill/SIGTERM daemon → daemon down 后 `ar sessions list`。
- 实际: 被中断会话磁盘 fold 停在 running,sessions list(读磁盘不问 daemon)照实显示 running。web handleSessions 解析此输出 → UI 显示"运行中"但实际无 turn 在跑,永不刷新。**与 A5-2/A5-3 同源**。

### [A8-3] `kill` 不存在的 handle 谎报成功 🟡 minor
- `{"cmd":"kill","session":"<live-sid>","handle":"nope"}` → 回 `killing nope`(成功语气)。killHandle(daemon.go:187)只把 handle 塞进 cancels channel 就返回 true,不校验 handle 真实性。

### [A8-4] 协议错误消息里 known 命令列表漏 `drive` 🟡 minor
- unknown command 错误 `(known: ping, run, attach, approve, send, close, interrupt, kill)`(daemon.go:420)漏了 drive;实际支持 9 个命令。

### [A8-5] 独立复现 A2-2:超长 socket 路径 bind 失败 🔴(已登记,不重计)

### 反向确认(非 bug,修正认知)
- **场景 9 / GAPS G12**: 实测**协议层 interrupt 完全实现且工作**(运行中打断、idle 接受、unknown 报错、缺 session 报错,daemon 全程存活)。web interrupt 按钮走协议**可用**,不是 gap。
- **协议健壮性**: 40MB 超大 payload(超 32MB scanner 上限)→ daemon 关连接但自己不崩,安全行为。

## 三、turn 计数
- 工作 XDG /tmp/ar8/xdg: **41 个 activity_completed,横跨 12 会话**。长路径 XDG: 0(A8-5 bind 失败)。总计 41。
- notifier dedup: 3 条 NotificationSent,3 unique key,零重复。

## 四、没测到的
- anthropic 次 provider 全链路(无 ANTHROPIC_API_KEY,BLOCKED)。模型 id 从测试确认为 claude-haiku-4-5-20251001。
- A8-1 的 web 端到端;A8-2 的 revive 恢复路径;drive 协议实跑;notifier command channel 外部命令投递。
