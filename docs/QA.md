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
| 1 | Home / Scheduled / Projects | Home 恰一个 New task 主操作和 composer；真实历史按 workspace 分组；Scheduled 空态明确 |
| 2 | deep link/reload/Web UI restart | 父/子/CLI 创建的 session 仍可按 hash 恢复；workspace/title 来自 journal-backed `sessions --json` |
| 3 | waiting:approval `20260709-134832-use-bash-to-echo-notify-test-7bca` | 卡片显示 Run command / echo B / Current workspace，Details 默认折叠；未代用户决策 |
| 4 | team `20260710-021026-task-ce2c` | Supervision 中 engineer/reviewer 各恰一行；点 engineer 进入完整只读子会话（textbox=0） |
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

## QA-33 结构化输出 CLI 端到端（INC-26,#91,UJ-01）

（补登 2026-07-10：场景于 INC-26 收口时真机执行并记 LOG,菜单当时漏登——
lint-docs 幻影锚检查抓出。）

**环境**：真实 Gemini；workspace 放 7 行 `sample.txt`；schema 约束
`{lines:int, name:string}`。

| # | 动作 | 验证 |
|---|---|---|
| 1 | `ar new --json-schema <path>` 让模型数行 | 回复为符合 schema 的 JSON,客户端校验通过 |
| 2 | 输出 | canonical structured_output 打印 `{"lines":7,"name":"sample.txt"}` |
| 3 | 独立复核 | python 独立确认 schema 符合且值合理（name~sample、lines=7） |

**结果**：PASS 首验通过（LOG INC-26 条目）。

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
| 3 | existing team `20260710-021026-task-ce2c` | inspect 前不投影伪空态；稳定后 engineer/reviewer 各一行；program/agent messages 默认隐藏 |
| 4 | Web UI restart → Scheduled | 既有 driver 仍按 task/schedule/status 出现；driver 不在 Projects；scratch id 投影为 Scratch |
| 5 | New scheduled task / menu / search / Changes | 产品词在主层、YAML 在 Advanced；dialog/menu/button/focus/Escape/方向键成立；非 Git 空态与审批主层不泄漏绝对路径 |
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
**结果**：待真机复验填充。

## QA-41 Codex 式任务收尾与首屏真相（INC-38,UJ-24）

**环境**：最新 `main`、共享 `~/.local/share/agentrunner/` store/daemon、
`http://127.0.0.1:8788`；默认 desktop 与 390×844。只读既有 session/diff，
未 send/approve/resume/close/commit/清理。证据保留在
`qa/runs/2026-07-10-webui-codex-detail-audit/`。

| # | 真实状态/动作 | 硬断言 |
|---|---|---|
| 1 | Web UI 不可用→恢复、deep link 首屏 | 旧版实证曾投影 `No tasks yet`+raw sid；加入 readiness 后代码/DOM 只在首个 sessions success 后允许真实空态，header 先 `Loading task…` |
| 2 | completed session `…qa-k-diff…` | journal timestamp 投影 `Worked for 9s`；最终 answer 只有一条 Worked；Copy 与 `Continue in new task` 可达 |
| 3 | 有真实 diff 的 completed goal session `20260710-062102-…-0d1e` | `Worked for 1m 17s`；内联 `Edited 1 file +1 / goal-r2.txt +1`；Review 进入原 Changes 并显示 `+DONE` |
| 4 | New task desktop / 390×844 | project/Local/branch 环境条常驻 composer 上缘；同一 trigger 打开 Recent/workspace/Interactive/Background/branch；mobile 不溢出 |
| 5 | sidebar task | 每行具 pin/archive hover action；hover preview 由 session workspace/status 与按需 git branch 查询组成；键盘 context menu 原路径不变 |
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
| 2 | New task desktop / 642px / 390×844 | 四个 popover 独立打开；Project/Branch 搜索、selected check、Escape focus return、ArrowDown/Home/End 与 viewport containment 成立；access/model/mic/send 均在视口内 |
| 3 | Project=`agentrunner`、New worktree、Branch=`main` 提交真实任务 | 后端从 selected ref 创建 `wt-20260710-143427`；session `20260710-213428-create-qa42-worktree-browser-t-d8ac` 完成，唯一改动 `qa42-worktree-browser.txt`，内容严格为 `QA42_WORKTREE_OK` |
| 4 | 390×844 approval | inline approval 是主操作；Supervision 不自动覆盖；从宽屏 resize 到窄屏同样撤回自动面板 |
| 5 | completed → Changes → Continue in new task | `Worked for 3m 46s` 在最终 answer 前；Changes 显示文件和 `QA42_WORKTREE_OK` 且无 Supervision；Continue 对话框可选择 checkpoint，原任务不变 |
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

## QA-50 外部事件唤醒 webhook ingress（INC-50,#E2,G14,UJ-12）

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

## QA-51 LLM 自动会话标题（INC-52,HANDA #14,UJ-24）

**环境**：共享 daemon + store + 真实 webui + 真实 Gemini（auto-title 仅顶层
托管 session 启用，须走 daemon 而非 headless 一次性 run）。跑完**不** close/
删除；`ar events <sid>` 导出与 webui 截图归档 `qa/runs/<日期>-INC52/`。

| # | 动作 | 硬断言（journal/UI 红线） |
|---|---|---|
| 1 | webui New task 发一条**长**多行 prompt（>48 字符），等首条回复 | 开局回复**不被延迟**（title 调用在 assistant 消息之后的安全边界，异步于开局回复） |
| 2 | `ar events <sid>` | 出现恰一条 `session_titled{source:"auto"}` + 一条 `activity{kind:llm,name:autotitle}`（usage 计入 budget）；title 非首行截断而是精简短句 |
| 3 | webui 侧栏 | 该会话显示精简短标题（`sessions list --json` 的 title = RawTitle）；旧 session（无事件）仍显示首行/派生 fallback |
| 4 | webui 手动 rename 该会话 → 触发再次静止/唤醒 | 标题变手动值；`session_titled` 不新增第二条；auto **不覆盖** manual（rename 仍 localStorage，displayTitle 胜出） |
| 5 | 断网/坏 key 复跑一条长 prompt | 会话正常完成、不 abort；无 `session_titled`；title 回退首行 |

**结果**：待验（reviewer 集中跑）。锚孪生（A 闸已绿）：
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
PASS（session `20260709-234601-task-381f`,协作期含成员互发消息与多次
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
| 1 | 真 Gemini lead 动态 spawn 一个 engineer，`agent_workspace: isolated` | `inspect.team_tasks` 为 quiescent，持久化 task/lease/member/workspace/base_ref；父 workspace 无目标文件，子 worktree 内容精确为 `REAL-ISOLATED-OK` |
| 2 | 检查 root/child store 与最新 fold snapshot，再对静止 session 执行当前二进制 `resume` | 两侧 `events.idx` 存在；snapshot 有非零 offset+64位 hash；resume rc=0 且 events 行数 48→48（只续 fold、不重跑） |
| 3 | 浏览器打开父 session 与 child hash URL | 父页显示 Subagents·1/完成回执/子链接；子页显示 read-only sub-task 与 write/message 链；console error/warning=0 |

**结果**：PASS。session
`20260710-000426-execute-the-team-task-now-exac-9c59` 与全部 workspace/journal
保留。有限树预算在 child reservation 存续期间会显示 transient
`limit exceeded`，settlement 后正常完成；这是既有 reserve-then-settle 的
可见表现，不是重复执行或预算越界。

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
