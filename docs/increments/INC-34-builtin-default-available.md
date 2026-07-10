# INC-34 内置 agent 默认全自动可用（变更单，#78 余项，SPRINT #16b）

**状态：📐 awaiting-review——触"多 agent 面永不静默变宽"治理边界，
本稿是变更单，未裁决不实现。**

## 问题

INC-25 发行了内置只读 agent（explore/plan），但要 spawn 必须在 spec
`agents:` 白名单列名。Claude Code 的内置 subagent（Explore/Plan/
general-purpose）是**默认可用**的（除非显式禁用）——#78 的完整对标
要求"不列名也可 spawn"。这与我们的白名单不变量正面相撞。

## 相撞的不变量

- SPEC B / `AgentSpec.Agents` 注释：**"The model only sees — and can
  only spawn — what is listed here."** `agents:` 白名单是 spec 作者对
  子 agent 面的**显式 opt-in**。
- DESIGN §3 / §10：**多 agent 面永不静默变宽**——spawn 是需 spec 作者
  声明的能力（涉树预算/深度/扇出/审批路由），workspace 内容或发行默认
  都不得替 spec 作者放开。

默认可用直接违反："`agents: []`（或省略）的 spec 本无子 agent 面，
默认可用会让模型凭空能 spawn explore/plan"——即便只读，也是**未经 spec
作者声明的 spawn 面拓宽**。

## 三个选项

**A. 保持现状（白名单列名才可用）** ——不变量零妥协。代价：与对方
"开箱即用"差一步；但 `ar init` 样例 spec 已含 `agents: [explore, plan]`
可把摩擦降到"一次列名"。

**B. 默认可用，但仅当 spec 已有 spawn 面**（推荐）——内置只读 agent
在 `agents:` 非空 **或** `agents_dynamic: true` 时自动进目录（无需逐个
列名）;若 spec **完全无 spawn 面**（空 agents + 非 dynamic）则内置**不**
可用。理由：
- **不变量的意图是"能力面不被静默拓宽"**。一个已经声明了 spawn 面的
  spec（列了别的 agent 或开了 dynamic）已经 opt-in 了"我要用子 agent"；
  在此基础上让**只读**内置 agent 免列名可用，不拓宽*能力类别*（spawn
  已在），只免去逐名登记的摩擦。
- 完全没 spawn 面的 spec（`agents` 空 + 非 dynamic）严格保持"零子
  agent"——**opt-out 者绝不被拓宽**，不变量的硬边界守住。
- 只读工具面（read_file/grep/glob/semantic_search）使其无副作用；树
  预算/深度/扇出上限照常兜底（那是独立关卡，不受此影响）。

**C. 全无条件默认可用**（对齐对方）——最激进，任何 spec 模型都能 spawn
内置。否决：违反 opt-out 硬边界，且我们的哲学是显式 opt-in 优先。

## 推荐

**取 B。** 它在"不静默拓宽 opt-out 者"（守住不变量硬边界）与"已 opt-in
spawn 者免逐名登记"（拿到对方开箱即用的大部分收益）之间取最优。实现
面极小：`renderAgentsDirectory` 与 `resolveSpawnTargetFull` 的白名单判定
在"spec 有 spawn 面"时把内置名视为**隐式在册**（`IsBuiltinAgent(name)
&& (len(Spec.Agents)>0 || Spec.AgentsDynamic)`）。可加 spec 级
`builtin_agents: off` 显式关（默认 on），给要极致封闭的 spec 逃生口。

## 若裁 B，实施（S）

1. spawn.go：`resolveSpawnTargetFull` 白名单判定加内置隐式在册分支
   （门控 = 有 spawn 面）;`renderAgentsDirectory` 把内置名并入目录展示
   （已有 resolver 取 description 的路径,INC-25 已铺）。
2. spec 加可选 `builtin_agents`（默认 on/"available"，"off" 关）。
3. 孪生：`TestBuiltinDefaultAvailableWhenSpawnFace`（agents 非空/dynamic
   → 免列名可 spawn explore）/`TestBuiltinUnavailableWhenNoSpawnFace`
   （空 agents+非 dynamic → 拒，不变量硬边界）/`TestBuiltinAgentsOff`
   （显式关 → 拒）。QA 复用 QA-32 形态（spec 不列 explore 仍能 spawn）。
4. 文档：SPEC B 行、CLAUDECODE-PARITY #78、DESIGN §3 白名单语义补
   "内置只读 agent 在已声明 spawn 面时隐式在册"。

## 交用户的裁决点

① A/B/C 三选（推荐 B）。② 若 B：`builtin_agents` 逃生口默认 on 是否
合适。③ 是否接受"已 opt-in spawn 面的 spec 隐式获得只读内置 agent"这
一对不变量的**收窄式**解读（不拓宽能力类别，只免登记摩擦）。
