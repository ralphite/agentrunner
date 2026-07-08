# INC-1 子会话寻址(观察面树形完备)

## 动机与 journey 锚

UJ-17(远程驾驶舱)承诺 attach 直播"工具调用与判定实时可见";UJ-5/
QA-04/05 的多子编排把大量真实工作放进了子 run——但子会话 journal
(`<parent>/sub/<call>-a<n>/`,spawn.go 落盘)在观察面上**不可寻址**:
`resolveSessionDir` 只扫 `sessions/` 顶层,`ar events / inspect /
attach(replay)/ ps` 对子会话全部报 "no session matches";`inspect`
的树只收录已 settle 的子(走 SubagentCompleted)。结果:在飞子 agent
的内部过程对用户完全不可见(2026-07-07 驾驶舱用户需求直接命中此缺口:
"i need to see link to open subagent session")。

本增量 = 观察面第一步:**让子会话 id 可寻址**。落地后 events/--state/
inspect/ps/attach-replay 对子会话全部自动生效(它们共用
resolveSessionDir),驾驶舱即可渲染在飞子的实时时间线。
(第二步"子事件进 attach 流(Out sink tee)"、第三步"父→子二次
消息"另立增量,见 web/PROGRESS.md 提案 P1②/P2。)

## Spec delta

SPEC.md §I(观察与远程面)`events / inspect` 行:能力注记增加
"子会话寻址(child_session 全 id,`-sub-` 分段映射到 `sub/` 目录,
任意深度)"。

## Design delta

DESIGN.md 观察章(“event log 就是 trace”段)加一句寻址语义:
`<parent>-sub-<call>-a<n>` 按 `-sub-` 分段映射
`sessions/<parent>/sub/<call>-a<n>[/sub/...]`;分段安全性由 CallID
铸造格式(`call_%d_%d`,provider.CallID)保证。**不触任何不变量**
(纯只读寻址;子 journal 的存在与形状是 S5.3 既有事实)。
sessions list 仍只列顶层(树入口),不变。

## 验收

- scripted:`TestResolveChildSessionDir`(全 id → sub/ 目录;孙级
  嵌套;不存在的子报错)、`TestEventsChildSession`(events 命令对子
  会话输出其 journal)。
- 真实:驾驶舱点击 spawn 卡的"打开子会话"链接,**子在飞时**看到其
  实时时间线(task 输入 → 轮次 → bash 运行中),返回父会话;CLI
  `ar events <child全id>` 同样可读。证据记 web/PROGRESS.md。
- QA.md 不新增场景:只读观察面,无运行时行为变化;QA-04/05 已覆盖
  子生命周期本体。

## 实施步骤

1. `internal/cli/events.go` resolveSessionDir 子会话分支 + 两个单测
   —— 完成标志:`./scripts/check.sh` 全绿。
2. web/:spawn/settle 卡 child_session 链接化;子会话页只读模式
   (无 composer/interrupt/close/SSE,ps 只读)+ "← 父会话"导航
   —— 完成标志:web gates 全绿。
3. 真验(上述"真实"栏)→ 三层收口(SPEC/DESIGN/LOG)→ 本纸归档
   `archive/increments/`。

## review 裁决

裁三视角 review。理由:纯只读寻址,无并发/安全/预算面变化;契约为
向后兼容的新增寻址形式;单测 + 真验覆盖。
