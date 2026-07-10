# INC-30 团队 workspace 语义接通与弃子回收(G24/G25)

> **认领**:INC-23 走查方 session,2026-07-10。并发方请勿重复实施;
> 进度见文末执行记录。

## 动机与 journey 锚

INC-23 走查在 UJ-23(工程团队模拟)真实场景上实锤两个高危缺陷
(GAPS G24/G25),本增量根修。证据 session
`20260710-043858-task-44c3` 的完整事实链(比走查时的初判更严重):

1. engineer(isolated 子)把 hello.py 写进**自己的 worktree**——
   isolated 子产出**没有任何自动回流机制**(全仓核实:无 sync-back
   代码路径);
2. reviewer 在 engineer 静止前 spawn,快照来自**父 workspace**(当时
   为空)→ 永远看不到队友产出,发消息求助;
3. **父 agent 自救**:自己 `write_file hello.py`(seq 110)手抄进父
   workspace——用户后来在 Changes 里看到的文件是这份手抄件,不是
   团队机制交付的;
4. 父另起 reviewer 二号(新快照已含手抄件,验证通过),但**没有 kill
   一号**;一号被邮件 revive(原空 worktree、满血 40 步新预算)继续
   迷路,总计烧掉 80 步 / 195k tokens 才撞上限。

根因不是隔离机制坏,而是**设计已有的协作原语没有被接通**:
- 快照/隔离语义(spawn 时刻、无回流)对模型完全不可见——
  `spawn_agent` 工具 schema 一字未提;
- 子 agent 自身也不知道自己在快照里——反复找文件而不是问;
- 制品正道(`publish_artifact` → spawn `inputs`)与 shared 模式
  (`agent_workspace: shared`,Team Lead persona 已带)存在但不可发现;
- 替换成员没有回收原语——`kill` 存在但模型不用,`spawn_agent` 无
  `replaces`,team task 按 CallID 记账(再委派=新任务,永不取消旧
  lease)。

Journey 锚:UJ-23 步骤 3/4/5(成员协作、静止唤醒、评审往复)与
UJ-18(编排底座)。不新增 journey;把 UJ-23 场景 5 "制品经
artifact/blackboard 留痕"的既有意图落到可发现、可依赖。

## 侦察事实(实现锚点,已核实)

- 物化:`spawn.go:310 prepareChildExecutor`——isolated 从**父 loop**
  的 Snapshots(shadow repo)`Snapshot()+Materialize()` 到
  `<store>/sub/<call>-a<n>/worktree`;shared 直接复用父 Executor。
  判定看**父 spec** 的 `agent_workspace`。
- 无 sync-back:全仓 `Materialize` 仅 4 处(spawn/fork/best-of-N/
  接口),无任何 child→parent 回拷;`SubagentCompleted` fold 只记
  账不搬文件。
- revive:`revive.go:386 childExecutorFromJournal` 重开原 worktree
  **原内容**,不重物化(SPEC 已钉
  `TestIsolatedTeamWorkspaceSurvivesRevive`,保成员半成品,合理)。
- team task:`task_id = "task-"+CallID`(非文本 hash);
  `teamSettle`/`teamRevive` 按 CallID;再委派新增条目,旧 lease 不动。
- 工具面:`spawn_agent{agent,role,task,inputs,depends_on}`、
  `kill{handle}`;无 replaces。`depends_on` 是"依赖必须已静止"的
  fail-fast 校验,但**不解决文件流**(快照源是父 ws,依赖的产出不在
  那里)。
- worker 无 limits → 吃 runtime 默认 40 步;revive 再给满血预算。

## 方案裁决

**哲学:不动隔离不变量,把已有设计接通。** isolated 语义、revive
复用原内容、spawn 非阻塞、快照教义全部保持;修的是**可见性**
(把规则告诉模型与子 agent)、**可发现性**(正道原语进 schema 文案)
与**回收原语**(replaces)。

裁掉的路线及理由:
- revive 时重物化/叠加父新文件——毁成员半成品,违反已钉验收,且
  首次 spawn 的时序问题依旧;
- spawn 前自动等兄弟静止——违反"spawn 一律非阻塞"不变量;
- 自动 idle 熔断/父静止杀子——后台子的合法长跑无法与空转区分,
  需要新不变量,收益低于显式 replaces;
- 按 task 文本相似度取消旧 lease——不可靠且魔法。

## Spec delta(SPEC.md)

- 「durable team task/DAG/lease + workspace assignment」行补:
  `spawn_agent.replaces` 显式替换回收(cancel-then-spawn,幂等);
  isolated 语义与制品正道进入工具契约文案与子任务注入。验收锚:
  TestSpawnReplacesCancelsPredecessor / TestIsolatedChildTaskNotice。
- 「Web UI composer/persona」:Dev persona 描述注明 worker 隔离语义
  并指路 Team Lead(shared);webui worker spec 加保守
  `limits.max_generation_steps: 24`(纯 spec 数据)。

## Design delta(DESIGN.md)

- §18.6 「child workspace」行**澄清性增补**(语义不变,把隐含事实
  写显):"isolated 子的文件改动不自动回流父 workspace;产出经
  publish_artifact/消息/父转运。" 不触发不变量变更流程——没有旧
  语义被推翻,只是把"从未承诺 sync-back"写明。
- §18.6 「spawn」行补 `replaces`:spawn 携带的显式替换语义——
  runtime 先对旧 handle 走既有 kill(parent)路径(旧子
  `SubagentCompleted{cancelled}`、lease 结算),再 SpawnRequested。
  additive 工具面扩展,记 §15 决策注记一条。

## 实施步骤(一步一提交)

1. **INC-30.1 机制可见性**:
   - `defs/spawn_agent.json`:description 与 task/inputs/depends_on
     字段文案补 isolated 快照语义、无回流事实、制品正道、
     "replace 前先回收";`defs/kill.json` 补"停止仍在花预算的成员"。
   - `spawn.go`:isolated 子(仅 isolated)的 task 前注入一段机制
     说明(workspace=spawn 时刻快照、队友后续改动不可见、找不到
     文件就报告父而不是反复搜索)。
   - 孪生:TestIsolatedChildTaskCarriesSnapshotNotice(shared 不注入)。
2. **INC-30.2 replaces 回收**:
   - schema 加 `replaces`(string,旧 handle);`planSpawn` 解析;
     spawn 执行前经既有 cancel 路径回收旧 handle(不存在/已静止则
     幂等跳过,journal 留 cancel 标记)。
   - 孪生:TestSpawnReplacesCancelsPredecessor /
     TestSpawnReplacesQuiescentIsNoop。
3. **INC-30.3 webui 引导**:Dev persona 文案、worker limits: 24、
   前端构建。
4. **INC-30.4 闸门 B + 收口**:真实 API 重演 UJ-23 双人团队场景
   (Dev/isolated 与 Team Lead/shared 对照),断言:reviewer 缺文件
   时一步报告(不空转);replaces 回收旧成员;Team Lead 路径文件
   直接可见。三层文档并回、GAPS G24/G25 关闭注记、LOG、QA.md 登记、
   工作纸归档。

## 验收

- 闸门 A:上述孪生进 check.sh 常跑,全绿。
- 闸门 B:`qa/runs/2026-07-10-INC30/`,真 Gemini:
  - 场景 1(Dev,isolated):重演原任务。断言 runtime 红线:子任务
    含机制注入;若模型重派成员,旧 handle 收到 cancel(journal 可
    见)或成员一步回报缺文件——不再出现 40+ 步空转。
  - 场景 2(Team Lead,shared):同任务,断言成员写的文件在父
    workspace 直接可见(无手抄)。

## review 裁决

runtime 触点集中在 spawn 路径(注入 + replaces),不触不变量;
按小增量裁掉三视角对抗 review,以孪生 + 真实 QA 双闸门代替。
`replaces` 的 additive 语义在 DESIGN §15 记一条决策注记。

---

## 执行记录

- 2026-07-10 认领,侦察完成(本文"侦察事实"节),方案裁决如上。
