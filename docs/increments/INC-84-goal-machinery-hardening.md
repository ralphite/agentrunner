# INC-84 goal 机制加固:判据校验、pause 持久化、fork 继承、one-shot 挂死

状态:**提案(待裁决)** · 来源:QA v2sim(2026-07-19/20,
docs/audit-2026-07-19-v2sim/)四项相互佐证的现场证据。

## 动机与 journey 锚

UJ-15(通宵冲目标)/UJ-22(会话内 goal)。真机复现的四个缺陷让
goal 在真实使用中一旦配置失误就变成"逃不出去的死局"(用户 iPhone
实测:假终态横幅 → Continue fork → 子会话原地复制死局):

1. **verifier 判据不校验**(v2sim L4-I1,P1):attach 时自然语言串
   (`"go vet ./... 无任何输出 并且 … 全部 PASS"`)被当 bash command
   逐代执行,必然失败 → goal 永远验不过,每代 re-fire 一条
   goal-verify 审批污染审批流。可执行命令路径已被 L2b 反向验证为
   健康(fire 一次、干净收敛)。
2. **pause 不跨 daemon 重启**(L4-I3 修订定性):daemon 对
   goal-pause 的回执是固定文案("a no-op unless a goal is
   attached"),pause 实际是否生效不可观测;实测 pause 后经 daemon
   重启,goal-verify 在新 daemon 里继续 re-fire(anchor 会话
   2026-07-20T00:13 事件铁证)。
3. **fork 全量继承 exhausted goal**(本轮新发现,P1):
   `Continue in new session`(fork)把耗尽/病态 goal 原样带进子会话,
   子会话立刻恢复 verify 循环(真实烧 token)并顶着同样的
   `goal_budget_exhausted` 状态——作为"逃生口"的动作复制了死局。
4. **goal 类 one-shot 挂死**(L1-I5 + 本轮 n5 复现,P1 候选):
   对带病态 goal 的会话调 webui `POST /approve`(goal-verify)或
   `POST /goal action=cancel`,HTTP 响应可能永不返回——尽管 handler
   走 `runAR(ctx, 60s)`。两次独立复现均直接卡死无超时客户端
   (Playwright executor)。根因未定位(daemon 侧阻塞?handler 在
   runAR 之前阻塞?),需最先查清。

## Spec delta

- goal attach:新增 verifier 类型语义——`--verify <cmd>`(可执行,
  现状)与 `--verify-llm <rubric>`(已有)之外,**拒绝**明显不可执行
  的 `--verify` 判据(启发式:含 CJK/无可执行头/shell 语法预检失败
  即 400,提示改用 --verify-llm 或写成真命令)。
- goal pause/resume/cancel:回执必须反映真实结果(attached goal
  存在与否、pause 是否已落),并跨 daemon 重启持久。
- fork:携带 goal 的会话 fork 时,goal 以 **paused** 状态进入子会话
  (不自动恢复 verify 循环);exhausted 标记不随 fork 继承为激活态。
- webui goal/approve one-shot:必须在 oneShotTimeout 内返回(修根因,
  而非仅靠客户端超时)。

## Design delta

- DESIGN goal 章:verifier 判据类型与 attach 校验;GoalState
  (含 Paused/Exhausted)入持久化状态并在 replay/restart 后生效。
- fork 章:goal 继承规则(paused 进入子会话)。
- 不触 §15 既有不变量;若"fork 全量复制上下文"被视为不变量表述,
  则按不变量变更流程单独裁决 goal 例外。

## 验收

- attach 自然语言 verifier → 400 且文案指路;attach 可执行 verifier
  → L2b 场景(fire 一次、收敛)保持绿。
- attach → pause → daemon 重启 → 新 turn:无 goal-verify effect;
  resume 后恢复。
- 病态 goal 会话:approve goal-verify / goal cancel 的 HTTP 均
  ≤60s 返回(回归锚:v2sim L1-I5/n5 两次 stall 的复现步骤)。
- fork 带 goal 会话 → 子会话 goal 显示 paused、零 verify effect、
  list status 不再 `goal_budget_exhausted`。
- 已落的前端修复(0c359d9:goal_* 不产 terminal notice、pill 改标)
  的 vitest 断言保持绿。

## 实施步骤(待裁决后展开)

1. 定位并修 one-shot 挂死(条目 4)——它同时是 QA 通道杀手,优先。
2. GoalState 持久化 + pause 跨重启(条目 2)。
3. attach 校验(条目 1)。
4. fork 继承规则(条目 3)。
每步:代码+测试+check.sh 绿+独立 commit。

## review 裁决

里程碑级(动 goal 状态机与 fork 语义):落地后做三视角对抗 review
(正确性/并发、安全、契约)。提案期不做。
