# 自动化命令

| 命令 | 当前结果 |
|---|---|
| `node -v` | `v22.13.1` |
| `npm run lint:storybook` | 172 targets，13 semantic states，5 global pairs，13 private exclusions，553 Stories，0 missing |
| `npm run build-storybook` | PASS |
| `npm run build` | PASS |
| `npm run test:storybook` | 62 files PASS / 2 intentionally skipped；546 tests PASS（新增最终 4 个 modal Story 前的全量基线） |
| `npx vitest run --config vitest.storybook.config.ts src/components/Modals.stories.tsx` | 1 file / 40 tests PASS；覆盖最终新增 Agent/Trust busy/failure |
| `npm run test:visual` + targeted Core Demo复验 | 最终覆盖 18/18：full run 17 PASS，唯一 Autoplay reset-focus 失败修复后 targeted 1 PASS |

首轮失败不是验收通过证据；上述为最新提交后的最终结果与明确的增量复验边界。
