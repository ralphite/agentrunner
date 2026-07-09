# INC-13 microcompact——assembly 层旧工具结果降级（不调 LLM 的轻量上下文回收）

## 动机与 journey 锚

UJ-09 长会话续命。现状只有一档"全量 LLM 摘要"（自动阈值 S3 + 手动
INC-6）：有损、有成本、有延迟；而长会话 context 的大头常是**可重算的
旧工具输出**（read_file/grep/glob/semantic_search 的结果）。对标
CLAUDECODE-PARITY #18/§3.2——Claude Code 四级体系的 microcompact 层：
把可重算旧 tool result 原地替换为占位符，**不调 LLM**（"非必要不调
LLM"）。我们的架构做它更顺：tool result 本就是 journal 事件、assembly
是纯渲染——降级是 assembly 策略，**零事件变更、fold 不动、resume
语义自动成立**。

## Spec delta

- SPEC A 新行：microcompact（assembly 层把久远的可重算 read-class
  工具结果渲染为占位符；先于 autocompact 生效；journal/fold 零变更）
  ——锚 `TestMicrocompact*` + QA-19。
- CLAUDECODE-PARITY §2 #18 状态 ❌→✅。

## Design delta（不触不变量）

DESIGN §4「Context assembly」加一段：
- **策略**：assembly 渲染时，若 context 估算超过 microcompact 阈值
  （默认 = autocompact 阈值之前的一档），从最老的 turn 起，把
  **read-class（幂等可重算）工具**的 tool_result part 替换为占位文本；
  最近 N 个 turn 的结果**永不降级**（保护工作集）；execute/edit 类
  结果不降级（不可重算，且是决策 #6 in-doubt 语义的载体）。
- **配对不变**：替换保留 call/result 配对与 harness call id（决策 #9
  严格配对不动）；只换 result 的内容 part。
- **占位文本**：`[old tool result cleared to save context — re-run
  the tool if needed]`——模型可自行重跑工具取新值。
- **不变量论证**：决策 #3（fold 纯、event 不动）、#5（无 code
  replay）、#9（配对）全部保持；assembly 本就是"从 state 渲染
  provider 消息"的策略自由度所在（同类先例：长贴折叠、组装 inflate、
  ContextCompacted 摘要头）。
- **余项记档**：首版不做"来源未变"检测（占位文本已明示 re-run；
  Claude Code 同样不保证）；跨 provider prompt-cache 影响（降级点
  移动会破前缀缓存——降级只在阈值跨越时批量发生一次，且 autocompact
  本就更粗暴地破缓存）。

## 验收

- 孪生：`TestMicrocompactClearsOldReadResults`（超阈值→旧 read 结果
  变占位、近窗保留、execute 结果保留、配对完整）/
  `TestMicrocompactBelowThresholdNoop` / `TestMicrocompactResumeStable`
  （resume 后同一 assembly 结果——纯函数自证）。
- 真实 API QA-19：真 Gemini 长会话灌大量 grep/read 输出→跨过
  microcompact 阈值→继续对话模型仍能正确工作（必要时重跑工具）；
  `ar events` 导出归档 qa/runs/。
- `./scripts/check.sh` 全绿。

## 实施步骤

1. 探码确认挂点（assembly 入口/token 估算/tool class 查询）——一步一
   commit 从 2 起。
2. assembly 降级逻辑 + 阈值配置。
3. 孪生测试三件。
4. QA-19 场景脚本 + 真实 API 跑通归档。
5. 文档行齐活（SPEC/CLAUDECODE-PARITY/LOG/SPRINT 状态表）。

## review 裁决

做。S 号、纯 assembly 策略、对方生产验证过的形态、UJ-09 直接受益、
不触不变量（论证见 Design delta）。
