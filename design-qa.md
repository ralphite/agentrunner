# Design QA — INC-19 Web UI

## Source of truth

- 用户确认的母版：
  `/Users/yadong/.codex/generated_images/019f4935-7896-7c13-ab9e-a40abefdb07c/exec-1a047500-51f2-4330-8ae6-7eb8cb676e44.png`
- 视觉规则：Codex 通用 UI/UX；AgentRunner 品牌；AgentRunner 独有能力只在
  Codex 视觉语言上的 Supervision 扩展。

## Final implementation evidence

- 全屏 1554×1014：
  `qa/runs/2026-07-09-QA27/approval.png`
- 同图对照（source 左 / implementation 右）：
  `qa/runs/2026-07-09-QA27/design-compare-full.png`
- 主工作区 focused 对照：
  `qa/runs/2026-07-09-QA27/design-compare-focus.png`
- 状态截图：`home.png`、`supervision.png`、`changes.png`
- 响应式：`responsive-900.png`、`responsive-700.png`

## Comparison history

1. Full-view pass：发现 P1——真实 team session 的 revive/重复回执让
   Supervision 把同一 engineer/reviewer 各画两次。按 child session id 去重，
   保留最新 inspect 状态；行改为语义化 button。单测与真实 DOM 均验证每个
   成员恰一行。
2. Full-view recheck：三栏比例、安静 header、单一 thread、内联 approval、
   底部 composer 与右侧 Supervision 的层级和母版一致。0 P0/P1/P2。
3. Focused pass：approval 先显示动作/对象/scope，raw args/gates 折叠；
   Approve once 为主操作、Deny 分立；Attention 与 card 使用同一 pending
   真相。0 P0/P1/P2。
4. Responsive pass：900/700 下 Supervision 变可关闭浮层，thread/composer
   不被拆成另一套 IA。0 P0/P1/P2。

## Final verdict

PASS。实现忠于已确认的 Codex 母版，同时只把 AgentRunner 的 Goal / Agents /
Attention / Background work 作为原生 Supervision 扩展；无阻塞交付的视觉、
交互或信息架构偏差。

---

## INC-23 B3–B6 黑盒复核

- 母版不变；最终同尺寸 1554×1012 对照：
  `qa/runs/2026-07-10-QA32/27-reference-vs-implementation.png`。
- 首轮证据暴露 7 个结构性 P1：窄窗遮挡、recovery 无入口且 Attention 撒谎、
  Scheduled 重启丢失、task 行不可键盘进入、非人类 input 冒充用户、raw
  launcher 主层泄漏、移动端 sidebar 覆盖内容。均已修复并用真实 session
  重走。
- 最终对照：sidebar/thread/approval/composer/Supervision 的比例、层级、边框、
  圆角、密度与 Codex 母版一致；AgentRunner 品牌与独有 supervision 数据不
  另造设计语言。P0/P1/P2=0，PASS。
