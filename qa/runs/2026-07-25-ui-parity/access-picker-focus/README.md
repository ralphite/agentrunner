# Access picker keyboard focus

## Why this batch

首页的 `Ask to approve` picker 打开时，键盘初始焦点落在未选中的
`Full access`。用户不移动焦点就按 Enter，会被带入更高权限确认流程。

## Visual evidence

| Before | After |
| --- | --- |
| [1440×900](before-1440x900.jpg) | [1440×900](after-1440x900.jpg) |

同一首页、同一 `Ask to approve` 状态：改前焦点蓝框包住 `Full access`；改后
焦点、`aria-current` 和 roving `tabindex=0` 都在 `Ask to approve`。

## Changed files

- `webui/frontend/src/components/Popover.tsx`
- `webui/frontend/src/features/composer/ComposerParts.tsx`
- `webui/frontend/src/components/ComposerParts.stories.tsx`

## Verification

- 独立盲审先发现问题；本机真实运行时复现了改前 `Full access` 获得初始焦点。
- 修复后同一运行时确认 `Ask to approve` 获得初始焦点与整行 focus ring。
- 定向 Storybook：`ComposerParts.stories.tsx` 与 `DiffParts.stories.tsx` 86/86 通过；远端
  CI 链接在 push 后提供。
