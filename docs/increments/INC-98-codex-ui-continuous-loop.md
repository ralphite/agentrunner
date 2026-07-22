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
