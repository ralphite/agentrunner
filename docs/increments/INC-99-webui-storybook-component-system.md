# INC-99 Web UI 组件体系与 Storybook 工作台

**状态**：设计候选，已吸收 2026-07-23 两份独立实施前 review，等待实施裁决。
本文只定义开发基础设施、组件边界、覆盖契约与迁移顺序；不改变当前产品行为，
也不代表 Storybook/组件重构已经落地。

## 动机与 journey 锚

### 增量归类

本增量保护的产品 journey 是 `UJ-24 Web UI 驾驶 AgentRunner`：组件重构前后，
用户仍能在同一 Web UI 完成派活、续聊、监督、审批和改动审阅，且 shared-store、
deep link、持久化偏好与移动端行为不回归。

Storybook 工作台本身是开发与交付质量基础设施，不是终端用户功能：

- `JOURNEYS.md` **无 delta**，不把开发者工作台塞进 UJ-24 的用户步骤；
- `SPEC.md` **无新产品功能点**，既有 Web UI 条目保持不变；
- `DESIGN.md` 追加 development-only 组件/测试边界；
- `QA.md` 新增组件重构的真实环境验收菜单，作为 UJ-24 的回归证据；
- 收口时 `LOG.md` 记录工具链、边界、耗时预算与 review 裁决。

若未来要把工作台发布给终端用户、加入 production route 或形成独立 maintainer
产品面，必须另起 journey/spec delta；本增量不提前承诺。

### 用户结果

后续新增或修改 Web UI 时，开发者能在不启动 daemon、不制造 session、不碰共享
数据的前提下：

1. 浏览 manifest 中每个可见 production component 的直接 Story；
2. 检查所有适用且用户可区分的状态，缺项或豁免可机械审计；
3. 重放确定性 CUJ，并用自动或逐步点击方式播放核心 Demo；
4. 在提交前发现布局、响应式、主题、键盘、可访问性和状态组合问题；
5. 最终仍以 shared-store 真 Web UI 证明 UJ-24 没有回归。

这不是视觉改版。当前 Codex 式信息架构、文案、动作层级、路由、持久化 key、
backend API、journal 语义和 shared-store 数据都保持不变。

### 当前问题与可重生成基线

AgentRunner 已有 React 组件与大量 DOM 测试，但文件级组件仍同时承担数据获取、
全局 store、持久化、计时、状态投影和多个独立 surface。基于
`916b4d36d29a15979693d2bd06d8cae4b3a4ed89`：

- `webui/frontend/src/components/` 有 36 个 production `.tsx`，共 14,780 行；
- 9 个组件超过 500 行：

| 组件 | 行数 | 主要耦合 |
|---|---:|---|
| `Composer` | 2101 | API/store/storage/timer + 多个 surface |
| `DiffView` | 1800 | API/store/storage + diff projection |
| `SessionView` | 1489 | API/store/polling/SSE/storage |
| `Timeline` | 1451 | timeline projection/storage/local UI timer |
| `Scheduled` | 1163 | API/store/timer + list/detail/form |
| `SupervisionPanel` | 994 | API/store/timer + 多个 section |
| `Modals` | 964 | API/store/timer + 多种 dialog |
| `Sidebar` | 889 | API/store/storage + navigation |
| `ChangesOutcome` | 588 | API/store + mutation |

- 当前没有 Storybook 配置、Story 文件或自动化 Story inventory；
- frontend test 数量、组件数、行数会继续变化，不能手填为迁移分母；
- `scripts/check.sh` 当前按既有用户决定暂停 frontend test/build 腿，前端改动依赖
  手跑命令；INC-99 必须先建立可执行、可计时的独立前端 gate，不能假设现有
  `check.sh` 已覆盖；
- 真实 QA 已覆盖 desktop/mobile、light/dark、shared-store 与特殊状态，但触发
  成本高，不适合作为日常组件工作台。

`99.1` 必须用脚本生成并提交 baseline artifact：production component
`source/export/private-visible-id`、LOC、测试数、storage key、route、Story target；
以后只由脚本更新，不以本文静态数字判断完成度。

### HANDA 实践结论

HANDA 当前有效做法：

1. `Base → Components → Panels → Pages → CUJs → Demos → Future` 分层；
2. Story 直接 import production component，共享 typed fixture，theme 用全局
   decorator；
3. Story 在 Playwright Chromium 中真实 render；
4. `play()` 通过 `step()`、role/test-id 与断言重放交互；
5. 演示 timing 与快速测试分离；
6. 单独维护 `Component × State` 与 `Workflow × Journey` 审计。

数字必须按时点解释：

- 2026-06-13 初始审计只覆盖 22 个 authoritative components，适用 cell 为
  `92/128 = 72%`；
- 随后的 TODO 已修复绝大多数 High/Medium 缺口，因此 72% **不是当前覆盖率**；
- 当前 checkout 有 72 个 Story 文件、467 个 named Story export，但没有重跑同口径
  的 22×state 矩阵，不能用 Story 数量代替状态覆盖。

同时吸收其限制：

- Storybook 不自动带来组件化，HANDA 仍有三个约 1200 行的大组件；
- page harness 复制生产 App 会导致 Story 与 production 漂移；
- `ProductShowcase` 是静态展示，不是可控 playback；
- transitive render 不等于直接覆盖，重复 Story 不等于新增状态证据；
- `Future` mock 若不隔离，会制造已有产品能力的假象。

因此 AgentRunner 采用其分层、fixture、browser play 与覆盖矩阵，但改进为：
**production shell 直接挂载、Story 与组件 colocate、权威 typed manifest、完整
services seam、快速 CUJ 与慢 Demo 分离、Future 不计覆盖、真实环境最终验收。**

## Goals

1. 将页面按现有产品边界拆成可复用 view component；远程 I/O、持久化和业务轮询
   留在 controller/runtime。
2. manifest 中每个 user-visible production component 都有直接 Story；每个适用
   状态 cell 都映射到实际 `storyId` 或结构化 `N/A`。
3. 形成 `Foundations → Components → Features → Pages → CUJs → Demos` 体系。
4. Story 在 Playwright Chromium 中执行 render、interaction 与 a11y。
5. 提供复用同一 scenario runner 的 `Play / Pause / Next / Reset / Replay` 核心 Demo。
6. 组件重构期间保持 DOM、文案、产品行为、route、storage 与 shared-store 数据兼容。

## Non-goals

- 不重做视觉语言，不引入第二套 design system；
- 不在 production Web UI 增加 Demo route、mock 开关或 Story-only prop；
- 不改变 backend API、journal、session/runtime、hash deep link 或 storage key；
- 不把所有状态做笛卡尔积截图；
- 不把 speculative `Future` mock 计作产品或测试覆盖；
- 首轮不接 Chromatic 或其他外部 SaaS；
- 不承诺 TypeScript 能穷尽 backend 的开放 `string/any` contract；未知值必须 truthful
  fallback，backend schema 收窄或生成另起增量。

## 三层 delta

### Journey delta

无。UJ-24 是受保护 journey，不增加“开发者打开 Storybook”的终端用户步骤。

### Spec delta

无。Storybook、manifest 和 Demo 是交付基础设施，不作为新的产品功能点登记。
既有 Web UI SPEC 条目的状态与验收锚不因 mock Story 自动升级。

### Design delta

在 `DESIGN.md § Web UI 产品 surface` 追加以下 additive 设计，不修改粗体不变量：

1. `webui/frontend` 分为 production runtime、可复用 view、feature controller 与
   development-only Storybook assets；Story/fixture/MSW 不进入 Go embed bundle。
2. Story fixture 只是输入，不复制 backend 状态机，也不是产品状态真相。
3. view component 禁止远程 I/O、Zustand singleton、router、持久化和业务轮询；
   允许 focus、open/close、copy feedback 等局部 ephemeral state 与短 timer。
4. production 与 Storybook 共用同一 `AppShell`/page view，不复制 layout。
5. store 支持 factory + Provider；production adapter 保留现有 key、迁移和行为。
6. controller/runtime 只依赖完整 `AppServices`；不并存一套全局 `ApiClient` DI 与
   另一套 MSW facade。
7. HTTP Story 通过 MSW v2 驱动 production request shape；SSE 通过同一 transport
   contract 的 `StreamClient` 驱动，不能绕过 `EventSource` 漏到真实 origin。
8. backend 开放字符串先进入 decoder/view-model 映射；已知值用 closed frontend
   union 穷尽，未知值进入显式 `unknown` Story 和 truthful UI fallback。

既有不变量继续成立：

- Web UI 不复制 session 状态机或建立第二套运行真相；
- responsive 只改可见性，不改状态、deep link 或 command；
- UI 重构不迁移/删除用户偏好、session、workspace、journal 与 QA 数据。

### UI/UX 约束

- **沿用模式**：保持现有 control、文案、密度、动作层级、focus 与 overlay 行为。
- **新增 UI**：只存在于 Storybook Canvas 的 Demo controls，不进入 production。
- **风险状态**：Reset、Replay、mock failure 只操作内存 scenario，不接触共享数据。
- **数据处理**：fixture 不包含真实凭据或用户内容；Gate B 使用现有共享数据且不清理。
- **未决产品问题**：无；任何视觉或交互变化必须另起增量，不夹带在抽取 commit。

## 目标架构

```text
webui/frontend/
  .storybook/
    main.ts
    preview.tsx
    vitest.setup.ts
    public/
      mockServiceWorker.js  # 仅由 Storybook staticDirs 服务，不进 Vite public/dist
  src/
    app/
      AppRuntime.tsx
      AppShell.tsx
      AppProviders.tsx
      appServices.ts
      productionServices.ts
    ui/
      actions/
      feedback/
      navigation/
      overlays/
      status/
    features/
      composer/
      sidebar/
      session/
      timeline/
      changes/
      supervision/
      scheduled/
      settings/
    pages/
      HomePage.tsx
      SessionPage.tsx
      ScheduledPage.tsx
      RunPage.tsx
      SettingsPage.tsx
    storybook/
      fixtures/
      handlers/
      streams/
      scenarios/
      StoryFrame.tsx
      ScenarioRunner.ts
      storyManifest.ts
      storageManifest.ts
```

Story 与 production component colocate：

```text
features/composer/
  ComposerController.tsx
  ComposerView.tsx
  ComposerView.stories.tsx
  ComposerView.test.tsx
```

### Runtime 与 services 边界

- `ui/**`：React、icons、CSS token 与纯类型；禁止 `AR`/store/router/persistence。
- feature view：typed props + callbacks；禁止远程 I/O、全局 store 与业务 polling。
- feature controller：允许 store、effect、optimistic reconciliation 与 feature-local
  polling；例如 session poll/inspect/stream 不上提为全局 list polling。
- page：组装 feature，不重写 feature 状态机。
- `AppRuntime`：唯一 app-wide list polling、hash routing、notification owner。
- `AppShell`：唯一 production layout，可独立挂载。

不引入通用 DI framework，只定义一套：

```ts
interface AppServices {
  transport: {
    request<T>(request: ApiRequest): Promise<T>
    subscribe(request: StreamRequest, observer: StreamObserver): StreamSubscription
  }
  storage: StorageAdapter
  navigation: NavigationAdapter
  clock: Clock
  ids: IdGenerator
}
```

- production `request` 封装现有 `AR/fetch`，`subscribe` 封装 `EventSource`；
- Story 的 HTTP 仍走 production request adapter，由 MSW 截获；
- Story 的 stream 使用 deterministic adapter，不建立真实网络连接；
- storage/navigation/clock/id 只在有相应行为的 controller/runtime 注入；
- leaf Story 只传 props，不引入 services。

### 组件拆分原则

采用“characterization → 抽一个 leaf 并替换 production import → 自动 gate →
必要的真实 Gate B”。不做一次性目录搬家。

目标 surface：

| 当前组件 | 目标 production 组件 |
|---|---|
| `App` | `AppRuntime`、`AppShell`、`GlobalShortcuts`、`MobileNavigation` |
| `Composer` | `ComposerView`、`ProjectContextBar`、`DraftInput`、`AttachmentStrip`、`AddMenu`、`AccessPicker`、`ModelPicker`、`DeliveryToggle`、`GoalOptions`、`ComposerActions` |
| `Sidebar` | `SidebarShell`、`PrimaryNav`、`SidebarSection`、`ProjectGroup`、`ProjectRow`、`SessionRow`、`SidebarFooter`、`SidebarResizeHandle` |
| `SessionView` | `SessionPageView`、`SessionHeader`、`SessionAlerts`、`ThreadPane`、`CompactGoalBar`、`EnvironmentOverlay`、`ChangesLayout` |
| `Timeline` | `TimelineView`、`UserMessage`、`AssistantMessage`、`MessageActions`、`ToolCard`、`ToolDetail`、`WorkedFold`、`RetryFold`、`TimelineSkeleton`、`JumpToLatest` |
| `DiffView` | `DiffPageView`、`DiffToolbar`、`DiffScopePicker`、`FileDiffCard`、`FileHeader`、`UnifiedDiffBody`、`SplitDiffBody`、`UntrackedFileCard`、`DiffSkeleton` |
| `ChangesOutcome` | `ChangesSummaryCard`、`ChangedFileList`、`ChangesActions` |
| `SupervisionPanel` | `EnvironmentPanel`、`GoalSection`、`ProgressSection`、`AgentsSection`、`AttentionSection`、`BackgroundSection`、`ArtifactsSection`、`EnvironmentSection` |
| `Scheduled` | `ScheduledPageView`、`ScheduleFilterBar`、`ScheduleRow`、`ScheduleList`、`ScheduleDetailPanel`、`ScheduleEmptyState`、`ScheduleSuggestions` |
| `Modals` | `Dialog` + `NewSessionDialog`、`RunDialog`、`ForkDialog`、`AgentDialog`、`ConfirmDialog`、`PromptDialog`、`TrustDialog`、`RenameDialog`、`ViewerDialog` |
| `Settings` | `SettingsShell` + 既有 section |

行数不是单独判据：

- orchestrator 只组装，不内嵌多个独立 surface；
- leaf/view 通常不超过约 300–400 行；
- 同一文件不同时承担 fetch + persistence + projection + 五个以上 UI 区块；
- `ui/**` 与 view 的 `AR/useStore` 直接依赖为 0；
- 局部 copy/focus/open state 不为追求“纯”而强制上提。

## Story 体系

### 分层

1. `Foundations`：token、typography、button、badge、spinner、skeleton、menu、
   popover、dialog、tooltip、focus。
2. `Components`：直接、可复用 leaf/view。
3. `Features`：完整功能区，可有 controller harness，不含 page routing。
4. `Pages`：production `AppShell`/page view 的完整静态状态。
5. `CUJs`：快速、确定性、带断言的连续 journey。
6. `Demos`：人眼可见 playback，默认 `!test`。
7. `Future`：必须 `!test` + `experimental`，不进入覆盖与产品结论。

### 权威 manifest

不能从“36 个文件”推断所有可见组件。`storyManifest.ts` 是唯一、版本化目标集，
每项按 `source + componentId` 标识，既能登记 named export，也能登记当前文件内的
private visible leaf：

```ts
type CoverageCell =
  | { status: "covered"; storyId: string }
  | {
      status: "n-a"
      reason: string
      evidence: string
      owner: string
    }

type ComponentTarget = {
  componentId: string
  source: string
  exportName?: string
  states: Record<StateCellId, CoverageCell>
}
```

约束：

- 每个 manifest component 必须有直接 Story；transitive render 不计；
- 每个 `storyId` 必须存在于 built Storybook `index.json`；
- 一条 Story 可覆盖多个明确 cell，但 cell 必须反向登记该 Story；
- `N/A` 只表达确实不适用，不能豁免难测组件；
- source 新增可见 export/private leaf 时，source annotation/lint 要求同步 manifest；
- 报告只由脚本生成：`COVERED / N-A / MISSING`，不手工维护百分比；
- `99.3` 固化初始 `MISSING` baseline，迁移期 CI 强制只减不增；被当前 commit
  修改/抽取的 target 不得继续 `MISSING`，`99.19` 收口要求全量归零。

### 状态模型

domain 按“用户可区分的等价类”枚举：

- closed frontend union：`satisfies Record<KnownState, Fixture>`；
- backend 开放 `string/any`：已知值 catalog + 至少一个 `unknown` fallback Story；
- 不声称 backend 新值一定触发 TypeScript compile error。

全局维度不做笛卡尔积，manifest 明确 pairwise cell：

| 维度 | 标准 cell |
|---|---|
| 数据生命周期 | initial/loading/ready/empty/error/retry 中适用值 |
| 交互 | idle/focus/open/disabled/submitting/success/failure 中适用值 |
| 密度 | minimal/populated/long-content/overflow |
| responsive | desktop `1280×720`、phone `390×844`、short `390×500` |
| theme | light/dark；system 只在 theme journey |
| accessibility | keyboard/Escape/focus-return/name/target/reduced-motion |
| domain | 每个已知可达值 + unknown fallback |

最低高风险 pair：

- `error + dark`
- `long-content + mobile`
- `overlay + short viewport`
- `loading + reload`
- `attention + mobile`

`scripts/lint-storybook.mjs` 检查 manifest closure、story existence、taxonomy、重复 id、
orphan Story、结构化 exemption、tag 与 production bundle exclusion。

## Demo 设计

### 位置与边界

完整 Demo 在 services/store/navigation/storage seam 完成后才落地。它只住
`Demos/Core Session Playback`，使用 production `AppShell`、pages、features、leaf；
不进 production route，不加 `demoMode/isStorybook` branch。

scenario 使用：

- 独立 `createAppStore()`；
- MSW HTTP + deterministic stream；
- 内存 localStorage/sessionStorage/navigation；
- 固定 clock/id；
- 真实 DOM click/type/keyboard，而不是直接改 React state。

### ScenarioRunner 协议

状态机：

```text
idle → running ⇄ paused → completed
           └──────→ failed
任意非 resetting 状态 → resetting → idle
```

执行协议：

1. 同一 Canvas 只有一个 owner：`manual | autoplay | interactions`；
2. `DemoStep.run(context, AbortSignal)` 每步重新查询 DOM，不能持有过期 element；
3. Pause 取消等待，并在当前 atomic user action 边界停住；不承诺打断浏览器已经提交的
   单次 input event；
4. Next 仅在 `idle/paused` 执行一个 step；
5. Reset 先 abort 旧 generation，再 unmount/remount Provider；
6. Reset 同时重建 store、MSW scenario state、stream、clock/id、local/session
   storage、hash/history、portal root；
7. 旧 generation 的 timer/fetch/stream callback 必须因 signal/generation token
   失效，不能写入新 store；
8. Interactions Replay 先强制 Reset，再取得唯一 owner；autoplay/manual 正在运行时
   不允许双跑；
9. failure 停在现场，显示 step/error，不自动跳过。

`play()` 与 Canvas controls 复用同一 actions/assertions：

- `CUJs`：零/极短延时 + deterministic clock，进入 CI；
- `Demos`：visible timing，可 `1×/2×`，标记 `!test`；
- timing 不写进 scenario action 本身。

### 首个完整 scenario

`Core session → completion → review`：

1. Home 空态；
2. 选择真实形态 project；
3. 点击 Build starter 与 suggestion；
4. 展开 access/model；
5. Send；
6. loading → user message → running tool → streamed assistant；
7. Environment 显示 progress/agent/attention；
8. turn 完成并显示 Worked/Changes；
9. Review 打开 Changes；
10. desktop/mobile 各验证关闭与返回。

后续 scenario：

- `Queue → Steer → Stop → Retry`
- `Approval + structured Ask`
- `Parent → child → answer/approval → parent`
- `Scheduled list → detail → pause/resume → reload`
- `draft/deep-link/theme persistence`

## 验收

### Gate 0：工具链兼容与预算

实施前先安装一个 smoke branchless commit，并锁定同组精确版本：

- `storybook`、`@storybook/react-vite`、`@storybook/addon-vitest`、
  `@storybook/addon-a11y`：`10.5.3`
- `vitest`、`@vitest/browser-playwright`：`4.1.10`
- `playwright` / `@playwright/test`：`1.61.1`
- `msw`：`2.15.0`
- `msw-storybook-addon`：`2.0.7`

版本以 `99.1` 当日 registry + clean `npm ci` 实测为准；若版本已变化，只能整体锁到
一组验证过的精确版本，不能写 `10.x`/`latest` 或 Vitest 3/4 混装。

`99.1` 记录固定 Node、cold/warm install、unit/build、Story smoke/full、golden
耗时与峰值内存。默认预算：

- 本地单 Story smoke ≤ 30s；
- frontend unit + production build 保留现有量级；
- full Story render/a11y/golden 可分 4 shard，CI 单 shard ≤ 5min；
- 超预算先减少 golden、提高 shard；不能用 changed-story 取代 full gate。

### Gate A：确定性自动化

MSW 必须完整接线：

1. `npx msw init .storybook/public/ --save` 生成 worker；
2. Storybook `staticDirs: ["./public"]` 只服务 `.storybook/public/`；
3. `preview.tsx` 调用 `initialize()` 并注册 `mswLoader`；
4. `/api/**` 的 unhandled request 直接 `error`；
5. 每个 Story/Reset 清 handler、scenario server state 与 stream；
6. production bundle 检查排除 worker、Story、fixture、MSW。

每个 INC-99 commit 在 push 前运行：

```bash
./scripts/check.sh
./scripts/check-webui.sh
```

`scripts/check-webui.sh` 明确执行：

```bash
cd webui/frontend
npm ci
npm run test
npm run build
npm run build-storybook
npm run lint:storybook
npm run test:storybook
npm run test:visual
```

不擅自撤销当前 `check.sh` 暂停 frontend 腿的用户决定；INC-99 通过独立 mandatory
script 补齐。若以后恢复到 `check.sh`，另记 LOG 决策。

CI 从 `99.2` 开始即 blocking，不等收口才接：

- 固定 Linux image、Node、lockfile 与 Chromium revision；
- `playwright install --with-deps chromium`；
- cache、4 shard、per-job timeout；
- 失败上传 Story URL、trace、DOM snapshot、console、actual/diff screenshot；
- global CSS、token、decorator、store、manifest 变化仍跑 full suite；
- changed-story 只可作为本地快速补充。

自动化判据：

- production 与 Storybook static build 成功；
- 所有 `test` Story 在 Chromium render；
- 所有 `play()` assertion 通过；
- 全局 `parameters.a11y.test = "error"`，无未登记 violation；
- manifest 无新增 `MISSING`，当前变更 target 已覆盖；收口时全量无 `MISSING`；
- `Future/Demos` 不误入 test coverage；
- production bundle 不含 development-only assets；
- 现有 frontend tests 不减少、不替换为只有 snapshot。

### a11y、mobile 与 visual

axe 只证明自动可检测部分，以下必须显式测试：

- `play()`：Tab path、Escape、focus return、role/name；
- 独立断言：touch target、coarse pointer、reduced-motion；
- Playwright mobile device context：touch、DPR、visual viewport；
- 目标包含 iOS 行为的 surface 另跑 WebKit；软键盘/safe-area 风险做真机 Gate B；
- theme：light/dark/system、live `prefers-color-scheme`、reduced-motion。

视觉回归用独立 `@playwright/test` runner：

- baseline：`webui/frontend/tests/visual/__screenshots__/`；
- 固定 Linux image、browser、fonts、DPR、locale、timezone；
- freeze clock/id，关闭非必要 animation，等待 fonts/network/stream settled；
- 更新只允许 `npm run test:visual:update`，actual/diff 必须人工 review；
- 像素 baseline 只覆盖稳定 Page/CUJ 关键帧，不覆盖所有 Story。

golden 初始集：

- Home；
- completed/running/attention Session；
- Changes；
- Scheduled list/detail；
- Settings；
- desktop/mobile × light/dark 的 curated pair。

### Gate B：真实 shared-store Web UI

任何涉及 store factory、route、polling、storage、page shell 或大组件切换的 commit，
都在真实 `~/.local/share/agentrunner/` 与 production URL 验证：

1. 页面身份：home/session/run/scheduled/scheduled-detail/settings；
2. list → detail、直接 deep link、Back/Forward、reload；
3. existing 与 fresh 的 completed/running/failed/attention/parent-child/scheduled；
4. console 无相关 error/warning，页面非空且无 crash overlay；
5. 至少驱动一个本 commit 受影响的真实 control；
6. desktop、touch/coarse-pointer mobile、short viewport；
7. light/dark/system、live theme change、reduced-motion；
8. Web UI restart 后 route、内容和偏好恢复；
9. storage manifest 中每个 local/session key 做迁移前后字节级对比，包括 draft、
   request id、timeline scroll、diff scope/wrap、supervision、sidebar/project folds、
   theme/appearance/git preference；
10. session/workspace/journal/QA 数据保留，不 close/delete/cleanup。

daemon restart 会影响真实 live work：默认只重启 Web UI。只有 `ar sessions`/真实状态
确认无在跑工作且获得用户同意的时间窗，才执行 daemon restart；否则 Gate B 记
`BLOCKED` 并保留到安全窗口，不能用隔离 store 冒充通过。

证据保存到 `qa/runs/<日期>-QA-89-webui-storybook-components/`：

- URL、storage root、session/project id；
- before/after API 与 console 摘要；
- desktop/mobile/light/dark screenshot；
- automated command 与耗时；
- 未测路径或 blocker。

### QA-89 菜单

实施时在 `QA.md` 新增：

- A：manifest/state matrix/render/a11y；
- B：CUJ + ScenarioRunner lifecycle；
- C：curated golden；
- D：shared-store Home/Session/Changes/Scheduled/Settings；
- E：route/Back/Forward/reload/Web UI restart；
- F：storage manifest 逐 key compatibility；
- G：touch/mobile/WebKit/reduced-motion/theme；
- H：Demo autoplay/manual/Pause/Next/Reset/Replay；
- I：production bundle 无 mock/Story。

## 实施步骤

`99.1/99.5/99.6/99.8/99.15/99.18/99.19` 与以下每个带 suffix 编号都是
独立可合并提交；`99.2/99.3/99.4/99.7`、`99.9–99.14`、`99.16/99.17`
只是 commit namespace，不是一个“大提交”。
每个 manifest target 由 `99.3a` 冻结唯一 suffix，并按下述两步分别提交：

1. `<target>-c`：只加当前 production surface 的 characterization Story/test；
2. `<target>-e`：只抽该 target、替换 production import、补直接 Story 与 manifest
   cell；涉及 route/storage/polling/page shell 时，同 commit 保存 Gate B 证据。

每个 commit 均运行 Gate A；迁移 namespace 不得整体提交。

1. **99.1 Compatibility + generated baseline**
   - 验证并锁定完整工具链版本；
   - 生成 component/LOC/test/storage/route baseline；
   - 记录 cold/warm 时间预算；
   - 只落最小 smoke，不改 production。

2. **99.2 Storybook + MSW + blocking CI foundation**
   - **99.2a**：接 `main/preview/vitest.setup`、真实 CSS/theme/portal/viewport，
     smoke Story 证明 clean `npm ci` 与 browser render；
   - **99.2b**：完成 Storybook-only worker/staticDirs/initialize/mswLoader/
     unhandled policy，并证明 `/api/**` 不漏到真实 origin；
   - **99.2c**：增加 `check-webui.sh`、blocking CI、shard/budget 与 failure
     artifacts，证明 a11y 与 production exclusion。

3. **99.3 Typed manifest + audit closure**
   - **99.3a**：定义 schema，登记当前可见 export/private visible leaf，
     固化 `MISSING` baseline 与后续每个 leaf 的 commit id；
   - **99.3b**：lint 从 manifest + Storybook `index.json` 证明 closure、
     `MISSING` 只减不增；不用“36 个文件”冒充 component 清单。

4. **99.4 Store factory isolation** — **Gate B**
   - **99.4a**：只增加 store/storage/migration characterization；
   - **99.4b**：将模块级 mutable state 收进 instance，增加
     `createAppStore()`，并让既有 production singleton 由 factory 构造，保证新
     export 已真实接线；尚不引入 Provider；
   - **99.4c**：接 Provider + production singleton adapter；全部 key/行为不变，
     执行 reload/restart/storage byte comparison Gate B。

5. **99.5 AppRuntime characterization** — **Gate B**
   - 先锁定 app-wide list polling、hash route、shortcuts、notification；
   - 明确 feature-local polling/SSE 仍在 controller；
   - production 行为零改动。

6. **99.6 AppShell extraction** — **Gate B**
   - 只抽唯一 layout 与 page composition；
   - Story 挂真实 `AppShell` 静态状态，不做完整 Demo；
   - 验证 desktop/mobile/overlay/navigation。

7. **99.7 Complete AppServices seam** — **Gate B**
   - **99.7a**：定义 transport/storage/navigation/clock/id contract，接
     production request adapter 与 app-wide consumers；
   - **99.7b**：迁移 `SessionView` poll/inspect/SSE，Story HTTP 由 MSW、
     stream 由 deterministic adapter；
   - **99.7c**：迁移 `RunView` SSE 与剩余 controller consumers；
   - 每个 suffix 分别 Gate B，并证明无 Story request/stream 漏到真实 origin。

8. **99.8 ScenarioRunner lifecycle**
   - 先用最小测试 fixture 落状态机、Abort、single owner、Reset/remount；
   - 覆盖 Pause/Next/Replay/double-run/stale callback；
   - 尚不承诺完整 Core Session Demo。

9. **99.9 Foundations 与既有小组件**
   - 每个 manifest target 使用独立 `-c/-e` commits，production import 保持行为
     不变，并补 keyboard/a11y；
   - 初始队列：`ApprovalCard`、`AskForm`、`Popover`、`Menu`、`Lightbox`、
     `Toasts`、`DaemonAlert`；
   - target base id 采用 `99.9a`、`99.9b`……，实际 commits 为
     `99.9a-c/99.9a-e` 等，不得一次批量重写。

10. **99.10 Sidebar + Home leaf queue** — 每个 navigation/storage leaf **Gate B**
    - 先各自 characterization；
    - `ProjectRow`、`SessionRow`、`SidebarSection`、`ResizeHandle` 等逐个抽取并替换
      production import；
    - 每个 commit 只抽一个 leaf，验证排序/fold/project new chat/mobile drawer。

11. **99.11 Scheduled + Settings + Dialog leaf queue** — route/storage leaf **Gate B**
    - list/detail/row/empty/suggestion、Settings section、每种 dialog 分别提交；
    - 覆盖 active/paused/terminal/error/loading、focus return 与 deep link；
    - 不把整页拆分压成一个 commit。

12. **99.12 Timeline + Supervision leaf queue**
    - Tool/Message/Worked/Retry/Attention/Agent/Goal/Progress 分别提交；
    - closed view-model union 穷尽，开放 backend kind/status 有 unknown Story；
    - 保留 copy feedback 等局部 timer。

13. **99.13 Composer leaf queue** — storage/navigation leaf **Gate B**
    - 先锁 home/session、draft/long、recording/transcribing、attachments、goal/plan/
      automation、queue/steer、access、loading/error；
    - `DraftInput`、picker、menu、toggle、actions 等逐个抽取并替换；
    - 最后单独 commit 将 API/persistence 收进 controller；
    - 每步验证 draft、Send/clear/reload。

14. **99.14 Diff + Changes leaf queue** — storage/route leaf **Gate B**
    - toolbar/scope/file/header/unified/split/untracked/skeleton/actions 分别提交；
    - 覆盖 unavailable/unknown/non-repo/nested/conflict/empty/large/binary/untracked；
    - 真实大 diff 与 iOS 宽容器复验。

15. **99.15 Session page composition** — **Gate B**
    - `SessionPageView` 只组装 header/alerts/thread/composer/environment/changes；
    - controller 保留 poll/reconcile；
    - 验证不存在 session 时不建 EventSource/polling。

16. **99.16 CUJs**
    - 五条关键 journey 使用快速 ScenarioRunner；
    - 零 visible delay、deterministic clock、进入 CI；
    - 每条 CUJ 使用独立 suffix/commit。

17. **99.17 Core Session Demo**
    - **99.17a**：所需 seam 与 production imports 就绪后，落 Core scenario 的
      HTTP/stream/clock fixtures 与快速 assertions；
    - **99.17b**：接 autoplay/manual Canvas controls，验证
      Pause/Next/Reset/Replay；
    - **99.17c**：补 mobile、stream failure 与现场 inspection；
    - visible timing 保持 `!test`。

18. **99.18 Golden + QA-89 final sweep**
    - 固定 curated baseline 与 update/review 流程；
    - 执行完整 shared-store、route、storage、theme、mobile/WebKit 验收；
    - 记录 full/shard 时间，full gate 不得被 changed-story 取代。

19. **99.19 收口**
    - delta 只并回 DESIGN/QA/LOG；JOURNEYS/SPEC 明确无 delta；
    - 三视角复审：component boundary、state/evidence、real-env persistence/navigation；
    - P0/P1 清零后归档本文。

### 叶组件 commit 完成定义

每个 `-e` leaf extraction commit 同时满足：

1. 抽取前 characterization Story/test 已存在；
2. 只抽一个 manifest target，并由 production 真实 import；
3. DOM、文案、样式、keyboard、storage/API call shape 无意外 delta；
4. manifest cell 指向真实 `storyId`，无新增 orphan/exemption；
5. `check.sh` + `check-webui.sh` 全绿；
6. 涉及 route/storage/polling/page shell 时 Gate B 证据同 commit 入档。

## 风险与控制

| 风险 | 控制 |
|---|---|
| 工具链混装 | 全家桶精确锁版 + clean `npm ci` smoke |
| mock 通过、production 失败 | production `AppShell`/request shape 共用 + Gate B |
| HTTP/SSE 漏到真实 origin | 完整 transport contract + `/api/**` unhandled error |
| coverage 自报全绿 | typed manifest cell ↔ built `storyId` 双向 closure |
| backend 新字符串漂移 | known union + unknown truthful fallback，不伪称 backend 穷尽 |
| Reset 后旧任务写新 store | AbortSignal + generation token + remount 全部 adapters |
| autoplay/Replay 双跑 | ScenarioRunner single-owner protocol |
| Story 数量爆炸 | domain 等价类 + 明确 pairwise；golden 只给关键帧 |
| page harness 漂移 | Story 直接挂唯一 production `AppShell/page view` |
| Demo 污染产品 | development-only entry + bundle audit + 无 production branch |
| CI 太慢 | 99.1 先测预算，full shard；changed-story 只补充 |
| axe 假安全 | 显式 keyboard/focus/touch/reduced-motion/WebKit Gate |
| store factory 改坏历史数据 | storage manifest 逐 key byte compare + real restart |
| 重启伤害真实工作 | daemon restart 只在用户批准的安全窗口 |
| 一次性大改 | characterization + 单 leaf production replacement + 每步双 gate |
| 视觉变化夹带 | 默认零视觉 delta；必要变化另起产品增量 |

## review 裁决

2026-07-23 两个独立 GPT 分别从架构/实施与 Storybook/Demo/QA 视角审查，结论均为
`REVISE`，无 P0。本文已逐项吸收其 P1：

1. 改为已验证的 Storybook 10.5.3 + Vitest 4 精确组合；
2. 补齐 MSW worker/loader/static/unhandled policy 与 SSE seam；
3. 用权威 typed manifest 取代文件数/手填百分比；
4. 定义 ScenarioRunner owner/Abort/Pause/Reset/Replay 协议；
5. 把 CI、budget、golden、artifact 前置到 foundation；
6. 扩展 a11y/mobile/theme/storage/navigation Gate；
7. 将巨大 `99.3` 与功能批次拆为 store/runtime/shell/services/runner/leaf commits；
8. 修正基础设施的 JOURNEYS/SPEC 归类与 HANDA 历史数据时点；
9. 收窄 view 禁止项，保留合理局部 UI state/timer；
10. daemon restart 增加 shared-store 安全窗口。

实施完成后仍必须再做三视角 review：

1. **架构**：view/controller、services 完整性、production shell 唯一性、bundle 隔离；
2. **状态与证据**：manifest closure、unknown fallback、无 transitive 假覆盖、CUJ/Demo；
3. **真实环境**：shared-store、route、restart、legacy storage、mobile/touch/theme。

本方案不触及 DESIGN runtime/journal 不变量。若实施中必须改变薄 projection、
storage key、route 或 backend contract，立即停下，另走不变量/产品 delta 裁决。

## 参考

- HANDA：`/Users/yadong/dev2/handa/storybook/web/`
- HANDA Stories：`/Users/yadong/dev2/handa/stories/web/`
- HANDA 初始覆盖审计：
  `/Users/yadong/dev2/handa/docs/reports/storybook-coverage-audit-2026-06-13.md`
- Storybook React/Vite：
  `https://storybook.js.org/docs/get-started/frameworks/react-vite/`
- Storybook network mocking：
  `https://storybook.js.org/docs/writing-stories/mocking-data-and-modules/mocking-network-requests/`
- Vitest addon：
  `https://storybook.js.org/docs/writing-tests/integrations/vitest-addon/`
- Accessibility addon：
  `https://storybook.js.org/docs/writing-tests/accessibility-testing/`
- Vitest visual regression：
  `https://v4.vitest.dev/guide/browser/visual-regression-testing`
