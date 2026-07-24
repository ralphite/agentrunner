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

## 诚实边界

- 未重启 daemon：共享 daemon 正承载用户数据，未经安全窗口授权不执行。
- 未做 daemon restart 后恢复断言；Web UI reload/Back/Forward 已验证。
- 未执行真 WebKit/touch device；Chromium 390×844 与 mobile sidebar 已验证。
- 未对全部 local/session storage key 做迁移前后字节级对比；本增量未改
  storage schema，真实 reload/theme/sidebar 状态已 spot check。

最终自动化结果见 `commands.md`。
