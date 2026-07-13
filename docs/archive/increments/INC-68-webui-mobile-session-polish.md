# INC-68 Web UI 移动端 session 细节收口

> **已归档（2026-07-13）**：实现与 QA-68 完成，活定义已并入
> SPEC/QA/GAPS/LOG；本文件只保留为历史工作纸。

## 动机与 journey 锚

UJ-24 要求 Web UI 在共享真实 session 上提供响应式导航、异常状态、子会话
导航与 deep link。2026-07-13 以 390×844 iOS 视口、真实 prompt 与保留的三
worker 会话逐屏审计，复现 G36 同族的五枚可见缺陷：侧栏可与 Environment
叠开；failed session 同时出现具体失败卡和泛化终态卡；activity group 暴露
浏览器原生 marker 且数量缺少语义；子会话 link 在窄卡片内黏连；child header
显示 machine slug 而不是 inspect 已知的 agent spec。

## Spec delta

- 扩充 SPEC「Web UI 交互语义」：移动导航与 Environment 互斥；具体 failure
  优先于泛化 terminal notice；activity/child-session 在窄屏仍可读；child header
  优先显示 inspect 的 agent spec。
- 实现后锚到本增量的 frontend DOM 测试与 QA-68 真浏览器证据。

## Design delta

不修改 DESIGN。改动只收紧既有 UJ-24 Web UI 投影与响应式展示，不改变
runtime/session/transfer 语义，也不触及 §15 不变量。

## 验收

- 闸门 A：frontend 测试覆盖 mobile navigation 打开即关闭 Environment、具体
  failure 去重、child inspect spec 标题、activity count 与 child link DOM；
  `./scripts/check.sh` 全绿。
- 闸门 B（QA-68）：只用共享 `~/.local/share/agentrunner/` daemon/store 和
  live Web UI；以用户给定中文 prompt 创建并保留新 session；在 390×844 打开
  parent 与全部三个 child、completed/failed/recovery/Changes/New session/
  picker/sidebar/Environment；1280×900 复核桌面无回归；逐步截图，console
  warning/error=0、页面横向 overflow=0、deep link/reload 保持；events、inspect、
  workspace diff 与截图归档到 `qa/runs/2026-07-13-QA-68/`。
- 枚举交付物：上述五枚 finding 各有 before/after；审计未发现缺陷的状态也在
  QA-68 记录为健康。session/workspace/journal 全部保留，不 close、不清理。

## 实施步骤

1. 一步提交：最小 frontend/CSS 修复、测试、QA-68 真机回归、活文档收口与
   工作纸归档；完成标志为 `./scripts/check.sh` 全绿并 push `origin/main`。

## review 裁决

裁掉三视角独立 review：这是不触不变量、无后端/持久化/权限语义变化的单步
UI 缺陷批；以同一真实状态的 before/after 对照、全量 gate 与桌面回归替代。
