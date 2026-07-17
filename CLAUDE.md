# AgentRunner — 项目约定

## Git 规则（硬性）

- **只用 main 分支。** 不创建任何其他分支；不在其他分支上工作。
- **每次改动完成后立即 commit 并 push 到 `origin/main`。** 不留未推送
  的本地提交，不留未提交的工作区改动。单人原型项目——分叉和滞后的
  代价远大于中间态提交的噪音。
- **每个 session 开始时先 `git fetch origin main` 并 fast-forward**，
  确保永远在最新代码上工作（曾发生过基于过时设计文档产出整份
  review 的事故）。
- **永远合并进 `origin/main`。** 即使工具把你放在 worktree / feature
  分支上工作，完成后也要把改动合并进 `origin/main` 并 push——不要停在
  feature 分支上等确认。用户已长期授权此操作（能 fast-forward 时
  `git push origin HEAD:main` 直接推；不能时先 rebase/cherry-pick 到
  最新 `origin/main` 上、解冲突、rebuild 再推。留意可能有并发 session
  也在推 main）。
- `.env` 已 gitignore（存本地凭据如 `GEMINI_API_KEY`），永不提交。

## QA / 测试数据规则（硬性）

- **一律用共享数据目录测试。** 默认走全局 daemon 与 store
  （`~/.local/share/agentrunner/`），**不**起 `HOME`/`XDG_DATA_HOME`
  隔离沙箱——除非用户明确要求隔离。这样测试产生的会话能在用户日常
  用的 CLI（`ar sessions`）与 webui 里直接看到、复现、追问。
- **测试数据一律保留，供事后核查。** 测完**不** close、不删除会话，
  不清理 workspace / journal / daemon store。需要清理时先问用户。
  `ar events` 导出与 workspace diff 归档到 `qa/runs/<日期>-<QA号>/`。
- 例外仅限**破坏性**测试（如 `kill -9` daemon）会波及用户在跑的真实
  会话时：先告知用户、征得同意，或在用户确认的时间窗内做——不擅自
  为"图省事"而隔离。

## 文档体系（全部住在 `docs/`，流程与冲突裁决见 `docs/PROCESS.md`）

三层产品定义 + 三份支撑件，共 7 份活文档：

- `docs/JOURNEYS.md` — 第一层：端到端 user journey（产品要做什么）。
- `docs/SPEC.md` — 第二层：功能点登记簿（有什么、什么状态、验收锚）。
- `docs/DESIGN.md` — 第三层：架构 source of truth（怎么成立、为什么）。
- `docs/QA.md` — journey 级真实 API 验收场景菜单（脚本在 `qa/`）。
- `docs/GAPS.md` — 审计件：journey × 设计/实现的缺口登记。
- `docs/LOG.md` — 增量与决策台账（只追加）。
- `docs/PROCESS.md` — 以上一切的流程：三层模型、增量开发流程、
  双闸门测试纪律、执行协议。**改任何文档前先读它。**

硬性规则：
- 动 `docs/DESIGN.md` 不变量必须走 PROCESS.md 的"不变量变更流程"
  （停下、写清冲突、单独 review），禁止代码里先绕。
- 新需求/新功能一律走 PROCESS.md 的增量流程（三层 delta 明确后再
  开发），不直接动手写代码。

## 语言与实现约定

- 叙述用中文，技术术语/代码/标识符用英文。
- 实现语言 Go 1.25+ 且使用受支持分支的安全 patch（DESIGN 决策 #1）；
  主 provider Gemini、次 Anthropic。
- 一步完成的标准：`./scripts/check.sh` 全绿 + 相关文档行齐活。

## 测试环境与 CI/CD

### GitHub Actions 预置环境（推荐用于黑盒测试）

**优势**：GitHub Actions runners 上已配置 GEMINI_API_KEY 和 ANTHROPIC_API_KEY，
无需本地 .env 配置，直接可运行包含 agent turn 的真实功能测试。

**可用 Workflows**（repo Settings → Actions 查看或从代码 dispatch）：

1. **qa-blackbox** — 黑盒 UI 测试（推荐用于 Tailwind 迁移/UI parity 验证）
   - 路径：`.github/workflows/qa-blackbox.yml`
   - 启动方式：GitHub Actions → qa-blackbox → Run workflow
   - 功能：真浏览器驱动真 webui，真 Gemini turn，验收标准：无 console 错误、无内部错误、无横向溢出，全程截图
   - 输出：artifacts/blackbox-run/（findings.json + 截图 + 日志）
   - 时长：~30 分钟
   - 环境变量：自动加载 GEMINI_API_KEY（secrets）

2. **phone-webui** — 移动端远程驾驶（Tailscale 穿透）
   - 路径：`.github/workflows/phone-webui.yml`
   - 启动：Actions → phone-webui → Run workflow，或自动每 30 分钟启动一次
   - 功能：启动 arwebui，通过 Tailscale 暴露给手机/Mac，会话数据跨 run 延续
   - 入参：minutes（30-340），smoke（仅构建+健康检查）
   - 所需 secrets：TS_AUTHKEY（可选），GEMINI_API_KEY/ANTHROPIC_API_KEY
   - 用例：长时间交互式测试，无需本地 API key

3. **release** — 生产构建 + smoke 测试
   - 路径：`.github/workflows/release.yml`
   - 触发：push tag v*（自动），或 Actions 手动 dispatch
   - 功能：跨平台打包（linux-x86_64/arm64、macos-arm64/x86_64）+ 安装脚本验证
   - 输出：GitHub Release assets + installer 验证

### 如何启用 Secrets（一次性配置）

1. repo 主页 → Settings → Secrets and variables → Actions
2. 新增 secrets：
   - `GEMINI_API_KEY`：从 Google AI Studio 获取（首选 provider）
   - `ANTHROPIC_API_KEY`：Anthropic console（备用 provider）
   - `TS_AUTHKEY`：Tailscale admin console → Settings → Keys（phone-webui 远程用）
3. 保存后 workflows 自动可访问，不会在日志中泄漏

### 与本地开发的协作模式

| 场景 | 推荐做法 |
|------|--------|
| 快速原型 + 单元测试 | 本地开发（`npm run dev` + `go run`） |
| UI parity 验证（Tailwind 迁移等） | **GitHub Actions qa-blackbox**（真浏览器 + 真 API） |
| 长交互式会话验证 | **phone-webui**（Tailscale 穿透到手机） |
| 集成测试 + agent turn | **GitHub Actions qa-blackbox** 或本地 + GEMINI_API_KEY |
| 发布前全量验证 | **release workflow**（smoke 三腿） |

### 本地快速启动模板

若本地有 GEMINI_API_KEY，启动完整栈：
```bash
# 1. 构建
go build -o bin/ar ./cmd/agentrunner
(cd webui/frontend && npm ci && npm run build)
(cd webui && go build -o ../bin/arwebui .)

# 2. 启动
export GEMINI_API_KEY="$(cat ~/.env | grep GEMINI_API_KEY | cut -d= -f2)"
./bin/arwebui -ar "$PWD/bin/ar" -addr 127.0.0.1:8788
# 访问 http://localhost:8788
```

## 历史归档

- `docs/archive/` 存已完成计划（v1 S1–S7、v2 M1–M5）与旧审查件，
  只读；与活文档冲突时以活文档为准。索引见 `docs/archive/README.md`。
