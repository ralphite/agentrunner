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
| `specs/spec.md` | 产品 Spec v0.7（功能、模型、交互、验收标准） |
| `specs/plan.md` | 技术方案：文件格式约定、原型分期、UI 基准 |
| `specs/tasks.md` | Stage 1 可交互原型的实施拆解 |
| `mock/index.html` | Stage 0 高保真 HTML mock（Notion 风格，四屏可切换，浏览器直接打开） |
| `reference/product-record.html` | 源头：ChatGPT session 的完整产品记录（v0.6 Spec + 需求归纳 + 16 轮决策） |
| `reference/product-record.md` | 上面的纯文本提取版，便于 grep 与 agent 阅读 |
| `reference/mock-*.png` | 设计讨论阶段的 4 张 UI 参考图 |

specs 的产物结构参考 GitHub Spec Kit（spec-driven development）的四件套：
constitution / spec / plan / tasks；只取其文档形态，不引入其 CLI 与流程工具。

## 当前状态

- **Stage 0（本目录当前内容）**：Spec v0.7 定稿 + 静态高保真 mock。
- **Stage 1（下一步）**：可交互 Web 原型——真实现文档树/编辑/选择引用/
  Widget/Inspector，agent 响应用 mock 模拟，数据层直接采用
  `specs/plan.md` 定义的文件格式。
- **Stage 2（后续）**：真文件系统落盘 + 真 Coding Agent 适配。

## 设计基准

UI 以 Notion 为直接参照（布局、密度、交互习惯"几乎照抄"），
细节对齐 `reference/mock-*.png` 四张参考图。
