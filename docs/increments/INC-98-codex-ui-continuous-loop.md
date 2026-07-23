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
- **98.3e ask_user design note**：structured answer 与兼容 composer answer 共用同一
  durable ask park，但 receipt 不同：表单走 `/answer`，普通 composer 被 loop 直接配对为
  `AskResolved{answered,answer}`，不会再落 `InputReceived`。前端 optimistic projection 因此必须
  同时以 `InputReceived.text` 和 `AskResolved.answer` 消费；兼容 composer 在 durable structured
  AskForm 可见时成功返回的同步 `status=delivered` receipt 也必须按本次 optimistic id 立即消费。
  真浏览器发现 journal poll
  可能在 React pending state commit 前先处理 AskResolved，单靠 event text reconciliation 仍会
  残留 `queued…` 假气泡直到 reload；AskForm context + 同步 receipt 是无竞态的主收敛点，journal matching 保留为
  reload/network fallback。Codex Default 明示 request_user_input 不可用；Plan 实窗生成
  `Asked 1 question` disclosure，但当批 app bridge 记录 `No answer provided`，随后 Alpha 作为
  普通 follow-up 继续。AgentRunner 对标不照抄该退化：保留 durable structured form、单选/
  多选/free-text/skip/多问与 reload。另实测 sessions list 只给 `waiting:input`，sidebar 无法区分
  ordinary idle 与 active ask，错误显示 Ready；backend 需新增 truthful attention projection，记 G48。
  capture driver 新增 Plan composer send 与 current-thread disclosure；动作均 OCR fail-closed，
  disclosure 取证后恢复 collapsed；当前 Codex 在 Plan request accepted 后已把 composer 恢复
  Full access，driver 不在等待卡上再做不安全的固定坐标 cleanup。
- **98.3f approval driver note**：Codex access menu 当前真实枚举为 Ask for approval /
  Approve for me / Full access / Custom。capture driver 新增 `--composer-access`，只在 New chat
  prompt 提交前选择并以 composer chip 复核；Ask 首行对 Vision 的 `Ask for` / `Askfor` 两种
  OCR 均 fail-closed。新增 `--thread-approval allow-once|deny`，先确认 thread-tail approval card
  的 exact action，再点击并确认 Allow/Deny 两个 action 一并离场；不触碰 dropdown 的 session/
  persistent 选项。approval card 可贴至窗口最底 1.5%，单独 `approval-tail` OCR region 避免把
  普通 thread 文本误当按钮。
- **98.3g approval truth design note**：shared-store 真 Gemini approve/deny/reload 与 child
  approval Gate B 均以真实浏览器跑通；child wait 从 fresh page 的 inspect tree 持久浮出，
  approve 精确路由到 child，child 在隔离 worktree 执行后 parent 自动收到结果。对照图同时
  暴露三处声明偏差：(1) Subagents 行只读 `report.status=waiting`，忽略更精确的
  `report.waiting.kind=approval`，故错标 Ready；(2) child approval 卡把 parent session 的
  workspace 当成执行位置，虽然同一份 parent inspect 已在 `delegations[].workspace` 给出精确
  child path/mode；(3) 1280px desktop 上 approval 自动打开 340px Environment 浮卡，遮住
  primary decision card。最小修订不改 runtime/event/schema：Subagents 以 typed wait 优先；
  SessionView 缓存既有 delegations 并把 child 的 path/mode 传给 ApprovalCard，isolated 时明示
  `Child worktree`；Environment 仅在 >1400px wide 自动强化，1280px 保留 header attention badge
  与完整 inline card。parent 在 sidebar 仍显示 Ready 属 session-list backend 缺失，单列 G49，
  不用 selected-thread local state 冒充全局修复。
  修后 production `b3ed334d-dirty-175653` 经 cache-bust 在既有 pending child session 复拍：
  fresh load、Environment open/close、Subagents status、真实 path/title、空 console logs 均通过；
  Codex reference 与无遮挡 AgentRunner 同状态合并图为 `QA88-98.3f-approval/38`。
- **98.3h error/access escalation design note**：两侧对同一个 harmless `bash exit 23`
  真执行后，均把 command failure 保留为可展开的 Worked/tool detail，并允许 agent 在正文解释；
  AgentRunner 额外保留 `stderr` 与 `Exit 23`，不把模型可处理的 tool error 错升级成 provider
  failure。另一个当前 Codex 差异更关键：New chat 从受限 posture 选择 `Full access` 会先展示
  `Turn on Full Access?` 风险确认，而 AgentRunner 的 `Ask to approve → Full access` 立即落本地
  state。最小修订复用现有 app-styled ConfirmModal，以同一 modal scroll region 分组列出 files、
  terminal、internet 三类授权范围，只拦截**从非 Full 升级到 Full**这条边；
  Cancel/Escape/outside 均保持原 posture 与 draft，Confirm 才持久化 `arwebui.lastAccess`，关闭后
  focus 回 access pill。Full→Full 不重复确认，Full→受限与受限↔受限仍保持单击可逆；不改
  runtime mode、spec、journal 或 backend。风险说明点名 commands、internet、全盘文件读写删除；
  confirm 使用既有 danger hierarchy。该小增量已有同产品 Dialog pattern，三视角 review 裁掉，
  以 access contract test + shared-browser Ask→Full Gate B 代替。
- **98.3i nested disclosure driver design note**：TH-10 已拿到 Codex 顶层
  `Worked for …` 展开证据，但真实 command/stderr/exit detail 还藏在第二层
  `Ran a command` disclosure。driver 新增 opt-in `--disclosure-nested VISIBLE_TEXT`，
  仅允许与 `--thread-disclosure` + `--disclosure-validate` 同用：先 OCR 定位并展开
  顶层，再从新截图 OCR 定位第二层，再用最终截图验证 detail 中的唯一 token；任何一层
  缺失都 fail-closed，不回退到猜测坐标。因 Codex 展开后会把 generic label（如
  `Ran a command`）改写成具体 action label（如 `Ran bash …`），driver 从 generic label
  提取稳定 action prefix，在新截图重定位；若上次异常退出留有展开态，先 inner-first 自愈归一。
  退出同样从新截图依次重定位并折叠 nested、outer，不复用展开前的陈旧坐标；默认单层行为保持不变。该增量只扩展 QA driver 与证据契约，
  不改变 AgentRunner product/runtime，不触及 DESIGN 不变量，规模不足以需要三视角 review。
- **98.3j compact shell detail design note**：同状态合并图 `61` 显示 Codex 的 nested
  command detail 直接在 disclosure 内给 concrete command + result；AgentRunner 已在外层 summary
  完整显示短单行 command，却在 detail 内再次画 `Shell` header 和同一 command，信息重复且把短失败
  拉成重卡片。沿用既有 tool `<details>`、Shell transcript、status 与 Copy pattern：≤160 字符且无换行的
  command 只在 summary 出现一次，detail 直接显示 stdout/stderr；多行或 >160 字符仍在 detail 保留完整
  command。移除无新增语义的 `Shell` header，把既有 Copy action 放到 status footer；失败 `Exit N`、
  Cancelled/Failed、完整 copy payload、长输出 240px 内滚动全部不变。无 destructive/risky state，不丢
  journal 数据；风险仅是隐藏 summary 已完整承载的重复文本，以短/长 command contract + 真 browser
  同 session 展开对照兜底。该小修复复用既有 disclosure/detail pattern，按 UI/UX review 裁掉三视角 review。
- **98.3k Markdown/math/theme design note**：TH-03 使用同一条真实回复 fixture 覆盖 heading、
  emphasis/strike、quote、unordered/ordered list、table、Go fence、inline/block math、link 与
  Mermaid；Codex accepted reference `12` 与 AgentRunner light screenshot `14` 合并为 `15` 后再裁决。
  两侧普通 Markdown、代码与 Mermaid 结构均成立，但 AgentRunner 仍把 `$…$`/`$$…$$` 原样露出，
  命中既有 G38；同时实测从 dark/system 切到 light 后，已 mount 的 Mermaid 仍保留 dark SVG，节点
  变黑且 label 近乎不可读。最小修订继续沿用既有 `react-markdown` pipeline：加入
  `remark-math + rehype-katex + KaTeX CSS/font`，不启用 `rehype-raw`，所以 raw HTML 禁止与
  Mermaid strict security red line 不变；inline 与 display math 都生成可访问的 KaTeX DOM。
  Mermaid 继续 lazy-load，只让 render effect 订阅产品 `theme`，显式 light/dark/system 切换后用
  fresh SVG 重绘；system 的实时 OS 变化另由 media query listener 驱动。无 backend/schema/journal
  改动；风险是 KaTeX bundle/font 增量与主题切换竞态，以 package/build asset 记录、math DOM、
  raw-HTML regression、Mermaid dark→light 与 system media tests、真实 shared-session 修后合并图兜底。
  Codex composer driver 同批修正两个 fail-closed 误判：请求 Full 且 root chip 已验证为 Full 时，
  access menu 只做 Escape 收回，不再依赖 checked row 的易漂 OCR；New chat starter 接受 heading 或
  starter card 两个独立视觉锚。未验证 root chip 时仍必须定位 row/modal，不能降级成猜坐标。该增量
  关闭 G38；不是 backend 缺口，不新增假 control。
- **98.3l Gemini thought/media design note**：补 TH-03 media 的双侧同 fixture 时，Gemini
  `IncludeThoughts=true` 返回的 `genai.Part{Thought:true,Text:…}` 被 provider 当前 `mapPart`
  当普通 `EventTextDelta`，最终 durable assistant message 在图片前永久显示内部 response-format
  推理；Codex reference 只显示最终 heading/images。该泄漏不是 Markdown renderer 问题，而是
  provider normalization 缺失的 backend gap G51。最小修订在 Gemini adapter 的 text 分支前
  丢弃 `part.Thought`：thought token 继续计入 usage，thinking budget/config 不变；tool call 自带的
  opaque `ThoughtSignature` 仍随 ToolCall extras 持久往返，不能误删；非-thought text/图片 Markdown
  原样进入 answer。Anthropic 已有“thinking deltas internal-only”同产品先例。用 mapPart 单测钉
  thought 不产生 user-visible stream event、普通 text 与 tool signature 不回归，再在同一 shared
  media session 的新 turn 或新 session 复拍。修后 standalone/link-wrapped external image 均需
  真 DOM、加载完成、同 viewport/light 与 Codex 合并验图；点击 linked image 只验证 target/href，
  不离开当前 QA tab。首次合并图另见我方 remote image 以 `420px` 高度撑大正文、约为 Codex
  `200px` 的两倍；继续复用现有 lightbox 放大能力，只把 inline `.md-img` cap 收敛到 `200px`，
  保持原图、alt/link、click-to-zoom 与窄屏 max-width 语义。无 schema/event/invariant 变更；本批
  关闭 G51，并把 TH-03 整行由 UNTESTED 改 PASS。
- **98.3m long-output/driver evidence note**：补 TH-02 缺失的 Codex long-output 同态，使用
  harmless Python command 真实输出 220 行、约 20.9KB，再只回复 completion marker。Codex
  completed thread 的 outer Worked 可展开，但 long `Ran a command` row 没有 nested chevron，
  点击 label 也不投 raw stdout；这是真实当前产品行为，不凭空补截图。AgentRunner 保留既有
  shared session 的完整 15,393-char stdout、240px inner scroll、full Copy 与零 body 横溢；能力
  强于 reference，不向下删除。合并图确认两侧都把长输出隔离在正文之外，我方仍可审计全部
  内容，故 TH-02 升 PASS。取证同时暴露非 Retina/低对比 disclosure label 会被 Vision OCR
  漏掉或合并词间空格；driver 仅在正常识别 miss 时 2x retry，并只对 folded 长 query（>=6）
  接受 joined-boundary match，短 query、region constraint 与 unknown state 仍 fail-closed；debug
  初始/validation frame 只在显式 env 下保存。无 backend/schema/invariant 变更。
- **98.3n Settings shell design note**：通过真实 profile menu 打开 Codex Settings，补 General /
  Appearance 双 tab 与 same-viewport 合并图；driver 新增语义 OCR `--settings` / read-only
  `--settings-tab`，capture 后 Escape 回原 thread，不更改任何 toggle。Codex 的通用入口稳定落
  General；AgentRunner 旧 `Settings` 组件默认 `initialSection="appearance"`，使普通 Settings
  入口直接跳主题页，与 rail 顺序、页面名和用户预期不符。最小修订只把无定向入口 default 改为
  General；从专用 deep link 传入 explicit initial section 的能力不变，Appearance/其已持久化设置
  也不迁移。General 保留 truthful daemon status/reset，Codex 的 account/billing/pets/browser/
  hosted integrations 属 INTENTIONAL 或既有 G43 等边界，不画假 row 填满页面。Desktop Done、
  Escape 与 mobile Back 均须回原 opener/focus；本批只在双侧 General/Appearance 与 close contract
  全部真测后升级 ST-01/ST-04，不用静态截图替代交互。真浏览器首次 Done 反证 activeElement 落
  `body`：Settings 从 sidebar menuitem 打开时保存的是即将 unmount 的 menu row。修订只在 opener
  位于 menu 时把 return target 提升到同一 `More options` trigger；⌘,、command palette、mobile
  sidebar 的既有 return target/fallback 不变。shared production 实测 Search autofocus、General
  `aria-current=true`，Done/Escape 均关闭 dialog 并回 `More options`；同 viewport 合并图通过，
  browser logs 为空。ST-01 升 PASS；ST-04 因 mobile Back 的当前双侧证据未齐继续 UNTESTED。
- **98.3o Settings secondary sections design note**：继续 ST-03，不把 General shell 通过外推到
  Keyboard shortcuts / Configuration / Worktrees / Archived。先扩展同一 fail-closed Settings
  driver，以左 rail 语义 OCR 只读切 tab、主区标题复核、capture 后 Escape 恢复原 thread；再对
  AgentRunner shared production 的对应 tab 做搜索、empty/populated、close/focus 与同 viewport
  comparison。只修被真交互反证的 UI；Codex 专有 setting 不补占位，若数据/API 缺失再单独登记
  backend gap。首轮 shared store 还反证 Worktrees 一次渲染 304 workspace / 641 session、scrollHeight
  49,883px；这不是 backend 缺口，按既有 sidebar progressive disclosure 收敛为首 40 个、显式
  `Show more` 分页展开，搜索仍对全量集合过滤。快捷键重绑/冲突校验/持久化、worktree registry /
  自动清理/逐项删除、session 永久删除则分别登记 backend 产品缺口，不画假 control。无 schema/API/
  invariant 预设变更。
- **98.3p Scheduled states design note**：转入 SC-02/03，先捕获当前 Codex thread 作为 restore
  锚，再从 Scheduled 主入口真实走 search/filter/create；所有 create 只到 validation/cancel 或用
  明确无副作用的专用 QA automation，AgentRunner 走 shared daemon/store 并永久保留 run/session/
  journal。empty/large/loading/error 不能用假 fixture 冒充；若 Codex 状态无法安全构造保持
  UNTESTED。先比较同状态与交互，再决定 UI 修复或 backend gap；不执行 delete/cleanup。
  实测 Codex search=`Weekly` 与 Paused=`cloc` 均经 OCR 动态定位、结果二次校验并恢复原 thread；
  `Create` 的四轮可逆 click/Enter/Space 校准未得到可验证 menu，`03..06` 明确拒收且未保留半工作
  driver。AgentRunner shared production 则完成大列表 search、Active/Finished、Create menu、one-time
  blank/filled validation 与 Cancel；未创建新 run。两侧同 viewport 合并图确认 search shell 无新的
  视觉缺陷，但 Codex 有真实 Paused series，而我方只有 Finished：不能改 label 冒充，登记 G55。
  SC-05 从 UNTESTED 改 GAP；SC-02/03/04 仍据实 UNTESTED。
- **98.3q Scheduled suggestions design note**：继续 SC-04，只测三张真实 suggestion 的 click→prefill/
  cadence→dismiss，不提交、不创建计划。Codex driver 仅在 OCR 命中 suggestion title 后点击，并在
  modal 内以可见 prompt/cadence 双锚验真；任何 modal 识别失败立即 Escape、拒收截图、移除半工作
  action。AgentRunner 用 production shared store 同样点三张 card 并 Cancel，断言没有新增 run/
  session。完成双侧同状态 comparison 后才决定 UI patch；若 Codex suggestion 需要外部 connector/
  account 而无法安全进入，保持 UNTESTED，不以文案猜能力。
  首次 Codex `Daily brief` 点击反证它不是 prefill/modal：单击即创建真实 automation，并立刻从
  Suggestions 移入 All、显示 `Next run in 12 hours`。本轮按数据保留纪律不删除/不暂停该 automation，
  立即停止另两张 Codex card，且完整移除会重复产生副作用的 driver action。AgentRunner 三张 card
  则逐一打开同一 Schedule modal，分别精确预填 prompt + `0 8 * * 1-5`、`0 16 * * 5`、
  `0 */6 * * *`，每次 Close，零 submit/零新 session。此处保留我方显式确认是安全优势，不向
  Codex 的 one-click side effect 对齐。实窗还反证 pointer 打开后 Close 把焦点丢到 `BODY`；只在
  suggestion pointerdown 上显式 focus 当前 card，复用 shared Modal 的既有 restore contract，keyboard
  路径不变。SC-04 以双侧真实差异证据判 PASS。
- **98.3r Scheduled detail design note**：继续 SC-06，只从已存在并保留的真实 series 进入详情；
  Codex 使用本轮新 `Daily brief`，AgentRunner 使用既有 shared rhythmic session。覆盖 row click、
  detail/history/next-run、back/deep-link reload 与只读 action affordance；不触发 Run now/Edit/Delete/
  Pause/Retry/Cancel。capture driver 必须以 row title OCR 动态定位，进入后用详情独有 heading/fact
  二次校验，再恢复原 thread；若 Codex row click 本身触发执行或缺乏稳定详情则立即停止并拒收。
  先做同 viewport combined comparison，只有真实交互反证才改 UI；需要 series-level backend 数据时
  追加 G55 或新 gap，不用 run/session 猜历史。
  实测 Codex row click 打开同页 split detail，显示 prompt、project/model/reasoning、frequency/notification
  与 pause/close；AgentRunner row click 打开可 deep-link/reload/back 的完整 iteration history，但没有
  series config/edit 投影，登记 G56。点击 terminal `Run details` 还把 inspect 全量 JSON 直接铺进 modal；
  这不是 backend 缺口，改为复用既有 structured Run details（Overview/Usage/Activity，raw 折到 disclosure）。
  SC-06 因 G56 判 GAP，不因 history/deep-link 已通过而提前判绿。
- **98.3s Scheduled create/restart design note**：继续 SC-03/07，在 shared production 用 UI 创建
  唯一专用 repeating fixture：prompt 只回复固定 marker、禁止工具，blank workspace 生成 retained
  scratch，interval 设 168h，driver `max_iterations: 2`，因此只立即跑一次并最多七天后再跑一次。
  自本批起视觉基线固定为双方真实 `1280×800` viewport：Codex 主窗口先经 AX 安全缩放、交互与
  截图完成后恢复原 geometry；AgentRunner Browser 也设置同一 viewport 并 reload。证据图归一到
  `1280×800`，高分辨率全窗图只作归档，不参与主要视觉裁决，禁止把大窗口截图事后缩小冒充布局
  重排。每次裁决前必须把同 state 的 Codex/AgentRunner 图并排合并后再验。
  实机已验证 Codex `1840×1353 → 1280×800 → 1840×1353` 可逆，输出像素严格为
  `1280×800`；AgentRunner 的 `innerWidth/innerHeight/devicePixelRatio` 为 `1280/800/1`，并已保存
  Scheduled 同 state 双图与 `2560×800` combined evidence。低分辨率首轮明确暴露 shared history
  密度、series config 投影与 suggestions 可见性差异，后续据此继续 SC-03/07，而不再沿用大窗裁决。
  创建前后记录 `/api/runs` + `/api/sessions`、modal validation、route、journal/events；普通 daemon
  restart 前后分别记录 cadence/nextRunAt/status，验证不重复立即 tick、不丢 next run。fixture/session/
  workspace/journal 永久保留，不 pause/cancel/close/delete/cleanup。若 Gemini/provider 当前不可用，
  据实保留 failure 但仍验证 schedule persistence；不把 provider failure 当 scheduler bug。
  实测创建 `20260723-043024-qa88-schedule-restart-6d2120bc409214e4` 后首轮无工具完成，timer 指向
  `2026-07-29T21:30:25-07:00`；普通 `SIGTERM` deploy/restart 后 cadence/nextRun/status 保留，journal
  只有一次 `series_iteration(n=1)`，仅出现预期 timer cancel/re-arm，没有重复 tick。但创建流程落到
  process-local `#run:run1`，Web UI restart 后该 route 只剩 `waiting for output…`。修复契约：drive
  创建后短轮询 `/api/runs` 的 daemon-assigned `sessionId`，一旦出现就刷新 sessions 并导航 durable
  `#<sessionId>`；仅在 session id 未及时出现时保留 transient run fallback。新增真实 modal→API→route
  回归，禁止 scheduled creation 再把 process-local run id 当 durable deep link。
- **98.3t Scheduled terminal semantics design note**：`98.3s` 修复部署后以第二个 retained
  `max_iterations: 1` fixture 真验创建 route，直接进入 durable `#20260723-…`，reload 后 iteration
  history 保留且无 `waiting for output…`。同一张 `1280×800` 证据又显示 interval series 被标成
  `Best-of-N winner` / `Apply winner`；journal 的 `best_iter` 对普通 series 代表可 promote 的 selected
  iteration，不代表并行竞赛。frontend fold 必须保留 `series_started.kind`：仅 `best_of_n/parallel`
  使用 winner/best，其他 series 使用 selected iteration/selected，按钮仍调用同一安全 promote API。
  新回归同时钉 interval 与 best-of-N 文案，不改 runtime selection/promotion 语义。
  clean production `6ccd4bf5-214730` 复拍通过：interval 页面同时出现 `selected #1`、
  `Selected iteration: #1`、`Apply selected iteration`，不再包含 `Best-of-N winner`，durable hash 与
  reload history 继续保真。第二个 single-iteration fixture/workspace/journal 同样永久保留。

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
