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
- [ ] **C2** G31 deadcode 甄别（M，可分多迭代）：
  `scripts/deadcode-baseline.txt` 19 个不可达导出逐项三选一
  （接线/删除/LOG 注记理由），基线只减不增。

## 第 3 批 · 中型增量（c 类，先工作纸后实施）

- [ ] **D1** G3 WAITING_APPROVAL 挂起期间消息唤醒语义（M，可能触
  park/唤醒不变量——工作纸里显式判定，触则走 PROCESS §四）。
- [ ] **D2** G2 barrier 对在飞 background/child work 的处置语义
  （M，时间旅行扩展层）。
- [ ] **D3** G1 多模态 blob 在 fork/rewind 下的归属语义（M）。
- [ ] **D4** G22a 中途崩溃 agent session 自动接续（M，受决策 #30
  标记约束）。
- [ ] **D5** G22b 优雅停机保活 cron（M，**明确走 DESIGN §四不变量
  变更流程**：driver 终态语义区分 shutdown-teardown 与用户 stop）。
- [ ] **D6** G15 best-of-N 胜者晋升（M：fork / apply diff 二选一
  设计 + 冲突处理）。
- [ ] **D7** G13 SCM/PR 工作流一等公民化（M-L：diff 审阅门→PR→
  元数据回填；顺带兑现 webui SettingsGit "Not wired" 两项）。
- [ ] **D8** web search 工具（M-L，G18 主项：搜索后端选型/凭据面/
  provider 服务端工具例外类别——选型若需用户裁决则先出工作纸再
  BLOCKED 等输入）。
- [ ] **D9** G32 Xcode.app 机器沙箱内 git 可用性（M，触 sandbox
  env/PATH 语义）。

## 第 4 批 · 大型（设计先行，每迭代一个可合并步骤）

- [ ] **E1** driver 子系统收敛为递归 session（L，SPEC.md:70、
  DESIGN §17 #4、UJ-22/G23：先工作纸拆步）。
- [ ] **E2** G11 云 workspace 生命周期（L，GAPS.md:299-304 ⚠️高：
  环境配置模型/setup 信任/secrets 注入/镜像缓存/per-env 网络/
  store 外置/回收重建语义；先工作纸拆步）。

## 附 · 显式不做（🧊 记档，loop 跳过）

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
  本 commit · 完成。
