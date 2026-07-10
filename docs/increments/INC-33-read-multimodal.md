# INC-33 Read 工具多模态（读图/PDF 入 context，#32，SPRINT #13）

## 动机与 journey 锚

CLAUDECODE-PARITY §2 #32 / SPRINT #13。对标 Claude Code Read 工具读图/
PDF。现状:输入侧多模态已通(INC-9:附件→CAS ref→part→assembly inflate→
provider 映射),但 **read_file 对 PNG/PDF 返回乱码文本**——模型无法主动
去读 workspace 里的图(截图/设计稿/PDF 文档)。本增量补工具侧,复用
INC-9 全部管线。

## 设计(不变量:journal 永不落 blob 字节)

**三段接线,零新管线**:

1. **executor(tool 侧)**:read_file 以 `http.DetectContentType` 检测——
   `image/*` → image 分支;`application/pdf` → file 分支;**其余走既有
   文本路径,零变化**。media 分支:bytes→`Executor.Blobs.Put`(新
   `BlobStore` 接口 seam,mutex 保护 SetBlobs 首设生效)→ 返回 **media
   envelope** `{kind:"image"|"file", media_type, ref, bytes, note}`——
   journal 只见 ref(blob-before-event:Put durable 先于 tool result
   落盘)。Blobs 未接(裸 executor)→ 显式 error;超 5MB 上限 → error。
2. **loop 接线(单点)**:`Run` 在既有 `ensureArtifacts()` 之后把
   `l.Artifacts`(树共享根 store)注入 `l.Exec.SetBlobs`——父子共享
   executor 时注入同一 store,幂等。
3. **assembly(消费侧)**:构造 toolMsg 时,`read_file` 的结果若为 media
   envelope(**工具名 + shape 双重门**,防 MCP 工具巧合 payload 长出
   bogus ref 毒化 turn)→ 在该消息**所有 tool_result parts 之后**追加
   `Part{Kind: image|file, Ref, MediaType}`(Anthropic 要求 tool_result
   块在前;Gemini 无所谓)。`inflateBlobs` 既有逻辑对全部消息生效——
   请求时 inflate,fold/journal 恒 byte-free。microcompact elide 时
   result 已是占位符,envelope 不再匹配 → 图自动随占位剥离(旧图正是
   最重的 context,行为恰当)。

## Spec delta

- SPEC C read_file 行:+ 读图/PDF(media envelope + part 注入,journal
  只 ref);defs/read_file.json description 提示模型可读图/PDF。
- CLAUDECODE-PARITY #32 状态更新。

## Design delta(不触不变量)

blob-before-event/fold byte-free/inflate-at-assembly 全部复用 INC-9 既有
纪律,零新事件类型。DESIGN §4 多模态段加一句工具侧。

## 验收

- 孪生:tool 包 `TestReadFileImage/PDF/TextUnchanged/NoBlobStore/
  OversizeMedia`(envelope 形状/ref 非空/payload 无字节/默认路径零变化/
  裸 executor 显式错/上限);agent 包 `TestReadFileMediaAssembly`(fold
  →assembled toolMsg 含 tool_result+PartImage{Ref},elide 后无图)+
  `TestReadFileImageEndToEnd`(scripted 全链:模型调 read_file(png)→
  第二请求含 inflate 后的 image part,journal 全程无 base64)。
- 真实 API QA-38(daemon-path→私有新二进制 daemon):workspace 放
  qa/fixtures/build-error.png,让 Gemini read_file 读图并说出图中错误
  的文件/行号(证模型真看到了图);journal 断言 envelope ref + 无大
  base64 行。
- 绿门(排除已知环境测试)。

## 实施步骤

1. tool: BlobStore 接口 + Executor.SetBlobs/blobStore + readFile media
   分支 + defs 描述。
2. agent: Run 注入 + assembly mediaResultPart 追加 + multimodal.go 归位。
3. 孪生 + QA-38。
4. 文档行齐活。

## review 裁决

做。M 收敛为 S+(一个接口 seam + 一个 media 分支 + assembly 一个追加
点)。inline 自审:correctness(默认文本路径零变化/envelope 双重门/
Anthropic 块序/elide 剥离)、security(WS.Resolve 边界不变/上限/journal
恒 byte-free)、contract(既有 read_file 测试不触/scripted provider 对
image part 容忍/registry 无新工具无 golden 变化)。
