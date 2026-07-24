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
| route / reload | reload 保持同一 hash/session；Back 回 Home；Forward 恢复同一 session |
| mobile 390×844 | `clientWidth = scrollWidth = 390`，无横向溢出 |
| mobile sidebar | 打开时 main 为 `inert` 且 `aria-hidden=true`；关闭后恢复 |
| theme | system → light → dark 可实时切换；最终恢复 system |
| AppShell Story | 1200×598 下 sidebar、Home、composer 填满 canvas，无裁切或错误外层 margin |
| browser log | 本次 Web UI 路径无相关 error/warning；Storybook 仅有 manager 自身的 PopoverProvider warning |

## 截图

- `webui-home-desktop-light.png`
- `webui-session-desktop-light.png`
- `webui-session-mobile-light.png`
- `webui-session-mobile-sidebar.png`
- `webui-session-mobile-dark.png`
- `storybook-appshell-desktop.png`

## 诚实边界

- 未重启 daemon：共享 daemon 正承载用户数据，未经安全窗口授权不执行。
- 未做 daemon restart 后恢复断言；Web UI reload/Back/Forward 已验证。
- 未执行真 WebKit/touch device；Chromium 390×844 与 mobile sidebar 已验证。
- 未对全部 local/session storage key 做迁移前后字节级对比；本增量未改
  storage schema，真实 reload/theme/sidebar 状态已 spot check。

最终自动化结果见 `commands.md`。
