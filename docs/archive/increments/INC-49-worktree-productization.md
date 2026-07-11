# INC-49 webui "New worktree" 运行位置的产品化改造

**认领人**：worktree-agent-ad96fade4ed7ab2b6（2026-07-10）
**占号**：INC-49 · QA-46（占号先推，见下）

## 动机与 journey 锚

现状：webui 的 `POST /api/worktree`（`webui/api.go handleWorktree`）对所选 repo
执行真 `git worktree add`，但 checkout 落在 webui 服务端 cwd 下的
`runtime/ws/wt-<时间戳>`。三个产品问题：

1. **位置绑死 webui cwd**：任何 repo 的 worktree 都堆进 `agentrunner/webui/runtime/ws`，
   名字只有时间戳，看不出属于哪个 repo、难找、跨 webui 重启不稳定。
2. **不可见**：会话头 / Changes 面板 / run location 选择器都不显示"这是谁的 worktree、
   在哪个 branch"。
3. **无生命周期闭环**：创建后没有"把改动 apply 回主 checkout"与"用完清理 worktree"
   的产品面——用户被迫回落到 bash+gh。

journey 锚：**UJ-10 提交流水**（worktree 一等公民化是其 SCM 闭环的一环）+
**UJ-22/24 Web UI composer / 任务收尾**（run location 可见性与 Changes 面板闭环）。
对应缺口 **G13 SCM/PR 工作流一等公民化**（本增量交付 worktree 位置/可见/生命周期
这一子面；diff→审阅门→PR 主体仍留 G13）与 CODEX-PARITY §04「worktree 一等公民」
🟡→🟢（位置/可见/apply-back/cleanup 四问）。

不触及任何 DESIGN §15 不变量：worktree 仅是 webui 编排层的 checkout 位置与 git 原生
操作，不动 journal-first / 凭据红线 / crash 语义。**无需走不变量变更流程。**

## Spec delta

SPEC.md webui 域「Web UI progressive-disclosure composer」行补注 run location 落稳定
共享位置 + 显示 repo/branch；**新增一行**：

- `Web UI worktree 一等公民化（run location 落 ~/.local/share/agentrunner/worktrees/
  <repo>-<branch>-<ts>；Changes 面板显示所属 repo/branch + Apply changes 回主 checkout
  + Remove worktree 清理，未 apply 改动防呆确认；旧 runtime/ws worktree 仍可打开）`
  状态 ✅，锚 INC-49 · TestWorktreeInDataDir/TestApplyBackCleanApply/TestApplyBackConflictReported/
  TestWorktreeRemoveGuardsDirty · QA-46（真机：选 repo 开 worktree→真实模型改文件→apply-back→cleanup）。

## Design delta

DESIGN.md 无不变量变动。webui 章（若有 webui 小节）补一段实现注记：

- **worktree 位置**：webui 不再把 worktree 放 `runtime/`，改放共享数据根
  `DataDir()/worktrees/`（DataDir = `$XDG_DATA_HOME/agentrunner` 或
  `~/.local/share/agentrunner`，与 daemon store 同根，webui 内独立复刻这段 XDG 逻辑，
  因 `arwebui` 是独立 module 不 import `internal/runtime`）。
- **命名**：`<repo basename>-<branch slug | "detached">-<YYYYMMDD-HHMMSS>`，同秒冲突加 `-2/-3`。
- **apply-back 机制（git 原生、clean-or-nothing）**：worktree 与主 checkout 共享 object DB。
  在 worktree `git add -A` → `write-tree` → `commit-tree -p HEAD` 得到含全部改动（含 untracked）
  的 commit 对象 C；`git diff --binary HEAD C` 得 patch；主 checkout 先 `git apply --check`
  干跑，**通过才** `git apply`（落 working tree、不 stage，供用户在主 checkout 里自行审阅提交），
  check 失败即如实报 conflict、**绝不改主 working tree**（不做静默半合并）。
- **cleanup**：`git worktree remove <path>`，脏树 git 原生拒绝→回 UI「有未 apply 改动」标志→
  用户确认后带 `--force` 再删；删后 `git worktree prune`。
- **nested 判定复审**：worktree 是自身 repo root（`--show-toplevel` 返回 worktree 自身），
  故 `handleDiff` 仍判 `isRepo:true`，不触发 `nested:true`；挪出 `runtime/` 后 meta.go:238
  附近 nested 特殊处理逻辑不受影响（那段针对的是内嵌 gitignored `runtime/` 的裸目录 workspace，
  worktree 从来不是那种）。diff 响应新增 `worktree`/`mainRepo`/`branch` 字段供 UI 显示与开关按钮。
- **安全**：所有传 git 的用户输入沿用 `validID` + `--end-of-options` 防注入；worktree 路径来自
  session metadata（非请求体），apply/remove 端点校验目标确是 linked worktree 才动手。
- **旧位置兼容**：已存在于 `runtime/ws/wt-*` 的旧 worktree 不迁移（迁移会打断其 git 链接与
  正在引用它的 session metadata，风险 > 收益）；它们仍是合法 workspace，Changes 面板照常
  打开与 diff。新建一律走新位置。裁决记 LOG。

## 验收

**闸门 A（scripted/单测，进 check.sh）** — 逐项对锚：
- `TestWorktreeInDataDir`：POST /api/worktree 落 `worktreeDir/<repo>-<branch>-<ts>`，
  HEAD 正确，返回 repo/branch。
- `TestApplyBackCleanApply`：worktree 改文件（含新增 untracked）→ apply 端点 → 主 checkout
  working tree 出现同样改动且未 staged，patch 干净。
- `TestApplyBackConflictReported`：主 checkout 与 worktree 对同文件分叉改动 → apply 端点
  返回 conflict 错误，主 working tree **未被改动**。
- `TestWorktreeRemoveGuardsDirty`：脏 worktree remove 无 force → 返回 dirty 标志、不删；
  force=true → 删除且 `worktree prune` 后主 repo `git worktree list` 不再含它。
- `TestDiffReportsWorktreeMeta`：linked worktree 的 diff 响应带 `worktree:true`/`mainRepo`/`branch`；
  普通 repo workspace 不带。
- 前端 vitest：DiffView 渲染 worktree 头（repo/branch）+ Apply/Remove 按钮（有此前 diffSummary
  类纯函数则加，UI 交互以真机 QA 为主锚）。

**闸门 B（真实 API，QA-46，归档 qa/runs/2026-07-10-qa46/）**：
共享 daemon + 真实 webui 强刷后，选 repo 开 worktree（确认落新位置 + UI 显示 repo/branch）→
真实模型 session 改几个文件 → apply-back 回主 checkout（确认主 checkout 出现改动）→
清理 worktree（确认 `git worktree list` 清干净）。证据截图 + ar events 归档。

**裁掉的项（显式声明）**：不做旧 `runtime/ws` worktree 的自动迁移（见 Design delta 裁决）；
不做 apply-back 的 3-way 自动合并（冲突一律回退让用户手工，符合"不静默半合并"）；
Settings→Worktrees 的删除/prune 入口本增量只把后端 remove 端点接到 Changes 面板，
Settings 页仍只读（其注册表 API 是 G13 后续）。

## 实施步骤（一步 = 一个可合并提交）

1. **占号**：本工作纸 + SPEC/GAPS/CODEX-PARITY/LOG 占位行，push origin/main。✅=占号推成。
2. **后端 worktree 位置/命名**：server 加 `worktreeDir`，main.go 设为 `DataDir()/worktrees`，
   `handleWorktree` 走新位置+命名，返回 branch；`TestWorktreeInDataDir` 绿。
3. **后端 apply-back + remove + diff worktree meta**：三端点 + handleDiff 补字段；对应单测绿。
4. **前端**：api.ts 加 apply/removeWorktree；DiffView 显示 worktree 头 + Apply/Remove 按钮
   （防呆确认）；Composer/会话头按现状合理补 repo/branch 显示；vitest 绿；node24 rebuild dist。
5. **收口**：check.sh 全绿 → 并回 SPEC/GAPS/CODEX-PARITY/LOG/QA，工作纸归档；QA-46 真机复验。

## review 裁决

薄层 webui 编排增量、不触不变量、git 原生 clean-or-nothing 已含安全/契约考量，
**裁掉三视角对抗 review**，理由：无并发状态机改动、无凭据面、apply-back 的 conflict
安全性由"干跑通过才落、否则零改动"这一单一不变量保证，单测 `TestApplyBackConflictReported`
直接钉它。真机 QA-46 作最终证。
