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

## 最终 feature boundary 浏览器复验

- 执行时间：2026-07-23 21:42–21:55 PDT
- 被测提交：`b40b169a`
- 环境：
  - Web UI：`http://127.0.0.1:5188/`
  - Storybook：`http://127.0.0.1:6009/`
  - 共享数据：`~/.local/share/agentrunner/`
- 本轮没有运行 unit/full gate；以下均为真实浏览器读取、交互和 DOM 几何断言。

| 浏览器路径 | 结果 |
|---|---|
| Web UI Home + retained Session | PASS；共享数据、Composer、Timeline、Environment 开/关均正常，无 overflow/crash |
| `pages-appshell--default` hard reload | PASS；`1576×1020`、sidebar `320px`、无 overflow/crash |
| AppShell `/api/agents` fixture | PASS；`b40b169a` 后 fresh unmatched-handler error=`0` |
| Composer Running Queued toolbar `360px` | PASS；body `360/360`、composer `294px`、max control right=`319px`、escaped=`[]` |
| SessionView Default / Running | PASS；Timeline/Composer/Thinking/Queue 状态可见，无 overflow/crash |
| Timeline Default | PASS；3 items、2 Copy actions，无 overflow/crash |
| ModelFields Default / Custom / Keyboard | PASS；model/effort 值正确，Keyboard Story 的 Effort combobox 获得焦点 |
| Demo `Reset→Next→Next` | PASS；project picker 未被 controls outside-click 关闭，Next 可继续推进 |
| Demo human pace | PASS；约 `1.0s` 仍 Step 1，约 `1.84s` 进入 Step 2 |
| Demo Pause / Next / popover | PASS；controls `z=100`、menu `z=50`、几何无重叠，paused Next 成功推进 |
| fresh Storybook console | 0 product/Vite/React/MSW errors；仅 Storybook 11 manager deprecation warning |
| fresh Web UI console | 0 product runtime/API error；仅 Vite dev HMR WebSocket reconnect error |

证据截图见 `README.md` 的 “Final feature boundary browser recheck” 与截图清单。

## 最终整合 WebUI 自动门禁

- 执行时间：2026-07-23 22:05–22:07 PDT
- 被测提交：`446e9e2e302049f215b140220886a4f9e9735950`
- 命令：`./scripts/check-webui.sh --skip-install`
- 运行时：Node.js `v22.13.1`，npm `10.9.2`
- 结果：`check-webui: all green`

| 门禁阶段 | 最终结果 |
|---|---|
| `baseline:storybook:check` | PASS；`storybook baseline: current` |
| `npm run test` | 85 files PASS；830 tests PASS |
| `npm run build` | PASS；TypeScript 与 Vite production build 完成，7,241 modules transformed |
| `npm run build-storybook` | PASS；Storybook static build 完成，7,595 modules transformed |
| `npm run lint:storybook` | 176 targets，13 semantic states，5 global pairs，12 private exclusions，562 Stories，0 missing |
| `npm run test:storybook` | 64 files PASS / 2 intentionally skipped；559 tests PASS |
| `npm run test:visual` | 18/18 PASS |
| production/Storybook 资产隔离检查 | PASS；production bundle 无 Storybook/MSW development assets，Storybook static build 含 `mockServiceWorker.js` |

门禁修复后还对先前失败的 `Home.stories.tsx`、`Modals.stories.tsx` 和
`PageHost.stories.tsx` 做了定向复验：3 files、52 tests 全部通过。最终完整
门禁只在上述提交上运行一次，没有运行 repository-wide Go tests。

## origin/main final reconciliation gate

- 执行时间：2026-07-23 22:16–22:19 PDT
- 被测提交：`40f89c16fcc70e6613293197e850e19a7ff6c5ca`
- 命令：`./scripts/check-webui.sh --skip-install`
- 运行时：Node.js `v22.13.1`，npm `10.9.2`
- 覆盖整合：origin/main attachment-only create contract、lazy Agent Catalog、
  AppRuntime mount no-I/O contract、Composer feature boundary 与最终 Storybook baseline
- 结果：`check-webui: all green`

| 门禁阶段 | 最终结果 |
|---|---|
| `baseline:storybook:check` | PASS；`storybook baseline: current` |
| `npm run test` | 85 files PASS；831 tests PASS |
| `npm run build` | PASS；TypeScript 与 Vite production build 完成，7,241 modules transformed |
| `npm run build-storybook` | PASS；Storybook static build 完成，7,595 modules transformed |
| `npm run lint:storybook` | 176 targets，13 semantic states，5 global pairs，12 private exclusions，562 Stories，0 missing |
| `npm run test:storybook` | 64 files PASS / 2 intentionally skipped；559 tests PASS |
| `npm run test:visual` | 18/18 PASS |
| production/Storybook 资产隔离检查 | PASS；production bundle 无 Storybook/MSW development assets，Storybook static build 含 `mockServiceWorker.js` |

这是上述 origin/main 最终整合 HEAD 的唯一一次完整 WebUI 门禁；未运行
repository-wide Go tests。
