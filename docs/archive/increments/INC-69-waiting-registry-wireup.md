# INC-69 waiting 注册表接线 + command.Discover 裁决（G31-unwired 三件套，audit-0717 D0）

## 动机与 journey 锚

audit-0717 C2 甄别出 3 个 G29 式"设计了未接线"导出（GAPS G31、LOG
2026-07-18）。不接不删是最坏态：注册表想防的 ad-hoc 内联已经长出来了。
无新 journey——本增量是既有不变量（DESIGN 2.14 waiting 注册表、3.5
denied-by-interrupt）的机制兑现 + 一项死码裁决，行为零变化。

## Spec delta

无新功能点。SPEC 附录 tool/命令清单不变。

## Design delta

- DESIGN 2.14 的 waiting 注册表由"声明性文档"变为**载荷真相源**：
  1. 生产中断路径（ask-park、审批 park，各 cmdAppend 变体）不再硬编码
     `superseded_by_interrupt`/`denied_by_interrupt` 字面量，改读
     `WaitRules[kind].OnInterrupt`；
  2. `WaitingEntered` 的三个生产点（loop.go idle park、ask park、
     approval park）经 `CanProduce` 守门（违规=loud error，行为中性
     ——现有产生点全部满足规则）。
- `ResolveWaitingOnInterrupt` 的整体形状（自带 InputReceived + 无
  cmdAppend/AskResolved 交错）与任何生产站点都不匹配（idle 处
  interrupt 依裁决 #11 是 no-op，ask/approval 站点各有配对交错），
  **删除**该函数；其测试改为断言注册表行值 + 生产站点经由注册表取词
  （字面量单一来源）。
- `command.Discover`/`parseFrontmatter`：接线前提是"列 slash 命令"的
  CLI 产品面，该面无 journey 锚（UJ-19 只覆盖展开执行）。裁决:
  **删除**，GAPS 记档"未来 slash 列表面按 skill.Discover 模式重建"
  （assembly.go:109 有活模板）。不触不变量。

## 验收

- 行为中性：既有全部单测/孪生不变绿（中断路径语义由
  TestPlanApprovalFullFlow、conversation/approval 既有套件锚定）。
- 新锚：TestWaitRulesAreResolutionSource（站点字面量=注册表值——防
  回退硬编码）、TestCanProduceGuardsProducers（守门接线事实）。
- deadcode 基线再减 4（CanProduce 接线、ResolveWaitingOnInterrupt/
  Discover/parseFrontmatter 删除），lint-wiring 绿。

## 实施步骤

1. 站点字面量改读注册表 + CanProduce 守门 + 删函数/死码 + 测试迁移
   + 基线更新 + GAPS/LOG/SPEC 行齐活（单 commit,check.sh 全绿）。

## review 裁决

小增量、行为中性、零线上语义变化：裁掉三视角 review,以"全部既有
测试不变绿 + 新锚"为界。
