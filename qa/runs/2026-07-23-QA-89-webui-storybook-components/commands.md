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
| `npm run test:visual` | 18/18 PASS；覆盖 curated desktop/phone/light/dark、Core Demo、reduced motion 与 5 组 global state pairs |

## 架构边界收口前的完整门禁快照

- 执行时间：2026-07-23 21:26 PDT
- 被测提交：`9ded88a5b97aa1e42a0eac82e50a7296565b9a9e`
- 命令：`./scripts/check-webui.sh --skip-install`
- 运行时：Node.js `v22.13.1`，npm `10.9.2`
- 结果：`check-webui: all green`

| 门禁阶段 | 实际结果 |
|---|---|
| `baseline:storybook:check` | PASS；`storybook baseline: current` |
| `npm run test` | 85 files PASS；833 tests PASS |
| `npm run build` | PASS；TypeScript 与 Vite production build 完成 |
| `npm run build-storybook` | PASS；Storybook static build 完成 |
| `npm run lint:storybook` | 174 targets，13 semantic states，5 global pairs，12 private exclusions，559 Stories，0 missing |
| `npm run test:storybook` | 64 files PASS / 2 intentionally skipped；556 tests PASS |
| `npm run test:visual` | 18/18 PASS |
| production/Storybook 资产隔离检查 | PASS；production bundle 无 Storybook/MSW development assets，Storybook static build 含 `mockServiceWorker.js` |

首轮失败不是验收通过证据；上述结果只证明 `9ded88a5` 快照。Composer、
Session、Timeline 最终 feature boundary 提交后的门禁必须另行记录，不能用本快照
冒充最终证据。
