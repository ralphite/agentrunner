# INC-79 UI polish 全面审计登记簿(QA-0719,用户裁决"全部找到、全部解决")

**性质**:审计件 + 消化登记簿。驻留至全部条目勾销,随后归档
`archive/increments/`。**勾销纪律**:每项 = 样式落地 + 出
`qa/tw-class-baseline.txt` + 本文勾 ✅;每轮至少消化一个分组;
新增 kebab 类无样式即 fail(lint-tw-classes 既有硬门)。

## 为什么此前 QA 没抓到(诚实诊断,流程修正已生效)

1. **方法盲区**:各轮验证"修过的东西 + 驱动过的场景",从未做
   全表面×全状态视觉地毯;非稳态(排队中/提问中/出错/加载骨架/
   展开明细)几乎从未进入截图。
2. **工具在手没当回事**:lint-tw-classes 早已点名 163 个无样式类,
   被当"backlog 告警"而非"审计发现"。本文即把它升级为登记簿。
3. **话术错误**:随 Routine prompt 报"parity %"没有任何测量基础,
   已停用;只报"勾销 N/累计 M"。

判据(精确化,2026-07-19):元素 className 中**没有任何一个** token
(kebab 或 utility)在编译产物 CSS 里有规则 → 真裸(131 处);
kebab 无规则但同元素带 utility → 语义锚,良性(≈54 类,已白名单化
处理见文末)。

## P0 · 交互控件/输入裸奔(功能能用但没有正式界面)

- [x] queued 消息块(SessionView `queued-*` 4 类)——已修
  (c56a621,footer 卡系+单行 clamp+Withdraw ghost)。
- [x] AskForm 自由文本输入 `ask-free`(AskForm.tsx:136):裸 input,
  无边框/焦点态,与 composer 输入不同族。
- [x] turn-error 卡按钮 `turn-error-toggle`(SessionView.tsx:950)、
  `turn-error-action`(:960):错误卡的 Technical details 开关与
  Retry 按钮零样式。
- [x] terminal-alert 按钮 `terminal-alert-action`(SessionView.tsx:1017)
  与 `tam-label`(:982,:1013 goal 元信息标签)。
- [x] ENVIRONMENT rail worktree 按钮组 `env-wt-actions/env-wt-action/
  env-wt-danger`(SupervisionPanel.tsx:822-856)——批次 1 落地
  (danger 红边、动作组布局)。
- [x] Scheduled `scheduled-create`(Scheduled.tsx:523,Create 按钮
  ghost 化曾有专样式,现无规则)。

## P1 · 高频可见明细/状态面裸奔

- [x] **Timeline 工具明细全家**(最大簇,Timeline.tsx 344-582,
  ~45 处):`cx-td-head/ic/path/meta/pattern/link/prompt/sub/tag/
  err/partial`、`cx-grep-files/file/fname/hit/ln/tx`、
  `cx-dl/sign/text/more`(mini-diff)、`cx-path-list/path`。
  症状:read/edit/grep/glob/spawn 展开明细无 mono/无色彩层次/无
  截断控制。修法:mono 路径、dim meta、grep 行号列、mini-diff
  ± 着色,与 dsk shell 块同族。
- [x] Timeline 活动行 `cx-activity-row/label`(:952-954)与
  `act-caret/act-cat`(:717-718):Worked 折叠内的活动分类行。
- [x] markdown 表格 `cx-table`(Markdown.tsx:264):**assistant 回答
  里的表格零样式**(用户实机截图 #1 diff 表格烂即含此因)。
- [x] ATTENTION 圆点 `attention-dot`(SupervisionPanel.tsx:142-166
  ×4):注意力行无状态色点。
- [x] 加载骨架:`tl-skeleton/tl-skel-row`(Timeline.tsx:1152-1160)、
  `changes-outcome-skel`(ChangesOutcome.tsx:436)——加载期白板。
- [x] 滚底浮钮 `tl-jump`(Timeline.tsx:1233)。
- [x] ENVIRONMENT 行标签 `env-row-label`(×5)与路径明细
  `env-detail/env-detail-path/env-path/env-path-copy`。
- [x] goal 文案 `goal-copy/goal-settled-copy`(SupervisionPanel
  321-364)与 `msg-goal-verdict`(Timeline.tsx:209)、
  `cx-goal-note`(:913)、`gbar-checks`(SessionView.tsx:1224)。
- [x] shell 块状态徽标 `shell-status`(Timeline.tsx:289)。
(以上批次 1 于 2026-07-19 落地:49 类出 baseline,163→110。)

## P2 · 次级/低频面

- [x] Modals 等宽字 `code`(×7)与行布局 `row-flex`(×4)、
  `confirm-copy`(:182)、`fork-empty`(:664)——fork/rewind/confirm
  模态内部排版。
- [x] approval Details 内部 `approval-gates/approval-readonly`
  (ApprovalCard.tsx:88,97)。
- [x] supervision 折叠明细 `supervision-details/label/caret`
  (SupervisionPanel.tsx:475-478)。
- [x] Scheduled 杂项 `sched-project-chip`(:712)`sched-pinned`(:719)。
- [x] Settings:`rs-archive-empty`(×2)`rs-archive-note`
  `rs-sc-grouptitle`;快捷键面板 `sc-group`(Shortcuts.tsx:45)。
- [x] `tl-notfound-id`(NotFound.tsx:14)`pop-section`(Popover.tsx:253)
  `imgnote`(Timeline.tsx:894,1225)。

**批次 2(2026-07-19)落地后里程碑:全代码库真裸元素清零**——
精确判据复扫 0 命中;baseline 重组为 anchor-only 注释登记(82 类,
逐一核对为语义锚),新增无样式类仍 fail。后续为主观 polish 打磨
(对金标逐屏比对),不再有"零规则"级问题。

## 良性结构锚(不是债,处理=白名单)

`turn` `worked` `worked-caret` `sys` `project-group` `proj-folder`
`pinned-section/projects-section/sessions-section`(sidebar-section
覆盖)等纯包装节点:布局由子元素承担,无视觉症状。处理:baseline
留存但在文件内注释分区标注"anchor-only",不计入待修。逐项核对后
挪入注释区。

## 待追查(验收轮遗留)

- [ ] Scheduled @390×844 深色:document 横向溢出 3px(run 29675251366
  验收轮实测;元素级探针被 SVG path pre-clip 几何干扰,下轮换
  "逐容器 scrollWidth" 探针定位)。

## 非样式类(同场审计发现,另行跟踪)

- worktree hop 无限堆叠(LOG 已记,产品裁决挂起)。
- last-turn 含 turn 后外部写入(QA-76 S1 observation,产品裁决挂起)。
- G39 child 不可见审批(INC 待立)。

## 验收

每个分组勾销时:①lint-tw-classes 绿且 baseline 行数下降;②远程
env 截图该表面(含深色+390 视口)人工核看;③LOG 记勾销条目。
全清标准:baseline 只剩 anchor-only 注释区,131 真裸元素归零。
