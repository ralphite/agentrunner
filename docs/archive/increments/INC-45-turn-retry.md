# INC-45 turn retry（HANDA #16，INC-44 §B 实施）

## 动机与 journey 锚

失败/中断/搁浅的轮只能手动重打消息。设计已在 INC-44 §B 定稿（契约
review M2 吸收：纯函数重组）。journey 锚 UJ-02/24。

## Spec delta

- SPEC A 区加行：`ar retry <sid>`——重发最后一条 user-class 输入为
  新 turn；载荷纯函数重组（文本 verbatim + CAS 附件读回 + 恒定
  provenance），command_id 派生 `retry:<原id>`（重复点击幂等，链式
  可再试）；忙/waiting 守卫；webui 失败态 Retry 按钮。

## Design delta

- 无（INC-44 §B 已裁：零协议变更；TurnID/ItemID 由 daemon 从
  command_id 确定性派生——纯函数约束天然满足）。

## 验收

- 孪生：planRetry 定位（跳过 program/agent 源）、派生 id、waiting/
  mid-turn 守卫、legacy 无 id 回退 seq；attachmentsFromCAS 字节一致。
- B 闸（真 Gemini）：interrupt 一轮后 `ar retry` 重发原文成功新轮；
  连点两次第二次幂等；webui Retry 按钮真验。归档 qa/runs/。

## 实施步骤

1. CLI retry + webui route/按钮 + 孪生 → check 绿 → 真验 → 文档 →
   commit。

## review 裁决

设计已过契约 review（INC-44 rev1），实施轮裁掉重复 review。

---

## 执行记录（2026-07-11 收口）

一步完成。真验抓出守卫 bug（待命形态 Waiting{input} 被误判为 ask
park）当场修——先判 Quiescence 再判等待；链式 retry 语义与 new 开场
无 CommandID 的 seq 回退常态化，均记 LOG。B 闸证据
`qa/runs/2026-07-11-INC45/`。SPEC A 区加行；SPRINT #16 ✅。
