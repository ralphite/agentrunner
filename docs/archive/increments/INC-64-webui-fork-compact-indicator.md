# INC-64 WebUI fork 可见性 + compaction indicator

## 动机与 journey 锚

用户给出 Codex mobile 截图，要求 WebUI 在允许时有 fork 按钮，并把
context compaction 作为 thread 中的低噪声分隔线；同时确认 `/compact`
支持手动指示。锚点为 UJ-09（手动 compact 带指示）、UJ-15
（barrier/fork）、UJ-24（Web UI 单一 task thread）。

## Spec delta

- A 区 `手动 compact（带指示）/ clear`：补 WebUI `/compact <directive>`
  接线测试锚。
- I 区 Web UI task thread/收尾：补 fork 顶栏可见性、compaction divider
  前端测试锚。

## Design delta

无。不改变 `ContextCompacted`、`CheckpointBarrier`、fork/rewind 或 durable
command 语义；WebUI 只是消费既有 journal/API。`directive` 仍是
`ar compact <sid> [directive]` 的单 argv 透传，无 shell。

## 验收

- 孪生：`TestHandleCompactForwardsDirective` /
  `TestHandleCompactOmitsEmptyDirective`。
- 前端：`slash.test.ts` 覆盖 `/compact <directive>`；`timeline.conv.test.ts`
  + `Timeline.thread.test.tsx` 覆盖 compaction divider；`SessionView.chrome.test.tsx`
  覆盖有 checkpoint 才显示 fork 顶栏按钮。
- 常规闸门：`./scripts/check.sh`。
- 真实环境浏览器 QA：共享 store / real webui，打开已有 checkpoint session
  `20260710-045637-use-the-bash-tool-to-run-exact-cd25`，验证 fork 顶栏按钮、
  compact divider、`/compact <directive>` 请求链。证据：
  `qa/runs/2026-07-12-INC64/`。

## 实施步骤

1. WebUI compact directive：前端 API/Composer slash → Go handler → `ar compact`
   argv，补 Go/前端单测。
2. WebUI timeline：`context_compacted` 投影为独立 divider，不进 Worked fold。
3. WebUI fork：有 `checkpoint_barrier` journal 事实时顶栏显示 icon-only fork
   button；无 checkpoint 保持安静，更多菜单仍可创建 checkpoint。

## review 裁决

小 UI/接线增量，裁掉三视角对抗 review；风险由单元测试、前端测试、
`check.sh` 和真实 WebUI QA 覆盖。
