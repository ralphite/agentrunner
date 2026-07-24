# 自动化命令

| 命令 | 当前结果 |
|---|---|
| `node -v` | `v22.13.1` |
| `npm run lint:storybook` | 174 targets，13 semantic states，5 global pairs，12 private exclusions，559 Stories，0 missing |
| `npm run build-storybook` | PASS |
| `npm run build` | PASS |
| `npm run test:storybook` | 64 files PASS / 2 intentionally skipped；556 tests PASS |
| `npx vitest run src/storybook/scenarios/ScenarioControls.test.tsx` | 1 file / 2 tests PASS；覆盖控制点击不触发 production outside-click |
| `npx playwright test tests/visual/core-session-playback.spec.ts --config playwright.visual.config.ts` | 1/1 PASS；覆盖 Core Demo manual/autoplay/replay |
| `npm run test:visual`（此前 full）+ 当前 focused Core Demo | 全集 18/18 已覆盖；本次 controller/view 与 transport 修订再定向复验 Core Demo |

首轮失败不是验收通过证据；上述为最新提交后的最终结果与明确的增量复验边界。
