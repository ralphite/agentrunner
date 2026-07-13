# INC-67 设计契约加固审计

## 动机与 journey 锚

对 `origin/main` 做全量检查、race 检查与 DESIGN 对照后，修复正常使用或
边界输入可触发、且不需要产品裁决的明确缺陷：session 路径越界、Web 上传
静默截断、共享配置/记忆的并发丢更新、daemon socket 权限失败被忽略、
session id 熵不足、worktree 分支校验不一致、流扫描失败被误报正常结束，
以及 clean checkout 的 gate 顺序错误。对应 UJ-01/02、UJ-19/20、UJ-24
与 PROCESS 的完成闸门。

## Spec delta

- 持久化/安全：session 只可解析到共享 store 内的合法目录；共享
  `CLAUDE.md`、user settings、trust registry、hook registry 与 artifact
  manifest 的 read-modify-write 跨 goroutine/进程串行且原子替换。
- Web UI：上传超过 10 MiB 明确返回 413，不产生截断文件；JSON body 超限或
  带第二个/trailing JSON value 明确拒绝；新 worktree 使用与 checkout 相同的
  Git branch 校验；attach/run 输出单行超限显式失败并取消可能堵塞的子进程。
- 运行时：daemon socket 必须成功收紧到 0600 才开始服务；session id 使用
  64-bit 随机后缀并在熵源失败时 fail closed。
- 工程闸门：frontend build 先于依赖 embed 产物的 WebUI Go 测试；race 下的
  approval-waiting child 恢复测试不依赖 goroutine 抢跑顺序；deploy 可显式
  复用真实 WebUI runtime，避免从不同 worktree 部署时切走既有 metadata/log。

## Design delta

除语言工具链外，其余实现只是兑现现有契约：数据根边界、owner-only socket、
blob/event 先后关系、共享状态不丢更新、输入上限不静默损坏。

### 不变量变更：Go 最低/安全版本

- 旧表述：Go 1.23+。
- 冲突：根 `go.mod` 与 MCP SDK v1.6.1 已要求 Go 1.25，README/DESIGN 的
  1.23+ 会让新用户在依赖解析期失败；`govulncheck` 又确认本机 Go 1.26.4
  的标准库可达 GO-2026-5856，只有写语言大版本仍会产出有漏洞 binary。
- 备选：降级 MCP SDK 以恢复 1.23（丢当前协议/修复，风险大）；只改文案为
  1.25（仍容许已知漏洞 patch）；校准为 1.25+ 并由 gate 拒绝已知不安全
  patch。
- 裁决：取第三项。当前安全下限为 Go 1.25.12 / 1.26.5；更高 stable
  版本可用，prerelease 不进发布 gate。CI 的 `go-version: "1.25"` 继续解析
  该分支最新 patch。

**单独 review 裁决**：通过。此变更承认仓库早已存在的真实依赖下限，不改变
runtime 模型或分发目标；安全 patch gate 防止 build provenance 把标准库漏洞
固化进静态 binary。README、DESIGN、check 与 release 的版本口径一致。

## 验收

- `TestResolveSessionDirRejectsTraversalAndSymlinkEscape`、
  `TestSessionDirRejectsUnsafeID`。
- Web API oversized upload/JSON、trailing JSON、slash branch 测试。
- memory/config/hooks/artifact 多 writer 回归测试（含 `-race`）。
- daemon socket mode 测试；session id shape/碰撞空间测试。
- `govulncheck` 无可达模块/标准库漏洞；已知不安全 Go patch 被 gate 拒绝。
- `npm audit` 无漏洞；Vite 升至已修复分支，Node engine/gate 与其真实下限一致。
- `./scripts/check.sh` 从无 `webui/frontend/dist` 的状态全绿。
- 共享 store + 真实 Web UI 验证 session list/detail、普通小文件上传、超限
  拒绝、daemon restart 后原 session 仍可读；证据保留在 QA run 目录。

## 实施步骤

1. 修持久化与路径/权限边界，补 Go 回归测试。
2. 修 Web 输入边界与 worktree 校验，补 handler 测试。
3. 修 gate 顺序与 race 时序测试，跑全量/race/真实环境 QA。
4. 并回三层与 LOG，归档工作纸。

## review 裁决

做 correctness/concurrency/security/contract 复核；不新增产品概念或 UI 控件，
用户可见变化仅为把原先的静默损坏改成既有 error/toast 模式的短错误。
