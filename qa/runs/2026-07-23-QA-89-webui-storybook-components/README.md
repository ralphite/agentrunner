# QA-89 Web UI 组件体系与 Storybook

日期：2026-07-23  
环境：Node.js `v22.13.1`，Storybook `http://127.0.0.1:6009/`，
Web UI `http://127.0.0.1:5188/`  
数据：真实共享 store `~/.local/share/agentrunner/`，未使用隔离
`HOME`/`XDG_DATA_HOME`

## 保留的真实数据

- project：`mt-test`
- session：
  `20260723-225218-reply-with-exactly-inc96-brows-2aa84db75337d904`
- session、workspace、journal 与浏览器偏好均保留，未 close、delete 或 cleanup。

## 浏览器硬断言

| 场景 | 结果 |
|---|---|
| Home desktop light | 真实 project/session 数据可见，daemon connected，无页面空白或 crash overlay |
| Session desktop light | deep link 进入保留 session，timeline、Environment、composer 均可见 |
| Changes | 从真实 retained session 的 More session actions 打开；显示 Last Turn、close/action controls 与真实 “No changes this turn” 空态 |
| Scheduled | `#scheduled` 载入共享 store 的 27 条真实 scheduled records，All/Active/Paused、failed/running/limit states 与 suggestions 可见 |
| Settings | 从 sidebar More options 打开真实 Settings dialog；General、Appearance、Keyboard、Git、Worktrees、Configuration、Archived 导航可见 |
| route / reload | reload 保持同一 hash/session；Back 回 Home；Forward 恢复同一 session |
| mobile 390×844 | `clientWidth = scrollWidth = 390`，无横向溢出 |
| mobile sidebar | 打开时 main 为 `inert` 且 `aria-hidden=true`；关闭后恢复 |
| theme | system → light → dark 可实时切换；最终恢复 system |
| AppShell Story | 最新模块在 1120×678 canvas 下 sidebar=320、main=800、composer=712×135.6；body 无 X/Y overflow，无裁切或错误外层 margin |
| Composer 360 | 使用 Storybook viewport toolbar 将 canonical Running Queued Story 设为 360px；body `clientWidth=scrollWidth=360`，全部控件右边界 ≤319px |
| Demo human pacing | Play 后 0.7s 仍停 Step 1、约 1.35s 才进 Step 2；Autoplay 同样保留可见停留；Play/Pause/Reset/Next 均可人工观察 |
| Demo manual control | `Reset→Next→Next` 保留 project Popover；`Play→Step 7→Pause→Next` 保留 access Popover 且进入 Paused Step 8，控制条不再被 production Popover 关闭或遮挡 |
| PageHost routes | Home/Session/Scheduled/Run 四个 route Story 均真实渲染；820×378 下 body `client=scroll`，无 overflow |
| browser log | Web UI 与最终 Story 页面无 Vite/React/runtime crash；Storybook manager 仅有自身 `PopoverProvider ariaLabel` deprecation warning |

## Final feature boundary browser recheck

被测提交：`b40b169a`。本轮只做真实浏览器 QA，未运行 unit/full gate。

| 场景 | 最新边界复验 |
|---|---|
| Web UI Home | `http://127.0.0.1:5188/` 使用共享 store 正常渲染；真实 sessions/projects 与 Composer 可见，`clientWidth=scrollWidth=1656`，无 crash overlay |
| retained Session | `#20260723-225218-reply-with-exactly-inc96-brows-2aa84db75337d904` 正常渲染；Timeline 3 个可见 item、Composer 与 Environment 开/关交互均正常，无横溢出或 crash |
| AppShell Default | hard reload 后 iframe `1576×1020`、sidebar `320px`、Composer 可见，body 无横溢出；`b40b169a` 后 fresh console 中 `/api/agents` unmatched handler 数量为 `0` |
| Composer Running Queued | 仅用 Storybook toolbar 将 canonical Story 调为 `360px`；body `360/360`、composer `294px`、最右控件 `319px`、`escaped=[]`，未新增 Phone/Dark Story |
| SessionView | Default 与 Running 均渲染；Default Timeline=2、Running Timeline=3；Running 的 Thinking、已输入 Queue 状态与 Composer 均可见，无 overflow/crash |
| Timeline Default | Timeline 高度约 `443.7px`、3 个 item、2 个 Copy action，无 overflow/crash |
| ModelFields | Default=`gemini/gemini-2.5-pro · high`；Custom=`custom/organization-specialist · high`；Keyboard Navigation 最终焦点在 Effort combobox |
| Core Demo manual | `Reset→Next→Next`：Step 2 project picker 保持可见且 Next 可点击，随后自然进入 Step 3；Play 后约 `1.0s` 仍为 Step 1、约 `1.84s` 才到 Step 2 |
| Core Demo popover/control | paused 时 model menu 保持打开；controls `z-index=100`、menu `z-index=50` 且几何不重叠；Next 可点击并从 paused Step 10 进入 Step 11 |
| fresh console | 最终 Story fresh logs 无 product/Vite/React/MSW error，仅 Storybook 11 `PopoverProvider ariaLabel` deprecation；Web UI 只有 dev server HMR WebSocket reconnect error，不影响产品 API/runtime |

## 截图

- `webui-home-desktop-light.png`
- `webui-session-desktop-light.png`
- `webui-session-mobile-light.png`
- `webui-session-mobile-sidebar.png`
- `webui-session-mobile-dark.png`
- `storybook-appshell-desktop.png`
- `storybook-appshell-desktop-final.png`
- `storybook-composer-running-queued-360-final.png`
- `storybook-demo-human-pacing-subagent.png`
- `storybook-demo-step7-pause-next-pass.png`
- `storybook-pagehost-home-route.png`
- `storybook-pagehost-session-route.png`
- `storybook-pagehost-scheduled-route.png`
- `storybook-pagehost-run-route.png`
- `webui-home-final-boundary-recheck.png`
- `webui-retained-session-final-boundary-recheck.png`
- `storybook-appshell-final-boundary-recheck.png`
- `storybook-composer-running-queued-360-final-boundary-recheck.png`
- `storybook-sessionview-default-final-boundary-recheck.png`
- `storybook-sessionview-running-final-boundary-recheck.png`
- `storybook-timeline-default-final-boundary-recheck.png`
- `storybook-model-fields-default-final-boundary-recheck.png`
- `storybook-model-fields-custom-model-final-boundary-recheck.png`
- `storybook-model-fields-keyboard-navigation-final-boundary-recheck.png`
- `storybook-demo-paused-popover-final-boundary-recheck.png`
- `storybook-demo-paused-next-final-boundary-recheck.png`

## 诚实边界

- 未重启 daemon：共享 daemon 正承载用户数据，未经安全窗口授权不执行。
- 未做 daemon restart 后恢复断言；Web UI reload/Back/Forward 已验证。
- 未执行真 WebKit/touch device；Chromium 390×844 与 mobile sidebar 已验证。
- 未对全部 local/session storage key 做迁移前后字节级对比；本增量未改
  storage schema，真实 reload/theme/sidebar 状态已 spot check。

最终自动化结果见 `commands.md`。
