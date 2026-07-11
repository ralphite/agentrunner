# INC-52 LLM 自动会话标题（HANDA-PARITY #14，缩水版 B3）

## 动机与 journey 锚

**UJ-24（Web UI 产品面）**：侧栏任务列表要能一眼区分会话。现状标题 =
首条消息首行（`webui/meta.go`、`ar sessions list --json`），7 条同名 prompt
无法辨别（INC-23 W9 记账的老问题，移交至此）。目标：会话开局后异步用
一次 LLM 调用把首条用户消息精简成短标题，**走 journal 事件**落地，供 webui
显示；LLM 不可用/失败则回退首行。

承接 INC-23 W9「生成短标题」移交；与 CLAUDECODE SPRINT #17（webui
服务端 rename）避让——本增量**只碰 auto-title 路径**。

## Spec delta

- SPEC 新增一行「LLM 自动会话标题」：`SessionTitled{source:auto}` journal
  事件 + fold `RawTitle` 投影 + 异步生成路径（不覆盖 manual）+ `ar sessions
  list --json` 与 webui 显示接线。锚见「验收」。
- SPEC 既有行 172「journal-backed session metadata」补 INC-52：`sessions list
  --json` 的 title 现由 `RawTitle` 投影优先，回退首行。

## Design delta

- **DESIGN §12「Session 管理」加一条非不变量描述**：auto session title 是
  从 `SessionTitled` fold 出的 title 投影，与既有「journal-backed metadata」
  教义一致（title 是投影，不是可变字段）。source 分立 auto/manual/fork，
  **auto 绝不覆盖 manual/fork**（不变量编码在 fold）。
- **不触及不变量，不走 §四**：
  - 新增 journal 事件 = additive（旧 journal 无此事件，fold 容忍缺失并回退
    首行）。sub-state `session` 版本**不 bump**（RawTitle/TitleSource 是
    additive-optional 字段，从零值 fold，同 MalformedRetries/Progress 等
    先例，决策 #18）。
  - §12:1204 粗体条款「rename 等 localStorage key 原样保留」：**未动**
    manual rename（仍 localStorage）。服务端 manual rename 若要做单独立项。
  - §12:1160「metadata 不得覆盖 journal 状态」：本增量**修正** webui
    `handleSessions` 让 journal-backed CLI title 优先于 meta cache（原码
    meta.Title 覆盖 CLI title，会遮蔽 auto-title），是**执行**该既有不变量，
    非改动它。
  - in-session LLM 维护调用 = 沿用 compaction summarizer / goal judge
    (INC-48) 的既有族：非 permission-gated 的 harness 维护调用、记为
    `llm_call` Activity、usage 结算进 budget、崩溃后复用已记录结果。**未
    新增预算/边界规则**。

## 验收

**A 闸（scripted 孪生，全绿）**：

| 交付物 | 锚 |
|---|---|
| SessionTitled round-trip / fold 覆盖 | `TestRoundTripAllTypes`、`TestApplyCoversRegistry`（event/state 自动覆盖新类型） |
| fold 投影 + source 优先级（auto 不覆盖 manual/fork；manual 覆盖 auto；旧 journal 无事件回退空） | `TestSessionTitledFoldProjection`、`TestSessionTitledAbsentFoldsEmpty` |
| 生成一次并 fold 出投影；二次调用幂等 | `TestAutoTitleGeneratesOnceAndFoldsProjection` |
| 不阻塞开局 turn（assistant 消息未落前不触发） | `TestAutoTitleWaitsForOpeningReply` |
| auto 不覆盖 manual（provider 不被调） | `TestAutoTitleDoesNotOverrideManual` |
| 短单行任务跳过（省调用） | `TestAutoTitleSkipsShortTask` |
| LLM 失败吞掉、不 abort、回退首行 | `TestAutoTitleSwallowsLLMFailure` |
| **崩溃重放不重复生成**（activity 已记录→复用，provider 不再调） | `TestAutoTitleReusesRecordedResultOnReplay` |
| CLI `sessions list --json` 用 RawTitle 优先，回退首行 | `TestCLISessionsJSONSurfacesAutoTitle` |
| webui 显示读 rawTitle，manual rename 仍胜 | `viewModels.test.ts`「shows the journal-backed auto title and still lets a manual rename win」 |

裁掉项显式声明（G29 教训）：
- **驱动/子 session/headless 一次性 run 不生成 title**——`AutoTitle` 仅由
  daemon 在**顶层托管交互 session**上置位（`NewChild/NewChildAt` 与
  scripted 测试均 false），避免为不出现在会话列表的 run 付一次 LLM 调用，
  也不扰动既有 scripted fixture。
- **服务端 manual rename 不做**（§12:1092 禁止迁移；单独立项走 §四）。
- source=fork 分立但本增量**不生产** fork title（fork 路径仍走 webui meta
  的 `title (fork @...)`）；fold 优先级已为 fork 预留（auto 不覆盖 fork）。

**B 闸（真实 API，QA-51，留给 reviewer 集中验）**：
1. 共享 daemon + 真 webui + 真 Gemini，New task 发一条**长**多行 prompt
   （>48 字符，触发生成而非短路）。
2. 开局 turn 的首条回复**不被延迟**（title 调用在 assistant 消息落后的安全
   边界，异步于开局回复）。
3. `ar events <sid>` 里出现一条 `session_titled{source:auto}` + 一条
   `llm_call` activity（name=autotitle，usage 计入 budget）。
4. webui 侧栏该会话显示精简短标题（非首行截断）。
5. 在 webui 手动 rename 后，标题变为手动值且**不再被 auto 改回**（auto 不
   覆盖 manual：manual 走 localStorage，displayTitle 层胜出）。
6. LLM 不可用（断网/坏 key）时会话仍正常，标题回退首行——不 abort。
证据归档 `qa/runs/<日期>-INC52/`。

## 实施步骤（均已落地，单 worktree 分支）

1. `internal/event/types.go`：`TypeSessionTitled` + `SessionTitled` 结构 +
   `TitleSource{Auto,Manual,Fork}` 常量 + Registry；`event_test.go` 补样本。
2. `internal/state/state.go`：`Session.RawTitle/TitleSource` + Apply case
   （auto 不覆盖 manual/fork）；`state_test.go` 补 fold 孪生。
3. `internal/agent/autotitle.go`（新）：`maybeAutoTitle` + cleanTitle 等；
   `loop.go` 加 `AutoTitle` 字段并在 drive 安全边界（goalResumeCheck 后）
   调用；`autotitle_test.go`（新）6 条生成孪生。
4. `internal/cli/resume.go`：`sessions list --json` 用 RawTitle 优先。
5. `internal/cli/daemon.go`：两处顶层托管 loop（新建 + resume）置
   `AutoTitle=true`。
6. `webui/api.go`：journal title 优先于 meta cache（执行 §12 不变量）。
7. `webui/frontend/src/viewModels.test.ts`：显示接线 vitest（前端源无需
   改动——displayTitle 早已读 rawTitle，接线自 INC-23 起就位）。
8. 文档 delta：本纸 + SPEC/DESIGN §12/LOG/HANDA-PARITY/SPRINT。

## review 裁决

小增量，**裁掉三视角对抗 review**。理由：additive 事件 + 既有维护调用族
（compaction/judge 先例）+ 不触不变量；安全面为零（无新 permission/网络/
凭据面，harness 内部调用不过管线，与 compaction 同）；并发面单一（loop 单
写者，title 调用同步于安全边界，不建后台句柄、不碰静止形状）。契约面
（fold 投影 + source 优先级 + 崩溃复用）由 A 闸孪生锚定。B 闸真机由
reviewer 集中验。
