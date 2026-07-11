# INC-47 结构化 ask_user（HANDA #7 = CLAUDECODE #10，INC-44 §C 实施）

## 动机与 journey 锚

`ask_user` 只有单问纯文本；对标 AskUserQuestion/handa
request_user_input：多问（≤4）+ 选项（2–4）+ multi_select + 自由
文本，应答走 typed 通道。UJ-06/24。设计已过契约 review（INC-44
rev1 §C：CommandAnswer 四触点、走 WaitInput 非 approval broker）。

## Spec delta

- SPEC C 区 ask_user 行升级：questions[] 结构化形态（park 载荷携带
  问题结构、AskResolved.Answers typed、`ar answer`/daemon `answer`
  应答通道、free-text send 兼容保留、resume replay 配对 answer）。
- webui 表单卡 + queued 撤回按钮（#29 余项）= 步 2。

## Design delta

- additive：AskResolved 载荷加 Answers（旧 Answer 保留）；
  CommandAnswer 命令种类（§2 家族新成员，park 应答类不进对话，
  语义同 approval 应答先例——不触铁律）；park detail 加 Questions
  （旧 detail 兼容）。

## 验收

- 孪生：park 校验（>4 问/选项数越界拒）、typed answer 配对（结构化
  tool result）、skip=cancelled、free-text send 兼容、resume replay
  配对 pending answer、fold 渲染。
- B 闸（真 Gemini）：模型 questions 提问→`ar answer` 选项应答→模型
  复述所选；--skip 路径。归档 qa/runs/。

## 实施步骤

1. （本轮）事件/协议/工具 def/park 构造校验/awaitAnswer typed 分支/
   replay/daemon answer/CLI ar answer + 孪生 + 真验 → commit。
2. （下轮）webui 分步表单卡 + send 返回 command_id + queued 撤回
   按钮（#29 余项）+ webui 真验 → commit。

## review 裁决

设计已过契约 review（INC-44 rev1），实施轮不重复。
