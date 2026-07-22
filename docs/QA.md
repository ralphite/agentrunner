# v2 — QA 场景菜单（可执行验收）

**这是什么**：v2 核心功能（DESIGN §16 C1–C10）的**真实使用场景**验收
菜单。每个场景 = 基础配置（agent + workspace）+ 用户输入 + 执行流程 +
客观通过标准，**逐字照做即可执行**。开发全程用它守门：一个功能没有
对应场景绿灯，就不算 work。

**与单元测试的分工**：这里全部走**真实 provider API**（不是 scripted
fixture）——测的是"产品在真实条件下 work"；确定性单元/集成测试
（scripted/routing provider）另在实现侧红绿推进，两层互不替代。

---

## 0. 总则

### 0.1 环境与 provider
- 凭据：workspace 根 `.env` 提供 `GEMINI_API_KEY`（主）或
  `ANTHROPIC_API_KEY`（备）。永不提交。
- 模型：主跑 `gemini` provider（QA-07/09 需要 vision 能力的模型）；
  每个场景标注可替换性。
- 真实 API 的非确定性对策（**全菜单通用**）：
  1. **指令式 prompt**——用户输入把要做的事说死（"启动恰好 3 个
     子 agent，分别负责 A/B/C"），不给模型发挥空间；
  2. **结构断言**——通过标准只看客观事实（journal 事件序列、文件
     状态、子 session 目录、测试红绿），不匹配模型的措辞；
  3. **一次重跑**——模型偶发不配合（如没按指令起 3 个子 agent）允许
     整场景重跑一次；连续两次不过 = FAIL，附 journal 归档分析。

### 0.2 workspace 准备（保证每次环境一致）
用 `qa/ws.sh`，全部 repo 按 **SHA 钉死**（与上游漂移无关）：

```
qa/ws.sh prepare <profile> <dir>    # color | cobra | cobra-broken | gin | blank
qa/ws.sh cleanup <dir>              # 用完即删
```

| profile | 内容 | 用途 |
|---|---|---|
| color | fatih/color @ 53d4ce9d | 小型，问答/续聊 |
| cobra | spf13/cobra @ ad460ea8 | 中型，常规开发 |
| cobra-broken | cobra + 注入失败测试包 `qa_inject/` | 修复类（红→绿客观判定）|
| gin | gin-gonic/gin @ 34dac209 | 中大型，多 agent 探索 |
| blank | 空目录 | 起项目类 |

### 0.3 CLI 契约（v2 的目标接口，实现必须提供这些动作）
```
ar new <spec.yaml> --workspace <dir>      # 建会话；默认渲染开场回复至待命
                                          # (脚本用 --detach: stdout 只有 <sid>)
ar send <sid> "文字" [--image <png>]       # 投一条用户消息;默认渲染回复至待命
                                          # (脚本用 --detach: 投递即回 "delivered")
ar attach <sid>                            # 订阅输出流（turn/工具/子 agent 事件）
ar interrupt <sid>                         # 带外打断当前活动（≠发消息）
ar ps <sid>                                # 列在飞子 session / 后台任务
ar kill <sid> <handle>                     # 用户侧杀死一个子 session
ar events <sid> / ar inspect <sid>         # 看 journal / 时间线
ar close <sid>                             # 关闭会话
# runtime 重启后 session 自动在：ar send 即续聊，无需特殊 resume 动作
```

### 0.4 基准 agent spec（各场景在此上微调）
```yaml
# base.yaml
name: dev
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 4096 }
system_prompt: |
  你是一个严谨的编码助手。严格按用户指令行动；用户要求启动子 agent 时,
  用 spawn_agent 工具、数量与分工严格照做；要求取消时用 kill。
# write_file 自 M4.3 起可用；此前的场景去掉它即可运行
tools: [read_file, write_file, edit_file, bash, spawn_agent, kill]
agents: [worker]
permissions:
  - { action: allow }        # QA 聚焦 runtime 行为；权限场景另有专项
```
```yaml
# worker.yaml —— 子 agent
name: worker
description: 执行父分派的调查/修改任务并汇报
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 4096 }
system_prompt: 完成任务后用简洁的要点汇报结论。
tools: [read_file, bash]
```

### 0.5 观察手段
- `ar attach` 的实时流（人工观察时序）；
- `ar events <sid>`：journal 事件（断言的主要依据）；
- 子 session journal：`<数据目录>/sessions/<sid>/sub/…`；
- workspace 文件与 `go test` 红绿。

---

## QA-01 三轮续聊问答 `覆盖 C1`
**环境**：`ws.sh prepare color ws1`；base.yaml。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar new base.yaml --workspace ws1` → sid | 会话建立，状态待命 |
| 2 | `ar send $sid "这个库的 NoColor 开关在哪里实现？引用文件和行号"` | 一个 turn 跑完，回答引用 color.go；**会话回到待命，没有结束** |
| 3 | 等 30 秒（模拟人思考） | 会话仍在，无新事件 |
| 4 | `ar send $sid "它和环境变量 NO_COLOR 的关系是？"` | 新 turn；回答衔接上一轮（说明它读过/记得 NoColor 实现），不重新自我介绍 |
| 5 | `ar send $sid "把你前两个回答合并成三句话总结"` | 新 turn；总结内容同时涉及前两轮 → 上下文连续的客观证据 |

**通过标准**：journal 里恰好 3 条 `user_message` 输入、≥3 个 turn、
**0 个 `session_closed` 标记**；步骤 5 的回答包含前两轮各自的要素——脚本用
钉入的暗号词（步骤 2/3 各埋一个，最终回答必须同时复述）把"要素"
变成客观断言（收口 F.3 起 FAIL 级）。
**清理**：`ar close $sid && ws.sh cleanup ws1`

## QA-02 忙时插话排队 `覆盖 C2, C8(输入侧)`
**环境**：`ws.sh prepare cobra ws2`；base.yaml。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar send $sid "运行 ./qa_slow.sh 然后告诉我输出"`（准备时放入 `qa_slow.sh`：`sleep 25; echo SLOW_DONE`） | agent 起 bash，turn 在飞 |
| 2 | bash 跑到约 5 秒时：`ar send $sid "跑完后，顺便数一下仓库里有多少个 _test.go 文件"` | 命令**不被打断**；输入落 journal（`InputReceived`）排队 |
| 3 | 约 10 秒时再发：`ar send $sid "用中文回答"` | 同上，第二条排队 |
| 4 | 等 bash 自然结束 | 输出含 SLOW_DONE；**下一 turn 开头**两条排队消息按序进入上下文 |
| 5 | 观察后续 turn | agent 数了 _test.go 数量且用中文——两条插话都生效 |

**通过标准**（收口 F.3 修正——实现语义是 journal-on-boundary 而非
journal-on-arrival，见 archive/v2/PROGRESS.md M2.1）：两条插话在 bash 期间投递
（mailbox 持久，确认即不丢），其 `InputReceived` 在安全边界按投递
顺序落 journal（必然在 bash `Completed` 之后，这是设计而非缺陷）；
bash 无 Cancelled；两条都进入下一 turn 的上下文。回答是否同时满足
两条插话属模型行为，不设 FAIL 闸（§0.1）。

## QA-03 修复注入 bug + 建新文件 `覆盖 C1(尾部)、核心9(write_file)`
**环境**：`ws.sh prepare cobra-broken ws3`；base.yaml。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `go test ./qa_inject/`（人工确认红） | FAIL 基线 |
| 2 | `ar send $sid "qa_inject 包的测试挂了，修复实现（文档说 Add 是加法），不要改测试"` | agent 读码 → 改 calc.go → 自己跑测试验证 |
| 3 | `go test ./qa_inject/`（人工复核） | **绿**；calc_test.go 未被修改（内容哈希前后对比——qa_inject 是 untracked，git diff 看不见它） |
| 4 | `ar send $sid "在仓库根新建 QA_NOTES.md，两句话记录你改了什么"` | 用 write_file 创建**新文件**（不是 bash heredoc）|
| 5 | 检查 QA_NOTES.md 存在且非空；journal 中该文件由 write_file 工具落盘 | 核心 9 的 write_file 路径真实走通 |

**通过标准**：步骤 3、5 的客观检查全绿。

## QA-04 三路并行子 agent、先回先处理 `覆盖 C3, C4`
**环境**：`ws.sh prepare gin ws4`；base.yaml + worker.yaml。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar send $sid "启动恰好 3 个子 agent 并行调查：A=render 目录的职责，B=binding 目录的职责，C=middleware 机制。它们跑的时候你先自己读一遍 README 等结果"` | 同一 turn 内 3 条 `ChildSpawned`；**turn 正常结束**（父没有卡在等子）|
| 2 | `ar ps $sid` | 列出 3 个在飞子 session（handle 可见）|
| 3 | 观察 3 个子 journal 的时间区间 | 两两重叠 → **真并行**的客观证据 |
| 4 | 等第一个 child_result 回灌 | 父**立即**起新 turn 消化（不等三个全回来）|
| 5 | 等其余两个 | 每个 child_result 各触发一个 turn（或与排队合并），最终父给出三路汇总 |

**通过标准**（收口 F.3 对齐——真实 API 闸门钉结构事实，时序/内容级
性质由确定性 scripted 孪生守）：真实闸门 FAIL 级 = ≥3 spawn、≥3
subagent_completed、≥3 个子 journal、父 turn ≥2（spawn+消费）、全部
spawn 落在首个父 turn（越界降 WARN 抗真实时序抖动）。"同 turn 并行
启动、两两重叠、先回先处理、双报告达模型"由 scripted 孪生
TestBackgroundSpawnParallelAndSettle 以确定性断言背书；汇总内容不设
FAIL 闸（§0.1）。

## QA-05 steer 杀一换一 `覆盖 C5, C6`
**环境**：同 QA-04（gin）。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar send $sid "启动 2 个子 agent：A=逐文件详细分析 render 目录，B=逐文件详细分析 binding 目录"`（任务刻意重，保证跑得久） | 2 条 ChildSpawned；父待命 |
| 2 | 两子在飞时：`ar send $sid "B 不用查了，取消它；改起一个新的 C 调查 gin 的路由树实现"` | 消息进 inbox；父下一 turn：`kill(B)` + `spawn_agent(C)` |
| 3 | 观察 B 的子 journal | 有取消收尾与 `killed` 标记（部分产出留存）；B 向父投了 `child_result{canceled}` |
| 4 | `ar ps $sid` | B 消失，A 与 C 在列 |
| 5 | 等 A、C 完成 | 父汇总只含 A 与 C 的结论，并提到 B 被取消 |
| 6 | 变体（用户直接杀）：重复步骤 1，然后 `ar kill $sid <handleA>` | 不经模型，A 直接取消；父下个 turn 看到 canceled 回执 |

**通过标准**（收口 F.3 对齐——脚本分工）：run-qa05.sh 实测**用户
直杀**路径（步骤 6）：ar ps 列出在飞 handle、kill 后该子结算为非
completed、部分产出 best-effort（WARN 级）、另一子不受影响、会话
续跑;直杀有持久起源 InputReceived{source:control}。**模型杀路径**
（步骤 1–5,C6）由 scripted 孪生 TestSteerChangesOrchestration 确定
性背书 + QA-09 真实 API 断言 kill 调用;两路径共用同一 cancel
注册表（代码层同一原语）。

## QA-06 interrupt 与消息分立 `覆盖 C8`
**环境**：`ws.sh prepare cobra ws6`；base.yaml；放入 `qa_slow.sh`（sleep 30）。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar send $sid "运行 ./qa_slow.sh"`；5 秒后 `ar send $sid "完事说 OK"` | **消息不打断** bash（对照组——run-qa06.sh 委托 QA-02 覆盖此步,不重复） |
| 2 | bash 自然结束后，再次 `ar send $sid "再跑一次 ./qa_slow.sh"` | 第二次长任务在飞 |
| 3 | 5 秒后 `ar interrupt $sid` | bash **被取消**：journal 有 Cancelled + 部分输出；进程组确认退出（`pgrep -f qa_slow` 为空）|
| 4 | `ar send $sid "刚才怎么了？"` | 会话正常继续；agent 能说明命令被打断 |

**通过标准**：同样的在飞状态，`send` 排队、`interrupt` 取消——两条
路径在 journal 里形状不同且互不串扰；打断后会话可继续。

## QA-07 图片输入 `覆盖 C9`
**环境**：`ws.sh prepare cobra ws7`；base.yaml；fixture：
`qa/fixtures/build-error.png`（一张含编译错误的"CI 截图"）。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar send $sid --image qa/fixtures/build-error.png "这是 CI 的截图。哪个文件哪一行报了什么错？"` | 回答准确说出 `command.go` / `1234` / `EnableTraverseRunHooks2`（图片真被模型读到）|
| 2 | 检查 journal | `InputReceived` 只含 **CAS ref**（无 base64 字节；journal 单行 < 2KB）|
| 3 | `ar send $sid "在这个仓库里搜一下截图里那个标识符存不存在"` | 续聊 turn 里 agent 检索 `EnableTraverseRunHooks`（上下文里图片信息延续）|

**通过标准**：步骤 1 三要素全说对；步骤 2 的 ref-not-bytes 断言成立；
步骤 3 证明多模态内容进入了持续上下文。

## QA-08 恢复三态 `覆盖 C10`
**环境**：`ws.sh prepare cobra ws8`；base.yaml。runtime 以 daemon 方式跑
（可被 kill -9）。

| # | 动作 | 验证 |
|---|---|---|
| a1 | 完成一轮问答（会话待命）→ `kill -9` runtime → 重启 | 无任何特殊操作 |
| a2 | `ar send $sid "接着刚才的话题，再补充一点"` | **续聊无缝**：回答衔接崩溃前内容 |
| b1 | `ar send $sid "跑 ./qa_slow.sh"`；bash 在飞时 `kill -9` runtime → 重启 | in-doubt 处置：bash 不静默重跑，渲染 interrupted-by-crash 类结果 |
| b2 | `ar send $sid "刚才的命令什么状态？"` | agent 基于 journal 事实回答；会话继续 |
| c1 | 起 2 个子 agent（QA-04 步骤 1 的缩减版）；子在飞时 `kill -9` → 重启 | 子 session 有独立 journal：已静止的 settle 回执补投父 inbox；未静止的恢复或按策略结算 |
| c2 | 观察后续 | 父最终收到每个子的回执（完成/取消/崩溃结算），无孤儿进程（`pgrep` 空）|

**通过标准**：三态各自恢复后**同一会话都能继续对话**；无输入丢失
——崩溃前排队的消息重启后恰好一次落 journal 且被后续 turn 消费
（结构断言;模型是否复述其内容不设闸）。**孤儿进程记档**：kill -9
daemon 会孤儿化在飞 bash 的子进程（进程组随 daemon 死而失管,
sleep 类自然退出;长驻型需重启后 pgid 清扫——已列 GAPS 余项,
原"pgrep 空"标准对 kill -9 场景不可达成,收口 F.3 修正）。

## QA-09 完整编排（压轴，用户原始用例） `覆盖 C7 = C1..C6+C9+C10a 串联`
**环境**：`ws.sh prepare gin ws9`；base.yaml + worker.yaml；
fixture 截图。**DESIGN §8 七步的真实 API 版**：

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar send $sid --image build-error.png "结合截图，启动恰好 3 个子 agent 分别调查：A=这个错误可能涉及的机制, B=binding 目录, C=middleware。等它们结果"` | 图片入上下文；3 子并行；父待命 |
| 2 | 第一个子回来 | 父起 turn 消化（先回先处理）|
| 3 | `ar send $sid "B 取消，换成 D：调查路由树"` | 下一 turn：cancel B + spawn D |
| 4 | 其余子全部回来 | 每个回执激活 turn；最终父给汇总（含 B 被取消的说明）|
| 5 | `ar send $sid "为什么你先处理了 A 的结果？"` | 续聊：基于本会话历史作答 |
| 6 | `kill -9` runtime → 重启 → `ar send $sid "把最终结论写进 SUMMARY.md"` | 恢复后续聊 + write_file 落盘 |

**通过标准**：全程一个 session；journal 完整讲述七步故事（3 spawn →
先回先处理 → cancel+spawn → 全回灌 → 续聊 → 崩溃 → 续聊+写文件）；
SUMMARY.md 存在且内容与会话结论一致。

---

## QA-10 session 内换 agent（决策 #32,UJ-11） `覆盖 G8 关闭`
**环境**：空 workspace;poet.yaml(诗人身份)+ auditor.yaml(审计员身份)。
脚本:`qa/run-qa10.sh`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar new poet.yaml "介绍一下你自己"` | 回复带诗人身份前缀 |
| 2 | `ar agent $sid auditor.yaml` | 单条命令、**无确认交互**;输出 agent switched |
| 3 | `ar send $sid "现在你是谁?"` | 回复带审计员身份;`spec_changed` 恰好 1 条;`session_started` 仍 1 条(同一 session,上下文延续) |

**通过标准**:runtime 红线只钉 spec_changed 落盘/新身份 spec_name/
同一 journal;不钉模型对上文的措辞。

---

## QA-11 grep / glob 独立工具（INC-3,UJ-01） `覆盖 G18 grep/glob 关闭`
**环境**：多文件 workspace(符号散落于子目录 + vendored 树 + 一个
凭据文件 .env);spec `tools: [read_file, grep, glob, bash]`。
脚本:`qa/run-qa11.sh`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar new "用 grep 找出引用 RefreshSentinel 的位置"` | journal 出现 `activity_started{name:grep}` + `activity_completed`(结果回模型) |
| 2 | `ar send $sid "用 glob 列出 internal 下所有 .go"` | journal 出现 `name:glob` 调用 |
| — | 凭据红线 | `.env` 里的 secret 值**从不**出现在 journal 任何处(grep 在 walk 层排除凭据文件) |

**通过标准**:runtime 红线只钉 grep/glob 被真实调用 + 结果落盘 +
凭据值零泄漏;不钉模型措辞。

---

## QA-12 手动 compact / clear（INC-6,UJ-09） `覆盖 G7 关闭`
**环境**：空 workspace,聊两轮(记两个暗号)。脚本:`qa/run-qa12.sh`。

| # | 动作 | 验证 |
|---|---|---|
| 1-2 | 两轮对话 | 2 条 assistant_message |
| 3 | `ar compact $sid "务必保留全部暗号原文"` | journal 出现 `context_compacted`,`summary` **非空**(idle 处 compact 曾因会话以 assistant 收尾致 Gemini 返回空 summary→上下文丢失,已修:summarizer 请求补 user 收尾 + 空 summary 不落) |
| 4 | 一轮对话 + `ar clear $sid` | `context_compacted` 追加一条 `cleared=true`、`summary=""`(clear 无新内容时为 no-op) |

**通过标准**:runtime 红线钉 compact 落非空 summary + clear 落 cleared;
不钉模型对暗号的复述措辞。

---

## QA-13 web_fetch + ask_user（INC-5,UJ-01/06） `覆盖 G18 web_fetch、G20 ask_user 关闭`
**环境**：本地 `python3 -m http.server` 服务一含推荐正文（PostgreSQL）+
`<script>` 噪音的页面;空 workspace;spec `tools: [web_fetch, ask_user,
write_file]`（无 bash——逼模型真用 web_fetch,不借 curl）。
脚本:`qa/run-qa13.sh`（需 python3 + `GEMINI_API_KEY`）。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar new "用 web_fetch 抓取 <url>…用 ask_user 问我…按回答 write_file"` | journal 出现 `name:web_fetch` 调用;页面正文 `PostgreSQL` 回模型;`<script>` 体（`MUST_NOT_SURFACE`）被 HTML→text 剥离、不入 journal |
| 2 | 模型 park 提问 | `waiting_entered{input}` 且 detail 含 `question`（与普通 standby idle 区分） |
| 3 | `ar send $sid "采用…"`（inbox 即应答,零新动词） | `ask_resolved{answered}` + `waiting_resolved{answered}` |
| 4 | 应答后同 session 续跑 | `decision.txt` 被 write_file 落盘（答案驱动真实后续工作） |
| — | 健康 | 无 `actor_crashed` |

**通过标准**:runtime 红线钉 web_fetch 真调用 + 正文回灌 + script 剥离、
ask_user park→inbox 应答→续跑落盘、零 crash;不钉模型措辞。
**结果**:2026-07-09 真实 Gemini PASS（归档 `qa/runs/2026-07-09-QA-13/`）。

---

## QA-14 完整 coding agent 端到端（INC-5,UJ-01/02/05/06） `全工具面协同`
**环境**：一个**真实 Go 项目**——semver 版本比较,`Compare` 是 panic 骨架、
`version_test.go` 全红,且 pre-release 排序规则(`beta.2<beta.11` 数值比、
`alpha.1<alpha.beta` 数字段优先级低)**不查规范必错**;本地 http server
服务真实 semver §11 规范页;coding agent spec 带**全工具面**(read/write/
edit/bash/grep/glob/semantic_search/web_fetch/ask_user,allow)。
脚本:`qa/run-qa14.sh`(fixture 在 `qa/fixtures/semver-broken/`,需 python3+go)。

| # | 动作 | 验证 |
|---|---|---|
| 1 | new 一个真实编码任务(实现 Compare,测试全红) | agent 用 glob/read_file 探索测试到底期望什么 |
| 2 | agent 查规范 | `name:web_fetch` 抓 semver-spec.html,规范正文回模型(不凭记忆猜) |
| 3 | agent 动手前对齐方案 | `waiting_entered{question}`(agent 简述完整实现方案)park;`ar send` 确认;`ask_resolved{answered}`,同 session 续跑 |
| 4 | agent 实现 + 验证 | write_file 落 Compare;`name:bash` 跑 `go test` |
| — | **硬证据** | workspace `go test ./...` **真的绿**——agent 完成了非平凡功能,pre-release 全谱排序正确 |
| — | 健康/红线 | 无 `actor_crashed`;`GEMINI_API_KEY` 零泄漏 |

**通过标准**:硬钉 workspace 测试**真转绿**(真实工作,非工具走过场)+
全工具面在一次 agentic flow 里协同;不钉模型措辞。
**结果**:2026-07-09 真实 Gemini,**两次跑均 PASS(可复现)**——8 个
generation step:`glob→read×2→web_fetch→ask_user→write_file→bash go test`,
agent 写出教科书级正确的 semver 实现。归档 `qa/runs/2026-07-09-QA-14/`
(含 agent 实现的 version.go + go test 输出)。

---

## QA-15 PDF/任意文件附件（INC-9,UJ-04） `覆盖 G1 余项 PDF/附件泛化关闭`
**环境**：驾驶舱 API（真实 Gemini gemini-flash-latest）；一份用 ps2pdf 生成的
真实 PDF,正文含秘密词 `ZEBRA-42-QUOKKA`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | 建会话 → `/api/upload` 传 PDF → `/api/sessions/{sid}/send` files:[...]（走 `ar send --file`） | 送达 delivered |
| 2 | 消息问"只回 PDF 里的秘密词" | journal `InputReceived.files` 携 CAS ref + `media_type=application/pdf`（**ref-not-bytes**，journal 不含原字节） |
| — | **硬证据** | Gemini 真实读出 PDF 文本——末条 assistant 回复 = `ZEBRA-42-QUOKKA` |

**通过标准**:file part 上链为 ref（非 bytes）+ 真实 provider 读出 PDF 内容;
不钉模型措辞。
**结果**:2026-07-09 真实 Gemini PASS——ref=sha256-b33ee0c…、mime=application/pdf、
回复=ZEBRA-42-QUOKKA。隔离实例跑（新二进制 daemon,避免重启打扰并发 session）,
归档 `qa/runs/2026-07-09-QA-15/`。

---

## QA-16 会话内 goal（INC-D1,UJ-22） `覆盖 G23 关闭`
**环境**：真实 Gemini gemini-flash-latest；真实 workspace；隔离新二进制 daemon
（goal 支持）。

| # | 动作 | 验证 |
|---|---|---|
| 1 | 真实会话（开场 turn 打招呼）→ `ar goal attach "创建 done.txt 含 FINISHED" --verify "test -f done.txt"` | goal attached |
| 2 | agent 真实干活 + verifier 在静止边界检查 | 真 Gemini 用 write_file/bash 建 done.txt=FINISHED;真命令 verifier `test -f` 通过 |
| — | **硬证据（context 延续）** | **单个 session_started**（非 fresh run）+ goal 作为 program 源输入进对话 + goal_achieved.reason=satisfied |

**通过标准**:goal 挂在同一会话、context 延续（单 SessionStarted）+ 真实
verifier 在真 workspace 判定 + 达成回执;不钉模型措辞。
**结果**:2026-07-09 真实 Gemini PASS——sessions=1、program_inputs=1、
checkpoints=1、achieved=satisfied、done.txt=FINISHED。miss→回灌→续跑的确定性
路径由孪生 TestInSessionGoalContinuity 覆盖。归档 `qa/runs/2026-07-09-QA-16/`。

---

## QA-17 goal 自证完成（INC-10,UJ-22 步骤 2b） `无 verifier 形态`
**环境**：真实 Gemini gemini-flash-latest；真实 workspace；共享 daemon
（新二进制，goal 工具面）。

| # | 动作 | 验证 |
|---|---|---|
| 1 | 真实会话（开场 turn 打招呼）→ `ar goal attach "<写文件类可 eyeball 的目标>"`（**不带 --verify**） | goal attached，attach 注入文含 goal_complete 指引 |
| 2 | agent 真实干活；完成后自行调 `goal_complete{summary}` | journal 出现 goal_completion_claimed（source=model，summary 非空） |
| — | **硬证据（自证达成）** | **单个 session_started** + goal_achieved.reason=satisfied + 最终 checkpoint detail 含 model-certified；目标产物真实存在（eyeball） |

**通过标准**:无 verifier 的 goal 可达成（对照 INC-10 前的语义洞：恒不可
达成）；裁决只在静止边界（goal_completion_claimed 之后仍有 assistant turn
收尾才出 achieved）；不钉模型措辞。若模型一轮内直接完成并声明，
checkpoints 可为 1；若先 miss 再完成，miss checkpoint 的 feedback 应含
结构化 continuation（目标重述）。webui 侧同增量手验：`/goal 一句话` 直接
attach、banner 显示 self-certified、edit 改文本、达成后 banner 消失。
**结果**:2026-07-09 真实 Gemini PASS（脚本 `qa/run-qa17.sh`,共享
daemon/store）——session_started=1、goal_completion_claimed=1(source=
model)、checkpoint detail=model-certified、achieved=satisfied、haiku.txt
真实落盘。webui 真跑 PASS（Chrome 全流程:Home /goal 直启、self-certified
banner、goal_complete 时间线可见、达成 banner 消失;另 CLI 真验 update 作
废 claim + resume 注入再武装）。归档 `qa/runs/2026-07-09-QA-17/`。

## QA-18 MCP 生产接线与重连（INC-11.4,UJ-19）

**环境**：共享 daemon + 真实 Gemini；本机启动一个真实 MCP stdio server，
另启动一个 streamable HTTP server（要求 bearer/header）；agent spec 同时声明
两者，只用 `env_from` / `headers_from_env` / `oauth.access_token_env` 引用 secret。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar new` 要求调用 stdio tool | spec 自动接线；`ToolsDiscovered` 入 journal；结果回到模型 |
| 2 | 调 HTTP structured/image tool、resource read、prompt get | JSON 与图片/resource/prompt 内容块不被扁平化 |
| 3 | server 新增 tool 并发 `list_changed` | 下一安全边界产生新 `ToolsDiscovered`，新 tool 可见 |
| 4 | 在两次调用间终止并重启 server | 下一次操作重建 session，会话不终止 |
| 5 | 让伪 `readOnlyHint` tool 在 `ActivityStarted` 后模拟崩溃 | activity 为 `idempotent:false`，resume 拒绝静默重跑 |

**通过标准**：前台/daemon/resume 均无需代码注入 Manager；secret 值不出现于
spec/journal；断线中的当前调用若结果未知，模型只收到 `outcome_unknown`，
runtime 不自动重放。

## QA-19 Turn/Item 与 typed ingress（INC-11.5）

**环境**：共享 store + 真实 Gemini。前台 run 后再经 daemon send 一条带文件
的消息；发送命令显式设置 principal/source/trust（外部 connector 形态）。

**通过标准**：原始 journal 的新输入含稳定 turn_id/item_id、typed content
且 binary 仅有 CAS ref；CommandLog 同时保留 principal/source/trust；assistant、
tool_call、tool_result 投影到同一 active turn。`inspect --json` 的 turns/items
非零并显示 schema_version=1 的 provider capability envelope。用旧共享 session
执行 inspect/resume 时，旧 Message/GenStep 日志可补投影；旧 snapshot 不阻断。

## QA-22 microcompact 无 LLM 上下文回收（INC-13,UJ-09）

**环境**：私有 daemon（新构建 binary，隔离 runtime 根，因共享 daemon 正
服务其他并发 session）+ 真实 Gemini；session 关掉 compaction
（`compact_at_tokens: 0`）只留 `microcompact_at_tokens`，令 workspace 三个
文件各含一条尾行 codeword；跑完把 session 拷回共享 store、export 归档
`qa/runs/2026-07-09-QA22/`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | 连续读三个大文件、复读，跨过 micro 阈值 | journal 落 `context_microcompacted`（cleared>0），无 LLM 调用为它发生 |
| 2 | 全程 | **无** `context_compacted`（compaction 关闭，证明 micro 独立自足） |
| 3 | 追问最老文件（其读结果已被降级）的 codeword | 模型看到占位符后**重跑 read_file**（调用数 5→7）并答出确切密钥 |

**通过标准**：三条红线均为 journal/行为事实（不判模型措辞）；被降级的旧
结果在 journal 中仍是全量（fork/resume 语义不损）。

## QA-23 记忆写回：remember → 新 session 冻结生效（INC-14,UJ-09,G9）

**环境**：私有 daemon（隔离 runtime 根，共享 daemon 正服务其他并发
session）+ 真实 Gemini；两个 session 共用一个 workspace；跑完拷回共享
store、export 归档 `qa/runs/2026-07-09-QA23/`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | session 1 中 `ar remember <sid> "本项目一律用 pnpm，禁 npm/yarn"` | note 写入 `<ws>/CLAUDE.md` 的 `## Remembered` 段 |
| 2 | 同上 | session 1 journal 出现一条 `source:program` 的 input_received 携带该 note（本会话可见） |
| 3 | 起**全新** session 2（同 workspace），问"用哪个包管理器" | 模型答 **pnpm**——记忆经**下次 session 冻结进 prefix**到达模型（跨会话持久，本增量的靶心） |

**通过标准**：三条红线均为 journal/文件/行为事实；取 A（追加消息 +
文件持久化）不改冻结 prefix，故不触不变量；重复同 note 幂等（文件不
双写，防 durable-command 崩溃重放）。

## QA-24 hooks 生命周期事件族（INC-15,G19）

**环境**：私有 daemon（隔离 XDG_DATA_HOME/XDG_CONFIG_HOME）+ 真实
Gemini；hooks 配在 user 层 settings.yaml 的 `hooks.lifecycle`；跑完
session 拷回共享 store、export 与 hook 标记文件归档
`qa/runs/2026-07-09-QA24/`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar new` 起会话 | session_start hook 收到 `{"event":"session_start",…}` stdin 并写标记文件 |
| 2 | send 一条含 FORBIDDEN 的输入（hook 对其 exit 2） | 输入**不落 journal**（无新 input_received）、不起 turn、session 存活 |
| 3 | 再 send 一条正常输入 | 正常落 journal 并得到回答（veto 是 per-input 的） |
| 4 | 回答后静止 | stop hook 在静止时刻触发（stop.log 含 `"event":"stop"`） |

**通过标准**：四条红线均为文件/journal 事实（不判模型措辞）；observe
事件坏 hook 不改控制流；blockable 仅 user_prompt_submit/pre_compact。

## QA-25 逐段权限裁决（INC-16,#53）

**环境**：私有 daemon（隔离根）+ 真实 Gemini；spec 配 `Bash(git *)`
allow + `Bash(rm *)` deny + catch-all ask；workspace 内预置 victim.txt；
跑完 session 拷回共享 store、export 归档 `qa/runs/2026-07-09-QA25/`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | 让模型原样执行 `git status && rm -rf victim.txt`（一次 bash 调用） | **victim.txt 仍存在**——rm 段被逐段 deny，尽管 git 段 allow（整条不被 git-allow 放行）。**文件系统硬红线，不依赖模型措辞** |
| 2 | 让模型执行 `git --version` | activity_completed（git 段 allow 真执行——不是全 deny） |

**通过标准**：红线1是文件系统事实（victim 未删=逐段 deny 生效）；旧
整条匹配会让 git-allow 放行整条并删掉 victim。孪生（TestCompound*）已
覆盖拆分/wrapper/只读集与"显式 deny 先于只读集"安全序。

## QA-26 审批"允许且不再问"（INC-17,G5,UJ-08）

**环境**：私有 daemon + 私有 XDG_CONFIG_HOME（隔离写回的 user 配置，不
碰真实 ~/.config）+ 真实 Gemini；spec 配 catch-all ask；跑完两个 session
拷回共享 store、export 与写回的 user-settings 归档 `qa/runs/2026-07-09-QA26/`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | session 1 让模型跑 `date`（catch-all ask） | journal 出现 approval_requested（ask 生效） |
| 2 | `ar approve <sid> <apid> approve --always` | user 配置追加一条该命令的**精确** allow 规则（**文件事实**——"不再问"的实质） |
| 3 | 起**全新** session 2（同 user 配置），跑同命令 | **无** approval_requested、命令直接执行（记住的规则生效，下次不再问） |

**通过标准**：三条红线均为 journal/文件事实；取 A（写文件、下次生效）
不动本 run 冻结 layers；精确匹配（`date` 记住不放宽到别的命令）。真机
QA 捕获并修一个 persist 主路径漏传 Remember 的 bug。

## QA-27 Web UI 产品化重构（INC-19,UJ-24）

**环境**：真实共享 `~/.local/share/agentrunner/` daemon/store + 当前
`main` Web UI，`http://127.0.0.1:8788`；未隔离 HOME/XDG，未删除或关闭
任何 session/workspace/journal。归档 `qa/runs/2026-07-09-QA27/`。

| # | 真实状态/动作 | 硬断言 |
|---|---|---|
| 1 | Home / Scheduled / Projects | Home 恰一个 New session 主操作和 composer；真实历史按 workspace 分组；Scheduled runs 空态明确 |
| 2 | deep link/reload/Web UI restart | 父/子/CLI 创建的 session 仍可按 hash 恢复；workspace/title 来自 journal-backed `sessions --json` |
| 3 | waiting:approval `20260709-134832-use-bash-to-echo-notify-test-7bca` | 卡片显示 Run command / echo B / Current workspace，Details 默认折叠；未代用户决策 |
| 4 | existing team session | Supervision 中 engineer/reviewer 各恰一行；点 engineer 进入完整只读子会话（textbox=0） |
| 5 | diff `20260710-030410-use-write-file-to-create-webui-4a0c` | Changes 显示真实 workspace、`webui_qa.txt +1` 与 `WEBUI-QA-DIFF-OK` |
| 6 | 1554×1014 + 900/700 响应式与 console | thread/composer/Supervision 可用；console error/warning=0 |

**结果**：PASS。截图、同图 design 对照、console/DOM 断言、原始 journal
副本与 workspace diff 均在归档目录；所有测试数据保留。
## QA-28 protected paths 写保护（INC-18,#59,UJ-08）

**环境**：私有 daemon（隔离 runtime 根）+ 真实 Gemini；spec `mode:
acceptEdits`、无 permission 规则（mode default 治理）；workspace 预置一个
普通文件与一个 `.mcp.json`；跑完 session 拷回共享 store、export 归档
`qa/runs/2026-07-09-QA28/`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | acceptEdits 下让模型改 `normal.txt` | 自动放行、编辑落地（无 approval_requested） |
| 2 | 让模型改 `.mcp.json`（protected） | **不**自动放行——journal 出 `approval_requested`（未审批不执行） |
| 3 | 审批 pending 期间 | `.mcp.json` **文件内容未变**（写被拦在审批前，文件系统硬证据） |

**通过标准**：三红线均为 journal/文件事实；protected 只收紧 acceptEdits
自动放行（bypass/显式规则/hardFloor 不变，`.claude/worktrees` carve-out）。

## QA-29 skill 模型侧 invoke（INC-20,#45/§3.5,UJ-19）

**环境**：私有 daemon（隔离 runtime 根）+ 真实 Gemini；workspace 放一个
`greet` skill（SKILL.md 指令要求回复含固定暗号），spec tools 含 `skill`；
跑完 session 拷回共享 store、export 归档 `qa/runs/2026-07-09-QA29/`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | 让模型「用 greet 技能打招呼」 | journal 出 `skill` tool_call，name=greet（模型按 name invoke，非 read_file path） |
| 2 | skill 工具返回 | tool_result 含 SKILL.md **正文**（暗号指令），**不含** frontmatter（description 行不泄漏） |
| 3 | 模型最终回复 | 遵循 skill 指令——回复含 SKILL.md 要求的暗号 |

**通过标准**：三红线均为 journal 事实；skill 是 read-class（免审批同
read_file）但 name 防遍历 + WS 边界；维持命令=用户宏裁决不动。

## QA-30 grep 参数增强（INC-22,#35,UJ-01）

**环境**：私有 daemon（隔离 runtime 根）+ 真实 Gemini；workspace 放
`src/*.go`（含大小写混合的 TODO/todo 标记）与 `docs/n.md`；spec tools
含 grep；system prompt 告知 grep 的新参数。跑完 session 拷回共享 store、
export 归档 `qa/runs/2026-07-09-QA30/`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | 让模型「统计每个 .go 文件的 TODO（忽略大小写）」 | journal 出 grep tool_call 带 `output_mode`/`glob`/`case_insensitive` 至少一个新参数 |
| 2 | grep 返回 | tool_result 用新 shape（`counts`/`files` 数组）或 case_insensitive 生效（命中小写 todo） |
| 3 | 模型最终作答 | 搜索完成、答案给出（glob *.go 排除 .md） |

**通过标准**：默认参数=旧行为（现有 grep 测试不破）；新参数无状态纯
扩展。

## QA-31 grep context lines（INC-24,#35 余项,UJ-01）

**环境**：私有 daemon（隔离 runtime 根）+ 真实 Gemini；workspace 放一个
含 `// PIVOT` 标记行的 handler.go；spec tools 含 grep；system prompt 告知
grep -A/-B/-C。跑完 session 拷回共享 store、export 归档
`qa/runs/2026-07-09-QA31/`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | 让模型「用 -C 2 看 PIVOT 行前后两行」 | grep tool_call 带 `-A`/`-B`/`-C` 至少一个 |
| 2 | grep 返回 | tool_result 带 `before`/`after` 上下文数组 |
| 3 | 模型作答 | 反映上下文（PIVOT 前后的 validate/persist 调用） |

**通过标准**：默认无 context = 旧行为；context 行受 redaction/截断/文件
边界钳制；files/count 模式忽略 context。

## QA-32 内置只读 agent 库 spawn（INC-25,#78,UJ-18）

（补登 2026-07-10：场景于 INC-25 收口时真机执行并记 LOG,菜单当时漏登——
lint-docs 幻影锚检查抓出,见 LOG 2026-07-10 复盘条目。）

**环境**：私有**新二进制** daemon + 真实 Gemini；spec `agents:` 只列名
`explore`,workspace 无同名 sibling spec 文件；workspace 放一个含常量
`512` 的源文件。（本场景首跑踩出"共享 daemon 跑旧二进制致假失败"，
已固化为 QA 通则：新 daemon-path 功能一律私有新二进制跑。）

| # | 动作 | 验证 |
|---|---|---|
| 1 | 让模型 spawn 内置 explore 查常量值 | 无 sibling spec 文件仍成功起子会话（embed 解析） |
| 2 | 子会话执行 | 只读面：无 write 类工具,以 read/grep 完成 |
| 3 | 返值回父 | 父收到正确常量值 512 |

**结果**：PASS（LOG INC-25 条目）。

## QA-33 结构化输出 fallback 端到端（INC-26 #91→PLAN 5.7 重裁,UJ-01）

（补登 2026-07-10;2026-07-19 PLAN 5.7 重裁:--json-schema flag 退役,
spec output_schema 成为单入口——本场景改测**非原生 provider(anthropic)
上 spec output_schema 自动触发客户端 validate-and-retry fallback**。
gemini 原生路径由 QA-39 覆盖。旧版 PASS(2026-07-10,LOG INC-26)对应
旧 flag 形态,重裁后待真机复验。）

**环境**：真实 Anthropic(claude-haiku);workspace 放 7 行 `sample.txt`;
spec 内 output_schema 约束 `{lines:int, name:string}`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar new`(无任何 schema flag)让模型数行 | fallback 自动接管:回复为符合 schema 的 JSON,客户端校验通过 |
| 2 | 输出 | canonical structured_output 打印 `{"lines":7,"name":"sample.txt"}` |
| 3 | 独立复核 | python 独立确认 schema 符合且值合理（name~sample、lines=7） |

**结果**：重裁后待跑（闸门 B 挂账）。

## QA-35 grep multiline（INC-27,#35 余项,UJ-01）

**环境**：私有 daemon（隔离 runtime 根,新二进制）+ 真实 Gemini；workspace
放一个含多行 `computeTotal` 函数体的 `billing.go`；spec tools 含 grep；
system prompt 告知 grep 的 multiline 参数。跑完 session 拷回共享 store、
export 归档 `qa/runs/2026-07-09-QA35/`。（编号让路：QA-34 已被 INC-23.B6
webui 证据占用。）

| # | 动作 | 验证 |
|---|---|---|
| 1 | 让模型「用 multiline 一次抓取整个 computeTotal 函数体」 | grep tool_call 带 `"multiline":true` |
| 2 | grep 返回 | 某 match 的 text 跨行（含嵌入换行,横跨 func 签名到 return） |
| 3 | 模型作答 | 反映跨行结构（函数行数/循环体对 sum 的操作） |

**通过标准**：默认 multiline=false=旧逐行行为；`(?sm)` 使 `.` 跨行且 `^`/`$`
锚行；起始行号正确；上下文/cap/redaction 复用。

## QA-34 Web UI 黑盒 QA-fix 第二轮（INC-23,UJ-24）

**环境**：最新 `main`、共享 `~/.local/share/agentrunner/` daemon/store、
`http://127.0.0.1:8788`；Web UI 中途重启验证持久投影。未审批、resume、close、
删除或清理任何真实 session/workspace/journal。证据保留在
`qa/runs/2026-07-10-QA34/`。

| # | 真实状态/动作 | 硬断言 |
|---|---|---|
| 1 | existing stranded `20260710-020627-bash-f92e` | 799px 默认不展开 Supervision；header 有 Resume；状态/Attention 均为 recovery，不执行 resume |
| 2 | existing waiting:approval `20260710-043755-reply-with-exactly-standing-b-a1a8` | program continuation 不冒充用户；approval card 与 Attention 同为 1；未代决策 |
| 3 | existing team session | inspect 前不投影伪空态；稳定后 engineer/reviewer 各一行；program/agent messages 默认隐藏 |
| 4 | Web UI restart → Scheduled | 既有 driver 仍按 run/schedule/status 出现；driver 不在 Projects；scratch id 投影为 Scratch |
| 5 | New scheduled run / menu / search / Changes | 产品词在主层、YAML 在 Advanced；dialog/menu/button/focus/Escape/方向键成立；非 Git 空态与审批主层不泄漏绝对路径 |
| 6 | 1554×1012 / 799 / 680 + 同图对照 | Codex 三栏/thread/composer/Supervision 层级一致；移动端 sidebar 默认关闭、scrim 打开、导航后关闭；无 error overlay |

**结果**：PASS。黑盒先发现 P1×7/P2×若干，全部当轮修复；scripted tests、
16 个 frontend tests、frontend build、Web UI Go tests 与根 check 全绿；
最终同图对照为 `29-reference-vs-latest.png`。

## QA-36 Web UI UX Round 3（INC-29,UJ-24）

**环境**：最新 `main`、共享 `~/.local/share/agentrunner/` daemon/store、
`http://127.0.0.1:8788`，1554×1012；light/dark 都走。只读 existing
approval/team/recovery session，未审批、resume、close 或清理。证据保留在
`qa/runs/2026-07-10-QA36/`。

| # | 真实状态/动作 | 硬断言 |
|---|---|---|
| 1 | approval → Supervision → Run details | 首层为 status/waiting/overview/usage/activity/provider；CLI `answer_with` 不可见；raw data 默认折叠 |
| 2 | revived team → Run details | engineer/reviewer 与详情均去重为 2；普通 waiting:input 不伪装 Attention；黑盒发现并修复初版 4/false-wait |
| 3 | stranded → Run details | 使用 restart-aware session status 显示 Needs recovery，不被 inspect 的 stale running 覆盖；黑盒发现并修复初版状态撒谎 |
| 4 | 多个同前缀 bash/reply 任务 | sidebar 首屏直接显示 `touch concurrent-{1..4}.txt` / `Reply · …`，完整原题仍在 tooltip；manual rename 优先 |
| 5 | 状态色 + Codex 同图 | running=绿、ready/unread=蓝、approval/recovery=琥珀、failed=红、terminal=灰；dark token 同语义；console error=0；同尺寸对照无 P0/P1/P2 |

**结果**：PASS。QA-fix 当轮关闭 agent 重复计数、普通 input false-attention、
recovery stale status 三个信息真实性缺陷；23 个 frontend tests、frontend
build、Web UI/根 check 全绿。最终同图对照 `07-reference-vs-latest.png`。

---

## QA-37 skills context:fork 一次性子 agent（INC-31,UJ-19）

（补登 2026-07-10：场景于 INC-31 收口时真机执行并记 LOG,菜单当时漏登——
lint-docs 幻影锚检查抓出。）

**环境**：私有**新二进制** daemon + 真实 Gemini；skill 带 `context: fork`
frontmatter 与 FORK-MARKER 正文；workspace 可数 widget 的源文件。

| # | 动作 | 七红线断言 |
|---|---|---|
| 1 | 用户消息触发 fork skill | ingest 展开为 `spawn_agent{role}` 入 journal |
| 2 | spawn | SpawnRequested Agent=skill 名;冻结 RoleSpec 载正文（FORK-MARKER）与 allowed-tools |
| 3 | 子会话执行 | 跑出 `WIDGET-COUNT: 4` |
| 4 | 收口 | receipt 回父,父答 4 |

**结果**：PASS 七红线全过（LOG INC-31 条目）。

## QA-38 read_file 读图（INC-33,UJ-02/05）

（补登 2026-07-10：场景于 INC-33 收口时真机执行并记 LOG,菜单当时漏登——
lint-docs 幻影锚检查抓出。）

**环境**：私有**新二进制** daemon + 真实 Gemini；workspace 放一张含
可辨识文本（文件名/数字/标识符）的截图 PNG。

| # | 动作 | 四红线断言 |
|---|---|---|
| 1 | 让模型 read_file 该 PNG | media envelope 入 journal,journal 恒 byte-free（最长行 2056B 无 blob） |
| 2 | CAS | ref 字节精确 |
| 3 | 第二次请求 | tool_result 后跟 inflate 的 image part,块序正确 |
| 4 | 模型作答 | **从像素读出截图内容**（command.go/1234/EnableTraverseRunHooks） |

**结果**：PASS 四红线全过（LOG INC-33 条目）。

## QA-39 provider-native 结构化输出（INC-35,#8b,UJ-01）

（补登 2026-07-10：场景于 INC-35 收口时真机执行并记 LOG,菜单当时漏登——
lint-docs 幻影锚检查抓出。）

**环境**：私有**新二进制** daemon + 真实 Gemini；spec 声明
`output_schema`；workspace 放 5 行 `report.txt`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | 模型按 schema 作答 | gemini 原生约束生效：`raw_json=True` 裸 JSON（无 ```json fences） |
| 2 | 输出 | `{lines:5,name:report.txt}` 单轮免 re-prompt（对比 QA-33 的 CLI 重试路径） |
| 3 | 工具面 | structuredOnly 抑制全部自动加工具（send_message 等）,tool-less 轮可达 |

**结果**：首跑暴露 Router 自动加 send_message 使 `len(Tools)==0` 门永不
触发（fences 复现）→ structuredOnly 修复后重跑 PASS（LOG INC-35 条目）。

## QA-44 mode 运行中切换（INC-42,G29,UJ-06）

**环境**：私有**新二进制** daemon + 真实 Gemini；spec 默认 mode、
`permissions: []`（mode default 治理）；workspace 预置 normal.txt 与
`.mcp.json`。脚本 `qa/run-qa44.sh <ar-binary>`。跑完 session 拷回共享
store、export 归档 `qa/runs/<日期>-QA44/`。webui 腿：新 arwebui 指向
同一私有栈（`-no-daemon -ar <新 ar>`），playwright + 系统 Chrome 走
真用户流。

| # | 动作 | 验证 |
|---|---|---|
| 1 | default 下让模型改 normal.txt | approval_requested；pending 期文件未变；deny 收轮 |
| 2 | `ar mode <sid> acceptEdits`（idle） | journal mode_changed{to:acceptEdits,cause:user} |
| 3 | 同类 edit 再来 | 免审直落、文件真实变更；approval_requested 计数不增 |
| 4 | 编辑 `.mcp.json`（protected） | 仍 ask；pending 期文件未变（INC-18 不松） |
| 5 | `ar mode <sid> bypass` | CLI 拒绝（非零退出）；journal 无新 mode_changed |
| 6 | `ar mode <sid> default` → 再 edit | mode_changed{to:default}；edit 重新 ask |
| 7 | webui：composer `/mode acceptEdits` → `/mode default` | toast 投递 ack；pill 诚实序 unknown→"Auto-accept edits"→unknown；system-events chips 逐条出现；截图 webui-1/2/3 |

**通过标准**：六红线全为 journal/文件/退出码事实，不钉模型措辞；pill
只对确定 mode（acceptEdits/plan）声称身份，default 显示诚实 unknown
（QA Round1 F-C3 规则）。
**首跑结果（2026-07-10）**：CLI 6/6 + webui 全绿；session
`20260711-025146-normal-txt-alpha-4368` 留共享 store，归档
`qa/runs/2026-07-10-QA44/`。

## QA-51 session mode pill 点击切换（INC-58,G29,UJ-06）

**环境**：共享 `~/.local/share/agentrunner/` daemon/store + 真实 Gemini；
webui 8809（launchd 守护，强刷载新 bundle）；运行中的真实 session。验证的是
INC-42 已通链路的**新点击入口**——pill 由只读 badge 变为 Popover 选择器。
证据（截图 + `ar events` 导出，`-a` 防截断）归档
`qa/runs/2026-07-11-QA-51/`；测试 session 保留不清理。

| # | 动作 | 验证 |
|---|---|---|
| 1 | 运行中 session composer 点 mode pill | Popover 开；列 Ask/Auto-accept（可点）、Full/Plan（disabled，desc 说明为何）；当前档按真值序高亮或诚实 unknown |
| 2 | 点「Auto-accept edits」 | toast 投递 ack；system-events chip `Mode changed · acceptEdits (user)`；pill 随 2.5s 轮询更新为 Auto-accept edits |
| 3 | 让模型改一个文件 | 免审直落、文件真实变更；无 approval_requested |
| 4 | 点「Ask to approve」切回 | chip `Mode changed · default (user)`；pill 回诚实序 |
| 5 | 切回后让模型再改文件 | approval_requested 回归；pending 期文件未变 |
| 6 | Full/Plan disabled 行点击 | 无动作（不发命令）；desc 解释启动期/审批退出 |

**通过标准**：切换走与 `/mode` 同一条 `ControlMode` durable command（chip/
文件/审批为事实锚，不钉模型措辞）；disabled 档不可 runtime 进入；被拒切换
落 rejected receipt chip（用户可见）。
**结果（2026-07-11）**：PASS。共享 daemon（含 INC-42）+ 真 Gemini，webui 8809
（部署版本 `8d3bd60-010922`）强刷后真用户浏览器流。六红线全绿——journal 事实
`approval_requested`(default) → `mode_changed{acceptEdits,user}`(pill 点击) →
`edit_file{GAMMA_AUTO}` 无审批(auto-accept) → `mode_changed{default,user}`(pill
点击) → `approval_requested`(审批回归)；菜单 Full/Plan disabled 带原因、active
以 ✓ 跟随 live mode、live 未知时诚实 "Access: set by agent spec"。session
`20260711-081456-ready-8e69` + `20260711-081124-ready-8ef8` 留共享 store，归档
`qa/runs/2026-07-11-QA-51/`（EVIDENCE.md + 2 份 events + normal.txt.final）。

## QA-41 Codex 式任务收尾与首屏真相（INC-38,UJ-24）

**环境**：最新 `main`、共享 `~/.local/share/agentrunner/` store/daemon、
`http://127.0.0.1:8788`；默认 desktop 与 390×844。只读既有 session/diff，
未 send/approve/resume/close/commit/清理。证据保留在
`qa/runs/2026-07-10-webui-codex-detail-audit/`。

| # | 真实状态/动作 | 硬断言 |
|---|---|---|
| 1 | Web UI 不可用→恢复、deep link 首屏 | 首个 sessions success 前只允许 loading，不投影空列表或 raw sid；header 先 `Loading session…` |
| 2 | completed session `…qa-k-diff…` | journal timestamp 投影 `Worked for 9s`；最终 answer 只有一条 Worked；Copy 与 `Continue in new session` 可达 |
| 3 | 有真实 diff 的 completed goal session `20260710-062102-…-0d1e` | `Worked for 1m 17s`；内联 `Edited 1 file +1 / goal-r2.txt +1`；Review 进入原 Changes 并显示 `+DONE` |
| 4 | New session desktop / 390×844 | project/Local/branch 环境条常驻 composer 上缘；同一 trigger 打开 Recent/workspace/Interactive/Background/branch；mobile 不溢出 |
| 5 | sidebar session | 每行具 pin/archive hover action；hover preview 由 session workspace/status 与按需 git branch 查询组成；键盘 context menu 原路径不变 |
| 6 | Codex 参考图同屏对照 + console | Worked/action/Changes card 与 environment strip 层级对齐；AgentRunner 品牌/Supervision 保留；console error/warning=0 |

**结果**：PASS。26 个 frontend tests、frontend build 与根 `check.sh` 全绿；
desktop/mobile 菜单、Review 路径均用 DOM 断言；测试数据全部保留。

---

## QA-42 Codex composer 行为同构与真实 New worktree（INC-40,UJ-24）

**环境**：本机安装的 ChatGPT/Codex `app.asar` 源码模块 + 用户提供的 Codex
截图为基准；最新 `main`、保留的 Web UI runtime/store、
`http://127.0.0.1:8788`；desktop、642px、390×844。证据与提取模块保留在
`qa/runs/2026-07-10-QA42-codex-ux-full/`，真实 session/worktree 不清理。

| # | 真实状态/动作 | 硬断言 |
|---|---|---|
| 1 | Codex 安装包源码取证 | Project、Local/New worktree、Local environment、Branch 是四个独立控件；Project picker 有 search/selected/New project/projectless；Branch 独立搜索 |
| 2 | New session desktop / 642px / 390×844 | 四个 popover 独立打开；Project/Branch 搜索、selected check、Escape focus return、ArrowDown/Home/End 与 viewport containment 成立；access/model/mic/send 均在视口内 |
| 3 | Project=`agentrunner`、New worktree、Branch=`main` 提交真实任务 | 后端从 selected ref 创建 `wt-20260710-143427`；session `20260710-213428-create-qa42-worktree-browser-t-d8ac` 完成，唯一改动 `qa42-worktree-browser.txt`，内容严格为 `QA42_WORKTREE_OK` |
| 4 | 390×844 approval | inline approval 是主操作；Supervision 不自动覆盖；从宽屏 resize 到窄屏同样撤回自动面板 |
| 5 | completed → Changes → Continue in new session | `Worked for 3m 46s` 在最终 answer 前；Changes 显示文件和 `QA42_WORKTREE_OK` 且无 Supervision；Continue 对话框可选择 checkpoint，原 session 不变 |
| 6 | Web UI restart + deep link reload + console | 同一 session/approval/完成态均恢复；console error/warning=0；browser 与 shell 均确认 worktree 只有目标文件未跟踪 |

**结果**：PASS。26 个 frontend tests、frontend build、Web UI Go tests 与根
`check.sh` 全绿；发现并修复 mobile composer 裁切、branch 缺搜索、resize 后
Supervision 覆盖审批三项真实浏览器缺陷。

---

## QA-45 运行中发消息投递模式 steer|queue（INC-43,UJ-07）

**环境**：最新 `main`、共享 `~/.local/share/agentrunner/` store/daemon、真实
Gemini 模型、`http://127.0.0.1:8788`（webui 强刷）。趁 turn 运行中分别用
queue 与 steer 发消息，观察注入时机。证据（`ar events` 导出 + journal grep
-a）归档 `qa/runs/2026-07-10-QA-45/`；测试 session 保留不清理。

| # | 真实状态/动作 | 硬断言 |
|---|---|---|
| 1 | 起真实模型 session，发一条要求多步工作的 prompt（turn 运行中） | session 进入 running；journal 见 generation_started/activity_started |
| 2 | 运行中 `ar send --steer <sid> "..."`（或 webui Steer） | steer 消息的 `input_received` seq 落在**当前 turn 收尾 assistant 之前**——模型本 turn 内看到并回应 |
| 3 | 另起一 turn，运行中 queue 发消息（默认，无 --steer / webui Queue） | queue 消息的 `input_received` seq 落在**当前 turn 收尾之后**，另起新 turn 消费 |
| 4 | webui composer 复核（强刷后） | 运行中出现 `Queue|Steer` 切换；⌘⏎ 反选；pending bubble 标注 steering…/queued… |
| 5 | durable 幂等 | 同 command_id 重发不双改；journal 无重复 input_received |

**环境补充**：因新增 `Delivery` 命令字段，共享 daemon 跑的旧二进制会静默丢弃它
（假失败），故按既定纪律用**私有 daemon 跑本增量新二进制**（`ar-inc43`，
`XDG_DATA_HOME=/tmp/claude-501/inc43-qa/data`，真实 Gemini `gemini-flash-latest`）；
store 全程保留，未清理。

**结果**：PASS（真机 Gemini）。同一注入时机（首个 bash 工具运行中），仅
`--steer` 旗标差异：
- **steer**：注入消息 `input_received` 落 **seq 10**——mid-turn（本 turn 收尾
  assistant `READY` 在 seq 49、首个 `waiting_entered` 在 seq 51 之前），turn 内
  gen-step 1→4 连续跑，模型本 turn 内看到。
- **queue**：`input_received` 落 **seq 51**——首个 turn 收尾（idle `waiting_entered`
  seq 50）**之后**，由新 turn（gen-step 5）消费并真跑 `echo INJECT_queue_TOKEN`。
- **webui**（强刷后真机截图）：运行中 composer 显示 `Queue|Steer` 段控件、默认
  Queue、点 Steer 高亮、send 提示 `Send · steer (⌘⏎ to queue)`；console error/warn=0。
证据：`qa/runs/2026-07-10-QA-45/`（steer-events.txt / queue-events.txt / 两张
composer 截图 / driver logs / spec.yaml）；孪生四条 + 91 vitest + build + 根
check.sh 全绿。

---

## QA-48 in-session LLM goal judge（INC-48,#8,UJ-22）

**环境**：本增量新二进制（daemon 路径新功能 → 按既定纪律私有 daemon +
`XDG_DATA_HOME` 私有 store，真实 Gemini），跑完 store 保留归档
`qa/runs/2026-07-10-QA-48/`。

| # | 真实状态/动作 | 硬断言 |
|---|---|---|
| 1 | 起真实模型 session，挂无命令 goal：`ar goal <sid> attach --verify-llm "<rubric>" <goal…>` | journal `goal_attached` 带 `{"kind":"llm_judge","rubric":...}` |
| 2 | 模型完成工作并调 `goal_complete` | journal `goal_completion_claimed`；claim 边界出现 `verifier:llm_judge` Activity（真 Gemini llm_call） |
| 3 | judge 裁决 | `goal_checkpoint` detail 带 judge reason；pass → `goal_achieved{satisfied}` |
| 4 | claim-gated 校验 | 无 claim 的边界**无** judge Activity（journal 里 judge Activity 数 ≤ claim 数） |
| 5 | judge 驳回续跑 | rubric 严于 prompt（要求模型没被告知的 CHANGELOG.md）→ 第一次 claim 被驳回、reason 指出缺项 → continuation 回灌 → 模型补齐 → 二次 claim pass |

**结果**：PASS（真机 Gemini，`qa/run-qa48.sh` + 驳回场景，2026-07-11）。
- **主场景**（`…-063736-…-0ba7`，events.jsonl）：goal_attached 带
  llm_judge rubric → 模型建 greeting.txt/VERSION → `goal_complete` →
  真 Gemini judge Activity（`verifier:llm_judge`）→ checkpoint detail
  "judge: The agent successfully created greeting.txt with a 13-word
  welcoming message…" → `goal_achieved{satisfied}`；judge 1 次 = claim
  1 次（claim-gated）；workspace 文件实测在盘。
- **驳回场景**（`…-063844-…-b7cd`，events-reject.jsonl）：goal 文本只说
  "发布准备"，rubric 硬要求 app.txt+CHANGELOG.md → claim 1 被驳
  "Workspace is missing CHANGELOG.md…"（check=1 pass=false）→ 反馈回灌
  → 模型补建 CHANGELOG.md → claim 2 判 pass（check=2）→
  `goal_achieved{satisfied,checks:2}`；claims=2 judges=2。
归档 `qa/runs/2026-07-10-QA-48/`；session 拷回共享 store 保留。

---

## QA-58 cron 跨重启唤醒 + boot sweep（INC-54,HANDA #28b,G22,UJ-14）

**环境**：私有新二进制 daemon + 隔离 XDG_DATA_HOME + 真 Gemini（子迭代
trivial）。脚本 `qa/run-qa58.sh <ar>`。长（~3-4min）：cron `* * * * *`。

| # | 动作 | 硬断言 |
|---|---|---|
| 1 | 本地 `ar drive` cron 跑 ≥1 迭代 → `kill -9`（崩溃，非优雅） | ≥1 `iteration_completed`；drive 进程被杀 |
| 2 | sleep 140s 隔过 ≥2 个 cron slot | —（漏 slot） |
| 3 | 启一个 daemon → boot sweep 扫 store 重挂孤立 drive | 每漏掉的 slot 恰一条 `iteration_skipped`（overlap=skip）或一次 coalesce catch-up；cadence 恢复 |

**结果**：PASS（2026-07-11，session 20260711-091031-qa58cron-b0fc）。崩溃前
1 迭代完成 → kill -9 → 隔 140s → **新 daemon boot sweep 重挂该 drive，漏掉
的 2 个 slot 各恰一条 `iteration_skipped`（skipped=2）**，drive 续跑。G22
crash-restart 支真机实证（补跑恰一次、不重复不丢）。归档
`qa/runs/2026-07-11-INC54/`。锚孪生（A 闸绿）：TestDriverCronBackfills /
Coalesces / ResumeIsIdempotent、TestBootSweep{Resumes,SkipsHosted,NoSideEffect,SkipsMarked}、
TestScanDriveSessionsGate。余项：优雅停机保活 cron（terminal 语义变更，
G22 注 b，另立增量走 §四）；agent session mid-turn 自动接续（G22 注 a）。

---

## QA-59 自定义 command tools（INC-55,HANDA #4,UJ-19,决策 #19/#34）

**状态**：PASS（2026-07-11，脚本 `qa/run-qa59.sh`，隔离 XDG 私有二进制）。
**环境**：私有二进制 + 隔离 XDG_CONFIG_HOME/XDG_DATA_HOME + 真实 Gemini；
归档 `qa/runs/2026-07-11-INC55/`。

| # | 动作 | 硬断言（runtime 红线，不钉模型措辞） |
|---|---|---|
| 1 | user 层置 `~/.config/agentrunner/tools/wordcount.json`（`{"name":"wordcount","description":"count words in the given text","command":"wc -w","params":{"type":"object","properties":{"text":{"type":"string"}}}}`；`wc -w` 读 stdin=args JSON）；起会话让模型"用 wordcount 数一段文字的词数" | 模型 face 见 `wordcount` 并调用；journal 出现 execute-class `EffectResolved`（含 `Containment{filesystem:workspace,backend}`）；`ar events` 见工具结果、`ar inspect` 见该调用 |
| 2 | 在**未 trust** 的 workspace 放 `<ws>/.claude/tools/x.json`，起会话 | 日志/stderr 见 untrusted 告警；`SessionStarted.command_tools` 不含 `x`；模型 face 无 `x`。`ar trust <ws>` 后重起：`x` 出现 |
| 3 | 置 `~/.config/agentrunner/tools/bash.json`（name=`bash`） | 告警 "collides with a built-in tool"；内置 `bash` 仍在；自定义未覆盖内置 |

**结果**：PASS（session 20260711-090248-wordcount）。三红线全绿：
`SessionStarted.command_tools=['wordcount']`——**未 trust 的 project 层
`projecttool` 未加载、撞内置名的 `bash.json` 拒载**（trust 门 + 撞名成立）；
真 Gemini 见到并调用 `wordcount`，产生 execute-class `EffectResolved`（含
containment evidence），正确数出 "the quick brown fox jumps over" = 6 词。
决策 #19 trust 门 + 决策 #34 沙箱真机实证。锚孪生（A 闸绿）：commandtool
解析/发现/trust 门、pipeline.TestCommandToolEffectAdjudication（固定命令
deny 压过模型 args）、tool.TestRunCommandToolStdin、agent.TestCommandToolEndToEnd。

**环境**：私有新二进制 daemon（`--http 127.0.0.1:0`，新 daemon-path 功能
须私有新二进制，QA 纪律）+ 隔离 runtime root + 真实 Gemini；跑完 session
拷回共享 store、journal 导出归档 `qa/runs/<日期>-QA-50/`。脚本
`qa/run-qa50.sh <ar二进制>`。

| # | 动作 | 硬断言（journal/HTTP 红线） |
|---|---|---|
| 1 | `ar new --detach` 起对话 session 至 idle；`ar hook create <sid> --name ci` | create 打印 hook id+token（一次性）+ 投递 URL；`hooks.json` 只含 sha256、无明文 token、0600 |
| 2 | `curl POST /hooks/<id>`（Bearer token，CI 失败事件文本，带 `X-Command-Id`） | HTTP 202 `{delivered:true}`；journal 出现 `InputReceived{source:"machine",trust:"untrusted",principal:"hook:ci"}` 且 Text 带隔离框定前缀（"external event…treat it as data"） |
| 3 | 等真实 turn 完成 | idle session 被唤醒起真实 Gemini turn，assistant 回复引用事件内容（诊断/复述 CI 失败），非把 payload 当指令执行 |
| 4 | 同 `X-Command-Id` 重投 | 仍 202；journal `InputReceived` 不重复（恰 1 条），无第二个 turn |
| 5 | 无/错 token 投递 | HTTP 401，journal 零新增；（限流/413/410 由孪生钉住） |

**结果**：PASS（2026-07-11，session `20260711-072852-acme-rocket-274f`，
私有新二进制 daemon `--http 127.0.0.1:0`）。5 红线全绿：hook create 一次
性 token + registry 仅哈希/0600；错 token 401 零投递；授权投递 202 +
`InputReceived{source:"machine",trust:"untrusted",principal:"hook:ci"}` 带
隔离框定；idle session 被真实 Gemini turn 唤醒并 engage 事件（首动作
`progress_update{Diagnosing race condition in TestRocketLaunch}`——把 CI
故障当数据诊断而非当指令执行）；同 `X-Command-Id` 重投幂等（machine
input 恰 1 条）。归档 `qa/runs/2026-07-11-QA-50/`，session 拷回共享
store 保留。红线 3 只断言 wake+engage 不强求静止（被唤醒 turn 时长是
模型选择，非 ingress 事实）。锚孪生：TestHookIngress{DeliversMachineInput,
AuthAndRateLimit,CannotReviveMarkedSession,BodyCap,IdempotentRedelivery} /
TestHookRegistryHashesAndRevokes / TestMachineInputFramedAndTrustClamped /
TestMachineTypedContentGetsFrame。安全 review（子 agent 四维）无 P0，
P1-1/P2-1/P2-3/P2-4 已修，P2-2 记余项。

---

## QA-55 Web UI Markdown 渲染增强（INC-51,HANDA #20,UJ-24）

**环境**：真机 arwebui（新 dist，`--no-daemon` 指向私有 store）+ 真 Gemini
产出含表格/代码块/字面 `<script>` 的会话 + 真 Chrome DOM 断言。

| # | 动作 | 硬断言（真浏览器 DOM） |
|---|---|---|
| 1 | 真 Gemini 输出 GFM 表格（Name/Role 两列 Alice/Bob 两行） | `<table>` 渲染，表头 [Name,Role]、单元格含 Alice/Bob/Admin；可滚动包裹 |
| 2 | 输出 python 代码块 `print("hello")` | 代码块出 `[class*="hljs-"]` 着色 span（highlight.js），关键字着色 |
| 3 | 正文含字面 `<script>alert(1)</script>` | **无注入 script 元素**（`querySelectorAll('script')` 无 alert(1)）、字面文本可见、无 `img[onerror]`——禁 raw HTML 红线 |
| 4 | 代码块 header | line-wrap 开关按钮存在 |

**结果**：PASS（2026-07-11，真 Chrome，session 20260711-083921-markd-7d8d）。
四红线全绿：GFM 表格（表头 Name/Role + Alice/Bob/Admin 单元格 + 可滚动包裹）；
2 个 hljs 高亮 span（python 关键字着色）；字面 `<script>` **无注入 script
元素、作可见文本渲染、无 img[onerror]**；wrap 开关按钮存在。同时旁证 INC-52
auto-title（sidebar 显示「Markdown 格式示例演示」）。锚孪生（A 闸绿）：
`Markdown.test.tsx`（表格/highlight/line-wrap/raw-HTML 转义安全断言）。

---

## QA-57 ar dictate + ar optimize（INC-56,HANDA #18/#19,UJ-01/02/04/24）

**环境**：真 Gemini + `ar` 一次性 provider 调用（不需 daemon，webui 薄壳经
ar）。脚本 `qa/run-qa57.sh <ar>`。dictate 用 macOS `say` 合成音频（`.aiff`
在 MIME 映射内）。

| # | 动作 | 硬断言 |
|---|---|---|
| 1 | `say -o note.aiff "...kubelet...cluster Artemis...rebase the auth branch"` → `ar dictate --context "..." note.aiff` | 转写非空、保留专有名词 kubelet/Artemis/rebase（provider `PartAudio`→Gemini inline_data 真转写） |
| 2 | `ar optimize --context "editing internal/auth..." "fix the thing that broke"` | 输出非空、**≠ 原草稿逐字**、更充实（LLM 真改写、领域感知） |

**结果**：PASS（2026-07-11）。dictate 转写「Please deploy the kubelet on
cluster Artemis, then rebase the auth branch」逐字保留专有名词；optimize 把
「fix the thing that broke」改写为「Investigate and fix the recently
introduced issue or bug that broke the token verification functionality in
internal/auth.」。归档 `qa/runs/2026-07-11-INC56/`。锚孪生（A 闸绿）：
TestToPartAudio / TestDictateEncodesAudioPartAndContext /
TestDictateRejectsOversizeAudio / TestHandleDictateRejectsNonUploadPath /
TestOptimizeRewritesDraft / TestOptimizeSurfacesProviderError /
TestHandleOptimizeForwardsAndGuardsDraft / slash.test / composerOptimize.test。

---

## QA-56 project overlay + 系统 launcher（INC-53,HANDA #24,UJ-24）

**环境**：真机 arwebui（`--no-daemon`，测试端口）+ 共享 store 真 workspace +
真实 HTTP。脚本 `qa/run-qa54.sh <arwebui> <ar>`。**真 `open -a` 不跑**（会
启动真 app，副作用；argv 构造由 Go 孪生 TestLaunchArgvWhitelist/
TestOpenLaunchesKnownWorkspace 覆盖），B 闸只钉新 OS-exec 面的安全红线 + overlay。

| # | 动作 | 硬断言 |
|---|---|---|
| 1 | `POST /api/open {app:"/bin/sh", workspace:<真>}` | HTTP 400，launch 不触发（app 白名单外拒绝） |
| 2 | `POST /api/open {app:"finder", workspace:"/etc"}` | HTTP 400，fail-closed（workspace 非 `ar sessions list` 已知成员即拒） |
| 3 | `POST /api/projects {workspace:<真>, displayName}` → `GET /api/projects` | 200；overlay 名落 `webui-meta.json`（原子）且列表暴露 |

**结果**：PASS（2026-07-11）。三红线全绿（off-whitelist app 400 / 任意
workspace 400 fail-closed / overlay 持久化 webui-meta.json + 暴露）。归档
`qa/runs/2026-07-11-INC53/`。锚孪生（A 闸绿）：TestLaunchArgvWhitelist /
TestOpenRejectsUnknownApp / TestOpenRejectsUnknownWorkspace /
TestOpenLaunchesKnownWorkspace / TestMetaStoreProjectOverlayRoundTrip /
TestMetaStoreLoadsLegacyFlatFile。

---

## QA-53 LLM 自动会话标题（INC-52,HANDA #14,UJ-24）

**环境**：共享 daemon + store + 真实 webui + 真实 Gemini（auto-title 仅顶层
托管 session 启用，须走 daemon 而非 headless 一次性 run）。跑完**不** close/
删除；`ar events <sid>` 导出与 webui 截图归档 `qa/runs/<日期>-INC52/`。

| # | 动作 | 硬断言（journal/UI 红线） |
|---|---|---|
| 1 | webui New session 发一条**长**多行 prompt（>48 字符），等首条回复 | 开局回复**不被延迟**（title 调用在 assistant 消息之后的安全边界，异步于开局回复） |
| 2 | `ar events <sid>` | 出现恰一条 `session_titled{source:"auto"}` + 一条 `activity{kind:llm,name:autotitle}`（usage 计入 budget）；title 非首行截断而是精简短句 |
| 3 | webui 侧栏 | 该会话显示精简短标题（`sessions list --json` 的 title = RawTitle）；旧 session（无事件）仍显示首行/派生 fallback |
| 4 | webui 手动 rename 该会话 → 触发再次静止/唤醒 | 标题变手动值；`session_titled` 不新增第二条；auto **不覆盖** manual（rename 仍 localStorage，displayTitle 胜出） |
| 5 | 断网/坏 key 复跑一条长 prompt | 会话正常完成、不 abort；无 `session_titled`；title 回退首行 |

**结果**：PASS（2026-07-11，脚本 `qa/run-qa53.sh`，私有新二进制 daemon）。
真 Gemini 红线 1-4 全绿：长多行 prompt → 精简 auto title「分析用户认证安全
机制」(30 字，非首行截断) + 恰一条 `session_titled{source:auto}` + 一条
`autotitle` llm_call；`sessions list --json` title = RawTitle；坏 key 起
第二个私有 daemon 跑同 prompt → **fail-closed 零 session_titled**（不产生
错误标题）。手动 rename 不覆盖 auto 由孪生 TestAutoTitleDoesNotOverrideManual
+ webui displayTitle 层保证（localStorage 手动值胜出）。归档
`qa/runs/2026-07-11-INC52/`，session 拷回共享 store。锚孪生（A 闸已绿）：
TestSessionTitledFoldProjection / TestAutoTitleGeneratesOnceAndFoldsProjection /
TestAutoTitleWaitsForOpeningReply / TestAutoTitleDoesNotOverrideManual /
TestAutoTitleReusesRecordedResultOnReplay / TestAutoTitleSwallowsLLMFailure /
TestCLISessionsJSONSurfacesAutoTitle / viewModels.test.ts。

---

## QA-46 worktree 运行位置产品化：位置/可见/apply-back/cleanup（INC-49,G13,UJ-10/24）

**环境**：最新 `main`、共享 `~/.local/share/agentrunner/` store/daemon、真实
Gemini 模型、`http://127.0.0.1:8788`（webui 强刷）。选一个真实 git repo 开
`New worktree`，跑真实模型改文件，再走 apply-back 与 cleanup 生命周期闭环。
证据（截图 + `ar events` 导出）归档 `qa/runs/2026-07-10-qa46/`；测试 session
与 repo 保留不清理。

| # | 真实状态/动作 | 硬断言 |
|---|---|---|
| 1 | 选 Git project + `New worktree` + Branch 提交真实任务 | 后端 worktree 落 `~/.local/share/agentrunner/worktrees/<repo>-<branch>-<ts>`（**不**在 webui `runtime/ws`）；目录名含 repo 与 branch/ref |
| 2 | 打开会话 Changes 面板 | 显示「worktree of <repo> · <branch\|detached>」徽标 + `Apply to project` / `Remove worktree` 按钮 |
| 3 | 真实模型改若干文件后 `Apply to project` | 确认后 patch 干净落主 checkout working tree（未 staged），主 repo `git status` 见对应改动 |
| 4 | 构造冲突（主 checkout 同文件另改）再 Apply | 报冲突、主 working tree **零改动**（不静默半合并） |
| 5 | `Remove worktree`（脏树先拒→确认→force） | 脏树弹「有未 apply 改动」确认；确认后 worktree 删除、`git worktree list` 不再含它、`worktree prune` 生效 |
| 6 | 旧 `runtime/ws/wt-*` worktree 兼容 | 已存在的旧 worktree 会话 Changes 面板仍能打开与 diff（不迁移、不弄丢） |

**结果**：PASS（6/6，真机 Gemini）。worktree 落
`~/.local/share/agentrunner/worktrees/ar-qa46-repo-main-<ts>`（非 webui runtime/ws）；
Changes 面板显「worktree of ar-qa46-repo · detached」徽标 + Apply/Remove 按钮；真实
模型改文件→Apply 干净落主 checkout（未 staged）；冲突 Apply 返 409 主树零改动；脏树
Remove 二次防呆确认后 force 删除 + prune。**发现并修两处真机缺陷**：INC-49.1
（apply 的 git add -A 致 Changes 视图误空 → commit-tree 后 git reset -q 还原未暂存）、
INC-49.2（ConfirmModal 自关吞掉 Remove 脏树二次确认框 → setTimeout(0) 推迟）。证据
归档 `qa/runs/2026-07-10-qa46/`（EVIDENCE.md + 4 份 ar events + 冲突响应体 + spec）；
测试 repo/worktree/session 全保留。
锚孪生：TestWorktreeInDataDir / TestApplyBackCleanApply / TestApplyBackConflictReported /
TestWorktreeRemoveGuardsDirty / TestDiffReportsWorktreeMeta。

---

## 覆盖矩阵

| 核心场景 | QA 流 |
|---|---|
| C1 多输入续聊 | QA-01, QA-03 |
| C2 忙时排队 | QA-02 |
| C3 并行 spawn | QA-04, QA-09 |
| C4 子完成激活父 | QA-04, QA-05, QA-09 |
| C5 杀子 agent | QA-05, QA-09 |
| C6 steer 改编排 | QA-05, QA-09 |
| C7 完整编排 | QA-09 |
## QA-20 工程团队模拟（INC-12,UJ-23） `覆盖 G10 关闭 · 决策 #35/#36 真验`

**环境**：共享 store + 全局 daemon（CLAUDE.md QA 规则：不隔离、数据
保留、归档 `qa/runs/<日期>-QA20/`）+ 真实 Gemini。脚本
`qa/run-qa20.sh <ar二进制>`。

| # | 动作 | 验证（只钉 runtime 红线） |
|---|---|---|
| 1 | lead spec `agents_dynamic: true`,开场要求组 engineer+reviewer 两个 inline role 完成 hello.py+评审,互发 send_message | ≥2 `SpawnRequested` 携 `role_spec`（构造 spec 冻结） |
| 2 | 团队自主协作至静止 | lead 收 ≥2 `SubagentCompleted`;某成员 journal 或 lead journal 存在 `source:"agent"` 的 `InputReceived`（树内消息真实投递） |
| 3 | `ar send <child-sid>` 给已静止成员发总结请求 | lead journal 出现 `ChildRevived`;成员 journal 出现 user-class 输入（`ar send` 的 cli 源∈user-class,决策 #30）并回答;成员 journal **恰一条** `SessionStarted`（context 延续,不起新会话） |

**通过标准**：三步断言全 PASS;会话保留可复查（webui/`ar attach
<child-sid>` 可点开每个成员的完整时间线与 live 流）。2026-07-09 首跑
PASS（existing team session，协作期含成员互发消息与多次
revive、gen_steps 同 context 递增;真验期抓获并修复三个 bug:CLI 子 id
截断、Resume 期转投无 Router、cli 源不入 user-class——记 LOG）。

## QA-21 runtime 基础加固收口（INC-11）

**环境**：当前 `main` 二进制、真实 Gemini、真实共享
`~/.local/share/agentrunner`（不清理已有 session）；共享 daemon 当时有活跃
审批，故不重启它，新增 one-shot session 直接写同一真实 store。当前 WebUI
以 `--no-daemon` 连接原共享 daemon，地址固定
`http://127.0.0.1:8788`。归档 `qa/runs/2026-07-09-QA21/`。

| # | 动作 | 硬断言 |
|---|---|---|
| 1 | 真 Gemini lead 动态 spawn 一个 engineer，`agent_workspace: isolated` | `inspect.delegations` 为 quiescent，持久化 delegation/lease/member/workspace/base_ref；父 workspace 无目标文件，子 worktree 内容精确为 `REAL-ISOLATED-OK` |
| 2 | 检查 root/child store 与最新 fold snapshot，再对静止 session 执行当前二进制 `resume` | 两侧 `events.idx` 存在；snapshot 有非零 offset+64位 hash；resume rc=0 且 events 行数 48→48（只续 fold、不重跑） |
| 3 | 浏览器打开父 session 与 child hash URL | 父页显示 Subagents·1/完成回执/子链接；子页显示 read-only sub-agent 与 write/message 链；console error/warning=0 |

**结果**：PASS。session
该 isolated team session 与全部 workspace/journal
保留。有限树预算在 child reservation 存续期间会显示 transient
`limit exceeded`，settlement 后正常完成；这是既有 reserve-then-settle 的
可见表现，不是重复执行或预算越界。

## QA-60 Web UI durable Last turn Changes（INC-57,UJ-24）

**环境**：live `http://127.0.0.1:8809` + 当前 `ar-live` + 真实 Gemini +
共享 `~/.local/share/agentrunner/`；不重启共享旧 daemon、不隔离、不清理。
multi-turn available session=`20260711-084204-use-edit-file-to-replace-base-7826`，
truthful unavailable session=`20260711-082007-use-the-edit-file-tool-to-repl-88c1`。
证据 `qa/runs/2026-07-11-QA60-last-turn-diff/`。

| # | 动作 | 硬断言 |
|---|---|---|
| 1 | 最新共享 daemon + 真 Gemini 连续两条 human turn：第一轮只改 A，第二轮改 A+B | 第二条 input seq=35 后 `bar-t4` seq=38；Working tree 的 A before=`BASE_FINAL_A`，Last turn before=`TURN_ONE_FINAL_A`，真实证明累计/本轮范围不同 |
| 2 | live Changes 默认打开 Working tree，再开范围 menu 切 Last turn | 两档都是真 API；menu 描述/active check；Last turn 不显示会提交全 workspace 的 Commit/Apply/Remove |
| 3 | 打开由仍在 socket 上旧 daemon 产生、无 barrier 的真实 two-turn session | 显示 `Last turn unavailable` + runtime reason，可切回 Working tree；绝不伪造空 diff |
| 4 | 1440×900 / 390×844 × light/dark；键盘 Escape；菜单 focus return | toolbar 不溢出、mobile 全宽、split disabled；Escape 后 `expanded=false` 且 focus 回 trigger |
| 5 | Codex reference 与 build 同屏比较；读稳态 console | sidebar/thread/right split、密度与控件层级对齐，保留 AgentRunner brand/Supervision；error+warning=`[]` |

**真验发现并修复**：初版 baseline 选择接受任意 input 后 barrier；浏览器真验
暴露显式 `ar barrier` 的 `bar-m*` 可在任意工作后产生，会错误缩小 Last turn。
最终只接受 loop-owned `bar-tN` generation-start barrier；`bar-m*` 与
`bar-final` 均跳过，TestPlanLastTurnDiffBaseline 直钉。

**结果**：PASS。所有 session/workspace/journal/screenshots 保留。

## QA-61 Web UI 大历史/大 Diff 完成性审计（INC-60,UJ-24）

**环境**：live `http://127.0.0.1:8809` + 私有候选 `127.0.0.1:8817`，均使用
共享 `~/.local/share/agentrunner/`（454 个 session）；in-app Browser 真机，
不隔离、不删除 session/workspace/journal。证据
`qa/runs/2026-07-11-QA61-completion-audit/`。

| # | 动作 | 硬断言 |
|---|---|---|
| 1 | 旧 live 全量 `/api/sessions` 与 Changes 基线 | sessions 32.21s 后 502；重入争用时 Changes 23.28s，真实复现长期 skeleton/`Loading session…` |
| 2 | 候选 CLI/API progressive page | CLI 40 条 0.05s、80 条后续页 0.04–0.17s；API 首页 0.06s；300ms 已有 session，约 1.2s 全历史 717 个 session button；同一 refresh chain 不重入 |
| 3 | deep link + large Changes | header 首帧可读且无 `Loading session…`；真实 616 个 generated untracked 文件隐藏但计数，API 0.09s/60,454 bytes，3 个 source 文件保留，默认折叠且首 paint 不先展开 |
| 4 | Settings/Command palette/project picker/diff scope 键盘 | Escape 关闭；Settings mobile 回 `Show sidebar`，Command palette 回原 composer，picker/scope 回 trigger；dialog/menu/scrim 均清理 |
| 5 | 1440/900/642/390 × light/dark，approval/Changes/Settings | body `scrollWidth==clientWidth`；900/642/390 Changes panel 均在 viewport；操作完整可达；AgentRunner brand + Codex IA + Supervision 分层不变 |
| 6 | 稳态 console + 全树闸门 + live redeploy/reload | error/warning=`[]`；`./scripts/check.sh` 全绿；部署后 8809 重做首屏/deep link/large diff（结果见证据清单） |

**真验发现并修复**：全量 journal fold 的 4s 无互斥 polling 是入口雪崩根因；
另发现 `unknown` build stamp 泄漏、Settings/Command palette focus 丢失、untracked
`node_modules` 巨型 diff 会让浏览器失败、large diff disclosure 在 effect 阶段太晚。
分别以 CLI/API 分页 + serialized hydration、稳定产品 label、focus return、generated
目录/字节/文件上限、首 paint 前 disclosure 修复。截图中的 screencast delta 黑块
列为拒收采集伪影，不作产品判断。

**结果**：PASS。`f2f1932` 已 push `origin/main`，launchd live 8809 加载
`index-CTcdOVfV.js`；最终 5 次首页 page 均 HTTP 200（0.10–0.29s），large Diff
HTTP 200（0.13s/60,454 bytes），390×844 dark deep link/默认折叠/focus return
复验通过且 console=`[]`。所有共享测试数据与截图保留。

## QA-43 Codex UI 全景验收（INC-41,UJ-24）

**环境**：live `http://127.0.0.1:8809` + 共享 store；in-app Browser 真机，
非隔离数据。证据 `qa/runs/2026-07-10-QA43-codex-ui-polish/`。

| # | 动作 | 硬断言 |
|---|---|---|
| 1 | Home/rich thread/approval/Scheduled/Settings/Changes 六主态，desktop/mobile × light/dark | 12 个主镜头均为真实产品状态；approval、真实 multi-turn diff、长 Markdown/代码内容均非 mock |
| 2 | 1554/1440/900/642/390 响应式补扫 | composer/sidebar/right panel 无裁切；900 Changes 与 642 Home 正常切换布局 |
| 3 | mobile 从 sidebar 打开 Settings | Settings dialog 出现且 `Close sidebar`/scrim 不再可见；修复前失败截图与修后 dark/light 图均保留 |
| 4 | mobile/desktop 打开 Changes | 截图前必须可见 `Change diff scope` 且 DOM 含 `final-a.txt`/`final-b.txt`，杜绝只截到未打开状态 |
| 5 | 三张 contact sheet 与 Codex reference 逐屏对照；读稳态 console | 信息架构、密度、层级与 Codex 对齐，保留 AgentRunner brand/Supervision；error+warning=`[]` |

**真验发现并修复**：`App.tsx` 的 Settings 入口只开 dialog，mobile sidebar
仍留在其上方。入口现先走与导航相同的 `closeAfterNavigate()`，再打开 Settings；
390×844 dark/light 复验侧栏与 scrim 均消失。

**结果**：PASS。全景截图、contact sheets 与浏览器状态断言全部保留。

| C8 interrupt vs 输入 | QA-02, QA-06 |
| C9 多模态 | QA-07, QA-09, QA-15（PDF/文件） |
| C10 恢复 | QA-08, QA-09(步骤6) |

反向索引（一个流盖多个 feature 的示例）：QA-09 一条压 8 个场景——它是
发布前的冒烟总闸；QA-01～08 是定位问题用的单项闸。

---

*执行纪律：每次跑完（无论过否）把 `ar events` 导出的 journal 与
workspace diff 归档到 `qa/runs/<日期>-<QA号>/`；FAIL 必须先归因
（runtime bug / prompt 不确定性 / 环境）再修。*

---

## QA-52 provider thinking 预算上限（空消息饿死修复，INC-59,G34,UJ-01/18）

**环境**：最新 `main` + 真实 Gemini（`gemini-flash-latest`）+ 共享
`~/.local/share/agentrunner/` daemon/store。缺陷：Gemini thought token 从
`MaxOutputTokens` 扣，`Thinking.Enabled` 且 budget≤0 时旧 provider 不设上限，
思考吃光 cap → 红条 `empty message (truncated at token cap...)`。真实 API
before/after 复验，证据归档 `qa/runs/2026-07-11-QA-52-thinking-budget/`
（live-gemini-thinking.txt / unit-budget-clamp.txt / user-session-evidence.txt /
README.md）。live regression `TestLiveThinkingStarvation`（-tags live）。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | 现场：共享 daemon session `20260711-073559-create-a-todo-app-ff36` events | seq 116 `activity_failed / provider_server: model returned an empty message (truncated at token cap...)` 真实存在（诚实归因：该 spec `Thinking.Enabled=false`，此条为大 tool-call 撞 4096 cap，loop.go 兜底已恢复；非思考饿死） |
| 2 | 真实 API：unbounded/over-budget thinking + 小 cap(256) | 思考 241 tok 挤到正文仅 11 tok、finish=MAX_TOKENS —— 思考饿死机理复现（tool call 原子，这点余量放不下 → 字面空消息） |
| 3 | 真实 API：budget 0（thinking off） | `thoughtToks==0` —— 确认 budget 0 在本模型真关思考 |
| 4 | 真实 API：修复后 provider（enabled + over-budget，同 cap） | `resolveThinkingBudget` 钳预算，正文 252 tok 真实产出，不再空；`gotText==true` |

**结果**：PASS（4/4，真机 Gemini）。单测各档位/边界全绿；`./scripts/check.sh`
全绿。锚孪生：`TestResolveThinkingBudget` / `TestToConfigEnabledTinyCapDisables` /
`TestToConfigEnabledNoBudgetIsBounded` / `TestToParamsThinkingBudgetClamped` /
`TestToParamsThinkingCapTooSmall` / live `TestLiveThinkingStarvation`。诚实边界：
gemini-flash-latest 被给 tool 时自适应缩短思考、通常自保，故本修复是**结构性
保证**（不依赖模型自适应）+ 移除 unbounded 路径。测试 session 保留不删。

---

## QA-62 审批常设应答（INC-62,G35,UJ-08/18）

**环境**：私有 daemon + 私有 XDG_DATA_HOME/XDG_CONFIG_HOME（场景写 user
配置，不碰真实 ~/.config，同 QA-26 理由）+ 真实 Gemini。可在 GitHub
Actions runner 执行（workflow `qa-inc62`，repo secrets 供
GEMINI_API_KEY）；journal 导出归档 `qa/runs/<日期>-QA62/`。脚本
`qa/run-qa62.sh`。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | mode default、无 rules 的编排 spec，一条消息要求起 3 个 worker（background） | 第一个 spawn_agent 落 `approval_requested`（execute 类 mode default ask） |
| 2 | `ar approve <sid> <apid> approve --always` 一次 | 后续 spawn 静默：`spawn_requested ≥ 3` 且 `approval_requested == 1`——三连 spawn 只问一次（G35 现场的反向断言） |
| 3 | 同 journal 审计链 | 至少一条 `effect_resolved` 判词含 "standing approval"（免问有名有据） |
| 4 | 写回检查 | user 配置获得 tool 级 `spawn_agent` allow 规则 |
| 5 | 全新 session 2 起 1 个 worker | `spawn_requested ≥ 1` 且 `approval_requested == 0`（写回规则次 session 生效） |

**判绿即**：GAPS G35 与 SPEC「审批'允许且不再问'」行回 ✅。

---

## QA-63 curl 一行安装分发（INC-63，UJ-25）

**环境**：GitHub Actions（release workflow）+ 一台干净的目标机器（无
Go/Node 工具链）。gate A 孪生（`scripts/test-install.sh`，离线 5 场景）
在 check.sh 常跑，本条只钉真实环境的 journey 全程。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | dispatch/tag 触发 release workflow | 4 target 打包全绿；linux 产物 smoke 三腿全过（`ar --version`/`ar init`、`arwebui /api/health` 探活、真 install.sh 装真产物后二进制可跑、安装器孪生 5/5） |
| 2 | 打 `v*` tag（首个真实 release，用户决策） | GitHub Release 挂出版本命名 + 稳定命名（`agentrunner-<target>.tar.gz`）+ `.sha256` + install.sh，共 4 target |
| 3 | 干净机器上 `curl -fsSL …/install.sh \| sh`（私有期带 GITHUB_TOKEN） | 装到 `releases/<tag>/`，`ar --version` 回显 tag 版本戳，`ar init && ar run` 可用 |
| 4 | 发第二个 tag 后重跑同一条命令 | symlink 切到新版、旧版本目录保留、运行中的进程不受影响 |

**执行结果（2026-07-12，全绿）**：
- 步骤 1：Actions run 29182533118/29184948127（workflow_dispatch）——4
  target 打包 + 三腿 smoke，日志逐条核对（非只看结论位）。
- 步骤 2：run #2 用 `publish_tag=v0.1.0` 由 CI（GITHUB_TOKEN）代建 tag
  并发布 release 352700642，17 资产齐（4 稳定名 + 4 版本名 tarball 各带
  .sha256 + install.sh）。
- 步骤 3：本容器真跑 `curl -fsSL …/install.sh | sh`（repo 已公开，免
  token 公网路径）→ 装出 v0.1.0，`arwebui` 起服 `/api/health`
  versionMatch=true（兄弟 `ar` 优先，未被 binutils 同名 ar 顶掉）。
- 步骤 4（升级 symlink 切换）：由孪生场景 2 锚（真实"两个 release 连装"
  留待第二个 tag，属既有能力回归、非新机制）。

**判绿即**：SPEC「curl 一行安装分发」行 🟡 → ✅。（已落。）

---

## QA-64 Web UI 多轮真实任务黑盒回归（UJ-24）

**环境**：live `http://127.0.0.1:8809` 基线 + 候选
`http://127.0.0.1:8944`，共享 `~/.local/share/agentrunner/` daemon/store
（457+ 个既有 session），真实 Gemini；390×844 / 642×800 / 900×800 /
1440×900。证据 `qa/runs/2026-07-12-QA64-blackbox/`，会话、workspace、
journal 与 diff 全保留。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | Home/Sidebar/Settings/Scheduled/搜索/缺失 deep link | 无 crash/console error/横向溢出；缺失任务可回 Home；`git` 搜索落 Git 且设置不被筛空 |
| 2 | 手机+桌面各建任务，真实 Gemini 连续回答 2 轮 | 新 session 完成且上下文连续；可重载；最终 session `20260712-202819-1-1-8bc0` / `20260712-202823-1-1-0606` |
| 3 | 给两条新 session 的真实 workspace 写唯一 QA 文件，从顶栏菜单开 Changes | 菜单全在 viewport；390/642/900 是可关闭 overlay，1440 是 split；文件名可见 |
| 4 | 既有 completed/diff/approval/recovery/scheduled 状态 + web server 重启 | 两文件 Last turn diff、审批卡、筛选空态均恢复；只重启 arwebui，不扰动共享 daemon |
| 5 | API 健康/分页/events/diff/404 与 Actions 真 key 环境 | 200 路径 0.01–0.06s；missing state 404 structured code；Actions run 29207066088 全绿 |

**真验发现并修复**：① sidebar 新任务写成 `just now ago`；② Settings
页名查询把自身内容筛空；③ Tailwind mobile media rule 把已打开的
Changes/Environment 设成 `display:none`；④顶栏 `···` 菜单一律向上，手机
`Changes` 落到 y=-306px。分别以统一 `relTimeAgo`、section-aware 搜索、
恢复 responsive overlay、`Menu` 复用 viewport-pinned `Popover` 修复；
⑤ Scheduled 弹窗的可见 `Prompt`/`Workspace` 标签未关联字段，首帧快速输入
可能被自动聚焦竞态写入错误字段，以 `htmlFor`/`id` 关联和可访问名称定位修复。

**harness 收敛**：侧栏宽泛 selector 与 Scheduled 吞异常 selector 各产生
一次假 finding；最终均改成可访问名称 + 提交前值断言。增强脚本还会给
真实 workspace 留一份唯一文件，强制验证 Changes 的非零几何尺寸。

**结果**：PASS。合并最新 main 后 candidate round 8/9 连续 0 finding；
定向 48 条与完整 `./scripts/check.sh` 全绿。测试数据不清理。

---

## QA-66 Runtime 状态正确性回归（INC-66，UJ-14/15/18/22/24）

**环境**：共享 `~/.local/share/agentrunner/` daemon/store、真实 Gemini、部署版
`http://127.0.0.1:8809/`。会话、journal、workspace 不清理；events、inspect、
health 与浏览器截图归档 `qa/runs/2026-07-13-INC66/`。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | 创建 in-session Goal，verifier=`false`/max-checks=1；随后 update verifier=`true`/max-checks=2 | 第一次只有 `goal_exhausted{budget}` 且保留 goal；update 后同 session 继续，第二次 pass 才有 `goal_achieved{satisfied}` |
| 2 | 20k tree budget 的 lead 同一 tool batch 启动 3 个无 child cap worker | 三份 reservation 都是 5366，三 child 均 quiescent/completed；settled+reserved 不超 cap，无 running progress 残影 |
| 3 | 1s fixed-rate/skip/max3，真实 child 运行跨 tick | missed slot 明确 journal 为 skipped；系列以 max_iterations 结束，terminal 无 `nextRunAt` |
| 4 | 对步骤 3 通过 `/api/sessions/:sid/retry` 执行 Scheduled Retry | 新建 series 会话（INC-80.2c 重裁：首事件 `session_started`+`series_started`，零 `driver_started`），没有 `input_received`；inspect 汇总 raw/cache/billed usage |
| 5 | 保留无 genesis 的空 session 目录后 list/send | list 隐藏；send non-zero 并明确要求创建新 session，不永久 queued |
| 6 | 部署并重启真实 daemon/webui，浏览器打开 Goal、multi-agent、Scheduled 及 `#run:run1` 后 reload | health `daemonUp/versionMatch=true`；状态与 deep link 持久化；console warning/error=0 |

**结果**：PASS。原始 20 条中按用户要求排除 #11 Ctrl-C，其余 19 条均有定向
孪生并通过真实环境验收。B 闸额外发现并修复：deploy 对 session 文本误判 running、
旧 launchd webui 被自动拉起导致假部署成功、daemon `nohup` 被调用方清理但
sessions 命令造成假存活、driver 终态只显示 billed 而 raw usage 为 0。

---

## QA-67 设计契约加固（INC-67，G37）

**环境**：live `http://127.0.0.1:8809` + 共享
`~/.local/share/agentrunner/` daemon/store；以 Go 1.26.5 构建并重启真实
daemon/webui，in-app Browser 验收。证据归档
`qa/runs/2026-07-13-INC67/`，测试数据不清理。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | 部署前读结构化 sessions status，再安全重启 daemon/webui | running=0 才重启；health daemonUp/versionMatch=true，版本使用无已知漏洞 Go patch |
| 2 | 共享 store session list/detail/deep link + 浏览器 console | 既有 session 数量不降，真实历史可打开；console error/warning=0 |
| 3 | multipart 上传小文件与 10 MiB+1 文件 | 小文件逐字保留；超限返回 413，uploads 不新增 partial/truncated 文件 |
| 4 | JSON 后附第二个 value、非法 session path/prefix | API 返回 400；CLI 明确拒绝 `../`，不解析 store 外 journal |
| 5 | clean dist 全量 gate、race、`govulncheck`、`npm audit` | `check.sh` 从仅 `.gitkeep` 起全绿；多 writer race 5 轮绿；可达 Go 漏洞 0；npm 漏洞 0 |

**结果**：PASS。共享 store 保留 584 个 session；小文件 SHA-256
`093054a58a184e264e6047ea68c477f05b3a01be5b69d6bc05004623af5aad40`，
uploads 仅增加该 1 份，10 MiB+1 请求为 413；trailing JSON 为 400，
`../` CLI 解析 exit 2。浏览器打开并 reload 真实三 worker session，主答案与
ready 状态保留、无横向 overflow、console warning/error=0。`check.sh` 从 clean
依赖/产物起全绿；rebase 最新 main 后 58 frontend files / 567 tests 与 installer
5 场景全绿；race 3 轮、
两套 `govulncheck` 与 `npm audit` 均为 0 可达漏洞。

---

## QA-68 Web UI iOS session 细节收口（INC-68，G36，UJ-24）

**环境**：live `http://127.0.0.1:8809` + 共享
`~/.local/share/agentrunner/` daemon/store（部署前 587 session、running=0）；
Go 1.26.5 candidate；in-app Browser 390×844 与 1280×900。证据归档
`qa/runs/2026-07-13-QA-68/`，新 session、workspace、journal、diff 均保留。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | New session 逐字发送「我们创建一个工程团队，合作写一个 agent runtime 吧，看看能不能做到。这个 runtime 需要支持 session state、transfer to agent 等功能」 | `20260713-082616-session-6d0d93bfcf9daa38` 等待输入；标题 `Building an Agent Runtime`；14 files/+932，8 tests PASS，session state/transfer demo 可见 |
| 2 | 390×844 打开三 worker parent，展开 Worked/activity，再逐一 deep link/reload 三 child | activity `×6` 无双 marker；child link 分栏不黏连；header 分别为 `worker_a/b/c` + Read-only；全部 overflow=0 |
| 3 | 打开 provider failed session | 具体 failure card 恰 1，泛化 terminal card 为 0；标题/hint 分行，Retry/Technical details 可用 |
| 4 | 先开 Environment 再开 mobile sidebar | Environment 1→0，sidebar/scrim 唯一且无第二 overlay；overflow=0 |
| 5 | recovery、Changes、New session、Project/Access/Model picker | recovery 单卡；Changes 366×820 在 viewport；三个 picker 全部 contained；无裁切/横向 overflow |
| 6 | 1280×900 desktop、health、browser dev logs | sidebar/Environment 桌面投影正常；`daemonUp/versionMatch=true`；warning/error=0 |

**结果**：PASS。五枚 finding 全有 before/after，16 张 after 截图；定向 24/24、
frontend 58 files / 572 tests 全绿。literal session metadata 中 `ios` 匹配为 0，
因此 `iOS` 作为 390×844 设备视口验收；真实 parent 的三个 child 已全部打开。

---

## QA-69 webui 双锚真浏览器验收（G30 收尾,audit-0717 F1,UJ-04/24）

**状态**:PASS(2026-07-18,脚本 `qa/run-qa69.sh`,证据
`qa/runs/2026-07-18-QA69/`——截图×3+日志)。
**环境**:真 Chromium(playwright)+ 真 arwebui + 真 daemon(scripted
provider——本场景验的是渲染红线,无模型面)。
**红线**:A. >10 渲染行 user 消息真布局下钳高(.utext.clamped,实测
220px)、Show more 展开(694px)、Show less 复钳;B. composer Add 菜单
呈 Add/Advanced 两组、根动作 ≥5(Plugins 占位组已随 PLAN 5.1 移除)。
**为何此前无锚**:折叠判定依赖真实 scrollHeight,jsdom 无布局——
这正是这两行 SPEC 长期只有档期名锚的原因。

---

## QA-70 daemon 生命周期(INC-71 stranded 自动接续 + INC-72 优雅停机保活 cron,UJ-14/21)

**状态**:PASS(2026-07-18,脚本 `qa/run-qa70.sh`,GitHub Actions
`qa-daemon-lifecycle` run #3(29632900834),证据 artifact
`qa70-evidence`——daemon/drive 日志 + 全部 events.jsonl)。
**环境**:真 Gemini + 真 daemon(ubuntu runner + bubblewrap;本地容器
无 key 时的指定跑法)。
**红线**:A. bash execute 在飞时 kill -9 daemon → 重启后**零 send**:
in-doubt 渲染 interrupted-by-crash、turn 继续、session park(INC-71
boot sweep);B. 本地 drive crash → boot sweep 收编为 daemon 托管 →
SIGTERM 优雅停机 → journal **无 driver_completed 终态** → 再次重启
系列复活(新 iteration 事实出现)(INC-72)。
**跑法教训记档**:run#1 等待条件在 LLM 阶段误判(须等 bash execute
activity 同行匹配);run#2 runner 缺 bwrap 致 execute fail-closed
(workflow 现自装+userns 放开+探针)。

## QA-75 OS 沙箱依赖交付（INC-75，UJ-25）

**环境**：GitHub Actions ubuntu runner（干净环境，未预装 bubblewrap）。
gate A 孪生（`ar doctor` 探针注入单测 + `scripts/test-install.sh` 场景
6–8）在 check.sh 常跑，本条钉真实 CI 环境从零到沙箱可用的全程。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | dispatch `sandbox-doctor` workflow | `setup-ar` action 装上 bubblewrap、放开 AppArmor userns、两档真实 probe（network=all/none）全过 |
| 2 | 同 run 内构建 `ar` 并跑 `bin/ar doctor` | 输出 backend=bwrap、network=all/none 均 OK、退出码 0 |

**结果**：见 LOG（INC-75 收口条目，run 链接归档）。

## QA-74 session 内 schedule(INC-74/E1①,UJ-14/22)

**状态**:PASS(2026-07-18,脚本 `qa/run-qa74.sh`,GitHub Actions
`qa-session-schedule` run #1(29634255244),证据 artifact
`qa74-evidence`——daemon/cli 日志 + events.jsonl;三条硬断言逐条
核对日志:PASS(1) 06:37:00 / PASS(2) 06:38:00 / PASS(3) 06:39:20,
pause 时 wakes=2 且此后不再增长)。
**环境**:真 Gemini + 真 daemon(spec 无 execute 工具,不需沙箱;
本地容器无 key,经 Actions secrets 跑)。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | `ar schedule <sid> attach --cron "* * * * *" "<prompt>"` | `schedule_attached`+`timer_set` 落 journal;≤1 个 cron slot 内 session **零 send 自主唤醒**:`schedule_wake` + assistant_message 增长(真 turn) |
| 2 | SIGTERM 优雅停机 → 重启 daemon(session 未托管) | timer sweep 到点 hostResume → 第二次自主唤醒仍完成真 turn——唤醒跨 daemon 重启存活 |
| 3 | `ar schedule <sid> pause` 后静置一个完整 slot | `schedule_paused` 落账;wake 计数不动;`schedule status` 呈现 paused |

## QA-76 UI 事实声明 × 系统真相对账（QA-0718 复盘产物,UJ-04/14/18/24）

**这是什么**:webui 每一条"事实声明"(Edited N files / Changes 计数 /
Background running / PROGRESS x/y / token 数 / 状态 pill)背后都有一个
可独立求证的权威真相源;QA-0718 用户实机连撞的三类核心 bug(幽灵
diff、真写盘卡消失、child 卡死却报 running+spending)全是**声明与真相
脱节**。本场景组把"对账"固化为常设回归:每个语义状态下同时读 UI
API 声明与真相源,自动断言一致。**新增任何 UI 事实声明,必须同步在
下表登记真相源并加对账场景——否则不算 work(执行纪律同 PROCESS
双闸门)。**

### 声明 ↔ 真相源清单(inventory,持续维护)

| UI 声明 | 权威真相源 | 对账场景 |
|---|---|---|
| timeline "Edited N files"(last-turn) | `ar diff --scope last-turn` 文件集 | S1 |
| timeline "Changes in workspace"(回退) | `git status --porcelain`(workspace) | S2/S3 |
| rail "Changes N files ±"(working-tree) | 同上 | S1–S4 |
| commit 后 Changes 清零 | git status 为空 | S4 |
| BACKGROUND WORK 行 "running" | child sub-store journal 末事件非终态 | S5 |
| ATTENTION "keeps spending tokens" | child journal usage 增量 > 0 | S5(G39 红锚) |
| child 审批可见性 | child journal `waiting_entered{approval}` ⇒ 父面 ATTENTION 必须呈现 | S5b(G39 红锚) |
| PROGRESS x/y 终态完成性 | 会话终态时 x=y 或有 error 事实 | S5 |
| 会话状态 pill | `/api/sessions[].status` | S1–S5 |
| 审批卡出现 ⇔ 待批 | `/api/sessions[].status`=waiting:approval + journal `approval_requested` | S7(远程驱动已验) |
| Approve ⇒ 命令真执行 | 目标文件入 working-tree/last-turn diff + journal tool_result | S7(远程驱动已验) |
| Deny ⇒ 命令真未执行 | 目标文件不在任何 diff;"Denied · <tool>" 审计 chip 在 turn 折叠内 | S7(远程驱动已验) |
| Review 入口 scope 配对 | 卡声明的 scope == DiffView 打开的 scope(单测锚) | S7(ChangesOutcome.test scope pairing) |
| Run details token 数 | journal usage 累计 | S6(待补) |

### 场景(qa/consistency/check.mjs 逐条执行,CI: qa-consistency.yml)

- **S1 写盘对账**:blank workspace 新会话(full access),指令式 prompt
  写死"创建 a.txt 内容 hello,不做别的"。turn 完:last-turn 文件集 ==
  working-tree 文件集 == git status == {a.txt};会话 status 回
  waiting:input。
- **S2 脏接手不谎报**:同 workspace 再开会话,prompt "只回复 hi,不碰
  文件"。turn 完:last-turn 为空(UI 不得声明 Edited);working-tree
  仍 == {a.txt}(rail 如实)。
- **S3 重启后不失踪**:重启 daemon+webui(journal replay)。两个 scope
  再对账:last-turn 允许为空,working-tree 必须仍 == git status(
  "Changes in workspace" 回退卡的数据面)。
- **S4 commit 清零**:经 API 提交,git status 空 == diff 两 scope 均空。
- **S5 子 agent 终态对账**:full access 会话 spawn 1 个 worker 写文件。
  终态:child journal 末事件为终态(SubagentCompleted 配对),文件落盘
  入 git status;父 status 回 waiting:input 且无悬挂 background 声明。
- **S5b(G39 红锚,expected-fail)**:ask 模式 spawn,child 卡
  `waiting_entered{approval}`——断言"父面可见该审批"。当前必红;
  G39 修复合入之日转绿,此行改为常设绿门。
- **S7 审批语义对账**(QA-0719 第十四轮远程驱动首验;**已并入
  check.mjs approval phase,随 qa-consistency 6h 定时跑**):ask spec
  (bash: action ask)会话请求 bash 写文件。status=waiting:approval ⇔
  journal 有未决 approval_requested;API approve ⇒ 文件真落盘(git+
  diff API 双侧);API deny ⇒ 双侧无痕。UI 层(卡出现/ATTENTION 计数/
  审计 chip)由远程驱动轮覆盖;Review 入口 scope 配对由 ChangesOutcome
  单测锚(scope pairing)常绿。
- 通过标准:findings.json 无 `mismatch`;S5b 单列 expected-fail 通道,
  转绿需人工把它提级为硬门(防悄悄退化)。

## QA-77 merged-stream series 真机验收（INC-80，UJ-14/15/16/24）

**闸门 B 登记（三视角 review 契约 P1-3 定的必补清单）**——INC-80 的
收敛在闸门 A（10+ 孪生）已钉，以下场景待真实 API 轮跑：

1. **cron/interval 值守跨 daemon 重启（series 形态）**：起 interval
   series（真 Gemini child），SIGTERM 优雅停机断言无 `series_ended`
   终态，重启后 boot sweep 从 `session_started` 头分派 ResumeSeries
   复活并按 overlap 恰好补跑（对齐 QA-70/QA-74 语义）。
2. **best-of-N 会话形态**：`n:2` 真机跑——`SeriesStarted.BaseRef` pin、
   两 attempt 各自 worktree、`SeriesEnded.BestIter` 选择、胜者树留盘、
   主 workspace 零改动。
3. **旧 DriverStarted 会话兼容读**：升级后 `sessions/inspect/events` 与
   webui Scheduled 对 07-19 前的 driver journal 仍正确投影（cadence/
   终态/迭代树）。
4. **webui /loop 落会话**：真浏览器 `/loop` 启动 → landInSeries 直落
   series 会话页、Scheduled 仅一行（session 行）、run 行让位。
5. **v1 头跨版本 resume**：07-18 窗口（series:1 头）的普通会话在新
   binary `ar resume` 成功（checkVersions 接受旧版本——契约 P1-1 修）。

## QA-0721 子 agent 编排 fire-and-yield（INC-85，UJ-18）

**状态**:PASS(2026-07-21,真 Gemini Flash,A/B 同任务同 workspace,
证据 `qa/runs/2026-07-21-QA-0721/`——两 session 保留共享 store +
run 日志 + events 导出 + findings.md)。
**动机**:主 agent 派完子 agent 后不该 `output` 轮询 / `bash sleep` 自旋等待
——runtime 早已 fire-and-yield(派完可结束 turn、完成作为消息自动唤醒)。
INC-85 把这契约补进 `spawn_agent`/`output` 描述 + dev prompt。断言只钉
runtime 红线(轮询次数),不钉模型措辞。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1（对照/旧） | 旧 `ar-live` + 旧 dev prompt,`ar run` 派 3 worker 审计小 workspace | 稳定复现 busy-wait:`output` 轮询 ≫0（本次 13 次、9 步空转）——阳性对照 |
| 2（修复/新） | 新二进制（含新描述）+ 新 dev prompt,同任务 | 派完即结束 turn:**`output` 轮询 = 0、`bash sleep` = 0**;3 个完成消息各自自动唤醒主 agent;末轮综合出完整报告 |

## QA-0721b loop mode 真机验收（INC-86，UJ-14）

**状态**:PASS(2026-07-21,真 Gemini Flash,**真浏览器 `/loop`**,证据
`qa/runs/2026-07-21-QA-0721b-loopmode/`——REVIEW_NOTES 产出 + events + findings)。
**动机**:gemini-flash-latest 当前拒绝 `thinkingBudget:0`,webui 默认 effort=off +
driver/worker 无 thinking → loop/goal/best-of-N + 默认聊天全 400。INC-86 改默认
medium thinking + 移除 no-thinking。断言只钉 runtime 红线(是否 400 / 是否迭代)。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1（对照/修前） | 旧默认(effort off),真浏览器 `/loop` | 第 1 轮 child 即 `INVALID_ARGUMENT`,series `child_failed`(20260721-162833) |
| 2（修复/修后） | INC-86 部署后,真浏览器 `/loop`(editable_mermaid2,30s,3 轮) | **零 INVALID_ARGUMENT**;Iter 1/2 Completed、Iter 3 overlap-skip、`max_iterations` 收尾;真项目 REVIEW_NOTES.md 累积 2 条真实审查发现(20260721-165144) |

## QA-78 Sidebar project controls（INC-87，UJ-24）

**状态**：PASS（2026-07-21，真实 `http://127.0.0.1:8809/` + 共享
`~/.local/share/agentrunner/`；证据
`qa/runs/2026-07-21-QA78-sidebar-project-controls/`）。测试项目为 `mt-test`
与 `agentrunner dev2`；结束时 project pin、Projects fold、desktop width 均恢复
原偏好，测试 worktree 按 QA 纪律保留。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | desktop project row 键盘 focus → `…` 菜单 | 快捷操作可见；菜单严格为 Pin project / Reveal in Finder / Create permanent worktree / Rename project / Archive chats / Remove；pin 刷新后变 Unpin，再恢复未置顶 |
| 2 | separator 260→480，刷新；Projects fold，刷新 | width=480 与 `aria-expanded=false` 均跨刷新；双击/点击分别恢复 260 与展开；console error/warn=0 |
| 3 | `agentrunner dev2` → Create permanent worktree，branch=`qa/inc87-sidebar-controls-20260721` | 创建于共享 store；`git worktree list --porcelain` 可见，working tree clean；worktree 保留 |
| 4 | viewport 390×844，打开 sidebar 与 project menu | sidebar 是 drawer；无 resize separator；六项 project menu 仍可达；viewport 最后 reset |

**非破坏边界**：未对共享历史实际执行 Archive chats 或确认 Remove；六项入口、
Remove 的“不删 chats/journal/files”确认文案、rail-only 隐藏、session collection
不变与 Restore 由 `Sidebar.nav.test.tsx` 常绿断言。pointer hover preview 同样由
component mouse-enter 测试覆盖；真浏览器验证了等价的 focus-visible controls。
本 QA 不创建 session，故无适用的 `ar events`；worktree status 单独归档。

## QA-79 Project new-chat shortcut + stable button press（INC-89，UJ-24）

**状态**：PASS（2026-07-21，真实 `http://127.0.0.1:8809/` + 共享
`~/.local/share/agentrunner/`；project=`mt-test`；证据
`qa/runs/2026-07-21-QA79-project-new-chat/`）。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | focus `mt-test` project row | 铅笔 accessible name=`New chat in mt-test`；`…` menu 仍含 Rename project |
| 2 | 点击铅笔 | 落 Home；headline/project chip=`mt-test`；`Do anything` active；未 Send，session count 605→605 |
| 3 | reload Home | `mt-test` last-project seed 与输入 focus 仍在；console error/warn=0 |
| 4 | 点击 project `…` | bounding box 24×24→24×24，无 pressed-state size change；CSS 全局无 active scale 由 `buttonPress.test.js` 锚 |

## QA-80 低价值 UI surface 收敛（INC-88，UJ-24）

**状态**：PASS（2026-07-21，当前 worktree 前端
`http://127.0.0.1:5188/` 连接真实 `http://127.0.0.1:8809/` + 共享
`~/.local/share/agentrunner/`；会话
`20260721-221631-say-hi-in-one-word-a4dd080497611f5d`；证据
`qa/runs/2026-07-21-QA80-low-value-ui-cleanup/`）。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | Home 选择 projectless，再恢复 `mt-test` | projectless 只有 project picker，location/worktree/branch 不渲染 |
| 2 | 打开 completed session 及菜单 | header 无 Stop/Fork；tail 只有 Copy；菜单无 link/raw ID/current view/empty Run group；Advanced Continue 仍在 |
| 3 | 原 hash deep-link reload | URL 不变，原 title/timeline 正常恢复 |
| 4 | 检查 clean Environment 与 Scheduled menu | clean 时无 Changes/Commit；Scheduled 只有真实 Organize 动作，无 link/ID |
| 5 | desktop Settings → 390×844 mobile Settings | desktop 只有 Done，mobile 只有 Back to app |
| 6 | 390×844 mobile sidebar/session menu | 每行只有一个 More actions，管理动作集中在菜单；console warning/error=0 |

**边界**：当时无真实 running session，未为截图新启模型任务；composer
Stop 唯一入口、RunView Stop 例外、dirty parent/sub-agent Environment 由前端
component tests 常绿锚定。本 QA 未创建/关闭/删除 session，未清理共享数据。

## QA-81 选中 session 时 project 仍可折叠（INC-90，UJ-24）

**状态**：PASS（2026-07-21，真实 `http://127.0.0.1:8809/` + 共享
`~/.local/share/agentrunner/`；版本 `de6b3966-214638`；project=`mt-test`；
session=`20260721-221631-say-hi-in-one-word-a4dd080497611f5d`；证据
`qa/runs/2026-07-21-QA81-selected-session-project-fold/`）。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | 选中 session 时点 `mt-test` heading | 立即 `aria-expanded=false`，session row 消失；central thread/`More session actions`/hash 不变 |
| 2 | 等待 4.5s sessions refresh | 仍 folded，row count=0，不回弹 |
| 3 | 整页 reload | 仍 `aria-expanded=false`，row count=0，原 hash/thread 恢复 |
| 4 | 再点 heading 展开 | 原 row 恢复，class 含 `project-session-wrap nested current`；console warning/error=0 |

**数据纪律**：验收结束时已恢复 `mt-test` expanded，不留测试偏好；
未创建、关闭、删除或清理 session/workspace/journal。

## QA-83 Sidebar session row 状态与动作（INC-92，UJ-24）

**状态**：PASS（2026-07-22，production build 部署于真实
`http://127.0.0.1:8809/` + 共享 `~/.local/share/agentrunner/`；session=
`20260721-074606-3-agent-3d7b48f9d77cccb6`；证据
`qa/runs/2026-07-22-QA83-sidebar-session-row-states/`）。

| # | 真实动作 | 硬断言 |
|---|---|---|
| 1 | 检查普通/managed-worktree resting row | worktree row 显示专用 marker；普通 row 无 marker；两者均无 row `…`；resting quick actions 隐藏 |
| 2 | keyboard focus session row | Pin/Archive 显示；背景覆盖 title 与 icons；其 computed color 与 current row 都是 `rgb(31,31,36)` |
| 3 | 点击 Pin，再右键 | Pinned section 出现；menu 文案切为 Unpin；随后 Unpin 恢复原偏好 |
| 4 | 右键与 `Shift+F10` | 两条入口均显示 Pin/Unpin、Rename、read state、Archive，且不导航 |
| 5 | 选中 session 并整页 reload | hash、thread/timeline、current class 与整行背景恢复，页面不 blank |
| 6 | 检查两个同名 `workspace` project | heading 均无 path subtitle；原生 title 分别为自己的完整 workspace；console warning/error=0 |

**动态态边界**：共享历史当时无 running/busy session，没有为截图新启模型任务；
spinner、running 时 spinner 与 quick actions 并存由 `Sidebar.nav.test.tsx` component
regression 锚定。全量 frontend **664/664**、production build 与 webui `go test ./...`
通过；视觉同屏比较无 P0/P1/P2，见项目根 `design-qa.md`。

**数据纪律**：验收前后 session count 均为 605；已恢复 Pin 偏好，未执行 Archive；
未创建、关闭、删除或清理 session/workspace/journal。
