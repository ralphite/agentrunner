# INC-98 Codex UI 持续对标循环

> 状态：进行中。该工作纸驻留到覆盖矩阵全部具备当前证据；每个可合并批次独立
> commit/push，不等整轮结束。用户要求长期运行，不能以一次截图或一次修复宣告完成。

## 动机与 journey 锚

修订 UJ-24 Web UI 驾驶 AgentRunner：此前 QA 多按单个增量取证，能证明修过的路径，
不能回答“Codex 与 AgentRunner 的所有页面、状态、功能点还有哪些没测”。QA-87 首次把
真实 Codex 窗口与真实 AgentRunner 同尺寸对比，并修复 Environment desktop reflow；
INC-98 将该方法固化为持续循环：

1. 枚举 Codex Desktop 与 AgentRunner 的全部主界面、子界面、popover/modal、状态与动作；
2. 每行记录 Codex 实窗、AgentRunner 真实 shared-store 页面、自动化/DOM 证据和最近复测时间；
3. 可由现有前端契约实现的差异直接修复；需要 backend/schema/产品语义的差异进入 GAPS；
4. Codex-only 且不符合 AgentRunner journey 的表面明确记“非目标”，不画假入口；
5. 新增页面/状态后只追加矩阵行，不删除历史证据与已关闭缺口。

### UI/UX design note

- **沿用模式**：以 Codex 当前实窗为视觉基线，以 AgentRunner 现有 token、组件、状态语义
  和 UJ-24 为产品边界；修复时复用最近的本产品控件，不另造平行模式。
- **提案**：`CODEX-PARITY.md` 增设 executable UI coverage matrix；状态只允许
  `UNTESTED / BLOCKED / GAP / PASS / INTENTIONAL`，每个 PASS 必须同时有 Codex 与
  AgentRunner 的当前截图/交互证据或点名可执行 QA。
- **风险态**：截图不等于功能验证；每个动作还要验证 focus、URL/reload、loading/error/
  empty/running/completed 等状态。截图不能证明 VoiceOver/WCAG，另记验证边界。
- **数据处理**：默认只读导航；不 Send/Approve/Deny/Archive/Remove/Commit/Push。
  必须产生状态时使用共享真实 QA session 并保留；破坏性状态先单独确认或用已有数据。
- **未决问题**：Codex Electron AX tree 当前只暴露顶层 group，系统坐标 click 可造成
  `System Events` 阻塞；首批先把 driver 改为 `lsappinfo + CoreGraphics/CGEvent`，不依赖
  AX element click。未来若 Codex 增加可用 accessibility tree，再切到语义定位。
- **98.2 裁决**：Codex palette 是窄宽、较低起点的快速导航面；AgentRunner
  保留既有九个真实 `⌘1..⌘9` 快捷语义，但把 Commands 移到 attention
  overflow 之前，不让大 shared store 把命令挤出首个 viewport；desktop 收到
  560px/15vh，mobile 继续使用既有 12px inset。正文搜索需 backend，记 G44。
- **98.2b sidebar design note**：沿用既有 220–480px pointer/keyboard resize、整行
  hover/focus 和同源 context menu；真实 Codex desktop rail 约 337px，我方旧
  default 260px 在大 shared store 中使 project/session 名过早截断，改为 320px，
  与既有 mobile drawer 上限相同；已持久的用户宽度不迁移、不覆盖，仍可随时
  拖回 220px。不增加说明文字或新控件，不改 session/project 数据；主画布
  减少 60px 是可逆取舍，900px 以下仍走原 mobile drawer。会话菜单保留产品
  已定的四项轻表面；不照搬 Codex 的 raw id/deeplink/path，精确 Continue 仍在消息级。
- **98.2c New session design note**：沿用现有四张 starter card、icon/token、单一
  Composer 与“显式 Send 才创建 session”的契约；真实 Codex 当前行为是 card click
  后隐藏 cards、只写入 `Explore/Build/Review/Fix` 短意图，并在 composer 上方显示四条
  可继续点选的具体 suggestion。我方旧行为直接写入长 prompt 且保留 cards，信息层级和
  可逆性均有可见 drift。改为受控两阶段：card click 只选择 intent，suggestion click
  才显式替换为具体 prompt，清空 draft 后恢复 cards；任何自动发送、session 创建、
  project/worktree/access/model 状态重置都禁止。复用 Home/Composer 现有样式与 draft
  入口，不新增 route/modal/backend。Codex 的 `No environment/Create local environment`
  与 Cloud workspace 是环境生命周期能力，不画假控件，引用 G11；本批只判定现有
  Local/New worktree/Branch 子路径通过。风险集中在用户 draft 被隐式覆盖、mobile
  composer 被挤出 viewport 与 focus 丢失，分别以仅显式 click 替换、同一 composer
  不 remount、390×844/短高视口及 Escape/focus 回归约束。
- **98.2d model/access design note**：同逻辑 1952×1465 真机对照确认两侧均采用
  `Model / Effort / Advanced` 分层；Codex 另有会改变服务 tier 的 `Speed: Fast` 与
  模型相关的 `Effort: Ultra`。AgentRunner 当前 `Speed` 子页只有唯一 `Standard`，既
  不能选择也不改变 runtime，是虚假 affordance；在 backend/provider contract 明确
  service tier、计费/额度、fallback、可用性与 journal 可观测性之前先移除该 root，
  不增加假 `Fast`。`Ultra` 也不凭空指定跨 Gemini/Anthropic 的预算；高级 exact
  thinking budget 继续满足 power-user 调节，preset 缺口进入 GAPS。Access 的
  Full/Ask/auto-accept/Plan 与 Codex 常用权限姿态语义已覆盖；Codex
  `Custom (config.toml)` 对应本产品 agent/spec，而非再造第二套配置入口。
- **98.2e input/attachment evidence note**：Computer Use 明确禁止控制宿主 Codex app，
  因而继续扩展已验证的 CoreGraphics/Vision driver，不借不可审计的固定全屏坐标。
  `--composer-text` 只向未发送 New chat draft 粘贴指定文本，以独立可见短串二次
  validation，截图后 `Cmd+A/Delete` 并验证 placeholder 恢复；用于同态 CJK/multiline/
  overflow，不冒充真实 IME composition。Add→Files and folders 已确认能打开原生 Open
  sheet，但宿主安全边界与 panel service 使自动选择/移除尚无可靠语义路径；所有校准图
  拒收，driver 不落半成品 `--attach-file`，NS-06 保持 UNTESTED。后续只用非敏感 fixture，
  禁止 Send；oversize/error 不触碰用户数据。同逻辑 1952×1465、同一 20 行中英混排 draft
  对照显示 Codex 可见约 18 行，而 AgentRunner 受 `180px` CSS 上限只见约 8 行；桌面长
  prompt 的校对上下文不足。沿用现有 composer，不改发送/持久化语义：`>=901px` 时把
  textarea 上限放宽为 `min(320px, 38dvh)`，JS autosize 同步封顶 `320px`；窄屏仍保持
  `180px`，避免移动端 composer 挤出 viewport。以同 viewport/state 拼图复验，不把这项
  可见修复误写成真实 IME 已通过。
- **98.2f Goal/Plan/Automation evidence note**：当前 Codex Add root 实窗为 Files and
  folders / Attach Google Chrome / Work in a project / Goal / Plan mode / plugin artifacts；
  AgentRunner 不复制浏览器绑定或 artifact plugin，占位能力仍禁止。`Goal` 只打开 launcher
  后 Escape，不创建 goal/session；`Plan mode` 只做可逆 on→off 并验证恢复 Full access。
  AgentRunner 的 Loop/Best-of-N/background/Agent→YAML 来自 UJ-14/18/22 的 Automation
  语义，不因 Codex 当前 root 不同而删除；但每个 launcher 都必须证明显式 Start 前无
  session、无 Send，close/Escape 回 Add opener，启动后的 shared session/data 永久保留。
  同态拼图确认两项可见 drift：我方 Goal 另叠一张三字段 card，造成双 composer；Plan
  只替换 access pill、仍显示通用 placeholder，且 Add row 不能原路关闭。修订为：Goal
  直接复用唯一 composer，改 placeholder 并显示 Goal chip，verifier/max rounds 收进 chip
  popover，Send 才创建/attach；Plan Add row 可逆 on/off、off 恢复进入 Plan 前的 access，
  Plan 时改为任务规划 placeholder。因我方 backend 把 Plan 编码为只读 access posture，
  pill 继续诚实显示 `Plan · read-only`，不照抄 Codex 同屏 `Full access + Plan` 的双语义。
- **98.2g responsive/theme evidence note**：用真实 New chat 做 Retina 2× Codex 与
  AgentRunner 逻辑 1952×1465 clean-state 合并比较，另覆盖 1840×1000、1280×720、
  390×844 的 light/dark home、mobile drawer 与 Appearance；所有 state 均以 UI 切换并
  reload，不能靠 DOM 强写主题。实测发现显式 dark 只在 `main.tsx` module 执行时恢复，
  在 system-light 设备存在首 paint 闪白窗口；把极小 `theme-init.js` 作为 head 中的
  parser-blocking boot restore，并让 `theme-color` 跟随 light/dark/system。比较还暴露
  capture driver 的 Plan 是 sticky preference，旧 cleanup 只点 New chat 并未关闭；driver
  现从 Add 的 `Turn plan mode off` 真正恢复并验证 `Turn plan mode on`，避免后续 baseline
  被污染。CSS build 同时暴露旧注释内 `*/` 提前闭合，已修正，不再吞掉后续 token。
- **98.2h Send/failure evidence note**：`--composer-send` 只在显式参数下提交 New chat，
  draft 与 thread 各经独立 Vision OCR，创建的 Codex thread 永久保留；thread 首屏可能
  在 worktree setup 后才出现，driver 以最多 15 次一秒轮询 fail-closed，不用固定 sleep
  假定成功。双侧用真实 `sleep 8` 捕获 submitting/running/Stop/completed/Worked，并以逻辑
  1952×1465 合并验图。AgentRunner 用不存在 model id 真实得到 `provider_invalid/model not
  found`，再经 UI 换回 Gemini Flash、Retry 成功，旧失败原位折叠；由此发现通用文案误导
  用户“缩短 conversation”，修为该子型专用“selected model unavailable / choose another
  model, then retry”。Codex 的 failure/retry 尚无安全可控同态路径，NS-10 保持 UNTESTED。
- **98.3a thread/action design note**：沿用现有“中间消息 hover/focus 才显动作、最终
  assistant answer 常驻动作行”、`Worked` 两级 disclosure、artifact `Open in` 与 Changes
  file-row Review；不另造第二套 message toolbar。实窗对照显示 Codex 的分支/续接动作是
  斜向外箭头，我方 `GitFork` 图形更像 agent tree，和“从此消息打开独立 session”的直接
  动作不一致；只把图标换成同一 Phosphor 库的 `ArrowUpRight`，label/tooltip/API/fork
  provenance 全不变。Codex 同排的 👍/👎 不能先画假按钮：AgentRunner 尚无 feedback
  event/schema、target item identity、持久化/导出、隐私边界与失败回执，登记 G46，backend
  契约落地前保持 UI 安静。真实 QA 必须分别验证 Copy 内容、Worked/tool 展开、human 前切
  draft、final assistant 后切、artifact URL、file-scoped Review 与 parent 不变；所有新 child
  session/workspace/journal 保留，不自动 Send、Download、Undo 或提交。
- **98.3b tool-state design note**：继续复用 `Worked → tool summary → detail` 两级
  disclosure，不把每次失败升级成独立 error banner；真实 `bash exit 7` 应在展开 detail
  内同时保留 command、stdout/stderr 与 `Exit 7`，最终 assistant prose 仍解释恢复结果。
  当前 `Copy command and result` 只复制 command/stdout，遗漏唯一能证明失败的 exit code，
  与按钮 label 和屏幕可见内容冲突。最小修订只在非成功状态追加可见的 `Exit N`、
  `Cancelled` 或 `Failed`；成功复制格式保持不变，避免给既有脚本增加噪音。复制仍只落本机
  clipboard，不改 journal/tool result；long output 继续使用已有 240px 内部滚动与 20k
  bounded projection，不能把整个 timeline 撑高或伪装未截断内容。
- **98.3c queue/steer design note**：运行中消息只有一个 truthful projection：`steer`
  在 daemon journal 落 `input_received` 前用 timeline 的 `steering…` optimistic bubble；
  `queue` 一旦 `send` receipt 成功便已进入 durable queue，应立刻切换成唯一的可撤回
  Queued card，不能同时保留 timeline `queued…` 副本。实测旧实现把 queue optimistic bubble
  一直等到未来 `input_received` 才移除，因而正常排队时重复显示；Withdraw 后永远不会有
  `input_received`，更留下直到 reload 的幽灵消息。最小修复不改 daemon/order/unqueue API：
  queue receipt 成功后前端立即刷新 `/queue` 并移除对应 optimistic bubble；steer 仍等真实
  journal receipt，保留其“正在注入当前 turn”的反馈。Queued card 顺序继续严格使用 backend
  返回顺序，不增加没有 domain contract 的拖拽重排。为取得 Codex 同态 running 交互，capture
  driver 新增白名单 `--thread-composer-send`：只操作当前已打开 thread 的 composer，支持
  Enter/Cmd+Enter，提交前后各以 Vision OCR fail-closed；不搜索历史、不切项目、不关闭 thread，
  真实创建的 follow-up 永久保留。
- **98.3d stop/recovery design note**：用户点击 Stop 是一个已确认、journal 有
  `limit_exceeded{interrupted}` + final barrier + waiting 的 durable terminal，不是 host crash。
  thread 内保留 `Stopped — you interrupted this turn` 与 tool partial output；topbar 提供 Retry，
  composer 继续可用，不再叠 `Session needs recovery / Resume`。只有真正 `stranded`（含 crash
  recovery path）才显示 Resume。status matcher 必须只把精确 durable `interrupted` 收敛为中性
  `Stopped`；`interrupted_by_crash` 等含 crash 复合原因仍走异常恢复，不能用宽泛 substring
  吞掉。Codex 实窗同态显示 `You stopped after Ns` 并保留 PARTIAL-1..N；其 background process
  是否继续是 Codex 自身行为，不反向削弱 AgentRunner 的“interrupt 必须取消进程组”不变量。

## Spec delta

- `JOURNEYS.md` UJ-24：增加“全部可见 surface/state 有持续 evidence matrix”的验收责任。
- `SPEC.md` Web UI 产品面/交互语义：增加 `CODEX-PARITY §7 + QA-88` 锚；不把未测写成齐平。
- `DESIGN.md`：首批不改产品架构；仅补 QA capture/driver 的非产品约束。未来矩阵行触及
  产品语义时，必须另写对应 Design delta；触及不变量时走 PROCESS §4。
- `QA.md`：新增 QA-88 持续循环协议，规定 capture → inspect → interact → recapture →
  compare → fix/gap → shared real-env regression → retain evidence。
- `GAPS.md`：新增 G42“Codex UI 全表面持续覆盖不足”；产品能力缺口优先引用既有 G 条目，
  新 backend 缺口在取证后逐项新增，不把“尚未测”误写成“功能缺失”。

## 验收

### A 闸

- `qa/capture-codex-ui.sh` 不再依赖可能阻塞的 System Events 来发现 PID；
- driver 支持按主窗口相对坐标做可逆、白名单化导航和截图，未知 target fail-closed；
- shell syntax 与平台无关 contract test 钉 target 表、非法参数、禁止 System Events
  回退与恢复 trap；PID/window 选择由本机真实 Codex current/palette 双捕获钉住；
- `./scripts/check.sh` 全绿。

### B 闸：QA-88，共享真实环境

1. 真实 Codex Desktop 捕获 current/command palette/至少五个 sidebar 主入口；逐张打开验图；
2. 每个入口只做只读导航，返回原任务后同一 running goal/thread 不丢；
3. 真实 AgentRunner `http://127.0.0.1:8809/` + `~/.local/share/agentrunner/`
   对应主面逐项导航，desktop/mobile/light/dark 由矩阵分批覆盖；
4. 每批保存截图、DOM geometry、browser logs、health、events/workspace diff 到
   `qa/runs/<date>-QA88-*/`，不清理共享数据；
5. CODEX-PARITY §7 的每个状态变化与证据目录同 commit 更新。

## 实施步骤

1. **INC-98.1 基础设施与总表**：修复 capture PID 发现死锁；新增白名单 navigation
   driver；建立 §7 全表面矩阵、G42、QA-88；真实捕获 sidebar 主入口并 push。
2. **INC-98.2 Global shell + New session**：sidebar/search/pinned/projects/user menu 与
   new-session composer 全状态对标，修前端差异、登记 backend gaps。
3. **INC-98.3 Thread + composer**：消息/tool/artifact/running/queue/ask/approval/error/
   recovery/continuation 与输入附件/access/model/add menu 全状态。
4. **INC-98.4 Environment + Changes**：goal/agents/attention/worktree 与 diff scopes、
   large/binary/untracked/commit/apply/remove、desktop/mobile。
5. **INC-98.5 Scheduled + Settings**：list/create/edit/retry/status 与所有 settings pages、
   theme/shortcut/archived/config/worktree。
6. **INC-98.6 Codex-only surfaces**：Pull Requests/Sites/Plugins/Profile/Voice/Pets 逐项判定
   `GAP` 或 `INTENTIONAL`；需要 backend 的条目进入 GAPS 并给最小 journey/Design delta。
7. **INC-98.7 Accessibility/resilience sweep**：keyboard/focus/target size/contrast/reflow/
   zoom/reduced motion/deep-link/restart/legacy data；明确 VoiceOver 未覆盖面。

## review 裁决

98.1 是只读 QA 基础设施与审计文档，小增量裁掉三视角 review；保留 shell contract、
真实 Codex 实窗和 shared-store browser gate。后续每个用户可见修复先做 UI/UX review；
涉及 backend/session/storage/navigation 的批次必须走 real-environment risky QA；触及不变量
单独 review，不因“持续 loop”而放宽。
