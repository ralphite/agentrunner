# INC-3 grep / glob 独立工具（G18a）

## 动机与 journey 锚
- **缺口**：GAPS **G18**（内置工具面完整性）——DESIGN §448 的内置 tool
  套件已列 `glob/grep`，§450 明记"尚未[实现]"；现状借 bash 走
  （SPEC C 行 `grep / glob 独立工具` = ❌）。这是**纯实现缺口**，
  设计已覆盖，不触任何不变量。
- **journey**：UJ-01 即问即答（覆盖功能标签 `代码检索(grep/glob)`）、
  UJ-04 贴图贴日志（`代码检索`）。
- **对标 Codex**：ripgrep 级 grep + glob 是前沿 coding agent 的日用检索
  底座；借 bash 有三害：命中凭据文件无红线、输出无 per-tool 截断纪律、
  在 network=none 收容下也被拦。独立工具都解决。

## Spec delta
- SPEC **C · 工具面**：`grep / glob 独立工具` ❌ → ✅，验收锚指
  `TestGrep*` / `TestGlob*`（新增 scripted 孪生）+ QA-11（真实 API：
  模型自发调用 grep 定位符号）。
- SPEC 附录「内置 tool 定义」清单 + `grep` `glob`。

## Design delta
- DESIGN §436 已实现工具清单加 `grep` `glob`。
- DESIGN §450 note「glob/grep/web 尚未[实现]」→「web 尚未；grep/glob
  已实现（INC-3）」。
- **不触不变量**：grep/glob 是 read-class 工具，走既有 effect pipeline
  四关卡（permission/budget），输出过 redaction，凭据路径硬排除与
  semantic_search 同源（内容落 journal 的工具共用一个排除谓词）。
- 实现注记：凭据排除谓词从 `internal/index` 导出为 `index.SkipDir` /
  `index.SkipFile`，index + grep/glob 共用一份——真 lockstep 取代
  index.go 注释里"手工保持同步"。snapshot 的 gitignore-pattern 机制
  是另一套（写 info/exclude），不动。

## 验收
- **闸门 A（scripted 孪生，进 check.sh）**：`internal/tool/exec_test.go`
  - `TestGrepFindsMatches`：多文件命中，返回 path/line/text，按路径+行序。
  - `TestGrepRespectsCredentialExclusion`：workspace 里放 `.env` 含
    `SECRET=xxx`，grep `SECRET` **不得**命中它（红线）。
  - `TestGrepTruncates`：命中数超上限时截断并标注。
  - `TestGrepBadRegex`：非法正则 = model-visible error，非 harness 崩。
  - `TestGlobMatches`：`**/*.go` 类模式命中，排除 skipDirs。
  - `TestGlobRespectsExclusion`：不列出 skipDirs / 凭据文件。
- **闸门 B（真实 API QA-11）**：真 daemon + 真 Gemini，给一个多文件
  workspace，问"grep 出所有定义 Foo 的地方"，断言 journal 里出现
  `activity_started{name:grep}`（结构断言，不钉模型措辞）。

## 实施步骤
1. **一步**（一个提交 `INC-3.1: grep/glob tools`）：
   - `internal/index/index.go`：`skipDir`/`skipFile` 导出为
     `SkipDir`/`SkipFile`（保留旧内部调用点）。
   - `internal/tool/defs/grep.json` `glob.json`（class=read）。
   - `internal/tool/exec.go`：`grep()` `glob()` + dispatch 两 case；
     复用 `index.SkipDir/SkipFile`、`e.WS.Resolve` 边界、`redact.FromEnv()`、
     head/tail 截断纪律。
   - `internal/tool/exec_test.go`：上述孪生。
   - 文档行：SPEC/GAPS/DESIGN/LOG 齐活。
   - `./scripts/check.sh` 全绿 → commit → push。
2. QA-11 脚本 `qa/run-qa11.sh` + QA.md 菜单条目（闸门 B），真验归档。

## review 裁决
小增量，inline 自审（正确性：正则/边界/排除；契约：class=read 过 mode
过滤）。规模未达里程碑，裁掉三视角对抗 review，理由记本纸。
