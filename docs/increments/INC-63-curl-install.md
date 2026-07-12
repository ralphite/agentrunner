# INC-63 curl 一行安装分发（自包含发布 + install.sh + release CI）

## 动机与 journey 锚

HANDA-PARITY 域二 #2（自包含发布 + 一行安装 + release CI/smoke）当时裁
⏸ defer（"单人原型阶段无分发需求"）。用户现在明确要求对齐 handa 的
curl 分发形态：不装 Go/Node 工具链的机器上，一条

```sh
curl -fsSL https://raw.githubusercontent.com/ralphite/agentrunner/main/install.sh | sh
```

就能装上可用的 `ar` + `arwebui`，再跑一遍即升级。

Journey 层新增 **UJ-25 一行安装与升级**（D 域）：下载预构建产物 →
解包到版本化目录 → 链接进 PATH → `ar --version`/`ar init` 即可用；
重跑安装 = 升级，且**永不覆盖运行中的二进制**（deploy.sh 两条血泪
规则之一，升级路径同样遵守）。§5 索引"云与远程"组加对应条目。

对比 handa 的两点简化（都是 Go 单二进制红利，DESIGN 决策 #1 的兑现）：
- handa 要在每个 OS 的原生 runner 上打 Python bundle（原生 wheel 平台
  绑定）；我们 `CGO_ENABLED=0` 从单个 ubuntu runner 交叉编译全部
  target，产物就是两个静态二进制的 tar.gz。
- handa 的 launcher shim 要解决 bundle 按 $0 定位 runtime 的问题；Go
  二进制没有这个问题，bin 目录里放 symlink 即可。

## Spec delta

SPEC §J（运行形态与云）新增一行：

- **curl 一行安装分发**：install.sh（多平台探测、私有 repo token 下载
  路径、sha256 校验、版本化解包 + symlink 切换）+
  scripts/package-release.sh（4 target 交叉编译打包）+ release
  workflow（tag → 构建 → smoke → 发布稳定命名资产）。状态 🟡：gate A
  （离线孪生 scripts/test-install.sh，进 check.sh）已常跑，gate B 的
  workflow 构建+smoke 段已在 Actions 真跑绿，"tag 发布 → 公网 curl 安装"
  全程（QA-63）留待首个真实 release 执行后转 ✅。

## Design delta

DESIGN §12（Surfaces）新增小节"分发与安装（INC-63）"，登记：

- 产物形态：`agentrunner-<version>-<target>.tar.gz`（内含 `ar`、
  `arwebui`），target ∈ {linux-x86_64, linux-arm64, macos-arm64,
  macos-x86_64}；同名 `.sha256` 伴随。release 另挂稳定命名副本
  `agentrunner-<target>.tar.gz` 供 install.sh 免解析版本号直取。
- 两个二进制以 `-ldflags "-X main.version=<tag>"` 打同一版本戳（沿用
  deploy.sh 的 skew 检测机制）。
- 安装布局：`$AR_HOME/releases/<version>/`（默认
  `~/.local/share/agentrunner/releases/`，与 deploy.sh 的 `bin/` 同根
  不同目录，互不干扰）+ `$AR_BIN_DIR`（默认 `~/.local/bin`）下的
  symlink。升级 = 新版本目录 + symlink 原子切换；同版本重装先解到
  临时目录再整体替换——两条路径都不对运行中的 inode 做原地写。
- 私有 repo：install.sh 见 `GITHUB_TOKEN`/`GH_TOKEN` 时走 GitHub API
  （release → asset id → `Accept: application/octet-stream`）；无 token
  走公开 browser download URL（repo 转公开后即免 token）。token 只进
  请求头，不落盘不回显。
- macOS 侧：无签名/公证；curl 下载不打 quarantine xattr，Gatekeeper
  不拦，原型阶段接受，记 §17 注记不单列。
- Windows：裁掉。daemon 走 unix socket，Windows 形态本身未验证，
  没有"能装但不能跑"的发布物存在的理由；待 Windows 支持立项时随行。

**不触任何既有不变量**（纯 additive 的发布面；daemon/journal/权限
语义零改动）。

## 验收

双闸门：

- **闸门 A（scripted 孪生，进 check.sh 常跑）**：`scripts/test-install.sh`
  ——离线、确定性。伪造 stub 二进制打出与真产物同构的 tar.gz + sha256，
  以 `AR_ASSET_URL=file://…` + 临时 `AR_HOME`/`AR_BIN_DIR` 驱动真
  install.sh，逐项断言：
  1. 解包落 `releases/<version>/`，`ar`/`arwebui` symlink 生效且可执行；
  2. 装第二个版本 = symlink 切到新版、旧版本目录保留；
  3. 同版本重装不留残缺（临时目录整体替换）；
  4. sha256 篡改 → 安装失败、bin 链接不被破坏；
  5. 不支持平台报错文案（`uname` 注入）。
  （临时 `AR_HOME` 是在参数化安装器目标目录，不是给 daemon/session
  测试开隔离沙箱，不违反 QA 共享目录规则。）
- **闸门 B（真实环境，QA-63 入 QA.md 菜单）**：release workflow 在
  Actions 真跑：构建全部 4 target → linux-x86_64 产物真解包、
  `ar --version`/`ar init` + `arwebui` 起服 `/api/health` 探活 →（tag
  时）发布 release。本增量内以 workflow_dispatch 真跑一轮构建+smoke；
  "tag → 公网 curl -fsSL | sh 安装"末段显式声明留待首个真实 release
  （发布是用户决策，不代做），完成后 SPEC 行转 ✅。

裁掉项显式声明：Windows 产物与 install.ps1（理由见 Design delta）；
Intel Mac 实机验证（交叉编译产物 smoke 只在 linux runner 跑，macos
产物的实机验证并入首个 release 的 QA-63）。

## 实施步骤

1. **INC-63.1**：工作纸入库（本文件）。
2. **INC-63.2**：`scripts/package-release.sh` + `install.sh` +
   `scripts/smoke-release.sh` + `scripts/test-install.sh`（wired 进
   check.sh）+ `.github/workflows/release.yml` + README 安装节。
   完成标志：check.sh 全绿（含新孪生）。
3. **INC-63.3**：三层收口（JOURNEYS UJ-25 + §5 / SPEC §J / DESIGN §12）
   + QA.md QA-63 + HANDA-PARITY 域二 #2 状态更新 + LOG 条目 + 工作纸
   归档。完成标志：check.sh 全绿、Actions workflow_dispatch 构建+smoke
   绿。

## review 裁决

裁掉三视角对抗 review：小增量，纯 additive 发布基建，不触 runtime
语义/并发/权限面；正确性由闸门 A 孪生 + Actions 真跑 smoke 双腿覆盖；
安全面唯一新增暴露是 install.sh 的 token 处理（只进请求头、不 echo、
不落盘），在孪生里以无 token 路径 + 代码走查覆盖。
