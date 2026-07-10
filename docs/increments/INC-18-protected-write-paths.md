# INC-18 protected paths 写保护集（#59）

## 动机与 journey 锚

CLAUDECODE-PARITY §2.06 #59 + UJ-08。靶心：`acceptEdits` 模式对 edit 类
**任何路径**静默 Allow（`modeDefault`，permission.go:244-247），于是
`.git/config`、`.claude/settings.yaml`、`.bashrc`、`.mcp.json` 等敏感
配置/系统文件在 acceptEdits 下被无声改写。对标 Claude Code 的 protected
paths 表：这些路径**除 bypass 外从不自动批准**。纯 pipeline 层，不触
不变量。

## 安全立场（本增量核心）

protected paths 只**收紧 mode default 的自动放行**，不新增 floor、不碰
显式规则：
- **acceptEdits 写 protected → Ask**（而非 Allow）：acceptEdits 的风险
  是"自动放行一切 edit"，本增量把敏感文件从自动放行里摘出来要求审批。
- **default 写 protected → 不变**（edit 类 default 本就 Ask）。
- **bypass → Allow 不变**（bypass 显式放弃保护，protected 不挡）。
- **显式 deny 规则 → deny 不变**；**显式 allow 规则 → Allow 不变**
  （rules 循环先于 modeDefault：用户显式配 `allow edit_file path:.git/**`
  是明确意图，尊重——与 Claude Code"allow 不预批 protected"的差异记档，
  我们的架构里显式规则=用户意图，不被 mode 保护覆盖）。
- **plan → deny 不变**（hardFloor + modeDefault 已 deny edit/execute）。
一句话：protected 是 **acceptEdits 更安全**，不是新 hardFloor。

## protected 路径集（workspace 相对）

- 目录（其下任意深度）：`.git`、`.claude`（**除 `.claude/worktrees`**——
  worktree 隔离用，carve-out）、`.config/git`、`.vscode`、`.idea`、
  `.husky`、`.github`。
- 文件（basename，任意深度）：shell rc（`.bashrc`/`.zshrc`/`.profile`/
  `.bash_profile`/`.zshenv`/`.zprofile`）、包管理器 rc（`.npmrc`/`.yarnrc`/
  `.yarnrc.yml`/`.pypirc`）、git 配置（`.gitconfig`/`.gitmodules`）、
  `.mcp.json`、`.claude.json`、`.pre-commit-config.yaml`、`.ripgreprc`、
  wrapper（`gradle-wrapper.properties`/`maven-wrapper.properties`）。

## Spec delta

- SPEC D 新行「protected paths 写保护（acceptEdits 下敏感配置/系统文件
  写需审批；bypass/显式规则不变）」，锚 `TestProtectedWrite*` + QA-27。
- CLAUDECODE-PARITY §2 #59 状态 🟡→✅。

## Design delta（不触不变量）

DESIGN §5 permission 加一小节「protected 写路径（INC-18，#59）」：
`PermissionGate.Check` 在 `modeDefault` 返回 Allow 后，若该 Allow 来自
**acceptEdits 的 edit 自动放行**且目标是 protected 路径，改 Ask。只影响
mode default 的自动放行，不改 rules/hardFloor/bypass。决策 #10（mode=
工具面过滤+默认）注记此收紧。

## 验收

- 孪生（permission_test.go 扩展）：
  - `TestAcceptEditsProtectedRequiresApproval`：acceptEdits 写 `.git/config`
    /`.claude/settings.yaml`/`.bashrc` → Ask；写 `src/a.go` → Allow。
  - `TestBypassIgnoresProtected`：bypass 写 protected → Allow。
  - `TestExplicitAllowOverridesProtected`：显式 `allow edit_file
    path:.git/**` 写 `.git/config` → Allow（显式规则优先）。
  - `TestProtectedWorktreeCarveout`：写 `.claude/worktrees/x/f` → 不
    protected（acceptEdits Allow）。
  - `TestDefaultModeProtectedStillAsks`：default 写 protected → Ask（不变）。
- 真实 API QA-27：acceptEdits spec，让模型改一个普通文件（放行）与
  `.git/config`（拦下需审批），验证逐路径裁决；`ar events` 归档 qa/runs/。
- `./scripts/check.sh` 全绿。

## 实施步骤

1. `isProtectedWritePath(rel)` + protected 集（command.go 或 permission.go）。
2. `PermissionGate.Check` 在 modeDefault Allow 后插 protected 检查。
3. 孪生扩展 + QA-27。
4. 文档行齐活。

## review 裁决

做。S 号、纯 pipeline 层、不触不变量。inline 三视角：correctness
（路径匹配、worktree carve-out）、security（只收紧 acceptEdits 自动放行、
bypass/显式规则不变、集覆盖敏感文件）、contract（不改 rules/hardFloor/
mode 语义，只加一层 acceptEdits 保护）。
