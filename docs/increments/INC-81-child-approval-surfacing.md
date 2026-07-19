# INC-81 child 审批浮出（G39 不可见审批死锁）

## 动机与 journey 锚

GAPS G39（QA-0718 七轮真机实证，证据链闭环）：spawn 的 child 卡在
approval 时，其 `approval_requested`+`waiting_entered{approval}` 只存在
于 child 自己的 sub-store journal；webui 只订阅父流、SSE ask 只活一瞬、
`ar inspect` 树只递归**已完成**（有 `SubagentCompleted`）的 child——
审批在一切 surface 上不可见，父面呈现"Background work still running…
keeps spending tokens"永久悬挂。锚 UJ-18/23/24（多 agent 监督）。

调查结论（2026-07-19 代码对读）：**approve 路由今天已通**——child 审批
以 child sid 为 key 注册进 root broker（spawn.go `Approvals: l.Approvals`
共享 resolver），`ar approve <child-sid> <apr-id>` 经 `-sub-` 寻址 +
`splitAddress` 精确送达（daemon_test/approval_test 已锚）。缺口纯属
**可发现性**，修复是只读投影，不触不变量、不新增事件类型。

## Spec delta

- B 表"子 agent 实时进度镜像 / 子审批根宿主路由"行：补锚
  `TestBuildInspectTreeSurfacesInFlightChildApproval`（in-flight child
  入树 + waiting 浮出 + AnswerWith）。
- GAPS G39：修复落地（闸门 A），闸门 B 真机复验后关闭。

## Design delta

无不变量变更。投影层增强：inspect 树的 child 递归从"仅
SubagentCompleted"扩展为"spawn 事实 ∪ 完成事实"（in-flight child 以
其自身 fold 呈现真相；revive 后的 settled child 维持 G26
latest-settlement 契约，已在工作纸记为已知边界）。

## 验收

- 闸门 A（本增量落地）：
  - Go：`TestBuildInspectTreeSurfacesInFlightChildApproval`——无
    settlement 的 child 进树、携带 waiting:approval、AnswerWith 给出
    精确 `agentrunner approve <child-sid> <apr-id>`、文本渲染含 apr id。
  - 前端 vitest：SupervisionPanel "G39" describe——child waiting
    approval 在 Attention 具名浮出（member + tool）；settled child 不加行。
- 闸门 B（待 QA 轮）：真机重现 QA-0718 场景（spawn worker 走
  ask-mode bash），断言 webui Attention 出现成员审批行、Approve 后
  child 继续、刷新页面审批卡仍在（inspect 持久源）。

## 实施步骤

1. INC-81.1（本 commit）：`internal/cli/inspect.go` buildInspectTree
   递归 in-flight child（spawn_requested 遍历 + childDirForSession +
   settled 去重）+ annotateChildAnswerWith（approval 与结构化 ask 两形态）；
   webui InspectNode.waiting 类型、SessionView openApprovals 从 inspect
   children 递归提升（SSE-only → 持久）、SupervisionPanel attentionRows
   具名成员行；孪生测试两侧。
2. INC-81.2（后续 QA 轮）：闸门 B 真机复验，绿后 GAPS G39 关闭。

## review 裁决

小增量，裁掉三视角 review：改动为只读投影 + 前端展示，无并发/安全面
（approve 路由未动，仅复用既有测试锚）；契约面由闸门 A 两侧孪生钉住。
