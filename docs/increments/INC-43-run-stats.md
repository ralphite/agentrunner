# INC-43 运行统计 stats（HANDA-PARITY #31）

## 动机与 journey 锚

会话只有 token 审计，没有"干了什么"的量化面：工具成败率/耗时、
文件行增删、活跃时长——QA 脚本与自动化消费（对齐 handa cli stats /
gemini-cli stats）。journey 锚：UJ-17（用量审计扩展），不新增。
PARITY §2 #31（review M4 已吸收：不在 fold 里 diff 已 redact 的
args；duration/active 用 envelope TS 做**报表投影**，非核心 fold）。

## Spec delta

- SPEC I 区（events/inspect 行）备注追加或新行：inspect stats——
  tools{calls,success,fail,duration_ms}（按工具名分组+总计）、
  files{lines_added,removed}、active_seconds（Σ turn span 扣
  waiting 配对区间）；文本+--json 两面。
- SPEC C 区 write/edit 行备注：result 带 lines_added/removed
  （模型可见 diff 统计）。

## Design delta

- 无新事件。行增删在 **executor 计算入 result payload**（比工作纸
  前身"写 ActivityCompleted 载荷"更轻：零事件 schema 变更，且模型
  可见——handa 对照的另一半价值顺手取得）；报表层从 journaled
  result 解析数字（redact 不动数字字段）。duration/active_seconds
  以 envelope TS 计，居 inspect 报表层，明示非核心 state fold。

## 验收

- 孪生：executor 行增删（write 新建/覆盖、edit 替换、多行中文）；
  inspect 聚合（成败计数/duration 非负/lines 求和/active 扣
  waiting）。
- B 闸（真 Gemini）：会话写+编辑文件后 `ar inspect` 文本与 --json
  显示 stats；数字与实际操作吻合。归档 qa/runs/。

## 实施步骤

1. executor 行统计 + inspect 聚合/渲染 + 孪生 → check 绿 → 真验 →
   文档齐活 → commit。

## review 裁决

M 增量、报表层为主，裁掉三视角 review。`ar run --json` 的 stats
出口留余项（主消费面是 inspect）。
