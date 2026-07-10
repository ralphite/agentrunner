# INC-25 内置 agent 库（explore/plan 只读，#78）

## 动机与 journey 锚

CLAUDECODE-PARITY §2.09 #78 + UJ-18。对标 Claude Code 内置 Explore/Plan
（只读子 agent，开箱即用、不必写 spec）。现状：spawn 子 agent 必须由
workspace 自带 `<name>.yaml`（siblingSpecResolver）。本增量发行内置**只读**
agent（explore/plan），spec 的 `agents:` 白名单列内置名即可 spawn，无需
自带 spec 文件。

## 范围（干净形态：resolver 单点改）

- embed `internal/agent/builtin/{explore,plan}.yaml`——**只读工具面**
  （read_file/grep/glob/semantic_search，无 edit/write/bash-execute），
  安全无副作用。
- `agent.BuiltinSpec(name) (*AgentSpec, bool)`——从 embed 加载。
- `siblingSpecResolver` 加一层（**只改这一处**，9 个调用点不动）：name
  命中内置 → 返回内置 spec，且**继承父 model**（resolver 内 LoadSpec 父
  spec 取 model 覆盖内置默认，拿不到则用内置默认 gemini）；否则回落到
  现有 sibling `<name>.yaml`。
- spec 白名单语义不变（模型只能 spawn `agents:` 列出的）——内置名是
  白名单的一个新来源，不是"默认全自动可用"（后者涉白名单封闭性讨论，
  拆余项 #16b）。

## Spec delta

- SPEC B/H 记内置 agent 库（explore/plan 只读，spec 列名即用，model 继承
  父）；锚 `TestBuiltinSpec*` + QA-32。
- CLAUDECODE-PARITY §2 #78 状态更新（explore/plan opt-in ✅，默认可用余项）。

## Design delta（不触不变量）

DESIGN §3/子 agent 加一句：内置只读 agent（explore/plan）随发行 embed，
resolver 优先内置来源、model 继承父；白名单封闭性不变（spec 显式列名）。
只读工具面使其自动可用无副作用风险，但默认可用留待 #16b。

## 验收

- 孪生：`TestBuiltinSpecLoads`（explore/plan 加载、只读工具、含
  description）/`TestResolverPrefersBuiltinAndInheritsModel`（resolver
  name=explore → 内置 spec，model = 父 model）/`TestResolverFallsBackToSibling`
  （未知名 → sibling yaml）。
- 真实 API QA-32：spec `agents: [explore]`，让模型 spawn explore 子 agent
  探索代码库，验证子会话用只读工具面完成、model 继承父；`ar events`
  归档 qa/runs/。
- 绿门（排除已知环境测试）。

## 实施步骤

1. embed builtin/explore.yaml + plan.yaml（只读）+ agent.BuiltinSpec。
2. siblingSpecResolver 加内置优先 + model 继承（单点改）。
3. 孪生 + QA-32。
4. 文档行齐活。

## review 裁决

做。干净 S（resolver 单点改 + embed 2 spec + BuiltinSpec 函数）。inline
自审：correctness（model 继承、未知回落）、security（内置只读工具面无
副作用；白名单封闭性不变）、contract（9 调用点不动、白名单语义不变）。
默认全自动可用拆余项 #16b。
