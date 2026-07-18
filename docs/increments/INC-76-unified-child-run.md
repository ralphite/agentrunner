# INC-76 子执行基座统一:iteration child 走 spawn 同机制(E1 步骤②)

E1(driver 收敛为递归 session,DESIGN §17 在案方向)四步的第二步。
①(INC-74 in-session schedule)已收口。本步**不动**驱动语义与事实流:
driver 的 `Iteration*` 事件族原样保留(事实流合一是步骤③,触 §3 教义
须走 §四)——本步收敛的是**子执行基座**:今天"把一个 child session
跑到静止并结算"这件事有两份实现,分别活在 spawn 路径与 driver 里。

## 动机与 journey 锚

UJ-14/15/22(§3"一套机制"教义,§17 收敛注记)。现状重复面
(2026-07-18 侦查,行号见当日 main):

| 关切 | spawn 路径(internal/agent/spawn.go) | driver(internal/driver/driver.go) |
|---|---|---|
| per-attempt 子目录 | `sub/<call>-aN` | `sub/iter-N(-aM)` |
| store 开/关 + run | `childLoopWithExec` + `child.Run` | `NewChild` 工厂 + `child.Run` |
| crash 后处置 | recovery.go in-doubt 纪律(fresh dir per attempt) | `LastSeq>0` → settledChild 捷径 / `child.Resume` |
| 静止结算 | `childFoldUsage`(错误路径 fold 兜底) | `settledChild` + `childSpent`(fold 读用量) |
| 报告读取 | `childReport(childDir)` | verifier/carry 侧自读 |

两份实现漂移的真实代价已经付过:INC-71/72 修 daemon 生命周期时,
settle-from-shape 的正确性要在两处分别核对;S5/S6 review 各自补过
"失败 child 未结算用量"同一类 bug。

## 目标形态(本步)

**一个 child-run 基座**,两个调用方:

- `internal/agent` 新增 childrun 基座(建议 `childrun.go`):
  `RunChildToQuiescence(ctx, dir, prompt, build func(*store.EventStore) *Loop)
  (RunResult, provider.Usage, error)` 拥有:
  1. store 开/关(defer close,错误路径不漏);
  2. **三态决策**:journal 已静止 → 从静止形状结算(不重跑,决策
     #29/#31);journal 非空未静止 → `Resume`(in-doubt 纪律在 child
     自身);空 → `Run(prompt)`;
  3. 结算:成功取 RunResult;错误路径从 child fold 读实际花费
     (childFoldUsage 与 childSpent 合一,只留一个 fold 读用量的
     函数)——树预算诚实(S5/S6 review 既有条款)。
- **spawn 路径改走基座**:buildHandoffRun / launchBackgroundSpawn 的
  child.Run + 错误 fold 兜底段替换为基座调用(SpawnRequested/
  SubagentCompleted/Activity bracket 等事实照旧,由调用方在基座外
  记账——基座只管"跑到静止拿结果")。
- **driver.runIteration 改走基座**:settledChild/Resume/Run 三态与
  childSpent 替换为基座调用;`Iteration*` 事实、on_child_failure
  retry 语义、per-attempt 目录命名、worktree(best-of-N)全部不变
  ——retry 循环仍在 driver,基座只被每个 attempt 调一次。
- `settledChild` / `childReport` / fold-usage 读取收敛为单份
  (settledChild 语义并入基座三态;childReport 移基座旁共享)。

**显式不做(留给 ③④)**:Loop 构造(工厂/闸继承)不统一——spawn 的
childLoopWithExec 依赖父 Loop 的 gates 重绑,driver 无父 Loop(cli/
daemon.go 的 NewChild 工厂从 driver spec 起 Loop),两者构造语义就该
不同,强并是伪统一;事实流(Iteration* vs SpawnRequested/
SubagentCompleted)合一是步骤③。

## 不变量核对

- 不触 §3 条款本文(是向其推进的机械收敛,无语义新增);
- 决策 #29(单一自愈/in-doubt 按类)、#31(静止是形状非事件)原样
  ——基座三态就是这两条的现行实现,收敛后单点核对;
- 预算条款(失败 child 结算实际花费)原样,实现合一;
- 零行为变更目标:全部既有孪生(driver suites + spawn/bgspawn/
  recovery suites)不改断言全绿即是 A 闸主体。

## Spec delta

SPEC F 表 verifier/loop 行不动;附录"代码事实对照"无新命令/工具。
F 表 driver-goal 行验收锚补"子执行基座统一(INC-76)"注。

## Design delta

DESIGN §17 "§3 收敛进度"条:步骤②由"待续"改"已落(INC-76,子执行
基座单份;Loop 构造与事实流合一留 ③④)"。

## 验收

A 闸:既有 driver/spawn/recovery 全套孪生**不改断言**全绿(行为保持
的主证据);新增孪生 TestChildRunThreeWayDecision(空 journal → Run/
未静止 → Resume/已静止 → 不重跑直接结算,fake 场景各一)+
TestChildRunSettlesUsageOnError(错误路径 fold 兜底单点)。
B 闸:纯重构无新行为,以 QA-70(daemon 生命周期,覆盖 driver crash
收编/复活)Actions 回归一跑为准——它走的正是被改写的 runIteration
路径。

## 实施步骤

1. ✅ INC-76.1:基座落 `internal/agent/childrun.go`(openChildRun/
   store/close + settled + run 三态,spent 一律 fold 读);**三个**
   agent 侧站点改走:buildHandoffRun、launchBackgroundSpawn(Loop
   构造留在 drive goroutine——它读父状态,基座只收"跑到静止")、
   recovery.reattachWaitingChildren(revive baseline 减法留调用方,
   基座返回 fold 累计值)。孪生 TestChildRunThreeWayDecision(空→Run/
   非静止→Resume/已静止→零 provider 调用零新事件)+
   TestChildRunSettlesUsageOnError;spawn/bgspawn/recovery/driver
   全套既有孪生不改断言全绿。
2. INC-76.2:driver.runIteration 改走基座(retry/事实/命名不变),
   删 driver 侧 settledChild/childSpent 重复实现;driver 孪生全绿。
3. INC-76.3:文档收口(§17/SPEC F 注/LOG)+ QA-70 回归 dispatch。

## 实施中发现的语义分歧(76.1 记档)

`childReport` 两份实现**语义不同**,按本纸条款记档、不静默择一:
agent 版取**末条 assistant 消息的首个 text part**(spawn 报告——
"最后一轮说了什么");driver 版取**全对话最后一个非空 text part**
(carry excerpt——"最后一段产出文本",跨消息兜底)。消费者不同、
两个定义各自合理:**报告读取不并入基座**,76.2 driver 改走基座时
保留 driver 版 childReport 原样。若 ③ 事实流合一时要统一,届时以
"parent 可见报告"语义单独裁。

## review 裁决

中型纯重构,不触不变量(逐条核对如上):裁掉三视角 review,以
"既有孪生不改断言全绿 + 新增三态孪生 + QA-70 回归"为界;若实施中
发现两处实现存在**语义分歧**(不是重复而是分叉),停下把分歧写进
本纸再裁,不静默择一。
