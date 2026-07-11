# INC-42 mode 运行中切换（default↔acceptEdits 用户命令，G29）

状态：**drafted，待裁决**（PROCESS §二第 3 步——开发者确认后进实施）。
认领：Claude session-permission-mode-83ea5f，2026-07-10。QA 号占用：**QA-44**
（QA-43 已被 INC-41 终局全景对照预订）。

## 动机与 journey 锚

GAPS **G29**（2026-07-10 复盘登记）：v1 PLAN S3.6c 白纸黑字的三条跃迁边
里，default↔acceptEdits（用户命令）从未接线——`pipeline.ValidTransition`
零生产调用方，daemon/CLI/webui 三入口皆无。本增量接上这条边。

用户手势：执行进入机械批量段（改一批文件）→ 切 acceptEdits 免逐次审批
→ 批量段结束切回 default。对标 Claude Code shift+tab 随时切换。

**UJ-06 delta**：步骤 4 后增补 4b——「批量改动阶段，用户把 session 切到
acceptEdits：后续 edit 免审批直落，execute 与 protected 写仍走审批；批量
段结束切回 default。两次切换都作为事件进时间线。」覆盖功能标签增
`mode 运行中切换(用户命令)`。

## Spec delta

- SPEC D「mode 运行中切换」**❌→✅**，锚 `TestModeControl*` + QA-44。
- CLAUDECODE-PARITY #56「运行中切换 ❌（G29）」→ ✅（`ar mode`/webui
  对应对方 shift+tab；plan/bypass 仍仅启动时，与对方一致处/差异处记档）。
- GAPS G29 关闭注记；`scripts/deadcode-baseline.txt` 移除 ValidTransition
  行（lint-wiring 会强制）。

## Design delta（不触不变量）

- **跃迁触发器集合成文为三个**：startup / exit_plan_mode 审批
  （plan→default）/ **mode control（user，default↔acceptEdits）**。
  落点：DESIGN 手动 compact/clear bullet 的 control 家族加 mode 一行；
  §18.2「control 输入」示例补 mode；§3.6 modes bullet 补一句
  「default↔acceptEdits 的用户命令触发器 = mode control」。
- **不变量核对（为什么不触）**：决策 #10——mode 只过滤 **permitted 面**，
  advertised 面 session 内稳定。default 与 acceptEdits 两侧 advertised 面
  相同、prompt suffix 相同（仅 plan 有注入）→ 零 prefix 影响、零缓存
  影响，无需 SpecChanged 式显式换代。gate 侧零改动：effect 随身携带
  live fold mode（`permission.go` effectiveMode 明言 "live fold state —
  the mode can change"），切换后续 effect 自动按新 mode 判。
- **语义边界（全部维持现状）**：
  - `ValidTransition` 表不变：runtime 仅 default↔acceptEdits；plan 只能
    经 exit_plan_mode 审批退出；**bypass 永不可 runtime 进入**（CLI 启动
    flag 专属，安全红线）。
  - 子 agent frozen-at-spawn（loop.go 决策）不变：切换只影响**后续**
    spawn 的 parentMode；在跑子树不受影响。
  - protected 写保护（INC-18）不变：acceptEdits 下敏感路径写仍 ask。
  - hooks：mode control 不挂 hook 事件（无可否决面——权限收放是用户
    主权操作；lifecycle observe 挂载记余项，不在本增量）。

## 机制

- `protocol.ControlMode = "mode"`，复用 `Control.Directive` 载目标 mode。
- daemon `case "mode"` → `handleControl(Control{Kind:ControlMode,
  Directive:cmd.Directive}, "mode change requested — the journal records
  the outcome", enc)`——与 compact/clear/remember 同 durable command /
  恰好一次 / revive 非托管 session 语义。
- `drainControls`（compaction.go）加 `case protocol.ControlMode` →
  `l.applyModeControl(ds, ctlAppend, ctl.Directive)`：
  - target 归一（""/缺省报错，"default"/"acceptEdits" 进表）；
    `pipeline.ValidMode` + `ValidTransition(CurrentMode 归一, target)`；
  - 合法且 ≠ 当前 → append `ModeChanged{To:target, Cause:"user"}`（fold
    `s.Mode=p.To` 已有）+ live emit `protocol.Event{KindModeChanged}`
    （replay 投影 replay.go 已有，live 路径对齐）；
  - 非法或同值 → append `CommandHandled{Result:"rejected: <from>→<to>"
    或 "no_op"}`——显式 receipt，审计可见、CLI 可读；
  - 生效时机 = drainControls 双路（安全边界/待命）,与 steering 同语义,
    不打断在飞 activity。
- CLI `ar mode <sid> <default|acceptEdits>`（镜像 `ar remember` one-shot
  实现；help 明示 plan/bypass 仅启动时可选）。
- webui：`composer_api.go` oneShotHandler 加 "ar mode"（带 arg）；
  Composer SLASH session variant 加 `/mode <default|acceptEdits>`；
  session 权限 pill 数据源取 inspect 的 `CurrentMode`（inspect.go 已
  serve）并随 mode_changed 事件刷新——**删除 "the session's fixed
  approval mode (display only)" 注释**（G29 事故遗迹）。
- 渲染：CLI `render.go` KindModeChanged 已有；webui timeline
  mode_changed 投影核对/补齐。

## 验收

孪生（闸门 A，进 check.sh）：
- `TestModeControlSwitchesToAcceptEdits`：default 会话投 mode control →
  `ModeChanged{Cause:"user"}` 落 journal → 后续 edit-class effect 的
  modeDefault=allow（EffectResolved 证）；
- `TestModeControlSwitchBack`：acceptEdits→default，edit 回 ask；
- `TestModeControlRejectsInvalid`：目标 bypass/plan、plan 会话目标
  acceptEdits → CommandHandled rejected、`s.Mode` 不变；
- `TestModeControlDurableIdempotent`：同 command_id 重放不双改（与
  TestRememberIdempotentCommand 同构）；
- `TestModeChangedAttachProjection`：attach/replay 投影 KindModeChanged；
- crash 注入：ModeChanged 落盘后崩溃 → resume 后 CurrentMode=目标值
  （fold 纯性）。

QA-44（闸门 B，真实 API；**私有新二进制 daemon**——daemon-path 功能）：
1. default：让模型改文件 → 审批 ask（不批，留 pending）；
2. `ar mode <sid> acceptEdits` → journal 见 mode_changed(cause=user)；
3. 同类 edit → 免审批直落、文件真实变更；
4. protected 路径写（.mcp.json）→ 仍 ask（INC-18 红线不松）；
5. `ar mode <sid> bypass` → rejected receipt、mode 不变；
6. `ar mode <sid> default` → edit 回 ask。
webui 复核：pill 随切换更新、`/mode` 可用。export 归档 `qa/runs/`。

`./scripts/check.sh` 全绿（含 lint-wiring：ValidTransition 移出基线——
接线的机械证明）。

## 实施步骤

1. **INC-42.1** protocol.ControlMode + drainControls/applyModeControl +
   孪生前四条——一提交；
2. **INC-42.2** daemon case "mode" + CLI `ar mode` + attach 投影/crash
   孪生——一提交；
3. **INC-42.3** webui（composer_api + /mode slash + pill live 化 + 注释
   清除）+ frontend 测试——一提交；
4. **INC-42.4** QA-44 真机 + 文档收口（SPEC/PARITY/GAPS/QA/JOURNEYS/LOG
   + deadcode 基线行移除）+ 工作纸归档——一提交。

## review 裁决

小增量，inline 自审（本工作纸即三层 delta 审）。correctness 关口两个：
(1) durable command 幂等（孪生钉）；(2) 生效时机 = 安全边界（drainControls
双路给定，不打断在飞 activity）。安全面零放宽：bypass 不可达、protected/
hardFloor/hooks 全不动；acceptEdits 放宽与 `approve --always` 同级，都是
用户显式主权操作。
