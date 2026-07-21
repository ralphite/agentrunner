# INC-85 子 agent 编排契约进模型可见面(消除 fire-and-yield 缺口)

## 动机与 journey 锚

**锚:UJ-18 多 agent 编排 step 2/3。** journey 本意是 fire-and-yield——
「并行启动 3 个后台子 agent,**主 agent 自己继续读源码**」「outcome 消息
**回灌**主 agent」。runtime 侧机制早已齐全并关闭(GAPS G2;DESIGN 静止子
唤醒 §、line 136「background outcome 作为 user-role 消息进对话、下个
generation 看到」;`awaitInput` 的 `case <-l.bg.done` 驻留点)。

**缺口:契约从没进模型可见面。** 现场取证(session
`20260721-070616-session-cff02416618dc178`,spec=dev,model=gemini-flash-latest):
主 agent 派完 3 个 worker 后,连续 4 分钟用 `output(handle)` 轮询 + `bash
"sleep N"`(9 次共 140s、~25 轮空转 generation)自旋等待,期间从未 idle,
把本可自动唤醒的路径彻底架空。根因——`spawn_agent`/`output` 的 description
只承诺结果「arrives as a message」,**从没告诉模型「派完可直接结束 turn、
完成会作为消息自动唤醒你、无需轮询/sleep」**。强模型能自己推出这个推论,
弱模型(默认 Gemini Flash)推不出,退回人类直觉式忙等。

**先例:`lead` persona 早有明证。** specs.ts 注释:「models otherwise
front-load everything at spawn time and never message mid-flight」——团队
已知弱模型不会自己编排、需要显式协议,并为 lead 写了协议;但**默认 dev
persona 缺同类「派完就停」指引**。本增量补齐。

新登记 GAPS **G40**(缺口发现 + 本增量关闭)。

## Spec delta

`SPEC.md` §B「子 agent 与编排」新增一条功能点(挂 UJ-18、锚 QA-0721):

> 子 agent 编排契约进模型可见面(fire-and-yield 不 busy-wait):
> `spawn_agent`/`output` 的 description 显式声明「派发非阻塞,派完可结束
> turn,子 agent 完成作为消息自动唤醒,无需轮询/sleep」;默认 dev prompt
> 同义补一句。INC-85 · QA-0721(真 Gemini Flash:派 3 worker 无 sleep 自旋)

## Design delta

**不触任何不变量**(唤醒机制、投递模式、静止子唤醒全部原样)。仅在
`DESIGN.md` line ~130「编排的智能在模型,runtime 只提供原语」一句后补一句
**非不变量注记**:

> 模型要正确行使这份编排智能,`spawn_agent`/`output` 的工具描述必须把
> fire-and-yield 契约讲明(派完可结束 turn、完成自动唤醒、无需轮询/
> sleep)——否则弱模型会 `output`+`sleep` 自旋空转(INC-85 现场)。

## 验收

**枚举型交付物(4 处文本改动,逐项对锚):**

| # | 文件 | 改动 |
|---|---|---|
| 1 | `internal/tool/defs/spawn_agent.json` desc | 补:派完无需轮询/sleep,结束 turn 即可,结果作为消息自动唤醒你 |
| 2 | `internal/tool/defs/output.json` desc | 补:仅偶尔一瞥,勿循环调用、勿 sleep 等完成;结束 turn 由消息唤醒 |
| 3 | `internal/agent/handle.go:368` note | 与 #2 对齐:无需再轮询/sleep,结束 turn,完成消息会唤醒你 |
| 4 | `webui/frontend/src/specs.ts` dev prompt | 补一句:派完别轮询/sleep,结束 turn,报告作为消息唤醒你综合 |

**闸门 A(scripted 孪生 / 单测):** 目标是**模型行为**(是否 busy-wait),
scripted provider 无法复现「模型选择 sleep」,故 A 闸只做**描述存在性**回归
断言——单测断言 spawn_agent/output 描述含反忙等关键词(防措辞回归静默丢
失)。真正证明力在 B 闸。**此为小增量 A 闸的合法收窄,理由在此声明。**

**闸门 B(真实 API,核心锚):** QA-0721。真 Gemini Flash,dev-like spec,
派 3 个 worker 做代码审计。断言(只钉 runtime 红线,不钉模型措辞):
- 主 agent 派完后**不再出现 `bash "sleep …"` 自旋**、不再对 `output` 循环轮询;
- 3 个 worker 报告仍作为消息正常回灌、主 agent 正常综合;
- 会话保留(共享 store),`ar events` 导出 + 证据归 `qa/runs/2026-07-21-QA-0721/`。

对照基线 = 本 session `20260721-070616`(修前 9 次 sleep + ~15 次 output 轮询)。

## 实施步骤

1. **INC-85.1**:改 4 处文本(defs×2 + handle.go note + specs.ts dev)+ 补
   描述存在性单测;`./scripts/check.sh` 全绿;重建 `ar`/`arwebui`(含
   frontend build);commit+push。完成标志:check 绿、二进制含新描述。
2. **INC-85.2**:B 闸真 Flash 复跑(共享 daemon,新二进制),取证归档;
   delta 并回 SPEC/DESIGN/GAPS/LOG,工作纸归 archive;commit+push。
   完成标志:QA-0721 PASS(无 sleep 自旋 + 报告正常综合)、文档行齐活。

## review 裁决

**裁掉三视角对抗 review。** 理由:改动面 = 4 处模型可见文本字符串,零
控制流、零状态机、零不变量、零并发路径改动;无 golden 测试锚死这些描述
(已核实),回归面仅「措辞是否含反忙等指引」由 A 闸存在性断言兜底;真实
行为改善由 B 闸真 Flash 直证。契约视角的唯一关注点(是否与既有唤醒不变量
矛盾)已确认一致(注记不改机制)。
