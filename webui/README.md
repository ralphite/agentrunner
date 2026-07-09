# arwebui — AgentRunner 驾驶舱(Codex 风格)

一个 Codex 云端风格的本机 web app,以真实用户的方式驱动 AgentRunner 的
**全部**会话能力:chat、插话排队、interrupt、close、resume、起/杀子
agent、图片输入、审批(含子 agent 上卷)、fork、换 agent、trust、后台
运行(submit / drive),以及崩溃恢复观察。

与 `web/`(极简 vanilla 驾驶舱)并列、独立:同样只通过公开的 `ar` CLI
契约(`os/exec`)与系统交互,是测试/QA 面,不是产品的一部分,不进
`docs/` 三层定义,不参与 `scripts/check.sh` 闸门。

- **后端** `arwebui`:stdlib-only Go(独立 module),把每个请求翻成一次
  `ar` 子命令,并把 Vite 构建产物嵌进二进制。
- **前端** `frontend/`:React + TypeScript + Vite。Codex 云端观感:
  - **首页 hero composer**:居中大输入框 +「Ask / Code」双动作
    (Ask=起 chat 会话,Code=submit 后台任务),下面是**任务卡片**网格
    (标题取开场消息,像 Codex 用任务描述当标题)。
  - **会话详情**:顶栏动作条 +「对话 / 改动」切换。对话是时间线
    (气泡/工具卡/审批/子 agent);**改动**是 Codex 式的 git diff 视图
    (改动文件列表 + 逐行 +/- 着色 + untracked 新文件)。
  - 底部 composer、右侧在飞任务面板、审批 dock。

## 快速上手

```bash
# 1. 构建 ar 二进制(仓库根)
go build -o /tmp/ar ./cmd/agentrunner

# 2. 构建前端(首次或改前端后)
cd webui/frontend && npm install && npm run build && cd ..

# 3. 构建并起驾驶舱(--env-file 把 GEMINI_API_KEY 递给 daemon)
go run . --ar /tmp/ar --env-file ../.env

# 4. 浏览器打开
open http://127.0.0.1:8788
```

> macOS 的 unix socket 路径有 104 字节上限。若 `$XDG_DATA_HOME` 过长会让
> daemon bind 失败——测试时用一个短路径,例如
> `XDG_DATA_HOME=/tmp/awui go run . ...`。

## 参数

| flag | 默认 | 说明 |
|---|---|---|
| `--ar` | `ar` | agentrunner 二进制路径 |
| `--addr` | `127.0.0.1:8788` | 监听地址(只应绑本机) |
| `--env-file` | (空) | KEY=VALUE 文件,载入环境后再 spawn daemon(传 `../.env`) |
| `--no-daemon` | false | 不托管 daemon,用外部已运行的 |
| `--runtime` | `./runtime` | 便利产物目录(specs/uploads/ws/runs/daemon.log,gitignored) |

## 功能 ↔ CLI 映射

| UI | ar 命令 |
|---|---|
| 首页任务卡 / 会话列表 | `sessions list`(标题/workspace 由 arwebui 侧记的元数据补全) |
| Ask / 新会话 / 开场消息 | `new --workspace W [--mode M] base.yaml "msg"` |
| 改动(Diff 视图) | `git -C <workspace> diff` + `status --porcelain`(workspace 仅 arwebui 建的会话可知) |
| 发消息 / 图片 | `send [--image f]... <sid> "text"` |
| 时间线(真相) | `events --json <sid>`(1s 增量轮询) |
| 流式打字 / 子审批上卷 | `attach --json <sid>`(SSE) |
| interrupt / close / resume | `interrupt` / `close` / `resume` |
| 在飞任务 + kill | `ps` / `kill <sid> <handle>` |
| 审批 | `approve <sid> <id> approve\|deny [reason]` |
| fork(选 barrier) | `fork --list` + `fork <sid> <barrier> [--workspace]` |
| 换 agent | `agent <sid> base.yaml` |
| 后台运行 submit / drive | `submit --json …` / `drive --json …`(SSE 流式日志) |
| trust | `trust <dir>` |
| 查看面板 | `events`(raw) / `events --state` / `inspect --json` |

## 开发

```bash
cd webui
gofmt -l . && go vet ./... && go test ./...   # 后端闸门
cd frontend && npm run build                  # 前端构建(产物嵌入二进制)
```

后端单测覆盖 CLI 输出解析的易碎点(session id 落在 stderr、daemon 不可达
的多种措辞)。真实验证按全局 QA 纪律走真 daemon + 真 Gemini 的端到端
UI 流程。
