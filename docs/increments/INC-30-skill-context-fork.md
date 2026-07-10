# INC-30 skill context:fork（一次性子 agent 执行，#45/§3.5 余项）

## 动机与 journey 锚

CLAUDECODE-PARITY §2.05 #45 / §3.5 余项，INC-20 拆出（SPRINT #7b）。对标
Claude Code skill `context: fork`——skill 不在父上下文里内联执行，而是在
**一次性子 agent** 里跑，父只拿结果。用途：重上下文/长指令 skill 不污染父
上下文；skill 作者可为 skill 限定更窄工具面。

## 设计（核心裁决：ingest 展开,复用动态角色全链）

**fork = ingest 时把 `skill` 调用展开为 `spawn_agent{role}`**——与
「命令=用户宏 ingest 展开」同一先例。生成收集后、journal
`assistant_message` **之前**（loop.go 单点），对每个 `skill` 工具调用：

1. 读 `<ws>/.claude/skills/<name>/SKILL.md`（同 executor 的防遍历校验）,
   解析 frontmatter（yaml）。
2. `context: fork` 且门控通过 → 改写该 tool_call part（message 与
   ToolCalls 同步）：`spawn_agent{role:{name:<skill名>, description:<
   frontmatter description 或 "skill <name>">, instructions:<正文>,
   tools:<frontmatter allowed-tools,可省>}, task:<模型给的 task 或默认>}`。
3. 其余情况**不改写**（skill 缺失/无 frontmatter/非 fork/正文空/名字不合
   roleNameRe/门控关）→ 走现有内联路径（返回正文）,行为与今天完全一致。

改写发生在 journal 之前 ⇒ fold/pipeline/crash 恢复看到的就是
spawn_agent：**树预算预留、深度/扇出上限、RoleSpec 冻结事件、审批路由、
receipts 全部免费复用,零 spawn 机制改动**。crash-safe：重放的是已改写的
journal,transform 不再跑(无 workspace 重读不确定性)。

**门控（多 agent 面永不静默变宽）**：改写仅在 `agents_dynamic: true` 时
发生。skill 文件是 workspace 内容（不可信,agent 自己可写）,无门控则
workspace 文件能替 spec 作者放开 spawn 面。门控关时 fork skill 内联执行
（安全降级,文档言明）。备选「`agents:` 白名单列 skill 名」弃：新命名
约定成本 > 收益,agents_dynamic 正是"运行期起草子 agent"的既有门。

**frontmatter 面**：`context`（fork 触发）、`description`（角色描述）、
`allowed-tools`（子工具面,dynamicRoleSpec 校验 ⊆ 父）。model/hooks/预算
不从 frontmatter 来（InlineRole 的 harness-control 裁决不动）。递归
fork 由既有深度/扇出上限兜底（决策 #20）。

## Spec delta

- SPEC H skills 行：+ context:fork（ingest 展开为 role spawn,agents_dynamic
  门控）。skill.json 加可选 `task` 参数。
- CLAUDECODE-PARITY #45：fork 余项 → ✅。

## Design delta（不触不变量）

DESIGN §10 skills「模型侧 invoke」段补 fork 一句：ingest 展开、动态角色
全链复用、agents_dynamic 门控。不动 journal/fold/pipeline/spawn 不变量。

## 验收

- 孪生：`TestForkSkillExpansion` 单元（fork 改写成形/非 fork 不动/门控关
  不动/坏名不动/allowed-tools 映射）+ `TestForkSkillSpawnsChild` 全链
  （镜像 TestSpawnDynamicRole：模型调 skill(name,task)，SpawnRequested
  Agent=skill 名、frozen RoleSpec.SystemPrompt=skill 正文、子会话跑完、
  receipts 回父）。
- 真实 API QA-37（daemon-path → 私有新二进制 daemon）：workspace 放
  context:fork skill,spec agents_dynamic;模型 invoke → journal 见
  spawn_agent{role=skill 名}、子会话存在且 system prompt 含 skill 正文
  标记、receipt 回父、父作答。（QA-36 已被 INC-29 webui 预订,让号。）
- 绿门（排除已知环境测试）。

## 实施步骤

1. internal/agent/skillfork.go：frontmatter 解析 + expandForkSkills。
2. loop.go journal assistant_message 前单点调用。
3. skill.json 加 task 参数 + fork 说明。
4. 孪生 + QA-37。
5. 文档行齐活（SPEC/CLAUDECODE-PARITY/DESIGN §10/LOG/SPRINT）。

## review 裁决

做。M 收敛为 S+（一个 transform 文件 + loop 单点 + def 参数）,零 spawn
机制改动。inline 自审：correctness（改写前 journal、message/ToolCalls 同步、
非 fork 路径零变化）、security（agents_dynamic 门控、防遍历复用、roleNameRe
复用、allowed-tools ⊆ 父由 dynamicRoleSpec 强制）、contract（内联 skill
行为不变、命令=用户宏裁决不动、InlineRole harness-control 不动）。
