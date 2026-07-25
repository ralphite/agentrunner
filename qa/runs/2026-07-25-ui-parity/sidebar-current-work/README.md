# Sidebar current-work hierarchy

## Why this batch

独立真实运行时盲审发现两个 P1：390px Sidebar 的每条历史行都常驻无说明
`…`；桌面默认先展开大量历史项目，当前工作被淹没。

## Visual evidence

| Before | After |
| --- | --- |
| [用户报告的 Sidebar 证据](before-user-report.png) | [390×844 真实运行时](after-390x844.jpg) |

`before-user-report.png` 是本需求提出前用户提供的真实问题证据；它与改后
390×844 视口并非逐像素对照。自本批起，每批会在改动前立即采集同视口截图。

## User-visible result

- 窄屏的普通项目/会话行不再常驻 `…`；整行仍可直接进入会话，项目的管理菜单
  在 hover 或键盘 focus 时可达。
- 当前会话所在项目默认位于 Projects 首位；默认项目范围从 8 收紧为 4，其余保留
  在已有 `Show more projects` 后。

## Changed files

- `webui/frontend/src/components/SidebarItems.tsx`
- `webui/frontend/src/tw.css`
- `webui/frontend/src/viewModels.nav.ts`
- `webui/frontend/src/components/SidebarItems.stories.tsx`
- `webui/frontend/src/components/Sidebar.stories.tsx`
- `webui/frontend/src/viewModels.nav.test.ts`
- `webui/frontend/src/components/Sidebar.nav.test.tsx`

## Verification

- 独立真实运行时复测：390×844 无常驻 `…`；当前项目置顶；键盘 focus 保留项目菜单。
- 定向 Storybook 与导航验证在提交前执行；远端 CI 链接在 push 后补到交接信息。
