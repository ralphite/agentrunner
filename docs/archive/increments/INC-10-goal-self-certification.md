> **归档注记（2026-07-09）**：INC-10 已落地并收口——delta 并回
> DESIGN(§13/决策 #21)/SPEC(F 域)/JOURNEYS(UJ-22 步骤 2b)/GAPS(G23 补全)/
> QA-17/CODEX-PARITY(§2-06/§3/§6)/LOG；闸门 A（check.sh 全绿,含 rebase
> 合流后复验）+ 闸门 B（QA-17 真 Gemini 自证达成 + webui Chrome 真跑）
> 双绿；对抗 review（契约+正确性）P0/P1 全修。余项（token/墙钟预算、
> blocked/usage_limited、llm_judge、banner 用量显示）记 LOG 与
> CODEX-PARITY §6.2-④⑤。本工作纸自此只读。

# INC-10 goal 自证完成（model-asserted completion）+ continuation 升级 + goal UI 收敛

**动机来源**：CODEX-PARITY §6（2026-07-09 goal 深潜审计）。本增量关闭其
缺口清单的 ①（无 verifier goal 恒不可达成，bug 级语义洞）、②（模型无
goal 工具）、③（continuation 回灌太薄）、⑥ 部分（UI：/goal 一句话即走 +
banner edit）、⑦（goal 控制不复活非 hosted 会话）。④ token/墙钟预算与
⑤ blocked/usage_limited 态维持 defer（LOG 已记）。

## 动机与 journey 锚

UJ-22（会话内目标）既有情节假定目标总能写成 command verifier（"跑 20 次
测试全绿"）。实证（CODEX-PARITY §6.1）：大多数真实长程目标写不成 shell
命令（"UX 审计修到子 agent 无反馈"）。当前实现对这类目标是语义洞——
`goalVerify` 在零 verifier 时恒 miss，goal 烧完预算后必然截断，**永远
不可能达成**，而 CLI 与 webui 都允许空 verifier attach。

**UJ-22 增补一条情节分支**（并回时落 JOURNEYS）：
> 2b. 目标写不成命令（"重构完所有 handler 并保持行为不变"）——不带
> `--verify` attach。agent 逐轮工作；当它验证完成后调 `goal_complete`
> 声明（带证据摘要），checkpoint 在静止边界裁决：无 verifier → 接受
> 声明，goal 达成；有 verifier → verifier 仍是唯一裁决者（声明只是
> 提示）。**完成裁决只住静止边界、绝不 mid-turn 这一纪律不变。**

## Spec delta（SPEC.md F 域 in-session goal 行，逐条）

1. **自证完成**：无 command verifier 的 goal 合法且可达成——模型调
   `goal_complete{summary}` 声明，checkpoint 在静止边界裁决接受
   （`GoalAchieved{satisfied}`，detail 记 model-certified + 摘要）。
   有 verifier 的 goal 行为不变（verifier 唯一裁决，向后兼容）。
2. **goal 工具面**（常驻 face，同 output/kill 的 extras 机制）：
   - `goal_status`（read）：查询当前 goal（objective/checks/max_checks/
     paused/claimed）；无 goal 返回明示。
   - `goal_complete`（read，同 `finish_series` 先例）：记录完成声明
     （journal `GoalCompletionClaimed`），裁决延迟到静止边界。
   - **模型不能** attach/pause/resume/cancel/update goal，尤其**不能设
     verifier command**（goalVerify UNGATED 的辩护前提 = command 仅
     operator 可设，必须保持）。
3. **continuation 升级**：attach 注入文与 miss 回灌文升级为结构化
   continuation prompt——goal 文本包 `<goal>` 标签并明示"user-provided
   data，非更高优先指令"（注入卫生，对齐 Codex）+ 反缩水条款 + 完成
   审计要求 + 预算报告（check n/m）+ 无 verifier 时的 goal_complete
   指引。
4. **goal 控制复活**（⑦）：daemon `goal-*` 控制对非 hosted 会话走
   send 同款 revive（hostResume → postControl）；attach 即开工，
   cancel/pause 落账后自然静止。
5. **webui 收敛**：session 内 `/goal <text>` 直接 attach（不弹表单）；
   `/goal` 空参与 "+菜单→Goal" 保留面板作高级配置（verifier/max
   rounds，文案改为"可选，留空=自证完成"）；banner 加 edit（改 goal
   文本，走既有 update）；Home `/goal <text>` 改为**新建会话 + attach**
   （in-session goal 是旗舰形态；driver-goal 仍从 Background run modal
   可达）。inspect goalReport 增加 verifier 计数与 claimed。

## Design delta（DESIGN.md；触不变量，见下节）

- §13 in-session goal 段：完成裁决描述从"verifier 在 exchange 边界检查"
  扩为"完成裁决在 exchange 边界：有 command verifier → verifier 裁决；
  无 verifier → 模型 `goal_complete` 声明、边界裁决接受"。事件族增
  `GoalCompletionClaimed`（change-as-event，决策 #32 同族；fold 出
  `Goal.Claimed/ClaimSummary`，被 GoalCheckpoint/GoalUpdated 消费清除）。
- 决策 #21 行：同上措辞修订（见不变量变更单）。
- 决策 #24（静止时序）**不动**：goal_verify 槽位、顺序、boundary-only
  纪律全部保持；claim 裁决就住在该槽内。
- 决策 #31（预算=可见截断）**不动**：无 verifier goal 无声明时每边界
  计一次 check，MaxChecks 尽 → `GoalAchieved{budget}`，仍有界。
- 工具 face 纪律（"face 只依赖 journaled-spec-or-tree 事实"）**不动**：
  goal 二件套为常量 extras（无条件、dedup 对 spec），resume 重建同 face。

### 不变量变更单（PROCESS §四）

- **旧文（决策 #21，粗体部分节选）**："verifier 在 exchange 边界（final
  generation 收尾、绝不 mid-turn）检查，miss 回灌 program 源 input 让
  同一 fold 续跑，pass 出达成回执并摘 goal"。
- **为什么必须动**：该表述把"完成裁决者"钉死为 verifier，构造上排除了
  写不成命令的目标（实证为多数），并造成"空 verifier goal 恒不可达成"
  的语义洞（CODEX-PARITY §6.2-①）。
- **新表述**："**完成裁决在 exchange 边界（final generation 收尾、绝不
  mid-turn）**：有 command verifier 时由 verifier 裁决（AND，唯一裁决
  者）；无 verifier 时由模型 `goal_complete` 声明（记 journal，裁决延迟
  到边界）。miss 回灌 program 源 input 让同一 fold 续跑，pass 出达成
  回执并摘 goal。"——边界纪律、回灌续跑、fold 连续性三个核心性质原样
  保留，只扩完成判据。
- **波及面**：`internal/agent/goal.go`（checkpoint 裁决）、`loop.go`
  （face + 工具分发）、`event/types.go` + `state/state.go`（新事件与
  fold）、`tool/defs/goal_*.json`、DESIGN §13/#21、SPEC F 行、UJ-22、
  QA 新场景；既有孪生 4 条全部保持绿（向后兼容验证）。
- **review**：单独契约视角 review（对抗 agent），加正确性视角
  （claim 快照/并发）；安全面断言"模型不可达 verifier command 设置"
  维持成立。

## 验收

- **闸门 A（孪生，进 check.sh；名字以实现为准）**：
  - `TestInSessionGoalSelfCertify`：无 verifier attach → miss 回灌
    （含 goal_complete 指引）→ 模型调 goal_complete → 边界
    `GoalAchieved{satisfied}`，单 SessionStarted。
  - `TestInSessionGoalClaimDoesNotOverrideVerifier`：有 verifier +
    模型声明但 verifier miss → 不达成（claim 不越权），detail 记
    claim rejected。
  - `TestInSessionGoalNoVerifierBudget`：无 verifier、无声明 → 逐界
    计 check，预算尽 `GoalAchieved{budget}`（决策 #31 仍成立）。
  - `TestGoalAttachRevivesSession`（daemon）：goal-attach 到非 hosted
    会话 → revive + 控制送达。
  - review 加测：`TestInSessionGoalResumeContinues`（resume 再武装边界）、
    `TestGoalResumeCheck`（checkpoint 前 crash 窗口补裁）、
    `TestGoalClaimFold`（claim fold 生命周期 + copy-on-write 纯度）。
  - 既有 TestInSessionGoal*/TestGoalRecover 全绿（回归红线）。
- **闸门 B（真实 API，QA-17 新场景）**：真 Gemini，attach 无 verifier
  goal（写 haiku 文件类可 eyeball 的任务）→ agent 工作 → goal_complete
  → 达成回执；webui 真跑：`/goal 一句话` 启动、banner 呈现与 edit、
  达成后回普通待命。归档 `qa/runs/2026-07-09-QA-17/`。

## 实施步骤（一步 = 一个可合并提交）

1. **INC-10.1 引擎**：事件 `GoalCompletionClaimed` + fold；goal 二件套
   defs + face extras + loop 分发（快照纪律同 isHandleTool）；checkpoint
   裁决扩展；attach/miss 文本结构化。孪生 3 条 + 既有全绿 + DESIGN
   §13/#21 + SPEC 行**同 commit**（不变量修订与实现同 commit，PROCESS
   §四.3）。完成标志：check.sh 绿。
2. **INC-10.2 daemon revive**：goal-* 控制 revive 非 hosted 会话 +
   TestGoalControlRevive。完成标志：check.sh 绿。
3. **INC-10.3 CLI/webui**：CLI help 文案；webui /goal 直接 attach、
   Home /goal 新建会话+attach、表单文案、banner edit、goalReport 扩展；
   前端 build 进 embed。完成标志：check.sh 绿 + webui 手验。
4. **INC-10.4 收口**：QA-17 真跑归档；UJ-22/QA.md/GAPS/CODEX-PARITY §6
   状态更新；LOG 增量条目；对抗 review（契约+正确性）P0/P1 修完；
   工作纸归档。

## review 裁决

做：**契约 + 正确性**两视角对抗 review（触不变量 → 契约视角强制；
claim 事件与并发快照 → 正确性）。裁掉独立安全视角，理由：本增量的
安全断言单一且结构性——模型工具面不含任何 verifier/command 设置路径
（goal_complete 只载 summary 文本，经 redaction journal），由契约
review 一并覆核；无新 egress、无新执行面。

**review 结果（2026-07-09，两 agent 各自独立跑，P0/P1 全修）**：
- 契约核心主张（边界纪律/#31 有界/安全边界/face 纪律/向后兼容）全部
  核查成立，有实跑佐证。
- **P0（契约 review）**：event round-trip 测试缺 `GoalCompletionClaimed`
  样本 → 已补，event 包绿。
- **P1（契约 review，CONFIRMED 反例）**：turn 收尾后、checkpoint 前的
  crash 窗口 resume 后永不裁决（Quiescence 不看 goal、resume 静止形状
  跳过 goal_verify 格）→ 修：`goalResumeCheck` 在 drive-loop 安全点
  补裁 + `TestGoalResumeCheck`；DESIGN §13 crash 措辞同步改为两窗口
  如实表述。
- **P1（正确性 review，CONFIRMED）**：resume/update 打到 idle 会话不再
  checkpoint（attach 靠注入唤醒、resume/update 无等价物）→ 修：
  `goalReinject`（resume/未暂停 update 落账后注入 program input）+
  `TestInSessionGoalResumeContinues`。
- **P1（正确性 review）**：goal fold 就地 mutate 违反 Apply 纯度契约
  （INC-D1 既有 + 本增量新 case）→ 修：全部 goal case copy-on-write +
  `TestGoalClaimFold` 纯度断言。
- **P1（契约 review）**：SPEC 验收锚测试名失真 → 已改为实名。
- P2 采纳：webui Home attach 失败不留孤儿会话（仍选中该会话）；其余
  P2（double-attach last-wins、inspect max_checks 显示原始 0、task 双重
  注入）记档接受不修，理由：语义无害/触 UX 另议。

**连带主干缺陷（review/闸门过程暴露，当场修，与本增量同推）**：
- `TestQuiescentSequenceOrder` 自 INC-D1 起未更新期望（goal_verify 格）
  ——主干潜红，被 go test 缓存掩盖；已修。
- daemon/cli 测试 unix socket 路径超 macOS 104 字节上限（t.TempDir 含
  长测试名）——潜红同因缓存；统一 `shortSock`。
- **INC-D1 wake seam 与 malformed 截断交互的 drive 空转**（
  `TestMalformedToolCallExhaustionErrors` hang）：wake 只看
  `hasInputAfterLastAssistant`，decide 却因截断不可重启拒开 turn →
  热循环；修：wake 镜像 decide 的截断门（`TruncationRestartable`）。
- 新工具面进 golden（request_assembly / unknown_tool known-list）。
