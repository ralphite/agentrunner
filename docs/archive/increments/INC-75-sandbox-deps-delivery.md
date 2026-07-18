# INC-75 OS 沙箱依赖交付（bubblewrap 检测/安装/一行接入）

## 动机与 journey 锚

现场事故（2026-07-18）：在 GitHub Actions 启动的环境里跑 `ar`，bash 工具
第一条命令（`pwd`）即被拒：

```
denied: required OS sandbox unavailable: bubblewrap unavailable: exec: "bwrap": executable file not found in $PATH
```

fail-closed 本身是决策 #34 的既定行为、**不动**。缺口在交付面：bwrap 是
Linux 运行时硬依赖，却完全没进入安装与部署故事——install.sh（INC-63）
不检测不安装；README 无 prerequisite；报错在第一条 bash 才暴露且无修复
指引；`qa-all.yml` 里的安装配方（apt + Ubuntu 24 AppArmor sysctl）是散落
的手抄知识，每个新环境都要人肉复制。

Journey 锚：**UJ-25 一行安装与升级**增补步骤——安装完成即代表 bash 工具
可用（Linux 沙箱依赖被检测/安装/验证），CI（GitHub Actions）环境一行
接入。

**显式取舍：不把 static bwrap 打包进产物。** Ubuntu 23.10+ 用 AppArmor
按二进制路径（`/usr/bin/bwrap` 的发行版 profile）放行非特权 userns；
自带 static bwrap 放在 `~/.local/...` 下照样被拦，仍需 root 改 sysctl 或
装 profile——打包在最需要它的场景恰好无效，而能用 static bwrap 的环境里
`apt install` 同为一行。另有 LGPL 再分发与 per-arch 维护成本。正解是
"检测 + 自动安装 + 一行接入"。

## Spec delta

- J·运行形态与云：新增功能点行"OS 沙箱依赖交付"——`ar doctor` 诊断
  命令、probe 报错自带修复指引、install.sh Linux 沙箱依赖检测/安装/
  probe 验证（`AR_SKIP_SANDBOX_DEPS`/`AR_REQUIRE_SANDBOX`）、composite
  action `.github/actions/setup-ar` 一行接入。
- 附录 CLI 子命令面（如触及）：`doctor` 登记。

## Design delta

- §分发与安装（INC-63）：新增小节"OS 沙箱依赖交付（INC-75）"，写明
  上述取舍与机制。**不触任何不变量**——决策 #34 fail-closed 原文不动，
  本增量只解决"缺席时怎么被发现与补齐"。

## 验收

枚举型交付物逐项对锚：

| 项 | 锚 |
|---|---|
| probe 报错带修复指引（bwrap 缺失 / probe 失败两形态） | `TestLinuxSandboxHint*`（linux 单测） |
| `ar doctor`：backend + network=all/none 两档探测、失败非零退出、指引可见 | `TestDoctor*`（探针可注入，双路径覆盖） |
| install.sh：跳过旗标（基线场景 1–5 即走此路径）、bwrap present→probe 验证（场景 6）、probe 失败且无修复权限→警告不失败（场景 7）、`AR_REQUIRE_SANDBOX=1`→硬失败（场景 8） | `scripts/test-install.sh` 场景 6–8（Linux only；macOS 声明跳过）。**显式裁掉**：missing-bwrap 分支的孪生——离线孪生无法在不做整 PATH 手术的前提下隐藏宿主 bwrap，该分支为纯诊断输出，由 gate B 真实无 bwrap runner 覆盖 |
| composite action 一行接入 | gate B：`sandbox-doctor.yml` 在 ubuntu runner 真跑绿（QA-75） |
| qa-all.yml 复用 action（配方去重） | 同 gate B（action 与 qa-all 同一路径） |

gate B = QA-75：dispatch `sandbox-doctor` workflow，断言 setup-ar 后
`bwrap --version` 可用、`ar doctor` 退出码 0 且两档 probe OK。

## 实施步骤

1. INC-75.1 本工作纸（docs only）。
2. INC-75.2 probe 修复指引 + `internal/tool.DoctorSandbox` + `ar doctor`
   （cli 接线、help、单测）。完成标志：`go test ./internal/...` 绿。
3. INC-75.3 install.sh 沙箱依赖步 + test-install.sh 新场景。完成标志：
   `scripts/test-install.sh` 全过。
4. INC-75.4 `.github/actions/setup-ar` + `sandbox-doctor.yml` +
   qa-all.yml 改用 action。完成标志：yaml lint 过（真跑在收口后）。
5. INC-75.5 收口：README prerequisite、SPEC/DESIGN/QA/LOG 行齐活、
   工作纸归档、check.sh 全绿、合并 origin/main、dispatch gate B 真跑。

## review 裁决

小增量（无新不变量、无并发面、攻击面只减不增：报错更可读 + 安装器多装
一个发行版官方包）。裁掉三视角对抗 review，理由：安全敏感点唯一是
install.sh 以 root 跑 `apt-get install bubblewrap`/sysctl——命令固定、
无用户输入拼接；sysctl 只在 probe 失败且用户未跳过时执行并回显。
