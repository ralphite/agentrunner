# INC-66 Runtime 状态正确性收口

## 动机与 journey 锚

2026-07-13 free-form 黑盒 QA 由两个独立 Agent 复核出 19 个正常使用路径下的
状态缺陷（排除未确认的 Ctrl-C #11）。它们影响 UJ-14 定时值守、UJ-15 Goal、
UJ-18 多 Agent 编排、UJ-22 in-session goal 与 UJ-24 Web UI 驾驶。

目标不是为旧错误状态加兼容分支，而是修正产生事实、fold 和公开 projection：

1. 一个 generation/effect 只执行一次，跳过的 generation 不得破坏 loop policy。
2. 同一 assistant batch 的多个 child 得到公平、守上限的 tree budget reservation。
3. Goal 只有 verifier pass 才能 achieved；预算耗尽是可恢复的 exhausted 状态。
4. Driver 的失败、retry attempt、overlap、next run 和 child 终态必须一致。
5. 空 journal 不可进入可发送 session 集；driver 不得走 conversation resume/retry。
6. 同 workspace 的 snapshot writer 跨 goroutine/进程串行，barrier 不因 index.lock 丢失。
7. denied tool call 计入失败统计；terminal report 不保留 running 进度。

## Spec delta

- B 树预算：同一 tool batch 的可解析 launch 按剩余 launch 数公平分配 parent
  remaining，再受 child cap 约束；公开 usage 同时展示 settled 与 reserved。
- E 持久化：shadow repo 并发 flock 从 backlog 改为完成；空/无 genesis journal
  不再作为 session 暴露或接收 command。
- F in-session goal：`GoalAchieved` 仅表示 satisfied；新增 `GoalExhausted{budget}`，
  goal 保留且可由 update 增加 `max_checks` 后继续。
- F loop：interval 是 fixed-rate cadence；长 iteration 跨 tick 时执行
  skip/coalesce，并 journal skipped/attempt facts。全失败系列以 child_failed 结束。
- I/观测：denied calls 进入 tool failure stats；terminal child 的未完成 progress
  投影为 failed；nextRunAt 必须严格晚于观测时刻。

## Design delta

### 不变量变更：Goal 预算耗尽

- 旧表述（DESIGN 决策 #21）：miss 且 `max_checks` 尽时写
  `GoalAchieved{budget}` 并摘除 goal。
- 冲突：事件名和 session projection 把 verifier 明确失败的目标表示成 achieved；
  goal 被摘除后 `goal update` 虽返回成功却无法产生 `goal_updated`，用户不能增加
  预算恢复。
- 新表述：只有 pass 写 `GoalAchieved{satisfied}` 并摘除 goal。预算耗尽写
  `GoalExhausted{reason:budget}`，保留 goal、停止自动 continuation；update 修改
  goal/budget 后清除 exhausted 并以 program input 重启，仍从已有 checks/context
  继续。cancel 仍摘除。
- 波及：event schema、goal fold/recovery/quiescence、CLI inspect、web timeline、
  tests 与 QA 文档。旧 QA 数据允许直接迁移/删除，不保留 `GoalAchieved{budget}`
  兼容写路径。

### 其余设计修订

- generation policy 以 journaled `LastAssistantGenStep` 判断当前 step 是否已有
  assistant message，不再用可缺号的 assistant message 数量。
- batch spawn reservation 使用动态 fair-share：`remaining / remaining_launches`，
  小 child cap 留下的额度自动给后续 launch；总和仍不超过 parent cap。
- interval/cron 都有 durable absolute tick；overlap 处理错过的 slot。
- retry 的每次 child attempt 有 parent-stream lifecycle facts；iteration completion
  仍只结算一次总 usage。
- shadow repo 的 init/Snapshot/ref mutation 受同一路径 advisory flock 保护；Diff
  继续使用 private index，可并发只读。

**契约 review 裁决**：通过。新 Goal 语义消除“失败即 achieved”的自相矛盾，
不放宽 verifier、budget、journal-first 或 effect pipeline 不变量；恢复能力通过
新 durable exhausted fact 实现，不引入第二套恢复机制。snapshot flock 只串行
writer，不改变 ref/credential 边界。interval fixed-rate 与 SPEC 已有“next run
锚在上次迭代开始”契约一致。

## 验收

### Scripted / unit

- generation：token truncation 后 settlement 开新 step，assistant tool call 不重复 LLM
  effect；每个 effect id 仅一对 requested/resolved。
- budget：3/7 sibling batch 均可获 fair share；peak settled+reserved 不超 cap；
  `LimitExceeded` 显示 settled/reserved。
- goal：fail→exhausted（无 achieved）；update 扩 budget→goal_updated+继续；
  step-limit 边界 pass→goal_satisfied receipt。
- driver：全 child error→child_failed+non-zero；retry attempts 全 journal；
  interval slow work 的 skip/coalesce 分叉；canceled child 不投影 running；
  nextRunAt 严格在未来；driver resume/retry 使用明确 domain contract。
- persistence：并发 Snapshot 无 index.lock、每次调用有 ref；empty session 不可 send/
  resume/list。
- observability：budget/permission denied tool 计 failure；terminal progress 无 running。

### 真实环境 B 闸

- 使用共享 store `~/.local/share/agentrunner/` 和真实 `http://127.0.0.1:8809/`。
- 复跑最小 Goal、3-agent batch、failure driver、slow interval、driver retry；保留
  session/journal/workspace，证据落 `qa/runs/2026-07-13-INC66/`。
- 重启真实 daemon/webui 后复查 session list/detail、deep link、API status、console。

## 实施步骤

1. generation + tree budget + usage/stats/progress projection。
2. Goal event/fold/control/recovery 语义迁移。
3. Driver cadence/retry/terminal/resume-retry contract。
4. snapshot flock + empty journal guard。
5. 文档收口、全量 check、共享环境 CLI/API/browser B 闸。

## review 裁决

本增量修改多个持久状态域，实施后做 correctness/concurrency/contract 三视角复核；
P0/P1 全部关闭后归档。#11 Ctrl-C 不在本增量范围。
