# INC-37 progress_update 进度清单工具（HANDA-PARITY #9）

## 动机与 journey 锚

长任务运行中用户只能从工具流反推 agent 在哪一步；模型侧没有维护
checklist 的通道。对标 handa progress_update：模型整表维护会话级
进度清单，人侧在 inspect 与 webui Supervision 直读。journey 锚：
UJ-18/22/24（监督面），不新增 journey。HANDA-PARITY §2 #9（review
MINOR 已吸收：不造 "state-class" 术语，按 goal_status/goal_complete
先例做 loop 内处理的内部工具，不过 effect 管线）。

## Spec delta

- SPEC C 区加行：progress_update（模型整表替换会话级清单
  items[{id,title,status}]；status 归一 pending/running/done/failed；
  条目/字段 clamp + redact；result 只回 ok+计数不回显全表；事件
  ProgressUpdated 纯 fold 出 state.Session.Progress）。
- SPEC I 区 Supervision 行备注追加 Progress 区（收口时并入）。

## Design delta

- additive：新事件类型 `progress_updated`（journal schema additive，
  旧 snapshot 缺该投影自动全量 fold——既有 schema guard 语义）；
  state.Session 加 Progress 投影。不触任何不变量；工具与 goal 面同
  seam（drive-goroutine 闭包 + serialAppend journal），不过管线
  （read-class、无副作用出 workspace）。

## 验收

- 孪生：scripted provider 调 progress_update → 事件入 journal →
  fold 出 Progress → inspect --json 携带；参数校验（空表=清空、
  未知 status 报错、超限 clamp）、归一别名、resume 后 Progress 保持。
- B 闸（真 Gemini + 共享 daemon）：模型按指示维护 3 项清单并逐步
  推进；`ar inspect` 可见状态推进；webui Supervision Progress 区
  渲染（真浏览器 DOM 断言）。归档 qa/runs/。

## 实施步骤

1. event.ProgressUpdated + fold + def JSON + loop seam（注入与
   dispatch）+ 归一/校验 + 孪生 → check 绿 → commit。
2. inspect（文本+JSON）+ webui SupervisionPanel Progress 区 + 真验
   → 文档齐活 → commit。

## review 裁决

S/M 增量、additive 事件、复用既有 seam，裁掉三视角 review；孪生 +
真浏览器双闸覆盖。
