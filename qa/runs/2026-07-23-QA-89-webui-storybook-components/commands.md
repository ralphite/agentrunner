# 自动化命令

| 命令 | 当前结果 |
|---|---|
| `node -v` | `v22.13.1` |
| `npm run lint:storybook` | 172 targets，13 semantic states，13 private exclusions，535 Stories，0 missing |
| `npm run build-storybook` | PASS |
| `npm run build` | PASS |
| `npm run test:storybook` | 首轮 517 PASS / 15 FAIL；失败项已进入集中修复，最终结果待更新 |
| targeted Playwright Demo + reduced motion | reduced motion PASS；Demo 首轮暴露 Pause 竞态，已修复，最终结果待更新 |

首轮失败不是验收通过证据；本文件会在最终集中复验后更新为最终结果。
