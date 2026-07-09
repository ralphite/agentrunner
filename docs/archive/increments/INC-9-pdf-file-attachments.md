> **归档注记（2026-07-09）**：INC-9 已落地并收口——delta 并回
> SPEC/DESIGN(§9.1)/GAPS(G1 余项)/QA-15/LOG；闸门 A（check.sh 全绿 +
> 三个新孪生/单测）+ 闸门 B（QA-15 真实 Gemini 读 PDF）双绿。本工作纸
> 只读封存，与活文档冲突时以活文档为准。

# INC-9 PDF / 任意文件附件（多模态泛化）

## 动机与 journey 锚

- **来源**：GAPS **G1 余项「PDF/附件泛化」**（`docs/GAPS.md`，G1 主体
  已于 v2 M4 关闭；本增量收该余项）。CODEX-PARITY §2「任意附件/PDF ❌
  仅图片」同指此缺口。
- **journey**：多模态用户输入（QA-07 vision 三要素同源）——用户把一份
  PDF/文本/任意文件连同消息发给 agent，模型据文件内容作答。今天只有
  图片能发（`ar send --image` 且 `loadImageAttachments` 硬拒非 image
  媒体，`conversation.go:201`）。
- **为什么现在**：Codex composer 对标要求"paste image、PDF 等"；驾驶舱
  已具 `+` → File 入口，产品侧补齐后即全链路可用。

## Spec delta（SPEC.md）

- 功能域「会话交互 / 多模态输入」新增功能点：
  **`ar send --file <path>`（可重复）——附带任意类型文件**，与
  `--image` 并列。文件字节走 CAS、journal 只存 ref（沿用 file part 既有
  语义）。验收锚：`TestConversationalFileInputEndToEnd`（孪生）+
  QA-15（真实 API，Gemini 读 PDF 作答）。
- 既有「`ar send --image`」条目状态不变；新增条目标注"泛化 --image 的
  非图片路径，opt-in，不改既有形态"。

## Design delta（DESIGN.md）

- **不触不变量**（调查确认）：`file` part 已是 §18「消息 parts」枚举的
  不变量部件类型（`text/tool_call/tool_result/image/file`）；part 模型、
  CAS、event（`InputReceived.Files`）、fold（`state.go` Files→PartFile）、
  inflate、**Gemini** provider（`inline_data` 按 MIME 泛型）**均已泛化**，
  本增量不新增部件类型、不改任何不变量。→ **无需走 PROCESS §4 不变量
  变更流程。**
- §17 实现状态注记：把 §9.1 记录的"`--image` 只在 send 路径"补一句
  "send 路径已泛化为任意文件（INC-9）；`ar new` 开场消息仍不带附件
  （非对称保留，见 §9.1）"。
- provider 适配薄层修订（§4「provider 适配层」范畴内）：**Anthropic**
  provider 今天把非 text 的 file part 一律发成 image block
  （`anthropic.go:274`），PDF 会以 image mime 误投——本增量加一个
  `application/pdf` → document block 分支（SDK 已支持
  `DocumentBlockParam`/`Base64PDFSourceParam`）。Gemini 零改动。

## 验收

- **闸门 A（scripted 孪生，进 check.sh）**：
  - `internal/agent/multimodal_test.go`：`TestConversationalFileInputEndToEnd`
    ——发 `UserInput{Files: [{application/pdf, bytes}]}`，断言 journal 的
    `InputReceived.Files` 携带 ref（非 bytes）+ MIME=application/pdf。
  - `internal/provider/gemini/gemini_test.go`：PDF file part → `inline_data`
    带 application/pdf（`TestToPartFilePDF`）。
  - `internal/provider/anthropic/anthropic_test.go`：PDF file part →
    document block（非 image block）（`TestUserBlocksFilePDF`）。
  - `internal/cli` / 或 protocol：`--file` 装载 sniff MIME、非图片不报错。
- **闸门 B（真实 API，QA-15）**：驾驶舱 `+` → File 传一份小 PDF（含一个
  已知 magic 词），发"这份 PDF 里写了什么关键词?"，Gemini 真实读出。
  断言只钉 runtime 红线（ref-not-bytes + 文件 part 上链），不钉模型措辞。

## 实施步骤

1. **INC-9.1 产品**（一个可合并提交，check.sh 全绿）：
   - `protocol/input.go`：加 `FileAttachment{MediaType, Data}` + `UserInput.Files`。
   - `daemon/daemon.go`：`Command.Files` + `UserInput{...Files: cmd.Files}`。
   - `agent/conversation.go journalInput`：摄入 `in.Files` → CAS → `event.Files`（真实 MIME）。
   - `cli/conversation.go`：`send --file`（可重复）；`loadFileAttachments`（sniff MIME，不拒）；`loadImageAttachments` 保持只收 image。
   - `provider/anthropic`：`application/pdf` → document block 分支。
   - 三个孪生/单测（闸门 A）。
2. **INC-9.2 驾驶舱 + 收口**（一个提交）：
   - webui `/api/sessions/{sid}/send` 加 `files` 参数 → `ar send --file`；
     `AR.send` 带 files；Composer `+`→File 走图片外的任意文件（≤10MB）。
   - 真实 API QA-15 归档 `qa/runs/`；SPEC/DESIGN(§17/§9.1)/GAPS(G1 余项)/
     QA/LOG 并回；本工作纸移 `archive/increments/`。

## review 裁决

小增量、不触不变量、opt-in（不改既有 image/长贴折叠形态）。**裁掉三视角
对抗 review**，理由：改动面窄且沿用已 review 过的 file-part 通路；正确性
由闸门 A（含 ref-not-bytes 断言）+ 闸门 B（真读 PDF）覆盖；安全面无新
ingress（附件字节沿用既有 CAS 通路，未改凭据红线——附件字节不过 redaction
是既有属性，LOG 记档不在本增量改）。
