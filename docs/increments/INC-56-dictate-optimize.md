# INC-56 · ar dictate（服务端语音听写）+ ar optimize（prompt 优化）

HANDA-PARITY §2 #18（M）+ #19（S）合并一轮（天然一对：都是「webui 薄壳把
composer 便利动作经 `ar` 命令落到 provider，绝不直调」）。worktree 子 agent F。

## 动机与 journey 锚

- **#18**：现状 webui `useVoice` = 浏览器 SpeechRecognition——无上下文、
  兼容受限、中英混合与专有名词转写差。目标：录音走 `ar dictate` →
  Gemini 转写，session/project 上下文消歧。锚 UJ-01/04（会话输入）。
- **#19**：把用户的草稿 prompt 改写得更清晰、解析模糊指代。目标
  `ar optimize` + composer Sparkles + `/optimize` slash + 单步 undo。
  锚 UJ-02/24（输入编辑）。
- 共同不变量：**webui 薄壳教义**（DESIGN §12:1075「webui 只经 `ar`
  CLI/daemon contract」+ 决策 #15c 凭据面）。两项都实现为**新 `ar`
  子命令**，webui 只上传/转发并显示结果，浏览器绝不接触 provider 或
  凭据。

## 定性：文本便利，非新模态（守 DESIGN 非目标 line 36「语音输入」）

`ar dictate` 是一次性 audio→text 转写 helper：音频转成文字后，作为
**普通文本 prompt** 进 composer。**agent 对话循环从不组装 audio part**、
journal 会话不含 audio、multimodal user input（image/file part，M4.1）不变。
因此这**不**把「语音输入」变成产品模态——它就是它替换的浏览器
SpeechRecognition 的同类（转写成文本的便利），只是更准、带上下文。

落地时刻意的三条边界，坐实此定性：

1. **provider 层只加一个 part kind**（`PartAudio`）+ Gemini adapter 映射到
   inline_data。这是 additive：与 image/file part 走同一条 `toPart` 的
   inline_data 分支。
2. **不动 `Capabilities`、不动 `Envelope` 的 `InputModalities`**。
   `InputModalities` 描述的是**对话循环接受**的 modality（text/image/file）；
   audio 只被 loop 外的 dictate helper 用，故不进 envelope、不进
   `SessionStarted.provider_capabilities`、inspect 里普通 session 不会多出
   一个 audio modality。`TestCapabilitiesMatrix`（断言 `len(InputModalities)==3`）
   因此原样通过——这不是巧合，是「audio 非对话模态」的机械证明。
3. **dictate/optimize 都不碰 daemon/journal/loop**：是独立一次性 provider
   调用（照 `internal/driver` verifyLLMJudge 范式：build `CompleteRequest`
   → `CollectTurnStreaming` → 取 assistant 文本）。

## Spec delta

`docs/SPEC.md` §A 新增两行（ ✅ + 真 twin 锚；真验 pending 见下 QA 段）：

- 服务端语音听写（`ar dictate <audio>` → provider audio part → Gemini 转写；
  `--context` 消歧；音频大小上限；webui 录音上传经 ar，SpeechRecognition
  fallback）。
- Prompt 优化（`ar optimize "draft"` → LLM 改写；`--context` 解析模糊指代；
  composer Sparkles + `/optimize` slash + 单步 undo，原稿留前端态）。

## Design delta

**Additive，不触不变量**（预期即如此，实证如上「三条边界」）：

- `provider.PartAudio` 新 part kind（provider.go）——记档为 audio 输入 part
  的 additive 扩展，明确「仅 dictate helper 用、非对话模态」。
- 其余无 DESIGN 语义变更。§12:1075 薄壳条款、决策 #15c 凭据面、line 36
  非目标**均不动**——本增量恰是它们的兑现（webui 经 ar、凭据落 ar 进程、
  audio 不成模态）。故 additive 记档，**不走 §四**。

## 验收（枚举型交付物逐项对锚）

### A 闸 · scripted 孪生（进 check.sh，确定性离线）

provider 层：
- `TestToPartAudio`（gemini）——PartAudio → inline_data，MIME+bytes 逐字；
  byte-less audio part 硬报错（不发空 blob）。

`ar dictate`（`internal/cli/dictate_test.go`，内联 fake provider 捕获请求）：
- `TestDictateEncodesAudioPartAndContext`——磁盘录音→PartAudio（bytes+MIME
  完整）+ context 进 system prompt + 只有 transcript 出 stdout + tool-less。
- `TestDictateRejectsOversizeAudio`——超限在 provider 调用**前**拒（零调用）。
- `TestDictateUnknownMIMENeedsFlag`——未知扩展名报 `--mime`；显式 `--mime` 生效。
- `TestDictateMissingAndEmptyFile`——缺文件/空文件 ExitUsage。
- `TestDictateCmdUsage`、`TestAudioMIMEInference`（扩展名映射表）。

`ar optimize`（`internal/cli/optimize_test.go`）：
- `TestOptimizeRewritesDraft`——draft 原样进 user turn + context 进 system +
  只有改写出 stdout + 原稿不被 mutate（undo 前端态） + tool-less。
- `TestOptimizeNoContextStillWorks`、`TestOptimizeEmptyDraft`（零调用）、
  `TestOptimizeSurfacesProviderError`（失败 ExitRun、stdout 空）、
  `TestOptimizeCmdUsage`。

webui 薄壳（`webui/composer_helpers_test.go`，fake `ar` stub）：
- `TestHandleDictateRejectsNonUploadPath`——**安全**：音频路径必须在 uploads
  目录内，`../` 逃逸/绝对外部路径/空 全 400，spawn ar 前即拒。
- `TestHandleDictateForwardsToAR`、`TestHandleOptimizeForwardsAndGuardsDraft`
  （`--context`/`--` 分隔转发正确；空 draft 400 零 spawn）、`TestUnderDir`。

前端 vitest（`webui/frontend/src/components/*.test.ts`，mock ar 调用）：
- `slash.test.ts`——`/optimize` 注册在 home+session、needsArgs、解析
  `/optimize <draft>`、裸 `/optimize` 不跑。
- `composerOptimize.test.ts`——Sparkles/undo 逻辑 7 例：改写换入+存 undo
  快照、draft+context 透传、空 draft no-op、空返回不覆盖、失败不 mutate、
  undo 还原、helperContext 拼接。

**裁掉的项显式声明**：MediaRecorder 真录音编解码 ↔ Gemini 格式接受性属
runtime/浏览器行为，vitest 不覆盖（jsdom 无 MediaRecorder）——归 B 闸。

### B 闸 · 真实 API（用户集中验，见下「QA 说明」）

## 实施步骤（已落地，单 commit：INC-56）

1. provider：`PartAudio` + gemini `toPart` inline_data 映射 + `TestToPartAudio`。
2. CLI：`dictate.go`/`optimize.go`（一次性 provider 调用 + 共享 helper）+
   cli.go 注册 + help 文本 + 两个 `*_test.go`。
3. webui 后端：`handleDictate`/`handleOptimize`（薄壳 shell 到 ar，dictate
   路径限 uploads 目录、optimize `--` 保护）+ 路由 + `composer_helpers_test.go`。
4. webui 前端：`api.ts`（AR.dictate/optimize）；抽 `slash.ts`（+`/optimize`）；
   `composerOptimize.ts`（纯 optimize/undo 控制器）；`useDictation.ts`
   （MediaRecorder→upload→ar dictate，fallback 标志）；Composer 接线
   （Sparkles 按钮 + Undo affordance + `/optimize` case + mic 优先服务端）+
   CSS + 两个 vitest。

## QA 说明（B 闸 · 交由用户）

真实 Gemini，共享 daemon/store（不隔离，QA 规则）。归档
`qa/runs/2026-07-11-INC56/`。

**#18 dictate**——两条路：
- **CLI 直验（最稳，绕开浏览器录音格式问题）**：准备一段真实中英混合
  音频，含专有名词。构造测试音频最简法：
  - macOS `say`：`say -o /tmp/note.aiff "deploy the kubelet on cluster A, 记得 rebase"`
    （`.aiff` 已在 MIME 映射内）；或 `say -o /tmp/note.wav --data-format=LEF32@22050 "..."`。
  - 或任意手机录音存 `.m4a`/`.wav`。
  跑 `ar dictate --context "Kubernetes, kubelet, cluster-A, rebase" /tmp/note.aiff`，
  断言：exit 0、stdout 是转写文本、专有名词拼写命中、中英各自保留语言。
  超限：`ar dictate --max-bytes 10 /tmp/note.aiff` → 非零 + 提示 limit、零 provider 调用。
- **webui 全链**：composer mic → 录音 → 松手转写落 textarea（真机 Chrome，
  MediaRecorder 大概率 `audio/webm`）。**待验风险**：Gemini 对
  `audio/webm`(opus) 的接受性未在 CLI 侧确认——若 webui 转写报错而 CLI
  `.wav/.aiff` 成功，即锁定为录音容器格式问题（fallback：前端录 wav 或
  后端转码；已在 useDictation 优先探测 `audio/ogg`）。SpeechRecognition
  fallback：无 MediaRecorder 的环境仍走浏览器听写。

**#19 optimize**——
- CLI：`ar optimize --context "editing internal/auth" "fix the thing that broke"`
  → 断言 exit 0、改写更具体、解析了「the thing」。
  stdin：`echo "make it faster" | ar optimize -`。
- webui：composer 输入草稿 → Sparkles 按钮 → textarea 换成改写 + 出现 Undo →
  点 Undo 还原原稿（单步）；`/optimize <draft>` slash 同链路。console 0 错误。

## review 裁决

小增量（S+M，全 additive，零不变量）。按 PROCESS §二裁掉三视角对抗
review，理由：(1) 无并发/状态机改动（一次性无状态 provider 调用）；
(2) 契约面只加一个 additive part kind + 两个新 ar 命令 + 两个 webui
端点，不改既有契约；(3) 安全面已在 A 闸覆盖关键红线——dictate 路径限
uploads 目录（防任意文件读）、optimize `--` 防 flag 注入、音频大小上限
（防滥用）、readBody 的 application/json 前置（既有 CSRF 兜底）。落地后
delta 并回 SPEC/HANDA-PARITY/LOG，本纸归档。
