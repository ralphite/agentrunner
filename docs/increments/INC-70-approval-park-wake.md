# INC-70 WAITING_APPROVAL 挂起期间消息唤醒（G3 余项，audit-0717 D1）——待裁决草案

**状态:BLOCKED 等用户裁决**(2026-07-18)。本工作纸只定义选项与波及
面;INC-D2/INC-50 有在案定案"排队不解栈",推翻它必须人裁,不得由
autonomous loop 自行定夺。

## 动机与 journey 锚

UJ-07(中途纠偏):审批挂起可能数小时,期间用户发来的纠偏消息
("别装这个依赖,直接改用 X")现在只排队,模型看不见——用户以为
说了话,session 纹丝不动,直到有人应答审批。SPEC A 表该行 🟡,
DESIGN §17 #5 明记"唤醒语义待定"。

## 选项

**A. 维持定案(排队不解栈)**:消息在审批应答后的安全边界消费。
零改动;缺点即动机所述。若选 A,SPEC 行从 🟡 改 🧊(显式裁决记档),
G3 关闭。

**B. 消息=转向式拒批(推荐)**:park 中收到 user-class 文本消息 →
视为对挂起审批的**拒绝+转向**:journal ApprovalResponded{deny,
reason:"superseded by user steering",source:"user"} + WaitingResolved
{denied_by_steer} + 消息本体照常入列,同一安全边界被消费——模型在
同一 turn 看到"审批被拒(用户转向)"的 tool result 与新指令。
- 不变量核对:审批应答只认 user 命令通道(G16 条款)——消息正是
  user 通道,principal 相同,合规;新增 WaitingResolved resolution
  枚举值 `denied_by_steer` 落 WaitRules(INC-69 后注册表是唯一源,
  加一行即全局生效);crash 重放路径与 INC-46 revoke 交互需覆盖
  (park 中 revoke 该消息 → 不触发 deny)。
- machine/hook 消息(untrusted)**不**触发——只排队,维持 G16
  "不可信来源不能驱动审批"红线。
- steer/queue 投递模式:仅 steer 语义消息触发?建议**不分**——
  park 中任何 user 文本都触发(park 中"下个安全边界"与"下个 turn"
  重合,两种投递模式此刻无差)。此点亦可裁。

**C. 消息唤醒但不动审批(并行转向)**:审批保持 pending,消息作为
新输入唤醒模型继续本 turn……与"审批挂起=效应未决不得继续执行"
冲突,模型在效应未决时行动会破坏 in-doubt 教义。**不推荐,列出仅为
完备**。

## Spec delta(若取 B)

SPEC A 表"WAITING_APPROVAL 挂起期间消息唤醒"🟡→✅,锚:新孪生
TestApprovalParkSteerDeniesPending / TestApprovalParkMachineInputOnlyQueues
/ TestApprovalParkSteerSurvivesRestart + QA 新场景(真 Gemini:park 中
发纠偏消息,审批拒、模型转向)。

## Design delta(若取 B)

§2(inbox 消费)+ §5 审批条款 + 2.14 WaitRules 增行;DESIGN §17 #5
删除。触及"审批应答通道"语义边缘——按 PROCESS §四单独成文(本纸
即是),需契约视角 review。

## 实施步骤(若取 B)

1. WaitRules 增 denied_by_steer + fold/枚举 + 孪生;
2. 消费点:approval park 的 select 增 user-input 分支(cmdAppend
   幂等、revoke 先查)+ resolveEffectAfterApproval(false) 复用;
3. QA 场景 + SPEC/GAPS/LOG 收口。

## review 裁决

选项 B 触审批语义,须契约视角 review(用户裁决即兼此审)。
