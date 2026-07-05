# AgentRunner — 功能点登记簿（SPEC，第二层）

**这是什么**：产品功能点的**正面清单**——从 JOURNEYS.md 的 journey
拆出，按功能域组织。每条登记：状态、journey 来源、验收锚（哪条测试
证明它 work）。镜像的负面清单（缺口审计）见 GAPS.md；功能点如何成立
见 DESIGN.md（实现偏差以其 §17 为准）。

**维护纪律**（PROCESS.md §二）：增量落地时更新对应条目；验收锚必须
指到真实存在的测试；状态变化不删行、只改标。

**状态图例**：✅ 已实现且有验收锚 · 🟡 部分实现（备注列缺口）·
❌ 未实现（引 GAPS 条目）· 🧊 显式推迟/裁掉（有记档）。

**验收锚缩写**：`QA-xx` = qa/run-qaxx.sh（真实 API，QA.md 菜单）；
`C1–C10` = DESIGN §16 核心验收（scripted 孪生）；`S1–S7` = v1
acceptance 26 场景（e2e/，按阶段）；具名测试 = Go 测试名。

---

## A · 会话与输入（内核）

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| conversational 续聊（答完 park 待命，close 才终结） | ✅ | UJ-01/03/09 | QA-01 · C1 · TestConversationalMultiInput/ParkResumes |
| 忙时投递排队（turn 边界按序消费，不丢不乱序） | ✅ | UJ-07 | QA-02/06 · C2 |
| durable mailbox（确认即持久、恰好一次，跨 kill -9） | ✅ | 不变量 | QA-08 FAIL 级断言 · inbox.jsonl 机制（DESIGN §2） |
| interrupt 与输入分立（Esc 杀活动 / 消息追加） | ✅ | UJ-07 | QA-02/06 · C8 · S3 |
| idle 处 interrupt = close（交互惯例） | ✅ | UJ-03 | 孪生（DESIGN §17 记档） |
| 图片输入（`ar send --image`，CAS ref、组装 inflate） | ✅ | UJ-04 | QA-07/03 · C9 · TestConversationalImageInputEndToEnd |
| 长贴折叠（>10KB 转 file part） | ✅ | UJ-04 | TestLongPasteFoldsToFilePart |
| `ar new` 开场消息折叠/带图（与 send 对称） | 🧊 | UJ-04 | 不对称记档（DESIGN §17），待真实使用反馈 |
| PDF/附件泛化 | ❌ | UJ-04 | GAPS G1 余项 |
| 外部事件唤醒既有 session（webhook → inbox，机器发送方） | ❌ | UJ-12 | GAPS G14（inbox 原语已备，缺投递壳） |
| WAITING_APPROVAL park 期间消息唤醒 | 🟡 | UJ-07 | 只排队不唤醒；GAPS G3 余项 |
| 手动 compact（带指示）/ clear | ❌ | UJ-09 | GAPS G7 |
| 自动 compaction（阈值触发） | ✅ | UJ-09 | S3 |
| 手动 barrier 打点（`ar barrier`，非运行中 session） | ✅ | UJ-15 | fork 全链路测试（S7 收口） |

## B · 子 agent 与编排

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| 后台 spawn（非阻塞拿 handle，`spawn_agent{background}`） | ✅ | UJ-18 | QA-04 · C3 |
| 完成回执激活父 turn（先回先处理） | ✅ | UJ-18 | QA-04/05 · C4 |
| 杀死子 agent（`task_kill` / `ar kill` 双路径，部分产出留存） | ✅ | UJ-18 | QA-05/09 · C5 |
| steer 改变编排（杀一个、起一个） | ✅ | UJ-07/18 | QA-06/09 · C6 |
| 完整编排七步（多输入+并行+杀+回灌+续聊+恢复） | ✅ | UJ-18 | QA-09 · C7 |
| 父崩溃 settle-from-child-fold | ✅ | 不变量 | QA-08(c) · C10(c) |
| 阻塞 spawn/await（v1 形态，保留） | ✅ | UJ-18 | S4 |
| handoff（`handoff_agent`）/ blackboard（`publish_note`/`read_notes`） | ✅ | UJ-18 | S4 |
| 树预算 / 权限冻结交集 / 深度扇出上限 | ✅ | UJ-18/20 | S4 |
| 子 agent 可被 steer | 🧊 | UJ-18 | v0 显式否（杀+重起代替），记档 |
| 子 agent 实时进度镜像 | ❌ | UJ-18 | GAPS G10 |
| 三套子执行收敛为递归 session | 🟡 | — | 阻塞/后台/driver 并存（DESIGN §17） |
| 并发子 agent 确定性测试基建（routing provider） | ✅ | — | C3–C7 孪生在用（GAPS G4 关闭事实） |

## C · 工具面

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| read_file / write_file / edit_file | ✅ | UJ-02/05 | S1 · QA-03（write_file） |
| bash 前台+后台（task_output/task_kill、进程组取消） | ✅ | UJ-02/18 | S1/S3 · QA-05 |
| semantic_search（IndexStore，BM25） | ✅ | UJ-01 | S7 |
| publish_artifact（`outputs:` contract、审批载荷） | ✅ | UJ-06 | S5 |
| exit_plan_mode（plan mode 跃迁） | ✅ | UJ-06/11 | S2/S3 |
| schedule_next / finish_series（loop 自定步调） | ✅ | UJ-14 | S6 |
| grep / glob 独立工具 | ❌ | UJ-01 | GAPS G18（现借 bash） |
| web fetch / search | ❌ | UJ-01 | GAPS G18（未 spec，牵动 network 与注入面） |
| ask_user（wait-class 提问）/ finish | 🧊 | UJ-06 | 记档不预做（DESIGN §17；GAPS G20） |
| tool 输出截断（per-tool 上限） | ✅ | 不变量 | S3 |

## D · 权限与安全

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| rules（tool/path/command/network + realpath 归一） | ✅ | UJ-08/20 | S2 · S7（network） |
| modes（default/plan/acceptEdits + bypass 不跳 hooks） | ✅ | UJ-06/11 | S2/S3 |
| 审批流（ask → WAITING_APPROVAL → 应答/拒绝理由回灌） | ✅ | UJ-08 | S2 · 远程审批 S6 |
| hooks（pre/post，observe+block） | ✅ | UJ-19 | S2 |
| 网络沙箱（netns 收容棘轮、fail-closed） | ✅ | UJ-20 | S7 |
| 凭据 redaction + 硬排除表（含 .netrc/.npmrc 等） | ✅ | UJ-20 | S2/S7 收口 |
| 信任模型（project 层 hooks 需显式 trust，`ar trust`） | ✅ | UJ-20 | S2 |
| 审批答复写回规则（"允许且不再问"） | ❌ | UJ-08 | GAPS G5（PolicyChanged 事件已设计） |
| prompt injection 威胁模型成文 | 🟡 | UJ-20 | GAPS G16（硬防线在，条款未成文） |
| hooks 生命周期扩展（session start/stop 等） | ❌ | — | GAPS G19（无 journey 覆盖） |

## E · 持久化与恢复

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| journal + 纯 fold + snapshot-resume | ✅ | 不变量 | S2 · crash 注入 |
| in-doubt 按类别处置（LLM 重发/只读重跑/执行不重跑） | ✅ | 不变量 | S2 · QA-08(b) |
| crash 矩阵三态复活（idle/在飞 bash/在飞子 agent） | ✅ | UJ-09 | QA-08 · C10 |
| send 即复活（journal 形状把关，拒绝 task/已 ended） | ✅ | UJ-09 | QA-08 · TestSendRevivalDiesWithDaemon + 拒绝测试 |
| 恢复分模式（conversational 自愈 / task 上浮） | ✅ | 不变量 | QA-08 · 决策 #29 |
| workspace 快照（shadow repo、排除表、pinned） | ✅ | UJ-15 | S2/S7 |
| daemon kill -9 后孤儿 bash 子进程清扫（pgid） | 🟡 | — | 记档观察项（DESIGN §17） |
| shadow repo 并发 flock（daemon 多 session） | 🧊 | — | backlog（加 gc 前必须先做，LOG/v2 台账记档） |

## F · 驱动（one-shot / goal / loop / best-of-N）

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| goal mode（verifier 三态、停滞检测、carry） | ✅ | UJ-15 | S6 |
| loop mode（interval/cron/self_paced、overlap skip/coalesce） | ✅ | UJ-14 | S6 |
| verifier 管线化（journaled effect、driver-trust 规则层） | ✅ | UJ-15 | S7 |
| best-of-N（隔离 worktree、per-attempt 判定、胜者留盘） | ✅ | UJ-16 | S7 |
| overlap: interrupt | 🧊 | UJ-14 | backlog（与顺序执行同理推迟） |
| 胜者晋升（fork / apply diff） | 🧊 | UJ-16 | GAPS G15（v0 用户手动晋升，记档） |
| cron 跨重启唤醒 | 🧊 | UJ-14 | backlog |

## G · 时间旅行（barrier / fork / rewind）

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| CheckpointBarrier（turn 边界/终态/手动，向量+快照 ref） | ✅ | UJ-15 | S7 |
| fork（单创世、处置向量落实、随行库复制、独立 worktree） | ✅ | UJ-15 | S7 收口 review 修复 + fork-of-fork 测试 |
| rewind（fork 后显式切换） | ✅ | UJ-15 | S7 |
| 多模态 blob 在 fork/rewind 下的归属语义 | 🟡 | — | GAPS G1 余项 |
| barrier tasks 对"任务=子 agent"的处置语义 | 🟡 | — | GAPS G2 余项 |

## H · 生态接入

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| MCP（stdio 全生命周期、schema 记录、断连恢复、写审批） | ✅ | UJ-19 | S5 |
| MCP transport: http + OAuth | 🧊 | UJ-19 | schema 预留，实现推迟 |
| skills（Claude Code 约定） | ✅ | UJ-19 | S5 |
| memory 文件读侧注入（CLAUDE.md 层级合并） | ✅ | UJ-09 | S3 |
| 记忆写回（# remember → CLAUDE.md） | ❌ | UJ-09 | GAPS G9 |
| 自定义命令 / slash 面 | ❌ | UJ-19 | GAPS G21 |

## I · 观察与远程面

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| events / inspect（时间线、判定、子树、用量） | ✅ | UJ-17 | S3/S6 |
| `ar ps`（fold 的在飞任务列表，无 daemon 可用） | ✅ | UJ-18 | QA-05/09 实测 |
| attach/detach（journal 补读 + live 订阅） | ✅ | UJ-17 | S6 |
| 远程审批（daemon approve） | ✅ | UJ-17 | S6 |
| notifier（生命周期通知、跨重启去重） | ✅ | UJ-14 | S6 |
| 远程 stop command | ❌ | UJ-17 | GAPS G12（interrupt 只绑终端信号） |
| HTTP/WS 壳 | 🧊 | UJ-13 | backlog |

## J · 运行形态与云

| 功能点 | 状态 | Journey | 验收锚 / 备注 |
|---|---|---|---|
| daemon 托管（socket、idem_key 幂等、优雅停机） | ✅ | UJ-17 | S6 |
| task 模式（run/drive/submit/resume，固定 epilogue） | ✅ | UJ-02/14/15 | S1–S7 |
| 运行中 spec 变更（换模型/角色切换的变更事件族） | ❌ | UJ-11 | GAPS G8 |
| 云 workspace 生命周期 | 🧊 | UJ-13 | GAPS G11（S7 预授权裁掉，重启走新增量） |
| IDE 集成 | 🧊 | — | 同上裁决 |
| 多根 workspace（--add-dir 类） | ❌ | — | GAPS G17（待 journey 目录定版） |

---

## 附录 · 代码事实对照（2026-07-05 盘点）

**CLI 子命令**（`internal/cli/cli.go`）：
`run` `drive` `submit` `resume` `new` `send` `close` `interrupt` `kill`
`ps` `approve` `fork` `barrier` `sessions` `trust` `attach` `daemon`
`events` `inspect` `accept` `record-fixture` `version`

**daemon 线协议命令**（`internal/daemon/daemon.go`）：
`ping` `run` `drive` `attach` `approve` `send` `close` `interrupt` `kill`

**内置 tool 定义**（`internal/tool/defs/*.json`）：
`read_file` `write_file` `edit_file` `bash` `task_output` `task_kill`
`spawn_agent` `handoff_agent` `publish_artifact` `publish_note`
`read_notes` `semantic_search` `exit_plan_mode` `schedule_next`
`finish_series`
