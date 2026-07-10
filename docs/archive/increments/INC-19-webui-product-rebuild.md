> **归档（2026-07-09）**：INC-19 已完成；活定义已并入 JOURNEYS/SPEC/
> DESIGN/QA/GAPS/LOG。本文件仅保留实施期决策与证据索引。

# INC-19 Web UI 产品化重构（Codex 交互模型）

## 动机与 journey 锚

现有 `webui/` 是按 CLI/journal 概念堆出的 QA 驾驶舱：任务在侧栏与首页
重复；创建前暴露 permission/persona/model/workspace/start-mode/branch；
审批直接暴露 gate/args；goal、子 agent、后台 handle 与 composer 争夺底部
空间；完成态也没有稳定的 review-first 收口。用户已明确选择当前 Codex
桌面端作为通用 UI/UX 母版，并允许 AgentRunner 独有的 supervision/goal/
agent tree 作为 Codex 视觉语言下的原生扩展。

新增 UJ-24「Web UI 驾驶 AgentRunner」：用户从项目/任务层级进入会话，
在单一时间线中派活、续聊、审批、审阅改动；复杂运行的 goal、成员、
attention 与后台工作在可收起的 Supervision 面板集中呈现；任何已有会话
（含 CLI 创建、父/子 session、运行中/审批中/失败/静止）都能直接打开。

## Spec delta

在 SPEC「观察与远程面」登记以下产品能力：

1. Codex 式 Web UI 信息架构：New task / Scheduled / Pinned / Projects → task。
2. 同一 task thread 承载历史、流式活动、附件、审批、diff 与 follow-up。
3. 审批默认只回答「要做什么、影响哪里、允许一次还是拒绝」；原始 args/
   gates 收进 Details，拒绝理由按需展开。
4. AgentRunner Supervision：goal、agent tree、attention、background handles
   集中在右侧次级面板；成员可点入完整子会话。
5. composer progressive disclosure：默认只留输入、附件、access、model、
   send；Goal/Loop/Best-of-N/spec 等高级启动器收进二级菜单。
6. Web UI 纳入常规构建/测试门，而不再声明为“非产品 QA 面”。

验收锚：前端 view-model/组件测试 + `npm run build` + `QA-27` 真实共享
daemon/store 浏览器验收。

## Design delta

修改 DESIGN §12 Surfaces，不触及 §15 不变量：Web UI 仍是公开 CLI/daemon
协议上的薄 surface，不新增第二套 session 真相、不复制状态机、不持久化
私有 session 元数据作为运行真相。布局与交互改造只改变 projection：

- 左侧按 workspace/project 分组 session；缺 workspace 的会话进入
  `Other sessions`，不因 metadata 缺失而不可见。
- task thread 的 journal/API 仍是唯一真相；Supervision 只投影 `inspect`、
  `ps`、goal 与 pending approval。
- diff 必须从 session/store 可得 workspace；Web UI 私有 metadata 仅作
  兼容补充，不能成为唯一来源。
- approval action 仍走 durable approve command；UI 不暗示本次审批会放宽
  未实现的权限范围。
- 所有用户已有 localStorage（pin/archive/rename/theme/sidebar/unread）保留，
  无迁移、无静默丢弃。

### 产品设计说明（实施前 review）

**沿用模式**：严格采用用户提供的三张 Codex 桌面端截图与已确认的
“Codex 原生结构 + AgentRunner Supervision”母版；品牌字样、项目名、
运行数据使用 AgentRunner 自己的。

**拟议 UI**：侧栏只做全局导航与 project/task hierarchy；首页只负责
创建任务，不再复制任务网格；task header 保持安静，Changes 与
Supervision 是右上角次级控制；会话主体保持单一滚动时间线；审批作为
时间线中的原生卡片；右侧 Supervision 卡片按 Goal / Agents / Attention /
Background work 分区。

**风险状态**：Approve once 是主操作但不默认自动触发；Deny 先展开可选
理由再提交；批量审批从默认面移除；kill/cancel/close 保留在明确的
advanced/lifecycle 菜单，不提升视觉层级。

**数据与内容**：不改 journal/store schema；不清理任何真实 session、
workspace、QA 数据或浏览器本地设置；CLI 创建的历史会话与子会话必须能
正常渲染；附件引用继续保留。

**已裁决问题**：Scheduled 映射现有 submit/drive run；Plugins/Sites 等
AgentRunner 没有的产品面不伪造；Trust directory 与 theme 留在品牌/环境
次级入口；Supervision 默认在宽屏显示、窄屏可收起。

## 验收

### scripted 孪生

- project grouping：workspace 路径稳定归组，未知 workspace 不丢 session，
  pinned 不在 project 内重复。
- approval presentation：bash/file/unknown tool 都得到短标题、主要对象与
  scope；raw args/gates 默认折叠。
- 前端 TypeScript build；后端 Go tests；根 `scripts/check.sh` 含 Web UI
  前端 test/build 与 webui Go module test。

### QA-27（真实共享环境）

使用全局 daemon/store `~/.local/share/agentrunner/` 与真实
`http://127.0.0.1:8788`，不隔离、不删除数据：

1. 首页：只出现一次任务导航；New task composer 可用；项目分组加载真实
   历史 session。
2. list/detail/deep link：从 project task 打开父 session；直接 hash 打开；
   reload 后保持；子 session 可从 Supervision 打开完整只读时间线。
3. 状态：至少核对 running / waiting approval / completed(or idle) / failed
   或 archived/missing metadata 的真实记录；页面不空白、无 crash overlay。
4. 审批：真实 pending approval 显示人类可读摘要，Details 折叠；本轮不替
   用户批准或拒绝。
5. review：Changes 打开真实 diff；无 workspace 时给可恢复提示而非空白。
6. restart：重启 Web UI（不破坏性 kill daemon），相同 deep link 与真实数据
   仍可见；console 无相关 error/warning。

截图、console/API 证据与 workspace diff 归档
`qa/runs/2026-07-09-QA27/`，session 与测试数据全部保留。

## 实施步骤

1. **INC-19.1 文档与 view model**：三层 delta、project grouping/
   approval presentation 纯函数与测试。完成标志：测试红转绿。
2. **INC-19.2 Codex shell**：品牌、侧栏、New task/Scheduled、project/task
   hierarchy、首页。完成标志：真实历史数据可导航。
3. **INC-19.3 task workbench**：安静 header、单一 timeline、内联审批、
   Changes、Supervision、responsive composer。完成标志：父/子/审批/diff
   主流程可用。
4. **INC-19.4 双闸门与收口**：自动检查、真实 QA-27、design QA 对照母版，
   三层/QA/GAPS/LOG 收口、工作纸归档。

## review 裁决

做三视角 review，不裁：

- 产品/可用性：Codex 模式忠实度、默认态安静、progressive disclosure。
- 契约/数据：所有 UI 状态均来自公开 journal/inspect/ps/diff 路径，历史与
  localStorage 不丢。
- 真实环境/可访问性：共享 store、deep link、reload/restart、键盘/focus、
  响应式与 console 健康。

P0/P1 全修；P2 若影响主 journey 同样阻塞，纯 P3 记入 design QA。

## 完成记录（2026-07-09）

- INC-19.1～19.4 全部完成；`scripts/check.sh` 纳入 Web UI Go/test/build。
- QA-27 连接共享 daemon/store PASS；真实 approval/team/diff/deep link/
  restart/console/响应式证据归档 `qa/runs/2026-07-09-QA27/`。
- Design QA 初轮发现同一成员因 revive 回执重复渲染（P1），按 child session
  去重并加纯函数测试后复验通过；最终 P0/P1/P2=0。
- 不变量未变；活文档 JOURNEYS/SPEC/DESIGN/QA/GAPS/LOG 已同步。本工作纸
  随增量关闭移入 archive，不再作为活决策源。
