# INC-103 Storybook 真实人类节奏与 QA Demo

> 归档于 2026-07-24：真实浏览器 QA、全量 paced driver、六条 QA Demo、
> 三视角终审与 QA-94 全部通过。

**状态**：已完成并归档。

## 动机与 journey 锚

锚定 UJ-24 的 Web UI 产品面与交付工作台，不增加终端用户 journey。

真实共享环境的 QA-89 浏览器复跑确认 Home、保留 Session、Environment、
Changes、Scheduled、Settings 仍可达；但 Storybook 有两个交付缺陷：

1. 实施起点的 68 个 Story 文件中有 472 个 `play` 定义，56 个文件直接使用
   `storybook/test` 的 `userEvent`，而只有 15 个文件、48 处显式人工停留；
   合入最新 `origin/main` 后共有 479 个 `play`，均受同一节奏契约约束。
   因此原实现的绝大多数可见交互会在回放时瞬间越过中间态。
2. 唯一完整 Demo 把配置、发送、stream、Environment、Changes 共 19 步塞进
   一条长线；Queue/Steer、审批、ask、子 agent、Goal、Scheduled、恢复与
   rich Changes 等真实 QA 没有可发现、可逐步观看的简化 Demo。

## Spec delta

- `Web UI component system / Storybook workbench` 增加 INC-103 / QA-94：
  - 所有通过 `userEvent` 产生可见变化的 Story 使用同一个 paced driver；
  - 默认 `human`，人工 typing 与每次用户动作均有真实可读停留；
  - `automated` 与 `instant` 必须显式选择，不能从 `navigator.webdriver`
    推断；
  - 增加按真实 QA 聚类的 6 条可控 Demo playlist。

## Design delta

更新 DESIGN §19，不触及既有不变量：

- Storybook global 提供 `human | automated | instant` 三档 playback pace；
  默认 `human`。Vitest/component test 强制 `instant`，Playwright/CI URL 显式
  选择 `automated` 或 `instant`。
- `pacedUserEvent` 包装 Storybook `userEvent` 的全部直接交互 API，连同
  `setup()` 返回的实例；human typing 默认 48ms/字符，组合键/逐键输入默认
  180ms/键，动作后停留 1.6s；
  automated 动作后停留 400ms；instant 为 0。
- lint 禁止 Story 文件重新从 `storybook/test` 直接导入 `userEvent`，防止新
  Story 绕过节奏契约。纯断言、无可见变化的 `play` 不强造 delay。
- Demo 继续复用现有 `ScenarioRunner` / `ScenarioControls` 与 canonical
  production Story，不复制产品组件。6 条 playlist 是真实 QA 的视觉化精简：
  Session & Delivery、Attention & Permissions、Supervision、Scheduled Work、
  Changes & Artifacts、Navigation & Recovery。
- 每个 Demo step 标出 QA refs 与观察重点；自动播放切换 canonical Story，
  Play/Pause/Next/Reset/Replay/0.5×–2× 保持同一控制语义。

### UI/UX 设计说明

- **沿用模式**：复用现有 Core Session Demo 的控制条、状态、速度与全屏
  canvas；不发明第二套 transport。
- **新增 UI**：仅在 Storybook 中增加短标题、QA refs、观察重点与被播放的
  canonical Story frame。
- **风险状态**：transport 显示 `Loading…/Ready`；canonical Story 自身的
  render/play error 继续由 Storybook 原生错误面在同一 frame 内呈现，不吞错；
  Pause 在当前动作安全边界后生效，不伪装为硬中断。
- **数据处理**：全部 Demo 使用现有 fixture/MSW；不写 production store，
  不清理真实 QA 数据。
- **范围裁决**：API-only、无可见 UI 的 QA 保留真实 API Gate B，不伪造成视觉
  Demo；其用户可见投影归入上述 6 条 playlist。

## 验收

### Gate A

- `humanPlayback` unit：human / automated / instant、typing delay、test override。
- lint：所有 Story 的 `userEvent` 都走 paced driver；新增 Demo story IDs 与
  manifest/index 闭环。
- Storybook interaction、production build、Storybook build、curated visual、
  `./scripts/check.sh` 全绿。
- 浏览器测量：
  - 普通交互 Story 在 0.7s 时仍保留第一可见状态，非瞬间完成；
  - human typing 按字符推进；
  - 6 条 Demo 均可 Play/Pause/Next/Reset/Replay，step/QA ref 可见；
  - `automated`/`instant` 不污染人工默认。

### Gate B / QA-94

共享 production `http://127.0.0.1:8809/`：

- 保留 Session deep-link、Environment、Changes、Scheduled、Settings；
- reload/direct hash、真实 shared store、console health；
- 不重启承载真实 session 的 daemon，除非获得安全窗口。

当前源码 Storybook：

- Core Session human pace；
- 六条 QA Demo 的关键步骤、无空白/crash/横向溢出；
- 截图与浏览器记录保存到
  `qa/runs/2026-07-24-QA-94-storybook-demo-audit/`。

## 实施步骤

1. INC-103.1：paced driver + global pace + lint + unit，替换全部 Story raw
   `userEvent` import。
2. INC-103.2：六条 QA Demo playlist + manifest/index + browser/component tests。
3. INC-103.3：真实 shared-store 与 Storybook 浏览器终验；三层、QA、LOG 收口，
   工作纸归档。

## review 裁决

规模达到跨 Storybook 全表面的节奏契约与 6 条新 Demo，执行
interaction / visual / contract 三视角 review；P0/P1 清零后关闭。
