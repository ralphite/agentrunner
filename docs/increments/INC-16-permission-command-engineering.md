# INC-16 权限规则工程三件套（复合命令拆分 + wrapper 剥离 + 只读集）

## 动机与 journey 锚

CLAUDECODE-PARITY §2.06 #53 + UJ-08。Claude Code 遥测：旧默认下 **93%
权限提示被反射式批准**（approval fatigue）。三件套是其主解，也修一个
**安全弱点**：现状 `PermissionGate.Check` 对**整条 command** 做规则
匹配（permission.go:72），于是一条 `Bash(git *)` allow 会误放行
`git status && rm -rf /` 整条——未审的子命令搭便车。纯 pipeline rules
层，不触不变量。

## 安全立场（本增量的核心，先写在最前）

三件套里两件是**收紧**、一件是**受控放松**：
- **复合命令逐段匹配 = 收紧**：整条 allow 的旧行为是 bug（放行了从未
  被规则匹配的子命令）。改为每个子命令独立评估，聚合取**最严**
  （任一 deny → deny；任一 ask → ask；全 allow 才 allow）。
- **wrapper 剥离 = 收紧方向的便利**：`timeout 60 npm test` 能被
  `Bash(npm test)` 匹配。剥离**只认白名单前缀**（timeout/time/nice/
  nohup/stdbuf/裸 xargs），剥不动就不剥（fail-safe：不剥 = 整词参与
  匹配，更严不更松）。
- **只读命令免提示集 = 受控放松**（唯一放松）：`ls/cat/echo/pwd/head/
  tail/grep/find/wc/which/diff/stat/du/cd` 等**内置只读**命令免提示。
  这些本就无害，Claude Code 同做。**危险排除**：`find -exec`/`find
  -delete`（find 能执行命令）恒不进只读集；任何带重定向 `>`/管道到
  非只读命令的段不算只读。
- **fail-safe 总纲**：拆分器/剥离器**拿不准就退回整体匹配**（更严）；
  只读集是白名单精确匹配（宁可漏放行让用户点一下，绝不误放行危险
  命令）。deny/ask 规则与 hardFloor 永远先于任何放松。

## Spec delta

- SPEC D「rules」行注记复合命令逐段匹配 + wrapper 剥离 + 只读集
  （INC-16）；锚 `TestCompound*/TestWrapper*/TestReadonly*` + QA-25。
- CLAUDECODE-PARITY §2 #53 状态 🟡→✅。

## Design delta（不触不变量）

DESIGN §5 permission 段加一小节「命令粒度匹配（INC-16）」：
- bash（Command 非空）effect 的规则匹配从"整条"改"**逐子命令聚合**"：
  `splitCompound(command)` 按顶层 `&&`/`||`/`;`/`|`/`|&`/`&`/换行拆
  （引号内不拆）；每段 `stripWrappers` 后先判只读集（命中 = 该段
  allow），否则走既有 `rule.matches`；段裁决聚合取最严。
- 只影响 **bash 命令匹配的粒度**，不改规则语义、不改 path/network
  匹配、不改 hardFloor/mode default。决策 #10（mode = 工具面过滤）
  与决策 #33/#34（沙箱棘轮）不动。
- **安全不变量成文**：拆分/剥离单调收紧（§安全立场），只读集白名单
  + exec 排除。

## 验收

- 孪生（permission_test.go 扩展）：
  - `TestCompoundSplitTakesStrictest`：`git status && rm -rf x` 在
    `Bash(git *)` allow 下**不** allow（rm 段未覆盖 → mode default）。
  - `TestCompoundAllSegmentsCovered`：`git add && git commit` 在
    `Bash(git *)` allow 下 allow（两段都覆盖）。
  - `TestWrapperStripped`：`timeout 60 npm test` 被 `Bash(npm test)`
    allow 命中；未知 wrapper 不剥。
  - `TestReadonlySetAllowed`：`ls -la` / `cat f` 免规则 allow；
    `find . -exec rm {} \;` **不**进只读集。
  - `TestQuotedSeparatorNotSplit`：`echo "a && b"` 不拆（引号内）。
  - `TestSplitFailSafeWholeMatch`：拿不准的复杂命令退回整体匹配。
- 真实 API QA-25：真 Gemini + 私有 daemon，配 `Bash(git *)` allow，
  让模型跑 `git status`（放行）与含危险段的复合命令（该段被拦），
  验证逐段裁决；`ar events` 归档 qa/runs/。
- `./scripts/check.sh` 全绿。

## 实施步骤

1. `internal/pipeline/command.go`：splitCompound / stripWrappers /
   isReadOnlyCommand（各带单测）。
2. 改 `PermissionGate.Check` bash 分支为逐段聚合。
3. 孪生扩展 + QA-25。
4. 文档行齐活。

## review 裁决

做。M 号、纯 rules 层、不触不变量。**安全敏感**——inline 三视角自审：
correctness（拆分覆盖所有 sh 分隔符、引号处理）、security（单调收紧、
只读集 exec 排除、fail-safe 退回整体）、contract（不改规则语义/其他
gate）。拆分器保守（拿不准退回整体），宁漏放行不误放行。
