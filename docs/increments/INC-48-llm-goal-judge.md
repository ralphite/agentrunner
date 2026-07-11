# INC-48 in-session LLM goal judge（HANDA #8，触 DESIGN §13/决策 #21）

**状态：📐 设计稿（awaiting contract review）——按 PROCESS §四先裁后码。**

## 动机与 journey 锚

多数真实长程目标写不成 shell 命令（"重构完所有 handler 并保持行为
不变"）。当前 in-session goal 只有两态：command verifier（唯一裁决）
与 self-cert（模型 goal_complete 声明边界接受）。缺第三态 **llm_judge**
——独立 LLM 对完成声明做裁决，比"无条件接受声明"强、比"必须写成
命令"宽。对标 handa goal_judge / CODEX-PARITY §6.2。UJ-22。

**关键事实**：DESIGN §13/决策 #21 **已命名** llm_judge 为 verifier
kind（deferred），driver 侧 `verifyLLMJudge`（driver.go:1228）已是
可复用范例；本增量把它接进 in-session goal 的 `goalVerify`。

## Spec delta

- SPEC F 区 in-session goal 行：verifier 增 `llm_judge` kind
  （`ar goal attach --verify-llm "<rubric>"`）；**claim-gated**（仅在
  goal_complete 声明待决时调用 judge，杜绝每边界一次 LLM 的无界花费）；
  judge = budget-gated `llm_call` 管线 effect（Activity-bracketed、
  journaled、crash 复用 journaled verdict）；三态判别器
  command / llm_judge / self-cert。

## Design delta（触不变量，走 §四）

见下"§四变更单"。

## §四变更单

**旧不变量**（DESIGN §15 决策 #21 粗体，§13，glossary "verifier"）：
> "有 command verifier 时 verifier 是唯一裁决者；无 verifier 时由
> 模型 goal_complete 声明"

——隐含"in-session 完成裁决只有 command 与 self-cert 两条路"。

**为什么必须动**：llm_judge 是决策 #21 自己命名的第三种 verifier
kind（§13 "command / llm_judge / human 三态"），但 in-session 形态
从未落地它；写不成命令的长程目标只能落到 self-cert（无条件接受
声明），失去独立裁决。补齐是决策 #21 的**兑现**而非违反，但"唯一
裁决者"的枚举需从 command 扩为 command｜llm_judge。

**新表述**（决策 #21 修订，§13 补 in-session llm_judge 段）：
> "完成裁决在 exchange 边界：**有 command verifier 时 command 是
> 唯一裁决者**（每边界跑，claim 仅注记）；**否则有 llm_judge
> verifier 时，judge 是唯一裁决者，但仅在 goal_complete 声明待决时
> 调用**（claim-gated：无声明=miss 续跑，不调 judge、零 LLM 花费）；
> **都没有时由模型 goal_complete 声明边界接受**（self-cert）。
> llm_judge 是 budget-gated 的 journaled `llm_call` 管线 effect
> （Activity-bracketed；crash 后复用 journaled verdict 不重判——同
> command verifier 的 completedVerifierResult 幂等窗）。judge 二态
> pass/fail（blocked 终态列余项，避免 judge 获得单方终结权这一更强
> 授权）。"

**四性/边界复核**：
- **边界纪律不变**：judge 仍只在 quiescent 边界跑，绝不 mid-turn。
- **budget**：judge 调用计入 goal max_checks 与（未来）token 预算；
  claim-gated 使调用次数 ≤ 声明次数 ≪ 边界次数。
- **crash 安全**：judge 是 Activity，其 ActivityCompleted 落盘后
  GoalCheckpoint 若未落，resume 复用 verdict（completedVerifierResult
  按 activity id，已对 command 成立，llm 同构）。
- **claim 不越权**：judge 裁决声明；模型的 claim 本身不完成 goal
  （与决策 #21 "声明不越权"一致）。

**波及面**：
- event `GoalVerifier.Kind` 增 "llm_judge" + `Rubric` 字段
  （additive，旧 journal 无该字段）；
- `goalVerify` 增 llm_judge 分支（复用 driver verifyLLMJudge 逻辑，
  provider 注入 Loop.Judge）；
- `verifiersHaveCommand` → 保留（command 专用），新增
  `verifiersHaveLLMJudge`；`goalCheckpoint` 三态 switch；
- `Loop.Judge provider.Provider` 字段 + daemon/cli 注入；
- `ar goal attach --verify-llm`（conversation.go）；
- goalContinuation/applyGoalControl 文案三态；
- DESIGN §13 + 决策 #21 + glossary；SPEC F 区；
- 孪生（judge pass/fail/claim-gated 不调/crash 复用）+ 真 Gemini QA。

## 验收（实施轮细化）

- 孪生：scripted judge provider——claim+pass→achieved；claim+fail→
  reject 续跑；无 claim→miss 不调 judge（provider 零调用断言）；
  crash 在 ActivityCompleted 后→复用 verdict 不重判；预算尽→截断。
- B 闸（真 Gemini judge）：无命令目标挂 --verify-llm，模型工作→
  goal_complete→judge 裁 pass→achieved；一次 judge 驳回续跑。

## 实施步骤（契约 review 通过后，单独轮）

1. event kind+rubric / Loop.Judge / goalVerify llm 分支 / 三态 switch /
   CLI / 文案 / DESIGN+SPEC + 孪生 → check → 真验 → commit。

## review 裁决

**本轮=设计+§四变更单+独立契约 review**（触决策 #21 粗体）；实施
另起一轮。blocked 终态与 token/墙钟预算列余项。
