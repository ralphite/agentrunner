# 历史归档（只读）

本目录存放已完成计划与过时审查的**封存件**。它们是历史事实的记录,
不再更新、不再作为任何决策依据;与活文档冲突时,一律以 `docs/` 根下的
活文档为准(裁决顺序见 `docs/PROCESS.md`)。每份文件头部有归档注记。

## 索引

### v1/ — 第一代计划(actor+durability 出发,S1–S7,已完成)

| 文件 | 是什么 | 有效内容的去向 |
|---|---|---|
| `v1/DESIGN.md` | v1 架构设计原文 | 与 v2 设计合并为 `docs/DESIGN.md` |
| `v1/STAGES.md` | 七阶段分期 | 已全部完成,无承接 |
| `v1/PLAN.md` | step-by-step 实施计划 | 执行协议/acceptance 框架 → `docs/PROCESS.md` |
| `v1/PROGRESS.md` | v1 决策台账 | 后续台账 → `docs/LOG.md` |

### v2/ — 第二代计划(会话内核重造,M1–M5+收口,已关闭)

| 文件 | 是什么 | 有效内容的去向 |
|---|---|---|
| `v2/DESIGN.md` | v2 中心模型设计原文 | 与 v1 设计合并为 `docs/DESIGN.md` |
| `v2/CORE.md` | 十项核心清单与现状对照 | 功能点活登记 → `docs/SPEC.md` |
| `v2/MIGRATION.md` | fix-in-place 迁移路线 | 已执行完毕,无承接 |
| `v2/PLAN.md` | v2 实施计划 | 执行协议 → `docs/PROCESS.md` |
| `v2/PROGRESS.md` | v2 决策台账 | 后续台账 → `docs/LOG.md` |

### reviews/ — 基于旧版设计的外部审查(结论已并入,原件留档)

| 文件 | 是什么 |
|---|---|
| `reviews/CAPABILITY-REVIEW.md` | 能力对照审查(旧设计基线),有效结论已并入设计 |
| `reviews/CAPABILITY-REVIEW-DETAILS.md` | 上者的逐项明细 |
| `reviews/DESIGN-SUGGESTIONS.md` | 外部设计建议,有效结论已并入设计 |
| `reviews/FEATURES.md` | 早期功能清单,已被 JOURNEYS/SPEC 体系取代 |

### increments/ — 已完成增量工作纸

| 文件 | 是什么 |
|---|---|
| `increments/INC-19-webui-product-rebuild.md` | Codex 母版 + AgentRunner Supervision 的 Web UI 产品化重构工作纸；活定义已并入三层文档与 QA |
| `increments/INC-29-webui-ux-round3.md` | 结构化 Run details、concise title 与统一状态语义；活定义已并入 SPEC/QA/GAPS/LOG |
| `increments/INC-38-codex-result-closure.md` | truthful hydration、Worked/Changes 收尾、环境条与 sidebar hover；活定义已并入三层文档与 QA-41 |

## 归档纪律

- 计划关闭、文档被取代时移入本目录,加头部归档注记,不改动正文。
- 归档件内部的相对路径引用**不修复**(保持历史原貌);读者需自行
  对照当时的仓库布局。
- 永不删除:journal 式留痕,审计与考古都靠它。
