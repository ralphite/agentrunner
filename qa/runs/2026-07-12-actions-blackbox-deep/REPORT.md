# AgentRunner GitHub Actions + 深度黑盒 QA 报告

测试时间：2026-07-12 至 2026-07-13（America/Los_Angeles）
本地深测版本：`3d6b4712391cc2d88dbbd7c372d4475e204c55b8`
Actions 基线版本：`7f615eb92021fe79de1a3126922650f899e3376a`
报告目录：`qa/runs/2026-07-12-actions-blackbox-deep/`

## 1. 测试步骤与流程

1. 同步 `origin/main` 并 fast-forward；本地深测最终基于 `3d6b471`，Actions 基线 run 基于 `7f615eb`。
2. 触发 GitHub Actions：
   - `qa-blackbox`：run `29214275371`，成功，artifact 已下载到 `actions/qa-blackbox-artifacts/blackbox-run/`。
   - `phone-webui`：run `29214275423`，进入 keep-alive；本机无法解析 `agentrunner-phone`，见 `metadata/phone-webui-connectivity.txt`。
3. 因 tailnet 部署在本机不可达，使用本地最新 `origin/main` 启动 `arwebui`，通过浏览器继续做深度黑盒测试。
4. 启动测试侧并发子 Agent：
   - 4 个前置探索 Agent：梳理 journeys/spec/历史风险/Goal/Loop/Multi-agent prompt/Web UI 表面。
   - 2 个审计 Agent：一个专看截图 UI/UX，一个专看 runtime events。
   - 另有 1 个审计 Agent 因线程上限未能启动。
5. 在被测 AgentRunner 内执行复杂任务：
   - A：10 层 runtime/call/message，Ask-to-approve 下多次写文件/编辑文件。
   - B：长输出后 Queue follow-up。
   - C：Steer/Withdraw/再次 Steer。
   - D：被测产品内启动 3 个 worker 子 agent 并行调查。
   - E：Goal mode。
   - F：Loop mode，`every 30s`，`max_iterations=2`。
6. 覆盖 Scheduled、Settings、Git settings、desktop 与 mobile viewport。
7. 导出 `ar events`、Actions 元数据、截图和连通性记录；未 close/delete/cleanup 测试会话。

## 2. 测试标准

- 黑盒优先：所有主要发现来自浏览器 UI、Actions artifact、CLI 可观察 events，不以内实现为前提。
- 真实环境：使用共享 daemon/store，不隔离 `HOME`/`XDG_DATA_HOME`，测试数据保留。
- 行为正确性：
  - Session/task/run 状态必须与 runtime events 一致。
  - Approval 只阻断真正有副作用的操作；pending 状态必须明显。
  - Queue/Steer/Withdraw 的语义必须和 UI 呈现一致。
  - Goal/Loop 判定必须验证实际行为，而不是只信模型自证。
  - Multi-agent 子任务必须能访问父任务声明的 workspace context。
- UX 标准：
  - 不暴露 raw event/json 作为主体验。
  - 文案、按钮、状态、tooltip 和 mobile layout 必须可读、可点、不可溢出。
  - 权限范围、安全边界、未接线功能必须明确。

## 3. 主要结论

`qa-blackbox` Actions 基线通过且 `findings.json` 为空，但覆盖面明显不足；本次深度黑盒测试发现 Goal、Loop、Multi-agent、Approval、Steer、Scheduled、移动端布局、文案和部署可达性等多类问题。Queue follow-up 的基本路径表现正常。

## 4. 问题清单（按重要程度排序）

| 优先级 | 问题 | 截图/证据 | 正常预期 Behavior |
|---|---|---|---|
| P0 | `phone-webui` GitHub Actions 部署启动后，本机无法通过 `http://agentrunner-phone:8788` 访问；DNS 解析失败，导致无法直接用 Actions 部署环境做浏览器深测。 | `metadata/actions-phone-webui.json`, `metadata/phone-webui-connectivity.txt` | Actions 部署应提供测试者可访问的 URL，或在 summary/artifact 中明确 tailnet 前提、访问方法和健康检查结果。 |
| P0 | Goal mode 误判成功：用户要求“不使用任何工具，只回答 GOAL-SATISFIED”，实际事件里出现 `goal_complete` tool call，且 UI 判定 Goal achieved。 | [local-29-goal-started.png](screenshots/local-29-goal-started.png), `events/events-goal-E.txt` | Goal 验证应检查事件轨迹；若要求不使用工具，应把内部 completion 与用户可见工具清楚隔离，或判定失败。 |
| P0 | Multi-agent 子 agent workspace 缺失父 repo 内容；3 个 worker 读指定文件时只能看到 `README.md`，找不到 `docs/SPEC.md`、`SupervisionPanel.tsx`、`qa/blackbox/drive.mjs`。 | [local-24-multi-agent-created.png](screenshots/local-24-multi-agent-created.png), `events/events-multi-D.txt` | 子 agent snapshot 应包含父任务 workspace 当前 repo 内容，至少能读到父任务指定文件。 |
| P0 | Multi-agent 在 Auto-accept 下执行广域 `find / -name "SPEC.md" 2>/dev/null`，超出任务“只读指定 repo 文件”的合理范围。 | [local-26-multi-agent-after-wait.png](screenshots/local-26-multi-agent-after-wait.png), `events/events-multi-D.txt` | 子任务失败时应限制在 workspace 内搜索；跨 filesystem 搜索需额外确认或默认禁止。 |
| P1 | Loop 第二轮没有获得正确轮次上下文；iter-1 和 iter-2 都输出 `LOOP-RUN 1`。 | [local-32-loop-after-42s.png](screenshots/local-32-loop-after-42s.png), `events/events-loop-F.txt` | 每轮应注入轮次、上一轮结果和目标状态；第二轮应知道自己是 iteration 2。 |
| P1 | Loop 状态语义冲突：详情页显示每轮 `Completed · failed · score 0`，整体又显示 `Scheduled run finished · Iteration limit reached · best #1`。 | [local-34-scheduled-loop-detail.png](screenshots/local-34-scheduled-loop-detail.png) | completed、failed、best、score 应有一致含义；失败轮不应被包装成成功结束。 |
| P1 | Run view 暴露 raw event/debug 流，如 `session_start`、`text_delta`、`run_end{...}`，移动端更明显。 | [local-31-loop-started.png](screenshots/local-31-loop-started.png), [local-38-mobile-loop-detail.png](screenshots/local-38-mobile-loop-detail.png) | 主视图应展示结构化 iteration/run 状态；raw JSON 只应在调试视图或可展开详情中出现。 |
| P1 | `progress_update` 也触发 approval，导致长任务被非破坏性 UI 更新反复阻断。 | [local-06-after-always-allow.png](screenshots/local-06-after-always-allow.png), `events/events-runtime-A.txt` | 进度更新应自动允许或不进入 approval；只有真实 side effect 才需要审批。 |
| P1 | Pending approval 时页面仍显示 `ready`/composer 可用，approval 状态不够显眼，容易误以为 agent 正在正常运行。 | [local-04-waiting-approval-missing.png](screenshots/local-04-waiting-approval-missing.png) | 全局状态应明确显示 `Needs approval`，composer 和环境面板应同步突出阻塞点。 |
| P1 | Approval 面板被 viewport 裁切，关键操作区域不完整。 | [local-04-waiting-approval-missing.png](screenshots/local-04-waiting-approval-missing.png) | 审批卡必须完整在视口内；窄宽度时应自动换行、滚动或重新定位。 |
| P1 | `Always allow` 范围不清；按钮看似全局授权，toast 才说只对 exact operation 生效，之后仍反复审批。 | [local-08-after-write-always.png](screenshots/local-08-after-write-always.png), [local-09-after-edit-always.png](screenshots/local-09-after-edit-always.png) | 按钮文案应直接说明 scope，如 `Always allow this exact operation/path`；重复写同一路径应可批量授权。 |
| P1 | Pending approval 时切换 mode 到 Auto-accept edits 反馈不清，pending request 不解除，UI 仍显示 Ask。 | [local-11-mode-menu.png](screenshots/local-11-mode-menu.png), [local-12-mode-auto-selected.png](screenshots/local-12-mode-auto-selected.png) | mode 变更应说明是否影响当前 pending approval，并立即反映 active/effective mode。 |
| P1 | Steer/Withdraw 状态混乱：撤回后仍残留 `queued...` 文本，缺少 withdrawn/removed 状态。 | [local-20-steer-sent.png](screenshots/local-20-steer-sent.png), [local-21-steer-toggle-after-withdraw.png](screenshots/local-21-steer-toggle-after-withdraw.png) | Queue/Steer pending item 应有统一状态卡；撤回后明确显示已撤回或从队列移除。 |
| P1 | Steer 看起来没有折入当前 generation，而是在当前 turn 完成后作为后续输入消费。 | [local-22-steer-second-sent.png](screenshots/local-22-steer-second-sent.png), [local-23-steer-after-wait.png](screenshots/local-23-steer-after-wait.png) | 如果不能实时 steer 当前 generation，应明确显示“将在下一轮发送”，不要伪装为当前 steer。 |
| P1 | 安全边界弱：普通“模拟多 Agent 协作日志”任务输出了密码破解、横向移动、清理痕迹等攻击流程。 | [local-22-steer-second-sent.png](screenshots/local-22-steer-second-sent.png), [local-23-steer-after-wait.png](screenshots/local-23-steer-after-wait.png) | Coding Agent 应将此类内容安全重写为防御/合规审计，不应生成攻击链细节。 |
| P1 | Multi-agent 父任务与子任务完成状态不同步；子任务完成后 UI 曾长时间仍显示 `running,,,`/Stop，缺少最终汇总。 | [local-25-multi-agent-kill.png](screenshots/local-25-multi-agent-kill.png), [local-26-multi-agent-after-wait.png](screenshots/local-26-multi-agent-after-wait.png) | 所有子 agent 完成后父任务应继续汇总并进入一致终态。 |
| P1 | Stop/Kill 后反馈不明确，仍显示 Stop、红色停止按钮和 `running,,,`。 | [local-25-multi-agent-kill.png](screenshots/local-25-multi-agent-kill.png) | 点击 Stop 后应立即进入 `stopping`/`stopped`/`killed` 状态，并说明子 agent 状态。 |
| P1 | Home/新任务 worktree 名无限级联，标题和 chip 超长溢出，移动端尤其严重。 | [local-27-add-task-options-menu.png](screenshots/local-27-add-task-options-menu.png), [local-35-mobile-home.png](screenshots/local-35-mobile-home.png) | worktree 名应使用短 ID/ellipsis/面包屑；新 worktree 不应基于嵌套名称继续拼接。 |
| P2 | Scheduled 列表中已达到 `max_iterations` 的 Loop 仍显示为 `Every 30s · Ran ... ago`，看起来像仍会继续调度。 | [local-33-scheduled.png](screenshots/local-33-scheduled.png) | 达到 iteration limit 的 run 应显示 finished/paused/limit reached，而不是普通 active schedule 文案。 |
| P2 | Loop 详情把两轮 iteration 放在 `AGENTS / Subagents · 2` 下，概念混淆。 | [local-34-scheduled-loop-detail.png](screenshots/local-34-scheduled-loop-detail.png) | Loop iterations 应显示为 Iterations/Runs，不应被标成 Subagents。 |
| P2 | Add menu 与 mode menu 文案粘连：如 `Ask to approveReads`、`GoalKeep`、`LoopRun`、`Files and foldersImages`。 | [local-11-mode-menu.png](screenshots/local-11-mode-menu.png), [local-27-add-task-options-menu.png](screenshots/local-27-add-task-options-menu.png) | label 与 description 应换行/分隔；读屏顺序也应包含空格和层级。 |
| P2 | Mobile Scheduled 行文本粘连：标题、频率、状态连在一起，如 `drive: loopEvery 30s · Ran...`。 | [local-36-mobile-scheduled.png](screenshots/local-36-mobile-scheduled.png) | 移动端列表应分行展示 title、schedule、status，保证可扫读。 |
| P2 | Settings/Git 暴露 `Branch prefix Not wired yet`、`PR merge method Not wired yet`，但像正式可配置项。 | [local-42-settings-git.png](screenshots/local-42-settings-git.png) | 未接线功能应隐藏、disabled，或明确说明“保存但暂不生效”。 |
| P2 | 状态文案暴露内部枚举 `acceptEdits`，与 UI 的 `Auto-accept edits` 不一致。 | [local-15-long-created.png](screenshots/local-15-long-created.png), [local-19-steer-started.png](screenshots/local-19-steer-started.png) | 所有用户可见文案应使用统一产品术语。 |
| P2 | 多处状态显示 `running,,,`，标点异常。 | [local-10-after-approval-loop.png](screenshots/local-10-after-approval-loop.png), [local-24-multi-agent-created.png](screenshots/local-24-multi-agent-created.png) | 应显示 `running...` 或统一状态文本。 |
| P2 | 工具调用文案拼接为 `read task outputcall_2_1`、`outputcall_2_0`。 | [local-24-multi-agent-created.png](screenshots/local-24-multi-agent-created.png) | 工具名、动作、call id 应分开显示，call id 使用次级/code 样式。 |
| P2 | Goal 完成元数据 `1 check00:02` 缺少分隔。 | [local-29-goal-started.png](screenshots/local-29-goal-started.png) | 应显示 `1 check · 00:02` 或同等清晰格式。 |
| P2 | `input_revoked` 后 events 记录 `waiting_resolved: input_received`，但没有对应新 input。 | `events/events-steer-C.txt` | revoked input 应记录为 `revoked`/`cancelled`/`noop`，不要伪装成 input_received。 |
| P2 | Multi-agent `output(handle)` 语义不清；父 agent 在子结果未到时得到 “output arrives as a message” 并继续生成伪中间汇报。 | `events/events-multi-D.txt` | `output` 应明确是 await、poll 还是 subscription；父 agent 应在子结果到达后再汇报。 |
| P2 | Sidebar 中大量重复/截断任务标题，区分度低。 | [local-02-home-after-wait.png](screenshots/local-02-home-after-wait.png) | 列表应显示时间、状态、短 ID、workspace 等辅助信息；hover/详情可看完整标题。 |
| P2 | Actions `qa-blackbox` 自动化返回 0 findings，但没有覆盖 Goal、Loop、Multi-agent、Approval、Steer 等高风险路径。 | `actions/qa-blackbox-artifacts/blackbox-run/blackbox-run.log`, `actions/qa-blackbox-artifacts/blackbox-run/blackbox-out/findings.json` | CI blackbox 应至少覆盖高风险 workflow；0 findings 不应代表深度质量通过。 |
| P3 | Mobile Home 只显示主内容，缺少明显入口去理解当前连接/daemon/项目列表状态。 | [local-35-mobile-home.png](screenshots/local-35-mobile-home.png) | 移动首页应保留清晰的当前 workspace、connection 和导航入口。 |
| P3 | `Back to app`/Settings overlay 与底层 task 内容同时出现在 DOM 文本里，自动化和读屏上下文可能混杂。 | [local-41-settings-attempt.png](screenshots/local-41-settings-attempt.png) | Modal/Settings 视图应正确管理 aria-hidden/focus trap，避免底层内容混入主要阅读顺序。 |
| P3 | `Create branch`、`Commit or push` 在 run/task侧栏中对未准备好或不适用状态仍显得可执行。 | [local-34-scheduled-loop-detail.png](screenshots/local-34-scheduled-loop-detail.png) | 不适用动作应 disabled，并说明原因。 |
| P3 | 本地普通 login shell 下 `npm ci`/`npm run build` 受 PATH/Node 版本影响失败；干净 PATH Node 25 成功，Actions Node 22 成功。 | 本轮命令输出，未纳入产品截图 | 构建文档/脚本应固定 Node 版本或主动检查，避免开发者本机 shell 污染导致误判。 |
| P3 | Actions phone-webui keep-alive run 的日志/URL 对未登录或无 tailnet 的测试者不可用，公开页面只显示 sign-in/summary error。 | `metadata/actions-phone-webui.json` | 部署 workflow 应把可访问地址、artifact、连接说明写入 summary；不可访问时应有 fallback artifact。 |

## 5. 正常路径记录

- Queue follow-up 基本路径通过：B 任务首轮长输出后进入 input wait，queued follow-up 触发第二轮，并输出 `QUEUE-MARKER`。证据：`events/events-queue-B.txt`、[local-18-queue-after-wait.png](screenshots/local-18-queue-after-wait.png)。
- `qa-blackbox` Actions 基线成功，覆盖了 phone/desktop home、settings、scheduled bad workspace、两轮任务、diff view、daemon-down friendly state。证据：`actions/qa-blackbox-artifacts/blackbox-run/blackbox-run.log`。

## 6. 测试数据与附件

- 截图：`screenshots/`
- Runtime events：`events/`
- Actions artifact：`actions/qa-blackbox-artifacts/blackbox-run/`
- Actions/run/连通性元数据：`metadata/`

测试会话、workspace、journal、daemon store 均按项目约定保留，未清理。
