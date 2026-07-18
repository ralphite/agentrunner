# INC-73 并发 send 的每命令输出定界（concurrent-send output scoping）

**状态:实施中**(2026-07-18)。修订 `ar send`/`ar new` 的**实时输出渲染**
契约,不触任何 DESIGN 不变量(journal 归属一直正确——见下"证据")。

## 动机与 journey 锚

UJ-05/UJ-06(会话对话):两个客户端(两个终端、脚本并发、CI)同时
`ar send` 到**同一会话**时,每个 `send` 会 latch 到会话的**共享 live
事件流**,把在它 attach 期间完成的**任意** turn 都打印出来——而不是它
自己那条消息的回复。症状(QA 反复复现):

- `dave-06`(P1):两个 send 打印相同输出;快 sender 看不到自己的回复。
- `heidi-02`(P1):A 打印自己的空 turn **和** B 的回复。
- `cli-life-01`(P1):两个 client 都 exit 0 但没打印任何回复。
- `nate-03`(P2):两个阻塞 send 各自渲染**两条** turn 的回复。

**证据:journal 归属始终正确**——每条 input 恰好消费一次、gen-step 与
回复的对应关系正确(4 位 tester 一致确认)。缺陷**只**在 live 输出定界:
`followTurn` 渲染流上的每个事件直到会话 idle,不区分是哪条命令触发的
turn。因此这是**渲染契约修订**,非数据完整性问题、非不变量。

## Spec delta

- `SPEC.md` 会话对话域:`ar send`/`ar new` 的"跟随并渲染回复(INC-2
  BB-me-4)"一行补注:**跟随只渲染并定界到调用方自己 input 触发的
  turn**;并发同会话 send 各自只看到自己的回复。验收锚 = 下方新增
  scripted 孪生 + QA 场景。

## Design delta

不动不变量。修订/新增的语义契约:

1. **每命令定界(新契约)**:一个跟随型 `send`/`new` 的 live 渲染,定界到
   **消费了它 input 的那条 user turn**(含该 turn 的多个 tool-loop
   gen-step),在该 turn 到达 idle 时脱离。合并(type-ahead 把多条 input
   并入一个 generation)时,被并入的每条 input 的 follower 都渲染该
   共享 turn——归属是**集合**。
2. **归属载体**:`protocol.Event`(live wire,非 journal)新增
   - `Seq int64`:daemon 在 "delivered" ack 上回传本 follower 自己的
     `DeliverySeq`,让 CLI 知道"我是哪条 input"。
   - `InputSeqs []int64`:`KindGenerationStart` 上携带**本 generation
     刚消费的新 input 的 DeliverySeq 集合**;tool-loop 续跑的 gen-step
     为空(归属沿用上一条)。仅 live 事件带,**journal 的
     `GenerationStarted` 不变**(replay 是单读者,无需定界)。
3. **CLI 定界状态机**(`followTurn`):
   - `mySeq` 取自 delivered ack 的 `Seq`。
   - `mine`:当前 turn 是否归属 mySeq;`seen`:我的 turn 是否已出现。
   - 收到 `KindGenerationStart`:若 `InputSeqs` 非空(新 user turn 起)→
     `mine = mySeq ∈ InputSeqs`,`mine` 时置 `seen`;`InputSeqs` 空
     (续跑)→ `mine` 沿用。
   - 只在 `mine` 时渲染 turn 内容事件(message/tool_call/tool_result/
     text_delta/generation_start/discard/error)。
   - `KindIdle`:仅当 `seen` 才脱离(我的 turn 已跑过);未 `seen` 忽略
     idle(别人的 turn 先 idle 不能让我提前脱离——正是 cli-life-01
     "没打印回复"的根因)。
   - `KindApprovalRequest`/`KindRunEnd`/`KindError`:approval/error 若属
     我的 turn 按旧逻辑脱离;runEnd 是会话级(会话结束)——无论如何脱离。
4. **向后兼容(硬要求,防挂起)**:`mySeq == 0`(旧 daemon 不回传 Seq,
   或 `new` 的 ack 无 seq)→ **回退到旧行为**(渲染全部、首个 idle
   脱离)。这样任何版本错配都不会把"显示串扰"退化成"永久挂起"。
5. **归属集合来源**:loop 侧 `driveState` 加内存字段 `pendingInputSeqs`;
   `journalInput` 消费一条 input 时 append 其 `DeliverySeq`;
   `KindGenerationStart` emit 时取出并清空。续跑 gen-step 之间无
   `journalInput`,故为空——正确沿用。

**波及面核对**:
- tree member 事件(INC-12.6,`e.Session != sid`)渲染路径不变(它先于
  定界分支返回)。
- `--detach`(webui 全部走此路)不跟随,无影响。
- `new` 首 turn 无并发;`mySeq==0` 回退保其行为不变。
- crash/replay:attach `--replay-only` 是单读者,不加定界,行为不变。

## 验收

新增/修订:
- **scripted 孪生**(单测):`followTurn` 定界状态机——分离 turn(A 只见
  自己回复)、合并 turn(两 follower 同见共享回复)、我的 turn 是第二条
  (不被别人的 idle 提前脱离)、`mySeq==0` 回退(渲染全部)。
- **loop 单测**:`KindGenerationStart` 带正确 `InputSeqs`(首 gen-step
  非空、tool-loop 续跑为空、合并两 input 带两 seq)。
- **QA 场景**(`docs/QA.md` + 真实/ mock 并发脚本):两终端并发 send 到
  同一会话,各自 stdout 只含自己的回复;对锚 dave-06/heidi-02/
  cli-life-01/nate-03 四条 finding。
- **枚举对锚**:protocol.Event 新增两字段(Seq、InputSeqs);followTurn
  分支(mine 渲染 / seen 脱离 / mySeq==0 回退)逐项覆盖。

## 实施步骤(一步一提交)

1. protocol.Event 加 `Seq`、`InputSeqs`;daemon delivered ack 回传
   `Seq: in.DeliverySeq`。(完成标志:字段就位、build 绿。)
2. loop:`driveState.pendingInputSeqs`;`journalInput` append;
   `KindGenerationStart` emit 带 `InputSeqs` 并清空 + 单测。
3. followTurn 定界状态机 + `mySeq==0` 回退 + 单测。
4. 并发 send 集成测试(mock)对锚四条 finding。
5. 收口:并回 SPEC/DESIGN,QA/GAPS 同步,LOG 记一条,工作纸归档。

**裁掉的 review**:小-中增量,单人原型;正确性/并发靠上述单测 + 并发
集成测试覆盖;无安全面(纯本地 socket 渲染)、契约面即本工作纸对锚。
