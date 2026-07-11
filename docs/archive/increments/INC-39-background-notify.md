# INC-39 后台任务 notify 门（HANDA-PARITY #10，缩水版）

## 动机与 journey 锚

后台 bash 任务的终态**恒**以 user-role 消息回流唤醒会话——对
fire-and-forget 类任务（telemetry 上传、预热、长灌注）是噪音与
token 花费。PARITY #10 经 review 勘误缩水：唤醒机制已存在
（conversation.go bg.done + TestBackgroundTaskSettlesBeforeQuiescence），
真 delta 仅是**回流抑制门**。journey 锚：UJ-18（后台形态），不新增。

## Spec delta

- SPEC C 区 bash 行备注追加：`notify: always|on_fail|none`（仅
  background 任务有效；on_fail=仅 IsError 回流；none=终态只摘
  handle 不回流，结果仅 journal 可查；默认 always=现状）。

## Design delta

- additive：fold 的 background Completed/Failed 注入过 notify 门
  （从 journaled `ActivityStarted.Args` 读，crash-resume 天然重放同一
  裁决）；**Cancelled 不过门**（kill 是显式动作，partial 渲染是 kill
  流程一部分，QA-05 依赖）。decide() 对无新输入的唤醒本就回 idle，
  none 门零空转、静止照走——不触不变量。
- 裁决：非法 notify 值由 schema enum 挡 + fold 宽容回退 always
  （fold 不得报错）；不加 spawn 侧强校验。

## 验收

- 孪生：fold 矩阵（always/on_fail/none × completed-ok/completed-err/
  failed）+ 非法值回退 + Cancelled 恒渲染 + none 摘 handle。
- B 闸（真 Gemini）：notify:none 任务 settle 后无 [background work]
  消息、会话正常待命；on_fail 失败任务回流。归档 qa/runs/。

## 实施步骤

1. bash.json notify 参数 + state.go notifyOf/门 + 孪生 → check 绿 →
   真验 → 文档齐活 → commit。

## review 裁决

S 增量、fold 内两分支 + 宽容读，裁掉三视角 review。

---

## 执行记录（2026-07-10 收口）

一步完成。范围较工作纸再缩水：结构化载荷核查后已存在（bash result
恒带 exit_code/stdout tail），真 delta 仅门本身。B 闸真 Gemini 双
场景并行 PASS（none：零回流零多余 turn；on_fail+exit 3：回流+模型
复述 exit code），证据 `qa/runs/2026-07-10-INC39/`。方法记档：断言
用 `--state` conversation（回流是 fold 派生投影非事件）。SPEC C 区
加行；SPRINT #10 ✅。
