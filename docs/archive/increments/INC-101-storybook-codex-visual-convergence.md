# INC-101 Storybook Codex 视觉收敛

> 归档于 2026-07-23：28 个人工批准的 review family 已机械展开到全部
> 67 个 Story 文件 / 567 个 Story，QA-92 与最终门禁全绿；后续新增或修改
> Story 时由 digest drift 重新打开审计，而不是沿用旧 PASS。

**状态**：已完成并归档。

## 动机与 journey 锚

本增量承接 UJ-24「Web UI 驾驶 AgentRunner」。INC-99 已把 176 个可见
production target、562 个 Story、关键 CUJ/Demo 与浏览器 gate 接入同一工作台，
但完整审计显示“覆盖闭环”不等于“设计闭环”：

- Storybook manager 仍是默认品牌、默认蓝色选中态、onboarding 与通知噪音；
- production 仍同时依赖 raw element 全局视觉规则与新 UI primitives；
- lifecycle 状态、radius、字号、控件高度、阴影与 transition 缺少统一语义；
- 深色主题未向原生控件声明 `color-scheme`；
- 复合 Story 的局部 wrapper 很多，逐 Story 修样式会产生第二套 UI。

本次不增加终端用户 journey，也不把 Storybook 当产品功能；可见改动必须落在
production component/token 或 Storybook manager 本身，不能只美化 mock wrapper。

## 用户结果

开发者浏览任一 Story 时，工作台与组件使用同一套克制、中性、紧凑的 Codex
设计语言；running/done/waiting/attention/failed 在 Sidebar、Scheduled、Runs、
Supervision 等表面具有一致的形状、颜色、motion 与 accessible label。真实 Web UI
的核心任务、数据、route、storage、API 与键盘行为保持不变。

## 三层 delta

### Journey delta

`JOURNEYS.md` 无新增步骤。UJ-24 继续作为受保护 journey，最终仍由 shared-store
真实 Web UI 证明，不以 Story 数量或截图数量冒充用户结果。

### Spec delta

不新增产品功能行。收口时：

- `Web UI component system / Storybook workbench` 增加 INC-101 / QA-92 视觉一致性锚；
- `Web UI Codex 实窗持续证据覆盖矩阵` 保持 🟡，只按本批真正取得的双侧证据更新，
  不因 Story 审计完成而虚升 ✅。

### Design delta

在 `DESIGN.md §19` 追加 additive 的 Story-driven visual conformance：

1. Story 是 production UI 的审计单元，不是第二套实现；修复优先落 token、primitive
   或 production component，禁止 Story-only 私有美化。
2. 对照必须使用同 state、viewport、theme 的当前 Codex 与 AgentRunner 证据；
   无证据的项保留 `UNTESTED/GAP/INTENTIONAL`。
3. typography、spacing、radius、border、color、focus、motion 优先由语义 token
   和 primitive 收敛；逐 Story override 是最后手段。
4. 完成判据是 manifest/index 闭环、审计 finding 有裁决、production 真页面无回归；
   Story、截图、测试与文档只是证据。

本增量不触及 DESIGN §15 或其他粗体不变量。若需要新增 backend capability、产品动作、
storage/API/journal/session/runtime 语义，立即停下并另立产品 delta/GAPS。

## UI/UX 裁决

- **沿用模式**：保留当前 Codex 式 sidebar、thread、composer、overlay、settings 与
  scheduled 信息架构；使用现有 Phosphor icons、Tailwind 与 UI primitives。
- **本批新增**：Codex 风格 Storybook manager theme、共享几何/密度 token、
  lifecycle primitive；不新增 production route 或假控件。
- **默认状态**：扁平、中性、低噪音；hover 只改变 fill/border/text，禁用无差别
  `transition-all` 与普通按钮抬升阴影。
- **风险状态**：审批/拒绝/失败保持现有安全层级；running motion 遵守
  `prefers-reduced-motion`；focus-visible 保持单一可见蓝色 ring。
- **数据处理**：Story fixture 与 manager 配置不触达共享 store；Gate B 只读使用
  `~/.local/share/agentrunner/`，保留 session/workspace/journal。
- **未决问题**：Codex 没有证据的 surface 不猜测；本批用现有产品语义裁为
  `INTENTIONAL` 或保留未测。

## 审计基线

2026-07-23 当前 `origin/main`：

- 66 个 Story 文件、562 个 Story；
- 176 个 manifest targets、13 个 semantic states、5 个 global state pairs、
  12 个 exclusions、0 missing；
- 469 个 Story 带 `play()`，93 个为展示矩阵；
- 当前证据保存于
  `qa/runs/2026-07-23-QA-91-storybook-codex-visual-audit/`。

首个高风险硬案例：
`components-sessions-sessionview--approval-required`，
dark、390×844。它同时覆盖 header、timeline、审批安全动作、composer、暗色对比与
移动布局；改造后再用 19 步 Core Session Demo 防止只优化单帧。

最终审计账本不再只报 aggregate：`storybook-review-ledger.json` 以 28 个
visual-design / interaction-a11y / contract-evidence family 裁决为人工层，
再由 built index 与 typed manifest 机械展开全部 567 Story、177 targets /
684 cells、13 semantic assertions、5 pairs、12 exclusions。每条保留 exact
Story ID/source/target refs、八轴 review、Codex 裁决与 family source digest；
computed digest 必须等于 manifest 中人工批准 digest，baseline update 不会改
批准值；任何新 Story、target、state 或源码变化都会让 `lint:storybook` 报 stale，且
`REVIEWED` 不等于 automated gate PASS 或 Codex parity PASS。

## 实施批次

1. **101.1 工作台与 foundations**
   - Storybook manager 使用 AgentRunner/Codex 中性色、品牌和密度；
   - 隐藏 onboarding/what's-new 噪音，保留搜索、taxonomy、toolbar、addons；
   - 添加语义 radius/type/control/shadow tokens；
   - explicit light/dark/system 同步原生 `color-scheme`；
   - 移除普通 button 的 `transition-all` / hover shadow，保持 overlay/composer
     等明确 elevation。

2. **101.2 lifecycle consistency**
   - 建立 `LifecycleStatus`：running spinner、done check、waiting ring、
     attention amber、failed red；
   - 迁移 Sidebar、Runs、Scheduled、Supervision/Subagents 的重复状态拼装；
   - visible copy 与 accessible label 分离，避免 sidebar spinner 文案占据标题宽度。

3. **101.3 Story review surface**
   - 对复合 Story 使用共享 `component/panel/overlay/full-page/mobile-sheet`
     Canvas preset，先覆盖高维护量族，不复制 production layout；
   - 统一人类可读 taxonomy，修正会误导 Docs/Controls 的 harness component；
   - 增加少量 Foundations/Review gallery，保留原 562 个状态 Story。

4. **101.4 真实对照与收口**
   - 扫描 Foundations、Sidebar、Home/Composer、Thread/Timeline、Environment、
     Changes、Scheduled、Settings/Dialog；
   - desktop/mobile/short、light/dark/system、keyboard/focus、touch、reduced motion、
     overflow；
   - shared-store 真页面验证 route/reload/storage 行为与 console；
   - fresh agents 分别做 visual/design、interaction/a11y、contract/evidence review，
     修清 P0/P1 后再收口。

## QA-92 验收

### Gate A：Storybook / production

- manifest target × semantic state × built index 为 0 missing、0 orphan；
- 每个 target 有当前 Codex evidence/parity 裁决或明确 `INTENTIONAL/GAP/UNTESTED`；
- Foundations 与高风险硬案例完成同 viewport/theme 前后对照；
- keyboard/focus、contrast、touch target、reduced motion、overflow 无回归；
- Story 与 production 复用同一 component；production bundle 无 Story/mock assets；
- 最终统一运行 `check-webui.sh` 与 `check.sh`，中途不反复消耗全量 unit gate。

### Gate B：真实 shared-store Web UI

- 使用默认 `~/.local/share/agentrunner/` 与全局 daemon；
- Home/Session/Changes/Scheduled/Settings 在 desktop/mobile、light/dark 可达；
- route/reload、sidebar/composer/overlay、focus return 与 storage 行为不变；
- browser console 无 product warning/error，body 无意外 overflow；
- 测试数据、workspace、journal 全部保留，不 close/delete/cleanup。

证据保存到：
`qa/runs/2026-07-23-QA-92-storybook-codex-visual-convergence/`。

## review 裁决

实施前已并行完成 Story inventory、design-system、文档 delta 与真实截图审计：
Story 架构无结构性缺口，P1 集中在 manager 默认外壳、全局样式/primitives 双轨、
lifecycle 语义和缺失的几何 token。实现完成后必须由 fresh-context reviewer 重看
authoritative code、最新截图与 diff，不能把绿色测试或截图数量当作设计完成。

### 收口时 Gate B 证据边界

本批没有部署或重启 production 8809：该进程当时仍由并发 session 使用旧 bundle，
强行替换会打断真实工作。current-source 只在 5199 通过既有 8788 backend 接入全局
daemon 与 `~/.local/share/agentrunner/`，验证 desktop Home、retained Session、
Scheduled list/detail/direct reload、Settings light/dark 与 console；8809 只作旧部署
非空 spot check。shared-store mobile、current-source Changes、真实 OS
reduced-motion、storage 全 key 与 production deploy/restart 明确为本批
`UNTESTED`，不升级 `CODEX-PARITY` 的任何 PASS。

接受这一证据边界的理由是：本增量不改 route、storage、API、journal 或 backend，
移动审批与 full-page shell 已由 390×844 Story、44px DOM geometry、interaction/a11y
gate 覆盖，跨平台 visual CI 由最新 main 提供。上述未测项继续由 QA-88/89 的既有
产品 gate 与后续同态证据循环承接；本批只声称 design-system/Storybook 收敛及
shared desktop spot check，不声称完成全产品 parity。

## 完成记录

- 最终基线：`origin/main@db2bdffc62ea`；同步后重新生成
  `storybook-baseline.json`、missing baseline 与 tracked review ledger。
- 审计闭环：28 个三角色 family、67 个 Story 文件、567 个 exact Story、
  177 targets / 684 cells（618 covered、66 N/A、0 missing）、
  13 semantic assertions、5 global pairs、12 exclusions。
- 人工批准 digest 覆盖 family 裁决、全部 inventory 分类、Story source、
  target production source 与共享 foundations；baseline update 无权改批准值。
- fresh contract/evidence、interaction/a11y、visual/design reviewer 最终均为
  PASS，P0/P1=0。
- `./scripts/check-webui.sh --skip-install && ./scripts/check.sh` 最终同轮全绿：
  86 files / 832 unit、65 passed + 2 skipped Story files / 564 interaction、
  18 visual、production/Storybook build、Storybook lint、Codex capture、
  Go lint/wiring/test/install 全部通过。
- shared QA 继续保留；没有 restart/kill daemon，也没有创建、关闭、删除或清理
  session/workspace/journal。未测边界继续按上节记录，不以自动门禁虚升 parity。
