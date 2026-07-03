# PROGRESS — 实施决策台账（PLAN §0.5 规定）

每步一节：状态、所做决定（凡计划未指定而自行选择的）、留给 stage
review 的 open questions。

## S1.0 — 工程基座　✅

- **状态**：完成。go module + CLI 骨架（`--version`）+ slog 约定 +
  `.golangci.yml` + `scripts/check.sh`，check 全绿。
- **决定**：
  - Go 1.24（环境预装 1.24.7，满足 PLAN 的 1.23+）。
  - golangci-lint 环境预装 **v2.5.0** → 配置用 **v2 格式**
    （`version: "2"`，非 PLAN 隐含的 v1 格式）；forbidigo 用
    `exclusions.rules[].path-except` 实现"只在
    internal/(kernel|state|pipeline) 生效"。
  - CLI 骨架放 `internal/cli`（`Run(args, version, stdout, stderr) int`
    可测试），main 只做转发；`run` 子命令留给 1.9。
  - 日志走 stderr（stdout 留给 run 输出），`AGENTRUNNER_DEBUG=1` 提级。
  - `.gitignore` 清理了 Python 时代残留（`__pycache__`），加编译产物。
  - errcheck 排除 `fmt.Fprint/Fprintf/Fprintln`（CLI 输出返回值检查是
    噪音；项目级惯例）。
- **DEFERRED**：无。
- **Open questions**：无。
