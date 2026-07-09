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
| C8 interrupt vs 输入 | QA-02, QA-06 |
| C9 多模态 | QA-07, QA-09, QA-15（PDF/文件） |
| C10 恢复 | QA-08, QA-09(步骤6) |

反向索引（一个流盖多个 feature 的示例）：QA-09 一条压 8 个场景——它是
发布前的冒烟总闸；QA-01～08 是定位问题用的单项闸。

---

*执行纪律：每次跑完（无论过否）把 `ar events` 导出的 journal 与
workspace diff 归档到 `qa/runs/<日期>-<QA号>/`；FAIL 必须先归因
（runtime bug / prompt 不确定性 / 环境）再修。*
