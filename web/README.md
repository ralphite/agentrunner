# arweb — AgentRunner 测试驾驶舱

极简本机 web UI,以真实用户的方式驱动 AgentRunner 会话:chat、忙时
插话、interrupt、起/杀子 agent、图片输入、审批、崩溃恢复观察。
设计与不变量见 [DESIGN.md](DESIGN.md),开发台账见 [PROGRESS.md](PROGRESS.md)。

## 快速上手

```bash
# 1. 构建 ar 二进制(仓库根)
go build -o /tmp/ar ./cmd/agentrunner

# 2. 起驾驶舱(web/ 目录;--env-file 把 GEMINI_API_KEY 递给 daemon)
cd web
go run . --ar /tmp/ar --env-file ../.env

# 3. 浏览器打开
open http://127.0.0.1:8787
```

首次使用:点「新会话」→ 表单里已预填 base.yaml / worker.yaml(可改)
→ 点「造空 workspace」或填任意目录 → 写开场消息 → 创建。之后就是
聊天界面:发消息、传图、interrupt、右侧面板看在飞子任务并可 kill、
待审批时出现批准/拒绝卡。

## 参数

| flag | 默认 | 说明 |
|---|---|---|
| `--ar` | `ar` | agentrunner 二进制路径 |
| `--addr` | `127.0.0.1:8787` | 监听地址(只应绑本机) |
| `--env-file` | (空) | KEY=VALUE 文件,载入环境后再 spawn daemon(传 `../.env`) |
| `--no-daemon` | false | 不托管 daemon,用外部已运行的 |
| `--runtime` | `./runtime` | 便利产物目录(specs/uploads/ws/daemon.log,gitignored) |

## 常见问题

- **health 红点 / "is the daemon running"**:daemon 没起来。检查
  `runtime/daemon.log`;最常见是 `GEMINI_API_KEY` 没进环境(用
  `--env-file ../.env`)。页面上也有「重启 daemon」按钮。
- **端口被占**:`--addr 127.0.0.1:8788`。
- **外部 daemon**:你自己 `ar daemon` 起的也行(`--no-daemon`),
  但注意 arweb 和它必须看到同一个 `$XDG_DATA_HOME`。
- **workspace 用 QA profile**:仓库根 `qa/ws.sh prepare gin /tmp/wsX`
  造好后把路径填进表单即可(arweb 不依赖 qa/,只是路径互通)。

## 开发

```bash
cd web
gofmt -l . && go vet ./... && go test ./...   # 全绿才算一步完成
```

单测用 fake-ar(脚本桩)不打真 API;真实验证(真 daemon + 真
Gemini)按 PROGRESS.md 各 milestone 的"真验"栏执行。
