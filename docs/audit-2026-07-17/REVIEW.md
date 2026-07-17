# 审计 2026-07-17：design↔代码一致性 review

**方法**：两路并行核查——①SPEC 附录"代码事实对照"、✅ 条目验收锚抽样、
DESIGN §17 偏差登记逐条对代码；②SPEC 🟡/❌ 全量 + GAPS §2 开放条目与
"余项" + QA 待验标注 + webui 占位 + LOG 末段。基线：`origin/main@5ad9477`。

## 一、结论速览

1. **验收锚诚实**：抽 10 个具名 Go 测试锚全部真实存在；glob 前缀锚
   （`TestWebFetch*` 等）逐一命中。无幻影锚。
2. **DESIGN §17 未漂移**：已核实的 4/6 条偏差登记与代码现状一致
   （#3 inbox 形状、#5 WAITING_APPROVAL 唤醒两条未深验，不下结论）。
3. **SPEC 附录"代码事实对照"整体滞后**（自称 2026-07-05 盘点）：
   - CLI 子命令漏登 **11 个**：`diff` `artifacts` `retry` `queue`
     `unqueue` `hook` `answer` `mode` `goal` `dictate` `optimize`
     （证据 `internal/cli/cli.go:40-126`）；
   - daemon 线协议漏登 **8 个**：`mode` `unqueue` `answer`
     `goal-attach` `goal-pause` `goal-resume` `goal-update`
     `goal-cancel`（证据 `internal/daemon/daemon.go:826-877`；
     注意 `dictate`/`optimize` 是前台一次性 CLI、`hook` 走 HTTP
     ingress，都**不是** wire 命令，不应误登）；
   - tool defs 漏登 **8 个**：`ask_user` `web_fetch` `progress_update`
     `send_message` `artifacts_list` `artifacts_read` `goal_complete`
     `goal_status`（`internal/tool/defs/` 实有 26 个 json）；
     `escalate` 无独立 def（由 spawn 路径承担），不应新增。
   - 方向一致均为"代码有、附录无"；无反向缺陷。
4. **两处纯文档漂移**：
   - SPEC.md:163 command tools 行仍写"QA-59 待验"，QA.md:911-913
     已记 PASS（2026-07-11）；
   - GAPS G4 仍列开放（GAPS.md:420-421）而 SPEC.md:71 已注
     "GAPS G4 关闭事实"——两活文档冲突，按 PROCESS §一当场修。
5. **代码内 TODO 极少**：产品代码唯一占位是
   `webui/frontend/src/components/SettingsConfiguration.tsx:58`
   "Approval policy & sandbox — Not surfaced"（daemon `/health` 未暴露
   全局审批/沙箱字段，文案诚实）。`qa/fixtures/semver-broken` 的
   TODO 是故意的测试夹具。
6. **环境事实**（本容器）：预装 go1.25.0 被 repo 自己的
   `scripts/check-go-toolchain.sh` 拒绝（要求 1.25.12+/1.26.5+）；
   golangci-lint 2.5.0（go1.25 构建）无法分析 go1.26 stdlib——
   已把 go1.25.12 装为系统 Go（`/usr/local/go` symlink）。注意
   `GOTOOLCHAIN=go1.25.12` 环境变量会渗入沙箱内子进程的 `go test`，
   断网沙箱下载 toolchain 必败——须用原生匹配版本而非 env 切换。
7. **测试平台假设缺口（本次修复）**：`TestBashFilesystemSandbox`
   断言钉死 "Operation not permitted"（macOS Seatbelt errno 措辞）；
   Linux bwrap 后端用 tmpfs//dev/null 掩蔽，读拒绝表现为 ENOENT/
   EACCES，测试必败。CI 因 runner 无 bwrap 一直 `t.Skipf` 跳过，
   掩盖了这一点——Linux 沙箱面从未被该测试真正回归过。已改为
   平台无关的"读被拒绝"断言（`internal/tool/exec_test.go:425`），
   泄漏检查（OUTSIDE/INSIDE-SECRET/ENV-SECRET 不出现）原样保留。

## 二、开放待办全量（按处置类别）

详细条目、证据行号、规模估计见同目录 `BACKLOG.md`（loop 工作清单，
带勾选状态）。分类：

- **a) 纯文档即可关闭**：SPEC 附录三清单补登、DOC-QA59、DOC-G4、
  G33 回标核实、G16 威胁模型条款成文。
- **b) 小代码改动（各自一个小增量）**：G22c（daemon kill -9 孤儿
  bash pgid 清扫）、G26（`ar inspect` children 按 call_id 去重）、
  G10（bash 后台任务进度 tail）、G18 余项（web_fetch host
  allowlist）、webui mermaid 懒加载、G36 余项（schedule 内联校验 +
  错误 details 披露）、SettingsConfiguration approval policy 接线
  （daemon `/health` 增只读字段）、G16 定界符。
- **工程债燃尽（登记簿真实性）**：G30 弱锚存量（31 行，只减不增）、
  G31 deadcode 存货甄别（19 个不可达导出，三选一处置）。
- **c) 需完整增量/不变量流程**：G3（WAITING_APPROVAL 唤醒语义）、
  G1（fork/rewind blob 归属）、G2（barrier 在飞处置）、G22a（崩溃
  session 自动接续）、**G22b（优雅停机保活 cron——GAPS 明确指向
  DESIGN §四不变量变更流程）**、G13（SCM/PR 工作流）、G15（best-of-N
  晋升）、web search（G18 主项）、G32（Xcode.app 沙箱 git）、
  driver 收敛为递归 session（large）、G11 云 workspace（large）。
- **🧊 显式推迟不计待办**：`ar new` 开场折叠/带图、`finish` 工具、
  overlap:interrupt、MCP 交互 OAuth、HTTP/WS 全壳、IDE 集成、
  --add-dir 多根。

## 三、裁决建议

- a/b/工程债两类共 ~15 项可由实施 loop 直接燃尽（每项一个可合并
  提交，check.sh 全绿 + push origin/main）。
- c 类每项先出 `docs/increments/INC-*.md` 工作纸（三层 delta 明确）
  再实施；G22b 单独走不变量变更流程；driver 收敛与 G11 规模为
  large，loop 内以"每迭代推进一个可合并步骤"的粒度处理，工作纸
  先行。
