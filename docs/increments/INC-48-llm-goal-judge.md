# INC-48 in-session LLM goal judge（HANDA #8，触 DESIGN §13/决策 #21）

**状态：✅ 已实施并双闸门验收（2026-07-11）。设计定稿（同日独立契约
review「修订后放行」，rev1 吸收 MAJOR-1/MINOR-2——见 §review 记录）；
DESIGN 决策 #21/§13/glossary 修订与实现同 commit（PROCESS §四）。
A 闸=孪生 4 条（TestGoalLLMJudge{Pass,RejectThenPass,CrashReuse,
ClaimGatedNoCall}）；B 闸=真 Gemini QA-48（主场景+驳回续跑场景，见
docs/QA.md 与 qa/runs/2026-07-10-QA-48/）。**

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

**judge 证据输入契约（rev1，MAJOR-1）**：driver 的 `verifyLLMJudge`
喂 judge 的证据是 `childReport(childDir)`——**driver 耦合，in-session
无 childDir，不可直接复用**。in-session judge 的证据 = **本会话自
goal attach 以来的工作证据**（fold 出的 assistant 文本/工具结果摘要）
**+ 模型的 claim summary**（`g.ClaimSummary`），而非 childReport。
judge 只在"JSON verdict 解析"层复用 driver 逻辑，"证据装配"层是
in-session 新写。judge model = `Loop.Judge` 注入的 provider（默认可
取会话选定 model，独立 provider 字段留扩展）。

**crash-replay verdict 解析（rev1，MINOR-2）**：judge 的
ActivityCompleted.Result 是 `{score,pass,reason}` JSON，**无
exit_code 字段**——crash 复用**必须走独立 verdict 解析**（读回
score/pass），**禁止复用 `verifierExitCode`**（其 `ExitCode==nil &&
!IsError` 兜底会把任何 judge replay 误读成 exit 0 = pass）。

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

## review 记录（PROCESS §四，2026-07-11）

独立契约 review（子 agent，对照决策 #21/§13/glossary 原文 + goal.go/
driver.go/state.go 取证）裁决**修订后放行**：§四对旧不变量转述忠实、
决策 #21 佐证准确（§13/glossary/types.go:341 均已命名 llm_judge
deferred）、新表述只扩枚举不削弱 command 唯一裁决；claim-gated 门控
落位/成本下降/crash 同构/budget 兜底（max_checks）/blocked 推迟五项
契约成立，四性复核通过。**放行前须补两条（rev1 已吸收，见上）**：
- MAJOR-1：in-session judge 证据输入契约（childReport 不可复用）+
  judge model 来源。
- MINOR-2：crash-replay verdict 独立解析，禁用 verifierExitCode。

**实施轮清单（非契约，review 追加）**：MINOR-1 无 claim 目标达成却
以 budget 截断的锐边——continuation 文案强化"必须 goal_complete 才
被裁决"；MINOR-3 in-session pipeline 须对 `verifier:llm_judge` 的
`llm_call` effect 有 operator-set 放行路径（非模型可注入）。blocked
终态与 token/墙钟预算列余项。实施另起一轮。
