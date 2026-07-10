# INC-29 Web UI UX Round 3（结构化详情/标题/状态语义）

## 动机与 journey 锚

UJ-24 的 Codex 式主路径已由 INC-23/QA-35 跑通，但 INC-23 执行记录明确
移交三个仍低于产品级的弱点：W21 Supervision 详情仍是整块 raw JSON、W9
同类指令的共同前缀挤掉真正差异、W33 running/ready/approval 等状态色在
sidebar/pill/Subagents 之间不一致。本增量把这三个用户可见弱点收口，不新增
journey。

## Spec delta

- SPEC I「Web UI Supervision」：补结构化 Run details（Overview/Usage/
  Provider/Waiting），raw JSON 仅作为折叠 advanced 证据。
- SPEC I「Web UI 产品面」：自动标题去常见命令/回复模板前缀，保留完整标题
  tooltip 与用户手动 rename 优先级。
- SPEC I「Web UI 交互语义」：统一状态色语义：running=绿脉冲、ready=蓝环、
  approval/recovery=琥珀、failed=红、terminal=灰。

## Design delta

无。不改 journal/API/daemon/状态机；只把既有 `inspect`、session title 与
status projection 做前端视图模型和 progressive disclosure。DESIGN 不变量
不变。

## 验收

- scripted/frontend：`summarizeInspect` 覆盖 waiting/usage/provider/未知字段；
  `conciseTitle` 覆盖 bash/reply 模板、普通中英文、manual rename 优先。
- 真实浏览器 QA-36：共享 store 的 approval/team/recovery 三种 session；
  Run details 不先展示 JSON/command answer；sidebar 同前缀任务可辨；light/
  dark 下五类状态语义一致；console error=0。
- `./scripts/check.sh` 全绿。

## 实施步骤

1. 结构化 inspect view model + Run details modal + tests。
2. concise title + 统一状态 token/CSS + tests。
3. QA-36 黑盒复验；三层/QA/GAPS/LOG 收口并归档工作纸。

## review 裁决

小型纯前端投影增量，不触不变量，裁掉三视角对抗 review；以真实
approval/team/recovery 状态的黑盒走查和 Codex 同图对照代替。任何 P0/P1/P2
仍按 QA-fix 当轮修复。
