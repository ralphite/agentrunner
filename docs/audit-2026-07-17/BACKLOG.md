# 审计 2026-07-17 实施 BACKLOG（loop 工作清单）

**协议（每次 loop 迭代）**：
1. 取下面第一个未勾选项（严格按序）。
2. b/c 类先判断是否需要工作纸：c 类必须先在 `docs/increments/` 落
   INC 工作纸（三层 delta + 验收 + 实施步骤）；触不变量的（明标）走
   PROCESS §四流程。a 类与工程债直接做。
3. 实施：代码 + 测试 + 文档行齐活；`GOTOOLCHAIN=go1.25.12
   ./scripts/check.sh` 全绿（本环境预装 go1.25.0 过不了 repo 的
   toolchain 门槛，golangci-lint 与 go1.26 不兼容，故钉 1.25.12）。
4. commit（`INC-<n>.<步>: <摘要>` 或 `audit-0717.<项>: <摘要>`）并
   push `origin/main`；同 commit 勾选本文件对应项、必要时同步
   SPEC/GAPS/LOG。
5. 一次迭代做不完的项：在该项下追加"进度："行记录断点，不勾选。
6. 卡住/需用户裁决的项：标 `[BLOCKED: 原因]` 跳过，继续下一项。

**规模图例**：S=小时级 M=天级 L=设计先行。

---

## 第 0 批 · 纯文档修正（a 类）

- [x] **A1** SPEC 附录"代码事实对照"补登（S）：CLI +11（`diff`
  `artifacts` `retry` `queue` `unqueue` `hook` `answer` `mode`
  `goal` `dictate` `optimize`）；daemon +8（`mode` `unqueue`
  `answer` `goal-attach/pause/resume/update/cancel`）；tool defs
  +8（`ask_user` `web_fetch` `progress_update` `send_message`
  `artifacts_list` `artifacts_read` `goal_complete` `goal_status`）；
  更新盘点日期；顺带订正 daemon.go:876 unknown-command 错误串漏项。
- [x] **A2** SPEC.md:163 "QA-59 待验"改为已 PASS（QA.md:911-913，
  2026-07-11）（S）。
- [x] **A3** GAPS G4 与 SPEC.md:71 冲突当场修：G4 标关闭或写明差异
  （PROCESS §一"两活文档冲突=缺陷"）（S）。
- [x] **A4** G33 现状核实并回标（机械加固已落地，GAPS.md:500-517）
  （S）。

## 第 1 批 · 小代码改动（b 类，各一个小增量）

- [x] **B1** G16 威胁模型成文（S，纯文档部分）：GAPS.md:193-200 的
  "workspace 内容不可信"统一信任分级条款写入 DESIGN 安全章；
  BEGIN/END 定界符改动另拆（见 B2）。
- [x] **B2** G16 定界符（S）：不可信内容注入用 BEGIN/END 文本定界
  （现为 JSON 兄弟布尔）。
- [x] **B3** G22c daemon kill -9 孤儿 bash pgid 清扫（S，
  SPEC.md:124、DESIGN §17 #6）。
- [x] **B4** G26 `ar inspect` children 按 call_id 去重（S，
  GAPS.md:459-461；webui 已做，收 CLI/契约侧）。
- [x] **B5** web_fetch host allowlist——核查发现 S1 **已有在案裁定**
  （LOG 2026-07-09:backlog,留待 G11 云形态),不实现,仅对齐 SPEC/
  GAPS 过时"待裁"措辞;allowlist 本体随 E2(G11) 重新立项。
- [x] **B6** webui SettingsConfiguration "Not surfaced" 接线（S-M）：
  daemon/webui `/health`（或专用只读端点）暴露 approval policy 与
  sandbox 现状，前端渲染真实值（SettingsConfiguration.tsx:57-62）。
- [x] **B7** webui mermaid 懒加载（S，SPEC.md:189 余项）。
- [x] **B8** G36 余项（S）：Scheduled 表单 interval/cron 内联校验 +
  错误 `.details` 披露 UI（GAPS.md:604-606）。
- [x] **B9** G10 bash 后台任务进度 tail（S-M，GAPS.md:291-294：
  复用 2.10 进度通道到后台 bash handle）。

## 第 2 批 · 登记簿工程债

- [x] **C1** G30 弱锚燃尽:31→2(24 行还真锚,2 行留债有因——
  composer 前端 it 名/用户消息折叠 jsdom 测不了,见 GAPS G30 注)。
- [x] **C2** G31 deadcode 甄别:17 项逐一落档——删 2(errs.Retryable
  自由函数/blackboard.Topics)、test-infra 注记 10、unwired 3 项转
  D0 工作纸(见 LOG 2026-07-18)。

## 第 3 批 · 中型增量（c 类，先工作纸后实施）

- [x] **D0** G31-unwired 三件套:INC-69 落地——注册表接线(5 站点
  改读 WaitRules)+ 脚手架/死码删 4,G31 关闭(工作纸已归档)。
- [ ] **D1** [BLOCKED: 需用户裁决——INC-D2/INC-50 在案定案"排队不
  解栈"的推翻属人裁] G3 唤醒语义:工作纸已备
  `docs/increments/INC-70-approval-park-wake.md`(A 维持记档 / B
  消息=转向式拒批(推荐) / C 不推荐),选定后按纸实施。
- [x] **D2** G2 barrier 在飞处置——核查为文档滞后(DESIGN §13+S7
  review P1 已落),对账关闭,无代码改动。
- [x] **D3** G1 blob 归属——同为文档滞后(DESIGN §13 随行库 verbatim
  复制裁决),对账关闭。
- [x] **D4** G22a boot 自动接续:INC-71 落地(第三类 boot sweep,
  决策 #29/#30 复用),孪生 4 例,工作纸归档。
- [x] **D5** G22b 优雅停机保活 cron:INC-72 落地(§四流程,cause 区分
  +loop-mode 无终态 teardown),G22 整条关闭。
- [x] **D6** 撤项:胜者晋升在 SPEC 已有 🧊 在案记档(v0 用户手动
  晋升,GAPS G15 显式推迟)且本文件附录亦列入"显式不做"——当初列入
  D 批系审计汇总重复,推翻 🧊 记档属人裁;如需做请在 G15 重新立项。
- [ ] **D7** [BLOCKED: 需用户裁决产品面] G13 SCM/PR 工作流(M-L)。
  裁决点:①平台绑定(GitHub 专属 vs 通用 SCM 抽象);②审阅门形态
  (webui Changes→Approve→push,或 PR 草稿先行);③"审阅通过才
  push"约束落 rules 还是新 gate。选定后出工作纸实施(顺带兑现
  SettingsGit 两项 Not wired)。
- [ ] **D8** [BLOCKED: 需用户选型+凭据] web search(M-L,G18)。
  裁决点:①后端(Brave/Tavily/SearXNG 自托管/provider 服务端工具
  (Gemini grounding——需破"客户端执行"例外并入 DESIGN 例外类别));
  ②凭据来源与红线(API key 放 env,与 GEMINI_API_KEY 同法);
  ③egress 语义(web_fetch 同款 execute-class+审批?)。选定后出工作纸。
- [ ] **D9** [BLOCKED: 本环境(Linux 容器)无法验证 macOS Seatbelt
  行为,双闸门 B 闸不可行] G32(M)。产品侧方案在 GAPS 有记
  (PATH 截击/host git 代理),需在真 macOS + Xcode.app 机器上开发
  验证——建议在用户 mac 上的 session 做。

## 第 4 批 · 大型（设计先行，每迭代一个可合并步骤）

- [ ] **E1** driver 收敛为递归 session(L,四步;进行中):
  ①✅ loop-mode 挂 session——**INC-74 完成**(74.1 事件族 7aa8e20 /
  74.2 安全点唤醒 bf37a1b / 74.3 CLI+wire+文档收口 23f39d9;B 闸
  QA-74 PASS,run 29634255244,工作纸已归档);
  ②iteration child 统一走 spawn_agent——**INC-76 工作纸已落**
  (docs/increments/INC-76-unified-child-run.md:子执行基座统一,
  三小步:基座+spawn 改走→driver 改走→收口+QA-70 回归),实施中;
  ③stream 合流(触 §3 教义,须 §四);④CLI 兼容层。
- [ ] **E2** [BLOCKED: L 级产品形态,设计需用户共创] G11 云
  workspace。核心裁决点:①环境模型(容器 per-session vs 长驻 pool);
  ②secrets 注入面(env 白名单 vs vault 引用);③store 外置
  (journal/CAS 挪对象存储?)④回收重建语义(workspace 可再生 vs
  持久卷)。每一项都是产品选择,建议专门 session 逐项裁决。

## 第 5 批 · 续作（用户 2026-07-18 指示:非裁决项持续推进）

- [x] **F1** QA-69 落地 PASS(真浏览器双锚),SPEC 还锚,
  spec-anchor-debt 清零,G30 关闭。
- [x] **F2** QA-70 脚本+workflow 就位(容器无 key,经 Actions
  secrets 执行);待 dispatch 真跑取证(见进度日志)。
- [x] **F3** 已定位修复:TestNewAndSendDetach 清理竞态(drain+
  settle 等待),-count=10 绿。

## 附 · 显式不做（🧊 记档，loop 跳过）+ 待裁决(见 DECISIONS-PENDING.md)

`ar new` 开场折叠/带图 · `finish` 工具 · overlap:interrupt ·
MCP 交互 OAuth · HTTP/WS 全 API 壳 · IDE 集成 · --add-dir 多根。

---

**进度日志**（loop 每迭代追加一行：日期 · 项 · commit · 状态）

- 2026-07-17 · 前置 · TestBashFilesystemSandbox 平台无关断言修复
  （Linux bwrap 语义，REVIEW.md 发现 #7）· 4bf220b · 完成。
- 2026-07-17 · A1–A4 · SPEC 附录补登 27 项 + daemon 错误串订正 +
  QA-59 回标 PASS + GAPS G4/G33 回标关闭 + LOG 台账 · 37a34e7 · 完成。
- 2026-07-17 · B1 · G16 统一信任分级条款成文 DESIGN §5,GAPS/SPEC 同步
  · 8d5d952 · 完成。
- 2026-07-17 · B2 · web_fetch content BEGIN/END 定界符(软标记入文本)
  · f516993 · 完成。
- 2026-07-17 · B3 · daemon boot 孤儿 bash 进程组清扫(标记+init-parent
  双证据,Linux/darwin) · 9b10ccc(经 rebase,原 9610ff5) · 完成。
- 2026-07-17 · B4 · inspect children 源头按 session/call_id 去重取最新,
  G26 关闭 · c5de9c1 · 完成。
- 2026-07-17 · B5 · host allowlist 裁决对账(已裁 backlog,不实现),
  SPEC/GAPS 措辞对齐,G16 收口 · 9c66210 · 完成。
- 2026-07-17 · B6 · Settings approval/sandbox 占位接线(/health 增
  sandboxBackend/Detected,前端真值渲染) · c4d9876+22cf7a1 · 完成。
- 2026-07-17 · B7 · mermaid 围栏懒加载渲染(单独 chunk,strict,回退
  代码块) · fb66858 · 完成。
- 2026-07-18 · B7.1 · embed 测试 helper 改取最大 js(code-split 小
  chunk 无 gz 变体致间歇红),fix-forward · b5a9107 · 完成。
- 2026-07-18 · B8 · schedule 内联校验(modal+launcher)+ toast Details
  披露(四类站点) · cb647ae · 完成。
- 2026-07-18 · B9 · bash 后台进度 tail(live tee+bgLog+bg_output
  ephemeral+output 工具 tail),G10 全关 · 321c683 · 完成。
- 2026-07-18 · 插入(用户指令) · check.sh 并行化提速:8min→53s,
  覆盖不减,重复 go vet 去除 · 0d3cd33 · 完成。
- 2026-07-18 · C1 · G30 弱锚燃尽 24/26(债 31→2,留 2 行有因) ·
  17ab2ff · 完成。
- 2026-07-18 · C2 · deadcode 17 项甄别(删 2/注记 10/unwired 3→D0)
  · 9cd689b · 完成。
- 2026-07-18 · D0/INC-69 · waiting 注册表接线+Discover 删除,G31 关闭
  · e3d74b5 · 完成。
- 2026-07-18 · D1 · BLOCKED:INC-70 工作纸落草案(A/B/C 选项),等
  用户裁决 · 53be817 · 跳过继续。
- 2026-07-18 · D2+D3 · 均为文档滞后,SPEC/GAPS 对账关闭(无代码) ·
  567020b · 完成。
- 2026-07-18 · D4/INC-71 · stranded session boot 自动接续,G22a 关闭 ·
  a287b78 · 完成。
- 2026-07-18 · D5/INC-72 · 优雅停机保活 cron(§四 不变量修订),G22
  整条关闭 · 27ef6d1 · 完成。
- 2026-07-18 · D6 撤项(🧊 在案)/D7 D8 E1 E2 BLOCKED(裁决点已列)/
  D9 BLOCKED(平台不可验证) · d73c5ad · loop 收口。
- 2026-07-18 · 用户指示:裁决项集中登记(DECISIONS-PENDING.md),
  非裁决项持续推进——E1 恢复可做,新增第 5 批 F1-F3 · 2eccea1 ·
  loop 重启。
- 2026-07-18 · F1/QA-69 · 真浏览器双锚 PASS,G30 债清零 · f7a7818 ·
  完成。
- 2026-07-18 · F2 QA-70 就位 + F3 瞬时红修复 · 22dc6db+8a8b7dc · 完成。
- 2026-07-18 · F2 追踪:run#1 误 kill 于 LLM 阶段(已证 INC-71 sweep
  真工作)、run#2 runner 缺 bwrap(workflow 已补装)、run#3 排队中。
- 2026-07-18 · E1① · INC-74 工作纸落盘,开始实施 · e2a57e0 · 进行中。
- 2026-07-18 · INC-74.1 · schedule 事件族(5 类)+ fold 子状态
  (copy-on-write)+ SubStateVersions "schedule":1 + round-trip 样本 +
  生命周期孪生 · 7aa8e20 · 完成(74.2 安全点唤醒 next)。
- 2026-07-18 · F2 收口 · QA-70 run#3 SUCCESS(真 Gemini:A 零 send
  自动接续 / B 优雅停机无终态复活),QA.md 登记+SPEC 挂锚 · 87461a9
  · 完成。
- 2026-07-18 · INC-74.2 · 安全点 schedule 检查:控制面四 kind +
  checkSchedule(到期重推/catch-up 折一/busy skip/幂等挂 timer)+
  awaitInput schedule-timer 唤醒 + close 撤 timer + 孪生六件 · 本
  commit · 完成(74.3 CLI/daemon wire + 文档收口 next)。
- 2026-07-18 · INC-74.3 · CLI `ar schedule`(attach/status/pause/
  resume/cancel,前置校验)+ daemon `schedule-*` wire(revive 同
  goal-*)+ SPEC A 表行/F 表注/DESIGN §13 两形态镜像+§17 E1① 注/
  GAPS/QA-74 登记 · 本 commit · 代码+文档完成,B 闸 Actions 真跑中。
- 2026-07-18 · E1① 收口 · QA-74 PASS(run 29634255244,真 Gemini:
  零 send 自主唤醒 → 跨 daemon 重启 sweep 唤醒 → pause 不再醒,三断言
  逐条核对)· QA/SPEC 登记,INC-74 工作纸归档 · 本 commit · E1① 完成
  (② iteration child 走 spawn_agent next,需工作纸)。
- 2026-07-18 · E1② · INC-76 工作纸落盘(子执行基座统一:spawn/driver
  两份"跑 child 到静止并结算"实现合一;Loop 构造与事实流合一显式留
  ③④)· 本 commit · 进行中(76.1 基座+spawn 改走 next)。
- 2026-07-18 · INC-76.1 · child-run 基座落 internal/agent(三态+fold
  结算),spawn 双路径+recovery reattach 三站点改走;childReport 语义
  分歧记档(报告读取不并入基座)· 本 commit · 完成(76.2 driver 改走
  next)。
