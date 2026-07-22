# INC-96 Web UI agent 配置显式 YAML 化

> 状态：设计已裁决，待实施。

## 动机与 journey 锚

UJ-24 第 2 步已经把 agent persona 与 YAML spec 放进
`Automation → Agent`，但实现仍有两个会妨碍理解和修改的问题：

1. Dev / Team Lead / Auditor / Reviewer / Chat 的核心 prompt、tools 与编排字段
   是 `webui/frontend/src/specs.ts` 里的 TypeScript 模板字符串，不是可独立查看、
   校验和修改的 agent spec 文件；
2. `Edit agent spec (YAML)…` 打开 session 时总用默认 Dev spec，而不是该 session
   最近由 Web UI 配置的 spec；高级编辑保存后也不更新 Web UI 的 remembered spec，
   后续切 model 可能基于旧配置重写。

目标：现有 Web UI agent 各自拥有一份完整、明确的 YAML；picker 与高级编辑器都从
这些 YAML 出发，后续修改 prompt/tools/permissions 只需改对应文件，session 内编辑
继续保留用户已经配置的内容。

### UI/UX design note

- **沿用模式**：保留现有 `Automation → Agent` 五项 picker 和末尾
  `Edit agent spec (YAML)…`；不新增 Settings 页面或第二套 agent 概念。
- **提案**：默认 agent 定义移到 `webui/frontend/src/agents/*.yaml`；picker 仍提供
  model / effort / access 组合，构建时仅替换 YAML 的 `model` 与 `permissions` 块；
  高级 editor 预填当前 session remembered spec，成功切换后写回 remembered spec。
- **风险态**：YAML 仍经现有 backend `AgentSpec` strict loader 校验；无删除、覆盖磁盘
  用户文件或 silent fallback。当前 session 没有 remembered spec 时诚实回退默认 Dev。
- **数据处理**：不迁移 journal、session、shared store 或 localStorage schema；已有
  `arwebui.sessSpecs` 记录继续兼容。用户输入的 YAML 原样发送并记住。
- **未决问题**：跨 release、无需 rebuild 的全局用户 agent 库不在本增量；它需要新的
  配置发现、持久化和冲突优先级契约，应另起 journey delta，不在本次重构中暗加。

## Spec delta

- `JOURNEYS.md` UJ-24 第 2 步：agent persona 的 shipped defaults 是独立 YAML，
  高级入口编辑当前 session spec。
- `SPEC.md` Web UI progressive-disclosure composer：补显式 YAML source-of-truth、
  picker 只组合 model/access、advanced editor round-trip 当前 spec。
- `DESIGN.md` §12 Web UI：补 agent config projection 契约；YAML 定义是默认 agent
  真相源，TypeScript 不再复制 prompt/tools。

**不变量**：不触及。runtime 的 YAML → strict `AgentSpec`、SessionStarted/SpecChanged
冻结、journal-first、权限层与 sibling sub-agent 解析均不改变。

## 验收

### A 闸

- `specs.test.ts`：五个 persona 均由独立 YAML 构建；每个输出含对应 name/prompt/tools，
  model/access override 不改变 persona body；`worker.yaml` 仍为 Dev sibling。
- `Modals` component test：session agent editor 优先显示 remembered spec；保存成功后
  remembered spec 与发送内容一致。
- frontend targeted/full vitest + production build；webui Go tests；
  `./scripts/check.sh` 全绿。

### B 闸

本增量不调用 provider、不改变 backend/runtime 行为；真实 browser 使用共享
`~/.local/share/agentrunner/` 验证：

1. New session 依次选择 Dev / Team Lead / Auditor / Reviewer / Chat，YAML 对应且可开局；
2. session 内打开 YAML editor，看到当前配置，改 prompt 后 switch，再次打开仍保留；
3. 切 model 后自定义 prompt/tools 不丢；
4. session、workspace 与 journal 全部保留，证据归档 `qa/runs/<日期>-QA87/`。

## 实施步骤

1. **INC-96.0**：工作纸明确三层 delta、UI/UX 与双闸门；commit/push。
2. **INC-96.1**：拆出 YAML agent files，接入 `specs.ts`，修 advanced editor
   round-trip，补 component/unit tests；全量 A 闸；commit/push。
3. **INC-96.2**：共享真实环境 browser QA；三层/QA/LOG 收口，工作纸归档；
   check 全绿；commit/push。

## review 裁决

裁掉里程碑级三视角 review：改动是前端配置 source 重构与 editor state 修正，不改
backend schema、并发、权限判定或持久化格式。保留 UI/UX pre-review、YAML exact-output
单测、component round-trip、全量 frontend/build/check 与共享真实 browser gate。
