# INC-47.2 结构化 ask_user webui（HANDA #7 步2）

## 动机
步1 落了模型侧+协议+CLI（INC-47.1）。步2 补 webui：结构化提问的分步
表单卡（waiting:input 且 park 带 questions 时渲染）、queued 消息撤回
按钮（复用 #29 的 ar queue/unqueue）。

## Spec delta
- SPEC ask_user 行「webui 表单卡=步2」→ ✅（表单卡 + queued 撤回）。

## Design delta
- 无。inspect waiting 报告 additive 暴露 ask questions（供前端渲染）；
  webui 后端加 `POST /answer`（→ ar answer）与 `POST /unqueue`
  （→ ar unqueue）+ `GET /queue`（→ ar queue --json）薄包装。

## 验收
- A: frontend build+vitest 绿。
- B（真浏览器+真 Gemini）：结构化提问 → 表单卡点选 → 提交 → 模型续跑；
  忙时排队 → 撤回按钮 → 消息消失。归档 qa/runs/。

## 实施步骤
1. inspect waiting.ask_questions（Go）+ webui /answer //queue /unqueue
   端点 + AR.answer/queue/unqueue（前端 api）。
2. AskForm 组件（分步/单多选/skip）+ queued 气泡撤回按钮 + 真验 →
   文档 → commit。

## review 裁决
纯 additive webui，裁三视角 review；B 闸真浏览器覆盖。
