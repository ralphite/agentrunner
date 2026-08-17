# Context Center

Document-first、Agent-aware 的项目工作空间原型。人和 AI 在同一棵可编辑的
文档树里维护项目记忆（背景、Workstream、Plan、Task、Bug、结果、Lesson），
把代码执行委派给现有 Coding Agent（Codex、Claude Code），结果与经验回流
到文档树。

> **本目录与仓库其余部分（agentrunner）无关**，是一个独立孵化的产品原型。
> 所有 Context Center 相关内容都住在 `contextcenter/` 下，不读写外面的
> `docs/`、`internal/` 等。

## 目录地图

| 路径 | 内容 |
|------|------|
| `specs/constitution.md` | 产品不变量（第一原则，改它要走修宪流程） |
| `specs/spec.md` | 产品 Spec（功能、模型、交互、验收标准；版本号见文件头） |
| `specs/plan.md` | 技术方案：文件格式约定、原型分期、UI 基准 |
| `specs/tasks.md` | Stage 1 可交互原型的实施拆解 |
| `specs/gaps.md` | 未定义清单（Gap Audit）：52 项全标注，P0 已清零 |
| `specs/pending-decisions.md` | 剩余待决 D1–D15（含建议方案）+ 已决事项 review 清单 |
| `app/` | **shadcn/ui + Radix 版应用壳**（React + Vite + TS + Tailwind v4；`npm i && npm run dev`）——mock 的正式栈迁移，行为仍为演示级 |
| `mock/index.html` | Stage 0 高保真 HTML mock（历史基线，浏览器直接打开） |
| `reference/product-record.html` | 源头：ChatGPT session 的完整产品记录（v0.6 Spec + 需求归纳 + 16 轮决策） |
| `reference/product-record.md` | 上面的纯文本提取版，便于 grep 与 agent 阅读 |
| `reference/mock-*.png` | 设计讨论阶段的 4 张 UI 参考图 |

specs 的产物结构参考 GitHub Spec Kit（spec-driven development）的四件套：
constitution / spec / plan / tasks；只取其文档形态，不引入其 CLI 与流程工具。

## 当前状态

- **Stage 0**：Spec v0.9 + 静态高保真 mock（历史基线）。
- **app/ 壳已启动（用户指示 2026-08-16）**：mock 迁移到正式栈
  （shadcn/ui + Radix），视觉 1:1、控件真实（下拉/右键菜单/tooltip/
  状态单源联动）；行为层 gaps 未裁决的部分仍是演示级。
- **✅ P0 已清零（2026-08-16），行为层解冻**：编辑器/Agent 协议/生命
  周期的关键裁决全部落档，见 `specs/gaps.md`；剩余开放项非阻塞。
- **Stage 1（进行中）**：在 app/ 上落数据层（`specs/plan.md` 文件格式
  + SQLite 伴随库）与真实行为，按 `specs/tasks.md` 推进。
- **Stage 2（后续）**：真文件系统落盘 + 真 Coding Agent 适配。

## 设计基准

UI 以 Notion 为直接参照（布局、密度、交互习惯"几乎照抄"），
细节对齐 `reference/mock-*.png` 四张参考图。
