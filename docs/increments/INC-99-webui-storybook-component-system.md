# INC-99 Web UI 组件体系与 Storybook 工作台

**状态**：设计候选，等待实施裁决。本文只定义架构、覆盖契约与迁移顺序；
不改变当前产品行为，不代表 Storybook/组件重构已经落地。

## 动机与 journey 锚

### 用户结果

锚定 `UJ-24 Web UI 驾驶 AgentRunner`：后续新增或修改 Web UI 时，开发者能在
不启动 daemon、不制造 session、不碰共享数据的前提下，直接浏览每个可见组件与
完整页面的真实状态，重放关键交互，并在提交前发现布局、响应式、主题、键盘、
可访问性和状态组合问题；最终仍以 shared-store 真 Web UI 证明真实 journey
没有回归。

这不是一轮视觉改版。当前 Codex 式信息架构、文案、动作层级、路由、持久化 key、
backend API、journal 语义和 shared-store 数据都保持不变。组件重构的第一目标是
让现有行为更容易独立开发、验证和交付。

### 当前问题

AgentRunner 已经有 React 组件与大量 DOM 测试，但「文件级组件」仍同时承担数据
获取、全局 store、持久化、计时、状态投影和多个独立 surface。当前基线：

- `webui/frontend/src/components/` 有 36 个生产 `.tsx`，共 14,650 行；
- 9 个组件超过 500 行，其中：

| 组件 | 行数 | `AR.*` 调用 | `useStore` | `useState` | effect |
|---|---:|---:|---:|---:|---:|
| `Composer` | 2101 | 23 | 4 | 37 | 15 |
| `DiffView` | 1800 | 11 | 6 | 21 | 10 |
| `SessionView` | 1489 | 23 | 4 | 31 | 9 |
| `Timeline` | 1451 | 0 | 0 | 12 | 6 |
| `Scheduled` | 1033 | 8 | 2 | 9 | 4 |
| `SupervisionPanel` | 994 | 8 | 7 | 6 | 5 |
| `Modals` | 964 | 11 | 13 | 31 | 4 |
| `Sidebar` | 889 | 3 | 3 | 10 | 3 |
| `ChangesOutcome` | 588 | 7 | 5 | 13 | 3 |

- 现有约 761 个 frontend `test()/it()`，擅长机制与 DOM contract，但没有可浏览的
  组件目录、统一状态矩阵、完整页面 fixture 和连续 journey playback；
- 真实 QA 已覆盖 desktop/mobile、light/dark、shared-store 与大量特殊状态，但
  触发成本高，不适合作为日常组件开发工作台；
- 当前没有 Storybook 配置、Story 文件或自动化 Story inventory。

### HANDA 实践结论

HANDA 当前 Storybook 的有效做法：

1. `Base → Components → Panels → Pages → CUJs → Demos → Future` 分层，
   把单组件状态、页面组装、内部回归 journey、对外演示和未来 mock 分开；
2. 生产组件由 Story 直接 import，共享 `storyStates.ts` fixture，theme 由全局
   decorator 切换；
3. `@storybook/addon-vitest` + Playwright Chromium 在真实浏览器中跑 Story；
4. `play()` 通过 `step()`、role/test-id 与断言重放交互；
5. `visibleInteractions.ts` 给演示动作增加人眼可见停顿；
6. 单独维护 `Component × State` 与 `Workflow × Journey` 审计，避免只凭 Story
   数量宣称覆盖。

同时必须吸收 HANDA 暴露出的限制：

- Storybook 不自动带来组件化；HANDA 仍有三个约 1200 行的大组件；
- 72 个 Story 文件、467 个 Story export 后，仍需要人工审计才发现最初只有
  72% applicable state cell 覆盖，responsive/dark 与连续 journey 最弱；
- page harness 复制 `App.vue` 的状态/布局逻辑，可能 Story 通过而生产 App 漂移；
- `ProductShowcase` 是静态注释页，不是自动/手动 playback；
- 可见延时与测试共用会拖慢 CI；`Future` mock 若不隔离会造成“已有产品能力”的
  假象；
- transitive render 不等于组件有直接 Story，重复 Story 也不等于新增覆盖。

因此 AgentRunner 采用 HANDA 的分层、fixture、browser play 与覆盖矩阵，但改进为：
**production shell 直接可挂载、Story 与组件 colocate、状态目录类型约束、快速测试
与慢速演示分离、Future 不计覆盖、真实环境作为最终闸门。**

## Goals

1. 将页面拆成可复用、可独立渲染的 view component；数据/副作用留在
   controller/hook。
2. 每个 user-visible component 都有直接 Story；所有适用状态都有 Story 或显式
   `N/A` 理由。
3. 形成 `Foundations → Components → Features → Pages → CUJs → Demos` 组件体系。
4. Story 在 Playwright Chromium 中成为可执行测试，覆盖 render、interaction 与
   a11y。
5. 提供一个使用真实 production components、mock API/store、可
   `Play / Pause / Next / Reset` 且可 autoplay 的核心 Demo。
6. 组件重构期间保持 DOM/文案/产品行为和 shared-store 数据兼容；不以 clean mock
   Storybook 代替真 Web UI QA。

## Non-goals

- 不重做视觉语言，不引入另一套 design system；
- 不在 production Web UI 增加 Demo route、mock 开关或 Story-only prop；
- 不改变 backend API、journal、session/runtime、hash deep link 或 localStorage key；
- 不把所有状态做笛卡尔积截图；覆盖的是所有**可达、可区分、会影响行为或布局**
  的状态；
- 不把 speculative `Future` mock 计作产品/测试覆盖；
- 首轮不接 Chromatic 或其他外部 SaaS；是否引入云视觉基线另行裁决。

## Spec delta

实施收口时在 `SPEC.md §I` 新增：

| 功能点 | 状态 | Journey | 验收锚 |
|---|---|---|---|
| Web UI component workshop（组件直接 Story、状态目录、browser interaction/a11y、Pages/CUJs、可控 Demo） | 实施中为 🟡，全闸关闭后 ✅ | UJ-24 | `test:storybook`、Story inventory/state audit、QA-89 |

既有 Web UI 产品面条目不拆、不改行为，只补 Storybook/组件工作台作为交付质量锚。

## Design delta

在 `DESIGN.md § Web UI 产品 surface` 追加以下 additive 设计，不修改粗体不变量：

1. `webui/frontend` 分为 production runtime、可复用 view、feature controller 与
   Storybook development-only assets；Story/fixture 不进入 Go embed 的 production
   bundle。
2. `webui/` 继续是公开 `ar`/daemon/journal 的薄 projection。Story fixture 只是测试
   输入，永远不是产品状态真相。
3. view component 不直接读 `AR`、Zustand singleton、localStorage、location 或 timer；
   controller/hook 负责副作用，再以 typed state/callback 传给 view。
4. production 与 Storybook 共用同一 `AppShell`/page view，不允许 Story 复制一份页面
   layout。
5. store 支持 factory + Provider，使每个 Story/test 有独立实例；production adapter
   继续使用原持久化 key 和原行为。
6. 完整页面 Story 在 HTTP 边界 mock API；leaf component 直接以 props/state 渲染。
   禁止 `if (storybook)` production branch。

以上不变量仍原样成立：

- Web UI 不复制 session 状态机或建立第二套运行真相；
- responsive 只改可见性，不改状态、deep link 或 command；
- UI 重构不迁移/删除用户偏好、session、workspace 与 QA 数据。

## 目标架构

```text
webui/frontend/
  .storybook/
    main.ts
    preview.tsx
    vitest.setup.ts
  src/
    app/
      AppRuntime.tsx        # polling、route、notification、production adapters
      AppShell.tsx          # 唯一 production 页面 layout，可独立挂载
      AppProviders.tsx
    ui/                     # 无 AR/store 的基础可见组件
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
      fixtures/             # typed builders；不复制 backend state machine
      handlers/             # MSW v2 HTTP handlers
      scenarios/            # CUJ/Demo deterministic scripts
      StoryFrame.tsx        # theme/viewport/portal/root helpers
      storyInventory.ts     # component/state/exemption catalog
```

Story 与 production component colocate，例如：

```text
features/composer/
  ComposerController.tsx
  ComposerView.tsx
  ComposerView.stories.tsx
  ComposerView.test.tsx
```

### 边界规则

- `ui/**`：只依赖 React、icons、CSS token 与纯类型；禁止 `AR`/store。
- `features/*/components`：typed props + callbacks；禁止直接 fetch。
- `features/*/controller`：允许 `AR`、store、effect、optimistic reconciliation。
- `pages/**`：组装 feature，不重写 feature 状态机。
- `AppRuntime`：唯一全局 polling/hash/notification owner。
- Story fixtures 使用 production `types.ts` 与 view-model builders；backend contract
  变化时 TypeScript 必须使 fixture 编译失败，而不是静默漂移。

首轮不引入通用 DI framework。只增加三个必要 seam：

1. `createAppStore()` + `AppStoreProvider`；
2. `ApiClient`/HTTP mock boundary；
3. clock/id 的测试 adapter（仅有 timer/id 行为的场景使用）。

## 组件拆分地图

拆分遵循“先 characterization Story，后抽 leaf，再换 production import”。不做一次性
目录搬家。

| 当前组件 | 目标 production 组件 |
|---|---|
| `App` | `AppRuntime`、`AppShell`、`GlobalShortcuts`、`MobileNavigation` |
| `Composer` | `ComposerView`、`ProjectContextBar`、`DraftInput`、`AttachmentStrip`、`AddMenu`、`AccessPicker`、`ModelPicker`、`DeliveryToggle`、`GoalOptions`、`ComposerActions`；网络/持久化留 `ComposerController` |
| `Sidebar` | `SidebarShell`、`PrimaryNav`、`SidebarSection`、`ProjectGroup`、`ProjectRow`、`SessionRow`、`SidebarFooter`、`SidebarResizeHandle` |
| `SessionView` | `SessionPageView`、`SessionHeader`、`SessionAlerts`、`ThreadPane`、`CompactGoalBar`、`EnvironmentOverlay`、`ChangesLayout`；poll/reconcile 留 controller |
| `Timeline` | `TimelineView`、`UserMessage`、`AssistantMessage`、`MessageActions`、`ToolCard`、`ToolDetail`、`WorkedFold`、`RetryFold`、`TimelineSkeleton`、`JumpToLatest` |
| `DiffView` | `DiffPageView`、`DiffToolbar`、`DiffScopePicker`、`FileDiffCard`、`FileHeader`、`UnifiedDiffBody`、`SplitDiffBody`、`UntrackedFileCard`、`DiffSkeleton` |
| `ChangesOutcome` | `ChangesSummaryCard`、`ChangedFileList`、`ChangesActions`；refresh/mutation 留 controller |
| `SupervisionPanel` | `EnvironmentPanel`、`GoalSection`、`ProgressSection`、`AgentsSection`、`AttentionSection`、`BackgroundSection`、`ArtifactsSection`、`EnvironmentSection` |
| `Scheduled` | `ScheduledPageView`、`ScheduleFilterBar`、`ScheduleRow`、`ScheduleList`、`ScheduleDetailPanel`、`ScheduleEmptyState`、`ScheduleSuggestions` |
| `Modals` | 通用 `Dialog` 外壳 + `NewSessionDialog`、`RunDialog`、`ForkDialog`、`AgentDialog`、`ConfirmDialog`、`PromptDialog`、`TrustDialog`、`RenameDialog`、`ViewerDialog` |
| `Settings` | `SettingsShell` + 既有 section；保留 desktop Done/mobile Back 单入口规则 |

不以行数作为唯一成功判据，但目标是：

- orchestrator 只组装，不再内嵌多个独立 surface；
- leaf/view 通常不超过约 300–400 行；
- 同一文件不再同时承担 fetch + persistence + projection + 五个以上独立 UI 区块；
- `ui/**` 与 view 层 `AR/useStore` 直接依赖为 0。

## Story 体系

### 分层

1. `Foundations`：token、typography、button、icon button、badge/pill、spinner、
   skeleton、menu/popover/dialog、tooltip/focus。
2. `Components`：直接、可复用 leaf/view。
3. `Features`：一个完整功能区，允许 stateful harness，但不含 page routing。
4. `Pages`：production `AppShell`/page view 的完整静态状态。
5. `CUJs`：快速、确定性、带断言的连续用户旅程。
6. `Demos`：人眼可见 playback，可 autoplay/手动控制，默认 `!test`。
7. `Future`：若存在必须 `!test`、`experimental`，不进入 coverage/产品结论。

### 全局状态维度

每个可见 component 在适用时覆盖：

| 维度 | 必须枚举 |
|---|---|
| 数据生命周期 | initial/loading/ready/empty/error/retry |
| 交互 | idle/hover/focus/open/disabled/submitting/success/failure |
| 密度 | minimal/populated/long-content/overflow |
| 响应式 | desktop `1280×720`、phone `390×844`；overlay/composer 另测 short `390×500` |
| 主题 | light/dark；system 只在 theme/persistence journey 单独覆盖 |
| 可访问性 | keyboard path、Escape、focus return、role/name、touch target、reduced motion |
| domain | 每个 status/mode/kind/capability union 的所有可达值 |

避免笛卡尔爆炸，采用三条规则：

1. 每个 domain enum/union 值必须有 Story；
2. 每个适用全局维度至少有一条直接 Story；
3. 高风险组合必须成对覆盖：`error+dark`、`long+mobile`、
   `overlay+short viewport`、`loading+reload`、`attention+mobile`。

### 状态目录与防漂移

- 每个 domain fixture catalog 使用
  `satisfies Record<DomainState, Fixture>`；新增 union 值时 TypeScript 直接报漏项。
- 每个 user-visible production component 必须有直接 `.stories.tsx`；transitive render
  不计覆盖。
- 允许 `story-exemptions`，但只用于非可见 adapter/helper，必须写 reason；不能把难测
  component 例外掉。
- `scripts/lint-storybook.mjs` 从 source + built Storybook `index.json` 检查：
  direct story、合法 taxonomy、重复 id、orphan Story、exemption 与 `Future/Demos`
  test tag。
- 覆盖报告由脚本生成，不手工维护百分比；报告状态只有
  `COVERED / N-A / MISSING`。

## Demo 设计

### 位置与边界

Demo 只住 `Demos/Core Session Playback`，不进 production route。它使用：

- production `AppShell`、page、feature 与 leaf components；
- 独立 `createAppStore()`；
- MSW/mock API 的确定性响应；
- 内存 localStorage/location adapter；
- 固定 clock/id；
- 真实 DOM click/type/keyboard，而不是直接改 React state。

禁止复制 production layout，也禁止在 production component 加
`demoMode/isStorybook` 分支。

### 首个 scenario

`Core session → completion → review`：

1. Home 空态；
2. 选择真实形态的 project；
3. 点击 Build starter 与具体 suggestion；
4. 展开 access/model 并确认；
5. Send；
6. session loading → user message → running tool → streamed assistant；
7. Environment 出现 progress/agent/attention；
8. turn 完成并显示 Worked/Changes；
9. Review 打开 Changes；
10. mobile/desktop 各验证一次关闭与返回。

第二批 scenario：

- `Queue → Steer → Stop → Retry`；
- `Approval + structured Ask`；
- `Parent → child → answer/approval → parent`；
- `Scheduled list → detail → pause/resume → reload`；
- `draft/deep-link/theme persistence`。

### Playback controller

scenario 是 typed `DemoStep[]`，每步包含：

- `id/label`；
- user-level action；
- checkpoint assertion；
- 可选 narration；
- timeout/failure message。

Canvas 内提供 `Play / Pause / Next / Reset`、速度 `1×/2×` 与当前步骤。Story
args 提供 `mode: "manual" | "autoplay"`：

- `autoplay` 在 Story 打开后从 clean fixture 开始；
- `manual` 默认停在 step 0，由用户逐步播放；
- Storybook Interactions 面板仍可 Replay；
- failure 停在现场并显示 step/error，不自动跳过。

`play()` 测试版与演示版复用同一 scenario/actions，但 timing 分离：

- `CUJs` 使用零/极短延时 + fake clock，进入 CI；
- `Demos` 使用 visible timing，标记 `!test`，不拖慢 CI。

## 验收

### Gate A：确定性自动化

基础命令：

```bash
cd webui/frontend
npm ci
npm run test
npm run build
npm run build-storybook
npm run test:storybook
```

Storybook 使用当前稳定 `10.4.x` 的 `@storybook/react-vite` 与
`@storybook/addon-vitest`；当前 React 18、Vite 7、Vitest 3、Node engine 满足官方
要求。另加 `@storybook/addon-a11y`；MSW 必须为 v2。

自动化判据：

- production build 与 Storybook static build 均成功；
- 所有 `test` Story 在 Playwright Chromium render；
- 所有 `play()` assertion 通过；
- `a11y` serious/critical violation 为 0；例外必须逐条记录，不能全局关闭；
- story inventory/state audit 无 `MISSING`；
- `Future/Demos` 不误入测试覆盖；
- production bundle 不包含 Story/fixture/MSW；
- 现有 frontend tests 不减少、不改成只测 snapshot。

视觉回归首轮不引外部 SaaS。只对下列 `golden` Page/CUJ Story 使用本地
Playwright screenshot baseline：

- Home；
- completed/running/attention Session；
- Changes；
- Scheduled list/detail；
- Settings；
- desktop/mobile × light/dark 的关键组合。

组件全部跑 browser render/a11y，像素 baseline 只覆盖稳定关键帧，避免 400+ Story
造成不可维护的截图噪声。

### Gate B：真实 shared-store Web UI

任何涉及 store factory、route、polling、localStorage、page shell 或大组件切换的步骤，
都必须在真实 `~/.local/share/agentrunner/` 与真实 production URL 验证：

1. 现有 Home/session/Scheduled/Settings/Changes 可打开且非空；
2. 既有 completed/running/failed/attention/parent-child/scheduled session 正确；
3. deep link、browser Back、reload、普通 daemon/webui restart 后保持；
4. text draft、theme、sidebar width/fold、pin/archive/project overlay key 不迁移、不丢；
5. desktop `1280×720`、phone `390×844`、short phone `390×500`；
6. light/dark；
7. console warning/error 为 0；
8. 至少驱动一个本步受影响的真实 control；
9. session/workspace/journal/QA 数据全部保留，不 close/delete/cleanup。

证据进 `qa/runs/<日期>-QA-89-webui-storybook-components/`。

### QA-89 菜单

实施时在 `QA.md` 新增：

- A：Story inventory + state matrix + CUJ + a11y + golden screenshots；
- B：shared-store production 的 Home/Session/Changes/Scheduled/Settings 全链；
- C：reload/restart/deep-link/persisted preferences；
- D：本次抽取前后同 fixture/同 viewport 比对；
- E：Demo autoplay 与 manual `Next/Reset`；
- F：production bundle 无 mock/Story。

## 实施步骤

每步均是独立可合并提交；行为重构步骤必须同 commit 带 Story、现有 test 与 QA 锚。

1. **99.1 Storybook foundation**
   - 安装并固定 Storybook `10.4.x` React/Vite、Vitest addon、a11y、Playwright、MSW v2；
   - 配置真实 `tw.css`、theme toolbar、portal root、viewport；
   - 加 build/test scripts 与最小 smoke Story；
   - production code/行为零改动。

2. **99.2 State catalog + inventory**
   - 建 typed fixture builders、Story taxonomy、exemption 与 audit script；
   - 产出当前 36 个可见组件的 baseline matrix；
   - 不用百分比掩盖 `MISSING`。

3. **99.3 Production `AppShell` seam + first Demo**
   - 从 `App` 抽唯一 `AppShell`，`AppRuntime` 保留 route/poll/notification；
   - 建独立 store factory，不改变 production key；
   - 用真实 `AppShell` 做 `Core session` autoplay/manual vertical slice；
   - 先得到一条可工作的端到端演示，再继续横向拆组件。

4. **99.4 Foundations + existing small components**
   - 固化现有 button/icon/status/menu/popover/dialog 视觉 pattern，不改样式；
   - 为 `ApprovalCard/AskForm/Popover/Menu/Lightbox/Toasts/DaemonAlert` 等直接补 Story；
   - keyboard/focus/a11y 先建立范例。

5. **99.5 Sidebar + Home**
   - 先 Story 锁 current/hover/focus/running/unread/attention/project actions；
   - 再抽 `ProjectRow/SessionRow/SidebarSection/ResizeHandle`；
   - shared-store 验证排序、fold、project-scoped new chat、mobile drawer。

6. **99.6 Scheduled + Settings + Dialogs**
   - list/detail/mobile full-surface、paused/active/terminal/error/loading 全状态；
   - 每种 dialog 独立 Story；
   - 保持 desktop Done/mobile Back 与 deep-link contract。

7. **99.7 Timeline + Supervision**
   - 从纯投影 leaf 开始拆 Tool/Message/Worked/Retry/Attention/Agent/Goal/Progress；
   - typed event/status fixture catalog；
   - parent/child/approval/ask/combined attention CUJ。

8. **99.8 Composer**
   - characterization Story 先覆盖 home/session、empty/draft/long、recording/
     transcribing、optimize/undo、attachments、goal/plan/automation、queue/steer、
     access escalation、loading/error；
   - 再逐 leaf 抽取，最后把 API/persistence 收进 controller；
   - 真机验证 draft 与 Send/clear/reload。

9. **99.9 Diff + Changes**
   - working-tree/last-turn unavailable、unknown/non-repo/nested/worktree/conflict、
     empty/large/binary/untracked、split/unified/wrap、desktop/mobile；
   - 抽 toolbar/file/body；
   - 对既有真实大 diff 与 iOS 宽容器复拍。

10. **99.10 Session page + CUJs**
    - `SessionPageView` 只组装 header/alerts/thread/composer/environment/changes；
    - controller 保留 poll/reconcile；
    - 落五条关键 CUJ 快速 play。

11. **99.11 Gates + QA-89**
    - Story build/test/a11y/inventory 进 CI；
    - curated golden screenshots；
    - shared-store production 全链与 restart；
    - 记录时间成本，必要时 changed-story fast gate + full main gate 分层，不能静默跳过。

12. **99.12 收口**
    - delta 并回 JOURNEYS/SPEC/DESIGN/QA/LOG；
    - 三视角 review：component boundary、state/evidence completeness、real-env
      persistence/navigation；
    - P0/P1 清零后归档本文。

## 风险与控制

| 风险 | 控制 |
|---|---|
| mock 通过、production 失败 | `AppShell` 共用 + Gate B shared-store；Storybook 不算最终闸门 |
| store factory 改坏持久化 | production adapter 保留 exact key；legacy data/reload/restart 真验 |
| Story 数量爆炸 | domain 全枚举 + global pairwise；像素 baseline 只给 golden；审计 redundancy |
| Story 漂移 | colocate + typed fixture + inventory lint；禁止 transitive coverage |
| page harness 复制 production | Story 直接挂 production `AppShell/page view` |
| Demo 代码污染产品 | development-only entry；bundle 检查；无 `demoMode` production branch |
| 可见延时拖慢 CI | CUJ fast clock 与 Demo visible timing 分离；Demo `!test` |
| 一次性大改难回滚 | strangler extraction；一步一 commit；每步 production 可运行 |
| 视觉改动混入重构 | extraction commit 默认零视觉 delta；必要视觉变化单独增量裁决 |

## review 裁决

这是 milestone 级前端架构增量，不能裁掉三视角 review。实施前后都要检查：

1. **架构视角**：view/controller 边界、production shell 唯一性、bundle 隔离；
2. **状态与证据视角**：domain 状态穷尽、无 transitive 假覆盖、CUJ 与 Demo 可重放；
3. **真实环境视角**：shared-store、deep link、restart、legacy localStorage 与移动端。

本方案不触及 DESIGN runtime/journal 不变量；若实施中发现必须改变薄 projection、
持久化 key、route 或 backend contract，立即停下，另走不变量/产品 delta 裁决，不能
借“组件化”顺手改变语义。

## 参考

- HANDA：`/Users/yadong/dev2/handa/storybook/web/`、
  `/Users/yadong/dev2/handa/stories/web/`、
  `docs/reports/storybook-coverage-audit-2026-06-13.md`
- Storybook 10.4 React/Vite：
  `https://storybook.js.org/docs/get-started/frameworks/react-vite/`
- Vitest addon：
  `https://storybook.js.org/docs/writing-tests/integrations/vitest-addon/`
- Accessibility addon：
  `https://storybook.js.org/docs/writing-tests/accessibility-testing/`
