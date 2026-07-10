# SPRINT — HANDA-PARITY 补齐冲刺（总控 · 活文档）

**这是什么**：把 docs/HANDA-PARITY.md §2 中 17 个「裁决实现」项
队列化的**冲刺总控**。每个具体项开工时仍按 PROCESS §二另立
`INC-<n>-<slug>.md`；本文件只管**队列、认领、状态、节律**。完成后
按归档纪律移入 archive。

**每轮 SOP 与并发协作约定**：同 `SPRINT-claudecode-parity.md`
（认领 = 改状态即 push；选题前先 pull 本文件与 git log；触不变量项
先出 PROCESS §4 变更单标 📐；冲突宁可让路换题）。

**跨 sprint 联动项**（两边任一处认领，另一处状态跟改，避免双做）：
- 本队列 **#7** = CLAUDECODE-PARITY SPRINT **#10**（ask_user 结构化选项）
- 本队列 **#28b** = CLAUDECODE-PARITY SPRINT **#15**（boot sweep + cron 跨重启）
- 本队列 **#14** 与 CLAUDECODE SPRINT **#17**（webui rename/归档/搜索）
  相邻——webui 区动工前互查认领。

## 队列与状态

图例：⬜ open · 🔧 in-progress · ✅ done · 📐 awaiting-review ·
⏸ blocked-external · 🚫 skipped。（# = HANDA-PARITY §2 编号；方案
细节与 review 修正以 HANDA-PARITY §2/§4 为准。）

### 批 1 · 速赢（全 additive，零不变量）

| # | 项 | 规模 | 状态 | 备注 |
|---|---|---|---|---|
| 32 | stdin 管道 prompt（ar run/new/send 读 stdin） | S | ✅ done (INC-28) | 双闸门全绿；真 Gemini 管道开场+`-` 多行续聊；/dev/null 按非管道处理记档 |
| 23 | 用户消息折叠（Timeline >10 行 Show more） | S | ✅ done (INC-36) | 双闸门全绿；真浏览器 DOM 断言（qa/runs/2026-07-10-INC36）；含 pending 气泡 |
| 9 | progress_update 内部工具 + fold + Supervision 区 | S/M | ✅ done (INC-37) | 双闸门全绿；真 Gemini 7 次自发调用+webui DOM 断言（qa/runs/2026-07-10-INC37）；面板不因 progress 强开（W5 语义） |
| 10 | 后台任务 notify 门 + settle 结构化载荷 | S | 🔧 in-progress (INC-39) | 唤醒已存在（勘误见 PARITY §4.1）；结构化载荷核查后亦已存在（result 带 exit_code/stdout tail），真 delta 仅 notify 门 |
| 11 | artifact 消费面（工具读回/CLI/webui 三面） | M | ⬜ | ArtifactPublished 已 fold，纯 additive |
| 31 | 运行统计 stats（IsError 聚合/行增删入载荷/TS 报表投影） | M | ⬜ | 行增删写 ActivityCompleted，不 diff redacted args |

### 批 2 · 命令面设计单元（一个 INC 设计、分步落地；#29 走 PROCESS §四）

| # | 项 | 规模 | 状态 | 备注 |
|---|---|---|---|---|
| 2U | 「命令身份·撤销·应答」统一设计单元（covers #16/#29/#7） | M(设计) | ⬜ | 三项同改 protocol/daemon-dispatch/消费侧，先设计后分步 |
| 16 | turn retry（派生 command_id `retry:<turn-id>`） | S | ⬜ | 依赖 2U |
| 29 | 排队消息撤销（durable revoke 五点语义） | M | ⬜ | 依赖 2U；触 §2 铁律走 §四 |
| 7 | 结构化 ask_user（typed AskResolved + ar answer + 表单卡） | M | ⬜ | 依赖 2U；= CC SPRINT #10 联动 |

### 批 3 · 内核（触不变量项单独 INC）

| # | 项 | 规模 | 状态 | 备注 |
|---|---|---|---|---|
| 8 | LLM goal judge（llm_call 管线 effect，门控触发，三态） | M/L | ⬜ | 走 §四；修订 §13/决策 #21（非 #34） |
| E2 | 外部事件唤醒 G14（HTTP ingress + source:machine + untrusted 硬条件） | M | ⬜ | UJ-12 卡死项；HTTP 壳联动 backlog |
| 28b | cron 跨重启唤醒 + boot sweep（G22） | M | ⬜ | = CC SPRINT #15 联动 |

### 批 4 · 工具面

| # | 项 | 规模 | 状态 | 备注 |
|---|---|---|---|---|
| 4 | 自定义 command tools（trust 门=hooks 级 + 全管线 + sandbox） | M | ⬜ | project 层=可执行配置（决策 #19） |

### 批 5 · webui 消费面

| # | 项 | 规模 | 状态 | 备注 |
|---|---|---|---|---|
| 20 | Markdown 增强（react-markdown+gfm 表格+highlight.js） | M | ⬜ | 保持禁 raw HTML |
| 14 | LLM auto-title（SessionTitled{auto} journal 事件） | S/M | ⬜ | manual rename 不迁移（§12:1092）；承接 INC-23 W9 移交；与 CC #17 避让 |
| 24 | project overlay + launcher（meta.json 扩展 + /api/open） | S/M | ⬜ | 不建服务端注册表 |
| 18 | ar dictate 服务端听写（provider 补 audio part） | M | ⬜ | 走 ar 命令，webui 薄壳不变 |
| 19 | ar optimize prompt 优化 | S | ⬜ | 搭 #18 的车 |

## 轮次日志（每轮一行，追加）

| 轮 | 日期 | 项 | 结果 | commit |
|---|---|---|---|---|
| 1 | 2026-07-10 | #32 stdin 管道 prompt (INC-28) | ✅ 双闸门全绿（孪生 7 测 + 真 Gemini 管道开场 PONG/`-` 多行续聊 PONG2，qa/runs/2026-07-10-INC28）；/dev/null 边界记档 | (见 push) |
| 2 | 2026-07-10 | #23 用户消息折叠 (INC-36) | ✅ 双闸门全绿（vitest+build + 真浏览器 DOM 断言：10lh 钳/Show more-less/mobile/console 0 err，qa/runs/2026-07-10-INC36）；宽度塌缩 bug 当场修（width:max-content） | (见 push) |
| 3 | 2026-07-10 | #9 progress_update (INC-37) | ✅ 双闸门全绿（孪生 4 测+event round-trip 守卫 + 真 Gemini 私有 daemon：7 次自发调用 3/3 done、inspect 两面、webui DOM 断言，qa/runs/2026-07-10-INC37）；面板不因 progress 强开（W5 裁决记 LOG） | (见 push) |
