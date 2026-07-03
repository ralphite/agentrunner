# AgentRunner — 项目约定

## Git 规则（硬性）

- **只用 main 分支。** 不创建任何其他分支；不在其他分支上工作。
- **每次改动完成后立即 commit 并 push 到 `origin/main`。** 不留未推送
  的本地提交，不留未提交的工作区改动。单人原型项目——分叉和滞后的
  代价远大于中间态提交的噪音。
- **每个 session 开始时先 `git fetch origin main` 并 fast-forward**，
  确保永远在最新代码上工作（曾发生过基于过时 DESIGN.md 产出整份
  review 的事故）。
- `.env` 已 gitignore（存本地凭据如 `GEMINI_API_KEY`），永不提交。

## 文档体系（改动任何一份都要检查与其他两份的一致性）

- `DESIGN.md` — 架构 source of truth（是什么、为什么）。
- `STAGES.md` — 七阶段分期（切成什么块、每块完成标志）。
- `PLAN.md` — step-by-step 实施计划（怎么做）；§0.5 是 loop-mode
  执行协议，§0.6 是 acceptance test 框架。实施顺序以 PLAN 为准。
- 动 DESIGN.md 不变量必须走 PLAN §末尾的"不变量变更流程"
  （停下、写清冲突、单独 review），禁止代码里先绕。

## 语言与实现约定

- 叙述用中文，技术术语/代码/标识符用英文。
- 实现语言 Go 1.23+（决策 #1）；主 provider Gemini、次 Anthropic。
- 实现进度记录在 `PROGRESS.md`（执行协议规定的决策台账）。

## 历史留档（只读，以 DESIGN.md 为准）

- `CAPABILITY-REVIEW*.md`、`DESIGN-SUGGESTIONS.md`、`FEATURES.md`
  是基于旧版设计的外部审查，有效结论已并入 DESIGN.md，顶部有鉴定注记。
