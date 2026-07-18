# INC-74 非-generation 重开也清 close 标记（compact/clear 复活后状态不撒谎）

**状态:实施中**(2026-07-18)。修 QA quinn-02(状态撒谎);quinn-01
按 DESIGN 现有不变量裁为**by-design**。不改不变量,只补齐状态派生。

## 动机与 journey 锚

UJ-01/03(会话):关闭的 session 被 `compact`/`clear` 显式复活后
(journal:`session_closed → context_compacted → waiting_entered{input}`,
`ar send` 确实能继续),`ar inspect`/`ar sessions` 仍报 "marked
(closed)"/"closed"——**状态撒谎**(quinn-02)。`send` 复活则正确显示
`waiting:input`。

**不变量核对(关键)**:DESIGN §恢复:"`send` 是用户的**显式重开手势,
对任何 session 成立**(含带 close 标记的——**标记只约束自动路径**);
自动路径(timer/boot sweep)绝不越标记。" 据此:

- **quinn-01(compact/remember 复活关闭会话)= by-design**:它们是**显式
  用户命令**,不是自动路径,标记本就不约束它们——与 `send` 同类,允许
  越标记复活。不裁为 bug。
- **quinn-02(复活后状态仍 closed)= bug**:`send` 复活经
  `GenerationStarted` 清 close 标记(决策 #30,state.go:652);compact
  复活无 generation,靠 `WaitingEntered{input}` 重新待命,但 fold 的
  WaitingEntered 处理不清标记 → 状态停留 closed。**缺的是对称清除**。

## Spec delta

- `SPEC.md` 会话对话/生命周期:补注"**任何显式重开(send 或 compact/
  clear 复活)后,close/stop 标记被清除,状态回到 waiting:input**;标记
  只约束自动路径(timer/boot sweep)"。验收锚 = 下方单测。

## Design delta

不动不变量(§恢复"标记只约束自动路径 / send 显式重开"原样成立)。补一
条实现注:**合法重开的信号有两个,对称**——`GenerationStarted`(send:
起新 turn)与 `WaitingEntered`(compact/clear/revive:无 turn 直接重新
待命)都清 close 标记(session 正在待命 = 活的,标记被超越)。仅清
`Closed`;`Failure`/`Truncated*` 是另一类标记,由各自的重开信号
(AssistantMessage/GenerationStarted)清除,WaitingEntered 不碰——
避免误清一个"失败后 idle 等输入"会话的失败态。

正常关闭序列 `waiting_entered → waiting_resolved → session_closed`:
WaitingEntered 在 close 之前,Closed 尚 nil,清除是 no-op;唯有
close **之后**的 WaitingEntered(=复活)才实际清标记。

## 验收

- **state 单测**:序列 `[..., SessionClosed, ContextCompacted,
  WaitingEntered{input}]` → fold 后 `Closed==nil`、Quiescence "completed"、
  `Waiting.Kind==input`(即 waiting:input)。
- **回归**:`TestSessionClosedOverridesWaitingAndInFlightState`
  (WaitingEntered 在 close 前 → 仍 closed/canceled)不变。
- **实测**:close → compact → `ar inspect`/`ar sessions` 同显 waiting:input;
  `ar send` 继续正常。

## 实施步骤(一步一提交)

1. state fold:`WaitingEntered` 清 `s.Session.Closed` + 单测;并回
   SPEC/DESIGN 注、LOG、归档工作纸。
2. quinn ledger:quinn-01→wontfix(by-design 记理由)、quinn-02→fixed。

**裁掉 review**:小增量,纯 fold 状态派生补齐,单测覆盖正/反序列;无
安全面、契约面即本纸对锚。
