# INC-78 Scratch 工程不再聚合 + project 改名触屏可达

## 动机与 journey 锚

用户实机(iPhone,2026-07-19)裁决:sidebar 把所有自动 workspace 会话
混进一个 "Scratch" 文件夹——"Building Collaborative Agent Runtime"、
"Create a todo app" 等互不相关的工程共居一组,组内既分不开也没法
分别命名;且 project 改名入口只有右键/Shift-F10,触屏根本打不开。
原话:"We should not mix projects in the scratch folder. Also we
should allow editing a project name."

- 动 UJ-24 步 1:"自动 workspace 合并为 Scratch" → 每个自动 workspace
  各自成组(默认名 "Scratch · MM-DD HH:MM"),不互相混合;组名可改,
  改名后即读作正式工程名。
- 触 DESIGN §UI 重构合同行(L1322-1324"投影为单一 Scratch"):该行
  当年动机是"不泄漏实现 id"——保留该动机(默认名仍不露 ws<ts> 裸名,
  用 Scratch · 时间形态),只撤"单一聚合"。非标记不变量;用户当面
  裁决,按增量流程落地。

## Spec delta

- sidebar 信息架构行(UJ-24 覆盖面):"自动 workspace 合并为单一
  Scratch" 改为 "每个自动 workspace 独立成组,默认名 Scratch · 创建
  时间;组名可编辑(INC-53 overlay,per-workspace 键)"。
- project 菜单行:补 "触屏(≤900px)在 project 标题行提供 ⋯ 菜单,
  与右键菜单同内容"。

## Design delta

- DESIGN L1322-1324:改为 "自动生成的 `ws<timestamp>`/`wt<timestamp>`
  workspace 各自成组,默认名投影为 `Scratch · MM-DD HH:MM`(不泄漏
  实现 id、不隐藏 session、不互相合并);组名经 project overlay
  (INC-53,workspace 为键)可改"。grouping 以 workspace 为键的总则
  不变(本增量使 Scratch 回归总则,删除唯一例外)。
- 旧 `__scratch__` 聚合键的 overlay 记录(改名/折叠)成为孤儿:装饰
  性数据,不迁移,显式接受(改名本就该落到具体 workspace 上)。

## 验收

- 单测(闸 A):viewModels buildSidebarModel——两个 scratch workspace
  → 两个组、key=workspace 路径、label 各带创建时间;不再出现
  `__scratch__`;既有断言(label "Scratch" 单组)同步修订。
  projectDisplayName per-workspace overlay 改名沿用既有测试。
- 远程真环境(闸 B,remote-qa-env agent 驱动):①sidebar 中两个
  scratch 会话各居其组;②390×844 视口下 project 标题行 ⋯ 可点、
  Rename project 可达、改名后组名即时更新且刷新后保持(overlay 持久)。
- 裁掉项:旧 `__scratch__` overlay 迁移(装饰性,见 Design delta);
  composer 标题/palette 等其他表面的 Scratch 文案(不属本诉求,维持
  projectLabel 现语义)。

## 实施步骤

1. viewModels.projectIdentity 拆分 scratch 聚合 + 测试修订(一个
   commit,含三层文档行与本工作纸归档)。
2. Sidebar project 标题行触屏 ⋯ 菜单(复用 session 行既有 Menu 模式,
   菜单内容与右键 ContextMenu 同源抽取)。
3. 远程闸 B 验收轮(红→绿证据记 LOG)。

## 用户裁决(2026-07-19)

用户质疑 "Scratch" 概念本身后(历史考古见 LOG 同日条目),呈报三个
方向:A(每 workspace 独立成组,可改名)、B(无项目会话完全平铺)、
C(取消静默铸造 ws-目录)。**用户裁决:选方案 A,继续落地**——B/C
不做,本工作纸即为终态记录。

## review 裁决

小增量(呈现层分组 + 菜单可达性,无并发/安全面,不触 daemon/journal
契约),裁掉三视角 review;正确性由闸 A 单测 + 闸 B 远程实驾覆盖。
