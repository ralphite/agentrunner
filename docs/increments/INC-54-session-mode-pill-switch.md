# INC-54 session mode pill 点击切换（INC-42 UI 收尾）

状态：**实施中**（webui-only；后端零改动）。
认领：Claude session bug-feature-triage-08c686，2026-07-11。占号 **INC-54 · QA-51**
（fetch origin main：INC-51/52/53 已被 batch-5 占，QA 最高 QA-50）。

## 动机与 journey 锚

INC-42 已把 mode 运行中切换全链路打通（`ControlMode` durable command、
`applyModeControl` 校验、live pill、`/mode` slash），但**点击入口**缺失：
session composer 左下角的 approval-mode pill 是 `disabled` badge，title 让
用户去打 `/mode`。用户（截图指着 "Ask to approve" pill）要求「we need to be
able to change this in chat session」——把 pill 变成可点击的切换选择器。

这是 INC-42 的 UI 收尾/延伸，不是新能力：后端 ValidTransition
（default↔acceptEdits）、rejected receipt、live fold 都已就绪。

**UJ-06 delta**：无新增语义；INC-42 已补的步骤 4b（批量段切 acceptEdits/切回）
现在多一个等价手势——直接点 pill 选档，与 `/mode` 命令同一条链路。

## Spec delta

- SPEC「mode 运行中切换」行（已 ✅）锚追加 `INC-54 · QA-51`，文字
  `webui /mode+pill live 化` → `webui /mode + pill 点击切换`。功能未变，
  补一个用户入口。
- CLAUDECODE-PARITY #56：webui 侧从「`/mode`」补为「`/mode` + pill 点击」，
  更贴近对方 shift+tab 的「随时点一下就切」手势。
- GAPS G29 已关闭，无残留，不动。

## Design delta（不触不变量，后端零改动）

pill 从 read-only badge 变为 `Popover` 选择器，与 Home 档位菜单同风格
（复用 `cx-pop-codex` / `PopItem` / `PopSection` / `AccessIcon`）。

**可选项与禁用项的信息结构决策**（对齐 Home 菜单：列全 ACCESS_LEVELS，
不可选的以 disabled + 原因呈现，而非隐藏——保持结构一致 + 诚实）：

| 档位 | runtime 可切？ | 表现 | 理由 |
|---|---|---|---|
| Ask to approve（→`/mode default`） | ✅ | 可点击 | INC-42 ValidTransition |
| Auto-accept edits（→`/mode acceptEdits`） | ✅ | 可点击 | INC-42 ValidTransition |
| Full access | ❌ | disabled + desc「Set at launch — 只在 Ask↔Auto 之间切」 | full 是启动期 spec 权限姿态，runtime 只设 fold mode 不改 spec 规则 |
| Plan · read-only | ❌ | disabled + desc「exits through an approval, not this switch」 | plan 退出须 exit_plan_mode 审批 |
| bypass | —（不在 ACCESS_LEVELS） | 不列 | 仅启动时可设，与 INC-42 一致 |

- 纯函数 `runtimeModeTarget(id)` 把 access id 映射到 `/mode` 目标或 `null`
  （specs.ts），pill 据此判定可点/禁用——单测锚点（`specs.test.ts`）。
- **命令链路复用**：pill 点击与 `/mode` 共用新抽出的 `switchMode(target)`，
  跑同一条 `AR.mode` → `ControlMode` durable command，故 live fold、rejected
  receipt、toast 完全一致（不另起 API，符合任务约束）。
- **active 高亮**跟随 pill 真值序（live 确定值 > 不矛盾的 remembered > 诚实
  unknown，INC-42 文件顶注）；live unknown 时无高亮，不撒谎。
- **切换后**：pill 随 INC-42 已有的 2.5s inspect 轮询更新 live fold。
- **拒绝反馈**：非法/被拒切换落 rejected receipt → timeline chip（既有），
  toast 为投递 ack，与 `/mode` 同——用户可见反馈复用既有模式。

## 双闸门

- A（自动化）：`specs.test.ts` 新增 `runtimeModeTarget` 五条（两 clickable
  映射、full/plan 拒绝、clickable 严格子集）；tsc/build 绿；整套 vitest 绿。
- B（真机）：QA-51——共享 daemon + 真实 Gemini，运行中 session 点 pill 切
  Auto-accept edits（后续编辑免审批）、再切回 Ask to approve（审批回归），
  webui 强刷后走真用户流，截图 + ar events 归档 `qa/runs/2026-07-11-QA-51/`。

## 实施与提交

单步（webui-only）：specs.ts + Popover.tsx（PopItem disabled）+ Composer.tsx
（pill Popover + switchMode）+ styles.composer.css（.pop-item.disabled）+
specs.test.ts + dist rebuild；文档行齐活；`./scripts/check.sh` 全绿。
落地后本纸归档 `docs/archive/increments/`。
